#!/usr/bin/env bash

set -euo pipefail

API_BASE="http://127.0.0.1:8080"
OUT_DIR="pilot_artifacts/cloud-ready-$(date +%Y%m%d-%H%M%S)"
SKIP_READYZ=0
PROBE_TIMEOUT_MS="${FLOWFORGE_CLOUD_PROBE_TIMEOUT_MS:-800}"

POSTGRES_ADDR="${FLOWFORGE_CLOUD_POSTGRES_ADDR:-127.0.0.1:15432}"
REDIS_ADDR="${FLOWFORGE_CLOUD_REDIS_ADDR:-127.0.0.1:16379}"
NATS_HEALTH_URL="${FLOWFORGE_CLOUD_NATS_HEALTH_URL:-http://127.0.0.1:18222/healthz}"
MINIO_HEALTH_URL="${FLOWFORGE_CLOUD_MINIO_HEALTH_URL:-http://127.0.0.1:19000/minio/health/live}"

usage() {
  cat <<EOF
Usage: ./scripts/cloud_ready_smoke.sh [options]

Options:
  --api-base <url>   API base for ready probe (default: http://127.0.0.1:8080)
  --out-dir <path>   Output artifact directory
  --skip-readyz      Skip API /readyz verification
  --timeout-ms <n>   Probe timeout in milliseconds (default: 800)
  -h, --help         Show this help text.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --api-base)
      API_BASE="$2"
      shift 2
      ;;
    --out-dir)
      OUT_DIR="$2"
      shift 2
      ;;
    --skip-readyz)
      SKIP_READYZ=1
      shift
      ;;
    --timeout-ms)
      PROBE_TIMEOUT_MS="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if ! [[ "$PROBE_TIMEOUT_MS" =~ ^[0-9]+$ ]] || [[ "$PROBE_TIMEOUT_MS" -le 0 ]]; then
  echo "ERROR: --timeout-ms must be a positive integer." >&2
  exit 1
fi

timeout_sec="$(awk "BEGIN {printf \"%.3f\", ${PROBE_TIMEOUT_MS}/1000}")"
curl_timeout_sec="$(awk "BEGIN {printf \"%.1f\", ${PROBE_TIMEOUT_MS}/1000}")"

mkdir -p "$OUT_DIR"
summary_file="$OUT_DIR/summary.tsv"
readyz_file="$OUT_DIR/readyz.json"

echo -e "check\tstatus\tdetail" > "$summary_file"

status=0

record_pass() {
  local check="$1"
  local detail="$2"
  echo -e "${check}\tPASS\t${detail}" >> "$summary_file"
}

record_fail() {
  local check="$1"
  local detail="$2"
  echo -e "${check}\tFAIL\t${detail}" >> "$summary_file"
  status=1
}

probe_tcp() {
  local check="$1"
  local addr="$2"
  local host="${addr%:*}"
  local port="${addr##*:}"
  if command -v nc >/dev/null 2>&1; then
    if nc -z -w "$curl_timeout_sec" "$host" "$port" >/dev/null 2>&1; then
      record_pass "$check" "$addr"
      return 0
    fi
    record_fail "$check" "$addr"
    return 0
  fi

  if python3 - "$host" "$port" "$timeout_sec" <<'PY' >/dev/null 2>&1; then
import socket
import sys

host = sys.argv[1]
port = int(sys.argv[2])
timeout = float(sys.argv[3])
s = socket.socket()
s.settimeout(timeout)
s.connect((host, port))
s.close()
PY
    record_pass "$check" "$addr"
    return 0
  fi
  record_fail "$check" "$addr"
}

probe_http() {
  local check="$1"
  local url="$2"
  local http_code
  http_code="$(curl -sS --max-time "$curl_timeout_sec" -o /dev/null -w "%{http_code}" "$url" || true)"
  if [[ "$http_code" == "200" ]]; then
    record_pass "$check" "$url"
  else
    record_fail "$check" "${url} (http=${http_code:-ERR})"
  fi
}

probe_readyz() {
  local readyz_url="${API_BASE%/}/readyz"
  local curl_rc=0
  local http_code
  http_code="$(curl -sS --max-time "$curl_timeout_sec" -o "$readyz_file" -w "%{http_code}" "$readyz_url")" || curl_rc=$?

  if [[ "$curl_rc" -ne 0 ]]; then
    record_fail "api_readyz" "${readyz_url} (curl_exit=${curl_rc})"
    return 0
  fi

  compact_payload="$(tr -d '\n\r\t ' < "$readyz_file")"
  if [[ "$http_code" != "200" ]]; then
    record_fail "api_readyz" "${readyz_url} (http=${http_code})"
    return 0
  fi
  if [[ "$compact_payload" != *'"status":"ready"'* ]]; then
    record_fail "api_readyz" "${readyz_url} (status!=ready)"
    return 0
  fi
  if [[ "$compact_payload" != *'"cloud_dependencies_required":true'* ]]; then
    record_fail "api_readyz" "${readyz_url} (cloud_dependencies_required!=true)"
    return 0
  fi

  record_pass "api_readyz" "$readyz_url"
}

echo "== FlowForge cloud readiness smoke =="
echo "Output: $OUT_DIR"
echo "Timeout(ms): $PROBE_TIMEOUT_MS"
echo "Postgres: $POSTGRES_ADDR"
echo "Redis: $REDIS_ADDR"
echo "NATS: $NATS_HEALTH_URL"
echo "MinIO: $MINIO_HEALTH_URL"
echo "API base: $API_BASE"
echo "Skip /readyz: $SKIP_READYZ"

probe_tcp "cloud_postgres" "$POSTGRES_ADDR"
probe_tcp "cloud_redis" "$REDIS_ADDR"
probe_http "cloud_nats" "$NATS_HEALTH_URL"
probe_http "cloud_minio" "$MINIO_HEALTH_URL"

if [[ "$SKIP_READYZ" == "0" ]]; then
  probe_readyz
else
  echo -e "api_readyz\tSKIP\tnot requested" >> "$summary_file"
fi

if [[ "$status" -ne 0 ]]; then
  echo "❌ Cloud readiness smoke failed. See: $summary_file" >&2
  exit 1
fi

echo "✅ Cloud readiness smoke passed. Summary: $summary_file"

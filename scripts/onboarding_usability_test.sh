#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

OUT_DIR="${1:-pilot_artifacts/onboarding-$(date +%Y%m%d-%H%M%S)}"
LOG_DIR="$OUT_DIR/logs"
REPORT_FILE="$OUT_DIR/report.md"

mkdir -p "$LOG_DIR"

API_PORT="${API_PORT:-8080}"
DASHBOARD_PORT="${DASHBOARD_PORT:-3001}"
DB_PATH="$OUT_DIR/onboarding.db"

START_TS="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
START_EPOCH="$(date +%s)"

OVERALL_STATUS="PASS"
SUMMARY_LINES=()

API_PID=""
DASHBOARD_PID=""

cleanup() {
  if [[ -n "$API_PID" ]]; then
    kill "$API_PID" >/dev/null 2>&1 || true
    wait "$API_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "$DASHBOARD_PID" ]]; then
    kill "$DASHBOARD_PID" >/dev/null 2>&1 || true
    wait "$DASHBOARD_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
}

record_step() {
  local step="$1"
  local status="$2"
  local duration="$3"
  local log_file="$4"
  SUMMARY_LINES+=("- ${step}: ${status} (${duration}s) - \`${log_file}\`")
  if [[ "$status" != "PASS" ]]; then
    OVERALL_STATUS="FAIL"
  fi
}

run_step() {
  local step="$1"
  shift
  local slug
  slug="$(echo "$step" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9' '_')"
  local log_file="$LOG_DIR/${slug}.log"
  local t0 t1 dur

  t0="$(date +%s)"
  if "$@" >"$log_file" 2>&1; then
    t1="$(date +%s)"
    dur=$((t1 - t0))
    record_step "$step" "PASS" "$dur" "$log_file"
    return 0
  fi

  t1="$(date +%s)"
  dur=$((t1 - t0))
  record_step "$step" "FAIL" "$dur" "$log_file"
  return 1
}

wait_for_http() {
  local url="$1"
  local retries="${2:-30}"
  local delay="${3:-1}"
  local i
  for ((i = 1; i <= retries; i++)); do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$delay"
  done
  return 1
}

require_cmd go
require_cmd npm
require_cmd curl
require_cmd python3

rm -f "$DB_PATH"
export FLOWFORGE_DB_PATH="$DB_PATH"

if command -v lsof >/dev/null 2>&1; then
  lsof -t -i :"${API_PORT}" -i :"${DASHBOARD_PORT}" | xargs kill -9 2>/dev/null || true
fi

run_step "Install Build Path" ./scripts/install.sh --no-services --no-demo || true
run_step "Demo Run" ./flowforge demo || true

DEMO_LOG="$(printf '%s\n' "${SUMMARY_LINES[@]}" | awk -F'`' '/Demo Run:/ {print $2; exit}')"
if [[ -n "${DEMO_LOG:-}" ]]; then
  if ! grep -q "Runaway detected in" "$DEMO_LOG"; then
    OVERALL_STATUS="FAIL"
    SUMMARY_LINES+=("- Demo summary check: FAIL (missing 'Runaway detected in') - \`${DEMO_LOG}\`")
  else
    SUMMARY_LINES+=("- Demo summary check: PASS (found 'Runaway detected in') - \`${DEMO_LOG}\`")
  fi
  if ! grep -q "CPU peaked at" "$DEMO_LOG"; then
    OVERALL_STATUS="FAIL"
    SUMMARY_LINES+=("- Demo CPU check: FAIL (missing 'CPU peaked at') - \`${DEMO_LOG}\`")
  else
    SUMMARY_LINES+=("- Demo CPU check: PASS (found 'CPU peaked at') - \`${DEMO_LOG}\`")
  fi
  if ! grep -q "Process recovered" "$DEMO_LOG"; then
    OVERALL_STATUS="FAIL"
    SUMMARY_LINES+=("- Demo recovery check: FAIL (missing 'Process recovered') - \`${DEMO_LOG}\`")
  else
    SUMMARY_LINES+=("- Demo recovery check: PASS (found 'Process recovered') - \`${DEMO_LOG}\`")
  fi
fi

API_LOG="$LOG_DIR/api_server.log"
(
  ./flowforge dashboard
) >"$API_LOG" 2>&1 &
API_PID=$!

if wait_for_http "http://127.0.0.1:${API_PORT}/healthz" 40 1; then
  SUMMARY_LINES+=("- API startup: PASS - \`${API_LOG}\`")
else
  OVERALL_STATUS="FAIL"
  SUMMARY_LINES+=("- API startup: FAIL - \`${API_LOG}\`")
fi

run_step "API Health Probe" curl -fsS "http://127.0.0.1:${API_PORT}/healthz" || true
run_step "API Timeline Probe" curl -fsS "http://127.0.0.1:${API_PORT}/timeline" || true

DASHBOARD_LOG="$LOG_DIR/dashboard_server.log"
(
  cd dashboard
  NEXT_PUBLIC_FLOWFORGE_API_BASE="http://localhost:${API_PORT}" npm run start -- -p "${DASHBOARD_PORT}"
) >"$DASHBOARD_LOG" 2>&1 &
DASHBOARD_PID=$!

if wait_for_http "http://127.0.0.1:${DASHBOARD_PORT}" 40 1; then
  SUMMARY_LINES+=("- Dashboard startup: PASS - \`${DASHBOARD_LOG}\`")
else
  OVERALL_STATUS="FAIL"
  SUMMARY_LINES+=("- Dashboard startup: FAIL - \`${DASHBOARD_LOG}\`")
fi

run_step "Dashboard Root Probe" curl -fsS "http://127.0.0.1:${DASHBOARD_PORT}" || true

END_EPOCH="$(date +%s)"
TOTAL_SECONDS=$((END_EPOCH - START_EPOCH))

if (( TOTAL_SECONDS > 300 )); then
  OVERALL_STATUS="FAIL"
  SUMMARY_LINES+=("- Time-to-first-value target: FAIL (>${TOTAL_SECONDS}s, target <=300s) - \`${REPORT_FILE}\`")
else
  SUMMARY_LINES+=("- Time-to-first-value target: PASS (${TOTAL_SECONDS}s, target <=300s) - \`${REPORT_FILE}\`")
fi

{
  echo "# Onboarding Usability Report"
  echo
  echo "Date (UTC): ${START_TS}"
  echo "Total Duration: ${TOTAL_SECONDS}s"
  echo "Overall Status: ${OVERALL_STATUS}"
  echo
  echo "## Step Results"
  printf '%s\n' "${SUMMARY_LINES[@]}"
  echo
  echo "## Notes"
  echo "- This script validates the first-run path from install to visible API/dashboard readiness."
  echo "- For true usability validation, run this with a developer who has not worked on FlowForge before."
} >"$REPORT_FILE"

echo "Onboarding usability test complete: $REPORT_FILE"
echo "Overall status: $OVERALL_STATUS"

if [[ "$OVERALL_STATUS" != "PASS" ]]; then
  exit 1
fi

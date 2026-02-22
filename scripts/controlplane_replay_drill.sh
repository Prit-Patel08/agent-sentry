#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

API_BASE="${API_BASE:-http://127.0.0.1:8080}"
API_KEY="${FLOWFORGE_API_KEY:-}"
OUT_DIR=""

usage() {
  cat <<'EOF'
Usage: ./scripts/controlplane_replay_drill.sh [options]

Options:
  --api-base URL   API base URL (default: http://127.0.0.1:8080 or API_BASE env).
  --api-key KEY    API key (default: FLOWFORGE_API_KEY env).
  --out DIR        Output directory (default: pilot_artifacts/controlplane-replay-<timestamp>).
  -h, --help       Show this help text.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --api-base)
      API_BASE="${2:-}"
      shift 2
      ;;
    --api-key)
      API_KEY="${2:-}"
      shift 2
      ;;
    --out)
      OUT_DIR="${2:-}"
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

if [[ -z "$API_KEY" ]]; then
  echo "FLOWFORGE_API_KEY is required (or pass --api-key)." >&2
  exit 1
fi

if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="pilot_artifacts/controlplane-replay-$(date +%Y%m%d-%H%M%S)"
fi
mkdir -p "$OUT_DIR"

for cmd in curl awk grep; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
done

curl -fsS --max-time 4 "${API_BASE}/healthz" >"${OUT_DIR}/healthz.json" 2>"${OUT_DIR}/healthz.stderr"

has_replay_header() {
  local header_file="$1"
  grep -qi '^X-Idempotent-Replay:[[:space:]]*true' "$header_file"
}

has_idempotency_conflict_error() {
  local body_file="$1"
  grep -qi 'idempotency key reused' "$body_file"
}

post_mutation() {
  local endpoint="$1"
  local key="$2"
  local body="$3"
  local name="$4"
  local code

  code="$(curl -sS -D "${OUT_DIR}/${name}.headers" -o "${OUT_DIR}/${name}.json" -w "%{http_code}" \
    -X POST \
    -H "Authorization: Bearer ${API_KEY}" \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: ${key}" \
    --data "$body" \
    "${API_BASE}${endpoint}" 2>"${OUT_DIR}/${name}.stderr" || true)"

  echo "$code"
}

run_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
suffix="$(date +%s)"
workspace_id="ws-replay-drill-${suffix}"
workspace_path="/tmp/${workspace_id}"

register_key="idem-register-${suffix}"
register_endpoint="/v1/integrations/workspaces/register"
register_body="{\"workspace_id\":\"${workspace_id}\",\"workspace_path\":\"${workspace_path}\",\"profile\":\"standard\",\"client\":\"drill\"}"
register_conflict_body="{\"workspace_id\":\"${workspace_id}-alt\",\"workspace_path\":\"/tmp/${workspace_id}-alt\",\"profile\":\"standard\",\"client\":\"drill\"}"

register_code_1="$(post_mutation "$register_endpoint" "$register_key" "$register_body" "register_first")"
register_code_2="$(post_mutation "$register_endpoint" "$register_key" "$register_body" "register_replay")"
register_code_3="$(post_mutation "$register_endpoint" "$register_key" "$register_conflict_body" "register_conflict")"

register_result="PASS"
if [[ "$register_code_1" != "200" ]]; then
  register_result="FAIL:first status=${register_code_1} expected 200"
elif [[ "$register_code_2" != "$register_code_1" ]]; then
  register_result="FAIL:replay status=${register_code_2} expected ${register_code_1}"
elif ! has_replay_header "${OUT_DIR}/register_replay.headers"; then
  register_result="FAIL:missing X-Idempotent-Replay header on register replay"
elif [[ "$register_code_3" != "409" ]]; then
  register_result="FAIL:conflict status=${register_code_3} expected 409"
elif ! has_idempotency_conflict_error "${OUT_DIR}/register_conflict.json"; then
  register_result="FAIL:register conflict payload missing idempotency conflict error"
fi

protection_key="idem-protection-${suffix}"
protection_endpoint="/v1/integrations/workspaces/${workspace_id}/protection"
protection_body='{"enabled":true,"reason":"drill protection noop"}'
protection_conflict_body='{"enabled":false,"reason":"drill protection conflict"}'

protection_code_1="$(post_mutation "$protection_endpoint" "$protection_key" "$protection_body" "protection_first")"
protection_code_2="$(post_mutation "$protection_endpoint" "$protection_key" "$protection_body" "protection_replay")"
protection_code_3="$(post_mutation "$protection_endpoint" "$protection_key" "$protection_conflict_body" "protection_conflict")"

protection_result="PASS"
if [[ "$protection_code_1" != "200" ]]; then
  protection_result="FAIL:first status=${protection_code_1} expected 200"
elif [[ "$protection_code_2" != "$protection_code_1" ]]; then
  protection_result="FAIL:replay status=${protection_code_2} expected ${protection_code_1}"
elif ! has_replay_header "${OUT_DIR}/protection_replay.headers"; then
  protection_result="FAIL:missing X-Idempotent-Replay header on protection replay"
elif [[ "$protection_code_3" != "409" ]]; then
  protection_result="FAIL:conflict status=${protection_code_3} expected 409"
elif ! has_idempotency_conflict_error "${OUT_DIR}/protection_conflict.json"; then
  protection_result="FAIL:protection conflict payload missing idempotency conflict error"
fi

actions_key="idem-actions-${suffix}"
actions_endpoint="/v1/integrations/workspaces/${workspace_id}/actions"
actions_body='{"action":"kill","reason":"drill action idempotency"}'
actions_conflict_body='{"action":"restart","reason":"drill action conflict"}'

actions_code_1="$(post_mutation "$actions_endpoint" "$actions_key" "$actions_body" "actions_first")"
actions_code_2="$(post_mutation "$actions_endpoint" "$actions_key" "$actions_body" "actions_replay")"
actions_code_3="$(post_mutation "$actions_endpoint" "$actions_key" "$actions_conflict_body" "actions_conflict")"

actions_result="PASS"
if [[ "$actions_code_1" -ge 500 ]]; then
  actions_result="FAIL:first status=${actions_code_1} expected <500"
elif [[ "$actions_code_2" != "$actions_code_1" ]]; then
  actions_result="FAIL:replay status=${actions_code_2} expected ${actions_code_1}"
elif ! has_replay_header "${OUT_DIR}/actions_replay.headers"; then
  actions_result="FAIL:missing X-Idempotent-Replay header on actions replay"
elif [[ "$actions_code_3" != "409" ]]; then
  actions_result="FAIL:conflict status=${actions_code_3} expected 409"
elif ! has_idempotency_conflict_error "${OUT_DIR}/actions_conflict.json"; then
  actions_result="FAIL:actions conflict payload missing idempotency conflict error"
fi

if curl -fsS --max-time 4 "${API_BASE}/metrics" >"${OUT_DIR}/metrics.prom" 2>"${OUT_DIR}/metrics.stderr"; then
  replay_total="$(awk '/^flowforge_controlplane_idempotent_replay_total /{print $2}' "${OUT_DIR}/metrics.prom" | tail -n 1)"
  conflict_total="$(awk '/^flowforge_controlplane_idempotency_conflict_total /{print $2}' "${OUT_DIR}/metrics.prom" | tail -n 1)"
  replay_total="${replay_total:-N/A}"
  conflict_total="${conflict_total:-N/A}"
else
  replay_total="N/A"
  conflict_total="N/A"
fi

overall="PASS"
for result in "$register_result" "$protection_result" "$actions_result"; do
  if [[ "$result" == FAIL:* ]]; then
    overall="FAIL"
    break
  fi
done

cat >"${OUT_DIR}/summary.tsv" <<EOF
run_timestamp	${run_ts}
api_base	${API_BASE}
workspace_id	${workspace_id}
register_first_code	${register_code_1}
register_replay_code	${register_code_2}
register_conflict_code	${register_code_3}
protection_first_code	${protection_code_1}
protection_replay_code	${protection_code_2}
protection_conflict_code	${protection_code_3}
actions_first_code	${actions_code_1}
actions_replay_code	${actions_code_2}
actions_conflict_code	${actions_code_3}
register_result	${register_result}
protection_result	${protection_result}
actions_result	${actions_result}
metrics_controlplane_replay_total	${replay_total}
metrics_controlplane_conflict_total	${conflict_total}
overall_status	${overall}
EOF

cat >"${OUT_DIR}/summary.md" <<EOF
# Control-Plane Replay Drill Summary

- Generated: \`${run_ts}\`
- API base: \`${API_BASE}\`
- Workspace ID: \`${workspace_id}\`
- Overall status: **${overall}**

## Scenario Results

1. Register endpoint replay + conflict: ${register_result}
2. Protection endpoint replay + conflict: ${protection_result}
3. Actions endpoint replay + conflict: ${actions_result}

## Metric Snapshot

- \`flowforge_controlplane_idempotent_replay_total\`: ${replay_total}
- \`flowforge_controlplane_idempotency_conflict_total\`: ${conflict_total}

## Artifacts

- \`${OUT_DIR}/summary.tsv\`
- \`${OUT_DIR}/register_first.json\`
- \`${OUT_DIR}/register_replay.json\`
- \`${OUT_DIR}/register_conflict.json\`
- \`${OUT_DIR}/protection_first.json\`
- \`${OUT_DIR}/protection_replay.json\`
- \`${OUT_DIR}/protection_conflict.json\`
- \`${OUT_DIR}/actions_first.json\`
- \`${OUT_DIR}/actions_replay.json\`
- \`${OUT_DIR}/actions_conflict.json\`
- \`${OUT_DIR}/metrics.prom\`
EOF

echo "Control-plane replay drill completed: ${OUT_DIR}/summary.md"
echo "Overall status: ${overall}"

if [[ "$overall" != "PASS" ]]; then
  exit 1
fi

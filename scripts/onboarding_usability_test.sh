#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

MODE="internal"
TESTER_NAME=""
TESTER_ROLE=""
OBSERVER_NAME=""
OUT_DIR=""

usage() {
  cat <<USAGE
Usage: ./scripts/onboarding_usability_test.sh [options] [out_dir]

Options:
  --mode MODE          internal|external (default: internal)
  --tester-name NAME   Required when --mode external
  --tester-role ROLE   Required when --mode external
  --observer-name NAME Optional observer name
  -h, --help           Show help text

Examples:
  ./scripts/onboarding_usability_test.sh --mode internal
  ./scripts/onboarding_usability_test.sh --mode external --tester-name "Alex" --tester-role "Backend Dev"
  ./scripts/onboarding_usability_test.sh --mode external --tester-name "Alex" --tester-role "Backend Dev" pilot_artifacts/onboarding-external-20260221
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode)
      MODE="${2:-}"
      shift 2
      ;;
    --tester-name)
      TESTER_NAME="${2:-}"
      shift 2
      ;;
    --tester-role)
      TESTER_ROLE="${2:-}"
      shift 2
      ;;
    --observer-name)
      OBSERVER_NAME="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --*)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
    *)
      if [[ -z "$OUT_DIR" ]]; then
        OUT_DIR="$1"
      else
        echo "Unexpected positional argument: $1" >&2
        usage >&2
        exit 1
      fi
      shift
      ;;
  esac
done

if [[ "$MODE" != "internal" && "$MODE" != "external" ]]; then
  echo "--mode must be one of: internal, external" >&2
  exit 1
fi

if [[ "$MODE" == "external" ]]; then
  if [[ -z "$TESTER_NAME" ]]; then
    echo "--tester-name is required when --mode external" >&2
    exit 1
  fi
  if [[ -z "$TESTER_ROLE" ]]; then
    echo "--tester-role is required when --mode external" >&2
    exit 1
  fi
fi

if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="pilot_artifacts/onboarding-$(date +%Y%m%d-%H%M%S)"
fi

LOG_DIR="$OUT_DIR/logs"
REPORT_FILE="$OUT_DIR/report.md"
SUMMARY_FILE="$OUT_DIR/summary.tsv"
FEEDBACK_FILE="$OUT_DIR/external_feedback.md"
OBSERVER_FILE="$OUT_DIR/observer_notes.md"

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

create_feedback_template_if_missing() {
  if [[ -f "$FEEDBACK_FILE" ]]; then
    return
  fi
  cat >"$FEEDBACK_FILE" <<EOT
# External Tester Feedback

Session metadata:
- Date (UTC): $START_TS
- Mode: $MODE
- Tester name: ${TESTER_NAME:-TODO}
- Tester role: ${TESTER_ROLE:-TODO}
- Observer: ${OBSERVER_NAME:-TODO}

1. Which step was least clear?
- Answer: TODO

2. Did any output look like internal debugging instead of product messaging?
- Answer: TODO

3. Could you repeat the flow without help tomorrow?
- Answer (yes/no): TODO
- Why: TODO

4. What was the most confusing moment?
- Answer: TODO

5. What one change would make onboarding easier?
- Answer: TODO
EOT
}

create_observer_template_if_missing() {
  if [[ -f "$OBSERVER_FILE" ]]; then
    return
  fi
  cat >"$OBSERVER_FILE" <<EOT
# Observer Notes

Session metadata:
- Date (UTC): $START_TS
- Tester name: ${TESTER_NAME:-TODO}
- Observer: ${OBSERVER_NAME:-TODO}

1. Any interventions required?
- Answer: TODO

2. Top 3 friction points
- 1: TODO
- 2: TODO
- 3: TODO

3. Recommended follow-up fixes (owner + due date)
- 1: TODO
- 2: TODO
- 3: TODO
EOT
}

is_completed_template() {
  local file="$1"
  if [[ ! -f "$file" ]]; then
    echo "NO"
    return
  fi
  if grep -q "TODO" "$file"; then
    echo "NO"
    return
  fi
  echo "YES"
}

require_cmd go
require_cmd npm
require_cmd curl
require_cmd python3

create_feedback_template_if_missing
create_observer_template_if_missing

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
  SUMMARY_LINES+=("- Time-to-first-value target: FAIL (${TOTAL_SECONDS}s, target <=300s) - \`${REPORT_FILE}\`")
  TTFV_TARGET_STATUS="FAIL"
else
  SUMMARY_LINES+=("- Time-to-first-value target: PASS (${TOTAL_SECONDS}s, target <=300s) - \`${REPORT_FILE}\`")
  TTFV_TARGET_STATUS="PASS"
fi

FEEDBACK_COMPLETE="$(is_completed_template "$FEEDBACK_FILE")"
OBSERVER_COMPLETE="$(is_completed_template "$OBSERVER_FILE")"

if [[ "$MODE" == "internal" ]]; then
  EXTERNAL_VALIDATION_STATUS="INTERNAL_DRY_RUN"
else
  if [[ "$FEEDBACK_COMPLETE" == "YES" && "$OBSERVER_COMPLETE" == "YES" ]]; then
    EXTERNAL_VALIDATION_STATUS="COMPLETE"
  else
    EXTERNAL_VALIDATION_STATUS="PENDING_HUMAN_INPUT"
  fi
fi

CHECKBOX_READINESS="NOT_READY"
if [[ "$MODE" == "external" && "$OVERALL_STATUS" == "PASS" && "$EXTERNAL_VALIDATION_STATUS" == "COMPLETE" ]]; then
  CHECKBOX_READINESS="READY_TO_MARK_PLAN_CHECKBOX"
fi

AUTOMATION_PASS="NO"
if [[ "$OVERALL_STATUS" == "PASS" ]]; then
  AUTOMATION_PASS="YES"
fi

cat >"$SUMMARY_FILE" <<EOT
start_ts_utc	${START_TS}
mode	${MODE}
tester_name	${TESTER_NAME:-N/A}
tester_role	${TESTER_ROLE:-N/A}
observer_name	${OBSERVER_NAME:-N/A}
out_dir	${OUT_DIR}
overall_status	${OVERALL_STATUS}
automation_pass	${AUTOMATION_PASS}
total_duration_seconds	${TOTAL_SECONDS}
time_to_first_value_target	${TTFV_TARGET_STATUS}
external_feedback_complete	${FEEDBACK_COMPLETE}
observer_notes_complete	${OBSERVER_COMPLETE}
external_validation_status	${EXTERNAL_VALIDATION_STATUS}
plan_checkbox_readiness	${CHECKBOX_READINESS}
EOT

{
  echo "# Onboarding Usability Report"
  echo
  echo "Date (UTC): ${START_TS}"
  echo "Mode: ${MODE}"
  echo "Tester Name: ${TESTER_NAME:-N/A}"
  echo "Tester Role: ${TESTER_ROLE:-N/A}"
  echo "Observer: ${OBSERVER_NAME:-N/A}"
  echo "Total Duration: ${TOTAL_SECONDS}s"
  echo "Overall Status: ${OVERALL_STATUS}"
  echo
  echo "## Step Results"
  printf '%s\n' "${SUMMARY_LINES[@]}"
  echo
  echo "## External Validation Gate"
  echo "- External feedback completed: ${FEEDBACK_COMPLETE}"
  echo "- Observer notes completed: ${OBSERVER_COMPLETE}"
  echo "- External validation status: ${EXTERNAL_VALIDATION_STATUS}"
  echo "- plan.md checkbox readiness: ${CHECKBOX_READINESS}"
  echo
  echo "## Artifacts"
  echo "- Summary TSV: \`${SUMMARY_FILE}\`"
  echo "- Feedback: \`${FEEDBACK_FILE}\`"
  echo "- Observer notes: \`${OBSERVER_FILE}\`"
  echo "- Logs: \`${LOG_DIR}\`"
  echo
  echo "## Notes"
  echo "- This script validates the first-run path from install to visible API/dashboard readiness."
  echo "- Mark plan.md external usability checkbox only when mode=external and readiness is READY_TO_MARK_PLAN_CHECKBOX."
} >"$REPORT_FILE"

echo "Onboarding usability test complete: $REPORT_FILE"
echo "Overall status: $OVERALL_STATUS"
echo "External validation status: $EXTERNAL_VALIDATION_STATUS"
echo "plan.md checkbox readiness: $CHECKBOX_READINESS"

if [[ "$OVERALL_STATUS" != "PASS" ]]; then
  exit 1
fi

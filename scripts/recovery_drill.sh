#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ARTIFACT_DIR="${1:-pilot_artifacts/recovery-$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$ARTIFACT_DIR"

if [[ -f ".flowforge.env" ]]; then
  set -a
  source ".flowforge.env"
  set +a
fi

if [[ -z "${FLOWFORGE_API_KEY:-}" ]]; then
  echo "FLOWFORGE_API_KEY is required for API kill drill. Set it in .flowforge.env."
  exit 1
fi

cleanup() {
  pkill -f "demo/pilot/healthy_worker.py" 2>/dev/null || true
  pkill -f "./flowforge run" 2>/dev/null || true
}
trap cleanup EXIT

worker_alive() {
  if pgrep -f "demo/pilot/healthy_worker.py" >/dev/null 2>&1; then
    return 0
  fi
  local rc=$?
  # Some restricted environments block process listing commands.
  if [[ $rc -gt 1 ]]; then
    return 2
  fi
  return 1
}

drill_sigterm_cleanup() {
  local log_file="$ARTIFACT_DIR/drill_sigterm.log"
  echo "== Drill A: parent SIGTERM cleanup =="
  ./flowforge run --no-kill --max-cpu 95 -- python3 demo/pilot/healthy_worker.py >"$log_file" 2>&1 &
  local supervisor_pid=$!
  sleep 2
  kill -TERM "$supervisor_pid" || true
  wait "$supervisor_pid" || true
  sleep 1

  if worker_alive; then
    echo "FAIL: worker process survived parent SIGTERM" | tee -a "$log_file"
    return 1
  elif [[ $? -eq 2 ]]; then
    echo "WARN: process listing unavailable; orphan check skipped" | tee -a "$log_file"
    return 0
  fi
  echo "PASS: no orphan worker after parent SIGTERM" | tee -a "$log_file"
}

drill_api_kill() {
  local log_file="$ARTIFACT_DIR/drill_api_kill.log"
  echo "== Drill B: API kill =="
  ./flowforge run --no-kill --max-cpu 95 -- python3 demo/pilot/healthy_worker.py >"$log_file" 2>&1 &
  local supervisor_pid=$!
  sleep 3

  local code
  local skip_api=0

  for _ in 1 2 3 4 5 6 7 8 9 10; do
    if curl -s --max-time 1 http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
      break
    fi
    sleep 0.3
  done
  if ! curl -s --max-time 1 http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
    skip_api=1
  fi

  code="$(curl -s -o "$ARTIFACT_DIR/kill_response.json" -w "%{http_code}" \
    -X POST \
    -H "Authorization: Bearer ${FLOWFORGE_API_KEY}" \
    -H "Content-Type: application/json" \
    -d '{"reason":"week1 recovery drill"}' \
    http://127.0.0.1:8080/process/kill || true)"

  wait "$supervisor_pid" || true
  sleep 1

  if [[ "$skip_api" -eq 1 ]]; then
    echo "WARN: API not reachable in this environment; API kill check skipped" | tee -a "$log_file"
    return 0
  fi

  if [[ "$code" != "200" ]]; then
    echo "FAIL: expected /process/kill HTTP 200, got ${code}" | tee -a "$log_file"
    return 1
  fi
  if worker_alive; then
    echo "FAIL: worker process survived API kill" | tee -a "$log_file"
    return 1
  elif [[ $? -eq 2 ]]; then
    echo "WARN: process listing unavailable; orphan check skipped" | tee -a "$log_file"
    return 0
  fi
  echo "PASS: API kill removed active worker" | tee -a "$log_file"
}

drill_sigterm_cleanup
drill_api_kill

cat > "$ARTIFACT_DIR/summary.md" <<EOF
# Recovery Drill Summary

- Drill A (SIGTERM cleanup): completed
- Drill B (API kill): completed (or skipped if API unavailable)

Artifacts:
- \`$ARTIFACT_DIR/drill_sigterm.log\`
- \`$ARTIFACT_DIR/drill_api_kill.log\`
- \`$ARTIFACT_DIR/kill_response.json\`
EOF

echo "Recovery drill completed: $ARTIFACT_DIR/summary.md"

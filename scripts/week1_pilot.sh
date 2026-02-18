#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ARTIFACT_DIR="${1:-pilot_artifacts/week1-$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$ARTIFACT_DIR"

run_case() {
  local name="$1"
  local threshold="$2"
  local script_path="$3"
  local expected="$4"
  local log_file="$ARTIFACT_DIR/${name}.log"
  local result_file="$ARTIFACT_DIR/${name}.result"

  echo "== Case: $name =="
  echo "Threshold: $threshold | Script: $script_path | Expected: $expected"

  set +e
  ./flowforge run --max-cpu "$threshold" -- python3 "$script_path" >"$log_file" 2>&1
  local code=$?
  set -e

  local detected="no"
  if rg -q "LOOP DETECTED" "$log_file"; then
    detected="yes"
  fi

  printf "name=%s\nexit_code=%s\ndetected=%s\nexpected=%s\n" "$name" "$code" "$detected" "$expected" >"$result_file"
  echo "Saved: $log_file"
}

summarize_incidents() {
  local out="$ARTIFACT_DIR/incidents_snapshot.txt"
  FLOWFORGE_DB_PATH="${FLOWFORGE_DB_PATH:-flowforge.db}" python3 - <<'PY' > "$out"
import os
import sqlite3

db = os.getenv("FLOWFORGE_DB_PATH", "flowforge.db")
conn = sqlite3.connect(db)
cur = conn.cursor()
cur.execute(
    """
    SELECT id, timestamp, exit_reason, max_cpu, reason
    FROM incidents
    ORDER BY id DESC
    LIMIT 12
    """
)
rows = cur.fetchall()
print("id | timestamp | exit_reason | max_cpu | reason")
for r in rows:
    print(f"{r[0]} | {r[1]} | {r[2]} | {r[3]:.1f} | {r[4] or ''}")
conn.close()
PY
  echo "Saved: $out"
}

write_summary() {
  local summary="$ARTIFACT_DIR/summary.md"
  {
    echo "# Week 1 Pilot Summary"
    echo
    echo "| Case | Exit Code | Detected | Expected |"
    echo "|---|---:|---|---|"
    for f in "$ARTIFACT_DIR"/*.result; do
      local name code detected expected
      name="$(awk -F= '$1=="name"{print $2}' "$f")"
      code="$(awk -F= '$1=="exit_code"{print $2}' "$f")"
      detected="$(awk -F= '$1=="detected"{print $2}' "$f")"
      expected="$(awk -F= '$1=="expected"{print $2}' "$f")"
      echo "| $name | $code | $detected | $expected |"
    done
    echo
    echo "Artifacts:"
    echo "- Logs: \`$ARTIFACT_DIR/*.log\`"
    echo "- Incident snapshot: \`$ARTIFACT_DIR/incidents_snapshot.txt\`"
  } > "$summary"
  echo "Saved: $summary"
}

run_case "healthy" "90" "demo/pilot/healthy_worker.py" "no intervention"
run_case "bursty" "82" "demo/pilot/bursty_worker.py" "no intervention"
run_case "runaway" "35" "demo/pilot/runaway_worker.py" "loop detection + termination"

summarize_incidents
write_summary

echo "Week 1 pilot completed. Review: $ARTIFACT_DIR/summary.md"

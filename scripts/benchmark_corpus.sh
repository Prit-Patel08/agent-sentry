#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

OUT_DIR="${1:-pilot_artifacts/corpus-$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$OUT_DIR"

run_case() {
  local name="$1"
  shift
  local log_file="$OUT_DIR/${name}.log"
  echo "== Fixture: ${name} =="
  set +e
  python3 "$@" >"$log_file" 2>&1
  local code=$?
  set -e
  echo "${name},exit_code=${code}" | tee -a "$OUT_DIR/results.csv"
}

echo "name,exit_code" > "$OUT_DIR/results.csv"

run_case "infinite_looper" test/fixtures/scripts/infinite_looper.py --timeout 2
run_case "memory_leaker" test/fixtures/scripts/memory_leaker.py --timeout 2
run_case "healthy_spike" test/fixtures/scripts/healthy_spike.py --timeout 20 --spike-seconds 2
run_case "zombie_spawner" test/fixtures/scripts/zombie_spawner.py --timeout 2

cat > "$OUT_DIR/summary.md" <<EOF
# Benchmark Corpus Run

- Output directory: \`$OUT_DIR\`
- Results: \`$OUT_DIR/results.csv\`
- Logs: \`$OUT_DIR/*.log\`

Expected shape:
- \`infinite_looper\`: non-zero (self-timeout)
- \`memory_leaker\`: non-zero (self-timeout)
- \`healthy_spike\`: zero exit (clean completion)
- \`zombie_spawner\`: non-zero (intentional parent crash)
EOF

echo "Corpus benchmark complete: $OUT_DIR/summary.md"

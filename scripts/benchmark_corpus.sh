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

assert_exit_shape() {
  local name="$1"
  local expected="$2"
  local actual
  actual="$(awk -F'[=,]' -v target="$name" '$1==target {print $3}' "$OUT_DIR/results.csv" | tail -n1)"
  if [[ -z "$actual" ]]; then
    echo "Missing result for ${name}" >&2
    return 1
  fi
  if [[ "$expected" == "zero" && "$actual" != "0" ]]; then
    echo "Expected ${name} to exit 0, got ${actual}" >&2
    return 1
  fi
  if [[ "$expected" == "nonzero" && "$actual" == "0" ]]; then
    echo "Expected ${name} to exit non-zero, got 0" >&2
    return 1
  fi
}

echo "name,exit_code" > "$OUT_DIR/results.csv"

run_case "infinite_looper" test/fixtures/scripts/infinite_looper.py --timeout 2
run_case "memory_leaker" test/fixtures/scripts/memory_leaker.py --timeout 2
run_case "healthy_spike" test/fixtures/scripts/healthy_spike.py --timeout 20 --spike-seconds 2
run_case "zombie_spawner" test/fixtures/scripts/zombie_spawner.py --timeout 2

assert_exit_shape "infinite_looper" "nonzero"
assert_exit_shape "memory_leaker" "nonzero"
assert_exit_shape "healthy_spike" "zero"
assert_exit_shape "zombie_spawner" "nonzero"

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

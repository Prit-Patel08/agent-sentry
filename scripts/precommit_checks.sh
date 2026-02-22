#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STRICT_MODE=0

usage() {
  cat <<EOF
Usage: ./scripts/precommit_checks.sh [options]

Options:
  --strict    Fail if optional tools (for example shellcheck) are not installed.
  -h, --help  Show this help text.
EOF
}

for arg in "$@"; do
  case "$arg" in
    --strict) STRICT_MODE=1 ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      usage >&2
      exit 1
      ;;
  esac
done

echo "== FlowForge pre-commit checks =="
echo "Root: $ROOT_DIR"
echo "Strict mode: $STRICT_MODE"

cd "$ROOT_DIR"

echo "[1/6] Tooling doctor"
doctor_args=()
if [[ "$STRICT_MODE" == "1" ]]; then
  doctor_args+=(--strict)
fi
if (( ${#doctor_args[@]} > 0 )); then
  ./scripts/tooling_doctor.sh "${doctor_args[@]}"
else
  ./scripts/tooling_doctor.sh
fi

echo "[2/6] Bash syntax: scripts/*.sh"
for script in scripts/*.sh; do
  bash -n "$script"
done

echo "[3/6] ShellCheck: scripts/*.sh"
if command -v shellcheck >/dev/null 2>&1; then
  shellcheck scripts/*.sh
else
  if [[ "$STRICT_MODE" == "1" ]]; then
    echo "ERROR: shellcheck is required in strict mode but is not installed." >&2
    exit 1
  fi
  echo "WARN: shellcheck not installed; skipping (use --strict to fail instead)."
fi

echo "[4/6] Release checkpoint contract tests"
./scripts/release_checkpoint_contract_test.sh

echo "[5/6] Control-plane replay retention contract tests"
./scripts/controlplane_replay_retention_contract_test.sh

echo "[6/6] SLO weekly review contract tests"
./scripts/slo_weekly_review_contract_test.sh

echo "✅ Pre-commit checks passed"

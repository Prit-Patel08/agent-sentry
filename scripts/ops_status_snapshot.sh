#!/usr/bin/env bash

set -euo pipefail

OUT_DIR="pilot_artifacts/ops-snapshot-$(date +%Y%m%d-%H%M%S)"
STRICT_DOCTOR=0
SKIP_CONTRACTS=0

usage() {
  cat <<EOF
Usage: ./scripts/ops_status_snapshot.sh [options]

Options:
  --out-dir <path>    Output directory for snapshot artifacts.
  --strict-doctor     Run tooling doctor in strict mode.
  --skip-contracts    Skip contract test execution.
  -h, --help          Show this help text.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out-dir)
      OUT_DIR="$2"
      shift 2
      ;;
    --strict-doctor)
      STRICT_DOCTOR=1
      shift
      ;;
    --skip-contracts)
      SKIP_CONTRACTS=1
      shift
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

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mkdir -p "$OUT_DIR"
summary_tsv="$OUT_DIR/summary.tsv"
summary_md="$OUT_DIR/summary.md"

echo -e "check\tstatus\tdetail" > "$summary_tsv"

run_check() {
  local check_name="$1"
  local detail="$2"
  shift 2
  if "$@" >"$OUT_DIR/${check_name}.stdout.log" 2>"$OUT_DIR/${check_name}.stderr.log"; then
    echo -e "${check_name}\tPASS\t${detail}" >> "$summary_tsv"
  else
    echo -e "${check_name}\tFAIL\t${detail}" >> "$summary_tsv"
    return 1
  fi
}

snapshot_status=0

doctor_args=(--summary-file "$OUT_DIR/tooling-summary.tsv")
if [[ "$STRICT_DOCTOR" == "1" ]]; then
  doctor_args=(--strict "${doctor_args[@]}")
fi
if ! run_check "tooling_doctor" "scripts/tooling_doctor.sh" ./scripts/tooling_doctor.sh "${doctor_args[@]}"; then
  snapshot_status=1
fi

if [[ "$SKIP_CONTRACTS" == "0" ]]; then
  if ! run_check "contract_tooling_doctor" "scripts/tooling_doctor_contract_test.sh" ./scripts/tooling_doctor_contract_test.sh; then
    snapshot_status=1
  fi
  if ! run_check "contract_release_checkpoint" "scripts/release_checkpoint_contract_test.sh" ./scripts/release_checkpoint_contract_test.sh; then
    snapshot_status=1
  fi
  if ! run_check "contract_controlplane_replay_retention" "scripts/controlplane_replay_retention_contract_test.sh" ./scripts/controlplane_replay_retention_contract_test.sh; then
    snapshot_status=1
  fi
  if ! run_check "contract_install_hook" "scripts/install_git_hook_contract_test.sh" ./scripts/install_git_hook_contract_test.sh; then
    snapshot_status=1
  fi
else
  echo -e "contracts\tSKIP\tnot requested" >> "$summary_tsv"
fi

git_sha="$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")"
generated_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

{
  echo "# Ops Status Snapshot"
  echo
  echo "Generated: ${generated_at}"
  echo "Commit: ${git_sha}"
  echo
  echo "| Check | Status | Detail |"
  echo "|---|---|---|"
  while IFS=$'\t' read -r check status detail; do
    if [[ "$check" == "check" ]]; then
      continue
    fi
    echo "| ${check} | ${status} | ${detail} |"
  done < "$summary_tsv"
  echo
  echo "Artifacts:"
  echo "- \`${summary_tsv}\`"
  echo "- \`${OUT_DIR}/tooling-summary.tsv\`"
  echo "- \`${OUT_DIR}/*.stdout.log\`"
  echo "- \`${OUT_DIR}/*.stderr.log\`"
} > "$summary_md"

if [[ "$snapshot_status" -ne 0 ]]; then
  echo "❌ Ops status snapshot has failures. See: $summary_md" >&2
  exit 1
fi

echo "✅ Ops status snapshot complete: $summary_md"

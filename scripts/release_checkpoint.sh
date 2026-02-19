#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

REPORT_DIR="${1:-pilot_artifacts/release-$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$REPORT_DIR"
REPORT_FILE="$REPORT_DIR/checkpoint.md"

check() {
  local name="$1"
  shift
  echo "[$name] $*"
  "$@"
}

check "verify-local" ./scripts/verify_local.sh --strict

if git ls-files | rg -q "flowforge\\.key|\\.flowforge_live|flowforge\\.db|sentry\\.key"; then
  echo "Blocked: secret/runtime artifacts still tracked." >&2
  exit 1
fi

if rg -n -i --hidden "quenvor|agent-sentry " -g '!.git/*' -g '!scripts/release_checkpoint.sh' -g '!README.md' | rg -q .; then
  echo "Blocked: legacy brand references found." >&2
  exit 1
fi

required_docs=(
  "docs/RUNBOOK.md"
  "docs/WEEK1_PILOT.md"
  "docs/RELEASE_CHECKLIST.md"
  "docs/ROLLBACK_CHECKLIST.md"
)

missing_docs=()
for doc in "${required_docs[@]}"; do
  if [[ ! -f "$doc" ]]; then
    missing_docs+=("$doc")
  fi
done

if (( ${#missing_docs[@]} > 0 )); then
  echo "Blocked: required operator docs missing:" >&2
  for doc in "${missing_docs[@]}"; do
    echo "  - $doc" >&2
  done
  exit 1
fi

cat > "$REPORT_FILE" <<EOF
# Release Checkpoint

Date: $(date -u +"%Y-%m-%dT%H:%M:%SZ")

## Status

- Local verification gate: PASS
- Tracked secret/runtime legacy artifacts: PASS
- Legacy naming scan: PASS
- Operator docs present (runbook/pilot/release/rollback): PASS

## Ready

Release checkpoint is complete.
EOF

echo "Release checkpoint complete: $REPORT_FILE"

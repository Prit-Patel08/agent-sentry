#!/usr/bin/env bash

set -euo pipefail

STRICT_MODE=0

usage() {
  cat <<EOF
Usage: ./scripts/tooling_doctor.sh [options]

Options:
  --strict    Treat missing optional tools as failures.
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

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "== FlowForge tooling doctor =="
echo "Root: $ROOT_DIR"
echo "Strict mode: $STRICT_MODE"

status=0

resolve_tool() {
  local name="$1"
  if command -v "$name" >/dev/null 2>&1; then
    command -v "$name"
    return 0
  fi

  if command -v go >/dev/null 2>&1; then
    local gobin
    gobin="$(go env GOPATH)/bin/${name}"
    if [[ -x "$gobin" ]]; then
      echo "$gobin"
      return 0
    fi
  fi

  return 1
}

print_ok() {
  local name="$1"
  local version="$2"
  echo "PASS: $name ($version)"
}

print_warn() {
  local name="$1"
  local hint="$2"
  echo "WARN: $name missing. $hint"
}

print_fail() {
  local name="$1"
  local hint="$2"
  echo "FAIL: $name missing. $hint" >&2
  status=1
}

check_tool() {
  local name="$1"
  local requirement="$2" # required or optional
  local version_arg="$3"
  local hint="$4"

  local tool_path
  if tool_path="$(resolve_tool "$name")"; then
    local version
    version="$("$tool_path" "$version_arg" 2>&1 | head -n 1 | tr -d '\r' || true)"
    if [[ -z "$version" ]]; then
      version="$tool_path"
    fi
    print_ok "$name" "$version"
    return 0
  fi

  if [[ "$requirement" == "required" ]]; then
    print_fail "$name" "$hint"
    return 0
  fi

  if [[ "$STRICT_MODE" == "1" ]]; then
    print_fail "$name" "$hint"
  else
    print_warn "$name" "$hint"
  fi
}

check_tool "go" "required" "version" "Install Go toolchain (>=1.25.7 recommended)."
check_tool "node" "required" "--version" "Install Node.js (>=20 recommended)."
check_tool "npm" "required" "--version" "Install npm bundled with Node.js."

check_tool "docker" "optional" "--version" "Install Docker Desktop for container and cloud-dev stack workflows."
check_tool "shellcheck" "optional" "--version" "Install shellcheck to lint scripts locally."
check_tool "staticcheck" "optional" "-version" "Install with: go install honnef.co/go/tools/cmd/staticcheck@latest"
check_tool "govulncheck" "optional" "-version" "Install with: go install golang.org/x/vuln/cmd/govulncheck@latest"

if [[ "$status" -ne 0 ]]; then
  echo "❌ Tooling doctor found blocking issues." >&2
  exit 1
fi

echo "✅ Tooling doctor passed."

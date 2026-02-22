#!/usr/bin/env bash

set -euo pipefail

INSTALL_STATICCHECK=1
INSTALL_GOVULNCHECK=1

STATICCHECK_VERSION="${FLOWFORGE_STATICCHECK_VERSION:-v0.6.1}"
GOVULNCHECK_VERSION="${FLOWFORGE_GOVULNCHECK_VERSION:-v1.1.4}"

usage() {
  cat <<EOF
Usage: ./scripts/install_go_tools.sh [options]

Options:
  --only <tool>  Install only one tool: staticcheck or govulncheck.
  -h, --help     Show this help text.

Environment:
  FLOWFORGE_STATICCHECK_VERSION  Default: v0.6.1
  FLOWFORGE_GOVULNCHECK_VERSION  Default: v1.1.4
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --only)
      if [[ $# -lt 2 ]]; then
        echo "ERROR: --only requires a tool name." >&2
        usage >&2
        exit 1
      fi
      case "$2" in
        staticcheck)
          INSTALL_STATICCHECK=1
          INSTALL_GOVULNCHECK=0
          ;;
        govulncheck)
          INSTALL_STATICCHECK=0
          INSTALL_GOVULNCHECK=1
          ;;
        *)
          echo "ERROR: unsupported tool for --only: $2" >&2
          usage >&2
          exit 1
          ;;
      esac
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

if ! command -v go >/dev/null 2>&1; then
  echo "ERROR: go is required to install tooling binaries." >&2
  exit 1
fi

echo "== FlowForge Go Tool Installer =="
echo "Go: $(go version)"
echo "GOPATH: $(go env GOPATH)"

if [[ "$INSTALL_STATICCHECK" == "1" ]]; then
  echo "Installing staticcheck @ ${STATICCHECK_VERSION}"
  go install "honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION}"
fi

if [[ "$INSTALL_GOVULNCHECK" == "1" ]]; then
  echo "Installing govulncheck @ ${GOVULNCHECK_VERSION}"
  go install "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}"
fi

echo "✅ Go tooling install complete"

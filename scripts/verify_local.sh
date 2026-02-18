#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export GOCACHE="${GOCACHE:-/tmp/agent-gocache}"

echo "== FlowForge local verification =="
echo "Root: $ROOT_DIR"
echo "GOCACHE: $GOCACHE"

cd "$ROOT_DIR"

echo "[1/7] go build ./..."
go build ./...

echo "[2/7] go test ./... -v"
go test ./... -v

echo "[3/7] go test ./... -race -v"
go test ./... -race -v

echo "[4/7] go vet ./..."
go vet ./...

echo "[5/7] staticcheck ./..."
if command -v staticcheck >/dev/null 2>&1; then
  staticcheck ./...
else
  echo "WARN: staticcheck not installed; skipping"
fi

echo "[6/7] govulncheck ./..."
if command -v govulncheck >/dev/null 2>&1; then
  govulncheck ./...
else
  echo "WARN: govulncheck not installed; skipping"
fi

echo "[7/7] dashboard build"
(
  cd dashboard
  npm ci
  npm run build
)

echo "✅ Local verification passed"

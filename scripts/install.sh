#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────
# Agent-Sentry — Universal Install Script
# ─────────────────────────────────────────────

BOLD='\033[1m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

info()    { echo -e "${CYAN}[info]${NC}  $1"; }
success() { echo -e "${GREEN}[  ✓ ]${NC}  $1"; }
warn()    { echo -e "${YELLOW}[warn]${NC}  $1"; }
fail()    { echo -e "${RED}[fail]${NC}  $1"; exit 1; }

echo -e "${BOLD}"
echo "  ╔══════════════════════════════════════╗"
echo "  ║   🛡️  Agent-Sentry — Installer       ║"
echo "  ╚══════════════════════════════════════╝"
echo -e "${NC}"

# ── Detect OS ────────────────────────────────
OS="$(uname -s)"
case "$OS" in
  Darwin) info "Detected macOS" ;;
  Linux)  info "Detected Linux" ;;
  *)      fail "Unsupported OS: $OS. Agent-Sentry supports macOS and Linux." ;;
esac

ARCH="$(uname -m)"
info "Architecture: $ARCH"

# ── Check Go ─────────────────────────────────
if command -v go &>/dev/null; then
  GO_VERSION=$(go version | awk '{print $3}')
  success "Go found: $GO_VERSION"
else
  fail "Go is not installed. Install it from https://go.dev/dl/ and try again."
fi

# ── Check Node/npm ───────────────────────────
if command -v node &>/dev/null; then
  NODE_VERSION=$(node --version)
  success "Node.js found: $NODE_VERSION"
else
  warn "Node.js not found. Dashboard will not be available."
  warn "Install Node.js from https://nodejs.org/ for dashboard support."
fi

if command -v npm &>/dev/null; then
  NPM_VERSION=$(npm --version)
  success "npm found: v$NPM_VERSION"
else
  warn "npm not found. Dashboard dependencies cannot be installed."
fi

# ── Build Go Binary ─────────────────────────
echo ""
info "Building Agent-Sentry binary..."

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

if go build -o sentry .; then
  success "Binary built: ./sentry"
else
  fail "Go build failed. Check the errors above."
fi

# ── Install Dashboard Dependencies ──────────
if [ -d "dashboard" ] && command -v npm &>/dev/null; then
  echo ""
  info "Installing dashboard dependencies..."
  cd dashboard
  if npm install --silent 2>/dev/null; then
    success "Dashboard dependencies installed"
  else
    warn "Dashboard dependency install had issues (non-fatal)"
  fi
  cd "$PROJECT_DIR"
else
  warn "Skipping dashboard setup (missing ./dashboard or npm)"
fi

# ── Create Default Config ───────────────────
if [ ! -f "sentry.yaml" ]; then
  echo ""
  info "Creating default sentry.yaml..."
  cat > sentry.yaml << 'EOF'
# Agent-Sentry Configuration
max-cpu: 90.0
profile: standard

profiles:
  light:
    max-cpu: 95.0
    poll-interval: 1000
    log-window: 5

  standard:
    max-cpu: 90.0
    poll-interval: 500
    log-window: 10

  heavy:
    max-cpu: 80.0
    poll-interval: 250
    log-window: 20
EOF
  success "Default sentry.yaml created"
else
  success "sentry.yaml already exists — skipping"
fi

# ── Done ────────────────────────────────────
echo ""
echo -e "${BOLD}${GREEN}  ✅ Agent-Sentry installed successfully!${NC}"
echo ""
echo "  Quick Start:"
echo "    ./sentry run -- python3 script.py    # Monitor a process"
echo "    ./sentry run --no-kill -- python3 s.py  # Watchdog mode"
echo "    ./sentry dashboard                   # Start the API"
echo "    ./sentry report --id 1               # Generate report"
echo "    ./sentry docs                        # Generate docs"
echo ""
echo "  Set SENTRY_API_KEY for secured endpoints:"
echo "    export SENTRY_API_KEY=your-secret-key"
echo ""

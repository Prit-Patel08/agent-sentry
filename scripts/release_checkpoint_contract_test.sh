#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

tmp_dir="$(mktemp -d)"
stop_mock_ready_server() {
  if [[ -n "${server_pid:-}" ]]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" 2>/dev/null || true
    unset server_pid
  fi
}

cleanup() {
  stop_mock_ready_server
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

copy_repo_script() {
  mkdir -p "$tmp_dir/scripts" "$tmp_dir/docs"
  cp "$ROOT_DIR/scripts/release_checkpoint.sh" "$tmp_dir/scripts/release_checkpoint.sh"
  chmod +x "$tmp_dir/scripts/release_checkpoint.sh"
}

write_stub_verify_local() {
  cat > "$tmp_dir/scripts/verify_local.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "stub verify_local pass"
EOF
  chmod +x "$tmp_dir/scripts/verify_local.sh"
}

write_required_docs() {
  for doc in RUNBOOK.md WEEK1_PILOT.md RELEASE_CHECKLIST.md ROLLBACK_CHECKLIST.md; do
    echo "# ${doc}" > "$tmp_dir/docs/$doc"
  done
  cat > "$tmp_dir/README.md" <<'EOF'
# FlowForge
EOF
}

init_temp_git_repo() {
  (
    cd "$tmp_dir"
    git init -q
    git add .
  )
}

run_case_no_cloud_required() {
  (
    cd "$tmp_dir"
    ./scripts/release_checkpoint.sh "$tmp_dir/out-case-1" >/dev/null
  )
  rg -q "Cloud dependency readiness gate: SKIPPED" "$tmp_dir/out-case-1/checkpoint.md"
}

run_case_cloud_required_unreachable_fails() {
  set +e
  (
    cd "$tmp_dir"
    FLOWFORGE_CLOUD_DEPS_REQUIRED=1 FLOWFORGE_API_BASE=http://127.0.0.1:65531 \
      ./scripts/release_checkpoint.sh "$tmp_dir/out-case-2"
  ) >"$tmp_dir/case2.stdout.log" 2>"$tmp_dir/case2.stderr.log"
  rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    echo "case2 expected failure but passed" >&2
    exit 1
  fi
  rg -q "Blocked: strict cloud readiness is enabled" "$tmp_dir/case2.stderr.log"
}

start_mock_ready_server() {
  local port="$1"
  local body="$2"
  cat > "$tmp_dir/mock_readyz_server.py" <<EOF
from http.server import BaseHTTPRequestHandler, HTTPServer

BODY = b'''${body}'''

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/readyz":
            self.send_response(404)
            self.end_headers()
            return
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(BODY)))
        self.end_headers()
        self.wfile.write(BODY)

    def log_message(self, format, *args):
        return

HTTPServer(("127.0.0.1", ${port}), Handler).serve_forever()
EOF
  python3 "$tmp_dir/mock_readyz_server.py" >/dev/null 2>&1 &
  server_pid=$!
  sleep 1
}

run_case_cloud_required_passes() {
  start_mock_ready_server \
    65530 \
    '{"status":"ready","cloud_dependencies_required":true,"checks":{"database":{"healthy":true}}}'

  (
    cd "$tmp_dir"
    FLOWFORGE_CLOUD_DEPS_REQUIRED=1 FLOWFORGE_API_BASE=http://127.0.0.1:65530 \
      ./scripts/release_checkpoint.sh "$tmp_dir/out-case-3" >/dev/null
  )
  rg -q "Cloud dependency readiness gate: PASS" "$tmp_dir/out-case-3/checkpoint.md"
  rg -q '"status":"ready"' "$tmp_dir/out-case-3/readyz.json"

  stop_mock_ready_server
}

run_case_cloud_required_mismatch_fails() {
  start_mock_ready_server \
    65529 \
    '{"status":"ready","cloud_dependencies_required":false,"checks":{"database":{"healthy":true}}}'

  set +e
  (
    cd "$tmp_dir"
    FLOWFORGE_CLOUD_DEPS_REQUIRED=1 FLOWFORGE_API_BASE=http://127.0.0.1:65529 \
      ./scripts/release_checkpoint.sh "$tmp_dir/out-case-4"
  ) >"$tmp_dir/case4.stdout.log" 2>"$tmp_dir/case4.stderr.log"
  rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    echo "case4 expected failure but passed" >&2
    exit 1
  fi
  rg -q "cloud_dependencies_required=true" "$tmp_dir/case4.stderr.log"

  stop_mock_ready_server
}

copy_repo_script
write_stub_verify_local
write_required_docs
init_temp_git_repo

run_case_no_cloud_required
run_case_cloud_required_unreachable_fails
run_case_cloud_required_passes
run_case_cloud_required_mismatch_fails

echo "release checkpoint contract tests passed"

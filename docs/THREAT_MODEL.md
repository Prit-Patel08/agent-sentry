# Agent-Sentry Threat Model

## Scope

Agent-Sentry supervises local subprocesses, monitors behavior, and exposes a local HTTP API for dashboards/control.

## Assets

- API credentials (`SENTRY_API_KEY`, `SENTRY_MASTER_KEY`)
- Incident database (`sentry.db`)
- Runtime process metadata and output stream
- Process control actions (kill/restart)

## Trust Boundaries

1. CLI runtime to supervised subprocess.
2. API server (`127.0.0.1`) to local client/browser.
3. Config and environment variable inputs to supervisor policy.
4. Database encryption boundary for persisted sensitive fields.

## Attack Surfaces

- API endpoints (`/process/*`, `/incidents`, `/stream`)
- Subprocess stdout/stderr data flow
- Config file values (`sentry.yaml`)
- Container/runtime deployment options

## Mitigations Implemented

- Local-only API binding (`127.0.0.1` / `localhost`)
- Constant-time API key comparison
- Auth failure throttling + request rate limiting
- No shell-based command execution for restarts/monitoring
- Redaction of common secrets before state/dashboard exposure
- Config validation for CPU/polling/window/memory/token thresholds
- Graceful shutdown flow for child process groups
- Security CI checks (`staticcheck`, `govulncheck`) + SBOM generation

## Residual Risks

- Users can still expose API if reverse-proxying localhost endpoints insecurely.
- Command output may include unknown secret formats not covered by static redaction patterns.
- Crash-level failures can terminate supervisor before a final cleanup signal is delivered.

## Operational Requirements

- Rotate any previously committed credentials immediately.
- Enforce strong per-environment API and master keys.
- Run container with `--read-only`, `--cap-drop=ALL`, and `--security-opt=no-new-privileges`.

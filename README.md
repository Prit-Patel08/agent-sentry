# Agent-Sentry

Autonomous supervision and security layer for AI agent subprocesses.

## Security and Reliability Highlights

- Zero-shell execution path (`exec.Command` with structured args)
- Local-only API binding (`127.0.0.1` / `localhost`)
- Constant-time API key checks
- API request throttling + auth brute-force protection
- In-memory runtime state guarded by `sync.RWMutex`
- Secret redaction before dashboard/state exposure
- Graceful process-group shutdown with forced fallback

## API Endpoints

- `GET /incidents`
- `GET /stream`
- `POST /process/kill`
- `POST /process/restart`
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

## Configuration

- Config file: `sentry.yaml` (or `--config`)
- Environment:
  - `SENTRY_API_KEY` for mutating API endpoints
  - `SENTRY_MASTER_KEY` for DB field encryption (64 hex chars)
  - `SENTRY_ALLOWED_ORIGIN` for CORS allow-list (default `http://localhost:3000`)
  - `SENTRY_BIND_HOST` (`127.0.0.1`/`localhost` only)
  - `NEXT_PUBLIC_SENTRY_API_BASE` for dashboard base URL

## Development

```bash
go build ./...
go test ./... -v
go test ./... -race -v
go test ./test -bench . -benchmem -run '^$'
```

Dashboard:

```bash
cd dashboard
npm ci
npm run build
```

## Security Documentation

- Threat model: `docs/THREAT_MODEL.md`
- Operations guide: `docs/OPERATIONS.md`
- Security policy: `SECURITY.md`

## Architecture Blueprint: FlowForge-Class Local-First + Cloud-Capable Supervisor Platform

This design assumes a serious target: enterprise-grade reliability, strong local autonomy, and a cloud control plane that can scale from single-team usage to multi-tenant infrastructure business.

---

## SECTION 1 — Product Requirements

### 1.1 Functional Requirements
1. Launch long-running jobs from API/UI/CLI with explicit command, args, env policy, resource policy, and execution target.
2. Monitor process health continuously (CPU, memory, I/O, exit code, heartbeat, log velocity, repetition score, entropy score).
3. Detect task pathologies:
   - Repetitive output loops
   - Stalled/no-progress jobs
   - Failing retry storms
   - Resource runaway
4. Stream logs in near-real-time to dashboard and API consumers.
5. Provide manual and automated interventions:
   - Pause (if runtime supports)
   - Graceful stop
   - Forced kill
   - Restart with bounded retries
6. Expose stable REST APIs for orchestration, status, logs, incidents, and policies.
7. Support local-first mode:
   - Full monitoring and interventions with no cloud dependency
   - Local UI/API reachable on loopback
8. Support cloud mode:
   - Multi-agent fleet management
   - Multi-tenant RBAC and policy governance
   - Centralized logs/metrics/audit
9. Persist evidence trail for all decisions/interventions (auditable timeline and incident chain).
10. Support deterministic replay/debug of “why action happened.”

### 1.2 Non-Functional Requirements
1. Latency:
   - Job launch API p95 < 250 ms (control-plane acceptance).
   - Kill command to signal dispatch p95 < 150 ms.
   - Log streaming fanout delay p95 < 1.5 s (agent to dashboard).
2. Reliability:
   - Control-plane API availability target: 99.95%.
   - Agent local supervision availability target: 99.99% (independent of cloud).
   - No silent drops for intervention events.
3. Durability:
   - Audit events: no data loss under single-node failure (cloud).
   - Local mode: crash-safe WAL for incident/action state.
4. Scalability:
   - Initial design point: 10k concurrently running tasks cloud-wide.
   - Burst job creation: 1k launches/min sustained for 10 minutes.
5. Security:
   - Default local bind to `127.0.0.1`.
   - Zero-trust cloud communication (mTLS + short-lived credentials).
   - Fine-grained tenant isolation.
6. Operability:
   - Full SLO instrumentation.
   - Safe rollouts with canary.
   - Backward-compatible API versioning.

### 1.3 Expected Load Assumptions
| Dimension | MVP | Growth | Target |
|---|---:|---:|---:|
| Registered users | 500 | 5,000 | 50,000 |
| Monthly active users | 200 | 2,000 | 10,000 |
| Concurrent tasks | 100 | 1,000 | 10,000 |
| Active agents | 50 | 500 | 3,000 |
| Log ingest rate | 2 MB/s | 20 MB/s | 120 MB/s |
| API RPS steady | 50 | 500 | 3,000 |
| API RPS burst | 200 | 2,000 | 10,000 |

Assumption for cost/scaling model:
- Average task footprint: 0.25 vCPU / 512 MiB.
- Average log rate: 0.5 KB/s per running task.
- 20% of tasks produce incidents requiring timeline retention beyond default.

### 1.4 Failure Scenarios to Design For
1. Agent disconnect from cloud while tasks running.
2. Process tree refuses graceful termination.
3. Log burst causes backpressure and memory pressure.
4. DB primary failover during active incidents.
5. Queue outage causing delayed event fanout.
6. Misconfigured detection policy causing false-positive kill storm.
7. Regional outage in cloud mode.
8. Secret leakage attempt via logs or env dump.
9. Noisy tenant starving shared control-plane resources.
10. Dashboard websocket/SSE storm during incident spikes.

---

## SECTION 2 — High-Level Architecture

### 2.1 Component Breakdown
1. **Edge Agent (local daemon)**  
   Runs on user machine/VM. Starts processes, monitors runtime, captures logs, executes kill/restart, persists local WAL state.
2. **Local State Store**  
   SQLite (WAL mode) for durable local events, task state, and replay.
3. **Cloud API Gateway**  
   AuthN/AuthZ enforcement, rate limits, request routing, request tracing.
4. **Control Plane API Service**  
   Job CRUD, policy management, incidents, task state aggregation.
5. **Scheduler/Dispatcher**  
   Assigns job intents to target agent/worker pools.
6. **Supervisor Workers (cloud runtime)**  
   For cloud-executed tasks: containerized supervisor pods with same core supervisor logic.
7. **Event Bus (NATS JetStream)**  
   Decouples state transitions, logs metadata, incident events, notifications.
8. **Metadata Database (PostgreSQL)**  
   Tenant, jobs, runs, incidents, audit references.
9. **Log Pipeline**  
   Stream ingest service + object storage for raw logs + indexed pointers.
10. **Metrics/Tracing Stack**  
    Prometheus/Mimir, OpenTelemetry collector, tracing backend.
11. **Web Dashboard (Next.js)**  
    Multi-tenant UI, live status/log stream, controls, incident drilldown.
12. **Policy Engine Service**  
    Shared deterministic scoring logic for detection and intervention decisions.

### 2.2 ASCII Architecture Diagram

```text
                          +---------------------------+
                          |        Web Dashboard      |
                          |  (Next.js + SSE client)   |
                          +-------------+-------------+
                                        |
                                  HTTPS / REST
                                        |
                          +-------------v-------------+
                          |      API Gateway/WAF      |
                          | AuthN, AuthZ, Rate Limit  |
                          +------+------+-------------+
                                 |      |
                     +-----------+      +------------------+
                     |                                  |
         +-----------v------------+         +-----------v------------+
         |   Control Plane API    |         |   Log Stream Service   |
         | Jobs, Runs, Incidents  |         | SSE fanout, cursors    |
         +-----+-----------+------+         +------------+------------+
               |           |                               |
               |           |                               |
     +---------v--+   +----v----------------+      +------v-------+
     | PostgreSQL |   | NATS JetStream Bus  |      | Object Store |
     | metadata   |   | events + commands   |      | (S3/minio)   |
     +-----+------+   +----+----------------+      +------+-------+
           |               |                              |
           |               |                              |
   +-------v------+  +-----v---------------------+  +-----v-----------------+
   | Scheduler    |  | Cloud Supervisor Workers  |  | Log Index (Postgres / |
   | / Dispatcher |  | (K8s pods, containerd)    |  | optional ClickHouse)  |
   +-------+------+  +---------------------------+  +-----------------------+
           |
           | mTLS command/control channel
           |
+----------v---------------------------------------------------------------+
|                        Edge Agent Fleet                                   |
| +----------------------+   +----------------------+   +----------------+ |
| | Local Supervisor     |   | Detection Engine     |   | SQLite WAL     | |
| | process groups       |   | repetition/stall/etc |   | local evidence | |
| +----------+-----------+   +----------+-----------+   +--------+-------+ |
|            |                          |                        |         |
|         child processes           log/timing features      sync spool    |
+-------------------------------------------------------------------------+
```

### 2.3 Data Flows

#### 1) Job Creation
1. User submits `POST /v1/jobs`.
2. API gateway validates token, tenant, quotas.
3. Control plane writes job intent to Postgres.
4. Dispatcher emits `job.created` event to NATS.
5. Target selection:
   - Local mode: assigned to local agent immediately.
   - Cloud mode: assigned to worker pool.
6. Job run record created (`QUEUED`), id returned to caller.

#### 2) Job Execution
1. Agent/worker receives assignment from command topic.
2. Supervisor creates process group/container with resource constraints.
3. Run status transitions: `QUEUED -> STARTING -> RUNNING`.
4. Events and timing markers persisted locally and synced cloud-side.
5. Health telemetry published at interval (`run.health`).

#### 3) Log Streaming
1. Supervisor tails stdout/stderr pipes with line framing.
2. Logs buffered in memory ring + local spool (backpressure safety).
3. Log chunks sent to cloud ingest with sequence numbers and run offsets.
4. Ingest writes raw chunk to object storage and metadata pointer to DB.
5. Dashboard subscribes via SSE endpoint with cursor; receives near-real-time chunks.

#### 4) Auto-Detection + Kill Logic
1. Detection engine computes rolling features:
   - repetition ratio
   - entropy drop
   - heartbeat stagnation
   - error recurrence
   - CPU/memory pressure
2. Rule engine computes confidence score.
3. Policy check gates action by mode:
   - `shadow` logs only
   - `canary` sampled enforcement
   - `enforce` full action
4. If threshold crossed, action pipeline executes:
   - emit `incident.detected`
   - issue graceful stop
   - escalate kill if needed
   - optionally restart (policy)
5. Evidence event appended with exact signals used.

#### 5) Failure Recovery
1. Agent crash/restart:
   - replay local WAL
   - reconcile orphaned process groups
   - publish reconciliation events
2. Cloud disconnect:
   - continue local supervision autonomously
   - spool deltas locally
   - replay on reconnect
3. DB failover:
   - API uses retry with idempotency keys
   - command/event consumers resume from durable offsets
4. Queue delay:
   - direct local safeguards still execute kill/restart without cloud dependency.

---

## SECTION 3 — Technology Stack Decisions

| Layer | Choice | Why This Choice | Rejected Alternatives | Tradeoffs |
|---|---|---|---|---|
| Core backend | **Go** | Strong concurrency, predictable latency, single static binaries for local agent, low memory overhead. | Node (GC pauses under heavy stream fanout), Python (perf/threading limits), Rust (excellent but slower hiring/onboarding). | Go gives fast iteration and operability; Rust may win on memory safety/perf edge cases. |
| Supervisor runtime control | **Go + OS primitives** | Direct process/signal/cgroup control with minimal dependencies. | Python subprocess-based supervisor. | Slightly more low-level code complexity. |
| Frontend | **Next.js (React + TS)** | Fast enterprise UI iteration, SSR/CSR flexibility, mature ecosystem. | Vue/Nuxt, SvelteKit. | Next build/tooling complexity; manageable with strict CI. |
| Primary DB | **PostgreSQL** | Strong transactional guarantees, JSONB flexibility, mature partitioning, robust tooling/PITR. | MySQL (less ergonomic JSON/event queries), NoSQL-only (weak relational integrity for control plane). | Requires careful indexing/partition strategy at scale. |
| Metrics TSDB | **Prometheus + Mimir/Thanos** | Standard for SLO math and alerting, strong ecosystem. | InfluxDB-only, Datadog-only. | Prometheus cardinality discipline required. |
| Cache/session | **Redis** | Fast distributed locks, rate-limits, ephemeral state, queue buffers. | No cache, Memcached. | Redis introduces operational dependency; worth it at scale. |
| Event bus | **NATS JetStream** | Low-latency control/events, simpler ops than Kafka at this scale, at-least-once durability. | Kafka (heavier ops), RabbitMQ (less ideal for replay-heavy event sourcing). | Kafka may be needed later for very high analytic throughput. |
| Raw log storage | **S3-compatible object store** | Cheap, durable, lifecycle policies, decouples compute from storage. | Postgres blob storage, EBS-only files. | Requires index indirection for fast querying. |
| Log indexing/query | **Postgres initially, optional ClickHouse later** | Start simple with Postgres pointers; move to ClickHouse when query volume grows. | Elasticsearch-only from day one. | Two-step architecture avoids early overbuild. |
| Container runtime | **containerd (via K8s)** | Stable, standard, lower overhead than Docker daemon in cluster. | Docker runtime directly. | Local dev still benefits from Docker CLI wrappers. |
| Orchestration | **Kubernetes** | Multi-tenant scheduling, autoscaling, mature policy controls, broad ecosystem. | ECS (lock-in), Nomad (good but smaller ecosystem). | K8s complexity; mitigated via Helm, ArgoCD, platform templates. |
| CI/CD | **GitHub Actions + ArgoCD** | Strong Git workflow integration, declarative deployment, auditable promotion path. | Jenkins, GitLab-only. | Need disciplined workflow and environment gating. |

---

## SECTION 4 — Database Design

### 4.1 Data Model Principles
1. Metadata in Postgres; high-volume logs in object storage.
2. Immutable event ledger for forensic traceability.
3. Tenant-first keys on all shared tables.
4. Partition large append-only tables by time.

### 4.2 Core Schema (DDL-style)

```sql
CREATE TABLE tenants (
  id UUID PRIMARY KEY,
  name TEXT NOT NULL,
  plan TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
  id UUID PRIMARY KEY,
  email CITEXT UNIQUE NOT NULL,
  display_name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id)
);

CREATE TABLE projects (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  environment TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE agents (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  hostname TEXT NOT NULL,
  version TEXT NOT NULL,
  mode TEXT NOT NULL,
  last_seen_at TIMESTAMPTZ,
  status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE jobs (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  created_by UUID NOT NULL REFERENCES users(id),
  name TEXT NOT NULL,
  command TEXT NOT NULL,
  args JSONB NOT NULL DEFAULT '[]',
  env_policy JSONB NOT NULL DEFAULT '{}',
  resource_policy JSONB NOT NULL DEFAULT '{}',
  schedule_policy JSONB NOT NULL DEFAULT '{}',
  detection_policy_version TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE job_runs (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  agent_id UUID REFERENCES agents(id),
  status TEXT NOT NULL,
  exit_code INT,
  started_at TIMESTAMPTZ,
  ended_at TIMESTAMPTZ,
  restart_count INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE incidents (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  run_id UUID NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
  severity TEXT NOT NULL,
  category TEXT NOT NULL,
  confidence NUMERIC(5,2) NOT NULL,
  summary TEXT NOT NULL,
  reason TEXT NOT NULL,
  opened_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  closed_at TIMESTAMPTZ
);

CREATE TABLE run_events (
  id BIGSERIAL PRIMARY KEY,
  tenant_id UUID NOT NULL,
  project_id UUID NOT NULL,
  run_id UUID NOT NULL,
  incident_id UUID,
  event_type TEXT NOT NULL,
  actor TEXT NOT NULL,
  title TEXT NOT NULL,
  reason_text TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
) PARTITION BY RANGE (created_at);

CREATE TABLE log_segments (
  id BIGSERIAL PRIMARY KEY,
  tenant_id UUID NOT NULL,
  project_id UUID NOT NULL,
  run_id UUID NOT NULL,
  segment_seq BIGINT NOT NULL,
  storage_uri TEXT NOT NULL,
  byte_offset BIGINT NOT NULL,
  byte_length BIGINT NOT NULL,
  line_count INT NOT NULL,
  checksum TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (run_id, segment_seq)
);

CREATE TABLE audit_events (
  id BIGSERIAL PRIMARY KEY,
  tenant_id UUID NOT NULL,
  project_id UUID NOT NULL,
  actor_id UUID,
  actor_type TEXT NOT NULL,
  action TEXT NOT NULL,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  request_id TEXT,
  trace_id TEXT,
  metadata JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_tokens (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  scopes TEXT[] NOT NULL,
  expires_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 4.3 Index Strategy
1. `job_runs(tenant_id, status, created_at DESC)` for active views.
2. `incidents(tenant_id, opened_at DESC)` and `incidents(run_id)`.
3. `run_events(tenant_id, run_id, created_at ASC)` for timeline replay.
4. `run_events(tenant_id, event_type, created_at DESC)` for operational analytics.
5. `agents(tenant_id, status, last_seen_at DESC)` for fleet page.
6. `log_segments(run_id, segment_seq)` unique and clustered.
7. Partial indexes on active statuses and open incidents.

### 4.4 Scaling Strategy
1. Partition `run_events` by month initially, by week at high volume.
2. Read replicas for dashboard-heavy reads.
3. Tenant-aware sharding trigger:
   - shard when single cluster exceeds 8 TB storage or sustained >20k writes/sec.
4. Keep hot metadata in Postgres; move heavy event analytics to ClickHouse stream if needed.

### 4.5 Retention Policies
1. `run_events` hot retention: 90 days.
2. `run_events` cold archive snapshots: 13 months.
3. Raw log segments: hot 7 days, infrequent 90 days, archive per compliance policy.
4. Audit events: minimum 1 year, configurable to 7 years.

### 4.6 Backup Strategy
1. Postgres PITR with continuous WAL archiving.
2. Daily full snapshots + 35-day WAL retention.
3. Cross-region backup replication for DR.
4. Quarterly restore drills with RTO/RPO verification.
5. Object storage versioning + lifecycle lock for audit buckets.

### 4.7 Migration Strategy
1. Expand-contract migrations only.
2. Backward-compatible API and schema during rollout window.
3. Feature-flagged reads for new columns.
4. Data backfills as idempotent jobs.
5. Rollback policy: disable writer path first, then revert readers.

---

## SECTION 5 — API Design

### 5.1 Core REST Endpoints
| Method | Path | Purpose |
|---|---|---|
| POST | `/v1/jobs` | Create job definition |
| GET | `/v1/jobs/{job_id}` | Read job |
| POST | `/v1/jobs/{job_id}/runs` | Start run |
| GET | `/v1/runs/{run_id}` | Run status |
| POST | `/v1/runs/{run_id}/kill` | Kill run |
| POST | `/v1/runs/{run_id}/restart` | Restart run |
| GET | `/v1/runs/{run_id}/events` | Timeline events |
| GET | `/v1/runs/{run_id}/logs/stream` | SSE log stream |
| GET | `/v1/incidents` | Incident list |
| GET | `/v1/incidents/{incident_id}` | Incident detail |
| GET | `/v1/agents` | Agent fleet status |
| GET | `/v1/metrics` | Prometheus metrics |
| GET | `/v1/healthz` | Liveness |
| GET | `/v1/readyz` | Readiness |

### 5.2 Request/Response Example

#### Create Job
```http
POST /v1/jobs
Authorization: Bearer <jwt>
Content-Type: application/json
Idempotency-Key: 39c0f3b7-...
```

```json
{
  "project_id": "f83f...",
  "name": "nightly-embedding-refresh",
  "command": "/usr/local/bin/python",
  "args": ["worker.py", "--mode", "full"],
  "resource_policy": {
    "cpu_limit_millicores": 1000,
    "memory_limit_mb": 1024,
    "max_runtime_seconds": 14400
  },
  "detection_policy_version": "v3.2.1"
}
```

```json
{
  "job_id": "3c47...",
  "created_at": "2026-02-21T12:01:05Z",
  "status": "CREATED"
}
```

#### Kill Run
```http
POST /v1/runs/9baf.../kill
Authorization: Bearer <jwt>
Content-Type: application/json
```

```json
{
  "reason": "manual intervention by on-call"
}
```

```json
{
  "status": "stop_requested",
  "run_id": "9baf...",
  "lifecycle": "STOPPING",
  "requested_at": "2026-02-21T12:10:22Z"
}
```

### 5.3 Authentication Model
1. User auth: OIDC/OAuth2 to obtain JWT access token.
2. Service-to-service: mTLS + workload identity.
3. Agent auth with rotating short-lived certs.
4. Local-only mode supports API key for mutating endpoints.

### 5.4 Rate Limiting Strategy
1. Global per-IP guard at gateway.
2. Per-tenant quotas for create/mutation/stream concurrency.
3. Redis token bucket.
4. Adaptive backoff under incident storms.

### 5.5 Versioning Strategy
1. URL major version (`/v1`).
2. Additive response evolution only in minor releases.
3. Deprecation headers and migration docs.
4. Minimum 2 major versions overlap in enterprise.

### 5.6 SSE vs WebSockets
Decision: **SSE for logs/events by default**, optional WebSocket for interactive shells.

### 5.7 Error Handling Format
Use RFC 7807 Problem Details.

```json
{
  "type": "https://api.flowforge.io/errors/conflict",
  "title": "Run state conflict",
  "status": 409,
  "detail": "Run is already stopping; restart not allowed.",
  "instance": "/v1/runs/9baf.../restart",
  "request_id": "req_01HT..."
}
```

---

## SECTION 6 — Supervisor Design

### 6.1 Safe Process Launch
1. No implicit shell execution by default.
2. Executable path validation + allow/deny policy.
3. Sanitized environment inheritance.
4. Runtime limits applied before start:
   - cgroup cpu/memory/pid limits
   - open file descriptor caps
   - runtime timeout.
5. Process gets unique run identity and metadata tags.
6. Startup event emitted before first user code runs.

### 6.2 Process Groups and Signals
1. Launch each run in its own process group/session.
2. Graceful stop:
   - send `SIGTERM` to process group
   - wait configurable grace window
3. Force stop:
   - send `SIGKILL` to process group
4. Verify stop with `waitpid` + existence check.
5. Windows parity uses Job Objects and termination API.

### 6.3 Isolation Strategy
1. Local trusted mode: raw process for lowest overhead.
2. Local semi-trusted mode: rootless container sandbox.
3. Cloud untrusted mode: container sandbox with seccomp/AppArmor.
4. High-risk mode: microVM (Firecracker/gVisor profile) for tenant isolation.
5. Isolation mode is policy-driven per project.

### 6.4 Log Capture and Streaming Pipeline
1. Capture stdout/stderr pipes asynchronously.
2. Line framing with truncation protection for giant lines.
3. Sequence IDs per line/chunk to preserve order and replay.
4. Ring buffer for live tail + spill-to-disk spool for outages.
5. Cloud ingest with retry and dedup by `(run_id, seq)`.
6. Redaction pipeline before egress (secrets, tokens, keys).

### 6.5 Heuristic Detection Logic
1. Input features:
   - repetition ratio on sliding window
   - token entropy trend
   - heartbeat/progress checkpoints
   - consecutive error signature count
   - CPU/memory slope.
2. Scoring:
   - weighted confidence 0–100
   - multi-signal gating to avoid single-noise triggers.
3. Policies:
   - shadow mode (log only)
   - canary mode (sampled enforce)
   - enforce mode.
4. Decision emits full evidence payload with feature vectors.

### 6.6 Safe Kill Strategy
1. Transition run to `STOPPING` immediately.
2. Emit audit + timeline event with actor and reason.
3. Attempt graceful terminate.
4. Escalate to force kill if not exited.
5. Emit terminal lifecycle state with duration metrics.
6. If kill fails, mark `FAILED_STOP` and raise incident.

### 6.7 False Positive Mitigation
1. Warm-up period after start.
2. Suppression for known benign repetitive patterns.
3. Require confirmation window (N consecutive abnormal windows).
4. Per-job guardrails and user-tunable sensitivity.
5. Replay tool for policy tuning against historical logs before rollout.

---

## SECTION 7 — Security Architecture

### 7.1 Secret Storage
1. Local: encrypted at rest with OS keychain-backed key.
2. Cloud: KMS envelope encryption + Vault for dynamic credentials.
3. Never log plaintext secrets; redaction before persistence and streaming.
4. Token hashes only in DB, never reversible secrets.

### 7.2 Network Exposure Strategy
1. Local API defaults to loopback only.
2. Cloud APIs behind WAF and private service mesh.
3. Agent outbound-only mTLS tunnel to cloud.
4. Strict egress controls for worker namespaces.

### 7.3 TLS Termination
1. Public edge TLS at gateway.
2. Internal service-to-service mTLS with workload identities.
3. Agent cert rotation on short intervals.

### 7.4 RBAC Model
1. Roles: Owner, Admin, Operator, Viewer, Service.
2. Resource scopes: tenant, project, job, run.
3. Mutations require Operator+.
4. Sensitive operations require justifications and audit context.

### 7.5 Audit Logging
1. Immutable append-only audit stream.
2. Include actor, target, reason, request_id, trace_id, source IP.
3. Tamper-evidence via hash chaining or signed batches.
4. Exportable for SIEM integration.

### 7.6 Sandbox Strategy for Untrusted Code
1. Container isolation baseline.
2. Seccomp/AppArmor profiles deny privileged syscalls.
3. Read-only root filesystem + ephemeral writable volume.
4. Optional microVM runtime for high-risk workloads.
5. Network policies default deny egress except allowlisted endpoints.

### 7.7 Threat Model (High-Level)
| Threat | Impact | Mitigation |
|---|---|---|
| Command injection | Host compromise | No shell-by-default, strict argument parsing, policy controls |
| Secret exfil via logs | Credential leakage | Real-time redaction, denylist detectors, DLP alerts |
| Agent impersonation | Unauthorized control | mTLS cert issuance + rotation + revocation |
| Tenant data leakage | Compliance breach | Tenant-scoped authZ checks on every query |
| Kill endpoint abuse | Service disruption | RBAC + per-endpoint rate limits + mandatory audit |
| Queue replay abuse | State corruption | idempotency keys + signed event metadata |
| Supply-chain compromise | broad risk | signed artifacts, SBOM, image scanning, provenance attestation |

---

## SECTION 8 — Observability

### 8.1 Metrics (Prometheus)
Key metrics:
1. `flowforge_runs_active{tenant,project}`
2. `flowforge_run_launch_latency_seconds_bucket`
3. `flowforge_detection_confidence`
4. `flowforge_incidents_total{category,severity}`
5. `flowforge_interventions_total{action,source}`
6. `flowforge_stop_latency_last_seconds`
7. `flowforge_restart_latency_last_seconds`
8. `flowforge_stop_slo_compliance_ratio`
9. `flowforge_restart_slo_compliance_ratio`
10. `flowforge_log_ingest_lag_seconds`
11. `flowforge_agent_heartbeat_age_seconds`
12. `flowforge_api_requests_total{route,status}`

### 8.2 Structured Logging
JSON log fields:
- `timestamp`
- `level`
- `service`
- `tenant_id`
- `project_id`
- `run_id`
- `incident_id`
- `event_type`
- `message`
- `request_id`
- `trace_id`
- `actor`
- `error_code`

### 8.3 Tracing (OpenTelemetry)
1. Trace spans across gateway -> API -> dispatcher -> agent command -> supervisor.
2. Include run_id and tenant_id baggage attributes.
3. Sampling:
   - baseline 5%
   - 100% for errors/incidents/mutations.

### 8.4 Alerting Strategy
1. SLO burn alerts for stop/restart compliance, API availability, log stream delay.
2. Capacity alerts for queue depth, DB CPU/IO, ingestion lag.
3. Security alerts for auth failure spikes and anomalous mutation bursts.

### 8.5 Dashboard Examples
1. Executive reliability dashboard.
2. Operations dashboard.
3. Tenant health dashboard.
4. Security dashboard.

---

## SECTION 9 — Scalability & Fault Tolerance

### 9.1 Horizontal Scaling
1. Gateway/API services stateless; scale via HPA on CPU + RPS + latency.
2. Dispatcher scales by queue lag and assignment throughput.
3. Log stream service scales by active SSE connection count.

### 9.2 SPOF Elimination
1. Multi-AZ Postgres with automatic failover.
2. NATS JetStream cluster (3+ nodes).
3. Redis HA.
4. Ingress redundancy across zones.
5. Object store with cross-AZ durability.

### 9.3 Database Scaling
1. Read replicas for dashboard queries.
2. Time partitioning for event tables.
3. Archive heavy historical query workloads to analytics store.
4. Tenant sharding when crossing write/storage thresholds.

### 9.4 Worker Scaling
1. K8s autoscaling by pending runs and CPU/memory utilization.
2. Separate pools for default/high-risk/burst workloads.
3. Pod disruption budgets to preserve active supervisors.

### 9.5 Load Balancing
1. L7 ingress for API/UI.
2. Sticky-free stateless control plane.
3. SSE-aware timeout tuning in ingress and proxies.

### 9.6 Graceful Degradation
1. Cloud outage: agents continue local supervision and enforce policies.
2. Object storage slowdown: local spool + compressed retries.
3. DB degraded: mutation circuit breakers + read-only status mode.
4. Queue backlog: prioritize control actions over analytics events.

---

## SECTION 10 — DevOps & Deployment

### 10.1 Docker Strategy
1. Multi-stage builds for Go binaries.
2. Distroless runtime images.
3. Separate images for API, dispatcher, ingest, agent.
4. SBOM generation + signed images in registry.

### 10.2 Kubernetes Manifests Overview
1. Deployments: `api`, `dispatcher`, `log-ingest`, `stream-gateway`.
2. StatefulSets: `postgres` (if self-managed), `nats`, `redis`.
3. Services, ingress, HPAs, PDBs, NetworkPolicies.
4. PodSecurity restricted profile.

### 10.3 Helm Chart Structure
```text
charts/flowforge/
  Chart.yaml
  values.yaml
  templates/
    api-deployment.yaml
    dispatcher-deployment.yaml
    stream-deployment.yaml
    service.yaml
    ingress.yaml
    hpa.yaml
    pdb.yaml
    networkpolicy.yaml
    secret.yaml
```

### 10.4 GitHub Actions Pipeline
1. Lint + static analysis.
2. Unit + race tests.
3. Integration tests with ephemeral Postgres/NATS.
4. Security scans + image scan.
5. Build/sign/push images.
6. Deploy staging via ArgoCD.
7. Synthetic smoke tests.
8. Manual prod approval gate.
9. Progressive rollout + automated rollback.

### 10.5 Deployment Strategy
Decision: **Progressive canary rollout**.

### 10.6 Staging vs Production Separation
1. Isolated clusters and credentials.
2. Separate KMS keys and secrets.
3. Staging mirrors prod topology at reduced scale.
4. Promotion by artifact digest only.

---

## SECTION 11 — Cost Estimation

### 11.1 Assumptions
1. Managed Kubernetes + managed Postgres + managed object store.
2. Average task footprint 0.25 vCPU / 512 MiB.
3. 24x7 concurrency.
4. Log retention: 30-day hot + 90-day warm.
5. Observability includes metrics + tracing + searchable logs.

### 11.2 Monthly Cost Estimates (Order-of-Magnitude)
| Concurrent Tasks | Compute (workers + control) | Data (DB + cache + queue) | Observability | Storage/Egress | Total / Month |
|---:|---:|---:|---:|---:|---:|
| 100 | $1.5k | $0.7k | $0.4k | $0.2k | **$2.8k** |
| 1,000 | $14k | $3.2k | $4.5k | $2k | **$23.7k** |
| 10,000 | $120k | $23k | $35k | $18k | **$196k** |

### 11.3 Cost Optimization Tradeoffs
1. Spot instances for non-critical worker pools.
2. Tiered log retention.
3. Move heavy analytics off primary DB.
4. Cardinality control in metrics.
5. Per-tenant quotas and burst pricing.
6. Reserved baseline + on-demand burst strategy.

---

## SECTION 12 — Engineering Roadmap

### Phase 1 (MVP, 0–3 months): Local-First Reliability Core
1. Edge agent + local supervisor + local SQLite WAL.
2. Core APIs: create run, status, kill, restart, timeline.
3. Basic detection engine with shadow/enforce.
4. Dashboard with live logs (SSE) and incident list.
5. Exit criteria:
   - stable local mode
   - deterministic incident evidence chain
   - kill/restart correctness under stress tests.

### Phase 2 (Scaling, 3–9 months): Cloud Control Plane
1. Multi-tenant API gateway + RBAC + quotas.
2. NATS event bus + dispatcher + cloud worker pools.
3. Postgres partitioning + object log storage.
4. SLO dashboards + burn-rate alerting.
5. Agent reconnect/replay and cloud outage resilience.
6. Exit criteria:
   - 1k concurrent tasks
   - 99.95% API availability target met
   - no critical audit gaps.

### Phase 3 (Enterprise, 9–18 months): Platform Differentiation
1. Policy governance, versioning, canary policy rollouts.
2. Advanced sandbox profiles (container hardening + microVM option).
3. BYOK, SSO/SCIM, compliance exports.
4. Cross-region active/standby DR posture.
5. Chargeback and tenant cost analytics.
6. Exit criteria:
   - 10k concurrent tasks
   - enterprise security/compliance feature completeness
   - strong retention and expansion economics.

---

## Final Architecture Positioning

This architecture intentionally builds long-term leverage through three durable primitives:
1. **Local autonomous supervisor** (works even when cloud is impaired).
2. **Immutable event evidence model** (debuggable, auditable, enterprise-safe).
3. **Policy-driven intervention engine** (core moat as workloads and tenants scale).

That combination is what makes it credible against infrastructure-grade incumbents: operational reliability, forensic explainability, and a path to multi-tenant scale without abandoning local-first trust and control.

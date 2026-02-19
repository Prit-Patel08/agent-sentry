# Local Daemon RFC (P1)

Status: Draft  
Owner: Core Runtime  
Target: P1 (Day 31-90)

## Problem

Current usage depends on explicit CLI commands (`flowforge run`, `flowforge demo`, `flowforge dashboard`).
This creates command fatigue and blocks zero-command adoption in IDE workflows.

## Goal

Introduce a local daemon that keeps supervision available in the background and exposes a stable local integration surface for IDE extensions.

## Non-Goals

- No cloud control plane.
- No multi-tenant model.
- No remote agent execution.
- No policy DSL expansion in this phase.

## High-Level Design

```text
IDE extension -> local daemon -> existing supervisor/runtime -> existing API/DB
```

Daemon responsibilities:

1. Workspace registration and lifecycle.
2. Attach/detach supervision for local commands.
3. Expose status/incidents for IDE UI.
4. Relay safe manual actions (pause/resume/restart/kill) with audit trail.

## Runtime Model

1. Daemon process: `flowforge daemon`
2. Bind: `127.0.0.1` only (default), separate port from dashboard API.
3. Persistent state:
   - workspace registry (path, profile, status)
   - last known protection state
4. Existing `flowforge` API remains source of incident/evidence data.

## Safety Requirements

1. No silent destructive action from IDE integration.
2. Destructive actions require explicit user action in IDE + reason text.
3. All daemon-triggered actions must write audit events (`actor=manual` or `actor=system` as appropriate).
4. Constant-time token checks for daemon auth token.

## Proposed Lifecycle

1. Start daemon on demand from first integration call.
2. Register workspace with profile (`standard`, `strict`, `watchdog`).
3. Integration requests `attach` for command execution context.
4. Daemon supervises and streams status.
5. Integration requests `pause`/`resume`/`restart`/`kill`.
6. Daemon writes action + reason to event/audit store.

## Rollout Plan

Phase 1:
- daemon skeleton + status endpoint
- workspace register/unregister

Phase 2:
- attach/detach supervision
- status + latest incident endpoints

Phase 3:
- intervention endpoints with audit enforcement
- integration soak tests

## Exit Criteria

1. New developer can enable protection from integration UI without CLI typing.
2. Crash-free daemon sessions >= 99% in soak tests.
3. All IDE-triggered actions visible in timeline/audit feed.

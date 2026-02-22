# Control-Plane Replay Drill Evidence (2026-02-22)

Date (UTC): 2026-02-22  
Owner: Reliability + API  
Drill command:

```bash
./scripts/controlplane_replay_drill.sh --api-key "$FLOWFORGE_API_KEY" --out pilot_artifacts/controlplane-replay-20260222-144928-live
```

Artifact directory:
- `pilot_artifacts/controlplane-replay-20260222-144928-live/`

## Scope

Validated idempotent mutation behavior for control-plane write paths:
1. integration workspace registration (`POST /v1/integrations/workspaces/register`)
2. workspace protection toggle (`POST /v1/integrations/workspaces/{workspace_id}/protection`)
3. workspace actions (`POST /v1/integrations/workspaces/{workspace_id}/actions`)

For each path, drill validates:
1. first call succeeds
2. second call with same key + same payload is replayed with `X-Idempotent-Replay: true`
3. third call with same key + different payload is rejected with `409`

## Results

Overall status: **PASS**

Evidence:
- `pilot_artifacts/controlplane-replay-20260222-144928-live/summary.md`
- `pilot_artifacts/controlplane-replay-20260222-144928-live/summary.tsv`
- endpoint response artifacts (`register_*`, `protection_*`, `actions_*`)

Metric snapshot (at drill time):
- `flowforge_controlplane_idempotent_replay_total`: `3`
- `flowforge_controlplane_idempotency_conflict_total`: `3`

## Findings

1. Persisted idempotency replay ledger returns deterministic replay responses across write endpoints.
2. Conflict protection is enforced when a key is reused with a different payload.
3. Replay/conflict counters are emitted for SLO visibility and weekly governance.

## Conclusion

Control-plane replay contract is verified in live local runtime and evidence is published.

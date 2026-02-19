# Rollback Checklist

Use this checklist when a release causes severe regressions.

## 1. Rollback Trigger

- [ ] Confirm trigger condition is real (failed health, broken intervention behavior, security regression, or data integrity risk).
- [ ] Capture incident summary (what changed, when, impact).
- [ ] Announce rollback decision in release channel/log.

## 2. Immediate Safety Actions

- [ ] Stop further rollout (do not cut new tags while rollback is active).
- [ ] Preserve artifacts/logs needed for postmortem.
- [ ] If required, switch operators to manual process control path.

## 3. Technical Rollback

- [ ] Identify last known good commit/tag.
- [ ] Revert release commit(s) or redeploy last known good tag.
- [ ] Push rollback commit/tag to `main`.
- [ ] Re-run CI and release checkpoint on rollback state.

## 4. Rollback Verification

- [ ] `GET /healthz` is healthy.
- [ ] `GET /readyz` is healthy.
- [ ] `GET /metrics` is present.
- [ ] Dashboard can load incidents and timeline.
- [ ] Supervised run/demo behavior returns to expected baseline.

## 5. Stabilization

- [ ] Open follow-up issue with root-cause owner.
- [ ] Add regression test for rollback trigger failure mode.
- [ ] Document final timeline and corrective actions.

## 6. Exit Criteria

Rollback is complete only when service behavior is stable and verification passes.

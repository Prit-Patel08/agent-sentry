# Onboarding Usability Test (External Validation Protocol)

Goal:
- validate that a true first-time developer can reach first value quickly with minimal help.

This protocol defines when `plan.md` item
`[ ] external first-time usability validation completed`
can be marked done.

## Definitions

Internal dry-run:
- team member runs the onboarding flow to validate tooling only.

External validation run:
- person who has never contributed to FlowForge runs the flow.
- observer can watch but must not assist except for hard blockers.

## Tester Profile (External Run)

Required:
1. has not contributed to this repository
2. can use terminal/shell basics
3. has no prior walkthrough of FlowForge internals

## Commands

Internal dry-run (tooling sanity only):

```bash
./scripts/onboarding_usability_test.sh --mode internal
```

External validation run:

```bash
./scripts/onboarding_usability_test.sh \
  --mode external \
  --tester-name "First Last" \
  --tester-role "Developer"
```

Custom output directory (recommended for external sessions):

```bash
./scripts/onboarding_usability_test.sh \
  --mode external \
  --tester-name "First Last" \
  --tester-role "Developer" \
  pilot_artifacts/onboarding-external-YYYYMMDD
```

## Artifact Contract

Each run produces:
1. `pilot_artifacts/onboarding-<timestamp>/report.md`
2. `pilot_artifacts/onboarding-<timestamp>/summary.tsv`
3. step logs in `pilot_artifacts/onboarding-<timestamp>/logs/`
4. `pilot_artifacts/onboarding-<timestamp>/external_feedback.md`
5. `pilot_artifacts/onboarding-<timestamp>/observer_notes.md`
6. reference template: `docs/ONBOARDING_EXTERNAL_FEEDBACK_TEMPLATE.md`

## Quantitative Gates

Must pass:
1. report `Overall Status` is `PASS`
2. time-to-first-value is `<= 300s`
3. demo summary includes:
   - `Runaway detected in ...`
   - `CPU peaked at ...`
   - `Process recovered`
4. API and dashboard probes succeed

## Qualitative Gates (External Run Only)

External run requires completed answers (no `TODO`) in:
1. `external_feedback.md`
2. `observer_notes.md`

Minimum feedback captured:
1. least clear step
2. output clarity vs debugging noise
3. confidence to repeat without help
4. top confusion points and concrete fix suggestions

## Definition of Done for `plan.md` Checkbox

Only mark `[x] external first-time usability validation completed` after:
1. one external validation run meets all quantitative gates
2. qualitative gates are complete
3. evidence files are committed or referenced in a published findings doc

## Recommended Session Discipline

1. Start from a fresh clone.
2. Use screen recording if possible.
3. Observer only intervenes for hard blockers.
4. Record every intervention in `observer_notes.md`.
5. Capture top 3 UX fixes with owner and due date.

# Onboarding Usability Test

Goal:
- verify a new developer can get first value quickly and predictably.

## Tester Profile

- person has not contributed to this repository before
- can use terminal and run shell scripts

## Test Procedure

From a fresh clone:

```bash
chmod +x scripts/onboarding_usability_test.sh
./scripts/onboarding_usability_test.sh
```

This produces:
- `pilot_artifacts/onboarding-<timestamp>/report.md`
- step logs under `pilot_artifacts/onboarding-<timestamp>/logs/`

## Pass Criteria

- report overall status is `PASS`
- time-to-first-value is `<= 300s`
- demo summary includes:
  - `Runaway detected in ...`
  - `CPU peaked at ...`
  - `Process recovered`
- API and dashboard probes succeed

## Human Feedback (Required)

After the run, ask the tester these 3 questions:

1. Which step was least clear?
2. Did any output look like internal debugging instead of product messaging?
3. Could you repeat the flow without help?

Record feedback in the same report directory as `feedback.txt`.

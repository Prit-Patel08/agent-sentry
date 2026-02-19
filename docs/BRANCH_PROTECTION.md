# Branch Protection Baseline

Configure `main` with required status checks before merge.

Required checks:
- `backend`
- `dashboard`
- `smoke`
- `docker`
- `sbom`
- `validate-pr-body` (from `PR Template Gate`)

Recommended settings:
- Require pull request before merging
- Require approvals: 1+
- Dismiss stale approvals on new commits
- Require branches to be up to date before merging
- Do not allow bypassing required checks for non-admin users
- Require conversation resolution before merging

This keeps release quality tied to build/test/race/staticcheck/govulncheck, dashboard build, and SBOM generation.

## GitHub UI Steps

1. Open repo `Settings` -> `Branches`.
2. Create/edit protection rule for branch name pattern `main`.
3. Enable:
   - Require a pull request before merging.
   - Require approvals (minimum 1).
   - Dismiss stale approvals when new commits are pushed.
   - Require status checks to pass before merging.
   - Require branches to be up to date before merging.
   - Require conversation resolution before merging.
4. In required checks, add:
   - `backend`
   - `dashboard`
   - `smoke`
   - `docker`
   - `sbom`
   - `validate-pr-body`
5. Save rule.

## Note

This environment does not currently have `gh` installed, so branch protection is documented for manual GitHub UI setup.

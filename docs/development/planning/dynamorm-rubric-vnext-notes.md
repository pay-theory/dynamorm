# DynamORM: Rubric vNext Notes (Not Yet Adopted)

This file captures **potential rubric improvements** discovered during remediation work.
It is **not** part of the current rubric version and should not be treated as a gate until explicitly adopted and
versioned in `docs/development/planning/dynamorm-10of10-rubric.md`.

## Candidates (discuss before adopting)

### 1) Make “CI enforces rubric” a scored rubric item

Today, M2 is a roadmap milestone but not directly scored. Consider adding a new **Completeness** item (e.g., `COM-6`)
that verifies:

- A workflow exists under `.github/workflows/` that runs `make rubric` for PRs to `premain`.
- Security-critical tools are pinned (no `@latest`).
- The workflow uploads at least `coverage_lib.out` and `gosec.sarif` as artifacts.

Rationale: prevents “10/10 locally” from drifting without CI enforcement.

### 2) Pin DynamoDB Local image (integration determinism)

Integration tests depend on DynamoDB Local. Consider a verifier to ensure we do not use `:latest` for:

- `docker-compose.yml` image tags
- Any `docker run amazon/dynamodb-local...` fallbacks

Rationale: reduces CI/non-CI drift when upstream images change.

### 3) Add a dedicated “CI surface parity” verifier

Current `scripts/verify-ci-toolchain.sh` checks for `go-version-file: go.mod` and rejects `@latest`, but it does not
assert that the **recommended rubric surface** is actually executed in CI. Consider a separate script (and rubric item)
that validates the expected workflow/commands exist.


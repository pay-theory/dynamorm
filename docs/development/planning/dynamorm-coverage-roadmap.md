# DynamORM: Coverage Roadmap (to 90% library coverage)

Goal: raise DynamORM “library coverage” to **≥ 90%** as measured by `bash scripts/verify-coverage.sh` (which uses `bash scripts/coverage.sh` and excludes `examples/` + `tests/` from the denominator).

## Current state (baseline)

Snapshot (2026-01-09):

- `bash scripts/verify-coverage.sh`: **51.2%** vs threshold **90%** (fails)

## Guardrails (no denominator games)

- Do not reduce the measurement surface by excluding additional production packages from `scripts/coverage.sh`.
- Do not claim coverage progress by moving logic into `examples/` or `tests/`.
- Keep `COVERAGE_THRESHOLD` as a *raise-only* override (the verifier rejects lowering below default).

## How we measure

Generate a coverage profile:

```bash
bash scripts/coverage.sh
```

Final rubric gate (overall threshold, default 90%):

```bash
bash scripts/verify-coverage.sh
```

## Workstreams

### 1) Stabilize the test harness (make adding tests cheap)

- Prefer table-driven unit tests for pure/near-pure packages.
- For DynamoDB behavior, keep integration tests focused and deterministic (fixtures, teardown, no sleeps).

### 2) Target the highest-leverage code paths first

Initial hotspots to prioritize (high churn / high surface area):

- `dynamorm.go`
- `pkg/core`
- `pkg/query`
- `pkg/marshal`
- `pkg/schema`

### 3) Close common gap patterns

- Error paths (DynamoDB conditional failures, throttling/retry paths, marshaling failures).
- Option handling (zero values, `omitempty`, conditional clauses, return values).
- Edge cases (empty collections, nested structs, pointer fields).

## Proposed milestones

This repo currently has a single hard gate at 90%. To make progress reviewable, adopt incremental milestones (modeled after K3) such as:

- COV-1: remove “0% islands” (every production package has tests)
- COV-2: broad floor (25%+)
- COV-3: meaningful safety net (50%+)
- COV-4: high confidence (70%+)
- COV-5: pre-finish (80%+)
- COV-6: finish line (90%+ + pass `bash scripts/verify-coverage.sh`)

Implementation note: to enforce package-level floors, add a targets-based verifier + targets files under `docs/development/planning/coverage-targets/` (same pattern as K3) rather than weakening the global gate.


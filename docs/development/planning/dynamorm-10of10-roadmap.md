# DynamORM: 10/10 Roadmap (Rubric v0.4)

This roadmap is the execution plan for achieving and maintaining **10/10** across **Quality**, **Consistency**,
**Completeness**, **Security**, **Maintainability**, and **Docs** as defined by:

- `docs/development/planning/dynamorm-10of10-rubric.md` (source of truth; versioned)

## Current scorecard (Rubric v0.4)

Scoring note: a check is only treated as “passing” if it is both green **and** enforced by a trustworthy verifier
(pinned toolchain, stable commands, and no “green by exclusion” shortcuts).

| Category | Grade | Blocking rubric items |
| --- | ---: | --- |
| Quality | 10/10 | — |
| Consistency | 10/10 | — |
| Completeness | 10/10 | — |
| Security | 10/10 | — |
| Maintainability | 3/10 | `MAI-1`, `MAI-3` |
| Docs | 10/10 | — |

Evidence (refresh whenever behavior changes):

- `make test-unit`
- `make integration`
- `bash scripts/verify-coverage.sh` (current: **90.1%** vs threshold **90%**)
- `bash scripts/verify-coverage-threshold.sh` (default threshold **90%**)
- `bash scripts/fmt-check.sh`
- `golangci-lint config verify -c .golangci-v2.yml`
- `make lint`
- `bash scripts/verify-go-modules.sh`
- `bash scripts/verify-ci-toolchain.sh`
- `bash scripts/verify-ci-rubric-enforced.sh`
- `bash scripts/verify-dynamodb-local-pin.sh`
- `bash scripts/verify-threat-controls-parity.sh`
- `bash scripts/verify-doc-integrity.sh`
- `bash scripts/verify-no-panics.sh`
- `bash scripts/verify-safe-defaults.sh`
- `bash scripts/verify-network-hygiene.sh`
- `bash scripts/verify-encrypted-tag-implemented.sh`
- `bash scripts/verify-go-file-size.sh` (**expected FAIL** until `dynamorm.go` is decomposed)
- `bash scripts/verify-maintainability-roadmap.sh`
- `bash scripts/verify-query-singleton.sh` (**expected FAIL** until query implementations converge)
- `bash scripts/verify-validation-parity.sh`
- `bash scripts/fuzz-smoke.sh`
- `bash scripts/sec-gosec.sh`
- `bash scripts/sec-govulncheck.sh`
- `go mod verify`

## Milestones (map directly to rubric IDs)

### M0 — Freeze rubric + planning artifacts

**Closes:** COM-3, DOC-1, DOC-2, DOC-3  
**Goal:** prevent goalpost drift by making the definition of “good” explicit and versioned.

**Acceptance criteria**
- Rubric exists and is versioned.
- Threat model exists and is owned.
- Evidence plan maps rubric IDs → verifiers → artifacts.

---

### M1 — Lint remediation (get `make lint` green)

**Closes:** CON-2  
**Goal:** remove surprises by making strict lint enforcement sustainable (no “works on my machine” exceptions).

Tracking document: `docs/development/planning/dynamorm-lint-green-roadmap.md`

**Acceptance criteria**
- `golangci-lint config verify -c .golangci-v2.yml` is green.
- `make lint` is green (0 issues) with `.golangci-v2.yml` (no threshold loosening and no new blanket excludes).
- Any `//nolint` usage is line-scoped and justified; remove stale linter names (e.g., `unusedparams`, `unusedwrite`).

---

### M1.5 — Coverage remediation (hit 90% and keep it honest)

**Closes:** QUA-3  
**Goal:** raise library coverage to **≥ 90%** without reducing the measurement surface.

Tracking document: `docs/development/planning/dynamorm-coverage-roadmap.md`

**Prerequisite**
- M1 is complete (lint is green). During the coverage push, treat `make lint` as a regression gate and keep it green after every coverage pass.

**Acceptance criteria**
- `make test-unit` is green.
- `make integration` is green (DynamoDB Local).
- `bash scripts/verify-coverage.sh` is green at the default threshold (≥ 90%).

Guardrails (no denominator games):
- Do not exclude production packages from `scripts/coverage.sh` beyond the existing `examples/` + `tests/` filtering.
- If we need package-level floors, add a targets-based verifier (modeled after K3) rather than weakening the global gate.

---

### M2 — Enforce the loop in CI (after remediation)

**Closes:** COM-6  
**Goal:** run the recommended rubric surface on every PR with pinned tooling.

**Acceptance criteria**
- CI runs the recommended surface from `docs/development/planning/dynamorm-10of10-rubric.md`.
- Tooling is pinned (no `@latest` for security-critical verifiers).

**Implementation (in repo)**
- Workflow: `.github/workflows/quality-gates.yml` runs `make rubric` on PRs to `premain` (and on pushes to `premain`).
- Tooling pins: `golangci-lint@v2.5.0`, `govulncheck@v1.1.4`, `gosec@v2.22.11` (plus `go.mod` toolchain `go1.25.3` via `go-version-file`).
- Integration infra pin: DynamoDB Local uses `amazon/dynamodb-local:3.1.0` (via `docker-compose.yml` and `DYNAMODB_LOCAL_IMAGE`).

---

### M2.5 — Determinism gates (integration stability)

**Closes:** COM-7  
**Goal:** reduce CI/non-CI drift by pinning integration infrastructure dependencies.

**Acceptance criteria**
- `bash scripts/verify-dynamodb-local-pin.sh` is green.

---

### M3 — Safety defaults (availability + security posture)

**Closes:** SEC-4, SEC-5, SEC-7  
**Goal:** make “safe by default” true in code paths that handle PHI/PII/CHD-like data, and prevent runtime crashers.

**Acceptance criteria**
- `bash scripts/verify-no-panics.sh` is green (no panics in production paths).
- `bash scripts/verify-safe-defaults.sh` is green (unsafe marshaling not wired into defaults).
- `bash scripts/verify-network-hygiene.sh` is green (HTTP timeouts + reviewed retry posture).

---

### M3.5 — Boundary hardening (validator parity + fuzz smoke)

**Closes:** QUA-4, QUA-5  
**Goal:** ensure inputs accepted by validators don’t crash downstream conversion/expression building, and add a cheap
“unknown unknown” detector for crashers.

**Acceptance criteria**
- `bash scripts/verify-validation-parity.sh` is green (no panics; errors are surfaced safely).
- `bash scripts/fuzz-smoke.sh` is green (bounded fuzz pass with at least one Fuzz target per package group).

---

### M3.75 — Implement `dynamorm:"encrypted"` semantics (KMS Key ARN only)

**Closes:** SEC-8  
**Goal:** remove “metadata-only encryption” risk by implementing real field-level encryption semantics with a provided KMS key ARN.

Tracking document: `docs/development/planning/dynamorm-encryption-tag-roadmap.md`

**Acceptance criteria**
- `bash scripts/verify-encrypted-tag-implemented.sh` is green.
- Encrypted fields fail closed if key ARN is not configured.
- Encrypted fields are rejected for PK/SK/index keys and are not queryable.

---

### M4 — Docs integrity + risk→control traceability

**Closes:** DOC-4, DOC-5  
**Goal:** prevent silent documentation drift and ensure every named top threat has at least one mapped control.

**Acceptance criteria**
- `bash scripts/verify-doc-integrity.sh` is green (internal links resolve; version claims match go.mod).
- `bash scripts/verify-threat-controls-parity.sh` is green (every `THR-*` maps to at least one control).

---

### M5 — Maintainability convergence (decompose + unify query)

**Closes:** MAI-1, MAI-2, MAI-3  
**Goal:** keep the codebase structurally convergent so future changes remain reviewable and safe.

Tracking document: `docs/development/planning/dynamorm-maintainability-roadmap.md`

**Acceptance criteria**
- `bash scripts/verify-go-file-size.sh` is green.
- `bash scripts/verify-maintainability-roadmap.sh` is green.
- `bash scripts/verify-query-singleton.sh` is green.

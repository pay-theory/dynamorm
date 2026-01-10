# DynamORM: 10/10 Roadmap (Rubric v0.2)

This roadmap is the execution plan for achieving and maintaining **10/10** across **Quality**, **Consistency**,
**Completeness**, **Security**, and **Docs** as defined by:

- `docs/development/planning/dynamorm-10of10-rubric.md` (source of truth; versioned)

## Current scorecard (Rubric v0.2)

Scoring note: a check is only treated as “passing” if it is both green **and** enforced by a trustworthy verifier
(pinned toolchain, stable commands, and no “green by exclusion” shortcuts).

| Category | Grade | Blocking rubric items |
| --- | ---: | --- |
| Quality | 7/10 | QUA-3 |
| Consistency | 10/10 | — |
| Completeness | 10/10 | — |
| Security | 10/10 | — |
| Docs | 10/10 | — |

Evidence (refresh whenever behavior changes):

- ✅ `make test-unit`
- ✅ `make integration`
- ❌ `bash scripts/verify-coverage.sh` (current: **64.3%** vs threshold **90%**)
- ✅ `bash scripts/verify-coverage-threshold.sh` (default threshold **90%**)
- ✅ `bash scripts/fmt-check.sh`
- ✅ `golangci-lint config verify -c .golangci-v2.yml`
- ✅ `make lint` (0 issues)
- ✅ `bash scripts/verify-go-modules.sh`
- ✅ `bash scripts/verify-ci-toolchain.sh`
- ✅ `bash scripts/sec-gosec.sh`
- ✅ `bash scripts/sec-govulncheck.sh`
- ✅ `go mod verify`

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

**Closes:** (durability milestone; supports all categories)  
**Goal:** run the recommended rubric surface on every PR with pinned tooling.

**Acceptance criteria**
- CI runs the recommended surface from `docs/development/planning/dynamorm-10of10-rubric.md`.
- Tooling is pinned (no `@latest` for security-critical verifiers).

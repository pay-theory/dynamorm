# DynamORM: 10/10 Roadmap (Rubric v0.1)

This roadmap is the execution plan for achieving and maintaining **10/10** across **Quality**, **Consistency**,
**Completeness**, **Security**, and **Docs** as defined by:

- `docs/development/planning/dynamorm-10of10-rubric.md` (source of truth; versioned)

## Current scorecard (Rubric v0.1)

Scoring note: a check is only treated as “passing” if it is both green **and** enforced by a trustworthy verifier
(pinned toolchain, stable commands, and no “green by exclusion” shortcuts).

| Category | Grade | Blocking rubric items |
| --- | ---: | --- |
| Quality | 10/10 | — |
| Consistency | 10/10 | — |
| Completeness | 10/10 | — |
| Security | 10/10 | — |
| Docs | 10/10 | — |

Evidence (refresh whenever behavior changes):

- `make test-unit`
- `make integration`
- `bash scripts/verify-coverage.sh`
- `bash scripts/fmt-check.sh`
- `make lint`
- `bash scripts/verify-go-modules.sh`
- `bash scripts/verify-ci-toolchain.sh`
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

### M1 — Install and enforce the gates in CI

**Closes:** COM-2 + all category verifiers  
**Goal:** make the “10/10” loop runnable on every PR (not just locally).

**Acceptance criteria**
- CI runs the recommended rubric surface from `docs/development/planning/dynamorm-10of10-rubric.md`.
- Tools are pinned (no `@latest` for security-critical verifiers).

---

### M2 — Raise the bar (planned rubric bump)

**Goal:** increase confidence without breaking determinism.

Planned follow-ups (require a rubric version bump):

- Increase the default coverage threshold in `scripts/verify-coverage.sh`.
- Add fuzz tests for expression parsing / marshaling edge cases.
- Add additional SAST/SCA checks if they provide signal without false-positive churn.


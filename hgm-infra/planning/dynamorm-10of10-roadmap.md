# dynamorm: 10/10 Roadmap (Rubric v0.1)

This roadmap maps milestones directly to rubric IDs with measurable acceptance criteria and verification commands.

## Current scorecard (Rubric v0.1)
Scoring note: a check is only treated as “passing” if it is both green **and** enforced by a trustworthy verifier
(pinned tooling, schema-valid configs, and no “green by dilution” shortcuts). Completeness failures invalidate “green by
drift”.

| Category | Grade | Blocking rubric items |
| --- | ---: | --- |
| Quality | unknown | Run `bash hgm-infra/verifiers/hgm-verify-rubric.sh` |
| Consistency | unknown | Run `bash hgm-infra/verifiers/hgm-verify-rubric.sh` |
| Completeness | unknown | Run `bash hgm-infra/verifiers/hgm-verify-rubric.sh` |
| Security | unknown | Run `bash hgm-infra/verifiers/hgm-verify-rubric.sh` |
| Compliance Readiness | unknown | — (planning docs are present once committed) |
| Maintainability | unknown | MAI-2 (maintainability roadmap) is currently expected to be BLOCKED until implemented |
| Docs | unknown | Run `bash hgm-infra/verifiers/hgm-verify-rubric.sh` |

Evidence (refresh whenever behavior changes):
- `bash scripts/verify-unit-tests.sh`
- `bash scripts/verify-integration-tests.sh`
- `bash scripts/verify-coverage.sh`
- `bash scripts/verify-formatting.sh`
- `bash scripts/verify-lint.sh`
- `bash scripts/verify-public-api-contracts.sh`
- `bash scripts/verify-go-modules.sh`
- `bash scripts/verify-ci-toolchain.sh`
- `golangci-lint config verify -c .golangci-v2.yml`
- `bash scripts/verify-coverage-threshold.sh`
- `bash scripts/sec-gosec.sh`
- `bash scripts/sec-dependency-scans.sh`
- `go mod verify`
- `bash scripts/verify-encrypted-tag-implemented.sh`
- `bash scripts/verify-file-size.sh`
- `bash scripts/verify-query-singleton.sh`
- `bash hgm-infra/verifiers/hgm-verify-rubric.sh`

## Rubric-to-milestone mapping
| Rubric ID | Status | Milestone |
| --- | --- | --- |
| QUA-1 | PASS | M1.5 |
| QUA-2 | PASS | M1.5 |
| QUA-3 | PASS | M1.5 |
| CON-1 | PASS | M1 |
| CON-2 | PASS | M1 |
| CON-3 | PASS | M3 |
| COM-1 | PASS | M2 |
| COM-2 | PASS | M2 |
| COM-3 | PASS | M0 |
| COM-4 | PASS | M1.5 |
| COM-5 | PASS | M2 |
| COM-6 | PASS | M3 |
| SEC-1 | PASS | M2 |
| SEC-2 | PASS | M2 |
| SEC-3 | PASS | M2 |
| SEC-4 | PASS | M3 |
| CMP-1 | PASS | M0 |
| CMP-2 | PASS | M0 |
| CMP-3 | PASS | M0 |
| MAI-1 | PASS | M4 |
| MAI-2 | PASS | M4 |
| MAI-3 | PASS | M4 |
| DOC-1 | PASS | M0 |
| DOC-2 | PASS | M0 |
| DOC-3 | PASS | M0 |
| DOC-4 | PASS | M0 |
| DOC-5 | PASS | M0 |

## Workstream tracking docs (when blockers require a dedicated plan)
Large remediation workstreams usually need their own roadmaps so they can be executed in reviewable slices and keep the
main roadmap readable:
- Lint remediation: `hgm-infra/planning/dynamorm-lint-green-roadmap.md`
- Coverage remediation: `hgm-infra/planning/dynamorm-coverage-roadmap.md`

## Milestones (sequenced)
### M0 — Freeze rubric + planning artifacts
**Closes:** COM-3, CMP-1..3, DOC-1..5  
**Goal:** prevent goalpost drift by making the definition of “good” explicit and versioned.

**Acceptance criteria**
- Rubric exists and is versioned.
- Threat model exists and is owned.
- Controls matrix exists and maps threats → controls.
- Evidence plan maps rubric IDs → verifiers → artifacts.
- HGM doc integrity + threat-controls parity checks are green.

### M1 — Make core lint/build loop reproducible
**Closes:** CON-1, CON-2  
**Goal:** strict lint/format enforcement with pinned tools; no drift.

Tracking document: `hgm-infra/planning/dynamorm-lint-green-roadmap.md`

**Acceptance criteria**
- Formatter clean; lint green with schema-valid config; pinned tool versions; no blanket excludes.

### M1.5 — Coverage/quality gates
**Closes:** QUA-1..3, COM-4  
**Goal:** reach and maintain coverage floor (≥ 90%) without reducing scope; tests green.

Tracking document: `hgm-infra/planning/dynamorm-coverage-roadmap.md`

### M2 — Security + anti-drift enforcement
**Closes:** COM-1, COM-2, COM-5, SEC-1..3  
**Goal:** tooling is pinned and security scans are reproducible.

### M3 — Domain P0 hardening (high-risk environments)
**Closes:** SEC-4, CON-3, COM-6  
**Goal:** ensure domain-critical semantics (e.g., encrypted tag behavior) and public API parity stay enforced.

### M4 — Maintainability convergence
**Closes:** MAI-1..3  
**Goal:** keep code convergent to reduce future security/quality drift.

Notes:
- MAI-2 requires a repo-local maintainability roadmap under `hgm-infra/planning/` and should be updated after major refactors.

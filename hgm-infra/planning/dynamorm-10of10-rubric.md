# dynamorm: 10/10 Rubric (Quality, Consistency, Completeness, Security, Compliance Readiness, Maintainability, Docs)

This rubric defines what “10/10” means and how category grades are computed. It is designed to prevent goalpost drift and
“green by dilution” by making scoring **versioned, measurable, and repeatable**.

## Versioning (no moving goalposts)
- **Rubric version:** `v0.1` (2026-01-17)
- **Comparability rule:** grades are comparable only within the same version.
- **Change rule:** bump the version + changelog entry for any rubric change (what changed + why).

### Changelog
- `v0.1`: Initial Hypergenium governance scaffold under `hgm-infra/`.

## Scoring (deterministic)
- Each category is scored **0–10**.
- Point weights sum to **10** per category.
- Requirements are **pass/fail** (either earn full points or 0).
- A category is **10/10 only if all requirements in that category pass**.

## Verification (commands + deterministic artifacts are the source of truth)
Every rubric item has exactly one verification mechanism:
- a command (`make ...`, `go test ...`, `bash scripts/...`), or
- a deterministic artifact check (required doc exists and matches an agreed format).

Enforcement rule (anti-drift):
- If an item’s verifier is a command/script, it only counts as passing once it runs and produces evidence under `hgm-infra/evidence/`.

---

## Quality (QUA) — reliable, testable, change-friendly
| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| QUA-1 | 4 | Unit tests stay green | `bash scripts/verify-unit-tests.sh` |
| QUA-2 | 3 | Integration or contract tests stay green | `bash scripts/verify-integration-tests.sh` |
| QUA-3 | 3 | Coverage ≥ 90% (no denominator games) | `bash scripts/verify-coverage.sh` |

**10/10 definition:** QUA-1 through QUA-3 pass.

## Consistency (CON) — one way to do the important things
| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| CON-1 | 3 | gofmt/formatter clean (no diffs) | `bash scripts/verify-formatting.sh` |
| CON-2 | 5 | Lint/static analysis green (pinned version) | `bash scripts/verify-lint.sh` |
| CON-3 | 2 | Public boundary contract parity (if applicable) | `bash scripts/verify-public-api-contracts.sh` |

**10/10 definition:** CON-1 through CON-3 pass.

## Completeness (COM) — verify the verifiers (anti-drift)
| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| COM-1 | 2 | All modules compile (no “mystery meat”) | `bash scripts/verify-go-modules.sh` |
| COM-2 | 2 | Toolchain pins align to repo (Go/lint/tool versions) | `bash scripts/verify-ci-toolchain.sh` |
| COM-3 | 2 | Lint config schema-valid (no silent skip) | `golangci-lint config verify -c .golangci-v2.yml` |
| COM-4 | 2 | Coverage threshold not diluted (≥ 90%) | `bash scripts/verify-coverage-threshold.sh` |
| COM-5 | 1 | Security scan config not diluted (no excluded high-signal rules) | `check_security_config_not_diluted` (implemented in `hgm-infra/verifiers/hgm-verify-rubric.sh`) |
| COM-6 | 1 | Logging/operational standards enforced (if applicable) | `TODO: add logging/operational standards verifier` |

**10/10 definition:** COM-1 through COM-6 pass.

## Security (SEC) — abuse-resilient and reviewable
| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| SEC-1 | 3 | Static security scan green (pinned version) | `bash scripts/sec-gosec.sh` |
| SEC-2 | 3 | Dependency vulnerability scan green | `bash scripts/sec-dependency-scans.sh` |
| SEC-3 | 2 | Supply-chain verification green | `go mod verify` |
| SEC-4 | 2 | Domain-specific P0 regression tests (e.g., CHD/SAD/PHI) | `bash scripts/verify-encrypted-tag-implemented.sh` |

**10/10 definition:** SEC-1 through SEC-4 pass.

## Compliance Readiness (CMP) — auditability and evidence
| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| CMP-1 | 4 | Controls matrix exists and is current | File exists: `hgm-infra/planning/dynamorm-controls-matrix.md` |
| CMP-2 | 3 | Evidence plan exists and is reproducible | File exists: `hgm-infra/planning/dynamorm-evidence-plan.md` |
| CMP-3 | 3 | Threat model exists and is current | File exists: `hgm-infra/planning/dynamorm-threat-model.md` |

**10/10 definition:** CMP-1 through CMP-3 pass.

## Maintainability (MAI) — convergent codebase (recommended for AI-heavy repos)
| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| MAI-1 | 4 | File-size/complexity budgets enforced | `bash scripts/verify-file-size.sh` |
| MAI-2 | 3 | Maintainability roadmap current | `TODO: add maintainability roadmap + verifier under hgm-infra/planning/` |
| MAI-3 | 3 | Canonical implementations (no duplicate semantics) | `bash scripts/verify-query-singleton.sh` |

**10/10 definition:** MAI-1 through MAI-3 pass.

## Docs (DOC) — integrity and parity
| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| DOC-1 | 2 | Threat model present | File exists: `hgm-infra/planning/dynamorm-threat-model.md` |
| DOC-2 | 2 | Evidence plan present | File exists: `hgm-infra/planning/dynamorm-evidence-plan.md` |
| DOC-3 | 2 | Rubric + roadmap present | File exists: `hgm-infra/planning/dynamorm-10of10-rubric.md` |
| DOC-4 | 2 | Doc integrity (links, version claims) | `check_hgm_doc_integrity` (implemented in `hgm-infra/verifiers/hgm-verify-rubric.sh`) |
| DOC-5 | 2 | Threat ↔ controls parity | `bash hgm-infra/verifiers/hgm-verify-rubric.sh # (DOC-5 parity check is built-in)` |

**10/10 definition:** DOC-1 through DOC-5 pass.

## Maintaining 10/10 (recommended CI surface)
```bash
bash scripts/verify-unit-tests.sh
bash scripts/verify-integration-tests.sh
bash scripts/verify-coverage-threshold.sh
bash scripts/verify-coverage.sh

bash scripts/verify-formatting.sh
bash scripts/verify-lint.sh
bash scripts/verify-public-api-contracts.sh

bash scripts/verify-go-modules.sh
bash scripts/verify-ci-toolchain.sh
golangci-lint config verify -c .golangci-v2.yml

bash scripts/sec-gosec.sh
bash scripts/sec-dependency-scans.sh
go mod verify

bash scripts/verify-file-size.sh
bash scripts/verify-query-singleton.sh

bash hgm-infra/verifiers/hgm-verify-rubric.sh
```

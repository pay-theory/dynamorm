# DynamORM: 10/10 Rubric (Quality, Consistency, Completeness, Security, Maintainability, Docs)

This rubric defines what “10/10” means for DynamORM and how category grades are computed.
It is designed for an **AI-generated codebase**: gates must be **versioned, measurable, repeatable**, and resistant to
“green by drift” (lowered thresholds, excluded scopes, unpinned tools).

## Versioning (no moving goalposts)

- **Rubric version:** `v0.5` (2026-01-11)
- **Comparability rule:** grades are only comparable within the same rubric version.
- **Change rule:** rubric changes must bump the version and include a brief changelog entry (what changed + why).

### Changelog

- `v0.5` (2026-01-11): Add explicit gates for (1) **public API/tag contract consistency** (including unmarshalling helpers), (2) **expression boundary hardening** (including list index updates), and (3) **branch/release supply-chain** controls (main releases, premain prereleases). Rebalance point weights accordingly.
- `v0.4` (2026-01-10): Add **Maintainability** category gates (file-size budget, documented maintainability plan, and “one query implementation” pressure) and add **SEC-8** to require enforced semantics for `dynamorm:"encrypted"` (no metadata-only security tags).
- `v0.3` (2026-01-10): Add **high-risk domain safety gates** that catch “10/10 but still risky” failure modes: CI rubric enforcement, DynamoDB Local pinning, doc integrity checks, threat-model↔controls parity, panic bans in production paths, safe-by-default marshaling, network hygiene defaults, validator↔converter parity, and bounded fuzz smoke passes.
- `v0.2` (2026-01-09): Require **90%** library coverage for **QUA-3**, add anti-dilution completeness gates for lint config validity and coverage threshold, and treat `.golangci-v2.yml` as the source of truth for `make lint`.
- `v0.1` (2026-01-09): Initial rubric for DynamORM.

## Scoring (deterministic)

- Each category is scored **0–10**.
- Each category has requirements with fixed point weights that sum to **10**.
- Requirements are **pass/fail** (either earn the full points or earn 0).
- A category is **10/10 only if all requirements in that category pass**.

## Verification (commands + deterministic artifacts are the source of truth)

Every rubric item has exactly one verification mechanism:

- a command (`make ...`, `go test ...`, `bash scripts/...`), or
- a deterministic artifact check (required doc exists and matches an agreed format).

Enforcement rule (to prevent “green by omission”):
- If an item’s verification mechanism is a command/script, it is only treated as passing once it is wired into `make rubric` (and run in CI for protected branches).

---

## Quality (QUA) — reliable behavior with regression coverage

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| QUA-1 | 3 | Unit tests stay green | `make test-unit` |
| QUA-2 | 2 | Integration tests stay green (DynamoDB Local required) | `make integration` |
| QUA-3 | 2 | Library coverage stays at or above the threshold (default **90%**) | `bash scripts/verify-coverage.sh` |
| QUA-4 | 2 | Validator ↔ converter parity (no “validated but crashes” inputs) | `bash scripts/verify-validation-parity.sh` |
| QUA-5 | 1 | Bounded fuzz smoke pass for crashers | `bash scripts/fuzz-smoke.sh` |

**10/10 definition:** QUA-1 through QUA-5 pass.

---

## Consistency (CON) — one way to do the important things

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| CON-1 | 3 | Go formatting is clean (no diffs) | `bash scripts/fmt-check.sh` |
| CON-2 | 5 | Lint stays green | `make lint` |
| CON-3 | 2 | Public API contract parity (exported helpers respect canonical DynamORM tags/metadata semantics) | `bash scripts/verify-public-api-contracts.sh` |

**10/10 definition:** CON-1 through CON-3 pass.

---

## Completeness (COM) — no drift, no mystery meat

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| COM-1 | 1 | All Go modules compile (including examples) | `bash scripts/verify-go-modules.sh` |
| COM-2 | 1 | CI toolchain aligns to repo expectations (Go + pinned tool versions) | `bash scripts/verify-ci-toolchain.sh` |
| COM-3 | 1 | Planning docs exist and are versioned | `bash scripts/verify-planning-docs.sh` |
| COM-4 | 1 | Lint configuration is schema-valid for golangci-lint v2 | `golangci-lint config verify -c .golangci-v2.yml` |
| COM-5 | 1 | Coverage gate configuration is not diluted (default threshold ≥ 90%) | `bash scripts/verify-coverage-threshold.sh` |
| COM-6 | 2 | CI enforces rubric surface (runs `make rubric`, pinned tools, uploads artifacts) | `bash scripts/verify-ci-rubric-enforced.sh` |
| COM-7 | 1 | DynamoDB Local image is pinned (no `:latest`) | `bash scripts/verify-dynamodb-local-pin.sh` |
| COM-8 | 2 | Branch + release supply-chain is enforced (`main` releases, `premain` prereleases; automated tagging/changelog; protections documented) | Artifact check: `.github/workflows/release.yml`, `.github/workflows/prerelease.yml`, `docs/development/planning/dynamorm-branch-release-policy.md` |

**10/10 definition:** COM-1 through COM-8 pass.

---

## Security (SEC) — abuse-resilient and reviewable by default

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| SEC-1 | 1 | Static security scan stays green (first-party only) | `bash scripts/sec-gosec.sh` |
| SEC-2 | 2 | Dependency vulnerability scan stays green | `bash scripts/sec-govulncheck.sh` |
| SEC-3 | 1 | Supply-chain verification stays green | `go mod verify` |
| SEC-4 | 2 | No `panic(...)` in production paths | `bash scripts/verify-no-panics.sh` |
| SEC-5 | 1 | Safe-by-default marshaling (unsafe only via explicit opt-in) | `bash scripts/verify-safe-defaults.sh` |
| SEC-6 | 1 | Expression boundary hardening (no injection-by-construction; list index update paths validated) | `bash scripts/verify-expression-hardening.sh` |
| SEC-7 | 1 | Network hygiene defaults (timeouts + retry posture) | `bash scripts/verify-network-hygiene.sh` |
| SEC-8 | 1 | `dynamorm:"encrypted"` has enforced semantics (KMS Key ARN required; no metadata-only tag) | `bash scripts/verify-encrypted-tag-implemented.sh` |

**10/10 definition:** SEC-1 through SEC-8 pass.

---

## Maintainability (MAI) — keep the codebase convergent

This category exists because AI-assisted code generation often “works” but accumulates long-lived structural debt:
monolithic files, duplicate implementations, and unclear canonical paths. In a high-risk domain, these are
reliability and security risks because they make future changes harder to reason about and easier to drift.

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| MAI-1 | 4 | Production Go files stay under a line-count budget (no “god files”) | `bash scripts/verify-go-file-size.sh` |
| MAI-2 | 3 | Maintainability roadmap exists and is current (hotspots + convergence plan) | `bash scripts/verify-maintainability-roadmap.sh` |
| MAI-3 | 3 | One canonical Query implementation (avoid parallel semantics drift) | `bash scripts/verify-query-singleton.sh` |

**10/10 definition:** MAI-1 through MAI-3 pass.

---

## Docs (DOC) — threat model + evidence as code

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| DOC-1 | 2 | Threat model exists and is current | `bash scripts/verify-planning-docs.sh` |
| DOC-2 | 2 | Evidence plan exists and is reproducible | `bash scripts/verify-planning-docs.sh` |
| DOC-3 | 2 | Rubric + roadmap exist and are current | `bash scripts/verify-planning-docs.sh` |
| DOC-4 | 2 | Docs integrity (no broken internal links; version claims match code) | `bash scripts/verify-doc-integrity.sh` |
| DOC-5 | 2 | Threat model ↔ controls parity (every `THR-*` maps to a control) | `bash scripts/verify-threat-controls-parity.sh` |

**10/10 definition:** DOC-1 through DOC-5 pass.

---

## Recommended CI surface (keep grades stable)

```bash
# NOTE (v0.5): This surface intentionally includes new verifiers that may not exist yet.
# Keep the rubric definition strict; land the verifiers/workflows in follow-up remediation PRs.

bash scripts/verify-planning-docs.sh
bash scripts/verify-threat-controls-parity.sh
bash scripts/verify-doc-integrity.sh
bash scripts/fmt-check.sh
golangci-lint config verify -c .golangci-v2.yml
make lint
bash scripts/verify-public-api-contracts.sh

make test-unit
make integration
bash scripts/verify-coverage-threshold.sh
bash scripts/verify-coverage.sh

bash scripts/verify-go-modules.sh
bash scripts/verify-ci-toolchain.sh
bash scripts/verify-ci-rubric-enforced.sh
bash scripts/verify-dynamodb-local-pin.sh

bash scripts/verify-no-panics.sh
bash scripts/verify-safe-defaults.sh
bash scripts/verify-network-hygiene.sh
bash scripts/verify-expression-hardening.sh
bash scripts/verify-encrypted-tag-implemented.sh
bash scripts/verify-go-file-size.sh
bash scripts/verify-maintainability-roadmap.sh
bash scripts/verify-query-singleton.sh
bash scripts/verify-validation-parity.sh
bash scripts/fuzz-smoke.sh

bash scripts/sec-gosec.sh
bash scripts/sec-govulncheck.sh
go mod verify
```

# dynamorm Controls Matrix (custom — v0.1)

This matrix is the “requirements → controls → verifiers → evidence” backbone for dynamorm. It is intentionally
engineering-focused: it does not claim compliance, but it makes security/quality assertions traceable and repeatable.

## Scope
- **System:** Dynamorm (multi-language DynamoDB library + tooling) used in security-critical production applications.
- **In-scope data:** PII, authentication/session tokens, secrets, and cardholder data (CHD) *by transitive usage* (data stored by consuming services).
- **Environments:** local dev, CI, staging, production; “prod-like” means CI running the full rubric surface with pinned tooling and DynamoDB Local where required.
- **Third parties:** AWS (DynamoDB, STS, KMS, Lambda), GitHub Actions, npm registry, PyPI.
- **Out of scope:** consuming service IAM/policy design, app-layer authn/authz, environment hardening (owned by consuming services).
- **Assurance target:** audit-ready engineering evidence for a high-risk shared library.

## Threats (reference IDs)
- Enumerate threats as stable IDs (`THR-*`) in `hgm-infra/planning/dynamorm-threat-model.md`.
- Each `THR-*` must map to ≥1 row in the controls table below.

## Status (evidence-driven)
If you track implementation status, treat it as evidence-driven:
- `unknown`: no verifier/evidence yet
- `partial`: some controls exist but coverage/evidence is incomplete
- `implemented`: verifier exists and evidence path is repeatable

## Engineering Controls (Threat → Control → Verifier → Evidence)
This table is the canonical mapping used by the rubric/roadmap/evidence plan.

| Area | Threat IDs | Control ID | Requirement | Control (what we implement) | Verification (command/gate) | Evidence (artifact/location) |
| --- | --- | --- | --- | --- | --- | --- |
| Quality | THR-1, THR-2, THR-3 | QUA-1 | Unit tests prevent regressions | Unit tests cover core update/query/marshal semantics across supported packages/languages | `bash scripts/verify-unit-tests.sh` | `hgm-infra/evidence/QUA-1-output.log` |
| Quality | THR-1, THR-2, THR-4 | QUA-2 | Integration/contract tests prevent runtime regressions | DynamoDB Local integration tests + contract tests (where applicable) | `bash scripts/verify-integration-tests.sh` | `hgm-infra/evidence/QUA-2-output.log` |
| Quality | THR-1, THR-2, THR-3 | QUA-3 | Coverage threshold is enforced (no denominator games) | Coverage gates are raise-only and default to ≥90% | `bash scripts/verify-coverage.sh` | `hgm-infra/evidence/QUA-3-output.log` |
| Consistency | — | CON-1 | Formatting is clean (no diffs) | gofmt + language formatters enforced | `bash scripts/verify-formatting.sh` | `hgm-infra/evidence/CON-1-output.log` |
| Consistency | THR-6 | CON-2 | Lint/static analysis is enforced (pinned toolchain) | golangci-lint + ruff/eslint (when applicable) stay green under pinned versions | `bash scripts/verify-lint.sh` | `hgm-infra/evidence/CON-2-output.log` |
| Consistency | THR-7 | CON-3 | Public API contract parity | Exported helper semantics match canonical Dynamorm tag/metadata semantics | `bash scripts/verify-public-api-contracts.sh` | `hgm-infra/evidence/CON-3-output.log` |
| Completeness | THR-6 | COM-1 | All modules compile (no “mystery meat”) | All in-repo Go modules compile (and multi-language builds if present) | `bash scripts/verify-go-modules.sh` | `hgm-infra/evidence/COM-1-output.log` |
| Completeness | THR-6 | COM-2 | CI/toolchain pins align to repo expectations | CI uses `go-version-file: go.mod`, Node/Python versions pinned, and tooling is pinned (no `latest`) | `bash scripts/verify-ci-toolchain.sh` | `hgm-infra/evidence/COM-2-output.log` |
| Completeness | THR-6 | COM-3 | Lint config schema-valid (no silent skip) | golangci-lint config is schema-valid under v2 | `golangci-lint config verify -c .golangci-v2.yml` | `hgm-infra/evidence/COM-3-output.log` |
| Completeness | THR-6 | COM-4 | Coverage threshold not diluted | Default threshold across languages remains ≥90% | `bash scripts/verify-coverage-threshold.sh` | `hgm-infra/evidence/COM-4-output.log` |
| Completeness | THR-5 | COM-6 | Logging/operational standards enforced (repo-scoped) | Logging/printing in library code is constrained; prohibited patterns are rejected | `bash hgm-infra/verifiers/hgm-verify-rubric.sh # (COM-6 check is built-in)` | `hgm-infra/evidence/COM-6-output.log` |
| Security | THR-2, THR-3, THR-6 | SEC-1 | Baseline SAST stays green | gosec stays green on first-party code | `bash scripts/sec-gosec.sh` | `hgm-infra/evidence/SEC-1-output.log` |
| Security | THR-6 | SEC-2 | Dependency vulnerability scan stays green | govulncheck + npm audit + pip-audit (when present) | `bash scripts/sec-dependency-scans.sh` | `hgm-infra/evidence/SEC-2-output.log` |
| Security | THR-6 | SEC-3 | Module integrity and checksum verification | Go module checksums verified | `go mod verify` | `hgm-infra/evidence/SEC-3-output.log` |
| Security | THR-5 | SEC-4 | Domain-specific P0 regression tests (fail closed) | `dynamorm:"encrypted"` semantics enforced in library behavior | `bash scripts/verify-encrypted-tag-implemented.sh` | `hgm-infra/evidence/SEC-4-output.log` |
| Maintainability | THR-6 | MAI-1 | File-size/complexity budgets enforced | File-size budgets prevent unreviewable “god files” | `bash scripts/verify-file-size.sh` | `hgm-infra/evidence/MAI-1-output.log` |
| Maintainability | THR-6 | MAI-2 | Maintainability roadmap current | Maintainability convergence plan is present and required sections stay current | `bash hgm-infra/verifiers/hgm-verify-rubric.sh # (MAI-2 check is built-in)` | `hgm-infra/evidence/MAI-2-output.log` |
| Maintainability | THR-6 | MAI-3 | Canonical implementations (no duplicate semantics) | One canonical Query implementation to prevent semantic drift | `bash scripts/verify-query-singleton.sh` | `hgm-infra/evidence/MAI-3-output.log` |
| Docs | THR-1, THR-2, THR-3, THR-6, THR-7 | DOC-5 | Threat model ↔ controls parity (no unmapped threats) | Threat IDs are stable and mapped to ≥1 control row | `bash hgm-infra/verifiers/hgm-verify-rubric.sh # (DOC-5 parity built-in)` | `hgm-infra/evidence/DOC-5-parity.log` |

> Add rows as needed for additional anti-drift controls (CI rubric enforcement, release automation integrity, maintainability convergence, etc).

## Framework Mapping (Optional; for PCI/HIPAA/SOC2)
If a compliance framework applies, keep standards text out of the repo; store only IDs + short titles and reference a KB
path/env var.

| Framework | Requirement ID | Requirement (short) | Status | Related Control IDs | Verification (command/gate) | Evidence (artifact/location) | Owner |
| --- | --- | --- | --- | --- | --- | --- | --- |
| (optional) | (id) | (title) | (status) | (control IDs) | (command) | (path) | (owner) |

## Notes
- Prefer deterministic verifiers (tests, static analysis, pinned build checks) over manual checklists.
- Treat this matrix as “source material”: the rubric/roadmap/evidence plan must stay consistent with Control IDs here.

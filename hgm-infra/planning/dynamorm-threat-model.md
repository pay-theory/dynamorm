# dynamorm Threat Model (custom — v0.1)

This document enumerates the highest-risk threats for the in-scope system and assigns stable IDs (`THR-*`) that must map
to controls in `hgm-infra/planning/dynamorm-controls-matrix.md`.

## Scope (must be explicit)
- **System:** Dynamorm (DynamoDB library and helpers; multi-language repo)
- **In-scope data:** PII, authentication/session tokens, secrets, and cardholder data (CHD) *by transitive usage* (data stored by consuming services)
- **Environments:** local dev, CI, staging, production (define “prod-like”: CI runs pinned toolchain + full rubric surface; integration tests use DynamoDB Local)
- **Third parties:** AWS (DynamoDB, STS, KMS, Lambda), GitHub Actions, npm registry, PyPI
- **Out of scope:** consuming service IAM policy design; application authn/authz; network perimeter; AWS account hardening
- **Assurance target:** audit-ready engineering evidence for a high-risk shared library

## Assets and Trust Boundaries (high level)
- **Primary assets:**
  - Correctness and integrity of DynamoDB items written/read via Dynamorm
  - Correctness of expression construction (Query/Scan/Update expressions)
  - Correctness of tag semantics (e.g., `pk`, `sk`, `attr:`, `encrypted`)
  - CI/verifier correctness (prevents “false green”)
- **Trust boundaries:**
  - Calling application boundary (untrusted/variable inputs)
  - Dynamorm public API boundary (must be stable and fail closed)
  - AWS SDK boundary (remote API behavior)
  - CI boundary (toolchain, pinned versions, evidence retention)
- **Entry points:**
  - Public Go API (root package + pkg/*)
  - Multi-account / Lambda helpers
  - Tag-driven marshaling/unmarshaling and expression construction

## Top Threats (stable IDs)
Threat IDs must be stable over time. When a new class of risk is discovered:
1) add a new `THR-*`,
2) add/adjust controls in the controls matrix,
3) update the rubric/roadmap if a new verifier is required.

| Threat ID | Title | What can go wrong | Primary controls (Control IDs) | Verification (gate) |
| --- | --- | --- | --- | --- |
| THR-1 | Data corruption / clobber via update semantics drift | Partial update paths overwrite attributes unexpectedly; divergent semantics across helpers | QUA-1, QUA-2, QUA-3 | `bash scripts/verify-unit-tests.sh`, `bash scripts/verify-integration-tests.sh`, `bash scripts/verify-coverage.sh` |
| THR-2 | Expression misuse / injection-by-construction | Unvalidated attribute names/paths produce incorrect expressions or broaden access patterns | QUA-1, QUA-2, QUA-3, SEC-1 | `bash scripts/verify-unit-tests.sh`, `bash scripts/verify-integration-tests.sh`, `bash scripts/sec-gosec.sh` |
| THR-3 | Unsafe reflection/unsafe operations lead to panics or memory safety hazards | Panic crashers or undefined behavior in marshaling/attribute conversion | QUA-1, QUA-3, SEC-1 | `bash scripts/verify-unit-tests.sh`, `bash scripts/verify-coverage.sh`, `bash scripts/sec-gosec.sh` |
| THR-4 | DoS / cost blowups via unbounded operations | Unbounded scans/queries/batches cause throttling or large spend | QUA-2 | `bash scripts/verify-integration-tests.sh` (plus targeted regression tests as needed) |
| THR-5 | Sensitive data exposure | Values that may include CHD/PII leak via logs/errors/examples; encryption tags become “paper security” | SEC-4 | `bash scripts/verify-encrypted-tag-implemented.sh` |
| THR-6 | Supply-chain and verifier drift | CI/tool versions drift or gates are weakened (excludes/threshold lowering) causing missed issues | COM-2, COM-3, COM-4, COM-5, SEC-2, SEC-3 | `bash scripts/verify-ci-toolchain.sh`, `golangci-lint config verify -c .golangci-v2.yml`, `bash scripts/verify-coverage-threshold.sh`, `bash scripts/sec-dependency-scans.sh`, `go mod verify` |
| THR-7 | Public API contract drift | Exported helpers diverge from canonical tag semantics; consumers make unsafe assumptions | CON-3 | `bash scripts/verify-public-api-contracts.sh` |

## Parity Rule (no “named threat without control”)
- Every `THR-*` listed above must appear at least once in the controls matrix “Threat IDs” column.
- The repo must have a deterministic parity check that fails if any threat is unmapped.

## Notes
- Prefer threats phrased as failure modes the repo can actually prevent or detect.
- Since Dynamorm is used in security-critical environments, “fail closed” behavior is preferred when configuration is missing (e.g., encryption tags without key material).

# DynamORM Controls Matrix (Quality + Security)

This controls matrix is the “requirements → controls → verifiers → evidence” backbone for DynamORM.
It is intentionally **engineering-focused** (not a compliance certification claim).

## Scope

- **System:** DynamORM (Go library for DynamoDB modeling/query/building expressions + example apps)
- **In-scope risks:** data loss/corruption via update semantics, expression misuse, unsafe reflection usage, resource/DoS risks, supply-chain drift
- **Out of scope:** IAM policy design, network perimeter controls, and application-layer auth (owned by consuming services)
- **Sensitive data note:** DynamORM may handle sensitive values (PII/tokens) as generic structs/attributes; it does not provide end-to-end encryption by default.

## Controls matrix

| Area | Control ID | Requirement | Control (what we implement) | Verification (tests/gates) | Evidence (where) |
| --- | --- | --- | --- | --- | --- |
| Quality | QUA-1 | Prevent regressions in core behavior | Unit tests for query/marshal/expr paths | `make test-unit` | CI logs + `coverage_lib.out` |
| Quality | QUA-2 | Prevent integration regressions | DynamoDB Local integration suite | `make integration` | CI logs |
| Quality | QUA-3 | Maintain baseline coverage | Coverage threshold + repeatable runner | `bash scripts/verify-coverage.sh` | `coverage_lib.out` artifact |
| Consistency | CON-1 | Reduce review noise | Enforce gofmt-clean diffs | `bash scripts/fmt-check.sh` | CI logs |
| Consistency | CON-2 | Enforce static analysis | Run golangci-lint with pinned CI toolchain | `make lint` | CI logs |
| Completeness | COM-1 | No “mystery meat” modules | All Go modules compile (including examples) | `bash scripts/verify-go-modules.sh` | CI logs |
| Completeness | COM-2 | No toolchain drift | CI Go version aligned to `go.mod` toolchain; pinned tool versions | `bash scripts/verify-ci-toolchain.sh` | CI logs |
| Docs | DOC-1 | Security posture is reviewable | Threat model exists and is maintained | `bash scripts/verify-planning-docs.sh` | `docs/development/planning/dynamorm-threat-model.md` |
| Docs | DOC-2 | Evidence is reproducible | Evidence plan exists and is maintained | `bash scripts/verify-planning-docs.sh` | `docs/development/planning/dynamorm-evidence-plan.md` |
| Security | SEC-1 | Baseline SAST | gosec scan is green on first-party code | `bash scripts/sec-gosec.sh` | SARIF/log output |
| Security | SEC-2 | Baseline dependency vuln scan | govulncheck is green | `bash scripts/sec-govulncheck.sh` | CLI output |
| Security | SEC-3 | Module integrity | Dependencies verified | `go mod verify` | CLI output |


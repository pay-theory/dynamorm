# DynamORM: 10/10 Rubric (Quality, Consistency, Completeness, Security, Docs)

This rubric defines what “10/10” means for DynamORM and how category grades are computed.
It is designed for an **AI-generated codebase**: gates must be **versioned, measurable, repeatable**, and resistant to
“green by drift” (lowered thresholds, excluded scopes, unpinned tools).

## Versioning (no moving goalposts)

- **Rubric version:** `v0.1` (2026-01-09)
- **Comparability rule:** grades are only comparable within the same rubric version.
- **Change rule:** rubric changes must bump the version and include a brief changelog entry (what changed + why).

### Changelog

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

---

## Quality (QUA) — reliable behavior with regression coverage

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| QUA-1 | 4 | Unit tests stay green | `make test-unit` |
| QUA-2 | 3 | Integration tests stay green (DynamoDB Local required) | `make integration` |
| QUA-3 | 3 | Library coverage stays at or above the threshold | `bash scripts/verify-coverage.sh` |

**10/10 definition:** QUA-1 through QUA-3 pass.

---

## Consistency (CON) — one way to do the important things

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| CON-1 | 4 | Go formatting is clean (no diffs) | `bash scripts/fmt-check.sh` |
| CON-2 | 6 | Lint stays green | `make lint` |

**10/10 definition:** CON-1 and CON-2 pass.

---

## Completeness (COM) — no drift, no mystery meat

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| COM-1 | 4 | All Go modules compile (including examples) | `bash scripts/verify-go-modules.sh` |
| COM-2 | 3 | CI toolchain aligns to repo expectations (Go + pinned tool versions) | `bash scripts/verify-ci-toolchain.sh` |
| COM-3 | 3 | Planning docs exist and are versioned | `bash scripts/verify-planning-docs.sh` |

**10/10 definition:** COM-1 through COM-3 pass.

---

## Security (SEC) — abuse-resilient and reviewable by default

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| SEC-1 | 4 | Static security scan stays green (first-party only) | `bash scripts/sec-gosec.sh` |
| SEC-2 | 4 | Dependency vulnerability scan stays green | `bash scripts/sec-govulncheck.sh` |
| SEC-3 | 2 | Supply-chain verification stays green | `go mod verify` |

**10/10 definition:** SEC-1 through SEC-3 pass.

---

## Docs (DOC) — threat model + evidence as code

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| DOC-1 | 4 | Threat model exists and is current | `bash scripts/verify-planning-docs.sh` |
| DOC-2 | 3 | Evidence plan exists and is reproducible | `bash scripts/verify-planning-docs.sh` |
| DOC-3 | 3 | Rubric + roadmap exist and are current | `bash scripts/verify-planning-docs.sh` |

**10/10 definition:** DOC-1 through DOC-3 pass.

---

## Recommended CI surface (keep grades stable)

```bash
bash scripts/verify-planning-docs.sh
bash scripts/fmt-check.sh
make lint

make test-unit
make integration
bash scripts/verify-coverage.sh

bash scripts/verify-go-modules.sh
bash scripts/verify-ci-toolchain.sh

bash scripts/sec-gosec.sh
bash scripts/sec-govulncheck.sh
go mod verify
```


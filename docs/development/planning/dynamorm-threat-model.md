# DynamORM Threat Model (Draft)

This is a living threat model for DynamORM. It is an engineering artifact to guide controls, tests, and evidence
generation; it is not a formal assessment or certification.

## Scope

- **System:** DynamORM (Go library for DynamoDB access patterns + example apps)
- **In-scope data:** whatever the consuming application persists in DynamoDB (often includes PII, tokens, secrets, or customer metadata)
- **Out of scope:** application authentication/authorization, IAM policy design, and environment hardening (owned by consuming services)
- **Important note:** the `dynamorm:"encrypted"` tag is currently **metadata only**; it does not provide encryption by itself.

## Assets (what we protect)

- Integrity of persisted data (no accidental overwrites, especially on partial updates)
- Correctness of DynamoDB expressions (no silent query broadening or unexpected filter behavior)
- AWS credentials and permissions (avoid patterns that encourage overbroad IAM usage)
- Reliability and resource safety (avoid unbounded reads/writes and retry storms)
- Supply chain integrity (dependencies and build toolchain)

## Trust boundaries (high level)

- **Calling application code** (trusted to provide correct models and validated inputs)
- **DynamORM library** (responsible for safe defaults, deterministic behavior, and clear docs)
- **AWS SDK / DynamoDB API** (remote dependency and policy enforcement point)
- **CI environment** (build/test/security toolchain)

## Top threats (initial list)

- **Data clobber via surprising update semantics:** empty-but-non-nil values overwriting stored attributes; mismatch between update APIs.
- **Expression misuse / injection-by-construction:** unvalidated attribute names or raw expression strings leading to broken queries or unintended access patterns.
- **Unsafe reflection hazards:** unsafe pointer math or reflect edge cases leading to panics, data corruption, or non-deterministic behavior.
- **DoS / cost blowups:** unbounded scans/queries, large batch operations, or aggressive retries causing throttling storms.
- **Sensitive data leakage:** user-provided values accidentally logged in examples/tests or surfaced in error strings.
- **Supply-chain compromise:** vulnerable dependencies or drift in security tooling causing missed findings.

## Mitigations (where we have controls today)

- Versioned planning rubric + CI gates: `docs/development/planning/dynamorm-10of10-rubric.md`
- Integration tests against DynamoDB Local: `make integration`
- Static analysis: `make lint`, `bash scripts/sec-gosec.sh`, `bash scripts/sec-govulncheck.sh`
- Resource limiting primitives: `pkg/protection/` (for consumers that choose to use them)

## Gaps / open questions

- Should DynamORM explicitly document and/or implement the `encrypted` tag semantics (or remove it)?
- Should we add fuzzing for expression building and marshaling edge cases?
- Where should “safe defaults” live for limits/retries (library vs application)?


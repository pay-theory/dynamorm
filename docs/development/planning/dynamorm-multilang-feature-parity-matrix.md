# DynamORM: Multi-language Feature Parity Matrix (Go / TypeScript / Python)

Goal: prevent “same name, different semantics” as we expand beyond Go. This document defines **what parity means** and
tracks **current parity status** across:

- Go (repo root) — current reference implementation
- TypeScript (`ts/`) — Node.js 24 / AWS SDK v3
- Python (`py/`) — Python 3.14 / boto3

Parity is not “APIs look similar”. Parity is **behavioral equivalence**, proven by shared fixtures + contract tests +
integration tests.

Related:

- Spec: `docs/development/planning/dynamorm-spec-dms-v0.1.md`
- Roadmap: `docs/development/planning/dynamorm-multilang-roadmap.md`
- Go↔TS matrix (historical): `docs/development/planning/dynamorm-go-ts-parity-matrix.md`
- Contract tests outline: `docs/development/planning/dynamorm-contract-tests-suite-outline.md`

## Parity tiers (behavioral)

- **P0 — Core parity:** schema + deterministic encoding + CRUD + conditional writes + typed errors (ship production services).
- **P1 — Query parity:** query/scan + index selection + pagination cursor rules (read-heavy patterns).
- **P2 — Multi-item parity:** batch get/write + transactions with explicit retry/partial failure semantics.
- **P3 — Streams parity:** stream image unmarshalling helpers (Lambda events).
- **P4 — Encryption parity:** `encrypted` semantics are real (KMS envelope, fail-closed, not queryable, not key/indexable).

## Snapshot (current)

This snapshot is intentionally blunt. “Partial” means there is known drift risk or missing contract tests.

| Language | P0 | P1 | P2 | P3 | P4 | Major known gaps |
| --- | --- | --- | --- | --- | --- | --- |
| Go | ✅ | ✅ | ✅ | ✅ | ✅ | (reference) |
| TypeScript (`ts/`) | ✅ | ✅ | ✅ | ✅ | ⚠️ | Encryption provider not KMS-based by default; needs KMS+AAD contract parity |
| Python (`py/`) | ⚠️ | ⚠️ | ✅ | ✅ | ✅ | Lifecycle automation missing; cursor format not contract-compatible with Go/TS |

## Parity matrix (features)

Legend:
- **Yes**: implemented with tests
- **Partial**: implemented but missing required behavior/tests for parity
- **No**: not implemented

| Area | Feature | Go | TypeScript | Python | Contract tests | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| Schema | PK/SK roles | Yes | Yes | Yes | No | Py uses dataclass metadata; TS uses `defineModel` schema |
| Schema | GSI/LSI definitions | Yes | Yes | Yes | No | All languages can declare indexes |
| Schema | Attribute naming determinism | Yes | Yes | Yes | No | DMS should be explicit-name first; avoid implicit drift |
| Encoding | `omitempty` emptiness rules | Yes | Yes | Yes | No | Must be pinned by fixtures (empty sets, falsey values) |
| Lifecycle | `created_at` / `updated_at` auto-populate | Yes | Yes | **No** | No | Py needs parity (or DMS must mark as “optional feature”) |
| Lifecycle | `version` optimistic locking | Yes | Yes | **No** | No | Py currently supports raw condition expressions but not automatic version semantics |
| Lifecycle | `ttl` epoch seconds | Yes | Yes | **No** | No | Py currently treats attributes as explicit; no TTL role semantics yet |
| CRUD | Put/Get/Update/Delete | Yes | Yes | Yes | No | All three work end-to-end against DynamoDB Local |
| Conditions | Conditional writes (if-not-exists / expressions) | Yes | Yes | Yes | No | TS has first-class flags; Py accepts raw expressions |
| Errors | Typed errors taxonomy | Yes | Yes | Yes | No | Needs parity mapping doc + contract tests for common AWS codes |
| Query | Query + key operators | Yes | Yes | Yes | No | Operators parity exists; must add cross-language fixtures |
| Query | Scan | Yes | Yes | Yes | No | Py/TS support basic scan + cursor |
| Pagination | Cursor encoding/decoding | Yes | Yes | **Partial** | **Partial** | TS cursor is contract-tested vs golden; Py cursor needs alignment to DMS cursor spec |
| Index | Index selection (table vs GSI/LSI) | Yes | Yes | Yes | No | Both TS and Py support index selection |
| Consistency | ConsistentRead rules | Yes | Yes | Yes | No | Must enforce “no consistent read on GSI” across languages |
| Batch | BatchGet + retry semantics | Yes | Yes | Yes | No | Needs explicit, shared “unprocessed” semantics fixtures |
| Batch | BatchWrite + retry semantics | Yes | Yes | Yes | No | Same as above |
| Tx | TransactWrite + error surfacing | Yes | Yes | Yes | No | TS/Py need parity on condition failures vs mixed cancellations |
| Streams | Unmarshal stream image | Yes | Yes | Yes | No | Py/TS implement Lambda stream helpers; add fixtures for map/list/binary |
| Encryption | Envelope format (`v`,`edk`,`nonce`,`ct`) | Yes | Yes | Yes | No | All languages use envelope map shape; KMS/AAD parity must be proven |
| Encryption | Fail-closed when unconfigured | Yes | Yes | Yes | No | TS requires encryption provider; Py requires `kms_key_arn` |
| Encryption | AAD binding to attribute name | Yes | **Partial** | Yes | No | TS provider is user-defined; provide official KMS provider + contract tests |
| Infra | Table create/migrate helpers | Yes | No | No | No | Keep “runtime only” unless we decide otherwise |

## What “parity complete” means (acceptance criteria)

A feature is “at parity” only when:

1) It is defined in DMS (or explicitly marked as “not in DMS / intentionally language-specific”), and
2) It passes:
   - unit tests (pure logic), and
   - DynamoDB Local-backed integration tests (end-to-end), and
   - shared contract tests (golden fixtures where drift risk is high: cursor, encryption envelope, reserved words), and
3) It is wired into the rubric with “no green-by-exclusion” (pinned tooling, strict defaults).

## Highest-risk drift points (prioritize next)

- **Cursor compatibility:** Py must adopt the canonical cursor format used by Go/TS (or DMS must standardize a new one).
- **Lifecycle parity:** decide whether lifecycle roles are required in all languages; if yes, implement in Py with tests.
- **Encryption parity:** publish an official KMS-based TS `EncryptionProvider` that matches Go/Py envelope+AAD rules and is
  verified by fixtures.
- **Mocks/testkit parity:** every language should ship a public, supported mocking surface for DynamoDB + KMS to make
  application testing cheap and consistent.


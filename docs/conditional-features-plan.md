# DynamORM Conditional & Transaction Enhancements Plan

This phased plan covers the three requested capabilities:

1. First-class conditional create/update/delete helpers  
2. A composable `TransactWriteItems` utility for multi-operation workflows  
3. A native `BatchGetItem` wrapper that handles up to 100 PK/SK pairs with retries

Each phase builds on the previous one so we can ship incremental value while protecting ABI stability.

---

## Phase 0 – Discovery & API Alignment ✅ COMPLETE
**Duration:** 1–2 days  
**Status:** ✅ Complete (November 9, 2025)  
**Goal:** lock down public API changes and success metrics.

- ✅ Audit existing hooks (`UpdateBuilder` conditions, transaction package) to understand reuse points.
- ✅ Draft API sketches (method names, fluent patterns, error semantics) and circulate for review.
- ✅ Define acceptance tests for each capability (conditional write happy/negative paths, transactional dual-write, batch get retries).
- ✅ Output: design notes + green light to proceed.

**Dependencies:** none  
**Deliverables:** 
- ✅ Comprehensive design brief: `/docs/phase-0-design-brief.md` (700+ lines)
- ✅ Executive summary: `/docs/phase-0-summary.md`
- ✅ 35+ acceptance test scenarios documented
- ✅ API designs with code examples for all three capabilities

**Key Findings:**
- `internal/expr.Builder` is reusable for all conditional operations
- Transaction package has solid foundation, needs fluent builder layer
- Batch operations need chunking, parallel execution, and better retry logic

**Next:** Proceed to Phase 1 upon team approval of API designs.

---

## Phase 1 – Conditional Create/Update/Delete Helpers
**Duration:** 1 sprint  
**Goal:** allow repositories to express DynamoDB conditions without dropping to the SDK.

Work items:
- Extend `core.Query` interface (and implementations) with fluent helpers such as `IfNotExists()`, `Condition(field, op, value)`, `ConditionExists`, mirroring the existing `UpdateBuilder` condition set.
- Teach `pkg/query/query.go` to translate those helpers into expression components via `internal/expr.Builder`.
- Update the Put/Update/Delete execution paths in `pkg/query/executor.go` so condition expressions flow into `dynamodb.PutItemInput`, `UpdateItemInput`, etc.
- Add unit tests that cover:
  - Create with `attribute_not_exists` guard (positive + failure cases).
  - Conditional update toggling a field (e.g., `Locked` true→false).
  - Error propagation when DynamoDB returns `ConditionalCheckFailedException`.
- Update mocks (`pkg/mocks`) and testing helpers if new methods are exposed.

**Dependencies:** Phase 0 design  
**Deliverables:** merged feature + README snippet showing usage.

---

## Phase 2 – TransactWriteItems Utility
**Duration:** 1 sprint  
**Goal:** expose a high-level API for composing conditioned puts/updates/deletes atomically.

Work items:
- Build a `transaction.Builder` (or similar) that reuses `pkg/transaction/transaction.go` marshaling but exposes a fluent DSL (e.g., `tx.Put(model, dynamorm.IfNotExists())`).
- Extend `core.ExtendedDB` with `TransactWrite(ctx, func(tx dynamorm.TransactionBuilder) error)` so repositories can describe multi-item operations without touching `types.TransactWriteItem`.
- Support conditional expressions per entry (leveraging Phase 1 helpers) and propagate cancellation reasons back to callers.
- Add retry logic for `TransactionCanceledException` when safe, respecting idempotency tokens.
- Tests:
  - Unit tests mocking DynamoDB client interactions.
  - Integration test covering the bookmark dual-write scenario (two conditioned operations committed atomically).

**Dependencies:** Phases 0 & 1  
**Deliverables:** new API, docs, integration test.

---

## Phase 3 – BatchGetItem Wrapper
**Duration:** < 1 sprint  
**Goal:** provide an ergonomic, retry-aware batch read surface.

Work items:
- Introduce a `KeyPair` helper (PK/SK) plus `BatchGet(keys []KeyPair, dest any)` on `core.Query` or `core.DB`.
- Implement the DynamoDB `BatchGetItem` call inside `pkg/query/executor.go`, respecting 100-item limit and splitting larger requests.
- Handle `UnprocessedKeys` with exponential backoff and jitter; expose metrics/logging hooks.
- Ensure results are unmarshaled via existing `UnmarshalItems`, preserving order if needed.
- Tests:
  - Unit tests verifying request chunking, retry behavior, and unmarshaling.
  - (Optional) integration test against DynamoDB Local covering mixed hits/misses.

**Dependencies:** Phase 0 (API alignment)  
**Deliverables:** batch get helper + documentation/examples.

---

## Phase 4 – Documentation, Samples, and Rollout
**Duration:** 2–3 days  
**Goal:** make the new capabilities discoverable and production-ready.

Tasks:
- Update `README.md` and docs in `docs/archive/` with new canonical examples (conditional CRUD, transaction DSL, batch get usage).
- Add example code under `examples/` (e.g., a bookmark service demonstrating dual-write transaction).
- Refresh testing docs to mention new helpers and how to mock them.
- Run full test suite (unit + integration + stress where relevant) and capture results.
- Prepare changelog entries and release notes outlining migration steps (e.g., new methods on interfaces).

**Dependencies:** Phases 1–3 completed  
**Deliverables:** documentation updates, release checklist, tagged release candidate.

---

## Leadership & Coordination
- **Project Lead (you):** own roadmap, review PRs, and guide contributing agents through each phase.
- **Implementation Agent(s):** pick up work items per phase under lead’s direction.
- **QA/Docs:** ensure coverage in Phase 4 before release.

Weekly checkpoints should confirm:
1. Phase milestones met.
2. Interface stability (no breaking changes without notice).
3. Tests/documentation updated alongside code.


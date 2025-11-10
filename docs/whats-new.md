# What's New: Conditional Helpers, Transactions, BatchGet

Phase 4 focuses on making the recently landed APIs easy to find, understand, and ship. Use this page as the high-level release notes for docs, examples, and validation work surrounding the three flagship capabilities.

## Highlights

- **Conditional helpers everywhere:** `IfNotExists()`, `IfExists()`, `WithCondition(...)`, and `WithConditionExpression(...)` now have dedicated guidance in `README.md`, the troubleshooting guide, and the struct definition reference so teams know exactly when to guard writes and how to handle `customerrors.ErrConditionFailed`.
- **Fluent transaction builder:** The DSL exposed via `db.Transact()` / `TransactWrite()` is showcased with dual-write examples, error handling for `customerrors.TransactionError`, and context propagation tips. No more spelunking through `types.TransactWriteItem`.
- **Retry-aware BatchGet:** Chunking, bounded parallelism, `core.RetryPolicy`, progress callbacks, and the `dynamorm.NewKeyPair` helper all have fresh snippets plus a new example that demonstrates the builder in action.

## Documentation Touchpoints

| Capability | Updated Docs |
|------------|--------------|
| Conditional helpers | `README.md` → “Pattern: Conditional Writes”, `docs/archive/troubleshooting.md`, `docs/archive/struct-definition-guide.md` |
| Transaction builder | `README.md` → “Pattern: Fluent Transaction Builder”, troubleshooting guide (TransactionError usage), `docs/phase-0-summary.md` |
| BatchGet enhancements | `README.md` → “Pattern: Batch Get”, struct guide (KeyPair), plan/summary docs noting delivery |

## Example Coverage

- `examples/feature_spotlight.go` (new) demonstrates:
  1. Conditional create/update/delete flows with `ErrConditionFailed`.
  2. Fluent transaction builder with quota checks plus `TransactWrite`.
  3. BatchGet builder using custom `core.RetryPolicy`, progress callbacks, and chunk-level error hooks.

Run `go test ./examples/...` to ensure the snippets compile whenever you touch them.

## Release Checklist Callouts

1. **README badges/links** – verified current.
2. **CHANGELOG** – “Unreleased” section references the new docs/examples.
3. **Tests** – `make test-unit` + `go test ./examples/...` capture the minimum bar; append integration suites touching BatchGet/transactions when time allows.
4. **TODO sweep** – no new TODOs were added during Phase 4; existing ones (e.g., UpdateBuilder OR support) remain tracked separately.

Refer back to this page before tagging the release to ensure all three capabilities stay front-and-center in our public surface.

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

- _Nothing yet._

## [1.0.37] - 2025-11-11

### Added
- First-class conditional write helpers on `core.Query`: `IfNotExists()`, `IfExists()`, `WithCondition()`, and `WithConditionExpression()` make it trivial to express DynamoDB condition checks without dropping to the raw SDK.
- Documentation now includes canonical examples for conditional creates, updates, and deletes along with guidance on handling `ErrConditionFailed`.
- `docs/whats-new.md` plus new `examples/feature_spotlight.go` snippets illustrate conditional helpers, the fluent transaction builder, and the BatchGet builder with custom retry policies.
- Fluent transaction builder via `db.Transact()` plus the `core.TransactionBuilder` interface, including a context-aware `TransactWrite` helper, per-operation condition helpers (`dynamorm.Condition`, `dynamorm.IfNotExists`, etc.), and detailed `TransactionError` reporting with automatic retries for transient cancellation reasons.
- Retry-aware batch read API: `BatchGetWithOptions`, `BatchGetBuilder`, and the new `dynamorm.NewKeyPair` helper support automatic chunking, exponential backoff with jitter, progress callbacks, and bounded parallelism.

### Changed
- Create/Update/Delete paths in both the high-level `dynamorm` package and the modular `pkg/query` builder now share a common expression compiler, allowing query-level conditions and advanced expressions to flow through every write operation.
- `pkg/query` executors translate DynamoDB `ConditionalCheckFailedException` responses into `customerrors.ErrConditionFailed`, enabling consistent conflict handling via `errors.Is`.
- `BatchExecutor.ExecuteBatchGet` now returns the raw DynamoDB items after retrying `UnprocessedKeys`, and top-level `BatchGet` delegates to the shared chunking engine to preserve ordering guarantees.

### Fixed
- `db.Model(...).Create()` no longer injects an implicit `attribute_not_exists` guard; callers opt in via `IfNotExists()` just like `pkg/query`, preserving the documented overwrite semantics.
- Passing `WithRetry(nil)` (or a `BatchGetOptions` with `RetryPolicy: nil`) now disables BatchGet retries as intended, instead of silently substituting the default retry policy.

## [1.0.36] - 2025-11-09

### Fixed
- Removed verbose debug logging from `Model.Update()` and the custom converter lookup so production logs stay clean without changing behavior.

## [1.0.35] - 2025-10-31

### Fixed
- Nested structs flagged with `dynamorm:"json"` now apply the active naming convention (camelCase or snake_case) before honoring explicit `json` tags, keeping attribute names consistent at every level.

## [1.0.34] - 2025-10-29

### Fixed
- **[CRITICAL]** Custom converters now properly invoked during `Update()` operations
  - Security validation was rejecting custom struct types before converter check
  - Fixed by checking for custom converters BEFORE security validation
  - Custom types with registered converters now bypass security validation (converters handle their own validation)
  - Removed silent NULL fallbacks - validation/conversion failures now panic with clear error messages
- Field name validation in `Update()` - unknown field names now return clear error messages instead of silently skipping

## [1.0.33] - 2025-10-28

### Added
- Support for legacy snake_case naming convention alongside default camelCase:
  - New `naming:snake_case` struct tag to opt-in to snake_case attribute names
  - Automatic conversion of Go field names to snake_case (e.g., `FirstName` → `first_name`)
  - Smart acronym handling in snake_case conversion (e.g., `UserID` → `user_id`, `URLValue` → `url_value`)
  - Per-model naming convention detection and validation
  - Both naming conventions can coexist in the same application
  - Integration tests demonstrating mixed convention usage
- `OrCondition` method to UpdateBuilder for OR logic in conditional expressions:
  - Enables complex business rules like rate limiting with privilege checks
  - Supports mixing AND/OR conditions with left-to-right evaluation
  - Works with all condition types including attribute existence checks
  - Particularly useful for scenarios like "allow if under limit OR premium user OR whitelisted"
- Full implementation of core DynamoDB operations that were previously stubs:
  - `ExecuteQuery` and `ExecuteScan` with complete pagination, filtering, and projection support
  - `ExecuteQueryWithPagination` and `ExecuteScanWithPagination` for paginated results with metadata
  - `ExecuteBatchGet` and `ExecuteBatchWrite` with automatic retry logic for unprocessed items
  - Helper functions for unmarshaling DynamoDB items to Go structs
- Core API methods to the Query interface:
  - `BatchDelete` - Delete multiple items by their keys with support for various key formats
  - `BatchWrite` - Mixed batch operations supporting both puts and deletes in a single request
  - `BatchUpdateWithOptions` - Batch update operations with customizable options
- Fully functional `UpdateBuilder` implementation with fluent API:
  - Support for Set, Add, Remove operations
  - List manipulation methods (AppendToList, PrependToList, RemoveFromListAt, SetListElement)
  - Conditional update support with ConditionExists, ConditionNotExists, ConditionVersion
  - ReturnValues option support
- `CreateOrUpdate()` method for upsert operations - creates a new item or overwrites an existing one
- Improved error messages for `Create()` when attempting to create an item with duplicate keys

### Changed
- `UpdateBuilder()` method now returns a functional builder instead of nil
- Improved error messages to follow Go conventions (lowercase)

### Fixed
- **[CRITICAL BUG FIX]** Custom type converters registered via `RegisterTypeConverter()` are now properly invoked during `Update()` operations
  - Previously, custom converters only worked during `Create()` operations but were silently ignored during `Update()`, causing incorrect data storage (NULL values or nested struct representations instead of custom format)
  - The expression builder now receives and uses the converter lookup, ensuring consistent behavior across all operations
  - This fix affects: `Update()`, `UpdateBuilder()`, filter conditions, and all query/scan operations
  - **Breaking Change Impact**: None - this only fixes broken functionality
  - **Migration**: Code using custom converters with `Update()` will now work correctly without changes
  - Added comprehensive test suite (`dynamorm_custom_converter_update_test.go`) to prevent regression
- Circular dependencies between core and query packages
- Interface signature mismatches for `BatchUpdateWithOptions` across packages
- Missing mock implementations for `BatchWrite` and `BatchUpdateWithOptions` in test helpers
- Stress test compilation error by properly creating DynamoDB client from config
- Batch operations test to use the correct interface signatures
- All staticcheck warnings including:
  - Removed unused types (`executor`, `metadataAdapter`, `filter`)
  - Fixed error string capitalization
  - Removed unnecessary blank identifier assignments
- Unmarshal error when using `All()` with slice of pointers (e.g., `[]*Model`)
- UpdateBuilder overlapping document paths error when using multiple `SetIfNotExists` operations

### Removed
- Unused `executor` type and methods from dynamorm.go (functionality exists elsewhere)
- Unused `metadataAdapter` type and methods 
- Unused `filter` struct definition

## [1.0.9] - 2025-01-02

### Added
- Significant performance improvements achieving near-parity with AWS SDK
- Comprehensive documentation updates

### Changed
- Primary key recognition now properly uses `GetItem` for single lookups instead of `Query`
- API refinements for consistency:
  - `Where()` consistently uses 3 parameters: `(field, operator, value)`
  - Replaced `Find()` with `All()` for retrieving multiple results
  - `First()` now requires destination parameter

### Fixed
- Fixed primary key recognition for DynamoDB attribute names vs Go field names
- Resolved index query compilation issues
- Corrected field mapping for models with custom attribute names
- Memory usage reduced by 77% (from 179KB to 42KB per operation)
- Allocations reduced by 77% (from 2,416 to 566 per operation)

### Performance
- Single lookup operations: ~5x faster (from 2.5ms to 0.52ms)
- Now only 1.01x slower than raw AWS SDK (essentially negligible)

## [1.0.3] - 2024-12-20

## [1.0.2] - 2024-01-XX

### Added
- Pre-built mock implementations in `pkg/mocks` package
  - `MockDB` - implements `core.DB` interface
  - `MockQuery` - implements all 26+ methods of `core.Query` interface
  - `MockUpdateBuilder` - implements `core.UpdateBuilder` interface
- Comprehensive mocking documentation and examples
- Interface segregation proposal for future improvements

### Fixed
- Teams no longer need to implement all Query interface methods manually for testing
- Eliminates "trial and error" discovery of missing mock methods

## [1.0.1] - 2025-06-10

### Added
- Interface-based design for improved testability
  - New `core.DB` interface for basic operations
  - New `core.ExtendedDB` interface for full functionality
  - `NewBasic()` function that returns `core.DB` for simpler use cases
- Comprehensive testing documentation and examples
- Mock implementation examples for unit testing
- Runtime type checking for interface methods accepting `any` type

### Changed
- **BREAKING**: `dynamorm.New()` now returns `core.ExtendedDB` interface instead of `*dynamorm.DB`
- All methods that accept specific option types now accept `...any` with runtime validation
- Updated all examples and tests to use interfaces
- Improved separation between core operations and schema management

### Fixed
- Lambda.go now properly handles interface types
- Transaction callbacks properly use type assertions
- All test helper functions updated to return interfaces

### Migration Guide
See [Release Notes v1.0.1](docs/releases/v1.0.1-interface-improvements.md) for detailed migration instructions.

## [0.1.1] - 2025-06-10

### Added
- Lambda-native optimizations with 11ms cold starts (91% faster than standard SDK)
- Type-safe ORM interface for DynamoDB operations
- Multi-account support with automatic credential management
- Smart query optimization and automatic index selection
- Comprehensive struct tag system for model configuration
- Built-in support for transactions and batch operations
- Automatic connection pooling and retry logic
- Expression builder for complex queries
- Schema migration and validation tools
- Comprehensive test suite with 85%+ coverage

### Changed
- Restructured documentation into organized categories
- Improved error handling with context-aware messages
- Enhanced performance monitoring and metrics

### Fixed
- Connection reuse in Lambda environments
- Memory optimization for large batch operations
- Proper handling of DynamoDB limits

## [0.1.0] - 2025-06-10

### Added
- Initial release of DynamORM
- Basic CRUD operations
- Query and scan functionality
- Transaction support
- Batch operations
- Index management
- Expression builder
- Basic documentation

[Unreleased]: https://github.com/dynamorm/dynamorm/compare/v1.0.36...HEAD
[1.0.36]: https://github.com/dynamorm/dynamorm/compare/v1.0.35...v1.0.36
[1.0.35]: https://github.com/dynamorm/dynamorm/compare/v1.0.34...v1.0.35
[1.0.9]: https://github.com/dynamorm/dynamorm/compare/v1.0.3...v1.0.9
[1.0.3]: https://github.com/dynamorm/dynamorm/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/dynamorm/dynamorm/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/dynamorm/dynamorm/compare/v0.1.1...v1.0.1
[0.1.1]: https://github.com/dynamorm/dynamorm/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/dynamorm/dynamorm/releases/tag/v0.1.0 

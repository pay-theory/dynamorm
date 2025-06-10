# DynamORM Implementation Checklist

This checklist provides a methodical approach to implementing the remaining features in DynamORM, organized by priority and dependencies.

## Phase 1: Core Infrastructure (Foundation)

### 1.1 Implement UpdateItemExecutor
- [x] Create concrete implementation of `UpdateItemExecutor` interface
  - Location: Create new file `pkg/core/executor_update.go`
  - Interface already defined in `pkg/query/query.go:84-86`
  - Method signature: `ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error`
- [x] Integrate with DynamoDB client for actual update operations
- [x] Handle error cases and conditional checks
- [x] Add unit tests for the executor

### 1.2 Extend Main Executor
- [x] Ensure main executor implements `UpdateItemExecutor` interface
- [x] Add type assertion checks in query execution paths
- [x] Update executor factory/initialization

## Phase 2: Update Operations

### 2.1 Complete UpdateBuilder Integration
- [x] Fix TODO in `dynamorm.go:1799`
- [x] Connect `query.UpdateBuilder()` to return proper implementation
- [x] Ensure UpdateBuilder can access query context and metadata
- [x] Test integration with existing query chains

### 2.2 Implement Return Values Support
- [x] Fix TODO in `pkg/query/update_builder.go:224`
- [x] Modify `ExecuteUpdateItem` to support return values
- [x] Implement `ExecuteWithResult` properly to unmarshal returned values
- [x] Support all DynamoDB return value options:
  - [x] NONE
  - [x] ALL_OLD
  - [x] UPDATED_OLD
  - [x] ALL_NEW
  - [x] UPDATED_NEW

### 2.3 Add Comprehensive Update Tests
- [x] Test all update operations (Set, Add, Remove, Delete)
- [x] Test list operations (Append, Prepend, RemoveAt, SetElement)
- [x] Test conditional updates
- [x] Test optimistic locking with version fields
- [x] Test return values functionality

## Phase 3: Batch Operations

### 3.1 Implement Batch Delete Executor
- [x] Fix TODO in `pkg/query/batch_operations.go:319-320`
- [x] Implement `BatchWriteItem` support in executor
- [x] Handle unprocessed items and retries
- [x] Respect batch size limits (25 items max)

### 3.2 Add Batch Write Support
- [x] Create executor method for batch write operations
- [x] Support mixed batch operations (puts and deletes)
- [x] Implement proper error handling for partial failures
- [x] Add retry logic for unprocessed items

### 3.3 Complete Batch Tests
- [x] Remove "not implemented" returns in `pkg/query/batch_operations_test.go`
- [x] Add integration tests for batch operations
- [x] Test batch size limits and pagination
- [x] Test error handling and retries

## Phase 4: Advanced Query Features

### 4.1 Implement GroupBy
- [x] Complete implementation in `pkg/query/aggregates.go`
- [x] Design aggregation result structure
- [x] Support common aggregation functions:
  - [x] COUNT
  - [x] SUM
  - [x] AVG
  - [x] MIN
  - [x] MAX
- [x] Handle grouped results properly

### 4.2 Implement Having Clause
- [x] Fix comment in `pkg/query/aggregates_test.go`
- [x] Implement Having in conjunction with GroupBy
- [x] Support filtering on aggregated values
- [x] Add proper validation for Having conditions

### 4.3 Add Cursor-based Pagination
- [x] Implement as mentioned in `tests/integration/query_integration_test.go:229`
- [x] Create cursor encoding/decoding functions
- [x] Support forward and backward pagination
- [x] Integrate with existing `Cursor()` and `SetCursor()` methods
- [x] Handle exclusive start keys properly

### 4.4 Refactor Segment Query
- [x] Fix TODO in `dynamorm.go:1849`
- [x] Properly use segmentQuery throughout parallel scan
- [x] Ensure all query properties are preserved in segments
- [x] Test parallel scan with complex queries

## Phase 5: Schema Enhancements

### 5.1 Implement Transform Function
- [x] Complete transform function in auto-migration
- [x] Support data transformation during migration
- [x] Handle type conversions and data mapping
- [x] Add validation for transform operations

### 5.2 Add Migration Tests
- [x] Test transform function with various data types
- [x] Test migration rollback scenarios
- [x] Test large-scale migrations with batching
- [x] Verify data integrity after migration

## Testing Strategy

### Integration Tests
- [ ] Create comprehensive integration test suite
- [ ] Test all new features against DynamoDB Local
- [ ] Add performance benchmarks for batch operations
- [ ] Test error scenarios and edge cases

### Documentation
- [ ] Update API documentation for new features
- [ ] Add examples for each new operation
- [ ] Update README with feature completion status
- [ ] Create migration guide for transform functions

## Implementation Order Recommendations

1. **Start with Phase 1**: Core infrastructure is needed for other features
2. **Complete Phase 2**: Update operations are fundamental
3. **Move to Phase 3**: Batch operations build on update infrastructure
4. **Implement Phase 4**: Advanced features can be done in parallel
5. **Finish with Phase 5**: Schema enhancements are less critical

## Notes

- Each item should be implemented with proper error handling
- All features must maintain backward compatibility
- Performance implications should be considered for batch operations
- Follow existing code patterns and conventions in the codebase
- Add appropriate logging for debugging and monitoring

## Progress Tracking

Use this checklist to track implementation progress. Check off items as they are completed and add notes about any challenges or decisions made during implementation. 
# Query Interface Segregation Proposal

## Problem Statement

The current `core.Query` interface contains 26+ methods, making it difficult to:
- Create mock implementations
- Understand which methods are actually needed for specific use cases
- Implement custom query builders
- Test code that only uses a subset of query functionality

## Current Interface Size

The `core.Query` interface currently includes:
```go
// Basic query building (7 methods)
Where, Index, Filter, OrFilter, FilterGroup, OrFilterGroup, OrderBy

// Result limiting and pagination (4 methods)
Limit, Offset, Cursor, SetCursor

// Field selection (1 method)
Select

// Query execution (5 methods)
First, All, AllPaginated, Count, Scan

// Parallel scanning (2 methods)
ParallelScan, ScanAllSegments

// Batch operations (2 methods)
BatchGet, BatchCreate

// Write operations (4 methods)
Create, Update, UpdateBuilder, Delete

// Context management (1 method)
WithContext
```

Total: 26 methods that must all be implemented.

## Proposed Solution: Interface Segregation

Break down the large interface into smaller, focused interfaces based on common usage patterns:

### 1. Query Builder Interface
```go
// QueryBuilder handles query construction
type QueryBuilder interface {
    Where(field string, op string, value any) QueryBuilder
    Index(indexName string) QueryBuilder
    Filter(field string, op string, value any) QueryBuilder
    OrderBy(field string, order string) QueryBuilder
    Limit(limit int) QueryBuilder
    Select(fields ...string) QueryBuilder
}
```

### 2. Query Executor Interface
```go
// QueryExecutor handles query execution
type QueryExecutor interface {
    First(dest any) error
    All(dest any) error
    Count() (int64, error)
}
```

### 3. Write Operations Interface
```go
// WriteOperations handles CRUD operations
type WriteOperations interface {
    Create() error
    Update(fields ...string) error
    Delete() error
}
```

### 4. Advanced Query Interface
```go
// AdvancedQuery combines builder and executor with additional features
type AdvancedQuery interface {
    QueryBuilder
    QueryExecutor
    
    // Advanced filtering
    OrFilter(field string, op string, value any) AdvancedQuery
    FilterGroup(func(QueryBuilder)) AdvancedQuery
    OrFilterGroup(func(QueryBuilder)) AdvancedQuery
    
    // Pagination
    Offset(offset int) AdvancedQuery
    Cursor(cursor string) AdvancedQuery
    AllPaginated(dest any) (*PaginatedResult, error)
}
```

### 5. Batch Operations Interface
```go
// BatchOperations handles batch operations
type BatchOperations interface {
    BatchGet(keys []any, dest any) error
    BatchCreate(items any) error
}
```

### 6. Scan Operations Interface
```go
// ScanOperations handles table scanning
type ScanOperations interface {
    Scan(dest any) error
    ParallelScan(segment int32, totalSegments int32) ScanOperations
    ScanAllSegments(dest any, totalSegments int32) error
}
```

## Benefits

1. **Easier Testing**: Mock only the interfaces you actually use
2. **Better Documentation**: Clear separation of concerns
3. **Flexibility**: Implement only what you need
4. **Gradual Migration**: Can be introduced alongside existing interface

## Implementation Strategy

### Phase 1: Add New Interfaces (Non-Breaking)
```go
// Keep existing Query interface
type Query interface {
    // ... all 26 methods
}

// Add new segregated interfaces
type QueryBuilder interface { /* ... */ }
type QueryExecutor interface { /* ... */ }
// etc.

// Make Query extend all segregated interfaces
type Query interface {
    QueryBuilder
    QueryExecutor
    WriteOperations
    // ... etc
}
```

### Phase 2: Update Documentation
- Show examples using segregated interfaces
- Recommend segregated interfaces for new code
- Provide migration guide

### Phase 3: Provide Adapters
```go
// Adapter to use only QueryBuilder + QueryExecutor
type BasicQuery struct {
    QueryBuilder
    QueryExecutor
}

// Helper to create BasicQuery from full Query
func AsBasicQuery(q Query) BasicQuery {
    return BasicQuery{
        QueryBuilder: q,
        QueryExecutor: q,
    }
}
```

## Example Usage

### Before (Current Approach)
```go
func QueryUsers(db core.DB) ([]User, error) {
    // Must accept full Query interface even though
    // we only use Where, OrderBy, and All
    var users []User
    err := db.Model(&User{}).
        Where("Status", "=", "active").
        OrderBy("CreatedAt", "DESC").
        All(&users)
    return users, err
}
```

### After (With Interface Segregation)
```go
func QueryUsers(db core.DB) ([]User, error) {
    // Can work with just the interfaces we need
    query := db.Model(&User{}).(interface {
        core.QueryBuilder
        core.QueryExecutor
    })
    
    var users []User
    err := query.
        Where("Status", "=", "active").
        OrderBy("CreatedAt", "DESC").
        All(&users)
    return users, err
}
```

### Testing Becomes Simpler
```go
// Only need to mock the methods we actually use
type MockBasicQuery struct {
    mock.Mock
}

func (m *MockBasicQuery) Where(field, op string, value any) core.QueryBuilder {
    args := m.Called(field, op, value)
    return args.Get(0).(core.QueryBuilder)
}

func (m *MockBasicQuery) OrderBy(field, order string) core.QueryBuilder {
    args := m.Called(field, order)
    return args.Get(0).(core.QueryBuilder)
}

func (m *MockBasicQuery) All(dest any) error {
    args := m.Called(dest)
    return args.Error(0)
}

// No need to implement the other 23 methods!
```

## Considerations

1. **Backward Compatibility**: The existing `Query` interface remains unchanged
2. **Type Assertions**: May require type assertions in some cases
3. **Learning Curve**: Developers need to understand which interfaces to use

## Next Steps

1. Gather feedback from the community
2. Create proof-of-concept implementation
3. Test with real-world use cases
4. Plan phased rollout

## Alternative Solutions

### 1. Query Options Pattern
Instead of method chaining, use options:
```go
results, err := db.Query(&User{}, 
    query.Where("Status", "=", "active"),
    query.OrderBy("CreatedAt", "DESC"),
    query.Limit(10),
)
```

### 2. Builder Pattern with Terminal Methods
Separate query building from execution:
```go
q := db.NewQuery(&User{}).
    Where("Status", "=", "active").
    Build()

users, err := db.Execute(q).All(&users)
```

### 3. Keep Large Interface but Provide Partial Mocks
Create a base mock that panics on unimplemented methods:
```go
type PartialMockQuery struct {
    *mocks.PanicQuery // Panics on any method call
    // Override only what you need
}
```

## Conclusion

Interface segregation would make DynamORM more testable and easier to understand, while maintaining backward compatibility. The phased approach allows for gradual adoption without breaking existing code. 
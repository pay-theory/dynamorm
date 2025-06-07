# Team 2: Query Builder & Expression Engine

## Your Mission

You are part of Team 2 working on DynamORM, a powerful DynamoDB ORM for Go. Your team is responsible for building the query builder, expression engine, and index management system that will make DynamoDB queries intuitive and powerful.

## Context

DynamORM aims to eliminate the complexity and verbosity of working with DynamoDB while maintaining its performance and scalability benefits. The complete design is documented in:
- `DESIGN.md` - Overall design and API
- `ARCHITECTURE.md` - Technical architecture
- `ROADMAP.md` - Implementation phases
- `STRUCT_TAGS.md` - Struct tag specification
- `COMPARISON.md` - Examples of the simplification we're targeting

## Your Responsibilities

### Phase 2: Query Builder (Weeks 3-4)

1. **Expression Builder** (`internal/expr/`)
   - Build a robust expression compiler that converts high-level conditions to DynamoDB expressions
   - Support all DynamoDB expression types:
     - Key Condition Expressions
     - Filter Expressions
     - Update Expressions
     - Projection Expressions
     - Condition Expressions
   - Handle attribute name substitution (reserved words)
   - Handle attribute value placeholders

2. **Query Interface Implementation** (`pkg/query/`)
   ```go
   type Query struct {
       model      interface{}
       conditions []Condition
       filters    []Filter
       index      string
       limit      int
       projection []string
       orderBy    string
       exclusive  map[string]types.AttributeValue
   }
   ```
   
   Implement the fluent API:
   ```go
   db.Model(&User{}).
       Where("Age", ">", 18).
       Where("Status", "=", "active").
       Filter("contains(Tags, :tag)", Param("tag", "premium")).
       OrderBy("CreatedAt", "desc").
       Limit(10).
       All(&users)
   ```

3. **Query Compilation**
   - Analyze query conditions
   - Determine if Query or Scan operation is needed
   - Build appropriate DynamoDB input structures
   - Optimize expression generation

### Phase 3: Advanced Queries (Weeks 5-6)

1. **Complex Conditions**
   - AND/OR logic support
   - Nested conditions
   - IN operator (up to 100 values)
   - BETWEEN operator
   - Function conditions:
     - begins_with
     - contains
     - attribute_exists
     - attribute_not_exists
     - attribute_type
     - size

2. **Query and Scan Operations**
   - Implement Query operation with pagination
   - Implement Scan operation with filtering
   - Parallel scan support for large tables
   - Consistent read support
   - Result pagination with cursor/token

3. **Advanced Features**
   - Projection expressions for field selection
   - Count queries
   - Aggregation helpers
   - Result streaming for large datasets

### Phase 4: Index Management (Weeks 7-8)

1. **Index Detection & Validation** (`pkg/index/`)
   - Parse GSI/LSI definitions from struct tags
   - Validate index configurations
   - Generate CloudFormation/Terraform compatible schemas
   - Detect index update requirements

2. **Automatic Index Selection**
   ```go
   type IndexSelector struct {
       indexes []IndexMetadata
       stats   QueryStatistics
   }
   
   func (is *IndexSelector) SelectOptimalIndex(conditions []Condition) (*Index, error) {
       // Implement smart index selection algorithm
       // Consider: key coverage, projection, read cost
   }
   ```

3. **Index Usage Optimization**
   - Cost-based index selection
   - Query plan caching
   - Index hints support
   - Fallback strategies when index unavailable

## Technical Requirements

1. **Performance Goals**
   - Query building < 1ms
   - Expression compilation < 2ms
   - Zero-allocation string building where possible
   - Reusable expression components

2. **Expression Safety**
   - Prevent injection attacks
   - Validate all user inputs
   - Safe handling of reserved words
   - Type-safe value substitution

3. **Error Handling**
   - Clear error messages for invalid queries
   - Suggest corrections for common mistakes
   - Detailed debugging information

## Deliverables

By the end of Week 8, you should have:

1. ✅ Complete expression builder with all DynamoDB expression types
2. ✅ Fluent query API with method chaining
3. ✅ Support for all comparison operators and functions
4. ✅ Query and Scan with pagination
5. ✅ Automatic index selection algorithm
6. ✅ Comprehensive test coverage
7. ✅ Performance benchmarks

## Example Test Cases

### Basic Query Test
```go
func TestQueryBuilder(t *testing.T) {
    query := db.Model(&User{}).
        Where("ID", "=", "user-123").
        Where("CreatedAt", ">", time.Now().Add(-24*time.Hour))
    
    // Should compile to:
    // KeyConditionExpression: "ID = :v1 AND CreatedAt > :v2"
    // ExpressionAttributeValues: {
    //   ":v1": {S: "user-123"},
    //   ":v2": {S: "2024-01-01T..."}
    // }
}
```

### Complex Query Test
```go
func TestComplexQuery(t *testing.T) {
    var users []User
    err := db.Model(&User{}).
        Index("gsi-age-status").
        Where("Age", "between", 18, 65).
        Where("Status", "in", []string{"active", "premium"}).
        Filter("attribute_exists(Email)").
        Filter("size(Tags) > :minTags", Param("minTags", 3)).
        OrderBy("CreatedAt", "desc").
        Limit(50).
        All(&users)
    
    require.NoError(t, err)
}
```

### Index Selection Test
```go
func TestIndexSelection(t *testing.T) {
    // Query should automatically select gsi-email index
    var user User
    err := db.Model(&User{}).
        Where("Email", "=", "test@example.com").
        First(&user)
    
    // Verify index was used (through debug/metrics)
    require.Equal(t, "gsi-email", query.SelectedIndex())
}
```

## Integration Points

Team 2 depends on Team 1's:
- Core interfaces (DB, Query)
- Model registry for metadata
- Type system for value conversion
- Error types

Coordinate closely on:
- Query interface methods
- Error handling patterns
- Type conversion requirements
- Performance benchmarks

## Advanced Challenges

Once basics are complete, implement:

1. **Query Optimization**
   - Query plan caching
   - Expression template reuse
   - Batch query optimization

2. **Developer Experience**
   - Query debugging/explain
   - Performance warnings
   - Cost estimation

3. **Extended Operators**
   - Custom function support
   - Complex type queries (JSON path)
   - Geospatial queries (if applicable)

## Getting Started

1. Review Team 1's interface definitions
2. Study DynamoDB expression syntax deeply
3. Set up integration tests with Team 1's code
4. Start with basic Where conditions
5. Build incrementally toward complex queries

Remember: You're creating the query experience that developers will love. Make it powerful yet intuitive! 
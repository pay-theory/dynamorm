# Team 2: Session 2 - Complete Query System & Integration

## Great Start! 🚀

Your team has built the foundation of the expression builder and query system. Now it's time to complete the implementation, fix the compilation issues, and integrate with Team 1's work to create a seamless query experience.

## Session 2 Objectives

### 1. Fix Compilation Issues (Immediate Priority)

Your `pkg/query/query.go` is missing several interface methods. Add these implementations:

```go
// BatchGet retrieves multiple items by their primary keys
func (q *Query) BatchGet(keys []interface{}, dest interface{}) error {
    // Validate dest is a pointer to slice
    // Build BatchGetItem requests
    // Use expression builder for projections
    // Coordinate with Team 1 for execution
}

// BatchCreate creates multiple items in batch
func (q *Query) BatchCreate(items interface{}) error {
    // Validate items is a slice
    // Build BatchWriteItem requests
    // Handle 25-item limit per batch
}

// Scan performs a table scan with filters
func (q *Query) Scan(dest interface{}) error {
    // Use compileScan() to build expression
    // Support parallel scan if segments > 1
    // Handle pagination
}

// WithContext sets the context for the query
func (q *Query) WithContext(ctx context.Context) core.Query {
    q.ctx = ctx
    return q
}

// Offset sets the starting offset for pagination
func (q *Query) Offset(offset int) core.Query {
    q.offset = &offset
    return q
}
```

### 2. Complete Expression Builder (`internal/expr/`)

Enhance your expression builder with:

1. **Reserved Word Handling**:
   ```go
   var reservedWords = map[string]bool{
       "NAME": true, "STATUS": true, "TYPE": true, // ... etc
   }
   
   func (b *Builder) processAttributeName(name string) string {
       if reservedWords[strings.ToUpper(name)] {
           placeholder := fmt.Sprintf("#%s", name)
           b.names[placeholder] = name
           return placeholder
       }
       return name
   }
   ```

2. **Complex Conditions Support**:
   ```go
   // Support for AND/OR groups
   func (b *Builder) BeginGroup() *Builder
   func (b *Builder) EndGroup() *Builder
   func (b *Builder) Or() *Builder
   
   // Example: (Age > 18 AND Status = 'active') OR (Role = 'admin')
   ```

3. **Advanced Functions**:
   ```go
   // Add support for:
   - size(attribute) - for lists, maps, sets
   - attribute_type(attribute, type)
   - attribute_not_exists(attribute)
   - list_append(list, value)
   ```

### 3. Query Compilation Enhancement (`pkg/query/`)

1. **Improve Index Selection**:
   ```go
   func (q *Query) selectBestIndex() (*model.IndexMetadata, error) {
       selector := index.NewSelector(q.metadata.Indexes)
       
       // Analyze conditions to find required keys
       requiredKeys := q.analyzeKeyRequirements()
       
       // Use your index selector algorithm
       return selector.SelectOptimal(requiredKeys, q.conditions)
   }
   ```

2. **Query vs Scan Decision Logic**:
   ```go
   func (q *Query) shouldUseQuery() bool {
       // Check if we have partition key condition
       // Check if selected index covers the query
       // Consider query complexity and estimated cost
   }
   ```

3. **Pagination Support**:
   ```go
   type PaginatedResult struct {
       Items      interface{}
       NextCursor string
       Count      int
   }
   
   func (q *Query) AllPaginated(dest interface{}) (*PaginatedResult, error) {
       // Build query/scan with limit
       // Handle ExclusiveStartKey
       // Encode LastEvaluatedKey as cursor
   }
   ```

### 4. Integration with Team 1

1. **Create Query Executor Interface**:
   ```go
   // pkg/query/executor.go
   type Executor interface {
       ExecuteQuery(input *CompiledQuery) (*QueryResult, error)
       ExecuteScan(input *CompiledScan) (*ScanResult, error)
       ExecuteBatchGet(input *CompiledBatchGet) (*BatchGetResult, error)
   }
   ```

2. **Compiled Query Types**:
   ```go
   type CompiledQuery struct {
       TableName       string
       IndexName       *string
       KeyCondition    Expression
       FilterCondition *Expression
       Projection      *Expression
       Limit           *int32
       ExclusiveStart  map[string]types.AttributeValue
       ConsistentRead  bool
       ScanForward     bool
   }
   ```

3. **Work with Team 1 to implement executor in their session package**

### 5. Testing & Validation

1. **Expression Builder Tests** (`internal/expr/builder_test.go`):
   ```go
   func TestComplexConditions(t *testing.T)
   func TestReservedWords(t *testing.T)
   func TestAllOperators(t *testing.T)
   func TestUpdateExpressions(t *testing.T)
   ```

2. **Query Compilation Tests** (`pkg/query/query_test.go`):
   ```go
   func TestIndexSelection(t *testing.T)
   func TestQueryVsScan(t *testing.T)
   func TestPagination(t *testing.T)
   func TestBatchOperations(t *testing.T)
   ```

3. **Integration Tests**:
   - Work with Team 1 to create end-to-end tests
   - Test all query patterns from COMPARISON.md
   - Benchmark query performance

### 6. Performance Optimizations

1. **Expression Caching**:
   ```go
   type ExpressionCache struct {
       cache sync.Map // Use sync.Map for concurrent access
   }
   
   func (c *ExpressionCache) GetOrBuild(key string, builder func() Expression) Expression
   ```

2. **Query Plan Caching**:
   ```go
   type QueryPlanCache struct {
       plans map[uint64]*QueryPlan // Hash of conditions -> plan
   }
   ```

3. **Batch Operation Optimization**:
   - Implement intelligent batching for large operations
   - Parallel processing for batch gets
   - Retry logic for unprocessed items

## Deliverables for Session 2

1. ✅ All compilation issues fixed
2. ✅ Complete expression builder with all operators
3. ✅ Reserved word handling
4. ✅ Complex condition support (AND/OR)
5. ✅ Smart index selection
6. ✅ Pagination implementation
7. ✅ Integration interfaces for Team 1
8. ✅ Comprehensive test suite
9. ✅ Performance optimizations

## Code Quality Checklist

- [ ] All methods from core.Query interface implemented
- [ ] No placeholder "not implemented" errors remain
- [ ] All operators from DESIGN.md supported
- [ ] Edge cases handled (empty conditions, nil values)
- [ ] Concurrent access is safe
- [ ] Clear error messages for invalid queries

## Success Metrics

Your implementation should enable queries like:

```go
// Complex query with automatic index selection
users, cursor, err := db.Model(&User{}).
    Where("Age", "between", 18, 65).
    Where("Status", "in", []string{"active", "premium"}).
    Filter("attribute_exists(Email)").
    Filter("size(Tags) > :min", Param("min", 2)).
    OrderBy("CreatedAt", "desc").
    Limit(50).
    AllPaginated(&users)

// Should compile to optimal DynamoDB query with:
// - Correct index selection
// - Efficient key conditions
// - Proper filter expressions
// - Pagination support
```

## Next Steps After Session 2

Once query system is complete:
1. Add query explain/debug capabilities
2. Implement query cost estimation
3. Add support for DynamoDB Streams
4. Create query performance analyzer

Remember: You're building the heart of DynamORM - make queries feel magical while being incredibly efficient under the hood! 
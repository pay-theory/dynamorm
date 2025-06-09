# Team 2 Test Coverage Implementation Guide

## Your Mission
Fix build issues and improve test coverage for query-related packages within 4 weeks.

## Assigned Packages & Current Status

| Package | Current Coverage | Target | Priority | Critical Issues |
|---------|-----------------|--------|----------|-----------------|
| pkg/query | Build Failed | 70% | CRITICAL (Week 1) | Fix build first |
| internal/expr | 35.3% | 70% | HIGH (Weeks 2-3) | Improve existing |
| pkg/index | 0% | 80% | MEDIUM (Week 4) | 151 lines |

## Week 1: Fix pkg/query Build Issues (Target: 70% coverage)

### Immediate Actions Required

1. **Diagnose Build Failure**
   ```bash
   # Check compilation errors
   go build ./pkg/query
   
   # Look for specific errors
   go test -v ./pkg/query 2>&1 | grep -E "undefined|cannot|error"
   ```

2. **Common Build Issues to Check**
   - Missing imports or dependencies
   - Interface changes in dependent packages
   - Incorrect type assertions
   - Circular dependencies

3. **Once Build is Fixed - Test Priority Areas**
   - Query builder functionality
   - Filter expressions
   - Index selection logic
   - Query optimization
   - Reserved word handling

### Query Builder Test Structure
```go
func TestQueryBuilder(t *testing.T) {
    tests := []struct {
        name     string
        setup    func() *query.Builder
        expected string
        wantErr  bool
    }{
        {
            name: "simple equality filter",
            setup: func() *query.Builder {
                return query.New("users").
                    Where("email", "=", "test@example.com")
            },
            expected: "email = :email",
            wantErr:  false,
        },
        {
            name: "compound filters with AND",
            setup: func() *query.Builder {
                return query.New("users").
                    Where("age", ">", 18).
                    Where("status", "=", "active")
            },
            expected: "age > :age AND status = :status",
            wantErr:  false,
        },
        {
            name: "using reserved words",
            setup: func() *query.Builder {
                return query.New("users").
                    Where("name", "=", "John").     // 'name' is reserved
                    Where("status", "=", "active")  // 'status' is reserved
            },
            expected: "#name = :name AND #status = :status",
            wantErr:  false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            qb := tt.setup()
            expr, err := qb.Build()
            
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            
            require.NoError(t, err)
            assert.Equal(t, tt.expected, expr.Filter)
        })
    }
}

// Test query optimization
func TestQueryOptimizer(t *testing.T) {
    optimizer := query.NewOptimizer()
    
    tests := []struct {
        name        string
        query       *query.Query
        shouldUseScan bool
        expectedIndex string
    }{
        {
            name: "use index for equality filter",
            query: &query.Query{
                TableName: "users",
                Filters: []query.Filter{
                    {Field: "email", Op: "=", Value: "test@example.com"},
                },
            },
            shouldUseScan: false,
            expectedIndex: "email-index",
        },
        {
            name: "fall back to scan for complex filters",
            query: &query.Query{
                TableName: "users",
                Filters: []query.Filter{
                    {Field: "age", Op: "BETWEEN", Value: []int{18, 65}},
                    {Field: "status", Op: "IN", Value: []string{"active", "pending"}},
                },
            },
            shouldUseScan: true,
            expectedIndex: "",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            plan := optimizer.Optimize(tt.query)
            assert.Equal(t, tt.shouldUseScan, plan.UseScan)
            if !tt.shouldUseScan {
                assert.Equal(t, tt.expectedIndex, plan.IndexName)
            }
        })
    }
}
```

## Weeks 2-3: Improve internal/expr (From 35.3% to 70%)

### Current Coverage Analysis
First, identify what's already tested and what's missing:
```bash
go test -coverprofile=expr_coverage.out ./internal/expr
go tool cover -func=expr_coverage.out | grep -E "0.0%|[0-9]\.[0-9]%" | head -20
```

### Priority Areas for New Tests

1. **Expression Builder (Currently 0% - CRITICAL)**
   ```go
   func TestExpressionBuilder(t *testing.T) {
       builder := expr.NewBuilder()
       
       tests := []struct {
           name     string
           setup    func(*expr.Builder)
           validate func(t *testing.T, result *expr.Result)
       }{
           {
               name: "key condition expression",
               setup: func(b *expr.Builder) {
                   b.AddKeyCondition("pk", "=", "user#123")
                   b.AddKeyCondition("sk", "BEGINS_WITH", "order#")
               },
               validate: func(t *testing.T, result *expr.Result) {
                   assert.Contains(t, result.KeyCondition, "pk = :pk")
                   assert.Contains(t, result.KeyCondition, "begins_with(sk, :sk)")
                   assert.Equal(t, "user#123", result.Values[":pk"])
                   assert.Equal(t, "order#", result.Values[":sk"])
               },
           },
           {
               name: "filter with functions",
               setup: func(b *expr.Builder) {
                   b.AddFilterCondition("email", "contains", "@example.com")
                   b.AddFilterCondition("age", "between", []int{18, 65})
               },
               validate: func(t *testing.T, result *expr.Result) {
                   assert.Contains(t, result.Filter, "contains(email, :email)")
                   assert.Contains(t, result.Filter, "age BETWEEN :age_start AND :age_end")
               },
           },
           {
               name: "update expressions",
               setup: func(b *expr.Builder) {
                   b.AddUpdateSet("name", "John Doe")
                   b.AddUpdateAdd("login_count", 1)
                   b.AddUpdateRemove("temp_field")
                   b.AddUpdateDelete("tags", []string{"obsolete", "deprecated"})
               },
               validate: func(t *testing.T, result *expr.Result) {
                   assert.Contains(t, result.Update, "SET name = :name")
                   assert.Contains(t, result.Update, "ADD login_count :login_count")
                   assert.Contains(t, result.Update, "REMOVE temp_field")
                   assert.Contains(t, result.Update, "DELETE tags :tags")
               },
           },
       }
       
       for _, tt := range tests {
           t.Run(tt.name, func(t *testing.T) {
               b := expr.NewBuilder()
               tt.setup(b)
               result, err := b.Build()
               require.NoError(t, err)
               tt.validate(t, result)
           })
       }
   }
   ```

2. **Advanced Functions (Currently 0%)**
   ```go
   func TestAdvancedFunctions(t *testing.T) {
       builder := expr.NewBuilder()
       
       // Test size() function
       builder.AddAdvancedFunction("size", "tags", ">", 0)
       
       // Test attribute_exists()
       builder.AddAdvancedFunction("attribute_exists", "email", "", nil)
       
       // Test attribute_not_exists()
       builder.AddAdvancedFunction("attribute_not_exists", "deleted_at", "", nil)
       
       // Test attribute_type()
       builder.AddAdvancedFunction("attribute_type", "data", "=", "M")
       
       result, err := builder.Build()
       require.NoError(t, err)
       
       assert.Contains(t, result.Filter, "size(tags) > :size_tags")
       assert.Contains(t, result.Filter, "attribute_exists(email)")
       assert.Contains(t, result.Filter, "attribute_not_exists(deleted_at)")
       assert.Contains(t, result.Filter, "attribute_type(data, :type_data)")
   }
   ```

3. **Reserved Words Handling**
   ```go
   func TestReservedWords(t *testing.T) {
       // Test all DynamoDB reserved words
       reservedWords := []string{
           "name", "status", "size", "values", "key", "range",
           "data", "type", "count", "timestamp", "hash", "item",
       }
       
       builder := expr.NewBuilder()
       for _, word := range reservedWords {
           builder.AddFilterCondition(word, "=", "test")
       }
       
       result, err := builder.Build()
       require.NoError(t, err)
       
       // All reserved words should be escaped with #
       for _, word := range reservedWords {
           assert.Contains(t, result.Names, "#"+word)
           assert.Equal(t, word, result.Names["#"+word])
       }
   }
   ```

4. **Type Converter Edge Cases**
   - Test the untested set conversion functions (0% coverage)
   - Test error paths in unmarshal functions
   - Test complex nested structures

## Week 4: pkg/index (Target: 80% coverage)

### Index Selector Implementation Tests

1. **Basic Index Selection**
   ```go
   func TestIndexSelector(t *testing.T) {
       selector := index.NewSelector()
       
       // Define table schema
       schema := &index.TableSchema{
           PrimaryKey: index.Key{
               PartitionKey: "id",
               SortKey:      "created_at",
           },
           GlobalSecondaryIndexes: []index.GSI{
               {
                   Name:         "email-index",
                   PartitionKey: "email",
                   SortKey:      "created_at",
               },
               {
                   Name:         "status-index", 
                   PartitionKey: "status",
                   SortKey:      "updated_at",
               },
           },
           LocalSecondaryIndexes: []index.LSI{
               {
                   Name:    "type-index",
                   SortKey: "type",
               },
           },
       }
       
       tests := []struct {
           name          string
           filters       map[string]interface{}
           expectedIndex string
           canUseIndex   bool
       }{
           {
               name: "use primary key",
               filters: map[string]interface{}{
                   "id":         "user123",
                   "created_at": "2024-01-01",
               },
               expectedIndex: "primary",
               canUseIndex:   true,
           },
           {
               name: "use GSI for email query",
               filters: map[string]interface{}{
                   "email": "test@example.com",
               },
               expectedIndex: "email-index",
               canUseIndex:   true,
           },
           {
               name: "no suitable index",
               filters: map[string]interface{}{
                   "age": 25,
                   "city": "New York",
               },
               expectedIndex: "",
               canUseIndex:   false,
           },
       }
       
       for _, tt := range tests {
           t.Run(tt.name, func(t *testing.T) {
               result := selector.SelectIndex(schema, tt.filters)
               assert.Equal(t, tt.canUseIndex, result.CanUseIndex)
               if tt.canUseIndex {
                   assert.Equal(t, tt.expectedIndex, result.IndexName)
               }
           })
       }
   }
   ```

2. **Index Cost Estimation**
   ```go
   func TestIndexCostEstimation(t *testing.T) {
       estimator := index.NewCostEstimator()
       
       tests := []struct {
           name         string
           indexType    string
           hasSort      bool
           selectivity  float64
           expectedCost float64
       }{
           {
               name:         "primary key with both keys",
               indexType:    "primary",
               hasSort:      true,
               selectivity:  1.0,
               expectedCost: 0.5, // Lowest cost
           },
           {
               name:         "GSI with partition key only",
               indexType:    "gsi",
               hasSort:      false,
               selectivity:  0.1,
               expectedCost: 2.0, // Higher cost
           },
           {
               name:         "scan operation",
               indexType:    "scan",
               hasSort:      false,
               selectivity:  0.0,
               expectedCost: 10.0, // Highest cost
           },
       }
       
       for _, tt := range tests {
           t.Run(tt.name, func(t *testing.T) {
               cost := estimator.EstimateCost(tt.indexType, tt.hasSort, tt.selectivity)
               assert.InDelta(t, tt.expectedCost, cost, 0.1)
           })
       }
   }
   ```

## Testing Strategy for Complex Packages

### 1. Query Package Architecture
The query package likely has several components:
- Query builder (constructs queries)
- Query optimizer (selects best execution plan)
- Query executor (runs queries)
- Result mapper (maps results to models)

Test each component in isolation first, then integration tests.

### 2. Expression Builder Patterns
```go
// Test method chaining
func TestMethodChaining(t *testing.T) {
    expr := expr.NewBuilder().
        AddKeyCondition("pk", "=", "123").
        AddFilterCondition("status", "=", "active").
        AddProjection("id", "name", "email").
        AddUpdateSet("last_seen", time.Now()).
        Build()
    
    require.NoError(t, expr.Error)
    assert.NotEmpty(t, expr.KeyCondition)
    assert.NotEmpty(t, expr.Filter)
    assert.Len(t, expr.Projection, 3)
    assert.Contains(t, expr.Update, "last_seen")
}

// Test error propagation
func TestErrorPropagation(t *testing.T) {
    expr := expr.NewBuilder().
        AddKeyCondition("", "=", "123"). // Invalid: empty key
        AddFilterCondition("status", "INVALID_OP", "active"). // Invalid operator
        Build()
    
    assert.Error(t, expr.Error)
    assert.Contains(t, expr.Error.Error(), "invalid")
}
```

### 3. Performance Considerations
```go
func BenchmarkQueryBuilder(b *testing.B) {
    b.Run("simple query", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            query.New("users").
                Where("id", "=", "123").
                Build()
        }
    })
    
    b.Run("complex query with 10 filters", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            q := query.New("users")
            for j := 0; j < 10; j++ {
                q.Where(fmt.Sprintf("field%d", j), "=", j)
            }
            q.Build()
        }
    })
}
```

## Progress Tracking & Metrics

### Daily Checklist
```bash
# Morning: Check current status
make team2-test
make coverage-dashboard

# During development: Test specific package
go test -v -cover ./pkg/query/...

# Before commit: Ensure no regression
go test ./...

# End of day: Generate report
go test -coverprofile=team2_coverage.out ./pkg/query ./internal/expr ./pkg/index
go tool cover -html=team2_coverage.out -o team2_coverage.html
```

### Weekly Goals
- **Week 1**: Query package building and 70% coverage
- **Week 2**: Expression builder from 0% to 40%
- **Week 3**: Complete expression builder to 70%
- **Week 4**: Index selector to 80%

## Common Challenges & Solutions

### 1. Fixing Build Issues
```bash
# Clean module cache
go clean -modcache

# Update dependencies
go mod tidy
go mod download

# Check for circular imports
go list -f '{{.ImportPath}} -> {{.Imports}}' ./... | grep circular
```

### 2. Testing Internal Packages
Internal packages can only be tested from within their module:
```go
// Place tests in internal/expr/expr_test.go
package expr_test // Use _test suffix for black-box testing

// Or use same package for white-box testing
package expr
```

### 3. Mocking Complex Dependencies
```go
// Create interfaces for testability
type QueryExecutor interface {
    Execute(ctx context.Context, query *Query) (*Result, error)
}

// Mock implementation
type MockExecutor struct {
    mock.Mock
}

func (m *MockExecutor) Execute(ctx context.Context, query *Query) (*Result, error) {
    args := m.Called(ctx, query)
    return args.Get(0).(*Result), args.Error(1)
}
```

## Integration with Team 1

Coordinate on shared interfaces:
1. Expression builder outputs (used by marshal/types)
2. Error types (ensure consistent error handling)
3. Model interfaces (query builder needs these)

## Quality Standards

1. **All tests must be deterministic** - No flaky tests
2. **Use meaningful test names** - Should describe the scenario
3. **Document complex test logic** - Add comments for clarity
4. **Test both success and failure paths**
5. **Include benchmarks for performance-critical code**

## Deliverables Checklist

- [ ] Query package builds successfully
- [ ] All assigned packages meet coverage targets
- [ ] No failing tests in CI
- [ ] Benchmarks show acceptable performance
- [ ] Integration tests with other packages
- [ ] Documentation updated for API changes
- [ ] Code review completed

## Resources

- Test templates: `docs/development/test-templates.md`
- Coverage dashboard: `make coverage-dashboard`
- Good examples: `pkg/transaction/*_test.go`
- DynamoDB expressions reference: AWS documentation

Remember: Focus on fixing the build issues first, then systematically improve coverage. The query functionality is critical to the ORM, so ensure thorough testing of all edge cases. 
# Team 2: Session 3 - Testing, Performance & Polish

## Incredible Work! 🚀

Your query system is feature-complete with smart index selection, complex expressions, and seamless integration. Now it's time to ensure everything is production-ready with comprehensive testing and performance optimization.

## Session 3 Objectives

### Priority 1: Comprehensive Testing Suite

#### 1.1 Integration Tests (`tests/integration/`)

Create end-to-end tests that validate the entire query pipeline:

```go
// tests/integration/query_integration_test.go
package integration

import (
    "testing"
    "github.com/pay-theory/dynamorm"
    "github.com/stretchr/testify/suite"
)

type QueryIntegrationSuite struct {
    suite.Suite
    db *dynamorm.DB
}

func (s *QueryIntegrationSuite) SetupSuite() {
    // Initialize DB with DynamoDB Local
    db, err := dynamorm.New(dynamorm.Config{
        Region:   "us-east-1",
        Endpoint: "http://localhost:8000",
    })
    s.Require().NoError(err)
    s.db = db
    
    // Create test tables
    s.Require().NoError(db.AutoMigrate(&User{}, &Product{}, &Order{}))
}

func (s *QueryIntegrationSuite) TestComplexQueryWithIndexSelection() {
    // Test that queries automatically select the right index
    // Verify results are correct
    // Check performance metrics
}

func (s *QueryIntegrationSuite) TestBatchOperationsWithLimits() {
    // Test batch operations respect DynamoDB limits
    // Test retry logic for unprocessed items
    // Verify all items are processed
}

func (s *QueryIntegrationSuite) TestPaginationAcrossMultiplePages() {
    // Create 1000+ items
    // Query with small page size
    // Verify cursor-based pagination works
    // Ensure no items are missed or duplicated
}
```

#### 1.2 Performance Benchmarks (`tests/benchmarks/`)

Create benchmarks to verify performance targets:

```go
// tests/benchmarks/query_bench_test.go
func BenchmarkSimpleQuery(b *testing.B) {
    db, _ := setupBenchDB()
    user := &User{ID: "bench-user"}
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        err := db.Model(&User{}).Where("ID", "=", "bench-user").First(user)
        if err != nil {
            b.Fatal(err)
        }
    }
    
    // Target: < 5ms overhead over raw SDK
}

func BenchmarkComplexQueryWithFilters(b *testing.B) {
    // Benchmark complex queries
    // Measure expression building overhead
    // Verify < 10ms for complex queries
}

func BenchmarkIndexSelection(b *testing.B) {
    // Measure index selection algorithm performance
    // Should be < 1ms even with many indexes
}

func BenchmarkExpressionBuilding(b *testing.B) {
    // Isolate expression builder performance
    // Target: < 2ms for complex expressions
}
```

#### 1.3 Stress Tests (`tests/stress/`)

Test system under load:

```go
func TestConcurrentQueries(t *testing.T) {
    // Run 1000 concurrent queries
    // Verify no race conditions
    // Check memory usage stays stable
}

func TestLargeItemHandling(t *testing.T) {
    // Test with items near 400KB limit
    // Test with 100+ attributes
    // Verify performance doesn't degrade
}
```

### Priority 2: Pagination Enhancement

#### 2.1 Cursor Implementation (`pkg/query/cursor.go`)

Implement proper cursor encoding/decoding:

```go
package query

import (
    "encoding/base64"
    "encoding/json"
)

type Cursor struct {
    LastEvaluatedKey map[string]types.AttributeValue
    SortDirection    string
    IndexName        string
}

func EncodeCursor(lastKey map[string]types.AttributeValue, indexName string) (string, error) {
    // Convert AttributeValues to JSON-friendly format
    jsonKey := make(map[string]interface{})
    for k, v := range lastKey {
        jsonKey[k] = attributeValueToJSON(v)
    }
    
    cursor := Cursor{
        LastEvaluatedKey: jsonKey,
        IndexName:        indexName,
    }
    
    data, err := json.Marshal(cursor)
    if err != nil {
        return "", err
    }
    
    return base64.URLEncoding.EncodeToString(data), nil
}

func DecodeCursor(encoded string) (*Cursor, error) {
    // Decode base64
    // Unmarshal JSON
    // Convert back to AttributeValues
}
```

#### 2.2 Update Query Methods

Add pagination methods to query:

```go
func (q *Query) Cursor(cursor string) core.Query {
    decoded, err := DecodeCursor(cursor)
    if err != nil {
        q.err = err
        return q
    }
    
    q.exclusiveStartKey = decoded.LastEvaluatedKey
    return q
}

func (q *Query) AllPaginated(dest interface{}) (*PaginatedResult, error) {
    // Execute query
    // Return results with cursor
    
    return &PaginatedResult{
        Items:      dest,
        NextCursor: encodedCursor,
        Count:      len(items),
        HasMore:    lastEvaluatedKey != nil,
    }, nil
}
```

### Priority 3: Performance Optimizations

#### 3.1 Expression Caching (`internal/expr/cache.go`)

Implement expression caching for repeated queries:

```go
package expr

import (
    "sync"
    "hash/fnv"
)

type ExpressionCache struct {
    cache sync.Map
    stats CacheStats
}

type CacheStats struct {
    Hits   int64
    Misses int64
    Size   int64
}

func (ec *ExpressionCache) GetOrBuild(key string, builder func() *Expression) *Expression {
    // Check cache
    if expr, found := ec.cache.Load(key); found {
        atomic.AddInt64(&ec.stats.Hits, 1)
        return expr.(*Expression)
    }
    
    // Build and cache
    atomic.AddInt64(&ec.stats.Misses, 1)
    expr := builder()
    ec.cache.Store(key, expr)
    
    return expr
}

// Use in query compilation
func (q *Query) compile() (*CompiledQuery, error) {
    cacheKey := q.buildCacheKey()
    
    compiled := exprCache.GetOrBuild(cacheKey, func() *Expression {
        return q.buildExpression()
    })
    
    return compiled, nil
}
```

#### 3.2 Query Plan Optimization

Enhance index selection with statistics:

```go
type QueryStatistics struct {
    IndexUsage map[string]IndexStats
    mu         sync.RWMutex
}

type IndexStats struct {
    UsageCount     int64
    AvgLatency     time.Duration
    AvgItemsRead   int64
    LastUsed       time.Time
}

func (qs *QueryStatistics) RecordIndexUsage(indexName string, latency time.Duration, itemsRead int) {
    // Update statistics
    // Use for future index selection
}
```

### Priority 4: Documentation & Examples

#### 4.1 API Documentation (`docs/api/`)

Generate comprehensive godoc documentation:

```go
// Package dynamorm provides a powerful, type-safe ORM for Amazon DynamoDB.
//
// Basic Usage:
//
//   db, err := dynamorm.New(dynamorm.Config{
//       Region: "us-east-1",
//   })
//   
//   var user User
//   err = db.Model(&User{}).
//       Where("Email", "=", "john@example.com").
//       First(&user)
//
// Complex Queries:
//
//   var users []User
//   err = db.Model(&User{}).
//       Where("Age", ">=", 18).
//       Where("Status", "=", "active").
//       Filter("contains(Tags, :tag)", dynamorm.Param("tag", "premium")).
//       OrderBy("CreatedAt", "desc").
//       Limit(20).
//       All(&users)
//
package dynamorm
```

#### 4.2 Example Application (`examples/`)

Create a complete example showing best practices:

```go
// examples/blog/main.go
package main

import (
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/examples/blog/models"
)

func main() {
    // Initialize DynamORM
    db, err := dynamorm.New(dynamorm.Config{
        Region: "us-east-1",
    })
    
    // Create tables
    err = db.AutoMigrate(
        &models.User{},
        &models.Post{},
        &models.Comment{},
    )
    
    // Example: Find posts by user with comments
    var posts []models.Post
    err = db.Model(&models.Post{}).
        Where("UserID", "=", userID).
        Where("Status", "=", "published").
        OrderBy("CreatedAt", "desc").
        All(&posts)
    
    // Load comments for each post...
}
```

### Priority 5: Error Enhancement

Improve error messages for better developer experience:

```go
// pkg/errors/query_errors.go
type QueryError struct {
    Operation   string
    Query       string
    Suggestion  string
    Cause       error
}

func (e *QueryError) Error() string {
    msg := fmt.Sprintf("Query failed during %s", e.Operation)
    if e.Query != "" {
        msg += fmt.Sprintf("\nQuery: %s", e.Query)
    }
    if e.Suggestion != "" {
        msg += fmt.Sprintf("\nSuggestion: %s", e.Suggestion)
    }
    if e.Cause != nil {
        msg += fmt.Sprintf("\nCause: %v", e.Cause)
    }
    return msg
}

// Example usage
if !hasPartitionKey {
    return &QueryError{
        Operation:  "Query",
        Query:      fmt.Sprintf("Where conditions: %v", conditions),
        Suggestion: "Query operation requires a partition key condition. Use Scan() for full table search.",
        Cause:      ErrMissingPartitionKey,
    }
}
```

## Deliverables for Session 3

### Must Have
1. ✅ Integration test suite with DynamoDB Local
2. ✅ Performance benchmarks proving < 5% overhead
3. ✅ Cursor-based pagination implementation
4. ✅ Basic API documentation

### Should Have
1. ✅ Expression caching for performance
2. ✅ Query statistics collection
3. ✅ Stress tests for concurrent usage
4. ✅ Example application

### Nice to Have
1. 🎯 Query explain/debug mode
2. 🎯 Cost estimation for queries
3. 🎯 Visual query plan output
4. 🎯 Performance profiling tools

## Testing Checklist

- [ ] All query patterns from COMPARISON.md have tests
- [ ] Pagination works across multiple pages
- [ ] Concurrent queries don't cause races
- [ ] Memory usage is stable under load
- [ ] Benchmarks meet performance targets
- [ ] Error messages are helpful

## Success Metrics

1. **Test Coverage**: > 90% for query package
2. **Performance**: < 5% overhead verified by benchmarks
3. **Reliability**: 1000+ concurrent queries without issues
4. **Documentation**: All public APIs documented
5. **Examples**: Working example application

## Quick Testing Commands

```bash
# Run all tests
make test

# Run integration tests
make integration

# Run benchmarks
make benchmark

# Check race conditions
go test -race ./pkg/query/...

# Generate coverage report
make coverage
```

Remember: Great testing and documentation are what separate a good library from a great one. Make DynamORM a joy to use! 
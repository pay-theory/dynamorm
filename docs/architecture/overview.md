# DynamORM Technical Architecture

## Overview

This document describes the technical architecture, design patterns, and implementation details of DynamORM.

## Core Architecture Principles

### 1. Layered Architecture

```
┌─────────────────────────────────────────┐
│           Application Layer             │
├─────────────────────────────────────────┤
│              Public API                 │
│   (DB, Query, Model, Transaction)       │
├─────────────────────────────────────────┤
│           Core Services                 │
│  (Registry, Builder, Engine, Manager)   │
├─────────────────────────────────────────┤
│         Internal Components             │
│   (Expression, Reflection, Utils)       │
├─────────────────────────────────────────┤
│            AWS SDK Layer                │
│        (DynamoDB Client v2)             │
└─────────────────────────────────────────┘
```

### 2. Interface-Driven Design

All major components are defined as interfaces to allow for:
- Easy testing with mocks
- Alternative implementations
- Clean separation of concerns
- Plugin architecture for extensions

### 3. Builder Pattern

Query construction uses a fluent builder pattern:
```go
query.Where("field", "=", value).
      OrderBy("created").
      Limit(10)
```

## Key Components

### 1. Model Registry

**Purpose**: Central repository for model metadata and configuration

**Implementation**:
```go
type Registry struct {
    models map[reflect.Type]*ModelMetadata
    mu     sync.RWMutex
}

type ModelMetadata struct {
    Type       reflect.Type
    TableName  string
    Keys       KeySchema
    Attributes []AttributeMetadata
    Indexes    []IndexMetadata
}
```

**Key Features**:
- Thread-safe registration
- Lazy initialization
- Caching of reflection results
- Validation of model structure

### 2. Query Builder

**Purpose**: Construct DynamoDB operations with a fluent API

**Architecture**:
```go
type Query struct {
    model      interface{}
    conditions []Condition
    index      string
    limit      int
    projection []string
    
    // Internal state
    compiled   *CompiledQuery
    registry   *Registry
}
```

**Query Compilation Process**:
1. Parse conditions into expression tree
2. Analyze available indexes
3. Select optimal access pattern
4. Generate DynamoDB expressions
5. Cache compiled query

### 3. Expression Engine

**Purpose**: Convert high-level conditions to DynamoDB expressions

**Key Components**:
```go
type ExpressionBuilder struct {
    keyConditions     []string
    filterConditions  []string
    attributeNames    map[string]string
    attributeValues   map[string]types.AttributeValue
}
```

**Expression Types**:
- Key Condition Expressions
- Filter Expressions
- Update Expressions
- Projection Expressions
- Condition Expressions

### 4. Type System

**Purpose**: Handle conversion between Go types and DynamoDB AttributeValues

**Type Converter Interface**:
```go
type Converter interface {
    ToDynamoDB(value interface{}) (types.AttributeValue, error)
    FromDynamoDB(av types.AttributeValue, target interface{}) error
}
```

**Built-in Converters**:
- Primitive types (string, int, float, bool)
- Time/Date types
- Binary data
- Collections (slice, map, set)
- Custom types via registration

### 5. Index Manager

**Purpose**: Automatic index selection and optimization

**Index Selection Algorithm**:
```go
func (im *IndexManager) SelectIndex(query *Query) (*Index, error) {
    // 1. Check if specific index requested
    if query.index != "" {
        return im.getIndex(query.index)
    }
    
    // 2. Analyze query conditions
    requiredKeys := im.extractRequiredKeys(query)
    
    // 3. Find matching indexes
    candidates := im.findCandidateIndexes(requiredKeys)
    
    // 4. Score candidates based on:
    //    - Key coverage
    //    - Projection efficiency
    //    - Read cost
    
    // 5. Return optimal index
    return im.selectOptimal(candidates)
}
```

### 6. Transaction Manager

**Purpose**: Handle DynamoDB transactions with proper isolation

**Implementation**:
```go
type Transaction struct {
    items []TransactItem
    db    *DB
}

type TransactItem struct {
    Operation OperationType
    Query     *Query
    Item      interface{}
}
```

**Transaction Flow**:
1. Collect operations
2. Validate consistency
3. Build TransactWriteItems request
4. Execute with automatic retry
5. Handle partial failures

## Design Patterns

### 1. Repository Pattern

Each model acts as a repository:
```go
// Simple, intuitive API
var user User
err := db.Model(&User{}).Where("ID", "=", id).First(&user)
```

### 2. Unit of Work

Transactions implement unit of work pattern:
```go
db.Transaction(func(tx *Tx) error {
    // All operations succeed or fail together
})
```

### 3. Lazy Loading

Queries are not executed until terminal operation:
```go
query := db.Model(&User{}).Where(...) // Not executed
users, err := query.All()              // Executed here
```

### 4. Chain of Responsibility

Query modifiers form a chain:
```go
type QueryModifier interface {
    Apply(query *Query) *Query
}
```

## Performance Optimizations

### 1. Query Plan Caching

```go
type QueryCache struct {
    plans map[string]*QueryPlan
    mu    sync.RWMutex
}
```

Cache key includes:
- Model type
- Conditions
- Selected index
- Projection

### 2. Connection Pooling

```go
type ClientPool struct {
    clients chan *dynamodb.Client
    config  aws.Config
}
```

Features:
- Configurable pool size
- Health checking
- Automatic retry

### 3. Batch Operation Optimization

```go
type BatchProcessor struct {
    queue     chan BatchItem
    batchSize int
    interval  time.Duration
}
```

Automatic batching based on:
- Item count (max 25 for DynamoDB)
- Time window
- Memory pressure

### 4. Expression Reuse

```go
type ExpressionCache struct {
    expressions map[string]*Expression
}
```

Cached expressions for:
- Common queries
- Update patterns
- Filter conditions

## Error Handling Strategy

### 1. Error Types

```go
type DynamORMError struct {
    Op        string                 // Operation that failed
    Model     string                 // Model involved
    Err       error                  // Underlying error
    Retryable bool                   // Can be retried
    Context   map[string]interface{} // Additional context
}
```

### 2. Error Categories

- **Validation Errors**: Invalid model structure, bad queries
- **AWS Errors**: Service limits, throttling, network issues
- **Logic Errors**: Item not found, condition failures
- **System Errors**: Out of memory, panic recovery

### 3. Retry Strategy

```go
type RetryPolicy struct {
    MaxAttempts     int
    BackoffStrategy BackoffFunc
    RetryableErrors []error
}
```

Exponential backoff with jitter for:
- Throttling exceptions
- Service unavailable
- Timeout errors

## Security Considerations

### 1. Input Validation

All user inputs are validated:
- SQL injection prevention
- Expression injection prevention
- Type validation

### 2. IAM Integration

```go
type Config struct {
    AssumeRole   string
    SessionName  string
    ExternalID   string
}
```

### 3. Encryption Support

- Encryption at rest (AWS managed)
- Client-side encryption hooks
- Field-level encryption

## Testing Architecture

### 1. Mock Client

```go
type MockDB struct {
    expectations []Expectation
    calls        []Call
}
```

### 2. Test Utilities

```go
// Table creation for tests
func CreateTestTable(t *testing.T, model interface{})

// Data fixtures
func LoadFixture(t *testing.T, fixture string)

// Assertions
func AssertQuery(t *testing.T, query *Query, expected string)
```

### 3. Integration Test Strategy

- LocalStack for local testing
- Isolated test tables
- Parallel test execution
- Automatic cleanup

## Monitoring and Observability

### 1. Metrics Collection

```go
type Metrics struct {
    Queries      Counter
    Errors       Counter
    Latency      Histogram
    Throughput   Gauge
}
```

### 2. Tracing Integration

```go
type TracingMiddleware struct {
    tracer trace.Tracer
}
```

OpenTelemetry support for:
- Query tracing
- Operation timing
- Error tracking

### 3. Logging

Structured logging with levels:
- DEBUG: Query plans, expressions
- INFO: Operations, results
- WARN: Performance issues, deprecations
- ERROR: Failures, retries

## Extension Points

### 1. Hooks

```go
type Hooks struct {
    BeforeCreate []func(interface{}) error
    AfterCreate  []func(interface{}) error
    BeforeQuery  []func(*Query) error
    AfterQuery   []func(*Query, interface{}) error
}
```

### 2. Plugins

```go
type Plugin interface {
    Name() string
    Initialize(*DB) error
    Shutdown() error
}
```

Example plugins:
- Caching layer
- Audit logging
- Metrics collection
- Custom validators

### 3. Custom Types

```go
func RegisterType(typ reflect.Type, converter Converter) {
    defaultRegistry.RegisterConverter(typ, converter)
}
```

## Performance Benchmarks

Target performance metrics:
- Model registration: < 1ms
- Simple query: < 5ms overhead
- Complex query: < 10ms overhead
- Batch operations: < 2ms per item
- Transaction: < 20ms overhead

## Future Considerations

### 1. Query Optimization Engine

- Cost-based optimization
- Query plan visualization
- Performance recommendations

### 2. Schema Evolution

- Online schema changes
- Zero-downtime migrations
- Automatic backfilling

### 3. Multi-Region Support

- Global table management
- Region failover
- Consistency guarantees

### 4. Advanced Features

- Change data capture
- Materialized views
- Cross-table joins (via Lambda) 
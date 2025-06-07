# Team Coordination Guide

## Overview

This document helps Team 1 (Core Foundation) and Team 2 (Query Builder) coordinate their efforts on DynamORM.

## Timeline & Dependencies

```
Week 1-2: Team 1 builds core foundation
         Team 2 reviews design, sets up environment, prototypes expressions

Week 3-4: Team 1 implements basic CRUD
         Team 2 builds query builder on Team 1's interfaces

Week 5-6: Team 1 supports Team 2, adds missing type converters
         Team 2 implements advanced queries

Week 7-8: Both teams collaborate on index management
         Team 1: Schema/table creation
         Team 2: Index selection algorithm
```

## Shared Interfaces

Both teams must agree on these core interfaces:

### From Team 1 (pkg/core/interfaces.go)
```go
package core

type DB interface {
    Model(interface{}) Query
    Transaction(func(*Tx) error) error
    Migrate() error
    Close() error
}

type Query interface {
    // Builder methods (Team 2 implements)
    Where(field string, op string, value interface{}) Query
    Filter(expr string, params ...Param) Query
    Index(name string) Query
    Limit(n int) Query
    OrderBy(field string, order string) Query
    Select(fields ...string) Query
    
    // Execution methods (Team 1 implements core, Team 2 extends)
    First(dest interface{}) error
    All(dest interface{}) error
    Count() (int64, error)
    Create() error
    Update(fields ...string) error
    Delete() error
    
    // Internal methods (Team 2 implements)
    compile() (*CompiledQuery, error)
}

type ModelMetadata interface {
    TableName() string
    PrimaryKey() KeySchema
    Indexes() []IndexSchema
    AttributeMetadata(field string) *AttributeMetadata
}
```

## Integration Points

### 1. Type System (Team 1 → Team 2)
Team 2 needs Team 1's type converters for expression building:

```go
// Team 1 provides
package types

type Converter interface {
    ToDynamoDB(value interface{}) (types.AttributeValue, error)
    FromDynamoDB(av types.AttributeValue, target interface{}) error
}

// Team 2 uses for expression values
func (eb *ExpressionBuilder) AddValue(key string, value interface{}) error {
    av, err := eb.converter.ToDynamoDB(value)
    if err != nil {
        return err
    }
    eb.values[key] = av
    return nil
}
```

### 2. Model Registry (Team 1 → Team 2)
Team 2 queries model metadata for query optimization:

```go
// Team 2 needs
metadata := registry.GetMetadata(model)
indexes := metadata.Indexes()
// Select best index for query conditions
```

### 3. Error Types (Shared)
Both teams use consistent error types:

```go
package errors

var (
    // Team 1 errors
    ErrItemNotFound      = New("item not found")
    ErrInvalidModel      = New("invalid model")
    ErrMissingPrimaryKey = New("missing primary key")
    
    // Team 2 errors  
    ErrInvalidOperator   = New("invalid query operator")
    ErrIndexNotFound     = New("index not found")
    ErrInvalidExpression = New("invalid expression")
)
```

## Communication Protocol

### Daily Sync Points
1. **Morning Standup** (15 min)
   - Progress updates
   - Blockers
   - Integration needs

2. **API Review** (as needed)
   - Before implementing new interfaces
   - When changing existing interfaces
   - Document decisions in `docs/decisions/`

### Code Review Process
1. Cross-team reviews for interface changes
2. Team lead approval for public API changes
3. Performance review for critical paths

### Integration Testing
Week 3 onwards: Daily integration tests
```bash
# Run from project root
make test-integration
```

## Shared Resources

### Development Environment
```yaml
# docker-compose.yml
version: '3.8'
services:
  dynamodb-local:
    image: amazon/dynamodb-local
    ports:
      - "8000:8000"
    command: "-jar DynamoDBLocal.jar -sharedDb"
```

### Test Data
Both teams use the same test models:
```go
// tests/models/test_models.go
type TestUser struct {
    ID        string    `dynamorm:"pk"`
    Email     string    `dynamorm:"index:gsi-email"`
    CreatedAt time.Time `dynamorm:"sk"`
}

type TestProduct struct {
    SKU      string  `dynamorm:"pk"`
    Category string  `dynamorm:"index:gsi-category,pk"`
    Price    float64 `dynamorm:"index:gsi-category,sk"`
}
```

## Conflict Resolution

### Interface Disputes
1. Document both proposals
2. Benchmark if performance-related
3. Team leads make final decision
4. Document decision and rationale

### Priority Conflicts
1. Blocker issues first
2. Integration points second
3. Features third
4. Optimizations last

## Success Metrics

### Week 3 Checkpoint
- [ ] Basic CRUD working (Team 1)
- [ ] Simple Where queries working (Team 2)
- [ ] Integration tests passing

### Week 6 Checkpoint
- [ ] All operators supported (Team 2)
- [ ] Type system complete (Team 1)
- [ ] 50+ integration tests passing

### Week 8 Checkpoint
- [ ] Index management working
- [ ] Performance benchmarks meeting targets
- [ ] API documentation complete

## Quick Reference

### Team 1 Contacts
- Core Interfaces: `pkg/core/`
- Model Registry: `pkg/model/`
- Type System: `pkg/types/`
- Slack: #dynamorm-team1

### Team 2 Contacts
- Query Builder: `pkg/query/`
- Expression Engine: `internal/expr/`
- Index Manager: `pkg/index/`
- Slack: #dynamorm-team2

### Shared Resources
- Design Docs: `/docs/`
- Integration Tests: `/tests/integration/`
- Benchmarks: `/tests/benchmarks/`
- CI/CD: `.github/workflows/`

Remember: We're building something amazing together. Communicate early, integrate often, and keep the end user in mind! 
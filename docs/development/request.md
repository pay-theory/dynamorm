# DynamORM Mocking Challenge
Date: 2025-06-10
Team: Team 2

## Issue
Cannot directly mock DynamORM's *dynamorm.DB type for unit testing because it's a concrete type, not an interface.

## Impact
- Cannot unit test services in isolation
- Need to use integration tests with real DynamoDB
- Makes test execution slower and more complex

## Potential Solutions

### 1. Interface Wrapper (Recommended)
Create our own interface that wraps DynamORM operations:

```go
type DynamoDBInterface interface {
    Model(model interface{}) QueryInterface
}

type QueryInterface interface {
    Create() error
    Where(field, op string, value interface{}) QueryInterface
    First(result interface{}) error
    All(results interface{}) error
    Update() error
    Delete() error
    // ... other methods
}
```

### 2. Integration Tests Only
Skip unit tests and rely on integration tests with local DynamoDB or test containers.

### 3. Request DynamORM Changes
Submit PR to DynamORM to expose interfaces for testing.

## Decision
For Sprint 2, we'll proceed with integration tests to maintain velocity. We can revisit the interface wrapper approach if testing becomes a bottleneck.

## Action Items
- [ ] Create integration test setup with test DynamoDB tables
- [ ] Document testing approach for Team 1
- [ ] Consider interface wrapper for Sprint 3 
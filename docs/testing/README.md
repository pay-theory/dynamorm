# DynamORM Testing Guide

This guide covers testing best practices for applications using DynamORM, with a focus on mock implementations and middleware integration.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Mock Implementations](#mock-implementations)
3. [Factory Pattern](#factory-pattern)
4. [Testing Middleware](#testing-middleware)
5. [Common Scenarios](#common-scenarios)
6. [Advanced Testing](#advanced-testing)

## Quick Start

DynamORM provides comprehensive mocking support through the `pkg/mocks` and `pkg/testing` packages.

### Basic Example

```go
import (
    "testing"
    "github.com/pay-theory/dynamorm/pkg/mocks"
    "github.com/pay-theory/dynamorm/pkg/testing"
    "github.com/stretchr/testify/assert"
)

func TestCreateUser(t *testing.T) {
    // Create a mock database with sensible defaults
    mockDB := mocks.NewMockExtendedDB()
    mockQuery := new(mocks.MockQuery)
    
    // Set up expectations
    mockDB.On("Model", &User{}).Return(mockQuery)
    mockQuery.On("Create").Return(nil)
    
    // Use the mock in your code
    err := CreateUser(mockDB, &User{Name: "Alice"})
    
    // Assert expectations were met
    assert.NoError(t, err)
    mockDB.AssertExpectations(t)
    mockQuery.AssertExpectations(t)
}
```

## Mock Implementations

### MockDB vs MockExtendedDB

DynamORM provides two levels of mock implementations:

1. **MockDB**: Implements the basic `core.DB` interface
2. **MockExtendedDB**: Implements the full `core.ExtendedDB` interface (recommended)

```go
// Use MockExtendedDB for full compatibility
mockDB := mocks.NewMockExtendedDB()

// MockExtendedDB includes sensible defaults for schema operations
// that are rarely used in unit tests
```

### Why MockExtendedDB?

The `ExtendedDB` interface includes additional methods for:
- Schema management (`CreateTable`, `DeleteTable`, `DescribeTable`)
- Enhanced migrations (`AutoMigrateWithOptions`, `EnsureTable`)
- Lambda-specific features (`WithLambdaTimeout`, `WithLambdaTimeoutBuffer`)
- Extended transactions (`TransactionFunc`)

`MockExtendedDB` provides default implementations for these methods, reducing boilerplate in your tests.

## Factory Pattern

The factory pattern enables dependency injection and makes testing easier.

### Basic Factory Usage

```go
// In your production code
type DBFactory interface {
    CreateDB(config session.Config) (core.ExtendedDB, error)
}

// In your middleware
func WithDynamORM(config *Config, factory DBFactory) Middleware {
    return func(next Handler) Handler {
        return func(ctx *Context) error {
            db, err := factory.CreateDB(sessionConfig)
            if err != nil {
                return err
            }
            ctx.Set("dynamorm", db)
            return next(ctx)
        }
    }
}

// In your tests
func TestHandler(t *testing.T) {
    // Create a mock factory
    factory := testing.NewMockDBFactory()
    
    // Or with custom setup
    factory := testing.SimpleMockFactory(func(db *mocks.MockExtendedDB) {
        // Set up your expectations here
        mockQuery := new(mocks.MockQuery)
        db.On("Model", &User{}).Return(mockQuery)
        mockQuery.On("Create").Return(nil)
    })
    
    // Use in your application
    app.Use(WithDynamORM(config, factory))
}
```

### Advanced Factory Features

```go
// Track all created instances
testFactory := &testing.TestDBFactory{}
app.Use(WithDynamORM(config, testFactory))

// Later, inspect created instances
lastDB := testFactory.GetLastInstance()

// Configure factory to return errors
factory := testing.NewMockDBFactory().
    WithError(errors.New("connection failed"))
```

## Testing Middleware

### Middleware Integration Pattern

```go
func TestMiddlewareIntegration(t *testing.T) {
    // Step 1: Create test database
    testDB := testing.NewTestDB()
    
    // Step 2: Set up common scenarios
    scenarios := testing.NewCommonScenarios(testDB)
    scenarios.SetupCRUD(&User{})
    
    // Step 3: Create factory
    factory := &testing.MockDBFactory{
        MockDB: testDB.MockDB,
    }
    
    // Step 4: Test your middleware
    app := NewApp()
    app.Use(WithDynamORM(config, factory))
    
    // Step 5: Make requests and assert
    resp := app.Test(httptest.NewRequest("POST", "/users", userJSON))
    assert.Equal(t, 201, resp.StatusCode)
    testDB.AssertExpectations(t)
}
```

### Multi-Tenant Testing

```go
func TestMultiTenantIsolation(t *testing.T) {
    testDB := testing.NewTestDB()
    scenarios := testing.NewCommonScenarios(testDB)
    
    // Set up multi-tenant expectations
    scenarios.SetupMultiTenant("tenant-123")
    
    // Your handler should automatically filter by tenant
    testDB.ExpectWhere("tenant_id", "=", "tenant-123")
    testDB.ExpectFind(&User{ID: "user-1", TenantID: "tenant-123"})
    
    // Test the handler
    result := GetUser(testDB.MockDB, "user-1", "tenant-123")
    assert.Equal(t, "tenant-123", result.TenantID)
}
```

## Common Scenarios

### CRUD Operations

```go
func TestCRUDOperations(t *testing.T) {
    testDB := testing.NewTestDB()
    
    // Create
    testDB.ExpectModel(&User{}).ExpectCreate()
    
    // Read
    testDB.ExpectModel(&User{}).
        ExpectWhere("ID", "=", "123").
        ExpectFind(&User{ID: "123", Name: "Alice"})
    
    // Update
    testDB.ExpectModel(&User{}).
        ExpectWhere("ID", "=", "123").
        ExpectUpdate("Name", "Email")
    
    // Delete
    testDB.ExpectModel(&User{}).
        ExpectWhere("ID", "=", "123").
        ExpectDelete()
}
```

### Transaction Testing

```go
func TestTransactions(t *testing.T) {
    testDB := testing.NewTestDB()
    
    // Success case
    testDB.ExpectTransaction(func(tx *core.Tx) {
        // Transaction will succeed
    })
    
    // Failure case
    testDB.ExpectTransactionError(errors.New("constraint violation"))
    
    // Complex transaction
    testDB.ExpectTransaction(func(tx *core.Tx) {
        // Set up expectations for operations within transaction
        testDB.ExpectModel(&Order{}).ExpectCreate()
        testDB.ExpectModel(&Inventory{}).ExpectUpdate("Quantity")
    })
}
```

### Batch Operations

```go
func TestBatchOperations(t *testing.T) {
    testDB := testing.NewTestDB()
    
    users := []User{{Name: "Alice"}, {Name: "Bob"}}
    testDB.ExpectBatchCreate(users)
    
    keys := []interface{}{"id1", "id2", "id3"}
    testDB.ExpectBatchGet(keys, &users)
    
    testDB.ExpectBatchDelete(keys)
}
```

### Query Chains

```go
func TestComplexQueries(t *testing.T) {
    testDB := testing.NewTestDB()
    
    // Use query chain builder
    users := []User{}
    testDB.NewQueryChain().
        Where("Status", "=", "active").
        Where("CreatedAt", ">", yesterday).
        OrderBy("CreatedAt", "DESC").
        Limit(10).
        ExpectAll(&users)
}
```

## Advanced Testing

### Performance Testing

```go
func BenchmarkUserCreation(b *testing.B) {
    testDB := testing.NewTestDB()
    testDB.ExpectModel(&User{}).ExpectCreate()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        CreateUser(testDB.MockDB, &User{Name: "Test"})
    }
}
```

### Error Scenarios

```go
func TestErrorHandling(t *testing.T) {
    testDB := testing.NewTestDB()
    scenarios := testing.NewCommonScenarios(testDB)
    
    // Set up various error scenarios
    scenarios.SetupErrorScenarios(map[string]error{
        "create": errors.ErrItemNotFound,
        "update": errors.ErrConditionFailed,
        "delete": errors.ErrInvalidModel,
    })
    
    // Test your error handling
    err := CreateUser(testDB.MockDB, &User{})
    assert.ErrorIs(t, err, errors.ErrItemNotFound)
}
```

### Testing Helpers

The `pkg/testing` package provides several helpers:

```go
// TestDB - Fluent interface for setting up expectations
testDB := testing.NewTestDB()

// CommonScenarios - Pre-built test scenarios
scenarios := testing.NewCommonScenarios(testDB)
scenarios.SetupCRUD(&User{})
scenarios.SetupPagination(25)
scenarios.SetupMultiTenant("tenant-id")
scenarios.SetupBatchOperations()

// Factory helpers
factory := testing.NewMockDBFactory()
factory = testing.SimpleMockFactory(setupFunc)

// Test utilities
testing.NewMockExtendedDB() // With defaults
testing.NewMockExtendedDBStrict() // Without defaults
```

## Best Practices

1. **Use MockExtendedDB**: It implements the full interface with sensible defaults
2. **Factory Pattern**: Use factories for dependency injection
3. **Test Helpers**: Leverage the testing package helpers to reduce boilerplate
4. **Scenarios**: Use CommonScenarios for standard test setups
5. **Assertions**: Always assert that expectations were met

## Migration from MockDB to MockExtendedDB

If you're currently using `MockDB` and encountering interface compatibility issues:

```go
// Before: Using MockDB (implements only core.DB)
mockDB := new(mocks.MockDB)
// This won't work with ExtendedDB interfaces

// After: Using MockExtendedDB (implements core.ExtendedDB)
mockDB := mocks.NewMockExtendedDB()
// This works with all DynamORM interfaces
```

## Troubleshooting

### "MockDB does not implement ExtendedDB"

Use `MockExtendedDB` instead:

```go
mockDB := mocks.NewMockExtendedDB()
```

### "Too many expectations set up"

Use `.Maybe()` for expectations that might be called:

```go
mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
```

### "Unexpected method call"

Check if you need to set up default expectations:

```go
// MockExtendedDB includes defaults for rarely-used methods
mockDB := mocks.NewMockExtendedDB()

// Or set up specific expectations
mockDB.On("AutoMigrateWithOptions", mock.Anything, mock.Anything).Return(nil)
```

## Contributing

We welcome contributions to improve testing support! Areas of interest:

1. Additional test helpers
2. More common scenarios
3. Integration test examples
4. Performance testing utilities

See [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines. 
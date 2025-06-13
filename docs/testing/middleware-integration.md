# Testing Middleware with DynamORM

This guide focuses on testing middleware-based architectures that integrate with DynamORM, such as the Lift framework.

## Overview

When building middleware that uses DynamORM, proper testing requires:
1. Mock implementations that satisfy the full `ExtendedDB` interface
2. Factory pattern for dependency injection
3. Context management for passing database instances
4. Proper test isolation

## The Interface Compatibility Solution

### The Problem

DynamORM exposes two main interfaces:
- `core.DB` - Basic database operations
- `core.ExtendedDB` - Full interface including schema management and Lambda features

The `dynamorm.New()` function returns `ExtendedDB`, so middleware should accept this interface for compatibility.

### The Solution

Use `MockExtendedDB` which implements the full interface:

```go
import "github.com/pay-theory/dynamorm/pkg/mocks"

// Create a mock that implements ExtendedDB
mockDB := mocks.NewMockExtendedDB()
```

## Middleware Testing Pattern

### 1. Define Your Middleware

```go
// middleware/dynamorm.go
package middleware

import (
    "github.com/pay-theory/dynamorm/pkg/core"
    "github.com/pay-theory/dynamorm/pkg/session"
    "github.com/pay-theory/lift/pkg/lift"
)

// DBFactory interface for dependency injection
type DBFactory interface {
    CreateDB(config session.Config) (core.ExtendedDB, error)
}

// WithDynamORM creates middleware that provides DynamORM to handlers
func WithDynamORM(config *Config, factory DBFactory) lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            sessionConfig := session.Config{
                Region:   config.Region,
                Endpoint: config.Endpoint,
            }
            
            db, err := factory.CreateDB(sessionConfig)
            if err != nil {
                return lift.InternalError("Failed to initialize DynamORM").WithCause(err)
            }
            
            // Store in context
            ctx.Set("dynamorm", db)
            
            // Add tenant isolation if enabled
            if config.TenantIsolation {
                tenantID := ctx.TenantID()
                if tenantID == "" {
                    return lift.Unauthorized("Tenant ID required")
                }
                ctx.Set("tenant_id", tenantID)
            }
            
            return next.Handle(ctx)
        })
    }
}

// DB retrieves the DynamORM instance from context
func DB(ctx *lift.Context) (core.ExtendedDB, error) {
    db, exists := ctx.Get("dynamorm").(core.ExtendedDB)
    if !exists {
        return nil, lift.InternalError("DynamORM not initialized")
    }
    return db, nil
}
```

### 2. Test the Middleware

```go
// middleware/dynamorm_test.go
package middleware_test

import (
    "testing"
    "github.com/pay-theory/dynamorm/pkg/mocks"
    "github.com/pay-theory/dynamorm/pkg/testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

func TestWithDynamORM(t *testing.T) {
    // Create mock database
    mockDB := mocks.NewMockExtendedDB()
    
    // Create factory that returns our mock
    factory := &testing.MockDBFactory{
        MockDB: mockDB,
    }
    
    // Create test app with middleware
    app := lift.New()
    app.Use(WithDynamORM(config, factory))
    
    // Add a test handler that uses the database
    app.Get("/test", func(ctx *lift.Context) error {
        db, err := DB(ctx)
        if err != nil {
            return err
        }
        
        // Use the database
        query := db.Model(&User{})
        return query.Create()
    })
    
    // Set up expectations
    mockQuery := new(mocks.MockQuery)
    mockDB.On("Model", &User{}).Return(mockQuery)
    mockQuery.On("Create").Return(nil)
    
    // Make request
    resp := app.Test(httptest.NewRequest("GET", "/test", nil))
    
    // Assert
    assert.Equal(t, 200, resp.StatusCode)
    mockDB.AssertExpectations(t)
    mockQuery.AssertExpectations(t)
}
```

### 3. Test Error Handling

```go
func TestWithDynamORM_ConnectionError(t *testing.T) {
    // Factory that returns an error
    factory := testing.NewMockDBFactory().
        WithError(errors.New("connection failed"))
    
    app := lift.New()
    app.Use(WithDynamORM(config, factory))
    
    app.Get("/test", func(ctx *lift.Context) error {
        // This shouldn't be reached
        t.Fatal("Handler should not be called")
        return nil
    })
    
    resp := app.Test(httptest.NewRequest("GET", "/test", nil))
    
    assert.Equal(t, 500, resp.StatusCode)
    assert.Contains(t, resp.Body.String(), "Failed to initialize DynamORM")
}
```

## Testing Handlers with DynamORM

### 1. Unit Testing Handlers

```go
// handlers/user.go
func CreateUser(ctx *lift.Context) error {
    db, err := middleware.DB(ctx)
    if err != nil {
        return err
    }
    
    var user User
    if err := ctx.Bind(&user); err != nil {
        return lift.BadRequest("Invalid user data")
    }
    
    if err := db.Model(&user).Create(); err != nil {
        return lift.InternalError("Failed to create user").WithCause(err)
    }
    
    return ctx.JSON(201, user)
}

// handlers/user_test.go
func TestCreateUser(t *testing.T) {
    testDB := testing.NewTestDB()
    
    // Set up expectations
    testDB.ExpectModel(&User{}).ExpectCreate()
    
    // Create app with test database
    app := createTestApp(testDB.MockDB)
    
    // Make request
    userJSON := `{"name": "Alice", "email": "alice@example.com"}`
    resp := app.Test(httptest.NewRequest("POST", "/users", 
        strings.NewReader(userJSON)))
    
    // Assert
    assert.Equal(t, 201, resp.StatusCode)
    testDB.AssertExpectations(t)
}

func createTestApp(db core.ExtendedDB) *lift.App {
    factory := &testing.MockDBFactory{MockDB: db}
    
    app := lift.New()
    app.Use(middleware.WithDynamORM(testConfig, factory))
    app.Post("/users", CreateUser)
    
    return app
}
```

### 2. Testing Complex Queries

```go
func TestListUsers(t *testing.T) {
    testDB := testing.NewTestDB()
    
    // Use query chain for complex expectations
    users := []User{
        {ID: "1", Name: "Alice", Status: "active"},
        {ID: "2", Name: "Bob", Status: "active"},
    }
    
    testDB.NewQueryChain().
        Where("Status", "=", "active").
        OrderBy("CreatedAt", "DESC").
        Limit(10).
        ExpectAll(&users)
    
    app := createTestApp(testDB.MockDB)
    resp := app.Test(httptest.NewRequest("GET", "/users?status=active", nil))
    
    assert.Equal(t, 200, resp.StatusCode)
    
    var result []User
    json.Unmarshal(resp.Body.Bytes(), &result)
    assert.Len(t, result, 2)
}
```

## Multi-Tenant Middleware Testing

```go
func TestMultiTenantIsolation(t *testing.T) {
    testDB := testing.NewTestDB()
    scenarios := testing.NewCommonScenarios(testDB)
    
    // Set up multi-tenant scenario
    tenantID := "tenant-123"
    scenarios.SetupMultiTenant(tenantID)
    
    // Create app with tenant middleware
    app := lift.New()
    app.Use(WithTenantID(tenantID))
    app.Use(WithDynamORM(config, &testing.MockDBFactory{MockDB: testDB.MockDB}))
    
    // Handler that queries data
    app.Get("/users", func(ctx *lift.Context) error {
        db, _ := middleware.DB(ctx)
        
        var users []User
        err := db.Model(&users).
            Where("tenant_id", "=", ctx.Get("tenant_id")).
            All(&users)
            
        return ctx.JSON(200, users)
    })
    
    // The mock should expect tenant filtering
    testDB.ExpectModel(&[]User{}).
        ExpectWhere("tenant_id", "=", tenantID).
        ExpectAll(&[]User{{TenantID: tenantID}})
    
    resp := app.Test(httptest.NewRequest("GET", "/users", nil))
    assert.Equal(t, 200, resp.StatusCode)
}
```

## Transaction Middleware Testing

```go
func TestTransactionMiddleware(t *testing.T) {
    testDB := testing.NewTestDB()
    
    // Test automatic transaction wrapping
    testDB.ExpectTransaction(func(tx *core.Tx) {
        // Operations within transaction
        testDB.ExpectModel(&Order{}).ExpectCreate()
        testDB.ExpectModel(&Payment{}).ExpectCreate()
    })
    
    app := createTestApp(testDB.MockDB)
    app.Use(WithAutoTransaction()) // Middleware that wraps writes in transactions
    
    app.Post("/orders", func(ctx *lift.Context) error {
        db, _ := middleware.DB(ctx)
        
        // These should be wrapped in a transaction
        order := &Order{ID: "123"}
        payment := &Payment{OrderID: "123"}
        
        if err := db.Model(order).Create(); err != nil {
            return err
        }
        
        if err := db.Model(payment).Create(); err != nil {
            return err
        }
        
        return ctx.JSON(201, order)
    })
    
    resp := app.Test(httptest.NewRequest("POST", "/orders", orderJSON))
    assert.Equal(t, 201, resp.StatusCode)
    testDB.AssertExpectations(t)
}
```

## Best Practices

### 1. Use Test Helpers

```go
// Create a test suite helper
type TestSuite struct {
    DB      *testing.TestDB
    App     *lift.App
    Factory *testing.MockDBFactory
}

func NewTestSuite() *TestSuite {
    testDB := testing.NewTestDB()
    factory := &testing.MockDBFactory{MockDB: testDB.MockDB}
    
    app := lift.New()
    app.Use(WithDynamORM(testConfig, factory))
    
    return &TestSuite{
        DB:      testDB,
        App:     app,
        Factory: factory,
    }
}

func (s *TestSuite) Reset() {
    s.DB.Reset()
}
```

### 2. Table-Driven Tests

```go
func TestCRUDOperations(t *testing.T) {
    tests := []struct {
        name     string
        method   string
        path     string
        setup    func(*testing.TestDB)
        wantCode int
    }{
        {
            name:   "create user",
            method: "POST",
            path:   "/users",
            setup: func(db *testing.TestDB) {
                db.ExpectModel(&User{}).ExpectCreate()
            },
            wantCode: 201,
        },
        {
            name:   "get user",
            method: "GET",
            path:   "/users/123",
            setup: func(db *testing.TestDB) {
                db.ExpectModel(&User{}).
                    ExpectWhere("ID", "=", "123").
                    ExpectFind(&User{ID: "123"})
            },
            wantCode: 200,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            suite := NewTestSuite()
            defer suite.Reset()
            
            tt.setup(suite.DB)
            
            req := httptest.NewRequest(tt.method, tt.path, nil)
            resp := suite.App.Test(req)
            
            assert.Equal(t, tt.wantCode, resp.StatusCode)
            suite.DB.AssertExpectations(t)
        })
    }
}
```

### 3. Integration Test Pattern

```go
func TestIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }
    
    // Use real DynamoDB Local for integration tests
    factory := &DefaultDBFactory{}
    
    app := lift.New()
    app.Use(WithDynamORM(integrationConfig, factory))
    
    // Run integration tests against real database
    // ...
}
```

## Common Pitfalls and Solutions

### 1. Interface Mismatch

**Problem**: `MockDB` doesn't implement `ExtendedDB`

**Solution**: Use `MockExtendedDB`:
```go
mockDB := mocks.NewMockExtendedDB()
```

### 2. Missing Context Setup

**Problem**: Handler can't find database in context

**Solution**: Ensure middleware runs before handler:
```go
app.Use(WithDynamORM(config, factory)) // Must come first
app.Get("/users", GetUsers)            // Handler comes after
```

### 3. Expectation Order

**Problem**: Mock expectations fail due to call order

**Solution**: Use `.Maybe()` for flexible ordering:
```go
mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
```

### 4. Transaction Testing

**Problem**: Complex transaction scenarios

**Solution**: Use transaction helpers:
```go
testDB.ExpectTransaction(func(tx *core.Tx) {
    // Set up expectations for operations in transaction
})
```

## Summary

Testing middleware with DynamORM requires:
1. Using `MockExtendedDB` for full interface compatibility
2. Factory pattern for clean dependency injection
3. Proper context management
4. Comprehensive test helpers from `pkg/testing`

The combination of these patterns enables thorough testing of middleware-based architectures while maintaining type safety and test clarity. 
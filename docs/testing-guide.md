# Testing Guide

This guide explains how to write unit and integration tests for applications using DynamORM.

## Unit Testing with Mocks

To write unit tests without connecting to DynamoDB, use the `core.DB` interface and the provided mocks.

### 1. Define Dependencies via Interface

Don't depend on the concrete `*dynamorm.DB` struct. Use `core.DB`.

```go
import "github.com/pay-theory/dynamorm/pkg/core"

type UserService struct {
    db core.DB
}

func NewUserService(db core.DB) *UserService {
    return &UserService{db: db}
}
```

### 2. Use Mocks in Tests

DynamORM provides mocks in the `mocks` package (or generate your own with mockery).

```go
import (
    "testing"
    "github.com/stretchr/testify/mock"
    "github.com/pay-theory/dynamorm/pkg/mocks"
)

func TestCreateUser(t *testing.T) {
    // Setup Mocks
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)
    
    // Expect Model() to be called, return mock query
    mockDB.On("Model", mock.Anything).Return(mockQuery)
    
    // Expect Create() to be called
    mockQuery.On("Create").Return(nil)
    
    // Test Service
    service := NewUserService(mockDB)
    err := service.CreateUser("john")
    
    // Assertions
    if err != nil {
        t.Errorf("Expected no error, got %v", err)
    }
    mockDB.AssertExpectations(t)
}
```

## Integration Testing

For integration tests, connect to a real DynamoDB instance or DynamoDB Local.

```go
func TestIntegration(t *testing.T) {
    // Connect to DynamoDB Local
    db, _ := dynamorm.New(session.Config{
        Endpoint: "http://localhost:8000",
        Region:   "us-east-1",
    })
    
    // Create Table
    db.CreateTable(&User{})
    
    // Run Test
    err := db.Model(&User{ID: "1"}).Create()
    if err != nil {
        t.Fatal(err)
    }
}
```

# Testing Example with DynamORM Interfaces

This example demonstrates how to use DynamORM's new interface-based approach for better testability.

## Service Using Interface

```go
package service

import (
    "github.com/pay-theory/dynamorm/pkg/core"
)

// UserService demonstrates dependency injection with interfaces
type UserService struct {
    db core.DB // Using interface instead of concrete type
}

// NewUserService creates a new service with dependency injection
func NewUserService(db core.DB) *UserService {
    return &UserService{db: db}
}

// GetUser retrieves a user by ID - easily testable!
func (s *UserService) GetUser(id string) (*User, error) {
    var user User
    err := s.db.Model(&User{}).Where("ID", "=", id).First(&user)
    if err != nil {
        return nil, err
    }
    return &user, nil
}

// CreateUser creates a new user
func (s *UserService) CreateUser(name, email string) (*User, error) {
    user := &User{
        ID:    generateID(),
        Name:  name,
        Email: email,
    }
    
    if err := s.db.Model(user).Create(); err != nil {
        return nil, err
    }
    
    return user, nil
}
```

## Unit Test with Mock

```go
package service_test

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/pay-theory/dynamorm/pkg/core"
)

// MockDB implements core.DB for testing
type MockDB struct {
    mock.Mock
}

func (m *MockDB) Model(model any) core.Query {
    args := m.Called(model)
    return args.Get(0).(core.Query)
}

// ... implement other DB methods ...

// MockQuery implements core.Query for testing
type MockQuery struct {
    mock.Mock  
}

func (m *MockQuery) Where(field string, op string, value any) core.Query {
    args := m.Called(field, op, value)
    return args.Get(0).(core.Query)
}

func (m *MockQuery) First(dest any) error {
    args := m.Called(dest)
    // Fill in test data
    if user, ok := dest.(*User); ok {
        user.ID = "123"
        user.Name = "Test User"
        user.Email = "test@example.com"
    }
    return args.Error(0)
}

func (m *MockQuery) Create() error {
    args := m.Called()
    return args.Error(0)
}

// ... implement other Query methods ...

func TestUserService_GetUser(t *testing.T) {
    // Arrange
    mockDB := new(MockDB)
    mockQuery := new(MockQuery)
    
    mockDB.On("Model", &User{}).Return(mockQuery)
    mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
    mockQuery.On("First", mock.Anything).Return(nil)
    
    service := NewUserService(mockDB)
    
    // Act
    user, err := service.GetUser("123")
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, "123", user.ID)
    assert.Equal(t, "Test User", user.Name)
    
    mockDB.AssertExpectations(t)
    mockQuery.AssertExpectations(t)
}
```

## Application Setup

```go
package main

import (
    "log"
    "github.com/pay-theory/dynamorm"
    "myapp/service"
)

func main() {
    // Use NewBasic for cleaner interface when you don't need schema operations
    db, err := dynamorm.NewBasic(dynamorm.Config{
        Region: "us-east-1",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Create service with dependency injection
    userService := service.NewUserService(db)
    
    // Use the service...
    user, err := userService.GetUser("123")
    if err != nil {
        log.Printf("Error: %v", err)
    }
}
```

## Benefits

1. **Testable**: Services can be unit tested without a real database
2. **Flexible**: Easy to swap implementations 
3. **Clean**: Clear separation of concerns
4. **Type-safe**: Still get all of Go's type safety

## When to Use Each Interface

- **`core.DB`**: For most application code that just needs CRUD operations
- **`core.ExtendedDB`**: For admin tools or setup code that needs schema operations

Both interfaces support the full query API, so your application code doesn't change! 
# DynamORM Testing Guide

## Prerequisites

1. **Docker**: Required for DynamoDB Local
2. **Go 1.19+**: Required for generics support
3. **Make**: For convenient test commands

## Quick Start

```bash
# Start DynamoDB Local
make docker-up

# Run all tests
make test-all

# Stop DynamoDB Local
make docker-down
```

## Test Categories

### Unit Tests
Tests for individual components without external dependencies.

```bash
# Run all unit tests
make test-unit

# Run specific package tests
go test ./pkg/query/...
go test ./pkg/index/...
go test ./internal/expr/...
```

### Integration Tests
End-to-end tests with DynamoDB Local.

```bash
# Run integration tests (starts DynamoDB Local automatically)
make integration

# Run integration tests manually
docker-compose up -d
go test ./tests/integration/... -v
docker-compose down
```

### Performance Benchmarks
Measure performance and verify < 5% overhead target.

```bash
# Run all benchmarks
make benchmark

# Run specific benchmarks
go test -bench=BenchmarkSimpleQuery ./tests/benchmarks/...
go test -bench=BenchmarkOverheadComparison ./tests/benchmarks/...

# Run benchmarks with memory profiling
go test -bench=. -benchmem ./tests/benchmarks/...
```

### Stress Tests
Test system under heavy load and with large items.

```bash
# Run stress tests
make stress

# Run specific stress tests
go test -run TestConcurrentQueries ./tests/stress/... -v
go test -run TestLargeItemHandling ./tests/stress/... -v
go test -run TestMemoryStability ./tests/stress/... -v

# Skip long-running tests
go test -short ./tests/stress/...
```

## Test Coverage

```bash
# Generate coverage report
make coverage

# View coverage in terminal
go test -cover ./...

# Generate detailed coverage by function
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

## Debugging Tests

### Verbose Output
```bash
# Run with verbose output
go test -v ./tests/integration/...
```

### Run Specific Tests
```bash
# Run a specific test function
go test -run TestComplexQueryWithIndexSelection ./tests/integration/... -v

# Run tests matching a pattern
go test -run ".*Pagination.*" ./tests/integration/... -v
```

### Race Detection
```bash
# Run with race detector
go test -race ./...
make test-race
```

## Team-Specific Tests

### Team 1 (Core Foundation)
```bash
make team1-test
# Or manually:
go test -v ./pkg/core/... ./pkg/model/... ./pkg/types/... ./pkg/session/...
```

### Team 2 (Query Builder)
```bash
make team2-test
# Or manually:
go test -v ./pkg/query/... ./internal/expr/... ./pkg/index/...
```

## Continuous Integration

For CI/CD pipelines:

```bash
# Full test suite suitable for CI
make docker-up
go test -race -coverprofile=coverage.out ./...
go test -tags=integration ./tests/integration/...
go test -bench=. -benchmem ./tests/benchmarks/...
make docker-down
```

## Common Issues

### DynamoDB Local Not Starting
```bash
# Check if port 8000 is already in use
lsof -i :8000

# Force recreate containers
docker-compose down -v
docker-compose up -d
```

### Test Timeouts
```bash
# Increase test timeout
go test -timeout 30m ./tests/stress/...
```

### Clean Test State
```bash
# Remove test artifacts
make clean
docker-compose down -v
```

## Performance Testing Tips

1. **Warm Up**: Run benchmarks multiple times to warm up caches
2. **Isolation**: Close other applications to reduce noise
3. **Consistency**: Use the same machine/environment for comparisons
4. **Multiple Runs**: Use `-count=10` to run benchmarks multiple times

```bash
# Example: Rigorous benchmark
go test -bench=BenchmarkSimpleQuery -benchmem -count=10 -benchtime=10s ./tests/benchmarks/...
```

## Test Data

Test models are defined in `tests/models/test_models.go`:
- `TestUser`: User model with various field types
- `TestProduct`: Product model with GSI
- `TestOrder`: Complex model with nested types

## Writing New Tests

### Integration Test Template
```go
func (s *QueryIntegrationSuite) TestYourFeature() {
    // Arrange
    user := models.TestUser{
        ID:     "test-1",
        Email:  "test@example.com",
        Status: "active",
    }
    err := s.db.Model(&user).Create()
    s.NoError(err)
    
    // Act
    var result models.TestUser
    err = s.db.Model(&models.TestUser{}).
        Where("ID", "=", "test-1").
        First(&result)
    
    // Assert
    s.NoError(err)
    s.Equal(user.Email, result.Email)
}
```

### Benchmark Template
```go
func BenchmarkYourOperation(b *testing.B) {
    db, _ := setupBenchDB(b)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // Your operation here
    }
}
```

## Makefile Targets Reference

| Target | Description |
|--------|-------------|
| `make test` | Run unit tests |
| `make integration` | Run integration tests |
| `make benchmark` | Run benchmarks |
| `make stress` | Run stress tests |
| `make test-all` | Run all tests |
| `make coverage` | Generate coverage report |
| `make docker-up` | Start DynamoDB Local |
| `make docker-down` | Stop DynamoDB Local |
| `make lint` | Run linters |
| `make fmt` | Format code |

# Testing with DynamORM

This guide explains how to write testable code with DynamORM using interfaces and mocking.

## The Problem

Previously, DynamORM's `New()` function returned a concrete `*dynamorm.DB` type, making it impossible to mock for unit testing. This forced developers to use integration tests with real DynamoDB instances.

## The Solution

DynamORM now provides interfaces that make mocking straightforward:

- **`core.DB`** - Basic interface with core functionality
- **`core.ExtendedDB`** - Full interface with all features including schema management

## Using Interfaces for Testability

### 1. Basic Usage (Recommended for Most Cases)

For most applications, use the `core.DB` interface:

```go
package service

import (
    "github.com/pay-theory/dynamorm/pkg/core"
)

type UserService struct {
    db core.DB  // Use interface, not concrete type
}

func NewUserService(db core.DB) *UserService {
    return &UserService{db: db}
}

func (s *UserService) GetUser(id string) (*User, error) {
    var user User
    err := s.db.Model(&User{}).Where("ID", "=", id).First(&user)
    if err != nil {
        return nil, err
    }
    return &user, nil
}
```

In your main application:

```go
func main() {
    // Use NewBasic for cleaner interface
    db, err := dynamorm.NewBasic(dynamorm.Config{
        Region: "us-east-1",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    userService := service.NewUserService(db)
    // ... use the service
}
```

### 2. Extended Features

If you need schema management or Lambda features, use `core.ExtendedDB`:

```go
type AdminService struct {
    db core.ExtendedDB  // Extended interface
}

func (s *AdminService) CreateUserTable() error {
    return s.db.CreateTable(&User{})
}
```

### 3. Writing Unit Tests with Mocks

Now you can easily mock the database for unit tests:

```go
package service_test

import (
    "testing"
    "github.com/stretchr/testify/mock"
    "github.com/pay-theory/dynamorm/pkg/core"
)

// MockDB implements core.DB interface
type MockDB struct {
    mock.Mock
}

func (m *MockDB) Model(model any) core.Query {
    args := m.Called(model)
    return args.Get(0).(core.Query)
}

func (m *MockDB) Transaction(fn func(tx *core.Tx) error) error {
    args := m.Called(fn)
    return args.Error(0)
}

// ... implement other methods

// MockQuery implements core.Query interface
type MockQuery struct {
    mock.Mock
}

func (m *MockQuery) Where(field string, op string, value any) core.Query {
    args := m.Called(field, op, value)
    return args.Get(0).(core.Query)
}

func (m *MockQuery) First(dest any) error {
    args := m.Called(dest)
    // Populate the destination with test data
    if user, ok := dest.(*User); ok {
        user.ID = "123"
        user.Name = "Test User"
    }
    return args.Error(0)
}

// ... implement other methods

func TestUserService_GetUser(t *testing.T) {
    // Create mocks
    mockDB := new(MockDB)
    mockQuery := new(MockQuery)
    
    // Setup expectations
    mockDB.On("Model", &User{}).Return(mockQuery)
    mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
    mockQuery.On("First", mock.Anything).Return(nil)
    
    // Create service with mock
    service := NewUserService(mockDB)
    
    // Test
    user, err := service.GetUser("123")
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, "123", user.ID)
    assert.Equal(t, "Test User", user.Name)
    
    // Verify expectations
    mockDB.AssertExpectations(t)
    mockQuery.AssertExpectations(t)
}
```

## Using Pre-built Mocks

DynamORM provides pre-built mock implementations in the test package:

```go
import "github.com/pay-theory/dynamorm/pkg/core"

func TestWithPrebuiltMocks(t *testing.T) {
    // The core package includes MockDB and MockQuery in its test files
    // You can copy these or create your own
}
```

## Best Practices

1. **Always use interfaces** in your service/repository layers:
   ```go
   type Repository struct {
       db core.DB  // ✅ Good - uses interface
   }
   
   // Not:
   type Repository struct {
       db *dynamorm.DB  // ❌ Bad - uses concrete type
   }
   ```

2. **Choose the right interface**:
   - Use `core.DB` for most cases
   - Use `core.ExtendedDB` only when you need schema management

3. **Inject dependencies**:
   ```go
   // ✅ Good - dependency injection
   func NewService(db core.DB) *Service {
       return &Service{db: db}
   }
   
   // ❌ Bad - creates dependency internally
   func NewService() *Service {
       db, _ := dynamorm.New(config)
       return &Service{db: db}
   }
   ```

4. **Mock at the right level**:
   - Mock the `DB` interface for repository tests
   - Mock your repository interface for service tests
   - Use integration tests for complex query logic

## Integration Tests

For integration tests, you can still use the concrete implementation:

```go
| `make fmt` | Format code | 
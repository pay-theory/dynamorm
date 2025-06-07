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
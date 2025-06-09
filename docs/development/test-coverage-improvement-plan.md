# DynamORM Test Coverage Improvement Plan

## Executive Summary

Current test coverage (excluding schema & examples): **~30.8%**
Target test coverage: **75%+ overall**
Timeline: **7 weeks**
Priority: **Critical packages first (types, marshal)**

## Current State Analysis

### Well-Tested Packages (Keep Momentum)
- `pkg/model`: 76.7% coverage ✓
- `pkg/transaction`: 74.5% coverage ✓
- `internal/expr`: 35.3% coverage (needs improvement)

### Untested Packages (0% Coverage)
1. **pkg/types** (491 lines) - Critical: Type conversion
2. **pkg/marshal** (392 lines) - Critical: DynamoDB marshaling  
3. **pkg/core** (266 lines) - Important: Core interfaces
4. **pkg/index** (151 lines) - Important: Index selection
5. **pkg/session** (136 lines) - Important: Session management
6. **pkg/errors** (112 lines) - Foundation: Error handling

### Broken/Failing
- `pkg/query`: Build failures
- Root package: Test failures (AWS SDK issues)

## Implementation Plan

### Phase 1: Foundation (Weeks 1-2)
**Goal: Fix broken tests and establish testing infrastructure**

#### Week 1: Fix Failing Tests
- [ ] Fix `pkg/query` build issues
- [ ] Fix root package test failures
- [ ] Setup mock DynamoDB client for unit tests
- [ ] Create test fixtures and helpers

#### Week 2: Test Foundation Package
- [ ] **pkg/errors** (Target: 90% coverage)
  - Test all error types
  - Test error wrapping/unwrapping
  - Test error checking functions
  - Estimated effort: 2-3 days

### Phase 2: Critical Packages (Weeks 3-4)
**Goal: Test the most critical packages for ORM functionality**

#### Week 3: Type System
- [ ] **pkg/types** (Target: 80% coverage)
  - Test all type conversions (Go ↔ DynamoDB)
  - Test custom converters
  - Test edge cases (nil, zero values)
  - Test set types (SS, NS, BS)
  - Estimated effort: 5 days

#### Week 4: Marshaling
- [ ] **pkg/marshal** (Target: 80% coverage)
  - Test struct marshaling
  - Test performance optimizations
  - Test caching mechanisms
  - Test unsafe pointer operations
  - Estimated effort: 5 days

### Phase 3: Core & Integration (Weeks 5-6)
**Goal: Complete core package testing and improve integration**

#### Week 5: Core Packages
- [ ] **pkg/core** (Target: 85% coverage)
  - Test interfaces
  - Test model registration
  - Mock implementations
  - Estimated effort: 3 days
  
- [ ] **pkg/index** (Target: 80% coverage)
  - Test index selection logic
  - Test GSI/LSI handling
  - Estimated effort: 2 days

#### Week 6: Session & Expression Builder
- [ ] **pkg/session** (Target: 80% coverage)
  - Test session management
  - Test transaction support
  - Estimated effort: 2 days
  
- [ ] **internal/expr** (From 35.3% to 70%)
  - Test expression builder
  - Test complex queries
  - Test reserved words
  - Estimated effort: 3 days

### Phase 4: Polish & Integration (Week 7)
**Goal: End-to-end testing and documentation**

- [ ] Integration test suite
- [ ] Performance benchmarks
- [ ] Test coverage reporting
- [ ] Documentation updates

## Testing Strategy

### Unit Testing Approach
```go
// 1. Use table-driven tests
func TestTypeConversion(t *testing.T) {
    tests := []struct {
        name     string
        input    interface{}
        expected types.AttributeValue
        wantErr  bool
    }{
        // Test cases...
    }
}

// 2. Mock DynamoDB client
type mockDynamoDBClient struct {
    mock.Mock
}

// 3. Test helpers
func setupTestDB(t *testing.T) *DB {
    // Setup code
}
```

### Coverage Goals by Package
| Package | Current | Target | Priority |
|---------|---------|--------|----------|
| pkg/types | 0% | 80% | Critical |
| pkg/marshal | 0% | 80% | Critical |
| pkg/core | 0% | 85% | High |
| pkg/errors | 0% | 90% | High |
| pkg/index | 0% | 80% | Medium |
| pkg/session | 0% | 80% | Medium |
| internal/expr | 35.3% | 70% | Medium |
| pkg/query | Failed | 70% | High |

## Resource Requirements

### Team Assignment
Based on Makefile team structure:
- **Team 1**: Focus on pkg/core, pkg/model, pkg/types, pkg/session, pkg/errors
- **Team 2**: Focus on pkg/query, internal/expr, pkg/index

### Tooling
- Use `mockgen` for generating mocks
- Use `testify` for assertions
- Use `go test -cover` for coverage tracking
- Setup GitHub Actions for CI/CD

## Success Metrics

1. **Coverage Targets**
   - Overall: 75%+ (from ~30.8%)
   - Critical packages: 80%+
   - Zero packages with 0% coverage

2. **Quality Metrics**
   - All tests pass reliably
   - Tests run in < 30 seconds
   - No flaky tests

3. **Maintainability**
   - Clear test patterns established
   - Mock infrastructure in place
   - Documentation updated

## Risk Mitigation

1. **Failing Tests**: Fix immediately in Phase 1
2. **Complex Packages**: Start with simpler test cases
3. **Time Constraints**: Focus on critical packages first
4. **Technical Debt**: Refactor as needed during testing

## Next Steps

1. Create GitHub issues for each package
2. Assign team members
3. Setup daily progress tracking
4. Weekly coverage reviews

## Commands for Tracking Progress

```bash
# Run all tests with coverage
make test

# Check specific package coverage
go test -cover ./pkg/types

# Generate coverage report
make coverage

# Team-specific tests
make team1-test
make team2-test

# Continuous monitoring
watch -n 10 'go test -cover ./... | grep coverage'
``` 
# Team 1 Test Coverage Implementation Guide

## Your Mission
Improve test coverage for your assigned packages from 0% to target levels within 4 weeks.

## Assigned Packages & Current Status

| Package | Current Coverage | Target | Priority | Lines to Test |
|---------|-----------------|--------|----------|---------------|
| pkg/errors | 0% | 90% | HIGH (Week 1) | 112 lines |
| pkg/types | 0% | 80% | CRITICAL (Week 2) | 491 lines |
| pkg/core | 0% | 85% | HIGH (Week 3) | 266 lines |
| pkg/session | 0% | 80% | MEDIUM (Week 4) | 136 lines |
| pkg/model | 76.7% | Maintain | - | Already tested |

## Week 1: pkg/errors (Target: 90% coverage)

### Test Implementation Checklist

1. **Error Types Testing**
   ```go
   // Test all predefined errors
   - ErrItemNotFound
   - ErrInvalidModel
   - ErrMissingPrimaryKey
   - ErrInvalidPrimaryKey
   - ErrConditionFailed
   - ErrIndexNotFound
   - ErrTransactionFailed
   - ErrBatchOperationFailed
   - ErrUnsupportedType
   - ErrInvalidTag
   - ErrTableNotFound
   - ErrDuplicatePrimaryKey
   - ErrEmptyValue
   - ErrInvalidOperator
   ```

2. **DynamORMError Testing**
   - Test Error() method with and without context
   - Test Unwrap() method
   - Test Is() method with various error types
   - Test NewError() constructor
   - Test NewErrorWithContext() constructor

3. **Error Checking Functions**
   - Test IsNotFound() with various error types
   - Test IsInvalidModel() with wrapped errors
   - Test IsConditionFailed() with nested errors

### Example Test Structure
```go
func TestErrorTypes(t *testing.T) {
    tests := []struct {
        name     string
        err      error
        expected string
    }{
        {
            name:     "ErrItemNotFound",
            err:      errors.ErrItemNotFound,
            expected: "item not found",
        },
        // Add all error types...
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            assert.Error(t, tt.err)
            assert.Contains(t, tt.err.Error(), tt.expected)
        })
    }
}

func TestDynamORMError_Wrapping(t *testing.T) {
    baseErr := errors.ErrItemNotFound
    wrapped := errors.NewError("GetItem", "User", baseErr)
    
    assert.ErrorIs(t, wrapped, baseErr)
    assert.True(t, errors.IsNotFound(wrapped))
    
    // Test double wrapping
    doubleWrapped := fmt.Errorf("operation failed: %w", wrapped)
    assert.ErrorIs(t, doubleWrapped, baseErr)
}
```

## Week 2: pkg/types (Target: 80% coverage)

### Critical Functions to Test

1. **Type Converter (ToAttributeValue)**
   - Basic types: string, int, float, bool
   - Numeric types: int8/16/32/64, uint8/16/32/64
   - Complex types: slices, maps, structs
   - Special types: time.Time, []byte
   - Nil and pointer handling
   - Custom converters

2. **Type Converter (FromAttributeValue)**
   - All DynamoDB types: S, N, BOOL, B, L, M, SS, NS, BS, NULL
   - Type mismatches and error cases
   - Nested structures
   - Zero values and nil handling

3. **Set Conversions**
   - String sets (SS)
   - Number sets (NS)
   - Binary sets (BS)
   - Empty sets handling

### Test Scenarios
```go
// Basic type conversions
func TestConverter_BasicTypes(t *testing.T) {
    converter := types.NewConverter()
    
    tests := []struct {
        name  string
        input interface{}
        want  types.AttributeValue
    }{
        // Strings
        {"empty string", "", &types.AttributeValueMemberS{Value: ""}},
        {"unicode", "Hello 世界", &types.AttributeValueMemberS{Value: "Hello 世界"}},
        
        // Numbers
        {"zero int", 0, &types.AttributeValueMemberN{Value: "0"}},
        {"negative int", -42, &types.AttributeValueMemberN{Value: "-42"}},
        {"max int64", int64(9223372036854775807), &types.AttributeValueMemberN{Value: "9223372036854775807"}},
        
        // Floats
        {"float with decimals", 3.14159, &types.AttributeValueMemberN{Value: "3.14159"}},
        {"scientific notation", 1.23e-10, &types.AttributeValueMemberN{Value: "0.000000000123"}},
        
        // Edge cases
        {"nil", nil, &types.AttributeValueMemberNULL{Value: true}},
        {"nil pointer", (*string)(nil), &types.AttributeValueMemberNULL{Value: true}},
    }
}

// Complex type conversions
func TestConverter_ComplexTypes(t *testing.T) {
    // Test nested structs
    type Address struct {
        Street string
        City   string
    }
    type User struct {
        Name    string
        Age     int
        Address Address
        Tags    []string
    }
    
    // Test with populated data
    // Test with zero values
    // Test with nil slices/maps
}
```

## Week 3: pkg/core (Target: 85% coverage)

### Focus Areas

1. **Interfaces Testing**
   - Create mock implementations
   - Test interface contracts
   - Verify method signatures

2. **Core Types**
   - Model interface implementation
   - CRUD operation interfaces
   - Configuration validation

### Mock Implementation Example
```go
type MockModel struct {
    mock.Mock
}

func (m *MockModel) TableName() string {
    args := m.Called()
    return args.String(0)
}

func (m *MockModel) PrimaryKey() (string, interface{}) {
    args := m.Called()
    return args.String(0), args.Get(1)
}

// Test interface compliance
func TestModelInterface(t *testing.T) {
    var _ core.Model = (*MockModel)(nil) // Compile-time check
    
    mockModel := new(MockModel)
    mockModel.On("TableName").Return("users")
    mockModel.On("PrimaryKey").Return("id", "user123")
    
    // Test usage
    table := mockModel.TableName()
    assert.Equal(t, "users", table)
    
    mockModel.AssertExpectations(t)
}
```

## Week 4: pkg/session (Target: 80% coverage)

### Test Requirements

1. **Session Management**
   - Session creation and configuration
   - Context handling
   - Transaction support
   - Error propagation

2. **Configuration Options**
   - Timeout settings
   - Retry policies
   - Custom endpoints

### Integration Points
```go
func TestSession_Creation(t *testing.T) {
    tests := []struct {
        name    string
        config  session.Config
        wantErr bool
    }{
        {
            name: "valid config",
            config: session.Config{
                Region:   "us-east-1",
                Endpoint: "http://localhost:8000",
            },
            wantErr: false,
        },
        {
            name: "missing region",
            config: session.Config{
                Endpoint: "http://localhost:8000",
            },
            wantErr: true,
        },
    }
}
```

## Testing Best Practices

1. **Use the provided test templates** (see `docs/development/test-templates.md`)

2. **Follow table-driven test pattern**
   - Group related test cases
   - Use descriptive test names
   - Cover edge cases

3. **Test error paths thoroughly**
   - Invalid inputs
   - Nil/empty values
   - Type mismatches

4. **Benchmark critical paths**
   ```go
   func BenchmarkTypeConversion(b *testing.B) {
       converter := types.NewConverter()
       data := generateLargeStruct()
       
       b.ResetTimer()
       for i := 0; i < b.N; i++ {
           _, _ = converter.ToAttributeValue(data)
       }
   }
   ```

5. **Mock external dependencies**
   - Use interfaces for testability
   - Create test doubles for DynamoDB client
   - Isolate unit tests from integration tests

## Progress Tracking

Run daily to track your progress:
```bash
# Check your team's coverage
make team1-test

# View coverage for specific package
go test -cover ./pkg/errors

# Generate detailed HTML report
go test -coverprofile=coverage.out ./pkg/errors && go tool cover -html=coverage.out

# Run the coverage dashboard
make coverage-dashboard
```

## Common Pitfalls to Avoid

1. **Don't skip error cases** - They often reveal bugs
2. **Test zero values** - Go's zero values can cause issues
3. **Test concurrent access** - Use race detector: `go test -race`
4. **Don't ignore helper functions** - They need coverage too
5. **Avoid test interdependencies** - Each test should be independent

## Questions to Answer in Your Tests

For each package, ensure your tests answer:
1. What happens with nil/empty inputs?
2. How are errors propagated?
3. Are all code paths covered?
4. Is the API intuitive and well-documented?
5. Are there any race conditions?

## Deliverables

By end of each week:
1. Tests achieving target coverage
2. All tests passing reliably
3. No reduction in existing coverage
4. Benchmarks for performance-critical code
5. Updated documentation if needed

## Getting Help

- Review test templates: `docs/development/test-templates.md`
- Check existing good examples: `pkg/model/*_test.go`
- Run `make help` for available commands
- Coordinate with Team 2 on shared interfaces

Remember: Quality over quantity. Well-written tests that cover important scenarios are better than many superficial tests. 
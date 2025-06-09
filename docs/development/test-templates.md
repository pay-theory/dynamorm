# DynamORM Test Templates

## Quick Start Templates for Test Development

### 1. Basic Package Test Template

```go
package packagename_test

import (
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/pay-theory/dynamorm/pkg/packagename"
)

func TestBasicFunctionality(t *testing.T) {
    // Arrange
    
    // Act
    
    // Assert
}
```

### 2. Table-Driven Test Template (Recommended)

```go
func TestTypeConversion(t *testing.T) {
    tests := []struct {
        name    string
        input   interface{}
        want    interface{}
        wantErr bool
    }{
        {
            name:    "convert string",
            input:   "hello",
            want:    &types.AttributeValueMemberS{Value: "hello"},
            wantErr: false,
        },
        {
            name:    "convert nil",
            input:   nil,
            want:    &types.AttributeValueMemberNULL{Value: true},
            wantErr: false,
        },
        // Add more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
            got, err := functionUnderTest(tt.input)
            
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            
            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### 3. Error Testing Template

```go
func TestErrorHandling(t *testing.T) {
    tests := []struct {
        name          string
        setupFunc     func() error
        expectedError error
        checkError    func(t *testing.T, err error)
    }{
        {
            name: "returns ErrItemNotFound",
            setupFunc: func() error {
                return errors.ErrItemNotFound
            },
            expectedError: errors.ErrItemNotFound,
            checkError: func(t *testing.T, err error) {
                assert.True(t, errors.IsNotFound(err))
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.setupFunc()
            
            if tt.expectedError != nil {
                assert.ErrorIs(t, err, tt.expectedError)
            }
            
            if tt.checkError != nil {
                tt.checkError(t, err)
            }
        })
    }
}
```

### 4. Mock DynamoDB Client Template

```go
package packagename_test

import (
    "context"
    "testing"
    
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
    "github.com/stretchr/testify/mock"
)

// MockDynamoDBClient mocks the DynamoDB client
type MockDynamoDBClient struct {
    mock.Mock
}

func (m *MockDynamoDBClient) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
    args := m.Called(ctx, params)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*dynamodb.GetItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
    args := m.Called(ctx, params)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*dynamodb.PutItemOutput), args.Error(1)
}

// Usage in tests
func TestWithMockDB(t *testing.T) {
    mockDB := new(MockDynamoDBClient)
    
    // Setup expectations
    mockDB.On("GetItem", mock.Anything, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
        return *input.TableName == "TestTable"
    })).Return(&dynamodb.GetItemOutput{
        Item: map[string]types.AttributeValue{
            "id": &types.AttributeValueMemberS{Value: "123"},
        },
    }, nil)
    
    // Use mock in test
    // ...
    
    // Assert expectations
    mockDB.AssertExpectations(t)
}
```

### 5. Benchmark Template

```go
func BenchmarkTypeConversion(b *testing.B) {
    converter := types.NewConverter()
    testData := generateTestData()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = converter.ToAttributeValue(testData)
    }
}

func BenchmarkMarshal(b *testing.B) {
    benchmarks := []struct {
        name string
        size int
    }{
        {"Small", 10},
        {"Medium", 100},
        {"Large", 1000},
    }
    
    for _, bm := range benchmarks {
        b.Run(bm.name, func(b *testing.B) {
            data := generateDataOfSize(bm.size)
            marshaler := marshal.New()
            
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                _, _ = marshaler.MarshalItem(data, metadata)
            }
        })
    }
}
```

### 6. Test Helpers

```go
// test_helpers.go
package testutil

import (
    "testing"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// MustAttributeValue creates an AttributeValue or fails the test
func MustAttributeValue(t *testing.T, v interface{}) types.AttributeValue {
    t.Helper()
    av, err := convertToAttributeValue(v)
    if err != nil {
        t.Fatalf("failed to create AttributeValue: %v", err)
    }
    return av
}

// AssertAttributeValueEqual asserts two AttributeValues are equal
func AssertAttributeValueEqual(t *testing.T, expected, actual types.AttributeValue) {
    t.Helper()
    // Implementation
}

// CreateTestItem creates a test DynamoDB item
func CreateTestItem(id string, attributes map[string]interface{}) map[string]types.AttributeValue {
    item := make(map[string]types.AttributeValue)
    item["id"] = &types.AttributeValueMemberS{Value: id}
    
    for k, v := range attributes {
        // Convert and add attributes
    }
    
    return item
}
```

### 7. Integration Test Template

```go
// +build integration

package integration_test

import (
    "context"
    "testing"
    "time"
    
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func TestIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    
    // Setup
    ctx := context.Background()
    cfg, err := config.LoadDefaultConfig(ctx,
        config.WithRegion("us-east-1"),
        config.WithEndpointResolver(/* local endpoint */),
    )
    require.NoError(t, err)
    
    client := dynamodb.NewFromConfig(cfg)
    
    // Create table
    tableName := fmt.Sprintf("test_%d", time.Now().Unix())
    defer cleanupTable(t, client, tableName)
    
    // Run tests
    t.Run("CreateItem", func(t *testing.T) {
        // Test implementation
    })
}
```

## Package-Specific Templates

### For pkg/types

```go
func TestConverter_ToAttributeValue(t *testing.T) {
    converter := types.NewConverter()
    
    tests := []struct {
        name  string
        input interface{}
        want  types.AttributeValue
    }{
        // Basic types
        {"string", "hello", &types.AttributeValueMemberS{Value: "hello"}},
        {"int", 42, &types.AttributeValueMemberN{Value: "42"}},
        {"bool", true, &types.AttributeValueMemberBOOL{Value: true}},
        
        // Complex types
        {"slice", []string{"a", "b"}, &types.AttributeValueMemberL{
            Value: []types.AttributeValue{
                &types.AttributeValueMemberS{Value: "a"},
                &types.AttributeValueMemberS{Value: "b"},
            },
        }},
        
        // Edge cases
        {"nil", nil, &types.AttributeValueMemberNULL{Value: true}},
        {"empty string", "", &types.AttributeValueMemberS{Value: ""}},
        {"zero int", 0, &types.AttributeValueMemberN{Value: "0"}},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := converter.ToAttributeValue(tt.input)
            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### For pkg/errors

```go
func TestDynamORMError(t *testing.T) {
    t.Run("Error formatting", func(t *testing.T) {
        err := errors.NewError("GetItem", "User", errors.ErrItemNotFound)
        assert.Contains(t, err.Error(), "GetItem")
        assert.Contains(t, err.Error(), "User")
        assert.Contains(t, err.Error(), "item not found")
    })
    
    t.Run("Error wrapping", func(t *testing.T) {
        baseErr := errors.ErrItemNotFound
        wrapped := errors.NewError("GetItem", "User", baseErr)
        
        assert.ErrorIs(t, wrapped, baseErr)
        assert.True(t, errors.IsNotFound(wrapped))
    })
    
    t.Run("Error with context", func(t *testing.T) {
        ctx := map[string]any{
            "table": "users",
            "key":   "user123",
        }
        err := errors.NewErrorWithContext("GetItem", "User", errors.ErrItemNotFound, ctx)
        
        assert.Contains(t, err.Error(), "table")
        assert.Contains(t, err.Error(), "users")
    })
}
```

## Running Tests

```bash
# Run tests for a specific package
go test -v -cover ./pkg/types

# Run tests with race detection
go test -race ./...

# Run only unit tests (skip integration)
go test -short ./...

# Run with coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run benchmarks
go test -bench=. -benchmem ./pkg/marshal

# Run specific test
go test -run TestConverter_ToAttributeValue ./pkg/types
``` 
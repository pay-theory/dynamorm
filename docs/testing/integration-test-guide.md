# DynamORM Integration Testing Guide

## Overview

This guide covers best practices for integration testing with DynamORM, including setup with DynamoDB Local and common testing patterns.

## Prerequisites

### 1. Install DynamoDB Local

DynamoDB Local is required for running integration tests without AWS costs.

#### Option A: Using Docker (Recommended)
```bash
# Start DynamoDB Local
docker run -p 8000:8000 amazon/dynamodb-local

# Verify it's running
curl http://localhost:8000
```

#### Option B: Using Java JAR
```bash
# Download DynamoDB Local
wget https://s3.us-west-2.amazonaws.com/dynamodb-local/dynamodb_local_latest.tar.gz
tar -xzf dynamodb_local_latest.tar.gz

# Start DynamoDB Local (requires Java)
java -Djava.library.path=./DynamoDBLocal_lib -jar DynamoDBLocal.jar -sharedDb
```

### 2. Set Environment Variables
```bash
export DYNAMODB_ENDPOINT="http://localhost:8000"
export AWS_REGION="us-east-1"
```

## Writing Integration Tests

### Test Setup

```go
import (
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/credentials"
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
)

// Initialize DynamORM for tests
sessionConfig := session.Config{
    Region:   "us-east-1",
    Endpoint: "http://localhost:8000",
    AWSConfigOptions: []func(*config.LoadOptions) error{
        config.WithCredentialsProvider(
            credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
        ),
    },
}

db, err := dynamorm.New(sessionConfig)
```

## Running Integration Tests

### 1. Run All Integration Tests
```bash
# From the project root
go test ./tests/integration/... -v
```

### 2. Run Specific Test
```bash
# Run query integration tests
go test ./tests/integration/query_integration_test.go -v

# Run with specific test function
go test ./tests/integration/... -v -run TestQueryIntegrationSuite
```

### 3. Skip Integration Tests (CI/CD)
```bash
# Set environment variable to skip
export SKIP_INTEGRATION=true
go test ./... -v
```

## Creating New Integration Tests

### Test Template
```go
package integration

import (
    "context"
    "testing"
    
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/credentials"
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestDynamORMIntegration(t *testing.T) {
    // Skip if integration tests disabled
    if os.Getenv("SKIP_INTEGRATION") == "true" {
        t.Skip("Integration tests disabled")
    }
    
    // Initialize DynamORM correctly
    sessionConfig := session.Config{
        Region:   "us-east-1",
        Endpoint: "http://localhost:8000",
        AWSConfigOptions: []func(*config.LoadOptions) error{
            config.WithCredentialsProvider(
                credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
            ),
        },
    }
    
    db, err := dynamorm.New(sessionConfig)
    require.NoError(t, err)
    defer db.Close()
    
    // Your test code here
}
```

## Common Issues During Testing

### 1. Nil Pointer Dereference
**Cause**: Incorrect initialization or missing AWS configuration  
**Fix**: Use the corrected initialization pattern above

### 2. "Table not found" Errors
**Cause**: DynamORM auto-pluralizes table names  
**Fix**: 
- Model `User` → Table `Users`
- Model `MigrationSession` → Table `MigrationSessions`
- Use `CreateTable()` or `AutoMigrate()` before tests

### 3. Connection Refused
**Cause**: DynamoDB Local not running  
**Fix**: Start DynamoDB Local (see Prerequisites)

### 4. Invalid Credentials
**Cause**: Missing credentials for local testing  
**Fix**: Always provide dummy credentials for DynamoDB Local

## Debugging Integration Tests

### Enable AWS SDK Logging
```go
import "github.com/aws/smithy-go/logging"

sessionConfig := session.Config{
    Region:   "us-east-1",
    Endpoint: "http://localhost:8000",
    AWSConfigOptions: []func(*config.LoadOptions) error{
        config.WithClientLogMode(aws.LogRequestWithBody | aws.LogResponseWithBody),
        config.WithLogger(logging.NewStandardLogger(os.Stdout)),
        config.WithCredentialsProvider(
            credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
        ),
    },
}
```

### Test Individual Components
```go
// Test 1: Can we create the DB?
db, err := dynamorm.New(sessionConfig)
assert.NoError(t, err)
assert.NotNil(t, db)

// Test 2: Can we create a query?
query := db.Model(&User{})
assert.NotNil(t, query)

// Test 3: Can we create a table?
err = db.CreateTable(&User{})
// Ignore "already exists" errors
if err != nil && !strings.Contains(err.Error(), "ResourceInUseException") {
    t.Fatal(err)
}
```

## Continuous Integration Setup

### GitHub Actions Example
```yaml
name: Integration Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    
    services:
      dynamodb:
        image: amazon/dynamodb-local
        ports:
          - 8000:8000
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.22'
    
    - name: Run Integration Tests
      env:
        DYNAMODB_ENDPOINT: http://localhost:8000
        AWS_REGION: us-east-1
      run: |
        go test ./tests/integration/... -v
```

## Testing Best Practices

1. **Isolate Tests**: Each test should create its own tables or use unique keys
2. **Clean Up**: Always clean up test data after tests complete
3. **Use Fixtures**: Create reusable test data setup functions
4. **Test Edge Cases**: Include tests for error conditions and limits
5. **Performance Tests**: Add benchmarks for critical operations

## Quick Test Script

Save this as `test_integration.sh`:
```bash
#!/bin/bash
set -e

echo "Starting DynamoDB Local..."
docker run -d -p 8000:8000 --name dynamodb-test amazon/dynamodb-local

echo "Waiting for DynamoDB Local to start..."
sleep 5

echo "Running integration tests..."
DYNAMODB_ENDPOINT=http://localhost:8000 AWS_REGION=us-east-1 \
  go test ./tests/integration/... -v

echo "Cleaning up..."
docker stop dynamodb-test
docker rm dynamodb-test

echo "Integration tests complete!"
```

Make it executable: `chmod +x test_integration.sh` 
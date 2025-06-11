# DynamORM v1.0.2 - Troubleshooting Guide

This guide addresses the most common issues encountered when using DynamORM v1.0.2.

## Table of Contents
1. [Critical: Nil Pointer Dereference](#nil-pointer-dereference)
2. [Table Name Mismatches](#table-name-mismatches)
3. [Composite Key Errors](#composite-key-errors)
4. [Atomic Operations Not Working](#atomic-operations-not-working)
5. [Mock Implementation Issues](#mock-implementation-issues)

## Nil Pointer Dereference

### Issue
```
panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x1 addr=0x18 pc=0x39b79f1]

goroutine 17 [running]:
github.com/aws/aws-sdk-go-v2/service/dynamodb.addRetry.func1(0xc00011cf90)
    .../aws-sdk-go-v2/service/dynamodb@v1.43.3/api_client.go:710 +0x31
```

### Root Causes
This error can occur due to multiple reasons:

1. **Incorrect initialization syntax** (initial issue)
2. **Missing AWS credentials or configuration** (persistent issue)
3. **AWS SDK v2 client not properly initialized**

### Quick Fix (May Not Be Sufficient)
```go
// Change from:
db, err := dynamorm.New(dynamorm.Config{...})

// To:
import "github.com/pay-theory/dynamorm/pkg/session"
db, err := dynamorm.New(session.Config{
    Region: "us-east-1",
})
```

### ⚠️ If Still Getting Nil Pointer After Quick Fix

The nil pointer can persist even after the initialization fix due to AWS SDK v2 configuration issues. **See the comprehensive fix guide: [nil-pointer-fix.md](./nil-pointer-fix.md)**

Common scenarios requiring the comprehensive fix:
- Local development with DynamoDB Local
- Custom AWS credentials or profiles
- Lambda environments
- Missing AWS configuration

### Example: Proper Initialization for Different Environments

```go
// For AWS environments (EC2, ECS, etc.)
import (
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/pay-theory/dynamorm/pkg/session"
)

awsCfg, err := config.LoadDefaultConfig(ctx)
if err != nil {
    log.Fatal("Failed to load AWS config:", err)
}

sessionConfig := session.Config{
    Region: awsCfg.Region,
    AWSConfigOptions: []func(*config.LoadOptions) error{
        config.WithRegion(awsCfg.Region),
    },
}

db, err := dynamorm.New(sessionConfig)

// For local development
import "github.com/aws/aws-sdk-go-v2/credentials"

sessionConfig := session.Config{
    Region:   "us-east-1",
    Endpoint: "http://localhost:8000",
    AWSConfigOptions: []func(*config.LoadOptions) error{
        config.WithCredentialsProvider(
            credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
        ),
    },
}
```

## Table Name Mismatches

// ... existing code ... 
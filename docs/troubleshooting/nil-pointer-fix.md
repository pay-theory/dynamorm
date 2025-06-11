# DynamORM v1.0.2 - Nil Pointer Dereference Fix

## Issue
After applying the recommended fix to use `session.Config` instead of `dynamorm.Config`, users still experience nil pointer dereference errors when attempting any DynamoDB operation.

## Root Cause
The nil pointer occurs because the AWS SDK v2 DynamoDB client is not being properly initialized. This can happen when:
1. AWS credentials are not available or not properly configured
2. The AWS config loading fails silently
3. Required AWS SDK v2 configuration options are missing

## Complete Fix

### 1. Ensure AWS SDK v2 Dependencies
First, ensure you have the correct AWS SDK v2 dependencies:

```bash
go get github.com/aws/aws-sdk-go-v2/config
go get github.com/aws/aws-sdk-go-v2/credentials
go get github.com/aws/aws-sdk-go-v2/service/dynamodb
```

### 2. Correct Initialization Pattern

#### For AWS Environment (EC2, Lambda, ECS)
```go
import (
    "context"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
)

func initializeDynamORM(ctx context.Context) (*dynamorm.DB, error) {
    // Load AWS config with explicit options
    awsCfg, err := config.LoadDefaultConfig(ctx,
        config.WithRegion("us-east-1"),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to load AWS config: %w", err)
    }

    // Create DynamORM config with AWS config options
    sessionConfig := session.Config{
        Region: awsCfg.Region,
        AWSConfigOptions: []func(*config.LoadOptions) error{
            config.WithRegion("us-east-1"),
        },
    }

    // Initialize DynamORM
    db, err := dynamorm.New(sessionConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to create DynamORM: %w", err)
    }

    return db, nil
}
```

#### For Local Development (DynamoDB Local)
```go
import (
    "context"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/credentials"
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
)

func initializeDynamORMLocal(ctx context.Context) (*dynamorm.DB, error) {
    sessionConfig := session.Config{
        Region:   "us-east-1",
        Endpoint: "http://localhost:8000", // DynamoDB Local endpoint
        AWSConfigOptions: []func(*config.LoadOptions) error{
            config.WithCredentialsProvider(
                credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
            ),
            config.WithRegion("us-east-1"),
        },
    }

    db, err := dynamorm.New(sessionConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to create DynamORM: %w", err)
    }

    return db, nil
}
```

#### For Custom AWS Profiles
```go
import (
    "context"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
)

func initializeDynamORMWithProfile(ctx context.Context, profile string) (*dynamorm.DB, error) {
    sessionConfig := session.Config{
        Region: "us-east-1",
        AWSConfigOptions: []func(*config.LoadOptions) error{
            config.WithSharedConfigProfile(profile),
            config.WithRegion("us-east-1"),
        },
    }

    db, err := dynamorm.New(sessionConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to create DynamORM: %w", err)
    }

    return db, nil
}
```

### 3. Complete Working Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/credentials"
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
)

type User struct {
    ID    string `dynamorm:"pk"`
    Email string `dynamorm:"sk"`
    Name  string
}

func main() {
    ctx := context.Background()
    
    // Initialize DynamORM with proper AWS configuration
    db, err := initializeDynamORM(ctx)
    if err != nil {
        log.Fatalf("Failed to initialize DynamORM: %v", err)
    }
    
    // Create table (optional for development)
    if err := db.CreateTable(&User{}); err != nil {
        // Ignore if table already exists
        log.Printf("Table creation: %v", err)
    }
    
    // Now you can use the database
    user := &User{
        ID:    "user-123",
        Email: "test@example.com",
        Name:  "Test User",
    }
    
    if err := db.Model(user).Create(); err != nil {
        log.Fatalf("Failed to create user: %v", err)
    }
    
    fmt.Println("User created successfully!")
}

func initializeDynamORM(ctx context.Context) (core.ExtendedDB, error) {
    // Determine environment
    if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
        // Lambda environment
        return initializeForLambda(ctx)
    } else if endpoint := os.Getenv("DYNAMODB_ENDPOINT"); endpoint != "" {
        // Local development
        return initializeForLocal(ctx, endpoint)
    } else {
        // Standard AWS environment
        return initializeForAWS(ctx)
    }
}

func initializeForAWS(ctx context.Context) (core.ExtendedDB, error) {
    // Ensure AWS config can be loaded
    awsCfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to load AWS config: %w", err)
    }
    
    sessionConfig := session.Config{
        Region: awsCfg.Region,
        AWSConfigOptions: []func(*config.LoadOptions) error{
            config.WithRegion(awsCfg.Region),
        },
    }
    
    return dynamorm.New(sessionConfig)
}

func initializeForLocal(ctx context.Context, endpoint string) (core.ExtendedDB, error) {
    sessionConfig := session.Config{
        Region:   "us-east-1",
        Endpoint: endpoint,
        AWSConfigOptions: []func(*config.LoadOptions) error{
            config.WithCredentialsProvider(
                credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
            ),
            config.WithRegion("us-east-1"),
        },
    }
    
    return dynamorm.New(sessionConfig)
}

func initializeForLambda(ctx context.Context) (core.ExtendedDB, error) {
    // Lambda has implicit credentials
    sessionConfig := session.Config{
        Region: os.Getenv("AWS_REGION"),
    }
    
    return dynamorm.New(sessionConfig)
}
```

### 4. Debugging Steps

If you still encounter issues:

1. **Enable AWS SDK Logging**:
```go
import "github.com/aws/smithy-go/logging"

sessionConfig := session.Config{
    Region: "us-east-1",
    AWSConfigOptions: []func(*config.LoadOptions) error{
        config.WithClientLogMode(aws.LogRequestWithBody | aws.LogResponseWithBody),
        config.WithLogger(logging.NewStandardLogger(os.Stdout)),
    },
}
```

2. **Verify AWS Credentials**:
```bash
# Check if credentials are available
aws sts get-caller-identity

# Check environment variables
echo $AWS_REGION
echo $AWS_PROFILE
echo $AWS_ACCESS_KEY_ID
```

3. **Test Direct DynamoDB Client**:
```go
// Test if AWS SDK v2 works directly
awsCfg, err := config.LoadDefaultConfig(ctx)
if err != nil {
    log.Fatalf("AWS config failed: %v", err)
}

client := dynamodb.NewFromConfig(awsCfg)
_, err = client.ListTables(ctx, &dynamodb.ListTablesInput{})
if err != nil {
    log.Fatalf("DynamoDB client failed: %v", err)
}
```

### 5. Common Mistakes to Avoid

1. **Don't use zero-value Config**:
```go
// WRONG - This will cause nil pointer
db, err := dynamorm.New(session.Config{})

// CORRECT - Provide at least region
db, err := dynamorm.New(session.Config{
    Region: "us-east-1",
})
```

2. **Don't ignore AWS config errors**:
```go
// WRONG - Ignoring config errors
awsCfg, _ := config.LoadDefaultConfig(ctx)

// CORRECT - Handle config errors
awsCfg, err := config.LoadDefaultConfig(ctx)
if err != nil {
    return nil, fmt.Errorf("failed to load AWS config: %w", err)
}
```

3. **Don't forget credentials for local development**:
```go
// WRONG - No credentials for local DynamoDB
sessionConfig := session.Config{
    Region:   "us-east-1",
    Endpoint: "http://localhost:8000",
}

// CORRECT - Provide dummy credentials
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

## Summary

The nil pointer dereference occurs when the DynamoDB client is not properly initialized due to missing or incorrect AWS configuration. The fix requires:

1. Proper AWS SDK v2 imports
2. Explicit credential configuration (especially for local development)
3. Proper error handling during initialization
4. Using `AWSConfigOptions` to pass AWS-specific configuration

Always ensure that your AWS environment is properly configured before initializing DynamORM. 
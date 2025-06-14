# DynamORM Installation & Initialization Guide

## Installation

Install DynamORM using Go modules:

```bash
go get github.com/pay-theory/dynamorm@v1.0.9
```

## Required Imports

```go
import (
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
    "github.com/pay-theory/dynamorm/pkg/core"
)
```

## Initialization

### Basic Initialization

```go
package main

import (
    "log"
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
)

func main() {
    // Basic configuration
    config := session.Config{
        Region: "us-east-1",
    }
    
    db, err := dynamorm.New(config)
    if err != nil {
        log.Fatal("Failed to initialize DynamORM:", err)
    }
    defer db.Close()
    
    // Your code here
}
```

## Configuration Options

### Basic Configuration

```go
config := session.Config{
    Region:     "us-east-1",
    MaxRetries: 3,  // Default: 3
    DefaultRCU: 5,  // Default read capacity units
    DefaultWCU: 5,  // Default write capacity units
}
```

### Local Development (DynamoDB Local)

```go
config := session.Config{
    Region:   "us-east-1",
    Endpoint: "http://localhost:8000",
}
```

### Using AWS Profile

```go
import (
    "github.com/aws/aws-sdk-go-v2/config"
)

config := session.Config{
    Region: "us-east-1",
    AWSConfigOptions: []func(*config.LoadOptions) error{
        config.WithSharedConfigProfile("my-profile"),
    },
}
```

### Lambda Environment

```go
import "os"

config := session.Config{
    Region: os.Getenv("AWS_REGION"),
    // AWS credentials are automatically loaded in Lambda
}

db, err := dynamorm.New(config)
if err != nil {
    return err
}

// Optional: Enable Lambda timeout handling
ctx := context.Background()
db = db.WithLambdaTimeout(ctx)
```

### Using Existing AWS Config

```go
import (
    "context"
    "github.com/aws/aws-sdk-go-v2/config"
)

// Load your AWS config
awsCfg, err := config.LoadDefaultConfig(context.Background())
if err != nil {
    log.Fatal(err)
}

// Use it with DynamORM
sessionConfig := session.Config{
    Region:              awsCfg.Region,
    CredentialsProvider: awsCfg.Credentials,
}

db, err := dynamorm.New(sessionConfig)
```

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"
    
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/core"
    "github.com/pay-theory/dynamorm/pkg/session"
)

// Define your model
type User struct {
    ID        string    `dynamorm:"pk" json:"id"`
    Email     string    `dynamorm:"sk" json:"email"`
    Name      string    `json:"name"`
    Active    bool      `json:"active"`
    CreatedAt time.Time `dynamorm:"created_at" json:"created_at"`
    UpdatedAt time.Time `dynamorm:"updated_at" json:"updated_at"`
}

func main() {
    // Initialize with proper configuration
    config := session.Config{
        Region: "us-east-1",
    }
    
    db, err := dynamorm.New(config)
    if err != nil {
        log.Fatal("Failed to initialize:", err)
    }
    defer db.Close()
    
    // Ensure table exists (development only)
    if err := db.EnsureTable(&User{}); err != nil {
        log.Printf("Could not ensure table: %v", err)
    }
    
    // Create a user
    user := &User{
        ID:     "user-123",
        Email:  "john@example.com",
        Name:   "John Doe",
        Active: true,
    }
    
    if err := db.Model(user).Create(); err != nil {
        log.Printf("Create failed: %v", err)
        return
    }
    
    fmt.Println("User created successfully")
    
    // Query the user
    var fetchedUser User
    err = db.Model(&User{}).
        Where("ID", "=", "user-123").
        Where("Email", "=", "john@example.com").
        First(&fetchedUser)
    
    if err != nil {
        log.Printf("Query failed: %v", err)
        return
    }
    
    fmt.Printf("Found user: %+v\n", fetchedUser)
}
```

## Common Initialization Errors

### 1. Nil Pointer Dereference

**Symptom**: `panic: runtime error: invalid memory address or nil pointer dereference`

**Cause**: Using incorrect initialization syntax or missing configuration

**Solution**: Use `session.Config` with proper initialization as shown above

### 2. Table Not Found

**Symptom**: `ResourceNotFoundException: Requested resource not found`

**Cause**: Table doesn't exist in DynamoDB

**Solution**: 
```go
// For development - create table if it doesn't exist
err := db.EnsureTable(&YourModel{})

// For production - use Infrastructure as Code (Terraform, CDK, CloudFormation)
```

### 3. Missing Credentials

**Symptom**: `NoCredentialProviders: no valid providers in chain`

**Solution**: Ensure AWS credentials are configured:
- Set `AWS_PROFILE` environment variable
- Use IAM roles (in EC2/Lambda)
- Configure credentials file (`~/.aws/credentials`)
- Pass credentials explicitly in config

## Type Aliases

DynamORM provides type aliases for convenience, but it's clearer to use the full import path:

```go
// These are equivalent:
config := session.Config{...}        // Recommended - clear origin
config := dynamorm.Config{...}       // Works via type alias

// Type alias definition (in dynamorm.go):
type Config = session.Config
```

## Next Steps

- [Define Your Models](./models.md)
- [Basic CRUD Operations](./crud.md)
- [Query Patterns](./queries.md)
- [Working with Indexes](./indexes.md) 
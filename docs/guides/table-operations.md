# Table Operations Guide

This guide covers DynamORM's table operation capabilities, which provide lightweight wrappers around DynamoDB's table management APIs.

## Overview

DynamORM provides simple table operations that are appropriate for an ORM:
- Thin wrappers over DynamoDB APIs
- No persistent state or version tracking
- Useful for development and testing
- Compatible with Infrastructure as Code patterns

## Basic Table Operations

### Creating Tables

```go
// Create a table from a model definition
type User struct {
    ID    string `dynamodbav:"id,pk"`
    Email string `dynamodbav:"email"`
}

err := db.CreateTable(&User{})
```

### Ensuring Tables Exist

```go
// Create table only if it doesn't exist (idempotent)
err := db.EnsureTable(&User{})
```

### Deleting Tables

```go
// Delete a table
err := db.DeleteTable(&User{})
```

### Describing Tables

```go
// Get table information
desc, err := db.DescribeTable(&User{})
if err == nil {
    fmt.Printf("Table status: %s\n", desc.TableStatus)
}
```

## AutoMigrate

The `AutoMigrate` function provides simple table creation based on model definitions:

```go
// Create tables for multiple models
err := db.AutoMigrate(&User{}, &Post{}, &Comment{})
```

## Enhanced AutoMigrate with Data Copy

For more complex scenarios, `AutoMigrateWithOptions` supports data migration between tables:

### Basic Data Copy

```go
// Copy data from one table to another
err := db.AutoMigrateWithOptions(&User{},
    dynamorm.WithTargetModel(&UserV2{}),
    dynamorm.WithDataCopy(true),
)
```

### Creating Backups

```go
// Create a backup before making changes
err := db.AutoMigrateWithOptions(&User{},
    dynamorm.WithBackupTable("users_backup_20240101"),
    dynamorm.WithDataCopy(true),
)
```

### Data Transformation

```go
// Transform data during migration
type UserV1 struct {
    ID    string `dynamodbav:"id,pk"`
    Name  string `dynamodbav:"name"`
}

type UserV2 struct {
    ID        string `dynamodbav:"id,pk"`
    FirstName string `dynamodbav:"first_name"`
    LastName  string `dynamodbav:"last_name"`
}

transformFunc := func(old *UserV1) *UserV2 {
    parts := strings.Split(old.Name, " ")
    return &UserV2{
        ID:        old.ID,
        FirstName: parts[0],
        LastName:  parts[1],
    }
}

err := db.AutoMigrateWithOptions(&UserV1{},
    dynamorm.WithTargetModel(&UserV2{}),
    dynamorm.WithDataCopy(true),
    dynamorm.WithTransform(transformFunc),
)
```

### Batch Size Control

```go
// Control batch size for large tables
err := db.AutoMigrateWithOptions(&User{},
    dynamorm.WithTargetModel(&UserV2{}),
    dynamorm.WithDataCopy(true),
    dynamorm.WithBatchSize(100), // Process 100 items at a time
)
```

## Usage Patterns

### Development Environment

```go
func setupDevEnvironment() error {
    db, err := dynamorm.New(dynamorm.Config{
        Endpoint: "http://localhost:8000", // DynamoDB Local
    })
    if err != nil {
        return err
    }

    // Create all tables for development
    models := []interface{}{
        &User{},
        &Post{},
        &Comment{},
    }

    for _, model := range models {
        if err := db.EnsureTable(model); err != nil {
            return err
        }
    }

    return nil
}
```

### Testing

```go
func TestUserOperations(t *testing.T) {
    // Create isolated table for testing
    testUser := &User{
        TableName: fmt.Sprintf("test_users_%d", time.Now().Unix()),
    }
    
    err := db.CreateTable(testUser)
    require.NoError(t, err)
    
    // Clean up after test
    defer db.DeleteTable(testUser)
    
    // Run tests...
}
```

### One-Time Setup Lambda

```go
func SetupTablesHandler(ctx context.Context) error {
    db, err := dynamorm.New(dynamorm.Config{})
    if err != nil {
        return err
    }

    // Ensure all required tables exist
    err = db.EnsureTable(&User{})
    if err != nil {
        return fmt.Errorf("failed to ensure users table: %w", err)
    }

    err = db.EnsureTable(&Post{})
    if err != nil {
        return fmt.Errorf("failed to ensure posts table: %w", err)
    }

    return nil
}
```

### Data Migration Lambda

```go
func DataMigrationHandler(ctx context.Context, event MigrationEvent) error {
    db, err := dynamorm.New(dynamorm.Config{})
    if err != nil {
        return err
    }

    // Migrate with transformation
    return db.AutoMigrateWithOptions(&UserV1{},
        dynamorm.WithTargetModel(&UserV2{}),
        dynamorm.WithDataCopy(true),
        dynamorm.WithBackupTable(fmt.Sprintf("users_backup_%s", time.Now().Format("20060102"))),
        dynamorm.WithTransform(transformUsers),
        dynamorm.WithBatchSize(250),
    )
}
```

## Best Practices

### 1. Use IaC for Production Tables

In production, tables should be defined using Infrastructure as Code tools:

```yaml
# CDK/CloudFormation example
Resources:
  UsersTable:
    Type: AWS::DynamoDB::Table
    Properties:
      TableName: users
      KeySchema:
        - AttributeName: id
          KeyType: HASH
      # ... full table definition
```

### 2. Table Operations for Development

Use DynamORM's table operations for:
- Local development with DynamoDB Local
- Integration testing
- Deployment scripts
- One-time setup tasks

### 3. Avoid Runtime Table Creation

Don't create tables during normal Lambda execution:

```go
// ❌ Bad: Creating tables in request handler
func Handler(ctx context.Context, request Request) (Response, error) {
    db.EnsureTable(&User{}) // Don't do this!
    // ... handle request
}

// ✅ Good: Tables already exist
func Handler(ctx context.Context, request Request) (Response, error) {
    // Tables created by IaC, just use them
    user := &User{ID: request.UserID}
    err := db.Model(user).First(user)
    // ...
}
```

### 4. Data Migration Patterns

For production data migrations:

1. Create a dedicated Lambda for migrations
2. Trigger it from your deployment pipeline
3. Use backups for safety
4. Test migrations in staging first

## Summary

DynamORM's table operations provide the convenience developers expect from an ORM while maintaining a clear separation from infrastructure management. They're perfect for development, testing, and controlled deployment scenarios, while production table definitions should remain in your Infrastructure as Code tools. 
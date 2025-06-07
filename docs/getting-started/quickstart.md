# Quickstart Guide

Get up and running with DynamORM in 5 minutes! This guide will walk you through creating your first DynamoDB application.

## Prerequisites

- Go 1.21+ installed
- AWS credentials configured (or Docker for local development)

## Step 1: Install DynamORM

```bash
go get github.com/dynamorm/dynamorm
```

## Step 2: Create Your First Model

Create a file `main.go`:

```go
package main

import (
    "fmt"
    "log"
    "time"
    
    "github.com/dynamorm/dynamorm"
)

// Define a User model
type User struct {
    ID        string    `dynamorm:"pk"`              // Partition key
    Email     string    `dynamorm:"index:email-idx"` // Global secondary index
    Name      string
    Age       int
    Active    bool
    CreatedAt time.Time `dynamorm:"created_at"`      // Auto-set on create
    UpdatedAt time.Time `dynamorm:"updated_at"`      // Auto-set on update
}

func main() {
    // Initialize DynamORM
    db, err := dynamorm.New(dynamorm.Config{
        Region: "us-east-1",
        // For local development with Docker:
        // Endpoint: "http://localhost:8000",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Create table (only needed once)
    err = db.AutoMigrate(&User{})
    if err != nil {
        log.Fatal(err)
    }

    // Create a user
    user := &User{
        ID:     "user-001",
        Email:  "alice@example.com",
        Name:   "Alice Smith",
        Age:    28,
        Active: true,
    }
    
    err = db.Model(user).Create()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("✅ User created:", user.Name)

    // Query the user
    var foundUser User
    err = db.Model(&User{}).
        Where("ID", "=", "user-001").
        First(&foundUser)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("✅ Found user: %s (created at %v)\n", 
        foundUser.Name, foundUser.CreatedAt)

    // Update the user
    err = db.Model(&User{}).
        Where("ID", "=", "user-001").
        Update("Age", 29)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("✅ User age updated")

    // Query by email index
    var userByEmail User
    err = db.Model(&User{}).
        Index("email-idx").
        Where("Email", "=", "alice@example.com").
        First(&userByEmail)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("✅ Found by email: %s\n", userByEmail.Name)

    // List all active users
    var activeUsers []User
    err = db.Model(&User{}).
        Where("Active", "=", true).
        All(&activeUsers)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("✅ Found %d active users\n", len(activeUsers))

    fmt.Println("\n🎉 Congratulations! You've successfully used DynamORM!")
}
```

## Step 3: Run Your Application

### Option A: Using AWS DynamoDB

```bash
# Make sure AWS credentials are configured
aws configure

# Run the application
go run main.go
```

### Option B: Using Local DynamoDB

```bash
# Start DynamoDB Local with Docker
docker run -p 8000:8000 amazon/dynamodb-local

# Update the config in main.go to use local endpoint:
# Endpoint: "http://localhost:8000",

# Run the application
go run main.go
```

## What Just Happened?

You've just:
1. ✅ Defined a model with DynamORM struct tags
2. ✅ Created a DynamoDB table automatically
3. ✅ Inserted a record
4. ✅ Queried by primary key
5. ✅ Updated a record
6. ✅ Queried using a secondary index
7. ✅ Listed multiple records

## Key Concepts

### Models
- Use struct tags to define keys and indexes
- `dynamorm:"pk"` - Partition key
- `dynamorm:"sk"` - Sort key (for composite keys)
- `dynamorm:"index:name"` - Secondary index

### Operations
- `Create()` - Insert new records
- `First()` - Get single record
- `All()` - Get multiple records
- `Update()` - Update fields
- `Delete()` - Remove records

### Queries
- `Where()` - Add conditions
- `Index()` - Use secondary index
- `Filter()` - Additional filtering
- `OrderBy()` - Sort results
- `Limit()` - Limit results

## Next Steps

### 1. Explore More Examples

Try these common patterns:

```go
// Batch operations
users := []*User{
    {ID: "user-002", Name: "Bob"},
    {ID: "user-003", Name: "Charlie"},
}
err = db.Model(&User{}).BatchCreate(users)

// Transactions
err = db.Transaction(func(tx *dynamorm.Tx) error {
    user1.Age = 30
    user2.Age = 25
    
    if err := tx.Model(user1).Update(); err != nil {
        return err
    }
    return tx.Model(user2).Update()
})

// Count records
count, err := db.Model(&User{}).
    Where("Active", "=", true).
    Count()
```

### 2. Learn More

- [Basic Usage Guide](basic-usage.md) - Deep dive into DynamORM features
- [API Reference](../reference/api.md) - Complete API documentation
- [Examples](../../examples/) - Real-world examples
- [Struct Tags](../reference/struct-tags.md) - All available tags

### 3. Best Practices

- Design your partition keys for even distribution
- Use indexes for non-key queries
- Batch operations when possible
- Handle errors appropriately

## Common Issues

### "ResourceNotFoundException"
The table doesn't exist. Make sure to run `AutoMigrate()` first.

### "ValidationException"
Check that your query conditions match the table's key schema.

### Local Development Connection Issues
Ensure DynamoDB Local is running on port 8000.

## Summary

In just 5 minutes, you've learned the basics of DynamORM! You can now:
- Define models with struct tags
- Perform CRUD operations
- Query using indexes
- Work with DynamoDB locally or in AWS

Ready to build something amazing? Check out our [Basic Usage Guide](basic-usage.md) for more advanced features!

---

<p align="center">
  🚀 Happy coding with DynamORM!
</p> 
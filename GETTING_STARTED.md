# Getting Started with DynamORM

Welcome to DynamORM - a powerful, type-safe ORM for Amazon DynamoDB in Go!

## Installation

```bash
go get github.com/pay-theory/dynamorm
```

## Quick Start

### 1. Define Your Models

```go
package models

import "time"

type User struct {
    // Primary key
    ID string `dynamorm:"pk"`
    
    // Attributes
    Email     string    `dynamorm:"index:gsi-email"`
    Name      string
    Age       int
    Status    string    `dynamorm:"index:gsi-status-created,pk"`
    CreatedAt time.Time `dynamorm:"index:gsi-status-created,sk"`
    
    // Special fields
    UpdatedAt time.Time `dynamorm:"updated_at"`
    Version   int       `dynamorm:"version"`
}

type Product struct {
    SKU         string  `dynamorm:"pk"`
    Category    string  `dynamorm:"index:gsi-category,pk"`
    Price       float64 `dynamorm:"index:gsi-category,sk"`
    Name        string
    Description string
    InStock     bool
    Tags        []string `dynamorm:"set"`
}
```

### 2. Initialize DynamORM

```go
package main

import (
    "log"
    "github.com/pay-theory/dynamorm"
    "your-app/models"
)

func main() {
    // Create DynamORM instance
    db, err := dynamorm.New(dynamorm.Config{
        Region: "us-east-1",
        // For local development:
        // Endpoint: "http://localhost:8000",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Create tables from your models
    err = db.AutoMigrate(&models.User{}, &models.Product{})
    if err != nil {
        log.Fatal(err)
    }
}
```

### 3. Basic CRUD Operations

```go
// Create
user := &models.User{
    ID:     "user-123",
    Email:  "john@example.com",
    Name:   "John Doe",
    Age:    30,
    Status: "active",
}
err := db.Model(user).Create()

// Read
var foundUser models.User
err = db.Model(&models.User{}).
    Where("ID", "=", "user-123").
    First(&foundUser)

// Update
err = db.Model(&models.User{}).
    Where("ID", "=", "user-123").
    Update("Status", "premium")

// Delete
err = db.Model(&models.User{}).
    Where("ID", "=", "user-123").
    Delete()
```

### 4. Querying

```go
// Query by index
var users []models.User
err = db.Model(&models.User{}).
    Index("gsi-email").
    Where("Email", "=", "john@example.com").
    All(&users)

// Complex queries
err = db.Model(&models.User{}).
    Where("Status", "=", "active").
    Where("Age", ">", 18).
    Filter("contains(Name, :search)", dynamorm.Param("search", "John")).
    OrderBy("CreatedAt", "desc").
    Limit(20).
    All(&users)

// Count
count, err := db.Model(&models.User{}).
    Where("Status", "=", "active").
    Count()
```

### 5. Batch Operations

```go
// Batch create
users := []*models.User{
    {ID: "1", Name: "Alice"},
    {ID: "2", Name: "Bob"},
    {ID: "3", Name: "Charlie"},
}
err = db.Model(&models.User{}).BatchCreate(users)

// Batch get
var foundUsers []*models.User
keys := []interface{}{"1", "2", "3"}
err = db.Model(&models.User{}).BatchGet(keys, &foundUsers)
```

### 6. Transactions

```go
import "github.com/pay-theory/dynamorm/pkg/transaction"

// Atomic operations
err = db.TransactionFunc(func(tx *transaction.Transaction) error {
    // Create new user
    newUser := &models.User{ID: "4", Name: "David", Balance: 100}
    if err := tx.Create(newUser); err != nil {
        return err
    }
    
    // Update existing user
    existingUser.Balance -= 50
    newUser.Balance += 50
    
    if err := tx.Update(existingUser); err != nil {
        return err
    }
    
    return tx.Update(newUser)
})
```

### 7. Pagination

```go
// First page
result1, err := db.Model(&models.User{}).
    Where("Status", "=", "active").
    Limit(10).
    AllPaginated(&users)

// Next page
result2, err := db.Model(&models.User{}).
    Where("Status", "=", "active").
    Cursor(result1.NextCursor).
    Limit(10).
    AllPaginated(&users)
```

## Best Practices

### 1. Model Design

- Use meaningful partition keys that distribute data evenly
- Add sort keys for time-series or hierarchical data
- Create GSIs for common query patterns
- Use sparse indexes for optional attributes

### 2. Query Optimization

- Always use indexes when possible (avoid scans)
- Use projections to retrieve only needed fields
- Batch operations when working with multiple items
- Use consistent reads only when necessary

### 3. Error Handling

```go
import "github.com/pay-theory/dynamorm/pkg/errors"

user := &models.User{}
err := db.Model(&models.User{}).
    Where("ID", "=", "nonexistent").
    First(user)

if err == errors.ErrItemNotFound {
    // Handle not found case
} else if err != nil {
    // Handle other errors
}
```

### 4. Table Configuration

```go
import "github.com/pay-theory/dynamorm/pkg/schema"

// Custom table configuration
err = db.CreateTable(&models.User{},
    schema.WithBillingMode(types.BillingModeProvisioned),
    schema.WithThroughput(5, 5), // 5 RCU, 5 WCU
    schema.WithStreamSpecification(types.StreamSpecification{
        StreamEnabled:  aws.Bool(true),
        StreamViewType: types.StreamViewTypeNewAndOldImages,
    }),
)
```

## Common Patterns

### 1. Optimistic Locking

```go
type Document struct {
    ID      string `dynamorm:"pk"`
    Content string
    Version int    `dynamorm:"version"` // Auto-incremented
}

// Version is automatically checked and incremented on update
```

### 2. Time-Based Queries

```go
type Event struct {
    UserID    string    `dynamorm:"pk"`
    Timestamp time.Time `dynamorm:"sk"`
    Type      string
    Data      string
}

// Query events for a user in time range
var events []Event
err = db.Model(&Event{}).
    Where("UserID", "=", userID).
    Where("Timestamp", "BETWEEN", startTime, endTime).
    All(&events)
```

### 3. Hierarchical Data

```go
type Item struct {
    PK string `dynamorm:"pk"` // "ORG#123"
    SK string `dynamorm:"sk"` // "DEPT#eng", "DEPT#eng#TEAM#backend", etc.
}

// Query all items under a department
err = db.Model(&Item{}).
    Where("PK", "=", "ORG#123").
    Where("SK", "BEGINS_WITH", "DEPT#eng").
    All(&items)
```

## Troubleshooting

### Local Development

1. Run DynamoDB Local:
```bash
docker run -p 8000:8000 amazon/dynamodb-local
```

2. Configure endpoint:
```go
db, err := dynamorm.New(dynamorm.Config{
    Region:   "us-east-1",
    Endpoint: "http://localhost:8000",
})
```

### Common Issues

**"ResourceNotFoundException"**
- Table doesn't exist. Run `db.AutoMigrate()` or `db.CreateTable()`

**"ValidationException"**  
- Check your query conditions match the key schema
- Ensure you're using the correct index

**"ProvisionedThroughputExceededException"**
- Consider using on-demand billing mode
- Increase provisioned capacity
- Implement exponential backoff

## Next Steps

- Read the [Design Document](DESIGN.md) for deeper understanding
- Check [Examples](examples/) for complete applications
- See [STRUCT_TAGS.md](STRUCT_TAGS.md) for all available struct tags
- Join our community for support and updates

Happy coding with DynamORM! 🚀 
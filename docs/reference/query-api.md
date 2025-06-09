# DynamORM Query API Reference

## Overview

The DynamORM Query API provides a fluent, type-safe interface for interacting with DynamoDB. This document covers all available query methods, operators, and patterns.

## Table of Contents
- [Basic Query Methods](#basic-query-methods)
- [UpdateBuilder API](#updatebuilder-api)
- [Filter Operations](#filter-operations)
- [Advanced Query Features](#advanced-query-features)
- [Batch Operations](#batch-operations)
- [Transaction Support](#transaction-support)

## Basic Query Methods

### Creating a Query

```go
// Start a query on a model
query := db.Model(&models.User{})
```

### Where Conditions

The `Where` method adds conditions to filter results. It supports various operators:

```go
// Equality
db.Model(&models.User{}).Where("ID", "=", "user123")

// Comparison operators
db.Model(&models.Product{}).Where("Price", ">", 100)
db.Model(&models.Product{}).Where("Stock", "<=", 10)

// BETWEEN operator
db.Model(&models.Order{}).Where("Total", "BETWEEN", []float64{100, 500})

// IN operator (up to 100 values)
db.Model(&models.Post{}).Where("Status", "IN", []string{"draft", "published"})

// String operations
db.Model(&models.User{}).Where("Email", "BEGINS_WITH", "admin@")
db.Model(&models.Post{}).Where("Tags", "CONTAINS", "golang")

// Existence checks
db.Model(&models.User{}).Where("DeletedAt", "NOT_EXISTS", nil)
db.Model(&models.User{}).Where("VerifiedAt", "EXISTS", nil)
```

### Index Usage

```go
// Use a Global Secondary Index
db.Model(&models.Post{}).
    Index("gsi-author").
    Where("AuthorID", "=", authorID)

// Use a composite index
db.Model(&models.Post{}).
    Index("gsi-status-date").
    Where("Status", "=", "published").
    Where("PublishedAt", ">", startDate)
```

### Retrieving Results

```go
// Get first matching item
var user models.User
err := db.Model(&models.User{}).
    Where("Email", "=", "user@example.com").
    First(&user)

// Get all matching items
var posts []models.Post
err := db.Model(&models.Post{}).
    Where("Status", "=", "published").
    All(&posts)

// Count matching items
count, err := db.Model(&models.Post{}).
    Where("AuthorID", "=", authorID).
    Count()
```

### Pagination

```go
// Limit results
var posts []models.Post
err := db.Model(&models.Post{}).
    Where("Status", "=", "published").
    Limit(10).
    All(&posts)

// Offset (for pagination)
err := db.Model(&models.Post{}).
    Where("Status", "=", "published").
    Limit(10).
    Offset(20).
    All(&posts)

// Cursor-based pagination
cursor := "eyJpZCI6InBvc3QtMTIzIiwidGltZSI6MTY0MjUxMjAwMH0="
err := db.Model(&models.Post{}).
    Where("Status", "=", "published").
    Cursor(cursor).
    Limit(10).
    All(&posts)
```

### Ordering

```go
// Order by field
err := db.Model(&models.Post{}).
    Where("Status", "=", "published").
    OrderBy("PublishedAt", "desc").
    All(&posts)
```

### Projection (Select specific fields)

```go
// Select specific fields only
var posts []models.Post
err := db.Model(&models.Post{}).
    Select("ID", "Title", "PublishedAt").
    Where("Status", "=", "published").
    All(&posts)
```

## UpdateBuilder API

The UpdateBuilder provides atomic update operations for DynamoDB items.

### Basic Updates

```go
// Update specific fields
err := db.Model(&models.User{
    ID: userID,
}).UpdateBuilder().
    Set("Name", "John Doe").
    Set("UpdatedAt", time.Now()).
    Execute()
```

### Atomic Counters

```go
// Increment a counter
err := db.Model(&models.Post{
    ID:       postID,
    AuthorID: authorID, // Required if part of key
}).UpdateBuilder().
    Increment("ViewCount").
    Execute()

// Increment by specific amount
err := db.Model(&models.Product{
    ID: productID,
}).UpdateBuilder().
    Add("Stock", -5). // Decrease stock by 5
    Execute()

// Decrement
err := db.Model(&models.Product{
    ID: productID,
}).UpdateBuilder().
    Decrement("Stock").
    Execute()
```

### Conditional Updates

```go
// Update with condition
err := db.Model(&models.User{
    ID: userID,
}).UpdateBuilder().
    Set("Status", "active").
    Set("UpdatedAt", time.Now()).
    Condition("Status", "=", "pending").
    Execute()

// Optimistic locking with version
err := db.Model(&models.Document{
    ID: docID,
}).UpdateBuilder().
    Set("Content", newContent).
    Add("Version", 1).
    ConditionVersion(currentVersion).
    Execute()

// Ensure field exists
err := db.Model(&models.User{
    ID: userID,
}).UpdateBuilder().
    Set("EmailVerified", true).
    ConditionExists("Email").
    Execute()
```

### List Operations

```go
// Set a list element at specific index
err := db.Model(&models.User{
    ID: userID,
}).UpdateBuilder().
    SetListElement("Preferences", 0, "dark-mode").
    Execute()

// Remove from list at index
err := db.Model(&models.User{
    ID: userID,
}).UpdateBuilder().
    RemoveFromListAt("Tags", 2).
    Execute()

// Append to list (when supported)
err := db.Model(&models.Session{
    ID: sessionID,
}).UpdateBuilder().
    AppendToList("Events", []string{"login", "view-dashboard"}).
    Execute()
```

### Remove Attributes

```go
// Remove attributes from item
err := db.Model(&models.User{
    ID: userID,
}).UpdateBuilder().
    Remove("TempToken").
    Remove("VerificationCode").
    Set("UpdatedAt", time.Now()).
    Execute()
```

### Complex Updates

```go
// Multiple operations in one update
err := db.Model(&models.Analytics{
    ID:   analyticsID,
    Date: date,
}).UpdateBuilder().
    Add("PageViews", 10).
    Add("UniqueVisitors", 3).
    Set("LastUpdated", time.Now()).
    Set("Status", "active").
    ConditionExists("ID").
    Execute()
```

## Filter Operations

### Basic Filters

```go
// AND filter
var posts []models.Post
err := db.Model(&models.Post{}).
    Where("AuthorID", "=", authorID).
    Filter("Status", "=", "published").
    Filter("ViewCount", ">", 100).
    All(&posts)

// OR filter
err := db.Model(&models.Post{}).
    Where("AuthorID", "=", authorID).
    Filter("Status", "=", "published").
    OrFilter("Featured", "=", true).
    All(&posts)
```

### Filter Groups

```go
// Grouped AND conditions
err := db.Model(&models.Product{}).
    Where("CategoryID", "=", categoryID).
    FilterGroup(func(q core.Query) {
        q.Filter("Price", ">", 100).
          Filter("Price", "<", 500)
    }).
    All(&products)

// Grouped OR conditions
err := db.Model(&models.Post{}).
    Where("AuthorID", "=", authorID).
    OrFilterGroup(func(q core.Query) {
        q.Filter("Status", "=", "draft").
          Filter("Status", "=", "review")
    }).
    All(&posts)
```

## Advanced Query Features

### Scan Operations

```go
// Full table scan with filters
var users []models.User
err := db.Model(&models.User{}).
    Filter("Active", "=", true).
    Filter("CreatedAt", ">", thirtyDaysAgo).
    Scan(&users)
```

### Batch Operations

```go
// Batch get multiple items
keys := []any{
    &models.User{ID: "user1"},
    &models.User{ID: "user2"},
    &models.User{ID: "user3"},
}
var users []models.User
err := db.Model(&models.User{}).BatchGet(keys, &users)

// Batch create multiple items
posts := []models.Post{
    {ID: "post1", Title: "First Post"},
    {ID: "post2", Title: "Second Post"},
}
err := db.Model(&models.Post{}).BatchCreate(posts)
```

## Transaction Support

### Basic Transaction

```go
err := db.Transaction(func(tx *core.Tx) error {
    // Create a new post
    post := &models.Post{
        ID:       "post123",
        AuthorID: "author456",
        Title:    "New Post",
        Status:   "published",
    }
    if err := tx.Create(post); err != nil {
        return err
    }
    
    // Update author post count
    // Note: UpdateBuilder may not be available in transactions
    // Use regular Update method
    author := &models.Author{
        ID:        "author456",
        PostCount: 11, // New count
        UpdatedAt: time.Now(),
    }
    if err := tx.Update(author, "PostCount", "UpdatedAt"); err != nil {
        return err
    }
    
    return nil
})
```

### Advanced Transaction

```go
// Using TransactionFunc for full control
err := db.TransactionFunc(func(tx *transaction.Transaction) error {
    // Multiple operations in a transaction
    user := &models.User{ID: userID, Balance: 150}
    if err := tx.Update(user); err != nil {
        return err
    }
    
    order := &models.Order{
        OrderID:    orderID,
        CustomerID: userID,
        Total:      50,
        Status:     "completed",
    }
    if err := tx.Create(order); err != nil {
        return err
    }
    
    return nil
})
```

## Error Handling

```go
import "github.com/pay-theory/dynamorm/pkg/errors"

// Check for specific errors
err := db.Model(&models.User{}).
    Where("Email", "=", email).
    First(&user)

if errors.Is(err, errors.ErrItemNotFound) {
    // Handle not found
}

// Handle conditional check failures
err := db.Model(&models.User{
    ID: userID,
}).UpdateBuilder().
    Set("Email", newEmail).
    ConditionNotExists("Email").
    Execute()

if errors.Is(err, errors.ErrConditionFailed) {
    // Email already exists
}
```

## Best Practices

1. **Use Indexes**: Always use indexes when possible to avoid expensive scan operations
2. **Limit Results**: Use `Limit()` to control the amount of data returned
3. **Project Fields**: Use `Select()` to retrieve only needed fields
4. **Batch Operations**: Use batch operations for bulk reads/writes
5. **Error Handling**: Always check for specific error types
6. **Atomic Updates**: Use UpdateBuilder for atomic counter operations
7. **Transactions**: Use transactions for operations that must succeed or fail together

## Performance Tips

1. **Query vs Scan**: Prefer Query operations over Scan when possible
2. **Consistent Reads**: Use eventually consistent reads for better performance when strong consistency isn't required
3. **Parallel Scans**: For large table scans, consider using parallel scan segments
4. **Batch Size**: Keep batch operations under 25 items for optimal performance
5. **Index Selection**: Choose the most selective index for your query pattern 
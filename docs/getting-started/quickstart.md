# DynamORM Quickstart Guide

This guide will help you get started with DynamORM, a type-safe ORM for Amazon DynamoDB in Go.

## Installation

```bash
go get github.com/pay-theory/dynamorm
```

## Prerequisites

- Go 1.18 or later
- AWS account with DynamoDB access
- AWS credentials configured (via environment variables, AWS CLI, or IAM role)

## Basic Setup

### 1. Define Your Models

Create your model structs with DynamORM tags:

```go
package models

import "time"

// User model with primary key
type User struct {
    ID        string    `dynamorm:"pk" json:"id"`
    Email     string    `dynamorm:"index:gsi-email,unique" json:"email"`
    Name      string    `json:"name"`
    Active    bool      `json:"active"`
    CreatedAt time.Time `dynamorm:"created_at" json:"created_at"`
    UpdatedAt time.Time `dynamorm:"updated_at" json:"updated_at"`
    Version   int       `dynamorm:"version" json:"version"`
}

// Post model with composite key
type Post struct {
    UserID      string    `dynamorm:"pk" json:"user_id"`
    PostID      string    `dynamorm:"sk" json:"post_id"`
    Title       string    `json:"title"`
    Content     string    `json:"content"`
    PublishedAt time.Time `json:"published_at"`
    Tags        []string  `dynamorm:"set" json:"tags"`
}
```

### 2. Initialize DynamORM

```go
package main

import (
    "log"
    "github.com/pay-theory/dynamorm"
)

func main() {
    // Initialize DynamORM
    db, err := dynamorm.New(dynamorm.Config{
        Region:   "us-east-1",
        Endpoint: "", // Leave empty for AWS, or use "http://localhost:8000" for local
    })
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Auto-create tables in development
    err = db.AutoMigrate(
        &models.User{},
        &models.Post{},
    )
    if err != nil {
        log.Fatal(err)
    }
}
```

## Basic Operations

### Creating Records

```go
// Create a new user
user := &models.User{
    ID:     "user-123",
    Email:  "john@example.com",
    Name:   "John Doe",
    Active: true,
}

err := db.Model(user).Create()
if err != nil {
    log.Printf("Failed to create user: %v", err)
}
```

### Reading Records

```go
// Find by primary key
var user models.User
err := db.Model(&models.User{}).
    Where("ID", "=", "user-123").
    First(&user)

// Find by index
err := db.Model(&models.User{}).
    Index("gsi-email").
    Where("Email", "=", "john@example.com").
    First(&user)

// Get all active users
var users []models.User
err := db.Model(&models.User{}).
    Where("Active", "=", true).
    All(&users)
```

### Updating Records

```go
// Update specific fields
err := db.Model(&models.User{}).
    Where("ID", "=", "user-123").
    Update("Name", "Active")

// Using UpdateBuilder for atomic operations
err := db.Model(&models.User{
    ID: "user-123",
}).UpdateBuilder().
    Set("Name", "Jane Doe").
    Set("UpdatedAt", time.Now()).
    Execute()
```

### Deleting Records

```go
// Delete by primary key
err := db.Model(&models.User{}).
    Where("ID", "=", "user-123").
    Delete()
```

## Advanced Features

### Composite Keys

```go
// Create with composite key
post := &models.Post{
    UserID:      "user-123",
    PostID:      "post-456",
    Title:       "My First Post",
    Content:     "Hello, World!",
    PublishedAt: time.Now(),
    Tags:        []string{"golang", "dynamodb"},
}
err := db.Model(post).Create()

// Query with composite key
var post models.Post
err := db.Model(&models.Post{}).
    Where("UserID", "=", "user-123").
    Where("PostID", "=", "post-456").
    First(&post)

// Get all posts by user
var posts []models.Post
err := db.Model(&models.Post{}).
    Where("UserID", "=", "user-123").
    All(&posts)
```

### Atomic Counters

```go
// Define a model with counters
type Stats struct {
    ID         string `dynamorm:"pk"`
    PageViews  int    `json:"page_views"`
    Downloads  int    `json:"downloads"`
    LastUpdate time.Time `json:"last_update"`
}

// Increment counters atomically
err := db.Model(&Stats{
    ID: "site-stats",
}).UpdateBuilder().
    Increment("PageViews").
    Add("Downloads", 5).
    Set("LastUpdate", time.Now()).
    Execute()
```

### Transactions

```go
// Perform multiple operations atomically
err := db.Transaction(func(tx *dynamorm.Tx) error {
    // Create a user
    user := &models.User{
        ID:    "user-789",
        Email: "new@example.com",
        Name:  "New User",
    }
    if err := tx.Create(user); err != nil {
        return err
    }

    // Create initial post
    post := &models.Post{
        UserID:  "user-789",
        PostID:  "post-001",
        Title:   "Welcome!",
        Content: "My first post",
    }
    if err := tx.Create(post); err != nil {
        return err
    }

    return nil
})
```

### Batch Operations

```go
// Batch get multiple items
keys := []any{
    &models.User{ID: "user-1"},
    &models.User{ID: "user-2"},
    &models.User{ID: "user-3"},
}
var users []models.User
err := db.Model(&models.User{}).BatchGet(keys, &users)

// Batch create
users := []models.User{
    {ID: "user-4", Email: "user4@example.com", Name: "User 4"},
    {ID: "user-5", Email: "user5@example.com", Name: "User 5"},
}
err := db.Model(&models.User{}).BatchCreate(users)
```

### Query Filters

```go
// Complex queries with filters
var posts []models.Post
err := db.Model(&models.Post{}).
    Where("UserID", "=", "user-123").
    Filter("PublishedAt", ">", time.Now().AddDate(0, -1, 0)).
    OrderBy("PublishedAt", "desc").
    Limit(10).
    All(&posts)

// Using IN operator
var users []models.User
err := db.Model(&models.User{}).
    Where("ID", "IN", []string{"user-1", "user-2", "user-3"}).
    All(&users)

// String operations
err := db.Model(&models.User{}).
    Where("Email", "BEGINS_WITH", "admin@").
    All(&users)
```

### Pagination

```go
// Cursor-based pagination
type PageResult struct {
    Items      []models.Post
    NextCursor string
}

func getPaginatedPosts(cursor string, limit int) (*PageResult, error) {
    query := db.Model(&models.Post{}).
        Where("UserID", "=", "user-123").
        OrderBy("PublishedAt", "desc").
        Limit(limit)
    
    if cursor != "" {
        query = query.Cursor(cursor)
    }
    
    var posts []models.Post
    result, err := query.AllPaginated(&posts)
    if err != nil {
        return nil, err
    }
    
    return &PageResult{
        Items:      posts,
        NextCursor: result.NextCursor,
    }, nil
}
```

## Best Practices

### 1. Model Design

```go
// Good: Use meaningful primary keys
type Order struct {
    OrderID    string `dynamorm:"pk,prefix:ORDER#"`
    CustomerID string `dynamorm:"sk,prefix:CUST#"`
    // ...
}

// Good: Use GSIs for access patterns
type Product struct {
    ProductID  string `dynamorm:"pk"`
    CategoryID string `dynamorm:"index:gsi-category,pk"`
    Price      float64 `dynamorm:"index:gsi-category,sk"`
    // ...
}
```

### 2. Error Handling

```go
import "github.com/pay-theory/dynamorm/pkg/errors"

err := db.Model(&models.User{}).
    Where("Email", "=", email).
    First(&user)

switch {
case errors.Is(err, errors.ErrItemNotFound):
    // Handle not found
    log.Println("User not found")
case errors.Is(err, errors.ErrConditionFailed):
    // Handle condition failure
    log.Println("Condition check failed")
case err != nil:
    // Handle other errors
    return fmt.Errorf("database error: %w", err)
}
```

### 3. Optimistic Locking

```go
// Model with version field
type Document struct {
    ID      string `dynamorm:"pk"`
    Content string
    Version int `dynamorm:"version"`
}

// Update with version check
doc := &Document{ID: "doc-123"}
err := db.Model(doc).First(doc)

// Update content
err = db.Model(&Document{
    ID: "doc-123",
}).UpdateBuilder().
    Set("Content", "Updated content").
    Add("Version", 1).
    ConditionVersion(doc.Version).
    Execute()

if errors.Is(err, errors.ErrConditionFailed) {
    // Document was modified by another process
}
```

### 4. Time-To-Live (TTL)

```go
// Model with TTL
type Session struct {
    SessionID string    `dynamorm:"pk"`
    UserID    string    
    ExpiresAt time.Time `dynamorm:"ttl"`
}

// Create session that expires in 24 hours
session := &Session{
    SessionID: "sess-123",
    UserID:    "user-456",
    ExpiresAt: time.Now().Add(24 * time.Hour),
}
err := db.Model(session).Create()
```

## Testing

```go
// Use local DynamoDB for testing
func setupTestDB(t *testing.T) *dynamorm.DB {
    db, err := dynamorm.New(dynamorm.Config{
        Region:   "us-east-1",
        Endpoint: "http://localhost:8000",
    })
    require.NoError(t, err)
    
    // Create test tables
    err = db.AutoMigrate(&models.User{}, &models.Post{})
    require.NoError(t, err)
    
    return db
}

func TestUserCreation(t *testing.T) {
    db := setupTestDB(t)
    
    user := &models.User{
        ID:    "test-user",
        Email: "test@example.com",
        Name:  "Test User",
    }
    
    err := db.Model(user).Create()
    assert.NoError(t, err)
    
    // Verify
    var retrieved models.User
    err = db.Model(&models.User{}).
        Where("ID", "=", "test-user").
        First(&retrieved)
    
    assert.NoError(t, err)
    assert.Equal(t, user.Email, retrieved.Email)
}
```

## Next Steps

- Read the [Query API Reference](../reference/query-api.md) for detailed query options
- Learn about [Table Operations](../guides/table-operations.md)
- Explore [Lambda Deployment](../guides/lambda-deployment.md) patterns
- Review [Performance Tuning](../guides/performance-tuning.md) guide

## Common Gotchas

1. **Empty Strings**: DynamoDB doesn't support empty strings. Use omitempty tags or default values.
2. **Batch Limits**: Batch operations are limited to 25 items per request.
3. **Index Projection**: Consider what attributes to project in your GSIs to optimize costs.
4. **Hot Partitions**: Design your keys to distribute load evenly.
5. **Eventually Consistent**: Remember that GSI queries are eventually consistent by default. 
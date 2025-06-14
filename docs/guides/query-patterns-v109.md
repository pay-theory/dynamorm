# DynamORM v1.0.9 Query Patterns Guide

This guide provides comprehensive examples of query patterns in DynamORM v1.0.9, clarifying the API changes and best practices.

## Table of Contents
- [Basic Queries](#basic-queries)
- [Index Queries](#index-queries)
- [Complex Queries](#complex-queries)
- [Batch Operations](#batch-operations)
- [Update Operations](#update-operations)
- [Common Patterns](#common-patterns)

## Basic Queries

### Single Item Lookup (Primary Key)

```go
// Get item by primary key
var user User
err := db.Model(&User{}).
    Where("ID", "=", "user-123").
    First(&user)

// With composite key (PK/SK)
var order Order
err := db.Model(&Order{}).
    Where("PK", "=", "USER#123").
    Where("SK", "=", "ORDER#456").
    First(&order)
```

### Query Multiple Items

```go
// Get all active users
var users []User
err := db.Model(&User{}).
    Where("Status", "=", "active").
    All(&users)

// With pagination
var users []User
err := db.Model(&User{}).
    Where("Status", "=", "active").
    Limit(20).
    All(&users)
```

### Count Items

```go
// Count matching items
count, err := db.Model(&User{}).
    Where("Status", "=", "active").
    Count()
```

## Index Queries

### Using Global Secondary Index (GSI)

```go
// Query by GSI
var posts []Post
err := db.Model(&Post{}).
    Index("gsi-author").
    Where("AuthorID", "=", "author-123").
    All(&posts)

// GSI with sort key condition
var posts []Post
err := db.Model(&Post{}).
    Index("gsi-status-date").
    Where("Status", "=", "published").
    Where("PublishedAt", ">", "2024-01-01").
    All(&posts)
```

### Local Secondary Index (LSI)

```go
// Query using LSI
var items []Item
err := db.Model(&Item{}).
    Index("lsi-category").
    Where("UserID", "=", "user-123").  // Partition key
    Where("Category", "=", "electronics").  // LSI sort key
    All(&items)
```

## Complex Queries

### Multiple Conditions

```go
// AND conditions
var products []Product
err := db.Model(&Product{}).
    Where("Category", "=", "electronics").
    Where("Price", "BETWEEN", []float64{100, 500}).
    Where("InStock", "=", true).
    All(&products)
```

### String Operations

```go
// BEGINS_WITH operator
var users []User
err := db.Model(&User{}).
    Where("Email", "BEGINS_WITH", "admin@").
    All(&users)

// CONTAINS operator (for sets/lists)
var posts []Post
err := db.Model(&Post{}).
    Where("Tags", "CONTAINS", "golang").
    All(&posts)
```

### IN Operator

```go
// Query with IN operator (up to 100 values)
var users []User
err := db.Model(&User{}).
    Where("ID", "IN", []string{"user-1", "user-2", "user-3"}).
    All(&users)
```

### Existence Checks

```go
// Check if attribute exists
var users []User
err := db.Model(&User{}).
    Where("EmailVerified", "EXISTS", nil).
    All(&users)

// Check if attribute does not exist
var users []User
err := db.Model(&User{}).
    Where("DeletedAt", "NOT_EXISTS", nil).
    All(&users)
```

## Scan Operations

### Basic Scan

```go
// Scan entire table with filter
var users []User
err := db.Model(&User{}).
    Filter("CreatedAt", ">", "2024-01-01").
    Scan(&users)
```

### Scan with Multiple Filters

```go
// Complex scan
var products []Product
err := db.Model(&Product{}).
    Filter("Category", "=", "electronics").
    Filter("Price", "<", 1000).
    Filter("Rating", ">=", 4.0).
    Scan(&products)
```

## Batch Operations

### Batch Get

```go
// Get multiple items by keys
keys := []any{
    &User{ID: "user-1"},
    &User{ID: "user-2"},
    &User{ID: "user-3"},
}
var users []User
err := db.Model(&User{}).BatchGet(keys, &users)
```

### Batch Create

```go
// Create multiple items
users := []User{
    {ID: "user-1", Name: "Alice"},
    {ID: "user-2", Name: "Bob"},
    {ID: "user-3", Name: "Charlie"},
}
err := db.Model(&User{}).BatchCreate(users)
```

### Batch Delete

```go
// Delete multiple items
keys := []any{
    &User{ID: "user-1"},
    &User{ID: "user-2"},
}
err := db.Model(&User{}).BatchDelete(keys)
```

## Update Operations

### Simple Update

```go
// Update specific fields
err := db.Model(&User{ID: "user-123"}).
    UpdateBuilder().
    Set("Name", "Updated Name").
    Set("UpdatedAt", time.Now()).
    Execute()
```

### Atomic Counter Operations

```go
// Increment counter
err := db.Model(&PageView{ID: "page-123"}).
    UpdateBuilder().
    Increment("ViewCount").
    Execute()

// Add/subtract specific value
err := db.Model(&Account{ID: "acc-123"}).
    UpdateBuilder().
    Add("Balance", -50.25).  // Subtract
    Execute()
```

### Conditional Updates

```go
// Update only if condition met
err := db.Model(&User{ID: "user-123"}).
    UpdateBuilder().
    Set("Status", "verified").
    Condition("Status", "=", "pending").
    Execute()

// With version check
err := db.Model(&Document{ID: "doc-123"}).
    UpdateBuilder().
    Set("Content", newContent).
    ConditionVersion(currentVersion).
    Execute()
```

### List Operations

```go
// Append to list
err := db.Model(&AuditLog{ID: "log-123"}).
    UpdateBuilder().
    AppendToList("Events", []string{"login", "view"}).
    Execute()

// Remove from list at index
err := db.Model(&AuditLog{ID: "log-123"}).
    UpdateBuilder().
    RemoveFromListAt("Events", 0).
    Execute()
```

## Common Patterns

### Pagination with Cursor

```go
// First page
var posts []Post
var cursor string
err := db.Model(&Post{}).
    Where("Status", "=", "published").
    Limit(20).
    All(&posts)
// Extract cursor from last item for next page

// Next page
err = db.Model(&Post{}).
    Where("Status", "=", "published").
    Cursor(cursor).
    Limit(20).
    All(&posts)
```

### Order and Limit

```go
// Get top 10 most recent posts
var posts []Post
err := db.Model(&Post{}).
    Where("Status", "=", "published").
    OrderBy("PublishedAt", "desc").
    Limit(10).
    All(&posts)
```

### Select Specific Fields

```go
// Project only needed fields
var users []User
err := db.Model(&User{}).
    Select("ID", "Name", "Email").
    Where("Status", "=", "active").
    All(&users)
```

### Upsert Pattern

```go
// Create or completely replace
user := User{
    ID:    "user-123",
    Name:  "John Doe",
    Email: "john@example.com",
}
err := db.Model(&user).CreateOrUpdate()
```

### Transaction Example

```go
// Multiple operations in transaction
err := db.Transaction(func(tx *dynamorm.Tx) error {
    // Create order
    order := &Order{ID: "order-123", Total: 100}
    if err := tx.Model(order).Create(); err != nil {
        return err
    }
    
    // Update inventory
    return tx.Model(&Product{ID: "prod-456"}).
        UpdateBuilder().
        Add("Stock", -1).
        Execute()
})
```

## Error Handling

```go
import "github.com/pay-theory/dynamorm/pkg/errors"

// Check for specific errors
err := db.Model(&User{}).
    Where("ID", "=", "user-123").
    First(&user)

switch {
case err == nil:
    // Success
case errors.Is(err, errors.ErrItemNotFound):
    // Item doesn't exist
case errors.Is(err, errors.ErrConditionFailed):
    // Conditional check failed
default:
    // Other error
}
```

## Best Practices

1. **Always use indexes** for queries when possible to avoid expensive scans
2. **Use projections** (`Select()`) to reduce data transfer
3. **Batch operations** for bulk actions (max 25 items per batch)
4. **Add conditions** to updates for optimistic locking
5. **Handle errors** appropriately, especially for conditional operations
6. **Use consistent read** only when necessary (eventually consistent is faster)

## Migration from Earlier Versions

If migrating from v1.0.8 or earlier:

1. Ensure all `Where()` calls use 3 parameters
2. Replace `Find(&results)` with `All(&results)`
3. Add destination parameter to `First()` calls
4. Update index query syntax to use `Model().Index().Where()`

See the [v1.0.9 release notes](../releases/v1.0.9-performance.md) for complete details. 
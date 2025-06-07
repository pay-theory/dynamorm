# Basic Usage Guide

This guide covers the core concepts and basic operations in DynamORM. After reading this, you'll understand how to effectively use DynamORM in your applications.

## Table of Contents

- [Defining Models](#defining-models)
- [CRUD Operations](#crud-operations)
- [Querying Data](#querying-data)
- [Batch Operations](#batch-operations)
- [Transactions](#transactions)
- [Error Handling](#error-handling)
- [Best Practices](#best-practices)

## Defining Models

Models in DynamORM are regular Go structs with special tags that define how they map to DynamoDB tables.

### Basic Model

```go
type User struct {
    // Primary key
    ID string `dynamorm:"pk"`
    
    // Regular attributes
    Email     string
    Name      string
    Age       int
    Active    bool
    Tags      []string              // Lists
    Metadata  map[string]string     // Maps
    
    // Special fields
    CreatedAt time.Time `dynamorm:"created_at"` // Auto-set on create
    UpdatedAt time.Time `dynamorm:"updated_at"` // Auto-set on update
    Version   int       `dynamorm:"version"`    // Optimistic locking
}
```

### Composite Keys

For tables with both partition and sort keys:

```go
type Order struct {
    UserID    string    `dynamorm:"pk"`        // Partition key
    OrderID   string    `dynamorm:"sk"`        // Sort key
    Amount    float64
    Status    string
    CreatedAt time.Time `dynamorm:"sk,format:2006-01-02"` // Alternative: use date as SK
}
```

### Secondary Indexes

Define Global Secondary Indexes (GSI) and Local Secondary Indexes (LSI):

```go
type Product struct {
    SKU         string  `dynamorm:"pk"`
    Category    string  `dynamorm:"index:gsi-category,pk"`
    Price       float64 `dynamorm:"index:gsi-category,sk"`
    Brand       string  `dynamorm:"index:lsi-brand,sk"`
    Name        string
    Description string
}
```

### Advanced Field Types

```go
type Document struct {
    ID          string                 `dynamorm:"pk"`
    
    // JSON serialization
    Data        json.RawMessage        `dynamorm:"json"`
    
    // Sets (unique values)
    Tags        []string               `dynamorm:"set"`
    Numbers     []int                  `dynamorm:"set"`
    
    // Binary data
    Thumbnail   []byte                 `dynamorm:"binary"`
    
    // Custom types (must implement dynamorm.Marshaler/Unmarshaler)
    CustomType  MyCustomType           `dynamorm:"custom"`
    
    // Nested structs
    Address     Address                `dynamorm:"json"`
    
    // Time with custom format
    ExpiresAt   time.Time              `dynamorm:"ttl"`
}
```

## CRUD Operations

### Create (Insert)

```go
// Single item
user := &User{
    ID:     "user-123",
    Email:  "john@example.com",
    Name:   "John Doe",
    Active: true,
}

err := db.Model(user).Create()

// With condition (only create if doesn't exist)
err = db.Model(user).
    Condition("attribute_not_exists(ID)").
    Create()
```

### Read (Query)

```go
// Get by primary key
var user User
err := db.Model(&User{}).
    Where("ID", "=", "user-123").
    First(&user)

// Get multiple items
var users []User
err = db.Model(&User{}).
    Where("Active", "=", true).
    All(&users)

// Get with projection (specific fields only)
err = db.Model(&User{}).
    Select("ID", "Name", "Email").
    Where("ID", "=", "user-123").
    First(&user)
```

### Update

```go
// Update specific fields
err := db.Model(&User{}).
    Where("ID", "=", "user-123").
    Update(map[string]interface{}{
        "Name":   "Jane Doe",
        "Active": false,
    })

// Update single field
err = db.Model(&User{}).
    Where("ID", "=", "user-123").
    Update("Name", "Jane Doe")

// Conditional update
err = db.Model(&User{}).
    Where("ID", "=", "user-123").
    Condition("Active = :active", dynamorm.Param("active", true)).
    Update("Status", "premium")

// Update with expressions
err = db.Model(&User{}).
    Where("ID", "=", "user-123").
    UpdateExpr("SET Age = Age + :inc", dynamorm.Param("inc", 1))
```

### Delete

```go
// Delete by key
err := db.Model(&User{}).
    Where("ID", "=", "user-123").
    Delete()

// Conditional delete
err = db.Model(&User{}).
    Where("ID", "=", "user-123").
    Condition("Active = :active", dynamorm.Param("active", false)).
    Delete()

// Return deleted item
var deletedUser User
err = db.Model(&User{}).
    Where("ID", "=", "user-123").
    Returning(&deletedUser).
    Delete()
```

## Querying Data

### Basic Queries

```go
// Query with partition key
var orders []Order
err := db.Model(&Order{}).
    Where("UserID", "=", "user-123").
    All(&orders)

// Query with partition and sort key
err = db.Model(&Order{}).
    Where("UserID", "=", "user-123").
    Where("OrderID", "=", "order-456").
    First(&order)

// Query with sort key condition
err = db.Model(&Order{}).
    Where("UserID", "=", "user-123").
    Where("CreatedAt", ">=", startDate).
    Where("CreatedAt", "<=", endDate).
    All(&orders)
```

### Using Indexes

```go
// Query using GSI
var products []Product
err := db.Model(&Product{}).
    Index("gsi-category").
    Where("Category", "=", "Electronics").
    Where("Price", "<=", 1000).
    All(&products)

// Query using LSI
err = db.Model(&Product{}).
    Index("lsi-brand").
    Where("SKU", "=", "ABC123").
    Where("Brand", "=", "Apple").
    All(&products)
```

### Advanced Querying

```go
// Filter expressions (post-query filtering)
var users []User
err := db.Model(&User{}).
    Where("Active", "=", true).
    Filter("Age >= :min AND Age <= :max", 
        dynamorm.Param("min", 18),
        dynamorm.Param("max", 65)).
    All(&users)

// Complex filter with functions
err = db.Model(&User{}).
    Where("Active", "=", true).
    Filter("contains(Email, :domain)", 
        dynamorm.Param("domain", "@company.com")).
    All(&users)

// Sorting
err = db.Model(&Order{}).
    Where("UserID", "=", "user-123").
    OrderBy("CreatedAt", "DESC").
    All(&orders)

// Limit results
err = db.Model(&User{}).
    Where("Active", "=", true).
    Limit(10).
    All(&users)
```

### Pagination

```go
// First page
page1, err := db.Model(&User{}).
    Where("Active", "=", true).
    Limit(10).
    Paginate()

// Process results
for _, user := range page1.Items {
    fmt.Println(user.Name)
}

// Next page
if page1.HasMore() {
    page2, err := db.Model(&User{}).
        Where("Active", "=", true).
        Cursor(page1.NextCursor).
        Limit(10).
        Paginate()
}
```

## Batch Operations

### Batch Get

```go
// Get multiple items by keys
keys := []interface{}{
    "user-123",
    "user-456",
    "user-789",
}

var users []User
err := db.Model(&User{}).BatchGet(keys, &users)

// Batch get with composite keys
keys = []interface{}{
    map[string]interface{}{"UserID": "u1", "OrderID": "o1"},
    map[string]interface{}{"UserID": "u1", "OrderID": "o2"},
}

var orders []Order
err = db.Model(&Order{}).BatchGet(keys, &orders)
```

### Batch Write

```go
// Batch create
users := []*User{
    {ID: "user-001", Name: "Alice"},
    {ID: "user-002", Name: "Bob"},
    {ID: "user-003", Name: "Charlie"},
}

err := db.Model(&User{}).BatchCreate(users)

// Mixed batch operations
err = db.BatchWrite().
    Put(&User{ID: "user-004", Name: "David"}).
    Update(&User{ID: "user-001"}, map[string]interface{}{"Active": false}).
    Delete(&User{ID: "user-002"}).
    Execute()
```

## Transactions

DynamORM supports DynamoDB transactions for atomic operations:

```go
// Transaction with function
err := db.Transaction(func(tx *dynamorm.Tx) error {
    // All operations here are atomic
    
    // Create new order
    order := &Order{
        UserID:  "user-123",
        OrderID: "order-789",
        Amount:  99.99,
    }
    if err := tx.Model(order).Create(); err != nil {
        return err
    }
    
    // Update user balance
    if err := tx.Model(&User{}).
        Where("ID", "=", "user-123").
        UpdateExpr("SET Balance = Balance - :amount", 
            dynamorm.Param("amount", 99.99)); err != nil {
        return err
    }
    
    // Update inventory
    if err := tx.Model(&Product{}).
        Where("SKU", "=", "PROD-123").
        UpdateExpr("SET Stock = Stock - :qty", 
            dynamorm.Param("qty", 1)); err != nil {
        return err
    }
    
    return nil // Commit transaction
})

// Manual transaction control
tx := db.BeginTransaction()

// Add operations
tx.Put(&User{ID: "user-999", Name: "Transaction User"})
tx.Update(&Product{SKU: "ABC"}, map[string]interface{}{"Price": 29.99})
tx.Delete(&Order{UserID: "u1", OrderID: "o1"})

// Execute transaction
err = tx.Commit()
// Or rollback: tx.Rollback()
```

## Error Handling

DynamORM provides typed errors for better error handling:

```go
import "github.com/dynamorm/dynamorm/errors"

// Check for specific errors
user := &User{}
err := db.Model(&User{}).
    Where("ID", "=", "nonexistent").
    First(user)

switch {
case errors.Is(err, errors.ErrItemNotFound):
    // Handle not found
    fmt.Println("User not found")
    
case errors.Is(err, errors.ErrConditionalCheckFailed):
    // Handle conditional check failure
    fmt.Println("Condition not met")
    
case errors.Is(err, errors.ErrValidation):
    // Handle validation error
    fmt.Println("Invalid input:", err)
    
case err != nil:
    // Handle other errors
    return fmt.Errorf("database error: %w", err)
}

// Get detailed error information
if dynErr, ok := errors.AsDynamoDBError(err); ok {
    fmt.Printf("AWS Error: %s - %s\n", dynErr.Code(), dynErr.Message())
}
```

## Best Practices

### 1. Model Design

```go
// Good: Meaningful partition key with good distribution
type Order struct {
    CustomerID string    `dynamorm:"pk"`        // Good distribution
    OrderID    string    `dynamorm:"sk"`        // Unique within customer
    // ...
}

// Bad: Partition key with poor distribution
type Order struct {
    Status  string `dynamorm:"pk"`  // Only a few possible values!
    OrderID string `dynamorm:"sk"`
    // ...
}
```

### 2. Use Appropriate Indexes

```go
// Define indexes for common access patterns
type User struct {
    ID       string `dynamorm:"pk"`
    Email    string `dynamorm:"index:gsi-email"`    // Query by email
    TenantID string `dynamorm:"index:gsi-tenant,pk"` // Multi-tenancy
    Status   string `dynamorm:"index:gsi-tenant,sk"` // Filter by status
}
```

### 3. Efficient Queries

```go
// Good: Use index and limit results
users, err := db.Model(&User{}).
    Index("gsi-tenant").
    Where("TenantID", "=", tenantID).
    Where("Status", "=", "active").
    Limit(100).
    All(&users)

// Bad: Scan entire table
users, err = db.Model(&User{}).
    Filter("TenantID = :tid", dynamorm.Param("tid", tenantID)).
    All(&users) // This scans the entire table!
```

### 4. Handle Errors Properly

```go
// Always check and handle errors appropriately
result, err := db.Model(&User{}).
    Where("ID", "=", userID).
    First(&user)

if errors.Is(err, errors.ErrItemNotFound) {
    // Create new user
    user = &User{ID: userID}
    err = db.Model(user).Create()
} else if err != nil {
    return fmt.Errorf("failed to get user: %w", err)
}
```

### 5. Use Batch Operations

```go
// Good: Batch operations for multiple items
err := db.Model(&User{}).BatchCreate(users)

// Less efficient: Individual operations
for _, user := range users {
    err := db.Model(user).Create() // Multiple API calls!
}
```

## Next Steps

Now that you understand the basics:

1. Explore [Advanced Features](../guides/advanced-features.md)
2. Learn about [Performance Optimization](../guides/performance-tuning.md)
3. Check out [Real-World Examples](../../examples/)
4. Read the [API Reference](../reference/api.md)

---

<p align="center">
  💪 You're now ready to build powerful applications with DynamORM!
</p> 
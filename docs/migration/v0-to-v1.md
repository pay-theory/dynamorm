# Migration Guide: v0.x to v1.0.9

This guide helps you migrate from DynamORM v0.x to v1.0.9. The new version introduces interface-based design, better testability, and some breaking changes.

## Overview

### Major Changes
1. **Import Path**: Now uses `github.com/pay-theory/dynamorm`
2. **Initialization**: New configuration-based setup
3. **Interfaces**: DB operations now use interfaces for better testing
4. **Composite Keys**: New PK/SK pattern for complex keys
5. **Error Handling**: Standardized error types
6. **Query API**: Refined method signatures

## Step-by-Step Migration

### 1. Update Import Paths

#### v0.x
```go
import "github.com/your-old-path/dynamorm"
```

#### v1.0.9
```go
import (
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
)
```

### 2. Update Initialization

#### v0.x
```go
db := dynamorm.New()
// or
db, err := dynamorm.NewWithConfig(config)
```

#### v1.0.9
```go
import "github.com/pay-theory/dynamorm/pkg/session"

config := session.Config{
    Region: "us-east-1",
}
db, err := dynamorm.New(config)
if err != nil {
    log.Fatal(err)
}
```

### 3. Update Model Tags

#### v0.x
```go
type User struct {
    ID    string `dynamodb:"id,hash"`
    Email string `dynamodb:"email,range"`
    Name  string `dynamodb:"name"`
}
```

#### v1.0.9
```go
type User struct {
    ID    string `dynamorm:"pk"`
    Email string `dynamorm:"sk"`
    Name  string `json:"name"`
}
```

### 4. Update Composite Keys

#### v0.x
```go
type Order struct {
    UserID  string `dynamodb:"user_id,hash"`
    OrderID string `dynamodb:"order_id,range"`
}
```

#### v1.0.9 (Use PK/SK pattern)
```go
type Order struct {
    PK      string `dynamorm:"pk"` // "USER#123"
    SK      string `dynamorm:"sk"` // "ORDER#456"
    UserID  string `json:"user_id"`
    OrderID string `json:"order_id"`
}

func (o *Order) SetKeys() {
    o.PK = fmt.Sprintf("USER#%s", o.UserID)
    o.SK = fmt.Sprintf("ORDER#%s", o.OrderID)
}
```

### 5. Update CRUD Operations

#### v0.x
```go
// Create
db.Put(user).Run()

// Read
db.Get("id", "123").One(&user)

// Update
db.Update("id", "123").Set("name", "John").Run()

// Delete
db.Delete("id", "123").Run()
```

#### v1.0.9
```go
// Create
err := db.Model(&user).Create()

// Read
err := db.Model(&User{}).
    Where("ID", "=", "123").
    First(&user)

// Update
err := db.Model(&User{ID: "123"}).
    UpdateBuilder().
    Set("Name", "John").
    Execute()

// Delete  
err := db.Model(&User{ID: "123"}).Delete()
```

### 6. Update Query Operations

#### v0.x
```go
var users []User
db.Scan().Filter("active", true).All(&users)
```

#### v1.0.9
```go
var users []User
err := db.Model(&User{}).
    Filter("Active", "=", true).
    Scan(&users)
```

### 7. Update Error Handling

#### v0.x
```go
if err == dynamorm.ErrNotFound {
    // handle not found
}
```

#### v1.0.9
```go
import "github.com/pay-theory/dynamorm/pkg/errors"

if errors.Is(err, errors.ErrItemNotFound) {
    // handle not found
}
```

### 8. Testing Updates

#### v0.x (Difficult to mock)
```go
// Required real DynamoDB connection
db := dynamorm.New()
// Hard to unit test
```

#### v1.0.9 (Easy to mock)
```go
import "github.com/pay-theory/dynamorm/pkg/core"

type Service struct {
    db core.DB // Use interface
}

// In tests
import "github.com/pay-theory/dynamorm/pkg/mocks"

mockDB := new(mocks.MockDB)
service := Service{db: mockDB}
```

## Common Patterns

### Before (v0.x)
```go
// Complex query
result := db.Query("UserIndex").
    Hash("user_id", "123").
    Range("created_at", ">", "2024-01-01").
    Limit(10).
    Run()
```

### After (v1.0.9)
```go
// Cleaner query
var items []Item
err := db.Model(&Item{}).
    Index("UserIndex").
    Where("UserID", "=", "123").
    Where("CreatedAt", ">", "2024-01-01").
    Limit(10).
    All(&items)
```

## Breaking Changes Summary

1. **No Default DB Instance**: Must explicitly create with `New(config)`
2. **Model-Centric API**: Operations start with `db.Model()`
3. **Explicit Error Returns**: All operations return errors
4. **New Tag Format**: Use `dynamorm:` tags instead of `dynamodb:`
5. **PK/SK Pattern**: For composite keys
6. **Interface Types**: DB operations use interfaces

## Need Help?

- Check the [examples](../examples/) for working code
- Review [API documentation](../reference/)
- See [troubleshooting guide](../troubleshooting/)

## Quick Reference Card

| Operation | v0.x | v1.0.9 |
|-----------|------|---------|
| Import | `github.com/old/dynamorm` | `github.com/pay-theory/dynamorm` |
| Initialize | `dynamorm.New()` | `dynamorm.New(session.Config{})` |
| Create | `db.Put(item).Run()` | `db.Model(item).Create()` |
| Read | `db.Get(hash, range).One(&item)` | `db.Model(&Type{}).Where(...).First(&item)` |
| Update | `db.Update(keys).Set(...).Run()` | `db.Model(item).UpdateBuilder().Set(...).Execute()` |
| Delete | `db.Delete(keys).Run()` | `db.Model(item).Delete()` |
| Query | `db.Query(index).Hash(...).Run()` | `db.Model(&Type{}).Index(...).Where(...).All(&items)` |
| Scan | `db.Scan().Filter(...).All(&items)` | `db.Model(&Type{}).Filter(...).Scan(&items)` | 
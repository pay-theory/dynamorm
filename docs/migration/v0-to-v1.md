# Migration Guide: v0.x to v1.0.2

This guide helps you migrate from DynamORM v0.x to v1.0.2. The new version introduces interface-based design, better testability, and some breaking changes.

## Breaking Changes Overview

1. **Initialization syntax changed**
2. **Import paths changed**
3. **Composite key syntax not supported**
4. **Table name tags removed**
5. **Return types now use interfaces**

## 1. Initialization Changes

### v0.x (Old)
```go
import (
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/dynamorm/dynamorm"
)

// Create DynamoDB client
client := dynamodb.NewFromConfig(cfg)

// Initialize DynamORM with client
db := dynamorm.New(client)
```

### v1.0.2 (New)
```go
import (
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
)

// Initialize with configuration
config := session.Config{
    Region: "us-east-1",
}

db, err := dynamorm.New(config)
if err != nil {
    log.Fatal(err)
}
```

## 2. Import Path Changes

### v0.x
```go
import "github.com/dynamorm/dynamorm"
```

### v1.0.2
```go
import (
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
    "github.com/pay-theory/dynamorm/pkg/core"
)
```

## 3. Model Definition Changes

### Table Name Tags (Removed)

#### v0.x
```go
type User struct {
    ID    string `dynamorm:"pk,table:users"`
    Email string `dynamorm:"sk"`
}
```

#### v1.0.2
```go
type User struct {
    ID    string `dynamorm:"pk"`  // Table name derived from struct name
    Email string `dynamorm:"sk"`
}
// Table name will be "Users" (pluralized struct name)
```

### Composite Keys (Not Supported)

#### v0.x (If you were using composite syntax)
```go
type Session struct {
    ID        string `dynamorm:"pk,composite:partner_id,session_id"`
    PartnerID string `dynamorm:"extract:partner_id"`
    SessionID string `dynamorm:"extract:session_id"`
}
```

#### v1.0.2 (Use PK/SK pattern)
```go
type Session struct {
    PK        string `dynamorm:"pk"` // Store partner_id
    SK        string `dynamorm:"sk"` // Store session_id
    PartnerID string
    SessionID string
}

func (s *Session) SetKeys() {
    s.PK = s.PartnerID
    s.SK = s.SessionID
}
```

## 4. Interface-Based Returns

### v0.x
```go
func InitDB() *dynamorm.DB {
    return dynamorm.New(client)
}
```

### v1.0.2
```go
func InitDB() (core.DB, error) {
    config := session.Config{Region: "us-east-1"}
    return dynamorm.New(config)
}

// Or for extended features:
func InitExtendedDB() (core.ExtendedDB, error) {
    config := session.Config{Region: "us-east-1"}
    return dynamorm.New(config)
}
```

## 5. Query Changes

The query interface remains similar, but some methods have been added:

### New in v1.0.2
```go
// Filter groups
db.Model(&User{}).
    FilterGroup(func(q core.Query) {
        q.Filter("Age", ">", 18).
          Filter("Age", "<", 65)
    }).
    All(&users)

// Update builder
db.Model(&Counter{ID: "123"}).
    UpdateBuilder().
    Add("Count", 1).        // Atomic increment
    Execute()
```

## 6. Testing Changes

### v0.x (Difficult to mock)
```go
type Service struct {
    db *dynamorm.DB  // Concrete type - hard to mock
}
```

### v1.0.2 (Easy to mock)
```go
import (
    "github.com/pay-theory/dynamorm/pkg/core"
    "github.com/pay-theory/dynamorm/pkg/mocks"
)

type Service struct {
    db core.DB  // Interface - easy to mock
}

// In tests:
mockDB := new(mocks.MockDB)
mockQuery := new(mocks.MockQuery)
```

## 7. Table Operations

### v0.x
```go
// Manual table creation via AWS SDK
```

### v1.0.2
```go
// Development helpers
err := db.CreateTable(&User{})
err := db.EnsureTable(&User{})
err := db.AutoMigrate(&User{})
```

## Complete Migration Example

### Before (v0.x)
```go
package main

import (
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/dynamorm/dynamorm"
)

type User struct {
    ID    string `dynamorm:"pk,table:my-users"`
    Email string `dynamorm:"sk"`
    Name  string
}

func main() {
    cfg, _ := config.LoadDefaultConfig(context.Background())
    client := dynamodb.NewFromConfig(cfg)
    
    db := dynamorm.New(client)
    
    user := &User{
        ID:    "user123",
        Email: "john@example.com",
        Name:  "John Doe",
    }
    
    db.Create(user)
}
```

### After (v1.0.2)
```go
package main

import (
    "log"
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
)

// Note: Table will be "Users" (pluralized)
// If you need "my-users", rename struct to "MyUser"
type User struct {
    ID    string `dynamorm:"pk"`
    Email string `dynamorm:"sk"`
    Name  string
}

func main() {
    config := session.Config{
        Region: "us-east-1",
    }
    
    db, err := dynamorm.New(config)
    if err != nil {
        log.Fatal(err)
    }
    
    user := &User{
        ID:    "user123",
        Email: "john@example.com",
        Name:  "John Doe",
    }
    
    if err := db.Model(user).Create(); err != nil {
        log.Printf("Create failed: %v", err)
    }
}
```

## Migration Checklist

- [ ] Update all imports to use `github.com/pay-theory/dynamorm`
- [ ] Add `pkg/session` import for Config
- [ ] Change initialization from `New(client)` to `New(config)`
- [ ] Remove any `table:` tags from struct fields
- [ ] Convert composite key syntax to PK/SK pattern
- [ ] Add `SetKeys()` methods for composite key models
- [ ] Update function signatures to return interfaces (`core.DB`)
- [ ] Update tests to use mocks from `pkg/mocks`
- [ ] Handle table name changes (struct name pluralization)
- [ ] Add error handling to initialization

## Common Issues During Migration

### 1. Nil Pointer Dereference
**Cause**: Using old initialization syntax
**Fix**: Use `session.Config` as shown above

### 2. Table Not Found
**Cause**: Table names changed due to automatic pluralization
**Fix**: Either rename structs or recreate tables with new names

### 3. Missing Primary Key
**Cause**: Using composite key syntax that's no longer supported
**Fix**: Switch to PK/SK pattern with helper methods

### 4. Type Mismatch
**Cause**: Functions expecting concrete `*dynamorm.DB` type
**Fix**: Update to use `core.DB` interface

## Gradual Migration Strategy

1. **Update imports and initialization first**
2. **Fix compilation errors**
3. **Update models one at a time**
4. **Test each model after updating**
5. **Update tests to use new mocking approach**
6. **Deploy with careful monitoring**

## Getting Help

If you encounter issues not covered here:

1. Check the [Troubleshooting Guide](../troubleshooting/nil-pointer-fix.md)
2. Review the [Composite Keys Guide](../guides/composite-keys.md)
3. See [Installation Guide](../getting-started/installation.md) for correct setup
4. Open an issue on GitHub with specific error messages 
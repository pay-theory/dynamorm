# DynamORM Struct Tag Specification

This document defines the struct tag syntax and behavior for DynamORM models.

## Overview

DynamORM uses struct tags to configure how Go structs map to DynamoDB tables. Tags are specified using the `dynamorm` key.

## Basic Syntax

```go
type Model struct {
    Field Type `dynamorm:"tag1,tag2:value,tag3"`
}
```

## Tag Reference

### Key Tags

#### `pk` - Primary Key (Partition Key)
```go
ID string `dynamorm:"pk"`
```
- Marks field as the table's partition key
- Only one field per struct can be `pk`
- Field must be string, number, or binary

#### `sk` - Sort Key (Range Key)
```go
CreatedAt time.Time `dynamorm:"sk"`
```
- Marks field as the table's sort key
- Optional - tables can have just a partition key
- Field must be string, number, or binary

### Attribute Tags

#### `attr:name` - Custom Attribute Name
```go
UserName string `dynamorm:"attr:username"`
```
- Maps Go field to different DynamoDB attribute name
- Default: field name is used as-is

#### `omitempty` - Omit Empty Values
```go
Description string `dynamorm:"omitempty"`
```
- Don't save attribute if value is zero/empty
- Useful for optional fields

#### `set` - DynamoDB Set Type
```go
Tags []string `dynamorm:"set"`
```
- Store slice as DynamoDB Set (SS, NS, or BS)
- Ensures uniqueness and enables set operations

### Index Tags

#### `index:name` - Global Secondary Index
```go
Email string `dynamorm:"index:gsi-email"`
```
- Field is partition key for the named GSI
- Multiple fields can reference same index

#### `index:name,pk` - GSI Partition Key
```go
Category string `dynamorm:"index:gsi-category-price,pk"`
```
- Explicitly marks field as partition key for GSI
- Required when GSI has composite key

#### `index:name,sk` - GSI Sort Key
```go
Price float64 `dynamorm:"index:gsi-category-price,sk"`
```
- Field is sort key for the named GSI
- Must be paired with a pk field for same index

#### `lsi:name` - Local Secondary Index
```go
Status string `dynamorm:"lsi:lsi-status"`
```
- Field is sort key for the named LSI
- LSI always shares partition key with main table

### Special Tags

#### `version` - Optimistic Locking
```go
Version int `dynamorm:"version"`
```
- Automatically incremented on updates
- Used for optimistic concurrency control
- Update fails if version doesn't match

#### `ttl` - Time To Live
```go
ExpiresAt int64 `dynamorm:"ttl"`
```
- Unix timestamp for item expiration
- DynamoDB automatically deletes expired items
- Must be number type

#### `created_at` - Creation Timestamp
```go
CreatedAt time.Time `dynamorm:"created_at"`
```
- Automatically set when item is created
- Not updated on subsequent saves

#### `updated_at` - Update Timestamp
```go
UpdatedAt time.Time `dynamorm:"updated_at"`
```
- Automatically updated on every save
- Set to current time on updates

#### `json` - JSON Serialization
```go
Metadata map[string]interface{} `dynamorm:"json"`
```
- Store complex types as JSON string
- Automatic marshal/unmarshal

#### `binary` - Binary Data
```go
Avatar []byte `dynamorm:"binary"`
```
- Store as DynamoDB Binary type
- Useful for images, files, etc.

#### `encrypted` - Client-Side Encryption
```go
SSN string `dynamorm:"encrypted"`
```
- Encrypt field before storing
- Requires encryption key configuration

### Projection Tags

#### `project:always` - Always Project
```go
DisplayName string `dynamorm:"project:always"`
```
- Include in all index projections
- Useful for frequently accessed fields

#### `project:keys_only` - Exclude from Projections
```go
LargeData string `dynamorm:"project:keys_only"`
```
- Exclude from index projections
- Only accessible via main table

## Complex Examples

### E-commerce Product Model
```go
type Product struct {
    // Composite primary key
    SKU        string    `dynamorm:"pk"`
    Version    string    `dynamorm:"sk"`
    
    // Attributes with indexes
    Category   string    `dynamorm:"index:gsi-category,pk"`
    Price      float64   `dynamorm:"index:gsi-category,sk"`
    Brand      string    `dynamorm:"index:gsi-brand"`
    
    // Regular attributes
    Name       string    
    Tags       []string  `dynamorm:"set"`
    Images     []string  
    InStock    bool      `dynamorm:"index:gsi-in-stock,sparse"`
    
    // Special attributes
    CreatedAt  time.Time `dynamorm:"created_at"`
    UpdatedAt  time.Time `dynamorm:"updated_at"`
    ViewCount  int       `dynamorm:"version"`
    TTL        int64     `dynamorm:"ttl"`
}
```

### User Profile Model
```go
type UserProfile struct {
    // Simple primary key
    UserID      string                 `dynamorm:"pk"`
    
    // User info with custom names
    Email       string                 `dynamorm:"attr:email_address,index:gsi-email"`
    UserName    string                 `dynamorm:"attr:username,index:gsi-username"`
    
    // Complex types
    Preferences map[string]interface{} `dynamorm:"json"`
    Avatar      []byte                 `dynamorm:"binary"`
    Followers   []string               `dynamorm:"set"`
    
    // Sensitive data
    Phone       string                 `dynamorm:"encrypted"`
    
    // Metadata
    JoinedAt    time.Time              `dynamorm:"created_at"`
    LastActive  time.Time              `dynamorm:"updated_at"`
    Version     int                    `dynamorm:"version"`
}
```

### Order Model with Composite Keys
```go
type Order struct {
    // Composite key for time-series data
    CustomerID string    `dynamorm:"pk"`
    OrderDate  time.Time `dynamorm:"sk"`
    
    // Order details
    OrderID    string    `dynamorm:"index:gsi-order-id"`
    Status     string    `dynamorm:"lsi:lsi-status"`
    Total      float64   
    
    // Complex nested structure
    Items      []OrderItem `dynamorm:"json"`
    
    // Timestamps
    CreatedAt  time.Time   `dynamorm:"created_at"`
    UpdatedAt  time.Time   `dynamorm:"updated_at"`
}
```

## Tag Validation Rules

1. **Primary Key Requirements**
   - Every model must have exactly one `pk` field
   - `pk` field cannot be nil or empty
   - `pk` must be string, number, or binary type

2. **Sort Key Rules**
   - Maximum one `sk` field per model
   - If present, `sk` cannot be nil or empty
   - `sk` must be string, number, or binary type

3. **Index Rules**
   - GSI names must be unique within a table
   - GSI can have different key schema than main table
   - LSI must share partition key with main table
   - Maximum 5 GSIs and 5 LSIs per table

4. **Type Restrictions**
   - `set` tag only valid on slice types
   - `version` must be numeric type
   - `ttl` must be numeric type
   - `created_at` and `updated_at` must be time.Time

5. **Naming Conventions**
   - Index names should follow pattern: `gsi-{purpose}` or `lsi-{purpose}`
   - Attribute names should be camelCase
   - Avoid DynamoDB reserved words

## Tag Combinations

Some tags can be combined for advanced configurations:

```go
// Optional field with custom name
Field string `dynamorm:"attr:custom_name,omitempty"`

// Indexed field with projection control
Field string `dynamorm:"index:gsi-1,project:always"`

// Sort key that's also in a GSI
Field string `dynamorm:"sk,index:gsi-1,pk"`
```

## Future Tags (Planned)

- `compute:expression` - Computed fields
- `ref:TableName` - Foreign key references
- `unique` - Unique constraint (via transactions)
- `default:value` - Default values
- `validate:regex` - Field validation 
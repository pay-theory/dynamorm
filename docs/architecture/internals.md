# DynamORM Design Document

## Overview

DynamORM is a powerful, type-safe Object-Relational Mapping (ORM) library for Amazon DynamoDB in Go. It aims to simplify DynamoDB interactions by providing an expressive API, automatic index management, and intelligent query building while maintaining the performance and scalability benefits of DynamoDB.

## Core Design Principles

1. **Developer Experience First**: Intuitive API that feels natural to Go developers
2. **Type Safety**: Compile-time checks for queries and operations
3. **Zero Magic**: Explicit, predictable behavior with no hidden complexity
4. **Performance**: Minimal overhead over raw DynamoDB SDK
5. **Flexibility**: Support both simple and complex use cases
6. **Best Practices by Default**: Encourage DynamoDB best practices through API design

## Key Features

### 1. Model Definition
- Struct tag-based configuration
- Automatic table and index creation
- Support for complex types (nested structs, slices, maps)
- Custom type converters
- Validation rules

### 2. Query Builder
- Fluent, chainable API
- Type-safe query conditions
- Automatic index selection
- Support for all DynamoDB operations (Query, Scan, GetItem, etc.)
- Projection expressions
- Filter expressions

### 3. Index Management
- Automatic GSI/LSI detection from struct tags
- Index usage optimization
- Sparse index support
- Composite key helpers
- Index projection configuration

### 4. Schema Management
- Migration support
- Table versioning
- Safe schema evolution
- Backup and restore helpers

### 5. Advanced Features
- Transactions
- Batch operations
- Optimistic locking
- TTL support
- Streams integration
- Global table support

## API Design

### Model Definition Example

```go
type User struct {
    // Primary Key
    ID          string    `dynamorm:"pk"`
    
    // Sort Key (for composite primary key)
    CreatedAt   time.Time `dynamorm:"sk"`
    
    // Attributes
    Email       string    `dynamorm:"index:gsi-email"`
    Name        string    `dynamorm:"index:gsi-name-age,pk"`
    Age         int       `dynamorm:"index:gsi-name-age,sk"`
    Tags        []string  `dynamorm:"set"`
    Preferences map[string]interface{}
    
    // Metadata
    UpdatedAt   time.Time `dynamorm:"updated_at"`
    Version     int       `dynamorm:"version"` // For optimistic locking
    TTL         int64     `dynamorm:"ttl"`
}
```

### Query API Examples

```go
// Simple queries
user, err := orm.Model(&User{}).
    Where("ID", "=", userId).
    First()

// Complex queries with index
users, err := orm.Model(&User{}).
    Index("gsi-email").
    Where("Email", "=", "user@example.com").
    All()

// Query builder with multiple conditions
users, err := orm.Model(&User{}).
    Where("Age", ">", 18).
    Where("Name", "begins_with", "John").
    OrderBy("CreatedAt", "desc").
    Limit(10).
    All()

// Scan with filter
users, err := orm.Model(&User{}).
    Filter("contains(Tags, :tag)", Param("tag", "active")).
    Scan()
```

### Transaction API

```go
err := orm.Transaction(func(tx *Transaction) error {
    // Create user
    user := &User{ID: "123", Name: "John"}
    if err := tx.Create(user); err != nil {
        return err
    }
    
    // Update another record
    if err := tx.Model(&Account{}).
        Where("UserID", "=", "123").
        Update("Balance", 100); err != nil {
        return err
    }
    
    return nil
})
```

### Batch Operations

```go
// Batch write
users := []*User{...}
err := orm.Model(&User{}).BatchCreate(users)

// Batch get
ids := []string{"id1", "id2", "id3"}
users, err := orm.Model(&User{}).BatchGet(ids)
```

## Architecture Components

### 1. Model Registry
- Stores model metadata
- Manages table configurations
- Handles type conversions
- Validates struct tags

### 2. Query Engine
- Builds DynamoDB expressions
- Optimizes query plans
- Selects appropriate indexes
- Handles pagination

### 3. Schema Manager
- Creates/updates tables
- Manages indexes
- Handles migrations
- Monitors table status

### 4. Session Manager
- Connection pooling
- Retry logic
- Error handling
- Metrics collection

### 5. Type System
- Marshal/unmarshal complex types
- Custom type converters
- Null handling
- Set types

## Implementation Phases

### Phase 1: Core Foundation
- Basic model definition
- Simple CRUD operations
- Primary key queries
- Type mapping

### Phase 2: Query Builder
- Where conditions
- Query and Scan operations
- Basic index support
- Pagination

### Phase 3: Advanced Queries
- Complex expressions
- Filter support
- Projection expressions
- Sorting and limits

### Phase 4: Index Management
- GSI/LSI creation
- Index selection algorithm
- Sparse indexes
- Index projections

### Phase 5: Schema Management
- Migration framework
- Table versioning
- Schema validation
- Backup/restore

### Phase 6: Advanced Features
- Transactions
- Batch operations
- Optimistic locking
- TTL support

### Phase 7: Performance & Polish
- Query optimization
- Caching layer
- Metrics and monitoring
- Documentation

## Error Handling

```go
// Typed errors for better handling
var (
    ErrItemNotFound = errors.New("item not found")
    ErrConditionFailed = errors.New("condition check failed")
    ErrIndexNotFound = errors.New("index not found")
)

// Error wrapping with context
type DynamORMError struct {
    Op      string
    Model   string
    Err     error
    Context map[string]interface{}
}
```

## Configuration

```go
// Global configuration
config := dynamorm.Config{
    Region:          "us-east-1",
    Endpoint:        "", // For local development
    MaxRetries:      3,
    DefaultRCU:      5,
    DefaultWCU:      5,
    AutoMigrate:     true,
    EnableMetrics:   true,
}

orm := dynamorm.New(config)
```

## Testing Support

```go
// Mock client for testing
mock := dynamorm.NewMock()
mock.ExpectQuery(&User{}).
    WithCondition("ID", "=", "123").
    WillReturn(&User{ID: "123", Name: "Test"})

// Table fixtures
fixture := dynamorm.Fixture{
    Users: []*User{...},
    Accounts: []*Account{...},
}
orm.LoadFixture(fixture)
```

## Performance Considerations

1. **Intelligent Batching**: Automatic batching of operations when beneficial
2. **Connection Pooling**: Reuse of DynamoDB clients
3. **Query Optimization**: Choose most efficient index automatically
4. **Lazy Loading**: Support for lazy loading of large attributes
5. **Caching**: Optional caching layer for read-heavy workloads

## Migration Example

```go
// Define migration
migration := &Migration{
    Version: "v1.0.0",
    Up: func(schema *Schema) error {
        return schema.CreateTable(&User{}, TableOptions{
            BillingMode: PayPerRequest,
            StreamSpec: &StreamSpecification{
                StreamEnabled: true,
                StreamViewType: NewAndOldImages,
            },
        })
    },
    Down: func(schema *Schema) error {
        return schema.DropTable(&User{})
    },
}

// Run migrations
err := orm.Migrate()
```

## Best Practices Enforcement

1. **Index Usage**: Warn when queries could benefit from indexes
2. **Hot Partitions**: Detect potential hot partition patterns
3. **Large Items**: Warn about items approaching size limits
4. **Cost Optimization**: Suggest more efficient query patterns
5. **Consistency**: Guide on consistency model selection

## Integration Examples

### HTTP Handler
```go
func GetUser(w http.ResponseWriter, r *http.Request) {
    userID := mux.Vars(r)["id"]
    
    user, err := orm.Model(&User{}).
        Where("ID", "=", userID).
        First()
    
    if err == dynamorm.ErrItemNotFound {
        http.NotFound(w, r)
        return
    }
    
    json.NewEncoder(w).Encode(user)
}
```

### GraphQL Resolver
```go
func (r *Resolver) User(ctx context.Context, id string) (*User, error) {
    return orm.Model(&User{}).
        Context(ctx).
        Where("ID", "=", id).
        First()
}
```

## Future Enhancements

1. **Code Generation**: Generate models from existing tables
2. **Admin UI**: Web interface for table management
3. **Multi-Region**: Native support for global tables
4. **Analytics**: Built-in query analytics and optimization
5. **GraphQL Integration**: Direct GraphQL schema generation 
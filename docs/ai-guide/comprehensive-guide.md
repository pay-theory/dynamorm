# DynamORM AI Assistant Comprehensive Guide

This guide provides AI assistants with comprehensive information about DynamORM, a Lambda-native, type-safe ORM for Amazon DynamoDB written in Go.

## Project Overview

DynamORM is designed specifically for serverless architectures, providing:
- **Lambda-Native**: 11ms cold starts (91% faster than AWS SDK)
- **Type-Safe**: Full Go type safety with compile-time checks
- **High Performance**: 20,000+ operations per second
- **Code Reduction**: 80% less code than raw AWS SDK
- **Interface-Driven**: Testable design with pre-built mocks

## Core Architecture

DynamORM follows a layered architecture:
1. **Interface Layer** (`pkg/core/interfaces.go`) - Core abstractions
2. **Query Layer** (`pkg/query/`) - Query building and execution
3. **Model Layer** (`pkg/model/`) - Model metadata and registry
4. **Session Layer** (`pkg/session/`) - DynamoDB connection management
5. **Marshal Layer** (`pkg/marshal/`) - High-performance serialization

## Important Types & Interfaces

### Core Database Interfaces

#### `core.DB` Interface
The basic interface for CRUD operations:
```go
type DB interface {
    Model(model any) Query
    Transaction(fn func(tx *Tx) error) error
    Migrate() error
    AutoMigrate(models ...any) error
    Close() error
    WithContext(ctx context.Context) DB
}
```

#### `core.ExtendedDB` Interface
Full interface including schema management and Lambda features:
```go
type ExtendedDB interface {
    DB  // Embeds basic operations
    
    // Schema management
    AutoMigrateWithOptions(model any, opts ...any) error
    CreateTable(model any, opts ...any) error
    EnsureTable(model any) error
    DeleteTable(model any) error
    DescribeTable(model any) (any, error)
    
    // Lambda optimizations
    WithLambdaTimeout(ctx context.Context) DB
    WithLambdaTimeoutBuffer(buffer time.Duration) DB
    TransactionFunc(fn func(tx any) error) error
}
```

#### `core.Query` Interface
Chainable query builder with 20+ methods:
```go
type Query interface {
    // Query construction
    Where(field string, op string, value any) Query
    Index(indexName string) Query
    Filter(field string, op string, value any) Query
    OrFilter(field string, op string, value any) Query
    FilterGroup(func(Query)) Query
    OrFilterGroup(func(Query)) Query
    OrderBy(field string, order string) Query
    Limit(limit int) Query
    Offset(offset int) Query
    Select(fields ...string) Query
    
    // Execution methods
    First(dest any) error
    All(dest any) error
    AllPaginated(dest any) (*PaginatedResult, error)
    Count() (int64, error)
    
    // CRUD operations
    Create() error
    CreateOrUpdate() error
    Update(fields ...string) error
    UpdateBuilder() UpdateBuilder
    Delete() error
    
    // Advanced operations
    Scan(dest any) error
    ParallelScan(segment int32, totalSegments int32) Query
    ScanAllSegments(dest any, totalSegments int32) error
    
    // Batch operations
    BatchGet(keys []any, dest any) error
    BatchCreate(items any) error
    BatchDelete(keys []any) error
    BatchWrite(putItems []any, deleteKeys []any) error
    BatchUpdateWithOptions(items []any, fields []string, options ...any) error
    
    // Pagination
    Cursor(cursor string) Query
    SetCursor(cursor string) error
    WithContext(ctx context.Context) Query
}
```

#### `core.UpdateBuilder` Interface
Fluent interface for complex update operations:
```go
type UpdateBuilder interface {
    // Basic updates
    Set(field string, value any) UpdateBuilder
    SetIfNotExists(field string, value any, defaultValue any) UpdateBuilder
    
    // Atomic operations
    Add(field string, value any) UpdateBuilder
    Increment(field string) UpdateBuilder
    Decrement(field string) UpdateBuilder
    Remove(field string) UpdateBuilder
    Delete(field string, value any) UpdateBuilder
    
    // List operations
    AppendToList(field string, values any) UpdateBuilder
    PrependToList(field string, values any) UpdateBuilder
    RemoveFromListAt(field string, index int) UpdateBuilder
    SetListElement(field string, index int, value any) UpdateBuilder
    
    // Conditions
    Condition(field string, operator string, value any) UpdateBuilder
    OrCondition(field string, operator string, value any) UpdateBuilder
    ConditionExists(field string) UpdateBuilder
    ConditionNotExists(field string) UpdateBuilder
    ConditionVersion(currentVersion int64) UpdateBuilder
    
    // Execution
    ReturnValues(option string) UpdateBuilder
    Execute() error
    ExecuteWithResult(result any) error
}
```

### Lambda-Specific Types

#### `LambdaDB` Struct
Lambda-optimized database wrapper:
```go
type LambdaDB struct {
    core.ExtendedDB
    db             *DB
    modelCache     *sync.Map
    isLambda       bool
    lambdaMemoryMB int
    xrayEnabled    bool
}
```

#### `MultiAccountDB` Struct
Multi-tenant database for cross-account operations:
```go
type MultiAccountDB struct {
    accounts map[string]AccountConfig
    cache    map[string]*DB
    mu       sync.RWMutex
}

type AccountConfig struct {
    RoleARN    string
    ExternalID string
    Region     string
}
```

### Data Types

#### `PaginatedResult` Struct
Contains paginated query results:
```go
type PaginatedResult struct {
    Items            any
    Count            int
    ScannedCount     int
    LastEvaluatedKey map[string]types.AttributeValue
    NextCursor       string
    HasMore          bool
}
```

#### `CompiledQuery` Struct
Represents a compiled DynamoDB query:
```go
type CompiledQuery struct {
    Operation                 string
    TableName                 string
    IndexName                 string
    KeyConditionExpression    string
    FilterExpression          string
    ProjectionExpression      string
    UpdateExpression          string
    ConditionExpression       string
    ExpressionAttributeNames  map[string]string
    ExpressionAttributeValues map[string]types.AttributeValue
    Limit                     *int32
    ExclusiveStartKey         map[string]types.AttributeValue
    ScanIndexForward          *bool
    Select                    string
    Offset                    *int
    ReturnValues              string
    Segment                   *int32
    TotalSegments             *int32
}
```

## Key Structs & Functions

### Model Definition

Models use struct tags for configuration:
```go
type User struct {
    ID        string    `dynamorm:"pk"`                    // Partition key
    Email     string    `dynamorm:"index:gsi-email"`       // GSI partition key
    CreatedAt time.Time `dynamorm:"sk"`                    // Sort key
    Name      string    `dynamorm:"attr:display_name"`     // Custom attribute name
    Tags      []string  `dynamorm:"set"`                   // DynamoDB set
    Active    bool      `dynamorm:"omitempty"`             // Skip if empty
    Version   int       `dynamorm:"version"`               // Optimistic locking
    UpdatedAt time.Time `dynamorm:"updated_at"`            // Auto-updated
}
```

### Initialization Patterns

#### Basic Initialization
```go
config := session.Config{
    Region: "us-east-1",
}
db, err := dynamorm.New(config)
```

#### Lambda-Optimized Initialization
```go
// Global initialization for connection reuse
var db *dynamorm.LambdaDB

func init() {
    db, _ = dynamorm.LambdaInit(&User{}, &Post{})
}

func handler(ctx context.Context, event Event) error {
    lambdaDB := db.WithLambdaTimeout(ctx)
    return lambdaDB.Model(&User{}).Create()
}
```

#### Multi-Account Setup
```go
accounts := map[string]dynamorm.AccountConfig{
    "prod": {
        RoleARN:    "arn:aws:iam::111111:role/dynamodb-role",
        ExternalID: "external-id",
        Region:     "us-east-1",
    },
}
multiDB, err := dynamorm.NewMultiAccount(accounts)
prodDB, err := multiDB.Partner("prod")
```

### Query Building Patterns

#### Basic Queries
```go
// Find by primary key
var user User
err := db.Model(&User{}).Where("ID", "=", "123").First(&user)

// Query with index
var users []User
err := db.Model(&User{}).
    Index("gsi-email").
    Where("Email", "=", "john@example.com").
    All(&users)

// Complex filtering
err := db.Model(&User{}).
    Where("Status", "=", "active").
    Filter("Age", ">", 18).
    OrderBy("CreatedAt", "DESC").
    Limit(10).
    All(&users)
```

#### Pagination
```go
result, err := db.Model(&User{}).
    Where("Status", "=", "active").
    Limit(25).
    AllPaginated(&users)

if result.HasMore {
    // Next page
    nextResult, err := db.Model(&User{}).
        Where("Status", "=", "active").
        Cursor(result.NextCursor).
        Limit(25).
        AllPaginated(&users)
}
```

#### Update Operations
```go
// Simple update
user.Name = "Updated Name"
err := db.Model(user).Update("Name")

// Complex update with builder
err := db.Model(&User{}).
    Where("ID", "=", "123").
    UpdateBuilder().
    Set("Name", "New Name").
    Increment("LoginCount").
    AppendToList("Tags", []string{"new-tag"}).
    ConditionExists("ID").
    Execute()
```

### Transaction Patterns

```go
err := db.Transaction(func(tx *dynamorm.Tx) error {
    // All operations in transaction
    user.Balance -= 100
    if err := tx.Model(user).Update("Balance"); err != nil {
        return err
    }
    
    transfer := &Transfer{
        FromUserID: user.ID,
        Amount:     100,
    }
    return tx.Model(transfer).Create()
})
```

### Schema Management

```go
// Create table
err := db.CreateTable(&User{})

// Ensure table exists (idempotent)
err := db.EnsureTable(&User{})

// Auto-migrate with data copy
err := db.AutoMigrateWithOptions(&UserV1{},
    dynamorm.WithTargetModel(&UserV2{}),
    dynamorm.WithDataCopy(true),
)
```

## Testing Utilities & Mocks

DynamORM provides comprehensive testing support through the `pkg/mocks` and `pkg/testing` packages.

### Pre-Built Mocks

#### MockDB
```go
import "github.com/pay-theory/dynamorm/pkg/mocks"

mockDB := new(mocks.MockDB)
mockQuery := new(mocks.MockQuery)

mockDB.On("Model", &User{}).Return(mockQuery)
mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
mockQuery.On("First", mock.Anything).Return(nil)
```

#### MockQuery
```go
mockQuery := new(mocks.MockQuery)
mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
mockQuery.On("Filter", "Status", "=", "active").Return(mockQuery)
mockQuery.On("All", mock.Anything).Return(nil)
```

#### MockUpdateBuilder
```go
mockBuilder := new(mocks.MockUpdateBuilder)
mockBuilder.On("Set", "Name", "Updated").Return(mockBuilder)
mockBuilder.On("Increment", "Version").Return(mockBuilder)
mockBuilder.On("Execute").Return(nil)
```

### Testing Helpers

#### TestDB Factory
```go
import "github.com/pay-theory/dynamorm/pkg/testing"

testDB := testing.NewTestDB()

// Fluent expectations
testDB.ExpectModel(&User{}).
    ExpectWhere("ID", "=", "123").
    ExpectFind(&User{ID: "123", Name: "John"})

// Execute test
service := NewUserService(testDB.MockDB)
user, err := service.GetUser("123")
assert.NoError(t, err)
assert.Equal(t, "John", user.Name)

testDB.AssertExpectations(t)
```

#### Common Scenarios
```go
scenarios := testing.NewCommonScenarios(testDB)

// Setup CRUD operations
scenarios.SetupCRUD(&User{})

// Setup pagination
scenarios.SetupPagination(25)

// Setup multi-tenant
scenarios.SetupMultiTenant("tenant123")

// Setup batch operations
scenarios.SetupBatchOperations()
```

### Integration Testing

#### DynamoDB Local Setup
```go
func TestWithDynamoDBLocal(t *testing.T) {
    // Start DynamoDB Local container
    config := session.Config{
        Endpoint: "http://localhost:8000",
        Region:   "us-east-1",
    }
    
    db, err := dynamorm.New(config)
    require.NoError(t, err)
    
    // Create test table
    err = db.CreateTable(&User{})
    require.NoError(t, err)
    
    // Run tests
    user := &User{ID: "test", Name: "Test User"}
    err = db.Model(user).Create()
    assert.NoError(t, err)
}
```

#### Test Data Factories
```go
func CreateTestUser(overrides ...func(*User)) *User {
    user := &User{
        ID:        "test-" + uuid.New().String(),
        Email:     "test@example.com",
        Name:      "Test User",
        CreatedAt: time.Now(),
        Active:    true,
    }
    
    for _, override := range overrides {
        override(user)
    }
    
    return user
}

// Usage
user := CreateTestUser(func(u *User) {
    u.Email = "custom@example.com"
    u.Active = false
})
```

## Implementation Patterns

### Repository Pattern
```go
type UserRepository interface {
    GetByID(ctx context.Context, id string) (*User, error)
    GetByEmail(ctx context.Context, email string) (*User, error)
    Create(ctx context.Context, user *User) error
    Update(ctx context.Context, user *User) error
    Delete(ctx context.Context, id string) error
}

type userRepository struct {
    db core.DB
}

func NewUserRepository(db core.DB) UserRepository {
    return &userRepository{db: db}
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*User, error) {
    var user User
    err := r.db.WithContext(ctx).
        Model(&User{}).
        Where("ID", "=", id).
        First(&user)
    if err != nil {
        return nil, err
    }
    return &user, nil
}
```

### Service Layer Pattern
```go
type UserService struct {
    repo UserRepository
    db   core.DB
}

func NewUserService(db core.DB) *UserService {
    return &UserService{
        repo: NewUserRepository(db),
        db:   db,
    }
}

func (s *UserService) CreateUser(ctx context.Context, req CreateUserRequest) (*User, error) {
    // Validation
    if err := req.Validate(); err != nil {
        return nil, err
    }
    
    // Business logic
    user := &User{
        ID:    uuid.New().String(),
        Email: req.Email,
        Name:  req.Name,
    }
    
    // Transaction
    err := s.db.Transaction(func(tx *dynamorm.Tx) error {
        if err := tx.Model(user).Create(); err != nil {
            return err
        }
        
        // Create related records
        profile := &UserProfile{UserID: user.ID}
        return tx.Model(profile).Create()
    })
    
    if err != nil {
        return nil, err
    }
    
    return user, nil
}
```

### Multi-Tenant Pattern
```go
type TenantService struct {
    multiDB *dynamorm.MultiAccountDB
}

func (s *TenantService) GetUserForTenant(ctx context.Context, tenantID, userID string) (*User, error) {
    db, err := s.multiDB.Partner(tenantID)
    if err != nil {
        return nil, err
    }
    
    var user User
    err = db.WithContext(ctx).
        Model(&User{}).
        Where("ID", "=", userID).
        First(&user)
    
    return &user, err
}
```

### Event-Driven Pattern
```go
type EventHandler struct {
    db core.DB
}

func (h *EventHandler) HandleUserCreated(ctx context.Context, event UserCreatedEvent) error {
    return h.db.Transaction(func(tx *dynamorm.Tx) error {
        // Update statistics
        stats := &UserStats{Date: time.Now().Format("2006-01-02")}
        err := tx.Model(stats).
            UpdateBuilder().
            Increment("NewUsers").
            Execute()
        if err != nil {
            return err
        }
        
        // Create welcome email task
        task := &EmailTask{
            UserID:   event.UserID,
            Template: "welcome",
            Status:   "pending",
        }
        return tx.Model(task).Create()
    })
}
```

## Best Practices

### Model Design
1. **Use appropriate struct tags**: `pk`, `sk`, `index`, `version`, `created_at`, `updated_at`
2. **Design for access patterns**: Create indexes for all query patterns
3. **Use composite keys**: Combine related data in sort keys
4. **Optimize for DynamoDB**: Avoid hot partitions, use sparse indexes

### Query Optimization
1. **Prefer Query over Scan**: Always use indexes when possible
2. **Use projections**: Select only needed fields with `Select()`
3. **Implement pagination**: Use cursor-based pagination for large datasets
4. **Batch operations**: Use batch methods for multiple items

### Error Handling
```go
import "github.com/pay-theory/dynamorm/pkg/errors"

err := db.Model(&User{}).Where("ID", "=", "123").First(&user)
if errors.Is(err, errors.ErrItemNotFound) {
    // Handle not found case
}
```

### Performance Optimization
1. **Use Lambda optimizations**: `LambdaInit()` and `WithLambdaTimeout()`
2. **Enable connection reuse**: Initialize globally in Lambda
3. **Pre-register models**: Reduce cold start time
4. **Monitor performance**: Use CloudWatch metrics

## Common Use Cases

### User Management System
```go
type User struct {
    ID        string    `dynamorm:"pk"`
    Email     string    `dynamorm:"index:gsi-email"`
    Username  string    `dynamorm:"index:gsi-username"`
    Status    string    `dynamorm:"index:gsi-status,pk"`
    CreatedAt time.Time `dynamorm:"index:gsi-status,sk"`
    Profile   UserProfile `dynamorm:"json"`
}

// Access patterns:
// 1. Get user by ID: Query on primary key
// 2. Get user by email: Query on gsi-email
// 3. Get users by status: Query on gsi-status
// 4. Get recent users: Query on gsi-status with sort
```

### E-commerce Product Catalog
```go
type Product struct {
    SKU       string  `dynamorm:"pk"`
    Category  string  `dynamorm:"index:gsi-category,pk"`
    Price     float64 `dynamorm:"index:gsi-category,sk"`
    Brand     string  `dynamorm:"index:gsi-brand"`
    InStock   bool    `dynamorm:"index:gsi-availability,sparse"`
    CreatedAt time.Time `dynamorm:"created_at"`
}

// Access patterns:
// 1. Get product by SKU: Query on primary key
// 2. Browse by category: Query on gsi-category
// 3. Filter by brand: Query on gsi-brand
// 4. Find available products: Query on gsi-availability
```

### Multi-Tenant SaaS
```go
type TenantData struct {
    TenantID  string `dynamorm:"pk"`
    DataType  string `dynamorm:"sk"`
    Data      map[string]interface{} `dynamorm:"json"`
    CreatedAt time.Time `dynamorm:"created_at"`
}

// Access patterns:
// 1. Get all data for tenant: Query with TenantID
// 2. Get specific data type: Query with TenantID and DataType
// 3. Cross-tenant queries: Use MultiAccountDB
```

### Time-Series Data
```go
type Metric struct {
    MetricName string    `dynamorm:"pk"`
    Timestamp  time.Time `dynamorm:"sk"`
    Value      float64
    Tags       map[string]string `dynamorm:"json"`
}

// Access patterns:
// 1. Get metric by name and time range: Query with MetricName
// 2. Get latest metrics: Query with reverse sort
// 3. Aggregate data: Use parallel scan for analytics
```

## Performance Considerations

### Lambda Optimization
- **Cold Start**: ~11ms with DynamORM vs ~127ms with AWS SDK
- **Memory Usage**: 18MB vs 42MB (57% reduction)
- **Throughput**: 20,000+ ops/sec vs 12,000 ops/sec

### DynamoDB Optimization
- **Use appropriate read/write capacity**: Start with on-demand, move to provisioned for predictable workloads
- **Design for even distribution**: Avoid hot partitions
- **Use sparse indexes**: Only index items that have the attribute
- **Implement caching**: Use DAX or application-level caching

### Query Performance
- **Prefer Query over Scan**: 10-100x faster
- **Use projections**: Reduce data transfer
- **Implement pagination**: Handle large result sets efficiently
- **Batch operations**: Reduce API calls

## Troubleshooting Guide

### Common Issues

#### "Item not found" errors
```go
err := db.Model(&User{}).Where("ID", "=", "123").First(&user)
if errors.Is(err, errors.ErrItemNotFound) {
    // Handle not found case
}
```

#### Conditional check failures
```go
err := db.Model(user).
    UpdateBuilder().
    Set("Status", "active").
    ConditionExists("ID").
    Execute()
if errors.Is(err, errors.ErrConditionFailed) {
    // Handle condition failure
}
```

#### Timeout issues in Lambda
```go
func handler(ctx context.Context, event Event) error {
    // Set timeout buffer
    db := db.WithLambdaTimeoutBuffer(1 * time.Second)
    lambdaDB := db.WithLambdaTimeout(ctx)
    
    // Use lambdaDB for operations
    return lambdaDB.Model(&User{}).Create()
}
```

### Debugging Tips
1. **Enable logging**: Set log level to debug
2. **Use X-Ray tracing**: Enable in Lambda environment
3. **Monitor CloudWatch metrics**: Track performance and errors
4. **Test with DynamoDB Local**: Reproduce issues locally

### Performance Monitoring
```go
// Get Lambda memory stats
if lambdaDB, ok := db.(*dynamorm.LambdaDB); ok {
    stats := lambdaDB.GetMemoryStats()
    log.Printf("Memory usage: %.2f%% (%d MB)", 
        stats.MemoryPercent, stats.LambdaMemoryMB)
}
```

This comprehensive guide provides AI assistants with the knowledge needed to effectively help developers use DynamORM for building high-performance, serverless applications with DynamoDB. 
# DynamORM AI Assistant Guide

This comprehensive guide provides AI assistants with everything needed to understand, implement, and work with DynamORM - a Lambda-native, type-safe ORM for Amazon DynamoDB in Go.

## Table of Contents

1. [Project Overview](#project-overview)
2. [Core Architecture](#core-architecture)
3. [Important Types & Interfaces](#important-types--interfaces)
4. [Key Structs & Functions](#key-structs--functions)
5. [Testing Utilities & Mocks](#testing-utilities--mocks)
6. [Implementation Patterns](#implementation-patterns)
7. [Best Practices](#best-practices)
8. [Common Use Cases](#common-use-cases)
9. [Performance Considerations](#performance-considerations)
10. [Troubleshooting Guide](#troubleshooting-guide)

## Project Overview

DynamORM is a production-ready ORM for DynamoDB that provides:
- **Lambda-native design** with 11ms cold starts (91% faster than alternatives)
- **Type-safe operations** with compile-time validation
- **80% code reduction** compared to raw AWS SDK
- **Multi-account support** for enterprise use cases
- **Atomic operations** with optimistic locking
- **Smart index selection** and query optimization

### Module Structure
```
github.com/pay-theory/dynamorm
├── pkg/core/           # Core interfaces and types
├── pkg/query/          # Query builder implementation
├── pkg/model/          # Model registry and metadata
├── pkg/types/          # Type conversion system
├── pkg/session/        # AWS session management
├── pkg/schema/         # Table management
├── pkg/transaction/    # Transaction support
├── pkg/marshal/        # High-performance marshaling
├── pkg/mocks/          # Pre-built test mocks
├── internal/expr/      # Expression building
└── examples/           # Production-ready examples
```

## Core Architecture

### Layered Design
```
Application Layer
    ↓
Public API (DB, Query, Transaction)
    ↓
Core Services (Registry, Builder, Engine)
    ↓
Internal Components (Expression, Reflection)
    ↓
AWS SDK v2 Layer
```

### Key Design Principles
1. **Interface-driven**: All major components are interfaces
2. **Builder pattern**: Fluent, chainable query API
3. **Zero magic**: Explicit, predictable behavior
4. **Performance first**: Minimal overhead over raw SDK
5. **Lambda optimized**: Connection reuse and pre-registration

## Important Types & Interfaces

### Core Interfaces

#### DB Interface
```go
type DB interface {
    Model(interface{}) Query
    Transaction(func(*Transaction) error) error
    TransactionFunc(func(*Transaction) error) error
    CreateTable(model interface{}, opts ...schema.TableOption) error
    EnsureTable(model interface{}) error
    DeleteTable(model interface{}) error
    AutoMigrate(models ...interface{}) error
    Close() error
}
```

#### Query Interface
```go
type Query interface {
    // Builder methods
    Where(field string, op string, value interface{}) Query
    Filter(expr string, params ...Param) Query
    Index(indexName string) Query
    Limit(limit int) Query
    OrderBy(field string, order string) Query
    Select(fields ...string) Query
    Cursor(cursor string) Query
    SetCursor(cursor string) Query
    Offset(offset int) Query
    WithContext(ctx context.Context) Query
    
    // Execution methods
    First(dest interface{}) error
    All(dest interface{}) error
    AllPaginated(dest interface{}) (*PaginatedResult, error)
    Count() (int64, error)
    Scan(dest interface{}) error
    ParallelScan(segments int32) Query
    ScanAllSegments(dest interface{}, segments int32) error
    
    // CRUD operations
    Create() error
    Update(fields ...string) error
    Delete() error
    
    // Batch operations
    BatchGet(keys []interface{}, dest interface{}) error
    BatchCreate(items interface{}) error
    
    // Advanced operations
    UpdateBuilder() UpdateBuilder
}
```

#### UpdateBuilder Interface
```go
type UpdateBuilder interface {
    Set(field string, value interface{}) UpdateBuilder
    Add(field string, value interface{}) UpdateBuilder
    Remove(field string) UpdateBuilder
    Delete(field string, value interface{}) UpdateBuilder
    Increment(field string, value ...interface{}) UpdateBuilder
    Decrement(field string, value ...interface{}) UpdateBuilder
    Append(field string, value interface{}) UpdateBuilder
    Prepend(field string, value interface{}) UpdateBuilder
    SetElement(field string, index int, value interface{}) UpdateBuilder
    RemoveElement(field string, index int) UpdateBuilder
    If(condition string, params ...Param) UpdateBuilder
    Execute() error
    ExecuteWithResult(dest interface{}) error
}
```

### Lambda-Specific Types

#### LambdaDB
```go
type LambdaDB struct {
    *DB
    preRegistered map[reflect.Type]bool
    warmStart     bool
    memoryMB      int
}

// Key methods
func NewLambdaOptimized(config Config) (*LambdaDB, error)
func (ldb *LambdaDB) PreRegisterModels(models ...interface{}) error
func (ldb *LambdaDB) WithLambdaTimeout(ctx context.Context) *LambdaDB
```

#### MultiAccountDB
```go
type MultiAccountDB struct {
    accounts map[string]AccountConfig
    cache    sync.Map // partnerID -> *LambdaDB
}

func NewMultiAccount(accounts map[string]AccountConfig) (*MultiAccountDB, error)
func (mdb *MultiAccountDB) Partner(partnerID string) (*LambdaDB, error)
```

### Model Definition Types

#### Struct Tags
```go
// Primary key
`dynamorm:"pk"`

// Sort key
`dynamorm:"sk"`

// Global Secondary Index
`dynamorm:"index:gsi-name,pk"`
`dynamorm:"index:gsi-name,sk"`

// Local Secondary Index
`dynamorm:"index:lsi-name,sk"`

// Special fields
`dynamorm:"created_at"`
`dynamorm:"updated_at"`
`dynamorm:"version"`
`dynamorm:"ttl"`

// Type modifiers
`dynamorm:"set"`
`dynamorm:"json"`
`dynamorm:"omitempty"`
```

## Key Structs & Functions

### Model Definition Example
```go
type User struct {
    ID        string    `dynamorm:"pk"`
    Email     string    `dynamorm:"index:gsi-email,unique"`
    Name      string    
    Age       int       `dynamorm:"index:gsi-age-status,pk"`
    Status    string    `dynamorm:"index:gsi-age-status,sk"`
    Tags      []string  `dynamorm:"set"`
    Profile   Profile   `dynamorm:"json"`
    CreatedAt time.Time `dynamorm:"created_at"`
    UpdatedAt time.Time `dynamorm:"updated_at"`
    Version   int       `dynamorm:"version"`
    TTL       time.Time `dynamorm:"ttl"`
}

type Profile struct {
    Bio     string
    Website string
    Social  map[string]string
}
```

### Initialization Functions

#### Standard Initialization
```go
func New(config Config) (*DB, error)

type Config struct {
    Region          string
    Endpoint        string // For DynamoDB Local
    MaxRetries      int
    DefaultRCU      int64
    DefaultWCU      int64
    EnableXRay      bool
    CustomConfig    *aws.Config
}
```

#### Lambda Initialization
```go
// Global instance for warm starts
var db *dynamorm.LambdaDB

func init() {
    var err error
    db, err = dynamorm.NewLambdaOptimized(dynamorm.Config{
        Region: "us-east-1",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Pre-register models for performance
    db.PreRegisterModels(&User{}, &Order{}, &Product{})
}

func handler(ctx context.Context, event Event) (Response, error) {
    // Use timeout-aware DB
    timeoutDB := db.WithLambdaTimeout(ctx)
    
    var user User
    err := timeoutDB.Model(&User{}).
        Where("ID", "=", event.UserID).
        First(&user)
    
    return Response{User: user}, err
}
```

### Query Building Functions

#### Basic Queries
```go
// Get single item
var user User
err := db.Model(&User{}).Where("ID", "=", "123").First(&user)

// Get multiple items
var users []User
err := db.Model(&User{}).
    Where("Status", "=", "active").
    OrderBy("CreatedAt", "desc").
    Limit(10).
    All(&users)

// Count items
count, err := db.Model(&User{}).
    Where("Status", "=", "active").
    Count()
```

#### Advanced Queries
```go
// Complex conditions with filters
var users []User
err := db.Model(&User{}).
    Index("gsi-age-status").
    Where("Age", ">=", 18).
    Where("Status", "=", "active").
    Filter("contains(Tags, :tag)", dynamorm.Param("tag", "premium")).
    Filter("attribute_exists(Email)").
    All(&users)

// Pagination
result, err := db.Model(&User{}).
    Where("Status", "=", "active").
    Limit(20).
    AllPaginated(&users)

if result.HasMore {
    // Get next page
    nextResult, err := db.Model(&User{}).
        Where("Status", "=", "active").
        Cursor(result.NextCursor).
        Limit(20).
        AllPaginated(&users)
}
```

### Transaction Functions

#### Simple Transactions
```go
err := db.Transaction(func(tx *transaction.Transaction) error {
    // Create user
    user := &User{ID: "123", Name: "John"}
    if err := tx.Create(user); err != nil {
        return err
    }
    
    // Update account
    account := &Account{UserID: "123", Balance: 100}
    if err := tx.Update(account); err != nil {
        return err
    }
    
    return nil
})
```

#### Atomic Operations
```go
// Atomic counter increment
err := db.Model(&Counter{}).
    Where("ID", "=", "page-views").
    UpdateBuilder().
    Increment("Count", 1).
    Execute()

// Conditional update with optimistic locking
err := db.Model(&User{}).
    Where("ID", "=", "123").
    UpdateBuilder().
    Set("Status", "premium").
    If("Version = :v", dynamorm.Param("v", user.Version)).
    Execute()
```

### Schema Management Functions

#### Table Operations
```go
// Create table with options
err := db.CreateTable(&User{},
    schema.WithBillingMode(types.BillingModePayPerRequest),
    schema.WithStreamSpecification(types.StreamSpecification{
        StreamEnabled:  true,
        StreamViewType: types.StreamViewTypeNewAndOldImages,
    }),
)

// Ensure table exists (idempotent)
err := db.EnsureTable(&User{})

// Auto-migrate multiple models
err := db.AutoMigrate(&User{}, &Order{}, &Product{})
```

## Testing Utilities & Mocks

### Pre-built Mocks Package

DynamORM provides a comprehensive mocks package to eliminate testing friction:

```go
import "github.com/pay-theory/dynamorm/pkg/mocks"

func TestUserService(t *testing.T) {
    // Use pre-built mocks
    mockDB := new(mocks.MockDB)
    mockQuery := new(mocks.MockQuery)
    mockUpdateBuilder := new(mocks.MockUpdateBuilder)
    
    // Setup expectations
    mockDB.On("Model", &User{}).Return(mockQuery)
    mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
    mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
        user := args.Get(0).(*User)
        user.ID = "123"
        user.Name = "Test User"
    }).Return(nil)
    
    // Test your service
    service := NewUserService(mockDB)
    user, err := service.GetUser("123")
    
    assert.NoError(t, err)
    assert.Equal(t, "123", user.ID)
    mockDB.AssertExpectations(t)
}
```

### Integration Testing

#### DynamoDB Local Setup
```go
func setupTestDB(t *testing.T) *dynamorm.DB {
    db, err := dynamorm.New(dynamorm.Config{
        Region:   "us-east-1",
        Endpoint: "http://localhost:8000", // DynamoDB Local
    })
    require.NoError(t, err)
    
    // Create test tables
    require.NoError(t, db.CreateTable(&User{}))
    require.NoError(t, db.CreateTable(&Order{}))
    
    return db
}

func TestUserCRUD(t *testing.T) {
    db := setupTestDB(t)
    
    // Test create
    user := &User{ID: "test-123", Name: "Test User"}
    err := db.Model(user).Create()
    require.NoError(t, err)
    
    // Test read
    var found User
    err = db.Model(&User{}).Where("ID", "=", "test-123").First(&found)
    require.NoError(t, err)
    assert.Equal(t, "Test User", found.Name)
    
    // Test update
    err = db.Model(&User{}).
        Where("ID", "=", "test-123").
        Update("Name", "Updated Name")
    require.NoError(t, err)
    
    // Test delete
    err = db.Model(&User{}).Where("ID", "=", "test-123").Delete()
    require.NoError(t, err)
}
```

### Test Utilities

#### Test Data Factories
```go
func CreateTestUser(id string) *User {
    return &User{
        ID:        id,
        Email:     fmt.Sprintf("%s@test.com", id),
        Name:      fmt.Sprintf("Test User %s", id),
        Status:    "active",
        CreatedAt: time.Now(),
    }
}

func CreateTestUsers(count int) []*User {
    users := make([]*User, count)
    for i := 0; i < count; i++ {
        users[i] = CreateTestUser(fmt.Sprintf("user-%d", i))
    }
    return users
}
```

#### Assertion Helpers
```go
func AssertUserEqual(t *testing.T, expected, actual *User) {
    assert.Equal(t, expected.ID, actual.ID)
    assert.Equal(t, expected.Email, actual.Email)
    assert.Equal(t, expected.Name, actual.Name)
    assert.Equal(t, expected.Status, actual.Status)
}

func AssertQueryResult(t *testing.T, query dynamorm.Query, expectedCount int) {
    count, err := query.Count()
    require.NoError(t, err)
    assert.Equal(t, int64(expectedCount), count)
}
```

## Implementation Patterns

### Repository Pattern
```go
type UserRepository struct {
    db dynamorm.DB
}

func NewUserRepository(db dynamorm.DB) *UserRepository {
    return &UserRepository{db: db}
}

func (r *UserRepository) GetByID(id string) (*User, error) {
    var user User
    err := r.db.Model(&User{}).Where("ID", "=", id).First(&user)
    if err != nil {
        return nil, fmt.Errorf("failed to get user %s: %w", id, err)
    }
    return &user, nil
}

func (r *UserRepository) GetByEmail(email string) (*User, error) {
    var user User
    err := r.db.Model(&User{}).
        Index("gsi-email").
        Where("Email", "=", email).
        First(&user)
    return &user, err
}

func (r *UserRepository) Create(user *User) error {
    user.CreatedAt = time.Now()
    user.UpdatedAt = time.Now()
    return r.db.Model(user).Create()
}

func (r *UserRepository) Update(user *User) error {
    user.UpdatedAt = time.Now()
    return r.db.Model(user).Update()
}
```

### Service Layer Pattern
```go
type UserService struct {
    repo *UserRepository
}

func NewUserService(db dynamorm.DB) *UserService {
    return &UserService{
        repo: NewUserRepository(db),
    }
}

func (s *UserService) CreateUser(req CreateUserRequest) (*User, error) {
    // Validation
    if req.Email == "" {
        return nil, errors.New("email is required")
    }
    
    // Check if user exists
    existing, err := s.repo.GetByEmail(req.Email)
    if err == nil && existing != nil {
        return nil, errors.New("user already exists")
    }
    
    // Create user
    user := &User{
        ID:     uuid.New().String(),
        Email:  req.Email,
        Name:   req.Name,
        Status: "active",
    }
    
    if err := s.repo.Create(user); err != nil {
        return nil, fmt.Errorf("failed to create user: %w", err)
    }
    
    return user, nil
}
```

### Multi-Tenant Pattern
```go
type TenantModel struct {
    TenantID string `dynamorm:"pk,composite:tenant_id,id"`
    ID       string `dynamorm:"extract:id"`
    // other fields
}

type TenantService struct {
    db       dynamorm.DB
    tenantID string
}

func NewTenantService(db dynamorm.DB, tenantID string) *TenantService {
    return &TenantService{db: db, tenantID: tenantID}
}

func (s *TenantService) GetResource(id string) (*Resource, error) {
    var resource Resource
    err := s.db.Model(&Resource{}).
        Where("TenantID", "=", s.tenantID).
        Where("ID", "=", id).
        First(&resource)
    return &resource, err
}
```

### Event-Driven Pattern
```go
type EventHandler struct {
    db dynamorm.DB
}

func (h *EventHandler) HandleUserCreated(ctx context.Context, event UserCreatedEvent) error {
    return h.db.Transaction(func(tx *transaction.Transaction) error {
        // Create welcome message
        message := &Message{
            ID:     uuid.New().String(),
            UserID: event.UserID,
            Type:   "welcome",
            Content: "Welcome to our platform!",
        }
        if err := tx.Create(message); err != nil {
            return err
        }
        
        // Update user stats
        return tx.Model(&UserStats{}).
            Where("ID", "=", "global").
            UpdateBuilder().
            Increment("TotalUsers", 1).
            Execute()
    })
}
```

## Best Practices

### Model Design

#### 1. Use Composite Keys for Multi-Tenancy
```go
type Order struct {
    ID         string `dynamorm:"pk,composite:customer_id,order_id"`
    CustomerID string `dynamorm:"extract:customer_id"`
    OrderID    string `dynamorm:"extract:order_id"`
    // other fields
}
```

#### 2. Design Indexes for Access Patterns
```go
type Product struct {
    SKU        string  `dynamorm:"pk"`
    CategoryID string  `dynamorm:"index:gsi-category-price,pk"`
    Price      float64 `dynamorm:"index:gsi-category-price,sk"`
    Name       string  `dynamorm:"index:gsi-name"`
    Status     string  `dynamorm:"index:gsi-status-created,pk"`
    CreatedAt  time.Time `dynamorm:"index:gsi-status-created,sk"`
}
```

#### 3. Use TTL for Automatic Cleanup
```go
type Session struct {
    ID        string    `dynamorm:"pk"`
    UserID    string    `dynamorm:"index:gsi-user"`
    Data      string
    ExpiresAt time.Time `dynamorm:"ttl"`
}
```

### Query Optimization

#### 1. Use Specific Indexes
```go
// Good: Uses specific index
users, err := db.Model(&User{}).
    Index("gsi-status-created").
    Where("Status", "=", "active").
    Where("CreatedAt", ">", yesterday).
    All(&users)

// Avoid: Forces table scan
users, err := db.Model(&User{}).
    Filter("Status = :status", dynamorm.Param("status", "active")).
    Scan(&users)
```

#### 2. Limit Result Sets
```go
// Good: Limits results
users, err := db.Model(&User{}).
    Where("Status", "=", "active").
    Limit(100).
    All(&users)

// Avoid: Unlimited results
users, err := db.Model(&User{}).
    Where("Status", "=", "active").
    All(&users)
```

#### 3. Use Projection for Large Items
```go
// Good: Only fetch needed fields
var users []User
err := db.Model(&User{}).
    Select("ID", "Name", "Email").
    Where("Status", "=", "active").
    All(&users)
```

### Error Handling

#### 1. Wrap Errors with Context
```go
func (s *UserService) GetUser(id string) (*User, error) {
    user, err := s.repo.GetByID(id)
    if err != nil {
        return nil, fmt.Errorf("failed to get user %s: %w", id, err)
    }
    return user, nil
}
```

#### 2. Handle Specific Error Types
```go
import "github.com/pay-theory/dynamorm/pkg/errors"

func (s *UserService) GetUser(id string) (*User, error) {
    user, err := s.repo.GetByID(id)
    if err != nil {
        if errors.Is(err, errors.ErrItemNotFound) {
            return nil, ErrUserNotFound
        }
        return nil, fmt.Errorf("database error: %w", err)
    }
    return user, nil
}
```

### Performance Optimization

#### 1. Pre-register Models in Lambda
```go
func init() {
    db.PreRegisterModels(&User{}, &Order{}, &Product{})
}
```

#### 2. Use Batch Operations
```go
// Good: Batch create
users := []*User{user1, user2, user3}
err := db.Model(&User{}).BatchCreate(users)

// Avoid: Individual creates
for _, user := range users {
    err := db.Model(user).Create()
}
```

#### 3. Use Transactions for Consistency
```go
// Good: Atomic operation
err := db.Transaction(func(tx *transaction.Transaction) error {
    if err := tx.Create(order); err != nil {
        return err
    }
    return tx.Model(&Inventory{}).
        Where("ProductID", "=", order.ProductID).
        UpdateBuilder().
        Decrement("Stock", order.Quantity).
        Execute()
})
```

## Common Use Cases

### 1. User Management System
```go
type User struct {
    ID        string    `dynamorm:"pk"`
    Email     string    `dynamorm:"index:gsi-email,unique"`
    Username  string    `dynamorm:"index:gsi-username,unique"`
    Status    string    `dynamorm:"index:gsi-status-created,pk"`
    CreatedAt time.Time `dynamorm:"index:gsi-status-created,sk"`
    Profile   UserProfile `dynamorm:"json"`
    Version   int       `dynamorm:"version"`
}

// Get user by email
var user User
err := db.Model(&User{}).
    Index("gsi-email").
    Where("Email", "=", email).
    First(&user)

// List active users
var users []User
err := db.Model(&User{}).
    Index("gsi-status-created").
    Where("Status", "=", "active").
    OrderBy("CreatedAt", "desc").
    Limit(50).
    All(&users)
```

### 2. E-commerce Order System
```go
type Order struct {
    ID         string      `dynamorm:"pk"`
    CustomerID string      `dynamorm:"index:gsi-customer-date,pk"`
    OrderDate  time.Time   `dynamorm:"index:gsi-customer-date,sk"`
    Status     string      `dynamorm:"index:gsi-status-date,pk"`
    Items      []OrderItem `dynamorm:"json"`
    Total      float64
    Version    int         `dynamorm:"version"`
}

// Get customer orders
var orders []Order
err := db.Model(&Order{}).
    Index("gsi-customer-date").
    Where("CustomerID", "=", customerID).
    OrderBy("OrderDate", "desc").
    All(&orders)

// Process order with inventory update
err := db.Transaction(func(tx *transaction.Transaction) error {
    // Create order
    if err := tx.Create(order); err != nil {
        return err
    }
    
    // Update inventory
    for _, item := range order.Items {
        err := tx.Model(&Inventory{}).
            Where("ProductID", "=", item.ProductID).
            UpdateBuilder().
            Decrement("Stock", item.Quantity).
            If("Stock >= :qty", dynamorm.Param("qty", item.Quantity)).
            Execute()
        if err != nil {
            return err
        }
    }
    
    return nil
})
```

### 3. Multi-Tenant SaaS
```go
type Project struct {
    ID       string `dynamorm:"pk,composite:org_id,project_id"`
    OrgID    string `dynamorm:"extract:org_id"`
    Name     string `dynamorm:"index:gsi-org-name,pk,composite:org_id,name"`
    Status   string `dynamorm:"index:gsi-org-status,pk,composite:org_id,status"`
    CreatedAt time.Time `dynamorm:"index:gsi-org-status,sk"`
}

// Get organization projects
var projects []Project
err := db.Model(&Project{}).
    Index("gsi-org-status").
    Where("OrgID", "=", orgID).
    Where("Status", "=", "active").
    All(&projects)
```

### 4. Time-Series Data (IoT/Analytics)
```go
type Metric struct {
    DeviceID  string    `dynamorm:"pk"`
    Timestamp time.Time `dynamorm:"sk"`
    Value     float64
    Type      string    `dynamorm:"index:gsi-type-time,pk"`
    TTL       time.Time `dynamorm:"ttl"`
}

// Get device metrics for time range
var metrics []Metric
err := db.Model(&Metric{}).
    Where("DeviceID", "=", deviceID).
    Where("Timestamp", "between", startTime, endTime).
    All(&metrics)

// Get metrics by type
var typeMetrics []Metric
err := db.Model(&Metric{}).
    Index("gsi-type-time").
    Where("Type", "=", "temperature").
    Where("Timestamp", ">", time.Now().Add(-24*time.Hour)).
    All(&typeMetrics)
```

## Performance Considerations

### Lambda Optimization
1. **Pre-register models** to avoid reflection overhead
2. **Use global DB instance** for connection reuse
3. **Set Lambda timeout** to prevent hanging operations
4. **Use appropriate memory allocation** (1024MB+ recommended)

### DynamoDB Optimization
1. **Design for access patterns** not normalization
2. **Use composite keys** for hierarchical data
3. **Leverage GSIs** for different query patterns
4. **Use TTL** for automatic data cleanup
5. **Batch operations** when possible
6. **Monitor hot partitions** and distribute load

### Query Performance
1. **Use specific indexes** instead of scans
2. **Limit result sets** with pagination
3. **Use projection** to reduce data transfer
4. **Cache frequently accessed data**
5. **Use eventually consistent reads** when possible

## Troubleshooting Guide

### Common Issues

#### 1. Item Not Found Errors
```go
// Check for specific error
if errors.Is(err, errors.ErrItemNotFound) {
    // Handle not found case
    return nil, ErrUserNotFound
}
```

#### 2. Conditional Check Failed
```go
// Handle optimistic locking failures
if errors.Is(err, errors.ErrConditionFailed) {
    // Retry with fresh data
    return s.retryUpdate(user)
}
```

#### 3. Index Not Found
```go
// Verify index exists in model definition
type User struct {
    Email string `dynamorm:"index:gsi-email"` // Make sure this exists
}
```

#### 4. Lambda Timeout Issues
```go
// Use timeout-aware DB
func handler(ctx context.Context, event Event) error {
    timeoutDB := db.WithLambdaTimeout(ctx)
    // Use timeoutDB for all operations
}
```

### Debugging Tips

#### 1. Enable Debug Logging
```go
db, err := dynamorm.New(dynamorm.Config{
    Region:     "us-east-1",
    DebugLevel: dynamorm.DebugAll,
})
```

#### 2. Check Query Compilation
```go
// Use Count() to test query without fetching data
count, err := db.Model(&User{}).
    Where("Status", "=", "active").
    Count()
```

#### 3. Verify Table Schema
```go
// Check if table exists and has correct schema
desc, err := db.DescribeTable(&User{})
if err != nil {
    log.Printf("Table issue: %v", err)
}
```

### Performance Monitoring

#### 1. Track Query Performance
```go
start := time.Now()
err := db.Model(&User{}).Where("ID", "=", id).First(&user)
duration := time.Since(start)
log.Printf("Query took %v", duration)
```

#### 2. Monitor Lambda Metrics
- Cold start frequency
- Memory usage
- Execution duration
- Error rates

#### 3. DynamoDB Metrics
- Read/write capacity usage
- Throttling events
- Hot partition warnings
- Index usage patterns

This comprehensive guide provides AI assistants with all the essential knowledge needed to work effectively with DynamORM, from basic usage to advanced patterns and troubleshooting. 
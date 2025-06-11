# DynamORM Interface Reference

This document provides a complete reference for DynamORM's interfaces introduced in v0.2.0.

## Core Interfaces

### `core.DB` Interface

The basic interface for CRUD operations. Use this when you only need data access functionality.

```go
type DB interface {
    // Model returns a new query builder for the given model
    Model(model any) Query
    
    // Transaction executes a function within a database transaction
    Transaction(fn func(tx *Tx) error) error
    
    // Migrate runs all pending migrations
    Migrate() error
    
    // AutoMigrate creates or updates tables based on the given models
    AutoMigrate(models ...any) error
    
    // Close closes the database connection
    Close() error
    
    // WithContext returns a new DB instance with the given context
    WithContext(ctx context.Context) DB
}
```

### `core.ExtendedDB` Interface

The full interface including schema management. This extends `core.DB` with additional capabilities.

```go
type ExtendedDB interface {
    DB  // Embeds all basic operations
    
    // AutoMigrateWithOptions performs enhanced auto-migration with data copy support
    AutoMigrateWithOptions(model any, opts ...any) error
    
    // CreateTable creates a DynamoDB table for the given model
    CreateTable(model any, opts ...any) error
    
    // EnsureTable checks if a table exists and creates it if not
    EnsureTable(model any) error
    
    // DeleteTable deletes the DynamoDB table for the given model
    DeleteTable(model any) error
    
    // DescribeTable returns the table description for the given model
    DescribeTable(model any) (any, error)
    
    // WithLambdaTimeout sets a deadline based on Lambda context
    WithLambdaTimeout(ctx context.Context) DB
    
    // WithLambdaTimeoutBuffer sets a custom timeout buffer for Lambda
    WithLambdaTimeoutBuffer(buffer time.Duration) DB
    
    // TransactionFunc executes a function within a transaction (full support)
    TransactionFunc(fn func(tx any) error) error
}
```

### `core.Query` Interface

The query builder interface returned by `Model()`.

```go
type Query interface {
    // Key Conditions
    Where(field string, op string, value any) Query
    
    // Filtering
    Filter(field string, op string, value any) Query
    
    // Index Selection
    Index(indexName string) Query
    
    // Ordering
    OrderBy(field string, direction string) Query
    
    // Pagination
    Limit(limit int) Query
    Offset(offset int) Query
    StartKey(key map[string]any) Query
    
    // Projection
    Select(fields ...string) Query
    
    // Execution Methods
    First(dest any) error
    All(dest any) error
    Count() (int64, error)
    Scan(dest any) error
    
    // Write Operations
    Create() error
    Update(fields ...string) error
    Delete() error
    
    // Batch Operations
    BatchCreate(items []any) error
    BatchGet(keys []any, dest any) error
    
    // Context
    WithContext(ctx context.Context) Query
}
```

### `core.Tx` Interface

The transaction interface for atomic operations.

```go
type Tx interface {
    // Create adds an item to the transaction
    Create(item any) error
    
    // Update modifies an item in the transaction
    Update(item any, fields ...string) error
    
    // Delete removes an item in the transaction
    Delete(item any) error
    
    // ConditionCheck adds a condition check to the transaction
    ConditionCheck(item any, condition string) error
}
```

## Usage Examples

### Basic Operations with `core.DB`

```go
func NewUserService(db core.DB) *UserService {
    return &UserService{db: db}
}

func (s *UserService) GetUser(id string) (*User, error) {
    var user User
    err := s.db.Model(&User{}).
        Where("ID", "=", id).
        First(&user)
    return &user, err
}

func (s *UserService) CreateUser(user *User) error {
    return s.db.Model(user).Create()
}
```

### Schema Management with `core.ExtendedDB`

```go
func SetupDatabase(db core.ExtendedDB) error {
    // Ensure tables exist
    if err := db.EnsureTable(&User{}); err != nil {
        return err
    }
    
    // Migrate with options
    return db.AutoMigrateWithOptions(&User{},
        WithBackup(true),
        WithDataCopy(true),
    )
}
```

### Creating Mocks

```go
// Mock for core.DB
type MockDB struct {
    mock.Mock
}

func (m *MockDB) Model(model any) core.Query {
    args := m.Called(model)
    return args.Get(0).(core.Query)
}

func (m *MockDB) Transaction(fn func(tx *core.Tx) error) error {
    args := m.Called(fn)
    return args.Error(0)
}

// ... implement other methods

// Mock for core.Query
type MockQuery struct {
    mock.Mock
}

func (m *MockQuery) Where(field, op string, value any) core.Query {
    args := m.Called(field, op, value)
    return args.Get(0).(core.Query)
}

func (m *MockQuery) First(dest any) error {
    args := m.Called(dest)
    return args.Error(0)
}

// ... implement other methods
```

## Interface Selection Guide

### When to use `core.DB`:
- Services that only perform CRUD operations
- Repository pattern implementations
- Business logic layers
- Most application code

### When to use `core.ExtendedDB`:
- Database initialization code
- Migration scripts
- Admin tools
- Development utilities
- Test setup/teardown

### When to use concrete `*dynamorm.DB`:
- Only when you need access to unexported methods
- During migration from older versions
- Special cases requiring internal access

## Type Assertions

When you need to access concrete type methods:

```go
func HandleSpecialCase(db core.ExtendedDB) error {
    // Type assert when needed
    concreteDB, ok := db.(*dynamorm.DB)
    if !ok {
        return errors.New("requires concrete DB type")
    }
    
    // Use concrete type methods
    return concreteDB.SomeInternalMethod()
}
```

## Best Practices

1. **Always use interfaces in function parameters**:
   ```go
   // Good
   func ProcessData(db core.DB) error
   
   // Avoid
   func ProcessData(db *dynamorm.DB) error
   ```

2. **Return interfaces from constructors**:
   ```go
   // Good
   func NewRepository() core.DB
   
   // Avoid
   func NewRepository() *dynamorm.DB
   ```

3. **Use the minimal interface needed**:
   ```go
   // If you only need Model() and nothing else
   type ModelProvider interface {
       Model(any) core.Query
   }
   ```

4. **Create custom interfaces for specific needs**:
   ```go
   type UserRepository interface {
       GetUser(id string) (*User, error)
       CreateUser(user *User) error
       UpdateUser(user *User) error
       DeleteUser(id string) error
   }
   ```

## Migration from Concrete Types

### Before (v0.1.x):
```go
type Service struct {
    db *dynamorm.DB
}

func New(db *dynamorm.DB) *Service {
    return &Service{db: db}
}
```

### After (v0.2.0+):
```go
type Service struct {
    db core.DB  // or core.ExtendedDB
}

func New(db core.DB) *Service {
    return &Service{db: db}
}
```

This simple change enables:
- Easy unit testing with mocks
- Better separation of concerns
- Dependency injection support
- Future-proof architecture 
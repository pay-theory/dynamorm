# Team 1: Core Foundation & Model System

## Your Mission

You are part of Team 1 working on DynamORM, a powerful DynamoDB ORM for Go. Your team is responsible for building the core foundation, model registry, and type system that will serve as the backbone of the entire ORM.

## Context

DynamORM aims to eliminate the complexity and verbosity of working with DynamoDB while maintaining its performance and scalability benefits. The complete design is documented in:
- `DESIGN.md` - Overall design and API
- `ARCHITECTURE.md` - Technical architecture 
- `ROADMAP.md` - Implementation phases
- `STRUCT_TAGS.md` - Struct tag specification

## Your Responsibilities

### Phase 1: Core Foundation (Weeks 1-2)

1. **Project Setup**
   - Initialize Go module as `github.com/pay-theory/dynamorm`
   - Set up folder structure per `ROADMAP.md`
   - Configure GitHub Actions CI/CD
   - Set up golangci-lint, go test, and benchmarks

2. **Core Interfaces** (`pkg/core/`)
   ```go
   type DB interface {
       Model(interface{}) *Query
       Transaction(func(*Tx) error) error
       Migrate() error
       Close() error
   }
   
   type Query interface {
       Where(field string, op string, value interface{}) Query
       First(dest interface{}) error
       All(dest interface{}) error
       Create() error
       Update(fields ...string) error
       Delete() error
   }
   ```

3. **Model Registry** (`pkg/model/`)
   - Implement thread-safe model registration
   - Parse struct tags according to `STRUCT_TAGS.md`
   - Extract and validate:
     - Primary keys (pk)
     - Sort keys (sk)
     - Indexes (GSI/LSI)
     - Special fields (version, ttl, timestamps)
   - Generate table metadata
   - Cache reflection results

4. **Basic Type System** (`pkg/types/`)
   - Implement converters for:
     - Primitives (string, int, float, bool)
     - time.Time support
     - []byte for binary
     - Basic slices and maps
   - Marshal/unmarshal between Go types and DynamoDB AttributeValues
   - Handle null values and zero values

### Phase 2: Basic Operations (Week 3)

1. **Session Management** (`pkg/session/`)
   - AWS SDK v2 client initialization
   - Configuration management
   - Connection pooling
   - Region and endpoint configuration

2. **Basic CRUD Operations**
   - Implement GetItem for primary key queries
   - Implement PutItem for creates
   - Implement UpdateItem for updates
   - Implement DeleteItem for deletes
   - Handle errors and return appropriate typed errors

3. **Error System** (`pkg/errors/`)
   ```go
   var (
       ErrItemNotFound = errors.New("item not found")
       ErrInvalidModel = errors.New("invalid model")
       ErrMissingPrimaryKey = errors.New("missing primary key")
   )
   ```

## Technical Requirements

1. **Code Quality**
   - 90%+ test coverage
   - All exported functions must have godoc comments
   - Follow Go best practices and idioms
   - Benchmark critical paths

2. **Performance**
   - Model registration < 1ms
   - Zero allocations in hot paths where possible
   - Efficient reflection usage (cache results)

3. **Testing**
   - Unit tests for all components
   - Integration tests with DynamoDB Local
   - Table-driven tests for type converters
   - Mock implementations for interfaces

## Deliverables

By the end of Week 3, you should have:

1. ✅ Complete project structure and CI/CD
2. ✅ Core interfaces defined and documented
3. ✅ Model registry with full struct tag parsing
4. ✅ Basic type system with primitive support
5. ✅ Working CRUD operations for simple models
6. ✅ Comprehensive test suite
7. ✅ Benchmarks for critical operations

## Example Test Case

Your implementation should make this test pass:

```go
func TestBasicCRUD(t *testing.T) {
    // Define model
    type User struct {
        ID        string    `dynamorm:"pk"`
        Email     string    `dynamorm:"index:gsi-email"`
        Name      string
        Age       int
        CreatedAt time.Time `dynamorm:"created_at"`
        UpdatedAt time.Time `dynamorm:"updated_at"`
    }
    
    // Initialize DB
    db, err := dynamorm.New(dynamorm.Config{
        Region: "us-east-1",
        Endpoint: "http://localhost:8000", // DynamoDB Local
    })
    require.NoError(t, err)
    
    // Create table
    err = db.AutoMigrate(&User{})
    require.NoError(t, err)
    
    // Create user
    user := &User{
        ID:    "user-123",
        Email: "test@example.com",
        Name:  "Test User",
        Age:   25,
    }
    err = db.Model(user).Create()
    require.NoError(t, err)
    
    // Read user
    var found User
    err = db.Model(&User{}).Where("ID", "=", "user-123").First(&found)
    require.NoError(t, err)
    require.Equal(t, user.Email, found.Email)
    
    // Update user
    err = db.Model(&User{}).
        Where("ID", "=", "user-123").
        Update("Age", 26)
    require.NoError(t, err)
    
    // Delete user
    err = db.Model(&User{}).Where("ID", "=", "user-123").Delete()
    require.NoError(t, err)
}
```

## Communication

- Coordinate with Team 2 on interface definitions
- Document all public APIs thoroughly
- Create issues for any design clarifications needed
- Update progress in the project board daily

## Getting Started

1. Review all design documents thoroughly
2. Set up your development environment with Go 1.21+
3. Install DynamoDB Local for testing
4. Fork the repository and create a feature branch
5. Start with project setup and core interfaces

Remember: You're building the foundation that everything else depends on. Make it rock solid! 
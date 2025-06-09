# DynamORM Table Management Proposal

**Date**: December 2024  
**Author**: Development Team  
**Status**: Proposal

## Executive Summary

This proposal outlines the recommended approach for table management operations in DynamORM, a Lambda-first ORM library for DynamoDB. We recommend removing complex migration systems while retaining simple table operation wrappers that align with ORM principles.

## Background

During development, we initially implemented a full database migration system similar to traditional SQL ORMs. However, after review, we identified that this approach conflicts with:
- Lambda architecture patterns
- Infrastructure as Code best practices
- Our goal of being a lightweight library

## Current Situation

### What We Removed
- **Migration tracking table** storing version history
- **CLI tool** for running migrations
- **Version management** with up/down rollback capabilities
- **Migration file loaders** and state management

### What Remains
- `CreateTable()` - Direct wrapper for DynamoDB CreateTable
- `DeleteTable()` - Direct wrapper for DynamoDB DeleteTable  
- `EnsureTable()` - Checks existence and creates if missing
- `AutoMigrate()` - Creates tables from model definitions
- `DescribeTable()` - Gets table information

## Proposal

### Core Principle
**An ORM should provide simplified access to underlying database operations, not implement infrastructure management systems.**

### Recommended Approach

#### 1. Keep Simple Table Operations
These functions are appropriate for an ORM because they:
- Are thin wrappers over DynamoDB APIs
- Require no persistent state
- Add convenience without complexity
- Support development workflows

```go
// Example: Simple table creation from model
type User struct {
    ID    string `dynamodbav:"id,pk"`
    Email string `dynamodbav:"email"`
}

// In deployment script or local development
db.CreateTable(&User{})
```

#### 2. Enhance for Common Use Cases
Add lightweight helpers for common operations:

```go
// AutoMigrate with simple data copy
db.AutoMigrate(&User{}, 
    WithBackupTable("users_backup"),
    WithDataCopy(true),
)

// EnsureTable for idempotent deployments
db.EnsureTable(&User{})
```

#### 3. Clear Separation of Concerns

| Responsibility | Tool | Example |
|----------------|------|---------|
| Infrastructure Definition | CDK/Terraform | Table creation, GSI, capacity |
| Schema Changes | IaC + Deployment Pipeline | Add indexes, change capacity |
| Runtime Operations | DynamORM | CRUD, queries, transactions |
| Development Helpers | DynamORM | CreateTable, EnsureTable |

## Benefits

### 1. Lambda Optimization
- No migration checks on cold start
- No version table queries
- Minimal initialization overhead

### 2. Architectural Alignment
- Follows AWS best practices
- Compatible with IaC workflows
- Clear separation of concerns

### 3. Developer Experience
- Simple table operations for development
- No complex migration syntax to learn
- Works seamlessly with DynamoDB Local

### 4. Maintenance Simplicity
- No migration state to manage
- No version conflicts
- No rollback complexity

## Implementation Guidelines

### What Table Operations Should Do
✅ Wrap DynamoDB API calls directly  
✅ Use model struct tags for configuration  
✅ Provide convenience for development  
✅ Support testing scenarios  
✅ Remain stateless  

### What Table Operations Should NOT Do
❌ Track version history  
❌ Manage migration state  
❌ Implement rollback logic  
❌ Require external files  
❌ Add startup overhead  

## Example Usage Patterns

### Development Environment
```go
// Local development with DynamoDB Local
func setupDevEnvironment() {
    db, _ := dynamorm.New(dynamorm.Config{
        Endpoint: "http://localhost:8000",
    })
    
    // Create all tables for local testing
    db.EnsureTable(&User{})
    db.EnsureTable(&Post{})
    db.EnsureTable(&Comment{})
}
```

### Test Environment
```go
func TestUserOperations(t *testing.T) {
    // Create isolated table for test
    testTable := &User{TableName: "test_users_" + uuid.New()}
    db.CreateTable(testTable)
    defer db.DeleteTable(testTable)
    
    // Run tests...
}
```

### Deployment Scripts
```go
// One-time setup Lambda
func SetupHandler(ctx context.Context) error {
    db, _ := dynamorm.New(dynamorm.Config{})
    
    // Ensure tables exist (idempotent)
    if err := db.EnsureTable(&User{}); err != nil {
        return err
    }
    
    return nil
}
```

### Data Migration Lambda
```go
// Separate Lambda for data migrations
func MigrateDataHandler(ctx context.Context, event MigrationEvent) error {
    db, _ := dynamorm.New(dynamorm.Config{})
    
    // Simple table copy with transformation
    return db.AutoMigrate(&UserV1{}, 
        WithTargetModel(&UserV2{}),
        WithTransform(func(old *UserV1) *UserV2 {
            return &UserV2{
                ID:       old.ID,
                Email:    old.Email,
                UpdatedAt: time.Now(),
            }
        }),
    )
}
```

## Comparison with Alternatives

### Option 1: Full Migration System (Rejected)
- ❌ Complex state management
- ❌ Cold start overhead
- ❌ Conflicts with IaC patterns
- ❌ Unnecessary for NoSQL

### Option 2: No Table Operations (Too Limited)
- ❌ Poor developer experience
- ❌ Difficult testing
- ❌ No development helpers
- ❌ Forces boilerplate

### Option 3: Simple Table Operations (Recommended)
- ✅ Lightweight wrappers
- ✅ Developer friendly
- ✅ No runtime overhead
- ✅ IaC compatible

## Conclusion

By keeping simple table operations while removing complex migration systems, DynamORM remains:
- **Lightweight**: Minimal code, no persistent state
- **Lambda-optimized**: No startup overhead
- **Developer-friendly**: Convenient helpers for common tasks
- **Architecturally sound**: Clear separation from infrastructure

This approach provides the convenience developers expect from an ORM while respecting the architectural patterns of serverless applications.

## Recommendation

1. **Retain** all current simple table operation methods
2. **Document** clear usage patterns for different scenarios
3. **Enhance** AutoMigrate for simple data copy operations
4. **Educate** users on proper IaC integration patterns

This balanced approach serves both development convenience and production best practices. 
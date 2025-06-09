# Migration Guide

This guide helps you migrate existing DynamoDB applications to DynamORM.

## Table of Contents
- [Migrating from AWS SDK](#migrating-from-aws-sdk)
- [Migrating from Other ORMs](#migrating-from-other-orms)
- [Data Migration Strategies](#data-migration-strategies)
- [Common Patterns](#common-patterns)

## Migrating from AWS SDK

### Before: Using AWS SDK v2

```go
// AWS SDK v2 approach
import (
    "context"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
    "github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
)

// Define struct
type User struct {
    ID    string
    Email string
    Name  string
}

// Create item
func createUser(client *dynamodb.Client, user User) error {
    item, err := attributevalue.MarshalMap(user)
    if err != nil {
        return err
    }
    
    input := &dynamodb.PutItemInput{
        TableName: aws.String("Users"),
        Item:      item,
    }
    
    _, err = client.PutItem(context.TODO(), input)
    return err
}

// Query by ID
func getUser(client *dynamodb.Client, id string) (*User, error) {
    input := &dynamodb.GetItemInput{
        TableName: aws.String("Users"),
        Key: map[string]types.AttributeValue{
            "ID": &types.AttributeValueMemberS{Value: id},
        },
    }
    
    result, err := client.GetItem(context.TODO(), input)
    if err != nil {
        return nil, err
    }
    
    var user User
    err = attributevalue.UnmarshalMap(result.Item, &user)
    return &user, err
}
```

### After: Using DynamORM

```go
// DynamORM approach
import "github.com/pay-theory/dynamorm"

// Define model with tags
type User struct {
    ID    string `dynamorm:"pk"`
    Email string `dynamorm:"index:gsi-email,unique"`
    Name  string
}

// Initialize once
db, _ := dynamorm.New(dynamorm.Config{
    Region: "us-east-1",
})

// Create item
func createUser(user *User) error {
    return db.Model(user).Create()
}

// Query by ID
func getUser(id string) (*User, error) {
    var user User
    err := db.Model(&User{}).
        Where("ID", "=", id).
        First(&user)
    return &user, err
}
```

### Key Differences

1. **No Manual Marshaling**: DynamORM handles all attribute value conversions
2. **Type Safety**: Compile-time checking of field names and types
3. **Simpler API**: Fluent interface instead of verbose input structs
4. **Automatic Features**: Timestamps, versioning, and TTL support

## Migrating Complex Queries

### Before: Complex Query with SDK

```go
// Query with filter and pagination
func queryPosts(client *dynamodb.Client, authorID string, lastKey map[string]types.AttributeValue) ([]Post, map[string]types.AttributeValue, error) {
    input := &dynamodb.QueryInput{
        TableName:              aws.String("Posts"),
        IndexName:              aws.String("gsi-author"),
        KeyConditionExpression: aws.String("#author = :authorId"),
        FilterExpression:       aws.String("#status = :status AND #views > :minViews"),
        ExpressionAttributeNames: map[string]string{
            "#author": "AuthorID",
            "#status": "Status",
            "#views":  "ViewCount",
        },
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":authorId": &types.AttributeValueMemberS{Value: authorID},
            ":status":   &types.AttributeValueMemberS{Value: "published"},
            ":minViews": &types.AttributeValueMemberN{Value: "100"},
        },
        Limit:             aws.Int32(10),
        ExclusiveStartKey: lastKey,
    }
    
    result, err := client.Query(context.TODO(), input)
    if err != nil {
        return nil, nil, err
    }
    
    var posts []Post
    err = attributevalue.UnmarshalListOfMaps(result.Items, &posts)
    return posts, result.LastEvaluatedKey, err
}
```

### After: Complex Query with DynamORM

```go
// Query with filter and pagination
func queryPosts(authorID string, cursor string) ([]Post, string, error) {
    var posts []Post
    
    query := db.Model(&Post{}).
        Index("gsi-author").
        Where("AuthorID", "=", authorID).
        Filter("Status", "=", "published").
        Filter("ViewCount", ">", 100).
        Limit(10)
    
    if cursor != "" {
        query = query.Cursor(cursor)
    }
    
    result, err := query.AllPaginated(&posts)
    if err != nil {
        return nil, "", err
    }
    
    return posts, result.NextCursor, nil
}
```

## Migrating Updates

### Before: Update with SDK

```go
// Atomic increment with SDK
func incrementViewCount(client *dynamodb.Client, postID string) error {
    input := &dynamodb.UpdateItemInput{
        TableName: aws.String("Posts"),
        Key: map[string]types.AttributeValue{
            "ID": &types.AttributeValueMemberS{Value: postID},
        },
        UpdateExpression: aws.String("ADD ViewCount :inc SET UpdatedAt = :now"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":inc": &types.AttributeValueMemberN{Value: "1"},
            ":now": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
        },
    }
    
    _, err := client.UpdateItem(context.TODO(), input)
    return err
}
```

### After: Update with DynamORM

```go
// Atomic increment with DynamORM
func incrementViewCount(postID string) error {
    return db.Model(&Post{ID: postID}).
        UpdateBuilder().
        Increment("ViewCount").
        Set("UpdatedAt", time.Now()).
        Execute()
}
```

## Migrating from Other ORMs

### From Guregu's dynamo

```go
// Before: Guregu's dynamo
table := dynamo.NewFromConfig(cfg).Table("Users")

// Put
err := table.Put(user).Run()

// Get
err := table.Get("ID", id).One(&user)

// Query
err := table.Get("Email", email).
    Index("EmailIndex").
    One(&user)

// After: DynamORM
// Put
err := db.Model(user).Create()

// Get
err := db.Model(&User{}).
    Where("ID", "=", id).
    First(&user)

// Query
err := db.Model(&User{}).
    Index("gsi-email").
    Where("Email", "=", email).
    First(&user)
```

## Data Migration Strategies

### 1. Parallel Write Strategy

Keep both systems running during migration:

```go
// Wrapper to write to both systems
type MigrationWrapper struct {
    oldClient *dynamodb.Client
    newDB     *dynamorm.DB
}

func (m *MigrationWrapper) CreateUser(user *User) error {
    // Write to old system
    if err := m.writeToOldSystem(user); err != nil {
        return err
    }
    
    // Write to new system
    return m.newDB.Model(user).Create()
}
```

### 2. Batch Migration

Migrate existing data in batches:

```go
func migrateUsers(oldClient *dynamodb.Client, newDB *dynamorm.DB) error {
    // Scan old table
    paginator := dynamodb.NewScanPaginator(oldClient, &dynamodb.ScanInput{
        TableName: aws.String("Users"),
    })
    
    for paginator.HasMorePages() {
        page, err := paginator.NextPage(context.TODO())
        if err != nil {
            return err
        }
        
        // Convert and batch insert
        var users []User
        for _, item := range page.Items {
            var user User
            attributevalue.UnmarshalMap(item, &user)
            users = append(users, user)
        }
        
        // Batch create in new system
        if err := newDB.Model(&User{}).BatchCreate(users); err != nil {
            return err
        }
    }
    
    return nil
}
```

### 3. Using AutoMigrateWithOptions

DynamORM provides migration support:

```go
// Migrate data from old table structure to new
err := db.AutoMigrateWithOptions(&NewUser{},
    schema.WithDataCopy(true),
    schema.WithBackupTable("Users_backup"),
    schema.WithTransform(func(old OldUser) NewUser {
        return NewUser{
            ID:       old.UserID,  // Field rename
            Email:    old.Email,
            FullName: old.FirstName + " " + old.LastName, // Combine fields
            Active:   true, // Set default
        }
    }),
)
```

## Common Patterns

### 1. Handling Field Renames

```go
// Old model
type OldUser struct {
    UserID   string // Old field name
    UserName string
}

// New model
type User struct {
    ID   string `dynamorm:"pk"`      // New field name
    Name string
}

// Migration function
func migrateUser(old OldUser) User {
    return User{
        ID:   old.UserID,
        Name: old.UserName,
    }
}
```

### 2. Adding Composite Keys

```go
// Old: Simple primary key
type OldOrder struct {
    OrderID    string
    CustomerID string
    Date       time.Time
}

// New: Composite key for better query patterns
type Order struct {
    CustomerID string `dynamorm:"pk"`
    OrderID    string `dynamorm:"sk,prefix:ORDER#"`
    Date       time.Time
}

// Migration
func migrateOrder(old OldOrder) Order {
    return Order{
        CustomerID: old.CustomerID,
        OrderID:    fmt.Sprintf("ORDER#%s", old.OrderID),
        Date:       old.Date,
    }
}
```

### 3. Index Changes

```go
// Add new GSI during migration
err := db.CreateTable(&User{},
    schema.WithGSI("gsi-email", "Email", ""),
    schema.WithGSI("gsi-created", "Status", "CreatedAt"),
)

// Update queries to use new indexes
query := db.Model(&User{}).
    Index("gsi-created").
    Where("Status", "=", "active").
    Where("CreatedAt", ">", lastWeek)
```

## Migration Checklist

- [ ] **Analyze Current Schema**: Document existing tables, indexes, and access patterns
- [ ] **Design New Models**: Add appropriate DynamORM tags
- [ ] **Test in Development**: Verify all queries work as expected
- [ ] **Plan Migration Strategy**: Choose parallel write, batch migration, or gradual rollout
- [ ] **Set Up Monitoring**: Track both old and new system performance
- [ ] **Implement Rollback Plan**: Ensure you can revert if needed
- [ ] **Migrate in Phases**: Start with read-heavy, non-critical tables
- [ ] **Validate Data**: Compare record counts and sample data
- [ ] **Update Application Code**: Replace SDK calls with DynamORM
- [ ] **Performance Test**: Ensure new system meets requirements
- [ ] **Cutover**: Switch traffic to new system
- [ ] **Cleanup**: Remove old code and tables

## Best Practices

1. **Start Small**: Migrate one table at a time
2. **Test Thoroughly**: Verify all access patterns work
3. **Monitor Performance**: Watch for unexpected hot partitions
4. **Keep Backups**: Use DynamoDB point-in-time recovery
5. **Document Changes**: Track schema and query modifications

## Troubleshooting

### "ValidationException" Errors

Check that your DynamORM tags match the actual table schema:

```go
// Verify table structure
desc, err := db.DescribeTable(&User{})
```

### Performance Degradation

Ensure indexes are properly utilized:

```go
// Check if query uses index
query := db.Model(&User{}).
    Where("Email", "=", email) // This will scan without index!

// Use index explicitly
query := db.Model(&User{}).
    Index("gsi-email").
    Where("Email", "=", email) // This uses index
```

### Data Type Mismatches

DynamORM handles type conversions, but be aware of:
- Time formats (RFC3339 by default)
- Number precision (use strings for large numbers)
- Empty strings (not allowed in DynamoDB)

## Conclusion

Migrating to DynamORM simplifies your codebase while providing type safety and better developer experience. Take time to plan your migration strategy and test thoroughly in development before moving to production. 
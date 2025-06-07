# DynamORM vs Raw DynamoDB SDK Comparison

This document demonstrates how DynamORM simplifies common DynamoDB operations compared to using the raw AWS SDK.

## Table of Contents
1. [Model Definition](#model-definition)
2. [Basic CRUD Operations](#basic-crud-operations)
3. [Query Operations](#query-operations)
4. [Index Management](#index-management)
5. [Transactions](#transactions)
6. [Batch Operations](#batch-operations)
7. [Complex Queries](#complex-queries)

## Model Definition

### Raw SDK
```go
// No built-in model definition
// You need to manually handle attribute names and types
type User struct {
    ID        string
    Email     string
    Name      string
    Age       int
    CreatedAt time.Time
    Tags      []string
}

// Manual marshaling required
func (u *User) MarshalDynamoDB() (map[string]*dynamodb.AttributeValue, error) {
    return map[string]*dynamodb.AttributeValue{
        "ID":        {S: aws.String(u.ID)},
        "Email":     {S: aws.String(u.Email)},
        "Name":      {S: aws.String(u.Name)},
        "Age":       {N: aws.String(strconv.Itoa(u.Age))},
        "CreatedAt": {S: aws.String(u.CreatedAt.Format(time.RFC3339))},
        "Tags":      {SS: aws.StringSlice(u.Tags)},
    }, nil
}
```

### DynamORM
```go
type User struct {
    ID        string    `dynamorm:"pk"`
    Email     string    `dynamorm:"index:gsi-email"`
    Name      string    
    Age       int       
    CreatedAt time.Time `dynamorm:"sk"`
    Tags      []string  `dynamorm:"set"`
}
// That's it! Marshaling is automatic
```

## Basic CRUD Operations

### Create Item

#### Raw SDK
```go
func CreateUser(user *User) error {
    item, err := dynamodbattribute.MarshalMap(user)
    if err != nil {
        return err
    }
    
    input := &dynamodb.PutItemInput{
        TableName: aws.String("users"),
        Item:      item,
    }
    
    _, err = svc.PutItem(input)
    return err
}
```

#### DynamORM
```go
func CreateUser(user *User) error {
    return db.Model(user).Create()
}
```

### Read Item

#### Raw SDK
```go
func GetUser(id string) (*User, error) {
    input := &dynamodb.GetItemInput{
        TableName: aws.String("users"),
        Key: map[string]*dynamodb.AttributeValue{
            "ID": {S: aws.String(id)},
        },
    }
    
    result, err := svc.GetItem(input)
    if err != nil {
        return nil, err
    }
    
    if result.Item == nil {
        return nil, errors.New("user not found")
    }
    
    user := &User{}
    err = dynamodbattribute.UnmarshalMap(result.Item, user)
    return user, err
}
```

#### DynamORM
```go
func GetUser(id string) (*User, error) {
    user := &User{}
    err := db.Model(user).Where("ID", "=", id).First()
    return user, err
}
```

### Update Item

#### Raw SDK
```go
func UpdateUserEmail(id, email string) error {
    input := &dynamodb.UpdateItemInput{
        TableName: aws.String("users"),
        Key: map[string]*dynamodb.AttributeValue{
            "ID": {S: aws.String(id)},
        },
        UpdateExpression: aws.String("SET Email = :email"),
        ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
            ":email": {S: aws.String(email)},
        },
    }
    
    _, err := svc.UpdateItem(input)
    return err
}
```

#### DynamORM
```go
func UpdateUserEmail(id, email string) error {
    return db.Model(&User{}).
        Where("ID", "=", id).
        Update("Email", email)
}
```

### Delete Item

#### Raw SDK
```go
func DeleteUser(id string) error {
    input := &dynamodb.DeleteItemInput{
        TableName: aws.String("users"),
        Key: map[string]*dynamodb.AttributeValue{
            "ID": {S: aws.String(id)},
        },
    }
    
    _, err := svc.DeleteItem(input)
    return err
}
```

#### DynamORM
```go
func DeleteUser(id string) error {
    return db.Model(&User{}).
        Where("ID", "=", id).
        Delete()
}
```

## Query Operations

### Query with Index

#### Raw SDK
```go
func GetUsersByEmail(email string) ([]*User, error) {
    input := &dynamodb.QueryInput{
        TableName:              aws.String("users"),
        IndexName:              aws.String("gsi-email"),
        KeyConditionExpression: aws.String("Email = :email"),
        ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
            ":email": {S: aws.String(email)},
        },
    }
    
    result, err := svc.Query(input)
    if err != nil {
        return nil, err
    }
    
    users := []*User{}
    for _, item := range result.Items {
        user := &User{}
        if err := dynamodbattribute.UnmarshalMap(item, user); err != nil {
            return nil, err
        }
        users = append(users, user)
    }
    
    return users, nil
}
```

#### DynamORM
```go
func GetUsersByEmail(email string) ([]*User, error) {
    var users []*User
    err := db.Model(&User{}).
        Index("gsi-email").
        Where("Email", "=", email).
        All(&users)
    return users, err
}
```

### Scan with Filter

#### Raw SDK
```go
func GetActiveUsers(minAge int) ([]*User, error) {
    input := &dynamodb.ScanInput{
        TableName:        aws.String("users"),
        FilterExpression: aws.String("Age >= :age AND contains(Tags, :tag)"),
        ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
            ":age": {N: aws.String(strconv.Itoa(minAge))},
            ":tag": {S: aws.String("active")},
        },
    }
    
    var users []*User
    err := svc.ScanPages(input, func(page *dynamodb.ScanOutput, lastPage bool) bool {
        for _, item := range page.Items {
            user := &User{}
            if err := dynamodbattribute.UnmarshalMap(item, user); err == nil {
                users = append(users, user)
            }
        }
        return !lastPage
    })
    
    return users, err
}
```

#### DynamORM
```go
func GetActiveUsers(minAge int) ([]*User, error) {
    var users []*User
    err := db.Model(&User{}).
        Where("Age", ">=", minAge).
        Filter("contains(Tags, :tag)", Param("tag", "active")).
        Scan(&users)
    return users, err
}
```

## Index Management

### Create Table with Indexes

#### Raw SDK
```go
func CreateUserTable() error {
    input := &dynamodb.CreateTableInput{
        TableName: aws.String("users"),
        KeySchema: []*dynamodb.KeySchemaElement{
            {
                AttributeName: aws.String("ID"),
                KeyType:       aws.String(dynamodb.KeyTypeHash),
            },
            {
                AttributeName: aws.String("CreatedAt"),
                KeyType:       aws.String(dynamodb.KeyTypeRange),
            },
        },
        AttributeDefinitions: []*dynamodb.AttributeDefinition{
            {
                AttributeName: aws.String("ID"),
                AttributeType: aws.String(dynamodb.ScalarAttributeTypeS),
            },
            {
                AttributeName: aws.String("CreatedAt"),
                AttributeType: aws.String(dynamodb.ScalarAttributeTypeS),
            },
            {
                AttributeName: aws.String("Email"),
                AttributeType: aws.String(dynamodb.ScalarAttributeTypeS),
            },
        },
        GlobalSecondaryIndexes: []*dynamodb.GlobalSecondaryIndex{
            {
                IndexName: aws.String("gsi-email"),
                KeySchema: []*dynamodb.KeySchemaElement{
                    {
                        AttributeName: aws.String("Email"),
                        KeyType:       aws.String(dynamodb.KeyTypeHash),
                    },
                },
                Projection: &dynamodb.Projection{
                    ProjectionType: aws.String(dynamodb.ProjectionTypeAll),
                },
                ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
                    ReadCapacityUnits:  aws.Int64(5),
                    WriteCapacityUnits: aws.Int64(5),
                },
            },
        },
        BillingMode: aws.String(dynamodb.BillingModePayPerRequest),
    }
    
    _, err := svc.CreateTable(input)
    return err
}
```

#### DynamORM
```go
// Automatic from struct tags!
err := db.AutoMigrate(&User{})
```

## Transactions

### Transaction Example

#### Raw SDK
```go
func TransferCredits(fromID, toID string, amount int) error {
    getFrom := &dynamodb.Get{
        TableName: aws.String("accounts"),
        Key: map[string]*dynamodb.AttributeValue{
            "ID": {S: aws.String(fromID)},
        },
    }
    
    getTo := &dynamodb.Get{
        TableName: aws.String("accounts"),
        Key: map[string]*dynamodb.AttributeValue{
            "ID": {S: aws.String(toID)},
        },
    }
    
    // Execute gets to check balances
    // ... complex balance checking logic ...
    
    updateFrom := &dynamodb.Update{
        TableName: aws.String("accounts"),
        Key: map[string]*dynamodb.AttributeValue{
            "ID": {S: aws.String(fromID)},
        },
        UpdateExpression: aws.String("SET Balance = Balance - :amount"),
        ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
            ":amount": {N: aws.String(strconv.Itoa(amount))},
        },
        ConditionExpression: aws.String("Balance >= :amount"),
    }
    
    updateTo := &dynamodb.Update{
        TableName: aws.String("accounts"),
        Key: map[string]*dynamodb.AttributeValue{
            "ID": {S: aws.String(toID)},
        },
        UpdateExpression: aws.String("SET Balance = Balance + :amount"),
        ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
            ":amount": {N: aws.String(strconv.Itoa(amount))},
        },
    }
    
    input := &dynamodb.TransactWriteItemsInput{
        TransactItems: []*dynamodb.TransactWriteItem{
            {Update: updateFrom},
            {Update: updateTo},
        },
    }
    
    _, err := svc.TransactWriteItems(input)
    return err
}
```

#### DynamORM
```go
func TransferCredits(fromID, toID string, amount int) error {
    return db.Transaction(func(tx *Transaction) error {
        // Check balance
        var fromAccount Account
        if err := tx.Model(&Account{}).
            Where("ID", "=", fromID).
            First(&fromAccount); err != nil {
            return err
        }
        
        if fromAccount.Balance < amount {
            return errors.New("insufficient balance")
        }
        
        // Update both accounts
        if err := tx.Model(&Account{}).
            Where("ID", "=", fromID).
            Update("Balance", fromAccount.Balance - amount); err != nil {
            return err
        }
        
        return tx.Model(&Account{}).
            Where("ID", "=", toID).
            Increment("Balance", amount)
    })
}
```

## Batch Operations

### Batch Write

#### Raw SDK
```go
func BatchCreateUsers(users []*User) error {
    // DynamoDB limits batch to 25 items
    for i := 0; i < len(users); i += 25 {
        end := i + 25
        if end > len(users) {
            end = len(users)
        }
        
        batch := users[i:end]
        writeRequests := make([]*dynamodb.WriteRequest, len(batch))
        
        for j, user := range batch {
            item, _ := dynamodbattribute.MarshalMap(user)
            writeRequests[j] = &dynamodb.WriteRequest{
                PutRequest: &dynamodb.PutRequest{
                    Item: item,
                },
            }
        }
        
        input := &dynamodb.BatchWriteItemInput{
            RequestItems: map[string][]*dynamodb.WriteRequest{
                "users": writeRequests,
            },
        }
        
        _, err := svc.BatchWriteItem(input)
        if err != nil {
            return err
        }
    }
    
    return nil
}
```

#### DynamORM
```go
func BatchCreateUsers(users []*User) error {
    return db.Model(&User{}).BatchCreate(users)
    // Automatic chunking and error handling!
}
```

## Complex Queries

### Multiple Conditions with Pagination

#### Raw SDK
```go
func SearchUsers(namePrefix string, minAge int, lastKey map[string]*dynamodb.AttributeValue) ([]*User, map[string]*dynamodb.AttributeValue, error) {
    input := &dynamodb.ScanInput{
        TableName:        aws.String("users"),
        FilterExpression: aws.String("begins_with(#name, :prefix) AND Age >= :age"),
        ExpressionAttributeNames: map[string]*string{
            "#name": aws.String("Name"), // Name is a reserved word
        },
        ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
            ":prefix": {S: aws.String(namePrefix)},
            ":age":    {N: aws.String(strconv.Itoa(minAge))},
        },
        Limit: aws.Int64(20),
    }
    
    if lastKey != nil {
        input.ExclusiveStartKey = lastKey
    }
    
    result, err := svc.Scan(input)
    if err != nil {
        return nil, nil, err
    }
    
    users := []*User{}
    for _, item := range result.Items {
        user := &User{}
        if err := dynamodbattribute.UnmarshalMap(item, user); err != nil {
            return nil, nil, err
        }
        users = append(users, user)
    }
    
    return users, result.LastEvaluatedKey, nil
}
```

#### DynamORM
```go
func SearchUsers(namePrefix string, minAge int, cursor string) ([]*User, string, error) {
    var users []*User
    query := db.Model(&User{}).
        Where("Name", "begins_with", namePrefix).
        Where("Age", ">=", minAge).
        Limit(20)
    
    if cursor != "" {
        query = query.Cursor(cursor)
    }
    
    nextCursor, err := query.Scan(&users)
    return users, nextCursor, err
}
```

## Summary

DynamORM dramatically reduces the complexity and verbosity of working with DynamoDB:

| Operation | Raw SDK Lines | DynamORM Lines | Reduction |
|-----------|--------------|----------------|-----------|
| Create Item | 15+ | 1 | 93% |
| Query with Index | 25+ | 5 | 80% |
| Transaction | 50+ | 15 | 70% |
| Batch Operations | 30+ | 1 | 97% |
| Complex Query | 35+ | 8 | 77% |

Beyond line count, DynamORM provides:
- **Type Safety**: Compile-time checking prevents runtime errors
- **Automatic Marshaling**: No manual AttributeValue handling
- **Index Management**: Automatic index selection and optimization
- **Error Handling**: Consistent, typed errors
- **Best Practices**: Built-in patterns for DynamoDB optimization 
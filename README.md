# DynamORM

A powerful, expressive, and type-safe DynamoDB ORM for Go that eliminates the complexity and verbosity of working with DynamoDB while maintaining its performance and scalability benefits.

## 🎯 Project Vision

DynamoDB is an incredible database - it's fast, cheap, and scales fantastically. However, its verbose API and complex data structures make it challenging to work with. DynamORM aims to provide developers with an intuitive, Go-idiomatic interface for DynamoDB without sacrificing any of its power.

## 📚 Documentation

- [**Design Document**](DESIGN.md) - Comprehensive overview of DynamORM's features and API design
- [**Architecture**](ARCHITECTURE.md) - Technical architecture and implementation details
- [**Roadmap**](ROADMAP.md) - Detailed implementation plan and timeline
- [**Comparison**](COMPARISON.md) - Side-by-side comparison with raw DynamoDB SDK

## ✨ Key Features

### 🚀 Developer Experience First
```go
// Instead of 15+ lines of SDK code, just:
err := db.Model(user).Create()
```

### 🔍 Intuitive Query Builder
```go
users, err := db.Model(&User{}).
    Where("Age", ">", 18).
    Where("Status", "=", "active").
    OrderBy("CreatedAt", "desc").
    Limit(10).
    All()
```

### 📊 Automatic Index Management
```go
type User struct {
    ID    string `dynamorm:"pk"`
    Email string `dynamorm:"index:gsi-email"`
    // DynamORM automatically creates and manages indexes
}
```

### 💰 Transaction Support
```go
err := db.Transaction(func(tx *Transaction) error {
    tx.Create(&user)
    tx.Update(&account)
    return nil
})
```

### 🎯 Type Safety
- Compile-time validation
- No manual AttributeValue handling
- Automatic marshaling/unmarshaling

### ⚡ Performance
- < 5% overhead vs raw SDK
- Intelligent query optimization
- Automatic batching
- Connection pooling

## 🏗️ Project Status

This project is currently in the **design phase**. We've completed:

- ✅ Comprehensive API design
- ✅ Technical architecture
- ✅ Implementation roadmap
- ✅ Feature comparison with raw SDK

Next steps:
- 🚧 Phase 1: Core foundation implementation
- 🚧 Phase 2: Query builder
- 🚧 Phase 3: Advanced queries
- 🚧 Phase 4: Index management

## 🤔 Why DynamORM?

### Without DynamORM (Raw SDK)
```go
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
```

### With DynamORM
```go
var users []*User
err := db.Model(&User{}).
    Index("gsi-email").
    Where("Email", "=", email).
    All(&users)
```

**80% less code. 100% more clarity.**

## 🎨 Design Principles

1. **Intuitive API** - If you know Go, you know DynamORM
2. **Zero Magic** - Explicit, predictable behavior
3. **Type Safety** - Catch errors at compile time, not runtime
4. **Best Practices by Default** - The easy way is the right way
5. **Progressive Disclosure** - Simple things simple, complex things possible

## 🚀 Coming Soon

DynamORM will provide:

- **Migrations** - Version your schema with confidence
- **Testing Utilities** - Mock DynamoDB for unit tests
- **Performance Monitoring** - Built-in metrics and tracing
- **CLI Tools** - Manage schemas and migrations
- **Plugin System** - Extend with custom functionality

## 📈 Success Metrics

Our goals for v1.0:
- < 5% performance overhead vs raw SDK
- 80% code reduction for common operations
- 100% type safety at compile time
- > 90% test coverage
- Comprehensive documentation

## 🤝 Contributing

DynamORM is currently in active design and development. We welcome feedback on our design documents and architecture decisions. 

## 📄 License

DynamORM will be released under the MIT License.

---

*Built with ❤️ for the Go and DynamoDB community*

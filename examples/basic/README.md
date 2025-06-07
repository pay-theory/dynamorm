# DynamORM Basic CRUD Tutorial

Welcome to the DynamORM basic CRUD tutorial! This guide will teach you the fundamentals of using DynamORM through three progressively complex examples.

## 🎯 Learning Path

1. **Todo App** - Learn the basics of Create, Read, Update, Delete
2. **Notes App** - Add indexes, tags, and timestamps
3. **Contacts App** - Master complex queries and search patterns

## 📚 Prerequisites

- Go 1.21 or later
- AWS account (for DynamoDB)
- Basic Go knowledge

## 🚀 Quick Start

Each example can be run locally with DynamoDB Local:

```bash
# Start DynamoDB Local (from any example directory)
docker-compose up -d

# Run the todo example
cd todo
go run main.go

# Run the notes example
cd ../notes
go run main.go

# Run the contacts example
cd ../contacts
go run main.go
```

## 📖 Tutorial Structure

### 1. Todo App - The Basics (15 minutes)

Learn fundamental CRUD operations with a simple todo list:
- Define models with DynamORM tags
- Create, read, update, and delete items
- Handle errors properly
- Use unique IDs

**Key concepts:**
- `dynamorm:"pk"` for primary keys
- Basic model operations
- Error handling patterns

### 2. Notes App - Intermediate Features (20 minutes)

Build on the basics with a note-taking app:
- Add global secondary indexes for queries
- Work with sets (tags)
- Implement timestamps
- Query by different attributes

**Key concepts:**
- `dynamorm:"index"` for GSI
- Working with sets and lists
- Timestamp patterns
- Query operations

### 3. Contacts App - Advanced Patterns (25 minutes)

Master DynamORM with a contacts management system:
- Composite keys for organization
- Complex filtering
- Pagination
- Search patterns
- Batch operations

**Key concepts:**
- Composite primary keys
- Advanced queries
- Pagination with cursors
- Batch operations
- Search strategies

## 🎓 What You'll Learn

By completing this tutorial, you'll understand:

1. **Model Definition**
   - How to structure DynamoDB tables with Go structs
   - Using DynamORM tags effectively
   - Choosing the right key schema

2. **CRUD Operations**
   - Creating items with validation
   - Reading single items and lists
   - Updating with optimistic locking
   - Safe deletion patterns

3. **Querying & Indexing**
   - When to use Query vs Scan
   - Designing effective GSIs
   - Filtering and pagination
   - Performance optimization

4. **Best Practices**
   - Error handling
   - ID generation strategies
   - Timestamp management
   - Testing approaches

## 🏗️ Common Patterns

### Model Definition
```go
type Todo struct {
    ID        string    `dynamorm:"pk"`
    Title     string    `dynamorm:"required"`
    Completed bool      
    CreatedAt time.Time
}
```

### Create Operation
```go
todo := Todo{
    ID:        uuid.New().String(),
    Title:     "Learn DynamORM",
    CreatedAt: time.Now(),
}
err := db.Model(&todo).Create()
```

### Query with Index
```go
var notes []Note
result, err := db.Query("gsi-user").
    Where("UserID", "=", userID).
    Limit(10).
    Execute(ctx, &notes)
```

### Update with Condition
```go
err := db.Model(&todo).
    Update().
    Set("Completed", true).
    Condition("ID", "=", todo.ID).
    Execute()
```

## 🚦 Next Steps

After completing this tutorial:

1. Explore the **Blog** example for content management patterns
2. Check out **E-commerce** for transaction handling
3. Study **Payment** for financial data patterns
4. Review **Multi-tenant** for SaaS architectures

## 💡 Tips for Success

1. **Start Simple**: Don't skip the todo example even if it seems basic
2. **Run the Code**: Execute each example and experiment with modifications
3. **Read Errors**: DynamORM provides helpful error messages
4. **Check AWS Console**: Verify your data in DynamoDB to understand storage
5. **Ask Questions**: The patterns you learn here apply to larger applications

## 📝 Need Help?

- Check individual example READMEs for detailed instructions
- Review DynamORM documentation for advanced features
- Join our community for questions and discussions

Happy coding! 🎉 
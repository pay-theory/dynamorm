# Todo App - DynamORM Basics

This is the simplest DynamORM example, designed to teach you the fundamental CRUD operations.

## What You'll Learn

- Model definition with DynamORM tags
- Creating items with validation
- Reading items by primary key
- Updating existing items
- Deleting items
- Basic error handling

## Quick Start

### 1. Start DynamoDB Local

```bash
# From this directory
docker-compose up -d
```

Or use the docker-compose.yml file:

```yaml
version: '3.8'
services:
  dynamodb-local:
    image: amazon/dynamodb-local:latest
    ports:
      - "8000:8000"
    command: ["-jar", "DynamoDBLocal.jar", "-sharedDb", "-inMemory"]
```

### 2. Run the Application

```bash
# Install dependencies
go mod tidy

# Run the todo app
go run main.go
```

### 3. Try These Commands

```
> add Learn DynamORM basics
✅ Created todo: Learn DynamORM basics

> add Build a real application
✅ Created todo: Build a real application

> list
📝 Your Todos:
─────────────
1. [ ] Learn DynamORM basics (ID: a1b2c3d4)
2. [ ] Build a real application (ID: e5f6g7h8)

> complete 1
✅ Todo updated successfully

> list
📝 Your Todos:
─────────────
1. [✓] Learn DynamORM basics (ID: a1b2c3d4)
2. [ ] Build a real application (ID: e5f6g7h8)

> delete 2
✅ Todo deleted successfully
```

## Code Walkthrough

### 1. Model Definition

```go
type Todo struct {
    ID        string    `dynamorm:"pk"`        // Primary key
    Title     string    `dynamorm:"required"`  // Required field
    Completed bool                            // Regular field
    CreatedAt time.Time                       // Timestamp
    UpdatedAt time.Time                       // Last modified
}
```

**Key Points:**
- Every DynamoDB table needs a primary key (`pk` tag)
- Use `required` tag for validation
- DynamORM handles type conversion automatically

### 2. Database Connection

```go
cfg := &session.Config{
    Region:   "us-east-1",
    Endpoint: "http://localhost:8000", // For local development
    AWSConfigOptions: []func(*config.LoadOptions) error{
        config.WithCredentialsProvider(
            credentials.NewStaticCredentialsProvider("dummy", "dummy", "")
        ),
    },
}

db, err := dynamorm.New(*cfg)
```

**Key Points:**
- Use `Endpoint` for local DynamoDB
- Dummy credentials work for local development
- In production, use IAM roles

### 3. Create Operation

```go
todo := &Todo{
    ID:        uuid.New().String(),
    Title:     title,
    Completed: false,
    CreatedAt: time.Now(),
    UpdatedAt: time.Now(),
}

err := db.Model(todo).Create()
```

**Key Points:**
- Always generate unique IDs (UUIDs work well)
- `Create()` validates required fields
- Returns error if item already exists

### 4. Read Operations

```go
// Get single item by primary key
var todo Todo
err := db.Model(&Todo{}).Where("ID", "=", id).First(&todo)

// List all items (scan)
var todos []Todo
err := db.Model(&Todo{}).Scan(&todos)
```

**Key Points:**
- `First()` for single items
- `Scan()` for all items (use sparingly on large tables)
- Always check for `errors.IsNotFound(err)`

### 5. Update Operation

```go
// Get the item first
todo, err := app.Get(id)

// Modify fields
todo.Title = "Updated title"
todo.UpdatedAt = time.Now()

// Save changes
err = db.Model(todo).Update()
```

**Key Points:**
- DynamORM uses full item replacement by default
- Always update timestamps
- Consider using conditional updates for concurrency

### 6. Delete Operation

```go
err := db.Model(&Todo{}).Where("ID", "=", id).Delete()
```

**Key Points:**
- Delete by primary key is most efficient
- Check if item exists before deleting
- Consider soft deletes for audit trails

## Common Patterns

### Error Handling

```go
if err != nil {
    if errors.IsNotFound(err) {
        // Item doesn't exist
        return fmt.Errorf("todo not found")
    }
    // Other error
    return fmt.Errorf("database error: %v", err)
}
```

### ID Generation

```go
// UUID - Recommended for distributed systems
id := uuid.New().String()

// Timestamp-based (sortable)
id := fmt.Sprintf("%d-%s", time.Now().Unix(), uuid.New().String()[:8])

// Custom prefix
id := fmt.Sprintf("TODO#%s", uuid.New().String())
```

### Timestamps

```go
type BaseModel struct {
    CreatedAt time.Time
    UpdatedAt time.Time
}

// Before create
model.CreatedAt = time.Now()
model.UpdatedAt = time.Now()

// Before update
model.UpdatedAt = time.Now()
```

## Exercises

1. **Add Priority**: Add a priority field (high/medium/low) to todos
2. **Due Dates**: Add due date support and list overdue todos
3. **Tags**: Add a tags field (string set) for categorization
4. **Search**: Implement search by title substring
5. **Sort**: Sort todos by creation date or priority

## What's Next?

Now that you understand the basics, move on to:

1. **Notes App**: Learn about indexes and queries
2. **Contacts App**: Master complex queries and filtering
3. **Blog Example**: See production patterns

## Troubleshooting

### "Table already exists" error
This is normal - the table persists in DynamoDB Local. Either ignore it or delete the table first.

### "Connection refused" error
Make sure DynamoDB Local is running: `docker-compose up -d`

### Import errors
Run `go mod tidy` to download dependencies

## Key Takeaways

✅ **Simple is Good**: Start with basic CRUD before complex features
✅ **IDs Matter**: Always use unique, generated IDs
✅ **Handle Errors**: Check for specific error types
✅ **Think NoSQL**: Design for your access patterns
✅ **Test Locally**: DynamoDB Local is perfect for development 
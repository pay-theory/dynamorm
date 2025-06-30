# Consistency Patterns in DynamORM

This guide explains how to handle DynamoDB's eventual consistency characteristics when using DynamORM, particularly with Global Secondary Indexes (GSIs).

## Understanding DynamoDB Consistency

DynamoDB offers two consistency models:

1. **Eventually Consistent Reads** (default) - May not reflect the most recent write
2. **Strongly Consistent Reads** - Always returns the most recent data

### Important Notes

- GSIs **always** use eventually consistent reads, even with transactions
- Main table queries can use strongly consistent reads
- Transactions ensure atomicity but don't change GSI consistency behavior

## ConsistentRead() - Strong Consistency for Main Table

Use `ConsistentRead()` for strongly consistent reads on the main table:

```go
// Strongly consistent read on main table
var user User
err := db.Model(&User{}).
    Where("ID", "=", "123").
    ConsistentRead().
    First(&user)

// Note: ConsistentRead is ignored when using GSIs
err = db.Model(&User{}).
    Index("email-index").
    Where("Email", "=", "user@example.com").
    ConsistentRead(). // This has no effect on GSI queries
    First(&user)
```

## WithRetry() - Handling GSI Eventual Consistency

When querying GSIs immediately after writes, use `WithRetry()` to handle eventual consistency:

```go
// Create a user
user := &User{
    ID:    "123",
    Email: "user@example.com",
    Name:  "John Doe",
}
err := db.Model(user).Create()

// Query GSI with retry
var result User
err = db.Model(&User{}).
    Index("email-index").
    Where("Email", "=", "user@example.com").
    WithRetry(5, 100*time.Millisecond). // Retry up to 5 times with 100ms initial delay
    First(&result)
```

### Retry Configuration

- `maxRetries`: Maximum number of retry attempts
- `initialDelay`: Initial delay between retries
- Uses exponential backoff with a maximum delay of 5 seconds

## Consistency Helper Package

DynamORM provides a `consistency` package with advanced patterns:

### ReadAfterWriteHelper

```go
import "github.com/pay-theory/dynamorm/pkg/consistency"

helper := consistency.NewReadAfterWriteHelper(db)

// Create with verification
err := helper.CreateWithConsistency(user, &consistency.WriteOptions{
    VerifyWrite:           true,  // Perform strongly consistent read after write
    WaitForGSIPropagation: 500 * time.Millisecond, // Wait for GSI propagation
})

// Update with verification
user.Name = "Jane Doe"
err = helper.UpdateWithConsistency(user, []string{"Name"}, &consistency.WriteOptions{
    VerifyWrite: true,
})
```

### QueryAfterWrite Patterns

```go
// Option 1: Use main table for immediate consistency
var result User
err := helper.QueryAfterWrite(&User{}, &consistency.QueryAfterWriteOptions{
    UseMainTable: true,
}).
    Where("ID", "=", "123").
    First(&result)

// Option 2: Use GSI with retry
err = helper.QueryAfterWrite(&User{}, &consistency.QueryAfterWriteOptions{
    RetryConfig: consistency.RecommendedRetryConfig(),
}).
    Index("email-index").
    Where("Email", "=", "user@example.com").
    First(&result)

// Option 3: Custom verification function
err = helper.QueryAfterWrite(&User{}, &consistency.QueryAfterWriteOptions{
    RetryConfig: consistency.RecommendedRetryConfig(),
    VerifyFunc: func(result any) bool {
        user := result.(*User)
        return user.UpdatedAt.After(writeTime)
    },
}).
    Index("email-index").
    Where("Email", "=", "user@example.com").
    First(&result)
```

### Complete Write-and-Read Pattern

```go
pattern := consistency.NewWriteAndReadPattern(db)

// Create and immediately query via GSI
err := pattern.CreateAndQueryGSI(
    user,           // Item to create
    "email-index",  // GSI name
    "Email",        // GSI key field
    "user@example.com", // GSI key value
    &result,        // Destination
)

// Update and verify with strong consistency
user.Name = "Updated Name"
err = pattern.UpdateAndVerify(user, []string{"Name"})
```

## Best Practices

### 1. Choose the Right Strategy

```go
bp := &consistency.BestPractices{}

// For GSI queries after writes
strategy := bp.ForGSIQuery() // Returns StrategyRetryWithBackoff

// For critical reads that must be consistent
strategy := bp.ForCriticalReads() // Returns StrategyStrongConsistency

// For high-throughput scenarios
strategy := bp.ForHighThroughput() // Returns StrategyDelayedRead
```

### 2. Recommended Configurations

```go
// Recommended retry configuration
config := consistency.RecommendedRetryConfig()
// MaxRetries: 5
// InitialDelay: 100ms
// MaxDelay: 2s
// BackoffFactor: 2.0

// Recommended GSI propagation delay
delay := consistency.RecommendedGSIPropagationDelay() // 500ms
```

### 3. Pattern Selection Guide

| Scenario | Recommended Pattern | Example |
|----------|-------------------|---------|
| Critical data read | ConsistentRead() on main table | User authentication |
| GSI query after write | WithRetry() | Email lookup after registration |
| High-volume writes | Delayed read with fixed wait | Bulk imports |
| Mixed consistency needs | ReadAfterWriteHelper | User profile updates |

## Common Pitfalls

1. **Don't assume GSI immediate consistency**
   ```go
   // BAD: May fail due to GSI eventual consistency
   db.Model(user).Create()
   db.Model(&User{}).Index("email-index").Where("Email", "=", email).First(&result)
   
   // GOOD: Use retry or main table
   db.Model(user).Create()
   db.Model(&User{}).Index("email-index").Where("Email", "=", email).WithRetry(5, 100*time.Millisecond).First(&result)
   ```

2. **Don't use ConsistentRead with GSIs**
   ```go
   // BAD: ConsistentRead is ignored for GSIs
   db.Model(&User{}).Index("email-index").ConsistentRead().First(&user)
   
   // GOOD: Use main table for consistent reads
   db.Model(&User{}).Where("ID", "=", id).ConsistentRead().First(&user)
   ```

3. **Don't retry indefinitely**
   ```go
   // BAD: Too many retries
   db.Model(&User{}).WithRetry(100, 10*time.Millisecond).First(&user)
   
   // GOOD: Reasonable retry limits
   db.Model(&User{}).WithRetry(5, 100*time.Millisecond).First(&user)
   ```

## Testing Consistency

When writing tests that involve GSI queries:

```go
// In tests, always account for eventual consistency
func TestUserCreation(t *testing.T) {
    user := &User{Email: "test@example.com"}
    db.Model(user).Create()
    
    // Use retry for GSI queries in tests
    var result User
    err := db.Model(&User{}).
        Index("email-index").
        Where("Email", "=", user.Email).
        WithRetry(5, 50*time.Millisecond).
        First(&result)
    
    assert.NoError(t, err)
    assert.Equal(t, user.Email, result.Email)
}
```

## Performance Considerations

1. **ConsistentRead** consumes twice the read capacity units
2. **Retries** can increase latency and API calls
3. **Fixed delays** block execution but are predictable
4. **Main table queries** may require different access patterns

Choose your consistency strategy based on your specific requirements for data freshness, performance, and cost.
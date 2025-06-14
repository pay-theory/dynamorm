# Atomic Operations Guide

DynamORM fully supports DynamoDB's native atomic operations through the UpdateBuilder interface. This guide shows how to use atomic operations for counters, sets, and lists without race conditions.

## Overview

Atomic operations execute at the DynamoDB level without read-modify-write cycles, eliminating race conditions and improving performance.

## Atomic Counter Operations

### Increment/Decrement

```go
// Atomic increment by 1
err := db.Model(&PageView{ID: "page-123"}).
    UpdateBuilder().
    Increment("ViewCount").
    Execute()

// Atomic decrement by 1
err := db.Model(&Inventory{SKU: "ABC123"}).
    UpdateBuilder().
    Decrement("Quantity").
    Execute()

// Atomic add/subtract custom value
err := db.Model(&Account{ID: "acc-456"}).
    UpdateBuilder().
    Add("Balance", 100.50).    // Add $100.50
    Execute()

// Subtract by using negative value
err := db.Model(&Account{ID: "acc-456"}).
    UpdateBuilder().
    Add("Balance", -50.25).    // Subtract $50.25
    Execute()
```

### Rate Limiter Example

```go
type RateLimit struct {
    PK           string    `dynamorm:"pk"`  // "USER#userId"
    SK           string    `dynamorm:"sk"`  // "WINDOW#2024-01-15T10:00:00Z"
    RequestCount int64     `json:"request_count"`
    WindowStart  time.Time `json:"window_start"`
    TTL          int64     `dynamorm:"ttl"`
}

func (r *RateLimit) SetKeys(userID string, windowStart time.Time) {
    r.PK = fmt.Sprintf("USER#%s", userID)
    r.SK = fmt.Sprintf("WINDOW#%s", windowStart.Format(time.RFC3339))
    r.WindowStart = windowStart
    r.TTL = windowStart.Add(time.Hour).Unix()
}

// Atomic rate limit check and increment
func CheckRateLimit(db core.DB, userID string, limit int64) (bool, int64, error) {
    windowStart := time.Now().UTC().Truncate(time.Hour)
    
    rateLimit := &RateLimit{}
    rateLimit.SetKeys(userID, windowStart)
    
    // Atomic increment with conditional check
    err := db.Model(rateLimit).
        UpdateBuilder().
        Add("RequestCount", 1).                           // Atomic increment
        SetIfNotExists("RequestCount", 1, 1).             // Initialize if new
        SetIfNotExists("WindowStart", windowStart, windowStart).
        SetIfNotExists("TTL", rateLimit.TTL, rateLimit.TTL).
        Condition("RequestCount", "<", limit).            // Fail if over limit
        ExecuteWithResult(rateLimit)                      // Get updated values
    
    if err != nil {
        if errors.Is(err, customerrors.ErrConditionFailed) {
            // Over rate limit
            return false, limit, nil
        }
        return false, 0, err
    }
    
    // Under limit, return new count
    return true, rateLimit.RequestCount, nil
}
```

## Atomic Set Operations

### Add to Set

```go
type UserTags struct {
    UserID string   `dynamorm:"pk"`
    Tags   []string `dynamorm:"set"`  // DynamoDB String Set
}

// Add tags atomically
err := db.Model(&UserTags{UserID: "user-123"}).
    UpdateBuilder().
    Add("Tags", []string{"premium", "verified"}).  // Add to set
    Execute()
```

### Remove from Set

```go
// Remove tags atomically
err := db.Model(&UserTags{UserID: "user-123"}).
    UpdateBuilder().
    Delete("Tags", []string{"trial"}).  // Remove from set
    Execute()
```

## Atomic List Operations

### Append to List

```go
type AuditLog struct {
    EntityID string   `dynamorm:"pk"`
    Events   []string `json:"events"`
}

// Append to end of list
err := db.Model(&AuditLog{EntityID: "order-456"}).
    UpdateBuilder().
    AppendToList("Events", []string{"shipped", "delivered"}).
    Execute()
```

### Prepend to List

```go
// Prepend to beginning of list
err := db.Model(&AuditLog{EntityID: "order-456"}).
    UpdateBuilder().
    PrependToList("Events", []string{"payment_received"}).
    Execute()
```

### Update List Element

```go
// Update specific element by index
err := db.Model(&AuditLog{EntityID: "order-456"}).
    UpdateBuilder().
    SetListElement("Events", 0, "payment_confirmed").
    Execute()

// Remove element at index
err := db.Model(&AuditLog{EntityID: "order-456"}).
    UpdateBuilder().
    RemoveFromListAt("Events", 2).
    Execute()
```

## Conditional Atomic Updates

### Optimistic Locking with Version

```go
type Document struct {
    ID      string `dynamorm:"pk"`
    Content string `json:"content"`
    Version int64  `dynamorm:"version"`
}

// Update only if version matches (prevents concurrent overwrites)
err := db.Model(&Document{ID: "doc-789"}).
    UpdateBuilder().
    Set("Content", "Updated content").
    ConditionVersion(5).  // Only update if current version is 5
    Execute()

if errors.Is(err, customerrors.ErrConditionFailed) {
    // Version mismatch - someone else updated the document
}
```

### Conditional Updates

```go
// Update only if conditions met
err := db.Model(&Product{SKU: "ABC123"}).
    UpdateBuilder().
    Add("ReservedQuantity", 5).
    Condition("AvailableQuantity", ">=", 5).  // Ensure sufficient stock
    Execute()

// Multiple conditions
err := db.Model(&Order{ID: "order-123"}).
    UpdateBuilder().
    Set("Status", "cancelled").
    Condition("Status", "=", "pending").      // Only if pending
    ConditionExists("PaymentID").             // And payment exists
    Execute()
```

### OR Conditions (New Feature)

DynamORM now supports OR logic in condition expressions, allowing for more complex business rules:

```go
// Simple OR condition
err := db.Model(&Item{ID: "item-123"}).
    UpdateBuilder().
    Set("ProcessedAt", time.Now()).
    Condition("Status", "=", "pending").      // if status = pending
    OrCondition("Priority", "=", "high").     // OR priority = high
    Execute()
```

#### Mixed AND/OR Conditions

```go
// Complex condition: (status = 'pending' AND region = 'US') OR priority = 'urgent'
err := db.Model(&Order{ID: "order-456"}).
    UpdateBuilder().
    Add("ProcessCount", 1).
    Condition("Status", "=", "pending").      // status = pending
    Condition("Region", "=", "US").           // AND region = US  
    OrCondition("Priority", "=", "urgent").   // OR priority = urgent
    Execute()
```

#### Rate Limiting with OR Conditions

```go
// Allow request if: under limit OR premium user OR whitelisted
func CheckRateLimitWithPrivileges(db core.DB, userID string, limit int64) (bool, int64, error) {
    rateLimit := &RateLimit{}
    rateLimit.SetKeys(userID, time.Now().UTC().Truncate(time.Hour))
    
    err := db.Model(rateLimit).
        UpdateBuilder().
        Add("RequestCount", 1).                           // Atomic increment
        SetIfNotExists("RequestCount", 1, 1).             // Initialize if new
        Condition("RequestCount", "<", limit).            // Under regular limit
        OrCondition("UserType", "=", "premium").          // OR is premium user
        OrCondition("Whitelisted", "=", true).            // OR is whitelisted
        ExecuteWithResult(rateLimit)
    
    if err != nil {
        if errors.Is(err, customerrors.ErrConditionFailed) {
            // Over limit and not premium/whitelisted
            return false, limit, nil
        }
        return false, 0, err
    }
    
    // Allowed - either under limit or has special access
    return true, rateLimit.RequestCount, nil
}
```

#### Multiple OR Conditions

```go
// Process if any status matches
err := db.Model(&Task{ID: "task-789"}).
    UpdateBuilder().
    Set("Status", "processing").
    Set("StartedAt", time.Now()).
    Condition("Status", "=", "new").          // status = new
    OrCondition("Status", "=", "retry").      // OR status = retry
    OrCondition("Status", "=", "failed").     // OR status = failed
    Execute()
```

#### OR with Attribute Existence

```go
// Update if field doesn't exist OR needs retry
err := db.Model(&Job{ID: "job-123"}).
    UpdateBuilder().
    Set("ProcessedBy", "worker-1").
    ConditionNotExists("ProcessedBy").        // if not already processed
    OrCondition("Status", "=", "failed").     // OR needs retry
    Execute()
```

#### Important Notes on Precedence

When mixing AND and OR conditions, operations are evaluated left-to-right:
- `A AND B OR C` evaluates as `(A AND B) OR C`
- `A OR B AND C` evaluates as `(A OR B) AND C`

For complex logic requiring different grouping, consider restructuring your conditions or using multiple operations.

## Complex Atomic Updates

### Multiple Operations in Single Request

```go
// Update multiple fields atomically
err := db.Model(&GameScore{PlayerID: "player-456"}).
    UpdateBuilder().
    Add("TotalScore", 100).           // Add to score
    Increment("GamesPlayed").         // Increment counter
    AppendToList("RecentScores", []int{100}).
    Set("LastPlayed", time.Now()).
    SetIfNotExists("HighScore", 100, 100).
    Execute()
```

### Return Updated Values

```go
var updated Product

err := db.Model(&Product{SKU: "ABC123"}).
    UpdateBuilder().
    Decrement("Quantity").
    Increment("SoldCount").
    Set("LastSold", time.Now()).
    ReturnValues("ALL_NEW").          // Return all attributes after update
    ExecuteWithResult(&updated)

fmt.Printf("New quantity: %d, Sold count: %d\n", 
    updated.Quantity, updated.SoldCount)
```

## Best Practices

### 1. Use Atomic Operations for Counters

❌ **Don't do this** (race condition):
```go
// Read
var counter Counter
db.Model(&Counter{ID: "123"}).First(&counter)

// Modify
counter.Value++

// Write
db.Model(&counter).Update()
```

✅ **Do this** (atomic):
```go
db.Model(&Counter{ID: "123"}).
    UpdateBuilder().
    Increment("Value").
    Execute()
```

### 2. Initialize Values with SetIfNotExists

```go
// Safe initialization + increment
err := db.Model(&Stats{ID: "daily-stats"}).
    UpdateBuilder().
    Add("PageViews", 1).
    SetIfNotExists("PageViews", 1, 1).       // Initialize to 1 if new
    SetIfNotExists("UniqueVisitors", 0, 0).  // Initialize other fields
    Execute()
```

### 3. Use Conditions for Business Logic

```go
// Atomic inventory reservation
func ReserveInventory(db core.DB, sku string, quantity int) error {
    return db.Model(&Inventory{SKU: sku}).
        UpdateBuilder().
        Add("Available", -quantity).          // Decrease available
        Add("Reserved", quantity).            // Increase reserved
        Condition("Available", ">=", quantity). // Ensure sufficient stock
        Execute()
}
```

### 4. Batch Atomic Updates

For updating multiple items atomically, use transactions:

```go
err := db.TransactionFunc(func(tx any) error {
    t := tx.(*transaction.Transaction)
    
    // Multiple atomic updates in transaction
    t.Update(&Account{ID: "from"}).
        Add("Balance", -100).
        Condition("Balance", ">=", 100)
    
    t.Update(&Account{ID: "to"}).
        Add("Balance", 100)
    
    return t.Commit()
})
```

## Performance Benefits

Atomic operations provide:
- **No race conditions**: Operations execute atomically at DynamoDB level
- **Better performance**: Single request instead of read-modify-write
- **Lower latency**: No round trips for reading current value
- **Reduced costs**: Fewer read/write operations

## Common Use Cases

1. **Rate Limiting**: Track API usage per time window
2. **Inventory Management**: Update stock levels atomically
3. **Analytics Counters**: Page views, clicks, conversions
4. **Financial Transactions**: Account balances, transaction counts
5. **Gaming**: Scores, achievements, player statistics
6. **Distributed Locks**: Atomic lock acquisition/release
7. **Audit Trails**: Append-only event logs

## Error Handling

```go
err := db.Model(&Counter{ID: "123"}).
    UpdateBuilder().
    Add("Value", 1).
    Condition("Value", "<", 1000).  // Max value 1000
    Execute()

switch {
case err == nil:
    // Success
case errors.Is(err, customerrors.ErrConditionFailed):
    // Condition not met (e.g., counter at max)
case errors.Is(err, customerrors.ErrItemNotFound):
    // Item doesn't exist
default:
    // Other error
}
```

## Summary

DynamORM's UpdateBuilder provides full access to DynamoDB's atomic operations:
- Use `Add()` for atomic increments/decrements
- Use `Increment()`/`Decrement()` for convenience
- Combine with conditions for business logic
- Return updated values with `ExecuteWithResult()`
- No race conditions or read-modify-write cycles

These operations are essential for building scalable, concurrent applications with DynamoDB. 
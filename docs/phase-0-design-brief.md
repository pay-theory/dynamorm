# Phase 0 Design Brief – Conditional & Transaction Enhancements

**Date:** November 9, 2025  
**Status:** Approved – Ready for Phase 1 Implementation  
**Authors:** DynamORM Development Team

---

## Executive Summary

This document presents the findings from the Phase 0 discovery audit and proposes API designs for three major capabilities:

1. **First-class conditional CRUD helpers** – Simplify conditional creates, updates, and deletes without dropping to raw SDK
2. **Composable TransactWriteItems utility** – Enable fluent transaction building for atomic multi-item operations
3. **Native BatchGetItem wrapper** – Provide ergonomic batch reads with automatic retry and chunking

All proposals prioritize:
- **ABI stability** – Extend existing interfaces without breaking changes
- **Fluent patterns** – Chainable builders matching existing DynamORM style
- **Error transparency** – Clear propagation of DynamoDB condition failures
- **Reuse** – Leverage existing `internal/expr.Builder` and `pkg/transaction` infrastructure

---

## Part 1: Discovery Audit Findings

### 1.1 Existing Condition Support (UpdateBuilder)

**Current Implementation:**
- `pkg/query/update_builder.go` implements `UpdateBuilder` with conditions:
  - `Condition(field, operator, value)` - Generic condition support
  - `ConditionExists(field)` - Requires attribute to exist
  - `ConditionNotExists(field)` - Requires attribute not to exist
  - `ConditionVersion(currentVersion)` - Optimistic locking
- Conditions are stored as `[]updateCondition` and translated to DynamoDB expressions in `Execute()`
- Uses `internal/expr.Builder` for secure expression generation

**Key Reuse Points:**
- `expr.Builder.AddConditionExpression(field, operator, value)` - Already handles condition building
- `expr.Builder.Build()` - Returns `ExpressionComponents` with `ConditionExpression`
- Secure validation via `pkg/validation` package
- Reserved word handling built into name escaping

**Gap Analysis:**
- Conditions only work on `UpdateBuilder`, not on `Create()` or `Delete()` operations
- No direct way to add conditions to `PutItem` or `DeleteItem` without custom code
- Query interface lacks conditional operation methods

### 1.2 Transaction Support

**Current Implementation:**
- `pkg/transaction/transaction.go` provides low-level transaction support:
  - `Create(model)` - Adds Put with `attribute_not_exists` condition
  - `Update(model)` - Adds Update with version checking if available
  - `Delete(model)` - Adds Delete with version checking if available
  - `Get(model, dest)` - Adds Get to transaction
  - `Commit()` - Executes via `TransactWriteItems` or `TransactGetItems`
- Handles optimistic locking with version fields automatically
- Error handling for `ConditionalCheckFailed` and `TransactionCanceled`

**Key Reuse Points:**
- `types.TransactWriteItem` building logic is solid
- Marshal/unmarshal via `pkgTypes.Converter` works well
- Primary key extraction from models is reusable

**Gap Analysis:**
- No fluent builder pattern - users call methods directly on transaction object
- Limited custom condition support (only version-based)
- No easy way to compose conditions per operation
- Transaction operations tied to full model marshaling (can't do partial updates)
- Missing retry logic for transient failures

### 1.3 Batch Operations

**Current Implementation:**
- `pkg/query/query.go` - `BatchGet(keys []any, dest any)` exists but:
  - Limited to 100 keys (enforced)
  - No automatic chunking for larger sets
  - Basic retry for `UnprocessedKeys` in executor
- `pkg/query/executor.go` - `ExecuteBatchGet()`:
  - Handles `UnprocessedKeys` with simple retry loop
  - No exponential backoff or jitter
  - No progress tracking or cancellation support
- Batch write operations (`BatchCreate`, `BatchWrite`) have better retry support in `batch_operations.go`

**Key Reuse Points:**
- `CompiledBatchGet` type captures necessary parameters
- `BatchExecutor` interface pattern is extensible
- Retry logic from batch writes can inform batch get improvements

**Gap Analysis:**
- No automatic chunking beyond 100 keys
- Retry strategy is basic (no backoff configuration)
- Missing metrics/observability hooks
- Cannot control consistency level per request (struct field exists but not exposed)

### 1.4 Core Interface Extension Points

**Analysis of `pkg/core/interfaces.go`:**

**Query Interface:**
- Already has `ConsistentRead()` for query operations
- `WithRetry(maxRetries, initialDelay)` exists for GSI queries
- Extension point: Add conditional methods here

**ExtendedDB Interface:**
- Includes `TransactionFunc(fn func(tx any) error)` but uses `any` type
- Could be extended with typed transaction builder

**UpdateBuilder Interface:**
- Already fluent with method chaining
- Has condition methods but they're scoped to updates only
- Pattern to follow for other conditional operations

**Recommendation:**
Extend `Query` interface with new conditional methods and add a new `TransactionBuilder` interface to `core` package.

---

## Part 2: API Design Proposals

### 2.1 Conditional Create/Update/Delete

**Design Principles:**
- Reuse `UpdateBuilder` patterns where possible
- Add methods to `Query` interface for conditional creates/deletes
- Leverage existing `expr.Builder` for condition compilation
- Return clear errors for condition failures

#### 2.1.1 Proposed API - Conditional Create

```go
// Add to Query interface
IfNotExists() Query  // Shorthand for create-only condition
WithCondition(field, operator string, value any) Query  // Generic condition
WithConditionExpression(expr string, values map[string]any) Query  // Advanced use

// Usage examples
err := db.Model(&user).
    IfNotExists().
    Create()
// Translates to: ConditionExpression = "attribute_not_exists(PK)"

err := db.Model(&bookmark).
    WithCondition("Status", "=", "active").
    Create()
// Translates to: ConditionExpression = "#status = :statusVal"
```

#### 2.1.2 Proposed API - Conditional Delete

```go
// Add to Query interface (already declared but enhance implementation)
// Methods cascade to Delete() operation

// Usage examples
err := db.Model(&user).
    Where("UserID", "=", "user123").
    WithCondition("Version", "=", 5).
    Delete()

err := db.Model(&session).
    Where("SessionID", "=", "sess_xyz").
    IfExists().  // New shorthand method
    Delete()
```

#### 2.1.3 Enhanced UpdateBuilder Conditions

```go
// Already exists, but document better patterns
updateBuilder := db.Model(&account).
    Where("AccountID", "=", "acc_123").
    UpdateBuilder()

err := updateBuilder.
    Set("Balance", 1000).
    ConditionVersion(currentVersion).  // Existing
    ConditionExists("Status").          // Existing
    Condition("Balance", ">=", 0).      // Existing - prevent negative
    Execute()
```

#### 2.1.4 Implementation Strategy

**Changes Required:**
1. Add to `pkg/core/interfaces.go`:
```go
type Query interface {
    // ... existing methods ...
    
    // IfNotExists adds attribute_not_exists condition for primary key
    IfNotExists() Query
    
    // IfExists adds attribute_exists condition for primary key  
    IfExists() Query
    
    // WithCondition adds a generic condition expression
    WithCondition(field, operator string, value any) Query
    
    // WithConditionExpression adds a raw condition expression
    WithConditionExpression(expr string, values map[string]any) Query
}
```

2. Extend `pkg/query/query.go`:
```go
type Query struct {
    // ... existing fields ...
    conditions []condition  // Store conditions
}

type condition struct {
    field    string
    operator string
    value    any
}

func (q *Query) IfNotExists() core.Query {
    q.conditions = append(q.conditions, condition{
        field:    q.metadata.PrimaryKey().PartitionKey,
        operator: "attribute_not_exists",
        value:    nil,
    })
    return q
}

func (q *Query) WithCondition(field, operator string, value any) core.Query {
    q.conditions = append(q.conditions, condition{
        field:    field,
        operator: operator,
        value:    value,
    })
    return q
}
```

3. Update `pkg/query/executor.go`:
   - Modify `ExecutePutItem()` to include conditions from query
   - Modify `ExecuteDeleteItem()` to include conditions from query
   - Use existing `expr.Builder.AddConditionExpression()` for compilation

**Backward Compatibility:**
- All new methods are additive to interfaces
- Existing code continues to work unchanged
- Conditions are optional (no breaking changes)

---

### 2.2 TransactWriteItems Builder API

**Design Principles:**
- Fluent builder pattern matching DynamORM style
- Support custom conditions per operation
- Handle up to 100 items (DynamoDB limit)
- Clear error messages with operation context
- Integrate with existing transaction package

#### 2.2.1 Proposed API

```go
// Add to pkg/core/interfaces.go
type TransactionBuilder interface {
    // Put adds a Put operation (create or replace)
    Put(model any, conditions ...TransactCondition) TransactionBuilder
    
    // Create adds a Put with attribute_not_exists condition
    Create(model any, conditions ...TransactCondition) TransactionBuilder
    
    // Update adds an Update operation
    Update(model any, fields []string, conditions ...TransactCondition) TransactionBuilder
    
    // UpdateWithBuilder allows complex updates
    UpdateWithBuilder(model any, updateFn func(UpdateBuilder) error, conditions ...TransactCondition) TransactionBuilder
    
    // Delete adds a Delete operation
    Delete(model any, conditions ...TransactCondition) TransactionBuilder
    
    // ConditionCheck adds a condition check without modification
    ConditionCheck(model any, conditions ...TransactCondition) TransactionBuilder
    
    // Execute commits the transaction
    Execute() error
    
    // ExecuteWithContext commits with cancellation support
    ExecuteWithContext(ctx context.Context) error
}

// TransactCondition represents a condition for a transactional operation
type TransactCondition struct {
    Field    string
    Operator string
    Value    any
}

// Helper constructors
func IfNotExists() TransactCondition
func IfExists() TransactCondition  
func AtVersion(version int64) TransactCondition
func Condition(field, operator string, value any) TransactCondition

// Add to ExtendedDB interface
Transact() TransactionBuilder
```

#### 2.2.2 Usage Examples

**Example 1: Bookmark Dual-Write**
```go
// Atomic bookmark creation with user bookmark count increment
err := db.Transact().
    Create(&bookmark, dynamorm.IfNotExists()).
    UpdateWithBuilder(&user, func(ub core.UpdateBuilder) error {
        return ub.Increment("BookmarkCount").Execute()
    }).
    Execute()

if errors.Is(err, dynamorm.ErrConditionFailed) {
    // Handle duplicate bookmark
}
```

**Example 2: Account Transfer**
```go
// Atomic money transfer between accounts
err := db.Transact().
    UpdateWithBuilder(&sourceAccount, func(ub core.UpdateBuilder) error {
        return ub.
            Add("Balance", -amount).
            Condition("Balance", ">=", amount).  // Ensure sufficient funds
            ConditionVersion(sourceVersion).
            Execute()
    }).
    UpdateWithBuilder(&destAccount, func(ub core.UpdateBuilder) error {
        return ub.
            Add("Balance", amount).
            ConditionVersion(destVersion).
            Execute()
    }).
    Execute()
```

**Example 3: Conditional Multi-Item Update**
```go
// Update order and inventory atomically
err := db.Transact().
    Update(&order, []string{"Status", "ProcessedAt"}, 
        dynamorm.Condition("Status", "=", "pending")).
    Update(&inventory, []string{"AvailableCount"},
        dynamorm.Condition("AvailableCount", ">=", order.Quantity)).
    Execute()
```

#### 2.2.3 Implementation Strategy

**New Package Structure:**
```
pkg/
  transaction/
    transaction.go     # Existing low-level implementation
    builder.go         # New fluent builder
    builder_test.go    # New tests
    conditions.go      # Condition helpers
```

**Key Components:**

1. **Builder Implementation** (`pkg/transaction/builder.go`):
```go
type TransactionBuilderImpl struct {
    db        *dynamorm.DB
    registry  *model.Registry
    converter *pkgTypes.Converter
    operations []transactOperation
    ctx        context.Context
}

type transactOperation struct {
    opType     string  // "put", "update", "delete", "check"
    model      any
    fields     []string
    conditions []core.TransactCondition
    updateFn   func(core.UpdateBuilder) error
}

func (tb *TransactionBuilderImpl) Create(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
    // Add attribute_not_exists to conditions
    allConditions := append([]core.TransactCondition{IfNotExists()}, conditions...)
    return tb.Put(model, allConditions...)
}

func (tb *TransactionBuilderImpl) Execute() error {
    return tb.ExecuteWithContext(tb.ctx)
}

func (tb *TransactionBuilderImpl) ExecuteWithContext(ctx context.Context) error {
    // Convert operations to types.TransactWriteItem
    // Reuse existing transaction.Transaction marshaling logic
    // Call TransactWriteItems
    // Parse errors and provide context
}
```

2. **Error Handling:**
```go
// pkg/errors/errors.go additions
type TransactionError struct {
    OperationIndex int
    Operation      string  // "Put", "Update", "Delete"
    Model          string  // Model type name
    Reason         string  // DynamoDB cancellation reason
    Err            error
}

func (e *TransactionError) Error() string
func (e *TransactionError) Unwrap() error
```

3. **Integration with DB:**
```go
// dynamorm.go
func (db *DB) Transact() core.TransactionBuilder {
    return transaction.NewBuilder(db, db.registry, db.converter)
}
```

**Retry Strategy:**
- Automatic retry for `TransactionCanceledException` with `ThrottlingError`
- No automatic retry for `ConditionalCheckFailedException` (business logic error)
- Exponential backoff: 100ms, 200ms, 400ms, 800ms (max 3 retries)
- Respect context cancellation

---

### 2.3 BatchGetItem Enhanced Wrapper

**Design Principles:**
- Accept more than 100 keys with automatic chunking
- Robust retry with exponential backoff
- Preserve result ordering when possible
- Support consistent reads and projections
- Observable (progress callbacks, metrics)

#### 2.3.1 Proposed API

```go
// Add to Query interface
BatchGetWithOptions(keys []any, dest any, opts *BatchGetOptions) error

// Options structure
type BatchGetOptions struct {
    // ChunkSize determines keys per request (max 100)
    ChunkSize int
    
    // ConsistentRead enables strong consistency
    ConsistentRead bool
    
    // Parallel enables concurrent chunk processing
    Parallel bool
    
    // MaxConcurrency limits parallel requests
    MaxConcurrency int
    
    // RetryPolicy controls retry behavior
    RetryPolicy *RetryPolicy
    
    // ProgressCallback is called after each chunk
    ProgressCallback func(retrieved, total int)
    
    // OnError handles errors for individual chunks
    OnError func(keys []any, err error) error
}

// Default options
func DefaultBatchGetOptions() *BatchGetOptions

// Builder pattern for complex cases
type BatchGetBuilder interface {
    Keys(keys []any) BatchGetBuilder
    ChunkSize(size int) BatchGetBuilder
    ConsistentRead() BatchGetBuilder
    Parallel(maxConcurrency int) BatchGetBuilder
    WithRetry(policy *RetryPolicy) BatchGetBuilder
    Select(fields ...string) BatchGetBuilder
    OnProgress(callback func(int, int)) BatchGetBuilder
    Execute(dest any) error
}

// Add builder method to Query
BatchGetBuilder() BatchGetBuilder
```

#### 2.3.2 Usage Examples

**Example 1: Simple Batch Get (Existing API Enhanced)**
```go
// Fetch up to 100 users by ID
var users []User
keys := []any{"user1", "user2", "user3", ...}  // Up to 100
err := db.Model(&User{}).BatchGet(keys, &users)
```

**Example 2: Large Batch with Chunking**
```go
// Fetch 500 users automatically chunked
var users []User
keys := make([]any, 500)  // 500 user IDs
// ... populate keys ...

opts := dynamorm.DefaultBatchGetOptions()
opts.ChunkSize = 100  // Explicit (default anyway)
opts.ProgressCallback = func(retrieved, total int) {
    log.Printf("Retrieved %d/%d items", retrieved, total)
}

err := db.Model(&User{}).BatchGetWithOptions(keys, &users, opts)
```

**Example 3: Parallel Batch Get**
```go
// Fetch 1000 items across 10 parallel requests
var items []Item
keys := make([]any, 1000)
// ... populate keys ...

opts := &dynamorm.BatchGetOptions{
    ChunkSize: 100,
    Parallel: true,
    MaxConcurrency: 10,
    RetryPolicy: &dynamorm.RetryPolicy{
        MaxRetries: 5,
        InitialDelay: 100 * time.Millisecond,
        MaxDelay: 5 * time.Second,
        BackoffFactor: 2.0,
    },
}

err := db.Model(&Item{}).BatchGetWithOptions(keys, &items, opts)
```

**Example 4: Fluent Builder Style**
```go
var orders []Order
err := db.Model(&Order{}).
    BatchGetBuilder().
    Keys(orderIDs).
    ConsistentRead().
    Select("OrderID", "Status", "Total").
    Parallel(5).
    OnProgress(func(retrieved, total int) {
        metrics.RecordBatchProgress(retrieved, total)
    }).
    Execute(&orders)
```

#### 2.3.3 Implementation Strategy

**Changes Required:**

1. **Enhance Query Interface** (`pkg/core/interfaces.go`):
```go
type Query interface {
    // ... existing methods ...
    
    BatchGetWithOptions(keys []any, dest any, opts *BatchGetOptions) error
    BatchGetBuilder() BatchGetBuilder
}

type BatchGetOptions struct {
    ChunkSize        int
    ConsistentRead   bool
    Parallel         bool
    MaxConcurrency   int
    RetryPolicy      *RetryPolicy
    ProgressCallback func(retrieved, total int)
    OnError          func(keys []any, err error) error
}

type BatchGetBuilder interface {
    Keys(keys []any) BatchGetBuilder
    ChunkSize(size int) BatchGetBuilder
    ConsistentRead() BatchGetBuilder
    Parallel(maxConcurrency int) BatchGetBuilder
    WithRetry(policy *RetryPolicy) BatchGetBuilder
    Select(fields ...string) BatchGetBuilder
    OnProgress(callback func(int, int)) BatchGetBuilder
    Execute(dest any) error
}
```

2. **Update Query Implementation** (`pkg/query/query.go`):
```go
func (q *Query) BatchGetWithOptions(keys []any, dest any, opts *BatchGetOptions) error {
    if opts == nil {
        opts = DefaultBatchGetOptions()
    }
    
    // Chunk keys
    chunks := chunkKeys(keys, opts.ChunkSize)
    
    if opts.Parallel {
        return q.executeBatchGetParallel(chunks, dest, opts)
    }
    return q.executeBatchGetSequential(chunks, dest, opts)
}

func (q *Query) BatchGetBuilder() core.BatchGetBuilder {
    return &batchGetBuilderImpl{
        query: q,
        opts:  DefaultBatchGetOptions(),
    }
}
```

3. **New Builder Implementation** (`pkg/query/batch_get_builder.go`):
```go
type batchGetBuilderImpl struct {
    query      *Query
    keys       []any
    opts       *core.BatchGetOptions
    projection []string
}

func (b *batchGetBuilderImpl) Keys(keys []any) core.BatchGetBuilder {
    b.keys = keys
    return b
}

func (b *batchGetBuilderImpl) ConsistentRead() core.BatchGetBuilder {
    b.opts.ConsistentRead = true
    return b
}

func (b *batchGetBuilderImpl) Execute(dest any) error {
    // Apply projection if specified
    if len(b.projection) > 0 {
        b.query = b.query.Select(b.projection...).(*Query)
    }
    return b.query.BatchGetWithOptions(b.keys, dest, b.opts)
}
```

4. **Enhanced Executor** (`pkg/query/executor.go`):
```go
func (e *MainExecutor) ExecuteBatchGetWithRetry(
    input *CompiledBatchGet,
    dest any,
    retryPolicy *core.RetryPolicy,
) error {
    var allItems []map[string]types.AttributeValue
    unprocessedKeys := input.Keys
    attempt := 0
    
    for len(unprocessedKeys) > 0 && attempt <= retryPolicy.MaxRetries {
        // Build request
        keysAndAttributes := &types.KeysAndAttributes{
            Keys: unprocessedKeys,
            ConsistentRead: &input.ConsistentRead,
        }
        if input.ProjectionExpression != "" {
            keysAndAttributes.ProjectionExpression = &input.ProjectionExpression
            keysAndAttributes.ExpressionAttributeNames = input.ExpressionAttributeNames
        }
        
        batchGetInput := &dynamodb.BatchGetItemInput{
            RequestItems: map[string]types.KeysAndAttributes{
                input.TableName: *keysAndAttributes,
            },
        }
        
        // Execute
        output, err := e.client.BatchGetItem(e.ctx, batchGetInput)
        if err != nil {
            return fmt.Errorf("batch get failed: %w", err)
        }
        
        // Collect items
        if items, exists := output.Responses[input.TableName]; exists {
            allItems = append(allItems, items...)
        }
        
        // Check for unprocessed keys
        if len(output.UnprocessedKeys) == 0 {
            break
        }
        
        // Extract unprocessed keys for retry
        if keysAttr, exists := output.UnprocessedKeys[input.TableName]; exists {
            unprocessedKeys = keysAttr.Keys
        } else {
            break
        }
        
        // Exponential backoff
        if len(unprocessedKeys) > 0 && attempt < retryPolicy.MaxRetries {
            delay := calculateBackoff(attempt, retryPolicy)
            time.Sleep(delay)
            attempt++
        }
    }
    
    // Unmarshal results
    return UnmarshalItems(allItems, dest)
}

func calculateBackoff(attempt int, policy *core.RetryPolicy) time.Duration {
    delay := policy.InitialDelay * time.Duration(math.Pow(policy.BackoffFactor, float64(attempt)))
    if delay > policy.MaxDelay {
        delay = policy.MaxDelay
    }
    // Add jitter (±25%)
    jitter := time.Duration(rand.Float64()*0.5*float64(delay)) - delay/4
    return delay + jitter
}
```

**Parallel Execution:**
```go
func (q *Query) executeBatchGetParallel(
    chunks [][]any,
    dest any,
    opts *BatchGetOptions,
) error {
    var wg sync.WaitGroup
    semaphore := make(chan struct{}, opts.MaxConcurrency)
    resultsChan := make(chan []map[string]types.AttributeValue, len(chunks))
    errorsChan := make(chan error, len(chunks))
    
    for i, chunk := range chunks {
        wg.Add(1)
        go func(chunkIdx int, keys []any) {
            defer wg.Done()
            semaphore <- struct{}{}        // Acquire
            defer func() { <-semaphore }() // Release
            
            // Execute chunk
            var chunkResults []map[string]types.AttributeValue
            err := q.executeSingleBatchGet(keys, &chunkResults, opts)
            
            if err != nil {
                if opts.OnError != nil {
                    if handlerErr := opts.OnError(keys, err); handlerErr != nil {
                        errorsChan <- handlerErr
                        return
                    }
                } else {
                    errorsChan <- err
                    return
                }
            }
            
            resultsChan <- chunkResults
            
            if opts.ProgressCallback != nil {
                opts.ProgressCallback(len(chunkResults), len(allKeys))
            }
        }(i, chunk)
    }
    
    wg.Wait()
    close(resultsChan)
    close(errorsChan)
    
    // Check for errors
    select {
    case err := <-errorsChan:
        return err
    default:
    }
    
    // Merge results
    var allItems []map[string]types.AttributeValue
    for items := range resultsChan {
        allItems = append(allItems, items...)
    }
    
    return UnmarshalItems(allItems, dest)
}
```

---

## Part 3: Acceptance Test Criteria

### 3.1 Conditional Write Tests

**Test: Conditional Create - Success**
```go
Given a user model with ID "user123" does not exist
When Create() is called with IfNotExists()
Then the user is created successfully
```

**Test: Conditional Create - Failure**
```go
Given a user model with ID "user123" already exists
When Create() is called with IfNotExists()
Then ErrConditionFailed is returned
And the existing user is unchanged
```

**Test: Conditional Update - Version Check**
```go
Given an account with Version=5 exists
When UpdateBuilder().ConditionVersion(5).Set("Balance", 1000).Execute()
Then the update succeeds and Version becomes 6
```

**Test: Conditional Update - Version Mismatch**
```go
Given an account with Version=6 exists
When UpdateBuilder().ConditionVersion(5).Set("Balance", 1000).Execute()
Then ErrConditionFailed is returned
And the account remains at Version=6 with original balance
```

**Test: Conditional Delete - With Field Check**
```go
Given a session with Status="expired" exists
When Delete() is called with WithCondition("Status", "=", "expired")
Then the session is deleted successfully
```

**Test: Conditional Delete - Field Mismatch**
```go
Given a session with Status="active" exists
When Delete() is called with WithCondition("Status", "=", "expired")
Then ErrConditionFailed is returned
And the session remains active
```

**Test: Complex Condition - Multiple Fields**
```go
Given an account with Balance=100, Status="active" exists
When UpdateBuilder().
    Condition("Balance", ">=", 50).
    Condition("Status", "=", "active").
    Add("Balance", -50).
    Execute()
Then the update succeeds and Balance becomes 50
```

### 3.2 Transaction Tests

**Test: Dual-Write Transaction - Success**
```go
Given user "user123" exists with BookmarkCount=10
And bookmark "bm_123" does not exist
When Transact().
    Create(&bookmark, IfNotExists()).
    UpdateWithBuilder(&user, func(ub) { ub.Increment("BookmarkCount") }).
    Execute()
Then bookmark "bm_123" is created
And user "user123" has BookmarkCount=11
```

**Test: Dual-Write Transaction - Condition Failed**
```go
Given bookmark "bm_123" already exists
And user "user123" exists with BookmarkCount=10
When Transact().
    Create(&bookmark, IfNotExists()).
    UpdateWithBuilder(&user, func(ub) { ub.Increment("BookmarkCount") }).
    Execute()
Then ErrConditionFailed is returned
And bookmark remains unchanged
And user BookmarkCount remains 10 (transaction rolled back)
```

**Test: Account Transfer - Insufficient Funds**
```go
Given source account with Balance=50
And destination account with Balance=100
When Transact().
    UpdateWithBuilder(&source, func(ub) {
        ub.Add("Balance", -100).Condition("Balance", ">=", 100)
    }).
    UpdateWithBuilder(&dest, func(ub) { ub.Add("Balance", 100) }).
    Execute()
Then ErrConditionFailed is returned
And source Balance remains 50
And destination Balance remains 100
```

**Test: Transaction with Condition Check**
```go
Given order "ord_123" with Status="pending"
And inventory item with AvailableCount=5
When Transact().
    ConditionCheck(&order, Condition("Status", "=", "pending")).
    Update(&inventory, []string{"AvailableCount"},
        Condition("AvailableCount", ">=", 3)).
    Execute()
Then both operations succeed atomically
```

**Test: Transaction Cancellation Context**
```go
Given a transaction with 3 operations
And a context with 100ms timeout
When Execute() takes longer than 100ms
Then context.DeadlineExceeded error is returned
And no operations are committed
```

### 3.3 Batch Get Tests

**Test: Batch Get - Under 100 Keys**
```go
Given 50 users exist in DynamoDB
When BatchGet() is called with 50 user IDs
Then all 50 users are retrieved in one request
And results are unmarshaled correctly
```

**Test: Batch Get - Exactly 100 Keys**
```go
Given 100 users exist
When BatchGet() is called with 100 user IDs
Then all 100 users are retrieved
And only one DynamoDB BatchGetItem call is made
```

**Test: Batch Get - Over 100 Keys (Chunking)**
```go
Given 250 users exist
When BatchGetWithOptions() is called with 250 IDs and ChunkSize=100
Then 3 BatchGetItem requests are made (100, 100, 50)
And all 250 users are retrieved correctly
And progress callback is called after each chunk
```

**Test: Batch Get - Unprocessed Keys Retry**
```go
Given a BatchGetItem request returns 80 items and 20 unprocessed keys
When ExecuteBatchGetWithRetry() is called
Then the 20 unprocessed keys are retried
And exponential backoff is applied between attempts
And all 100 items are eventually retrieved
```

**Test: Batch Get - Parallel Execution**
```go
Given 500 users exist
When BatchGetWithOptions() is called with Parallel=true and MaxConcurrency=5
Then up to 5 concurrent BatchGetItem requests are made
And all 500 users are retrieved
And results are merged correctly
```

**Test: Batch Get - Consistent Read**
```go
Given ConsistentRead=true in options
When BatchGet() is called
Then ConsistentRead flag is set in DynamoDB request
```

**Test: Batch Get - Projection**
```go
Given projection is set to ["UserID", "Email"]
When BatchGetBuilder().Select("UserID", "Email").Execute()
Then only UserID and Email fields are retrieved
And ProjectionExpression is correctly set
```

**Test: Batch Get - Partial Failure Handling**
```go
Given OnError handler is provided
And one chunk fails with throttling error
When BatchGetWithOptions() is called
Then OnError handler is invoked for failed chunk
And error handling logic can retry or skip
```

**Test: Batch Get - Context Cancellation**
```go
Given a batch get operation with 10 chunks
And context is cancelled after 3 chunks complete
When ExecuteWithContext() is called
Then operation stops after current chunk completes
And context.Canceled error is returned
```

### 3.4 Integration Test Scenarios

**Scenario: Bookmark Service Full Flow**
```go
// Setup
user := &User{UserID: "u123", BookmarkCount: 0}
db.Model(user).Create()

// Test 1: Create first bookmark
bookmark1 := &Bookmark{BookmarkID: "bm1", UserID: "u123"}
err := db.Transact().
    Create(bookmark1, dynamorm.IfNotExists()).
    UpdateWithBuilder(user, func(ub) { ub.Increment("BookmarkCount") }).
    Execute()

assert.NoError(err)
// Verify: bookmark exists, user.BookmarkCount == 1

// Test 2: Attempt duplicate
err = db.Transact().
    Create(bookmark1, dynamorm.IfNotExists()).
    UpdateWithBuilder(user, func(ub) { ub.Increment("BookmarkCount") }).
    Execute()

assert.ErrorIs(err, dynamorm.ErrConditionFailed)
// Verify: user.BookmarkCount still == 1 (no increment)

// Test 3: Fetch bookmarks in batch
var bookmarks []Bookmark
err = db.Model(&Bookmark{}).
    Where("UserID", "=", "u123").
    All(&bookmarks)

assert.NoError(err)
assert.Len(bookmarks, 1)
```

---

## Part 4: Success Metrics

### 4.1 API Usability Metrics
- **Developer Experience:** API should reduce conditional write code from 15+ lines to 3-5 lines
- **Error Clarity:** Condition failures should include field/operator/expected value in error message
- **Discoverability:** IDE autocomplete should surface conditional methods naturally

### 4.2 Performance Metrics
- **Batch Get Throughput:** 10,000 items retrieved in <2 seconds with parallel=true
- **Transaction Latency:** <50ms overhead vs raw SDK `TransactWriteItems`
- **Retry Efficiency:** <3 retries needed on average for throttled operations

### 4.3 Reliability Metrics
- **Condition Accuracy:** 100% of condition failures correctly identified and propagated
- **Transaction Atomicity:** 100% rollback on any operation failure (verified via read-after-write)
- **Batch Completeness:** 100% of unprocessed keys eventually retrieved within retry limit

### 4.4 Testing Metrics
- **Unit Test Coverage:** >95% for new code paths
- **Integration Test Coverage:** All acceptance criteria automated
- **Stress Test Results:** 1M operations without memory leaks or crashes

---

## Part 5: Risk Assessment & Mitigation

### 5.1 ABI Stability Risks

**Risk:** New interface methods could conflict with existing implementations  
**Mitigation:** 
- Use additive changes only (no method signature changes)
- Provide default implementations where possible
- Run full test suite against all examples

### 5.2 Performance Risks

**Risk:** Parallel batch get could overwhelm DynamoDB with throttling  
**Mitigation:**
- Default to sequential execution (opt-in parallel)
- Configurable concurrency limits
- Built-in exponential backoff

**Risk:** Transaction builder adds overhead vs raw SDK  
**Mitigation:**
- Reuse existing marshaling code
- Minimize allocations in hot paths
- Benchmark before/after

### 5.3 Error Handling Risks

**Risk:** Transaction errors may not clearly indicate which operation failed  
**Mitigation:**
- Parse DynamoDB cancellation reasons
- Create structured `TransactionError` with operation context
- Include operation index and model type in error

**Risk:** Batch get partial failures could lose data  
**Mitigation:**
- Provide `OnError` callback for custom handling
- Log all unprocessed keys before giving up
- Document retry limits clearly

### 5.4 Migration Risks

**Risk:** Existing transaction code might break with new builder  
**Mitigation:**
- Keep existing `pkg/transaction.Transaction` unchanged
- Add new builder as separate type
- Provide migration guide in CHANGELOG

---

## Part 6: Open Questions for Review

1. **Condition DSL Scope:** Should we support OR logic in conditions, or keep AND-only for Phase 1?
   - **Recommendation:** AND-only for Phase 1 (matches DynamoDB's ConditionExpression simplicity)

2. **Transaction Builder Naming:** `db.Transact()` vs `db.TransactionBuilder()` vs `db.BeginTransaction()`?
   - **Recommendation:** `db.Transact()` for brevity and fluency

3. **Batch Get Chunking:** Should chunking be automatic (always) or opt-in via options?
   - **Recommendation:** Automatic beyond 100 keys, configurable chunk size

4. **Error Types:** Should `ErrConditionFailed` include field-level details or stay generic?
   - **Recommendation:** Generic `ErrConditionFailed` + structured error type with details

5. **Retry Defaults:** What should default retry policy be? Conservative (3 retries) or aggressive (10)?
   - **Recommendation:** 3 retries default, user-configurable

---

## Part 7: Next Steps (Phase 1 Readiness)

### 7.1 Pre-Implementation Checklist
- [ ] Circulate this design brief for team review
- [ ] Address open questions in team sync
- [ ] Finalize interface signatures in `pkg/core/interfaces.go`
- [ ] Create tracking issues for each capability
- [ ] Set up feature branch: `feature/conditional-enhancements`

### 7.2 Phase 1 Kickoff Requirements
- [ ] Design brief approved by maintainers
- [ ] Acceptance tests documented and agreed upon
- [ ] Performance baseline captured (current batch/transaction benchmarks)
- [ ] Mock implementation of new interfaces for testing
- [ ] Documentation stubs created in `docs/` for each feature

### 7.3 Communication Plan
- [ ] Update CHANGELOG.md with "Upcoming in v0.X.0" section
- [ ] Create RFC discussion in GitHub Discussions
- [ ] Notify users via Discord/Slack (if applicable)
- [ ] Prepare blog post draft for release announcement

---

## Appendices

### Appendix A: Expression Builder Reuse Pattern

The `internal/expr.Builder` is the foundation for all condition handling:

```go
// Example: Building a conditional put
builder := expr.NewBuilder()

// Add condition
builder.AddConditionExpression("status", "=", "active")
builder.AddConditionExpression("version", "=", 5)

// Compile
components := builder.Build()

// Use in DynamoDB call
putInput := &dynamodb.PutItemInput{
    TableName: aws.String("users"),
    Item: item,
    ConditionExpression: aws.String(components.ConditionExpression),
    ExpressionAttributeNames: components.ExpressionAttributeNames,
    ExpressionAttributeValues: components.ExpressionAttributeValues,
}
```

**Benefits:**
- Automatic reserved word escaping
- Type-safe value conversion
- Reusable across all operation types

### Appendix B: Transaction Item Limit Handling

DynamoDB enforces a 100-item limit per `TransactWriteItems`. Proposed handling:

1. **Phase 1:** Error if >100 operations added to transaction
2. **Phase 2 (Future):** Auto-split into multiple transactions with dependency graph

**Rationale:** Auto-splitting is complex (requires topological sort for dependencies). Phase 1 focuses on single-transaction use cases (<100 items), which covers 95% of real-world scenarios.

### Appendix C: Comparison with Other ORMs

**GORM (SQL):**
```go
db.Where("age > ?", 18).First(&user)  // Conditions in query
db.Transaction(func(tx *gorm.DB) error { ... })  // Transaction wrapper
```

**AWS DynamoDB SDK (Raw):**
```go
putInput := &dynamodb.PutItemInput{
    ConditionExpression: aws.String("attribute_not_exists(PK)"),
    ExpressionAttributeNames: map[string]string{"#pk": "PK"},
}
transactInput := &dynamodb.TransactWriteItemsInput{
    TransactItems: []types.TransactWriteItem{ ... },
}
```

**DynamORM (Proposed):**
```go
db.Model(&user).IfNotExists().Create()  // Fluent conditions
db.Transact().Create(&user).Update(&account).Execute()  // Fluent transactions
```

**Analysis:** DynamORM's approach is more fluent than raw SDK while maintaining type safety better than GORM.

---

## Conclusion

This design brief proposes a cohesive enhancement to DynamORM's conditional and transactional capabilities that:

1. **Builds on existing patterns** – Reuses `UpdateBuilder` fluency, `expr.Builder` infrastructure, and transaction marshaling
2. **Maintains ABI stability** – All changes are additive to interfaces
3. **Improves developer experience** – Reduces boilerplate and clarifies error handling
4. **Enables advanced workflows** – Atomic multi-item operations with custom conditions

**Recommendation:** Proceed to Phase 1 with conditional CRUD helpers as the foundation, followed by transaction builder and batch get enhancements in subsequent phases.

**Timeline Estimate:**
- Phase 0 (this brief): ✅ Complete
- Phase 1 (Conditional CRUD): 1 sprint
- Phase 2 (Transaction Builder): 1 sprint  
- Phase 3 (Batch Get Enhancements): <1 sprint
- Phase 4 (Documentation & Release): 2-3 days

**Total:** ~3-4 sprints end-to-end

---

**Approval Signatures:**

- [ ] Lead Architect: _____________________ Date: _______
- [ ] Backend Team Lead: _________________ Date: _______
- [ ] QA Lead: ____________________________ Date: _______

**Revision History:**
- v1.0 (2025-11-09): Initial draft based on Phase 0 discovery

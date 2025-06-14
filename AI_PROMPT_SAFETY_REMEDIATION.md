# AI Assistant Prompt: DynamORM Safety Remediation

## Context
You are a senior Go developer tasked with fixing critical safety and stability issues in the DynamORM library. Your focus is on eliminating application crashes, improving error handling, and ensuring robust operation under all conditions.

## Your Mission
Fix the following critical safety issues in order of priority:

### PRIMARY TARGETS (Week 1 - CRITICAL)

#### 1. ELIMINATE ALL PANIC STATEMENTS
**Objective:** Remove all `panic()` calls from library code and replace with proper error returns

**Target File:** `pkg/session/session.go`
```go
// CURRENT CODE (DANGEROUS):
func (s *Session) Client() *dynamodb.Client {
    if s == nil {
        panic("session is nil")
    }
    if s.client == nil {
        panic("DynamoDB client is nil")
    }
    return s.client
}

// YOUR TASK: Replace with error returns
func (s *Session) Client() (*dynamodb.Client, error) {
    // Implement safe error handling
}
```

**Action Items:**
1. Change `Client()` method signature to return `(*dynamodb.Client, error)`
2. Find ALL callers of `session.Client()` (approximately 47 locations)
3. Update each caller to handle the error properly
4. Add comprehensive error handling tests

**Files to Modify:**
- `pkg/session/session.go`
- `dynamorm.go` (17+ locations)
- `multiaccount.go` (3+ locations)
- `lambda.go` (2+ locations)

#### 2. ADD GOROUTINE PANIC RECOVERY
**Objective:** Prevent goroutine panics from crashing the application

**Target Location:** `dynamorm.go:2715-2780` (parallel scan operations)

**Current Vulnerable Code:**
```go
go func(seg int32) {
    items, err := q.executeScanSegment(metadata, seg, totalSegments)
    results <- segmentResult{items: items, err: err}
}(int32(i))
```

**Your Task:** Add panic recovery wrapper:
```go
go func(seg int32) {
    defer func() {
        if r := recover(); r != nil {
            // Implement safe recovery with logging
            results <- segmentResult{
                err: fmt.Errorf("scan segment %d panicked: %v", seg, r),
            }
        }
    }()
    
    items, err := q.executeScanSegment(metadata, seg, totalSegments)
    results <- segmentResult{items: items, err: err}
}(int32(i))
```

**Requirements:**
- Add recovery to ALL goroutines in the codebase
- Implement structured logging for panic events
- Ensure panic recovery doesn't mask legitimate errors
- Add tests that verify panic recovery works

#### 3. FIX UNHANDLED ERROR PATTERNS
**Objective:** Eliminate the dangerous `_, err := func()` pattern where errors are ignored

**Critical Locations:**
```go
// pkg/transaction/transaction.go:141, 146, 168, 226
av, _ := tx.converter.ToAttributeValue(currentVersion)  // DANGEROUS

// pkg/schema/manager.go:437
desiredGSIs, _ := m.buildIndexes(metadata)  // DANGEROUS

// lambda.go:402, 412
cfg, _ := config.LoadDefaultConfig(context.Background())  // DANGEROUS
```

**Your Task:**
1. Search for ALL instances of `_, err :=` or `_, _ :=` patterns
2. Replace with proper error handling:
```go
// GOOD PATTERN:
av, err := tx.converter.ToAttributeValue(currentVersion)
if err != nil {
    return fmt.Errorf("converting attribute value: %w", err)
}
```

**Standards to Follow:**
- Always check errors
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Never ignore errors with `_` unless absolutely necessary and documented
- Add error handling tests for all modified functions

### SECONDARY TARGETS (Week 2-3)

#### 4. FIX RACE CONDITIONS
**Target:** `dynamorm.go:283-301` - `WithLambdaTimeoutBuffer` method

**Issue:** Modifying shared state instead of creating new instance
```go
// CURRENT DANGEROUS CODE:
func (db *DB) WithLambdaTimeoutBuffer(buffer time.Duration) core.DB {
    db.mu.Lock()
    defer db.mu.Unlock()
    db.lambdaTimeoutBuffer = buffer
    return db  // Returns same instance - RACE CONDITION!
}
```

**Your Task:** Create new instance instead of modifying existing:
```go
func (db *DB) WithLambdaTimeoutBuffer(buffer time.Duration) core.DB {
    // Create new instance safely
    // Copy all fields properly
    // Ensure thread safety
}
```

#### 5. IMPROVE CONTEXT TIMEOUT HANDLING
**Objective:** Ensure all operations properly respect context cancellation

**Areas to Fix:**
- DynamoDB operations that don't check context
- Long-running operations without timeout enforcement
- Goroutines that don't handle context cancellation

**Implementation:**
```go
// Add context checks in loops and long operations
select {
case <-ctx.Done():
    return ctx.Err()
default:
    // Continue operation
}
```

## Implementation Guidelines

### Error Handling Standards
```go
// GOOD: Contextual error wrapping
func (db *DB) someOperation() error {
    result, err := db.session.Client()
    if err != nil {
        return fmt.Errorf("getting client for operation: %w", err)
    }
    // ... rest of function
}

// BAD: Ignoring errors
func (db *DB) someOperation() error {
    result, _ := db.session.Client()  // DON'T DO THIS
    // ... rest of function
}
```

### Panic Recovery Pattern
```go
// Standard panic recovery wrapper
defer func() {
    if r := recover(); r != nil {
        // Log the panic with context
        log.Error("Panic recovered", 
            "panic", r,
            "stack", string(debug.Stack()),
            "operation", "operation_name")
        
        // Convert to error if needed
        if errChan != nil {
            errChan <- fmt.Errorf("operation panicked: %v", r)
        }
    }
}()
```

### Testing Requirements
For each fix, create tests that verify:
1. **Happy Path:** Normal operation works correctly
2. **Error Conditions:** Errors are properly handled and propagated
3. **Edge Cases:** Nil pointers, empty values, etc.
4. **Concurrent Access:** Thread safety where applicable
5. **Panic Recovery:** Panics are caught and converted to errors

## Success Criteria
- [ ] Zero `panic()` statements in library code
- [ ] All goroutines have panic recovery
- [ ] 100% of errors are handled (no `_, err :=` patterns)
- [ ] All race conditions eliminated
- [ ] Context cancellation properly respected
- [ ] Comprehensive test coverage for all fixes
- [ ] No performance regression >10%

## Tools and Techniques
1. **Search Patterns:** Use `grep -r "panic\|_, err :=\|_, _ :=" .` to find issues
2. **Race Detection:** Run tests with `go test -race`
3. **Static Analysis:** Use `go vet` and `golangci-lint`
4. **Error Wrapping:** Always use `fmt.Errorf("context: %w", err)`

## Deliverables
1. **Modified source files** with all safety issues fixed
2. **Comprehensive test suite** covering all error conditions
3. **Documentation** of error handling patterns
4. **Performance benchmarks** showing no significant regression
5. **Migration guide** for users (if API changes are needed)

## Important Notes
- **Library Code Must Never Panic:** This is the #1 rule
- **Preserve API Compatibility:** Where possible, maintain existing interfaces
- **Document Breaking Changes:** If API changes are needed, document them clearly
- **Test Everything:** Safety fixes must be thoroughly tested
- **Focus on Robustness:** Better to be slow and safe than fast and crashy

## Questions to Ask Yourself
1. "Can this function ever panic?"
2. "What happens if this error is ignored?"
3. "Is this operation thread-safe?"
4. "Does this respect context cancellation?"
5. "How can I test this failure mode?"

---

**Priority:** CRITICAL - Week 1 deliverable
**Success Metric:** Zero production crashes from DynamORM
**Testing:** Must pass with `go test -race` and comprehensive error injection tests 
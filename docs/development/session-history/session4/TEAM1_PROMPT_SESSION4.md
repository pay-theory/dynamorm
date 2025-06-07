# Team 1 Prompt - Session 4: Lambda Core Implementation

## Context
You are Team 1 working on DynamORM, a Go ORM for DynamoDB. The core functionality is complete and working. Your task in Session 4 is to implement Lambda-specific optimizations to support Pay Theory's serverless architecture.

## Your Mission
Implement core Lambda support including:
1. Lambda-optimized DB wrapper
2. Multi-account support with AssumeRole
3. Context-aware timeout handling
4. Connection reuse patterns

## Key Requirements

### 1. Create `lambda.go`
```go
package dynamorm

// LambdaDB should:
// - Wrap the standard DB with Lambda optimizations
// - Support connection reuse across invocations
// - Enable model pre-registration
// - Provide Lambda-aware configuration
// - Detect Lambda environment automatically
```

### 2. Create `multiaccount.go`
```go
package dynamorm

// MultiAccountDB should:
// - Manage connections to multiple AWS accounts
// - Cache connections per account
// - Support AssumeRole with external ID
// - Propagate partner context through queries
// - Handle credential refresh automatically
```

### 3. Update `dynamorm.go`
Add Lambda-specific features:
- `WithLambdaTimeout(ctx)` - Respect Lambda execution limits
- `lambdaDeadline` field - Track timeout
- Early termination before Lambda timeout
- Context propagation through all operations

### 4. Lambda Helpers
Create utility functions:
- `IsLambdaEnvironment()` - Detect Lambda runtime
- `GetLambdaMemoryMB()` - Get allocated memory
- `EnableXRayTracing()` - X-Ray integration
- `OptimizeForMemory(mb)` - Adjust behavior based on memory

## Technical Specifications

### Connection Optimization
```go
// Reuse connections across warm invocations
var (
    globalDB *LambdaDB
    once     sync.Once
)

func init() {
    once.Do(func() {
        globalDB = initializeLambdaDB()
    })
}
```

### Multi-Account Pattern
```go
type AccountConfig struct {
    RoleARN    string
    ExternalID string  
    Region     string
}

// Cache assumed role credentials
cache sync.Map // partnerID -> *LambdaDB
```

### Timeout Handling
```go
// Leave 1 second buffer for cleanup
adjustedDeadline := lambdaDeadline.Add(-1 * time.Second)

// Check before each operation
if time.Until(adjustedDeadline) <= 0 {
    return ErrLambdaTimeout
}
```

## Constraints
1. Maintain backward compatibility with non-Lambda usage
2. Keep Lambda features optional (don't force Lambda dependencies)
3. Minimize cold start impact
4. Support concurrent Lambda executions
5. Handle credential expiration gracefully

## Deliverables
1. Working `lambda.go` with tests
2. Working `multiaccount.go` with tests
3. Updated `dynamorm.go` with Lambda support
4. Lambda example in `examples/lambda/`
5. Benchmark showing cold start improvements

## Testing Requirements
- Unit tests for all new functions
- Integration test with real Lambda environment
- Multi-account flow test
- Timeout handling test
- Cold start benchmark

## Files You'll Need to Read
- `dynamorm.go` - Main DB implementation
- `pkg/session/session.go` - Session management
- `pkg/core/interfaces.go` - Core interfaces
- `LAMBDA_IMPLEMENTATION_GUIDE.md` - Implementation details

## Success Criteria
- [ ] Lambda cold start < 100ms
- [ ] Multi-account switching works
- [ ] Timeout handling prevents Lambda crashes
- [ ] All existing tests still pass
- [ ] New Lambda tests pass

Remember: Focus on making DynamORM "Lambda-native" not just "Lambda-compatible"! 
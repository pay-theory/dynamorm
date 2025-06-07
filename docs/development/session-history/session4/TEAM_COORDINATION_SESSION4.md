# Team Coordination Guide - Session 4: Lambda Optimizations

## Overview
Session 4 focuses on implementing Lambda-specific optimizations and payment features. Both teams need to coordinate closely as Team 2's examples depend on Team 1's core Lambda implementation.

## Team Responsibilities

### Team 1: Core Lambda Infrastructure
- `lambda.go` - Lambda-optimized DB wrapper
- `multiaccount.go` - Multi-account support
- Updates to `dynamorm.go` for timeout handling
- Lambda detection and optimization utilities

### Team 2: Payment Features & Examples
- Payment models with all features
- Lambda handler implementations
- Helper utilities (idempotency, audit, cost)
- Integration tests and benchmarks

## Coordination Points

### 1. Shared Interfaces

#### Lambda DB Interface (Team 1 defines, Team 2 uses)
```go
type LambdaDB interface {
    *DB
    PreRegisterModels(models ...interface{}) error
    WithLambdaTimeout(ctx context.Context) *LambdaDB
    IsWarmStart() bool
}
```

#### Multi-Account Interface (Team 1 defines, Team 2 uses)
```go
type MultiAccountDB interface {
    Partner(partnerID string) (*LambdaDB, error)
    ListPartners() []string
    InvalidatePartner(partnerID string)
}
```

### 2. Model Registration (Both teams)

Team 1 provides:
```go
// In lambda.go
func (ldb *LambdaDB) PreRegisterModels(models ...interface{}) error
```

Team 2 uses:
```go
// In examples/payment/init.go
func init() {
    db.PreRegisterModels(
        &Payment{},
        &Transaction{},
        &Customer{},
    )
}
```

### 3. Configuration Structure

Shared configuration that both teams use:
```go
type LambdaConfig struct {
    // Team 1 implements
    BaseConfig    Config
    EnableXRay    bool
    WarmPoolSize  int
    TimeoutBuffer time.Duration
    
    // Team 2 adds
    PaymentConfig PaymentConfig
}

type PaymentConfig struct {
    IdempotencyTTL time.Duration
    AuditTableName string
    KMSKeyID       string
}
```

### 4. Error Types

Common errors both teams should handle:
```go
var (
    // Team 1 defines
    ErrLambdaTimeout    = errors.New("lambda execution timeout")
    ErrPartnerNotFound  = errors.New("partner account not found")
    ErrAssumeRoleFailed = errors.New("failed to assume role")
    
    // Team 2 defines
    ErrDuplicatePayment = errors.New("duplicate idempotency key")
    ErrInvalidMerchant  = errors.New("invalid merchant context")
)
```

## Timeline & Dependencies

```
Day 1-2: Team 1 Core Implementation
├── lambda.go basic structure
├── multiaccount.go interfaces
└── Basic tests

Day 2-3: Team 2 Models & Utils (can start in parallel)
├── Payment models
├── Helper utilities
└── Unit tests

Day 3-4: Integration (requires Team 1 completion)
├── Team 2 Lambda handlers
├── Integration tests
└── Performance benchmarks

Day 5: Joint Testing & Documentation
├── End-to-end testing
├── Performance validation
└── Documentation updates
```

## Integration Checklist

### Team 1 → Team 2 Handoff
- [ ] LambdaDB type exported and documented
- [ ] MultiAccountDB implemented with caching
- [ ] Timeout handling tested
- [ ] Example usage provided
- [ ] Performance benchmarks shared

### Team 2 → Team 1 Feedback
- [ ] API ergonomics feedback
- [ ] Performance requirements met?
- [ ] Missing features identified
- [ ] Integration issues reported
- [ ] Documentation gaps noted

## Testing Strategy

### Unit Tests (Independent)
- Team 1: Lambda core functionality
- Team 2: Payment logic and utilities

### Integration Tests (Joint)
```go
// tests/lambda_integration_test.go
func TestLambdaPaymentFlow(t *testing.T) {
    // Team 1's Lambda DB
    db := dynamorm.NewLambdaOptimized()
    
    // Team 2's payment processing
    processor := payment.NewProcessor(db)
    
    // End-to-end test
    result := processor.ProcessPayment(testPayment)
}
```

### Performance Tests (Joint)
- Cold start benchmarks
- Multi-account switching
- High-volume payment processing
- Memory usage profiling

## Communication Protocol

### Daily Sync Points
1. **Morning**: Review overnight progress
2. **Midday**: Address blockers
3. **EOD**: Update status and plan

### Shared Documents
- API changes log
- Performance metrics dashboard
- Integration test results
- Known issues tracker

### Code Review Process
1. Team 1 reviews Team 2's Lambda usage
2. Team 2 reviews Team 1's API design
3. Joint review of integration points
4. Performance review by both teams

## Success Metrics

### Team 1 Deliverables
- [ ] Lambda cold start < 100ms
- [ ] Multi-account switch < 50ms  
- [ ] Zero timeout errors in tests
- [ ] 100% backward compatibility

### Team 2 Deliverables
- [ ] Payment processing < 50ms
- [ ] Idempotency working correctly
- [ ] All integration tests passing
- [ ] Example handlers documented

### Joint Success
- [ ] Full payment flow working end-to-end
- [ ] Performance targets met
- [ ] Documentation complete
- [ ] Ready for Session 5

## Potential Issues & Mitigations

### Issue: API Changes
**Risk**: Team 1 changes affect Team 2
**Mitigation**: Lock interfaces early, version changes

### Issue: Performance Regression  
**Risk**: Features impact performance
**Mitigation**: Continuous benchmarking, profiling

### Issue: Integration Complexity
**Risk**: Multi-account + Lambda + Payments
**Mitigation**: Incremental integration, good tests

Remember: Communication is key! Don't hesitate to reach out early and often. 
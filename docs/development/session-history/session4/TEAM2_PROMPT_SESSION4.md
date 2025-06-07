# Team 2 Prompt - Session 4: Payment Features & Examples

## Context
You are Team 2 working on DynamORM, a Go ORM for DynamoDB. While Team 1 implements core Lambda support, your task is to build payment-specific features and comprehensive examples that showcase DynamORM's capabilities for Pay Theory's use case.

## Your Mission
Create payment platform features and examples including:
1. Payment models with idempotency
2. Lambda handlers for payment processing
3. Helper utilities for common patterns
4. Integration tests for real scenarios

## Key Requirements

### 1. Payment Models (`examples/payment/models.go`)
Create realistic payment platform models:

```go
// Payment with idempotency support
type Payment struct {
    ID             string    `dynamorm:"pk"`
    IdempotencyKey string    `dynamorm:"index:gsi-idempotency,unique"`
    MerchantID     string    `dynamorm:"index:gsi-merchant,pk"`
    Amount         int64     // Always in cents
    Currency       string
    Status         string    `dynamorm:"index:gsi-merchant,sk,prefix:status"`
    CreatedAt      time.Time `dynamorm:"created_at"`
    UpdatedAt      time.Time `dynamorm:"updated_at"`
    Version        int       `dynamorm:"version"`
}

// Transaction with audit trail
type Transaction struct {
    ID            string         `dynamorm:"pk"`
    PaymentID     string         `dynamorm:"index:gsi-payment"`
    Type          string         // capture, refund, void
    Amount        int64
    Status        string
    ProcessedAt   time.Time
    AuditTrail    []AuditEntry   `dynamorm:"json"`
    Version       int            `dynamorm:"version"`
}

// Customer with PCI compliance
type Customer struct {
    ID           string    `dynamorm:"pk"`
    MerchantID   string    `dynamorm:"index:gsi-merchant,pk"`
    Email        string    `dynamorm:"index:gsi-email,encrypted"`
    PaymentMethods []PaymentMethod `dynamorm:"json,encrypted:pci"`
}
```

### 2. Lambda Handlers (`examples/payment/lambda/`)

#### Process Payment Handler
```go
func ProcessPaymentHandler(ctx context.Context, event APIGatewayProxyRequest) (APIGatewayProxyResponse, error) {
    // Extract merchant from JWT
    // Check idempotency
    // Process payment
    // Return response
}
```

#### Batch Reconciliation Handler
```go
func ReconciliationHandler(ctx context.Context, event S3Event) error {
    // Stream large CSV from S3
    // Process in batches
    // Update payment statuses
    // Generate report
}
```

#### Multi-Tenant Query Handler
```go
func MerchantPaymentsHandler(ctx context.Context, event APIGatewayProxyRequest) (APIGatewayProxyResponse, error) {
    // Get merchant context
    // Query with pagination
    // Apply filters
    // Return results
}
```

### 3. Helper Utilities (`examples/payment/utils/`)

#### Idempotency Middleware
```go
type IdempotencyMiddleware struct {
    db *dynamorm.DB
    ttl time.Duration
}

func (m *IdempotencyMiddleware) Process(key string, fn func() (interface{}, error)) (interface{}, error) {
    // Check if already processed
    // If yes, return cached result
    // If no, process and cache
}
```

#### Audit Trail Tracker
```go
type AuditTracker struct {
    db *dynamorm.DB
}

func (a *AuditTracker) Track(entity interface{}, action string, user string) error {
    // Record who did what when
    // Store in audit log
    // Maintain compliance
}
```

#### Cost Estimator
```go
type CostEstimator struct {
    readCost  float64
    writeCost float64
}

func (c *CostEstimator) EstimateMonthly(metrics Metrics) float64 {
    // Calculate based on usage
    // Include storage costs
    // Project monthly bill
}
```

### 4. Integration Tests (`examples/payment/tests/`)

#### Multi-Account Flow Test
```go
func TestMultiAccountPaymentFlow(t *testing.T) {
    // Create payment in account A
    // Transfer to account B
    // Verify in both accounts
    // Check audit trail
}
```

#### High Volume Test
```go
func TestHighVolumeProcessing(t *testing.T) {
    // Generate 10,000 payments
    // Process in parallel
    // Verify all succeeded
    // Check performance metrics
}
```

#### Error Handling Test
```go
func TestPaymentErrorScenarios(t *testing.T) {
    // Duplicate idempotency key
    // Invalid merchant
    // Timeout scenarios
    // Recovery procedures
}
```

## Technical Specifications

### Pagination Pattern
```go
type PagedResult struct {
    Items      []Payment
    NextCursor string
    Total      int64
}

func GetMerchantPayments(merchantID string, cursor string) (*PagedResult, error) {
    // Use DynamoDB pagination
    // Encode/decode cursor
    // Return consistent pages
}
```

### Batch Processing Pattern
```go
func ProcessPaymentBatch(payments []*Payment) error {
    // Chunk into DynamoDB limits
    // Process in parallel
    // Handle partial failures
    // Return aggregate result
}
```

### Encryption Pattern
```go
type EncryptedField struct {
    KMSKeyID string
    Value    []byte
}

// Transparent encryption/decryption
// Audit encryption access
// Rotate keys safely
```

## Deliverables
1. Complete payment models with all features
2. Three working Lambda handlers
3. Utility package with helpers
4. Comprehensive test suite
5. Performance benchmark results

## Performance Targets
- Payment creation: < 50ms
- Idempotency check: < 10ms
- Batch of 1000: < 5 seconds
- Query 10k records: < 200ms

## Example Structure
```
examples/payment/
├── models.go           # Domain models
├── lambda/
│   ├── process/       # Payment processor
│   ├── reconcile/     # Reconciliation
│   └── query/         # Query API
├── utils/
│   ├── idempotency.go
│   ├── audit.go
│   └── cost.go
├── tests/
│   ├── integration_test.go
│   ├── load_test.go
│   └── benchmarks_test.go
└── README.md          # How to run
```

## Files You'll Need
- Team 1's Lambda implementation
- `PAYTHEORY_OPTIMIZATIONS.md` - Feature requirements
- `pkg/types/converter.go` - Type conversion
- `pkg/model/registry.go` - Model registration

## Success Criteria
- [ ] All payment flows working end-to-end
- [ ] Idempotency prevents duplicates
- [ ] Audit trail captures all changes
- [ ] Performance meets targets
- [ ] Examples are clear and reusable

Remember: These examples should serve as templates for Pay Theory's actual implementation! 
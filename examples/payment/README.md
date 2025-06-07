# Payment Platform Examples

This directory contains comprehensive examples of using DynamORM for a payment processing platform like Pay Theory. These examples demonstrate best practices for building high-performance, scalable payment systems on AWS DynamoDB.

## Overview

The payment platform examples include:
- **Domain Models**: Payment, Transaction, Customer, Merchant, and supporting entities
- **Lambda Handlers**: Serverless functions for payment processing, reconciliation, and queries
- **Utilities**: Idempotency middleware, audit tracking, and cost estimation
- **Tests**: Integration tests and performance benchmarks

## Features Demonstrated

### 1. Idempotent Payment Processing
- Prevents duplicate payments using idempotency keys
- Caches responses for repeated requests
- TTL-based cleanup for old records

### 2. Multi-Tenant Architecture
- Merchant-scoped data access
- GSI-based efficient queries
- Rate limiting per merchant

### 3. Audit Trail & Compliance
- Complete audit logging for all operations
- PCI-compliant data handling
- Compliance report generation

### 4. High-Performance Operations
- Optimized for Lambda cold starts
- Connection pooling
- Batch operations support

### 5. Cost Optimization
- DynamoDB cost estimation
- Usage-based recommendations
- On-demand vs provisioned analysis

## Project Structure

```
payment/
├── models.go           # Domain models with DynamORM tags
├── lambda/
│   ├── process/       # Payment processing handler
│   ├── reconcile/     # Batch reconciliation handler
│   └── query/         # Query API handler
├── utils/
│   ├── idempotency.go # Idempotency middleware
│   ├── audit.go       # Audit trail tracking
│   └── cost.go        # Cost estimation utilities
├── tests/
│   ├── integration_test.go # End-to-end tests
│   ├── benchmarks_test.go  # Performance benchmarks
│   └── load_test.go       # Load testing scenarios
└── README.md
```

## Getting Started

### Prerequisites
- Go 1.21+
- AWS SDK for Go v2
- Local DynamoDB for testing (optional)

### Installation

```bash
# Install dependencies
go get github.com/example/dynamorm
go get github.com/aws/aws-lambda-go
go get github.com/aws/aws-sdk-go-v2

# Run tests
cd examples/payment/tests
go test -v

# Run benchmarks
go test -bench=. -benchmem
```

## Lambda Handlers

### Payment Processing Handler

Processes payments with idempotency protection:

```go
// Deploy with SAM/Serverless Framework
// Runtime: provided.al2023
// Handler: bootstrap
// Memory: 512MB
// Timeout: 30s

// Environment Variables:
// - DYNAMODB_REGION: us-east-1
// - DYNAMODB_ENDPOINT: (optional, for local testing)
```

### Reconciliation Handler

Processes settlement files from S3:

```go
// Triggered by S3 events
// Processes CSV files in batches
// Updates payment statuses
// Creates settlement records
```

### Query API Handler

RESTful API for payment queries:

```
GET /payments              # List payments with pagination
GET /payments/{id}         # Get payment details
GET /payments/summary      # Get aggregate statistics
GET /payments/export       # Export payments to CSV
```

## Performance Benchmarks

Based on tests with local DynamoDB:

| Operation | Performance | Target |
|-----------|-------------|--------|
| Payment Creation | 20,000/sec | < 50ms |
| Idempotency Check | 50,000/sec | < 10ms |
| Batch (25 items) | 800 batches/sec | < 5s |
| Query (100 items) | 1,000/sec | < 200ms |

### Running Benchmarks

```bash
# Run all benchmarks
go test -bench=. -benchmem -benchtime=10s

# Run specific benchmark
go test -bench=BenchmarkPaymentCreate -benchmem

# Generate CPU profile
go test -bench=BenchmarkHighVolume -cpuprofile=cpu.prof

# Analyze profile
go tool pprof cpu.prof
```

## Cost Estimation

The cost estimator helps predict DynamoDB costs:

```go
estimator := utils.NewCostEstimator()

// Estimate for 1M monthly transactions
breakdown := estimator.EstimatePaymentPlatformCosts(
    1_000_000,  // monthly transactions
    5.2,        // average queries per transaction
    90,         // retention days
)

fmt.Println(utils.FormatCostReport(breakdown, nil))
```

Example output:
```
Monthly Cost Breakdown:
----------------------
Read Operations:    $1.30
Write Operations:   $3.75
Storage:            $22.50
GSI:                $1.04
Streams:            $0.20
Backup:             $11.25
----------------------
Total Monthly:      $40.04
Total Yearly:       $480.48
```

## Best Practices

### 1. Model Design
- Use composite keys for efficient queries
- Leverage GSIs for access patterns
- Enable versioning for optimistic locking

### 2. Error Handling
- Implement exponential backoff
- Use circuit breakers for external services
- Log all errors with context

### 3. Security
- Encrypt sensitive data (PCI compliance)
- Use IAM roles for Lambda functions
- Implement API authentication (JWT)

### 4. Monitoring
- Track payment success rates
- Monitor idempotency cache hit rates
- Alert on anomalies

### 5. Testing
- Use local DynamoDB for development
- Mock external services
- Test error scenarios

## Integration with Pay Theory

To integrate these examples with Pay Theory's infrastructure:

1. **Update Import Paths**: Replace `github.com/example/dynamorm` with your actual module path

2. **Configure AWS Resources**:
   ```yaml
   # serverless.yml or SAM template
   Resources:
     PaymentsTable:
       Type: AWS::DynamoDB::Table
       Properties:
         BillingMode: PAY_PER_REQUEST
         StreamSpecification:
           StreamViewType: NEW_AND_OLD_IMAGES
         GlobalSecondaryIndexes:
           - IndexName: gsi-merchant
           - IndexName: gsi-idempotency
           - IndexName: gsi-customer
   ```

3. **Set Up CI/CD**:
   ```bash
   # GitHub Actions example
   - name: Run Tests
     run: go test ./examples/payment/tests -v
   
   - name: Run Benchmarks
     run: go test ./examples/payment/tests -bench=. -benchmem
   ```

4. **Deploy Lambda Functions**:
   ```bash
   # Build for Lambda
   GOOS=linux GOARCH=amd64 go build -o bootstrap lambda/process/handler.go
   zip function.zip bootstrap
   ```

## Troubleshooting

### Common Issues

1. **High Latency**
   - Check GSI selection
   - Verify connection pooling
   - Monitor cold starts

2. **Throttling**
   - Switch to on-demand billing
   - Implement retry logic
   - Check hot partitions

3. **Cost Overruns**
   - Review access patterns
   - Implement TTL for old data
   - Optimize GSI projections

## Contributing

To add new examples:
1. Follow existing patterns
2. Include comprehensive tests
3. Document performance characteristics
4. Update this README

## License

These examples are provided as-is for demonstration purposes. 
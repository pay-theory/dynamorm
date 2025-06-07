# DynamORM Lambda Optimizations

## Context
Pay Theory runs a 100% serverless architecture on AWS Lambda with partner stack deployments partitioned by AWS accounts. This requires specific optimizations for cold starts, memory efficiency, and cross-account operations.

## 🚀 Cold Start Optimizations

### 1. Lazy Initialization & Connection Reuse

```go
package main

import (
    "context"
    "sync"
    "github.com/pay-theory/dynamorm"
)

var (
    db   *dynamorm.DB
    once sync.Once
)

// Initialize DynamORM outside handler for connection reuse
func initDB() {
    once.Do(func() {
        config := dynamorm.Config{
            Region: os.Getenv("AWS_REGION"),
            // Lambda-optimized settings
            HTTPClient: &http.Client{
                Timeout: 5 * time.Second, // Short timeout for Lambda
                Transport: &http.Transport{
                    MaxIdleConns:        10,  // Lower for Lambda
                    MaxIdleConnsPerHost: 10,
                    IdleConnTimeout:     90 * time.Second,
                },
            },
        }
        
        var err error
        db, err = dynamorm.New(config)
        if err != nil {
            panic(err)
        }
        
        // Pre-warm model registry (reduces first query latency)
        db.PreRegisterModels(
            &Payment{},
            &Transaction{},
            &Customer{},
        )
    })
}

func handler(ctx context.Context, event Event) error {
    initDB() // Reuses connection across invocations
    
    // Your handler logic
    return db.Model(&Payment{}).Create()
}
```

### 2. Minimal Dependency Loading

```go
// dynamorm_lambda.go - Lambda-specific build
// +build lambda

package dynamorm

import (
    // Only essential imports for Lambda
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    // Exclude heavy dependencies not needed in Lambda
)

// LambdaDB is a lightweight version optimized for Lambda
type LambdaDB struct {
    *DB
    // Skip features not needed in Lambda:
    // - Connection pooling
    // - Background workers
    // - Metrics collection
}

func NewLambda(config Config) (*LambdaDB, error) {
    // Streamlined initialization
    config.DisablePooling = true
    config.DisableMetrics = true
    config.DisableBackgroundTasks = true
    
    db, err := New(config)
    if err != nil {
        return nil, err
    }
    
    return &LambdaDB{DB: db}, nil
}
```

### 3. Model Registry Pre-compilation

```go
// Pre-compile model metadata at build time to reduce cold starts
//go:generate dynamorm-gen models

// Generated file: models_gen.go
var precompiledModels = map[string]*model.Metadata{
    "Payment": {
        TableName: "payments",
        Fields: map[string]*model.FieldMetadata{
            "ID": {Name: "ID", DynamoName: "id", IsPK: true},
            // ... pre-computed metadata
        },
    },
}

// Use pre-compiled metadata in Lambda
func (db *LambdaDB) Model(model interface{}) Query {
    typeName := reflect.TypeOf(model).Elem().Name()
    if metadata, ok := precompiledModels[typeName]; ok {
        return &query{db: db, metadata: metadata}
    }
    // Fallback to runtime registration
    return db.DB.Model(model)
}
```

## 🏢 Multi-Account Partner Architecture

### 1. Cross-Account Assume Role Support

```go
// Built-in support for cross-account access
type MultiAccountConfig struct {
    // Base account configuration
    BaseConfig Config
    
    // Partner account mappings
    PartnerAccounts map[string]PartnerAccount
}

type PartnerAccount struct {
    AccountID   string
    RoleARN     string
    ExternalID  string
    Region      string
}

// Create DB with partner context
func NewMultiAccount(config MultiAccountConfig) *MultiAccountDB {
    return &MultiAccountDB{
        base:     config.BaseConfig,
        partners: config.PartnerAccounts,
        cache:    make(map[string]*DB),
    }
}

// Use partner account dynamically
func (mdb *MultiAccountDB) Partner(partnerID string) (*DB, error) {
    // Check cache first (important for Lambda reuse)
    if db, ok := mdb.cache[partnerID]; ok {
        return db, nil
    }
    
    partner, ok := mdb.partners[partnerID]
    if !ok {
        return nil, fmt.Errorf("unknown partner: %s", partnerID)
    }
    
    // Assume role for partner account
    cfg, err := config.LoadDefaultConfig(context.Background(),
        config.WithRegion(partner.Region),
        config.WithAssumeRoleCredentialOptions(func(o *stscreds.AssumeRoleOptions) {
            o.RoleARN = partner.RoleARN
            o.ExternalID = partner.ExternalID
            o.RoleSessionName = fmt.Sprintf("dynamorm-%s", partnerID)
        }),
    )
    
    db, err := New(Config{Config: cfg})
    if err != nil {
        return nil, err
    }
    
    mdb.cache[partnerID] = db
    return db, nil
}
```

### 2. Partner-Aware Queries

```go
// Automatic partner context from Lambda event
func handler(ctx context.Context, event APIGatewayEvent) error {
    // Extract partner context from JWT claims or headers
    partnerID := extractPartnerID(event)
    
    // Get partner-specific DB
    db, err := multiAccountDB.Partner(partnerID)
    if err != nil {
        return err
    }
    
    // All queries now use partner's DynamoDB tables
    payment := &Payment{}
    err = db.Model(payment).
        Where("ID", "=", event.PathParameters["id"]).
        First(payment)
    
    return err
}
```

## ⚡ Lambda-Specific Performance

### 1. Batch Request Optimization

```go
// Lambda-aware batch processing
type LambdaBatcher struct {
    db            *DB
    maxBatchSize  int
    flushInterval time.Duration
}

// Optimize for Lambda's execution model
func NewLambdaBatcher(db *DB) *LambdaBatcher {
    return &LambdaBatcher{
        db:            db,
        maxBatchSize:  25, // DynamoDB limit
        flushInterval: 100 * time.Millisecond, // Quick flush for Lambda
    }
}

// Process items in optimal batches
func (b *LambdaBatcher) ProcessPayments(payments []*Payment) error {
    // Split into optimal chunks for Lambda memory
    chunks := chunkSlice(payments, b.maxBatchSize)
    
    // Process in parallel (Lambda has CPU proportional to memory)
    errChan := make(chan error, len(chunks))
    
    for _, chunk := range chunks {
        go func(items []*Payment) {
            errChan <- b.db.Model(&Payment{}).BatchCreate(items)
        }(chunk)
    }
    
    // Collect errors
    for i := 0; i < len(chunks); i++ {
        if err := <-errChan; err != nil {
            return err
        }
    }
    
    return nil
}
```

### 2. Memory-Efficient Pagination

```go
// Stream large result sets without loading all into memory
func (db *LambdaDB) StreamQuery(model interface{}, process func(item interface{}) error) error {
    query := db.Model(model)
    
    // Use smaller page size for Lambda memory constraints
    pageSize := 100
    if lambdaMemoryMB := getLambdaMemoryMB(); lambdaMemoryMB < 512 {
        pageSize = 25 // Smaller pages for low-memory Lambdas
    }
    
    var lastEvaluatedKey map[string]types.AttributeValue
    
    for {
        page := reflect.New(reflect.SliceOf(reflect.TypeOf(model))).Interface()
        
        err := query.
            Limit(pageSize).
            StartKey(lastEvaluatedKey).
            All(page)
        
        if err != nil {
            return err
        }
        
        // Process items one by one to minimize memory
        items := reflect.ValueOf(page).Elem()
        for i := 0; i < items.Len(); i++ {
            if err := process(items.Index(i).Interface()); err != nil {
                return err
            }
        }
        
        // Check if more pages
        if query.LastEvaluatedKey() == nil {
            break
        }
        lastEvaluatedKey = query.LastEvaluatedKey()
    }
    
    return nil
}
```

### 3. Context-Aware Timeouts

```go
// Respect Lambda's remaining execution time
func (db *LambdaDB) WithLambdaContext(ctx context.Context) *LambdaDB {
    deadline, ok := ctx.Deadline()
    if !ok {
        return db
    }
    
    // Leave 1 second buffer for Lambda cleanup
    timeRemaining := time.Until(deadline) - time.Second
    
    // Set shorter timeout for DynamoDB operations
    queryCtx, cancel := context.WithTimeout(ctx, timeRemaining)
    
    return &LambdaDB{
        DB:     db.WithContext(queryCtx),
        cancel: cancel,
    }
}

// Usage in handler
func handler(ctx context.Context, event Event) error {
    db := globalDB.WithLambdaContext(ctx)
    
    // All queries will respect Lambda timeout
    return db.Model(&Payment{}).Where("Status", "=", "pending").All(&payments)
}
```

## 🔧 Lambda Configuration

### 1. Environment-Based Configuration

```go
// Lambda-friendly configuration from environment
func NewFromEnvironment() (*LambdaDB, error) {
    config := Config{
        Region:    os.Getenv("AWS_REGION"),
        TablePrefix: os.Getenv("DYNAMODB_TABLE_PREFIX"), // For multi-tenant
        
        // Lambda-specific optimizations
        MaxRetries: 3, // Lower for Lambda timeout
        RetryMode:  aws.RetryModeAdaptive,
        
        // Disable features not needed in Lambda
        DisableSSL:      false, // Keep SSL but optimize handshake
        DisableCompute:  true,  // No need for compute optimization in Lambda
        
        // Custom timeout for Lambda
        Timeout: parseDuration(os.Getenv("DYNAMODB_TIMEOUT"), 5*time.Second),
    }
    
    return NewLambda(config)
}
```

### 2. Lambda Layer Support

```bash
# Build DynamORM as a Lambda layer
make lambda-layer

# Creates: dynamorm-layer.zip with optimal structure:
# /opt/lib/dynamorm.so        # Pre-compiled for fast loading
# /opt/bin/dynamorm-gen       # Model generator
```

### 3. SAM/CDK Integration

```go
// CDK construct for DynamORM tables
type DynamORMStack struct {
    stack.Stack
}

func NewDynamORMStack(scope constructs.Construct, id string, props *DynamORMStackProps) *DynamORMStack {
    stack := stack.NewStack(scope, id, props)
    
    // Create tables with Lambda-optimized settings
    paymentsTable := dynamodb.NewTable(stack, jsii.String("PaymentsTable"), &dynamodb.TableProps{
        PartitionKey: &dynamodb.Attribute{
            Name: jsii.String("id"),
            Type: dynamodb.AttributeType_STRING,
        },
        BillingMode: dynamodb.BillingMode_PAY_PER_REQUEST, // Best for Lambda
        PointInTimeRecovery: jsii.Bool(true),
        
        // Optimize for Lambda access patterns
        TableClass: dynamodb.TableClass_STANDARD_INFREQUENT_ACCESS,
    })
    
    // Grant Lambda permissions
    paymentsTable.GrantReadWriteData(lambdaFunction)
    
    return stack
}
```

## 📊 Monitoring & Debugging

### 1. X-Ray Integration

```go
// Automatic X-Ray tracing for Lambda
func init() {
    if os.Getenv("_X_AMZN_TRACE_ID") != "" {
        xray.Configure(xray.Config{
            LogLevel: "error",
        })
        
        // Wrap DynamoDB client
        dynamorm.EnableXRayTracing()
    }
}
```

### 2. CloudWatch Metrics

```go
// Lightweight metrics for Lambda
type LambdaMetrics struct {
    Namespace string
}

func (m *LambdaMetrics) RecordLatency(operation string, duration time.Duration) {
    // Use EMF for zero-latency metrics
    fmt.Printf(`{"_aws":{"Timestamp":%d,"CloudWatchMetrics":[{"Namespace":"%s","Dimensions":[["Operation"]],"Metrics":[{"Name":"Latency","Unit":"Milliseconds"}]}]},"Operation":"%s","Latency":%d}`,
        time.Now().Unix()*1000,
        m.Namespace,
        operation,
        duration.Milliseconds(),
    )
}
```

## 🎯 Best Practices

### 1. Lambda Handler Pattern

```go
// Recommended handler structure
package main

import (
    "context"
    "github.com/pay-theory/dynamorm"
)

var db *dynamorm.LambdaDB

func init() {
    // Initialize once, reuse across invocations
    db = initializeDynamORM()
}

func handler(ctx context.Context, event Event) (Response, error) {
    // Create timeout-aware context
    ctx, cancel := context.WithTimeout(ctx, 29*time.Second) // 1s buffer
    defer cancel()
    
    // Use partner-specific DB if needed
    partnerDB := db
    if event.PartnerID != "" {
        partnerDB = db.Partner(event.PartnerID)
    }
    
    // Execute with context
    return processEvent(partnerDB.WithContext(ctx), event)
}
```

### 2. Testing in Lambda Environment

```go
// Lambda-specific test utilities
func TestLambdaHandler(t *testing.T) {
    // Simulate Lambda environment
    os.Setenv("AWS_LAMBDA_FUNCTION_NAME", "test-function")
    os.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "256")
    
    // Test with Lambda constraints
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    
    response, err := handler(ctx, testEvent)
    assert.NoError(t, err)
}
```

## Summary

These Lambda-specific optimizations ensure DynamORM:
1. **Minimizes cold starts** through lazy initialization and pre-compilation
2. **Supports multi-account architectures** with built-in assume role
3. **Optimizes for Lambda constraints** (memory, timeout, CPU)
4. **Integrates with serverless tooling** (SAM, CDK, X-Ray)
5. **Provides Lambda-aware patterns** for common use cases

The key is making DynamORM "Lambda-native" rather than just "Lambda-compatible". 
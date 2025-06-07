# Lambda Implementation Guide for DynamORM

## Quick Start Implementation

### Step 1: Add Lambda-Optimized Configuration

Create `lambda.go` in the root package:

```go
// lambda.go
package dynamorm

import (
    "context"
    "net/http"
    "os"
    "sync"
    "time"
    
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/pay-theory/dynamorm/pkg/session"
)

// LambdaDB wraps DB with Lambda-specific optimizations
type LambdaDB struct {
    *DB
    modelCache sync.Map // Cache pre-registered models
}

// NewLambdaOptimized creates a Lambda-optimized DB instance
func NewLambdaOptimized() (*LambdaDB, error) {
    // Load config with Lambda optimizations
    cfg, err := config.LoadDefaultConfig(context.Background(),
        config.WithRegion(os.Getenv("AWS_REGION")),
        config.WithHTTPClient(&http.Client{
            Timeout: 5 * time.Second,
            Transport: &http.Transport{
                MaxIdleConns:        10,
                MaxIdleConnsPerHost: 10,
                IdleConnTimeout:     90 * time.Second,
            },
        }),
        config.WithRetryMode(aws.RetryModeAdaptive),
        config.WithRetryMaxAttempts(3),
    )
    if err != nil {
        return nil, err
    }
    
    db, err := New(session.Config{
        Config: cfg,
    })
    if err != nil {
        return nil, err
    }
    
    return &LambdaDB{
        DB: db,
    }, nil
}

// PreRegisterModels registers models at init time to reduce cold starts
func (ldb *LambdaDB) PreRegisterModels(models ...interface{}) error {
    for _, model := range models {
        if err := ldb.registry.Register(model); err != nil {
            return err
        }
        // Cache the model type for fast lookup
        ldb.modelCache.Store(reflect.TypeOf(model), true)
    }
    return nil
}
```

### Step 2: Add Multi-Account Support

Create `multiacccount.go`:

```go
// multiacccount.go
package dynamorm

import (
    "context"
    "fmt"
    "sync"
    
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/credentials/stscreds"
    "github.com/aws/aws-sdk-go-v2/service/sts"
)

// MultiAccountDB manages DynamoDB connections across multiple AWS accounts
type MultiAccountDB struct {
    baseDB   *LambdaDB
    accounts map[string]AccountConfig
    cache    sync.Map // Cache DB connections per account
}

// AccountConfig holds configuration for a partner account
type AccountConfig struct {
    RoleARN    string
    ExternalID string
    Region     string
}

// NewMultiAccount creates a multi-account aware DB
func NewMultiAccount(accounts map[string]AccountConfig) (*MultiAccountDB, error) {
    baseDB, err := NewLambdaOptimized()
    if err != nil {
        return nil, err
    }
    
    return &MultiAccountDB{
        baseDB:   baseDB,
        accounts: accounts,
    }, nil
}

// Partner returns a DB instance for the specified partner account
func (mdb *MultiAccountDB) Partner(partnerID string) (*LambdaDB, error) {
    // Check cache first
    if cached, ok := mdb.cache.Load(partnerID); ok {
        return cached.(*LambdaDB), nil
    }
    
    // Get account config
    account, ok := mdb.accounts[partnerID]
    if !ok {
        return nil, fmt.Errorf("unknown partner: %s", partnerID)
    }
    
    // Create STS client
    cfg, err := config.LoadDefaultConfig(context.Background())
    if err != nil {
        return nil, err
    }
    
    stsClient := sts.NewFromConfig(cfg)
    
    // Create credentials provider for assume role
    creds := stscreds.NewAssumeRoleProvider(stsClient, account.RoleARN, func(o *stscreds.AssumeRoleOptions) {
        o.ExternalID = &account.ExternalID
        o.RoleSessionName = fmt.Sprintf("dynamorm-%s", partnerID)
    })
    
    // Create new config with assumed role
    partnerCfg, err := config.LoadDefaultConfig(context.Background(),
        config.WithRegion(account.Region),
        config.WithCredentialsProvider(creds),
    )
    if err != nil {
        return nil, err
    }
    
    // Create partner DB
    db, err := New(session.Config{
        Config: partnerCfg,
    })
    if err != nil {
        return nil, err
    }
    
    lambdaDB := &LambdaDB{DB: db}
    
    // Cache for reuse
    mdb.cache.Store(partnerID, lambdaDB)
    
    return lambdaDB, nil
}
```

### Step 3: Add Context-Aware Operations

Update `dynamorm.go` to add Lambda context support:

```go
// Add to DB struct
type DB struct {
    // ... existing fields ...
    lambdaDeadline time.Time
}

// WithLambdaTimeout sets a deadline based on Lambda context
func (db *DB) WithLambdaTimeout(ctx context.Context) *DB {
    deadline, ok := ctx.Deadline()
    if !ok {
        return db
    }
    
    // Leave 1 second buffer for Lambda cleanup
    adjustedDeadline := deadline.Add(-1 * time.Second)
    
    newDB := &DB{
        session:        db.session,
        registry:       db.registry,
        converter:      db.converter,
        ctx:           db.ctx,
        lambdaDeadline: adjustedDeadline,
    }
    
    return newDB
}

// Update query execution to respect Lambda deadline
func (q *query) executeWithTimeout(fn func() error) error {
    if !q.db.lambdaDeadline.IsZero() {
        remaining := time.Until(q.db.lambdaDeadline)
        if remaining <= 0 {
            return fmt.Errorf("lambda timeout exceeded")
        }
        
        ctx, cancel := context.WithTimeout(q.ctx, remaining)
        defer cancel()
        q.ctx = ctx
    }
    
    return fn()
}
```

### Step 4: Create Lambda Handler Template

Create `cmd/lambda-template/main.go`:

```go
package main

import (
    "context"
    "log"
    "os"
    "sync"
    
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/dynamorm"
)

var (
    db   *dynamorm.MultiAccountDB
    once sync.Once
)

// Initialize DB once during cold start
func init() {
    once.Do(func() {
        // Configure partner accounts from environment
        accounts := map[string]dynamorm.AccountConfig{
            "partner1": {
                RoleARN:    os.Getenv("PARTNER1_ROLE_ARN"),
                ExternalID: os.Getenv("PARTNER1_EXTERNAL_ID"),
                Region:     os.Getenv("PARTNER1_REGION"),
            },
            // Add more partners as needed
        }
        
        var err error
        db, err = dynamorm.NewMultiAccount(accounts)
        if err != nil {
            log.Fatalf("Failed to initialize DynamORM: %v", err)
        }
        
        // Pre-register all models to reduce cold start
        baseDB, _ := db.Partner("") // Get base DB
        err = baseDB.PreRegisterModels(
            &Payment{},
            &Transaction{},
            &Customer{},
            // Add all your models here
        )
        if err != nil {
            log.Fatalf("Failed to register models: %v", err)
        }
    })
}

type Event struct {
    PartnerID string                 `json:"partnerId"`
    Action    string                 `json:"action"`
    Data      map[string]interface{} `json:"data"`
}

type Response struct {
    Success bool        `json:"success"`
    Data    interface{} `json:"data,omitempty"`
    Error   string      `json:"error,omitempty"`
}

func handler(ctx context.Context, event Event) (Response, error) {
    // Get partner-specific DB
    partnerDB, err := db.Partner(event.PartnerID)
    if err != nil {
        return Response{Success: false, Error: err.Error()}, nil
    }
    
    // Apply Lambda timeout
    partnerDB = partnerDB.WithLambdaTimeout(ctx)
    
    // Route based on action
    switch event.Action {
    case "getPayment":
        return handleGetPayment(partnerDB, event.Data)
    case "createPayment":
        return handleCreatePayment(partnerDB, event.Data)
    default:
        return Response{Success: false, Error: "unknown action"}, nil
    }
}

func main() {
    lambda.Start(handler)
}
```

### Step 5: Add Makefile Targets for Lambda

Update `Makefile`:

```makefile
# Lambda-specific targets
.PHONY: lambda-build lambda-layer lambda-deploy

LAMBDA_BUILD_FLAGS = -tags lambda -ldflags="-s -w"
GOOS = linux
GOARCH = amd64

# Build Lambda function
lambda-build:
	@echo "Building Lambda function..."
	@mkdir -p build/lambda
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LAMBDA_BUILD_FLAGS) \
		-o build/lambda/bootstrap cmd/lambda-template/main.go
	@cd build/lambda && zip function.zip bootstrap

# Build Lambda layer with DynamORM
lambda-layer:
	@echo "Building Lambda layer..."
	@mkdir -p build/layer/lib
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LAMBDA_BUILD_FLAGS) \
		-buildmode=plugin -o build/layer/lib/dynamorm.so .
	@cd build/layer && zip -r dynamorm-layer.zip .

# Deploy with SAM
lambda-deploy: lambda-build
	@echo "Deploying Lambda function..."
	@sam deploy --template-file sam-template.yaml \
		--stack-name dynamorm-lambda \
		--capabilities CAPABILITY_IAM \
		--parameter-overrides \
			Runtime=provided.al2 \
			MemorySize=256 \
			Timeout=30
```

### Step 6: Create SAM Template

Create `sam-template.yaml`:

```yaml
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31

Parameters:
  Runtime:
    Type: String
    Default: provided.al2
  MemorySize:
    Type: Number
    Default: 256
  Timeout:
    Type: Number
    Default: 30

Globals:
  Function:
    Runtime: !Ref Runtime
    MemorySize: !Ref MemorySize
    Timeout: !Ref Timeout
    Environment:
      Variables:
        AWS_LAMBDA_EXEC_WRAPPER: /opt/bootstrap
        DYNAMODB_TABLE_PREFIX: !Sub "${AWS::StackName}-"

Resources:
  DynamORMFunction:
    Type: AWS::Serverless::Function
    Properties:
      CodeUri: build/lambda/
      Handler: bootstrap
      Layers:
        - !Ref DynamORMLayer
      Environment:
        Variables:
          PARTNER1_ROLE_ARN: !GetAtt Partner1Role.Arn
          PARTNER1_EXTERNAL_ID: "unique-external-id"
          PARTNER1_REGION: !Ref AWS::Region
      Policies:
        - DynamoDBCrudPolicy:
            TableName: !Ref PaymentsTable
        - Statement:
          - Effect: Allow
            Action:
              - sts:AssumeRole
            Resource:
              - !GetAtt Partner1Role.Arn

  DynamORMLayer:
    Type: AWS::Serverless::LayerVersion
    Properties:
      LayerName: dynamorm
      ContentUri: build/layer/
      CompatibleRuntimes:
        - provided.al2

  PaymentsTable:
    Type: AWS::DynamoDB::Table
    Properties:
      TableName: !Sub "${AWS::StackName}-payments"
      BillingMode: PAY_PER_REQUEST
      AttributeDefinitions:
        - AttributeName: id
          AttributeType: S
      KeySchema:
        - AttributeName: id
          KeyType: HASH

  Partner1Role:
    Type: AWS::IAM::Role
    Properties:
      AssumeRolePolicyDocument:
        Version: '2012-10-17'
        Statement:
          - Effect: Allow
            Principal:
              AWS: !Sub "arn:aws:iam::${AWS::AccountId}:root"
            Action: sts:AssumeRole
            Condition:
              StringEquals:
                sts:ExternalId: "unique-external-id"
```

### Step 7: Performance Testing

Create `tests/lambda_test.go`:

```go
package tests

import (
    "context"
    "testing"
    "time"
    
    "github.com/pay-theory/dynamorm"
    "github.com/stretchr/testify/assert"
)

func BenchmarkLambdaColdStart(b *testing.B) {
    for i := 0; i < b.N; i++ {
        // Simulate cold start
        db, err := dynamorm.NewLambdaOptimized()
        assert.NoError(b, err)
        
        // Register models
        err = db.PreRegisterModels(&TestModel{})
        assert.NoError(b, err)
        
        // First query
        var result TestModel
        err = db.Model(&TestModel{}).Where("ID", "=", "test").First(&result)
        _ = err // Ignore not found errors
    }
}

func TestLambdaTimeout(t *testing.T) {
    db, _ := dynamorm.NewLambdaOptimized()
    
    // Create context with short deadline
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    
    // Apply Lambda timeout
    lambdaDB := db.WithLambdaTimeout(ctx)
    
    // This should timeout
    time.Sleep(200 * time.Millisecond)
    
    err := lambdaDB.Model(&TestModel{}).Create()
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "timeout")
}
```

## Implementation Checklist

- [ ] Add `lambda.go` with optimized configuration
- [ ] Add `multiacccount.go` for partner support
- [ ] Update `dynamorm.go` with timeout support
- [ ] Create Lambda handler template
- [ ] Update Makefile with Lambda targets
- [ ] Create SAM template
- [ ] Add Lambda-specific tests
- [ ] Document Lambda patterns in README

## Next Steps

1. **Test locally** with SAM Local
2. **Benchmark cold starts** to verify optimizations
3. **Deploy to staging** and test multi-account flows
4. **Monitor with X-Ray** to identify bottlenecks
5. **Iterate on optimizations** based on real usage

This implementation provides a solid foundation for Lambda usage while maintaining compatibility with non-Lambda environments. 
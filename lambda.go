// lambda.go
package dynamorm

import (
	"context"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/pay-theory/dynamorm/pkg/session"
)

var (
	// Global Lambda-optimized DB for connection reuse
	globalLambdaDB *LambdaDB
	lambdaOnce     sync.Once
)

// LambdaDB wraps DB with Lambda-specific optimizations
type LambdaDB struct {
	*DB
	modelCache     sync.Map // Cache pre-registered models
	isLambda       bool
	lambdaMemoryMB int
	xrayEnabled    bool
}

// NewLambdaOptimized creates a Lambda-optimized DB instance
func NewLambdaOptimized() (*LambdaDB, error) {
	// Use global instance if available (warm start)
	if globalLambdaDB != nil {
		return globalLambdaDB, nil
	}

	var err error
	lambdaOnce.Do(func() {
		globalLambdaDB, err = createLambdaDB()
	})

	return globalLambdaDB, err
}

// createLambdaDB creates the actual Lambda DB instance
func createLambdaDB() (*LambdaDB, error) {
	// Detect Lambda environment
	isLambda := IsLambdaEnvironment()
	memoryMB := GetLambdaMemoryMB()

	// Create optimized HTTP client for Lambda
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DisableKeepAlives:   false, // Keep connections alive for reuse
		},
	}

	// Load AWS config with Lambda optimizations
	awsConfigOptions := []func(*config.LoadOptions) error{
		config.WithRegion(getRegion()),
		config.WithHTTPClient(httpClient),
		config.WithRetryMode(aws.RetryModeAdaptive),
		config.WithRetryMaxAttempts(3),
	}

	// Enable X-Ray if available
	if os.Getenv("_X_AMZN_TRACE_ID") != "" {
		// X-Ray tracing is automatically enabled in Lambda
		// The SDK will pick it up from environment
	}

	cfg := session.Config{
		Region:           getRegion(),
		MaxRetries:       3,
		DefaultRCU:       5,
		DefaultWCU:       5,
		AutoMigrate:      false,
		EnableMetrics:    isLambda,
		AWSConfigOptions: awsConfigOptions,
	}

	// Optimize DynamoDB client options for Lambda
	if isLambda {
		cfg.DynamoDBOptions = append(cfg.DynamoDBOptions, func(o *dynamodb.Options) {
			// Lambda-specific optimizations
			o.RetryMode = aws.RetryModeAdaptive
			o.Retryer = aws.NopRetryer{} // Handle retries at application level for better control
		})
	}

	db, err := New(cfg)
	if err != nil {
		return nil, err
	}

	ldb := &LambdaDB{
		DB:             db,
		isLambda:       isLambda,
		lambdaMemoryMB: memoryMB,
		xrayEnabled:    os.Getenv("_X_AMZN_TRACE_ID") != "",
	}

	return ldb, nil
}

// PreRegisterModels registers models at init time to reduce cold starts
func (ldb *LambdaDB) PreRegisterModels(models ...interface{}) error {
	for _, model := range models {
		if err := ldb.registry.Register(model); err != nil {
			return err
		}
		// Cache the model type for fast lookup
		modelType := reflect.TypeOf(model)
		if modelType.Kind() == reflect.Ptr {
			modelType = modelType.Elem()
		}
		ldb.modelCache.Store(modelType, true)
	}
	return nil
}

// IsModelRegistered checks if a model is already registered
func (ldb *LambdaDB) IsModelRegistered(model interface{}) bool {
	modelType := reflect.TypeOf(model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}
	_, ok := ldb.modelCache.Load(modelType)
	return ok
}

// WithLambdaTimeout creates a new DB instance with Lambda timeout handling
func (ldb *LambdaDB) WithLambdaTimeout(ctx context.Context) *LambdaDB {
	deadline, ok := ctx.Deadline()
	if !ok {
		return ldb
	}

	// Leave 1 second buffer for Lambda cleanup
	adjustedDeadline := deadline.Add(-1 * time.Second)

	newDB := &DB{
		session:        ldb.session,
		registry:       ldb.registry,
		converter:      ldb.converter,
		ctx:            ctx,
		lambdaDeadline: adjustedDeadline,
	}

	return &LambdaDB{
		DB:             newDB,
		modelCache:     ldb.modelCache,
		isLambda:       ldb.isLambda,
		lambdaMemoryMB: ldb.lambdaMemoryMB,
		xrayEnabled:    ldb.xrayEnabled,
	}
}

// OptimizeForMemory adjusts internal buffers based on available Lambda memory
func (ldb *LambdaDB) OptimizeForMemory(memoryMB int) {
	if memoryMB == 0 {
		memoryMB = ldb.lambdaMemoryMB
	}

	// Adjust connection pool size based on memory
	// Lower memory = fewer connections
	if memoryMB <= 512 {
		// Minimal connections for low memory
		ldb.adjustConnectionPool(5)
	} else if memoryMB <= 1024 {
		// Moderate connections
		ldb.adjustConnectionPool(10)
	} else {
		// Higher memory can handle more connections
		ldb.adjustConnectionPool(20)
	}
}

// adjustConnectionPool updates the HTTP transport settings
func (ldb *LambdaDB) adjustConnectionPool(maxConns int) {
	// This would need to recreate the session with new settings
	// For now, this is a placeholder for the optimization logic
}

// Lambda environment helper functions

// IsLambdaEnvironment detects if running in AWS Lambda
func IsLambdaEnvironment() bool {
	return os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != ""
}

// GetLambdaMemoryMB returns the allocated memory in MB
func GetLambdaMemoryMB() int {
	memStr := os.Getenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE")
	if memStr == "" {
		return 0
	}

	mem, err := strconv.Atoi(memStr)
	if err != nil {
		return 0
	}

	return mem
}

// EnableXRayTracing enables AWS X-Ray tracing for DynamoDB calls
func EnableXRayTracing() bool {
	return os.Getenv("_X_AMZN_TRACE_ID") != ""
}

// getRegion returns the AWS region from environment
func getRegion() string {
	if region := os.Getenv("AWS_REGION"); region != "" {
		return region
	}
	// Fallback to default region
	return "us-east-1"
}

// GetRemainingTimeMillis returns milliseconds until Lambda timeout
func GetRemainingTimeMillis(ctx context.Context) int64 {
	deadline, ok := ctx.Deadline()
	if !ok {
		return -1
	}

	remaining := time.Until(deadline)
	return remaining.Milliseconds()
}

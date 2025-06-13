package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/session"
	"github.com/pay-theory/dynamorm/tests/models"
	"github.com/stretchr/testify/require"
)

// TestContext holds test database and cleanup functions
type TestContext struct {
	DB             core.ExtendedDB
	DynamoDBClient *dynamodb.Client
	TablesCreated  []string
	cleanup        []func() error
}

// InitTestDB creates a test database instance with proper cleanup setup
func InitTestDB(t *testing.T) *TestContext {
	t.Helper()

	// Always check for DynamoDB Local availability first
	// This will skip the test with a clear message if DynamoDB Local is not running
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	// Check if DynamoDB Local is running
	if !isDynamoDBLocalRunning(endpoint) {
		t.Skip(`DynamoDB Local is not running.

To run integration tests:
1. Install Docker: https://www.docker.com/
2. Start DynamoDB Local: ./tests/setup_test_env.sh
3. Run tests: go test ./tests/integration -v

Or skip integration tests: SKIP_INTEGRATION=true go test ./...`)
	}

	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Integration tests disabled")
	}

	sessionConfig := session.Config{
		Region:   "us-east-1",
		Endpoint: endpoint,
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
			),
			config.WithRegion("us-east-1"),
		},
	}

	db, err := dynamorm.New(sessionConfig)
	require.NoError(t, err)
	require.NotNil(t, db)

	// Create DynamoDB client for direct operations
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
		),
	)
	require.NoError(t, err)

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = &endpoint
	})

	testCtx := &TestContext{
		DB:             db,
		DynamoDBClient: client,
		TablesCreated:  make([]string, 0),
		cleanup:        make([]func() error, 0),
	}

	// Register cleanup on test completion
	t.Cleanup(func() {
		if err := testCtx.Cleanup(); err != nil {
			t.Logf("Cleanup error: %v", err)
		}
	})

	return testCtx
}

// CreateTable creates a table and registers it for cleanup
func (tc *TestContext) CreateTable(t *testing.T, model any) {
	t.Helper()

	err := tc.DB.CreateTable(model)
	if err != nil && !isTableExistsError(err) {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Get table name for cleanup tracking
	tableName := getTableName(model)
	tc.TablesCreated = append(tc.TablesCreated, tableName)

	// Wait for table to be ready
	tc.WaitForTable(t, tableName)
}

// CreateTableIfNotExists creates a table only if it doesn't exist
func (tc *TestContext) CreateTableIfNotExists(t *testing.T, model any) {
	t.Helper()

	tableName := getTableName(model)

	// Check if table exists
	_, err := tc.DynamoDBClient.DescribeTable(context.TODO(), &dynamodb.DescribeTableInput{
		TableName: &tableName,
	})

	if err != nil {
		// Table doesn't exist, create it
		tc.CreateTable(t, model)
	} else {
		// Table exists, add to cleanup list and clear its data
		tc.TablesCreated = append(tc.TablesCreated, tableName)
		tc.ClearTableData(t, tableName)
	}
}

// ClearTableData removes all items from a table
func (tc *TestContext) ClearTableData(t *testing.T, tableName string) {
	t.Helper()

	ctx := context.TODO()

	// Get table description to understand key schema
	descResp, err := tc.DynamoDBClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: &tableName,
	})
	if err != nil {
		t.Logf("Failed to describe table %s for cleanup: %v", tableName, err)
		return
	}

	table := descResp.Table
	if table == nil {
		return
	}

	// Extract key attributes
	var partitionKey, sortKey string
	for _, keyElement := range table.KeySchema {
		if keyElement.KeyType == types.KeyTypeHash {
			partitionKey = *keyElement.AttributeName
		} else if keyElement.KeyType == types.KeyTypeRange {
			sortKey = *keyElement.AttributeName
		}
	}

	// Scan and delete all items
	scanInput := &dynamodb.ScanInput{
		TableName: &tableName,
	}

	for {
		scanResp, err := tc.DynamoDBClient.Scan(ctx, scanInput)
		if err != nil {
			t.Logf("Failed to scan table %s for cleanup: %v", tableName, err)
			break
		}

		// Delete items in batches
		if len(scanResp.Items) > 0 {
			tc.batchDeleteItems(t, tableName, scanResp.Items, partitionKey, sortKey)
		}

		// Check for more items
		if scanResp.LastEvaluatedKey == nil {
			break
		}
		scanInput.ExclusiveStartKey = scanResp.LastEvaluatedKey
	}
}

// batchDeleteItems deletes items in batches
func (tc *TestContext) batchDeleteItems(t *testing.T, tableName string, items []map[string]types.AttributeValue, partitionKey, sortKey string) {
	t.Helper()

	const batchSize = 25 // DynamoDB limit
	ctx := context.TODO()

	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		writeRequests := make([]types.WriteRequest, 0, end-i)

		for j := i; j < end; j++ {
			item := items[j]
			key := make(map[string]types.AttributeValue)

			// Add partition key
			if pk, exists := item[partitionKey]; exists {
				key[partitionKey] = pk
			}

			// Add sort key if it exists
			if sortKey != "" {
				if sk, exists := item[sortKey]; exists {
					key[sortKey] = sk
				}
			}

			writeRequests = append(writeRequests, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: key,
				},
			})
		}

		if len(writeRequests) > 0 {
			input := &dynamodb.BatchWriteItemInput{
				RequestItems: map[string][]types.WriteRequest{
					tableName: writeRequests,
				},
			}

			_, err := tc.DynamoDBClient.BatchWriteItem(ctx, input)
			if err != nil {
				t.Logf("Failed to batch delete items from %s: %v", tableName, err)
			}
		}
	}
}

// WaitForTable waits for a table to become active
func (tc *TestContext) WaitForTable(t *testing.T, tableName string) {
	t.Helper()

	ctx := context.TODO()
	for i := 0; i < 30; i++ {
		resp, err := tc.DynamoDBClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: &tableName,
		})

		if err == nil && resp.Table != nil && resp.Table.TableStatus == types.TableStatusActive {
			return
		}

		time.Sleep(1 * time.Second)
	}

	t.Fatalf("Table %s did not become active", tableName)
}

// DeleteTable deletes a table (alternative cleanup strategy)
func (tc *TestContext) DeleteTable(t *testing.T, tableName string) {
	t.Helper()

	ctx := context.TODO()
	_, err := tc.DynamoDBClient.DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: &tableName,
	})

	if err != nil {
		// Ignore ResourceNotFoundException
		if !strings.Contains(err.Error(), "ResourceNotFoundException") {
			t.Logf("Failed to delete table %s: %v", tableName, err)
		}
	}

	// Wait for table to be deleted
	for i := 0; i < 30; i++ {
		_, err := tc.DynamoDBClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: &tableName,
		})

		if err != nil && strings.Contains(err.Error(), "ResourceNotFoundException") {
			return // Table successfully deleted
		}

		time.Sleep(1 * time.Second)
	}
}

// AddCleanupFunc adds a custom cleanup function
func (tc *TestContext) AddCleanupFunc(cleanup func() error) {
	tc.cleanup = append(tc.cleanup, cleanup)
}

// Cleanup performs all registered cleanup operations
func (tc *TestContext) Cleanup() error {
	var errors []string

	// Run custom cleanup functions first
	for _, cleanup := range tc.cleanup {
		if err := cleanup(); err != nil {
			errors = append(errors, err.Error())
		}
	}

	// Cleanup strategy: Clear data instead of deleting tables to be faster
	for _, tableName := range tc.TablesCreated {
		tc.ClearTableData(&testing.T{}, tableName)
	}

	// Close database connection
	if err := tc.DB.Close(); err != nil {
		errors = append(errors, fmt.Sprintf("failed to close DB: %v", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// Test utility functions

func isTableExistsError(err error) bool {
	return err != nil &&
		(strings.Contains(err.Error(), "ResourceInUseException") ||
			strings.Contains(err.Error(), "already exists"))
}

func getTableName(model any) string {
	// This is a simple implementation - in practice you'd use reflection
	// to get the table name from the model or TableName() method
	switch model.(type) {
	// Handle models from models package
	case *models.TestUser, models.TestUser:
		return "TestUsers"
	case *models.TestOrder, models.TestOrder:
		return "TestOrders"
	case *models.TestProduct, models.TestProduct:
		return "TestProducts"
	// Handle local test models
	case *TestUser, TestUser:
		return "TestUsers"
	case *TestOrder, TestOrder:
		return "TestOrders"
	case *TestProduct, TestProduct:
		return "TestProducts"
	case *TestAccount, TestAccount:
		return "TestAccounts"
	case *TestBlogPost, TestBlogPost:
		return "TestBlogPosts"
	case *TestComment, TestComment:
		return "TestComments"
	case *TestNote, TestNote:
		return "TestNotes"
	case *TestContact, TestContact:
		return "TestContacts"
	default:
		// Fallback: try to extract from type name
		typeName := fmt.Sprintf("%T", model)
		// Handle models from models package
		if strings.Contains(typeName, "models.") {
			// Extract the base type name
			parts := strings.Split(typeName, ".")
			if len(parts) > 1 {
				baseName := parts[len(parts)-1]
				// Remove pointer prefix if present
				baseName = strings.TrimPrefix(baseName, "*")
				// Return pluralized form
				if !strings.HasSuffix(baseName, "s") {
					return baseName + "s"
				}
				return baseName
			}
		}
		return typeName + "s"
	}
}

// Common test model definitions for reuse across tests

type TestUser struct {
	ID        string `dynamorm:"pk"`
	Email     string `dynamorm:"index:gsi-email"`
	Name      string
	Active    bool
	CreatedAt time.Time `dynamorm:"created_at"`
	UpdatedAt time.Time `dynamorm:"updated_at"`
}

type TestOrder struct {
	OrderID    string `dynamorm:"pk"`
	CustomerID string `dynamorm:"sk"`
	Amount     float64
	Status     string
	CreatedAt  time.Time `dynamorm:"created_at"`
}

type TestProduct struct {
	ProductID string `dynamorm:"pk"`
	Name      string
	Price     float64
	Category  string `dynamorm:"index:gsi-category"`
	InStock   bool
	UpdatedAt time.Time `dynamorm:"updated_at"`
}

type TestAccount struct {
	AccountID string `dynamorm:"pk"`
	UserID    string `dynamorm:"sk"`
	Balance   float64
	Type      string
	Version   int64 `dynamorm:"version"`
}

type TestBlogPost struct {
	PostID      string `dynamorm:"pk"`
	Title       string
	Content     string
	AuthorID    string   `dynamorm:"index:gsi-author"`
	Tags        []string `dynamorm:"set"`
	PublishedAt time.Time
	CreatedAt   time.Time `dynamorm:"created_at"`
	UpdatedAt   time.Time `dynamorm:"updated_at"`
}

type TestComment struct {
	CommentID string `dynamorm:"pk"`
	PostID    string `dynamorm:"sk"`
	AuthorID  string
	Content   string
	CreatedAt time.Time `dynamorm:"created_at"`
}

type TestNote struct {
	ID        string `dynamorm:"pk"`
	Title     string
	Content   string
	Priority  int
	Archived  bool
	Tags      []string
	CreatedAt time.Time `dynamorm:"created_at"`
	UpdatedAt time.Time `dynamorm:"updated_at"`
}

type TestContact struct {
	ID      string `dynamorm:"pk"`
	Name    string
	Email   string
	Phone   string
	Company string
	Active  bool
}

// isDynamoDBLocalRunning checks if DynamoDB Local is accessible
func isDynamoDBLocalRunning(endpoint string) bool {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...any) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:           endpoint,
					SigningRegion: "us-east-1",
				}, nil
			})),
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(
			func(ctx context.Context) (aws.Credentials, error) {
				return aws.Credentials{
					AccessKeyID:     "dummy",
					SecretAccessKey: "dummy",
				}, nil
			})),
	)
	if err != nil {
		return false
	}

	client := dynamodb.NewFromConfig(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.ListTables(ctx, &dynamodb.ListTablesInput{
		Limit: aws.Int32(1),
	})

	return err == nil
}

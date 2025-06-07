package benchmarks

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/tests/models"
)

var (
	benchDB        *dynamorm.DB
	benchClient    *dynamodb.Client
	benchTableName = "BenchmarkTable"
)

func setupBenchDB(b *testing.B) (*dynamorm.DB, *dynamodb.Client) {
	if benchDB != nil && benchClient != nil {
		return benchDB, benchClient
	}

	// Initialize AWS config
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:           "http://localhost:8000",
					SigningRegion: "us-east-1",
				}, nil
			})),
	)
	if err != nil {
		b.Fatal(err)
	}

	// Initialize clients
	benchClient = dynamodb.NewFromConfig(cfg)

	db, err := dynamorm.New(dynamorm.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	if err != nil {
		b.Fatal(err)
	}
	benchDB = db

	// Create test table
	createBenchTable(b)

	// Seed initial data
	seedBenchData(b)

	return benchDB, benchClient
}

func createBenchTable(b *testing.B) {
	ctx := context.TODO()

	// Delete existing table if it exists
	_, _ = benchClient.DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: aws.String(benchTableName),
	})

	// Wait a bit for deletion
	time.Sleep(2 * time.Second)

	// Create table
	input := &dynamodb.CreateTableInput{
		TableName: aws.String(benchTableName),
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("ID"),
				KeyType:       types.KeyTypeHash,
			},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("ID"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("Email"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("Status"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("gsi-email"),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("Email"),
						KeyType:       types.KeyTypeHash,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
			},
			{
				IndexName: aws.String("gsi-status"),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("Status"),
						KeyType:       types.KeyTypeHash,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
			},
		},
	}

	_, err := benchClient.CreateTable(ctx, input)
	if err != nil {
		b.Fatal(err)
	}

	// Wait for table to be active
	for i := 0; i < 30; i++ {
		desc, err := benchClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(benchTableName),
		})
		if err == nil && desc.Table.TableStatus == "ACTIVE" {
			// Check all GSIs are active
			allActive := true
			for _, gsi := range desc.Table.GlobalSecondaryIndexes {
				if gsi.IndexStatus != "ACTIVE" {
					allActive = false
					break
				}
			}
			if allActive {
				return
			}
		}
		time.Sleep(1 * time.Second)
	}
	b.Fatal("Table not active after 30 seconds")
}

func seedBenchData(b *testing.B) {
	ctx := context.TODO()

	// Seed 1000 items for benchmark
	for i := 0; i < 1000; i++ {
		item := map[string]types.AttributeValue{
			"ID":        &types.AttributeValueMemberS{Value: fmt.Sprintf("bench-user-%d", i)},
			"Email":     &types.AttributeValueMemberS{Value: fmt.Sprintf("bench%d@example.com", i)},
			"Status":    &types.AttributeValueMemberS{Value: "active"},
			"Age":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", 20+(i%50))},
			"Name":      &types.AttributeValueMemberS{Value: fmt.Sprintf("Bench User %d", i)},
			"CreatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		}

		_, err := benchClient.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(benchTableName),
			Item:      item,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmarks

func BenchmarkSimpleQuery(b *testing.B) {
	db, _ := setupBenchDB(b)
	user := &models.TestUser{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := db.Model(&models.TestUser{}).
			Where("ID", "=", "bench-user-100").
			First(user)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRawSDKQuery(b *testing.B) {
	_, client := setupBenchDB(b)
	ctx := context.TODO()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		output, err := client.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(benchTableName),
			Key: map[string]types.AttributeValue{
				"ID": &types.AttributeValueMemberS{Value: "bench-user-100"},
			},
		})
		if err != nil {
			b.Fatal(err)
		}
		if output.Item == nil {
			b.Fatal("Item not found")
		}
	}
}

func BenchmarkComplexQueryWithFilters(b *testing.B) {
	db, _ := setupBenchDB(b)
	var users []models.TestUser

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := db.Model(&models.TestUser{}).
			Where("Status", "=", "active").
			Filter("Age > :minAge AND Age < :maxAge",
				core.Param{Name: "minAge", Value: 25},
				core.Param{Name: "maxAge", Value: 35}).
			Limit(20).
			All(&users)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIndexSelection(b *testing.B) {
	db, _ := setupBenchDB(b)

	// Pre-warm the registry with model metadata
	db.Model(&models.TestUser{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Measure just the query building and index selection
		query := db.Model(&models.TestUser{}).
			Where("Email", "=", "bench100@example.com").
			Where("Status", "=", "active")

		// Force compilation without execution
		_ = query
	}
}

func BenchmarkExpressionBuilding(b *testing.B) {
	db, _ := setupBenchDB(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Build a complex query to measure expression building overhead
		query := db.Model(&models.TestUser{}).
			Where("Status", "=", "active").
			Filter("Age > :minAge", core.Param{Name: "minAge", Value: 25}).
			Filter("contains(Tags, :tag)", core.Param{Name: "tag", Value: "premium"}).
			OrderBy("CreatedAt", "desc").
			Select("ID", "Email", "Name", "Age").
			Limit(50)

		// Force compilation without execution
		_ = query
	}
}

func BenchmarkBatchGet(b *testing.B) {
	db, _ := setupBenchDB(b)

	// Prepare keys
	keys := make([]interface{}, 20)
	for i := 0; i < 20; i++ {
		keys[i] = fmt.Sprintf("bench-user-%d", i*10)
	}

	var users []models.TestUser

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := db.Model(&models.TestUser{}).BatchGet(keys, &users)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScanWithFilters(b *testing.B) {
	db, _ := setupBenchDB(b)
	var users []models.TestUser

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := db.Model(&models.TestUser{}).
			Filter("Age > :age", core.Param{Name: "age", Value: 30}).
			Limit(50).
			Scan(&users)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Helper function to calculate overhead percentage
func calculateOverhead(dynamormTime, sdkTime time.Duration) float64 {
	return float64(dynamormTime-sdkTime) / float64(sdkTime) * 100
}

// Comparative benchmark to measure overhead
func BenchmarkOverheadComparison(b *testing.B) {
	db, client := setupBenchDB(b)
	ctx := context.TODO()

	// Benchmark DynamORM
	start := time.Now()
	for i := 0; i < 1000; i++ {
		user := &models.TestUser{}
		err := db.Model(&models.TestUser{}).
			Where("ID", "=", fmt.Sprintf("bench-user-%d", i%100)).
			First(user)
		if err != nil {
			b.Fatal(err)
		}
	}
	dynamormTime := time.Since(start)

	// Benchmark raw SDK
	start = time.Now()
	for i := 0; i < 1000; i++ {
		output, err := client.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(benchTableName),
			Key: map[string]types.AttributeValue{
				"ID": &types.AttributeValueMemberS{Value: fmt.Sprintf("bench-user-%d", i%100)},
			},
		})
		if err != nil {
			b.Fatal(err)
		}
		if output.Item == nil {
			b.Fatal("Item not found")
		}
	}
	sdkTime := time.Since(start)

	overhead := calculateOverhead(dynamormTime, sdkTime)
	b.Logf("DynamORM time: %v, SDK time: %v, Overhead: %.2f%%", dynamormTime, sdkTime, overhead)

	if overhead > 5.0 {
		b.Errorf("Overhead %.2f%% exceeds 5%% target", overhead)
	}
}

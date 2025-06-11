// Package integration contains integration tests for DynamORM
package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUser is a simple model for testing
type TestUser struct {
	ID        string `dynamorm:"pk"`
	Email     string `dynamorm:"index:gsi-email"`
	Name      string
	Active    bool
	CreatedAt time.Time `dynamorm:"created_at"`
	UpdatedAt time.Time `dynamorm:"updated_at"`
}

// TestBasicOperations tests the core CRUD operations
func TestBasicOperations(t *testing.T) {
	// Skip if integration tests are disabled
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Integration tests disabled")
	}

	// Initialize DynamORM with correct pattern
	db := initTestDB(t)
	defer db.Close()

	// Debug: Let's check if the DB was created properly
	t.Logf("DB created: %v", db != nil)

	// Create table
	t.Log("Attempting to create table...")
	err := db.CreateTable(&TestUser{})
	if err != nil && !isTableExistsError(err) {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Wait for table to be ready
	waitForTable(t, "TestUsers")

	t.Run("Create", func(t *testing.T) {
		user := &TestUser{
			ID:     "test-user-1",
			Email:  "test@example.com",
			Name:   "Test User",
			Active: true,
		}

		err := db.Model(user).Create()
		assert.NoError(t, err)
		assert.NotZero(t, user.CreatedAt)
		assert.NotZero(t, user.UpdatedAt)
	})

	t.Run("Query", func(t *testing.T) {
		var user TestUser
		err := db.Model(&TestUser{}).
			Where("ID", "=", "test-user-1").
			First(&user)

		assert.NoError(t, err)
		assert.Equal(t, "test-user-1", user.ID)
		assert.Equal(t, "test@example.com", user.Email)
		assert.Equal(t, "Test User", user.Name)
		assert.True(t, user.Active)
	})

	t.Run("Update", func(t *testing.T) {
		user := &TestUser{
			ID:     "test-user-1",
			Name:   "Updated Name",
			Active: false,
		}

		err := db.Model(user).
			Where("ID", "=", "test-user-1").
			Update("Name", "Active")

		assert.NoError(t, err)

		// Verify update
		var updated TestUser
		err = db.Model(&TestUser{}).
			Where("ID", "=", "test-user-1").
			First(&updated)

		assert.NoError(t, err)
		assert.Equal(t, "Updated Name", updated.Name)
		assert.False(t, updated.Active)
		assert.Equal(t, "test@example.com", updated.Email) // Unchanged
	})

	t.Run("Delete", func(t *testing.T) {
		err := db.Model(&TestUser{}).
			Where("ID", "=", "test-user-1").
			Delete()

		assert.NoError(t, err)

		// Verify deletion
		var deleted TestUser
		err = db.Model(&TestUser{}).
			Where("ID", "=", "test-user-1").
			First(&deleted)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestNilPointerScenarios specifically tests scenarios that might cause nil pointer dereference
func TestNilPointerScenarios(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Integration tests disabled")
	}

	t.Run("MinimalConfig", func(t *testing.T) {
		// Test with minimal config that was causing issues
		sessionConfig := session.Config{
			Region: "us-east-1",
		}

		// This should fail without proper AWS setup
		_, err := dynamorm.New(sessionConfig)
		// Should create DB without error (but operations will fail without AWS)
		assert.NoError(t, err)
	})

	t.Run("LocalConfig", func(t *testing.T) {
		// Test with local config
		sessionConfig := session.Config{
			Region:   "us-east-1",
			Endpoint: "http://localhost:8000",
			AWSConfigOptions: []func(*config.LoadOptions) error{
				config.WithCredentialsProvider(
					credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
				),
			},
		}

		db, err := dynamorm.New(sessionConfig)
		require.NoError(t, err)
		assert.NotNil(t, db)

		// Create a query - this shouldn't panic
		query := db.Model(&TestUser{})
		assert.NotNil(t, query)
	})
}

// Helper functions

func initTestDB(t *testing.T) core.ExtendedDB {
	t.Helper()

	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
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

	return db
}

func waitForTable(t *testing.T, tableName string) {
	t.Helper()

	// Create a DynamoDB client for checking table status
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
		),
	)
	require.NoError(t, err)

	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = &endpoint
	})

	// Wait up to 30 seconds for table to be active
	ctx := context.TODO()
	for i := 0; i < 30; i++ {
		resp, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: &tableName,
		})

		if err == nil && resp.Table != nil && resp.Table.TableStatus == "ACTIVE" {
			return
		}

		time.Sleep(1 * time.Second)
	}

	t.Fatalf("Table %s did not become active", tableName)
}

func isTableExistsError(err error) bool {
	return err != nil &&
		(contains(err.Error(), "ResourceInUseException") ||
			contains(err.Error(), "already exists"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[0:len(substr)] == substr ||
		len(s) > len(substr) && contains(s[1:], substr)
}

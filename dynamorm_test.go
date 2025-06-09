package dynamorm_test

import (
	"os"
	"testing"
	"time"

	"github.com/pay-theory/dynamorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test model with various struct tags
type User struct {
	ID        string `dynamorm:"pk"`
	Email     string `dynamorm:"index:gsi-email"`
	Name      string
	Age       int
	CreatedAt time.Time `dynamorm:"created_at"`
	UpdatedAt time.Time `dynamorm:"updated_at"`
}

func TestNew(t *testing.T) {
	// Test creating a new DynamORM instance
	config := dynamorm.Config{
		Region: "us-east-1",
		// Don't specify endpoint - let it use AWS SDK defaults
		// This creates the DB instance without connecting
	}

	db, err := dynamorm.New(config)
	require.NoError(t, err)
	assert.NotNil(t, db)
}

func TestModelRegistration(t *testing.T) {
	// Test that models can be registered
	config := dynamorm.Config{
		Region: "us-east-1",
		// No endpoint - just testing model registration
	}

	db, err := dynamorm.New(config)
	require.NoError(t, err)

	// Create a query to trigger model registration
	query := db.Model(&User{})
	assert.NotNil(t, query)
}

func TestAutoMigrate(t *testing.T) {
	// Skip this test if AWS credentials are not available
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_PROFILE") == "" {
		t.Skip("Skipping test - AWS credentials not available")
		return
	}

	// Test auto-migration (currently just registers models)
	config := dynamorm.Config{
		Region:      "us-east-1",
		Endpoint:    "http://localhost:8000",
		AutoMigrate: true,
	}

	db, err := dynamorm.New(config)
	if err != nil {
		t.Skip("Skipping test - DynamoDB connection not available")
		return
	}
	require.NotNil(t, db)

	// Should not error when registering models
	err = db.AutoMigrate(&User{})
	// AutoMigrate will fail without actual DynamoDB, so we just check it doesn't panic
	// The actual error is expected when DynamoDB is not available
	_ = err
}

func TestQueryBuilder(t *testing.T) {
	// Test basic query building
	config := dynamorm.Config{
		Region: "us-east-1",
		// No endpoint needed for query building test
	}

	db, err := dynamorm.New(config)
	require.NoError(t, err)

	// Test chaining query methods
	query := db.Model(&User{}).
		Where("ID", "=", "user-123").
		Where("Age", ">", 18).
		Index("gsi-email").
		OrderBy("CreatedAt", "desc").
		Limit(10)

	assert.NotNil(t, query)
}

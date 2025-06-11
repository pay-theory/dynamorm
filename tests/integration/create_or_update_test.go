package integration

import (
	"errors"
	"testing"
	"time"

	"github.com/pay-theory/dynamorm"
	customerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestItem for testing create operations
type TestItem struct {
	ID        string    `dynamorm:"pk"`
	SK        string    `dynamorm:"sk"`
	Name      string    `json:"name"`
	Value     int       `json:"value"`
	Version   int64     `dynamorm:"version"`
	CreatedAt time.Time `dynamorm:"created_at"`
	UpdatedAt time.Time `dynamorm:"updated_at"`
}

func (t *TestItem) SetKeys() {
	// Set composite key if needed
}

func TestCreateOrUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Initialize DynamORM with local DynamoDB
	db, err := dynamorm.New(dynamorm.Config{
		Region:   "us-east-1",
		Endpoint: "http://127.0.0.1:8000",
	})
	require.NoError(t, err)

	// Create table
	err = db.CreateTable(&TestItem{})
	require.NoError(t, err)
	defer db.DeleteTable(&TestItem{})

	t.Run("Create with duplicate key shows helpful error", func(t *testing.T) {
		// Create first item
		item1 := &TestItem{
			ID:    "test-id",
			SK:    "test-sk",
			Name:  "First Item",
			Value: 100,
		}
		err := db.Model(item1).Create()
		require.NoError(t, err)

		// Try to create duplicate - should fail with helpful error
		item2 := &TestItem{
			ID:    "test-id",
			SK:    "test-sk",
			Name:  "Second Item",
			Value: 200,
		}
		err = db.Model(item2).Create()
		require.Error(t, err)
		// The error should wrap ErrConditionFailed
		assert.True(t, errors.Is(err, customerrors.ErrConditionFailed))
		// And should contain the helpful message
		assert.Contains(t, err.Error(), "item with the same key already exists")
	})

	t.Run("CreateOrUpdate works for new item", func(t *testing.T) {
		// Create new item using CreateOrUpdate
		item := &TestItem{
			ID:    "upsert-id",
			SK:    "upsert-sk",
			Name:  "New Item",
			Value: 300,
		}
		err := db.Model(item).CreateOrUpdate()
		require.NoError(t, err)
		assert.NotZero(t, item.CreatedAt)
		assert.NotZero(t, item.UpdatedAt)
		assert.Equal(t, int64(0), item.Version) // Version starts at 0

		// Verify item was created
		var retrieved TestItem
		err = db.Model(&TestItem{}).
			Where("ID", "=", "upsert-id").
			Where("SK", "=", "upsert-sk").
			First(&retrieved)
		require.NoError(t, err)
		assert.Equal(t, "New Item", retrieved.Name)
		assert.Equal(t, 300, retrieved.Value)
	})

	t.Run("CreateOrUpdate overwrites existing item", func(t *testing.T) {
		// First create an item
		original := &TestItem{
			ID:      "overwrite-id",
			SK:      "overwrite-sk",
			Name:    "Original Item",
			Value:   400,
			Version: 5, // Set a specific version
		}
		err := db.Model(original).CreateOrUpdate()
		require.NoError(t, err)
		originalCreatedAt := original.CreatedAt

		// Wait a bit to ensure timestamps are different
		time.Sleep(10 * time.Millisecond)

		// Overwrite with new data
		updated := &TestItem{
			ID:      "overwrite-id",
			SK:      "overwrite-sk",
			Name:    "Updated Item",
			Value:   500,
			Version: 10, // Different version - should be overwritten
		}
		err = db.Model(updated).CreateOrUpdate()
		require.NoError(t, err)

		// Verify item was overwritten
		var retrieved TestItem
		err = db.Model(&TestItem{}).
			Where("ID", "=", "overwrite-id").
			Where("SK", "=", "overwrite-sk").
			First(&retrieved)
		require.NoError(t, err)
		assert.Equal(t, "Updated Item", retrieved.Name)
		assert.Equal(t, 500, retrieved.Value)
		assert.Equal(t, int64(10), retrieved.Version) // Version was overwritten

		// CreatedAt should be updated (since it's a full overwrite)
		assert.NotEqual(t, originalCreatedAt, retrieved.CreatedAt)
	})

	t.Run("Create with version field initializes to 0", func(t *testing.T) {
		item := &TestItem{
			ID:   "version-test",
			SK:   "version-sk",
			Name: "Version Test",
		}
		// Don't set version explicitly
		err := db.Model(item).Create()
		require.NoError(t, err)
		assert.Equal(t, int64(0), item.Version)

		// Verify in database
		var retrieved TestItem
		err = db.Model(&TestItem{}).
			Where("ID", "=", "version-test").
			Where("SK", "=", "version-sk").
			First(&retrieved)
		require.NoError(t, err)
		assert.Equal(t, int64(0), retrieved.Version)
	})

	t.Run("Create vs CreateOrUpdate behavior", func(t *testing.T) {
		// Create an item
		item := &TestItem{
			ID:    "behavior-test",
			SK:    "behavior-sk",
			Name:  "Test Item",
			Value: 600,
		}
		err := db.Model(item).Create()
		require.NoError(t, err)

		// Try Create again - should fail
		item2 := &TestItem{
			ID:    "behavior-test",
			SK:    "behavior-sk",
			Name:  "Updated Name",
			Value: 700,
		}
		err = db.Model(item2).Create()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")

		// CreateOrUpdate should succeed
		err = db.Model(item2).CreateOrUpdate()
		require.NoError(t, err)

		// Verify the update
		var retrieved TestItem
		err = db.Model(&TestItem{}).
			Where("ID", "=", "behavior-test").
			Where("SK", "=", "behavior-sk").
			First(&retrieved)
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", retrieved.Name)
		assert.Equal(t, 700, retrieved.Value)
	})
}

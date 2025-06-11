package integration

import (
	"testing"
	"time"

	"github.com/pay-theory/dynamorm"
	"github.com/stretchr/testify/require"
)

// TestUpdateBuilder_OverlappingPaths tests that the UpdateBuilder correctly handles
// multiple SetIfNotExists operations combined with Set operations without creating
// overlapping document paths
func TestUpdateBuilder_OverlappingPaths(t *testing.T) {
	// Skip if not integration test
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Initialize DynamORM with local DynamoDB
	db, err := dynamorm.New(dynamorm.Config{
		Region:   "us-east-1",
		Endpoint: "http://127.0.0.1:8000",
	})
	require.NoError(t, err)

	// Define test model
	type RateLimitEntry struct {
		PartnerID   string `dynamorm:"pk"`
		WindowStart int64  `dynamorm:"sk"`
		WindowID    string
		Endpoint    string
		Count       int64
		CreatedAt   time.Time
		UpdatedAt   time.Time
		TTL         int64
	}

	// Create table
	err = db.CreateTable(&RateLimitEntry{})
	require.NoError(t, err)

	// Test data
	partnerID := "partner-001"
	windowStart := int64(1749676440)
	windowID := "window-123"
	endpoint := "test-endpoint"
	now := time.Now()
	ttl := now.Add(24 * time.Hour).Unix()

	// Create entry for testing
	entry := &RateLimitEntry{
		PartnerID:   partnerID,
		WindowStart: windowStart,
	}

	// Test the problematic update pattern
	t.Run("SetIfNotExists with Set should not overlap", func(t *testing.T) {
		// This should work without overlapping paths error
		err := db.Model(entry).
			Where("PartnerID", "=", partnerID).
			Where("WindowStart", "=", windowStart).
			UpdateBuilder().
			Add("Count", 1).
			SetIfNotExists("WindowID", windowID, windowID).
			SetIfNotExists("CreatedAt", now, now).
			SetIfNotExists("Endpoint", endpoint, endpoint).
			SetIfNotExists("TTL", ttl, ttl).
			Set("UpdatedAt", now).
			Execute()

		// Should succeed without validation error
		require.NoError(t, err)
	})

	// Verify the entry was created/updated correctly
	t.Run("Verify entry contents", func(t *testing.T) {
		var result RateLimitEntry
		err := db.Model(&RateLimitEntry{}).
			Where("PartnerID", "=", partnerID).
			Where("WindowStart", "=", windowStart).
			First(&result)

		require.NoError(t, err)
		require.Equal(t, int64(1), result.Count)
		require.Equal(t, windowID, result.WindowID)
		require.Equal(t, endpoint, result.Endpoint)
		require.Equal(t, ttl, result.TTL)
		require.NotZero(t, result.CreatedAt)
		require.NotZero(t, result.UpdatedAt)
	})

	// Test multiple updates to ensure SetIfNotExists works correctly
	t.Run("SetIfNotExists should not overwrite existing values", func(t *testing.T) {
		// Update again with different values for SetIfNotExists
		newWindowID := "window-456"
		newEndpoint := "different-endpoint"
		newNow := time.Now().Add(1 * time.Hour)

		err := db.Model(entry).
			Where("PartnerID", "=", partnerID).
			Where("WindowStart", "=", windowStart).
			UpdateBuilder().
			Add("Count", 1).
			SetIfNotExists("WindowID", newWindowID, newWindowID). // Should not change
			SetIfNotExists("Endpoint", newEndpoint, newEndpoint). // Should not change
			Set("UpdatedAt", newNow).                             // Should change
			Execute()

		require.NoError(t, err)

		// Verify values
		var result RateLimitEntry
		err = db.Model(&RateLimitEntry{}).
			Where("PartnerID", "=", partnerID).
			Where("WindowStart", "=", windowStart).
			First(&result)

		require.NoError(t, err)
		require.Equal(t, int64(2), result.Count)                 // Should increment
		require.Equal(t, windowID, result.WindowID)              // Should not change
		require.Equal(t, endpoint, result.Endpoint)              // Should not change
		require.Equal(t, newNow.Unix(), result.UpdatedAt.Unix()) // Should update
	})

	// Clean up
	t.Cleanup(func() {
		_ = db.DeleteTable(&RateLimitEntry{})
	})
}

// TestUpdateBuilder_MixedOperations tests various combinations of update operations
func TestUpdateBuilder_MixedOperations(t *testing.T) {
	// Skip if not integration test
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Initialize DynamORM with local DynamoDB
	db, err := dynamorm.New(dynamorm.Config{
		Region:   "us-east-1",
		Endpoint: "http://127.0.0.1:8000",
	})
	require.NoError(t, err)

	// Define test model
	type TestRecord struct {
		PK        string `dynamorm:"pk"`
		SK        string `dynamorm:"sk"`
		Count     int
		CreatedAt time.Time
		UpdatedAt time.Time
		Tags      []string
	}

	// Create table
	err = db.CreateTable(&TestRecord{})
	require.NoError(t, err)

	// Test combining ADD, SET, and SetIfNotExists
	t.Run("Mixed operations should work without conflicts", func(t *testing.T) {
		record := &TestRecord{
			PK: "test#123",
			SK: "record",
		}

		now := time.Now()

		// This should work without overlapping paths
		err := db.Model(record).
			Where("PK", "=", "test#123").
			Where("SK", "=", "record").
			UpdateBuilder().
			Add("Count", 1).
			SetIfNotExists("CreatedAt", now, now).
			Set("UpdatedAt", now).
			Execute()

		require.NoError(t, err)

		// Verify the record
		var result TestRecord
		err = db.Model(&TestRecord{}).
			Where("PK", "=", "test#123").
			Where("SK", "=", "record").
			First(&result)

		require.NoError(t, err)
		require.Equal(t, 1, result.Count)
		require.NotZero(t, result.CreatedAt)
		require.NotZero(t, result.UpdatedAt)
	})

	// Clean up
	t.Cleanup(func() {
		_ = db.DeleteTable(&TestRecord{})
	})
}

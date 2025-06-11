package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateBuilderOrCondition tests the new OrCondition functionality
func TestUpdateBuilderOrCondition(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Create test table
	type TestItem struct {
		ID       string `dynamorm:"pk"`
		Status   string `json:"status"`
		Priority string `json:"priority"`
		Count    int    `json:"count"`
		Region   string `json:"region"`
	}

	err := db.CreateTable(&TestItem{})
	require.NoError(t, err)

	// Test 1: Simple OR condition - should succeed
	t.Run("Simple OR condition success", func(t *testing.T) {
		// Create test item
		item := &TestItem{
			ID:       "or-test-1",
			Status:   "pending",
			Priority: "low",
			Count:    5,
			Region:   "US",
		}
		err := db.Model(item).Create()
		require.NoError(t, err)

		// Update with OR condition (status = 'pending' OR priority = 'high')
		err = db.Model(&TestItem{ID: "or-test-1"}).
			Where("ID", "=", "or-test-1").
			UpdateBuilder().
			Set("Count", 10).
			Condition("Status", "=", "pending").  // This is true
			OrCondition("Priority", "=", "high"). // This is false
			Execute()

		assert.NoError(t, err, "Update should succeed because status = pending")

		// Verify update
		var result TestItem
		err = db.Model(&TestItem{}).Where("ID", "=", "or-test-1").First(&result)
		assert.NoError(t, err)
		assert.Equal(t, 10, result.Count)
	})

	// Test 2: Simple OR condition - should fail
	t.Run("Simple OR condition failure", func(t *testing.T) {
		// Create test item
		item := &TestItem{
			ID:       "or-test-2",
			Status:   "completed",
			Priority: "low",
			Count:    5,
			Region:   "US",
		}
		err := db.Model(item).Create()
		require.NoError(t, err)

		// Update with OR condition (status = 'pending' OR priority = 'high')
		err = db.Model(&TestItem{ID: "or-test-2"}).
			Where("ID", "=", "or-test-2").
			UpdateBuilder().
			Set("Count", 10).
			Condition("Status", "=", "pending").  // This is false
			OrCondition("Priority", "=", "high"). // This is false
			Execute()

		assert.Error(t, err, "Update should fail because neither condition is met")

		// Verify no update occurred
		var result TestItem
		err = db.Model(&TestItem{}).Where("ID", "=", "or-test-2").First(&result)
		assert.NoError(t, err)
		assert.Equal(t, 5, result.Count) // Should still be 5
	})

	// Test 3: Rate limiting scenario with OR conditions
	t.Run("Rate limiting with OR conditions", func(t *testing.T) {
		// Create test item for rate limiting
		item := &TestItem{
			ID:       "or-test-rate-limit",
			Status:   "active",
			Priority: "normal",
			Count:    98,
			Region:   "US",
		}
		err := db.Model(item).Create()
		require.NoError(t, err)

		// First increment should succeed (under limit)
		err = db.Model(&TestItem{}).
			Where("ID", "=", "or-test-rate-limit").
			UpdateBuilder().
			Add("Count", 1).
			Condition("Count", "<", 100).            // True (98 < 100)
			OrCondition("Priority", "=", "premium"). // False
			Execute()

		assert.NoError(t, err)

		// Second increment should succeed (still under limit)
		err = db.Model(&TestItem{}).
			Where("ID", "=", "or-test-rate-limit").
			UpdateBuilder().
			Add("Count", 1).
			Condition("Count", "<", 100).            // True (99 < 100)
			OrCondition("Priority", "=", "premium"). // False
			Execute()

		assert.NoError(t, err)

		// Third increment should fail (at limit)
		err = db.Model(&TestItem{}).
			Where("ID", "=", "or-test-rate-limit").
			UpdateBuilder().
			Add("Count", 1).
			Condition("Count", "<", 100).            // False (100 < 100)
			OrCondition("Priority", "=", "premium"). // False
			Execute()

		assert.Error(t, err, "Should fail - at limit and not premium")

		// Make user premium
		err = db.Model(&TestItem{}).
			Where("ID", "=", "or-test-rate-limit").
			UpdateBuilder().
			Set("Priority", "premium").
			Execute()
		require.NoError(t, err)

		// Now increment should work despite being at limit
		err = db.Model(&TestItem{}).
			Where("ID", "=", "or-test-rate-limit").
			UpdateBuilder().
			Add("Count", 1).
			Condition("Count", "<", 100).            // False
			OrCondition("Priority", "=", "premium"). // True now
			Execute()

		assert.NoError(t, err, "Should succeed - user is premium")

		// Verify final count
		var result TestItem
		err = db.Model(&TestItem{}).Where("ID", "=", "or-test-rate-limit").First(&result)
		assert.NoError(t, err)
		assert.Equal(t, 101, result.Count)
		assert.Equal(t, "premium", result.Priority)
	})

	// Test 4: Mixed AND/OR conditions
	t.Run("Mixed AND/OR conditions", func(t *testing.T) {
		// Create test item
		item := &TestItem{
			ID:       "or-test-mixed",
			Status:   "pending",
			Priority: "medium",
			Count:    5,
			Region:   "EU",
		}
		err := db.Model(item).Create()
		require.NoError(t, err)

		// Update with: (status = 'pending' AND region = 'US') OR priority = 'urgent'
		// Should fail: (true AND false) OR false = false
		err = db.Model(&TestItem{}).
			Where("ID", "=", "or-test-mixed").
			UpdateBuilder().
			Set("Count", 20).
			Condition("Status", "=", "pending").    // True
			Condition("Region", "=", "US").         // AND False
			OrCondition("Priority", "=", "urgent"). // OR False
			Execute()

		assert.Error(t, err)

		// Make priority urgent
		err = db.Model(&TestItem{}).
			Where("ID", "=", "or-test-mixed").
			UpdateBuilder().
			Set("Priority", "urgent").
			Execute()
		require.NoError(t, err)

		// Now same update should succeed
		// (true AND false) OR true = true
		err = db.Model(&TestItem{}).
			Where("ID", "=", "or-test-mixed").
			UpdateBuilder().
			Set("Count", 20).
			Condition("Status", "=", "pending").    // True
			Condition("Region", "=", "US").         // AND False
			OrCondition("Priority", "=", "urgent"). // OR True
			Execute()

		assert.NoError(t, err)

		// Verify update
		var result TestItem
		err = db.Model(&TestItem{}).Where("ID", "=", "or-test-mixed").First(&result)
		assert.NoError(t, err)
		assert.Equal(t, 20, result.Count)
		assert.Equal(t, "urgent", result.Priority)
	})
}

// TestUpdateBuilderOrConditionEdgeCases tests edge cases for OR conditions
func TestUpdateBuilderOrConditionEdgeCases(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	type TestItem struct {
		ID          string    `dynamorm:"pk"`
		Status      string    `json:"status"`
		Description string    `json:"description,omitempty"`
		Count       int       `json:"count"`
		UpdatedAt   time.Time `dynamorm:"updated_at"`
	}

	err := db.CreateTable(&TestItem{})
	require.NoError(t, err)

	// Test: OR with attribute existence
	t.Run("OR with attribute existence", func(t *testing.T) {
		// Create item without Description
		item := &TestItem{
			ID:     "or-edge-1",
			Status: "new",
			Count:  0,
		}
		err := db.Model(item).Create()
		require.NoError(t, err)

		// Update if Description doesn't exist OR status = 'retry'
		err = db.Model(&TestItem{}).
			Where("ID", "=", "or-edge-1").
			UpdateBuilder().
			Set("Count", 50).
			ConditionNotExists("Description").   // True
			OrCondition("Status", "=", "retry"). // False
			Execute()

		assert.NoError(t, err, "Should succeed - Description doesn't exist")

		// Add Description
		err = db.Model(&TestItem{}).
			Where("ID", "=", "or-edge-1").
			UpdateBuilder().
			Set("Description", "Test description").
			Execute()
		require.NoError(t, err)

		// Same update should now fail
		err = db.Model(&TestItem{}).
			Where("ID", "=", "or-edge-1").
			UpdateBuilder().
			Set("Count", 100).
			ConditionNotExists("Description").   // False now
			OrCondition("Status", "=", "retry"). // Still false
			Execute()

		assert.Error(t, err, "Should fail - Description exists and status != retry")
	})

	// Test: All OR conditions
	t.Run("All OR conditions", func(t *testing.T) {
		item := &TestItem{
			ID:     "or-edge-all",
			Status: "inactive",
			Count:  50,
		}
		err := db.Model(item).Create()
		require.NoError(t, err)

		// Update with multiple OR conditions - at least one should match
		err = db.Model(&TestItem{}).
			Where("ID", "=", "or-edge-all").
			UpdateBuilder().
			Set("Status", "processed").
			OrCondition("Count", "<", 10).          // False
			OrCondition("Count", ">", 100).         // False
			OrCondition("Status", "=", "active").   // False
			OrCondition("Status", "=", "inactive"). // True
			Execute()

		assert.NoError(t, err, "Should succeed - one condition matches")
	})

	// Test: No conditions (should succeed)
	t.Run("No conditions", func(t *testing.T) {
		item := &TestItem{
			ID:     "or-edge-none",
			Status: "test",
			Count:  1,
		}
		err := db.Model(item).Create()
		require.NoError(t, err)

		// Update with no conditions
		err = db.Model(&TestItem{}).
			Where("ID", "=", "or-edge-none").
			UpdateBuilder().
			Set("Count", 999).
			Execute()

		assert.NoError(t, err, "Should succeed - no conditions")

		// Verify
		var result TestItem
		err = db.Model(&TestItem{}).Where("ID", "=", "or-edge-none").First(&result)
		assert.NoError(t, err)
		assert.Equal(t, 999, result.Count)
	})
}

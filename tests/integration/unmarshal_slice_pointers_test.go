package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModel for testing slice of pointers
type TestModel struct {
	PK        string    `dynamorm:"pk"`
	SK        string    `dynamorm:"sk"`
	Name      string    `json:"name"`
	Version   int64     `dynamorm:"version"`
	CreatedAt time.Time `dynamorm:"created_at"`
	UpdatedAt time.Time `dynamorm:"updated_at"`
}

func (t *TestModel) SetKeys() {
	// Set composite key if needed
}

func TestUnmarshalSliceOfPointers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Initialize test context with automatic cleanup
	testCtx := InitTestDB(t)

	// Create table with automatic cleanup
	testCtx.CreateTableIfNotExists(t, &TestModel{})

	// Create test data
	items := []TestModel{
		{PK: "test", SK: "item1", Name: "First Item"},
		{PK: "test", SK: "item2", Name: "Second Item"},
		{PK: "test", SK: "item3", Name: "Third Item"},
	}

	// Insert test data
	for _, item := range items {
		err := testCtx.DB.Model(&item).Create()
		require.NoError(t, err)
	}

	// Test 1: All() with slice of pointers - THIS SHOULD NOW WORK!
	t.Run("All with slice of pointers", func(t *testing.T) {
		var results []*TestModel // Slice of pointers
		err := testCtx.DB.Model(&TestModel{}).
			Where("PK", "=", "test").
			All(&results)

		require.NoError(t, err)
		assert.Len(t, results, 3)

		// Verify all items are properly unmarshaled
		for _, result := range results {
			assert.NotNil(t, result)
			assert.Equal(t, "test", result.PK)
			assert.Contains(t, []string{"item1", "item2", "item3"}, result.SK)
			assert.NotEmpty(t, result.Name)
			assert.NotZero(t, result.CreatedAt)
			assert.NotZero(t, result.UpdatedAt)
		}
	})

	// Test 2: All() with slice of values - should still work
	t.Run("All with slice of values", func(t *testing.T) {
		var results []TestModel // Slice of values
		err := testCtx.DB.Model(&TestModel{}).
			Where("PK", "=", "test").
			All(&results)

		require.NoError(t, err)
		assert.Len(t, results, 3)

		// Verify all items are properly unmarshaled
		for _, result := range results {
			assert.Equal(t, "test", result.PK)
			assert.Contains(t, []string{"item1", "item2", "item3"}, result.SK)
			assert.NotEmpty(t, result.Name)
		}
	})

	// Test 3: Scan() with slice of pointers
	t.Run("Scan with slice of pointers", func(t *testing.T) {
		var results []*TestModel
		err := testCtx.DB.Model(&TestModel{}).
			Filter("PK", "=", "test").
			Scan(&results)

		require.NoError(t, err)
		assert.Len(t, results, 3)

		for _, result := range results {
			assert.NotNil(t, result)
			assert.Equal(t, "test", result.PK)
		}
	})

	// Test 4: BatchGet() with slice of pointers
	t.Run("BatchGet with slice of pointers", func(t *testing.T) {
		keys := []any{
			&TestModel{PK: "test", SK: "item1"},
			&TestModel{PK: "test", SK: "item2"},
		}

		var results []*TestModel
		err := testCtx.DB.Model(&TestModel{}).BatchGet(keys, &results)

		require.NoError(t, err)
		assert.Len(t, results, 2)

		for _, result := range results {
			assert.NotNil(t, result)
			assert.Equal(t, "test", result.PK)
			assert.Contains(t, []string{"item1", "item2"}, result.SK)
		}
	})

	// Test 5: Empty results
	t.Run("Empty results with slice of pointers", func(t *testing.T) {
		var results []*TestModel
		err := testCtx.DB.Model(&TestModel{}).
			Where("PK", "=", "nonexistent").
			All(&results)

		require.NoError(t, err)
		assert.Len(t, results, 0)
	})
}

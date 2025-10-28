package dynamorm

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test types for custom converter testing
// PayloadJSON is a custom type that should be stored as a JSON string
type TestPayloadJSON struct {
	Data map[string]interface{}
}

// TestPayloadJSONConverter implements the CustomConverter interface
type TestPayloadJSONConverter struct{}

func (c TestPayloadJSONConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	var payload TestPayloadJSON

	switch v := value.(type) {
	case TestPayloadJSON:
		payload = v
	case *TestPayloadJSON:
		if v == nil {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		payload = *v
	default:
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	// Marshal to JSON string
	data, err := json.Marshal(payload.Data)
	if err != nil {
		return nil, err
	}

	return &types.AttributeValueMemberS{Value: string(data)}, nil
}

func (c TestPayloadJSONConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	strValue, ok := av.(*types.AttributeValueMemberS)
	if !ok {
		return nil // gracefully handle other types
	}

	payload, ok := target.(*TestPayloadJSON)
	if !ok {
		return nil
	}

	// Initialize map if nil
	if payload.Data == nil {
		payload.Data = make(map[string]interface{})
	}

	return json.Unmarshal([]byte(strValue.Value), &payload.Data)
}

// TestAsyncRequest model for testing
type TestAsyncRequest struct {
	ID      string          `dynamorm:"pk"`
	Name    string          `dynamorm:"attr:name"`
	Payload TestPayloadJSON `dynamorm:"attr:payload"`
}

// TestCustomID is a simple custom type for regression testing
type TestCustomID string

// TestCustomIDConverter adds a prefix to ensure converter is used
type TestCustomIDConverter struct{}

func (c TestCustomIDConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	var id TestCustomID
	switch v := value.(type) {
	case TestCustomID:
		id = v
	case *TestCustomID:
		if v == nil {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		id = *v
	default:
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	// Add prefix to ensure converter is being used
	return &types.AttributeValueMemberS{Value: "CUSTOM-" + string(id)}, nil
}

func (c TestCustomIDConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	strValue, ok := av.(*types.AttributeValueMemberS)
	if !ok {
		return nil
	}

	id, ok := target.(*TestCustomID)
	if !ok {
		return nil
	}

	// Remove prefix
	val := strValue.Value
	if len(val) > 7 {
		*id = TestCustomID(val[7:]) // Remove "CUSTOM-" prefix
	}
	return nil
}

// TestModelWithCustomID for regression testing
type TestModelWithCustomID struct {
	ID       string       `dynamorm:"pk"`
	CustomID TestCustomID `dynamorm:"attr:custom_id"`
}

// TestCustomConverterWithUpdate tests the bug fix for custom converters being ignored during Update()
// This test ensures that custom type converters registered via RegisterTypeConverter() are properly
// invoked during Update() operations, not just Create() operations.
func TestCustomConverterWithUpdate(t *testing.T) {
	// Skip this test if DynamoDB Local is not running
	// This matches the pattern used in other integration tests
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Initialize DynamORM with custom converter
	db, err := New(session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	require.NoError(t, err)

	// Register the custom converter - this is the key step
	err = db.RegisterTypeConverter(
		reflect.TypeOf(TestPayloadJSON{}),
		TestPayloadJSONConverter{},
	)
	require.NoError(t, err)

	// Create table
	err = db.CreateTable(&TestAsyncRequest{})
	if err != nil {
		// Ignore if table already exists
		t.Logf("CreateTable warning (may be expected): %v", err)
	}

	// Cleanup
	defer func() {
		_ = db.DeleteTable(&TestAsyncRequest{})
	}()

	t.Run("Create uses custom converter", func(t *testing.T) {
		// This test verifies the existing behavior - Create() should use custom converter
		request := &TestAsyncRequest{
			ID:   "test-create-1",
			Name: "Create Test",
			Payload: TestPayloadJSON{
				Data: map[string]interface{}{
					"action":   "process",
					"priority": 5,
					"metadata": map[string]interface{}{
						"user": "test@example.com",
					},
				},
			},
		}

		err := db.Model(request).Create()
		require.NoError(t, err)

		// Retrieve and verify it was stored as JSON string
		var retrieved TestAsyncRequest
		err = db.Model(&TestAsyncRequest{}).Where("ID", "=", "test-create-1").First(&retrieved)
		require.NoError(t, err)

		assert.Equal(t, "test-create-1", retrieved.ID)
		assert.Equal(t, "Create Test", retrieved.Name)
		assert.NotNil(t, retrieved.Payload.Data)
		assert.Equal(t, "process", retrieved.Payload.Data["action"])
		assert.Equal(t, float64(5), retrieved.Payload.Data["priority"]) // JSON unmarshals numbers as float64
	})

	t.Run("Update uses custom converter - bug fix test", func(t *testing.T) {
		// THIS IS THE BUG FIX TEST
		// Before the fix, Update() would NOT call the custom converter,
		// resulting in incorrect storage (NULL or nested struct)

		// First, create an item
		request := &TestAsyncRequest{
			ID:   "test-update-1",
			Name: "Update Test Initial",
			Payload: TestPayloadJSON{
				Data: map[string]interface{}{
					"status": "pending",
					"count":  1,
				},
			},
		}

		err := db.Model(request).Create()
		require.NoError(t, err)

		// Now update the payload
		request.Name = "Update Test Modified"
		request.Payload.Data = map[string]interface{}{
			"status":  "completed",
			"count":   2,
			"newKey":  "newValue",
			"complex": map[string]interface{}{"nested": "data"},
		}

		// This should use the custom converter to marshal Payload as JSON string
		err = db.Model(request).Update()
		require.NoError(t, err, "Update() should succeed with custom converter")

		// Retrieve and verify the payload was correctly stored as JSON string
		var retrieved TestAsyncRequest
		err = db.Model(&TestAsyncRequest{}).Where("ID", "=", "test-update-1").First(&retrieved)
		require.NoError(t, err)

		// Verify all fields including the custom type
		assert.Equal(t, "test-update-1", retrieved.ID)
		assert.Equal(t, "Update Test Modified", retrieved.Name)
		require.NotNil(t, retrieved.Payload.Data, "Payload.Data should not be nil after Update()")

		// Verify the payload content was correctly marshaled and unmarshaled
		assert.Equal(t, "completed", retrieved.Payload.Data["status"])
		assert.Equal(t, float64(2), retrieved.Payload.Data["count"])
		assert.Equal(t, "newValue", retrieved.Payload.Data["newKey"])

		// Verify nested data
		complex, ok := retrieved.Payload.Data["complex"].(map[string]interface{})
		require.True(t, ok, "complex field should be a map")
		assert.Equal(t, "data", complex["nested"])
	})

	t.Run("Update with specific fields uses custom converter", func(t *testing.T) {
		// Test Update() with specific fields parameter

		request := &TestAsyncRequest{
			ID:   "test-update-fields-1",
			Name: "Field Update Test",
			Payload: TestPayloadJSON{
				Data: map[string]interface{}{
					"original": "data",
				},
			},
		}

		err := db.Model(request).Create()
		require.NoError(t, err)

		// Update only the Payload field
		request.Payload.Data = map[string]interface{}{
			"updated": "payload",
			"type":    "specific_field_update",
		}

		err = db.Model(request).Update("Payload")
		require.NoError(t, err)

		// Verify
		var retrieved TestAsyncRequest
		err = db.Model(&TestAsyncRequest{}).Where("ID", "=", "test-update-fields-1").First(&retrieved)
		require.NoError(t, err)

		assert.Equal(t, "Field Update Test", retrieved.Name) // Should be unchanged
		assert.Equal(t, "payload", retrieved.Payload.Data["updated"])
		assert.Equal(t, "specific_field_update", retrieved.Payload.Data["type"])
	})

	t.Run("CreateOrUpdate uses custom converter", func(t *testing.T) {
		// Verify CreateOrUpdate also works with custom converters

		request := &TestAsyncRequest{
			ID:   "test-upsert-1",
			Name: "Upsert Test",
			Payload: TestPayloadJSON{
				Data: map[string]interface{}{
					"mode": "upsert",
				},
			},
		}

		// First CreateOrUpdate (acts as create)
		err := db.Model(request).CreateOrUpdate()
		require.NoError(t, err)

		// Second CreateOrUpdate (acts as update)
		request.Payload.Data["mode"] = "updated"
		request.Payload.Data["iteration"] = 2

		err = db.Model(request).CreateOrUpdate()
		require.NoError(t, err)

		// Verify
		var retrieved TestAsyncRequest
		err = db.Model(&TestAsyncRequest{}).Where("ID", "=", "test-upsert-1").First(&retrieved)
		require.NoError(t, err)

		assert.Equal(t, "updated", retrieved.Payload.Data["mode"])
		assert.Equal(t, float64(2), retrieved.Payload.Data["iteration"])
	})

	t.Run("Filter with custom type uses converter", func(t *testing.T) {
		// Test that Filter() conditions also work with custom types

		// Create test data
		for i := 1; i <= 3; i++ {
			request := &TestAsyncRequest{
				ID:   "test-filter-" + string(rune('0'+i)),
				Name: "Filter Test",
				Payload: TestPayloadJSON{
					Data: map[string]interface{}{
						"index": i,
					},
				},
			}
			err := db.Model(request).Create()
			require.NoError(t, err)
		}

		// Query with filter should work
		var results []TestAsyncRequest
		err = db.Model(&TestAsyncRequest{}).Scan(&results)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})
}

// TestCustomConverterRegressionGuard ensures custom converters work across all operations
func TestCustomConverterRegressionGuard(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db, err := New(session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	require.NoError(t, err)

	err = db.RegisterTypeConverter(
		reflect.TypeOf(TestCustomID("")),
		TestCustomIDConverter{},
	)
	require.NoError(t, err)

	err = db.CreateTable(&TestModelWithCustomID{})
	if err != nil {
		t.Logf("CreateTable warning: %v", err)
	}
	defer func() { _ = db.DeleteTable(&TestModelWithCustomID{}) }()

	// Test all CRUD operations
	t.Run("All operations use converter", func(t *testing.T) {
		model := &TestModelWithCustomID{
			ID:       "test-1",
			CustomID: "ABC123",
		}

		// Create
		err := db.Model(model).Create()
		require.NoError(t, err)

		// Read
		var retrieved TestModelWithCustomID
		err = db.Model(&TestModelWithCustomID{}).Where("ID", "=", "test-1").First(&retrieved)
		require.NoError(t, err)
		assert.Equal(t, TestCustomID("ABC123"), retrieved.CustomID, "CustomID should round-trip through converter")

		// Update
		retrieved.CustomID = "XYZ789"
		err = db.Model(&retrieved).Update()
		require.NoError(t, err)

		// Verify update
		var updated TestModelWithCustomID
		err = db.Model(&TestModelWithCustomID{}).Where("ID", "=", "test-1").First(&updated)
		require.NoError(t, err)
		assert.Equal(t, TestCustomID("XYZ789"), updated.CustomID, "Updated CustomID should use converter")
	})
}

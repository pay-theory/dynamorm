package dynamorm

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOrder represents a test model for stream processing
type TestOrder struct {
	PK         string   `dynamorm:"PK" dynamodb:"PK"`
	SK         string   `dynamorm:"SK" dynamodb:"SK"`
	OrderID    string   `dynamorm:"order_id" dynamodb:"order_id"`
	CustomerID string   `dynamorm:"customer_id" dynamodb:"customer_id"`
	Total      float64  `dynamorm:"total" dynamodb:"total"`
	Status     string   `dynamorm:"status" dynamodb:"status"`
	Items      []string `dynamorm:"items" dynamodb:"items"`
}

func TestUnmarshalStreamImage(t *testing.T) {
	// Create a mock DynamoDB stream image
	streamImage := map[string]events.DynamoDBAttributeValue{
		"PK": events.NewStringAttribute("ORDER#123"),
		"SK": events.NewStringAttribute("METADATA"),
		"order_id": events.NewStringAttribute("123"),
		"customer_id": events.NewStringAttribute("CUST456"),
		"total": events.NewNumberAttribute("99.99"),
		"status": events.NewStringAttribute("pending"),
		"items": events.NewListAttribute([]events.DynamoDBAttributeValue{
			events.NewStringAttribute("ITEM1"),
			events.NewStringAttribute("ITEM2"),
		}),
	}

	var order TestOrder
	err := UnmarshalStreamImage(streamImage, &order)
	require.NoError(t, err)

	assert.Equal(t, "ORDER#123", order.PK)
	assert.Equal(t, "METADATA", order.SK)
	assert.Equal(t, "123", order.OrderID)
	assert.Equal(t, "CUST456", order.CustomerID)
	assert.Equal(t, 99.99, order.Total)
	assert.Equal(t, "pending", order.Status)
	assert.Equal(t, []string{"ITEM1", "ITEM2"}, order.Items)
}

func TestUnmarshalStreamImage_ComplexTypes(t *testing.T) {
	// Test individual conversions to ensure all types are handled
	assert.NotNil(t, convertLambdaAttributeValue(events.NewStringAttribute("test")))
	assert.NotNil(t, convertLambdaAttributeValue(events.NewNumberAttribute("123")))
	assert.NotNil(t, convertLambdaAttributeValue(events.NewBooleanAttribute(true)))
	assert.NotNil(t, convertLambdaAttributeValue(events.NewNullAttribute()))
	assert.NotNil(t, convertLambdaAttributeValue(events.NewBinaryAttribute([]byte("data"))))
	
	// Test complex types
	listAttr := events.NewListAttribute([]events.DynamoDBAttributeValue{
		events.NewStringAttribute("item1"),
		events.NewNumberAttribute("42"),
	})
	assert.NotNil(t, convertLambdaAttributeValue(listAttr))
	
	mapAttr := events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
		"key": events.NewStringAttribute("value"),
	})
	assert.NotNil(t, convertLambdaAttributeValue(mapAttr))
	
	// Test set types
	assert.NotNil(t, convertLambdaAttributeValue(events.NewStringSetAttribute([]string{"a", "b"})))
	assert.NotNil(t, convertLambdaAttributeValue(events.NewNumberSetAttribute([]string{"1", "2"})))
	assert.NotNil(t, convertLambdaAttributeValue(events.NewBinarySetAttribute([][]byte{[]byte("data1"), []byte("data2")})))
}

func TestUnmarshalStreamImage_EmptyImage(t *testing.T) {
	streamImage := make(map[string]events.DynamoDBAttributeValue)
	
	var order TestOrder
	err := UnmarshalStreamImage(streamImage, &order)
	// Should not error on empty image
	assert.NoError(t, err)
}

func TestUnmarshalStreamImage_NilDestination(t *testing.T) {
	streamImage := map[string]events.DynamoDBAttributeValue{
		"PK": events.NewStringAttribute("TEST"),
	}
	
	err := UnmarshalStreamImage(streamImage, nil)
	assert.Error(t, err)
}

// TestUnmarshalStreamImage_JSONString tests unmarshaling JSON strings into structs
func TestUnmarshalStreamImage_JSONString(t *testing.T) {
	type Address struct {
		Street  string `json:"street"`
		City    string `json:"city"`
		Country string `json:"country"`
	}
	
	type Customer struct {
		PK      string  `dynamorm:"PK"`
		SK      string  `dynamorm:"SK"`
		Name    string  `dynamorm:"name"`
		Address Address `dynamorm:"address"`
		Tags    []string `dynamorm:"tags"`
	}
	
	// Create stream image with JSON string for struct field
	streamImage := map[string]events.DynamoDBAttributeValue{
		"PK":   events.NewStringAttribute("CUSTOMER#123"),
		"SK":   events.NewStringAttribute("PROFILE"),
		"name": events.NewStringAttribute("John Doe"),
		"address": events.NewStringAttribute(`{"street":"123 Main St","city":"New York","country":"USA"}`),
		"tags": events.NewStringAttribute(`["premium","verified"]`),
	}
	
	var customer Customer
	err := UnmarshalStreamImage(streamImage, &customer)
	require.NoError(t, err)
	
	assert.Equal(t, "CUSTOMER#123", customer.PK)
	assert.Equal(t, "PROFILE", customer.SK)
	assert.Equal(t, "John Doe", customer.Name)
	assert.Equal(t, "123 Main St", customer.Address.Street)
	assert.Equal(t, "New York", customer.Address.City)
	assert.Equal(t, "USA", customer.Address.Country)
	assert.Equal(t, []string{"premium", "verified"}, customer.Tags)
}

// TestUnmarshalStreamImage_TimeFields tests unmarshaling time fields
func TestUnmarshalStreamImage_TimeFields(t *testing.T) {
	type Event struct {
		PK        string    `dynamorm:"PK"`
		SK        string    `dynamorm:"SK"`
		CreatedAt time.Time `dynamorm:"created_at"`
		UpdatedAt time.Time `dynamorm:"updated_at"`
		ExpiresAt time.Time `dynamorm:"expires_at"`
	}
	
	now := time.Now().UTC().Truncate(time.Second) // Truncate to match RFC3339 precision
	
	// Test various time formats
	streamImage := map[string]events.DynamoDBAttributeValue{
		"PK":         events.NewStringAttribute("EVENT#123"),
		"SK":         events.NewStringAttribute("METADATA"),
		"created_at": events.NewStringAttribute(now.Format(time.RFC3339)),
		"updated_at": events.NewStringAttribute(now.Add(time.Hour).Format(time.RFC3339Nano)),
		"expires_at": events.NewStringAttribute(fmt.Sprintf("%d", now.Add(24*time.Hour).Unix())),
	}
	
	var event Event
	err := UnmarshalStreamImage(streamImage, &event)
	require.NoError(t, err)
	
	assert.Equal(t, "EVENT#123", event.PK)
	assert.Equal(t, "METADATA", event.SK)
	assert.Equal(t, now, event.CreatedAt)
	assert.Equal(t, now.Add(time.Hour).Truncate(time.Second), event.UpdatedAt.Truncate(time.Second))
	assert.Equal(t, now.Add(24*time.Hour).Unix(), event.ExpiresAt.Unix())
}
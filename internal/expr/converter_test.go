package expr

import (
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test struct for various scenarios
type TestStruct struct {
	ID           string         `dynamorm:"id,pk"`
	Name         string         `dynamorm:"attr:name"`
	Age          int            `dynamorm:"age"`
	Active       bool           `dynamorm:"active"`
	Score        float64        `dynamorm:"score"`
	Tags         []string       `dynamorm:"tags,set"`
	CreatedAt    time.Time      `dynamorm:"created_at"`
	UpdatedAt    time.Time      `dynamorm:"updated_at,omitempty"`
	Metadata     map[string]any `dynamorm:"metadata"`
	IgnoreField  string         `dynamorm:"-"`
	privateField string         // Should be ignored
}

// Test struct with JSON tags
type JSONStruct struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

func TestConvertToAttributeValue_BasicTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected types.AttributeValue
	}{
		{
			name:     "string",
			input:    "hello",
			expected: &types.AttributeValueMemberS{Value: "hello"},
		},
		{
			name:     "int",
			input:    42,
			expected: &types.AttributeValueMemberN{Value: "42"},
		},
		{
			name:     "float",
			input:    3.14,
			expected: &types.AttributeValueMemberN{Value: "3.14"},
		},
		{
			name:     "bool",
			input:    true,
			expected: &types.AttributeValueMemberBOOL{Value: true},
		},
		{
			name:     "nil",
			input:    nil,
			expected: &types.AttributeValueMemberNULL{Value: true},
		},
		{
			name:     "byte slice",
			input:    []byte("binary"),
			expected: &types.AttributeValueMemberB{Value: []byte("binary")},
		},
		{
			name:  "string slice",
			input: []string{"a", "b", "c"},
			expected: &types.AttributeValueMemberL{Value: []types.AttributeValue{
				&types.AttributeValueMemberS{Value: "a"},
				&types.AttributeValueMemberS{Value: "b"},
				&types.AttributeValueMemberS{Value: "c"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertToAttributeValue(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertToAttributeValue_Struct(t *testing.T) {
	now := time.Now().UTC()
	testStruct := TestStruct{
		ID:        "123",
		Name:      "Test User",
		Age:       25,
		Active:    true,
		Score:     98.5,
		Tags:      []string{"go", "dynamodb"},
		CreatedAt: now,
		UpdatedAt: now,
		Metadata: map[string]any{
			"key1": "value1",
			"key2": 42,
		},
		IgnoreField:  "should be ignored",
		privateField: "also ignored",
	}

	av, err := ConvertToAttributeValue(testStruct)
	require.NoError(t, err)

	m, ok := av.(*types.AttributeValueMemberM)
	require.True(t, ok)
	require.NotNil(t, m.Value)

	// Check fields are properly mapped
	assert.Contains(t, m.Value, "id")
	assert.Contains(t, m.Value, "name") // Using attr: tag
	assert.Contains(t, m.Value, "age")
	assert.Contains(t, m.Value, "active")
	assert.Contains(t, m.Value, "score")
	assert.Contains(t, m.Value, "tags")
	assert.Contains(t, m.Value, "created_at")
	assert.Contains(t, m.Value, "updated_at")
	assert.Contains(t, m.Value, "metadata")

	// Check ignored fields
	assert.NotContains(t, m.Value, "IgnoreField")
	assert.NotContains(t, m.Value, "privateField")

	// Verify values
	assert.Equal(t, &types.AttributeValueMemberS{Value: "123"}, m.Value["id"])
	assert.Equal(t, &types.AttributeValueMemberS{Value: "Test User"}, m.Value["name"])
	assert.Equal(t, &types.AttributeValueMemberN{Value: "25"}, m.Value["age"])
	assert.Equal(t, &types.AttributeValueMemberBOOL{Value: true}, m.Value["active"])
}

func TestConvertFromAttributeValue_BasicTypes(t *testing.T) {
	tests := []struct {
		name     string
		av       types.AttributeValue
		target   any
		expected any
	}{
		{
			name:     "string",
			av:       &types.AttributeValueMemberS{Value: "hello"},
			target:   new(string),
			expected: "hello",
		},
		{
			name:     "int",
			av:       &types.AttributeValueMemberN{Value: "42"},
			target:   new(int),
			expected: 42,
		},
		{
			name:     "float",
			av:       &types.AttributeValueMemberN{Value: "3.14"},
			target:   new(float64),
			expected: 3.14,
		},
		{
			name:     "bool",
			av:       &types.AttributeValueMemberBOOL{Value: true},
			target:   new(bool),
			expected: true,
		},
		{
			name:     "binary",
			av:       &types.AttributeValueMemberB{Value: []byte("data")},
			target:   new([]byte),
			expected: []byte("data"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ConvertFromAttributeValue(tt.av, tt.target)
			assert.NoError(t, err)

			// Dereference the pointer to get the actual value
			targetVal := reflect.ValueOf(tt.target).Elem().Interface()
			assert.Equal(t, tt.expected, targetVal)
		})
	}
}

func TestConvertFromAttributeValue_Struct(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Nanosecond)

	// Create AttributeValue map
	avMap := map[string]types.AttributeValue{
		"id":     &types.AttributeValueMemberS{Value: "123"},
		"name":   &types.AttributeValueMemberS{Value: "Test User"},
		"age":    &types.AttributeValueMemberN{Value: "25"},
		"active": &types.AttributeValueMemberBOOL{Value: true},
		"score":  &types.AttributeValueMemberN{Value: "98.5"},
		"tags": &types.AttributeValueMemberL{Value: []types.AttributeValue{
			&types.AttributeValueMemberS{Value: "go"},
			&types.AttributeValueMemberS{Value: "dynamodb"},
		}},
		"created_at": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339Nano)},
		"metadata": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"key1": &types.AttributeValueMemberS{Value: "value1"},
			"key2": &types.AttributeValueMemberN{Value: "42"},
		}},
	}

	var result TestStruct
	err := ConvertFromAttributeValue(&types.AttributeValueMemberM{Value: avMap}, &result)
	require.NoError(t, err)

	assert.Equal(t, "123", result.ID)
	assert.Equal(t, "Test User", result.Name)
	assert.Equal(t, 25, result.Age)
	assert.Equal(t, true, result.Active)
	assert.Equal(t, 98.5, result.Score)
	assert.Equal(t, []string{"go", "dynamodb"}, result.Tags)
	assert.Equal(t, now, result.CreatedAt)
}

func TestConvertFromAttributeValue_NullHandling(t *testing.T) {
	type NullableStruct struct {
		StringPtr *string
		IntPtr    *int
		BoolPtr   *bool
	}

	avMap := map[string]types.AttributeValue{
		"StringPtr": &types.AttributeValueMemberNULL{Value: true},
		"IntPtr":    &types.AttributeValueMemberN{Value: "42"},
		"BoolPtr":   &types.AttributeValueMemberNULL{Value: true},
	}

	var result NullableStruct
	err := ConvertFromAttributeValue(&types.AttributeValueMemberM{Value: avMap}, &result)
	require.NoError(t, err)

	assert.Nil(t, result.StringPtr)
	assert.NotNil(t, result.IntPtr)
	assert.Equal(t, 42, *result.IntPtr)
	assert.Nil(t, result.BoolPtr)
}

func TestConvertToAttributeValue_OmitEmpty(t *testing.T) {
	type OmitStruct struct {
		Required string `dynamorm:"required"`
		Optional string `dynamorm:"optional,omitempty"`
		Number   int    `dynamorm:"number,omitempty"`
	}

	s := OmitStruct{
		Required: "value",
		Optional: "", // Empty, should be omitted
		Number:   0,  // Zero, should be omitted
	}

	av, err := ConvertToAttributeValue(s)
	require.NoError(t, err)

	m, ok := av.(*types.AttributeValueMemberM)
	require.True(t, ok)

	assert.Contains(t, m.Value, "required")
	assert.NotContains(t, m.Value, "optional")
	assert.NotContains(t, m.Value, "number")
}

func TestBidirectionalConversion(t *testing.T) {
	original := TestStruct{
		ID:        "test-123",
		Name:      "Bidirectional Test",
		Age:       30,
		Active:    false,
		Score:     75.5,
		Tags:      []string{"test", "bidirectional"},
		CreatedAt: time.Now().UTC().Truncate(time.Nanosecond),
	}

	// Convert to AttributeValue
	av, err := ConvertToAttributeValue(original)
	require.NoError(t, err)

	// Convert back
	var result TestStruct
	err = ConvertFromAttributeValue(av, &result)
	require.NoError(t, err)

	// Compare relevant fields (ignoring private/ignored fields)
	assert.Equal(t, original.ID, result.ID)
	assert.Equal(t, original.Name, result.Name)
	assert.Equal(t, original.Age, result.Age)
	assert.Equal(t, original.Active, result.Active)
	assert.Equal(t, original.Score, result.Score)
	assert.Equal(t, original.Tags, result.Tags)
	assert.Equal(t, original.CreatedAt, result.CreatedAt)
}

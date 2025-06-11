package marshal

import (
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/model"
)

// SafeMarshaler provides a safe marshaling implementation without unsafe operations
type SafeMarshaler struct {
	cache sync.Map
}

// NewSafeMarshaler creates a new safe marshaler
func NewSafeMarshaler() *SafeMarshaler {
	return &SafeMarshaler{}
}

// MarshalItem safely marshals a model to DynamoDB AttributeValues using only reflection
func (m *SafeMarshaler) MarshalItem(model any, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil, fmt.Errorf("cannot marshal nil pointer")
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("model must be a struct or pointer to struct")
	}

	result := make(map[string]types.AttributeValue)
	now := time.Now()

	for _, fieldMeta := range metadata.Fields {
		field := v.FieldByIndex([]int{fieldMeta.Index})

		// Handle special fields
		if fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt {
			result[fieldMeta.DBName] = &types.AttributeValueMemberS{Value: now.Format(time.RFC3339Nano)}
			continue
		}

		if fieldMeta.IsVersion && field.Kind() == reflect.Int64 {
			result[fieldMeta.DBName] = &types.AttributeValueMemberN{Value: strconv.FormatInt(field.Int(), 10)}
			continue
		}

		// Marshal the field value
		av, err := m.marshalValue(field, fieldMeta)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", fieldMeta.DBName, err)
		}

		// Skip NULL values if omitempty
		if _, isNull := av.(*types.AttributeValueMemberNULL); isNull && fieldMeta.OmitEmpty {
			continue
		}

		result[fieldMeta.DBName] = av
	}

	return result, nil
}

// marshalValue safely marshals a reflect.Value to AttributeValue
func (m *SafeMarshaler) marshalValue(v reflect.Value, fieldMeta *model.FieldMetadata) (types.AttributeValue, error) {
	// Handle nil pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		v = v.Elem()
	}

	// Check for zero values with omitempty
	if fieldMeta.OmitEmpty && v.IsZero() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	switch v.Kind() {
	case reflect.String:
		return &types.AttributeValueMemberS{Value: v.String()}, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(v.Int(), 10)}, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &types.AttributeValueMemberN{Value: strconv.FormatUint(v.Uint(), 10)}, nil

	case reflect.Float32, reflect.Float64:
		return &types.AttributeValueMemberN{Value: strconv.FormatFloat(v.Float(), 'f', -1, 64)}, nil

	case reflect.Bool:
		return &types.AttributeValueMemberBOOL{Value: v.Bool()}, nil

	case reflect.Struct:
		// Special handling for time.Time
		if v.Type() == reflect.TypeOf(time.Time{}) {
			t := v.Interface().(time.Time)
			if fieldMeta.IsTTL {
				return &types.AttributeValueMemberN{Value: strconv.FormatInt(t.Unix(), 10)}, nil
			}
			return &types.AttributeValueMemberS{Value: t.Format(time.RFC3339Nano)}, nil
		}
		// For other structs, marshal as a map
		return m.marshalStruct(v)

	case reflect.Slice:
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		// Handle string sets
		if v.Type().Elem().Kind() == reflect.String && fieldMeta.IsSet {
			strings := make([]string, v.Len())
			for i := 0; i < v.Len(); i++ {
				strings[i] = v.Index(i).String()
			}
			return &types.AttributeValueMemberSS{Value: strings}, nil
		}
		// Handle regular lists
		return m.marshalSlice(v)

	case reflect.Map:
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		return m.marshalMap(v)

	case reflect.Interface:
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		return m.marshalValue(v.Elem(), fieldMeta)

	default:
		return nil, fmt.Errorf("unsupported type: %v", v.Kind())
	}
}

// marshalSlice safely marshals a slice
func (m *SafeMarshaler) marshalSlice(v reflect.Value) (types.AttributeValue, error) {
	list := make([]types.AttributeValue, v.Len())
	for i := 0; i < v.Len(); i++ {
		elem, err := m.marshalValue(v.Index(i), &model.FieldMetadata{})
		if err != nil {
			return nil, fmt.Errorf("slice index %d: %w", i, err)
		}
		list[i] = elem
	}
	return &types.AttributeValueMemberL{Value: list}, nil
}

// marshalMap safely marshals a map
func (m *SafeMarshaler) marshalMap(v reflect.Value) (types.AttributeValue, error) {
	avMap := make(map[string]types.AttributeValue, v.Len())
	for _, key := range v.MapKeys() {
		keyStr := fmt.Sprintf("%v", key.Interface())
		val, err := m.marshalValue(v.MapIndex(key), &model.FieldMetadata{})
		if err != nil {
			return nil, fmt.Errorf("map key %s: %w", keyStr, err)
		}
		avMap[keyStr] = val
	}
	return &types.AttributeValueMemberM{Value: avMap}, nil
}

// marshalStruct safely marshals a struct as a map
func (m *SafeMarshaler) marshalStruct(v reflect.Value) (types.AttributeValue, error) {
	structMap := make(map[string]types.AttributeValue)
	typ := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := typ.Field(i)
		// Skip unexported fields
		if field.PkgPath != "" {
			continue
		}

		fieldValue := v.Field(i)
		// Skip zero values
		if fieldValue.IsZero() {
			continue
		}

		av, err := m.marshalValue(fieldValue, &model.FieldMetadata{})
		if err != nil {
			return nil, fmt.Errorf("struct field %s: %w", field.Name, err)
		}

		structMap[field.Name] = av
	}
	return &types.AttributeValueMemberM{Value: structMap}, nil
}

// Benchmark comparison tests
func BenchmarkMarshalerComparison(b *testing.B) {
	// Prepare test data
	input := SimpleStruct{
		ID:     "bench-id",
		Name:   "Benchmark Test",
		Age:    30,
		Score:  98.5,
		Active: true,
	}

	metadata := createMetadata(
		createFieldMetadata("ID", "id", 0, reflect.TypeOf("")),
		createFieldMetadata("Name", "name", 1, reflect.TypeOf("")),
		createFieldMetadata("Age", "age", 2, reflect.TypeOf(0)),
		createFieldMetadata("Score", "score", 3, reflect.TypeOf(0.0)),
		createFieldMetadata("Active", "active", 4, reflect.TypeOf(false)),
	)

	b.Run("Unsafe", func(b *testing.B) {
		marshaler := New() // Unsafe marshaler
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = marshaler.MarshalItem(input, metadata)
		}
	})

	b.Run("Safe", func(b *testing.B) {
		marshaler := NewSafeMarshaler()
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = marshaler.MarshalItem(input, metadata)
		}
	})
}

func BenchmarkMarshalerComplexComparison(b *testing.B) {
	// Prepare complex test data
	optional := "optional"
	input := ComplexStruct{
		ID:            "bench-id",
		Tags:          []string{"tag1", "tag2", "tag3", "tag4", "tag5"},
		Attributes:    map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
		Version:       1,
		TTL:           time.Now().Add(24 * time.Hour),
		OptionalField: &optional,
		StringSet:     []string{"set1", "set2", "set3"},
	}

	metadata := createMetadata(
		createFieldMetadata("ID", "id", 0, reflect.TypeOf("")),
		createFieldMetadata("Tags", "tags", 1, reflect.TypeOf([]string{})),
		createFieldMetadata("Attributes", "attributes", 2, reflect.TypeOf(map[string]string{})),
		createFieldMetadata("CreatedAt", "created_at", 3, reflect.TypeOf(time.Time{}), withCreatedAt()),
		createFieldMetadata("UpdatedAt", "updated_at", 4, reflect.TypeOf(time.Time{}), withUpdatedAt()),
		createFieldMetadata("Version", "version", 5, reflect.TypeOf(int64(0)), withVersion()),
		createFieldMetadata("TTL", "ttl", 6, reflect.TypeOf(time.Time{}), withTTL()),
		createFieldMetadata("OptionalField", "optional", 7, reflect.TypeOf(&optional), withOmitEmpty()),
		createFieldMetadata("StringSet", "string_set", 8, reflect.TypeOf([]string{}), withSet()),
	)

	b.Run("Unsafe", func(b *testing.B) {
		marshaler := New()
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = marshaler.MarshalItem(input, metadata)
		}
	})

	b.Run("Safe", func(b *testing.B) {
		marshaler := NewSafeMarshaler()
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = marshaler.MarshalItem(input, metadata)
		}
	})
}

// Test large struct with many fields
type LargeStruct struct {
	Field1  string  `dynamodb:"field1"`
	Field2  string  `dynamodb:"field2"`
	Field3  string  `dynamodb:"field3"`
	Field4  string  `dynamodb:"field4"`
	Field5  string  `dynamodb:"field5"`
	Field6  int     `dynamodb:"field6"`
	Field7  int     `dynamodb:"field7"`
	Field8  int     `dynamodb:"field8"`
	Field9  int     `dynamodb:"field9"`
	Field10 int     `dynamodb:"field10"`
	Field11 float64 `dynamodb:"field11"`
	Field12 float64 `dynamodb:"field12"`
	Field13 float64 `dynamodb:"field13"`
	Field14 float64 `dynamodb:"field14"`
	Field15 float64 `dynamodb:"field15"`
	Field16 bool    `dynamodb:"field16"`
	Field17 bool    `dynamodb:"field17"`
	Field18 bool    `dynamodb:"field18"`
	Field19 bool    `dynamodb:"field19"`
	Field20 bool    `dynamodb:"field20"`
}

func BenchmarkMarshalerLargeStructComparison(b *testing.B) {
	input := LargeStruct{
		Field1:  "value1",
		Field2:  "value2",
		Field3:  "value3",
		Field4:  "value4",
		Field5:  "value5",
		Field6:  6,
		Field7:  7,
		Field8:  8,
		Field9:  9,
		Field10: 10,
		Field11: 11.1,
		Field12: 12.2,
		Field13: 13.3,
		Field14: 14.4,
		Field15: 15.5,
		Field16: true,
		Field17: false,
		Field18: true,
		Field19: false,
		Field20: true,
	}

	metadata := createMetadata(
		createFieldMetadata("Field1", "field1", 0, reflect.TypeOf("")),
		createFieldMetadata("Field2", "field2", 1, reflect.TypeOf("")),
		createFieldMetadata("Field3", "field3", 2, reflect.TypeOf("")),
		createFieldMetadata("Field4", "field4", 3, reflect.TypeOf("")),
		createFieldMetadata("Field5", "field5", 4, reflect.TypeOf("")),
		createFieldMetadata("Field6", "field6", 5, reflect.TypeOf(0)),
		createFieldMetadata("Field7", "field7", 6, reflect.TypeOf(0)),
		createFieldMetadata("Field8", "field8", 7, reflect.TypeOf(0)),
		createFieldMetadata("Field9", "field9", 8, reflect.TypeOf(0)),
		createFieldMetadata("Field10", "field10", 9, reflect.TypeOf(0)),
		createFieldMetadata("Field11", "field11", 10, reflect.TypeOf(0.0)),
		createFieldMetadata("Field12", "field12", 11, reflect.TypeOf(0.0)),
		createFieldMetadata("Field13", "field13", 12, reflect.TypeOf(0.0)),
		createFieldMetadata("Field14", "field14", 13, reflect.TypeOf(0.0)),
		createFieldMetadata("Field15", "field15", 14, reflect.TypeOf(0.0)),
		createFieldMetadata("Field16", "field16", 15, reflect.TypeOf(false)),
		createFieldMetadata("Field17", "field17", 16, reflect.TypeOf(false)),
		createFieldMetadata("Field18", "field18", 17, reflect.TypeOf(false)),
		createFieldMetadata("Field19", "field19", 18, reflect.TypeOf(false)),
		createFieldMetadata("Field20", "field20", 19, reflect.TypeOf(false)),
	)

	b.Run("Unsafe", func(b *testing.B) {
		marshaler := New()
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = marshaler.MarshalItem(input, metadata)
		}
	})

	b.Run("Safe", func(b *testing.B) {
		marshaler := NewSafeMarshaler()
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = marshaler.MarshalItem(input, metadata)
		}
	})
}

// Concurrent benchmark
func BenchmarkMarshalerConcurrentComparison(b *testing.B) {
	input := SimpleStruct{
		ID:     "bench-id",
		Name:   "Benchmark Test",
		Age:    30,
		Score:  98.5,
		Active: true,
	}

	metadata := createMetadata(
		createFieldMetadata("ID", "id", 0, reflect.TypeOf("")),
		createFieldMetadata("Name", "name", 1, reflect.TypeOf("")),
		createFieldMetadata("Age", "age", 2, reflect.TypeOf(0)),
		createFieldMetadata("Score", "score", 3, reflect.TypeOf(0.0)),
		createFieldMetadata("Active", "active", 4, reflect.TypeOf(false)),
	)

	b.Run("Unsafe-Concurrent", func(b *testing.B) {
		marshaler := New()
		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, _ = marshaler.MarshalItem(input, metadata)
			}
		})
	})

	b.Run("Safe-Concurrent", func(b *testing.B) {
		marshaler := NewSafeMarshaler()
		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, _ = marshaler.MarshalItem(input, metadata)
			}
		})
	})
}

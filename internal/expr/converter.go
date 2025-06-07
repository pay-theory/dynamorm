package expr

import (
	"fmt"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// ConvertToAttributeValue converts a Go value to a DynamoDB AttributeValue
func ConvertToAttributeValue(value interface{}) (types.AttributeValue, error) {
	if value == nil {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	v := reflect.ValueOf(value)

	switch v.Kind() {
	case reflect.String:
		return &types.AttributeValueMemberS{Value: v.String()}, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", v.Int())}, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", v.Uint())}, nil

	case reflect.Float32, reflect.Float64:
		return &types.AttributeValueMemberN{Value: fmt.Sprintf("%g", v.Float())}, nil

	case reflect.Bool:
		return &types.AttributeValueMemberBOOL{Value: v.Bool()}, nil

	case reflect.Slice:
		// Handle []byte as binary
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return &types.AttributeValueMemberB{Value: v.Bytes()}, nil
		}

		// Handle other slices as lists
		list := make([]types.AttributeValue, v.Len())
		for i := 0; i < v.Len(); i++ {
			item, err := ConvertToAttributeValue(v.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			list[i] = item
		}
		return &types.AttributeValueMemberL{Value: list}, nil

	case reflect.Map:
		// Handle map[string]interface{} as M type
		if v.Type().Key().Kind() == reflect.String {
			m := make(map[string]types.AttributeValue)
			for _, key := range v.MapKeys() {
				val, err := ConvertToAttributeValue(v.MapIndex(key).Interface())
				if err != nil {
					return nil, err
				}
				m[key.String()] = val
			}
			return &types.AttributeValueMemberM{Value: m}, nil
		}
		return nil, fmt.Errorf("unsupported map type: %v", v.Type())

	case reflect.Struct:
		// Special handling for time.Time
		if t, ok := value.(time.Time); ok {
			return &types.AttributeValueMemberS{Value: t.Format(time.RFC3339Nano)}, nil
		}
		// For other structs, we'd need more complex marshaling
		return nil, fmt.Errorf("struct marshaling not yet implemented for type: %v", v.Type())

	case reflect.Ptr:
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		return ConvertToAttributeValue(v.Elem().Interface())

	default:
		return nil, fmt.Errorf("unsupported type: %v", v.Type())
	}
}

// ConvertFromAttributeValue converts a DynamoDB AttributeValue to a Go value
func ConvertFromAttributeValue(av types.AttributeValue, target interface{}) error {
	// This is a placeholder - Team 1 should provide the full implementation
	// For now, we'll just return an error
	return fmt.Errorf("ConvertFromAttributeValue not yet implemented")
}

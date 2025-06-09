// Package query provides aggregate functionality for DynamoDB queries
package query

import (
	"fmt"
	"reflect"

	"github.com/pay-theory/dynamorm/pkg/core"
)

// AggregateResult holds the result of an aggregate operation
type AggregateResult struct {
	Count   int64
	Sum     float64
	Average float64
	Min     any
	Max     any
}

// Sum calculates the sum of a numeric field
func (q *Query) Sum(field string) (float64, error) {
	// Get all items
	items, err := q.getAllItems()
	if err != nil {
		return 0, err
	}

	sum := 0.0
	for _, item := range items {
		value, err := extractNumericValue(item, field)
		if err != nil {
			continue // Skip invalid values
		}
		sum += value
	}

	return sum, nil
}

// Average calculates the average of a numeric field
func (q *Query) Average(field string) (float64, error) {
	// Get all items
	items, err := q.getAllItems()
	if err != nil {
		return 0, err
	}

	if len(items) == 0 {
		return 0, nil
	}

	sum := 0.0
	count := 0
	for _, item := range items {
		value, err := extractNumericValue(item, field)
		if err != nil {
			continue // Skip invalid values
		}
		sum += value
		count++
	}

	if count == 0 {
		return 0, nil
	}

	return sum / float64(count), nil
}

// Min finds the minimum value of a field
func (q *Query) Min(field string) (any, error) {
	// Get all items
	items, err := q.getAllItems()
	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no items found")
	}

	var minValue any
	first := true

	for _, item := range items {
		value := extractFieldValue(item, field)
		if value == nil {
			continue
		}

		if first {
			minValue = value
			first = false
			continue
		}

		if compareValues(value, minValue) < 0 {
			minValue = value
		}
	}

	if minValue == nil {
		return nil, fmt.Errorf("no valid values found for field %s", field)
	}

	return minValue, nil
}

// Max finds the maximum value of a field
func (q *Query) Max(field string) (any, error) {
	// Get all items
	items, err := q.getAllItems()
	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no items found")
	}

	var maxValue any
	first := true

	for _, item := range items {
		value := extractFieldValue(item, field)
		if value == nil {
			continue
		}

		if first {
			maxValue = value
			first = false
			continue
		}

		if compareValues(value, maxValue) > 0 {
			maxValue = value
		}
	}

	if maxValue == nil {
		return nil, fmt.Errorf("no valid values found for field %s", field)
	}

	return maxValue, nil
}

// Aggregate performs multiple aggregate operations in a single pass
func (q *Query) Aggregate(fields ...string) (*AggregateResult, error) {
	// Get all items
	items, err := q.getAllItems()
	if err != nil {
		return nil, err
	}

	result := &AggregateResult{
		Count: int64(len(items)),
	}

	if len(fields) == 0 {
		return result, nil
	}

	// Calculate aggregates for the first field
	field := fields[0]
	sum := 0.0
	count := 0
	var minValue, maxValue any
	first := true

	for _, item := range items {
		// Try numeric operations
		numValue, err := extractNumericValue(item, field)
		if err == nil {
			sum += numValue
			count++
		}

		// Get general value for min/max
		value := extractFieldValue(item, field)
		if value == nil {
			continue
		}

		if first {
			minValue = value
			maxValue = value
			first = false
			continue
		}

		if compareValues(value, minValue) < 0 {
			minValue = value
		}
		if compareValues(value, maxValue) > 0 {
			maxValue = value
		}
	}

	result.Sum = sum
	if count > 0 {
		result.Average = sum / float64(count)
	}
	result.Min = minValue
	result.Max = maxValue

	return result, nil
}

// GroupBy groups results by a field and performs aggregate operations
type GroupedResult struct {
	Key        any
	Count      int64
	Items      []any
	Aggregates map[string]*AggregateResult
}

// GroupBy groups items by a field
func (q *Query) GroupBy(field string) ([]*GroupedResult, error) {
	// Get all items
	items, err := q.getAllItems()
	if err != nil {
		return nil, err
	}

	// Group items
	groups := make(map[string]*GroupedResult)
	for _, item := range items {
		key := extractFieldValue(item, field)
		if key == nil {
			continue
		}

		keyStr := fmt.Sprintf("%v", key)
		if group, exists := groups[keyStr]; exists {
			group.Count++
			group.Items = append(group.Items, item)
		} else {
			groups[keyStr] = &GroupedResult{
				Key:   key,
				Count: 1,
				Items: []any{item},
			}
		}
	}

	// Convert map to slice
	result := make([]*GroupedResult, 0, len(groups))
	for _, group := range groups {
		result = append(result, group)
	}

	return result, nil
}

// getAllItems is a helper to retrieve all items for aggregate operations
func (q *Query) getAllItems() ([]any, error) {
	// Create a slice type based on model
	modelType := reflect.TypeOf(q.model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}
	sliceType := reflect.SliceOf(modelType)
	resultsPtr := reflect.New(sliceType)

	// Execute query
	err := q.All(resultsPtr.Interface())
	if err != nil {
		return nil, err
	}

	// Convert to []any
	results := resultsPtr.Elem()
	items := make([]any, results.Len())
	for i := 0; i < results.Len(); i++ {
		items[i] = results.Index(i).Interface()
	}

	return items, nil
}

// extractNumericValue extracts a numeric value from an item
func extractNumericValue(item any, field string) (float64, error) {
	v := reflect.ValueOf(item)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	fieldValue := v.FieldByName(field)
	if !fieldValue.IsValid() {
		return 0, fmt.Errorf("field %s not found", field)
	}

	switch fieldValue.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(fieldValue.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(fieldValue.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return fieldValue.Float(), nil
	default:
		return 0, fmt.Errorf("field %s is not numeric", field)
	}
}

// extractFieldValue extracts any field value from an item
func extractFieldValue(item any, field string) any {
	v := reflect.ValueOf(item)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	fieldValue := v.FieldByName(field)
	if !fieldValue.IsValid() || fieldValue.IsZero() {
		return nil
	}

	return fieldValue.Interface()
}

// compareValues compares two values of the same type
func compareValues(a, b any) int {
	// Handle numeric types
	aFloat, aErr := toFloat64(a)
	bFloat, bErr := toFloat64(b)
	if aErr == nil && bErr == nil {
		if aFloat < bFloat {
			return -1
		} else if aFloat > bFloat {
			return 1
		}
		return 0
	}

	// Handle strings
	aStr, aOk := a.(string)
	bStr, bOk := b.(string)
	if aOk && bOk {
		if aStr < bStr {
			return -1
		} else if aStr > bStr {
			return 1
		}
		return 0
	}

	// Default: convert to string and compare
	aStrVal := fmt.Sprintf("%v", a)
	bStrVal := fmt.Sprintf("%v", b)
	if aStrVal < bStrVal {
		return -1
	} else if aStrVal > bStrVal {
		return 1
	}
	return 0
}

// toFloat64 attempts to convert a value to float64
func toFloat64(v any) (float64, error) {
	switch val := v.(type) {
	case int:
		return float64(val), nil
	case int8:
		return float64(val), nil
	case int16:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case uint:
		return float64(val), nil
	case uint8:
		return float64(val), nil
	case uint16:
		return float64(val), nil
	case uint32:
		return float64(val), nil
	case uint64:
		return float64(val), nil
	case float32:
		return float64(val), nil
	case float64:
		return val, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

// CountDistinct counts unique values for a field
func (q *Query) CountDistinct(field string) (int64, error) {
	items, err := q.getAllItems()
	if err != nil {
		return 0, err
	}

	uniqueValues := make(map[string]bool)
	for _, item := range items {
		value := extractFieldValue(item, field)
		if value != nil {
			key := fmt.Sprintf("%v", value)
			uniqueValues[key] = true
		}
	}

	return int64(len(uniqueValues)), nil
}

// Having adds a condition on aggregate results (for use with GroupBy)
func (q *Query) Having(condition string, value any) core.Query {
	// This would need to be implemented in conjunction with GroupBy
	// For now, we just return the query unchanged
	// In a full implementation, this would filter grouped results
	return q
}

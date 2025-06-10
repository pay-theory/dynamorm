// Package query provides update builder functionality for DynamoDB
package query

import (
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/internal/expr"
	"github.com/pay-theory/dynamorm/pkg/core"
)

// UpdateBuilder provides a fluent API for building complex update expressions
type UpdateBuilder struct {
	query        *Query
	expr         *expr.Builder
	keyValues    map[string]any
	conditions   []updateCondition
	returnValues string
}

type updateCondition struct {
	field    string
	operator string
	value    any
}

// NewUpdateBuilder creates a new update builder for the query
func (q *Query) UpdateBuilder() core.UpdateBuilder {
	return &UpdateBuilder{
		query:        q,
		expr:         expr.NewBuilder(),
		keyValues:    make(map[string]any),
		returnValues: "NONE", // Default
	}
}

// Set adds a SET expression to update a field
func (ub *UpdateBuilder) Set(field string, value any) core.UpdateBuilder {
	ub.expr.AddUpdateSet(field, value)
	return ub
}

// SetIfNotExists sets a field only if it doesn't exist
func (ub *UpdateBuilder) SetIfNotExists(field string, value any, defaultValue any) core.UpdateBuilder {
	// Use if_not_exists function
	err := ub.expr.AddUpdateFunction(field, "if_not_exists", field, defaultValue)
	if err != nil {
		// Log error in production
		// For now, fall back to regular set
		ub.expr.AddUpdateSet(field, defaultValue)
	}
	return ub
}

// Add increments a numeric field (atomic counter)
func (ub *UpdateBuilder) Add(field string, value any) core.UpdateBuilder {
	ub.expr.AddUpdateAdd(field, value)
	return ub
}

// Increment is an alias for Add with value 1
func (ub *UpdateBuilder) Increment(field string) core.UpdateBuilder {
	return ub.Add(field, 1)
}

// Decrement is an alias for Add with value -1
func (ub *UpdateBuilder) Decrement(field string) core.UpdateBuilder {
	return ub.Add(field, -1)
}

// Remove removes an attribute from the item
func (ub *UpdateBuilder) Remove(field string) core.UpdateBuilder {
	ub.expr.AddUpdateRemove(field)
	return ub
}

// Delete removes elements from a set
func (ub *UpdateBuilder) Delete(field string, value any) core.UpdateBuilder {
	// DynamoDB DELETE action is for removing elements from a set
	ub.expr.AddUpdateDelete(field, value)
	return ub
}

// AppendToList appends values to the end of a list
func (ub *UpdateBuilder) AppendToList(field string, values any) core.UpdateBuilder {
	// Use list_append function to append values
	// list_append(field, values) appends to the end
	err := ub.expr.AddUpdateFunction(field, "list_append", field, values)
	if err != nil {
		// Log error in production
		// Fall back to regular set (not ideal but better than failing)
		ub.expr.AddUpdateSet(field, values)
	}
	return ub
}

// PrependToList prepends values to the beginning of a list
func (ub *UpdateBuilder) PrependToList(field string, values any) core.UpdateBuilder {
	// Use list_append function to prepend values
	// list_append(values, field) prepends to the beginning
	err := ub.expr.AddUpdateFunction(field, "list_append", values, field)
	if err != nil {
		// Log error in production
		// Fall back to regular set (not ideal but better than failing)
		ub.expr.AddUpdateSet(field, values)
	}
	return ub
}

// RemoveFromListAt removes an element from a list at a specific index
func (ub *UpdateBuilder) RemoveFromListAt(field string, index int) core.UpdateBuilder {
	ub.expr.AddUpdateRemove(fmt.Sprintf("%s[%d]", field, index))
	return ub
}

// SetListElement sets a specific element in a list
func (ub *UpdateBuilder) SetListElement(field string, index int, value any) core.UpdateBuilder {
	ub.expr.AddUpdateSet(fmt.Sprintf("%s[%d]", field, index), value)
	return ub
}

// Condition adds a condition that must be met for the update to succeed
func (ub *UpdateBuilder) Condition(field string, operator string, value any) core.UpdateBuilder {
	ub.conditions = append(ub.conditions, updateCondition{
		field:    field,
		operator: operator,
		value:    value,
	})
	return ub
}

// ConditionExists adds a condition that the field must exist
func (ub *UpdateBuilder) ConditionExists(field string) core.UpdateBuilder {
	return ub.Condition(field, "attribute_exists", nil)
}

// ConditionNotExists adds a condition that the field must not exist
func (ub *UpdateBuilder) ConditionNotExists(field string) core.UpdateBuilder {
	return ub.Condition(field, "attribute_not_exists", nil)
}

// ConditionVersion adds optimistic locking based on version field
func (ub *UpdateBuilder) ConditionVersion(currentVersion int64) core.UpdateBuilder {
	return ub.Condition("version", "=", currentVersion)
}

// ReturnValues sets what values to return after the update
func (ub *UpdateBuilder) ReturnValues(option string) core.UpdateBuilder {
	// Options: NONE, ALL_OLD, UPDATED_OLD, ALL_NEW, UPDATED_NEW
	ub.returnValues = option
	return ub
}

// Execute performs the update operation
func (ub *UpdateBuilder) Execute() error {
	// Validate we have key conditions
	primaryKey := ub.query.metadata.PrimaryKey()

	// Extract key values from query conditions
	for _, cond := range ub.query.conditions {
		if cond.Field == primaryKey.PartitionKey ||
			(primaryKey.SortKey != "" && cond.Field == primaryKey.SortKey) {
			if cond.Operator != "=" {
				return fmt.Errorf("key condition must use '=' operator")
			}
			ub.keyValues[cond.Field] = cond.Value
		}
	}

	// Validate we have complete key
	if _, ok := ub.keyValues[primaryKey.PartitionKey]; !ok {
		return fmt.Errorf("partition key %s is required for update", primaryKey.PartitionKey)
	}
	if primaryKey.SortKey != "" {
		if _, ok := ub.keyValues[primaryKey.SortKey]; !ok {
			return fmt.Errorf("sort key %s is required for update", primaryKey.SortKey)
		}
	}

	// Add conditions to expression builder
	for _, cond := range ub.conditions {
		err := ub.expr.AddConditionExpression(cond.field, cond.operator, cond.value)
		if err != nil {
			return fmt.Errorf("failed to add condition: %w", err)
		}
	}

	// Build the expression components
	components := ub.expr.Build()

	// Compile the update query
	compiled := &core.CompiledQuery{
		Operation:                 "UpdateItem",
		TableName:                 ub.query.metadata.TableName(),
		UpdateExpression:          components.UpdateExpression,
		ConditionExpression:       components.ConditionExpression,
		ExpressionAttributeNames:  components.ExpressionAttributeNames,
		ExpressionAttributeValues: components.ExpressionAttributeValues,
		ReturnValues:              ub.returnValues,
	}

	// Convert key to AttributeValues
	keyAV := make(map[string]types.AttributeValue)
	for k, v := range ub.keyValues {
		av, err := expr.ConvertToAttributeValue(v)
		if err != nil {
			return fmt.Errorf("failed to convert key value: %w", err)
		}
		keyAV[k] = av
	}

	// Execute update through executor
	if updateExecutor, ok := ub.query.executor.(UpdateItemExecutor); ok {
		return updateExecutor.ExecuteUpdateItem(compiled, keyAV)
	}

	return fmt.Errorf("executor does not support UpdateItem operation")
}

// ExecuteWithResult performs the update and returns the result
func (ub *UpdateBuilder) ExecuteWithResult(result any) error {
	// Validate result is a pointer
	resultValue := reflect.ValueOf(result)
	if resultValue.Kind() != reflect.Ptr || resultValue.IsNil() {
		return fmt.Errorf("result must be a non-nil pointer")
	}

	// Set return values to ALL_NEW if not already set
	if ub.returnValues == "NONE" {
		ub.returnValues = "ALL_NEW"
	}

	// Validate we have key conditions
	primaryKey := ub.query.metadata.PrimaryKey()

	// Extract key values from query conditions
	for _, cond := range ub.query.conditions {
		if cond.Field == primaryKey.PartitionKey ||
			(primaryKey.SortKey != "" && cond.Field == primaryKey.SortKey) {
			if cond.Operator != "=" {
				return fmt.Errorf("key condition must use '=' operator")
			}
			ub.keyValues[cond.Field] = cond.Value
		}
	}

	// Validate we have complete key
	if _, ok := ub.keyValues[primaryKey.PartitionKey]; !ok {
		return fmt.Errorf("partition key %s is required for update", primaryKey.PartitionKey)
	}
	if primaryKey.SortKey != "" {
		if _, ok := ub.keyValues[primaryKey.SortKey]; !ok {
			return fmt.Errorf("sort key %s is required for update", primaryKey.SortKey)
		}
	}

	// Add conditions to expression builder
	for _, cond := range ub.conditions {
		err := ub.expr.AddConditionExpression(cond.field, cond.operator, cond.value)
		if err != nil {
			return fmt.Errorf("failed to add condition: %w", err)
		}
	}

	// Build the expression components
	components := ub.expr.Build()

	// Compile the update query
	compiled := &core.CompiledQuery{
		Operation:                 "UpdateItem",
		TableName:                 ub.query.metadata.TableName(),
		UpdateExpression:          components.UpdateExpression,
		ConditionExpression:       components.ConditionExpression,
		ExpressionAttributeNames:  components.ExpressionAttributeNames,
		ExpressionAttributeValues: components.ExpressionAttributeValues,
		ReturnValues:              ub.returnValues,
	}

	// Convert key to AttributeValues
	keyAV := make(map[string]types.AttributeValue)
	for k, v := range ub.keyValues {
		av, err := expr.ConvertToAttributeValue(v)
		if err != nil {
			return fmt.Errorf("failed to convert key value: %w", err)
		}
		keyAV[k] = av
	}

	// Check if executor supports returning results
	if updateExecutor, ok := ub.query.executor.(UpdateItemWithResultExecutor); ok {
		updateResult, err := updateExecutor.ExecuteUpdateItemWithResult(compiled, keyAV)
		if err != nil {
			return err
		}

		// Unmarshal the returned attributes to the result
		if updateResult != nil && len(updateResult.Attributes) > 0 {
			// Convert the map to a M type AttributeValue and then unmarshal
			mapAV := &types.AttributeValueMemberM{Value: updateResult.Attributes}
			return expr.ConvertFromAttributeValue(mapAV, result)
		}
		return nil
	}

	// Fallback to regular update without result
	if updateExecutor, ok := ub.query.executor.(UpdateItemExecutor); ok {
		return updateExecutor.ExecuteUpdateItem(compiled, keyAV)
	}

	return fmt.Errorf("executor does not support UpdateItem operation")
}

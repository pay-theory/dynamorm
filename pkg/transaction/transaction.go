// Package transaction provides atomic transaction support for DynamORM
package transaction

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/model"
	"github.com/pay-theory/dynamorm/pkg/session"
	pkgTypes "github.com/pay-theory/dynamorm/pkg/types"
)

// Transaction represents a DynamoDB transaction
type Transaction struct {
	session   *session.Session
	registry  *model.Registry
	converter *pkgTypes.Converter
	writes    []types.TransactWriteItem
	reads     []types.TransactGetItem
	results   map[string]map[string]types.AttributeValue
	ctx       context.Context
}

// NewTransaction creates a new transaction
func NewTransaction(session *session.Session, registry *model.Registry, converter *pkgTypes.Converter) *Transaction {
	return &Transaction{
		session:   session,
		registry:  registry,
		converter: converter,
		writes:    make([]types.TransactWriteItem, 0),
		reads:     make([]types.TransactGetItem, 0),
		results:   make(map[string]map[string]types.AttributeValue),
		ctx:       context.Background(),
	}
}

// WithContext sets the context for the transaction
func (tx *Transaction) WithContext(ctx context.Context) *Transaction {
	tx.ctx = ctx
	return tx
}

// Create adds a create operation to the transaction
func (tx *Transaction) Create(model any) error {
	metadata, err := tx.registry.GetMetadata(model)
	if err != nil {
		return fmt.Errorf("failed to get model metadata: %w", err)
	}

	// Marshal item
	item, err := tx.marshalItem(model, metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal item: %w", err)
	}

	// Build condition expression to ensure item doesn't exist
	conditionExpression := fmt.Sprintf("attribute_not_exists(#pk)")
	expressionAttributeNames := map[string]string{
		"#pk": metadata.PrimaryKey.PartitionKey.DBName,
	}

	// Add to transaction
	tx.writes = append(tx.writes, types.TransactWriteItem{
		Put: &types.Put{
			TableName:                aws.String(metadata.TableName),
			Item:                     item,
			ConditionExpression:      aws.String(conditionExpression),
			ExpressionAttributeNames: expressionAttributeNames,
		},
	})

	return nil
}

// Update adds an update operation to the transaction
func (tx *Transaction) Update(model any) error {
	metadata, err := tx.registry.GetMetadata(model)
	if err != nil {
		return fmt.Errorf("failed to get model metadata: %w", err)
	}

	// Extract primary key
	key, err := tx.extractPrimaryKey(model, metadata)
	if err != nil {
		return fmt.Errorf("failed to extract primary key: %w", err)
	}

	// Build update expression
	updateExpression := "SET "
	expressionAttributeNames := make(map[string]string)
	expressionAttributeValues := make(map[string]types.AttributeValue)

	modelValue := reflect.ValueOf(model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}

	updateCount := 0
	for fieldName, fieldMeta := range metadata.Fields {
		// Skip primary key fields
		if fieldMeta.IsPK || fieldMeta.IsSK {
			continue
		}

		fieldValue := modelValue.Field(fieldMeta.Index)
		if !fieldValue.IsValid() || (fieldMeta.OmitEmpty && fieldValue.IsZero()) {
			continue
		}

		if updateCount > 0 {
			updateExpression += ", "
		}

		attrName := fmt.Sprintf("#f%d", updateCount)
		attrValue := fmt.Sprintf(":v%d", updateCount)

		expressionAttributeNames[attrName] = fieldMeta.DBName
		av, err := tx.converter.ToAttributeValue(fieldValue.Interface())
		if err != nil {
			return fmt.Errorf("failed to convert field %s: %w", fieldName, err)
		}
		expressionAttributeValues[attrValue] = av

		updateExpression += fmt.Sprintf("%s = %s", attrName, attrValue)
		updateCount++
	}

	// Handle version field for optimistic locking
	var conditionExpression string
	if metadata.VersionField != nil {
		versionValue := modelValue.Field(metadata.VersionField.Index)
		if versionValue.IsValid() && !versionValue.IsZero() {
			currentVersion := versionValue.Int()
			conditionExpression = "#ver = :currentVer"
			expressionAttributeNames["#ver"] = metadata.VersionField.DBName

			av, err := tx.converter.ToAttributeValue(currentVersion)
			if err != nil {
				return fmt.Errorf("failed to convert current version: %w", err)
			}
			expressionAttributeValues[":currentVer"] = av

			// Increment version
			updateExpression += fmt.Sprintf(", #ver = :newVer")
			newAv, err := tx.converter.ToAttributeValue(currentVersion + 1)
			if err != nil {
				return fmt.Errorf("failed to convert new version: %w", err)
			}
			expressionAttributeValues[":newVer"] = newAv
		}
	}

	// Handle updated_at field
	if metadata.UpdatedAtField != nil {
		// Check if we already have updated_at in the update expression
		alreadyUpdated := false
		for _, fieldMeta := range metadata.Fields {
			if fieldMeta.DBName == metadata.UpdatedAtField.DBName {
				fieldValue := modelValue.Field(fieldMeta.Index)
				if fieldValue.IsValid() && !fieldValue.IsZero() {
					alreadyUpdated = true
					break
				}
			}
		}

		if !alreadyUpdated {
			updateExpression += fmt.Sprintf(", #upd = :updTime")
			expressionAttributeNames["#upd"] = metadata.UpdatedAtField.DBName

			av, err := tx.converter.ToAttributeValue(time.Now())
			if err != nil {
				return fmt.Errorf("failed to convert updated_at timestamp: %w", err)
			}
			expressionAttributeValues[":updTime"] = av
		}
	}

	// Build update item
	updateItem := &types.Update{
		TableName:                 aws.String(metadata.TableName),
		Key:                       key,
		UpdateExpression:          aws.String(updateExpression),
		ExpressionAttributeNames:  expressionAttributeNames,
		ExpressionAttributeValues: expressionAttributeValues,
	}

	if conditionExpression != "" {
		updateItem.ConditionExpression = aws.String(conditionExpression)
	}

	// Add to transaction
	tx.writes = append(tx.writes, types.TransactWriteItem{
		Update: updateItem,
	})

	return nil
}

// Delete adds a delete operation to the transaction
func (tx *Transaction) Delete(model any) error {
	metadata, err := tx.registry.GetMetadata(model)
	if err != nil {
		return fmt.Errorf("failed to get model metadata: %w", err)
	}

	// Extract primary key
	key, err := tx.extractPrimaryKey(model, metadata)
	if err != nil {
		return fmt.Errorf("failed to extract primary key: %w", err)
	}

	// Build delete item
	deleteItem := &types.Delete{
		TableName: aws.String(metadata.TableName),
		Key:       key,
	}

	// Handle version field for optimistic locking
	if metadata.VersionField != nil {
		modelValue := reflect.ValueOf(model)
		if modelValue.Kind() == reflect.Ptr {
			modelValue = modelValue.Elem()
		}
		versionValue := modelValue.Field(metadata.VersionField.Index)

		if versionValue.IsValid() && !versionValue.IsZero() {
			deleteItem.ConditionExpression = aws.String("#ver = :ver")
			deleteItem.ExpressionAttributeNames = map[string]string{
				"#ver": metadata.VersionField.DBName,
			}

			av, err := tx.converter.ToAttributeValue(versionValue.Interface())
			if err != nil {
				return fmt.Errorf("failed to convert version for delete condition: %w", err)
			}
			deleteItem.ExpressionAttributeValues = map[string]types.AttributeValue{
				":ver": av,
			}
		}
	}

	// Add to transaction
	tx.writes = append(tx.writes, types.TransactWriteItem{
		Delete: deleteItem,
	})

	return nil
}

// Get adds a get operation to the transaction
func (tx *Transaction) Get(model any, dest any) error {
	metadata, err := tx.registry.GetMetadata(model)
	if err != nil {
		return fmt.Errorf("failed to get model metadata: %w", err)
	}

	// Extract primary key
	key, err := tx.extractPrimaryKey(model, metadata)
	if err != nil {
		return fmt.Errorf("failed to extract primary key: %w", err)
	}

	// Add to transaction
	tx.reads = append(tx.reads, types.TransactGetItem{
		Get: &types.Get{
			TableName: aws.String(metadata.TableName),
			Key:       key,
		},
	})

	// Store destination reference for later unmarshaling
	// In a real implementation, we'd need a better way to track this
	// For now, we'll handle it in Commit()

	return nil
}

// Commit executes the transaction
func (tx *Transaction) Commit() error {
	// Execute writes if any
	if len(tx.writes) > 0 {
		input := &dynamodb.TransactWriteItemsInput{
			TransactItems: tx.writes,
		}

		client, err := tx.session.Client()
		if err != nil {
			return fmt.Errorf("failed to get client for transaction commit: %w", err)
		}

		_, err = client.TransactWriteItems(tx.ctx, input)
		if err != nil {
			return tx.handleTransactionError(err)
		}
	}

	// Execute reads if any
	if len(tx.reads) > 0 {
		input := &dynamodb.TransactGetItemsInput{
			TransactItems: tx.reads,
		}

		client, err := tx.session.Client()
		if err != nil {
			return fmt.Errorf("failed to get client for transaction reads: %w", err)
		}

		output, err := client.TransactGetItems(tx.ctx, input)
		if err != nil {
			return tx.handleTransactionError(err)
		}

		// Store results for retrieval
		for i, response := range output.Responses {
			if response.Item != nil && i < len(tx.reads) {
				// Store by table name and index
				key := fmt.Sprintf("%d", i)
				tx.results[key] = response.Item
			}
		}
	}

	return nil
}

// Rollback cancels the transaction (no-op for DynamoDB)
func (tx *Transaction) Rollback() error {
	// DynamoDB transactions are atomic - they either succeed or fail entirely
	// Clear any pending operations
	tx.writes = nil
	tx.reads = nil
	tx.results = nil
	return nil
}

// handleTransactionError converts DynamoDB transaction errors to domain errors
func (tx *Transaction) handleTransactionError(err error) error {
	if err == nil {
		return nil
	}

	// Check for specific transaction errors
	errStr := err.Error()
	switch {
	case contains(errStr, "ConditionalCheckFailed"):
		return errors.ErrConditionFailed
	case contains(errStr, "TransactionCanceled"):
		// Parse cancellation reasons
		return fmt.Errorf("transaction canceled: %w", err)
	case contains(errStr, "ValidationException"):
		return fmt.Errorf("validation error: %w", err)
	default:
		return err
	}
}

// marshalItem converts a model to DynamoDB attribute values
func (tx *Transaction) marshalItem(model any, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
	item := make(map[string]types.AttributeValue)

	modelValue := reflect.ValueOf(model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}

	for fieldName, fieldMeta := range metadata.Fields {
		fieldValue := modelValue.Field(fieldMeta.Index)

		// Skip zero values if omitempty
		if fieldMeta.OmitEmpty && fieldValue.IsZero() {
			continue
		}

		// Convert to attribute value
		av, err := tx.converter.ToAttributeValue(fieldValue.Interface())
		if err != nil {
			return nil, fmt.Errorf("failed to convert field %s: %w", fieldName, err)
		}

		// Skip null values
		if av == nil {
			continue
		}

		item[fieldMeta.DBName] = av
	}

	return item, nil
}

// extractPrimaryKey extracts the primary key from a model
func (tx *Transaction) extractPrimaryKey(model any, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
	key := make(map[string]types.AttributeValue)

	modelValue := reflect.ValueOf(model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}

	// Extract partition key
	pkField := metadata.PrimaryKey.PartitionKey
	pkValue := modelValue.Field(pkField.Index)
	if pkValue.IsZero() {
		return nil, fmt.Errorf("partition key %s is empty", pkField.Name)
	}

	av, err := tx.converter.ToAttributeValue(pkValue.Interface())
	if err != nil {
		return nil, fmt.Errorf("failed to convert partition key: %w", err)
	}
	key[pkField.DBName] = av

	// Extract sort key if present
	if metadata.PrimaryKey.SortKey != nil {
		skField := metadata.PrimaryKey.SortKey
		skValue := modelValue.Field(skField.Index)
		if skValue.IsZero() {
			return nil, fmt.Errorf("sort key %s is empty", skField.Name)
		}

		av, err := tx.converter.ToAttributeValue(skValue.Interface())
		if err != nil {
			return nil, fmt.Errorf("failed to convert sort key: %w", err)
		}
		key[skField.DBName] = av
	}

	return key, nil
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[0:len(substr)] == substr || contains(s[1:], substr)))
}

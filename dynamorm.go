// Package dynamorm provides a type-safe ORM for Amazon DynamoDB in Go
package dynamorm

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/internal/expr"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/model"
	"github.com/pay-theory/dynamorm/pkg/schema"
	"github.com/pay-theory/dynamorm/pkg/session"
	"github.com/pay-theory/dynamorm/pkg/transaction"
	pkgTypes "github.com/pay-theory/dynamorm/pkg/types"
)

// DB is the main DynamORM database instance
type DB struct {
	session        *session.Session
	registry       *model.Registry
	converter      *pkgTypes.Converter
	ctx            context.Context
	mu             sync.RWMutex
	lambdaDeadline time.Time // Lambda execution deadline for timeout handling
}

// New creates a new DynamORM instance with the given configuration
func New(config session.Config) (*DB, error) {
	sess, err := session.NewSession(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &DB{
		session:   sess,
		registry:  model.NewRegistry(),
		converter: pkgTypes.NewConverter(),
		ctx:       context.Background(),
	}, nil
}

// Model returns a new query builder for the given model
func (db *DB) Model(model interface{}) core.Query {
	// Ensure model is registered
	if err := db.registry.Register(model); err != nil {
		// Return a query that will error on execution
		return &errorQuery{err: err}
	}

	return &query{
		db:    db,
		model: model,
		ctx:   db.ctx,
	}
}

// Transaction executes a function within a database transaction
func (db *DB) Transaction(fn func(tx *core.Tx) error) error {
	// For now, we'll use a simple wrapper that doesn't support full transaction features
	// Users should use TransactionFunc for full transaction support
	tx := &core.Tx{}
	return fn(tx)
}

// Migrate runs all pending migrations
func (db *DB) Migrate() error {
	// This will be implemented by the schema management team
	return fmt.Errorf("migrate not yet implemented")
}

// AutoMigrate creates or updates tables based on the given models
func (db *DB) AutoMigrate(models ...interface{}) error {
	manager := schema.NewManager(db.session, db.registry)

	for _, model := range models {
		if err := db.registry.Register(model); err != nil {
			return fmt.Errorf("failed to register model %T: %w", model, err)
		}

		// Check if table exists, create if not
		metadata, err := db.registry.GetMetadata(model)
		if err != nil {
			return err
		}

		exists, err := manager.TableExists(metadata.TableName)
		if err != nil {
			return fmt.Errorf("failed to check table existence: %w", err)
		}

		if !exists {
			if err := manager.CreateTable(model); err != nil {
				return fmt.Errorf("failed to create table for %T: %w", model, err)
			}
		}
	}

	return nil
}

// CreateTable creates a DynamoDB table for the given model
func (db *DB) CreateTable(model interface{}, opts ...schema.TableOption) error {
	// Register model first
	if err := db.registry.Register(model); err != nil {
		return fmt.Errorf("failed to register model %T: %w", model, err)
	}

	manager := schema.NewManager(db.session, db.registry)
	return manager.CreateTable(model, opts...)
}

// EnsureTable checks if a table exists for the model and creates it if not
func (db *DB) EnsureTable(model interface{}) error {
	// Register model first
	if err := db.registry.Register(model); err != nil {
		return fmt.Errorf("failed to register model %T: %w", model, err)
	}

	metadata, err := db.registry.GetMetadata(model)
	if err != nil {
		return err
	}

	manager := schema.NewManager(db.session, db.registry)
	exists, err := manager.TableExists(metadata.TableName)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if !exists {
		return manager.CreateTable(model)
	}

	return nil
}

// DeleteTable deletes the DynamoDB table for the given model
func (db *DB) DeleteTable(model interface{}) error {
	metadata, err := db.registry.GetMetadata(model)
	if err != nil {
		return err
	}

	manager := schema.NewManager(db.session, db.registry)
	return manager.DeleteTable(metadata.TableName)
}

// DescribeTable returns the table description for the given model
func (db *DB) DescribeTable(model interface{}) (*types.TableDescription, error) {
	// Register model first
	if err := db.registry.Register(model); err != nil {
		return nil, fmt.Errorf("failed to register model %T: %w", model, err)
	}

	manager := schema.NewManager(db.session, db.registry)
	return manager.DescribeTable(model)
}

// Close closes the database connection
func (db *DB) Close() error {
	// AWS SDK v2 clients don't need explicit closing
	return nil
}

// WithContext returns a new DB instance with the given context
func (db *DB) WithContext(ctx context.Context) core.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()

	return &DB{
		session:        db.session,
		registry:       db.registry,
		converter:      db.converter,
		ctx:            ctx,
		lambdaDeadline: db.lambdaDeadline,
	}
}

// WithLambdaTimeout sets a deadline based on Lambda context
func (db *DB) WithLambdaTimeout(ctx context.Context) *DB {
	deadline, ok := ctx.Deadline()
	if !ok {
		return db
	}

	// Leave 1 second buffer for Lambda cleanup
	adjustedDeadline := deadline.Add(-1 * time.Second)

	db.mu.RLock()
	defer db.mu.RUnlock()

	return &DB{
		session:        db.session,
		registry:       db.registry,
		converter:      db.converter,
		ctx:            ctx,
		lambdaDeadline: adjustedDeadline,
	}
}

// query implements the core.Query interface
type query struct {
	db    *DB
	model interface{}
	ctx   context.Context

	// Query conditions
	conditions []condition
	indexName  string
	filters    []filter
	orderBy    *orderBy
	limit      *int
	offset     *int
	fields     []string
}

type condition struct {
	field string
	op    string
	value interface{}
}

type filter struct {
	expression string
	values     []interface{}
}

type orderBy struct {
	field string
	order string
}

// checkLambdaTimeout checks if Lambda execution is about to timeout
func (q *query) checkLambdaTimeout() error {
	if q.db.lambdaDeadline.IsZero() {
		return nil // No Lambda deadline set
	}

	remaining := time.Until(q.db.lambdaDeadline)
	if remaining <= 0 {
		return fmt.Errorf("Lambda timeout exceeded")
	}

	// If we have less than 100ms, consider it too close to timeout
	if remaining < 100*time.Millisecond {
		return fmt.Errorf("Lambda timeout imminent: only %v remaining", remaining)
	}

	return nil
}

// Where adds a condition to the query
func (q *query) Where(field string, op string, value interface{}) core.Query {
	q.conditions = append(q.conditions, condition{
		field: field,
		op:    op,
		value: value,
	})
	return q
}

// Index specifies which index to use for the query
func (q *query) Index(indexName string) core.Query {
	q.indexName = indexName
	return q
}

// Filter adds a filter expression to the query
func (q *query) Filter(expression string, values ...interface{}) core.Query {
	q.filters = append(q.filters, filter{
		expression: expression,
		values:     values,
	})
	return q
}

// OrderBy sets the sort order for the query
func (q *query) OrderBy(field string, order string) core.Query {
	q.orderBy = &orderBy{
		field: field,
		order: order,
	}
	return q
}

// Limit sets the maximum number of items to return
func (q *query) Limit(limit int) core.Query {
	q.limit = &limit
	return q
}

// Offset sets the starting position for the query
func (q *query) Offset(offset int) core.Query {
	q.offset = &offset
	return q
}

// Select specifies which fields to retrieve
func (q *query) Select(fields ...string) core.Query {
	q.fields = fields
	return q
}

// First retrieves the first matching item
func (q *query) First(dest interface{}) error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Build GetItem request if we have a primary key condition
	if pk := q.extractPrimaryKey(metadata); pk != nil {
		return q.getItem(metadata, pk, dest)
	}

	// Otherwise, use Query with limit 1
	q.limit = new(int)
	*q.limit = 1

	// Create a slice to hold results
	sliceType := reflect.SliceOf(reflect.TypeOf(dest).Elem())
	results := reflect.New(sliceType).Interface()

	if err := q.All(results); err != nil {
		return err
	}

	// Extract first result
	resultsValue := reflect.ValueOf(results).Elem()
	if resultsValue.Len() == 0 {
		return errors.ErrItemNotFound
	}

	// Copy first item to dest
	reflect.ValueOf(dest).Elem().Set(resultsValue.Index(0))
	return nil
}

// All retrieves all matching items
func (q *query) All(dest interface{}) error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Validate destination is a slice pointer
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	// Determine if we should use Query or Scan
	useQuery := false
	var keyConditions []condition
	var filterConditions []condition

	// Check if we have partition key condition
	for _, cond := range q.conditions {
		fieldMeta, exists := metadata.Fields[cond.field]
		if !exists {
			filterConditions = append(filterConditions, cond)
			continue
		}

		if fieldMeta.IsPK || (q.indexName != "" && q.isIndexKey(fieldMeta, q.indexName, metadata)) {
			if cond.op == "=" || cond.op == "BEGINS_WITH" {
				keyConditions = append(keyConditions, cond)
				useQuery = true
			} else {
				filterConditions = append(filterConditions, cond)
			}
		} else {
			filterConditions = append(filterConditions, cond)
		}
	}

	// Execute Query or Scan
	var items []map[string]types.AttributeValue
	if useQuery {
		items, err = q.executeQuery(metadata, keyConditions, filterConditions)
	} else {
		items, err = q.executeScan(metadata, filterConditions)
	}

	if err != nil {
		return err
	}

	// Unmarshal items to destination slice
	return q.unmarshalItems(items, dest, metadata)
}

// Count returns the number of matching items
func (q *query) Count() (int64, error) {
	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return 0, err
	}

	// Determine if we should use Query or Scan
	useQuery := false
	var keyConditions []condition
	var filterConditions []condition

	// Check if we have partition key condition
	for _, cond := range q.conditions {
		fieldMeta, exists := metadata.Fields[cond.field]
		if !exists {
			filterConditions = append(filterConditions, cond)
			continue
		}

		if fieldMeta.IsPK || (q.indexName != "" && q.isIndexKey(fieldMeta, q.indexName, metadata)) {
			if cond.op == "=" || cond.op == "BEGINS_WITH" {
				keyConditions = append(keyConditions, cond)
				useQuery = true
			} else {
				filterConditions = append(filterConditions, cond)
			}
		} else {
			filterConditions = append(filterConditions, cond)
		}
	}

	// Execute count operation
	if useQuery {
		return q.executeQueryCount(metadata, keyConditions, filterConditions)
	}
	return q.executeScanCount(metadata, filterConditions)
}

// Create creates a new item
func (q *query) Create() error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Use PutItem to create the item
	return q.putItem(metadata)
}

// Update updates the matching items
func (q *query) Update(fields ...string) error {
	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Build UpdateItem request
	return q.updateItem(metadata, fields)
}

// Delete deletes the matching items
func (q *query) Delete() error {
	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Build DeleteItem request
	return q.deleteItem(metadata)
}

// Scan performs a table scan
func (q *query) Scan(dest interface{}) error {
	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Validate destination is a slice pointer
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	// Convert all conditions to filter conditions for scan
	var filterConditions []condition
	filterConditions = append(filterConditions, q.conditions...)

	// Execute scan
	items, err := q.executeScan(metadata, filterConditions)
	if err != nil {
		return err
	}

	// Unmarshal items to destination slice
	return q.unmarshalItems(items, dest, metadata)
}

// BatchGet retrieves multiple items by their primary keys
func (q *query) BatchGet(keys []interface{}, dest interface{}) error {
	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Validate destination is a slice pointer
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	// Build batch get request
	keysAndAttributes := &types.KeysAndAttributes{
		Keys: make([]map[string]types.AttributeValue, 0, len(keys)),
	}

	// Add projection if specified
	if len(q.fields) > 0 {
		builder := expr.NewBuilder()
		builder.AddProjection(q.fields...)
		components := builder.Build()

		if components.ProjectionExpression != "" {
			keysAndAttributes.ProjectionExpression = aws.String(components.ProjectionExpression)
			keysAndAttributes.ExpressionAttributeNames = components.ExpressionAttributeNames
		}
	}

	// Convert keys to DynamoDB format
	for _, key := range keys {
		keyMap := make(map[string]types.AttributeValue)

		// Handle different key formats
		switch k := key.(type) {
		case map[string]interface{}:
			// Key is a map with pk and optional sk
			if pk, hasPK := k["pk"]; hasPK {
				av, err := q.db.converter.ToAttributeValue(pk)
				if err != nil {
					return fmt.Errorf("failed to convert partition key: %w", err)
				}
				keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
			}
			if sk, hasSK := k["sk"]; hasSK && metadata.PrimaryKey.SortKey != nil {
				av, err := q.db.converter.ToAttributeValue(sk)
				if err != nil {
					return fmt.Errorf("failed to convert sort key: %w", err)
				}
				keyMap[metadata.PrimaryKey.SortKey.DBName] = av
			}
		default:
			// Key is just the partition key value
			av, err := q.db.converter.ToAttributeValue(key)
			if err != nil {
				return fmt.Errorf("failed to convert partition key: %w", err)
			}
			keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
		}

		keysAndAttributes.Keys = append(keysAndAttributes.Keys, keyMap)
	}

	// Build BatchGetItem input
	input := &dynamodb.BatchGetItemInput{
		RequestItems: map[string]types.KeysAndAttributes{
			metadata.TableName: *keysAndAttributes,
		},
	}

	// Execute batch get
	var allItems []map[string]types.AttributeValue

	for {
		output, err := q.db.session.Client().BatchGetItem(q.ctx, input)
		if err != nil {
			return fmt.Errorf("failed to batch get items: %w", err)
		}

		// Collect items
		if items, exists := output.Responses[metadata.TableName]; exists {
			allItems = append(allItems, items...)
		}

		// Check for unprocessed keys
		if len(output.UnprocessedKeys) == 0 {
			break
		}

		// Retry unprocessed keys
		input.RequestItems = output.UnprocessedKeys
	}

	// Unmarshal items to destination slice
	return q.unmarshalItems(allItems, dest, metadata)
}

// BatchCreate creates multiple items
func (q *query) BatchCreate(items interface{}) error {
	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Validate items is a slice
	itemsValue := reflect.ValueOf(items)
	if itemsValue.Kind() == reflect.Ptr {
		itemsValue = itemsValue.Elem()
	}
	if itemsValue.Kind() != reflect.Slice {
		return fmt.Errorf("items must be a slice")
	}

	// Process items in batches of 25 (DynamoDB limit)
	const batchSize = 25
	totalItems := itemsValue.Len()

	for i := 0; i < totalItems; i += batchSize {
		end := i + batchSize
		if end > totalItems {
			end = totalItems
		}

		// Build batch write request
		writeRequests := make([]types.WriteRequest, 0, end-i)

		for j := i; j < end; j++ {
			itemValue := itemsValue.Index(j)
			if itemValue.Kind() == reflect.Ptr {
				itemValue = itemValue.Elem()
			}

			// Marshal item
			item, err := q.marshalItem(itemValue.Interface(), metadata)
			if err != nil {
				return fmt.Errorf("failed to marshal item %d: %w", j, err)
			}

			writeRequests = append(writeRequests, types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: item,
				},
			})
		}

		// Build BatchWriteItem input
		input := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				metadata.TableName: writeRequests,
			},
		}

		// Execute batch write with retries for unprocessed items
		for {
			output, err := q.db.session.Client().BatchWriteItem(q.ctx, input)
			if err != nil {
				return fmt.Errorf("failed to batch create items: %w", err)
			}

			// Check for unprocessed items
			if len(output.UnprocessedItems) == 0 {
				break
			}

			// Retry unprocessed items
			input.RequestItems = output.UnprocessedItems
		}
	}

	return nil
}

// WithContext sets the context for the query
func (q *query) WithContext(ctx context.Context) core.Query {
	q.ctx = ctx
	return q
}

// Helper methods for basic CRUD operations

func (q *query) extractPrimaryKey(metadata *model.Metadata) map[string]interface{} {
	if len(q.conditions) == 0 {
		return nil
	}

	pk := make(map[string]interface{})

	// Check for partition key
	for _, cond := range q.conditions {
		if cond.op != "=" {
			continue
		}

		if field, exists := metadata.Fields[cond.field]; exists {
			if field.IsPK {
				pk["pk"] = cond.value
			} else if field.IsSK {
				pk["sk"] = cond.value
			}
		}
	}

	// Must have at least partition key
	if _, hasPK := pk["pk"]; !hasPK {
		return nil
	}

	return pk
}

func (q *query) getItem(metadata *model.Metadata, pk map[string]interface{}, dest interface{}) error {
	// Build GetItem input
	tableName := metadata.TableName

	// Convert primary key to DynamoDB attribute values
	keyMap := make(map[string]types.AttributeValue)

	// Add partition key
	if pkValue, hasPK := pk["pk"]; hasPK {
		av, err := q.db.converter.ToAttributeValue(pkValue)
		if err != nil {
			return fmt.Errorf("failed to convert partition key: %w", err)
		}
		keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
	}

	// Add sort key if present
	if skValue, hasSK := pk["sk"]; hasSK && metadata.PrimaryKey.SortKey != nil {
		av, err := q.db.converter.ToAttributeValue(skValue)
		if err != nil {
			return fmt.Errorf("failed to convert sort key: %w", err)
		}
		keyMap[metadata.PrimaryKey.SortKey.DBName] = av
	}

	// Build GetItem input
	input := &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       keyMap,
	}

	// Add projection expression if fields are specified
	if len(q.fields) > 0 {
		builder := expr.NewBuilder()
		builder.AddProjection(q.fields...)
		components := builder.Build()

		if components.ProjectionExpression != "" {
			input.ProjectionExpression = aws.String(components.ProjectionExpression)
			input.ExpressionAttributeNames = components.ExpressionAttributeNames
		}
	}

	// Execute GetItem
	output, err := q.db.session.Client().GetItem(q.ctx, input)
	if err != nil {
		return fmt.Errorf("failed to get item: %w", err)
	}

	// Check if item was found
	if output.Item == nil {
		return errors.ErrItemNotFound
	}

	// Unmarshal item to destination
	return q.unmarshalItem(output.Item, dest, metadata)
}

// unmarshalItem converts DynamoDB item to Go struct
func (q *query) unmarshalItem(item map[string]types.AttributeValue, dest interface{}, metadata *model.Metadata) error {
	// Use reflection to populate the destination struct
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr {
		return fmt.Errorf("destination must be a pointer")
	}
	destValue = destValue.Elem()

	// Iterate through the item attributes
	for attrName, attrValue := range item {
		// Find the corresponding field in metadata
		field, exists := metadata.FieldsByDBName[attrName]
		if !exists {
			continue // Skip unknown fields
		}

		// Get the struct field
		structField := destValue.FieldByIndex([]int{field.Index})
		if !structField.CanSet() {
			continue
		}

		// Convert and set the value
		if err := q.db.converter.FromAttributeValue(attrValue, structField.Addr().Interface()); err != nil {
			return fmt.Errorf("failed to unmarshal field %s: %w", field.Name, err)
		}
	}

	return nil
}

func (q *query) putItem(metadata *model.Metadata) error {
	// Marshal the model to DynamoDB item
	item, err := q.marshalItem(q.model, metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal item: %w", err)
	}

	// Build PutItem input
	input := &dynamodb.PutItemInput{
		TableName: aws.String(metadata.TableName),
		Item:      item,
	}

	// Add condition to ensure item doesn't already exist
	pkField := metadata.PrimaryKey.PartitionKey
	builder := expr.NewBuilder()
	builder.AddConditionExpression(pkField.DBName, "NOT_EXISTS", nil)

	if metadata.PrimaryKey.SortKey != nil {
		skField := metadata.PrimaryKey.SortKey
		builder.AddConditionExpression(skField.DBName, "NOT_EXISTS", nil)
	}

	components := builder.Build()
	if components.ConditionExpression != "" {
		input.ConditionExpression = aws.String(components.ConditionExpression)
		input.ExpressionAttributeNames = components.ExpressionAttributeNames
	}

	// Execute PutItem
	_, err = q.db.session.Client().PutItem(q.ctx, input)
	if err != nil {
		// Check if it's a conditional check failure
		if isConditionalCheckFailedException(err) {
			return errors.ErrConditionFailed
		}
		return fmt.Errorf("failed to put item: %w", err)
	}

	return nil
}

// marshalItem converts a Go struct to DynamoDB item
func (q *query) marshalItem(model interface{}, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
	item := make(map[string]types.AttributeValue)

	modelValue := reflect.ValueOf(model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}

	// Process each field
	for fieldName, fieldMeta := range metadata.Fields {
		fieldValue := modelValue.FieldByIndex([]int{fieldMeta.Index})

		// Skip zero values if omitempty
		if fieldMeta.OmitEmpty && fieldValue.IsZero() {
			continue
		}

		// Handle special fields
		if fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt {
			// Set to current time
			now := time.Now()
			fieldValue = reflect.ValueOf(now)
		} else if fieldMeta.IsVersion {
			// Initialize version to 0 for new items
			if fieldValue.IsZero() {
				fieldValue = reflect.ValueOf(int64(0))
			}
		} else if fieldMeta.IsTTL {
			// Convert TTL to Unix timestamp if it's a time.Time
			if fieldValue.Type().String() == "time.Time" && !fieldValue.IsZero() {
				ttlTime := fieldValue.Interface().(time.Time)
				fieldValue = reflect.ValueOf(ttlTime.Unix())
			}
		}

		// Convert to AttributeValue
		var av types.AttributeValue
		var err error

		if fieldMeta.IsSet {
			av, err = q.db.converter.ConvertToSet(fieldValue.Interface(), true)
		} else {
			av, err = q.db.converter.ToAttributeValue(fieldValue.Interface())
		}

		if err != nil {
			return nil, fmt.Errorf("failed to convert field %s: %w", fieldName, err)
		}

		// Skip NULL values unless explicitly included
		if _, isNull := av.(*types.AttributeValueMemberNULL); isNull && fieldMeta.OmitEmpty {
			continue
		}

		item[fieldMeta.DBName] = av
	}

	return item, nil
}

// isConditionalCheckFailedException checks if the error is a conditional check failure
func isConditionalCheckFailedException(err error) bool {
	// In AWS SDK v2, we need to check the error type
	// This is a simplified check - in production, use proper error type checking
	return err != nil && (contains(err.Error(), "ConditionalCheckFailedException") ||
		contains(err.Error(), "conditional request failed"))
}

// contains is a helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && strings.Contains(s, substr))
}

// executeQuery performs a DynamoDB Query operation
func (q *query) executeQuery(metadata *model.Metadata, keyConditions []condition, filterConditions []condition) ([]map[string]types.AttributeValue, error) {
	builder := expr.NewBuilder()

	// Add key conditions
	for _, cond := range keyConditions {
		fieldMeta, _ := metadata.Fields[cond.field]
		builder.AddKeyCondition(fieldMeta.DBName, cond.op, cond.value)
	}

	// Add filter conditions
	for _, cond := range filterConditions {
		fieldMeta, exists := metadata.Fields[cond.field]
		if exists {
			builder.AddFilterCondition(fieldMeta.DBName, cond.op, cond.value)
		} else {
			builder.AddFilterCondition(cond.field, cond.op, cond.value)
		}
	}

	// Add raw filters
	for _, filter := range q.filters {
		params := make(map[string]interface{})
		for i, v := range filter.values {
			params[fmt.Sprintf("param%d", i)] = v
		}
		builder.AddRawFilter(filter.expression, params)
	}

	// Add projection
	if len(q.fields) > 0 {
		builder.AddProjection(q.fields...)
	}

	// Build expressions
	components := builder.Build()

	// Build Query input
	input := &dynamodb.QueryInput{
		TableName: aws.String(metadata.TableName),
	}

	if components.KeyConditionExpression != "" {
		input.KeyConditionExpression = aws.String(components.KeyConditionExpression)
	}
	if components.FilterExpression != "" {
		input.FilterExpression = aws.String(components.FilterExpression)
	}
	if components.ProjectionExpression != "" {
		input.ProjectionExpression = aws.String(components.ProjectionExpression)
	}
	if len(components.ExpressionAttributeNames) > 0 {
		input.ExpressionAttributeNames = components.ExpressionAttributeNames
	}
	if len(components.ExpressionAttributeValues) > 0 {
		input.ExpressionAttributeValues = components.ExpressionAttributeValues
	}

	// Set index name if specified
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}

	// Set scan direction
	if q.orderBy != nil && q.orderBy.order == "DESC" {
		input.ScanIndexForward = aws.Bool(false)
	}

	// Set limit
	if q.limit != nil {
		input.Limit = aws.Int32(int32(*q.limit))
	}

	// Execute query and collect results
	var items []map[string]types.AttributeValue
	paginator := dynamodb.NewQueryPaginator(q.db.session.Client(), input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(q.ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query items: %w", err)
		}

		items = append(items, output.Items...)

		// Stop if we have enough items
		if q.limit != nil && len(items) >= *q.limit {
			items = items[:*q.limit]
			break
		}
	}

	return items, nil
}

// executeScan performs a DynamoDB Scan operation
func (q *query) executeScan(metadata *model.Metadata, filterConditions []condition) ([]map[string]types.AttributeValue, error) {
	builder := expr.NewBuilder()

	// Add filter conditions
	for _, cond := range filterConditions {
		fieldMeta, exists := metadata.Fields[cond.field]
		if exists {
			builder.AddFilterCondition(fieldMeta.DBName, cond.op, cond.value)
		} else {
			builder.AddFilterCondition(cond.field, cond.op, cond.value)
		}
	}

	// Add raw filters
	for _, filter := range q.filters {
		params := make(map[string]interface{})
		for i, v := range filter.values {
			params[fmt.Sprintf("param%d", i)] = v
		}
		builder.AddRawFilter(filter.expression, params)
	}

	// Add projection
	if len(q.fields) > 0 {
		builder.AddProjection(q.fields...)
	}

	// Build expressions
	components := builder.Build()

	// Build Scan input
	input := &dynamodb.ScanInput{
		TableName: aws.String(metadata.TableName),
	}

	if components.FilterExpression != "" {
		input.FilterExpression = aws.String(components.FilterExpression)
	}
	if components.ProjectionExpression != "" {
		input.ProjectionExpression = aws.String(components.ProjectionExpression)
	}
	if len(components.ExpressionAttributeNames) > 0 {
		input.ExpressionAttributeNames = components.ExpressionAttributeNames
	}
	if len(components.ExpressionAttributeValues) > 0 {
		input.ExpressionAttributeValues = components.ExpressionAttributeValues
	}

	// Set index name if specified
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}

	// Set limit
	if q.limit != nil {
		input.Limit = aws.Int32(int32(*q.limit))
	}

	// Execute scan and collect results
	var items []map[string]types.AttributeValue
	paginator := dynamodb.NewScanPaginator(q.db.session.Client(), input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(q.ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to scan items: %w", err)
		}

		items = append(items, output.Items...)

		// Stop if we have enough items
		if q.limit != nil && len(items) >= *q.limit {
			items = items[:*q.limit]
			break
		}
	}

	return items, nil
}

// isIndexKey checks if a field is a key in the specified index
func (q *query) isIndexKey(fieldMeta *model.FieldMetadata, indexName string, metadata *model.Metadata) bool {
	for _, index := range metadata.Indexes {
		if index.Name == indexName {
			return (index.PartitionKey != nil && index.PartitionKey.Name == fieldMeta.Name) ||
				(index.SortKey != nil && index.SortKey.Name == fieldMeta.Name)
		}
	}
	return false
}

// unmarshalItems converts DynamoDB items to Go slice
func (q *query) unmarshalItems(items []map[string]types.AttributeValue, dest interface{}, metadata *model.Metadata) error {
	destValue := reflect.ValueOf(dest).Elem()
	elemType := destValue.Type().Elem()

	// Create a new slice with the correct length
	newSlice := reflect.MakeSlice(destValue.Type(), len(items), len(items))

	// Unmarshal each item
	for i, item := range items {
		elem := reflect.New(elemType)
		if err := q.unmarshalItem(item, elem.Interface(), metadata); err != nil {
			return fmt.Errorf("failed to unmarshal item %d: %w", i, err)
		}

		// Set the element in the slice
		if elemType.Kind() == reflect.Ptr {
			newSlice.Index(i).Set(elem)
		} else {
			newSlice.Index(i).Set(elem.Elem())
		}
	}

	// Set the destination slice
	destValue.Set(newSlice)
	return nil
}

// executeQueryCount performs a DynamoDB Query operation to count items
func (q *query) executeQueryCount(metadata *model.Metadata, keyConditions []condition, filterConditions []condition) (int64, error) {
	builder := expr.NewBuilder()

	// Add key conditions
	for _, cond := range keyConditions {
		fieldMeta, _ := metadata.Fields[cond.field]
		builder.AddKeyCondition(fieldMeta.DBName, cond.op, cond.value)
	}

	// Add filter conditions
	for _, cond := range filterConditions {
		fieldMeta, exists := metadata.Fields[cond.field]
		if exists {
			builder.AddFilterCondition(fieldMeta.DBName, cond.op, cond.value)
		} else {
			builder.AddFilterCondition(cond.field, cond.op, cond.value)
		}
	}

	// Add raw filters
	for _, filter := range q.filters {
		params := make(map[string]interface{})
		for i, v := range filter.values {
			params[fmt.Sprintf("param%d", i)] = v
		}
		builder.AddRawFilter(filter.expression, params)
	}

	// Build expressions
	components := builder.Build()

	// Build Query input with Select = COUNT
	input := &dynamodb.QueryInput{
		TableName: aws.String(metadata.TableName),
		Select:    types.SelectCount,
	}

	if components.KeyConditionExpression != "" {
		input.KeyConditionExpression = aws.String(components.KeyConditionExpression)
	}
	if components.FilterExpression != "" {
		input.FilterExpression = aws.String(components.FilterExpression)
	}
	if len(components.ExpressionAttributeNames) > 0 {
		input.ExpressionAttributeNames = components.ExpressionAttributeNames
	}
	if len(components.ExpressionAttributeValues) > 0 {
		input.ExpressionAttributeValues = components.ExpressionAttributeValues
	}

	// Set index name if specified
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}

	// Execute query and count results
	var totalCount int64
	paginator := dynamodb.NewQueryPaginator(q.db.session.Client(), input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(q.ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to count items: %w", err)
		}

		totalCount += int64(output.Count)
	}

	return totalCount, nil
}

// executeScanCount performs a DynamoDB Scan operation to count items
func (q *query) executeScanCount(metadata *model.Metadata, filterConditions []condition) (int64, error) {
	builder := expr.NewBuilder()

	// Add filter conditions
	for _, cond := range filterConditions {
		fieldMeta, exists := metadata.Fields[cond.field]
		if exists {
			builder.AddFilterCondition(fieldMeta.DBName, cond.op, cond.value)
		} else {
			builder.AddFilterCondition(cond.field, cond.op, cond.value)
		}
	}

	// Add raw filters
	for _, filter := range q.filters {
		params := make(map[string]interface{})
		for i, v := range filter.values {
			params[fmt.Sprintf("param%d", i)] = v
		}
		builder.AddRawFilter(filter.expression, params)
	}

	// Build expressions
	components := builder.Build()

	// Build Scan input with Select = COUNT
	input := &dynamodb.ScanInput{
		TableName: aws.String(metadata.TableName),
		Select:    types.SelectCount,
	}

	if components.FilterExpression != "" {
		input.FilterExpression = aws.String(components.FilterExpression)
	}
	if len(components.ExpressionAttributeNames) > 0 {
		input.ExpressionAttributeNames = components.ExpressionAttributeNames
	}
	if len(components.ExpressionAttributeValues) > 0 {
		input.ExpressionAttributeValues = components.ExpressionAttributeValues
	}

	// Set index name if specified
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}

	// Execute scan and count results
	var totalCount int64
	paginator := dynamodb.NewScanPaginator(q.db.session.Client(), input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(q.ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to count items: %w", err)
		}

		totalCount += int64(output.Count)
	}

	return totalCount, nil
}

func (q *query) updateItem(metadata *model.Metadata, fields []string) error {
	// Extract primary key from conditions
	pk := q.extractPrimaryKey(metadata)
	if pk == nil {
		return fmt.Errorf("update requires primary key in conditions")
	}

	// Build key map
	keyMap := make(map[string]types.AttributeValue)

	// Add partition key
	if pkValue, hasPK := pk["pk"]; hasPK {
		av, err := q.db.converter.ToAttributeValue(pkValue)
		if err != nil {
			return fmt.Errorf("failed to convert partition key: %w", err)
		}
		keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
	}

	// Add sort key if present
	if skValue, hasSK := pk["sk"]; hasSK && metadata.PrimaryKey.SortKey != nil {
		av, err := q.db.converter.ToAttributeValue(skValue)
		if err != nil {
			return fmt.Errorf("failed to convert sort key: %w", err)
		}
		keyMap[metadata.PrimaryKey.SortKey.DBName] = av
	}

	// Build update expression
	builder := expr.NewBuilder()

	modelValue := reflect.ValueOf(q.model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}

	// Determine which fields to update
	fieldsToUpdate := fields
	if len(fieldsToUpdate) == 0 {
		// If no fields specified, update all non-zero fields except primary keys and special fields
		fieldsToUpdate = []string{}
		for fieldName, fieldMeta := range metadata.Fields {
			if fieldMeta.IsPK || fieldMeta.IsSK || fieldMeta.IsCreatedAt {
				continue
			}
			fieldValue := modelValue.FieldByIndex([]int{fieldMeta.Index})
			if !fieldValue.IsZero() || !fieldMeta.OmitEmpty {
				fieldsToUpdate = append(fieldsToUpdate, fieldName)
			}
		}
	}

	// Build SET expressions
	for _, fieldName := range fieldsToUpdate {
		fieldMeta, exists := metadata.Fields[fieldName]
		if !exists {
			continue
		}

		fieldValue := modelValue.FieldByIndex([]int{fieldMeta.Index})

		// Handle special fields
		if fieldMeta.IsUpdatedAt {
			// Always update to current time
			builder.AddUpdateSet(fieldMeta.DBName, time.Now())
		} else if fieldMeta.IsVersion {
			// Increment version
			builder.AddUpdateAdd(fieldMeta.DBName, int64(1))
		} else {
			// Regular field update
			value := fieldValue.Interface()
			builder.AddUpdateSet(fieldMeta.DBName, value)
		}
	}

	// Always update updated_at if it exists
	if metadata.UpdatedAtField != nil {
		builder.AddUpdateSet(metadata.UpdatedAtField.DBName, time.Now())
	}

	// Add version check if version field exists
	if metadata.VersionField != nil {
		currentVersion := modelValue.FieldByIndex([]int{metadata.VersionField.Index}).Int()
		builder.AddConditionExpression(metadata.VersionField.DBName, "=", currentVersion)
	}

	// Build the update expression
	components := builder.Build()

	// Build UpdateItem input
	input := &dynamodb.UpdateItemInput{
		TableName:                 aws.String(metadata.TableName),
		Key:                       keyMap,
		UpdateExpression:          aws.String(components.UpdateExpression),
		ExpressionAttributeNames:  components.ExpressionAttributeNames,
		ExpressionAttributeValues: components.ExpressionAttributeValues,
	}

	if components.ConditionExpression != "" {
		input.ConditionExpression = aws.String(components.ConditionExpression)
	}

	// Execute UpdateItem
	_, err := q.db.session.Client().UpdateItem(q.ctx, input)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return errors.ErrConditionFailed
		}
		return fmt.Errorf("failed to update item: %w", err)
	}

	return nil
}

func (q *query) deleteItem(metadata *model.Metadata) error {
	// Extract primary key from conditions
	pk := q.extractPrimaryKey(metadata)
	if pk == nil {
		return fmt.Errorf("delete requires primary key in conditions")
	}

	// Build key map
	keyMap := make(map[string]types.AttributeValue)

	// Add partition key
	if pkValue, hasPK := pk["pk"]; hasPK {
		av, err := q.db.converter.ToAttributeValue(pkValue)
		if err != nil {
			return fmt.Errorf("failed to convert partition key: %w", err)
		}
		keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
	}

	// Add sort key if present
	if skValue, hasSK := pk["sk"]; hasSK && metadata.PrimaryKey.SortKey != nil {
		av, err := q.db.converter.ToAttributeValue(skValue)
		if err != nil {
			return fmt.Errorf("failed to convert sort key: %w", err)
		}
		keyMap[metadata.PrimaryKey.SortKey.DBName] = av
	}

	// Build DeleteItem input
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(metadata.TableName),
		Key:       keyMap,
	}

	// Add condition expression if we have additional conditions
	builder := expr.NewBuilder()
	hasConditions := false

	// Check for version field condition
	if metadata.VersionField != nil && q.model != nil {
		modelValue := reflect.ValueOf(q.model)
		if modelValue.Kind() == reflect.Ptr {
			modelValue = modelValue.Elem()
		}
		versionValue := modelValue.FieldByIndex([]int{metadata.VersionField.Index})
		if !versionValue.IsZero() {
			builder.AddConditionExpression(metadata.VersionField.DBName, "=", versionValue.Int())
			hasConditions = true
		}
	}

	// Add any other conditions from the query
	for _, cond := range q.conditions {
		// Skip primary key conditions as they're already in the key
		fieldMeta, exists := metadata.Fields[cond.field]
		if exists && (fieldMeta.IsPK || fieldMeta.IsSK) {
			continue
		}

		builder.AddConditionExpression(cond.field, cond.op, cond.value)
		hasConditions = true
	}

	if hasConditions {
		components := builder.Build()
		if components.ConditionExpression != "" {
			input.ConditionExpression = aws.String(components.ConditionExpression)
			input.ExpressionAttributeNames = components.ExpressionAttributeNames
			input.ExpressionAttributeValues = components.ExpressionAttributeValues
		}
	}

	// Execute DeleteItem
	_, err := q.db.session.Client().DeleteItem(q.ctx, input)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return errors.ErrConditionFailed
		}
		return fmt.Errorf("failed to delete item: %w", err)
	}

	return nil
}

// errorQuery is a query that always returns an error
type errorQuery struct {
	err error
}

func (e *errorQuery) Where(field string, op string, value interface{}) core.Query { return e }
func (e *errorQuery) Index(indexName string) core.Query                           { return e }
func (e *errorQuery) Filter(expression string, values ...interface{}) core.Query  { return e }
func (e *errorQuery) OrderBy(field string, order string) core.Query               { return e }
func (e *errorQuery) Limit(limit int) core.Query                                  { return e }
func (e *errorQuery) Offset(offset int) core.Query                                { return e }
func (e *errorQuery) Select(fields ...string) core.Query                          { return e }
func (e *errorQuery) First(dest interface{}) error                                { return e.err }
func (e *errorQuery) All(dest interface{}) error                                  { return e.err }
func (e *errorQuery) Count() (int64, error)                                       { return 0, e.err }
func (e *errorQuery) Create() error                                               { return e.err }
func (e *errorQuery) Update(fields ...string) error                               { return e.err }
func (e *errorQuery) Delete() error                                               { return e.err }
func (e *errorQuery) Scan(dest interface{}) error                                 { return e.err }
func (e *errorQuery) BatchGet(keys []interface{}, dest interface{}) error         { return e.err }
func (e *errorQuery) BatchCreate(items interface{}) error                         { return e.err }
func (e *errorQuery) WithContext(ctx context.Context) core.Query                  { return e }

// Re-export types for convenience
type (
	Config = session.Config
)

// TransactionFunc executes a function within a database transaction
// This is the actual implementation that uses our sophisticated transaction support
func (db *DB) TransactionFunc(fn func(tx *transaction.Transaction) error) error {
	// Create a new transaction
	tx := transaction.NewTransaction(db.session, db.registry, db.converter)
	tx = tx.WithContext(db.ctx)

	// Execute the transaction function
	if err := fn(tx); err != nil {
		// Rollback on error
		_ = tx.Rollback()
		return err
	}

	// Commit the transaction
	return tx.Commit()
}

// Package dynamorm provides a type-safe ORM for Amazon DynamoDB in Go
package dynamorm

import (
	"context"
	"errors"
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
	customerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/marshal"
	"github.com/pay-theory/dynamorm/pkg/model"
	queryPkg "github.com/pay-theory/dynamorm/pkg/query"
	"github.com/pay-theory/dynamorm/pkg/schema"
	"github.com/pay-theory/dynamorm/pkg/session"
	"github.com/pay-theory/dynamorm/pkg/transaction"
	pkgTypes "github.com/pay-theory/dynamorm/pkg/types"
)

// DB is the main DynamORM database instance
type DB struct {
	session             *session.Session
	registry            *model.Registry
	converter           *pkgTypes.Converter
	marshaler           *marshal.Marshaler // Optimized marshaler
	ctx                 context.Context
	mu                  sync.RWMutex
	lambdaDeadline      time.Time // Lambda execution deadline for timeout handling
	lambdaTimeoutBuffer time.Duration
}

// New creates a new DynamORM instance with the given configuration
func New(config session.Config) (core.ExtendedDB, error) {
	sess, err := session.NewSession(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &DB{
		session:   sess,
		registry:  model.NewRegistry(),
		converter: pkgTypes.NewConverter(),
		marshaler: marshal.New(),
		ctx:       context.Background(),
	}, nil
}

// NewBasic creates a new DynamORM instance that returns the basic DB interface
// Use this when you only need core functionality and want easier mocking
func NewBasic(config session.Config) (core.DB, error) {
	return New(config)
}

// Model returns a new query builder for the given model
func (db *DB) Model(model any) core.Query {
	// Ensure model is registered
	if err := db.registry.Register(model); err != nil {
		// Return a query that will error on execution
		return &errorQuery{err: err}
	}

	return &query{
		db:      db,
		model:   model,
		ctx:     db.ctx,
		builder: expr.NewBuilder(),
	}
}

// Transaction executes a function within a database transaction
func (db *DB) Transaction(fn func(tx *core.Tx) error) error {
	// For now, we'll use a simple wrapper that doesn't support full transaction features
	// Users should use TransactionFunc for full transaction support
	tx := &core.Tx{}
	return fn(tx)
}

// AutoMigrate creates or updates tables based on the given models
func (db *DB) AutoMigrate(models ...any) error {
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

// AutoMigrateWithOptions performs enhanced auto-migration with data copy support
func (db *DB) AutoMigrateWithOptions(model any, opts ...any) error {
	// Convert opts to the expected type
	var options []schema.AutoMigrateOption
	for _, opt := range opts {
		if option, ok := opt.(schema.AutoMigrateOption); ok {
			options = append(options, option)
		} else {
			return fmt.Errorf("invalid option type: expected schema.AutoMigrateOption, got %T", opt)
		}
	}

	manager := schema.NewManager(db.session, db.registry)
	return manager.AutoMigrateWithOptions(model, options...)
}

// CreateTable creates a DynamoDB table for the given model
func (db *DB) CreateTable(model any, opts ...any) error {
	// Register model first
	if err := db.registry.Register(model); err != nil {
		return fmt.Errorf("failed to register model %T: %w", model, err)
	}

	// Convert opts to the expected type
	var options []schema.TableOption
	for _, opt := range opts {
		if option, ok := opt.(schema.TableOption); ok {
			options = append(options, option)
		} else {
			return fmt.Errorf("invalid option type: expected schema.TableOption, got %T", opt)
		}
	}

	manager := schema.NewManager(db.session, db.registry)
	return manager.CreateTable(model, options...)
}

// EnsureTable checks if a table exists for the model and creates it if not
func (db *DB) EnsureTable(model any) error {
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
func (db *DB) DeleteTable(model any) error {
	metadata, err := db.registry.GetMetadata(model)
	if err != nil {
		return err
	}

	manager := schema.NewManager(db.session, db.registry)
	return manager.DeleteTable(metadata.TableName)
}

// DescribeTable returns the table description for the given model
func (db *DB) DescribeTable(model any) (any, error) {
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

// Migrate runs all pending migrations
func (db *DB) Migrate() error {
	// DynamORM doesn't support traditional migrations
	// Use infrastructure as code tools like Terraform or CloudFormation instead
	return fmt.Errorf("DynamORM does not support migrations. Use infrastructure as code tools (Terraform, CloudFormation) or AutoMigrate for development")
}

// WithContext returns a new DB instance with the given context
func (db *DB) WithContext(ctx context.Context) core.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()

	return &DB{
		session:             db.session,
		registry:            db.registry,
		converter:           db.converter,
		marshaler:           db.marshaler,
		ctx:                 ctx,
		lambdaDeadline:      db.lambdaDeadline,
		lambdaTimeoutBuffer: db.lambdaTimeoutBuffer,
	}
}

// WithLambdaTimeout sets a deadline based on Lambda context
func (db *DB) WithLambdaTimeout(ctx context.Context) core.DB {
	deadline, ok := ctx.Deadline()
	if !ok {
		return db
	}

	// Leave a buffer for Lambda cleanup
	buffer := db.lambdaTimeoutBuffer
	if buffer == 0 {
		buffer = 500 * time.Millisecond // Default buffer
	}
	adjustedDeadline := deadline.Add(-buffer)

	db.mu.RLock()
	defer db.mu.RUnlock()

	return &DB{
		session:             db.session,
		registry:            db.registry,
		converter:           db.converter,
		marshaler:           db.marshaler,
		ctx:                 ctx,
		lambdaDeadline:      adjustedDeadline,
		lambdaTimeoutBuffer: db.lambdaTimeoutBuffer,
	}
}

// WithLambdaTimeoutBuffer sets a custom timeout buffer for Lambda execution
func (db *DB) WithLambdaTimeoutBuffer(buffer time.Duration) core.DB {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.lambdaTimeoutBuffer = buffer
	return db
}

// query implements the core.Query interface
type query struct {
	db      *DB
	model   any
	ctx     context.Context
	builder *expr.Builder

	// Query conditions
	conditions []condition
	indexName  string
	orderBy    *orderBy
	limit      *int
	offset     *int
	fields     []string

	// Parallel scan fields
	segment       *int32
	totalSegments *int32

	// Pagination fields
	exclusiveStartKey string
}

type condition struct {
	field string
	op    string
	value any
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
		return fmt.Errorf("lambda timeout exceeded")
	}

	// Use configurable buffer, default to 100ms
	buffer := q.db.lambdaTimeoutBuffer
	if buffer == 0 {
		buffer = 100 * time.Millisecond
	}

	// If we have less than the buffer, consider it too close to timeout
	if remaining < buffer {
		return fmt.Errorf("lambda timeout imminent: only %v remaining", remaining)
	}

	return nil
}

// Where adds a condition to the query
func (q *query) Where(field string, op string, value any) core.Query {
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

// Filter adds an AND filter condition
func (q *query) Filter(field string, op string, value any) core.Query {
	q.builder.AddFilterCondition("AND", field, op, value)
	return q
}

// OrFilter adds an OR filter condition
func (q *query) OrFilter(field string, op string, value any) core.Query {
	q.builder.AddFilterCondition("OR", field, op, value)
	return q
}

// FilterGroup adds a grouped AND filter condition
func (q *query) FilterGroup(fn func(q core.Query)) core.Query {
	q.addGroup("AND", fn)
	return q
}

// OrFilterGroup adds a grouped OR filter condition
func (q *query) OrFilterGroup(fn func(q core.Query)) core.Query {
	q.addGroup("OR", fn)
	return q
}

func (q *query) addGroup(logicalOp string, fn func(q core.Query)) {
	// Create a new sub-query and builder for the group
	subBuilder := expr.NewBuilder()
	subQuery := &query{
		db:      q.db,
		model:   q.model,
		ctx:     q.ctx,
		builder: subBuilder,
	}

	// Execute the user's function to build the sub-query
	fn(subQuery)

	// Build the components from the sub-query
	components := subBuilder.Build()

	// Add the built group to the main builder
	q.builder.AddGroupFilter(logicalOp, components)
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
func (q *query) First(dest any) error {
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
		return customerrors.ErrItemNotFound
	}

	// Copy first item to dest
	reflect.ValueOf(dest).Elem().Set(resultsValue.Index(0))
	return nil
}

// All retrieves all matching items
func (q *query) All(dest any) error {
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
func (q *query) Scan(dest any) error {
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
func (q *query) BatchGet(keys []any, dest any) error {
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
		case map[string]any:
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
			// Check if key is a struct with the same type as our model
			keyValue := reflect.ValueOf(key)
			if keyValue.Kind() == reflect.Ptr {
				keyValue = keyValue.Elem()
			}

			if keyValue.Kind() == reflect.Struct {
				// Extract primary key fields from struct
				for _, field := range metadata.Fields {
					if field.IsPK {
						fieldValue := keyValue.FieldByIndex([]int{field.Index})
						av, err := q.db.converter.ToAttributeValue(fieldValue.Interface())
						if err != nil {
							return fmt.Errorf("failed to convert partition key: %w", err)
						}
						keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
					} else if field.IsSK && metadata.PrimaryKey.SortKey != nil {
						fieldValue := keyValue.FieldByIndex([]int{field.Index})
						av, err := q.db.converter.ToAttributeValue(fieldValue.Interface())
						if err != nil {
							return fmt.Errorf("failed to convert sort key: %w", err)
						}
						keyMap[metadata.PrimaryKey.SortKey.DBName] = av
					}
				}
			} else {
				// Key is just the partition key value
				av, err := q.db.converter.ToAttributeValue(key)
				if err != nil {
					return fmt.Errorf("failed to convert partition key: %w", err)
				}
				keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
			}
		}

		// Validate that we have at least a partition key
		if len(keyMap) == 0 {
			return fmt.Errorf("invalid key: missing partition key")
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
func (q *query) BatchCreate(items any) error {
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

// BatchDelete deletes multiple items by their primary keys
func (q *query) BatchDelete(keys []any) error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Process keys in batches of 25 (DynamoDB limit)
	const batchSize = 25

	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}

		// Build batch write request
		writeRequests := make([]types.WriteRequest, 0, end-i)

		for j := i; j < end; j++ {
			key := keys[j]
			keyMap := make(map[string]types.AttributeValue)

			// Handle different key formats
			switch k := key.(type) {
			case map[string]any:
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
				// Check if key is a struct with the same type as our model
				keyValue := reflect.ValueOf(key)
				if keyValue.Kind() == reflect.Ptr {
					keyValue = keyValue.Elem()
				}

				if keyValue.Kind() == reflect.Struct {
					// Extract primary key fields from struct
					for _, field := range metadata.Fields {
						if field.IsPK {
							fieldValue := keyValue.FieldByIndex([]int{field.Index})
							av, err := q.db.converter.ToAttributeValue(fieldValue.Interface())
							if err != nil {
								return fmt.Errorf("failed to convert partition key: %w", err)
							}
							keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
						} else if field.IsSK && metadata.PrimaryKey.SortKey != nil {
							fieldValue := keyValue.FieldByIndex([]int{field.Index})
							av, err := q.db.converter.ToAttributeValue(fieldValue.Interface())
							if err != nil {
								return fmt.Errorf("failed to convert sort key: %w", err)
							}
							keyMap[metadata.PrimaryKey.SortKey.DBName] = av
						}
					}
				} else {
					// Key is just the partition key value
					av, err := q.db.converter.ToAttributeValue(key)
					if err != nil {
						return fmt.Errorf("failed to convert partition key: %w", err)
					}
					keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
				}
			}

			// Validate that we have at least a partition key
			if len(keyMap) == 0 {
				return fmt.Errorf("invalid key at index %d: missing partition key", j)
			}

			writeRequests = append(writeRequests, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: keyMap,
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
				return fmt.Errorf("failed to batch delete items: %w", err)
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

func (q *query) extractPrimaryKey(metadata *model.Metadata) map[string]any {
	pk := make(map[string]any)

	// First try to extract from conditions
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

	// If no primary key found in conditions, try to extract from model
	if _, hasPK := pk["pk"]; !hasPK && q.model != nil {
		modelValue := reflect.ValueOf(q.model)
		if modelValue.Kind() == reflect.Ptr {
			modelValue = modelValue.Elem()
		}

		// Extract primary key from model
		if metadata.PrimaryKey.PartitionKey != nil {
			pkField := modelValue.FieldByIndex([]int{metadata.PrimaryKey.PartitionKey.Index})
			if !pkField.IsZero() {
				pk["pk"] = pkField.Interface()
			}
		}

		// Extract sort key from model if exists
		if metadata.PrimaryKey.SortKey != nil {
			skField := modelValue.FieldByIndex([]int{metadata.PrimaryKey.SortKey.Index})
			if !skField.IsZero() {
				pk["sk"] = skField.Interface()
			}
		}
	}

	// Must have at least partition key
	if _, hasPK := pk["pk"]; !hasPK {
		return nil
	}

	return pk
}

func (q *query) getItem(metadata *model.Metadata, pk map[string]any, dest any) error {
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
		return customerrors.ErrItemNotFound
	}

	// Unmarshal item to destination
	return q.unmarshalItem(output.Item, dest, metadata)
}

// unmarshalItem converts DynamoDB item to Go struct
func (q *query) unmarshalItem(item map[string]types.AttributeValue, dest any, metadata *model.Metadata) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr {
		return fmt.Errorf("destination must be a pointer")
	}
	destValue = destValue.Elem()

	// Handle map destination (e.g., when ExecuteWithResult is used with a map)
	if destValue.Kind() == reflect.Map {
		// If it's a map, just convert each attribute value
		if destValue.IsNil() {
			destValue.Set(reflect.MakeMap(destValue.Type()))
		}

		for attrName, attrValue := range item {
			// Convert the attribute value to the appropriate Go type
			var val any
			if err := q.db.converter.FromAttributeValue(attrValue, &val); err != nil {
				return fmt.Errorf("failed to unmarshal field %s: %w", attrName, err)
			}

			// Set the value in the map
			destValue.SetMapIndex(reflect.ValueOf(attrName), reflect.ValueOf(val))
		}

		return nil
	}

	// Handle struct destination (original behavior)
	if destValue.Kind() != reflect.Struct {
		return fmt.Errorf("destination must be a pointer to a struct or map")
	}

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
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to put item: %w", err)
	}

	// Update timestamp fields in the original model
	q.updateTimestampsInModel(metadata)

	return nil
}

// updateTimestampsInModel updates the created_at and updated_at fields in the original model
func (q *query) updateTimestampsInModel(metadata *model.Metadata) {
	modelValue := reflect.ValueOf(q.model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}

	now := time.Now()

	// Update timestamp fields
	for _, fieldMeta := range metadata.Fields {
		if fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt {
			field := modelValue.FieldByIndex([]int{fieldMeta.Index})
			if field.CanSet() && field.Type() == reflect.TypeOf(time.Time{}) {
				field.Set(reflect.ValueOf(now))
			}
		}
	}
}

// marshalItem converts a Go struct to DynamoDB item
func (q *query) marshalItem(model any, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
	// Use optimized marshaler if available
	if q.db.marshaler != nil {
		return q.db.marshaler.MarshalItem(model, metadata)
	}

	// Fall back to reflection-based marshaling
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
	var ccfe *types.ConditionalCheckFailedException
	return errors.As(err, &ccfe)
}

// executeQuery performs a DynamoDB Query operation
func (q *query) executeQuery(metadata *model.Metadata, keyConditions []condition, filterConditions []condition) ([]map[string]types.AttributeValue, error) {
	builder := expr.NewBuilder()

	// Add key conditions
	for _, cond := range keyConditions {
		fieldMeta := metadata.Fields[cond.field]
		builder.AddKeyCondition(fieldMeta.DBName, cond.op, cond.value)
	}

	// Add filter conditions
	for _, cond := range filterConditions {
		fieldMeta, exists := metadata.Fields[cond.field]
		if exists {
			builder.AddFilterCondition("AND", fieldMeta.DBName, cond.op, cond.value)
		} else {
			builder.AddFilterCondition("AND", cond.field, cond.op, cond.value)
		}
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
			builder.AddFilterCondition("AND", fieldMeta.DBName, cond.op, cond.value)
		} else {
			builder.AddFilterCondition("AND", cond.field, cond.op, cond.value)
		}
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
func (q *query) unmarshalItems(items []map[string]types.AttributeValue, dest any, metadata *model.Metadata) error {
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
		fieldMeta := metadata.Fields[cond.field]
		builder.AddKeyCondition(fieldMeta.DBName, cond.op, cond.value)
	}

	// Add filter conditions
	for _, cond := range filterConditions {
		fieldMeta, exists := metadata.Fields[cond.field]
		if exists {
			builder.AddFilterCondition("AND", fieldMeta.DBName, cond.op, cond.value)
		} else {
			builder.AddFilterCondition("AND", cond.field, cond.op, cond.value)
		}
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
			builder.AddFilterCondition("AND", fieldMeta.DBName, cond.op, cond.value)
		} else {
			builder.AddFilterCondition("AND", cond.field, cond.op, cond.value)
		}
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
	// Extract primary key from conditions or model
	pk := q.extractPrimaryKey(metadata)
	if pk == nil {
		return fmt.Errorf("update requires primary key")
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
			if fieldMeta.IsPK || fieldMeta.IsSK || fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt {
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
			return customerrors.ErrConditionFailed
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
		if fieldMeta, exists := metadata.Fields[cond.field]; exists && (fieldMeta.IsPK || fieldMeta.IsSK) {
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
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to delete item: %w", err)
	}

	return nil
}

// errorQuery is a query that always returns an error
type errorQuery struct {
	err error
}

func (e *errorQuery) Where(field string, op string, value any) core.Query  { return e }
func (e *errorQuery) Index(indexName string) core.Query                    { return e }
func (e *errorQuery) Filter(field string, op string, value any) core.Query { return e }
func (e *errorQuery) OrFilter(field string, op string, value any) core.Query {
	return e
}
func (e *errorQuery) FilterGroup(fn func(core.Query)) core.Query { return e }
func (e *errorQuery) OrFilterGroup(fn func(core.Query)) core.Query {
	return e
}
func (e *errorQuery) OrderBy(field string, order string) core.Query     { return e }
func (e *errorQuery) Limit(limit int) core.Query                        { return e }
func (e *errorQuery) Offset(offset int) core.Query                      { return e }
func (e *errorQuery) Select(fields ...string) core.Query                { return e }
func (e *errorQuery) First(dest any) error                              { return e.err }
func (e *errorQuery) All(dest any) error                                { return e.err }
func (e *errorQuery) Count() (int64, error)                             { return 0, e.err }
func (e *errorQuery) Create() error                                     { return e.err }
func (e *errorQuery) Update(fields ...string) error                     { return e.err }
func (e *errorQuery) Delete() error                                     { return e.err }
func (e *errorQuery) Scan(dest any) error                               { return e.err }
func (e *errorQuery) BatchGet(keys []any, dest any) error               { return e.err }
func (e *errorQuery) BatchCreate(items any) error                       { return e.err }
func (e *errorQuery) BatchDelete(keys []any) error                      { return e.err }
func (e *errorQuery) BatchWrite(putItems []any, deleteKeys []any) error { return e.err }
func (e *errorQuery) BatchUpdateWithOptions(items []any, fields []string, options ...any) error {
	return e.err
}
func (e *errorQuery) WithContext(ctx context.Context) core.Query                 { return e }
func (e *errorQuery) AllPaginated(dest any) (*core.PaginatedResult, error)       { return nil, e.err }
func (e *errorQuery) UpdateBuilder() core.UpdateBuilder                          { return nil }
func (e *errorQuery) ParallelScan(segment int32, totalSegments int32) core.Query { return e }
func (e *errorQuery) ScanAllSegments(dest any, totalSegments int32) error        { return e.err }
func (e *errorQuery) Cursor(cursor string) core.Query                            { return e }
func (e *errorQuery) SetCursor(cursor string) error                              { return e.err }

// Re-export types for convenience
type (
	Config            = session.Config
	AutoMigrateOption = schema.AutoMigrateOption
)

// Re-export AutoMigrate options for convenience
var (
	WithBackupTable = schema.WithBackupTable
	WithDataCopy    = schema.WithDataCopy
	WithTargetModel = schema.WithTargetModel
	WithTransform   = schema.WithTransform
	WithBatchSize   = schema.WithBatchSize
)

// TransactionFunc executes a function within a database transaction
// This is the actual implementation that uses our sophisticated transaction support
func (db *DB) TransactionFunc(fn func(tx any) error) error {
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

// AllPaginated retrieves all matching items with pagination metadata
func (q *query) AllPaginated(dest any) (*core.PaginatedResult, error) {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return nil, err
	}

	// Get metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return nil, err
	}

	// Separate key conditions from filter conditions
	var keyConditions []condition
	var filterConditions []condition

	for _, cond := range q.conditions {
		fieldMeta, exists := metadata.Fields[cond.field]
		if !exists {
			return nil, fmt.Errorf("field %s not found in model", cond.field)
		}

		isKey := fieldMeta.IsPK || fieldMeta.IsSK ||
			(q.indexName != "" && q.isIndexKey(fieldMeta, q.indexName, metadata))

		if isKey {
			keyConditions = append(keyConditions, cond)
		} else {
			filterConditions = append(filterConditions, cond)
		}
	}

	// Determine operation type
	var items []map[string]types.AttributeValue
	var scannedCount int
	var lastEvaluatedKey map[string]types.AttributeValue

	if len(keyConditions) > 0 {
		// Use Query operation
		builder := expr.NewBuilder()

		// Add key conditions
		for _, cond := range keyConditions {
			fieldMeta := metadata.Fields[cond.field]
			builder.AddKeyCondition(fieldMeta.DBName, cond.op, cond.value)
		}

		// Add filter conditions
		for _, cond := range filterConditions {
			fieldMeta, exists := metadata.Fields[cond.field]
			if exists {
				builder.AddFilterCondition("AND", fieldMeta.DBName, cond.op, cond.value)
			} else {
				builder.AddFilterCondition("AND", cond.field, cond.op, cond.value)
			}
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

		// Execute query
		output, err := q.db.session.Client().Query(q.ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to query items: %w", err)
		}

		items = output.Items
		scannedCount = int(output.ScannedCount)
		lastEvaluatedKey = output.LastEvaluatedKey
	} else {
		// Use Scan operation
		builder := expr.NewBuilder()

		// Add filter conditions
		for _, cond := range filterConditions {
			fieldMeta, exists := metadata.Fields[cond.field]
			if exists {
				builder.AddFilterCondition("AND", fieldMeta.DBName, cond.op, cond.value)
			} else {
				builder.AddFilterCondition("AND", cond.field, cond.op, cond.value)
			}
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

		// Execute scan
		output, err := q.db.session.Client().Scan(q.ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to scan items: %w", err)
		}

		items = output.Items
		scannedCount = int(output.ScannedCount)
		lastEvaluatedKey = output.LastEvaluatedKey
	}

	// Unmarshal items
	if len(items) > 0 {
		if err := q.unmarshalItems(items, dest, metadata); err != nil {
			return nil, err
		}
	}

	// Create result
	result := &core.PaginatedResult{
		Items:            dest,
		Count:            len(items),
		ScannedCount:     scannedCount,
		LastEvaluatedKey: lastEvaluatedKey,
		HasMore:          len(lastEvaluatedKey) > 0,
	}

	// Generate next cursor if there are more results
	if result.HasMore {
		cursor, err := queryPkg.EncodeCursor(lastEvaluatedKey, q.indexName, "")
		if err != nil {
			return nil, fmt.Errorf("failed to encode cursor: %w", err)
		}
		result.NextCursor = cursor
	}

	return result, nil
}

// UpdateBuilder returns a builder for complex update operations
func (q *query) UpdateBuilder() core.UpdateBuilder {
	// Create a local update builder to avoid circular dependency
	return &updateBuilder{
		query:        q,
		updates:      make(map[string]any),
		conditions:   make([]updateCondition, 0),
		returnValues: "NONE",
	}
}

// updateBuilder provides a fluent API for building update operations
type updateBuilder struct {
	query        *query
	updates      map[string]any
	conditions   []updateCondition
	returnValues string
}

type updateCondition struct {
	field    string
	operator string
	value    any
}

func (ub *updateBuilder) Set(field string, value any) core.UpdateBuilder {
	ub.updates[field] = value
	return ub
}

func (ub *updateBuilder) SetIfNotExists(field string, value any, defaultValue any) core.UpdateBuilder {
	// Store with special marker for SetIfNotExists operation
	ub.updates["SETIFNOTEXISTS:"+field] = defaultValue
	return ub
}

func (ub *updateBuilder) Add(field string, value any) core.UpdateBuilder {
	// Store with special marker for ADD operation
	ub.updates["ADD:"+field] = value
	return ub
}

func (ub *updateBuilder) Increment(field string) core.UpdateBuilder {
	return ub.Add(field, 1)
}

func (ub *updateBuilder) Decrement(field string) core.UpdateBuilder {
	return ub.Add(field, -1)
}

func (ub *updateBuilder) Remove(field string) core.UpdateBuilder {
	// Store with special marker for REMOVE operation
	ub.updates["REMOVE:"+field] = true
	return ub
}

func (ub *updateBuilder) Delete(field string, value any) core.UpdateBuilder {
	// Store with special marker for DELETE operation
	ub.updates["DELETE:"+field] = value
	return ub
}

func (ub *updateBuilder) AppendToList(field string, values any) core.UpdateBuilder {
	ub.updates["APPEND:"+field] = values
	return ub
}

func (ub *updateBuilder) PrependToList(field string, values any) core.UpdateBuilder {
	ub.updates["PREPEND:"+field] = values
	return ub
}

func (ub *updateBuilder) RemoveFromListAt(field string, index int) core.UpdateBuilder {
	ub.updates[fmt.Sprintf("REMOVE:%s[%d]", field, index)] = true
	return ub
}

func (ub *updateBuilder) SetListElement(field string, index int, value any) core.UpdateBuilder {
	ub.updates[fmt.Sprintf("%s[%d]", field, index)] = value
	return ub
}

func (ub *updateBuilder) Condition(field string, operator string, value any) core.UpdateBuilder {
	ub.conditions = append(ub.conditions, updateCondition{
		field:    field,
		operator: operator,
		value:    value,
	})
	return ub
}

func (ub *updateBuilder) ConditionExists(field string) core.UpdateBuilder {
	return ub.Condition(field, "attribute_exists", nil)
}

func (ub *updateBuilder) ConditionNotExists(field string) core.UpdateBuilder {
	return ub.Condition(field, "attribute_not_exists", nil)
}

func (ub *updateBuilder) ConditionVersion(currentVersion int64) core.UpdateBuilder {
	return ub.Condition("version", "=", currentVersion)
}

func (ub *updateBuilder) ReturnValues(option string) core.UpdateBuilder {
	ub.returnValues = option
	return ub
}

func (ub *updateBuilder) Execute() error {
	return ub.executeInternal(nil)
}

func (ub *updateBuilder) ExecuteWithResult(result any) error {
	return ub.executeInternal(result)
}

func (ub *updateBuilder) executeInternal(result any) error {
	// Check Lambda timeout
	if err := ub.query.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get metadata
	metadata, err := ub.query.db.registry.GetMetadata(ub.query.model)
	if err != nil {
		return err
	}

	// Extract primary key from conditions
	pk := ub.query.extractPrimaryKey(metadata)
	if pk == nil {
		return fmt.Errorf("update requires primary key in conditions")
	}

	// Build key map
	keyMap := make(map[string]types.AttributeValue)

	// Add partition key
	if pkValue, hasPK := pk["pk"]; hasPK {
		av, err := ub.query.db.converter.ToAttributeValue(pkValue)
		if err != nil {
			return fmt.Errorf("failed to convert partition key: %w", err)
		}
		keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
	}

	// Add sort key if present
	if skValue, hasSK := pk["sk"]; hasSK && metadata.PrimaryKey.SortKey != nil {
		av, err := ub.query.db.converter.ToAttributeValue(skValue)
		if err != nil {
			return fmt.Errorf("failed to convert sort key: %w", err)
		}
		keyMap[metadata.PrimaryKey.SortKey.DBName] = av
	}

	// Build update expression
	builder := expr.NewBuilder()

	// Track which fields we've already processed to avoid duplicates
	processedFields := make(map[string]bool)

	// Process updates
	for field, value := range ub.updates {
		// Handle special operation markers
		if strings.HasPrefix(field, "ADD:") {
			fieldName := field[4:]
			fieldMeta, exists := metadata.Fields[fieldName]
			if !exists {
				continue
			}
			builder.AddUpdateAdd(fieldMeta.DBName, value)
			processedFields[fieldName] = true
		} else if strings.HasPrefix(field, "REMOVE:") {
			fieldName := field[7:]
			// Check if it's an indexed remove like "Tags[1]"
			if idx := strings.Index(fieldName, "["); idx > 0 {
				// Handle list element removal
				actualField := fieldName[:idx]
				fieldMeta, exists := metadata.Fields[actualField]
				if !exists {
					continue
				}
				builder.AddUpdateRemove(fieldMeta.DBName + fieldName[idx:])
			} else {
				// Regular field removal
				fieldMeta, exists := metadata.Fields[fieldName]
				if !exists {
					continue
				}
				builder.AddUpdateRemove(fieldMeta.DBName)
				processedFields[fieldName] = true
			}
		} else if strings.HasPrefix(field, "DELETE:") {
			fieldName := field[7:]
			fieldMeta, exists := metadata.Fields[fieldName]
			if !exists {
				continue
			}
			builder.AddUpdateDelete(fieldMeta.DBName, value)
			processedFields[fieldName] = true
		} else if strings.HasPrefix(field, "SETIFNOTEXISTS:") {
			fieldName := field[15:]
			fieldMeta, exists := metadata.Fields[fieldName]
			if !exists {
				continue
			}
			// Skip if we've already processed this field with a regular SET
			if processedFields[fieldName] {
				continue
			}
			// Use AddUpdateFunction for if_not_exists
			err := builder.AddUpdateFunction(fieldMeta.DBName, "if_not_exists", fieldMeta.DBName, value)
			if err != nil {
				return fmt.Errorf("failed to add if_not_exists for %s: %w", fieldName, err)
			}
			processedFields[fieldName] = true
		} else if strings.HasPrefix(field, "APPEND:") {
			fieldName := field[7:]
			fieldMeta, exists := metadata.Fields[fieldName]
			if !exists {
				continue
			}
			// Use AddUpdateFunction for list_append operations
			err := builder.AddUpdateFunction(fieldMeta.DBName, "list_append", fieldMeta.DBName, value)
			if err != nil {
				return fmt.Errorf("failed to add list append: %w", err)
			}
			processedFields[fieldName] = true
		} else if strings.HasPrefix(field, "PREPEND:") {
			fieldName := field[8:]
			fieldMeta, exists := metadata.Fields[fieldName]
			if !exists {
				continue
			}
			// Use AddUpdateFunction for prepend (value first, then field)
			err := builder.AddUpdateFunction(fieldMeta.DBName, "list_append", value, fieldMeta.DBName)
			if err != nil {
				return fmt.Errorf("failed to add list prepend: %w", err)
			}
			processedFields[fieldName] = true
		} else if strings.Contains(field, "[") && strings.Contains(field, "]") {
			// Handle list element update like "Features[1]"
			idx := strings.Index(field, "[")
			fieldName := field[:idx]
			fieldMeta, exists := metadata.Fields[fieldName]
			if !exists {
				continue
			}
			builder.AddUpdateSet(fieldMeta.DBName+field[idx:], value)
		} else {
			// Regular SET operation
			fieldMeta, exists := metadata.Fields[field]
			if !exists {
				continue
			}
			// Skip if we've already processed this field
			if processedFields[field] {
				continue
			}
			builder.AddUpdateSet(fieldMeta.DBName, value)
			processedFields[field] = true
		}
	}

	// Only update updated_at if it hasn't been explicitly set by the user
	if metadata.UpdatedAtField != nil && !processedFields[metadata.UpdatedAtField.Name] {
		builder.AddUpdateSet(metadata.UpdatedAtField.DBName, time.Now())
	}

	// Add conditions from the builder
	for _, cond := range ub.conditions {
		if cond.operator == "attribute_exists" {
			builder.AddConditionExpression(cond.field, "attribute_exists", nil)
		} else if cond.operator == "attribute_not_exists" {
			builder.AddConditionExpression(cond.field, "attribute_not_exists", nil)
		} else {
			// For regular conditions, check if we need to use DB name
			fieldMeta, exists := metadata.Fields[cond.field]
			if exists {
				builder.AddConditionExpression(fieldMeta.DBName, cond.operator, cond.value)
			} else {
				builder.AddConditionExpression(cond.field, cond.operator, cond.value)
			}
		}
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

	// Set return values
	if ub.returnValues != "NONE" {
		input.ReturnValues = types.ReturnValue(ub.returnValues)
	}

	// Execute UpdateItem
	output, err := ub.query.db.session.Client().UpdateItem(ub.query.ctx, input)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to update item: %w", err)
	}

	// Handle return values if requested
	if result != nil && output.Attributes != nil {
		if err := ub.query.unmarshalItem(output.Attributes, result, metadata); err != nil {
			return fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}

	return nil
}

// ParallelScan configures parallel scanning with segment and total segments
func (q *query) ParallelScan(segment int32, totalSegments int32) core.Query {
	q.segment = &segment
	q.totalSegments = &totalSegments
	return q
}

// ScanAllSegments performs parallel scan across all segments automatically
func (q *query) ScanAllSegments(dest any, totalSegments int32) error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Ensure dest is a pointer to a slice
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("dest must be a pointer to a slice")
	}

	// Create a channel for results
	type segmentResult struct {
		items []map[string]types.AttributeValue
		err   error
	}
	resultsChan := make(chan segmentResult, totalSegments)

	// Launch parallel scans
	var wg sync.WaitGroup
	for i := int32(0); i < totalSegments; i++ {
		wg.Add(1)
		go func(segment int32) {
			defer wg.Done()

			// Clone the query for this segment
			segmentQuery := &query{
				ctx:           q.ctx,
				db:            q.db,
				model:         q.model,
				builder:       q.builder,
				conditions:    q.conditions,
				indexName:     q.indexName,
				orderBy:       q.orderBy,
				limit:         q.limit,
				offset:        q.offset,
				fields:        q.fields,
				segment:       &segment,
				totalSegments: &totalSegments,
			}

			// Execute scan for this segment using the cloned query
			items, err := segmentQuery.executeScanSegment(metadata, segment, totalSegments)
			if err != nil {
				resultsChan <- segmentResult{nil, fmt.Errorf("segment %d failed: %w", segment, err)}
				return
			}

			resultsChan <- segmentResult{items, nil}
		}(i)
	}

	// Wait for all segments to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	var allItems []map[string]types.AttributeValue
	for result := range resultsChan {
		if result.err != nil {
			return result.err
		}
		allItems = append(allItems, result.items...)
	}

	// Unmarshal items
	if len(allItems) > 0 {
		if err := q.unmarshalItems(allItems, dest, metadata); err != nil {
			return err
		}
	}

	return nil
}

// executeScanSegment executes a scan for a specific segment
func (q *query) executeScanSegment(metadata *model.Metadata, segment, totalSegments int32) ([]map[string]types.AttributeValue, error) {
	builder := expr.NewBuilder()

	// Add filter conditions
	var filterConditions []condition
	for _, cond := range q.conditions {
		fieldMeta, exists := metadata.Fields[cond.field]
		if !exists || (!fieldMeta.IsPK && !fieldMeta.IsSK) {
			filterConditions = append(filterConditions, cond)
		}
	}

	for _, cond := range filterConditions {
		fieldMeta, exists := metadata.Fields[cond.field]
		if exists {
			builder.AddFilterCondition("AND", fieldMeta.DBName, cond.op, cond.value)
		} else {
			builder.AddFilterCondition("AND", cond.field, cond.op, cond.value)
		}
	}

	// Add projection
	if len(q.fields) > 0 {
		builder.AddProjection(q.fields...)
	}

	// Build expressions
	components := builder.Build()

	// Build Scan input
	input := &dynamodb.ScanInput{
		TableName:     aws.String(metadata.TableName),
		Segment:       aws.Int32(segment),
		TotalSegments: aws.Int32(totalSegments),
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

	// Set limit if specified (but be careful with parallel scans)
	if q.limit != nil && *q.limit > 0 {
		// Distribute limit across segments
		segmentLimit := (*q.limit + int(totalSegments) - 1) / int(totalSegments)
		input.Limit = aws.Int32(int32(segmentLimit))
	}

	// Execute scan and collect results
	var items []map[string]types.AttributeValue
	paginator := dynamodb.NewScanPaginator(q.db.session.Client(), input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(q.ctx)
		if err != nil {
			return nil, err
		}

		items = append(items, output.Items...)

		// If we have a limit and reached it, stop
		if q.limit != nil && len(items) >= *q.limit/int(totalSegments) {
			break
		}
	}

	return items, nil
}

// Cursor sets the pagination cursor for the query
func (q *query) Cursor(cursor string) core.Query {
	q.exclusiveStartKey = cursor
	return q
}

// SetCursor sets the cursor from a string (alternative to Cursor)
func (q *query) SetCursor(cursor string) error {
	q.exclusiveStartKey = cursor
	return nil
}

// BatchWrite performs mixed batch write operations (puts and deletes)
func (q *query) BatchWrite(putItems []any, deleteKeys []any) error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Simply combine puts and deletes into batch operations
	// First, perform all puts using BatchCreate
	if len(putItems) > 0 {
		if err := q.BatchCreate(putItems); err != nil {
			return fmt.Errorf("failed to batch put items: %w", err)
		}
	}

	// Then, perform all deletes using BatchDelete
	if len(deleteKeys) > 0 {
		if err := q.BatchDelete(deleteKeys); err != nil {
			return fmt.Errorf("failed to batch delete items: %w", err)
		}
	}

	return nil
}

// BatchUpdateWithOptions performs batch update operations with custom options
func (q *query) BatchUpdateWithOptions(items []any, fields []string, options ...any) error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// For now, perform updates sequentially
	// In a full implementation, this would support parallel updates with options
	itemsValue := reflect.ValueOf(items)
	if itemsValue.Kind() == reflect.Ptr {
		itemsValue = itemsValue.Elem()
	}
	if itemsValue.Kind() != reflect.Slice {
		return fmt.Errorf("items must be a slice")
	}

	// Get metadata for the model type
	if itemsValue.Len() == 0 {
		return nil // Nothing to update
	}

	firstItem := itemsValue.Index(0).Interface()
	metadata, err := q.db.registry.GetMetadata(firstItem)
	if err != nil {
		return fmt.Errorf("failed to get metadata: %w", err)
	}

	for i := 0; i < itemsValue.Len(); i++ {
		item := itemsValue.Index(i).Interface()

		// Create a new query with the model
		updateQuery := q.db.Model(item)

		// Extract and set primary key conditions
		itemValue := reflect.ValueOf(item)
		if itemValue.Kind() == reflect.Ptr {
			itemValue = itemValue.Elem()
		}

		// Add partition key condition
		if metadata.PrimaryKey.PartitionKey != nil {
			pkField := itemValue.FieldByIndex([]int{metadata.PrimaryKey.PartitionKey.Index})
			updateQuery = updateQuery.Where(metadata.PrimaryKey.PartitionKey.Name, "=", pkField.Interface())
		}

		// Add sort key condition if present
		if metadata.PrimaryKey.SortKey != nil {
			skField := itemValue.FieldByIndex([]int{metadata.PrimaryKey.SortKey.Index})
			updateQuery = updateQuery.Where(metadata.PrimaryKey.SortKey.Name, "=", skField.Interface())
		}

		// Perform the update
		if err := updateQuery.Update(fields...); err != nil {
			return fmt.Errorf("failed to update item %d: %w", i, err)
		}
	}

	return nil
}

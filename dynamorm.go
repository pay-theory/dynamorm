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

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/pay-theory/dynamorm/internal/expr"
	"github.com/pay-theory/dynamorm/internal/numutil"
	"github.com/pay-theory/dynamorm/internal/reflectutil"
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

const (
	operatorBeginsWith = "BEGINS_WITH"
	operatorBetween    = "BETWEEN"
)

// DB is the main DynamORM database instance
type DB struct {
	lambdaDeadline      time.Time
	ctx                 context.Context
	session             *session.Session
	registry            *model.Registry
	converter           *pkgTypes.Converter
	marshaler           *marshal.Marshaler
	metadataCache       sync.Map
	lambdaTimeoutBuffer time.Duration
	mu                  sync.RWMutex
}

// UnmarshalItem unmarshals a DynamoDB AttributeValue map into a Go struct.
// This is the recommended way to unmarshal DynamoDB stream records or any
// DynamoDB items when using DynamORM.
//
// The function respects DynamORM struct tags (dynamorm:"pk", dynamorm:"attr:name", etc.)
// and handles all DynamoDB attribute types correctly.
//
// Example usage with DynamoDB Streams:
//
//	func processDynamoDBStream(record events.DynamoDBEventRecord) (*MyModel, error) {
//	    image := record.Change.NewImage
//	    if image == nil {
//	        return nil, nil
//	    }
//
//	    var model MyModel
//	    if err := dynamorm.UnmarshalItem(image, &model); err != nil {
//	        return nil, fmt.Errorf("failed to unmarshal: %w", err)
//	    }
//
//	    return &model, nil
//	}
func UnmarshalItem(item map[string]types.AttributeValue, dest interface{}) error {
	// Use the internal unmarshalItem function from the query executor
	return queryPkg.UnmarshalItem(item, dest)
}

// UnmarshalItems unmarshals a slice of DynamoDB AttributeValue maps into a slice of Go structs.
// This is useful for batch operations or when processing multiple items from a query result.
func UnmarshalItems(items []map[string]types.AttributeValue, dest interface{}) error {
	// Use the internal unmarshalItems function from the query executor
	return queryPkg.UnmarshalItems(items, dest)
}

// UnmarshalStreamImage unmarshals a DynamoDB stream image (from Lambda events) into a Go struct.
// This function handles the conversion from Lambda's events.DynamoDBAttributeValue to the standard types.AttributeValue
// and then unmarshals into your DynamORM model.
//
// Example usage:
//
//	func handleStream(record events.DynamoDBEventRecord) error {
//	    var order Order
//	    if err := dynamorm.UnmarshalStreamImage(record.Change.NewImage, &order); err != nil {
//	        return err
//	    }
//	    // Process order...
//	}
func UnmarshalStreamImage(streamImage map[string]events.DynamoDBAttributeValue, dest interface{}) error {
	// Convert Lambda event AttributeValues to SDK v2 AttributeValues
	item := make(map[string]types.AttributeValue, len(streamImage))
	for k, v := range streamImage {
		item[k] = convertLambdaAttributeValue(v)
	}

	return UnmarshalItem(item, dest)
}

// convertLambdaAttributeValue converts a Lambda event AttributeValue to SDK v2 AttributeValue
func convertLambdaAttributeValue(attr events.DynamoDBAttributeValue) types.AttributeValue {
	switch attr.DataType() {
	case events.DataTypeString:
		return &types.AttributeValueMemberS{Value: attr.String()}
	case events.DataTypeNumber:
		return &types.AttributeValueMemberN{Value: attr.Number()}
	case events.DataTypeBinary:
		return &types.AttributeValueMemberB{Value: attr.Binary()}
	case events.DataTypeBoolean:
		return &types.AttributeValueMemberBOOL{Value: attr.Boolean()}
	case events.DataTypeNull:
		return &types.AttributeValueMemberNULL{Value: true}
	case events.DataTypeList:
		list := make([]types.AttributeValue, 0, len(attr.List()))
		for _, item := range attr.List() {
			list = append(list, convertLambdaAttributeValue(item))
		}
		return &types.AttributeValueMemberL{Value: list}
	case events.DataTypeMap:
		m := make(map[string]types.AttributeValue)
		for k, v := range attr.Map() {
			m[k] = convertLambdaAttributeValue(v)
		}
		return &types.AttributeValueMemberM{Value: m}
	case events.DataTypeStringSet:
		return &types.AttributeValueMemberSS{Value: attr.StringSet()}
	case events.DataTypeNumberSet:
		return &types.AttributeValueMemberNS{Value: attr.NumberSet()}
	case events.DataTypeBinarySet:
		return &types.AttributeValueMemberBS{Value: attr.BinarySet()}
	default:
		// This shouldn't happen, but return NULL if unknown type
		return &types.AttributeValueMemberNULL{Value: true}
	}
}

// New creates a new DynamORM instance with the given configuration
func New(config session.Config) (core.ExtendedDB, error) {
	sess, err := session.NewSession(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	converter := pkgTypes.NewConverter()

	return &DB{
		session:   sess,
		registry:  model.NewRegistry(),
		converter: converter,
		marshaler: marshal.New(converter),
		ctx:       context.Background(),
	}, nil
}

// NewBasic creates a new DynamORM instance that returns the basic DB interface
// Use this when you only need core functionality and want easier mocking
func NewBasic(config session.Config) (core.DB, error) {
	return New(config)
}

// RegisterTypeConverter registers a custom converter for a specific Go type. This allows
// callers to control how values are marshaled to and unmarshaled from DynamoDB without
// forking the internal marshaler. Registering a converter clears any cached marshalers
// so subsequent operations use the new logic.
func (db *DB) RegisterTypeConverter(typ reflect.Type, converter pkgTypes.CustomConverter) error {
	if typ == nil {
		return fmt.Errorf("converter type cannot be nil")
	}
	if converter == nil {
		return fmt.Errorf("converter implementation cannot be nil")
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	db.converter.RegisterConverter(typ, converter)
	if db.marshaler != nil {
		db.marshaler.ClearCache()
	}
	return nil
}

// Model returns a new query builder for the given model
func (db *DB) Model(model any) core.Query {
	// Ensure model is registered
	if err := db.registry.Register(model); err != nil {
		// Log the error for debugging
		if db.ctx != nil {
			// Include context info if available
			return &errorQuery{err: fmt.Errorf("failed to register model %T: %w", model, err)}
		}
		// Return a query that will error on execution
		return &errorQuery{err: fmt.Errorf("failed to register model %T: %w", model, err)}
	}

	// Fast-path metadata lookup - cache for later use
	typ := reflect.TypeOf(model)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	// Check cache first
	if _, ok := db.metadataCache.Load(typ); !ok {
		// Get from registry and cache
		meta, err := db.registry.GetMetadata(model)
		if err != nil {
			return &errorQuery{err: fmt.Errorf("failed to get metadata for model %T: %w", model, err)}
		}
		db.metadataCache.Store(typ, meta)
	}

	// Use the context from the DB if query doesn't have one
	ctx := db.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	return &query{
		db:              db,
		model:           model,
		ctx:             ctx,
		builder:         expr.NewBuilderWithConverter(db.converter),
		conditions:      make([]condition, 0, 4), // Pre-allocate for typical use case
		writeConditions: make([]condition, 0),
		rawConditions:   make([]rawConditionExpression, 0),
	}
}

// Transaction executes a function within a database transaction
func (db *DB) Transaction(fn func(tx *core.Tx) error) error {
	// For now, we'll use a simple wrapper that doesn't support full transaction features
	// Users should use TransactionFunc for full transaction support
	tx := &core.Tx{}
	// Set the db field to avoid nil pointer panic
	tx.SetDB(db)
	return fn(tx)
}

// Transact returns a fluent transaction builder for composing TransactWriteItems requests.
func (db *DB) Transact() core.TransactionBuilder {
	builder := transaction.NewBuilder(db.session, db.registry, db.converter)
	if db.ctx != nil {
		builder.WithContext(db.ctx)
	}
	return builder
}

// TransactWrite executes the supplied function with a transaction builder and automatically commits it.
func (db *DB) TransactWrite(ctx context.Context, fn func(core.TransactionBuilder) error) error {
	if fn == nil {
		return fmt.Errorf("transaction function cannot be nil")
	}

	builder := db.Transact()
	if ctx != nil {
		builder = builder.WithContext(ctx)
	} else if db.ctx != nil {
		builder = builder.WithContext(db.ctx)
	}

	if err := fn(builder); err != nil {
		return err
	}

	return builder.Execute()
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
	if tableName, ok := model.(string); ok {
		manager := schema.NewManager(db.session, db.registry)
		return manager.DeleteTable(tableName)
	}

	// Register model first
	if err := db.registry.Register(model); err != nil {
		return fmt.Errorf("failed to register model %T: %w", model, err)
	}

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

	newDB := &DB{
		session:             db.session,
		registry:            db.registry,
		converter:           db.converter,
		marshaler:           db.marshaler,
		ctx:                 ctx,
		lambdaDeadline:      db.lambdaDeadline,
		lambdaTimeoutBuffer: db.lambdaTimeoutBuffer,
	}

	// Copy metadata cache
	db.metadataCache.Range(func(key, value any) bool {
		newDB.metadataCache.Store(key, value)
		return true
	})

	return newDB
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

	newDB := &DB{
		session:             db.session,
		registry:            db.registry,
		converter:           db.converter,
		marshaler:           db.marshaler,
		ctx:                 ctx,
		lambdaDeadline:      adjustedDeadline,
		lambdaTimeoutBuffer: db.lambdaTimeoutBuffer,
	}

	// Copy metadata cache
	db.metadataCache.Range(func(key, value any) bool {
		newDB.metadataCache.Store(key, value)
		return true
	})

	return newDB
}

// WithLambdaTimeoutBuffer sets a custom timeout buffer for Lambda execution
func (db *DB) WithLambdaTimeoutBuffer(buffer time.Duration) core.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()

	// Create new instance instead of modifying existing one to avoid race conditions
	newDB := &DB{
		session:             db.session,
		registry:            db.registry,
		converter:           db.converter,
		marshaler:           db.marshaler,
		ctx:                 db.ctx,
		lambdaDeadline:      db.lambdaDeadline,
		lambdaTimeoutBuffer: buffer, // Set the new buffer value
	}

	// Copy metadata cache
	db.metadataCache.Range(func(key, value any) bool {
		newDB.metadataCache.Store(key, value)
		return true
	})

	return newDB
}

// query implements the core.Query interface
type query struct {
	model             any
	ctx               context.Context
	builderErr        error
	offset            *int
	builder           *expr.Builder
	retryConfig       *retryConfig
	totalSegments     *int32
	segment           *int32
	db                *DB
	orderBy           *orderBy
	limit             *int
	indexName         string
	exclusiveStartKey string
	fields            []string
	rawConditions     []rawConditionExpression
	writeConditions   []condition
	conditions        []condition
	consistentRead    bool
}

type condition struct {
	value any
	field string
	op    string
}

type rawConditionExpression struct {
	values     map[string]any
	expression string
}

func normalizeOperator(op string) string {
	if op == "" {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(op))
}

type orderBy struct {
	field string
	order string
}

type retryConfig struct {
	maxRetries   int
	initialDelay time.Duration
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
	q.addFilterCondition("AND", field, op, value)
	return q
}

// OrFilter adds an OR filter condition
func (q *query) OrFilter(field string, op string, value any) core.Query {
	q.addFilterCondition("OR", field, op, value)
	return q
}

func (q *query) addFilterCondition(logicalOp, field, op string, value any) {
	normalizedOp := normalizeOperator(op)
	if normalizedOp == "" {
		q.recordBuilderError(fmt.Errorf("operator cannot be empty"))
		return
	}

	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		q.recordBuilderError(err)
		return
	}

	attrName := mapToAttributeName(metadata, field)
	if err := q.builder.AddFilterCondition(logicalOp, attrName, normalizedOp, value); err != nil {
		q.recordBuilderError(err)
	}
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

// IfNotExists adds an attribute_not_exists guard for the primary key
func (q *query) IfNotExists() core.Query {
	q.addPrimaryKeyCondition("attribute_not_exists")
	return q
}

// IfExists adds an attribute_exists guard for the primary key
func (q *query) IfExists() core.Query {
	q.addPrimaryKeyCondition("attribute_exists")
	return q
}

// WithCondition appends an additional conditional expression for writes
func (q *query) WithCondition(field, operator string, value any) core.Query {
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		q.recordBuilderError(err)
		return q
	}

	op := normalizeOperator(operator)
	if op == "" {
		q.recordBuilderError(fmt.Errorf("operator cannot be empty"))
		return q
	}

	if fieldMeta, exists := lookupField(metadata, field); exists {
		q.writeConditions = append(q.writeConditions, condition{
			field: fieldMeta.DBName,
			op:    op,
			value: value,
		})
		return q
	}

	q.writeConditions = append(q.writeConditions, condition{
		field: field,
		op:    op,
		value: value,
	})
	return q
}

// WithConditionExpression adds a raw condition expression
func (q *query) WithConditionExpression(exprStr string, values map[string]any) core.Query {
	exprStr = strings.TrimSpace(exprStr)
	if exprStr == "" {
		q.recordBuilderError(fmt.Errorf("condition expression cannot be empty"))
		return q
	}

	q.rawConditions = append(q.rawConditions, rawConditionExpression{
		expression: exprStr,
		values:     cloneRawConditionValues(values),
	})
	return q
}

func (q *query) addGroup(logicalOp string, fn func(q core.Query)) {
	// Create a new sub-query and builder for the group
	subBuilder := expr.NewBuilderWithConverter(q.db.converter)
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

// recordBuilderError memoizes the first builder error encountered
func (q *query) recordBuilderError(err error) {
	if err != nil && q.builderErr == nil {
		q.builderErr = err
	}
}

// checkBuilderError returns any previously recorded builder error
func (q *query) checkBuilderError() error {
	return q.builderErr
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
	if len(fields) == 0 {
		q.fields = fields
		return q
	}

	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		q.recordBuilderError(err)
		return q
	}

	resolved := make([]string, 0, len(fields))
	for _, field := range fields {
		resolved = append(resolved, mapToAttributeName(metadata, field))
	}

	q.fields = resolved
	return q
}

// ConsistentRead enables strongly consistent reads for Query operations
func (q *query) ConsistentRead() core.Query {
	q.consistentRead = true
	return q
}

// WithRetry configures retry behavior for eventually consistent reads
func (q *query) WithRetry(maxRetries int, initialDelay time.Duration) core.Query {
	q.retryConfig = &retryConfig{
		maxRetries:   maxRetries,
		initialDelay: initialDelay,
	}
	return q
}

// First retrieves the first matching item
func (q *query) First(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	if q.retryConfig != nil {
		return q.firstWithRetry(dest)
	}
	return q.firstInternal(dest)
}

// firstInternal is the actual implementation of First
func (q *query) firstInternal(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
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
		// Use optimized path when no projections are specified
		if len(q.fields) == 0 {
			return q.getItemDirect(metadata, pk, dest)
		}
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

// firstWithRetry executes First with retry logic
func (q *query) firstWithRetry(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	delay := q.retryConfig.initialDelay
	maxDelay := 5 * time.Second
	backoffFactor := 2.0

	for attempt := 0; attempt <= q.retryConfig.maxRetries; attempt++ {
		err := q.firstInternal(dest)

		// If successful or it's not a "not found" error, return
		if err == nil || (err != customerrors.ErrItemNotFound && attempt == q.retryConfig.maxRetries) {
			return err
		}

		// Don't sleep on the last attempt
		if attempt < q.retryConfig.maxRetries {
			time.Sleep(delay)

			// Calculate next delay with exponential backoff
			delay = time.Duration(float64(delay) * backoffFactor)
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}

	return customerrors.ErrItemNotFound
}

// All retrieves all matching items
func (q *query) All(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	if q.retryConfig != nil {
		return q.allWithRetry(dest)
	}
	return q.allInternal(dest)
}

func (q *query) partitionConditions(metadata *model.Metadata) (bool, []condition, []condition) {
	useQuery := false
	var keyConditions []condition
	var filterConditions []condition

	for _, cond := range q.conditions {
		normalizedCond := condition{
			field: cond.field,
			op:    normalizeOperator(cond.op),
			value: cond.value,
		}

		fieldMeta, exists := lookupField(metadata, normalizedCond.field)
		if !exists {
			filterConditions = append(filterConditions, normalizedCond)
			continue
		}

		isPK, isSK := q.determineKeyRoles(fieldMeta, metadata)

		if isPK || isSK {
			// DynamoDB supports these operators for key conditions:
			// Partition key: = (equality only)
			// Sort key: =, <, <=, >, >=, BETWEEN, BEGINS_WITH
			if normalizedCond.op == "=" || normalizedCond.op == operatorBeginsWith ||
				normalizedCond.op == "<" || normalizedCond.op == "<=" || normalizedCond.op == ">" || normalizedCond.op == ">=" ||
				normalizedCond.op == operatorBetween {
				// Partition keys still require equality. If a non-equality comparison is attempted
				// against the partition key, treat it as a filter to surface the correct Dynamo error.
				if isPK && normalizedCond.op != "=" {
					filterConditions = append(filterConditions, normalizedCond)
				} else {
					keyConditions = append(keyConditions, normalizedCond)
					useQuery = true
				}
			} else {
				filterConditions = append(filterConditions, normalizedCond)
			}
		} else {
			filterConditions = append(filterConditions, normalizedCond)
		}
	}

	return useQuery, keyConditions, filterConditions
}

// allInternal is the actual implementation of All
func (q *query) allInternal(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
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
	useQuery, keyConditions, filterConditions := q.partitionConditions(metadata)

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

// allWithRetry executes All with retry logic
func (q *query) allWithRetry(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	delay := q.retryConfig.initialDelay
	maxDelay := 5 * time.Second
	backoffFactor := 2.0

	// Get the slice value to check if results are empty
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	var lastErr error
	for attempt := 0; attempt <= q.retryConfig.maxRetries; attempt++ {
		// Clear the slice before each attempt
		destValue.Elem().Set(reflect.MakeSlice(destValue.Elem().Type(), 0, 0))

		err := q.allInternal(dest)
		lastErr = err

		// If we have an error (other than no results), return it
		if err != nil {
			// For actual errors, keep retrying unless it's the last attempt
			if attempt < q.retryConfig.maxRetries {
				time.Sleep(delay)
				delay = time.Duration(float64(delay) * backoffFactor)
				if delay > maxDelay {
					delay = maxDelay
				}
				continue
			}
			return err
		}

		// If successful and we have results, return
		if destValue.Elem().Len() > 0 {
			return nil
		}

		// No error but empty results - retry if not last attempt
		if attempt < q.retryConfig.maxRetries {
			time.Sleep(delay)

			// Calculate next delay with exponential backoff
			delay = time.Duration(float64(delay) * backoffFactor)
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}

	// Return success even if empty after retries (All doesn't error on empty results)
	// This maintains backward compatibility - callers should check if slice is empty
	return lastErr
}

// Count returns the number of matching items
func (q *query) Count() (int64, error) {
	if err := q.checkBuilderError(); err != nil {
		return 0, err
	}
	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return 0, err
	}

	// Determine if we should use Query or Scan
	useQuery, keyConditions, filterConditions := q.partitionConditions(metadata)

	// Execute count operation
	if useQuery {
		return q.executeQueryCount(metadata, keyConditions, filterConditions)
	}
	return q.executeScanCount(metadata, filterConditions)
}

// Create creates a new item
func (q *query) Create() error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Put item
	err = q.putItem(metadata)
	if err != nil {
		// Provide a more helpful error message for duplicate key errors
		if errors.Is(err, customerrors.ErrConditionFailed) {
			return fmt.Errorf("%w: item with the same key already exists", customerrors.ErrConditionFailed)
		}
		return err
	}

	return nil
}

// CreateOrUpdate creates a new item or updates an existing one (upsert)
func (q *query) CreateOrUpdate() error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Marshal the model to DynamoDB item
	item, err := q.marshalItem(q.model, metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal item: %w", err)
	}

	// Build PutItem input without condition expression (allowing overwrites)
	input := &dynamodb.PutItemInput{
		TableName: aws.String(metadata.TableName),
		Item:      item,
	}

	// Execute PutItem
	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for put item: %w", err)
	}

	_, err = client.PutItem(q.ctx, input)
	if err != nil {
		return fmt.Errorf("failed to put item: %w", err)
	}

	// Update timestamp fields in the original model
	q.updateTimestampsInModel(metadata)

	return nil
}

// Update updates the matching items
func (q *query) Update(fields ...string) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
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
	if err := q.checkBuilderError(); err != nil {
		return err
	}
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
	if err := q.checkBuilderError(); err != nil {
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

// BatchGet retrieves multiple items by their primary keys.
func (q *query) BatchGet(keys []any, dest any) error {
	return q.BatchGetWithOptions(keys, dest, nil)
}

// BatchGetWithOptions retrieves multiple items with advanced options.
func (q *query) BatchGetWithOptions(keys []any, dest any, opts *core.BatchGetOptions) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	internal, metadata, err := q.buildBatchGetQuery()
	if err != nil {
		return err
	}

	var rawItems []map[string]types.AttributeValue
	if err := internal.BatchGetWithOptions(keys, &rawItems, opts); err != nil {
		return err
	}

	return q.unmarshalItems(rawItems, dest, metadata)
}

// BatchGetBuilder returns a fluent builder for composing batch get operations.
func (q *query) BatchGetBuilder() core.BatchGetBuilder {
	if err := q.checkBuilderError(); err != nil {
		return &errorBatchGetBuilder{err: err}
	}
	if err := q.checkLambdaTimeout(); err != nil {
		return &errorBatchGetBuilder{err: err}
	}

	internal, metadata, err := q.buildBatchGetQuery()
	if err != nil {
		return &errorBatchGetBuilder{err: err}
	}

	return &batchGetBuilderWrapper{
		builder:  internal.BatchGetBuilder(),
		query:    q,
		metadata: metadata,
	}
}

func (q *query) buildBatchGetQuery() (*queryPkg.Query, *model.Metadata, error) {
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return nil, nil, err
	}

	client, err := q.db.session.Client()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get client for batch get: %w", err)
	}

	ctx := q.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	adapter := &metadataAdapter{metadata: metadata}
	executor := queryPkg.NewExecutor(client, ctx)
	internal := queryPkg.New(q.model, adapter, executor)
	internal.WithContext(ctx)

	if len(q.fields) > 0 {
		internal.Select(q.fields...)
	}
	if q.consistentRead {
		internal.ConsistentRead()
	}

	return internal, metadata, nil
}

// BatchCreate creates multiple items
func (q *query) BatchCreate(items any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	itemsValue, err := sliceValue(items)
	if err != nil {
		return err
	}

	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for batch create: %w", err)
	}

	// Process items in batches of 25 (DynamoDB limit)
	const batchSize = 25
	totalItems := itemsValue.Len()

	for i := 0; i < totalItems; i += batchSize {
		end := minInt(i+batchSize, totalItems)
		writeRequests, err := q.buildBatchCreateWriteRequests(itemsValue, i, end, metadata)
		if err != nil {
			return err
		}

		// Build BatchWriteItem input
		input := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				metadata.TableName: writeRequests,
			},
		}

		// Execute batch write with retries for unprocessed items
		if err := q.batchWriteWithRetries(q.ctx, client, input, "batch create", true); err != nil {
			return err
		}
	}

	return nil
}

// BatchDelete deletes multiple items by their primary keys
func (q *query) BatchDelete(keys []any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
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
	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for batch delete: %w", err)
	}

	for i := 0; i < len(keys); i += batchSize {
		end := minInt(i+batchSize, len(keys))
		writeRequests, err := q.buildBatchDeleteWriteRequests(keys, i, end, metadata)
		if err != nil {
			return err
		}

		// Build BatchWriteItem input
		input := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				metadata.TableName: writeRequests,
			},
		}

		// Execute batch write with retries for unprocessed items
		if err := q.batchWriteWithRetries(q.ctx, client, input, "batch delete", false); err != nil {
			return err
		}
	}

	return nil
}

func (q *query) buildBatchCreateWriteRequests(itemsValue reflect.Value, start, end int, metadata *model.Metadata) ([]types.WriteRequest, error) {
	writeRequests := make([]types.WriteRequest, 0, end-start)
	for i := start; i < end; i++ {
		itemValue := itemsValue.Index(i)
		if itemValue.Kind() == reflect.Ptr {
			itemValue = itemValue.Elem()
		}

		item, err := q.marshalItem(itemValue.Interface(), metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal item %d: %w", i, err)
		}

		writeRequests = append(writeRequests, types.WriteRequest{
			PutRequest: &types.PutRequest{
				Item: item,
			},
		})
	}
	return writeRequests, nil
}

func (q *query) buildBatchDeleteWriteRequests(keys []any, start, end int, metadata *model.Metadata) ([]types.WriteRequest, error) {
	writeRequests := make([]types.WriteRequest, 0, end-start)
	for i := start; i < end; i++ {
		keyMap, err := q.buildKeyMapForBatchDelete(keys[i], metadata)
		if err != nil {
			return nil, err
		}
		if len(keyMap) == 0 {
			return nil, fmt.Errorf("invalid key at index %d: missing partition key", i)
		}

		writeRequests = append(writeRequests, types.WriteRequest{
			DeleteRequest: &types.DeleteRequest{
				Key: keyMap,
			},
		})
	}
	return writeRequests, nil
}

func (q *query) buildKeyMapForBatchDelete(key any, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
	switch k := key.(type) {
	case map[string]any:
		return q.buildKeyMapFromPrimaryKey(metadata, k)
	default:
		return q.buildKeyMapFromAnyKey(key, metadata)
	}
}

func (q *query) buildKeyMapFromAnyKey(key any, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
	keyValue := reflect.ValueOf(key)
	if keyValue.IsValid() && keyValue.Kind() == reflect.Ptr {
		if keyValue.IsNil() {
			return q.buildKeyMapFromPartitionKeyValue(key, metadata)
		}
		keyValue = keyValue.Elem()
	}

	if keyValue.IsValid() && keyValue.Kind() == reflect.Struct {
		return q.buildKeyMapFromStructKey(keyValue, metadata)
	}

	return q.buildKeyMapFromPartitionKeyValue(key, metadata)
}

func (q *query) buildKeyMapFromStructKey(keyValue reflect.Value, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
	result := make(map[string]types.AttributeValue)

	for _, field := range metadata.Fields {
		switch {
		case field.IsPK:
			av, err := q.db.converter.ToAttributeValue(keyValue.FieldByIndex(field.IndexPath).Interface())
			if err != nil {
				return nil, fmt.Errorf("failed to convert partition key: %w", err)
			}
			result[metadata.PrimaryKey.PartitionKey.DBName] = av
		case field.IsSK && metadata.PrimaryKey.SortKey != nil:
			av, err := q.db.converter.ToAttributeValue(keyValue.FieldByIndex(field.IndexPath).Interface())
			if err != nil {
				return nil, fmt.Errorf("failed to convert sort key: %w", err)
			}
			result[metadata.PrimaryKey.SortKey.DBName] = av
		}
	}

	return result, nil
}

func (q *query) buildKeyMapFromPartitionKeyValue(key any, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
	av, err := q.db.converter.ToAttributeValue(key)
	if err != nil {
		return nil, fmt.Errorf("failed to convert partition key: %w", err)
	}

	return map[string]types.AttributeValue{
		metadata.PrimaryKey.PartitionKey.DBName: av,
	}, nil
}

func (q *query) batchWriteWithRetries(ctx context.Context, client *dynamodb.Client, input *dynamodb.BatchWriteItemInput, operation string, checkCtx bool) error {
	for {
		output, err := client.BatchWriteItem(ctx, input)
		if err != nil {
			return fmt.Errorf("failed to %s items: %w", operation, err)
		}
		if len(output.UnprocessedItems) == 0 {
			return nil
		}
		if checkCtx {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context canceled during %s retry: %w", operation, ctx.Err())
			default:
			}
		}

		input.RequestItems = output.UnprocessedItems
	}
}

func sliceValue(items any) (reflect.Value, error) {
	value := reflect.ValueOf(items)
	if !value.IsValid() {
		return reflect.Value{}, fmt.Errorf("items must be a slice")
	}

	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return reflect.Value{}, fmt.Errorf("items must be a slice")
		}
		value = value.Elem()
	}

	if value.Kind() != reflect.Slice {
		return reflect.Value{}, fmt.Errorf("items must be a slice")
	}

	return value, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// WithContext sets the context for the query
func (q *query) WithContext(ctx context.Context) core.Query {
	q.ctx = ctx
	return q
}

// Helper methods for basic CRUD operations

// lookupField provides consistent field lookup by checking both Go field names and DynamoDB attribute names
func lookupField(metadata *model.Metadata, fieldName string) (*model.FieldMetadata, bool) {
	// First check by Go field name
	if field, exists := metadata.Fields[fieldName]; exists {
		return field, true
	}

	// Then check by DynamoDB attribute name
	if field, exists := metadata.FieldsByDBName[fieldName]; exists {
		return field, true
	}

	return nil, false
}

func mapToAttributeName(metadata *model.Metadata, field string) string {
	if metadata == nil {
		return field
	}
	if fieldMeta, exists := lookupField(metadata, field); exists {
		return fieldMeta.DBName
	}
	return field
}

func cloneRawConditionValues(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for k, v := range values {
		cloned[k] = v
	}
	return cloned
}

func (q *query) addPrimaryKeyCondition(operator string) {
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		q.recordBuilderError(err)
		return
	}
	if metadata.PrimaryKey == nil || metadata.PrimaryKey.PartitionKey == nil {
		q.recordBuilderError(fmt.Errorf("primary key metadata missing"))
		return
	}

	op := strings.ToUpper(operator)
	q.writeConditions = append(q.writeConditions, condition{
		field: metadata.PrimaryKey.PartitionKey.DBName,
		op:    op,
	})

	if metadata.PrimaryKey.SortKey != nil && op == "ATTRIBUTE_EXISTS" {
		q.writeConditions = append(q.writeConditions, condition{
			field: metadata.PrimaryKey.SortKey.DBName,
			op:    op,
		})
	}
}

func (q *query) buildConditionExpression(metadata *model.Metadata, includeWhereConditions bool, skipKeyConditions bool, defaultIfEmpty bool) (string, map[string]string, map[string]types.AttributeValue, error) {
	builder := expr.NewBuilderWithConverter(q.db.converter)
	hasCondition, err := q.addWriteConditionsToBuilder(builder)
	if err != nil {
		return "", nil, nil, err
	}

	if includeWhereConditions {
		whereHasCondition, whereErr := q.addWhereConditionsToBuilder(builder, metadata, skipKeyConditions)
		if whereErr != nil {
			return "", nil, nil, whereErr
		}
		hasCondition = hasCondition || whereHasCondition
	}

	if defaultIfEmpty && !hasCondition && len(q.rawConditions) == 0 {
		if err = q.addDefaultNotExistsConditionsToBuilder(builder, metadata); err != nil {
			return "", nil, nil, err
		}
	}

	components := builder.Build()
	mergedExpr, mergedValues, err := q.mergeRawConditionExpressions(components.ConditionExpression, components.ExpressionAttributeValues)
	if err != nil {
		return "", nil, nil, err
	}

	return mergedExpr, components.ExpressionAttributeNames, mergedValues, nil
}

func (q *query) addWriteConditionsToBuilder(builder *expr.Builder) (bool, error) {
	hasCondition := false
	for _, cond := range q.writeConditions {
		if err := builder.AddConditionExpression(cond.field, cond.op, cond.value); err != nil {
			return false, fmt.Errorf("failed to add condition for %s: %w", cond.field, err)
		}
		hasCondition = true
	}
	return hasCondition, nil
}

func (q *query) addWhereConditionsToBuilder(builder *expr.Builder, metadata *model.Metadata, skipKeyConditions bool) (bool, error) {
	hasCondition := false
	for _, cond := range q.conditions {
		fieldMeta, exists := lookupField(metadata, cond.field)
		if !exists {
			continue
		}
		if skipKeyConditions && (fieldMeta.IsPK || fieldMeta.IsSK) {
			continue
		}
		if err := builder.AddConditionExpression(fieldMeta.DBName, normalizeOperator(cond.op), cond.value); err != nil {
			return false, fmt.Errorf("failed to add condition for %s: %w", cond.field, err)
		}
		hasCondition = true
	}
	return hasCondition, nil
}

func (q *query) addDefaultNotExistsConditionsToBuilder(builder *expr.Builder, metadata *model.Metadata) error {
	if metadata.PrimaryKey == nil || metadata.PrimaryKey.PartitionKey == nil {
		return fmt.Errorf("partition key metadata missing")
	}

	if err := builder.AddConditionExpression(metadata.PrimaryKey.PartitionKey.DBName, "attribute_not_exists", nil); err != nil {
		return fmt.Errorf("failed to add default partition key condition: %w", err)
	}
	if metadata.PrimaryKey.SortKey != nil {
		if err := builder.AddConditionExpression(metadata.PrimaryKey.SortKey.DBName, "attribute_not_exists", nil); err != nil {
			return fmt.Errorf("failed to add default sort key condition: %w", err)
		}
	}

	return nil
}

func (q *query) mergeRawConditionExpressions(conditionExpr string, values map[string]types.AttributeValue) (string, map[string]types.AttributeValue, error) {
	mergedExpr := conditionExpr
	mergedValues := values

	for _, raw := range q.rawConditions {
		if raw.expression == "" {
			continue
		}

		mergedExpr = mergeAndExpression(mergedExpr, raw.expression)
		if len(raw.values) == 0 {
			continue
		}

		if mergedValues == nil {
			mergedValues = make(map[string]types.AttributeValue)
		}

		if err := q.mergeRawConditionValues(mergedValues, raw.values); err != nil {
			return "", nil, err
		}
	}

	return mergedExpr, mergedValues, nil
}

func (q *query) mergeRawConditionValues(dst map[string]types.AttributeValue, values map[string]any) error {
	for key, val := range values {
		if _, exists := dst[key]; exists {
			return fmt.Errorf("duplicate placeholder %s in condition expression", key)
		}
		av, err := q.db.converter.ToAttributeValue(val)
		if err != nil {
			return fmt.Errorf("failed to convert condition value for %s: %w", key, err)
		}
		dst[key] = av
	}
	return nil
}

func mergeAndExpression(current, next string) string {
	if current == "" {
		return next
	}
	return fmt.Sprintf("(%s) AND (%s)", current, next)
}

func (q *query) extractPrimaryKey(metadata *model.Metadata) map[string]any {
	pk := make(map[string]any)

	// First try to extract from conditions
	for _, cond := range q.conditions {
		if normalizeOperator(cond.op) != "=" {
			continue
		}

		// Check field name using enhanced lookup
		if field, exists := lookupField(metadata, cond.field); exists {
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
			pkField := modelValue.FieldByIndex(metadata.PrimaryKey.PartitionKey.IndexPath)
			if !pkField.IsZero() {
				pk["pk"] = pkField.Interface()
			}
		}

		// Extract sort key from model if exists
		if metadata.PrimaryKey.SortKey != nil {
			skField := modelValue.FieldByIndex(metadata.PrimaryKey.SortKey.IndexPath)
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
		builder := expr.NewBuilderWithConverter(q.db.converter)
		builder.AddProjection(q.fields...)
		components := builder.Build()

		if components.ProjectionExpression != "" {
			input.ProjectionExpression = aws.String(components.ProjectionExpression)
			input.ExpressionAttributeNames = components.ExpressionAttributeNames
		}
	}

	// Set consistent read
	if q.consistentRead {
		input.ConsistentRead = aws.Bool(true)
	}

	// Execute GetItem
	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for get item: %w", err)
	}

	output, err := client.GetItem(q.ctx, input)
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

// getItemDirect performs a direct GetItem without expression builder overhead
func (q *query) getItemDirect(metadata *model.Metadata, pk map[string]any, dest any) error {
	// Pre-allocate with exact size
	keyMap := make(map[string]types.AttributeValue, 2)

	// Direct conversion without error handling in hot path
	if pkValue, hasPK := pk["pk"]; hasPK {
		if av, err := q.db.converter.ToAttributeValue(pkValue); err == nil {
			keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
		} else {
			return fmt.Errorf("failed to convert partition key: %w", err)
		}
	}

	if skValue, hasSK := pk["sk"]; hasSK && metadata.PrimaryKey.SortKey != nil {
		if av, err := q.db.converter.ToAttributeValue(skValue); err == nil {
			keyMap[metadata.PrimaryKey.SortKey.DBName] = av
		} else {
			return fmt.Errorf("failed to convert sort key: %w", err)
		}
	}

	// Direct API call
	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for direct get item: %w", err)
	}

	input := &dynamodb.GetItemInput{
		TableName: aws.String(metadata.TableName),
		Key:       keyMap,
	}

	// Set consistent read
	if q.consistentRead {
		input.ConsistentRead = aws.Bool(true)
	}

	output, err := client.GetItem(q.ctx, input)

	if err != nil {
		return fmt.Errorf("failed to get item: %w", err)
	}

	if output.Item == nil {
		return customerrors.ErrItemNotFound
	}

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
		structField := destValue.FieldByIndex(field.IndexPath)
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

	conditionExpr, names, values, err := q.buildConditionExpression(metadata, false, false, false)
	if err != nil {
		return err
	}
	if conditionExpr != "" {
		input.ConditionExpression = aws.String(conditionExpr)
	}
	if len(names) > 0 {
		input.ExpressionAttributeNames = names
	}
	if len(values) > 0 {
		input.ExpressionAttributeValues = values
	}

	// Execute PutItem
	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for put item: %w", err)
	}

	_, err = client.PutItem(q.ctx, input)
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
			field := modelValue.FieldByIndex(fieldMeta.IndexPath)
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

	modelValue := reflect.ValueOf(model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}

	return q.marshalItemReflect(modelValue, metadata)
}

func (q *query) marshalItemReflect(modelValue reflect.Value, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
	item := make(map[string]types.AttributeValue)
	now := time.Now()

	for fieldName, fieldMeta := range metadata.Fields {
		av, ok, err := q.marshalFieldValue(modelValue, fieldName, fieldMeta, now)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		item[fieldMeta.DBName] = av
	}

	return item, nil
}

func (q *query) marshalFieldValue(modelValue reflect.Value, fieldName string, fieldMeta *model.FieldMetadata, now time.Time) (types.AttributeValue, bool, error) {
	fieldValue := modelValue.FieldByIndex(fieldMeta.IndexPath)

	if fieldMeta.OmitEmpty && fieldValue.IsZero() {
		return nil, false, nil
	}

	valueToConvert, err := q.valueForField(fieldName, fieldMeta, fieldValue, now)
	if err != nil {
		return nil, false, err
	}

	av, err := q.convertFieldValue(fieldMeta, valueToConvert)
	if err != nil {
		return nil, false, fmt.Errorf("failed to convert field %s: %w", fieldName, err)
	}

	if _, isNull := av.(*types.AttributeValueMemberNULL); isNull && fieldMeta.OmitEmpty {
		return nil, false, nil
	}

	return av, true, nil
}

func (q *query) valueForField(fieldName string, fieldMeta *model.FieldMetadata, fieldValue reflect.Value, now time.Time) (any, error) {
	switch {
	case fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt:
		return now, nil
	case fieldMeta.IsVersion:
		if fieldValue.IsZero() {
			return int64(0), nil
		}
	case fieldMeta.IsTTL:
		return q.ttlValue(fieldName, fieldValue)
	}

	return fieldValue.Interface(), nil
}

func (q *query) ttlValue(fieldName string, fieldValue reflect.Value) (any, error) {
	if fieldValue.Type() != reflect.TypeOf(time.Time{}) || fieldValue.IsZero() {
		return fieldValue.Interface(), nil
	}

	ttlTime, ok := fieldValue.Interface().(time.Time)
	if !ok {
		return nil, fmt.Errorf("expected time.Time for TTL field %s, got %T", fieldName, fieldValue.Interface())
	}

	return ttlTime.Unix(), nil
}

func (q *query) convertFieldValue(fieldMeta *model.FieldMetadata, value any) (types.AttributeValue, error) {
	if fieldMeta.IsSet {
		return q.db.converter.ConvertToSet(value, true)
	}
	return q.db.converter.ToAttributeValue(value)
}

// isConditionalCheckFailedException checks if the error is a conditional check failure
func isConditionalCheckFailedException(err error) bool {
	var ccfe *types.ConditionalCheckFailedException
	return errors.As(err, &ccfe)
}

func (q *query) queryExpressionBuilder() *expr.Builder {
	if q.builder != nil {
		return q.builder.Clone()
	}
	return expr.NewBuilderWithConverter(q.db.converter)
}

func (q *query) scanExpressionBuilder() *expr.Builder {
	if q.builder != nil {
		return q.builder
	}
	return expr.NewBuilderWithConverter(q.db.converter)
}

func (q *query) addKeyConditionsToBuilder(builder *expr.Builder, metadata *model.Metadata, keyConditions []condition) error {
	for _, cond := range keyConditions {
		fieldMeta, exists := lookupField(metadata, cond.field)
		if !exists || fieldMeta == nil {
			return fmt.Errorf("failed to add key condition for %s: field not found", cond.field)
		}

		op := normalizeOperator(cond.op)
		if err := builder.AddKeyCondition(fieldMeta.DBName, op, cond.value); err != nil {
			return fmt.Errorf("failed to add key condition for %s: %w", cond.field, err)
		}
	}

	return nil
}

func (q *query) addFilterConditionsToBuilder(builder *expr.Builder, metadata *model.Metadata, filterConditions []condition) error {
	for _, cond := range filterConditions {
		op := normalizeOperator(cond.op)
		fieldName := cond.field

		if fieldMeta, exists := lookupField(metadata, cond.field); exists && fieldMeta != nil {
			fieldName = fieldMeta.DBName
		}

		if err := builder.AddFilterCondition("AND", fieldName, op, cond.value); err != nil {
			return fmt.Errorf("failed to add filter condition for %s: %w", cond.field, err)
		}
	}

	return nil
}

func (q *query) addFilterConditionsToBuilderWithRecording(builder *expr.Builder, metadata *model.Metadata, filterConditions []condition) error {
	for _, cond := range filterConditions {
		op := normalizeOperator(cond.op)
		fieldName := cond.field

		if fieldMeta, exists := lookupField(metadata, cond.field); exists && fieldMeta != nil {
			fieldName = fieldMeta.DBName
		}

		if err := builder.AddFilterCondition("AND", fieldName, op, cond.value); err != nil {
			q.recordBuilderError(err)
			return fmt.Errorf("failed to add filter condition for %s: %w", cond.field, err)
		}
	}

	return nil
}

func (q *query) addProjectionToBuilder(builder *expr.Builder) {
	if len(q.fields) == 0 {
		return
	}
	builder.AddProjection(q.fields...)
}

func queryInputFromComponents(tableName string, components expr.ExpressionComponents) *dynamodb.QueryInput {
	input := &dynamodb.QueryInput{
		TableName: aws.String(tableName),
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

	return input
}

func scanInputFromComponents(tableName string, components expr.ExpressionComponents) *dynamodb.ScanInput {
	input := &dynamodb.ScanInput{
		TableName: aws.String(tableName),
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

	return input
}

func (q *query) applyQueryReadOptions(input *dynamodb.QueryInput) {
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}
	if q.orderBy != nil && q.orderBy.order == "DESC" {
		input.ScanIndexForward = aws.Bool(false)
	}
	if q.limit != nil {
		input.Limit = aws.Int32(numutil.ClampIntToInt32(*q.limit))
	}
	if q.consistentRead && q.indexName == "" {
		input.ConsistentRead = aws.Bool(true)
	}
}

func (q *query) applyQueryCountOptions(input *dynamodb.QueryInput) {
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}
}

func (q *query) applyScanReadOptions(input *dynamodb.ScanInput) {
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}
	if q.limit != nil {
		input.Limit = aws.Int32(numutil.ClampIntToInt32(*q.limit))
	}
	if q.consistentRead && q.indexName == "" {
		input.ConsistentRead = aws.Bool(true)
	}
}

func (q *query) paginationLimit() (int, bool) {
	if q.limit == nil {
		return 0, false
	}
	if *q.limit <= 0 {
		return 0, true
	}
	return *q.limit, true
}

func (q *query) collectPaginatedItems(
	hasMorePages func() bool,
	nextPage func(context.Context) ([]map[string]types.AttributeValue, error),
	limit int,
	hasLimit bool,
	trim bool,
) ([]map[string]types.AttributeValue, error) {
	var items []map[string]types.AttributeValue

	for hasMorePages() {
		pageItems, err := nextPage(q.ctx)
		if err != nil {
			return nil, err
		}

		items = append(items, pageItems...)
		if hasLimit && len(items) >= limit {
			if trim {
				return items[:limit], nil
			}
			break
		}
	}

	return items, nil
}

func (q *query) collectQueryCount(client *dynamodb.Client, input *dynamodb.QueryInput) (int64, error) {
	var totalCount int64
	paginator := dynamodb.NewQueryPaginator(client, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(q.ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to count items: %w", err)
		}
		totalCount += int64(output.Count)
	}

	return totalCount, nil
}

// executeQuery performs a DynamoDB Query operation
func (q *query) executeQuery(metadata *model.Metadata, keyConditions []condition, filterConditions []condition) ([]map[string]types.AttributeValue, error) {
	builder := q.queryExpressionBuilder()
	if err := q.addKeyConditionsToBuilder(builder, metadata, keyConditions); err != nil {
		return nil, err
	}
	if err := q.addFilterConditionsToBuilder(builder, metadata, filterConditions); err != nil {
		return nil, err
	}
	q.addProjectionToBuilder(builder)

	components := builder.Build()

	input := queryInputFromComponents(metadata.TableName, components)
	q.applyQueryReadOptions(input)

	client, err := q.db.session.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for query: %w", err)
	}

	paginator := dynamodb.NewQueryPaginator(client, input)
	limit, hasLimit := q.paginationLimit()
	return q.collectPaginatedItems(
		paginator.HasMorePages,
		func(ctx context.Context) ([]map[string]types.AttributeValue, error) {
			output, err := paginator.NextPage(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to query items: %w", err)
			}
			return output.Items, nil
		},
		limit,
		hasLimit,
		true,
	)
}

// executeScan performs a DynamoDB Scan operation
func (q *query) executeScan(metadata *model.Metadata, filterConditions []condition) ([]map[string]types.AttributeValue, error) {
	// Use the existing builder from the query to preserve Filter() conditions
	builder := q.scanExpressionBuilder()

	if err := q.addFilterConditionsToBuilderWithRecording(builder, metadata, filterConditions); err != nil {
		return nil, err
	}

	q.addProjectionToBuilder(builder)
	components := builder.Build()

	input := scanInputFromComponents(metadata.TableName, components)
	q.applyScanReadOptions(input)

	client, err := q.db.session.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for scan: %w", err)
	}

	paginator := dynamodb.NewScanPaginator(client, input)
	limit, hasLimit := q.paginationLimit()
	return q.collectPaginatedItems(
		paginator.HasMorePages,
		func(ctx context.Context) ([]map[string]types.AttributeValue, error) {
			output, err := paginator.NextPage(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to scan items: %w", err)
			}
			return output.Items, nil
		},
		limit,
		hasLimit,
		true,
	)
}

// isIndexKey checks if a field is a key in the specified index
func (q *query) isIndexKey(fieldMeta *model.FieldMetadata, indexName string, metadata *model.Metadata) bool {
	return q.isIndexPartitionKey(fieldMeta, indexName, metadata) ||
		q.isIndexSortKey(fieldMeta, indexName, metadata)
}

func (q *query) isIndexPartitionKey(fieldMeta *model.FieldMetadata, indexName string, metadata *model.Metadata) bool {
	for _, index := range metadata.Indexes {
		if index.Name == indexName && index.PartitionKey != nil && index.PartitionKey.Name == fieldMeta.Name {
			return true
		}
	}
	return false
}

func (q *query) isIndexSortKey(fieldMeta *model.FieldMetadata, indexName string, metadata *model.Metadata) bool {
	for _, index := range metadata.Indexes {
		if index.Name == indexName && index.SortKey != nil && index.SortKey.Name == fieldMeta.Name {
			return true
		}
	}
	return false
}

func (q *query) determineKeyRoles(fieldMeta *model.FieldMetadata, metadata *model.Metadata) (bool, bool) {
	isPK := fieldMeta.IsPK
	isSK := fieldMeta.IsSK

	if q.indexName != "" && q.isIndexKey(fieldMeta, q.indexName, metadata) {
		if !isPK && q.isIndexPartitionKey(fieldMeta, q.indexName, metadata) {
			isPK = true
		}
		if !isSK && q.isIndexSortKey(fieldMeta, q.indexName, metadata) {
			isSK = true
		}
	}

	return isPK, isSK
}

// unmarshalItems converts DynamoDB items to Go slice
func (q *query) unmarshalItems(items []map[string]types.AttributeValue, dest any, metadata *model.Metadata) error {
	destValue := reflect.ValueOf(dest).Elem()
	elemType := destValue.Type().Elem()

	// Create a new slice with the correct length
	newSlice := reflect.MakeSlice(destValue.Type(), len(items), len(items))

	// Unmarshal each item
	for i, item := range items {
		var elem reflect.Value

		// Check if element type is already a pointer
		if elemType.Kind() == reflect.Ptr {
			// For []*Type, create a new instance of Type (not *Type)
			elem = reflect.New(elemType.Elem())
		} else {
			// For []Type, create a new instance of *Type
			elem = reflect.New(elemType)
		}

		if err := q.unmarshalItem(item, elem.Interface(), metadata); err != nil {
			return fmt.Errorf("failed to unmarshal item %d: %w", i, err)
		}

		// Set the element in the slice
		if elemType.Kind() == reflect.Ptr {
			// For []*Type, elem is already a pointer, just set it
			newSlice.Index(i).Set(elem)
		} else {
			// For []Type, dereference the pointer
			newSlice.Index(i).Set(elem.Elem())
		}
	}

	// Set the destination slice
	destValue.Set(newSlice)
	return nil
}

// executeQueryCount performs a DynamoDB Query operation to count items
func (q *query) executeQueryCount(metadata *model.Metadata, keyConditions []condition, filterConditions []condition) (int64, error) {
	builder := q.queryExpressionBuilder()
	if err := q.addKeyConditionsToBuilder(builder, metadata, keyConditions); err != nil {
		return 0, err
	}
	if err := q.addFilterConditionsToBuilder(builder, metadata, filterConditions); err != nil {
		return 0, err
	}

	components := builder.Build()
	input := queryInputFromComponents(metadata.TableName, components)
	input.Select = types.SelectCount
	q.applyQueryCountOptions(input)

	client, err := q.db.session.Client()
	if err != nil {
		return 0, fmt.Errorf("failed to get client for query count: %w", err)
	}

	return q.collectQueryCount(client, input)
}

// executeScanCount performs a DynamoDB Scan operation to count items
func (q *query) executeScanCount(metadata *model.Metadata, filterConditions []condition) (int64, error) {
	// Use the existing builder from the query to preserve Filter() conditions
	builder := q.builder
	if builder == nil {
		builder = expr.NewBuilderWithConverter(q.db.converter)
	}

	// Add filter conditions from parameters (these come from Where() conditions)
	for _, cond := range filterConditions {
		fieldMeta, exists := lookupField(metadata, cond.field)
		op := normalizeOperator(cond.op)
		if exists {
			if err := builder.AddFilterCondition("AND", fieldMeta.DBName, op, cond.value); err != nil {
				q.recordBuilderError(err)
				return 0, fmt.Errorf("failed to add filter condition for %s: %w", cond.field, err)
			}
		} else {
			if err := builder.AddFilterCondition("AND", cond.field, op, cond.value); err != nil {
				q.recordBuilderError(err)
				return 0, fmt.Errorf("failed to add filter condition for %s: %w", cond.field, err)
			}
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

	client, err := q.db.session.Client()
	if err != nil {
		return 0, fmt.Errorf("failed to get client for scan count: %w", err)
	}

	paginator := dynamodb.NewScanPaginator(client, input)

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

	keyMap, err := q.buildKeyMapFromPrimaryKey(metadata, pk)
	if err != nil {
		return err
	}

	// Build update expression with custom converter support
	builder := expr.NewBuilderWithConverter(q.db.converter)

	modelValue := derefValue(reflect.ValueOf(q.model))

	fieldsToUpdate := q.fieldsToUpdate(metadata, modelValue, fields)
	if err = q.addUpdateExpressions(builder, metadata, modelValue, fieldsToUpdate); err != nil {
		return err
	}

	q.addUpdatedAtUpdate(builder, metadata)

	if err = q.addUpdateVersionCondition(builder, metadata, modelValue); err != nil {
		return err
	}

	// Build the update expression
	components := builder.Build()

	conditionExpr, exprAttrNames, exprAttrValues, err := q.mergeQueryConditions(metadata, components.ConditionExpression, components.ExpressionAttributeNames, components.ExpressionAttributeValues)
	if err != nil {
		return err
	}

	// Build UpdateItem input
	input := &dynamodb.UpdateItemInput{
		TableName:                 aws.String(metadata.TableName),
		Key:                       keyMap,
		UpdateExpression:          aws.String(components.UpdateExpression),
		ExpressionAttributeNames:  exprAttrNames,
		ExpressionAttributeValues: exprAttrValues,
	}

	if conditionExpr != "" {
		input.ConditionExpression = aws.String(conditionExpr)
	}

	// Execute UpdateItem
	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for update item: %w", err)
	}

	_, err = client.UpdateItem(q.ctx, input)
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

	keyMap, err := q.buildKeyMapFromPrimaryKey(metadata, pk)
	if err != nil {
		return err
	}

	// Build DeleteItem input
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(metadata.TableName),
		Key:       keyMap,
	}

	builder := expr.NewBuilderWithConverter(q.db.converter)

	if err = q.addDeleteVersionCondition(builder, metadata); err != nil {
		return err
	}

	components := builder.Build()
	conditionExpr, exprAttrNames, exprAttrValues, err := q.mergeQueryConditions(metadata, components.ConditionExpression, components.ExpressionAttributeNames, components.ExpressionAttributeValues)
	if err != nil {
		return err
	}

	if conditionExpr != "" {
		input.ConditionExpression = aws.String(conditionExpr)
		input.ExpressionAttributeNames = exprAttrNames
		input.ExpressionAttributeValues = exprAttrValues
	}

	// Execute DeleteItem
	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for delete item: %w", err)
	}

	_, err = client.DeleteItem(q.ctx, input)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to delete item: %w", err)
	}

	return nil
}

func (q *query) buildKeyMapFromPrimaryKey(metadata *model.Metadata, pk map[string]any) (map[string]types.AttributeValue, error) {
	keyMap := make(map[string]types.AttributeValue)

	if pkValue, hasPK := pk["pk"]; hasPK {
		av, err := q.db.converter.ToAttributeValue(pkValue)
		if err != nil {
			return nil, fmt.Errorf("failed to convert partition key: %w", err)
		}
		keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
	}

	if skValue, hasSK := pk["sk"]; hasSK && metadata.PrimaryKey.SortKey != nil {
		av, err := q.db.converter.ToAttributeValue(skValue)
		if err != nil {
			return nil, fmt.Errorf("failed to convert sort key: %w", err)
		}
		keyMap[metadata.PrimaryKey.SortKey.DBName] = av
	}

	return keyMap, nil
}

func derefValue(value reflect.Value) reflect.Value {
	if value.Kind() == reflect.Ptr {
		return value.Elem()
	}
	return value
}

func (q *query) fieldsToUpdate(metadata *model.Metadata, modelValue reflect.Value, fields []string) []string {
	if len(fields) > 0 {
		return fields
	}

	fieldsToUpdate := make([]string, 0, len(metadata.Fields))
	for fieldName, fieldMeta := range metadata.Fields {
		if fieldMeta.IsPK || fieldMeta.IsSK || fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt {
			continue
		}
		fieldValue := modelValue.FieldByIndex(fieldMeta.IndexPath)
		if !reflectutil.IsEmpty(fieldValue) || !fieldMeta.OmitEmpty {
			fieldsToUpdate = append(fieldsToUpdate, fieldName)
		}
	}

	return fieldsToUpdate
}

func (q *query) addUpdateExpressions(builder *expr.Builder, metadata *model.Metadata, modelValue reflect.Value, fieldsToUpdate []string) error {
	for _, fieldName := range fieldsToUpdate {
		fieldMeta, exists := lookupField(metadata, fieldName)
		if !exists {
			return fmt.Errorf("field '%s' not found in model metadata (use Go field name or DB attribute name)", fieldName)
		}

		fieldValue := modelValue.FieldByIndex(fieldMeta.IndexPath)
		switch {
		case fieldMeta.IsUpdatedAt:
			builder.AddUpdateSet(fieldMeta.DBName, time.Now())
		case fieldMeta.IsVersion:
			builder.AddUpdateAdd(fieldMeta.DBName, int64(1))
		default:
			builder.AddUpdateSet(fieldMeta.DBName, fieldValue.Interface())
		}
	}

	return nil
}

func (q *query) addUpdatedAtUpdate(builder *expr.Builder, metadata *model.Metadata) {
	if metadata.UpdatedAtField == nil {
		return
	}
	builder.AddUpdateSet(metadata.UpdatedAtField.DBName, time.Now())
}

func (q *query) addUpdateVersionCondition(builder *expr.Builder, metadata *model.Metadata, modelValue reflect.Value) error {
	if metadata.VersionField == nil {
		return nil
	}
	currentVersion := modelValue.FieldByIndex(metadata.VersionField.IndexPath).Int()
	if err := builder.AddConditionExpression(metadata.VersionField.DBName, "=", currentVersion); err != nil {
		return fmt.Errorf("failed to add version condition: %w", err)
	}
	return nil
}

func (q *query) addDeleteVersionCondition(builder *expr.Builder, metadata *model.Metadata) error {
	if metadata.VersionField == nil || q.model == nil {
		return nil
	}

	modelValue := derefValue(reflect.ValueOf(q.model))
	versionValue := modelValue.FieldByIndex(metadata.VersionField.IndexPath)
	if versionValue.IsZero() {
		return nil
	}

	if err := builder.AddConditionExpression(metadata.VersionField.DBName, "=", versionValue.Int()); err != nil {
		return fmt.Errorf("failed to add version condition: %w", err)
	}
	return nil
}

func (q *query) mergeQueryConditions(
	metadata *model.Metadata,
	conditionExpr string,
	exprAttrNames map[string]string,
	exprAttrValues map[string]types.AttributeValue,
) (string, map[string]string, map[string]types.AttributeValue, error) {
	queryCondExpr, queryCondNames, queryCondValues, err := q.buildConditionExpression(metadata, true, true, false)
	if err != nil {
		return "", nil, nil, err
	}

	if queryCondExpr != "" {
		conditionExpr = mergeAndExpression(conditionExpr, queryCondExpr)
	}

	if exprAttrNames == nil {
		exprAttrNames = make(map[string]string)
	}
	for k, v := range queryCondNames {
		exprAttrNames[k] = v
	}

	if exprAttrValues == nil {
		exprAttrValues = make(map[string]types.AttributeValue)
	}
	for k, v := range queryCondValues {
		if _, exists := exprAttrValues[k]; exists {
			return "", nil, nil, fmt.Errorf("duplicate condition value placeholder: %s", k)
		}
		exprAttrValues[k] = v
	}

	return conditionExpr, exprAttrNames, exprAttrValues, nil
}

// errorQuery is a query that always returns an error
type errorQuery struct {
	err error
}

func (e *errorQuery) Where(_ string, _ string, _ any) core.Query  { return e }
func (e *errorQuery) Index(_ string) core.Query                   { return e }
func (e *errorQuery) Filter(_ string, _ string, _ any) core.Query { return e }
func (e *errorQuery) OrFilter(_ string, _ string, _ any) core.Query {
	return e
}
func (e *errorQuery) FilterGroup(_ func(q core.Query)) core.Query { return e }
func (e *errorQuery) OrFilterGroup(_ func(core.Query)) core.Query {
	return e
}
func (e *errorQuery) IfNotExists() core.Query { return e }
func (e *errorQuery) IfExists() core.Query    { return e }
func (e *errorQuery) WithCondition(_ string, _ string, _ any) core.Query {
	return e
}
func (e *errorQuery) WithConditionExpression(_ string, _ map[string]any) core.Query {
	return e
}
func (e *errorQuery) OrderBy(_ string, _ string) core.Query       { return e }
func (e *errorQuery) Limit(_ int) core.Query                      { return e }
func (e *errorQuery) Offset(_ int) core.Query                     { return e }
func (e *errorQuery) Select(_ ...string) core.Query               { return e }
func (e *errorQuery) ConsistentRead() core.Query                  { return e }
func (e *errorQuery) WithRetry(_ int, _ time.Duration) core.Query { return e }
func (e *errorQuery) First(_ any) error                           { return e.err }
func (e *errorQuery) All(_ any) error                             { return e.err }
func (e *errorQuery) Count() (int64, error)                       { return 0, e.err }
func (e *errorQuery) Create() error                               { return e.err }
func (e *errorQuery) CreateOrUpdate() error                       { return e.err }
func (e *errorQuery) Update(_ ...string) error                    { return e.err }
func (e *errorQuery) Delete() error                               { return e.err }
func (e *errorQuery) Scan(_ any) error                            { return e.err }
func (e *errorQuery) BatchGet(_ []any, _ any) error               { return e.err }
func (e *errorQuery) BatchGetWithOptions(_ []any, _ any, _ *core.BatchGetOptions) error {
	return e.err
}
func (e *errorQuery) BatchGetBuilder() core.BatchGetBuilder { return &errorBatchGetBuilder{err: e.err} }
func (e *errorQuery) BatchCreate(_ any) error               { return e.err }
func (e *errorQuery) BatchDelete(_ []any) error             { return e.err }
func (e *errorQuery) BatchWrite(_ []any, _ []any) error     { return e.err }
func (e *errorQuery) BatchUpdateWithOptions(_ []any, _ []string, _ ...any) error {
	return e.err
}
func (e *errorQuery) WithContext(_ context.Context) core.Query          { return e }
func (e *errorQuery) AllPaginated(_ any) (*core.PaginatedResult, error) { return nil, e.err }
func (e *errorQuery) UpdateBuilder() core.UpdateBuilder                 { return nil }
func (e *errorQuery) ParallelScan(_ int32, _ int32) core.Query          { return e }
func (e *errorQuery) ScanAllSegments(_ any, _ int32) error              { return e.err }
func (e *errorQuery) Cursor(_ string) core.Query                        { return e }
func (e *errorQuery) SetCursor(_ string) error                          { return e.err }

// Re-export types for convenience
type (
	Config            = session.Config
	AutoMigrateOption = schema.AutoMigrateOption
	BatchGetOptions   = core.BatchGetOptions
	KeyPair           = core.KeyPair
)

// Re-export AutoMigrate options for convenience
var (
	WithBackupTable = schema.WithBackupTable
	WithDataCopy    = schema.WithDataCopy
	WithTargetModel = schema.WithTargetModel
	WithTransform   = schema.WithTransform
	WithBatchSize   = schema.WithBatchSize
)

// NewKeyPair constructs a composite key helper for BatchGet operations.
func NewKeyPair(partitionKey any, sortKey ...any) core.KeyPair {
	return core.NewKeyPair(partitionKey, sortKey...)
}

// DefaultBatchGetOptions returns the library defaults for BatchGet operations.
func DefaultBatchGetOptions() *core.BatchGetOptions {
	return core.DefaultBatchGetOptions()
}

// TransactionFunc executes a function within a database transaction
// This is the actual implementation that uses our sophisticated transaction support
func (db *DB) TransactionFunc(fn func(tx any) error) error {
	// Create a new transaction
	tx := transaction.NewTransaction(db.session, db.registry, db.converter)
	tx = tx.WithContext(db.ctx)

	// Execute the transaction function
	if err := fn(tx); err != nil {
		// Rollback on error
		if rbErr := tx.Rollback(); rbErr != nil {
			return errors.Join(err, fmt.Errorf("rollback failed: %w", rbErr))
		}
		return err
	}

	// Commit the transaction
	return tx.Commit()
}

// AllPaginated retrieves all matching items with pagination metadata
func (q *query) AllPaginated(dest any) (*core.PaginatedResult, error) {
	if err := q.checkBuilderError(); err != nil {
		return nil, err
	}
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return nil, err
	}

	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return nil, err
	}

	keyConditions, filterConditions := q.splitPaginatedConditions(metadata)

	items, scannedCount, lastEvaluatedKey, err := q.fetchPaginatedItems(metadata, keyConditions, filterConditions)
	if err != nil {
		return nil, err
	}

	if err := q.unmarshalPaginatedItems(items, dest, metadata); err != nil {
		return nil, err
	}

	return q.buildPaginatedResult(dest, items, scannedCount, lastEvaluatedKey)
}

func (q *query) splitPaginatedConditions(metadata *model.Metadata) ([]condition, []condition) {
	keyConditions := make([]condition, 0, len(q.conditions))
	filterConditions := make([]condition, 0, len(q.conditions))

	for _, cond := range q.conditions {
		normalized := normalizeCondition(cond)
		fieldMeta, exists := lookupField(metadata, normalized.field)
		if !exists || fieldMeta == nil {
			filterConditions = append(filterConditions, normalized)
			continue
		}

		if q.isKeyConditionForPagination(fieldMeta, metadata, normalized.op) {
			keyConditions = append(keyConditions, normalized)
			continue
		}

		filterConditions = append(filterConditions, normalized)
	}

	return keyConditions, filterConditions
}

func normalizeCondition(cond condition) condition {
	return condition{
		field: cond.field,
		op:    normalizeOperator(cond.op),
		value: cond.value,
	}
}

func (q *query) isKeyConditionForPagination(fieldMeta *model.FieldMetadata, metadata *model.Metadata, op string) bool {
	isPK, isSK := q.determineKeyRoles(fieldMeta, metadata)
	if !isPK && !isSK {
		return false
	}
	if !isKeyConditionOperator(op) {
		return false
	}
	if isPK && op != "=" {
		return false
	}
	return true
}

func isKeyConditionOperator(op string) bool {
	switch op {
	case "=", operatorBeginsWith, "<", "<=", ">", ">=", operatorBetween:
		return true
	default:
		return false
	}
}

func (q *query) fetchPaginatedItems(
	metadata *model.Metadata,
	keyConditions []condition,
	filterConditions []condition,
) ([]map[string]types.AttributeValue, int, map[string]types.AttributeValue, error) {
	if len(keyConditions) > 0 {
		return q.fetchPaginatedQueryItems(metadata, keyConditions, filterConditions)
	}
	return q.fetchPaginatedScanItems(metadata, filterConditions)
}

func (q *query) fetchPaginatedQueryItems(
	metadata *model.Metadata,
	keyConditions []condition,
	filterConditions []condition,
) ([]map[string]types.AttributeValue, int, map[string]types.AttributeValue, error) {
	builder := expr.NewBuilderWithConverter(q.db.converter)

	if err := q.addKeyConditionsToBuilder(builder, metadata, keyConditions); err != nil {
		return nil, 0, nil, err
	}
	if err := q.addFilterConditionsToBuilder(builder, metadata, filterConditions); err != nil {
		return nil, 0, nil, err
	}
	q.addProjectionToBuilder(builder)

	components := builder.Build()
	input := queryInputFromComponents(metadata.TableName, components)
	q.applyPaginatedQueryOptions(input)

	client, err := q.db.session.Client()
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to get client for paginated query: %w", err)
	}

	output, err := client.Query(q.ctx, input)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to query items: %w", err)
	}

	return output.Items, int(output.ScannedCount), output.LastEvaluatedKey, nil
}

func (q *query) fetchPaginatedScanItems(
	metadata *model.Metadata,
	filterConditions []condition,
) ([]map[string]types.AttributeValue, int, map[string]types.AttributeValue, error) {
	builder := expr.NewBuilderWithConverter(q.db.converter)

	if err := q.addFilterConditionsToBuilder(builder, metadata, filterConditions); err != nil {
		return nil, 0, nil, err
	}
	q.addProjectionToBuilder(builder)

	components := builder.Build()
	input := scanInputFromComponents(metadata.TableName, components)
	q.applyPaginatedScanOptions(input)

	client, err := q.db.session.Client()
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to get client for paginated scan: %w", err)
	}

	output, err := client.Scan(q.ctx, input)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to scan items: %w", err)
	}

	return output.Items, int(output.ScannedCount), output.LastEvaluatedKey, nil
}

func (q *query) applyPaginatedQueryOptions(input *dynamodb.QueryInput) {
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}
	if q.orderBy != nil && q.orderBy.order == "DESC" {
		input.ScanIndexForward = aws.Bool(false)
	}
	if q.limit != nil {
		input.Limit = aws.Int32(numutil.ClampIntToInt32(*q.limit))
	}
}

func (q *query) applyPaginatedScanOptions(input *dynamodb.ScanInput) {
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}
	if q.limit != nil {
		input.Limit = aws.Int32(numutil.ClampIntToInt32(*q.limit))
	}
}

func (q *query) unmarshalPaginatedItems(items []map[string]types.AttributeValue, dest any, metadata *model.Metadata) error {
	if len(items) == 0 {
		return nil
	}
	return q.unmarshalItems(items, dest, metadata)
}

func (q *query) buildPaginatedResult(
	dest any,
	items []map[string]types.AttributeValue,
	scannedCount int,
	lastEvaluatedKey map[string]types.AttributeValue,
) (*core.PaginatedResult, error) {
	result := &core.PaginatedResult{
		Items:            dest,
		Count:            len(items),
		ScannedCount:     scannedCount,
		LastEvaluatedKey: lastEvaluatedKey,
		HasMore:          len(lastEvaluatedKey) > 0,
	}

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
	// Get metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		// Return an error-producing UpdateBuilder
		return &errorUpdateBuilder{err: err}
	}

	// Create Query struct for pkg/query using the proper constructor
	adapter := &metadataAdapter{metadata: metadata}
	executor := &queryExecutor{db: q.db}
	conditions := convertConditions(q.conditions)

	pkgQuery := queryPkg.NewWithConditions(q.model, adapter, executor, conditions, q.ctx)

	return queryPkg.NewUpdateBuilder(pkgQuery)
}

// errorUpdateBuilder is a simple error-returning UpdateBuilder
type errorUpdateBuilder struct {
	err error
}

func (e *errorUpdateBuilder) Set(_ string, _ any) core.UpdateBuilder { return e }
func (e *errorUpdateBuilder) SetIfNotExists(_ string, _ any, _ any) core.UpdateBuilder {
	return e
}
func (e *errorUpdateBuilder) Add(_ string, _ any) core.UpdateBuilder           { return e }
func (e *errorUpdateBuilder) Increment(_ string) core.UpdateBuilder            { return e }
func (e *errorUpdateBuilder) Decrement(_ string) core.UpdateBuilder            { return e }
func (e *errorUpdateBuilder) Remove(_ string) core.UpdateBuilder               { return e }
func (e *errorUpdateBuilder) Delete(_ string, _ any) core.UpdateBuilder        { return e }
func (e *errorUpdateBuilder) AppendToList(_ string, _ any) core.UpdateBuilder  { return e }
func (e *errorUpdateBuilder) PrependToList(_ string, _ any) core.UpdateBuilder { return e }
func (e *errorUpdateBuilder) RemoveFromListAt(_ string, _ int) core.UpdateBuilder {
	return e
}
func (e *errorUpdateBuilder) SetListElement(_ string, _ int, _ any) core.UpdateBuilder {
	return e
}
func (e *errorUpdateBuilder) Condition(_ string, _ string, _ any) core.UpdateBuilder {
	return e
}
func (e *errorUpdateBuilder) OrCondition(_ string, _ string, _ any) core.UpdateBuilder {
	return e
}
func (e *errorUpdateBuilder) ConditionExists(_ string) core.UpdateBuilder    { return e }
func (e *errorUpdateBuilder) ConditionNotExists(_ string) core.UpdateBuilder { return e }
func (e *errorUpdateBuilder) ConditionVersion(_ int64) core.UpdateBuilder    { return e }
func (e *errorUpdateBuilder) ReturnValues(_ string) core.UpdateBuilder       { return e }
func (e *errorUpdateBuilder) Execute() error                                 { return e.err }
func (e *errorUpdateBuilder) ExecuteWithResult(_ any) error                  { return e.err }

// errorBatchGetBuilder is returned when BatchGet builder construction fails.
type errorBatchGetBuilder struct {
	err error
}

func (b *errorBatchGetBuilder) Keys(_ []any) core.BatchGetBuilder                  { return b }
func (b *errorBatchGetBuilder) ChunkSize(_ int) core.BatchGetBuilder               { return b }
func (b *errorBatchGetBuilder) ConsistentRead() core.BatchGetBuilder               { return b }
func (b *errorBatchGetBuilder) Parallel(_ int) core.BatchGetBuilder                { return b }
func (b *errorBatchGetBuilder) WithRetry(_ *core.RetryPolicy) core.BatchGetBuilder { return b }
func (b *errorBatchGetBuilder) Select(_ ...string) core.BatchGetBuilder            { return b }
func (b *errorBatchGetBuilder) OnProgress(_ core.BatchProgressCallback) core.BatchGetBuilder {
	return b
}
func (b *errorBatchGetBuilder) OnError(_ core.BatchChunkErrorHandler) core.BatchGetBuilder {
	return b
}
func (b *errorBatchGetBuilder) Execute(_ any) error { return b.err }

type batchGetBuilderWrapper struct {
	builder  core.BatchGetBuilder
	query    *query
	metadata *model.Metadata
}

func (b *batchGetBuilderWrapper) Keys(keys []any) core.BatchGetBuilder {
	b.builder = b.builder.Keys(keys)
	return b
}

func (b *batchGetBuilderWrapper) ChunkSize(size int) core.BatchGetBuilder {
	b.builder = b.builder.ChunkSize(size)
	return b
}

func (b *batchGetBuilderWrapper) ConsistentRead() core.BatchGetBuilder {
	b.builder = b.builder.ConsistentRead()
	return b
}

func (b *batchGetBuilderWrapper) Parallel(maxConcurrency int) core.BatchGetBuilder {
	b.builder = b.builder.Parallel(maxConcurrency)
	return b
}

func (b *batchGetBuilderWrapper) WithRetry(policy *core.RetryPolicy) core.BatchGetBuilder {
	b.builder = b.builder.WithRetry(policy)
	return b
}

func (b *batchGetBuilderWrapper) Select(fields ...string) core.BatchGetBuilder {
	b.builder = b.builder.Select(fields...)
	return b
}

func (b *batchGetBuilderWrapper) OnProgress(callback core.BatchProgressCallback) core.BatchGetBuilder {
	b.builder = b.builder.OnProgress(callback)
	return b
}

func (b *batchGetBuilderWrapper) OnError(handler core.BatchChunkErrorHandler) core.BatchGetBuilder {
	b.builder = b.builder.OnError(handler)
	return b
}

func (b *batchGetBuilderWrapper) Execute(dest any) error {
	var rawItems []map[string]types.AttributeValue
	if err := b.builder.Execute(&rawItems); err != nil {
		return err
	}
	return b.query.unmarshalItems(rawItems, dest, b.metadata)
}

// ParallelScan configures parallel scanning with segment and total segments
func (q *query) ParallelScan(segment int32, totalSegments int32) core.Query {
	q.segment = &segment
	q.totalSegments = &totalSegments
	return q
}

// ScanAllSegments performs parallel scan across all segments automatically
func (q *query) ScanAllSegments(dest any, totalSegments int32) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
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
		err   error
		items []map[string]types.AttributeValue
	}
	resultsChan := make(chan segmentResult, totalSegments)

	// Launch parallel scans
	var wg sync.WaitGroup
	for i := int32(0); i < totalSegments; i++ {
		wg.Add(1)
		go func(segment int32) {
			defer func() {
				if r := recover(); r != nil {
					// Log the panic with context
					err := fmt.Errorf("scan segment %d panicked: %v", segment, r)
					resultsChan <- segmentResult{err: err}
				}
				wg.Done()
			}()

			// Clone the query for this segment
			segmentQuery := &query{
				ctx:           q.ctx,
				db:            q.db,
				model:         q.model,
				builder:       q.builder,
				builderErr:    q.builderErr,
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
				resultsChan <- segmentResult{err: fmt.Errorf("segment %d failed: %w", segment, err)}
				return
			}

			resultsChan <- segmentResult{items: items}
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
	builder := expr.NewBuilderWithConverter(q.db.converter)
	filterConditions := q.scanSegmentFilterConditions(metadata)
	if err := q.addFilterConditionsToBuilderWithRecording(builder, metadata, filterConditions); err != nil {
		return nil, err
	}

	q.addProjectionToBuilder(builder)
	components := builder.Build()

	input := scanInputFromComponents(metadata.TableName, components)
	input.Segment = aws.Int32(segment)
	input.TotalSegments = aws.Int32(totalSegments)
	q.applyScanSegmentOptions(input, totalSegments)

	client, err := q.db.session.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for scan segment: %w", err)
	}
	return q.collectScanSegmentItems(client, input, totalSegments)
}

func (q *query) scanSegmentFilterConditions(metadata *model.Metadata) []condition {
	filterConditions := make([]condition, 0, len(q.conditions))
	for _, cond := range q.conditions {
		normalizedCond := condition{
			field: cond.field,
			op:    normalizeOperator(cond.op),
			value: cond.value,
		}

		fieldMeta, exists := lookupField(metadata, normalizedCond.field)
		if !exists || (!fieldMeta.IsPK && !fieldMeta.IsSK) {
			filterConditions = append(filterConditions, normalizedCond)
		}
	}

	return filterConditions
}

func (q *query) applyScanSegmentOptions(input *dynamodb.ScanInput, totalSegments int32) {
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}

	if q.limit == nil || *q.limit <= 0 {
		return
	}

	segmentLimit := (*q.limit + int(totalSegments) - 1) / int(totalSegments)
	input.Limit = aws.Int32(numutil.ClampIntToInt32(segmentLimit))
}

func (q *query) collectScanSegmentItems(client *dynamodb.Client, input *dynamodb.ScanInput, totalSegments int32) ([]map[string]types.AttributeValue, error) {
	paginator := dynamodb.NewScanPaginator(client, input)
	limit := 0
	hasLimit := q.limit != nil
	if q.limit != nil {
		limit = *q.limit / int(totalSegments)
	}

	return q.collectPaginatedItems(
		paginator.HasMorePages,
		func(ctx context.Context) ([]map[string]types.AttributeValue, error) {
			output, err := paginator.NextPage(ctx)
			if err != nil {
				return nil, err
			}
			return output.Items, nil
		},
		limit,
		hasLimit,
		false,
	)
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
	if err := q.checkBuilderError(); err != nil {
		return err
	}
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
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	_ = options

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
			pkField := itemValue.FieldByIndex(metadata.PrimaryKey.PartitionKey.IndexPath)
			updateQuery = updateQuery.Where(metadata.PrimaryKey.PartitionKey.Name, "=", pkField.Interface())
		}

		// Add sort key condition if present
		if metadata.PrimaryKey.SortKey != nil {
			skField := itemValue.FieldByIndex(metadata.PrimaryKey.SortKey.IndexPath)
			updateQuery = updateQuery.Where(metadata.PrimaryKey.SortKey.Name, "=", skField.Interface())
		}

		// Perform the update
		if err := updateQuery.Update(fields...); err != nil {
			return fmt.Errorf("failed to update item %d: %w", i, err)
		}
	}

	return nil
}

// convertConditions converts dynamorm conditions to pkg/query conditions
func convertConditions(conditions []condition) []queryPkg.Condition {
	result := make([]queryPkg.Condition, len(conditions))
	for i, cond := range conditions {
		result[i] = queryPkg.Condition{
			Field:    cond.field,
			Operator: normalizeOperator(cond.op),
			Value:    cond.value,
		}
	}
	return result
}

// queryExecutor implements the executor interface for pkg/query
type queryExecutor struct {
	db *DB
}

// ExecuteQuery implements QueryExecutor interface
func (qe *queryExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	_ = input
	_ = dest
	// For now, return not implemented
	return fmt.Errorf("ExecuteQuery not implemented")
}

// ExecuteScan implements QueryExecutor interface
func (qe *queryExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	_ = input
	_ = dest
	// For now, return not implemented
	return fmt.Errorf("ExecuteScan not implemented")
}

// ExecuteUpdateItem implements UpdateItemExecutor interface
func (qe *queryExecutor) ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	client, err := qe.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for update item: %w", err)
	}

	updateInput := &dynamodb.UpdateItemInput{
		TableName:                 aws.String(input.TableName),
		Key:                       key,
		UpdateExpression:          aws.String(input.UpdateExpression),
		ExpressionAttributeNames:  input.ExpressionAttributeNames,
		ExpressionAttributeValues: input.ExpressionAttributeValues,
	}

	if input.ConditionExpression != "" {
		updateInput.ConditionExpression = aws.String(input.ConditionExpression)
	}

	if input.ReturnValues != "" {
		updateInput.ReturnValues = types.ReturnValue(input.ReturnValues)
	}

	_, err = client.UpdateItem(context.Background(), updateInput)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to update item: %w", err)
	}

	return nil
}

// ExecuteUpdateItemWithResult implements UpdateItemWithResultExecutor interface
func (qe *queryExecutor) ExecuteUpdateItemWithResult(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error) {
	client, err := qe.db.session.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for update item: %w", err)
	}

	updateInput := &dynamodb.UpdateItemInput{
		TableName:                 aws.String(input.TableName),
		Key:                       key,
		UpdateExpression:          aws.String(input.UpdateExpression),
		ExpressionAttributeNames:  input.ExpressionAttributeNames,
		ExpressionAttributeValues: input.ExpressionAttributeValues,
	}

	if input.ConditionExpression != "" {
		updateInput.ConditionExpression = aws.String(input.ConditionExpression)
	}

	if input.ReturnValues != "" {
		updateInput.ReturnValues = types.ReturnValue(input.ReturnValues)
	}

	output, err := client.UpdateItem(context.Background(), updateInput)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return nil, customerrors.ErrConditionFailed
		}
		return nil, fmt.Errorf("failed to update item: %w", err)
	}

	return &core.UpdateResult{
		Attributes: output.Attributes,
	}, nil
}

// metadataAdapter adapts *model.Metadata to core.ModelMetadata interface
type metadataAdapter struct {
	metadata *model.Metadata
}

func (ma *metadataAdapter) TableName() string {
	return ma.metadata.TableName
}

func (ma *metadataAdapter) PrimaryKey() core.KeySchema {
	if ma.metadata.PrimaryKey == nil {
		return core.KeySchema{}
	}

	schema := core.KeySchema{}
	if ma.metadata.PrimaryKey.PartitionKey != nil {
		schema.PartitionKey = ma.metadata.PrimaryKey.PartitionKey.Name
	}
	if ma.metadata.PrimaryKey.SortKey != nil {
		schema.SortKey = ma.metadata.PrimaryKey.SortKey.Name
	}
	return schema
}

func (ma *metadataAdapter) Indexes() []core.IndexSchema {
	indexes := make([]core.IndexSchema, len(ma.metadata.Indexes))
	for i, idx := range ma.metadata.Indexes {
		schema := core.IndexSchema{
			Name:            idx.Name,
			Type:            string(idx.Type),
			ProjectionType:  idx.ProjectionType,
			ProjectedFields: idx.ProjectedFields,
		}
		if idx.PartitionKey != nil {
			schema.PartitionKey = idx.PartitionKey.Name
		}
		if idx.SortKey != nil {
			schema.SortKey = idx.SortKey.Name
		}
		indexes[i] = schema
	}
	return indexes
}

func (ma *metadataAdapter) AttributeMetadata(field string) *core.AttributeMetadata {
	// First check by Go field name
	fieldMeta, exists := ma.metadata.Fields[field]
	if !exists {
		// Then check by DynamoDB attribute name
		fieldMeta, exists = ma.metadata.FieldsByDBName[field]
		if !exists {
			return nil
		}
	}

	return &core.AttributeMetadata{
		Name:         fieldMeta.Name,
		Type:         fieldMeta.Type.String(),
		DynamoDBName: fieldMeta.DBName,
		Tags:         fieldMeta.Tags,
	}
}

func (ma *metadataAdapter) VersionFieldName() string {
	if ma.metadata == nil {
		return ""
	}
	if ma.metadata.VersionField != nil {
		if ma.metadata.VersionField.DBName != "" {
			return ma.metadata.VersionField.DBName
		}
		return ma.metadata.VersionField.Name
	}
	return ""
}

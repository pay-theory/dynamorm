package dynamorm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/internal/encryption"
	"github.com/pay-theory/dynamorm/internal/expr"
	"github.com/pay-theory/dynamorm/internal/numutil"
	"github.com/pay-theory/dynamorm/internal/reflectutil"
	"github.com/pay-theory/dynamorm/pkg/core"
	customerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/model"
	queryPkg "github.com/pay-theory/dynamorm/pkg/query"
	"github.com/pay-theory/dynamorm/pkg/session"
)

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
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		q.recordBuilderError(err)
		return q
	}

	if fieldMeta, exists := lookupField(metadata, field); exists && fieldMeta != nil {
		if fieldMeta.IsEncrypted {
			q.recordBuilderError(fmt.Errorf("%w: %s", customerrors.ErrEncryptedFieldNotQueryable, fieldMeta.Name))
			return q
		}
	}

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

	if fieldMeta, exists := lookupField(metadata, field); exists && fieldMeta != nil && fieldMeta.IsEncrypted {
		q.recordBuilderError(fmt.Errorf("%w: %s", customerrors.ErrEncryptedFieldNotQueryable, fieldMeta.Name))
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
		if fieldMeta != nil && fieldMeta.IsEncrypted {
			q.recordBuilderError(fmt.Errorf("%w: %s", customerrors.ErrEncryptedFieldNotQueryable, fieldMeta.Name))
			return q
		}

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

	var encSvc *encryption.Service
	if metadata != nil && encryption.MetadataHasEncryptedFields(metadata) {
		if err := encryption.FailClosedIfEncryptedWithoutKMSKeyARN(q.db.session, metadata); err != nil {
			return err
		}
		svc, err := newEncryptionService(q.db.session)
		if err != nil {
			return err
		}
		encSvc = svc
	}

	// Handle map destination (e.g., when ExecuteWithResult is used with a map)
	if destValue.Kind() == reflect.Map {
		// If it's a map, just convert each attribute value
		if destValue.IsNil() {
			destValue.Set(reflect.MakeMap(destValue.Type()))
		}

		for attrName, attrValue := range item {
			if encSvc != nil && metadata != nil {
				if fieldMeta, ok := metadata.FieldsByDBName[attrName]; ok && fieldMeta != nil && fieldMeta.IsEncrypted {
					decrypted, err := encSvc.DecryptAttributeValue(contextOrBackground(q.ctx), fieldMeta.DBName, attrValue)
					if err != nil {
						return &customerrors.EncryptedFieldError{
							Operation: "decrypt",
							Field:     fieldMeta.Name,
							Err:       err,
						}
					}
					attrValue = decrypted
				}
			}

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

		if encSvc != nil && field != nil && field.IsEncrypted {
			decrypted, err := encSvc.DecryptAttributeValue(contextOrBackground(q.ctx), field.DBName, attrValue)
			if err != nil {
				return &customerrors.EncryptedFieldError{
					Operation: "decrypt",
					Field:     field.Name,
					Err:       err,
				}
			}
			attrValue = decrypted
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
	if err := encryption.FailClosedIfEncryptedWithoutKMSKeyARN(q.db.session, metadata); err != nil {
		return nil, err
	}

	var item map[string]types.AttributeValue

	// Use optimized marshaler if available
	if q.db.marshaler != nil {
		var err error
		item, err = q.db.marshaler.MarshalItem(model, metadata)
		if err != nil {
			return nil, err
		}
	} else {
		modelValue := reflect.ValueOf(model)
		if modelValue.Kind() == reflect.Ptr {
			modelValue = modelValue.Elem()
		}

		var err error
		item, err = q.marshalItemReflect(modelValue, metadata)
		if err != nil {
			return nil, err
		}
	}

	if err := q.encryptItemAttributes(metadata, item); err != nil {
		return nil, err
	}

	return item, nil
}

func (q *query) encryptItemAttributes(metadata *model.Metadata, item map[string]types.AttributeValue) error {
	if len(item) == 0 || !encryption.MetadataHasEncryptedFields(metadata) {
		return nil
	}

	svc, err := newEncryptionService(q.db.session)
	if err != nil {
		return err
	}

	for _, fieldMeta := range metadata.Fields {
		if fieldMeta == nil || !fieldMeta.IsEncrypted {
			continue
		}

		av, ok := item[fieldMeta.DBName]
		if !ok {
			continue
		}

		encryptedAV, err := svc.EncryptAttributeValue(contextOrBackground(q.ctx), fieldMeta.DBName, av)
		if err != nil {
			return fmt.Errorf("failed to encrypt field %s: %w", fieldMeta.DBName, err)
		}
		item[fieldMeta.DBName] = encryptedAV
	}

	return nil
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

func contextOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func newEncryptionService(sess *session.Session) (*encryption.Service, error) {
	if sess == nil || sess.Config() == nil {
		return nil, fmt.Errorf("%w: session is nil", customerrors.ErrEncryptionNotConfigured)
	}

	keyARN := sess.Config().KMSKeyARN
	if keyARN == "" {
		return nil, fmt.Errorf("%w: session.Config.KMSKeyARN is empty", customerrors.ErrEncryptionNotConfigured)
	}

	return encryption.NewServiceFromAWSConfig(keyARN, sess.AWSConfig()), nil
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

	if err := encryption.FailClosedIfEncryptedWithoutKMSKeyARN(q.db.session, metadata); err != nil {
		return err
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

	if err = q.addUpdatedAtUpdate(builder, metadata); err != nil {
		return err
	}

	if err = q.addUpdateVersionCondition(builder, metadata, modelValue); err != nil {
		return err
	}

	// Build the update expression
	components := builder.Build()

	conditionExpr, exprAttrNames, exprAttrValues, err := q.mergeQueryConditions(metadata, components.ConditionExpression, components.ExpressionAttributeNames, components.ExpressionAttributeValues)
	if err != nil {
		return err
	}

	if encryption.MetadataHasEncryptedFields(metadata) {
		svc, err := newEncryptionService(q.db.session)
		if err != nil {
			return err
		}
		if err := encryption.EncryptUpdateExpressionValues(contextOrBackground(q.ctx), svc, metadata, components.UpdateExpression, exprAttrNames, exprAttrValues); err != nil {
			return err
		}
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
			if err := builder.AddUpdateSet(fieldMeta.DBName, time.Now()); err != nil {
				return fmt.Errorf("failed to build updated_at update: %w", err)
			}
		case fieldMeta.IsVersion:
			if err := builder.AddUpdateAdd(fieldMeta.DBName, int64(1)); err != nil {
				return fmt.Errorf("failed to build version increment: %w", err)
			}
		default:
			if err := builder.AddUpdateSet(fieldMeta.DBName, fieldValue.Interface()); err != nil {
				return fmt.Errorf("failed to build update for %s: %w", fieldName, err)
			}
		}
	}

	return nil
}

func (q *query) addUpdatedAtUpdate(builder *expr.Builder, metadata *model.Metadata) error {
	if metadata.UpdatedAtField == nil {
		return nil
	}
	if err := builder.AddUpdateSet(metadata.UpdatedAtField.DBName, time.Now()); err != nil {
		return fmt.Errorf("failed to build updated_at update: %w", err)
	}
	return nil
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

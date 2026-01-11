package dynamorm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/internal/encryption"
	"github.com/pay-theory/dynamorm/internal/expr"
	"github.com/pay-theory/dynamorm/internal/numutil"
	"github.com/pay-theory/dynamorm/pkg/core"
	customerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/model"
	queryPkg "github.com/pay-theory/dynamorm/pkg/query"
	"github.com/pay-theory/dynamorm/pkg/schema"
	"github.com/pay-theory/dynamorm/pkg/session"
	"github.com/pay-theory/dynamorm/pkg/transaction"
)

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
	metadata, err := qe.db.registry.GetMetadataByTable(input.TableName)
	if err != nil {
		return fmt.Errorf("failed to resolve model metadata for table %s: %w", input.TableName, err)
	}
	if err := encryption.FailClosedIfEncryptedWithoutKMSKeyARN(qe.db.session, metadata); err != nil {
		return err
	}

	exprAttrValues := input.ExpressionAttributeValues
	if exprAttrValues == nil {
		exprAttrValues = make(map[string]types.AttributeValue)
	}
	if encryption.MetadataHasEncryptedFields(metadata) {
		svc, err := newEncryptionService(qe.db.session)
		if err != nil {
			return err
		}
		if err := encryption.EncryptUpdateExpressionValues(contextOrBackground(qe.db.ctx), svc, metadata, input.UpdateExpression, input.ExpressionAttributeNames, exprAttrValues); err != nil {
			return err
		}
	}

	client, err := qe.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for update item: %w", err)
	}

	updateInput := &dynamodb.UpdateItemInput{
		TableName:                 aws.String(input.TableName),
		Key:                       key,
		UpdateExpression:          aws.String(input.UpdateExpression),
		ExpressionAttributeNames:  input.ExpressionAttributeNames,
		ExpressionAttributeValues: exprAttrValues,
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
	metadata, err := qe.db.registry.GetMetadataByTable(input.TableName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve model metadata for table %s: %w", input.TableName, err)
	}
	if err := encryption.FailClosedIfEncryptedWithoutKMSKeyARN(qe.db.session, metadata); err != nil {
		return nil, err
	}

	exprAttrValues := input.ExpressionAttributeValues
	if exprAttrValues == nil {
		exprAttrValues = make(map[string]types.AttributeValue)
	}
	if encryption.MetadataHasEncryptedFields(metadata) {
		svc, err := newEncryptionService(qe.db.session)
		if err != nil {
			return nil, err
		}
		if err := encryption.EncryptUpdateExpressionValues(contextOrBackground(qe.db.ctx), svc, metadata, input.UpdateExpression, input.ExpressionAttributeNames, exprAttrValues); err != nil {
			return nil, err
		}
	}

	client, err := qe.db.session.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for update item: %w", err)
	}

	updateInput := &dynamodb.UpdateItemInput{
		TableName:                 aws.String(input.TableName),
		Key:                       key,
		UpdateExpression:          aws.String(input.UpdateExpression),
		ExpressionAttributeNames:  input.ExpressionAttributeNames,
		ExpressionAttributeValues: exprAttrValues,
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

	if encryption.MetadataHasEncryptedFields(metadata) && len(output.Attributes) > 0 {
		svc, err := newEncryptionService(qe.db.session)
		if err != nil {
			return nil, err
		}

		for attrName, attrValue := range output.Attributes {
			fieldMeta, ok := metadata.FieldsByDBName[attrName]
			if !ok || fieldMeta == nil || !fieldMeta.IsEncrypted {
				continue
			}

			decrypted, err := svc.DecryptAttributeValue(contextOrBackground(qe.db.ctx), fieldMeta.DBName, attrValue)
			if err != nil {
				return nil, &customerrors.EncryptedFieldError{
					Operation: "decrypt",
					Field:     fieldMeta.Name,
					Err:       err,
				}
			}
			output.Attributes[attrName] = decrypted
		}
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

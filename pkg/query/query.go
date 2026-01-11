package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/pay-theory/dynamorm/internal/expr"
	"github.com/pay-theory/dynamorm/internal/numutil"
	"github.com/pay-theory/dynamorm/internal/reflectutil"
	"github.com/pay-theory/dynamorm/pkg/core"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/index"
	"github.com/pay-theory/dynamorm/pkg/marshal"
	"github.com/pay-theory/dynamorm/pkg/model"
)

// Query represents a DynamoDB query builder
type Query struct {
	builderErr              error
	executor                QueryExecutor
	metadata                core.ModelMetadata
	rawMetadata             *model.Metadata
	converter               AttributeValueConverter
	marshaler               marshal.MarshalerInterface
	ctx                     context.Context
	model                   any
	exclusive               map[string]types.AttributeValue
	retryConfig             *RetryConfig
	totalSegments           *int32
	segment                 *int32
	builder                 *expr.Builder
	offset                  *int
	orderBy                 OrderBy
	index                   string
	projection              []string
	rawFilters              []RawFilter
	filters                 []Filter
	rawConditionExpressions []conditionExpression
	writeConditions         []Condition
	conditions              []Condition
	limit                   int
	consistentRead          bool
}

// Condition represents a query condition
type Condition struct {
	Value    any
	Field    string
	Operator string
}

type conditionExpression struct {
	Values     map[string]any
	Expression string
}

// AttributeValueConverter allows DynamORM callers to inject custom converter behavior.
// It intentionally mirrors the relevant subset of `pkg/types.Converter` without requiring
// callers to depend on that concrete type.
type AttributeValueConverter interface {
	HasCustomConverter(typ reflect.Type) bool
	ToAttributeValue(value any) (types.AttributeValue, error)
	FromAttributeValue(av types.AttributeValue, target any) error
	ConvertToSet(slice any, isSet bool) (types.AttributeValue, error)
}

type rawMetadataProvider interface {
	RawMetadata() *model.Metadata
}

type executorContextSetter interface {
	SetContext(ctx context.Context)
}

// normalizeCondition resolves a condition's field to its canonical DynamoDB attribute name
// and returns the normalized condition along with the Go field name and DynamoDB attribute name.
func (q *Query) normalizeCondition(cond Condition) (Condition, string, string) {
	normalized := cond
	goField := cond.Field
	attrName := cond.Field

	if q.metadata != nil {
		if meta := q.metadata.AttributeMetadata(cond.Field); meta != nil {
			goField = meta.Name
			if meta.DynamoDBName != "" {
				attrName = meta.DynamoDBName
			} else {
				attrName = meta.Name
			}
			normalized.Field = attrName
		}
	}

	return normalized, goField, attrName
}

func (q *Query) rejectEncryptedConditionField(field string) error {
	if q == nil || q.metadata == nil || field == "" {
		return nil
	}

	meta := q.metadata.AttributeMetadata(field)
	if meta == nil || len(meta.Tags) == 0 {
		return nil
	}

	if _, ok := meta.Tags["encrypted"]; !ok {
		return nil
	}

	name := meta.Name
	if name == "" {
		name = field
	}

	return fmt.Errorf("%w: %s", dynamormErrors.ErrEncryptedFieldNotQueryable, name)
}

// addPrimaryKeyCondition appends a condition targeting the table primary key
func (q *Query) addPrimaryKeyCondition(operator string) {
	if q.metadata == nil {
		q.recordBuilderError(fmt.Errorf("metadata is required for conditional helpers"))
		return
	}

	primaryKey := q.metadata.PrimaryKey()
	if primaryKey.PartitionKey == "" {
		q.recordBuilderError(fmt.Errorf("partition key is required for conditional helpers"))
		return
	}

	attrName := q.resolveAttributeName(primaryKey.PartitionKey)
	q.writeConditions = append(q.writeConditions, Condition{
		Field:    attrName,
		Operator: operator,
	})

	if primaryKey.SortKey != "" && operator == "attribute_exists" {
		// attribute_exists(sortKey) ensures full item presence for composite keys
		sortAttr := q.resolveAttributeName(primaryKey.SortKey)
		q.writeConditions = append(q.writeConditions, Condition{
			Field:    sortAttr,
			Operator: operator,
		})
	}
}

// resolveAttributeName maps a Go struct field to its DynamoDB attribute name
func (q *Query) resolveAttributeName(field string) string {
	if q.metadata == nil || field == "" {
		return field
	}

	if meta := q.metadata.AttributeMetadata(field); meta != nil {
		if meta.DynamoDBName != "" {
			return meta.DynamoDBName
		}
		if meta.Name != "" {
			return meta.Name
		}
	}
	return field
}

func (q *Query) resolveGoFieldName(field string) string {
	if q.metadata == nil || field == "" {
		return field
	}
	if meta := q.metadata.AttributeMetadata(field); meta != nil && meta.Name != "" {
		return meta.Name
	}
	return field
}

func cloneConditionValues(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for k, v := range values {
		cloned[k] = v
	}
	return cloned
}

func (q *Query) buildConditionExpression(builder *expr.Builder, includeWhereConditions bool, skipKeyConditions bool, defaultIfEmpty bool) (string, map[string]string, map[string]types.AttributeValue, error) {
	if builder == nil {
		builder = q.newBuilder()
	}
	hasCondition, err := q.addWriteConditions(builder)
	if err != nil {
		return "", nil, nil, err
	}

	if includeWhereConditions {
		added, whereErr := q.addWhereConditions(builder, skipKeyConditions)
		if whereErr != nil {
			return "", nil, nil, whereErr
		}
		hasCondition = hasCondition || added
	}

	if defaultIfEmpty && !hasCondition && len(q.rawConditionExpressions) == 0 {
		if defaultErr := q.addDefaultCondition(builder); defaultErr != nil {
			return "", nil, nil, defaultErr
		}
	}

	components := builder.Build()
	conditionExpr := components.ConditionExpression
	names := components.ExpressionAttributeNames
	values := components.ExpressionAttributeValues

	mergedExpr, mergedValues, err := mergeConditionExpressions(conditionExpr, values, q.rawConditionExpressions, q.converter)
	if err != nil {
		return "", nil, nil, err
	}

	return mergedExpr, names, mergedValues, nil
}

func (q *Query) addWriteConditions(builder *expr.Builder) (bool, error) {
	hasCondition := false
	for _, cond := range q.writeConditions {
		if cond.Field == "" {
			return false, fmt.Errorf("condition field cannot be empty")
		}
		if err := q.rejectEncryptedConditionField(cond.Field); err != nil {
			return false, err
		}
		if err := builder.AddConditionExpression(cond.Field, cond.Operator, cond.Value); err != nil {
			return false, fmt.Errorf("failed to add condition for %s: %w", cond.Field, err)
		}
		hasCondition = true
	}
	return hasCondition, nil
}

func (q *Query) addWhereConditions(builder *expr.Builder, skipKeyConditions bool) (bool, error) {
	if q.metadata == nil {
		return false, fmt.Errorf("model metadata is required for conditional operations")
	}
	primaryKey := q.metadata.PrimaryKey()

	hasCondition := false
	for _, original := range q.conditions {
		if err := q.rejectEncryptedConditionField(original.Field); err != nil {
			return false, err
		}
		normalized, goField, attrName := q.normalizeCondition(original)
		if skipKeyConditions && q.isKeyField(primaryKey, goField, attrName) {
			continue
		}
		if err := builder.AddConditionExpression(normalized.Field, normalized.Operator, normalized.Value); err != nil {
			return false, fmt.Errorf("failed to add condition for %s: %w", normalized.Field, err)
		}
		hasCondition = true
	}
	return hasCondition, nil
}

func (q *Query) addDefaultCondition(builder *expr.Builder) error {
	if q.metadata == nil {
		return fmt.Errorf("model metadata is required for conditional operations")
	}
	pk := q.metadata.PrimaryKey()
	if pk.PartitionKey == "" {
		return fmt.Errorf("partition key is required for default condition")
	}
	if err := builder.AddConditionExpression(q.resolveAttributeName(pk.PartitionKey), "attribute_not_exists", nil); err != nil {
		return fmt.Errorf("failed to add default condition: %w", err)
	}
	return nil
}

func mergeConditionExpressions(baseExpr string, baseValues map[string]types.AttributeValue, rawExpressions []conditionExpression, converter AttributeValueConverter) (string, map[string]types.AttributeValue, error) {
	mergedExpr := baseExpr
	mergedValues := baseValues

	for _, raw := range rawExpressions {
		if raw.Expression == "" {
			continue
		}
		if mergedExpr == "" {
			mergedExpr = raw.Expression
		} else {
			mergedExpr = fmt.Sprintf("(%s) AND (%s)", mergedExpr, raw.Expression)
		}

		if len(raw.Values) == 0 {
			continue
		}

		if mergedValues == nil {
			mergedValues = make(map[string]types.AttributeValue)
		}

		for key, val := range raw.Values {
			if _, exists := mergedValues[key]; exists {
				return "", nil, fmt.Errorf("duplicate placeholder %s in condition expression", key)
			}
			var av types.AttributeValue
			var err error
			if converter != nil {
				av, err = converter.ToAttributeValue(val)
			} else {
				av, err = expr.ConvertToAttributeValue(val)
			}
			if err != nil {
				return "", nil, fmt.Errorf("failed to convert condition value %s: %w", key, err)
			}
			mergedValues[key] = av
		}
	}

	return mergedExpr, mergedValues, nil
}

func (q *Query) isKeyField(schema core.KeySchema, goField, attrName string) bool {
	if schema.PartitionKey != "" {
		if strings.EqualFold(goField, schema.PartitionKey) || strings.EqualFold(attrName, q.resolveAttributeName(schema.PartitionKey)) {
			return true
		}
	}
	if schema.SortKey != "" {
		if strings.EqualFold(goField, schema.SortKey) || strings.EqualFold(attrName, q.resolveAttributeName(schema.SortKey)) {
			return true
		}
	}
	return false
}

// Filter represents a filter expression
type Filter struct {
	Params     map[string]any
	Expression string
}

// RawFilter represents a raw filter with parameters
type RawFilter struct {
	Expression string
	Params     []core.Param
}

// OrderBy represents ordering configuration
type OrderBy struct {
	Field string
	Order string // "asc" or "desc"
}

// RetryConfig configures retry behavior
type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
}

// QueryExecutor is the base query executor interface
type QueryExecutor interface {
	ExecuteQuery(input *core.CompiledQuery, dest any) error
	ExecuteScan(input *core.CompiledQuery, dest any) error
}

// PaginatedQueryExecutor extends QueryExecutor with pagination support
type PaginatedQueryExecutor interface {
	QueryExecutor
	ExecuteQueryWithPagination(input *core.CompiledQuery, dest any) (*QueryResult, error)
	ExecuteScanWithPagination(input *core.CompiledQuery, dest any) (*ScanResult, error)
}

// GetItemExecutor extends QueryExecutor with GetItem support.
type GetItemExecutor interface {
	QueryExecutor
	ExecuteGetItem(input *core.CompiledQuery, key map[string]types.AttributeValue, dest any) error
}

// PutItemExecutor extends QueryExecutor with PutItem support
type PutItemExecutor interface {
	QueryExecutor
	ExecutePutItem(input *core.CompiledQuery, item map[string]types.AttributeValue) error
}

// UpdateItemExecutor extends QueryExecutor with UpdateItem support
type UpdateItemExecutor interface {
	QueryExecutor
	ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error
}

// UpdateItemWithResultExecutor extends UpdateItemExecutor with result support
type UpdateItemWithResultExecutor interface {
	UpdateItemExecutor
	ExecuteUpdateItemWithResult(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error)
}

// DeleteItemExecutor extends QueryExecutor with DeleteItem support
type DeleteItemExecutor interface {
	QueryExecutor
	ExecuteDeleteItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error
}

// BatchWriteItemExecutor extends QueryExecutor with BatchWriteItem support
type BatchWriteItemExecutor interface {
	QueryExecutor
	ExecuteBatchWriteItem(tableName string, writeRequests []types.WriteRequest) (*core.BatchWriteResult, error)
}

// New creates a new Query instance
func New(model any, metadata core.ModelMetadata, executor QueryExecutor) *Query {
	q := &Query{
		model:                   model,
		metadata:                metadata,
		executor:                executor,
		ctx:                     context.Background(),
		filters:                 make([]Filter, 0),
		writeConditions:         make([]Condition, 0),
		rawConditionExpressions: make([]conditionExpression, 0),
	}
	if provider, ok := metadata.(rawMetadataProvider); ok {
		q.rawMetadata = provider.RawMetadata()
	}
	q.setExecutorContext(q.ctx)
	return q
}

// Where adds a condition to the query
func (q *Query) Where(field string, op string, value any) core.Query {
	if err := q.rejectEncryptedConditionField(field); err != nil {
		q.recordBuilderError(err)
		return q
	}
	q.conditions = append(q.conditions, Condition{
		Field:    field,
		Operator: op,
		Value:    value,
	})
	return q
}

// Filter adds a filter expression to the query
func (q *Query) Filter(field string, op string, value any) core.Query {
	if err := q.rejectEncryptedConditionField(field); err != nil {
		q.recordBuilderError(err)
		return q
	}
	// Initialize builder if not already done
	if q.builder == nil {
		q.builder = q.newBuilder()
	}

	if err := q.builder.AddFilterCondition("AND", q.resolveAttributeName(field), op, value); err != nil {
		q.recordBuilderError(err)
	}
	return q
}

// Index specifies which index to use
func (q *Query) Index(name string) core.Query {
	q.index = name
	return q
}

// Limit sets the maximum number of items to return
func (q *Query) Limit(n int) core.Query {
	q.limit = n
	return q
}

// Offset sets the starting position for the query
func (q *Query) Offset(offset int) core.Query {
	q.offset = &offset
	return q
}

// OrderBy sets the sort order
func (q *Query) OrderBy(field string, order string) core.Query {
	q.orderBy = OrderBy{
		Field: field,
		Order: order,
	}
	return q
}

// Select specifies which fields to return
func (q *Query) Select(fields ...string) core.Query {
	if len(fields) == 0 {
		q.projection = nil
		return q
	}

	resolved := make([]string, 0, len(fields))
	for _, field := range fields {
		resolved = append(resolved, q.resolveAttributeName(field))
	}
	q.projection = resolved
	return q
}

// ConsistentRead enables strongly consistent reads for Query operations
func (q *Query) ConsistentRead() core.Query {
	q.consistentRead = true
	return q
}

// WithRetry configures retry behavior for eventually consistent reads
func (q *Query) WithRetry(maxRetries int, initialDelay time.Duration) core.Query {
	q.retryConfig = &RetryConfig{
		MaxRetries:   maxRetries,
		InitialDelay: initialDelay,
	}
	return q
}

// First executes the query and returns the first result
func (q *Query) First(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	if q.retryConfig != nil {
		return q.firstWithRetry(dest)
	}
	return q.firstInternal(dest)
}

// All executes the query and returns all results
func (q *Query) All(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	if q.retryConfig != nil {
		return q.allWithRetry(dest)
	}
	return q.allInternal(dest)
}

// Count returns the count of matching items
func (q *Query) Count() (int64, error) {
	if err := q.checkBuilderError(); err != nil {
		return 0, err
	}
	compiled, err := q.Compile()
	if err != nil {
		return 0, err
	}

	// Set select to COUNT for efficiency
	compiled.Select = "COUNT"

	var result struct {
		Count        int64
		ScannedCount int64
	}

	if compiled.Operation == operationQuery {
		err = q.executor.ExecuteQuery(compiled, &result)
	} else {
		err = q.executor.ExecuteScan(compiled, &result)
	}

	return result.Count, err
}

func (q *Query) firstInternal(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}

	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.IsNil() {
		return fmt.Errorf("destination must be a pointer")
	}
	if destValue.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("destination must be a pointer to a struct")
	}

	clone := *q
	clone.limit = 1

	if getExecutor, ok := clone.executor.(GetItemExecutor); ok {
		getCompiled, key, ok, err := clone.compileGetItem()
		if err != nil {
			return err
		}
		if ok {
			return getExecutor.ExecuteGetItem(getCompiled, key, dest)
		}
	}

	results := reflect.New(reflect.SliceOf(destValue.Elem().Type()))
	if err := clone.allInternal(results.Interface()); err != nil {
		return err
	}

	resultsValue := results.Elem()
	if resultsValue.Len() == 0 {
		return dynamormErrors.ErrItemNotFound
	}

	destValue.Elem().Set(resultsValue.Index(0))
	return nil
}

func (q *Query) firstWithRetry(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}

	delay := q.retryConfig.InitialDelay
	maxDelay := 5 * time.Second

	for attempt := 0; attempt <= q.retryConfig.MaxRetries; attempt++ {
		err := q.firstInternal(dest)
		if err == nil {
			return nil
		}

		if !errors.Is(err, dynamormErrors.ErrItemNotFound) {
			return err
		}

		if attempt >= q.retryConfig.MaxRetries {
			return err
		}

		if delay > 0 {
			time.Sleep(delay)
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}

	return dynamormErrors.ErrItemNotFound
}

func (q *Query) allInternal(dest any) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.IsNil() || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	compiled, err := q.Compile()
	if err != nil {
		return err
	}

	if compiled.Operation == operationQuery {
		return q.executor.ExecuteQuery(compiled, dest)
	}
	return q.executor.ExecuteScan(compiled, dest)
}

func (q *Query) allWithRetry(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}

	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.IsNil() || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	delay := q.retryConfig.InitialDelay
	maxDelay := 5 * time.Second
	var lastErr error

	for attempt := 0; attempt <= q.retryConfig.MaxRetries; attempt++ {
		destValue.Elem().Set(reflect.MakeSlice(destValue.Elem().Type(), 0, 0))

		err := q.allInternal(dest)
		lastErr = err
		if err != nil {
			if attempt >= q.retryConfig.MaxRetries {
				return err
			}
		} else if destValue.Elem().Len() > 0 {
			return nil
		} else if attempt >= q.retryConfig.MaxRetries {
			return nil
		}

		if delay > 0 {
			time.Sleep(delay)
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}

	return lastErr
}

// Create creates a new item
func (q *Query) Create() error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	// Marshal the model to AttributeValues
	item, err := q.marshalItem(q.model)
	if err != nil {
		return fmt.Errorf("failed to marshal item: %w", err)
	}

	// Build PutItem request
	compiled := &core.CompiledQuery{
		Operation: "PutItem",
		TableName: q.metadata.TableName(),
	}

	conditionExpr, names, values, err := q.buildConditionExpression(nil, false, false, false)
	if err != nil {
		return err
	}
	if conditionExpr != "" {
		compiled.ConditionExpression = conditionExpr
	}
	if len(names) > 0 {
		compiled.ExpressionAttributeNames = names
	}
	if len(values) > 0 {
		compiled.ExpressionAttributeValues = values
	}

	// Execute through a specialized PutItem executor
	if putExecutor, ok := q.executor.(PutItemExecutor); ok {
		if err := putExecutor.ExecutePutItem(compiled, item); err != nil {
			if errors.Is(err, dynamormErrors.ErrConditionFailed) {
				return fmt.Errorf("%w: item with the same key already exists", dynamormErrors.ErrConditionFailed)
			}
			return err
		}
		q.updateTimestampsInModel()
		return nil
	}

	// Fallback: return error if executor doesn't support PutItem
	return fmt.Errorf("executor does not support PutItem operation")
}

// CreateOrUpdate creates a new item or updates an existing one (upsert)
func (q *Query) CreateOrUpdate() error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	item, err := q.marshalItem(q.model)
	if err != nil {
		return fmt.Errorf("failed to marshal item: %w", err)
	}

	// Compile the query for PutItem (without condition expression)
	compiled := &core.CompiledQuery{
		Operation: "PutItem",
		TableName: q.metadata.TableName(),
	}

	// Execute through a specialized PutItem executor
	if putExecutor, ok := q.executor.(PutItemExecutor); ok {
		if err := putExecutor.ExecutePutItem(compiled, item); err != nil {
			return err
		}
		q.updateTimestampsInModel()
		return nil
	}

	// Fallback: return error if executor doesn't support PutItem
	return fmt.Errorf("executor does not support PutItem operation")
}

// isZeroValue checks if a reflect.Value is the zero value for its type
func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	case reflect.Struct:
		// Check if it's time.Time
		if v.Type().String() == "time.Time" {
			if isZeroer, ok := v.Interface().(interface{ IsZero() bool }); ok {
				return isZeroer.IsZero()
			}
			return v.IsZero()
		}
		// For other structs, check if all fields are zero
		for i := 0; i < v.NumField(); i++ {
			if !isZeroValue(v.Field(i)) {
				return false
			}
		}
		return true
	default:
		// For other types (chan, func), compare with zero value
		return v.IsZero()
	}
}

// Update updates specified fields on an item
func (q *Query) Update(fields ...string) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}

	key, err := q.buildPrimaryKeyMap("update")
	if err != nil {
		return err
	}

	modelValue := reflect.ValueOf(q.model)
	if modelValue.Kind() == reflect.Ptr {
		if modelValue.IsNil() {
			return fmt.Errorf("model cannot be nil")
		}
		modelValue = modelValue.Elem()
	}
	if modelValue.Kind() != reflect.Struct {
		return fmt.Errorf("model must be a struct or pointer to struct")
	}

	builder := q.newBuilder()

	// Build SET updates either from registry metadata (preferred) or from struct tags.
	if q.rawMetadata != nil {
		fieldsToUpdate := fields
		if len(fieldsToUpdate) == 0 {
			fieldsToUpdate = make([]string, 0, len(q.rawMetadata.Fields))
			for fieldName, fieldMeta := range q.rawMetadata.Fields {
				if fieldMeta == nil || fieldMeta.IsPK || fieldMeta.IsSK || fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt || fieldMeta.IsVersion {
					continue
				}
				fieldValue := modelValue.FieldByIndex(fieldMeta.IndexPath)
				if fieldMeta.OmitEmpty && reflectutil.IsEmpty(fieldValue) {
					continue
				}
				fieldsToUpdate = append(fieldsToUpdate, fieldName)
			}
		}

		for _, fieldName := range fieldsToUpdate {
			fieldMeta, ok := q.rawMetadata.Fields[fieldName]
			if !ok {
				fieldMeta, ok = q.rawMetadata.FieldsByDBName[fieldName]
			}
			if !ok || fieldMeta == nil {
				return fmt.Errorf("field '%s' not found in model metadata (use Go field name or DB attribute name)", fieldName)
			}

			switch {
			case fieldMeta.IsPK || fieldMeta.IsSK:
				return fmt.Errorf("field '%s' is part of the primary key and cannot be updated", fieldName)
			case fieldMeta.IsCreatedAt:
				continue
			case fieldMeta.IsUpdatedAt, fieldMeta.IsVersion:
				continue // handled below
			}

			fieldValue := modelValue.FieldByIndex(fieldMeta.IndexPath)
			if err := builder.AddUpdateSet(fieldMeta.DBName, fieldValue.Interface()); err != nil {
				return fmt.Errorf("failed to build update for %s: %w", fieldName, err)
			}
		}

		// Always update updated_at and increment version when present.
		if q.rawMetadata.UpdatedAtField != nil {
			if err := builder.AddUpdateSet(q.rawMetadata.UpdatedAtField.DBName, time.Now()); err != nil {
				return fmt.Errorf("failed to build updated_at update: %w", err)
			}
		}

		if q.rawMetadata.VersionField != nil {
			current := modelValue.FieldByIndex(q.rawMetadata.VersionField.IndexPath).Int()
			if err := builder.AddConditionExpression(q.rawMetadata.VersionField.DBName, "=", current); err != nil {
				return fmt.Errorf("failed to add version condition: %w", err)
			}
			if err := builder.AddUpdateAdd(q.rawMetadata.VersionField.DBName, int64(1)); err != nil {
				return fmt.Errorf("failed to build version increment: %w", err)
			}
		}
	} else {
		if len(fields) > 0 {
			for _, field := range fields {
				fieldValue := modelValue.FieldByName(field)
				if !fieldValue.IsValid() {
					return fmt.Errorf("field %s not found in model", field)
				}
				if err := builder.AddUpdateSet(q.resolveAttributeName(field), fieldValue.Interface()); err != nil {
					return fmt.Errorf("failed to build update for %s: %w", field, err)
				}
			}
		} else {
			primaryKey := q.metadata.PrimaryKey()
			modelType := modelValue.Type()
			for i := 0; i < modelType.NumField(); i++ {
				field := modelType.Field(i)
				if !field.IsExported() {
					continue
				}

				tag := field.Tag.Get("dynamorm")
				if shouldSkipUpdateField(field, tag, primaryKey) {
					continue
				}

				fieldValue := modelValue.Field(i)
				if strings.Contains(tag, "omitempty") && isZeroValue(fieldValue) {
					continue
				}

				attrName := q.resolveAttributeName(field.Name)
				if err := builder.AddUpdateSet(attrName, fieldValue.Interface()); err != nil {
					return fmt.Errorf("failed to build update for %s: %w", field.Name, err)
				}
			}
		}
	}

	conditionExpr, names, values, err := q.buildConditionExpression(builder, true, true, false)
	if err != nil {
		return err
	}

	components := builder.Build()
	if components.UpdateExpression == "" {
		return fmt.Errorf("no non-key fields to update")
	}

	compiled := &core.CompiledQuery{
		Operation:                 "UpdateItem",
		TableName:                 q.metadata.TableName(),
		UpdateExpression:          components.UpdateExpression,
		ConditionExpression:       conditionExpr,
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
	}

	if updateExecutor, ok := q.executor.(UpdateItemExecutor); ok {
		return updateExecutor.ExecuteUpdateItem(compiled, key)
	}

	return fmt.Errorf("executor does not support UpdateItem operation")
}

func extractKeyValues(primaryKey core.KeySchema, conditions []Condition) (map[string]any, error) {
	keyValues := make(map[string]any)
	for _, cond := range conditions {
		if cond.Field != primaryKey.PartitionKey &&
			(primaryKey.SortKey == "" || cond.Field != primaryKey.SortKey) {
			continue
		}
		if cond.Operator != "=" {
			return nil, fmt.Errorf("key condition must use '=' operator")
		}
		keyValues[cond.Field] = cond.Value
	}
	return keyValues, nil
}

func validateKeyValues(primaryKey core.KeySchema, keyValues map[string]any, operation string) error {
	if _, ok := keyValues[primaryKey.PartitionKey]; !ok {
		return fmt.Errorf("partition key %s is required for %s", primaryKey.PartitionKey, operation)
	}
	if primaryKey.SortKey == "" {
		return nil
	}
	if _, ok := keyValues[primaryKey.SortKey]; !ok {
		return fmt.Errorf("sort key %s is required for %s", primaryKey.SortKey, operation)
	}
	return nil
}

func (q *Query) buildUpdateComponents(fields []string) (string, map[string]string, map[string]types.AttributeValue, error) {
	modelValue := reflect.ValueOf(q.model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}

	updateParts, updateValues, fieldNames, err := q.buildUpdateAssignments(modelValue, fields)
	if err != nil {
		return "", nil, nil, err
	}

	updateExpression := "SET " + strings.Join(updateParts, ", ")
	expressionAttributeNames := buildExpressionAttributeNames(fieldNames)

	expressionAttributeValues, err := convertAnyMapToAttributeValues(updateValues, "failed to convert update value")
	if err != nil {
		return "", nil, nil, err
	}

	return updateExpression, expressionAttributeNames, expressionAttributeValues, nil
}

func (q *Query) buildUpdateAssignments(modelValue reflect.Value, fields []string) ([]string, map[string]any, []string, error) {
	if len(fields) > 0 {
		return buildUpdateAssignmentsForFields(modelValue, fields)
	}
	return buildUpdateAssignmentsForAllFields(modelValue, q.metadata.PrimaryKey())
}

func buildUpdateAssignmentsForFields(modelValue reflect.Value, fields []string) ([]string, map[string]any, []string, error) {
	updateParts := make([]string, 0, len(fields))
	updateValues := make(map[string]any, len(fields))
	fieldNames := make([]string, 0, len(fields))

	for i, field := range fields {
		fieldValue := modelValue.FieldByName(field)
		if !fieldValue.IsValid() {
			return nil, nil, nil, fmt.Errorf("field %s not found in model", field)
		}

		placeholder := fmt.Sprintf(":val%d", i)
		updateParts = append(updateParts, fmt.Sprintf("#%s = %s", field, placeholder))
		updateValues[placeholder] = fieldValue.Interface()
		fieldNames = append(fieldNames, field)
	}

	return updateParts, updateValues, fieldNames, nil
}

func buildUpdateAssignmentsForAllFields(modelValue reflect.Value, primaryKey core.KeySchema) ([]string, map[string]any, []string, error) {
	modelType := modelValue.Type()
	fieldIndex := 0

	updateParts := make([]string, 0, modelType.NumField())
	updateValues := make(map[string]any)
	fieldNames := make([]string, 0, modelType.NumField())

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("dynamorm")
		if shouldSkipUpdateField(field, tag, primaryKey) {
			continue
		}

		fieldValue := modelValue.Field(i)
		if !fieldValue.IsValid() {
			continue
		}

		if strings.Contains(tag, "omitempty") && isZeroValue(fieldValue) {
			continue
		}

		placeholder := fmt.Sprintf(":val%d", fieldIndex)
		updateParts = append(updateParts, fmt.Sprintf("#%s = %s", field.Name, placeholder))
		updateValues[placeholder] = fieldValue.Interface()
		fieldNames = append(fieldNames, field.Name)
		fieldIndex++
	}

	if len(updateParts) == 0 {
		return nil, nil, nil, fmt.Errorf("no non-key fields to update")
	}

	return updateParts, updateValues, fieldNames, nil
}

func shouldSkipUpdateField(field reflect.StructField, tag string, primaryKey core.KeySchema) bool {
	if tag == "-" {
		return true
	}
	if field.Name == primaryKey.PartitionKey || field.Name == primaryKey.SortKey {
		return true
	}
	if strings.Contains(tag, "pk") || strings.Contains(tag, "sk") {
		return true
	}
	return strings.Contains(tag, "created_at")
}

func buildExpressionAttributeNames(fields []string) map[string]string {
	names := make(map[string]string, len(fields))
	for _, field := range fields {
		names["#"+field] = field
	}
	return names
}

func convertAnyMapToAttributeValues(values map[string]any, errorPrefix string) (map[string]types.AttributeValue, error) {
	expressionAttributeValues := make(map[string]types.AttributeValue, len(values))
	for k, v := range values {
		av, err := expr.ConvertToAttributeValue(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", errorPrefix, err)
		}
		expressionAttributeValues[k] = av
	}
	return expressionAttributeValues, nil
}

func mergeExpressionAttributeNames(dst map[string]string, src map[string]string) {
	if len(src) == 0 {
		return
	}
	for k, v := range src {
		dst[k] = v
	}
}

func mergeExpressionAttributeValues(dst map[string]types.AttributeValue, src map[string]types.AttributeValue) error {
	if len(src) == 0 {
		return nil
	}
	for k, v := range src {
		if _, exists := dst[k]; exists {
			return fmt.Errorf("duplicate expression attribute value placeholder: %s", k)
		}
		dst[k] = v
	}
	return nil
}

// Delete deletes an item
func (q *Query) Delete() error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}

	key, err := q.buildPrimaryKeyMap("delete")
	if err != nil {
		return err
	}

	builder := q.newBuilder()
	if q.rawMetadata != nil && q.rawMetadata.VersionField != nil && q.model != nil {
		modelValue := reflect.ValueOf(q.model)
		if modelValue.Kind() == reflect.Ptr && !modelValue.IsNil() {
			modelValue = modelValue.Elem()
		}

		if modelValue.Kind() == reflect.Struct {
			versionValue := modelValue.FieldByIndex(q.rawMetadata.VersionField.IndexPath)
			if !versionValue.IsZero() {
				if err := builder.AddConditionExpression(q.rawMetadata.VersionField.DBName, "=", versionValue.Int()); err != nil {
					return fmt.Errorf("failed to add version condition: %w", err)
				}
			}
		}
	}

	conditionExpr, condNames, condValues, err := q.buildConditionExpression(builder, true, true, false)
	if err != nil {
		return err
	}

	compiled := &core.CompiledQuery{
		Operation:                 "DeleteItem",
		TableName:                 q.metadata.TableName(),
		ConditionExpression:       conditionExpr,
		ExpressionAttributeNames:  condNames,
		ExpressionAttributeValues: condValues,
	}

	if deleteExecutor, ok := q.executor.(DeleteItemExecutor); ok {
		return deleteExecutor.ExecuteDeleteItem(compiled, key)
	}

	return fmt.Errorf("executor does not support DeleteItem operation")
}

// Scan performs a table scan
func (q *Query) Scan(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	compiled, err := q.compileScan()
	if err != nil {
		return err
	}

	return q.executor.ExecuteScan(compiled, dest)
}

// ParallelScan performs a parallel table scan with the specified segment
func (q *Query) ParallelScan(segment int32, totalSegments int32) core.Query {
	q.segment = &segment
	q.totalSegments = &totalSegments
	return q
}

// ScanAllSegments performs a parallel scan across all segments and combines results
func (q *Query) ScanAllSegments(dest any, totalSegments int32) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	// Validate destination is a slice pointer
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}
	sliceType := destValue.Elem().Type()

	// Create a channel to collect results from each segment
	type segmentResult struct {
		err   error
		items []any
	}

	results := make(chan segmentResult, totalSegments)

	// Launch goroutines for each segment
	for i := int32(0); i < totalSegments; i++ {
		go func(segment int32) {
			// Create a new query for this segment
			segmentQuery := &Query{
				builderErr:     q.builderErr,
				model:          q.model,
				conditions:     q.conditions,
				filters:        q.filters,
				rawFilters:     q.rawFilters,
				index:          q.index,
				limit:          q.limit,
				offset:         q.offset,
				projection:     q.projection,
				orderBy:        q.orderBy,
				exclusive:      q.exclusive,
				consistentRead: q.consistentRead,
				ctx:            q.ctx,
				metadata:       q.metadata,
				rawMetadata:    q.rawMetadata,
				converter:      q.converter,
				marshaler:      q.marshaler,
				executor:       q.executor,
				builder:        q.builder,
				segment:        &segment,
				totalSegments:  &totalSegments,
			}

			// Create a slice to hold this segment's results
			elemType := sliceType.Elem()
			segmentDest := reflect.New(reflect.SliceOf(elemType))

			// Execute scan for this segment
			err := segmentQuery.Scan(segmentDest.Interface())
			if err != nil {
				results <- segmentResult{err: err}
				return
			}

			// Convert results to []any
			segmentSlice := segmentDest.Elem()
			items := make([]any, segmentSlice.Len())
			for j := 0; j < segmentSlice.Len(); j++ {
				items[j] = segmentSlice.Index(j).Interface()
			}

			results <- segmentResult{items: items}
		}(i)
	}

	// Collect results from all segments
	var allItems []any
	for i := int32(0); i < totalSegments; i++ {
		result := <-results
		if result.err != nil {
			return result.err
		}
		allItems = append(allItems, result.items...)
	}

	// Combine all results into the destination slice
	destSlice := destValue.Elem()
	newSlice := reflect.MakeSlice(destSlice.Type(), len(allItems), len(allItems))

	for i, item := range allItems {
		newSlice.Index(i).Set(reflect.ValueOf(item))
	}

	destSlice.Set(newSlice)
	return nil
}

// BatchCreate creates multiple items
func (q *Query) BatchCreate(items any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	// Validate items is a slice
	itemsValue := reflect.ValueOf(items)
	if itemsValue.Kind() != reflect.Slice {
		return errors.New("items must be a slice")
	}

	if itemsValue.Len() == 0 {
		return nil
	}

	// Try to use the new BatchWriteItemExecutor first
	if _, ok := q.executor.(BatchWriteItemExecutor); ok {
		tableName := q.metadata.TableName()
		const batchSize = 25
		totalItems := itemsValue.Len()

		for i := 0; i < totalItems; i += batchSize {
			end := i + batchSize
			if end > totalItems {
				end = totalItems
			}

			writeRequests := make([]types.WriteRequest, 0, end-i)
			for j := i; j < end; j++ {
				item := itemsValue.Index(j).Interface()
				av, err := q.marshalItem(item)
				if err != nil {
					return fmt.Errorf("failed to marshal item %d: %w", j, err)
				}

				writeRequests = append(writeRequests, types.WriteRequest{
					PutRequest: &types.PutRequest{
						Item: av,
					},
				})
			}

			if err := q.executeBatchWriteWithRetries(tableName, writeRequests, nil); err != nil {
				return err
			}
		}

		return nil
	}

	// Fall back to old BatchExecutor for backward compatibility
	if executor, ok := q.executor.(BatchExecutor); ok {
		// Build batch write request
		batchWrite := &CompiledBatchWrite{
			TableName: q.metadata.TableName(),
			Items:     make([]map[string]types.AttributeValue, 0, itemsValue.Len()),
		}

		// Convert items to AttributeValues
		for i := 0; i < itemsValue.Len(); i++ {
			item := itemsValue.Index(i).Interface()

			// Convert item to map[string]types.AttributeValue
			av, err := q.marshalItem(item)
			if err != nil {
				return fmt.Errorf("failed to convert item %d: %w", i, err)
			}

			batchWrite.Items = append(batchWrite.Items, av)
		}

		return executor.ExecuteBatchWrite(batchWrite)
	}

	return errors.New("executor does not support batch operations")
}

// WithConverter configures the query to use the provided converter for expression and key/value conversion.
//
// This is optional; when unset, the query falls back to the internal expression converter.
func (q *Query) WithConverter(converter AttributeValueConverter) *Query {
	q.converter = converter
	return q
}

// WithMarshaler configures the query to use the provided marshaler for PutItem-style operations.
//
// This is optional; when unset, the query falls back to reflection-based conversion.
func (q *Query) WithMarshaler(marshaler marshal.MarshalerInterface) *Query {
	q.marshaler = marshaler
	return q
}

func (q *Query) setExecutorContext(ctx context.Context) {
	if ctx == nil {
		return
	}
	if setter, ok := q.executor.(executorContextSetter); ok && setter != nil {
		setter.SetContext(ctx)
	}
}

// WithContext sets the context for the query
func (q *Query) WithContext(ctx context.Context) core.Query {
	if ctx == nil {
		ctx = context.Background()
	}
	q.ctx = ctx
	q.setExecutorContext(ctx)
	return q
}

// selectBestIndex analyzes conditions and selects the optimal index
func (q *Query) selectBestIndex() (*core.IndexSchema, error) {
	// Get all indexes including the primary index
	rawIndexes := make([]core.IndexSchema, 0, len(q.metadata.Indexes())+1)

	// Add the primary index (name is empty)
	primaryKey := q.metadata.PrimaryKey()
	rawIndexes = append(rawIndexes, core.IndexSchema{
		Name:         "",
		Type:         "PRIMARY",
		PartitionKey: primaryKey.PartitionKey,
		SortKey:      primaryKey.SortKey,
	})

	// Add GSIs and LSIs
	rawIndexes = append(rawIndexes, q.metadata.Indexes()...)

	// Keep Go field names; Compile() resolves to DynamoDB names when needed
	selector := index.NewSelector(rawIndexes)

	// Convert our conditions to index.Condition type
	indexConditions := make([]index.Condition, len(q.conditions))
	for i, cond := range q.conditions {
		normalized, goField, attrName := q.normalizeCondition(cond)

		fieldForIndex := goField
		if fieldForIndex == "" {
			fieldForIndex = attrName
		}
		if fieldForIndex == "" {
			fieldForIndex = normalized.Field
		}

		indexConditions[i] = index.Condition{
			Field:    fieldForIndex,
			Operator: normalized.Operator,
			Value:    normalized.Value,
		}
	}

	// Analyze conditions to find required keys
	requiredKeys := index.AnalyzeConditions(indexConditions)

	// Use the selector to find the best index
	return selector.SelectOptimal(requiredKeys, nil)
}

// Compile compiles the query into executable form
func (q *Query) Compile() (*core.CompiledQuery, error) {
	builder := q.effectiveBuilder()

	compiled := &core.CompiledQuery{
		TableName: q.metadata.TableName(),
	}

	if q.index != "" {
		compiled.IndexName = q.index

		var indexSchema *core.IndexSchema
		for _, idx := range q.metadata.Indexes() {
			if idx.Name == q.index {
				copyIdx := idx
				indexSchema = &copyIdx
				break
			}
		}

		keys := q.keyNamesForIndex(indexSchema)
		keyConditions, filterConditions := q.partitionConditionsForKeys(keys)
		hasPartitionKey := false
		for _, cond := range keyConditions {
			if strings.EqualFold(cond.Field, keys.pkAttr) {
				hasPartitionKey = true
				break
			}
		}

		if hasPartitionKey {
			compiled.Operation = operationQuery
			for _, cond := range keyConditions {
				if err := builder.AddKeyCondition(cond.Field, cond.Operator, cond.Value); err != nil {
					return nil, err
				}
			}
			for _, cond := range filterConditions {
				if err := builder.AddFilterCondition("AND", cond.Field, cond.Operator, cond.Value); err != nil {
					return nil, err
				}
			}
		} else {
			compiled.Operation = operationScan
			if err := q.applyScanConditions(builder); err != nil {
				return nil, err
			}
		}
	} else {
		bestIndex, err := q.selectBestIndex()
		if err != nil {
			return nil, err
		}

		if bestIndex != nil {
			compiled.Operation = operationQuery
			if bestIndex.Name != "" {
				compiled.IndexName = bestIndex.Name
			}
			if err := q.applyQueryConditions(builder, bestIndex); err != nil {
				return nil, err
			}
		} else {
			compiled.Operation = operationScan
			if err := q.applyScanConditions(builder); err != nil {
				return nil, err
			}
		}
	}

	q.applyProjections(builder)
	q.applyExpressionComponents(compiled, builder)
	q.applyCompiledSettings(compiled)

	return compiled, nil
}

func (q *Query) partitionConditionsForKeys(keys keyNameSet) ([]Condition, []Condition) {
	keyConditions := make([]Condition, 0)
	filterConditions := make([]Condition, 0)

	for _, original := range q.conditions {
		normalized, goField, attrName := q.normalizeCondition(original)
		condGoName, condAttrName := q.resolveConditionNames(goField, attrName)

		if !keys.isKey(condGoName, condAttrName) {
			filterConditions = append(filterConditions, normalized)
			continue
		}

		operator := strings.ToUpper(strings.TrimSpace(normalized.Operator))
		if keys.isPartitionKey(condGoName, condAttrName) {
			if operator == "=" {
				keyConditions = append(keyConditions, normalized)
			} else {
				filterConditions = append(filterConditions, normalized)
			}
			continue
		}

		switch operator {
		case "=", "<", "<=", ">", ">=", "BETWEEN", "BEGINS_WITH":
			keyConditions = append(keyConditions, normalized)
		default:
			filterConditions = append(filterConditions, normalized)
		}
	}

	return keyConditions, filterConditions
}

func (q *Query) effectiveBuilder() *expr.Builder {
	if q.builder != nil {
		return q.builder.Clone()
	}
	return q.newBuilder()
}

func (q *Query) newBuilder() *expr.Builder {
	if q.converter != nil {
		return expr.NewBuilderWithConverter(q.converter)
	}
	return expr.NewBuilder()
}

func (q *Query) toAttributeValue(value any) (types.AttributeValue, error) {
	if q != nil && q.converter != nil {
		return q.converter.ToAttributeValue(value)
	}
	return expr.ConvertToAttributeValue(value)
}

func (q *Query) buildPrimaryKeyMap(operation string) (map[string]types.AttributeValue, error) {
	if q == nil {
		return nil, fmt.Errorf("query cannot be nil")
	}
	if q.metadata == nil {
		return nil, fmt.Errorf("model metadata is required for %s operations", operation)
	}

	schema := q.metadata.PrimaryKey()
	if schema.PartitionKey == "" {
		return nil, fmt.Errorf("partition key is required for %s", operation)
	}

	pkGo := schema.PartitionKey
	pkAttr := q.resolveAttributeName(pkGo)
	skGo := schema.SortKey
	skAttr := ""
	if skGo != "" {
		skAttr = q.resolveAttributeName(skGo)
	}

	var pkValue any
	var skValue any
	pkFound := false
	skFound := false

	for _, cond := range q.conditions {
		_, goField, attrName := q.normalizeCondition(cond)

		if strings.EqualFold(goField, pkGo) || strings.EqualFold(attrName, pkAttr) {
			if strings.TrimSpace(cond.Operator) != "=" {
				return nil, fmt.Errorf("key condition must use '=' operator")
			}
			pkValue = cond.Value
			pkFound = true
			continue
		}

		if skGo != "" && (strings.EqualFold(goField, skGo) || strings.EqualFold(attrName, skAttr)) {
			if strings.TrimSpace(cond.Operator) != "=" {
				return nil, fmt.Errorf("key condition must use '=' operator")
			}
			skValue = cond.Value
			skFound = true
		}
	}

	if (!pkFound || (skGo != "" && !skFound)) && q.model != nil {
		modelValue := reflect.ValueOf(q.model)
		if modelValue.IsValid() && modelValue.Kind() == reflect.Ptr {
			if modelValue.IsNil() {
				modelValue = reflect.Value{}
			} else {
				modelValue = modelValue.Elem()
			}
		}

		if modelValue.IsValid() && modelValue.Kind() == reflect.Struct {
			if q.rawMetadata != nil && q.rawMetadata.PrimaryKey != nil && q.rawMetadata.PrimaryKey.PartitionKey != nil {
				field := modelValue.FieldByIndex(q.rawMetadata.PrimaryKey.PartitionKey.IndexPath)
				if !field.IsZero() && !pkFound {
					pkValue = field.Interface()
					pkFound = true
				}

				if skGo != "" && q.rawMetadata.PrimaryKey.SortKey != nil {
					field := modelValue.FieldByIndex(q.rawMetadata.PrimaryKey.SortKey.IndexPath)
					if !field.IsZero() && !skFound {
						skValue = field.Interface()
						skFound = true
					}
				}
			} else {
				if !pkFound {
					field := modelValue.FieldByName(pkGo)
					if field.IsValid() && !field.IsZero() {
						pkValue = field.Interface()
						pkFound = true
					}
				}

				if skGo != "" && !skFound {
					field := modelValue.FieldByName(skGo)
					if field.IsValid() && !field.IsZero() {
						skValue = field.Interface()
						skFound = true
					}
				}
			}
		}
	}

	if !pkFound {
		return nil, fmt.Errorf("partition key %s is required for %s", pkGo, operation)
	}
	if skGo != "" && !skFound {
		return nil, fmt.Errorf("sort key %s is required for %s", skGo, operation)
	}

	pkAV, err := q.toAttributeValue(pkValue)
	if err != nil {
		return nil, fmt.Errorf("failed to convert partition key: %w", err)
	}

	key := map[string]types.AttributeValue{
		pkAttr: pkAV,
	}
	if skGo != "" {
		skAV, err := q.toAttributeValue(skValue)
		if err != nil {
			return nil, fmt.Errorf("failed to convert sort key: %w", err)
		}
		key[skAttr] = skAV
	}

	return key, nil
}

type keyNameSet struct {
	pkGo   string
	pkAttr string
	skGo   string
	skAttr string
}

func (k keyNameSet) isKey(goName, attrName string) bool {
	return k.isPartitionKey(goName, attrName) || k.isSortKey(goName, attrName)
}

func (k keyNameSet) isPartitionKey(goName, attrName string) bool {
	if k.pkGo == "" {
		return false
	}
	return strings.EqualFold(goName, k.pkGo) || strings.EqualFold(attrName, k.pkAttr)
}

func (k keyNameSet) isSortKey(goName, attrName string) bool {
	if k.skGo == "" {
		return false
	}
	return strings.EqualFold(goName, k.skGo) || strings.EqualFold(attrName, k.skAttr)
}

func (q *Query) applyQueryConditions(builder *expr.Builder, bestIndex *core.IndexSchema) error {
	keys := q.keyNamesForIndex(bestIndex)
	keyConditions, filterConditions := q.splitConditionsByKey(keys)

	for _, cond := range keyConditions {
		if err := builder.AddKeyCondition(cond.Field, cond.Operator, cond.Value); err != nil {
			return err
		}
	}

	for _, cond := range filterConditions {
		if err := builder.AddFilterCondition("AND", cond.Field, cond.Operator, cond.Value); err != nil {
			return err
		}
	}

	return nil
}

func (q *Query) applyScanConditions(builder *expr.Builder) error {
	for _, original := range q.conditions {
		normalized, _, _ := q.normalizeCondition(original)
		if err := builder.AddFilterCondition("AND", normalized.Field, normalized.Operator, normalized.Value); err != nil {
			return err
		}
	}
	return nil
}

func (q *Query) keyNamesForIndex(bestIndex *core.IndexSchema) keyNameSet {
	primaryKey := q.metadata.PrimaryKey()
	primaryPKGo, primaryPKAttr := q.resolveGoAndAttrName(primaryKey.PartitionKey)
	primarySKGo, primarySKAttr := q.resolveGoAndAttrName(primaryKey.SortKey)

	if bestIndex == nil || bestIndex.Name == "" {
		return keyNameSet{
			pkGo:   primaryPKGo,
			pkAttr: primaryPKAttr,
			skGo:   primarySKGo,
			skAttr: primarySKAttr,
		}
	}

	pkGoName, pkAttrName := q.resolveGoAndAttrName(bestIndex.PartitionKey)
	skGoName, skAttrName := q.resolveGoAndAttrName(bestIndex.SortKey)

	if pkGoName == "" {
		pkGoName = primaryPKGo
	}
	if pkAttrName == "" {
		pkAttrName = primaryPKAttr
	}
	if skGoName == "" {
		skGoName = primarySKGo
	}
	if skAttrName == "" {
		skAttrName = primarySKAttr
	}

	return keyNameSet{
		pkGo:   pkGoName,
		pkAttr: pkAttrName,
		skGo:   skGoName,
		skAttr: skAttrName,
	}
}

func (q *Query) resolveGoAndAttrName(field string) (string, string) {
	return q.resolveGoFieldName(field), q.resolveAttributeName(field)
}

func (q *Query) splitConditionsByKey(keys keyNameSet) ([]Condition, []Condition) {
	keyConditions := make([]Condition, 0)
	filterConditions := make([]Condition, 0)

	for _, original := range q.conditions {
		normalized, goField, attrName := q.normalizeCondition(original)
		condGoName, condAttrName := q.resolveConditionNames(goField, attrName)

		if keys.isKey(condGoName, condAttrName) {
			keyConditions = append(keyConditions, normalized)
		} else {
			filterConditions = append(filterConditions, normalized)
		}
	}

	return keyConditions, filterConditions
}

func (q *Query) resolveConditionNames(goField, attrName string) (string, string) {
	condGoName := goField
	condAttrName := attrName

	if meta := q.metadata.AttributeMetadata(goField); meta != nil {
		if meta.Name != "" {
			condGoName = meta.Name
		}
		if meta.DynamoDBName != "" {
			condAttrName = meta.DynamoDBName
		} else if condAttrName == "" {
			condAttrName = condGoName
		}
	} else if meta := q.metadata.AttributeMetadata(attrName); meta != nil {
		if meta.Name != "" {
			condGoName = meta.Name
		}
		if meta.DynamoDBName != "" {
			condAttrName = meta.DynamoDBName
		}
	}

	return condGoName, condAttrName
}

func (q *Query) applyProjections(builder *expr.Builder) {
	if len(q.projection) == 0 {
		return
	}
	builder.AddProjection(q.projection...)
}

func (q *Query) applyExpressionComponents(compiled *core.CompiledQuery, builder *expr.Builder) {
	components := builder.Build()
	compiled.KeyConditionExpression = components.KeyConditionExpression
	compiled.FilterExpression = components.FilterExpression
	compiled.ProjectionExpression = components.ProjectionExpression
	compiled.ExpressionAttributeNames = components.ExpressionAttributeNames
	compiled.ExpressionAttributeValues = components.ExpressionAttributeValues
}

func (q *Query) applyCompiledSettings(compiled *core.CompiledQuery) {
	if q.limit > 0 {
		limit := numutil.ClampIntToInt32(q.limit)
		compiled.Limit = &limit
	}

	if strings.EqualFold(q.orderBy.Order, "desc") {
		forward := false
		compiled.ScanIndexForward = &forward
	}

	if len(q.exclusive) > 0 {
		compiled.ExclusiveStartKey = q.exclusive
	}

	if q.consistentRead && compiled.IndexName == "" {
		compiled.ConsistentRead = &q.consistentRead
	}
}

// compileScan compiles a scan operation
func (q *Query) compileScan() (*core.CompiledQuery, error) {
	builder := q.effectiveBuilder()

	compiled := &core.CompiledQuery{
		TableName: q.metadata.TableName(),
		Operation: operationScan,
	}
	if q.index != "" {
		compiled.IndexName = q.index
	}

	// Add filter conditions from Where clauses
	for _, original := range q.conditions {
		normalized, _, _ := q.normalizeCondition(original)
		if err := builder.AddFilterCondition("AND", normalized.Field, normalized.Operator, normalized.Value); err != nil {
			return nil, err
		}
	}

	// Note: Additional filters from Filter/OrFilter calls are already in the builder

	// Add projections
	if len(q.projection) > 0 {
		builder.AddProjection(q.projection...)
	}

	// Build the expressions
	components := builder.Build()
	compiled.FilterExpression = components.FilterExpression
	compiled.ProjectionExpression = components.ProjectionExpression
	compiled.ExpressionAttributeNames = components.ExpressionAttributeNames
	compiled.ExpressionAttributeValues = components.ExpressionAttributeValues

	// Set parameters
	if q.limit > 0 {
		limit := numutil.ClampIntToInt32(q.limit)
		compiled.Limit = &limit
	}

	// Handle offset with pagination
	if q.offset != nil && *q.offset > 0 {
		// Note: DynamoDB doesn't support direct offset, so this would need
		// to be handled by the executor with multiple requests
		compiled.Offset = q.offset
	}

	compiled.ExclusiveStartKey = q.exclusive

	// Set parallel scan parameters if specified
	if q.segment != nil && q.totalSegments != nil {
		compiled.Segment = q.segment
		compiled.TotalSegments = q.totalSegments
	}

	// Set consistent read (only for main table scan, not GSI)
	if q.consistentRead && q.index == "" {
		compiled.ConsistentRead = &q.consistentRead
	}

	return compiled, nil
}

func (q *Query) compileGetItem() (*core.CompiledQuery, map[string]types.AttributeValue, bool, error) {
	if q == nil {
		return nil, nil, false, fmt.Errorf("query cannot be nil")
	}
	if q.metadata == nil {
		return nil, nil, false, fmt.Errorf("model metadata is required for get item operations")
	}
	if q.index != "" {
		return nil, nil, false, nil
	}
	if q.builder != nil {
		// Filters (Filter/OrFilter/FilterGroup) cannot be applied via GetItem.
		return nil, nil, false, nil
	}

	schema := q.metadata.PrimaryKey()
	if schema.PartitionKey == "" {
		return nil, nil, false, fmt.Errorf("partition key is required for get item operations")
	}

	pkGo := schema.PartitionKey
	pkAttr := q.resolveAttributeName(pkGo)
	skGo := schema.SortKey
	skAttr := ""
	if skGo != "" {
		skAttr = q.resolveAttributeName(skGo)
	}

	var pkValue any
	var skValue any
	pkFound := false
	skFound := false

	for _, cond := range q.conditions {
		_, goField, attrName := q.normalizeCondition(cond)

		if strings.EqualFold(goField, pkGo) || strings.EqualFold(attrName, pkAttr) {
			if strings.TrimSpace(cond.Operator) != "=" {
				return nil, nil, false, nil
			}
			pkValue = cond.Value
			pkFound = true
			continue
		}

		if skGo != "" && (strings.EqualFold(goField, skGo) || strings.EqualFold(attrName, skAttr)) {
			if strings.TrimSpace(cond.Operator) != "=" {
				return nil, nil, false, nil
			}
			skValue = cond.Value
			skFound = true
			continue
		}

		// Non-key WHERE conditions must use Query/Scan semantics.
		return nil, nil, false, nil
	}

	// Fall back to extracting key values from the model.
	if (!pkFound || (skGo != "" && !skFound)) && q.model != nil {
		modelValue := reflect.ValueOf(q.model)
		if modelValue.IsValid() && modelValue.Kind() == reflect.Ptr {
			if modelValue.IsNil() {
				modelValue = reflect.Value{}
			} else {
				modelValue = modelValue.Elem()
			}
		}

		if modelValue.IsValid() && modelValue.Kind() == reflect.Struct {
			if q.rawMetadata != nil && q.rawMetadata.PrimaryKey != nil && q.rawMetadata.PrimaryKey.PartitionKey != nil {
				field := modelValue.FieldByIndex(q.rawMetadata.PrimaryKey.PartitionKey.IndexPath)
				if !field.IsZero() && !pkFound {
					pkValue = field.Interface()
					pkFound = true
				}

				if skGo != "" && q.rawMetadata.PrimaryKey.SortKey != nil {
					field := modelValue.FieldByIndex(q.rawMetadata.PrimaryKey.SortKey.IndexPath)
					if !field.IsZero() && !skFound {
						skValue = field.Interface()
						skFound = true
					}
				}
			} else {
				if !pkFound {
					field := modelValue.FieldByName(pkGo)
					if field.IsValid() && !field.IsZero() {
						pkValue = field.Interface()
						pkFound = true
					}
				}

				if skGo != "" && !skFound {
					field := modelValue.FieldByName(skGo)
					if field.IsValid() && !field.IsZero() {
						skValue = field.Interface()
						skFound = true
					}
				}
			}
		}
	}

	if !pkFound {
		return nil, nil, false, nil
	}
	if skGo != "" && !skFound {
		return nil, nil, false, nil
	}

	pkAV, err := q.toAttributeValue(pkValue)
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to convert partition key: %w", err)
	}

	key := map[string]types.AttributeValue{
		pkAttr: pkAV,
	}
	if skGo != "" {
		skAV, err := q.toAttributeValue(skValue)
		if err != nil {
			return nil, nil, false, fmt.Errorf("failed to convert sort key: %w", err)
		}
		key[skAttr] = skAV
	}

	compiled := &core.CompiledQuery{
		Operation: "GetItem",
		TableName: q.metadata.TableName(),
	}
	if len(q.projection) > 0 {
		builder := q.newBuilder()
		builder.AddProjection(q.projection...)
		components := builder.Build()
		compiled.ProjectionExpression = components.ProjectionExpression
		compiled.ExpressionAttributeNames = components.ExpressionAttributeNames
	}
	if q.consistentRead {
		compiled.ConsistentRead = &q.consistentRead
	}

	return compiled, key, true, nil
}

// convertItemToAttributeValue converts an item to DynamoDB AttributeValue map
func convertItemToAttributeValue(item any) (map[string]types.AttributeValue, error) {
	// Use our new converter
	av, err := expr.ConvertToAttributeValue(item)
	if err != nil {
		return nil, fmt.Errorf("failed to convert item: %w", err)
	}

	// The converter returns a M type for structs
	if m, ok := av.(*types.AttributeValueMemberM); ok {
		return m.Value, nil
	}

	return nil, fmt.Errorf("expected map type for struct conversion, got %T", av)
}

func (q *Query) marshalItem(item any) (map[string]types.AttributeValue, error) {
	if q == nil {
		return nil, fmt.Errorf("query cannot be nil")
	}

	if q.rawMetadata != nil {
		if q.marshaler != nil {
			return q.marshaler.MarshalItem(item, q.rawMetadata)
		}
		return q.marshalItemReflect(item)
	}

	return q.marshalItemTagged(item)
}

func (q *Query) marshalItemReflect(item any) (map[string]types.AttributeValue, error) {
	if q == nil || q.rawMetadata == nil {
		return nil, fmt.Errorf("model metadata is required for reflection marshal")
	}

	modelValue := reflect.ValueOf(item)
	if modelValue.Kind() == reflect.Ptr {
		if modelValue.IsNil() {
			return nil, fmt.Errorf("item cannot be nil")
		}
		modelValue = modelValue.Elem()
	}
	if modelValue.Kind() != reflect.Struct {
		return nil, fmt.Errorf("item must be a struct or pointer to struct")
	}

	itemMap := make(map[string]types.AttributeValue)
	now := time.Now()

	for _, fieldMeta := range q.rawMetadata.Fields {
		if fieldMeta == nil {
			continue
		}

		fieldValue := modelValue.FieldByIndex(fieldMeta.IndexPath)
		if fieldMeta.OmitEmpty && fieldValue.IsZero() {
			continue
		}

		valueToConvert := fieldValue.Interface()
		switch {
		case fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt:
			valueToConvert = now
		case fieldMeta.IsVersion:
			if fieldValue.IsZero() {
				valueToConvert = int64(0)
			}
		case fieldMeta.IsTTL:
			if fieldValue.Type() == reflect.TypeOf(time.Time{}) && !fieldValue.IsZero() {
				ttlTime, ok := fieldValue.Interface().(time.Time)
				if !ok {
					return nil, fmt.Errorf("expected time.Time for TTL field %s, got %T", fieldMeta.DBName, fieldValue.Interface())
				}
				valueToConvert = ttlTime.Unix()
			}
		}

		var av types.AttributeValue
		var err error
		if q.converter != nil {
			if fieldMeta.IsSet {
				av, err = q.converter.ConvertToSet(valueToConvert, true)
			} else {
				av, err = q.converter.ToAttributeValue(valueToConvert)
			}
		} else {
			av, err = expr.ConvertToAttributeValue(valueToConvert)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to convert field %s: %w", fieldMeta.DBName, err)
		}

		if _, isNull := av.(*types.AttributeValueMemberNULL); isNull && fieldMeta.OmitEmpty {
			continue
		}

		itemMap[fieldMeta.DBName] = av
	}

	return itemMap, nil
}

func (q *Query) marshalItemTagged(item any) (map[string]types.AttributeValue, error) {
	modelValue := reflect.ValueOf(item)
	if !modelValue.IsValid() {
		return nil, fmt.Errorf("item must be a struct")
	}
	if modelValue.Kind() == reflect.Ptr {
		if modelValue.IsNil() {
			return nil, fmt.Errorf("item must be a struct")
		}
		modelValue = modelValue.Elem()
	}
	if modelValue.Kind() != reflect.Struct {
		return nil, fmt.Errorf("item must be a struct")
	}

	modelType := modelValue.Type()
	out := make(map[string]types.AttributeValue)

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("dynamorm")
		if tag == "-" {
			continue
		}

		fieldValue := modelValue.Field(i)
		if !fieldValue.IsValid() {
			continue
		}

		if strings.Contains(tag, "omitempty") && isZeroValue(fieldValue) {
			continue
		}

		var av types.AttributeValue
		var err error
		if q != nil && q.converter != nil {
			av, err = q.converter.ToAttributeValue(fieldValue.Interface())
		} else {
			av, err = expr.ConvertToAttributeValue(fieldValue.Interface())
		}
		if err != nil {
			return nil, fmt.Errorf("failed to convert field %s: %w", field.Name, err)
		}

		out[field.Name] = av
	}

	return out, nil
}

func (q *Query) updateTimestampsInModel() {
	if q == nil || q.rawMetadata == nil || q.model == nil {
		return
	}

	modelValue := reflect.ValueOf(q.model)
	if modelValue.Kind() != reflect.Ptr || modelValue.IsNil() {
		return
	}
	modelValue = modelValue.Elem()
	if modelValue.Kind() != reflect.Struct {
		return
	}

	now := time.Now()

	for _, fieldMeta := range q.rawMetadata.Fields {
		if fieldMeta == nil || (!fieldMeta.IsCreatedAt && !fieldMeta.IsUpdatedAt) {
			continue
		}

		field := modelValue.FieldByIndex(fieldMeta.IndexPath)
		if field.CanSet() && field.Type() == reflect.TypeOf(time.Time{}) {
			field.Set(reflect.ValueOf(now))
		}
	}
}

// OrFilter adds an OR filter condition
func (q *Query) OrFilter(field string, op string, value any) core.Query {
	if err := q.rejectEncryptedConditionField(field); err != nil {
		q.recordBuilderError(err)
		return q
	}
	// Initialize builder if not already done
	if q.builder == nil {
		q.builder = q.newBuilder()
	}

	if err := q.builder.AddFilterCondition("OR", q.resolveAttributeName(field), op, value); err != nil {
		q.recordBuilderError(err)
	}
	return q
}

func (q *Query) addFilterGroup(groupOperator string, fn func(core.Query)) core.Query {
	// Initialize builder if not already done
	if q.builder == nil {
		q.builder = q.newBuilder()
	}

	// Create a new sub-query and builder for the group
	subBuilder := q.newBuilder()
	subQuery := &Query{
		model:    q.model,
		metadata: q.metadata,
		executor: q.executor,
		ctx:      q.ctx,
		builder:  subBuilder,
		// Ensure grouped conditions behave identically to the parent query.
		rawMetadata: q.rawMetadata,
		converter:   q.converter,
		marshaler:   q.marshaler,
	}

	// Execute the user's function to build the sub-query
	fn(subQuery)
	if err := subQuery.checkBuilderError(); err != nil {
		q.recordBuilderError(err)
	}

	// Build the components from the sub-query
	components := subBuilder.Build()

	// Add the built group to the main builder
	q.builder.AddGroupFilter(groupOperator, components)
	return q
}

// FilterGroup adds a grouped AND filter condition
func (q *Query) FilterGroup(fn func(core.Query)) core.Query {
	return q.addFilterGroup("AND", fn)
}

// OrFilterGroup adds a grouped OR filter condition
func (q *Query) OrFilterGroup(fn func(core.Query)) core.Query {
	return q.addFilterGroup("OR", fn)
}

// IfNotExists ensures the primary key does not exist prior to write
func (q *Query) IfNotExists() core.Query {
	q.addPrimaryKeyCondition("attribute_not_exists")
	return q
}

// IfExists ensures the primary key exists prior to write
func (q *Query) IfExists() core.Query {
	q.addPrimaryKeyCondition("attribute_exists")
	return q
}

// WithCondition appends an additional write condition
func (q *Query) WithCondition(field, operator string, value any) core.Query {
	if err := q.rejectEncryptedConditionField(field); err != nil {
		q.recordBuilderError(err)
		return q
	}
	attrName := q.resolveAttributeName(field)
	q.writeConditions = append(q.writeConditions, Condition{
		Field:    attrName,
		Operator: operator,
		Value:    value,
	})
	return q
}

// WithConditionExpression appends a raw condition expression for advanced cases
func (q *Query) WithConditionExpression(exprStr string, values map[string]any) core.Query {
	exprStr = strings.TrimSpace(exprStr)
	if exprStr == "" {
		q.recordBuilderError(fmt.Errorf("condition expression cannot be empty"))
		return q
	}

	q.rawConditionExpressions = append(q.rawConditionExpressions, conditionExpression{
		Expression: exprStr,
		Values:     cloneConditionValues(values),
	})
	return q
}

// recordBuilderError memoizes the first builder error encountered
func (q *Query) recordBuilderError(err error) {
	if err != nil && q.builderErr == nil {
		q.builderErr = err
	}
}

// checkBuilderError returns any previously recorded builder error
func (q *Query) checkBuilderError() error {
	return q.builderErr
}

// UpdateBuilder returns a builder for complex update operations
func (q *Query) UpdateBuilder() core.UpdateBuilder {
	return NewUpdateBuilder(q)
}

// NewWithConditions creates a new Query instance with all necessary fields
func NewWithConditions(model any, metadata core.ModelMetadata, executor QueryExecutor, conditions []Condition, ctx context.Context) *Query { //nolint:revive // context-as-argument: keep signature for compatibility
	if ctx == nil {
		ctx = context.Background()
	}

	q := &Query{
		model:                   model,
		metadata:                metadata,
		executor:                executor,
		conditions:              conditions,
		ctx:                     ctx,
		filters:                 make([]Filter, 0),
		writeConditions:         make([]Condition, 0),
		rawConditionExpressions: make([]conditionExpression, 0),
	}
	if provider, ok := metadata.(rawMetadataProvider); ok {
		q.rawMetadata = provider.RawMetadata()
	}
	q.setExecutorContext(q.ctx)
	return q
}

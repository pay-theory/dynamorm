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
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/index"
)

// Query represents a DynamoDB query builder
type Query struct {
	model                   any
	conditions              []Condition
	writeConditions         []Condition
	rawConditionExpressions []conditionExpression
	filters                 []Filter
	rawFilters              []RawFilter
	index                   string
	limit                   int
	offset                  *int
	projection              []string
	orderBy                 OrderBy
	exclusive               map[string]types.AttributeValue
	ctx                     context.Context

	// Internal state
	metadata core.ModelMetadata
	executor QueryExecutor
	builder  *expr.Builder
	// builderErr captures any expression builder errors encountered while composing filters
	builderErr error

	// Parallel scan configuration
	segment       *int32
	totalSegments *int32

	// Consistency options
	consistentRead bool

	// Retry configuration
	retryConfig *RetryConfig
}

// Condition represents a query condition
type Condition struct {
	Field    string
	Operator string
	Value    any
}

type conditionExpression struct {
	Expression string
	Values     map[string]any
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

func (q *Query) buildConditionExpression(includeWhereConditions bool, skipKeyConditions bool, defaultIfEmpty bool) (string, map[string]string, map[string]types.AttributeValue, error) {
	builder := expr.NewBuilder()
	hasCondition := false

	addCondition := func(field, operator string, value any) error {
		if err := builder.AddConditionExpression(field, operator, value); err != nil {
			return err
		}
		hasCondition = true
		return nil
	}

	for _, cond := range q.writeConditions {
		if cond.Field == "" {
			return "", nil, nil, fmt.Errorf("condition field cannot be empty")
		}
		if err := addCondition(cond.Field, cond.Operator, cond.Value); err != nil {
			return "", nil, nil, fmt.Errorf("failed to add condition for %s: %w", cond.Field, err)
		}
	}

	if includeWhereConditions {
		if q.metadata == nil {
			return "", nil, nil, fmt.Errorf("model metadata is required for conditional operations")
		}
		primaryKey := q.metadata.PrimaryKey()

		for _, original := range q.conditions {
			normalized, goField, attrName := q.normalizeCondition(original)
			if skipKeyConditions && q.isKeyField(primaryKey, goField, attrName) {
				continue
			}
			if err := addCondition(normalized.Field, normalized.Operator, normalized.Value); err != nil {
				return "", nil, nil, fmt.Errorf("failed to add condition for %s: %w", normalized.Field, err)
			}
		}
	}

	if defaultIfEmpty && !hasCondition && len(q.rawConditionExpressions) == 0 {
		if q.metadata == nil {
			return "", nil, nil, fmt.Errorf("model metadata is required for conditional operations")
		}
		pk := q.metadata.PrimaryKey()
		if pk.PartitionKey == "" {
			return "", nil, nil, fmt.Errorf("partition key is required for default condition")
		}
		if err := addCondition(q.resolveAttributeName(pk.PartitionKey), "attribute_not_exists", nil); err != nil {
			return "", nil, nil, fmt.Errorf("failed to add default condition: %w", err)
		}
	}

	components := builder.Build()
	conditionExpr := components.ConditionExpression
	names := components.ExpressionAttributeNames
	values := components.ExpressionAttributeValues

	mergedExpr := conditionExpr
	mergedValues := values

	for _, raw := range q.rawConditionExpressions {
		if raw.Expression == "" {
			continue
		}
		if mergedExpr == "" {
			mergedExpr = raw.Expression
		} else {
			mergedExpr = fmt.Sprintf("(%s) AND (%s)", mergedExpr, raw.Expression)
		}
		if len(raw.Values) > 0 {
			if mergedValues == nil {
				mergedValues = make(map[string]types.AttributeValue)
			}
			for key, val := range raw.Values {
				if _, exists := mergedValues[key]; exists {
					return "", nil, nil, fmt.Errorf("duplicate placeholder %s in condition expression", key)
				}
				av, err := expr.ConvertToAttributeValue(val)
				if err != nil {
					return "", nil, nil, fmt.Errorf("failed to convert condition value %s: %w", key, err)
				}
				mergedValues[key] = av
			}
		}
	}

	return mergedExpr, names, mergedValues, nil
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
	Expression string
	Params     map[string]any
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
	return &Query{
		model:                   model,
		metadata:                metadata,
		executor:                executor,
		filters:                 make([]Filter, 0),
		writeConditions:         make([]Condition, 0),
		rawConditionExpressions: make([]conditionExpression, 0),
	}
}

// Where adds a condition to the query
func (q *Query) Where(field string, op string, value any) core.Query {
	q.conditions = append(q.conditions, Condition{
		Field:    field,
		Operator: op,
		Value:    value,
	})
	return q
}

// Filter adds a filter expression to the query
func (q *Query) Filter(field string, op string, value any) core.Query {
	// Initialize builder if not already done
	if q.builder == nil {
		q.builder = expr.NewBuilder()
	}

	if err := q.builder.AddFilterCondition("AND", field, op, value); err != nil {
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
	q.projection = fields
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
	// Set limit to 1 for efficiency
	q.limit = 1

	compiled, err := q.Compile()
	if err != nil {
		return err
	}

	if compiled.Operation == "Query" {
		return q.executor.ExecuteQuery(compiled, dest)
	}
	return q.executor.ExecuteScan(compiled, dest)
}

// All executes the query and returns all results
func (q *Query) All(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	compiled, err := q.Compile()
	if err != nil {
		return err
	}

	if compiled.Operation == "Query" {
		return q.executor.ExecuteQuery(compiled, dest)
	}
	return q.executor.ExecuteScan(compiled, dest)
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

	if compiled.Operation == "Query" {
		err = q.executor.ExecuteQuery(compiled, &result)
	} else {
		err = q.executor.ExecuteScan(compiled, &result)
	}

	return result.Count, err
}

// Create creates a new item
func (q *Query) Create() error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	// Marshal the model to AttributeValues
	item, err := convertItemToAttributeValue(q.model)
	if err != nil {
		return fmt.Errorf("failed to marshal item: %w", err)
	}

	// Build PutItem request
	compiled := &core.CompiledQuery{
		Operation: "PutItem",
		TableName: q.metadata.TableName(),
	}

	conditionExpr, names, values, err := q.buildConditionExpression(false, false, false)
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
		return putExecutor.ExecutePutItem(compiled, item)
	}

	// Fallback: return error if executor doesn't support PutItem
	return fmt.Errorf("executor does not support PutItem operation")
}

// CreateOrUpdate creates a new item or updates an existing one (upsert)
func (q *Query) CreateOrUpdate() error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	// Build the item to put
	item := make(map[string]types.AttributeValue)

	modelValue := reflect.ValueOf(q.model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}
	modelType := modelValue.Type()

	// Convert all fields to AttributeValues
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Parse dynamorm tags
		tag := field.Tag.Get("dynamorm")
		if tag == "-" {
			continue
		}

		// Get field value
		fieldValue := modelValue.Field(i)
		if !fieldValue.IsValid() {
			continue
		}

		// Skip zero values if omitempty is set
		if strings.Contains(tag, "omitempty") && isZeroValue(fieldValue) {
			continue
		}

		// Convert to AttributeValue
		av, err := expr.ConvertToAttributeValue(fieldValue.Interface())
		if err != nil {
			return fmt.Errorf("failed to convert field %s: %w", field.Name, err)
		}

		// Use field name as key
		item[field.Name] = av
	}

	// Compile the query for PutItem (without condition expression)
	compiled := &core.CompiledQuery{
		Operation: "PutItem",
		TableName: q.metadata.TableName(),
	}

	// Execute through a specialized PutItem executor
	if putExecutor, ok := q.executor.(PutItemExecutor); ok {
		return putExecutor.ExecutePutItem(compiled, item)
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
			return v.Interface().(interface{ IsZero() bool }).IsZero()
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
	// Validate we have key conditions
	primaryKey := q.metadata.PrimaryKey()
	keyValues := make(map[string]any)

	// Extract key values from conditions
	for _, cond := range q.conditions {
		if cond.Field == primaryKey.PartitionKey ||
			(primaryKey.SortKey != "" && cond.Field == primaryKey.SortKey) {
			if cond.Operator != "=" {
				return fmt.Errorf("key condition must use '=' operator")
			}
			keyValues[cond.Field] = cond.Value
		}
	}

	// Validate we have complete key
	if _, ok := keyValues[primaryKey.PartitionKey]; !ok {
		return fmt.Errorf("partition key %s is required for update", primaryKey.PartitionKey)
	}
	if primaryKey.SortKey != "" {
		if _, ok := keyValues[primaryKey.SortKey]; !ok {
			return fmt.Errorf("sort key %s is required for update", primaryKey.SortKey)
		}
	}

	// Build update expression for specified fields
	updateParts := []string{}
	updateValues := make(map[string]any)

	modelValue := reflect.ValueOf(q.model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}
	modelType := modelValue.Type()

	if len(fields) > 0 {
		// Update only specified fields
		for i, field := range fields {
			// Get field value from model
			fieldValue := modelValue.FieldByName(field)
			if !fieldValue.IsValid() {
				return fmt.Errorf("field %s not found in model", field)
			}

			// Add to update expression
			placeholder := fmt.Sprintf(":val%d", i)
			updateParts = append(updateParts, fmt.Sprintf("#%s = %s", field, placeholder))
			updateValues[placeholder] = fieldValue.Interface()
		}
	} else {
		// Update all non-key fields
		primaryKey := q.metadata.PrimaryKey()
		fieldIndex := 0

		for i := 0; i < modelType.NumField(); i++ {
			field := modelType.Field(i)

			// Skip unexported fields
			if !field.IsExported() {
				continue
			}

			// Parse dynamorm tags
			tag := field.Tag.Get("dynamorm")
			if tag == "-" {
				continue
			}

			// Skip primary key fields
			if field.Name == primaryKey.PartitionKey || field.Name == primaryKey.SortKey {
				continue
			}

			// Check if this is a primary key field based on tags
			if strings.Contains(tag, "pk") || strings.Contains(tag, "sk") {
				continue
			}

			// Skip special fields based on tags
			if strings.Contains(tag, "created_at") {
				continue
			}

			// Get field value
			fieldValue := modelValue.Field(i)
			if !fieldValue.IsValid() {
				continue
			}

			// Skip zero values if omitempty is set
			if strings.Contains(tag, "omitempty") && isZeroValue(fieldValue) {
				continue
			}

			// Add to update expression
			placeholder := fmt.Sprintf(":val%d", fieldIndex)
			updateParts = append(updateParts, fmt.Sprintf("#%s = %s", field.Name, placeholder))
			updateValues[placeholder] = fieldValue.Interface()

			// Also add to expression attribute names
			fields = append(fields, field.Name)
			fieldIndex++
		}

		// Check if we have any fields to update
		if len(updateParts) == 0 {
			return fmt.Errorf("no non-key fields to update")
		}
	}

	// Build update expression manually
	updateExpression := ""
	if len(updateParts) > 0 {
		updateExpression = "SET " + strings.Join(updateParts, ", ")
	}

	// Build expression attribute names
	expressionAttributeNames := make(map[string]string)
	for _, field := range fields {
		expressionAttributeNames["#"+field] = field
	}

	// Convert update values to AttributeValues
	expressionAttributeValues := make(map[string]types.AttributeValue)
	for k, v := range updateValues {
		av, err := expr.ConvertToAttributeValue(v)
		if err != nil {
			return fmt.Errorf("failed to convert update value: %w", err)
		}
		expressionAttributeValues[k] = av
	}

	conditionExpr, condNames, condValues, err := q.buildConditionExpression(true, true, false)
	if err != nil {
		return err
	}

	if len(condNames) > 0 {
		for k, v := range condNames {
			expressionAttributeNames[k] = v
		}
	}

	if len(condValues) > 0 {
		for k, v := range condValues {
			if _, exists := expressionAttributeValues[k]; exists {
				return fmt.Errorf("duplicate expression attribute value placeholder: %s", k)
			}
			expressionAttributeValues[k] = v
		}
	}

	// Compile the update query
	compiled := &core.CompiledQuery{
		Operation:                 "UpdateItem",
		TableName:                 q.metadata.TableName(),
		UpdateExpression:          updateExpression,
		ConditionExpression:       conditionExpr,
		ExpressionAttributeNames:  expressionAttributeNames,
		ExpressionAttributeValues: expressionAttributeValues,
	}

	// Convert key to AttributeValues
	keyAV := make(map[string]types.AttributeValue)
	for k, v := range keyValues {
		av, err := expr.ConvertToAttributeValue(v)
		if err != nil {
			return fmt.Errorf("failed to convert key value: %w", err)
		}
		keyAV[k] = av
	}

	// Execute update
	if updateExecutor, ok := q.executor.(UpdateItemExecutor); ok {
		return updateExecutor.ExecuteUpdateItem(compiled, keyAV)
	}

	return fmt.Errorf("executor does not support UpdateItem operation")
}

// Delete deletes an item
func (q *Query) Delete() error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	// Validate we have key conditions
	primaryKey := q.metadata.PrimaryKey()
	keyValues := make(map[string]any)

	// Extract key values from conditions
	for _, cond := range q.conditions {
		if cond.Field == primaryKey.PartitionKey ||
			(primaryKey.SortKey != "" && cond.Field == primaryKey.SortKey) {
			if cond.Operator != "=" {
				return fmt.Errorf("key condition must use '=' operator")
			}
			keyValues[cond.Field] = cond.Value
		}
	}

	// Validate we have complete key
	if _, ok := keyValues[primaryKey.PartitionKey]; !ok {
		return fmt.Errorf("partition key %s is required for delete", primaryKey.PartitionKey)
	}
	if primaryKey.SortKey != "" {
		if _, ok := keyValues[primaryKey.SortKey]; !ok {
			return fmt.Errorf("sort key %s is required for delete", primaryKey.SortKey)
		}
	}

	conditionExpr, condNames, condValues, err := q.buildConditionExpression(true, true, false)
	if err != nil {
		return err
	}

	// Compile the delete query
	compiled := &core.CompiledQuery{
		Operation:                 "DeleteItem",
		TableName:                 q.metadata.TableName(),
		ConditionExpression:       conditionExpr,
		ExpressionAttributeNames:  condNames,
		ExpressionAttributeValues: condValues,
	}

	// Convert key to AttributeValues
	keyAV := make(map[string]types.AttributeValue)
	for k, v := range keyValues {
		av, err := expr.ConvertToAttributeValue(v)
		if err != nil {
			return fmt.Errorf("failed to convert key value: %w", err)
		}
		keyAV[k] = av
	}

	// Execute delete
	if deleteExecutor, ok := q.executor.(DeleteItemExecutor); ok {
		return deleteExecutor.ExecuteDeleteItem(compiled, keyAV)
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

	// Create a channel to collect results from each segment
	type segmentResult struct {
		items []any
		err   error
	}

	results := make(chan segmentResult, totalSegments)

	// Launch goroutines for each segment
	for i := int32(0); i < totalSegments; i++ {
		go func(segment int32) {
			// Create a new query for this segment
			segmentQuery := &Query{
				model:         q.model,
				conditions:    q.conditions,
				filters:       q.filters,
				rawFilters:    q.rawFilters,
				index:         q.index,
				limit:         q.limit,
				offset:        q.offset,
				projection:    q.projection,
				orderBy:       q.orderBy,
				exclusive:     q.exclusive,
				ctx:           q.ctx,
				metadata:      q.metadata,
				executor:      q.executor,
				builder:       q.builder,
				segment:       &segment,
				totalSegments: &totalSegments,
			}

			// Create a slice to hold this segment's results
			elemType := destValue.Type().Elem()
			segmentDest := reflect.New(reflect.SliceOf(elemType))

			// Execute scan for this segment
			err := segmentQuery.Scan(segmentDest.Interface())
			if err != nil {
				results <- segmentResult{nil, err}
				return
			}

			// Convert results to []any
			segmentSlice := segmentDest.Elem()
			items := make([]any, segmentSlice.Len())
			for j := 0; j < segmentSlice.Len(); j++ {
				items[j] = segmentSlice.Index(j).Interface()
			}

			results <- segmentResult{items, nil}
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

// BatchGet retrieves multiple items by their primary keys
func (q *Query) BatchGet(keys []any, dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	// Validate dest is a pointer to slice
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return errors.New("dest must be a pointer to slice")
	}

	// Validate keys
	if len(keys) == 0 {
		return errors.New("no keys provided")
	}

	if len(keys) > 100 {
		return errors.New("BatchGet supports maximum 100 keys per request")
	}

	// Build batch get request
	batchGet := &CompiledBatchGet{
		TableName: q.metadata.TableName(),
		Keys:      make([]map[string]types.AttributeValue, 0, len(keys)),
	}

	// Add projection if specified
	if len(q.projection) > 0 {
		builder := expr.NewBuilder()
		builder.AddProjection(q.projection...)
		components := builder.Build()
		batchGet.ProjectionExpression = components.ProjectionExpression
		batchGet.ExpressionAttributeNames = components.ExpressionAttributeNames
	}

	// Convert keys to AttributeValues
	primaryKey := q.metadata.PrimaryKey()
	for _, key := range keys {
		keyMap := make(map[string]types.AttributeValue)

		// Handle composite keys
		keyValue := reflect.ValueOf(key)
		if keyValue.Kind() == reflect.Struct {
			// Extract partition and sort key from struct
			for i := 0; i < keyValue.NumField(); i++ {
				field := keyValue.Type().Field(i)
				if field.Name == primaryKey.PartitionKey ||
					(primaryKey.SortKey != "" && field.Name == primaryKey.SortKey) {
					av, err := expr.ConvertToAttributeValue(keyValue.Field(i).Interface())
					if err != nil {
						return fmt.Errorf("failed to convert key field %s: %w", field.Name, err)
					}
					keyMap[field.Name] = av
				}
			}
		} else {
			// Simple primary key
			av, err := expr.ConvertToAttributeValue(key)
			if err != nil {
				return fmt.Errorf("failed to convert key: %w", err)
			}
			keyMap[primaryKey.PartitionKey] = av
		}

		batchGet.Keys = append(batchGet.Keys, keyMap)
	}

	// Execute batch get through executor
	if executor, ok := q.executor.(BatchExecutor); ok {
		return executor.ExecuteBatchGet(batchGet, dest)
	}

	return errors.New("executor does not support batch operations")
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
		return errors.New("no items to create")
	}

	if itemsValue.Len() > 25 {
		return errors.New("BatchCreate supports maximum 25 items per request")
	}

	// Try to use the new BatchWriteItemExecutor first
	if batchWriteExecutor, ok := q.executor.(BatchWriteItemExecutor); ok {
		// Convert items to write requests
		writeRequests := make([]types.WriteRequest, 0, itemsValue.Len())

		for i := 0; i < itemsValue.Len(); i++ {
			item := itemsValue.Index(i).Interface()

			// Convert item to AttributeValues
			av, err := convertItemToAttributeValue(item)
			if err != nil {
				return fmt.Errorf("failed to convert item %d: %w", i, err)
			}

			writeRequests = append(writeRequests, types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: av,
				},
			})
		}

		// Execute batch write
		result, err := batchWriteExecutor.ExecuteBatchWriteItem(q.metadata.TableName(), writeRequests)
		if err != nil {
			return err
		}

		// Check for unprocessed items
		if len(result.UnprocessedItems) > 0 {
			unprocessedCount := 0
			for _, items := range result.UnprocessedItems {
				unprocessedCount += len(items)
			}
			if unprocessedCount > 0 {
				return fmt.Errorf("%d items were not processed", unprocessedCount)
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
			av, err := convertItemToAttributeValue(item)
			if err != nil {
				return fmt.Errorf("failed to convert item %d: %w", i, err)
			}

			batchWrite.Items = append(batchWrite.Items, av)
		}

		return executor.ExecuteBatchWrite(batchWrite)
	}

	return errors.New("executor does not support batch operations")
}

// WithContext sets the context for the query
func (q *Query) WithContext(ctx context.Context) core.Query {
	q.ctx = ctx
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

// AllPaginated executes the query and returns paginated results
func (q *Query) AllPaginated(dest any) (*core.PaginatedResult, error) {
	if err := q.checkBuilderError(); err != nil {
		return nil, err
	}
	// Set a reasonable limit if not specified
	if q.limit == 0 {
		q.limit = 100
	}

	compiled, err := q.Compile()
	if err != nil {
		return nil, err
	}

	// Execute the query
	var result any
	if compiled.Operation == "Query" {
		result, err = q.executePaginatedQuery(compiled, dest)
	} else {
		result, err = q.executePaginatedScan(compiled, dest)
	}

	if err != nil {
		return nil, err
	}

	// Extract pagination info
	queryResult := result.(map[string]any)

	// Build the paginated result
	paginatedResult := &core.PaginatedResult{
		Items:        dest,
		NextCursor:   q.encodeCursor(queryResult["LastEvaluatedKey"]),
		Count:        0,
		ScannedCount: 0,
	}

	// Safely extract counts
	if count, ok := queryResult["Count"].(int64); ok {
		paginatedResult.Count = int(count)
	} else if count, ok := queryResult["Count"].(int); ok {
		paginatedResult.Count = count
	}

	if scannedCount, ok := queryResult["ScannedCount"].(int64); ok {
		paginatedResult.ScannedCount = int(scannedCount)
	} else if scannedCount, ok := queryResult["ScannedCount"].(int); ok {
		paginatedResult.ScannedCount = scannedCount
	}

	// Set HasMore based on cursor
	paginatedResult.HasMore = paginatedResult.NextCursor != ""

	// Extract LastEvaluatedKey
	if lastKey, ok := queryResult["LastEvaluatedKey"].(map[string]types.AttributeValue); ok {
		paginatedResult.LastEvaluatedKey = lastKey
	}

	return paginatedResult, nil
}

// SetCursor sets the pagination cursor for the query
func (q *Query) SetCursor(cursor string) error {
	if cursor == "" {
		return nil
	}

	// Decode the cursor to ExclusiveStartKey
	startKey, err := q.decodeCursor(cursor)
	if err != nil {
		return fmt.Errorf("invalid cursor: %w", err)
	}

	q.exclusive = startKey
	return nil
}

// Cursor is a fluent method to set the pagination cursor
func (q *Query) Cursor(cursor string) core.Query {
	if err := q.SetCursor(cursor); err != nil {
		q.recordBuilderError(err)
	}
	return q
}

// executePaginatedQuery executes a query with pagination support
func (q *Query) executePaginatedQuery(compiled *core.CompiledQuery, dest any) (any, error) {
	// Check if executor supports pagination
	if paginatedExecutor, ok := q.executor.(PaginatedQueryExecutor); ok {
		result, err := paginatedExecutor.ExecuteQueryWithPagination(compiled, dest)
		if err != nil {
			return nil, err
		}

		// Return the actual pagination info
		return map[string]any{
			"Count":            result.Count,
			"ScannedCount":     result.ScannedCount,
			"LastEvaluatedKey": result.LastEvaluatedKey,
		}, nil
	}

	// Fall back to regular query without pagination info
	err := q.executor.ExecuteQuery(compiled, dest)
	if err != nil {
		return nil, err
	}

	// Return mock result for backward compatibility
	return map[string]any{
		"Count":            0,
		"ScannedCount":     0,
		"LastEvaluatedKey": nil,
	}, nil
}

// executePaginatedScan executes a scan with pagination support
func (q *Query) executePaginatedScan(compiled *core.CompiledQuery, dest any) (any, error) {
	// Check if executor supports pagination
	if paginatedExecutor, ok := q.executor.(PaginatedQueryExecutor); ok {
		result, err := paginatedExecutor.ExecuteScanWithPagination(compiled, dest)
		if err != nil {
			return nil, err
		}

		// Return the actual pagination info
		return map[string]any{
			"Count":            result.Count,
			"ScannedCount":     result.ScannedCount,
			"LastEvaluatedKey": result.LastEvaluatedKey,
		}, nil
	}

	// Fall back to regular scan without pagination info
	err := q.executor.ExecuteScan(compiled, dest)
	if err != nil {
		return nil, err
	}

	// Return mock result for backward compatibility
	return map[string]any{
		"Count":            0,
		"ScannedCount":     0,
		"LastEvaluatedKey": nil,
	}, nil
}

// encodeCursor encodes the LastEvaluatedKey as a cursor string
func (q *Query) encodeCursor(lastKey any) string {
	if lastKey == nil {
		return ""
	}

	// Convert to map[string]types.AttributeValue if needed
	var avMap map[string]types.AttributeValue
	switch v := lastKey.(type) {
	case map[string]types.AttributeValue:
		avMap = v
	case map[string]any:
		// Handle the case where lastKey is map[string]any
		// This would come from the executor results
		if val, ok := v["LastEvaluatedKey"]; ok {
			if m, ok := val.(map[string]types.AttributeValue); ok {
				avMap = m
			}
		}
	default:
		return ""
	}

	if len(avMap) == 0 {
		return ""
	}

	// Use the new EncodeCursor function
	encoded, err := EncodeCursor(avMap, q.index, q.orderBy.Order)
	if err != nil {
		// Log error in production
		return ""
	}
	return encoded
}

// decodeCursor decodes a cursor string to ExclusiveStartKey
func (q *Query) decodeCursor(cursor string) (map[string]types.AttributeValue, error) {
	if cursor == "" {
		return nil, nil
	}

	// Use the new DecodeCursor function
	decodedCursor, err := DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}

	if decodedCursor == nil {
		return nil, nil
	}

	// Convert back to AttributeValues
	return decodedCursor.ToAttributeValues()
}

// Compile compiles the query into executable form
func (q *Query) Compile() (*core.CompiledQuery, error) {
	// Use existing builder if available (contains filters from Filter/OrFilter calls)
	// Otherwise create a new one
	var builder *expr.Builder
	if q.builder != nil {
		builder = q.builder
	} else {
		builder = expr.NewBuilder()
	}

	// Select the best index
	bestIndex, err := q.selectBestIndex()
	if err != nil {
		return nil, err
	}

	compiled := &core.CompiledQuery{
		TableName: q.metadata.TableName(),
	}

	// If we have a suitable index, use Query operation
	if bestIndex != nil {
		compiled.Operation = "Query"
		if bestIndex.Name != "" {
			compiled.IndexName = bestIndex.Name
		}

		// Separate key conditions from filter conditions
		var keyConditions []Condition
		var filterConditions []Condition

		resolveNames := func(field string) (string, string) {
			if field == "" {
				return "", ""
			}
			goName := field
			attrName := field
			if meta := q.metadata.AttributeMetadata(field); meta != nil {
				if meta.Name != "" {
					goName = meta.Name
				}
				if meta.DynamoDBName != "" {
					attrName = meta.DynamoDBName
				} else {
					attrName = goName
				}
			}
			return goName, attrName
		}

		primaryKey := q.metadata.PrimaryKey()
		primaryPKGo, primaryPKAttr := resolveNames(primaryKey.PartitionKey)
		primarySKGo, primarySKAttr := resolveNames(primaryKey.SortKey)

		// Resolve Go and DynamoDB names for the keys based on the selected index
		var pkGoName, pkAttrName, skGoName, skAttrName string

		if bestIndex.Name == "" {
			// Primary table uses primary key definition
			pkGoName, pkAttrName = resolveNames(primaryKey.PartitionKey)
			skGoName, skAttrName = resolveNames(primaryKey.SortKey)
		} else {
			// Secondary indexes provide their own Go field names
			pkGoName, pkAttrName = resolveNames(bestIndex.PartitionKey)
			skGoName, skAttrName = resolveNames(bestIndex.SortKey)
		}

		// Fall back to primary key metadata if resolution fails
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

		for _, original := range q.conditions {
			normalized, goField, attrName := q.normalizeCondition(original)

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

			isPartitionKey := false
			if pkGoName != "" {
				isPartitionKey = strings.EqualFold(condGoName, pkGoName) || strings.EqualFold(condAttrName, pkAttrName)
			}

			isSortKey := false
			if skGoName != "" {
				isSortKey = strings.EqualFold(condGoName, skGoName) || strings.EqualFold(condAttrName, skAttrName)
			}

			if isPartitionKey || isSortKey {
				keyConditions = append(keyConditions, normalized)
			} else {
				filterConditions = append(filterConditions, normalized)
			}
		}

		// Add key conditions
		for _, cond := range keyConditions {
			if err := builder.AddKeyCondition(cond.Field, cond.Operator, cond.Value); err != nil {
				return nil, err
			}
		}

		// Add filter conditions from Where clauses
		for _, cond := range filterConditions {
			if err := builder.AddFilterCondition("AND", cond.Field, cond.Operator, cond.Value); err != nil {
				return nil, err
			}
		}
	} else {
		// Must use Scan
		compiled.Operation = "Scan"

		// All conditions become filters
		for _, original := range q.conditions {
			normalized, _, _ := q.normalizeCondition(original)
			if err := builder.AddFilterCondition("AND", normalized.Field, normalized.Operator, normalized.Value); err != nil {
				return nil, err
			}
		}
	}

	// Note: Additional filters from Filter/OrFilter calls are already in the builder

	// Add projections
	if len(q.projection) > 0 {
		builder.AddProjection(q.projection...)
	}

	// Build the expressions
	components := builder.Build()
	compiled.KeyConditionExpression = components.KeyConditionExpression
	compiled.FilterExpression = components.FilterExpression
	compiled.ProjectionExpression = components.ProjectionExpression
	compiled.ExpressionAttributeNames = components.ExpressionAttributeNames
	compiled.ExpressionAttributeValues = components.ExpressionAttributeValues

	// Set other parameters
	if q.limit > 0 {
		limit := int32(q.limit)
		compiled.Limit = &limit
	}

	if q.orderBy.Order == "desc" {
		forward := false
		compiled.ScanIndexForward = &forward
	}

	// Handle cursor/exclusive start key
	if len(q.exclusive) > 0 {
		compiled.ExclusiveStartKey = q.exclusive
	}

	// Set consistent read (only for main table, not GSI)
	if q.consistentRead && compiled.IndexName == "" {
		compiled.ConsistentRead = &q.consistentRead
	}

	return compiled, nil
}

// compileScan compiles a scan operation
func (q *Query) compileScan() (*core.CompiledQuery, error) {
	// Use existing builder if available (contains filters from Filter/OrFilter calls)
	// Otherwise create a new one
	var builder *expr.Builder
	if q.builder != nil {
		builder = q.builder
	} else {
		builder = expr.NewBuilder()
	}

	compiled := &core.CompiledQuery{
		TableName: q.metadata.TableName(),
		Operation: "Scan",
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
		limit := int32(q.limit)
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

// OrFilter adds an OR filter condition
func (q *Query) OrFilter(field string, op string, value any) core.Query {
	// Initialize builder if not already done
	if q.builder == nil {
		q.builder = expr.NewBuilder()
	}

	if err := q.builder.AddFilterCondition("OR", field, op, value); err != nil {
		q.recordBuilderError(err)
	}
	return q
}

// FilterGroup adds a grouped AND filter condition
func (q *Query) FilterGroup(fn func(core.Query)) core.Query {
	// Initialize builder if not already done
	if q.builder == nil {
		q.builder = expr.NewBuilder()
	}

	// Create a new sub-query and builder for the group
	subBuilder := expr.NewBuilder()
	subQuery := &Query{
		model:    q.model,
		metadata: q.metadata,
		executor: q.executor,
		ctx:      q.ctx,
		builder:  subBuilder,
	}

	// Execute the user's function to build the sub-query
	fn(subQuery)
	if err := subQuery.checkBuilderError(); err != nil {
		q.recordBuilderError(err)
	}

	// Build the components from the sub-query
	components := subBuilder.Build()

	// Add the built group to the main builder
	q.builder.AddGroupFilter("AND", components)
	return q
}

// OrFilterGroup adds a grouped OR filter condition
func (q *Query) OrFilterGroup(fn func(core.Query)) core.Query {
	// Initialize builder if not already done
	if q.builder == nil {
		q.builder = expr.NewBuilder()
	}

	// Create a new sub-query and builder for the group
	subBuilder := expr.NewBuilder()
	subQuery := &Query{
		model:    q.model,
		metadata: q.metadata,
		executor: q.executor,
		ctx:      q.ctx,
		builder:  subBuilder,
	}

	// Execute the user's function to build the sub-query
	fn(subQuery)
	if err := subQuery.checkBuilderError(); err != nil {
		q.recordBuilderError(err)
	}

	// Build the components from the sub-query
	components := subBuilder.Build()

	// Add the built group to the main builder
	q.builder.AddGroupFilter("OR", components)
	return q
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
func NewWithConditions(model any, metadata core.ModelMetadata, executor QueryExecutor, conditions []Condition, ctx context.Context) *Query {
	return &Query{
		model:                   model,
		metadata:                metadata,
		executor:                executor,
		conditions:              conditions,
		ctx:                     ctx,
		filters:                 make([]Filter, 0),
		builder:                 expr.NewBuilder(),
		writeConditions:         make([]Condition, 0),
		rawConditionExpressions: make([]conditionExpression, 0),
	}
}

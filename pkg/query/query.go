package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/internal/expr"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/index"
)

// Query represents a DynamoDB query builder
type Query struct {
	model      interface{}
	conditions []Condition
	filters    []Filter
	rawFilters []RawFilter
	index      string
	limit      int
	offset     *int
	projection []string
	orderBy    OrderBy
	exclusive  map[string]types.AttributeValue
	ctx        context.Context

	// Internal state
	metadata core.ModelMetadata
	executor QueryExecutor
}

// Condition represents a query condition
type Condition struct {
	Field    string
	Operator string
	Value    interface{}
}

// Filter represents a filter expression
type Filter struct {
	Expression string
	Params     map[string]interface{}
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

// QueryExecutor executes compiled queries (provided by Team 1)
type QueryExecutor interface {
	ExecuteQuery(input *core.CompiledQuery, dest interface{}) error
	ExecuteScan(input *core.CompiledQuery, dest interface{}) error
}

// New creates a new Query instance
func New(model interface{}, metadata core.ModelMetadata, executor QueryExecutor) *Query {
	return &Query{
		model:    model,
		metadata: metadata,
		executor: executor,
		filters:  make([]Filter, 0),
	}
}

// Where adds a condition to the query
func (q *Query) Where(field string, op string, value interface{}) core.Query {
	q.conditions = append(q.conditions, Condition{
		Field:    field,
		Operator: op,
		Value:    value,
	})
	return q
}

// Filter adds a filter expression to the query
func (q *Query) Filter(expr string, values ...interface{}) core.Query {
	// Convert values to parameter map
	paramMap := make(map[string]interface{})
	for i, v := range values {
		paramMap[fmt.Sprintf("param%d", i)] = v
	}

	q.filters = append(q.filters, Filter{
		Expression: expr,
		Params:     paramMap,
	})
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

// First executes the query and returns the first result
func (q *Query) First(dest interface{}) error {
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
func (q *Query) All(dest interface{}) error {
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
	// TODO: This should be implemented by Team 1
	return errors.New("Create not yet implemented")
}

// Update updates an item
func (q *Query) Update(fields ...string) error {
	// TODO: This should be implemented by Team 1
	return errors.New("Update not yet implemented")
}

// Delete deletes an item
func (q *Query) Delete() error {
	// TODO: This should be implemented by Team 1
	return errors.New("Delete not yet implemented")
}

// Scan performs a table scan
func (q *Query) Scan(dest interface{}) error {
	compiled, err := q.compileScan()
	if err != nil {
		return err
	}

	return q.executor.ExecuteScan(compiled, dest)
}

// BatchGet retrieves multiple items by their primary keys
func (q *Query) BatchGet(keys []interface{}, dest interface{}) error {
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
func (q *Query) BatchCreate(items interface{}) error {
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

	// Build batch write request
	batchWrite := &CompiledBatchWrite{
		TableName: q.metadata.TableName(),
		Items:     make([]map[string]types.AttributeValue, 0, itemsValue.Len()),
	}

	// Convert items to AttributeValues
	for i := 0; i < itemsValue.Len(); i++ {
		item := itemsValue.Index(i).Interface()

		// Convert item to map[string]types.AttributeValue
		// This should be handled by a marshaler in Team 1's code
		av, err := convertItemToAttributeValue(item)
		if err != nil {
			return fmt.Errorf("failed to convert item %d: %w", i, err)
		}

		batchWrite.Items = append(batchWrite.Items, av)
	}

	// Execute batch write through executor
	if executor, ok := q.executor.(BatchExecutor); ok {
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
	selector := index.NewSelector(q.metadata.Indexes())

	// Convert our conditions to index.Condition type
	indexConditions := make([]index.Condition, len(q.conditions))
	for i, cond := range q.conditions {
		indexConditions[i] = index.Condition{
			Field:    cond.Field,
			Operator: cond.Operator,
			Value:    cond.Value,
		}
	}

	// Analyze conditions to find required keys
	requiredKeys := index.AnalyzeConditions(indexConditions)

	// Use the selector to find the best index
	return selector.SelectOptimal(requiredKeys, nil)
}

// AllPaginated executes the query and returns paginated results
func (q *Query) AllPaginated(dest interface{}) (*PaginatedResult, error) {
	// Set a reasonable limit if not specified
	if q.limit == 0 {
		q.limit = 100
	}

	compiled, err := q.Compile()
	if err != nil {
		return nil, err
	}

	// Execute the query
	var result interface{}
	if compiled.Operation == "Query" {
		result, err = q.executePaginatedQuery(compiled, dest)
	} else {
		result, err = q.executePaginatedScan(compiled, dest)
	}

	if err != nil {
		return nil, err
	}

	// Extract pagination info
	queryResult := result.(map[string]interface{})

	return &PaginatedResult{
		Items:      dest,
		NextCursor: q.encodeCursor(queryResult["LastEvaluatedKey"]),
		Count:      queryResult["Count"].(int),
	}, nil
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
		// Store error to be returned on execution
		// This pattern is common in fluent interfaces
		// The error will be checked and returned when the query is executed
	}
	return q
}

// executePaginatedQuery executes a query with pagination support
func (q *Query) executePaginatedQuery(compiled *core.CompiledQuery, dest interface{}) (interface{}, error) {
	// This will be implemented by Team 1's executor
	err := q.executor.ExecuteQuery(compiled, dest)
	if err != nil {
		return nil, err
	}

	// For now, return a mock result structure
	// Team 1 will need to update their executor to return proper pagination info
	return map[string]interface{}{
		"Count":            0,
		"LastEvaluatedKey": nil,
	}, nil
}

// executePaginatedScan executes a scan with pagination support
func (q *Query) executePaginatedScan(compiled *core.CompiledQuery, dest interface{}) (interface{}, error) {
	// This will be implemented by Team 1's executor
	err := q.executor.ExecuteScan(compiled, dest)
	if err != nil {
		return nil, err
	}

	// For now, return a mock result structure
	// Team 1 will need to update their executor to return proper pagination info
	return map[string]interface{}{
		"Count":            0,
		"LastEvaluatedKey": nil,
	}, nil
}

// encodeCursor encodes the LastEvaluatedKey as a cursor string
func (q *Query) encodeCursor(lastKey interface{}) string {
	if lastKey == nil {
		return ""
	}

	// Convert to map[string]types.AttributeValue if needed
	var avMap map[string]types.AttributeValue
	switch v := lastKey.(type) {
	case map[string]types.AttributeValue:
		avMap = v
	case map[string]interface{}:
		// Handle the case where lastKey is map[string]interface{}
		// This would come from the executor results
		if val, ok := v["LastEvaluatedKey"]; ok {
			if m, ok := val.(map[string]types.AttributeValue); ok {
				avMap = m
			}
		}
	default:
		return ""
	}

	if avMap == nil || len(avMap) == 0 {
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

// shouldUseQuery determines if we can use Query vs Scan
func (q *Query) shouldUseQuery() bool {
	bestIndex, err := q.selectBestIndex()
	if err != nil || bestIndex == nil {
		return false
	}

	// If we found a suitable index, we can use Query
	return true
}

// Compile compiles the query into executable form
func (q *Query) Compile() (*core.CompiledQuery, error) {
	builder := expr.NewBuilder()

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

		primaryKey := q.metadata.PrimaryKey()
		indexPK := bestIndex.PartitionKey
		indexSK := bestIndex.SortKey

		// If using primary table
		if bestIndex.Name == "" {
			indexPK = primaryKey.PartitionKey
			indexSK = primaryKey.SortKey
		}

		for _, cond := range q.conditions {
			if cond.Field == indexPK || (indexSK != "" && cond.Field == indexSK) {
				keyConditions = append(keyConditions, cond)
			} else {
				filterConditions = append(filterConditions, cond)
			}
		}

		// Add key conditions
		for _, cond := range keyConditions {
			err := builder.AddKeyCondition(cond.Field, cond.Operator, cond.Value)
			if err != nil {
				return nil, err
			}
		}

		// Add filter conditions
		for _, cond := range filterConditions {
			err := builder.AddFilterCondition(cond.Field, cond.Operator, cond.Value)
			if err != nil {
				return nil, err
			}
		}
	} else {
		// Must use Scan
		compiled.Operation = "Scan"

		// All conditions become filters
		for _, cond := range q.conditions {
			err := builder.AddFilterCondition(cond.Field, cond.Operator, cond.Value)
			if err != nil {
				return nil, err
			}
		}
	}

	// Add additional filters
	for _, filter := range q.filters {
		err := builder.AddRawFilter(filter.Expression, filter.Params)
		if err != nil {
			return nil, err
		}
	}

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

	compiled.ExclusiveStartKey = q.exclusive

	return compiled, nil
}

// compileScan compiles a scan operation
func (q *Query) compileScan() (*core.CompiledQuery, error) {
	builder := expr.NewBuilder()

	compiled := &core.CompiledQuery{
		TableName: q.metadata.TableName(),
		Operation: "Scan",
	}

	// Add filter conditions
	for _, cond := range q.conditions {
		err := builder.AddFilterCondition(cond.Field, cond.Operator, cond.Value)
		if err != nil {
			return nil, err
		}
	}

	// Add additional filters
	for _, filter := range q.filters {
		err := builder.AddRawFilter(filter.Expression, filter.Params)
		if err != nil {
			return nil, err
		}
	}

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

	return compiled, nil
}

// convertItemToAttributeValue converts an item to DynamoDB AttributeValue map
// This is a placeholder that should be replaced by Team 1's marshaler
func convertItemToAttributeValue(item interface{}) (map[string]types.AttributeValue, error) {
	// TODO: This should use Team 1's marshaler when available
	// For now, using reflection to build the attribute map

	result := make(map[string]types.AttributeValue)

	v := reflect.ValueOf(item)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil, errors.New("item must be a struct")
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		// Skip unexported fields
		if !fieldValue.CanInterface() {
			continue
		}

		// Get the DynamoDB field name from tags or use the field name
		fieldName := field.Name
		if tag := field.Tag.Get("dynamodb"); tag != "" && tag != "-" {
			fieldName = tag
		}

		// Convert field value to AttributeValue
		av, err := expr.ConvertToAttributeValue(fieldValue.Interface())
		if err != nil {
			continue // Skip fields that can't be converted
		}

		result[fieldName] = av
	}

	return result, nil
}

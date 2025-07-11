package query

import (
	"context"
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/core"
)

// DynamoDBAPI defines the interface for all DynamoDB operations
type DynamoDBAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error)
	BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
}

// MainExecutor is the main executor that implements all executor interfaces
type MainExecutor struct {
	client DynamoDBAPI
	ctx    context.Context
}

// NewExecutor creates a new MainExecutor instance
func NewExecutor(client DynamoDBAPI, ctx context.Context) *MainExecutor {
	return &MainExecutor{
		client: client,
		ctx:    ctx,
	}
}

// ExecuteQuery implements QueryExecutor.ExecuteQuery
func (e *MainExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}

	// Build QueryInput
	queryInput := &dynamodb.QueryInput{
		TableName: &input.TableName,
	}

	// Set index name if specified
	if input.IndexName != "" {
		queryInput.IndexName = &input.IndexName
	}

	// Set key condition expression
	if input.KeyConditionExpression != "" {
		queryInput.KeyConditionExpression = &input.KeyConditionExpression
	}

	// Set filter expression
	if input.FilterExpression != "" {
		queryInput.FilterExpression = &input.FilterExpression
	}

	// Set projection expression
	if input.ProjectionExpression != "" {
		queryInput.ProjectionExpression = &input.ProjectionExpression
	}

	// Set expression attribute names
	if len(input.ExpressionAttributeNames) > 0 {
		queryInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}

	// Set expression attribute values
	if len(input.ExpressionAttributeValues) > 0 {
		queryInput.ExpressionAttributeValues = input.ExpressionAttributeValues
	}

	// Set limit
	if input.Limit != nil {
		queryInput.Limit = input.Limit
	}

	// Set exclusive start key
	if len(input.ExclusiveStartKey) > 0 {
		queryInput.ExclusiveStartKey = input.ExclusiveStartKey
	}

	// Set scan index forward
	if input.ScanIndexForward != nil {
		queryInput.ScanIndexForward = input.ScanIndexForward
	}

	// Set consistent read
	if input.ConsistentRead != nil {
		queryInput.ConsistentRead = input.ConsistentRead
	}

	// Execute the query
	var allItems []map[string]types.AttributeValue
	var lastEvaluatedKey map[string]types.AttributeValue

	for {
		if lastEvaluatedKey != nil {
			queryInput.ExclusiveStartKey = lastEvaluatedKey
		}

		output, err := e.client.Query(e.ctx, queryInput)
		if err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}

		allItems = append(allItems, output.Items...)

		// Check if we need to paginate
		if output.LastEvaluatedKey == nil || (input.Limit != nil && int32(len(allItems)) >= *input.Limit) {
			break
		}

		lastEvaluatedKey = output.LastEvaluatedKey
	}

	// Unmarshal the results into dest
	return UnmarshalItems(allItems, dest)
}

// ExecuteScan implements QueryExecutor.ExecuteScan
func (e *MainExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}

	// Build ScanInput
	scanInput := &dynamodb.ScanInput{
		TableName: &input.TableName,
	}

	// Set index name if specified
	if input.IndexName != "" {
		scanInput.IndexName = &input.IndexName
	}

	// Set filter expression
	if input.FilterExpression != "" {
		scanInput.FilterExpression = &input.FilterExpression
	}

	// Set projection expression
	if input.ProjectionExpression != "" {
		scanInput.ProjectionExpression = &input.ProjectionExpression
	}

	// Set expression attribute names
	if len(input.ExpressionAttributeNames) > 0 {
		scanInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}

	// Set expression attribute values
	if len(input.ExpressionAttributeValues) > 0 {
		scanInput.ExpressionAttributeValues = input.ExpressionAttributeValues
	}

	// Set limit
	if input.Limit != nil {
		scanInput.Limit = input.Limit
	}

	// Set exclusive start key
	if len(input.ExclusiveStartKey) > 0 {
		scanInput.ExclusiveStartKey = input.ExclusiveStartKey
	}

	// Set segment and total segments for parallel scan
	if input.Segment != nil {
		scanInput.Segment = input.Segment
	}
	if input.TotalSegments != nil {
		scanInput.TotalSegments = input.TotalSegments
	}

	// Set consistent read
	if input.ConsistentRead != nil {
		scanInput.ConsistentRead = input.ConsistentRead
	}

	// Execute the scan
	var allItems []map[string]types.AttributeValue
	var lastEvaluatedKey map[string]types.AttributeValue

	for {
		if lastEvaluatedKey != nil {
			scanInput.ExclusiveStartKey = lastEvaluatedKey
		}

		output, err := e.client.Scan(e.ctx, scanInput)
		if err != nil {
			return fmt.Errorf("failed to execute scan: %w", err)
		}

		allItems = append(allItems, output.Items...)

		// Check if we need to paginate
		if output.LastEvaluatedKey == nil || (input.Limit != nil && int32(len(allItems)) >= *input.Limit) {
			break
		}

		lastEvaluatedKey = output.LastEvaluatedKey
	}

	// Unmarshal the results into dest
	return UnmarshalItems(allItems, dest)
}

// ExecutePutItem implements PutItemExecutor.ExecutePutItem
func (e *MainExecutor) ExecutePutItem(input *core.CompiledQuery, item map[string]types.AttributeValue) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}

	if len(item) == 0 {
		return fmt.Errorf("item cannot be empty")
	}

	// Build PutItem input
	putInput := &dynamodb.PutItemInput{
		TableName: &input.TableName,
		Item:      item,
	}

	// Set condition expression if present
	if input.ConditionExpression != "" {
		putInput.ConditionExpression = &input.ConditionExpression
	}

	// Set expression attribute names
	if len(input.ExpressionAttributeNames) > 0 {
		putInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}

	// Set expression attribute values
	if len(input.ExpressionAttributeValues) > 0 {
		putInput.ExpressionAttributeValues = input.ExpressionAttributeValues
	}

	// Execute the put
	_, err := e.client.PutItem(e.ctx, putInput)
	if err != nil {
		return fmt.Errorf("failed to put item: %w", err)
	}

	return nil
}

// ExecuteUpdateItem implements UpdateItemExecutor.ExecuteUpdateItem
func (e *MainExecutor) ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	// Use the UpdateExecutor from core package
	updateExecutor := core.NewUpdateExecutor(e.client, e.ctx)
	return updateExecutor.ExecuteUpdateItem(input, key)
}

// ExecuteUpdateItemWithResult implements UpdateItemWithResultExecutor.ExecuteUpdateItemWithResult
func (e *MainExecutor) ExecuteUpdateItemWithResult(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error) {
	// Use the UpdateExecutor from core package
	updateExecutor := core.NewUpdateExecutor(e.client, e.ctx)
	return updateExecutor.ExecuteUpdateItemWithResult(input, key)
}

// ExecuteDeleteItem implements DeleteItemExecutor.ExecuteDeleteItem
func (e *MainExecutor) ExecuteDeleteItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}

	if len(key) == 0 {
		return fmt.Errorf("key cannot be empty")
	}

	// Build DeleteItem input
	deleteInput := &dynamodb.DeleteItemInput{
		TableName: &input.TableName,
		Key:       key,
	}

	// Set condition expression if present
	if input.ConditionExpression != "" {
		deleteInput.ConditionExpression = &input.ConditionExpression
	}

	// Set expression attribute names
	if len(input.ExpressionAttributeNames) > 0 {
		deleteInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}

	// Set expression attribute values
	if len(input.ExpressionAttributeValues) > 0 {
		deleteInput.ExpressionAttributeValues = input.ExpressionAttributeValues
	}

	// Execute the delete
	_, err := e.client.DeleteItem(e.ctx, deleteInput)
	if err != nil {
		return fmt.Errorf("failed to delete item: %w", err)
	}

	return nil
}

// ExecuteQueryWithPagination implements PaginatedQueryExecutor.ExecuteQueryWithPagination
func (e *MainExecutor) ExecuteQueryWithPagination(input *core.CompiledQuery, dest any) (*QueryResult, error) {
	if input == nil {
		return nil, fmt.Errorf("compiled query cannot be nil")
	}

	// Build QueryInput
	queryInput := &dynamodb.QueryInput{
		TableName: &input.TableName,
	}

	// Set index name if specified
	if input.IndexName != "" {
		queryInput.IndexName = &input.IndexName
	}

	// Set key condition expression
	if input.KeyConditionExpression != "" {
		queryInput.KeyConditionExpression = &input.KeyConditionExpression
	}

	// Set filter expression
	if input.FilterExpression != "" {
		queryInput.FilterExpression = &input.FilterExpression
	}

	// Set projection expression
	if input.ProjectionExpression != "" {
		queryInput.ProjectionExpression = &input.ProjectionExpression
	}

	// Set expression attribute names
	if len(input.ExpressionAttributeNames) > 0 {
		queryInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}

	// Set expression attribute values
	if len(input.ExpressionAttributeValues) > 0 {
		queryInput.ExpressionAttributeValues = input.ExpressionAttributeValues
	}

	// Set limit
	if input.Limit != nil {
		queryInput.Limit = input.Limit
	}

	// Set exclusive start key
	if len(input.ExclusiveStartKey) > 0 {
		queryInput.ExclusiveStartKey = input.ExclusiveStartKey
	}

	// Set scan index forward
	if input.ScanIndexForward != nil {
		queryInput.ScanIndexForward = input.ScanIndexForward
	}

	// Set consistent read
	if input.ConsistentRead != nil {
		queryInput.ConsistentRead = input.ConsistentRead
	}

	// Execute the query (single page only for pagination)
	output, err := e.client.Query(e.ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	// Unmarshal the results into dest
	if err := UnmarshalItems(output.Items, dest); err != nil {
		return nil, err
	}

	// Return the result with pagination info
	return &QueryResult{
		Items:            output.Items,
		Count:            int64(len(output.Items)),
		ScannedCount:     int64(output.ScannedCount),
		LastEvaluatedKey: output.LastEvaluatedKey,
	}, nil
}

// ExecuteScanWithPagination implements PaginatedQueryExecutor.ExecuteScanWithPagination
func (e *MainExecutor) ExecuteScanWithPagination(input *core.CompiledQuery, dest any) (*ScanResult, error) {
	if input == nil {
		return nil, fmt.Errorf("compiled query cannot be nil")
	}

	// Build ScanInput
	scanInput := &dynamodb.ScanInput{
		TableName: &input.TableName,
	}

	// Set index name if specified
	if input.IndexName != "" {
		scanInput.IndexName = &input.IndexName
	}

	// Set filter expression
	if input.FilterExpression != "" {
		scanInput.FilterExpression = &input.FilterExpression
	}

	// Set projection expression
	if input.ProjectionExpression != "" {
		scanInput.ProjectionExpression = &input.ProjectionExpression
	}

	// Set expression attribute names
	if len(input.ExpressionAttributeNames) > 0 {
		scanInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}

	// Set expression attribute values
	if len(input.ExpressionAttributeValues) > 0 {
		scanInput.ExpressionAttributeValues = input.ExpressionAttributeValues
	}

	// Set limit
	if input.Limit != nil {
		scanInput.Limit = input.Limit
	}

	// Set exclusive start key
	if len(input.ExclusiveStartKey) > 0 {
		scanInput.ExclusiveStartKey = input.ExclusiveStartKey
	}

	// Set segment and total segments for parallel scan
	if input.Segment != nil {
		scanInput.Segment = input.Segment
	}
	if input.TotalSegments != nil {
		scanInput.TotalSegments = input.TotalSegments
	}

	// Set consistent read
	if input.ConsistentRead != nil {
		scanInput.ConsistentRead = input.ConsistentRead
	}

	// Execute the scan (single page only for pagination)
	output, err := e.client.Scan(e.ctx, scanInput)
	if err != nil {
		return nil, fmt.Errorf("failed to execute scan: %w", err)
	}

	// Unmarshal the results into dest
	if err := UnmarshalItems(output.Items, dest); err != nil {
		return nil, err
	}

	// Return the result with pagination info
	return &ScanResult{
		Items:            output.Items,
		Count:            int64(len(output.Items)),
		ScannedCount:     int64(output.ScannedCount),
		LastEvaluatedKey: output.LastEvaluatedKey,
	}, nil
}

// ExecuteBatchGet implements BatchExecutor.ExecuteBatchGet
func (e *MainExecutor) ExecuteBatchGet(input *CompiledBatchGet, dest any) error {
	if input == nil {
		return fmt.Errorf("compiled batch get cannot be nil")
	}

	if len(input.Keys) == 0 {
		return nil // No keys to fetch
	}

	// Build KeysAndAttributes
	keysAndAttributes := &types.KeysAndAttributes{
		Keys: input.Keys,
	}

	// Set projection expression if specified
	if input.ProjectionExpression != "" {
		keysAndAttributes.ProjectionExpression = &input.ProjectionExpression
	}

	// Set expression attribute names
	if len(input.ExpressionAttributeNames) > 0 {
		keysAndAttributes.ExpressionAttributeNames = input.ExpressionAttributeNames
	}

	// Set consistent read
	if input.ConsistentRead {
		keysAndAttributes.ConsistentRead = &input.ConsistentRead
	}

	// Build BatchGetItem input
	batchGetInput := &dynamodb.BatchGetItemInput{
		RequestItems: map[string]types.KeysAndAttributes{
			input.TableName: *keysAndAttributes,
		},
	}

	// Execute batch get with retry for unprocessed items
	var allItems []map[string]types.AttributeValue

	for {
		output, err := e.client.BatchGetItem(e.ctx, batchGetInput)
		if err != nil {
			return fmt.Errorf("failed to batch get items: %w", err)
		}

		// Collect items from the response
		if items, exists := output.Responses[input.TableName]; exists {
			allItems = append(allItems, items...)
		}

		// Check for unprocessed keys
		if len(output.UnprocessedKeys) == 0 {
			break
		}

		// Retry unprocessed keys
		batchGetInput.RequestItems = output.UnprocessedKeys
	}

	// Unmarshal the results
	return UnmarshalItems(allItems, dest)
}

// ExecuteBatchWrite implements BatchExecutor.ExecuteBatchWrite
func (e *MainExecutor) ExecuteBatchWrite(input *CompiledBatchWrite) error {
	if input == nil {
		return fmt.Errorf("compiled batch write cannot be nil")
	}

	if len(input.Items) == 0 {
		return nil // No items to write
	}

	// Process items in batches of 25 (DynamoDB limit)
	const batchSize = 25

	for i := 0; i < len(input.Items); i += batchSize {
		end := i + batchSize
		if end > len(input.Items) {
			end = len(input.Items)
		}

		// Build write requests for this batch
		writeRequests := make([]types.WriteRequest, 0, end-i)
		for j := i; j < end; j++ {
			writeRequests = append(writeRequests, types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: input.Items[j],
				},
			})
		}

		// Build BatchWriteItem input
		batchWriteInput := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				input.TableName: writeRequests,
			},
		}

		// Execute batch write with retry for unprocessed items
		for {
			output, err := e.client.BatchWriteItem(e.ctx, batchWriteInput)
			if err != nil {
				return fmt.Errorf("failed to batch write items: %w", err)
			}

			// Check for unprocessed items
			if len(output.UnprocessedItems) == 0 {
				break
			}

			// Retry unprocessed items
			batchWriteInput.RequestItems = output.UnprocessedItems
		}
	}

	return nil
}

// UnmarshalItems unmarshals DynamoDB items into the destination.
// This function is exported for use with DynamoDB streams and other external data sources.
func UnmarshalItems(items []map[string]types.AttributeValue, dest any) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr {
		return fmt.Errorf("destination must be a pointer")
	}

	destElem := destValue.Elem()

	// Handle single item result
	if destElem.Kind() != reflect.Slice {
		if len(items) == 0 {
			return fmt.Errorf("no items found")
		}
		// For single item, unmarshal the first item
		return UnmarshalItem(items[0], dest)
	}

	// Handle slice result
	sliceType := destElem.Type()
	itemType := sliceType.Elem()

	// Create a new slice with the appropriate capacity
	newSlice := reflect.MakeSlice(sliceType, 0, len(items))

	for _, item := range items {
		// Create a new instance of the item type
		newItem := reflect.New(itemType)
		if itemType.Kind() == reflect.Ptr {
			newItem = reflect.New(itemType.Elem())
		}

		// Unmarshal the item
		if err := UnmarshalItem(item, newItem.Interface()); err != nil {
			return fmt.Errorf("failed to unmarshal item: %w", err)
		}

		// Append to slice
		if itemType.Kind() == reflect.Ptr {
			newSlice = reflect.Append(newSlice, newItem)
		} else {
			newSlice = reflect.Append(newSlice, newItem.Elem())
		}
	}

	// Set the result
	destElem.Set(newSlice)
	return nil
}

// UnmarshalItem unmarshals a single DynamoDB item into a Go struct.
// This function respects both "dynamodb" and "dynamorm" struct tags.
func UnmarshalItem(item map[string]types.AttributeValue, dest any) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr {
		return fmt.Errorf("destination must be a pointer")
	}

	destElem := destValue.Elem()
	destType := destElem.Type()

	// For each field in the struct
	for i := 0; i < destType.NumField(); i++ {
		field := destType.Field(i)
		fieldValue := destElem.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get the dynamodb tag
		tag := field.Tag.Get("dynamodb")
		if tag == "" {
			tag = field.Tag.Get("dynamorm")
		}
		if tag == "" || tag == "-" {
			continue
		}

		// Use tag as the attribute name
		attrName := tag
		if attrName == "" {
			attrName = field.Name
		}

		// Get the attribute value
		if av, exists := item[attrName]; exists {
			if err := unmarshalAttributeValue(av, fieldValue); err != nil {
				return fmt.Errorf("failed to unmarshal field %s: %w", field.Name, err)
			}
		}
	}

	return nil
}

// unmarshalAttributeValue unmarshals a DynamoDB attribute value into a reflect.Value
func unmarshalAttributeValue(av types.AttributeValue, dest reflect.Value) error {
	if !dest.CanSet() {
		return fmt.Errorf("cannot set value")
	}

	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		dest.SetString(v.Value)
	case *types.AttributeValueMemberN:
		// Handle numeric types
		switch dest.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			var n int64
			_, err := fmt.Sscanf(v.Value, "%d", &n)
			if err != nil {
				return err
			}
			dest.SetInt(n)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			var n uint64
			_, err := fmt.Sscanf(v.Value, "%d", &n)
			if err != nil {
				return err
			}
			dest.SetUint(n)
		case reflect.Float32, reflect.Float64:
			var f float64
			_, err := fmt.Sscanf(v.Value, "%f", &f)
			if err != nil {
				return err
			}
			dest.SetFloat(f)
		}
	case *types.AttributeValueMemberBOOL:
		dest.SetBool(v.Value)
	case *types.AttributeValueMemberNULL:
		// Set to zero value
		dest.Set(reflect.Zero(dest.Type()))
	case *types.AttributeValueMemberL:
		// Handle list
		if dest.Kind() != reflect.Slice {
			return fmt.Errorf("cannot unmarshal list into non-slice type")
		}
		sliceType := dest.Type()
		newSlice := reflect.MakeSlice(sliceType, len(v.Value), len(v.Value))
		for i, item := range v.Value {
			if err := unmarshalAttributeValue(item, newSlice.Index(i)); err != nil {
				return err
			}
		}
		dest.Set(newSlice)
	case *types.AttributeValueMemberM:
		// Handle map
		if dest.Kind() == reflect.Map {
			mapType := dest.Type()
			keyType := mapType.Key()
			elemType := mapType.Elem()
			newMap := reflect.MakeMap(mapType)
			for k, v := range v.Value {
				keyValue := reflect.New(keyType).Elem()
				keyValue.SetString(k)
				elemValue := reflect.New(elemType).Elem()
				if err := unmarshalAttributeValue(v, elemValue); err != nil {
					return err
				}
				newMap.SetMapIndex(keyValue, elemValue)
			}
			dest.Set(newMap)
		} else if dest.Kind() == reflect.Struct {
			// Unmarshal into struct
			for k, v := range v.Value {
				// Find field by name
				field := dest.FieldByName(k)
				if field.IsValid() && field.CanSet() {
					if err := unmarshalAttributeValue(v, field); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// Verify that MainExecutor implements all required interfaces
var (
	_ QueryExecutor                = (*MainExecutor)(nil)
	_ PutItemExecutor              = (*MainExecutor)(nil)
	_ UpdateItemExecutor           = (*MainExecutor)(nil)
	_ UpdateItemWithResultExecutor = (*MainExecutor)(nil)
	_ DeleteItemExecutor           = (*MainExecutor)(nil)
	_ PaginatedQueryExecutor       = (*MainExecutor)(nil)
	_ BatchExecutor                = (*MainExecutor)(nil)
)

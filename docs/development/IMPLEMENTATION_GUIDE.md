# DynamORM Implementation Guide

Quick reference for implementing the remaining features in DynamORM.

## 1. UpdateItemExecutor Implementation

### Location: `pkg/core/executor_update.go`

```go
package core

import (
    "context"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Ensure DefaultExecutor implements UpdateItemExecutor
var _ query.UpdateItemExecutor = (*DefaultExecutor)(nil)

func (e *DefaultExecutor) ExecuteUpdateItem(input *CompiledQuery, key map[string]types.AttributeValue) error {
    updateInput := &dynamodb.UpdateItemInput{
        TableName:                 &input.TableName,
        Key:                      key,
        UpdateExpression:         input.UpdateExpression,
        ConditionExpression:      input.ConditionExpression,
        ExpressionAttributeNames: input.ExpressionAttributeNames,
        ExpressionAttributeValues: input.ExpressionAttributeValues,
        ReturnValues:             types.ReturnValueNone, // Make configurable
    }
    
    _, err := e.client.UpdateItem(context.TODO(), updateInput)
    return err
}
```

## 2. UpdateBuilder Integration Fix

### Location: `dynamorm.go:1799`

```go
// UpdateBuilder returns a builder for complex update operations
func (q *query) UpdateBuilder() core.UpdateBuilder {
    // Create a new query instance from the query package
    queryPkg := queryPackage.New(q.db.session.Client(), q.db.registry, q.db.converter)
    
    // Copy query state
    if q.model != nil {
        queryPkg = queryPkg.Model(q.model)
    }
    
    // Copy conditions
    for _, cond := range q.conditions {
        queryPkg = queryPkg.Where(cond.field, cond.op, cond.value)
    }
    
    return queryPkg.UpdateBuilder()
}
```

## 3. Batch Delete Implementation

### Location: `pkg/query/batch_operations.go:319`

```go
// executeDeleteBatch executes a single delete batch
func (q *Query) executeDeleteBatch(batch []any, opts *BatchUpdateOptions) error {
    writeRequests := make([]types.WriteRequest, 0, len(batch))
    
    for _, key := range batch {
        keyAV, err := q.extractKeyAttributeValues(key)
        if err != nil {
            return fmt.Errorf("failed to extract key: %w", err)
        }
        
        writeRequests = append(writeRequests, types.WriteRequest{
            DeleteRequest: &types.DeleteRequest{
                Key: keyAV,
            },
        })
    }
    
    // Execute batch write with retry
    return q.executeWithRetry(func() error {
        input := &dynamodb.BatchWriteItemInput{
            RequestItems: map[string][]types.WriteRequest{
                q.metadata.TableName(): writeRequests,
            },
        }
        
        output, err := q.executor.Client().BatchWriteItem(q.ctx, input)
        if err != nil {
            return err
        }
        
        // Handle unprocessed items
        if len(output.UnprocessedItems) > 0 {
            // Retry unprocessed items
            // This should be handled in the retry logic
            return fmt.Errorf("unprocessed items: %d", len(output.UnprocessedItems))
        }
        
        return nil
    }, opts.RetryPolicy)
}
```

## 4. GroupBy Implementation Skeleton

### Location: `pkg/query/aggregates.go`

```go
// GroupBy groups results by specified fields
func (q *Query) GroupBy(fields ...string) *AggregateQuery {
    return &AggregateQuery{
        query:      q,
        groupByFields: fields,
        aggregations: make(map[string]aggregation),
    }
}

// AggregateQuery represents a query with aggregations
type AggregateQuery struct {
    query         *Query
    groupByFields []string
    aggregations  map[string]aggregation
    havingClauses []havingClause
}

type aggregation struct {
    function string // COUNT, SUM, AVG, MIN, MAX
    field    string
    alias    string
}

// Count adds a COUNT aggregation
func (aq *AggregateQuery) Count(field, alias string) *AggregateQuery {
    aq.aggregations[alias] = aggregation{
        function: "COUNT",
        field:    field,
        alias:    alias,
    }
    return aq
}

// Execute runs the aggregation query
func (aq *AggregateQuery) Execute() ([]map[string]any, error) {
    // 1. Execute the base query
    // 2. Group results in memory by groupByFields
    // 3. Apply aggregation functions
    // 4. Apply having clauses
    // 5. Return results
    return nil, fmt.Errorf("not implemented")
}
```

## 5. Cursor Implementation

### Location: `pkg/query/cursor.go`

```go
package query

import (
    "encoding/base64"
    "encoding/json"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// CursorData represents pagination cursor data
type CursorData struct {
    LastEvaluatedKey map[string]types.AttributeValue `json:"-"`
    EncodedKey       map[string]any                  `json:"key"`
    IndexName        string                          `json:"index,omitempty"`
    Direction        string                          `json:"dir,omitempty"`
}

// EncodeCursor creates a cursor string from LastEvaluatedKey
func EncodeCursor(lastKey map[string]types.AttributeValue, indexName, direction string) (string, error) {
    if len(lastKey) == 0 {
        return "", nil
    }
    
    // Convert AttributeValues to simple map
    encodedKey := make(map[string]any)
    for k, v := range lastKey {
        // Convert AttributeValue to simple type
        encodedKey[k] = attributeValueToSimple(v)
    }
    
    cursor := CursorData{
        EncodedKey: encodedKey,
        IndexName:  indexName,
        Direction:  direction,
    }
    
    data, err := json.Marshal(cursor)
    if err != nil {
        return "", err
    }
    
    return base64.URLEncoding.EncodeToString(data), nil
}

// DecodeCursor decodes a cursor string to LastEvaluatedKey
func DecodeCursor(cursor string) (*CursorData, error) {
    if cursor == "" {
        return nil, nil
    }
    
    data, err := base64.URLEncoding.DecodeString(cursor)
    if err != nil {
        return nil, err
    }
    
    var cursorData CursorData
    if err := json.Unmarshal(data, &cursorData); err != nil {
        return nil, err
    }
    
    // Convert simple map back to AttributeValues
    lastKey := make(map[string]types.AttributeValue)
    for k, v := range cursorData.EncodedKey {
        lastKey[k] = simpleToAttributeValue(v)
    }
    cursorData.LastEvaluatedKey = lastKey
    
    return &cursorData, nil
}
```

## 6. Transform Function Example

### Location: `pkg/schema/transform.go`

```go
// TransformFunc defines a function that transforms data during migration
type TransformFunc func(oldItem map[string]types.AttributeValue) (map[string]types.AttributeValue, error)

// Example transform function
func ExampleTransform(old map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
    new := make(map[string]types.AttributeValue)
    
    // Copy all fields
    for k, v := range old {
        new[k] = v
    }
    
    // Transform specific fields
    if nameVal, ok := old["Name"]; ok {
        if s, ok := nameVal.(*types.AttributeValueMemberS); ok {
            // Convert name to uppercase
            new["Name"] = &types.AttributeValueMemberS{
                Value: strings.ToUpper(s.Value),
            }
        }
    }
    
    // Add new fields
    new["MigratedAt"] = &types.AttributeValueMemberS{
        Value: time.Now().Format(time.RFC3339),
    }
    
    return new, nil
}
```

## Common Patterns

### Error Handling
```go
if err != nil {
    // Check for specific DynamoDB errors
    var cfe *types.ConditionalCheckFailedException
    if errors.As(err, &cfe) {
        return customerrors.ErrConditionFailed
    }
    return fmt.Errorf("operation failed: %w", err)
}
```

### Testing Mock
```go
type mockExecutor struct {
    mock.Mock
}

func (m *mockExecutor) ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
    args := m.Called(input, key)
    return args.Error(0)
}
```

### Integration with Existing Code
- Always check existing patterns in the codebase
- Use the same error handling approach
- Follow naming conventions
- Add comprehensive tests for each feature

## Testing Checklist

For each implementation:
- [ ] Unit tests with mocks
- [ ] Integration tests with DynamoDB Local
- [ ] Error case coverage
- [ ] Performance benchmarks (for batch operations)
- [ ] Documentation updates 
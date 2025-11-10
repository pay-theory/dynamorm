package query_test

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/model"
	"github.com/pay-theory/dynamorm/pkg/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Mock types for testing
type mockMetadata struct {
	mock.Mock
}

type attrAwareMetadata struct{}

func (m *mockMetadata) TableName() string {
	return "test-table"
}

func (m *mockMetadata) PrimaryKey() core.KeySchema {
	return core.KeySchema{
		PartitionKey: "id",
		SortKey:      "timestamp",
	}
}

func (m *mockMetadata) Indexes() []core.IndexSchema {
	return []core.IndexSchema{
		{
			Name:           "status-index",
			Type:           "GSI",
			PartitionKey:   "status",
			SortKey:        "timestamp",
			ProjectionType: "ALL",
		},
		{
			Name:           "user-index",
			Type:           "GSI",
			PartitionKey:   "userId",
			SortKey:        "createdAt",
			ProjectionType: "KEYS_ONLY",
		},
	}
}

func (m *mockMetadata) AttributeMetadata(field string) *core.AttributeMetadata {
	return &core.AttributeMetadata{
		Name:         field,
		Type:         "S",
		DynamoDBName: field,
	}
}

func (m *attrAwareMetadata) TableName() string {
	return "hashtag-table"
}

func (m *attrAwareMetadata) PrimaryKey() core.KeySchema {
	return core.KeySchema{
		PartitionKey: "PK",
		SortKey:      "SK",
	}
}

func (m *attrAwareMetadata) Indexes() []core.IndexSchema {
	return nil
}

func (m *attrAwareMetadata) AttributeMetadata(field string) *core.AttributeMetadata {
	mapping := map[string]*core.AttributeMetadata{
		"PK": {Name: "PK", Type: "S", DynamoDBName: "PK"},
		"SK": {Name: "SK", Type: "S", DynamoDBName: "SK"},
	}
	return mapping[field]
}

type mockExecutor struct {
	mock.Mock
}

type recordingExecutor struct {
	lastCompiled *core.CompiledQuery
	lastItem     map[string]types.AttributeValue
	lastKey      map[string]types.AttributeValue
}

func (r *recordingExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }
func (r *recordingExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return nil }

func (r *recordingExecutor) ExecutePutItem(input *core.CompiledQuery, item map[string]types.AttributeValue) error {
	r.lastCompiled = input
	r.lastItem = item
	return nil
}

func (r *recordingExecutor) ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	r.lastCompiled = input
	r.lastKey = key
	return nil
}

func (r *recordingExecutor) ExecuteDeleteItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	r.lastCompiled = input
	r.lastKey = key
	return nil
}

type registryMetadataAdapter struct {
	meta *model.Metadata
}

func (r *registryMetadataAdapter) TableName() string {
	return r.meta.TableName
}

func (r *registryMetadataAdapter) PrimaryKey() core.KeySchema {
	return core.KeySchema{
		PartitionKey: r.meta.PrimaryKey.PartitionKey.Name,
		SortKey:      r.meta.PrimaryKey.SortKey.Name,
	}
}

func (r *registryMetadataAdapter) Indexes() []core.IndexSchema {
	indexes := make([]core.IndexSchema, len(r.meta.Indexes))
	for i, idx := range r.meta.Indexes {
		indexes[i] = core.IndexSchema{
			Name:            idx.Name,
			Type:            string(idx.Type),
			PartitionKey:    idx.PartitionKey.Name,
			SortKey:         idx.SortKey.Name,
			ProjectionType:  idx.ProjectionType,
			ProjectedFields: idx.ProjectedFields,
		}
	}
	return indexes
}

func (r *registryMetadataAdapter) AttributeMetadata(field string) *core.AttributeMetadata {
	if meta, ok := r.meta.Fields[field]; ok {
		return &core.AttributeMetadata{Name: meta.Name, Type: meta.Type.String(), DynamoDBName: meta.DBName}
	}
	if meta, ok := r.meta.FieldsByDBName[field]; ok {
		return &core.AttributeMetadata{Name: meta.Name, Type: meta.Type.String(), DynamoDBName: meta.DBName}
	}
	return nil
}

func (m *mockExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	args := m.Called(input, dest)
	return args.Error(0)
}

func (m *mockExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	args := m.Called(input, dest)
	return args.Error(0)
}

// Test model
type TestItem struct {
	ID        string `dynamodb:"id"`
	Timestamp int64  `dynamodb:"timestamp"`
	Status    string `dynamodb:"status"`
	UserID    string `dynamodb:"userId"`
	CreatedAt int64  `dynamodb:"createdAt"`
	Data      string `dynamodb:"data"`
}

func TestQuery_BasicQuery(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Test basic query with partition key
	q.Where("id", "=", "test-123")

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.Equal(t, "Query", compiled.Operation)
	assert.Equal(t, "test-table", compiled.TableName)
	assert.NotEmpty(t, compiled.KeyConditionExpression)
	assert.Contains(t, compiled.KeyConditionExpression, "#n1 = :v1")
}

func TestQuery_QueryWithSortKey(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Query with partition and sort key
	q.Where("id", "=", "test-123").
		Where("timestamp", ">", 1000)

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.Equal(t, "Query", compiled.Operation)
	assert.Contains(t, compiled.KeyConditionExpression, "AND")
}

func TestQuery_IndexSelection(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Query that should use status-index
	q.Where("status", "=", "active").
		Where("timestamp", ">", 1000)

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.Equal(t, "Query", compiled.Operation)
	assert.Equal(t, "status-index", compiled.IndexName)
}

func TestQuery_ScanFallback(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Query without partition key should fall back to scan
	q.Where("data", "=", "some-value")

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.Equal(t, "Scan", compiled.Operation)
	assert.NotEmpty(t, compiled.FilterExpression)
}

func TestQuery_ComplexFilters(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Complex query with multiple conditions
	q.Where("id", "=", "test-123").
		Where("timestamp", "BETWEEN", []any{1000, 2000}).
		Filter("status", "IN", []string{"active", "pending"})

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.NotEmpty(t, compiled.FilterExpression)
	assert.Contains(t, compiled.FilterExpression, "IN")
}

func TestQuery_Projection(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Query with projection
	q.Where("id", "=", "test-123").
		Select("id", "status", "data")

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.NotEmpty(t, compiled.ProjectionExpression)
	assert.Contains(t, compiled.ProjectionExpression, "#n")
}

func TestQuery_WriteConditions(t *testing.T) {
	metadata := &mockMetadata{}

	t.Run("create without helpers has no default condition", func(t *testing.T) {
		exec := &recordingExecutor{}
		item := &TestItem{ID: "user#122"}
		q := query.New(item, metadata, exec)

		err := q.Create()
		require.NoError(t, err)
		require.NotNil(t, exec.lastCompiled)
		assert.Equal(t, "", exec.lastCompiled.ConditionExpression)
	})

	t.Run("create ignores where clauses", func(t *testing.T) {
		exec := &recordingExecutor{}
		item := &TestItem{ID: "user#122"}
		q := query.New(item, metadata, exec)

		err := q.Where("id", "=", "user#122").
			Where("status", "=", "pending").
			Create()
		require.NoError(t, err)
		require.NotNil(t, exec.lastCompiled)
		assert.Equal(t, "", exec.lastCompiled.ConditionExpression)
	})

	t.Run("create with helpers", func(t *testing.T) {
		exec := &recordingExecutor{}
		item := &TestItem{ID: "user#123", Status: "pending"}
		q := query.New(item, metadata, exec)

		err := q.IfNotExists().WithCondition("status", "=", "active").Create()
		require.NoError(t, err)
		require.NotNil(t, exec.lastCompiled)
		assert.Contains(t, exec.lastCompiled.ConditionExpression, "attribute_not_exists")

		nameFound := false
		for _, name := range exec.lastCompiled.ExpressionAttributeNames {
			if name == "status" {
				nameFound = true
				break
			}
		}
		assert.True(t, nameFound)

		valueFound := false
		for _, val := range exec.lastCompiled.ExpressionAttributeValues {
			if member, ok := val.(*types.AttributeValueMemberS); ok && member.Value == "active" {
				valueFound = true
				break
			}
		}
		assert.True(t, valueFound)
	})

	t.Run("create with raw expression", func(t *testing.T) {
		exec := &recordingExecutor{}
		item := &TestItem{ID: "user#124"}
		q := query.New(item, metadata, exec)

		err := q.WithConditionExpression("attribute_exists(id) AND Status <> :inactive", map[string]any{
			":inactive": "inactive",
		}).Create()
		require.NoError(t, err)
		require.NotNil(t, exec.lastCompiled)
		assert.Equal(t, "attribute_exists(id) AND Status <> :inactive", exec.lastCompiled.ConditionExpression)

		val, ok := exec.lastCompiled.ExpressionAttributeValues[":inactive"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		assert.Equal(t, "inactive", val.Value)
	})

	t.Run("raw expression maintains grouping", func(t *testing.T) {
		exec := &recordingExecutor{}
		item := &TestItem{ID: "user#125"}
		q := query.New(item, metadata, exec)

		err := q.IfNotExists().
			WithConditionExpression("attribute_exists(#pk) OR attribute_exists(GSI)", map[string]any{
				":dummy": "value",
			}).
			Create()
		require.NoError(t, err)
		require.NotNil(t, exec.lastCompiled)
		assert.Contains(t, exec.lastCompiled.ConditionExpression, ") AND (")
	})

	t.Run("update with conditions", func(t *testing.T) {
		exec := &recordingExecutor{}
		item := &TestItem{ID: "user#200", Status: "active", Data: "initial"}
		q := query.New(item, metadata, exec)

		err := q.Where("id", "=", item.ID).
			Where("timestamp", "=", int64(1)).
			WithCondition("status", "=", "active").
			Update("Data")
		require.NoError(t, err)
		require.NotNil(t, exec.lastCompiled)
		assert.Contains(t, exec.lastCompiled.ConditionExpression, "#")
		foundStatusName := false
		for placeholder, name := range exec.lastCompiled.ExpressionAttributeNames {
			if strings.EqualFold(name, "status") {
				assert.Contains(t, exec.lastCompiled.ConditionExpression, placeholder)
				foundStatusName = true
			}
		}
		assert.True(t, foundStatusName, "status field should be referenced in expression attribute names")
	})

	t.Run("delete with if exists", func(t *testing.T) {
		exec := &recordingExecutor{}
		item := &TestItem{ID: "user#201", Timestamp: 42}
		q := query.New(item, metadata, exec)

		err := q.Where("id", "=", item.ID).
			Where("timestamp", "=", item.Timestamp).
			IfExists().
			Delete()
		require.NoError(t, err)
		require.NotNil(t, exec.lastCompiled)
		assert.Contains(t, exec.lastCompiled.ConditionExpression, "attribute_exists")
	})
}

func TestQuery_Pagination(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Query with limit
	q.Where("id", "=", "test-123").
		Limit(10)

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.NotNil(t, compiled.Limit)
	assert.Equal(t, int32(10), *compiled.Limit)
}

func TestQuery_OrderBy(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Query with descending order
	q.Where("id", "=", "test-123").
		OrderBy("timestamp", "desc")

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.NotNil(t, compiled.ScanIndexForward)
	assert.False(t, *compiled.ScanIndexForward)
}

func TestQuery_SortKeyDetectedWhenUsingAttributeName(t *testing.T) {
	metadata := &attrAwareMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	q.Where("PK", "=", "HASHTAG_TIMELINE#test").
		Where("SK", ">", "cursor#1")

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.Equal(t, "Query", compiled.Operation)
	assert.Contains(t, compiled.KeyConditionExpression, "AND")
	assert.NotContains(t, compiled.FilterExpression, "SK")
}

func TestQuery_SortKeyDetectedWithRegistryMetadata(t *testing.T) {
	type HashtagStatusIndex struct {
		PK string `dynamorm:"pk"`
		SK string `dynamorm:"sk"`
	}

	reg := model.NewRegistry()
	require.NoError(t, reg.Register(&HashtagStatusIndex{}))
	meta, err := reg.GetMetadata(&HashtagStatusIndex{})
	require.NoError(t, err)

	adapter := &registryMetadataAdapter{meta: meta}
	executor := &mockExecutor{}

	q := query.New(&HashtagStatusIndex{}, adapter, executor)

	q.Where("PK", "=", "HASHTAG#foo").
		Where("SK", ">", "cursor").
		Limit(2).
		OrderBy("SK", "asc")

	compiled, err := q.Compile()
	require.NoError(t, err)
	require.Equal(t, "Query", compiled.Operation)
	require.Contains(t, compiled.KeyConditionExpression, "AND")
	require.Empty(t, compiled.FilterExpression)
	require.Equal(t, "PK", compiled.ExpressionAttributeNames["#n1"])
	require.Equal(t, "SK", compiled.ExpressionAttributeNames["#n2"])
}

func TestQuery_CustomAttributeBeginsWithAsKeyCondition(t *testing.T) {
	type Notification struct {
		Partition string `dynamorm:"pk,attr:PK"`
		Sort      string `dynamorm:"sk,attr:SK"`
		Type      string `dynamorm:"attr:type"`
	}

	reg := model.NewRegistry()
	require.NoError(t, reg.Register(&Notification{}))
	meta, err := reg.GetMetadata(&Notification{})
	require.NoError(t, err)

	adapter := &registryMetadataAdapter{meta: meta}
	executor := &mockExecutor{}

	q := query.New(&Notification{}, adapter, executor)

	q.Where("PK", "=", "USER#admin").
		Where("SK", "begins_with", "NOTIF#")

	compiled, err := q.Compile()
	require.NoError(t, err)
	require.Equal(t, "Query", compiled.Operation)
	require.Contains(t, compiled.KeyConditionExpression, "begins_with")
	require.Empty(t, compiled.FilterExpression)
	require.Equal(t, "PK", compiled.ExpressionAttributeNames["#n1"])
	require.Equal(t, "SK", compiled.ExpressionAttributeNames["#n2"])
}

func TestQuery_BatchGet(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockBatchExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Batch get with composite keys
	keys := []any{
		TestItem{ID: "test-1", Timestamp: 1000},
		TestItem{ID: "test-2", Timestamp: 2000},
	}

	var results []TestItem
	err := q.BatchGet(keys, &results)
	assert.NoError(t, err)
}

func TestQuery_BatchCreate(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockBatchExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Batch create
	items := []TestItem{
		{ID: "test-1", Timestamp: 1000, Status: "active"},
		{ID: "test-2", Timestamp: 2000, Status: "pending"},
	}

	err := q.BatchCreate(items)
	assert.NoError(t, err)
}

func TestQuery_ReservedWords(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	// Add a field with reserved word
	type ItemWithReserved struct {
		ID     string `dynamodb:"id"`
		Status string `dynamodb:"status"` // "STATUS" is a reserved word
		Size   int    `dynamodb:"size"`   // "SIZE" is a reserved word
	}

	q := query.New(&ItemWithReserved{}, metadata, executor)

	q.Where("status", "=", "active").
		Where("size", ">", 10)

	compiled, err := q.Compile()
	assert.NoError(t, err)

	// Check that reserved words are properly escaped
	assert.Contains(t, compiled.ExpressionAttributeNames, "#STATUS")
	assert.Contains(t, compiled.ExpressionAttributeNames, "#SIZE")
}

func TestQuery_ComplexExpressions(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	// Test various operators
	testCases := []struct {
		name     string
		setup    func(*query.Query)
		validate func(*testing.T, *core.CompiledQuery)
	}{
		{
			name: "IN operator",
			setup: func(q *query.Query) {
				q.Where("id", "=", "test").
					Where("status", "IN", []string{"active", "pending", "completed"})
			},
			validate: func(t *testing.T, c *core.CompiledQuery) {
				assert.Contains(t, c.FilterExpression, "IN")
			},
		},
		{
			name: "BEGINS_WITH operator",
			setup: func(q *query.Query) {
				q.Where("id", "=", "test").
					Where("data", "BEGINS_WITH", "prefix")
			},
			validate: func(t *testing.T, c *core.CompiledQuery) {
				assert.Contains(t, c.FilterExpression, "begins_with")
			},
		},
		{
			name: "CONTAINS operator",
			setup: func(q *query.Query) {
				q.Where("id", "=", "test").
					Where("data", "CONTAINS", "substring")
			},
			validate: func(t *testing.T, c *core.CompiledQuery) {
				assert.Contains(t, c.FilterExpression, "contains")
			},
		},
		{
			name: "EXISTS operator",
			setup: func(q *query.Query) {
				q.Where("id", "=", "test").
					Where("data", "EXISTS", nil)
			},
			validate: func(t *testing.T, c *core.CompiledQuery) {
				assert.Contains(t, c.FilterExpression, "attribute_exists")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q := query.New(&TestItem{}, metadata, executor)
			tc.setup(q)
			compiled, err := q.Compile()
			assert.NoError(t, err)
			tc.validate(t, compiled)
		})
	}
}

// Mock batch executor for testing batch operations
type mockBatchExecutor struct {
	mockExecutor
}

func (m *mockBatchExecutor) ExecuteBatchGet(input *query.CompiledBatchGet, dest any) error {
	return nil
}

func (m *mockBatchExecutor) ExecuteBatchWrite(input *query.CompiledBatchWrite) error {
	return nil
}

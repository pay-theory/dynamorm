package dynamorm

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/require"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/session"
	transactionPkg "github.com/pay-theory/dynamorm/pkg/transaction"
)

func TestNewKeyPairAndDefaultBatchGetOptions_COV5(t *testing.T) {
	pair := NewKeyPair("p1", "s1")
	require.Equal(t, "p1", pair.PartitionKey)
	require.Equal(t, "s1", pair.SortKey)

	opts := DefaultBatchGetOptions()
	require.NotNil(t, opts)
	require.Equal(t, 100, opts.ChunkSize)
	require.NotNil(t, opts.RetryPolicy)
}

func TestDB_Transaction_SetsDBOnTx_COV5(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	called := false
	require.NoError(t, db.Transaction(func(tx *core.Tx) error {
		called = true
		q := tx.Model(&cov4RootItem{})
		_, ok := q.(*query)
		require.True(t, ok)
		return nil
	}))
	require.True(t, called)
}

func TestDB_TransactionFunc_CommitsAndRollsBack_COV5(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	require.NoError(t, db.registry.Register(&cov4RootItem{}))

	require.ErrorContains(t, db.TransactionFunc(func(any) error {
		return errors.New("boom")
	}), "boom")

	require.NoError(t, db.TransactionFunc(func(tx any) error {
		txx, ok := tx.(*transactionPkg.Transaction)
		require.True(t, ok)
		return txx.Create(&cov4RootItem{ID: "u1", Name: "alice"})
	}))

	require.Equal(t, 1, countRequestsByTarget(httpClient.Requests(), "DynamoDB_20120810.TransactWriteItems"))
}

func TestQuery_Scan_UnmarshalsItems_COV5(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.Scan": `{"Items":[{"id":{"S":"u1"},"name":{"S":"alice"}}],"Count":1,"ScannedCount":1}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	var out []cov4RootItem
	require.NoError(t, db.Model(&cov4RootItem{}).Scan(&out))
	require.Len(t, out, 1)
	require.Equal(t, cov4RootItem{ID: "u1", Name: "alice"}, out[0])
}

func TestQuery_UpdateBuilder_ExecuteAndExecuteWithResult_COV5(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.UpdateItem": `{"Attributes":{"id":{"S":"u1"},"name":{"S":"bob"}}}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	q := db.Model(&cov4RootItem{}).Where("ID", "=", "u1")

	require.NoError(t, q.UpdateBuilder().Set("Name", "bob").Execute())

	var out cov4RootItem
	require.NoError(t, q.UpdateBuilder().Set("Name", "bob").ExecuteWithResult(&out))
	require.Equal(t, cov4RootItem{ID: "u1", Name: "bob"}, out)

	require.GreaterOrEqual(t, countRequestsByTarget(httpClient.Requests(), "DynamoDB_20120810.UpdateItem"), 2)
}

func TestErrorUpdateBuilder_MethodsReturnSelf_COV5(t *testing.T) {
	errBoom := errors.New("boom")
	b := &errorUpdateBuilder{err: errBoom}

	require.Same(t, b, b.Set("field", "value"))
	require.Same(t, b, b.SetIfNotExists("field", "value", "default"))
	require.Same(t, b, b.Add("field", 1))
	require.Same(t, b, b.Increment("field"))
	require.Same(t, b, b.Decrement("field"))
	require.Same(t, b, b.Remove("field"))
	require.Same(t, b, b.Delete("field", "value"))
	require.Same(t, b, b.AppendToList("field", []string{"x"}))
	require.Same(t, b, b.PrependToList("field", []string{"y"}))
	require.Same(t, b, b.RemoveFromListAt("field", 0))
	require.Same(t, b, b.SetListElement("field", 1, "v"))
	require.Same(t, b, b.Condition("field", "=", "v"))
	require.Same(t, b, b.OrCondition("field", "=", "v"))
	require.Same(t, b, b.ConditionExists("field"))
	require.Same(t, b, b.ConditionNotExists("field"))
	require.Same(t, b, b.ConditionVersion(1))
	require.Same(t, b, b.ReturnValues("ALL_NEW"))

	require.ErrorIs(t, b.Execute(), errBoom)
	require.ErrorIs(t, b.ExecuteWithResult(&cov4RootItem{}), errBoom)
}

func TestDB_CreateTableAndEnsureTable_InvalidInputs_COV5(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	require.Error(t, db.CreateTable(123))
	require.Error(t, db.CreateTable(&cov4RootItem{}, "not-a-table-option"))
	require.Error(t, db.EnsureTable(123))
}

func TestDB_EnsureTable_TableExists_COV5(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.DescribeTable": `{"Table":{"TableName":"cov4_items","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	require.NoError(t, db.EnsureTable(&cov4RootItem{}))
	require.GreaterOrEqual(t, countRequestsByTarget(httpClient.Requests(), "DynamoDB_20120810.DescribeTable"), 1)
}

func TestDB_CreateTable_Success_COV5(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.CreateTable":   `{}`,
		"DynamoDB_20120810.DescribeTable": `{"Table":{"TableName":"cov4_items","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	require.NoError(t, db.CreateTable(&cov4RootItem{}))
	require.Equal(t, 1, countRequestsByTarget(httpClient.Requests(), "DynamoDB_20120810.CreateTable"))
}

func TestQuery_BatchGet_WithOptionsAndBuilder_COV5(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.BatchGetItem": `{"Responses":{"cov4_items":[{"id":{"S":"u2"},"name":{"S":"bob"}},{"id":{"S":"u1"},"name":{"S":"alice"}}]},"UnprocessedKeys":{}}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	keys := []any{
		NewKeyPair("u1"),
		NewKeyPair("u2"),
	}

	var out []cov4RootItem
	require.NoError(t, db.Model(&cov4RootItem{}).BatchGetWithOptions(keys, &out, nil))
	require.Len(t, out, 2)
	require.Equal(t, "u1", out[0].ID)
	require.Equal(t, "u2", out[1].ID)

	out = nil
	require.NoError(t, db.Model(&cov4RootItem{}).BatchGetBuilder().Keys(keys).Execute(&out))
	require.Len(t, out, 2)
	require.Equal(t, "u1", out[0].ID)
	require.Equal(t, "u2", out[1].ID)

	require.GreaterOrEqual(t, countRequestsByTarget(httpClient.Requests(), "DynamoDB_20120810.BatchGetItem"), 2)
}

func TestQuery_UpdateAndDelete_VersionConditions_COV5(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.UpdateItem": `{}`,
		"DynamoDB_20120810.DeleteItem": `{}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	model := &cov4MarshalModel{
		ID:      "u1",
		Name:    "alice",
		Version: 2,
	}

	q := db.Model(model).Where("ID", "=", "u1")

	require.NoError(t, q.Update("Name"))
	require.NoError(t, q.Update())
	require.NoError(t, q.Delete())

	updateReq := findRequestByTarget(httpClient.Requests(), "DynamoDB_20120810.UpdateItem")
	require.NotNil(t, updateReq)
	require.Contains(t, updateReq.Payload, "ConditionExpression")

	deleteReq := findRequestByTarget(httpClient.Requests(), "DynamoDB_20120810.DeleteItem")
	require.NotNil(t, deleteReq)
	require.Contains(t, deleteReq.Payload, "ConditionExpression")
}

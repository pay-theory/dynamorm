package dynamorm

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestQuery_mergeQueryConditions_HandlesNilMapsAndDuplicatePlaceholders_COV6(t *testing.T) {
	db := newBareDB()

	qAny := db.Model(&cov4RootItem{})
	q, ok := qAny.(*query)
	require.True(t, ok)

	meta, err := db.registry.GetMetadata(&cov4RootItem{})
	require.NoError(t, err)

	q.Where("Name", "=", "alice")

	expr, names, values, err := q.mergeQueryConditions(meta, "attribute_exists(id)", nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, expr)
	require.NotEmpty(t, names)
	require.NotEmpty(t, values)

	_, _, _, err = q.mergeQueryConditions(meta, "", nil, map[string]types.AttributeValue{
		":v1": &types.AttributeValueMemberS{Value: "dup"},
	})
	require.ErrorContains(t, err, "duplicate condition value placeholder")
}

func TestQuery_WithCondition_ValidatesOperatorAndMissingField_COV6(t *testing.T) {
	db := newBareDB()

	qAny := db.Model(&cov4RootItem{})
	q, ok := qAny.(*query)
	require.True(t, ok)

	q.WithCondition("Name", "", "alice")
	require.ErrorContains(t, q.checkBuilderError(), "operator cannot be empty")

	qAny = db.Model(&cov4RootItem{})
	q, ok = qAny.(*query)
	require.True(t, ok)
	q.WithCondition("missing", "=", "v")
	require.NoError(t, q.checkBuilderError())
}

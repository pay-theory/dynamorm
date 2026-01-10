package dynamorm

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestQuery_buildKeyMapFromAnyKey_HandlesStructPointerAndNilPointer_COV6(t *testing.T) {
	db := newBareDB()

	qAny := db.Model(&cov4RootItem{})
	q, ok := qAny.(*query)
	require.True(t, ok)

	meta, err := db.registry.GetMetadata(&cov4RootItem{})
	require.NoError(t, err)

	m, err := q.buildKeyMapFromAnyKey(cov4RootItem{ID: "u1"}, meta)
	require.NoError(t, err)
	require.Contains(t, m, "id")
	_, ok = m["id"].(*types.AttributeValueMemberS)
	require.True(t, ok)

	m, err = q.buildKeyMapFromAnyKey(&cov4RootItem{ID: "u2"}, meta)
	require.NoError(t, err)
	_, ok = m["id"].(*types.AttributeValueMemberS)
	require.True(t, ok)

	m, err = q.buildKeyMapFromAnyKey("u3", meta)
	require.NoError(t, err)
	_, ok = m["id"].(*types.AttributeValueMemberS)
	require.True(t, ok)

	var nilPtr *cov4RootItem
	m, err = q.buildKeyMapFromAnyKey(nilPtr, meta)
	require.NoError(t, err)
	_, ok = m["id"].(*types.AttributeValueMemberNULL)
	require.True(t, ok)
}

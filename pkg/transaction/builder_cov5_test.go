package transaction

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	"github.com/pay-theory/dynamorm/internal/expr"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/model"
	pkgTypes "github.com/pay-theory/dynamorm/pkg/types"
)

type cov5Item struct {
	PK      string
	Status  string
	Version int64
}

func cov5ItemMetadata(t *testing.T) *model.Metadata {
	t.Helper()

	typ := reflect.TypeOf(cov5Item{})
	pkField, ok := typ.FieldByName("PK")
	require.True(t, ok)
	statusField, ok := typ.FieldByName("Status")
	require.True(t, ok)
	versionField, ok := typ.FieldByName("Version")
	require.True(t, ok)

	pkMeta := &model.FieldMetadata{
		Name:      "PK",
		DBName:    "pk",
		Type:      reflect.TypeOf(""),
		Index:     pkField.Index[0],
		IndexPath: pkField.Index,
	}
	statusMeta := &model.FieldMetadata{
		Name:      "Status",
		DBName:    "status",
		Type:      reflect.TypeOf(""),
		Index:     statusField.Index[0],
		IndexPath: statusField.Index,
	}
	versionMeta := &model.FieldMetadata{
		Name:      "Version",
		DBName:    "ver",
		Type:      reflect.TypeOf(int64(0)),
		Index:     versionField.Index[0],
		IndexPath: versionField.Index,
	}

	meta := &model.Metadata{
		TableName:      "tbl",
		Fields:         make(map[string]*model.FieldMetadata),
		FieldsByDBName: make(map[string]*model.FieldMetadata),
		PrimaryKey: &model.KeySchema{
			PartitionKey: pkMeta,
		},
		VersionField: versionMeta,
	}

	meta.Fields["PK"] = pkMeta
	meta.FieldsByDBName["pk"] = pkMeta
	meta.Fields["Status"] = statusMeta
	meta.FieldsByDBName["status"] = statusMeta
	meta.Fields["Version"] = versionMeta
	meta.FieldsByDBName["ver"] = versionMeta

	return meta
}

func TestBuilder_applyConditionsToBuilder_CoversConditionKinds(t *testing.T) {
	converter := pkgTypes.NewConverter()
	b := &Builder{converter: converter}
	meta := cov5ItemMetadata(t)

	builder := expr.NewBuilderWithConverter(converter)
	raw, err := b.applyConditionsToBuilder(meta, builder, []core.TransactCondition{
		{Field: "Status", Operator: "=", Value: "ok"},
		{Kind: core.TransactConditionKindPrimaryKeyExists},
		{Kind: core.TransactConditionKindPrimaryKeyNotExists},
		{Kind: core.TransactConditionKindVersionEquals, Value: int64(1)},
		{Kind: core.TransactConditionKindExpression, Expression: "status = :v", Values: map[string]any{":v": "ok"}},
	})
	require.NoError(t, err)
	require.Len(t, raw, 1)
}

func TestBuilder_buildBuilderUpdate_AppliesBuilderAndRawConditions(t *testing.T) {
	converter := pkgTypes.NewConverter()
	b := &Builder{converter: converter}
	meta := cov5ItemMetadata(t)

	item := cov5Item{PK: "p1", Status: "ok", Version: 1}
	op := transactOperation{
		model:    &item,
		metadata: meta,
		typ:      opUpdateWithBuilder,
		updateFn: func(ub core.UpdateBuilder) error {
			ub.Set("Status", "new")
			return nil
		},
		conditions: []core.TransactCondition{
			{Kind: core.TransactConditionKindPrimaryKeyExists},
			{Kind: core.TransactConditionKindVersionEquals, Value: int64(1)},
			{Kind: core.TransactConditionKindField, Field: "status", Operator: "=", Value: "ok"},
			{Kind: core.TransactConditionKindExpression, Expression: "attribute_exists(pk)", Values: map[string]any{}},
		},
	}

	update, err := b.buildBuilderUpdate(op, 0)
	require.NoError(t, err)
	require.NotNil(t, update)
	require.NotNil(t, update.UpdateExpression)
	require.NotEmpty(t, aws.ToString(update.UpdateExpression))
	require.NotNil(t, update.ConditionExpression)
	require.NotEmpty(t, aws.ToString(update.ConditionExpression))
	require.NotEmpty(t, update.Key)
}

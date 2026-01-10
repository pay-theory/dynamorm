package dynamorm

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/pay-theory/dynamorm/internal/expr"
	"github.com/pay-theory/dynamorm/pkg/marshal"
	"github.com/pay-theory/dynamorm/pkg/model"
	pkgTypes "github.com/pay-theory/dynamorm/pkg/types"
)

type cov5StringConverter struct{}

func (cov5StringConverter) ToAttributeValue(value any) (ddbTypes.AttributeValue, error) {
	s, ok := value.(string)
	if !ok {
		return nil, errors.New("expected string")
	}
	return &ddbTypes.AttributeValueMemberS{Value: s}, nil
}

func (cov5StringConverter) FromAttributeValue(av ddbTypes.AttributeValue, target any) error {
	member, ok := av.(*ddbTypes.AttributeValueMemberS)
	if !ok {
		return errors.New("expected string attribute")
	}
	dst, ok := target.(*string)
	if !ok {
		return errors.New("expected *string target")
	}
	*dst = member.Value
	return nil
}

func TestDB_RegisterTypeConverter_ValidatesInputs_COV5(t *testing.T) {
	converter := pkgTypes.NewConverter()
	db := &DB{
		converter: converter,
		marshaler: marshal.New(converter),
	}

	require.Error(t, db.RegisterTypeConverter(nil, cov5StringConverter{}))
	require.Error(t, db.RegisterTypeConverter(reflect.TypeOf(""), nil))

	require.NoError(t, db.RegisterTypeConverter(reflect.TypeOf(""), cov5StringConverter{}))
	require.True(t, db.converter.HasCustomConverter(reflect.TypeOf("")))
}

func TestQuery_unmarshalItem_MapDestinationAndTypeErrors_COV5(t *testing.T) {
	q := &query{db: &DB{converter: pkgTypes.NewConverter()}}

	var emptyDest map[string]any
	require.NoError(t, q.unmarshalItem(map[string]ddbTypes.AttributeValue{}, &emptyDest, nil))
	require.NotNil(t, emptyDest)

	item := map[string]ddbTypes.AttributeValue{
		"id": &ddbTypes.AttributeValueMemberS{Value: "u1"},
		"n":  &ddbTypes.AttributeValueMemberN{Value: "3"},
	}

	var dest map[string]any
	require.Error(t, q.unmarshalItem(item, &dest, nil))
	require.NotNil(t, dest)

	require.Error(t, q.unmarshalItem(item, dest, nil))

	var invalid int
	require.Error(t, q.unmarshalItem(item, &invalid, nil))
}

func TestErrorBatchGetBuilder_FluentMethodsReturnSelf_COV5(t *testing.T) {
	errBoom := errors.New("boom")
	b := &errorBatchGetBuilder{err: errBoom}

	require.Same(t, b, b.Keys(nil))
	require.Same(t, b, b.ChunkSize(1))
	require.Same(t, b, b.ConsistentRead())
	require.Same(t, b, b.Parallel(2))
	require.Same(t, b, b.WithRetry(nil))
	require.Same(t, b, b.Select("ID"))
	require.Same(t, b, b.OnProgress(nil))
	require.Same(t, b, b.OnError(nil))
	require.ErrorIs(t, b.Execute(nil), errBoom)
}

type cov5KeyModel struct {
	PK string
	SK string
}

func TestQuery_extractPrimaryKey_CoversConditionsAndModelFallback_COV5(t *testing.T) {
	typ := reflect.TypeOf(cov5KeyModel{})
	pkField, ok := typ.FieldByName("PK")
	require.True(t, ok)
	skField, ok := typ.FieldByName("SK")
	require.True(t, ok)

	pkMeta := &model.FieldMetadata{
		Name:      "PK",
		DBName:    "pk",
		IsPK:      true,
		Index:     pkField.Index[0],
		IndexPath: pkField.Index,
	}
	skMeta := &model.FieldMetadata{
		Name:      "SK",
		DBName:    "sk",
		IsSK:      true,
		Index:     skField.Index[0],
		IndexPath: skField.Index,
	}

	meta := &model.Metadata{
		TableName: "tbl",
		PrimaryKey: &model.KeySchema{
			PartitionKey: pkMeta,
			SortKey:      skMeta,
		},
		Fields: map[string]*model.FieldMetadata{
			"PK": pkMeta,
			"SK": skMeta,
		},
		FieldsByDBName: map[string]*model.FieldMetadata{
			"pk": pkMeta,
			"sk": skMeta,
		},
	}

	q := &query{
		model: &cov5KeyModel{PK: "mp", SK: "ms"},
		conditions: []condition{
			{field: "pk", op: "=", value: "p1"},
			{field: "SK", op: "=", value: "s1"},
			{field: "PK", op: ">", value: "ignored"},
		},
	}

	pk := q.extractPrimaryKey(meta)
	require.NotNil(t, pk)
	require.Equal(t, "p1", pk["pk"])
	require.Equal(t, "s1", pk["sk"])

	q = &query{model: &cov5KeyModel{PK: "mp2", SK: "ms2"}}
	pk = q.extractPrimaryKey(meta)
	require.NotNil(t, pk)
	require.Equal(t, "mp2", pk["pk"])
	require.Equal(t, "ms2", pk["sk"])

	q = &query{model: &cov5KeyModel{}}
	require.Nil(t, q.extractPrimaryKey(meta))
}

func TestQuery_ttlValue_CoversTimeConversion_COV5(t *testing.T) {
	q := &query{}

	v, err := q.ttlValue("ttl", reflect.ValueOf(int64(5)))
	require.NoError(t, err)
	require.Equal(t, int64(5), v)

	v, err = q.ttlValue("ttl", reflect.ValueOf(time.Time{}))
	require.NoError(t, err)
	require.Equal(t, time.Time{}, v)

	v, err = q.ttlValue("ttl", reflect.ValueOf(time.Unix(123, 0)))
	require.NoError(t, err)
	require.Equal(t, int64(123), v)
}

func TestQuery_checkLambdaTimeout_CoversDeadlineBranches_COV5(t *testing.T) {
	q := &query{db: &DB{}}
	require.NoError(t, q.checkLambdaTimeout())

	q = &query{db: &DB{lambdaDeadline: time.Now().Add(-time.Second)}}
	require.Error(t, q.checkLambdaTimeout())

	q = &query{db: &DB{lambdaDeadline: time.Now().Add(200 * time.Millisecond), lambdaTimeoutBuffer: time.Second}}
	require.Error(t, q.checkLambdaTimeout())

	q = &query{db: &DB{lambdaDeadline: time.Now().Add(500 * time.Millisecond), lambdaTimeoutBuffer: 10 * time.Millisecond}}
	require.NoError(t, q.checkLambdaTimeout())
}

func TestQuery_determineKeyRoles_UsesIndexMetadata_COV5(t *testing.T) {
	pkMeta := &model.FieldMetadata{Name: "GSI_PK"}
	skMeta := &model.FieldMetadata{Name: "GSI_SK"}

	meta := &model.Metadata{
		Indexes: []model.IndexSchema{
			{
				Name:         "byGSI",
				PartitionKey: pkMeta,
				SortKey:      skMeta,
			},
		},
	}

	q := &query{indexName: "byGSI"}

	isPK, isSK := q.determineKeyRoles(&model.FieldMetadata{Name: "GSI_PK"}, meta)
	require.True(t, isPK)
	require.False(t, isSK)

	isPK, isSK = q.determineKeyRoles(&model.FieldMetadata{Name: "GSI_SK"}, meta)
	require.False(t, isPK)
	require.True(t, isSK)

	isPK, isSK = q.determineKeyRoles(&model.FieldMetadata{Name: "Other"}, meta)
	require.False(t, isPK)
	require.False(t, isSK)
}

func TestQuery_applyReadOptions_CoversIndexAndConsistencyRules_COV5(t *testing.T) {
	limit := 5

	q := &query{
		indexName:      "idx",
		consistentRead: true,
		orderBy:        &orderBy{order: "DESC"},
		limit:          &limit,
	}

	queryInput := &dynamodb.QueryInput{}
	q.applyQueryReadOptions(queryInput)
	require.NotNil(t, queryInput.IndexName)
	require.Equal(t, "idx", *queryInput.IndexName)
	require.NotNil(t, queryInput.ScanIndexForward)
	require.Equal(t, false, *queryInput.ScanIndexForward)
	require.NotNil(t, queryInput.Limit)
	require.Equal(t, int32(5), *queryInput.Limit)
	require.Nil(t, queryInput.ConsistentRead)

	queryCountInput := &dynamodb.QueryInput{}
	q.applyQueryCountOptions(queryCountInput)
	require.NotNil(t, queryCountInput.IndexName)

	q2 := &query{
		consistentRead: true,
		limit:          &limit,
	}
	scanInput := &dynamodb.ScanInput{}
	q2.applyScanReadOptions(scanInput)
	require.NotNil(t, scanInput.Limit)
	require.NotNil(t, scanInput.ConsistentRead)
}

func TestMetadataAdapter_CoversPrimaryKeyIndexesAndAttributes_COV5(t *testing.T) {
	pk := &model.FieldMetadata{
		Name:   "PK",
		DBName: "pk",
		Type:   reflect.TypeOf(""),
		Tags: map[string]string{
			"tag": "value",
		},
	}
	sk := &model.FieldMetadata{
		Name:   "SK",
		DBName: "sk",
		Type:   reflect.TypeOf(""),
	}

	meta := &model.Metadata{
		TableName: "tbl",
		PrimaryKey: &model.KeySchema{
			PartitionKey: pk,
			SortKey:      sk,
		},
		Fields: map[string]*model.FieldMetadata{
			"PK": pk,
			"SK": sk,
		},
		FieldsByDBName: map[string]*model.FieldMetadata{
			"pk": pk,
			"sk": sk,
		},
		Indexes: []model.IndexSchema{
			{
				Name:         "byGSI",
				Type:         model.GlobalSecondaryIndex,
				PartitionKey: pk,
				SortKey:      sk,
			},
			{
				Name:         "byPK",
				Type:         model.LocalSecondaryIndex,
				PartitionKey: pk,
			},
		},
		VersionField: &model.FieldMetadata{
			Name:   "Version",
			DBName: "ver",
		},
	}

	adapter := &metadataAdapter{metadata: meta}
	require.Equal(t, "tbl", adapter.TableName())

	keySchema := adapter.PrimaryKey()
	require.Equal(t, "PK", keySchema.PartitionKey)
	require.Equal(t, "SK", keySchema.SortKey)

	indexes := adapter.Indexes()
	require.Len(t, indexes, 2)
	require.Equal(t, "byGSI", indexes[0].Name)
	require.Equal(t, "PK", indexes[0].PartitionKey)
	require.Equal(t, "SK", indexes[0].SortKey)
	require.Equal(t, "byPK", indexes[1].Name)
	require.Equal(t, "PK", indexes[1].PartitionKey)
	require.Empty(t, indexes[1].SortKey)

	attr := adapter.AttributeMetadata("PK")
	require.NotNil(t, attr)
	require.Equal(t, "PK", attr.Name)
	require.Equal(t, "pk", attr.DynamoDBName)
	require.Equal(t, "tag", func() string {
		for k := range attr.Tags {
			return k
		}
		return ""
	}())

	attr = adapter.AttributeMetadata("pk")
	require.NotNil(t, attr)
	require.Nil(t, adapter.AttributeMetadata("missing"))

	require.Equal(t, "ver", adapter.VersionFieldName())
	meta.VersionField.DBName = ""
	require.Equal(t, "Version", adapter.VersionFieldName())
	meta.VersionField = nil
	require.Empty(t, adapter.VersionFieldName())

	emptyAdapter := &metadataAdapter{metadata: &model.Metadata{TableName: "tbl"}}
	emptySchema := emptyAdapter.PrimaryKey()
	require.Empty(t, emptySchema.PartitionKey)
	require.Empty(t, emptySchema.SortKey)
	require.Nil(t, emptyAdapter.AttributeMetadata("pk"))

	nilAdapter := &metadataAdapter{}
	require.Empty(t, nilAdapter.VersionFieldName())
}

func TestSliceValue_CoversErrorsAndPointerHandling_COV5(t *testing.T) {
	_, err := sliceValue(nil)
	require.Error(t, err)

	var nilPtr *[]int
	_, err = sliceValue(nilPtr)
	require.Error(t, err)

	_, err = sliceValue(123)
	require.Error(t, err)

	got, err := sliceValue([]int{1, 2})
	require.NoError(t, err)
	require.Equal(t, reflect.Slice, got.Kind())

	slice := []int{1}
	got, err = sliceValue(&slice)
	require.NoError(t, err)
	require.Equal(t, reflect.Slice, got.Kind())
}

func TestQuery_mergeRawConditionValues_CoversDuplicateAndConversionErrors_COV5(t *testing.T) {
	q := &query{db: &DB{converter: pkgTypes.NewConverter()}}

	dst := map[string]ddbTypes.AttributeValue{
		":v": &ddbTypes.AttributeValueMemberS{Value: "x"},
	}
	require.Error(t, q.mergeRawConditionValues(dst, map[string]any{":v": "dup"}))

	dst = map[string]ddbTypes.AttributeValue{}
	require.Error(t, q.mergeRawConditionValues(dst, map[string]any{":bad": make(chan int)}))

	require.NoError(t, q.mergeRawConditionValues(dst, map[string]any{":ok": "v"}))
	require.Contains(t, dst, ":ok")
}

func TestMapToAttributeName_CoversAllBranches_COV5(t *testing.T) {
	require.Equal(t, "Field", mapToAttributeName(nil, "Field"))

	fieldMeta := &model.FieldMetadata{
		Name:   "Field",
		DBName: "ddb_field",
	}
	meta := &model.Metadata{
		Fields: map[string]*model.FieldMetadata{
			"Field": fieldMeta,
		},
		FieldsByDBName: map[string]*model.FieldMetadata{
			"ddb_field": fieldMeta,
		},
	}

	require.Equal(t, "ddb_field", mapToAttributeName(meta, "Field"))
	require.Equal(t, "ddb_field", mapToAttributeName(meta, "ddb_field"))
	require.Equal(t, "Missing", mapToAttributeName(meta, "Missing"))
}

func TestMinInt_CoversBothBranches_COV5(t *testing.T) {
	require.Equal(t, 1, minInt(1, 2))
	require.Equal(t, 2, minInt(3, 2))
}

func TestCloneRawConditionValues_CoversEmptyAndCopies_COV5(t *testing.T) {
	require.Nil(t, cloneRawConditionValues(nil))
	require.Nil(t, cloneRawConditionValues(map[string]any{}))

	values := map[string]any{"k": 1}
	cloned := cloneRawConditionValues(values)
	require.NotNil(t, cloned)
	require.Equal(t, 1, cloned["k"])

	values["k"] = 2
	require.Equal(t, 1, cloned["k"])
}

func TestQuery_addWhereConditionsToBuilder_CoversMissingAndSkipKeyConditions_COV5(t *testing.T) {
	converter := pkgTypes.NewConverter()
	pkMeta := &model.FieldMetadata{Name: "PK", DBName: "pk", IsPK: true}
	nameMeta := &model.FieldMetadata{Name: "Name", DBName: "name"}
	meta := &model.Metadata{
		Fields: map[string]*model.FieldMetadata{
			"PK":   pkMeta,
			"Name": nameMeta,
		},
		FieldsByDBName: map[string]*model.FieldMetadata{
			"pk":   pkMeta,
			"name": nameMeta,
		},
	}

	q := &query{
		db: &DB{converter: converter},
		conditions: []condition{
			{field: "Missing", op: "=", value: "x"},
			{field: "PK", op: "=", value: "p1"},
			{field: "Name", op: "=", value: "n1"},
		},
	}

	builder := expr.NewBuilderWithConverter(converter)
	hasCondition, err := q.addWhereConditionsToBuilder(builder, meta, true)
	require.NoError(t, err)
	require.True(t, hasCondition)

	components := builder.Build()
	require.NotEmpty(t, components.ConditionExpression)

	names := make([]string, 0, len(components.ExpressionAttributeNames))
	for _, name := range components.ExpressionAttributeNames {
		names = append(names, name)
	}
	require.Contains(t, names, "name")
	require.NotContains(t, names, "pk")
}

func TestQuery_addWhereConditionsToBuilder_ReturnsErrorOnInvalidOperator_COV5(t *testing.T) {
	converter := pkgTypes.NewConverter()
	nameMeta := &model.FieldMetadata{Name: "Name", DBName: "name"}
	meta := &model.Metadata{
		Fields: map[string]*model.FieldMetadata{
			"Name": nameMeta,
		},
		FieldsByDBName: map[string]*model.FieldMetadata{
			"name": nameMeta,
		},
	}

	q := &query{
		db: &DB{converter: converter},
		conditions: []condition{
			{field: "Name", op: "INVALID", value: "n1"},
		},
	}

	builder := expr.NewBuilderWithConverter(converter)
	hasCondition, err := q.addWhereConditionsToBuilder(builder, meta, false)
	require.Error(t, err)
	require.False(t, hasCondition)
}

func TestQuery_addDefaultNotExistsConditionsToBuilder_CoversErrorAndSortKeyBranch_COV5(t *testing.T) {
	q := &query{db: &DB{converter: pkgTypes.NewConverter()}}
	builder := expr.NewBuilderWithConverter(q.db.converter)

	require.Error(t, q.addDefaultNotExistsConditionsToBuilder(builder, &model.Metadata{}))

	pkMeta := &model.FieldMetadata{Name: "PK", DBName: "pk"}
	skMeta := &model.FieldMetadata{Name: "SK", DBName: "sk"}
	meta := &model.Metadata{
		PrimaryKey: &model.KeySchema{
			PartitionKey: pkMeta,
			SortKey:      skMeta,
		},
	}

	builder = expr.NewBuilderWithConverter(q.db.converter)
	require.NoError(t, q.addDefaultNotExistsConditionsToBuilder(builder, meta))
	components := builder.Build()

	names := make([]string, 0, len(components.ExpressionAttributeNames))
	for _, name := range components.ExpressionAttributeNames {
		names = append(names, name)
	}
	require.Contains(t, names, "pk")
	require.Contains(t, names, "sk")
}

type cov5PrimaryKeyConditionModel struct {
	PK string `dynamorm:"pk"`
	SK string `dynamorm:"sk"`
}

func TestQuery_addPrimaryKeyCondition_CoversSortKeyAndRegistryError_COV5(t *testing.T) {
	converter := pkgTypes.NewConverter()
	db := &DB{
		registry:  model.NewRegistry(),
		converter: converter,
		marshaler: marshal.New(converter),
	}

	q := &query{db: db, model: &cov5PrimaryKeyConditionModel{}}
	q.addPrimaryKeyCondition("attribute_exists")
	require.Error(t, q.builderErr)
	require.Empty(t, q.writeConditions)

	require.NoError(t, db.registry.Register(&cov5PrimaryKeyConditionModel{}))
	meta, err := db.registry.GetMetadata(&cov5PrimaryKeyConditionModel{})
	require.NoError(t, err)

	q = &query{db: db, model: &cov5PrimaryKeyConditionModel{}}
	q.addPrimaryKeyCondition("attribute_exists")
	require.NoError(t, q.builderErr)
	require.Len(t, q.writeConditions, 2)
	require.Equal(t, meta.PrimaryKey.PartitionKey.DBName, q.writeConditions[0].field)
	require.Equal(t, "ATTRIBUTE_EXISTS", q.writeConditions[0].op)
	require.Equal(t, meta.PrimaryKey.SortKey.DBName, q.writeConditions[1].field)
	require.Equal(t, "ATTRIBUTE_EXISTS", q.writeConditions[1].op)
}

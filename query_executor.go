package dynamorm

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/pay-theory/dynamorm/internal/encryption"
	"github.com/pay-theory/dynamorm/pkg/core"
	customerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/model"
	"github.com/pay-theory/dynamorm/pkg/query"
	"github.com/pay-theory/dynamorm/pkg/session"
)

type queryExecutor struct {
	db       *DB
	metadata *model.Metadata
	ctx      context.Context
}

func (qe *queryExecutor) SetContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	qe.ctx = ctx
}

func (qe *queryExecutor) ctxOrBackground() context.Context {
	if qe.ctx != nil {
		return qe.ctx
	}
	if qe.db != nil && qe.db.ctx != nil {
		return qe.db.ctx
	}
	return context.Background()
}

func (qe *queryExecutor) checkLambdaTimeout() error {
	if qe == nil || qe.db == nil || qe.db.lambdaDeadline.IsZero() {
		return nil
	}

	remaining := time.Until(qe.db.lambdaDeadline)
	if remaining <= 0 {
		return fmt.Errorf("lambda timeout exceeded")
	}

	buffer := qe.db.lambdaTimeoutBuffer
	if buffer == 0 {
		buffer = 100 * time.Millisecond
	}
	if remaining < buffer {
		return fmt.Errorf("lambda timeout imminent: only %v remaining", remaining)
	}

	return nil
}

func (qe *queryExecutor) encryptionService() (*encryption.Service, error) {
	if qe == nil {
		return nil, fmt.Errorf("%w: query executor is nil", customerrors.ErrEncryptionNotConfigured)
	}
	if qe.db == nil || qe.db.session == nil || qe.db.session.Config() == nil {
		return nil, fmt.Errorf("%w: session is nil", customerrors.ErrEncryptionNotConfigured)
	}

	keyARN := qe.db.session.Config().KMSKeyARN
	if keyARN == "" {
		return nil, fmt.Errorf("%w: session.Config.KMSKeyARN is empty", customerrors.ErrEncryptionNotConfigured)
	}

	return encryption.NewServiceFromAWSConfig(keyARN, qe.db.session.AWSConfig()), nil
}

func (qe *queryExecutor) failClosedIfEncrypted() error {
	if qe == nil {
		return nil
	}
	return encryption.FailClosedIfEncryptedWithoutKMSKeyARN(qe.session(), qe.metadata)
}

func (qe *queryExecutor) session() *session.Session {
	if qe == nil || qe.db == nil {
		return nil
	}
	return qe.db.session
}

func (qe *queryExecutor) decryptItem(item map[string]types.AttributeValue) error {
	if len(item) == 0 || qe == nil || qe.metadata == nil || !encryption.MetadataHasEncryptedFields(qe.metadata) {
		return nil
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	svc, err := qe.encryptionService()
	if err != nil {
		return err
	}

	for attrName, attrValue := range item {
		fieldMeta, ok := qe.metadata.FieldsByDBName[attrName]
		if !ok || fieldMeta == nil || !fieldMeta.IsEncrypted {
			continue
		}

		decrypted, err := svc.DecryptAttributeValue(qe.ctxOrBackground(), fieldMeta.DBName, attrValue)
		if err != nil {
			return &customerrors.EncryptedFieldError{
				Operation: "decrypt",
				Field:     fieldMeta.Name,
				Err:       err,
			}
		}
		item[attrName] = decrypted
	}

	return nil
}

func (qe *queryExecutor) encryptItem(item map[string]types.AttributeValue) error {
	if len(item) == 0 || qe == nil || qe.metadata == nil || !encryption.MetadataHasEncryptedFields(qe.metadata) {
		return nil
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	svc, err := qe.encryptionService()
	if err != nil {
		return err
	}

	for _, fieldMeta := range qe.metadata.Fields {
		if fieldMeta == nil || !fieldMeta.IsEncrypted {
			continue
		}

		av, ok := item[fieldMeta.DBName]
		if !ok {
			continue
		}

		encryptedAV, err := svc.EncryptAttributeValue(qe.ctxOrBackground(), fieldMeta.DBName, av)
		if err != nil {
			return fmt.Errorf("failed to encrypt field %s: %w", fieldMeta.DBName, err)
		}
		item[fieldMeta.DBName] = encryptedAV
	}

	return nil
}

func (qe *queryExecutor) unmarshalItem(item map[string]types.AttributeValue, dest any) error {
	if qe == nil || qe.db == nil || qe.db.converter == nil {
		return fmt.Errorf("converter is required for unmarshal")
	}
	if dest == nil {
		return fmt.Errorf("destination must be a pointer to a struct or map")
	}

	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.IsNil() {
		return fmt.Errorf("destination must be a pointer")
	}
	destValue = destValue.Elem()

	if destValue.Kind() == reflect.Map {
		if destValue.IsNil() {
			destValue.Set(reflect.MakeMap(destValue.Type()))
		}

		for attrName, attrValue := range item {
			var val any
			if err := qe.db.converter.FromAttributeValue(attrValue, &val); err != nil {
				return fmt.Errorf("failed to unmarshal field %s: %w", attrName, err)
			}
			destValue.SetMapIndex(reflect.ValueOf(attrName), reflect.ValueOf(val))
		}

		return nil
	}

	if destValue.Kind() != reflect.Struct {
		return fmt.Errorf("destination must be a pointer to a struct or map")
	}

	if qe.metadata == nil {
		return fmt.Errorf("model metadata is required for unmarshal")
	}

	for attrName, attrValue := range item {
		fieldMeta, exists := qe.metadata.FieldsByDBName[attrName]
		if !exists || fieldMeta == nil {
			continue
		}

		structField := destValue.FieldByIndex(fieldMeta.IndexPath)
		if !structField.CanSet() {
			continue
		}

		if err := qe.db.converter.FromAttributeValue(attrValue, structField.Addr().Interface()); err != nil {
			return fmt.Errorf("failed to unmarshal field %s: %w", fieldMeta.Name, err)
		}
	}

	return nil
}

func (qe *queryExecutor) unmarshalItems(items []map[string]types.AttributeValue, dest any) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.IsNil() || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	destSlice := destValue.Elem()
	elemType := destSlice.Type().Elem()
	newSlice := reflect.MakeSlice(destSlice.Type(), len(items), len(items))

	for i, item := range items {
		var elem reflect.Value
		if elemType.Kind() == reflect.Ptr {
			elem = reflect.New(elemType.Elem())
		} else {
			elem = reflect.New(elemType)
		}

		if err := qe.unmarshalItem(item, elem.Interface()); err != nil {
			return fmt.Errorf("failed to unmarshal item %d: %w", i, err)
		}

		if elemType.Kind() == reflect.Ptr {
			newSlice.Index(i).Set(elem)
		} else {
			newSlice.Index(i).Set(elem.Elem())
		}
	}

	destSlice.Set(newSlice)
	return nil
}

func (qe *queryExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	client, err := qe.session().Client()
	if err != nil {
		return fmt.Errorf("failed to get client for query: %w", err)
	}

	queryInput := buildDynamoQueryInput(input)

	if isCountSelect(input.Select) {
		queryInput.Select = types.SelectCount
		queryInput.Limit = nil

		var totalCount int64
		var scannedCount int64

		paginator := dynamodb.NewQueryPaginator(client, queryInput)
		for paginator.HasMorePages() {
			output, err := paginator.NextPage(qe.ctxOrBackground())
			if err != nil {
				return fmt.Errorf("failed to count items: %w", err)
			}
			totalCount += int64(output.Count)
			scannedCount += int64(output.ScannedCount)
		}

		return writeCountResult(dest, totalCount, scannedCount)
	}

	paginator := dynamodb.NewQueryPaginator(client, queryInput)

	limit, hasLimit := compiledQueryLimit(input)
	items, err := collectPaginatedItems(
		paginator.HasMorePages,
		func(ctx context.Context) ([]map[string]types.AttributeValue, error) {
			output, err := paginator.NextPage(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to query items: %w", err)
			}
			return output.Items, nil
		},
		limit,
		hasLimit,
		true,
		qe.ctxOrBackground(),
	)
	if err != nil {
		return err
	}

	for _, item := range items {
		if err := qe.decryptItem(item); err != nil {
			return err
		}
	}

	if rawDest, ok := dest.(*[]map[string]types.AttributeValue); ok && rawDest != nil {
		*rawDest = append((*rawDest)[:0], items...)
		return nil
	}

	return qe.unmarshalItems(items, dest)
}

func (qe *queryExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	if input == nil {
		return fmt.Errorf("compiled scan cannot be nil")
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	client, err := qe.session().Client()
	if err != nil {
		return fmt.Errorf("failed to get client for scan: %w", err)
	}

	scanInput := buildDynamoScanInput(input)

	if isCountSelect(input.Select) {
		scanInput.Select = types.SelectCount
		scanInput.Limit = nil

		var totalCount int64
		var scannedCount int64

		paginator := dynamodb.NewScanPaginator(client, scanInput)
		for paginator.HasMorePages() {
			output, err := paginator.NextPage(qe.ctxOrBackground())
			if err != nil {
				return fmt.Errorf("failed to count items: %w", err)
			}
			totalCount += int64(output.Count)
			scannedCount += int64(output.ScannedCount)
		}

		return writeCountResult(dest, totalCount, scannedCount)
	}

	paginator := dynamodb.NewScanPaginator(client, scanInput)

	limit, hasLimit := compiledQueryLimit(input)
	items, err := collectPaginatedItems(
		paginator.HasMorePages,
		func(ctx context.Context) ([]map[string]types.AttributeValue, error) {
			output, err := paginator.NextPage(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to scan items: %w", err)
			}
			return output.Items, nil
		},
		limit,
		hasLimit,
		true,
		qe.ctxOrBackground(),
	)
	if err != nil {
		return err
	}

	for _, item := range items {
		if err := qe.decryptItem(item); err != nil {
			return err
		}
	}

	if rawDest, ok := dest.(*[]map[string]types.AttributeValue); ok && rawDest != nil {
		*rawDest = append((*rawDest)[:0], items...)
		return nil
	}

	return qe.unmarshalItems(items, dest)
}

func (qe *queryExecutor) ExecuteQueryWithPagination(input *core.CompiledQuery, dest any) (*query.QueryResult, error) {
	if input == nil {
		return nil, fmt.Errorf("compiled query cannot be nil")
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return nil, err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return nil, err
	}

	client, err := qe.session().Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for query: %w", err)
	}

	out, err := client.Query(qe.ctxOrBackground(), buildDynamoQueryInput(input))
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	for _, item := range out.Items {
		if err := qe.decryptItem(item); err != nil {
			return nil, err
		}
	}

	if rawDest, ok := dest.(*[]map[string]types.AttributeValue); ok && rawDest != nil {
		*rawDest = append((*rawDest)[:0], out.Items...)
	} else if err := qe.unmarshalItems(out.Items, dest); err != nil {
		return nil, err
	}

	return &query.QueryResult{
		Items:            out.Items,
		Count:            int64(out.Count),
		ScannedCount:     int64(out.ScannedCount),
		LastEvaluatedKey: out.LastEvaluatedKey,
	}, nil
}

func (qe *queryExecutor) ExecuteScanWithPagination(input *core.CompiledQuery, dest any) (*query.ScanResult, error) {
	if input == nil {
		return nil, fmt.Errorf("compiled scan cannot be nil")
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return nil, err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return nil, err
	}

	client, err := qe.session().Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for scan: %w", err)
	}

	out, err := client.Scan(qe.ctxOrBackground(), buildDynamoScanInput(input))
	if err != nil {
		return nil, fmt.Errorf("failed to execute scan: %w", err)
	}

	for _, item := range out.Items {
		if err := qe.decryptItem(item); err != nil {
			return nil, err
		}
	}

	if rawDest, ok := dest.(*[]map[string]types.AttributeValue); ok && rawDest != nil {
		*rawDest = append((*rawDest)[:0], out.Items...)
	} else if err := qe.unmarshalItems(out.Items, dest); err != nil {
		return nil, err
	}

	return &query.ScanResult{
		Items:            out.Items,
		Count:            int64(out.Count),
		ScannedCount:     int64(out.ScannedCount),
		LastEvaluatedKey: out.LastEvaluatedKey,
	}, nil
}

func (qe *queryExecutor) ExecuteGetItem(input *core.CompiledQuery, key map[string]types.AttributeValue, dest any) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}
	if len(key) == 0 {
		return fmt.Errorf("key cannot be empty")
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	client, err := qe.session().Client()
	if err != nil {
		return fmt.Errorf("failed to get client for get item: %w", err)
	}

	getInput := &dynamodb.GetItemInput{
		TableName: aws.String(input.TableName),
		Key:       key,
	}

	if input.ProjectionExpression != "" {
		getInput.ProjectionExpression = aws.String(input.ProjectionExpression)
	}
	if len(input.ExpressionAttributeNames) > 0 {
		getInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}
	if input.ConsistentRead != nil {
		getInput.ConsistentRead = input.ConsistentRead
	}

	out, err := client.GetItem(qe.ctxOrBackground(), getInput)
	if err != nil {
		return fmt.Errorf("failed to get item: %w", err)
	}
	if out.Item == nil {
		return customerrors.ErrItemNotFound
	}

	if err := qe.decryptItem(out.Item); err != nil {
		return err
	}

	if rawDest, ok := dest.(*map[string]types.AttributeValue); ok && rawDest != nil {
		*rawDest = out.Item
		return nil
	}

	return qe.unmarshalItem(out.Item, dest)
}

func (qe *queryExecutor) ExecutePutItem(input *core.CompiledQuery, item map[string]types.AttributeValue) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}
	if len(item) == 0 {
		return fmt.Errorf("item cannot be empty")
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	if err := qe.encryptItem(item); err != nil {
		return err
	}

	client, err := qe.session().Client()
	if err != nil {
		return fmt.Errorf("failed to get client for put item: %w", err)
	}

	putInput := &dynamodb.PutItemInput{
		TableName: aws.String(input.TableName),
		Item:      item,
	}

	if input.ConditionExpression != "" {
		putInput.ConditionExpression = aws.String(input.ConditionExpression)
	}
	if len(input.ExpressionAttributeNames) > 0 {
		putInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}
	if len(input.ExpressionAttributeValues) > 0 {
		putInput.ExpressionAttributeValues = input.ExpressionAttributeValues
	}

	_, err = client.PutItem(qe.ctxOrBackground(), putInput)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to put item: %w", err)
	}

	return nil
}

func (qe *queryExecutor) ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}
	if len(key) == 0 {
		return fmt.Errorf("key cannot be empty")
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	exprAttrValues := input.ExpressionAttributeValues
	if exprAttrValues == nil {
		exprAttrValues = make(map[string]types.AttributeValue)
	}

	if qe.metadata != nil && encryption.MetadataHasEncryptedFields(qe.metadata) {
		svc, err := qe.encryptionService()
		if err != nil {
			return err
		}
		if err := encryption.EncryptUpdateExpressionValues(qe.ctxOrBackground(), svc, qe.metadata, input.UpdateExpression, input.ExpressionAttributeNames, exprAttrValues); err != nil {
			return err
		}
	}

	client, err := qe.session().Client()
	if err != nil {
		return fmt.Errorf("failed to get client for update item: %w", err)
	}

	updateInput := &dynamodb.UpdateItemInput{
		TableName:                 aws.String(input.TableName),
		Key:                       key,
		UpdateExpression:          aws.String(input.UpdateExpression),
		ExpressionAttributeNames:  input.ExpressionAttributeNames,
		ExpressionAttributeValues: exprAttrValues,
	}

	if input.ConditionExpression != "" {
		updateInput.ConditionExpression = aws.String(input.ConditionExpression)
	}
	if input.ReturnValues != "" {
		updateInput.ReturnValues = types.ReturnValue(input.ReturnValues)
	}

	_, err = client.UpdateItem(qe.ctxOrBackground(), updateInput)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to update item: %w", err)
	}

	return nil
}

func (qe *queryExecutor) ExecuteUpdateItemWithResult(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error) {
	if input == nil {
		return nil, fmt.Errorf("compiled query cannot be nil")
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("key cannot be empty")
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return nil, err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return nil, err
	}

	exprAttrValues := input.ExpressionAttributeValues
	if exprAttrValues == nil {
		exprAttrValues = make(map[string]types.AttributeValue)
	}

	if qe.metadata != nil && encryption.MetadataHasEncryptedFields(qe.metadata) {
		svc, err := qe.encryptionService()
		if err != nil {
			return nil, err
		}
		if err := encryption.EncryptUpdateExpressionValues(qe.ctxOrBackground(), svc, qe.metadata, input.UpdateExpression, input.ExpressionAttributeNames, exprAttrValues); err != nil {
			return nil, err
		}
	}

	client, err := qe.session().Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for update item: %w", err)
	}

	updateInput := &dynamodb.UpdateItemInput{
		TableName:                 aws.String(input.TableName),
		Key:                       key,
		UpdateExpression:          aws.String(input.UpdateExpression),
		ExpressionAttributeNames:  input.ExpressionAttributeNames,
		ExpressionAttributeValues: exprAttrValues,
	}

	if input.ConditionExpression != "" {
		updateInput.ConditionExpression = aws.String(input.ConditionExpression)
	}
	if input.ReturnValues != "" {
		updateInput.ReturnValues = types.ReturnValue(input.ReturnValues)
	}

	output, err := client.UpdateItem(qe.ctxOrBackground(), updateInput)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return nil, customerrors.ErrConditionFailed
		}
		return nil, fmt.Errorf("failed to update item: %w", err)
	}

	if qe.metadata != nil && encryption.MetadataHasEncryptedFields(qe.metadata) && len(output.Attributes) > 0 {
		svc, err := qe.encryptionService()
		if err != nil {
			return nil, err
		}

		for attrName, attrValue := range output.Attributes {
			fieldMeta, ok := qe.metadata.FieldsByDBName[attrName]
			if !ok || fieldMeta == nil || !fieldMeta.IsEncrypted {
				continue
			}

			decrypted, err := svc.DecryptAttributeValue(qe.ctxOrBackground(), fieldMeta.DBName, attrValue)
			if err != nil {
				return nil, &customerrors.EncryptedFieldError{
					Operation: "decrypt",
					Field:     fieldMeta.Name,
					Err:       err,
				}
			}
			output.Attributes[attrName] = decrypted
		}
	}

	return &core.UpdateResult{
		Attributes: output.Attributes,
	}, nil
}

func (qe *queryExecutor) ExecuteDeleteItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}
	if len(key) == 0 {
		return fmt.Errorf("key cannot be empty")
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	client, err := qe.session().Client()
	if err != nil {
		return fmt.Errorf("failed to get client for delete item: %w", err)
	}

	deleteInput := &dynamodb.DeleteItemInput{
		TableName: aws.String(input.TableName),
		Key:       key,
	}

	if input.ConditionExpression != "" {
		deleteInput.ConditionExpression = aws.String(input.ConditionExpression)
	}
	if len(input.ExpressionAttributeNames) > 0 {
		deleteInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}
	if len(input.ExpressionAttributeValues) > 0 {
		deleteInput.ExpressionAttributeValues = input.ExpressionAttributeValues
	}

	_, err = client.DeleteItem(qe.ctxOrBackground(), deleteInput)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to delete item: %w", err)
	}

	return nil
}

func (qe *queryExecutor) ExecuteBatchGet(input *query.CompiledBatchGet, opts *core.BatchGetOptions) ([]map[string]types.AttributeValue, error) {
	if input == nil {
		return nil, fmt.Errorf("compiled batch get cannot be nil")
	}
	if len(input.Keys) == 0 {
		return nil, nil
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return nil, err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return nil, err
	}

	client, err := qe.session().Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for batch get: %w", err)
	}

	if opts == nil {
		opts = core.DefaultBatchGetOptions()
	} else {
		opts = opts.Clone()
	}

	requestItems := map[string]types.KeysAndAttributes{
		input.TableName: buildKeysAndAttributes(input),
	}

	var collected []map[string]types.AttributeValue
	retryAttempt := 0

	for len(requestItems) > 0 {
		output, err := client.BatchGetItem(qe.ctxOrBackground(), &dynamodb.BatchGetItemInput{
			RequestItems: requestItems,
		})
		if err != nil {
			return collected, fmt.Errorf("failed to batch get items: %w", err)
		}

		if items, exists := output.Responses[input.TableName]; exists {
			for _, item := range items {
				if err := qe.decryptItem(item); err != nil {
					return collected, err
				}
				collected = append(collected, item)
			}
		}

		unprocessed := output.UnprocessedKeys
		if len(unprocessed) == 0 {
			break
		}

		remaining := countUnprocessedKeys(unprocessed)
		if remaining == 0 {
			break
		}

		if opts.RetryPolicy == nil || retryAttempt >= opts.RetryPolicy.MaxRetries {
			return collected, fmt.Errorf("batch get exhausted retries with %d unprocessed keys", remaining)
		}

		delay := calculateBatchRetryDelay(opts.RetryPolicy, retryAttempt)
		retryAttempt++
		time.Sleep(delay)

		requestItems = unprocessed
	}

	return collected, nil
}

func (qe *queryExecutor) ExecuteBatchWrite(input *query.CompiledBatchWrite) error {
	if input == nil {
		return fmt.Errorf("compiled batch write cannot be nil")
	}
	if len(input.Items) == 0 {
		return nil
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	for i := 0; i < len(input.Items); i += 25 {
		end := i + 25
		if end > len(input.Items) {
			end = len(input.Items)
		}

		writeRequests := make([]types.WriteRequest, 0, end-i)
		for _, item := range input.Items[i:end] {
			writeRequests = append(writeRequests, types.WriteRequest{
				PutRequest: &types.PutRequest{Item: item},
			})
		}

		for {
			result, err := qe.ExecuteBatchWriteItem(input.TableName, writeRequests)
			if err != nil {
				return err
			}
			if result == nil || len(result.UnprocessedItems) == 0 {
				break
			}

			var unprocessed []types.WriteRequest
			for _, reqs := range result.UnprocessedItems {
				unprocessed = append(unprocessed, reqs...)
			}
			if len(unprocessed) == 0 {
				break
			}
			writeRequests = unprocessed
		}
	}

	return nil
}

func (qe *queryExecutor) ExecuteBatchWriteItem(tableName string, writeRequests []types.WriteRequest) (*core.BatchWriteResult, error) {
	if err := qe.checkLambdaTimeout(); err != nil {
		return nil, err
	}
	if len(writeRequests) == 0 {
		return &core.BatchWriteResult{}, nil
	}
	if len(writeRequests) > 25 {
		return nil, fmt.Errorf("batch write supports maximum 25 items per request, got %d", len(writeRequests))
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return nil, err
	}

	if qe.metadata != nil && encryption.MetadataHasEncryptedFields(qe.metadata) {
		for i := range writeRequests {
			put := writeRequests[i].PutRequest
			if put == nil || len(put.Item) == 0 {
				continue
			}
			if err := qe.encryptItem(put.Item); err != nil {
				return nil, err
			}
		}
	}

	client, err := qe.session().Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for batch write: %w", err)
	}

	output, err := client.BatchWriteItem(qe.ctxOrBackground(), &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			tableName: writeRequests,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("batch write failed: %w", err)
	}

	return &core.BatchWriteResult{
		UnprocessedItems: output.UnprocessedItems,
		ConsumedCapacity: output.ConsumedCapacity,
	}, nil
}

func buildDynamoQueryInput(input *core.CompiledQuery) *dynamodb.QueryInput {
	out := &dynamodb.QueryInput{
		TableName: aws.String(input.TableName),
	}

	if input.IndexName != "" {
		out.IndexName = aws.String(input.IndexName)
	}
	if input.KeyConditionExpression != "" {
		out.KeyConditionExpression = aws.String(input.KeyConditionExpression)
	}
	if input.FilterExpression != "" {
		out.FilterExpression = aws.String(input.FilterExpression)
	}
	if input.ProjectionExpression != "" {
		out.ProjectionExpression = aws.String(input.ProjectionExpression)
	}
	if len(input.ExpressionAttributeNames) > 0 {
		out.ExpressionAttributeNames = input.ExpressionAttributeNames
	}
	if len(input.ExpressionAttributeValues) > 0 {
		out.ExpressionAttributeValues = input.ExpressionAttributeValues
	}
	if input.Limit != nil {
		out.Limit = input.Limit
	}
	if len(input.ExclusiveStartKey) > 0 {
		out.ExclusiveStartKey = input.ExclusiveStartKey
	}
	if input.ScanIndexForward != nil {
		out.ScanIndexForward = input.ScanIndexForward
	}
	if input.ConsistentRead != nil {
		out.ConsistentRead = input.ConsistentRead
	}

	return out
}

func buildDynamoScanInput(input *core.CompiledQuery) *dynamodb.ScanInput {
	out := &dynamodb.ScanInput{
		TableName: aws.String(input.TableName),
	}

	if input.IndexName != "" {
		out.IndexName = aws.String(input.IndexName)
	}
	if input.FilterExpression != "" {
		out.FilterExpression = aws.String(input.FilterExpression)
	}
	if input.ProjectionExpression != "" {
		out.ProjectionExpression = aws.String(input.ProjectionExpression)
	}
	if len(input.ExpressionAttributeNames) > 0 {
		out.ExpressionAttributeNames = input.ExpressionAttributeNames
	}
	if len(input.ExpressionAttributeValues) > 0 {
		out.ExpressionAttributeValues = input.ExpressionAttributeValues
	}
	if input.Limit != nil {
		out.Limit = input.Limit
	}
	if len(input.ExclusiveStartKey) > 0 {
		out.ExclusiveStartKey = input.ExclusiveStartKey
	}
	if input.ConsistentRead != nil {
		out.ConsistentRead = input.ConsistentRead
	}
	if input.Segment != nil {
		out.Segment = input.Segment
	}
	if input.TotalSegments != nil {
		out.TotalSegments = input.TotalSegments
	}

	return out
}

func compiledQueryLimit(input *core.CompiledQuery) (int, bool) {
	if input == nil || input.Limit == nil {
		return 0, false
	}
	if *input.Limit <= 0 {
		return 0, true
	}
	return int(*input.Limit), true
}

func collectPaginatedItems(
	hasMorePages func() bool,
	nextPage func(context.Context) ([]map[string]types.AttributeValue, error),
	limit int,
	hasLimit bool,
	trim bool,
	ctx context.Context,
) ([]map[string]types.AttributeValue, error) {
	var items []map[string]types.AttributeValue
	for hasMorePages() {
		pageItems, err := nextPage(ctx)
		if err != nil {
			return nil, err
		}

		items = append(items, pageItems...)
		if hasLimit && len(items) >= limit {
			if trim {
				return items[:limit], nil
			}
			break
		}
	}
	return items, nil
}

func isCountSelect(selectValue string) bool {
	return selectValue == "COUNT"
}

func writeCountResult(dest any, count int64, scannedCount int64) error {
	if dest == nil {
		return fmt.Errorf("destination must be a pointer")
	}

	value := reflect.ValueOf(dest)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		return fmt.Errorf("destination must be a pointer")
	}

	elem := value.Elem()
	switch elem.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		elem.SetInt(count)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if count < 0 {
			return fmt.Errorf("count is negative")
		}
		elem.SetUint(uint64(count))
		return nil
	case reflect.Struct:
		if field := elem.FieldByName("Count"); field.IsValid() && field.CanSet() {
			setIntLike(field, count)
		}
		if field := elem.FieldByName("ScannedCount"); field.IsValid() && field.CanSet() {
			setIntLike(field, scannedCount)
		}
		return nil
	default:
		return fmt.Errorf("destination must be a pointer to an integer or struct")
	}
}

func setIntLike(field reflect.Value, value int64) {
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		field.SetInt(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if value >= 0 {
			field.SetUint(uint64(value))
		}
	}
}

func isConditionalCheckFailedException(err error) bool {
	var ccfe *types.ConditionalCheckFailedException
	return errors.As(err, &ccfe)
}

func buildKeysAndAttributes(input *query.CompiledBatchGet) types.KeysAndAttributes {
	kaa := types.KeysAndAttributes{
		Keys: input.Keys,
	}

	if input.ProjectionExpression != "" {
		expr := input.ProjectionExpression
		kaa.ProjectionExpression = &expr
	}

	if len(input.ExpressionAttributeNames) > 0 {
		kaa.ExpressionAttributeNames = input.ExpressionAttributeNames
	}

	if input.ConsistentRead {
		consistent := input.ConsistentRead
		kaa.ConsistentRead = &consistent
	}

	return kaa
}

func countUnprocessedKeys(unprocessed map[string]types.KeysAndAttributes) int {
	total := 0
	for _, entry := range unprocessed {
		total += len(entry.Keys)
	}
	return total
}

func cryptoFloat64() (float64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}

	u := binary.BigEndian.Uint64(b[:]) >> 11
	return float64(u) / (1 << 53), nil
}

func calculateBatchRetryDelay(policy *core.RetryPolicy, attempt int) time.Duration {
	if policy == nil {
		return 0
	}

	delay := policy.InitialDelay
	if delay <= 0 {
		delay = 50 * time.Millisecond
	}

	if attempt > 0 {
		delay = time.Duration(float64(delay) * math.Pow(policy.BackoffFactor, float64(attempt)))
	}

	if policy.MaxDelay > 0 && delay > policy.MaxDelay {
		delay = policy.MaxDelay
	}

	if policy.Jitter > 0 {
		if r, err := cryptoFloat64(); err == nil {
			offset := (r*2 - 1) * policy.Jitter * float64(delay)
			delay += time.Duration(offset)
		}
		if delay < 0 {
			delay = policy.InitialDelay
			if delay <= 0 {
				delay = 50 * time.Millisecond
			}
		}
	}

	return delay
}

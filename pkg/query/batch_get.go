package query

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/pay-theory/dynamorm/internal/expr"
	"github.com/pay-theory/dynamorm/pkg/core"
)

// BatchGet retrieves multiple items by their primary keys using default options.
func (q *Query) BatchGet(keys []any, dest any) error {
	return q.BatchGetWithOptions(keys, dest, nil)
}

// BatchGetWithOptions retrieves items with fine-grained control over chunking, retries, and callbacks.
func (q *Query) BatchGetWithOptions(keys []any, dest any, opts *core.BatchGetOptions) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}

	if q.metadata == nil {
		return errors.New("model metadata is required for batch get")
	}

	if len(keys) == 0 {
		return errors.New("no keys provided")
	}

	destValue := reflect.ValueOf(dest)
	if !destValue.IsValid() {
		return errors.New("dest must be a pointer to slice")
	}
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return errors.New("dest must be a pointer to slice")
	}

	executor, ok := q.executor.(BatchExecutor)
	if !ok {
		return errors.New("executor does not support batch operations")
	}

	effectiveOpts := q.normalizeBatchGetOptions(opts)

	keySpecs, err := q.convertBatchGetKeys(keys)
	if err != nil {
		return err
	}

	projectionExpr, projectionNames, err := q.buildBatchGetProjection()
	if err != nil {
		return err
	}

	consistentRead := effectiveOpts.ConsistentRead || q.consistentRead
	chunks := q.buildBatchGetChunks(keySpecs, effectiveOpts.ChunkSize, consistentRead, projectionExpr, projectionNames)

	ordered := make([]map[string]types.AttributeValue, len(keySpecs))
	var orderMu sync.Mutex

	progress := makeProgressReporter(effectiveOpts.ProgressCallback, len(keySpecs))

	processChunk := func(chunk batchGetChunk) error {
		items, execErr := executor.ExecuteBatchGet(chunk.request, effectiveOpts)
		if execErr != nil {
			if effectiveOpts.OnChunkError != nil {
				return effectiveOpts.OnChunkError(chunk.originals, execErr)
			}
			return execErr
		}

		matched := alignChunkResults(chunk.keys, items, ordered, &orderMu)
		progress(matched)
		return nil
	}

	if effectiveOpts.Parallel && effectiveOpts.MaxConcurrency > 1 {
		if err := runChunksParallel(chunks, processChunk, effectiveOpts.MaxConcurrency); err != nil {
			return err
		}
	} else {
		for _, chunk := range chunks {
			if err := processChunk(chunk); err != nil {
				return err
			}
		}
	}

	var flattened []map[string]types.AttributeValue
	for _, item := range ordered {
		if item != nil {
			flattened = append(flattened, item)
		}
	}

	if rawDest, ok := dest.(*[]map[string]types.AttributeValue); ok {
		*rawDest = append((*rawDest)[:0], flattened...)
		return nil
	}

	return UnmarshalItems(flattened, dest)
}

// BatchGetBuilder returns a fluent builder for composing advanced BatchGet operations.
func (q *Query) BatchGetBuilder() core.BatchGetBuilder {
	return &batchGetBuilder{
		query: q,
		opts:  core.DefaultBatchGetOptions(),
	}
}

type batchKeySpec struct {
	attrs    map[string]types.AttributeValue
	original any
	index    int
}

type batchGetChunk struct {
	request   *CompiledBatchGet
	keys      []batchKeySpec
	originals []any
}

func (q *Query) normalizeBatchGetOptions(opts *core.BatchGetOptions) *core.BatchGetOptions {
	userProvided := opts != nil
	if opts == nil {
		opts = core.DefaultBatchGetOptions()
	} else {
		opts = opts.Clone()
	}

	if opts.ChunkSize <= 0 || opts.ChunkSize > 100 {
		opts.ChunkSize = 100
	}

	if opts.RetryPolicy == nil {
		if !userProvided {
			opts.RetryPolicy = core.DefaultRetryPolicy()
		}
	} else {
		opts.RetryPolicy = opts.RetryPolicy.Clone()
	}

	if opts.MaxConcurrency <= 0 {
		opts.MaxConcurrency = 1
	}
	if !opts.Parallel || opts.MaxConcurrency == 1 {
		opts.Parallel = false
		opts.MaxConcurrency = 1
	}

	return opts
}

func (q *Query) convertBatchGetKeys(keys []any) ([]batchKeySpec, error) {
	specs := make([]batchKeySpec, len(keys))
	for i, key := range keys {
		attrs, err := q.buildBatchGetKey(key)
		if err != nil {
			return nil, fmt.Errorf("invalid key at index %d: %w", i, err)
		}
		specs[i] = batchKeySpec{
			attrs:    attrs,
			original: key,
			index:    i,
		}
	}
	return specs, nil
}

func (q *Query) buildBatchGetKey(key any) (map[string]types.AttributeValue, error) {
	if key == nil {
		return nil, errors.New("key cannot be nil")
	}

	switch typed := key.(type) {
	case core.KeyPair:
		return q.keyPairToAttributes(typed)
	case *core.KeyPair:
		if typed == nil {
			return nil, errors.New("key cannot be nil")
		}
		return q.keyPairToAttributes(*typed)
	case map[string]types.AttributeValue:
		if len(typed) == 0 {
			return nil, errors.New("key map cannot be empty")
		}
		return q.remapKeyAttributes(typed), nil
	case map[string]any:
		if len(typed) == 0 {
			return nil, errors.New("key map cannot be empty")
		}
		converted := make(map[string]types.AttributeValue, len(typed))
		for attr, value := range typed {
			av, err := expr.ConvertToAttributeValue(value)
			if err != nil {
				return nil, fmt.Errorf("failed to convert key attribute %s: %w", attr, err)
			}
			converted[attr] = av
		}
		return q.remapKeyAttributes(converted), nil
	default:
		if isStructLike(key) {
			raw, err := q.extractKeyAttributeValues(key)
			if err != nil {
				return nil, err
			}
			return q.remapKeyAttributes(raw), nil
		}
		return q.partitionOnlyKey(key)
	}
}

func (q *Query) keyPairToAttributes(pair core.KeyPair) (map[string]types.AttributeValue, error) {
	schema := q.metadata.PrimaryKey()
	if schema.PartitionKey == "" {
		return nil, errors.New("model is missing a partition key")
	}
	if pair.PartitionKey == nil {
		return nil, fmt.Errorf("partition key value is required for %s", schema.PartitionKey)
	}

	attrs := make(map[string]types.AttributeValue, 2)
	pk, err := expr.ConvertToAttributeValue(pair.PartitionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert partition key: %w", err)
	}
	partitionAttr := q.resolveAttributeName(schema.PartitionKey)
	attrs[partitionAttr] = pk

	if schema.SortKey != "" {
		if pair.SortKey == nil {
			return nil, fmt.Errorf("sort key value is required for %s", schema.SortKey)
		}
		sk, err := expr.ConvertToAttributeValue(pair.SortKey)
		if err != nil {
			return nil, fmt.Errorf("failed to convert sort key: %w", err)
		}
		attrs[q.resolveAttributeName(schema.SortKey)] = sk
	}

	return attrs, nil
}

func (q *Query) partitionOnlyKey(value any) (map[string]types.AttributeValue, error) {
	schema := q.metadata.PrimaryKey()
	if schema.PartitionKey == "" {
		return nil, errors.New("model is missing a partition key")
	}
	if schema.SortKey != "" {
		return nil, fmt.Errorf("composite key requires both %s and %s", schema.PartitionKey, schema.SortKey)
	}

	av, err := expr.ConvertToAttributeValue(value)
	if err != nil {
		return nil, fmt.Errorf("failed to convert partition key value: %w", err)
	}

	return map[string]types.AttributeValue{
		q.resolveAttributeName(schema.PartitionKey): av,
	}, nil
}

func isStructLike(value any) bool {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return false
	}
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return false
		}
		rv = rv.Elem()
	}
	return rv.Kind() == reflect.Struct
}

func (q *Query) buildBatchGetProjection() (string, map[string]string, error) {
	if len(q.projection) == 0 {
		return "", nil, nil
	}
	builder := expr.NewBuilder()
	builder.AddProjection(q.projection...)
	components := builder.Build()
	return components.ProjectionExpression, components.ExpressionAttributeNames, nil
}

func (q *Query) buildBatchGetChunks(specs []batchKeySpec, chunkSize int, consistent bool, projection string, projectionNames map[string]string) []batchGetChunk {
	total := (len(specs) + chunkSize - 1) / chunkSize
	chunks := make([]batchGetChunk, 0, total)

	tableName := q.metadata.TableName()

	for i := 0; i < len(specs); i += chunkSize {
		end := i + chunkSize
		if end > len(specs) {
			end = len(specs)
		}

		window := specs[i:end]
		request := &CompiledBatchGet{
			TableName:                tableName,
			Keys:                     make([]map[string]types.AttributeValue, len(window)),
			ProjectionExpression:     projection,
			ExpressionAttributeNames: projectionNames,
			ConsistentRead:           consistent,
		}

		originals := make([]any, len(window))
		for idx, spec := range window {
			request.Keys[idx] = spec.attrs
			originals[idx] = spec.original
		}

		chunks = append(chunks, batchGetChunk{
			request:   request,
			keys:      window,
			originals: originals,
		})
	}

	return chunks
}

func runChunksParallel(chunks []batchGetChunk, worker func(batchGetChunk) error, maxConcurrency int) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(chunks))
	sem := make(chan struct{}, maxConcurrency)

	for _, chunk := range chunks {
		chunkCopy := chunk
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := worker(chunkCopy); err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return nil
}

func alignChunkResults(keys []batchKeySpec, items []map[string]types.AttributeValue, ordered []map[string]types.AttributeValue, orderMu *sync.Mutex) int {
	if len(items) == 0 {
		return 0
	}

	used := make([]bool, len(items))
	matched := 0

	for _, key := range keys {
		idx := findMatchingItem(key.attrs, items, used)
		if idx < 0 {
			continue
		}

		orderMu.Lock()
		ordered[key.index] = items[idx]
		orderMu.Unlock()

		used[idx] = true
		matched++
	}

	return matched
}

func findMatchingItem(key map[string]types.AttributeValue, items []map[string]types.AttributeValue, used []bool) int {
	for i, item := range items {
		if used[i] {
			continue
		}
		if keyMatchesItem(key, item) {
			return i
		}
	}
	return -1
}

func keyMatchesItem(key map[string]types.AttributeValue, item map[string]types.AttributeValue) bool {
	for attr, expected := range key {
		actual, ok := item[attr]
		if !ok || !attributeValuesEqual(expected, actual) {
			return false
		}
	}
	return true
}

func attributeValuesEqual(a, b types.AttributeValue) bool {
	switch va := a.(type) {
	case *types.AttributeValueMemberS:
		vb, ok := b.(*types.AttributeValueMemberS)
		return ok && va.Value == vb.Value
	case *types.AttributeValueMemberN:
		vb, ok := b.(*types.AttributeValueMemberN)
		return ok && va.Value == vb.Value
	case *types.AttributeValueMemberB:
		vb, ok := b.(*types.AttributeValueMemberB)
		return ok && bytes.Equal(va.Value, vb.Value)
	default:
		return reflect.DeepEqual(a, b)
	}
}

func makeProgressReporter(cb core.BatchProgressCallback, total int) func(delta int) {
	if cb == nil {
		return func(int) {}
	}

	var mu sync.Mutex
	retrieved := 0

	return func(delta int) {
		mu.Lock()
		if delta != 0 {
			retrieved += delta
		}
		current := retrieved
		mu.Unlock()

		cb(current, total)
	}
}

type batchGetBuilder struct {
	query      *Query
	opts       *core.BatchGetOptions
	keys       []any
	projection []string
}

func (b *batchGetBuilder) Keys(keys []any) core.BatchGetBuilder {
	b.keys = keys
	return b
}

func (b *batchGetBuilder) ChunkSize(size int) core.BatchGetBuilder {
	b.opts.ChunkSize = size
	return b
}

func (b *batchGetBuilder) ConsistentRead() core.BatchGetBuilder {
	b.opts.ConsistentRead = true
	return b
}

func (b *batchGetBuilder) Parallel(maxConcurrency int) core.BatchGetBuilder {
	if maxConcurrency > 1 {
		b.opts.Parallel = true
		b.opts.MaxConcurrency = maxConcurrency
	} else {
		b.opts.Parallel = false
		b.opts.MaxConcurrency = 1
	}
	return b
}

func (b *batchGetBuilder) WithRetry(policy *core.RetryPolicy) core.BatchGetBuilder {
	if policy == nil {
		b.opts.RetryPolicy = nil
		return b
	}
	b.opts.RetryPolicy = policy.Clone()
	return b
}

func (b *batchGetBuilder) Select(fields ...string) core.BatchGetBuilder {
	if len(fields) == 0 {
		b.projection = nil
		return b
	}
	b.projection = append([]string(nil), fields...)
	return b
}

func (b *batchGetBuilder) OnProgress(callback core.BatchProgressCallback) core.BatchGetBuilder {
	b.opts.ProgressCallback = callback
	return b
}

func (b *batchGetBuilder) OnError(handler core.BatchChunkErrorHandler) core.BatchGetBuilder {
	b.opts.OnChunkError = handler
	return b
}

func (b *batchGetBuilder) Execute(dest any) error {
	if len(b.projection) > 0 {
		if next, ok := b.query.Select(b.projection...).(*Query); ok {
			b.query = next
		}
	}
	return b.query.BatchGetWithOptions(b.keys, dest, b.opts)
}
func (q *Query) remapKeyAttributes(key map[string]types.AttributeValue) map[string]types.AttributeValue {
	if len(key) == 0 {
		return key
	}
	remapped := make(map[string]types.AttributeValue, len(key))
	for field, val := range key {
		remapped[q.resolveAttributeName(field)] = val
	}
	return remapped
}

// Package query provides enhanced batch operations for DynamoDB
package query

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/internal/expr"
	"github.com/pay-theory/dynamorm/pkg/core"
)

// BatchUpdateOptions configures batch update operations
type BatchUpdateOptions struct {
	// MaxBatchSize limits items per batch (max 25 for DynamoDB)
	MaxBatchSize int
	// Parallel enables parallel batch execution
	Parallel bool
	// MaxConcurrency limits concurrent batches
	MaxConcurrency int
	// ProgressCallback is called after each batch
	ProgressCallback func(processed, total int)
	// ErrorHandler handles individual item errors
	ErrorHandler func(item any, err error) error
	// RetryPolicy defines retry behavior
	RetryPolicy *RetryPolicy
}

// RetryPolicy defines retry behavior for batch operations
type RetryPolicy struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
}

// DefaultBatchOptions returns default batch options
func DefaultBatchOptions() *BatchUpdateOptions {
	return &BatchUpdateOptions{
		MaxBatchSize:   25,
		Parallel:       false,
		MaxConcurrency: 5,
		RetryPolicy: &RetryPolicy{
			MaxRetries:    3,
			InitialDelay:  100 * time.Millisecond,
			MaxDelay:      5 * time.Second,
			BackoffFactor: 2.0,
		},
	}
}

// BatchUpdate performs batch update operations
func (q *Query) BatchUpdate(items any, fields ...string) error {
	return q.batchUpdateWithOptionsInternal(items, DefaultBatchOptions(), fields...)
}

// BatchUpdateWithOptions implements core.Query interface with the expected signature
func (q *Query) BatchUpdateWithOptions(items []any, fields []string, options ...any) error {
	// Default options
	opts := DefaultBatchOptions()

	// Override with provided options if any
	if len(options) > 0 {
		if batchOpts, ok := options[0].(*BatchUpdateOptions); ok {
			opts = batchOpts
		}
	}

	// Delegate to the internal implementation
	return q.batchUpdateWithOptionsInternal(items, opts, fields...)
}

// batchUpdateWithOptionsInternal is the actual implementation with the internal signature
func (q *Query) batchUpdateWithOptionsInternal(items any, opts *BatchUpdateOptions, fields ...string) error {
	// Validate input
	itemsValue := reflect.ValueOf(items)
	if itemsValue.Kind() != reflect.Slice {
		return fmt.Errorf("items must be a slice")
	}

	if itemsValue.Len() == 0 {
		return nil
	}

	// Prepare batches
	batches := q.prepareBatches(itemsValue, opts.MaxBatchSize)
	totalItems := itemsValue.Len()
	processed := 0

	// Execute batches
	if opts.Parallel {
		return q.executeBatchesParallel(batches, opts, fields, &processed, totalItems)
	}

	return q.executeBatchesSequential(batches, opts, fields, &processed, totalItems)
}

// BatchDelete performs batch delete operations
func (q *Query) BatchDelete(keys []any) error {
	return q.BatchDeleteWithOptions(keys, DefaultBatchOptions())
}

// BatchDeleteWithOptions performs batch delete with custom options
func (q *Query) BatchDeleteWithOptions(keys []any, opts *BatchUpdateOptions) error {
	if len(keys) == 0 {
		return nil
	}

	// Prepare key batches
	batches := q.prepareKeyBatches(keys, opts.MaxBatchSize)
	totalItems := len(keys)
	processed := 0

	// Execute delete batches
	for _, batch := range batches {
		err := q.executeDeleteBatch(batch, opts)
		if err != nil {
			if opts.ErrorHandler != nil {
				if handlerErr := opts.ErrorHandler(batch, err); handlerErr != nil {
					return handlerErr
				}
			} else {
				return err
			}
		}

		processed += len(batch)
		if opts.ProgressCallback != nil {
			opts.ProgressCallback(processed, totalItems)
		}
	}

	return nil
}

// prepareBatches splits items into batches
func (q *Query) prepareBatches(items reflect.Value, batchSize int) [][]any {
	if batchSize <= 0 || batchSize > 25 {
		batchSize = 25
	}

	totalItems := items.Len()
	numBatches := (totalItems + batchSize - 1) / batchSize
	batches := make([][]any, numBatches)

	for i := 0; i < numBatches; i++ {
		start := i * batchSize
		end := start + batchSize
		if end > totalItems {
			end = totalItems
		}

		batch := make([]any, end-start)
		for j := start; j < end; j++ {
			batch[j-start] = items.Index(j).Interface()
		}
		batches[i] = batch
	}

	return batches
}

// prepareKeyBatches splits keys into batches
func (q *Query) prepareKeyBatches(keys []any, batchSize int) [][]any {
	if batchSize <= 0 || batchSize > 25 {
		batchSize = 25
	}

	totalKeys := len(keys)
	numBatches := (totalKeys + batchSize - 1) / batchSize
	batches := make([][]any, numBatches)

	for i := 0; i < numBatches; i++ {
		start := i * batchSize
		end := start + batchSize
		if end > totalKeys {
			end = totalKeys
		}
		batches[i] = keys[start:end]
	}

	return batches
}

// executeBatchesSequential executes batches one by one
func (q *Query) executeBatchesSequential(batches [][]any, opts *BatchUpdateOptions, fields []string, processed *int, total int) error {
	for _, batch := range batches {
		err := q.executeUpdateBatch(batch, opts, fields)
		if err != nil {
			if opts.ErrorHandler != nil {
				if handlerErr := opts.ErrorHandler(batch, err); handlerErr != nil {
					return handlerErr
				}
			} else {
				return err
			}
		}

		*processed += len(batch)
		if opts.ProgressCallback != nil {
			opts.ProgressCallback(*processed, total)
		}
	}

	return nil
}

// executeBatchesParallel executes batches concurrently
func (q *Query) executeBatchesParallel(batches [][]any, opts *BatchUpdateOptions, fields []string, processed *int, total int) error {
	if opts.MaxConcurrency <= 0 {
		opts.MaxConcurrency = 5
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, opts.MaxConcurrency)
	errChan := make(chan error, len(batches))
	progressMutex := &sync.Mutex{}

	for _, batch := range batches {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore

		go func(b []any) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore

			err := q.executeUpdateBatch(b, opts, fields)
			if err != nil {
				if opts.ErrorHandler != nil {
					if handlerErr := opts.ErrorHandler(b, err); handlerErr != nil {
						errChan <- handlerErr
						return
					}
				} else {
					errChan <- err
					return
				}
			}

			// Update progress
			progressMutex.Lock()
			*processed += len(b)
			currentProgress := *processed
			progressMutex.Unlock()

			if opts.ProgressCallback != nil {
				opts.ProgressCallback(currentProgress, total)
			}
		}(batch)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

// executeUpdateBatch executes a single update batch
func (q *Query) executeUpdateBatch(batch []any, opts *BatchUpdateOptions, fields []string) error {
	// In DynamoDB, we need to use TransactWrite for batch updates
	// or individual UpdateItem calls

	for _, item := range batch {
		// Extract key from item
		key, err := q.extractKey(item)
		if err != nil {
			return fmt.Errorf("failed to extract key: %w", err)
		}

		// Build update expression
		updateBuilder := &UpdateBuilder{
			query:      q,
			expr:       expr.NewBuilder(),
			keyValues:  key,
			conditions: []updateCondition{},
		}

		// Update specified fields
		itemValue := reflect.ValueOf(item)
		if itemValue.Kind() == reflect.Ptr {
			itemValue = itemValue.Elem()
		}

		for _, field := range fields {
			fieldValue := itemValue.FieldByName(field)
			if fieldValue.IsValid() {
				updateBuilder.Set(field, fieldValue.Interface())
			}
		}

		// Execute with retry
		err = q.executeWithRetry(func() error {
			return updateBuilder.Execute()
		}, opts.RetryPolicy)

		if err != nil {
			return err
		}
	}

	return nil
}

// executeDeleteBatch executes a single delete batch
func (q *Query) executeDeleteBatch(batch []any, opts *BatchUpdateOptions) error {
	// Use BatchWriteItem for batch deletes
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

	// Use the new executeBatchWriteWithRetries function for better retry handling
	return q.executeBatchWriteWithRetries(q.metadata.TableName(), writeRequests, opts)
}

// extractKey extracts primary key values from an item
func (q *Query) extractKey(item any) (map[string]any, error) {
	key := make(map[string]any)
	primaryKey := q.metadata.PrimaryKey()

	itemValue := reflect.ValueOf(item)
	if itemValue.Kind() == reflect.Ptr {
		itemValue = itemValue.Elem()
	}

	// Extract partition key
	pkField := itemValue.FieldByName(primaryKey.PartitionKey)
	if !pkField.IsValid() {
		return nil, fmt.Errorf("partition key field %s not found", primaryKey.PartitionKey)
	}
	key[primaryKey.PartitionKey] = pkField.Interface()

	// Extract sort key if present
	if primaryKey.SortKey != "" {
		skField := itemValue.FieldByName(primaryKey.SortKey)
		if !skField.IsValid() {
			return nil, fmt.Errorf("sort key field %s not found", primaryKey.SortKey)
		}
		key[primaryKey.SortKey] = skField.Interface()
	}

	return key, nil
}

// extractKeyAttributeValues converts key to AttributeValues
func (q *Query) extractKeyAttributeValues(key any) (map[string]types.AttributeValue, error) {
	keyMap, err := q.extractKey(key)
	if err != nil {
		return nil, err
	}

	keyAV := make(map[string]types.AttributeValue)
	for k, v := range keyMap {
		av, err := expr.ConvertToAttributeValue(v)
		if err != nil {
			return nil, fmt.Errorf("failed to convert key value: %w", err)
		}
		keyAV[k] = av
	}

	return keyAV, nil
}

// executeWithRetry executes a function with retry logic
func (q *Query) executeWithRetry(fn func() error, policy *RetryPolicy) error {
	if policy == nil {
		return fn()
	}

	var lastErr error
	delay := policy.InitialDelay

	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			return err
		}

		if attempt < policy.MaxRetries {
			time.Sleep(delay)
			delay = time.Duration(float64(delay) * policy.BackoffFactor)
			if delay > policy.MaxDelay {
				delay = policy.MaxDelay
			}
		}
	}

	return fmt.Errorf("operation failed after %d retries: %w", policy.MaxRetries, lastErr)
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for common retryable DynamoDB errors
	errStr := err.Error()
	retryableErrors := []string{
		"ProvisionedThroughputExceededException",
		"ThrottlingException",
		"InternalServerError",
		"ServiceUnavailable",
		"RequestLimitExceeded",
	}

	for _, retryable := range retryableErrors {
		if contains(errStr, retryable) {
			return true
		}
	}

	return false
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	if substr == "" {
		return false
	}
	return len(s) >= len(substr) && s != "" && (s == substr || contains(s[1:], substr) || (len(s) >= len(substr) && s[:len(substr)] == substr))
}

// BatchResult represents the result of a batch operation
type BatchResult struct {
	Succeeded       int
	Failed          int
	UnprocessedKeys []any
	Errors          []error
}

// BatchCreateWithResult creates multiple items and returns detailed results
func (q *Query) BatchCreateWithResult(items any) (*BatchResult, error) {
	opts := DefaultBatchOptions()
	result := &BatchResult{
		Errors: make([]error, 0),
	}

	// Custom error handler to collect results
	opts.ErrorHandler = func(item any, err error) error {
		result.Failed++
		result.Errors = append(result.Errors, err)
		// Don't stop on error, continue processing
		return nil
	}

	// Custom progress callback to track success
	opts.ProgressCallback = func(processed, total int) {
		result.Succeeded = processed - result.Failed
	}

	err := q.BatchCreate(items)
	return result, err
}

// QueryTimeout sets a timeout for the query execution
func (q *Query) QueryTimeout(timeout time.Duration) core.Query {
	// This would need to be integrated with context handling
	ctx, cancel := context.WithTimeout(q.ctx, timeout)
	q.ctx = ctx
	// Store cancel function for cleanup
	// In a full implementation, this would be properly managed
	_ = cancel
	return q
}

// QueryCancel provides a way to cancel long-running queries
type QueryCanceler struct {
	cancel context.CancelFunc
}

// WithCancellation returns a query that can be cancelled
func (q *Query) WithCancellation() (core.Query, *QueryCanceler) {
	ctx, cancel := context.WithCancel(q.ctx)
	q.ctx = ctx
	return q, &QueryCanceler{cancel: cancel}
}

// Cancel cancels the query execution
func (qc *QueryCanceler) Cancel() {
	if qc.cancel != nil {
		qc.cancel()
	}
}

// executeBatchWriteWithRetries executes batch write operations with automatic retry for unprocessed items
func (q *Query) executeBatchWriteWithRetries(tableName string, writeRequests []types.WriteRequest, opts *BatchUpdateOptions) error {
	if len(writeRequests) == 0 {
		return nil
	}

	batchExecutor, ok := q.executor.(BatchWriteItemExecutor)
	if !ok {
		return fmt.Errorf("executor does not support batch write operations")
	}

	remainingRequests := writeRequests
	attempts := 0
	maxAttempts := 5 // Maximum number of attempts for unprocessed items

	for len(remainingRequests) > 0 && attempts < maxAttempts {
		attempts++

		// Execute batch write
		result, err := batchExecutor.ExecuteBatchWriteItem(tableName, remainingRequests)
		if err != nil {
			return fmt.Errorf("batch write failed: %w", err)
		}

		// Check for unprocessed items
		if len(result.UnprocessedItems) == 0 {
			// All items processed successfully
			return nil
		}

		// Collect unprocessed items for retry
		var unprocessed []types.WriteRequest
		for _, items := range result.UnprocessedItems {
			unprocessed = append(unprocessed, items...)
		}

		if len(unprocessed) == 0 {
			return nil
		}

		// Log or callback for unprocessed items
		if opts != nil && opts.ProgressCallback != nil {
			processed := len(writeRequests) - len(unprocessed)
			opts.ProgressCallback(processed, len(writeRequests))
		}

		// Exponential backoff before retry
		if attempts < maxAttempts {
			backoffTime := time.Duration(attempts) * 100 * time.Millisecond
			if backoffTime > 2*time.Second {
				backoffTime = 2 * time.Second
			}
			time.Sleep(backoffTime)
		}

		remainingRequests = unprocessed
	}

	if len(remainingRequests) > 0 {
		return fmt.Errorf("failed to process %d items after %d attempts", len(remainingRequests), attempts)
	}

	return nil
}

// BatchWrite performs mixed batch write operations (puts and deletes)
func (q *Query) BatchWrite(putItems []any, deleteKeys []any) error {
	return q.BatchWriteWithOptions(putItems, deleteKeys, DefaultBatchOptions())
}

// BatchWriteWithOptions performs mixed batch write operations with custom options
func (q *Query) BatchWriteWithOptions(putItems []any, deleteKeys []any, opts *BatchUpdateOptions) error {
	totalItems := len(putItems) + len(deleteKeys)
	if totalItems == 0 {
		return nil
	}

	// Validate batch size
	if opts.MaxBatchSize <= 0 || opts.MaxBatchSize > 25 {
		opts.MaxBatchSize = 25
	}

	// Prepare write requests
	var allRequests []types.WriteRequest

	// Add put requests
	for _, item := range putItems {
		itemAV, err := convertItemToAttributeValue(item)
		if err != nil {
			if opts.ErrorHandler != nil {
				if handlerErr := opts.ErrorHandler(item, err); handlerErr != nil {
					return handlerErr
				}
				continue
			}
			return fmt.Errorf("failed to marshal item: %w", err)
		}

		allRequests = append(allRequests, types.WriteRequest{
			PutRequest: &types.PutRequest{
				Item: itemAV,
			},
		})
	}

	// Add delete requests
	for _, key := range deleteKeys {
		keyAV, err := q.extractKeyAttributeValues(key)
		if err != nil {
			if opts.ErrorHandler != nil {
				if handlerErr := opts.ErrorHandler(key, err); handlerErr != nil {
					return handlerErr
				}
				continue
			}
			return fmt.Errorf("failed to extract key: %w", err)
		}

		allRequests = append(allRequests, types.WriteRequest{
			DeleteRequest: &types.DeleteRequest{
				Key: keyAV,
			},
		})
	}

	// Split into batches
	batches := q.splitWriteRequests(allRequests, opts.MaxBatchSize)

	// Execute batches
	processed := 0
	for _, batch := range batches {
		err := q.executeBatchWriteWithRetries(q.metadata.TableName(), batch, opts)
		if err != nil {
			if opts.ErrorHandler != nil {
				if handlerErr := opts.ErrorHandler(batch, err); handlerErr != nil {
					return handlerErr
				}
			} else {
				return err
			}
		}

		processed += len(batch)
		if opts.ProgressCallback != nil {
			opts.ProgressCallback(processed, totalItems)
		}
	}

	return nil
}

// splitWriteRequests splits write requests into batches
func (q *Query) splitWriteRequests(requests []types.WriteRequest, batchSize int) [][]types.WriteRequest {
	if batchSize <= 0 || batchSize > 25 {
		batchSize = 25
	}

	var batches [][]types.WriteRequest
	for i := 0; i < len(requests); i += batchSize {
		end := i + batchSize
		if end > len(requests) {
			end = len(requests)
		}
		batches = append(batches, requests[i:end])
	}

	return batches
}

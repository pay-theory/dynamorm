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
	return q.BatchUpdateWithOptions(items, DefaultBatchOptions(), fields...)
}

// BatchUpdateWithOptions performs batch update with custom options
func (q *Query) BatchUpdateWithOptions(items any, opts *BatchUpdateOptions, fields ...string) error {
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

	// Execute batch write with retry
	return q.executeWithRetry(func() error {
		// This would need to be implemented through the executor
		// For now, return an error indicating the feature needs implementation
		_ = writeRequests // TODO: Use writeRequests when executor implementation is complete
		return fmt.Errorf("batch delete execution not yet implemented in executor")
	}, opts.RetryPolicy)
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

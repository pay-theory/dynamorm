# Batch Operations Guide

This guide covers the batch operations features in DynamORM, including batch create, batch get, batch delete, and mixed batch write operations.

## Table of Contents
- [Overview](#overview)
- [Batch Create](#batch-create)
- [Batch Get](#batch-get)
- [Batch Delete](#batch-delete)
- [Mixed Batch Operations](#mixed-batch-operations)
- [Advanced Options](#advanced-options)
- [Error Handling](#error-handling)
- [Performance Considerations](#performance-considerations)

## Overview

Batch operations in DynamORM allow you to perform multiple DynamoDB operations efficiently in a single request. DynamoDB has a limit of 25 items per batch operation, and DynamORM handles this automatically with support for:

- Automatic batching of larger sets
- Retry logic for unprocessed items
- Parallel execution for performance
- Progress tracking and error handling

## Batch Create

Create multiple items in a single operation:

```go
// Create up to 25 items in one request
items := []Product{
    {ID: "prod1", Name: "Product 1", Price: 99.99},
    {ID: "prod2", Name: "Product 2", Price: 149.99},
    {ID: "prod3", Name: "Product 3", Price: 199.99},
}

err := db.Model(&Product{}).BatchCreate(items)
if err != nil {
    log.Fatal(err)
}
```

### Limitations
- Maximum 25 items per BatchCreate call
- For larger sets, use batch operations with custom options (see Advanced Options)

## Batch Get

Retrieve multiple items by their keys:

```go
// Define the keys to retrieve
keys := []any{
    Product{ID: "prod1"},
    Product{ID: "prod2"},
    Product{ID: "prod3"},
}

var results []Product
err := db.Model(&Product{}).BatchGet(keys, &results)
if err != nil {
    log.Fatal(err)
}

// results now contains the retrieved products
for _, product := range results {
    fmt.Printf("Product: %s - $%.2f\n", product.Name, product.Price)
}
```

### Batch Get with Composite Keys

For tables with composite keys (partition key + sort key):

```go
keys := []any{
    Order{UserID: "user1", OrderID: "order1"},
    Order{UserID: "user1", OrderID: "order2"},
    Order{UserID: "user2", OrderID: "order1"},
}

var orders []Order
err := db.Model(&Order{}).BatchGet(keys, &orders)
```

## Batch Delete

Delete multiple items efficiently:

```go
// Using the query package for advanced batch operations
import "github.com/pay-theory/dynamorm/pkg/query"

// Create a query instance
q := query.New(&Product{}, metadata, executor)

// Define keys to delete
deleteKeys := []any{
    Product{ID: "prod1"},
    Product{ID: "prod2"},
    Product{ID: "prod3"},
}

err := q.BatchDelete(deleteKeys)
if err != nil {
    log.Fatal(err)
}
```

### Batch Delete with Options

```go
opts := &query.BatchUpdateOptions{
    MaxBatchSize: 25,
    ProgressCallback: func(processed, total int) {
        fmt.Printf("Deleted %d/%d items\n", processed, total)
    },
    ErrorHandler: func(item any, err error) error {
        log.Printf("Failed to delete item: %v, error: %v\n", item, err)
        return nil // Continue processing
    },
}

err := q.BatchDeleteWithOptions(deleteKeys, opts)
```

## Mixed Batch Operations

Perform puts and deletes in a single batch operation:

```go
// Items to create/update
putItems := []any{
    Product{ID: "new1", Name: "New Product 1", Price: 299.99},
    Product{ID: "new2", Name: "New Product 2", Price: 399.99},
}

// Items to delete
deleteKeys := []any{
    Product{ID: "old1"},
    Product{ID: "old2"},
}

// Execute mixed batch write
err := q.BatchWrite(putItems, deleteKeys)
if err != nil {
    log.Fatal(err)
}
```

## Advanced Options

### Batch Update with Custom Options

```go
opts := &query.BatchUpdateOptions{
    MaxBatchSize:   10,    // Smaller batches
    Parallel:       true,  // Enable parallel execution
    MaxConcurrency: 4,     // Limit concurrent batches
    
    // Retry configuration
    RetryPolicy: &query.RetryPolicy{
        MaxRetries:    3,
        InitialDelay:  100 * time.Millisecond,
        MaxDelay:      5 * time.Second,
        BackoffFactor: 2.0,
    },
    
    // Progress tracking
    ProgressCallback: func(processed, total int) {
        percentage := float64(processed) / float64(total) * 100
        fmt.Printf("Progress: %.1f%% (%d/%d)\n", percentage, processed, total)
    },
    
    // Error handling
    ErrorHandler: func(item any, err error) error {
        // Log error but continue processing
        log.Printf("Error processing item %v: %v\n", item, err)
        return nil
    },
}

// Batch update specific fields
items := getItemsToUpdate() // Your items
err := q.BatchUpdateWithOptions(items, opts, "Status", "UpdatedAt")
```

### Batch Operations with Results

Get detailed results from batch operations:

```go
result, err := q.BatchCreateWithResult(items)
if err != nil {
    log.Printf("Batch create completed with errors: %v\n", err)
}

fmt.Printf("Successfully created: %d\n", result.Succeeded)
fmt.Printf("Failed: %d\n", result.Failed)
for _, err := range result.Errors {
    log.Printf("Error: %v\n", err)
}
```

## Error Handling

### Handling Unprocessed Items

DynamoDB may not process all items in a batch due to capacity limits. DynamORM automatically retries unprocessed items:

```go
// The executeBatchWriteWithRetries function handles this automatically
// It will retry up to 5 times with exponential backoff

// For manual control, use custom error handlers:
opts := &query.BatchUpdateOptions{
    ErrorHandler: func(item any, err error) error {
        if strings.Contains(err.Error(), "unprocessed") {
            // Custom retry logic
            return retryItem(item)
        }
        return err // Stop processing on other errors
    },
}
```

### Common Error Scenarios

1. **Provisioned Throughput Exceeded**
   ```go
   // Handled automatically with retry logic
   // Exponential backoff between retries
   ```

2. **Validation Errors**
   ```go
   opts.ErrorHandler = func(item any, err error) error {
       if strings.Contains(err.Error(), "ValidationException") {
           log.Printf("Invalid item: %v\n", item)
           return nil // Skip invalid item
       }
       return err
   }
   ```

3. **Partial Failures**
   ```go
   // Track failures for later processing
   var failedItems []any
   
   opts.ErrorHandler = func(item any, err error) error {
       failedItems = append(failedItems, item)
       return nil // Continue with other items
   }
   
   // Process failed items later
   if len(failedItems) > 0 {
       // Retry failed items with different strategy
   }
   ```

## Performance Considerations

### Parallel vs Sequential Execution

```go
// Sequential (default)
seqOpts := &query.BatchUpdateOptions{
    MaxBatchSize: 25,
    Parallel:     false,
}

// Parallel - better for large datasets
parOpts := &query.BatchUpdateOptions{
    MaxBatchSize:   25,
    Parallel:       true,
    MaxConcurrency: 10, // Adjust based on your capacity
}

// Benchmark to find optimal settings
start := time.Now()
err := q.BatchUpdateWithOptions(items, parOpts, fields...)
fmt.Printf("Parallel execution took: %v\n", time.Since(start))
```

### Best Practices

1. **Batch Size Optimization**
   - Use the maximum batch size (25) for best throughput
   - Smaller batches only if you have specific throttling concerns

2. **Parallel Execution**
   - Enable for large datasets (> 100 items)
   - Set MaxConcurrency based on your table's provisioned capacity
   - Monitor CloudWatch metrics for throttling

3. **Error Handling**
   - Always implement error handlers for production use
   - Log failed items for analysis
   - Consider dead letter queues for persistent failures

4. **Memory Usage**
   - Be mindful of memory when processing large batches
   - Stream results when possible
   - Use pagination for very large datasets

### Example: Processing Large Dataset

```go
// Process 1000 items efficiently
func processManyItems(db *dynamorm.DB, items []Product) error {
    // Split into chunks of 25
    for i := 0; i < len(items); i += 25 {
        end := i + 25
        if end > len(items) {
            end = len(items)
        }
        
        batch := items[i:end]
        err := db.Model(&Product{}).BatchCreate(batch)
        if err != nil {
            return fmt.Errorf("batch %d failed: %w", i/25, err)
        }
        
        // Optional: Add delay to avoid throttling
        if i+25 < len(items) {
            time.Sleep(100 * time.Millisecond)
        }
    }
    
    return nil
}
```

## Conclusion

Batch operations in DynamORM provide a powerful way to work with multiple items efficiently. Key takeaways:

- Use batch operations for bulk creates, reads, updates, and deletes
- Respect the 25-item limit per batch
- Implement proper error handling for production use
- Consider parallel execution for large datasets
- Monitor your DynamoDB metrics to optimize performance

For more examples, see the [integration tests](../../tests/integration/batch_operations_test.go). 
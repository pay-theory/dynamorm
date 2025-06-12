# DynamORM Performance Optimization Implementation Guide

## Context

DynamORM is currently ~5x slower than direct AWS SDK calls for simple primary key lookups. Benchmarks show:
- AWS SDK: ~515,408 ns/op (0.5ms) with 559 allocations
- DynamORM: ~2,566,280 ns/op (2.5ms) with 2,416 allocations

The root cause is that DynamORM doesn't properly recognize primary key queries and falls back to the more expensive Query operation instead of using GetItem.

## Optimization Tasks (Priority Order)

### Task 1: Fix Primary Key Recognition [CRITICAL - 80% improvement]

**Problem**: In `dynamorm.go`, the `extractPrimaryKey` function only checks Go field names, not DynamoDB attribute names.

**Files to modify**: `dynamorm.go`

**Changes needed**:

1. Locate the `extractPrimaryKey` function (around line 1004)
2. Replace the existing function with:

```go
func (q *query) extractPrimaryKey(metadata *model.Metadata) map[string]any {
	pk := make(map[string]any)

	// First try to extract from conditions
	for _, cond := range q.conditions {
		if cond.op != "=" {
			continue
		}

		// Check by Go field name first
		if field, exists := metadata.Fields[cond.field]; exists {
			if field.IsPK {
				pk["pk"] = cond.value
			} else if field.IsSK {
				pk["sk"] = cond.value
			}
		} else {
			// NEW: Also check by DynamoDB attribute name
			if field, exists := metadata.FieldsByDBName[cond.field]; exists {
				if field.IsPK {
					pk["pk"] = cond.value
				} else if field.IsSK {
					pk["sk"] = cond.value
				}
			}
		}
	}

	// If no primary key found in conditions, try to extract from model
	if _, hasPK := pk["pk"]; !hasPK && q.model != nil {
		modelValue := reflect.ValueOf(q.model)
		if modelValue.Kind() == reflect.Ptr {
			modelValue = modelValue.Elem()
		}

		// Extract primary key from model
		if metadata.PrimaryKey.PartitionKey != nil {
			pkField := modelValue.FieldByIndex([]int{metadata.PrimaryKey.PartitionKey.Index})
			if !pkField.IsZero() {
				pk["pk"] = pkField.Interface()
			}
		}

		// Extract sort key from model if exists
		if metadata.PrimaryKey.SortKey != nil {
			skField := modelValue.FieldByIndex([]int{metadata.PrimaryKey.SortKey.Index})
			if !skField.IsZero() {
				pk["sk"] = skField.Interface()
			}
		}
	}

	// Must have at least partition key
	if _, hasPK := pk["pk"]; !hasPK {
		return nil
	}

	return pk
}
```

### Task 2: Add Metadata Fast-Path Cache [10-15% improvement]

**Files to modify**: `dynamorm.go`

**Changes needed**:

1. Add to the `DB` struct (around line 29):
```go
type DB struct {
	session             *session.Session
	registry            *model.Registry
	converter           *pkgTypes.Converter
	marshaler           *marshal.Marshaler
	ctx                 context.Context
	mu                  sync.RWMutex
	lambdaDeadline      time.Time
	lambdaTimeoutBuffer time.Duration
	metadataCache       sync.Map // NEW: Add this field - type -> *model.Metadata
}
```

2. Update the `Model` method (around line 63):
```go
func (db *DB) Model(model any) core.Query {
	// Fast-path metadata lookup
	var metadata *model.Metadata
	typ := reflect.TypeOf(model)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	
	// Check cache first
	if cached, ok := db.metadataCache.Load(typ); ok {
		metadata = cached.(*model.Metadata)
	} else {
		// Get from registry and cache
		meta, err := db.registry.GetMetadata(model)
		if err != nil {
			return &errorQuery{err: err}
		}
		metadata = meta
		db.metadataCache.Store(typ, metadata)
	}

	return &query{
		db:         db,
		model:      model,
		ctx:        db.ctx,
		conditions: make([]condition, 0),
	}
}
```

### Task 3: Optimize GetItem Operation [5-10% improvement]

**Files to modify**: `dynamorm.go`

**Changes needed**:

1. Add a new optimized `getItemDirect` method after the existing `getItem` method:

```go
// getItemDirect performs a direct GetItem without expression builder overhead
func (q *query) getItemDirect(metadata *model.Metadata, pk map[string]any, dest any) error {
	// Pre-allocate with exact size
	keyMap := make(map[string]types.AttributeValue, 2)
	
	// Direct conversion without error handling in hot path
	if pkValue, hasPK := pk["pk"]; hasPK {
		if av, err := q.db.converter.ToAttributeValue(pkValue); err == nil {
			keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
		} else {
			return fmt.Errorf("failed to convert partition key: %w", err)
		}
	}
	
	if skValue, hasSK := pk["sk"]; hasSK && metadata.PrimaryKey.SortKey != nil {
		if av, err := q.db.converter.ToAttributeValue(skValue); err == nil {
			keyMap[metadata.PrimaryKey.SortKey.DBName] = av
		} else {
			return fmt.Errorf("failed to convert sort key: %w", err)
		}
	}
	
	// Direct API call
	output, err := q.db.session.Client().GetItem(q.ctx, &dynamodb.GetItemInput{
		TableName: aws.String(metadata.TableName),
		Key:       keyMap,
	})
	
	if err != nil {
		return fmt.Errorf("failed to get item: %w", err)
	}
	
	if output.Item == nil {
		return customerrors.ErrItemNotFound
	}
	
	return q.unmarshalItem(output.Item, dest, metadata)
}
```

2. Update the `First` method to use `getItemDirect` for simple cases:
```go
func (q *query) First(dest any) error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get model metadata (use cached version if available)
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Build GetItem request if we have a primary key condition
	if pk := q.extractPrimaryKey(metadata); pk != nil {
		// Use optimized path when no projections are specified
		if len(q.fields) == 0 {
			return q.getItemDirect(metadata, pk, dest)
		}
		return q.getItem(metadata, pk, dest)
	}

	// ... rest of the existing First method
}
```

### Task 4: Reduce Allocations in Where Clauses

**Files to modify**: `dynamorm.go`

**Changes needed**:

1. Pre-allocate the conditions slice in the query struct initialization:
```go
func (db *DB) Model(model any) core.Query {
	// ... existing metadata lookup code ...
	
	return &query{
		db:         db,
		model:      model,
		ctx:        db.ctx,
		conditions: make([]condition, 0, 4), // Pre-allocate for typical use case
	}
}
```

### Task 5: Add Benchmarks for Verification

**Files to create**: `dynamorm_bench_test.go` in the root directory

**Content**:
```go
package dynamorm

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/session"
)

type BenchmarkModel struct {
	ID        string    `dynamorm:"pk,attr:id"`
	Name      string    `dynamorm:"attr:name"`
	CreatedAt time.Time `dynamorm:"attr:created_at"`
}

func (b BenchmarkModel) TableName() string {
	return "benchmark_table"
}

func BenchmarkGetItemDirect(b *testing.B) {
	// Setup mock or local DynamoDB
	db, _ := NewBasic(session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	
	model := &BenchmarkModel{ID: "test-id"}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result BenchmarkModel
		_ = db.Model(model).Where("id", "=", "test-id").First(&result)
	}
}

func BenchmarkGetItemByAttribute(b *testing.B) {
	// Test querying by DynamoDB attribute name
	db, _ := NewBasic(session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result BenchmarkModel
		_ = db.Model(&BenchmarkModel{}).Where("id", "=", "test-id").First(&result)
	}
}
```

## Testing Instructions

1. **Unit Tests**: Run existing tests to ensure no regressions:
   ```bash
   go test ./...
   ```

2. **Benchmark Comparison**: 
   - Run benchmarks before implementing changes:
     ```bash
     go test -bench=. -benchmem -benchtime=10s > before.txt
     ```
   - Run benchmarks after each optimization:
     ```bash
     go test -bench=. -benchmem -benchtime=10s > after.txt
     ```
   - Compare results:
     ```bash
     benchstat before.txt after.txt
     ```

3. **Integration Test**: Test with the bin-lookup-service example:
   ```go
   // This query should now use GetItem instead of Query
   db.Model(&BinRecord{}).Where("card_bin", "=", "411111").Where("card_bin_extended", "=", ";").First(&record)
   ```

## Verification Checklist

- [ ] Primary key queries use GetItem (verify with AWS SDK debug logging)
- [ ] Metadata cache reduces registry lookups (add logging to verify)
- [ ] Memory allocations reduced by at least 50%
- [ ] Query performance within 2x of raw AWS SDK
- [ ] All existing tests pass
- [ ] No breaking changes to public API

## Additional Notes

- The `extractPrimaryKey` fix is the most critical change - implement it first
- Test each optimization independently to measure its impact
- Consider adding debug logging temporarily to verify GetItem is being used
- The optimizations maintain backward compatibility

## Expected Results

After implementing all optimizations:
- Simple GetItem queries: ~600,000-800,000 ns/op (from ~2,500,000)
- Memory allocations: ~800-1000 per op (from ~2,400)
- Performance gap vs AWS SDK: 1.2-1.5x (from 5x)
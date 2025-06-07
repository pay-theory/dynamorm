# Team 1: Session 2 - Integration & AWS Operations

## Excellent Progress! 🎉

Your team has successfully built the core foundation of DynamORM. The model registry, type system, and basic query structure are all in place. Now it's time to integrate with Team 2's work and implement the actual DynamoDB operations.

## Session 2 Objectives

### 1. Integration with Team 2's Expression Builder

Team 2 has implemented the expression builder in `internal/expr/`. Your task is to:

1. **Complete the Query Execution Methods** in `dynamorm.go`:
   ```go
   func (q *query) getItem(metadata *model.Metadata, pk map[string]interface{}, dest interface{}) error {
       // Use Team 2's expression builder to create the request
       // Use AWS SDK to execute GetItem
       // Use your type converter to unmarshal results
   }
   
   func (q *query) putItem(metadata *model.Metadata) error {
       // Marshal the model using your type converter
       // Handle special fields (created_at, updated_at, version)
       // Execute PutItem
   }
   
   func (q *query) updateItem(metadata *model.Metadata, fields []string) error {
       // Build update expression using Team 2's builder
       // Handle version increment for optimistic locking
       // Execute UpdateItem
   }
   
   func (q *query) deleteItem(metadata *model.Metadata) error {
       // Build key from conditions
       // Execute DeleteItem
   }
   ```

2. **Implement Query/Scan Operations**:
   ```go
   func (q *query) All(dest interface{}) error {
       // Work with Team 2 to use their query compilation
       // Determine Query vs Scan
       // Handle pagination
       // Unmarshal results to slice
   }
   ```

### 2. Schema Management (`pkg/schema/`)

Create the schema package to handle table operations:

```go
package schema

type Manager struct {
    session *session.Session
    registry *model.Registry
}

func (m *Manager) CreateTable(model interface{}) error {
    // Extract metadata from registry
    // Build CreateTableInput
    // Handle indexes (GSI/LSI)
    // Set billing mode, throughput
}

func (m *Manager) UpdateTable(model interface{}) error {
    // Compare current schema with desired
    // Update indexes if needed
}

func (m *Manager) EnsureTable(model interface{}) error {
    // Check if table exists
    // Create if not, update if schema changed
}
```

### 3. Transaction Support (`pkg/transaction/`)

Implement proper transaction support:

```go
type Transaction struct {
    writes []types.TransactWriteItem
    db     *DB
}

func (t *Transaction) Create(model interface{}) *Transaction {
    // Add Put operation to transaction
}

func (t *Transaction) Update(model interface{}) *Transaction {
    // Add Update operation to transaction
}

func (t *Transaction) Delete(model interface{}) *Transaction {
    // Add Delete operation to transaction
}

func (t *Transaction) Execute() error {
    // Execute TransactWriteItems
}
```

### 4. Batch Operations

Implement batch operations in the query:

```go
func (q *query) BatchGet(keys []interface{}, dest interface{}) error {
    // Validate keys match model's primary key structure
    // Build BatchGetItem request (max 100 items)
    // Handle pagination if > 100 keys
    // Unmarshal results preserving order
}

func (q *query) BatchCreate(items interface{}) error {
    // Validate items is a slice
    // Build BatchWriteItem requests (max 25 items per request)
    // Handle unprocessed items with retry
}
```

### 5. Special Field Handling

Enhance your type converter to handle special fields automatically:

```go
// In putItem, handle:
- created_at: Set to current time on create
- updated_at: Set to current time on create/update
- version: Initialize to 0 on create
- ttl: Validate and convert to Unix timestamp

// In updateItem, handle:
- updated_at: Always update to current time
- version: Increment if field exists
```

## Integration Points with Team 2

1. **Use their Expression Builder**:
   ```go
   import "github.com/pay-theory/dynamorm/internal/expr"
   
   // For update operations
   builder := expr.NewBuilder()
   for field, value := range updates {
       builder.AddUpdate(field, value)
   }
   updateExpr := builder.BuildUpdate()
   ```

2. **Query Compilation**:
   ```go
   import "github.com/pay-theory/dynamorm/pkg/query"
   
   // Team 2's query package should provide:
   compiled, err := query.Compile(q)
   // Use compiled.KeyCondition, compiled.FilterExpression, etc.
   ```

## Testing Requirements

1. **Integration Tests** (`tests/integration/`):
   - Full CRUD operations with DynamoDB Local
   - Transaction tests
   - Batch operation tests
   - Error handling scenarios

2. **Schema Tests**:
   - Table creation with various configurations
   - Index management
   - Schema updates

3. **Performance Benchmarks**:
   ```go
   func BenchmarkCreate(b *testing.B)
   func BenchmarkQuery(b *testing.B)
   func BenchmarkBatchOperations(b *testing.B)
   ```

## Deliverables for Session 2

1. ✅ Working CRUD operations using AWS SDK
2. ✅ Integration with Team 2's expression builder
3. ✅ Schema management for table operations
4. ✅ Transaction support
5. ✅ Batch operations
6. ✅ Comprehensive integration tests
7. ✅ Performance benchmarks

## Code Quality Checklist

- [ ] All placeholder methods in `dynamorm.go` are implemented
- [ ] Special fields (timestamps, version, TTL) work correctly
- [ ] Error handling provides clear, actionable messages
- [ ] Integration tests cover all major scenarios
- [ ] No race conditions in concurrent operations
- [ ] AWS SDK errors are properly wrapped

## Next Steps After Session 2

Once core operations are working:
1. Implement migration system
2. Add middleware/hooks support
3. Implement caching layer (optional)
4. Add metrics and observability

## Quick Reference

### AWS SDK v2 Operations You'll Need:
- `dynamodb.GetItem`
- `dynamodb.PutItem`
- `dynamodb.UpdateItem`
- `dynamodb.DeleteItem`
- `dynamodb.Query`
- `dynamodb.Scan`
- `dynamodb.BatchGetItem`
- `dynamodb.BatchWriteItem`
- `dynamodb.TransactWriteItems`
- `dynamodb.CreateTable`
- `dynamodb.UpdateTable`
- `dynamodb.DescribeTable`

Remember: You're implementing the bridge between DynamORM's elegant API and DynamoDB's powerful but complex operations. Make it reliable and performant! 
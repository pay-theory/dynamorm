# Team 1: Session 3 - Schema Management & Transactions

## Outstanding Progress! 🌟

Your team has successfully implemented all core CRUD operations with full AWS SDK integration. The ORM is functional but needs schema management and transaction support to be production-ready.

## Session 3 Objectives

### Priority 1: Schema Management (`pkg/schema/`)

Without table creation, users can't actually use DynamORM. This is the #1 priority.

#### 1.1 Create Schema Manager

```go
package schema

type Manager struct {
    session  *session.Session
    registry *model.Registry
}

func NewManager(session *session.Session, registry *model.Registry) *Manager {
    return &Manager{
        session:  session,
        registry: registry,
    }
}
```

#### 1.2 Implement Table Creation

```go
func (m *Manager) CreateTable(model interface{}, opts ...TableOption) error {
    metadata, err := m.registry.GetMetadata(model)
    if err != nil {
        return err
    }
    
    input := &dynamodb.CreateTableInput{
        TableName: aws.String(metadata.TableName),
        BillingMode: types.BillingModePayPerRequest, // Default
    }
    
    // Build key schema
    input.KeySchema = m.buildKeySchema(metadata)
    
    // Build attribute definitions
    input.AttributeDefinitions = m.buildAttributeDefinitions(metadata)
    
    // Build GSI/LSI
    input.GlobalSecondaryIndexes = m.buildGlobalSecondaryIndexes(metadata)
    input.LocalSecondaryIndexes = m.buildLocalSecondaryIndexes(metadata)
    
    // Apply options (billing mode, throughput, etc.)
    for _, opt := range opts {
        opt(input)
    }
    
    // Create table
    _, err = m.session.Client().CreateTable(context.Background(), input)
    if err != nil {
        return fmt.Errorf("failed to create table: %w", err)
    }
    
    // Wait for table to be active
    return m.waitForTableActive(metadata.TableName)
}
```

#### 1.3 Implement Table Updates

```go
func (m *Manager) UpdateTable(model interface{}) error {
    // Compare current table schema with model
    // Update indexes if needed
    // Handle throughput changes
}

func (m *Manager) DescribeTable(model interface{}) (*TableDescription, error) {
    // Get current table schema from DynamoDB
    // Compare with model metadata
}
```

#### 1.4 Add to Main DB Interface

```go
// In dynamorm.go
func (db *DB) CreateTable(model interface{}, opts ...schema.TableOption) error {
    manager := schema.NewManager(db.session, db.registry)
    return manager.CreateTable(model, opts)
}

func (db *DB) EnsureTable(model interface{}) error {
    // Check if table exists, create if not
}

func (db *DB) AutoMigrate(models ...interface{}) error {
    for _, model := range models {
        if err := db.EnsureTable(model); err != nil {
            return err
        }
    }
    return nil
}
```

### Priority 2: Transaction Support (`pkg/transaction/`)

Implement DynamoDB transactions for atomic operations.

#### 2.1 Transaction Builder

```go
package transaction

type Transaction struct {
    db     *DB
    writes []types.TransactWriteItem
    reads  []types.TransactGetItem
}

func (db *DB) Transaction(fn func(*Transaction) error) error {
    tx := &Transaction{db: db}
    
    // Build transaction
    if err := fn(tx); err != nil {
        return err
    }
    
    // Execute transaction
    return tx.Commit()
}
```

#### 2.2 Transaction Operations

```go
func (tx *Transaction) Create(model interface{}) error {
    // Marshal item
    item, err := tx.db.marshalItem(model, metadata)
    if err != nil {
        return err
    }
    
    // Add to transaction
    tx.writes = append(tx.writes, types.TransactWriteItem{
        Put: &types.Put{
            TableName: aws.String(metadata.TableName),
            Item:      item,
            ConditionExpression: aws.String("attribute_not_exists(#pk)"),
            ExpressionAttributeNames: map[string]string{
                "#pk": metadata.PrimaryKey.PartitionKey.DBName,
            },
        },
    })
    return nil
}

func (tx *Transaction) Update(model interface{}) error {
    // Build update expression
    // Add to transaction with version check
}

func (tx *Transaction) Delete(model interface{}) error {
    // Build delete with conditions
    // Add to transaction
}

func (tx *Transaction) Get(model interface{}, dest interface{}) error {
    // Add to read transaction
    tx.reads = append(tx.reads, types.TransactGetItem{
        Get: &types.Get{
            TableName: aws.String(metadata.TableName),
            Key:       key,
        },
    })
}

func (tx *Transaction) Commit() error {
    ctx := context.Background()
    
    // Execute writes if any
    if len(tx.writes) > 0 {
        input := &dynamodb.TransactWriteItemsInput{
            TransactItems: tx.writes,
        }
        
        _, err := tx.db.session.Client().TransactWriteItems(ctx, input)
        if err != nil {
            return tx.handleTransactionError(err)
        }
    }
    
    // Execute reads if any
    if len(tx.reads) > 0 {
        input := &dynamodb.TransactGetItemsInput{
            TransactItems: tx.reads,
        }
        
        output, err := tx.db.session.Client().TransactGetItems(ctx, input)
        if err != nil {
            return err
        }
        
        // Unmarshal results
        return tx.unmarshalTransactionResults(output)
    }
    
    return nil
}
```

### Priority 3: Migration System (`pkg/migration/`)

Basic migration support for schema versioning.

```go
package migration

type Migration struct {
    Version string
    Up      func(*schema.Manager) error
    Down    func(*schema.Manager) error
}

type Migrator struct {
    db         *DB
    migrations []Migration
}

func (m *Migrator) Run() error {
    // Create migration history table
    // Run pending migrations
    // Record completed migrations
}
```

### Testing Requirements

1. **Schema Tests** (`pkg/schema/schema_test.go`):
   - Table creation with various configurations
   - Index creation (GSI/LSI)
   - Table updates
   - Error handling

2. **Transaction Tests** (`pkg/transaction/transaction_test.go`):
   - Multi-item transactions
   - Rollback on failure
   - Conflict handling
   - Mixed read/write transactions

3. **Integration Tests** (`tests/integration/`):
   - Full workflow: create table → CRUD → transactions
   - Error scenarios
   - Performance benchmarks

### Integration with Existing Code

1. **Update AutoMigrate** in `dynamorm.go`:
   ```go
   func (db *DB) AutoMigrate(models ...interface{}) error {
       manager := schema.NewManager(db.session, db.registry)
       for _, model := range models {
           if err := manager.CreateTable(model); err != nil {
               // Check if table exists error
               if !isResourceInUseException(err) {
                   return err
               }
           }
       }
       return nil
   }
   ```

2. **Add Table Options**:
   ```go
   type TableOption func(*dynamodb.CreateTableInput)
   
   func WithBillingMode(mode types.BillingMode) TableOption
   func WithThroughput(rcu, wcu int64) TableOption
   func WithStreamSpecification(spec types.StreamSpecification) TableOption
   func WithSSESpecification(spec types.SSESpecification) TableOption
   ```

## Deliverables for Session 3

### Must Have
1. ✅ Complete schema management (CreateTable, UpdateTable)
2. ✅ Transaction support (TransactWrite, TransactGet)
3. ✅ Update AutoMigrate to actually create tables
4. ✅ Integration tests with table lifecycle

### Should Have
1. ✅ Basic migration system
2. ✅ Table existence checking
3. ✅ Wait for table ready states
4. ✅ Index management in updates

### Nice to Have
1. 🎯 Table backup/restore
2. 🎯 Global table support
3. 🎯 Point-in-time recovery setup
4. 🎯 Auto-scaling configuration

## Code Quality Checklist

- [ ] Schema creation handles all struct tag configurations
- [ ] Transactions properly handle errors and rollbacks
- [ ] No resource leaks in table operations
- [ ] Proper timeout handling for table creation
- [ ] Clear error messages for schema mismatches
- [ ] Thread-safe schema operations

## Example Test Case

Your implementation should make this work:

```go
func TestCompleteWorkflow(t *testing.T) {
    // Initialize DB
    db, err := dynamorm.New(dynamorm.Config{
        Region:   "us-east-1",
        Endpoint: "http://localhost:8000",
    })
    require.NoError(t, err)
    
    // Create table
    err = db.CreateTable(&User{}, 
        schema.WithBillingMode(types.BillingModePayPerRequest),
    )
    require.NoError(t, err)
    
    // Use transactions
    err = db.Transaction(func(tx *dynamorm.Transaction) error {
        user1 := &User{ID: "1", Name: "Alice", Balance: 100}
        user2 := &User{ID: "2", Name: "Bob", Balance: 50}
        
        if err := tx.Create(user1); err != nil {
            return err
        }
        if err := tx.Create(user2); err != nil {
            return err
        }
        
        // Transfer funds atomically
        user1.Balance -= 25
        user2.Balance += 25
        
        if err := tx.Update(user1); err != nil {
            return err
        }
        return tx.Update(user2)
    })
    require.NoError(t, err)
}
```

## Success Criteria

1. **Users can create tables from models** - The #1 blocker removed
2. **Transactions work reliably** - Data integrity guaranteed
3. **Schema updates are safe** - No data loss during updates
4. **Migration system tracks versions** - Schema evolution supported

Remember: Schema management is what makes an ORM usable in the real world. Make it rock solid! 
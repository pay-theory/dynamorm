# Team 2: Query Builder & Expression Engine - Task List

## Current Status

### ✅ Completed
1. **Project Structure** - Created directory structure for Team 2's packages
2. **Core Interfaces** - Set up shared interfaces between Team 1 and Team 2
3. **Expression Builder** - Implemented basic expression builder with:
   - Key condition expressions
   - Filter expressions
   - Projection expressions
   - Update expressions
   - Support for common operators (=, <, >, BETWEEN, IN, CONTAINS, etc.)
4. **Query Builder** - Started implementation of fluent query API
5. **Type Converter** - Basic implementation for converting Go types to DynamoDB AttributeValues
6. **Test Models** - Created shared test models for integration testing
7. **Integration Tests** - Created test templates demonstrating query usage

### ⚠️ In Progress
1. **Query Interface Implementation** - The Query struct needs to implement all methods from core.Query interface:
   - [ ] BatchGet
   - [ ] BatchCreate
   - [ ] Scan
   - [ ] WithContext
   - [ ] Offset

### 🔴 TODO

#### Phase 2: Query Builder (Weeks 3-4)
1. **Complete Query Implementation**
   ```go
   // Add missing methods to pkg/query/query.go
   func (q *Query) BatchGet(keys []interface{}, dest interface{}) error
   func (q *Query) BatchCreate(items interface{}) error  
   func (q *Query) Scan(dest interface{}) error
   func (q *Query) WithContext(ctx context.Context) core.Query
   func (q *Query) Offset(offset int) core.Query
   ```

2. **Reserved Words Handling**
   - Implement proper attribute name substitution for DynamoDB reserved words
   - Update `processAttributeNames` in expression builder

3. **Complex Expression Support**
   - AND/OR logic in conditions
   - Nested conditions
   - Function expressions (size, attribute_type, etc.)

4. **Query Optimization**
   - Query vs Scan decision logic improvements
   - Cost estimation
   - Query plan caching

#### Phase 3: Advanced Queries (Weeks 5-6)
1. **Pagination Support**
   - Implement cursor-based pagination
   - Handle LastEvaluatedKey
   - Support for forward/backward pagination

2. **Batch Operations**
   - BatchGetItem implementation
   - BatchWriteItem implementation
   - Transaction support

3. **Parallel Scan**
   - Implement parallel scan for large tables
   - Segment management
   - Result aggregation

4. **Advanced Expression Functions**
   - attribute_type()
   - size()
   - Nested attribute access
   - List/Map operations

#### Phase 4: Index Management (Weeks 7-8)
1. **Index Selector** (`pkg/index/selector.go`)
   ```go
   type IndexSelector struct {
       indexes []IndexMetadata
       stats   QueryStatistics
   }
   
   func SelectOptimalIndex(conditions []Condition) (*Index, error)
   ```

2. **Index Metadata Parser**
   - Parse index definitions from struct tags
   - Validate index configurations
   - Generate CloudFormation schemas

3. **Query Statistics**
   - Track query performance
   - Index usage statistics
   - Cost tracking

## Integration Notes

### Dependencies on Team 1
1. **Type System** - Need full implementation of type converters
2. **Model Registry** - Need access to model metadata
3. **Query Executor** - Need implementation of ExecuteQuery/ExecuteScan
4. **Error Handling** - Coordinate on error types

### API Compatibility Issues
The core.Query interface has more methods than originally documented in TEAM_COORDINATION.md. Need to sync with Team 1 on:
- Whether all methods are required
- Implementation priorities
- Interface updates

## Testing Strategy

1. **Unit Tests**
   - Expression builder tests
   - Query compilation tests
   - Operator tests

2. **Integration Tests**
   - End-to-end query tests with DynamoDB Local
   - Performance benchmarks
   - Edge case testing

3. **Test Coverage Goals**
   - Expression builder: 90%+
   - Query builder: 85%+
   - Index selector: 80%+

## Next Steps

1. **Immediate** (Today/Tomorrow)
   - Fix compilation errors in query.go
   - Implement missing Query interface methods
   - Add unit tests for expression builder

2. **This Week**
   - Complete basic query operations
   - Add support for all operators
   - Implement pagination

3. **Next Week**
   - Advanced query features
   - Performance optimizations
   - Integration with Team 1's code

## Questions for Team 1

1. Is the QueryExecutor interface correct? Should it handle pagination internally?
2. Are all methods in core.Query required for MVP?
3. How should we handle type conversion errors?
4. What's the preferred error handling pattern?

## Resources

- AWS DynamoDB Expression Documentation
- Go AWS SDK v2 Examples
- DynamoDB Best Practices Guide 
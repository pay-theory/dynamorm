# Aggregates and Advanced Queries Guide

This guide covers the advanced query features added in Phase 4 of DynamORM, including GroupBy, Having clauses, cursor-based pagination, and parallel scans.

## Table of Contents
- [GroupBy Operations](#groupby-operations)
- [Aggregate Functions](#aggregate-functions)
- [Having Clauses](#having-clauses)
- [Cursor-based Pagination](#cursor-based-pagination)
- [Parallel Scans](#parallel-scans)

## GroupBy Operations

GroupBy allows you to group items by a field and perform aggregate operations on each group.

### Basic GroupBy

```go
// Group products by category
results, err := db.Model(&Product{}).
    GroupBy("Category").
    Execute()

// Each result contains:
// - Key: The grouping value (e.g., "electronics", "books")
// - Count: Number of items in the group
// - Items: The actual items in the group
```

### GroupBy with Aggregations

```go
// Group orders by status with multiple aggregations
results, err := db.Model(&Order{}).
    GroupBy("Status").
    Count("order_count").
    Sum("Total", "revenue").
    Avg("Total", "avg_order").
    Min("CreatedAt", "first_order").
    Max("CreatedAt", "last_order").
    Execute()

// Access aggregation results
for _, group := range results {
    fmt.Printf("Status: %v\n", group.Key)
    fmt.Printf("Orders: %d\n", group.Aggregates["order_count"].Count)
    fmt.Printf("Revenue: $%.2f\n", group.Aggregates["revenue"].Sum)
    fmt.Printf("Average: $%.2f\n", group.Aggregates["avg_order"].Average)
}
```

## Aggregate Functions

DynamORM supports the following aggregate functions:

### COUNT
Count the number of items in each group.

```go
GroupBy("Category").Count("item_count")
```

### SUM
Calculate the sum of a numeric field.

```go
GroupBy("Category").Sum("Price", "total_price")
```

### AVG
Calculate the average of a numeric field.

```go
GroupBy("Category").Avg("Rating", "avg_rating")
```

### MIN
Find the minimum value of a field (works with numbers, strings, dates).

```go
GroupBy("Category").Min("Price", "lowest_price")
```

### MAX
Find the maximum value of a field.

```go
GroupBy("Category").Max("Price", "highest_price")
```

## Having Clauses

Having clauses filter groups based on aggregate values, similar to SQL HAVING.

### Basic Having

```go
// Find categories with more than 10 items
results, err := db.Model(&Product{}).
    GroupBy("Category").
    Count("count").
    Having("COUNT(*)", ">", 10).
    Execute()
```

### Multiple Having Conditions

```go
// Find high-performing categories
results, err := db.Model(&Sales{}).
    GroupBy("Category").
    Count("sales_count").
    Sum("Amount", "total_sales").
    Avg("Amount", "avg_sale").
    Having("COUNT(*)", ">=", 100).
    Having("total_sales", ">", 10000).
    Having("avg_sale", ">", 50).
    Execute()
```

### Supported Having Operators

- `=` - Equal to
- `>` - Greater than
- `>=` - Greater than or equal to
- `<` - Less than
- `<=` - Less than or equal to
- `!=` - Not equal to

## Cursor-based Pagination

Cursor-based pagination provides efficient navigation through large result sets.

### Basic Pagination

```go
// First page
result, err := db.Model(&User{}).
    Where("Status", "=", "active").
    Limit(20).
    AllPaginated(&users)

if result.HasMore {
    // Get next page using cursor
    nextResult, err := db.Model(&User{}).
        Where("Status", "=", "active").
        Cursor(result.NextCursor).
        Limit(20).
        AllPaginated(&moreUsers)
}
```

### Pagination Loop

```go
var allUsers []User
cursor := ""

for {
    var pageUsers []User
    result, err := db.Model(&User{}).
        Where("Status", "=", "active").
        Cursor(cursor).
        Limit(50).
        AllPaginated(&pageUsers)
    
    if err != nil {
        return err
    }
    
    allUsers = append(allUsers, pageUsers...)
    
    if !result.HasMore {
        break
    }
    
    cursor = result.NextCursor
}
```

### Pagination with Ordering

```go
// Paginate through users ordered by creation date
result, err := db.Model(&User{}).
    OrderBy("CreatedAt", "desc").
    Limit(10).
    AllPaginated(&recentUsers)
```

## Parallel Scans

Parallel scans divide a table into segments and scan them concurrently for improved performance.

### Basic Parallel Scan

```go
// Scan with 4 parallel segments
var allItems []Item
err := db.Model(&Item{}).
    ScanAllSegments(&allItems, 4)
```

### Parallel Scan with Filters

```go
// Parallel scan with filtering
var expensiveItems []Product
err := db.Model(&Product{}).
    Filter("Price", ">", 100).
    Filter("InStock", "=", true).
    ScanAllSegments(&expensiveItems, 8)
```

### Choosing Segment Count

- Use 2-10 segments for most cases
- More segments = more parallelism but also more resources
- Consider your table size and available compute capacity

```go
// Adaptive segment count based on table size
segmentCount := int32(4) // Default
if estimatedItemCount > 100000 {
    segmentCount = 8
}
if estimatedItemCount > 1000000 {
    segmentCount = 16
}

err := db.Model(&Item{}).
    ScanAllSegments(&items, segmentCount)
```

## Complete Example

Here's a comprehensive example combining all features:

```go
type SalesRecord struct {
    ID         string    `dynamorm:"id,pk"`
    StoreID    string    `dynamorm:"store_id"`
    Category   string    `dynamorm:"category"`
    ProductID  string    `dynamorm:"product_id"`
    Amount     float64   `dynamorm:"amount"`
    Quantity   int       `dynamorm:"quantity"`
    Date       time.Time `dynamorm:"date"`
}

// Analyze sales performance by store and category
func analyzeSales(db *dynamorm.DB, minRevenue float64) error {
    // 1. Group by store and category with aggregations
    results, err := db.Model(&SalesRecord{}).
        GroupBy("StoreID").
        Count("transaction_count").
        Sum("Amount", "total_revenue").
        Avg("Amount", "avg_transaction").
        Sum("Quantity", "units_sold").
        Having("total_revenue", ">", minRevenue).
        Execute()
    
    if err != nil {
        return err
    }
    
    // 2. Process high-performing stores
    for _, store := range results {
        fmt.Printf("Store %v: %d transactions, $%.2f revenue\n",
            store.Key,
            store.Aggregates["transaction_count"].Count,
            store.Aggregates["total_revenue"].Sum)
        
        // 3. Paginate through store's transactions
        var storeTransactions []SalesRecord
        cursor := ""
        
        for {
            result, err := db.Model(&SalesRecord{}).
                Where("StoreID", "=", store.Key).
                OrderBy("Date", "desc").
                Cursor(cursor).
                Limit(100).
                AllPaginated(&storeTransactions)
            
            if err != nil {
                return err
            }
            
            // Process transactions...
            
            if !result.HasMore {
                break
            }
            cursor = result.NextCursor
        }
    }
    
    // 4. Parallel scan for inventory analysis
    var allRecords []SalesRecord
    err = db.Model(&SalesRecord{}).
        Filter("Date", ">", time.Now().AddDate(0, -1, 0)).
        ScanAllSegments(&allRecords, 8)
    
    return err
}
```

## Best Practices

1. **GroupBy Performance**: GroupBy operations load all items into memory. Use filters to reduce the dataset size before grouping.

2. **Having vs Where**: Use Where for filtering individual items, Having for filtering groups based on aggregate values.

3. **Cursor Storage**: Store cursors securely if exposing them to clients. They contain encoded table data.

4. **Parallel Scan Limits**: Be mindful of read capacity when using parallel scans. Each segment consumes separate read capacity.

5. **Aggregate Accuracy**: Aggregations are performed in-memory after retrieving items. For very large datasets, consider using DynamoDB Streams with external aggregation systems.

## Error Handling

```go
results, err := db.Model(&Item{}).
    GroupBy("Category").
    Count("count").
    Execute()

if err != nil {
    switch {
    case errors.Is(err, dynamorm.ErrInvalidField):
        // Handle invalid field error
    case errors.Is(err, dynamorm.ErrQueryTimeout):
        // Handle timeout
    default:
        // Handle other errors
    }
}
```

## Limitations

1. **In-Memory Processing**: All aggregations are performed in-memory after fetching data from DynamoDB.

2. **No Cross-Partition Queries**: GroupBy operations require scanning or querying to retrieve all items first.

3. **Result Size**: Large result sets may cause memory issues. Use pagination and filters to limit data size.

4. **Having Clause Evaluation**: Having clauses are evaluated after all data is retrieved and aggregated. 
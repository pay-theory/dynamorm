# Development Guidelines

This guide outlines the coding standards and best practices for developing with DynamORM.

## Struct Definition Standards

DynamORM relies heavily on Go struct tags. Follow these rules strictly:

1.  **Primary Keys:** Always tag your partition key with `dynamorm:"pk"` and sort key with `dynamorm:"sk"`.
2.  **JSON Tags:** Always include `json:"name"` tags matching your attribute names (usually snake_case).
3.  **Types:** Use standard Go types (`string`, `int`, `int64`, `float64`, `bool`, `time.Time`).

```go
// ✅ CORRECT
type Product struct {
    ID    string  `dynamorm:"pk" json:"id"`
    Price float64 `json:"price"`
}

// ❌ INCORRECT
type Product struct {
    ID string // Missing tags!
}
```

## Error Handling

Always check errors. DynamORM returns typed errors where possible.

- **Validation Errors:** Occur before network calls (invalid struct tags, missing keys).
- **Runtime Errors:** Occur during AWS execution (throughput exceeded, conditional check failed).

```go
if err := db.Model(item).Create(); err != nil {
    if errors.Is(err, customerrors.ErrConditionFailed) {
        // Handle duplicate
    }
    return err
}
```

## Code Style

- **Fluent Chains:** Break long query chains onto multiple lines for readability.
- **Context:** Use `context.TODO()` or `context.Background()` if you aren't passing a request context (though `WithContext` is preferred).

```go
// Readable
db.Model(&Item{}).
    Where("ID", "=", "1").
    Limit(1).
    First(&item)
```

## Contribution Workflow

1.  **Fork & Branch:** Create a feature branch.
2.  **Test:** Run `go test ./...` to ensure no regressions.
3.  **Docs:** Update documentation if you change public APIs.
4.  **PR:** Submit a Pull Request with a clear description.

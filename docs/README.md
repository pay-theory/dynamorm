# DynamORM Documentation

Welcome to the DynamORM documentation! This guide will help you get started with DynamORM and master its features.

## 📚 Documentation Structure

### Getting Started
- **[Installation & Initialization](./getting-started/installation.md)** - Start here!
- [Quick Start Guide](./getting-started/quickstart.md)
- [Basic Usage](./getting-started/basic-usage.md)

### Core Guides
- **[Atomic Operations](./guides/atomic-operations.md)** - Rate limiting, counters, and more
- **[Composite Keys](./guides/composite-keys.md)** - PK/SK patterns and best practices
- [Testing with Mocks](./guides/testing.md) - Unit testing with pre-built mocks
- [Query Patterns](./guides/queries.md) - Advanced querying techniques
- **[Query Patterns v1.0.9](./guides/query-patterns-v109.md)** - Complete v1.0.9 API examples
- [Working with Indexes](./guides/indexes.md) - GSI and LSI usage

### Migration & Troubleshooting
- **[v0.x to v1.0.9 Migration](./migration/v0-to-v1.md)** - Step-by-step migration guide
- [Common Errors](./troubleshooting/common-errors.md) - Error solutions

### Architecture & Design
- [Architecture Overview](./architecture/overview.md)
- [Interface Design](./architecture/interfaces.md)
- [Performance Optimization](./architecture/performance.md)
- [Interface Segregation Proposal](./architecture/interface-segregation-proposal.md)

### API Reference
- [Core Interfaces](./reference/interfaces.md)
- [Query Builder API](./reference/query-builder.md)
- [Update Builder API](./reference/update-builder.md)
- [Configuration Options](./reference/configuration.md)

### Examples
- [Basic CRUD Operations](../examples/basic/)
- [E-commerce Application](../examples/ecommerce/)
- [Multi-tenant System](../examples/multi-tenant/)
- [Payment Processing](../examples/payment/)
- [Testing Examples](../examples/testing/)

### Release Notes
- [v1.0.9 - Performance Improvements](./releases/v1.0.9-performance.md)
- [v1.0.3 - Core Implementation](./releases/v1.0.3-core-implementation.md)
- [v1.0.2 - Mocks Package](./releases/v1.0.2-mocks-package.md)
- [v1.0.1 - Interface Improvements](./releases/v1.0.1-interface-improvements.md)

## 🎯 Quick Links by Use Case

### "I'm upgrading from v0.x"
→ Follow [Migration Guide](./migration/v0-to-v1.md)

### "I need composite keys"
→ See [Composite Keys Guide](./guides/composite-keys.md)

### "I need atomic operations"
→ Check [Atomic Operations Guide](./guides/atomic-operations.md)

### "I want to write tests"
→ Use [Testing with Mocks](./guides/testing.md)

### "I'm new to DynamORM"
→ Start with [Installation](./getting-started/installation.md)

## 📋 Common Code Patterns

### Correct Initialization
```go
import (
    "github.com/pay-theory/dynamorm"
    "github.com/pay-theory/dynamorm/pkg/session"
)

config := session.Config{
    Region: "us-east-1",
}
db, err := dynamorm.New(config)
```

### PK/SK Pattern for Composite Keys
```go
type Model struct {
    PK string `dynamorm:"pk"`
    SK string `dynamorm:"sk"`
    // other fields
}

func (m *Model) SetKeys() {
    m.PK = m.Field1
    m.SK = m.Field2
}
```

### Atomic Operations
```go
db.Model(&Counter{ID: "123"}).
    UpdateBuilder().
    Increment("Count").
    Execute()
```

## 🤝 Contributing

Found an issue or want to contribute? Check our [Contributing Guide](../CONTRIBUTING.md).

## 📞 Support

- GitHub Issues: [Report bugs or request features](https://github.com/pay-theory/dynamorm/issues)
- Discussions: [Ask questions](https://github.com/pay-theory/dynamorm/discussions)

---

*Last updated for DynamORM v1.0.9* 
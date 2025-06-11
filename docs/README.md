# DynamORM Documentation

## ⚠️ CRITICAL: v1.0.2 Issues

**DynamORM v1.0.2 has critical issues that affect all users:**

1. **[Nil Pointer Dereference](./troubleshooting/nil-pointer-fix.md)** - Occurs on any DynamoDB operation
2. **Integration tests have never been run** - The library was released without proper testing
3. **Documentation shows non-working examples** - Many examples use incorrect initialization

**If you're experiencing issues, please read:**
- 🚨 **[Critical Issues Summary](./releases/v1.0.2-critical-issues.md)** - Complete list of known issues
- 🔧 **[Nil Pointer Fix Guide](./troubleshooting/nil-pointer-fix.md)** - Comprehensive fix for the most common issue
- 🧪 **[Integration Testing Guide](./testing/integration-test-guide.md)** - How to properly test DynamORM

---

Welcome to the DynamORM documentation! This guide will help you get started with DynamORM and master its features.

## 🚨 Important Notes for v1.0.2

- **[Migration Guide from v0.x](./migration/v0-to-v1.md)** - Breaking changes and migration steps
- **[Composite Keys Guide](./guides/composite-keys.md)** - Composite key syntax has changed

## 📚 Documentation Structure

### Getting Started
- **[Installation & Initialization](./getting-started/installation.md)** ⭐ Start here!
- [Quick Start Guide](./getting-started/quickstart.md)
- [Basic Usage](./getting-started/basic-usage.md)

### Core Guides
- **[Atomic Operations](./guides/atomic-operations.md)** - Rate limiting, counters, and more
- **[Composite Keys](./guides/composite-keys.md)** - PK/SK patterns and best practices
- [Testing with Mocks](./guides/testing.md) - Unit testing with pre-built mocks
- [Query Patterns](./guides/queries.md) - Advanced querying techniques
- [Working with Indexes](./guides/indexes.md) - GSI and LSI usage

### Migration & Troubleshooting
- **[v0.x to v1.0.2 Migration](./migration/v0-to-v1.md)** - Step-by-step migration guide
- **[Nil Pointer Fix](./troubleshooting/nil-pointer-fix.md)** - Common initialization issues
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
- [v1.0.2 - Mocks Package](./releases/v1.0.2-mocks-package.md)
- [v1.0.1 - Interface Improvements](./releases/v1.0.1-interface-improvements.md)

## 🎯 Quick Links by Use Case

### "I'm getting a nil pointer error"
→ Read [Nil Pointer Fix Guide](./troubleshooting/nil-pointer-fix.md)

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

### Correct Initialization (v1.0.2)
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

*Last updated for DynamORM v1.0.2* 
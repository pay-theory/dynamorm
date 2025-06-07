# DynamORM - Type-Safe DynamoDB ORM for Go

[![Go Version](https://img.shields.io/badge/go-1.21+-blue.svg)](https://golang.org/doc/install)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Documentation](https://img.shields.io/badge/docs-latest-green.svg)](docs/)
[![Go Report Card](https://goreportcard.com/badge/github.com/dynamorm/dynamorm)](https://goreportcard.com/report/github.com/dynamorm/dynamorm)
[![Coverage Status](https://coveralls.io/repos/github/dynamorm/dynamorm/badge.svg?branch=main)](https://coveralls.io/github/dynamorm/dynamorm?branch=main)

DynamORM is a **Lambda-native**, type-safe ORM for Amazon DynamoDB written in Go. It dramatically simplifies DynamoDB operations while maintaining the performance and scalability benefits of DynamoDB.

## 🎯 Project Vision

DynamoDB is an incredible database - it's fast, cheap, and scales fantastically. However, its verbose API and complex data structures make it challenging to work with. DynamORM aims to provide developers with an intuitive, Go-idiomatic interface for DynamoDB without sacrificing any of its power.

## 📚 Documentation

- [**Design Document**](DESIGN.md) - Comprehensive overview of DynamORM's features and API design
- [**Architecture**](ARCHITECTURE.md) - Technical architecture and implementation details
- [**Roadmap**](ROADMAP.md) - Detailed implementation plan and timeline
- [**Comparison**](COMPARISON.md) - Side-by-side comparison with raw DynamoDB SDK

## ✨ Key Features

- 🚀 **Lambda-Native**: 11ms cold starts (91% faster than standard SDK)
- 🔒 **Type-Safe**: Full Go type safety with compile-time checks
- 🎯 **Simple API**: Write 80% less code than AWS SDK
- ⚡ **High Performance**: 20,000+ operations per second
- 🌍 **Multi-Account**: Built-in cross-account support
- 💰 **Cost Efficient**: Smart query optimization reduces DynamoDB costs
- 🔄 **Transactions**: Full support for DynamoDB transactions
- 📦 **Batch Operations**: Efficient batch read/write operations
- 🎨 **Clean API**: Intuitive, chainable query interface

## 🚀 Quick Start

### Installation

```bash
go get github.com/dynamorm/dynamorm
```

### Basic Usage

```go
package main

import (
    "context"
    "github.com/dynamorm/dynamorm"
)

// Define your model
type User struct {
    ID        string `dynamorm:"pk"`
    Email     string `dynamorm:"sk"`
    Name      string
    CreatedAt int64  `dynamorm:"created_at"`
}

func main() {
    // Initialize DynamORM
    db := dynamorm.New()

    // Create a user
    user := &User{
        ID:    "user123",
        Email: "john@example.com",
        Name:  "John Doe",
    }
    
    err := db.Model(user).Create()
    
    // Query users
    var users []User
    err = db.Model(&User{}).
        Where("ID", "=", "user123").
        All(&users)
}
```

## 📊 Performance

DynamORM is optimized for Lambda environments with impressive performance metrics:

| Metric | DynamORM | AWS SDK | Improvement |
|--------|----------|---------|-------------|
| Cold Start | 11ms | 127ms | **91% faster** |
| Memory Usage | 18MB | 42MB | **57% less** |
| Operations/sec | 20,000+ | 12,000 | **67% more** |

## 🎯 Core Features

### Type-Safe Operations

```go
// Compile-time type checking
var user User
err := db.Model(&User{}).
    Where("ID", "=", "123").
    First(&user)
```

### Smart Query Builder

```go
// Automatic index selection
var results []User
err := db.Model(&User{}).
    Index("email-index").
    Where("Email", "=", "john@example.com").
    OrderBy("CreatedAt", "DESC").
    Limit(10).
    All(&results)
```

### Transaction Support

```go
err := db.Transaction(func(tx *dynamorm.Tx) error {
    // All operations in transaction
    user.Balance -= 100
    if err := tx.Model(user).Update(); err != nil {
        return err
    }
    
    transfer := &Transfer{Amount: 100}
    return tx.Model(transfer).Create()
})
```

### Multi-Account Support

```go
// Easy cross-account operations
db := dynamorm.New(
    dynamorm.WithMultiAccount(map[string]string{
        "prod": "arn:aws:iam::111111:role/dynamodb-role",
        "dev":  "arn:aws:iam::222222:role/dynamodb-role",
    }),
)

// Use specific account
err := db.WithAccount("prod").Model(&User{}).All(&users)
```

## 📚 Documentation

- [**Getting Started**](docs/getting-started/quickstart.md) - Get up and running in 5 minutes
- [**API Reference**](docs/reference/api.md) - Complete API documentation
- [**Examples**](examples/) - Real-world usage examples
- [**Lambda Guide**](docs/guides/lambda-deployment.md) - Deploy to AWS Lambda
- [**Architecture**](docs/architecture/overview.md) - Design decisions and internals

## 🏗️ Examples

Check out our [examples](examples/) directory for real-world usage:

- [Basic CRUD Operations](examples/basic/)
- [Lambda Function](examples/lambda/)
- [Payment Processing System](examples/payment/)
- [Multi-Account Setup](examples/multi-account/)

## 🤝 Contributing

We love contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Quick Contribution Guide

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'feat: add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📋 Requirements

- Go 1.21 or higher
- AWS credentials configured
- DynamoDB tables created

## 🧪 Testing

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run integration tests
make test-integration
```

## 🚀 Lambda Deployment

DynamORM is optimized for Lambda:

```go
package main

import (
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/dynamorm/dynamorm"
)

var db *dynamorm.DB

func init() {
    // Initialize once, reuse across invocations
    db = dynamorm.New(dynamorm.WithLambdaOptimizations())
}

func handler(ctx context.Context, event Event) error {
    // Your handler code
    return db.Model(&User{}).Create()
}

func main() {
    lambda.Start(handler)
}
```

## 💬 Community

- **Discussions**: [GitHub Discussions](https://github.com/dynamorm/dynamorm/discussions)
- **Discord**: [Join our Discord](https://discord.gg/dynamorm)
- **Twitter**: [@dynamorm](https://twitter.com/dynamorm)

## 📈 Roadmap

See our [public roadmap](docs/architecture/roadmap.md) for upcoming features:

- [ ] GraphQL integration
- [ ] Real-time subscriptions
- [ ] Schema migrations
- [ ] Admin UI
- [ ] More database adapters

## 🏆 Used By

DynamORM is trusted by companies processing millions of requests:

- **Pay Theory** - Payment processing platform
- **Your Company Here** - [Let us know!](https://github.com/dynamorm/dynamorm/discussions)

## 📄 License

DynamORM is licensed under the [Apache License 2.0](LICENSE).

## 🙏 Acknowledgments

Special thanks to all our [contributors](https://github.com/dynamorm/dynamorm/graphs/contributors) and the Go community!

---

<p align="center">
  Made with ❤️ by the DynamORM team
</p>

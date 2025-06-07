# Installation

This guide will help you install DynamORM and set up your development environment.

## Prerequisites

Before installing DynamORM, ensure you have:

- **Go 1.21 or higher** installed ([Download Go](https://golang.org/dl/))
- **AWS credentials** configured (if using AWS DynamoDB)
- **Docker** (optional, for local development with DynamoDB Local)

## Installing DynamORM

### Standard Installation

Install DynamORM using Go modules:

```bash
go get github.com/dynamorm/dynamorm
```

This will add DynamORM to your `go.mod` file and download the package.

### Specific Version

To install a specific version:

```bash
go get github.com/dynamorm/dynamorm@v1.0.0
```

### Latest Development Version

To get the latest development version:

```bash
go get github.com/dynamorm/dynamorm@main
```

## Verifying Installation

Create a simple test file to verify the installation:

```go
package main

import (
    "fmt"
    "github.com/dynamorm/dynamorm"
)

func main() {
    fmt.Println("DynamORM version:", dynamorm.Version)
}
```

Run the file:

```bash
go run main.go
```

## Setting Up AWS Credentials

DynamORM uses the AWS SDK, which requires credentials. Set them up using one of these methods:

### 1. AWS CLI (Recommended)

```bash
aws configure
```

### 2. Environment Variables

```bash
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_REGION=us-east-1
```

### 3. IAM Role (For EC2/Lambda)

If running on AWS infrastructure, IAM roles are automatically used.

## Local Development Setup

For local development, we recommend using DynamoDB Local:

### Using Docker (Recommended)

```bash
# Pull and run DynamoDB Local
docker run -p 8000:8000 amazon/dynamodb-local

# Or using docker-compose (see docker-compose.yml in repo)
docker-compose up -d dynamodb-local
```

### Using JAR File

```bash
# Download DynamoDB Local
wget https://s3.us-west-2.amazonaws.com/dynamodb-local/dynamodb_local_latest.zip
unzip dynamodb_local_latest.zip

# Run DynamoDB Local
java -Djava.library.path=./DynamoDBLocal_lib -jar DynamoDBLocal.jar -sharedDb
```

## IDE Setup

### VS Code

Install the Go extension:

```bash
code --install-extension golang.go
```

### GoLand

DynamORM works out of the box with GoLand. Enable Go modules support in settings.

### Vim/Neovim

Install `gopls` for Go language server support:

```bash
go install golang.org/x/tools/gopls@latest
```

## Project Structure

Recommended project structure for DynamORM:

```
myapp/
├── main.go
├── go.mod
├── go.sum
├── models/
│   ├── user.go
│   ├── product.go
│   └── order.go
├── repositories/
│   ├── user_repository.go
│   └── product_repository.go
├── config/
│   └── database.go
└── migrations/
    └── 001_initial_schema.go
```

## Configuration File

Create a configuration file for your database connection:

```go
// config/database.go
package config

import (
    "github.com/dynamorm/dynamorm"
)

func NewDB() (*dynamorm.DB, error) {
    config := dynamorm.Config{
        Region: "us-east-1",
        // For local development:
        // Endpoint: "http://localhost:8000",
    }
    
    return dynamorm.New(config)
}
```

## Dependencies

DynamORM has minimal dependencies:

- `github.com/aws/aws-sdk-go-v2` - AWS SDK for Go
- `github.com/google/uuid` - UUID generation
- Standard library packages

## Troubleshooting Installation

### Module Errors

If you encounter module errors:

```bash
# Clear module cache
go clean -modcache

# Download dependencies again
go mod download
```

### Permission Errors

On Unix systems, you might need to set proper permissions:

```bash
# Fix permissions for Go module cache
chmod -R 755 ~/go/pkg/mod
```

### Proxy Issues

If behind a corporate proxy:

```bash
# Set Go proxy
export GOPROXY=https://proxy.golang.org,direct
export GOSUMDB=off
```

## Next Steps

Now that you have DynamORM installed:

1. Read the [Quickstart Guide](quickstart.md) to build your first application
2. Learn about [Basic Usage](basic-usage.md) patterns
3. Check out the [Examples](../../examples/) directory

## Getting Help

If you run into issues:

- Check our [Troubleshooting Guide](../guides/troubleshooting.md)
- Visit [GitHub Issues](https://github.com/dynamorm/dynamorm/issues)
- Join our [Discord Community](https://discord.gg/dynamorm) 
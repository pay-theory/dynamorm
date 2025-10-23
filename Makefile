# DynamORM Makefile

.PHONY: all build test test-unit clean lint fmt docker-up docker-down docker-clean integration benchmark stress test-all

# Variables
GOMOD := github.com/pay-theory/dynamorm
UNIT_PACKAGES := $(shell go list ./... | grep -v /vendor/ | grep -v /examples/ | grep -v /tests/stress | grep -v /tests/integration)
ALL_PACKAGES := $(shell go list ./... | grep -v /vendor/ | grep -v /examples/ | grep -v /tests/stress)
INTEGRATION_PACKAGES := $(shell go list ./tests/integration/...)

# Default target
all: fmt lint test build

# Build the project
build:
	@echo "Building DynamORM..."
	@go build -v ./...

# Run all tests (unit + integration)
test: docker-up
	@echo "Running all tests (unit + integration)..."
	@go test -v -race -coverprofile=coverage.out -count=1 $(ALL_PACKAGES)
	@echo ""
	@echo "✅ All tests passed!"

# Run only unit tests (fast, no DynamoDB required)
test-unit:
	@echo "Running unit tests only..."
	@go test -v -race -coverprofile=coverage.out $(UNIT_PACKAGES)

# Run integration tests (requires DynamoDB Local)
integration: docker-up
	@echo "Running integration tests..."
	@go test -v $(INTEGRATION_PACKAGES)

# Run benchmarks
benchmark:
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./tests/benchmarks/...

# Run stress tests
stress:
	@echo "Running stress tests..."
	@go test -v ./tests/stress/...

# Run all tests including integration, benchmarks, and stress
test-all: docker-up
	@echo "Running all tests (unit, integration, benchmarks, stress)..."
	@go test -v -race -coverprofile=coverage.out $(ALL_PACKAGES)
	@go test -bench=. -benchmem ./tests/benchmarks/...
	@go test -v ./tests/stress/...
	@echo ""
	@echo "✅ All tests completed!"
	@echo "Note: Run 'make docker-down' to stop DynamoDB Local when done"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@gofmt -s -w .

# Run linters
lint:
	@echo "Running linters..."
	@golangci-lint run ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f coverage.out
	@go clean -cache

# Start DynamoDB Local
docker-up:
	@echo "Starting DynamoDB Local..."
	@if docker ps -a | grep -q dynamodb-local; then \
		if docker ps | grep -q dynamodb-local; then \
			echo "DynamoDB Local is already running"; \
		else \
			echo "Starting existing container..."; \
			docker start dynamodb-local; \
		fi \
	else \
		if [ -f docker-compose.yml ]; then \
			docker-compose up -d dynamodb-local; \
		else \
			docker run -d --name dynamodb-local -p 8000:8000 amazon/dynamodb-local -jar DynamoDBLocal.jar -inMemory -sharedDb; \
		fi \
	fi
	@echo "Waiting for DynamoDB Local to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		if curl -s http://localhost:8000 > /dev/null 2>&1; then \
			echo "✅ DynamoDB Local is ready!"; \
			break; \
		fi; \
		if [ $$i -eq 10 ]; then \
			echo "❌ Error: DynamoDB Local failed to start"; \
			exit 1; \
		fi; \
		echo "  Waiting... (attempt $$i/10)"; \
		sleep 2; \
	done

# Stop DynamoDB Local
docker-down:
	@echo "Stopping DynamoDB Local..."
	@if docker ps | grep -q dynamodb-local; then \
		docker stop dynamodb-local; \
	else \
		echo "DynamoDB Local is not running"; \
	fi

# Remove DynamoDB Local container (useful for cleanup)
docker-clean:
	@echo "Removing DynamoDB Local container..."
	@if docker ps -a | grep -q dynamodb-local; then \
		docker rm -f dynamodb-local; \
		echo "Container removed"; \
	else \
		echo "No dynamodb-local container found"; \
	fi

# Install development dependencies
install-tools:
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/golang/mock/mockgen@latest

# Generate mocks
generate:
	@echo "Generating mocks..."
	@go generate ./...

# Check for compilation errors
check:
	@echo "Checking for compilation errors..."
	@go build -o /dev/null ./... 2>&1 | grep -E "^#|error" || echo "✅ No compilation errors"

# Show test coverage in browser
coverage: test
	@echo "Generating coverage report..."
	@go tool cover -html=coverage.out

# Show coverage dashboard
coverage-dashboard:
	@./scripts/coverage-dashboard.sh

# Quick test for development (unit tests only, no race detector)
quick-test:
	@echo "Running quick tests (unit only, no race detector)..."
	@go test -short $(UNIT_PACKAGES)

# Team 1 specific targets
team1-test:
	@echo "Running Team 1 tests..."
	@go test -v ./pkg/core/... ./pkg/model/... ./pkg/types/... ./pkg/session/... ./pkg/errors/...

# Team 2 specific targets
team2-test:
	@echo "Running Team 2 tests..."
	@go test -v ./pkg/query/... ./internal/expr/... ./pkg/index/...

# Test examples separately
examples-test:
	@echo "Running example tests..."
	@go test -v ./examples/...

# Help target
help:
	@echo "DynamORM Makefile Commands:"
	@echo ""
	@echo "Build & Test:"
	@echo "  make build       - Build the project"
	@echo "  make test        - Run ALL tests (unit + integration) [STARTS DynamoDB Local]"
	@echo "  make test-unit   - Run only unit tests (fast, no Docker required)"
	@echo "  make integration - Run integration tests only (requires Docker)"
	@echo "  make test-all    - Run all tests including benchmarks and stress tests"
	@echo "  make benchmark   - Run performance benchmarks"
	@echo "  make stress      - Run stress tests"
	@echo ""
	@echo "Code Quality:"
	@echo "  make fmt         - Format code"
	@echo "  make lint        - Run linters"
	@echo "  make check       - Check for compilation errors"
	@echo "  make coverage    - Show test coverage in browser"
	@echo "  make coverage-dashboard - Show coverage dashboard in terminal"
	@echo ""
	@echo "Docker/DynamoDB:"
	@echo "  make docker-up   - Start DynamoDB Local"
	@echo "  make docker-down - Stop DynamoDB Local"
	@echo "  make docker-clean - Remove DynamoDB Local container"
	@echo ""
	@echo "Team/Specific Tests:"
	@echo "  make team1-test  - Run Team 1 specific tests"
	@echo "  make team2-test  - Run Team 2 specific tests"
	@echo "  make examples-test - Run example tests"
	@echo ""
	@echo "Lambda:"
	@echo "  make lambda-build - Build Lambda function example"
	@echo "  make lambda-test  - Test Lambda functionality"
	@echo "  make lambda-bench - Run Lambda benchmarks"
	@echo ""
	@echo "  make help        - Show this help message"

# Lambda-specific targets
LAMBDA_BUILD_FLAGS = -tags lambda -ldflags="-s -w"
GOOS = linux
GOARCH = amd64

# Build Lambda function example
lambda-build:
	@echo "Building Lambda function..."
	@mkdir -p build/lambda
	@cd examples/lambda && GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LAMBDA_BUILD_FLAGS) \
		-o ../../build/lambda/bootstrap main.go
	@cd build/lambda && zip function.zip bootstrap
	@echo "Lambda function built: build/lambda/function.zip"

# Test Lambda functionality
lambda-test:
	@echo "Running Lambda tests..."
	@go test -v ./lambda_test.go -run TestLambda

# Run Lambda benchmarks
lambda-bench:
	@echo "Running Lambda benchmarks..."
	@go test -bench=BenchmarkLambda -benchmem ./lambda_test.go 
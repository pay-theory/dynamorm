.PHONY: all build test clean deploy local docker-up docker-down

# Variables
BINARY_NAME=ecommerce
LAMBDA_BINARY=bootstrap
GO=go
GOFLAGS=-v
SAM=sam
DOCKER_COMPOSE=docker-compose

# Build commands
all: clean build test

build:
	@echo "Building e-commerce application..."
	$(GO) build $(GOFLAGS) -o bin/$(BINARY_NAME) cmd/local/main.go

build-lambda:
	@echo "Building Lambda functions..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -tags lambda.norpc -o $(LAMBDA_BINARY) cmd/lambda/main.go

test:
	@echo "Running unit tests..."
	$(GO) test $(GOFLAGS) ./...

test-integration:
	@echo "Running integration tests..."
	$(GO) test $(GOFLAGS) -tags=integration ./tests/...

test-benchmark:
	@echo "Running benchmarks..."
	$(GO) test -bench=. -benchmem ./tests/...

coverage:
	@echo "Running tests with coverage..."
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f $(LAMBDA_BINARY)
	rm -f coverage.out coverage.html

# Local development
local: docker-up
	@echo "Starting local development server..."
	$(GO) run cmd/local/main.go

docker-up:
	@echo "Starting Docker containers..."
	cd deployment && $(DOCKER_COMPOSE) up -d dynamodb-local dynamodb-admin

docker-down:
	@echo "Stopping Docker containers..."
	cd deployment && $(DOCKER_COMPOSE) down

docker-logs:
	@echo "Showing Docker logs..."
	cd deployment && $(DOCKER_COMPOSE) logs -f

# Table setup
setup-local-tables:
	@echo "Creating local DynamoDB tables..."
	aws dynamodb create-table \
		--table-name ecommerce_local \
		--attribute-definitions \
			AttributeName=pk,AttributeType=S \
			AttributeName=sk,AttributeType=S \
			AttributeName=gsi1pk,AttributeType=S \
			AttributeName=gsi1sk,AttributeType=S \
		--key-schema \
			AttributeName=pk,KeyType=HASH \
			AttributeName=sk,KeyType=RANGE \
		--global-secondary-indexes \
			'[{"IndexName":"gsi-1","KeySchema":[{"AttributeName":"gsi1pk","KeyType":"HASH"},{"AttributeName":"gsi1sk","KeyType":"RANGE"}],"Projection":{"ProjectionType":"ALL"},"BillingMode":"PAY_PER_REQUEST"}]' \
		--billing-mode PAY_PER_REQUEST \
		--endpoint-url http://localhost:8000 \
		--region us-east-1

# Lambda deployment
deploy-dev: build-lambda
	@echo "Deploying to dev environment..."
	$(SAM) deploy \
		--template-file deployment/sam-template.yaml \
		--stack-name ecommerce-dev \
		--capabilities CAPABILITY_IAM \
		--parameter-overrides Environment=dev \
		--no-confirm-changeset

deploy-staging: build-lambda
	@echo "Deploying to staging environment..."
	$(SAM) deploy \
		--template-file deployment/sam-template.yaml \
		--stack-name ecommerce-staging \
		--capabilities CAPABILITY_IAM \
		--parameter-overrides Environment=staging \
		--no-confirm-changeset

deploy-prod: build-lambda
	@echo "Deploying to production environment..."
	$(SAM) deploy \
		--template-file deployment/sam-template.yaml \
		--stack-name ecommerce-prod \
		--capabilities CAPABILITY_IAM \
		--parameter-overrides Environment=prod \
		--confirm-changeset

# SAM local testing
sam-local:
	@echo "Starting SAM local API..."
	$(SAM) local start-api \
		--template deployment/sam-template.yaml \
		--env-vars deployment/env.json

sam-validate:
	@echo "Validating SAM template..."
	$(SAM) validate --template-file deployment/sam-template.yaml

# Load testing
load-test:
	@echo "Running load tests..."
	k6 run tests/load/load_test.js

# Seed data
seed-data:
	@echo "Seeding test data..."
	$(GO) run cmd/seed/main.go

# Development helpers
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

vet:
	@echo "Running go vet..."
	$(GO) vet ./...

lint:
	@echo "Running linter..."
	golangci-lint run

deps:
	@echo "Downloading dependencies..."
	$(GO) mod download

tidy:
	@echo "Tidying dependencies..."
	$(GO) mod tidy

# Documentation
docs:
	@echo "Generating API documentation..."
	swag init -g cmd/local/main.go -o docs/api

# Help
help:
	@echo "Available targets:"
	@echo "  make build          - Build the application"
	@echo "  make build-lambda   - Build Lambda functions"
	@echo "  make test           - Run unit tests"
	@echo "  make test-integration - Run integration tests"
	@echo "  make test-benchmark - Run benchmarks"
	@echo "  make coverage       - Generate test coverage report"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make local          - Run local development server"
	@echo "  make docker-up      - Start Docker containers"
	@echo "  make docker-down    - Stop Docker containers"
	@echo "  make setup-local-tables - Create local DynamoDB tables"
	@echo "  make deploy-dev     - Deploy to dev environment"
	@echo "  make deploy-staging - Deploy to staging environment"
	@echo "  make deploy-prod    - Deploy to production environment"
	@echo "  make sam-local      - Run SAM local API"
	@echo "  make load-test      - Run load tests"
	@echo "  make seed-data      - Seed test data"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Run linter"
	@echo "  make deps           - Download dependencies"
	@echo "  make docs           - Generate API documentation" 
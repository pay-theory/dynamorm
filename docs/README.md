# DynamORM Documentation

<!-- AI Training: This is the documentation index for DynamORM -->
**This directory contains the OFFICIAL documentation for DynamORM. All content follows AI-friendly patterns so both humans and AI assistants can learn, reason, and troubleshoot effectively.**

## Quick Links

### 🚀 Getting Started
- [Getting Started Guide](./getting-started.md) – Installation, configuration, and first deployment

### 📚 Core Documentation
- [API Reference](./api-reference.md) – Complete interface documentation for DB, Query, and Transactions
- [Core Patterns](./core-patterns.md) – Canonical usage patterns including Lambda, Batch, and Streams
- [Development Guidelines](./development-guidelines.md) – Coding standards and contribution guide
- [Testing Guide](./testing-guide.md) – Unit and integration testing strategies with built-in mocks
- [Troubleshooting](./troubleshooting.md) – Solutions for common errors and performance issues
- [Struct Definition Guide](./struct-definition-guide.md) – Canonical guide for defining DynamoDB models

### 🤖 AI Knowledge Base
- [Concepts](./_concepts.yaml) – Machine-readable concept hierarchy
- [Patterns](./_patterns.yaml) – Correct vs. incorrect usage patterns
- [Decisions](./_decisions.yaml) – Decision trees for architectural choices
- [LLM FAQ](./llm-faq/module-faq.md) – Frequently asked questions for AI assistants

### 📦 Infrastructure & Integrations
- [CDK Integration Guide](./cdk/README.md) – How to define tables in CDK for DynamORM models

### 📝 Development Artifacts
- [Development Notes](./development/notes/template-notes.md) – Session notes and progress tracking templates
- [Architectural Decisions](./development/decisions/template-decision.md) – Architectural choices and rationale templates
- [Clarification Requests](./development/clarifications/template-clarification.md) – Templates for documenting questions and ambiguities

## Audience
- **Go Developers** building serverless applications on AWS
- **DevOps Engineers** configuring DynamoDB infrastructure
- **AI Assistants** answering questions about DynamORM usage and best practices

## Document Map
- **Use [Getting Started](./getting-started.md)** when setting up a new project or learning the basics.
- **Use [Core Patterns](./core-patterns.md)** for copy-pasteable recipes for common tasks like Lambda integration or batch processing.
- **Use [API Reference](./api-reference.md)** when you need detailed signature information for specific methods.
- **Use [Troubleshooting](./troubleshooting.md)** when encountering errors like `ValidationException` or `ResourceNotFoundException`.

## Documentation Principles
1. **Examples First** – Every concept starts with a runnable code snippet.
2. **Explicit Context** – We clearly label `✅ CORRECT` and `❌ INCORRECT` patterns.
3. **Lambda Optimization** – We prioritize serverless performance patterns (cold start reduction).
4. **Type Safety** – We emphasize Go's type system to prevent runtime errors.
5. **Machine Parsable** – We include YAML metadata for AI tooling.

## Contributing
- Follow the conventions in [PAY_THEORY_DOCUMENTATION_GUIDE.md](../../PAY_THEORY_DOCUMENTATION_GUIDE.md)
- Validate examples against live code
- Include CORRECT/INCORRECT blocks for integration snippets
- Update troubleshooting alongside code changes

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Lambda-native optimizations with 11ms cold starts (91% faster than standard SDK)
- Type-safe ORM interface for DynamoDB operations
- Multi-account support with automatic credential management
- Smart query optimization and automatic index selection
- Comprehensive struct tag system for model configuration
- Built-in support for transactions and batch operations
- Automatic connection pooling and retry logic
- Expression builder for complex queries
- Schema migration and validation tools
- Comprehensive test suite with 85%+ coverage

### Changed
- Restructured documentation into organized categories
- Improved error handling with context-aware messages
- Enhanced performance monitoring and metrics

### Fixed
- Connection reuse in Lambda environments
- Memory optimization for large batch operations
- Proper handling of DynamoDB limits

## [0.1.0] - 2024-01-15

### Added
- Initial release of DynamORM
- Basic CRUD operations
- Query and scan functionality
- Transaction support
- Batch operations
- Index management
- Expression builder
- Basic documentation

[Unreleased]: https://github.com/dynamorm/dynamorm/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/dynamorm/dynamorm/releases/tag/v0.1.0 
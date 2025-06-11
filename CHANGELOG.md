# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.2] - 2024-01-XX

### Added
- Pre-built mock implementations in `pkg/mocks` package
  - `MockDB` - implements `core.DB` interface
  - `MockQuery` - implements all 26+ methods of `core.Query` interface
  - `MockUpdateBuilder` - implements `core.UpdateBuilder` interface
- Comprehensive mocking documentation and examples
- Interface segregation proposal for future improvements

### Fixed
- Teams no longer need to implement all Query interface methods manually for testing
- Eliminates "trial and error" discovery of missing mock methods

## [1.0.1] - 2025-06-10

### Added
- Interface-based design for improved testability
  - New `core.DB` interface for basic operations
  - New `core.ExtendedDB` interface for full functionality
  - `NewBasic()` function that returns `core.DB` for simpler use cases
- Comprehensive testing documentation and examples
- Mock implementation examples for unit testing
- Runtime type checking for interface methods accepting `any` type

### Changed
- **BREAKING**: `dynamorm.New()` now returns `core.ExtendedDB` interface instead of `*dynamorm.DB`
- All methods that accept specific option types now accept `...any` with runtime validation
- Updated all examples and tests to use interfaces
- Improved separation between core operations and schema management

### Fixed
- Lambda.go now properly handles interface types
- Transaction callbacks properly use type assertions
- All test helper functions updated to return interfaces

### Migration Guide
See [Release Notes v1.0.1](docs/releases/v1.0.1-interface-improvements.md) for detailed migration instructions.

## [0.1.1] - 2025-06-10

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

## [0.1.0] - 2025-06-10

### Added
- Initial release of DynamORM
- Basic CRUD operations
- Query and scan functionality
- Transaction support
- Batch operations
- Index management
- Expression builder
- Basic documentation

[Unreleased]: https://github.com/dynamorm/dynamorm/compare/v1.0.2...HEAD
[1.0.2]: https://github.com/dynamorm/dynamorm/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/dynamorm/dynamorm/compare/v0.1.1...v1.0.1
[0.1.1]: https://github.com/dynamorm/dynamorm/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/dynamorm/dynamorm/releases/tag/v0.1.0 
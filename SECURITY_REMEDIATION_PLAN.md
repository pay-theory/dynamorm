# DynamORM Security Remediation Plan

**Version:** 1.0  
**Date:** December 2024  
**Priority:** Critical  
**Timeline:** 90 days  

## Executive Summary

This remediation plan addresses critical security vulnerabilities in DynamORM, with immediate focus on:
1. Eliminating unsafe pointer operations
2. Fixing all unhandled errors
3. Removing panic statements
4. Implementing comprehensive security controls

## Phase 1: Critical Issues (Week 1-2)

### 1.1 Remove Panic Statements ⚠️ CRITICAL

**Files to modify:**
- `pkg/session/session.go` (lines 160, 163)

**Current code:**
```go
func (s *Session) Client() *dynamodb.Client {
    if s == nil {
        panic("session is nil")
    }
    if s.client == nil {
        panic("DynamoDB client is nil")
    }
    return s.client
}
```

**Remediation:**
```go
func (s *Session) Client() (*dynamodb.Client, error) {
    if s == nil {
        return nil, fmt.Errorf("session is nil")
    }
    if s.client == nil {
        return nil, fmt.Errorf("DynamoDB client is nil")
    }
    return s.client, nil
}
```

**Impact:** This will require updating all callers of `Client()` to handle the error.

### 1.2 Replace Unsafe Marshaler ⚠️ CRITICAL

**Implementation Plan:**

1. **Create Safe Marshaler Interface**
```go
// pkg/marshal/interface.go
type Marshaler interface {
    MarshalItem(model any, metadata *model.Metadata) (map[string]types.AttributeValue, error)
}

type MarshalerType int

const (
    SafeMarshaler MarshalerType = iota
    UnsafeMarshaler // Deprecated
)
```

2. **Update DB Configuration**
```go
// pkg/session/config.go
type Config struct {
    // ... existing fields ...
    
    // MarshalerType determines which marshaler implementation to use
    // Defaults to SafeMarshaler for security
    MarshalerType MarshalerType
    
    // AllowUnsafeMarshaler must be explicitly set to true to use unsafe marshaler
    // Deprecated: Will be removed in v2.0
    AllowUnsafeMarshaler bool
}
```

3. **Implement Marshaler Factory**
```go
// pkg/marshal/factory.go
func NewMarshaler(config *session.Config) Marshaler {
    if config.AllowUnsafeMarshaler {
        log.Warn("SECURITY WARNING: Unsafe marshaler is deprecated and will be removed in v2.0")
        return newUnsafeMarshaler()
    }
    return newSafeMarshaler()
}
```

### 1.3 Fix Unhandled Errors

**Systematic Error Handling Review:**

1. **Search and Fix Pattern:** `_, err :=` → `if err != nil { return err }`
2. **Common locations:**
   - `multiaccount.go:224` - credential refresh errors
   - `examples/` - all example files
   - I/O operations without error checks

**Error Handling Standards:**
```go
// Bad
value, _ := someFunction()

// Good
value, err := someFunction()
if err != nil {
    return fmt.Errorf("context description: %w", err)
}
```

## Phase 2: High Priority Issues (Week 3-4)

### 2.1 Implement Structured Logging

**Create Logger Interface:**
```go
// pkg/logger/logger.go
package logger

type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
    WithContext(ctx context.Context) Logger
}

type Field struct {
    Key   string
    Value any
}

// Default implementation
type defaultLogger struct {
    level LogLevel
}

var DefaultLogger Logger = &defaultLogger{level: InfoLevel}
```

**Replace All Direct Output:**
- Replace `fmt.Printf` → `logger.Info`
- Replace `fmt.Println` → `logger.Debug`
- Never log sensitive data (credentials, keys)

### 2.2 Fix Resource Exhaustion Vulnerabilities

**Unbounded Reads:**
```go
// Bad
bodyBytes, _ := ioutil.ReadAll(r.Body)

// Good
const maxBodySize = 10 * 1024 * 1024 // 10MB
bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
if err != nil {
    return fmt.Errorf("reading body: %w", err)
}
```

**Implementation locations:**
- `examples/ecommerce/cmd/local/main.go:219`
- All HTTP handlers in examples

### 2.3 Improve Concurrent Operations

**Add Proper Error Propagation:**
```go
// Bad
go func() {
    err := doWork()
    // Error is lost
}()

// Good
errCh := make(chan error, 1)
go func() {
    errCh <- doWork()
}()

select {
case err := <-errCh:
    if err != nil {
        return fmt.Errorf("concurrent work failed: %w", err)
    }
case <-ctx.Done():
    return ctx.Err()
}
```

## Phase 3: Medium Priority Issues (Week 5-8)

### 3.1 Configuration Management

**Create Configuration Constants:**
```go
// pkg/config/constants.go
package config

const (
    // DynamoDB limits
    DefaultBatchSize     = 25
    MaxBatchSize         = 25
    DefaultRetryAttempts = 3
    MaxRetryAttempts     = 10
    
    // Timeouts
    DefaultTimeout       = 30 * time.Second
    DefaultRetryDelay    = 100 * time.Millisecond
    MaxRetryDelay        = 5 * time.Second
    
    // Lambda
    DefaultLambdaBuffer  = 500 * time.Millisecond
)
```

### 3.2 Input Validation Layer

**Implement Validation:**
```go
// pkg/validation/validator.go
package validation

import "regexp"

var (
    tableNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
    maxTableNameLen = 255
)

func ValidateTableName(name string) error {
    if len(name) == 0 {
        return fmt.Errorf("table name cannot be empty")
    }
    if len(name) > maxTableNameLen {
        return fmt.Errorf("table name exceeds maximum length of %d", maxTableNameLen)
    }
    if !tableNameRegex.MatchString(name) {
        return fmt.Errorf("table name contains invalid characters")
    }
    return nil
}
```

### 3.3 Rate Limiting

**Implement Token Bucket:**
```go
// pkg/ratelimit/limiter.go
package ratelimit

import "golang.org/x/time/rate"

type Limiter struct {
    readLimiter  *rate.Limiter
    writeLimiter *rate.Limiter
}

func NewLimiter(readRPS, writeRPS int) *Limiter {
    return &Limiter{
        readLimiter:  rate.NewLimiter(rate.Limit(readRPS), readRPS),
        writeLimiter: rate.NewLimiter(rate.Limit(writeRPS), writeRPS),
    }
}
```

## Phase 4: Security Enhancements (Week 9-12)

### 4.1 Security Testing Suite

**Create Security Tests:**
```go
// tests/security/marshaler_test.go
func TestMarshalerMemorySafety(t *testing.T) {
    // Test with malformed data
    // Test with concurrent access
    // Test with large inputs
}

func FuzzExpressionBuilder(f *testing.F) {
    // Add seed corpus
    f.Add("field = value")
    f.Add("field > 100")
    
    f.Fuzz(func(t *testing.T, expr string) {
        // Should not panic
        _, err := parseExpression(expr)
        if err != nil {
            // Expected for invalid input
            return
        }
    })
}
```

### 4.2 AWS Security Integration

**CloudTrail Support:**
```go
// pkg/cloudtrail/config.go
type CloudTrailConfig struct {
    Enabled            bool
    LogDataPlaneEvents bool
    LogGroup          string
}
```

**KMS Integration:**
```go
// pkg/encryption/kms.go
type KMSConfig struct {
    KeyID              string
    EncryptionContext  map[string]string
    GrantTokens        []string
}
```

## Implementation Checklist

### Week 1-2: Critical Issues
- [ ] Remove all panic statements
- [ ] Create safe marshaler implementation
- [ ] Update all Client() method callers
- [ ] Fix critical unhandled errors
- [ ] Create error handling guidelines

### Week 3-4: High Priority
- [ ] Implement structured logging
- [ ] Replace all fmt.Print statements
- [ ] Fix unbounded reads
- [ ] Add proper goroutine error handling
- [ ] Create concurrent operation tests

### Week 5-8: Medium Priority
- [ ] Extract all hardcoded values
- [ ] Implement input validation
- [ ] Add rate limiting
- [ ] Improve timeout handling
- [ ] Update all examples

### Week 9-12: Security Enhancements
- [ ] Add security test suite
- [ ] Implement fuzz testing
- [ ] Add CloudTrail integration
- [ ] Document security best practices
- [ ] Performance benchmarks for safe marshaler

## Migration Guide

### For Library Users

1. **Update to Safe Marshaler (Default in v1.5)**
```go
// No action needed - safe marshaler is default
db, err := dynamorm.New(config)
```

2. **Temporary Unsafe Marshaler Usage (Deprecated)**
```go
// Will show deprecation warning
config := dynamorm.Config{
    AllowUnsafeMarshaler: true, // Will be removed in v2.0
}
```

3. **Handle New Error Returns**
```go
// Old
client := session.Client()

// New
client, err := session.Client()
if err != nil {
    return fmt.Errorf("getting client: %w", err)
}
```

### For Contributors

1. **Error Handling Standard:**
   - Always check errors
   - Wrap errors with context
   - Never ignore errors with `_`

2. **Logging Standard:**
   - Use structured logger
   - Never log secrets
   - Use appropriate log levels

3. **Testing Standard:**
   - Include error cases
   - Test concurrent access
   - Add security-focused tests

## Rollback Plan

If issues arise during remediation:

1. **Version Tags:**
   - Tag current version as `v1.4-pre-security`
   - All changes in feature branches
   - Gradual rollout with feature flags

2. **Compatibility Mode:**
   - Maintain old interfaces with deprecation
   - Provide migration tools
   - Support period of 6 months

## Success Metrics

1. **Security:**
   - Zero panics in library code
   - 100% error handling coverage
   - All unsafe operations removed/isolated

2. **Performance:**
   - Safe marshaler within 50% of unsafe
   - No regression in p99 latency
   - Memory usage stable

3. **Adoption:**
   - 90% users on safe marshaler within 3 months
   - Zero security incidents reported
   - Positive community feedback

## Communication Plan

1. **Security Advisory:**
   - CVE request for unsafe marshaler
   - Blog post explaining changes
   - Migration guide published

2. **Community Engagement:**
   - RFC for API changes
   - Preview releases
   - Office hours for migration help

3. **Documentation Updates:**
   - Security best practices guide
   - Updated examples
   - Performance tuning guide

## Appendix: Error Handling Patterns

### Pattern 1: Wrap with Context
```go
func (db *DB) Create(item any) error {
    result, err := db.marshaler.Marshal(item)
    if err != nil {
        return fmt.Errorf("marshaling item: %w", err)
    }
    // ...
}
```

### Pattern 2: Sentinel Errors
```go
var (
    ErrSessionNil = errors.New("session is nil")
    ErrClientNil  = errors.New("client is nil")
)
```

### Pattern 3: Error Types
```go
type ValidationError struct {
    Field string
    Value any
    Reason string
}

func (e ValidationError) Error() string {
    return fmt.Sprintf("validation failed for %s: %s", e.Field, e.Reason)
}
```

## Appendix B: Specific Error Handling Fixes Required

### Critical Files - Core Library
1. **pkg/transaction/transaction.go**
   - Line 141: `av, _ := tx.converter.ToAttributeValue(currentVersion)`
   - Line 146: `newAv, _ := tx.converter.ToAttributeValue(currentVersion + 1)`
   - Line 168: `av, _ := tx.converter.ToAttributeValue(time.Now())`
   - Line 226: `av, _ := tx.converter.ToAttributeValue(versionValue.Interface())`

2. **pkg/schema/manager.go**
   - Line 437: `desiredGSIs, _ := m.buildIndexes(metadata)`

3. **lambda.go**
   - Line 402: `cfg, _ := config.LoadDefaultConfig(context.Background())`
   - Line 412: `db, _ := NewLambdaOptimized()`

4. **multiaccount.go**
   - Line 221: Missing error check for `createPartnerDB`

### High Priority - Examples (Security Risk)
1. **examples/ecommerce/cmd/local/main.go**
   - Line 219: `bodyBytes, _ := ioutil.ReadAll(r.Body)` ⚠️ UNBOUNDED READ

2. **examples/blog/services/webhook_provider.go**
   - Line 138: `body, _ := io.ReadAll(resp.Body)` ⚠️ UNBOUNDED READ

3. **examples/payment/utils/idempotency.go**
   - Line 82: `responseData, _ = json.Marshal(map[string]string{"error": fnErr.Error()})`
   - Line 109: `jsonData, _ := json.Marshal(data)`

### Medium Priority - Example Handlers
1. **examples/payment/lambda/query/handler.go**
   - Lines 247, 252: Date parsing without error handling
   - Lines 365, 381: JSON marshaling errors ignored

2. **examples/blog/handlers/posts.go**
   - Line 74: `limit, _ := strconv.Atoi(...)`
   - Lines 723, 739: JSON marshaling errors ignored

3. **examples/ecommerce/handlers/orders.go**
   - Lines 278, 298, 318: Cursor marshaling errors ignored
   - Line 660: JSON marshaling error ignored

4. **examples/ecommerce/handlers/inventory.go**
   - Lines 147, 640: Cursor marshaling errors ignored

### Error Handling Fix Templates

#### Template 1: Converter Errors
```go
// Before
av, _ := tx.converter.ToAttributeValue(currentVersion)

// After
av, err := tx.converter.ToAttributeValue(currentVersion)
if err != nil {
    return fmt.Errorf("converting version to attribute value: %w", err)
}
```

#### Template 2: Unbounded Reads
```go
// Before
bodyBytes, _ := ioutil.ReadAll(r.Body)

// After
const maxBodySize = 10 * 1024 * 1024 // 10MB
bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
if err != nil {
    http.Error(w, "Failed to read request body", http.StatusBadRequest)
    return
}
```

#### Template 3: JSON Marshaling
```go
// Before
body, _ := json.Marshal(response)

// After
body, err := json.Marshal(response)
if err != nil {
    // Log error internally
    logger.Error("failed to marshal response", "error", err)
    // Return generic error to client
    http.Error(w, "Internal server error", http.StatusInternalServerError)
    return
}
```

#### Template 4: Strconv Parsing
```go
// Before
limit, _ := strconv.Atoi(request.QueryStringParameters["limit"])

// After
limitStr := request.QueryStringParameters["limit"]
limit := 10 // default
if limitStr != "" {
    parsedLimit, err := strconv.Atoi(limitStr)
    if err != nil {
        return BadRequestError("Invalid limit parameter")
    }
    limit = parsedLimit
}
```

### Automated Detection Script
```bash
#!/bin/bash
# Find all ignored errors in Go files

echo "=== Files with ignored errors ==="
grep -n "_, err :=" $(find . -name "*.go" -not -path "*_test.go") | grep -v "// ignore"

echo -e "\n=== Files with underscore assignment ==="
grep -n "_\s*=" $(find . -name "*.go" -not -path "*_test.go") | grep -E "=.*\("

echo -e "\n=== Potential unbounded reads ==="
grep -n "ioutil.ReadAll\|io.ReadAll" $(find . -name "*.go") | grep -v "LimitReader"
```

---

**Document Version:** 1.0  
**Last Updated:** December 2024  
**Next Review:** January 2025  
**Owner:** DynamORM Security Team 
# AI Assistant Prompt: DynamORM Security Remediation

## Context
You are a security engineer tasked with hardening the DynamORM library against security vulnerabilities. Your focus is on eliminating memory safety issues, preventing injection attacks, implementing proper input validation, and securing data handling.

## Your Mission
Address the following critical security vulnerabilities in order of priority:

### PRIMARY TARGETS (Week 1-2 - CRITICAL)

#### 1. SECURE UNSAFE MEMORY OPERATIONS
**Objective:** Eliminate or safely isolate the extensive `unsafe` package usage in the marshaler

**Target File:** `pkg/marshal/marshaler.go`
**Risk:** Memory corruption, buffer overflows, arbitrary code execution

**Current Dangerous Patterns:**
```go
// Direct memory access without bounds checking
fieldPtr := unsafe.Add(ptr, fm.offset)
val := *(*int64)(fieldPtr)

// Type punning bypassing Go's type safety  
s := *(*string)(ptr)
i := *(*int)(ptr)

// Stack allocation risk
ptr = unsafe.Pointer(v.UnsafeAddr())
```

**Your Task: Implement Dual Marshaler Strategy**

1. **Create Safe Marshaler (Default)**
```go
// pkg/marshal/safe_marshaler.go
type SafeMarshaler struct {
    cache sync.Map
}

func (m *SafeMarshaler) MarshalItem(model any, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
    // Use reflection-based marshaling - SAFE
    v := reflect.ValueOf(model)
    if v.Kind() == reflect.Ptr {
        v = v.Elem()
    }
    
    // Safe field access using reflection
    for _, fieldMeta := range metadata.Fields {
        fieldValue := v.Field(fieldMeta.Index)
        // Convert safely without unsafe operations
    }
}
```

2. **Isolate Unsafe Marshaler with Warnings**
```go
// pkg/marshal/unsafe_marshaler.go
// +build unsafe

// SECURITY WARNING: This marshaler uses unsafe operations
type UnsafeMarshaler struct {
    // Add bounds checking to existing unsafe code
}

func NewUnsafeMarshaler() *UnsafeMarshaler {
    log.Warn("SECURITY WARNING: Using unsafe marshaler. Deprecated in v2.0")
    return &UnsafeMarshaler{}
}
```

3. **Secure Configuration**
```go
type Config struct {
    // Default to SAFE marshaler
    MarshalerType MarshalerType `default:"safe"`
    
    // Explicit opt-in required for unsafe operations
    AllowUnsafeMarshaler bool `json:"-"` // Don't serialize this
}

func NewMarshalerFactory(config *Config) Marshaler {
    if config.AllowUnsafeMarshaler {
        // Log security warning
        // Require explicit acknowledgment
        return NewUnsafeMarshaler()
    }
    return NewSafeMarshaler() // Default
}
```

**Security Requirements:**
- Add runtime bounds checking to any remaining unsafe code
- Validate struct field offsets before memory access
- Add memory alignment checks
- Implement stack vs heap detection
- Create comprehensive memory safety tests

#### 2. PREVENT EXPRESSION INJECTION ATTACKS
**Objective:** Secure the expression builder against injection attacks

**Target Files:** 
- `internal/expr/builder.go`
- `pkg/query/update_builder.go`
- `pkg/transaction/transaction.go`

**Current Vulnerable Patterns:**
```go
// Direct string formatting with user input - DANGEROUS
expr := fmt.Sprintf("%s[%d]", field, index)  
updateExpression += fmt.Sprintf("%s = %s", attrName, attrValue)
conditionExpression := fmt.Sprintf("attribute_not_exists(#pk)")
```

**Your Task: Implement Secure Expression Building**

1. **Input Validation Layer**
```go
// pkg/validation/field_validator.go
func ValidateFieldName(field string) error {
    // AWS DynamoDB attribute name rules
    if len(field) == 0 {
        return fmt.Errorf("field name cannot be empty")
    }
    if len(field) > 255 {
        return fmt.Errorf("field name exceeds 255 characters")
    }
    
    // Check for injection attempts
    if containsSQLMetachars(field) {
        return fmt.Errorf("field name contains invalid characters")
    }
    
    return nil
}

func ValidateOperator(op string) error {
    allowedOps := map[string]bool{
        "=": true, "!=": true, "<": true, "<=": true, 
        ">": true, ">=": true, "BETWEEN": true, "IN": true,
        "BEGINS_WITH": true, "CONTAINS": true, "EXISTS": true,
    }
    
    if !allowedOps[strings.ToUpper(op)] {
        return fmt.Errorf("invalid operator: %s", op)
    }
    return nil
}
```

2. **Secure Expression Builder**
```go
func (b *Builder) buildCondition(field string, operator string, value any) (string, error) {
    // SECURITY: Validate all inputs
    if err := ValidateFieldName(field); err != nil {
        return "", fmt.Errorf("invalid field: %w", err)
    }
    
    if err := ValidateOperator(operator); err != nil {
        return "", fmt.Errorf("invalid operator: %w", err)
    }
    
    // Use ONLY parameterized expressions
    nameRef := b.addName(field)     // Safe placeholder
    valueRef := b.addValue(value)   // Safe placeholder
    
    // No direct string interpolation
    return fmt.Sprintf("%s %s %s", nameRef, operator, valueRef), nil
}
```

3. **Secure Reserved Word Handling**
```go
func (b *Builder) addName(name string) string {
    // Validate name before processing
    if err := ValidateFieldName(name); err != nil {
        // Log security violation attempt
        log.Warn("Invalid field name rejected", "field", name, "error", err)
        return "#invalid"
    }
    
    // Safe placeholder generation
    placeholder := fmt.Sprintf("#f%d", b.nameCounter)
    b.names[placeholder] = name
    b.nameCounter++
    return placeholder
}
```

#### 3. ELIMINATE INFORMATION DISCLOSURE
**Objective:** Prevent sensitive information leakage through logs and error messages

**Target Files:**
- `multiaccount.go:224` (credential logging)
- `pkg/errors/errors.go` (error message content)
- All `fmt.Printf` statements in production code

**Current Vulnerabilities:**
```go
// DANGEROUS: Logging credentials and internal details
fmt.Printf("Failed to refresh credentials for partner %s: %v\n", partnerID, err)

// Error messages exposing internal structure
return fmt.Errorf("failed to marshal field %s: %w", fieldName, err)
```

**Your Task: Implement Secure Logging**

1. **Create Security-Aware Logger**
```go
// pkg/logger/secure_logger.go
type SecureLogger struct {
    underlying Logger
    sanitizer  *LogSanitizer
}

type LogSanitizer struct {
    sensitivePatterns []string
    replacements      map[string]string
}

func (l *SecureLogger) Error(msg string, fields ...Field) {
    // Sanitize all fields before logging
    sanitized := l.sanitizer.SanitizeFields(fields)
    l.underlying.Error(msg, sanitized...)
}

func (s *LogSanitizer) SanitizeFields(fields []Field) []Field {
    sanitized := make([]Field, len(fields))
    for i, field := range fields {
        if s.isSensitive(field.Key, field.Value) {
            sanitized[i] = Field{Key: field.Key, Value: "[REDACTED]"}
        } else {
            sanitized[i] = field
        }
    }
    return sanitized
}
```

2. **Secure Error Messages**
```go
// pkg/errors/secure_errors.go
func NewSecureError(operation string, internalErr error) error {
    // Generate operation ID for correlation
    opID := generateOperationID()
    
    // Log detailed error internally (for debugging)
    log.Error("Operation failed", 
        "operation_id", opID,
        "operation", operation,
        "internal_error", internalErr,
    )
    
    // Return sanitized error to user
    return fmt.Errorf("operation failed: operation_id=%s", opID)
}
```

3. **Replace All Direct Output**
```go
// Replace this DANGEROUS pattern:
fmt.Printf("Failed to refresh credentials for partner %s: %v\n", partnerID, err)

// With this SECURE pattern:
log.Error("Credential refresh failed",
    "partner_id", sanitizePartnerID(partnerID),
    "operation_id", opID,
    // Don't log the actual error details
)
```

### SECONDARY TARGETS (Week 3-4)

#### 4. IMPLEMENT RESOURCE PROTECTION
**Objective:** Prevent resource exhaustion and DoS attacks

**Target Areas:**
- HTTP request body handling
- Batch operation limits
- Memory allocation controls

**Current Vulnerabilities:**
```go
// Unbounded reads - DANGEROUS
bodyBytes, _ := ioutil.ReadAll(r.Body)

// No batch size limits
// No memory allocation controls
```

**Your Task: Add Resource Limits**

1. **HTTP Request Protection**
```go
const (
    MaxRequestBodySize = 10 * 1024 * 1024 // 10MB
    MaxRequestTimeout  = 30 * time.Second
)

func secureBodyReader(r *http.Request) ([]byte, error) {
    // Limit request size
    body := http.MaxBytesReader(nil, r.Body, MaxRequestBodySize)
    
    // Add timeout
    ctx, cancel := context.WithTimeout(r.Context(), MaxRequestTimeout)
    defer cancel()
    
    // Read with limits
    bodyBytes, err := io.ReadAll(body)
    if err != nil {
        return nil, fmt.Errorf("reading request body: %w", err)
    }
    
    return bodyBytes, nil
}
```

2. **Batch Operation Limits**
```go
const (
    MaxBatchSize        = 25  // DynamoDB limit
    MaxConcurrentBatch  = 10  // Concurrent operations
    BatchRateLimit      = 100 // Operations per second
)

type BatchLimiter struct {
    semaphore chan struct{}
    rateLimiter *rate.Limiter
}

func (bl *BatchLimiter) Acquire(ctx context.Context) error {
    // Rate limiting
    if err := bl.rateLimiter.Wait(ctx); err != nil {
        return err
    }
    
    // Concurrency limiting
    select {
    case bl.semaphore <- struct{}{}:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

#### 5. SECURE INPUT VALIDATION
**Objective:** Validate all external inputs to prevent malformed data attacks

**Validation Requirements:**
- Table name format (AWS rules)
- Attribute name length and format
- Index name validation
- Value type and size validation

**Implementation:**
```go
// pkg/validation/aws_validator.go
func ValidateTableName(name string) error {
    if len(name) < 3 || len(name) > 255 {
        return fmt.Errorf("table name must be 3-255 characters")
    }
    
    // AWS table name pattern
    pattern := `^[a-zA-Z0-9_.-]+$`
    if matched, _ := regexp.MatchString(pattern, name); !matched {
        return fmt.Errorf("invalid table name format")
    }
    
    return nil
}

func ValidateAttributeValue(value any) error {
    // Check value size limits
    // Validate data types
    // Prevent oversized payloads
}
```

## Implementation Guidelines

### Security-First Principles
1. **Validate All Inputs:** Never trust external data
2. **Fail Securely:** Default to deny/error rather than allow
3. **Minimize Information Disclosure:** Log internally, return generic errors
4. **Defense in Depth:** Multiple layers of protection
5. **Principle of Least Privilege:** Minimal required permissions

### Secure Coding Patterns
```go
// SECURE: Input validation + parameterized operations
func (b *Builder) SecureAddCondition(field, op string, value any) error {
    if err := ValidateField(field); err != nil {
        return &SecurityError{Type: "InvalidField", Detail: "field validation failed"}
    }
    
    if err := ValidateOperator(op); err != nil {
        return &SecurityError{Type: "InvalidOperator", Detail: "operator validation failed"}
    }
    
    // Use parameterized approach only
    return b.addParameterizedCondition(field, op, value)
}

// INSECURE: Direct string manipulation
func (b *Builder) InsecureAddCondition(field, op string, value any) {
    expr := fmt.Sprintf("%s %s %v", field, op, value) // DON'T DO THIS
}
```

### Memory Safety Patterns
```go
// SAFE: Bounds checking before unsafe operations
func (m *UnsafeMarshaler) safeFieldAccess(ptr unsafe.Pointer, offset uintptr, structSize uintptr) (unsafe.Pointer, error) {
    // Validate offset is within struct bounds
    if offset >= structSize {
        return nil, fmt.Errorf("offset %d exceeds struct size %d", offset, structSize)
    }
    
    // Check for overflow
    if uintptr(ptr) + offset < uintptr(ptr) {
        return nil, fmt.Errorf("pointer arithmetic overflow")
    }
    
    return unsafe.Add(ptr, offset), nil
}
```

## Security Testing Requirements

### 1. Injection Attack Tests
```go
func TestExpressionInjection(t *testing.T) {
    maliciousInputs := []string{
        "field'; DROP TABLE users; --",
        "field\x00malicious",
        strings.Repeat("a", 1000000), // DoS attempt
    }
    
    for _, input := range maliciousInputs {
        err := builder.AddCondition(input, "=", "value")
        assert.Error(t, err, "Should reject malicious input: %s", input)
    }
}
```

### 2. Memory Safety Tests
```go
func TestMarshalerMemorySafety(t *testing.T) {
    // Test with various struct sizes
    // Test with malformed structs
    // Test concurrent access
    // Test with nil pointers
}
```

### 3. Resource Exhaustion Tests
```go
func TestResourceLimits(t *testing.T) {
    // Test oversized requests
    // Test too many concurrent operations
    // Test memory allocation limits
}
```

### 4. Information Disclosure Tests
```go
func TestNoInformationLeakage(t *testing.T) {
    // Verify error messages don't expose internals
    // Check logs don't contain sensitive data
    // Validate stack traces are not exposed
}
```

## Success Criteria
- [ ] All unsafe operations eliminated or secured with bounds checking
- [ ] Zero expression injection vulnerabilities
- [ ] No sensitive information in logs or error messages
- [ ] Resource limits implemented and tested
- [ ] Comprehensive input validation for all external data
- [ ] Security test suite passes with 100% coverage
- [ ] Performance degradation <20% from security measures

## Tools and Techniques
1. **Static Analysis:** `gosec`, `staticcheck` for security issues
2. **Fuzzing:** `go-fuzz` for input validation testing  
3. **Memory Analysis:** `go test -race`, valgrind for memory safety
4. **Security Scanning:** SAST tools for vulnerability detection

## Deliverables
1. **Secure marshaler implementation** with dual safe/unsafe options
2. **Hardened expression builder** with injection prevention
3. **Secure logging system** with data sanitization
4. **Resource protection mechanisms** with configurable limits
5. **Comprehensive security test suite** with attack simulation
6. **Security documentation** with threat model and mitigations
7. **Performance benchmarks** showing security vs speed trade-offs

## Important Security Notes
- **Zero Trust:** Validate everything, trust nothing
- **Security by Default:** Safe options should be the default
- **Fail Fast:** Reject invalid input immediately
- **Audit Trail:** Log security events for investigation
- **Regular Updates:** Keep security measures current

## Questions to Ask Yourself
1. "Can an attacker manipulate this input?"
2. "What's the worst case if this validation fails?"
3. "Are we exposing sensitive information?"
4. "Can this be used for DoS attacks?"
5. "How can I test this security measure?"

---

**Priority:** CRITICAL - Week 1-2 deliverable  
**Success Metric:** Zero security vulnerabilities in production  
**Testing:** Must pass security test suite and penetration testing 
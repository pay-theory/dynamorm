package validation

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// SecurityError represents a security validation error
type SecurityError struct {
	Type   string
	Field  string
	Detail string
}

func (e *SecurityError) Error() string {
	return fmt.Sprintf("security validation failed [%s]: %s - %s", e.Type, e.Field, e.Detail)
}

// Field validation constants
const (
	MaxFieldNameLength   = 255
	MaxOperatorLength    = 20
	MaxValueStringLength = 400000 // DynamoDB item size limit
	MaxNestedDepth       = 32
	MaxExpressionLength  = 4096
)

// SQL injection and dangerous patterns
var dangerousPatterns = []string{
	"'", "\"", ";", "--", "/*", "*/", "union", "select", "insert", "update", "delete",
	"drop", "create", "alter", "exec", "execute", "script", "javascript", "vbscript",
	"<script", "</script", "eval(", "expression(", "import(", "require(",
}

// Valid operator whitelist
var allowedOperators = map[string]bool{
	"=":                    true,
	"!=":                   true,
	"<>":                   true,
	"<":                    true,
	"<=":                   true,
	">":                    true,
	">=":                   true,
	"BETWEEN":              true,
	"IN":                   true,
	"BEGINS_WITH":          true,
	"CONTAINS":             true,
	"EXISTS":               true,
	"NOT_EXISTS":           true,
	"ATTRIBUTE_EXISTS":     true,
	"ATTRIBUTE_NOT_EXISTS": true,
	"EQ":                   true,
	"NE":                   true,
	"LT":                   true,
	"LE":                   true,
	"GT":                   true,
	"GE":                   true,
}

// ValidateFieldName validates a DynamoDB attribute name according to AWS rules and security best practices
func ValidateFieldName(field string) error {
	if field == "" {
		return &SecurityError{
			Type:   "InvalidField",
			Field:  field,
			Detail: "field name cannot be empty",
		}
	}

	if len(field) > MaxFieldNameLength {
		return &SecurityError{
			Type:   "InvalidField",
			Field:  field,
			Detail: fmt.Sprintf("field name exceeds maximum length of %d characters", MaxFieldNameLength),
		}
	}

	// Check for dangerous patterns
	fieldLower := strings.ToLower(field)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(fieldLower, pattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  field,
				Detail: fmt.Sprintf("field name contains dangerous pattern: %s", pattern),
			}
		}
	}

	// Check for control characters
	for _, r := range field {
		if unicode.IsControl(r) {
			return &SecurityError{
				Type:   "InvalidField",
				Field:  field,
				Detail: "field name contains control characters",
			}
		}
	}

	// Validate nested field paths
	if strings.Contains(field, ".") {
		parts := strings.Split(field, ".")
		if len(parts) > MaxNestedDepth {
			return &SecurityError{
				Type:   "InvalidField",
				Field:  field,
				Detail: fmt.Sprintf("nested field depth exceeds maximum of %d", MaxNestedDepth),
			}
		}

		for _, part := range parts {
			if err := validateFieldPart(part); err != nil {
				return &SecurityError{
					Type:   "InvalidField",
					Field:  field,
					Detail: fmt.Sprintf("invalid field part '%s': %s", part, err.Error()),
				}
			}
		}
	} else {
		return validateFieldPart(field)
	}

	return nil
}

// validateFieldPart validates a single part of a field name
func validateFieldPart(part string) error {
	if part == "" {
		return fmt.Errorf("field part cannot be empty")
	}

	// AWS DynamoDB attribute name rules
	validPattern := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	if !validPattern.MatchString(part) {
		return fmt.Errorf("field part must start with letter or underscore and contain only alphanumeric characters and underscores")
	}

	return nil
}

// ValidateOperator validates a DynamoDB condition operator
func ValidateOperator(op string) error {
	if op == "" {
		return &SecurityError{
			Type:   "InvalidOperator",
			Field:  op,
			Detail: "operator cannot be empty",
		}
	}

	if len(op) > MaxOperatorLength {
		return &SecurityError{
			Type:   "InvalidOperator",
			Field:  op,
			Detail: fmt.Sprintf("operator exceeds maximum length of %d characters", MaxOperatorLength),
		}
	}

	// Check against whitelist
	opUpper := strings.ToUpper(strings.TrimSpace(op))
	if !allowedOperators[opUpper] {
		return &SecurityError{
			Type:   "InvalidOperator",
			Field:  op,
			Detail: fmt.Sprintf("operator '%s' is not allowed", op),
		}
	}

	// Check for dangerous patterns
	opLower := strings.ToLower(op)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(opLower, pattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  op,
				Detail: fmt.Sprintf("operator contains dangerous pattern: %s", pattern),
			}
		}
	}

	return nil
}

// ValidateValue validates a value used in DynamoDB expressions
func ValidateValue(value any) error {
	if value == nil {
		return nil // NULL values are allowed
	}

	switch v := value.(type) {
	case string:
		return validateStringValue(v)
	case []any:
		return validateSliceValue(v)
	case map[string]any:
		return validateMapValue(v)
	default:
		// For other types (int, float, bool, etc.), basic validation
		return validateBasicValue(v)
	}
}

// validateStringValue validates string values
func validateStringValue(s string) error {
	if len(s) > MaxValueStringLength {
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "string_value",
			Detail: fmt.Sprintf("string value exceeds maximum length of %d characters", MaxValueStringLength),
		}
	}

	// Check for dangerous patterns in string values
	stringLower := strings.ToLower(s)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(stringLower, pattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  "string_value",
				Detail: fmt.Sprintf("string value contains dangerous pattern: %s", pattern),
			}
		}
	}

	return nil
}

// validateSliceValue validates slice values (for IN operator, etc.)
func validateSliceValue(slice []any) error {
	if len(slice) > 100 { // DynamoDB IN operator limit
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "slice_value",
			Detail: "slice value exceeds maximum length of 100 items",
		}
	}

	for i, item := range slice {
		if err := ValidateValue(item); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "slice_value",
				Detail: fmt.Sprintf("invalid item at index %d: %s", i, err.Error()),
			}
		}
	}

	return nil
}

// validateMapValue validates map values
func validateMapValue(m map[string]any) error {
	if len(m) > 100 { // Reasonable limit for map size
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "map_value",
			Detail: "map value exceeds maximum of 100 keys",
		}
	}

	for key, value := range m {
		if err := ValidateFieldName(key); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "map_key",
				Detail: fmt.Sprintf("invalid map key '%s': %s", key, err.Error()),
			}
		}

		if err := ValidateValue(value); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "map_value",
				Detail: fmt.Sprintf("invalid map value for key '%s': %s", key, err.Error()),
			}
		}
	}

	return nil
}

// validateBasicValue validates basic types (int, float, bool)
func validateBasicValue(value any) error {
	// Basic type validation - these are generally safe
	switch value.(type) {
	case int, int8, int16, int32, int64:
		return nil
	case uint, uint8, uint16, uint32, uint64:
		return nil
	case float32, float64:
		return nil
	case bool:
		return nil
	default:
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "basic_value",
			Detail: fmt.Sprintf("unsupported value type: %T", value),
		}
	}
}

// ValidateExpression validates a complete expression for security
func ValidateExpression(expression string) error {
	if len(expression) > MaxExpressionLength {
		return &SecurityError{
			Type:   "InvalidExpression",
			Field:  "expression",
			Detail: fmt.Sprintf("expression exceeds maximum length of %d characters", MaxExpressionLength),
		}
	}

	// Check for dangerous patterns
	exprLower := strings.ToLower(expression)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(exprLower, pattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  "expression",
				Detail: fmt.Sprintf("expression contains dangerous pattern: %s", pattern),
			}
		}
	}

	return nil
}

// ValidateTableName validates a DynamoDB table name
func ValidateTableName(name string) error {
	if len(name) < 3 || len(name) > 255 {
		return &SecurityError{
			Type:   "InvalidTableName",
			Field:  name,
			Detail: "table name must be 3-255 characters",
		}
	}

	// AWS table name pattern
	pattern := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	if !pattern.MatchString(name) {
		return &SecurityError{
			Type:   "InvalidTableName",
			Field:  name,
			Detail: "table name can only contain letters, numbers, dots, dashes, and underscores",
		}
	}

	// Check for dangerous patterns
	nameLower := strings.ToLower(name)
	for _, dangerousPattern := range dangerousPatterns {
		if strings.Contains(nameLower, dangerousPattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  name,
				Detail: fmt.Sprintf("table name contains dangerous pattern: %s", dangerousPattern),
			}
		}
	}

	return nil
}

// ValidateIndexName validates a DynamoDB index name
func ValidateIndexName(name string) error {
	if name == "" {
		return nil // Empty index name is allowed (means no index)
	}

	if len(name) < 3 || len(name) > 255 {
		return &SecurityError{
			Type:   "InvalidIndexName",
			Field:  name,
			Detail: "index name must be 3-255 characters",
		}
	}

	// AWS index name pattern (similar to table name)
	pattern := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	if !pattern.MatchString(name) {
		return &SecurityError{
			Type:   "InvalidIndexName",
			Field:  name,
			Detail: "index name can only contain letters, numbers, dots, dashes, and underscores",
		}
	}

	return nil
}

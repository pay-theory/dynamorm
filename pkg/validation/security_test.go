package validation

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFieldNameValidation tests field name security validation
func TestFieldNameValidation(t *testing.T) {
	t.Run("ValidFieldNames", func(t *testing.T) {
		validNames := []string{
			"UserID",
			"user_id",
			"_internal",
			"Name",
			"nested.field",
			"deeply.nested.field.name",
		}

		for _, name := range validNames {
			t.Run(name, func(t *testing.T) {
				err := ValidateFieldName(name)
				assert.NoError(t, err, "Valid field name should not error: %s", name)
			})
		}
	})

	t.Run("RejectEmptyFieldName", func(t *testing.T) {
		err := ValidateFieldName("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "field name cannot be empty")
	})

	t.Run("RejectOversizedFieldName", func(t *testing.T) {
		longName := strings.Repeat("a", MaxFieldNameLength+1)
		err := ValidateFieldName(longName)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum length")
	})

	t.Run("RejectSQLInjectionPatterns", func(t *testing.T) {
		dangerousNames := []string{
			"field'; DROP TABLE users; --",
			"field\"; DELETE FROM table; --",
			"field/*comment*/",
			"field UNION SELECT",
			"field<script>alert('xss')</script>",
			"field'OR'1'='1",
		}

		for _, name := range dangerousNames {
			t.Run(name, func(t *testing.T) {
				err := ValidateFieldName(name)
				assert.Error(t, err, "Should reject dangerous field name: %s", name)
				assert.Contains(t, err.Error(), "dangerous pattern")
			})
		}
	})

	t.Run("RejectControlCharacters", func(t *testing.T) {
		controlChars := []string{
			"field\x00null",
			"field\ttab",
			"field\nnewline",
			"field\rcarriage",
		}

		for _, name := range controlChars {
			t.Run("Control_"+name, func(t *testing.T) {
				err := ValidateFieldName(name)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "control characters")
			})
		}
	})

	t.Run("RejectExcessiveNesting", func(t *testing.T) {
		parts := make([]string, MaxNestedDepth+1)
		for i := range parts {
			parts[i] = "field"
		}
		deepName := strings.Join(parts, ".")

		err := ValidateFieldName(deepName)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nested field depth exceeds maximum")
	})

	t.Run("ValidateNestedFieldParts", func(t *testing.T) {
		// Valid nested field
		err := ValidateFieldName("user.profile.name")
		assert.NoError(t, err)

		// Invalid nested field with SQL injection
		err = ValidateFieldName("user.'; DROP TABLE; --.name")
		assert.Error(t, err)
	})
}

// TestOperatorValidation tests operator security validation
func TestOperatorValidation(t *testing.T) {
	t.Run("ValidOperators", func(t *testing.T) {
		validOps := []string{
			"=", "!=", "<>", "<", "<=", ">", ">=",
			"BETWEEN", "IN", "BEGINS_WITH", "CONTAINS",
			"EXISTS", "NOT_EXISTS", "ATTRIBUTE_EXISTS", "ATTRIBUTE_NOT_EXISTS",
			"EQ", "NE", "LT", "LE", "GT", "GE",
		}

		for _, op := range validOps {
			t.Run(op, func(t *testing.T) {
				err := ValidateOperator(op)
				assert.NoError(t, err, "Valid operator should not error: %s", op)
			})
		}
	})

	t.Run("RejectEmptyOperator", func(t *testing.T) {
		err := ValidateOperator("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "operator cannot be empty")
	})

	t.Run("RejectOversizedOperator", func(t *testing.T) {
		longOp := strings.Repeat("a", MaxOperatorLength+1)
		err := ValidateOperator(longOp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum length")
	})

	t.Run("RejectInvalidOperators", func(t *testing.T) {
		invalidOps := []string{
			"DROP", "DELETE", "INSERT", "UPDATE", "CREATE",
			"EXEC", "EXECUTE", "UNION", "SELECT",
			"||", "&&", "XOR", "OR", "AND",
		}

		for _, op := range invalidOps {
			t.Run(op, func(t *testing.T) {
				err := ValidateOperator(op)
				assert.Error(t, err, "Should reject invalid operator: %s", op)
				assert.Contains(t, err.Error(), "is not allowed")
			})
		}
	})

	t.Run("RejectInjectionPatterns", func(t *testing.T) {
		injectionOps := []string{
			"'; DROP TABLE; --",
			"UNION SELECT",
			"/*comment*/",
			"<script>",
		}

		for _, op := range injectionOps {
			t.Run(op, func(t *testing.T) {
				err := ValidateOperator(op)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "dangerous pattern")
			})
		}
	})

	t.Run("CaseInsensitiveValidation", func(t *testing.T) {
		// Should accept operators in different cases
		cases := []string{"eq", "EQ", "Eq", "eQ"}
		for _, op := range cases {
			err := ValidateOperator(op)
			assert.NoError(t, err, "Should accept case variation: %s", op)
		}
	})
}

// TestValueValidation tests value security validation
func TestValueValidation(t *testing.T) {
	t.Run("ValidBasicValues", func(t *testing.T) {
		validValues := []any{
			nil,
			"string value",
			123,
			int64(456),
			3.14,
			true,
			false,
		}

		for i, value := range validValues {
			t.Run(fmt.Sprintf("Valid_%d", i), func(t *testing.T) {
				err := ValidateValue(value)
				assert.NoError(t, err, "Valid value should not error: %v", value)
			})
		}
	})

	t.Run("ValidSliceValues", func(t *testing.T) {
		validSlices := []any{
			[]string{"a", "b", "c"},
			[]int{1, 2, 3},
			[]any{"mixed", 123, true},
		}

		for i, value := range validSlices {
			t.Run(fmt.Sprintf("ValidSlice_%d", i), func(t *testing.T) {
				err := ValidateValue(value)
				assert.NoError(t, err, "Valid slice should not error: %v", value)
			})
		}
	})

	t.Run("ValidMapValues", func(t *testing.T) {
		validMaps := []any{
			map[string]any{"key": "value"},
			map[string]string{"a": "b"},
			map[string]int{"count": 5},
		}

		for i, value := range validMaps {
			t.Run(fmt.Sprintf("ValidMap_%d", i), func(t *testing.T) {
				err := ValidateValue(value)
				assert.NoError(t, err, "Valid map should not error: %v", value)
			})
		}
	})

	t.Run("RejectOversizedString", func(t *testing.T) {
		largeString := strings.Repeat("a", MaxValueStringLength+1)
		err := ValidateValue(largeString)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum length")
	})

	t.Run("RejectDangerousStringPatterns", func(t *testing.T) {
		dangerousStrings := []string{
			"'; DROP TABLE users; --",
			"<script>alert('xss')</script>",
			"UNION SELECT * FROM passwords",
			"/* malicious comment */",
		}

		for _, str := range dangerousStrings {
			t.Run(str, func(t *testing.T) {
				err := ValidateValue(str)
				assert.Error(t, err, "Should reject dangerous string: %s", str)
				assert.Contains(t, err.Error(), "dangerous pattern")
			})
		}
	})

	t.Run("RejectOversizedSlice", func(t *testing.T) {
		largeSlice := make([]string, 101)
		for i := range largeSlice {
			largeSlice[i] = "item"
		}

		err := ValidateValue(largeSlice)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum length")
	})

	t.Run("RejectOversizedMap", func(t *testing.T) {
		largeMap := make(map[string]string)
		for i := 0; i <= 100; i++ {
			largeMap[fmt.Sprintf("key%d", i)] = "value"
		}

		err := ValidateValue(largeMap)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum")
	})

	t.Run("RejectInvalidMapKeys", func(t *testing.T) {
		invalidMap := map[string]string{
			"'; DROP TABLE; --": "value",
		}

		err := ValidateValue(invalidMap)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid map key")
	})

	t.Run("RejectUnsupportedTypes", func(t *testing.T) {
		unsupportedValues := []any{
			make(chan int),
			func() {},
			complex(1, 2),
		}

		for i, value := range unsupportedValues {
			t.Run(fmt.Sprintf("Unsupported_%d", i), func(t *testing.T) {
				err := ValidateValue(value)
				assert.Error(t, err, "Should reject unsupported type: %T", value)
				assert.Contains(t, err.Error(), "unsupported value type")
			})
		}
	})
}

// TestExpressionValidation tests complete expression validation
func TestExpressionValidation(t *testing.T) {
	t.Run("ValidExpressions", func(t *testing.T) {
		validExpressions := []string{
			"#pk = :val",
			"attribute_exists(#field)",
			"#name BETWEEN :start AND :end",
			"contains(#tags, :tag)",
		}

		for _, expr := range validExpressions {
			t.Run(expr, func(t *testing.T) {
				err := ValidateExpression(expr)
				assert.NoError(t, err, "Valid expression should not error: %s", expr)
			})
		}
	})

	t.Run("RejectOversizedExpression", func(t *testing.T) {
		largeExpr := strings.Repeat("a", MaxExpressionLength+1)
		err := ValidateExpression(largeExpr)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum length")
	})

	t.Run("RejectDangerousExpressions", func(t *testing.T) {
		dangerousExpressions := []string{
			"'; DROP TABLE users; --",
			"UNION SELECT * FROM passwords",
			"<script>alert('xss')</script>",
			"/* comment */ DELETE FROM table",
		}

		for _, expr := range dangerousExpressions {
			t.Run(expr, func(t *testing.T) {
				err := ValidateExpression(expr)
				assert.Error(t, err, "Should reject dangerous expression: %s", expr)
				assert.Contains(t, err.Error(), "dangerous pattern")
			})
		}
	})
}

// TestTableNameValidation tests table name validation
func TestTableNameValidation(t *testing.T) {
	t.Run("ValidTableNames", func(t *testing.T) {
		validNames := []string{
			"Users",
			"user-table",
			"user_table",
			"table123",
			"My.Table",
		}

		for _, name := range validNames {
			t.Run(name, func(t *testing.T) {
				err := ValidateTableName(name)
				assert.NoError(t, err, "Valid table name should not error: %s", name)
			})
		}
	})

	t.Run("RejectInvalidLength", func(t *testing.T) {
		// Too short
		err := ValidateTableName("ab")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be 3-255 characters")

		// Too long
		longName := strings.Repeat("a", 256)
		err = ValidateTableName(longName)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be 3-255 characters")
	})

	t.Run("RejectInvalidCharacters", func(t *testing.T) {
		invalidNames := []string{
			"table@name",
			"table#name",
			"table name", // space
			"table/name",
			"table\\name",
		}

		for _, name := range invalidNames {
			t.Run(name, func(t *testing.T) {
				err := ValidateTableName(name)
				assert.Error(t, err, "Should reject invalid table name: %s", name)
				assert.Contains(t, err.Error(), "can only contain")
			})
		}
	})

	t.Run("RejectDangerousPatterns", func(t *testing.T) {
		dangerousNames := []string{
			"users'; DROP TABLE",
			"table--comment",
			"table/*comment*/",
		}

		for _, name := range dangerousNames {
			t.Run(name, func(t *testing.T) {
				err := ValidateTableName(name)
				assert.Error(t, err, "Should reject dangerous table name: %s", name)
				assert.Contains(t, err.Error(), "dangerous pattern")
			})
		}
	})
}

// TestIndexNameValidation tests index name validation
func TestIndexNameValidation(t *testing.T) {
	t.Run("ValidIndexNames", func(t *testing.T) {
		validNames := []string{
			"", // Empty is allowed
			"gsi-users",
			"lsi_status",
			"index123",
			"My.Index",
		}

		for _, name := range validNames {
			t.Run(name, func(t *testing.T) {
				err := ValidateIndexName(name)
				assert.NoError(t, err, "Valid index name should not error: %s", name)
			})
		}
	})

	t.Run("RejectInvalidLength", func(t *testing.T) {
		// Too short (but not empty)
		err := ValidateIndexName("ab")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be 3-255 characters")

		// Too long
		longName := strings.Repeat("a", 256)
		err = ValidateIndexName(longName)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be 3-255 characters")
	})

	t.Run("RejectInvalidCharacters", func(t *testing.T) {
		invalidNames := []string{
			"index@name",
			"index#name",
			"index name", // space
			"index/name",
		}

		for _, name := range invalidNames {
			t.Run(name, func(t *testing.T) {
				err := ValidateIndexName(name)
				assert.Error(t, err, "Should reject invalid index name: %s", name)
				assert.Contains(t, err.Error(), "can only contain")
			})
		}
	})
}

// TestSecurityErrorTypes tests security error handling
func TestSecurityErrorTypes(t *testing.T) {
	t.Run("SecurityErrorFormat", func(t *testing.T) {
		err := &SecurityError{
			Type:   "TestType",
			Field:  "testField",
			Detail: "test detail",
		}

		expected := "security validation failed [TestType]: testField - test detail"
		assert.Equal(t, expected, err.Error())
	})

	t.Run("SecurityErrorTypes", func(t *testing.T) {
		// Test different error types are properly categorized
		fieldErr := ValidateFieldName("'; DROP TABLE")
		assert.Contains(t, fieldErr.Error(), "[InjectionAttempt]")

		opErr := ValidateOperator("INVALID_OP")
		assert.Contains(t, opErr.Error(), "[InvalidOperator]")

		valueErr := ValidateValue(strings.Repeat("a", MaxValueStringLength+1))
		assert.Contains(t, valueErr.Error(), "[InvalidValue]")
	})
}

// Benchmark security validation performance
func BenchmarkSecurityValidation(b *testing.B) {
	b.Run("FieldNameValidation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ValidateFieldName("user.profile.name")
		}
	})

	b.Run("OperatorValidation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ValidateOperator("=")
		}
	})

	b.Run("ValueValidation", func(b *testing.B) {
		value := map[string]any{
			"key1": "value1",
			"key2": 123,
			"key3": true,
		}
		for i := 0; i < b.N; i++ {
			_ = ValidateValue(value)
		}
	})
}

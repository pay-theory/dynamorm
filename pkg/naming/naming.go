package naming

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

var camelCasePattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)

// ResolveAttrName determines the DynamoDB attribute name for a field.
// It returns the attribute name and a bool indicating whether the field should be skipped.
func ResolveAttrName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("dynamorm")
	if tag == "-" {
		return "", true
	}

	if attr := attrFromTag(tag); attr != "" {
		return attr, false
	}

	return DefaultAttrName(field.Name), false
}

// DefaultAttrName converts a Go struct field name to the preferred camelCase DynamoDB attribute name.
func DefaultAttrName(name string) string {
	if name == "" {
		return ""
	}

	if name == "PK" || name == "SK" {
		return name
	}

	runes := []rune(name)
	if len(runes) == 1 {
		return strings.ToLower(name)
	}

	boundary := 1
	for boundary < len(runes) {
		if !unicode.IsUpper(runes[boundary]) {
			break
		}

		if boundary+1 < len(runes) && !unicode.IsUpper(runes[boundary+1]) {
			break
		}

		boundary++
	}

	prefix := strings.ToLower(string(runes[:boundary]))
	return prefix + string(runes[boundary:])
}

// ValidateAttrName enforces camelCase (with PK/SK exceptions) for DynamoDB attribute names.
func ValidateAttrName(name string) error {
	if name == "" {
		return fmt.Errorf("attribute name cannot be empty")
	}

	if name == "PK" || name == "SK" {
		return nil
	}

	if !camelCasePattern.MatchString(name) {
		return fmt.Errorf("attribute name must be camelCase (got %q)", name)
	}
	return nil
}

func attrFromTag(tag string) string {
	if tag == "" {
		return ""
	}

	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "attr:") {
			return strings.TrimPrefix(part, "attr:")
		}
	}
	return ""
}

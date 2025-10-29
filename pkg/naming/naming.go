package naming

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

// Convention represents the naming convention for DynamoDB attribute names.
type Convention int

const (
	// CamelCase convention: "firstName", "createdAt", with special handling for "PK" and "SK"
	CamelCase Convention = 0
	// SnakeCase convention: "first_name", "created_at"
	SnakeCase Convention = 1
)

var camelCasePattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
var snakeCasePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// ResolveAttrName determines the DynamoDB attribute name for a field using CamelCase convention.
// It returns the attribute name and a bool indicating whether the field should be skipped.
func ResolveAttrName(field reflect.StructField) (string, bool) {
	return ResolveAttrNameWithConvention(field, CamelCase)
}

// ResolveAttrNameWithConvention determines the DynamoDB attribute name for a field using the specified convention.
// It returns the attribute name and a bool indicating whether the field should be skipped.
func ResolveAttrNameWithConvention(field reflect.StructField, convention Convention) (string, bool) {
	tag := field.Tag.Get("dynamorm")
	if tag == "-" {
		return "", true
	}

	if attr := attrFromTag(tag); attr != "" {
		return attr, false
	}

	return ConvertAttrName(field.Name, convention), false
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

// ToSnakeCase converts a Go struct field name to snake_case DynamoDB attribute name.
// It uses smart acronym handling: "URLValue" → "url_value", "ID" → "id", "UserID" → "user_id".
func ToSnakeCase(name string) string {
	if name == "" {
		return ""
	}

	runes := []rune(name)
	if len(runes) == 1 {
		return strings.ToLower(name)
	}

	var result []rune
	var currentWord []rune

	for i := 0; i < len(runes); i++ {
		ch := runes[i]

		// If we hit an uppercase letter, we might be starting a new word
		if unicode.IsUpper(ch) {
			// Look ahead to determine if this is part of an acronym sequence
			isAcronym := false
			if i+1 < len(runes) && unicode.IsUpper(runes[i+1]) {
				// Next char is also uppercase, so this is part of an acronym
				isAcronym = true
			}

			// Check if previous character was a digit
			previousWasDigit := i > 0 && unicode.IsDigit(runes[i-1])

			// If we have a current word, flush it before starting new one
			// But don't add underscore if previous char was a digit
			if len(currentWord) > 0 {
				if len(result) > 0 && !previousWasDigit {
					result = append(result, '_')
				}
				result = append(result, currentWord...)
				currentWord = nil
			}

			// Start collecting the new word (or acronym)
			if isAcronym {
				// Collect the acronym sequence
				acronym := []rune{ch}
				j := i + 1
				for j < len(runes) && unicode.IsUpper(runes[j]) {
					// Check if next char after this is lowercase (end of acronym)
					if j+1 < len(runes) && !unicode.IsUpper(runes[j+1]) {
						// This uppercase char belongs to the next word
						// e.g., in "URLValue", 'V' starts "Value"
						break
					}
					acronym = append(acronym, runes[j])
					j++
				}

				// Add the acronym as a word
				if len(result) > 0 {
					result = append(result, '_')
				}
				for _, r := range acronym {
					result = append(result, unicode.ToLower(r))
				}
				i = j - 1 // -1 because loop will increment
			} else {
				// Single uppercase letter starting a word
				currentWord = []rune{unicode.ToLower(ch)}
			}
		} else {
			// Lowercase or digit, add to current word
			currentWord = append(currentWord, ch)
		}
	}

	// Flush any remaining word
	if len(currentWord) > 0 {
		// Check if last character of result is a digit
		shouldAddUnderscore := len(result) > 0 && !unicode.IsDigit(result[len(result)-1])
		if shouldAddUnderscore {
			result = append(result, '_')
		}
		result = append(result, currentWord...)
	}

	return string(result)
}

// ConvertAttrName converts a field name to the appropriate naming convention.
func ConvertAttrName(name string, convention Convention) string {
	switch convention {
	case SnakeCase:
		return ToSnakeCase(name)
	case CamelCase:
		fallthrough
	default:
		return DefaultAttrName(name)
	}
}

// ValidateAttrName enforces the naming convention for DynamoDB attribute names.
// For CamelCase: allows "PK" and "SK" as exceptions, otherwise enforces camelCase pattern.
// For SnakeCase: enforces snake_case pattern (no special exceptions).
func ValidateAttrName(name string, convention Convention) error {
	if name == "" {
		return fmt.Errorf("attribute name cannot be empty")
	}

	switch convention {
	case SnakeCase:
		if !snakeCasePattern.MatchString(name) {
			return fmt.Errorf("attribute name must be snake_case (got %q)", name)
		}
		return nil
	case CamelCase:
		fallthrough
	default:
		// CamelCase validation with PK/SK exceptions
		if name == "PK" || name == "SK" {
			return nil
		}
		if !camelCasePattern.MatchString(name) {
			return fmt.Errorf("attribute name must be camelCase (got %q)", name)
		}
		return nil
	}
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

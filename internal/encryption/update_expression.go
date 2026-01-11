package encryption

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/pay-theory/dynamorm/pkg/model"
)

func encryptedAttributeNameSet(metadata *model.Metadata) map[string]struct{} {
	if metadata == nil {
		return nil
	}

	out := make(map[string]struct{})
	for _, fieldMeta := range metadata.Fields {
		if fieldMeta == nil {
			continue
		}
		if !fieldMeta.IsEncrypted {
			if _, ok := fieldMeta.Tags["encrypted"]; !ok {
				continue
			}
		}
		out[fieldMeta.DBName] = struct{}{}
	}
	return out
}

// EncryptUpdateExpressionValues mutates exprAttrValues in-place by encrypting values assigned to encrypted fields.
// It currently supports direct SET assignments and if_not_exists() defaults for encrypted fields.
func EncryptUpdateExpressionValues(
	ctx context.Context,
	svc *Service,
	metadata *model.Metadata,
	updateExpression string,
	exprAttrNames map[string]string,
	exprAttrValues map[string]types.AttributeValue,
) error {
	if updateExpression == "" || len(exprAttrValues) == 0 {
		return nil
	}

	encrypted := encryptedAttributeNameSet(metadata)
	if len(encrypted) == 0 {
		return nil
	}

	sections := splitUpdateExpressionSections(updateExpression)

	if setExpr, ok := sections["SET"]; ok && setExpr != "" {
		assignments := splitTopLevelCommaSeparated(setExpr)
		for _, assignment := range assignments {
			lhs, rhs, ok := splitAssignment(assignment)
			if !ok {
				continue
			}

			baseName, hasIndexOrNested := baseNamePlaceholder(lhs)
			attrName := exprAttrNames[baseName]
			if attrName == "" {
				continue
			}

			if _, isEncrypted := encrypted[attrName]; !isEncrypted {
				continue
			}

			if hasIndexOrNested {
				return fmt.Errorf("encrypted field %s does not support nested or indexed updates", attrName)
			}

			if strings.HasPrefix(rhs, "if_not_exists(") {
				valueRef, ok := ifNotExistsDefaultValueRef(rhs)
				if !ok {
					return fmt.Errorf("unsupported if_not_exists expression for encrypted field %s", attrName)
				}
				if err := encryptValueRef(ctx, svc, attrName, valueRef, exprAttrValues); err != nil {
					return err
				}
				continue
			}

			if isValuePlaceholder(rhs) {
				if err := encryptValueRef(ctx, svc, attrName, rhs, exprAttrValues); err != nil {
					return err
				}
				continue
			}

			return fmt.Errorf("unsupported update expression for encrypted field %s", attrName)
		}
	}

	for _, action := range []string{"ADD", "DELETE"} {
		segment, ok := sections[action]
		if !ok || segment == "" {
			continue
		}

		parts := splitTopLevelCommaSeparated(segment)
		for _, part := range parts {
			fields := strings.Fields(part)
			if len(fields) < 1 {
				continue
			}
			base := fields[0]
			attrName := exprAttrNames[base]
			if attrName == "" {
				continue
			}
			if _, isEncrypted := encrypted[attrName]; isEncrypted {
				return fmt.Errorf("encrypted field %s does not support %s updates", attrName, action)
			}
		}
	}

	return nil
}

func encryptValueRef(ctx context.Context, svc *Service, attrName, valueRef string, values map[string]types.AttributeValue) error {
	plaintext, ok := values[valueRef]
	if !ok {
		return fmt.Errorf("missing expression attribute value %s for encrypted field %s", valueRef, attrName)
	}
	encrypted, err := svc.EncryptAttributeValue(ctx, attrName, plaintext)
	if err != nil {
		return err
	}
	values[valueRef] = encrypted
	return nil
}

func isValuePlaceholder(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, ":") && len(s) > 1
}

func splitUpdateExpressionSections(expr string) map[string]string {
	sections := make(map[string]string)
	tokens := strings.Fields(expr)

	action := ""
	var buf []string
	flush := func() {
		if action == "" {
			return
		}
		sections[action] = strings.Join(buf, " ")
		buf = nil
	}

	for _, tok := range tokens {
		switch tok {
		case "SET", "REMOVE", "ADD", "DELETE":
			flush()
			action = tok
		default:
			if action != "" {
				buf = append(buf, tok)
			}
		}
	}
	flush()

	return sections
}

func splitTopLevelCommaSeparated(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	var parts []string
	start := 0
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				part := strings.TrimSpace(s[start:i])
				if part != "" {
					parts = append(parts, part)
				}
				start = i + 1
			}
		}
	}

	last := strings.TrimSpace(s[start:])
	if last != "" {
		parts = append(parts, last)
	}
	return parts
}

func splitAssignment(expr string) (string, string, bool) {
	idx := strings.Index(expr, "=")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(expr[:idx]), strings.TrimSpace(expr[idx+1:]), true
}

func baseNamePlaceholder(lhs string) (string, bool) {
	lhs = strings.TrimSpace(lhs)
	hasIndexOrNested := strings.Contains(lhs, ".") || strings.Contains(lhs, "[")

	stop := len(lhs)
	for _, ch := range []byte{'.', '['} {
		if idx := strings.IndexByte(lhs, ch); idx >= 0 && idx < stop {
			stop = idx
		}
	}

	return strings.TrimSpace(lhs[:stop]), hasIndexOrNested
}

func ifNotExistsDefaultValueRef(rhs string) (string, bool) {
	rhs = strings.TrimSpace(rhs)
	if !strings.HasPrefix(rhs, "if_not_exists(") || !strings.HasSuffix(rhs, ")") {
		return "", false
	}

	inner := strings.TrimSuffix(strings.TrimPrefix(rhs, "if_not_exists("), ")")
	args := splitTopLevelCommaSeparated(inner)
	if len(args) != 2 {
		return "", false
	}

	val := strings.TrimSpace(args[1])
	if !isValuePlaceholder(val) {
		return "", false
	}
	return val, true
}

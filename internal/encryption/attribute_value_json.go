package encryption

import (
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func marshalAVJSON(av types.AttributeValue) (avJSON, error) {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		s := v.Value
		return avJSON{Type: "S", S: &s}, nil
	case *types.AttributeValueMemberN:
		n := v.Value
		return avJSON{Type: "N", N: &n}, nil
	case *types.AttributeValueMemberB:
		encoded := base64.StdEncoding.EncodeToString(v.Value)
		return avJSON{Type: "B", B: &encoded}, nil
	case *types.AttributeValueMemberBOOL:
		val := v.Value
		return avJSON{Type: "BOOL", BOOL: &val}, nil
	case *types.AttributeValueMemberNULL:
		return avJSON{Type: "NULL", NULL: true}, nil
	case *types.AttributeValueMemberL:
		list := make([]avJSON, len(v.Value))
		for i := range v.Value {
			elem, err := marshalAVJSON(v.Value[i])
			if err != nil {
				return avJSON{}, err
			}
			list[i] = elem
		}
		return avJSON{Type: "L", L: list}, nil
	case *types.AttributeValueMemberM:
		m := make(map[string]avJSON, len(v.Value))
		for key, val := range v.Value {
			encoded, err := marshalAVJSON(val)
			if err != nil {
				return avJSON{}, err
			}
			m[key] = encoded
		}
		return avJSON{Type: "M", M: m}, nil
	case *types.AttributeValueMemberSS:
		return avJSON{Type: "SS", SS: append([]string(nil), v.Value...)}, nil
	case *types.AttributeValueMemberNS:
		return avJSON{Type: "NS", NS: append([]string(nil), v.Value...)}, nil
	case *types.AttributeValueMemberBS:
		encoded := make([]string, len(v.Value))
		for i := range v.Value {
			encoded[i] = base64.StdEncoding.EncodeToString(v.Value[i])
		}
		return avJSON{Type: "BS", BS: encoded}, nil
	default:
		return avJSON{}, fmt.Errorf("unsupported attribute value type: %T", av)
	}
}

func unmarshalAVJSON(enc avJSON) (types.AttributeValue, error) {
	switch enc.Type {
	case "S":
		if enc.S == nil {
			return &types.AttributeValueMemberS{Value: ""}, nil
		}
		return &types.AttributeValueMemberS{Value: *enc.S}, nil
	case "N":
		if enc.N == nil {
			return &types.AttributeValueMemberN{Value: "0"}, nil
		}
		return &types.AttributeValueMemberN{Value: *enc.N}, nil
	case "B":
		if enc.B == nil {
			return &types.AttributeValueMemberB{Value: nil}, nil
		}
		decoded, err := base64.StdEncoding.DecodeString(*enc.B)
		if err != nil {
			return nil, fmt.Errorf("failed to decode binary: %w", err)
		}
		return &types.AttributeValueMemberB{Value: decoded}, nil
	case "BOOL":
		val := false
		if enc.BOOL != nil {
			val = *enc.BOOL
		}
		return &types.AttributeValueMemberBOOL{Value: val}, nil
	case "NULL":
		return &types.AttributeValueMemberNULL{Value: true}, nil
	case "L":
		list := make([]types.AttributeValue, len(enc.L))
		for i := range enc.L {
			elem, err := unmarshalAVJSON(enc.L[i])
			if err != nil {
				return nil, err
			}
			list[i] = elem
		}
		return &types.AttributeValueMemberL{Value: list}, nil
	case "M":
		m := make(map[string]types.AttributeValue, len(enc.M))
		for key, val := range enc.M {
			decoded, err := unmarshalAVJSON(val)
			if err != nil {
				return nil, err
			}
			m[key] = decoded
		}
		return &types.AttributeValueMemberM{Value: m}, nil
	case "SS":
		return &types.AttributeValueMemberSS{Value: append([]string(nil), enc.SS...)}, nil
	case "NS":
		return &types.AttributeValueMemberNS{Value: append([]string(nil), enc.NS...)}, nil
	case "BS":
		decoded := make([][]byte, len(enc.BS))
		for i := range enc.BS {
			b, err := base64.StdEncoding.DecodeString(enc.BS[i])
			if err != nil {
				return nil, fmt.Errorf("failed to decode binary set: %w", err)
			}
			decoded[i] = b
		}
		return &types.AttributeValueMemberBS{Value: decoded}, nil
	default:
		return nil, fmt.Errorf("unsupported encoded attribute value type: %s", enc.Type)
	}
}

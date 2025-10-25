package naming

import (
	"reflect"
	"testing"
)

type sample struct {
	Simple         string
	URLValue       string
	ID             string
	CustomAttr     string `dynamorm:"attr:customName"`
	Skip           string `dynamorm:"-"`
	PK             string `dynamorm:"pk"`
	SK             string `dynamorm:"sk"`
	ExplicitPK     string `dynamorm:"pk,attr:PK"`
	ExplicitCustom string `dynamorm:"attr:camelCase"`
}

func TestDefaultAttrName(t *testing.T) {
	tests := map[string]string{
		"Name":      "name",
		"CreatedAt": "createdAt",
		"URLValue":  "urlValue",
		"ID":        "id",
		"UUID":      "uuid",
		"HTTPCode":  "httpCode",
		"PK":        "PK",
		"SK":        "SK",
	}

	for input, expected := range tests {
		if got := DefaultAttrName(input); got != expected {
			t.Errorf("DefaultAttrName(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestValidateAttrName(t *testing.T) {
	valid := []string{"name", "createdAt", "value1", "PK", "SK"}
	for _, v := range valid {
		if err := ValidateAttrName(v); err != nil {
			t.Errorf("ValidateAttrName(%q) unexpected error: %v", v, err)
		}
	}

	invalid := []string{"", "snake_case", "CamelCase", "hyphen-name"}
	for _, v := range invalid {
		if err := ValidateAttrName(v); err == nil {
			t.Errorf("ValidateAttrName(%q) expected error", v)
		}
	}
}

func TestResolveAttrName(t *testing.T) {
	typ := reflect.TypeOf(sample{})

	field := typ.Field(0)
	name, skip := ResolveAttrName(field)
	if skip || name != "simple" {
		t.Fatalf("expected simple, got %q skip=%v", name, skip)
	}

	field = typ.Field(1)
	name, skip = ResolveAttrName(field)
	if skip || name != "urlValue" {
		t.Fatalf("expected urlValue, got %q", name)
	}

	field = typ.Field(3)
	name, skip = ResolveAttrName(field)
	if skip || name != "customName" {
		t.Fatalf("expected customName, got %q", name)
	}

	field = typ.Field(4)
	if _, skip = ResolveAttrName(field); !skip {
		t.Fatalf("expected skip for field with dynamorm:\"-\"")
	}

	field = typ.Field(6)
	name, skip = ResolveAttrName(field)
	if skip || name != "SK" {
		t.Fatalf("expected SK, got %q", name)
	}

	field = typ.Field(7)
	name, skip = ResolveAttrName(field)
	if skip || name != "PK" {
		t.Fatalf("expected PK, got %q", name)
	}
}

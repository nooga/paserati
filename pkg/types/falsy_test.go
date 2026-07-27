package types

import (
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

func TestExtractFalsyTypes(t *testing.T) {
	cases := []struct {
		name string
		in   Type
		want string
	}{
		// Compared against emptyStringType's own rendering rather than a literal
		// `""`: Type.String() prints a string literal type without its quotes,
		// a known divergence from TypeScript that is not this file's business.
		{"string keeps only its empty value", String, emptyStringType.String()},
		{"number keeps only zero", Number, "0"},
		{"boolean keeps only false", Boolean, "false"},
		{"null is already falsy", Null, "null"},
		{"objects are always truthy", NewUnionType(NonPrimitive), "never"},
		{"union splits", NewUnionType(String, Null), emptyStringType.String() + " | null"},
		{"truthy literal drops out", &LiteralType{Value: vm.String("hi")}, "never"},
	}
	for _, c := range cases {
		if got := ExtractFalsyTypes(c.in).String(); got != c.want {
			t.Errorf("%s: ExtractFalsyTypes(%s) = %s, want %s", c.name, c.in.String(), got, c.want)
		}
	}
}

func TestRemoveFalsyTypes(t *testing.T) {
	cases := []struct {
		name string
		in   Type
		want string
	}{
		{"boolean keeps only true", Boolean, "true"},
		{"null cannot be truthy", Null, "never"},
		{"undefined cannot be truthy", Undefined, "never"},
		// TypeScript has no "non-empty string" type, so these stay whole.
		{"string stays whole", String, "string"},
		{"number stays whole", Number, "number"},
		{"union drops the nullish part", NewUnionType(String, Undefined), "string"},
	}
	for _, c := range cases {
		if got := RemoveFalsyTypes(c.in).String(); got != c.want {
			t.Errorf("%s: RemoveFalsyTypes(%s) = %s, want %s", c.name, c.in.String(), got, c.want)
		}
	}
}

func TestRemoveNullishTypes(t *testing.T) {
	// Unlike ||, ?? keeps falsy-but-defined values — that is its whole point.
	got := RemoveNullishTypes(NewUnionType(Number, Null, Undefined)).String()
	if got != "number" {
		t.Errorf("RemoveNullishTypes(number | null | undefined) = %s, want number", got)
	}
	if got := RemoveNullishTypes(zeroType).String(); got != "0" {
		t.Errorf("RemoveNullishTypes(0) = %s, want 0 — ?? must keep falsy values", got)
	}
}

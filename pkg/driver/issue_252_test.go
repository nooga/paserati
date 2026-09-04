package driver

import (
	"reflect"
	"testing"

	"github.com/nooga/paserati/pkg/vm"
)

// TestNativeFunctionAcceptsIntegerRepresentedNumber covers paserati#252:
// vmValueToReflectValue converted a JS number argument into a Go numeric
// parameter (float64/float32/int/int64) via the raw Value.AsFloat()
// accessor, which panics ("value is not a float") whenever the argument
// happens to be TypeIntegerNumber-represented internally rather than
// TypeFloatNumber - a distinction invisible at the JS level (typeof is
// "number" either way; e.g. "hello".length is IntegerValue-represented).
// The fix swaps in the safe, representation-agnostic Value.ToFloat().
func TestNativeFunctionAcceptsIntegerRepresentedNumber(t *testing.T) {
	p := NewPaserati()
	p.DeclareModule("issue252mod", func(m *ModuleBuilder) {
		m.Function("takesFloat64", func(n float64) float64 { return n })
		m.Function("takesFloat32", func(n float32) float32 { return n })
		m.Function("takesInt", func(n int) int { return n })
		m.Function("takesInt64", func(n int64) int64 { return n })
	})

	res, errs := p.RunString(`
		import { takesFloat64, takesFloat32, takesInt, takesInt64 } from "issue252mod";
		// "hello".length is internally an IntegerValue-represented number,
		// indistinguishable from a "float" one at the JS level.
		const n = "hello".length;
		JSON.stringify([
			takesFloat64(n),
			takesFloat32(n),
			takesInt(n),
			takesInt64(n),
		]);
	`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	got := res.ToString()
	want := `[5,5,5,5]`
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

// TestVMValueToReflectValueHandlesIntegerNumber constructs a
// TypeIntegerNumber vm.Value directly (via vm.IntegerValue) and passes it
// through the reflection conversion helper, guarding against a regression
// back to the panicking Value.AsFloat() accessor.
func TestVMValueToReflectValueHandlesIntegerNumber(t *testing.T) {
	iv := vm.IntegerValue(42)
	if !iv.IsNumber() {
		t.Fatalf("expected IntegerValue to report IsNumber() == true")
	}

	cases := []struct {
		name string
		typ  reflect.Type
		want interface{}
	}{
		{"float64", reflect.TypeOf(float64(0)), float64(42)},
		{"float32", reflect.TypeOf(float32(0)), float32(42)},
		{"int", reflect.TypeOf(int(0)), int(42)},
		{"int64", reflect.TypeOf(int64(0)), int64(42)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := vmValueToReflectValue(iv, c.typ).Interface()
			if got != c.want {
				t.Fatalf("expected %v (%T), got %v (%T)", c.want, c.want, got, got)
			}
		})
	}
}

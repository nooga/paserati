package vm

import (
	"math"
	"math/big"
	"testing"
)

// toIntegerCases pins ToInteger (ECMAScript ToInt32) behavior across input kinds.
// Values are hand-derived from the spec (32-bit wrap; NaN/Inf -> 0; string parse;
// bool 1/0). The test must pass BOTH before and after the fast-path reorder — it
// is the equivalence guard for that change.
var toIntegerCases = []struct {
	name string
	v    Value
	want int32
}{
	{"int-pos", IntegerValue(5), 5},
	{"int-neg", IntegerValue(-3), -3},
	{"float-trunc-pos", NumberValue(3.9), 3},
	{"float-trunc-neg", NumberValue(-3.9), -3},
	{"float-nan", NumberValue(math.NaN()), 0},
	{"float-posinf", NumberValue(math.Inf(1)), 0},
	{"float-neginf", NumberValue(math.Inf(-1)), 0},
	{"float-wrap-2^32+5", NumberValue(4294967301), 5},
	{"float-wrap-2^31", NumberValue(2147483648), -2147483648},
	{"float-negzero", NumberValue(math.Copysign(0, -1)), 0},
	{"bool-true", BooleanValue(true), 1},
	{"bool-false", BooleanValue(false), 0},
	{"str-int", NewString("42"), 42},
	{"str-pad", NewString("  10 "), 10},
	{"str-float", NewString("3.7"), 3},
	{"str-empty", NewString(""), 0},
	{"str-garbage", NewString("abc"), 0},
	{"undefined", Undefined, 0},
	{"null", Null, 0},
	{"bigint-small", NewBigInt(big.NewInt(10)), 10},
}

func TestToIntegerEquivalence(t *testing.T) {
	for _, tc := range toIntegerCases {
		if got := tc.v.ToInteger(); got != tc.want {
			t.Errorf("ToInteger(%s) = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// BenchmarkToInteger models the hot path: mostly integer operands (bitwise ops),
// a few floats. Measures the fast-path reorder — before the change, each call
// runs the IsObject/object guard before reaching the number case.
func BenchmarkToInteger(b *testing.B) {
	vs := make([]Value, 0, 32)
	for i := 0; i < 28; i++ {
		vs = append(vs, IntegerValue(int32(i*7-13)))
	}
	vs = append(vs, NumberValue(3.5), NumberValue(-9.9), NumberValue(4294967301), NumberValue(2147483648))
	b.ResetTimer()
	var sink int32
	for i := 0; i < b.N; i++ {
		for j := range vs {
			sink += vs[j].ToInteger()
		}
	}
	_ = sink
}

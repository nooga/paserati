package builtins

import (
	"math"
	"math/rand"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// toUint32 implements ECMAScript's ToUint32 abstract operation
// Per spec: NaN, +0, -0, +Infinity, -Infinity all become 0
func toUint32(n float64) uint32 {
	if math.IsNaN(n) || math.IsInf(n, 0) || n == 0 {
		return 0
	}
	// Get the integer part with sign
	intVal := math.Trunc(n)
	// Modulo 2^32
	mod := math.Mod(intVal, 4294967296) // 2^32
	if mod < 0 {
		mod += 4294967296
	}
	return uint32(mod)
}

// toInt32 implements ECMAScript's ToInt32 abstract operation
func toInt32(n float64) int32 {
	u := toUint32(n)
	if u >= 2147483648 { // 2^31
		return int32(int64(u) - 4294967296)
	}
	return int32(u)
}

// float64ToFloat16ToFloat64 converts a float64 to half-precision (float16) and back
// This implements the rounding behavior specified for Math.f16round
func float64ToFloat16ToFloat64(x float64) float64 {
	// Get the bits of the float64
	bits := math.Float64bits(x)
	sign := bits >> 63
	exp := int((bits >> 52) & 0x7FF)
	frac := bits & 0xFFFFFFFFFFFFF

	// float16 parameters
	const f16ExpBias = 15
	const f64ExpBias = 1023
	const f16MaxExp = 30   // Biased max exponent (31 is for inf/nan)
	const f16MinExp = 1    // Biased min normal exponent
	const f16FracBits = 10 // Mantissa bits in float16

	// Compute unbiased exponent
	unbiasedExp := exp - f64ExpBias

	var f16Bits uint16

	if exp == 0x7FF {
		// Infinity or NaN - already handled before calling this function
		if frac == 0 {
			// Infinity
			f16Bits = uint16(sign<<15) | 0x7C00
		} else {
			// NaN - preserve sign, set NaN pattern
			f16Bits = uint16(sign<<15) | 0x7E00
		}
	} else if exp == 0 {
		// Denormal or zero in float64 - becomes zero in float16
		f16Bits = uint16(sign << 15)
	} else if unbiasedExp > 15 {
		// Overflow to infinity
		f16Bits = uint16(sign<<15) | 0x7C00
	} else if unbiasedExp < -24 {
		// Underflow to zero
		f16Bits = uint16(sign << 15)
	} else if unbiasedExp < -14 {
		// Denormal in float16
		// Shift the mantissa right to create a denormal
		shift := uint(-14 - unbiasedExp)
		// Add implicit 1 bit
		frac16 := (frac >> 42) | 0x400 // 10 bits + implicit 1
		frac16 = frac16 >> shift
		// Round to nearest even
		roundBit := (frac >> (42 + shift - 1)) & 1
		if roundBit == 1 {
			frac16++
		}
		f16Bits = uint16(sign<<15) | uint16(frac16&0x3FF)
	} else {
		// Normal number
		f16Exp := uint16(unbiasedExp + f16ExpBias)
		// Take top 10 bits of mantissa, round to nearest even
		frac16 := frac >> 42 // Top 10 bits
		// Check for rounding
		roundBits := (frac >> 41) & 1 // 11th bit
		if roundBits == 1 {
			// Round up, but check for tie (round to even)
			lowerBits := frac & 0x1FFFFFFFFFF // bits 0-40
			if lowerBits != 0 || (frac16&1) == 1 {
				frac16++
				if frac16 > 0x3FF {
					// Overflow to next exponent
					frac16 = 0
					f16Exp++
					if f16Exp > 30 {
						// Overflow to infinity
						f16Bits = uint16(sign<<15) | 0x7C00
						goto convert
					}
				}
			}
		}
		f16Bits = uint16(sign<<15) | (f16Exp << 10) | uint16(frac16&0x3FF)
	}

convert:
	// Convert float16 bits back to float64
	f16Sign := (f16Bits >> 15) & 1
	f16Exp := (f16Bits >> 10) & 0x1F
	f16Frac := f16Bits & 0x3FF

	var result float64
	if f16Exp == 0x1F {
		// Infinity or NaN
		if f16Frac == 0 {
			result = math.Inf(1)
		} else {
			result = math.NaN()
		}
	} else if f16Exp == 0 {
		if f16Frac == 0 {
			result = 0
		} else {
			// Denormal
			result = float64(f16Frac) / 1024.0 * math.Pow(2, -14)
		}
	} else {
		// Normal
		result = (1.0 + float64(f16Frac)/1024.0) * math.Pow(2, float64(f16Exp)-15)
	}

	if f16Sign == 1 {
		result = -result
	}
	return result
}

type MathInitializer struct{}

func (m *MathInitializer) Name() string {
	return "Math"
}

func (m *MathInitializer) Priority() int {
	return PriorityMath // 100 - After core types
}

func (m *MathInitializer) InitTypes(ctx *TypeContext) error {
	// Create Math namespace type with all constants and methods
	mathType := types.NewObjectType().
		// Constants (readonly)
		WithReadOnlyProperty("E", types.Number).
		WithReadOnlyProperty("LN10", types.Number).
		WithReadOnlyProperty("LN2", types.Number).
		WithReadOnlyProperty("LOG10E", types.Number).
		WithReadOnlyProperty("LOG2E", types.Number).
		WithReadOnlyProperty("PI", types.Number).
		WithReadOnlyProperty("SQRT1_2", types.Number).
		WithReadOnlyProperty("SQRT2", types.Number).
		// Methods
		WithProperty("abs", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("acos", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("acosh", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("asin", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("asinh", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("atan", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("atanh", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("atan2", types.NewSimpleFunction([]types.Type{types.Number, types.Number}, types.Number)).
		WithProperty("cbrt", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("ceil", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("clz32", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("cos", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("cosh", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("exp", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("expm1", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("floor", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("fround", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("f16round", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithVariadicProperty("hypot", []types.Type{}, types.Number, &types.ArrayType{ElementType: types.Number}).
		WithProperty("imul", types.NewSimpleFunction([]types.Type{types.Number, types.Number}, types.Number)).
		WithProperty("log", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("log1p", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("log10", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("log2", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithVariadicProperty("max", []types.Type{}, types.Number, &types.ArrayType{ElementType: types.Number}).
		WithVariadicProperty("min", []types.Type{}, types.Number, &types.ArrayType{ElementType: types.Number}).
		WithProperty("pow", types.NewSimpleFunction([]types.Type{types.Number, types.Number}, types.Number)).
		WithProperty("random", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("round", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("sign", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("sin", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("sinh", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("sqrt", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("sumPrecise", types.NewSimpleFunction([]types.Type{&types.ArrayType{ElementType: types.Number}}, types.Number)).
		WithProperty("tan", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("tanh", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("trunc", types.NewSimpleFunction([]types.Type{types.Number}, types.Number))

	// Define Math namespace in global environment
	return ctx.DefineGlobal("Math", mathType)
}

func (m *MathInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Math object with Object.prototype as its prototype (ECMAScript spec)
	mathObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Set @@toStringTag to "Math" so Object.prototype.toString.call(Math) returns "[object Math]"
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		falseVal := false
		trueVal := true
		mathObj.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Math"),
			&falseVal, // writable: false
			&falseVal, // enumerable: false
			&trueVal,  // configurable: true (per ECMAScript spec 20.2.1.9)
		)
	}

	// Add constants (non-configurable, non-writable, non-enumerable per ECMAScript spec)
	f := false
	mathObj.DefineOwnProperty("E", vm.NumberValue(math.E), &f, &f, &f)
	mathObj.DefineOwnProperty("LN10", vm.NumberValue(math.Ln10), &f, &f, &f)
	mathObj.DefineOwnProperty("LN2", vm.NumberValue(math.Ln2), &f, &f, &f)
	mathObj.DefineOwnProperty("LOG10E", vm.NumberValue(math.Log10E), &f, &f, &f)
	mathObj.DefineOwnProperty("LOG2E", vm.NumberValue(math.Log2E), &f, &f, &f)
	mathObj.DefineOwnProperty("PI", vm.NumberValue(math.Pi), &f, &f, &f)
	mathObj.DefineOwnProperty("SQRT1_2", vm.NumberValue(math.Sqrt2/2), &f, &f, &f)
	mathObj.DefineOwnProperty("SQRT2", vm.NumberValue(math.Sqrt2), &f, &f, &f)

	// Add methods
	mathObj.SetOwnNonEnumerable("abs", vm.NewNativeFunction(1, false, "abs", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Abs(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("acos", vm.NewNativeFunction(1, false, "acos", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Acos(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("acosh", vm.NewNativeFunction(1, false, "acosh", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Acosh(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("asin", vm.NewNativeFunction(1, false, "asin", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Asin(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("asinh", vm.NewNativeFunction(1, false, "asinh", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Asinh(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("atan", vm.NewNativeFunction(1, false, "atan", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Atan(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("atanh", vm.NewNativeFunction(1, false, "atanh", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Atanh(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("atan2", vm.NewNativeFunction(2, false, "atan2", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Atan2(vmInstance.ToNumber(args[0]), vmInstance.ToNumber(args[1]))), nil
	}))

	mathObj.SetOwnNonEnumerable("cbrt", vm.NewNativeFunction(1, false, "cbrt", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Cbrt(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("ceil", vm.NewNativeFunction(1, false, "ceil", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Ceil(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("clz32", vm.NewNativeFunction(1, false, "clz32", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(32), nil
		}
		// Convert to 32-bit unsigned integer using ToUint32 semantics
		val := toUint32(vmInstance.ToNumber(args[0]))
		if val == 0 {
			return vm.NumberValue(32), nil
		}
		count := 0
		for i := 31; i >= 0; i-- {
			if (val>>i)&1 == 1 {
				break
			}
			count++
		}
		return vm.NumberValue(float64(count)), nil
	}))

	mathObj.SetOwnNonEnumerable("cos", vm.NewNativeFunction(1, false, "cos", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Cos(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("cosh", vm.NewNativeFunction(1, false, "cosh", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Cosh(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("exp", vm.NewNativeFunction(1, false, "exp", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Exp(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("expm1", vm.NewNativeFunction(1, false, "expm1", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Expm1(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("floor", vm.NewNativeFunction(1, false, "floor", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Floor(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("fround", vm.NewNativeFunction(1, false, "fround", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		// Convert to float32 and back to float64 for single precision
		return vm.NumberValue(float64(float32(vmInstance.ToNumber(args[0])))), nil
	}))

	mathObj.SetOwnNonEnumerable("f16round", vm.NewNativeFunction(1, false, "f16round", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		x := vmInstance.ToNumber(args[0])
		// Handle special cases
		if math.IsNaN(x) {
			return vm.NumberValue(math.NaN()), nil
		}
		if math.IsInf(x, 0) || x == 0 {
			return vm.NumberValue(x), nil
		}
		// Convert to half-precision (float16) and back
		return vm.NumberValue(float64ToFloat16ToFloat64(x)), nil
	}))

	mathObj.SetOwnNonEnumerable("hypot", vm.NewNativeFunction(2, true, "hypot", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(0), nil
		}
		// ECMAScript spec: First coerce ALL arguments, then check for infinity/NaN
		// This ensures valueOf/toString is called on all arguments before returning
		coerced := make([]float64, len(args))
		for i, arg := range args {
			// Use ToPrimitive with proper helper call tracking for exception propagation
			if arg.IsObject() || arg.IsCallable() {
				vmInstance.EnterHelperCall()
				primVal := vmInstance.ToPrimitive(arg, "number")
				vmInstance.ExitHelperCall()
				if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
					return vm.Undefined, nil // Let exception propagate
				}
				coerced[i] = primVal.ToFloat()
			} else {
				coerced[i] = arg.ToFloat()
			}
		}
		// Now check for infinity/NaN in coerced values
		hasNaN := false
		sum := 0.0
		for _, val := range coerced {
			if math.IsInf(val, 0) {
				return vm.NumberValue(math.Inf(1)), nil
			}
			if math.IsNaN(val) {
				hasNaN = true
			}
			sum += val * val
		}
		if hasNaN {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Sqrt(sum)), nil
	}))

	mathObj.SetOwnNonEnumerable("imul", vm.NewNativeFunction(2, false, "imul", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.NumberValue(0), nil
		}
		// Per ECMAScript spec:
		// 1. Let a be ToUint32(x).
		// 2. Let b be ToUint32(y).
		// 3. Let product be (a × b) modulo 2^32.
		// 4. If product ≥ 2^31, return product - 2^32; otherwise return product.
		a := toUint32(vmInstance.ToNumber(args[0]))
		b := toUint32(vmInstance.ToNumber(args[1]))
		product := uint64(a) * uint64(b)
		product = product % 4294967296 // modulo 2^32
		if product >= 2147483648 {     // >= 2^31
			return vm.NumberValue(float64(int64(product) - 4294967296)), nil
		}
		return vm.NumberValue(float64(product)), nil
	}))

	mathObj.SetOwnNonEnumerable("log", vm.NewNativeFunction(1, false, "log", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Log(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("log1p", vm.NewNativeFunction(1, false, "log1p", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Log1p(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("log10", vm.NewNativeFunction(1, false, "log10", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Log10(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("log2", vm.NewNativeFunction(1, false, "log2", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Log2(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("max", vm.NewNativeFunction(2, true, "max", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.Inf(-1)), nil // -Infinity
		}
		// ECMAScript spec: First coerce ALL arguments, then find max
		coerced := make([]float64, len(args))
		for i, arg := range args {
			// Use ToPrimitive with proper helper call tracking for exception propagation
			if arg.IsObject() || arg.IsCallable() {
				vmInstance.EnterHelperCall()
				primVal := vmInstance.ToPrimitive(arg, "number")
				vmInstance.ExitHelperCall()
				if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
					return vm.Undefined, nil // Let exception propagate
				}
				coerced[i] = primVal.ToFloat()
			} else {
				coerced[i] = arg.ToFloat()
			}
		}
		// Find max in coerced values
		result := coerced[0]
		for _, val := range coerced {
			if math.IsNaN(val) {
				return vm.NumberValue(math.NaN()), nil
			}
			// +0 is considered larger than -0
			if val > result || (val == 0 && result == 0 && math.Signbit(result) && !math.Signbit(val)) {
				result = val
			}
		}
		return vm.NumberValue(result), nil
	}))

	mathObj.SetOwnNonEnumerable("min", vm.NewNativeFunction(2, true, "min", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.Inf(1)), nil // +Infinity
		}
		// ECMAScript spec: First coerce ALL arguments, then find min
		coerced := make([]float64, len(args))
		for i, arg := range args {
			// Use ToPrimitive with proper helper call tracking for exception propagation
			if arg.IsObject() || arg.IsCallable() {
				vmInstance.EnterHelperCall()
				primVal := vmInstance.ToPrimitive(arg, "number")
				vmInstance.ExitHelperCall()
				if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
					return vm.Undefined, nil // Let exception propagate
				}
				coerced[i] = primVal.ToFloat()
			} else {
				coerced[i] = arg.ToFloat()
			}
		}
		// Find min in coerced values
		result := coerced[0]
		for _, val := range coerced {
			if math.IsNaN(val) {
				return vm.NumberValue(math.NaN()), nil
			}
			// -0 is considered smaller than +0
			if val < result || (val == 0 && result == 0 && !math.Signbit(result) && math.Signbit(val)) {
				result = val
			}
		}
		return vm.NumberValue(result), nil
	}))

	mathObj.SetOwnNonEnumerable("pow", vm.NewNativeFunction(2, false, "pow", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.NumberValue(math.NaN()), nil
		}
		base := vmInstance.ToNumber(args[0])
		exponent := vmInstance.ToNumber(args[1])
		// ECMAScript spec special cases that differ from Go's math.Pow:
		// - If exponent is NaN, return NaN (Go returns 1 for Pow(1, NaN))
		// - If abs(base) is 1 and exponent is +∞ or -∞, return NaN
		if math.IsNaN(exponent) {
			return vm.NumberValue(math.NaN()), nil
		}
		if math.Abs(base) == 1 && math.IsInf(exponent, 0) {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Pow(base, exponent)), nil
	}))

	mathObj.SetOwnNonEnumerable("random", vm.NewNativeFunction(0, false, "random", func(args []vm.Value) (vm.Value, error) {
		return vm.NumberValue(rand.Float64()), nil
	}))

	mathObj.SetOwnNonEnumerable("round", vm.NewNativeFunction(1, false, "round", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		x := vmInstance.ToNumber(args[0])
		// ECMAScript spec special cases:
		// - If x is NaN, return NaN
		// - If x is +0 or -0, return x
		// - If x > 0 and x < 0.5, return +0
		// - If x >= -0.5 and x < 0, return -0 (not -1 like Go's math.Round!)
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return vm.NumberValue(x), nil
		}
		if x == 0 {
			return vm.NumberValue(x), nil // preserves -0
		}
		if x > 0 && x < 0.5 {
			return vm.NumberValue(0), nil
		}
		if x >= -0.5 && x < 0 {
			return vm.NumberValue(math.Copysign(0, -1)), nil // -0
		}
		// For other values, use floor(x + 0.5) which handles positive values correctly
		// But for negative values with fractional part exactly 0.5, we need special handling
		return vm.NumberValue(math.Floor(x + 0.5)), nil
	}))

	mathObj.SetOwnNonEnumerable("sign", vm.NewNativeFunction(1, false, "sign", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		val := vmInstance.ToNumber(args[0])
		if math.IsNaN(val) {
			return vm.NumberValue(math.NaN()), nil
		}
		if val == 0 || val == -0 {
			return vm.NumberValue(val), nil
		}
		if val > 0 {
			return vm.NumberValue(1), nil
		}
		return vm.NumberValue(-1), nil
	}))

	mathObj.SetOwnNonEnumerable("sin", vm.NewNativeFunction(1, false, "sin", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Sin(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("sinh", vm.NewNativeFunction(1, false, "sinh", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Sinh(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("sqrt", vm.NewNativeFunction(1, false, "sqrt", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Sqrt(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("sumPrecise", vm.NewNativeFunction(1, false, "sumPrecise", func(args []vm.Value) (vm.Value, error) {
		// ECMAScript 2025 Math.sumPrecise:
		// 1. If items is not present, throw TypeError
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("Math.sumPrecise requires an iterable argument")
		}

		iterable := args[0]

		// 2. If items is null or undefined, throw TypeError (not iterable)
		if iterable.Type() == vm.TypeNull || iterable.Type() == vm.TypeUndefined {
			return vm.Undefined, vmInstance.NewTypeError("Math.sumPrecise requires an iterable argument")
		}

		// Get the iterator using Symbol.iterator
		var iteratorMethod vm.Value
		var hasIterator bool

		// Check for Symbol.iterator property
		if iterable.Type() == vm.TypeArray {
			// Arrays have builtin iterator via prototype
			if method, ok := vmInstance.GetSymbolProperty(iterable, SymbolIterator); ok && method.IsCallable() {
				iteratorMethod = method
				hasIterator = true
			}
		} else if iterable.IsObject() {
			// Check object for Symbol.iterator
			if method, ok := vmInstance.GetSymbolProperty(iterable, SymbolIterator); ok && method.IsCallable() {
				iteratorMethod = method
				hasIterator = true
			}
		}

		if !hasIterator {
			return vm.Undefined, vmInstance.NewTypeError("Math.sumPrecise requires an iterable argument")
		}

		// Call the iterator method to get the iterator object
		vmInstance.EnterHelperCall()
		iterator, err := vmInstance.Call(iteratorMethod, iterable, nil)
		vmInstance.ExitHelperCall()
		if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
			return vm.Undefined, nil
		}
		if err != nil {
			return vm.Undefined, err
		}

		// Get the 'next' method from the iterator
		nextMethod, err := vmInstance.GetProperty(iterator, "next")
		if err != nil {
			return vm.Undefined, err
		}
		if !nextMethod.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator must have a callable next method")
		}

		// ECMAScript spec for sumPrecise:
		// - If any element is NaN, return NaN (but check for infinities first)
		// - If +Infinity and -Infinity both present, return NaN
		// - If only +Infinity present, return +Infinity
		// - If only -Infinity present, return -Infinity
		// - Otherwise, use Kahan summation for precise sum
		hasNaN := false
		hasPosInf := false
		hasNegInf := false
		hasNegZero := false
		hasPosZero := false
		count := 0
		sum := 0.0
		compensation := 0.0 // Kahan summation compensation

		// Iterate through the iterator
		for {
			// Call next()
			vmInstance.EnterHelperCall()
			result, err := vmInstance.Call(nextMethod, iterator, nil)
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}
			if err != nil {
				return vm.Undefined, err
			}

			// Check if done
			var done bool
			var value vm.Value

			// Get the 'done' property
			if doneVal, err := vmInstance.GetProperty(result, "done"); err == nil {
				done = doneVal.IsTruthy()
			}

			// Get the 'value' property
			if valueVal, err := vmInstance.GetProperty(result, "value"); err == nil {
				value = valueVal
			} else {
				value = vm.Undefined
			}

			if done {
				break
			}

			// Check that value is a number
			if !value.IsNumber() {
				// Close the iterator if it has a return method
				if returnMethod, err := vmInstance.GetProperty(iterator, "return"); err == nil && returnMethod.IsCallable() {
					vmInstance.EnterHelperCall()
					_, _ = vmInstance.Call(returnMethod, iterator, nil)
					vmInstance.ExitHelperCall()
				}
				return vm.Undefined, vmInstance.NewTypeError("Math.sumPrecise can only sum numbers")
			}

			val := vmInstance.ToNumber(value)
			count++

			if math.IsNaN(val) {
				hasNaN = true
			} else if math.IsInf(val, 1) {
				hasPosInf = true
			} else if math.IsInf(val, -1) {
				hasNegInf = true
			} else {
				// Track zeros
				if val == 0 {
					if math.Signbit(val) {
						hasNegZero = true
					} else {
						hasPosZero = true
					}
				}
				// Kahan summation
				y := val - compensation
				t := sum + y
				compensation = (t - sum) - y
				sum = t
			}
		}

		// Handle special cases
		if hasPosInf && hasNegInf {
			return vm.NumberValue(math.NaN()), nil
		}
		if hasPosInf {
			return vm.NumberValue(math.Inf(1)), nil
		}
		if hasNegInf {
			return vm.NumberValue(math.Inf(-1)), nil
		}
		if hasNaN {
			return vm.NumberValue(math.NaN()), nil
		}

		// Return -0 for empty iterable (per spec)
		if count == 0 {
			return vm.NumberValue(math.Copysign(0, -1)), nil
		}

		// Handle zero result
		if sum == 0 {
			// Return -0 only if we had -0 and no +0
			if hasNegZero && !hasPosZero {
				return vm.NumberValue(math.Copysign(0, -1)), nil
			}
			// Otherwise return +0
			return vm.NumberValue(0), nil
		}

		return vm.NumberValue(sum), nil
	}))

	mathObj.SetOwnNonEnumerable("tan", vm.NewNativeFunction(1, false, "tan", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Tan(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("tanh", vm.NewNativeFunction(1, false, "tanh", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Tanh(vmInstance.ToNumber(args[0]))), nil
	}))

	mathObj.SetOwnNonEnumerable("trunc", vm.NewNativeFunction(1, false, "trunc", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Trunc(vmInstance.ToNumber(args[0]))), nil
	}))

	// Register Math object as global
	return ctx.DefineGlobal("Math", vm.NewValueFromPlainObject(mathObj))
}

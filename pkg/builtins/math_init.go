package builtins

import (
	"math"
	"math/rand"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

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
		WithProperty("tan", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("tanh", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("trunc", types.NewSimpleFunction([]types.Type{types.Number}, types.Number))

	// Define Math namespace in global environment
	return ctx.DefineGlobal("Math", mathType)
}

func (m *MathInitializer) InitRuntime(ctx *RuntimeContext) error {
	// Create Math object
	mathObj := vm.NewObject(vm.Null).AsPlainObject()

	// Add constants (non-configurable, non-writable per ECMAScript spec)
	f := false
	t := true
	mathObj.DefineOwnProperty("E", vm.NumberValue(math.E), &f, &t, &f)
	mathObj.DefineOwnProperty("LN10", vm.NumberValue(math.Ln10), &f, &t, &f)
	mathObj.DefineOwnProperty("LN2", vm.NumberValue(math.Ln2), &f, &t, &f)
	mathObj.DefineOwnProperty("LOG10E", vm.NumberValue(math.Log10E), &f, &t, &f)
	mathObj.DefineOwnProperty("LOG2E", vm.NumberValue(math.Log2E), &f, &t, &f)
	mathObj.DefineOwnProperty("PI", vm.NumberValue(math.Pi), &f, &t, &f)
	mathObj.DefineOwnProperty("SQRT1_2", vm.NumberValue(math.Sqrt2/2), &f, &t, &f)
	mathObj.DefineOwnProperty("SQRT2", vm.NumberValue(math.Sqrt2), &f, &t, &f)

	// Add methods
	mathObj.SetOwnNonEnumerable("abs", vm.NewNativeFunction(1, false, "abs", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Abs(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("acos", vm.NewNativeFunction(1, false, "acos", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Acos(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("acosh", vm.NewNativeFunction(1, false, "acosh", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Acosh(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("asin", vm.NewNativeFunction(1, false, "asin", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Asin(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("asinh", vm.NewNativeFunction(1, false, "asinh", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Asinh(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("atan", vm.NewNativeFunction(1, false, "atan", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Atan(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("atanh", vm.NewNativeFunction(1, false, "atanh", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Atanh(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("atan2", vm.NewNativeFunction(2, false, "atan2", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Atan2(args[0].ToFloat(), args[1].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("cbrt", vm.NewNativeFunction(1, false, "cbrt", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Cbrt(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("ceil", vm.NewNativeFunction(1, false, "ceil", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Ceil(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("clz32", vm.NewNativeFunction(1, false, "clz32", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(32), nil
		}
		// Convert to 32-bit unsigned integer
		val := uint32(args[0].ToFloat())
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
		return vm.NumberValue(math.Cos(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("cosh", vm.NewNativeFunction(1, false, "cosh", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Cosh(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("exp", vm.NewNativeFunction(1, false, "exp", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Exp(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("expm1", vm.NewNativeFunction(1, false, "expm1", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Expm1(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("floor", vm.NewNativeFunction(1, false, "floor", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Floor(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("fround", vm.NewNativeFunction(1, false, "fround", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		// Convert to float32 and back to float64 for single precision
		return vm.NumberValue(float64(float32(args[0].ToFloat()))), nil
	}))

	mathObj.SetOwnNonEnumerable("hypot", vm.NewNativeFunction(0, true, "hypot", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(0), nil
		}
		sum := 0.0
		for _, arg := range args {
			val := arg.ToFloat()
			sum += val * val
		}
		return vm.NumberValue(math.Sqrt(sum)), nil
	}))

	mathObj.SetOwnNonEnumerable("imul", vm.NewNativeFunction(2, false, "imul", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.NumberValue(0), nil
		}
		a := int32(args[0].ToFloat())
		b := int32(args[1].ToFloat())
		return vm.NumberValue(float64(a * b)), nil
	}))

	mathObj.SetOwnNonEnumerable("log", vm.NewNativeFunction(1, false, "log", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Log(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("log1p", vm.NewNativeFunction(1, false, "log1p", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Log1p(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("log10", vm.NewNativeFunction(1, false, "log10", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Log10(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("log2", vm.NewNativeFunction(1, false, "log2", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Log2(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("max", vm.NewNativeFunction(0, true, "max", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.Inf(-1)), nil // -Infinity
		}
		max := args[0].ToFloat()
		for i := 1; i < len(args); i++ {
			val := args[i].ToFloat()
			if val > max || math.IsNaN(val) {
				max = val
			}
		}
		return vm.NumberValue(max), nil
	}))

	mathObj.SetOwnNonEnumerable("min", vm.NewNativeFunction(0, true, "min", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.Inf(1)), nil // +Infinity
		}
		min := args[0].ToFloat()
		for i := 1; i < len(args); i++ {
			val := args[i].ToFloat()
			if val < min || math.IsNaN(val) {
				min = val
			}
		}
		return vm.NumberValue(min), nil
	}))

	mathObj.SetOwnNonEnumerable("pow", vm.NewNativeFunction(2, false, "pow", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Pow(args[0].ToFloat(), args[1].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("random", vm.NewNativeFunction(0, false, "random", func(args []vm.Value) (vm.Value, error) {
		return vm.NumberValue(rand.Float64()), nil
	}))

	mathObj.SetOwnNonEnumerable("round", vm.NewNativeFunction(1, false, "round", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Round(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("sign", vm.NewNativeFunction(1, false, "sign", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		val := args[0].ToFloat()
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
		return vm.NumberValue(math.Sin(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("sinh", vm.NewNativeFunction(1, false, "sinh", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Sinh(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("sqrt", vm.NewNativeFunction(1, false, "sqrt", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Sqrt(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("tan", vm.NewNativeFunction(1, false, "tan", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Tan(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("tanh", vm.NewNativeFunction(1, false, "tanh", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Tanh(args[0].ToFloat())), nil
	}))

	mathObj.SetOwnNonEnumerable("trunc", vm.NewNativeFunction(1, false, "trunc", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NumberValue(math.NaN()), nil
		}
		return vm.NumberValue(math.Trunc(args[0].ToFloat())), nil
	}))

	// Register Math object as global
	return ctx.DefineGlobal("Math", vm.NewValueFromPlainObject(mathObj))
}

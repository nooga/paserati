package builtins

import (
	"math"
	"math/rand"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// registerMath creates and registers the Math object with its methods and constants
func registerMath() {
	// Create the Math object as a PlainObject
	mathObj := vm.NewObject(vm.Undefined)
	mathObject := mathObj.AsPlainObject()

	// Register Math constants
	mathObject.SetOwn("E", vm.Number(math.E))
	mathObject.SetOwn("LN10", vm.Number(math.Ln10))
	mathObject.SetOwn("LN2", vm.Number(math.Ln2))
	mathObject.SetOwn("LOG10E", vm.Number(math.Log10E))
	mathObject.SetOwn("LOG2E", vm.Number(math.Log2E))
	mathObject.SetOwn("PI", vm.Number(math.Pi))
	mathObject.SetOwn("SQRT1_2", vm.Number(math.Sqrt2/2))
	mathObject.SetOwn("SQRT2", vm.Number(math.Sqrt2))

	// Register Math methods
	mathObject.SetOwn("abs", vm.NewNativeFunction(1, false, "abs", mathAbsImpl))
	mathObject.SetOwn("acos", vm.NewNativeFunction(1, false, "acos", mathAcosImpl))
	mathObject.SetOwn("acosh", vm.NewNativeFunction(1, false, "acosh", mathAcoshImpl))
	mathObject.SetOwn("asin", vm.NewNativeFunction(1, false, "asin", mathAsinImpl))
	mathObject.SetOwn("asinh", vm.NewNativeFunction(1, false, "asinh", mathAsinhImpl))
	mathObject.SetOwn("atan", vm.NewNativeFunction(1, false, "atan", mathAtanImpl))
	mathObject.SetOwn("atanh", vm.NewNativeFunction(1, false, "atanh", mathAtanhImpl))
	mathObject.SetOwn("atan2", vm.NewNativeFunction(2, false, "atan2", mathAtan2Impl))
	mathObject.SetOwn("cbrt", vm.NewNativeFunction(1, false, "cbrt", mathCbrtImpl))
	mathObject.SetOwn("ceil", vm.NewNativeFunction(1, false, "ceil", mathCeilImpl))
	mathObject.SetOwn("clz32", vm.NewNativeFunction(1, false, "clz32", mathClz32Impl))
	mathObject.SetOwn("cos", vm.NewNativeFunction(1, false, "cos", mathCosImpl))
	mathObject.SetOwn("cosh", vm.NewNativeFunction(1, false, "cosh", mathCoshImpl))
	mathObject.SetOwn("exp", vm.NewNativeFunction(1, false, "exp", mathExpImpl))
	mathObject.SetOwn("expm1", vm.NewNativeFunction(1, false, "expm1", mathExpm1Impl))
	mathObject.SetOwn("floor", vm.NewNativeFunction(1, false, "floor", mathFloorImpl))
	mathObject.SetOwn("fround", vm.NewNativeFunction(1, false, "fround", mathFroundImpl))
	mathObject.SetOwn("hypot", vm.NewNativeFunction(-1, true, "hypot", mathHypotImpl))
	mathObject.SetOwn("imul", vm.NewNativeFunction(2, false, "imul", mathImulImpl))
	mathObject.SetOwn("log", vm.NewNativeFunction(1, false, "log", mathLogImpl))
	mathObject.SetOwn("log1p", vm.NewNativeFunction(1, false, "log1p", mathLog1pImpl))
	mathObject.SetOwn("log10", vm.NewNativeFunction(1, false, "log10", mathLog10Impl))
	mathObject.SetOwn("log2", vm.NewNativeFunction(1, false, "log2", mathLog2Impl))
	mathObject.SetOwn("max", vm.NewNativeFunction(-1, true, "max", mathMaxImpl))
	mathObject.SetOwn("min", vm.NewNativeFunction(-1, true, "min", mathMinImpl))
	mathObject.SetOwn("pow", vm.NewNativeFunction(2, false, "pow", mathPowImpl))
	mathObject.SetOwn("random", vm.NewNativeFunction(0, false, "random", mathRandomImpl))
	mathObject.SetOwn("round", vm.NewNativeFunction(1, false, "round", mathRoundImpl))
	mathObject.SetOwn("sign", vm.NewNativeFunction(1, false, "sign", mathSignImpl))
	mathObject.SetOwn("sin", vm.NewNativeFunction(1, false, "sin", mathSinImpl))
	mathObject.SetOwn("sinh", vm.NewNativeFunction(1, false, "sinh", mathSinhImpl))
	mathObject.SetOwn("sqrt", vm.NewNativeFunction(1, false, "sqrt", mathSqrtImpl))
	mathObject.SetOwn("tan", vm.NewNativeFunction(1, false, "tan", mathTanImpl))
	mathObject.SetOwn("tanh", vm.NewNativeFunction(1, false, "tanh", mathTanhImpl))
	mathObject.SetOwn("trunc", vm.NewNativeFunction(1, false, "trunc", mathTruncImpl))

	// Define the type for Math object with all methods and constants
	mathType := &types.ObjectType{Properties: map[string]types.Type{
		// Constants
		"E":       types.Number,
		"LN10":    types.Number,
		"LN2":     types.Number,
		"LOG10E":  types.Number,
		"LOG2E":   types.Number,
		"PI":      types.Number,
		"SQRT1_2": types.Number,
		"SQRT2":   types.Number,

		// Methods - single parameter functions
		"abs":    &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"acos":   &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"acosh":  &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"asin":   &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"asinh":  &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"atan":   &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"atanh":  &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"cbrt":   &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"ceil":   &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"clz32":  &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"cos":    &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"cosh":   &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"exp":    &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"expm1":  &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"floor":  &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"fround": &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"log":    &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"log1p":  &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"log10":  &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"log2":   &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"round":  &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"sign":   &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"sin":    &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"sinh":   &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"sqrt":   &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"tan":    &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"tanh":   &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},
		"trunc":  &types.FunctionType{ParameterTypes: []types.Type{types.Number}, ReturnType: types.Number, IsVariadic: false},

		// Two parameter functions
		"atan2": &types.FunctionType{ParameterTypes: []types.Type{types.Number, types.Number}, ReturnType: types.Number, IsVariadic: false},
		"imul":  &types.FunctionType{ParameterTypes: []types.Type{types.Number, types.Number}, ReturnType: types.Number, IsVariadic: false},
		"pow":   &types.FunctionType{ParameterTypes: []types.Type{types.Number, types.Number}, ReturnType: types.Number, IsVariadic: false},

		// Zero parameter functions
		"random": &types.FunctionType{ParameterTypes: []types.Type{}, ReturnType: types.Number, IsVariadic: false},

		// Variadic functions
		"hypot": &types.FunctionType{ParameterTypes: []types.Type{}, ReturnType: types.Number, IsVariadic: true, RestParameterType: &types.ArrayType{ElementType: types.Number}},
		"max":   &types.FunctionType{ParameterTypes: []types.Type{}, ReturnType: types.Number, IsVariadic: true, RestParameterType: &types.ArrayType{ElementType: types.Number}},
		"min":   &types.FunctionType{ParameterTypes: []types.Type{}, ReturnType: types.Number, IsVariadic: true, RestParameterType: &types.ArrayType{ElementType: types.Number}},
	}}

	// Register the Math object
	registerObject("Math", mathObj, mathType)
}

// --- Math Method Implementations ---

func mathAbsImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Abs(args[0].ToFloat()))
}

func mathAcosImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Acos(args[0].ToFloat()))
}

func mathAcoshImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Acosh(args[0].ToFloat()))
}

func mathAsinImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Asin(args[0].ToFloat()))
}

func mathAsinhImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Asinh(args[0].ToFloat()))
}

func mathAtanImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Atan(args[0].ToFloat()))
}

func mathAtanhImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Atanh(args[0].ToFloat()))
}

func mathAtan2Impl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Atan2(args[0].ToFloat(), args[1].ToFloat()))
}

func mathCbrtImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Cbrt(args[0].ToFloat()))
}

func mathCeilImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Ceil(args[0].ToFloat()))
}

func mathClz32Impl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(32)
	}
	n := uint32(args[0].ToFloat())
	if n == 0 {
		return vm.Number(32)
	}
	// Count leading zeros in 32-bit representation
	count := 0
	for i := 31; i >= 0; i-- {
		if (n & (1 << i)) != 0 {
			break
		}
		count++
	}
	return vm.Number(float64(count))
}

func mathCosImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Cos(args[0].ToFloat()))
}

func mathCoshImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Cosh(args[0].ToFloat()))
}

func mathExpImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Exp(args[0].ToFloat()))
}

func mathExpm1Impl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Expm1(args[0].ToFloat()))
}

func mathFloorImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Floor(args[0].ToFloat()))
}

func mathFroundImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	// Convert to float32 and back to simulate JavaScript's fround
	return vm.Number(float64(float32(args[0].ToFloat())))
}

func mathHypotImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(0)
	}
	sum := 0.0
	for _, arg := range args {
		val := arg.ToFloat()
		sum += val * val
	}
	return vm.Number(math.Sqrt(sum))
}

func mathImulImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.Number(0)
	}
	// JavaScript's imul performs 32-bit integer multiplication
	a := int32(args[0].ToFloat())
	b := int32(args[1].ToFloat())
	return vm.Number(float64(a * b))
}

func mathLogImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Log(args[0].ToFloat()))
}

func mathLog1pImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Log1p(args[0].ToFloat()))
}

func mathLog10Impl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Log10(args[0].ToFloat()))
}

func mathLog2Impl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Log2(args[0].ToFloat()))
}

func mathMaxImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.Inf(-1)) // -Infinity
	}
	max := args[0].ToFloat()
	for i := 1; i < len(args); i++ {
		val := args[i].ToFloat()
		if math.IsNaN(val) {
			return vm.Number(math.NaN())
		}
		if val > max {
			max = val
		}
	}
	return vm.Number(max)
}

func mathMinImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.Inf(1)) // +Infinity
	}
	min := args[0].ToFloat()
	for i := 1; i < len(args); i++ {
		val := args[i].ToFloat()
		if math.IsNaN(val) {
			return vm.Number(math.NaN())
		}
		if val < min {
			min = val
		}
	}
	return vm.Number(min)
}

func mathPowImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Pow(args[0].ToFloat(), args[1].ToFloat()))
}

func mathRandomImpl(args []vm.Value) vm.Value {
	return vm.Number(rand.Float64())
}

func mathRoundImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Round(args[0].ToFloat()))
}

func mathSignImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	val := args[0].ToFloat()
	if math.IsNaN(val) {
		return vm.Number(math.NaN())
	}
	if val > 0 {
		return vm.Number(1)
	}
	if val < 0 {
		return vm.Number(-1)
	}
	return vm.Number(val) // ±0
}

func mathSinImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Sin(args[0].ToFloat()))
}

func mathSinhImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Sinh(args[0].ToFloat()))
}

func mathSqrtImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Sqrt(args[0].ToFloat()))
}

func mathTanImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Tan(args[0].ToFloat()))
}

func mathTanhImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Tanh(args[0].ToFloat()))
}

func mathTruncImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Number(math.NaN())
	}
	return vm.Number(math.Trunc(args[0].ToFloat()))
}

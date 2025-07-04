package builtins

import (
	"math"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"strconv"
)

type NumberInitializer struct{}

func (n *NumberInitializer) Name() string {
	return "Number"
}

func (n *NumberInitializer) Priority() int {
	return 350 // After String (300)
}

func (n *NumberInitializer) InitTypes(ctx *TypeContext) error {
	// Create Number constructor type first (needed for constructor property)
	numberCtorType := types.NewSimpleFunction([]types.Type{types.Any}, types.Number).
		WithProperty("MAX_VALUE", types.Number).
		WithProperty("MIN_VALUE", types.Number).
		WithProperty("NaN", types.Number).
		WithProperty("NEGATIVE_INFINITY", types.Number).
		WithProperty("POSITIVE_INFINITY", types.Number).
		WithProperty("MAX_SAFE_INTEGER", types.Number).
		WithProperty("MIN_SAFE_INTEGER", types.Number).
		WithProperty("EPSILON", types.Number).
		WithProperty("isNaN", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("isFinite", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("isInteger", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("isSafeInteger", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("parseFloat", types.NewSimpleFunction([]types.Type{types.String}, types.Number)).
		WithProperty("parseInt", types.NewOptionalFunction([]types.Type{types.String, types.Number}, types.Number, []bool{false, true}))

	// Create Number.prototype type with all methods
	// Note: 'this' is implicit and not included in type signatures
	numberProtoType := types.NewObjectType().
		WithProperty("toString", types.NewOptionalFunction([]types.Type{types.Number}, types.String, []bool{true})).
		WithProperty("toLocaleString", types.NewOptionalFunction([]types.Type{types.String, types.Any}, types.String, []bool{true, true})).
		WithProperty("valueOf", types.NewSimpleFunction([]types.Type{}, types.Number)).
		WithProperty("toFixed", types.NewOptionalFunction([]types.Type{types.Number}, types.String, []bool{true})).
		WithProperty("toExponential", types.NewOptionalFunction([]types.Type{types.Number}, types.String, []bool{true})).
		WithProperty("toPrecision", types.NewOptionalFunction([]types.Type{types.Number}, types.String, []bool{true})).
		WithProperty("constructor", types.Any) // Avoid circular reference, use Any for constructor property

	// Register number primitive prototype
	ctx.SetPrimitivePrototype("number", numberProtoType)

	// Add prototype property to constructor
	numberCtorType = numberCtorType.WithProperty("prototype", numberProtoType)

	// Define Number constructor in global environment
	return ctx.DefineGlobal("Number", numberCtorType)
}

func (n *NumberInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Number.prototype inheriting from Object.prototype
	numberProto := vm.NewObject(objectProto).AsPlainObject()

	// Add Number prototype methods
	numberProto.SetOwn("toString", vm.NewNativeFunction(1, false, "toString", func(args []vm.Value) vm.Value {
		thisNum := vmInstance.GetThis()
		
		// Check if this is a number
		if thisNum.Type() != vm.TypeFloatNumber && thisNum.Type() != vm.TypeIntegerNumber {
			// Try to convert or throw error
			if thisNum.Type() == vm.TypeBigInt {
				return vm.NewString(thisNum.ToString())
			}
			// For non-numbers, we could throw a TypeError but for now return string conversion
			return vm.NewString(thisNum.ToString())
		}

		var radix int = 10
		if len(args) > 0 {
			radix = int(args[0].ToFloat())
			if radix < 2 || radix > 36 {
				// In real JS this would throw RangeError, for now use default
				radix = 10
			}
		}

		if radix == 10 {
			return vm.NewString(thisNum.ToString())
		}

		// Handle different radix
		if thisNum.Type() == vm.TypeIntegerNumber {
			return vm.NewString(strconv.FormatInt(int64(thisNum.AsInteger()), radix))
		} else {
			// For float numbers with non-10 radix, convert to int first (JS behavior)
			intVal := int64(thisNum.ToFloat())
			return vm.NewString(strconv.FormatInt(intVal, radix))
		}
	}))

	numberProto.SetOwn("toLocaleString", vm.NewNativeFunction(2, false, "toLocaleString", func(args []vm.Value) vm.Value {
		thisNum := vmInstance.GetThis()
		
		// Check if this is a number
		if thisNum.Type() != vm.TypeFloatNumber && thisNum.Type() != vm.TypeIntegerNumber && thisNum.Type() != vm.TypeBigInt {
			return vm.NewString(thisNum.ToString())
		}

		// For now, just return the string representation (proper locale support would be complex)
		// TODO: Implement proper locale formatting
		return vm.NewString(thisNum.ToString())
	}))

	numberProto.SetOwn("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) vm.Value {
		thisNum := vmInstance.GetThis()
		
		// Return the primitive number value
		if thisNum.Type() == vm.TypeFloatNumber || thisNum.Type() == vm.TypeIntegerNumber {
			return thisNum
		}
		
		// Convert if possible
		return vm.NumberValue(thisNum.ToFloat())
	}))

	numberProto.SetOwn("toFixed", vm.NewNativeFunction(1, false, "toFixed", func(args []vm.Value) vm.Value {
		thisNum := vmInstance.GetThis()
		
		if thisNum.Type() != vm.TypeFloatNumber && thisNum.Type() != vm.TypeIntegerNumber {
			return vm.NewString(thisNum.ToString())
		}

		digits := 0
		if len(args) > 0 {
			digits = int(args[0].ToFloat())
			if digits < 0 || digits > 20 {
				// In real JS this would throw RangeError
				digits = 0
			}
		}

		numVal := thisNum.ToFloat()
		return vm.NewString(strconv.FormatFloat(numVal, 'f', digits, 64))
	}))

	numberProto.SetOwn("toExponential", vm.NewNativeFunction(1, false, "toExponential", func(args []vm.Value) vm.Value {
		thisNum := vmInstance.GetThis()
		
		if thisNum.Type() != vm.TypeFloatNumber && thisNum.Type() != vm.TypeIntegerNumber {
			return vm.NewString(thisNum.ToString())
		}

		digits := -1
		if len(args) > 0 {
			digits = int(args[0].ToFloat())
			if digits < 0 || digits > 20 {
				// In real JS this would throw RangeError
				digits = -1
			}
		}

		numVal := thisNum.ToFloat()
		return vm.NewString(strconv.FormatFloat(numVal, 'e', digits, 64))
	}))

	numberProto.SetOwn("toPrecision", vm.NewNativeFunction(1, false, "toPrecision", func(args []vm.Value) vm.Value {
		thisNum := vmInstance.GetThis()
		
		if thisNum.Type() != vm.TypeFloatNumber && thisNum.Type() != vm.TypeIntegerNumber {
			return vm.NewString(thisNum.ToString())
		}

		if len(args) == 0 {
			return vm.NewString(thisNum.ToString())
		}

		precision := int(args[0].ToFloat())
		if precision < 1 || precision > 21 {
			// In real JS this would throw RangeError
			return vm.NewString(thisNum.ToString())
		}

		numVal := thisNum.ToFloat()
		return vm.NewString(strconv.FormatFloat(numVal, 'g', precision, 64))
	}))

	// Set Number.prototype
	vmInstance.NumberPrototype = vm.NewValueFromPlainObject(numberProto)

	// Create Number constructor function
	numberConstructor := vm.NewNativeFunctionWithProps(1, false, "Number", func(args []vm.Value) vm.Value {
		if len(args) == 0 {
			return vm.NumberValue(0)
		}
		
		arg := args[0]
		switch arg.Type() {
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			return arg
		case vm.TypeString:
			str := arg.ToString()
			if str == "" {
				return vm.NumberValue(0)
			}
			if val, err := strconv.ParseFloat(str, 64); err == nil {
				return vm.NumberValue(val)
			}
			return vm.NaN
		case vm.TypeBoolean:
			if arg.AsBoolean() {
				return vm.NumberValue(1)
			}
			return vm.NumberValue(0)
		case vm.TypeNull:
			return vm.NumberValue(0)
		case vm.TypeUndefined:
			return vm.NaN
		default:
			return vm.NaN
		}
	})

	// Add Number static properties
	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("MAX_VALUE", vm.NumberValue(math.MaxFloat64))
	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("MIN_VALUE", vm.NumberValue(math.SmallestNonzeroFloat64))
	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("NaN", vm.NaN)
	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("NEGATIVE_INFINITY", vm.NumberValue(math.Inf(-1)))
	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("POSITIVE_INFINITY", vm.NumberValue(math.Inf(1)))
	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("MAX_SAFE_INTEGER", vm.NumberValue(9007199254740991))  // 2^53 - 1
	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("MIN_SAFE_INTEGER", vm.NumberValue(-9007199254740991)) // -(2^53 - 1)
	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("EPSILON", vm.NumberValue(math.Nextafter(1.0, 2.0)-1.0))

	// Add Number static methods
	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("isNaN", vm.NewNativeFunction(1, false, "isNaN", func(args []vm.Value) vm.Value {
		if len(args) == 0 {
			return vm.BooleanValue(false)
		}
		val := args[0]
		if val.Type() != vm.TypeFloatNumber && val.Type() != vm.TypeIntegerNumber {
			return vm.BooleanValue(false) // Only numbers can be NaN
		}
		return vm.BooleanValue(math.IsNaN(val.ToFloat()))
	}))

	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("isFinite", vm.NewNativeFunction(1, false, "isFinite", func(args []vm.Value) vm.Value {
		if len(args) == 0 {
			return vm.BooleanValue(false)
		}
		val := args[0]
		if val.Type() != vm.TypeFloatNumber && val.Type() != vm.TypeIntegerNumber {
			return vm.BooleanValue(false) // Only numbers can be finite
		}
		f := val.ToFloat()
		return vm.BooleanValue(!math.IsInf(f, 0) && !math.IsNaN(f))
	}))

	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("isInteger", vm.NewNativeFunction(1, false, "isInteger", func(args []vm.Value) vm.Value {
		if len(args) == 0 {
			return vm.BooleanValue(false)
		}
		val := args[0]
		if val.Type() != vm.TypeFloatNumber && val.Type() != vm.TypeIntegerNumber {
			return vm.BooleanValue(false)
		}
		f := val.ToFloat()
		return vm.BooleanValue(!math.IsInf(f, 0) && !math.IsNaN(f) && math.Floor(f) == f)
	}))

	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("isSafeInteger", vm.NewNativeFunction(1, false, "isSafeInteger", func(args []vm.Value) vm.Value {
		if len(args) == 0 {
			return vm.BooleanValue(false)
		}
		val := args[0]
		if val.Type() != vm.TypeFloatNumber && val.Type() != vm.TypeIntegerNumber {
			return vm.BooleanValue(false)
		}
		f := val.ToFloat()
		maxSafe := 9007199254740991.0 // 2^53 - 1
		return vm.BooleanValue(!math.IsInf(f, 0) && !math.IsNaN(f) && math.Floor(f) == f && f >= -maxSafe && f <= maxSafe)
	}))

	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("parseFloat", vm.NewNativeFunction(1, false, "parseFloat", func(args []vm.Value) vm.Value {
		if len(args) == 0 {
			return vm.NaN
		}
		str := args[0].ToString()
		if val, err := strconv.ParseFloat(str, 64); err == nil {
			return vm.NumberValue(val)
		}
		return vm.NaN
	}))

	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("parseInt", vm.NewNativeFunction(2, false, "parseInt", func(args []vm.Value) vm.Value {
		if len(args) == 0 {
			return vm.NaN
		}
		
		str := args[0].ToString()
		radix := 10
		if len(args) > 1 {
			radix = int(args[1].ToFloat())
		}
		
		if radix == 0 {
			radix = 10 // Default radix
		}
		if radix < 2 || radix > 36 {
			return vm.NaN
		}
		
		if val, err := strconv.ParseInt(str, radix, 64); err == nil {
			return vm.NumberValue(float64(val))
		}
		return vm.NaN
	}))

	numberConstructor.AsNativeFunctionWithProps().Properties.SetOwn("prototype", vmInstance.NumberPrototype)

	// Set constructor property on prototype
	numberProto.SetOwn("constructor", numberConstructor)

	// Define Number constructor in global scope
	return ctx.DefineGlobal("Number", numberConstructor)
}
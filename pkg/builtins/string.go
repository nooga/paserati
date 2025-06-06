package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// registerString registers the String constructor and static methods
func registerString() {
	// Register String constructor using NativeFunctionWithProps
	stringConstructorValue := vm.NewNativeFunctionWithProps(-1, true, "String", stringConstructor)

	// Add fromCharCode as a property on the String constructor
	stringConstructorObj := stringConstructorValue.AsNativeFunctionWithProps()
	stringConstructorObj.Properties.SetOwn("fromCharCode",
		vm.NewNativeFunction(-1, true, "fromCharCode", stringFromCharCode))

	// Register with type checker using CallableType
	stringCallableType := &types.CallableType{
		CallSignature: &types.FunctionType{
			ParameterTypes:    []types.Type{}, // No fixed parameters
			ReturnType:        types.String,
			IsVariadic:        true,
			RestParameterType: &types.ArrayType{ElementType: types.Any}, // Accept any values
		},
		Properties: map[string]types.Type{
			"fromCharCode": &types.FunctionType{
				ParameterTypes:    []types.Type{}, // No fixed parameters
				ReturnType:        types.String,
				IsVariadic:        true,
				RestParameterType: &types.ArrayType{ElementType: types.Number}, // Accept numbers only
			},
		},
	}

	registerValue("String", stringConstructorValue, stringCallableType)

	// Register String prototype methods (both runtime and types)
	registerStringPrototypeMethods()
}

// registerStringPrototypeMethods registers String prototype methods with both implementations and types
func registerStringPrototypeMethods() {
	// Register charAt method
	vm.RegisterStringPrototypeMethod("charAt",
		vm.NewNativeFunction(1, false, "charAt", stringCharAtImpl))
	RegisterPrototypeMethod("string", "charAt", &types.FunctionType{
		ParameterTypes: []types.Type{types.Number},
		ReturnType:     types.String,
		IsVariadic:     false,
	})

	// Register charCodeAt method
	vm.RegisterStringPrototypeMethod("charCodeAt",
		vm.NewNativeFunction(1, false, "charCodeAt", stringCharCodeAtImpl))
	RegisterPrototypeMethod("string", "charCodeAt", &types.FunctionType{
		ParameterTypes: []types.Type{types.Number},
		ReturnType:     types.Number,
		IsVariadic:     false,
	})

	// Register substring method
	vm.RegisterStringPrototypeMethod("substring",
		vm.NewNativeFunction(2, false, "substring", stringSubstringImpl))
	RegisterPrototypeMethod("string", "substring", &types.FunctionType{
		ParameterTypes: []types.Type{types.Number, types.Number},
		ReturnType:     types.String,
		IsVariadic:     false,
		OptionalParams: []bool{false, true}, // start required, end optional
	})

	// Register slice method
	vm.RegisterStringPrototypeMethod("slice",
		vm.NewNativeFunction(2, false, "slice", stringSliceImpl))
	RegisterPrototypeMethod("string", "slice", &types.FunctionType{
		ParameterTypes: []types.Type{types.Number, types.Number},
		ReturnType:     types.String,
		IsVariadic:     false,
		OptionalParams: []bool{false, true}, // start required, end optional
	})

	// Register indexOf method
	vm.RegisterStringPrototypeMethod("indexOf",
		vm.NewNativeFunction(1, false, "indexOf", stringIndexOfImpl))
	RegisterPrototypeMethod("string", "indexOf", &types.FunctionType{
		ParameterTypes: []types.Type{types.String},
		ReturnType:     types.Number,
		IsVariadic:     false,
	})

	// Register includes method
	vm.RegisterStringPrototypeMethod("includes",
		vm.NewNativeFunction(1, false, "includes", stringIncludesImpl))
	RegisterPrototypeMethod("string", "includes", &types.FunctionType{
		ParameterTypes: []types.Type{types.String},
		ReturnType:     types.Boolean,
		IsVariadic:     false,
	})

	// Register startsWith method
	vm.RegisterStringPrototypeMethod("startsWith",
		vm.NewNativeFunction(1, false, "startsWith", stringStartsWithImpl))
	RegisterPrototypeMethod("string", "startsWith", &types.FunctionType{
		ParameterTypes: []types.Type{types.String},
		ReturnType:     types.Boolean,
		IsVariadic:     false,
	})

	// Register endsWith method
	vm.RegisterStringPrototypeMethod("endsWith",
		vm.NewNativeFunction(1, false, "endsWith", stringEndsWithImpl))
	RegisterPrototypeMethod("string", "endsWith", &types.FunctionType{
		ParameterTypes: []types.Type{types.String},
		ReturnType:     types.Boolean,
		IsVariadic:     false,
	})
}

// stringCharAtImpl implements String.prototype.charAt
func stringCharAtImpl(args []vm.Value) vm.Value {
	// args[0] is 'this' (the string), args[1] is the index
	if len(args) < 2 {
		return vm.NewString("")
	}

	str := args[0].ToString()
	index := int(args[1].ToFloat())

	if index < 0 || index >= len(str) {
		return vm.NewString("")
	}

	return vm.NewString(string(str[index]))
}

// stringCharCodeAtImpl implements String.prototype.charCodeAt
func stringCharCodeAtImpl(args []vm.Value) vm.Value {
	// args[0] is 'this' (the string), args[1] is the index
	if len(args) < 2 {
		return vm.Number(float64('N')) // Default to 'N' if no args? Or NaN?
	}

	str := args[0].ToString()
	index := int(args[1].ToFloat())

	if index < 0 || index >= len(str) {
		return vm.Number(float64('?')) // Return '?' for out of bounds? Or NaN?
	}

	return vm.Number(float64(str[index]))
}

// stringSubstringImpl implements String.prototype.substring
func stringSubstringImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.NewString("")
	}

	str := args[0].ToString()
	start := int(args[1].ToFloat())

	// Default end to string length if not provided
	end := len(str)
	if len(args) > 2 && args[2].Type() != vm.TypeUndefined {
		end = int(args[2].ToFloat())
	}

	// Clamp values
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if start > len(str) {
		start = len(str)
	}
	if end > len(str) {
		end = len(str)
	}

	// Swap if start > end
	if start > end {
		start, end = end, start
	}

	return vm.NewString(str[start:end])
}

// stringSliceImpl implements String.prototype.slice
func stringSliceImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.NewString("")
	}

	str := args[0].ToString()
	start := int(args[1].ToFloat())

	// Default end to string length if not provided
	end := len(str)
	if len(args) > 2 && args[2].Type() != vm.TypeUndefined {
		end = int(args[2].ToFloat())
	}

	// Handle negative indices
	if start < 0 {
		start = len(str) + start
		if start < 0 {
			start = 0
		}
	}
	if end < 0 {
		end = len(str) + end
		if end < 0 {
			end = 0
		}
	}

	// Clamp to string bounds
	if start > len(str) {
		start = len(str)
	}
	if end > len(str) {
		end = len(str)
	}

	// Return empty string if start >= end
	if start >= end {
		return vm.NewString("")
	}

	return vm.NewString(str[start:end])
}

// stringIndexOfImpl implements String.prototype.indexOf
func stringIndexOfImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.Number(-1)
	}

	str := args[0].ToString()
	searchStr := args[1].ToString()

	for i := 0; i <= len(str)-len(searchStr); i++ {
		if str[i:i+len(searchStr)] == searchStr {
			return vm.Number(float64(i))
		}
	}

	return vm.Number(-1)
}

// stringIncludesImpl implements String.prototype.includes
func stringIncludesImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.BooleanValue(false)
	}

	str := args[0].ToString()
	searchStr := args[1].ToString()

	for i := 0; i <= len(str)-len(searchStr); i++ {
		if str[i:i+len(searchStr)] == searchStr {
			return vm.BooleanValue(true)
		}
	}

	return vm.BooleanValue(false)
}

// stringStartsWithImpl implements String.prototype.startsWith
func stringStartsWithImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.BooleanValue(false)
	}

	str := args[0].ToString()
	searchStr := args[1].ToString()

	if len(searchStr) > len(str) {
		return vm.BooleanValue(false)
	}

	return vm.BooleanValue(str[:len(searchStr)] == searchStr)
}

// stringEndsWithImpl implements String.prototype.endsWith
func stringEndsWithImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.BooleanValue(false)
	}

	str := args[0].ToString()
	searchStr := args[1].ToString()

	if len(searchStr) > len(str) {
		return vm.BooleanValue(false)
	}

	return vm.BooleanValue(str[len(str)-len(searchStr):] == searchStr)
}

// stringConstructor implements the String() constructor
func stringConstructor(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.NewString("")
	}
	return vm.NewString(args[0].ToString())
}

// stringFromCharCode implements String.fromCharCode (static method)
func stringFromCharCode(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.NewString("")
	}

	result := make([]byte, len(args))
	for i, arg := range args {
		code := int(arg.ToFloat()) & 0xFFFF // Mask to 16 bits like JS
		result[i] = byte(code)
	}

	return vm.NewString(string(result))
}

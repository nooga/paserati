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

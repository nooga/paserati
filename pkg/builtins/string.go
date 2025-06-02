package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// registerString registers the String constructor and static methods
func registerString() {
	// Register String constructor - should handle String() and String(...values)
	register("String", -1, true, stringConstructor, &types.FunctionType{
		ParameterTypes:    []types.Type{}, // No fixed parameters
		ReturnType:        types.String,
		IsVariadic:        true,
		RestParameterType: &types.ArrayType{ElementType: types.Any}, // Accept any values
	})

	// Register static String methods
	register("String.fromCharCode", -1, true, stringFromCharCode, &types.FunctionType{
		ParameterTypes:    []types.Type{}, // No fixed parameters
		ReturnType:        types.String,
		IsVariadic:        true,
		RestParameterType: &types.ArrayType{ElementType: types.Number}, // Accept numbers only
	})
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

package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// registerString registers the String constructor and static methods
func registerString() {
	// Register String constructor
	register("String", -1, true, stringConstructor, &types.FunctionType{
		ParameterTypes: []types.Type{types.Any},
		ReturnType:     types.String,
		IsVariadic:     true,
	})

	// Register static String methods
	register("String.fromCharCode", -1, true, stringFromCharCode, &types.FunctionType{
		ParameterTypes: []types.Type{types.Number},
		ReturnType:     types.String,
		IsVariadic:     true,
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

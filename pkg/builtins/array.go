package builtins

import (
	"paserati/pkg/types"
)

// registerArray registers Array prototype methods with proper types
func registerArray() {
	// Register Array.prototype.push - variadic method
	register("Array.prototype.push", -1, true, nil, &types.FunctionType{
		ParameterTypes:    []types.Type{}, // No fixed parameters
		ReturnType:        types.Number,   // Returns new length
		IsVariadic:        true,
		RestParameterType: &types.ArrayType{ElementType: types.Any}, // Accept any values
	})

	// Register Array.prototype.concat - variadic method
	register("Array.prototype.concat", -1, true, nil, &types.FunctionType{
		ParameterTypes:    []types.Type{},                           // No fixed parameters
		ReturnType:        &types.ArrayType{ElementType: types.Any}, // Returns new array
		IsVariadic:        true,
		RestParameterType: &types.ArrayType{ElementType: types.Any}, // Accept any values
	})

	// Register Array.prototype.pop - non-variadic method
	register("Array.prototype.pop", 0, false, nil, &types.FunctionType{
		ParameterTypes: []types.Type{}, // No parameters
		ReturnType:     types.Any,      // Returns any (the popped element)
		IsVariadic:     false,
	})

	// Register Array.prototype.join - method with optional separator
	register("Array.prototype.join", 1, false, nil, &types.FunctionType{
		ParameterTypes: []types.Type{types.String}, // Optional separator parameter
		ReturnType:     types.String,               // Returns string
		IsVariadic:     false,
		OptionalParams: []bool{true}, // Separator is optional
	})
}

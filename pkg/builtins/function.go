package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// registerFunction registers Function prototype methods
func registerFunction() {
	registerFunctionPrototypeMethods()
}

// registerFunctionPrototypeMethods registers Function prototype methods with both implementations and types
func registerFunctionPrototypeMethods() {
	// Register call method
	vm.RegisterFunctionPrototypeMethod("call",
		vm.NewNativeFunction(-1, true, "call", functionPrototypeCallImpl))
	RegisterPrototypeMethod("function", "call",
		types.NewVariadicFunction([]types.Type{types.Any}, types.Any, &types.ArrayType{ElementType: types.Any}))
}

// functionPrototypeCallImpl implements Function.prototype.call()
// Syntax: func.call(thisArg, arg1, arg2, ...)
func functionPrototypeCallImpl(args []vm.Value) vm.Value {
	// args[0] is 'this' (the function being called)
	// args[1] is thisArg (the 'this' value for the function call)
	// args[2:] are the arguments to pass to the function
	
	if len(args) < 1 {
		return vm.Undefined // Error: no function provided
	}
	
	thisFunction := args[0]
	
	// Determine the 'this' value for the call
	var thisArg vm.Value
	if len(args) > 1 {
		thisArg = args[1]
	} else {
		thisArg = vm.Undefined
	}
	
	// Extract the arguments to pass to the function
	var callArgs []vm.Value
	if len(args) > 2 {
		callArgs = args[2:]
	} else {
		callArgs = []vm.Value{}
	}
	
	// Call the function with the specified 'this' value
	return callFunctionWithThis(thisFunction, thisArg, callArgs)
}

// callFunctionWithThis calls a function with a specific 'this' value
func callFunctionWithThis(function vm.Value, thisValue vm.Value, args []vm.Value) vm.Value {
	// For the prototype chain test to work, we need to handle constructor functions
	// This is a simplified implementation that handles the basic case
	
	if function.IsFunction() || function.IsClosure() {
		// For user-defined functions and closures, we can't easily call them
		// from within a builtin function with the current VM architecture.
		// This would require VM support for calling user functions from builtins.
		// For now, return undefined to avoid crashes.
		// TODO: Implement proper VM support for calling user functions from builtins
		return vm.Undefined
		
	} else if function.IsNativeFunction() {
		// For native functions, call with 'this' prepended
		nativeFunc := function.AsNativeFunction()
		
		// Prepend 'this' to arguments
		fullArgs := make([]vm.Value, len(args)+1)
		fullArgs[0] = thisValue
		copy(fullArgs[1:], args)
		
		return nativeFunc.Fn(fullArgs)
	}
	
	// If not a callable type, return undefined
	return vm.Undefined
}
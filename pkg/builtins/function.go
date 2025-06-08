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
	// For native functions, call with 'this' prepended
	if function.IsNativeFunction() {
		nativeFunc := function.AsNativeFunction()
		
		// Prepend 'this' to arguments
		fullArgs := make([]vm.Value, len(args)+1)
		fullArgs[0] = thisValue
		copy(fullArgs[1:], args)
		
		return nativeFunc.Fn(fullArgs)
	}
	
	// For user-defined functions and closures, we need VM support
	// Since built-ins can't directly invoke the VM's frame mechanism,
	// we'll need to handle this differently.
	// For now, return a special marker that indicates this needs VM handling
	if function.IsFunction() || function.IsClosure() {
		// TODO: This is a temporary workaround. The proper solution would be:
		// 1. Add a VM method for calling functions from built-ins
		// 2. Pass VM context to built-in functions that need it
		// 3. Use OpCallMethod mechanism from within built-ins
		return vm.Undefined
	}
	
	// If not a callable type, return undefined
	return vm.Undefined
}
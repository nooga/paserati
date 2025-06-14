package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// registerFunction registers Function prototype types (for type checker)
func registerFunction() {
	registerFunctionPrototypeMethods()
}

// registerFunctionPrototypeMethods registers Function prototype method types
func registerFunctionPrototypeMethods() {
	// Register type information for type checker
	RegisterPrototypeMethod("function", "call",
		types.NewVariadicFunction([]types.Type{}, types.Any, &types.ArrayType{ElementType: types.Any}))
	RegisterPrototypeMethod("function", "apply",
		types.NewSimpleFunction([]types.Type{types.Any, &types.ArrayType{ElementType: types.Any}}, types.Any))
}

// setupFunctionPrototype sets up Function prototype methods for a specific VM instance
// This is called via VM initialization callback to ensure VM isolation
func setupFunctionPrototype(vmInstance *vm.VM) {
	funcProto := vmInstance.FunctionPrototype.AsPlainObject()

	// Function.prototype.call - using closure pattern like array methods
	callImpl := func(args []vm.Value) vm.Value {
		return functionPrototypeCallWithVM(vmInstance, args)
	}
	funcProto.SetOwn("call", vm.NewNativeFunction(0, true, "call", callImpl))

	// Function.prototype.apply - using regular native function with VM isolation
	applyImpl := func(args []vm.Value) vm.Value {
		return functionPrototypeApply(vmInstance, args)
	}
	funcProto.SetOwn("apply", vm.NewNativeFunction(2, false, "apply", applyImpl))
}

// functionPrototypeCallAsync implements Function.prototype.call using AsyncNativeFunction
// This avoids the method binding recursion issue by handling the call directly
func functionPrototypeCallAsync(caller vm.VMCaller, args []vm.Value) vm.Value {
	// When called as a method (e.g., Vehicle.call), the bound method prepends the function as args[0]
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

	// For native functions, call directly without using the VM
	if thisFunction.IsNativeFunction() {
		nativeFunc := thisFunction.AsNativeFunction()

		// Prepend 'this' to arguments
		fullArgs := make([]vm.Value, len(callArgs)+1)
		fullArgs[0] = thisArg
		copy(fullArgs[1:], callArgs)

		return nativeFunc.Fn(fullArgs)
	}

	// For user-defined functions and closures, use the caller's CallBytecode method
	// This bypasses the normal method binding process and calls the function directly
	if thisFunction.IsFunction() || thisFunction.IsClosure() {
		return caller.CallBytecode(thisFunction, thisArg, callArgs)
	}

	// If not a callable type, return undefined
	return vm.Undefined
}

// functionPrototypeCallWithVM implements Function.prototype.call with VM access
// This follows the same pattern as array methods like map, filter, etc.
// Syntax: func.call(thisArg, arg1, arg2, ...)
func functionPrototypeCallWithVM(vmInstance *vm.VM, args []vm.Value) vm.Value {
	// args[0] is 'this' (the function being called) - this comes from the bound method
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

	// For native functions, call with 'this' prepended
	if thisFunction.IsNativeFunction() {
		nativeFunc := thisFunction.AsNativeFunction()

		// Prepend 'this' to arguments
		fullArgs := make([]vm.Value, len(callArgs)+1)
		fullArgs[0] = thisArg
		copy(fullArgs[1:], callArgs)

		return nativeFunc.Fn(fullArgs)
	}

	// For user-defined functions and closures, use VM's CallFunctionDirectWithoutMethodBinding
	// This avoids the method binding recursion that was causing infinite loops
	if thisFunction.IsFunction() || thisFunction.IsClosure() {
		result, err := vmInstance.CallFunctionDirectWithoutMethodBinding(thisFunction, thisArg, callArgs)
		if err != nil {
			// For now, return undefined on error
			// TODO: Throw proper error or propagate it properly
			return vm.Undefined
		}
		return result
	}

	// If not a callable type, return undefined
	return vm.Undefined
}

// functionPrototypeApply implements Function.prototype.apply with VM isolation
// Syntax: func.apply(thisArg, argsArray)
func functionPrototypeApply(vmInstance *vm.VM, args []vm.Value) vm.Value {
	// args[0] is 'this' (the function being called)
	// args[1] is thisArg (the 'this' value for the function call)
	// args[2] is argsArray (array of arguments to pass to the function)

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

	// Extract arguments array
	var callArgs []vm.Value
	if len(args) > 2 {
		argsArray := args[2]
		if argsArray.Type() == vm.TypeArray {
			arr := argsArray.AsArray()
			callArgs = make([]vm.Value, arr.Length())
			for i := 0; i < arr.Length(); i++ {
				callArgs[i] = arr.Get(i)
			}
		} else if argsArray.Type() != vm.TypeNull && argsArray.Type() != vm.TypeUndefined {
			// TypeError: second argument must be an array or null/undefined
			return vm.Undefined
		}
	} else {
		callArgs = []vm.Value{}
	}

	// Call the function with the specified 'this' value
	return callFunctionWithThis(vmInstance, thisFunction, thisArg, callArgs)
}

// callFunctionWithThis calls a function with a specific 'this' value using the provided VM instance
func callFunctionWithThis(vmInstance *vm.VM, function vm.Value, thisValue vm.Value, args []vm.Value) vm.Value {
	// For native functions, call with 'this' prepended
	if function.IsNativeFunction() {
		nativeFunc := function.AsNativeFunction()

		// Prepend 'this' to arguments
		fullArgs := make([]vm.Value, len(args)+1)
		fullArgs[0] = thisValue
		copy(fullArgs[1:], args)

		return nativeFunc.Fn(fullArgs)
	}

	// For user-defined functions and closures, use the VM's CallFunctionFromBuiltin
	// This method is designed to handle calling user functions from within builtins
	if function.IsFunction() || function.IsClosure() {
		result, err := vmInstance.CallFunctionFromBuiltin(function, thisValue, args)
		if err != nil {
			// For now, return undefined on error
			// TODO: Throw proper error or propagate it properly
			return vm.Undefined
		}
		return result
	}

	// If not a callable type, return undefined
	return vm.Undefined
}

package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"strings"
)

// FunctionInitializer implements the Function builtin
type FunctionInitializer struct{}

func (f *FunctionInitializer) Name() string {
	return "Function"
}

func (f *FunctionInitializer) Priority() int {
	return PriorityFunction // Must be after Object but before others
}

func (f *FunctionInitializer) InitTypes(ctx *TypeContext) error {
	// Create Function.prototype type using fluent API
	functionProtoType := types.NewObjectType().
		WithProperty("call", types.NewVariadicFunction([]types.Type{}, types.Any, &types.ArrayType{ElementType: types.Any})).
		WithProperty("apply", types.NewSimpleFunction([]types.Type{types.Any, &types.ArrayType{ElementType: types.Any}}, types.Any)).
		WithProperty("bind", types.NewVariadicFunction([]types.Type{types.Any}, types.Any, &types.ArrayType{ElementType: types.Any}))

	// Create Function constructor type using fluent API
	functionCtorType := types.NewObjectType().
		// Constructor is callable (Function constructor creates functions from strings)
		WithSimpleCallSignature([]types.Type{}, types.Any).                 // Function() -> function
		WithVariadicCallSignature([]types.Type{}, types.Any, types.String). // Function(...args, body) -> function
		// Static properties
		WithProperty("prototype", functionProtoType)

	// Define the constructor globally
	if err := ctx.DefineGlobal("Function", functionCtorType); err != nil {
		return err
	}

	// Store the prototype type for primitive "function"
	ctx.SetPrimitivePrototype("function", functionProtoType)

	return nil
}

func (f *FunctionInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Function.prototype inheriting from Object.prototype
	functionProto := vm.NewObject(ctx.ObjectPrototype).AsPlainObject()

	// Add prototype methods
	// Function.prototype.call
	callImpl := func(args []vm.Value) vm.Value {
		return functionPrototypeCallImpl(vmInstance, args)
	}
	functionProto.SetOwn("call", vm.NewNativeFunction(0, true, "call", callImpl))

	// Function.prototype.apply
	applyImpl := func(args []vm.Value) vm.Value {
		return functionPrototypeApplyImpl(vmInstance, args)
	}
	functionProto.SetOwn("apply", vm.NewNativeFunction(2, false, "apply", applyImpl))

	// Function.prototype.bind
	bindImpl := func(args []vm.Value) vm.Value {
		return functionPrototypeBindImpl(vmInstance, args)
	}
	functionProto.SetOwn("bind", vm.NewNativeFunction(0, true, "bind", bindImpl))

	// Create Function constructor
	functionCtor := vm.NewNativeFunction(-1, true, "Function", func(args []vm.Value) vm.Value {
		// TODO: Implement Function constructor (creates functions from strings)
		// For now, return a simple function
		return vm.NewNativeFunction(0, false, "anonymous", func([]vm.Value) vm.Value {
			return vm.Undefined
		})
	})

	// Make it a proper constructor with static methods
	if ctorObj := functionCtor.AsNativeFunction(); ctorObj != nil {
		// Convert to object with properties
		ctorWithProps := vm.NewNativeFunctionWithProps(ctorObj.Arity, ctorObj.Variadic, ctorObj.Name, ctorObj.Fn)
		ctorPropsObj := ctorWithProps.AsNativeFunctionWithProps()

		// Add prototype property
		ctorPropsObj.Properties.SetOwn("prototype", vm.NewValueFromPlainObject(functionProto))

		functionCtor = ctorWithProps
	}

	// Set constructor property on prototype
	functionProto.SetOwn("constructor", functionCtor)

	// Store in VM
	vmInstance.FunctionPrototype = vm.NewValueFromPlainObject(functionProto)

	// Define globally
	return ctx.DefineGlobal("Function", functionCtor)
}

// Implementation methods

func functionPrototypeCallImpl(vmInstance *vm.VM, args []vm.Value) vm.Value {
	// Get 'this' from VM instead of first argument
	thisValue := vmInstance.GetThis()

	if !thisValue.IsCallable() {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined
	}

	newThis := vm.Undefined
	var callArgs []vm.Value

	if len(args) > 0 {
		newThis = args[0]
		callArgs = args[1:]
	}

	// For user-defined functions, use direct call to avoid method binding recursion
	if thisValue.Type() == vm.TypeClosure || thisValue.Type() == vm.TypeFunction {
		result, err := vmInstance.CallFunctionDirectWithoutMethodBinding(thisValue, newThis, callArgs)
		if err != nil {
			// TODO: Proper error handling when error objects are implemented
			return vm.Undefined
		}
		return result
	} else {
		// For native functions, use the normal path
		result, err := vmInstance.CallFunctionFromBuiltin(thisValue, newThis, callArgs)
		if err != nil {
			// TODO: Proper error handling when error objects are implemented
			return vm.Undefined
		}
		return result
	}
}

func functionPrototypeApplyImpl(vmInstance *vm.VM, args []vm.Value) vm.Value {
	// Get 'this' from VM instead of first argument
	thisValue := vmInstance.GetThis()

	if !thisValue.IsCallable() {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined
	}

	newThis := vm.Undefined
	var callArgs []vm.Value

	if len(args) > 0 {
		newThis = args[0]
	}

	if len(args) > 1 {
		argsArray := args[1]
		if argsArray.IsArray() {
			arrayObj := argsArray.AsArray()
			length := arrayObj.Length()
			callArgs = make([]vm.Value, length)
			for i := 0; i < length; i++ {
				callArgs[i] = arrayObj.Get(i)
			}
		}
	}

	// For user-defined functions, use direct call to avoid method binding recursion
	if thisValue.Type() == vm.TypeClosure || thisValue.Type() == vm.TypeFunction {
		result, err := vmInstance.CallFunctionDirectWithoutMethodBinding(thisValue, newThis, callArgs)
		if err != nil {
			// TODO: Proper error handling when error objects are implemented
			return vm.Undefined
		}
		return result
	} else {
		// For native functions, use the normal path
		result, err := vmInstance.CallFunctionFromBuiltin(thisValue, newThis, callArgs)
		if err != nil {
			// TODO: Proper error handling when error objects are implemented
			return vm.Undefined
		}
		return result
	}
}

func functionPrototypeBindImpl(vmInstance *vm.VM, args []vm.Value) vm.Value {
	// Get 'this' from VM instead of first argument
	originalFunc := vmInstance.GetThis()

	if !originalFunc.IsCallable() {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined
	}

	// Check if we're trying to bind a bound function - prevent infinite recursion
	if originalFunc.Type() == vm.TypeNativeFunction {
		if nativeFunc := originalFunc.AsNativeFunction(); nativeFunc != nil {
			if strings.HasPrefix(nativeFunc.Name, "bound ") {
				// This is already a bound function - this should be allowed in JavaScript
				// but we'll continue with the binding anyway
			}
		}
	}

	var boundThis vm.Value = vm.Undefined
	var partialArgs []vm.Value

	if len(args) > 0 {
		boundThis = args[0]    // First argument is the 'this' value to bind to
		partialArgs = args[1:] // Remaining arguments are partial arguments
	}

	// Calculate new arity
	originalArity := originalFunc.GetArity()

	newArity := originalArity - len(partialArgs)
	if newArity < 0 {
		newArity = 0
	}

	// IMPORTANT: Capture the original function value at bind-time!
	// We cannot rely on GetThis() inside the bound function because
	// when the bound function is called, 'this' will be the bound function itself
	capturedOriginalFunc := originalFunc
	capturedBoundThis := boundThis
	capturedPartialArgs := make([]vm.Value, len(partialArgs))
	copy(capturedPartialArgs, partialArgs)

	// Create bound function
	boundFunction := func(additionalArgs []vm.Value) vm.Value {
		// Combine partial args with additional args
		finalArgs := make([]vm.Value, len(capturedPartialArgs)+len(additionalArgs))
		copy(finalArgs, capturedPartialArgs)
		copy(finalArgs[len(capturedPartialArgs):], additionalArgs)

		// For user-defined functions, we need to call them directly to avoid
		// method binding issues that cause infinite recursion
		if capturedOriginalFunc.Type() == vm.TypeClosure || capturedOriginalFunc.Type() == vm.TypeFunction {
			// Create a minimal call frame directly - bypass CallFunctionFromBuiltin to avoid recursion
			result, err := vmInstance.CallFunctionDirectWithoutMethodBinding(capturedOriginalFunc, capturedBoundThis, finalArgs)
			if err != nil {
				// TODO: Proper error handling when error objects are implemented
				return vm.Undefined
			}
			return result
		} else {
			// For native functions, use the normal path
			result, err := vmInstance.CallFunctionFromBuiltin(capturedOriginalFunc, capturedBoundThis, finalArgs)
			if err != nil {
				// TODO: Proper error handling when error objects are implemented
				return vm.Undefined
			}
			return result
		}
	}

	// Use a simple name to avoid infinite recursion during Inspect()
	functionName := "bound"
	if originalFunc.Type() == vm.TypeNativeFunction {
		if nativeFunc := originalFunc.AsNativeFunction(); nativeFunc != nil {
			functionName = "bound " + nativeFunc.Name
		}
	} else if originalFunc.Type() == vm.TypeNativeFunctionWithProps {
		if nativeFuncWithProps := originalFunc.AsNativeFunctionWithProps(); nativeFuncWithProps != nil {
			functionName = "bound " + nativeFuncWithProps.Name
		}
	}

	result := vm.NewNativeFunction(newArity, false, functionName, boundFunction)
	return result
}

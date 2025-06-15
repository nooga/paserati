package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
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
		WithSimpleCallSignature([]types.Type{}, types.Any). // Function() -> function
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

// Implementation methods (simplified versions of the existing ones)

func functionPrototypeCallImpl(vmInstance *vm.VM, args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Undefined
	}

	thisValue := args[0]
	if len(args) == 1 {
		// Only 'this' provided, no additional arguments
		if thisValue.IsCallable() {
			// Call with empty arguments
			result, err := vmInstance.CallFunctionFromBuiltin(thisValue, vm.Undefined, []vm.Value{})
			if err != nil {
				// TODO: Proper error handling when error objects are implemented
				return vm.Undefined
			}
			return result
		}
		return vm.Undefined
	}

	// Get the function to call (should be 'this' in the method call context)
	// args[0] = the function being called
	// args[1] = the 'this' value for the call
	// args[2:] = the arguments for the call
	if !thisValue.IsCallable() {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined
	}

	newThis := vm.Undefined
	var callArgs []vm.Value
	if len(args) > 1 {
		newThis = args[1]
		callArgs = args[2:]
	}

	result, err := vmInstance.CallFunctionFromBuiltin(thisValue, newThis, callArgs)
	if err != nil {
		// TODO: Proper error handling when error objects are implemented
		return vm.Undefined
	}
	return result
}

func functionPrototypeApplyImpl(vmInstance *vm.VM, args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Undefined
	}

	thisValue := args[0]
	if !thisValue.IsCallable() {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined
	}

	newThis := vm.Undefined
	var callArgs []vm.Value

	if len(args) > 1 {
		newThis = args[1]
	}

	if len(args) > 2 {
		argsArray := args[2]
		if argsArray.IsArray() {
			arrayObj := argsArray.AsArray()
			length := arrayObj.Length()
			callArgs = make([]vm.Value, length)
			for i := 0; i < length; i++ {
				callArgs[i] = arrayObj.Get(i)
			}
		}
	}

	result, err := vmInstance.CallFunctionFromBuiltin(thisValue, newThis, callArgs)
	if err != nil {
		// TODO: Proper error handling when error objects are implemented
		return vm.Undefined
	}
	return result
}

func functionPrototypeBindImpl(vmInstance *vm.VM, args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.Undefined
	}

	originalFunc := args[0]
	if !originalFunc.IsCallable() {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined
	}

	var boundThis vm.Value = vm.Undefined
	var partialArgs []vm.Value

	if len(args) > 1 {
		boundThis = args[1]
		partialArgs = args[2:]
	}

	// Create bound function
	boundFunction := func(additionalArgs []vm.Value) vm.Value {
		// Combine partial args with additional args
		finalArgs := make([]vm.Value, len(partialArgs)+len(additionalArgs))
		copy(finalArgs, partialArgs)
		copy(finalArgs[len(partialArgs):], additionalArgs)

		result, err := vmInstance.CallFunctionFromBuiltin(originalFunc, boundThis, finalArgs)
		if err != nil {
			// TODO: Proper error handling when error objects are implemented
			return vm.Undefined
		}
		return result
	}

	// Calculate new arity
	originalArity := 0
	if nativeFunc := originalFunc.AsNativeFunction(); nativeFunc != nil {
		originalArity = nativeFunc.Arity
	}

	newArity := originalArity - len(partialArgs)
	if newArity < 0 {
		newArity = 0
	}

	// Create the bound function with appropriate arity
	return vm.NewNativeFunction(newArity, true, "bound", boundFunction)
}
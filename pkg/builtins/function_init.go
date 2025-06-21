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

	// For now, just return undefined to break the infinite recursion
	// TODO: Re-implement call functionality without causing recursion
	return vm.Undefined
}

func functionPrototypeApplyImpl(vmInstance *vm.VM, args []vm.Value) vm.Value {
	// Get 'this' from VM instead of first argument
	thisValue := vmInstance.GetThis()

	if !thisValue.IsCallable() {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined
	}

	// For now, just return undefined to break the infinite recursion
	// TODO: Re-implement apply functionality without causing recursion
	return vm.Undefined
}

func functionPrototypeBindImpl(vmInstance *vm.VM, args []vm.Value) vm.Value {
	// Get 'this' from VM instead of first argument
	originalFunc := vmInstance.GetThis()

	if !originalFunc.IsCallable() {
		// TODO: Throw TypeError when error objects are implemented
		return vm.Undefined
	}

	var boundThis vm.Value = vm.Undefined
	var partialArgs []vm.Value

	if len(args) > 0 {
		boundThis = args[0]    // First argument is the 'this' value to bind to
		partialArgs = args[1:] // Remaining arguments are partial arguments
	}

	// Create function name for debugging
	functionName := "bound"
	switch originalFunc.Type() {
	case vm.TypeNativeFunction:
		if nativeFunc := originalFunc.AsNativeFunction(); nativeFunc != nil {
			functionName = "bound " + nativeFunc.Name
		}
	case vm.TypeNativeFunctionWithProps:
		if nativeFuncWithProps := originalFunc.AsNativeFunctionWithProps(); nativeFuncWithProps != nil {
			functionName = "bound " + nativeFuncWithProps.Name
		}
	case vm.TypeFunction:
		if fn := originalFunc.AsFunction(); fn != nil {
			functionName = "bound " + fn.Name
		}
	case vm.TypeClosure:
		if closure := originalFunc.AsClosure(); closure != nil && closure.Fn != nil {
			functionName = "bound " + closure.Fn.Name
		}
	case vm.TypeBoundFunction:
		if boundFn := originalFunc.AsBoundFunction(); boundFn != nil {
			functionName = "bound " + boundFn.Name
		}
	}

	// Create a bound function using the new BoundFunction type
	result := vm.NewBoundFunction(originalFunc, boundThis, partialArgs, functionName)
	return result
}

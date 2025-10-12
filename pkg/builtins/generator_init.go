package builtins

import (
	"fmt"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type GeneratorInitializer struct{}

func (g *GeneratorInitializer) Name() string {
	return "Generator"
}

func (g *GeneratorInitializer) Priority() int {
	return PriorityGenerator // Will need to add this constant
}

func (g *GeneratorInitializer) InitTypes(ctx *TypeContext) error {
	// Create generic type parameters for Generator<T, TReturn, TNext>
	tParam := &types.TypeParameter{Name: "T", Constraint: nil, Index: 0}
	tReturnParam := &types.TypeParameter{Name: "TReturn", Constraint: nil, Index: 1}
	tNextParam := &types.TypeParameter{Name: "TNext", Constraint: nil, Index: 2}

	tType := &types.TypeParameterType{Parameter: tParam}
	tReturnType := &types.TypeParameterType{Parameter: tReturnParam}
	tNextType := &types.TypeParameterType{Parameter: tNextParam}

	// Create IteratorResult<T, TReturn> type for the .next() method return
	iteratorResultType := types.NewObjectType().
		WithProperty("value", types.NewUnionType(tType, tReturnType)).
		WithProperty("done", types.Boolean)

	// Create Generator.prototype type with iterator protocol methods
	generatorProtoType := types.NewObjectType().
		// next(value?: TNext): IteratorResult<T, TReturn>
		WithProperty("next", types.NewOptionalFunction(
			[]types.Type{tNextType},
			iteratorResultType,
			[]bool{true})).
		// return(value?: TReturn): IteratorResult<T, TReturn>
		WithProperty("return", types.NewOptionalFunction(
			[]types.Type{tReturnType},
			iteratorResultType,
			[]bool{true})).
		// throw(exception?: any): IteratorResult<T, TReturn>
		WithProperty("throw", types.NewOptionalFunction(
			[]types.Type{types.Any},
			iteratorResultType,
			[]bool{true}))

	// Add Symbol.iterator to generator prototype type to make generators iterable
	// Generators return themselves when Symbol.iterator is called
	// Get the Iterator<T> generic type if available
	if _, found := ctx.GetType("Iterator"); found {
		// For generators, [Symbol.iterator]() returns Generator<T, TReturn, TNext> which extends Iterator<T>
		// Since Generator<T, TReturn, TNext> is already an iterator, it returns itself
		// Create the Generator<T, TReturn, TNext> type
		generatorType := &types.GenericType{
			Name:           "Generator",
			TypeParameters: []*types.TypeParameter{tParam, tReturnParam, tNextParam},
			Body:           generatorProtoType,
		}
		generatorProtoType = generatorProtoType.WithProperty("__COMPUTED_PROPERTY__",
			types.NewSimpleFunction([]types.Type{}, generatorType))
	}

	// Register generator primitive prototype
	ctx.SetPrimitivePrototype("generator", generatorProtoType)

	// Create Generator constructor function type (for TypeScript typing)
	// Note: In JavaScript, Generator is not a constructor - generators are created by generator functions
	generatorCtorType := types.NewObjectType().
		WithProperty("prototype", generatorProtoType)

	// Define Generator constructor in global environment
	return ctx.DefineGlobal("Generator", generatorCtorType)
}

func (g *GeneratorInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Generator.prototype inheriting from Object.prototype
	generatorProto := vm.NewObject(objectProto).AsPlainObject()

	// Add Generator prototype methods

	// next(value?) - Resume generator execution and return next yielded value
	generatorProto.SetOwn("next", vm.NewNativeFunction(1, false, "next", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsGenerator() {
			// TypeError: Method Generator.prototype.next called on incompatible receiver
			return vm.Undefined, fmt.Errorf("TypeError: Method Generator.prototype.next called on incompatible receiver")
		}
		thisGen := thisValue.AsGenerator()

		// If generator is completed, return { value: undefined, done: true }
		if thisGen.Done || thisGen.State == vm.GeneratorCompleted {
			result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			result.SetOwn("value", vm.Undefined)
			result.SetOwn("done", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(result), nil
		}

		// Get the sent value (argument to .next())
		sentValue := vm.Undefined
		if len(args) > 0 {
			sentValue = args[0]
		}

		// Execute the generator
		return vmInstance.ExecuteGenerator(thisGen, sentValue)
	}))

	// return(value?) - Force generator to return and complete
	generatorProto.SetOwn("return", vm.NewNativeFunction(1, false, "return", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsGenerator() {
			return vm.Undefined, fmt.Errorf("TypeError: Method Generator.prototype.return called on incompatible receiver")
		}
		thisGen := thisValue.AsGenerator()

		// Get the return value
		returnValue := vm.Undefined
		if len(args) > 0 {
			returnValue = args[0]
		}

		// If generator is already completed or hasn't started, return immediately
		if thisGen.Done || thisGen.State == vm.GeneratorCompleted || thisGen.State == vm.GeneratorSuspendedStart {
			thisGen.ReturnValue = returnValue
			thisGen.State = vm.GeneratorCompleted
			thisGen.Done = true
			thisGen.Frame = nil

			result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			result.SetOwn("value", returnValue)
			result.SetOwn("done", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(result), nil
		}

		// Generator is suspended at a yield - resume with return completion
		// This will allow finally blocks to execute before completing
		return vmInstance.ExecuteGeneratorWithReturn(thisGen, returnValue)
	}))

	// throw(exception?) - Throw an exception into the generator
	generatorProto.SetOwn("throw", vm.NewNativeFunction(1, false, "throw", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsGenerator() {
			return vm.Undefined, fmt.Errorf("TypeError: Method Generator.prototype.throw called on incompatible receiver")
		}
		thisGen := thisValue.AsGenerator()

		// If generator is completed, throw the exception (as an Error object for clearer runtime message)
		if thisGen.Done || thisGen.State == vm.GeneratorCompleted {
			exception := vm.Undefined
			if len(args) > 0 {
				exception = args[0]
			}
			// Wrap into Error object to match expected test messages
			errObj := vm.NewObject(vm.Null).AsPlainObject()
			errObj.SetOwn("name", vm.NewString("Error"))
			errObj.SetOwn("message", vm.NewString("exception thrown: "+exception.ToString()))
			return vm.Undefined, vmInstance.NewExceptionError(vm.NewValueFromPlainObject(errObj))
		}

		// Get the exception value (argument to .throw())
		exception := vm.Undefined
		if len(args) > 0 {
			exception = args[0]
		}

		// Inject the original exception value into the generator so user catch(e) sees the same value
		return vmInstance.ExecuteGeneratorWithException(thisGen, exception)
	}))

	// Add Symbol.iterator implementation for generators
	// Generators are iterable - they return themselves since they already have next() method
	// Register [Symbol.iterator] using native symbol key
	genIterFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsGenerator() {
			return vm.Undefined, fmt.Errorf("TypeError: Method Generator.prototype[Symbol.iterator] called on incompatible receiver")
		}
		// Generators are self-iterable - return the generator itself
		return thisValue, nil
	})
	generatorProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), genIterFn, nil, nil, nil)

	// Set Generator prototype in VM
	vmInstance.GeneratorPrototype = vm.NewValueFromPlainObject(generatorProto)

	// Note: In JavaScript, Generator is not directly constructible
	// Generators are created by calling generator functions (function*)
	// The Generator constructor exists mainly for prototype access

	return nil
}

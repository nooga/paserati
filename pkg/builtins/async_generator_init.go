package builtins

import (
	"fmt"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type AsyncGeneratorInitializer struct{}

func (g *AsyncGeneratorInitializer) Name() string {
	return "AsyncGenerator"
}

func (g *AsyncGeneratorInitializer) Priority() int {
	return PriorityAsyncGenerator
}

func (g *AsyncGeneratorInitializer) InitTypes(ctx *TypeContext) error {
	// Simplified type for AsyncGenerator - just define basic structure
	// Methods return promises, but we'll keep type system simple for now
	asyncGeneratorProtoType := types.NewObjectType().
		WithProperty("next", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("return", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("throw", types.NewSimpleFunction([]types.Type{types.Any}, types.Any))

	ctx.SetPrimitivePrototype("asyncgenerator", asyncGeneratorProtoType)

	asyncGeneratorCtorType := types.NewObjectType().
		WithProperty("prototype", asyncGeneratorProtoType)

	return ctx.DefineGlobal("AsyncGenerator", asyncGeneratorCtorType)
}

func (g *AsyncGeneratorInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	objectProto := vmInstance.ObjectPrototype
	asyncGeneratorProto := vm.NewObject(objectProto).AsPlainObject()

	// next(value?) - Returns Promise that resolves to next yielded value
	asyncGeneratorProto.SetOwn("next", vm.NewNativeFunction(1, false, "next", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if thisValue.Type() != vm.TypeAsyncGenerator {
			return vm.Undefined, fmt.Errorf("TypeError: Method AsyncGenerator.prototype.next called on incompatible receiver")
		}
		thisGen := thisValue.AsAsyncGenerator()

		// If generator is completed, return resolved promise with { value: undefined, done: true }
		if thisGen.Done || thisGen.State == vm.GeneratorCompleted {
			result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			result.SetOwn("value", vm.Undefined)
			result.SetOwn("done", vm.BooleanValue(true))
			resultVal := vm.NewValueFromPlainObject(result)
			return vmInstance.NewResolvedPromise(resultVal), nil
		}

		// Get the sent value (argument to .next())
		sentValue := vm.Undefined
		if len(args) > 0 {
			sentValue = args[0]
		}

		// For now, treat AsyncGenerator like a regular Generator
		// ExecuteGenerator works with GeneratorObject, so we need to cast
		// This is a simplification - proper implementation would need separate ExecuteAsyncGenerator
		genObj := &vm.GeneratorObject{
			Function:     thisGen.Function,
			State:        thisGen.State,
			Frame:        thisGen.Frame,
			YieldedValue: thisGen.YieldedValue,
			ReturnValue:  thisGen.ReturnValue,
			Done:         thisGen.Done,
			Args:         thisGen.Args,
		}

		result, err := vmInstance.ExecuteGenerator(genObj, sentValue)

		// Sync back the state
		thisGen.State = genObj.State
		thisGen.Frame = genObj.Frame
		thisGen.YieldedValue = genObj.YieldedValue
		thisGen.ReturnValue = genObj.ReturnValue
		thisGen.Done = genObj.Done

		if err != nil {
			return vmInstance.NewRejectedPromise(vm.NewString(err.Error())), nil
		}

		return vmInstance.NewResolvedPromise(result), nil
	}))

	// return(value?) - Returns Promise that resolves to force generator completion
	asyncGeneratorProto.SetOwn("return", vm.NewNativeFunction(1, false, "return", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if thisValue.Type() != vm.TypeAsyncGenerator {
			return vm.Undefined, fmt.Errorf("TypeError: Method AsyncGenerator.prototype.return called on incompatible receiver")
		}
		thisGen := thisValue.AsAsyncGenerator()

		returnValue := vm.Undefined
		if len(args) > 0 {
			returnValue = args[0]
		}
		thisGen.ReturnValue = returnValue
		thisGen.State = vm.GeneratorCompleted
		thisGen.Done = true
		thisGen.Frame = nil

		// Return a promise that resolves to { value: returnValue, done: true }
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		result.SetOwn("value", returnValue)
		result.SetOwn("done", vm.BooleanValue(true))
		resultVal := vm.NewValueFromPlainObject(result)

		return vmInstance.NewResolvedPromise(resultVal), nil
	}))

	// throw(exception?) - Returns Promise that may reject based on generator handling
	asyncGeneratorProto.SetOwn("throw", vm.NewNativeFunction(1, false, "throw", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if thisValue.Type() != vm.TypeAsyncGenerator {
			return vm.Undefined, fmt.Errorf("TypeError: Method AsyncGenerator.prototype.throw called on incompatible receiver")
		}
		thisGen := thisValue.AsAsyncGenerator()

		exception := vm.Undefined
		if len(args) > 0 {
			exception = args[0]
		}

		// If generator is completed, return rejected promise
		if thisGen.Done || thisGen.State == vm.GeneratorCompleted {
			return vmInstance.NewRejectedPromise(exception), nil
		}

		// For now, just reject - proper implementation would throw into the generator
		return vmInstance.NewRejectedPromise(exception), nil
	}))

	// Add Symbol.asyncIterator - async generators are their own async iterators
	// Set asyncGeneratorProto[Symbol.asyncIterator] = function() { return this; }
	asyncIteratorMethod := vm.NewNativeFunction(0, false, "[Symbol.asyncIterator]", func(args []vm.Value) (vm.Value, error) {
		return vmInstance.GetThis(), nil
	})
	// Use DefineOwnPropertyByKey with symbol key (like generators do with Symbol.iterator)
	asyncGeneratorProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolAsyncIterator), asyncIteratorMethod, nil, nil, nil)

	vmInstance.AsyncGeneratorPrototype = vm.NewValueFromPlainObject(asyncGeneratorProto)

	return nil
}

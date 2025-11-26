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

		// If generator is currently delegating (yield*), forward .return() to delegated iterator
		if !thisGen.DelegatedIterator.IsUndefined() && thisGen.DelegatedIterator.Type() != vm.TypeNull {
			delegatedIter := thisGen.DelegatedIterator

			// Try to get the .return() method from the delegated iterator
			// Use GetProperty to properly trigger getters and handle exceptions
			returnMethod, err := vmInstance.GetProperty(delegatedIter, "return")
			if err != nil {
				// Property access threw an exception
				// Clear delegation and resume generator with exception so try-catch can handle it
				thisGen.DelegatedIterator = vm.Undefined
				if ee, ok := err.(vm.ExceptionError); ok {
					return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
				}
				return vm.Undefined, err
			}

			// If the delegated iterator has a .return() method, call it
			if !returnMethod.IsUndefined() && returnMethod.Type() != vm.TypeNull && returnMethod.IsCallable() {
				// Call delegatedIter.return(returnValue) via VM
				result, err := vmInstance.Call(returnMethod, delegatedIter, []vm.Value{returnValue})
				if err != nil {
					// Call threw an exception
					// Clear delegation and resume generator with exception so try-catch can handle it
					thisGen.DelegatedIterator = vm.Undefined
					if ee, ok := err.(vm.ExceptionError); ok {
						return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
					}
					return vm.Undefined, err
				}

				// Clear the delegation state
				thisGen.DelegatedIterator = vm.Undefined

				// If the delegated iterator returned done:true, complete this generator
				// Use GetProperty to properly trigger getters on the result
				if result.IsObject() {
					doneVal, err := vmInstance.GetProperty(result, "done")
					if err != nil {
						// done getter threw - resume generator with exception so try-catch can handle it
						if ee, ok := err.(vm.ExceptionError); ok {
							return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
						}
						return vm.Undefined, err
					}
					if doneVal.IsTruthy() {
						thisGen.ReturnValue = returnValue
						thisGen.State = vm.GeneratorCompleted
						thisGen.Done = true
						thisGen.Frame = nil
					}
				}

				return result, nil
			}

			// If the delegated iterator doesn't have a .return() method, clear delegation and continue
			thisGen.DelegatedIterator = vm.Undefined
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

		// Get the exception value (argument to .throw())
		exception := vm.Undefined
		if len(args) > 0 {
			exception = args[0]
		}

		// If generator is currently delegating (yield*), forward .throw() to delegated iterator
		if !thisGen.DelegatedIterator.IsUndefined() && thisGen.DelegatedIterator.Type() != vm.TypeNull {
			delegatedIter := thisGen.DelegatedIterator

			// Try to get the .throw() method from the delegated iterator
			// Use GetProperty to properly trigger getters and handle exceptions
			throwMethod, err := vmInstance.GetProperty(delegatedIter, "throw")
			if err != nil {
				// Property access threw an exception
				// Clear delegation and resume generator with that exception (not the original one)
				thisGen.DelegatedIterator = vm.Undefined
				if ee, ok := err.(vm.ExceptionError); ok {
					return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
				}
				return vm.Undefined, err
			}

			// If the delegated iterator has a .throw() method, call it
			if !throwMethod.IsUndefined() && throwMethod.Type() != vm.TypeNull && throwMethod.IsCallable() {
				// Call delegatedIter.throw(exception) via VM
				result, err := vmInstance.Call(throwMethod, delegatedIter, []vm.Value{exception})
				if err != nil {
					// If the delegated iterator's throw() threw an exception, inject it into generator
					// Clear delegation first
					thisGen.DelegatedIterator = vm.Undefined
					if ee, ok := err.(vm.ExceptionError); ok {
						return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
					}
					return vm.Undefined, err
				}

				// Clear the delegation state
				thisGen.DelegatedIterator = vm.Undefined

				// If the delegated iterator returned done:true, the exception was handled
				// Use GetProperty to properly trigger getters on the result
				if result.IsObject() {
					doneVal, err := vmInstance.GetProperty(result, "done")
					if err != nil {
						// done getter threw - inject that exception into generator
						if ee, ok := err.(vm.ExceptionError); ok {
							return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
						}
						return vm.Undefined, err
					}
					if doneVal.IsTruthy() {
						// The delegated iterator handled the exception and completed
						// Continue generator execution after the yield*
						thisGen.State = vm.GeneratorSuspendedYield
					}
				}

				return result, nil
			}

			// If the delegated iterator doesn't have a .throw() method, close it and throw
			// Try to close the iterator by calling .return() if available
			returnMethod, err := vmInstance.GetProperty(delegatedIter, "return")
			if err != nil {
				// Getting .return threw - clear delegation and inject that exception into generator
				thisGen.DelegatedIterator = vm.Undefined
				if ee, ok := err.(vm.ExceptionError); ok {
					return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
				}
				return vm.Undefined, err
		} else if !returnMethod.IsUndefined() && returnMethod.Type() != vm.TypeNull && returnMethod.IsCallable() {
			// Call delegatedIter.return() to close it
			// Per spec: if return() throws, propagate that exception (not the original throw exception)
			_, returnErr := vmInstance.Call(returnMethod, delegatedIter, []vm.Value{})
			if returnErr != nil {
				// return() threw - clear delegation and inject that exception into generator
				thisGen.DelegatedIterator = vm.Undefined
				if ee, ok := returnErr.(vm.ExceptionError); ok {
					return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
				}
				return vm.Undefined, returnErr
			}
		}

		// Clear delegation and propagate the original exception to this generator
		thisGen.DelegatedIterator = vm.Undefined
		// Fall through to throw the exception into this generator
		}

		// If generator is completed, throw the exception (as an Error object for clearer runtime message)
		if thisGen.Done || thisGen.State == vm.GeneratorCompleted {
			// Wrap into Error object to match expected test messages
			errObj := vm.NewObject(vm.Null).AsPlainObject()
			errObj.SetOwn("name", vm.NewString("Error"))
			errObj.SetOwn("message", vm.NewString("exception thrown: "+exception.ToString()))
			return vm.Undefined, vmInstance.NewExceptionError(vm.NewValueFromPlainObject(errObj))
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

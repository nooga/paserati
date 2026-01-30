package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
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
	generatorProto.SetOwnNonEnumerable("next", vm.NewNativeFunction(1, false, "next", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsGenerator() {
			// TypeError: Method Generator.prototype.next called on incompatible receiver
			return vm.Undefined, vmInstance.NewTypeError("Method Generator.prototype.next called on incompatible receiver")
		}
		thisGen := thisValue.AsGenerator()

		// If generator is completed, return { value: undefined, done: true }
		if thisGen.Done || thisGen.State == vm.GeneratorCompleted {
			result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			result.SetOwnNonEnumerable("value", vm.Undefined)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
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
	generatorProto.SetOwnNonEnumerable("return", vm.NewNativeFunction(1, false, "return", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsGenerator() {
			return vm.Undefined, vmInstance.NewTypeError("Method Generator.prototype.return called on incompatible receiver")
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

				// ECMAScript spec step v: If Type(innerReturnResult) is not Object, throw TypeError
				if !result.IsObject() {
					thisGen.DelegatedIterator = vm.Undefined
					typeErr := vmInstance.NewTypeError("Iterator result is not an object")
					if ee, ok := typeErr.(vm.ExceptionError); ok {
						return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
					}
					return vm.Undefined, typeErr
				}

				// Get done value using GetProperty to properly trigger getters
				doneVal, err := vmInstance.GetProperty(result, "done")
				if err != nil {
					// done getter threw - resume generator with exception so try-catch can handle it
					thisGen.DelegatedIterator = vm.Undefined
					if ee, ok := err.(vm.ExceptionError); ok {
						return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
					}
					return vm.Undefined, err
				}

				if doneVal.IsTruthy() {
					// ECMAScript spec step vii: done is true
					// Get the value from the inner result
					innerValue, err := vmInstance.GetProperty(result, "value")
					if err != nil {
						thisGen.DelegatedIterator = vm.Undefined
						if ee, ok := err.(vm.ExceptionError); ok {
							return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
						}
						return vm.Undefined, err
					}

					// Clear delegation state
					thisGen.DelegatedIterator = vm.Undefined

					// Resume generator with return completion to run finally blocks
					// The generator will complete after finally blocks execute
					return vmInstance.ExecuteGeneratorWithReturn(thisGen, innerValue)
				}

				// ECMAScript spec step viii: done is false
				// GeneratorYield(innerReturnResult) - yield and wait for next received value
				// Don't clear DelegatedIterator - delegation continues
				// The next call to return() will forward again
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
			result.SetOwnNonEnumerable("value", returnValue)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(result), nil
		}

		// Generator is suspended at a yield - resume with return completion
		// This will allow finally blocks to execute before completing
		return vmInstance.ExecuteGeneratorWithReturn(thisGen, returnValue)
	}))

	// throw(exception?) - Throw an exception into the generator
	generatorProto.SetOwnNonEnumerable("throw", vm.NewNativeFunction(1, false, "throw", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsGenerator() {
			return vm.Undefined, vmInstance.NewTypeError("Method Generator.prototype.throw called on incompatible receiver")
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

				// ECMAScript spec step ii.2: If Type(innerResult) is not Object, throw TypeError
				if !result.IsObject() {
					thisGen.DelegatedIterator = vm.Undefined
					typeErr := vmInstance.NewTypeError("Iterator result is not an object")
					if ee, ok := typeErr.(vm.ExceptionError); ok {
						return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
					}
					return vm.Undefined, typeErr
				}

				// Get done value using GetProperty to properly trigger getters
				doneVal, err := vmInstance.GetProperty(result, "done")
				if err != nil {
					// done getter threw - inject that exception into generator
					thisGen.DelegatedIterator = vm.Undefined
					if ee, ok := err.(vm.ExceptionError); ok {
						return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
					}
					return vm.Undefined, err
				}

				if doneVal.IsTruthy() {
					// ECMAScript spec step ii.4: done is true
					// Get the value from the inner result (step ii.4.a: IteratorValue)
					innerValue, err := vmInstance.GetProperty(result, "value")
					if err != nil {
						thisGen.DelegatedIterator = vm.Undefined
						if ee, ok := err.(vm.ExceptionError); ok {
							return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
						}
						return vm.Undefined, err
					}

					// Clear delegation and set the result value
					// The VM will use DelegationResult to skip the yield* loop and continue
					thisGen.DelegatedIterator = vm.Undefined
					thisGen.DelegationResult = innerValue
					thisGen.DelegationResultReady = true

					// Resume generator - it will see DelegationResultReady and continue past yield*
					return vmInstance.ExecuteGenerator(thisGen, vm.Undefined)
				}

				// ECMAScript spec step ii.5/6: done is false, GeneratorYield(innerResult)
				// The delegation continues - don't clear DelegatedIterator
				// Just return the result and let the caller call next/throw/return again
				return result, nil
			}

			// If the delegated iterator doesn't have a .throw() method, close it and throw TypeError
			// Per ECMAScript spec step iii: close iterator then throw TypeError
			returnMethod, err := vmInstance.GetProperty(delegatedIter, "return")
			if err != nil {
				// Getting .return threw - clear delegation and inject that exception into generator
				thisGen.DelegatedIterator = vm.Undefined
				if ee, ok := err.(vm.ExceptionError); ok {
					return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
				}
				return vm.Undefined, err
			}
			if !returnMethod.IsUndefined() && returnMethod.Type() != vm.TypeNull && returnMethod.IsCallable() {
				// Call delegatedIter.return() to close it
				// Per spec: if return() throws, propagate that exception (not the TypeError)
				innerResult, returnErr := vmInstance.Call(returnMethod, delegatedIter, []vm.Value{})
				if returnErr != nil {
					// return() threw - clear delegation and inject that exception into generator
					thisGen.DelegatedIterator = vm.Undefined
					if ee, ok := returnErr.(vm.ExceptionError); ok {
						return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
					}
					return vm.Undefined, returnErr
				}
				// Check if return() result is an object (spec step iii.3/4)
				if !innerResult.IsUndefined() && !innerResult.IsObject() {
					thisGen.DelegatedIterator = vm.Undefined
					// Inject TypeError into generator so it can be caught by try-catch
					typeErr := vmInstance.NewTypeError("Iterator result is not an object")
					if ee, ok := typeErr.(vm.ExceptionError); ok {
						return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
					}
					return vm.Undefined, typeErr
				}
			}

			// ECMAScript spec step iii.6: Throw a TypeError exception
			// (iterator doesn't have a throw method - protocol violation)
			// Inject into generator so it can be caught by try-catch
			thisGen.DelegatedIterator = vm.Undefined
			typeErr := vmInstance.NewTypeError("Iterator does not have a throw method")
			if ee, ok := typeErr.(vm.ExceptionError); ok {
				return vmInstance.ExecuteGeneratorWithException(thisGen, ee.GetExceptionValue())
			}
			return vm.Undefined, typeErr
		}

		// If generator is completed, throw the exception (as an Error object for clearer runtime message)
		if thisGen.Done || thisGen.State == vm.GeneratorCompleted {
			// Wrap into Error object to match expected test messages
			errObj := vm.NewObject(vm.Null).AsPlainObject()
			errObj.SetOwnNonEnumerable("name", vm.NewString("Error"))
			errObj.SetOwnNonEnumerable("message", vm.NewString("exception thrown: "+exception.ToString()))
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
			return vm.Undefined, vmInstance.NewTypeError("Method Generator.prototype[Symbol.iterator] called on incompatible receiver")
		}
		// Generators are self-iterable - return the generator itself
		return thisValue, nil
	})
	generatorProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), genIterFn, nil, nil, nil)

	// Add Symbol.toStringTag = "Generator" per ECMAScript spec
	falseVal := false
	trueVal := true
	generatorProto.DefineOwnPropertyByKey(
		vm.NewSymbolKey(SymbolToStringTag),
		vm.NewString("Generator"),
		&falseVal, // writable: false
		&falseVal, // enumerable: false
		&trueVal,  // configurable: true
	)

	// Set Generator prototype in VM
	vmInstance.GeneratorPrototype = vm.NewValueFromPlainObject(generatorProto)

	// Create GeneratorFunction.prototype (%GeneratorFunction.prototype%)
	// This is the [[Prototype]] of all generator functions (function*)
	// It inherits from Function.prototype and has a .prototype property pointing to GeneratorPrototype
	generatorFunctionProto := vm.NewObject(vmInstance.FunctionPrototype).AsPlainObject()

	// Set the .prototype property to GeneratorPrototype
	// Per ECMAScript: GeneratorFunction.prototype.prototype === Generator.prototype
	w, e, c := false, false, false // writable=false, enumerable=false, configurable=false
	generatorFunctionProto.DefineOwnProperty("prototype", vmInstance.GeneratorPrototype, &w, &e, &c)

	// Set constructor property (pointing to GeneratorFunction constructor)
	// Note: GeneratorFunction is not a global but is accessible via (function*(){}).constructor
	// We'll set this up later if needed, for now just mark it as not enumerable

	// Store in VM
	vmInstance.GeneratorFunctionPrototype = vm.NewValueFromPlainObject(generatorFunctionProto)

	// Note: In JavaScript, Generator is not directly constructible
	// Generators are created by calling generator functions (function*)
	// The Generator constructor exists mainly for prototype access

	return nil
}

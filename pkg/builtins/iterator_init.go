package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type IteratorInitializer struct{}

func (i *IteratorInitializer) Name() string {
	return "Iterator"
}

func (i *IteratorInitializer) Priority() int {
	return PriorityIterator
}

func (i *IteratorInitializer) InitTypes(ctx *TypeContext) error {
	// Create generic type parameter T for iterator types
	tParam := &types.TypeParameter{Name: "T", Constraint: nil, Index: 0}
	tType := &types.TypeParameterType{Parameter: tParam}

	// Create IteratorResult<T> interface
	// interface IteratorResult<T> { value: T; done: boolean; }
	iteratorResultType := types.NewObjectType().
		WithProperty("value", tType).
		WithProperty("done", types.Boolean)

	// Create generic IteratorResult type
	iteratorResultGeneric := &types.GenericType{
		Name:           "IteratorResult",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           iteratorResultType,
	}

	// Create Iterator<T> interface - first create the generic, then add self-referential [Symbol.iterator]
	// interface Iterator<T> { next(): IteratorResult<T>; [Symbol.iterator](): Iterator<T>; ... helper methods }
	iteratorGeneric := &types.GenericType{
		Name:           "Iterator",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           nil, // Will be set below
	}

	// Create type parameter U for methods like map and flatMap
	uParam := &types.TypeParameter{Name: "U", Constraint: nil, Index: 1}
	uType := &types.TypeParameterType{Parameter: uParam}

	// Create callback types for iterator methods
	// (value: T) => boolean - for filter, some, every, find
	predicateType := types.NewSimpleFunction([]types.Type{tType}, types.Boolean)
	// (value: T) => U - for map
	mapperType := types.NewSimpleFunction([]types.Type{tType}, uType)
	// (value: T) => void - for forEach
	forEachCallbackType := types.NewSimpleFunction([]types.Type{tType}, types.Undefined)

	// Create the iterator type with all methods
	iteratorType := types.NewObjectType().
		// next(): IteratorResult<T>
		WithProperty("next", types.NewSimpleFunction([]types.Type{},
			&types.InstantiatedType{
				Generic:       iteratorResultGeneric,
				TypeArguments: []types.Type{tType},
			})).
		// [Symbol.iterator](): Iterator<T> (self-referential)
		WithProperty("__COMPUTED_PROPERTY__", types.NewSimpleFunction([]types.Type{},
			&types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{tType},
			})).
		// map<U>(mapper: (value: T) => U): Iterator<U>
		WithProperty("map", &types.GenericType{
			Name:           "map",
			TypeParameters: []*types.TypeParameter{uParam},
			Body: types.NewSimpleFunction([]types.Type{mapperType},
				&types.InstantiatedType{
					Generic:       iteratorGeneric,
					TypeArguments: []types.Type{uType},
				}),
		}).
		// filter(predicate: (value: T) => boolean): Iterator<T>
		WithProperty("filter", types.NewSimpleFunction([]types.Type{predicateType},
			&types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{tType},
			})).
		// take(limit: number): Iterator<T>
		WithProperty("take", types.NewSimpleFunction([]types.Type{types.Number},
			&types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{tType},
			})).
		// drop(limit: number): Iterator<T>
		WithProperty("drop", types.NewSimpleFunction([]types.Type{types.Number},
			&types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{tType},
			})).
		// toArray(): T[]
		WithProperty("toArray", types.NewSimpleFunction([]types.Type{},
			&types.ArrayType{ElementType: tType})).
		// forEach(fn: (value: T) => void): void
		WithProperty("forEach", types.NewSimpleFunction([]types.Type{forEachCallbackType},
			types.Undefined)).
		// reduce(reducer: (acc: any, value: T) => any, initialValue?: any): any
		// Note: reduce is complex with overloads, using any for simplicity
		WithProperty("reduce", types.NewOptionalFunction(
			[]types.Type{
				types.NewSimpleFunction([]types.Type{types.Any, tType}, types.Any),
				types.Any,
			},
			types.Any,
			[]bool{false, true})).
		// some(predicate: (value: T) => boolean): boolean
		WithProperty("some", types.NewSimpleFunction([]types.Type{predicateType}, types.Boolean)).
		// every(predicate: (value: T) => boolean): boolean
		WithProperty("every", types.NewSimpleFunction([]types.Type{predicateType}, types.Boolean)).
		// find(predicate: (value: T) => boolean): T | undefined
		WithProperty("find", types.NewSimpleFunction([]types.Type{predicateType},
			types.NewUnionType(tType, types.Undefined))).
		// flatMap<U>(mapper: (value: T) => Iterable<U>): Iterator<U>
		WithProperty("flatMap", &types.GenericType{
			Name:           "flatMap",
			TypeParameters: []*types.TypeParameter{uParam},
			Body: types.NewSimpleFunction([]types.Type{mapperType},
				&types.InstantiatedType{
					Generic:       iteratorGeneric,
					TypeArguments: []types.Type{uType},
				}),
		})

	// Set the body of the generic type
	iteratorGeneric.Body = iteratorType

	// Create Iterable<T> interface
	// interface Iterable<T> { [Symbol.iterator](): Iterator<T>; }
	iterableType := types.NewObjectType().
		WithProperty("__COMPUTED_PROPERTY__", types.NewSimpleFunction([]types.Type{},
			&types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{tType},
			}))

	// Create generic Iterable type
	iterableGeneric := &types.GenericType{
		Name:           "Iterable",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           iterableType,
	}

	// Register the types in global environment
	_ = ctx.DefineGlobal("IteratorResult", iteratorResultGeneric)
	_ = ctx.DefineGlobal("Iterable", iterableGeneric)
	// Register the generic Iterator type for internal use (other initializers need it for type definitions)
	_ = ctx.DefineGlobal("__IteratorGeneric__", iteratorGeneric)

	// Create Iterator constructor type with static methods
	// Iterator.from(items: Iterable<T>): Iterator<T>
	// Iterator.concat(...iterables: Iterable<T>[]): Iterator<T>
	// Iterator.zip(iterables: Iterable<T>[], options?: {...}): Iterator<T[]>
	// Iterator.zipKeyed(iterables: {[key: string]: Iterable<any>}, options?: {...}): Iterator<{[key: string]: any}>
	iteratorCtorType := types.NewObjectType().
		// from(items: any): Iterator<any>
		// Note: ideally from<T>(items: Iterable<T>): Iterator<T>, but simplified for now
		WithProperty("from", types.NewSimpleFunction(
			[]types.Type{types.Any},
			&types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{types.Any},
			})).
		// concat(...iterables: any[]): Iterator<any>
		// Note: ideally concat<T>(...iterables: Iterable<T>[]): Iterator<T>, but simplified for now
		WithProperty("concat", types.NewVariadicFunction(
			[]types.Type{},
			&types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{types.Any},
			},
			&types.ArrayType{ElementType: types.Any})).
		// zip<T>(iterables: Iterable<Iterable<T>>, options?: {mode?: string, padding?: T[]}): Iterator<T[]>
		WithProperty("zip", types.NewOptionalFunction(
			[]types.Type{
				types.Any, // iterables
				types.Any, // options
			},
			&types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{&types.ArrayType{ElementType: types.Any}},
			},
			[]bool{false, true})).
		// zipKeyed(iterables: {[key: string]: Iterable<any>}, options?: {...}): Iterator<{[key: string]: any}>
		WithProperty("zipKeyed", types.NewOptionalFunction(
			[]types.Type{
				types.Any, // iterables object
				types.Any, // options
			},
			&types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{types.NewObjectType()},
			},
			[]bool{false, true})).
		WithProperty("prototype", iteratorType)

	_ = ctx.DefineGlobal("Iterator", iteratorCtorType)

	return nil
}

func (i *IteratorInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// ============================================
	// Create Iterator.prototype (%Iterator.prototype%)
	// ============================================
	iteratorProto := vm.NewObject(objectProto).AsPlainObject()

	// Helper to get iterator's next method and call it
	getIteratorNext := func(iterator vm.Value) (vm.Value, error) {
		nextMethod, err := vmInstance.GetProperty(iterator, "next")
		if err != nil {
			return vm.Undefined, err
		}
		if !nextMethod.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator next is not callable")
		}
		return vmInstance.Call(nextMethod, iterator, []vm.Value{})
	}

	// Helper to create iterator result object
	createIteratorResult := func(value vm.Value, done bool) vm.Value {
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		result.SetOwn("value", value)
		result.SetOwn("done", vm.BooleanValue(done))
		return vm.NewValueFromPlainObject(result)
	}

	// Helper to close an iterator (call return() if it exists)
	closeIterator := func(iterator vm.Value) {
		returnMethod, err := vmInstance.GetProperty(iterator, "return")
		if err != nil || returnMethod.IsUndefined() || !returnMethod.IsCallable() {
			return
		}
		// Ignore errors from return()
		_, _ = vmInstance.Call(returnMethod, iterator, []vm.Value{})
	}

	w, e, c := true, false, true // writable, not enumerable, configurable
	falseVal := false
	trueVal := true

	// Add Symbol.iterator to Iterator.prototype - returns this
	iteratorProto.DefineOwnPropertyByKey(
		vm.NewSymbolKey(SymbolIterator),
		vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(args []vm.Value) (vm.Value, error) {
			return vmInstance.GetThis(), nil
		}),
		&w, // writable: true
		&e, // enumerable: false
		&c, // configurable: true
	)

	// Add Symbol.toStringTag = "Iterator" to Iterator.prototype
	iteratorProto.DefineOwnPropertyByKey(
		vm.NewSymbolKey(SymbolToStringTag),
		vm.NewString("Iterator"),
		&falseVal, // writable: false
		&falseVal, // enumerable: false
		&trueVal,  // configurable: true
	)

	// Add Symbol.dispose to Iterator.prototype - calls return() if it exists
	iteratorProto.DefineOwnPropertyByKey(
		vm.NewSymbolKey(SymbolDispose),
		vm.NewNativeFunction(0, false, "[Symbol.dispose]", func(args []vm.Value) (vm.Value, error) {
			thisValue := vmInstance.GetThis()
			returnMethod, err := vmInstance.GetProperty(thisValue, "return")
			if err != nil || returnMethod.IsUndefined() || !returnMethod.IsCallable() {
				return vm.Undefined, nil
			}
			_, err = vmInstance.Call(returnMethod, thisValue, []vm.Value{})
			if err != nil {
				return vm.Undefined, err
			}
			return vm.Undefined, nil
		}),
		&w, // writable: true
		&e, // enumerable: false
		&c, // configurable: true
	)

	// ============================================
	// Create IteratorHelper prototype (%IteratorHelperPrototype%)
	// This is the prototype for iterator objects returned by map, filter, etc.
	// ============================================
	iteratorHelperProto := vm.NewObject(vm.NewValueFromPlainObject(iteratorProto)).AsPlainObject()

	// Add Symbol.toStringTag = "Iterator Helper" to IteratorHelperPrototype
	iteratorHelperProto.DefineOwnPropertyByKey(
		vm.NewSymbolKey(SymbolToStringTag),
		vm.NewString("Iterator Helper"),
		&falseVal, // writable: false
		&falseVal, // enumerable: false
		&trueVal,  // configurable: true
	)

	// Store IteratorHelperPrototype in VM
	vmInstance.IteratorHelperPrototype = vm.NewValueFromPlainObject(iteratorHelperProto)

	// ============================================
	// Create WrapForValidIteratorPrototype
	// For Iterator.from() wrapped iterators
	// ============================================
	wrapForValidIteratorProto := vm.NewObject(vm.NewValueFromPlainObject(iteratorProto)).AsPlainObject()

	// Add next method for wrapped iterators
	wrapForValidIteratorProto.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsObject() {
			return vm.Undefined, vmInstance.NewTypeError("next called on non-object")
		}
		obj := thisValue.AsPlainObject()
		if obj == nil {
			return vm.Undefined, vmInstance.NewTypeError("next called on non-object")
		}

		// Get the wrapped iterator
		wrappedIter, exists := obj.GetOwn("[[Iterated]]")
		if !exists || wrappedIter.IsUndefined() {
			return createIteratorResult(vm.Undefined, true), nil
		}

		// Call next on wrapped iterator
		result, err := getIteratorNext(wrappedIter)
		if err != nil {
			return vm.Undefined, err
		}
		return result, nil
	}))

	// Add return method for wrapped iterators
	wrapForValidIteratorProto.SetOwnNonEnumerable("return", vm.NewNativeFunction(0, false, "return", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsObject() {
			return vm.Undefined, vmInstance.NewTypeError("return called on non-object")
		}
		obj := thisValue.AsPlainObject()
		if obj == nil {
			return createIteratorResult(vm.Undefined, true), nil
		}

		// Get the wrapped iterator
		wrappedIter, exists := obj.GetOwn("[[Iterated]]")
		if !exists || wrappedIter.IsUndefined() {
			return createIteratorResult(vm.Undefined, true), nil
		}

		// Close the wrapped iterator
		closeIterator(wrappedIter)
		return createIteratorResult(vm.Undefined, true), nil
	}))

	vmInstance.WrapForValidIteratorPrototype = vm.NewValueFromPlainObject(wrapForValidIteratorProto)

	// ============================================
	// Iterator.prototype[Symbol.iterator]
	// Returns this iterator
	// ============================================
	symbolIteratorFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(args []vm.Value) (vm.Value, error) {
		return vmInstance.GetThis(), nil
	})
	iteratorProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), symbolIteratorFn, &w, &e, &c)

	// ============================================
	// Iterator.prototype[Symbol.toStringTag] - accessor property
	// Per spec: { get: function, set: function, enumerable: false, configurable: true }
	// ============================================
	toStringTagGetter := vm.NewNativeFunction(0, false, "get [Symbol.toStringTag]", func(args []vm.Value) (vm.Value, error) {
		return vm.NewString("Iterator"), nil
	})
	toStringTagSetter := vm.NewNativeFunction(1, false, "set [Symbol.toStringTag]", func(args []vm.Value) (vm.Value, error) {
		// Setter does nothing per spec
		return vm.Undefined, nil
	})
	iteratorProto.DefineAccessorPropertyByKey(
		vm.NewSymbolKey(SymbolToStringTag),
		toStringTagGetter, true,
		toStringTagSetter, true,
		&falseVal, // enumerable: false
		&trueVal,  // configurable: true
	)

	// ============================================
	// Iterator.prototype.map(mapper)
	// Returns a new iterator that applies mapper to each value
	// ============================================
	iteratorProto.SetOwnNonEnumerable("map", vm.NewNativeFunction(1, false, "map", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsObject() && !thisValue.IsGenerator() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.map called on non-object")
		}

		if len(args) == 0 || !args[0].IsCallable() {
			// Close the iterator before throwing
			closeIterator(thisValue)
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.map requires a callable argument")
		}
		mapper := args[0]

		// Create iterator helper object
		helper := vm.NewObject(vmInstance.IteratorHelperPrototype).AsPlainObject()
		helper.SetOwn("[[UnderlyingIterator]]", thisValue)
		helper.SetOwn("[[Mapper]]", mapper)
		helper.SetOwn("[[Counter]]", vm.NumberValue(0))

		// Add next method
		helper.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
			helperThis := vmInstance.GetThis()
			helperObj := helperThis.AsPlainObject()
			if helperObj == nil {
				return vm.Undefined, vmInstance.NewTypeError("next called on non-object")
			}

			underlyingIter, _ := helperObj.GetOwn("[[UnderlyingIterator]]")
			mapperFn, _ := helperObj.GetOwn("[[Mapper]]")
			counterVal, _ := helperObj.GetOwn("[[Counter]]")
			counter := int(counterVal.ToFloat())

			// Get next value from underlying iterator
			result, err := getIteratorNext(underlyingIter)
			if err != nil {
				return vm.Undefined, err
			}

			// Check if done
			doneVal, _ := vmInstance.GetProperty(result, "done")
			if doneVal.IsTruthy() {
				return createIteratorResult(vm.Undefined, true), nil
			}

			// Get value and apply mapper
			valueVal, _ := vmInstance.GetProperty(result, "value")
			mapped, err := vmInstance.Call(mapperFn, vm.Undefined, []vm.Value{valueVal, vm.NumberValue(float64(counter))})
			if err != nil {
				closeIterator(underlyingIter)
				return vm.Undefined, err
			}

			// Update counter
			helperObj.SetOwn("[[Counter]]", vm.NumberValue(float64(counter+1)))

			return createIteratorResult(mapped, false), nil
		}))

		// Add return method
		helper.SetOwnNonEnumerable("return", vm.NewNativeFunction(0, false, "return", func(args []vm.Value) (vm.Value, error) {
			helperThis := vmInstance.GetThis()
			helperObj := helperThis.AsPlainObject()
			if helperObj != nil {
				underlyingIter, _ := helperObj.GetOwn("[[UnderlyingIterator]]")
				closeIterator(underlyingIter)
			}
			return createIteratorResult(vm.Undefined, true), nil
		}))

		return vm.NewValueFromPlainObject(helper), nil
	}))

	// ============================================
	// Iterator.prototype.filter(predicate)
	// Returns a new iterator that yields only values that pass the predicate
	// ============================================
	iteratorProto.SetOwnNonEnumerable("filter", vm.NewNativeFunction(1, false, "filter", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsObject() && !thisValue.IsGenerator() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.filter called on non-object")
		}

		if len(args) == 0 || !args[0].IsCallable() {
			// Close the iterator before throwing
			closeIterator(thisValue)
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.filter requires a callable argument")
		}
		predicate := args[0]

		// Create iterator helper object
		helper := vm.NewObject(vmInstance.IteratorHelperPrototype).AsPlainObject()
		helper.SetOwn("[[UnderlyingIterator]]", thisValue)
		helper.SetOwn("[[Predicate]]", predicate)
		helper.SetOwn("[[Counter]]", vm.NumberValue(0))

		// Add next method
		helper.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
			helperThis := vmInstance.GetThis()
			helperObj := helperThis.AsPlainObject()
			if helperObj == nil {
				return vm.Undefined, vmInstance.NewTypeError("next called on non-object")
			}

			underlyingIter, _ := helperObj.GetOwn("[[UnderlyingIterator]]")
			predicateFn, _ := helperObj.GetOwn("[[Predicate]]")
			counterVal, _ := helperObj.GetOwn("[[Counter]]")
			counter := int(counterVal.ToFloat())

			for {
				// Get next value from underlying iterator
				result, err := getIteratorNext(underlyingIter)
				if err != nil {
					return vm.Undefined, err
				}

				// Check if done
				doneVal, _ := vmInstance.GetProperty(result, "done")
				if doneVal.IsTruthy() {
					return createIteratorResult(vm.Undefined, true), nil
				}

				// Get value and test predicate
				valueVal, _ := vmInstance.GetProperty(result, "value")
				passed, err := vmInstance.Call(predicateFn, vm.Undefined, []vm.Value{valueVal, vm.NumberValue(float64(counter))})
				counter++
				helperObj.SetOwn("[[Counter]]", vm.NumberValue(float64(counter)))

				if err != nil {
					closeIterator(underlyingIter)
					return vm.Undefined, err
				}

				if passed.IsTruthy() {
					return createIteratorResult(valueVal, false), nil
				}
				// Continue to next value
			}
		}))

		// Add return method
		helper.SetOwnNonEnumerable("return", vm.NewNativeFunction(0, false, "return", func(args []vm.Value) (vm.Value, error) {
			helperThis := vmInstance.GetThis()
			helperObj := helperThis.AsPlainObject()
			if helperObj != nil {
				underlyingIter, _ := helperObj.GetOwn("[[UnderlyingIterator]]")
				closeIterator(underlyingIter)
			}
			return createIteratorResult(vm.Undefined, true), nil
		}))

		return vm.NewValueFromPlainObject(helper), nil
	}))

	// ============================================
	// Iterator.prototype.take(limit)
	// Returns a new iterator that yields at most limit values
	// ============================================
	iteratorProto.SetOwnNonEnumerable("take", vm.NewNativeFunction(1, false, "take", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsObject() && !thisValue.IsGenerator() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.take called on non-object")
		}

		// Per spec: Let numLimit be ? ToNumber(limit)
		var limitVal vm.Value
		if len(args) > 0 {
			limitVal = args[0]
		} else {
			limitVal = vm.Undefined
		}

		// Convert to number - for objects, call ToPrimitive first
		var numLimit float64
		if limitVal.IsObject() || limitVal.IsCallable() {
			vmInstance.EnterHelperCall()
			primitiveVal := vmInstance.ToPrimitive(limitVal, "number")
			vmInstance.ExitHelperCall()
			// Check if ToPrimitive threw an exception
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				closeIterator(thisValue)
				return vm.Undefined, nil // Let exception propagate
			}
			numLimit = primitiveVal.ToFloat()
		} else {
			numLimit = limitVal.ToFloat()
		}

		// Per spec: If numLimit is NaN, throw a RangeError exception
		if numLimit != numLimit { // NaN check
			closeIterator(thisValue)
			return vm.Undefined, vmInstance.NewRangeError("Iterator.prototype.take: limit must be a finite number")
		}

		// Per spec: Let integerLimit be ! ToIntegerOrInfinity(numLimit)
		limit := int(numLimit)

		// Per spec: If integerLimit < 0, throw a RangeError exception
		if limit < 0 {
			closeIterator(thisValue)
			return vm.Undefined, vmInstance.NewRangeError("Iterator.prototype.take: limit must be non-negative")
		}

		// Per spec step 7: GetIteratorDirect(O) - read next method eagerly
		nextMethod, err := vmInstance.GetProperty(thisValue, "next")
		if err != nil {
			return vm.Undefined, err
		}

		// Create iterator helper object
		helper := vm.NewObject(vmInstance.IteratorHelperPrototype).AsPlainObject()
		helper.SetOwn("[[UnderlyingIterator]]", thisValue)
		helper.SetOwn("[[NextMethod]]", nextMethod)
		helper.SetOwn("[[Remaining]]", vm.NumberValue(float64(limit)))

		// Add next method
		helper.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
			helperThis := vmInstance.GetThis()
			helperObj := helperThis.AsPlainObject()
			if helperObj == nil {
				return vm.Undefined, vmInstance.NewTypeError("next called on non-object")
			}

			remainingVal, _ := helperObj.GetOwn("[[Remaining]]")
			remaining := int(remainingVal.ToFloat())

			if remaining <= 0 {
				underlyingIter, _ := helperObj.GetOwn("[[UnderlyingIterator]]")
				closeIterator(underlyingIter)
				return createIteratorResult(vm.Undefined, true), nil
			}

			underlyingIter, _ := helperObj.GetOwn("[[UnderlyingIterator]]")
			storedNext, _ := helperObj.GetOwn("[[NextMethod]]")

			// Call stored next method
			var result vm.Value
			if storedNext.IsCallable() {
				result, err = vmInstance.Call(storedNext, underlyingIter, []vm.Value{})
			} else {
				result, err = getIteratorNext(underlyingIter)
			}
			if err != nil {
				return vm.Undefined, err
			}

			// Check if done
			doneVal, _ := vmInstance.GetProperty(result, "done")
			if doneVal.IsTruthy() {
				return createIteratorResult(vm.Undefined, true), nil
			}

			// Decrement remaining
			helperObj.SetOwn("[[Remaining]]", vm.NumberValue(float64(remaining-1)))

			// Get value
			valueVal, _ := vmInstance.GetProperty(result, "value")
			return createIteratorResult(valueVal, false), nil
		}))

		// Add return method
		helper.SetOwnNonEnumerable("return", vm.NewNativeFunction(0, false, "return", func(args []vm.Value) (vm.Value, error) {
			helperThis := vmInstance.GetThis()
			helperObj := helperThis.AsPlainObject()
			if helperObj != nil {
				underlyingIter, _ := helperObj.GetOwn("[[UnderlyingIterator]]")
				closeIterator(underlyingIter)
			}
			return createIteratorResult(vm.Undefined, true), nil
		}))

		return vm.NewValueFromPlainObject(helper), nil
	}))

	// ============================================
	// Iterator.prototype.drop(count)
	// Returns a new iterator that skips the first count values
	// ============================================
	iteratorProto.SetOwnNonEnumerable("drop", vm.NewNativeFunction(1, false, "drop", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsObject() && !thisValue.IsGenerator() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.drop called on non-object")
		}

		// Per spec: Let numLimit be ? ToNumber(limit)
		var limitVal vm.Value
		if len(args) > 0 {
			limitVal = args[0]
		} else {
			limitVal = vm.Undefined
		}

		// Convert to number - for objects, call ToPrimitive first
		var numLimit float64
		if limitVal.IsObject() || limitVal.IsCallable() {
			vmInstance.EnterHelperCall()
			primitiveVal := vmInstance.ToPrimitive(limitVal, "number")
			vmInstance.ExitHelperCall()
			// Check if ToPrimitive threw an exception
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				closeIterator(thisValue)
				return vm.Undefined, nil // Let exception propagate
			}
			numLimit = primitiveVal.ToFloat()
		} else {
			numLimit = limitVal.ToFloat()
		}

		// Per spec: If numLimit is NaN, throw a RangeError exception
		if numLimit != numLimit { // NaN check
			closeIterator(thisValue)
			return vm.Undefined, vmInstance.NewRangeError("Iterator.prototype.drop: limit must be a finite number")
		}

		// Per spec: Let integerLimit be ! ToIntegerOrInfinity(numLimit)
		// ToIntegerOrInfinity: truncates towards zero
		count := int(numLimit)

		// Per spec: If integerLimit < 0, throw a RangeError exception
		if count < 0 {
			closeIterator(thisValue)
			return vm.Undefined, vmInstance.NewRangeError("Iterator.prototype.drop: limit must be non-negative")
		}

		// Per spec step 7: GetIteratorDirect(O) - read next method eagerly
		nextMethod, err := vmInstance.GetProperty(thisValue, "next")
		if err != nil {
			return vm.Undefined, err
		}

		// Create iterator helper object
		helper := vm.NewObject(vmInstance.IteratorHelperPrototype).AsPlainObject()
		helper.SetOwn("[[UnderlyingIterator]]", thisValue)
		helper.SetOwn("[[NextMethod]]", nextMethod)
		helper.SetOwn("[[ToSkip]]", vm.NumberValue(float64(count)))

		// Add next method
		helper.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
			helperThis := vmInstance.GetThis()
			helperObj := helperThis.AsPlainObject()
			if helperObj == nil {
				return vm.Undefined, vmInstance.NewTypeError("next called on non-object")
			}

			underlyingIter, _ := helperObj.GetOwn("[[UnderlyingIterator]]")
			storedNext, _ := helperObj.GetOwn("[[NextMethod]]")
			toSkipVal, _ := helperObj.GetOwn("[[ToSkip]]")
			toSkip := int(toSkipVal.ToFloat())

			// Helper to call stored next method
			callNext := func() (vm.Value, error) {
				if storedNext.IsCallable() {
					return vmInstance.Call(storedNext, underlyingIter, []vm.Value{})
				}
				return getIteratorNext(underlyingIter)
			}

			// Skip values
			for toSkip > 0 {
				result, err := callNext()
				if err != nil {
					return vm.Undefined, err
				}
				doneVal, _ := vmInstance.GetProperty(result, "done")
				if doneVal.IsTruthy() {
					return createIteratorResult(vm.Undefined, true), nil
				}
				toSkip--
				helperObj.SetOwn("[[ToSkip]]", vm.NumberValue(float64(toSkip)))
			}

			// Get next value from underlying iterator
			result, err := callNext()
			if err != nil {
				return vm.Undefined, err
			}

			// Check if done
			doneVal, _ := vmInstance.GetProperty(result, "done")
			if doneVal.IsTruthy() {
				return createIteratorResult(vm.Undefined, true), nil
			}

			// Get value
			valueVal, _ := vmInstance.GetProperty(result, "value")
			return createIteratorResult(valueVal, false), nil
		}))

		// Add return method
		helper.SetOwnNonEnumerable("return", vm.NewNativeFunction(0, false, "return", func(args []vm.Value) (vm.Value, error) {
			helperThis := vmInstance.GetThis()
			helperObj := helperThis.AsPlainObject()
			if helperObj != nil {
				underlyingIter, _ := helperObj.GetOwn("[[UnderlyingIterator]]")
				closeIterator(underlyingIter)
			}
			return createIteratorResult(vm.Undefined, true), nil
		}))

		return vm.NewValueFromPlainObject(helper), nil
	}))

	// ============================================
	// Iterator.prototype.toArray()
	// Collects all remaining values into an array
	// ============================================
	iteratorProto.SetOwnNonEnumerable("toArray", vm.NewNativeFunction(0, false, "toArray", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsObject() && !thisValue.IsGenerator() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.toArray called on non-object")
		}

		result := vm.NewArray()
		arr := result.AsArray()

		for {
			next, err := getIteratorNext(thisValue)
			if err != nil {
				return vm.Undefined, err
			}

			doneVal, _ := vmInstance.GetProperty(next, "done")
			if doneVal.IsTruthy() {
				break
			}

			valueVal, _ := vmInstance.GetProperty(next, "value")
			arr.Append(valueVal)
		}

		return result, nil
	}))

	// ============================================
	// Iterator.prototype.forEach(fn)
	// Calls fn for each value, returns undefined
	// ============================================
	iteratorProto.SetOwnNonEnumerable("forEach", vm.NewNativeFunction(1, false, "forEach", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsObject() && !thisValue.IsGenerator() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.forEach called on non-object")
		}

		if len(args) == 0 || !args[0].IsCallable() {
			// Close the iterator before throwing
			closeIterator(thisValue)
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.forEach requires a callable argument")
		}
		fn := args[0]

		counter := 0
		for {
			next, err := getIteratorNext(thisValue)
			if err != nil {
				return vm.Undefined, err
			}

			doneVal, _ := vmInstance.GetProperty(next, "done")
			if doneVal.IsTruthy() {
				break
			}

			valueVal, _ := vmInstance.GetProperty(next, "value")
			_, err = vmInstance.Call(fn, vm.Undefined, []vm.Value{valueVal, vm.NumberValue(float64(counter))})
			if err != nil {
				closeIterator(thisValue)
				return vm.Undefined, err
			}
			counter++
		}

		return vm.Undefined, nil
	}))

	// ============================================
	// Iterator.prototype.reduce(reducer, initialValue?)
	// Reduces iterator to a single value
	// ============================================
	iteratorProto.SetOwnNonEnumerable("reduce", vm.NewNativeFunction(1, false, "reduce", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsObject() && !thisValue.IsGenerator() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.reduce called on non-object")
		}

		if len(args) == 0 || !args[0].IsCallable() {
			// Close the iterator before throwing
			closeIterator(thisValue)
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.reduce requires a callable argument")
		}
		reducer := args[0]

		var accumulator vm.Value
		hasInitial := len(args) >= 2
		if hasInitial {
			accumulator = args[1]
		}

		counter := 0
		for {
			next, err := getIteratorNext(thisValue)
			if err != nil {
				return vm.Undefined, err
			}

			doneVal, _ := vmInstance.GetProperty(next, "done")
			if doneVal.IsTruthy() {
				break
			}

			valueVal, _ := vmInstance.GetProperty(next, "value")

			if !hasInitial && counter == 0 {
				// First value becomes accumulator
				accumulator = valueVal
				counter++
				continue
			}

			accumulator, err = vmInstance.Call(reducer, vm.Undefined, []vm.Value{accumulator, valueVal, vm.NumberValue(float64(counter))})
			if err != nil {
				closeIterator(thisValue)
				return vm.Undefined, err
			}
			counter++
		}

		if !hasInitial && counter == 0 {
			return vm.Undefined, vmInstance.NewTypeError("Reduce of empty iterator with no initial value")
		}

		return accumulator, nil
	}))

	// ============================================
	// Iterator.prototype.some(predicate)
	// Returns true if any value passes the predicate
	// ============================================
	iteratorProto.SetOwnNonEnumerable("some", vm.NewNativeFunction(1, false, "some", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsObject() && !thisValue.IsGenerator() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.some called on non-object")
		}

		if len(args) == 0 || !args[0].IsCallable() {
			// Close the iterator before throwing
			closeIterator(thisValue)
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.some requires a callable argument")
		}
		predicate := args[0]

		counter := 0
		for {
			next, err := getIteratorNext(thisValue)
			if err != nil {
				return vm.Undefined, err
			}

			doneVal, _ := vmInstance.GetProperty(next, "done")
			if doneVal.IsTruthy() {
				break
			}

			valueVal, _ := vmInstance.GetProperty(next, "value")
			result, err := vmInstance.Call(predicate, vm.Undefined, []vm.Value{valueVal, vm.NumberValue(float64(counter))})
			if err != nil {
				closeIterator(thisValue)
				return vm.Undefined, err
			}

			if result.IsTruthy() {
				closeIterator(thisValue)
				return vm.BooleanValue(true), nil
			}
			counter++
		}

		return vm.BooleanValue(false), nil
	}))

	// ============================================
	// Iterator.prototype.every(predicate)
	// Returns true if all values pass the predicate
	// ============================================
	iteratorProto.SetOwnNonEnumerable("every", vm.NewNativeFunction(1, false, "every", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsObject() && !thisValue.IsGenerator() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.every called on non-object")
		}

		if len(args) == 0 || !args[0].IsCallable() {
			// Close the iterator before throwing
			closeIterator(thisValue)
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.every requires a callable argument")
		}
		predicate := args[0]

		counter := 0
		for {
			next, err := getIteratorNext(thisValue)
			if err != nil {
				return vm.Undefined, err
			}

			doneVal, _ := vmInstance.GetProperty(next, "done")
			if doneVal.IsTruthy() {
				break
			}

			valueVal, _ := vmInstance.GetProperty(next, "value")
			result, err := vmInstance.Call(predicate, vm.Undefined, []vm.Value{valueVal, vm.NumberValue(float64(counter))})
			if err != nil {
				closeIterator(thisValue)
				return vm.Undefined, err
			}

			if !result.IsTruthy() {
				closeIterator(thisValue)
				return vm.BooleanValue(false), nil
			}
			counter++
		}

		return vm.BooleanValue(true), nil
	}))

	// ============================================
	// Iterator.prototype.find(predicate)
	// Returns the first value that passes the predicate
	// ============================================
	iteratorProto.SetOwnNonEnumerable("find", vm.NewNativeFunction(1, false, "find", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsObject() && !thisValue.IsGenerator() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.find called on non-object")
		}

		if len(args) == 0 || !args[0].IsCallable() {
			// Close the iterator before throwing
			closeIterator(thisValue)
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.find requires a callable argument")
		}
		predicate := args[0]

		counter := 0
		for {
			next, err := getIteratorNext(thisValue)
			if err != nil {
				return vm.Undefined, err
			}

			doneVal, _ := vmInstance.GetProperty(next, "done")
			if doneVal.IsTruthy() {
				break
			}

			valueVal, _ := vmInstance.GetProperty(next, "value")
			result, err := vmInstance.Call(predicate, vm.Undefined, []vm.Value{valueVal, vm.NumberValue(float64(counter))})
			if err != nil {
				closeIterator(thisValue)
				return vm.Undefined, err
			}

			if result.IsTruthy() {
				closeIterator(thisValue)
				return valueVal, nil
			}
			counter++
		}

		return vm.Undefined, nil
	}))

	// ============================================
	// Iterator.prototype.flatMap(mapper)
	// Maps each value to an iterator and flattens one level
	// ============================================
	iteratorProto.SetOwnNonEnumerable("flatMap", vm.NewNativeFunction(1, false, "flatMap", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsObject() && !thisValue.IsGenerator() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.flatMap called on non-object")
		}

		if len(args) == 0 || !args[0].IsCallable() {
			// Close the iterator before throwing
			closeIterator(thisValue)
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.flatMap requires a callable argument")
		}
		mapper := args[0]

		// Create iterator helper object
		helper := vm.NewObject(vmInstance.IteratorHelperPrototype).AsPlainObject()
		helper.SetOwn("[[UnderlyingIterator]]", thisValue)
		helper.SetOwn("[[Mapper]]", mapper)
		helper.SetOwn("[[InnerIterator]]", vm.Undefined)
		helper.SetOwn("[[Counter]]", vm.NumberValue(0))

		// Helper to get iterator from value using GetIteratorFlattenable semantics
		// GetIteratorFlattenable ONLY accepts objects, not primitives
		getIterator := func(value vm.Value) (vm.Value, error) {
			// Per spec: GetIteratorFlattenable rejects primitives even if their prototype has Symbol.iterator
			// Only objects are allowed to be flattened
			if !value.IsObject() && !value.IsGenerator() {
				return vm.Undefined, vmInstance.NewTypeError("Value is not an Object")
			}
			// Try to get Symbol.iterator
			if iterMethod, ok := vmInstance.GetSymbolProperty(value, SymbolIterator); ok && iterMethod.IsCallable() {
				return vmInstance.Call(iterMethod, value, []vm.Value{})
			}
			// Check if it's already an iterator (has next method)
			nextMethod, _ := vmInstance.GetProperty(value, "next")
			if nextMethod.IsCallable() {
				return value, nil
			}
			return vm.Undefined, vmInstance.NewTypeError("Value is not iterable")
		}

		// Add next method
		helper.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
			helperThis := vmInstance.GetThis()
			helperObj := helperThis.AsPlainObject()
			if helperObj == nil {
				return vm.Undefined, vmInstance.NewTypeError("next called on non-object")
			}

			underlyingIter, _ := helperObj.GetOwn("[[UnderlyingIterator]]")
			mapperFn, _ := helperObj.GetOwn("[[Mapper]]")
			innerIter, _ := helperObj.GetOwn("[[InnerIterator]]")
			counterVal, _ := helperObj.GetOwn("[[Counter]]")
			counter := int(counterVal.ToFloat())

			for {
				// If we have an inner iterator, try to get values from it
				if !innerIter.IsUndefined() {
					result, err := getIteratorNext(innerIter)
					if err != nil {
						closeIterator(underlyingIter)
						return vm.Undefined, err
					}

					doneVal, _ := vmInstance.GetProperty(result, "done")
					if !doneVal.IsTruthy() {
						valueVal, _ := vmInstance.GetProperty(result, "value")
						return createIteratorResult(valueVal, false), nil
					}

					// Inner iterator exhausted
					helperObj.SetOwn("[[InnerIterator]]", vm.Undefined)
					innerIter = vm.Undefined
				}

				// Get next value from underlying iterator
				result, err := getIteratorNext(underlyingIter)
				if err != nil {
					return vm.Undefined, err
				}

				doneVal, _ := vmInstance.GetProperty(result, "done")
				if doneVal.IsTruthy() {
					return createIteratorResult(vm.Undefined, true), nil
				}

				// Apply mapper
				valueVal, _ := vmInstance.GetProperty(result, "value")
				mapped, err := vmInstance.Call(mapperFn, vm.Undefined, []vm.Value{valueVal, vm.NumberValue(float64(counter))})
				if err != nil {
					closeIterator(underlyingIter)
					return vm.Undefined, err
				}
				counter++
				helperObj.SetOwn("[[Counter]]", vm.NumberValue(float64(counter)))

				// Get iterator from mapped value
				innerIter, err = getIterator(mapped)
				if err != nil {
					closeIterator(underlyingIter)
					return vm.Undefined, err
				}
				helperObj.SetOwn("[[InnerIterator]]", innerIter)
			}
		}))

		// Add return method
		helper.SetOwnNonEnumerable("return", vm.NewNativeFunction(0, false, "return", func(args []vm.Value) (vm.Value, error) {
			helperThis := vmInstance.GetThis()
			helperObj := helperThis.AsPlainObject()
			if helperObj != nil {
				underlyingIter, _ := helperObj.GetOwn("[[UnderlyingIterator]]")
				closeIterator(underlyingIter)
				innerIter, _ := helperObj.GetOwn("[[InnerIterator]]")
				if !innerIter.IsUndefined() {
					closeIterator(innerIter)
				}
			}
			return createIteratorResult(vm.Undefined, true), nil
		}))

		return vm.NewValueFromPlainObject(helper), nil
	}))

	// Store Iterator.prototype in VM
	vmInstance.IteratorPrototype = vm.NewValueFromPlainObject(iteratorProto)

	// ============================================
	// Create specific iterator prototypes
	// These inherit from Iterator.prototype and have their own Symbol.toStringTag
	// ============================================

	// %ArrayIteratorPrototype%
	arrayIteratorProto := vm.NewObject(vmInstance.IteratorPrototype).AsPlainObject()
	arrayIteratorProto.DefineOwnPropertyByKey(
		vm.NewSymbolKey(SymbolToStringTag),
		vm.NewString("Array Iterator"),
		&falseVal, // writable: false
		&falseVal, // enumerable: false
		&trueVal,  // configurable: true
	)
	vmInstance.ArrayIteratorPrototype = vm.NewValueFromPlainObject(arrayIteratorProto)

	// %MapIteratorPrototype%
	mapIteratorProto := vm.NewObject(vmInstance.IteratorPrototype).AsPlainObject()
	mapIteratorProto.DefineOwnPropertyByKey(
		vm.NewSymbolKey(SymbolToStringTag),
		vm.NewString("Map Iterator"),
		&falseVal, // writable: false
		&falseVal, // enumerable: false
		&trueVal,  // configurable: true
	)
	// %MapIteratorPrototype%.next - defined on prototype with proper length/name
	mapIterNextFn := vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// Check for internal slots (branding)
		if !thisVal.IsObject() {
			return vm.Undefined, vmInstance.NewTypeError("%MapIteratorPrototype%.next requires that 'this' be an Object")
		}
		thisObj := thisVal.AsPlainObject()
		if thisObj == nil {
			return vm.Undefined, vmInstance.NewTypeError("%MapIteratorPrototype%.next requires that 'this' be an Object")
		}
		// Check for [[IteratedMap]] internal slot
		iteratedMap, hasSlot := thisObj.GetOwn("[[IteratedMap]]")
		if !hasSlot {
			return vm.Undefined, vmInstance.NewTypeError("%MapIteratorPrototype%.next requires that 'this' be a Map Iterator")
		}

		result := vm.NewObject(vm.Undefined).AsPlainObject()

		// Check if exhausted
		if exhaustedVal, ok := thisObj.GetOwn("[[Exhausted]]"); ok && exhaustedVal == vm.True {
			result.SetOwn("value", vm.Undefined)
			result.SetOwn("done", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(result), nil
		}

		// Get current index and kind
		indexVal, _ := thisObj.GetOwn("[[MapNextIndex]]")
		kindVal, _ := thisObj.GetOwn("[[MapIterationKind]]")
		currentIndex := int(indexVal.ToFloat())
		kind := kindVal.ToString() // "entries", "keys", or "values"

		// Get the map
		if iteratedMap.Type() != vm.TypeMap {
			result.SetOwn("value", vm.Undefined)
			result.SetOwn("done", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(result), nil
		}
		mapObj := iteratedMap.AsMap()

		// Iterate
		for currentIndex < mapObj.OrderLen() {
			key, value, exists := mapObj.GetEntryAt(currentIndex)
			currentIndex++
			thisObj.SetOwn("[[MapNextIndex]]", vm.NumberValue(float64(currentIndex)))
			if exists {
				var resultValue vm.Value
				switch kind {
				case "entries":
					entry := vm.NewArray()
					entry.AsArray().Append(key)
					entry.AsArray().Append(value)
					resultValue = entry
				case "keys":
					resultValue = key
				case "values":
					resultValue = value
				default:
					entry := vm.NewArray()
					entry.AsArray().Append(key)
					entry.AsArray().Append(value)
					resultValue = entry
				}
				result.SetOwn("value", resultValue)
				result.SetOwn("done", vm.BooleanValue(false))
				return vm.NewValueFromPlainObject(result), nil
			}
		}

		// Exhausted
		thisObj.SetOwn("[[Exhausted]]", vm.BooleanValue(true))
		thisObj.SetOwn("[[MapNextIndex]]", vm.NumberValue(float64(currentIndex)))
		result.SetOwn("value", vm.Undefined)
		result.SetOwn("done", vm.BooleanValue(true))
		return vm.NewValueFromPlainObject(result), nil
	})
	// Set length and name properties on the next function
	mapIterNextFnObj := mapIterNextFn.AsNativeFunction()
	_ = mapIterNextFnObj // length and name are already set by NewNativeFunction
	mapIteratorProto.DefineOwnProperty("next", mapIterNextFn, &trueVal, &falseVal, &trueVal)
	vmInstance.MapIteratorPrototype = vm.NewValueFromPlainObject(mapIteratorProto)

	// %SetIteratorPrototype%
	setIteratorProto := vm.NewObject(vmInstance.IteratorPrototype).AsPlainObject()
	setIteratorProto.DefineOwnPropertyByKey(
		vm.NewSymbolKey(SymbolToStringTag),
		vm.NewString("Set Iterator"),
		&falseVal, // writable: false
		&falseVal, // enumerable: false
		&trueVal,  // configurable: true
	)
	// %SetIteratorPrototype%.next - defined on prototype with proper length/name
	setIterNextFn := vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// Check for internal slots (branding)
		if !thisVal.IsObject() {
			return vm.Undefined, vmInstance.NewTypeError("%SetIteratorPrototype%.next requires that 'this' be an Object")
		}
		thisObj := thisVal.AsPlainObject()
		if thisObj == nil {
			return vm.Undefined, vmInstance.NewTypeError("%SetIteratorPrototype%.next requires that 'this' be an Object")
		}
		// Check for [[IteratedSet]] internal slot
		iteratedSet, hasSlot := thisObj.GetOwn("[[IteratedSet]]")
		if !hasSlot {
			return vm.Undefined, vmInstance.NewTypeError("%SetIteratorPrototype%.next requires that 'this' be a Set Iterator")
		}

		result := vm.NewObject(vm.Undefined).AsPlainObject()

		// Check if exhausted
		if exhaustedVal, ok := thisObj.GetOwn("[[Exhausted]]"); ok && exhaustedVal == vm.True {
			result.SetOwn("value", vm.Undefined)
			result.SetOwn("done", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(result), nil
		}

		// Get current index and kind
		indexVal, _ := thisObj.GetOwn("[[SetNextIndex]]")
		kindVal, _ := thisObj.GetOwn("[[SetIterationKind]]")
		currentIndex := int(indexVal.ToFloat())
		kind := kindVal.ToString() // "entries", "keys", or "values"

		// Get the set
		if iteratedSet.Type() != vm.TypeSet {
			result.SetOwn("value", vm.Undefined)
			result.SetOwn("done", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(result), nil
		}
		setObj := iteratedSet.AsSet()

		// Iterate
		for currentIndex < setObj.OrderLen() {
			value, exists := setObj.GetValueAt(currentIndex)
			currentIndex++
			thisObj.SetOwn("[[SetNextIndex]]", vm.NumberValue(float64(currentIndex)))
			if exists {
				var resultValue vm.Value
				switch kind {
				case "entries":
					// For Set entries(), return [value, value]
					entry := vm.NewArray()
					entry.AsArray().Append(value)
					entry.AsArray().Append(value)
					resultValue = entry
				case "keys", "values":
					resultValue = value
				default:
					resultValue = value
				}
				result.SetOwn("value", resultValue)
				result.SetOwn("done", vm.BooleanValue(false))
				return vm.NewValueFromPlainObject(result), nil
			}
		}

		// Exhausted
		thisObj.SetOwn("[[Exhausted]]", vm.BooleanValue(true))
		thisObj.SetOwn("[[SetNextIndex]]", vm.NumberValue(float64(currentIndex)))
		result.SetOwn("value", vm.Undefined)
		result.SetOwn("done", vm.BooleanValue(true))
		return vm.NewValueFromPlainObject(result), nil
	})
	setIteratorProto.DefineOwnProperty("next", setIterNextFn, &trueVal, &falseVal, &trueVal)
	vmInstance.SetIteratorPrototype = vm.NewValueFromPlainObject(setIteratorProto)

	// %StringIteratorPrototype%
	stringIteratorProto := vm.NewObject(vmInstance.IteratorPrototype).AsPlainObject()
	stringIteratorProto.DefineOwnPropertyByKey(
		vm.NewSymbolKey(SymbolToStringTag),
		vm.NewString("String Iterator"),
		&falseVal, // writable: false
		&falseVal, // enumerable: false
		&trueVal,  // configurable: true
	)
	vmInstance.StringIteratorPrototype = vm.NewValueFromPlainObject(stringIteratorProto)

	// %RegExpStringIteratorPrototype%
	regexpStringIteratorProto := vm.NewObject(vmInstance.IteratorPrototype).AsPlainObject()
	regexpStringIteratorProto.DefineOwnPropertyByKey(
		vm.NewSymbolKey(SymbolToStringTag),
		vm.NewString("RegExp String Iterator"),
		&falseVal, // writable: false
		&falseVal, // enumerable: false
		&trueVal,  // configurable: true
	)
	vmInstance.RegExpStringIteratorPrototype = vm.NewValueFromPlainObject(regexpStringIteratorProto)

	// ============================================
	// Create Iterator constructor
	// ============================================
	iteratorCtor := vm.NewConstructorWithProps(0, true, "Iterator", func(args []vm.Value) (vm.Value, error) {
		// Iterator is abstract - per ECMAScript spec, calling Iterator() directly
		// returns a new object with Iterator.prototype. Subclasses extend this.
		obj := vm.NewObject(vmInstance.IteratorPrototype).AsPlainObject()
		return vm.NewValueFromPlainObject(obj), nil
	})

	// Set Iterator.prototype with { writable: false, enumerable: false, configurable: false }
	ctorProps := iteratorCtor.AsNativeFunctionWithProps()
	if ctorProps != nil && ctorProps.Properties != nil {
		w, e, c := false, false, false
		ctorProps.Properties.DefineOwnProperty("prototype", vmInstance.IteratorPrototype, &w, &e, &c)

		// Add Iterator.from() static method
		ctorProps.Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
			if len(args) == 0 {
				return vm.Undefined, vmInstance.NewTypeError("Iterator.from requires an argument")
			}
			obj := args[0]

			// Check if already an Iterator instance
			if obj.IsObject() || obj.IsGenerator() {
				// Check prototype chain
				if obj.IsGenerator() {
					// Generators are already iterators, return as-is
					return obj, nil
				}

				// Check if it inherits from Iterator.prototype
				proto := vmInstance.IteratorPrototype
				current := obj
				for {
					currentProto, err := vmInstance.GetProperty(current, "__proto__")
					if err != nil || currentProto.IsUndefined() || currentProto.Type() == vm.TypeNull {
						break
					}
					if currentProto.Equals(proto) {
						// Already an Iterator, return as-is
						return obj, nil
					}
					current = currentProto
				}
			}

			// Try to get iterator from object
			var iterator vm.Value
			if obj.IsObject() || obj.Type() == vm.TypeArray || obj.Type() == vm.TypeSet || obj.Type() == vm.TypeMap {
				// Try Symbol.iterator
				if iterMethod, ok := vmInstance.GetSymbolProperty(obj, SymbolIterator); ok && iterMethod.IsCallable() {
					iter, err := vmInstance.Call(iterMethod, obj, []vm.Value{})
					if err != nil {
						return vm.Undefined, err
					}
					iterator = iter
				}
			}

			// If no iterator from Symbol.iterator, check if it has next()
			if iterator.IsUndefined() {
				nextMethod, _ := vmInstance.GetProperty(obj, "next")
				if nextMethod.IsCallable() {
					iterator = obj
				} else {
					return vm.Undefined, vmInstance.NewTypeError("Object is not iterable")
				}
			}

			// Wrap the iterator
			wrapper := vm.NewObject(vmInstance.WrapForValidIteratorPrototype).AsPlainObject()
			wrapper.SetOwn("[[Iterated]]", iterator)
			return vm.NewValueFromPlainObject(wrapper), nil
		}))

		// Add Iterator.concat(...items) static method
		ctorProps.Properties.SetOwnNonEnumerable("concat", vm.NewNativeFunction(0, true, "concat", func(args []vm.Value) (vm.Value, error) {
			// Collect all iterables from arguments
			var iterables []vm.Value

			for _, item := range args {
				// Per spec: "If item is not an Object, throw a TypeError exception."
				// This means primitives (string, number, boolean, etc.) are not allowed,
				// even if they are iterable.
				if !item.IsObject() && item.Type() != vm.TypeArray && item.Type() != vm.TypeSet && item.Type() != vm.TypeMap && !item.IsGenerator() {
					return vm.Undefined, vmInstance.NewTypeError("Iterator.concat requires an Object argument")
				}

				// Now check if item has Symbol.iterator
				var hasIterator bool
				if item.Type() == vm.TypeArray || item.Type() == vm.TypeSet || item.Type() == vm.TypeMap {
					hasIterator = true
				} else if item.IsObject() || item.IsGenerator() {
					if iterMethod, ok := vmInstance.GetSymbolProperty(item, SymbolIterator); ok && iterMethod.IsCallable() {
						hasIterator = true
					}
				}

				if !hasIterator {
					return vm.Undefined, vmInstance.NewTypeError("Iterator.concat: argument is not iterable")
				}

				iterables = append(iterables, item)
			}

			// Create the concat iterator
			concatIter := vm.NewObject(vmInstance.IteratorHelperPrototype).AsPlainObject()
			currentIterableIndex := 0
			var currentIterator vm.Value = vm.Undefined
			closed := false

			// Helper to get iterator from value
			getIteratorFromValue := func(value vm.Value) (vm.Value, error) {
				// For strings, use Symbol.iterator which creates a string iterator
				if iterMethod, ok := vmInstance.GetSymbolProperty(value, SymbolIterator); ok && iterMethod.IsCallable() {
					return vmInstance.Call(iterMethod, value, []vm.Value{})
				}
				return vm.Undefined, vmInstance.NewTypeError("Value is not iterable")
			}

			concatIter.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(innerArgs []vm.Value) (vm.Value, error) {
				if closed {
					return createIteratorResult(vm.Undefined, true), nil
				}

				for {
					// If we have a current iterator, try to get next value
					if !currentIterator.IsUndefined() {
						nextMethod, _ := vmInstance.GetProperty(currentIterator, "next")
						if nextMethod.IsCallable() {
							result, err := vmInstance.Call(nextMethod, currentIterator, []vm.Value{})
							if err != nil {
								return vm.Undefined, err
							}
							doneVal, _ := vmInstance.GetProperty(result, "done")
							if !doneVal.IsTruthy() {
								// Return the value
								valueVal, _ := vmInstance.GetProperty(result, "value")
								return createIteratorResult(valueVal, false), nil
							}
							// Current iterator exhausted, move to next
							currentIterator = vm.Undefined
						}
					}

					// Move to next iterable
					if currentIterableIndex >= len(iterables) {
						// All iterables exhausted
						return createIteratorResult(vm.Undefined, true), nil
					}

					// Get iterator for next iterable
					iter, err := getIteratorFromValue(iterables[currentIterableIndex])
					if err != nil {
						return vm.Undefined, err
					}
					currentIterator = iter
					currentIterableIndex++
				}
			}))

			// Add return method to close the iterator
			concatIter.SetOwnNonEnumerable("return", vm.NewNativeFunction(0, false, "return", func(innerArgs []vm.Value) (vm.Value, error) {
				if !closed {
					closed = true
					// Close the current underlying iterator if any
					if !currentIterator.IsUndefined() {
						returnMethod, _ := vmInstance.GetProperty(currentIterator, "return")
						if returnMethod.IsCallable() {
							_, _ = vmInstance.Call(returnMethod, currentIterator, []vm.Value{})
						}
						currentIterator = vm.Undefined
					}
				}
				return createIteratorResult(vm.Undefined, true), nil
			}))

			// Add Symbol.iterator that returns self
			iterSelfFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(fnArgs []vm.Value) (vm.Value, error) {
				return vm.NewValueFromPlainObject(concatIter), nil
			})
			concatIter.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterSelfFn, nil, nil, nil)

			return vm.NewValueFromPlainObject(concatIter), nil
		}))

		// Add Iterator.zip(iterables, options) static method
		ctorProps.Properties.SetOwnNonEnumerable("zip", vm.NewNativeFunction(1, false, "zip", func(args []vm.Value) (vm.Value, error) {
			if len(args) == 0 {
				return vm.Undefined, vmInstance.NewTypeError("Iterator.zip requires an iterable argument")
			}

			iterablesArg := args[0]

			// Parse options
			mode := "shortest" // default
			var padding []vm.Value

			if len(args) > 1 && !args[1].IsUndefined() {
				options := args[1]
				if options.IsObject() {
					// Get mode - must be undefined or a valid string (no coercion)
					modeVal, _ := vmInstance.GetProperty(options, "mode")
					if !modeVal.IsUndefined() {
						// Mode must be a string type, not coerced
						if modeVal.Type() != vm.TypeString {
							return vm.Undefined, vmInstance.NewTypeError("Iterator.zip: mode must be a string")
						}
						modeStr := modeVal.ToString()
						if modeStr != "shortest" && modeStr != "longest" && modeStr != "strict" {
							return vm.Undefined, vmInstance.NewTypeError("Iterator.zip: mode must be 'shortest', 'longest', or 'strict'")
						}
						mode = modeStr
					}
					// Get padding for "longest" mode
					if mode == "longest" {
						paddingVal, _ := vmInstance.GetProperty(options, "padding")
						if !paddingVal.IsUndefined() {
							// Convert padding iterable to array
							if iterMethod, ok := vmInstance.GetSymbolProperty(paddingVal, SymbolIterator); ok && iterMethod.IsCallable() {
								padIter, err := vmInstance.Call(iterMethod, paddingVal, []vm.Value{})
								if err != nil {
									return vm.Undefined, err
								}
								for {
									nextMethod, _ := vmInstance.GetProperty(padIter, "next")
									result, err := vmInstance.Call(nextMethod, padIter, []vm.Value{})
									if err != nil {
										break
									}
									doneVal, _ := vmInstance.GetProperty(result, "done")
									if doneVal.IsTruthy() {
										break
									}
									valueVal, _ := vmInstance.GetProperty(result, "value")
									padding = append(padding, valueVal)
								}
							}
						}
					}
				}
			}

			// Helper: GetIteratorFlattenable - gets iterator from object
			// Accepts both iterables (with Symbol.iterator) and iterators (with next method)
			getIteratorFlattenable := func(obj vm.Value) (vm.Value, error) {
				// Step 1: If obj is not an Object, throw TypeError
				if !obj.IsObject() && obj.Type() != vm.TypeArray && obj.Type() != vm.TypeSet && obj.Type() != vm.TypeMap && !obj.IsGenerator() {
					return vm.Undefined, vmInstance.NewTypeError("Iterator.zip: value is not an object")
				}

				// Step 2: Try to get Symbol.iterator method
				if iterMethod, ok := vmInstance.GetSymbolProperty(obj, SymbolIterator); ok && iterMethod.IsCallable() {
					// Step 4: Call the method to get iterator
					iter, err := vmInstance.Call(iterMethod, obj, []vm.Value{})
					if err != nil {
						return vm.Undefined, err
					}
					// Step 5: Result must be an object
					if !iter.IsObject() && !iter.IsGenerator() {
						return vm.Undefined, vmInstance.NewTypeError("Iterator.zip: Symbol.iterator did not return an object")
					}
					return iter, nil
				}

				// Step 3: If method is undefined, use obj as iterator directly
				// (it should have a next method)
				nextMethod, _ := vmInstance.GetProperty(obj, "next")
				if nextMethod.IsCallable() {
					return obj, nil
				}

				return vm.Undefined, vmInstance.NewTypeError("Iterator.zip: value is not iterable")
			}

			// Collect iterators from the iterables argument
			var iterators []vm.Value

			// Get iterator for the iterables argument itself (using GetIterator, not GetIteratorFlattenable)
			var iterablesIter vm.Value
			if iterMethod, ok := vmInstance.GetSymbolProperty(iterablesArg, SymbolIterator); ok && iterMethod.IsCallable() {
				iter, err := vmInstance.Call(iterMethod, iterablesArg, []vm.Value{})
				if err != nil {
					return vm.Undefined, err
				}
				iterablesIter = iter
			} else {
				return vm.Undefined, vmInstance.NewTypeError("Iterator.zip: first argument is not iterable")
			}

			// Collect all iterators from iterables using GetIteratorFlattenable
			for {
				nextMethod, _ := vmInstance.GetProperty(iterablesIter, "next")
				result, err := vmInstance.Call(nextMethod, iterablesIter, []vm.Value{})
				if err != nil {
					return vm.Undefined, err
				}
				doneVal, _ := vmInstance.GetProperty(result, "done")
				if doneVal.IsTruthy() {
					break
				}
				item, _ := vmInstance.GetProperty(result, "value")
				// Use GetIteratorFlattenable for each item
				itemIter, err := getIteratorFlattenable(item)
				if err != nil {
					return vm.Undefined, err
				}
				iterators = append(iterators, itemIter)
			}

			// Create the zip iterator
			zipIter := vm.NewObject(vmInstance.IteratorHelperPrototype).AsPlainObject()
			exhausted := make([]bool, len(iterators))
			allDone := false
			closed := false

			// Helper to close all non-exhausted iterators
			closeAllIterators := func() {
				for i, iter := range iterators {
					if !exhausted[i] {
						returnMethod, _ := vmInstance.GetProperty(iter, "return")
						if returnMethod.IsCallable() {
							_, _ = vmInstance.Call(returnMethod, iter, []vm.Value{})
						}
						exhausted[i] = true
					}
				}
			}

			zipIter.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(innerArgs []vm.Value) (vm.Value, error) {
				if allDone || closed {
					return createIteratorResult(vm.Undefined, true), nil
				}

				results := make([]vm.Value, len(iterators))
				anyDone := false
				allExhausted := true

				for i, iter := range iterators {
					if exhausted[i] {
						// Already exhausted, use padding
						if i < len(padding) {
							results[i] = padding[i]
						} else {
							results[i] = vm.Undefined
						}
						continue
					}

					allExhausted = false

					nextMethod, _ := vmInstance.GetProperty(iter, "next")
					result, err := vmInstance.Call(nextMethod, iter, []vm.Value{})
					if err != nil {
						return vm.Undefined, err
					}

					doneVal, _ := vmInstance.GetProperty(result, "done")
					if doneVal.IsTruthy() {
						exhausted[i] = true
						anyDone = true
						// Use padding for this position
						if i < len(padding) {
							results[i] = padding[i]
						} else {
							results[i] = vm.Undefined
						}
					} else {
						valueVal, _ := vmInstance.GetProperty(result, "value")
						results[i] = valueVal
					}
				}

				// Handle modes
				switch mode {
				case "shortest":
					if anyDone {
						allDone = true
						return createIteratorResult(vm.Undefined, true), nil
					}
				case "strict":
					if anyDone {
						// Check if ALL are done
						allAreDone := true
						for _, ex := range exhausted {
							if !ex {
								allAreDone = false
								break
							}
						}
						if !allAreDone {
							allDone = true
							return vm.Undefined, vmInstance.NewTypeError("Iterator.zip: iterators have different lengths in strict mode")
						}
						allDone = true
						return createIteratorResult(vm.Undefined, true), nil
					}
				case "longest":
					// Check if all iterators were already exhausted at the start
					if allExhausted {
						allDone = true
						return createIteratorResult(vm.Undefined, true), nil
					}
					// Also check if all iterators became exhausted during this iteration
					// (this handles the case where the last remaining iterators all finish together)
					nowAllExhausted := true
					for _, ex := range exhausted {
						if !ex {
							nowAllExhausted = false
							break
						}
					}
					if nowAllExhausted {
						allDone = true
						return createIteratorResult(vm.Undefined, true), nil
					}
				}

				// Create result array
				resultArr := vm.NewArray()
				for _, v := range results {
					resultArr.AsArray().Append(v)
				}
				return createIteratorResult(resultArr, false), nil
			}))

			// Add return method to close all iterators
			zipIter.SetOwnNonEnumerable("return", vm.NewNativeFunction(0, false, "return", func(innerArgs []vm.Value) (vm.Value, error) {
				if !closed {
					closed = true
					closeAllIterators()
				}
				return createIteratorResult(vm.Undefined, true), nil
			}))

			// Add Symbol.iterator that returns self
			iterSelfFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(fnArgs []vm.Value) (vm.Value, error) {
				return vm.NewValueFromPlainObject(zipIter), nil
			})
			zipIter.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterSelfFn, nil, nil, nil)

			return vm.NewValueFromPlainObject(zipIter), nil
		}))
	}

	// Add Iterator.zipKeyed static method
	ctorProps.Properties.SetOwnNonEnumerable("zipKeyed", vm.NewNativeFunction(1, false, "zipKeyed", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.zipKeyed: expected object argument")
		}

		iterablesObj := args[0]
		if !iterablesObj.IsObject() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.zipKeyed: first argument must be an object")
		}

		// Parse options (same as zip)
		mode := "shortest"
		var padding map[string]vm.Value

		if len(args) >= 2 && args[1].IsObject() {
			optsObj := args[1]
			// Get mode option - must be undefined or a valid string (no coercion)
			modeVal, _ := vmInstance.GetProperty(optsObj, "mode")
			if !modeVal.IsUndefined() {
				// Mode must be a string type, not coerced
				if modeVal.Type() != vm.TypeString {
					return vm.Undefined, vmInstance.NewTypeError("Iterator.zipKeyed: mode must be a string")
				}
				modeStr := modeVal.ToString()
				if modeStr != "shortest" && modeStr != "longest" && modeStr != "strict" {
					return vm.Undefined, vmInstance.NewTypeError("Iterator.zipKeyed: mode must be 'shortest', 'longest', or 'strict'")
				}
				mode = modeStr
			}
			// Get padding option (for "longest" mode)
			paddingVal, _ := vmInstance.GetProperty(optsObj, "padding")
			if !paddingVal.IsUndefined() && paddingVal.IsObject() {
				padding = make(map[string]vm.Value)
				// Extract padding values keyed by property name
				if plainObj := paddingVal.AsPlainObject(); plainObj != nil {
					for _, key := range plainObj.OwnKeys() {
						if val, exists := plainObj.GetOwn(key); exists {
							padding[key] = val
						}
					}
				}
			}
		}

		// Helper: GetIteratorFlattenable - gets iterator from object
		// Accepts both iterables (with Symbol.iterator) and iterators (with next method)
		getIteratorFlattenable := func(obj vm.Value) (vm.Value, error) {
			// Step 1: If obj is not an Object, throw TypeError
			if !obj.IsObject() && obj.Type() != vm.TypeArray && obj.Type() != vm.TypeSet && obj.Type() != vm.TypeMap && !obj.IsGenerator() {
				return vm.Undefined, vmInstance.NewTypeError("Iterator.zipKeyed: value is not an object")
			}

			// Step 2: Try to get Symbol.iterator method
			if iterMethod, ok := vmInstance.GetSymbolProperty(obj, SymbolIterator); ok && iterMethod.IsCallable() {
				// Step 4: Call the method to get iterator
				iter, err := vmInstance.Call(iterMethod, obj, []vm.Value{})
				if err != nil {
					return vm.Undefined, err
				}
				// Step 5: Result must be an object
				if !iter.IsObject() && !iter.IsGenerator() {
					return vm.Undefined, vmInstance.NewTypeError("Iterator.zipKeyed: Symbol.iterator did not return an object")
				}
				return iter, nil
			}

			// Step 3: If method is undefined, use obj as iterator directly
			// (it should have a next method)
			nextMethod, _ := vmInstance.GetProperty(obj, "next")
			if nextMethod.IsCallable() {
				return obj, nil
			}

			return vm.Undefined, vmInstance.NewTypeError("Iterator.zipKeyed: value is not iterable")
		}

		// Collect keys and iterators from the iterables object
		var keys []string
		iterators := make(map[string]vm.Value)

		if plainObj := iterablesObj.AsPlainObject(); plainObj != nil {
			for _, key := range plainObj.OwnKeys() {
				keys = append(keys, key)
				item, _ := plainObj.GetOwn(key)

				// Use GetIteratorFlattenable for each item
				itemIter, err := getIteratorFlattenable(item)
				if err != nil {
					return vm.Undefined, err
				}
				iterators[key] = itemIter
			}
		} else {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.zipKeyed: first argument must be a plain object")
		}

			// Create the zipKeyed iterator
		zipKeyedIter := vm.NewObject(vmInstance.IteratorHelperPrototype).AsPlainObject()
		exhausted := make(map[string]bool)
		for _, k := range keys {
			exhausted[k] = false
		}
		allDone := false
		closed := false

		// Helper to close all non-exhausted iterators
		closeAllIterators := func() {
			for key, iter := range iterators {
				if !exhausted[key] {
					returnMethod, _ := vmInstance.GetProperty(iter, "return")
					if returnMethod.IsCallable() {
						_, _ = vmInstance.Call(returnMethod, iter, []vm.Value{})
					}
					exhausted[key] = true
				}
			}
		}

		zipKeyedIter.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(innerArgs []vm.Value) (vm.Value, error) {
			if allDone || closed {
				return createIteratorResult(vm.Undefined, true), nil
			}

				results := make(map[string]vm.Value)
				anyDone := false
				allExhausted := true

				for _, key := range keys {
					if exhausted[key] {
						// Already exhausted, use padding
						if padding != nil {
							if padVal, ok := padding[key]; ok {
								results[key] = padVal
							} else {
								results[key] = vm.Undefined
							}
						} else {
							results[key] = vm.Undefined
						}
						continue
					}

					allExhausted = false
					iter := iterators[key]

					nextMethod, _ := vmInstance.GetProperty(iter, "next")
					result, err := vmInstance.Call(nextMethod, iter, []vm.Value{})
					if err != nil {
						return vm.Undefined, err
					}

					doneVal, _ := vmInstance.GetProperty(result, "done")
					if doneVal.IsTruthy() {
						exhausted[key] = true
						anyDone = true
						// Use padding for this key
						if padding != nil {
							if padVal, ok := padding[key]; ok {
								results[key] = padVal
							} else {
								results[key] = vm.Undefined
							}
						} else {
							results[key] = vm.Undefined
						}
					} else {
						valueVal, _ := vmInstance.GetProperty(result, "value")
						results[key] = valueVal
					}
				}

				// Handle modes
				switch mode {
				case "shortest":
					if anyDone {
						allDone = true
						return createIteratorResult(vm.Undefined, true), nil
					}
				case "strict":
					if anyDone {
						// Check if ALL are done
						allAreDone := true
						for _, ex := range exhausted {
							if !ex {
								allAreDone = false
								break
							}
						}
						if !allAreDone {
							allDone = true
							return vm.Undefined, vmInstance.NewTypeError("Iterator.zipKeyed: iterators have different lengths in strict mode")
						}
						allDone = true
						return createIteratorResult(vm.Undefined, true), nil
					}
				case "longest":
					// Check if all iterators were already exhausted at the start
					if allExhausted {
						allDone = true
						return createIteratorResult(vm.Undefined, true), nil
					}
					// Also check if all iterators became exhausted during this iteration
					nowAllExhausted := true
					for _, ex := range exhausted {
						if !ex {
							nowAllExhausted = false
							break
						}
					}
					if nowAllExhausted {
						allDone = true
						return createIteratorResult(vm.Undefined, true), nil
					}
				}

				// Create result object
				resultObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				for _, key := range keys {
					resultObj.SetOwn(key, results[key])
				}
				return createIteratorResult(vm.NewValueFromPlainObject(resultObj), false), nil
			}))

		// Add return method to close all iterators
		zipKeyedIter.SetOwnNonEnumerable("return", vm.NewNativeFunction(0, false, "return", func(innerArgs []vm.Value) (vm.Value, error) {
			if !closed {
				closed = true
				closeAllIterators()
			}
			return createIteratorResult(vm.Undefined, true), nil
		}))

		// Add Symbol.iterator that returns self
		iterSelfFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(fnArgs []vm.Value) (vm.Value, error) {
			return vm.NewValueFromPlainObject(zipKeyedIter), nil
		})
		zipKeyedIter.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterSelfFn, nil, nil, nil)

		return vm.NewValueFromPlainObject(zipKeyedIter), nil
	}))

	// Set constructor property on Iterator.prototype - accessor property
	// Per spec: { get: function, set: function, enumerable: false, configurable: true }
	constructorGetter := vm.NewNativeFunction(0, false, "get constructor", func(args []vm.Value) (vm.Value, error) {
		return iteratorCtor, nil
	})
	constructorSetter := vm.NewNativeFunction(1, false, "set constructor", func(args []vm.Value) (vm.Value, error) {
		// Setter does nothing per spec
		return vm.Undefined, nil
	})
	iteratorProto.DefineAccessorProperty("constructor", constructorGetter, true, constructorSetter, true, &e, &c)

	// Register Iterator constructor globally
	return ctx.DefineGlobal("Iterator", iteratorCtor)
}

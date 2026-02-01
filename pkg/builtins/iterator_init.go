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
	_ = ctx.DefineGlobal("Iterator", iteratorGeneric)
	_ = ctx.DefineGlobal("Iterable", iterableGeneric)

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

	// ============================================
	// Create IteratorHelper prototype (%IteratorHelperPrototype%)
	// This is the prototype for iterator objects returned by map, filter, etc.
	// ============================================
	iteratorHelperProto := vm.NewObject(vm.NewValueFromPlainObject(iteratorProto)).AsPlainObject()

	// Add Symbol.toStringTag = "Iterator Helper" to IteratorHelperPrototype
	falseVal := false
	trueVal := true
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
	// Iterator.prototype[Symbol.toStringTag] = "Iterator"
	// ============================================
	iteratorProto.DefineOwnPropertyByKey(
		vm.NewSymbolKey(SymbolToStringTag),
		vm.NewString("Iterator"),
		&falseVal, // writable: false
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

		limit := 0
		if len(args) > 0 {
			limit = int(args[0].ToFloat())
		}
		if limit < 0 {
			return vm.Undefined, vmInstance.NewRangeError("Iterator.prototype.take limit must be non-negative")
		}

		// Create iterator helper object
		helper := vm.NewObject(vmInstance.IteratorHelperPrototype).AsPlainObject()
		helper.SetOwn("[[UnderlyingIterator]]", thisValue)
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

		count := 0
		if len(args) > 0 {
			count = int(args[0].ToFloat())
		}
		if count < 0 {
			return vm.Undefined, vmInstance.NewRangeError("Iterator.prototype.drop count must be non-negative")
		}

		// Create iterator helper object
		helper := vm.NewObject(vmInstance.IteratorHelperPrototype).AsPlainObject()
		helper.SetOwn("[[UnderlyingIterator]]", thisValue)
		helper.SetOwn("[[ToSkip]]", vm.NumberValue(float64(count)))

		// Add next method
		helper.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
			helperThis := vmInstance.GetThis()
			helperObj := helperThis.AsPlainObject()
			if helperObj == nil {
				return vm.Undefined, vmInstance.NewTypeError("next called on non-object")
			}

			underlyingIter, _ := helperObj.GetOwn("[[UnderlyingIterator]]")
			toSkipVal, _ := helperObj.GetOwn("[[ToSkip]]")
			toSkip := int(toSkipVal.ToFloat())

			// Skip values
			for toSkip > 0 {
				result, err := getIteratorNext(underlyingIter)
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
			result, err := getIteratorNext(underlyingIter)
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
	iteratorProto.SetOwnNonEnumerable("reduce", vm.NewNativeFunction(2, false, "reduce", func(args []vm.Value) (vm.Value, error) {
		thisValue := vmInstance.GetThis()
		if !thisValue.IsObject() && !thisValue.IsGenerator() {
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.reduce called on non-object")
		}

		if len(args) == 0 || !args[0].IsCallable() {
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
			return vm.Undefined, vmInstance.NewTypeError("Iterator.prototype.flatMap requires a callable argument")
		}
		mapper := args[0]

		// Create iterator helper object
		helper := vm.NewObject(vmInstance.IteratorHelperPrototype).AsPlainObject()
		helper.SetOwn("[[UnderlyingIterator]]", thisValue)
		helper.SetOwn("[[Mapper]]", mapper)
		helper.SetOwn("[[InnerIterator]]", vm.Undefined)
		helper.SetOwn("[[Counter]]", vm.NumberValue(0))

		// Helper to get iterator from value
		getIterator := func(value vm.Value) (vm.Value, error) {
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
	// Create Iterator constructor
	// ============================================
	iteratorCtor := vm.NewConstructorWithProps(0, true, "Iterator", func(args []vm.Value) (vm.Value, error) {
		// Iterator is abstract - per ECMAScript spec, calling Iterator() directly
		// returns a new object with Iterator.prototype. Subclasses extend this.
		obj := vm.NewObject(vmInstance.IteratorPrototype).AsPlainObject()
		return vm.NewValueFromPlainObject(obj), nil
	})

	// Set Iterator.prototype
	ctorProps := iteratorCtor.AsNativeFunctionWithProps()
	if ctorProps != nil && ctorProps.Properties != nil {
		ctorProps.Properties.SetOwn("prototype", vmInstance.IteratorPrototype)

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
	}

	// Set constructor property on Iterator.prototype
	iteratorProto.DefineOwnProperty("constructor", iteratorCtor, &w, &e, &c)

	// Register Iterator constructor globally
	return ctx.DefineGlobal("Iterator", iteratorCtor)
}

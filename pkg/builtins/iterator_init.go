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
	iteratorResultOfT := &types.InstantiatedType{
		Generic:       iteratorResultGeneric,
		TypeArguments: []types.Type{tType},
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
		WithProperty("next", types.NewSimpleFunction([]types.Type{}, iteratorResultOfT)).
		// return(value?: any): IteratorResult<T>
		WithProperty("return", types.NewOptionalFunction([]types.Type{types.Any}, iteratorResultOfT, []bool{true})).
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
	installIteratorHelperPrototype(vmInstance, iteratorHelperProto)

	// ============================================
	// Create WrapForValidIteratorPrototype
	// For Iterator.from() wrapped iterators
	// ============================================
	wrapForValidIteratorProto := vm.NewObject(vm.NewValueFromPlainObject(iteratorProto)).AsPlainObject()

	installWrapForValidIteratorPrototype(vmInstance, wrapForValidIteratorProto)

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
		// SetterThatIgnoresPrototypeProperties(%Iterator.prototype%, %Symbol.toStringTag%, v)
		thisVal := vmInstance.GetThis()
		// 1. If this is not an Object, throw TypeError
		if thisVal.Type() != vm.TypeObject {
			return vm.Undefined, vmInstance.NewTypeError("setter called on non-object")
		}
		// 2. If this is home, throw TypeError
		if thisVal.AsPlainObject() == iteratorProto {
			return vm.Undefined, vmInstance.NewTypeError("Cannot set Symbol.toStringTag on Iterator.prototype directly")
		}
		v := vm.Undefined
		if len(args) > 0 {
			v = args[0]
		}
		propKey := vm.NewSymbolKey(SymbolToStringTag)
		po := thisVal.AsPlainObject()
		// 3. Let desc = this.[[GetOwnProperty]](p)
		if _, hasOwn := po.GetOwnByKey(propKey); !hasOwn {
			// 4. If desc is undefined, CreateDataPropertyOrThrow(this, p, v)
			w, e, c := true, true, true
			po.DefineOwnPropertyByKey(propKey, v, &w, &e, &c)
		} else {
			// 5. Else, Set(this, p, v, true)
			po.DefineOwnPropertyByKey(propKey, v, nil, nil, nil)
		}
		return vm.Undefined, nil
	})
	iteratorProto.DefineAccessorPropertyByKey(
		vm.NewSymbolKey(SymbolToStringTag),
		toStringTagGetter, true,
		toStringTagSetter, true,
		&falseVal, // enumerable: false
		&trueVal,  // configurable: true
	)

	installIteratorPrototypeMethods(vmInstance, iteratorProto)

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
		if thisVal.Type() != vm.TypeObject {
			return vm.Undefined, vmInstance.NewTypeError("%MapIteratorPrototype%.next requires that 'this' be an Object")
		}
		thisObj := thisVal.AsPlainObject()
		// Brand check: the internal iterator state doubles as the spec's
		// [[IteratedMap]] internal slot (a Set iterator has Set-kind state
		// and must be rejected here).
		st := thisObj.InternalIterState()
		if st == nil || !st.Kind.IsMapKind() {
			return vm.Undefined, vmInstance.NewTypeError("%MapIteratorPrototype%.next requires that 'this' be a Map Iterator")
		}

		result := vm.NewObject(vm.Undefined).AsPlainObject()
		v, done := st.Step()
		result.SetOwn("value", v)
		result.SetOwn("done", vm.BooleanValue(done))
		return vm.NewValueFromPlainObject(result), nil
	})
	// Tag the shared next so the for-of fast path (OpIterFastCheck) can
	// recognize it; the sentinel kind tells OpFastIterNext to resolve the
	// real per-iterator state from the iterator object instead.
	mapIterNextFn.AsNativeFunction().IterState = &vm.BuiltinIterState{Kind: vm.IterKindStateOnIterator}
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
		if thisVal.Type() != vm.TypeObject {
			return vm.Undefined, vmInstance.NewTypeError("%SetIteratorPrototype%.next requires that 'this' be an Object")
		}
		thisObj := thisVal.AsPlainObject()
		// Brand check: the internal iterator state doubles as the spec's
		// [[IteratedSet]] internal slot (a Map iterator has Map-kind state
		// and must be rejected here).
		st := thisObj.InternalIterState()
		if st == nil || !st.Kind.IsSetKind() {
			return vm.Undefined, vmInstance.NewTypeError("%SetIteratorPrototype%.next requires that 'this' be a Set Iterator")
		}

		result := vm.NewObject(vm.Undefined).AsPlainObject()
		v, done := st.Step()
		result.SetOwn("value", v)
		result.SetOwn("done", vm.BooleanValue(done))
		return vm.NewValueFromPlainObject(result), nil
	})
	// Tag the shared next for the for-of fast path (see map next above).
	setIterNextFn.AsNativeFunction().IterState = &vm.BuiltinIterState{Kind: vm.IterKindStateOnIterator}
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
		// Iterator is abstract — `new Iterator()` returns a fresh object with
		// Iterator.prototype. For `class S extends Iterator {} new S()` we need
		// the instance to chain through S.prototype so subclass-added methods
		// (next, return, custom helpers) are reachable from instances.
		proto := vmInstance.IteratorPrototype
		if nt := vmInstance.GetNewTarget(); nt.Type() != vm.TypeUndefined {
			if p, gpErr := vmInstance.GetProperty(nt, "prototype"); gpErr == nil && p.IsObject() {
				proto = p
			}
		}
		obj := vm.NewObject(proto).AsPlainObject()
		return vm.NewValueFromPlainObject(obj), nil
	})

	// Set Iterator.prototype with { writable: false, enumerable: false, configurable: false }
	ctorProps := iteratorCtor.AsNativeFunctionWithProps()
	if ctorProps != nil && ctorProps.Properties != nil {
		w, e, c := false, false, false
		ctorProps.Properties.DefineOwnProperty("prototype", vmInstance.IteratorPrototype, &w, &e, &c)

		ctorProps.Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
			return iteratorFrom(vmInstance, iteratorCtor, args)
		}))

		installIteratorStatics(vmInstance, ctorProps.Properties)
	}

	// Set constructor property on Iterator.prototype - accessor property
	// Per spec: { get: function, set: function, enumerable: false, configurable: true }
	constructorGetter := vm.NewNativeFunction(0, false, "get constructor", func(args []vm.Value) (vm.Value, error) {
		return iteratorCtor, nil
	})
	constructorSetter := vm.NewNativeFunction(1, false, "set constructor", func(args []vm.Value) (vm.Value, error) {
		// SetterThatIgnoresPrototypeProperties(%Iterator.prototype%, "constructor", v)
		thisVal := vmInstance.GetThis()
		// 1. If this is not an Object, throw TypeError
		if thisVal.Type() != vm.TypeObject {
			return vm.Undefined, vmInstance.NewTypeError("setter called on non-object")
		}
		// 2. If this is home, throw TypeError
		if thisVal.AsPlainObject() == iteratorProto {
			return vm.Undefined, vmInstance.NewTypeError("Cannot set constructor on Iterator.prototype directly")
		}
		v := vm.Undefined
		if len(args) > 0 {
			v = args[0]
		}
		po := thisVal.AsPlainObject()
		// 3. Let desc = this.[[GetOwnProperty]](p)
		if _, hasOwn := po.GetOwn("constructor"); !hasOwn {
			// 4. If desc is undefined, CreateDataPropertyOrThrow(this, p, v)
			w, e, c := true, true, true
			po.DefineOwnProperty("constructor", v, &w, &e, &c)
		} else {
			// 5. Else, Set(this, p, v, true)
			po.SetOwn("constructor", v)
		}
		return vm.Undefined, nil
	})
	iteratorProto.DefineAccessorProperty("constructor", constructorGetter, true, constructorSetter, true, &e, &c)

	// Register Iterator constructor globally
	return ctx.DefineGlobal("Iterator", iteratorCtor)
}

package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type MapInitializer struct{}

func (m *MapInitializer) Name() string {
	return "Map"
}

func (m *MapInitializer) Priority() int {
	return 400 // After Number (350)
}

func (m *MapInitializer) InitTypes(ctx *TypeContext) error {
	// Create generic type parameters K, V for map methods
	kParam := &types.TypeParameter{Name: "K", Constraint: nil, Index: 0}
	vParam := &types.TypeParameter{Name: "V", Constraint: nil, Index: 1}
	kType := &types.TypeParameterType{Parameter: kParam}
	vType := &types.TypeParameterType{Parameter: vParam}

	// Create the generic type first (with placeholder body)
	mapType := &types.GenericType{
		Name:           "Map",
		TypeParameters: []*types.TypeParameter{kParam, vParam},
		Body:           nil, // Will be set below
	}

	// Create Map instance type with methods (using type parameters directly)
	mapInstanceType := types.NewObjectType().
		WithProperty("set", types.NewSimpleFunction([]types.Type{kType, vType}, mapType)). // Return this for chaining
		WithProperty("get", types.NewSimpleFunction([]types.Type{kType}, types.NewUnionType(vType, types.Undefined))).
		WithProperty("has", types.NewSimpleFunction([]types.Type{kType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{kType}, types.Boolean)).
		WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Void)).
		WithProperty("size", types.Number)

	// Now set the body of the generic type
	mapType.Body = mapInstanceType

	// Create Map.prototype type for runtime (same structure)
	mapProtoType := types.NewObjectType().
		WithProperty("set", types.NewSimpleFunction([]types.Type{kType, vType}, mapType)). // Return this for chaining
		WithProperty("get", types.NewSimpleFunction([]types.Type{kType}, types.NewUnionType(vType, types.Undefined))).
		WithProperty("has", types.NewSimpleFunction([]types.Type{kType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{kType}, types.Boolean)).
		WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Void)).
		WithProperty("size", types.Number)

	// Register map primitive prototype
	ctx.SetPrimitivePrototype("map", mapProtoType)

	// Create Map constructor type - use an ObjectType with call signature and static methods
	mapCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, mapType). // new Map()
		// Map.groupBy(items, callbackfn)
		WithProperty("groupBy", types.NewSimpleFunction(
			[]types.Type{
				types.Any, // items: Iterable<T>
				types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Any), // callbackfn: (value: T, index: number) => K
			},
			mapType, // Map<K, T[]>
		))

	// Define Map constructor in global environment
	err := ctx.DefineGlobal("Map", mapCtorType)
	if err != nil {
		return err
	}

	// Also define the type alias for type annotations like Map<string, number>
	return ctx.DefineTypeAlias("Map", mapType)
}

func (m *MapInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Map.prototype inheriting from Object.prototype
	mapProto := vm.NewObject(objectProto).AsPlainObject()

	// Add Map prototype methods
	mapProto.SetOwnNonEnumerable("set", vm.NewNativeFunction(2, false, "set", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()

		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.set called on incompatible receiver")
		}

		if len(args) < 2 {
			return thisMap, nil // Return the map for chaining
		}

		mapObj := thisMap.AsMap()
		mapObj.Set(args[0], args[1])
		return thisMap, nil // Return the map for chaining
	}))
	// Ensure method attributes: writable: true, enumerable: false, configurable: true
	if v, ok := mapProto.GetOwn("set"); ok {
		w, e, c := true, false, true
		mapProto.DefineOwnProperty("set", v, &w, &e, &c)
	}

	mapProto.SetOwnNonEnumerable("get", vm.NewNativeFunction(1, false, "get", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()

		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.get called on incompatible receiver")
		}

		if len(args) < 1 {
			return vm.Undefined, nil
		}

		mapObj := thisMap.AsMap()
		return mapObj.Get(args[0]), nil
	}))
	if v, ok := mapProto.GetOwn("get"); ok {
		w, e, c := true, false, true
		mapProto.DefineOwnProperty("get", v, &w, &e, &c)
	}

	mapProto.SetOwnNonEnumerable("has", vm.NewNativeFunction(1, false, "has", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()

		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.has called on incompatible receiver")
		}

		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}

		mapObj := thisMap.AsMap()
		return vm.BooleanValue(mapObj.Has(args[0])), nil
	}))
	if v, ok := mapProto.GetOwn("has"); ok {
		w, e, c := true, false, true
		mapProto.DefineOwnProperty("has", v, &w, &e, &c)
	}

	mapProto.SetOwnNonEnumerable("delete", vm.NewNativeFunction(1, false, "delete", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()

		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.delete called on incompatible receiver")
		}

		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}

		mapObj := thisMap.AsMap()
		return vm.BooleanValue(mapObj.Delete(args[0])), nil
	}))
	if v, ok := mapProto.GetOwn("delete"); ok {
		w, e, c := true, false, true
		mapProto.DefineOwnProperty("delete", v, &w, &e, &c)
	}

	mapProto.SetOwnNonEnumerable("clear", vm.NewNativeFunction(0, false, "clear", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()

		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.clear called on incompatible receiver")
		}

		mapObj := thisMap.AsMap()
		mapObj.Clear()
		return vm.Undefined, nil
	}))
	if v, ok := mapProto.GetOwn("clear"); ok {
		w, e, c := true, false, true
		mapProto.DefineOwnProperty("clear", v, &w, &e, &c)
	}

	// forEach(callback[, thisArg]) - calls callback(value, key, map) for each entry
	mapProto.SetOwnNonEnumerable("forEach", vm.NewNativeFunction(1, false, "forEach", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()

		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.forEach called on incompatible receiver")
		}

		if len(args) < 1 || !args[0].IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("callback is not a function")
		}

		callback := args[0]
		thisArg := vm.Undefined
		if len(args) >= 2 {
			thisArg = args[1]
		}

		mapObj := thisMap.AsMap()
		// Iterate manually so we can propagate callback errors
		for i := 0; i < mapObj.OrderLen(); i++ {
			key, value, exists := mapObj.GetEntryAt(i)
			if exists {
				// Call callback(value, key, map) with thisArg as 'this'
				_, err := vmInstance.Call(callback, thisArg, []vm.Value{value, key, thisMap})
				if err != nil {
					return vm.Undefined, err
				}
			}
		}

		return vm.Undefined, nil
	}))
	if v, ok := mapProto.GetOwn("forEach"); ok {
		w, e, c := true, false, true
		mapProto.DefineOwnProperty("forEach", v, &w, &e, &c)
	}

	// getOrInsert(key, value) - returns existing value if key present, otherwise inserts and returns value
	mapProto.SetOwnNonEnumerable("getOrInsert", vm.NewNativeFunction(2, false, "getOrInsert", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()

		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.getOrInsert called on incompatible receiver")
		}

		if len(args) < 1 {
			return vm.Undefined, nil
		}

		key := args[0]
		value := vm.Undefined
		if len(args) >= 2 {
			value = args[1]
		}

		mapObj := thisMap.AsMap()

		// Canonicalize key: -0 -> +0
		key = vm.CanonicalizeKeyedCollectionKey(key)

		// Check if key already exists
		if existing := mapObj.Get(key); !existing.IsUndefined() || mapObj.Has(key) {
			return existing, nil
		}

		// Insert and return the value
		mapObj.Set(key, value)
		return value, nil
	}))
	if v, ok := mapProto.GetOwn("getOrInsert"); ok {
		w, e, c := true, false, true
		mapProto.DefineOwnProperty("getOrInsert", v, &w, &e, &c)
	}

	// getOrInsertComputed(key, callbackfn) - returns existing value or calls callback to compute value
	mapProto.SetOwnNonEnumerable("getOrInsertComputed", vm.NewNativeFunction(2, false, "getOrInsertComputed", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()

		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.getOrInsertComputed called on incompatible receiver")
		}

		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.getOrInsertComputed requires 2 arguments")
		}

		key := args[0]
		callbackfn := args[1]

		if !callbackfn.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.getOrInsertComputed: callback is not a function")
		}

		mapObj := thisMap.AsMap()

		// Canonicalize key: -0 -> +0
		key = vm.CanonicalizeKeyedCollectionKey(key)

		// Check if key already exists
		if existing := mapObj.Get(key); !existing.IsUndefined() || mapObj.Has(key) {
			return existing, nil
		}

		// Call callback with the key to compute the value
		value, err := vmInstance.Call(callbackfn, key, []vm.Value{key})
		if err != nil {
			return vm.Undefined, err
		}

		// Insert and return the computed value
		mapObj.Set(key, value)
		return value, nil
	}))
	if v, ok := mapProto.GetOwn("getOrInsertComputed"); ok {
		w, e, c := true, false, true
		mapProto.DefineOwnProperty("getOrInsertComputed", v, &w, &e, &c)
	}

	// Add size accessor (getter) - must be defined as an accessor property per spec
	sizeGetter := vm.NewNativeFunction(0, false, "get size", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.size called on incompatible receiver")
		}
		mapObj := thisMap.AsMap()
		return vm.IntegerValue(int32(mapObj.Size())), nil
	})
	e, c := false, true
	mapProto.DefineAccessorProperty("size", sizeGetter, true, vm.Undefined, false, &e, &c)

	// Live iterator for Map.prototype.entries() - respects deletions during iteration
	// Per ECMAScript spec: Map.prototype[@@iterator] === Map.prototype.entries
	entriesFn := vm.NewNativeFunction(0, false, "entries", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.entries called on incompatible receiver")
		}

		// Create iterator object with internal slots, inheriting from MapIteratorPrototype
		// The prototype's next() method uses these slots for iteration
		it := vm.NewObject(vmInstance.MapIteratorPrototype).AsPlainObject()
		it.SetOwn("[[IteratedMap]]", thisMap)
		it.SetOwn("[[MapNextIndex]]", vm.NumberValue(0))
		it.SetOwn("[[MapIterationKind]]", vm.NewString("entries"))
		it.SetOwn("[[Exhausted]]", vm.BooleanValue(false))

		// [Symbol.iterator]() { return this }
		it.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(a []vm.Value) (vm.Value, error) {
			return vm.NewValueFromPlainObject(it), nil
		}), nil, nil, nil)
		return vm.NewValueFromPlainObject(it), nil
	})
	// Set entries and Symbol.iterator to the SAME function object per ECMAScript spec
	mapProto.SetOwnNonEnumerable("entries", entriesFn)
	{
		w, e, c := true, false, true
		mapProto.DefineOwnProperty("entries", entriesFn, &w, &e, &c)
	}
	{
		wb, eb, cb := true, false, true
		mapProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), entriesFn, &wb, &eb, &cb)
	}
	// Live iterator for Map.prototype.values() - respects deletions during iteration
	mapProto.SetOwnNonEnumerable("values", vm.NewNativeFunction(0, false, "values", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.values called on incompatible receiver")
		}

		// Create iterator object with internal slots, inheriting from MapIteratorPrototype
		it := vm.NewObject(vmInstance.MapIteratorPrototype).AsPlainObject()
		it.SetOwn("[[IteratedMap]]", thisMap)
		it.SetOwn("[[MapNextIndex]]", vm.NumberValue(0))
		it.SetOwn("[[MapIterationKind]]", vm.NewString("values"))
		it.SetOwn("[[Exhausted]]", vm.BooleanValue(false))

		it.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(a []vm.Value) (vm.Value, error) {
			return vm.NewValueFromPlainObject(it), nil
		}), nil, nil, nil)
		return vm.NewValueFromPlainObject(it), nil
	}))
	if v, ok := mapProto.GetOwn("values"); ok {
		w, e, c := true, false, true
		mapProto.DefineOwnProperty("values", v, &w, &e, &c)
	}
	// Live iterator for Map.prototype.keys() - respects deletions during iteration
	mapProto.SetOwnNonEnumerable("keys", vm.NewNativeFunction(0, false, "keys", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.keys called on incompatible receiver")
		}

		// Create iterator object with internal slots, inheriting from MapIteratorPrototype
		it := vm.NewObject(vmInstance.MapIteratorPrototype).AsPlainObject()
		it.SetOwn("[[IteratedMap]]", thisMap)
		it.SetOwn("[[MapNextIndex]]", vm.NumberValue(0))
		it.SetOwn("[[MapIterationKind]]", vm.NewString("keys"))
		it.SetOwn("[[Exhausted]]", vm.BooleanValue(false))

		it.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(a []vm.Value) (vm.Value, error) {
			return vm.NewValueFromPlainObject(it), nil
		}), nil, nil, nil)
		return vm.NewValueFromPlainObject(it), nil
	}))
	if v, ok := mapProto.GetOwn("keys"); ok {
		w, e, c := true, false, true
		mapProto.DefineOwnProperty("keys", v, &w, &e, &c)
	}

	// Create Map constructor function (before setting prototype, so we can reference it)
	mapConstructor := vm.NewConstructorWithProps(0, false, "Map", func(args []vm.Value) (vm.Value, error) {
		// Create new Map instance
		newMap := vm.NewMap()
		mapObj := newMap.AsMap()

		// If an iterable argument is provided, add all its [key, value] pairs
		if len(args) > 0 && !args[0].IsUndefined() && args[0].Type() != vm.TypeNull {
			iterable := args[0]

			// Handle different iterable types
			switch iterable.Type() {
			case vm.TypeArray:
				// Array: expect array of [key, value] pairs
				arr := iterable.AsArray()
				for i := 0; i < arr.Length(); i++ {
					entry := arr.Get(i)
					// Each entry should be an array with [key, value]
					if entry.Type() == vm.TypeArray {
						entryArr := entry.AsArray()
						if entryArr.Length() >= 2 {
							key := entryArr.Get(0)
							value := entryArr.Get(1)
							mapObj.Set(key, value)
						}
					}
				}
			case vm.TypeMap:
				// Map: copy all entries from the source map
				srcMap := iterable.AsMap()
				srcMap.ForEach(func(key vm.Value, val vm.Value) {
					mapObj.Set(key, val)
				})
				// For other types, we'd need full iterator protocol support
				// For now, silently ignore non-iterable arguments
			}
		}

		return newMap, nil
	})

	// Set constructor property on Map.prototype to point to Map constructor
	mapProto.SetOwnNonEnumerable("constructor", mapConstructor)
	if v, ok := mapProto.GetOwn("constructor"); ok {
		w, e, c := true, false, true // writable, not enumerable, configurable
		mapProto.DefineOwnProperty("constructor", v, &w, &e, &c)
	}

	// Add Symbol.toStringTag to Map.prototype (writable: false, enumerable: false, configurable: true)
	wFalse, eFalse, cTrue := false, false, true
	mapProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolToStringTag), vm.NewString("Map"), &wFalse, &eFalse, &cTrue)

	// Set Map.prototype in VM (must be before adding prototype property to constructor)
	vmInstance.MapPrototype = vm.NewValueFromPlainObject(mapProto)

	// Add prototype property
	mapConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vmInstance.MapPrototype)
	if v, ok := mapConstructor.AsNativeFunctionWithProps().Properties.GetOwn("prototype"); ok {
		w, e, c := false, false, false
		mapConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("prototype", v, &w, &e, &c)
	}

	// Add Map.groupBy(items, callbackfn) static method
	mapConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("groupBy", vm.NewNativeFunction(2, false, "groupBy", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Map.groupBy requires 2 arguments")
		}

		items := args[0]
		callbackfn := args[1]

		// Check that callbackfn is callable
		if !callbackfn.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Map.groupBy: callback is not a function")
		}

		// Create result Map
		result := vm.NewMap()
		resultMap := result.AsMap()

		// Get iterator from items
		var iterator vm.Value
		var iterMethod vm.Value
		var hasIterator bool

		// Handle string type specially - get iterator from String.prototype
		if items.Type() == vm.TypeString {
			if vmInstance.StringPrototype.Type() != vm.TypeUndefined {
				proto := vmInstance.StringPrototype.AsPlainObject()
				if proto != nil {
					iterMethod, hasIterator = proto.GetOwnByKey(vm.NewSymbolKey(SymbolIterator))
				}
			}
		} else {
			iterMethod, hasIterator = vmInstance.GetSymbolProperty(items, SymbolIterator)
		}

		if hasIterator && iterMethod.IsCallable() {
			iter, err := vmInstance.Call(iterMethod, items, []vm.Value{})
			if err != nil {
				return vm.Undefined, err
			}
			iterator = iter
		} else {
			return vm.Undefined, vmInstance.NewTypeError("Map.groupBy: items is not iterable")
		}

		// Iterate over items
		k := 0
		for {
			nextMethod, _ := vmInstance.GetProperty(iterator, "next")
			iterResult, err := vmInstance.Call(nextMethod, iterator, []vm.Value{})
			if err != nil {
				return vm.Undefined, err
			}

			doneVal, _ := vmInstance.GetProperty(iterResult, "done")
			if doneVal.IsTruthy() {
				break
			}

			value, _ := vmInstance.GetProperty(iterResult, "value")

			// Call callback with (value, k)
			keyResult, err := vmInstance.Call(callbackfn, vm.Undefined, []vm.Value{value, vm.NumberValue(float64(k))})
			if err != nil {
				return vm.Undefined, err
			}

			// Get or create group array (key can be any value, not just string)
			var group vm.Value
			if existing := resultMap.Get(keyResult); !existing.IsUndefined() {
				group = existing
			} else {
				group = vm.NewArray()
				resultMap.Set(keyResult, group)
			}

			// Append value to group
			group.AsArray().Append(value)

			k++
		}

		return result, nil
	}))

	// Define Map constructor in global scope
	return ctx.DefineGlobal("Map", mapConstructor)
}

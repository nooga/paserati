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

	// Create Map constructor type - use a generic constructor
	mapCtorType := &types.GenericType{
		Name:           "Map",
		TypeParameters: []*types.TypeParameter{kParam, vParam},
		Body:           types.NewSimpleFunction([]types.Type{}, mapType),
	}

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
	mapProto.SetOwnNonEnumerable("forEach", vm.NewNativeFunction(2, false, "forEach", func(args []vm.Value) (vm.Value, error) {
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

	// Add size accessor (getter)
	sizeGetter := vm.NewNativeFunction(0, false, "get size", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.size called on incompatible receiver")
		}
		mapObj := thisMap.AsMap()
		return vm.IntegerValue(int32(mapObj.Size())), nil
	})
	mapProto.SetOwnNonEnumerable("size", sizeGetter)
	w, e, c := true, false, true
	mapProto.DefineOwnProperty("size", sizeGetter, &w, &e, &c)

	// Live iterator for Map.prototype.entries() - respects deletions during iteration
	mapProto.SetOwnNonEnumerable("entries", vm.NewNativeFunction(0, false, "entries", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.entries called on incompatible receiver")
		}
		mapObj := thisMap.AsMap()

		// Create iterator object with live reference to the Map, inheriting from Iterator.prototype
		it := vm.NewObject(vmInstance.IteratorPrototype).AsPlainObject()
		currentIndex := 0 // Closure captures this for live iteration

		// next() - iterates live over the Map, skipping deleted entries
		it.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(a []vm.Value) (vm.Value, error) {
			result := vm.NewObject(vm.Undefined).AsPlainObject()

			// Skip deleted entries (tombstones) and find the next valid entry
			for currentIndex < mapObj.OrderLen() {
				key, value, exists := mapObj.GetEntryAt(currentIndex)
				currentIndex++
				if exists {
					// Found a valid entry - return it
					entry := vm.NewArray()
					entry.AsArray().Append(key)
					entry.AsArray().Append(value)
					result.SetOwnNonEnumerable("value", entry)
					result.SetOwnNonEnumerable("done", vm.BooleanValue(false))
					return vm.NewValueFromPlainObject(result), nil
				}
				// Entry was deleted, continue to next
			}

			// No more entries
			result.SetOwnNonEnumerable("value", vm.Undefined)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(result), nil
		}))

		// [Symbol.iterator]() { return this }
		it.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(a []vm.Value) (vm.Value, error) {
			return vm.NewValueFromPlainObject(it), nil
		}), nil, nil, nil)
		return vm.NewValueFromPlainObject(it), nil
	}))
	if v, ok := mapProto.GetOwn("entries"); ok {
		w, e, c := true, false, true
		mapProto.DefineOwnProperty("entries", v, &w, &e, &c)
	}
	// Live iterator for Map.prototype.values() - respects deletions during iteration
	mapProto.SetOwnNonEnumerable("values", vm.NewNativeFunction(0, false, "values", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, vmInstance.NewTypeError("Map.prototype.values called on incompatible receiver")
		}
		mapObj := thisMap.AsMap()
		it := vm.NewObject(vmInstance.IteratorPrototype).AsPlainObject()
		currentIndex := 0
		it.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(a []vm.Value) (vm.Value, error) {
			result := vm.NewObject(vm.Undefined).AsPlainObject()
			// Live iteration: skip tombstones, check at each step
			for currentIndex < mapObj.OrderLen() {
				_, value, exists := mapObj.GetEntryAt(currentIndex)
				currentIndex++
				if exists {
					result.SetOwnNonEnumerable("value", value)
					result.SetOwnNonEnumerable("done", vm.BooleanValue(false))
					return vm.NewValueFromPlainObject(result), nil
				}
			}
			result.SetOwnNonEnumerable("value", vm.Undefined)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(result), nil
		}))
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
		mapObj := thisMap.AsMap()
		it := vm.NewObject(vmInstance.IteratorPrototype).AsPlainObject()
		currentIndex := 0
		it.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(a []vm.Value) (vm.Value, error) {
			result := vm.NewObject(vm.Undefined).AsPlainObject()
			// Live iteration: skip tombstones, check at each step
			for currentIndex < mapObj.OrderLen() {
				key, _, exists := mapObj.GetEntryAt(currentIndex)
				currentIndex++
				if exists {
					result.SetOwnNonEnumerable("value", key)
					result.SetOwnNonEnumerable("done", vm.BooleanValue(false))
					return vm.NewValueFromPlainObject(result), nil
				}
			}
			result.SetOwnNonEnumerable("value", vm.Undefined)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
			return vm.NewValueFromPlainObject(result), nil
		}))
		it.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(a []vm.Value) (vm.Value, error) {
			return vm.NewValueFromPlainObject(it), nil
		}), nil, nil, nil)
		return vm.NewValueFromPlainObject(it), nil
	}))
	if v, ok := mapProto.GetOwn("keys"); ok {
		w, e, c := true, false, true
		mapProto.DefineOwnProperty("keys", v, &w, &e, &c)
	}
	// Map.prototype[Symbol.iterator] - calls entries() to return an iterator
	wIter := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(args []vm.Value) (vm.Value, error) {
		if v, ok := mapProto.GetOwn("entries"); ok {
			// Call entries() as a method on the current Map instance
			thisMap := vmInstance.GetThis()
			return vmInstance.Call(v, thisMap, []vm.Value{})
		}
		return vm.Undefined, nil
	})
	wb, eb, cb := true, false, true
	mapProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), wIter, &wb, &eb, &cb)

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

	// Define Map constructor in global scope
	return ctx.DefineGlobal("Map", mapConstructor)
}

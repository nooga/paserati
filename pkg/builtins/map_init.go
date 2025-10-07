package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
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
	mapProto.SetOwn("set", vm.NewNativeFunction(2, false, "set", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()

		if thisMap.Type() != vm.TypeMap {
			// TODO: Should throw TypeError
			return vm.Undefined, nil
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

	mapProto.SetOwn("get", vm.NewNativeFunction(1, false, "get", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()

		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, nil
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

	mapProto.SetOwn("has", vm.NewNativeFunction(1, false, "has", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()

		if thisMap.Type() != vm.TypeMap {
			return vm.BooleanValue(false), nil
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

	mapProto.SetOwn("delete", vm.NewNativeFunction(1, false, "delete", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()

		if thisMap.Type() != vm.TypeMap {
			return vm.BooleanValue(false), nil
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

	mapProto.SetOwn("clear", vm.NewNativeFunction(0, false, "clear", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()

		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, nil
		}

		mapObj := thisMap.AsMap()
		mapObj.Clear()
		return vm.Undefined, nil
	}))
	if v, ok := mapProto.GetOwn("clear"); ok {
		w, e, c := true, false, true
		mapProto.DefineOwnProperty("clear", v, &w, &e, &c)
	}

	// Add size accessor (getter)
	sizeGetter := vm.NewNativeFunction(0, false, "get size", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		if thisMap.Type() != vm.TypeMap {
			return vm.IntegerValue(0), nil
		}
		mapObj := thisMap.AsMap()
		return vm.IntegerValue(int32(mapObj.Size())), nil
	})
	mapProto.SetOwn("size", sizeGetter)
	w, e, c := true, false, true
	mapProto.DefineOwnProperty("size", sizeGetter, &w, &e, &c)

	// Minimal iterator helpers for harness usage -> implement proper iterators
	mapProto.SetOwn("entries", vm.NewNativeFunction(0, false, "entries", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, nil
		}
		// Snapshot pairs
		pairs := vm.NewArray()
		pairsArr := pairs.AsArray()
		thisMap.AsMap().ForEach(func(key vm.Value, val vm.Value) {
			entry := vm.NewArray()
			entry.AsArray().Append(key)
			entry.AsArray().Append(val)
			pairsArr.Append(entry)
		})
		// Create iterator object
		it := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		it.SetOwn("__data__", pairs)
		it.SetOwn("__index__", vm.IntegerValue(0))
		// next()
		it.SetOwn("next", vm.NewNativeFunction(0, false, "next", func(a []vm.Value) (vm.Value, error) {
			self := vmInstance.GetThis().AsPlainObject()
			dataVal, _ := self.GetOwn("__data__")
			idxVal, _ := self.GetOwn("__index__")
			data := dataVal.AsArray()
			idx := int(idxVal.ToInteger())
			result := vm.NewObject(vm.Undefined).AsPlainObject()
			if idx >= data.Length() {
				result.SetOwn("value", vm.Undefined)
				result.SetOwn("done", vm.BooleanValue(true))
				return vm.NewValueFromPlainObject(result), nil
			}
			result.SetOwn("value", data.Get(idx))
			result.SetOwn("done", vm.BooleanValue(false))
			self.SetOwn("__index__", vm.IntegerValue(int32(idx+1)))
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
	mapProto.SetOwn("values", vm.NewNativeFunction(0, false, "values", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, nil
		}
		// Snapshot values
		vals := vm.NewArray()
		valsArr := vals.AsArray()
		thisMap.AsMap().ForEach(func(_ vm.Value, val vm.Value) {
			valsArr.Append(val)
		})
		it := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		it.SetOwn("__data__", vals)
		it.SetOwn("__index__", vm.IntegerValue(0))
		it.SetOwn("next", vm.NewNativeFunction(0, false, "next", func(a []vm.Value) (vm.Value, error) {
			self := vmInstance.GetThis().AsPlainObject()
			dataVal, _ := self.GetOwn("__data__")
			idxVal, _ := self.GetOwn("__index__")
			data := dataVal.AsArray()
			idx := int(idxVal.ToInteger())
			result := vm.NewObject(vm.Undefined).AsPlainObject()
			if idx >= data.Length() {
				result.SetOwn("value", vm.Undefined)
				result.SetOwn("done", vm.BooleanValue(true))
				return vm.NewValueFromPlainObject(result), nil
			}
			result.SetOwn("value", data.Get(idx))
			result.SetOwn("done", vm.BooleanValue(false))
			self.SetOwn("__index__", vm.IntegerValue(int32(idx+1)))
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
	mapProto.SetOwn("keys", vm.NewNativeFunction(0, false, "keys", func(args []vm.Value) (vm.Value, error) {
		thisMap := vmInstance.GetThis()
		if thisMap.Type() != vm.TypeMap {
			return vm.Undefined, nil
		}
		// Snapshot keys
		ks := vm.NewArray()
		ksArr := ks.AsArray()
		thisMap.AsMap().ForEach(func(key vm.Value, _ vm.Value) {
			ksArr.Append(key)
		})
		it := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		it.SetOwn("__data__", ks)
		it.SetOwn("__index__", vm.IntegerValue(0))
		it.SetOwn("next", vm.NewNativeFunction(0, false, "next", func(a []vm.Value) (vm.Value, error) {
			self := vmInstance.GetThis().AsPlainObject()
			dataVal, _ := self.GetOwn("__data__")
			idxVal, _ := self.GetOwn("__index__")
			data := dataVal.AsArray()
			idx := int(idxVal.ToInteger())
			result := vm.NewObject(vm.Undefined).AsPlainObject()
			if idx >= data.Length() {
				result.SetOwn("value", vm.Undefined)
				result.SetOwn("done", vm.BooleanValue(true))
				return vm.NewValueFromPlainObject(result), nil
			}
			result.SetOwn("value", data.Get(idx))
			result.SetOwn("done", vm.BooleanValue(false))
			self.SetOwn("__index__", vm.IntegerValue(int32(idx+1)))
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
	mapConstructor := vm.NewNativeFunctionWithProps(0, false, "Map", func(args []vm.Value) (vm.Value, error) {
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
	mapProto.SetOwn("constructor", mapConstructor)
	if v, ok := mapProto.GetOwn("constructor"); ok {
		w, e, c := true, false, true // writable, not enumerable, configurable
		mapProto.DefineOwnProperty("constructor", v, &w, &e, &c)
	}

	// Set Map.prototype in VM (must be before adding prototype property to constructor)
	vmInstance.MapPrototype = vm.NewValueFromPlainObject(mapProto)

	// Add prototype property
	mapConstructor.AsNativeFunctionWithProps().Properties.SetOwn("prototype", vmInstance.MapPrototype)
	if v, ok := mapConstructor.AsNativeFunctionWithProps().Properties.GetOwn("prototype"); ok {
		w, e, c := false, false, false
		mapConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("prototype", v, &w, &e, &c)
	}

	// Define Map constructor in global scope
	return ctx.DefineGlobal("Map", mapConstructor)
}

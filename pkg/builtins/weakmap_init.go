package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type WeakMapInitializer struct{}

func (w *WeakMapInitializer) Name() string {
	return "WeakMap"
}

func (w *WeakMapInitializer) Priority() int {
	return 410 // After Map (400)
}

func (w *WeakMapInitializer) InitTypes(ctx *TypeContext) error {
	// Create generic type parameters K, V for WeakMap methods
	// K is constrained to object (non-primitive)
	objectConstraint := types.NewObjectType()
	kParam := &types.TypeParameter{Name: "K", Constraint: objectConstraint, Index: 0}
	vParam := &types.TypeParameter{Name: "V", Constraint: nil, Index: 1}
	kType := &types.TypeParameterType{Parameter: kParam}
	vType := &types.TypeParameterType{Parameter: vParam}

	// Create the generic type first (with placeholder body)
	weakMapType := &types.GenericType{
		Name:           "WeakMap",
		TypeParameters: []*types.TypeParameter{kParam, vParam},
		Body:           nil, // Will be set below
	}

	// Create WeakMap instance type with methods
	// Note: WeakMap has no size, forEach, keys, values, entries (by ECMAScript design)
	weakMapInstanceType := types.NewObjectType().
		WithProperty("set", types.NewSimpleFunction([]types.Type{kType, vType}, weakMapType)).
		WithProperty("get", types.NewSimpleFunction([]types.Type{kType}, types.NewUnionType(vType, types.Undefined))).
		WithProperty("has", types.NewSimpleFunction([]types.Type{kType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{kType}, types.Boolean))

	// Now set the body of the generic type
	weakMapType.Body = weakMapInstanceType

	// Create WeakMap.prototype type for runtime (same structure)
	weakMapProtoType := types.NewObjectType().
		WithProperty("set", types.NewSimpleFunction([]types.Type{kType, vType}, weakMapType)).
		WithProperty("get", types.NewSimpleFunction([]types.Type{kType}, types.NewUnionType(vType, types.Undefined))).
		WithProperty("has", types.NewSimpleFunction([]types.Type{kType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{kType}, types.Boolean))

	// Register WeakMap primitive prototype
	ctx.SetPrimitivePrototype("weakmap", weakMapProtoType)

	// Create WeakMap constructor type - use a generic constructor
	weakMapCtorType := &types.GenericType{
		Name:           "WeakMap",
		TypeParameters: []*types.TypeParameter{kParam, vParam},
		Body:           types.NewSimpleFunction([]types.Type{}, weakMapType),
	}

	// Define WeakMap constructor in global environment
	err := ctx.DefineGlobal("WeakMap", weakMapCtorType)
	if err != nil {
		return err
	}

	// Also define the type alias for type annotations like WeakMap<object, string>
	return ctx.DefineTypeAlias("WeakMap", weakMapType)
}

func (w *WeakMapInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create WeakMap.prototype inheriting from Object.prototype
	weakMapProto := vm.NewObject(objectProto).AsPlainObject()

	// Add WeakMap prototype methods
	weakMapProto.SetOwnNonEnumerable("set", vm.NewNativeFunction(2, false, "set", func(args []vm.Value) (vm.Value, error) {
		thisWeakMap := vmInstance.GetThis()

		if thisWeakMap.Type() != vm.TypeWeakMap {
			return vm.Undefined, vmInstance.NewTypeError("WeakMap.prototype.set called on non-WeakMap")
		}

		if len(args) < 2 {
			return thisWeakMap, nil // Return the WeakMap for chaining
		}

		key := args[0]
		if !key.CanBeHeldWeakly() {
			return vm.Undefined, vmInstance.NewTypeError("Invalid value used as weak map key")
		}

		weakMapObj := thisWeakMap.AsWeakMap()
		weakMapObj.Set(key, args[1])
		return thisWeakMap, nil // Return the WeakMap for chaining
	}))
	if v, ok := weakMapProto.GetOwn("set"); ok {
		w, e, c := true, false, true
		weakMapProto.DefineOwnProperty("set", v, &w, &e, &c)
	}

	weakMapProto.SetOwnNonEnumerable("get", vm.NewNativeFunction(1, false, "get", func(args []vm.Value) (vm.Value, error) {
		thisWeakMap := vmInstance.GetThis()

		if thisWeakMap.Type() != vm.TypeWeakMap {
			return vm.Undefined, vmInstance.NewTypeError("WeakMap.prototype.get called on non-WeakMap")
		}

		if len(args) < 1 {
			return vm.Undefined, nil
		}

		key := args[0]
		if !key.CanBeHeldWeakly() {
			return vm.Undefined, nil // Keys that can't be held weakly always return undefined
		}

		weakMapObj := thisWeakMap.AsWeakMap()
		val, _ := weakMapObj.Get(key)
		return val, nil
	}))
	if v, ok := weakMapProto.GetOwn("get"); ok {
		w, e, c := true, false, true
		weakMapProto.DefineOwnProperty("get", v, &w, &e, &c)
	}

	weakMapProto.SetOwnNonEnumerable("has", vm.NewNativeFunction(1, false, "has", func(args []vm.Value) (vm.Value, error) {
		thisWeakMap := vmInstance.GetThis()

		if thisWeakMap.Type() != vm.TypeWeakMap {
			return vm.BooleanValue(false), vmInstance.NewTypeError("WeakMap.prototype.has called on non-WeakMap")
		}

		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}

		key := args[0]
		if !key.CanBeHeldWeakly() {
			return vm.BooleanValue(false), nil // Keys that can't be held weakly are never present
		}

		weakMapObj := thisWeakMap.AsWeakMap()
		return vm.BooleanValue(weakMapObj.Has(key)), nil
	}))
	if v, ok := weakMapProto.GetOwn("has"); ok {
		w, e, c := true, false, true
		weakMapProto.DefineOwnProperty("has", v, &w, &e, &c)
	}

	weakMapProto.SetOwnNonEnumerable("delete", vm.NewNativeFunction(1, false, "delete", func(args []vm.Value) (vm.Value, error) {
		thisWeakMap := vmInstance.GetThis()

		if thisWeakMap.Type() != vm.TypeWeakMap {
			return vm.BooleanValue(false), vmInstance.NewTypeError("WeakMap.prototype.delete called on non-WeakMap")
		}

		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}

		key := args[0]
		if !key.CanBeHeldWeakly() {
			return vm.BooleanValue(false), nil // Keys that can't be held weakly are never present
		}

		weakMapObj := thisWeakMap.AsWeakMap()
		return vm.BooleanValue(weakMapObj.Delete(key)), nil
	}))
	if v, ok := weakMapProto.GetOwn("delete"); ok {
		w, e, c := true, false, true
		weakMapProto.DefineOwnProperty("delete", v, &w, &e, &c)
	}

	// getOrInsert(key, value) - returns existing value if key present, otherwise inserts and returns value
	weakMapProto.SetOwnNonEnumerable("getOrInsert", vm.NewNativeFunction(2, false, "getOrInsert", func(args []vm.Value) (vm.Value, error) {
		thisWeakMap := vmInstance.GetThis()

		if thisWeakMap.Type() != vm.TypeWeakMap {
			return vm.Undefined, vmInstance.NewTypeError("WeakMap.prototype.getOrInsert called on non-WeakMap")
		}

		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("WeakMap.prototype.getOrInsert requires at least 1 argument")
		}

		key := args[0]
		if !key.CanBeHeldWeakly() {
			return vm.Undefined, vmInstance.NewTypeError("Invalid value used as weak map key")
		}

		value := vm.Undefined
		if len(args) >= 2 {
			value = args[1]
		}

		weakMapObj := thisWeakMap.AsWeakMap()

		// Check if key already exists
		if existing, found := weakMapObj.Get(key); found {
			return existing, nil
		}

		// Insert and return the value
		weakMapObj.Set(key, value)
		return value, nil
	}))
	if v, ok := weakMapProto.GetOwn("getOrInsert"); ok {
		w, e, c := true, false, true
		weakMapProto.DefineOwnProperty("getOrInsert", v, &w, &e, &c)
	}

	// getOrInsertComputed(key, callbackfn) - returns existing value or calls callback to compute value
	weakMapProto.SetOwnNonEnumerable("getOrInsertComputed", vm.NewNativeFunction(2, false, "getOrInsertComputed", func(args []vm.Value) (vm.Value, error) {
		thisWeakMap := vmInstance.GetThis()

		if thisWeakMap.Type() != vm.TypeWeakMap {
			return vm.Undefined, vmInstance.NewTypeError("WeakMap.prototype.getOrInsertComputed called on non-WeakMap")
		}

		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("WeakMap.prototype.getOrInsertComputed requires 2 arguments")
		}

		key := args[0]
		if !key.CanBeHeldWeakly() {
			return vm.Undefined, vmInstance.NewTypeError("Invalid value used as weak map key")
		}

		callbackfn := args[1]
		if !callbackfn.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("WeakMap.prototype.getOrInsertComputed: callback is not a function")
		}

		weakMapObj := thisWeakMap.AsWeakMap()

		// Check if key already exists
		if existing, found := weakMapObj.Get(key); found {
			return existing, nil
		}

		// Call callback with the key to compute the value
		value, err := vmInstance.Call(callbackfn, key, []vm.Value{key})
		if err != nil {
			return vm.Undefined, err
		}

		// Insert and return the computed value
		weakMapObj.Set(key, value)
		return value, nil
	}))
	if v, ok := weakMapProto.GetOwn("getOrInsertComputed"); ok {
		w, e, c := true, false, true
		weakMapProto.DefineOwnProperty("getOrInsertComputed", v, &w, &e, &c)
	}

	// Create WeakMap constructor function
	weakMapConstructor := vm.NewConstructorWithProps(0, false, "WeakMap", func(args []vm.Value) (vm.Value, error) {
		// Get the prototype from newTarget using GetPrototypeFromConstructor
		// This implements ECMAScript spec behavior for cross-realm construction
		newTarget := vmInstance.GetNewTarget()
		var prototype vm.Value
		if newTarget.Type() != vm.TypeUndefined {
			prototype = vmInstance.GetPrototypeFromConstructor(newTarget, "%WeakMapPrototype%")
		} else {
			prototype = vmInstance.WeakMapPrototype
		}

		// Create new WeakMap instance with the determined prototype
		newWeakMap := vm.NewWeakMapWithPrototype(prototype)
		weakMapObj := newWeakMap.AsWeakMap()

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
							// WeakMap requires keys that can be held weakly
							if key.CanBeHeldWeakly() {
								weakMapObj.Set(key, value)
							} else {
								return vm.Undefined, vmInstance.NewTypeError("Invalid value used as weak map key")
							}
						}
					}
				}
			}
		}

		return newWeakMap, nil
	})

	// Set constructor property on WeakMap.prototype
	weakMapProto.SetOwnNonEnumerable("constructor", weakMapConstructor)
	if v, ok := weakMapProto.GetOwn("constructor"); ok {
		w, e, c := true, false, true
		weakMapProto.DefineOwnProperty("constructor", v, &w, &e, &c)
	}

	// Set WeakMap.prototype in VM
	vmInstance.WeakMapPrototype = vm.NewValueFromPlainObject(weakMapProto)

	// Add prototype property to constructor
	weakMapConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vmInstance.WeakMapPrototype)
	if v, ok := weakMapConstructor.AsNativeFunctionWithProps().Properties.GetOwn("prototype"); ok {
		w, e, c := false, false, false
		weakMapConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("prototype", v, &w, &e, &c)
	}

	// Define WeakMap constructor in global scope
	return ctx.DefineGlobal("WeakMap", weakMapConstructor)
}

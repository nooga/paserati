package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type SetInitializer struct{}

func (s *SetInitializer) Name() string {
	return "Set"
}

func (s *SetInitializer) Priority() int {
	return 410 // After Map (400)
}

func (s *SetInitializer) InitTypes(ctx *TypeContext) error {
	// Create generic type parameter T for set methods
	tParam := &types.TypeParameter{Name: "T", Constraint: nil, Index: 0}
	tType := &types.TypeParameterType{Parameter: tParam}

	// Create the generic type first (with placeholder body)
	setType := &types.GenericType{
		Name:           "Set",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           nil, // Will be set below
	}

	// Create Set instance type with methods (using type parameters directly)
	// forEach callback: (value: T, value2: T, set: Set<T>) => void
	forEachCallbackType := types.NewSimpleFunction([]types.Type{tType, tType, setType}, types.Void)

	setInstanceType := types.NewObjectType().
		WithProperty("add", types.NewSimpleFunction([]types.Type{tType}, setType)). // Return this for chaining
		WithProperty("has", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Void)).
		WithProperty("forEach", types.NewSimpleFunction([]types.Type{forEachCallbackType}, types.Void)).
		WithProperty("size", types.Number)

	// Now set the body of the generic type
	setType.Body = setInstanceType

	// Create Set.prototype type for runtime (same structure)
	setProtoType := types.NewObjectType().
		WithProperty("add", types.NewSimpleFunction([]types.Type{tType}, setType)). // Return this for chaining
		WithProperty("has", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Void)).
		WithProperty("forEach", types.NewSimpleFunction([]types.Type{forEachCallbackType}, types.Void)).
		WithProperty("size", types.Number)

	// Register set primitive prototype
	ctx.SetPrimitivePrototype("set", setProtoType)

	// Create Set constructor type - use a generic constructor
	setCtorType := &types.GenericType{
		Name:           "Set",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           types.NewSimpleFunction([]types.Type{}, setType),
	}

	// Define Set constructor in global environment
	err := ctx.DefineGlobal("Set", setCtorType)
	if err != nil {
		return err
	}

	// Also define the type alias for type annotations like Set<string>
	return ctx.DefineTypeAlias("Set", setType)
}

// createSetMethod creates a generic method with T type parameter
func (s *SetInitializer) createSetMethod(name string, tParam *types.TypeParameter, methodType types.Type) types.Type {
	return &types.GenericType{
		Name:           name,
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           methodType,
	}
}

func (s *SetInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Set.prototype inheriting from Object.prototype
	setProto := vm.NewObject(objectProto).AsPlainObject()

	// Add Set prototype methods
	setProto.SetOwnNonEnumerable("add", vm.NewNativeFunction(1, false, "add", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()

		if thisSet.Type() != vm.TypeSet {
			// TODO: Should throw TypeError
			return vm.Undefined, nil
		}

		if len(args) < 1 {
			return thisSet, nil // Return the set for chaining
		}

		setObj := thisSet.AsSet()
		setObj.Add(args[0])
		return thisSet, nil // Return the set for chaining
	}))
	if v, ok := setProto.GetOwn("add"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("add", v, &w, &e, &c)
	}

	setProto.SetOwnNonEnumerable("has", vm.NewNativeFunction(1, false, "has", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()

		if thisSet.Type() != vm.TypeSet {
			return vm.BooleanValue(false), nil
		}

		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}

		setObj := thisSet.AsSet()
		return vm.BooleanValue(setObj.Has(args[0])), nil
	}))
	if v, ok := setProto.GetOwn("has"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("has", v, &w, &e, &c)
	}

	setProto.SetOwnNonEnumerable("delete", vm.NewNativeFunction(1, false, "delete", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()

		if thisSet.Type() != vm.TypeSet {
			return vm.BooleanValue(false), nil
		}

		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}

		setObj := thisSet.AsSet()
		return vm.BooleanValue(setObj.Delete(args[0])), nil
	}))
	if v, ok := setProto.GetOwn("delete"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("delete", v, &w, &e, &c)
	}

	setProto.SetOwnNonEnumerable("clear", vm.NewNativeFunction(0, false, "clear", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()

		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, nil
		}

		setObj := thisSet.AsSet()
		setObj.Clear()
		return vm.Undefined, nil
	}))
	if v, ok := setProto.GetOwn("clear"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("clear", v, &w, &e, &c)
	}

	// forEach(callback, thisArg)
	setProto.SetOwnNonEnumerable("forEach", vm.NewNativeFunction(1, false, "forEach", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.forEach called on non-Set")
		}

		if len(args) < 1 || !args[0].IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.forEach requires a callable function")
		}

		callback := args[0]
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		setObj := thisSet.AsSet()
		setObj.ForEach(func(val vm.Value) {
			// forEach callback receives (value, value, set) - value is passed twice for consistency with Map
			_, _ = vmInstance.Call(callback, thisArg, []vm.Value{val, val, thisSet})
		})

		return vm.Undefined, nil
	}))
	if v, ok := setProto.GetOwn("forEach"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("forEach", v, &w, &e, &c)
	}

	// Minimal iterator helpers: values(), keys(), entries(), and [Symbol.iterator]
	// These use live iteration - checking the set at each step, not a snapshot.
	setProto.SetOwnNonEnumerable("values", vm.NewNativeFunction(0, false, "values", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, nil
		}
		setObj := thisSet.AsSet()
		it := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		currentIndex := 0
		it.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(a []vm.Value) (vm.Value, error) {
			result := vm.NewObject(vm.Undefined).AsPlainObject()
			// Live iteration: skip tombstones, check at each step
			for currentIndex < setObj.OrderLen() {
				val, exists := setObj.GetValueAt(currentIndex)
				currentIndex++
				if exists {
					result.SetOwnNonEnumerable("value", val)
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
	if v, ok := setProto.GetOwn("values"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("values", v, &w, &e, &c)
	}
	// keys() is an alias of values() for Set - uses same live iteration
	setProto.SetOwnNonEnumerable("keys", vm.NewNativeFunction(0, false, "keys", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, nil
		}
		setObj := thisSet.AsSet()
		it := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		currentIndex := 0
		it.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(a []vm.Value) (vm.Value, error) {
			result := vm.NewObject(vm.Undefined).AsPlainObject()
			// Live iteration: skip tombstones, check at each step
			for currentIndex < setObj.OrderLen() {
				val, exists := setObj.GetValueAt(currentIndex)
				currentIndex++
				if exists {
					result.SetOwnNonEnumerable("value", val)
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
	if v, ok := setProto.GetOwn("keys"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("keys", v, &w, &e, &c)
	}
	// entries() yields [value, value] - uses live iteration
	setProto.SetOwnNonEnumerable("entries", vm.NewNativeFunction(0, false, "entries", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, nil
		}
		setObj := thisSet.AsSet()
		it := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		currentIndex := 0
		it.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(a []vm.Value) (vm.Value, error) {
			result := vm.NewObject(vm.Undefined).AsPlainObject()
			// Live iteration: skip tombstones, check at each step
			for currentIndex < setObj.OrderLen() {
				val, exists := setObj.GetValueAt(currentIndex)
				currentIndex++
				if exists {
					// Set entries() yields [value, value]
					entry := vm.NewArray()
					entry.AsArray().Append(val)
					entry.AsArray().Append(val)
					result.SetOwnNonEnumerable("value", entry)
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
	if v, ok := setProto.GetOwn("entries"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("entries", v, &w, &e, &c)
	}
	// Set.prototype[Symbol.iterator] - calls values() to return an iterator
	wIter := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(args []vm.Value) (vm.Value, error) {
		if v, ok := setProto.GetOwn("values"); ok {
			// Call values() as a method on the current Set instance
			thisSet := vmInstance.GetThis()
			return vmInstance.Call(v, thisSet, []vm.Value{})
		}
		return vm.Undefined, nil
	})
	wb, eb, cb := true, false, true
	setProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), wIter, &wb, &eb, &cb)

	// Add size accessor (getter)
	sizeGetter := vm.NewNativeFunction(0, false, "get size", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.IntegerValue(0), nil
		}
		setObj := thisSet.AsSet()
		return vm.IntegerValue(int32(setObj.Size())), nil
	})
	setProto.SetOwnNonEnumerable("size", sizeGetter)
	w, e, c := true, false, true
	setProto.DefineOwnProperty("size", sizeGetter, &w, &e, &c)

	// Create Set constructor function (before setting prototype, so we can reference it)
	setConstructor := vm.NewConstructorWithProps(0, false, "Set", func(args []vm.Value) (vm.Value, error) {
		// Create new Set instance
		newSet := vm.NewSet()
		setObj := newSet.AsSet()

		// If an iterable argument is provided, add all its elements
		if len(args) > 0 && !args[0].IsUndefined() && args[0].Type() != vm.TypeNull {
			iterable := args[0]

			// Handle different iterable types
			switch iterable.Type() {
			case vm.TypeArray:
				// Array: iterate and add all elements
				arr := iterable.AsArray()
				for i := 0; i < arr.Length(); i++ {
					setObj.Add(arr.Get(i))
				}
			case vm.TypeString:
				// String: iterate and add each character
				str := iterable.AsString()
				for _, char := range str {
					setObj.Add(vm.NewString(string(char)))
				}
			case vm.TypeSet:
				// Set: copy all values from the source set
				srcSet := iterable.AsSet()
				srcSet.ForEach(func(val vm.Value) {
					setObj.Add(val)
				})
			case vm.TypeMap:
				// Map: add all [key, value] pairs
				srcMap := iterable.AsMap()
				srcMap.ForEach(func(key vm.Value, val vm.Value) {
					pairVal := vm.NewArray()
					pairArr := pairVal.AsArray()
					pairArr.Append(key)
					pairArr.Append(val)
					setObj.Add(pairVal)
				})
				// For other types, we'd need full iterator protocol support
				// For now, silently ignore non-iterable arguments
			}
		}

		return newSet, nil
	})

	// Set constructor property on Set.prototype to point to Set constructor
	setProto.SetOwnNonEnumerable("constructor", setConstructor)
	if v, ok := setProto.GetOwn("constructor"); ok {
		w, e, c := true, false, true // writable, not enumerable, configurable
		setProto.DefineOwnProperty("constructor", v, &w, &e, &c)
	}

	// Add Symbol.toStringTag to Set.prototype (writable: false, enumerable: false, configurable: true)
	wFalse, eFalse, cTrue := false, false, true
	setProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolToStringTag), vm.NewString("Set"), &wFalse, &eFalse, &cTrue)

	// Set Set.prototype in VM (must be before adding prototype property to constructor)
	vmInstance.SetPrototype = vm.NewValueFromPlainObject(setProto)

	// Add prototype property
	setConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vmInstance.SetPrototype)
	if v, ok := setConstructor.AsNativeFunctionWithProps().Properties.GetOwn("prototype"); ok {
		w, e, c := false, false, false
		setConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("prototype", v, &w, &e, &c)
	}

	// Define Set constructor in global scope
	return ctx.DefineGlobal("Set", setConstructor)
}

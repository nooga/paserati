package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
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
	setInstanceType := types.NewObjectType().
		WithProperty("add", types.NewSimpleFunction([]types.Type{tType}, setType)). // Return this for chaining
		WithProperty("has", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Void)).
		WithProperty("size", types.Number)

	// Now set the body of the generic type
	setType.Body = setInstanceType

	// Create Set.prototype type for runtime (same structure)
	setProtoType := types.NewObjectType().
		WithProperty("add", types.NewSimpleFunction([]types.Type{tType}, setType)). // Return this for chaining
		WithProperty("has", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Void)).
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
	setProto.SetOwn("add", vm.NewNativeFunction(1, false, "add", func(args []vm.Value) (vm.Value, error) {
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

	setProto.SetOwn("has", vm.NewNativeFunction(1, false, "has", func(args []vm.Value) (vm.Value, error) {
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

	setProto.SetOwn("delete", vm.NewNativeFunction(1, false, "delete", func(args []vm.Value) (vm.Value, error) {
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

	setProto.SetOwn("clear", vm.NewNativeFunction(0, false, "clear", func(args []vm.Value) (vm.Value, error) {
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

	// Minimal iterator helpers: values(), keys(), entries(), and [Symbol.iterator]
	setProto.SetOwn("values", vm.NewNativeFunction(0, false, "values", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, nil
		}
		vals := vm.NewArray()
		valsArr := vals.AsArray()
		thisSet.AsSet().ForEach(func(val vm.Value) {
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
	if v, ok := setProto.GetOwn("values"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("values", v, &w, &e, &c)
	}
	// keys() is an alias of values() for Set
	setProto.SetOwn("keys", vm.NewNativeFunction(0, false, "keys", func(args []vm.Value) (vm.Value, error) {
		// Call values()
		if v, ok := setProto.GetOwn("values"); ok {
			return v, nil
		}
		return vm.Undefined, nil
	}))
	if v, ok := setProto.GetOwn("keys"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("keys", v, &w, &e, &c)
	}
	// entries() yields [value, value]
	setProto.SetOwn("entries", vm.NewNativeFunction(0, false, "entries", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, nil
		}
		pairs := vm.NewArray()
		pairsArr := pairs.AsArray()
		thisSet.AsSet().ForEach(func(val vm.Value) {
			pair := vm.NewArray()
			pair.AsArray().Append(val)
			pair.AsArray().Append(val)
			pairsArr.Append(pair)
		})
		it := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		it.SetOwn("__data__", pairs)
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
	if v, ok := setProto.GetOwn("entries"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("entries", v, &w, &e, &c)
	}
	// Set.prototype[Symbol.iterator] === values
	wIter := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(args []vm.Value) (vm.Value, error) {
		if v, ok := setProto.GetOwn("values"); ok {
			return v, nil
		}
		return vm.Undefined, nil
	})
	wb, eb, cb := true, false, true
	setProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), wIter, &wb, &eb, &cb)

	// Set Set.prototype
	vmInstance.SetPrototype = vm.NewValueFromPlainObject(setProto)

	// Create Set constructor function
	setConstructor := vm.NewNativeFunctionWithProps(0, false, "Set", func(args []vm.Value) (vm.Value, error) {
		// Create new Set instance
		return vm.NewSet(), nil
	})

	// Add prototype property
	setConstructor.AsNativeFunctionWithProps().Properties.SetOwn("prototype", vmInstance.SetPrototype)
	if v, ok := setConstructor.AsNativeFunctionWithProps().Properties.GetOwn("prototype"); ok {
		w, e, c := false, false, false
		setConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("prototype", v, &w, &e, &c)
	}

	// Define Set constructor in global scope
	return ctx.DefineGlobal("Set", setConstructor)
}

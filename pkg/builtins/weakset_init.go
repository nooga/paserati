package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type WeakSetInitializer struct{}

func (w *WeakSetInitializer) Name() string {
	return "WeakSet"
}

func (w *WeakSetInitializer) Priority() int {
	return 415 // After WeakMap (410)
}

func (w *WeakSetInitializer) InitTypes(ctx *TypeContext) error {
	// Create generic type parameter T for WeakSet methods
	// T is constrained to object (non-primitive)
	objectConstraint := types.NewObjectType()
	tParam := &types.TypeParameter{Name: "T", Constraint: objectConstraint, Index: 0}
	tType := &types.TypeParameterType{Parameter: tParam}

	// Create the generic type first (with placeholder body)
	weakSetType := &types.GenericType{
		Name:           "WeakSet",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           nil, // Will be set below
	}

	// Create WeakSet instance type with methods
	// Note: WeakSet has no size, forEach, values, keys, entries (by ECMAScript design)
	weakSetInstanceType := types.NewObjectType().
		WithProperty("add", types.NewSimpleFunction([]types.Type{tType}, weakSetType)).
		WithProperty("has", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{tType}, types.Boolean))

	// Now set the body of the generic type
	weakSetType.Body = weakSetInstanceType

	// Create WeakSet.prototype type for runtime (same structure)
	weakSetProtoType := types.NewObjectType().
		WithProperty("add", types.NewSimpleFunction([]types.Type{tType}, weakSetType)).
		WithProperty("has", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{tType}, types.Boolean))

	// Register WeakSet primitive prototype
	ctx.SetPrimitivePrototype("weakset", weakSetProtoType)

	// Create WeakSet constructor type - use a generic constructor
	weakSetCtorType := &types.GenericType{
		Name:           "WeakSet",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           types.NewSimpleFunction([]types.Type{}, weakSetType),
	}

	// Define WeakSet constructor in global environment
	err := ctx.DefineGlobal("WeakSet", weakSetCtorType)
	if err != nil {
		return err
	}

	// Also define the type alias for type annotations like WeakSet<object>
	return ctx.DefineTypeAlias("WeakSet", weakSetType)
}

func (w *WeakSetInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create WeakSet.prototype inheriting from Object.prototype
	weakSetProto := vm.NewObject(objectProto).AsPlainObject()

	// Add WeakSet prototype methods
	weakSetProto.SetOwnNonEnumerable("add", vm.NewNativeFunction(1, false, "add", func(args []vm.Value) (vm.Value, error) {
		thisWeakSet := vmInstance.GetThis()

		if thisWeakSet.Type() != vm.TypeWeakSet {
			return vm.Undefined, vmInstance.NewTypeError("WeakSet.prototype.add called on non-WeakSet")
		}

		if len(args) < 1 {
			return thisWeakSet, nil // Return the WeakSet for chaining
		}

		value := args[0]
		if !value.CanBeHeldWeakly() {
			return vm.Undefined, vmInstance.NewTypeError("Invalid value used in weak set")
		}

		weakSetObj := thisWeakSet.AsWeakSet()
		weakSetObj.Add(value)
		return thisWeakSet, nil // Return the WeakSet for chaining
	}))
	if v, ok := weakSetProto.GetOwn("add"); ok {
		w, e, c := true, false, true
		weakSetProto.DefineOwnProperty("add", v, &w, &e, &c)
	}

	weakSetProto.SetOwnNonEnumerable("has", vm.NewNativeFunction(1, false, "has", func(args []vm.Value) (vm.Value, error) {
		thisWeakSet := vmInstance.GetThis()

		if thisWeakSet.Type() != vm.TypeWeakSet {
			return vm.BooleanValue(false), vmInstance.NewTypeError("WeakSet.prototype.has called on non-WeakSet")
		}

		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}

		value := args[0]
		if !value.CanBeHeldWeakly() {
			return vm.BooleanValue(false), nil // Values that can't be held weakly are never present
		}

		weakSetObj := thisWeakSet.AsWeakSet()
		return vm.BooleanValue(weakSetObj.Has(value)), nil
	}))
	if v, ok := weakSetProto.GetOwn("has"); ok {
		w, e, c := true, false, true
		weakSetProto.DefineOwnProperty("has", v, &w, &e, &c)
	}

	weakSetProto.SetOwnNonEnumerable("delete", vm.NewNativeFunction(1, false, "delete", func(args []vm.Value) (vm.Value, error) {
		thisWeakSet := vmInstance.GetThis()

		if thisWeakSet.Type() != vm.TypeWeakSet {
			return vm.BooleanValue(false), vmInstance.NewTypeError("WeakSet.prototype.delete called on non-WeakSet")
		}

		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}

		value := args[0]
		if !value.CanBeHeldWeakly() {
			return vm.BooleanValue(false), nil // Values that can't be held weakly are never present
		}

		weakSetObj := thisWeakSet.AsWeakSet()
		return vm.BooleanValue(weakSetObj.Delete(value)), nil
	}))
	if v, ok := weakSetProto.GetOwn("delete"); ok {
		w, e, c := true, false, true
		weakSetProto.DefineOwnProperty("delete", v, &w, &e, &c)
	}

	// Create WeakSet constructor function
	weakSetConstructor := vm.NewConstructorWithProps(0, false, "WeakSet", func(args []vm.Value) (vm.Value, error) {
		// Create new WeakSet instance
		newWeakSet := vm.NewWeakSet()
		weakSetObj := newWeakSet.AsWeakSet()

		// If an iterable argument is provided, add all its values
		if len(args) > 0 && !args[0].IsUndefined() && args[0].Type() != vm.TypeNull {
			iterable := args[0]

			// Handle different iterable types
			switch iterable.Type() {
			case vm.TypeArray:
				// Array: add all elements
				arr := iterable.AsArray()
				for i := 0; i < arr.Length(); i++ {
					value := arr.Get(i)
					// WeakSet requires values that can be held weakly
					if value.CanBeHeldWeakly() {
						weakSetObj.Add(value)
					} else {
						return vm.Undefined, vmInstance.NewTypeError("Invalid value used in weak set")
					}
				}
			}
		}

		return newWeakSet, nil
	})

	// Set constructor property on WeakSet.prototype
	weakSetProto.SetOwnNonEnumerable("constructor", weakSetConstructor)
	if v, ok := weakSetProto.GetOwn("constructor"); ok {
		w, e, c := true, false, true
		weakSetProto.DefineOwnProperty("constructor", v, &w, &e, &c)
	}

	// Set WeakSet.prototype in VM
	vmInstance.WeakSetPrototype = vm.NewValueFromPlainObject(weakSetProto)

	// Add prototype property to constructor
	weakSetConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vmInstance.WeakSetPrototype)
	if v, ok := weakSetConstructor.AsNativeFunctionWithProps().Properties.GetOwn("prototype"); ok {
		w, e, c := false, false, false
		weakSetConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("prototype", v, &w, &e, &c)
	}

	// Define WeakSet constructor in global scope
	return ctx.DefineGlobal("WeakSet", weakSetConstructor)
}

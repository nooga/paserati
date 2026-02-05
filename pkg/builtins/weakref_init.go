package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type WeakRefInitializer struct{}

func (w *WeakRefInitializer) Name() string {
	return "WeakRef"
}

func (w *WeakRefInitializer) Priority() int {
	return 415 // After WeakMap (410)
}

func (w *WeakRefInitializer) InitTypes(ctx *TypeContext) error {
	// Create generic type parameter T for WeakRef
	// T is constrained to object (non-primitive)
	objectConstraint := types.NewObjectType()
	tParam := &types.TypeParameter{Name: "T", Constraint: objectConstraint, Index: 0}
	tType := &types.TypeParameterType{Parameter: tParam}

	// Create the generic type first (with placeholder body)
	weakRefType := &types.GenericType{
		Name:           "WeakRef",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           nil, // Will be set below
	}

	// Create WeakRef instance type with deref method
	// WeakRef only has deref() method per ECMAScript spec
	weakRefInstanceType := types.NewObjectType().
		WithProperty("deref", types.NewSimpleFunction([]types.Type{}, types.NewUnionType(tType, types.Undefined)))

	// Now set the body of the generic type
	weakRefType.Body = weakRefInstanceType

	// Create WeakRef.prototype type for runtime (same structure)
	weakRefProtoType := types.NewObjectType().
		WithProperty("deref", types.NewSimpleFunction([]types.Type{}, types.NewUnionType(tType, types.Undefined)))

	// Register WeakRef primitive prototype
	ctx.SetPrimitivePrototype("weakref", weakRefProtoType)

	// Create WeakRef constructor type - use a generic constructor
	weakRefCtorType := &types.GenericType{
		Name:           "WeakRef",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           types.NewSimpleFunction([]types.Type{tType}, weakRefType),
	}

	// Define WeakRef constructor in global environment
	err := ctx.DefineGlobal("WeakRef", weakRefCtorType)
	if err != nil {
		return err
	}

	// Also define the type alias for type annotations like WeakRef<object>
	return ctx.DefineTypeAlias("WeakRef", weakRefType)
}

func (w *WeakRefInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create WeakRef.prototype inheriting from Object.prototype
	weakRefProto := vm.NewObject(objectProto).AsPlainObject()

	// Add WeakRef.prototype.deref() method
	// Per ECMAScript spec: https://tc39.es/ecma262/#sec-weak-ref.prototype.deref
	weakRefProto.SetOwnNonEnumerable("deref", vm.NewNativeFunction(0, false, "deref", func(args []vm.Value) (vm.Value, error) {
		thisWeakRef := vmInstance.GetThis()

		if thisWeakRef.Type() != vm.TypeWeakRef {
			return vm.Undefined, vmInstance.NewTypeError("WeakRef.prototype.deref called on non-WeakRef")
		}

		weakRefObj := thisWeakRef.AsWeakRef()
		return weakRefObj.Deref(), nil
	}))
	if v, ok := weakRefProto.GetOwn("deref"); ok {
		w, e, c := true, false, true
		weakRefProto.DefineOwnProperty("deref", v, &w, &e, &c)
	}

	// Add [Symbol.toStringTag] property
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		falseVal := false
		weakRefProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("WeakRef"),
			&falseVal, // writable: false
			&falseVal, // enumerable: false
			&falseVal, // configurable: false
		)
	}

	// Create WeakRef constructor function
	// Per ECMAScript spec: new WeakRef(target)
	// - target must be an object
	// - returns a new WeakRef holding a weak reference to target
	weakRefConstructor := vm.NewConstructorWithProps(1, false, "WeakRef", func(args []vm.Value) (vm.Value, error) {
		// Must be called with new
		newTarget := vmInstance.GetNewTarget()
		if newTarget.Type() == vm.TypeUndefined {
			return vm.Undefined, vmInstance.NewTypeError("Constructor WeakRef requires 'new'")
		}

		// Get target argument
		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("WeakRef constructor requires a target")
		}

		target := args[0]

		// Target must be an object
		if !target.IsObject() {
			return vm.Undefined, vmInstance.NewTypeError("WeakRef constructor argument must be an object")
		}

		// Get the prototype from newTarget using GetPrototypeFromConstructor
		// This implements ECMAScript spec behavior for cross-realm construction
		var prototype vm.Value
		if newTarget.Type() != vm.TypeUndefined {
			prototype = vmInstance.GetPrototypeFromConstructor(newTarget, "%WeakRefPrototype%")
		} else {
			prototype = vmInstance.WeakRefPrototype
		}

		// Create new WeakRef instance with the determined prototype
		newWeakRef := vm.NewWeakRefWithPrototype(target, prototype)

		return newWeakRef, nil
	})

	// Set constructor property on WeakRef.prototype
	weakRefProto.SetOwnNonEnumerable("constructor", weakRefConstructor)
	if v, ok := weakRefProto.GetOwn("constructor"); ok {
		w, e, c := true, false, true
		weakRefProto.DefineOwnProperty("constructor", v, &w, &e, &c)
	}

	// Set WeakRef.prototype in VM
	vmInstance.WeakRefPrototype = vm.NewValueFromPlainObject(weakRefProto)

	// Add prototype property to constructor
	weakRefConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vmInstance.WeakRefPrototype)
	if v, ok := weakRefConstructor.AsNativeFunctionWithProps().Properties.GetOwn("prototype"); ok {
		w, e, c := false, false, false
		weakRefConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("prototype", v, &w, &e, &c)
	}

	// Define WeakRef constructor in global scope
	return ctx.DefineGlobal("WeakRef", weakRefConstructor)
}

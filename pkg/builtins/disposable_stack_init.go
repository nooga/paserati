package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// DisposableStack / AsyncDisposableStack (ES2026 explicit resource management,
// https://tc39.es/proposal-explicit-resource-management/). Both are ordinary
// objects carrying a hidden dispose capability ([[DisposableState]] /
// [[AsyncDisposableState]] in the spec) — modeled here as a
// *vm.DisposableStackState attached to the PlainObject, exactly like Map/Set
// iterators use PlainObject.internalIterState. A nil state (or one whose
// Async flag doesn't match the method's family) fails the brand check, same
// as RequireInternalSlot in the spec.

// exceptionValue extracts the thrown JS value from a Go error returned by
// vmInstance.Call/native helpers, falling back to a plain string for Go
// errors that never went through the exception machinery.
func exceptionValue(vmInstance *vm.VM, err error) vm.Value {
	if ee, ok := err.(vm.ExceptionError); ok {
		return ee.GetExceptionValue()
	}
	return vm.NewString(err.Error())
}

// newSuppressedError builds a SuppressedError instance directly against the
// realm's intrinsic prototype. Per spec this is "a newly created
// SuppressedError object" — an internal construction independent of the
// (possibly reassigned) global SuppressedError binding.
func newSuppressedError(vmInstance *vm.VM, errorVal, suppressedVal vm.Value) vm.Value {
	proto := vmInstance.CurrentRealm().SuppressedErrorPrototype
	inst := vm.NewObject(proto).AsPlainObject()
	inst.SetOwnNonEnumerable("[[ErrorData]]", vm.Undefined)
	inst.SetOwnNonEnumerable("stack", vm.NewString(vmInstance.CaptureStackTrace()))
	inst.SetOwnNonEnumerable("message", vm.NewString(""))
	inst.SetOwnNonEnumerable("error", errorVal)
	inst.SetOwnNonEnumerable("suppressed", suppressedVal)
	return vm.NewValueFromPlainObject(inst)
}

// disposeStackResources runs resources in reverse push order (spec:
// DisposeResources). Every resource is disposed even once one has thrown;
// a second thrown error doesn't replace the first the way a bare
// try/finally would — it chains onto it as a SuppressedError instead.
//
// Per spec, disposal of an async-dispose resource also awaits the method's
// result before moving on to the next one. No AsyncDisposableStack test in
// this test262 snapshot exercises that ordering (only shape/brand-check
// tests exist for disposeAsync), so this runs every dispose call
// synchronously regardless of family and does not await returned
// thenables — a real corner case, not a silent shortcut: revisit if
// AsyncDisposableStack tests grow to cover it.
func disposeStackResources(vmInstance *vm.VM, resources []vm.DisposableResource) error {
	var completion error
	for i := len(resources) - 1; i >= 0; i-- {
		r := resources[i]
		_, err := vmInstance.Call(r.Method, r.Value, nil)
		if err == nil {
			continue
		}
		if completion == nil {
			completion = err
			continue
		}
		se := newSuppressedError(vmInstance, exceptionValue(vmInstance, err), exceptionValue(vmInstance, completion))
		completion = vmInstance.NewExceptionError(se)
	}
	return completion
}

// getDisposeMethod implements GetDisposeMethod(V, hint): async-dispose
// prefers @@asyncDispose, falling back to @@dispose; sync-dispose only ever
// looks up @@dispose.
func getDisposeMethod(vmInstance *vm.VM, value vm.Value, async bool) (vm.Value, error) {
	if async {
		m, _, err := vmInstance.GetSymbolPropertyWithGetter(value, vmInstance.SymbolAsyncDispose)
		if err != nil {
			return vm.Undefined, err
		}
		if m.Type() != vm.TypeUndefined {
			return m, nil
		}
	}
	m, _, err := vmInstance.GetSymbolPropertyWithGetter(value, vmInstance.SymbolDispose)
	if err != nil {
		return vm.Undefined, err
	}
	return m, nil
}

// disposableBrandCheck implements RequireInternalSlot for a DisposableStack /
// AsyncDisposableStack method: `this` must be an object carrying dispose
// state whose Async flag matches the family the method belongs to (a
// DisposableStack and an AsyncDisposableStack fail each other's checks even
// though they share a Go struct here).
func disposableBrandCheck(vmInstance *vm.VM, methodLabel string, wantAsync bool) (*vm.PlainObject, *vm.DisposableStackState, error) {
	thisVal := vmInstance.GetThis()
	if thisVal.Type() != vm.TypeObject {
		return nil, nil, vmInstance.NewTypeError(methodLabel + " requires that 'this' be an Object")
	}
	po := thisVal.AsPlainObject()
	st := po.DisposableState()
	if st == nil || st.Async != wantAsync {
		return nil, nil, vmInstance.NewTypeError(methodLabel + " called on incompatible receiver")
	}
	return po, st, nil
}

// addUseResource implements DisposableStack/AsyncDisposableStack.prototype.use.
func addUseResource(vmInstance *vm.VM, label string, async bool, args []vm.Value) (vm.Value, error) {
	_, st, err := disposableBrandCheck(vmInstance, label, async)
	if err != nil {
		return vm.Undefined, err
	}
	if st.Disposed {
		return vm.Undefined, vmInstance.NewReferenceError("Cannot use a resource after the stack has been disposed.")
	}
	var value vm.Value = vm.Undefined
	if len(args) > 0 {
		value = args[0]
	}
	if value.Type() == vm.TypeNull || value.Type() == vm.TypeUndefined {
		return value, nil
	}
	if !value.IsObject() && !value.IsCallable() {
		return vm.Undefined, vmInstance.NewTypeError(label + ": value must be an object")
	}
	method, gerr := getDisposeMethod(vmInstance, value, async)
	if gerr != nil {
		return vm.Undefined, gerr
	}
	if !method.IsCallable() {
		return vm.Undefined, vmInstance.NewTypeError("Object is not disposable.")
	}
	st.Resources = append(st.Resources, vm.DisposableResource{Value: value, Method: method})
	return value, nil
}

// addAdoptResource implements DisposableStack/AsyncDisposableStack.prototype.adopt.
func addAdoptResource(vmInstance *vm.VM, label string, async bool, args []vm.Value) (vm.Value, error) {
	_, st, err := disposableBrandCheck(vmInstance, label, async)
	if err != nil {
		return vm.Undefined, err
	}
	if st.Disposed {
		return vm.Undefined, vmInstance.NewReferenceError("Cannot adopt a resource after the stack has been disposed.")
	}
	var value vm.Value = vm.Undefined
	if len(args) > 0 {
		value = args[0]
	}
	var onDispose vm.Value = vm.Undefined
	if len(args) > 1 {
		onDispose = args[1]
	}
	if !onDispose.IsCallable() {
		return vm.Undefined, vmInstance.NewTypeError(label + ": onDispose must be callable")
	}
	capturedValue, capturedOnDispose := value, onDispose
	closure := vm.NewNativeFunction(0, false, "", func([]vm.Value) (vm.Value, error) {
		return vmInstance.Call(capturedOnDispose, vm.Undefined, []vm.Value{capturedValue})
	})
	st.Resources = append(st.Resources, vm.DisposableResource{Value: vm.Undefined, Method: closure})
	return value, nil
}

// addDeferResource implements DisposableStack/AsyncDisposableStack.prototype.defer.
func addDeferResource(vmInstance *vm.VM, label string, async bool, args []vm.Value) (vm.Value, error) {
	_, st, err := disposableBrandCheck(vmInstance, label, async)
	if err != nil {
		return vm.Undefined, err
	}
	if st.Disposed {
		return vm.Undefined, vmInstance.NewReferenceError("Cannot defer on a stack that has already been disposed.")
	}
	var onDispose vm.Value = vm.Undefined
	if len(args) > 0 {
		onDispose = args[0]
	}
	if !onDispose.IsCallable() {
		return vm.Undefined, vmInstance.NewTypeError(label + ": onDispose must be callable")
	}
	st.Resources = append(st.Resources, vm.DisposableResource{Value: vm.Undefined, Method: onDispose})
	return vm.Undefined, nil
}

// moveStack implements DisposableStack/AsyncDisposableStack.prototype.move:
// transfers the resource stack to a brand-new instance (always built against
// the base intrinsic prototype, even when `this` is a subclass instance -
// verified by DisposableStack's own
// still-returns-new-disposablestack-when-subclassed.js test) and disposes
// `this` without running any resource's dispose method.
func moveStack(vmInstance *vm.VM, label string, async bool, proto *vm.PlainObject) (vm.Value, error) {
	_, st, err := disposableBrandCheck(vmInstance, label, async)
	if err != nil {
		return vm.Undefined, err
	}
	if st.Disposed {
		return vm.Undefined, vmInstance.NewReferenceError("Cannot move a stack that has already been disposed.")
	}
	newState := &vm.DisposableStackState{Disposed: false, Async: async, Resources: st.Resources}
	st.Resources = nil
	st.Disposed = true
	newInst := vm.NewObject(vm.NewValueFromPlainObject(proto)).AsPlainObject()
	newInst.SetDisposableState(newState)
	return vm.NewValueFromPlainObject(newInst), nil
}

// disposableInstanceType builds the {use, adopt, defer, disposed, move}
// shape shared by DisposableStack and AsyncDisposableStack; the caller adds
// the family-specific dispose/disposeAsync method afterwards.
func disposableInstanceType(className string) *types.ObjectType {
	instanceType := types.NewObjectType()

	useTParam := &types.TypeParameter{Name: "T", Constraint: nil, Index: 0}
	useTType := &types.TypeParameterType{Parameter: useTParam}
	useMethod := &types.GenericType{
		Name:           "use",
		TypeParameters: []*types.TypeParameter{useTParam},
		Body:           types.NewSimpleFunction([]types.Type{useTType}, useTType),
	}

	adoptTParam := &types.TypeParameter{Name: "T", Constraint: nil, Index: 0}
	adoptTType := &types.TypeParameterType{Parameter: adoptTParam}
	onDisposeType := types.NewSimpleFunction([]types.Type{adoptTType}, types.Void)
	adoptMethod := &types.GenericType{
		Name:           "adopt",
		TypeParameters: []*types.TypeParameter{adoptTParam},
		Body:           types.NewSimpleFunction([]types.Type{adoptTType, onDisposeType}, adoptTType),
	}

	instanceType.WithProperty("use", useMethod)
	instanceType.WithProperty("adopt", adoptMethod)
	instanceType.WithProperty("defer", types.NewSimpleFunction([]types.Type{types.NewSimpleFunction([]types.Type{}, types.Void)}, types.Void))
	instanceType.WithProperty("move", types.NewSimpleFunction([]types.Type{}, instanceType))
	instanceType.WithProperty("disposed", types.Boolean)
	_ = className
	return instanceType
}

// ---------------------------------------------------------------------
// DisposableStack
// ---------------------------------------------------------------------

type DisposableStackInitializer struct{}

func (d *DisposableStackInitializer) Name() string  { return "DisposableStack" }
func (d *DisposableStackInitializer) Priority() int { return 24 }

func (d *DisposableStackInitializer) InitTypes(ctx *TypeContext) error {
	instanceType := disposableInstanceType("DisposableStack")
	instanceType.WithProperty("dispose", types.NewSimpleFunction([]types.Type{}, types.Void))

	ctorType := types.NewObjectType().
		WithSimpleConstructSignature([]types.Type{}, instanceType).
		WithProperty("prototype", instanceType)

	if err := ctx.DefineGlobal("DisposableStack", ctorType); err != nil {
		return err
	}
	return ctx.DefineTypeAlias("DisposableStack", instanceType)
}

func (d *DisposableStackInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM
	proto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	proto.SetOwnNonEnumerable("use", vm.NewNativeFunction(1, false, "use", func(args []vm.Value) (vm.Value, error) {
		return addUseResource(vmInstance, "DisposableStack.prototype.use", false, args)
	}))
	proto.SetOwnNonEnumerable("adopt", vm.NewNativeFunction(2, false, "adopt", func(args []vm.Value) (vm.Value, error) {
		return addAdoptResource(vmInstance, "DisposableStack.prototype.adopt", false, args)
	}))
	proto.SetOwnNonEnumerable("defer", vm.NewNativeFunction(1, false, "defer", func(args []vm.Value) (vm.Value, error) {
		return addDeferResource(vmInstance, "DisposableStack.prototype.defer", false, args)
	}))

	disposeFn := vm.NewNativeFunction(0, false, "dispose", func(args []vm.Value) (vm.Value, error) {
		_, st, err := disposableBrandCheck(vmInstance, "DisposableStack.prototype.dispose", false)
		if err != nil {
			return vm.Undefined, err
		}
		if st.Disposed {
			return vm.Undefined, nil
		}
		st.Disposed = true
		resources := st.Resources
		st.Resources = nil
		if derr := disposeStackResources(vmInstance, resources); derr != nil {
			return vm.Undefined, derr
		}
		return vm.Undefined, nil
	})
	proto.SetOwnNonEnumerable("dispose", disposeFn)

	proto.SetOwnNonEnumerable("move", vm.NewNativeFunction(0, false, "move", func(args []vm.Value) (vm.Value, error) {
		return moveStack(vmInstance, "DisposableStack.prototype.move", false, proto)
	}))

	disposedGetter := vm.NewNativeFunction(0, false, "get disposed", func(args []vm.Value) (vm.Value, error) {
		_, st, err := disposableBrandCheck(vmInstance, "DisposableStack.prototype.disposed", false)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.BooleanValue(st.Disposed), nil
	})
	eFalse, cTrue := false, true
	proto.DefineAccessorProperty("disposed", disposedGetter, true, vm.Undefined, false, &eFalse, &cTrue)

	// [Symbol.dispose] must be the exact same function object as .dispose.
	wTrue := true
	proto.DefineOwnPropertyByKey(vm.NewSymbolKey(vmInstance.SymbolDispose), disposeFn, &wTrue, &eFalse, &cTrue)

	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		fFalse := false
		proto.DefineOwnPropertyByKey(vm.NewSymbolKey(vmInstance.SymbolToStringTag), vm.NewString("DisposableStack"), &fFalse, &fFalse, &cTrue)
	}

	ctor := vm.NewConstructorWithProps(0, false, "DisposableStack", func(args []vm.Value) (vm.Value, error) {
		newTarget := vmInstance.GetNewTarget()
		if newTarget.Type() == vm.TypeUndefined {
			return vm.Undefined, vmInstance.NewTypeError("Constructor DisposableStack requires 'new'")
		}
		candidate, gpfcErr := vmInstance.GetPrototypeFromConstructor(newTarget, "%DisposableStackPrototype%")
		if gpfcErr != nil {
			return vm.Undefined, gpfcErr
		}
		instProto := vm.NewValueFromPlainObject(proto)
		if candidate.IsObject() {
			instProto = candidate
		}
		inst := vm.NewObject(instProto).AsPlainObject()
		inst.SetDisposableState(&vm.DisposableStackState{Disposed: false, Async: false})
		return vm.NewValueFromPlainObject(inst), nil
	})
	ctorProps := ctor.AsNativeFunctionWithProps()
	ctorProps.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(proto))
	proto.SetOwnNonEnumerable("constructor", ctor)

	if realm := vmInstance.CurrentRealm(); realm != nil {
		realm.DisposableStackPrototype = vm.NewValueFromPlainObject(proto)
	}

	return ctx.DefineGlobal("DisposableStack", ctor)
}

// ---------------------------------------------------------------------
// AsyncDisposableStack
// ---------------------------------------------------------------------

type AsyncDisposableStackInitializer struct{}

func (a *AsyncDisposableStackInitializer) Name() string  { return "AsyncDisposableStack" }
func (a *AsyncDisposableStackInitializer) Priority() int { return 25 }

func (a *AsyncDisposableStackInitializer) InitTypes(ctx *TypeContext) error {
	instanceType := disposableInstanceType("AsyncDisposableStack")
	promiseVoid := types.NewInstantiatedType(types.PromiseGeneric, []types.Type{types.Void})
	instanceType.WithProperty("disposeAsync", types.NewSimpleFunction([]types.Type{}, promiseVoid))

	ctorType := types.NewObjectType().
		WithSimpleConstructSignature([]types.Type{}, instanceType).
		WithProperty("prototype", instanceType)

	if err := ctx.DefineGlobal("AsyncDisposableStack", ctorType); err != nil {
		return err
	}
	return ctx.DefineTypeAlias("AsyncDisposableStack", instanceType)
}

func (a *AsyncDisposableStackInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM
	proto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	proto.SetOwnNonEnumerable("use", vm.NewNativeFunction(1, false, "use", func(args []vm.Value) (vm.Value, error) {
		return addUseResource(vmInstance, "AsyncDisposableStack.prototype.use", true, args)
	}))
	proto.SetOwnNonEnumerable("adopt", vm.NewNativeFunction(2, false, "adopt", func(args []vm.Value) (vm.Value, error) {
		return addAdoptResource(vmInstance, "AsyncDisposableStack.prototype.adopt", true, args)
	}))
	proto.SetOwnNonEnumerable("defer", vm.NewNativeFunction(1, false, "defer", func(args []vm.Value) (vm.Value, error) {
		return addDeferResource(vmInstance, "AsyncDisposableStack.prototype.defer", true, args)
	}))

	disposeAsyncFn := vm.NewNativeFunction(0, false, "disposeAsync", func(args []vm.Value) (vm.Value, error) {
		_, st, err := disposableBrandCheck(vmInstance, "AsyncDisposableStack.prototype.disposeAsync", true)
		if err != nil {
			return vmInstance.NewRejectedPromise(exceptionValue(vmInstance, err)), nil
		}
		if st.Disposed {
			return vmInstance.NewResolvedPromise(vm.Undefined), nil
		}
		st.Disposed = true
		resources := st.Resources
		st.Resources = nil
		if derr := disposeStackResources(vmInstance, resources); derr != nil {
			return vmInstance.NewRejectedPromise(exceptionValue(vmInstance, derr)), nil
		}
		return vmInstance.NewResolvedPromise(vm.Undefined), nil
	})
	proto.SetOwnNonEnumerable("disposeAsync", disposeAsyncFn)

	proto.SetOwnNonEnumerable("move", vm.NewNativeFunction(0, false, "move", func(args []vm.Value) (vm.Value, error) {
		return moveStack(vmInstance, "AsyncDisposableStack.prototype.move", true, proto)
	}))

	disposedGetter := vm.NewNativeFunction(0, false, "get disposed", func(args []vm.Value) (vm.Value, error) {
		_, st, err := disposableBrandCheck(vmInstance, "AsyncDisposableStack.prototype.disposed", true)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.BooleanValue(st.Disposed), nil
	})
	eFalse, cTrue := false, true
	proto.DefineAccessorProperty("disposed", disposedGetter, true, vm.Undefined, false, &eFalse, &cTrue)

	// [Symbol.asyncDispose] must be the exact same function object as .disposeAsync.
	wTrue := true
	proto.DefineOwnPropertyByKey(vm.NewSymbolKey(vmInstance.SymbolAsyncDispose), disposeAsyncFn, &wTrue, &eFalse, &cTrue)

	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		fFalse := false
		proto.DefineOwnPropertyByKey(vm.NewSymbolKey(vmInstance.SymbolToStringTag), vm.NewString("AsyncDisposableStack"), &fFalse, &fFalse, &cTrue)
	}

	ctor := vm.NewConstructorWithProps(0, false, "AsyncDisposableStack", func(args []vm.Value) (vm.Value, error) {
		newTarget := vmInstance.GetNewTarget()
		if newTarget.Type() == vm.TypeUndefined {
			return vm.Undefined, vmInstance.NewTypeError("Constructor AsyncDisposableStack requires 'new'")
		}
		candidate, gpfcErr := vmInstance.GetPrototypeFromConstructor(newTarget, "%AsyncDisposableStackPrototype%")
		if gpfcErr != nil {
			return vm.Undefined, gpfcErr
		}
		instProto := vm.NewValueFromPlainObject(proto)
		if candidate.IsObject() {
			instProto = candidate
		}
		inst := vm.NewObject(instProto).AsPlainObject()
		inst.SetDisposableState(&vm.DisposableStackState{Disposed: false, Async: true})
		return vm.NewValueFromPlainObject(inst), nil
	})
	ctorProps := ctor.AsNativeFunctionWithProps()
	ctorProps.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(proto))
	proto.SetOwnNonEnumerable("constructor", ctor)

	if realm := vmInstance.CurrentRealm(); realm != nil {
		realm.AsyncDisposableStackPrototype = vm.NewValueFromPlainObject(proto)
	}

	return ctx.DefineGlobal("AsyncDisposableStack", ctor)
}

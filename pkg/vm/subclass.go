package vm

// Subclassing native constructors (ECMAScript "class Sub extends NativeBase {}").
//
// When `new Sub()` runs, the base native constructor builds and returns an exotic
// object carrying the *intrinsic* prototype (e.g. Boolean.prototype). Per spec the
// instance's [[Prototype]] must instead be newTarget.prototype (Sub.prototype),
// which OrdinaryCreateFromConstructor would have selected. Most native ctors here
// hardcode the intrinsic, so the VM applies the override after super() returns.
//
// Storage: PlainObject/DictObject/Array/Map/Set/WeakRef/WeakMap and FunctionObject
// carry their own [[Prototype]] field; the remaining exotic types (RegExp,
// ArrayBuffer, SharedArrayBuffer, DataView, TypedArray, Promise, WeakSet) gained a
// per-instance `prototype` field for this purpose. Undefined means "use intrinsic".

// applySubclassPrototype sets instance's per-instance [[Prototype]] to
// newTarget's "prototype" property. Callers must only invoke this for genuine
// subclass super() calls (newTarget is the subclass, not the base native ctor).
// No-op when newTarget has no object "prototype", or when the instance type has
// no settable per-instance prototype.
func (vm *VM) applySubclassPrototype(instance Value, newTarget Value) {
	if !newTarget.IsCallable() {
		return
	}
	proto, err := vm.GetProperty(newTarget, "prototype")
	if err != nil || !proto.IsObject() {
		return
	}
	switch instance.Type() {
	case TypeObject:
		instance.AsPlainObject().SetPrototype(proto)
	case TypeDictObject:
		instance.AsDictObject().SetPrototype(proto)
	case TypeArray:
		instance.AsArray().SetPrototype(proto)
	case TypeMap:
		instance.AsMap().SetPrototype(proto)
	case TypeSet:
		instance.AsSet().SetPrototype(proto)
	case TypeWeakMap:
		instance.AsWeakMap().SetPrototype(proto)
	case TypeWeakSet:
		instance.AsWeakSet().SetPrototype(proto)
	case TypeRegExp:
		instance.AsRegExpObject().SetPrototype(proto)
	case TypeArrayBuffer:
		instance.AsArrayBuffer().SetPrototype(proto)
	case TypeSharedArrayBuffer:
		instance.AsSharedArrayBuffer().SetPrototype(proto)
	case TypeDataView:
		instance.AsDataView().SetPrototype(proto)
	case TypeTypedArray:
		instance.AsTypedArray().SetPrototype(proto)
	case TypePromise:
		instance.AsPromise().SetPrototype(proto)
	case TypeWeakRef:
		instance.AsWeakRef().SetPrototype(proto)
	case TypeFinalizationRegistry:
		instance.AsFinalizationRegistry().SetPrototype(proto)
	case TypeFunction:
		instance.AsFunction().subclassPrototype = proto
	}
}

// InstancePrototypeOverride returns the per-instance [[Prototype]] override set
// on v by subclassing a native constructor, and true if one is present (an
// object). Types that store [[Prototype]] inline (PlainObject/DictObject) are
// handled directly by callers and return false here.
func (vm *VM) InstancePrototypeOverride(v Value) (Value, bool) {
	var p Value
	switch v.Type() {
	case TypeArray:
		p = v.AsArray().GetPrototype()
	case TypeMap:
		p = v.AsMap().GetPrototype()
	case TypeSet:
		p = v.AsSet().GetPrototype()
	case TypeWeakMap:
		p = v.AsWeakMap().GetPrototype()
	case TypeWeakSet:
		p = v.AsWeakSet().GetPrototype()
	case TypeWeakRef:
		p = v.AsWeakRef().GetPrototype()
	case TypeFinalizationRegistry:
		p = v.AsFinalizationRegistry().GetPrototype()
	case TypeRegExp:
		p = v.AsRegExpObject().GetPrototype()
	case TypeArrayBuffer:
		p = v.AsArrayBuffer().GetPrototype()
	case TypeSharedArrayBuffer:
		p = v.AsSharedArrayBuffer().GetPrototype()
	case TypeDataView:
		p = v.AsDataView().GetPrototype()
	case TypeTypedArray:
		p = v.AsTypedArray().GetPrototype()
	case TypePromise:
		p = v.AsPromise().GetPrototype()
	case TypeFunction:
		p = v.AsFunction().subclassPrototype
	default:
		return Undefined, false
	}
	// The zero Value is TypeUndefined ("no override set" - the field's
	// natural default), and applySubclassPrototype only ever stores an
	// object. But Object.setPrototypeOf legitimately stores an explicit
	// TypeNull override too (setPrototypeOf(v, null) - see
	// objectSetPrototypeOfWithVM in pkg/builtins/object_init.go), which
	// `p.IsObject()` alone would misreport as "no override", falling back
	// to the intrinsic prototype instead of the null the caller asked for
	// (#418). Undefined is never a legitimately stored override, so "not
	// Undefined" is the correct presence check for both cases.
	if p.Type() != TypeUndefined {
		return p, true
	}
	return Undefined, false
}

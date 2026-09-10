package vm

// Symbol-keyed property lookup across the callable object flavours.
//
// Historically every branch of opGetPropSymbol re-implemented its own walk,
// and only TypeObject/TypeNativeFunction ever invoked an accessor's getter.
// That made a symbol-keyed accessor on a constructor (the shape every
// well-known-symbol accessor takes, e.g. `get [Symbol.species]`) unreadable:
// `RegExp[Symbol.species]` returned undefined even though the accessor was
// correctly defined and `descriptor.get.call(RegExp)` worked. Inherited
// accessors were worse off still - lookupSymbolOnProtoChain took no receiver,
// so it structurally could not call a getter with the right `this`, which is
// exactly what `Uint8Array[Symbol.species]` (inherited from %TypedArray%) and
// `class Foo extends Uint8Array {}` -> `Foo[Symbol.species]` need.
//
// The helpers here centralise both halves: resolving the slot along the real
// [[Prototype]] chain, and invoking a found getter with the original receiver.

// symbolSlot is the result of resolving a symbol-keyed property along a
// prototype chain. When found is true exactly one of the accessor/data halves
// is meaningful: isAccessor selects which.
type symbolSlot struct {
	found      bool
	isAccessor bool
	getter     Value // meaningful when isAccessor; Undefined for a setter-only accessor
	value      Value // meaningful when !isAccessor
}

// symbolPropsAndProto returns the own property table(s) backing v - empty when
// it has none yet, and two for a closure, whose lazily-created per-closure
// table sits in front of its shared FunctionObject's - along with v's
// [[Prototype]]. It mirrors getPrototypeOfValue's function
// cases in pkg/builtins/object_init.go: a class constructor's [[Prototype]] is
// its FunctionObject.Prototype (set to the superclass by `extends`), and a
// built-in constructor's is its Properties table's prototype - which is how
// Uint8Array reaches %TypedArray%. Returning FunctionPrototype as the fallback
// keeps plain functions terminating on Function.prototype as before.
func (vm *VM) symbolPropsAndProto(v Value) ([]*PlainObject, Value, bool) {
	switch v.Type() {
	case TypeObject:
		po := v.AsPlainObject()
		if po == nil {
			return nil, Undefined, false
		}
		return []*PlainObject{po}, po.GetPrototype(), true
	case TypeDictObject:
		// DictObject has no symbol-key storage; only its prototype matters.
		d := v.AsDictObject()
		if d == nil {
			return nil, Undefined, false
		}
		return nil, d.GetPrototype(), true
	case TypeFunction:
		fn := v.AsFunction()
		if fn == nil {
			return nil, Undefined, false
		}
		return []*PlainObject{fn.Properties}, vm.functionProtoOrDefault(fn.Prototype), true
	case TypeClosure:
		cl := v.AsClosure()
		if cl == nil {
			return nil, Undefined, false
		}
		proto := Undefined
		if cl.Fn != nil {
			proto = cl.Fn.Prototype
		}
		// A closure's own Properties shadow its FunctionObject's, but both are
		// consulted: the per-closure table is created lazily, so a property
		// installed on the shared FunctionObject still has to be visible once
		// the closure grows a table of its own.
		var tables []*PlainObject
		if cl.Properties != nil {
			tables = append(tables, cl.Properties)
		}
		if cl.Fn != nil && cl.Fn.Properties != nil {
			tables = append(tables, cl.Fn.Properties)
		}
		return tables, vm.functionProtoOrDefault(proto), true
	case TypeNativeFunction:
		nf := v.AsNativeFunction()
		if nf == nil {
			return nil, Undefined, false
		}
		return []*PlainObject{nf.Properties}, vm.FunctionPrototype, true
	case TypeNativeFunctionWithProps:
		nfp := v.AsNativeFunctionWithProps()
		if nfp == nil {
			return nil, Undefined, false
		}
		if nfp.Properties == nil {
			return nil, vm.FunctionPrototype, true
		}
		proto := nfp.Properties.GetPrototype()
		if proto.Type() == TypeUndefined || proto.Type() == TypeNull || proto == DefaultObjectPrototype {
			// Built-ins default to Function.prototype unless a custom
			// [[Prototype]] was installed (e.g. the TypedArray constructors,
			// whose Properties prototype is %TypedArray%).
			proto = vm.FunctionPrototype
		}
		return []*PlainObject{nfp.Properties}, proto, true
	case TypeBoundFunction:
		bf := v.AsBoundFunction()
		if bf == nil {
			return nil, Undefined, false
		}
		return []*PlainObject{bf.Properties}, vm.FunctionPrototype, true
	}
	return nil, Undefined, false
}

// functionProtoOrDefault normalises an unset function [[Prototype]] to
// Function.prototype so the walk still reaches the intrinsics.
func (vm *VM) functionProtoOrDefault(proto Value) Value {
	if proto.Type() == TypeUndefined || proto.Type() == TypeNull {
		return vm.FunctionPrototype
	}
	return proto
}

// findSymbolSlot resolves key starting at start (inclusive) and walking the
// [[Prototype]] chain, reporting an accessor without invoking it so the caller
// can supply the correct receiver.
func (vm *VM) findSymbolSlot(start Value, key PropertyKey) symbolSlot {
	current := start
	// The depth cap matches the one the per-flavour walks this replaced used:
	// real chains are a handful of links, and a cycle installed via
	// Object.setPrototypeOf (which the proto == current check below only
	// catches when it is a self-loop) should cost as little as possible on a
	// path that now runs for every symbol read on every constructor.
	for i := 0; i < 100; i++ {
		tables, proto, ok := vm.symbolPropsAndProto(current)
		if !ok {
			break
		}
		for _, props := range tables {
			if props == nil {
				continue
			}
			if g, _, _, _, isAcc := props.GetOwnAccessorByKey(key); isAcc {
				return symbolSlot{found: true, isAccessor: true, getter: g}
			}
			if v, exists := props.GetOwnByKey(key); exists {
				return symbolSlot{found: true, value: v}
			}
		}
		if proto.Type() == TypeUndefined || proto.Type() == TypeNull {
			break
		}
		// Self-referential prototype: stop rather than spin.
		if proto == current {
			break
		}
		current = proto
	}
	return symbolSlot{}
}

// resolveSymbolSlot is findSymbolSlot plus getter invocation, returning the
// value in the same (ok, status, value) shape opGetPropSymbol's branches use.
func (vm *VM) resolveSymbolSlot(frame *CallFrame, ip int, frameWasNil bool, base Value, key PropertyKey, dest *Value) (bool, InterpretResult, Value) {
	slot := vm.findSymbolSlot(base, key)
	if !slot.found {
		*dest = Undefined
		return true, InterpretOK, *dest
	}
	if !slot.isAccessor {
		*dest = slot.value
		return true, InterpretOK, *dest
	}
	return vm.invokeSymbolGetter(frame, ip, frameWasNil, slot.getter, base, dest)
}

// invokeSymbolGetter calls getter with `this` = receiver, translating a Go
// error into a thrown VM exception. The frame.ip = ip - 4 rewind matches what
// opGetPropSymbol's existing accessor branches do so a getter that throws
// still reports the property-access site.
func (vm *VM) invokeSymbolGetter(frame *CallFrame, ip int, frameWasNil bool, getter Value, receiver Value, dest *Value) (bool, InterpretResult, Value) {
	if getter.Type() == TypeUndefined {
		// Setter-only accessor: reads as undefined per spec.
		*dest = Undefined
		return true, InterpretOK, *dest
	}
	res, err := vm.Call(getter, receiver, nil)
	if err == nil {
		*dest = res
		return true, InterpretOK, *dest
	}

	var excVal Value
	if ee, ok := err.(ExceptionError); ok {
		excVal = ee.GetExceptionValue()
	} else {
		excVal = vm.errorValueFromGoError(err)
	}
	if frame != nil && !frameWasNil {
		frame.ip = ip - 4
	}
	vm.throwException(excVal)
	if !vm.unwinding {
		return false, InterpretOK, Undefined
	}
	return false, InterpretRuntimeError, Undefined
}

// errorValueFromGoError wraps a non-exception Go error as a JS Error value.
func (vm *VM) errorValueFromGoError(err error) Value {
	if errCtor, ok := vm.GetGlobal("Error"); ok {
		if res, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
			return res
		}
	}
	eo := NewObject(vm.ErrorPrototype).AsPlainObject()
	eo.SetOwn("name", NewString("Error"))
	eo.SetOwn("message", NewString(err.Error()))
	return NewValueFromPlainObject(eo)
}

// symbolProtoOf returns v's [[Prototype]] as used by the symbol-key walk, or
// Undefined for kinds this walk doesn't model.
func (vm *VM) symbolProtoOf(v Value) Value {
	_, proto, ok := vm.symbolPropsAndProto(v)
	if !ok {
		return Undefined
	}
	return proto
}

// lookupSymbolWithReceiver resolves key starting at start (inclusive), walking
// the [[Prototype]] chain and invoking any accessor's getter with `this` =
// receiver. Unlike resolveSymbolSlot it returns a Go error instead of throwing
// into the interpreter loop, for the native-facing callers (Proxy targets,
// GetSymbolPropertyWithGetter) that need to propagate one.
func (vm *VM) lookupSymbolWithReceiver(start Value, key PropertyKey, receiver Value) (Value, bool, error) {
	slot := vm.findSymbolSlot(start, key)
	if !slot.found {
		return Undefined, false, nil
	}
	if !slot.isAccessor {
		return slot.value, true, nil
	}
	if slot.getter.Type() == TypeUndefined {
		return Undefined, true, nil
	}
	res, err := vm.Call(slot.getter, receiver, nil)
	if err != nil {
		if ee, ok := err.(ExceptionError); ok {
			vm.throwException(ee.GetExceptionValue())
		}
		return Undefined, false, err
	}
	return res, true, nil
}

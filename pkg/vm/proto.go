package vm

// Generic [[Prototype]] chain traversal for arbitrary object-kind values.
//
// Most VM chain walks are written for PlainObject/DictObject only, since that
// covers the overwhelming majority of real prototype chains. But any object
// value can legally sit in a prototype chain (`Foo.prototype = new Array(...)`,
// `Bar.prototype = someFunction`, etc.), and code that assumes PlainObject and
// calls AsPlainObject() unconditionally will panic when it hits one. The
// helpers here give those slow/uncommon paths a correct, non-panicking way to
// step through a mixed-kind chain.

// PrototypeOf returns v's [[Prototype]], honoring per-instance overrides set
// by subclassing a native constructor (see subclass.go) and otherwise falling
// back to the realm's intrinsic prototype for v's kind. Returns Undefined for
// values with no [[Prototype]] concept (primitive numbers aside from their
// boxed prototype, null, undefined) — callers should treat that as "chain
// exhausted".
func (vm *VM) PrototypeOf(v Value) Value {
	return vm.prototypeOf(v)
}

func (vm *VM) prototypeOf(v Value) Value {
	if p, ok := vm.InstancePrototypeOverride(v); ok {
		return p
	}
	switch v.Type() {
	case TypeObject:
		return v.AsPlainObject().GetPrototype()
	case TypeDictObject:
		return v.AsDictObject().GetPrototype()
	case TypeArray:
		return vm.ArrayPrototype
	case TypeRegExp:
		return vm.RegExpPrototype
	case TypeMap:
		return vm.MapPrototype
	case TypeSet:
		return vm.SetPrototype
	case TypeWeakMap:
		return vm.WeakMapPrototype
	case TypeWeakSet:
		return vm.WeakSetPrototype
	case TypeWeakRef:
		return vm.WeakRefPrototype
	case TypeFinalizationRegistry:
		return vm.FinalizationRegistryPrototype
	case TypeArrayBuffer:
		return vm.ArrayBufferPrototype
	case TypeSharedArrayBuffer:
		return vm.SharedArrayBufferPrototype
	case TypeDataView:
		return vm.DataViewPrototype
	case TypeTypedArray:
		return vm.TypedArrayPrototypeForKind(v.AsTypedArray().elementType)
	case TypeArguments:
		return vm.ObjectPrototype
	case TypePromise:
		return vm.PromisePrototype
	case TypeFunction:
		fn := v.AsFunction()
		if fn.IsAsync && fn.IsGenerator {
			return vm.AsyncGeneratorFunctionPrototype
		} else if fn.IsGenerator {
			return vm.GeneratorFunctionPrototype
		} else if fn.IsAsync {
			return vm.AsyncFunctionPrototype
		}
		return vm.FunctionPrototype
	case TypeClosure:
		cl := v.AsClosure()
		if cl.Fn.IsAsync && cl.Fn.IsGenerator {
			return vm.AsyncGeneratorFunctionPrototype
		} else if cl.Fn.IsGenerator {
			return vm.GeneratorFunctionPrototype
		} else if cl.Fn.IsAsync {
			return vm.AsyncFunctionPrototype
		}
		return vm.FunctionPrototype
	case TypeNativeFunctionWithProps:
		// A built-in function's [[Prototype]] is Function.prototype unless one
		// was explicitly installed (the TypedArray constructors, whose is
		// %TypedArray%). An untouched Properties table reports the default
		// object prototype, which is not the answer here - leaving it made
		// `Map instanceof Function` false. Mirrors getPrototypeOfValue's
		// TypeNativeFunctionWithProps case (pkg/builtins/object_init.go).
		nfp := v.AsNativeFunctionWithProps()
		if nfp.Properties != nil {
			proto := nfp.Properties.GetPrototype()
			if proto.Type() != TypeUndefined && proto.Type() != TypeNull && proto != DefaultObjectPrototype {
				return proto
			}
		}
		return vm.FunctionPrototype
	case TypeNativeFunction, TypeBoundFunction, TypeAsyncNativeFunction:
		return vm.FunctionPrototype
	case TypeGenerator:
		// In practice this case is unreachable: Prototype is a plain Value
		// now (#418) and InstancePrototypeOverride's own TypeGenerator case
		// (subclass.go) already returns it via the InstancePrototypeOverride
		// check at the top of this function, since call.go always resolves
		// it eagerly at creation time. Kept as a defensive fallback for the
		// same reason the other exotic kinds above have one.
		genObj := v.AsGenerator()
		if genObj.Prototype.Type() != TypeUndefined {
			return genObj.Prototype
		}
		return vm.GeneratorPrototype
	case TypeAsyncGenerator:
		asyncGenObj := v.AsAsyncGenerator()
		if asyncGenObj.Prototype.Type() != TypeUndefined {
			return asyncGenObj.Prototype
		}
		return vm.AsyncGeneratorPrototype
	case TypeString:
		return vm.StringPrototype
	case TypeFloatNumber, TypeIntegerNumber:
		return vm.NumberPrototype
	case TypeBoolean:
		return vm.BooleanPrototype
	case TypeSymbol:
		return vm.SymbolPrototype
	case TypeBigInt:
		return vm.BigIntPrototype
	default:
		return Undefined
	}
}

// GetOwnGeneric looks up an own (non-inherited) property by name on any
// object-kind value. It does not consult accessors beyond what the
// underlying kind's GetOwn already reports as a plain value.
func (vm *VM) GetOwnGeneric(v Value, propName string) (Value, bool) {
	return vm.getOwnGeneric(v, propName)
}

func (vm *VM) getOwnGeneric(v Value, propName string) (Value, bool) {
	switch v.Type() {
	case TypeObject:
		return v.AsPlainObject().GetOwn(propName)
	case TypeDictObject:
		return v.AsDictObject().GetOwn(propName)
	case TypeArray:
		return v.AsArray().GetOwn(propName)
	case TypeClosure:
		cl := v.AsClosure()
		if cl.Properties != nil {
			if val, ok := cl.Properties.GetOwn(propName); ok {
				return val, true
			}
		}
		if cl.Fn.Properties != nil {
			return cl.Fn.Properties.GetOwn(propName)
		}
		return Undefined, false
	case TypeFunction:
		fn := v.AsFunction()
		if fn.Properties != nil {
			return fn.Properties.GetOwn(propName)
		}
		return Undefined, false
	case TypeNativeFunctionWithProps:
		nfp := v.AsNativeFunctionWithProps()
		if nfp.Properties != nil {
			return nfp.Properties.GetOwn(propName)
		}
		return Undefined, false
	case TypeNativeFunction:
		nf := v.AsNativeFunction()
		if nf != nil && nf.Properties != nil {
			return nf.Properties.GetOwn(propName)
		}
		return Undefined, false
	case TypeBoundFunction:
		bf := v.AsBoundFunction()
		if bf != nil && bf.Properties != nil {
			return bf.Properties.GetOwn(propName)
		}
		return Undefined, false
	default:
		// Other object kinds used as ad-hoc prototypes (RegExp, Map, Set,
		// Promise, TypedArray, ...) don't carry extra own properties in this
		// VM's model beyond their intrinsic .prototype object, which is
		// reached separately via prototypeOf.
		return Undefined, false
	}
}

// GetInheritedGeneric walks start's own properties and then its full
// [[Prototype]] chain (of any object kind) looking for propName. It is an
// uncached, correctness-first fallback for the uncommon case where a
// prototype chain contains something other than a PlainObject/DictObject
// (e.g. `Foo.prototype = new Array(...)` or `Foo.prototype = someFunction`).
func (vm *VM) GetInheritedGeneric(start Value, propName string) (Value, bool) {
	return vm.getInheritedGeneric(start, propName)
}

func (vm *VM) getInheritedGeneric(start Value, propName string) (Value, bool) {
	current := start
	for i := 0; i < 200 && current.Type() != TypeNull && current.Type() != TypeUndefined; i++ {
		if v, ok := vm.getOwnGeneric(current, propName); ok {
			return v, true
		}
		current = vm.prototypeOf(current)
	}
	return Undefined, false
}

// TypedArrayPrototypeForKind returns the intrinsic per-kind prototype for a
// typed array element type (%Uint8Array.prototype%, ...). Typed arrays created
// by a constructor carry the right prototype in their per-instance override,
// but ones the runtime builds directly (subarray, slice, %TypedArray%.from, ...)
// leave it Undefined and must be resolved from the element type — falling back
// to the abstract %TypedArray%.prototype makes `u.subarray(0,2) instanceof
// Uint8Array` false.
func (vm *VM) TypedArrayPrototypeForKind(kind TypedArrayKind) Value {
	switch kind {
	case TypedArrayInt8:
		return vm.Int8ArrayPrototype
	case TypedArrayUint8:
		return vm.Uint8ArrayPrototype
	case TypedArrayUint8Clamped:
		return vm.Uint8ClampedArrayPrototype
	case TypedArrayInt16:
		return vm.Int16ArrayPrototype
	case TypedArrayUint16:
		return vm.Uint16ArrayPrototype
	case TypedArrayInt32:
		return vm.Int32ArrayPrototype
	case TypedArrayUint32:
		return vm.Uint32ArrayPrototype
	case TypedArrayFloat16:
		return vm.Float16ArrayPrototype
	case TypedArrayFloat32:
		return vm.Float32ArrayPrototype
	case TypedArrayFloat64:
		return vm.Float64ArrayPrototype
	case TypedArrayBigInt64:
		return vm.BigInt64ArrayPrototype
	case TypedArrayBigUint64:
		return vm.BigUint64ArrayPrototype
	default:
		return vm.TypedArrayPrototype
	}
}

// plainPrototypeOf resolves v's [[Prototype]] for the property-read paths,
// returning nil when it is not a PlainObject.
//
// The read paths walk chains with AsPlainObject, whose pointer reinterpretation
// is only valid for TypeObject. Value.IsObject() is NOT a sufficient guard: it
// is the contiguous [TypeObject, TypeProxy] range check (see the comment on
// getPropertyWithReceiver's TypeObject case), so a Proxy or DictObject standing
// in as a [[Prototype]] - reachable from ordinary code via
// `Reflect.construct(C, args, F)` with an exotic `F.prototype` - passes it and
// then panics the VM. Callers that get nil should treat the chain as exhausted.
func (vm *VM) plainPrototypeOf(v Value) *PlainObject {
	proto := vm.PrototypeOf(v)
	if proto.Type() != TypeObject {
		return nil
	}
	return proto.AsPlainObject()
}

// lookupOnPrototypeChain walks a PlainObject [[Prototype]] chain for a
// string-keyed property, invoking an accessor's getter with this = receiver.
//
// This is the shared form of the walk that the per-kind blocks in opGetProp
// (WeakMap, WeakSet, ArrayBuffer, SharedArrayBuffer, DataView, RegExp) each
// used to hand-roll. Those copies had two defects this fixes: they started from
// the hardcoded intrinsic prototype rather than the instance's own, so a
// subclass's methods were unreachable, and they only ever did GetOwn, so a
// subclass's getter read as undefined.
func (vm *VM) lookupOnPrototypeChain(start *PlainObject, propName string, receiver Value) (Value, bool, error) {
	for current := start; current != nil; {
		if getter, _, _, _, isAccessor := current.GetOwnAccessor(propName); isAccessor {
			if getter.Type() == TypeUndefined {
				// Setter-only accessor reads as undefined per spec.
				return Undefined, true, nil
			}
			res, err := vm.Call(getter, receiver, nil)
			if err != nil {
				return Undefined, false, err
			}
			return res, true, nil
		}
		if v, exists := current.GetOwn(propName); exists {
			return v, true, nil
		}
		protoVal := current.GetPrototype()
		if protoVal.Type() != TypeObject {
			break
		}
		current = protoVal.AsPlainObject()
	}
	return Undefined, false, nil
}

// finishProtoChainGet resolves propName on receiver's own [[Prototype]] chain
// and writes the result - Undefined when absent, per spec - into dest, in
// opGetProp's (ok, status, value) shape. A getter that throws is converted into
// a VM exception the same way opGetPropSymbol's accessor branches do.
func (vm *VM) finishProtoChainGet(frame *CallFrame, ip int, frameWasNil bool, propName string, receiver Value, dest *Value) (bool, InterpretResult, Value) {
	v, found, err := vm.lookupOnPrototypeChain(vm.plainPrototypeOf(receiver), propName, receiver)
	if err != nil {
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
	if found {
		*dest = v
		return true, InterpretOK, *dest
	}
	*dest = Undefined
	return true, InterpretOK, *dest
}

// getOwnInstanceProperty returns an own, non-index property stored directly on
// an exotic instance - the lazily-created side table that a plain
// `promise.foo = 1` / Object.defineProperty writes into for RegExp/Map/Set/
// Promise (and functions), plus the per-kind own storage Array, TypedArray,
// ArrayBuffer and SharedArrayBuffer keep. An own accessor's getter is invoked
// with `this` = objVal.
//
// This exists so a prototype lookup can be preceded by the own-property check
// [[Get]] requires. Without it `p.constructor = C` on a Promise (or Map, Set,
// RegExp, Array) read back as the intrinsic constructor: the own value was
// stored and Object.getOwnPropertyDescriptor could see it, but the read path
// reached the prototype's `constructor` first and stopped there.
func (vm *VM) getOwnInstanceProperty(objVal Value, propName string) (Value, bool) {
	if props := OwnPropertiesTable(objVal); props != nil {
		if g, _, _, _, isAccessor := props.GetOwnAccessor(propName); isAccessor {
			if g.Type() == TypeUndefined {
				return Undefined, true
			}
			res, err := vm.Call(g, objVal, nil)
			if err != nil {
				return Undefined, false
			}
			return res, true
		}
		if v, ok := props.GetOwn(propName); ok {
			return v, true
		}
		return Undefined, false
	}
	switch objVal.Type() {
	case TypeArray:
		if arr := objVal.AsArray(); arr != nil {
			return arr.GetOwn(propName)
		}
	case TypeTypedArray:
		if ta := objVal.AsTypedArray(); ta != nil {
			return ta.GetOwnProperty(propName)
		}
	case TypeArrayBuffer:
		if ab := objVal.AsArrayBuffer(); ab != nil {
			return ab.GetOwnProperty(propName)
		}
	case TypeSharedArrayBuffer:
		if sab := objVal.AsSharedArrayBuffer(); sab != nil {
			return sab.GetOwnProperty(propName)
		}
	}
	return Undefined, false
}

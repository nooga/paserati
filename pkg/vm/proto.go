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

// prototypeOf returns v's [[Prototype]], honoring per-instance overrides set
// by subclassing a native constructor (see subclass.go) and otherwise falling
// back to the realm's intrinsic prototype for v's kind. Returns Undefined for
// values with no [[Prototype]] concept (primitive numbers aside from their
// boxed prototype, null, undefined) — callers should treat that as "chain
// exhausted".
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
		return vm.TypedArrayPrototype
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
		nfp := v.AsNativeFunctionWithProps()
		if nfp.Properties != nil {
			return nfp.Properties.GetPrototype()
		}
		return vm.FunctionPrototype
	case TypeNativeFunction, TypeBoundFunction, TypeAsyncNativeFunction:
		return vm.FunctionPrototype
	case TypeGenerator:
		genObj := v.AsGenerator()
		if genObj.Prototype != nil {
			return NewValueFromPlainObject(genObj.Prototype)
		}
		return vm.GeneratorPrototype
	case TypeAsyncGenerator:
		asyncGenObj := v.AsAsyncGenerator()
		if asyncGenObj.Prototype != nil {
			return NewValueFromPlainObject(asyncGenObj.Prototype)
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

// getOwnGeneric looks up an own (non-inherited) property by name on any
// object-kind value. It does not consult accessors beyond what the
// underlying kind's GetOwn already reports as a plain value.
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

// getInheritedGeneric walks start's own properties and then its full
// [[Prototype]] chain (of any object kind) looking for propName. It is an
// uncached, correctness-first fallback for the uncommon case where a
// prototype chain contains something other than a PlainObject/DictObject
// (e.g. `Foo.prototype = new Array(...)` or `Foo.prototype = someFunction`).
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

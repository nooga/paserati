package builtins

import (
	"strconv"

	"github.com/nooga/paserati/pkg/vm"
)

// This file holds the shared "array-like" accessors used by Array.prototype's
// generic methods (every, some, filter, map, forEach, indexOf, push, splice,
// ...). Per ECMA-262, these methods are intentionally generic: their `this`
// value only needs a "length" property and integer-indexed properties, not
// an actual Array object (22.1.3: "the definition of these methods does not
// require that its this value be an Array object").
//
// The methods used to implement that with a two-branch dance:
//
//	if arr := thisVal.AsArray(); arr != nil {
//	    ... fast path ...
//	} else if po := thisVal.AsPlainObject(); po != nil {
//	    ... array-like fallback ...
//	}
//
// but vm.Value.AsArray()/AsPlainObject() panic on a type mismatch instead of
// returning nil - so the `!= nil` checks were dead code, and the panic fired
// on the very first receiver that was neither a real Array nor a plain
// Object (Arguments, TypedArray, Proxy, boxed primitives, Map/Set/...,
// anything). See issue #46. Fixed for good by centralizing the three
// operations these methods actually need - length, indexed get, indexed set
// - into helpers that dispatch on the concrete type once, in one place, and
// fall back to the VM's ordinary (correct-for-every-type) property-access
// path for anything that isn't a real Array or PlainObject.

// maxSafeInteger is ToLength's cap (2^53 - 1).
const maxSafeInteger = 9007199254740991

// maxArrayLength is the largest valid Array length (2^32 - 1, ECMA-262
// 23.1.4.1's array index bound). ArrayCreate(len) and ArraySetLength both
// throw a RangeError above this - critically, *before* doing any per-element
// work. Array.prototype methods that build a result sized to an array-like's
// declared length (map, toReversed, toSorted, toSpliced, with, the Array(len)
// constructor) MUST check this up front: LengthOfArrayLike/ToLength allows
// lengths up to 2^53-1 for an arbitrary array-like receiver (nothing stops
// `{length: 2**32}.length` from being read), and a bare `for i := 0; i <
// length; i++` loop over a multi-billion length hangs the process long
// before finishing - the RangeError is the spec's mechanism for cutting
// that off immediately rather than requiring a fast per-iteration no-op.
const maxArrayLength = 4294967295

// checkArrayCreateLength returns a RangeError if length exceeds the largest
// valid Array length, matching ArrayCreate's own bounds check. Call this
// immediately after computing a result array's target length and before any
// loop over it - see maxArrayLength's comment for why this can't wait.
func checkArrayCreateLength(vmInstance *vm.VM, length int) error {
	if length > maxArrayLength {
		return vmInstance.NewRangeError("Invalid array length")
	}
	return nil
}

// toLengthInt clamps a raw "length" value the way ToLength does: NaN or <= 0
// becomes 0, anything above 2^53-1 is capped, otherwise truncated to an
// integer. Returned as a plain int since no array-like this codebase deals
// with can plausibly hold more than MaxInt elements anyway.
func toLengthInt(v vm.Value) int {
	n := v.ToFloat()
	if n != n || n <= 0 { // NaN or <= 0
		return 0
	}
	if n > maxSafeInteger {
		n = maxSafeInteger
	}
	return int(n)
}

// toLengthWithVM converts v via the real ECMAScript ToLength algorithm:
// ToNumber(v) - invoking a user-defined valueOf/toString through the VM when
// v is an object, per OrdinaryToPrimitive - clamped to [0, 2^53-1] the same
// way toLengthInt clamps a value already known to be a number. Any exception
// thrown during that conversion (a throwing valueOf/toString, or a Symbol)
// is propagated as ErrVMUnwinding/TypeError instead of silently coercing to
// NaN, mirroring toIntegerOrInfinityWithVM's handling for ToInteger.
func toLengthWithVM(vmInstance *vm.VM, v vm.Value) (int, error) {
	if v.Type() == vm.TypeSymbol {
		return 0, vmInstance.NewTypeError("Cannot convert a Symbol value to a number")
	}
	if !v.IsObject() && !v.IsCallable() {
		return toLengthInt(v), nil
	}
	vmInstance.EnterHelperCall()
	n := vmInstance.ToNumber(v)
	vmInstance.ExitHelperCall()
	if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
		return 0, ErrVMUnwinding
	}
	return toLengthInt(vm.NumberValue(n)), nil
}

// arrayLikeLength returns LengthOfArrayLike(thisVal) (ECMA-262 7.3.20): the
// real Length() for an Array, otherwise ToLength(Get(thisVal, "length")).
// The PlainObject case goes through getOwnPlainObjectProperty so a "length"
// defined as an accessor is invoked correctly rather than silently treated
// as absent (see that helper's comment - this matters for real Test262
// cases, not just theoretical ones). Everything else goes through the VM's
// generic property get, which correctly handles TypedArray/Arguments/Proxy/
// getters/etc. Both branches finish through toLengthWithVM so a "length"
// whose value is itself an object (e.g. `{toString(){return '2'}}`) gets
// ToNumber's real valueOf/toString dispatch rather than silently reading as
// NaN/0 - see toLengthWithVM's comment.
func arrayLikeLength(vmInstance *vm.VM, thisVal vm.Value) (int, error) {
	switch thisVal.Type() {
	case vm.TypeArray:
		return thisVal.AsArray().Length(), nil
	case vm.TypeObject:
		lv, exists, err := getOwnPlainObjectProperty(vmInstance, thisVal.AsPlainObject(), thisVal, "length")
		if err != nil {
			return 0, err
		}
		if !exists {
			return 0, nil
		}
		return toLengthWithVM(vmInstance, lv)
	default:
		lv, err := vmInstance.GetProperty(thisVal, "length")
		if err != nil {
			return 0, err
		}
		return toLengthWithVM(vmInstance, lv)
	}
}

// getOwnPlainObjectProperty reads own property `key` of po (whose Value form
// is `receiver`, used as `this` for an accessor getter), returning
// (value, exists, error). This exists because PlainObject.GetOwn/Get have no
// VM to invoke a getter with, so they silently can't distinguish "own
// accessor property present" from "absent" - which several array-like
// generic methods actually depend on. Test262 exploits this directly: more
// than one near-integer-length test (Array.prototype.{unshift,reverse}, and
// implicitly anything using arrayLikeGet's exists flag) places a *throwing
// getter* at a specific index specifically to stop what would otherwise be
// an iteration up near 2^53-1 after only a handful of steps. Treating that
// accessor as "doesn't exist" (the bug this replaces) skips the getter
// entirely and lets the loop run its full, computationally impossible
// length instead of throwing - which manifested as an actual multi-gigabyte,
// unbounded-runtime hang, not just a wrong answer.
func getOwnPlainObjectProperty(vmInstance *vm.VM, po *vm.PlainObject, receiver vm.Value, key string) (vm.Value, bool, error) {
	if g, _, _, _, ok := po.GetOwnAccessor(key); ok {
		if g.Type() == vm.TypeUndefined {
			return vm.Undefined, true, nil
		}
		v, err := vmInstance.Call(g, receiver, nil)
		if err != nil {
			return vm.Undefined, false, err
		}
		return v, true, nil
	}
	if v, ok := po.GetOwn(key); ok {
		return v, true, nil
	}
	return vm.Undefined, false, nil
}

// arrayIndexGetFromProto walks arr's prototype chain looking for key (a
// canonical array index or any other property name) - the
// OrdinaryHasProperty (9.1.7) + [[Get]] (9.1.8) fallback used once an
// index's own slot has come up empty (a hole, or never set). Returns
// (value, found, error); found mirrors HasProperty so callers can
// distinguish "inherited value is undefined" from "no such property
// anywhere". receiver is passed as `this` to an inherited getter, per
// [[Get]]'s Receiver parameter.
//
// This does its own chain walk with vmInstance.Call rather than delegating
// to vmInstance.GetProperty: GetProperty's own TypeArray fast path (see
// opGetProp) returns arr.Get(idx) - Undefined for a hole - as soon as
// idx < arr.Length(), without ever reaching its own prototype-walk code
// below that check. Duplicating the walk here is what actually reaches the
// prototype for an in-bounds hole.
func arrayIndexGetFromProto(vmInstance *vm.VM, arr *vm.ArrayObject, receiver vm.Value, key string) (vm.Value, bool, error) {
	proto := arr.GetPrototype()
	if proto.Type() != vm.TypeObject {
		proto = vmInstance.ArrayPrototype
	}
	// proto.IsObject() is true for TypeArray/TypeProxy/TypeMap/... too (see
	// Value.IsObject's doc comment) - not just TypeObject/PlainObject, so
	// this must check the concrete type before calling AsPlainObject
	// (which panics on anything else), same as opGetProp's own prototype
	// walk for arrays. A chain member that isn't a PlainObject (a Proxy
	// spliced in via Object.setPrototypeOf, say) ends the walk rather than
	// being consulted - a known gap, not a crash.
	for proto.Type() == vm.TypeObject {
		po := proto.AsPlainObject()
		if g, _, _, _, ok := po.GetOwnAccessor(key); ok {
			if g.Type() == vm.TypeUndefined {
				return vm.Undefined, true, nil
			}
			v, err := vmInstance.Call(g, receiver, nil)
			if err != nil {
				return vm.Undefined, false, err
			}
			return v, true, nil
		}
		if v, ok := po.GetOwn(key); ok {
			return v, true, nil
		}
		proto = po.GetPrototype()
	}
	return vm.Undefined, false, nil
}

// arrayLikeGet returns (value, exists, error) for index i of an array-like
// `this`. "exists" mirrors HasProperty so callers can skip holes in a sparse
// Array or a PlainObject missing that key, matching spec semantics for
// every/filter/forEach/etc. Real Arrays check for an own accessor at that
// index first (set via Object.defineProperty - see ArrayDefineOwnProperty)
// before falling back to the exact hole-checking the previous fast path
// did; PlainObjects go through getOwnPlainObjectProperty for correct
// accessor handling (see its comment); every other type (TypedArray,
// Arguments, Proxy, boxed primitives, Map/Set/...) has no holes to speak
// of, so a plain Get is both correct and simpler.
func arrayLikeGet(vmInstance *vm.VM, thisVal vm.Value, i int) (vm.Value, bool, error) {
	switch thisVal.Type() {
	case vm.TypeArray:
		arr := thisVal.AsArray()
		if arr.HasAccessors() {
			if g, _, _, _, ok := arr.GetOwnAccessor(strconv.Itoa(i)); ok {
				if g.Type() == vm.TypeUndefined {
					return vm.Undefined, true, nil
				}
				v, err := vmInstance.Call(g, thisVal, nil)
				if err != nil {
					return vm.Undefined, false, err
				}
				return v, true, nil
			}
		}
		if arr.HasIndex(i) {
			return arr.Get(i), true, nil
		}
		// Own slot is a hole (or absent, or was `delete`d): [[HasProperty]]
		// (7.3.11 -> OrdinaryHasProperty 9.1.7) doesn't stop at "no own
		// property" - it walks the prototype chain. Array.prototype[i] is
		// rarely set, so only pay for the walk once the fast own-value path
		// above has already missed.
		return arrayIndexGetFromProto(vmInstance, arr, thisVal, strconv.Itoa(i))
	case vm.TypeObject:
		return getOwnPlainObjectProperty(vmInstance, thisVal.AsPlainObject(), thisVal, strconv.Itoa(i))
	case vm.TypeProxy:
		// Proxy needs a real HasProperty check (invoking the "has" trap
		// when present) before Get - unlike the default branch below,
		// which just calls Get and always reports exists=true. That
		// conflation is harmless for types with no genuinely-absent
		// indices, but for a Proxy it means a "has" trap that would throw
		// (or return false) never runs at all, and DeletePropertyOrThrow-
		// based callers like copyWithin never see "this index doesn't
		// exist, delete the destination instead" - see arrayLikeGetProxy.
		return arrayLikeGetProxy(vmInstance, thisVal, i)
	default:
		key := strconv.Itoa(i)
		v, err := vmInstance.GetProperty(thisVal, key)
		if err != nil {
			return vm.Undefined, false, err
		}
		return v, true, nil
	}
}

// arrayLikeGetProxy invokes a Proxy's "has" trap when one is defined,
// before falling through to Get - fixing the specific failure mode where a
// "has" trap that throws (or returns false) must be seen by a
// DeletePropertyOrThrow-based caller like copyWithin's
// `fromPresent := HasProperty(...)` step, which arrayLikeGet's default
// branch below can't do at all (it only ever calls Get).
//
// This deliberately does NOT attempt full HasProperty semantics when no
// "has" trap is defined (i.e. it does not fall back to consulting the
// target's own properties/prototype chain for existence) - callers that
// discard the returned `exists` bool (the majority - see arrayLikeGet's
// callers) need Get invoked unconditionally regardless of "existence",
// exactly like Array.prototype.includes's own spec algorithm (Get only, no
// HasProperty at all: its Proxy test fixture defines only a "get" trap and
// expects it invoked for every index). Reporting exists=false without ever
// calling Get - which a target-consulting fallback would do for an
// absent-on-target index - broke that. So: no has trap means "assume
// present, just Get", same as this function's non-Proxy siblings; only an
// explicit has-trap result changes that answer.
func arrayLikeGetProxy(vmInstance *vm.VM, proxyVal vm.Value, i int) (vm.Value, bool, error) {
	proxy := proxyVal.AsProxy()
	if proxy.Revoked {
		return vm.Undefined, false, vmInstance.NewTypeError("Cannot perform 'has' on a proxy that has been revoked")
	}
	key := strconv.Itoa(i)
	if hasTrap, ok := proxy.Handler().AsPlainObject().GetOwn("has"); ok && hasTrap.Type() != vm.TypeUndefined && hasTrap.Type() != vm.TypeNull {
		if !hasTrap.IsCallable() {
			return vm.Undefined, false, vmInstance.NewTypeError("'has' on proxy: trap is not a function")
		}
		result, err := vmInstance.Call(hasTrap, proxy.Handler(), []vm.Value{proxy.Target(), vm.NewString(key)})
		if err != nil {
			return vm.Undefined, false, err
		}
		if !result.IsTruthy() {
			return vm.Undefined, false, nil
		}
	}
	v, err := vmInstance.GetProperty(proxyVal, key)
	if err != nil {
		return vm.Undefined, false, err
	}
	return v, true, nil
}

// arrayLikeSet writes index i of an array-like `this` to val - used by the
// mutating generic methods (push, splice, copyWithin, fill, reverse, sort,
// ...). Real Arrays invoke an own accessor setter if present (mirroring the
// PlainObject case just below) before falling back to a plain element
// write; PlainObjects invoke an own accessor setter if present before
// falling back to a plain data-property write; anything else goes through
// the VM's generic property set (correct for TypedArray element coercion,
// Proxy set traps, setters, etc.).
func arrayLikeSet(vmInstance *vm.VM, thisVal vm.Value, i int, val vm.Value) error {
	switch thisVal.Type() {
	case vm.TypeArray:
		arr := thisVal.AsArray()
		if arr.HasAccessors() {
			if _, s, _, _, ok := arr.GetOwnAccessor(strconv.Itoa(i)); ok {
				if s.Type() == vm.TypeUndefined {
					return nil // accessor with no setter: silently ignored (sloppy-mode semantics)
				}
				_, err := vmInstance.Call(s, thisVal, []vm.Value{val})
				return err
			}
		}
		arr.Set(i, val)
		return nil
	case vm.TypeObject:
		po := thisVal.AsPlainObject()
		key := strconv.Itoa(i)
		if _, s, _, _, ok := po.GetOwnAccessor(key); ok && s.Type() != vm.TypeUndefined {
			_, err := vmInstance.Call(s, thisVal, []vm.Value{val})
			return err
		}
		po.SetOwn(key, val)
		return nil
	default:
		return vmInstance.SetProperty(thisVal, strconv.Itoa(i), val)
	}
}

// arrayLikeSetLength writes a new "length" onto an array-like `this` -
// needed by push/splice/etc. when the receiver isn't a real Array (whose
// length is derived, not stored).
func arrayLikeSetLength(vmInstance *vm.VM, thisVal vm.Value, length int) error {
	switch thisVal.Type() {
	case vm.TypeArray:
		// A real Array's length is spec-capped at 2^32-1 (ArraySetLength);
		// an arbitrary object's "length" property has no such limit, so
		// this check only applies to the TypeArray branch.
		if err := checkArrayCreateLength(vmInstance, length); err != nil {
			return err
		}
		thisVal.AsArray().SetLength(length)
		return nil
	case vm.TypeObject:
		thisVal.AsPlainObject().SetOwn("length", vm.NumberValue(float64(length)))
		return nil
	default:
		return vmInstance.SetProperty(thisVal, "length", vm.NumberValue(float64(length)))
	}
}

// asPlainObjectOrNil is vm.Value.AsPlainObject() without the panic: nil for
// anything that isn't exactly TypeObject, so it's safe to use in the
// `if po := asPlainObjectOrNil(v); po != nil` idiom several call sites in
// this file rely on (unlike AsPlainObject() itself, which panics on a type
// mismatch before that nil check ever runs - see issue #46).
func asPlainObjectOrNil(v vm.Value) *vm.PlainObject {
	if v.Type() != vm.TypeObject {
		return nil
	}
	return v.AsPlainObject()
}

// isArraySpec implements the IsArray abstract operation (ECMA-262 7.2.2)
// backing Array.isArray: true for a real Array, recursing through a Proxy's
// target (throwing if the proxy has been revoked), false for everything
// else - except this engine's Array.prototype itself, which is represented
// as a PlainObject rather than a real ArrayObject (see InitRuntime above),
// so it needs an explicit identity check to satisfy "Array.prototype is an
// Array exotic object" per spec.
func isArraySpec(vmInstance *vm.VM, v vm.Value) (bool, error) {
	switch v.Type() {
	case vm.TypeArray:
		return true, nil
	case vm.TypeProxy:
		proxy := v.AsProxy()
		if proxy.Revoked {
			return false, vmInstance.NewTypeError("Cannot perform 'IsArray' on a proxy that has been revoked")
		}
		return isArraySpec(vmInstance, proxy.Target())
	case vm.TypeObject:
		if vmInstance.ArrayPrototype.Type() == vm.TypeObject && v.AsPlainObject() == vmInstance.ArrayPrototype.AsPlainObject() {
			return true, nil
		}
		return false, nil
	default:
		return false, nil
	}
}

// isConcatSpreadable implements IsConcatSpreadable (ECMA-262 23.1.3.1.1):
// non-objects are never spreadable; an object's own/inherited
// @@isConcatSpreadable overrides everything if defined; otherwise an object
// spreads iff it's a real Array. Used by Array.prototype.concat to decide
// whether an argument (or the receiver itself) contributes its elements or
// is appended as a single value.
func isConcatSpreadable(vmInstance *vm.VM, v vm.Value) (bool, error) {
	if !v.IsObject() && !v.IsCallable() {
		return false, nil
	}
	spreadable, found, err := vmInstance.GetSymbolPropertyWithGetter(v, vmInstance.SymbolIsConcatSpreadable)
	if err != nil {
		return false, err
	}
	if found && !spreadable.IsUndefined() {
		return spreadable.IsTruthy(), nil
	}
	// Fallback is IsArray(v), not a bare Type() check - a Proxy wrapping a
	// real Array must still be recognized as spreadable (isArraySpec
	// recurses through the proxy's target, per spec).
	return isArraySpec(vmInstance, v)
}

// arrayLikeDelete implements DeletePropertyOrThrow(O, ToString(i))
// (ECMA-262 7.3.11) for index i of an array-like `this` - used by
// copyWithin/splice/pop/shift-style methods where spec semantics call for
// a real delete (so a subsequent HasProperty/`in` check on that index is
// false) that throws a TypeError if it fails, rather than the silent
// always-succeeds write-undefined this replaced.
func arrayLikeDelete(vmInstance *vm.VM, thisVal vm.Value, i int) error {
	key := strconv.Itoa(i)
	switch thisVal.Type() {
	case vm.TypeArray:
		if !thisVal.AsArray().DeleteIndex(i) {
			return vmInstance.NewTypeError("Cannot delete property '" + key + "' of array")
		}
		return nil
	case vm.TypeObject:
		if !thisVal.AsPlainObject().DeleteOwn(key) {
			return vmInstance.NewTypeError("Cannot delete property '" + key + "' of object")
		}
		return nil
	case vm.TypeProxy:
		return proxyDeleteProperty(vmInstance, thisVal, i, key)
	default:
		return vmInstance.SetProperty(thisVal, key, vm.Undefined)
	}
}

// proxyDeleteProperty implements the Proxy exotic object's [[Delete]]
// (ECMA-262 10.5.10) for one property key: invoke the deleteProperty trap
// if present, checking the "can't report deleting a non-configurable
// target property" invariant on a truthy result; otherwise delegate to the
// target (recursively, so a Proxy wrapping a Proxy wrapping an Array all
// resolves through the same DeleteIndex/DeleteOwn paths above).
func proxyDeleteProperty(vmInstance *vm.VM, proxyVal vm.Value, i int, key string) error {
	proxy := proxyVal.AsProxy()
	if proxy.Revoked {
		return vmInstance.NewTypeError("Cannot delete property from a revoked Proxy")
	}
	deleteTrap, ok := proxy.Handler().AsPlainObject().GetOwn("deleteProperty")
	if !ok || deleteTrap.Type() == vm.TypeUndefined || deleteTrap.Type() == vm.TypeNull {
		return arrayLikeDelete(vmInstance, proxy.Target(), i)
	}
	if !deleteTrap.IsCallable() {
		return vmInstance.NewTypeError("'deleteProperty' on proxy: trap is not a function")
	}
	result, err := vmInstance.Call(deleteTrap, proxy.Handler(), []vm.Value{proxy.Target(), vm.NewString(key)})
	if err != nil {
		return err
	}
	if !result.IsTruthy() {
		return vmInstance.NewTypeError("'deleteProperty' on proxy: trap returned falsish for property '" + key + "'")
	}
	if target := proxy.Target(); target.Type() == vm.TypeObject {
		if _, _, _, configurable, found := target.AsPlainObject().GetOwnDescriptor(key); found && !configurable {
			return vmInstance.NewTypeError("'deleteProperty' on proxy: trap returned truish for property '" + key + "' which is non-configurable in the proxy target")
		}
	}
	return nil
}

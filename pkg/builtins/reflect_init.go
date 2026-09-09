package builtins

import (
	"strconv"
	"strings"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// isConstructor checks if a value can be used as a constructor (called with 'new')
// Per ECMAScript: only regular functions and classes are constructors.
// Arrow functions, generators, async functions, and async generators are NOT constructors.
func isConstructor(v vm.Value) bool {
	switch v.Type() {
	case vm.TypeFunction:
		fn := vm.AsFunction(v)
		// Arrow functions, generators, and async functions are not constructors
		if fn.IsArrowFunction || fn.IsGenerator || fn.IsAsync {
			return false
		}
		return true
	case vm.TypeClosure:
		cl := vm.AsClosure(v)
		if cl.Fn.IsArrowFunction || cl.Fn.IsGenerator || cl.Fn.IsAsync {
			return false
		}
		return true
	case vm.TypeNativeFunction:
		nf := vm.AsNativeFunction(v)
		return nf.IsConstructor
	case vm.TypeNativeFunctionWithProps:
		nfp := v.AsNativeFunctionWithProps()
		return nfp.IsConstructor
	case vm.TypeBoundFunction:
		// Bound functions inherit constructor status from original
		bf := v.AsBoundFunction()
		return isConstructor(bf.OriginalFunction)
	default:
		return false
	}
}

// reflectGetOwnAccessorGeneric returns the getter/setter pair for v's own
// accessor property named propKey, covering PlainObject's and
// ArrayObject's own accessor support plus the shared side-table
// (*PlainObject) the nine ownPropertiesSlot kinds (Function/Closure/
// NativeFunction/NativeFunctionWithProps/BoundFunction/RegExp/Map/Set/
// Promise) keep under OwnPropertiesTable, via the `default` branch.
// DictObject doesn't support accessors at all (pre-existing, matches
// every other DictObject case in this file). Any OTHER kind (e.g.
// TypeArguments, TypeTypedArray) that OwnPropertiesTable doesn't cover
// falls through the same `default` branch to isAccessor=false, not found
// - reflectOrdinarySet's walk treats that as "no own property here" and
// continues up the chain, so an exotic kind sitting as a link in a
// prototype chain (rare, and not exercised by this fix's own tests) is
// silently skipped rather than mishandled. Callers still need to
// separately check for a data property when isAccessor is false.
func reflectGetOwnAccessorGeneric(v vm.Value, propKey string) (getter vm.Value, setter vm.Value, isAccessor bool) {
	switch v.Type() {
	case vm.TypeObject:
		g, s, _, _, ok := v.AsPlainObject().GetOwnAccessor(propKey)
		return g, s, ok
	case vm.TypeArray:
		g, s, _, _, ok := v.AsArray().GetOwnAccessor(propKey)
		return g, s, ok
	case vm.TypeDictObject:
		return vm.Undefined, vm.Undefined, false
	default:
		if props := vm.OwnPropertiesTable(v); props != nil {
			g, s, _, _, ok := props.GetOwnAccessor(propKey)
			return g, s, ok
		}
		return vm.Undefined, vm.Undefined, false
	}
}

// reflectGetOwnDataDescriptorGeneric returns the value and writability of
// v's own DATA property named propKey (call reflectGetOwnAccessorGeneric
// first to rule out an accessor - this function doesn't check for one).
// Mirrors reflectGetOwnAccessorGeneric's kind coverage, plus TypeArray's
// own index/"length"/named-property shapes (an array has no side-table
// analog for these - they're modeled directly on ArrayObject).
func reflectGetOwnDataDescriptorGeneric(v vm.Value, propKey string) (value vm.Value, writable bool, found bool) {
	switch v.Type() {
	case vm.TypeObject:
		val, w, _, _, ok := v.AsPlainObject().GetOwnDescriptor(propKey)
		return val, w, ok
	case vm.TypeDictObject:
		val, w, _, _, ok := v.AsDictObject().GetOwnDescriptor(propKey)
		return val, w, ok
	case vm.TypeArray:
		arr := v.AsArray()
		if propKey == "length" {
			return vm.Number(float64(arr.Length())), true, true
		}
		if idx, err := strconv.Atoi(propKey); err == nil && idx >= 0 {
			if arr.HasOwnIndexProperty(propKey, idx) {
				return arr.Get(idx), true, true
			}
			return vm.Undefined, false, false
		}
		if val, ok := arr.GetOwn(propKey); ok {
			return val, true, true
		}
		return vm.Undefined, false, false
	default:
		if props := vm.OwnPropertiesTable(v); props != nil {
			val, w, _, _, ok := props.GetOwnDescriptor(propKey)
			return val, w, ok
		}
		return vm.Undefined, false, false
	}
}

// reflectGetOwnAccessorGenericByKey is reflectGetOwnAccessorGeneric for a
// symbol key (sym must be a TypeSymbol value) - the symbol-key counterpart
// backing Reflect.set's own symbol-key path (reflectSetDispatchByKey and
// below), which used to have none at all: Reflect.set(target, key, ...)
// stringified any key - Symbol included - via key.ToString() before it ever
// reached this file's dispatch, so a symbol-keyed accessor on target was
// never even looked for, let alone invoked.
func reflectGetOwnAccessorGenericByKey(v vm.Value, sym vm.Value) (getter vm.Value, setter vm.Value, isAccessor bool) {
	switch v.Type() {
	case vm.TypeObject:
		g, s, _, _, ok := v.AsPlainObject().GetOwnAccessorByKey(vm.NewSymbolKey(sym))
		return g, s, ok
	case vm.TypeArray:
		symObj := sym.AsSymbolObject()
		if symObj == nil {
			return vm.Undefined, vm.Undefined, false
		}
		g, s, _, _, ok := v.AsArray().GetOwnSymbolAccessor(symObj)
		return g, s, ok
	case vm.TypeDictObject:
		// DictObjects have no symbol-keyed storage at all - matches every
		// other DictObject-and-symbols case in this codebase (see
		// reflectDeleteProperty, pkg/builtins/reflect_delete.go, for the
		// identical rule applied to delete).
		return vm.Undefined, vm.Undefined, false
	default:
		if props := vm.OwnPropertiesTable(v); props != nil {
			g, s, _, _, ok := props.GetOwnAccessorByKey(vm.NewSymbolKey(sym))
			return g, s, ok
		}
		return vm.Undefined, vm.Undefined, false
	}
}

// reflectGetOwnDataDescriptorGenericByKey is reflectGetOwnDataDescriptorGeneric
// for a symbol key. Unlike the string-key version, TypeArray has no
// index/"length" special case here - a symbol can never equal a numeric
// index or the string "length", so an array's own symbol-keyed data
// property (plain `arr[sym] = v`, or one defined with explicit attributes
// via Object.defineProperty - see ArrayObject.GetSymbolPropertyDescriptor)
// is the only shape to check.
func reflectGetOwnDataDescriptorGenericByKey(v vm.Value, sym vm.Value) (value vm.Value, writable bool, found bool) {
	switch v.Type() {
	case vm.TypeObject:
		val, w, _, _, ok := v.AsPlainObject().GetOwnDescriptorByKey(vm.NewSymbolKey(sym))
		return val, w, ok
	case vm.TypeDictObject:
		return vm.Undefined, false, false
	case vm.TypeArray:
		symObj := sym.AsSymbolObject()
		if symObj == nil {
			return vm.Undefined, false, false
		}
		val, desc, ok := v.AsArray().GetSymbolPropertyDescriptor(symObj)
		return val, desc.Writable, ok
	default:
		if props := vm.OwnPropertiesTable(v); props != nil {
			val, w, _, _, ok := props.GetOwnDescriptorByKey(vm.NewSymbolKey(sym))
			return val, w, ok
		}
		return vm.Undefined, false, false
	}
}

// reflectOrdinarySet implements ECMA-262 10.1.9 OrdinarySet /
// 10.1.9.2 OrdinarySetWithOwnDescriptor for Reflect.set's non-Proxy-target
// path: walk `target`'s own property, then its whole [[Prototype]] chain,
// for the first applicable descriptor. An accessor's setter is invoked
// with `receiver` as `this` regardless of where in the chain it was found
// (accessors don't write data anywhere, so `target` vs `receiver` doesn't
// matter for this branch). A data descriptor - found on target itself, an
// ancestor, or nowhere at all (the implicit "value: undefined, writable:
// true" default per 10.1.9 step 4) - always has its actual write land on
// `receiver`, never on whichever object in the chain the descriptor was
// found on: this is the exact distinction the pre-existing "Simple
// property set on target" fallback got wrong, unconditionally writing to
// `target` regardless of `receiver`.
//
// Before this fix, `target`'s OWN accessor wasn't invoked at all either -
// the old fallback went straight to a raw SetOwn/Set call with no
// descriptor awareness whatsoever, for every case including
// receiver === target (verified against Node: Reflect.set on an object
// with its own setter silently no-opped the setter instead of calling
// it). Fixing the general-receiver case required walking descriptors
// anyway, so both bugs share one fix.
func reflectOrdinarySet(vmInstance *vm.VM, target vm.Value, propKey string, value vm.Value, receiver vm.Value) (bool, error) {
	current := target
	for i := 0; i < 200 && current.Type() != vm.TypeNull && current.Type() != vm.TypeUndefined; i++ {
		if _, setter, isAccessor := reflectGetOwnAccessorGeneric(current, propKey); isAccessor {
			// Per 10.1.9.2 step 3: an accessor descriptor with no setter
			// means the property is effectively read-only - Set fails
			// (verified against Node: returns false, doesn't throw here
			// since Reflect.set never throws for an ordinary failure).
			if setter.Type() == vm.TypeUndefined {
				return false, nil
			}
			_, err := vmInstance.Call(setter, receiver, []vm.Value{value})
			return err == nil, err
		}
		if _, writable, found := reflectGetOwnDataDescriptorGeneric(current, propKey); found {
			if !writable {
				return false, nil
			}
			// A writable data descriptor exists somewhere in target's own
			// chain - per 10.1.9.2 step 4, the actual write still targets
			// Receiver, not wherever this descriptor was found (which may
			// be `target` itself, or an ancestor `target` inherits from).
			return reflectCreateOrUpdateDataProperty(vmInstance, receiver, propKey, value)
		}
		current = vmInstance.PrototypeOf(current)
	}
	// Not found anywhere in target's chain - 10.1.9 step 4's implicit
	// {value: undefined, writable: true, enumerable: true, configurable:
	// true} default takes the same data-write path.
	return reflectCreateOrUpdateDataProperty(vmInstance, receiver, propKey, value)
}

// reflectOrdinarySetByKey is reflectOrdinarySet for a symbol key - same
// chain walk, same accessor-then-data priority, same "invoke the setter
// with receiver as `this` regardless of where in the chain it was found,
// but a data write always lands on receiver" rule (see reflectOrdinarySet's
// own doc comment for the full spec citation).
func reflectOrdinarySetByKey(vmInstance *vm.VM, target vm.Value, sym vm.Value, value vm.Value, receiver vm.Value) (bool, error) {
	current := target
	for i := 0; i < 200 && current.Type() != vm.TypeNull && current.Type() != vm.TypeUndefined; i++ {
		if _, setter, isAccessor := reflectGetOwnAccessorGenericByKey(current, sym); isAccessor {
			if setter.Type() == vm.TypeUndefined {
				return false, nil
			}
			_, err := vmInstance.Call(setter, receiver, []vm.Value{value})
			return err == nil, err
		}
		if _, writable, found := reflectGetOwnDataDescriptorGenericByKey(current, sym); found {
			if !writable {
				return false, nil
			}
			return reflectCreateOrUpdateDataPropertyByKey(vmInstance, receiver, sym, value)
		}
		current = vmInstance.PrototypeOf(current)
	}
	return reflectCreateOrUpdateDataPropertyByKey(vmInstance, receiver, sym, value)
}

// reflectCreateOrUpdateDataProperty implements the receiver-side half of
// 10.1.9.2 OrdinarySetWithOwnDescriptor once target's chain has determined
// a data write is called for: consult receiver's OWN descriptor for the
// same key (an accessor or non-writable data property there refuses the
// write - verified against Node for both), and otherwise create or
// overwrite receiver's own data property with the new value.
//
// A TypeProxy receiver is real, common code here, not a rare edge case:
// Reflect.set defaults `receiver` to `target`, so
// Reflect.set(someProxy, key, value) - the single most natural way to call
// Reflect.set on a Proxy at all - reaches this function with a Proxy
// receiver on its very first, simplest invocation. reflectGetOwnAccessorGeneric
// and reflectGetOwnDataDescriptorGeneric don't special-case TypeProxy (their
// `default` branch reports "not found", since OwnPropertiesTable doesn't
// cover Proxy), so those two pre-checks above the switch are a no-op for a
// Proxy receiver rather than a real 10.1.9.2 step 4.c
// Receiver.[[GetOwnProperty]](P) check - verified against Node that this
// doesn't change the outcome for the common "receiver has no existing
// descriptor" case, which is what reflectProxyDefineDataProperty's own
// getOwnPropertyDescriptor trap invocation (for side-effect parity, not
// result-branching - matching this file's pre-existing level of rigor for
// that trap) also confirms.
func reflectCreateOrUpdateDataProperty(vmInstance *vm.VM, receiver vm.Value, propKey string, value vm.Value) (bool, error) {
	// Per 10.1.9.2 step 4.a: if Receiver is not an object, return false -
	// verified against Node: Reflect.set({}, "y", 5, 42) is false, not a
	// throw.
	if !receiver.IsObject() && !receiver.IsCallable() {
		return false, nil
	}

	if _, _, isAccessor := reflectGetOwnAccessorGeneric(receiver, propKey); isAccessor {
		return false, nil
	}
	if _, writable, found := reflectGetOwnDataDescriptorGeneric(receiver, propKey); found && !writable {
		return false, nil
	}

	switch receiver.Type() {
	case vm.TypeObject:
		receiver.AsPlainObject().SetOwn(propKey, value)
		return true, nil
	case vm.TypeDictObject:
		receiver.AsDictObject().SetOwn(propKey, value)
		return true, nil
	case vm.TypeArray:
		arr := receiver.AsArray()
		if propKey == "length" {
			// The old "Simple property set on target" fallback this
			// function replaces had an explicit "Setting length is
			// complex, skip for now" no-op stub here - meaning
			// Reflect.set(arr, "length", n) already silently failed to
			// resize before this fix (verified against Node, which does
			// resize). Since this generic data-write path would otherwise
			// treat "length" as an ordinary named property and stash a
			// bogus one via arr.SetOwn - worse than the prior no-op,
			// since it'd shadow/corrupt the array's real length concept -
			// it needs its own case, not silence, now that this path
			// handles it at all.
			arr.SetLength(reflectToArrayLength(value))
			return true, nil
		}
		if idx, err := strconv.Atoi(propKey); err == nil && idx >= 0 {
			arr.Set(idx, value)
			return true, nil
		}
		arr.SetOwn(propKey, value)
		return true, nil
	case vm.TypeProxy:
		return reflectProxyDefineDataProperty(vmInstance, receiver, propKey, value)
	default:
		// A callable or other exotic receiver kind (Function/Closure/
		// NativeFunction/.../Promise) - use its shared side-table, the
		// same mechanism Object.defineProperty and friends already use
		// for these kinds elsewhere in this codebase.
		if props := vm.EnsureOwnPropertiesTable(receiver); props != nil {
			props.SetOwn(propKey, value)
			return true, nil
		}
		return false, nil
	}
}

// reflectCreateOrUpdateDataPropertyByKey is reflectCreateOrUpdateDataProperty
// for a symbol key. A symbol can never equal "length" or a numeric index
// string, so the TypeArray case here is simpler than the string-key
// version's: just the array's own symbol-keyed data slot (ArrayObject.
// SetSymbolProp - the same storage `arr[sym] = v` already writes into, so a
// brand-new key naturally comes out {writable: true, enumerable: true,
// configurable: true} the same way a plain assignment does, since nothing
// records an explicit descriptor for it unless Object.defineProperty
// later does).
//
// For TypeObject and the shared side-table (`default`) cases, unlike the
// string-key version's single receiver.AsPlainObject().SetOwn(propKey,
// value) call (whose PlainObject.SetOwn already implements "preserve
// existing writable/enumerable/configurable, default new key to
// true/true/true" internally for a string key), there is no equivalent
// SetOwnByKey - so this reproduces that same rule explicitly via
// HasOwnByKey + DefineOwnPropertyByKey, mirroring vm.setOwnCheckedByKey's
// identical pattern (pkg/vm/properties_table.go) for the exact same
// "ordinary [[Set]], not Object.defineProperty" distinction that
// function's own doc comment explains.
func reflectCreateOrUpdateDataPropertyByKey(vmInstance *vm.VM, receiver vm.Value, sym vm.Value, value vm.Value) (bool, error) {
	if !receiver.IsObject() && !receiver.IsCallable() {
		return false, nil
	}

	if _, _, isAccessor := reflectGetOwnAccessorGenericByKey(receiver, sym); isAccessor {
		return false, nil
	}
	if _, writable, found := reflectGetOwnDataDescriptorGenericByKey(receiver, sym); found && !writable {
		return false, nil
	}

	key := vm.NewSymbolKey(sym)
	switch receiver.Type() {
	case vm.TypeObject:
		plainObj := receiver.AsPlainObject()
		if plainObj.HasOwnByKey(key) {
			plainObj.DefineOwnPropertyByKey(key, value, nil, nil, nil)
		} else {
			w, e, c := true, true, true
			plainObj.DefineOwnPropertyByKey(key, value, &w, &e, &c)
		}
		return true, nil
	case vm.TypeDictObject:
		// DictObjects have no symbol-keyed storage at all - see
		// reflectGetOwnAccessorGenericByKey's identical case.
		return false, nil
	case vm.TypeArray:
		arr := receiver.AsArray()
		symObj := sym.AsSymbolObject()
		if symObj == nil {
			return false, nil
		}
		arr.SetSymbolProp(symObj, value)
		return true, nil
	case vm.TypeProxy:
		return reflectProxyDefineDataPropertyByKey(vmInstance, receiver, sym, value)
	default:
		// A callable or other exotic receiver kind (Function/Closure/
		// NativeFunction/.../Promise) - use its shared side-table, same as
		// the string-key version.
		if props := vm.EnsureOwnPropertiesTable(receiver); props != nil {
			if props.HasOwnByKey(key) {
				props.DefineOwnPropertyByKey(key, value, nil, nil, nil)
			} else {
				w, e, c := true, true, true
				props.DefineOwnPropertyByKey(key, value, &w, &e, &c)
			}
			return true, nil
		}
		return false, nil
	}
}

// reflectProxyDefineDataProperty implements the receiver-side write
// (10.1.9.2's CreateDataProperty(Receiver, ...) step, generalized to an
// "update or create" per reflectCreateOrUpdateDataProperty's own contract)
// when `receiver` turns out to be a Proxy - i.e. Receiver.[[DefineOwnProperty]]
// (10.5.6): the handler's `defineProperty` trap if present, else delegate
// straight to CreateDataProperty on the proxy's own target (which may
// itself be another Proxy, handled by the recursive call into
// reflectCreateOrUpdateDataProperty below).
//
// Also invokes the handler's `getOwnPropertyDescriptor` trap first, if
// present, purely for spec/side-effect parity with what this function
// supersedes (the old inline "receiver is a distinct Proxy" block this
// commit removes from the "set" closure) - verified against Node that its
// result doesn't change the outcome for the common case of a receiver with
// no pre-existing descriptor for this key, which is the only case this
// function (and its predecessor) actually handles; a receiver-Proxy with a
// genuinely conflicting existing descriptor is not modeled here, matching
// the prior code's own scope.
func reflectProxyDefineDataProperty(vmInstance *vm.VM, proxyVal vm.Value, propKey string, value vm.Value) (bool, error) {
	proxy := proxyVal.AsProxy()
	if proxy.Revoked {
		return false, vmInstance.NewTypeError("Cannot perform 'defineProperty' on a revoked Proxy")
	}
	handler := proxy.Handler()
	target := proxy.Target()

	// GetMethod(handler, "getOwnPropertyDescriptor"): side-effect-only call,
	// matching the block this supersedes - its result isn't consulted.
	var getOwnPropDescTrap vm.Value
	var hasGetOwnPropDesc bool
	switch handler.Type() {
	case vm.TypeObject:
		getOwnPropDescTrap, hasGetOwnPropDesc = handler.AsPlainObject().Get("getOwnPropertyDescriptor")
	case vm.TypeDictObject:
		getOwnPropDescTrap, hasGetOwnPropDesc = handler.AsDictObject().Get("getOwnPropertyDescriptor")
	}
	if hasGetOwnPropDesc && getOwnPropDescTrap.Type() != vm.TypeUndefined && getOwnPropDescTrap.Type() != vm.TypeNull {
		if !getOwnPropDescTrap.IsCallable() {
			return false, vmInstance.NewTypeError("'getOwnPropertyDescriptor' on proxy: trap is not a function")
		}
		if _, err := vmInstance.Call(getOwnPropDescTrap, handler, []vm.Value{target, vm.NewString(propKey)}); err != nil {
			return false, err
		}
	}

	// GetMethod(handler, "defineProperty"): an inherited trap counts,
	// undefined/null mean "no trap" - mirrors reflectHas's proxyReflectHas
	// (reflect_has.go), since proxyGetTrap (pkg/vm) is unexported and
	// unreachable from this package.
	var defineTrap vm.Value
	var hasDefineTrap bool
	switch handler.Type() {
	case vm.TypeObject:
		defineTrap, hasDefineTrap = handler.AsPlainObject().Get("defineProperty")
	case vm.TypeDictObject:
		defineTrap, hasDefineTrap = handler.AsDictObject().Get("defineProperty")
	}
	if !hasDefineTrap || defineTrap.Type() == vm.TypeUndefined || defineTrap.Type() == vm.TypeNull {
		// No trap: delegate straight to CreateDataProperty on the
		// underlying target - recurses through this same generic function
		// for whatever kind `target` turns out to be, including yet
		// another Proxy.
		return reflectCreateOrUpdateDataProperty(vmInstance, target, propKey, value)
	}
	if !defineTrap.IsCallable() {
		return false, vmInstance.NewTypeError("'defineProperty' on proxy: trap is not a function")
	}

	// CreateDataProperty's descriptor is always {value: V, writable: true,
	// enumerable: true, configurable: true} (7.3.5) - not whatever
	// descriptor `target`'s own property (if any) already had, since this
	// function models the "property doesn't exist on receiver yet" case.
	descObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	descObj.SetOwn("value", value)
	descObj.SetOwn("writable", vm.BooleanValue(true))
	descObj.SetOwn("enumerable", vm.BooleanValue(true))
	descObj.SetOwn("configurable", vm.BooleanValue(true))
	result, err := vmInstance.Call(defineTrap, handler, []vm.Value{target, vm.NewString(propKey), vm.NewValueFromPlainObject(descObj)})
	if err != nil {
		return false, err
	}
	return result.IsTruthy(), nil
}

// reflectProxyDefineDataPropertyByKey is reflectProxyDefineDataProperty for
// a symbol key - identical shape, but passes the real Symbol value to both
// traps (their "getOwnPropertyDescriptor"/"defineProperty" handler
// signatures take the actual property key per ECMA-262, not a stringified
// form) instead of vm.NewString(propKey).
func reflectProxyDefineDataPropertyByKey(vmInstance *vm.VM, proxyVal vm.Value, sym vm.Value, value vm.Value) (bool, error) {
	proxy := proxyVal.AsProxy()
	if proxy.Revoked {
		return false, vmInstance.NewTypeError("Cannot perform 'defineProperty' on a revoked Proxy")
	}
	handler := proxy.Handler()
	target := proxy.Target()

	var getOwnPropDescTrap vm.Value
	var hasGetOwnPropDesc bool
	switch handler.Type() {
	case vm.TypeObject:
		getOwnPropDescTrap, hasGetOwnPropDesc = handler.AsPlainObject().Get("getOwnPropertyDescriptor")
	case vm.TypeDictObject:
		getOwnPropDescTrap, hasGetOwnPropDesc = handler.AsDictObject().Get("getOwnPropertyDescriptor")
	}
	if hasGetOwnPropDesc && getOwnPropDescTrap.Type() != vm.TypeUndefined && getOwnPropDescTrap.Type() != vm.TypeNull {
		if !getOwnPropDescTrap.IsCallable() {
			return false, vmInstance.NewTypeError("'getOwnPropertyDescriptor' on proxy: trap is not a function")
		}
		if _, err := vmInstance.Call(getOwnPropDescTrap, handler, []vm.Value{target, sym}); err != nil {
			return false, err
		}
	}

	var defineTrap vm.Value
	var hasDefineTrap bool
	switch handler.Type() {
	case vm.TypeObject:
		defineTrap, hasDefineTrap = handler.AsPlainObject().Get("defineProperty")
	case vm.TypeDictObject:
		defineTrap, hasDefineTrap = handler.AsDictObject().Get("defineProperty")
	}
	if !hasDefineTrap || defineTrap.Type() == vm.TypeUndefined || defineTrap.Type() == vm.TypeNull {
		return reflectCreateOrUpdateDataPropertyByKey(vmInstance, target, sym, value)
	}
	if !defineTrap.IsCallable() {
		return false, vmInstance.NewTypeError("'defineProperty' on proxy: trap is not a function")
	}

	descObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	descObj.SetOwn("value", value)
	descObj.SetOwn("writable", vm.BooleanValue(true))
	descObj.SetOwn("enumerable", vm.BooleanValue(true))
	descObj.SetOwn("configurable", vm.BooleanValue(true))
	result, err := vmInstance.Call(defineTrap, handler, []vm.Value{target, sym, vm.NewValueFromPlainObject(descObj)})
	if err != nil {
		return false, err
	}
	return result.IsTruthy(), nil
}

// reflectProxySet implements Reflect.set's Proxy-TARGET path - ECMA-262
// 10.5.9 [[Set]] - which this function previously had no case for at all:
// `target.Type() == TypeProxy` passed the "set" closure's object gate
// (Value.IsObject() is a contiguous [TypeObject, TypeProxy] range check, so
// it's true for a Proxy), but neither the removed isDataProp computation
// nor the final target-kind switch had a case for TypeProxy, so it fell
// through to an unconditional `return false` for ANY Proxy target, with or
// without a `set` trap:
//
//	const target = {};
//	const p = new Proxy(target, {});
//	Reflect.set(p, "x", 5); // before: false, target.x stayed undefined - Node: true, target.x === 5
//
// Mirrors op_setprop.go's opSetProp TypeProxy handling (the bytecode
// `obj.x = v` path, which already gets this right for that narrower case),
// but forwards Reflect.set's own `receiver` argument to the trap - NOT
// necessarily the proxy itself, unlike ordinary assignment where the
// receiver is always the object the property access happened on. Also
// deliberately does NOT copy opSetProp's own no-trap fallback shape (it
// writes directly to `target` instead of `Receiver` in the "property
// absent everywhere" case - the same class of bug task_cd1507d7 fixed for
// Reflect.set's non-Proxy path) - the no-trap branch here instead recurses
// into reflectSetDispatch, reusing the already-correct
// reflectOrdinarySet/reflectCreateOrUpdateDataProperty pair.
func reflectProxySet(vmInstance *vm.VM, proxyVal vm.Value, propKey string, value vm.Value, receiver vm.Value) (bool, error) {
	proxy := proxyVal.AsProxy()
	if proxy.Revoked {
		return false, vmInstance.NewTypeError("Cannot perform 'set' on a revoked Proxy")
	}
	handler := proxy.Handler()
	target := proxy.Target()

	// GetMethod(handler, "set"): an inherited trap counts, undefined/null
	// mean "no trap" - mirrors reflectHas's proxyReflectHas (reflect_has.go).
	var trap vm.Value
	var hasTrap bool
	switch handler.Type() {
	case vm.TypeObject:
		trap, hasTrap = handler.AsPlainObject().Get("set")
	case vm.TypeDictObject:
		trap, hasTrap = handler.AsDictObject().Get("set")
	}
	if !hasTrap || trap.Type() == vm.TypeUndefined || trap.Type() == vm.TypeNull {
		// No trap: per spec, return target.[[Set]](P, V, Receiver) -
		// recurse into Reflect.set's own general dispatch for whatever
		// kind `target` (the proxy's own target, which may itself be
		// another Proxy) turns out to be, with the SAME receiver
		// Reflect.set was originally called with (not necessarily this
		// proxy).
		return reflectSetDispatch(vmInstance, target, propKey, value, receiver)
	}
	if !trap.IsCallable() {
		return false, vmInstance.NewTypeError("'set' on proxy: trap is not a function")
	}

	// Trap args per 10.5.9 step 8: (target, propertyKey, V, Receiver) -
	// propKey is passed as a plain string here (pre-existing, unrelated to
	// this fix: Reflect.set's own args[1].ToString() up in the "set"
	// closure already stringifies a Symbol key before it ever reaches this
	// function - so Reflect.set(proxy, Symbol.iterator, v) hands the trap
	// "Symbol(Symbol.iterator)" as a string rather than the real Symbol.
	// Not fixed here - see the PR body for why - but flagged, since this
	// is the first place that mangled key becomes externally observable to
	// user code (a trap function), not just internally wrong).
	result, err := vmInstance.Call(trap, handler, []vm.Value{target, vm.NewString(propKey), value, receiver})
	if err != nil {
		return false, err
	}
	if result.IsFalsey() {
		return false, nil
	}

	// ECMAScript 10.5.9 invariant validation (steps 13-15): a truish trap
	// result is rejected if target has a non-configurable property whose
	// invariant the trap tried to silently violate. Mirrors opSetProp's own
	// TypeObject-only version of this check (pkg/vm/op_setprop.go) - not
	// generalized to every VM kind here, matching that existing precedent
	// and this file's own established scope (a Proxy's real-world target is
	// overwhelmingly a plain object in practice).
	if target.Type() == vm.TypeObject {
		targetObj := target.AsPlainObject()
		if _, s, _, c, isAccessor := targetObj.GetOwnAccessor(propKey); isAccessor {
			if !c && s.Type() == vm.TypeUndefined {
				return false, vmInstance.NewTypeError("'set' on proxy: trap returned truish for property '" + propKey + "' which exists in the proxy target as a non-configurable accessor without a setter")
			}
		} else if v, w, _, c, found := targetObj.GetOwnDescriptor(propKey); found {
			if !c && !w {
				if !v.StrictlyEquals(value) {
					return false, vmInstance.NewTypeError("'set' on proxy: trap returned truish for property '" + propKey + "' which exists in the proxy target as a non-configurable and non-writable data property with a different value")
				}
			}
		}
	}

	return true, nil
}

// reflectProxySetByKey is reflectProxySet for a symbol key. Unlike
// reflectProxySet's own string-key path - which has always stringified a
// Symbol key into "Symbol(...)" before handing it to a `set` trap, a
// pre-existing bug noted but deliberately not fixed there (see that
// function's doc comment) - this path was built alongside a proper
// symbol-key Reflect.set from the start, so it passes the real Symbol
// value straight through to the trap, matching ECMA-262 10.5.9 step 8
// (the trap receives the actual property key, never a stringified form).
func reflectProxySetByKey(vmInstance *vm.VM, proxyVal vm.Value, sym vm.Value, value vm.Value, receiver vm.Value) (bool, error) {
	proxy := proxyVal.AsProxy()
	if proxy.Revoked {
		return false, vmInstance.NewTypeError("Cannot perform 'set' on a revoked Proxy")
	}
	handler := proxy.Handler()
	target := proxy.Target()

	var trap vm.Value
	var hasTrap bool
	switch handler.Type() {
	case vm.TypeObject:
		trap, hasTrap = handler.AsPlainObject().Get("set")
	case vm.TypeDictObject:
		trap, hasTrap = handler.AsDictObject().Get("set")
	}
	if !hasTrap || trap.Type() == vm.TypeUndefined || trap.Type() == vm.TypeNull {
		return reflectSetDispatchByKey(vmInstance, target, sym, value, receiver)
	}
	if !trap.IsCallable() {
		return false, vmInstance.NewTypeError("'set' on proxy: trap is not a function")
	}

	result, err := vmInstance.Call(trap, handler, []vm.Value{target, sym, value, receiver})
	if err != nil {
		return false, err
	}
	if result.IsFalsey() {
		return false, nil
	}

	if target.Type() == vm.TypeObject {
		targetObj := target.AsPlainObject()
		key := vm.NewSymbolKey(sym)
		if _, s, _, c, isAccessor := targetObj.GetOwnAccessorByKey(key); isAccessor {
			if !c && s.Type() == vm.TypeUndefined {
				return false, vmInstance.NewTypeError("'set' on proxy: trap returned truish for a Symbol property which exists in the proxy target as a non-configurable accessor without a setter")
			}
		} else if v, w, _, c, found := targetObj.GetOwnDescriptorByKey(key); found {
			if !c && !w {
				if !v.StrictlyEquals(value) {
					return false, vmInstance.NewTypeError("'set' on proxy: trap returned truish for a Symbol property which exists in the proxy target as a non-configurable and non-writable data property with a different value")
				}
			}
		}
	}

	return true, nil
}

// reflectSetDispatch is Reflect.set's core dispatch, shared by the "set"
// NativeFunction closure itself and every recursive delegation site above
// (a Proxy target with no trap, a Proxy receiver with no trap) that needs
// to re-enter the same logic for a different target/receiver pair.
func reflectSetDispatch(vmInstance *vm.VM, target vm.Value, propKey string, value vm.Value, receiver vm.Value) (bool, error) {
	if target.Type() == vm.TypeProxy {
		return reflectProxySet(vmInstance, target, propKey, value, receiver)
	}

	// Module Namespace Exotic Object [[Set]] behavior (ECMAScript 10.4.6.9)
	// [[Set]] on a namespace always returns false
	if target.Type() == vm.TypeObject {
		if po := target.AsPlainObject(); po.IsModuleNamespace() {
			return false, nil
		}
	}

	switch target.Type() {
	case vm.TypeObject, vm.TypeDictObject, vm.TypeArray:
		return reflectOrdinarySet(vmInstance, target, propKey, value, receiver)
	}

	// target is some other kind this function doesn't model a set for.
	// Matches this function's prior behavior for every kind it didn't have
	// a case for.
	return false, nil
}

// reflectSetDispatchByKey is reflectSetDispatch for a symbol key - the
// entry point Reflect.set's "set" closure and every recursive symbol-key
// delegation site above (a Proxy target or receiver with no trap) re-enter.
// This whole symbol-key chain (reflectSetDispatchByKey ->
// reflectProxySetByKey / reflectOrdinarySetByKey ->
// reflectCreateOrUpdateDataPropertyByKey / reflectProxyDefineDataPropertyByKey)
// used to not exist at all: Reflect.set(target, key, value) stringified
// ANY key via key.ToString() before it ever reached this file's dispatch,
// so a Symbol key silently became its string form ("Symbol(...)") on every
// target kind - plain object, array, Map/Set/RegExp/callable side-tables,
// and Proxy alike:
//
//	const o = {}; const s = Symbol("k");
//	Reflect.set(o, s, 5); // before: true, but o[s] stayed undefined - Node: true, o[s] === 5
func reflectSetDispatchByKey(vmInstance *vm.VM, target vm.Value, sym vm.Value, value vm.Value, receiver vm.Value) (bool, error) {
	if target.Type() == vm.TypeProxy {
		return reflectProxySetByKey(vmInstance, target, sym, value, receiver)
	}

	// Module Namespace Exotic Object [[Set]] behavior (ECMAScript 10.4.6.9)
	// [[Set]] on a namespace always returns false - matches the string-key
	// version. A module namespace's own exports are always string-named,
	// so this is here purely for symmetry with reflectSetDispatch rather
	// than because a namespace can hold a symbol-keyed export.
	if target.Type() == vm.TypeObject {
		if po := target.AsPlainObject(); po.IsModuleNamespace() {
			return false, nil
		}
	}

	switch target.Type() {
	case vm.TypeObject, vm.TypeDictObject, vm.TypeArray:
		return reflectOrdinarySetByKey(vmInstance, target, sym, value, receiver)
	}

	return false, nil
}

// reflectToArrayLength mirrors pkg/vm/vm_init.go's unexported
// toLengthIntForSetProperty (ToLength clamping for an array's "length"
// property) - duplicated rather than imported since that helper is
// unexported and this is the one place in package builtins that needs the
// exact same clamp.
func reflectToArrayLength(v vm.Value) int {
	n := v.ToFloat()
	if n != n || n <= 0 {
		return 0
	}
	const maxSafeInteger = 9007199254740991
	if n > maxSafeInteger {
		n = maxSafeInteger
	}
	return int(n)
}

type ReflectInitializer struct{}

func (r *ReflectInitializer) Name() string  { return "Reflect" }
func (r *ReflectInitializer) Priority() int { return 104 } // After Console (102), Symbol must be initialized first

func (r *ReflectInitializer) InitTypes(ctx *TypeContext) error {
	// Property key type: string | symbol
	keyType := types.NewUnionType(types.String, types.Symbol)

	// Create Reflect object type with all 13 methods
	reflectType := types.NewObjectType().
		// Property operations
		//
		// "get" takes an optional third `receiver` argument (used as the
		// `this` an accessor's getter is called with - the runtime
		// implementation now actually reads it, see reflectObj's "get"
		// closure below). "set"/"construct" have the same shape of gap for
		// their own optional trailing arguments - fixed alongside "get"
		// here rather than left as a separate follow-up, since the runtime
		// already accepted a 4th/3rd argument for both before this fix
		// (Reflect.set(t, k, v, receiver) already worked under
		// --no-typecheck; Reflect.construct(t, args, newTarget) did NOT -
		// see the "construct" closure below for the real, causally-coupled
		// runtime bug this surfaced and fixed in the same commit).
		//
		// "set"'s spec signature (ECMA-262, and lib.es2015.reflect.d.ts in
		// the pinned TypeScript v6.0.3 tree) is
		// `set(target, propertyKey, value, receiver?)` - only the trailing
		// `receiver` is optional, `value` is required (it legitimately
		// defaults to `undefined` at the VALUE level when omitted, same as
		// any other required `any`-typed parameter given `undefined` -
		// that's not the same as the parameter itself being optional).
		WithProperty("get", types.NewOptionalFunction([]types.Type{types.Any, keyType, types.Any}, types.Any, []bool{false, false, true})).
		WithProperty("set", types.NewOptionalFunction([]types.Type{types.Any, keyType, types.Any, types.Any}, types.Boolean, []bool{false, false, false, true})).
		WithProperty("has", types.NewSimpleFunction([]types.Type{types.Any, keyType}, types.Boolean)).
		WithProperty("deleteProperty", types.NewSimpleFunction([]types.Type{types.Any, keyType}, types.Boolean)).
		// Prototype operations
		WithProperty("getPrototypeOf", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("setPrototypeOf", types.NewSimpleFunction([]types.Type{types.Any, types.Any}, types.Boolean)).
		// Descriptor operations
		WithProperty("defineProperty", types.NewSimpleFunction([]types.Type{types.Any, keyType, types.Any}, types.Boolean)).
		WithProperty("getOwnPropertyDescriptor", types.NewSimpleFunction([]types.Type{types.Any, keyType}, types.Any)).
		// Key operations
		WithProperty("ownKeys", types.NewSimpleFunction([]types.Type{types.Any}, &types.ArrayType{ElementType: types.Any})).
		// Extensibility operations
		WithProperty("isExtensible", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("preventExtensions", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		// Function operations
		WithProperty("apply", types.NewSimpleFunction([]types.Type{types.Any, types.Any, types.Any}, types.Any)).
		WithProperty("construct", types.NewOptionalFunction([]types.Type{types.Any, types.Any, types.Any}, types.Any, []bool{false, false, true}))

	return ctx.DefineGlobal("Reflect", reflectType)
}

func (r *ReflectInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Reflect object with Object.prototype as its prototype (ECMAScript spec)
	reflectObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Set @@toStringTag to "Reflect" so Object.prototype.toString.call(Reflect) returns "[object Reflect]"
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		falseVal := false
		reflectObj.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Reflect"),
			&falseVal, // writable: false
			&falseVal, // enumerable: false
			&falseVal, // configurable: false (per ECMAScript spec 28.1)
		)
	}

	// Reflect.get(target, propertyKey [, receiver])
	// Per ECMAScript spec, this invokes [[Get]] and returns the result -
	// receiver defaults to target, and is what any accessor's getter along
	// the way is called with as `this` (10.1.8 [[Get]] step 5/6). The
	// actual per-kind dispatch (own-accessor-then-data, prototype-chain
	// walk, Proxy trap invocation and invariant checks, both string and
	// Symbol keys) lives in pkg/vm/vm_init.go's
	// GetPropertyWithReceiver/ReflectGetSymbolPropertyWithReceiver, which
	// this used to duplicate a much narrower, TypeObject/TypeDictObject/
	// TypeArray-only, string-key-only version of inline - silently
	// returning undefined for every other kind (Function, Map, Set,
	// RegExp, BoundFunction, NativeFunction, NativeFunctionWithProps,
	// Promise, TypedArray, Proxy, ...) and unconditionally stringifying
	// even a Symbol key.
	reflectObj.SetOwnNonEnumerable("get", vm.NewNativeFunction(2, false, "get", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.get requires at least 2 arguments")
		}
		target := args[0]
		key := args[1]

		if !target.IsObject() && !target.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.get called on non-object")
		}

		// Receiver defaults to target if not provided (ECMA-262 28.1.7).
		receiver := target
		if len(args) >= 3 {
			receiver = args[2]
		}

		if key.Type() == vm.TypeSymbol {
			return vmInstance.ReflectGetSymbolPropertyWithReceiver(target, key, receiver)
		}
		return vmInstance.GetPropertyWithReceiver(target, key.ToString(), receiver)
	}))

	// Reflect.set(target, propertyKey [, value [, receiver]])
	// Per ECMAScript spec, value defaults to undefined, receiver defaults to target
	reflectObj.SetOwnNonEnumerable("set", vm.NewNativeFunction(3, false, "set", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.BooleanValue(false), vmInstance.NewTypeError("Reflect.set requires at least 2 arguments")
		}
		target := args[0]
		key := args[1]

		// Value defaults to undefined if not provided
		value := vm.Undefined
		if len(args) >= 3 {
			value = args[2]
		}

		// Receiver defaults to target if not provided
		receiver := target
		if len(args) >= 4 {
			receiver = args[3]
		}

		if !target.IsObject() && !target.IsCallable() {
			return vm.BooleanValue(false), vmInstance.NewTypeError("Reflect.set called on non-object")
		}

		// The actual algorithm - ECMA-262 10.1.9/10.1.9.2 OrdinarySet(WithOwnDescriptor)
		// for a plain target, or 10.5.9 [[Set]] when target is a Proxy -
		// lives in reflectSetDispatch (string keys) / reflectSetDispatchByKey
		// (symbol keys), shared with the recursive delegation sites a Proxy
		// target or a Proxy receiver without the relevant trap need to
		// re-enter.
		//
		// This used to inline a "receiver is a distinct Proxy" special case
		// right here (checking isDataProp on `target` first) that only
		// handled a receiver-Proxy WITH a defineProperty trap present, and
		// otherwise silently fell through to a "Simple property set on
		// target" fallback that ignored `receiver` entirely - the bug
		// task_cd1507d7 fixed for every OTHER receiver kind, but this
		// Proxy-receiver special case sat upstream of that fix and kept
		// intercepting before it could run. It's superseded now, not
		// patched: reflectCreateOrUpdateDataProperty's own TypeProxy case
		// (reflectProxyDefineDataProperty) handles a Proxy receiver
		// completely - trap present or not - so this closure no longer
		// needs a separate inline special case for it, and `target` never
		// had a Proxy case here at all (task_18cd4923 - `target` itself
		// being a Proxy fell through to an unconditional `false` for ANY
		// Proxy target, trap or no trap).
		//
		// A Symbol key used to be unconditionally stringified via
		// key.ToString() before reaching any of this - Reflect.set(target,
		// someSymbol, v) silently coerced the symbol to a string key on
		// EVERY target kind (plain object, array, Map/Set/RegExp/callable
		// side-tables, Proxy), unlike Reflect.get and Reflect.has just
		// above/below this closure, which both already dispatched on
		// key.Type() == vm.TypeSymbol. See reflectSetDispatchByKey's doc
		// comment for the full symbol-key call chain this now threads
		// through, mirroring the one Reflect.get already had.
		if key.Type() == vm.TypeSymbol {
			ok, err := reflectSetDispatchByKey(vmInstance, target, key, value, receiver)
			return vm.BooleanValue(ok), err
		}
		ok, err := reflectSetDispatch(vmInstance, target, key.ToString(), value, receiver)
		return vm.BooleanValue(ok), err
	}))

	// Reflect.has(target, propertyKey)
	// Reflect.has(target, propertyKey)
	// Per ECMAScript spec, this invokes [[HasProperty]] and returns the
	// result. The dispatch across object kinds (including the Proxy 'has'
	// trap) lives in reflect_has.go.
	reflectObj.SetOwnNonEnumerable("has", vm.NewNativeFunction(2, false, "has", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.BooleanValue(false), vmInstance.NewTypeError("Reflect.has requires 2 arguments")
		}
		target := args[0]
		propKeyArg := args[1]

		// In ECMAScript, Reflect.has works with any object, including functions
		if !target.IsObject() && !target.IsCallable() {
			return vm.BooleanValue(false), vmInstance.NewTypeError("Reflect.has called on non-object")
		}

		hasProperty, err := reflectHas(vmInstance, target, propKeyArg)
		if err != nil {
			return vm.BooleanValue(false), err
		}
		return vm.BooleanValue(hasProperty), nil
	}))

	// Reflect.deleteProperty(target, propertyKey)
	// Per ECMAScript spec, this invokes [[Delete]] and returns the result.
	// The dispatch across object kinds lives in reflect_delete.go.
	reflectObj.SetOwnNonEnumerable("deleteProperty", vm.NewNativeFunction(2, false, "deleteProperty", func(args []vm.Value) (vm.Value, error) {
		return reflectDeletePropertyImpl(vmInstance, args)
	}))

	// Helper: check target is an object, throw TypeError if not (Reflect methods require objects)
	requireObject := func(methodName string, args []vm.Value) error {
		if len(args) == 0 {
			return vmInstance.NewTypeError("Reflect." + methodName + " requires a target")
		}
		target := args[0]
		if !target.IsObject() && !target.IsCallable() {
			return vmInstance.NewTypeError("Reflect." + methodName + " called on non-object")
		}
		return nil
	}

	// Reflect.getPrototypeOf(target) - must be an object (unlike Object.getPrototypeOf which coerces)
	reflectObj.SetOwnNonEnumerable("getPrototypeOf", vm.NewNativeFunction(1, false, "getPrototypeOf", func(args []vm.Value) (vm.Value, error) {
		if err := requireObject("getPrototypeOf", args); err != nil {
			return vm.Undefined, err
		}
		return objectGetPrototypeOfWithVM(vmInstance, args)
	}))

	// Reflect.setPrototypeOf(target, prototype)
	reflectObj.SetOwnNonEnumerable("setPrototypeOf", vm.NewNativeFunction(2, false, "setPrototypeOf", func(args []vm.Value) (vm.Value, error) {
		result, err := objectSetPrototypeOfWithVM(vmInstance, args)
		if err != nil {
			// Reflect.setPrototypeOf returns false (instead of throwing) for:
			// - Prototype cycle detection
			// - Non-extensible objects
			// - Immutable prototype exotic objects
			// - Proxy trap returning falsish
			// Only actual type errors (wrong arg types, revoked proxy) should throw
			if ee, ok := err.(vm.ExceptionError); ok {
				excVal := ee.GetExceptionValue()
				if msgVal, _ := vmInstance.GetProperty(excVal, "message"); msgVal.Type() == vm.TypeString {
					msg := msgVal.ToString()
					if strings.Contains(msg, "Cannot set prototype") ||
						strings.Contains(msg, "non-extensible") ||
						strings.Contains(msg, "trap returned falsish") {
						return vm.BooleanValue(false), nil
					}
				}
			}
			return vm.BooleanValue(false), err
		}
		// Object.setPrototypeOf returns the object on success
		return vm.BooleanValue(result.Type() != vm.TypeUndefined), nil
	}))

	// Reflect.defineProperty(target, propertyKey, attributes)
	// Per ECMAScript spec, this returns boolean instead of throwing for invalid operations
	// EXCEPT: Proxy invariant violations must still throw TypeError
	reflectObj.SetOwnNonEnumerable("defineProperty", vm.NewNativeFunction(3, false, "defineProperty", func(args []vm.Value) (vm.Value, error) {
		if err := requireObject("defineProperty", args); err != nil {
			return vm.BooleanValue(false), err
		}

		result, err := objectDefinePropertyWithVM(vmInstance, args)
		if err != nil {
			// Per spec: Proxy invariant violations must throw through Reflect.defineProperty.
			// "trap returned falsish" is NOT an invariant violation — it means the trap said no.
			// Only invariant violations (trap returned true but invariants violated) propagate.
			if args[0].Type() == vm.TypeProxy {
				// Extract the error message from the exception value
				isTrapFalsish := false
				if ee, ok := err.(vm.ExceptionError); ok {
					excVal := ee.GetExceptionValue()
					if msgVal, _ := vmInstance.GetProperty(excVal, "message"); msgVal.Type() == vm.TypeString {
						if strings.Contains(msgVal.ToString(), "trap returned falsish") {
							isTrapFalsish = true
						}
					}
				}
				if !isTrapFalsish {
					return vm.Undefined, err
				}
			}
			return vm.BooleanValue(false), nil
		}
		// defineProperty returns the object on success
		return vm.BooleanValue(result.Type() != vm.TypeUndefined), nil
	}))

	// Reflect.getOwnPropertyDescriptor(target, propertyKey)
	reflectObj.SetOwnNonEnumerable("getOwnPropertyDescriptor", vm.NewNativeFunction(2, false, "getOwnPropertyDescriptor", func(args []vm.Value) (vm.Value, error) {
		if err := requireObject("getOwnPropertyDescriptor", args); err != nil {
			return vm.Undefined, err
		}
		return objectGetOwnPropertyDescriptorWithVM(vmInstance, args)
	}))

	// Reflect.ownKeys(target) - returns array of all own property keys
	reflectObj.SetOwnNonEnumerable("ownKeys", vm.NewNativeFunction(1, false, "ownKeys", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.ownKeys requires 1 argument")
		}
		target := args[0]

		// Every sibling Reflect method in this file (get/set/has/apply/
		// construct/...) gates on "!IsObject() && !IsCallable()" - this one
		// used to gate on "!IsObject()" alone, so it threw a TypeError
		// outright for a plain function, a class, a native function, a
		// native constructor, or a bound function - values every other
		// Reflect method here already accepts. Confirmed against Node:
		// Reflect.ownKeys(Array.prototype.slice) is ["length","name"]
		// there, not a throw.
		if !target.IsObject() && !target.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.ownKeys called on non-object")
		}

		// Get all own keys (including non-enumerable, unlike Object.keys)
		keysArray := vm.NewArray()
		arr := keysArray.AsArray()

		switch target.Type() {
		case vm.TypeFunction, vm.TypeClosure, vm.TypeNativeFunction, vm.TypeNativeFunctionWithProps, vm.TypeBoundFunction:
			// Fixing the gate above just made these five kinds reach this
			// switch instead of throwing - but the switch itself had no
			// case for any of them, so they'd have fallen through to
			// "return keysArray, nil" and answered [] instead of actually
			// throwing OR actually working (silently wrong either way).
			//
			// Rather than hand-rolling a THIRD independent per-kind
			// name/length/prototype synthesis switch alongside
			// objectGetOwnPropertyNamesWithVM's (Object.getOwnPropertyNames)
			// and objectGetOwnPropertySymbolsWithVM's (Object.
			// getOwnPropertySymbols) own already-correct ones - the exact
			// "N independent copies of the same per-kind dispatch slowly
			// drift apart" pattern behind most of this session's bug
			// fixes - delegate straight to those two for these five kinds
			// only (TypeObject/TypeDictObject/TypeArray/TypeProxy below
			// keep their own existing, already-correct logic exactly as
			// it was; TypeNativeFunction/TypeBoundFunction were just added
			// to objectGetOwnPropertyNamesWithVM as part of this same fix,
			// since it had no case for either of them either - the same
			// gap, just one level down).
			//
			// Per ECMAScript 10.1.11 OrdinaryOwnPropertyKeys, Reflect.
			// ownKeys's required order - integer indices ascending, then
			// string keys in creation order, then symbol keys in creation
			// order - is exactly what concatenating these two functions'
			// own outputs already produces, so no reordering is needed
			// here.
			namesVal, err := objectGetOwnPropertyNamesWithVM(vmInstance, []vm.Value{target})
			if err != nil {
				return vm.Undefined, err
			}
			if namesVal.Type() == vm.TypeArray {
				namesArr := namesVal.AsArray()
				for i := 0; i < namesArr.Length(); i++ {
					arr.Append(namesArr.Get(i))
				}
			}
			symsVal, err := objectGetOwnPropertySymbolsWithVM(vmInstance, []vm.Value{target})
			if err != nil {
				return vm.Undefined, err
			}
			if symsVal.Type() == vm.TypeArray {
				symsArr := symsVal.AsArray()
				for i := 0; i < symsArr.Length(); i++ {
					arr.Append(symsArr.Get(i))
				}
			}
			return keysArray, nil
		case vm.TypeObject:
			obj := target.AsPlainObject()
			// 1. String keys (all, including non-enumerable)
			for _, key := range obj.OwnPropertyNames() {
				arr.Append(vm.NewString(key))
			}
			// 2. Symbol keys
			for _, sym := range obj.OwnSymbolKeys() {
				arr.Append(sym)
			}
		case vm.TypeDictObject:
			// DictObject only supports string keys for now
			for _, key := range target.AsDictObject().OwnPropertyNames() {
				arr.Append(vm.NewString(key))
			}
		case vm.TypeArray:
			// This used to hand-roll its own index/sparse-index/"length"
			// logic - byte-for-byte identical to
			// objectGetOwnPropertyNamesWithVM's own TypeArray case except
			// for one thing: it never appended NAMED (non-index) string
			// properties at all, so an array with an ad-hoc property like
			// `arr.foo = "bar"` was missing "foo" from Reflect.ownKeys even
			// though Object.getOwnPropertyNames(arr) correctly included it
			// (verified against Node, which lists it in both). It also had
			// no symbol-key coverage whatsoever - ArrayObject.OwnSymbolKeys
			// (pkg/vm/value.go) is a brand-new method this exact fix added,
			// since Object.getOwnPropertySymbols had no TypeArray case
			// either before this.
			//
			// Rather than hand-rolling BOTH gaps' worth of duplicate logic
			// a second time in a second independent switch - the "N
			// independent copies of the same per-kind dispatch slowly
			// drift apart" pattern behind most of this session's bug
			// fixes, and exactly how this array case ended up missing
			// named properties while its sibling function didn't - this
			// now delegates to objectGetOwnPropertyNamesWithVM +
			// objectGetOwnPropertySymbolsWithVM, mirroring the five
			// callable kinds' own delegation above (task_06547fb2). Their
			// index/sparse-index/"length" handling is already identical to
			// what this case had, so this is a pure superset: same output
			// for everything that already worked, plus the two gaps closed.
			namesVal, err := objectGetOwnPropertyNamesWithVM(vmInstance, []vm.Value{target})
			if err != nil {
				return vm.Undefined, err
			}
			if namesVal.Type() == vm.TypeArray {
				namesArr := namesVal.AsArray()
				for i := 0; i < namesArr.Length(); i++ {
					arr.Append(namesArr.Get(i))
				}
			}
			symsVal, err := objectGetOwnPropertySymbolsWithVM(vmInstance, []vm.Value{target})
			if err != nil {
				return vm.Undefined, err
			}
			if symsVal.Type() == vm.TypeArray {
				symsArr := symsVal.AsArray()
				for i := 0; i < symsArr.Length(); i++ {
					arr.Append(symsArr.Get(i))
				}
			}
		case vm.TypeProxy:
			// This used to claim (inaccurately) to "delegate to
			// Object.getOwnPropertyNames + getOwnPropertySymbols as a
			// simplification" - but the delegation target itself had no
			// TypeProxy case at all (objectGetOwnPropertyNamesWithVM,
			// object_init.go), so the "simplification" was a complete
			// no-op: Reflect.ownKeys on ANY Proxy, trap or no trap,
			// always answered [] before this fix. proxyOwnPropertyKeys
			// (object_init.go) is the real, shared ECMA-262 10.5.11
			// [[OwnPropertyKeys]] implementation - see its own comment for
			// what it does and doesn't validate - used here directly
			// rather than through Object.getOwnPropertyNames, since this
			// caller wants the FULL mixed string+symbol result, not one
			// filtered half of it.
			keys, err := proxyOwnPropertyKeys(vmInstance, target)
			if err != nil {
				return vm.Undefined, err
			}
			for _, k := range keys {
				arr.Append(k)
			}
		}

		return keysArray, nil
	}))

	// Reflect.isExtensible(target) - must be an object
	reflectObj.SetOwnNonEnumerable("isExtensible", vm.NewNativeFunction(1, false, "isExtensible", func(args []vm.Value) (vm.Value, error) {
		if err := requireObject("isExtensible", args); err != nil {
			return vm.Undefined, err
		}
		return objectIsExtensibleWithVM(vmInstance, args)
	}))

	// Reflect.preventExtensions(target) - must be an object
	reflectObj.SetOwnNonEnumerable("preventExtensions", vm.NewNativeFunction(1, false, "preventExtensions", func(args []vm.Value) (vm.Value, error) {
		if err := requireObject("preventExtensions", args); err != nil {
			return vm.Undefined, err
		}
		result, err := objectPreventExtensionsWithVM(vmInstance, args)
		if err != nil {
			// For proxies: "trap returned falsish" means the trap said no — return false.
			// Invariant violations (trap returned true but target still extensible) must throw.
			if len(args) > 0 && args[0].Type() == vm.TypeProxy {
				isTrapFalsish := false
				if ee, ok := err.(vm.ExceptionError); ok {
					excVal := ee.GetExceptionValue()
					if msgVal, _ := vmInstance.GetProperty(excVal, "message"); msgVal.Type() == vm.TypeString {
						if strings.Contains(msgVal.ToString(), "trap returned falsish") {
							isTrapFalsish = true
						}
					}
				}
				if isTrapFalsish {
					return vm.BooleanValue(false), nil
				}
			}
			return vm.BooleanValue(false), err
		}
		return vm.BooleanValue(result.Type() != vm.TypeUndefined), nil
	}))

	// Reflect.apply(target, thisArgument, argumentsList)
	reflectObj.SetOwnNonEnumerable("apply", vm.NewNativeFunction(3, false, "apply", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 3 {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.apply requires 3 arguments")
		}
		target := args[0]
		thisArg := args[1]
		argsList := args[2]

		if !target.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.apply: target is not a function")
		}

		// CreateListFromArrayLike(argumentsList)
		// Per ECMAScript spec:
		// 1. If Type(obj) is not Object, throw TypeError
		// 2. Get length from obj.length (may throw)
		// 3. Iterate indexed properties
		if !argsList.IsObject() && argsList.Type() != vm.TypeArray {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.apply: argumentsList is not an object")
		}

		var callArgs []vm.Value
		if argsList.Type() == vm.TypeArray {
			// Fast path for arrays
			arr := argsList.AsArray()
			callArgs = make([]vm.Value, arr.Length())
			for i := 0; i < arr.Length(); i++ {
				callArgs[i] = arr.Get(i)
			}
		} else {
			// Generic array-like object: access .length property (may throw)
			vmInstance.EnterHelperCall()
			lengthVal, err := vmInstance.GetProperty(argsList, "length")
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				// Exception thrown while accessing .length - propagate it
				return vm.Undefined, nil
			}
			if err != nil {
				return vm.Undefined, err
			}
			length := int(lengthVal.ToFloat())
			if length < 0 {
				length = 0
			}
			callArgs = make([]vm.Value, length)
			for i := 0; i < length; i++ {
				vmInstance.EnterHelperCall()
				val, err := vmInstance.GetProperty(argsList, strconv.Itoa(i))
				vmInstance.ExitHelperCall()
				if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
					return vm.Undefined, nil
				}
				if err != nil {
					callArgs[i] = vm.Undefined
				} else {
					callArgs[i] = val
				}
			}
		}

		// Call the function
		return vmInstance.Call(target, thisArg, callArgs)
	}))

	// Reflect.construct(target, argumentsList [, newTarget])
	reflectObj.SetOwnNonEnumerable("construct", vm.NewNativeFunction(2, false, "construct", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.construct requires at least 2 arguments")
		}
		target := args[0]
		argsList := args[1]

		// newTarget defaults to target
		newTarget := target
		if len(args) >= 3 {
			newTarget = args[2]
		}

		if !target.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.construct: target is not a constructor")
		}

		// Check if target is actually a constructor (not arrow function, async, etc.)
		if !isConstructor(target) {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.construct: target is not a constructor")
		}

		// Check if newTarget is a constructor (per ECMAScript spec)
		if !isConstructor(newTarget) {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.construct: newTarget is not a constructor")
		}

		// CreateListFromArrayLike(argumentsList)
		// Per ECMAScript spec:
		// 1. If Type(obj) is not Object, throw TypeError
		// 2. Get length from obj.length (may throw)
		// 3. Iterate indexed properties
		if !argsList.IsObject() && argsList.Type() != vm.TypeArray {
			return vm.Undefined, vmInstance.NewTypeError("Reflect.construct: argumentsList is not an object")
		}

		var constructArgs []vm.Value
		if argsList.Type() == vm.TypeArray {
			// Fast path for arrays
			arr := argsList.AsArray()
			constructArgs = make([]vm.Value, arr.Length())
			for i := 0; i < arr.Length(); i++ {
				constructArgs[i] = arr.Get(i)
			}
		} else {
			// Generic array-like object: access .length property (may throw)
			vmInstance.EnterHelperCall()
			lengthVal, err := vmInstance.GetProperty(argsList, "length")
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				// Exception thrown while accessing .length - propagate it
				return vm.Undefined, nil
			}
			if err != nil {
				return vm.Undefined, err
			}
			length := int(lengthVal.ToFloat())
			if length < 0 {
				length = 0
			}
			constructArgs = make([]vm.Value, length)
			for i := 0; i < length; i++ {
				vmInstance.EnterHelperCall()
				val, err := vmInstance.GetProperty(argsList, strconv.Itoa(i))
				vmInstance.ExitHelperCall()
				if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
					return vm.Undefined, nil
				}
				if err != nil {
					constructArgs[i] = vm.Undefined
				} else {
					constructArgs[i] = val
				}
			}
		}

		// Use ConstructWithNewTarget to properly invoke the constructor with custom new.target
		return vmInstance.ConstructWithNewTarget(target, constructArgs, newTarget)
	}))

	// Define Reflect globally
	return ctx.DefineGlobal("Reflect", vm.NewValueFromPlainObject(reflectObj))
}

package builtins

import (
	"github.com/nooga/paserati/pkg/vm"
)

// reflectHas is target.[[HasProperty]](key) as observed by Reflect.has
// (ECMA-262 28.1.7): unlike the `in` operator (which throws when its RHS
// isn't an object), the only TypeErrors here are the Proxy ones - a
// non-callable trap, or a trap whose false answer violates the "target
// has a non-configurable own property" / "target is non-extensible"
// invariant - plus whatever the trap itself throws. The caller
// (reflect_init.go) has already validated target.IsObject() ||
// target.IsCallable() before this runs.
//
// key is expected to already be a property key value (a Symbol, or
// anything else stringified via key.ToString()) - unlike
// reflectDeleteProperty's contract, the caller here does NOT run
// ToPropertyKey first (no ToPrimitive-with-hint-string / user-defined
// toString/Symbol.toPrimitive dance), so an object key whose own toString
// throws will not surface that abruptly the way the spec's algorithm
// requires. That gap predates this function and is not fixed here.
func reflectHas(vmInstance *vm.VM, target vm.Value, key vm.Value) (bool, error) {
	isSym := key.Type() == vm.TypeSymbol
	var name string
	if !isSym {
		name = key.ToString()
	}
	var propKey vm.PropertyKey
	if isSym {
		propKey = vm.NewSymbolKey(key)
	} else {
		propKey = vm.NewStringKey(name)
	}

	switch target.Type() {
	case vm.TypeProxy:
		return proxyReflectHas(vmInstance, target, key)

	case vm.TypeObject:
		obj := target.AsPlainObject()
		if isSym {
			// Own-only for a symbol key on a plain object - pre-existing,
			// not part of this fix (obj.Has's own prototype-aware walk
			// only takes a string key today).
			_, ok := obj.GetOwnByKey(propKey)
			return ok, nil
		}
		return obj.Has(name), nil

	case vm.TypeDictObject:
		return target.AsDictObject().Has(name), nil

	case vm.TypeArray:
		arr := target.AsArray()
		if isSym {
			if arr.HasOwnSymbolProp(key.AsSymbolObject()) {
				return true, nil
			}
			return vmInstance.HasPropertyOnPrototypeChain(target, propKey), nil
		}
		// A numerically-in-range index is NOT necessarily an own
		// property (paserati#176/#178 - see ArrayHasOwnIndex), and an
		// index that isn't an own property still needs the
		// prototype-chain walk (Array.prototype[5] = ...) rather than
		// reporting absent outright - `in`/Reflect.has implement
		// HasProperty, not HasOwnProperty.
		if idx, ok := vm.ParseArrayIndex(name); ok {
			if vm.ArrayHasOwnIndex(arr, idx) {
				return true, nil
			}
			return vmInstance.HasPropertyOnPrototypeChain(target, propKey), nil
		}
		if name == "length" {
			return true, nil
		}
		if vm.ArrayHasOwnNamedProperty(arr, name) {
			return true, nil
		}
		return vmInstance.HasPropertyOnPrototypeChain(target, propKey), nil

	case vm.TypeMap, vm.TypeSet:
		// Map/Set: own "size", then the lazily-created side table
		// (OwnPropertiesTable - the same one Object.freeze/seal and a
		// plain `map.foo = 1` assignment use), then Map.prototype/
		// Set.prototype and beyond.
		if !isSym && name == "size" {
			return true, nil
		}
		if props := vm.OwnPropertiesTable(target); props != nil {
			if isSym {
				if props.HasOwnByKey(propKey) {
					return true, nil
				}
			} else if props.HasOwn(name) {
				return true, nil
			}
		}
		return vmInstance.HasPropertyOnPrototypeChain(target, propKey), nil

	case vm.TypePromise:
		// Promises don't expose own state as ordinary properties, but a
		// plain property CAN still be assigned onto one (the same side
		// table Map/Set use) - check that, then Promise.prototype.
		if props := vm.OwnPropertiesTable(target); props != nil {
			if isSym {
				if props.HasOwnByKey(propKey) {
					return true, nil
				}
			} else if props.HasOwn(name) {
				return true, nil
			}
		}
		if vmInstance.PromisePrototype.IsObject() {
			return vmInstance.HasPropertyOnGivenPrototypeChain(vmInstance.PromisePrototype, propKey), nil
		}
		return false, nil

	case vm.TypeRegExp:
		// RegExp: "lastIndex" lives on the RegExpObject itself (writable,
		// non-configurable - see reflectDeleteProperty's TypeRegExp case),
		// not in the side table. Then the side table (OwnPropertiesTable -
		// user-assigned own properties), then this instance's own
		// [[Prototype]] chain - a subclass's RegExp.prototype
		// (RegExpObject.prototype, set "for subclassing" per its field
		// comment) when overridden, or vmInstance.RegExpPrototype itself
		// otherwise - for inherited methods (test/exec) and accessors
		// (source/flags/global/...), and (for a symbol key)
		// @@match/@@matchAll/@@replace/@@search/@@split (pkg/builtins/
		// regexp_init.go) - mirrors OpIn's TypeRegExp case (pkg/vm/vm.go).
		re := target.AsRegExpObject()
		if !isSym && name == "lastIndex" {
			return true, nil
		}
		if props := vm.OwnPropertiesTable(target); props != nil {
			if isSym {
				if props.HasOwnByKey(propKey) {
					return true, nil
				}
			} else if props.HasOwn(name) {
				return true, nil
			}
		}
		proto := vm.Undefined
		if re != nil {
			proto = re.GetPrototype()
		}
		if !proto.IsObject() {
			proto = vmInstance.RegExpPrototype
		}
		if !proto.IsObject() {
			return false, nil
		}
		if isSym {
			return vmInstance.HasPropertyOnGivenPrototypeChain(proto, propKey), nil
		}
		return proto.AsPlainObject().Has(name), nil

	case vm.TypeArguments:
		// Mirrors OpIn's own TypeArguments case (pkg/vm/vm.go) exactly -
		// including its non-symbol-only scope; a symbol key on an
		// Arguments object isn't handled by either `in` or here.
		argObj := target.AsArguments()
		if !isSym {
			if name == "length" {
				return true, nil
			}
			if name == "callee" && !argObj.IsStrict() {
				return true, nil
			}
			if idx, ok := vm.ParseArrayIndex(name); ok {
				return idx < argObj.Length(), nil
			}
		}
		if vmInstance.ObjectPrototype.IsObject() {
			if isSym {
				return vmInstance.HasPropertyOnGivenPrototypeChain(vmInstance.ObjectPrototype, propKey), nil
			}
			return vmInstance.ObjectPrototype.AsPlainObject().Has(name), nil
		}
		return false, nil

	case vm.TypeFunction, vm.TypeClosure, vm.TypeNativeFunction, vm.TypeNativeFunctionWithProps, vm.TypeBoundFunction:
		// Every callable kind used to answer false for anything beyond
		// its own "name"/"length"/"prototype" and side-table properties -
		// so `Reflect.has(fn, "call")` was false even though
		// `"call" in fn` (which does walk FunctionPrototype) was true.
		// callableTables (reflect_delete.go) is reused here for the
		// exact same "closure's own table, then the shared function's"
		// order OpDeleteProp/OpIn already use.
		// HasOwnFunctionIntrinsic correctly excludes "prototype" for an
		// arrow function or a native (non-constructor) function/method -
		// unlike a bare `name == "prototype"` check, which the original
		// pre-#316 Reflect.has used and which this reflect_has.go
		// initially carried forward unconditionally for EVERY callable
		// kind grouped in this case, including plain native methods that
		// never have one (`Reflect.has(Array.prototype.push, "prototype")`
		// would have incorrectly been true). It returns false for
		// TypeBoundFunction entirely (bound functions' name/length are
		// real entries in bf.Properties, not synthesized - the table
		// check right below already covers them).
		if !isSym && vm.HasOwnFunctionIntrinsic(target, name) {
			return true, nil
		}
		for _, t := range callableTables(target) {
			if t == nil {
				continue
			}
			if isSym {
				if t.HasOwnByKey(propKey) {
					return true, nil
				}
			} else if t.HasOwn(name) {
				return true, nil
			}
		}
		if isSym {
			// HasFunctionPrototypeSymbolProperty mirrors OpIn's own
			// TypeFunction/TypeClosure/TypeBoundFunction/TypeNativeFunction/
			// TypeNativeFunctionWithProps symbol-key cases (vm.go), which
			// walk FunctionPrototype's chain for an inherited symbol
			// property like Symbol.hasInstance. Before this, `in` and
			// Reflect.has already disagreed here in principle (in's walk
			// used to look for FunctionPrototype as a bare PlainObject and
			// silently find nothing, since it's actually a
			// TypeNativeFunctionWithProps at runtime - see that function's
			// doc comment), but with in's walk now fixed, leaving this
			// unconditionally false would make the disagreement real
			// instead of masked.
			return vmInstance.HasFunctionPrototypeSymbolProperty(propKey), nil
		}
		return vmInstance.HasFunctionPrototypeProperty(name), nil
	}

	// Not (yet) handled: WeakMap/WeakSet/WeakRef/FinalizationRegistry,
	// Generator/AsyncGenerator. Each answers false unconditionally, same
	// as before this fix - a real gap, not silently implied covered by a
	// blanket default (effectiveBuiltinPrototype, the obvious-looking
	// shortcut, only resolves a prototype for Array/Map/Set/WeakRef/
	// FinalizationRegistry - a default case built on it would look like
	// it covers these kinds and actually return false for all of them).
	return false, nil
}

// proxyReflectHas is the Proxy exotic object's [[HasProperty]] (10.5.7)
// with the result reported rather than thrown on: run the handler's
// `has` trap if it has one, enforce the non-configurable and
// non-extensible target invariants on a false answer, and otherwise
// defer to the target's own [[HasProperty]] (recursing through
// reflectHas, since the target can itself be a Proxy, or any other kind
// this function already knows how to answer for).
func proxyReflectHas(vmInstance *vm.VM, proxyVal vm.Value, key vm.Value) (bool, error) {
	proxy := proxyVal.AsProxy()
	if proxy.Revoked {
		return false, vmInstance.NewTypeError("Cannot perform 'has' on a proxy that has been revoked")
	}
	handler := proxy.Handler()
	target := proxy.Target()

	// GetMethod(handler, "has"): an inherited trap counts, and
	// undefined/null mean "no trap".
	var trap vm.Value
	var hasTrap bool
	switch handler.Type() {
	case vm.TypeObject:
		trap, hasTrap = handler.AsPlainObject().Get("has")
	case vm.TypeDictObject:
		trap, hasTrap = handler.AsDictObject().Get("has")
	}
	if !hasTrap || trap.Type() == vm.TypeUndefined || trap.Type() == vm.TypeNull {
		return reflectHas(vmInstance, target, key)
	}
	if !trap.IsCallable() {
		return false, vmInstance.NewTypeError("'has' on proxy: trap is not a function")
	}

	result, err := vmInstance.Call(trap, handler, []vm.Value{target, key})
	if err != nil {
		return false, err
	}
	if result.IsTruthy() {
		return true, nil
	}

	// Invariants (10.5.7 steps 11-12): a false answer is only allowed if
	// the target has no such own property, or has one that is
	// configurable on an extensible target.
	display := key.ToString()
	exists, nonConfig := targetOwnNonConfigurable(target, key)
	if !exists {
		return false, nil
	}
	if nonConfig {
		return false, vmInstance.NewTypeError("'has' on proxy: trap returned falsish for property '" + display + "' which exists in the proxy target as non-configurable")
	}
	if !isTargetExtensible(target) {
		return false, vmInstance.NewTypeError("'has' on proxy: trap returned falsish for property '" + display + "' but the proxy target is non-extensible")
	}
	return false, nil
}

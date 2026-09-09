package builtins

import (
	"github.com/nooga/paserati/pkg/vm"
)

// reflectDeleteProperty is target.[[Delete]](key) as observed by
// Reflect.deleteProperty (ECMA-262 28.1.4): it reports the [[Delete]]
// result as a boolean and never converts a refused delete into a TypeError
// the way the strict-mode `delete` operator does. The only TypeErrors it
// produces are the Proxy ones - a non-callable trap, or a trap that
// violates the "can't report deleting a non-configurable target property"
// invariant - plus whatever the trap itself throws.
//
// The VM's own delete opcodes (OpDeleteProp/OpDeleteIndex in vm.go) hold
// the equivalent logic inline, per object kind, interleaved with the
// strict-mode throw/reload-frame boilerplate; this is the same dispatch
// with the throwing removed. It has to cover every object kind, not just
// PlainObject: functions and classes are ordinary objects too, and real
// code deletes own properties off them (#297 - undici's
// `Reflect.deleteProperty(Response, 'getResponseHeaders')`).
//
// key must already be a property key (a Symbol or a String value) - the
// caller runs ToPropertyKey first so an abrupt completion from a
// user-defined toString surfaces before any deletion happens.
func reflectDeleteProperty(vmInstance *vm.VM, target vm.Value, key vm.Value) (bool, error) {
	isSym := key.Type() == vm.TypeSymbol
	var name string
	if !isSym {
		name = key.ToString()
	}

	switch target.Type() {
	case vm.TypeProxy:
		return proxyReflectDelete(vmInstance, target, key)

	case vm.TypeObject:
		po := target.AsPlainObject()
		// Module Namespace Exotic Object [[Delete]] (ECMA-262 10.4.6.8): a
		// string key that names an export can never be deleted; a symbol
		// key goes through OrdinaryDelete.
		if po.IsModuleNamespace() && !isSym {
			_, exists := po.GetOwn(name)
			return !exists, nil
		}
		if isSym {
			return deleteFromTableByKey(po, vm.NewSymbolKey(key)), nil
		}
		// The global object keeps var/function declarations in the heap
		// with their own DontDelete bit; mirror OpDeleteProp's handling so
		// Reflect.deleteProperty(globalThis, 'x') agrees with `delete
		// globalThis.x`.
		heap := vmInstance.GetHeap()
		if po == vmInstance.GlobalObject && heap != nil {
			if idx, exists := heap.GetNameToIndex()[name]; exists && !heap.IsConfigurable(idx) {
				return false, nil
			}
		}
		success := deleteFromTable(po, name)
		if success && po == vmInstance.GlobalObject && heap != nil {
			if idx, exists := heap.GetNameToIndex()[name]; exists {
				heap.Delete(idx)
			}
		}
		return success, nil

	case vm.TypeDictObject:
		if isSym {
			// DictObjects have no symbol-keyed storage, so the property
			// cannot exist - OrdinaryDelete of an absent property is true.
			return true, nil
		}
		d := target.AsDictObject()
		if exists, nonConfig := d.IsOwnPropertyNonConfigurable(name); exists && nonConfig {
			return false, nil
		}
		return d.DeleteOwn(name), nil

	case vm.TypeArray:
		arr := target.AsArray()
		if isSym {
			// A symbol property can now be explicitly non-configurable via
			// Object.defineProperty (see ArrayDefineOwnSymbolProperty,
			// pkg/vm/array_props.go) - propagate DeleteSymbolProp's actual
			// result instead of hardcoding success, matching every other
			// case in this function.
			return arr.DeleteSymbolProp(key.AsSymbolObject()), nil
		}
		if idx, isIndex := vm.ParseArrayIndex(name); isIndex {
			return arr.DeleteIndex(idx), nil
		}
		if name == "length" {
			return false, nil
		}
		return arr.DeleteOwn(name), nil

	case vm.TypeArguments:
		args := target.AsArguments()
		if isSym {
			args.DeleteSymbolProp(key.AsSymbolObject())
			return true, nil
		}
		return args.Delete(name), nil

	case vm.TypeFunction, vm.TypeClosure, vm.TypeNativeFunction, vm.TypeNativeFunctionWithProps, vm.TypeBoundFunction:
		return deleteFromCallable(target, key, isSym, name), nil

	case vm.TypeRegExp:
		// lastIndex lives on the RegExpObject itself, not in its side
		// table, and is {writable: true, configurable: false}.
		if !isSym && name == "lastIndex" {
			return false, nil
		}
	}

	// Every remaining exotic kind (RegExp, Map, Set, Promise, ...) keeps
	// its ordinary own properties in a lazily-created side table; a kind
	// with no table at all has no deletable own properties, and deleting an
	// absent property succeeds.
	props := vm.OwnPropertiesTable(target)
	if props == nil {
		return true, nil
	}
	if isSym {
		return deleteFromTableByKey(props, vm.NewSymbolKey(key)), nil
	}
	return deleteFromTable(props, name), nil
}

// deleteFromCallable is OrdinaryDelete for the five callable kinds. Their
// intrinsic "name"/"length" are synthesized lazily and tracked as deleted
// via a flag rather than removed from a table; "prototype" on a function
// that has one is {configurable: false} per MakeConstructor / ClassDefinition
// and so can never be deleted. Everything else is an ordinary side-table
// property (a closure checks its own table before the shared function's,
// matching OpDeleteProp).
func deleteFromCallable(target vm.Value, key vm.Value, isSym bool, name string) bool {
	tables := callableTables(target)

	if !isSym {
		switch name {
		case "name", "length":
			// A table entry (materialized by Object.freeze/seal, or defined
			// by user code) carries the authoritative attributes.
			for _, t := range tables {
				if t == nil || !t.HasOwn(name) {
					continue
				}
				if exists, nonConfig := t.IsOwnPropertyNonConfigurable(name); exists && nonConfig {
					return false
				}
				t.DeleteOwn(name)
			}
			markIntrinsicDeleted(target, name)
			return true
		case "prototype":
			if callableHasPrototype(target) {
				return false
			}
		}
	}

	for _, t := range tables {
		if t == nil {
			continue
		}
		if isSym {
			k := vm.NewSymbolKey(key)
			if !t.HasOwnByKey(k) {
				continue
			}
			return deleteFromTableByKey(t, k)
		}
		if !t.HasOwn(name) {
			continue
		}
		return deleteFromTable(t, name)
	}
	// Absent everywhere: OrdinaryDelete returns true.
	return true
}

// callableTables lists the side tables an own-property lookup on a callable
// consults, most specific first.
func callableTables(target vm.Value) []*vm.PlainObject {
	switch target.Type() {
	case vm.TypeClosure:
		cl := target.AsClosure()
		return []*vm.PlainObject{cl.Properties, cl.Fn.Properties}
	default:
		return []*vm.PlainObject{vm.OwnPropertiesTable(target)}
	}
}

// callableHasPrototype reports whether the callable has an own "prototype"
// property in this runtime: every user function that is not an arrow or a
// plain async function (see MaterializeIntrinsicOwnProperties for the same
// rule), or a native/bound function that had one defined in its table.
func callableHasPrototype(target vm.Value) bool {
	var fn *vm.FunctionObject
	switch target.Type() {
	case vm.TypeFunction:
		fn = target.AsFunction()
	case vm.TypeClosure:
		fn = target.AsClosure().Fn
	default:
		if t := vm.OwnPropertiesTable(target); t != nil && t.HasOwn("prototype") {
			if exists, nonConfig := t.IsOwnPropertyNonConfigurable("prototype"); exists && nonConfig {
				return true
			}
		}
		return false
	}
	return !fn.IsArrowFunction && !(fn.IsAsync && !fn.IsGenerator)
}

// markIntrinsicDeleted records the deletion of a lazily-synthesized "name" or
// "length" on the kinds that synthesize them (bound functions keep theirs in
// the table, which deleteFromCallable has already handled).
func markIntrinsicDeleted(target vm.Value, name string) {
	switch target.Type() {
	case vm.TypeFunction:
		setDeletedFlag(target.AsFunction(), name)
	case vm.TypeClosure:
		setDeletedFlag(target.AsClosure().Fn, name)
	case vm.TypeNativeFunction:
		nf := target.AsNativeFunction()
		if name == "name" {
			nf.DeletedName = true
		} else {
			nf.DeletedLength = true
		}
	case vm.TypeNativeFunctionWithProps:
		nfp := target.AsNativeFunctionWithProps()
		if name == "name" {
			nfp.DeletedName = true
		} else {
			nfp.DeletedLength = true
		}
	}
}

func setDeletedFlag(fn *vm.FunctionObject, name string) {
	if name == "name" {
		fn.DeletedName = true
	} else {
		fn.DeletedLength = true
	}
}

// deleteFromTable is OrdinaryDelete (10.1.7) on a PlainObject-backed
// property bag for a string key.
func deleteFromTable(po *vm.PlainObject, name string) bool {
	if exists, nonConfig := po.IsOwnPropertyNonConfigurable(name); exists && nonConfig {
		return false
	}
	return po.DeleteOwn(name)
}

// deleteFromTableByKey is deleteFromTable for a symbol (or other non-string)
// key.
func deleteFromTableByKey(po *vm.PlainObject, key vm.PropertyKey) bool {
	if exists, nonConfig := po.IsOwnPropertyNonConfigurableByKey(key); exists && nonConfig {
		return false
	}
	return po.DeleteOwnByKey(key)
}

// proxyReflectDelete is the Proxy exotic object's [[Delete]] (10.5.10) with
// the result reported rather than thrown on: run the handler's
// deleteProperty trap if it has one, enforce the non-configurable and
// non-extensible target invariants on a truthy answer, and otherwise defer
// to the target's own [[Delete]].
func proxyReflectDelete(vmInstance *vm.VM, proxyVal vm.Value, key vm.Value) (bool, error) {
	proxy := proxyVal.AsProxy()
	if proxy.Revoked {
		return false, vmInstance.NewTypeError("Cannot perform 'deleteProperty' on a proxy that has been revoked")
	}
	handler := proxy.Handler()
	target := proxy.Target()

	// GetMethod(handler, "deleteProperty"): an inherited trap counts, and
	// undefined/null mean "no trap".
	var trap vm.Value
	var hasTrap bool
	switch handler.Type() {
	case vm.TypeObject:
		trap, hasTrap = handler.AsPlainObject().Get("deleteProperty")
	case vm.TypeDictObject:
		trap, hasTrap = handler.AsDictObject().Get("deleteProperty")
	}
	if !hasTrap || trap.Type() == vm.TypeUndefined || trap.Type() == vm.TypeNull {
		return reflectDeleteProperty(vmInstance, target, key)
	}
	if !trap.IsCallable() {
		return false, vmInstance.NewTypeError("'deleteProperty' on proxy: trap is not a function")
	}

	result, err := vmInstance.Call(trap, handler, []vm.Value{target, key})
	if err != nil {
		return false, err
	}
	if !result.IsTruthy() {
		return false, nil
	}

	// Invariants (10.5.10 steps 10-13): a truthy answer is only allowed if
	// the target has no such own property, or has one that is configurable
	// on an extensible target.
	display := key.ToString()
	exists, nonConfig := targetOwnNonConfigurable(target, key)
	if !exists {
		return true, nil
	}
	if nonConfig {
		return false, vmInstance.NewTypeError("'deleteProperty' on proxy: trap returned truish for property '" + display + "' which is non-configurable in the proxy target")
	}
	if !isTargetExtensible(target) {
		return false, vmInstance.NewTypeError("'deleteProperty' on proxy: trap returned truish for property '" + display + "' but the proxy target is non-extensible")
	}
	return true, nil
}

// targetOwnNonConfigurable is a best-effort target.[[GetOwnProperty]](key)
// reduced to (exists, configurable == false) for the Proxy invariant check,
// covering the kinds whose descriptors this runtime can report.
func targetOwnNonConfigurable(target vm.Value, key vm.Value) (exists bool, nonConfig bool) {
	if key.Type() == vm.TypeSymbol {
		k := vm.NewSymbolKey(key)
		switch target.Type() {
		case vm.TypeObject:
			return target.AsPlainObject().IsOwnPropertyNonConfigurableByKey(k)
		case vm.TypeArray:
			return target.AsArray().HasOwnSymbolProp(key.AsSymbolObject()), false
		default:
			if t := vm.OwnPropertiesTable(target); t != nil {
				return t.IsOwnPropertyNonConfigurableByKey(k)
			}
		}
		return false, false
	}
	name := key.ToString()
	switch target.Type() {
	case vm.TypeObject:
		return target.AsPlainObject().IsOwnPropertyNonConfigurable(name)
	case vm.TypeDictObject:
		return target.AsDictObject().IsOwnPropertyNonConfigurable(name)
	case vm.TypeArray:
		arr := target.AsArray()
		if name == "length" {
			return true, true
		}
		if idx, isIndex := vm.ParseArrayIndex(name); isIndex {
			if idx < arr.Length() {
				return true, arr.IsFrozen()
			}
			return false, false
		}
		_, has := arr.GetOwn(name)
		return has, false
	case vm.TypeFunction, vm.TypeClosure, vm.TypeNativeFunction, vm.TypeNativeFunctionWithProps, vm.TypeBoundFunction:
		if name == "prototype" && callableHasPrototype(target) {
			return true, true
		}
		for _, t := range callableTables(target) {
			if t != nil && t.HasOwn(name) {
				return t.IsOwnPropertyNonConfigurable(name)
			}
		}
		return name == "name" || name == "length", false
	default:
		if t := vm.OwnPropertiesTable(target); t != nil {
			return t.IsOwnPropertyNonConfigurable(name)
		}
	}
	return false, false
}

// reflectDeletePropertyImpl is the Reflect.deleteProperty native: validate
// the target, run ToPropertyKey, then report [[Delete]].
func reflectDeletePropertyImpl(vmInstance *vm.VM, args []vm.Value) (vm.Value, error) {
	target, keyArg := vm.Undefined, vm.Undefined
	if len(args) > 0 {
		target = args[0]
	}
	if len(args) > 1 {
		keyArg = args[1]
	}
	if !target.IsObject() && !target.IsCallable() {
		return vm.Undefined, vmInstance.NewTypeError("Reflect.deleteProperty called on non-object")
	}
	key, err := toPropertyKeyValue(vmInstance, keyArg)
	if err != nil {
		return vm.Undefined, err
	}
	if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
		// ToPropertyKey threw (e.g. a user toString); let it propagate.
		return vm.Undefined, nil
	}
	ok, err := reflectDeleteProperty(vmInstance, target, key)
	if err != nil {
		return vm.Undefined, err
	}
	return vm.BooleanValue(ok), nil
}

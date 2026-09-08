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
			return reflectCreateOrUpdateDataProperty(receiver, propKey, value)
		}
		current = vmInstance.PrototypeOf(current)
	}
	// Not found anywhere in target's chain - 10.1.9 step 4's implicit
	// {value: undefined, writable: true, enumerable: true, configurable:
	// true} default takes the same data-write path.
	return reflectCreateOrUpdateDataProperty(receiver, propKey, value)
}

// reflectCreateOrUpdateDataProperty implements the receiver-side half of
// 10.1.9.2 OrdinarySetWithOwnDescriptor once target's chain has determined
// a data write is called for: consult receiver's OWN descriptor for the
// same key (an accessor or non-writable data property there refuses the
// write - verified against Node for both), and otherwise create or
// overwrite receiver's own data property with the new value.
func reflectCreateOrUpdateDataProperty(receiver vm.Value, propKey string, value vm.Value) (bool, error) {
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
		propKey := args[1].ToString()

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

		// Module Namespace Exotic Object [[Set]] behavior (ECMAScript 10.4.6.9)
		// [[Set]] on a namespace always returns false
		if target.Type() == vm.TypeObject {
			if po := target.AsPlainObject(); po.IsModuleNamespace() {
				return vm.BooleanValue(false), nil
			}
		}

		// For Proxy targets, we need to use the set trap differently
		// The set trap was already called by the caller (opSetProp), so here we
		// are implementing the actual Set algorithm that Reflect.set uses internally
		// when called from a Proxy set trap

		// Check if the property is a data property on the target
		isDataProp := false
		switch target.Type() {
		case vm.TypeObject:
			obj := target.AsPlainObject()
			if _, _, _, _, isAccessor := obj.GetOwnAccessor(propKey); !isAccessor {
				isDataProp = true
			}
		case vm.TypeDictObject:
			isDataProp = true // DictObject doesn't support accessors
		case vm.TypeArray:
			isDataProp = true
		}

		// If receiver is different from target (e.g., receiver is a Proxy),
		// we need to call receiver's [[GetOwnProperty]] and [[DefineOwnProperty]]
		// per ECMAScript 10.1.9.2 OrdinarySetWithOwnDescriptor
		if isDataProp && receiver.Type() == vm.TypeProxy && receiver != target {
			proxy := receiver.AsProxy()
			if proxy.Revoked {
				return vm.BooleanValue(false), vmInstance.NewTypeError("Cannot perform 'set' on a revoked Proxy")
			}

			handler := proxy.Handler()
			proxyTarget := proxy.Target()

			// Step 2.c: Let existingDescriptor be ? Receiver.[[GetOwnProperty]](P).
			// This triggers the getOwnPropertyDescriptor trap on the receiver Proxy
			getOwnPropDescTrap, hasGetOwnPropDesc := handler.AsPlainObject().GetOwn("getOwnPropertyDescriptor")
			if hasGetOwnPropDesc && getOwnPropDescTrap.IsCallable() {
				trapArgs := []vm.Value{proxyTarget, vm.NewString(propKey)}
				_, err := vmInstance.Call(getOwnPropDescTrap, handler, trapArgs)
				if err != nil {
					return vm.BooleanValue(false), err
				}
			}

			// Step 2.d.iv: Return ? Receiver.[[DefineOwnProperty]](P, valueDesc).
			// This triggers the defineProperty trap on the receiver Proxy
			definePropertyTrap, hasDefineProperty := handler.AsPlainObject().GetOwn("defineProperty")
			if hasDefineProperty && definePropertyTrap.IsCallable() {
				// Create a property descriptor with just the value
				valueDesc := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
				valueDesc.SetOwn("value", value)
				trapArgs := []vm.Value{proxyTarget, vm.NewString(propKey), vm.NewValueFromPlainObject(valueDesc)}
				result, err := vmInstance.Call(definePropertyTrap, handler, trapArgs)
				if err != nil {
					return vm.BooleanValue(false), err
				}
				return vm.BooleanValue(result.IsTruthy()), nil
			}
		}

		// Property set on target, implementing the real ECMA-262 10.1.9
		// OrdinarySet / 10.1.9.2 OrdinarySetWithOwnDescriptor algorithm -
		// see reflectOrdinarySet's own comment for the full rationale.
		// This used to be a "just SetOwn/Set directly on target" fallback
		// that ignored `receiver` entirely (writing to target even when a
		// distinct receiver was given - the exact bug this fix closes)
		// and never checked for an own or inherited accessor at all (so
		// even the receiver === target case silently clobbered an
		// existing setter instead of calling it - found while fixing the
		// receiver bug, since walking descriptors is required for either
		// fix).
		switch target.Type() {
		case vm.TypeObject, vm.TypeDictObject, vm.TypeArray:
			ok, err := reflectOrdinarySet(vmInstance, target, propKey, value, receiver)
			return vm.BooleanValue(ok), err
		}

		// target is some other kind this function doesn't model a set for
		// (e.g. a Proxy - Reflect.set(someProxy, ...) has its own,
		// separate, larger pre-existing gap: unrelated to the receiver
		// bug this fix closes, not touched here). Matches this function's
		// prior behavior for every kind it didn't have a case for.
		return vm.BooleanValue(false), nil
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
			arrayObj := target.AsArray()
			// Add numeric indices - skipping holes (paserati#300): a hole
			// from `delete arr[i]`, a literal elision, or `new Array(n)` is
			// not an own property at all.
			for i := 0; i < arrayObj.DenseLength(); i++ {
				key := strconv.Itoa(i)
				if !arrayObj.HasOwnIndexProperty(key, i) {
					continue
				}
				arr.Append(vm.NewString(key))
			}
			// A sparse index beyond the dense range (paserati#176/#178 -
			// see arraySparseIndices in object_init.go) is an integer-
			// indexed own key too, so per OrdinaryOwnPropertyKeys it
			// belongs here, in ascending numeric order, before "length" -
			// not visited by iterating up to it, which is exactly the
			// multi-billion-iteration hang this fixes. Reflect.ownKeys
			// wants every own key regardless of enumerability, hence
			// enumerableOnly=false.
			for _, idx := range arraySparseIndices(arrayObj, false) {
				arr.Append(vm.NewString(strconv.Itoa(idx)))
			}
			// Add "length"
			arr.Append(vm.NewString("length"))
		case vm.TypeProxy:
			// For proxies, this should invoke the ownKeys trap
			// For now, delegate to Object.getOwnPropertyNames + getOwnPropertySymbols
			// This is a simplification
			if objCtor, ok := vmInstance.GetGlobal("Object"); ok {
				if objCtor.Type() == vm.TypeNativeFunctionWithProps {
					nfp := objCtor.AsNativeFunctionWithProps()
					if f, ok := nfp.Properties.GetOwn("getOwnPropertyNames"); ok {
						if names, err := vmInstance.Call(f, vm.Undefined, []vm.Value{target}); err == nil {
							if names.Type() == vm.TypeArray {
								namesArr := names.AsArray()
								for i := 0; i < namesArr.Length(); i++ {
									arr.Append(namesArr.Get(i))
								}
							}
						}
					}
				}
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

package vm

import (
	"fmt"
	"strconv"
	"unsafe"
)

// VMInitCallback is a function that initializes VM-specific functionality
// It receives the VM instance and can set up prototypes, global objects, etc.
// type VMInitCallback func(vm *VM) error

// // Global registry of initialization callbacks
// var (
// 	globalInitCallbacks []VMInitCallback
// )

// // RegisterGlobalInitCallback registers a callback that will be called
// // for every new VM instance during initialization
// func RegisterGlobalInitCallback(callback VMInitCallback) {
// 	globalInitCallbacks = append(globalInitCallbacks, callback)
// }

// // initializeVM runs all registered initialization callbacks
// func (vm *VM) initializeVM() error {
// 	// Run global callbacks first (if any)
// 	for _, callback := range globalInitCallbacks {
// 		if err := callback(vm); err != nil {
// 			return err
// 		}
// 	}

// 	// Run instance-specific callbacks
// 	for _, callback := range vm.initCallbacks {
// 		if err := callback(vm); err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }

// // AddStandardCallbacks adds a set of standard callbacks to this VM instance
// // This allows external packages to provide standard initialization without circular dependencies
// func (vm *VM) AddStandardCallbacks(callbacks []VMInitCallback) {
// 	vm.initCallbacks = append(vm.initCallbacks, callbacks...)
// }

// // InitializeWithCallbacks runs the initialization callbacks that were added to this VM
// // This is separate from the constructor to allow adding callbacks after VM creation
// func (vm *VM) InitializeWithCallbacks() error {
// 	return vm.initializeVM()
// }

// vmCaller implements the VMCaller interface for async native functions
type vmCaller struct {
	vm           *VM
	yieldCh      chan *BytecodeCall
	currentFrame *CallFrame
}

func (vc *vmCaller) CallBytecode(fn Value, thisValue Value, args []Value) Value {
	// Create a channel to receive the result
	resultCh := make(chan Value, 1)

	// Create the bytecode call request
	call := &BytecodeCall{
		Function:  fn,
		ThisValue: thisValue,
		Args:      args,
		ResultCh:  resultCh,
	}

	// Send the call request to the VM
	vc.yieldCh <- call

	// Wait for the result
	result := <-resultCh
	return result
}

// executeAsyncNativeFunction executes an async native function that can call bytecode
func (vm *VM) executeAsyncNativeFunction(asyncFn *AsyncNativeFunctionObject, args []Value, destReg byte, callerRegisters []Value) (Value, error) {
	// Create channels for communication
	yieldCh := make(chan *BytecodeCall, 1)
	completeCh := make(chan Value, 1)

	// Create the VM caller interface
	caller := &vmCaller{
		vm:      vm,
		yieldCh: yieldCh,
	}

	// Run the async native function in a goroutine
	go func() {
		result := asyncFn.AsyncFn(caller, args)
		completeCh <- result
	}()

	// Process bytecode calls and wait for completion
	for {
		select {
		case call := <-yieldCh:
			// Execute the bytecode call
			result, err := vm.executeUserFunctionReentrant(call.Function, call.ThisValue, call.Args)
			if err != nil {
				call.ResultCh <- Undefined
			} else {
				call.ResultCh <- result
			}

		case result := <-completeCh:
			// Async function completed
			if int(destReg) < len(callerRegisters) {
				callerRegisters[destReg] = result
			}
			return result, nil
		}
	}
}

// executeUserFunctionReentrant executes a user-defined function from within a builtin
// This creates a minimal execution context similar to how modern JS engines handle builtin->JS calls
func (vm *VM) executeUserFunctionReentrant(fn Value, thisValue Value, args []Value) (Value, error) {

	// Check if we have space for another frame
	if vm.frameCount >= len(vm.frames) {
		return Undefined, fmt.Errorf("stack overflow during re-entrant call")
	}

	// Use the existing prepareCall infrastructure
	// Create dummy caller registers and IP for the context
	dummyCallerRegisters := make([]Value, 1) // Just need space for result
	dummyCallerIP := 0
	dummyDestReg := byte(0)

	// Use prepareDirectCall so the created frame is marked as a direct-call boundary
	shouldSwitch, err := vm.prepareDirectCall(fn, thisValue, args, dummyDestReg, dummyCallerRegisters, dummyCallerIP)
	if err != nil {
		return Undefined, fmt.Errorf("failed to prepare re-entrant call: %v", err)
	}

	if !shouldSwitch {
		// Native function was executed directly, return the result
		return dummyCallerRegisters[dummyDestReg], nil
	}

	// We have a new frame for bytecode execution, run the interpreter
	// The new frame is set up, now run the VM until it returns
	// Since prepareCall set up the frame, we can just call run()
	status, _ := vm.run()

	if status == InterpretRuntimeError {
		// If VM is unwinding and has a currentException, surface it as an ExceptionError
		if vm.unwinding && vm.currentException != Null {
			return Undefined, exceptionError{exception: vm.currentException}
		}
		return Undefined, fmt.Errorf("runtime error during re-entrant execution")
	}

	// The function should have returned and placed its result in dummyCallerRegisters[0]
	return dummyCallerRegisters[dummyDestReg], nil
}

// Deprecated: ExecuteUserFunctionForBuiltin has been removed. Use vm.Call instead.

// RegisterInitCallback registers a callback for this specific VM instance
// func (vm *VM) RegisterInitCallback(callback VMInitCallback) {
// 	vm.initCallbacks = append(vm.initCallbacks, callback)
// }

// initializePrototypes sets up the built-in prototype objects
func (vm *VM) initializePrototypes() {
	// Create the root Object.prototype (with null prototype)
	vm.ObjectPrototype = NewObject(Null)
	// Object.prototype is an Immutable Prototype Exotic Object (ECMAScript 9.4.7)
	vm.ObjectPrototype.AsPlainObject().SetImmutablePrototype()

	// Function.prototype inherits from Object.prototype
	vm.FunctionPrototype = NewObject(vm.ObjectPrototype)

	// Array.prototype inherits from Object.prototype
	vm.ArrayPrototype = NewObject(vm.ObjectPrototype)

	// String.prototype inherits from Object.prototype
	vm.StringPrototype = NewObject(vm.ObjectPrototype)

	// Number.prototype inherits from Object.prototype
	vm.NumberPrototype = NewObject(vm.ObjectPrototype)

	// Boolean.prototype inherits from Object.prototype
	vm.BooleanPrototype = NewObject(vm.ObjectPrototype)

	// Error.prototype inherits from Object.prototype
	vm.ErrorPrototype = NewObject(vm.ObjectPrototype)

	// TypeError.prototype inherits from Error.prototype
	vm.TypeErrorPrototype = NewObject(vm.ErrorPrototype)

	// ReferenceError.prototype inherits from Error.prototype
	vm.ReferenceErrorPrototype = NewObject(vm.ErrorPrototype)

	// Symbol.prototype inherits from Object.prototype
	vm.SymbolPrototype = NewObject(vm.ObjectPrototype)
}

// Deprecated: CallFunctionFromBuiltin has been removed. Builtins should use vm.Call.

// CallFunctionDirectly executes a user-defined function directly without re-entrant execution
// This is specifically designed for Function.prototype.call to avoid infinite recursion
func (vm *VM) CallFunctionDirectly(fn Value, thisValue Value, args []Value) (Value, error) {
	// fmt.Printf("[DEBUG CallFunctionDirectly] Called with fn type=%d, args=%d\n", fn.Type(), len(args))
	// Only handle user-defined functions and closures
	if !fn.IsFunction() && !fn.IsClosure() {
		return Undefined, fmt.Errorf("CallFunctionDirectly: not a user-defined function")
	}

	// Check if we have space for another frame
	if vm.frameCount >= len(vm.frames) {
		return Undefined, fmt.Errorf("stack overflow during direct function call")
	}

	// Get function arity and adjust arguments accordingly
	var expectedArity int
	if fn.Type() == TypeFunction {
		fnObj := fn.AsFunction()
		expectedArity = fnObj.Arity
	} else if fn.IsClosure() {
		closureObj := fn.AsClosure()
		expectedArity = closureObj.Fn.Arity
	}

	// Truncate arguments to match expected arity (JavaScript allows extra arguments to be ignored)
	adjustedArgs := args
	if len(args) > expectedArity {
		adjustedArgs = args[:expectedArity]
	}

	// Create registers for the call result
	resultRegisters := make([]Value, 1)
	dummyCallerIP := 0
	destReg := byte(0)

	// Use prepareDirectCall to set up the function call with isDirectCall flag
	shouldSwitch, err := vm.prepareDirectCall(fn, thisValue, adjustedArgs, destReg, resultRegisters, dummyCallerIP)
	if err != nil {
		return Undefined, fmt.Errorf("failed to prepare direct call: %v", err)
	}

	if !shouldSwitch {
		// Native function was executed directly, return the result
		return resultRegisters[destReg], nil
	}

	// We have a new frame for bytecode execution with isDirectCall = true
	// Execute the VM run loop - it will return immediately when the frame returns
	if debugCalls {
		fmt.Printf("[DEBUG CallFunctionDirectly] About to execute bytecode, frameCount=%d\n", vm.frameCount)
	}
	initialFrameCount := vm.frameCount
	status, result := vm.run()
	currentFrameCount := vm.frameCount
	if debugCalls {
		fmt.Printf("[DEBUG CallFunctionDirectly] Bytecode execution finished, status=%d, result=%s, frameCount=%d->%d\n", status, result.Inspect(), initialFrameCount, currentFrameCount)
	}

	if status == InterpretRuntimeError {
		// If VM is unwinding and has a currentException, surface it as an ExceptionError
		if vm.unwinding && vm.currentException != Null {
			return Undefined, exceptionError{exception: vm.currentException}
		}
		return Undefined, fmt.Errorf("runtime error during direct function execution")
	}

	// Check if the frame count dropped to 0 - this indicates the entire script execution
	// was completed due to an exception being caught by an outer handler
	if currentFrameCount == 0 {
		if debugCalls {
			fmt.Printf("[DEBUG CallFunctionDirectly] Frame count dropped to 0 (from %d) - script execution completed\n", initialFrameCount)
		}
		// The script execution has completed. This means we're no longer in a callback context
		// but the entire program has terminated. Signal this to the caller.
		// IMPORTANT: Do not return the script's final result value here, as that can
		// corrupt native method return paths. Return undefined with the special error signal.
		return Undefined, fmt.Errorf("SCRIPT_COMPLETED_WITH_RESULT: %s", result.Inspect())
	}

	return result, nil
}

// IsUnwinding returns true if the VM is currently in an exception unwinding state
func (vm *VM) IsUnwinding() bool {
	return vm.unwinding
}

// ThrowExceptionValue throws a JavaScript exception with the given value.
// This is used by native functions to propagate exceptions from vm.Call.
func (vm *VM) ThrowExceptionValue(value Value) {
	vm.throwException(value)
}

// EnterHelperCall increments the helper call depth counter.
// This should be called before native functions call helpers like ToPrimitive
// that might throw exceptions which need to be caught by try/catch blocks.
func (vm *VM) EnterHelperCall() {
	vm.helperCallDepth++
}

// ExitHelperCall decrements the helper call depth counter.
// This should be called after native functions return from helpers like ToPrimitive.
func (vm *VM) ExitHelperCall() {
	vm.helperCallDepth--
}

// IsHandlerFound returns true if an exception handler was found during a helper call.
// After checking this, the caller should call ClearHandlerFound().
func (vm *VM) IsHandlerFound() bool {
	return vm.handlerFound
}

// ClearHandlerFound clears the handler found flag.
func (vm *VM) ClearHandlerFound() {
	vm.handlerFound = false
}

// GetFrameCount returns the current frame count for debugging
func (vm *VM) GetFrameCount() int {
	return vm.frameCount
}

// GetProperty gets a property from an object value, properly handling getters and prototype chain
// This is safe to call from native functions and will trigger property getters/throw exceptions
func (vm *VM) GetProperty(obj Value, propName string) (Value, error) {
	return vm.getPropertyWithReceiver(obj, propName, obj)
}

// GetPropertyWithReceiver is GetProperty with an explicit receiver for any
// own or inherited accessor's getter to be called with as `this`, instead
// of always the object the property was actually found on/started from.
// Added for Reflect.get(target, key[, receiver]) (pkg/builtins/
// reflect_init.go), whose optional third argument is exactly this - per
// ECMA-262 10.1.8 [[Get]], an accessor's getter is invoked with Receiver
// as its this value, which can legitimately differ from the object
// [[Get]] started walking from (e.g. Reflect.get(proto, "x", instance)
// calls proto's getter for "x" with `this = instance`).
func (vm *VM) GetPropertyWithReceiver(obj Value, propName string, receiver Value) (Value, error) {
	return vm.getPropertyWithReceiver(obj, propName, receiver)
}

// getPropertyWithReceiver is GetProperty/GetPropertyWithReceiver's shared
// implementation. receiver is threaded through every accessor-getter
// vm.Call and every recursive self-call (walking a [[Prototype]] set via
// Object.setPrototypeOf, a Proxy's target when no trap is present, ...) so
// a getter anywhere on the chain is always invoked with the ORIGINAL
// receiver, not whichever intermediate object it was actually found on -
// GetProperty's plain callers (which don't have a distinct receiver
// concept) get receiver == obj, matching this function's behavior before
// receiver support existed.
func (vm *VM) getPropertyWithReceiver(obj Value, propName string, receiver Value) (Value, error) {
	// Simple implementation that doesn't use opGetProp to avoid unwinding issues
	// Check for getter (including prototype chain) and call it, or return the property value

	switch obj.Type() {
	case TypeObject:
		po := obj.AsPlainObject()
		// Check own accessor first
		if g, _, _, _, ok := po.GetOwnAccessor(propName); ok && g.Type() != TypeUndefined {
			// Call the getter with this=obj
			result, err := vm.Call(g, receiver, nil)
			if err != nil {
				return Undefined, err
			}
			return result, nil
		}
		// Check for own data property
		if value, exists := po.GetOwn(propName); exists {
			return value, nil
		}
		// Walk prototype chain for accessor or data properties
		current := po.GetPrototype()
		for current.typ != TypeNull && current.typ != TypeUndefined {
			if current.Type() == TypeObject {
				proto := current.AsPlainObject()
				// Check for accessor in prototype
				if g, _, _, _, ok := proto.GetOwnAccessor(propName); ok && g.Type() != TypeUndefined {
					// Call the getter with this=original obj (not proto)
					result, err := vm.Call(g, receiver, nil)
					if err != nil {
						return Undefined, err
					}
					return result, nil
				}
				// Check for data property in prototype
				if value, exists := proto.GetOwn(propName); exists {
					return value, nil
				}
				current = proto.GetPrototype()
				continue
			}
			// The chain leaves plain-TypeObject territory (a callable or
			// other exotic kind used as a [[Prototype]] via
			// Object.create(fn)/Object.setPrototypeOf, or a class
			// extending a native constructor) - recurse into the full
			// per-kind dispatch instead of silently stopping, so an own
			// property on THAT value's own table (a Function's
			// Properties, say) is still found. NOT gated on
			// current.IsObject(): every callable kind (TypeFunction,
			// TypeClosure, TypeNativeFunction, TypeNativeFunctionWithProps,
			// TypeBoundFunction) sorts BEFORE TypeObject in the ValueType
			// enum (pkg/vm/value.go), so IsObject() - a contiguous
			// [TypeObject, TypeProxy] range check - is FALSE for exactly
			// the values this branch exists to handle; gating on it here
			// (an earlier version of this fix did) silently reintroduced
			// the very "stops early" bug this comment describes fixing.
			// Found via the symbol-key sibling of this exact case
			// (getSymbolPropertyWithReceiver) needing the identical fix
			// for Reflect.get(Object.create(fn), sym) to work - this is
			// the same gap for a string key, fixed alongside it in the
			// same commit rather than left silently asymmetric between
			// key kinds.
			return vm.getPropertyWithReceiver(current, propName, receiver)
		}
		return Undefined, nil

	case TypeDictObject:
		// DictObject (a module namespace or TS enum value at runtime,
		// pkg/vm/object.go's NewDictObject) - own properties only.
		// DictObject doesn't support accessors (its own .Get comment says
		// so) and has no user-facing prototype chain of its own, so this
		// is just a direct own-table lookup. This case didn't exist at all
		// before this fix, so Reflect.get on a module namespace or enum
		// always answered undefined - found while implementing Reflect.get's
		// general property support, not previously reported.
		if v, ok := obj.AsDictObject().Get(propName); ok {
			return v, nil
		}
		return Undefined, nil

	case TypeGenerator:
		// Generator objects: consult Generator.prototype chain for regular properties
		proto := vm.GeneratorPrototype
		if proto.IsObject() {
			po := proto.AsPlainObject()
			if v, ok := po.GetOwn(propName); ok {
				return v, nil
			}
			// Walk the prototype chain
			current := po.prototype
			for current.typ != TypeNull && current.typ != TypeUndefined {
				if current.IsObject() {
					if current.Type() == TypeObject {
						proto2 := current.AsPlainObject()
						if v, ok := proto2.GetOwn(propName); ok {
							return v, nil
						}
						current = proto2.prototype
					} else {
						break
					}
				} else {
					break
				}
			}
		}
		return Undefined, nil

	case TypeArray:
		// Arrays: check own properties and prototype chain
		arr := obj.AsArray()
		if arr != nil {
			// Check for 'length' property
			if propName == "length" {
				return NumberValue(float64(arr.Length())), nil
			}
			// Check for numeric index access (e.g., "0", "1", "2")
			if idx, err := strconv.Atoi(propName); err == nil && idx >= 0 && idx < arr.Length() {
				if idx < len(arr.elements) && arr.elements[idx].typ != TypeHole {
					return arr.elements[idx], nil
				}
				// idx is within .length but not backed by a dense element -
				// see paserati#176: fall back to the named-property store
				// (where an index beyond maxDenseArraySetIndex/
				// maxDenseArrayDefineIndex lives) before Undefined.
				if v, ok := arr.GetOwn(propName); ok {
					return v, nil
				}
				return Undefined, nil
			}
			// Check own named properties on the array
			if v, ok := arr.GetOwn(propName); ok {
				return v, nil
			}
			// vm.plainPrototypeOf(obj) rather than the hardcoded
			// Array.prototype: a `class S extends Array {}` instance carries
			// its own [[Prototype]], and starting from the intrinsic made
			// every override on S.prototype unreachable from native code.
			if v, found, err := vm.lookupOnPrototypeChain(vm.plainPrototypeOf(obj), propName, receiver); found || err != nil {
				return v, err
			}
		}
		return Undefined, nil

	case TypeProxy:
		// For Proxy, call the 'get' trap
		proxy := obj.AsProxy()
		if proxy.Revoked {
			return Undefined, vm.NewTypeError("Cannot perform 'get' on a revoked Proxy")
		}
		// proxyGetTrap (not a bare proxy.handler.AsPlainObject().GetOwn("get"))
		// for the same two reasons documented on its own definition: GetMethod
		// semantics mean an INHERITED "get" trap counts too, and a handler
		// that happens to be a TypeDictObject (a TS enum/module namespace
		// value) must not panic AsPlainObject().
		getTrap, hasGetTrap := proxyGetTrap(proxy.handler, "get")
		if hasGetTrap && getTrap.Type() != TypeUndefined && getTrap.Type() != TypeNull {
			// Validate trap is callable
			if !getTrap.IsCallable() {
				return Undefined, vm.NewTypeError("'get' on proxy: trap is not a function")
			}
			// Call the get trap: handler.get(target, propertyKey, receiver) -
			// receiver, not obj: obj is whichever value this switch is
			// currently examining (which can be a Proxy reached partway
			// through a longer chain), while receiver is the ORIGINAL
			// object/value the caller's property access started from -
			// exactly what ECMA-262 10.5.8 step 8 passes as the trap's
			// third argument.
			trapArgs := []Value{proxy.target, NewString(propName), receiver}
			result, err := vm.Call(getTrap, proxy.handler, trapArgs)
			if err != nil {
				return Undefined, err
			}
			// ECMAScript 10.5.8 invariant validation. Only PlainObject targets
			// go through this check - proxy.target can legally be any object
			// type (Array, TypedArray, another Proxy, ...), and AsPlainObject()
			// panics on anything that isn't exactly TypeObject.
			if proxy.target.Type() == TypeObject {
				targetObj := proxy.target.AsPlainObject()
				// Check for non-configurable accessor with get=undefined
				if g, _, _, c, isAccessor := targetObj.GetOwnAccessor(propName); isAccessor && !c {
					if g.Type() == TypeUndefined && !result.IsUndefined() {
						return Undefined, vm.NewTypeError("'get' on proxy: property '" + propName + "' is a non-configurable accessor property on the proxy target and does not have a getter function, but the trap returned a non-undefined value")
					}
				} else if v, w, _, c, found := targetObj.GetOwnDescriptor(propName); found && !c && !w {
					// Non-configurable, non-writable data property: trap must return SameValue
					if !v.StrictlyEquals(result) {
						return Undefined, vm.NewTypeError("'get' on proxy: property '" + propName + "' is a read-only and non-configurable data property on the proxy target but the proxy did not return its actual value")
					}
				}
			}
			return result, nil
		}
		// No get trap, fall through to target - still threading the
		// original receiver through, not proxy.target, so a getter found
		// further down the chain still sees the right `this`.
		return vm.getPropertyWithReceiver(proxy.target, propName, receiver)

	case TypePromise:
		// Promise objects: own side-table property first (same table/gap
		// as TypeSet/TypeMap above - Promises don't expose own state as
		// ordinary properties, but a plain assignment or
		// Object.defineProperty can still add one), then Promise.prototype.
		if props := OwnPropertiesTable(obj); props != nil {
			if g, _, _, _, ok := props.GetOwnAccessor(propName); ok {
				if g.Type() != TypeUndefined {
					result, err := vm.Call(g, receiver, nil)
					if err != nil {
						return Undefined, err
					}
					return result, nil
				}
				return Undefined, nil
			}
			if v, ok := props.GetOwn(propName); ok {
				return v, nil
			}
		}
		// vm.PrototypeOf, not vm.PromisePrototype: a promise built by a
		// subclass constructor carries a per-instance [[Prototype]] override,
		// and hardcoding the intrinsic made every native read on such a
		// promise - `constructor` above all, which is step 1 of
		// SpeciesConstructor - answer as though it were a plain Promise. The
		// bytecode read path (pkg/vm/property_helpers.go's TypePromise case)
		// has honored the override since paserati#198.
		// Type() == TypeObject, not IsObject(): IsObject is the contiguous
		// [TypeObject, TypeProxy] range check, so a [[Prototype]] that is a
		// Proxy/DictObject/Array (reachable via Reflect.construct with a
		// newTarget whose .prototype is one) would pass it and then have its
		// pointer reinterpreted by AsPlainObject. Same guard as
		// property_helpers.go's per-kind cases.
		if proto := vm.PrototypeOf(obj); proto.Type() == TypeObject {
			if v, ok := proto.AsPlainObject().Get(propName); ok {
				return v, nil
			}
		}
		return Undefined, nil

	case TypeRegExp:
		// RegExp objects: check own properties first, then RegExp.prototype
		regexObj := obj.AsRegExpObject()
		if regexObj != nil {
			// Handle lastIndex as own data property
			if propName == "lastIndex" {
				return NumberValue(float64(regexObj.GetLastIndex())), nil
			}
			// Check own properties (for overridden methods like custom exec)
			if regexObj.Properties != nil {
				if v, ok := regexObj.Properties.GetOwn(propName); ok {
					return v, nil
				}
			}
			// vm.plainPrototypeOf(obj) rather than the hardcoded
			// RegExp.prototype: a `class S extends RegExp {}` instance carries
			// its own [[Prototype]], and starting from the intrinsic meant
			// Symbol.replace/match/split - which read `exec` off the regexp
			// natively - could never see a subclass's override.
			if v, found, err := vm.lookupOnPrototypeChain(vm.plainPrototypeOf(obj), propName, receiver); found || err != nil {
				return v, err
			}
		}
		return Undefined, nil

	case TypeTypedArray:
		// TypedArray: check own properties first, then built-in properties, then prototype
		ta := obj.AsTypedArray()
		if ta != nil {
			// Check own properties first (e.g., overridden constructor)
			if v, ok := ta.GetOwnProperty(propName); ok {
				return v, nil
			}
			// Check built-in properties
			switch propName {
			case "length":
				return NumberValue(float64(ta.GetLength())), nil
			case "byteLength":
				return NumberValue(float64(ta.GetByteLength())), nil
			case "byteOffset":
				return NumberValue(float64(ta.GetByteOffset())), nil
			case "buffer":
				if ta.GetBuffer() != nil {
					return Value{typ: TypeArrayBuffer, obj: unsafe.Pointer(ta.GetBuffer())}, nil
				}
				return Undefined, nil
			case "BYTES_PER_ELEMENT":
				return NumberValue(float64(ta.GetBytesPerElement())), nil
			}
			// Check numeric index access
			if idx, err := strconv.Atoi(propName); err == nil && idx >= 0 && idx < ta.GetLength() {
				return ta.GetElement(idx), nil
			}
			// vm.PrototypeOf, not a local element-type switch: a typed array
			// built by a subclass constructor (`class Foo extends Uint8Array {}`)
			// or by Reflect.construct with a custom newTarget carries a
			// per-instance [[Prototype]] override, and only PrototypeOf consults
			// it before falling back to the intrinsic for the element type. The
			// bytecode read path (pkg/vm/property_helpers.go's TypeTypedArray
			// case) has always honored the override; these native ones silently
			// resolved every subclass instance to the intrinsic prototype, which
			// is why TypedArraySpeciesCreate read `constructor` as Uint8Array and
			// filter/map/subarray never produced subclass results.
			proto := vm.PrototypeOf(obj)
			// Check prototype chain - need to check for accessors (getters) first.
			// Type() == TypeObject rather than IsObject() - see the TypePromise
			// case above for why the range check is not safe before AsPlainObject.
			if proto.Type() == TypeObject {
				cur := proto.AsPlainObject()
				for cur != nil {
					// Check for accessor (getter) first
					if getter, _, _, _, ok := cur.GetOwnAccessor(propName); ok {
						if getter.Type() != TypeUndefined {
							// Call the getter with this=obj (the TypedArray)
							result, err := vm.Call(getter, receiver, nil)
							if err != nil {
								return Undefined, err
							}
							return result, nil
						}
						// Accessor exists but no getter - return undefined
						return Undefined, nil
					}
					// Check for regular property
					if v, ok := cur.GetOwn(propName); ok {
						return v, nil
					}
					// Walk prototype chain
					protoVal := cur.GetPrototype()
					if protoVal.Type() != TypeObject {
						break
					}
					cur = protoVal.AsPlainObject()
				}
			}
		}
		return Undefined, nil

	case TypeSet:
		// Set objects: own side-table property first (the same lazily-
		// allocated table a plain `set.foo = 1`/Object.defineProperty
		// assignment writes into - OwnPropertiesTable,
		// pkg/vm/properties_table.go), THEN the prototype chain (for
		// accessor properties like size). Before this, a Set's own custom
		// property was invisible here even though Object.getOwnPropertyDescriptor/
		// `in`/Reflect.has already agreed it existed - same "kind missing a
		// side-table check" gap already fixed elsewhere in this switch
		// (RegExp) and across OpIn/Object.keys/etc. in prior PRs.
		if props := OwnPropertiesTable(obj); props != nil {
			if g, _, _, _, ok := props.GetOwnAccessor(propName); ok {
				if g.Type() != TypeUndefined {
					result, err := vm.Call(g, receiver, nil)
					if err != nil {
						return Undefined, err
					}
					return result, nil
				}
				return Undefined, nil
			}
			if v, ok := props.GetOwn(propName); ok {
				return v, nil
			}
		}
		// vm.plainPrototypeOf(obj) rather than the hardcoded intrinsic:
		// a subclass instance carries its own [[Prototype]], and starting
		// from Set.prototype made every override on it unreachable
		// from native code. lookupOnPrototypeChain (pkg/vm/proto.go) walks
		// that chain invoking accessors with this = receiver.
		if v, found, err := vm.lookupOnPrototypeChain(vm.plainPrototypeOf(obj), propName, receiver); found || err != nil {
			return v, err
		}
		return Undefined, nil

	case TypeMap:
		// Map objects: same own-side-table-then-prototype-chain shape as
		// TypeSet just above - see its comment.
		if props := OwnPropertiesTable(obj); props != nil {
			if g, _, _, _, ok := props.GetOwnAccessor(propName); ok {
				if g.Type() != TypeUndefined {
					result, err := vm.Call(g, receiver, nil)
					if err != nil {
						return Undefined, err
					}
					return result, nil
				}
				return Undefined, nil
			}
			if v, ok := props.GetOwn(propName); ok {
				return v, nil
			}
		}
		// vm.plainPrototypeOf(obj) rather than the hardcoded intrinsic:
		// a subclass instance carries its own [[Prototype]], and starting
		// from Map.prototype made every override on it unreachable
		// from native code. lookupOnPrototypeChain (pkg/vm/proto.go) walks
		// that chain invoking accessors with this = receiver.
		if v, found, err := vm.lookupOnPrototypeChain(vm.plainPrototypeOf(obj), propName, receiver); found || err != nil {
			return v, err
		}
		return Undefined, nil

	case TypeFunction:
		// Function objects: check own properties, then [[Prototype]] chain, then Function.prototype
		fn := obj.AsFunction()
		if fn != nil {
			// Check own properties first
			if fn.Properties != nil {
				// Check for accessor (getter) on own properties
				if getter, _, _, _, ok := fn.Properties.GetOwnAccessor(propName); ok {
					if getter.Type() != TypeUndefined {
						result, err := vm.Call(getter, receiver, nil)
						if err != nil {
							return Undefined, err
						}
						return result, nil
					}
					return Undefined, nil
				}
				if v, ok := fn.Properties.GetOwn(propName); ok {
					return v, nil
				}
			}
			// Check built-in properties
			switch propName {
			case "name":
				return NewString(fn.Name), nil
			case "length":
				return NumberValue(float64(fn.Length)), nil
			}
			// Walk [[Prototype]] chain (set by Object.setPrototypeOf)
			if fn.Prototype.Type() != TypeUndefined && fn.Prototype.Type() != TypeNull {
				return vm.getPropertyWithReceiver(fn.Prototype, propName, receiver)
			}
			// Fall back to Function.prototype - use function's own realm if available (cross-realm)
			funcProto := vm.FunctionPrototype
			if fn.HomeRealm != nil && fn.HomeRealm != vm.currentRealm {
				funcProto = fn.HomeRealm.FunctionPrototype
			}
			if funcProto.Type() == TypeNativeFunctionWithProps {
				nfp := funcProto.AsNativeFunctionWithProps()
				if nfp != nil && nfp.Properties != nil {
					// Check for accessor first
					if getter, _, _, _, ok := nfp.Properties.GetOwnAccessor(propName); ok {
						if getter.Type() != TypeUndefined {
							result, err := vm.Call(getter, receiver, nil)
							if err != nil {
								return Undefined, err
							}
							return result, nil
						}
						return Undefined, nil
					}
					if v, ok := nfp.Properties.Get(propName); ok {
						return v, nil
					}
				}
			}
		}
		return Undefined, nil

	case TypeClosure:
		// Closure objects: check own properties, then [[Prototype]] chain, then Function.prototype
		cl := obj.AsClosure()
		if cl != nil {
			// Check own properties first
			if cl.Properties != nil {
				// Check for accessor (getter) on own properties
				if getter, _, _, _, ok := cl.Properties.GetOwnAccessor(propName); ok {
					if getter.Type() != TypeUndefined {
						result, err := vm.Call(getter, receiver, nil)
						if err != nil {
							return Undefined, err
						}
						return result, nil
					}
					return Undefined, nil
				}
				if v, ok := cl.Properties.GetOwn(propName); ok {
					return v, nil
				}
			}
			// Check built-in properties
			if cl.Fn != nil {
				switch propName {
				case "name":
					return NewString(cl.Fn.Name), nil
				case "length":
					return NumberValue(float64(cl.Fn.Length)), nil
				}
				// Walk [[Prototype]] chain (set by Object.setPrototypeOf)
				if cl.Fn.Prototype.Type() != TypeUndefined && cl.Fn.Prototype.Type() != TypeNull {
					return vm.getPropertyWithReceiver(cl.Fn.Prototype, propName, receiver)
				}
			}
			// Fall back to Function.prototype (which is a NativeFunctionWithProps)
			if vm.FunctionPrototype.Type() == TypeNativeFunctionWithProps {
				nfp := vm.FunctionPrototype.AsNativeFunctionWithProps()
				if nfp != nil && nfp.Properties != nil {
					// Check for accessor first
					if getter, _, _, _, ok := nfp.Properties.GetOwnAccessor(propName); ok {
						if getter.Type() != TypeUndefined {
							result, err := vm.Call(getter, receiver, nil)
							if err != nil {
								return Undefined, err
							}
							return result, nil
						}
						return Undefined, nil
					}
					if v, ok := nfp.Properties.Get(propName); ok {
						return v, nil
					}
				}
			}
		}
		return Undefined, nil

	case TypeNativeFunctionWithProps:
		// Native function with props: check own properties, then Function.prototype
		nfp := obj.AsNativeFunctionWithProps()
		if nfp != nil && nfp.Properties != nil {
			// Check for accessor (getter) on own properties
			if getter, _, _, _, ok := nfp.Properties.GetOwnAccessor(propName); ok {
				if getter.Type() != TypeUndefined {
					result, err := vm.Call(getter, receiver, nil)
					if err != nil {
						return Undefined, err
					}
					return result, nil
				}
				return Undefined, nil
			}
			if v, ok := nfp.Properties.GetOwn(propName); ok {
				return v, nil
			}
		}
		// Check built-in properties
		if nfp != nil {
			switch propName {
			case "name":
				if !nfp.DeletedName {
					return NewString(nfp.Name), nil
				}
			case "length":
				if !nfp.DeletedLength {
					return NumberValue(float64(nfp.Arity)), nil
				}
			}
		}
		// Check Function.prototype (which is a NativeFunctionWithProps)
		if vm.FunctionPrototype.Type() == TypeNativeFunctionWithProps {
			fpNfp := vm.FunctionPrototype.AsNativeFunctionWithProps()
			if fpNfp != nil && fpNfp.Properties != nil {
				// Check for accessor first
				if getter, _, _, _, ok := fpNfp.Properties.GetOwnAccessor(propName); ok {
					if getter.Type() != TypeUndefined {
						result, err := vm.Call(getter, receiver, nil)
						if err != nil {
							return Undefined, err
						}
						return result, nil
					}
					return Undefined, nil
				}
				if v, ok := fpNfp.Properties.Get(propName); ok {
					return v, nil
				}
			}
		}
		return Undefined, nil

	case TypeNativeFunction:
		// A plain native function (e.g. Array.prototype.push) - same shape
		// as TypeNativeFunctionWithProps just above, except its own
		// intrinsics live directly on the value's own fields rather than
		// under a *PlainObject with a IsConstructor flag. This case didn't
		// exist at all before this fix (found while implementing
		// Reflect.get's general property support, but this function -
		// GetProperty/GetPropertyWithReceiver - is also used elsewhere,
		// e.g. Reflect.apply/construct's generic array-like .length
		// access), so Reflect.get(Array.prototype.push, "name") and
		// friends always answered undefined, and any custom own property
		// (Object.defineProperty, bracket-notation assignment) was
		// invisible too - even though `in`/Reflect.has/
		// Object.getOwnPropertyDescriptor already agreed those exist
		// (fixed for THEM in earlier PRs in this stack).
		nf := obj.AsNativeFunction()
		if nf != nil && nf.Properties != nil {
			if getter, _, _, _, ok := nf.Properties.GetOwnAccessor(propName); ok {
				if getter.Type() != TypeUndefined {
					result, err := vm.Call(getter, receiver, nil)
					if err != nil {
						return Undefined, err
					}
					return result, nil
				}
				return Undefined, nil
			}
			if v, ok := nf.Properties.GetOwn(propName); ok {
				return v, nil
			}
		}
		if nf != nil {
			switch propName {
			case "name":
				if !nf.DeletedName {
					return NewString(nf.Name), nil
				}
			case "length":
				if !nf.DeletedLength {
					return NumberValue(float64(nf.Arity)), nil
				}
			}
		}
		if vm.FunctionPrototype.Type() == TypeNativeFunctionWithProps {
			fpNfp := vm.FunctionPrototype.AsNativeFunctionWithProps()
			if fpNfp != nil && fpNfp.Properties != nil {
				if getter, _, _, _, ok := fpNfp.Properties.GetOwnAccessor(propName); ok {
					if getter.Type() != TypeUndefined {
						result, err := vm.Call(getter, receiver, nil)
						if err != nil {
							return Undefined, err
						}
						return result, nil
					}
					return Undefined, nil
				}
				if v, ok := fpNfp.Properties.Get(propName); ok {
					return v, nil
				}
			}
		}
		return Undefined, nil

	case TypeBoundFunction:
		// Bound function: check own properties, then Function.prototype
		bf := obj.AsBoundFunction()
		if bf != nil && bf.Properties != nil {
			// Check for accessor (getter) on own properties
			if getter, _, _, _, ok := bf.Properties.GetOwnAccessor(propName); ok {
				if getter.Type() != TypeUndefined {
					result, err := vm.Call(getter, receiver, nil)
					if err != nil {
						return Undefined, err
					}
					return result, nil
				}
				return Undefined, nil
			}
			if v, ok := bf.Properties.GetOwn(propName); ok {
				return v, nil
			}
		}
		// Check Function.prototype (which is a NativeFunctionWithProps)
		if vm.FunctionPrototype.Type() == TypeNativeFunctionWithProps {
			nfp := vm.FunctionPrototype.AsNativeFunctionWithProps()
			if nfp != nil && nfp.Properties != nil {
				// Check for accessor first
				if getter, _, _, _, ok := nfp.Properties.GetOwnAccessor(propName); ok {
					if getter.Type() != TypeUndefined {
						result, err := vm.Call(getter, receiver, nil)
						if err != nil {
							return Undefined, err
						}
						return result, nil
					}
					return Undefined, nil
				}
				if v, ok := nfp.Properties.Get(propName); ok {
					return v, nil
				}
			}
		}
		return Undefined, nil

	case TypeArrayBuffer, TypeSharedArrayBuffer, TypeDataView,
		TypeWeakMap, TypeWeakSet, TypeWeakRef, TypeFinalizationRegistry:
		// These kinds had no case at all and fell through to the primitive
		// default at the bottom of this switch, which leaves proto unset - so
		// every native read on them answered undefined, including the
		// `constructor` that ArrayBuffer.prototype.slice's SpeciesConstructor
		// step depends on. Own side-table property first (the same table a
		// plain `ab.foo = 1` writes into), then the instance's own
		// [[Prototype]] chain so a subclass's overrides are reachable.
		if props := OwnPropertiesTable(obj); props != nil {
			if v, found, err := vm.getOwnFromTableByKey(props, keyFromString(propName), receiver); found || err != nil {
				return v, err
			}
		}
		if v, found, err := vm.lookupOnPrototypeChain(vm.plainPrototypeOf(obj), propName, receiver); found || err != nil {
			return v, err
		}
		return Undefined, nil

	case TypeArguments:
		// Arguments objects: check length, numeric indices, and named properties
		args := obj.AsArguments()
		if args != nil {
			// 'length'/'callee' can be overridden by an explicit assignment
			// (stored in the overflow named-property map, since the real
			// a.length/a.callee fields back the live argument count/callee
			// instead) - an override must win over the live value. See
			// op_setprop.go's TypeArguments case, which is the write side
			// of this same convention.
			if propName == "length" {
				if v, ok := args.GetNamedProp("length"); ok {
					return v, nil
				}
				return NumberValue(float64(args.Length())), nil
			}
			// Check for 'callee' property (only in non-strict mode)
			if propName == "callee" && !args.IsStrict() {
				if v, ok := args.GetNamedProp("callee"); ok {
					return v, nil
				}
				return args.Callee(), nil
			}
			// Check for numeric index access. Goes through argumentsGet (see
			// arguments_props.go) rather than the raw Get()/mappedRegs fast
			// path so a defineProperty-installed accessor or an explicit
			// deletion is respected instead of always reading the live
			// value straight through.
			if _, isIndex := ParseArgumentsIndex(propName); isIndex {
				return vm.argumentsGet(args, propName)
			}
			// Check for named properties (like value, writable, get, set, etc.)
			if v, ok := args.GetNamedProp(propName); ok {
				return v, nil
			}
			// Check Object.prototype for inherited properties
			if vm.ObjectPrototype.IsObject() {
				proto := vm.ObjectPrototype.AsPlainObject()
				if v, ok := proto.Get(propName); ok {
					return v, nil
				}
			}
		}
		return Undefined, nil

	case TypeBigInt:
		// BigInt values: check BigInt.prototype chain
		if vm.BigIntPrototype.IsObject() {
			proto := vm.BigIntPrototype.AsPlainObject()
			// Check for accessor (getter) first
			if getter, _, _, _, ok := proto.GetOwnAccessor(propName); ok {
				if getter.Type() != TypeUndefined {
					// Call the getter with this=obj (the BigInt)
					result, err := vm.Call(getter, receiver, nil)
					if err != nil {
						return Undefined, err
					}
					return result, nil
				}
				return Undefined, nil
			}
			// Check for regular property
			if v, ok := proto.Get(propName); ok {
				return v, nil
			}
		}
		return Undefined, nil

	default:
		// For primitive types, look up property on their wrapper prototype
		var proto Value
		switch obj.Type() {
		case TypeBoolean:
			proto = vm.BooleanPrototype
		case TypeFloatNumber, TypeIntegerNumber:
			proto = vm.NumberPrototype
		case TypeString:
			proto = vm.StringPrototype
		case TypeSymbol:
			proto = vm.SymbolPrototype
		case TypeBigInt:
			proto = vm.BigIntPrototype
		}
		if proto.IsObject() {
			po := proto.AsPlainObject()
			// Check for accessor (getter) first
			if g, _, _, _, ok := po.GetOwnAccessor(propName); ok && g.Type() != TypeUndefined {
				result, err := vm.Call(g, receiver, nil)
				if err != nil {
					return Undefined, err
				}
				return result, nil
			}
			if v, ok := po.GetOwn(propName); ok {
				return v, nil
			}
			// Walk prototype chain
			current := po.GetPrototype()
			for current.typ != TypeNull && current.typ != TypeUndefined {
				if current.IsObject() {
					if current.Type() == TypeObject {
						cur := current.AsPlainObject()
						if g, _, _, _, ok := cur.GetOwnAccessor(propName); ok && g.Type() != TypeUndefined {
							result, err := vm.Call(g, receiver, nil)
							if err != nil {
								return Undefined, err
							}
							return result, nil
						}
						if v, ok := cur.GetOwn(propName); ok {
							return v, nil
						}
						current = cur.GetPrototype()
					} else {
						break
					}
				} else {
					break
				}
			}
		}
		return Undefined, nil
	}
}

// getOwnFromTableByKey checks one *PlainObject-backed own-property table
// (a plain object's own fields, or an exotic kind's lazily-allocated
// Properties side table - Function/Closure/NativeFunction/
// NativeFunctionWithProps/BoundFunction/RegExp/Map/Set/Promise all keep
// one, see pkg/vm/properties_table.go) for key, invoking an accessor's
// getter with `this = receiver` if key names one. Returns (value, found,
// error) - found is false and error is nil when key simply isn't there,
// so callers can fall through to whatever comes next (a prototype chain,
// a different table) without misreading "not found" as "found undefined".
func (vm *VM) getOwnFromTableByKey(table *PlainObject, key PropertyKey, receiver Value) (Value, bool, error) {
	if table == nil {
		return Undefined, false, nil
	}
	if g, _, _, _, ok := table.GetOwnAccessorByKey(key); ok {
		if g.Type() != TypeUndefined {
			res, err := vm.Call(g, receiver, nil)
			if err != nil {
				return Undefined, true, err
			}
			return res, true, nil
		}
		// Setter-only accessor (no getter): reads as undefined per spec,
		// but IS present, so still reported as found - a caller falling
		// through to a lower-priority table/prototype for this key would
		// otherwise incorrectly resurrect a shadowed property from there.
		return Undefined, true, nil
	}
	if v, ok := table.GetOwnByKey(key); ok {
		return v, true, nil
	}
	return Undefined, false, nil
}

// walkPlainObjectChainForKey walks a chain of ordinary (TypeObject)
// prototypes starting at start, checking each level's own table via
// getOwnFromTableByKey - the same accessor-then-data, getter-invoking
// shape opGetPropSymbol's own TypeObject case (pkg/vm/op_getprop.go)
// already uses for a direct property access, reused here so
// GetPropertyWithReceiver (backing Reflect.get) gets the same fidelity
// instead of a second, weaker, hand-rolled walk. Stops (found=false) as
// soon as the chain reaches a non-TypeObject value (Null, a Proxy, or any
// other exotic kind) - every concrete built-in prototype this is called
// with (Array.prototype, Map.prototype, RegExp.prototype, the primitive
// wrapper prototypes, ...) terminates in Object.prototype, itself a plain
// TypeObject, so this is not a real limitation for those callers.
func (vm *VM) walkPlainObjectChainForKey(start Value, key PropertyKey, receiver Value) (Value, bool, error) {
	current := start
	for current.IsObject() {
		if current.Type() != TypeObject {
			break
		}
		po := current.AsPlainObject()
		if v, found, err := vm.getOwnFromTableByKey(po, key, receiver); found || err != nil {
			return v, found, err
		}
		current = po.GetPrototype()
	}
	return Undefined, false, nil
}

// ReflectGetSymbolProperty is GetProperty for a Symbol-keyed property
// (rather than a string-keyed one) - see GetPropertyWithReceiver's doc
// comment for the receiver parameter's meaning. sym must be a Value of
// TypeSymbol. Named distinctly from the pre-existing, narrower
// GetSymbolProperty (obj, symbol) (Value, bool) below (RegExp/TypeObject
// only, no getter invocation on non-RegExp/TypeObject kinds, no error
// return) rather than overloading that name for a wider, error-returning
// implementation an existing caller of the old one isn't expecting.
func (vm *VM) ReflectGetSymbolProperty(obj Value, sym Value) (Value, error) {
	return vm.getSymbolPropertyWithReceiver(obj, sym, obj)
}

// ReflectGetSymbolPropertyWithReceiver is ReflectGetSymbolProperty with an
// explicit receiver - the symbol-key counterpart of
// GetPropertyWithReceiver, added for the same reason
// (Reflect.get(target, someSymbol[, receiver]), pkg/builtins/reflect_init.go).
func (vm *VM) ReflectGetSymbolPropertyWithReceiver(obj Value, sym Value, receiver Value) (Value, error) {
	return vm.getSymbolPropertyWithReceiver(obj, sym, receiver)
}

// getSymbolPropertyWithReceiver mirrors getPropertyWithReceiver's per-kind
// switch (same case list, same comments' reasoning) with the string-keyed
// PlainObject methods (GetOwn/GetOwnAccessor/Get) replaced by their
// *ByKey counterparts, i.e. using key := NewSymbolKey(sym) throughout
// instead of propName. Where getPropertyWithReceiver open-codes its own
// per-kind accessor/data check and hand-rolled prototype walk inline,
// this uses the two shared helpers above instead - written once, for the
// new symbol path, rather than reproducing that duplication a second
// time; getPropertyWithReceiver's already-tested string-key logic is left
// exactly as it was, not retrofitted onto the same helpers, to avoid
// risking a regression there for this task's sake.
//
// The prototype-chain fallback used by every callable kind
// (TypeFunction/TypeClosure/TypeNativeFunction/TypeNativeFunctionWithProps/
// TypeBoundFunction) goes through lookupSymbolWithReceiver
// (pkg/vm/symbol_lookup.go) rather than walkPlainObjectChainForKey, since a
// callable's chain can pass through Function.prototype - itself a
// TypeNativeFunctionWithProps at runtime - and through other constructors,
// neither of which walkPlainObjectChainForKey's TypeObject-only walk
// handles. That helper invokes an accessor's getter found anywhere on the
// chain with this = receiver; an earlier version used
// lookupSymbolOnProtoChain against a hardcoded Function.prototype and did
// neither, which is what made an inherited `get [Symbol.species]`
// unreadable.
func (vm *VM) getSymbolPropertyWithReceiver(obj Value, sym Value, receiver Value) (Value, error) {
	key := NewSymbolKey(sym)

	switch obj.Type() {
	case TypeObject:
		// Not walkPlainObjectChainForKey directly: that helper stops the
		// instant the chain reaches a non-TypeObject value, which is fine
		// for the built-in-prototype walks below (Array.prototype and
		// siblings always terminate in Object.prototype, itself a plain
		// TypeObject) but wrong here - obj's own [[Prototype]] chain can
		// legitimately pass through a callable (`Object.create(someFn)`,
		// or a class extending a native constructor), and a symbol
		// property can live in THAT value's own Properties side table,
		// not a PlainObject's fields. Recursing into the full per-kind
		// dispatch (this same function) once the chain leaves TypeObject
		// picks that up, instead of silently answering "not found":
		//
		//   const sym = Symbol("s");
		//   function base() {}
		//   base[sym] = "found";
		//   Reflect.get(Object.create(base), sym); // Node: "found"
		current := obj
		for current.Type() == TypeObject {
			po := current.AsPlainObject()
			if v, found, err := vm.getOwnFromTableByKey(po, key, receiver); found || err != nil {
				return v, err
			}
			current = po.GetPrototype()
		}
		if current.typ == TypeNull || current.typ == TypeUndefined {
			return Undefined, nil
		}
		// NOT gated on current.IsObject(): every callable kind sorts
		// BEFORE TypeObject in the ValueType enum (pkg/vm/value.go), so
		// IsObject() - a contiguous [TypeObject, TypeProxy] range check -
		// is FALSE for exactly the values (TypeFunction, TypeClosure,
		// TypeNativeFunction, TypeNativeFunctionWithProps,
		// TypeBoundFunction) this branch exists to reach. Gating on it
		// (an earlier version of this fix did) silently reintroduced the
		// "stops early, answers not-found" bug the comment above
		// describes fixing - see getPropertyWithReceiver's TypeObject
		// case (the string-key sibling of this one) for the same mistake
		// caught and fixed the same way.
		return vm.getSymbolPropertyWithReceiver(current, sym, receiver)

	case TypeDictObject:
		// DictObject ignores symbols entirely - matches OpIn's own
		// symbol-key TypeDictObject case (pkg/vm/vm.go).
		return Undefined, nil

	case TypeArray:
		arr := obj.AsArray()
		if arr != nil {
			symObj := sym.AsSymbolObject()
			// A symbol-keyed accessor defined via Object.defineProperty(arr,
			// sym, {get, set}) takes priority over the plain symbolProps
			// value - mirrors getOwnFromTableByKey's accessor-then-data
			// order just above, and opGetPropSymbol's own TypeArray case
			// (pkg/vm/op_getprop.go), which this function must agree with
			// per PR #350's own invariant (arr[sym] === Reflect.get(arr,
			// sym)) - Object.defineProperty on a symbol key was itself
			// still a no-op when that invariant was written, so this
			// couldn't yet be tested there. See ArrayObject.GetOwnSymbolAccessor.
			if arr.HasSymbolAccessors() {
				if g, _, _, _, ok := arr.GetOwnSymbolAccessor(symObj); ok {
					if g.Type() == TypeUndefined {
						return Undefined, nil
					}
					return vm.Call(g, receiver, nil)
				}
			}
			if v, ok := arr.GetSymbolProp(symObj); ok {
				return v, nil
			}
			if vm.ArrayPrototype.IsObject() {
				v, _, err := vm.walkPlainObjectChainForKey(vm.ArrayPrototype, key, receiver)
				return v, err
			}
		}
		return Undefined, nil

	case TypeGenerator:
		if vm.GeneratorPrototype.IsObject() {
			v, _, err := vm.walkPlainObjectChainForKey(vm.GeneratorPrototype, key, receiver)
			return v, err
		}
		return Undefined, nil

	case TypeProxy:
		proxy := obj.AsProxy()
		if proxy.Revoked {
			return Undefined, vm.NewTypeError("Cannot perform 'get' on a revoked Proxy")
		}
		getTrap, hasGetTrap := proxyGetTrap(proxy.handler, "get")
		if hasGetTrap && getTrap.Type() != TypeUndefined && getTrap.Type() != TypeNull {
			if !getTrap.IsCallable() {
				return Undefined, vm.NewTypeError("'get' on proxy: trap is not a function")
			}
			trapArgs := []Value{proxy.target, sym, receiver}
			result, err := vm.Call(getTrap, proxy.handler, trapArgs)
			if err != nil {
				return Undefined, err
			}
			// ECMAScript 10.5.8 invariant validation, symbol-key version
			// of getPropertyWithReceiver's TypeProxy case above.
			if proxy.target.Type() == TypeObject {
				targetObj := proxy.target.AsPlainObject()
				if g, _, _, c, isAccessor := targetObj.GetOwnAccessorByKey(key); isAccessor && !c {
					if g.Type() == TypeUndefined && !result.IsUndefined() {
						return Undefined, vm.NewTypeError("'get' on proxy: property is a non-configurable accessor property on the proxy target and does not have a getter function, but the trap returned a non-undefined value")
					}
				} else if v, w, _, c, found := targetObj.GetOwnDescriptorByKey(key); found && !c && !w {
					if !v.StrictlyEquals(result) {
						return Undefined, vm.NewTypeError("'get' on proxy: property is a read-only and non-configurable data property on the proxy target but the proxy did not return its actual value")
					}
				}
			}
			return result, nil
		}
		return vm.getSymbolPropertyWithReceiver(proxy.target, sym, receiver)

	case TypePromise:
		if props := OwnPropertiesTable(obj); props != nil {
			if v, found, err := vm.getOwnFromTableByKey(props, key, receiver); found || err != nil {
				return v, err
			}
		}
		if vm.PromisePrototype.IsObject() {
			v, _, err := vm.walkPlainObjectChainForKey(vm.PromisePrototype, key, receiver)
			return v, err
		}
		return Undefined, nil

	case TypeRegExp:
		regexObj := obj.AsRegExpObject()
		if regexObj != nil {
			if regexObj.Properties != nil {
				if v, found, err := vm.getOwnFromTableByKey(regexObj.Properties, key, receiver); found || err != nil {
					return v, err
				}
			}
			proto := regexObj.GetPrototype()
			if !proto.IsObject() {
				proto = vm.RegExpPrototype
			}
			if proto.IsObject() {
				v, _, err := vm.walkPlainObjectChainForKey(proto, key, receiver)
				return v, err
			}
		}
		return Undefined, nil

	case TypeTypedArray:
		// TypedArrays don't expose own symbol-keyed properties (only
		// indices and the fixed set of string built-ins) - straight to
		// the prototype, resolved the same way as
		// getPropertyWithReceiver's TypeTypedArray case: vm.PrototypeOf so a
		// subclass instance's per-instance override wins over the intrinsic.
		ta := obj.AsTypedArray()
		if ta != nil {
			proto := vm.PrototypeOf(obj)
			if proto.Type() == TypeObject {
				v, _, err := vm.walkPlainObjectChainForKey(proto, key, receiver)
				return v, err
			}
		}
		return Undefined, nil

	case TypeSet:
		if props := OwnPropertiesTable(obj); props != nil {
			if v, found, err := vm.getOwnFromTableByKey(props, key, receiver); found || err != nil {
				return v, err
			}
		}
		if vm.SetPrototype.IsObject() {
			v, _, err := vm.walkPlainObjectChainForKey(vm.SetPrototype, key, receiver)
			return v, err
		}
		return Undefined, nil

	case TypeMap:
		if props := OwnPropertiesTable(obj); props != nil {
			if v, found, err := vm.getOwnFromTableByKey(props, key, receiver); found || err != nil {
				return v, err
			}
		}
		if vm.MapPrototype.IsObject() {
			v, _, err := vm.walkPlainObjectChainForKey(vm.MapPrototype, key, receiver)
			return v, err
		}
		return Undefined, nil

	case TypeFunction:
		fn := obj.AsFunction()
		if fn != nil && fn.Properties != nil {
			if v, found, err := vm.getOwnFromTableByKey(fn.Properties, key, receiver); found || err != nil {
				return v, err
			}
		}
		// Walk obj's real [[Prototype]] chain, not a hardcoded
		// Function.prototype: a class constructor's parent is its superclass
		// (`class Foo extends Uint8Array {}`), and a built-in constructor's is
		// whatever its Properties table carries (Uint8Array -> %TypedArray%).
		// Accessors found along the way are invoked with this = receiver.
		if v, found, err := vm.lookupSymbolWithReceiver(vm.symbolProtoOf(obj), key, receiver); found || err != nil {
			return v, err
		}
		return Undefined, nil

	case TypeClosure:
		cl := obj.AsClosure()
		if cl != nil {
			if cl.Properties != nil {
				if v, found, err := vm.getOwnFromTableByKey(cl.Properties, key, receiver); found || err != nil {
					return v, err
				}
			}
			if cl.Fn != nil && cl.Fn.Properties != nil {
				if v, found, err := vm.getOwnFromTableByKey(cl.Fn.Properties, key, receiver); found || err != nil {
					return v, err
				}
			}
		}
		// obj's real [[Prototype]] chain, accessors invoked with this = receiver
		// (see this function's doc comment).
		if v, found, err := vm.lookupSymbolWithReceiver(vm.symbolProtoOf(obj), key, receiver); found || err != nil {
			return v, err
		}
		return Undefined, nil

	case TypeNativeFunction:
		nf := obj.AsNativeFunction()
		if nf != nil && nf.Properties != nil {
			if v, found, err := vm.getOwnFromTableByKey(nf.Properties, key, receiver); found || err != nil {
				return v, err
			}
		}
		// obj's real [[Prototype]] chain, accessors invoked with this = receiver
		// (see this function's doc comment).
		if v, found, err := vm.lookupSymbolWithReceiver(vm.symbolProtoOf(obj), key, receiver); found || err != nil {
			return v, err
		}
		return Undefined, nil

	case TypeNativeFunctionWithProps:
		nfp := obj.AsNativeFunctionWithProps()
		if nfp != nil && nfp.Properties != nil {
			if v, found, err := vm.getOwnFromTableByKey(nfp.Properties, key, receiver); found || err != nil {
				return v, err
			}
		}
		// obj's real [[Prototype]] chain, accessors invoked with this = receiver
		// (see this function's doc comment).
		if v, found, err := vm.lookupSymbolWithReceiver(vm.symbolProtoOf(obj), key, receiver); found || err != nil {
			return v, err
		}
		return Undefined, nil

	case TypeBoundFunction:
		bf := obj.AsBoundFunction()
		if bf != nil && bf.Properties != nil {
			if v, found, err := vm.getOwnFromTableByKey(bf.Properties, key, receiver); found || err != nil {
				return v, err
			}
		}
		// obj's real [[Prototype]] chain, accessors invoked with this = receiver
		// (see this function's doc comment).
		if v, found, err := vm.lookupSymbolWithReceiver(vm.symbolProtoOf(obj), key, receiver); found || err != nil {
			return v, err
		}
		return Undefined, nil

	case TypeArguments:
		args := obj.AsArguments()
		if args != nil {
			if v, ok := args.GetSymbolProp(sym.AsSymbolObject()); ok {
				return v, nil
			}
			// Symbol.iterator is inherited from Array.prototype - check
			// that first, then Object.prototype, matching OpIn's own
			// symbol-key TypeArguments case (pkg/vm/vm.go).
			if vm.ArrayPrototype.IsObject() {
				if v, found, err := vm.walkPlainObjectChainForKey(vm.ArrayPrototype, key, receiver); found || err != nil {
					return v, err
				}
			}
			if vm.ObjectPrototype.IsObject() {
				v, _, err := vm.walkPlainObjectChainForKey(vm.ObjectPrototype, key, receiver)
				return v, err
			}
		}
		return Undefined, nil

	case TypeBigInt:
		if vm.BigIntPrototype.IsObject() {
			v, _, err := vm.walkPlainObjectChainForKey(vm.BigIntPrototype, key, receiver)
			return v, err
		}
		return Undefined, nil

	default:
		var proto Value
		switch obj.Type() {
		case TypeBoolean:
			proto = vm.BooleanPrototype
		case TypeFloatNumber, TypeIntegerNumber:
			proto = vm.NumberPrototype
		case TypeString:
			proto = vm.StringPrototype
		case TypeSymbol:
			proto = vm.SymbolPrototype
		case TypeBigInt:
			proto = vm.BigIntPrototype
		}
		if proto.IsObject() {
			v, _, err := vm.walkPlainObjectChainForKey(proto, key, receiver)
			return v, err
		}
		return Undefined, nil
	}
}

// SetProperty sets a property on an object value, properly handling setters
// This is safe to call from native functions and will trigger property setters/throw exceptions
func (vm *VM) SetProperty(obj Value, propName string, value Value) error {
	switch obj.Type() {
	case TypeObject:
		po := obj.AsPlainObject()
		// Check if it's an accessor (setter)
		if _, s, _, _, ok := po.GetOwnAccessor(propName); ok && s.Type() != TypeUndefined {
			// Call the setter with this=obj
			_, err := vm.Call(s, obj, []Value{value})
			return err
		}
		// Not an accessor, set as regular property
		po.SetOwn(propName, value)
		return nil

	case TypeRegExp:
		// Handle RegExp's lastIndex property specially
		if propName == "lastIndex" {
			regexObj := obj.AsRegExpObject()
			regexObj.SetLastIndex(int(value.ToFloat()))
			return nil
		}
		// For other properties, store on the wrapper Properties object
		regexObj := obj.AsRegExpObject()
		if regexObj.Properties == nil {
			regexObj.Properties = newPropertiesTable()
		}
		regexObj.Properties.SetOwn(propName, value)
		return nil

	case TypeArray:
		arr := obj.AsArray()
		if propName == "length" {
			arr.SetLength(toLengthIntForSetProperty(value))
			return nil
		}
		if idx, err := strconv.Atoi(propName); err == nil && idx >= 0 {
			arr.Set(idx, value)
			return nil
		}
		// Non-index properties on an Array (e.g. an ad-hoc named property)
		// aren't represented on ArrayObject in this engine; silently
		// dropping matches this function's pre-existing "no-op for
		// anything it doesn't model" convention rather than panicking.
		return nil

	case TypeArguments:
		// Mirrors op_setprop.go's TypeArguments case (the bytecode
		// OpSetProp handler) so native Go callers of SetProperty see the
		// same semantics as compiled `arguments.x = v` / `arguments[i] = v`.
		args := obj.AsArguments()
		switch propName {
		case "callee":
			args.SetNamedProp("callee", value)
		case "length":
			args.SetNamedProp("length", value)
		default:
			if _, isIndex := ParseArgumentsIndex(propName); isIndex {
				// Non-strict: rejected writes (non-writable, accessor with
				// no setter) silently no-op rather than erroring, matching
				// this function's other cases.
				return vm.argumentsSet(args, propName, value, false)
			}
			args.SetNamedProp(propName, value)
		}
		return nil

	case TypeProxy:
		// This case didn't exist at all before this fix - obj.Type() ==
		// TypeProxy fell to the `default: return nil` no-op below, so
		// every native Go caller of SetProperty (e.g.
		// pkg/builtins/array_generic.go's arrayLikeSetLength, which
		// Array.prototype.pop/shift/... use to shrink .length) silently
		// did nothing when writing to a Proxy - `Array.prototype.pop.call(
		// new Proxy(realArray, {}))` deleted the right element but never
		// actually updated realArray.length, even though ordinary
		// `proxy.length = n` assignment syntax (the bytecode OpSetProp
		// path, op_setprop.go) already handled this correctly. Mirrors
		// op_setprop.go's own TypeProxy case: GetMethod(handler, "set")
		// via getInheritedGeneric (an inherited trap counts, and this
		// covers the same wider range of legal handler kinds
		// proxyGetTrap's TypeObject/TypeDictObject-only switch doesn't -
		// Array, Closure, Function, ...), and a no-trap fallback that
		// recurses into SetProperty on the target - covering the same
		// TypeArray/TypeObject/TypeDictObject/TypeArguments/TypeRegExp
		// cases above (including "length" on a real Array) plus a
		// further-nested Proxy target automatically, since this same
		// switch runs again for it.
		proxy := obj.AsProxy()
		if proxy.Revoked {
			return vm.NewTypeError("Cannot set property on a revoked Proxy")
		}
		setTrap, hasSetTrap := vm.getInheritedGeneric(proxy.Handler(), "set")
		if hasSetTrap && setTrap.Type() != TypeUndefined && setTrap.Type() != TypeNull {
			if !setTrap.IsCallable() {
				return vm.NewTypeError("'set' on proxy: trap is not a function")
			}
			// handler.set(target, propertyKey, value, receiver) - receiver
			// is the proxy itself, per ECMA-262 10.5.9 step 8.
			result, err := vm.Call(setTrap, proxy.Handler(), []Value{proxy.Target(), NewString(propName), value, obj})
			if err != nil {
				return err
			}
			if result.IsFalsey() {
				return vm.NewTypeError("'set' on proxy: trap returned falsish for property '" + propName + "'")
			}
			return nil
		}
		// No set trap: delegate to target.[[Set]]() - recursing through
		// this same switch handles a further-nested Proxy target too.
		return vm.SetProperty(proxy.Target(), propName, value)

	default:
		// For non-objects, this is a no-op (or could throw in strict mode)
		return nil
	}
}

// toLengthIntForSetProperty mirrors builtins.toLengthInt (ToLength clamping)
// without creating an import cycle - SetProperty needs the same clamping
// when a caller writes an Array's "length" property to an arbitrary value.
func toLengthIntForSetProperty(v Value) int {
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

// GetSymbolPropertyWithGetter gets a symbol property from an object value, handling getters and prototype chain
// This is safe to call from native functions and will trigger property getters/throw exceptions
func (vm *VM) GetSymbolPropertyWithGetter(obj Value, symbol Value) (Value, bool, error) {
	if symbol.Type() != TypeSymbol {
		return Undefined, false, nil
	}
	symKey := NewSymbolKey(symbol)

	// Handle TypeRegExp - look up in RegExp.prototype
	if obj.Type() == TypeRegExp {
		// RegExp values check their Properties first, then RegExp.prototype
		regexpObj := obj.AsRegExpObject()
		if regexpObj.Properties != nil {
			// Check for accessor (getter) first
			if getter, _, _, _, ok := regexpObj.Properties.GetOwnAccessorByKey(symKey); ok {
				if getter.Type() != TypeUndefined {
					result, err := vm.Call(getter, obj, nil)
					if err != nil {
						if ee, ok := err.(ExceptionError); ok {
							vm.throwException(ee.GetExceptionValue())
						}
						return Undefined, false, err
					}
					return result, true, nil
				}
				return Undefined, true, nil
			}
			// Check for regular property
			if v, ok := regexpObj.Properties.GetOwnByKey(symKey); ok {
				return v, true, nil
			}
		}
		// Check RegExp.prototype
		if vm.RegExpPrototype != Undefined && vm.RegExpPrototype.Type() == TypeObject {
			proto := vm.RegExpPrototype.AsPlainObject()
			// Check for accessor (getter) first
			if getter, _, _, _, ok := proto.GetOwnAccessorByKey(symKey); ok {
				if getter.Type() != TypeUndefined {
					result, err := vm.Call(getter, obj, nil)
					if err != nil {
						if ee, ok := err.(ExceptionError); ok {
							vm.throwException(ee.GetExceptionValue())
						}
						return Undefined, false, err
					}
					return result, true, nil
				}
				return Undefined, true, nil
			}
			// Check for regular property
			if v, ok := proto.GetOwnByKey(symKey); ok {
				return v, true, nil
			}
			// Also check Object.prototype via the prototype chain
			protoVal := proto.GetPrototype()
			if protoVal.Type() == TypeObject {
				objProto := protoVal.AsPlainObject()
				// Check for accessor (getter) first
				if getter, _, _, _, ok := objProto.GetOwnAccessorByKey(symKey); ok {
					if getter.Type() != TypeUndefined {
						result, err := vm.Call(getter, obj, nil)
						if err != nil {
							if ee, ok := err.(ExceptionError); ok {
								vm.throwException(ee.GetExceptionValue())
							}
							return Undefined, false, err
						}
						return result, true, nil
					}
					return Undefined, true, nil
				}
				// Check for regular property
				if v, ok := objProto.GetOwnByKey(symKey); ok {
					return v, true, nil
				}
			}
		}
		return Undefined, false, nil
	}

	if obj.Type() == TypeObject {
		po := obj.AsPlainObject()
		if po == nil {
			return Undefined, false, nil
		}

		// Check for accessor (getter) first - need to check own property and prototype chain
		cur := po
		for cur != nil {
			// Check if it's an accessor (getter)
			if getter, _, _, _, ok := cur.GetOwnAccessorByKey(symKey); ok {
				if getter.Type() != TypeUndefined {
					// Call the getter with this=obj (original object, not prototype)
					result, err := vm.Call(getter, obj, nil)
					if err != nil {
						// If the getter threw an exception, throw it as a VM exception
						if ee, ok := err.(ExceptionError); ok {
							vm.throwException(ee.GetExceptionValue())
						}
						return Undefined, false, err
					}
					return result, true, nil
				}
				// Accessor exists but no getter - return undefined
				return Undefined, true, nil
			}
			// Check for regular property
			if v, ok := cur.GetOwnByKey(symKey); ok {
				return v, true, nil
			}
			// Walk prototype chain
			protoVal := cur.GetPrototype()
			if protoVal.Type() != TypeObject {
				break
			}
			cur = protoVal.AsPlainObject()
		}
		return Undefined, false, nil
	}

	// Callables (constructors above all) resolve through the shared
	// symbol-key walk: own properties, then the real [[Prototype]] chain,
	// invoking any accessor's getter with this = obj. Without this case the
	// function kinds fell straight through to the "non-objects" return below,
	// so native callers such as TypedArraySpeciesCreate read every
	// constructor's [[Symbol.species]] as absent and silently used the default
	// constructor - invisible from the bytecode read path, which has its own
	// implementation in opGetPropSymbol.
	switch obj.Type() {
	case TypeFunction, TypeClosure, TypeNativeFunction, TypeNativeFunctionWithProps, TypeBoundFunction:
		return vm.lookupSymbolWithReceiver(obj, symKey, obj)
	}

	// For non-objects, just return undefined
	return Undefined, false, nil
}

// GetSymbolProperty gets a symbol property from an object value, properly handling prototype chain
// This is safe to call from native functions
func (vm *VM) GetSymbolProperty(obj Value, symbol Value) (Value, bool) {
	if symbol.Type() != TypeSymbol {
		return Undefined, false
	}
	symKey := NewSymbolKey(symbol)

	// Handle array type
	if obj.Type() == TypeArray {
		arr := obj.AsArray()
		if arr != nil {
			// First check array's own symbol properties (e.g., overridden Symbol.iterator)
			sym := symbol.AsSymbolObject()
			if sym != nil {
				if v, ok := arr.GetSymbolProp(sym); ok {
					return v, true
				}
			}
			// Fall back to ArrayPrototype for inherited symbol properties
			if vm.ArrayPrototype.Type() != TypeUndefined {
				proto := vm.ArrayPrototype.AsPlainObject()
				if proto != nil {
					if v, ok := proto.GetOwnByKey(symKey); ok {
						return v, true
					}
				}
			}
		}
		return Undefined, false
	}

	// Handle generator type
	if obj.Type() == TypeGenerator {
		if vm.GeneratorPrototype.Type() != TypeUndefined {
			proto := vm.GeneratorPrototype.AsPlainObject()
			if proto != nil {
				if v, ok := proto.GetOwnByKey(symKey); ok {
					return v, true
				}
			}
		}
		return Undefined, false
	}

	// Handle async generator type
	if obj.Type() == TypeAsyncGenerator {
		if vm.AsyncGeneratorPrototype.Type() != TypeUndefined {
			proto := vm.AsyncGeneratorPrototype.AsPlainObject()
			if proto != nil {
				if v, ok := proto.GetOwnByKey(symKey); ok {
					return v, true
				}
			}
		}
		return Undefined, false
	}

	// Handle Set type
	if obj.Type() == TypeSet {
		if vm.SetPrototype.Type() != TypeUndefined {
			proto := vm.SetPrototype.AsPlainObject()
			if proto != nil {
				// Walk prototype chain
				for cur := proto; cur != nil; {
					if v, ok := cur.GetOwnByKey(symKey); ok {
						return v, true
					}
					protoVal := cur.GetPrototype()
					if protoVal.Type() != TypeObject {
						break
					}
					cur = protoVal.AsPlainObject()
				}
			}
		}
		return Undefined, false
	}

	// Handle Map type
	if obj.Type() == TypeMap {
		if vm.MapPrototype.Type() != TypeUndefined {
			proto := vm.MapPrototype.AsPlainObject()
			if proto != nil {
				// Walk prototype chain
				for cur := proto; cur != nil; {
					if v, ok := cur.GetOwnByKey(symKey); ok {
						return v, true
					}
					protoVal := cur.GetPrototype()
					if protoVal.Type() != TypeObject {
						break
					}
					cur = protoVal.AsPlainObject()
				}
			}
		}
		return Undefined, false
	}

	// Handle plain object type
	if obj.Type() == TypeObject {
		po := obj.AsPlainObject()
		if po == nil {
			return Undefined, false
		}

		// Check own property first
		if v, ok := po.GetOwnByKey(symKey); ok {
			return v, true
		}

		// Walk prototype chain
		cur := po
		for cur != nil {
			protoVal := cur.GetPrototype()
			if protoVal.Type() != TypeObject {
				break
			}
			proto := protoVal.AsPlainObject()
			if proto == nil {
				break
			}
			if v, ok := proto.GetOwnByKey(symKey); ok {
				return v, true
			}
			cur = proto
		}
		return Undefined, false
	}

	return Undefined, false
}

// Call is a unified function calling interface that handles all function types properly
// This replaces the complex web of CallFunctionDirectly, CallUserFunction, etc.
// getArgsBuf returns a reusable []Value of length n (n <= 4) for callback
// arguments, popping from the pool or allocating a cap-4 buffer. Reentries nest
// strictly LIFO, so an in-use buffer is never handed out twice.
func (vm *VM) getArgsBuf(n int) []Value {
	if k := len(vm.argsBufPool); k > 0 {
		b := vm.argsBufPool[k-1]
		vm.argsBufPool = vm.argsBufPool[:k-1]
		return b[:n]
	}
	return make([]Value, n, 4)
}

// putArgsBuf returns a buffer to the pool, clearing it so pooled buffers don't
// retain argument values.
func (vm *VM) putArgsBuf(b []Value) {
	b = b[:cap(b)]
	for i := range b {
		b[i] = Undefined
	}
	vm.argsBufPool = append(vm.argsBufPool, b)
}

// CallArgs2/3/4 invoke fn with the given arguments through a pooled buffer,
// avoiding the per-call []Value allocation that every array callback site paid.
// Safe because Call copies the arguments out before returning (into the callee's
// registers and, if accessed, a copied arguments object) - nothing retains the
// slice past the call.
func (vm *VM) CallArgs2(fn, thisValue, a0, a1 Value) (Value, error) {
	b := vm.getArgsBuf(2)
	b[0], b[1] = a0, a1
	r, err := vm.Call(fn, thisValue, b)
	vm.putArgsBuf(b)
	return r, err
}

func (vm *VM) CallArgs3(fn, thisValue, a0, a1, a2 Value) (Value, error) {
	b := vm.getArgsBuf(3)
	b[0], b[1], b[2] = a0, a1, a2
	r, err := vm.Call(fn, thisValue, b)
	vm.putArgsBuf(b)
	return r, err
}

func (vm *VM) CallArgs4(fn, thisValue, a0, a1, a2, a3 Value) (Value, error) {
	b := vm.getArgsBuf(4)
	b[0], b[1], b[2], b[3] = a0, a1, a2, a3
	r, err := vm.Call(fn, thisValue, b)
	vm.putArgsBuf(b)
	return r, err
}

// enterOrdinaryNativeCall establishes the state a native callee sees for an
// ordinary [[Call]], and returns the function that restores it.
//
// The point is what it *clears*: currentNewTarget and inConstructorCall. A
// [[Call]] has no new.target - only [[Construct]] carries one - but vm.Call
// used to save and set only currentThis, so both of those leaked in from
// whatever was on the Go stack above. Called from inside a native constructor
// (which the interpreter runs with currentNewTarget set to the constructor and
// inConstructorCall true), every nested vm.Call inherited them, and any native
// callee reading GetNewTarget()/IsConstructorCall() silently behaved as though
// it were being constructed by the *outer* constructor.
//
// That is how `new String(o)`, for an object with no callable toString/valueOf,
// came to throw a String rather than a TypeError (#154): ToPrimitive's final
// step calls vm.ThrowTypeError, which builds its instance with
// vm.Call(TypeError, ...) - and that call resolved its prototype from
// new.target, which was still the String constructor. The thrown object had
// name "TypeError" and the right message, but String.prototype for its
// [[Prototype]], so `e instanceof TypeError` was false.
//
// ConstructWithNewTarget is the deliberate counterpart: it saves the same three
// fields and *sets* them, because it really is a [[Construct]].
func (vm *VM) enterOrdinaryNativeCall(thisValue Value) func() {
	prevThis := vm.currentThis
	vm.currentThis = thisValue
	restoreCallState := vm.enterOrdinaryCall()
	return func() {
		restoreCallState()
		vm.currentThis = prevThis
	}
}

// enterOrdinaryCall clears the construct context for the duration of an
// ordinary [[Call]] and returns the function that restores it. See
// enterOrdinaryNativeCall for why.
//
// Both of vm.Call's callee kinds need this, for different reasons. A native
// callee reads these fields itself. A *user-JS* callee does not - new.target
// inside a function is per-frame and was already correct - but the bytecode it
// runs plain-calls natives through the interpreter, and the interpreter's
// plain-call path does not touch these fields (only its four construct sites
// do). So a native constructor that calls back into user JS used to leave its
// construct context visible to every native that user JS called:
//
//	new String({ toString() { return typeof String("a"); } })
//
// reported "object" - the inner String("a"), an ordinary call, saw
// inConstructorCall still true from the outer `new String` and returned a
// wrapper object instead of a primitive.
//
// executeUserFunctionWithNewTarget is the construct counterpart and is
// deliberately left alone: it sets up new.target rather than clearing it.
func (vm *VM) enterOrdinaryCall() func() {
	prevNewTarget := vm.currentNewTarget
	prevInConstructorCall := vm.inConstructorCall
	vm.currentNewTarget = Undefined
	vm.inConstructorCall = false
	return func() {
		vm.currentNewTarget = prevNewTarget
		vm.inConstructorCall = prevInConstructorCall
	}
}

func (vm *VM) Call(fn Value, thisValue Value, args []Value) (Value, error) {
	switch fn.Type() {
	case TypeNativeFunction:
		// For native functions, call directly with proper 'this' context
		nativeFunc := AsNativeFunction(fn)
		restore := vm.enterOrdinaryNativeCall(thisValue)
		defer restore()
		return nativeFunc.Fn(args)

	case TypeNativeFunctionWithProps:
		// Handle native function with properties
		nativeFuncWithProps := fn.AsNativeFunctionWithProps()
		restore := vm.enterOrdinaryNativeCall(thisValue)
		defer restore()
		return nativeFuncWithProps.Fn(args)

	case TypeClosure, TypeFunction:
		// For user-defined functions, use the sentinel safe execution path which
		// integrates correctly with the interpreter loop and ensures exceptions
		// are surfaced as ExceptionError without corrupting VM state.
		return vm.executeUserFunctionSafe(fn, thisValue, args)

	case TypeBoundFunction:
		// Handle bound functions by delegating to the original function
		boundFunc := fn.AsBoundFunction()
		// Combine partial args with call-time args
		finalArgs := make([]Value, len(boundFunc.PartialArgs)+len(args))
		copy(finalArgs, boundFunc.PartialArgs)
		copy(finalArgs[len(boundFunc.PartialArgs):], args)
		// Use the bound 'this' value
		return vm.Call(boundFunc.OriginalFunction, boundFunc.BoundThis, finalArgs)

	case TypeProxy:
		// Handle Proxy with apply trap
		proxy := fn.AsProxy()
		if proxy.Revoked {
			return Undefined, vm.NewTypeError("Cannot perform 'apply' on a proxy that has been revoked")
		}

		// Get the apply trap from handler. GetMethod(handler, "apply") per
		// spec: an inherited trap counts, not just an own one - proxyGetTrap
		// (not a bare handler.AsPlainObject().GetOwn("apply")) for the same
		// reason documented on its own definition.
		handler := proxy.Handler()
		applyTrap, hasApplyTrap := proxyGetTrap(handler, "apply")

		// Check for apply trap
		if hasApplyTrap && applyTrap.Type() != TypeUndefined && applyTrap.Type() != TypeNull {
			// Validate trap is callable
			if !applyTrap.IsCallable() {
				return Undefined, vm.NewTypeError("'apply' on proxy: trap is not a function")
			}

			// Convert args to array for trap call
			argsArray := NewArray()
			arrObj := argsArray.AsArray()
			for _, arg := range args {
				arrObj.Append(arg)
			}

			// Call handler.apply(target, thisArg, argumentsList)
			trapArgs := []Value{proxy.Target(), thisValue, argsArray}
			return vm.Call(applyTrap, handler, trapArgs)
		}

		// No apply trap, delegate to target
		return vm.Call(proxy.Target(), thisValue, args)

	default:
		return Undefined, fmt.Errorf("cannot call non-function value of type %v", fn.Type())
	}
}

// IsConstructor checks if a value can be used as a constructor
func (vm *VM) IsConstructor(val Value) bool {
	switch val.Type() {
	case TypeNativeFunction:
		return val.AsNativeFunction().IsConstructor
	case TypeNativeFunctionWithProps:
		return val.AsNativeFunctionWithProps().IsConstructor
	case TypeClosure:
		cl := val.AsClosure()
		// Arrow functions and async (non-generator) functions cannot be constructors
		return !cl.Fn.IsArrowFunction && !(cl.Fn.IsAsync && !cl.Fn.IsGenerator)
	case TypeFunction:
		fn := val.AsFunction()
		return !fn.IsArrowFunction && !(fn.IsAsync && !fn.IsGenerator)
	case TypeBoundFunction:
		// Bound functions inherit constructability from the original
		return vm.IsConstructor(val.AsBoundFunction().OriginalFunction)
	default:
		return false
	}
}

// Construct calls a constructor function with the given arguments, similar to 'new Constructor(args)'
// Per ECMAScript spec, Construct(F, args) defaults newTarget to F and delegates to [[Construct]].
func (vm *VM) Construct(constructor Value, args []Value) (Value, error) {
	return vm.ConstructWithNewTarget(constructor, args, constructor)
}

// ConstructWithNewTarget calls a constructor function with a custom new.target value
// This is used by Reflect.construct to support the third argument
func (vm *VM) ConstructWithNewTarget(constructor Value, args []Value, newTarget Value) (Value, error) {
	if !constructor.IsCallable() {
		return Undefined, fmt.Errorf("%s is not a constructor", constructor.TypeName())
	}

	switch constructor.Type() {
	case TypeNativeFunction:
		nf := constructor.AsNativeFunction()
		if !nf.IsConstructor {
			return Undefined, fmt.Errorf("%s is not a constructor", nf.Name)
		}
		// For native constructors, call directly - they handle creating the object
		// Set currentNewTarget and inConstructorCall so native constructors can detect constructor calls
		prevThis := vm.currentThis
		prevNewTarget := vm.currentNewTarget
		prevInConstructorCall := vm.inConstructorCall
		vm.currentThis = Undefined
		vm.currentNewTarget = newTarget
		vm.inConstructorCall = true
		defer func() {
			vm.currentThis = prevThis
			vm.currentNewTarget = prevNewTarget
			vm.inConstructorCall = prevInConstructorCall
		}()
		return nf.Fn(args)

	case TypeNativeFunctionWithProps:
		nfp := constructor.AsNativeFunctionWithProps()
		if !nfp.IsConstructor {
			return Undefined, fmt.Errorf("%s is not a constructor", nfp.Name)
		}
		// Set currentNewTarget and inConstructorCall so native constructors can detect constructor calls
		prevThis := vm.currentThis
		prevNewTarget := vm.currentNewTarget
		prevInConstructorCall := vm.inConstructorCall
		vm.currentThis = Undefined
		vm.currentNewTarget = newTarget
		vm.inConstructorCall = true
		defer func() {
			vm.currentThis = prevThis
			vm.currentNewTarget = prevNewTarget
			vm.inConstructorCall = prevInConstructorCall
		}()
		return nfp.Fn(args)

	case TypeClosure, TypeFunction:
		// For user-defined constructors
		var fn *FunctionObject
		if constructor.Type() == TypeClosure {
			fn = constructor.AsClosure().Fn
		} else {
			fn = constructor.AsFunction()
		}

		// Check if constructable
		if fn.IsArrowFunction || (fn.IsAsync && !fn.IsGenerator) {
			return Undefined, fmt.Errorf("function is not a constructor")
		}

		// Get prototype from newTarget (not constructor)
		// Per ECMAScript, the prototype is determined by newTarget
		//
		// This used to unconditionally unwrap TypeClosure to its
		// underlying, SHARED *FunctionObject (newTarget.AsClosure().Fn)
		// and call GetOrCreatePrototypeWithVM on that directly - which
		// creates/reads "prototype" on the function's own table, not the
		// closure INSTANCE's table. But `someClosure.prototype = x`
		// (op_setprop.go's TypeClosure case) writes into
		// closure.Properties, a per-closure-instance side table that
		// shadows the shared Fn's - exactly the same shadowing
		// TypeFunction/TypeClosure's ordinary property-read path
		// (getPropertyWithReceiver above) already respects. So
		// Reflect.construct(Ctor, args, newTarget) with an explicit
		// newTarget whose .prototype had been reassigned (or a class,
		// which compiles to TypeClosure) silently used newTarget's
		// stale, unshadowed default prototype instead of the real one:
		//
		//   function C(a) { this.a = a; }
		//   function D() {}
		//   D.prototype = { fromD: true };
		//   Object.getPrototypeOf(Reflect.construct(C, [1], D)) === D.prototype; // before: false - Node: true
		//
		// Fixed by using ClosureObject.GetPrototypeWithVM (pkg/vm/function.go),
		// which already gets this shadowing right - it's the same method
		// OpValidateSuperclass (`class X extends Y`, pkg/vm/vm.go) already
		// uses for the identical purpose. TypeNativeFunction/
		// TypeNativeFunctionWithProps/TypeBoundFunction newTargets (a
		// native or bound constructor) had the same bug via the same
		// "fall back to constructor's own prototype, not newTarget's"
		// path - handled by GetPrototypeFromConstructor below directly
		// (their own "prototype" is a real, non-lazily-created property,
		// so vm.GetProperty - which GetPrototypeFromConstructor calls -
		// already reads it correctly for all three, unlike
		// TypeFunction/TypeClosure's synthesized-on-first-access one).
		var prototype Value
		switch newTarget.Type() {
		case TypeClosure:
			prototype = newTarget.AsClosure().GetPrototypeWithVM(vm)
		case TypeFunction:
			prototype = newTarget.AsFunction().GetOrCreatePrototypeWithVM(vm)
		default:
			var gpfcErr error
			prototype, gpfcErr = vm.GetPrototypeFromConstructor(newTarget, "%ObjectPrototype%")
			if gpfcErr != nil {
				return Undefined, gpfcErr
			}
		}

		// ECMAScript spec 9.1.14 GetPrototypeFromConstructor:
		// If prototype is not an object, use the realm of newTarget's intrinsic default
		if !prototype.IsObject() && !prototype.IsCallable() {
			var gpfcErr error
			prototype, gpfcErr = vm.GetPrototypeFromConstructor(newTarget, "%ObjectPrototype%")
			if gpfcErr != nil {
				return Undefined, gpfcErr
			}
		}

		// For derived constructors, 'this' is in TDZ until super() is called
		// We don't create an object beforehand - super() will create it
		var newObj Value
		if fn.IsDerivedConstructor {
			// For derived constructors, pass Uninitialized as this (TDZ sentinel)
			// super() will create the object with the correct prototype
			newObj = Uninitialized
		} else {
			// For base constructors, create the object now
			newObj = NewObject(prototype)
		}

		// Use executeUserFunctionWithNewTarget for proper new.target handling
		result, err := vm.executeUserFunctionWithNewTarget(constructor, newObj, args, newTarget, fn.IsDerivedConstructor)
		if err != nil {
			return Undefined, err
		}

		// For derived constructors, result should be the 'this' that was set by super()
		// (handled by sentinel frame constructor semantics in OpReturn)
		// For base constructors, result may be the explicit return or we use newObj
		if result.IsObject() {
			return result, nil
		}
		// For non-object returns (including undefined), use newObj for base constructors
		// For derived constructors, newObj is Undefined and result should have been
		// the this value from super() - if we get here with undefined, super wasn't called
		if !fn.IsDerivedConstructor {
			return newObj, nil
		}
		// For derived constructor returning undefined, this is valid if super() wasn't called
		// (which would throw ReferenceError), so we shouldn't reach here in normal flow
		return result, nil

	case TypeBoundFunction:
		bf := constructor.AsBoundFunction()
		// Combine partial args with call-time args
		finalArgs := make([]Value, len(bf.PartialArgs)+len(args))
		copy(finalArgs, bf.PartialArgs)
		copy(finalArgs[len(bf.PartialArgs):], args)
		// Per ECMAScript §10.4.1.2 step 5:
		// If SameValue(F, newTarget) is true, let newTarget be target.
		if constructor.StrictlyEquals(newTarget) {
			newTarget = bf.OriginalFunction
		}
		return vm.ConstructWithNewTarget(bf.OriginalFunction, finalArgs, newTarget)

	default:
		return Undefined, fmt.Errorf("%s is not a constructor", constructor.TypeName())
	}
}

// executeUserFunctionWithNewTarget executes a user function with constructor semantics and custom new.target
// getSentinelReg returns a 1-element register slice to serve as a native->JS
// reentry's sentinel-frame result holder, reusing a pooled buffer when one is
// free (reentries nest strictly LIFO, so the pool recycles).
func (vm *VM) getSentinelReg() []Value {
	if n := len(vm.sentinelRegPool); n > 0 {
		r := vm.sentinelRegPool[n-1]
		vm.sentinelRegPool = vm.sentinelRegPool[:n-1]
		return r
	}
	return make([]Value, 1)
}

// putSentinelReg returns a sentinel register slice to the pool, clearing the
// slot so the pooled buffer doesn't keep the last result value alive.
func (vm *VM) putSentinelReg(r []Value) {
	r[0] = Undefined
	vm.sentinelRegPool = append(vm.sentinelRegPool, r)
}

// truncateFramesTo drops every frame from entryCount up to the current
// vm.frameCount and resets vm.unwindingCrossedNative. Used by
// executeUserFunctionSafe/executeUserFunctionWithNewTarget's error paths to
// fully undo a call once its exception has been taken over as a Go error:
// unwindException deliberately leaves the frame(s) it stopped at (a
// sentinel, or the isDirectCall closure frame one level in) unpopped, so
// without this the frame slot could be mistaken for a still-live boundary by
// a later, unrelated throw (see the callers' comments).
//
// Resetting the flag here matters just as much as dropping the frame: this
// call's boundary has now been fully closed out (converted to a Go error,
// its frame gone), so whatever throws next - a sibling call made by the
// same native caller, or a re-throw of this very error one level further
// out - is a *different* boundary and deserves its own first chance to
// stop, not to inherit "already crossed" from a boundary that no longer
// exists. Without this reset, a re-throw from an outer native caller (e.g.
// executeGeneratorPrologue re-injecting this error into the calling
// frame's bytecode) would find crossedNative already true and blow
// straight through its own boundary too.
//
// This function itself does NOT reclaim vm.regDir's cursor for the dropped
// frame(s)' register windows - only frameCount and upvalues. unwindException's
// own frame-popping loop has the same gap: it decrements vm.frameCount
// without touching vm.regDir for every frame it walks past, because register
// space during a multi-boundary unwind is only reclaimed in bulk relative to
// the frame the dispatch loop eventually resumes at, not frame-by-frame
// while walking past intermediate native boundaries - see issue #61, which
// remains open for that general case.
//
// executeUserFunctionSafe/executeUserFunctionWithNewTarget are a narrower,
// fully-solvable case, however: by the time either calls this, it already
// knows exactly which single register-directory mark to restore to - the
// cursor as it was before it pushed its own sentinel frame (their own
// `entryRegMark`, captured with vm.regDir.mark() before that push, since a
// sentinel frame never itself goes through regDir.push). Both callers do
// vm.regDir.popTo(entryRegMark) themselves, right after every call to this
// that follows an actual exception (vm.run() returning InterpretRuntimeError,
// or an inline exception raised before any frame switch) - the only paths
// that can genuinely have registers left to reclaim. Their plain
// call-completed-without-switching-frames and prepareCall-returned-an-error
// exits deliberately do NOT call popTo even though the cursor is expected to
// already equal entryRegMark there too (see the comments at those call
// sites): asserting that via popTo on a path with nothing to reclaim would
// silently paper over the invariant no longer holding, converting a loud
// checkRegWindowRelease panic into silent corruption instead - worse than
// leaving it caught. Left as this function's own caller's responsibility
// rather than folded in here because the *other*
// class of caller (a bare unwindException walk with no single call frame to
// scope a mark to) has no equivalent fixed point to restore to - see #414
// for the fix and the leak it closes, and the comment above for why #61
// itself is a different, still-open case.
func (vm *VM) truncateFramesTo(entryCount int) {
	// unwindException leaves the native-boundary frame it stopped at on the
	// stack without popping it; dropping it here retires it for good, so its
	// open upvalues must be closed like any other returning frame's (see the
	// matching note in unwindException's pop loop).
	for i := vm.frameCount - 1; i >= entryCount && i >= 0; i-- {
		if f := &vm.frames[i]; f.openUpvalues != nil {
			vm.closeFrameUpvalues(f)
		}
	}
	vm.frameCount = entryCount
	vm.unwindingCrossedNative = false
}

func (vm *VM) executeUserFunctionWithNewTarget(fn Value, thisValue Value, args []Value, newTarget Value, isDerivedConstructor bool) (Value, error) {
	// Clear stale unwinding state
	if vm.unwinding && vm.currentException == Null {
		vm.unwinding = false
		vm.unwindingCrossedNative = false
	}

	// See the matching comment in executeUserFunctionSafe: remember our entry
	// depth so the error paths below can drop any frame(s) unwinding stopped
	// at without popping, once we've taken ownership of the exception as a
	// Go error.
	frameCountAtEntry := vm.frameCount
	// truncateFramesTo (see its own doc comment, #414) drops frames but
	// deliberately doesn't touch the register directory's cursor - that's
	// this call's own register space to reclaim, the same way Interpret's
	// nested-call error path already does for itself (vm.go, "entryMark").
	// Captured before the sentinel frame push below: sentinel frames reuse a
	// pooled 1-element slice rather than going through regDir.push, so the
	// cursor here is exactly where it will still be once every frame this
	// call pushed - sentinel included - is gone.
	entryRegMark := vm.regDir.mark()

	// Set up the caller context (pooled 1-element result holder, not a per-call alloc)
	callerRegisters := vm.getSentinelReg()
	defer vm.putSentinelReg(callerRegisters)
	destReg := byte(0)
	callerIP := 0

	// Add a sentinel frame
	sentinelFrame := &vm.frames[vm.frameCount]
	sentinelFrame.isSentinelFrame = true
	sentinelFrame.closure = nil
	sentinelFrame.openUpvalues = nil // Never captures; clear any stale head from prior slot use
	sentinelFrame.targetRegister = destReg
	sentinelFrame.registers = callerRegisters
	vm.frameCount++

	// Set constructor call flag
	prevInConstructorCall := vm.inConstructorCall
	vm.inConstructorCall = true
	defer func() { vm.inConstructorCall = prevInConstructorCall }()

	// Use prepareCall to set up the function call
	// For derived constructors, this is in TDZ until super() is called
	effectiveThis := thisValue
	if isDerivedConstructor {
		effectiveThis = Uninitialized
	}

	shouldSwitch, err := vm.prepareCall(fn, effectiveThis, args, destReg, callerRegisters, callerIP)
	if err != nil {
		// No vm.regDir.popTo(entryRegMark) needed here (unlike the exception
		// paths below): prepareCall only ever returns a non-nil err before it
		// has pushed a register window for the callee (a frame-limit check,
		// or regDir.push itself failing - which per its own doc comment
		// never mutates the cursor on failure), so the cursor is already
		// exactly entryRegMark. Deliberately not added defensively either -
		// popTo's trim() isn't free, and silently popping here if that
		// invariant ever changed would convert a loud, catchable
		// checkRegWindowRelease panic into silent register-space corruption
		// instead (#414).
		vm.frameCount--
		return Undefined, err
	}

	if !shouldSwitch {
		// Same reasoning as the err != nil case just above: a native
		// function ran synchronously and returned normally without pushing
		// anything here itself. If it made its own nested vm.Call that threw
		// and got absorbed, that nested call's own error path (this
		// function or executeUserFunctionSafe) already restored its own
		// window before returning to us - so the cursor is already back to
		// entryRegMark by the time we get here regardless.
		vm.frameCount--
		return callerRegisters[destReg], nil
	}

	// Set constructor-specific frame properties
	if vm.frameCount > 1 {
		frame := &vm.frames[vm.frameCount-1]
		frame.isDirectCall = true
		frame.isConstructorCall = true
		frame.newTargetValue = newTarget
		// For derived constructors, this is in TDZ until super() is called
		if isDerivedConstructor {
			frame.thisValue = Uninitialized
		} else {
			frame.thisValue = thisValue
		}
	}

	// Execute the VM run loop
	status, result := vm.run()

	if status == InterpretRuntimeError {
		if vm.unwinding && vm.currentException != Null {
			ex := vm.currentException
			// Clear vm.unwinding along with currentException when handing the
			// exception off as a Go error - see the matching, longer comment
			// in executeUserFunctionSafe (#142). Leaving vm.unwinding=true
			// here left the exact same stale-state trap for `new`-constructor
			// calls that absorb a Go error (e.g. Reflect.construct-style
			// wrappers) without re-throwing.
			vm.currentException = Null
			vm.unwinding = false
			vm.truncateFramesTo(frameCountAtEntry)
			vm.regDir.popTo(entryRegMark)
			return Undefined, exceptionError{exception: ex}
		}
		return Undefined, fmt.Errorf("runtime error during constructor execution")
	}

	if vm.unwinding && vm.currentException != Null {
		ex := vm.currentException
		vm.currentException = Null
		vm.unwinding = false
		vm.truncateFramesTo(frameCountAtEntry)
		vm.regDir.popTo(entryRegMark)
		return Undefined, exceptionError{exception: ex}
	}

	return result, nil
}

// lastRecordedErrorMessage returns the message of the most recently recorded
// internal diagnostic in vm.errors, without consuming it. Returns "" if
// nothing was recorded.
//
// vm.runtimeError() - used for internal-invariant failures (a stack overflow
// mid-construction, a corrupted register/constant index, a recovered Go
// panic) - records the real diagnostic here and returns InterpretRuntimeError
// *without* ever setting vm.unwinding/vm.currentException, because there is
// no JS-level exception *value* to hand back (see runtimeError's callers in
// vm.go). A caller of vm.run() that only checks
// `vm.unwinding && vm.currentException != Null` to decide it got a real
// exception therefore has nothing to report for this case and, before this
// helper existed, fell back to a fixed generic string - discarding the one
// piece of real information the VM actually recorded (#130).
//
// Deliberately a peek, not a pop (unlike the similar-looking vm.errors
// fallback in the OpDynamicImport handling in vm.go, which *does* consume the
// entry because it owns turning this into a terminal promise rejection):
// executeUserFunctionSafe is not always the terminal consumer. When this
// runtimeError fired inside a callback invoked from bytecode that is itself
// running under the driver's own top-level Interpret(), popping here would
// remove the entry Interpret() is still going to report from vm.errors
// wholesale once the (re-wrapped, re-thrown) exception finishes propagating -
// verified by hand: popping left a real top-level failure printing as a
// multi-thousand-line garbage dump instead of the one-line diagnostic it
// prints today.
func (vm *VM) lastRecordedErrorMessage() string {
	if len(vm.errors) == 0 {
		return ""
	}
	return vm.errors[len(vm.errors)-1].Error()
}

// executeUserFunctionSafe executes a user function from a native function using sentinel frames
// This allows proper nested calls without infinite recursion
func (vm *VM) executeUserFunctionSafe(fn Value, thisValue Value, args []Value) (Value, error) {
	// This is an ordinary [[Call]], so nothing it reaches - including natives
	// its bytecode calls - may see an enclosing construct context.
	defer vm.enterOrdinaryCall()()

	// If unwinding flags are set but currentException is Null, it means the exception was
	// already handed off to native code as a Go error. Native code either:
	// 1. Handled it and is making a new call (not re-throwing) - clear the flags
	// 2. Is about to re-throw it - but then it will call throwException() which will set them again
	// So we can safely clear stale unwinding state here at the start of a new bytecode execution.
	if vm.unwinding && vm.currentException == Null {
		if debugExceptions {
			fmt.Println("[DEBUG executeUserFunctionSafe] Clearing stale unwinding state (exception was handed to native)")
		}
		vm.unwinding = false
		vm.unwindingCrossedNative = false
	}

	// Remember the frame depth this call started at so the error paths below
	// can fully unwind back to it. When unwindException stops at our own
	// isDirectCall boundary (or, one level further out via a nested native
	// call, at our sentinel), it deliberately leaves that frame on the stack
	// instead of popping it. We hand the exception to our native caller as a
	// Go error here, taking ownership of it - so from the VM's perspective
	// this call is over and every frame it pushed (the sentinel, and the
	// direct-call frame if unwinding stopped there without popping it) must
	// go with it. Left in place, a stale frame would sit on vm.frames and
	// get mistaken for a fresh, still-live native boundary by a *later*,
	// unrelated throw during a subsequent call made by the same native
	// caller (e.g. DisposableStack running several dispose() callbacks in
	// sequence, each via its own executeUserFunctionSafe call).
	frameCountAtEntry := vm.frameCount
	// truncateFramesTo (see its own doc comment, #414) drops frames but
	// deliberately doesn't touch the register directory's cursor - that's
	// this call's own register space to reclaim, the same way Interpret's
	// nested-call error path already does for itself (vm.go, "entryMark").
	// Captured before the sentinel frame push below: sentinel frames reuse a
	// pooled 1-element slice rather than going through regDir.push, so the
	// cursor here is exactly where it will still be once every frame this
	// call pushed - sentinel included - is gone. Without this, a promise
	// reaction whose handler throws (triggerPromiseReactions in promise.go
	// absorbs the resulting Go error from exactly this call) leaked the
	// callee's whole register window into whatever frame is resumed next -
	// harmless in isolation, but flagged by checkRegWindowRelease under
	// StrictRegWindowChecks the next time that frame (often the top-level
	// script itself, via top-level await) finally pops (#414).
	entryRegMark := vm.regDir.mark()

	// Remember whether we were already unwinding *with* a live exception at
	// entry (the cleanup just above only scrubs the currentException==Null
	// case, deliberately leaving this one alone - a native caller mid-
	// unwind, forwarding or absorbing an exception, is expected to be able
	// to make further vm.Call calls). The shouldSwitch=false branch below
	// must only treat vm.unwinding as *this* call's own doing if it became
	// true during it - otherwise a prepareCall that runs a native function
	// to completion, perfectly successfully, while an unrelated exception
	// from before this call is still sitting on the VM, would get its
	// legitimate result thrown away and replaced with that stale exception.
	unwindingAtEntry := vm.unwinding && vm.currentException != Null

	// Set up the caller context first (pooled 1-element result holder)
	callerRegisters := vm.getSentinelReg()
	defer vm.putSentinelReg(callerRegisters)
	destReg := byte(0)
	callerIP := 0

	// Bail out before touching vm.frames if the call stack is already full.
	// This path is reentrant - a native Go call into JS, e.g. vm.Call from
	// ThrowTypeError/ThrowRangeError/etc. constructing an exception instance,
	// or any other builtin invoking a callback - and unlike the bytecode-level
	// call sites (OpCall/OpSpreadNew), which already guard against
	// overflowing vm.frames before pushing a new frame, this one used to
	// index straight past the end of the fixed-size frames array once
	// frameCount reached len(vm.frames), panicking with a raw "index out of
	// range" instead of raising a catchable exception (#231 - reassigning
	// globalThis.TypeError to a class made every internal ThrowTypeError call
	// recurse into itself indefinitely through exactly this path). Build the
	// exception directly rather than going through vm.ThrowRangeError/vm.Call
	// - the stack is exactly as full as it was a moment ago, so calling back
	// into a (possibly user-reassigned) RangeError constructor here would hit
	// this very guard again.
	if vm.frameCount >= len(vm.frames) {
		return Undefined, exceptionError{exception: vm.newStackOverflowError()}
	}

	// Add a sentinel frame that will cause vm.run() to return when it hits this frame
	sentinelFrame := &vm.frames[vm.frameCount]
	sentinelFrame.isSentinelFrame = true
	sentinelFrame.closure = nil               // Sentinel frames don't have closures
	sentinelFrame.openUpvalues = nil          // Never captures; clear any stale head from prior slot use
	sentinelFrame.targetRegister = destReg    // Target register in caller
	sentinelFrame.registers = callerRegisters // Give it the caller registers for the result
	vm.frameCount++

	// Use prepareCall to set up the function call
	shouldSwitch, err := vm.prepareCall(fn, thisValue, args, destReg, callerRegisters, callerIP)
	if err != nil {
		// No vm.regDir.popTo(entryRegMark) needed here (unlike the exception
		// paths below): prepareCall only ever returns a non-nil err before it
		// has pushed a register window for the callee (a frame-limit check,
		// or regDir.push itself failing - which per its own doc comment
		// never mutates the cursor on failure), so the cursor is already
		// exactly entryRegMark. Deliberately not added defensively either -
		// popTo's trim() isn't free, and silently popping here if that
		// invariant ever changed would convert a loud, catchable
		// checkRegWindowRelease panic into silent register-space corruption
		// instead (#414).
		vm.frameCount--
		return Undefined, err
	}

	if !shouldSwitch {
		// prepareCall executed synchronously without switching frames - either
		// a native function ran directly, or an exception (e.g. ThrowTypeError
		// for a class constructor called without `new`, see call.go's
		// IsClassConstructor check) was raised inline. The interpreter's own
		// dispatch loop handles the latter case implicitly, by checking
		// vm.unwinding after every instruction it executes - but this is a
		// native Go call into JS with no such loop watching it, so it must
		// check for a pending exception itself before reporting success. Left
		// unchecked, this silently discarded the real exception and reported
		// success with a stale/undefined "result" instead - which is how
		// reassigning globalThis.TypeError to a class went from an unbounded-
		// recursion panic (#231, fixed by the stack-depth guard above) to an
		// "Uncaught exception: undefined" instead of the catchable TypeError
		// real engines produce: the reassigned class's own construction hit
		// this exact "called without new" check, and its resulting exception
		// was getting swallowed right here before ThrowTypeError even saw it.
		//
		// Gated on the transition (!unwindingAtEntry), not just the current
		// state: a native caller can legitimately already be mid-unwind with
		// a live exception when it makes this call (e.g. IteratorClose
		// forwarding a throw completion while it runs cleanup) and have
		// prepareCall's native function complete normally - that must return
		// its real result, not get it discarded in favor of the pre-existing,
		// unrelated exception.
		if !unwindingAtEntry && vm.unwinding && vm.currentException != Null {
			ex := vm.currentException
			vm.currentException = Null
			vm.unwinding = false
			vm.truncateFramesTo(frameCountAtEntry)
			vm.regDir.popTo(entryRegMark)
			return Undefined, exceptionError{exception: ex}
		}
		// Native function was executed directly. Remove sentinel frame - no
		// popTo needed: it may have made its own nested vm.Call that threw
		// and got absorbed, but that nested call's own error path (this
		// function or executeUserFunctionWithNewTarget) already restored its
		// own window before returning to us, so the cursor is already back
		// to entryRegMark regardless. Same reasoning as the err != nil case
		// above (#414).
		vm.frameCount--
		return callerRegisters[destReg], nil
	}

	// We have a new frame set up, mark it as direct call
	if vm.frameCount > 1 { // frameCount includes the sentinel frame
		vm.frames[vm.frameCount-1].isDirectCall = true
	}

	// Execute the VM run loop - it will return when it hits the sentinel frame
	status, result := vm.run()

	if status == InterpretRuntimeError {
		// If the VM is unwinding an exception, surface it as an ExceptionError
		if vm.unwinding && vm.currentException != Null {
			ex := vm.currentException
			// Clear vm.unwinding here too (#142), not just currentException.
			// An earlier version of this left vm.unwinding=true deliberately
			// ("persist for re-throw detection"), reasoning that whatever
			// native code receives this Go error would either make a new
			// vm.Call (whose own entry guard, a few lines below this
			// function, scrubs stale state when currentException==Null) or
			// re-throw via vm.throwException (which sets vm.unwinding=true
			// itself regardless of its prior value). Both are true, but they
			// don't cover a third, common case: native code that ABSORBS the
			// exception - turns it into a rejected promise, aggregates it
			// into a SuppressedError while disposing further resources, etc.
			// - and then returns normally (nil error) without making another
			// vm.Call or re-throwing at all. vm.unwinding is checked, bare,
			// after nearly every opcode across vm.go (that's the actual
			// convention - a fresh, per-operation "did unwinding just start"
			// flag, not sticky ambient state) - including once per
			// instruction in the main dispatch loop itself. Left true, a
			// single absorbed exception poisons every subsequent instruction
			// any vm.run() invocation executes, anywhere, until the next
			// vm.Call happens to scrub it: the very next opcode sees
			// unwinding=true with currentException=Null (a phantom
			// exception) and either aborts the script outright or corrupts
			// unrelated control flow. Reproduced via a promise reaction
			// whose handler throws (pkg/vm/promise.go's
			// triggerPromiseReactions absorbs the Go error into a rejection
			// without ever clearing vm.unwinding, unlike
			// NewPromiseFromExecutor's ClearUnwindingState call for the same
			// shape) followed by an unrelated top-level-await resuming
			// normally - the resumption's very first instruction reported a
			// bogus "Uncaught exception: null" and aborted the script.
			vm.currentException = Null
			vm.unwinding = false
			// We're taking ownership of the exception as a Go error - drop any
			// frame(s) unwinding stopped at without popping (see comment above
			// frameCountAtEntry) so they can't be mistaken for a live boundary
			// by a later, unrelated throw.
			vm.truncateFramesTo(frameCountAtEntry)
			vm.regDir.popTo(entryRegMark)
			return Undefined, exceptionError{exception: ex}
		}
		if msg := vm.lastRecordedErrorMessage(); msg != "" {
			return Undefined, fmt.Errorf("%s", msg)
		}
		return Undefined, fmt.Errorf("runtime error during user function execution")
	}
	// If we reached a direct-call boundary and returned without InterpretRuntimeError,
	// propagate any pending exception to the native caller.
	if vm.unwinding && vm.currentException != Null {
		ex := vm.currentException
		vm.currentException = Null
		vm.unwinding = false // see the matching comment above (#142)
		vm.truncateFramesTo(frameCountAtEntry)
		vm.regDir.popTo(entryRegMark)
		return Undefined, exceptionError{exception: ex}
	}

	return result, nil
}

// ExecuteGenerator is the public interface for generator execution
func (vm *VM) ExecuteGenerator(genObj *GeneratorObject, sentValue Value) (Value, error) {
	return vm.executeGenerator(genObj, sentValue)
}

// ExecuteGeneratorWithException is the public interface for generator execution with exception injection
func (vm *VM) ExecuteGeneratorWithException(genObj *GeneratorObject, exception Value) (Value, error) {
	return vm.executeGeneratorWithException(genObj, exception)
}

// ExecuteGeneratorWithReturn is the public interface for generator execution with return completion
func (vm *VM) ExecuteGeneratorWithReturn(genObj *GeneratorObject, returnValue Value) (Value, error) {
	return vm.resumeGeneratorWithReturn(genObj, returnValue)
}

// NewExceptionError creates an ExceptionError from a VM Value for use in builtins.
func (vm *VM) NewExceptionError(value Value) error {
	return exceptionError{exception: value}
}

// ClearErrors clears all recorded errors from the VM.
// This is used by async generators which convert exceptions to rejected promises.
func (vm *VM) ClearErrors() {
	vm.errors = nil
}

// ClearUnwindingState clears the exception unwinding state.
// This should be called when native code successfully handles an exception
// (e.g., by returning a rejected promise) so the VM knows the exception has been handled.
func (vm *VM) ClearUnwindingState() {
	vm.unwinding = false
	vm.unwindingCrossedNative = false
	vm.currentException = Null
}

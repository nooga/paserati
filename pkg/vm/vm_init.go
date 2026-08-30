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
	// Simple implementation that doesn't use opGetProp to avoid unwinding issues
	// Check for getter (including prototype chain) and call it, or return the property value

	switch obj.Type() {
	case TypeObject:
		po := obj.AsPlainObject()
		// Check own accessor first
		if g, _, _, _, ok := po.GetOwnAccessor(propName); ok && g.Type() != TypeUndefined {
			// Call the getter with this=obj
			result, err := vm.Call(g, obj, nil)
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
			if current.IsObject() {
				if current.Type() == TypeObject {
					proto := current.AsPlainObject()
					// Check for accessor in prototype
					if g, _, _, _, ok := proto.GetOwnAccessor(propName); ok && g.Type() != TypeUndefined {
						// Call the getter with this=original obj (not proto)
						result, err := vm.Call(g, obj, nil)
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
				} else {
					break
				}
			} else {
				break
			}
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
				return arr.Get(idx), nil
			}
			// Check own named properties on the array
			if v, ok := arr.GetOwn(propName); ok {
				return v, nil
			}
			// Check array prototype
			if vm.ArrayPrototype.IsObject() {
				proto := vm.ArrayPrototype.AsPlainObject()
				if v, ok := proto.Get(propName); ok {
					return v, nil
				}
			}
		}
		return Undefined, nil

	case TypeProxy:
		// For Proxy, call the 'get' trap
		proxy := obj.AsProxy()
		if proxy.Revoked {
			return Undefined, vm.NewTypeError("Cannot perform 'get' on a revoked Proxy")
		}
		getTrap, hasGetTrap := proxy.handler.AsPlainObject().GetOwn("get")
		if hasGetTrap && getTrap.Type() != TypeUndefined && getTrap.Type() != TypeNull {
			// Validate trap is callable
			if !getTrap.IsCallable() {
				return Undefined, vm.NewTypeError("'get' on proxy: trap is not a function")
			}
			// Call the get trap: handler.get(target, propertyKey, receiver)
			trapArgs := []Value{proxy.target, NewString(propName), obj}
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
		// No get trap, fall through to target
		return vm.GetProperty(proxy.target, propName)

	case TypePromise:
		// Promise objects: check Promise.prototype chain
		if vm.PromisePrototype.IsObject() {
			proto := vm.PromisePrototype.AsPlainObject()
			if v, ok := proto.Get(propName); ok {
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
			// Check RegExp.prototype (with accessor invocation)
			if vm.RegExpPrototype.IsObject() {
				proto := vm.RegExpPrototype.AsPlainObject()
				if proto != nil {
					// Check for accessor property (getter) on prototype
					if g, _, _, _, ok := proto.GetOwnAccessor(propName); ok && g.Type() != TypeUndefined {
						// Call the getter with this=original RegExp object
						result, err := vm.Call(g, obj, nil)
						if err != nil {
							return Undefined, err
						}
						return result, nil
					}
					// Check for data property on prototype
					if v, ok := proto.GetOwn(propName); ok {
						return v, nil
					}
					// Walk prototype chain (Object.prototype)
					current := proto.GetPrototype()
					for current.typ != TypeNull && current.typ != TypeUndefined {
						if current.IsObject() {
							if current.Type() == TypeObject {
								grandProto := current.AsPlainObject()
								if g, _, _, _, ok := grandProto.GetOwnAccessor(propName); ok && g.Type() != TypeUndefined {
									result, err := vm.Call(g, obj, nil)
									if err != nil {
										return Undefined, err
									}
									return result, nil
								}
								if v, ok := grandProto.GetOwn(propName); ok {
									return v, nil
								}
								current = grandProto.GetPrototype()
							} else {
								break
							}
						} else {
							break
						}
					}
				}
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
			// Get the specific TypedArray prototype based on element type
			var proto Value
			switch ta.GetElementType() {
			case TypedArrayInt8:
				proto = vm.Int8ArrayPrototype
			case TypedArrayUint8:
				proto = vm.Uint8ArrayPrototype
			case TypedArrayUint8Clamped:
				proto = vm.Uint8ClampedArrayPrototype
			case TypedArrayInt16:
				proto = vm.Int16ArrayPrototype
			case TypedArrayUint16:
				proto = vm.Uint16ArrayPrototype
			case TypedArrayInt32:
				proto = vm.Int32ArrayPrototype
			case TypedArrayUint32:
				proto = vm.Uint32ArrayPrototype
			case TypedArrayFloat16:
				proto = vm.Float16ArrayPrototype
			case TypedArrayFloat32:
				proto = vm.Float32ArrayPrototype
			case TypedArrayFloat64:
				proto = vm.Float64ArrayPrototype
			case TypedArrayBigInt64:
				proto = vm.BigInt64ArrayPrototype
			case TypedArrayBigUint64:
				proto = vm.BigUint64ArrayPrototype
			default:
				proto = vm.TypedArrayPrototype
			}
			// Check prototype chain - need to check for accessors (getters) first
			if proto.IsObject() {
				cur := proto.AsPlainObject()
				for cur != nil {
					// Check for accessor (getter) first
					if getter, _, _, _, ok := cur.GetOwnAccessor(propName); ok {
						if getter.Type() != TypeUndefined {
							// Call the getter with this=obj (the TypedArray)
							result, err := vm.Call(getter, obj, nil)
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
		// Set objects: check prototype chain (especially for accessor properties like size)
		if vm.SetPrototype.IsObject() {
			proto := vm.SetPrototype.AsPlainObject()
			// Check for accessor (getter) first
			if getter, _, _, _, ok := proto.GetOwnAccessor(propName); ok {
				if getter.Type() != TypeUndefined {
					// Call the getter with this=obj (the Set)
					result, err := vm.Call(getter, obj, nil)
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

	case TypeMap:
		// Map objects: check prototype chain (especially for accessor properties like size)
		if vm.MapPrototype.IsObject() {
			proto := vm.MapPrototype.AsPlainObject()
			// Check for accessor (getter) first
			if getter, _, _, _, ok := proto.GetOwnAccessor(propName); ok {
				if getter.Type() != TypeUndefined {
					// Call the getter with this=obj (the Map)
					result, err := vm.Call(getter, obj, nil)
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

	case TypeFunction:
		// Function objects: check own properties, then [[Prototype]] chain, then Function.prototype
		fn := obj.AsFunction()
		if fn != nil {
			// Check own properties first
			if fn.Properties != nil {
				// Check for accessor (getter) on own properties
				if getter, _, _, _, ok := fn.Properties.GetOwnAccessor(propName); ok {
					if getter.Type() != TypeUndefined {
						result, err := vm.Call(getter, obj, nil)
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
				return vm.GetProperty(fn.Prototype, propName)
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
							result, err := vm.Call(getter, obj, nil)
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
						result, err := vm.Call(getter, obj, nil)
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
					return vm.GetProperty(cl.Fn.Prototype, propName)
				}
			}
			// Fall back to Function.prototype (which is a NativeFunctionWithProps)
			if vm.FunctionPrototype.Type() == TypeNativeFunctionWithProps {
				nfp := vm.FunctionPrototype.AsNativeFunctionWithProps()
				if nfp != nil && nfp.Properties != nil {
					// Check for accessor first
					if getter, _, _, _, ok := nfp.Properties.GetOwnAccessor(propName); ok {
						if getter.Type() != TypeUndefined {
							result, err := vm.Call(getter, obj, nil)
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
					result, err := vm.Call(getter, obj, nil)
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
						result, err := vm.Call(getter, obj, nil)
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
					result, err := vm.Call(getter, obj, nil)
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
						result, err := vm.Call(getter, obj, nil)
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
					result, err := vm.Call(getter, obj, nil)
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
				result, err := vm.Call(g, obj, nil)
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
							result, err := vm.Call(g, obj, nil)
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

func (vm *VM) Call(fn Value, thisValue Value, args []Value) (Value, error) {
	switch fn.Type() {
	case TypeNativeFunction:
		// For native functions, call directly with proper 'this' context
		nativeFunc := AsNativeFunction(fn)
		prevThis := vm.currentThis
		vm.currentThis = thisValue
		defer func() { vm.currentThis = prevThis }()
		return nativeFunc.Fn(args)

	case TypeNativeFunctionWithProps:
		// Handle native function with properties
		nativeFuncWithProps := fn.AsNativeFunctionWithProps()
		prevThis := vm.currentThis
		vm.currentThis = thisValue
		defer func() { vm.currentThis = prevThis }()
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

		// Get the apply trap from handler (handler can be PlainObject or DictObject)
		handler := proxy.Handler()
		var applyTrap Value
		var hasApplyTrap bool

		switch handler.Type() {
		case TypeObject:
			applyTrap, hasApplyTrap = handler.AsPlainObject().GetOwn("apply")
		case TypeDictObject:
			applyTrap, hasApplyTrap = handler.AsDictObject().GetOwn("apply")
		}

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
		var newTargetFn *FunctionObject
		if newTarget.Type() == TypeClosure {
			newTargetFn = newTarget.AsClosure().Fn
		} else if newTarget.Type() == TypeFunction {
			newTargetFn = newTarget.AsFunction()
		}

		var prototype Value
		if newTargetFn != nil {
			prototype = newTargetFn.GetOrCreatePrototypeWithVM(vm)
		} else {
			// Fallback to constructor's prototype
			prototype = fn.GetOrCreatePrototypeWithVM(vm)
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
// This does NOT reclaim vm.nextRegSlot for the dropped frame(s)' register
// windows. That's a real leaked-registers gap (unwindException's own
// frame-popping loop has the same gap: it decrements vm.frameCount without
// touching vm.nextRegSlot for every frame it walks past), but register
// space is only reclaimed in bulk relative to the frame the dispatch loop
// eventually resumes at, not frame-by-frame during unwinding - subtracting
// an additional, independently-computed delta here corrupted that
// accounting and broke a previously-working reproducer (nextRegSlot went
// negative). Left as a documented pre-existing gap rather than a local fix
// that isn't provably correct; see issue #61.
func (vm *VM) truncateFramesTo(entryCount int) {
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

	// Set up the caller context (pooled 1-element result holder, not a per-call alloc)
	callerRegisters := vm.getSentinelReg()
	defer vm.putSentinelReg(callerRegisters)
	destReg := byte(0)
	callerIP := 0

	// Add a sentinel frame
	sentinelFrame := &vm.frames[vm.frameCount]
	sentinelFrame.isSentinelFrame = true
	sentinelFrame.closure = nil
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
		vm.frameCount--
		return Undefined, err
	}

	if !shouldSwitch {
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
			vm.currentException = Null
			vm.truncateFramesTo(frameCountAtEntry)
			return Undefined, exceptionError{exception: ex}
		}
		return Undefined, fmt.Errorf("runtime error during constructor execution")
	}

	if vm.unwinding && vm.currentException != Null {
		ex := vm.currentException
		vm.currentException = Null
		vm.truncateFramesTo(frameCountAtEntry)
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

	// Set up the caller context first (pooled 1-element result holder)
	callerRegisters := vm.getSentinelReg()
	defer vm.putSentinelReg(callerRegisters)
	destReg := byte(0)
	callerIP := 0

	// Add a sentinel frame that will cause vm.run() to return when it hits this frame
	sentinelFrame := &vm.frames[vm.frameCount]
	sentinelFrame.isSentinelFrame = true
	sentinelFrame.closure = nil               // Sentinel frames don't have closures
	sentinelFrame.targetRegister = destReg    // Target register in caller
	sentinelFrame.registers = callerRegisters // Give it the caller registers for the result
	vm.frameCount++

	// Use prepareCall to set up the function call
	shouldSwitch, err := vm.prepareCall(fn, thisValue, args, destReg, callerRegisters, callerIP)
	if err != nil {
		// Remove sentinel frame on error
		vm.frameCount--
		return Undefined, err
	}

	if !shouldSwitch {
		// Native function was executed directly
		// Remove sentinel frame
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
			// ⚠️ CRITICAL CHANGE: Don't clear vm.unwinding or vm.unwindingCrossedNative!
			// These flags need to persist for re-throw detection
			// Only clear currentException since we're passing it as a Go error
			vm.currentException = Null
			// vm.unwinding = false         // OLD: Don't clear this!
			// vm.unwindingCrossedNative... // OLD: Don't clear this either!
			// We're taking ownership of the exception as a Go error - drop any
			// frame(s) unwinding stopped at without popping (see comment above
			// frameCountAtEntry) so they can't be mistaken for a live boundary
			// by a later, unrelated throw.
			vm.truncateFramesTo(frameCountAtEntry)
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
		// vm.unwinding = false         // OLD: Don't clear this!
		// vm.unwindingCrossedNative... // OLD: Don't clear this either!
		vm.truncateFramesTo(frameCountAtEntry)
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

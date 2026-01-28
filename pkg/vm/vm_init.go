package vm

import "fmt"

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
	if vm.frameCount >= MaxFrames {
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
	if vm.frameCount >= MaxFrames {
		return Undefined, fmt.Errorf("stack overflow during direct function call")
	}

	// Get function arity and adjust arguments accordingly
	var expectedArity int
	if fn.IsFunction() {
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
	// Just check for getter and call it, or return the property value

	switch obj.Type() {
	case TypeObject:
		po := obj.AsPlainObject()
		// Check if it's an accessor (getter)
		if g, _, _, _, ok := po.GetOwnAccessor(propName); ok && g.Type() != TypeUndefined {
			// Call the getter with this=obj
			result, err := vm.Call(g, obj, nil)
			if err != nil {
				return Undefined, err
			}
			return result, nil
		}
		// Not an accessor, check for regular property including prototype chain
		if value, exists := po.Get(propName); exists {
			return value, nil
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
					if proto2 := current.AsPlainObject(); proto2 != nil {
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
		if hasGetTrap && getTrap.IsCallable() {
			// Call the get trap: handler.get(target, propertyKey, receiver)
			trapArgs := []Value{proxy.target, NewString(propName), obj}
			result, err := vm.Call(getTrap, proxy.handler, trapArgs)
			if err != nil {
				return Undefined, err
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

	default:
		// For non-objects, just return undefined
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

	default:
		// For non-objects, this is a no-op (or could throw in strict mode)
		return nil
	}
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
			// Arrays use ArrayPrototype for symbol properties
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
// This creates a new object and calls the constructor with that object as 'this'
func (vm *VM) Construct(constructor Value, args []Value) (Value, error) {
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
		prevThis := vm.currentThis
		vm.currentThis = Undefined // Native constructors typically create their own 'this'
		defer func() { vm.currentThis = prevThis }()
		return nf.Fn(args)

	case TypeNativeFunctionWithProps:
		nfp := constructor.AsNativeFunctionWithProps()
		if !nfp.IsConstructor {
			return Undefined, fmt.Errorf("%s is not a constructor", nfp.Name)
		}
		prevThis := vm.currentThis
		vm.currentThis = Undefined
		defer func() { vm.currentThis = prevThis }()
		return nfp.Fn(args)

	case TypeClosure, TypeFunction:
		// For user-defined constructors, create a new object with the prototype
		// and call the function with that object as 'this'
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

		// Create new object with constructor's prototype
		prototype := fn.GetOrCreatePrototypeWithVM(vm)
		newObj := NewObject(prototype)

		// Set constructor call flag so prepareCall allows class constructors
		prevInConstructorCall := vm.inConstructorCall
		vm.inConstructorCall = true

		// Call the constructor with the new object as 'this'
		result, err := vm.executeUserFunctionSafe(constructor, newObj, args)

		// Restore previous state
		vm.inConstructorCall = prevInConstructorCall
		if err != nil {
			return Undefined, err
		}

		// If the constructor returns an object, use that; otherwise use the new object
		if result.IsObject() {
			return result, nil
		}
		return newObj, nil

	case TypeBoundFunction:
		bf := constructor.AsBoundFunction()
		// Combine partial args with call-time args
		finalArgs := make([]Value, len(bf.PartialArgs)+len(args))
		copy(finalArgs, bf.PartialArgs)
		copy(finalArgs[len(bf.PartialArgs):], args)
		// Bound functions ignore their boundThis when called as constructors
		return vm.Construct(bf.OriginalFunction, finalArgs)

	default:
		return Undefined, fmt.Errorf("%s is not a constructor", constructor.TypeName())
	}
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
		// Note: native constructors don't fully support custom newTarget
		prevThis := vm.currentThis
		vm.currentThis = Undefined
		defer func() { vm.currentThis = prevThis }()
		return nf.Fn(args)

	case TypeNativeFunctionWithProps:
		nfp := constructor.AsNativeFunctionWithProps()
		if !nfp.IsConstructor {
			return Undefined, fmt.Errorf("%s is not a constructor", nfp.Name)
		}
		prevThis := vm.currentThis
		vm.currentThis = Undefined
		defer func() { vm.currentThis = prevThis }()
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
		// Bound functions ignore their boundThis when called as constructors
		return vm.ConstructWithNewTarget(bf.OriginalFunction, finalArgs, newTarget)

	default:
		return Undefined, fmt.Errorf("%s is not a constructor", constructor.TypeName())
	}
}

// executeUserFunctionWithNewTarget executes a user function with constructor semantics and custom new.target
func (vm *VM) executeUserFunctionWithNewTarget(fn Value, thisValue Value, args []Value, newTarget Value, isDerivedConstructor bool) (Value, error) {
	// Clear stale unwinding state
	if vm.unwinding && vm.currentException == Null {
		vm.unwinding = false
		vm.unwindingCrossedNative = false
	}

	// Set up the caller context
	callerRegisters := make([]Value, 1)
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
			return Undefined, exceptionError{exception: ex}
		}
		return Undefined, fmt.Errorf("runtime error during constructor execution")
	}

	if vm.unwinding && vm.currentException != Null {
		ex := vm.currentException
		vm.currentException = Null
		return Undefined, exceptionError{exception: ex}
	}

	return result, nil
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

	// Set up the caller context first
	callerRegisters := make([]Value, 1)
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
			return Undefined, exceptionError{exception: ex}
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

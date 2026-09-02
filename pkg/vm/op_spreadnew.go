package vm

import "fmt"

// populateSpreadCallRegisters copies spreadArgs into a newly-created frame's
// parameter registers and builds any rest-parameter array, mirroring what
// prepareCallWithGeneratorMode (call.go) already does for every other kind
// of call (a direct call, and a spread call to an ordinary function).
// handleOpSpreadNew (super(...)/new Foo(...) with a spread argument) had
// neither of these (paserati#182):
//   - without Undefined-padding, a declared parameter beyond argCount but
//     within the callee's arity kept whatever stale value already happened
//     to be sitting in that register-stack slot from an earlier frame that
//     used the same slots (register-stack space is reused across frames,
//     not zeroed per call) - only reachable, and only observable, once a
//     spread call passes fewer arguments than the callee declares;
//   - without rest-parameter handling, a variadic constructor's `...rest`
//     parameter received whatever got copied positionally into its
//     register (the first spread argument, or undefined) instead of a real
//     array - the exact shape of a `super(...arguments)` pass-through
//     constructor once the "arguments is not iterable" bug this issue is
//     about stopped masking it.
func populateSpreadCallRegisters(vmInstance *VM, calleeFunc *FunctionObject, spreadArgs []Value, registers []Value) {
	argCount := len(spreadArgs)
	maxArgsToCopy := argCount
	if calleeFunc.Arity > maxArgsToCopy {
		maxArgsToCopy = calleeFunc.Arity
	}
	if maxArgsToCopy > len(registers) {
		maxArgsToCopy = len(registers)
	}
	for i := 0; i < maxArgsToCopy; i++ {
		if i < argCount {
			registers[i] = spreadArgs[i]
		} else {
			registers[i] = Undefined
		}
	}

	if calleeFunc.Variadic {
		extraArgCount := argCount - calleeFunc.Arity
		var restArray Value
		if extraArgCount <= 0 {
			restArray = vmInstance.emptyRestArray
		} else {
			restArray = NewArray()
			restArrayObj := restArray.AsArray()
			for i := 0; i < extraArgCount; i++ {
				argIndex := calleeFunc.Arity + i
				if argIndex < len(spreadArgs) {
					restArrayObj.Append(spreadArgs[argIndex])
				}
			}
		}
		if calleeFunc.Arity < len(registers) {
			registers[calleeFunc.Arity] = restArray
		}
	}
}

// handleOpSpreadNew handles OpSpreadNew bytecode instruction for constructor calls with spread arguments
func (vm *VM) handleOpSpreadNew(code []byte, ip *int, frame *CallFrame, registers []Value) (InterpretResult, Value) {
	destReg := code[*ip]
	constructorReg := code[*ip+1]
	spreadArgReg := code[*ip+2]
	flags := code[*ip+3]
	*ip += 4
	inheritNewTarget := (flags & 0x01) != 0

	callerRegisters := registers
	callerIP := *ip

	constructorVal := callerRegisters[constructorReg]

	// ES6 12.3.3.1.1 step 7: Validate that constructor is constructible
	// This must throw TypeError for primitives and non-constructor objects
	if !constructorVal.IsCallable() {
		frame.ip = callerIP
		vm.ThrowTypeError(fmt.Sprintf("%s is not a constructor", constructorVal.TypeName()))
		if !vm.unwinding {
			// Exception was caught by a handler; caller reloads its cached
			// frame/registers from vm.frameCount and continues.
			return InterpretOK, Undefined
		}
		return InterpretRuntimeError, Undefined
	}

	// Additional check for functions that are not constructors
	// Arrow functions, async functions (non-generator), and plain generators cannot be constructors
	if constructorVal.Type() == TypeFunction {
		fn := AsFunction(constructorVal)
		if fn.IsArrowFunction || (fn.IsAsync && !fn.IsGenerator) {
			frame.ip = callerIP
			vm.ThrowTypeError(fmt.Sprintf("%s is not a constructor", constructorVal.TypeName()))
			if !vm.unwinding {
				return InterpretOK, Undefined
			}
			return InterpretRuntimeError, Undefined
		}
	} else if constructorVal.Type() == TypeClosure {
		cl := AsClosure(constructorVal)
		if cl.Fn.IsArrowFunction || (cl.Fn.IsAsync && !cl.Fn.IsGenerator) {
			frame.ip = callerIP
			vm.ThrowTypeError(fmt.Sprintf("%s is not a constructor", constructorVal.TypeName()))
			if !vm.unwinding {
				return InterpretOK, Undefined
			}
			return InterpretRuntimeError, Undefined
		}
	}

	spreadArrayVal := callerRegisters[spreadArgReg]

	// Extract arguments from spread array
	spreadArgs, err := vm.extractSpreadArguments(spreadArrayVal)
	if err != nil {
		frame.ip = callerIP
		// Check if it's a VM exception (TypeError, etc.) and propagate it
		if ee, ok := err.(ExceptionError); ok {
			vm.throwException(ee.GetExceptionValue())
			if !vm.unwinding {
				return InterpretOK, Undefined
			}
			return InterpretRuntimeError, Undefined
		}
		// Otherwise wrap as generic runtime error
		status := vm.runtimeError("Spread constructor call error: %s", err.Error())
		return status, Undefined
	}
	argCount := len(spreadArgs)

	switch constructorVal.Type() {
	case TypeClosure:
		constructorClosure := AsClosure(constructorVal)
		constructorFunc := constructorClosure.Fn

		// Check if it's an arrow function
		if constructorFunc.IsArrowFunction {
			frame.ip = callerIP
			vm.ThrowTypeError("Arrow functions cannot be used as constructors")
			if !vm.unwinding {
				return InterpretOK, Undefined
			}
			return InterpretRuntimeError, Undefined
		}

		// Check stack limits
		if vm.frameCount >= len(vm.frames) {
			frame.ip = callerIP
			// #135: a catchable RangeError, not an internal runtimeError - deep
			// constructor recursion is a normal (if buggy) JS program state, not
			// a VM invariant violation, so it must be try/catch-able like
			// V8/Node's "Maximum call stack size exceeded".
			vm.ThrowRangeError("Maximum call stack size exceeded")
			if !vm.unwinding {
				return InterpretOK, Undefined
			}
			return InterpretRuntimeError, Undefined
		}
		requiredRegs := constructorFunc.RegisterSize
		if vm.nextRegSlot+requiredRegs > len(vm.registerStack) {
			frame.ip = callerIP
			vm.ThrowRangeError("Maximum call stack size exceeded")
			if !vm.unwinding {
				return InterpretOK, Undefined
			}
			return InterpretRuntimeError, Undefined
		}

		// Determine the new.target value for this constructor call
		// If inheritNewTarget flag is set (super() calls), inherit new.target from caller
		// Otherwise, new.target is the constructor being called
		var newTargetValue Value
		if inheritNewTarget && frame.isConstructorCall && frame.newTargetValue.Type() != TypeUndefined {
			// This is a super() call from a derived constructor - inherit new.target
			newTargetValue = frame.newTargetValue
		} else {
			// Direct new Constructor() call - new.target is the constructor
			newTargetValue = constructorVal
		}

		// Get the prototype to use for the instance from new.target.prototype
		// This ensures derived classes create instances with the correct prototype
		var instancePrototype Value
		if newTargetValue.Type() == TypeClosure {
			newTargetClosure := AsClosure(newTargetValue)
			// Use closure's GetPrototypeWithVM which checks closure.Properties first
			instancePrototype = newTargetClosure.GetPrototypeWithVM(vm)
		} else if newTargetValue.Type() == TypeFunction {
			newTargetFunc := AsFunction(newTargetValue)
			instancePrototype = newTargetFunc.GetOrCreatePrototypeWithVM(vm)
		} else {
			// Fallback: use the constructor's prototype
			instancePrototype = constructorFunc.GetOrCreatePrototypeWithVM(vm)
		}

		// Create instance (or leave undefined for derived constructors)
		var newInstance Value
		if constructorFunc.IsDerivedConstructor {
			newInstance = Uninitialized // 'this' is in TDZ until super() is called
		} else {
			newInstance = NewObject(instancePrototype)
		}

		frame.ip = callerIP

		// Create new frame
		newFrame := &vm.frames[vm.frameCount]
		newFrame.closure = constructorClosure
		newFrame.ip = 0
		newFrame.targetRegister = destReg
		newFrame.thisValue = newInstance
		newFrame.homeObject = instancePrototype  // Set [[HomeObject]] for super property access in constructors
		newFrame.isConstructorCall = true
		newFrame.isDirectCall = false            // Not a direct call (spread new)
		newFrame.isSentinelFrame = false         // Clear sentinel flag when reusing frame
		newFrame.newTargetValue = newTargetValue // Use propagated new.target
		newFrame.argCount = argCount             // Store actual argument count for arguments object
		// Avoid per-call allocation: keep a view of spreadArgs for OpGetArguments.
		// (If `arguments` is accessed, NewArguments will allocate as needed.)
		newFrame.args = spreadArgs
		newFrame.argumentsObject = Undefined // Initialize to Undefined (will be created on first access)
		newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
		newFrame.allocatedRegSize = requiredRegs // Track actual allocation for proper cleanup
		vm.nextRegSlot += requiredRegs

		// Allocate spill slots if this function needs them (for register overflow)
		if constructorFunc.Chunk.NumSpillSlots > 0 {
			newFrame.spillSlots = make([]Value, constructorFunc.Chunk.NumSpillSlots)
		} else {
			newFrame.spillSlots = nil
		}

		// Copy spread arguments to new frame, padding any declared parameter
		// beyond argCount with Undefined and building a real rest-parameter
		// array if the constructor is variadic - see populateSpreadCallRegisters
		// (paserati#182).
		populateSpreadCallRegisters(vm, constructorFunc, spreadArgs, newFrame.registers)
		vm.frameCount++

		// Store instance in caller's destination register
		callerRegisters[destReg] = newInstance

		// Return OK - caller will switch to new frame
		return InterpretOK, Undefined

	case TypeFunction:
		funcToCall := AsFunction(constructorVal)

		// Check if it's an arrow function
		if funcToCall.IsArrowFunction {
			frame.ip = callerIP
			vm.ThrowTypeError("Arrow functions cannot be used as constructors")
			if !vm.unwinding {
				return InterpretOK, Undefined
			}
			return InterpretRuntimeError, Undefined
		}

		constructorClosure := &ClosureObject{Fn: funcToCall, Upvalues: []*Upvalue{}}
		constructorFunc := constructorClosure.Fn

		// Check stack limits
		if vm.frameCount >= len(vm.frames) {
			frame.ip = callerIP
			// #135: a catchable RangeError, not an internal runtimeError - deep
			// constructor recursion is a normal (if buggy) JS program state, not
			// a VM invariant violation, so it must be try/catch-able like
			// V8/Node's "Maximum call stack size exceeded".
			vm.ThrowRangeError("Maximum call stack size exceeded")
			if !vm.unwinding {
				return InterpretOK, Undefined
			}
			return InterpretRuntimeError, Undefined
		}
		requiredRegs := constructorFunc.RegisterSize
		if vm.nextRegSlot+requiredRegs > len(vm.registerStack) {
			frame.ip = callerIP
			vm.ThrowRangeError("Maximum call stack size exceeded")
			if !vm.unwinding {
				return InterpretOK, Undefined
			}
			return InterpretRuntimeError, Undefined
		}

		// Determine the new.target value for this constructor call
		// If inheritNewTarget flag is set (super() calls), inherit new.target from caller
		// Otherwise, new.target is the constructor being called
		var newTargetValue Value
		if inheritNewTarget && frame.isConstructorCall && frame.newTargetValue.Type() != TypeUndefined {
			// This is a super() call from a derived constructor - inherit new.target
			newTargetValue = frame.newTargetValue
		} else {
			// Direct new Constructor() call - new.target is the constructor
			newTargetValue = constructorVal
		}

		// Get the prototype to use for the instance from new.target.prototype
		// This ensures derived classes create instances with the correct prototype
		var instancePrototype Value
		if newTargetValue.Type() == TypeClosure {
			newTargetClosure := AsClosure(newTargetValue)
			// Use closure's GetPrototypeWithVM which checks closure.Properties first
			instancePrototype = newTargetClosure.GetPrototypeWithVM(vm)
		} else if newTargetValue.Type() == TypeFunction {
			newTargetFunc := AsFunction(newTargetValue)
			instancePrototype = newTargetFunc.GetOrCreatePrototypeWithVM(vm)
		} else {
			// Fallback: use the constructor's prototype
			instancePrototype = constructorFunc.GetOrCreatePrototypeWithVM(vm)
		}

		// Create instance (or leave uninitialized for derived constructors)
		var newInstance Value
		if constructorFunc.IsDerivedConstructor {
			newInstance = Uninitialized // 'this' is in TDZ until super() is called
		} else {
			newInstance = NewObject(instancePrototype)
		}

		frame.ip = callerIP

		// Create new frame
		newFrame := &vm.frames[vm.frameCount]
		newFrame.closure = constructorClosure
		newFrame.ip = 0
		newFrame.targetRegister = destReg
		newFrame.thisValue = newInstance
		newFrame.homeObject = instancePrototype  // Set [[HomeObject]] for super property access in constructors
		newFrame.isConstructorCall = true
		newFrame.isDirectCall = false            // Not a direct call (spread new)
		newFrame.isSentinelFrame = false         // Clear sentinel flag when reusing frame
		newFrame.newTargetValue = newTargetValue // Use propagated new.target
		newFrame.argCount = argCount             // Store actual argument count for arguments object
		// Avoid per-call allocation: keep a view of spreadArgs for OpGetArguments.
		newFrame.args = spreadArgs
		newFrame.argumentsObject = Undefined // Initialize to Undefined (will be created on first access)
		newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
		newFrame.allocatedRegSize = requiredRegs // Track actual allocation for proper cleanup
		vm.nextRegSlot += requiredRegs

		// Allocate spill slots if this function needs them (for register overflow)
		if constructorFunc.Chunk.NumSpillSlots > 0 {
			newFrame.spillSlots = make([]Value, constructorFunc.Chunk.NumSpillSlots)
		} else {
			newFrame.spillSlots = nil
		}

		// Copy spread arguments to new frame, padding any declared parameter
		// beyond argCount with Undefined and building a real rest-parameter
		// array if the constructor is variadic - see populateSpreadCallRegisters
		// (paserati#182).
		populateSpreadCallRegisters(vm, constructorFunc, spreadArgs, newFrame.registers)
		vm.frameCount++

		// Store instance in caller's destination register
		callerRegisters[destReg] = newInstance

		// Return OK - caller will switch to new frame
		return InterpretOK, Undefined

	case TypeNativeFunction, TypeNativeFunctionWithProps:
		// Native constructors handle their own instance creation. Resolve
		// the function pointer per type — TypeNativeFunctionWithProps has a
		// different struct layout than TypeNativeFunction, so a single
		// AsNativeFunction cast would panic on the with-props case.
		var fn func(args []Value) (Value, error)
		var isCtor bool
		var name string
		switch constructorVal.Type() {
		case TypeNativeFunction:
			nf := constructorVal.AsNativeFunction()
			fn, isCtor, name = nf.Fn, nf.IsConstructor, nf.Name
		case TypeNativeFunctionWithProps:
			nfp := constructorVal.AsNativeFunctionWithProps()
			fn, isCtor, name = nfp.Fn, nfp.IsConstructor, nfp.Name
		}
		if !isCtor {
			frame.ip = callerIP
			vm.ThrowTypeError(fmt.Sprintf("%s is not a constructor", name))
			if !vm.unwinding {
				return InterpretOK, Undefined
			}
			return InterpretRuntimeError, Undefined
		}

		// Pick newTarget: caller-inherited for super(), constructor for direct new.
		newTargetForNative := constructorVal
		if inheritNewTarget && frame.isConstructorCall && frame.newTargetValue.Type() != TypeUndefined {
			newTargetForNative = frame.newTargetValue
		}
		frame.ip = callerIP
		prevNewTarget := vm.currentNewTarget
		vm.currentNewTarget = newTargetForNative
		vm.inConstructorCall = true
		result, nativeErr := fn(spreadArgs)
		vm.inConstructorCall = false
		vm.currentNewTarget = prevNewTarget
		if nativeErr != nil {
			if ee, ok := nativeErr.(ExceptionError); ok {
				vm.throwException(ee.GetExceptionValue())
				if !vm.unwinding {
					return InterpretOK, Undefined
				}
				return InterpretRuntimeError, Undefined
			}
			status := vm.runtimeError("Native constructor error: %s", nativeErr.Error())
			return status, Undefined
		}

		// Subclass-of-native: when super(...args) reaches a native ctor that built
		// its instance with the intrinsic prototype, retarget the instance's
		// [[Prototype]] to the subclass (newTarget.prototype). Skip direct
		// `new Native(...)` where newTarget is the base ctor itself.
		if inheritNewTarget && !newTargetForNative.Is(constructorVal) {
			vm.applySubclassPrototype(result, newTargetForNative)
		}

		callerRegisters[destReg] = result
		return InterpretOK, Undefined

	default:
		frame.ip = callerIP
		status := vm.runtimeError("Cannot use '%s' as a constructor.", constructorVal.TypeName())
		return status, Undefined
	}
}

package vm

import (
	"fmt"
	"os"
	"paseratti2/pkg/bytecode"
	"paseratti2/pkg/value"
	"unsafe"
)

const RegFileSize = 256 // Max registers per function call frame
const MaxFrames = 64    // Max call stack depth

// CallFrame represents a single active function call.
type CallFrame struct {
	closure *value.Closure // Closure being executed (contains Function and Upvalues)
	ip      int            // Instruction pointer *within* this frame's closure.Fn.Chunk.Code
	// `registers` is a slice pointing into the VM's main register stack,
	// defining the window for this frame.
	registers []value.Value
}

// VM represents the virtual machine state.
type VM struct {
	// The call stack
	frames     [MaxFrames]CallFrame
	frameCount int

	// Register file, treated as a stack. Each CallFrame gets a window into this.
	// This avoids reallocating register arrays for every call.
	registerStack [RegFileSize * MaxFrames]value.Value
	nextRegSlot   int // Points to the next available slot in registerStack

	// List of upvalues pointing to variables still on the registerStack
	openUpvalues []*value.Upvalue
	// Globals, open upvalues, etc. would go here later
}

// InterpretResult represents the outcome of an interpretation.
type InterpretResult uint8

const (
	InterpretOK InterpretResult = iota
	InterpretCompileError
	InterpretRuntimeError
)

// NewVM creates a new VM instance.
func NewVM() *VM {
	return &VM{
		// frameCount and nextRegSlot initialized to 0
		openUpvalues: make([]*value.Upvalue, 0, 16), // Pre-allocate slightly
	}
}

// Reset clears the VM state, ready for new execution.
func (vm *VM) Reset() {
	vm.frameCount = 0
	vm.nextRegSlot = 0
	vm.openUpvalues = vm.openUpvalues[:0] // Clear slice while keeping capacity
	// No need to clear registerStack explicitly, slots will be overwritten.
}

// Interpret starts executing the given chunk of bytecode in a new top-level frame.
func (vm *VM) Interpret(chunk *bytecode.Chunk) InterpretResult {
	vm.Reset()

	// Wrap the main script chunk in a dummy function and closure
	mainFunc := &bytecode.Function{Chunk: chunk, Name: "<script>", RegisterSize: RegFileSize} // Assume max regs for script
	mainClosure := &value.Closure{Fn: mainFunc, Upvalues: []*value.Upvalue{}}                 // No upvalues for main

	// Allocate registers for the main script body/function
	initialRegs := mainFunc.RegisterSize
	if vm.nextRegSlot+initialRegs > len(vm.registerStack) {
		fmt.Println("Register stack overflow (initial frame)")
		return InterpretRuntimeError
	}

	frame := &vm.frames[vm.frameCount] // Get pointer to the frame slot
	frame.closure = mainClosure
	frame.ip = 0
	frame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+initialRegs]
	vm.nextRegSlot += initialRegs
	vm.frameCount++

	return vm.run()
}

// run is the main execution loop.
func (vm *VM) run() InterpretResult {
	// --- Caching frame variables ---
	if vm.frameCount == 0 {
		return InterpretOK // Nothing to run
	}
	frame := &vm.frames[vm.frameCount-1]
	// Get function/chunk/constants FROM the closure in the frame
	closure := frame.closure
	// We need to type-assert closure.Fn to the expected *bytecode.Function
	function, ok := closure.Fn.(*bytecode.Function)
	if !ok {
		// This indicates an internal error - a non-function was stored in a closure
		return vm.runtimeError("Internal VM Error: Closure does not contain a valid function.")
	}
	code := function.Chunk.Code
	constants := function.Chunk.Constants
	registers := frame.registers // This is the frame's register window
	ip := frame.ip

	// Store the target register for the *caller* when returning from a function
	var returnTargetReg byte = 0 // Default for top-level

	for {
		// --- Debugging (Optional) ---
		// fmt.Printf("ip=%04d regs=")
		// for i := 0; i < 10; i++ { // Print first few regs
		// 	if i < len(frame.registers) {
		// 		fmt.Printf("[R%d:%s] ", i, frame.registers[i])
		// 	} else { break }
		// }
		// fmt.Println()
		// frame.chunk.DisassembleInstruction(frame.ip) // Requires offset -> line mapping fixed
		// ---------------------------

		if ip >= len(code) {
			// Save IP before erroring
			frame.ip = ip
			if vm.frameCount > 1 {
				// If we run off the end of a function without OpReturn, that's an error
				return vm.runtimeError("Implicit return missing in function?")
			} else {
				// Running off end of main script is okay if implicit return wasn't added (shouldn't happen)
				fmt.Println("Warning: Reached end of main script bytecode without explicit or implicit return.")
				return InterpretOK
			}
		}

		opcode := bytecode.OpCode(code[ip])
		ip++ // Advance IP past the opcode itself

		switch opcode {
		case bytecode.OpLoadConst:
			reg := code[ip]
			constIdxHi := code[ip+1]
			constIdxLo := code[ip+2]
			constIdx := uint16(constIdxHi)<<8 | uint16(constIdxLo)
			ip += 3
			if int(constIdx) >= len(constants) {
				frame.ip = ip // Save IP
				return vm.runtimeError("Invalid constant index %d", constIdx)
			}
			registers[reg] = constants[constIdx]

		case bytecode.OpLoadNull:
			reg := code[ip]
			ip++
			registers[reg] = value.Null()

		case bytecode.OpLoadUndefined:
			reg := code[ip]
			ip++
			registers[reg] = value.Undefined()

		case bytecode.OpLoadTrue:
			reg := code[ip]
			ip++
			registers[reg] = value.Bool(true)

		case bytecode.OpLoadFalse:
			reg := code[ip]
			ip++
			registers[reg] = value.Bool(false)

		case bytecode.OpMove:
			regDest := code[ip]
			regSrc := code[ip+1]
			ip += 2
			registers[regDest] = registers[regSrc]

		case bytecode.OpNegate:
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2
			srcVal := registers[srcReg]
			if !value.IsNumber(srcVal) {
				frame.ip = ip
				return vm.runtimeError("Operand must be a number for negation.")
			}
			registers[destReg] = value.Number(-value.AsNumber(srcVal))

		case bytecode.OpNot:
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2
			srcVal := registers[srcReg]
			// In many languages, ! evaluates truthiness
			registers[destReg] = value.Bool(isFalsey(srcVal))

		case bytecode.OpAdd, bytecode.OpSubtract, bytecode.OpMultiply, bytecode.OpDivide,
			bytecode.OpEqual, bytecode.OpNotEqual, bytecode.OpGreater, bytecode.OpLess,
			bytecode.OpLessEqual:
			destReg := code[ip]
			leftReg := code[ip+1]
			rightReg := code[ip+2]
			ip += 3
			leftVal := registers[leftReg]
			rightVal := registers[rightReg]

			// Type checking specific to operation groups
			switch opcode {
			case bytecode.OpAdd:
				// Allow string concatenation or number addition
				if value.IsNumber(leftVal) && value.IsNumber(rightVal) {
					registers[destReg] = value.Number(value.AsNumber(leftVal) + value.AsNumber(rightVal))
				} else if value.IsString(leftVal) && value.IsString(rightVal) {
					// Consider performance of string concat later
					registers[destReg] = value.String(value.AsString(leftVal) + value.AsString(rightVal))
				} else {
					frame.ip = ip
					return vm.runtimeError("Operands must be two numbers or two strings for '+'.")
				}
			case bytecode.OpSubtract, bytecode.OpMultiply, bytecode.OpDivide:
				// Strictly numbers for these
				if !value.IsNumber(leftVal) || !value.IsNumber(rightVal) {
					frame.ip = ip
					opStr := opcode.String()                                              // Get opcode name
					return vm.runtimeError("Operands must be numbers for %s.", opStr[2:]) // Simple way to get op name like Subtract
				}
				leftNum := value.AsNumber(leftVal)
				rightNum := value.AsNumber(rightVal)
				switch opcode {
				case bytecode.OpSubtract:
					registers[destReg] = value.Number(leftNum - rightNum)
				case bytecode.OpMultiply:
					registers[destReg] = value.Number(leftNum * rightNum)
				case bytecode.OpDivide:
					if rightNum == 0 {
						frame.ip = ip
						return vm.runtimeError("Division by zero.")
					}
					registers[destReg] = value.Number(leftNum / rightNum)
				}
			case bytecode.OpEqual, bytecode.OpNotEqual:
				// Use a helper for equality check (handles type differences)
				isEqual := valuesEqual(leftVal, rightVal)
				if opcode == bytecode.OpEqual {
					registers[destReg] = value.Bool(isEqual)
				} else {
					registers[destReg] = value.Bool(!isEqual)
				}
			case bytecode.OpGreater, bytecode.OpLess, bytecode.OpLessEqual:
				// Strictly numbers for comparison for now
				if !value.IsNumber(leftVal) || !value.IsNumber(rightVal) {
					frame.ip = ip
					opStr := opcode.String() // Get opcode name
					return vm.runtimeError("Operands must be numbers for %s.", opStr[2:])
				}
				leftNum := value.AsNumber(leftVal)
				rightNum := value.AsNumber(rightVal)
				switch opcode {
				case bytecode.OpGreater:
					registers[destReg] = value.Bool(leftNum > rightNum)
				case bytecode.OpLess:
					registers[destReg] = value.Bool(leftNum < rightNum)
				case bytecode.OpLessEqual:
					registers[destReg] = value.Bool(leftNum <= rightNum)
				}
			}

		case bytecode.OpCall:
			destReg := code[ip]         // Rx: Where the return value should go (in caller frame)
			funcReg := code[ip+1]       // Ry: Register holding the function/closure to call
			argCount := int(code[ip+2]) // Rz: Number of arguments passed
			ip += 3

			// 1. Get the callable value (Function or Closure)
			calleeVal := registers[funcReg]
			var calleeClosure *value.Closure
			var calleeFunc *bytecode.Function
			var neededRegSlots int

			if value.IsClosure(calleeVal) {
				calleeClosure = value.AsClosure(calleeVal)
				// Assert Fn type
				var fnOk bool
				calleeFunc, fnOk = calleeClosure.Fn.(*bytecode.Function)
				if !fnOk {
					frame.ip = ip
					return vm.runtimeError("Internal VM Error: Closure contains invalid function type.")
				}
			} else if value.IsFunction(calleeVal) {
				// Allow calling plain functions too (like the main script)
				// Wrap it in a temporary closure with no upvalues for consistency in CallFrame
				var fnOk bool
				calleeFunc, fnOk = value.AsFunction(calleeVal).(*bytecode.Function)
				if !fnOk {
					frame.ip = ip
					return vm.runtimeError("Internal VM Error: Function value contains invalid type.")
				}
				calleeClosure = &value.Closure{Fn: calleeFunc, Upvalues: []*value.Upvalue{}}
			} else {
				frame.ip = ip
				return vm.runtimeError("Can only call functions or closures.")
			}
			neededRegSlots = calleeFunc.RegisterSize

			// 2. Check arity
			if argCount != calleeFunc.Arity {
				frame.ip = ip
				return vm.runtimeError("Expected %d arguments but got %d.", calleeFunc.Arity, argCount)
			}

			// 3. Check for stack overflow (frames and registers)
			if vm.frameCount == MaxFrames {
				frame.ip = ip
				return vm.runtimeError("Call stack overflow.")
			}
			if vm.nextRegSlot+neededRegSlots > len(vm.registerStack) {
				frame.ip = ip
				return vm.runtimeError("Register stack overflow during call.")
			}

			// 4. Push new CallFrame
			frame.ip = ip             // Save current IP before switching frame
			returnTargetReg = destReg // Remember where to put the return value in *this* frame

			// Get pointer to the new frame slot BEFORE accessing caller registers
			callerFrameRegisters := registers // Cache caller registers before frame points to new one
			frame = &vm.frames[vm.frameCount] // Point frame to the new frame slot

			frame.closure = calleeClosure // Store the closure being executed
			frame.ip = 0                  // Start at the beginning of the function's code
			frame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+neededRegSlots]
			vm.nextRegSlot += neededRegSlots
			vm.frameCount++

			// 5. Copy arguments into the new frame's parameter registers (R0, R1, ...)
			argStartReg := funcReg + 1
			for i := 0; i < argCount; i++ {
				// Copy from caller frame (which is vm.frameCount-2, but we cached its registers)
				frame.registers[i] = callerFrameRegisters[argStartReg+byte(i)]
			}

			// 6. Update cached variables for the new frame
			closure = frame.closure                       // Update cached closure
			function, _ = closure.Fn.(*bytecode.Function) // Re-assert (already checked ok)
			code = function.Chunk.Code
			constants = function.Chunk.Constants
			registers = frame.registers
			ip = frame.ip

		case bytecode.OpReturn:
			retReg := code[ip]
			ip++

			result := registers[retReg] // Value being returned

			// --- Close upvalues pointing into the returning frame's registers ---
			vm.closeUpvalues(registers) // Pass the frame's register slice

			// --- Pop the frame ---
			frame.ip = ip // Save final IP of the returning frame
			returnFrameRegCount := len(registers)
			vm.frameCount--
			vm.nextRegSlot -= returnFrameRegCount // Reclaim register slots

			if vm.frameCount == 0 {
				// Returned from the top-level script, print the final result
				fmt.Println(result)
				return InterpretOK
			} else {
				// Returning from a function to its caller
				frame = &vm.frames[vm.frameCount-1] // Switch back to caller frame

				// Place the return value into the designated register in the caller frame
				frame.registers[returnTargetReg] = result

				// Update cached variables for the caller frame
				closure = frame.closure // Update cached closure
				function, ok = closure.Fn.(*bytecode.Function)
				if !ok {
					return vm.runtimeError("Internal VM Error: Invalid closure on frame return.")
				}
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
				// Reset returnTargetReg for next potential return?
				// Should be set again by the next OpCall.
			}

		case bytecode.OpReturnUndefined:
			// Get the undefined value
			result := value.Undefined()

			// --- Close upvalues pointing into the returning frame's registers ---
			vm.closeUpvalues(registers)

			// --- Pop the frame ---
			frame.ip = ip // Save final IP (although it's just past OpReturnUndefined)
			returnFrameRegCount := len(registers)
			vm.frameCount--
			vm.nextRegSlot -= returnFrameRegCount // Reclaim register slots

			if vm.frameCount == 0 {
				// Returned from the top-level script
				fmt.Println(result) // Should print "undefined"
				return InterpretOK
			} else {
				// Returning from a function to its caller
				frame = &vm.frames[vm.frameCount-1] // Switch back to caller frame

				// Place the undefined value into the designated register in the caller frame
				// We still need returnTargetReg from the preceding OpCall
				frame.registers[returnTargetReg] = result

				// Update cached variables for the caller frame
				closure = frame.closure
				function, ok = closure.Fn.(*bytecode.Function)
				if !ok {
					return vm.runtimeError("Internal VM Error: Invalid closure on frame return.")
				}
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
			}

		// --- Control Flow Opcodes ---
		case bytecode.OpJumpIfFalse:
			conditionReg := code[ip] // Rx
			offsetHi := code[ip+1]   // Offset (16-bit, signed)
			offsetLo := code[ip+2]
			offset := int16(uint16(offsetHi)<<8 | uint16(offsetLo))
			ip += 3

			conditionValue := registers[conditionReg]
			if isFalsey(conditionValue) {
				ip += int(offset) // Apply the jump offset
			}

		case bytecode.OpJump:
			offsetHi := code[ip] // Offset (16-bit, signed)
			offsetLo := code[ip+1]
			offset := int16(uint16(offsetHi)<<8 | uint16(offsetLo))
			ip += 2

			ip += int(offset) // Apply the jump offset unconditionally

		// --- Closure Opcodes ---
		case bytecode.OpClosure:
			destReg := code[ip]          // Rx
			funcConstIdxHi := code[ip+1] // FuncConstIdx (16-bit)
			funcConstIdxLo := code[ip+2]
			funcConstIdx := uint16(funcConstIdxHi)<<8 | uint16(funcConstIdxLo)
			upvalueCount := int(code[ip+3]) // UpvalueCount
			ip += 4

			// Get the function blueprint from constants
			if int(funcConstIdx) >= len(constants) {
				frame.ip = ip
				return vm.runtimeError("Invalid function constant index %d for OpClosure.", funcConstIdx)
			}
			funcProtoVal := constants[funcConstIdx]
			if !value.IsFunction(funcProtoVal) {
				frame.ip = ip
				return vm.runtimeError("Constant %d is not a function for OpClosure.", funcConstIdx)
			}
			funcProto := value.AsFunction(funcProtoVal).(*bytecode.Function)

			// Create upvalue storage
			upvalues := make([]*value.Upvalue, upvalueCount)

			// Capture upvalues
			for i := 0; i < upvalueCount; i++ {
				isLocal := code[ip] == 1
				index := code[ip+1]
				ip += 2

				if isLocal {
					// Capture local register pointer from the *current* frame
					captureLoc := &registers[index]
					// Check if an existing open upvalue already points here
					found := false
					for _, openUV := range vm.openUpvalues {
						// Important: Compare pointers directly
						if openUV.Location == captureLoc {
							upvalues[i] = openUV
							found = true
							break
						}
					}
					if !found {
						// Create new open upvalue if not found
						newUpvalue := &value.Upvalue{Location: captureLoc}
						upvalues[i] = newUpvalue
						vm.openUpvalues = append(vm.openUpvalues, newUpvalue)
					}
				} else {
					// Capture from the *enclosing* function's upvalues
					if int(index) >= len(closure.Upvalues) { // Use current closure's upvalues
						frame.ip = ip
						return vm.runtimeError("Invalid upvalue index %d for upvalue capture.", index)
					}
					upvalues[i] = closure.Upvalues[index] // Share the upvalue from the enclosing closure
				}
			}

			// Create the closure and store it
			newClosure := value.NewClosure(funcProto, upvalues)
			registers[destReg] = newClosure

		case bytecode.OpLoadFree:
			destReg := code[ip]        // Rx
			upvalueIndex := code[ip+1] // UpvalueIndex
			ip += 2

			if int(upvalueIndex) >= len(closure.Upvalues) {
				frame.ip = ip
				return vm.runtimeError("Invalid upvalue index %d for OpLoadFree.", upvalueIndex)
			}
			upvalue := closure.Upvalues[upvalueIndex]
			// Check if upvalue is closed
			if upvalue.Location == nil {
				registers[destReg] = upvalue.Closed
			} else {
				registers[destReg] = *upvalue.Location // Dereference the pointer to get the value
			}

		case bytecode.OpSetUpvalue:
			upvalueIndex := code[ip] // UpvalueIndex
			srcReg := code[ip+1]     // Ry: Register holding the value to store
			ip += 2

			if int(upvalueIndex) >= len(closure.Upvalues) {
				frame.ip = ip
				return vm.runtimeError("Invalid upvalue index %d for OpSetUpvalue.", upvalueIndex)
			}
			upvalue := closure.Upvalues[upvalueIndex]
			valueToSet := registers[srcReg]
			// Check if upvalue is closed
			if upvalue.Location == nil {
				upvalue.Closed = valueToSet // Assign to closed value
			} else {
				*upvalue.Location = valueToSet // Assign value through the pointer
			}

		default:
			frame.ip = ip // Save IP
			return vm.runtimeError("Unknown opcode %d", opcode)
		}
	}
}

// --- Runtime Helpers ---

// closeUpvalues closes any open upvalues that point to register slots within the memory range of the frame being returned.
func (vm *VM) closeUpvalues(frameRegisters []value.Value) {
	if len(frameRegisters) == 0 {
		return // Nothing to close if frame had no registers
	}
	// Get the memory range of the frame's register slice
	frameStartAddr := uintptr(unsafe.Pointer(&frameRegisters[0]))
	// Address *just after* the last element
	frameEndAddr := uintptr(unsafe.Pointer(&frameRegisters[len(frameRegisters)-1])) + unsafe.Sizeof(value.Value{})

	newOpenUpvalues := vm.openUpvalues[:0] // Reuse slice capacity
	for _, upvalue := range vm.openUpvalues {
		if upvalue.Location != nil { // Only consider open upvalues
			uvLocAddr := uintptr(unsafe.Pointer(upvalue.Location))
			// Check if the upvalue points within the frame's memory range
			if uvLocAddr >= frameStartAddr && uvLocAddr < frameEndAddr {
				// Points into the closing frame -> Close it
				upvalue.Closed = *upvalue.Location // Copy value
				upvalue.Location = nil             // Mark as closed
				// Don't add to newOpenUpvalues
			} else {
				// Points outside the closing frame -> Keep it open
				newOpenUpvalues = append(newOpenUpvalues, upvalue)
			}
		} else {
			// Already closed -> Keep it (shouldn't strictly be in openUpvalues, but handle defensively)
			newOpenUpvalues = append(newOpenUpvalues, upvalue)
		}
	}
	vm.openUpvalues = newOpenUpvalues
}

// runtimeError formats a runtime error message and returns the appropriate result code.
// It also prints the error to stderr.
func (vm *VM) runtimeError(format string, args ...interface{}) InterpretResult {
	// TODO: Include line number if available
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "Runtime Error: %s\n", msg)

	// Optionally print stack trace here

	vm.Reset() // Clear VM state on runtime error
	return InterpretRuntimeError
}

// isFalsey determines the truthiness of a value (null, undefined, and false are falsey).
func isFalsey(v value.Value) bool {
	return value.IsNull(v) || value.IsUndefined(v) || (value.IsBool(v) && !value.AsBool(v))
}

// valuesEqual compares two VM values for strict equality (like ===).
func valuesEqual(a, b value.Value) bool {
	if a.Type != b.Type {
		return false // Strict type equality
	}
	switch a.Type {
	case value.TypeUndefined:
		return true // undefined === undefined
	case value.TypeNull:
		return true // null === null
	case value.TypeBool:
		return value.AsBool(a) == value.AsBool(b)
	case value.TypeNumber:
		return value.AsNumber(a) == value.AsNumber(b)
	case value.TypeString:
		return value.AsString(a) == value.AsString(b)
	// TODO: Add object/function comparison later (likely by reference)
	default:
		return false // Uncomparable types
	}
}

// No more push/pop/peek related to the operand stack
// Register access is direct via frame.registers[index]

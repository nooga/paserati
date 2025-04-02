package vm

import (
	"fmt"
	"os"
	"unsafe"
)

const RegFileSize = 256 // Max registers per function call frame
const MaxFrames = 64    // Max call stack depth

// CallFrame represents a single active function call.
type CallFrame struct {
	closure *Closure // Closure being executed (contains Function and Upvalues)
	ip      int      // Instruction pointer *within* this frame's closure.Fn.Chunk.Code
	// `registers` is a slice pointing into the VM's main register stack,
	// defining the window for this frame.
	registers      []Value
	targetRegister byte // Which register in the CALLER the result should go into
}

// VM represents the virtual machine state.
type VM struct {
	// The call stack
	frames     [MaxFrames]CallFrame
	frameCount int

	// Register file, treated as a stack. Each CallFrame gets a window into this.
	// This avoids reallocating register arrays for every call.
	registerStack [RegFileSize * MaxFrames]Value
	nextRegSlot   int // Points to the next available slot in registerStack

	// List of upvalues pointing to variables still on the registerStack
	openUpvalues []*Upvalue
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
		openUpvalues: make([]*Upvalue, 0, 16), // Pre-allocate slightly
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
func (vm *VM) Interpret(chunk *Chunk) InterpretResult {
	vm.Reset()

	// Wrap the main script chunk in a dummy function and closure
	mainFunc := &Function{Chunk: chunk, Name: "<script>", RegisterSize: RegFileSize} // Assume max regs for script
	// Create closure using the NewClosure constructor which returns a Value
	// We store the *Closure pointer in the CallFrame, not the Value itself.
	// So, we need the *Closure part of the value returned by NewClosure.
	// Let's adjust NewClosure slightly or handle it here.
	// FOR NOW: Manually create the closure struct here.
	mainClosure := &Closure{Fn: mainFunc, Upvalues: []*Upvalue{}} // No upvalues for main

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
	// We now directly access the *Function pointer
	function := closure.Fn
	if function == nil { // Check if the function pointer itself is nil
		return vm.runtimeError("Internal VM Error: Closure contains a nil function pointer.")
	}
	if function.Chunk == nil { // Check if the chunk within the function is nil
		return vm.runtimeError("Internal VM Error: Function associated with closure has a nil chunk.")
	}
	code := function.Chunk.Code
	constants := function.Chunk.Constants
	registers := frame.registers // This is the frame's register window
	ip := frame.ip

	// Store the target register for the *caller* when returning from a function
	// var returnTargetReg byte = 0 // REMOVED - Now stored in CallFrame

	for {
		// --- Debugging (Optional) ---
		// fmt.Printf("ip=%04d regs=")
		// for i := 0; i < 10; i++ { // Print first few regs
		// 	if i < len(frame.registers) {
		// 		fmt.Printf("[R%d:%s] ", i, frame.registers[i])
		// 	} else { break }
		// }
		// fmt.Println()
		// frame.closure.Fn.Chunk.DisassembleInstruction(ip) // Adjusted for new structure
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

		opcode := OpCode(code[ip]) // Use local OpCode
		ip++                       // Advance IP past the opcode itself

		switch opcode {
		case OpLoadConst: // Use local OpCode constant
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

		case OpLoadNull:
			reg := code[ip]
			ip++
			registers[reg] = Null() // Use local Null()

		case OpLoadUndefined:
			reg := code[ip]
			ip++
			registers[reg] = Undefined() // Use local Undefined()

		case OpLoadTrue:
			reg := code[ip]
			ip++
			registers[reg] = Bool(true) // Use local Bool()

		case OpLoadFalse:
			reg := code[ip]
			ip++
			registers[reg] = Bool(false) // Use local Bool()

		case OpMove:
			regDest := code[ip]
			regSrc := code[ip+1]
			ip += 2
			registers[regDest] = registers[regSrc]

		case OpNegate:
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2
			srcVal := registers[srcReg]
			if !IsNumber(srcVal) { // Use local IsNumber
				frame.ip = ip
				return vm.runtimeError("Operand must be a number for negation.")
			}
			registers[destReg] = Number(-AsNumber(srcVal)) // Use local Number/AsNumber

		case OpNot:
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2
			srcVal := registers[srcReg]
			// In many languages, ! evaluates truthiness
			registers[destReg] = Bool(isFalsey(srcVal)) // Use local Bool

		case OpAdd, OpSubtract, OpMultiply, OpDivide,
			OpEqual, OpNotEqual, OpStrictEqual, OpStrictNotEqual,
			OpGreater, OpLess, OpLessEqual:
			destReg := code[ip]
			leftReg := code[ip+1]
			rightReg := code[ip+2]
			ip += 3
			leftVal := registers[leftReg]
			rightVal := registers[rightReg]

			// Type checking specific to operation groups
			switch opcode {
			case OpAdd:
				// Allow string concatenation or number addition
				if IsNumber(leftVal) && IsNumber(rightVal) {
					registers[destReg] = Number(AsNumber(leftVal) + AsNumber(rightVal))
				} else if IsString(leftVal) && IsString(rightVal) {
					// Consider performance of string concat later
					registers[destReg] = String(AsString(leftVal) + AsString(rightVal))
				} else {
					frame.ip = ip
					return vm.runtimeError("Operands must be two numbers or two strings for '+'.")
				}
			case OpSubtract, OpMultiply, OpDivide:
				// Strictly numbers for these
				if !IsNumber(leftVal) || !IsNumber(rightVal) {
					frame.ip = ip
					opStr := opcode.String()                                              // Get opcode name
					return vm.runtimeError("Operands must be numbers for %s.", opStr[2:]) // Simple way to get op name like Subtract
				}
				leftNum := AsNumber(leftVal)
				rightNum := AsNumber(rightVal)
				switch opcode {
				case OpSubtract:
					registers[destReg] = Number(leftNum - rightNum)
				case OpMultiply:
					registers[destReg] = Number(leftNum * rightNum)
				case OpDivide:
					if rightNum == 0 {
						frame.ip = ip
						return vm.runtimeError("Division by zero.")
					}
					registers[destReg] = Number(leftNum / rightNum)
				}
			case OpEqual, OpNotEqual:
				// Use a helper for equality check (handles type differences)
				isEqual := valuesEqual(leftVal, rightVal)
				if opcode == OpEqual {
					registers[destReg] = Bool(isEqual)
				} else {
					registers[destReg] = Bool(!isEqual)
				}
			case OpStrictEqual, OpStrictNotEqual: // Added cases
				isStrictlyEqual := valuesStrictEqual(leftVal, rightVal)
				if opcode == OpStrictEqual {
					registers[destReg] = Bool(isStrictlyEqual)
				} else { // OpStrictNotEqual
					registers[destReg] = Bool(!isStrictlyEqual)
				}
			case OpGreater, OpLess, OpLessEqual:
				// Strictly numbers for comparison
				if !IsNumber(leftVal) || !IsNumber(rightVal) {
					frame.ip = ip
					opStr := opcode.String() // Get opcode name
					return vm.runtimeError("Operands must be numbers for comparison (%s).", opStr[2:])
				}
				leftNum := AsNumber(leftVal)
				rightNum := AsNumber(rightVal)
				var result bool
				switch opcode {
				case OpGreater:
					result = leftNum > rightNum
				case OpLess:
					result = leftNum < rightNum
				case OpLessEqual:
					result = leftNum <= rightNum
				}
				registers[destReg] = Bool(result)
			}

		case OpJump:
			offsetHi := code[ip]
			offsetLo := code[ip+1]
			ip += 2
			offset := int16(uint16(offsetHi)<<8 | uint16(offsetLo))
			ip += int(offset) // Apply jump relative to IP *after* reading offset bytes

		case OpJumpIfFalse:
			condReg := code[ip]
			offsetHi := code[ip+1]
			offsetLo := code[ip+2]
			ip += 3
			if isFalsey(registers[condReg]) {
				offset := int16(uint16(offsetHi)<<8 | uint16(offsetLo))
				ip += int(offset) // Apply jump relative to IP *after* reading offset bytes
			}

		case OpCall:
			destReg := code[ip]         // Where the result should go
			funcReg := code[ip+1]       // Register holding the function/closure to call
			argCount := int(code[ip+2]) // Number of arguments provided
			ip += 3

			calleeVal := registers[funcReg]
			var calleeClosure *Closure

			switch calleeVal.Type {
			case TypeClosure:
				calleeClosure = AsClosure(calleeVal) // Use local AsClosure
			case TypeFunction:
				// Allow calling plain functions directly (implicitly creating a closure with no upvalues)
				// This is useful for top-level functions that don't close over anything.
				funcToCall := AsFunction(calleeVal) // Use local AsFunction
				// Create a temporary closure on the fly
				calleeClosure = &Closure{Fn: funcToCall, Upvalues: []*Upvalue{}} // Empty slice is okay
			default:
				frame.ip = ip
				return vm.runtimeError("Can only call functions and closures.")
			}

			calleeFunc := calleeClosure.Fn

			if argCount != calleeFunc.Arity {
				frame.ip = ip
				return vm.runtimeError("Expected %d arguments but got %d.", calleeFunc.Arity, argCount)
			}

			if vm.frameCount == MaxFrames {
				frame.ip = ip
				return vm.runtimeError("Stack overflow.")
			}

			// --- Setup New Frame ---
			// Check if enough space in the global register stack
			requiredRegs := calleeFunc.RegisterSize
			if vm.nextRegSlot+requiredRegs > len(vm.registerStack) {
				// TODO: Implement garbage collection or stack resizing?
				return vm.runtimeError("Register stack overflow during call.")
			}

			// !! Store caller registers *before* getting pointer to new frame slot !!
			callerRegisters := registers

			// Save current IP into the current (soon-to-be caller) frame
			frame.ip = ip

			// Get pointer to the new frame slot
			newFrame := &vm.frames[vm.frameCount]
			newFrame.closure = calleeClosure
			newFrame.ip = 0                   // Start at the beginning of the called function's code
			newFrame.targetRegister = destReg // Store where the return value should go in the CALLER

			// Allocate register window for the new frame
			newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
			vm.nextRegSlot += requiredRegs

			// --- Copy Arguments ---
			// Arguments are typically in registers immediately following the function register in the *caller's* frame.
			argStartRegInCaller := funcReg + 1
			for i := 0; i < argCount; i++ {
				// Copy from callerRegisters into the new frame's first registers (R0, R1, ...)
				if i < len(newFrame.registers) && int(argStartRegInCaller)+i < len(callerRegisters) {
					newFrame.registers[i] = callerRegisters[argStartRegInCaller+byte(i)]
				} else {
					// Bounds check error - should ideally not happen if compiler is correct
					return vm.runtimeError("Internal Error: Argument register index out of bounds during call setup.")
				}
			}

			vm.frameCount++

			// --- Switch Context --- (Update cached variables)
			frame = newFrame // Current frame is now the new frame
			closure = frame.closure
			function = closure.Fn
			code = function.Chunk.Code
			constants = function.Chunk.Constants
			registers = frame.registers
			ip = frame.ip
			// --- Context Switch Complete ---

		case OpReturn:
			srcReg := code[ip]
			ip++
			result := registers[srcReg]

			// Save frame pointer and register slice before popping
			returnFrame := frame
			frameRegs := returnFrame.registers

			// Close any upvalues that point into the returning frame's registers
			// vm.closeUpvalues(frameRegs) // Pass the register slice
			vm.closeUpvalues(returnFrame.registers)

			// Pop the current frame
			vm.frameCount--
			vm.nextRegSlot -= len(frameRegs) // Reclaim register space

			if vm.frameCount == 0 {
				// Returning from the top-level script
				// Print the result to standard output for testing/REPL purposes
				fmt.Println(result.String()) // Use String() method for proper output
				return InterpretOK
			}

			// --- Switch Context Back to Caller ---
			frame = &vm.frames[vm.frameCount-1]
			closure = frame.closure
			function = closure.Fn
			code = function.Chunk.Code
			constants = function.Chunk.Constants
			registers = frame.registers
			ip = frame.ip // Restore caller's IP

			// Store the result in the caller's target register
			registers[returnFrame.targetRegister] = result
			// --- Context Switch Complete ---

		case OpReturnUndefined:
			result := Undefined() // Implicit return value
			returnFrame := frame
			frameRegs := returnFrame.registers

			// Close upvalues before popping the frame
			vm.closeUpvalues(frameRegs)

			vm.frameCount--
			vm.nextRegSlot -= len(frameRegs)

			if vm.frameCount == 0 {
				// Print the result to standard output for testing/REPL purposes
				fmt.Println(result.String()) // Use String() method for proper output
				return InterpretOK           // Return from top-level script
			}

			frame = &vm.frames[vm.frameCount-1]
			closure = frame.closure
			function = closure.Fn
			code = function.Chunk.Code
			constants = function.Chunk.Constants
			registers = frame.registers
			ip = frame.ip

			registers[returnFrame.targetRegister] = result

		case OpClosure:
			destReg := code[ip]
			funcConstIdxHi := code[ip+1]
			funcConstIdxLo := code[ip+2]
			funcConstIdx := uint16(funcConstIdxHi)<<8 | uint16(funcConstIdxLo)
			upvalueCount := int(code[ip+3])
			ip += 4

			if int(funcConstIdx) >= len(constants) {
				frame.ip = ip
				return vm.runtimeError("Invalid function constant index %d for closure.", funcConstIdx)
			}
			protoVal := constants[funcConstIdx]
			if !IsFunction(protoVal) {
				frame.ip = ip
				return vm.runtimeError("Constant %d is not a function, cannot create closure.", funcConstIdx)
			}
			protoFunc := AsFunction(protoVal)

			// Allocate upvalue pointers slice
			upvalues := make([]*Upvalue, upvalueCount)
			for i := 0; i < upvalueCount; i++ {
				isLocal := code[ip] == 1
				index := int(code[ip+1])
				ip += 2

				if isLocal {
					// Capture local variable from the *current* frame's registers.
					// The location is index bytes *relative to the start of the current frame's registers*.
					if index >= len(registers) {
						frame.ip = ip
						return vm.runtimeError("Invalid local register index %d for upvalue capture.", index)
					}
					// Pass pointer to the stack slot (register) itself.
					location := &registers[index]
					upvalues[i] = vm.captureUpvalue(location)
				} else {
					// Capture upvalue from the *enclosing* function (i.e., the current closure).
					if closure == nil || index >= len(closure.Upvalues) {
						frame.ip = ip
						return vm.runtimeError("Invalid upvalue index %d for capture.", index)
					}
					upvalues[i] = closure.Upvalues[index]
				}
			}

			// Create the closure Value using the constructor
			// newClosureVal := NewClosure(protoFunc, upvalues)
			// Create the closure struct directly
			newClosure := &Closure{
				Fn:       protoFunc,
				Upvalues: upvalues,
			}
			registers[destReg] = ClosureV(newClosure) // Use ClosureV to create the value

		case OpLoadFree:
			destReg := code[ip]
			upvalueIndex := int(code[ip+1])
			ip += 2

			if closure == nil || upvalueIndex >= len(closure.Upvalues) {
				frame.ip = ip
				return vm.runtimeError("Invalid upvalue index %d for OpLoadFree.", upvalueIndex)
			}
			upvalue := closure.Upvalues[upvalueIndex]
			if upvalue.Location != nil {
				// Variable is still open (on the stack)
				registers[destReg] = *upvalue.Location // Dereference pointer to get value
			} else {
				// Variable is closed
				registers[destReg] = upvalue.Closed
			}

		case OpSetUpvalue:
			upvalueIndex := int(code[ip])
			srcReg := code[ip+1]
			ip += 2
			valueToStore := registers[srcReg]

			if closure == nil || upvalueIndex >= len(closure.Upvalues) {
				frame.ip = ip
				return vm.runtimeError("Invalid upvalue index %d for OpSetUpvalue.", upvalueIndex)
			}
			upvalue := closure.Upvalues[upvalueIndex]
			if upvalue.Location != nil {
				// Variable is still open (on the stack), update the stack slot
				*upvalue.Location = valueToStore // Update value via pointer
			} else {
				// Variable is closed, update the Closed field
				upvalue.Closed = valueToStore
			}

		// --- NEW: Array Opcodes ---
		case OpMakeArray:
			destReg := code[ip]
			startReg := code[ip+1]
			count := int(code[ip+2])
			ip += 3

			// Create a new slice and copy elements from registers
			elements := make([]Value, count)
			startIdx := int(startReg)
			endIdx := startIdx + count

			// Bounds check for register access
			if startIdx < 0 || endIdx > len(registers) {
				frame.ip = ip
				return vm.runtimeError("Internal Error: Register index out of bounds during array creation (start=%d, count=%d, frame size=%d)", startIdx, count, len(registers))
			}

			copy(elements, registers[startIdx:endIdx])

			// Create the array value
			arrayValue := NewArray(elements)
			registers[destReg] = arrayValue

		case OpGetIndex:
			destReg := code[ip]
			arrayReg := code[ip+1]
			indexReg := code[ip+2]
			ip += 3

			arrayVal := registers[arrayReg]
			indexVal := registers[indexReg]

			if !IsArray(arrayVal) {
				frame.ip = ip
				return vm.runtimeError("Cannot index non-array type '%v'", arrayVal.Type)
			}
			if !IsNumber(indexVal) {
				frame.ip = ip
				return vm.runtimeError("Array index must be a number, got '%v'", indexVal.Type)
			}

			arr := AsArray(arrayVal)
			idx := int(AsNumber(indexVal)) // TODO: Handle non-integer indices?

			// Bounds check
			if idx < 0 || idx >= len(arr.Elements) {
				frame.ip = ip
				// Return undefined for out-of-bounds access, like JS?
				// Or throw runtime error? Let's return undefined for now.
				registers[destReg] = Undefined()
			} else {
				registers[destReg] = arr.Elements[idx]
			}

		case OpSetIndex:
			arrayReg := code[ip]
			indexReg := code[ip+1]
			valueReg := code[ip+2]
			ip += 3

			arrayVal := registers[arrayReg]
			indexVal := registers[indexReg]
			valueVal := registers[valueReg]

			if !IsArray(arrayVal) {
				frame.ip = ip
				return vm.runtimeError("Cannot set index on non-array type '%v'", arrayVal.Type)
			}
			if !IsNumber(indexVal) {
				frame.ip = ip
				return vm.runtimeError("Array index must be a number, got '%v'", indexVal.Type)
			}

			arr := AsArray(arrayVal)
			idx := int(AsNumber(indexVal)) // TODO: Handle non-integer indices? Handle potential float truncation?

			// --- NEW: Handle Array Expansion ---
			if idx < 0 {
				frame.ip = ip
				// Negative indices are invalid
				return vm.runtimeError("Array index cannot be negative, got %d", idx)
			} else if idx < len(arr.Elements) {
				// Index is within current bounds: Overwrite existing element
				arr.Elements[idx] = valueVal
			} else if idx == len(arr.Elements) {
				// Index is exactly at the end: Append the new element
				arr.Elements = append(arr.Elements, valueVal)
			} else { // idx > len(arr.Elements)
				// Index is beyond the end: Expand array and then append
				neededCapacity := idx + 1
				if cap(arr.Elements) < neededCapacity {
					// Reallocate with enough capacity if needed
					newElements := make([]Value, len(arr.Elements), neededCapacity)
					copy(newElements, arr.Elements)
					arr.Elements = newElements
				}
				// Fill the gap with Undefined values
				for i := len(arr.Elements); i < idx; i++ {
					arr.Elements = append(arr.Elements, Undefined())
				}
				// Append the actual value at the target index
				arr.Elements = append(arr.Elements, valueVal)
			}
			// --- END NEW ---

			// OpSetIndex itself doesn't produce a result register, the assignment expression does (valueReg)

		// --- End Array Opcodes ---

		// --- NEW: Get Length Opcode ---
		case OpGetLength:
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2

			srcVal := registers[srcReg]
			var length float64 = -1 // Initialize to -1 to indicate error if type is wrong

			switch srcVal.Type {
			case TypeArray:
				arr := AsArray(srcVal)
				length = float64(len(arr.Elements))
			case TypeString:
				str := AsString(srcVal)
				// Use rune count for string length to handle multi-byte chars correctly
				length = float64(len(str))
			default:
				frame.ip = ip
				return vm.runtimeError("Cannot get length of type '%v'", srcVal.Type)
			}

			registers[destReg] = Number(length)
		// --- END NEW ---

		default:
			frame.ip = ip // Save IP before erroring
			return vm.runtimeError("Unknown opcode %d encountered.", opcode)
		}
	}

	// Unreachable, but keeps the compiler happy
	// return InterpretRuntimeError
}

// captureUpvalue creates a new Upvalue object for a local variable at the given stack location.
// It checks if an upvalue for this location already exists in the openUpvalues list.
func (vm *VM) captureUpvalue(location *Value) *Upvalue {
	// Search from the end because upvalues for locals higher in the stack
	// are likely to be found sooner (LIFO behavior of stack frames).
	for i := len(vm.openUpvalues) - 1; i >= 0; i-- {
		upvalue := vm.openUpvalues[i]
		// Compare pointers directly to see if it's the same stack slot
		if upvalue.Location == location {
			return upvalue // Found existing open upvalue
		}
		// Optimization: If the current upvalue's location is below the target location
		// on the stack, we won't find the target location later in the list (assuming
		// openUpvalues is sorted or locals are captured in order).
		// Requires careful management or unsafe.Pointer comparison.
		// Let's skip this optimization for now for clarity.
		// if uintptr(unsafe.Pointer(upvalue.Location)) < uintptr(unsafe.Pointer(location)) {
		//     break
		// }
	}

	// If not found, create a new one
	newUpvalue := &Upvalue{Location: location} // Closed field is zero-value (Undefined)
	vm.openUpvalues = append(vm.openUpvalues, newUpvalue)
	return newUpvalue
}

// closeUpvalues closes all open upvalues that point to stack slots within the given
// frame's registers (which are about to become invalid).
// It takes the slice representing the frame's registers as input.
func (vm *VM) closeUpvalues(frameRegisters []Value) {
	if len(frameRegisters) == 0 || len(vm.openUpvalues) == 0 {
		return // Nothing to close or no registers in frame
	}

	// Get the memory address range of the frame's register slice.
	// This is somewhat fragile if the underlying array is reallocated,
	// but should be okay as registerStack has fixed size.
	frameStartPtr := uintptr(unsafe.Pointer(&frameRegisters[0]))
	// Address of one past the last element
	frameEndPtr := frameStartPtr + uintptr(len(frameRegisters))*unsafe.Sizeof(Value{})

	// Iterate through openUpvalues and close those pointing into the frame.
	// We also filter the openUpvalues list, removing the closed ones.
	newOpenUpvalues := vm.openUpvalues[:0] // Reuse underlying array
	for _, upvalue := range vm.openUpvalues {
		if upvalue.Location == nil { // Skip already closed upvalues
			continue
		}
		upvaluePtr := uintptr(unsafe.Pointer(upvalue.Location))
		// Check if the upvalue's location points within the memory range of frameRegisters
		if upvaluePtr >= frameStartPtr && upvaluePtr < frameEndPtr {
			// This upvalue points into the frame being popped, close it.
			closedValue := *upvalue.Location // Copy the value from the stack
			upvalue.Close(closedValue)       // Update the upvalue object
			// Do NOT add it back to newOpenUpvalues
		} else {
			// This upvalue points elsewhere (e.g., higher up the stack), keep it open.
			newOpenUpvalues = append(newOpenUpvalues, upvalue)
		}
	}
	vm.openUpvalues = newOpenUpvalues
}

// runtimeError prints an error message and the stack trace.
func (vm *VM) runtimeError(format string, args ...interface{}) InterpretResult {
	fmt.Fprintf(os.Stderr, "Runtime Error: %s\n", fmt.Sprintf(format, args...))

	// Print stack trace
	for i := vm.frameCount - 1; i >= 0; i-- {
		frame := &vm.frames[i]
		closure := frame.closure
		function := closure.Fn

		// Calculate the line number for the current instruction pointer (ip)
		// This requires the line information in the Chunk, which we don't store per-instruction yet.
		// For now, just print the function name and IP offset.
		line := -1 // Placeholder
		if function.Chunk != nil && frame.ip > 0 && frame.ip <= len(function.Chunk.Code) {
			// Need a mapping from instruction offset to line number.
			// Placeholder: Find the line corresponding to the *previous* instruction.
			// Assuming frame.ip points to the *next* instruction to be executed.
			// This requires the `Lines` array in the Chunk to be populated correctly during compilation.
			// if len(function.Chunk.Lines) > 0 { ... }
		}

		funcName := function.Name
		if funcName == "" {
			funcName = "<script>"
		}
		fmt.Fprintf(os.Stderr, "[line %d] in %s (ip: %d)\n", line, funcName, frame.ip-1) // Show IP of failed instruction
	}

	vm.Reset() // Reset VM state after error
	return InterpretRuntimeError
}

// valuesEqual compares two values for equality (loose comparison like ==).
// Already defined in value.go - REMOVING DUPLICATE
// func valuesEqual(a, b Value) bool { ... }

// valuesStrictEqual compares two values for strict equality (like ===).
func valuesStrictEqual(a, b Value) bool {
	if a.Type != b.Type {
		return false // Different types are never strictly equal
	}

	// If types are the same, compare values based on type
	switch a.Type {
	case TypeNull:
		return true // null === null
	case TypeUndefined:
		return true // undefined === undefined
	case TypeBool:
		return AsBool(a) == AsBool(b)
	case TypeNumber:
		return AsNumber(a) == AsNumber(b)
	case TypeString:
		return AsString(a) == AsString(b)
	case TypeFunction: // Compare function pointers for identity
		return AsFunction(a) == AsFunction(b)
	case TypeClosure: // Compare closure pointers for identity
		return AsClosure(a) == AsClosure(b)
	default:
		// Should not happen for valid types
		return false
	}
}

// isFalsey determines the truthiness of a value.
// Already defined in value.go - REMOVING DUPLICATE
// func isFalsey(value Value) bool { ... }

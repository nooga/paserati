package vm

import (
	"fmt"
	"math"
	"os"
	"paserati/pkg/errors"
	"strconv"
	"unicode/utf8"
	"unsafe"
)

const RegFileSize = 256 // Max registers per function call frame
const MaxFrames = 64    // Max call stack depth

// CallFrame represents a single active function call.
type CallFrame struct {
	// closure is the current runtime closure (user-level ClosureObject)
	closure *ClosureObject // ClosureObject being executed (contains FunctionObject and Upvalues)
	ip      int            // Instruction pointer *within* this frame's closure.Fn.Chunk.Code
	// `registers` is a slice pointing into the VM's main register stack,
	// defining the window for this frame.
	registers         []Value
	targetRegister    byte  // Which register in the CALLER the result should go into
	thisValue         Value // The 'this' value for method calls (undefined for regular function calls)
	isConstructorCall bool  // Whether this frame was created by a constructor call (new expression)
}

// PropCacheState represents the different states of inline cache
type PropCacheState uint8

const (
	CacheStateUninitialized PropCacheState = iota
	CacheStateMonomorphic                  // Single shape cached
	CacheStatePolymorphic                  // Multiple shapes cached (up to 4)
	CacheStateMegamorphic                  // Too many shapes, fallback to map lookup
)

// PropCacheEntry represents a single shape+offset entry in the cache
type PropCacheEntry struct {
	shape  *Shape // The shape this cache entry is valid for
	offset int    // The property offset in the object's properties slice
}

// PropInlineCache represents the inline cache for a property access site
type PropInlineCache struct {
	state      PropCacheState
	entries    [4]PropCacheEntry // Support up to 4 shapes (polymorphic)
	entryCount int               // Number of active entries
	hitCount   uint32            // For debugging/metrics
	missCount  uint32            // For debugging/metrics
}

// ICacheStats holds statistics about inline cache performance
type ICacheStats struct {
	totalHits       uint64
	totalMisses     uint64
	monomorphicHits uint64
	polymorphicHits uint64
	megamorphicHits uint64
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

	// Enhanced inline cache for property access (maps instruction pointer to cache)
	propCache map[int]*PropInlineCache

	// Cache statistics for debugging/profiling
	cacheStats ICacheStats

	// Global variables table (indexed by global variable index for performance)
	globals []Value
	// Global variable names (parallel array for debugging, maps index to name)
	globalNames []string

	// Globals, open upvalues, etc. would go here later
	errors []errors.PaseratiError
}

// lookupInCache performs a property lookup using the inline cache
func (ic *PropInlineCache) lookupInCache(shape *Shape) (int, bool) {
	switch ic.state {
	case CacheStateUninitialized:
		return -1, false
	case CacheStateMonomorphic:
		if ic.entries[0].shape == shape {
			ic.hitCount++
			return ic.entries[0].offset, true
		}
		ic.missCount++
		return -1, false
	case CacheStatePolymorphic:
		for i := 0; i < ic.entryCount; i++ {
			if ic.entries[i].shape == shape {
				ic.hitCount++
				// Move hit entry to front for better cache locality
				if i > 0 {
					entry := ic.entries[i]
					copy(ic.entries[1:i+1], ic.entries[0:i])
					ic.entries[0] = entry
				}
				return ic.entries[0].offset, true
			}
		}
		ic.missCount++
		return -1, false
	case CacheStateMegamorphic:
		// Always miss in megamorphic state - forces full lookup
		ic.missCount++
		return -1, false
	}
	return -1, false
}

// updateCache updates the inline cache with a new shape+offset entry
func (ic *PropInlineCache) updateCache(shape *Shape, offset int) {
	switch ic.state {
	case CacheStateUninitialized:
		// First entry - transition to monomorphic
		ic.state = CacheStateMonomorphic
		ic.entries[0] = PropCacheEntry{shape: shape, offset: offset}
		ic.entryCount = 1
	case CacheStateMonomorphic:
		// Check if it's the same shape (update offset)
		if ic.entries[0].shape == shape {
			ic.entries[0].offset = offset
			return
		}
		// Different shape - transition to polymorphic
		ic.state = CacheStatePolymorphic
		ic.entries[1] = PropCacheEntry{shape: shape, offset: offset}
		ic.entryCount = 2
	case CacheStatePolymorphic:
		// Check if shape already exists
		for i := 0; i < ic.entryCount; i++ {
			if ic.entries[i].shape == shape {
				ic.entries[i].offset = offset
				return
			}
		}
		// New shape
		if ic.entryCount < 4 {
			ic.entries[ic.entryCount] = PropCacheEntry{shape: shape, offset: offset}
			ic.entryCount++
		} else {
			// Too many shapes - transition to megamorphic
			ic.state = CacheStateMegamorphic
			ic.entryCount = 0
		}
	case CacheStateMegamorphic:
		// Don't cache in megamorphic state
		return
	}
}

// resetCache clears the inline cache (used when shapes change)
func (ic *PropInlineCache) resetCache() {
	ic.state = CacheStateUninitialized
	ic.entryCount = 0
	// Don't reset hit/miss counts for debugging
}

// GetCacheStats returns the current inline cache statistics
func (vm *VM) GetCacheStats() ICacheStats {
	return vm.cacheStats
}

// PrintCacheStats prints detailed cache performance information for debugging
func (vm *VM) PrintCacheStats() {
	stats := vm.cacheStats
	total := stats.totalHits + stats.totalMisses
	if total == 0 {
		fmt.Printf("IC Stats: No cache activity\n")
		return
	}

	hitRate := float64(stats.totalHits) / float64(total) * 100.0
	fmt.Printf("IC Stats: Total: %d, Hits: %d (%.1f%%), Misses: %d\n",
		total, stats.totalHits, hitRate, stats.totalMisses)
	fmt.Printf("  Monomorphic: %d, Polymorphic: %d, Megamorphic: %d\n",
		stats.monomorphicHits, stats.polymorphicHits, stats.megamorphicHits)

	// Print per-site cache information
	fmt.Printf("  Cache sites: %d\n", len(vm.propCache))
	for ip, cache := range vm.propCache {
		stateStr := "UNINITIALIZED"
		switch cache.state {
		case CacheStateMonomorphic:
			stateStr = "MONOMORPHIC"
		case CacheStatePolymorphic:
			stateStr = fmt.Sprintf("POLYMORPHIC(%d)", cache.entryCount)
		case CacheStateMegamorphic:
			stateStr = "MEGAMORPHIC"
		}
		fmt.Printf("    IP %d: %s (hits: %d, misses: %d)\n",
			ip, stateStr, cache.hitCount, cache.missCount)
	}
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
		openUpvalues: make([]*Upvalue, 0, 16),         // Pre-allocate slightly
		propCache:    make(map[int]*PropInlineCache),  // Initialize inline cache
		cacheStats:   ICacheStats{},                   // Initialize cache statistics
		globals:      make([]Value, 0),                // Initialize global variables table
		globalNames:  make([]string, 0),               // Initialize global variable names
		errors:       make([]errors.PaseratiError, 0), // Initialize error list
	}
}

// Reset clears the VM state, ready for new execution.
func (vm *VM) Reset() {
	vm.frameCount = 0
	vm.nextRegSlot = 0
	vm.openUpvalues = vm.openUpvalues[:0] // Clear slice while keeping capacity
	vm.errors = vm.errors[:0]             // Clear errors slice
	// Clear inline cache
	for k := range vm.propCache {
		delete(vm.propCache, k)
	}
	// Reset cache statistics
	vm.cacheStats = ICacheStats{}
	// Clear global variables
	vm.globals = make([]Value, 0)
	vm.globalNames = make([]string, 0)
	// No need to clear registerStack explicitly, slots will be overwritten.
}

// Interpret starts executing the given chunk of bytecode.
// It sets up a new top-level frame for the chunk's execution
// on top of the existing VM state.
// It does NOT reset the VM state; call Reset() explicitly if needed.
// Returns the final value produced by the chunk and any runtime errors.
func (vm *VM) Interpret(chunk *Chunk) (Value, []errors.PaseratiError) {
	// vm.Reset() // REMOVED: Reset should be handled externally for persistent sessions.

	// Clear errors from previous interpretations within the same VM instance, if any.
	vm.errors = vm.errors[:0]

	// --- Sanity Check: Ensure enough stack space BEFORE pushing frame ---
	// We need space for the new frame in frames array and registers in registerStack.
	if vm.frameCount >= MaxFrames {
		// Cannot add another frame.
		placeholderToken := errors.Position{Line: 0, Column: 0} // TODO: Better position?
		runtimeErr := &errors.RuntimeError{
			Position: placeholderToken,
			Msg:      "Stack overflow (cannot push initial script frame)",
		}
		vm.errors = append(vm.errors, runtimeErr)
		return Undefined, vm.errors
	}

	// Wrap the main script chunk in a dummy function and closure
	// Use a reasonable default register size for the script body.
	// TODO: Should the compiler determine the required registers for the top level?
	scriptRegSize := RegFileSize // Default to max for now
	// Wrap the main script chunk in a dummy FunctionObject and ClosureObject
	mainFuncObj := &FunctionObject{
		Arity:        0,
		Variadic:     false,
		Chunk:        chunk,
		Name:         "<script>",
		UpvalueCount: 0,
		RegisterSize: scriptRegSize,
	}
	mainClosureObj := &ClosureObject{Fn: mainFuncObj, Upvalues: []*Upvalue{}}

	// Check if enough space in the global register stack for this new frame
	if vm.nextRegSlot+scriptRegSize > len(vm.registerStack) {
		placeholderToken := errors.Position{Line: 0, Column: 0} // TODO: Better position?
		runtimeErr := &errors.RuntimeError{
			Position: placeholderToken,
			Msg:      fmt.Sprintf("Register stack overflow (needed %d, available %d)", scriptRegSize, len(vm.registerStack)-vm.nextRegSlot),
		}
		vm.errors = append(vm.errors, runtimeErr)
		return Undefined, vm.errors
	}

	// --- Push the new frame ---
	frame := &vm.frames[vm.frameCount] // Get pointer to the frame slot
	// Initialize the first frame to run the mainClosureObj
	frame.closure = mainClosureObj
	frame.ip = 0
	frame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+scriptRegSize]
	frame.targetRegister = 0    // Result of script isn't stored in caller's reg
	frame.thisValue = Undefined // Main script has no 'this' context
	vm.nextRegSlot += scriptRegSize
	vm.frameCount++

	// Run the VM
	resultStatus, finalValue := vm.run() // Capture both status and value

	if resultStatus == InterpretRuntimeError {
		// An error occurred, return the potentially partial value and the collected errors
		return finalValue, vm.errors
	} else {
		// Execution finished without runtime error (InterpretOK)
		// Return the final value returned by run() and empty errors slice (errors were cleared)
		return finalValue, vm.errors // vm.errors should be empty here if InterpretOK
	}
}

// run is the main execution loop.
// It now returns the InterpretResult status AND the final script Value.
func (vm *VM) run() (InterpretResult, Value) {
	// --- Caching frame variables ---
	if vm.frameCount == 0 {
		return InterpretOK, Undefined // Nothing to run
	}
	frame := &vm.frames[vm.frameCount-1]
	// Get function/chunk/constants FROM the closure in the frame
	closure := frame.closure
	// We now directly access the *Function pointer
	function := closure.Fn
	if function == nil { // Check if the function pointer itself is nil
		// runtimeError now collects the error and returns the enum
		// Need to return a default value along with the error status
		status := vm.runtimeError("Internal VM Error: Closure contains a nil function pointer.")
		return status, Undefined
	}
	if function.Chunk == nil { // Check if the chunk within the function is nil
		status := vm.runtimeError("Internal VM Error: Function associated with closure has a nil chunk.")
		return status, Undefined
	}
	code := function.Chunk.Code
	constants := function.Chunk.Constants
	registers := frame.registers // This is the frame's register window
	ip := frame.ip

	for {
		if ip >= len(code) {
			// Save IP before erroring
			frame.ip = ip
			if vm.frameCount > 1 {
				// If we run off the end of a function without OpReturn, that's an error
				status := vm.runtimeError("Implicit return missing in function?")
				return status, Undefined
			} else {
				// Running off end of main script is okay, return Undefined implicitly
				return InterpretOK, Undefined
			}
		}

		opcode := OpCode(code[ip]) // Use local OpCode
		//fmt.Printf("%s | ip: %d | %s\n", frame.closure.Fn.Name, ip, opcode.String())
		ip++ // Advance IP past the opcode itself

		switch opcode {
		case OpLoadConst:
			reg := code[ip]
			constIdxHi := code[ip+1]
			constIdxLo := code[ip+2]
			constIdx := uint16(constIdxHi)<<8 | uint16(constIdxLo)
			ip += 3
			if int(constIdx) >= len(constants) {
				frame.ip = ip // Save IP
				status := vm.runtimeError("Invalid constant index %d", constIdx)
				return status, Undefined
			}
			registers[reg] = constants[constIdx]

		case OpLoadNull:
			reg := code[ip]
			ip++
			registers[reg] = Null // Use local Null

		case OpLoadUndefined:
			reg := code[ip]
			ip++
			registers[reg] = Undefined // Use local Undefined

		case OpLoadTrue:
			reg := code[ip]
			ip++
			registers[reg] = BooleanValue(true) // Use local BooleanValue()

		case OpLoadFalse:
			reg := code[ip]
			ip++
			registers[reg] = BooleanValue(false) // Use local BooleanValue()

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
				status := vm.runtimeError("Operand must be a number for negation.")
				return status, Undefined
			}
			registers[destReg] = Number(-AsNumber(srcVal)) // Use local Number/AsNumber

		case OpNot:
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2
			srcVal := registers[srcReg]
			// In many languages, ! evaluates truthiness
			registers[destReg] = BooleanValue(isFalsey(srcVal)) // Use local Bool

		case OpTypeof:
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2
			srcVal := registers[srcReg]
			// Get the typeof string for the value
			typeofStr := getTypeofString(srcVal)
			registers[destReg] = String(typeofStr)

		case OpToNumber:
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2
			srcVal := registers[srcReg]
			// Convert value to number using the ToFloat() method
			registers[destReg] = Number(srcVal.ToFloat())

		case OpStringConcat:
			destReg := code[ip]
			leftReg := code[ip+1]
			rightReg := code[ip+2]
			ip += 3
			leftVal := registers[leftReg]
			rightVal := registers[rightReg]

			// Optimized string concatenation: convert both operands to strings
			leftStr := leftVal.ToString()
			rightStr := rightVal.ToString()
			registers[destReg] = String(leftStr + rightStr)

		case OpAdd, OpSubtract, OpMultiply, OpDivide,
			OpEqual, OpNotEqual, OpStrictEqual, OpStrictNotEqual,
			OpGreater, OpLess, OpLessEqual,
			OpRemainder, OpExponent:
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
				} else if IsString(leftVal) && IsNumber(rightVal) {
					registers[destReg] = String(AsString(leftVal) + fmt.Sprintf("%v", AsNumber(rightVal)))
				} else if IsNumber(leftVal) && IsString(rightVal) {
					registers[destReg] = String(fmt.Sprintf("%v", AsNumber(leftVal)) + AsString(rightVal))
				} else {
					frame.ip = ip
					status := vm.runtimeError("Operands must be two numbers, two strings, or a string and a number for '+'.")
					return status, Undefined
				}
			case OpSubtract, OpMultiply, OpDivide:
				// Strictly numbers for these
				if !IsNumber(leftVal) || !IsNumber(rightVal) {
					frame.ip = ip
					opStr := opcode.String()                                                 // Get opcode name
					status := vm.runtimeError("Operands must be numbers for %s.", opStr[2:]) // Simple way to get op name like Subtract
					return status, Undefined
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
						status := vm.runtimeError("Division by zero.")
						return status, Undefined
					}
					registers[destReg] = Number(leftNum / rightNum)
				}
			case OpRemainder:
				if !IsNumber(leftVal) || !IsNumber(rightVal) {
					frame.ip = ip
					status := vm.runtimeError("Operands must be numbers for %%.")
					return status, Undefined
				}
				leftNum := AsNumber(leftVal)
				rightNum := AsNumber(rightVal)
				if rightNum == 0 {
					frame.ip = ip
					status := vm.runtimeError("Division by zero (in remainder operation).")
					return status, Undefined
				}
				registers[destReg] = Number(math.Mod(leftNum, rightNum))

			case OpExponent:
				if !IsNumber(leftVal) || !IsNumber(rightVal) {
					frame.ip = ip
					status := vm.runtimeError("Operands must be numbers for **.")
					return status, Undefined
				}
				leftNum := AsNumber(leftVal)
				rightNum := AsNumber(rightVal)
				registers[destReg] = Number(math.Pow(leftNum, rightNum))
			case OpEqual, OpNotEqual:
				// Use a helper for equality check (handles type differences)
				isEqual := valuesEqual(leftVal, rightVal)
				if opcode == OpEqual {
					registers[destReg] = BooleanValue(isEqual)
				} else {
					registers[destReg] = BooleanValue(!isEqual)
				}
			case OpStrictEqual, OpStrictNotEqual: // Added cases
				isStrictlyEqual := valuesStrictEqual(leftVal, rightVal)
				if opcode == OpStrictEqual {
					registers[destReg] = BooleanValue(isStrictlyEqual)
				} else { // OpStrictNotEqual
					registers[destReg] = BooleanValue(!isStrictlyEqual)
				}
			case OpGreater, OpLess, OpLessEqual:
				// Strictly numbers for comparison
				if !IsNumber(leftVal) || !IsNumber(rightVal) {
					frame.ip = ip
					opStr := opcode.String() // Get opcode name
					status := vm.runtimeError("Operands must be numbers for comparison (%s).", opStr[2:])
					return status, Undefined
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
				registers[destReg] = BooleanValue(result)
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
			destReg := code[ip]         // Where the result should go IN THE CALLER frame
			funcReg := code[ip+1]       // Register holding the function/closure/builtin to call
			argCount := int(code[ip+2]) // Number of arguments provided
			ip += 3

			// !! Capture caller registers BEFORE potential frame switch !!
			callerRegisters := registers
			callerIP := ip // Save IP before potential jump/call

			calleeVal := callerRegisters[funcReg] // Get callee from CALLER registers

			switch calleeVal.Type() {
			case TypeClosure:
				// --- Existing Closure Handling ---
				calleeClosure := AsClosure(calleeVal) // Use local AsClosure
				calleeFunc := calleeClosure.Fn
				if argCount != calleeFunc.Arity {
					frame.ip = callerIP // Use saved IP for error context
					status := vm.runtimeError("Expected %d arguments but got %d.", calleeFunc.Arity, argCount)
					return status, Undefined
				}
				if vm.frameCount == MaxFrames {
					frame.ip = callerIP
					status := vm.runtimeError("Stack overflow.")
					return status, Undefined
				}
				requiredRegs := calleeFunc.RegisterSize
				if vm.nextRegSlot+requiredRegs > len(vm.registerStack) {
					frame.ip = callerIP
					status := vm.runtimeError("Register stack overflow during call.")
					return status, Undefined
				}

				frame.ip = callerIP // Store return IP in the outgoing frame

				newFrame := &vm.frames[vm.frameCount]
				newFrame.closure = calleeClosure
				newFrame.ip = 0
				newFrame.targetRegister = destReg // Store target register for return
				newFrame.thisValue = Undefined    // Regular function call has no 'this' context
				newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
				vm.nextRegSlot += requiredRegs

				argStartRegInCaller := funcReg + 1
				for i := 0; i < argCount; i++ {
					if i < len(newFrame.registers) && int(argStartRegInCaller)+i < len(callerRegisters) {
						newFrame.registers[i] = callerRegisters[argStartRegInCaller+byte(i)]
					} else {
						// Rollback frame setup before erroring
						vm.nextRegSlot -= requiredRegs
						frame.ip = callerIP
						status := vm.runtimeError("Internal Error: Argument register index out of bounds during call setup.")
						return status, Undefined
					}
				}
				vm.frameCount++

				// Switch context (update cached variables)
				frame = newFrame
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
				// --- End Existing Closure Handling ---

			case TypeFunction:
				// --- Existing Function Handling (implicit closure) ---
				funcToCall := AsFunction(calleeVal) // Use local AsFunction
				calleeClosure := &ClosureObject{Fn: funcToCall, Upvalues: []*Upvalue{}}
				calleeFunc := calleeClosure.Fn

				if argCount != calleeFunc.Arity {
					frame.ip = callerIP
					status := vm.runtimeError("Expected %d arguments but got %d.", calleeFunc.Arity, argCount)
					return status, Undefined
				}
				if vm.frameCount == MaxFrames {
					frame.ip = callerIP
					status := vm.runtimeError("Stack overflow.")
					return status, Undefined
				}
				requiredRegs := calleeFunc.RegisterSize
				if vm.nextRegSlot+requiredRegs > len(vm.registerStack) {
					frame.ip = callerIP
					status := vm.runtimeError("Register stack overflow during call.")
					return status, Undefined
				}

				frame.ip = callerIP // Store return IP in the outgoing frame

				newFrame := &vm.frames[vm.frameCount]
				newFrame.closure = calleeClosure
				newFrame.ip = 0
				newFrame.targetRegister = destReg
				newFrame.thisValue = Undefined // Regular function call has no 'this' context
				newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
				vm.nextRegSlot += requiredRegs

				argStartRegInCaller := funcReg + 1
				for i := 0; i < argCount; i++ {
					if i < len(newFrame.registers) && int(argStartRegInCaller)+i < len(callerRegisters) {
						newFrame.registers[i] = callerRegisters[argStartRegInCaller+byte(i)]
					} else {
						// Rollback frame setup before erroring
						vm.nextRegSlot -= requiredRegs
						frame.ip = callerIP
						status := vm.runtimeError("Internal Error: Argument register index out of bounds during call setup.")
						return status, Undefined
					}
				}
				vm.frameCount++

				// Switch context
				frame = newFrame
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
				// --- End Existing Function Handling ---

			// <<< NEW CASE FOR NATIVE FUNCTIONS/BUILTINS >>>
			case TypeNativeFunction:
				builtin := AsNativeFunction(calleeVal)

				// --- Arity Check ---
				if builtin.Arity >= 0 && builtin.Arity != argCount {
					frame.ip = callerIP // Use saved IP for error context
					status := vm.runtimeError("Builtin function '%s' expected %d arguments but got %d.",
						builtin.Name, builtin.Arity, argCount)
					return status, Undefined
				}

				// --- Collect Arguments from CALLER registers ---
				args := make([]Value, argCount)
				argStartRegInCaller := funcReg + 1
				for i := 0; i < argCount; i++ {
					// Bounds check against caller's register window length
					if int(argStartRegInCaller)+i < len(callerRegisters) {
						args[i] = callerRegisters[argStartRegInCaller+byte(i)]
					} else {
						frame.ip = callerIP
						status := vm.runtimeError("Internal Error: Argument register index %d out of bounds for builtin call.", argStartRegInCaller+byte(i))
						return status, Undefined
					}
				}

				// --- Execute Built-in ---
				// Note: Builtins run *within* the caller's frame context. No frame switch.
				result := builtin.Fn(args)

				// --- Store Result in CALLER's target register ---
				// Bounds check against caller's register window length
				if int(destReg) < len(callerRegisters) {
					callerRegisters[destReg] = result
				} else {
					frame.ip = callerIP
					status := vm.runtimeError("Internal Error: Invalid target register %d for builtin return value.", destReg)
					return status, Undefined
				}
				// --- Builtin call finished, continue in the same frame ---
				// No context switch needed, ip continues from where OpCall finished reading operands.

			default:
				frame.ip = callerIP // Use saved IP for error context
				status := vm.runtimeError("Can only call functions and closures (got %s).", calleeVal.TypeName())
				return status, Undefined
			}

		case OpReturn:
			srcReg := code[ip]
			ip++
			result := registers[srcReg]
			frame.ip = ip // Save final IP of this frame

			// Close upvalues for the returning frame
			vm.closeUpvalues(frame.registers)

			// Pop the current frame
			// Stash required info before modifying frameCount/nextRegSlot
			returningFrameRegSize := function.RegisterSize
			callerTargetRegister := frame.targetRegister
			isConstructor := frame.isConstructorCall
			constructorThisValue := frame.thisValue

			vm.frameCount--
			vm.nextRegSlot -= returningFrameRegSize // Reclaim register space

			if vm.frameCount == 0 {
				// Returned from the top-level script frame.
				// Return the result directly.
				return InterpretOK, result
			}

			// Get the caller frame (which is now the top frame)
			callerFrame := &vm.frames[vm.frameCount-1]

			// Handle constructor return semantics
			var finalResult Value
			if isConstructor {
				// Constructor call: only return the explicit value if it's an object,
				// otherwise return the instance (this)
				if result.IsObject() {
					finalResult = result // Return the explicit object
				} else {
					finalResult = constructorThisValue // Return the instance
				}
			} else {
				// Regular function call: return the explicit value
				finalResult = result
			}

			// Place the final result into the caller's target register
			if int(callerTargetRegister) < len(callerFrame.registers) {
				callerFrame.registers[callerTargetRegister] = finalResult
			} else {
				// This would be an internal error (compiler/vm mismatch)
				status := vm.runtimeError("Internal Error: Invalid target register %d for return value.", callerTargetRegister)
				return status, Undefined
			}

			// Restore cached variables for the caller frame
			frame = callerFrame // Update local frame pointer
			closure = frame.closure
			function = closure.Fn
			code = function.Chunk.Code
			constants = function.Chunk.Constants
			registers = frame.registers
			ip = frame.ip // Restore caller's IP

		case OpReturnUndefined:
			frame.ip = ip // Save final IP

			// Close upvalues for the returning frame
			vm.closeUpvalues(frame.registers)

			// Pop the current frame
			returningFrameRegSize := function.RegisterSize
			callerTargetRegister := frame.targetRegister
			isConstructor := frame.isConstructorCall
			constructorThisValue := frame.thisValue

			vm.frameCount--
			vm.nextRegSlot -= returningFrameRegSize

			if vm.frameCount == 0 {
				// Returned undefined from top-level
				return InterpretOK, Undefined
			}

			// Get the caller frame
			callerFrame := &vm.frames[vm.frameCount-1]

			// Handle constructor return semantics
			var finalResult Value
			if isConstructor {
				// Constructor returning undefined: return the instance (this)
				finalResult = constructorThisValue
			} else {
				// Regular function returning undefined
				finalResult = Undefined
			}

			// Place the final result into the caller's target register
			if int(callerTargetRegister) < len(callerFrame.registers) {
				callerFrame.registers[callerTargetRegister] = finalResult
			} else {
				status := vm.runtimeError("Internal Error: Invalid target register %d for return undefined.", callerTargetRegister)
				return status, Undefined
			}

			// Restore cached variables for the caller frame
			frame = callerFrame // Update local frame pointer
			closure = frame.closure
			function = closure.Fn
			code = function.Chunk.Code
			constants = function.Chunk.Constants
			registers = frame.registers
			ip = frame.ip // Restore caller's IP

		case OpClosure:
			destReg := code[ip]
			funcConstIdxHi := code[ip+1]
			funcConstIdxLo := code[ip+2]
			funcConstIdx := uint16(funcConstIdxHi)<<8 | uint16(funcConstIdxLo)
			upvalueCount := int(code[ip+3])
			ip += 4

			if int(funcConstIdx) >= len(constants) {
				frame.ip = ip
				status := vm.runtimeError("Invalid function constant index %d for closure.", funcConstIdx)
				return status, Undefined
			}
			protoVal := constants[funcConstIdx]
			if !IsFunction(protoVal) {
				frame.ip = ip
				status := vm.runtimeError("Constant %d is not a function, cannot create closure.", funcConstIdx)
				return status, Undefined
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
						status := vm.runtimeError("Invalid local register index %d for upvalue capture.", index)
						return status, Undefined
					}
					// Pass pointer to the stack slot (register) itself.
					location := &registers[index]
					upvalues[i] = vm.captureUpvalue(location)
				} else {
					// Capture upvalue from the *enclosing* function (i.e., the current closure).
					if closure == nil || index >= len(closure.Upvalues) {
						frame.ip = ip
						status := vm.runtimeError("Invalid upvalue index %d for capture.", index)
						return status, Undefined
					}
					upvalues[i] = closure.Upvalues[index]
				}
			}

			// Create the closure Value using the constructor
			// newClosureVal := NewClosure(protoFunc, upvalues)
			// Create a new closure value using the value-level constructor
			registers[destReg] = NewClosure(protoFunc, upvalues)

		case OpLoadFree:
			destReg := code[ip]
			upvalueIndex := int(code[ip+1])
			ip += 2

			if closure == nil || upvalueIndex >= len(closure.Upvalues) {
				frame.ip = ip
				status := vm.runtimeError("Invalid upvalue index %d for OpLoadFree.", upvalueIndex)
				return status, Undefined
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
				status := vm.runtimeError("Invalid upvalue index %d for OpSetUpvalue.", upvalueIndex)
				return status, Undefined
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
				status := vm.runtimeError("Internal Error: Register index out of bounds during array creation (start=%d, count=%d, frame size=%d)", startIdx, count, len(registers))
				return status, Undefined
			}

			copy(elements, registers[startIdx:endIdx])

			// Create the array value
			arrayValue := NewArray()
			arrayObj := AsArray(arrayValue)
			arrayObj.elements = elements
			arrayObj.length = len(elements)
			registers[destReg] = arrayValue

		case OpGetIndex:
			destReg := code[ip]
			baseReg := code[ip+1] // Renamed from arrayReg for clarity
			indexReg := code[ip+2]
			ip += 3

			baseVal := registers[baseReg]
			indexVal := registers[indexReg]

			// --- MODIFIED: Handle Array, Object, String ---
			switch baseVal.Type() {
			case TypeArray:
				if !IsNumber(indexVal) {
					frame.ip = ip
					status := vm.runtimeError("Array index must be a number, got '%v'", indexVal.Type())
					return status, Undefined
				}
				arr := AsArray(baseVal)
				idx := int(AsNumber(indexVal))
				if idx < 0 || idx >= len(arr.elements) {
					registers[destReg] = Undefined // Out of bounds -> undefined
				} else {
					registers[destReg] = arr.elements[idx]
				}

			case TypeObject, TypeDictObject: // <<< NEW
				var key string
				switch indexVal.Type() {
				case TypeString:
					key = AsString(indexVal)
				case TypeFloatNumber, TypeIntegerNumber:
					key = strconv.FormatFloat(AsNumber(indexVal), 'f', -1, 64) // Consistent conversion
					// Or: key = fmt.Sprintf("%v", AsNumber(indexVal))
				default:
					frame.ip = ip
					status := vm.runtimeError("Object index must be a string or number, got '%v'", indexVal.Type())
					return status, Undefined
				}

				var propValue Value
				var ok bool
				if baseVal.Type() == TypeDictObject {
					dict := AsDictObject(baseVal)
					propValue, ok = dict.GetOwn(key)
				} else {
					obj := AsPlainObject(baseVal)
					propValue, ok = obj.GetOwn(key)
				}

				if !ok {
					registers[destReg] = Undefined // Property not found -> undefined
				} else {
					registers[destReg] = propValue
				}

			case TypeString: // <<< NEW (or ensure existing logic is here)
				if !IsNumber(indexVal) {
					frame.ip = ip
					status := vm.runtimeError("String index must be a number, got '%v'", indexVal.Type())
					return status, Undefined
				}
				str := AsString(baseVal)
				idx := int(AsNumber(indexVal))
				runes := []rune(str)
				if idx < 0 || idx >= len(runes) {
					registers[destReg] = Undefined // Out of bounds -> undefined
				} else {
					registers[destReg] = String(string(runes[idx])) // Return char as string
				}

			default:
				frame.ip = ip
				status := vm.runtimeError("Cannot index non-array/object/string type '%v' at IP %d", baseVal.Type(), ip)
				return status, Undefined
			}
			// --- END MODIFICATION ---

		case OpSetIndex:
			baseReg := code[ip] // Renamed from arrayReg
			indexReg := code[ip+1]
			valueReg := code[ip+2]
			ip += 3

			baseVal := registers[baseReg]
			indexVal := registers[indexReg]
			valueVal := registers[valueReg]

			// --- MODIFIED: Handle Array and Object ---
			switch baseVal.Type() {
			case TypeArray:
				if !IsNumber(indexVal) {
					frame.ip = ip
					status := vm.runtimeError("Array index must be a number, got '%v'", indexVal.Type())
					return status, Undefined
				}

				arr := AsArray(baseVal)
				idx := int(AsNumber(indexVal))

				// Handle Array Expansion (keep existing logic)
				if idx < 0 {
					frame.ip = ip
					status := vm.runtimeError("Array index cannot be negative, got %d", idx)
					return status, Undefined
				} else if idx < len(arr.elements) {
					arr.elements[idx] = valueVal
				} else if idx == len(arr.elements) {
					arr.elements = append(arr.elements, valueVal)
					arr.length++
				} else {
					neededCapacity := idx + 1
					if cap(arr.elements) < neededCapacity {
						newElements := make([]Value, len(arr.elements), neededCapacity)
						copy(newElements, arr.elements)
						arr.elements = newElements
					}
					for i := len(arr.elements); i < idx; i++ {
						arr.elements = append(arr.elements, Undefined)
					}
					arr.elements = append(arr.elements, valueVal)
					arr.length = len(arr.elements)
				}

			case TypeObject, TypeDictObject: // <<< NEW
				var key string
				switch indexVal.Type() {
				case TypeString:
					key = AsString(indexVal)
				case TypeFloatNumber, TypeIntegerNumber:
					key = strconv.FormatFloat(AsNumber(indexVal), 'f', -1, 64) // Consistent conversion
				default:
					frame.ip = ip
					status := vm.runtimeError("Object index must be a string or number, got '%v'", indexVal.Type())
					return status, Undefined
				}

				// Set the property on the object
				if baseVal.Type() == TypeDictObject {
					dict := AsDictObject(baseVal)
					dict.SetOwn(key, valueVal)
				} else {
					obj := AsPlainObject(baseVal)
					obj.SetOwn(key, valueVal)
				}

			default:
				frame.ip = ip
				status := vm.runtimeError("Cannot set index on non-array/object type '%v'", baseVal.Type())
				return status, Undefined
			}
			// --- END MODIFICATION ---

		// --- End Array Opcodes ---

		// --- NEW: Get Length Opcode ---
		case OpGetLength:
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2

			srcVal := registers[srcReg]
			var length float64 = -1 // Initialize to -1 to indicate error if type is wrong

			switch srcVal.Type() {
			case TypeArray:
				arr := AsArray(srcVal)
				length = float64(len(arr.elements))
			case TypeString:
				str := AsString(srcVal)
				// Use rune count for string length to handle multi-byte chars correctly
				length = float64(len(str))
			default:
				frame.ip = ip
				status := vm.runtimeError("Cannot get length of type '%v'", srcVal.Type())
				return status, Undefined
			}

			registers[destReg] = Number(length)
		// --- END NEW ---

		// --- NEW: Bitwise & Shift ---
		case OpBitwiseNot:
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2
			srcVal := registers[srcReg]

			// JavaScript-style type coercion for bitwise operations
			// undefined becomes 0, null becomes 0, booleans become 0/1, etc.
			srcInt := int64(srcVal.ToInteger())
			result := ^srcInt
			registers[destReg] = Number(float64(result))

		case OpBitwiseAnd, OpBitwiseOr, OpBitwiseXor,
			OpShiftLeft, OpShiftRight, OpUnsignedShiftRight:
			destReg := code[ip]
			leftReg := code[ip+1]
			rightReg := code[ip+2]
			ip += 3
			leftVal := registers[leftReg]
			rightVal := registers[rightReg]

			// JavaScript-style type coercion for bitwise operations
			// undefined becomes 0, null becomes 0, booleans become 0/1, etc.
			leftInt := int64(leftVal.ToInteger())
			rightInt := int64(rightVal.ToInteger())
			var result int64 // Keep result var for cases that use it

			switch opcode {
			case OpBitwiseAnd:
				result = leftInt & rightInt
				registers[destReg] = Number(float64(result)) // Store result
			case OpBitwiseOr:
				result = leftInt | rightInt
				registers[destReg] = Number(float64(result)) // Store result
			case OpBitwiseXor:
				result = leftInt ^ rightInt
				registers[destReg] = Number(float64(result)) // Store result
			case OpShiftLeft:
				shiftAmount := uint64(rightInt) & 63 // Cap to 64 bits
				result = leftInt << shiftAmount
				registers[destReg] = Number(float64(result)) // Store result
			case OpShiftRight: // Arithmetic shift (preserves sign)
				shiftAmount := uint64(rightInt) & 63
				result = leftInt >> shiftAmount
				registers[destReg] = Number(float64(result)) // Store result
			case OpUnsignedShiftRight: // Logical shift (zero-fills)
				// Convert left operand to uint32 first to mimic JS >>> behavior
				leftUint32 := uint32(leftVal.ToInteger()) // Use ToInteger() for coercion
				shiftAmount := uint64(rightInt) & 31      // JS uses lower 5 bits for 32-bit shift count
				unsignedResult := leftUint32 >> shiftAmount
				// Result is converted back to Number (float64)
				registers[destReg] = Number(float64(unsignedResult)) // Store the uint32 result directly as Number
				// No need to assign to 'result' variable here
			}
			// No shared assignment needed here anymore

		// --- NEW: Object Opcodes ---
		case OpMakeEmptyObject:
			destReg := code[ip]
			ip++
			// Create a new empty object value
			// Create a new empty object using the shape-based PlainObject
			registers[destReg] = NewObject(Undefined)

		case OpGetProp:
			destReg := code[ip]
			objReg := code[ip+1]
			nameConstIdxHi := code[ip+2]
			nameConstIdxLo := code[ip+3]
			nameConstIdx := uint16(nameConstIdxHi)<<8 | uint16(nameConstIdxLo)
			// Calculate cache key based on instruction pointer (before advancing ip)
			ip += 4

			// Get object and property name values
			objVal := registers[objReg]

			// Get property name from constants
			if int(nameConstIdx) >= len(constants) {
				frame.ip = ip
				status := vm.runtimeError("Invalid constant index %d for property name.", nameConstIdx)
				return status, Undefined
			}
			nameVal := constants[nameConstIdx]
			if !IsString(nameVal) { // Compiler should ensure this
				frame.ip = ip
				status := vm.runtimeError("Internal Error: Property name constant %d is not a string.", nameConstIdx)
				return status, Undefined
			}
			propName := AsString(nameVal)

			// FIX: Use hash-based cache key to avoid collisions
			// Combine instruction pointer with property name hash
			propNameHash := 0
			for _, b := range []byte(propName) {
				propNameHash = propNameHash*31 + int(b)
			}
			cacheKey := (ip-5)*100000 + (propNameHash & 0xFFFF) // Use ip-5 since ip was advanced by 4
			cache, exists := vm.propCache[cacheKey]
			if !exists {
				cache = &PropInlineCache{
					state: CacheStateUninitialized,
				}
				vm.propCache[cacheKey] = cache
			}

			// Initialize prototypes if needed
			initPrototypes()

			// --- Special handling for .length ---
			// Check the *original* value type before checking if it's an Object type
			if propName == "length" {
				switch objVal.Type() {
				case TypeArray:
					arr := AsArray(objVal)
					registers[destReg] = Number(float64(len(arr.elements)))
					continue // Skip general object lookup
				case TypeString:
					str := AsString(objVal)
					// Use rune count for correct length of multi-byte strings
					registers[destReg] = Number(float64(utf8.RuneCountInString(str)))
					continue // Skip general object lookup
				}
				// If not Array or String, fall through to general object property lookup
			}

			// --- Handle prototype methods for primitives ---
			// Handle String prototype methods
			if objVal.IsString() {
				if method, exists := StringPrototype[propName]; exists {
					registers[destReg] = createBoundMethod(objVal, method)
					continue // Skip object lookup
				}
			}

			// Handle Array prototype methods
			if objVal.IsArray() {
				if method, exists := ArrayPrototype[propName]; exists {
					registers[destReg] = createBoundMethod(objVal, method)
					continue // Skip object lookup
				}
			}

			// Handle static methods on native function constructors (like String.fromCharCode)
			if objVal.IsNativeFunction() {
				nativeFn := AsNativeFunction(objVal)
				// Check for known static methods - hard-coded for now
				// TODO: Make this more extensible
				if nativeFn.Name == "String" && propName == "fromCharCode" {
					// Return the String.fromCharCode function
					staticMethod := &NativeFunctionObject{
						Arity:    -1,
						Variadic: true,
						Name:     "fromCharCode",
						Fn:       stringFromCharCodeStaticImpl,
					}
					registers[destReg] = NewNativeFunction(staticMethod.Arity, staticMethod.Variadic, staticMethod.Name, staticMethod.Fn)
					continue // Skip object lookup
				}
			}

			// General property lookup
			if !objVal.IsObject() {
				frame.ip = ip
				// Check for null/undefined specifically for a better error message
				switch objVal.Type() {
				case TypeNull, TypeUndefined:
					status := vm.runtimeError("Cannot read property '%s' of %s", propName, objVal.TypeName())
					return status, Undefined
				default:
					// Generic error for other non-object types
					status := vm.runtimeError("Cannot access property '%s' on non-object type '%s'", propName, objVal.TypeName())
					return status, Undefined
				}
			}

			// --- INLINE CACHE CHECK (PlainObjects only for now) ---
			if objVal.Type() == TypeObject {
				po := AsPlainObject(objVal)

				// Try cache lookup first
				if offset, hit := cache.lookupInCache(po.shape); hit {
					// Cache hit! Use cached offset directly (fast path)
					vm.cacheStats.totalHits++
					switch cache.state {
					case CacheStateMonomorphic:
						vm.cacheStats.monomorphicHits++
					case CacheStatePolymorphic:
						vm.cacheStats.polymorphicHits++
					case CacheStateMegamorphic:
						vm.cacheStats.megamorphicHits++
					}

					if offset < len(po.properties) {
						result := po.properties[offset]
						registers[destReg] = result
						continue // Skip slow path lookup
					}
					// Cached offset is out of bounds - cache is stale, fall through to slow path
				}

				// Cache miss - do slow path lookup and update cache
				vm.cacheStats.totalMisses++
				if fv, ok := po.GetOwn(propName); ok {
					registers[destReg] = fv
					// Update cache: find the offset for this property in the shape
					for _, field := range po.shape.fields {
						if field.name == propName {
							cache.updateCache(po.shape, field.offset)
							break
						}
					}
				} else {
					registers[destReg] = Undefined
					// Don't cache undefined lookups for now
				}
				continue // Skip the old dispatch logic below
			}

			// --- Fallback for DictObject (no caching) ---
			// Dispatch to PlainObject or DictObject lookup
			switch objVal.Type() {
			case TypeDictObject:
				dict := AsDictObject(objVal)
				if fv, ok := dict.GetOwn(propName); ok {
					registers[destReg] = fv
				} else {
					registers[destReg] = Undefined
				}
			default:
				// PlainObject or other object types (should not reach here due to continue above)
				po := AsPlainObject(objVal)
				if fv, ok := po.GetOwn(propName); ok {
					registers[destReg] = fv
				} else {
					registers[destReg] = Undefined
				}
			}

		case OpSetProp:
			objReg := code[ip]   // Register holding the object
			valReg := code[ip+1] // Register holding the value to set
			nameConstIdxHi := code[ip+2]
			nameConstIdxLo := code[ip+3]
			nameConstIdx := uint16(nameConstIdxHi)<<8 | uint16(nameConstIdxLo)
			// Calculate cache key based on instruction pointer (before advancing ip)
			ip += 4

			// Get object, property name, and value
			objVal := registers[objReg]
			valueToSet := registers[valReg]

			// Get property name from constants
			if int(nameConstIdx) >= len(constants) {
				frame.ip = ip
				status := vm.runtimeError("Invalid constant index %d for property name.", nameConstIdx)
				return status, Undefined
			}
			nameVal := constants[nameConstIdx]
			if !IsString(nameVal) { // Compiler should ensure this
				frame.ip = ip
				status := vm.runtimeError("Internal Error: Property name constant %d is not a string.", nameConstIdx)
				return status, Undefined
			}
			propName := AsString(nameVal)

			// FIX: Use hash-based cache key to avoid collisions
			// Combine instruction pointer with property name hash
			propNameHash := 0
			for _, b := range []byte(propName) {
				propNameHash = propNameHash*31 + int(b)
			}
			cacheKey := (ip-5)*100000 + (propNameHash & 0xFFFF) // Use ip-5 since ip was advanced by 4
			cache, exists := vm.propCache[cacheKey]
			if !exists {
				cache = &PropInlineCache{
					state: CacheStateUninitialized,
				}
				vm.propCache[cacheKey] = cache
			}

			// Check if the base is actually an object
			if !objVal.IsObject() {
				frame.ip = ip
				// Error setting property on non-object
				status := vm.runtimeError("Cannot set property '%s' on non-object type '%s'", propName, objVal.TypeName())
				return status, Undefined
			}

			// --- INLINE CACHE CHECK FOR PROPERTY WRITES (PlainObjects only) ---
			if objVal.Type() == TypeObject {
				po := AsPlainObject(objVal)

				// Try cache lookup for existing property write
				if offset, hit := cache.lookupInCache(po.shape); hit {
					// Cache hit! Check if this is an existing property update (fast path)
					vm.cacheStats.totalHits++
					switch cache.state {
					case CacheStateMonomorphic:
						vm.cacheStats.monomorphicHits++
					case CacheStatePolymorphic:
						vm.cacheStats.polymorphicHits++
					case CacheStateMegamorphic:
						vm.cacheStats.megamorphicHits++
					}

					// Check if property exists in current shape
					for _, field := range po.shape.fields {
						if field.name == propName && field.offset == offset {
							// Existing property - fast update path
							if offset < len(po.properties) {
								po.properties[offset] = valueToSet
								continue // Skip slow path
							}
							break
						}
					}
					// Cache was stale or property layout changed, fall through to slow path
				}

				// Cache miss or new property - use slow path and update cache
				vm.cacheStats.totalMisses++
				originalShape := po.shape
				po.SetOwn(propName, valueToSet)

				// Update cache if shape didn't change (existing property)
				// or if shape changed (new property added)
				for _, field := range po.shape.fields {
					if field.name == propName {
						cache.updateCache(po.shape, field.offset)
						break
					}
				}

				// If shape changed significantly, we might want to invalidate related caches
				// This is a trade-off between cache accuracy and performance
				if originalShape != po.shape {
					// Shape transition occurred - could invalidate other caches
					// For now, just update this cache
				}
				continue // Skip the old dispatch logic below
			}

			// --- Fallback for DictObject (no caching) ---
			// Set property on DictObject or PlainObject
			switch objVal.Type() {
			case TypeDictObject:
				d := AsDictObject(objVal)
				d.SetOwn(propName, valueToSet)
			default:
				po := AsPlainObject(objVal)
				po.SetOwn(propName, valueToSet)
			}

		case OpCallMethod:
			destReg := code[ip]         // Where the result should go in the caller
			funcReg := code[ip+1]       // Register holding the method function/closure
			thisReg := code[ip+2]       // Register holding the 'this' object
			argCount := int(code[ip+3]) // Number of arguments provided
			ip += 4

			// Capture caller context before potential frame switch
			callerRegisters := registers
			callerIP := ip

			calleeVal := callerRegisters[funcReg]
			thisVal := callerRegisters[thisReg]

			switch calleeVal.Type() {
			case TypeClosure:
				// Method call on closure
				calleeClosure := AsClosure(calleeVal)
				calleeFunc := calleeClosure.Fn
				if argCount != calleeFunc.Arity {
					frame.ip = callerIP
					status := vm.runtimeError("Method expected %d arguments but got %d.", calleeFunc.Arity, argCount)
					return status, Undefined
				}
				if vm.frameCount == MaxFrames {
					frame.ip = callerIP
					status := vm.runtimeError("Stack overflow during method call.")
					return status, Undefined
				}
				requiredRegs := calleeFunc.RegisterSize
				if vm.nextRegSlot+requiredRegs > len(vm.registerStack) {
					frame.ip = callerIP
					status := vm.runtimeError("Register stack overflow during method call.")
					return status, Undefined
				}

				frame.ip = callerIP // Store return IP

				newFrame := &vm.frames[vm.frameCount]
				newFrame.closure = calleeClosure
				newFrame.ip = 0
				newFrame.targetRegister = destReg
				newFrame.thisValue = thisVal // Set 'this' context for the method
				newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
				vm.nextRegSlot += requiredRegs

				// Copy arguments to new frame (starting from register 0)
				argStartRegInCaller := funcReg + 1
				for i := 0; i < argCount; i++ {
					if i < len(newFrame.registers) && int(argStartRegInCaller)+i < len(callerRegisters) {
						newFrame.registers[i] = callerRegisters[argStartRegInCaller+byte(i)]
					} else {
						vm.nextRegSlot -= requiredRegs
						frame.ip = callerIP
						status := vm.runtimeError("Internal Error: Argument register index out of bounds during method call setup.")
						return status, Undefined
					}
				}
				vm.frameCount++

				// Switch context to new frame
				frame = newFrame
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip

			case TypeFunction:
				// Method call on function (create implicit closure)
				funcToCall := AsFunction(calleeVal)
				calleeClosure := &ClosureObject{Fn: funcToCall, Upvalues: []*Upvalue{}}
				calleeFunc := calleeClosure.Fn

				if argCount != calleeFunc.Arity {
					frame.ip = callerIP
					status := vm.runtimeError("Method expected %d arguments but got %d.", calleeFunc.Arity, argCount)
					return status, Undefined
				}
				if vm.frameCount == MaxFrames {
					frame.ip = callerIP
					status := vm.runtimeError("Stack overflow during method call.")
					return status, Undefined
				}
				requiredRegs := calleeFunc.RegisterSize
				if vm.nextRegSlot+requiredRegs > len(vm.registerStack) {
					frame.ip = callerIP
					status := vm.runtimeError("Register stack overflow during method call.")
					return status, Undefined
				}

				frame.ip = callerIP

				newFrame := &vm.frames[vm.frameCount]
				newFrame.closure = calleeClosure
				newFrame.ip = 0
				newFrame.targetRegister = destReg
				newFrame.thisValue = thisVal // Set 'this' context for the method
				newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
				vm.nextRegSlot += requiredRegs

				// Copy arguments to new frame
				argStartRegInCaller := funcReg + 1
				for i := 0; i < argCount; i++ {
					if i < len(newFrame.registers) && int(argStartRegInCaller)+i < len(callerRegisters) {
						newFrame.registers[i] = callerRegisters[argStartRegInCaller+byte(i)]
					} else {
						vm.nextRegSlot -= requiredRegs
						frame.ip = callerIP
						status := vm.runtimeError("Internal Error: Argument register index out of bounds during method call setup.")
						return status, Undefined
					}
				}
				vm.frameCount++

				// Switch context
				frame = newFrame
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip

			case TypeNativeFunction:
				// Method call on builtin function
				builtin := AsNativeFunction(calleeVal)

				if builtin.Arity >= 0 && builtin.Arity != argCount {
					frame.ip = callerIP
					status := vm.runtimeError("Built-in method '%s' expected %d arguments but got %d.",
						builtin.Name, builtin.Arity, argCount)
					return status, Undefined
				}

				// Collect arguments for builtin method call
				args := make([]Value, argCount)
				argStartRegInCaller := funcReg + 1
				for i := 0; i < argCount; i++ {
					if int(argStartRegInCaller)+i < len(callerRegisters) {
						args[i] = callerRegisters[argStartRegInCaller+byte(i)]
					} else {
						frame.ip = callerIP
						status := vm.runtimeError("Internal Error: Argument register index %d out of bounds for builtin method call.", argStartRegInCaller+byte(i))
						return status, Undefined
					}
				}

				// Execute builtin method (builtins receive 'this' as context)
				// TODO: Update builtin signature to receive 'this' as first parameter or context
				result := builtin.Fn(args)

				// Store result in caller's target register
				if int(destReg) < len(callerRegisters) {
					callerRegisters[destReg] = result
				} else {
					frame.ip = callerIP
					status := vm.runtimeError("Internal Error: Invalid target register %d for builtin method return value.", destReg)
					return status, Undefined
				}

			default:
				frame.ip = callerIP
				status := vm.runtimeError("Cannot call method on non-function type '%s'.", calleeVal.TypeName())
				return status, Undefined
			}

		case OpNew:
			destReg := code[ip]          // Where the created instance should go in the caller
			constructorReg := code[ip+1] // Register holding the constructor function/closure
			argCount := int(code[ip+2])  // Number of arguments provided to the constructor
			ip += 3

			// Capture caller context before potential frame switch
			callerRegisters := registers
			callerIP := ip

			constructorVal := callerRegisters[constructorReg]

			switch constructorVal.Type() {
			case TypeClosure:
				// Constructor call on closure
				constructorClosure := AsClosure(constructorVal)
				constructorFunc := constructorClosure.Fn
				if argCount != constructorFunc.Arity {
					frame.ip = callerIP
					status := vm.runtimeError("Constructor expected %d arguments but got %d.", constructorFunc.Arity, argCount)
					return status, Undefined
				}
				if vm.frameCount == MaxFrames {
					frame.ip = callerIP
					status := vm.runtimeError("Stack overflow during constructor call.")
					return status, Undefined
				}
				requiredRegs := constructorFunc.RegisterSize
				if vm.nextRegSlot+requiredRegs > len(vm.registerStack) {
					frame.ip = callerIP
					status := vm.runtimeError("Register stack overflow during constructor call.")
					return status, Undefined
				}

				// Create new instance object as 'this'
				newInstance := NewObject(DefaultObjectPrototype)

				frame.ip = callerIP // Store return IP

				newFrame := &vm.frames[vm.frameCount]
				newFrame.closure = constructorClosure
				newFrame.ip = 0
				newFrame.targetRegister = destReg
				newFrame.thisValue = newInstance  // Set the new instance as 'this'
				newFrame.isConstructorCall = true // Mark this as a constructor call
				newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
				vm.nextRegSlot += requiredRegs

				// Copy arguments to new frame (starting from register 0)
				argStartRegInCaller := constructorReg + 1
				for i := 0; i < argCount; i++ {
					if i < len(newFrame.registers) && int(argStartRegInCaller)+i < len(callerRegisters) {
						newFrame.registers[i] = callerRegisters[argStartRegInCaller+byte(i)]
					} else {
						vm.nextRegSlot -= requiredRegs
						frame.ip = callerIP
						status := vm.runtimeError("Internal Error: Argument register index out of bounds during constructor call setup.")
						return status, Undefined
					}
				}
				vm.frameCount++

				// Store the new instance in the caller's destination register
				// NOTE: This is different from regular calls - we set the result immediately
				// and the constructor can modify 'this', but the instance is always returned
				// unless the constructor explicitly returns a different object
				callerRegisters[destReg] = newInstance

				// Switch context to new frame
				frame = newFrame
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip

			case TypeFunction:
				// Constructor call on function (create implicit closure)
				funcToCall := AsFunction(constructorVal)
				constructorClosure := &ClosureObject{Fn: funcToCall, Upvalues: []*Upvalue{}}
				constructorFunc := constructorClosure.Fn

				if argCount != constructorFunc.Arity {
					frame.ip = callerIP
					status := vm.runtimeError("Constructor expected %d arguments but got %d.", constructorFunc.Arity, argCount)
					return status, Undefined
				}
				if vm.frameCount == MaxFrames {
					frame.ip = callerIP
					status := vm.runtimeError("Stack overflow during constructor call.")
					return status, Undefined
				}
				requiredRegs := constructorFunc.RegisterSize
				if vm.nextRegSlot+requiredRegs > len(vm.registerStack) {
					frame.ip = callerIP
					status := vm.runtimeError("Register stack overflow during constructor call.")
					return status, Undefined
				}

				// Create new instance object as 'this'
				newInstance := NewObject(DefaultObjectPrototype)

				frame.ip = callerIP

				newFrame := &vm.frames[vm.frameCount]
				newFrame.closure = constructorClosure
				newFrame.ip = 0
				newFrame.targetRegister = destReg
				newFrame.thisValue = newInstance  // Set the new instance as 'this'
				newFrame.isConstructorCall = true // Mark this as a constructor call
				newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
				vm.nextRegSlot += requiredRegs

				// Copy arguments to new frame
				argStartRegInCaller := constructorReg + 1
				for i := 0; i < argCount; i++ {
					if i < len(newFrame.registers) && int(argStartRegInCaller)+i < len(callerRegisters) {
						newFrame.registers[i] = callerRegisters[argStartRegInCaller+byte(i)]
					} else {
						vm.nextRegSlot -= requiredRegs
						frame.ip = callerIP
						status := vm.runtimeError("Internal Error: Argument register index out of bounds during constructor call setup.")
						return status, Undefined
					}
				}
				vm.frameCount++

				// Store the new instance in the caller's destination register
				callerRegisters[destReg] = newInstance

				// Switch context
				frame = newFrame
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip

			case TypeNativeFunction:
				// Constructor call on builtin function
				builtin := AsNativeFunction(constructorVal)

				if builtin.Arity >= 0 && builtin.Arity != argCount {
					frame.ip = callerIP
					status := vm.runtimeError("Built-in constructor '%s' expected %d arguments but got %d.",
						builtin.Name, builtin.Arity, argCount)
					return status, Undefined
				}

				// Collect arguments for builtin constructor call
				args := make([]Value, argCount)
				argStartRegInCaller := constructorReg + 1
				for i := 0; i < argCount; i++ {
					if int(argStartRegInCaller)+i < len(callerRegisters) {
						args[i] = callerRegisters[argStartRegInCaller+byte(i)]
					} else {
						frame.ip = callerIP
						status := vm.runtimeError("Internal Error: Argument register index %d out of bounds for builtin constructor call.", argStartRegInCaller+byte(i))
						return status, Undefined
					}
				}

				// Execute builtin constructor
				// For builtins, we let them handle instance creation
				result := builtin.Fn(args)

				// Store result in caller's target register
				if int(destReg) < len(callerRegisters) {
					callerRegisters[destReg] = result
				} else {
					frame.ip = callerIP
					status := vm.runtimeError("Internal Error: Invalid target register %d for builtin constructor return value.", destReg)
					return status, Undefined
				}

			default:
				frame.ip = callerIP
				status := vm.runtimeError("Cannot use '%s' as a constructor.", constructorVal.TypeName())
				return status, Undefined
			}

		case OpLoadThis:
			destReg := code[ip]
			ip++

			// Load 'this' value from current call frame context
			// If no 'this' context is set (regular function call), return undefined
			registers[destReg] = frame.thisValue

		case OpGetGlobal:
			destReg := code[ip]
			globalIdxHi := code[ip+1]
			globalIdxLo := code[ip+2]
			globalIdx := uint16(globalIdxHi)<<8 | uint16(globalIdxLo)
			ip += 3

			// Use direct global index access
			if int(globalIdx) >= len(vm.globals) {
				frame.ip = ip
				status := vm.runtimeError("Invalid global variable index %d (max: %d)", globalIdx, len(vm.globals)-1)
				return status, Undefined
			}

			// Direct array access - much faster than map lookup
			registers[destReg] = vm.globals[globalIdx]

		case OpSetGlobal:
			globalIdxHi := code[ip]
			globalIdxLo := code[ip+1]
			srcReg := code[ip+2]
			globalIdx := uint16(globalIdxHi)<<8 | uint16(globalIdxLo)
			ip += 3

			// Use direct global index access for setting
			if int(globalIdx) >= len(vm.globals) {
				// Expand globals array if needed (for dynamic global assignment)
				for len(vm.globals) <= int(globalIdx) {
					vm.globals = append(vm.globals, Undefined)
					vm.globalNames = append(vm.globalNames, "") // Placeholder name
				}
			}

			// Direct array assignment - much faster than map assignment
			vm.globals[globalIdx] = registers[srcReg]

		default:
			frame.ip = ip // Save IP before erroring
			status := vm.runtimeError("Unknown opcode %d encountered.", opcode)
			return status, Undefined
		}
	}
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
			upvalue.Closed = closedValue     // Store the value
			upvalue.Location = nil           // Mark as closed
			// Do NOT add it back to newOpenUpvalues
		} else {
			// This upvalue points elsewhere (e.g., higher up the stack), keep it open.
			newOpenUpvalues = append(newOpenUpvalues, upvalue)
		}
	}
	vm.openUpvalues = newOpenUpvalues
}

// runtimeError formats a runtime error message, appends it to the VM's error list,
// and returns the InterpretRuntimeError status.
func (vm *VM) runtimeError(format string, args ...interface{}) InterpretResult {
	// Get the current frame to access chunk and IP
	if vm.frameCount == 0 {
		// Should not happen if called during run()
		// Create a generic error if no frame context
		runtimeErr := &errors.RuntimeError{
			Position: errors.Position{Line: 0, Column: 0, StartPos: 0, EndPos: 0}, // No position info
			Msg:      fmt.Sprintf(format, args...),
		}
		vm.errors = append(vm.errors, runtimeErr)
		// Also print to stderr as a fallback in this unexpected case
		fmt.Fprintf(os.Stderr, "[VM Error - No Frame]: %s\n", runtimeErr.Msg)
		return InterpretRuntimeError
	}

	frame := &vm.frames[vm.frameCount-1]
	// ip points to the *next* instruction, error occurred at ip-1
	instructionPos := frame.ip - 1
	line := 0
	// Safety check for chunk and bounds before calling GetLine
	if frame.closure != nil && frame.closure.Fn != nil && frame.closure.Fn.Chunk != nil {
		line = frame.closure.Fn.Chunk.GetLine(instructionPos)
	}

	msg := fmt.Sprintf(format, args...)

	runtimeErr := &errors.RuntimeError{
		// TODO: Get Column/StartPos/EndPos if possible later
		Position: errors.Position{
			Line:     line,
			Column:   0, // Placeholder
			StartPos: 0, // Placeholder
			EndPos:   0, // Placeholder
		},
		Msg: msg,
	}
	vm.errors = append(vm.errors, runtimeErr)

	// --- Keep stderr print temporarily for immediate feedback during refactor? ---
	// fmt.Fprintf(os.Stderr, "[line %d] Runtime Error: %s\n", line, msg)
	// --- Remove later ---

	return InterpretRuntimeError
}

// getTypeofString returns the JavaScript typeof string for a given value
func getTypeofString(val Value) string {
	switch val.Type() {
	case TypeUndefined:
		return "undefined"
	case TypeNull:
		return "object" // In JavaScript, typeof null === "object" (historical quirk)
	case TypeBoolean:
		return "boolean"
	case TypeFloatNumber, TypeIntegerNumber:
		return "number"
	case TypeString:
		return "string"
	case TypeFunction, TypeClosure, TypeNativeFunction:
		return "function"
	case TypeObject, TypeDictObject:
		return "object"
	case TypeArray:
		return "object" // Arrays are objects in JavaScript
	default:
		return "object" // Default fallback
	}
}

// stringFromCharCodeStaticImpl implements String.fromCharCode (static method)
func stringFromCharCodeStaticImpl(args []Value) Value {
	if len(args) == 0 {
		return NewString("")
	}

	result := make([]byte, len(args))
	for i, arg := range args {
		code := int(arg.ToFloat()) & 0xFFFF // Mask to 16 bits like JS
		result[i] = byte(code)
	}

	return NewString(string(result))
}

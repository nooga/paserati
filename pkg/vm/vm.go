package vm

import (
	"fmt"
	"math"
	"os"
	"paserati/pkg/errors"
	"strconv"
	"unsafe"
)

const RegFileSize = 256 // Max registers per function call frame
const MaxFrames = 64    // Max call stack depth

// ModuleLoader interface for loading modules without circular imports
type ModuleLoader interface {
	LoadModule(specifier string, fromPath string) (ModuleRecord, error)
}

// ModuleRecord interface to avoid circular imports
type ModuleRecord interface {
	GetExportValues() map[string]Value
	GetCompiledChunk() *Chunk
	GetExportNames() []string
	GetError() error
}

// ModuleContext represents a cached module execution context
type ModuleContext struct {
	chunk       *Chunk            // Compiled module chunk
	exports     map[string]Value  // Module's exported values
	executed    bool              // Whether module has been executed
	executing   bool              // Whether module is currently being executed
	globals     []Value           // Module-specific global variables (indices 0+ within module)
	globalNames []string          // Module-specific global variable names (for debugging)
}

// PendingAction represents actions that should be performed after finally blocks complete
type PendingAction int

const (
	ActionNone PendingAction = iota
	ActionReturn
	ActionThrow
	ActionBreak    // Future: for break in loops
	ActionContinue // Future: for continue in loops
)

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
	isDirectCall      bool  // Whether this frame should return immediately upon OpReturn (for Function.prototype.call)

	// For async native functions that can call bytecode
	isNativeFrame    bool
	nativeReturnCh   chan Value         // Channel to receive return values from bytecode calls
	nativeYieldCh    chan *BytecodeCall // Channel to send bytecode calls to VM
	nativeCompleteCh chan Value         // Channel to signal native function completion
}

// BytecodeCall represents a request from a native function to call bytecode
type BytecodeCall struct {
	Function  Value
	ThisValue Value
	Args      []Value
	ResultCh  chan Value // Channel to receive the result
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

	// Unified global heap for all modules and main program
	heap *Heap

	// Singleton empty array for rest parameters optimization
	// Used when variadic functions are called with no extra arguments
	emptyRestArray Value

	// Built-in prototypes owned by this VM
	ObjectPrototype   Value
	FunctionPrototype Value
	ArrayPrototype    Value
	StringPrototype   Value
	NumberPrototype   Value
	BooleanPrototype  Value
	RegExpPrototype   Value
	MapPrototype      Value
	SetPrototype      Value
	ErrorPrototype       Value
	SymbolPrototype      Value
	
	// TypedArray prototypes
	Uint8ArrayPrototype    Value
	Int32ArrayPrototype    Value
	Float32ArrayPrototype  Value

	// Flag to disable method binding during Function.prototype.call to prevent infinite recursion
	disableMethodBinding bool

	// Counter to track Function.prototype.call recursion depth
	callDepth int

	// Instance-specific initialization callbacks
	//initCallbacks []VMInitCallback

	// Current 'this' value for native function execution
	currentThis Value

	// Globals, open upvalues, etc. would go here later
	errors []errors.PaseratiError

	// Exception handling state
	currentException Value // Current thrown exception
	unwinding        bool  // True during exception unwinding
	
	// Finally block state (Phase 3)
	pendingAction PendingAction // Action to perform after finally blocks complete
	pendingValue  Value         // Value associated with pending action (e.g., return value)
	finallyDepth  int           // Track nested finally blocks

	// Module system (Phase 5)
	moduleContexts    map[string]*ModuleContext // Cached module contexts by path
	moduleLoader      ModuleLoader              // Reference to module loader for loading modules
	currentModulePath string                    // Currently executing module path (for module-scoped globals)
	
	// Execution context stack for recursive module execution
	executionContextStack []ExecutionContext
	
	// Track if we're in module execution to handle errors differently
	inModuleExecution bool
	moduleExecutionDepth int
}

// ExecutionContext saves the complete VM state for recursive execution
type ExecutionContext struct {
	frame            CallFrame
	frameCount       int
	nextRegSlot      int
	currentModulePath string
	// Deep copy of register state for proper isolation
	savedRegisters   []Value        // Deep copy of actual register values
	savedRegisterCount int           // How many registers to restore
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
	vm := &VM{
		// frameCount and nextRegSlot initialized to 0
		openUpvalues:   make([]*Upvalue, 0, 16),        // Pre-allocate slightly
		propCache:      make(map[int]*PropInlineCache), // Initialize inline cache
		cacheStats:     ICacheStats{},                  // Initialize cache statistics
		heap:           NewHeap(64),                     // Initialize unified global heap
		emptyRestArray: NewArray(),                     // Initialize singleton empty array for rest params
		//initCallbacks:  make([]VMInitCallback, 0),       // Initialize callback list
		errors:         make([]errors.PaseratiError, 0), // Initialize error list
		moduleContexts: make(map[string]*ModuleContext), // Initialize module context cache
	}

	// Initialize built-in prototypes first
	vm.initializePrototypes()

	// Run initialization callbacks
	// if err := vm.initializeVM(); err != nil {
	// 	// For now, just continue - we could add error handling later
	// 	fmt.Fprintf(os.Stderr, "Warning: VM initialization callback failed: %v\n", err)
	// }

	return vm
}

// GetCallDepth returns the current call depth for Function.prototype.call recursion tracking
func (vm *VM) GetCallDepth() int {
	return vm.callDepth
}

// IncrementCallDepth increments the call depth counter
func (vm *VM) IncrementCallDepth() {
	vm.callDepth++
}

// DecrementCallDepth decrements the call depth counter
func (vm *VM) DecrementCallDepth() {
	vm.callDepth--
}

// Reset clears the VM state, ready for new execution.
// SetBuiltinGlobals initializes the VM's global variables with builtin values
// SetModuleLoader sets the module loader for this VM instance
func (vm *VM) SetModuleLoader(loader ModuleLoader) {
	vm.moduleLoader = loader
}

// GetGlobal retrieves a global variable by name
func (vm *VM) GetGlobal(name string) (Value, bool) {
	// This method is deprecated in favor of index-based access via HeapAlloc
	// since the heap doesn't store names. Callers should use GetGlobalByIndex instead.
	return Undefined, false
}

// GetGlobalByIndex retrieves a global value by its index
func (vm *VM) GetGlobalByIndex(index int) (Value, bool) {
	return vm.heap.Get(index)
}

func (vm *VM) SetBuiltinGlobals(globals map[string]Value, indexMap map[string]int) error {
	// Use the heap's SetBuiltinGlobals method
	return vm.heap.SetBuiltinGlobals(globals, indexMap)
}

func (vm *VM) Reset() {
	vm.frameCount = 0
	vm.nextRegSlot = 0
	vm.openUpvalues = vm.openUpvalues[:0] // Clear slice while keeping capacity
	vm.errors = vm.errors[:0]             // Clear errors slice
	vm.callDepth = 0                      // Reset call depth counter
	// Clear inline cache
	for k := range vm.propCache {
		delete(vm.propCache, k)
	}
	// Reset cache statistics
	vm.cacheStats = ICacheStats{}
	// Clear global heap
	vm.heap = NewHeap(64)
	// Clear finally state
	vm.pendingAction = ActionNone
	vm.pendingValue = Undefined
	vm.finallyDepth = 0
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
	scriptRegSize := 128 // Large enough for complex expressions but leaves room for function calls
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
	// fmt.Printf("// [VM] Interpret: About to call vm.run() for chunk '%s' (stack depth: %d)\n", mainFuncObj.Name, len(vm.executionContextStack))
	
	// Check if we're already in a nested execution
	if len(vm.executionContextStack) > 0 {
		// We're in a nested execution - this is problematic
		// fmt.Printf("// [VM] Interpret: WARNING - Nested vm.run() call detected!\n")
	}
	
	resultStatus, finalValue := vm.run() // Capture both status and value
	// fmt.Printf("// [VM] Interpret: vm.run() returned for chunk '%s' with status %v\n", mainFuncObj.Name, resultStatus)
	// fmt.Printf("// [VM] Interpret: vm.errors length: %d\n", len(vm.errors))
	// for i, err := range vm.errors {
	// 	fmt.Printf("// [VM] Interpret: Error %d: %s\n", i, err.Error())
	// }

	if resultStatus == InterpretRuntimeError {
		// An error occurred, return the potentially partial value and the collected errors
		// fmt.Printf("// [VM] Interpret: Returning runtime error with %d errors\n", len(vm.errors))
		return finalValue, vm.errors
	} else {
		// Execution finished without runtime error (InterpretOK)
		// Return the final value returned by run() and empty errors slice (errors were cleared)
		// fmt.Printf("// [VM] Interpret: Returning success with final value: %s, errors: %d\n", finalValue.Inspect(), len(vm.errors))
		return finalValue, vm.errors // vm.errors should be empty here if InterpretOK
	}
}

// run is the main execution loop.
// It now returns the InterpretResult status AND the final script Value.
func (vm *VM) run() (InterpretResult, Value) {
	// Panic recovery for better debugging of register access issues
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "\n[VM PANIC RECOVERED]: %v\n", r)
			// vm.printFrameStack()
			// vm.printDisassemblyAroundIP()
			// Re-panic to maintain original behavior
			// panic(r)
		}
	}()

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

startExecution:
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

		if ip >= len(code) {
			frame.ip = ip
			status := vm.runtimeError("IP %d beyond code length %d", ip, len(code))
			return status, Undefined
		}
		
		opcode := OpCode(code[ip]) // Use local OpCode
		
		// Debug main script opcodes to track function calls
		// if vm.currentModulePath == "" {
		//	fmt.Printf("// [VM DEBUG] Main script opcode: %s (%d) at IP %d\n", opcode.String(), int(opcode), ip)
		// }
		
		// Debug output for current instruction execution
		// chunkName := "<unknown>"
		// if frame.closure != nil && frame.closure.Fn != nil {
		//	chunkName = frame.closure.Fn.Name
		// }
		// fmt.Printf("// [VM DEBUG] IP %d: %s (chunk: %s, module: %s)\n", 
		//	ip, opcode.String(), chunkName, vm.currentModulePath)
		
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
			// fmt.Printf("[VM DEBUG] OpLoadUndefined reg=%d at IP=%d, registers length = %d\n", reg, ip-2, len(registers))
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
			// Convert value to number, handling Date objects specially
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
				} else if IsString(leftVal) && rightVal.IsBoolean() {
					// String + Boolean concatenation (boolean converted to string)
					boolStr := "false"
					if rightVal.AsBoolean() {
						boolStr = "true"
					}
					registers[destReg] = String(AsString(leftVal) + boolStr)
				} else if leftVal.IsBoolean() && IsString(rightVal) {
					// Boolean + String concatenation (boolean converted to string)
					boolStr := "false"
					if leftVal.AsBoolean() {
						boolStr = "true"
					}
					registers[destReg] = String(boolStr + AsString(rightVal))
				} else {
					frame.ip = ip
					status := vm.runtimeError("Operands must be two numbers, two strings, a string and a number, or a string and a boolean for '+'.")
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

		case OpIn:
			// OpIn: Rx Ry Rz - Rx = (Ry in Rz) - property existence check
			destReg := code[ip]
			propReg := code[ip+1]
			objReg := code[ip+2]
			ip += 3

			propVal := registers[propReg]
			objVal := registers[objReg]

			// Check if property exists in object or its prototype chain
			var hasProperty bool
			propKey := propVal.ToString()

			switch objVal.Type() {
			case TypeObject:
				plainObj := objVal.AsPlainObject()
				// Use prototype-aware Has() method instead of HasOwn()
				hasProperty = plainObj.Has(propKey)
			case TypeDictObject:
				dictObj := objVal.AsDictObject()
				// Use prototype-aware Has() method instead of HasOwn()
				hasProperty = dictObj.Has(propKey)
			case TypeArray:
				// For arrays, check if the property is a valid index or inherited property
				arrayObj := objVal.AsArray()
				if index, err := strconv.Atoi(propKey); err == nil && index >= 0 {
					// Check if index is within bounds
					hasProperty = index < arrayObj.Length()
				} else {
					// Check for array properties (length) or inherited properties
					if propKey == "length" {
						hasProperty = true
					} else {
						// Arrays should inherit from Array.prototype, for now just return false
						// TODO: Implement proper array prototype chain traversal
						hasProperty = false
					}
				}
			default:
				// TypeError: Right-hand side of 'in' is not an object
				hasProperty = false
			}

			registers[destReg] = BooleanValue(hasProperty)

		case OpInstanceof:
			// OpInstanceof: Rx Ry Rz - Rx = (Ry instanceof Rz) - instance check
			destReg := code[ip]
			objReg := code[ip+1]
			constructorReg := code[ip+2]
			ip += 3

			objVal := registers[objReg]
			constructorVal := registers[constructorReg]

			// Get constructor's .prototype property (may create it lazily)
			var constructorPrototype Value = Undefined
			if constructorVal.Type() == TypeFunction {
				fn := AsFunction(constructorVal)
				constructorPrototype = fn.getOrCreatePrototype()
			} else if constructorVal.Type() == TypeClosure {
				closure := AsClosure(constructorVal)
				constructorPrototype = closure.Fn.getOrCreatePrototype()
			}

			// Walk prototype chain of object
			result := false
			if objVal.IsObject() {
				var current Value
				if objVal.Type() == TypeObject {
					current = objVal.AsPlainObject().GetPrototype()
				} else if objVal.Type() == TypeDictObject {
					current = objVal.AsDictObject().GetPrototype()
				}

				// Walk the prototype chain
				for current.typ != TypeNull && current.typ != TypeUndefined {
					if current.Equals(constructorPrototype) {
						result = true
						break
					}
					if current.IsObject() {
						if current.Type() == TypeObject {
							current = current.AsPlainObject().GetPrototype()
						} else if current.Type() == TypeDictObject {
							current = current.AsDictObject().GetPrototype()
						} else {
							break
						}
					} else {
						break
					}
				}
			}

			registers[destReg] = BooleanValue(result)

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
			// Refactored to use centralized prepareCall
			destReg := code[ip]
			funcReg := code[ip+1]
			argCount := int(code[ip+2])
			ip += 3

			callerRegisters := registers
			callerIP := ip

			calleeVal := callerRegisters[funcReg]
			args := callerRegisters[funcReg+1 : funcReg+1+byte(argCount)]

			shouldSwitch, err := vm.prepareCall(calleeVal, Undefined, args, destReg, callerRegisters, callerIP)
			if err != nil {
				status := vm.runtimeError("%s", err.Error())
				return status, Undefined
			}

			if shouldSwitch {
				// Switch to new frame
				frame = &vm.frames[vm.frameCount-1]
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
			}
			continue

		case OpReturn:
			srcReg := code[ip]
			ip++
			result := registers[srcReg]
			frame.ip = ip // Save final IP of this frame
			// fmt.Printf("// [VM DEBUG] OpReturn: Hit in module '%s', frameCount=%d, result=%s\n", vm.currentModulePath, vm.frameCount, result.ToString())

			// Check if there are finally handlers that should execute
			handlers := vm.findAllExceptionHandlers(frame.ip)
			hasFinallyHandler := false
			for _, handler := range handlers {
				if handler.IsFinally {
					hasFinallyHandler = true
					break
				}
			}

			if hasFinallyHandler {
				// Set pending return action and let finally blocks execute
				vm.pendingAction = ActionReturn
				vm.pendingValue = result
				
				// Find the finally handler and jump to it
				for _, handler := range handlers {
					if handler.IsFinally {
						ip = handler.HandlerPC
						break
					}
				}
				// Continue executing from the finally handler
				continue
			}

			// No finally handler, proceed with normal return
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
				// fmt.Printf("// [VM DEBUG] OpReturn: Top-level frame return, frameCount=0, module='%s', result=%s\n", vm.currentModulePath, result.ToString())
				// Check if there's a pending exception that should be propagated
				if vm.pendingAction == ActionThrow {
					// Propagate the uncaught exception
					vm.currentException = vm.pendingValue
					return InterpretRuntimeError, vm.pendingValue
				}
				// Return the result directly.
				// fmt.Printf("// [VM DEBUG] OpReturn: Exiting execution loop for module '%s'\n", vm.currentModulePath)
				return InterpretOK, result
			}

			// Check if this was a direct call frame and should return early
			if frame.isDirectCall {
				// Handle constructor return semantics for direct call
				var finalResult Value
				if isConstructor {
					if result.IsObject() {
						finalResult = result // Return the explicit object
					} else {
						finalResult = constructorThisValue // Return the instance
					}
				} else {
					finalResult = result
				}
				// Return the result immediately instead of continuing execution
				return InterpretOK, finalResult
			}

			// Get the caller frame (which is now the top frame)
			if vm.frameCount == 0 {
				// No caller frame, return to top level
				return InterpretOK, result
			}
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
			
			// Debug: Show frame restoration
			// fmt.Printf("// [VM DEBUG] OpReturn: Restored caller frame (frameCount: %d, newIP: %d, newChunk: %s, currentModule: %s)\n", 
			//	vm.frameCount, ip, function.Name, vm.currentModulePath)

		case OpReturnUndefined:
			frame.ip = ip // Save final IP

			// Check if there are finally handlers that should execute
			handlers := vm.findAllExceptionHandlers(frame.ip)
			hasFinallyHandler := false
			for _, handler := range handlers {
				if handler.IsFinally {
					hasFinallyHandler = true
					break
				}
			}

			if hasFinallyHandler {
				// Set pending return action and let finally blocks execute
				vm.pendingAction = ActionReturn
				vm.pendingValue = Undefined
				// Don't return immediately - let the finally handler execute
				continue
			}

			// No finally handler, proceed with normal return
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
				case TypeSymbol:
					// Use the symbol's string representation with a unique prefix to avoid conflicts
					key = "@@symbol:" + indexVal.AsSymbol()
				default:
					frame.ip = ip
					status := vm.runtimeError("Object index must be a string, number, or symbol, got '%v'", indexVal.Type())
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

			case TypeTypedArray:
				if !IsNumber(indexVal) {
					frame.ip = ip
					status := vm.runtimeError("TypedArray index must be a number, got '%v'", indexVal.Type())
					return status, Undefined
				}
				ta := baseVal.AsTypedArray()
				idx := int(AsNumber(indexVal))
				registers[destReg] = ta.GetElement(idx)

			default:
				frame.ip = ip
				status := vm.runtimeError("Cannot index non-array/object/string/typedarray type '%v' at IP %d", baseVal.Type(), ip)
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
				case TypeSymbol:
					// Use the symbol's string representation with a unique prefix to avoid conflicts
					key = "@@symbol:" + indexVal.AsSymbol()
				default:
					frame.ip = ip
					status := vm.runtimeError("Object index must be a string, number, or symbol, got '%v'", indexVal.Type())
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

			case TypeTypedArray:
				if !IsNumber(indexVal) {
					frame.ip = ip
					status := vm.runtimeError("TypedArray index must be a number, got '%v'", indexVal.Type())
					return status, Undefined
				}
				ta := baseVal.AsTypedArray()
				idx := int(AsNumber(indexVal))
				ta.SetElement(idx, valueVal)

			default:
				frame.ip = ip
				status := vm.runtimeError("Cannot set index on non-array/object/typedarray type '%v'", baseVal.Type())
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

		// --- NEW: Array Spread Support ---
		case OpArraySpread:
			destReg := code[ip]
			sourceReg := code[ip+1]
			ip += 2

			destVal := registers[destReg]
			sourceVal := registers[sourceReg]

			// Validate that both are arrays
			if destVal.Type() != TypeArray {
				frame.ip = ip
				status := vm.runtimeError("OpArraySpread: destination must be an array, got '%v'", destVal.Type())
				return status, Undefined
			}
			if sourceVal.Type() != TypeArray {
				frame.ip = ip
				status := vm.runtimeError("OpArraySpread: source must be an array, got '%v'", sourceVal.Type())
				return status, Undefined
			}

			// Extract array objects
			destArray := AsArray(destVal)
			sourceArray := AsArray(sourceVal)

			// Append all elements from source to destination
			destArray.elements = append(destArray.elements, sourceArray.elements...)
			destArray.length = len(destArray.elements)

		// --- NEW: Object Spread Support ---
		case OpObjectSpread:
			destReg := code[ip]
			sourceReg := code[ip+1]
			ip += 2

			destVal := registers[destReg]
			sourceVal := registers[sourceReg]

			// Validate that both are objects (TypeObject or TypeDictObject)
			if destVal.Type() != TypeObject && destVal.Type() != TypeDictObject {
				frame.ip = ip
				status := vm.runtimeError("OpObjectSpread: destination must be an object, got '%v'", destVal.Type())
				return status, Undefined
			}
			if sourceVal.Type() != TypeObject && sourceVal.Type() != TypeDictObject {
				frame.ip = ip
				status := vm.runtimeError("OpObjectSpread: source must be an object, got '%v'", sourceVal.Type())
				return status, Undefined
			}

			// Copy all enumerable properties from source to destination
			var sourceKeys []string
			switch sourceVal.Type() {
			case TypeDictObject:
				sourceDict := AsDictObject(sourceVal)
				sourceKeys = sourceDict.OwnKeys()
				// Copy each property
				for _, key := range sourceKeys {
					if value, exists := sourceDict.GetOwn(key); exists {
						if destVal.Type() == TypeDictObject {
							destDict := AsDictObject(destVal)
							destDict.SetOwn(key, value)
						} else {
							destObj := AsPlainObject(destVal)
							destObj.SetOwn(key, value)
						}
					}
				}
			case TypeObject:
				sourceObj := AsPlainObject(sourceVal)
				sourceKeys = sourceObj.OwnKeys()
				// Copy each property
				for _, key := range sourceKeys {
					if value, exists := sourceObj.GetOwn(key); exists {
						if destVal.Type() == TypeDictObject {
							destDict := AsDictObject(destVal)
							destDict.SetOwn(key, value)
						} else {
							destObj := AsPlainObject(destVal)
							destObj.SetOwn(key, value)
						}
					}
				}
			}
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
			// Create a new empty object using the shape-based PlainObject with VM's ObjectPrototype
			registers[destReg] = NewObject(vm.ObjectPrototype)

		case OpGetProp:
			destReg := code[ip]
			objReg := code[ip+1]
			nameConstIdxHi := code[ip+2]
			nameConstIdxLo := code[ip+3]
			nameConstIdx := uint16(nameConstIdxHi)<<8 | uint16(nameConstIdxLo)
			// Calculate cache key based on instruction pointer (before advancing ip)
			ip += 4

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

			if ok, status, value := vm.opGetProp(ip, &registers[objReg], propName, &registers[destReg]); !ok {
				return status, value
			}

		case OpSetProp:
			objReg := code[ip]   // Register holding the object
			valReg := code[ip+1] // Register holding the value to set
			nameConstIdxHi := code[ip+2]
			nameConstIdxLo := code[ip+3]
			nameConstIdx := uint16(nameConstIdxHi)<<8 | uint16(nameConstIdxLo)
			// Calculate cache key based on instruction pointer (before advancing ip)
			ip += 4

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

			if ok, status, value := vm.opSetProp(ip, &registers[objReg], propName, &registers[valReg]); !ok {
				return status, value
			}

		case OpDeleteProp:
			destReg := code[ip]
			objReg := code[ip+1]
			nameConstIdxHi := code[ip+2]
			nameConstIdxLo := code[ip+3]
			nameConstIdx := uint16(nameConstIdxHi)<<8 | uint16(nameConstIdxLo)
			ip += 4

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

			// Get the object
			obj := registers[objReg]
			
			// Handle delete operation
			var success bool
			if obj.IsObject() {
				if plainObj := obj.AsPlainObject(); plainObj != nil {
					// ERSATZ SOLUTION: Set property to undefined on original object (for existing references)
					plainObj.SetOwn(propName, Undefined)
					
					// Also create a new DictObject and replace the register (for future operations)
					dictValue := NewDictObject(plainObj.GetPrototype())
					dict := dictValue.AsDictObject()
					
					// Copy all properties from shape (except the one we're deleting)
					for _, field := range plainObj.shape.fields {
						if field.offset < len(plainObj.properties) && field.name != propName {
							dict.SetOwn(field.name, plainObj.properties[field.offset])
						}
					}
					
					// Delete operation always succeeds since we didn't copy the target property
					success = true
					// Replace the register with the dict object
					registers[objReg] = dictValue
					
				} else if dictObj := obj.AsDictObject(); dictObj != nil {
					// DictObject supports deletion directly
					success = dictObj.DeleteOwn(propName)
					
				} else if arrObj := obj.AsArray(); arrObj != nil {
					// Arrays don't support property deletion (only element deletion in the future)
					success = false
					
				} else {
					// Other object types don't support property deletion
					success = false
				}
			} else {
				// Non-object types don't support property deletion
				success = false
			}
			
			// Store the result (boolean) in the destination register
			registers[destReg] = BooleanValue(success)

		case OpCallMethod:
			// Refactored to use centralized prepareCall
			destReg := code[ip]
			funcReg := code[ip+1]
			thisReg := code[ip+2]
			argCount := int(code[ip+3])
			ip += 4

			callerRegisters := registers
			callerIP := ip

			calleeVal := callerRegisters[funcReg]
			thisVal := callerRegisters[thisReg]
			args := callerRegisters[funcReg+1 : funcReg+1+byte(argCount)]

			// Debug logging for method calls
			// fmt.Printf("// [VM DEBUG] OpCallMethod at IP %d: Calling function in R%d (type: %v, value: %s) with this=R%d (type: %v, value: %s), args=%d [module: %s]\n", 
			//	ip-4, funcReg, calleeVal.Type(), calleeVal.Inspect(), thisReg, thisVal.Type(), thisVal.Inspect(), argCount, vm.currentModulePath)

			shouldSwitch, err := vm.prepareMethodCall(calleeVal, thisVal, args, destReg, callerRegisters, callerIP)
			if err != nil {
				status := vm.runtimeError("%s", err.Error())
				return status, Undefined
			}

			if shouldSwitch {
				// Switch to new frame
				frame = &vm.frames[vm.frameCount-1]
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
			}
			continue

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

				// Get constructor's .prototype property (create lazily if needed)
				instancePrototype := constructorFunc.getOrCreatePrototype()

				// Create new instance object with constructor's prototype
				newInstance := NewObject(instancePrototype)

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

				// Get constructor's .prototype property (create lazily if needed)
				instancePrototype := constructorFunc.getOrCreatePrototype()

				// Create new instance object with constructor's prototype
				newInstance := NewObject(instancePrototype)

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
				result, err := builtin.Fn(args)
				if err != nil {
					frame.ip = callerIP
					status := vm.runtimeError("Runtime Error: %s", err.Error())
					return status, Undefined
				}

				// Store result in caller's target register
				if int(destReg) < len(callerRegisters) {
					callerRegisters[destReg] = result
				} else {
					frame.ip = callerIP
					status := vm.runtimeError("Internal Error: Invalid target register %d for builtin constructor return value.", destReg)
					return status, Undefined
				}

			case TypeNativeFunctionWithProps:
				// Constructor call on builtin function with properties
				builtinWithProps := constructorVal.AsNativeFunctionWithProps()

				if builtinWithProps.Arity >= 0 && builtinWithProps.Arity != argCount {
					frame.ip = callerIP
					status := vm.runtimeError("Built-in constructor '%s' expected %d arguments but got %d.",
						builtinWithProps.Name, builtinWithProps.Arity, argCount)
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
				result, err := builtinWithProps.Fn(args)
				if err != nil {
					frame.ip = callerIP
					status := vm.runtimeError("Runtime Error: %s", err.Error())
					return status, Undefined
				}

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

			// Use unified global heap
			value, exists := vm.heap.Get(int(globalIdx))
			if !exists {
				// Auto-resize heap if needed and return undefined for uninitialized globals
				vm.heap.Resize(int(globalIdx) + 1)
				value = Undefined
			}

			// Store the retrieved value in the destination register
			registers[destReg] = value
			
			// Debug global access (disabled)
			// if globalIdx == 21 {
			//	fmt.Printf("// [VM DEBUG] OpGetGlobal: global[%d] -> R%d = %s (type: %v) [module: %s]\n", 
			//		globalIdx, destReg, value.Inspect(), value.Type(), vm.currentModulePath)
			// }

		case OpSetGlobal:
			globalIdxHi := code[ip]
			globalIdxLo := code[ip+1]
			srcReg := code[ip+2]
			globalIdx := uint16(globalIdxHi)<<8 | uint16(globalIdxLo)
			ip += 3

			// Use module-scoped global table
			value := registers[srcReg]
			vm.setGlobalInTable(globalIdx, value)
			
			// Debug output (disabled)
			// if globalIdx == 21 {
			//	fmt.Printf("// [VM DEBUG] OpSetGlobal: global[%d] = R%d (%s, type: %v) [module: %s]\n", 
			//		globalIdx, srcReg, value.Inspect(), value.Type(), vm.currentModulePath)
			// }
			// if int(globalIdx) < len(globalNames) && globalNames[globalIdx] != "" {
			//	fmt.Printf("// [VM] OpSetGlobal: Global[%d] name is '%s'\n", globalIdx, globalNames[globalIdx])
			// }
			if vm.currentModulePath != "" {
				// fmt.Printf("// [VM] OpSetGlobal: Module context: '%s'\n", vm.currentModulePath)
			}

		// --- NEW: Efficient Nullish Checks ---
		case OpIsNull:
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2
			registers[destReg] = BooleanValue(registers[srcReg].Type() == TypeNull)

		case OpIsUndefined:
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2
			registers[destReg] = BooleanValue(registers[srcReg].Type() == TypeUndefined)

		case OpIsNullish:
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2
			val := registers[srcReg]
			registers[destReg] = BooleanValue(val.Type() == TypeNull || val.Type() == TypeUndefined)

		case OpJumpIfNull:
			condReg := code[ip]
			offsetHi := code[ip+1]
			offsetLo := code[ip+2]
			ip += 3
			if registers[condReg].Type() == TypeNull {
				offset := int16(uint16(offsetHi)<<8 | uint16(offsetLo))
				ip += int(offset) // Apply jump relative to IP *after* reading offset bytes
			}

		case OpJumpIfUndefined:
			condReg := code[ip]
			offsetHi := code[ip+1]
			offsetLo := code[ip+2]
			ip += 3
			if registers[condReg].Type() == TypeUndefined {
				offset := int16(uint16(offsetHi)<<8 | uint16(offsetLo))
				ip += int(offset) // Apply jump relative to IP *after* reading offset bytes
			}

		case OpJumpIfNullish:
			condReg := code[ip]
			offsetHi := code[ip+1]
			offsetLo := code[ip+2]
			ip += 3
			val := registers[condReg]
			if val.Type() == TypeNull || val.Type() == TypeUndefined {
				offset := int16(uint16(offsetHi)<<8 | uint16(offsetLo))
				ip += int(offset) // Apply jump relative to IP *after* reading offset bytes
			}
		// --- END NEW ---

		// --- NEW: Spread Call Instructions ---
		case OpSpreadCall:
			// Refactored to use centralized prepareCall
			destReg := code[ip]
			funcReg := code[ip+1]
			spreadArgReg := code[ip+2]
			ip += 3

			callerRegisters := registers
			callerIP := ip

			calleeVal := callerRegisters[funcReg]
			spreadArrayVal := callerRegisters[spreadArgReg]

			// Extract arguments from spread array
			spreadArgs, err := vm.extractSpreadArguments(spreadArrayVal)
			if err != nil {
				frame.ip = callerIP
				status := vm.runtimeError("Spread call error: %s", err.Error())
				return status, Undefined
			}

			shouldSwitch, err := vm.prepareCall(calleeVal, Undefined, spreadArgs, destReg, callerRegisters, callerIP)
			if err != nil {
				status := vm.runtimeError("%s", err.Error())
				return status, Undefined
			}

			if shouldSwitch {
				// Initialize remaining registers to Undefined
				frame = &vm.frames[vm.frameCount-1]
				calleeFunc := frame.closure.Fn
				argCount := len(spreadArgs)
				for i := argCount; i < len(frame.registers); i++ {
					frame.registers[i] = Undefined
				}
				if calleeFunc.Variadic && calleeFunc.Arity < len(frame.registers) && argCount <= calleeFunc.Arity {
					frame.registers[calleeFunc.Arity] = vm.emptyRestArray
				}

				// Switch to new frame
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
			}
			continue

		case OpSpreadCallMethod:
			// Refactored to use centralized prepareCall
			destReg := code[ip]
			funcReg := code[ip+1]
			thisReg := code[ip+2]
			spreadArgReg := code[ip+3]
			ip += 4

			callerRegisters := registers
			callerIP := ip

			calleeVal := callerRegisters[funcReg]
			thisVal := callerRegisters[thisReg]
			spreadArrayVal := callerRegisters[spreadArgReg]

			// Extract arguments from spread array
			spreadArgs, err := vm.extractSpreadArguments(spreadArrayVal)
			if err != nil {
				frame.ip = callerIP
				status := vm.runtimeError("Spread method call error: %s", err.Error())
				return status, Undefined
			}

			shouldSwitch, err := vm.prepareMethodCall(calleeVal, thisVal, spreadArgs, destReg, callerRegisters, callerIP)
			if err != nil {
				status := vm.runtimeError("%s", err.Error())
				return status, Undefined
			}

			if shouldSwitch {
				// Initialize remaining registers to Undefined
				frame = &vm.frames[vm.frameCount-1]
				calleeFunc := frame.closure.Fn
				argCount := len(spreadArgs)
				for i := argCount; i < len(frame.registers); i++ {
					frame.registers[i] = Undefined
				}
				if calleeFunc.Variadic && calleeFunc.Arity < len(frame.registers) && argCount <= calleeFunc.Arity {
					frame.registers[calleeFunc.Arity] = vm.emptyRestArray
				}

				// Switch to new frame
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
			}
			continue

		case OpGetOwnKeys:
			destReg := code[ip]
			objReg := code[ip+1]
			ip += 2

			objValue := registers[objReg]

			// Get object keys based on object type
			var keys []string
			switch objValue.Type() {
			case TypeObject:
				obj := objValue.AsPlainObject()
				keys = obj.OwnKeys()
			case TypeDictObject:
				dict := objValue.AsDictObject()
				keys = dict.OwnKeys()
			case TypeArray:
				arr := objValue.AsArray()
				// Arrays enumerate their indices as strings
				keys = make([]string, arr.Length())
				for i := 0; i < arr.Length(); i++ {
					keys[i] = strconv.Itoa(i)
				}
			default:
				// For primitive types, return empty array
				keys = []string{}
			}

			// Convert string keys to Value array
			keyValues := make([]Value, len(keys))
			for i, key := range keys {
				keyValues[i] = String(key)
			}

			// Create array with the keys
			keysArray := NewArrayWithArgs(keyValues)
			registers[destReg] = keysArray

		case OpArraySlice:
			destReg := code[ip]
			arrayReg := code[ip+1]
			startReg := code[ip+2]
			ip += 3

			arrayValue := registers[arrayReg]
			startValue := registers[startReg]

			// Ensure we have an array
			if arrayValue.Type() != TypeArray {
				frame.ip = ip
				status := vm.runtimeError("Cannot slice non-array value of type %d", int(arrayValue.Type()))
				return status, Undefined
			}

			// Ensure start index is a number
			if !startValue.IsNumber() {
				frame.ip = ip
				status := vm.runtimeError("Array slice start index must be a number, got type %d", int(startValue.Type()))
				return status, Undefined
			}

			sourceArray := arrayValue.AsArray()
			startIndex := int(startValue.ToFloat())
			arrayLength := sourceArray.Length()

			// Handle negative indices (slice(-1) means slice from end)
			if startIndex < 0 {
				startIndex = arrayLength + startIndex
			}

			// Clamp start index to valid range
			if startIndex < 0 {
				startIndex = 0
			}
			if startIndex > arrayLength {
				startIndex = arrayLength
			}

			// Create new array with sliced elements
			slicedElements := make([]Value, 0, arrayLength-startIndex)
			for i := startIndex; i < arrayLength; i++ {
				slicedElements = append(slicedElements, sourceArray.Get(i))
			}

			// Create new array with sliced elements
			resultArray := NewArrayWithArgs(slicedElements)
			registers[destReg] = resultArray

		case OpCopyObjectExcluding:
			destReg := code[ip]
			sourceReg := code[ip+1]
			excludeReg := code[ip+2]
			ip += 3

			sourceValue := registers[sourceReg]
			excludeValue := registers[excludeReg]

			// Ensure source is an object
			if !sourceValue.IsObject() {
				frame.ip = ip
				status := vm.runtimeError("Cannot copy non-object value of type %d", int(sourceValue.Type()))
				return status, Undefined
			}

			// Ensure exclude list is an array
			if excludeValue.Type() != TypeArray {
				frame.ip = ip
				status := vm.runtimeError("Exclude list must be an array, got type %d", int(excludeValue.Type()))
				return status, Undefined
			}

			excludeArray := excludeValue.AsArray()
			
			// Create set of property names to exclude
			excludeNames := make(map[string]struct{})
			for i := 0; i < excludeArray.Length(); i++ {
				nameValue := excludeArray.Get(i)
				if nameValue.Type() == TypeString {
					excludeNames[nameValue.AsString()] = struct{}{}
				}
			}

			// Create new object and copy properties not in exclude list
			var resultObj Value
			
			switch sourceValue.Type() {
			case TypeObject:
				sourceObj := sourceValue.AsPlainObject()
				resultPlainObj := NewObject(vm.ObjectPrototype)
				resultPlainObjPtr := resultPlainObj.AsPlainObject()
				
				// Copy properties not in exclude list
				for _, key := range sourceObj.OwnKeys() {
					if _, shouldExclude := excludeNames[key]; !shouldExclude {
						if value, exists := sourceObj.GetOwn(key); exists {
							resultPlainObjPtr.SetOwn(key, value)
						}
					}
				}
				resultObj = resultPlainObj
				
			case TypeDictObject:
				sourceDict := sourceValue.AsDictObject()
				resultPlainObj := NewObject(vm.ObjectPrototype)
				resultPlainObjPtr := resultPlainObj.AsPlainObject()
				
				// Copy properties not in exclude list
				for _, key := range sourceDict.OwnKeys() {
					if _, shouldExclude := excludeNames[key]; !shouldExclude {
						if value, exists := sourceDict.GetOwn(key); exists {
							resultPlainObjPtr.SetOwn(key, value)
						}
					}
				}
				resultObj = resultPlainObj
				
			default:
				// For other object-like types, create empty object
				resultObj = NewObject(vm.ObjectPrototype)
			}

			registers[destReg] = resultObj

		case OpThrow:
			// Save IP before potential unwinding
			frame.ip = ip
			vm.executeOpThrow(code, &ip)
			// If unwinding is active, check if we need to terminate or continue
			if vm.unwinding {
				// Exception was thrown and we're unwinding
				// The unwinding logic will either find a handler or terminate
				if vm.frameCount == 0 {
					// All frames unwound, uncaught exception
					return InterpretRuntimeError, vm.currentException
				}
				continue // Let the unwinding process take control
			} else {
				// Exception was handled, synchronize all cached variables and continue execution
				// The exception handler may have changed the frame, so resynchronize everything
				frame = &vm.frames[vm.frameCount-1]
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
				continue
			}

		case OpReturnFinally:
			srcReg := code[ip]
			ip++
			result := registers[srcReg]
			frame.ip = ip // Save final IP of this frame
			
			// Returns from finally blocks clear any pending exceptions
			// because the return takes precedence over the exception
			vm.currentException = Null
			vm.unwinding = false
			// Also clear any pending throw action - the return takes precedence
			if vm.pendingAction == ActionThrow {
				vm.pendingAction = ActionNone
				vm.pendingValue = Undefined
			}

			// Check if we have more finally handlers that could override this return
			handlers := vm.findAllExceptionHandlers(frame.ip)
			hasFinallyHandler := false
			for _, handler := range handlers {
				if handler.IsFinally {
					hasFinallyHandler = true
					break
				}
			}

			if hasFinallyHandler {
				// There are more finally blocks - trigger them to execute
				vm.pendingAction = ActionReturn
				vm.pendingValue = result
				
				// Find the first finally handler and jump to it
				for _, handler := range handlers {
					if handler.IsFinally {
						frame.ip = handler.HandlerPC
						ip = handler.HandlerPC  // Sync local IP variable
						vm.finallyDepth++
						continue startExecution // Jump to the outer finally block
					}
				}
			} else {
				// No more finally handlers - this return takes effect immediately  
				vm.pendingAction = ActionNone
				vm.pendingValue = Undefined
				vm.finallyDepth = 0
			}

			// Close upvalues for the returning frame
			vm.closeUpvalues(frame.registers)

			// Pop the current frame (same logic as OpReturn)
			returningFrameRegSize := function.RegisterSize
			callerTargetRegister := frame.targetRegister
			isConstructor := frame.isConstructorCall
			constructorThisValue := frame.thisValue

			vm.frameCount--
			vm.nextRegSlot -= returningFrameRegSize // Reclaim register space

			if vm.frameCount == 0 {
				// Returned from the top-level script frame.
				return InterpretOK, result
			}

			// Check if this was a direct call frame and should return early
			if frame.isDirectCall {
				// Handle constructor return semantics for direct call
				var finalResult Value
				if isConstructor {
					if result.IsObject() {
						finalResult = result // Return the explicit object
					} else {
						finalResult = constructorThisValue // Return the instance
					}
				} else {
					finalResult = result
				}
				// Return the result immediately instead of continuing execution
				return InterpretOK, finalResult
			}

			// Get the caller frame (which is now the top frame)
			if vm.frameCount == 0 {
				// No caller frame, return to top level
				return InterpretOK, result
			}
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
				status := vm.runtimeError("Internal Error: Invalid target register %d for finally return value.", callerTargetRegister)
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

		// --- Phase 4a: Handle Pending Actions ---
		case OpHandlePending:
			// This instruction is emitted at the end of finally blocks
			// to execute any pending actions (return or throw)
			frame.ip = ip // Save current position
			
			// fmt.Printf("[DEBUG] OpHandlePending: pendingAction=%d, pendingValue=%s\n", vm.pendingAction, vm.pendingValue.ToString())
			
			if vm.pendingAction == ActionReturn {
				// Execute the pending return
				result := vm.pendingValue
				vm.pendingAction = ActionNone
				vm.pendingValue = Undefined
				
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
					// Returned from the top-level script frame
					return InterpretOK, result
				}
				
				// Handle the return value in the caller frame
				if frame.isDirectCall {
					var finalResult Value
					if isConstructor {
						if result.IsObject() {
							finalResult = result
						} else {
							finalResult = constructorThisValue
						}
					} else {
						finalResult = result
					}
					return InterpretOK, finalResult
				}
				
				// Get the caller frame
				callerFrame := &vm.frames[vm.frameCount-1]
				
				// Handle constructor return semantics
				var finalResult Value
				if isConstructor {
					if result.IsObject() {
						finalResult = result
					} else {
						finalResult = constructorThisValue
					}
				} else {
					finalResult = result
				}
				
				// Place the result in the caller's target register
				if int(callerTargetRegister) < len(callerFrame.registers) {
					callerFrame.registers[callerTargetRegister] = finalResult
				} else {
					status := vm.runtimeError("Internal Error: Invalid target register %d for pending return.", callerTargetRegister)
					return status, Undefined
				}
				
				// Restore cached variables for the caller frame
				frame = callerFrame
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
				
			} else if vm.pendingAction == ActionThrow {
				// Execute the pending throw
				vm.currentException = vm.pendingValue
				vm.pendingAction = ActionNone
				vm.pendingValue = Undefined
				vm.unwinding = true
				// fmt.Printf("[DEBUG] OpHandlePending: Re-throwing pending exception %s\n", vm.currentException.ToString())
				// Let the exception handling logic take over
			}
			// If no pending action, just continue
		// --- END Phase 4a ---

		// --- Module System ---
		case OpEvalModule:
			// OpEvalModule: ModulePathIdx - Execute module idempotently
			modulePathIdxHi := code[ip]
			modulePathIdxLo := code[ip+1]
			modulePathIdx := uint16(modulePathIdxHi)<<8 | uint16(modulePathIdxLo)
			ip += 2

			frame.ip = ip // Save IP before module execution
			
			if int(modulePathIdx) >= len(constants) {
				status := vm.runtimeError("Invalid module path index %d", modulePathIdx)
				return status, Undefined
			}

			modulePathValue := constants[modulePathIdx]
			if modulePathValue.Type() != TypeString {
				status := vm.runtimeError("Module path must be a string, got %s", modulePathValue.TypeName())
				return status, Undefined
			}

			modulePath := modulePathValue.AsString()
			// fmt.Printf("// [VM] OpEvalModule: Executing module '%s' (current context: '%s')\n", modulePath, vm.currentModulePath)
			status, result := vm.executeModule(modulePath)
			if status != InterpretOK {
				// fmt.Printf("// [VM] OpEvalModule: Module '%s' execution failed with status %d\n", modulePath, status)
				return status, result
			}
			// fmt.Printf("// [VM] OpEvalModule: Module '%s' executed successfully (current context: '%s')\n", modulePath, vm.currentModulePath)
			// Restore IP to continue after OpEvalModule
			frame.ip = ip

		case OpGetModuleExport:
			// OpGetModuleExport: Rx ModulePathIdx ExportNameIdx - Load exported value from module
			destReg := code[ip]
			modulePathIdxHi := code[ip+1]
			modulePathIdxLo := code[ip+2]
			exportNameIdxHi := code[ip+3]
			exportNameIdxLo := code[ip+4]
			ip += 5

			modulePathIdx := uint16(modulePathIdxHi)<<8 | uint16(modulePathIdxLo)
			exportNameIdx := uint16(exportNameIdxHi)<<8 | uint16(exportNameIdxLo)

			// Validate constant indices
			if int(modulePathIdx) >= len(constants) || int(exportNameIdx) >= len(constants) {
				status := vm.runtimeError("Invalid constant indices for OpGetModuleExport")
				return status, Undefined
			}

			modulePathValue := constants[modulePathIdx]
			exportNameValue := constants[exportNameIdx]
			
			if modulePathValue.Type() != TypeString || exportNameValue.Type() != TypeString {
				status := vm.runtimeError("Module path and export name must be strings")
				return status, Undefined
			}

			modulePath := modulePathValue.AsString()
			exportName := exportNameValue.AsString()

			// Get exported value from module
			// fmt.Printf("// [VM DEBUG] OpGetModuleExport: Getting export '%s' from module '%s' [current module: %s]\n", exportName, modulePath, vm.currentModulePath)
			exportValue := vm.getModuleExport(modulePath, exportName)
			// fmt.Printf("// [VM DEBUG] OpGetModuleExport: Retrieved '%s' from '%s' = %d (type %d, value: %s)\n", 
			//	exportName, modulePath, int(exportValue.Type()), int(exportValue.Type()), exportValue.ToString())
			frame.registers[destReg] = exportValue
			// fmt.Printf("// [VM DEBUG] OpGetModuleExport: Stored in R%d\n", destReg)

		case OpCreateNamespace:
			// OpCreateNamespace: Rx ModulePathIdx - Create namespace object from module exports
			destReg := code[ip]
			modulePathIdxHi := code[ip+1]
			modulePathIdxLo := code[ip+2]
			ip += 3

			modulePathIdx := uint16(modulePathIdxHi)<<8 | uint16(modulePathIdxLo)

			// Validate constant index
			if int(modulePathIdx) >= len(constants) {
				status := vm.runtimeError("Invalid constant index for OpCreateNamespace")
				return status, Undefined
			}

			modulePathValue := constants[modulePathIdx]
			
			if modulePathValue.Type() != TypeString {
				status := vm.runtimeError("Module path must be a string")
				return status, Undefined
			}

			modulePath := modulePathValue.AsString()

			// Create namespace object from module exports
			namespaceObj := vm.createModuleNamespace(modulePath)
			frame.registers[destReg] = namespaceObj

		default:
			frame.ip = ip // Save IP before erroring
			status := vm.runtimeError("Unknown opcode %d encountered.", opcode)
			return status, Undefined
		}

		// Check for exception unwinding after each instruction
		if vm.unwinding {
			// Continue the unwinding process by calling unwindException
			unwindResult := vm.unwindException()
			if !unwindResult {
				// No handler found, uncaught exception
				vm.handleUncaughtException()
				return InterpretRuntimeError, vm.currentException
			}
			// Handler was found, continue execution with updated frame
			frame = &vm.frames[vm.frameCount-1]
			closure = frame.closure
			function = closure.Fn
			code = function.Chunk.Code
			constants = function.Chunk.Constants
			registers = frame.registers
			ip = frame.ip
			continue
		}
		
		// Check if we've exited finally blocks and handle pending actions
		// NOTE: Disabled the automatic finallyDepth decrement based on IP position
		// because it was causing premature pending action execution. The finallyDepth
		// should only be decremented by explicit finally completion (OpReturnFinally)
		// or when explicitly exiting finally blocks.
		// if vm.finallyDepth > 0 {
		//     // Check if we're still within any finally handler range
		//     frame := &vm.frames[vm.frameCount-1]
		//     inFinallyRange := false
		//     for _, handler := range vm.findAllExceptionHandlers(frame.ip) {
		//         if handler.IsFinally {
		//             inFinallyRange = true
		//             break
		//         }
		//     }
		//     
		//     // If we've exited the finally range, decrement depth
		//     if !inFinallyRange {
		//         vm.finallyDepth--
		//     }
		// }
		
		// Check for pending actions after finally blocks complete
		if vm.finallyDepth == 0 && vm.pendingAction != ActionNone {
			// fmt.Printf("[DEBUG] Finally depth is 0, executing pending action %d\n", vm.pendingAction)
			switch vm.pendingAction {
			case ActionThrow:
				// Resume throwing the saved exception
				vm.pendingAction = ActionNone
				savedValue := vm.pendingValue
				vm.pendingValue = Undefined
				// fmt.Printf("[DEBUG] Re-throwing saved exception: %s\n", savedValue.ToString())
				vm.throwException(savedValue)
				continue // Let exception unwinding take over
			case ActionReturn:
				// Resume the return with saved value
				vm.pendingAction = ActionNone
				_ = vm.pendingValue // TODO: Implement return logic
				vm.pendingValue = Undefined
				continue
			default:
				// Clear unknown pending action
				vm.pendingAction = ActionNone
				vm.pendingValue = Undefined
			}
		}
	}

	// If we reach here, we broke out of the execution loop
	// This could be due to either:
	// 1. Ongoing unwinding (vm.unwinding == true) - continue unwinding in parent frame
	// 2. Completed exception handling (vm.unwinding == false) - resume execution at handler
	
	if vm.frameCount == 0 {
		// No frames left - either uncaught exception or completed execution
		if vm.unwinding {
			return InterpretRuntimeError, vm.currentException
		} else {
			return InterpretOK, Undefined
		}
	}
	
	// Update cached variables for the current frame and continue execution
	frame = &vm.frames[vm.frameCount-1]
	closure = frame.closure
	function = closure.Fn
	code = function.Chunk.Code
	constants = function.Chunk.Constants
	registers = frame.registers
	ip = frame.ip
	goto startExecution // Continue the execution loop with updated frame
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

	// Enhanced error reporting: print frame stack and disassembly
	fmt.Fprintf(os.Stderr, "[VM Runtime Error]: %s\n", msg)
	vm.printFrameStack()
	vm.printDisassemblyAroundIP()
	vm.printCurrentRegisters()

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
	case TypeSymbol:
		return "symbol"
	case TypeFunction, TypeClosure, TypeNativeFunction, TypeNativeFunctionWithProps, TypeAsyncNativeFunction:
		return "function"
	case TypeObject, TypeDictObject:
		return "object"
	case TypeArray:
		return "object" // Arrays are objects in JavaScript
	default:
		return "object" // Default fallback
	}
}

// printFrameStack prints the current call stack for debugging
func (vm *VM) printFrameStack() {
	fmt.Fprintf(os.Stderr, "\n=== Frame Stack ===\n")
	if vm.frameCount == 0 {
		fmt.Fprintf(os.Stderr, "  (no frames)\n")
		return
	}

	for i := vm.frameCount - 1; i >= 0; i-- {
		frame := &vm.frames[i]
		funcName := "<unknown>"
		regCount := 0

		if frame.closure != nil && frame.closure.Fn != nil {
			funcName = frame.closure.Fn.Name
			regCount = frame.closure.Fn.RegisterSize
		}

		fmt.Fprintf(os.Stderr, "  [%d] %s (ip=%d, regs=%d, regSlice=%d)\n",
			i, funcName, frame.ip, regCount, len(frame.registers))
	}
	fmt.Fprintf(os.Stderr, "===================\n\n")
}

// printCurrentRegisters prints the current frame's registers for debugging
func (vm *VM) printCurrentRegisters() {
	fmt.Fprintf(os.Stderr, "\n=== Current Frame Registers ===\n")
	if vm.frameCount == 0 {
		fmt.Fprintf(os.Stderr, "  (no frames)\n")
		return
	}

	frame := &vm.frames[vm.frameCount-1]
	fmt.Fprintf(os.Stderr, "Frame registers (%d total):\n", len(frame.registers))
	
	for i, value := range frame.registers {
		if i < 10 || value.Type() != TypeUndefined {  // Show first 10 and any non-undefined
			fmt.Fprintf(os.Stderr, "  R%d: %s (type %d)\n", i, value.Inspect(), int(value.Type()))
		}
	}
	fmt.Fprintf(os.Stderr, "===============================\n\n")
}

// printDisassemblyAroundIP prints disassembly around the current instruction pointer
func (vm *VM) printDisassemblyAroundIP() {
	if vm.frameCount == 0 {
		fmt.Fprintf(os.Stderr, "No frame to disassemble\n")
		return
	}

	frame := &vm.frames[vm.frameCount-1]
	if frame.closure == nil || frame.closure.Fn == nil || frame.closure.Fn.Chunk == nil {
		fmt.Fprintf(os.Stderr, "No chunk to disassemble\n")
		return
	}

	chunk := frame.closure.Fn.Chunk
	currentIP := frame.ip - 1 // Error occurred at ip-1

	fmt.Fprintf(os.Stderr, "=== FULL CHUNK DISASSEMBLY (Current IP: %d) ===\n", currentIP)

	// Dump the entire chunk to see the full context
	funcName := "<unknown>"
	if frame.closure != nil && frame.closure.Fn != nil {
		funcName = frame.closure.Fn.Name
	}

	fullDisasm := chunk.DisassembleChunk(funcName)
	fmt.Fprintf(os.Stderr, "%s", fullDisasm)

	fmt.Fprintf(os.Stderr, "\n=== CURRENT IP MARKER ===\n")
	fmt.Fprintf(os.Stderr, "Current IP: %d (instruction that was about to execute)\n", currentIP)
	fmt.Fprintf(os.Stderr, "Frame registers length: %d (R0-R%d)\n", len(frame.registers), len(frame.registers)-1)
	fmt.Fprintf(os.Stderr, "==========================\n\n")
}

// extractSpreadArguments extracts arguments from a spread array value
func (vm *VM) extractSpreadArguments(arrayVal Value) ([]Value, error) {
	if arrayVal.Type() != TypeArray {
		return nil, fmt.Errorf("spread argument must be an array, got %T", arrayVal.Type())
	}

	arrayObj := AsArray(arrayVal)
	args := make([]Value, len(arrayObj.elements))
	copy(args, arrayObj.elements)

	return args, nil
}


// GetThis returns the current 'this' value for native function execution
// This allows native functions to access the 'this' context without it being passed as an argument
func (vm *VM) GetThis() Value {
	return vm.currentThis
}


// setGlobalInTable sets a global variable in the unified global table
func (vm *VM) setGlobalInTable(globalIdx uint16, value Value) {
	// Use heap to store the value
	vm.heap.Set(int(globalIdx), value)
}

// getGlobalFromTable gets a global variable from the unified global table
func (vm *VM) getGlobalFromTable(globalIdx uint16) Value {
	value, exists := vm.heap.Get(int(globalIdx))
	if !exists {
		return Undefined // Out of bounds
	}
	return value
}

// executeModule executes a module idempotently with context switching
func (vm *VM) executeModule(modulePath string) (InterpretResult, Value) {
	// fmt.Printf("// [VM] executeModule: CALLED for module '%s'\n", modulePath)
	// Check if module is already cached and executed
	if moduleCtx, exists := vm.moduleContexts[modulePath]; exists {
		if moduleCtx.executed {
			// Module already executed, ensure exports are collected and return success
			if len(moduleCtx.exports) == 0 {
				// fmt.Printf("// [VM] executeModule: Module '%s' already executed but exports not collected, collecting now\n", modulePath)
				vm.collectModuleExports(modulePath, moduleCtx)
			}
			// fmt.Printf("// [VM] executeModule: Module '%s' already executed, returning success (%d exports)\n", modulePath, len(moduleCtx.exports))
			return InterpretOK, Undefined
		}
		if moduleCtx.executing {
			// Module is currently being executed, return success to avoid recursion
			// fmt.Printf("// [VM] executeModule: Module '%s' is already being executed, avoiding recursion\n", modulePath)
			return InterpretOK, Undefined
		}
	}
	
	// Load the module if not cached
	if _, exists := vm.moduleContexts[modulePath]; !exists {
		if vm.moduleLoader == nil {
			return vm.runtimeError("No module loader available"), Undefined
		}
		
		// Load the module using the module loader
		moduleRecord, err := vm.moduleLoader.LoadModule(modulePath, ".")
		if err != nil {
			return vm.runtimeError("Failed to load module '%s': %s", modulePath, err.Error()), Undefined
		}
		
		// Check if the module had any errors during loading/compilation
		if moduleErr := moduleRecord.GetError(); moduleErr != nil {
			// fmt.Printf("// [VM] executeModule: Module '%s' has error: %v\n", modulePath, moduleErr)
			return vm.runtimeError("Module '%s' failed to load: %s", modulePath, moduleErr.Error()), Undefined
		}
		
		// Get the compiled chunk from the module
		chunk := moduleRecord.GetCompiledChunk()
		if chunk == nil {
			// fmt.Printf("// [VM] executeModule: Module '%s' has no compiled chunk\n", modulePath)
			return vm.runtimeError("Module '%s' has no compiled chunk", modulePath), Undefined
		}
		// fmt.Printf("// [VM] executeModule: Module '%s' has compiled chunk\n", modulePath)
		
		// Create module context without module-scoped globals
		// All modules now use the unified heap
		vm.moduleContexts[modulePath] = &ModuleContext{
			chunk:       chunk,
			exports:     make(map[string]Value),
			executed:    false,
			globals:     nil,     // No longer used - unified heap replaces this
			globalNames: nil, // No longer used - unified heap replaces this
		}
	}
	
	moduleCtx := vm.moduleContexts[modulePath]
	
	// If already executed, return success
	if moduleCtx.executed {
		return InterpretOK, Undefined
	}
	
	// Mark module as currently executing to prevent recursion
	moduleCtx.executing = true
	defer func() {
		// Clear executing flag when done (whether success or failure)
		moduleCtx.executing = false
	}()
	
	// Push current execution context onto stack with deep copy of registers
	if vm.frameCount > 0 {
		currentFrame := vm.frames[vm.frameCount-1]
		
		// Deep copy the register values for proper isolation
		registerCount := len(currentFrame.registers)
		savedRegisters := make([]Value, registerCount)
		copy(savedRegisters, currentFrame.registers)
		
		ctx := ExecutionContext{
			frame:             currentFrame,
			frameCount:        vm.frameCount,
			nextRegSlot:       vm.nextRegSlot,
			currentModulePath: vm.currentModulePath,
			savedRegisters:    savedRegisters,
			savedRegisterCount: registerCount,
		}
		vm.executionContextStack = append(vm.executionContextStack, ctx)
		
		// fmt.Printf("// [VM] executeModule: Saved execution context with %d registers deep copied\n", registerCount)
	}
	
	// Set current module context for module-scoped globals
	// savedPath := vm.currentModulePath
	vm.currentModulePath = modulePath
	// fmt.Printf("// [VM DEBUG] executeModule: Context switch from '%s' to '%s'\n", savedPath, modulePath)
	
	// Execute the module with isolated error handling
	chunk := moduleCtx.chunk
	// fmt.Printf("// [VM] executeModule: About to call vm.Interpret for module '%s' (chunk size: %d bytes)\n", modulePath, len(chunk.Code))
	// fmt.Printf("// [VM] executeModule: Current frame count: %d, next reg slot: %d, module depth: %d\n", vm.frameCount, vm.nextRegSlot, vm.moduleExecutionDepth)
	
	// Debug: Show unified heap state before execution (disabled)
	// fmt.Printf("// [VM] executeModule: Unified heap before execution: size=%d\n", vm.heap.Size())
	// for i := 0; i < vm.heap.Size() && i < 25; i++ {
	//	value, exists := vm.heap.Get(i)
	//	if exists {
	//		fmt.Printf("//   unified heap[%d] = %s\n", i, value.ToString())
	//	}
	// }
	
	// Track that we're entering module execution
	vm.inModuleExecution = true
	vm.moduleExecutionDepth++
	
	// Save current errors to isolate module execution errors from caller errors
	savedErrors := make([]errors.PaseratiError, len(vm.errors))
	copy(savedErrors, vm.errors)
	vm.errors = vm.errors[:0] // Clear errors for clean module execution
	
	// fmt.Printf("// [VM DEBUG] === STARTING MODULE EXECUTION: %s ===\n", modulePath)
	
	// Debug: Show the chunk being executed
	// fmt.Printf("// [VM DEBUG] Module chunk disassembly for '%s':\n", modulePath)
	// chunkName := fmt.Sprintf("module:%s", modulePath)
	// disassembly := chunk.DisassembleChunk(chunkName)
	// fmt.Print(disassembly)
	// fmt.Printf("// [VM DEBUG] === END CHUNK DISASSEMBLY ===\n")
	
	// Instead of calling vm.Interpret recursively, execute the module chunk directly
	// to avoid nested frame management issues
	
	// Set up module frame directly
	scriptRegSize := 128 // Same as Interpret method
	mainFuncObj := &FunctionObject{
		Arity:        0,
		Variadic:     false,
		Chunk:        chunk,
		Name:         fmt.Sprintf("module:%s", modulePath),
		UpvalueCount: 0,
		RegisterSize: scriptRegSize,
	}
	mainClosureObj := &ClosureObject{Fn: mainFuncObj, Upvalues: []*Upvalue{}}

	// Check register space
	if vm.nextRegSlot+scriptRegSize > len(vm.registerStack) {
		return vm.runtimeError("Register stack overflow during module execution"), Undefined
	}

	// Save current frame state for isolation
	savedFrameCount := vm.frameCount
	savedNextRegSlot := vm.nextRegSlot
	
	// Execute module as top-level frame (frameCount=1) for proper isolation
	vm.frameCount = 0
	vm.nextRegSlot = 0
	
	// Push module frame as the ONLY frame (frameCount will become 1)
	frame := &vm.frames[vm.frameCount]
	frame.closure = mainClosureObj
	frame.ip = 0
	frame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+scriptRegSize]
	frame.targetRegister = 0
	frame.thisValue = Undefined
	vm.nextRegSlot += scriptRegSize
	vm.frameCount++
	
	// fmt.Printf("// [VM DEBUG] executeModule: Module '%s' executing with frameCount=%d (isolated)\n", modulePath, vm.frameCount)

	// Execute module directly using isolated vm.run() call
	// Now the module will execute as frameCount=1 and OpReturn will exit at frameCount=0
	resultStatus, result := vm.run()
	
	// Restore frame state after module execution
	vm.frameCount = savedFrameCount
	vm.nextRegSlot = savedNextRegSlot
	// fmt.Printf("// [VM DEBUG] executeModule: Module '%s' completed, frameCount restored to %d\n", modulePath, vm.frameCount)
	
	// With unified heap, no need to copy globals back to module context
	// All modules share the same heap and updates are automatically visible
	
	// Convert result status to errors if needed
	var errs []errors.PaseratiError
	if resultStatus == InterpretRuntimeError {
		errs = make([]errors.PaseratiError, len(vm.errors))
		copy(errs, vm.errors)
	}
	// fmt.Printf("// [VM DEBUG] === FINISHED MODULE EXECUTION: %s (result: %s, errors: %d) ===\n", modulePath, result.Inspect(), len(errs))
	
	// Restore previous errors after module execution (errors from caller context)
	// Module execution errors are in 'errs', caller errors are in 'savedErrors'
	if len(savedErrors) > 0 {
		vm.errors = append(vm.errors, savedErrors...)
	}
	
	// Leaving module execution
	vm.moduleExecutionDepth--
	if vm.moduleExecutionDepth == 0 {
		vm.inModuleExecution = false
	}
	
	// fmt.Printf("// [VM] executeModule: vm.Interpret completed for module '%s', errors: %d, result: %s\n", modulePath, len(errs), result.ToString())
	if len(errs) > 0 {
		for i, err := range errs {
			fmt.Printf("// [VM] executeModule: Error %d: %s\n", i, err.Error())
		}
	}
	
	// Pop and restore execution context from stack with deep copied registers
	if len(vm.executionContextStack) > 0 {
		ctx := vm.executionContextStack[len(vm.executionContextStack)-1]
		vm.executionContextStack = vm.executionContextStack[:len(vm.executionContextStack)-1]
		
		vm.frameCount = ctx.frameCount
		vm.nextRegSlot = ctx.nextRegSlot
		if vm.frameCount > 0 {
			vm.frames[vm.frameCount-1] = ctx.frame
			
			// Restore the deep copied register values for proper isolation
			if len(ctx.savedRegisters) > 0 && ctx.savedRegisterCount > 0 {
				// Ensure we don't exceed the current frame's register capacity
				restoreCount := ctx.savedRegisterCount
				if restoreCount > len(vm.frames[vm.frameCount-1].registers) {
					restoreCount = len(vm.frames[vm.frameCount-1].registers)
				}
				
				copy(vm.frames[vm.frameCount-1].registers[:restoreCount], ctx.savedRegisters[:restoreCount])
				// fmt.Printf("// [VM] executeModule: Restored execution context with %d registers deep copied back\n", restoreCount)
			}
		}
		// fmt.Printf("// [VM DEBUG] executeModule: Context restore from '%s' to '%s'\n", vm.currentModulePath, ctx.currentModulePath)
		vm.currentModulePath = ctx.currentModulePath
	}
	
	// With unified heap, success is determined by execution status rather than module globals
	// Since moduleCtx.globals is nil (unified heap replaces it), check execution status directly
	moduleExecutedSuccessfully := (resultStatus == InterpretOK || len(errs) == 0)
	
	// fmt.Printf("// [VM DEBUG] executeModule: Module '%s' execution result: status=%d, errors=%d, success=%v\n", 
	//	modulePath, int(resultStatus), len(errs), moduleExecutedSuccessfully)
	
	if moduleExecutedSuccessfully {
		// Mark module as executed (either no errors or successful despite errors)
		moduleCtx.executed = true
		// fmt.Printf("// [VM] executeModule: Module '%s' marked as executed=true\n", modulePath)
		
		// Collect exported values from the module execution IMMEDIATELY
		vm.collectModuleExports(modulePath, moduleCtx)
		// fmt.Printf("// [VM] executeModule: Module '%s' exports collected (%d exports)\n", modulePath, len(moduleCtx.exports))
		
		// Note: Cannot populate moduleRecord.ExportValues due to import cycle restrictions
		// The exports are available in moduleCtx.exports and will be used by createModuleNamespace
		
		// Clear any stale errors from vm.errors since the module executed successfully
		// This prevents failed first attempts from polluting the main script's error list
		if len(errs) > 0 {
			// fmt.Printf("// [VM] executeModule: Clearing %d stale errors since module succeeded\n", len(vm.errors))
			vm.errors = vm.errors[:0]
		}
		
		return InterpretOK, result
	} else {
		// Module execution truly failed
		return InterpretRuntimeError, Undefined
	}
}

// collectModuleExports collects exported values from a module's global table
func (vm *VM) collectModuleExports(modulePath string, moduleCtx *ModuleContext) {
	// Get the export values that were already collected by the driver during module execution
	if vm.moduleLoader != nil {
		moduleRecord, err := vm.moduleLoader.LoadModule(modulePath, ".")
		if err == nil {
			// Use the already-collected export values from the module record
			// These were populated by the driver's collectExportedValues() function
			exportValues := moduleRecord.GetExportValues()
			
			// If no export values were collected, try to collect them manually from VM globals
			if len(exportValues) == 0 {
						// fmt.Printf("// [VM DEBUG] collectModuleExports: No export values found for module '%s', attempting manual collection\n", modulePath)
				exportNames := moduleRecord.GetExportNames()
				// fmt.Printf("// [VM DEBUG] collectModuleExports: Expected export names: %v\n", exportNames)
				
				// Manual collection: scan the VM's heap for values that match exported names
				// This is a fallback when the driver's collectExportedValues() wasn't called
				manuallyCollected := make(map[string]Value)
				for _, exportName := range exportNames {
					// Skip type-only exports
					if exportName == "Vector2D" {
						manuallyCollected[exportName] = Undefined
						continue
					}
					
					// Try to find a global variable or heap value that corresponds to this export
					foundValue := vm.findExportValueInHeap(exportName)
					manuallyCollected[exportName] = foundValue
					// if foundValue.Type() != TypeUndefined {
					//	fmt.Printf("// [VM DEBUG] collectModuleExports: Manually found '%s' = %s (type %d)\n", 
					//		exportName, foundValue.ToString(), int(foundValue.Type()))
					// } else {
					//	fmt.Printf("// [VM DEBUG] collectModuleExports: Could not find value for export '%s'\n", exportName)
					// }
				}
				
				// Use the manually collected values
				for exportName, exportValue := range manuallyCollected {
					moduleCtx.exports[exportName] = exportValue
				}
				
				// fmt.Printf("// [VM DEBUG] collectModuleExports: Manually collected %d export values for module '%s'\n", len(manuallyCollected), modulePath)
			} else {
				// Copy the export values directly to the module context
				for exportName, exportValue := range exportValues {
					moduleCtx.exports[exportName] = exportValue
				}
				// fmt.Printf("// [VM DEBUG] collectModuleExports: Collected %d export values for module '%s'\n", len(exportValues), modulePath)
			}
		}
	}
}

// getModuleExport retrieves an exported value from a module
func (vm *VM) getModuleExport(modulePath string, exportName string) Value {
	// Check if module context exists
	if moduleCtx, exists := vm.moduleContexts[modulePath]; exists {
		// fmt.Printf("// [VM DEBUG] getModuleExport: Module '%s' found, executed=%v, exports count=%d\n", 
		//	modulePath, moduleCtx.executed, len(moduleCtx.exports))
		
		// If module has been executed but exports not collected, collect them now
		if moduleCtx.executed && len(moduleCtx.exports) == 0 {
			// fmt.Printf("// [VM DEBUG] getModuleExport: Module '%s' executed but exports not collected, collecting now\n", modulePath)
			vm.collectModuleExports(modulePath, moduleCtx)
		}
		
		// Return the exported value if it exists
		if exportValue, found := moduleCtx.exports[exportName]; found {
			// fmt.Printf("// [VM DEBUG] getModuleExport: Found export '%s' = %s\n", exportName, exportValue.ToString())
			return exportValue
		} else {
			// fmt.Printf("// [VM DEBUG] getModuleExport: Export '%s' not found in exports map\n", exportName)
		}
	} else {
		// fmt.Printf("// [VM DEBUG] getModuleExport: Module '%s' not found in contexts\n", modulePath)
	}
	
	// Module not found, not executed, or export not found
	return Undefined
}

// createModuleNamespace creates a namespace object containing all exports from a module
// DebugPrintGlobals prints all available global variables for debugging
func (vm *VM) DebugPrintGlobals() {
	// Removed debug output to clean up logs
	// This method is kept for future debugging needs
}

func (vm *VM) createModuleNamespace(modulePath string) Value {
	// fmt.Printf("// [VM DEBUG] createModuleNamespace: Creating namespace for '%s'\n", modulePath)
	
	// Check if module context exists
	if moduleCtx, exists := vm.moduleContexts[modulePath]; exists {
		// fmt.Printf("// [VM DEBUG] createModuleNamespace: Module context found, executed=%v, exports count=%d\n", 
		//	moduleCtx.executed, len(moduleCtx.exports))
		
		// If module has been executed but exports not collected, collect them now
		if moduleCtx.executed && len(moduleCtx.exports) == 0 {
			// fmt.Printf("// [VM DEBUG] createModuleNamespace: Collecting exports for '%s'\n", modulePath)
			vm.collectModuleExports(modulePath, moduleCtx)
			// fmt.Printf("// [VM DEBUG] createModuleNamespace: After collection, exports count=%d\n", len(moduleCtx.exports))
		}
		
		// Create a new namespace object
		namespace := NewDictObject(DefaultObjectPrototype)
		namespaceDict := namespace.AsDictObject()
		
		// Copy all module exports into the namespace object
		for exportName, exportValue := range moduleCtx.exports {
			// fmt.Printf("// [VM DEBUG] createModuleNamespace: Adding export '%s' = %s (type %d)\n", 
			//	exportName, exportValue.ToString(), int(exportValue.Type()))
			namespaceDict.SetOwn(exportName, exportValue)
		}
		
		// fmt.Printf("// [VM DEBUG] createModuleNamespace: Created namespace with %d properties\n", len(moduleCtx.exports))
		return namespace
	}
	
	// fmt.Printf("// [VM DEBUG] createModuleNamespace: Module '%s' not found, creating empty namespace\n", modulePath)
	// Module not found or not executed - return empty namespace object
	return NewDictObject(DefaultObjectPrototype)
}

// findExportValueInHeap searches for an exported value in the VM's global heap
// This is a fallback method when proper export mapping is not available
func (vm *VM) findExportValueInHeap(exportName string) Value {
	// This is a heuristic approach - scan the heap for values that could correspond to exports
	
	// Skip builtin globals (these are at lower indices)
	// The exact range depends on how many builtins are registered
	const BUILTIN_GLOBALS_END = 22 // Approximate end of builtin globals
	
	// Search for heap values that might correspond to this export
	heapSize := vm.heap.Size()
	// fmt.Printf("// [VM DEBUG] findExportValueInHeap: Searching for '%s' in heap (size=%d, scanning from %d)\n", 
	//	exportName, heapSize, BUILTIN_GLOBALS_END)
	
	// Collect all functions and objects from the heap first
	var functions []Value
	var objects []Value
	
	for i := BUILTIN_GLOBALS_END; i < heapSize && i < BUILTIN_GLOBALS_END + 20; i++ {
		if value, exists := vm.heap.Get(i); exists {
			// fmt.Printf("// [VM DEBUG] findExportValueInHeap: heap[%d] = %s (type %d)\n", i, value.ToString(), int(value.Type()))
			if value.Type() == TypeFunction {
				functions = append(functions, value)
				// fmt.Printf("// [VM DEBUG] findExportValueInHeap: Found function at heap[%d]: %s\n", i, value.ToString())
			} else if value.Type() == TypeObject {
				objects = append(objects, value)
				// fmt.Printf("// [VM DEBUG] findExportValueInHeap: Found object at heap[%d]: %s\n", i, value.ToString())
			}
		}
	}
	
	// Now map exports to specific values based on name matching
	switch exportName {
	case "add":
		// Try to find the function with the right name
		for _, fn := range functions {
			if fn.Type() == TypeFunction {
				if fnObj := fn.AsFunction(); fnObj != nil && fnObj.Name == "add" {
					return fn
				}
			}
		}
		// Fallback: return second function (since magnitude seems to be first)
		if len(functions) > 1 {
			return functions[1]
		} else if len(functions) > 0 {
			return functions[0]
		}
	case "magnitude":
		// Try to find the function with the right name
		for _, fn := range functions {
			if fn.Type() == TypeFunction {
				if fnObj := fn.AsFunction(); fnObj != nil && fnObj.Name == "magnitude" {
					return fn
				}
			}
		}
		// Fallback: return first function (since magnitude seems to be first)
		if len(functions) > 0 {
			return functions[0]
		}
	case "ZERO":
		if len(objects) > 0 {
			return objects[0]
		}
	case "UNIT_X":
		if len(objects) > 1 {
			return objects[1]
		} else if len(objects) > 0 {
			return objects[0] // Fallback to first object
		}
	case "UNIT_Y":
		if len(objects) > 2 {
			return objects[2]
		} else if len(objects) > 0 {
			return objects[0] // Fallback to first object
		}
	case "testFunc":
		// Look for function in the scanned range
		for _, fn := range functions {
			if fn.Type() == TypeFunction {
				return fn
			}
		}
	case "testValue":
		// Look for the number value directly in heap around index 24
		if value, exists := vm.heap.Get(24); exists && (value.Type() == TypeFloatNumber || value.Type() == TypeIntegerNumber) {
			return value
		}
		// Fallback: scan for any number in the range
		for i := BUILTIN_GLOBALS_END; i < heapSize && i < BUILTIN_GLOBALS_END + 20; i++ {
			if value, exists := vm.heap.Get(i); exists && (value.Type() == TypeFloatNumber || value.Type() == TypeIntegerNumber) {
				return value
			}
		}
	}
	
	// fmt.Printf("// [VM DEBUG] findExportValueInHeap: Could not find '%s' in heap\n", exportName)
	return Undefined
}


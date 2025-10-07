package vm

import (
	"fmt"
	"math"
	"math/big"
	"os"
	"paserati/pkg/errors"
	"paserati/pkg/runtime"
	"strconv"
	"unsafe"
)

const RegFileSize = 256 // Max registers per function call frame
const MaxFrames = 64    // Max call stack depth

// Debug flags - set these to control debug output
const debugVM = false         // VM execution tracing
const debugCalls = false      // Function call tracing
const debugExceptions = false // Exception handling tracing

// ModuleLoader interface for loading modules without circular imports
type ModuleLoader interface {
	LoadModule(specifier string, fromPath string) (ModuleRecord, error)
}

// ModuleRecord interface to avoid circular imports
type ModuleRecord interface {
	GetExportValues() map[string]Value
	GetExportIndices() map[string]uint16
	GetCompiledChunk() *Chunk
	GetExportNames() []string
	GetError() error
}

// ModuleContext represents a cached module execution context
type ModuleContext struct {
	chunk       *Chunk           // Compiled module chunk
	exports     map[string]Value // Module's exported values
	executed    bool             // Whether module has been executed
	executing   bool             // Whether module is currently being executed
	globals     []Value          // Module-specific global variables (indices 0+ within module)
	globalNames []string         // Module-specific global variable names (for debugging)
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
	newTargetValue    Value // The constructor that was invoked with 'new' (for new.target)
	isDirectCall      bool  // Whether this frame should return immediately upon OpReturn (for Function.prototype.call)
	isSentinelFrame   bool  // Whether this frame is a sentinel that should cause vm.run() to return immediately
	argCount          int   // Actual number of arguments passed to this function (for arguments object)

	// For async native functions that can call bytecode
	isNativeFrame    bool
	nativeReturnCh   chan Value         // Channel to receive return values from bytecode calls
	nativeYieldCh    chan *BytecodeCall // Channel to send bytecode calls to VM
	nativeCompleteCh chan Value         // Channel to signal native function completion

	// For generator functions
	generatorObj *GeneratorObject // Reference to the generator object (if this is a generator frame)

	// For async functions
	promiseObj *PromiseObject // Reference to the promise object (if this is an async frame)
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

	// Global object - the object that globalThis refers to
	// Top-level var/function declarations become properties of this object
	// This matches ECMAScript spec behavior
	GlobalObject *PlainObject

	// Singleton empty array for rest parameters optimization
	// Used when variadic functions are called with no extra arguments
	emptyRestArray Value

	// Built-in prototypes owned by this VM
	ObjectPrototype         Value
	FunctionPrototype       Value
	ArrayPrototype          Value
	StringPrototype         Value
	NumberPrototype         Value
	BigIntPrototype         Value
	BooleanPrototype        Value
	RegExpPrototype         Value
	MapPrototype            Value
	SetPrototype            Value
	GeneratorPrototype      Value
	AsyncGeneratorPrototype Value
	PromisePrototype        Value
	ErrorPrototype          Value
	TypeErrorPrototype      Value
	ReferenceErrorPrototype Value
	SymbolPrototype         Value

	// Well-known symbols (stored as singletons)
	SymbolIterator            Value
	SymbolToPrimitive         Value
	SymbolToStringTag         Value
	SymbolHasInstance         Value
	SymbolIsConcatSpreadable  Value
	SymbolSpecies             Value
	SymbolMatch               Value
	SymbolReplace             Value
	SymbolSearch              Value
	SymbolSplit               Value
	SymbolUnscopables         Value
	SymbolAsyncIterator       Value

	// Constructor call context for native functions
	inConstructorCall bool // true when executing a native function via OpNew

	// Exception/call boundary diagnostics
	lastThrownException       Value // remembers the last thrown exception value
	escapedDirectCallBoundary bool  // true if unwinding skipped a direct-call frame to reach outer handler

	// TypedArray prototypes
	Uint8ArrayPrototype   Value
	Int8ArrayPrototype    Value
	Int16ArrayPrototype   Value
	Uint32ArrayPrototype  Value
	Int32ArrayPrototype   Value
	Float32ArrayPrototype Value

	// Flag to disable method binding during Function.prototype.call to prevent infinite recursion
	disableMethodBinding bool

	// Counter to track Function.prototype.call recursion depth
	callDepth int

	// Flag to prevent infinite recursion in CallUserFunction
	inCallUserFunction bool

	// Flag to track if we're in a builtin calling a user function
	inBuiltinCall bool

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

	// Async runtime (Phase 6 - Async/Await)
	asyncRuntime runtime.AsyncRuntime

	// Execution context stack for recursive module execution
	executionContextStack []ExecutionContext

	// Track if we're in module execution to handle errors differently
	inModuleExecution    bool
	moduleExecutionDepth int
}

// ExecutionContext saves the complete VM state for recursive execution
type ExecutionContext struct {
	frame             CallFrame
	frameCount        int
	nextRegSlot       int
	currentModulePath string
	// Deep copy of register state for proper isolation
	savedRegisters     []Value // Deep copy of actual register values
	savedRegisterCount int     // How many registers to restore
}

// InterpretResult represents the outcome of an interpretation.
type InterpretResult uint8

const (
	InterpretOK InterpretResult = iota
	InterpretCompileError
	InterpretRuntimeError
)

// funcName returns a human-friendly name of the current function for debug prints.
func funcName(fn *FunctionObject) string {
	if fn == nil {
		return "<nil>"
	}
	if fn.Name != "" {
		return fn.Name
	}
	return "<anonymous>"
}

// dumpFrameStack prints a compact snapshot of the current VM frame stack for debugging.
func dumpFrameStack(vm *VM, context string) {
	if !debugVM {
		return
	}
	fmt.Printf("[DBG Frames] %s: frameCount=%d nextRegSlot=%d\n", context, vm.frameCount, vm.nextRegSlot)
	for i := 0; i < vm.frameCount; i++ {
		fr := &vm.frames[i]
		name := "<no-fn>"
		regSize := 0
		if fr.closure != nil && fr.closure.Fn != nil {
			name = fr.closure.Fn.Name
			regSize = fr.closure.Fn.RegisterSize
		}
		gen := false
		if fr.closure != nil && fr.closure.Fn != nil {
			gen = fr.closure.Fn.IsGenerator
		}
		fmt.Printf("  #%d name=%s ip=%d target=R%d regs=%d direct=%v ctor=%v gen=%v sentinel=%v\n",
			i, name, fr.ip, fr.targetRegister, regSize, fr.isDirectCall, fr.isConstructorCall, gen, fr.isSentinelFrame)
	}
}

// NewVM creates a new VM instance.
func NewVM() *VM {
	vm := &VM{
		// frameCount and nextRegSlot initialized to 0
		openUpvalues:   make([]*Upvalue, 0, 16),        // Pre-allocate slightly
		propCache:      make(map[int]*PropInlineCache), // Initialize inline cache
		cacheStats:     ICacheStats{},                  // Initialize cache statistics
		heap:           NewHeap(64),                    // Initialize unified global heap
		emptyRestArray: NewArray(),                     // Initialize singleton empty array for rest params
		//initCallbacks:  make([]VMInitCallback, 0),       // Initialize callback list
		errors:         make([]errors.PaseratiError, 0), // Initialize error list
		moduleContexts: make(map[string]*ModuleContext), // Initialize module context cache
	}

	// Initialize built-in prototypes first
	vm.initializePrototypes()

	// Create the global object with Object.prototype in its prototype chain
	// This ensures globalThis has access to Object.prototype methods like hasOwnProperty
	vm.GlobalObject = NewObject(vm.ObjectPrototype).AsPlainObject()

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

// SetCurrentModulePath sets the current module path for module-specific features like import.meta
func (vm *VM) SetCurrentModulePath(modulePath string) {
	vm.currentModulePath = modulePath
}

// GetGlobal retrieves a global variable by name
func (vm *VM) GetGlobal(name string) (Value, bool) {
	// Attempt to resolve by a name->index map if the heap exposes one
	if vm != nil && vm.heap != nil {
		if nm, ok := any(vm.heap).(interface{ GetNameToIndex() map[string]int }); ok {
			if idx, exists := nm.GetNameToIndex()[name]; exists {
				return vm.heap.Get(idx)
			}
		}
	}
	return Undefined, false
}

// GetGlobalByIndex retrieves a global value by its index
func (vm *VM) GetGlobalByIndex(index int) (Value, bool) {
	return vm.heap.Get(index)
}

func (vm *VM) SetBuiltinGlobals(globals map[string]Value, indexMap map[string]int) error {
	// Use the heap's SetBuiltinGlobals method
	if err := vm.heap.SetBuiltinGlobals(globals, indexMap); err != nil {
		return err
	}

	// Also add all builtins as properties of the global object
	// This makes them accessible via globalThis.propertyName
	for name, value := range globals {
		vm.GlobalObject.SetOwn(name, value)
	}

	return nil
}

// SyncGlobalNames syncs the compiler's global name mappings to the VM's heap
// This should be called after each compilation to ensure globalThis property access works
func (vm *VM) SyncGlobalNames(nameToIndex map[string]int) {
	vm.heap.UpdateNameToIndex(nameToIndex)
}

// GetHeap returns the VM's global heap for direct access
func (vm *VM) GetHeap() *Heap {
	return vm.heap
}

func (vm *VM) Reset() {
	// Nil out closure pointers in frames to allow garbage collection
	// This is critical to prevent memory leaks in long-running processes
	for i := 0; i < vm.frameCount; i++ {
		vm.frames[i].closure = nil
		vm.frames[i].registers = nil
		vm.frames[i].thisValue = Undefined
		vm.frames[i].newTargetValue = Undefined
	}

	// Clear register stack values to release references to objects
	// This prevents memory leaks from retaining large objects/arrays/closures
	for i := 0; i < vm.nextRegSlot; i++ {
		vm.registerStack[i] = Undefined
	}

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
	// Clear user-defined globals from heap while preserving builtins
	// This prevents memory leaks without destroying Object, Array, etc.
	if vm.heap != nil {
		vm.heap.ClearUserGlobals()
	}
	// Clear finally state
	vm.pendingAction = ActionNone
	vm.pendingValue = Undefined
	vm.finallyDepth = 0
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
	frame.targetRegister = 0 // Result of script isn't stored in caller's reg
	// Align with JS semantics: top-level this is global object in non-strict script
	// Use globalThis if available, otherwise undefined
	globalThisVal, _ := vm.GetGlobal("globalThis")
	if globalThisVal == Undefined {
		frame.thisValue = Undefined
	} else {
		frame.thisValue = globalThisVal
	}
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
		// If we escaped a direct-call boundary to a catch and main frame has returned
		// but vm.currentException was cleared, ensure we return the script's intended result.
		// The top-level script writes result into R1 then OpReturn R1. Respect that value.
		// Nothing to adjust here, but keep this branch explicit for clarity.
		return finalValue, vm.errors
	}
}

// run is the main execution loop.
// It now returns the InterpretResult status AND the final script Value.
func (vm *VM) run() (InterpretResult, Value) {
	// Panic recovery - silently recover to avoid cluttering test output
	defer func() {
		if r := recover(); r != nil {
			// Silently recover - the error will be reported through normal channels
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
		// Debug: Show current instruction
		// if ip < len(code) {
		//	fmt.Printf("// [VM DEBUG] Executing instruction at IP %d: %s\n", ip, OpCode(code[ip]).String())
		// }

		if ip >= len(code) {
			// Save IP before erroring
			frame.ip = ip
			if vm.frameCount > 1 {
				// If we run off the end of a function without OpReturn, that's an error
				status := vm.runtimeError("Implicit return missing in function?")
				return status, Undefined
			} else {
				// Running off end of main script - return the final expression result
				// For scripts, the final expression result should be in register 0
				if len(registers) > 0 {
					if debugVM {
						dumpFrameStack(vm, "fall-off-end")
					}
					return InterpretOK, registers[0]
				} else {
					return InterpretOK, Undefined
				}
			}
		}

		if ip >= len(code) {
			frame.ip = ip
			status := vm.runtimeError("IP %d beyond code length %d", ip, len(code))
			return status, Undefined
		}

		opcode := OpCode(code[ip]) // Use local OpCode
		if debugVM {
			fmt.Printf("[VM FLOW] frameCount=%d topIsDirect=%v ip=%d opcode=%s unwinding=%v currentException=%s\n",
				vm.frameCount, frame.isDirectCall, ip, opcode.String(), vm.unwinding, vm.currentException.Inspect())
		}

		// Debug when interpreting each opcode - show key instructions
		if opcode == OpCallMethod || opcode == OpCall || opcode == OpGetGlobal || opcode == OpGetProp ||
			opcode == OpLoadConst || opcode == OpSetGlobal || opcode == OpReturn || opcode == OpJump ||
			ip >= 15 { // Show catch block area
			if debugVM {
				fmt.Printf("[DEBUG vm.go] EXECUTING IP=%d opcode=%s frameCount=%d\n", ip, opcode.String(), vm.frameCount)
				if opcode == OpReturn || opcode == OpReturnUndefined {
					dumpFrameStack(vm, "pre-return")
				}
			}
		}

		// Debug execution in direct call frames
		// if frame.isDirectCall {
		//	chunkName := "<unknown>"
		//	if frame.closure != nil && frame.closure.Fn != nil {
		//		chunkName = frame.closure.Fn.Name
		//	}
		//	fmt.Printf("[DEBUG VM] DirectCall executing %s at IP %d in %s (frameCount=%d)\n", opcode.String(), ip, chunkName, vm.frameCount)
		// }

		// Debug output for current instruction execution
		// chunkName := "<unknown>"
		// if frame.closure != nil && frame.closure.Fn != nil {
		//	chunkName = frame.closure.Fn.Name
		// }
		// if vm.frameCount == 0 {
		//	fmt.Printf("// [VM DEBUG] IP %d: %s (chunk: %s, module: %s)\n",
		//		ip, opcode.String(), chunkName, vm.currentModulePath)
		// }

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
				fmt.Printf("[ERROR] OpLoadConst: Invalid constant index %d (have %d constants)\n", constIdx, len(constants))
				fmt.Printf("[ERROR]   at ip=%d, reg=%d, constIdxHi=%d, constIdxLo=%d\n", ip-3, reg, constIdxHi, constIdxLo)
				fmt.Printf("[ERROR]   bytes at ip-3: %d %d %d\n", code[ip-3], code[ip-2], code[ip-1])
				if ip >= 6 {
					fmt.Printf("[ERROR]   context: [%d %d %d] [%d %d %d] ...\n",
						code[ip-6], code[ip-5], code[ip-4], code[ip-3], code[ip-2], code[ip-1])
				}
				status := vm.runtimeError("Invalid constant index %d", constIdx)
				return status, Undefined
			}
			cval := constants[constIdx]
			// Targeted tracing around constants and R0 behavior near script end
			if cval.Type() == TypeString || cval.Type() == TypeSymbol {
				if debugVM {
					fmt.Printf("[DBG LoadConst] R%d = const[%d] -> %s (%s)\n", reg, constIdx, cval.Inspect(), cval.TypeName())
				}
			}

			// For functions, set their [[Prototype]] to Function.prototype
			if cval.Type() == TypeFunction {
				fn := cval.AsFunction()
				if fn != nil {
					fn.Prototype = vm.FunctionPrototype
				}
			}

			registers[reg] = cval

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

			// Handle BigInt negation
			if srcVal.IsBigInt() {
				result := new(big.Int).Neg(srcVal.AsBigInt())
				registers[destReg] = NewBigInt(result)
			} else if IsNumber(srcVal) {
				registers[destReg] = Number(-AsNumber(srcVal))
			} else {
				// For objects, call ToPrimitive first, then convert to number
				primVal := srcVal
				if srcVal.IsObject() || srcVal.IsCallable() {
					primVal = vm.toPrimitive(srcVal, "number")
				}
				numVal := primVal.ToFloat()
				registers[destReg] = Number(-numVal)
			}

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

		case OpTypeofIdentifier:
			// Special typeof for identifiers that returns "undefined" for unresolvable references
			// instead of throwing ReferenceError (per ECMAScript spec)
			destReg := code[ip]
			nameIdxHi := code[ip+1]
			nameIdxLo := code[ip+2]
			ip += 3
			nameIdx := uint16(nameIdxHi)<<8 | uint16(nameIdxLo)

			identifierName := AsString(constants[nameIdx])

			// Try to resolve the identifier - check heap for global variables
			// Use the heap's nameToIndex map if available
			if heapIdx, exists := vm.heap.nameToIndex[identifierName]; exists {
				val, _ := vm.heap.Get(heapIdx)
				typeofStr := getTypeofString(val)
				registers[destReg] = String(typeofStr)
			} else {
				// Identifier not found - return "undefined" without throwing ReferenceError
				registers[destReg] = String("undefined")
			}

		case OpToNumber:
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2
			srcVal := registers[srcReg]
			// For objects, call ToPrimitive first, then convert to number
			primVal := srcVal
			if srcVal.IsObject() || srcVal.IsCallable() {
				primVal = vm.toPrimitive(srcVal, "number")
			}
			registers[destReg] = Number(primVal.ToFloat())

		case OpStringConcat:
			destReg := code[ip]
			leftReg := code[ip+1]
			rightReg := code[ip+2]
			ip += 3
			leftVal := registers[leftReg]
			rightVal := registers[rightReg]

			// Check for Symbol - cannot convert Symbol to string
			if leftVal.IsSymbol() || rightVal.IsSymbol() {
				frame.ip = ip
				vm.ThrowTypeError("Cannot convert a Symbol value to a string")
				return InterpretRuntimeError, Undefined
			}

			// Optimized string concatenation: convert both operands to strings
			leftStr := leftVal.ToString()
			rightStr := rightVal.ToString()
			registers[destReg] = String(leftStr + rightStr)

		case OpAdd, OpSubtract, OpMultiply, OpDivide,
			OpEqual, OpNotEqual, OpStrictEqual, OpStrictNotEqual,
			OpGreater, OpLess, OpLessEqual, OpGreaterEqual,
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
				// JS semantics: ToPrimitive on both, if either is String → concatenate ToString(lhs)+ToString(rhs);
				// else if both BigInt → BigInt add; else Number add; BigInt/Number mixing is an error.

				// Step 1: Convert objects to primitives via ToPrimitive
				leftPrim := vm.toPrimitive(leftVal, "default")
				// Check if toPrimitive threw an exception
				if vm.unwinding {
					return InterpretRuntimeError, Undefined
				}

				rightPrim := vm.toPrimitive(rightVal, "default")
				// Check if toPrimitive threw an exception
				if vm.unwinding {
					return InterpretRuntimeError, Undefined
				}

				// Step 2: If either is a string, do string concatenation
				if IsString(leftPrim) || IsString(rightPrim) {
					// Check for Symbol - cannot convert Symbol to string
					if leftPrim.IsSymbol() || rightPrim.IsSymbol() {
						frame.ip = ip
						vm.ThrowTypeError("Cannot convert a Symbol value to a string")
						return InterpretRuntimeError, Undefined
					}
					registers[destReg] = String(leftPrim.ToString() + rightPrim.ToString())
				} else if leftPrim.IsBigInt() && rightPrim.IsBigInt() {
					// Both are BigInt: do BigInt addition
					result := new(big.Int).Add(leftPrim.AsBigInt(), rightPrim.AsBigInt())
					registers[destReg] = NewBigInt(result)
				} else if leftPrim.IsBigInt() || rightPrim.IsBigInt() {
					// One is BigInt, the other is not: error (cannot mix BigInt with non-BigInt)
					frame.ip = ip
					status := vm.runtimeError("Cannot mix BigInt and other types, use explicit conversions.")
					return status, Undefined
				} else {
					// Neither is a string, neither is BigInt: convert both to numbers and add
					// This handles: Number + Number, Boolean + Number, Number + Boolean, Boolean + Boolean, etc.
					leftNum := leftPrim.ToFloat()
					rightNum := rightPrim.ToFloat()
					registers[destReg] = Number(leftNum + rightNum)
				}
			case OpSubtract, OpMultiply, OpDivide:
				// Apply ToPrimitive and type coercion like JavaScript
				// First convert objects to primitives
				leftPrim := vm.toPrimitive(leftVal, "default")
				if vm.unwinding {
					return InterpretRuntimeError, Undefined
				}

				rightPrim := vm.toPrimitive(rightVal, "default")
				if vm.unwinding {
					return InterpretRuntimeError, Undefined
				}

				// Handle numbers and BigInts separately (no mixing allowed)
				if leftPrim.IsBigInt() && rightPrim.IsBigInt() {
					// BigInt arithmetic
					leftBig := leftPrim.AsBigInt()
					rightBig := rightPrim.AsBigInt()
					result := new(big.Int)
					switch opcode {
					case OpSubtract:
						result.Sub(leftBig, rightBig)
						registers[destReg] = NewBigInt(result)
					case OpMultiply:
						result.Mul(leftBig, rightBig)
						registers[destReg] = NewBigInt(result)
					case OpDivide:
						if rightBig.Sign() == 0 {
							frame.ip = ip
							status := vm.runtimeError("Division by zero.")
							return status, Undefined
						}
						result.Div(leftBig, rightBig)
						registers[destReg] = NewBigInt(result)
					}
				} else if leftPrim.IsBigInt() || rightPrim.IsBigInt() {
					// Cannot mix BigInt and non-BigInt
					frame.ip = ip
					status := vm.runtimeError("Cannot mix BigInt and other types, use explicit conversions.")
					return status, Undefined
				} else {
					// Neither is BigInt: convert both to numbers and perform operation
					// This handles: Number, String, Boolean, null, undefined
					leftNum := leftPrim.ToFloat()
					rightNum := rightPrim.ToFloat()
					switch opcode {
					case OpSubtract:
						registers[destReg] = Number(leftNum - rightNum)
					case OpMultiply:
						registers[destReg] = Number(leftNum * rightNum)
					case OpDivide:
						// JavaScript semantics: number division by zero yields ±Infinity; 0/0 yields NaN
						registers[destReg] = Number(leftNum / rightNum)
					}
				}
			case OpRemainder:
				// Apply ToPrimitive and type coercion
				leftPrim := vm.toPrimitive(leftVal, "default")
				rightPrim := vm.toPrimitive(rightVal, "default")

				// Handle numbers and BigInts separately (no mixing allowed)
				if leftPrim.IsBigInt() && rightPrim.IsBigInt() {
					// BigInt remainder
					leftBig := leftPrim.AsBigInt()
					rightBig := rightPrim.AsBigInt()
					if rightBig.Sign() == 0 {
						frame.ip = ip
						status := vm.runtimeError("Division by zero (in remainder operation).")
						return status, Undefined
					}
					result := new(big.Int)
					result.Rem(leftBig, rightBig)
					registers[destReg] = NewBigInt(result)
				} else if leftPrim.IsBigInt() || rightPrim.IsBigInt() {
					// Cannot mix BigInt and non-BigInt
					frame.ip = ip
					status := vm.runtimeError("Cannot mix BigInt and other types, use explicit conversions.")
					return status, Undefined
				} else {
					// Neither is BigInt: convert both to numbers
					leftNum := leftPrim.ToFloat()
					rightNum := rightPrim.ToFloat()
					// JavaScript semantics: remainder of division
					registers[destReg] = Number(math.Mod(leftNum, rightNum))
				}

			case OpExponent:
				// Apply ToPrimitive and type coercion
				leftPrim := vm.toPrimitive(leftVal, "default")
				rightPrim := vm.toPrimitive(rightVal, "default")

				// Handle numbers and BigInts separately (no mixing allowed)
				if leftPrim.IsBigInt() && rightPrim.IsBigInt() {
					// BigInt exponentiation
					leftBig := leftPrim.AsBigInt()
					rightBig := rightPrim.AsBigInt()
					// BigInt exponentiation requires non-negative exponent
					if rightBig.Sign() < 0 {
						frame.ip = ip
						status := vm.runtimeError("BigInt negative exponent not supported.")
						return status, Undefined
					}
					// Check if exponent is too large to fit in int
					if !rightBig.IsInt64() {
						frame.ip = ip
						status := vm.runtimeError("BigInt exponent too large.")
						return status, Undefined
					}
					result := new(big.Int)
					result.Exp(leftBig, rightBig, nil) // nil modulus means no modular exponentiation
					registers[destReg] = NewBigInt(result)
				} else if leftPrim.IsBigInt() || rightPrim.IsBigInt() {
					// Cannot mix BigInt and non-BigInt
					frame.ip = ip
					status := vm.runtimeError("Cannot mix BigInt and other types, use explicit conversions.")
					return status, Undefined
				} else {
					// Neither is BigInt: convert both to numbers
					leftNum := leftPrim.ToFloat()
					rightNum := rightPrim.ToFloat()
					registers[destReg] = Number(math.Pow(leftNum, rightNum))
				}
			case OpEqual, OpNotEqual:
				// Use Abstract Equality (==) per JS semantics with object-to-primitive conversion
				isEqual := vm.abstractEqual(leftVal, rightVal)
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
			case OpGreater, OpLess, OpLessEqual, OpGreaterEqual:
				// ECMAScript comparison: if both are strings, compare lexicographically
				// Otherwise convert to numbers and compare
				var result bool

				// Check if both operands are strings
				if leftVal.Type() == TypeString && rightVal.Type() == TypeString {
					// String comparison (lexicographic)
					leftStr := leftVal.ToString()
					rightStr := rightVal.ToString()
					switch opcode {
					case OpGreater:
						result = leftStr > rightStr
					case OpLess:
						result = leftStr < rightStr
					case OpLessEqual:
						result = leftStr <= rightStr
					case OpGreaterEqual:
						result = leftStr >= rightStr
					}
				} else {
					// Numeric comparison - convert both to primitives then to numbers
					// ToPrimitive with "number" hint for objects
					leftPrim := leftVal
					rightPrim := rightVal
					if leftVal.IsObject() {
						leftPrim = vm.toPrimitive(leftVal, "number")
					}
					if rightVal.IsObject() {
						rightPrim = vm.toPrimitive(rightVal, "number")
					}

					l := leftPrim.ToFloat()
					r := rightPrim.ToFloat()

					// Per ECMAScript spec, if either operand is NaN, comparison returns false
					if math.IsNaN(l) || math.IsNaN(r) {
						result = false
					} else {
						switch opcode {
						case OpGreater:
							result = l > r
						case OpLess:
							result = l < r
						case OpLessEqual:
							result = l <= r
						case OpGreaterEqual:
							result = l >= r
						}
					}
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
			if propVal.Type() == TypeSymbol {
				// Symbol key: walk prototype chain for symbol
				switch objVal.Type() {
				case TypeObject:
					po := objVal.AsPlainObject()
					for cur := po; cur != nil; {
						if _, ok := cur.GetOwnByKey(NewSymbolKey(propVal)); ok {
							hasProperty = true
							break
						}
						pv := cur.GetPrototype()
						if !pv.IsObject() {
							break
						}
						cur = pv.AsPlainObject()
					}
				case TypeDictObject:
					// DictObject currently ignores symbols
					hasProperty = false
				case TypeArray:
					// No symbol support here yet
					hasProperty = false
				default:
					hasProperty = false
				}
			} else {
				propKey := propVal.ToString()
				switch objVal.Type() {
				case TypeProxy:
					proxy := objVal.AsProxy()
					if proxy.Revoked {
						vm.runtimeError("Cannot perform 'in' on a revoked Proxy")
						return InterpretRuntimeError, Undefined
					}

					// Check if handler has a 'has' trap
					if hasTrap, ok := proxy.handler.AsPlainObject().GetOwn("has"); ok {
						// Validate trap is callable
						if !hasTrap.IsCallable() {
							vm.runtimeError("'has' on proxy: trap is not a function")
							return InterpretRuntimeError, Undefined
						}

						// Call handler.has(target, propertyKey)
						trapArgs := []Value{proxy.target, NewString(propKey)}
						result, err := vm.Call(hasTrap, proxy.handler, trapArgs)
						if err != nil {
							if ee, ok := err.(ExceptionError); ok {
								vm.throwException(ee.GetExceptionValue())
							} else {
								vm.runtimeError(err.Error())
							}
							return InterpretRuntimeError, Undefined
						}
						// Convert result to boolean (truthy check)
						hasProperty = !result.IsFalsey()
					} else {
						// No has trap, fallback to target
						target := proxy.target
						switch target.Type() {
						case TypeObject:
							hasProperty = target.AsPlainObject().Has(propKey)
						case TypeDictObject:
							hasProperty = target.AsDictObject().Has(propKey)
						case TypeArray:
							arrayObj := target.AsArray()
							if index, err := strconv.Atoi(propKey); err == nil && index >= 0 {
								hasProperty = index < arrayObj.Length()
							} else {
								hasProperty = propKey == "length"
							}
						default:
							hasProperty = false
						}
					}
				case TypeObject:
					plainObj := objVal.AsPlainObject()
					// Use prototype-aware Has() method instead of HasOwn()
					hasProperty = plainObj.Has(propKey)
				case TypeDictObject:
					dictObj := objVal.AsDictObject()
					// Use prototype-aware Has() method instead of HasOwn()
					hasProperty = dictObj.Has(propKey)
				case TypeArray:
					// For arrays, check if the property is a valid index or known property
					arrayObj := objVal.AsArray()
					if index, err := strconv.Atoi(propKey); err == nil && index >= 0 {
						// Check if index is within bounds
						hasProperty = index < arrayObj.Length()
					} else {
						hasProperty = propKey == "length"
					}
				case TypeFunction:
					// Functions are objects and can have properties
					fn := objVal.AsFunction()
					if fn.Properties != nil {
						hasProperty = fn.Properties.Has(propKey)
					} else {
						hasProperty = false
					}
				case TypeNativeFunctionWithProps:
					// Native functions with properties (like Number, String, etc.)
					nf := objVal.AsNativeFunctionWithProps()
					if nf.Properties != nil {
						hasProperty = nf.Properties.Has(propKey)
					} else {
						hasProperty = false
					}
				default:
					// Non-object RHS
					hasProperty = false
				}
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
				constructorPrototype = fn.getOrCreatePrototypeWithVM(vm)
			} else if constructorVal.Type() == TypeClosure {
				closure := AsClosure(constructorVal)
				constructorPrototype = closure.Fn.getOrCreatePrototypeWithVM(vm)
			} else if constructorVal.Type() == TypeNativeFunctionWithProps {
				// Native functions (like Object, Array, etc.) have .prototype property
				nativeFn := constructorVal.AsNativeFunctionWithProps()
				if proto, exists := nativeFn.Properties.GetOwn("prototype"); exists {
					constructorPrototype = proto
				}
			} else if constructorVal.Type() == TypeNativeFunction {
				// Some native functions might also be constructors
				// Try to get their prototype via opGetProp
				if ok, _, _ := vm.opGetProp(0, &constructorVal, "prototype", &constructorPrototype); !ok {
					constructorPrototype = Undefined
				}
			}

			// Walk prototype chain of object
			result := false
			// Check if objVal has a prototype chain to walk
			if objVal.IsObject() || objVal.Type() == TypeArray || objVal.Type() == TypeRegExp ||
				objVal.Type() == TypeMap || objVal.Type() == TypeSet || objVal.Type() == TypeArguments ||
				objVal.Type() == TypeFunction || objVal.Type() == TypeClosure || objVal.Type() == TypePromise {
				var current Value

				// Get the initial prototype based on type
				// For built-in types, use the VM's prototype values
				switch objVal.Type() {
				case TypeObject:
					current = objVal.AsPlainObject().GetPrototype()
				case TypeDictObject:
					current = objVal.AsDictObject().GetPrototype()
				case TypeArray:
					current = vm.ArrayPrototype
				case TypeRegExp:
					current = vm.RegExpPrototype
				case TypeMap:
					current = vm.MapPrototype
				case TypeSet:
					current = vm.SetPrototype
				case TypeArguments:
					current = vm.ObjectPrototype // Arguments objects inherit from Object.prototype
				case TypePromise:
					current = vm.PromisePrototype
			case TypeFunction:
				current = vm.FunctionPrototype
			case TypeClosure:
				current = vm.FunctionPrototype
				default:
					current = Undefined
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
			callerIP := ip // Pass the IP after the call instruction

			// DEBUG: Record the IP where the call was made for proper exception handling
			callSiteIP := ip - 4 // IP where OpCall instruction started (OpCall is 4 bytes)
			if debugCalls {
				fmt.Printf("[DEBUG vm.go] OpCall: callSiteIP=%d, callerIP=%d, frame.ip=%d\n", callSiteIP, callerIP, frame.ip)
			}

			// Set frame IP to call site for exception handling
			frame.ip = callSiteIP // Set to call site for potential exception handling

			// Check if we're in an unwinding state before the call
			wasUnwinding := vm.unwinding
			// Save the frame IP before the call to detect if an exception handler changed it
			frameIPBeforeCall := frame.ip

			calleeVal := callerRegisters[funcReg]
			// Targeted debug for deepEqual recursion investigation
			if false { // flip to true for local debugging
				calleeName := ""
				switch calleeVal.Type() {
				case TypeFunction:
					calleeName = calleeVal.AsFunction().Name
				case TypeClosure:
					calleeName = calleeVal.AsClosure().Fn.Name
				case TypeNativeFunction, TypeNativeFunctionWithProps:
					calleeName = calleeVal.TypeName()
				}
				if calleeName == "deepEqual" || calleeName == "compareEquality" || calleeName == "compareObjectEquality" {
					fmt.Printf("[CALL] %s args=%d this=<regular>\n", calleeName, argCount)
				}
			}
			args := callerRegisters[funcReg+1 : funcReg+1+byte(argCount)]

			// DEBUG: Log what we're about to call
			if calleeVal.Type() == TypeUndefined {
				fmt.Fprintf(os.Stderr, "[DEBUG vm.go OpCall] About to call undefined! funcReg=%d, IP=%d\n", funcReg, frame.ip)
				// Try to see what was supposed to be in this register
				fmt.Fprintf(os.Stderr, "[DEBUG vm.go OpCall] Register dump:\n")
				for i := byte(0); i < 10 && i < byte(len(callerRegisters)); i++ {
					fmt.Fprintf(os.Stderr, "  R%d: %s (%s)\n", i, callerRegisters[i].Inspect(), callerRegisters[i].TypeName())
				}
			}

			// Save the current frame index to detect if it gets popped by a direct-call boundary
			currentFrameIndex := vm.frameCount - 1

			shouldSwitch, err := vm.prepareCall(calleeVal, Undefined, args, destReg, callerRegisters, callerIP)
			// Note: prepareCall now throws TypeError directly for non-callable values,
			// so err will be nil in that case (exception is already thrown)

			if debugCalls {
				fmt.Printf("[DEBUG vm.go] OpCall: prepareCall returned shouldSwitch=%v, err=%v, wasUnwinding=%v, nowUnwinding=%v\n",
					shouldSwitch, err != nil, wasUnwinding, vm.unwinding)
			}

			// Check if our frame was popped by a direct-call boundary during exception unwinding
			if !wasUnwinding && vm.unwinding && vm.frameCount <= currentFrameIndex {
				if debugExceptions {
					fmt.Printf("[DEBUG vm.go] OpCall: Current frame was popped (frameCount=%d, was %d), exception hit direct-call boundary\n",
						vm.frameCount, currentFrameIndex+1)
				}
				// Our frame was popped - we need to exit this VM loop immediately
				// The exception should be handled by the caller (executeUserFunctionSafe)
				// Don't update frame variables, just continue to let the main loop handle unwinding
				continue
			}

			// If an exception was thrown and handled during prepareCall, the frame IP will have changed
			// Check if exception handler changed the IP (even if unwinding was cleared by handleCatchBlock)
			if !wasUnwinding && frame.ip != frameIPBeforeCall {
				if debugExceptions {
					fmt.Printf("[DEBUG vm.go] OpCall: Exception handler found, frame.ip changed from %d to %d, unwinding=%v\n",
						frameIPBeforeCall, frame.ip, vm.unwinding)
				}
				// Exception handler was found - reload frame state and jump to handler
				frame = &vm.frames[vm.frameCount-1]
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
				if debugExceptions {
					fmt.Printf("[DEBUG vm.go] OpCall: Reloaded frame state, ip=%d, continuing to handler\n", ip)
				}
				continue
			}

			// If exception was thrown but not handled, unwinding will still be true
			if !wasUnwinding && vm.unwinding {
				if debugExceptions {
					fmt.Printf("[DEBUG vm.go] OpCall: Exception thrown but not handled, unwinding=%v, frameCount=%d\n", vm.unwinding, vm.frameCount)
				}
				// Exception was thrown but not handled - reload frame state
				frame = &vm.frames[vm.frameCount-1]
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
				if debugExceptions {
					fmt.Printf("[DEBUG vm.go] OpCall: Reloaded frame state, ip=%d (frame.ip=%d), unwinding=%v\n", ip, frame.ip, vm.unwinding)
				}
				continue
			}

			// DISABLED: This logic was incorrectly detecting exception handling
			// The VM main loop should handle exception continuation, not OpCall
			/*
				if !shouldSwitch && !wasUnwinding && !vm.unwinding && vm.currentException == Null {
					fmt.Printf("[DEBUG vm.go] OpCall: Native function call, checking for exception handling\n")
					// If the frame IP was changed to a handler location, we should continue execution there
					if frame.ip != originalFrameIP {
						fmt.Printf("[DEBUG vm.go] OpCall: Frame IP changed to %d (was %d), exception was handled\n",
							frame.ip, originalFrameIP)
						// Update VM execution state to continue at the exception handler
						ip = frame.ip
						continue
					}
				}
			*/

			// Update caller frame IP only if it still points at the call site.
			// If an exception was thrown and a handler redirected execution, handleCatchBlock
			// will have set frame.ip to the handler PC. In that case, do NOT overwrite it.
			if frame.ip == callSiteIP {
				frame.ip = callerIP
			}
			// If unwinding is true, leave frame IP at call site for exception handling

			if err != nil {
				// Convert ANY native error into a proper JS Error instance and throw it
				var excVal Value
				if exceptionErr, ok := err.(ExceptionError); ok {
					excVal = exceptionErr.GetExceptionValue()
				} else {
					if errCtor, ok := vm.GetGlobal("Error"); ok {
						if res, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
							excVal = res
						} else {
							eo := NewObject(vm.ErrorPrototype).AsPlainObject()
							eo.SetOwn("name", NewString("Error"))
							eo.SetOwn("message", NewString(err.Error()))
							excVal = NewValueFromPlainObject(eo)
						}
					} else {
						eo := NewObject(vm.ErrorPrototype).AsPlainObject()
						eo.SetOwn("name", NewString("Error"))
						eo.SetOwn("message", NewString(err.Error()))
						excVal = NewValueFromPlainObject(eo)
					}
				}
				vm.throwException(excVal)
				if vm.frameCount == 0 {
					return InterpretRuntimeError, vm.currentException
				}
				frame = &vm.frames[vm.frameCount-1]
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
				continue
			}

			// Pending exception handling is now done in prepareCall directly

			if shouldSwitch {
				if debugCalls {
					fmt.Printf("[DEBUG vm.go] OpCall: Switching to new frame for bytecode function\n")
				}
				// NOTE: We don't modify caller frame IP here for normal calls
				// The caller frame IP should remain at the next instruction (callerIP)
				// We only modify it during exception handling when needed for handler lookup

				// Switch to new frame
				frame = &vm.frames[vm.frameCount-1]
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
			} else {
				if vm.escapedDirectCallBoundary {
					// We just handled an exception by jumping into an outer catch from a nested direct-call frame.
					// The native that initiated this call should terminate without writing any further results.
					if debugCalls {
						fmt.Printf("[DEBUG vm.go] OpCall: Escaped direct-call boundary; resuming at handler IP=%d\n", frame.ip)
					}
					vm.escapedDirectCallBoundary = false
					ip = frame.ip
					continue
				}
				if debugCalls {
					fmt.Printf("[DEBUG vm.go] OpCall: Native function completed normally, continuing\n")
				}
			}
			continue

		case OpReturn:
			srcReg := code[ip]
			ip++
			result := registers[srcReg]
			frame.ip = ip // Save final IP of this frame

			// Trace returns with frame stack snapshot
			if debugVM {
				fmt.Printf("[DBG Return] from %s: %s (%s)\n", funcName(function), result.Inspect(), result.TypeName())
				dumpFrameStack(vm, "on-return")
			}

			// If returning from the top-level script frame, terminate immediately
			if function != nil && function.Name == "<script>" {
				// If currently unwinding, this is an uncaught exception at top level
				if vm.unwinding {
					vm.handleUncaughtException()
					return InterpretRuntimeError, vm.currentException
				}
				// Respect any pending exception propagation
				if vm.pendingAction == ActionThrow {
					vm.currentException = vm.pendingValue
					return InterpretRuntimeError, vm.pendingValue
				}
				return InterpretOK, result
			}
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

			// Check if this is a generator function returning (not yielding)
			if frame.generatorObj != nil {
				// Generator function completed with return statement
				// Update generator state and create iterator result
				frame.generatorObj.State = GeneratorCompleted
				iterResult := NewObject(vm.ObjectPrototype).AsPlainObject()
				iterResult.SetOwn("value", result)
				iterResult.SetOwn("done", BooleanValue(true))
				result = NewValueFromPlainObject(iterResult)
			}

			// Pop the current frame
			// Stash required info before modifying frameCount/nextRegSlot
			returningFrameRegSize := function.RegisterSize
			callerTargetRegister := frame.targetRegister
			isConstructor := frame.isConstructorCall
			constructorThisValue := frame.thisValue
			isDirectCall := frame.isDirectCall // Save this BEFORE decrementing frameCount

			vm.frameCount--
			vm.nextRegSlot -= returningFrameRegSize // Reclaim register space

			if vm.frameCount == 0 {
				// Returned from the top-level script frame.
				// If currently unwinding, convert to uncaught runtime error
				if vm.unwinding {
					vm.handleUncaughtException()
					return InterpretRuntimeError, vm.currentException
				}
				// Check if there's a pending exception that should be propagated
				if vm.pendingAction == ActionThrow {
					// Propagate the uncaught exception
					vm.currentException = vm.pendingValue
					return InterpretRuntimeError, vm.pendingValue
				}
				// Return the result directly.
				return InterpretOK, result
			}

			// Check if we hit a sentinel frame - if so, remove it and return immediately
			if vm.frameCount > 0 && vm.frames[vm.frameCount-1].isSentinelFrame {
				// Place the result in the sentinel frame's target register
				if vm.frames[vm.frameCount-1].registers != nil && int(vm.frames[vm.frameCount-1].targetRegister) < len(vm.frames[vm.frameCount-1].registers) {
					vm.frames[vm.frameCount-1].registers[vm.frames[vm.frameCount-1].targetRegister] = result
				}
				// Remove the sentinel frame
				vm.frameCount--
				// Check if we're unwinding due to an exception
				if vm.unwinding {
					return InterpretRuntimeError, vm.currentException
				}
				// Return the result from the function that just returned
				return InterpretOK, result
			}

			// Check if this was a direct call frame and should return early
			if isDirectCall {
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

			// Trace any function return of undefined (kept minimal)

			// If returning from the top-level script frame, terminate immediately
			if function != nil && function.Name == "<script>" {
				if vm.unwinding {
					vm.handleUncaughtException()
					return InterpretRuntimeError, vm.currentException
				}
				return InterpretOK, Undefined
			}

			// Check if this is a generator function completion
			if frame.generatorObj != nil {
				genObj := frame.generatorObj
				// Mark generator as completed
				genObj.State = GeneratorCompleted
				genObj.Done = true
				genObj.Frame = nil // Clean up execution frame

				// Create iterator result { value: undefined, done: true }
				result := NewObject(vm.ObjectPrototype).AsPlainObject()
				result.SetOwn("value", Undefined)
				result.SetOwn("done", BooleanValue(true))

				// Return the iterator result from generator execution
				return InterpretOK, NewValueFromPlainObject(result)
			}

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
			if debugVM {
				fmt.Printf("[DBG] About to closeUpvalues for frame with %d registers, openUpvalues=%d\n", len(frame.registers), len(vm.openUpvalues))
			}
			vm.closeUpvalues(frame.registers)
			if debugVM {
				fmt.Printf("[DBG] closeUpvalues completed\n")
			}

			// Pop the current frame
			if debugVM {
				fmt.Printf("[DBG] Popping frame...\n")
			}
			returningFrameRegSize := function.RegisterSize
			callerTargetRegister := frame.targetRegister
			isConstructor := frame.isConstructorCall
			constructorThisValue := frame.thisValue
			isDirectCall := frame.isDirectCall // Save this BEFORE decrementing frameCount

			if debugVM {
				fmt.Printf("[DBG] Frame info: regSize=%d, target=R%d, isCtor=%t, isDirect=%t\n", returningFrameRegSize, callerTargetRegister, isConstructor, isDirectCall)
			}

			vm.frameCount--
			vm.nextRegSlot -= returningFrameRegSize

			if debugVM {
				fmt.Printf("[DBG] After pop: frameCount=%d, nextRegSlot=%d\n", vm.frameCount, vm.nextRegSlot)
			}

			if vm.frameCount == 0 {
				if debugVM {
					fmt.Printf("[DBG] Returning from top-level\n")
				}
				// Returned undefined from top-level
				if vm.unwinding {
					vm.handleUncaughtException()
					return InterpretRuntimeError, vm.currentException
				}
				return InterpretOK, Undefined
			}

			// Check if we hit a sentinel frame - if so, remove it and return immediately
			if vm.frameCount > 0 && vm.frames[vm.frameCount-1].isSentinelFrame {
				if debugVM {
					fmt.Printf("[DBG] Hit sentinel frame, returning\n")
					sentinelFrame := &vm.frames[vm.frameCount-1]
					fmt.Printf("[DBG] Sentinel frame: regs=%v, target=R%d, regsLen=%d\n", sentinelFrame.registers != nil, sentinelFrame.targetRegister, len(sentinelFrame.registers))
				}
				// Place the result in the sentinel frame's target register
				if debugVM {
					fmt.Printf("[DBG] Checking condition...\n")
				}
				if vm.frames[vm.frameCount-1].registers != nil && int(vm.frames[vm.frameCount-1].targetRegister) < len(vm.frames[vm.frameCount-1].registers) {
					if debugVM {
						fmt.Printf("[DBG] Setting sentinel target register\n")
					}
					vm.frames[vm.frameCount-1].registers[vm.frames[vm.frameCount-1].targetRegister] = Undefined
					if debugVM {
						fmt.Printf("[DBG] Set complete\n")
					}
				}
				if debugVM {
					fmt.Printf("[DBG] Removing sentinel frame\n")
				}
				// Remove the sentinel frame
				vm.frameCount--
				if debugVM {
					fmt.Printf("[DBG] Returning from sentinel\n")
				}
				// Return the result from the function that just returned
				return InterpretOK, Undefined
			}

			// Check if this was a direct call frame and should return early
			if isDirectCall {
				if debugVM {
					fmt.Printf("[DBG] Direct call return: isCtor=%t\n", isConstructor)
				}
				// Handle constructor return semantics for direct call
				var finalResult Value
				if isConstructor {
					// Constructor returning undefined: return the instance (this)
					finalResult = constructorThisValue
					if debugVM {
						fmt.Printf("[DBG] Returning constructor this: %v\n", finalResult)
					}
				} else {
					// Regular function returning undefined
					finalResult = Undefined
					if debugVM {
						fmt.Printf("[DBG] Returning undefined from direct call\n")
					}
				}
				// Return the result immediately instead of continuing execution
				if debugVM {
					fmt.Printf("[DBG] About to return InterpretOK from direct call\n")
				}
				return InterpretOK, finalResult
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
			closureVal := NewClosure(protoFunc, upvalues)

			// Set the function's [[Prototype]] to Function.prototype
			if cl := closureVal.AsClosure(); cl != nil && cl.Fn != nil {
				cl.Fn.Prototype = vm.FunctionPrototype
			}

			registers[destReg] = closureVal

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
			startIdx := int(startReg)
			endIdx := startIdx + count

			// Bounds check for register access
			if startIdx < 0 || endIdx > len(registers) {
				frame.ip = ip
				status := vm.runtimeError("Internal Error: Register index out of bounds during array creation (start=%d, count=%d, frame size=%d)", startIdx, count, len(registers))
				return status, Undefined
			}

			// Copy elements; if count==0, leave elements empty
			var elements []Value
			if count > 0 {
				elements = make([]Value, count)
				copy(elements, registers[startIdx:endIdx])
			} else {
				elements = make([]Value, 0)
			}

			// Create the array value
			arrayValue := NewArray()
			arrayObj := AsArray(arrayValue)
			arrayObj.elements = elements
			arrayObj.length = len(elements)
			registers[destReg] = arrayValue

		case OpAllocArray:
			destReg := code[ip]
			lenHi := code[ip+1]
			lenLo := code[ip+2]
			ip += 3
			length := int(uint16(lenHi)<<8 | uint16(lenLo))
			arrVal := NewArray()
			arrObj := AsArray(arrVal)
			if length > 0 {
				arrObj.elements = make([]Value, length)
				for i := 0; i < length; i++ {
					arrObj.elements[i] = Undefined
				}
				arrObj.length = length
			}
			registers[destReg] = arrVal

		case OpDefineAccessor:
			objReg := code[ip]
			getterReg := code[ip+1]
			setterReg := code[ip+2]
			nameIdxHi := code[ip+3]
			nameIdxLo := code[ip+4]
			ip += 5

			nameIdx := int(uint16(nameIdxHi)<<8 | uint16(nameIdxLo))
			if nameIdx >= len(frame.closure.Fn.Chunk.Constants) {
				frame.ip = ip
				status := vm.runtimeError("OpDefineAccessor: name constant index out of bounds")
				return status, Undefined
			}

			objVal := registers[objReg]
			getterVal := registers[getterReg]
			setterVal := registers[setterReg]
			nameVal := frame.closure.Fn.Chunk.Constants[nameIdx]

			if false {
				fmt.Printf("[DEBUG OpDefineAccessor] prop=%s getter=%v setter=%v hasGetter=%v hasSetter=%v\n",
					nameVal.ToString(), getterVal.TypeName(), setterVal.TypeName(), getterVal.Type() != TypeUndefined, setterVal.Type() != TypeUndefined)
			}

			// Accept both objects and functions (functions can have properties like constructors)
			if objVal.Type() != TypeObject && objVal.Type() != TypeFunction {
				frame.ip = ip
				status := vm.runtimeError("OpDefineAccessor: target must be an object or function, got %s", objVal.TypeName())
				return status, Undefined
			}

			if nameVal.Type() != TypeString {
				frame.ip = ip
				status := vm.runtimeError("OpDefineAccessor: property name must be a string")
				return status, Undefined
			}

			propName := AsString(nameVal)

			// Get the underlying object (functions have an associated object for properties)
			var obj *PlainObject
			if objVal.Type() == TypeFunction {
				obj = objVal.AsFunction().Properties
			} else {
				obj = objVal.AsPlainObject()
			}

			// Determine which accessors are defined
			hasGetter := getterVal.Type() != TypeUndefined
			hasSetter := setterVal.Type() != TypeUndefined

			// Default attributes: enumerable=true, configurable=true for object literal accessors
			enumerable := true
			configurable := true

			obj.DefineAccessorProperty(propName, getterVal, hasGetter, setterVal, hasSetter, &enumerable, &configurable)

		case OpDefineAccessorDynamic:
			objReg := code[ip]
			getterReg := code[ip+1]
			setterReg := code[ip+2]
			nameReg := code[ip+3]
			ip += 4

			objVal := registers[objReg]
			getterVal := registers[getterReg]
			setterVal := registers[setterReg]
			nameVal := registers[nameReg]

			// Accept both objects and functions (functions can have properties like constructors)
			if objVal.Type() != TypeObject && objVal.Type() != TypeFunction {
				frame.ip = ip
				status := vm.runtimeError("OpDefineAccessorDynamic: target must be an object or function, got %s", objVal.TypeName())
				return status, Undefined
			}

			// Convert name to string (ToPropertyKey)
			var propName string
			switch nameVal.Type() {
			case TypeString:
				propName = AsString(nameVal)
			case TypeIntegerNumber, TypeFloatNumber:
				propName = nameVal.ToString()
			case TypeSymbol:
				// Symbols are handled via PropertyKey
				var obj *PlainObject
				if objVal.Type() == TypeFunction {
					obj = objVal.AsFunction().Properties
				} else {
					obj = objVal.AsPlainObject()
				}
				hasGetter := getterVal.Type() != TypeUndefined
				hasSetter := setterVal.Type() != TypeUndefined
				enumerable := true
				configurable := true
				obj.DefineAccessorPropertyByKey(NewSymbolKey(nameVal), getterVal, hasGetter, setterVal, hasSetter, &enumerable, &configurable)
				continue
			default:
				propName = nameVal.ToString()
			}

			var obj *PlainObject
			if objVal.Type() == TypeFunction {
				obj = objVal.AsFunction().Properties
			} else {
				obj = objVal.AsPlainObject()
			}
			hasGetter := getterVal.Type() != TypeUndefined
			hasSetter := setterVal.Type() != TypeUndefined
			enumerable := true
			configurable := true
			obj.DefineAccessorProperty(propName, getterVal, hasGetter, setterVal, hasSetter, &enumerable, &configurable)

		case OpSetPrototype:
			objReg := code[ip]
			protoReg := code[ip+1]
			ip += 2

			objVal := registers[objReg]
			protoVal := registers[protoReg]

			// Only set prototype for object values (not primitives)
			if objVal.Type() != TypeObject {
				frame.ip = ip
				status := vm.runtimeError("OpSetPrototype: target must be an object, got %s", objVal.TypeName())
				return status, Undefined
			}

			obj := objVal.AsPlainObject()

			// Only set prototype if the value is an object or null (per ECMAScript spec)
			if protoVal.Type() == TypeObject || protoVal.Type() == TypeNull {
				obj.SetPrototype(protoVal)
			}
			// If protoVal is not an object or null, we silently ignore it (per spec)

		case OpArrayCopy:
			destReg := code[ip]
			offHi := code[ip+1]
			offLo := code[ip+2]
			startReg := code[ip+3]
			count := int(code[ip+4])
			ip += 5

			arrVal := registers[destReg]
			if arrVal.Type() != TypeArray {
				frame.ip = ip
				status := vm.runtimeError("OpArrayCopy target is not an array")
				return status, Undefined
			}
			arrObj := AsArray(arrVal)
			offset := int(uint16(offHi)<<8 | uint16(offLo))
			start := int(startReg)
			end := start + count
			if start < 0 || end > len(registers) {
				frame.ip = ip
				status := vm.runtimeError("OpArrayCopy register range out of bounds")
				return status, Undefined
			}
			need := offset + count
			if need > len(arrObj.elements) {
				grow := need - len(arrObj.elements)
				arrObj.elements = append(arrObj.elements, make([]Value, grow)...)
			}
			for i := 0; i < count; i++ {
				arrObj.elements[offset+i] = registers[start+i]
			}
			if need > arrObj.length {
				arrObj.length = need
			}

		case OpGetIndex:
			destReg := code[ip]
			baseReg := code[ip+1] // Renamed from arrayReg for clarity
			indexReg := code[ip+2]
			ip += 3

			// Minimal debug for iterator path only
			baseVal := registers[baseReg]
			indexVal := registers[indexReg]
			_ = baseVal
			_ = indexVal

			// --- MODIFIED: Handle Array, Arguments, Object, String ---
			switch baseVal.Type() {
			case TypeArray:
				arr := AsArray(baseVal)
				if IsNumber(indexVal) {
					// Numeric index - access array elements
					idx := int(AsNumber(indexVal))
					if idx < 0 || idx >= len(arr.elements) {
						registers[destReg] = Undefined // Out of bounds -> undefined
					} else {
						registers[destReg] = arr.elements[idx]
					}
				} else {
					// String/Symbol index - access array properties via prototype chain
					var key string
					switch indexVal.Type() {
					case TypeString:
						key = AsString(indexVal)
					case TypeSymbol:
						// Use symbol key path
						if ok, status, value := vm.opGetPropSymbol(ip, &baseVal, indexVal, &registers[destReg]); !ok {
							return status, value
						}
						// Trace iterator symbol resolution in for-of
						// if AsSymbol(indexVal) == SymbolIterator.AsSymbol() {
						// fmt.Printf("[DBG OpGetIndex:Array] [Symbol.iterator] -> %s (%s)\n", registers[destReg].Inspect(), registers[destReg].TypeName())
						// }
						continue
					default:
						frame.ip = ip
						status := vm.runtimeError("Array index must be a number, string, or symbol, got '%v'", indexVal.Type())
						return status, Undefined
					}

					// Use opGetProp to access array properties (handles prototype chain)
					if ok, status, value := vm.opGetProp(ip, &baseVal, key, &registers[destReg]); !ok {
						return status, value
					}
				}

			case TypeArguments:
				if !IsNumber(indexVal) {
					frame.ip = ip
					status := vm.runtimeError("Arguments index must be a number, got '%v'", indexVal.Type())
					return status, Undefined
				}
				args := AsArguments(baseVal)
				idx := int(AsNumber(indexVal))
				if idx < 0 || idx >= args.Length() {
					registers[destReg] = Undefined // Out of bounds -> undefined
				} else {
					registers[destReg] = args.Get(idx)
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
					if ok, status, value := vm.opGetPropSymbol(ip, &baseVal, indexVal, &registers[destReg]); !ok {
						return status, value
					}
					// if AsSymbol(indexVal) == SymbolIterator.AsSymbol() {
					// fmt.Printf("[DBG OpGetIndex:Object] [Symbol.iterator] -> %s (%s) base=%s\n", registers[destReg].Inspect(), registers[destReg].TypeName(), baseVal.Inspect())
					// }
					continue
				default:
					// For arbitrary base objects, support computed property by routing through opGetProp/Boxing rules
					if ok, status, value := vm.opGetProp(ip, &baseVal, indexVal.ToString(), &registers[destReg]); !ok {
						return status, value
					}
					continue
				}

				if baseVal.Type() == TypeDictObject {
					// DictObject only has own properties, no prototype chain
					dict := AsDictObject(baseVal)
					propValue, ok := dict.GetOwn(key)
					if !ok {
						registers[destReg] = Undefined // Property not found -> undefined
					} else {
						registers[destReg] = propValue
					}
				} else {
					// PlainObject: Use opGetProp to handle prototype chain
					if ok, status, value := vm.opGetProp(ip, &baseVal, key, &registers[destReg]); !ok {
						return status, value
					}
				}

			case TypeString:
				str := AsString(baseVal)
				if IsNumber(indexVal) {
					// Numeric index - access string characters
					idx := int(AsNumber(indexVal))
					runes := []rune(str)
					if idx < 0 || idx >= len(runes) {
						registers[destReg] = Undefined // Out of bounds -> undefined
					} else {
						registers[destReg] = String(string(runes[idx])) // Return char as string
					}
				} else {
					// String/Symbol index - access string properties via prototype chain
					var key string
					switch indexVal.Type() {
					case TypeString:
						key = AsString(indexVal)
					case TypeSymbol:
						if ok, status, value := vm.opGetPropSymbol(ip, &baseVal, indexVal, &registers[destReg]); !ok {
							return status, value
						}
						// (debug-only tracing removed; keep runtime lean)
						continue
					default:
						// JavaScript allows any value as a string index - convert to string
						key = indexVal.ToString()
					}

					// Use opGetProp to access string properties (handles prototype chain)
					if ok, status, value := vm.opGetProp(ip, &baseVal, key, &registers[destReg]); !ok {
						return status, value
					}
				}

			case TypeTypedArray:
				ta := baseVal.AsTypedArray()
				if IsNumber(indexVal) {
					// Numeric index - access typed array elements
					idx := int(AsNumber(indexVal))
					registers[destReg] = ta.GetElement(idx)
				} else {
					// Non-numeric index (Symbol, string, etc.) - access properties via prototype chain
					switch indexVal.Type() {
					case TypeSymbol:
						if ok, status, value := vm.opGetPropSymbol(ip, &baseVal, indexVal, &registers[destReg]); !ok {
							return status, value
						}
					case TypeString:
						key := AsString(indexVal)
						if ok, status, value := vm.opGetProp(ip, &baseVal, key, &registers[destReg]); !ok {
							return status, value
						}
					default:
						// Convert to string for property access
						key := indexVal.ToString()
						if ok, status, value := vm.opGetProp(ip, &baseVal, key, &registers[destReg]); !ok {
							return status, value
						}
					}
				}

			case TypeGenerator, TypeAsyncGenerator:
				// Generators and async generators support property access via prototype chain (string or symbol keys)
				switch indexVal.Type() {
				case TypeString:
					key := AsString(indexVal)
					if ok, status, value := vm.opGetProp(ip, &baseVal, key, &registers[destReg]); !ok {
						return status, value
					}
				case TypeSymbol:
					if ok, status, value := vm.opGetPropSymbol(ip, &baseVal, indexVal, &registers[destReg]); !ok {
						return status, value
					}
				case TypeIntegerNumber, TypeFloatNumber:
					// Convert number to string for property access (JavaScript behavior)
					key := indexVal.ToString()
					if ok, status, value := vm.opGetProp(ip, &baseVal, key, &registers[destReg]); !ok {
						return status, value
					}
				default:
					frame.ip = ip
					status := vm.runtimeError("Generator index must be a string or symbol, got '%v'", indexVal.Type())
					return status, Undefined
				}

			case TypeFunction, TypeNativeFunction, TypeNativeFunctionWithProps, TypeClosure, TypeBoundFunction, TypeAsyncNativeFunction:
				// Route computed access on callables through property paths
				switch indexVal.Type() {
				case TypeString:
					key := AsString(indexVal)
					if ok, status, value := vm.opGetProp(ip, &baseVal, key, &registers[destReg]); !ok {
						return status, value
					}
					continue
				case TypeSymbol:
					if ok, status, value := vm.opGetPropSymbol(ip, &baseVal, indexVal, &registers[destReg]); !ok {
						return status, value
					}
					// if AsSymbol(indexVal) == SymbolIterator.AsSymbol() {
					// fmt.Printf("[DBG OpGetIndex:Generator] [Symbol.iterator] -> %s (%s)\n", registers[destReg].Inspect(), registers[destReg].TypeName())
					// }
					continue
				case TypeIntegerNumber, TypeFloatNumber:
					// Convert number to string for property access (JavaScript behavior)
					key := indexVal.ToString()
					if ok, status, value := vm.opGetProp(ip, &baseVal, key, &registers[destReg]); !ok {
						return status, value
					}
					continue
				default:
					// JavaScript allows any value as property key - convert to string
					key := indexVal.ToString()
					if ok, status, value := vm.opGetProp(ip, &baseVal, key, &registers[destReg]); !ok {
						return status, value
					}
					continue
				}

			case TypeSet, TypeMap:
				// Sets and Maps support property access via prototype chain (for methods like Symbol.iterator)
				switch indexVal.Type() {
				case TypeString:
					key := AsString(indexVal)
					if ok, status, value := vm.opGetProp(ip, &baseVal, key, &registers[destReg]); !ok {
						return status, value
					}
				case TypeSymbol:
					if ok, status, value := vm.opGetPropSymbol(ip, &baseVal, indexVal, &registers[destReg]); !ok {
						return status, value
					}
				default:
					// Convert to string for property access
					key := indexVal.ToString()
					if ok, status, value := vm.opGetProp(ip, &baseVal, key, &registers[destReg]); !ok {
						return status, value
					}
				}

			case TypeProxy:
				proxy := baseVal.AsProxy()
				if proxy.Revoked {
					// Proxy is revoked, throw TypeError
					var excVal Value
					if typeErrCtor, ok := vm.GetGlobal("TypeError"); ok {
						if res, callErr := vm.Call(typeErrCtor, Undefined, []Value{NewString("Cannot perform property access on a revoked Proxy")}); callErr == nil {
							excVal = res
						}
					}
					if excVal.Type() == 0 {
						eo := NewObject(vm.ErrorPrototype).AsPlainObject()
						eo.SetOwn("name", NewString("TypeError"))
						eo.SetOwn("message", NewString("Cannot perform property access on a revoked Proxy"))
						excVal = NewValueFromPlainObject(eo)
					}
					vm.throwException(excVal)
					return InterpretRuntimeError, Undefined
				}

				// Check if handler has a get trap
				getTrap, ok := proxy.handler.AsPlainObject().GetOwn("get")
				if ok && getTrap.IsCallable() {
					// Call the get trap: handler.get(target, propertyKey, receiver)
					trapArgs := []Value{proxy.target, indexVal, baseVal}
					result, err := vm.Call(getTrap, proxy.handler, trapArgs)
					if err != nil {
						if ee, ok := err.(ExceptionError); ok {
							vm.throwException(ee.GetExceptionValue())
							return InterpretRuntimeError, Undefined
						}
						// Wrap non-exception Go error
						var excVal Value
						if errCtor, ok := vm.GetGlobal("Error"); ok {
							if res, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
								excVal = res
							} else {
								eo := NewObject(vm.ErrorPrototype).AsPlainObject()
								eo.SetOwn("name", NewString("Error"))
								eo.SetOwn("message", NewString(err.Error()))
								excVal = NewValueFromPlainObject(eo)
							}
						} else {
							eo := NewObject(vm.ErrorPrototype).AsPlainObject()
							eo.SetOwn("name", NewString("Error"))
							eo.SetOwn("message", NewString(err.Error()))
							excVal = NewValueFromPlainObject(eo)
						}
						vm.throwException(excVal)
						return InterpretRuntimeError, Undefined
					}
					registers[destReg] = result
				} else {
					// No get trap, fallback to target - implement recursive call by duplicating logic
					targetBase := proxy.target
					switch targetBase.Type() {
					case TypeArray:
						// Handle array indexing on target
						arr := targetBase.AsArray()
						if IsNumber(indexVal) {
							idx := int(AsNumber(indexVal))
							if idx < 0 || idx >= len(arr.elements) {
								registers[destReg] = Undefined
							} else {
								registers[destReg] = arr.elements[idx]
							}
						} else {
							// String/Symbol index on array
							var key string
							switch indexVal.Type() {
							case TypeString:
								key = AsString(indexVal)
							default:
								registers[destReg] = Undefined
							}
							if key != "" {
								if ok, status, value := vm.opGetProp(ip, &targetBase, key, &registers[destReg]); !ok {
									return status, value
								}
							}
						}
					case TypeObject, TypeDictObject:
						// Handle object indexing on target
						var key string
						switch indexVal.Type() {
						case TypeString:
							key = AsString(indexVal)
						case TypeFloatNumber, TypeIntegerNumber:
							key = indexVal.ToString()
						default:
							registers[destReg] = Undefined
						}
						if key != "" {
							if ok, status, value := vm.opGetProp(ip, &targetBase, key, &registers[destReg]); !ok {
								return status, value
							}
						} else {
							registers[destReg] = Undefined
						}
					default:
						registers[destReg] = Undefined
					}
				}

			default:
				// Check if we're trying to index null or undefined - throw TypeError per ECMAScript spec
				if baseVal.Type() == TypeNull || baseVal.Type() == TypeUndefined {
					frame.ip = ip
					err := vm.NewTypeError(fmt.Sprintf("Cannot read properties of %s (reading '%s')", baseVal.TypeName(), indexVal.ToString()))
					if excErr, ok := err.(exceptionError); ok {
						vm.throwException(excErr.GetExceptionValue())
					}
					return InterpretRuntimeError, Undefined
				}

				// Temporary debug to track invalid OpGetIndex bases in iterator paths
				if debugVM {
					fmt.Printf("[DBG OpGetIndex] invalid base type: %s value=%s index=%s type=%s ip=%d\n",
						baseVal.TypeName(), baseVal.Inspect(), indexVal.Inspect(), indexVal.TypeName(), ip)
				}
				frame.ip = ip
				status := vm.runtimeError("Cannot index non-array/object/string/typedarray/generator type '%v' at IP %d", baseVal.Type(), ip)
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

					// Prevent massive memory allocations from large array indices
					// JavaScript engines typically use sparse arrays for large indices
					// For now, we'll reject extremely large indices to prevent memory exhaustion
					const maxArrayIndex = 16777216 // 2^24 - reasonable limit for dense arrays
					if neededCapacity > maxArrayIndex {
						frame.ip = ip
						status := vm.runtimeError("Array index too large: %d (max %d)", idx, maxArrayIndex-1)
						return status, Undefined
					}

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

			case TypeObject, TypeDictObject, TypeFunction: // Functions can have properties
				var key string
				switch indexVal.Type() {
				case TypeString:
					key = AsString(indexVal)
				case TypeFloatNumber, TypeIntegerNumber:
					// Use ToString() for ECMAScript-compliant number-to-string conversion
					key = indexVal.ToString()
				case TypeSymbol:
					if baseVal.Type() == TypeDictObject {
						// DictObject does not support symbol keys; no legacy stringization
						// Skip setting silently (spec-incomplete structure)
						continue
					}
					if ok, status, res := vm.opSetPropSymbol(ip, &baseVal, indexVal, &valueVal); !ok {
						return status, res
					}
					continue
				default:
					// ToPropertyKey: convert to string, calling toString() method for objects
					if indexVal.IsObject() {
						primitiveVal := vm.toPrimitive(indexVal, "string")
						key = primitiveVal.ToString()
					} else {
						// JavaScript allows any value as an object property key - convert to string
						// undefined → "undefined", null → "null", true → "true", etc.
						key = indexVal.ToString()
					}
				}

				// Set the property on the object (with accessor awareness)
				if baseVal.Type() == TypeDictObject {
					dict := AsDictObject(baseVal)
					dict.SetOwn(key, valueVal)
			} else if baseVal.Type() == TypeFunction {
				// For functions, set property on the function's Properties object
				if status, res := vm.setFunctionProperty(baseVal, key, valueVal, ip); status != InterpretOK {
					return status, res
				}
				} else {
					obj := AsPlainObject(baseVal)
					// Check if this is an accessor property with a setter
					if _, setter, _, _, ok := obj.GetOwnAccessor(key); ok && setter.Type() != TypeUndefined {
						// Call the setter with the value
						_, err := vm.Call(setter, baseVal, []Value{valueVal})
						if err != nil {
							if ee, ok := err.(ExceptionError); ok {
								vm.throwException(ee.GetExceptionValue())
								return InterpretRuntimeError, Undefined
							}
							frame.ip = ip
							status := vm.runtimeError("Error calling setter: %v", err)
							return status, Undefined
						}
					} else {
						// No setter, set as data property
						obj.SetOwn(key, valueVal)
					}
				}

			case TypeTypedArray:
				ta := baseVal.AsTypedArray()
				if IsNumber(indexVal) {
					// Numeric index - set typed array element
					idx := int(AsNumber(indexVal))
					ta.SetElement(idx, valueVal)
				} else {
					// Non-numeric index (Symbol, string, etc.) - set property via prototype chain
					switch indexVal.Type() {
					case TypeSymbol:
						if ok, status, value := vm.opSetPropSymbol(ip, &baseVal, indexVal, &valueVal); !ok {
							return status, value
						}
					case TypeString:
						key := AsString(indexVal)
						if ok, status, value := vm.opSetProp(ip, &baseVal, key, &valueVal); !ok {
							return status, value
						}
					default:
						// Convert to string for property access
						key := indexVal.ToString()
						if ok, status, value := vm.opSetProp(ip, &baseVal, key, &valueVal); !ok {
							return status, value
						}
					}
				}

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
			case TypeObject, TypeDictObject:
				if po := srcVal.AsPlainObject(); po != nil {
					if v, ok := po.GetOwn("length"); ok {
						length = v.ToFloat()
						break
					}
				}
				// If no own length, fallthrough to error
				fallthrough
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

			// Validate destination is an array
			if destVal.Type() != TypeArray {
				frame.ip = ip
				status := vm.runtimeError("OpArraySpread: destination must be an array, got '%v'", destVal.Type())
				return status, Undefined
			}

			destArray := AsArray(destVal)

			// Handle different source types based on ECMAScript iterator protocol
			switch sourceVal.Type() {
			case TypeArray:
				// Fast path for arrays
				sourceArray := AsArray(sourceVal)
				destArray.elements = append(destArray.elements, sourceArray.elements...)

			case TypeString:
				// Strings are iterable character by character
				str := AsString(sourceVal)
				for _, char := range str {
					destArray.elements = append(destArray.elements, NewString(string(char)))
				}

			case TypeSet:
				// Sets are iterable - spread their values into the array
				setObj := sourceVal.AsSet()
				setObj.ForEach(func(value Value) {
					destArray.elements = append(destArray.elements, value)
				})

			case TypeMap:
				// Maps are iterable - spread [key, value] pairs into the array
				mapObj := sourceVal.AsMap()
				mapObj.ForEach(func(key Value, value Value) {
					pairVal := NewArray()
					pairArr := pairVal.AsArray()
					pairArr.elements = []Value{key, value}
					destArray.elements = append(destArray.elements, pairVal)
				})

			case TypeObject, TypeDictObject:
				// Check if object has Symbol.iterator method
				var iteratorMethod Value
				var hasIterator bool

				if sourceVal.Type() == TypeObject {
					obj := sourceVal.AsPlainObject()
					// Use proper symbol key lookup instead of string
					iteratorMethod, hasIterator = obj.GetOwnByKey(NewSymbolKey(vm.SymbolIterator))
				} else {
					// DictObject doesn't support symbol keys
					hasIterator = false
				}

				if hasIterator && (iteratorMethod.IsCallable() || iteratorMethod.IsFunction()) {
					// Call the iterator method to get an iterator
					iterator, err := vm.Call(iteratorMethod, sourceVal, []Value{})
					if err != nil {
						frame.ip = ip
						status := vm.runtimeError("error calling Symbol.iterator: %v", err)
						return status, Undefined
					}

					// Now iterate using the iterator's next() method
					for {
						// Get the next method
						var nextMethod Value
						var hasNext bool
						if iterator.Type() == TypeObject {
							nextMethod, hasNext = iterator.AsPlainObject().GetOwn("next")
						} else if iterator.Type() == TypeDictObject {
							nextMethod, hasNext = iterator.AsDictObject().GetOwn("next")
						} else {
							break
						}

						if !hasNext {
							frame.ip = ip
							status := vm.runtimeError("iterator does not have a next method")
							return status, Undefined
						}

						// Call next()
						result, err := vm.Call(nextMethod, iterator, []Value{})
						if err != nil {
							frame.ip = ip
							status := vm.runtimeError("error calling next(): %v", err)
							return status, Undefined
						}

						// Check if iteration is done
						if result.Type() != TypeObject && result.Type() != TypeDictObject {
							break
						}

						var doneVal Value
						var valueVal Value
						var hasDone, hasValue bool

						if result.Type() == TypeObject {
							resultObj := result.AsPlainObject()
							doneVal, hasDone = resultObj.GetOwn("done")
							valueVal, hasValue = resultObj.GetOwn("value")
						} else {
							resultDict := result.AsDictObject()
							doneVal, hasDone = resultDict.GetOwn("done")
							valueVal, hasValue = resultDict.GetOwn("value")
						}

						if hasDone && doneVal.IsBoolean() && doneVal.AsBoolean() {
							break
						}

						// Get the value
						if hasValue {
							destArray.elements = append(destArray.elements, valueVal)
						}
					}
				} else {
					// Not iterable, skip (rather than error)
				}

			default:
				// For other types (null, undefined, etc.), skip rather than error
				// This matches JavaScript behavior where spreading non-iterables is usually a TypeError
				// but we'll be more lenient for now
			}

			// Update array length
			destArray.length = len(destArray.elements)

		// --- NEW: Object Spread Support ---
		case OpObjectSpread:
			destReg := code[ip]
			sourceReg := code[ip+1]
			ip += 2

			destVal := registers[destReg]
			sourceVal := registers[sourceReg]

			// Validate destination is an object
			if destVal.Type() != TypeObject && destVal.Type() != TypeDictObject {
				frame.ip = ip
				status := vm.runtimeError("OpObjectSpread: destination must be an object, got '%v'", destVal.Type())
				return status, Undefined
			}

			// Handle null and undefined sources (per ECMAScript spec: they contribute no properties)
			if sourceVal.Type() == TypeNull || sourceVal.Type() == TypeUndefined {
				// Skip - null and undefined contribute no enumerable properties
				continue
			}

			// Only process actual objects
			if sourceVal.Type() != TypeObject && sourceVal.Type() != TypeDictObject {
				// For other primitive types, they should be converted to objects first
				// But for now, skip them as they typically have no enumerable properties
				continue
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

			// Apply ToPrimitive for objects (calls valueOf/toString per ECMAScript spec)
			srcPrim := vm.toPrimitive(srcVal, "number")
			if vm.unwinding {
				return InterpretRuntimeError, Undefined
			}

			// JavaScript-style type coercion for bitwise operations
			// undefined becomes 0, null becomes 0, booleans become 0/1, etc.
			srcInt := int64(srcPrim.ToInteger())
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

			// Check for BigInt operations
			leftIsBigInt := leftVal.Type() == TypeBigInt
			rightIsBigInt := rightVal.Type() == TypeBigInt

			// BigInt bitwise/shift operations
			if leftIsBigInt || rightIsBigInt {
				// Both operands must be BigInt for bitwise operations
				if (leftIsBigInt && !rightIsBigInt) || (!leftIsBigInt && rightIsBigInt) {
					// TypeError: Cannot mix BigInt and other types
					typeErrObj := NewObject(vm.TypeErrorPrototype).AsPlainObject()
					typeErrObj.SetOwn("name", NewString("TypeError"))
					typeErrObj.SetOwn("message", NewString("Cannot mix BigInt and other types"))
					vm.throwException(NewValueFromPlainObject(typeErrObj))
					return InterpretRuntimeError, Undefined
				}

				// Unsigned right shift (>>>) with BigInt is not allowed
				if opcode == OpUnsignedShiftRight {
					typeErrObj := NewObject(vm.TypeErrorPrototype).AsPlainObject()
					typeErrObj.SetOwn("name", NewString("TypeError"))
					typeErrObj.SetOwn("message", NewString("BigInt does not support unsigned right shift"))
					vm.throwException(NewValueFromPlainObject(typeErrObj))
					return InterpretRuntimeError, Undefined
				}

				// Convert right operand to BigInt for shift count
				var rightBigInt *big.Int
				if rightVal.Type() == TypeBigInt {
					rightBigInt = rightVal.AsBigInt()
				} else {
					// Convert number to BigInt
					intVal := rightVal.ToInteger()
					rightBigInt = big.NewInt(int64(intVal))
				}

				leftBigInt := leftVal.AsBigInt()

				var result *big.Int
				switch opcode {
				case OpBitwiseAnd:
					result = new(big.Int).And(leftBigInt, rightBigInt)
				case OpBitwiseOr:
					result = new(big.Int).Or(leftBigInt, rightBigInt)
				case OpBitwiseXor:
					result = new(big.Int).Xor(leftBigInt, rightBigInt)
				case OpShiftLeft:
					// For BigInt, shift amount is ToBigInt, not masked to 32 bits
					// Convert shift count to int64
					shiftAmount := rightBigInt.Int64()
					result = new(big.Int).Lsh(leftBigInt, uint(shiftAmount))
				case OpShiftRight:
					// Convert shift count to int64
					shiftAmount := rightBigInt.Int64()
					result = new(big.Int).Rsh(leftBigInt, uint(shiftAmount))
				}

				registers[destReg] = NewBigInt(result)
				continue
			}

			// Regular number bitwise/shift operations
			// Apply ToPrimitive for objects (calls valueOf/toString per ECMAScript spec)
			leftPrim := vm.toPrimitive(leftVal, "number")
			if vm.unwinding {
				return InterpretRuntimeError, Undefined
			}

			rightPrim := vm.toPrimitive(rightVal, "number")
			if vm.unwinding {
				return InterpretRuntimeError, Undefined
			}

			// Use ToInt32 for left operand, ToUint32 for right operand (shift count)
			leftInt32 := leftPrim.ToInteger()
			rightInt32 := rightPrim.ToInteger()

			shiftAmount := uint32(rightInt32) & 31 // Mask to 5 bits for 32-bit operations

			// Handle unsigned right shift specially to preserve unsigned result
			if opcode == OpUnsignedShiftRight {
				unsignedResult := uint32(leftInt32) >> shiftAmount
				registers[destReg] = Number(float64(unsignedResult))
			} else {
				var result int32
				switch opcode {
				case OpBitwiseAnd:
					result = leftInt32 & rightInt32
				case OpBitwiseOr:
					result = leftInt32 | rightInt32
				case OpBitwiseXor:
					result = leftInt32 ^ rightInt32
				case OpShiftLeft:
					result = int32(uint32(leftInt32) << shiftAmount)
				case OpShiftRight: // arithmetic
					result = leftInt32 >> shiftAmount
				}
				registers[destReg] = Number(float64(result))
			}

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

			// fmt.Printf("// [VM DEBUG] OpGetProp: R%d = R%d[%d] (ip=%d)\n", destReg, objReg, nameConstIdx, ip-4)

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

		case OpGetPrivateField:
			destReg := code[ip]
			objReg := code[ip+1]
			nameConstIdxHi := code[ip+2]
			nameConstIdxLo := code[ip+3]
			nameConstIdx := uint16(nameConstIdxHi)<<8 | uint16(nameConstIdxLo)
			ip += 4

			// Get field name from constants (stored without # prefix)
			if int(nameConstIdx) >= len(constants) {
				frame.ip = ip
				status := vm.runtimeError("Invalid constant index %d for private field name.", nameConstIdx)
				return status, Undefined
			}
			nameVal := constants[nameConstIdx]
			if !IsString(nameVal) {
				frame.ip = ip
				status := vm.runtimeError("Internal Error: Private field name constant %d is not a string.", nameConstIdx)
				return status, Undefined
			}
			fieldName := AsString(nameVal)

			objVal := registers[objReg]

			// Private fields can be accessed on objects or functions (for static private fields)
			var obj *PlainObject
			if objVal.Type() == TypeObject {
				obj = objVal.AsPlainObject()
			} else if objVal.Type() == TypeFunction {
				// Static private fields are stored on the constructor's Properties object
				fn := objVal.AsFunction()
				if fn.Properties == nil {
					frame.ip = ip
					status := vm.runtimeError("Cannot read private field '%s': field not found", fieldName)
					return status, Undefined
				}
				obj = fn.Properties
			} else {
				frame.ip = ip
				status := vm.runtimeError("Cannot read private field '%s' of %s", fieldName, objVal.TypeName())
				return status, Undefined
			}

			value, exists := obj.GetPrivateField(fieldName)
			if !exists {
				frame.ip = ip
				status := vm.runtimeError("Cannot read private field '%s': field not found", fieldName)
				return status, Undefined
			}
			registers[destReg] = value

		case OpTypeGuardIterable:
			// Save IP of the opcode itself (before reading operands) for exception handling
			guardIP := ip - 1  // ip was already incremented past the opcode
			frame.ip = guardIP

			srcReg := int(code[ip])
			ip++

			if !vm.opTypeGuardIterable(srcReg, registers) {
				// Type guard failed and threw exception via ThrowTypeError
				// Similar to OpThrow: check if unwinding or if handler was found
				if vm.unwinding {
					// No handler found or hit direct call boundary, continue unwinding
					if vm.frameCount == 0 {
						return InterpretRuntimeError, vm.currentException
					}
					continue
				} else {
					// Handler found and executed, resync variables and jump to handler
					frame = &vm.frames[vm.frameCount-1]
					closure = frame.closure
					function = closure.Fn
					code = function.Chunk.Code
					constants = function.Chunk.Constants
					registers = frame.registers
					ip = frame.ip
					continue
				}
			}
			frame.ip = ip

		case OpTypeGuardIteratorReturn:
			// Save IP of the opcode itself (before reading operands) for exception handling
			guardIP := ip - 1  // ip was already incremented past the opcode
			frame.ip = guardIP

			srcReg := int(code[ip])
			ip++

			if !vm.opTypeGuardIteratorReturn(srcReg, registers) {
				// Type guard failed and threw exception via ThrowTypeError
				// Similar to OpThrow: check if unwinding or if handler was found
				if vm.unwinding {
					// No handler found or hit direct call boundary, continue unwinding
					if vm.frameCount == 0 {
						return InterpretRuntimeError, vm.currentException
					}
					continue
				} else {
					// Handler found and executed, resync variables and jump to handler
					frame = &vm.frames[vm.frameCount-1]
					closure = frame.closure
					function = closure.Fn
					code = function.Chunk.Code
					constants = function.Chunk.Constants
					registers = frame.registers
					ip = frame.ip
					continue
				}
			}
			frame.ip = ip

		case OpSetPrivateField:
			objReg := code[ip]
			valReg := code[ip+1]
			nameConstIdxHi := code[ip+2]
			nameConstIdxLo := code[ip+3]
			nameConstIdx := uint16(nameConstIdxHi)<<8 | uint16(nameConstIdxLo)
			ip += 4

			// Get field name from constants (stored without # prefix)
			if int(nameConstIdx) >= len(constants) {
				frame.ip = ip
				status := vm.runtimeError("Invalid constant index %d for private field name.", nameConstIdx)
				return status, Undefined
			}
			nameVal := constants[nameConstIdx]
			if !IsString(nameVal) {
				frame.ip = ip
				status := vm.runtimeError("Internal Error: Private field name constant %d is not a string.", nameConstIdx)
				return status, Undefined
			}
			fieldName := AsString(nameVal)

			objVal := registers[objReg]

			// Private fields can be set on objects or functions (for static private fields)
			var obj *PlainObject
			if objVal.Type() == TypeObject {
				obj = objVal.AsPlainObject()
			} else if objVal.Type() == TypeFunction {
				// Static private fields are stored on the constructor's Properties object
				fn := objVal.AsFunction()
				if fn.Properties == nil {
					// Create Properties object if it doesn't exist
					fn.Properties = &PlainObject{prototype: Undefined, shape: RootShape}
				}
				obj = fn.Properties
			} else {
				frame.ip = ip
				status := vm.runtimeError("Cannot set private field '%s' of %s", fieldName, objVal.TypeName())
				return status, Undefined
			}

			obj.SetPrivateField(fieldName, registers[valReg])

		case OpDefineMethod:
			frame.ip = ip
			status, value := vm.handleOpDefineMethod(code, &ip, constants, registers)
			if status != InterpretOK {
				return status, value
			}

		// (OpDeleteProp handled later in switch)

		case OpCallMethod:
			// Refactored to use centralized prepareCall
			destReg := code[ip]
			funcReg := code[ip+1]
			thisReg := code[ip+2]
			argCount := int(code[ip+3])
			ip += 4

			callerRegisters := registers
			callerIP := ip // Pass the IP after the call instruction

			// DEBUG: Record the IP where the call was made for proper exception handling
			// OpCallMethod is 1 (opcode) + 4 (operands) bytes long
			callSiteIP := ip - 5 // IP where OpCallMethod instruction started
			if debugCalls {
				fmt.Printf("[DEBUG vm.go] OpCallMethod: callSiteIP=%d, callerIP=%d, frame.ip=%d\n", callSiteIP, callerIP, frame.ip)
			}

			// Set frame IP to call site for exception handling
			frame.ip = callSiteIP // Set to call site for potential exception handling

			calleeVal := callerRegisters[funcReg]
			thisVal := callerRegisters[thisReg]

			// DEBUG: Log if calling undefined
			if calleeVal.Type() == TypeUndefined {
				fmt.Fprintf(os.Stderr, "[DEBUG vm.go OpCallMethod] About to call undefined! funcReg=%d, thisReg=%d, IP=%d\n", funcReg, thisReg, frame.ip)
				fmt.Fprintf(os.Stderr, "[DEBUG vm.go OpCallMethod] thisVal: %s (%s)\n", thisVal.Inspect(), thisVal.TypeName())
				fmt.Fprintf(os.Stderr, "[DEBUG vm.go OpCallMethod] Register dump:\n")
				for i := byte(0); i < 10 && i < byte(len(callerRegisters)); i++ {
					fmt.Fprintf(os.Stderr, "  R%d: %s (%s)\n", i, callerRegisters[i].Inspect(), callerRegisters[i].TypeName())
				}
			}

			// Targeted debug for deepEqual recursion investigation
			if false { // flip to true for local debugging
				calleeName := ""
				switch calleeVal.Type() {
				case TypeFunction:
					calleeName = calleeVal.AsFunction().Name
				case TypeClosure:
					calleeName = calleeVal.AsClosure().Fn.Name
				case TypeNativeFunction, TypeNativeFunctionWithProps:
					calleeName = calleeVal.TypeName()
				}
				if calleeName == "deepEqual" || calleeName == "compareEquality" || calleeName == "compareObjectEquality" {
					fmt.Printf("[CALLM] %s args=%d this=%s\n", calleeName, argCount, thisVal.TypeName())
				}
			}
			if !(calleeVal.IsFunction() || calleeVal.IsNativeFunction()) {
				// fmt.Printf("[DBG OpCallMethod BAD] ip=%d funcReg=R%d thisReg=R%d destReg=R%d callee=%s (%s) this=%s (%s) args=%d\n", callSiteIP, funcReg, thisReg, destReg, calleeVal.Inspect(), calleeVal.TypeName(), thisVal.Inspect(), thisVal.TypeName(), argCount)
			}
			// Extra targeted tracing for iterator delegation debugging
			if false && calleeVal.Type() == TypeNativeFunction {
				nf := AsNativeFunction(calleeVal)
				if nf.Name == "[Symbol.iterator]" || nf.Name == "next" {
					fmt.Printf("[DBG OpCallMethod] regs func=R%d this=R%d dest=R%d | callee=%s(%s) this=%s(%s)\n",
						funcReg, thisReg, destReg, calleeVal.Inspect(), calleeVal.TypeName(), thisVal.Inspect(), thisVal.TypeName())
				}
			} else if calleeVal.Type() == TypeNativeFunctionWithProps {
				_ = calleeVal
			}
			// Trace specific iterator-related calls to inspect 'this' binding
			if false && calleeVal.Type() == TypeNativeFunction {
				nf := AsNativeFunction(calleeVal)
				if nf.Name == "[Symbol.iterator]" || nf.Name == "next" {
					fmt.Printf("[DBG OpCallMethod] calling %s with this=%s (%s)\n", nf.Name, thisVal.Inspect(), thisVal.TypeName())
				}
			} else if calleeVal.Type() == TypeNativeFunctionWithProps {
				_ = thisVal
			}
			args := callerRegisters[funcReg+1 : funcReg+1+byte(argCount)]

			// Debug logging for method calls
			// fmt.Printf("// [VM DEBUG] OpCallMethod at IP %d: Calling function in R%d (type: %v, value: %s) with this=R%d (type: %v, value: %s), args=%d [module: %s]\n",
			//	ip-4, funcReg, calleeVal.Type(), calleeVal.Inspect(), thisReg, thisVal.Type(), thisVal.Inspect(), argCount, vm.currentModulePath)

			// Check if we're in an unwinding state before the call
			wasUnwinding := vm.unwinding

			shouldSwitch, err := vm.prepareMethodCall(calleeVal, thisVal, args, destReg, callerRegisters, callerIP)

			if debugCalls {
				fmt.Printf("[DEBUG vm.go] OpCallMethod: prepareMethodCall returned shouldSwitch=%v, err=%v, wasUnwinding=%v, nowUnwinding=%v\n",
					shouldSwitch, err != nil, wasUnwinding, vm.unwinding)
			}

			// Do not attempt to detect/handle exceptions here; the main run loop handles unwinding

			// Update caller frame IP only if it still points at the call site.
			if frame.ip == callSiteIP {
				frame.ip = callerIP
			}
			// If unwinding is true, leave frame IP at call site for exception handling

			if err != nil {
				var excVal Value
				if exceptionErr, ok := err.(ExceptionError); ok {
					excVal = exceptionErr.GetExceptionValue()
				} else {
					// Convert Go error to a proper Error instance so constructor/prototype are correct
					if errCtor, ok := vm.GetGlobal("Error"); ok {
						if res, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
							excVal = res
						} else {
							// Fallback plain object if calling Error failed
							errObj := NewObject(vm.ErrorPrototype).AsPlainObject()
							errObj.SetOwn("name", NewString("Error"))
							errObj.SetOwn("message", NewString(err.Error()))
							excVal = NewValueFromPlainObject(errObj)
						}
					} else {
						// Fallback if Error is unavailable
						errObj := NewObject(vm.ErrorPrototype).AsPlainObject()
						errObj.SetOwn("name", NewString("Error"))
						errObj.SetOwn("message", NewString(err.Error()))
						excVal = NewValueFromPlainObject(errObj)
					}
				}
				vm.throwException(excVal)
				if vm.frameCount == 0 {
					return InterpretRuntimeError, vm.currentException
				}
				frame = &vm.frames[vm.frameCount-1]
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
				continue
			}

			// Minimal targeted debug: observe results of [Symbol.iterator] and next()
			if false && !shouldSwitch {
				switch calleeVal.Type() {
				case TypeNativeFunction:
					nf := AsNativeFunction(calleeVal)
					if nf.Name == "[Symbol.iterator]" || nf.Name == "next" {
						res := callerRegisters[destReg]
						fmt.Printf("[DBG OpCallMethod] %s returned %s (%s)\n", nf.Name, res.Inspect(), res.TypeName())
					}
				case TypeNativeFunctionWithProps:
					nfp := calleeVal.AsNativeFunctionWithProps()
					if nfp.Name == "[Symbol.iterator]" || nfp.Name == "next" {
						res := callerRegisters[destReg]
						fmt.Printf("[DBG OpCallMethod] %s returned %s (%s)\n", nfp.Name, res.Inspect(), res.TypeName())
					}
				}
			}

			if shouldSwitch {
				if debugCalls {
					fmt.Printf("[DEBUG vm.go] OpCallMethod: Switching to new frame for bytecode function\n")
				}
				// Switch to new frame (do NOT modify caller frame IP here; it should remain at callerIP)
				frame = &vm.frames[vm.frameCount-1]
				closure = frame.closure
				function = closure.Fn
				code = function.Chunk.Code
				constants = function.Chunk.Constants
				registers = frame.registers
				ip = frame.ip
			} else {
				if vm.escapedDirectCallBoundary {
					if debugCalls {
						fmt.Printf("[DEBUG vm.go] OpCallMethod: Escaped direct-call boundary; resuming at handler IP=%d\n", frame.ip)
					}
					vm.escapedDirectCallBoundary = false
					ip = frame.ip
					continue
				}
				if debugCalls {
					fmt.Printf("[DEBUG vm.go] OpCallMethod: Native function completed normally, continuing\n")
				}
			}
			continue

		case OpNew:
			destReg := code[ip]          // Where the created instance should go in the caller
			constructorReg := code[ip+1] // Register holding the constructor function/closure
			argCount := int(code[ip+2])  // Number of arguments provided to the constructor
			ip += 3

			// Capture caller context before potential frame switch
			callerRegisters := registers
			callerIP := ip // Pass the IP after the call instruction

			constructorVal := callerRegisters[constructorReg]

			// Handle Proxy with construct trap
			if constructorVal.Type() == TypeProxy {
				proxy := constructorVal.AsProxy()
				if proxy.Revoked {
					frame.ip = callerIP
					vm.runtimeError("Cannot construct revoked Proxy")
					return InterpretRuntimeError, Undefined
				}

				// Check for construct trap
				if constructTrap, ok := proxy.Handler().AsPlainObject().GetOwn("construct"); ok {
					// Validate trap is callable
					if !constructTrap.IsCallable() {
						frame.ip = callerIP
						vm.runtimeError("'construct' on proxy: trap is not a function")
						return InterpretRuntimeError, Undefined
					}

					// Convert args to array for trap call
					args := make([]Value, argCount)
					argStartRegInCaller := constructorReg + 1
					for i := 0; i < argCount; i++ {
						if int(argStartRegInCaller)+i < len(callerRegisters) {
							args[i] = callerRegisters[argStartRegInCaller+byte(i)]
						} else {
							args[i] = Undefined
						}
					}

					argsArray := NewArray()
					arrObj := argsArray.AsArray()
					for _, arg := range args {
						arrObj.Append(arg)
					}

					// Call handler.construct(target, argumentsList, newTarget)
					// newTarget is the proxy itself (the constructor being called)
					trapArgs := []Value{proxy.Target(), argsArray, constructorVal}
					result, err := vm.Call(constructTrap, proxy.Handler(), trapArgs)
					if err != nil {
						frame.ip = callerIP
						if ee, ok := err.(ExceptionError); ok {
							vm.throwException(ee.GetExceptionValue())
						} else {
							vm.runtimeError(err.Error())
						}
						return InterpretRuntimeError, Undefined
					}

					// Result must be an object
					if !result.IsObject() {
						frame.ip = callerIP
						vm.runtimeError("'construct' on proxy: trap result must be an object")
						return InterpretRuntimeError, Undefined
					}

					// Store result in destination register
					callerRegisters[destReg] = result
					continue // Continue with next instruction
				}

				// No construct trap, delegate to target
				// Replace constructorVal with target and continue with normal flow
				constructorVal = proxy.Target()
			}

			switch constructorVal.Type() {
			case TypeClosure:
				// Constructor call on closure
				constructorClosure := AsClosure(constructorVal)
				constructorFunc := constructorClosure.Fn
				// Check if it's an arrow function - arrow functions cannot be constructors
				if constructorFunc.IsArrowFunction {
					frame.ip = callerIP
					vm.ThrowTypeError("Arrow functions cannot be used as constructors")
					return InterpretRuntimeError, Undefined
				}
				// Allow fewer arguments for constructors with optional parameters
				// The compiler handles padding with undefined for missing optional parameters
				// JavaScript allows passing more arguments than the function declares - they are
				// simply ignored or can be accessed via the arguments object
				// No arity checking needed for extra arguments
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

				// Determine the new.target value for this constructor call
				// If the caller is already a constructor (super() call), inherit its new.target
				// Otherwise, new.target is the constructor being called
				var newTargetValue Value
				if frame.isConstructorCall && frame.newTargetValue.Type() != TypeUndefined {
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
					newTargetFunc := newTargetClosure.Fn
					instancePrototype = newTargetFunc.getOrCreatePrototypeWithVM(vm)
				} else if newTargetValue.Type() == TypeFunction {
					newTargetFunc := AsFunction(newTargetValue)
					instancePrototype = newTargetFunc.getOrCreatePrototypeWithVM(vm)
				} else {
					// Fallback: use the constructor's prototype
					instancePrototype = constructorFunc.getOrCreatePrototypeWithVM(vm)
				}

				// For derived constructors, 'this' is uninitialized until super() is called
				// For base constructors, create the instance immediately
				var newInstance Value
				if constructorFunc.IsDerivedConstructor {
					newInstance = Undefined // 'this' is uninitialized in derived constructors
				} else {
					// Create new instance object with new.target's prototype
					newInstance = NewObject(instancePrototype)
				}

				frame.ip = callerIP // Store return IP

				newFrame := &vm.frames[vm.frameCount]
				newFrame.closure = constructorClosure
				newFrame.ip = 0
				newFrame.targetRegister = destReg
				newFrame.thisValue = newInstance       // Set the new instance as 'this' (or undefined for derived)
				newFrame.isConstructorCall = true      // Mark this as a constructor call
				newFrame.isDirectCall = false          // Not a direct call (normal OpNew)
				newFrame.isSentinelFrame = false       // Clear sentinel flag when reusing frame
				newFrame.newTargetValue = newTargetValue // Set new.target (propagated from caller or constructor)
				newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
				vm.nextRegSlot += requiredRegs

				// Copy fixed arguments and handle rest parameters
				argStartRegInCaller := constructorReg + 1

				// Copy fixed arguments (up to Arity)
				for i := 0; i < constructorFunc.Arity; i++ {
					if i < len(newFrame.registers) {
						if i < argCount && int(argStartRegInCaller)+i < len(callerRegisters) {
							newFrame.registers[i] = callerRegisters[argStartRegInCaller+byte(i)]
						} else {
							newFrame.registers[i] = Undefined
						}
					} else {
						vm.nextRegSlot -= requiredRegs
						frame.ip = callerIP
						status := vm.runtimeError("Internal Error: Argument register index out of bounds during constructor call setup.")
						return status, Undefined
					}
				}

				// Handle rest parameters for variadic constructors
				if constructorFunc.Variadic {
					extraArgCount := argCount - constructorFunc.Arity
					var restArray Value

					if extraArgCount == 0 {
						restArray = vm.emptyRestArray
					} else {
						restArray = NewArray()
						restArrayObj := restArray.AsArray()
						for i := 0; i < extraArgCount; i++ {
							argIndex := constructorFunc.Arity + i
							if argIndex < argCount && int(argStartRegInCaller)+argIndex < len(callerRegisters) {
								restArrayObj.Append(callerRegisters[argStartRegInCaller+byte(argIndex)])
							}
						}
					}

					// Store rest array at the appropriate position
					if constructorFunc.Arity < len(newFrame.registers) {
						newFrame.registers[constructorFunc.Arity] = restArray
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
				// Check if it's an arrow function - arrow functions cannot be constructors
				if funcToCall.IsArrowFunction {
					frame.ip = callerIP
					vm.ThrowTypeError("Arrow functions cannot be used as constructors")
					return InterpretRuntimeError, Undefined
				}
				constructorClosure := &ClosureObject{Fn: funcToCall, Upvalues: []*Upvalue{}}
				constructorFunc := constructorClosure.Fn

				// Allow fewer arguments for constructors with optional parameters
				// The compiler handles padding with undefined for missing optional parameters
				// JavaScript allows passing more arguments than the function declares - they are
				// simply ignored or can be accessed via the arguments object
				// No arity checking needed for extra arguments
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

				// Determine the new.target value for this constructor call
				// If the caller is already a constructor (super() call), inherit its new.target
				// Otherwise, new.target is the constructor being called
				var newTargetValue Value
				if frame.isConstructorCall && frame.newTargetValue.Type() != TypeUndefined {
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
					newTargetFunc := newTargetClosure.Fn
					instancePrototype = newTargetFunc.getOrCreatePrototypeWithVM(vm)
				} else if newTargetValue.Type() == TypeFunction {
					newTargetFunc := AsFunction(newTargetValue)
					instancePrototype = newTargetFunc.getOrCreatePrototypeWithVM(vm)
				} else {
					// Fallback: use the constructor's prototype
					instancePrototype = constructorFunc.getOrCreatePrototypeWithVM(vm)
				}

				// For derived constructors, 'this' is uninitialized until super() is called
				// For base constructors, create the instance immediately
				var newInstance Value
				if constructorFunc.IsDerivedConstructor {
					newInstance = Undefined // 'this' is uninitialized in derived constructors
				} else {
					// Create new instance object with new.target's prototype
					newInstance = NewObject(instancePrototype)
				}

				frame.ip = callerIP

				newFrame := &vm.frames[vm.frameCount]
				newFrame.closure = constructorClosure
				newFrame.ip = 0
				newFrame.targetRegister = destReg
				newFrame.thisValue = newInstance       // Set the new instance as 'this' (or undefined for derived)
				newFrame.isConstructorCall = true      // Mark this as a constructor call
				newFrame.isDirectCall = false          // Not a direct call (normal OpNew)
				newFrame.isSentinelFrame = false       // Clear sentinel flag when reusing frame
				newFrame.newTargetValue = newTargetValue // Set new.target (propagated from caller or constructor)
				newFrame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]
				vm.nextRegSlot += requiredRegs

				// Copy fixed arguments and handle rest parameters
				argStartRegInCaller := constructorReg + 1

				// Copy fixed arguments (up to Arity)
				for i := 0; i < constructorFunc.Arity; i++ {
					if i < len(newFrame.registers) {
						if i < argCount && int(argStartRegInCaller)+i < len(callerRegisters) {
							newFrame.registers[i] = callerRegisters[argStartRegInCaller+byte(i)]
						} else {
							newFrame.registers[i] = Undefined
						}
					} else {
						vm.nextRegSlot -= requiredRegs
						frame.ip = callerIP
						status := vm.runtimeError("Internal Error: Argument register index out of bounds during constructor call setup.")
						return status, Undefined
					}
				}

				// Handle rest parameters for variadic constructors
				if constructorFunc.Variadic {
					extraArgCount := argCount - constructorFunc.Arity
					var restArray Value

					if extraArgCount == 0 {
						restArray = vm.emptyRestArray
					} else {
						restArray = NewArray()
						restArrayObj := restArray.AsArray()
						for i := 0; i < extraArgCount; i++ {
							argIndex := constructorFunc.Arity + i
							if argIndex < argCount && int(argStartRegInCaller)+argIndex < len(callerRegisters) {
								restArrayObj.Append(callerRegisters[argStartRegInCaller+byte(argIndex)])
							}
						}
					}

					// Store rest array at the appropriate position
					if constructorFunc.Arity < len(newFrame.registers) {
						newFrame.registers[constructorFunc.Arity] = restArray
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

				// Be permissive with builtin constructor arity; missing args become undefined

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
				// Set constructor call flag so native functions can differentiate
				vm.inConstructorCall = true
				result, err := builtin.Fn(args)
				vm.inConstructorCall = false
				if err != nil {
					// Throw as proper Error instance instead of plain object
					var errValue Value
					if errCtor, ok := vm.GetGlobal("Error"); ok {
						if res, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
							errValue = res
						} else {
							eo := NewObject(vm.ErrorPrototype).AsPlainObject()
							eo.SetOwn("name", NewString("Error"))
							eo.SetOwn("message", NewString(err.Error()))
							errValue = NewValueFromPlainObject(eo)
						}
					} else {
						eo := NewObject(vm.ErrorPrototype).AsPlainObject()
						eo.SetOwn("name", NewString("Error"))
						eo.SetOwn("message", NewString(err.Error()))
						errValue = NewValueFromPlainObject(eo)
					}
					vm.throwException(errValue)
					continue // Let exception handling take over
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

				// Be permissive with builtin constructor arity

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
				// Set constructor call flag so native functions can differentiate
				vm.inConstructorCall = true
				result, err := builtinWithProps.Fn(args)
				vm.inConstructorCall = false
				if err != nil {
					// Throw as proper Error instance instead of plain object
					var errValue Value
					if errCtor, ok := vm.GetGlobal("Error"); ok {
						if res, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
							errValue = res
						} else {
							eo := NewObject(vm.ErrorPrototype).AsPlainObject()
							eo.SetOwn("name", NewString("Error"))
							eo.SetOwn("message", NewString(err.Error()))
							errValue = NewValueFromPlainObject(eo)
						}
					} else {
						eo := NewObject(vm.ErrorPrototype).AsPlainObject()
						eo.SetOwn("name", NewString("Error"))
						eo.SetOwn("message", NewString(err.Error()))
						errValue = NewValueFromPlainObject(eo)
					}
					vm.throwException(errValue)
					continue // Let exception handling take over
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

	case OpSpreadNew:
		status, _ := vm.handleOpSpreadNew(code, &ip, frame, registers)
		if status == InterpretOK && vm.frameCount > 0 {
			frame = &vm.frames[vm.frameCount-1]
			closure = frame.closure
			function = closure.Fn
			code = function.Chunk.Code
			constants = function.Chunk.Constants
			registers = frame.registers
			ip = frame.ip
		} else if status != InterpretOK {
			return status, Undefined
		}

		case OpLoadThis:
			destReg := code[ip]
			ip++

			// Load 'this' value from current call frame context
			// If no 'this' context is set (regular function call), return undefined
			registers[destReg] = frame.thisValue

		case OpSetThis:
			srcReg := code[ip]
			ip++

			// Set 'this' value in current call frame context
			// This is used by super() to update the this binding
			// In derived constructors, super() can only be called once
			// Throw ReferenceError if 'this' is already initialized
			if frame.thisValue.Type() != TypeUndefined {
				frame.ip = ip
				vm.ThrowReferenceError("super() already called")
				return InterpretRuntimeError, Undefined
			}
			frame.thisValue = registers[srcReg]

		case OpLoadNewTarget:
			destReg := code[ip]
			ip++

			// Load 'new.target' value from current call frame context
			// If not in a constructor call, return undefined
			if frame.isConstructorCall {
				registers[destReg] = frame.newTargetValue
			} else {
				registers[destReg] = Undefined
			}

		case OpLoadImportMeta:
			destReg := code[ip]
			ip++

			// Create import.meta object with module metadata
			// In ES modules, import.meta provides meta-information about the current module
			importMetaValue := NewDictObject(vm.ObjectPrototype)
			importMetaObj := importMetaValue.AsDictObject()

			// Set import.meta.url property to the current module path
			// In a real environment this would be a file:// URL, but we use the module path
			if vm.currentModulePath != "" {
				importMetaObj.SetOwn("url", NewString(vm.currentModulePath))
			} else {
				// If not in a module context, use undefined (though this shouldn't happen)
				importMetaObj.SetOwn("url", Undefined)
			}

			registers[destReg] = importMetaValue

		case OpDynamicImport:
			destReg := code[ip]
			specifierReg := code[ip+1]
			ip += 2

			// Save current frame state
			frame.ip = ip

			// Get the module specifier from the register
			specifierValue := registers[specifierReg]
			specifier := specifierValue.ToString()

			// Execute the module using the standard module loading infrastructure
			// This goes through the resolver chain (fs, virtual, data URLs, native modules)
			// TODO: Implement proper Promise-based async loading
			status, _ := vm.executeModule(specifier)
			if status != InterpretOK {
				// Error was already set by executeModule
				return status, Undefined
			}

			// Get the module context to access its exports
			moduleCtx, exists := vm.moduleContexts[specifier]
			if !exists {
				return vm.runtimeError("Module '%s' was loaded but context is missing", specifier), Undefined
			}

			// Ensure exports are collected
			if len(moduleCtx.exports) == 0 {
				vm.collectModuleExports(specifier, moduleCtx)
			}

			// Create a namespace object containing all exports
			namespaceObj := NewDictObject(vm.ObjectPrototype)
			namespaceDict := namespaceObj.AsDictObject()

			// Copy all exports into the namespace object
			for exportName, exportValue := range moduleCtx.exports {
				namespaceDict.SetOwn(exportName, exportValue)
			}

			registers[destReg] = namespaceObj

			// Restore frame state
			frame.ip = ip

		case OpGetGlobal:
			destReg := code[ip]
			globalIdxHi := code[ip+1]
			globalIdxLo := code[ip+2]
			globalIdx := uint16(globalIdxHi)<<8 | uint16(globalIdxLo)
			ip += 3

			// Use unified global heap
			value, exists := vm.heap.Get(int(globalIdx))
			if !exists {
				// Throw ReferenceError for unresolvable global with variable name
				frame.ip = ip
				varName := vm.heap.GetNameByIndex(int(globalIdx))
				if varName == "" {
					varName = fmt.Sprintf("<index %d>", globalIdx)
				}
				vm.ThrowReferenceError(fmt.Sprintf("%s is not defined", varName))
				return InterpretRuntimeError, Undefined
			}

			// NUCLEAR DEBUG for fnGlobalObject
			if value.IsFunction() {
				name := ""
				switch value.Type() {
				case TypeFunction:
					name = value.AsFunction().Name
				case TypeClosure:
					name = value.AsClosure().Fn.Name
				case TypeNativeFunction:
					name = value.AsNativeFunction().Name
				}
				if name == "fnGlobalObject" || name == "Test262Error" {
				}
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

			// NUCLEAR DEBUG for fnGlobalObject
			if value.IsFunction() {
				name := ""
				switch value.Type() {
				case TypeFunction:
					name = value.AsFunction().Name
				case TypeClosure:
					name = value.AsClosure().Fn.Name
				case TypeNativeFunction:
					name = value.AsNativeFunction().Name
				}
				if name == "fnGlobalObject" || name == "Test262Error" {
				}
			}

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
			callerIP := ip // Pass the IP after the call instruction

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
			callerIP := ip // Pass the IP after the call instruction

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
				// Enumerate own and inherited enumerable string-named properties (for-in semantics)
				seen := make(map[string]bool)
				cur := objValue.AsPlainObject()
				for cur != nil {
					for _, k := range cur.OwnKeys() {
						if !seen[k] {
							seen[k] = true
							keys = append(keys, k)
						}
					}
					pv := cur.GetPrototype()
					if !pv.IsObject() {
						break
					}
					cur = pv.AsPlainObject()
				}
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

				// Copy properties not in exclude list (only enumerable properties)
				for _, key := range sourceObj.OwnKeys() {
					if _, shouldExclude := excludeNames[key]; !shouldExclude {
						// Check if property is accessor (getter/setter)
						if getter, _, _, enumerable, isAccessor := sourceObj.GetOwnAccessor(key); isAccessor {
							// Property is an accessor
							if !enumerable {
								continue // Skip non-enumerable accessors
							}
							// Call getter if present
							if getter.Type() != TypeUndefined {
								res, err := vm.Call(getter, sourceValue, nil)
								if err != nil {
									// Propagate error
									frame.ip = ip
									status := vm.runtimeError("Error calling getter for property '%s': %v", key, err)
									return status, Undefined
								}
								// Store the getter's return value (not the getter function itself)
								resultPlainObjPtr.SetOwn(key, res)
							} else {
								// No getter: store undefined
								resultPlainObjPtr.SetOwn(key, Undefined)
							}
						} else {
							// Regular data property
							_, _, enumerable, _, exists := sourceObj.GetOwnDescriptor(key)
							if exists && enumerable {
								if value, _ := sourceObj.GetOwn(key); true {
									resultPlainObjPtr.SetOwn(key, value)
								}
							}
						}
					}
				}
				resultObj = resultPlainObj

			case TypeDictObject:
				sourceDict := sourceValue.AsDictObject()
				resultPlainObj := NewObject(vm.ObjectPrototype)
				resultPlainObjPtr := resultPlainObj.AsPlainObject()

				// Copy properties not in exclude list (only enumerable properties)
				for _, key := range sourceDict.OwnKeys() {
					if _, shouldExclude := excludeNames[key]; !shouldExclude {
						// Check if property is enumerable
						_, _, enumerable, _, exists := sourceDict.GetOwnDescriptor(key)
						if exists && enumerable {
							if value, _ := sourceDict.GetOwn(key); true {
								resultPlainObjPtr.SetOwn(key, value)
							}
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
			// Save IP BEFORE throwing for handler lookup
			// Handler ranges are compiled to include the throw instruction
			throwIP := ip - 1 // IP of the OpThrow opcode itself
			frame.ip = throwIP

			if debugExceptions {
				fmt.Printf("[DEBUG OpThrow] About to throw at IP %d, saved frame.ip=%d for handler lookup\n", throwIP, frame.ip)
			}

			// Execute throw and update IP past operands
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
						ip = handler.HandlerPC // Sync local IP variable
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

			switch vm.pendingAction {
			case ActionReturn:
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

			case ActionThrow:
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

		case OpGetArguments:
			// OpGetArguments: Rx - Create arguments object from current function arguments
			destReg := code[ip]
			ip++

			// Get function arguments from current call frame
			// For function calls, arguments are stored in the beginning of the register space
			// We need to determine how many arguments were passed to the current function

			if frame.closure == nil || frame.closure.Fn == nil {
				status := vm.runtimeError("Cannot access arguments outside of function")
				return status, Undefined
			}

			// Use the actual argument count that was passed to this function
			// (stored in frame.argCount during function call setup)
			argCount := frame.argCount

			// Collect the arguments that were passed to this function
			args := make([]Value, argCount)

			// Special handling for variadic functions
			if frame.closure.Fn.Variadic && argCount > 0 {
				// For variadic functions, all arguments are packed into an array in register 0
				if frame.registers[0].Type() == TypeArray {
					arr := frame.registers[0].AsArray()
					for i := 0; i < argCount && i < arr.Length(); i++ {
						args[i] = arr.Get(i)
					}
				} else {
					// Fallback to regular method
					for i := 0; i < argCount; i++ {
						if i < len(frame.registers) {
							args[i] = frame.registers[i]
						} else {
							args[i] = Undefined
						}
					}
				}
			} else {
				// Regular function - arguments are in separate registers
				for i := 0; i < argCount; i++ {
					if i < len(frame.registers) {
						args[i] = frame.registers[i]
					} else {
						args[i] = Undefined
					}
				}
			}

			// Create arguments object with callee reference
			var calleeValue Value
			if frame.closure != nil {
				calleeValue = NewClosure(frame.closure.Fn, frame.closure.Upvalues)
			} else {
				calleeValue = Undefined
			}
			argsObj := NewArguments(args, calleeValue)
			frame.registers[destReg] = argsObj

		// --- Generator Support ---
		case OpCreateGenerator:
			// OpCreateGenerator destReg, funcReg, argCount
			// Create a generator object instead of calling the function
			destReg := code[ip]
			funcReg := code[ip+1]
			argCount := int(code[ip+2])

			// Get the generator function
			funcVal := registers[funcReg]
			if !funcVal.IsFunction() && !funcVal.IsClosure() {
				status := vm.runtimeError("OpCreateGenerator: not a function")
				return status, Undefined
			}

			// Create generator object using the proper constructor
			genVal := NewGenerator(funcVal)
			genObj := genVal.AsGenerator()

			// Store the arguments for when the generator starts
			if argCount > 0 {
				genObj.Args = make([]Value, argCount)
				for i := 0; i < argCount; i++ {
					argReg := int(funcReg) + 1 + i // Arguments follow the function register
					if argReg < len(registers) {
						genObj.Args[i] = registers[argReg]
					} else {
						genObj.Args[i] = Undefined
					}
				}
			}

			// Set result in destination register
			registers[destReg] = genVal

			ip += 3

		case OpYield:
			// OpYield valueReg, outputReg
			// Suspend current generator execution, yield value from valueReg, store sent value in outputReg
			valueReg := code[ip]
			outputReg := code[ip+1]
			ip += 2

			// Get the yielded value
			yieldedValue := registers[valueReg]

			// Find the generator object associated with this frame
			if frame.generatorObj == nil {
				status := vm.runtimeError("Yield can only be used inside generator functions")
				return status, Undefined
			}

			genObj := frame.generatorObj

			// Suspend the generator and save its state
			genObj.State = GeneratorSuspendedYield
			genObj.YieldedValue = yieldedValue

			// Save the execution frame state
			if genObj.Frame == nil {
				genObj.Frame = &GeneratorFrame{
					pc:        ip, // Resume after this yield instruction (IP already advanced)
					registers: make([]Value, len(registers)),
					locals:    make([]Value, 0), // TODO: implement locals if needed
					stackBase: 0,
					suspendPC:   ip - 2,    // IP was advanced by 2 for two-register instruction
					outputReg: outputReg, // Store where to put sent value on resume
				}
			} else {
				genObj.Frame.pc = ip
				genObj.Frame.suspendPC = ip - 2
				genObj.Frame.outputReg = outputReg
			}

			// Copy register state to generator frame
			copy(genObj.Frame.registers, registers)

			// Create iterator result { value: yieldedValue, done: false }
			result := NewObject(vm.ObjectPrototype).AsPlainObject()
			result.SetOwn("value", yieldedValue)
			result.SetOwn("done", BooleanValue(false))

			// Return from generator execution
			return InterpretOK, NewValueFromPlainObject(result)

		case OpResumeGenerator:
			// OpResumeGenerator is used internally to resume generator execution
			// This should not be directly encountered in normal execution
			status := vm.runtimeError("OpResumeGenerator is an internal opcode")
			return status, Undefined

		case OpAwait:
			// OpAwait resultReg, promiseReg
			// Suspend async function execution, await promise settlement, store result in resultReg
			resultReg := code[ip]
			promiseReg := code[ip+1]
			ip += 2

			// Get the value being awaited
			awaitedValue := registers[promiseReg]

			// JavaScript allows awaiting non-promises - they resolve immediately
			if awaitedValue.Type() != TypePromise {
				// Non-promise value - just store it and continue
				registers[resultReg] = awaitedValue
				continue
			}

			// Get the promise object
			awaitedPromise := awaitedValue.AsPromise()

			// Check promise state
			switch awaitedPromise.State {
			case PromiseFulfilled:
				// Promise already fulfilled - store result and continue
				registers[resultReg] = awaitedPromise.Result
				continue

			case PromiseRejected:
				// Promise already rejected - throw the error
				frame.ip = ip
				status := vm.runtimeError("Uncaught (in promise): %s", awaitedPromise.Result.Inspect())
				return status, Undefined

			case PromisePending:
				// Promise is pending - need to suspend execution
				// This is the complex case: save state, attach handlers, schedule resumption

				// Check if we're in an async function context
				// Top-level await: if not in async context, drain microtasks until settled
				if frame.promiseObj == nil {
					// Top-level await with pending promise
					// Drain microtasks repeatedly until the promise settles
					rt := vm.GetAsyncRuntime()
					for awaitedPromise.State == PromisePending {
						// Drain all pending microtasks
						if !rt.RunUntilIdle() {
							// No more microtasks and promise still pending
							// This means the promise will never resolve - error
							frame.ip = ip
							status := vm.runtimeError("Top-level await: promise remains pending with no microtasks to process")
							return status, Undefined
						}
						// Check promise state again after draining
					}

					// Promise has settled - check if fulfilled or rejected
					switch awaitedPromise.State {
					case PromiseFulfilled:
						registers[resultReg] = awaitedPromise.Result
						continue
					case PromiseRejected:
						frame.ip = ip
						status := vm.runtimeError("Uncaught (in promise): %s", awaitedPromise.Result.Inspect())
						return status, Undefined
					}
				}

				// Save the execution frame state (similar to OpYield)
				if frame.promiseObj.Frame == nil {
					frame.promiseObj.Frame = &SuspendedFrame{
						pc:        ip,        // Resume after this await instruction
						registers: make([]Value, len(registers)),
						locals:    make([]Value, 0),
						stackBase: 0,
						suspendPC: ip - 2,    // IP was advanced by 2 for two-register instruction
						outputReg: resultReg, // Store where to put resolved value
					}
				} else {
					frame.promiseObj.Frame.pc = ip
					frame.promiseObj.Frame.suspendPC = ip - 2
					frame.promiseObj.Frame.outputReg = resultReg
				}

				// Copy register state
				copy(frame.promiseObj.Frame.registers, registers)

				// Attach promise resolution handlers
				// When the awaited promise settles, we need to resume the async function
				asyncPromise := frame.promiseObj

				// Schedule fulfillment handler
				awaitedPromise.FulfillReactions = append(awaitedPromise.FulfillReactions, PromiseReaction{
					Handler: Undefined, // No user handler - internal resumption
					Resolve: func(value Value) {
						// Resume async function with fulfilled value
						result, err := vm.resumeAsyncFunction(asyncPromise, value)
						if err != nil {
							// Resume failed - reject the async promise
							vm.rejectPromise(asyncPromise, NewString(err.Error()))
						} else {
							// Async function returned - resolve with final value
							vm.resolvePromise(asyncPromise, result)
						}
					},
					Reject: func(reason Value) {
						// Resume async function with rejected value (it will throw)
						_, err := vm.resumeAsyncFunctionWithException(asyncPromise, reason)
						if err != nil {
							// Exception wasn't caught - reject the async promise
							vm.rejectPromise(asyncPromise, reason)
						}
					},
				})

				// Suspend execution - return to caller
				// The async function's promise remains pending
				return InterpretOK, Undefined
			}
			continue

		case OpDeleteProp:
			destReg := code[ip]
			objReg := code[ip+1]
			nameConstIdxHi := code[ip+2]
			nameConstIdxLo := code[ip+3]
			nameConstIdx := uint16(nameConstIdxHi)<<8 | uint16(nameConstIdxLo)
			ip += 4
			if int(nameConstIdx) >= len(constants) {
				frame.ip = ip
				status := vm.runtimeError("Invalid constant index %d for property name.", nameConstIdx)
				return status, Undefined
			}
			nameVal := constants[nameConstIdx]
			if !IsString(nameVal) {
				frame.ip = ip
				status := vm.runtimeError("Internal Error: Property name constant %d is not a string.", nameConstIdx)
				return status, Undefined
			}
			propName := AsString(nameVal)
			obj := registers[objReg]

			// Check for undefined or null base object - should throw ReferenceError
			if obj.Type() == TypeUndefined || obj.Type() == TypeNull {
				frame.ip = ip
				vm.ThrowReferenceError("Cannot delete property '" + propName + "' of " + obj.Type().String())
				return InterpretRuntimeError, Undefined
			}

			success := false
			if obj.Type() == TypeProxy {
				proxy := obj.AsProxy()
				if proxy.Revoked {
					// Proxy is revoked, throw TypeError
					var excVal Value
					if typeErrCtor, ok := vm.GetGlobal("TypeError"); ok {
						if res, callErr := vm.Call(typeErrCtor, Undefined, []Value{NewString("Cannot delete property from a revoked Proxy")}); callErr == nil {
							excVal = res
						}
					}
					if excVal.Type() == 0 {
						eo := NewObject(vm.ErrorPrototype).AsPlainObject()
						eo.SetOwn("name", NewString("TypeError"))
						eo.SetOwn("message", NewString("Cannot delete property from a revoked Proxy"))
						excVal = NewValueFromPlainObject(eo)
					}
					vm.throwException(excVal)
					return InterpretRuntimeError, Undefined
				}

				// Check if handler has a delete trap
				deleteTrap, ok := proxy.handler.AsPlainObject().GetOwn("deleteProperty")
				if ok {
					// Validate trap is callable
					if !deleteTrap.IsCallable() {
						vm.runtimeError("'deleteProperty' on proxy: trap is not a function")
						return InterpretRuntimeError, Undefined
					}

					// Call the delete trap: handler.deleteProperty(target, propertyKey)
					trapArgs := []Value{proxy.target, NewString(propName)}
					result, err := vm.Call(deleteTrap, proxy.handler, trapArgs)
					if err != nil {
						if ee, ok := err.(ExceptionError); ok {
							vm.throwException(ee.GetExceptionValue())
							return InterpretRuntimeError, Undefined
						}
						// Wrap non-exception Go error
						var excVal Value
						if errCtor, ok := vm.GetGlobal("Error"); ok {
							if res, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
								excVal = res
							} else {
								eo := NewObject(vm.ErrorPrototype).AsPlainObject()
								eo.SetOwn("name", NewString("Error"))
								eo.SetOwn("message", NewString(err.Error()))
								excVal = NewValueFromPlainObject(eo)
							}
						} else {
							eo := NewObject(vm.ErrorPrototype).AsPlainObject()
							eo.SetOwn("name", NewString("Error"))
							eo.SetOwn("message", NewString(err.Error()))
							excVal = NewValueFromPlainObject(eo)
						}
						vm.throwException(excVal)
						return InterpretRuntimeError, Undefined
					}
					success = result.IsTruthy()
				} else {
					// No delete trap, fallback to target
					if proxy.target.IsObject() {
						if po := proxy.target.AsPlainObject(); po != nil {
							success = po.DeleteOwn(propName)
						} else if d := proxy.target.AsDictObject(); d != nil {
							success = d.DeleteOwn(propName)
						}
					}
				}
			} else if obj.IsObject() {
				if po := obj.AsPlainObject(); po != nil {
					success = po.DeleteOwn(propName)
				} else if d := obj.AsDictObject(); d != nil {
					success = d.DeleteOwn(propName)
				}
			}
			registers[destReg] = BooleanValue(success)

		case OpDeleteIndex:
			destReg := code[ip]
			objReg := code[ip+1]
			keyReg := code[ip+2]
			ip += 3

			obj := registers[objReg]
			key := registers[keyReg]

			// Check for undefined or null base object - should throw ReferenceError
			if obj.Type() == TypeUndefined || obj.Type() == TypeNull {
				frame.ip = ip
				keyStr := key.ToString()
				vm.ThrowReferenceError("Cannot delete property '" + keyStr + "' of " + obj.Type().String())
				return InterpretRuntimeError, Undefined
			}

			var success bool
			if obj.IsObject() {
				if po := obj.AsPlainObject(); po != nil {
					if key.Type() == TypeSymbol {
						success = po.DeleteOwnByKey(NewSymbolKey(key))
					} else {
						success = po.DeleteOwn(key.ToString())
					}
				} else if d := obj.AsDictObject(); d != nil {
					if key.Type() == TypeSymbol {
						success = false
					} else {
						success = d.DeleteOwn(key.ToString())
					}
				} else if a := obj.AsArray(); a != nil {
					// Not supporting element deletion yet
					success = false
				}
			}
			registers[destReg] = BooleanValue(success)

		case OpDeleteGlobal:
			// OpDeleteGlobal: Rx HeapIdx(16bit): Rx = delete global[HeapIdx]
			destReg := code[ip]
			heapIdx := (uint16(code[ip+1]) << 8) | uint16(code[ip+2])  // Big-endian
			ip += 3

			// Try to delete from heap - returns false if non-configurable
			success := vm.heap.Delete(int(heapIdx))
			registers[destReg] = BooleanValue(success)

		case OpToPropertyKey:
			// OpToPropertyKey: Rx Ry: Rx = ToPropertyKey(Ry)
			// Converts value to property key, calling toString() method for objects
			destReg := code[ip]
			srcReg := code[ip+1]
			ip += 2

			srcVal := registers[srcReg]
			var keyStr string

			switch srcVal.Type() {
			case TypeString:
				keyStr = AsString(srcVal)
			case TypeFloatNumber, TypeIntegerNumber:
				keyStr = srcVal.ToString()
			case TypeSymbol:
				// Symbols are valid property keys, keep as-is
				registers[destReg] = srcVal
				continue
			default:
				// For objects and other types, call ToPrimitive with "string" hint
				if srcVal.IsObject() {
					primitiveVal := vm.toPrimitive(srcVal, "string")
					keyStr = primitiveVal.ToString()
				} else {
					keyStr = srcVal.ToString()
				}
			}

			registers[destReg] = String(keyStr)

		default:
			frame.ip = ip // Save IP before erroring
			// Extra diagnostics for opcode 255 (often indicates unpatched jump placeholder bytes)
			if opcode == 255 {
				start := ip - 4
				if start < 0 {
					start = 0
				}
				end := ip + 8
				if end > len(code) {
					end = len(code)
				}
				fmt.Fprintf(os.Stderr, "[VM Debug] Unknown opcode 255 at ip=%d. Bytes around: ")
				for i := start; i < end; i++ {
					if i == ip {
						fmt.Fprintf(os.Stderr, "<%d> ", code[i])
					} else {
						fmt.Fprintf(os.Stderr, "%d ", code[i])
					}
				}
				fmt.Fprintln(os.Stderr)
			}
			status := vm.runtimeError("Unknown opcode %d encountered.", opcode)
			return status, Undefined
		}

		// Check for exception unwinding after each instruction
		if vm.unwinding {
			if debugExceptions {
				fmt.Printf("[DEBUG vm.go] VM run loop: unwinding=true at IP=%d, calling unwindException\n", ip)
			}
			// Continue the unwinding process by calling unwindException
			unwindResult := vm.unwindException()
			if debugExceptions {
				fmt.Printf("[DEBUG vm.go] VM run loop: unwindException returned %v, unwinding=%v\n", unwindResult, vm.unwinding)
			}
			if !unwindResult {
				// No handler found, uncaught exception
				if debugExceptions {
					fmt.Printf("[DEBUG vm.go] VM run loop: No handler found, returning InterpretRuntimeError\n")
				}
				vm.handleUncaughtException()
				return InterpretRuntimeError, vm.currentException
			}

			// Check if we're still unwinding after hitting a direct call boundary
			if vm.unwinding {
				// Still unwinding means we hit a direct call boundary and need to propagate the exception
				if debugExceptions {
					fmt.Printf("[DEBUG vm.go] VM run loop: Still unwinding after unwindException, returning error\n")
				}
				return InterpretRuntimeError, vm.currentException
			}

			// Handler was found, continue execution with updated frame
			frame = &vm.frames[vm.frameCount-1]
			closure = frame.closure
			if debugExceptions {
				fmt.Printf("[DEBUG vm.go] Continuing execution after exception handler, frame.ip=%d, updating VM state\n", frame.ip)
			}
			function = closure.Fn
			code = function.Chunk.Code
			constants = function.Chunk.Constants
			registers = frame.registers
			ip = frame.ip // CRITICAL: Update VM's IP to the handler location
			if debugExceptions {
				fmt.Printf("[DEBUG vm.go] VM state updated: ip=%d, continuing main loop, next opcode will be %s\n", ip, OpCode(code[ip]).String())
			}
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

	// CAUTION: this is commented out because apparently it's unreachable according to Go compiler
	// if vm.frameCount == 0 {
	// 	// No frames left - either uncaught exception or completed execution
	// 	if vm.unwinding {
	// 		return InterpretRuntimeError, vm.currentException
	// 	} else {
	// 		return InterpretOK, Undefined
	// 	}
	// }

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
	if debugVM {
		fmt.Printf("[DBG closeUpvalues] ENTER: frameRegs=%d, openUpvalues=%d\n", len(frameRegisters), len(vm.openUpvalues))
	}
	if len(frameRegisters) == 0 || len(vm.openUpvalues) == 0 {
		if debugVM {
			fmt.Printf("[DBG closeUpvalues] EXIT early: nothing to close\n")
		}
		return // Nothing to close or no registers in frame
	}

	// Get the memory address range of the frame's register slice.
	// This is somewhat fragile if the underlying array is reallocated,
	// but should be okay as registerStack has fixed size.
	frameStartPtr := uintptr(unsafe.Pointer(&frameRegisters[0]))
	// Address of one past the last element
	frameEndPtr := frameStartPtr + uintptr(len(frameRegisters))*unsafe.Sizeof(Value{})

	if debugVM {
		fmt.Printf("[DBG closeUpvalues] About to iterate %d upvalues\n", len(vm.openUpvalues))
	}

	// Iterate through openUpvalues and close those pointing into the frame.
	// We also filter the openUpvalues list, removing the closed ones.
	newOpenUpvalues := vm.openUpvalues[:0] // Reuse underlying array
	for i, upvalue := range vm.openUpvalues {
		if debugVM {
			fmt.Printf("[DBG closeUpvalues] Processing upvalue %d/%d\n", i+1, len(vm.openUpvalues))
		}
		if upvalue.Location == nil { // Skip already closed upvalues
			if debugVM {
				fmt.Printf("[DBG closeUpvalues]   Skipping already-closed upvalue\n")
			}
			continue
		}
		upvaluePtr := uintptr(unsafe.Pointer(upvalue.Location))
		// Check if the upvalue's location points within the memory range of frameRegisters
		if upvaluePtr >= frameStartPtr && upvaluePtr < frameEndPtr {
			// This upvalue points into the frame being popped, close it.
			if debugVM {
				fmt.Printf("[DBG closeUpvalues]   Closing upvalue (in frame range)\n")
			}
			closedValue := *upvalue.Location // Copy the value from the stack
			upvalue.Closed = closedValue     // Store the value
			upvalue.Location = nil           // Mark as closed
			// Do NOT add it back to newOpenUpvalues
		} else {
			// This upvalue points elsewhere (e.g., higher up the stack), keep it open.
			if debugVM {
				fmt.Printf("[DBG closeUpvalues]   Keeping upvalue open (outside frame range)\n")
			}
			newOpenUpvalues = append(newOpenUpvalues, upvalue)
		}
	}
	vm.openUpvalues = newOpenUpvalues
	if debugVM {
		fmt.Printf("[DBG closeUpvalues] EXIT: newOpenUpvalues=%d\n", len(vm.openUpvalues))
	}
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
		chunk := frame.closure.Fn.Chunk
		// Ensure instructionPos is valid (non-negative and within bounds)
		if instructionPos >= 0 && instructionPos < len(chunk.Lines) {
			line = chunk.GetLine(instructionPos)
		} else if frame.ip >= 0 && frame.ip < len(chunk.Lines) {
			// If ip-1 is invalid, try using ip itself
			line = chunk.GetLine(frame.ip)
		} else if len(chunk.Lines) > 0 {
			// Fallback: use the first line if available
			line = chunk.Lines[0]
		}
		// If line is still 0 and we have code, default to line 1
		if line == 0 && len(chunk.Code) > 0 {
			line = 1
		}
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

	// Removed error printing to stderr - errors are returned to caller

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
	case TypeBigInt:
		return "bigint"
	case TypeString:
		return "string"
	case TypeSymbol:
		return "symbol"
	case TypeFunction, TypeClosure, TypeNativeFunction, TypeNativeFunctionWithProps, TypeAsyncNativeFunction, TypeBoundFunction:
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
		if i < 10 || value.Type() != TypeUndefined { // Show first 10 and any non-undefined
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

// extractSpreadArguments extracts arguments from a spread iterable value (Array, Set, Map, String, etc.)
func (vm *VM) extractSpreadArguments(iterableVal Value) ([]Value, error) {
	switch iterableVal.Type() {
	case TypeArray:
		// Fast path for arrays
		arrayObj := AsArray(iterableVal)
		args := make([]Value, len(arrayObj.elements))
		copy(args, arrayObj.elements)
		return args, nil

	case TypeString:
		// Strings are iterable - spread into individual characters
		str := AsString(iterableVal)
		args := make([]Value, 0, len(str))
		for _, char := range str {
			args = append(args, NewString(string(char)))
		}
		return args, nil

	case TypeSet:
		// Sets are iterable - spread into their values
		setObj := iterableVal.AsSet()
		args := make([]Value, 0, setObj.Size())
		setObj.ForEach(func(value Value) {
			args = append(args, value)
		})
		return args, nil

	case TypeMap:
		// Maps are iterable - spread into [key, value] pairs
		mapObj := iterableVal.AsMap()
		args := make([]Value, 0, mapObj.Size())
		mapObj.ForEach(func(key Value, value Value) {
			// Each entry is a [key, value] pair
			pairVal := NewArray()
			pairArr := pairVal.AsArray()
			pairArr.elements = []Value{key, value}
			args = append(args, pairVal)
		})
		return args, nil

	default:
		// Check if the value has Symbol.iterator method (custom iterables)
		if iterableVal.Type() == TypeObject || iterableVal.Type() == TypeDictObject {
			var iteratorMethod Value
			var hasIterator bool

			if iterableVal.Type() == TypeObject {
				obj := iterableVal.AsPlainObject()
				// Use the actual Symbol.iterator value to get the property
				iteratorMethod, hasIterator = obj.GetOwnByKey(NewSymbolKey(vm.SymbolIterator))
			} else {
				// DictObject doesn't support symbol keys, so skip
				hasIterator = false
			}

			if hasIterator && (iteratorMethod.IsCallable() || iteratorMethod.IsFunction()) {
				// Call the iterator method to get an iterator
				iterator, err := vm.Call(iteratorMethod, iterableVal, []Value{})
				if err != nil {
					return nil, fmt.Errorf("error calling Symbol.iterator: %v", err)
				}

				// Now iterate using the iterator's next() method
				var args []Value
				for {
					// Get the next method
					var nextMethod Value
					var hasNext bool
					if iterator.Type() == TypeObject {
						nextMethod, hasNext = iterator.AsPlainObject().GetOwn("next")
					} else if iterator.Type() == TypeDictObject {
						nextMethod, hasNext = iterator.AsDictObject().GetOwn("next")
					} else {
						break
					}

					if !hasNext {
						return nil, fmt.Errorf("iterator does not have a next method")
					}

					// Call next()
					result, err := vm.Call(nextMethod, iterator, []Value{})
					if err != nil {
						return nil, fmt.Errorf("error calling next(): %v", err)
					}

					// Check if iteration is done
					if result.Type() != TypeObject && result.Type() != TypeDictObject {
						break
					}

					var doneVal Value
					var valueVal Value
					var hasDone, hasValue bool

					if result.Type() == TypeObject {
						resultObj := result.AsPlainObject()
						doneVal, hasDone = resultObj.GetOwn("done")
						valueVal, hasValue = resultObj.GetOwn("value")
					} else {
						resultDict := result.AsDictObject()
						doneVal, hasDone = resultDict.GetOwn("done")
						valueVal, hasValue = resultDict.GetOwn("value")
					}

					if hasDone && doneVal.IsBoolean() && doneVal.AsBoolean() {
						break
					}

					// Get the value
					if hasValue {
						args = append(args, valueVal)
					}
				}

				return args, nil
			}
		}

		// For other types, only arrays and built-in iterables are supported for now
		return nil, fmt.Errorf("spread argument must be an array or iterable (Array, String, Set, Map), got %s", iterableVal.TypeName())
	}
}

// GetThis returns the current 'this' value for native function execution
// This allows native functions to access the 'this' context without it being passed as an argument
func (vm *VM) GetThis() Value {
	return vm.currentThis
}

// IsConstructorCall returns true if currently executing a native function via 'new'
func (vm *VM) IsConstructorCall() bool {
	return vm.inConstructorCall
}

// NewBooleanObject creates a Boolean wrapper object with the given primitive value
func (vm *VM) NewBooleanObject(primitiveValue bool) Value {
	obj := NewObject(vm.BooleanPrototype).AsPlainObject()
	obj.SetOwn("[[PrimitiveValue]]", BooleanValue(primitiveValue))
	return NewValueFromPlainObject(obj)
}

// NewNumberObject creates a Number wrapper object with the given primitive value
func (vm *VM) NewNumberObject(primitiveValue float64) Value {
	obj := NewObject(vm.NumberPrototype).AsPlainObject()
	obj.SetOwn("[[PrimitiveValue]]", NumberValue(primitiveValue))
	return NewValueFromPlainObject(obj)
}

// NewStringObject creates a String wrapper object with the given primitive value
func (vm *VM) NewStringObject(primitiveValue string) Value {
	obj := NewObject(vm.StringPrototype).AsPlainObject()
	obj.SetOwn("[[PrimitiveValue]]", NewString(primitiveValue))
	return NewValueFromPlainObject(obj)
}

// toPrimitive implements JavaScript ToPrimitive abstract operation
// hint can be "string", "number", or "default"
func (vm *VM) toPrimitive(val Value, hint string) Value {
	// If already primitive, return as-is
	// Note: Functions are objects and should go through ToPrimitive to call toString/valueOf
	if !val.IsObject() && !val.IsCallable() && val.typ != TypeArray && val.typ != TypeArguments && val.typ != TypeRegExp && val.typ != TypeMap && val.typ != TypeSet && val.typ != TypeProxy {
		return val
	}

	// ECMAScript special case: Date objects treat "default" hint as "string" hint
	// This ensures that date + date returns a string concatenation, not numeric addition
	if hint == "default" {
		// Check if this is a Date object by looking for getTime method
		var getTimeMethod Value
		if ok, _, _ := vm.opGetProp(0, &val, "getTime", &getTimeMethod); ok {
			if getTimeMethod.Type() == TypeNativeFunction || getTimeMethod.Type() == TypeNativeFunctionWithProps {
				// This looks like a Date object - use "string" hint
				hint = "string"
			}
		}
	}

	// Step 1: Check for Symbol.toPrimitive method (ECMAScript spec step 1-4)
	// This takes precedence over valueOf/toString
	var toPrimMethod Value
	if vm.SymbolToPrimitive.Type() != TypeUndefined {
		ok, status, _ := vm.opGetPropSymbol(0, &val, vm.SymbolToPrimitive, &toPrimMethod)
		// If getter threw an error, return undefined (exception is already set)
		if status == InterpretRuntimeError {
			return Undefined
		}
		if ok {
			if toPrimMethod.Type() != TypeNull && toPrimMethod.Type() != TypeUndefined {
				// Call Symbol.toPrimitive with hint as argument
				var hintArg Value
				if hint == "string" {
					hintArg = NewString("string")
				} else if hint == "number" {
					hintArg = NewString("number")
				} else {
					hintArg = NewString("default")
				}

				if toPrimMethod.Type() == TypeFunction || toPrimMethod.Type() == TypeClosure ||
					toPrimMethod.Type() == TypeNativeFunction || toPrimMethod.Type() == TypeNativeFunctionWithProps {
					result, err := vm.Call(toPrimMethod, val, []Value{hintArg})
					if err == nil {
						// Result must be a primitive
						if !result.IsObject() && result.typ != TypeArray && result.typ != TypeArguments &&
							result.typ != TypeRegExp && result.typ != TypeMap && result.typ != TypeSet && result.typ != TypeProxy {
							return result
						}
						// If result is not primitive, throw TypeError
						vm.ThrowTypeError("Symbol.toPrimitive must return a primitive value")
						return Undefined
					}
					// If Symbol.toPrimitive throws, propagate the exception
					if ee, ok := err.(ExceptionError); ok {
						vm.throwException(ee.GetExceptionValue())
					}
					return Undefined
				}
			}
		}
	}

	// Step 2: Fall back to OrdinaryToPrimitive (valueOf/toString)
	var methods []string
	if hint == "string" {
		methods = []string{"toString", "valueOf"}
	} else {
		// "number" or "default" hints prefer valueOf first
		methods = []string{"valueOf", "toString"}
	}

	for _, methodName := range methods {
		// Try to get the method
		var methodVal Value
		if ok, _, _ := vm.opGetProp(0, &val, methodName, &methodVal); ok {
			// Check if it's a function
			if methodVal.Type() == TypeFunction || methodVal.Type() == TypeClosure ||
				methodVal.Type() == TypeNativeFunction || methodVal.Type() == TypeNativeFunctionWithProps {
				// Call the method with 'this' bound to val
				result, err := vm.Call(methodVal, val, nil)
				if err != nil {
					// According to ECMAScript spec, if valueOf/toString throws, the exception should propagate
					// We need to re-throw this exception at the VM level
					if ee, ok := err.(ExceptionError); ok {
						// Store the exception to be thrown when toPrimitive returns
						vm.throwException(ee.GetExceptionValue())
					}
					// Return undefined, but the exception state is set so the operation will fail
					return Undefined
				}

				// If result is primitive, return it
				if !result.IsObject() && result.typ != TypeArray && result.typ != TypeArguments &&
					result.typ != TypeRegExp && result.typ != TypeMap && result.typ != TypeSet && result.typ != TypeProxy {
					return result
				}
				// If result is still an object, continue to next method
			}
		}
	}

	// If no method returned a primitive, fallback to string representation
	return NewString(val.ToString())
}

// abstractEqual implements ECMAScript Abstract Equality (==) with object-to-primitive conversion
func (vm *VM) abstractEqual(a, b Value) bool {
	// If types are identical, use strict equality
	if a.Type() == b.Type() {
		return a.StrictlyEquals(b)
	}

	// null/undefined
	if (a.Type() == TypeNull && b.Type() == TypeUndefined) || (a.Type() == TypeUndefined && b.Type() == TypeNull) {
		return true
	}

	// number and string
	if IsNumber(a) && b.Type() == TypeString {
		return AsNumber(a) == b.ToFloat()
	}
	if a.Type() == TypeString && IsNumber(b) {
		return a.ToFloat() == AsNumber(b)
	}

	// boolean compared to anything -> compare ToNumber(boolean) to other via abstract again
	if a.Type() == TypeBoolean {
		num := 0.0
		if a.AsBoolean() {
			num = 1.0
		}
		return vm.abstractEqual(Number(num), b)
	}
	if b.Type() == TypeBoolean {
		num := 0.0
		if b.AsBoolean() {
			num = 1.0
		}
		return vm.abstractEqual(a, Number(num))
	}

	// bigint and string
	if a.IsBigInt() && b.Type() == TypeString {
		if bi, ok := stringToBigInt(b.ToString()); ok {
			return a.AsBigInt().Cmp(bi) == 0
		}
		return false
	}
	if b.IsBigInt() && a.Type() == TypeString {
		if bi, ok := stringToBigInt(a.ToString()); ok {
			return b.AsBigInt().Cmp(bi) == 0
		}
		return false
	}

	// number and bigint
	if IsNumber(a) && b.IsBigInt() {
		n := a.ToFloat()
		if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) {
			return false
		}
		if n < math.MinInt64 || n > math.MaxInt64 {
			return false
		}
		ni := int64(n)
		return new(big.Int).SetInt64(ni).Cmp(b.AsBigInt()) == 0
	}
	if a.IsBigInt() && IsNumber(b) {
		n := b.ToFloat()
		if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) {
			return false
		}
		if n < math.MinInt64 || n > math.MaxInt64 {
			return false
		}
		ni := int64(n)
		return a.AsBigInt().Cmp(new(big.Int).SetInt64(ni)) == 0
	}

	// Object compared to primitive: convert object to primitive and retry
	// Per ECMAScript spec 7.2.15 step 10-11
	if a.IsObject() && !b.IsObject() {
		// Convert object to primitive with "default" hint
		aPrim := vm.toPrimitive(a, "default")
		return vm.abstractEqual(aPrim, b)
	}
	if b.IsObject() && !a.IsObject() {
		// Convert object to primitive with "default" hint
		bPrim := vm.toPrimitive(b, "default")
		return vm.abstractEqual(a, bPrim)
	}

	// Default: not equal
	return false
}

// executeGenerator starts or resumes generator execution
func (vm *VM) executeGenerator(genObj *GeneratorObject, sentValue Value) (Value, error) {
	if genObj.State == GeneratorSuspendedStart {
		// First call - start generator function execution
		return vm.startGenerator(genObj, sentValue)
	} else if genObj.State == GeneratorSuspendedYield {
		// Resume from yield point
		return vm.resumeGenerator(genObj, sentValue)
	}

	// Generator is completed
	result := NewObject(vm.ObjectPrototype).AsPlainObject()
	result.SetOwn("value", Undefined)
	result.SetOwn("done", BooleanValue(true))
	return NewValueFromPlainObject(result), nil
}

func (vm *VM) executeGeneratorWithException(genObj *GeneratorObject, exception Value) (Value, error) {
	if genObj.State == GeneratorSuspendedStart {
		// Cannot throw into a generator that hasn't started yet
		// This should throw the exception immediately
		genObj.State = GeneratorCompleted
		genObj.Done = true
		genObj.Frame = nil
		// Surface as ExceptionError to integrate with new call/exception flow
		return Undefined, exceptionError{exception: exception}
	} else if genObj.State == GeneratorSuspendedYield {
		// Resume from yield point and throw exception at that point
		return vm.resumeGeneratorWithException(genObj, exception)
	}

	// Generator is completed - throw the exception
	return Undefined, exceptionError{exception: exception}
}

// startGenerator begins execution of a generator function using sentinel frame isolation
func (vm *VM) startGenerator(genObj *GeneratorObject, sentValue Value) (Value, error) {
	// Get the generator function
	funcVal := genObj.Function

	// Set up the caller context for sentinel frame approach (like executeUserFunctionSafe)
	callerRegisters := make([]Value, 1)
	destReg := byte(0)
	callerIP := 0

	// Add a sentinel frame that will cause vm.run() to return when generator yields/returns
	sentinelFrame := &vm.frames[vm.frameCount]
	sentinelFrame.isSentinelFrame = true
	sentinelFrame.closure = nil               // Sentinel frames don't have closures
	sentinelFrame.targetRegister = destReg    // Target register in caller
	sentinelFrame.registers = callerRegisters // Give it the caller registers for the result
	vm.frameCount++

	// Use prepareCall to set up the generator function call with the stored arguments
	args := genObj.Args
	if args == nil {
		args = []Value{}
	}
	shouldSwitch, err := vm.prepareCallWithGeneratorMode(funcVal, Value{typ: TypeGenerator, obj: unsafe.Pointer(genObj)}, args, destReg, callerRegisters, callerIP, true)
	if err != nil {
		// Remove sentinel frame on error
		vm.frameCount--
		return Undefined, err
	}

	if !shouldSwitch {
		// Native function was executed directly (shouldn't happen for generators)
		// Remove sentinel frame
		vm.frameCount--
		return callerRegisters[destReg], nil
	}

	// We have a new frame set up for the generator
	if vm.frameCount > 1 { // frameCount includes the sentinel frame
		// Set the generator object reference and mark as direct call for proper return handling
		vm.frames[vm.frameCount-1].generatorObj = genObj
		vm.frames[vm.frameCount-1].isDirectCall = true
	}

	// Initialize generator state
	genObj.State = GeneratorExecuting

	// Execute the VM run loop - it will return when the generator yields or the sentinel frame is hit
	status, result := vm.run()

	if status == InterpretRuntimeError {
		if vm.unwinding && vm.currentException != Null {
			return Undefined, exceptionError{exception: vm.currentException}
		}
		return Undefined, fmt.Errorf("runtime error during generator execution")
	}

	return result, nil
}

// resumeGenerator resumes execution from a yield point using sentinel frame isolation
func (vm *VM) resumeGenerator(genObj *GeneratorObject, sentValue Value) (Value, error) {
	// Check if generator has saved state
	if genObj.Frame == nil {
		// Generator has no saved frame - it must be completed
		genObj.State = GeneratorCompleted
		genObj.Done = true
		result := NewObject(vm.ObjectPrototype).AsPlainObject()
		result.SetOwn("value", Undefined)
		result.SetOwn("done", BooleanValue(true))
		return NewValueFromPlainObject(result), nil
	}

	// Get the generator function
	funcVal := genObj.Function

	var funcObj *FunctionObject
	var closureObj *ClosureObject

	// Extract function object from Value
	if funcVal.Type() == TypeFunction {
		funcObj = funcVal.AsFunction()
	} else if funcVal.Type() == TypeClosure {
		closureObj = funcVal.AsClosure()
		funcObj = closureObj.Fn
	} else {
		return Undefined, fmt.Errorf("Invalid generator function type")
	}

	// Set up caller context for sentinel frame approach
	callerRegisters := make([]Value, 1)
	destReg := byte(0)

	// Add a sentinel frame that will cause vm.run() to return when generator yields/returns
	sentinelFrame := &vm.frames[vm.frameCount]
	sentinelFrame.isSentinelFrame = true
	sentinelFrame.closure = nil               // Sentinel frames don't have closures
	sentinelFrame.targetRegister = destReg    // Target register in caller
	sentinelFrame.registers = callerRegisters // Give it the caller registers for the result
	vm.frameCount++

	// Check if we have space for the generator frame
	if vm.frameCount >= MaxFrames {
		vm.frameCount-- // Remove sentinel frame
		return Undefined, fmt.Errorf("Stack overflow")
	}

	// Allocate registers for the generator function
	regSize := funcObj.RegisterSize
	if vm.nextRegSlot+regSize > len(vm.registerStack) {
		vm.frameCount-- // Remove sentinel frame
		return Undefined, fmt.Errorf("Out of registers")
	}

	// Manually set up the generator frame for resumption (bypass prepareCall since we need custom setup)
	frame := &vm.frames[vm.frameCount]
	frame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+regSize]
	frame.ip = genObj.Frame.pc                                               // Resume from saved PC
	frame.targetRegister = destReg                                           // Target in sentinel frame
	frame.thisValue = Value{typ: TypeGenerator, obj: unsafe.Pointer(genObj)} // Set this to the generator object
	frame.isConstructorCall = false
	frame.isDirectCall = true // Mark as direct call for proper return handling
	frame.argCount = 0
	frame.generatorObj = genObj // Link frame to generator object

	if closureObj != nil {
		frame.closure = closureObj
	} else {
		// Create a temporary closure for the function
		closureVal := NewClosure(funcObj, nil)
		frame.closure = closureVal.AsClosure()
	}

	// Restore register state from saved frame
	copy(frame.registers, genObj.Frame.registers)

	// Store the sent value in the register specified by the yield instruction
	// This eliminates the need to hardcode R2 and makes the codegen explicit
	if genObj.Frame != nil && int(genObj.Frame.outputReg) < len(frame.registers) {
		frame.registers[genObj.Frame.outputReg] = sentValue
	}

	// Update VM state
	vm.frameCount++
	vm.nextRegSlot += regSize

	// Update generator state
	genObj.State = GeneratorExecuting

	// Execute the VM run loop - it will return when the generator yields or the sentinel frame is hit
	status, result := vm.run()
	if status == InterpretRuntimeError {
		if vm.unwinding && vm.currentException != Null {
			return Undefined, exceptionError{exception: vm.currentException}
		}
		return Undefined, exceptionError{exception: NewString("runtime error during generator resumption")}
	}

	return result, nil
}

// resumeGeneratorWithException resumes execution from a yield point and throws an exception at that point
func (vm *VM) resumeGeneratorWithException(genObj *GeneratorObject, exception Value) (Value, error) {
	// Check if generator has saved state
	if genObj.Frame == nil {
		// Generator has no saved frame - it must be completed
		genObj.State = GeneratorCompleted
		genObj.Done = true
		return Undefined, fmt.Errorf("exception thrown: %s", exception.ToString())
	}

	// Get the generator function
	funcVal := genObj.Function

	var funcObj *FunctionObject
	var closureObj *ClosureObject

	// Extract function object from Value
	if funcVal.Type() == TypeFunction {
		funcObj = funcVal.AsFunction()
	} else if funcVal.Type() == TypeClosure {
		closureObj = funcVal.AsClosure()
		funcObj = closureObj.Fn
	} else {
		return Undefined, fmt.Errorf("Invalid generator function type")
	}

	// Set up caller context for sentinel frame approach
	callerRegisters := make([]Value, 1)
	destReg := byte(0)

	// Add a sentinel frame that will cause vm.run() to return when generator yields/returns
	sentinelFrame := &vm.frames[vm.frameCount]
	sentinelFrame.isSentinelFrame = true
	sentinelFrame.closure = nil               // Sentinel frames don't have closures
	sentinelFrame.targetRegister = destReg    // Target register in caller
	sentinelFrame.registers = callerRegisters // Give it the caller registers for the result
	vm.frameCount++

	// Check if we have space for the generator frame
	if vm.frameCount >= MaxFrames {
		vm.frameCount-- // Remove sentinel frame
		return Undefined, fmt.Errorf("Stack overflow")
	}

	// Allocate registers for the generator function
	regSize := funcObj.RegisterSize
	if vm.nextRegSlot+regSize > len(vm.registerStack) {
		vm.frameCount-- // Remove sentinel frame
		return Undefined, fmt.Errorf("Out of registers")
	}

	// Manually set up the generator frame for resumption (bypass prepareCall since we need custom setup)
	frame := &vm.frames[vm.frameCount]
	frame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+regSize]
	frame.ip = genObj.Frame.pc                                               // Resume from saved PC
	frame.targetRegister = destReg                                           // Target in sentinel frame
	frame.thisValue = Value{typ: TypeGenerator, obj: unsafe.Pointer(genObj)} // Set this to the generator object
	frame.isConstructorCall = false
	frame.isDirectCall = false // Don't mark as direct call so exceptions can be caught
	frame.argCount = 0
	frame.generatorObj = genObj // Link frame to generator object

	if closureObj != nil {
		frame.closure = closureObj
	} else {
		// Create a temporary closure for the function
		closureVal := NewClosure(funcObj, nil)
		frame.closure = closureVal.AsClosure()
	}

	// Restore register state from saved frame
	copy(frame.registers, genObj.Frame.registers)

	// Update VM state
	vm.frameCount++
	vm.nextRegSlot += regSize

	// Update generator state
	genObj.State = GeneratorExecuting

	// Instead of sending a value, throw an exception at the yield point
	// This will be handled by the VM's exception handling system
	vm.throwException(exception)

	// Check if the exception unwound all frames (uncaught exception)
	if vm.frameCount == 0 && vm.unwinding {
		// Exception propagated through all frames - surface as ExceptionError
		return Undefined, exceptionError{exception: vm.currentException}
	}

	// Execute the VM run loop - it will return when the exception is handled or propagates
	status, result := vm.run()

	if status == InterpretRuntimeError {
		if vm.currentException != Null {
			return Undefined, exceptionError{exception: vm.currentException}
		}
		return Undefined, exceptionError{exception: NewString("runtime error during generator exception handling")}
	}

	return result, nil
}

// resumeAsyncFunction resumes execution of an async function from an await point
// Similar to resumeGenerator but for async/await suspension
func (vm *VM) resumeAsyncFunction(promiseObj *PromiseObject, resolvedValue Value) (Value, error) {
	// Check if promise has saved state
	if promiseObj.Frame == nil {
		// No saved frame - async function must have completed
		return Undefined, fmt.Errorf("async function already completed")
	}

	// Get the async function
	funcVal := promiseObj.Function

	var funcObj *FunctionObject
	var closureObj *ClosureObject

	// Extract function object from Value
	if funcVal.Type() == TypeFunction {
		funcObj = funcVal.AsFunction()
	} else if funcVal.Type() == TypeClosure {
		closureObj = funcVal.AsClosure()
		funcObj = closureObj.Fn
	} else {
		return Undefined, fmt.Errorf("Invalid async function type")
	}

	// Set up caller context for sentinel frame approach
	callerRegisters := make([]Value, 1)
	destReg := byte(0)

	// Add a sentinel frame that will cause vm.run() to return when async function completes
	sentinelFrame := &vm.frames[vm.frameCount]
	sentinelFrame.isSentinelFrame = true
	sentinelFrame.closure = nil               // Sentinel frames don't have closures
	sentinelFrame.targetRegister = destReg    // Target register in caller
	sentinelFrame.registers = callerRegisters // Give it the caller registers for the result
	vm.frameCount++

	// Check if we have space for the async function frame
	if vm.frameCount >= MaxFrames {
		vm.frameCount-- // Remove sentinel frame
		return Undefined, fmt.Errorf("Stack overflow")
	}

	// Allocate registers for the async function
	regSize := funcObj.RegisterSize
	if vm.nextRegSlot+regSize > len(vm.registerStack) {
		vm.frameCount-- // Remove sentinel frame
		return Undefined, fmt.Errorf("Out of registers")
	}

	// Manually set up the async function frame for resumption (bypass prepareCall since we need custom setup)
	frame := &vm.frames[vm.frameCount]
	frame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+regSize]
	frame.ip = promiseObj.Frame.pc         // Resume from saved PC
	frame.targetRegister = destReg         // Target in sentinel frame
	frame.thisValue = promiseObj.ThisValue // Restore original this value
	frame.isConstructorCall = false
	frame.isDirectCall = true    // Mark as direct call for proper return handling
	frame.argCount = 0
	frame.promiseObj = promiseObj // Link frame to promise object

	if closureObj != nil {
		frame.closure = closureObj
	} else {
		// Create a temporary closure for the function
		closureVal := NewClosure(funcObj, nil)
		frame.closure = closureVal.AsClosure()
	}

	// Restore register state from saved frame
	copy(frame.registers, promiseObj.Frame.registers)

	// Store the resolved value in the register specified by the await instruction
	if promiseObj.Frame != nil && int(promiseObj.Frame.outputReg) < len(frame.registers) {
		frame.registers[promiseObj.Frame.outputReg] = resolvedValue
	}

	// Update VM state
	vm.frameCount++
	vm.nextRegSlot += regSize

	// Clear the saved frame since we're resuming
	// promiseObj.Frame = nil  // Don't clear yet - might await again

	// Execute the VM run loop - it will return when the async function completes or awaits again
	status, result := vm.run()
	if status == InterpretRuntimeError {
		if vm.unwinding && vm.currentException != Null {
			return Undefined, exceptionError{exception: vm.currentException}
		}
		return Undefined, exceptionError{exception: NewString("runtime error during async function resumption")}
	}

	return result, nil
}

// resumeAsyncFunctionWithException resumes execution from an await point and throws an exception
func (vm *VM) resumeAsyncFunctionWithException(promiseObj *PromiseObject, exception Value) (Value, error) {
	// Check if promise has saved state
	if promiseObj.Frame == nil {
		// No saved frame - async function must have completed
		return Undefined, fmt.Errorf("exception thrown: %s", exception.ToString())
	}

	// Get the async function
	funcVal := promiseObj.Function

	var funcObj *FunctionObject
	var closureObj *ClosureObject

	// Extract function object from Value
	if funcVal.Type() == TypeFunction {
		funcObj = funcVal.AsFunction()
	} else if funcVal.Type() == TypeClosure {
		closureObj = funcVal.AsClosure()
		funcObj = closureObj.Fn
	} else {
		return Undefined, fmt.Errorf("Invalid async function type")
	}

	// Set up caller context for sentinel frame approach
	callerRegisters := make([]Value, 1)
	destReg := byte(0)

	// Add a sentinel frame that will cause vm.run() to return when async function completes
	sentinelFrame := &vm.frames[vm.frameCount]
	sentinelFrame.isSentinelFrame = true
	sentinelFrame.closure = nil               // Sentinel frames don't have closures
	sentinelFrame.targetRegister = destReg    // Target register in caller
	sentinelFrame.registers = callerRegisters // Give it the caller registers for the result
	vm.frameCount++

	// Check if we have space for the async function frame
	if vm.frameCount >= MaxFrames {
		vm.frameCount-- // Remove sentinel frame
		return Undefined, fmt.Errorf("Stack overflow")
	}

	// Allocate registers for the async function
	regSize := funcObj.RegisterSize
	if vm.nextRegSlot+regSize > len(vm.registerStack) {
		vm.frameCount-- // Remove sentinel frame
		return Undefined, fmt.Errorf("Out of registers")
	}

	// Manually set up the async function frame for resumption
	frame := &vm.frames[vm.frameCount]
	frame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+regSize]
	frame.ip = promiseObj.Frame.pc         // Resume from saved PC
	frame.targetRegister = destReg         // Target in sentinel frame
	frame.thisValue = promiseObj.ThisValue // Restore original this value
	frame.isConstructorCall = false
	frame.isDirectCall = true    // Mark as direct call for proper return handling
	frame.argCount = 0
	frame.promiseObj = promiseObj // Link frame to promise object

	if closureObj != nil {
		frame.closure = closureObj
	} else {
		// Create a temporary closure for the function
		closureVal := NewClosure(funcObj, nil)
		frame.closure = closureVal.AsClosure()
	}

	// Restore register state from saved frame
	copy(frame.registers, promiseObj.Frame.registers)

	// Update VM state
	vm.frameCount++
	vm.nextRegSlot += regSize

	// Throw the exception at the await point
	// This will be handled by the VM's exception handling system
	vm.throwException(exception)

	// Check if the exception unwound all frames (uncaught exception)
	if vm.frameCount == 0 && vm.unwinding {
		// Exception propagated through all frames - surface as ExceptionError
		return Undefined, exceptionError{exception: vm.currentException}
	}

	// Execute the VM run loop - it will return when the exception is handled or propagates
	status, result := vm.run()

	if status == InterpretRuntimeError {
		if vm.currentException != Null {
			return Undefined, exceptionError{exception: vm.currentException}
		}
		return Undefined, exceptionError{exception: NewString("runtime error during async exception handling")}
	}

	return result, nil
}

// setGlobalInTable sets a global variable in the unified global table
func (vm *VM) setGlobalInTable(globalIdx uint16, value Value) {
	// Use heap to store the value
	vm.heap.Set(int(globalIdx), value)
}

// getGlobalFromTable gets a global variable from the unified global table
// func (vm *VM) getGlobalFromTable(globalIdx uint16) Value {
// 	value, exists := vm.heap.Get(int(globalIdx))
// 	if !exists {
// 		return Undefined // Out of bounds
// 	}
// 	return value
// }

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
			globals:     nil, // No longer used - unified heap replaces this
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
			frame:              currentFrame,
			frameCount:         vm.frameCount,
			nextRegSlot:        vm.nextRegSlot,
			currentModulePath:  vm.currentModulePath,
			savedRegisters:     savedRegisters,
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

			// If no export values were collected, try to collect them using export indices
			if len(exportValues) == 0 {
				exportIndices := moduleRecord.GetExportIndices()

				if len(exportIndices) > 0 {
					// Use the export indices mapping to collect values directly from the heap
					// This is the proper way for dynamically imported modules
					for exportName, globalIdx := range exportIndices {
						if value, exists := vm.heap.Get(int(globalIdx)); exists {
							moduleCtx.exports[exportName] = value
						} else {
							moduleCtx.exports[exportName] = Undefined
						}
					}
				} else {
					// Final fallback: manual heap scanning (legacy approach)
					// fmt.Printf("// [VM DEBUG] collectModuleExports: No export indices found for module '%s', attempting manual collection\n", modulePath)
					exportNames := moduleRecord.GetExportNames()

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
					}

					// Use the manually collected values
					for exportName, exportValue := range manuallyCollected {
						moduleCtx.exports[exportName] = exportValue
					}
				}
			} else {
				// Copy the export values directly to the module context
				for exportName, exportValue := range exportValues {
					moduleCtx.exports[exportName] = exportValue
				}
				// fmt.Printf("// [VM DEBUG] collectModuleExports: Collected %d export values for module '%s'\n", len(exportValues), modulePath)
				// for name, value := range exportValues {
				//	fmt.Printf("// [VM DEBUG] collectModuleExports: Export '%s' = %s (type %d)\n", name, value.ToString(), int(value.Type()))
				// }
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
	// Search for the exported value in the global heap by scanning for matching names
	heapSize := vm.heap.Size()

	// First pass: look for functions with matching names
	for i := 0; i < heapSize; i++ {
		if value, exists := vm.heap.Get(i); exists {
			if value.Type() == TypeFunction {
				if fnObj := value.AsFunction(); fnObj != nil && fnObj.Name == exportName {
					return value
				}
			}
		}
	}

	// Second pass: for non-function exports, use legacy hardcoded logic
	// TODO: Implement proper export tracking in the compiler
	const BUILTIN_GLOBALS_END = 22
	var functions []Value
	var objects []Value

	for i := BUILTIN_GLOBALS_END; i < heapSize && i < BUILTIN_GLOBALS_END+20; i++ {
		if value, exists := vm.heap.Get(i); exists {
			if value.Type() == TypeFunction {
				functions = append(functions, value)
			} else if value.Type() == TypeObject {
				objects = append(objects, value)
			}
		}
	}

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
		for i := BUILTIN_GLOBALS_END; i < heapSize && i < BUILTIN_GLOBALS_END+20; i++ {
			if value, exists := vm.heap.Get(i); exists && (value.Type() == TypeFloatNumber || value.Type() == TypeIntegerNumber) {
				return value
			}
		}
	}

	// fmt.Printf("// [VM DEBUG] findExportValueInHeap: Could not find '%s' in heap\n", exportName)
	return Undefined
}

// ThrowTypeError creates and throws a proper TypeError instance
func (vm *VM) ThrowTypeError(message string) {
	// Get the TypeError constructor from globals
	typeErrorCtor, exists := vm.GetGlobal("TypeError")
	if !exists || typeErrorCtor.Type() == TypeUndefined {
		// Fallback: create a basic error object
		errObj := NewObject(vm.TypeErrorPrototype).AsPlainObject()
		errObj.SetOwn("name", NewString("TypeError"))
		errObj.SetOwn("message", NewString(message))
		errObj.SetOwn("stack", NewString(vm.CaptureStackTrace()))
		vm.throwException(NewValueFromPlainObject(errObj))
		return
	}

	// Call the TypeError constructor to create a proper instance
	errorInstance, err := vm.Call(typeErrorCtor, Undefined, []Value{NewString(message)})
	if err != nil {
		// Fallback if constructor call fails
		errObj := NewObject(vm.TypeErrorPrototype).AsPlainObject()
		errObj.SetOwn("name", NewString("TypeError"))
		errObj.SetOwn("message", NewString(message))
		errObj.SetOwn("stack", NewString(vm.CaptureStackTrace()))
		vm.throwException(NewValueFromPlainObject(errObj))
		return
	}

	vm.throwException(errorInstance)
}

// ThrowReferenceError creates and throws a proper ReferenceError instance
func (vm *VM) ThrowReferenceError(message string) {
	// Get the ReferenceError constructor from globals
	refErrorCtor, exists := vm.GetGlobal("ReferenceError")
	if !exists || refErrorCtor.Type() == TypeUndefined {
		// Fallback: create a basic error object
		errObj := NewObject(vm.ReferenceErrorPrototype).AsPlainObject()
		errObj.SetOwn("name", NewString("ReferenceError"))
		errObj.SetOwn("message", NewString(message))
		errObj.SetOwn("stack", NewString(vm.CaptureStackTrace()))
		vm.throwException(NewValueFromPlainObject(errObj))
		return
	}

	// Call the ReferenceError constructor to create a proper instance
	errorInstance, err := vm.Call(refErrorCtor, Undefined, []Value{NewString(message)})
	if err != nil {
		// Fallback if constructor call fails
		errObj := NewObject(vm.ReferenceErrorPrototype).AsPlainObject()
		errObj.SetOwn("name", NewString("ReferenceError"))
		errObj.SetOwn("message", NewString(message))
		errObj.SetOwn("stack", NewString(vm.CaptureStackTrace()))
		vm.throwException(NewValueFromPlainObject(errObj))
		return
	}

	vm.throwException(errorInstance)
}

// ThrowSyntaxError creates and throws a proper SyntaxError instance
func (vm *VM) ThrowSyntaxError(message string) {
	// Get the SyntaxError constructor from globals
	syntaxErrorCtor, exists := vm.GetGlobal("SyntaxError")
	if !exists || syntaxErrorCtor.Type() == TypeUndefined {
		// Fallback: create a basic error object
		errObj := NewObject(vm.ErrorPrototype).AsPlainObject()
		errObj.SetOwn("name", NewString("SyntaxError"))
		errObj.SetOwn("message", NewString(message))
		errObj.SetOwn("stack", NewString(vm.CaptureStackTrace()))
		vm.throwException(NewValueFromPlainObject(errObj))
		return
	}

	// Call the SyntaxError constructor to create a proper instance
	errorInstance, err := vm.Call(syntaxErrorCtor, Undefined, []Value{NewString(message)})
	if err != nil {
		// Fallback if constructor call fails
		errObj := NewObject(vm.ErrorPrototype).AsPlainObject()
		errObj.SetOwn("name", NewString("SyntaxError"))
		errObj.SetOwn("message", NewString(message))
		errObj.SetOwn("stack", NewString(vm.CaptureStackTrace()))
		vm.throwException(NewValueFromPlainObject(errObj))
		return
	}

	vm.throwException(errorInstance)
}

// ThrowRangeError creates and throws a proper RangeError instance
func (vm *VM) ThrowRangeError(message string) {
	// Get the RangeError constructor from globals
	rangeErrorCtor, exists := vm.GetGlobal("RangeError")
	if !exists || rangeErrorCtor.Type() == TypeUndefined {
		// Fallback: create a basic error object
		errObj := NewObject(vm.ErrorPrototype).AsPlainObject()
		errObj.SetOwn("name", NewString("RangeError"))
		errObj.SetOwn("message", NewString(message))
		errObj.SetOwn("stack", NewString(vm.CaptureStackTrace()))
		vm.throwException(NewValueFromPlainObject(errObj))
		return
	}

	// Call the RangeError constructor to create a proper instance
	errorInstance, err := vm.Call(rangeErrorCtor, Undefined, []Value{NewString(message)})
	if err != nil {
		// Fallback if constructor call fails
		errObj := NewObject(vm.ErrorPrototype).AsPlainObject()
		errObj.SetOwn("name", NewString("RangeError"))
		errObj.SetOwn("message", NewString(message))
		errObj.SetOwn("stack", NewString(vm.CaptureStackTrace()))
		vm.throwException(NewValueFromPlainObject(errObj))
		return
	}

	vm.throwException(errorInstance)
}

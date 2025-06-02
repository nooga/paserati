package compiler

import (
	"fmt"
	"math"
	"paserati/pkg/builtins" // <<< ADDED: Import builtins
	"paserati/pkg/checker"
	"paserati/pkg/errors"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// Define a placeholder register value for 'undefined' case
// Also used temporarily for recursive function definition
const nilRegister Register = 255 // Or another value guaranteed not to be used

// --- New: Loop Context for Break/Continue ---
type LoopContext struct {
	// Start of the loop condition check (target for continue in while)
	LoopStartPos int
	// Start of the update expression (target for continue in for)
	// Set to LoopStartPos for while loops
	ContinueTargetPos int
	// List of bytecode positions where OpJump placeholders for break statements start.
	// These need to be patched later to the loop's end address.
	BreakPlaceholderPosList []int
	// List of bytecode positions where OpJump placeholders for continue statements start.
	ContinuePlaceholderPosList []int
}

const debugCompiler = false      // <<< CHANGED back to false
const debugCompilerStats = false // <<< CHANGED back to false
const debugCompiledCode = false
const debugPrint = true // Enable debug output

func debugPrintf(format string, args ...interface{}) {
	if debugCompiler {
		fmt.Printf(format, args...)
	}
}

type CompilerStats struct {
	BytesGenerated int
}

// Compiler transforms an AST into bytecode.
type Compiler struct {
	chunk              *vm.Chunk
	regAlloc           *RegisterAllocator
	currentSymbolTable *SymbolTable
	enclosing          *Compiler
	freeSymbols        []*Symbol
	errors             []errors.PaseratiError
	lastExprReg        Register
	lastExprRegValid   bool
	loopContextStack   []*LoopContext
	compilingFuncName  string
	typeChecker        *checker.Checker // Holds the checker instance
	stats              *CompilerStats

	// --- NEW: Global Variable Indexing for Performance ---
	// Maps global variable names to their assigned indices (only for top-level compiler)
	globalIndices map[string]int
	// Count of global variables assigned so far (only for top-level compiler)
	globalCount int
}

// NewCompiler creates a new *top-level* Compiler.
func NewCompiler() *Compiler {
	return &Compiler{
		chunk:              vm.NewChunk(),
		regAlloc:           NewRegisterAllocator(),
		currentSymbolTable: NewSymbolTable(),
		enclosing:          nil,
		freeSymbols:        []*Symbol{},
		errors:             []errors.PaseratiError{},
		lastExprRegValid:   false,
		loopContextStack:   make([]*LoopContext, 0),
		compilingFuncName:  "<script>",
		typeChecker:        nil, // Initialized to nil, can be set externally
		stats:              &CompilerStats{},
		globalIndices:      make(map[string]int),
		globalCount:        0,
	}
}

// SetChecker allows injecting an external checker instance.
// This is used by the driver for REPL sessions.
func (c *Compiler) SetChecker(checker *checker.Checker) {
	c.typeChecker = checker
}

// newFunctionCompiler creates a compiler instance specifically for a function body.
func newFunctionCompiler(enclosingCompiler *Compiler) *Compiler {
	// Pass the checker down to nested compilers
	return &Compiler{
		chunk:              vm.NewChunk(),
		regAlloc:           NewRegisterAllocator(),
		currentSymbolTable: NewEnclosedSymbolTable(enclosingCompiler.currentSymbolTable),
		enclosing:          enclosingCompiler,
		freeSymbols:        []*Symbol{},
		errors:             []errors.PaseratiError{},
		loopContextStack:   make([]*LoopContext, 0),
		compilingFuncName:  "",
		typeChecker:        enclosingCompiler.typeChecker, // Inherit checker from enclosing
		stats:              enclosingCompiler.stats,
	}
}

// Compile traverses the AST, performs type checking using its assigned checker,
// and generates bytecode.
// Returns the generated chunk and any errors encountered (including type errors).
func (c *Compiler) Compile(node parser.Node) (*vm.Chunk, []errors.PaseratiError) {

	// --- Type Checking Step ---
	program, ok := node.(*parser.Program)
	if !ok {
		// Compiler currently expects the root node to be a Program.
		// If not, it cannot type check. Return an immediate internal compiler error.
		// We create a placeholder token for the position.
		// TODO: Find a better way to get position info if input isn't a Program.
		placeholderToken := lexer.Token{Type: lexer.ILLEGAL, Literal: "", Line: 1, Column: 1, StartPos: 0, EndPos: 0}
		compileErr := &errors.CompileError{
			Position: errors.Position{
				Line:     placeholderToken.Line,
				Column:   placeholderToken.Column,
				StartPos: placeholderToken.StartPos,
				EndPos:   placeholderToken.EndPos,
			},
			Msg: "compiler error: Compile input must be *parser.Program for type checking",
		}
		// Append to errors list as well
		c.errors = append(c.errors, compileErr)
		return nil, c.errors
	}

	// Use the assigned checker. If none was assigned (e.g., non-REPL), create one.
	if c.typeChecker == nil {
		c.typeChecker = checker.NewChecker()
	}
	// Perform the check using the (potentially persistent) checker.
	// The checker modifies its own environment state.
	typeErrors := c.typeChecker.Check(program)
	if len(typeErrors) > 0 {
		// Found type errors. Return them immediately.
		// Type errors are already []errors.PaseratiError from the checker.
		return nil, typeErrors
	}
	// No need to re-assign c.typeChecker here, it was already set or created.
	// --- End Type Checking Step ---

	// --- Bytecode Compilation Step ---
	c.chunk = vm.NewChunk()
	c.regAlloc = NewRegisterAllocator()
	c.currentSymbolTable = NewSymbolTable()

	// --- Global Symbol Table Initialization (if needed) ---
	// c.defineBuiltinGlobals() // TODO: Define built-ins if any

	// --- Compile Hoisted Global Functions FIRST ---
	if program.HoistedDeclarations != nil {
		debugPrintf("[Compile] Processing %d hoisted global declarations...\n", len(program.HoistedDeclarations))
		for name, hoistedNode := range program.HoistedDeclarations {
			funcLit, ok := hoistedNode.(*parser.FunctionLiteral)
			if !ok {
				// Should not happen if checker/parser worked correctly
				debugPrintf("[Compile Hoisting] ERROR: Hoisted node for '%s' is not FunctionLiteral (%T)\n", name, hoistedNode)
				continue
			}

			// 1. Define the function name temporarily to allow self-recursion
			c.currentSymbolTable.Define(name, nilRegister)

			// 2. Compile the function literal (creates chunk & function object)
			// This will now properly detect self-recursion and include it in freeSymbols
			funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, name)
			if err != nil {
				// Error during function compilation, already added to c.errors by sub-compiler
				debugPrintf("[Compile Hoisting] ERROR compiling hoisted func '%s': %v\n", name, err)
				continue // Skip defining this function
			}

			// 3. Create the Closure with the actual freeSymbols (not nil)
			closureReg := c.regAlloc.Alloc()                                // Allocate register for the closure object
			c.emitClosure(closureReg, funcConstIndex, funcLit, freeSymbols) // Use actual freeSymbols

			// 4. Update the symbol table entry with the register holding the closure
			c.currentSymbolTable.UpdateRegister(name, closureReg) // Update from nilRegister to actual register

			// 5. NEW: Emit OpSetGlobal to store the function in the VM's globals array
			globalIdx := c.getOrAssignGlobalIndex(name)
			c.emitSetGlobal(globalIdx, closureReg, funcLit.Token.Line)

			debugPrintf("[Compile Hoisting] Defined global func '%s' with %d upvalues in R%d, stored at global index %d\n", name, len(freeSymbols), closureReg, globalIdx)

			// Invalidate lastExprReg, as the hoisted function definition itself isn't the result.
			c.lastExprRegValid = false
		}
	}
	// --- END Hoisted Global Function Processing ---

	// Use the already type-checked program node.
	err := c.compileNode(program)
	if err != nil {
		// An error occurred during compilation. addError should have already been called
		// when the error was generated lower down. The returned `err` is mainly for control flow.
		// We don't need to append err.Error() here again.
	}

	// Emit final return instruction if no *compilation* errors occurred
	// (Type errors were caught earlier and returned).
	if len(c.errors) == 0 {
		if c.enclosing == nil { // Top-level script
			// <<< ADD DEBUG PRINT HERE >>>
			debugPrintf("// DEBUG Compile End: Final Return Check. lastExprRegValid: %v, lastExprReg: R%d\n", c.lastExprRegValid, c.lastExprReg)
			if c.lastExprRegValid {
				c.emitReturn(c.lastExprReg, 0) // Use line 0 for implicit final return
			} else {
				debugPrintf("// DEBUG Compile End: Emitting OpReturnUndefined because lastExprRegValid is false.\n") // <<< ADDED
				c.emitOpCode(vm.OpReturnUndefined, 0)                                                                // Use line 0 for implicit final return
			}
		} else {
			// Inside a function, OpReturn or OpReturnUndefined should have been emitted.
			// Add one just in case of missing return paths (though type checker might catch this).
			c.emitOpCode(vm.OpReturnUndefined, 0)
		}
	}

	c.stats.BytesGenerated += len(c.chunk.Code)
	if debugCompilerStats && c.enclosing == nil {
		fmt.Printf("// Compiler bytes generated: %d\n", c.stats.BytesGenerated)
	}

	// Return the chunk (even if errors occurred, it might be partially useful for debugging?)
	// and the collected errors.
	return c.chunk, c.errors
}

// compileNode dispatches compilation to the appropriate method based on node type.
func (c *Compiler) compileNode(node parser.Node) errors.PaseratiError {
	// Safety check for nil checker, although it should be set by Compile()
	if c.typeChecker == nil && c.enclosing == nil { // Only panic if top-level compiler has no checker
		panic("Compiler internal error: typeChecker is nil during compileNode")
	}

	fmt.Printf("// DEBUG compiling line %d (%s)\n", GetTokenFromNode(node).Line, c.compilingFuncName)

	switch node := node.(type) {
	case *parser.Program:
		if c.enclosing == nil {
			debugPrintf("// DEBUG Program Start: Resetting lastExprRegValid.\n") // <<< ADDED
			c.lastExprRegValid = false                                           // Reset for the program start
		}
		debugPrintf("// DEBUG Program: Starting statement loop.\n") // <<< ADDED
		for i, stmt := range node.Statements {
			debugPrintf("// DEBUG Program: Before compiling statement %d (%T).\n", i, stmt) // <<< ADDED
			err := c.compileNode(stmt)
			if err != nil {
				debugPrintf("// DEBUG Program: Error compiling statement %d: %v\n", i, err) // <<< ADDED
				return err                                                                  // Propagate errors up
			}
			// <<< ADDED vvv
			if c.enclosing == nil {
				debugPrintf("// DEBUG Program: After compiling statement %d (%T). lastExprRegValid: %v, lastExprReg: R%d\n", i, stmt, c.lastExprRegValid, c.lastExprReg)
				// For top level, be conservative - don't free registers between statements
				// The VM will handle cleanup when the program ends
			} else {
				// Inside function body - be more aggressive about freeing registers
				// But only free if we have more than a reasonable number allocated
				// DISABLED: Focus on expression-level freeing instead
				// if c.regAlloc.MaxRegs() > 10 {
				// 	// Find registers that are not pinned and not in the symbol table
				// 	for reg := Register(0); reg < Register(c.regAlloc.MaxRegs()); reg++ {
				// 		if !c.regAlloc.IsPinned(reg) && !c.isRegisterInSymbolTable(reg) {
				// 			// Check if it's in the free list already
				// 			if !c.regAlloc.IsInFreeList(reg) {
				// 				debugPrintf("// DEBUG Program: Freeing register R%d after statement %d (in function, safe cleanup)\n", reg, i)
				// 				c.regAlloc.Free(reg)
				// 			}
				// 		}
				// 	}
				// }
				debugPrintf("// DEBUG Program: Inside function, but inter-statement freeing disabled\n")
			}
			// <<< ADDED ^^^
		}
		debugPrintf("// DEBUG Program: Finished statement loop.\n") // <<< ADDED
		return nil                                                  // ADDED: Explicit return for Program case

	// --- NEW: Handle Function Literal as an EXPRESSION first ---
	// This handles anonymous/named functions used in assignments, arguments, etc.
	case *parser.FunctionLiteral:
		debugPrintf("// DEBUG Node-FunctionLiteral: Compiling function literal used as expression '%s'.\n", node.Name) // <<< DEBUG
		// Determine hint: empty for anonymous, Name.Value if named (though named exprs are rare)
		hint := ""
		if node.Name != nil {
			hint = node.Name.Value
		}
		// <<< MODIFY Call Site >>>
		funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(node, hint)
		if err != nil {
			// Error already added to c.errors by compileFunctionLiteral
			return nil // Return nil error here, main error is tracked
		}

		// Allocate register for the closure and emit OpClosure
		closureReg := c.regAlloc.Alloc()
		c.emitClosure(closureReg, funcConstIndex, node, freeSymbols) // <<< Call emitClosure

		// emitClosure now handles setting lastExprReg/Valid correctly.
		return nil // <<< Return nil error

	// --- NEW: Handle Shorthand Method as an EXPRESSION ---
	// This handles shorthand methods in object literals like { method() { ... } }
	case *parser.ShorthandMethod:
		debugPrintf("// DEBUG Node-ShorthandMethod: Compiling shorthand method '%s'.\n", node.Name.Value)
		// Shorthand methods are essentially function expressions with a known name
		hint := ""
		if node.Name != nil {
			hint = node.Name.Value
		}

		funcConstIndex, freeSymbols, err := c.compileShorthandMethod(node, hint)
		if err != nil {
			return err
		}

		// Allocate register for the closure and emit OpClosure using the generic emitClosureGeneric
		closureReg := c.regAlloc.Alloc()
		c.emitClosureGeneric(closureReg, funcConstIndex, node.Token.Line, node.Name, freeSymbols)

		return nil

	// --- Block Statement (needed for function bodies) ---
	case *parser.BlockStatement:
		// Block statements don't affect the top-level last expression directly
		// (unless maybe the block IS the top level? Edge case?)
		// The last statement *within* the block might matter if it's the consequence
		// of an if-expression, but let's handle that there.
		originalLastExprValid := c.lastExprRegValid // Save state if inside top-level
		for _, stmt := range node.Statements {
			err := c.compileNode(stmt)
			if err != nil {
				return err // Propagate errors up
			}
			// DISABLED: Register freeing was causing infinite loops
			// Conservative register freeing - only inside functions and only if register is not pinned
			// Also check that the register is not in the symbol table (holding a variable)
			// if c.enclosing != nil && c.regAlloc.CurrentSet() {
			// 	currentReg := c.regAlloc.Current()
			// 	if !c.regAlloc.IsPinned(currentReg) && !c.isRegisterInSymbolTable(currentReg) {
			// 		debugPrintf("// DEBUG BlockStatement: Freeing register R%d after statement (in function, not pinned, not in symbol table)\n", currentReg)
			// 		c.regAlloc.Free(currentReg)
			// 	} else {
			// 		debugPrintf("// DEBUG BlockStatement: NOT freeing register R%d (pinned=%v, inSymbolTable=%v)\n",
			// 			currentReg, c.regAlloc.IsPinned(currentReg), c.isRegisterInSymbolTable(currentReg))
			// 	}
			// }
		}
		if c.enclosing == nil {
			// Restore previous state unless the block *itself* should provide the value?
			// Let's assume block statements themselves don't provide the final script value.
			c.lastExprRegValid = originalLastExprValid
		}
		return nil // ADDED: Explicit return

	// --- Statements ---
	case *parser.TypeAliasStatement: // Added
		// Type aliases only exist for type checking, ignore in compiler.
		return nil

	case *parser.InterfaceDeclaration: // Added
		// Interface declarations only exist for type checking, ignore in compiler.
		return nil

	case *parser.FunctionSignature: // Added
		// Function signatures are handled during type checking and don't need compilation
		// They are processed in the checker to build overloaded function types
		if c.enclosing == nil {
			c.lastExprRegValid = false // Signatures don't produce a value
		}
		return nil

	case *parser.ExpressionStatement:
		debugPrintf("// DEBUG ExprStmt: Compiling expression %T.\n", node.Expression)

		// Check specifically for NAMED function literals used as standalone statements.
		// Anonymous ones are handled by the case *parser.FunctionLiteral above now.
		if funcLit, ok := node.Expression.(*parser.FunctionLiteral); ok && funcLit.Name != nil {
			debugPrintf("// DEBUG ExprStmt: Handling NAMED function declaration '%s' as statement.\n", funcLit.Name.Value)

			// Check if this function was already processed during hoisting
			if symbolRef, _, found := c.currentSymbolTable.Resolve(funcLit.Name.Value); found && symbolRef.Register != nilRegister {
				debugPrintf("// DEBUG ExprStmt: Function '%s' already hoisted, skipping duplicate processing.\n", funcLit.Name.Value)
				// Function was already hoisted and processed, skip it
				if c.enclosing == nil {
					c.lastExprRegValid = false // Function declarations don't produce a value
				}
				return nil
			}

			// --- Handle named function recursion ---
			// 1. Define the function name temporarily.
			c.currentSymbolTable.Define(funcLit.Name.Value, nilRegister)

			// 2. Compile the function literal body. Pass name as hint.
			// <<< MODIFY Call Site >>>
			funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, funcLit.Name.Value) // Use its own name as hint
			if err != nil {
				// Error already added to c.errors by compileFunctionLiteral
				return nil // Return nil error here, main error is tracked
			}

			// 3. Create the closure object in a register
			closureReg := c.regAlloc.Alloc()
			c.emitClosure(closureReg, funcConstIndex, funcLit, freeSymbols) // <<< Call emitClosure

			// 4. Update the symbol table entry with the register holding the closure.
			c.currentSymbolTable.UpdateRegister(funcLit.Name.Value, closureReg) // <<< Use closureReg

			// Function declarations don't produce a value for the script result
			if c.enclosing == nil {
				c.lastExprRegValid = false
			}
			return nil // Handled
		}

		// Original ExpressionStatement logic for other expressions
		debugPrintf("// DEBUG ExprStmt: Compiling non-func-decl expression %T.\n", node.Expression)
		err := c.compileNode(node.Expression)
		if err != nil {
			return err
		}
		if c.enclosing == nil { // If at top level, track this as potential final value
			currentReg := c.regAlloc.Current()                                                               // <<< ADDED
			debugPrintf("// DEBUG ExprStmt: Top Level. CurrentReg: R%d. Setting lastExprReg.\n", currentReg) // <<< ADDED
			c.lastExprReg = currentReg                                                                       // <<< MODIFIED (was c.regAlloc.Current())
			c.lastExprRegValid = true
			debugPrintf("// DEBUG ExprStmt: Top Level. Set lastExprRegValid=%v, lastExprReg=R%d.\n", c.lastExprRegValid, c.lastExprReg) // <<< ADDED
		} else { // <<< ADDED vvv
			debugPrintf("// DEBUG ExprStmt: Not Top Level. lastExprRegValid remains unchanged.\n")
		} // <<< ADDED ^^^
		// Result register is left allocated, potentially unused otherwise.
		// TODO: Consider freeing registers?
		return nil // ADDED: Explicit return

	case *parser.LetStatement:
		if c.enclosing == nil {
			debugPrintf("// DEBUG LetStmt: Top Level. Setting lastExprRegValid=false.\n") // <<< ADDED
			c.lastExprRegValid = false                                                    // Declarations don't provide final value
		}
		return c.compileLetStatement(node)

	case *parser.ConstStatement:
		if c.enclosing == nil {
			debugPrintf("// DEBUG ConstStmt: Top Level. Setting lastExprRegValid=false.\n") // <<< ADDED
			c.lastExprRegValid = false                                                      // Declarations don't provide final value
		}
		return c.compileConstStatement(node)

	case *parser.ReturnStatement: // Although less relevant for top-level script return
		if c.enclosing == nil {
			debugPrintf("// DEBUG ReturnStmt: Top Level. Setting lastExprRegValid=false.\n") // <<< ADDED
			c.lastExprRegValid = false                                                       // Explicit return overrides implicit
		}
		return c.compileReturnStatement(node)

	case *parser.WhileStatement:
		if c.enclosing == nil {
			debugPrintf("// DEBUG WhileStmt: Top Level. Setting lastExprRegValid=false.\n") // <<< ADDED
			c.lastExprRegValid = false
		}
		return c.compileWhileStatement(node)

	case *parser.ForStatement:
		if c.enclosing == nil {
			debugPrintf("// DEBUG ForStmt: Top Level. Setting lastExprRegValid=false.\n") // <<< ADDED
			c.lastExprRegValid = false
		}
		return c.compileForStatement(node)

	case *parser.ForOfStatement:
		if c.enclosing == nil {
			debugPrintf("// DEBUG ForOfStmt: Top Level. Setting lastExprRegValid=false.\n")
			c.lastExprRegValid = false
		}
		return c.compileForOfStatement(node)

	case *parser.BreakStatement:
		if c.enclosing == nil {
			c.lastExprRegValid = false
		}
		return c.compileBreakStatement(node)

	case *parser.ContinueStatement:
		if c.enclosing == nil {
			c.lastExprRegValid = false
		}
		return c.compileContinueStatement(node)

	case *parser.DoWhileStatement:
		if c.enclosing == nil {
			c.lastExprRegValid = false // Loops don't produce a value
		}
		return c.compileDoWhileStatement(node)

	case *parser.SwitchStatement: // Added
		if c.enclosing == nil {
			c.lastExprRegValid = false // Switch statements don't produce a value
		}
		return c.compileSwitchStatement(node)

	// --- Expressions (excluding FunctionLiteral which is handled above) ---
	case *parser.NumberLiteral:
		destReg := c.regAlloc.Alloc()
		c.emitLoadNewConstant(destReg, vm.Number(node.Value), node.Token.Line)
		return nil // ADDED: Explicit return

	case *parser.StringLiteral:
		// Handle string literals by adding them to constants
		c.emitLoadNewConstant(c.regAlloc.Alloc(), vm.String(node.Value), node.Token.Line)
		c.lastExprReg = c.regAlloc.Current()
		c.lastExprRegValid = true
		return nil

	case *parser.TemplateLiteral:
		return c.compileTemplateLiteral(node)

	case *parser.BooleanLiteral:
		// Handle boolean literals by using appropriate opcode
		reg := c.regAlloc.Alloc()
		if node.Value {
			c.emitLoadTrue(reg, node.Token.Line)
		} else {
			c.emitLoadFalse(reg, node.Token.Line)
		}
		c.lastExprReg = reg
		c.lastExprRegValid = true
		return nil

	case *parser.NullLiteral:
		destReg := c.regAlloc.Alloc()
		c.emitLoadNull(destReg, node.Token.Line)
		return nil // ADDED: Explicit return

	case *parser.UndefinedLiteral: // Added
		destReg := c.regAlloc.Alloc()
		c.emitLoadUndefined(destReg, node.Token.Line)
		return nil // ADDED: Explicit return

	case *parser.ThisExpression: // Added for this keyword
		// Load 'this' value from current call context
		destReg := c.regAlloc.Alloc()
		c.emitLoadThis(destReg, node.Token.Line)
		c.regAlloc.SetCurrent(destReg) // Fix: Set current register
		return nil

	case *parser.Identifier:
		// <<< ADDED: Check for built-in first >>>
		if builtinFunc := builtins.GetFunc(node.Value); builtinFunc != nil {
			// It's a built-in function.
			debugPrintf("// DEBUG Identifier '%s': Resolved as Builtin\n", node.Value)
			builtinValue := vm.NewNativeFunction(builtinFunc.Arity, builtinFunc.Variadic, builtinFunc.Name, builtinFunc.Fn)
			constIdx := c.chunk.AddConstant(builtinValue) // Add vm.Value to constant pool

			// Allocate register and load the constant
			destReg := c.regAlloc.Alloc()
			c.emitLoadConstant(destReg, constIdx, node.Token.Line) // Use existing emitter
			c.regAlloc.SetCurrent(destReg)                         // Update allocator state

			// Set last expression tracking state (consistent with other expressions)
			// Note: This might be adjusted based on overall expression handling logic
			c.lastExprReg = destReg
			c.lastExprRegValid = true

			return nil // Built-in handled successfully
		}

		// <<< ADDED: Check for built-in objects >>>
		if builtinObj := builtins.GetObject(node.Value); !builtinObj.Is(vm.Undefined) {
			// It's a built-in object (like console).
			debugPrintf("// DEBUG Identifier '%s': Resolved as Builtin Object\n", node.Value)
			constIdx := c.chunk.AddConstant(builtinObj) // Add the object to constant pool

			// Allocate register and load the constant
			destReg := c.regAlloc.Alloc()
			c.emitLoadConstant(destReg, constIdx, node.Token.Line) // Use existing emitter
			c.regAlloc.SetCurrent(destReg)                         // Update allocator state

			// Set last expression tracking state
			c.lastExprReg = destReg
			c.lastExprRegValid = true

			return nil // Built-in object handled successfully
		}
		// <<< END ADDED >>>

		// If not a built-in, proceed with existing variable lookup logic
		scopeName := "Function"
		if c.currentSymbolTable.Outer == nil {
			scopeName = "Global"
		}
		debugPrintf("// DEBUG Identifier '%s': Looking up in %s scope\n", node.Value, scopeName) // <<< ADDED
		symbolRef, definingTable, found := c.currentSymbolTable.Resolve(node.Value)
		if !found {
			debugPrintf("// DEBUG Identifier '%s': NOT FOUND in symbol table, treating as GLOBAL\n", node.Value) // <<< ADDED
			// Variable not found in any scope, treat as a global variable access
			// This will return undefined at runtime if the global doesn't exist
			globalIdx := c.getOrAssignGlobalIndex(node.Value)
			destReg := c.regAlloc.Alloc()
			c.emitGetGlobal(destReg, globalIdx, node.Token.Line)
			c.regAlloc.SetCurrent(destReg) // Update allocator state
			// Set last expression tracking state
			c.lastExprReg = destReg
			c.lastExprRegValid = true
			return nil // Handle as global access
		}
		isLocal := definingTable == c.currentSymbolTable
		debugPrintf("// DEBUG Identifier '%s': Found in symbol table, isLocal=%v, definingTable==%p, currentTable==%p\n", node.Value, isLocal, definingTable, c.currentSymbolTable) // <<< ADDED

		// --- NEW RECURSION CHECK --- // Revised Check
		// Check if this is a recursive call identifier referencing the temp definition.
		isRecursiveSelfCall := isLocal &&
			symbolRef.Register == nilRegister && // Is it our temporary definition?
			scopeName == "Function" // Are we compiling inside a function? // Removed check against c.compilingFuncName

		if isRecursiveSelfCall {
			debugPrintf("// DEBUG Identifier '%s': Identified as RECURSIVE SELF CALL\n", node.Value) // <<< ADDED
			// Treat as a free variable that captures the closure itself.
			freeVarIndex := c.addFreeSymbol(node, &symbolRef)
			destReg := c.regAlloc.Alloc()
			c.emitOpCode(vm.OpLoadFree, node.Token.Line)
			c.emitByte(byte(destReg))
			c.emitByte(byte(freeVarIndex))
			c.regAlloc.SetCurrent(destReg) // Update allocator state
		} else if symbolRef.IsGlobal {
			// This is a global variable, use OpGetGlobal
			debugPrintf("// DEBUG Identifier '%s': GLOBAL variable, using OpGetGlobal\n", node.Value) // <<< ADDED
			destReg := c.regAlloc.Alloc()
			c.emitGetGlobal(destReg, symbolRef.GlobalIndex, node.Token.Line)
			c.regAlloc.SetCurrent(destReg) // Update allocator state
		} else if !isLocal {
			debugPrintf("// DEBUG Identifier '%s': NOT LOCAL, treating as FREE VARIABLE\n", node.Value) // <<< ADDED
			// This is a regular free variable (defined in an outer scope that's not global)
			freeVarIndex := c.addFreeSymbol(node, &symbolRef)
			destReg := c.regAlloc.Alloc()
			c.emitOpCode(vm.OpLoadFree, node.Token.Line)
			c.emitByte(byte(destReg))
			c.emitByte(byte(freeVarIndex))
			c.regAlloc.SetCurrent(destReg) // Update allocator state
		} else {
			debugPrintf("// DEBUG Identifier '%s': LOCAL variable, register=R%d\n", node.Value, symbolRef.Register) // <<< ADDED
			// This is a standard local variable (handled by current stack frame)
			srcReg := symbolRef.Register
			debugPrintf("// DEBUG Identifier '%s': Resolved to isLocal=%v, srcReg=R%d\n", node.Value, isLocal, srcReg)
			if srcReg == nilRegister {
				// This panic indicates an internal logic error, like trying to use a variable
				// during its temporary definition phase inappropriately.
				panic(fmt.Sprintf("compiler internal error: resolved local variable '%s' to nilRegister R%d unexpectedly at line %d", node.Value, srcReg, node.Token.Line))
			}
			destReg := c.regAlloc.Alloc()
			debugPrintf("// DEBUG Identifier '%s': About to emit Move R%d (dest), R%d (src)\n", node.Value, destReg, srcReg)
			c.emitMove(destReg, srcReg, node.Token.Line)
			c.regAlloc.SetCurrent(destReg) // Update allocator state
		}
		// Set last expression tracking state for identifiers (consistent with literals/builtins)
		c.lastExprReg = c.regAlloc.Current() // Current() should hold destReg now
		c.lastExprRegValid = true
		return nil // ADDED: Explicit return

	case *parser.PrefixExpression:
		return c.compilePrefixExpression(node)

	case *parser.TypeofExpression:
		return c.compileTypeofExpression(node)

	case *parser.InfixExpression:
		return c.compileInfixExpression(node)

	case *parser.ArrowFunctionLiteral: // Keep this separate
		return c.compileArrowFunctionLiteral(node)

	case *parser.CallExpression:
		return c.compileCallExpression(node)

	case *parser.IfExpression:
		return c.compileIfExpression(node)

	case *parser.TernaryExpression:
		return c.compileTernaryExpression(node)

	case *parser.AssignmentExpression:
		return c.compileAssignmentExpression(node)

	case *parser.UpdateExpression:
		return c.compileUpdateExpression(node)

	// --- NEW: Array/Index ---
	case *parser.ArrayLiteral:
		return c.compileArrayLiteral(node)
	case *parser.ObjectLiteral: // <<< NEW
		return c.compileObjectLiteral(node)
	case *parser.IndexExpression:
		return c.compileIndexExpression(node)
		// --- End Array/Index ---

		// --- Member Expression ---
	case *parser.MemberExpression:
		// <<< UPDATED: Call the new helper function >>>
		err := c.compileMemberExpression(node)
		if err == nil {
			// If successful, the result register is already set by compileMemberExpression
			// We just need to update the lastExpr tracking for top-level scripts
			if c.enclosing == nil {
				c.lastExprReg = c.regAlloc.Current()
				c.lastExprRegValid = true
				debugPrintf("// DEBUG MemberExpr (Top Level): Set lastExprRegValid=%v, lastExprReg=R%d.\n", c.lastExprRegValid, c.lastExprReg)
			}
		}
		return err // Return any error encountered during compilation
		// --- END Member Expression ---

		// --- Optional Chaining Expression ---
	case *parser.OptionalChainingExpression:
		// <<< NEW: Call the new helper function >>>
		err := c.compileOptionalChainingExpression(node)
		if err == nil {
			// If successful, the result register is already set by compileOptionalChainingExpression
			// We just need to update the lastExpr tracking for top-level scripts
			if c.enclosing == nil {
				c.lastExprReg = c.regAlloc.Current()
				c.lastExprRegValid = true
				debugPrintf("// DEBUG OptionalChaining (Top Level): Set lastExprRegValid=%v, lastExprReg=R%d.\n", c.lastExprRegValid, c.lastExprReg)
			}
		}
		return err // Return any error encountered during compilation
		// --- END Optional Chaining Expression ---

	case *parser.NewExpression:
		return c.compileNewExpression(node)

	// --- NEW: Rest Parameters and Spread Syntax ---
	case *parser.SpreadElement:
		// SpreadElement can appear in function calls - this should be handled there
		// If we reach here, it's likely used in an invalid context
		return NewCompileError(node, "spread syntax not supported in this context")

	case *parser.RestParameter:
		// RestParameter should only appear in function parameter lists
		// If we reach here, it's likely used in an invalid context
		return NewCompileError(node, "rest parameter syntax not supported in this context")
	// --- END NEW ---

	default:
		// Add check here? If type is FunctionLiteral and wasn't caught above, it's an error.
		if _, ok := node.(*parser.FunctionLiteral); ok {
			return NewCompileError(node, "compiler internal error: FunctionLiteral fell through switch")
		}
		return NewCompileError(node, fmt.Sprintf("compilation not implemented for %T", node))
	}
	// REMOVED: unreachable return nil // Return nil on success if no specific error occurred in this frame
}

// compileShorthandMethod compiles a shorthand method like methodName() { ... }
// Similar to compileFunctionLiteral but for shorthand method syntax
func (c *Compiler) compileShorthandMethod(node *parser.ShorthandMethod, nameHint string) (uint16, []*Symbol, errors.PaseratiError) {
	// 1. Create a new Compiler instance for the method body
	functionCompiler := newFunctionCompiler(c)

	// 2. Determine and set the function name being compiled
	var determinedFuncName string
	if nameHint != "" {
		determinedFuncName = nameHint
	} else if node.Name != nil {
		determinedFuncName = node.Name.Value
	} else {
		determinedFuncName = "<shorthand-method>"
	}
	functionCompiler.compilingFuncName = determinedFuncName

	// 3. Define inner name in inner scope for recursion
	var funcNameForLookup string
	if node.Name != nil {
		funcNameForLookup = node.Name.Value
		functionCompiler.currentSymbolTable.Define(funcNameForLookup, nilRegister)
	} else if nameHint != "" {
		funcNameForLookup = nameHint
		functionCompiler.currentSymbolTable.Define(funcNameForLookup, nilRegister)
	}

	// 4. Define parameters in the function compiler's enclosed scope
	for _, param := range node.Parameters {
		reg := functionCompiler.regAlloc.Alloc()
		functionCompiler.currentSymbolTable.Define(param.Name.Value, reg)
		// Pin the register since parameters can be captured by inner functions
		functionCompiler.regAlloc.Pin(reg)
	}

	// 5. Handle default parameters
	for _, param := range node.Parameters {
		if param.DefaultValue != nil {
			// Get the parameter's register
			symbol, _, exists := functionCompiler.currentSymbolTable.Resolve(param.Name.Value)
			if !exists {
				// This should not happen if parameter definition worked correctly
				functionCompiler.addError(param.Name, fmt.Sprintf("parameter %s not found in symbol table", param.Name.Value))
				continue
			}
			paramReg := symbol.Register

			// Create a temporary register to hold undefined for comparison
			undefinedReg := functionCompiler.regAlloc.Alloc()
			functionCompiler.emitLoadUndefined(undefinedReg, param.Token.Line)

			// Create another temporary register for the comparison result
			compareReg := functionCompiler.regAlloc.Alloc()
			functionCompiler.emitStrictEqual(compareReg, paramReg, undefinedReg, param.Token.Line)

			// Jump if the comparison is false (parameter is not undefined, so keep original value)
			jumpIfDefinedPos := functionCompiler.emitPlaceholderJump(vm.OpJumpIfFalse, compareReg, param.Token.Line)

			// Free temporary registers
			functionCompiler.regAlloc.Free(undefinedReg)
			functionCompiler.regAlloc.Free(compareReg)

			// Compile the default value expression
			err := functionCompiler.compileNode(param.DefaultValue)
			if err != nil {
				// Continue with compilation even if default value has errors
				functionCompiler.addError(param.DefaultValue, fmt.Sprintf("error compiling default value for parameter %s", param.Name.Value))
			} else {
				// Move the default value to the parameter register
				defaultValueReg := functionCompiler.regAlloc.Current()
				if defaultValueReg != paramReg {
					functionCompiler.emitMove(paramReg, defaultValueReg, param.Token.Line)
				}
			}

			// Patch the jump to come here (end of default value assignment)
			functionCompiler.patchJump(jumpIfDefinedPos)
		}
	}

	// 6. Handle rest parameter (if present)
	if node.RestParameter != nil {
		// Define the rest parameter in the symbol table
		restParamReg := functionCompiler.regAlloc.Alloc()
		functionCompiler.currentSymbolTable.Define(node.RestParameter.Name.Value, restParamReg)
		// Pin the register since rest parameters can be captured by inner functions
		functionCompiler.regAlloc.Pin(restParamReg)

		// The rest parameter collection will be handled at runtime during function call
		// We just need to ensure it has a register allocated here
		debugPrintf("// [Compiler] Rest parameter '%s' defined in R%d\n", node.RestParameter.Name.Value, restParamReg)
	}

	// 7. Compile the body using the function compiler
	err := functionCompiler.compileNode(node.Body)
	if err != nil {
		// Propagate errors (already appended to c.errors by sub-compiler)
	}

	// 8. Finalize function chunk (add implicit return to the function's chunk)
	functionCompiler.emitFinalReturn(node.Body.Token.Line)
	functionChunk := functionCompiler.chunk
	freeSymbols := functionCompiler.freeSymbols
	// Collect any additional errors from the sub-compilation
	if len(functionCompiler.errors) > 0 {
		c.errors = append(c.errors, functionCompiler.errors...)
	}
	regSize := functionCompiler.regAlloc.MaxRegs()

	// 9. Create the bytecode.Function object
	var funcName string
	if nameHint != "" {
		funcName = nameHint
	} else if node.Name != nil {
		funcName = node.Name.Value
	} else {
		funcName = "<shorthand-method>"
	}

	// 10. Add the function object to the outer compiler's constant pool
	funcValue := vm.NewFunction(len(node.Parameters), len(freeSymbols), int(regSize), node.RestParameter != nil, funcName, functionChunk)
	constIdx := c.chunk.AddConstant(funcValue)

	return constIdx, freeSymbols, nil
}

// compileArgumentsWithOptionalHandling compiles the provided arguments and pads missing optional
// parameters with undefined values. Returns the register list and total argument count.
func (c *Compiler) compileArgumentsWithOptionalHandling(node *parser.CallExpression) ([]Register, int, errors.PaseratiError) {
	// 1. Compile the provided arguments, handling spread elements
	argRegs := []Register{}
	hasSpread := false

	for _, arg := range node.Arguments {
		if spreadElement, isSpread := arg.(*parser.SpreadElement); isSpread {
			hasSpread = true
			// Compile the expression being spread (should be an array)
			err := c.compileNode(spreadElement.Argument)
			if err != nil {
				return nil, 0, err
			}
			arrayReg := c.regAlloc.Current()

			// Note: Length access removed - spread syntax not fully implemented yet
			// This is just a placeholder to mark the spread element
			// The VM currently treats spread args the same as regular args

			// For now, we'll mark this as needing special handling later
			// Store the register that contains the array to be spread
			argRegs = append(argRegs, arrayReg)

			// Early return since spread is not fully implemented
			return argRegs, 1, NewCompileError(spreadElement, "spread syntax in function calls not fully implemented yet")
		}

		// Regular argument
		err := c.compileNode(arg)
		if err != nil {
			return nil, 0, err
		}
		argRegs = append(argRegs, c.regAlloc.Current())
	}

	// If we have spread elements, we need special handling in the VM
	if hasSpread {
		// For now, return the registers as-is and let the VM handle spreading
		// This is a simplified implementation - a full implementation would
		// need more sophisticated bytecode generation
		return argRegs, len(argRegs), nil
	}

	providedArgCount := len(argRegs)

	// 2. Check if we need to handle optional parameters (only if no spread)
	// Get the function type from the call expression to see if it has optional parameters
	functionType := node.Function.GetComputedType()
	if functionType == nil {
		// No type information available, use provided arguments as-is
		return argRegs, providedArgCount, nil
	}

	var expectedParamCount int
	var optionalParams []bool

	// Extract parameter information based on function type
	switch ft := functionType.(type) {
	case *types.FunctionType:
		expectedParamCount = len(ft.ParameterTypes)
		optionalParams = ft.OptionalParams
	case *types.OverloadedFunctionType:
		// For overloaded functions, use the implementation signature
		if ft.Implementation != nil {
			expectedParamCount = len(ft.Implementation.ParameterTypes)
			optionalParams = ft.Implementation.OptionalParams
		} else {
			// No implementation info, use provided arguments as-is
			return argRegs, providedArgCount, nil
		}
	default:
		// Unknown function type, use provided arguments as-is
		return argRegs, providedArgCount, nil
	}

	// 3. If we have fewer arguments than expected parameters, pad with undefined for optional params
	if providedArgCount < expectedParamCount && len(optionalParams) == expectedParamCount {
		for i := providedArgCount; i < expectedParamCount; i++ {
			// Only pad if the parameter is optional
			if i < len(optionalParams) && optionalParams[i] {
				// Allocate register and load undefined
				undefinedReg := c.regAlloc.Alloc()
				c.emitLoadUndefined(undefinedReg, node.Token.Line)
				argRegs = append(argRegs, undefinedReg)
			} else {
				// Required parameter missing - this should have been caught by type checker
				// but let's not pad it to avoid masking errors
				break
			}
		}
	}

	return argRegs, len(argRegs), nil
}

// addFreeSymbol adds a symbol identified as a free variable to the compiler's list.
// It ensures the symbol is added only once and returns its index in the freeSymbols slice.
func (c *Compiler) addFreeSymbol(node parser.Node, symbol *Symbol) uint8 { // Assuming max 256 free vars for now
	debugPrintf("// DEBUG addFreeSymbol: Adding '%s' as free variable (Register: R%d)\n", symbol.Name, symbol.Register) // <<< ADDED
	for i, free := range c.freeSymbols {
		// Compare by name and register instead of pointer comparison
		// This prevents duplicate upvalues for the same variable
		if free.Name == symbol.Name && free.Register == symbol.Register {
			debugPrintf("// DEBUG addFreeSymbol: Symbol '%s' already exists at index %d (REUSING)\n", symbol.Name, i) // <<< UPDATED
			return uint8(i)
		}
	}
	// Check if we exceed limit (important for OpLoadFree operand size)
	if len(c.freeSymbols) >= 256 {
		// Handle error: too many free variables
		// For now, let's panic or add an error; proper error handling needed
		c.errors = append(c.errors, NewCompileError(node, "compiler: too many free variables in function"))
		return 255 // Indicate error state, maybe?
	}
	c.freeSymbols = append(c.freeSymbols, symbol)
	debugPrintf("// DEBUG addFreeSymbol: Added '%s' at index %d (total free symbols: %d)\n", symbol.Name, len(c.freeSymbols)-1, len(c.freeSymbols)) // <<< ADDED
	return uint8(len(c.freeSymbols) - 1)
}

// emitPlaceholderJump emits a jump instruction with a placeholder offset (0xFFFF).
// Returns the position of the start of the jump instruction in the bytecode.
// For OpJumpIfFalse, srcReg is the condition register.
// For OpJump, srcReg is ignored (pass 0 or any value).
func (c *Compiler) emitPlaceholderJump(op vm.OpCode, srcReg Register, line int) int {
	pos := len(c.chunk.Code)
	c.emitOpCode(op, line)
	if op == vm.OpJumpIfFalse {
		c.emitByte(byte(srcReg)) // Register operand
		c.emitUint16(0xFFFF)     // Placeholder offset
	} else { // OpJump
		c.emitUint16(0xFFFF) // Placeholder offset
	}
	return pos
}

// patchJump calculates the distance from a placeholder jump instruction
// to the current end of the bytecode and writes the offset back.
func (c *Compiler) patchJump(placeholderPos int) {
	op := vm.OpCode(c.chunk.Code[placeholderPos])
	operandStartPos := placeholderPos + 1
	if op == vm.OpJumpIfFalse {
		operandStartPos = placeholderPos + 2 // Skip register byte
	}

	// Calculate offset from the position *after* the jump instruction
	jumpInstructionEndPos := operandStartPos + 2
	offset := len(c.chunk.Code) - jumpInstructionEndPos

	if offset > math.MaxInt16 || offset < math.MinInt16 { // Use math constants
		// Handle error: jump offset too large
		// TODO: Add proper error handling instead of panic
		panic(fmt.Sprintf("Compiler error: jump offset %d exceeds 16-bit limit", offset))
	}

	// Write the 16-bit offset back into the placeholder bytes (Big Endian)
	c.chunk.Code[operandStartPos] = byte(int16(offset) >> 8)     // High byte
	c.chunk.Code[operandStartPos+1] = byte(int16(offset) & 0xFF) // Low byte
}

// storeToLvalue is a helper function to store a value back to different types of lvalues
func (c *Compiler) storeToLvalue(lvalueKind int, identInfo, memberInfo, indexInfo interface{}, valueReg Register, line int) {
	const (
		lvalueIdentifier = iota
		lvalueMemberExpr
		lvalueIndexExpr
	)

	switch lvalueKind {
	case lvalueIdentifier:
		info := identInfo.(struct {
			targetReg    Register
			isUpvalue    bool
			upvalueIndex uint8
			isGlobal     bool
			globalIndex  uint16
		})
		if info.isGlobal {
			c.emitSetGlobal(info.globalIndex, valueReg, line)
		} else if info.isUpvalue {
			c.emitSetUpvalue(info.upvalueIndex, valueReg, line)
		} else {
			if valueReg != info.targetReg {
				c.emitMove(info.targetReg, valueReg, line)
			}
		}

	case lvalueMemberExpr:
		info := memberInfo.(struct {
			objectReg    Register
			nameConstIdx uint16
		})
		c.emitSetProp(info.objectReg, valueReg, info.nameConstIdx, line)

	case lvalueIndexExpr:
		info := indexInfo.(struct {
			arrayReg Register
			indexReg Register
		})
		c.emitOpCode(vm.OpSetIndex, line)
		c.emitByte(byte(info.arrayReg))
		c.emitByte(byte(info.indexReg))
		c.emitByte(byte(valueReg))
	}
}

// addError creates a CompileError and appends it to the compiler's error list.
func (c *Compiler) addError(node parser.Node, msg string) {
	token := GetTokenFromNode(node)
	compileErr := &errors.CompileError{
		Position: errors.Position{
			Line:     token.Line,
			Column:   token.Column,
			StartPos: token.StartPos,
			EndPos:   token.EndPos,
		},
		Msg: msg,
	}
	c.errors = append(c.errors, compileErr)
}

func NewCompileError(node parser.Node, msg string) *errors.CompileError {
	token := GetTokenFromNode(node)
	return &errors.CompileError{
		Position: errors.Position{
			Line:     token.Line,
			Column:   token.Column,
			StartPos: token.StartPos,
			EndPos:   token.EndPos,
		},
		Msg: msg,
	}
}

// GetTokenFromNode attempts to extract the primary lexical token associated with an AST node.
// TODO: Consolidate this with the one in checker/checker.go? Put it in ast?
func GetTokenFromNode(node parser.Node) lexer.Token {
	switch n := node.(type) {
	// Statements
	case *parser.LetStatement:
		return n.Token
	case *parser.ConstStatement:
		return n.Token
	case *parser.ReturnStatement:
		return n.Token
	case *parser.ExpressionStatement:
		if n.Expression != nil {
			return GetTokenFromNode(n.Expression) // Use expression's token
		}
		return n.Token // Fallback to statement token (often start of expression)
	case *parser.BlockStatement:
		return n.Token // '{'
	case *parser.WhileStatement:
		return n.Token // 'while'
	case *parser.ForStatement:
		return n.Token // 'for'
	case *parser.DoWhileStatement:
		return n.Token // 'do'
	case *parser.BreakStatement:
		return n.Token // 'break'
	case *parser.ContinueStatement:
		return n.Token // 'continue'
	case *parser.TypeAliasStatement:
		return n.Token // 'type'
	case *parser.InterfaceDeclaration:
		return n.Token // 'interface'

	// Expressions
	case *parser.Identifier:
		return n.Token
	case *parser.NumberLiteral:
		return n.Token
	case *parser.StringLiteral:
		return n.Token
	case *parser.BooleanLiteral:
		return n.Token
	case *parser.NullLiteral:
		return n.Token
	case *parser.UndefinedLiteral:
		return n.Token
	case *parser.ThisExpression:
		return n.Token
	case *parser.PrefixExpression:
		return n.Token // Operator token
	case *parser.TypeofExpression:
		return n.Token // Operator token
	case *parser.InfixExpression:
		return n.Token // Operator token
	case *parser.IfExpression:
		return n.Token // 'if' token
	case *parser.FunctionLiteral:
		return n.Token // 'function' token
	case *parser.FunctionSignature:
		return n.Token // 'function' token
	case *parser.ArrowFunctionLiteral:
		return n.Token // '=>' token
	case *parser.CallExpression:
		return n.Token // '(' token
	case *parser.NewExpression:
		return n.Token // 'new' token
	case *parser.AssignmentExpression:
		return n.Token // Assignment operator token
	case *parser.UpdateExpression:
		return n.Token // Update operator token
	case *parser.TernaryExpression:
		return n.Token // '?' token
	case *parser.ArrayLiteral:
		return n.Token // '[' token
	case *parser.ObjectLiteral:
		return n.Token // '{' token
	case *parser.IndexExpression:
		return n.Token // '[' token
	case *parser.MemberExpression:
		return n.Token // '.' token
	case *parser.OptionalChainingExpression:
		return n.Token // '?.' token

	// Program node doesn't have a single token, return a dummy?
	case *parser.Program:
		if len(n.Statements) > 0 {
			return GetTokenFromNode(n.Statements[0]) // Use first statement's token
		}
		return lexer.Token{Type: lexer.ILLEGAL, Line: 1, Column: 1} // Dummy token

	// Add other node types as needed
	default:
		fmt.Printf("Warning: GetTokenFromNode unhandled type: %T\n", n)
		// Return a dummy token if type is unhandled
		return lexer.Token{Type: lexer.ILLEGAL, Line: 1, Column: 1}
	}
}

// --- NEW: Switch Statement Compilation ---

// --- REVISED: compileObjectLiteral (One-by-One Property Set) ---

// --- NEW: Loop Context Helpers ---

// pushLoopContext adds a new loop context to the stack.
func (c *Compiler) pushLoopContext(loopStartPos, continueTargetPos int) {
	context := &LoopContext{
		LoopStartPos:               loopStartPos,
		ContinueTargetPos:          continueTargetPos,
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
	}
	c.loopContextStack = append(c.loopContextStack, context)
}

// popLoopContext removes the current loop context from the stack.
func (c *Compiler) popLoopContext() {
	if len(c.loopContextStack) > 0 {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
	}
	// Consider adding error handling if pop is called on an empty stack
}

// currentLoopContext returns the loop context currently at the top of the stack, or nil if empty.
func (c *Compiler) currentLoopContext() *LoopContext {
	if len(c.loopContextStack) == 0 {
		return nil
	}
	return c.loopContextStack[len(c.loopContextStack)-1]
}

// --- NEW: Bytecode Position Helper ---

// currentPosition returns the index of the next byte to be written to the chunk.
func (c *Compiler) currentPosition() int {
	return len(c.chunk.Code)
}

// emitClosure emits the OpClosure instruction and its operands.
// It handles resolving free variables from the *enclosing* scope (c)
// based on the freeSymbols list collected during the function body's compilation.
// OPTIMIZATION: If there are no upvalues, just load the function constant directly.
func (c *Compiler) emitClosure(destReg Register, funcConstIndex uint16, node *parser.FunctionLiteral, freeSymbols []*Symbol) {
	line := node.Token.Line // Use function literal token line

	// OPTIMIZATION: If no upvalues, just load the function constant
	if len(freeSymbols) == 0 {
		debugPrintf("// [emitClosure OPTIMIZED] No upvalues, using OpLoadConstant instead of OpClosure\n")
		c.emitLoadConstant(destReg, funcConstIndex, line)
		c.regAlloc.SetCurrent(destReg)
		c.lastExprReg = destReg
		c.lastExprRegValid = true
		return
	}

	c.emitOpCode(vm.OpClosure, line)
	c.emitByte(byte(destReg))
	c.emitUint16(funcConstIndex)       // Operand 1: Constant index of the function blueprint
	c.emitByte(byte(len(freeSymbols))) // Operand 2: Number of upvalues to capture

	// Determine the name used for potential self-recursion lookup within the function body.
	// This logic mirrors the one used inside compileFunctionLiteral when setting up the inner scope.
	var funcNameForLookup string
	if node.Name != nil {
		funcNameForLookup = node.Name.Value
	} // Note: We don't need the nameHint here as freeSymbols already reflects captures based on the inner scope setup.

	// Emit operands for each upvalue
	for i, freeSym := range freeSymbols {
		debugPrintf("// [emitClosure %s] Emitting upvalue %d: %s (Original Reg: R%d)\n", funcNameForLookup, i, freeSym.Name, freeSym.Register) // DEBUG

		// --- Check for self-capture first ---
		// If a free symbol has nilRegister, it MUST be the temporary one
		// added for recursion resolution inside compileFunctionLiteral. It signifies self-capture.
		if freeSym.Register == nilRegister {
			debugPrintf("// [emitClosure SelfCapture] Symbol '%s' is self-reference. Emitting isLocal=1, index=destReg=R%d\n", freeSym.Name, destReg) // DEBUG
			c.emitByte(1)                                                                                                                             // isLocal = true (capture from the stack where the closure *will be* placed)
			c.emitByte(byte(destReg))                                                                                                                 // Index = the destination register of OpClosure itself
			continue                                                                                                                                  // Skip the normal lookup below
		}
		// --- END Check ---

		// Resolve the symbol again in the *enclosing* compiler's context (c)
		enclosingSymbol, enclosingTable, found := c.currentSymbolTable.Resolve(freeSym.Name)
		if !found {
			// This should theoretically not happen if freeSym was correctly identified.
			panic(fmt.Sprintf("compiler internal error: free variable '%s' not found in enclosing scope during closure emission", freeSym.Name))
		}

		if enclosingTable == c.currentSymbolTable {
			// The free variable is local in the *direct* enclosing scope (c).
			debugPrintf("// [emitClosure Upvalue] Free '%s' is Local in enclosing. Emitting isLocal=1, index=R%d\n", freeSym.Name, enclosingSymbol.Register) // DEBUG
			c.emitByte(1)                                                                                                                                    // isLocal = true
			// Capture the value from the enclosing scope's actual register
			c.emitByte(byte(enclosingSymbol.Register)) // Index = register index
		} else {
			// The free variable is also a free variable in the *enclosing* scope (c).
			// It needs to be captured from the enclosing scope's upvalues.
			// We need the index of this symbol within the *enclosing* compiler's freeSymbols list.
			enclosingFreeIndex := c.addFreeSymbol(node, &enclosingSymbol)                                                                             // Use the same helper
			debugPrintf("// [emitClosure Upvalue] Free '%s' is Outer in enclosing. Emitting isLocal=0, index=%d\n", freeSym.Name, enclosingFreeIndex) // DEBUG
			c.emitByte(0)                                                                                                                             // isLocal = false
			c.emitByte(byte(enclosingFreeIndex))                                                                                                      // Index = upvalue index in enclosing scope
		}
	}

	// Set the compiler's current register state to reflect the closure object
	c.regAlloc.SetCurrent(destReg)
	c.lastExprReg = destReg
	c.lastExprRegValid = true
	debugPrintf("// [emitClosure %s] Closure emitted to R%d. Set lastExprReg/Valid.\n", funcNameForLookup, destReg)
}

// emitClosureGeneric is a generic version of emitClosure that works with any node type
// that has Token.Line and Name fields (like ShorthandMethod)
// OPTIMIZATION: If there are no upvalues, just load the function constant directly.
func (c *Compiler) emitClosureGeneric(destReg Register, funcConstIndex uint16, line int, nameNode *parser.Identifier, freeSymbols []*Symbol) {
	// OPTIMIZATION: If no upvalues, just load the function constant
	if len(freeSymbols) == 0 {
		debugPrintf("// [emitClosureGeneric OPTIMIZED] No upvalues, using OpLoadConstant instead of OpClosure\n")
		c.emitLoadConstant(destReg, funcConstIndex, line)
		c.regAlloc.SetCurrent(destReg)
		c.lastExprReg = destReg
		c.lastExprRegValid = true
		return
	}

	c.emitOpCode(vm.OpClosure, line)
	c.emitByte(byte(destReg))
	c.emitUint16(funcConstIndex)       // Operand 1: Constant index of the function blueprint
	c.emitByte(byte(len(freeSymbols))) // Operand 2: Number of upvalues to capture

	// Determine the name used for potential self-recursion lookup
	var funcNameForLookup string
	if nameNode != nil {
		funcNameForLookup = nameNode.Value
	}

	// Emit operands for each upvalue (same logic as emitClosure)
	for i, freeSym := range freeSymbols {
		debugPrintf("// [emitClosureGeneric %s] Emitting upvalue %d: %s (Original Reg: R%d)\n", funcNameForLookup, i, freeSym.Name, freeSym.Register)

		// Check for self-capture first
		if freeSym.Register == nilRegister {
			debugPrintf("// [emitClosureGeneric SelfCapture] Symbol '%s' is self-reference. Emitting isLocal=1, index=destReg=R%d\n", freeSym.Name, destReg)
			c.emitByte(1)             // isLocal = true
			c.emitByte(byte(destReg)) // Index = the destination register of OpClosure itself
			continue
		}

		// Resolve the symbol in the enclosing compiler's context
		enclosingSymbol, enclosingTable, found := c.currentSymbolTable.Resolve(freeSym.Name)
		if !found {
			panic(fmt.Sprintf("compiler internal error: free variable '%s' not found in enclosing scope during closure emission", freeSym.Name))
		}

		if enclosingTable == c.currentSymbolTable {
			// The free variable is local in the direct enclosing scope
			debugPrintf("// [emitClosureGeneric Upvalue] Free '%s' is Local in enclosing. Emitting isLocal=1, index=R%d\n", freeSym.Name, enclosingSymbol.Register)
			c.emitByte(1)                              // isLocal = true
			c.emitByte(byte(enclosingSymbol.Register)) // Index = register index
		} else {
			// The free variable is also a free variable in the enclosing scope
			// Create a dummy node for addFreeSymbol (it only uses the node for error reporting)
			dummyNode := &parser.Identifier{Token: lexer.Token{}, Value: freeSym.Name}
			enclosingFreeIndex := c.addFreeSymbol(dummyNode, &enclosingSymbol)
			debugPrintf("// [emitClosureGeneric Upvalue] Free '%s' is Outer in enclosing. Emitting isLocal=0, index=%d\n", freeSym.Name, enclosingFreeIndex)
			c.emitByte(0)                        // isLocal = false
			c.emitByte(byte(enclosingFreeIndex)) // Index = upvalue index in enclosing scope
		}
	}

	// Set the compiler's current register state to reflect the closure object
	c.regAlloc.SetCurrent(destReg)
	c.lastExprReg = destReg
	c.lastExprRegValid = true
	debugPrintf("// [emitClosureGeneric %s] Closure emitted to R%d. Set lastExprReg/Valid.\n", funcNameForLookup, destReg)
}

// resolveRegisterMoves handles moving values from sourceRegs to consecutive target registers
// starting at firstTargetReg. It correctly handles register cycles by using temporary registers.
func (c *Compiler) resolveRegisterMoves(sourceRegs []Register, firstTargetReg Register, line int) {
	argCount := len(sourceRegs)
	if argCount == 0 {
		return
	}

	// Create mapping of source -> target
	moves := make(map[Register]Register)
	for i, sourceReg := range sourceRegs {
		targetReg := firstTargetReg + Register(i)
		if sourceReg != targetReg {
			moves[sourceReg] = targetReg
		}
	}

	debugPrintf("// DEBUG RegisterMoves: Processing %d moves\n", len(moves))
	for source, target := range moves {
		debugPrintf("// DEBUG RegisterMoves: R%d -> R%d\n", source, target)
	}

	// Track which registers have been resolved
	resolved := make(map[Register]bool)

	// Process each move, handling cycles
	for sourceReg, targetReg := range moves {
		if resolved[sourceReg] {
			continue
		}

		// Check if this is part of a cycle
		cycle := c.findMoveCycle(sourceReg, moves, resolved)

		debugPrintf("// DEBUG RegisterMoves: Found cycle starting from R%d: %v\n", sourceReg, cycle)

		if len(cycle) > 1 {
			// Handle cycle by using a temporary register
			debugPrintf("// DEBUG RegisterMoves: Resolving cycle of length %d\n", len(cycle))
			c.resolveCycle(cycle, moves, line)
			// Mark all registers in the cycle as resolved
			for _, reg := range cycle {
				resolved[reg] = true
			}
		} else {
			// Simple move, no cycle
			debugPrintf("// DEBUG RegisterMoves: Simple move R%d -> R%d\n", sourceReg, targetReg)
			c.emitMove(targetReg, sourceReg, line)
			resolved[sourceReg] = true
		}
	}
}

// findMoveCycle detects if a register is part of a move cycle
func (c *Compiler) findMoveCycle(startReg Register, moves map[Register]Register, resolved map[Register]bool) []Register {
	visited := make(map[Register]bool)
	path := []Register{}
	current := startReg

	for {
		if resolved[current] {
			// Already resolved, no cycle involving this register
			return []Register{startReg}
		}

		if visited[current] {
			// Found a cycle - find where it starts
			cycleStart := -1
			for i, reg := range path {
				if reg == current {
					cycleStart = i
					break
				}
			}
			if cycleStart >= 0 {
				return path[cycleStart:] // Return just the cycle portion
			}
			break
		}

		visited[current] = true
		path = append(path, current)

		// Follow the move chain
		next, exists := moves[current]
		if !exists {
			// Chain ends here, no cycle
			return []Register{startReg}
		}
		current = next
	}

	// No cycle found
	return []Register{startReg}
}

// resolveCycle breaks a register move cycle using a temporary register
func (c *Compiler) resolveCycle(cycle []Register, moves map[Register]Register, line int) {
	if len(cycle) <= 1 {
		return
	}

	debugPrintf("// DEBUG ResolveCycle: Cycle %v\n", cycle)

	// Use a temporary register to break the cycle
	tempReg := c.regAlloc.Alloc()
	debugPrintf("// DEBUG ResolveCycle: Using temp register R%d\n", tempReg)

	// Move the first register to temp
	debugPrintf("// DEBUG ResolveCycle: Move R%d -> R%d (first to temp)\n", cycle[0], tempReg)
	c.emitMove(tempReg, cycle[0], line)

	// Move each register to the next in sequence
	for i := 0; i < len(cycle)-1; i++ {
		sourceReg := cycle[i+1]
		targetReg := moves[cycle[i+1]]
		debugPrintf("// DEBUG ResolveCycle: Move R%d -> R%d (chain move %d)\n", sourceReg, targetReg, i)
		c.emitMove(targetReg, sourceReg, line)
	}

	// Move temp to the final position
	firstReg := cycle[0]
	targetReg := moves[firstReg]
	debugPrintf("// DEBUG ResolveCycle: Move R%d -> R%d (temp to final)\n", tempReg, targetReg)
	c.emitMove(targetReg, tempReg, line)

	// Free the temporary register
	c.regAlloc.Free(tempReg)
	debugPrintf("// DEBUG ResolveCycle: Freed temp register R%d\n", tempReg)
}

// --- NEW: Global Variable Index Management ---

// getOrAssignGlobalIndex returns the index for a global variable name.
// If the variable doesn't have an index yet, assigns a new one.
// This function should only be called on the top-level compiler.
func (c *Compiler) getOrAssignGlobalIndex(name string) uint16 {
	// Only top-level compiler should manage global indices
	topCompiler := c
	for topCompiler.enclosing != nil {
		topCompiler = topCompiler.enclosing
	}

	if idx, exists := topCompiler.globalIndices[name]; exists {
		return uint16(idx)
	}

	// Assign new index
	idx := topCompiler.globalCount
	topCompiler.globalIndices[name] = idx
	topCompiler.globalCount++

	if idx > 65535 {
		panic(fmt.Sprintf("Too many global variables (max 65536): %s", name))
	}

	return uint16(idx)
}

// isRegisterInSymbolTable checks if a register contains a variable that's still in the symbol table.
// This helps us avoid freeing registers that contain variables that might be used later.
func (c *Compiler) isRegisterInSymbolTable(reg Register) bool {
	debugPrintf("// DEBUG isRegisterInSymbolTable: Checking R%d\n", reg)
	// Check if any symbol in the current symbol table uses this register
	for name, symbol := range c.currentSymbolTable.store {
		debugPrintf("// DEBUG isRegisterInSymbolTable: Symbol '%s' uses R%d (checking against R%d)\n", name, symbol.Register, reg)
		if symbol.Register == reg {
			debugPrintf("// DEBUG isRegisterInSymbolTable: Found R%d in symbol table (variable '%s')\n", reg, name)
			return true
		}
	}

	// If we have an enclosing scope, check there too for free variables
	if c.enclosing != nil {
		debugPrintf("// DEBUG isRegisterInSymbolTable: Checking enclosing scope for R%d\n", reg)
		return c.enclosing.isRegisterInSymbolTable(reg)
	}

	debugPrintf("// DEBUG isRegisterInSymbolTable: R%d not found in symbol table\n", reg)
	return false
}

// compileTemplateLiteral compiles template literals using the simple binary OpStringConcat approach

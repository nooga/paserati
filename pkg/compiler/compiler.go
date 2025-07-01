package compiler

import (
	"fmt"
	"math"
	"paserati/pkg/checker"
	"paserati/pkg/errors"
	"paserati/pkg/lexer"
	"paserati/pkg/modules"
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

const debugCompiler = false     // Enable for debugging register allocation issue
const debugCompilerStats = false // <<< CHANGED back to false
const debugCompiledCode = false
const debugPrint = false // Enable debug output

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
	loopContextStack   []*LoopContext
	compilingFuncName  string
	typeChecker        *checker.Checker // Holds the checker instance
	stats              *CompilerStats

	// --- NEW: Global Variable Indexing for Performance ---
	// Maps global variable names to their assigned indices (only for top-level compiler)
	globalIndices map[string]int
	// Count of global variables assigned so far (only for top-level compiler)
	globalCount int
	// line tracking
	line int

	// --- NEW: Constant Register Cache for Performance ---
	// Maps constant index to register that currently holds it
	constantCache map[uint16]Register

	// --- Phase 4a: Finally Context Tracking ---
	inFinallyBlock  bool // Track if we're compiling inside finally block
	tryFinallyDepth int  // Number of enclosing try-with-finally blocks
	
	// --- Phase 5: Module Bindings ---
	moduleBindings  *ModuleBindings      // Module-aware binding resolver
	moduleLoader    modules.ModuleLoader // Reference to module loader
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
		loopContextStack:   make([]*LoopContext, 0),
		compilingFuncName:  "<script>",
		typeChecker:        nil, // Initialized to nil, can be set externally
		stats:              &CompilerStats{},
		globalIndices:      make(map[string]int),
		globalCount:        0,
		line:               -1,
		constantCache:      make(map[uint16]Register),
	}
}

// SetChecker allows injecting an external checker instance.
// This is used by the driver for REPL sessions.
func (c *Compiler) SetChecker(checker *checker.Checker) {
	c.typeChecker = checker
}

// EnableModuleMode enables module-aware compilation with binding resolution
// Parallels the checker's EnableModuleMode method
func (c *Compiler) EnableModuleMode(modulePath string, loader modules.ModuleLoader) {
	c.moduleBindings = NewModuleBindings(modulePath, loader)
	c.moduleLoader = loader
}

// IsModuleMode returns true if the compiler is in module mode
func (c *Compiler) IsModuleMode() bool {
	return c.moduleBindings != nil
}

// GetModuleExports returns all exported values from the current module
func (c *Compiler) GetModuleExports() map[string]vm.Value {
	if c.moduleBindings == nil {
		return make(map[string]vm.Value)
	}
	return c.moduleBindings.GetAllExports()
}

// newFunctionCompiler creates a compiler instance specifically for a function body.
func newFunctionCompiler(enclosingCompiler *Compiler) *Compiler {
	// Pass the checker and module bindings down to nested compilers
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
		constantCache:      make(map[uint16]Register), // Each function has its own constant cache
		moduleBindings:     enclosingCompiler.moduleBindings, // Inherit module bindings
		moduleLoader:       enclosingCompiler.moduleLoader,   // Inherit module loader
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
	
	// Check if this program has already been type-checked by comparing the AST
	// In module loading flow, the checker is already used to check the program
	if c.typeChecker.GetProgram() != program {
		// Program hasn't been checked yet, perform type checking
		// The checker modifies its own environment state.
		typeErrors := c.typeChecker.Check(program)
		if len(typeErrors) > 0 {
			// Found type errors. Return them immediately.
			// Type errors are already []errors.PaseratiError from the checker.
			return nil, typeErrors
		}
	} else {
		// Program was already type-checked by this checker, skip redundant check
		debugPrintf("// [Compiler] Skipping type check - program already checked by this checker\n")
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
			closureReg := c.emitClosure(c.regAlloc.Alloc(), funcConstIndex, funcLit, freeSymbols) // Use actual freeSymbols

			// 4. Get global index and define as global symbol
			globalIdx := c.GetOrAssignGlobalIndex(name)

			// Update the symbol table entry to mark it as global
			c.currentSymbolTable.DefineGlobal(name, globalIdx)

			// 5. Emit OpSetGlobal to store the function in the VM's globals array
			c.emitSetGlobal(globalIdx, closureReg, funcLit.Token.Line)

			debugPrintf("[Compile Hoisting] Defined global func '%s' with %d upvalues in R%d, stored at global index %d\n", name, len(freeSymbols), closureReg, globalIdx)

		}
	}
	// --- END Hoisted Global Function Processing ---

	resultReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(resultReg)
	// Use the already type-checked program node.
	resultReg, err := c.compileNode(program, resultReg)
	if err != nil {
		// An error occurred during compilation. addError should have already been called
		// when the error was generated lower down. The returned `err` is mainly for control flow.
		// We don't need to append err.Error() here again.
	}

	// Emit final return instruction if no *compilation* errors occurred
	// (Type errors were caught earlier and returned).
	if len(c.errors) == 0 {
		if c.enclosing == nil { // Top-level script
			c.emitReturn(resultReg, 0)
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

	// <<< ADDED: Log final register allocator state for debugging register leaks >>>
	if debugCompiler && c.enclosing == nil {
		fmt.Printf("// [FINAL REGALLOC STATE] NextReg: R%d, MaxReg: R%d, FreeRegs: %v, Pinned: %d\n",
			c.regAlloc.nextReg, c.regAlloc.maxReg+1, c.regAlloc.freeRegs, len(c.regAlloc.pinnedRegs))
		if c.regAlloc.nextReg > 20 { // Only warn if we have a lot of registers allocated
			fmt.Printf("// [REGALLOC WARNING] High register usage detected - potential leakage!\n")
		}
	}
	// <<< END ADDED >>>

	// <<< ADDED: Debug dump bytecode for functions >>>
	if debugCompiler && c.compilingFuncName != "<script>" && c.compilingFuncName != "" {
		// This is a function, dump its bytecode
		fmt.Printf("\n=== Function Bytecode: %s ===\n", c.compilingFuncName)
		fmt.Print(c.chunk.DisassembleChunk(c.compilingFuncName))
		fmt.Printf("=== END %s ===\n\n", c.compilingFuncName)
	}
	// <<< END ADDED >>>

	// Return the chunk (even if errors occurred, it might be partially useful for debugging?)
	// and the collected errors.
	return c.chunk, c.errors
}

// compileNode dispatches compilation to the appropriate method based on node type.
func (c *Compiler) compileNode(node parser.Node, hint Register) (Register, errors.PaseratiError) {
	// Safety check for nil checker, although it should be set by Compile()
	if c.typeChecker == nil && c.enclosing == nil { // Only panic if top-level compiler has no checker
		panic("Compiler internal error: typeChecker is nil during compileNode")
	}

	if c.line != parser.GetTokenFromNode(node).Line {
		c.line = parser.GetTokenFromNode(node).Line
		debugPrintf("// DEBUG compiling line %d (%s)\n", c.line, c.compilingFuncName)
	}

	// <<< ADDED: Enhanced node compilation logging >>>
	if debugCompiler {
		nodeType := fmt.Sprintf("%T", node)
		debugPrintf("// [NODE] Compiling %s at line %d, hint=R%d, nextReg=R%d\n",
			nodeType, c.line, hint, c.regAlloc.nextReg)
	}
	// <<< END ADDED >>>

	switch node := node.(type) {
	case *parser.Program:
		debugPrintf("// DEBUG Program: Starting statement loop.\n") // <<< ADDED
		for i, stmt := range node.Statements {
			debugPrintf("// DEBUG Program: Before compiling statement %d (%T).\n", i, stmt) // <<< ADDED
			tlReg, err := c.compileNode(stmt, hint)
			if err != nil {
				debugPrintf("// DEBUG Program: Error compiling statement %d: %v\n", i, err) // <<< ADDED
				return BadRegister, err                                                     // Propagate errors up
			}

			// <<< ADDED vvv
			if c.enclosing == nil {
				debugPrintf("// DEBUG Program: After compiling statement %d (%T). Result: R%d\n", i, stmt, tlReg)
				// For top level, be conservative - don't free registers between statements
				// The VM will handle cleanup when the program ends
			} else {
				// Inside function body - be more aggressive about freeing registers
				// But only free if we have more than a reasonable number allocated
				// DISABLED: Focus on expression-level freeing instead
				debugPrintf("// DEBUG Program: Inside function, but inter-statement freeing disabled\n")
			}
			// <<< ADDED ^^^
		}
		debugPrintf("// DEBUG Program: Finished statement loop. Final result: R%d\n", hint) // <<< ADDED
		return hint, nil                                                                    // Return the last meaningful result

	// --- NEW: Handle Function Literal as an EXPRESSION first ---
	// This handles anonymous/named functions used in assignments, arguments, etc.
	case *parser.FunctionLiteral:
		debugPrintf("// DEBUG Node-FunctionLiteral: Compiling function literal used as expression '%s'.\n", node.Name) // <<< DEBUG
		// Determine hint: empty for anonymous, Name.Value if named (though named exprs are rare)
		nameHint := ""
		if node.Name != nil {
			nameHint = node.Name.Value
		}
		// <<< MODIFY Call Site >>>
		funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(node, nameHint)
		if err != nil {
			// Error already added to c.errors by compileFunctionLiteral
			return BadRegister, nil // Return nil error here, main error is tracked
		}

		// Allocate register for the closure and emit OpClosure
		//closureReg := c.regAlloc.Alloc()
		c.emitClosure(hint, funcConstIndex, node, freeSymbols) // <<< Call emitClosure

		// emitClosure now handles setting lastExprReg/Valid correctly.
		return hint, nil // <<< Return nil error

	// --- NEW: Handle Shorthand Method as an EXPRESSION ---
	// This handles shorthand methods in object literals like { method() { ... } }
	case *parser.ShorthandMethod:
		debugPrintf("// DEBUG Node-ShorthandMethod: Compiling shorthand method '%s'.\n", node.Name.Value)
		// Shorthand methods are essentially function expressions with a known name
		nameHint := ""
		if node.Name != nil {
			nameHint = node.Name.Value
		}

		funcConstIndex, freeSymbols, err := c.compileShorthandMethod(node, nameHint)
		if err != nil {
			return BadRegister, err
		}

		// Allocate register for the closure and emit OpClosure using the generic emitClosureGeneric
		c.emitClosureGeneric(hint, funcConstIndex, node.Token.Line, node.Name, freeSymbols)

		return hint, nil

	// --- Block Statement (needed for function bodies) ---
	case *parser.BlockStatement:
		for _, stmt := range node.Statements {
			// For statements in blocks, allocate a temporary register if needed
			stmtReg := c.regAlloc.Alloc()
			_, err := c.compileNode(stmt, stmtReg)
			c.regAlloc.Free(stmtReg) // Free immediately since block statements don't return values
			if err != nil {
				return BadRegister, err // Propagate errors up
			}
		}
		return BadRegister, nil // ADDED: Explicit return

	// --- Statements ---
	case *parser.TypeAliasStatement: // Added
		// Type aliases only exist for type checking, ignore in compiler.
		return BadRegister, nil

	case *parser.InterfaceDeclaration: // Added
		// Interface declarations only exist for type checking, ignore in compiler.
		return BadRegister, nil

	case *parser.FunctionSignature: // Added

		return BadRegister, nil

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
				return BadRegister, nil
			}

			// --- Handle named function recursion ---
			// 1. Define the function name temporarily.
			c.currentSymbolTable.Define(funcLit.Name.Value, nilRegister)

			// 2. Compile the function literal body. Pass name as hint.
			// <<< MODIFY Call Site >>>
			funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, funcLit.Name.Value) // Use its own name as hint
			if err != nil {
				// Error already added to c.errors by compileFunctionLiteral
				return BadRegister, nil // Return nil error here, main error is tracked
			}

			// 3. Create the closure object in a register
			c.emitClosure(hint, funcConstIndex, funcLit, freeSymbols) // <<< Call emitClosure

			// 4. Update the symbol table entry with the register holding the closure.
			c.currentSymbolTable.UpdateRegister(funcLit.Name.Value, hint) // <<< Use closureReg

			// 5. If at top level, also set as global variable
			if c.enclosing == nil {
				globalIdx := c.GetOrAssignGlobalIndex(funcLit.Name.Value)
				c.emitSetGlobal(globalIdx, hint, funcLit.Token.Line)
				c.currentSymbolTable.DefineGlobal(funcLit.Name.Value, globalIdx)
			}

			// Function declarations don't produce a value for the script result
			return hint, nil // Handled
		}

		// Original ExpressionStatement logic for other expressions
		debugPrintf("// DEBUG ExprStmt: Compiling non-func-decl expression %T.\n", node.Expression)
		hint, err := c.compileNode(node.Expression, hint)
		if err != nil {
			return BadRegister, err
		}
		debugPrintf("// DEBUG ExprStmt: Top Level. CurrentReg: R%d.\n", hint) // <<< ADDED
		// Result register is left allocated, potentially unused otherwise.
		// TODO: Consider freeing registers?
		return hint, nil // ADDED: Explicit return

	case *parser.ClassDeclaration:
		return c.compileClassDeclaration(node, hint)

	case *parser.ClassExpression:
		// Convert ClassExpression to ClassDeclaration for compilation
		// The compiler treats them the same way
		if node.Name == nil {
			// Anonymous classes not yet supported
			return BadRegister, NewCompileError(node, "anonymous classes are not yet supported")
		}
		classDecl := &parser.ClassDeclaration{
			Token:          node.Token,
			Name:           node.Name,
			TypeParameters: node.TypeParameters,
			SuperClass:     node.SuperClass,
			Implements:     node.Implements,
			Body:           node.Body,
			IsAbstract:     node.IsAbstract,
		}
		return c.compileClassDeclaration(classDecl, hint)

	case *parser.LetStatement:
		return c.compileLetStatement(node, hint) // TODO: Fix this

	case *parser.ConstStatement:
		return c.compileConstStatement(node, hint) // TODO: Fix this

	case *parser.ArrayDestructuringDeclaration:
		return c.compileArrayDestructuringDeclaration(node, hint)

	case *parser.ObjectDestructuringDeclaration:
		return c.compileObjectDestructuringDeclaration(node, hint)

	case *parser.ReturnStatement: // Although less relevant for top-level script return
		return c.compileReturnStatement(node, hint) // TODO: Fix this

	case *parser.WhileStatement:
		return c.compileWhileStatement(node, hint) // TODO: Fix this

	case *parser.ForStatement:
		return c.compileForStatement(node, hint) // TODO: Fix this

	case *parser.ForOfStatement:
		return c.compileForOfStatement(node, hint) // TODO: Fix this

	case *parser.ForInStatement:
		return c.compileForInStatement(node, hint)

	case *parser.BreakStatement:
		return c.compileBreakStatement(node, hint) // TODO: Fix this

	case *parser.ContinueStatement:
		return c.compileContinueStatement(node, hint) // TODO: Fix this

	case *parser.DoWhileStatement:
		return c.compileDoWhileStatement(node, hint) // TODO: Fix this

	case *parser.SwitchStatement: // Added
		return c.compileSwitchStatement(node, hint) // TODO: Fix this

	// --- Exception Handling Statements ---
	case *parser.TryStatement:
		return c.compileTryStatement(node, hint)

	case *parser.ThrowStatement:
		return c.compileThrowStatement(node, hint)

	// --- Module Statements ---
	case *parser.ImportDeclaration:
		return c.compileImportDeclaration(node, hint)

	case *parser.ExportNamedDeclaration:
		return c.compileExportNamedDeclaration(node, hint)

	case *parser.ExportDefaultDeclaration:
		return c.compileExportDefaultDeclaration(node, hint)

	case *parser.ExportAllDeclaration:
		return c.compileExportAllDeclaration(node, hint)

	// --- Expressions (excluding FunctionLiteral which is handled above) ---
	case *parser.NumberLiteral:
		//fmt.Printf("[NUMBER LITERAL DEBUG] Compiling NumberLiteral value=%f with hint=R%d\n", node.Value, hint)
		c.emitLoadNewConstant(hint, vm.Number(node.Value), node.Token.Line)
		return hint, nil // ADDED: Explicit return

	case *parser.StringLiteral:
		// Handle string literals by adding them to constants
		c.emitLoadNewConstant(hint, vm.String(node.Value), node.Token.Line)
		return hint, nil

	case *parser.TemplateLiteral:
		return c.compileTemplateLiteral(node, hint) // TODO: Fix this

	case *parser.BooleanLiteral:
		// Handle boolean literals by using appropriate opcode
		if node.Value {
			c.emitLoadTrue(hint, node.Token.Line)
		} else {
			c.emitLoadFalse(hint, node.Token.Line)
		}
		return hint, nil

	case *parser.NullLiteral:
		c.emitLoadNull(hint, node.Token.Line)
		return hint, nil // ADDED: Explicit return

	case *parser.UndefinedLiteral: // Added
		c.emitLoadUndefined(hint, node.Token.Line)
		return hint, nil // ADDED: Explicit return

	case *parser.RegexLiteral: // Added for regex literals
		// Create a RegExp object from pattern and flags
		regexValue, err := vm.NewRegExp(node.Pattern, node.Flags)
		if err != nil {
			return BadRegister, NewCompileError(node, fmt.Sprintf("Invalid regex: %s", err.Error()))
		}
		c.emitLoadNewConstant(hint, regexValue, node.Token.Line)
		return hint, nil

	case *parser.ThisExpression: // Added for this keyword
		// Load 'this' value from current call context
		c.emitLoadThis(hint, node.Token.Line)
		return hint, nil

	case *parser.SuperExpression: // Added for super expressions
		// Load 'super' value - for now, this is the same as 'this' since we use prototype-based inheritance
		// TODO: Implement proper super method binding when needed
		c.emitLoadThis(hint, node.Token.Line)
		return hint, nil

	case *parser.Identifier:
		// All identifiers (including builtins) now use standard variable lookup
		// Builtins are registered as global variables via the new initializer system
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
			globalIdx := c.GetOrAssignGlobalIndex(node.Value)
			c.emitGetGlobal(hint, globalIdx, node.Token.Line)
			return hint, nil // Handle as global access
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
			c.emitOpCode(vm.OpLoadFree, node.Token.Line)
			c.emitByte(byte(hint))
			c.emitByte(byte(freeVarIndex))
		} else if symbolRef.IsGlobal {
			// This is a global variable, use OpGetGlobal
			debugPrintf("// DEBUG Identifier '%s': GLOBAL variable, using OpGetGlobal\n", node.Value) // <<< ADDED
			c.emitGetGlobal(hint, symbolRef.GlobalIndex, node.Token.Line)
		} else if !isLocal {
			debugPrintf("// DEBUG Identifier '%s': NOT LOCAL, treating as FREE VARIABLE\n", node.Value) // <<< ADDED
			// This is a regular free variable (defined in an outer scope that's not global)
			freeVarIndex := c.addFreeSymbol(node, &symbolRef)
			c.emitOpCode(vm.OpLoadFree, node.Token.Line)
			c.emitByte(byte(hint))
			c.emitByte(byte(freeVarIndex))
		} else {
			debugPrintf("// DEBUG Identifier '%s': LOCAL variable, register=R%d\n", node.Value, symbolRef.Register) // <<< ADDED
			// This is a standard local variable (handled by current stack frame)
			srcReg := symbolRef.Register
			debugPrintf("// DEBUG Identifier '%s': Resolved to isLocal=%v, srcReg=R%d\n", node.Value, isLocal, srcReg)
			if srcReg == nilRegister {
				// Check if this is an imported identifier
				if c.IsModuleMode() && c.moduleBindings.IsImported(node.Value) {
					debugPrintf("// DEBUG Identifier '%s': This is an imported name, generating runtime import resolution\n", node.Value)
					// Generate code to resolve the import at runtime
					c.emitImportResolve(hint, node.Value, node.Token.Line)
				} else {
					// This panic indicates an internal logic error, like trying to use a variable
					// during its temporary definition phase inappropriately.
					panic(fmt.Sprintf("compiler internal error: resolved local variable '%s' to nilRegister R%d unexpectedly at line %d", node.Value, srcReg, node.Token.Line))
				}
			} else {
				debugPrintf("// DEBUG Identifier '%s': About to emit Move R%d (dest), R%d (src)\n", node.Value, hint, srcReg)
				c.emitMove(hint, srcReg, node.Token.Line)
			}
		}
		return hint, nil // ADDED: Explicit return

	case *parser.PrefixExpression:
		return c.compilePrefixExpression(node, hint) // TODO: Fix this

	case *parser.TypeofExpression:
		return c.compileTypeofExpression(node, hint) // TODO: Fix this

	case *parser.TypeAssertionExpression:
		return c.compileTypeAssertionExpression(node, hint)

	case *parser.InfixExpression:
		return c.compileInfixExpression(node, hint) // TODO: Fix this

	case *parser.ArrowFunctionLiteral: // Keep this separate
		return c.compileArrowFunctionLiteral(node, hint) // TODO: Fix this

	case *parser.CallExpression:
		return c.compileCallExpression(node, hint) // TODO: Fix this

	case *parser.IfExpression:
		return c.compileIfExpression(node, hint) // TODO: Fix this

	case *parser.IfStatement:
		// Handle if statements - reuse IfExpression compilation but ignore return value
		// Convert IfStatement to IfExpression for compilation
		ifExpr := &parser.IfExpression{
			Token:       node.Token,
			Condition:   node.Condition,
			Consequence: node.Consequence,
			Alternative: node.Alternative,
		}
		_, err := c.compileIfExpression(ifExpr, hint)
		return hint, err // IfStatement doesn't produce a value

	case *parser.TernaryExpression:
		return c.compileTernaryExpression(node, hint) // TODO: Fix this

	case *parser.AssignmentExpression:
		return c.compileAssignmentExpression(node, hint) // TODO: Fix this

	case *parser.ArrayDestructuringAssignment:
		return c.compileArrayDestructuringAssignment(node, hint)

	case *parser.ObjectDestructuringAssignment:
		return c.compileObjectDestructuringAssignment(node, hint)

	case *parser.UpdateExpression:
		return c.compileUpdateExpression(node, hint) // TODO: Fix this

	// --- NEW: Array/Index ---
	case *parser.ArrayLiteral:
		return c.compileArrayLiteral(node, hint) // TODO: Fix this
	case *parser.ObjectLiteral: // <<< NEW
		return c.compileObjectLiteral(node, hint) // TODO: Fix this
	case *parser.IndexExpression:
		return c.compileIndexExpression(node, hint) // TODO: Fix this
		// --- End Array/Index ---

		// --- Member Expression ---
	case *parser.MemberExpression:
		return c.compileMemberExpression(node, hint) // TODO: Fix this
		// --- END Member Expression ---

		// --- Optional Chaining Expression ---
	case *parser.OptionalChainingExpression:
		return c.compileOptionalChainingExpression(node, hint) // TODO: Fix this
		// --- END Optional Chaining Expression ---

	case *parser.NewExpression:
		return c.compileNewExpression(node, hint) // TODO: Fix this

	// --- NEW: Rest Parameters and Spread Syntax ---
	case *parser.SpreadElement:
		// SpreadElement can appear in function calls - this should be handled there
		// If we reach here, it's likely used in an invalid context
		return BadRegister, NewCompileError(node, "spread syntax not supported in this context")

	case *parser.RestParameter:
		// RestParameter should only appear in function parameter lists
		// If we reach here, it's likely used in an invalid context
		return BadRegister, NewCompileError(node, "rest parameter syntax not supported in this context")
	// --- END NEW ---

	default:
		// Add check here? If type is FunctionLiteral and wasn't caught above, it's an error.
		if _, ok := node.(*parser.FunctionLiteral); ok {
			return BadRegister, NewCompileError(node, "compiler internal error: FunctionLiteral fell through switch")
		}
		return BadRegister, NewCompileError(node, fmt.Sprintf("compilation not implemented for %T", node))
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
			defaultValueReg := functionCompiler.regAlloc.Alloc()
			_, err := functionCompiler.compileNode(param.DefaultValue, defaultValueReg)
			if err != nil {
				// Continue with compilation even if default value has errors
				functionCompiler.addError(param.DefaultValue, fmt.Sprintf("error compiling default value for parameter %s", param.Name.Value))
			} else {
				// Move the default value to the parameter register
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
	bodyReg := functionCompiler.regAlloc.Alloc()
	_, err := functionCompiler.compileNode(node.Body, bodyReg)
	functionCompiler.regAlloc.Free(bodyReg) // Free since function body doesn't return a value
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
	// Count parameters excluding 'this' parameters for arity calculation
	arity := 0
	for _, param := range node.Parameters {
		if !param.IsThis {
			arity++
		}
	}
	funcValue := vm.NewFunction(arity, len(freeSymbols), int(regSize), node.RestParameter != nil, funcName, functionChunk)
	constIdx := c.chunk.AddConstant(funcValue)

	return constIdx, freeSymbols, nil
}

// compileArgumentsWithOptionalHandling compiles the provided arguments and pads missing optional
// parameters with undefined values. Uses contiguous allocation to place arguments in their final positions.
func (c *Compiler) compileArgumentsWithOptionalHandling(node *parser.CallExpression, firstTargetReg Register) ([]Register, int, errors.PaseratiError) {
	// 1. Determine the expected argument count including optional parameters
	providedArgCount := len(node.Arguments)

	// Get function type to check for optional parameters
	functionType := node.Function.GetComputedType()
	var expectedParamCount int
	var optionalParams []bool

	if functionType != nil {
		if objType, ok := functionType.(*types.ObjectType); ok && objType.IsCallable() && len(objType.CallSignatures) > 0 {
			// TODO: This is a temporary solution. The checker should resolve overloads during type checking
			// and attach the specific selected signature to the call expression.
			// For now, try to pick the best matching signature based on argument count
			sig := objType.CallSignatures[0] // Default to first signature
			bestMatch := sig
			bestScore := -1

			for _, candidateSig := range objType.CallSignatures {
				score := 0
				// Prefer exact parameter count match
				if len(candidateSig.ParameterTypes) == providedArgCount {
					score += 10
				}
				// Or compatible with optional parameters
				requiredParams := 0
				for i, isOptional := range candidateSig.OptionalParams {
					if i < len(candidateSig.ParameterTypes) && !isOptional {
						requiredParams++
					}
				}
				if providedArgCount >= requiredParams && providedArgCount <= len(candidateSig.ParameterTypes) {
					score += 5
				}

				if score > bestScore {
					bestScore = score
					bestMatch = candidateSig
				}
			}

			expectedParamCount = len(bestMatch.ParameterTypes)
			optionalParams = bestMatch.OptionalParams
		}
	}

	// Determine final argument count (provided args + undefined padding for optional params)
	finalArgCount := providedArgCount
	if len(optionalParams) == expectedParamCount && providedArgCount < expectedParamCount {
		// Count how many optional parameters we need to pad
		for i := providedArgCount; i < expectedParamCount; i++ {
			if i < len(optionalParams) && optionalParams[i] {
				finalArgCount++
			} else {
				break // Stop at first required parameter
			}
		}
	}

	// 2. Build register list for arguments without reserving them
	// The hint-based compilation will handle register allocation correctly
	var argRegs []Register
	if finalArgCount > 0 {
		for i := 0; i < finalArgCount; i++ {
			targetReg := firstTargetReg + Register(i)
			argRegs = append(argRegs, targetReg)
		}
	}

	// 3. Compile provided arguments, handling spread syntax
	effectiveArgIndex := 0
	for _, arg := range node.Arguments {
		if spreadElement, isSpread := arg.(*parser.SpreadElement); isSpread {
			// For now, spread syntax should be handled by the new spread call instructions
			// This path should not be reached with proper compiler flow
			return nil, 0, NewCompileError(spreadElement, "spread syntax should be handled by spread call instructions")
		} else {
			// Regular argument - compile directly into its target register
			if effectiveArgIndex >= len(argRegs) {
				return nil, 0, NewCompileError(arg, "too many arguments provided")
			}
			targetReg := argRegs[effectiveArgIndex]
			_, err := c.compileNode(arg, targetReg)
			if err != nil {
				return nil, 0, err
			}
			effectiveArgIndex++
		}
	}

	// 4. Pad missing optional parameters with undefined
	for i := providedArgCount; i < finalArgCount; i++ {
		targetReg := argRegs[i]
		c.emitLoadUndefined(targetReg, node.Token.Line)
	}

	return argRegs, finalArgCount, nil
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
	if op == vm.OpJumpIfFalse || op == vm.OpJumpIfUndefined {
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
	if op == vm.OpJumpIfFalse || op == vm.OpJumpIfUndefined {
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
	token := parser.GetTokenFromNode(node)
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
	token := parser.GetTokenFromNode(node)
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
func (c *Compiler) emitClosure(destReg Register, funcConstIndex uint16, node *parser.FunctionLiteral, freeSymbols []*Symbol) Register {
	line := node.Token.Line // Use function literal token line

	// OPTIMIZATION: If no upvalues, just load the function constant
	if len(freeSymbols) == 0 {
		debugPrintf("// [emitClosure OPTIMIZED] No upvalues, using OpLoadConstant instead of OpClosure\n")
		c.emitLoadConstant(destReg, funcConstIndex, line)
		return destReg
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

	debugPrintf("// [emitClosure %s] Closure emitted to R%d. Set lastExprReg/Valid.\n", funcNameForLookup, destReg)
	return destReg
}

// emitClosureGeneric is a generic version of emitClosure that works with any node type
// that has Token.Line and Name fields (like ShorthandMethod)
// OPTIMIZATION: If there are no upvalues, just load the function constant directly.
func (c *Compiler) emitClosureGeneric(destReg Register, funcConstIndex uint16, line int, nameNode *parser.Identifier, freeSymbols []*Symbol) Register {
	// OPTIMIZATION: If no upvalues, just load the function constant
	if len(freeSymbols) == 0 {
		debugPrintf("// [emitClosureGeneric OPTIMIZED] No upvalues, using OpLoadConstant instead of OpClosure\n")
		c.emitLoadConstant(destReg, funcConstIndex, line)
		return destReg
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

	debugPrintf("// [emitClosureGeneric %s] Closure emitted to R%d. Set lastExprReg/Valid.\n", funcNameForLookup, destReg)
	return destReg
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
		targetReg := moves[cycle[i]] // Fix: use moves[cycle[i]], not moves[cycle[i+1]]
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

// GetOrAssignGlobalIndex returns the index for a global variable name.
// If the variable doesn't have an index yet, assigns a new one.
// This function should only be called on the top-level compiler.
func (c *Compiler) GetOrAssignGlobalIndex(name string) uint16 {
	// Only top-level compiler should manage global indices
	topCompiler := c
	for topCompiler.enclosing != nil {
		topCompiler = topCompiler.enclosing
	}

	if idx, exists := topCompiler.globalIndices[name]; exists {
		// debugPrintf("// [Compiler] GetOrAssignGlobalIndex: '%s' already has index %d\n", name, idx)
		return uint16(idx)
	}

	// Assign new index
	idx := topCompiler.globalCount
	topCompiler.globalIndices[name] = idx
	topCompiler.globalCount++
	// debugPrintf("// [Compiler] GetOrAssignGlobalIndex: Assigned new index %d to '%s' (total globals: %d)\n", idx, name, topCompiler.globalCount)

	if idx > 65535 {
		panic(fmt.Sprintf("Too many global variables (max 65536): %s", name))
	}

	return uint16(idx)
}

// GetGlobalNames returns a slice of all global variable names that have been assigned indices
func (c *Compiler) GetGlobalNames() []string {
	// Only top-level compiler should manage global indices
	topCompiler := c
	for topCompiler.enclosing != nil {
		topCompiler = topCompiler.enclosing
	}

	names := make([]string, 0, len(topCompiler.globalIndices))
	for name := range topCompiler.globalIndices {
		names = append(names, name)
	}
	return names
}

// GetGlobalIndex returns the index for a global variable name, or -1 if not found
func (c *Compiler) GetGlobalIndex(name string) int {
	// Only top-level compiler should manage global indices
	topCompiler := c
	for topCompiler.enclosing != nil {
		topCompiler = topCompiler.enclosing
	}

	if idx, exists := topCompiler.globalIndices[name]; exists {
		return idx
	}
	return -1
}

// SetGlobalIndex forces a global variable name to use a specific index
// This is used to coordinate global indices between different compiler instances
func (c *Compiler) SetGlobalIndex(name string, index int) {
	// Only top-level compiler should manage global indices
	topCompiler := c
	for topCompiler.enclosing != nil {
		topCompiler = topCompiler.enclosing
	}

	topCompiler.globalIndices[name] = index
	// Update global count if this index is beyond current count
	if index >= topCompiler.globalCount {
		topCompiler.globalCount = index + 1
	}
	// debugPrintf("// [Compiler] SetGlobalIndex: Forced '%s' to index %d\n", name, index)
}

// GetExportGlobalIndices returns a mapping from export names to their global indices
// This allows the VM to efficiently collect export values from the global table
func (c *Compiler) GetExportGlobalIndices() map[string]int {
	if !c.IsModuleMode() {
		return make(map[string]int)
	}

	exportIndices := make(map[string]int)
	
	// Get all exported names from module bindings
	for exportName := range c.moduleBindings.ExportedNames {
		// The export name should correspond to the same name in globals
		// (exports are stored as globals with the same name)
		if globalIdx := c.GetGlobalIndex(exportName); globalIdx >= 0 {
			exportIndices[exportName] = globalIdx
			// debugPrintf("// [Compiler] GetExportGlobalIndices: Export '%s' maps to global[%d]\n", exportName, globalIdx)
		}
	}

	// Handle default export if it exists
	if c.moduleBindings.DefaultExport != nil {
		defaultLocalName := c.moduleBindings.DefaultExport.LocalName
		if globalIdx := c.GetGlobalIndex(defaultLocalName); globalIdx >= 0 {
			exportIndices["default"] = globalIdx
			debugPrintf("// [Compiler] GetExportGlobalIndices: Default export '%s' maps to global[%d]\n", defaultLocalName, globalIdx)
		}
	}

	return exportIndices
}

// hasMethodInType checks if an object type has a method with the given name
// This includes checking the object's own properties and any prototype chain
func (c *Compiler) hasMethodInType(objType *types.ObjectType, methodName string) bool {
	// Check own properties
	if _, exists := objType.Properties[methodName]; exists {
		return true
	}

	// Check in base types (prototypes)
	for _, baseType := range objType.BaseTypes {
		if baseObjType, ok := baseType.(*types.ObjectType); ok {
			if c.hasMethodInType(baseObjType, methodName) {
				return true
			}
		}
	}

	return false
}

// --- Module Compilation Methods ---

// compileImportDeclaration handles compilation of import statements
// Following the same pattern as type checker's checkImportDeclaration
func (c *Compiler) compileImportDeclaration(node *parser.ImportDeclaration, hint Register) (Register, errors.PaseratiError) {
	// Type-only imports are handled during type checking only - no runtime code needed
	if node.IsTypeOnly {
		debugPrintf("// [Compiler] Skipping type-only import from: %s\n", node.Source.Value)
		return BadRegister, nil
	}

	if node.Source == nil {
		return BadRegister, NewCompileError(node, "import statement missing source module")
	}

	sourceModulePath := node.Source.Value
	debugPrintf("// [Compiler] Processing import from: %s\n", sourceModulePath)
	
	// Handle bare imports (side-effect only)
	if len(node.Specifiers) == 0 {
		debugPrintf("// [Compiler] Bare import (side effects only): %s\n", sourceModulePath)
		// For bare imports, we just need to ensure the module was loaded
		// No bindings are created in the local environment
		return BadRegister, nil
	}
	
	// Process import specifiers and create bindings
	for _, spec := range node.Specifiers {
		switch importSpec := spec.(type) {
		case *parser.ImportDefaultSpecifier:
			// Default import: import defaultName from "module"
			if importSpec.Local == nil {
				return BadRegister, NewCompileError(node, "import default specifier missing local name")
			}
			
			localName := importSpec.Local.Value
			c.processImportBinding(localName, sourceModulePath, "default", ImportDefaultRef)
			
		case *parser.ImportNamedSpecifier:
			// Named import: import { name } or import { name as alias }
			if importSpec.Local == nil || importSpec.Imported == nil {
				return BadRegister, NewCompileError(node, "import named specifier missing names")
			}
			
			localName := importSpec.Local.Value
			importedName := importSpec.Imported.Value
			c.processImportBinding(localName, sourceModulePath, importedName, ImportNamedRef)
			
		case *parser.ImportNamespaceSpecifier:
			// Namespace import: import * as name from "module"
			if importSpec.Local == nil {
				return BadRegister, NewCompileError(node, "import namespace specifier missing local name")
			}
			
			localName := importSpec.Local.Value
			c.processImportBinding(localName, sourceModulePath, "*", ImportNamespaceRef)
			
		default:
			return BadRegister, NewCompileError(node, fmt.Sprintf("unknown import specifier type: %T", spec))
		}
	}
	
	return BadRegister, nil
}

// processImportBinding handles the binding of an imported name
// Parallels type checker's processImportBinding
func (c *Compiler) processImportBinding(localName, sourceModule, sourceName string, importType ImportReferenceType) {
	// If we're in module mode, use the module bindings for proper tracking
	if c.IsModuleMode() {
		c.moduleBindings.DefineImport(localName, sourceModule, sourceName, importType)
		
		// Try to resolve the actual value from the source module
		resolvedValue := c.moduleBindings.ResolveImportedValue(localName)
		if resolvedValue != vm.Undefined {
			debugPrintf("// [Compiler] Imported %s: %s = %s (resolved)\n", localName, sourceName, resolvedValue.Type())
		} else {
			debugPrintf("// [Compiler] Imported %s: %s = undefined (unresolved)\n", localName, sourceName)
		}
		
		// Define the imported name in the symbol table
		// This allows the rest of the code to reference it normally
		c.currentSymbolTable.Define(localName, nilRegister) // Will be resolved at runtime
	} else {
		// Fallback: just define the name in the symbol table
		c.currentSymbolTable.Define(localName, nilRegister)
		debugPrintf("// [Compiler] Imported %s: %s = undefined (no module mode)\n", localName, sourceName)
	}
}

// compileExportNamedDeclaration handles compilation of named export statements
// Parallels type checker's checkExportNamedDeclaration
func (c *Compiler) compileExportNamedDeclaration(node *parser.ExportNamedDeclaration, hint Register) (Register, errors.PaseratiError) {
	// Type-only exports are handled during type checking only - no runtime code needed
	if node.IsTypeOnly {
		debugPrintf("// [Compiler] Skipping type-only export\n")
		return BadRegister, nil
	}

	if node.Declaration != nil {
		// Direct export: export const x = 1; export function foo() {}
		debugPrintf("// [Compiler] Processing direct export declaration\n")
		
		// Compile the declaration first
		_, err := c.compileNode(node.Declaration, hint)
		if err != nil {
			return BadRegister, err
		}
		
		// Extract and register exported names
		c.processExportDeclaration(node.Declaration)
		
	} else if len(node.Specifiers) > 0 {
		// Named exports: export { x, y } or export { x } from "module"
		debugPrintf("// [Compiler] Processing named export specifiers\n")
		
		if node.Source != nil {
			// Re-export: export { x } from "module"
			sourceModule := node.Source.Value
			debugPrintf("// [Compiler] Re-export from: %s\n", sourceModule)
			
			for _, spec := range node.Specifiers {
				if exportSpec, ok := spec.(*parser.ExportNamedSpecifier); ok {
					localName := exportSpec.Local.Value
					exportName := exportSpec.Exported.Value
					
					if c.IsModuleMode() {
						c.moduleBindings.DefineReExport(exportName, sourceModule, localName)
						debugPrintf("// [Compiler] Re-exported: %s as %s from %s\n", localName, exportName, sourceModule)
					}
				}
			}
		} else {
			// Local export: export { x, y }
			// Validate that the exported names exist and register them
			for _, spec := range node.Specifiers {
				if exportSpec, ok := spec.(*parser.ExportNamedSpecifier); ok {
					if exportSpec.Local == nil {
						return BadRegister, NewCompileError(node, "export specifier missing local name")
					}
					
					localName := exportSpec.Local.Value
					exportName := exportSpec.Exported.Value
					
					// Check if the local name exists in current symbol table
					if _, _, exists := c.currentSymbolTable.Resolve(localName); exists {
						// Register the export in module bindings
						if c.IsModuleMode() {
							// For now, we can't get the runtime value until execution
							// So we register it with undefined and resolve later
							c.moduleBindings.DefineExport(localName, exportName, vm.Undefined, nil)
						}
						debugPrintf("// [Compiler] Exported: %s as %s\n", localName, exportName)
					} else {
						return BadRegister, NewCompileError(node, fmt.Sprintf("exported name '%s' not found in current scope", localName))
					}
				}
			}
		}
	}
	
	return BadRegister, nil
}

// compileExportDefaultDeclaration handles compilation of default export statements
// Parallels type checker's checkExportDefaultDeclaration
func (c *Compiler) compileExportDefaultDeclaration(node *parser.ExportDefaultDeclaration, hint Register) (Register, errors.PaseratiError) {
	if node.Declaration == nil {
		return BadRegister, NewCompileError(node, "export default statement missing declaration")
	}
	
	debugPrintf("// [Compiler] Processing default export\n")
	
	// Compile the default export expression
	resultReg, err := c.compileNode(node.Declaration, hint)
	if err != nil {
		return BadRegister, err
	}
	
	// Register the default export
	if c.IsModuleMode() {
		// For now, we can't get the runtime value until execution
		// So we register it with undefined and resolve later
		c.moduleBindings.DefineExport("default", "default", vm.Undefined, nil)
		debugPrintf("// [Compiler] Default export registered\n")
	} else {
		debugPrintf("// [Compiler] Default export processed (no module mode)\n")
	}
	
	return resultReg, nil
}

// compileExportAllDeclaration handles compilation of export all statements
// Transforms "export * from './module'" into equivalent individual exports
func (c *Compiler) compileExportAllDeclaration(node *parser.ExportAllDeclaration, hint Register) (Register, errors.PaseratiError) {
	if node.Source == nil {
		return BadRegister, NewCompileError(node, "export * statement missing source module")
	}
	
	sourceModule := node.Source.Value
	debugPrintf("// [Compiler] Processing export * from: %s\n", sourceModule)
	
	if !c.IsModuleMode() {
		debugPrintf("// [Compiler] Not in module mode, skipping re-export\n")
		return BadRegister, nil
	}
	
	// Get the source module to find its exports
	if c.moduleLoader == nil {
		debugPrintf("// [Compiler] No module loader available for re-export\n")
		return BadRegister, nil
	}
	
	sourceModuleRecord, err := c.moduleLoader.LoadModule(sourceModule, ".")
	if err != nil {
		return BadRegister, NewCompileError(node, fmt.Sprintf("Failed to load source module '%s' for re-export: %v", sourceModule, err))
	}
	
	// Get export names from the source module
	// Try runtime values first, then fall back to export names
	sourceExports := sourceModuleRecord.GetExportValues()
	exportNames := sourceModuleRecord.GetExportNames()
	
	debugPrintf("// [Compiler] Source module '%s' has %d runtime exports and %d export names\n", sourceModule, len(sourceExports), len(exportNames))
	
	// If we have runtime exports, use those names (most reliable)
	if len(sourceExports) > 0 {
		exportNames = nil // Clear the names array
		for exportName := range sourceExports {
			exportNames = append(exportNames, exportName)
		}
		debugPrintf("// [Compiler] Using runtime export names: %v\n", exportNames)
	} else if len(exportNames) > 0 {
		debugPrintf("// [Compiler] Using export names from module record: %v\n", exportNames)
	} else {
		debugPrintf("// [Compiler] No exports found in source module\n")
	}
	
	debugPrintf("// [Compiler] Will re-export %d names from '%s'\n", len(exportNames), sourceModule)
	
	// For each export in the source module, create an equivalent re-export
	// This transforms "export * from './math'" into:
	// 1. Generate import resolution for each export
	// 2. Store each imported value as a global (like normal exports do)
	for _, exportName := range exportNames {
		// Skip default exports for "export *" (TypeScript behavior)
		if exportName == "default" {
			debugPrintf("// [Compiler] Skipping default export '%s' in re-export all\n", exportName)
			continue
		}
		
		debugPrintf("// [Compiler] Re-exporting '%s' from '%s'\n", exportName, sourceModule)
		
		// 1. Define the import binding (like processImportDeclaration does)
		c.moduleBindings.DefineImport(exportName, sourceModule, exportName, ImportNamedRef)
		
		// 2. Define the export binding (like processExportDeclaration does)  
		c.moduleBindings.DefineExport(exportName, exportName, vm.Undefined, nil)
		
		// 3. Generate bytecode to import and re-export the value
		// Allocate a temporary register for the imported value
		tempReg := c.regAlloc.Alloc()
		
		// Generate import resolution (OpEvalModule + OpGetModuleExport)
		c.emitImportResolve(tempReg, exportName, node.Token.Line)
		
		// Store the imported value as a global (like normal exports do)
		globalIdx := c.GetOrAssignGlobalIndex(exportName)
		c.emitSetGlobal(globalIdx, tempReg, node.Token.Line)
		
		debugPrintf("// [Compiler] Stored re-exported '%s' as global at index %d\n", exportName, globalIdx)
		
		// Free the temporary register
		c.regAlloc.Free(tempReg)
	}
	
	debugPrintf("// [Compiler] Completed re-export transformation for %d exports\n", len(exportNames))
	return BadRegister, nil
}

// extractExportNamesFromAST extracts export names from a module's AST
// This is used when runtime export values are not yet available
func (c *Compiler) extractExportNamesFromAST(program *parser.Program) []string {
	var exportNames []string
	
	if program == nil || program.Statements == nil {
		return exportNames
	}
	
	for _, stmt := range program.Statements {
		switch node := stmt.(type) {
		case *parser.ExportNamedDeclaration:
			if node.Declaration != nil {
				// Direct export: export const x = 1; export function foo() {}
				switch decl := node.Declaration.(type) {
				case *parser.LetStatement:
					if decl.Name != nil {
						exportNames = append(exportNames, decl.Name.Value)
					}
				case *parser.ConstStatement:
					if decl.Name != nil {
						exportNames = append(exportNames, decl.Name.Value)
					}
				case *parser.ExpressionStatement:
					// Handle function declarations: export function foo() {}
					if expr, ok := decl.Expression.(*parser.FunctionLiteral); ok && expr.Name != nil {
						exportNames = append(exportNames, expr.Name.Value)
					}
				}
			} else if len(node.Specifiers) > 0 {
				// Named exports: export { x, y }
				for _, spec := range node.Specifiers {
					if exportSpec, ok := spec.(*parser.ExportNamedSpecifier); ok {
						if exportSpec.Local != nil {
							// Use the export name (or local name if no alias)
							exportName := exportSpec.Local.Value
							if exportSpec.Exported != nil {
								exportName = exportSpec.Exported.Value
							}
							exportNames = append(exportNames, exportName)
						}
					}
				}
			}
		case *parser.ExportDefaultDeclaration:
			exportNames = append(exportNames, "default")
		case *parser.ExportAllDeclaration:
			// Note: export * from "module" doesn't add any names directly
			// The names come from the source module
		}
	}
	
	return exportNames
}

// processExportDeclaration processes a declaration that's being exported directly
// Parallels type checker's processExportDeclaration
func (c *Compiler) processExportDeclaration(decl parser.Statement) {
	switch d := decl.(type) {
	case *parser.LetStatement:
		if c.IsModuleMode() {
			if d.Name != nil {
				c.moduleBindings.DefineExport(d.Name.Value, d.Name.Value, vm.Undefined, d)
				debugPrintf("// [Compiler] Exported let: %s\n", d.Name.Value)
				// TODO: Generate bytecode to store the value in export table at runtime
			}
		}
		
	case *parser.ConstStatement:
		if c.IsModuleMode() {
			if d.Name != nil {
				c.moduleBindings.DefineExport(d.Name.Value, d.Name.Value, vm.Undefined, d)
				debugPrintf("// [Compiler] Exported const: %s\n", d.Name.Value)
				// TODO: Generate bytecode to store the value in export table at runtime
			}
		}
		
	case *parser.ExpressionStatement:
		// Handle function declarations in expression statements
		if expr, ok := d.Expression.(*parser.FunctionLiteral); ok && expr.Name != nil {
			if c.IsModuleMode() {
				c.moduleBindings.DefineExport(expr.Name.Value, expr.Name.Value, vm.Undefined, d)
				debugPrintf("// [Compiler] Exported function: %s\n", expr.Name.Value)
				// TODO: Generate bytecode to store the value in export table at runtime
			}
		}
		
	default:
		debugPrintf("// [Compiler] Unhandled export declaration type: %T\n", decl)
	}
}

// emitImportResolve generates code to resolve an imported name at runtime
// This generates OpEvalModule to execute the source module and make imports available
func (c *Compiler) emitImportResolve(destReg Register, importName string, line int) {
	if !c.IsModuleMode() {
		debugPrintf("// [Compiler] emitImportResolve: Not in module mode, loading undefined for '%s'\n", importName)
		c.emitLoadUndefined(destReg, line)
		return
	}
	
	// Get the import reference for this name
	if importRef, exists := c.moduleBindings.ImportedNames[importName]; exists {
		debugPrintf("// [Compiler] emitImportResolve: Generating OpEvalModule for '%s' from module '%s'\n", 
			importName, importRef.SourceModule)
		
		// Generate OpEvalModule to execute the source module
		c.emitEvalModule(importRef.SourceModule, line)
		
		// Generate OpGetModuleExport to get the exported value
		c.emitGetModuleExport(destReg, importRef.SourceModule, importRef.SourceName, line)
	} else {
		debugPrintf("// [Compiler] emitImportResolve: No import binding found for '%s', loading undefined\n", importName)
		c.emitLoadUndefined(destReg, line)
	}
}

// emitEvalModule generates OpEvalModule bytecode to execute a module
func (c *Compiler) emitEvalModule(modulePath string, line int) {
	// Add module path as a string constant
	modulePathValue := vm.String(modulePath)
	constantIdx := c.chunk.AddConstant(modulePathValue)
	
	debugPrintf("// [Compiler] emitEvalModule: Emitting OpEvalModule for '%s' (constantIdx: %d)\n", modulePath, constantIdx)
	
	// Emit OpEvalModule with the constant index
	c.emitOpCode(vm.OpEvalModule, line)
	c.emitByte(byte(constantIdx >> 8))   // High byte
	c.emitByte(byte(constantIdx & 0xFF)) // Low byte
}

// emitGetModuleExport generates OpGetModuleExport bytecode to get an exported value
func (c *Compiler) emitGetModuleExport(destReg Register, modulePath string, exportName string, line int) {
	// Add module path as a string constant
	modulePathValue := vm.String(modulePath)
	modulePathIdx := c.chunk.AddConstant(modulePathValue)
	
	// Add export name as a string constant
	exportNameValue := vm.String(exportName)
	exportNameIdx := c.chunk.AddConstant(exportNameValue)
	
	debugPrintf("// [Compiler] emitGetModuleExport: R%d = module['%s']['%s']\n", destReg, modulePath, exportName)
	
	// Emit OpGetModuleExport with register and two constant indices
	c.emitOpCode(vm.OpGetModuleExport, line)
	c.emitByte(byte(destReg))
	c.emitByte(byte(modulePathIdx >> 8))     // Module path high byte
	c.emitByte(byte(modulePathIdx & 0xFF))   // Module path low byte
	c.emitByte(byte(exportNameIdx >> 8))     // Export name high byte
	c.emitByte(byte(exportNameIdx & 0xFF))   // Export name low byte
}

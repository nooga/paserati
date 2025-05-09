package compiler

import (
	"fmt"
	"math"
	"paserati/pkg/checker"
	"paserati/pkg/errors"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

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

const debugCompiler = false
const debugCompilerStats = false

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
			}
			// <<< ADDED ^^^
		}
		debugPrintf("// DEBUG Program: Finished statement loop.\n") // <<< ADDED
		return nil                                                  // ADDED: Explicit return for Program case

	// --- NEW: Handle Function Literal as an EXPRESSION first ---
	// This handles anonymous/named functions used in assignments, arguments, etc.
	case *parser.FunctionLiteral:
		debugPrintf("// DEBUG Node-FunctionLiteral: Compiling function literal used as expression.\n")
		// Determine hint: empty for anonymous, Name.Value if named (though named exprs are rare)
		hint := ""
		if node.Name != nil {
			hint = node.Name.Value
		}
		err := c.compileFunctionLiteral(node, hint) // Directly compile it
		if err != nil {
			return err
		}
		// lastExprReg/Valid are set correctly by compileFunctionLiteral
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

	case *parser.ExpressionStatement:
		debugPrintf("// DEBUG ExprStmt: Compiling expression %T.\n", node.Expression)

		// Check specifically for NAMED function literals used as standalone statements.
		// Anonymous ones are handled by the case *parser.FunctionLiteral above now.
		if funcLit, ok := node.Expression.(*parser.FunctionLiteral); ok && funcLit.Name != nil {
			debugPrintf("// DEBUG ExprStmt: Handling NAMED function declaration '%s' as statement.\n", funcLit.Name.Value)
			// --- Handle named function recursion ---
			// 1. Define the function name temporarily.
			c.currentSymbolTable.Define(funcLit.Name.Value, nilRegister)
			// 2. Compile the function literal body. Pass name as hint.
			err := c.compileFunctionLiteral(funcLit, funcLit.Name.Value) // Use its own name as hint
			if err != nil {
				return err
			}
			valueReg := c.regAlloc.Current()
			// 3. Update the symbol table entry.
			c.currentSymbolTable.UpdateRegister(funcLit.Name.Value, valueReg)
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
		destReg := c.regAlloc.Alloc()
		c.emitLoadNewConstant(destReg, vm.String(node.Value), node.Token.Line)
		return nil // ADDED: Explicit return

	case *parser.BooleanLiteral:
		destReg := c.regAlloc.Alloc()
		if node.Value {
			c.emitLoadTrue(destReg, node.Token.Line)
		} else {
			c.emitLoadFalse(destReg, node.Token.Line)
		}
		return nil // ADDED: Explicit return

	case *parser.NullLiteral:
		destReg := c.regAlloc.Alloc()
		c.emitLoadNull(destReg, node.Token.Line)
		return nil // ADDED: Explicit return

	case *parser.UndefinedLiteral: // Added
		destReg := c.regAlloc.Alloc()
		c.emitLoadUndefined(destReg, node.Token.Line)
		return nil // ADDED: Explicit return

	case *parser.Identifier:
		scopeName := "Function"
		if c.currentSymbolTable.Outer == nil {
			scopeName = "Global"
		}
		symbolRef, definingTable, found := c.currentSymbolTable.Resolve(node.Value)
		if !found {
			return NewCompileError(node, fmt.Sprintf("undefined variable '%s'", node.Value))
		}
		isLocal := definingTable == c.currentSymbolTable

		// --- NEW RECURSION CHECK --- // Revised Check
		// Check if this is a recursive call identifier referencing the temp definition.
		isRecursiveSelfCall := isLocal &&
			symbolRef.Register == nilRegister && // Is it our temporary definition?
			scopeName == "Function" // Are we compiling inside a function? // Removed check against c.compilingFuncName

		if isRecursiveSelfCall {
			// Treat as a free variable that captures the closure itself.
			// This requires adding it to freeSymbols and emitting OpLoadFree.
			// The closure emission logic already handles the self-capture part
			// when it sees a free var matching funcName.
			freeVarIndex := c.addFreeSymbol(node, &symbolRef)
			destReg := c.regAlloc.Alloc()
			c.emitOpCode(vm.OpLoadFree, node.Token.Line)
			c.emitByte(byte(destReg))
			c.emitByte(byte(freeVarIndex))
		} else if !isLocal {
			// This is a regular free variable (defined in an outer scope)
			freeVarIndex := c.addFreeSymbol(node, &symbolRef)
			destReg := c.regAlloc.Alloc()
			c.emitOpCode(vm.OpLoadFree, node.Token.Line)
			c.emitByte(byte(destReg))
			c.emitByte(byte(freeVarIndex))
		} else {
			// This is a standard local or global variable (handled by current stack frame)
			srcReg := symbolRef.Register
			// <<< ADDED LOGGING >>> vvv
			debugPrintf("// DEBUG Identifier '%s': Resolved to isLocal=%v, srcReg=R%d\n", node.Value, isLocal, srcReg)
			if srcReg == nilRegister {
				panic(fmt.Sprintf("compiler internal error: resolved local variable '%s' to nilRegister R%d unexpectedly at line %d", node.Value, srcReg, node.Token.Line))
			}
			// <<< END LOGGING >>>
			destReg := c.regAlloc.Alloc()
			debugPrintf("// DEBUG Identifier '%s': About to emit Move R%d (dest), R%d (src)\n", node.Value, destReg, srcReg)
			c.emitMove(destReg, srcReg, node.Token.Line)
			c.regAlloc.SetCurrent(destReg)
		}
		return nil // ADDED: Explicit return

	case *parser.PrefixExpression:
		return c.compilePrefixExpression(node)

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

	default:
		// Add check here? If type is FunctionLiteral and wasn't caught above, it's an error.
		if _, ok := node.(*parser.FunctionLiteral); ok {
			return NewCompileError(node, "compiler internal error: FunctionLiteral fell through switch")
		}
		return NewCompileError(node, fmt.Sprintf("compilation not implemented for %T", node))
	}
	// REMOVED: unreachable return nil // Return nil on success if no specific error occurred in this frame
}

// --- Statement Compilation ---

// Define a placeholder register value for 'undefined' case
// Also used temporarily for recursive function definition
const nilRegister Register = 255 // Or another value guaranteed not to be used

func (c *Compiler) compileLetStatement(node *parser.LetStatement) errors.PaseratiError {
	var valueReg Register = nilRegister
	var err errors.PaseratiError
	isValueFunc := false // Flag to track if value is a function literal

	if funcLit, ok := node.Value.(*parser.FunctionLiteral); ok {
		isValueFunc = true
		// --- Handle let f = function g() {} or let f = function() {} ---
		// 1. Define the *variable name (f)* temporarily for potential recursion
		//    within the function body (e.g., recursive anonymous function).
		c.currentSymbolTable.Define(node.Name.Value, nilRegister)

		// 2. Compile the function literal body.
		//    Pass the variable name (f) as the hint for the function object's name
		//    if the function literal itself is anonymous.
		err = c.compileFunctionLiteral(funcLit, node.Name.Value)
		if err != nil {
			return err
		}
		valueReg = c.regAlloc.Current() // Register holding the closure

		// 3. Update the symbol table entry for the *variable name (f)*.
		c.currentSymbolTable.UpdateRegister(node.Name.Value, valueReg)

	} else if node.Value != nil {
		// Compile other value types normally
		err = c.compileNode(node.Value)
		if err != nil {
			return err
		}
		valueReg = c.regAlloc.Current()
	} // else: node.Value is nil (implicit undefined handled below)

	// Handle implicit undefined (`let x;`)
	if valueReg == nilRegister {
		undefReg := c.regAlloc.Alloc()
		c.emitLoadUndefined(undefReg, node.Name.Token.Line)
		valueReg = undefReg
		// Define symbol for the `let x;` case
		c.currentSymbolTable.Define(node.Name.Value, valueReg)
	} else if !isValueFunc {
		// Define symbol ONLY for non-function values.
		// Function assignments were handled above.
		c.currentSymbolTable.Define(node.Name.Value, valueReg)
	}

	return nil
}

func (c *Compiler) compileConstStatement(node *parser.ConstStatement) errors.PaseratiError {
	if node.Value == nil { /* ... error ... */
	}
	var valueReg Register = nilRegister
	var err errors.PaseratiError
	isValueFunc := false // Flag

	if funcLit, ok := node.Value.(*parser.FunctionLiteral); ok {
		isValueFunc = true
		// --- Handle const f = function g() {} or const f = function() {} ---
		// 1. Define the *const name (f)* temporarily for recursion.
		c.currentSymbolTable.Define(node.Name.Value, nilRegister)

		// 2. Compile the function literal body, passing const name as hint.
		err = c.compileFunctionLiteral(funcLit, node.Name.Value)
		if err != nil {
			return err
		}
		valueReg = c.regAlloc.Current()

		// 3. Update the temporary definition for the *const name (f)*.
		c.currentSymbolTable.UpdateRegister(node.Name.Value, valueReg)
	} else {
		// Compile other value types normally
		err = c.compileNode(node.Value)
		if err != nil {
			return err
		}
		valueReg = c.regAlloc.Current()
	}

	// Define symbol ONLY for non-function values.
	// Const function assignments were handled above.
	if !isValueFunc {
		c.currentSymbolTable.Define(node.Name.Value, valueReg)
	}
	return nil
}

func (c *Compiler) compileReturnStatement(node *parser.ReturnStatement) errors.PaseratiError {
	if node.ReturnValue != nil {
		var err errors.PaseratiError
		// Check if the return value is a function literal itself
		if funcLit, ok := node.ReturnValue.(*parser.FunctionLiteral); ok {
			// Compile directly, bypassing the compileNode case for declarations.
			// Pass empty hint as it's an anonymous function expression here.
			err = c.compileFunctionLiteral(funcLit, "")
		} else {
			// Compile other expression types normally via compileNode
			err = c.compileNode(node.ReturnValue)
		}

		if err != nil {
			return err
		}
		returnReg := c.regAlloc.Current() // Value to return is in the last allocated reg
		c.emitReturn(returnReg, node.Token.Line)
	} else {
		// Return undefined implicitly using the optimized opcode
		c.emitOpCode(vm.OpReturnUndefined, node.Token.Line)
	}
	return nil
}

// --- Expression Compilation ---

func (c *Compiler) compilePrefixExpression(node *parser.PrefixExpression) errors.PaseratiError {
	// Compile the right operand first
	err := c.compileNode(node.Right)
	if err != nil {
		return err
	}
	rightReg := c.regAlloc.Current() // Register holding the operand value

	// Allocate a new register for the result
	destReg := c.regAlloc.Alloc()

	// Emit the corresponding unary opcode
	switch node.Operator {
	case "!":
		c.emitNot(destReg, rightReg, node.Token.Line)
	case "-":
		c.emitNegate(destReg, rightReg, node.Token.Line)
	// --- NEW ---
	case "~":
		c.emitBitwiseNot(destReg, rightReg, node.Token.Line)
	// --- END NEW ---
	default:
		return NewCompileError(node, fmt.Sprintf("unknown prefix operator '%s'", node.Operator))
	}
	// Free the operand register now that the result is in destReg
	c.regAlloc.Free(rightReg)

	// The result is now in destReg
	c.regAlloc.SetCurrent(destReg) // Explicitly set current for clarity
	return nil
}

func (c *Compiler) compileInfixExpression(node *parser.InfixExpression) errors.PaseratiError {
	line := node.Token.Line // Use operator token line number

	// --- Standard binary operators (arithmetic, comparison, bitwise, shift) ---
	if node.Operator != "&&" && node.Operator != "||" && node.Operator != "??" {
		err := c.compileNode(node.Left)
		if err != nil {
			return err
		}
		leftReg := c.regAlloc.Current()

		err = c.compileNode(node.Right)
		if err != nil {
			return err
		}
		rightReg := c.regAlloc.Current()

		destReg := c.regAlloc.Alloc() // Allocate dest register *before* emitting op

		switch node.Operator {
		// Arithmetic
		case "+":
			c.emitAdd(destReg, leftReg, rightReg, line)
		case "-":
			c.emitSubtract(destReg, leftReg, rightReg, line)
		case "*":
			c.emitMultiply(destReg, leftReg, rightReg, line)
		case "/":
			c.emitDivide(destReg, leftReg, rightReg, line)
		case "%":
			c.emitRemainder(destReg, leftReg, rightReg, line)
		case "**":
			c.emitExponent(destReg, leftReg, rightReg, line)

		// Comparison
		case "<=":
			c.emitLessEqual(destReg, leftReg, rightReg, line)
		case ">=":
			// Implement as !(left < right)
			tempReg := c.regAlloc.Alloc() // Temp register for (left < right)
			c.emitLess(tempReg, leftReg, rightReg, line)
			c.emitNot(destReg, tempReg, line) // destReg = !(tempReg)
			c.regAlloc.Free(tempReg)          // Free the intermediate temp register
		case "==":
			c.emitEqual(destReg, leftReg, rightReg, line)
		case "!=":
			c.emitNotEqual(destReg, leftReg, rightReg, line)
		case "<":
			c.emitLess(destReg, leftReg, rightReg, line)
		case ">":
			c.emitGreater(destReg, leftReg, rightReg, line)
		case "===":
			c.emitStrictEqual(destReg, leftReg, rightReg, line)
		case "!==":
			c.emitStrictNotEqual(destReg, leftReg, rightReg, line)

		// --- NEW: Bitwise & Shift ---
		case "&":
			c.emitBitwiseAnd(destReg, leftReg, rightReg, line)
		case "|":
			c.emitBitwiseOr(destReg, leftReg, rightReg, line)
		case "^":
			c.emitBitwiseXor(destReg, leftReg, rightReg, line)
		case "<<":
			c.emitShiftLeft(destReg, leftReg, rightReg, line)
		case ">>":
			c.emitShiftRight(destReg, leftReg, rightReg, line)
		case ">>>":
			c.emitUnsignedShiftRight(destReg, leftReg, rightReg, line)
		// --- END NEW ---

		default:
			return NewCompileError(node, fmt.Sprintf("unknown standard infix operator '%s'", node.Operator))
		}
		// Result is now in destReg (allocator current is destReg)
		c.regAlloc.SetCurrent(destReg) // Explicitly set current

		//Free operand registers after use (check against destReg for safety)
		if leftReg != destReg {
			c.regAlloc.Free(leftReg)
		}
		if rightReg != destReg {
			c.regAlloc.Free(rightReg)
		}

		return nil
	}

	// --- Logical Operators (&&, ||, ??) with Short-Circuiting ---
	// Allocate result register *before* compiling operands for logical ops too
	destReg := c.regAlloc.Alloc()

	if node.Operator == "||" { // a || b
		err := c.compileNode(node.Left)
		if err != nil {
			return err
		}
		leftReg := c.regAlloc.Current()

		// Jump to right eval if left is FALSEY
		jumpToRightPlaceholder := c.emitPlaceholderJump(vm.OpJumpIfFalse, leftReg, line)

		// If left was TRUTHY: result is left, move to destReg and jump to end
		c.emitMove(destReg, leftReg, line)
		// Free leftReg if its value was used and moved
		c.regAlloc.Free(leftReg)
		jumpToEndPlaceholder := c.emitPlaceholderJump(vm.OpJump, 0, line)

		// Patch jumpToRightPlaceholder to land here (start of right operand eval)
		c.patchJump(jumpToRightPlaceholder)

		// Compile right operand (only executed if left was falsey)
		err = c.compileNode(node.Right)
		if err != nil {
			return err
		}
		rightReg := c.regAlloc.Current()
		// Result is right, move to destReg
		c.emitMove(destReg, rightReg, line)
		// Free rightReg after moving its value
		c.regAlloc.Free(rightReg)

		// Patch jumpToEndPlaceholder to land here
		c.patchJump(jumpToEndPlaceholder)
		// Result is now unified in destReg
		c.regAlloc.SetCurrent(destReg)
		return nil

	} else if node.Operator == "&&" { // a && b
		err := c.compileNode(node.Left)
		if err != nil {
			return err
		}
		leftReg := c.regAlloc.Current()

		// If left is FALSEY, jump to end, result is left
		jumpToEndPlaceholder := c.emitPlaceholderJump(vm.OpJumpIfFalse, leftReg, line)

		// If left was TRUTHY (didn't jump), compile right operand
		err = c.compileNode(node.Right)
		if err != nil {
			return err
		}
		rightReg := c.regAlloc.Current()
		// Result is right, move rightReg to destReg
		c.emitMove(destReg, rightReg, line) // If true path, result is right
		// Free rightReg after move
		c.regAlloc.Free(rightReg)
		// Jump over the false path's move
		jumpSkipFalseMovePlaceholder := c.emitPlaceholderJump(vm.OpJump, 0, line)
		// Patch jumpToEndPlaceholder to land here (false path)
		c.patchJump(jumpToEndPlaceholder)
		// Result is left, move leftReg to destReg
		c.emitMove(destReg, leftReg, line) // If false path, result is left
		// Free leftReg after move
		c.regAlloc.Free(leftReg)

		// Patch the skip jump
		c.patchJump(jumpSkipFalseMovePlaceholder)

		// Result is now unified in destReg
		c.regAlloc.SetCurrent(destReg)
		return nil

	} else if node.Operator == "??" { // a ?? b
		err := c.compileNode(node.Left)
		if err != nil {
			return err
		}
		leftReg := c.regAlloc.Current()

		// Temp registers for checks
		isNullReg := c.regAlloc.Alloc()
		isUndefReg := c.regAlloc.Alloc()
		nullConstReg := c.regAlloc.Alloc()
		undefConstReg := c.regAlloc.Alloc()

		// Load null and undefined constants
		c.emitLoadNewConstant(nullConstReg, vm.Null(), line)
		c.emitLoadNewConstant(undefConstReg, vm.Undefined(), line)

		// Check if left == null
		c.emitStrictEqual(isNullReg, leftReg, nullConstReg, line)
		// Jump if *NOT* null (jump if false) to the undefined check
		jumpCheckUndefPlaceholder := c.emitPlaceholderJump(vm.OpJumpIfFalse, isNullReg, line)

		// If left IS null. Jump straight to evaluating the right side.
		jumpEvalRightPlaceholder := c.emitPlaceholderJump(vm.OpJump, 0, line)

		// Land here if left was NOT null. Patch the first jump.
		c.patchJump(jumpCheckUndefPlaceholder)

		// Check if left == undefined
		c.emitStrictEqual(isUndefReg, leftReg, undefConstReg, line)
		// Jump if *NOT* undefined (and also not null) to skip the right side eval.
		jumpSkipRightPlaceholder := c.emitPlaceholderJump(vm.OpJumpIfFalse, isUndefReg, line)

		// Land here if left *was* null OR undefined. Patch the jump from the null check.
		c.patchJump(jumpEvalRightPlaceholder)

		// --- Eval Right Path ---
		// Compile right operand
		err = c.compileNode(node.Right)
		if err != nil {
			return err
		}
		rightReg := c.regAlloc.Current()
		// Move result to destReg
		c.emitMove(destReg, rightReg, line)
		// Free rightReg after move
		c.regAlloc.Free(rightReg)
		// Jump over the skip-right landing pad
		jumpEndPlaceholder := c.emitPlaceholderJump(vm.OpJump, 0, line)

		// --- Skip Right Path ---
		// Land here if left was NOT nullish. Patch the jump from the undefined check.
		c.patchJump(jumpSkipRightPlaceholder)
		// Result is already correctly in leftReg. Move it to destReg.
		c.emitMove(destReg, leftReg, line)
		// Free leftReg after move
		c.regAlloc.Free(leftReg)

		// Land here after either path finishes. Patch the jump from the right-eval path.
		c.patchJump(jumpEndPlaceholder)

		// Release temporary registers
		c.regAlloc.Free(isNullReg)
		c.regAlloc.Free(isUndefReg)
		c.regAlloc.Free(nullConstReg)
		c.regAlloc.Free(undefConstReg)

		// Unified result is now in destReg
		c.regAlloc.SetCurrent(destReg)
		return nil
	}

	// Should be unreachable
	return NewCompileError(node, fmt.Sprintf("logical/coalescing operator '%s' compilation fell through", node.Operator))
}

func (c *Compiler) compileFunctionLiteral(node *parser.FunctionLiteral, nameHint string) errors.PaseratiError {
	// 1. Create a new Compiler instance for the function body, linked to the current one
	functionCompiler := newFunctionCompiler(c) // Pass `c` as the enclosing compiler

	// --- Determine and set the function name being compiled ---
	var determinedFuncName string
	if nameHint != "" {
		determinedFuncName = nameHint
	} else if node.Name != nil {
		determinedFuncName = node.Name.Value
	} else {
		determinedFuncName = "<anonymous>"
	}
	functionCompiler.compilingFuncName = determinedFuncName
	// --- End Set Name ---

	// --- NEW: Define inner name in inner scope for recursion ---
	var funcNameForLookup string // Name used for potential recursive lookup
	if node.Name != nil {
		funcNameForLookup = node.Name.Value
		// Define the function's own name within its scope temporarily
		// This allows the name to be resolved locally during body compilation.
		// It will be treated as a free variable pointing to the closure itself.
		functionCompiler.currentSymbolTable.Define(funcNameForLookup, nilRegister)
	} else if nameHint != "" {
		// If anonymous but assigned (e.g., let f = function() { f(); }),
		// use the hint name for potential recursive calls.
		funcNameForLookup = nameHint
		// Define the hint name within the function's scope temporarily.
		functionCompiler.currentSymbolTable.Define(funcNameForLookup, nilRegister)
	}
	// --- END NEW ---

	// 2. Define parameters in the function compiler's *enclosed* scope
	for _, param := range node.Parameters {
		reg := functionCompiler.regAlloc.Alloc()
		// --- FIX: Access Name field ---
		functionCompiler.currentSymbolTable.Define(param.Name.Value, reg)
	}

	// 3. Compile the body using the function compiler
	// This will populate functionCompiler.freeSymbols
	err := functionCompiler.compileNode(node.Body)
	if err != nil {
		// Propagate errors
		c.errors = append(c.errors, functionCompiler.errors...)
		c.errors = append(c.errors, err)
		// Proceed to create function/closure object even if body has errors?
		// Let's continue for now, errors are collected.
	}

	// 4. Finalize function chunk (add implicit return to the function's chunk)
	functionCompiler.emitFinalReturn(node.Body.Token.Line)
	functionChunk := functionCompiler.chunk
	freeSymbols := functionCompiler.freeSymbols // Get the list of identified free variables
	// Collect any additional errors from the sub-compilation
	if len(functionCompiler.errors) > 0 {
		c.errors = append(c.errors, functionCompiler.errors...)
	}

	// Get required register count from the function's allocator
	regSize := functionCompiler.regAlloc.MaxRegs()

	// 5. Create the bytecode.Function object
	var funcName string
	if nameHint != "" { // Prioritize hint from let/const assignment
		funcName = nameHint
	} else if node.Name != nil { // Fallback to name from function keyword syntax
		funcName = node.Name.Value
	} else {
		funcName = "<anonymous>" // Default for anonymous literals not assigned
	}
	funcObj := vm.Function{
		Arity:        len(node.Parameters),
		Chunk:        functionChunk,
		Name:         funcName, // Use determined name
		RegisterSize: int(regSize),
	}

	// 6. Add the function object to the *outer* compiler's constant pool.
	funcValue := vm.NewFunction(&funcObj)      // Still use value.Function for the raw compiled code
	constIdx := c.chunk.AddConstant(funcValue) // Index of the function proto in outer chunk

	// 7. Emit OpClosure in the *outer* chunk.
	destReg := c.regAlloc.Alloc()                                              // Register for the resulting closure object in the outer scope
	debugPrintf("// [Closure %s] Allocated destReg: R%d\n", funcName, destReg) // DEBUG
	c.emitOpCode(vm.OpClosure, node.Token.Line)
	c.emitByte(byte(destReg))
	c.emitUint16(constIdx)             // Operand 1: Constant index of the function blueprint
	c.emitByte(byte(len(freeSymbols))) // Operand 2: Number of upvalues to capture

	// Emit operands for each upvalue
	for i, freeSym := range freeSymbols {
		debugPrintf("// [Closure Loop %s] Checking freeSym[%d]: %s (Reg %d) against funcNameForLookup: '%s'\n", funcName, i, freeSym.Name, freeSym.Register, funcNameForLookup) // DEBUG

		// --- Check for self-capture first (Simplified Check) ---
		// If a free symbol has nilRegister, it MUST be the temporary one
		// added for recursion resolution. It signifies self-capture.
		if freeSym.Register == nilRegister {
			// This is the special self-capture case identified during body compilation.
			debugPrintf("// [Closure SelfCapture %s] Symbol '%s' has nilRegister. Emitting isLocal=1, index=destReg=R%d\n", funcName, freeSym.Name, destReg) // DEBUG
			c.emitByte(1)                                                                                                                                    // isLocal = true (capture from the stack where the closure will be placed)
			c.emitByte(byte(destReg))                                                                                                                        // Index = the destination register of OpClosure
			continue                                                                                                                                         // Skip the normal lookup below
		}
		// --- END Check ---

		// Resolve the symbol again in the *enclosing* compiler's context
		// (This part should now only run for *non-recursive* free variables)
		enclosingSymbol, enclosingTable, found := c.currentSymbolTable.Resolve(freeSym.Name)
		if !found {
			// This should theoretically not happen if it was resolved during body compilation
			// but handle defensively.
			panic(fmt.Sprintf("compiler internal error: free variable %s not found in enclosing scope during closure creation", freeSym.Name))
		}

		if enclosingTable == c.currentSymbolTable {
			debugPrintf("// [Closure Loop %s] Free '%s' is Local in enclosing. Emitting isLocal=1, index=R%d\n", funcName, freeSym.Name, enclosingSymbol.Register) // DEBUG
			// The free variable is local in the *direct* enclosing scope.
			c.emitByte(1) // isLocal = true
			// Capture the value from the enclosing scope's actual register
			c.emitByte(byte(enclosingSymbol.Register)) // Index = register index
		} else {
			// The free variable is also a free variable in the enclosing scope.
			// We need to capture it from the enclosing scope's upvalues.
			// We need the index of this symbol within the *enclosing* compiler's freeSymbols list.
			enclosingFreeIndex := c.addFreeSymbol(node, &enclosingSymbol)                                                                                   // Use the same helper
			debugPrintf("// [Closure Loop %s] Free '%s' is Outer in enclosing. Emitting isLocal=0, index=%d\n", funcName, freeSym.Name, enclosingFreeIndex) // DEBUG
			c.emitByte(0)                                                                                                                                   // isLocal = false
			c.emitByte(byte(enclosingFreeIndex))                                                                                                            // Index = upvalue index in enclosing scope
		}
	}

	// 8. Set the result register for the caller
	c.lastExprReg = destReg
	c.lastExprRegValid = true

	return nil // Return nil even if there were body errors, errors are collected in c.errors
}

func (c *Compiler) compileCallExpression(node *parser.CallExpression) errors.PaseratiError {
	// 1. Compile the expression being called (e.g., function name)
	err := c.compileNode(node.Function)
	if err != nil {
		return err
	}
	funcReg := c.regAlloc.Current() // Register holding the function value

	// 2. Compile arguments
	argRegs := []Register{}
	for _, arg := range node.Arguments {
		err = c.compileNode(arg)
		if err != nil {
			return err
		}
		argRegs = append(argRegs, c.regAlloc.Current())
	}
	argCount := len(argRegs)

	// TODO: Check arity at compile time or runtime?

	// Ensure arguments are in the correct registers for the call convention.
	// Convention: Args must be in registers funcReg+1, funcReg+2, ...
	for i := 0; i < argCount; i++ {
		targetArgReg := funcReg + 1 + Register(i)
		actualArgReg := argRegs[i]
		// Only move if the argument isn't already in the target register
		if actualArgReg != targetArgReg {
			// TODO: Ensure targetArgReg was allocated or handle allocation?
			// For now, assume the register allocator provides enough headroom
			// or that targetArgReg might overwrite something no longer needed.
			// This is slightly dangerous and might need a more robust register
			// allocation strategy for calls.
			c.emitMove(targetArgReg, actualArgReg, node.Token.Line) // Use line of call expression
		}
	}

	// 3. Allocate register for the return value
	resultReg := c.regAlloc.Alloc()

	// 4. Emit OpCall
	c.emitCall(resultReg, funcReg, byte(argCount), node.Token.Line)

	// The result of the call is now in resultReg
	return nil
}

func (c *Compiler) compileIfExpression(node *parser.IfExpression) errors.PaseratiError {
	// 1. Compile the condition
	err := c.compileNode(node.Condition)
	if err != nil {
		return err
	}
	conditionReg := c.regAlloc.Current() // Register holding the condition result

	// 2. Emit placeholder jump for false condition
	jumpIfFalsePos := c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)

	// 3. Compile the consequence block
	// TODO: Handle block scope if needed later
	err = c.compileNode(node.Consequence)
	if err != nil {
		return err
	}
	// TODO: How does an if-expr produce a value? Need convention.
	// Does the last expr statement value remain in a register?

	if node.Alternative != nil {
		// 4a. If there's an else, emit placeholder jump over the else block
		jumpElsePos := c.emitPlaceholderJump(vm.OpJump, 0, node.Consequence.Token.Line) // Use line of opening brace? Or token after consequence?

		// 5a. Backpatch the OpJumpIfFalse to jump *here* (start of else)
		c.patchJump(jumpIfFalsePos)

		// 6a. Compile the alternative block
		err = c.compileNode(node.Alternative)
		if err != nil {
			return err
		}

		// 7a. Backpatch the OpJump to jump *here* (end of else block)
		c.patchJump(jumpElsePos)

	} else {
		// 4b. If no else, backpatch OpJumpIfFalse to jump *here* (end of if block)
		c.patchJump(jumpIfFalsePos)
		// TODO: What value should an if without else produce? Undefined?
		// If so, might need to emit OpLoadUndefined here.
	}

	// TODO: Free conditionReg if no longer needed?
	return nil
}

// compileTernaryExpression compiles condition ? consequence : alternative
func (c *Compiler) compileTernaryExpression(node *parser.TernaryExpression) errors.PaseratiError {
	// 1. Compile condition
	err := c.compileNode(node.Condition)
	if err != nil {
		return err
	}
	conditionReg := c.regAlloc.Current()

	// 2. Jump if false
	jumpFalsePos := c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)
	// Free condition register now that jump is emitted
	c.regAlloc.Free(conditionReg)

	// --- Consequence Path ---
	// 3. Compile consequence
	err = c.compileNode(node.Consequence)
	if err != nil {
		return err
	}
	consequenceReg := c.regAlloc.Current() // Result is here for this path

	// 4. Allocate result register NOW
	resultReg := c.regAlloc.Alloc()
	// 5. Move consequence to result
	c.emitMove(resultReg, consequenceReg, node.Token.Line)
	// Free consequence register after moving its value
	c.regAlloc.Free(consequenceReg)

	// 6. Jump over alternative
	jumpEndPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)

	// --- Alternative Path ---
	// 7. Patch jumpFalse
	c.patchJump(jumpFalsePos)

	// 8. Compile alternative
	err = c.compileNode(node.Alternative)
	if err != nil {
		return err
	}
	alternativeReg := c.regAlloc.Current() // Result is here for this path

	// 9. Move alternative to result (OVERWRITE resultReg)
	c.emitMove(resultReg, alternativeReg, node.Token.Line)
	// Free alternative register after moving its value
	c.regAlloc.Free(alternativeReg)

	// --- End ---
	// 10. Patch jumpEnd
	c.patchJump(jumpEndPos)

	// Now, regardless of path, resultReg holds the correct value.
	// The allocator's current register *might* be alternativeReg if that path was last
	// compiled, but we need it to be resultReg.
	// Since we can't SetCurrent, let's just move resultReg to a NEW register
	// to make *that* the current one. This is slightly wasteful but works.
	finalReg := c.regAlloc.Alloc()
	c.emitMove(finalReg, resultReg, node.Token.Line)
	// Free the intermediate result register now that value is in finalReg
	c.regAlloc.Free(resultReg)

	// Now Current() correctly points to finalReg which holds the unified result.
	c.regAlloc.SetCurrent(finalReg) // Set current explicitly
	return nil
}

// --- Loop Compilation (Updated) ---

func (c *Compiler) compileWhileStatement(node *parser.WhileStatement) errors.PaseratiError {
	line := node.Token.Line

	// --- Setup Loop Context ---
	loopStartPos := len(c.chunk.Code) // Position before condition evaluation
	loopContext := &LoopContext{
		LoopStartPos:               loopStartPos,
		ContinueTargetPos:          loopStartPos, // Continue goes back to condition in while
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
	}
	c.loopContextStack = append(c.loopContextStack, loopContext)

	// --- Compile Condition ---
	err := c.compileNode(node.Condition)
	if err != nil {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1] // Pop context on error
		return NewCompileError(node, "error compiling while condition").CausedBy(err)
	}
	conditionReg := c.regAlloc.Current()

	// --- Jump Out If False ---
	jumpToEndPlaceholderPos := c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, line)

	// --- Compile Body ---
	err = c.compileNode(node.Body)
	if err != nil {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1] // Pop context on error
		return NewCompileError(node, "error compiling while body").CausedBy(err)
	}

	// --- Jump Back To Start ---
	jumpBackInstructionEndPos := len(c.chunk.Code) + 1 + 2 // OpCode + 16bit offset
	backOffset := loopStartPos - jumpBackInstructionEndPos
	c.emitOpCode(vm.OpJump, line)
	c.emitUint16(uint16(int16(backOffset))) // Emit calculated signed offset

	// --- Finish Loop ---
	// Patch the initial conditional jump to land here (after the backward jump)
	c.patchJump(jumpToEndPlaceholderPos)

	// Pop context and patch breaks
	poppedContext := c.loopContextStack[len(c.loopContextStack)-1]
	c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
	for _, breakPlaceholderPos := range poppedContext.BreakPlaceholderPosList {
		c.patchJump(breakPlaceholderPos) // Patch break jumps to loop end
	}

	// --- NEW: Patch continue jumps ---
	// Continue jumps back to the condition check (loopStartPos)
	for _, continuePos := range poppedContext.ContinuePlaceholderPosList {
		jumpInstructionEndPos := continuePos + 1 + 2 // OpCode + 16bit offset
		targetOffset := poppedContext.LoopStartPos - jumpInstructionEndPos
		if targetOffset > math.MaxInt16 || targetOffset < math.MinInt16 {
			return NewCompileError(node, fmt.Sprintf("internal compiler error: continue jump offset %d exceeds 16-bit limit", targetOffset))
		}
		// Manually write the 16-bit offset into the placeholder jump instruction
		c.chunk.Code[continuePos+1] = byte(int16(targetOffset) >> 8)   // High byte
		c.chunk.Code[continuePos+2] = byte(int16(targetOffset) & 0xFF) // Low byte
	}

	return nil
}

func (c *Compiler) compileForStatement(node *parser.ForStatement) errors.PaseratiError {
	// No new scope for initializer, it shares the outer scope

	// 1. Initializer
	if node.Initializer != nil {
		if err := c.compileNode(node.Initializer); err != nil {
			return err
		}
	}

	// --- Loop Start & Context Setup ---
	loopStartPos := len(c.chunk.Code) // Position before condition check
	loopContext := &LoopContext{
		LoopStartPos:               loopStartPos,
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
	}
	c.loopContextStack = append(c.loopContextStack, loopContext)
	// Scope for body/vars is handled by compileNode for the BlockStatement

	// --- 2. Condition (Optional) ---
	var conditionExitJumpPlaceholderPos int = -1
	if node.Condition != nil {
		if err := c.compileNode(node.Condition); err != nil {
			// Clean up loop context if condition compilation fails
			c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
			return err
		}
		conditionReg := c.regAlloc.Current()
		conditionExitJumpPlaceholderPos = c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)
	} // If no condition, it's an infinite loop (handled by break/return)

	// --- 3. Body ---
	// Continue placeholders will be added to loopContext here
	if err := c.compileNode(node.Body); err != nil {
		// Clean up loop context if body compilation fails
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return err
	}

	// --- 4. Patch Continues & Compile Update ---

	// *** Patch Continue Jumps ***
	// Patch continue jumps to land here, *before* the update expression
	// updateStartPos := len(c.chunk.Code) // REMOVED - patchJump uses current position
	for _, continuePos := range loopContext.ContinuePlaceholderPosList { // Use context on stack
		c.patchJump(continuePos) // Patch placeholder to jump to current position
	}

	// *** Compile Update Expression (Optional) ***
	if node.Update != nil {
		if err := c.compileNode(node.Update); err != nil {
			// Clean up loop context if update compilation fails
			c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
			return err
		}
		// Result of update expression is discarded implicitly by not using c.lastReg
	}

	// --- 5. Jump back to Loop Start (before condition) ---
	jumpBackInstructionEndPos := len(c.chunk.Code) + 1 + 2 // OpCode + 16bit offset
	backOffset := loopStartPos - jumpBackInstructionEndPos
	c.emitOpCode(vm.OpJump, node.Body.Token.Line) // Use body's line for jump back
	c.emitUint16(uint16(int16(backOffset)))

	// --- 6. Loop End & Patch Condition/Breaks ---

	// Position *after* the loop (target for breaks/condition exit) is implicitly len(c.chunk.Code)
	// loopEndPos := len(c.chunk.Code) // REMOVED - Not needed if patchJump uses current len()

	// Pop loop context
	poppedContext := c.loopContextStack[len(c.loopContextStack)-1]
	c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]

	// Patch the condition exit jump (if there was a condition)
	// This needs to happen *at* the final position
	if conditionExitJumpPlaceholderPos != -1 {
		c.patchJump(conditionExitJumpPlaceholderPos) // Patch to jump to current position
	}

	// Patch all break jumps
	// This needs to happen *at* the final position
	for _, breakPos := range poppedContext.BreakPlaceholderPosList {
		c.patchJump(breakPos) // Patch to jump to current position
	}

	return nil
}

// --- New: Break/Continue Compilation ---

func (c *Compiler) compileBreakStatement(node *parser.BreakStatement) errors.PaseratiError {
	if len(c.loopContextStack) == 0 {
		return NewCompileError(node, "break statement not within a loop")
	}

	// Get current loop context (top of stack)
	currentLoopContext := c.loopContextStack[len(c.loopContextStack)-1]

	// Emit placeholder jump (OpJump) - Pass 0 for srcReg as it's ignored
	placeholderPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)

	// Add placeholder position to the context's list for later patching
	currentLoopContext.BreakPlaceholderPosList = append(currentLoopContext.BreakPlaceholderPosList, placeholderPos)

	return nil
}

func (c *Compiler) compileContinueStatement(node *parser.ContinueStatement) errors.PaseratiError {
	if len(c.loopContextStack) == 0 {
		return NewCompileError(node, "continue statement not within a loop")
	}

	// Get current loop context (top of stack)
	currentLoopContext := c.loopContextStack[len(c.loopContextStack)-1]

	// Emit placeholder jump (OpJump) - Pass 0 for srcReg as it's ignored
	placeholderPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)

	// Add placeholder position to the context's list for later patching
	currentLoopContext.ContinuePlaceholderPosList = append(currentLoopContext.ContinuePlaceholderPosList, placeholderPos)

	return nil
}

// --- Bytecode Emission Helpers ---

func (c *Compiler) emitOpCode(op vm.OpCode, line int) {
	c.chunk.WriteOpCode(op, line)
}

func (c *Compiler) emitByte(b byte) {
	c.chunk.WriteByte(b)
}

func (c *Compiler) emitUint16(val uint16) {
	c.chunk.WriteUint16(val)
}

func (c *Compiler) emitLoadConstant(dest Register, constIdx uint16, line int) {
	c.emitOpCode(vm.OpLoadConst, line)
	c.emitByte(byte(dest))
	c.emitUint16(constIdx)
}

func (c *Compiler) emitLoadNull(dest Register, line int) {
	c.emitOpCode(vm.OpLoadNull, line)
	c.emitByte(byte(dest))
}

func (c *Compiler) emitLoadUndefined(dest Register, line int) {
	c.emitOpCode(vm.OpLoadUndefined, line)
	c.emitByte(byte(dest))
}

func (c *Compiler) emitLoadTrue(dest Register, line int) {
	c.emitOpCode(vm.OpLoadTrue, line)
	c.emitByte(byte(dest))
}

func (c *Compiler) emitLoadFalse(dest Register, line int) {
	c.emitOpCode(vm.OpLoadFalse, line)
	c.emitByte(byte(dest))
}

func (c *Compiler) emitMove(dest, src Register, line int) {
	c.emitOpCode(vm.OpMove, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

func (c *Compiler) emitReturn(src Register, line int) {
	c.emitOpCode(vm.OpReturn, line)
	c.emitByte(byte(src))
}

func (c *Compiler) emitNegate(dest, src Register, line int) {
	c.emitOpCode(vm.OpNegate, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

func (c *Compiler) emitNot(dest, src Register, line int) {
	c.emitOpCode(vm.OpNot, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

func (c *Compiler) emitAdd(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpAdd, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitSubtract(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpSubtract, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitMultiply(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpMultiply, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitDivide(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpDivide, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitEqual(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpEqual, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitNotEqual(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpNotEqual, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitGreater(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpGreater, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitLess(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpLess, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitLessEqual(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpLessEqual, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitCall(dest, funcReg Register, argCount byte, line int) {
	c.emitOpCode(vm.OpCall, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(funcReg))
	c.emitByte(argCount)
}

// emitFinalReturn adds the final OpReturnUndefined instruction.
func (c *Compiler) emitFinalReturn(line int) {
	// No need to load undefined first
	c.emitOpCode(vm.OpReturnUndefined, line)
}

// Overload or new function to handle adding constant and emitting load
func (c *Compiler) emitLoadNewConstant(dest Register, val vm.Value, line int) {
	constIdx := c.chunk.AddConstant(val)
	c.emitLoadConstant(dest, constIdx, line)
}

// addFreeSymbol adds a symbol identified as a free variable to the compiler's list.
// It ensures the symbol is added only once and returns its index in the freeSymbols slice.
func (c *Compiler) addFreeSymbol(node parser.Node, symbol *Symbol) uint8 { // Assuming max 256 free vars for now
	for i, free := range c.freeSymbols {
		if free == symbol { // Pointer comparison should work if Resolve returns the same Symbol instance
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

// compileArrowFunctionLiteral compiles an arrow function literal expression.
func (c *Compiler) compileArrowFunctionLiteral(node *parser.ArrowFunctionLiteral) errors.PaseratiError {
	// 1. Create a compiler for the function scope
	funcCompiler := newFunctionCompiler(c)
	funcCompiler.compilingFuncName = "<arrow>" // Set name for arrow functions

	// 2. Define parameters in the function's symbol table
	for _, p := range node.Parameters {
		reg := funcCompiler.regAlloc.Alloc()
		// --- FIX: Access Name field ---
		funcCompiler.currentSymbolTable.Define(p.Name.Value, reg)
	}

	// 3. Compile the function body
	var returnReg Register
	implicitReturnNeeded := true
	switch bodyNode := node.Body.(type) {
	case *parser.BlockStatement:
		err := funcCompiler.compileNode(bodyNode)
		if err != nil {
			funcCompiler.errors = append(funcCompiler.errors, err)
		}
		implicitReturnNeeded = false // Block handles its own returns or falls through
	case parser.Expression:
		err := funcCompiler.compileNode(bodyNode)
		if err != nil {
			funcCompiler.errors = append(funcCompiler.errors, err)
			returnReg = 0 // Indicate error or inability to get result reg
		} else {
			returnReg = funcCompiler.regAlloc.Current()
		}
		implicitReturnNeeded = true // Expression body needs implicit return
	default:
		funcCompiler.errors = append(funcCompiler.errors, NewCompileError(node, fmt.Sprintf("invalid body type %T for arrow function", node.Body)))
		implicitReturnNeeded = false
	}
	if implicitReturnNeeded {
		funcCompiler.emitReturn(returnReg, node.Token.Line)
	}

	// Add final implicit return for the function (catches paths that don't hit explicit/implicit returns)
	funcCompiler.emitFinalReturn(node.Token.Line) // Use arrow token line number

	// Collect errors from sub-compilation
	if len(funcCompiler.errors) > 0 {
		c.errors = append(c.errors, funcCompiler.errors...)
		// Continue even with errors to potentially catch more issues
	}

	// Get captured free variables and required register count
	freeSymbols := funcCompiler.freeSymbols
	regSize := funcCompiler.regAlloc.MaxRegs()
	functionChunk := funcCompiler.chunk

	// 6. Create the function object
	compiledFunc := vm.Function{
		Chunk:        functionChunk,
		Arity:        len(node.Parameters),
		RegisterSize: int(regSize),
		Name:         "<arrow>", // Arrow functions are anonymous
	}

	// 7. Add function constant to the *enclosing* compiler (c)
	funcValue := vm.NewFunction(&compiledFunc)
	constIdx := c.chunk.AddConstant(funcValue)

	// 8. Emit OpClosure in the *enclosing* compiler (c)
	destReg := c.regAlloc.Alloc()                                                                    // Register for the resulting closure object in the outer scope
	debugPrintf("// [Closure %s] Allocated destReg: R%d\n", funcCompiler.compilingFuncName, destReg) // DEBUG
	c.emitOpCode(vm.OpClosure, node.Token.Line)
	c.emitByte(byte(destReg))
	c.emitUint16(constIdx)             // Operand 1: Constant index of the function blueprint
	c.emitByte(byte(len(freeSymbols))) // Operand 2: Number of upvalues to capture

	// Emit operands for each upvalue
	for i, freeSym := range freeSymbols {
		debugPrintf("// [Closure Loop %s] Checking freeSym[%d]: %s (Reg %d) against funcNameForLookup: '%s'\n", funcCompiler.compilingFuncName, i, freeSym.Name, freeSym.Register, funcCompiler.compilingFuncName) // DEBUG

		// --- Check for self-capture first (Simplified Check) ---
		// If a free symbol has nilRegister, it MUST be the temporary one
		// added for recursion resolution. It signifies self-capture.
		if freeSym.Register == nilRegister {
			// This is the special self-capture case identified during body compilation.
			debugPrintf("// [Closure SelfCapture %s] Symbol '%s' has nilRegister. Emitting isLocal=1, index=destReg=R%d\n", funcCompiler.compilingFuncName, freeSym.Name, destReg) // DEBUG
			c.emitByte(1)                                                                                                                                                          // isLocal = true (capture from the stack where the closure will be placed)
			c.emitByte(byte(destReg))                                                                                                                                              // Index = the destination register of OpClosure
			continue                                                                                                                                                               // Skip the normal lookup below
		}
		// --- END Check ---

		// Resolve the symbol again in the *enclosing* compiler's context
		// (This part should now only run for *non-recursive* free variables)
		enclosingSymbol, enclosingTable, found := c.currentSymbolTable.Resolve(freeSym.Name)
		if !found {
			// This should theoretically not happen if it was resolved during body compilation
			// but handle defensively.
			panic(fmt.Sprintf("compiler internal error: free variable %s not found in enclosing scope during closure creation", freeSym.Name))
		}

		if enclosingTable == c.currentSymbolTable {
			debugPrintf("// [Closure Loop %s] Free '%s' is Local in enclosing. Emitting isLocal=1, index=R%d\n", funcCompiler.compilingFuncName, freeSym.Name, enclosingSymbol.Register) // DEBUG
			// The free variable is local in the *direct* enclosing scope.
			c.emitByte(1) // isLocal = true
			// Capture the value from the enclosing scope's actual register
			c.emitByte(byte(enclosingSymbol.Register)) // Index = register index
		} else {
			// The free variable is also a free variable in the enclosing scope.
			// We need to capture it from the enclosing scope's upvalues.
			// We need the index of this symbol within the *enclosing* compiler's freeSymbols list.
			enclosingFreeIndex := c.addFreeSymbol(node, &enclosingSymbol)                                                                                                         // Use the same helper
			debugPrintf("// [Closure Loop %s] Free '%s' is Outer in enclosing. Emitting isLocal=0, index=%d\n", funcCompiler.compilingFuncName, freeSym.Name, enclosingFreeIndex) // DEBUG
			c.emitByte(0)                                                                                                                                                         // isLocal = false
			c.emitByte(byte(enclosingFreeIndex))                                                                                                                                  // Index = upvalue index in enclosing scope
		}
	}

	// 8. Set the result register for the caller
	c.lastExprReg = destReg
	c.lastExprRegValid = true

	return nil // Return nil even if there were body errors, errors are collected in c.errors
}

// Added Helper
func (c *Compiler) emitStrictEqual(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpStrictEqual, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

// Added Helper
func (c *Compiler) emitStrictNotEqual(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpStrictNotEqual, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

// Added helper for OpSetUpvalue
func (c *Compiler) emitSetUpvalue(upvalueIndex uint8, srcReg Register, line int) {
	c.emitOpCode(vm.OpSetUpvalue, line)
	c.emitByte(byte(upvalueIndex))
	c.emitByte(byte(srcReg))
}

// --- NEW: DoWhile Statement Compilation ---

func (c *Compiler) compileDoWhileStatement(node *parser.DoWhileStatement) errors.PaseratiError {
	line := node.Token.Line

	// 1. Mark Loop Start (before body)
	loopStartPos := len(c.chunk.Code)

	// 2. Setup Loop Context
	loopContext := &LoopContext{
		LoopStartPos:               loopStartPos, // Continue jumps here
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
	}
	c.loopContextStack = append(c.loopContextStack, loopContext)

	// 3. Compile Body (executes at least once)
	if err := c.compileNode(node.Body); err != nil {
		// Pop context if body compilation fails
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return NewCompileError(node, "error compiling do-while body").CausedBy(err)
	}

	// 4. Mark Condition Position (for clarity, not used directly in jump calcs below)
	_ = len(c.chunk.Code) // conditionPos := len(c.chunk.Code)

	// 5. Compile Condition
	if err := c.compileNode(node.Condition); err != nil {
		// Pop context if condition compilation fails
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return NewCompileError(node, "error compiling do-while condition").CausedBy(err)
	}
	conditionReg := c.regAlloc.Current()

	// 6. Conditional Jump back to Loop Start
	// We need OpJumpIfTrue, but we only have OpJumpIfFalse.
	// So, we invert the condition and use OpJumpIfFalse.
	invertedConditionReg := c.regAlloc.Alloc()
	c.emitNot(invertedConditionReg, conditionReg, line)

	// Now jump back if the *inverted* condition is FALSE (i.e., original was TRUE)
	jumpBackInstructionEndPos := len(c.chunk.Code) + 1 + 2 + 1 // OpCode + Reg + 16bit offset
	backOffset := loopStartPos - jumpBackInstructionEndPos
	if backOffset > math.MaxInt16 || backOffset < math.MinInt16 {
		return NewCompileError(node, fmt.Sprintf("internal compiler error: do-while loop jump offset %d exceeds 16-bit limit", backOffset))
	}
	c.emitOpCode(vm.OpJumpIfFalse, line)    // Use OpJumpIfFalse on inverted result
	c.emitByte(byte(invertedConditionReg))  // Jump based on the inverted condition
	c.emitUint16(uint16(int16(backOffset))) // Emit calculated signed offset

	// Release the temporary inverted condition register if possible
	// c.regAlloc.Release(invertedConditionReg) // Depends on allocator design

	// --- 7. Loop End & Patching ---
	// Position after the loop (target for breaks) is implicitly len(c.chunk.Code)

	// 8. Pop loop context
	poppedContext := c.loopContextStack[len(c.loopContextStack)-1]
	c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]

	// 9. Patch Break Jumps
	for _, breakPos := range poppedContext.BreakPlaceholderPosList {
		c.patchJump(breakPos) // Patch break jumps to loop end
	}

	// 10. Patch Continue Jumps
	// Continue jumps back to the body start (loopStartPos)
	for _, continuePos := range poppedContext.ContinuePlaceholderPosList {
		jumpInstructionEndPos := continuePos + 1 + 2 // OpJump OpCode + 16bit offset
		targetOffset := poppedContext.LoopStartPos - jumpInstructionEndPos
		if targetOffset > math.MaxInt16 || targetOffset < math.MinInt16 {
			return NewCompileError(node, fmt.Sprintf("internal compiler error: do-while continue jump offset %d exceeds 16-bit limit", targetOffset))
		}
		// Manually write the 16-bit offset into the placeholder OpJump instruction
		c.chunk.Code[continuePos+1] = byte(int16(targetOffset) >> 8)   // High byte
		c.chunk.Code[continuePos+2] = byte(int16(targetOffset) & 0xFF) // Low byte
	}

	return nil
}

// --- NEW: Update Expression Compilation ---

func (c *Compiler) compileUpdateExpression(node *parser.UpdateExpression) errors.PaseratiError {
	line := node.Token.Line

	// 1. Argument must be an identifier (parser should enforce, but check again)
	ident, ok := node.Argument.(*parser.Identifier)
	if !ok {
		return NewCompileError(node, fmt.Sprintf("invalid target for %s: expected identifier, got %T", node.Operator, node.Argument))
	}

	// 2. Resolve identifier and determine if local or upvalue
	symbolRef, definingTable, found := c.currentSymbolTable.Resolve(ident.Value)
	if !found {
		return NewCompileError(node, fmt.Sprintf("applying %s to undeclared variable '%s'", node.Operator, ident.Value))
	}

	var targetReg Register
	isUpvalue := false
	var upvalueIndex uint8
	targetRegIsTemporary := false // Flag if targetReg was allocated for upvalue load

	if definingTable == c.currentSymbolTable {
		// Local variable: Get its register.
		targetReg = symbolRef.Register
		// If it's a compound assignment, the existing value in targetReg is used directly.
		// If it's simple assignment '=', targetReg will be overwritten later.
	} else {
		// Upvalue: Get its index and load current value into a temporary register.
		isUpvalue = true
		upvalueIndex = c.addFreeSymbol(node, &symbolRef)
		targetReg = c.regAlloc.Alloc()
		targetRegIsTemporary = true // Mark this register as temporary
		c.emitOpCode(vm.OpLoadFree, line)
		c.emitByte(byte(targetReg))
		c.emitByte(upvalueIndex)
	}
	// Now targetReg holds the *current* value (either directly or loaded from upvalue)

	// 3. Load constant 1
	constOneReg := c.regAlloc.Alloc()
	constOneIdx := c.chunk.AddConstant(vm.Number(1))
	c.emitLoadConstant(constOneReg, constOneIdx, line)

	// 4. Perform Pre/Post logic
	resultReg := c.regAlloc.Alloc() // Register to hold the expression's final result

	if node.Prefix {
		// Prefix (++x or --x):
		// a. Operate: targetReg = targetReg +/- constOneReg
		switch node.Operator {
		case "++":
			c.emitAdd(targetReg, targetReg, constOneReg, line)
		case "--":
			c.emitSubtract(targetReg, targetReg, constOneReg, line)
		}
		// b. Store back if upvalue
		if isUpvalue {
			c.emitSetUpvalue(upvalueIndex, targetReg, line)
		}
		// c. Result of expression is the *new* value
		c.emitMove(resultReg, targetReg, line)

		// Free the temporary targetReg if it was allocated for an upvalue
		if targetRegIsTemporary {
			c.regAlloc.Free(targetReg)
		}

	} else {
		// Postfix (x++ or x--):
		// a. Save original value: resultReg = targetReg
		c.emitMove(resultReg, targetReg, line)
		// b. Operate: targetReg = targetReg +/- constOneReg
		switch node.Operator {
		case "++":
			c.emitAdd(targetReg, targetReg, constOneReg, line)
		case "--":
			c.emitSubtract(targetReg, targetReg, constOneReg, line)
		}
		// c. Store back if upvalue
		if isUpvalue {
			c.emitSetUpvalue(upvalueIndex, targetReg, line)
			// Free the temporary targetReg AFTER storing back
			c.regAlloc.Free(targetReg)
		}
		// d. Result of expression is the *original* value (already saved in resultReg)
	}

	// Release temporary register for constant 1
	c.regAlloc.Free(constOneReg)

	// 5. Set compiler's current register to the expression result
	c.regAlloc.SetCurrent(resultReg)
	return nil
}

// --- NEW: Array/Index ---
func (c *Compiler) compileArrayLiteral(node *parser.ArrayLiteral) errors.PaseratiError {
	elementCount := len(node.Elements)
	if elementCount > 255 { // Check against OpMakeArray count operand size
		return NewCompileError(node, "array literal exceeds maximum size of 255 elements")
	}

	// 1. Compile elements and store their final registers
	elementRegs := make([]Register, elementCount)
	elementRegsContinuous := true
	for i, elem := range node.Elements {
		err := c.compileNode(elem)
		if err != nil {
			return err
		}
		elementRegs[i] = c.regAlloc.Current() // Store the register holding the final result of this element
		if i > 0 && elementRegs[i] != elementRegs[i-1]+1 {
			elementRegsContinuous = false
		}
	}

	// 2. Allocate a contiguous block for the elements and move them
	var firstTargetReg Register
	if elementCount > 0 && elementRegsContinuous {
		firstTargetReg = elementRegs[0]
	} else if elementCount > 0 {
		// Allocate the first register of the target contiguous block
		firstTargetReg = c.regAlloc.Alloc()
		// Allocate the rest of the registers needed for the contiguous block
		for i := 1; i < elementCount; i++ {
			// We rely on the allocator returning consecutive regs here
			// when called consecutively. A more robust allocator might
			// offer a specific "allocate block" function.
			_ = c.regAlloc.Alloc()
		}

		// Move elements from their original registers (elementRegs)
		// into the new contiguous block starting at firstTargetReg.

		for i := 0; i < elementCount; i++ {
			targetReg := firstTargetReg + Register(i)
			sourceReg := elementRegs[i]
			if sourceReg != targetReg { // Avoid redundant moves
				c.emitMove(targetReg, sourceReg, node.Token.Line) // Use array literal's line number?
				// Free the original element register if it was moved and not needed anymore
				// (This assumes the sourceReg isn't needed elsewhere, which should be true
				// if it was just the result of the element expression compilation).
				//c.regAlloc.Free(sourceReg) // REMOVED: Potentially unsafe if sourceReg is needed later or reused.
			}
		}
	} else {
		// Handle empty array case: OpMakeArray needs a StartReg, use 0.
		firstTargetReg = 0
	}

	// 3. Allocate register for the array itself
	arrayReg := c.regAlloc.Alloc()

	// 4. Emit OpMakeArray using the contiguous block
	c.emitOpCode(vm.OpMakeArray, node.Token.Line)
	c.emitByte(byte(arrayReg))       // DestReg: where the new array object goes
	c.emitByte(byte(firstTargetReg)) // StartReg: start of the contiguous element block
	c.emitByte(byte(elementCount))   // Count: number of elements

	// Free the contiguous block registers if any were used
	if elementCount > 0 {
		// Free registers from firstTargetReg up to firstTargetReg + count - 1
		for i := 0; i < elementCount; i++ {
			// Free registers in reverse order to potentially help stack allocation reuse
			c.regAlloc.Free(firstTargetReg + Register(elementCount-1-i))
		}
		// If firstTargetReg was one of the elementRegs initially and already freed,
		// freeing again might be problematic. The current Free implementation is safe
		// but a more complex one might need checks.
	}

	// Result (the array) is now in arrayReg
	c.regAlloc.SetCurrent(arrayReg)
	return nil
}

func (c *Compiler) compileIndexExpression(node *parser.IndexExpression) errors.PaseratiError {
	line := GetTokenFromNode(node).Line                                  // Use '[' token line
	debugPrintf(">>> Enter compileIndexExpression: %s\n", node.String()) // <<< DEBUG ENTRY

	// 1. Compile the expression being indexed (the base: array/object/string)
	debugPrintf("--- Compiling Base: %s\n", node.Left.String()) // <<< DEBUG
	err := c.compileNode(node.Left)
	if err != nil {
		debugPrintf("<<< Exit compileIndexExpression (Error compiling base)\n") // <<< DEBUG EXIT
		return NewCompileError(node.Left, "error compiling base of index expression").CausedBy(err)
	}
	// <<< DEBUG BASE RESULT >>>
	baseRegFromState := c.lastExprReg
	baseRegFromCurrent := c.regAlloc.Current()
	debugPrintf("--- Base Compiled. lastExprReg: %s, lastExprRegValid: %t, regAlloc.Current(): %s\n", baseRegFromState, c.lastExprRegValid, baseRegFromCurrent)
	// --- Temporarily use Current() as per existing code for testing ---
	arrayReg := c.regAlloc.Current()                                  // Keep existing logic for now
	debugPrintf("--- Using arrayReg = %s (from Current)\n", arrayReg) // <<< DEBUG WHICH REGISTER IS CHOSEN

	// 2. Compile the index expression
	debugPrintf("--- Compiling Index: %s\n", node.Index.String()) // <<< DEBUG
	err = c.compileNode(node.Index)
	if err != nil {
		debugPrintf("<<< Exit compileIndexExpression (Error compiling index)\n") // <<< DEBUG EXIT
		// Note: Need to consider freeing baseReg here if it was allocated and valid
		return NewCompileError(node.Index, "error compiling index part of index expression").CausedBy(err)
	}
	// <<< DEBUG INDEX RESULT >>>
	indexRegFromState := c.lastExprReg
	indexRegFromCurrent := c.regAlloc.Current()
	debugPrintf("--- Index Compiled. lastExprReg: %s, lastExprRegValid: %t, regAlloc.Current(): %s\n", indexRegFromState, c.lastExprRegValid, indexRegFromCurrent)
	// --- Temporarily use Current() as per existing code for testing ---
	indexReg := c.regAlloc.Current()                                  // Keep existing logic for now
	debugPrintf("--- Using indexReg = %s (from Current)\n", indexReg) // <<< DEBUG WHICH REGISTER IS CHOSEN

	// 3. Allocate register for the result
	destReg := c.regAlloc.Alloc()
	debugPrintf("--- Allocated destReg = %s\n", destReg) // <<< DEBUG DEST REG

	// 4. Emit OpGetIndex
	debugPrintf("--- Emitting OpGetIndex %s, %s, %s (Dest, Base, Index)\n", destReg, arrayReg, indexReg) // <<< DEBUG EMIT
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(destReg))
	c.emitByte(byte(arrayReg)) // Using potentially wrong base register
	c.emitByte(byte(indexReg)) // Using potentially wrong index register

	// Free operand registers (REMOVED in original code - keep removed for now)
	// c.regAlloc.Free(arrayReg)
	// c.regAlloc.Free(indexReg)

	// Result is now in destReg
	c.regAlloc.SetCurrent(destReg) // Existing logic might rely on this?

	// --- Missing state update for the overall expression ---
	// c.lastExprReg = destReg
	// c.lastExprRegValid = true

	debugPrintf("<<< Exit compileIndexExpression (Success)\n") // <<< DEBUG EXIT
	return nil
}

// --- End Array/Index ---

// --- NEW: emitGetLength ---
func (c *Compiler) emitGetLength(dest, src Register, line int) {
	c.emitOpCode(vm.OpGetLength, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

// --- Error Helper ---

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
	case *parser.PrefixExpression:
		return n.Token // Operator token
	case *parser.InfixExpression:
		return n.Token // Operator token
	case *parser.IfExpression:
		return n.Token // 'if' token
	case *parser.FunctionLiteral:
		return n.Token // 'function' token
	case *parser.ArrowFunctionLiteral:
		return n.Token // '=>' token
	case *parser.CallExpression:
		return n.Token // '(' token
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

// compileSwitchStatement compiles a switch statement.
//
//	switch (expr) {
//	  case val1: body1; break;
//	  case val2: body2;
//	  default: bodyD; break;
//	}
//
// Compilation strategy:
//  1. Compile switch expression.
//  2. For each case:
//     a. Compile case value.
//     b. Compare with switch expression value (StrictEqual).
//     c. If not equal, jump to the *next* case test (OpJumpIfFalse).
//     d. If equal, execute the case body.
//     e. Handle break: Jumps to the end of the entire switch.
//     f. Implicit fallthrough means after a body (without break), execution continues to the next case test.
//  3. Handle default: If reached (all cases failed), execute default body.
//  4. Patch all jumps.
func (c *Compiler) compileSwitchStatement(node *parser.SwitchStatement) errors.PaseratiError {
	// 1. Compile the expression being switched on
	err := c.compileNode(node.Expression)
	if err != nil {
		return err
	}
	switchExprReg := c.regAlloc.Current()
	// Keep this register allocated until the end of the switch

	// List to hold the positions of OpJumpIfFalse instructions for each case test.
	// These jump to the *next* case test if the current one fails.
	caseTestFailJumps := []int{}

	// List to hold the positions of OpJump instructions that jump to the end of the switch
	// (used by break statements and potentially at the end of cases without breaks).
	jumpToEndPatches := []int{}

	// Find the default case (if any) - needed for patching the last case's jump
	defaultCaseBodyIndex := -1
	for i, caseClause := range node.Cases {
		if caseClause.Condition == nil { // This is the default case
			if defaultCaseBodyIndex != -1 {
				// Use the switch statement node for error reporting context
				c.addError(node, "Multiple default cases in switch statement")
				return nil // Indicate error occurred
			}
			defaultCaseBodyIndex = i
		}
	}

	// Push a context to handle break statements within the switch
	c.pushLoopContext(-1, -1) // -1 indicates no target for continue/loop start

	// --- Iterate through cases to emit comparison and body code ---
	caseBodyStartPositions := make([]int, len(node.Cases))

	for i, caseClause := range node.Cases {
		// Get line info directly from the token
		caseLine := caseClause.Token.Line

		// Patch jumps from *previous* failed case tests to point here
		for _, jumpPos := range caseTestFailJumps {
			c.patchJump(jumpPos)
		}
		caseTestFailJumps = []int{} // Clear the list for the current case

		if caseClause.Condition != nil { // Regular 'case expr:'
			// Compile the case condition
			err = c.compileNode(caseClause.Condition)
			if err != nil {
				return err
			}
			// Use Current() as CurrentAndFree is not available
			caseCondReg := c.regAlloc.Current()

			// Compare switch expression value with case condition value
			// Use Alloc() instead of Allocate()
			matchReg := c.regAlloc.Alloc()
			c.emitStrictEqual(matchReg, switchExprReg, caseCondReg, caseLine)

			// If no match, jump to the next case test (or default/end)
			jumpPos := c.emitPlaceholderJump(vm.OpJumpIfFalse, matchReg, caseLine)
			caseTestFailJumps = append(caseTestFailJumps, jumpPos)
			// Remove Free(), not available in current allocator
			c.regAlloc.Free(matchReg)

			// Record the start position of the body for potential jumps
			caseBodyStartPositions[i] = c.currentPosition()

			// Compile the case body
			err = c.compileNode(caseClause.Body)
			if err != nil {
				return err
			}
			// Implicit fallthrough *to the end* unless break exists.
			// Add a jump to the end, break will have already added its own.
			// Check if the last instruction was already a jump (from break/return)
			// This is tricky, let's always add the jump for now, might be redundant.
			endCaseJumpPos := c.emitPlaceholderJump(vm.OpJump, 0, caseLine) // 0 = unused reg for OpJump
			jumpToEndPatches = append(jumpToEndPatches, endCaseJumpPos)

		} else { // 'default:' case
			// Record the start position of the default body
			caseBodyStartPositions[i] = c.currentPosition()

			// Compile the default case body
			err = c.compileNode(caseClause.Body)
			if err != nil {
				return err
			}
			// Add jump to end (optional, could just fall out if it's last)
			endCaseJumpPos := c.emitPlaceholderJump(vm.OpJump, 0, caseLine) // 0 = unused reg for OpJump
			jumpToEndPatches = append(jumpToEndPatches, endCaseJumpPos)
		}
	}

	// Patch the last set of test failure jumps to point to the end of the switch
	for _, jumpPos := range caseTestFailJumps {
		c.patchJump(jumpPos)
	}

	// Remove unused variable
	// endSwitchPos := c.currentPosition()

	// Patch all break jumps and end-of-case jumps
	loopCtx := c.currentLoopContext()
	if loopCtx != nil { // Should always exist here
		for _, breakJumpPos := range loopCtx.BreakPlaceholderPosList {
			c.patchJump(breakJumpPos)
		}
	}
	for _, endJumpPos := range jumpToEndPatches {
		c.patchJump(endJumpPos)
	}

	// Pop the break context
	c.popLoopContext()

	// Remove Free(), not available in current allocator
	c.regAlloc.Free(switchExprReg)

	return nil
}

// --- REVISED: compileObjectLiteral (One-by-One Property Set) ---
func (c *Compiler) compileObjectLiteral(node *parser.ObjectLiteral) errors.PaseratiError {
	debugPrintf("Compiling Object Literal (One-by-One): %s\n", node.String())
	line := GetTokenFromNode(node).Line

	// 1. Create an empty object
	objReg := c.regAlloc.Alloc()
	c.emitMakeEmptyObject(objReg, line)

	// 2. Set properties one by one
	for _, prop := range node.Properties {
		// Compile Key (must evaluate to string constant for OpSetProp in Phase 1)
		var keyConstIdx uint16 = 0xFFFF // Invalid index marker
		switch keyNode := prop.Key.(type) {
		case *parser.Identifier:
			keyStr := keyNode.Value
			keyConstIdx = c.chunk.AddConstant(vm.String(keyStr))
		case *parser.StringLiteral:
			keyStr := keyNode.Value
			keyConstIdx = c.chunk.AddConstant(vm.String(keyStr))
		case *parser.NumberLiteral: // Allow number literal keys, convert to string
			keyStr := keyNode.TokenLiteral()
			keyConstIdx = c.chunk.AddConstant(vm.String(keyStr))
		default:
			// TODO: Handle computed keys [expr]. For Phase 1, only Ident/String/Number keys.
			// Computed keys would require compiling the expression, ensuring it's a string/number,
			// and potentially a different OpSetComputedProp or dynamic lookup within OpSetProp.
			return NewCompileError(prop.Key, fmt.Sprintf("compiler only supports identifier, string, or number literal keys in object literals (Phase 1), got %T", prop.Key))
		}

		// Compile Value into a temporary register
		err := c.compileNode(prop.Value)
		if err != nil {
			return err
		}
		// if !c.lastExprRegValid {
		// 	return NewCompileError(prop.Value, "expected expression for object property value")
		// }
		valueReg := c.regAlloc.Current() // Register holding the compiled value
		debugPrintf("--- OL Value Compiled. lastExprReg: %s, lastExprRegValid: %t, regAlloc.Current(): %s\n", valueReg, c.lastExprRegValid, c.regAlloc.Current())

		// Emit OpSetProp: objReg[keyConstIdx] = valueReg
		c.emitSetProp(objReg, valueReg, keyConstIdx, line)

		// Free the temporary value register if it's safe to do so
		// (Depends on how regAlloc and expression compilation manage registers.
		// Assuming lastExprReg is available for freeing after use here).
		c.regAlloc.Free(valueReg) // Free the register used for the value
	}

	// The object is fully constructed in objReg
	c.regAlloc.SetCurrent(objReg)
	c.lastExprReg = objReg
	c.lastExprRegValid = true
	return nil
}

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

// --- NEW: emitRemainder and emitExponent ---
func (c *Compiler) emitRemainder(dest, left, right Register, line int) {
	// REMOVED: c.stats.BytesGenerated += 4
	c.emitOpCode(vm.OpRemainder, line) // Fixed: Use emitOpCode
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitExponent(dest, left, right Register, line int) {
	// REMOVED: c.stats.BytesGenerated += 4
	c.emitOpCode(vm.OpExponent, line) // Fixed: Use emitOpCode
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

// --- END NEW ---

// --- NEW: Bitwise/Shift Emit Helpers ---

func (c *Compiler) emitBitwiseNot(dest, src Register, line int) {
	c.emitOpCode(vm.OpBitwiseNot, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

func (c *Compiler) emitBitwiseAnd(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpBitwiseAnd, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitBitwiseOr(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpBitwiseOr, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitBitwiseXor(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpBitwiseXor, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitShiftLeft(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpShiftLeft, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitShiftRight(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpShiftRight, line) // Arithmetic shift
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitUnsignedShiftRight(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpUnsignedShiftRight, line) // Logical shift
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

// emitMakeEmptyObject emits OpMakeEmptyObject DestReg
func (c *Compiler) emitMakeEmptyObject(dest Register, line int) {
	c.emitOpCode(vm.OpMakeEmptyObject, line) // Use the placeholder opcode
	c.emitByte(byte(dest))
}

// emitGetProp emits OpGetProp DestReg, ObjReg, NameConstIdx(Uint16)
func (c *Compiler) emitGetProp(dest, obj Register, nameConstIdx uint16, line int) {
	c.emitOpCode(vm.OpGetProp, line) // Use the placeholder opcode
	c.emitByte(byte(dest))
	c.emitByte(byte(obj))
	c.emitUint16(nameConstIdx)
}

// emitSetProp emits OpSetProp ObjReg, ValueReg, NameConstIdx(Uint16)
// Note: The order ObjReg, ValueReg, NameIdx seems reasonable for VM stack manipulation.
func (c *Compiler) emitSetProp(obj, val Register, nameConstIdx uint16, line int) {
	c.emitOpCode(vm.OpSetProp, line) // Use the placeholder opcode
	c.emitByte(byte(obj))
	c.emitByte(byte(val))
	c.emitUint16(nameConstIdx)
}

// --- END REVISED/NEW ---

// compileMemberExpression compiles expressions like obj.prop or obj['prop'] (latter is future work)
func (c *Compiler) compileMemberExpression(node *parser.MemberExpression) errors.PaseratiError {
	// 1. Compile the object part
	err := c.compileNode(node.Object)
	if err != nil {
		return NewCompileError(node.Object, "error compiling object part of member expression").CausedBy(err)
	}
	objectReg := c.regAlloc.Current() // Register holding the object

	// 2. Get Property Name (Assume Identifier for now: obj.prop)
	propIdent := node.Property
	// if !ok {
	// 	// TODO: Handle computed member access obj[expr] later.
	// 	return NewCompileError(node.Property, "compiler only supports identifier properties for member access (e.g., obj.prop)")
	// }
	propertyName := propIdent.Value

	// 3. <<< NEW: Special case for .length >>>
	if propertyName == "length" {
		// Check the static type provided by the checker
		objectStaticType := node.Object.GetComputedType()
		if objectStaticType == nil {
			// This might happen if type checking failed earlier, but Compile should have caught it.
			// Still, good to have a safeguard.
			return NewCompileError(node.Object, "compiler internal error: checker did not provide type for object in member expression")
		}

		// Widen the type to handle cases like `string | null` having `.length`
		widenedObjectType := types.GetWidenedType(objectStaticType)

		// Check if the widened type supports .length
		_, isArray := widenedObjectType.(*types.ArrayType)
		if isArray || widenedObjectType == types.String {
			// Emit specialized OpGetLength
			destReg := c.regAlloc.Alloc()
			c.emitGetLength(destReg, objectReg, node.Token.Line)
			c.regAlloc.SetCurrent(destReg) // Result is now in destReg
			// Free objectReg? Maybe not needed if GetLength copies or doesn't invalidate.
			// c.regAlloc.Free(objectReg)
			return nil // Handled by OpGetLength
		}
		// If type doesn't support .length, fall through to generic OpGetProp
		// The type checker *should* have caught this, but OpGetProp will likely return undefined/error at runtime.
		debugPrintf("// DEBUG CompileMember: .length requested on non-array/string type %s (widened from %s). Falling through to OpGetProp.\n",
			widenedObjectType.String(), objectStaticType.String())
	}
	// --- END Special case for .length ---

	// 4. Add property name string to constant pool (for generic OpGetProp)
	nameConstIdx := c.chunk.AddConstant(vm.String(propertyName))

	// 5. Allocate destination register for the result
	destReg := c.regAlloc.Alloc()

	// 6. Emit OpGetProp (Generic case)
	c.emitGetProp(destReg, objectReg, nameConstIdx, node.Token.Line) // Use '.' token line

	// Free the object register? Maybe not, it might be needed later.
	// c.regAlloc.Free(objectReg)

	// Result is now in destReg
	c.regAlloc.SetCurrent(destReg) // Set current register
	// Note: We don't need to set lastExprReg/Valid here, as compileNode will handle it.
	return nil
}

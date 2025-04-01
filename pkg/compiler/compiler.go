package compiler

import (
	"fmt"
	"math"
	"paserati/pkg/checker" // <<< Added import
	"paserati/pkg/parser"
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

// Compiler transforms an AST into bytecode.
type Compiler struct {
	chunk              *vm.Chunk
	regAlloc           *RegisterAllocator
	currentSymbolTable *SymbolTable // Changed from symbolTable
	// Add scope management later (stack of symbol tables)
	enclosing   *Compiler // Pointer to the enclosing compiler instance (nil for global)
	freeSymbols []*Symbol // Symbols resolved in outer scopes (captured)
	errors      []string

	// Tracking for implicit return from last expression statement in top level
	lastExprReg      Register
	lastExprRegValid bool

	// --- New: Stack for loop contexts ---
	loopContextStack []*LoopContext

	// --- New: Name of the function being compiled (for recursive lookup) ---
	compilingFuncName string
}

// NewCompiler creates a new *top-level* Compiler.
func NewCompiler() *Compiler {
	return &Compiler{
		chunk:              vm.NewChunk(),
		regAlloc:           NewRegisterAllocator(),
		currentSymbolTable: NewSymbolTable(), // Initialize global symbol table
		enclosing:          nil,              // No enclosing compiler for the top level
		freeSymbols:        []*Symbol{},      // Initialize empty free symbols slice
		errors:             []string{},
		lastExprRegValid:   false,                   // Initialize tracking fields
		loopContextStack:   make([]*LoopContext, 0), // Initialize loop stack
		compilingFuncName:  "<script>",              // Indicate top-level script context
	}
}

// newFunctionCompiler creates a compiler instance specifically for a function body.
func newFunctionCompiler(enclosingCompiler *Compiler) *Compiler {
	// Determine the best name for the function being compiled
	// (Needs the node and nameHint from the call site... how to pass?)
	// For now, let's leave it blank and set it later where it's called.
	// We will set it inside compileFunctionLiteral / compileArrowFunctionLiteral
	return &Compiler{
		chunk:              vm.NewChunk(),                                                // Function gets its own chunk
		regAlloc:           NewRegisterAllocator(),                                       // Function gets its own registers
		currentSymbolTable: NewEnclosedSymbolTable(enclosingCompiler.currentSymbolTable), // Enclosed scope
		enclosing:          enclosingCompiler,                                            // Link to the outer compiler
		freeSymbols:        []*Symbol{},                                                  // Initialize empty free symbols slice
		errors:             []string{},                                                   // Function compilation might have errors
		loopContextStack:   make([]*LoopContext, 0),                                      // Function gets its own loop stack
		compilingFuncName:  "",                                                           // Initialize as empty, set by caller
		// lastExprReg tracking only needed for top-level
	}
}

// Compile traverses the AST, performs type checking, and generates bytecode.
// Returns the generated chunk and any errors encountered (including type errors).
func (c *Compiler) Compile(node parser.Node) (*vm.Chunk, []string) {

	// --- Type Checking Step ---
	program, ok := node.(*parser.Program)
	if !ok {
		// Compiler currently expects the root node to be a Program.
		// If not, it cannot type check. Return an immediate error.
		// This might need adjustment if Compile is ever called on subtrees directly.
		return nil, []string{"compiler error: Compile input must be *parser.Program for type checking"}
	}

	typeChecker := checker.NewChecker()
	typeErrors := typeChecker.Check(program)
	if len(typeErrors) > 0 {
		// Found type errors. Convert them to strings and return immediately.
		errorStrings := make([]string, len(typeErrors))
		for i, err := range typeErrors {
			errorStrings[i] = err.Error() // Use the Error() method from TypeError
		}
		// Optionally append to existing compiler errors? For now, just return type errors.
		return nil, errorStrings
	}
	// --- End Type Checking Step ---

	// --- Bytecode Compilation Step ---
	// Reset is now implicit when a new Compiler is made for a function
	// c.regAlloc.Reset()
	// c.symbolTable = NewSymbolTable() // Reset symbol table for new compilation

	// Type checking passed, proceed to compile the AST.
	// Use the already type-checked program node.
	err := c.compileNode(program)
	if err != nil {
		c.errors = append(c.errors, err.Error()) // Add the final compile error if compileNode returns one
	}

	// Combine any remaining *compiler* errors (should be rare if type checking is robust)
	if len(c.errors) > 0 {
		return c.chunk, c.errors
	}

	// Emit final return instruction.
	// For top level, return last expression value if valid, otherwise undefined.
	// For functions, always return undefined implicitly (explicit return handled earlier).
	if c.enclosing == nil { // Top-level script
		if c.lastExprRegValid {
			c.emitReturn(c.lastExprReg, 0) // Use line 0 for implicit final return
		} else {
			c.emitOpCode(vm.OpReturnUndefined, 0) // Use line 0 for implicit final return
		}
	} else {
		// Inside a function, OpReturn or OpReturnUndefined should have been emitted by
		// compileReturnStatement or compileFunctionLiteral's finalization.
		// We still add one just in case there's a path without a return.
		c.emitOpCode(vm.OpReturnUndefined, 0)
	}

	return c.chunk, nil // Return chunk and nil errors on success
}

// compileNode dispatches compilation to the appropriate method based on node type.
func (c *Compiler) compileNode(node parser.Node) error {
	switch node := node.(type) {
	case *parser.Program:
		if c.enclosing == nil {
			c.lastExprRegValid = false // Reset for the program start
		}
		for _, stmt := range node.Statements {
			err := c.compileNode(stmt)
			if err != nil {
				return err // Propagate errors up
			}
		}

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

	// --- Statements ---
	case *parser.ExpressionStatement:
		err := c.compileNode(node.Expression)
		if err != nil {
			return err
		}
		if c.enclosing == nil { // If at top level, track this as potential final value
			c.lastExprReg = c.regAlloc.Current()
			c.lastExprRegValid = true
		}
		// Result register is left allocated, potentially unused otherwise.
		// TODO: Consider freeing registers?

	case *parser.LetStatement:
		if c.enclosing == nil {
			c.lastExprRegValid = false // Declarations don't provide final value
		}
		return c.compileLetStatement(node)

	case *parser.ConstStatement:
		if c.enclosing == nil {
			c.lastExprRegValid = false // Declarations don't provide final value
		}
		return c.compileConstStatement(node)

	case *parser.ReturnStatement:
		if c.enclosing == nil {
			c.lastExprRegValid = false // Explicit return overrides implicit
		}
		return c.compileReturnStatement(node)

	case *parser.WhileStatement:
		if c.enclosing == nil {
			c.lastExprRegValid = false
		}
		return c.compileWhileStatement(node)

	case *parser.ForStatement:
		if c.enclosing == nil {
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

	case *parser.FunctionLiteral: // Handle Function *Declarations* (when used as a statement)
		// This handles `function foo() {}` syntax at the statement level.
		// Function literals used in expressions (e.g., assignments) are handled below.
		if node.Name == nil {
			// Anonymous function used as a statement - likely an error or useless code.
			// For now, we could compile it but it won't be callable.
			// Or return an error?
			return fmt.Errorf("line %d: anonymous function used as statement", node.Token.Line)
		}

		if c.enclosing == nil {
			c.lastExprRegValid = false // Function declarations don't produce a value
		}

		// --- Handle named function recursion ---
		// 1. Define the function name temporarily.
		c.currentSymbolTable.Define(node.Name.Value, nilRegister)

		// 2. Compile the function literal body.
		//    Pass the variable name (f) as the hint for the function object's name
		//    if the function literal itself is anonymous.
		err := c.compileFunctionLiteral(node, node.Name.Value) // Pass name as hint
		if err != nil {
			return err
		}
		valueReg := c.regAlloc.Current() // Register holding the closure

		// 3. Update the symbol table entry.
		c.currentSymbolTable.UpdateRegister(node.Name.Value, valueReg)

	// --- Expressions ---
	case *parser.NumberLiteral:
		destReg := c.regAlloc.Alloc()
		c.emitLoadNewConstant(destReg, vm.Number(node.Value), node.Token.Line)

	case *parser.StringLiteral:
		destReg := c.regAlloc.Alloc()
		c.emitLoadNewConstant(destReg, vm.String(node.Value), node.Token.Line)

	case *parser.BooleanLiteral:
		destReg := c.regAlloc.Alloc()
		if node.Value {
			c.emitLoadTrue(destReg, node.Token.Line)
		} else {
			c.emitLoadFalse(destReg, node.Token.Line)
		}

	case *parser.NullLiteral:
		destReg := c.regAlloc.Alloc()
		c.emitLoadNull(destReg, node.Token.Line)

	case *parser.Identifier:
		// Use currentSymbolTable for resolution
		scopeName := "Function"
		if c.currentSymbolTable.Outer == nil {
			scopeName = "Global"
		}
		symbolRef, definingTable, found := c.currentSymbolTable.Resolve(node.Value)
		if !found {
			return fmt.Errorf("line %d: undefined variable '%s'", node.Token.Line, node.Value)
		}

		// Check if the symbol is defined in an outer scope (a free variable)
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
			freeVarIndex := c.addFreeSymbol(&symbolRef)
			destReg := c.regAlloc.Alloc()
			c.emitOpCode(vm.OpLoadFree, node.Token.Line)
			c.emitByte(byte(destReg))
			c.emitByte(byte(freeVarIndex))
		} else if !isLocal {
			// This is a regular free variable (defined in an outer scope)
			freeVarIndex := c.addFreeSymbol(&symbolRef)
			destReg := c.regAlloc.Alloc()
			c.emitOpCode(vm.OpLoadFree, node.Token.Line)
			c.emitByte(byte(destReg))
			c.emitByte(byte(freeVarIndex))
		} else {
			// This is a standard local or global variable (handled by current stack frame)
			srcReg := symbolRef.Register
			// --- PANIC CHECK --- Check if srcReg is the nilRegister unexpectedly
			if srcReg == nilRegister {
				// This panic should now be unreachable if the logic is correct
				panic(fmt.Sprintf("compiler internal error: resolved local variable '%s' to nilRegister R%d unexpectedly at line %d", node.Value, srcReg, node.Token.Line))
			}
			// --- END PANIC CHECK ---
			destReg := c.regAlloc.Alloc()
			c.emitMove(destReg, srcReg, node.Token.Line)
		}

	case *parser.PrefixExpression:
		return c.compilePrefixExpression(node)

	case *parser.InfixExpression:
		return c.compileInfixExpression(node)

	case *parser.ArrowFunctionLiteral:
		// Arrow functions are always anonymous expressions
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

	default:
		return fmt.Errorf("compilation not implemented for %T", node)
	}
	return nil // Return nil on success if no specific error occurred in this frame
}

// --- Statement Compilation ---

// Define a placeholder register value for 'undefined' case
// Also used temporarily for recursive function definition
const nilRegister Register = 255 // Or another value guaranteed not to be used

func (c *Compiler) compileLetStatement(node *parser.LetStatement) error {
	var valueReg Register = nilRegister
	var err error
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

func (c *Compiler) compileConstStatement(node *parser.ConstStatement) error {
	if node.Value == nil { /* ... error ... */
	}
	var valueReg Register = nilRegister
	var err error
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

func (c *Compiler) compileReturnStatement(node *parser.ReturnStatement) error {
	if node.ReturnValue != nil {
		var err error
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

func (c *Compiler) compilePrefixExpression(node *parser.PrefixExpression) error {
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
	default:
		return fmt.Errorf("line %d: unknown prefix operator '%s'", node.Token.Line, node.Operator)
	}

	// The result is now in destReg
	return nil
}

func (c *Compiler) compileInfixExpression(node *parser.InfixExpression) error {
	line := node.Token.Line // Use operator token line number

	// --- Standard binary operators (arithmetic, comparison) ---
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
		case "+":
			c.emitAdd(destReg, leftReg, rightReg, line)
			break
		case "-":
			c.emitSubtract(destReg, leftReg, rightReg, line)
			break
		case "*":
			c.emitMultiply(destReg, leftReg, rightReg, line)
			break
		case "/":
			c.emitDivide(destReg, leftReg, rightReg, line)
			break
		case "<=":
			c.emitLessEqual(destReg, leftReg, rightReg, line)
			break
		case ">=":
			// Implement as !(left < right)
			tempReg := c.regAlloc.Alloc() // Temp register for (left < right)
			c.emitLess(tempReg, leftReg, rightReg, line)
			c.emitNot(destReg, tempReg, line) // destReg = !(tempReg)
			// Allocator should ideally handle freeing tempReg if needed, or maybe release manually?
			break
		case "==":
			c.emitEqual(destReg, leftReg, rightReg, line)
			break
		case "!=":
			c.emitNotEqual(destReg, leftReg, rightReg, line)
			break
		case "<":
			c.emitLess(destReg, leftReg, rightReg, line)
			break
		case ">":
			c.emitGreater(destReg, leftReg, rightReg, line)
			break
		case "===":
			c.emitStrictEqual(destReg, leftReg, rightReg, line)
			break
		case "!==":
			c.emitStrictNotEqual(destReg, leftReg, rightReg, line)
			break
		default:
			return fmt.Errorf("line %d: unknown standard infix operator '%s'", line, node.Operator)
		}
		// Result is now in destReg (allocator current is destReg)
		c.regAlloc.SetCurrent(destReg)
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
		// Jump over the false path's move
		jumpSkipFalseMovePlaceholder := c.emitPlaceholderJump(vm.OpJump, 0, line)

		// Patch jumpToEndPlaceholder to land here (false path)
		c.patchJump(jumpToEndPlaceholder)
		// Result is left, move leftReg to destReg
		c.emitMove(destReg, leftReg, line) // If false path, result is left

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
		// Jump over the skip-right landing pad
		jumpEndPlaceholder := c.emitPlaceholderJump(vm.OpJump, 0, line)

		// --- Skip Right Path ---
		// Land here if left was NOT nullish. Patch the jump from the undefined check.
		c.patchJump(jumpSkipRightPlaceholder)
		// Result is already correctly in leftReg. Move it to destReg.
		c.emitMove(destReg, leftReg, line)

		// Land here after either path finishes. Patch the jump from the right-eval path.
		c.patchJump(jumpEndPlaceholder)

		// Release temporary registers (if allocator had a mechanism)
		// c.regAlloc.Release(isNullReg)
		// c.regAlloc.Release(isUndefReg)
		// c.regAlloc.Release(nullConstReg)
		// c.regAlloc.Release(undefConstReg)

		// Unified result is now in destReg
		c.regAlloc.SetCurrent(destReg)
		return nil
	}

	// Should be unreachable
	return fmt.Errorf("line %d: logical/coalescing operator '%s' compilation fell through", line, node.Operator)
}

func (c *Compiler) compileFunctionLiteral(node *parser.FunctionLiteral, nameHint string) error {
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
		c.errors = append(c.errors, err.Error())
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
	destReg := c.regAlloc.Alloc()                                             // Register for the resulting closure object in the outer scope
	fmt.Printf("// [Closure %s] Allocated destReg: R%d\n", funcName, destReg) // DEBUG
	c.emitOpCode(vm.OpClosure, node.Token.Line)
	c.emitByte(byte(destReg))
	c.emitUint16(constIdx)             // Operand 1: Constant index of the function blueprint
	c.emitByte(byte(len(freeSymbols))) // Operand 2: Number of upvalues to capture

	// Emit operands for each upvalue
	for i, freeSym := range freeSymbols {
		fmt.Printf("// [Closure Loop %s] Checking freeSym[%d]: %s (Reg %d) against funcNameForLookup: '%s'\n", funcName, i, freeSym.Name, freeSym.Register, funcNameForLookup) // DEBUG

		// --- Check for self-capture first (Simplified Check) ---
		// If a free symbol has nilRegister, it MUST be the temporary one
		// added for recursion resolution. It signifies self-capture.
		if freeSym.Register == nilRegister {
			// This is the special self-capture case identified during body compilation.
			fmt.Printf("// [Closure SelfCapture %s] Symbol '%s' has nilRegister. Emitting isLocal=1, index=destReg=R%d\n", funcName, freeSym.Name, destReg) // DEBUG
			c.emitByte(1)                                                                                                                                   // isLocal = true (capture from the stack where the closure will be placed)
			c.emitByte(byte(destReg))                                                                                                                       // Index = the destination register of OpClosure
			continue                                                                                                                                        // Skip the normal lookup below
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
			fmt.Printf("// [Closure Loop %s] Free '%s' is Local in enclosing. Emitting isLocal=1, index=R%d\n", funcName, freeSym.Name, enclosingSymbol.Register) // DEBUG
			// The free variable is local in the *direct* enclosing scope.
			c.emitByte(1) // isLocal = true
			// Capture the value from the enclosing scope's actual register
			c.emitByte(byte(enclosingSymbol.Register)) // Index = register index
		} else {
			// The free variable is also a free variable in the enclosing scope.
			// We need to capture it from the enclosing scope's upvalues.
			// We need the index of this symbol within the *enclosing* compiler's freeSymbols list.
			enclosingFreeIndex := c.addFreeSymbol(&enclosingSymbol)                                                                                        // Use the same helper
			fmt.Printf("// [Closure Loop %s] Free '%s' is Outer in enclosing. Emitting isLocal=0, index=%d\n", funcName, freeSym.Name, enclosingFreeIndex) // DEBUG
			c.emitByte(0)                                                                                                                                  // isLocal = false
			c.emitByte(byte(enclosingFreeIndex))                                                                                                           // Index = upvalue index in enclosing scope
		}
	}

	return nil // Return nil even if there were body errors, errors are collected in c.errors
}

func (c *Compiler) compileCallExpression(node *parser.CallExpression) error {
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

func (c *Compiler) compileIfExpression(node *parser.IfExpression) error {
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
func (c *Compiler) compileTernaryExpression(node *parser.TernaryExpression) error {
	// 1. Compile condition
	err := c.compileNode(node.Condition)
	if err != nil {
		return err
	}
	conditionReg := c.regAlloc.Current()

	// 2. Jump if false
	jumpFalsePos := c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)

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

	// Now Current() correctly points to finalReg which holds the unified result.
	return nil
}

// compileAssignmentExpression compiles identifier = value
func (c *Compiler) compileAssignmentExpression(node *parser.AssignmentExpression) error {
	line := node.Token.Line

	// 1. Ensure left-hand side is an identifier (parser check helps, but double-check)
	ident, ok := node.Left.(*parser.Identifier)
	if !ok {
		return fmt.Errorf("line %d: invalid assignment target, expected identifier got %T", line, node.Left)
	}

	// 2. Resolve the identifier to find its storage location (register or upvalue)
	symbolRef, definingTable, found := c.currentSymbolTable.Resolve(ident.Value)
	if !found {
		return fmt.Errorf("line %d: assignment to undeclared variable '%s'", line, ident.Value)
	}

	// 3. Determine target register and load current value if needed (for compound ops or upvalues)
	var targetReg Register
	isUpvalue := false
	var upvalueIndex uint8

	if definingTable == c.currentSymbolTable {
		// Local variable: Get its assigned register.
		targetReg = symbolRef.Register
		// If it's a compound assignment, the existing value in targetReg is used directly.
		// If it's simple assignment '=', targetReg will be overwritten later.
	} else {
		// Free variable (upvalue): Find its index.
		isUpvalue = true
		upvalueIndex = c.addFreeSymbol(&symbolRef)
		// Allocate a register to hold the current value loaded from the upvalue.
		targetReg = c.regAlloc.Alloc()
		// Emit OpLoadFree manually
		c.emitOpCode(vm.OpLoadFree, line)
		c.emitByte(byte(targetReg)) // Destination register
		c.emitByte(upvalueIndex)    // Upvalue index
	}

	// 4. Compile the value expression (right-hand side)
	err := c.compileNode(node.Value)
	if err != nil {
		return err
	}
	valueReg := c.regAlloc.Current() // RHS Value is in this register

	// 5. Perform operation if compound assignment, or move for simple assignment
	switch node.Operator {
	case "+=":
		c.emitAdd(targetReg, targetReg, valueReg, line) // target = target + value
	case "-=":
		c.emitSubtract(targetReg, targetReg, valueReg, line) // target = target - value
	case "*=":
		c.emitMultiply(targetReg, targetReg, valueReg, line) // target = target * value
	case "/=":
		c.emitDivide(targetReg, targetReg, valueReg, line) // target = target / value
	case "=":
		// Simple assignment: Move RHS value into target register.
		// If it was an upvalue, targetReg holds the *loaded* current value,
		// but we want to overwrite it with the new valueReg before SetUpvalue.
		c.emitMove(targetReg, valueReg, line)
	default:
		return fmt.Errorf("line %d: unsupported assignment operator '%s'", line, node.Operator)
	}
	// Result of operation (or move) is now in targetReg

	// 6. Store result back if it was an upvalue
	if isUpvalue {
		c.emitSetUpvalue(upvalueIndex, targetReg, line)
	}

	// 7. Assignment expression evaluates to the assigned value (now in targetReg).
	c.regAlloc.SetCurrent(targetReg)

	return nil
}

// --- Loop Compilation (Updated) ---

func (c *Compiler) compileWhileStatement(node *parser.WhileStatement) error {
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
		return fmt.Errorf("error compiling while condition: %w", err)
	}
	conditionReg := c.regAlloc.Current()

	// --- Jump Out If False ---
	jumpToEndPlaceholderPos := c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, line)

	// --- Compile Body ---
	err = c.compileNode(node.Body)
	if err != nil {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1] // Pop context on error
		return fmt.Errorf("error compiling while body: %w", err)
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
			return fmt.Errorf("internal compiler error: continue jump offset %d exceeds 16-bit limit at line %d", targetOffset, node.Token.Line)
		}
		// Manually write the 16-bit offset into the placeholder jump instruction
		c.chunk.Code[continuePos+1] = byte(int16(targetOffset) >> 8)   // High byte
		c.chunk.Code[continuePos+2] = byte(int16(targetOffset) & 0xFF) // Low byte
	}

	return nil
}

func (c *Compiler) compileForStatement(node *parser.ForStatement) error {
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

func (c *Compiler) compileBreakStatement(node *parser.BreakStatement) error {
	if len(c.loopContextStack) == 0 {
		return fmt.Errorf("line %d: break statement not within a loop", node.Token.Line)
	}

	// Get current loop context (top of stack)
	currentLoopContext := c.loopContextStack[len(c.loopContextStack)-1]

	// Emit placeholder jump (OpJump) - Pass 0 for srcReg as it's ignored
	placeholderPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)

	// Add placeholder position to the context's list for later patching
	currentLoopContext.BreakPlaceholderPosList = append(currentLoopContext.BreakPlaceholderPosList, placeholderPos)

	return nil
}

func (c *Compiler) compileContinueStatement(node *parser.ContinueStatement) error {
	if len(c.loopContextStack) == 0 {
		return fmt.Errorf("line %d: continue statement not within a loop", node.Token.Line)
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
func (c *Compiler) addFreeSymbol(symbol *Symbol) uint8 { // Assuming max 256 free vars for now
	for i, free := range c.freeSymbols {
		if free == symbol { // Pointer comparison should work if Resolve returns the same Symbol instance
			return uint8(i)
		}
	}
	// Check if we exceed limit (important for OpLoadFree operand size)
	if len(c.freeSymbols) >= 256 {
		// Handle error: too many free variables
		// For now, let's panic or add an error; proper error handling needed
		c.errors = append(c.errors, "compiler: too many free variables in function")
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
func (c *Compiler) compileArrowFunctionLiteral(node *parser.ArrowFunctionLiteral) error {
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
			funcCompiler.errors = append(funcCompiler.errors, err.Error())
		}
		implicitReturnNeeded = false // Block handles its own returns or falls through
	case parser.Expression:
		err := funcCompiler.compileNode(bodyNode)
		if err != nil {
			funcCompiler.errors = append(funcCompiler.errors, err.Error())
			returnReg = 0 // Indicate error or inability to get result reg
		} else {
			returnReg = funcCompiler.regAlloc.Current()
		}
		implicitReturnNeeded = true // Expression body needs implicit return
	default:
		funcCompiler.errors = append(funcCompiler.errors, fmt.Sprintf("invalid body type %T for arrow function", node.Body))
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
	destReg := c.regAlloc.Alloc()                                                                   // Register for the resulting closure object in the outer scope
	fmt.Printf("// [Closure %s] Allocated destReg: R%d\n", funcCompiler.compilingFuncName, destReg) // DEBUG
	c.emitOpCode(vm.OpClosure, node.Token.Line)
	c.emitByte(byte(destReg))
	c.emitUint16(constIdx)             // Operand 1: Constant index of the function blueprint
	c.emitByte(byte(len(freeSymbols))) // Operand 2: Number of upvalues to capture

	// Emit operands for each upvalue
	for i, freeSym := range freeSymbols {
		fmt.Printf("// [Closure Loop %s] Checking freeSym[%d]: %s (Reg %d) against funcNameForLookup: '%s'\n", funcCompiler.compilingFuncName, i, freeSym.Name, freeSym.Register, funcCompiler.compilingFuncName) // DEBUG

		// --- Check for self-capture first (Simplified Check) ---
		// If a free symbol has nilRegister, it MUST be the temporary one
		// added for recursion resolution. It signifies self-capture.
		if freeSym.Register == nilRegister {
			// This is the special self-capture case identified during body compilation.
			fmt.Printf("// [Closure SelfCapture %s] Symbol '%s' has nilRegister. Emitting isLocal=1, index=destReg=R%d\n", funcCompiler.compilingFuncName, freeSym.Name, destReg) // DEBUG
			c.emitByte(1)                                                                                                                                                         // isLocal = true (capture from the stack where the closure will be placed)
			c.emitByte(byte(destReg))                                                                                                                                             // Index = the destination register of OpClosure
			continue                                                                                                                                                              // Skip the normal lookup below
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
			fmt.Printf("// [Closure Loop %s] Free '%s' is Local in enclosing. Emitting isLocal=1, index=R%d\n", funcCompiler.compilingFuncName, freeSym.Name, enclosingSymbol.Register) // DEBUG
			// The free variable is local in the *direct* enclosing scope.
			c.emitByte(1) // isLocal = true
			// Capture the value from the enclosing scope's actual register
			c.emitByte(byte(enclosingSymbol.Register)) // Index = register index
		} else {
			// The free variable is also a free variable in the enclosing scope.
			// We need to capture it from the enclosing scope's upvalues.
			// We need the index of this symbol within the *enclosing* compiler's freeSymbols list.
			enclosingFreeIndex := c.addFreeSymbol(&enclosingSymbol)                                                                                                              // Use the same helper
			fmt.Printf("// [Closure Loop %s] Free '%s' is Outer in enclosing. Emitting isLocal=0, index=%d\n", funcCompiler.compilingFuncName, freeSym.Name, enclosingFreeIndex) // DEBUG
			c.emitByte(0)                                                                                                                                                        // isLocal = false
			c.emitByte(byte(enclosingFreeIndex))                                                                                                                                 // Index = upvalue index in enclosing scope
		}
	}

	return nil // Return nil even if errors occurred; they are collected in c.errors
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

// --- New: DoWhile Statement Compilation ---

func (c *Compiler) compileDoWhileStatement(node *parser.DoWhileStatement) error {
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
		return fmt.Errorf("error compiling do-while body: %w", err)
	}

	// 4. Mark Condition Position (for clarity, not used directly in jump calcs below)
	_ = len(c.chunk.Code) // conditionPos := len(c.chunk.Code)

	// 5. Compile Condition
	if err := c.compileNode(node.Condition); err != nil {
		// Pop context if condition compilation fails
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return fmt.Errorf("error compiling do-while condition: %w", err)
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
		return fmt.Errorf("internal compiler error: do-while loop jump offset %d exceeds 16-bit limit at line %d", backOffset, line)
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
			return fmt.Errorf("internal compiler error: do-while continue jump offset %d exceeds 16-bit limit at line %d", targetOffset, line)
		}
		// Manually write the 16-bit offset into the placeholder OpJump instruction
		c.chunk.Code[continuePos+1] = byte(int16(targetOffset) >> 8)   // High byte
		c.chunk.Code[continuePos+2] = byte(int16(targetOffset) & 0xFF) // Low byte
	}

	return nil
}

// --- New: Update Expression Compilation ---

func (c *Compiler) compileUpdateExpression(node *parser.UpdateExpression) error {
	line := node.Token.Line

	// 1. Argument must be an identifier (parser should enforce, but check again)
	ident, ok := node.Argument.(*parser.Identifier)
	if !ok {
		return fmt.Errorf("line %d: invalid target for %s: expected identifier, got %T", line, node.Operator, node.Argument)
	}

	// 2. Resolve identifier and determine if local or upvalue
	symbolRef, definingTable, found := c.currentSymbolTable.Resolve(ident.Value)
	if !found {
		return fmt.Errorf("line %d: applying %s to undeclared variable '%s'", line, node.Operator, ident.Value)
	}

	var targetReg Register
	isUpvalue := false
	var upvalueIndex uint8

	if definingTable == c.currentSymbolTable {
		// Local variable: Get its register.
		targetReg = symbolRef.Register
		// If it's a compound assignment, the existing value in targetReg is used directly.
		// If it's simple assignment '=', targetReg will be overwritten later.
	} else {
		// Upvalue: Get its index and load current value into a temporary register.
		isUpvalue = true
		upvalueIndex = c.addFreeSymbol(&symbolRef)
		targetReg = c.regAlloc.Alloc()
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
		}
		// d. Result of expression is the *original* value (already saved in resultReg)
	}

	// Release temporary register for constant 1 (optional, depends on allocator)
	// c.regAlloc.Release(constOneReg)

	// 5. Set compiler's current register to the expression result
	c.regAlloc.SetCurrent(resultReg)
	return nil
}

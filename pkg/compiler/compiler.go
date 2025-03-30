package compiler

import (
	"fmt"
	"paseratti2/pkg/bytecode" // For token line numbers

	// For token line numbers
	"paseratti2/pkg/parser"
	"paseratti2/pkg/value"
)

// Compiler transforms an AST into bytecode.
type Compiler struct {
	chunk              *bytecode.Chunk
	regAlloc           *RegisterAllocator
	currentSymbolTable *SymbolTable // Changed from symbolTable
	// Add scope management later (stack of symbol tables)
	enclosing   *Compiler // Pointer to the enclosing compiler instance (nil for global)
	freeSymbols []*Symbol // Symbols resolved in outer scopes (captured)
	errors      []string

	// Tracking for implicit return from last expression statement in top level
	lastExprReg      Register
	lastExprRegValid bool
}

// NewCompiler creates a new *top-level* Compiler.
func NewCompiler() *Compiler {
	return &Compiler{
		chunk:              bytecode.NewChunk(),
		regAlloc:           NewRegisterAllocator(),
		currentSymbolTable: NewSymbolTable(), // Initialize global symbol table
		enclosing:          nil,              // No enclosing compiler for the top level
		freeSymbols:        []*Symbol{},      // Initialize empty free symbols slice
		errors:             []string{},
		lastExprRegValid:   false, // Initialize tracking fields
	}
}

// newFunctionCompiler creates a compiler instance specifically for a function body.
func newFunctionCompiler(enclosingCompiler *Compiler) *Compiler {
	return &Compiler{
		chunk:              bytecode.NewChunk(),                                          // Function gets its own chunk
		regAlloc:           NewRegisterAllocator(),                                       // Function gets its own registers
		currentSymbolTable: NewEnclosedSymbolTable(enclosingCompiler.currentSymbolTable), // Enclosed scope
		enclosing:          enclosingCompiler,                                            // Link to the outer compiler
		freeSymbols:        []*Symbol{},                                                  // Initialize empty free symbols slice
		errors:             []string{},                                                   // Function compilation might have errors
		// lastExprReg tracking only needed for top-level
	}
}

// Compile traverses the AST and generates bytecode.
// Returns the generated chunk and any errors encountered.
func (c *Compiler) Compile(node parser.Node) (*bytecode.Chunk, []string) {
	// Reset is now implicit when a new Compiler is made for a function
	// c.regAlloc.Reset()
	// c.symbolTable = NewSymbolTable() // Reset symbol table for new compilation

	err := c.compileNode(node)
	if err != nil {
		c.errors = append(c.errors, err.Error()) // Add the final error if compileNode returns one
	}

	// Emit final return instruction.
	// For top level, return last expression value if valid, otherwise undefined.
	// For functions, always return undefined implicitly (explicit return handled earlier).
	if c.enclosing == nil { // Top-level script
		if c.lastExprRegValid {
			c.emitReturn(c.lastExprReg, 0) // Use line 0 for implicit final return
		} else {
			c.emitOpCode(bytecode.OpReturnUndefined, 0) // Use line 0 for implicit final return
		}
	} else {
		// Inside a function, OpReturn or OpReturnUndefined should have been emitted by
		// compileReturnStatement or compileFunctionLiteral's finalization.
		// We still add one just in case there's a path without a return.
		c.emitOpCode(bytecode.OpReturnUndefined, 0)
	}

	// c.emitFinalReturn(0) // REMOVED - Replaced by the logic above

	return c.chunk, c.errors
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

	// --- Expressions ---
	case *parser.NumberLiteral:
		destReg := c.regAlloc.Alloc()
		c.emitLoadNewConstant(destReg, value.Number(node.Value), node.Token.Line)

	case *parser.StringLiteral:
		destReg := c.regAlloc.Alloc()
		c.emitLoadNewConstant(destReg, value.String(node.Value), node.Token.Line)

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
		symbolRef, definingTable, ok := c.currentSymbolTable.Resolve(node.Value)
		if !ok {
			return fmt.Errorf("line %d: undefined variable '%s'", node.Token.Line, node.Value)
		}

		// Check if the symbol is defined in an outer scope (a free variable)
		if definingTable != c.currentSymbolTable {
			// This is a free variable
			freeVarIndex := c.addFreeSymbol(&symbolRef) // Add to list and get index
			destReg := c.regAlloc.Alloc()               // Allocate register for the loaded value
			c.emitOpCode(bytecode.OpLoadFree, node.Token.Line)
			c.emitByte(byte(destReg))
			c.emitByte(byte(freeVarIndex))
			// fmt.Printf("// Emitted OpLoadFree R%d, Index %d (%s)\n", destReg, freeVarIndex, symbolRef.Name) // Keep for debug if needed
		} else {
			// This is a local or global variable (handled by current stack frame)
			// Current logic:
			srcReg := symbolRef.Register
			destReg := c.regAlloc.Alloc()
			c.emitMove(destReg, srcReg, node.Token.Line)
		}

		// Note: c.regAlloc.Current() should point to the register holding the value
		// whether loaded via OpMove or (eventually) OpLoadFree.

	case *parser.PrefixExpression:
		return c.compilePrefixExpression(node)

	case *parser.InfixExpression:
		return c.compileInfixExpression(node)

	case *parser.FunctionLiteral:
		return c.compileFunctionLiteral(node, "") // Pass empty name hint

	case *parser.CallExpression:
		return c.compileCallExpression(node)

	case *parser.IfExpression:
		return c.compileIfExpression(node)

	default:
		return fmt.Errorf("compiler: unhandled AST node type %T", node)
	}
	return nil // Success for this node
}

// --- Statement Compilation ---

// Define a placeholder register value for 'undefined' case
// Also used temporarily for recursive function definition
const nilRegister Register = 255 // Or another value guaranteed not to be used

func (c *Compiler) compileLetStatement(node *parser.LetStatement) error {
	var valueReg Register = nilRegister // Placeholder for uninitialized or function being defined
	var err error

	// Check if the value is a function literal BEFORE compiling value
	if funcLit, ok := node.Value.(*parser.FunctionLiteral); ok {
		// --- Handle named function recursion ---
		// 1. Define the function name in the current scope TEMPORARILY.
		//    It points to an invalid register initially (nilRegister).
		//    This allows the function body to resolve the name recursively.
		c.currentSymbolTable.Define(node.Name.Value, nilRegister)

		// 2. Compile the function literal body.
		//    This resolves free vars (including potentially the recursive name).
		//    It emits OpClosure into a specific register.
		err = c.compileFunctionLiteral(funcLit, node.Name.Value)
		if err != nil {
			return err
		}
		valueReg = c.regAlloc.Current() // Register holding the *actual* closure object

		// 3. Update the symbol table entry to point to the correct closure register.
		c.currentSymbolTable.UpdateRegister(node.Name.Value, valueReg)

	} else if node.Value != nil {
		// Compile other value types normally
		err = c.compileNode(node.Value)
		if err != nil {
			return err
		}
		valueReg = c.regAlloc.Current()
	} // else: node.Value is nil (implicit undefined handled below)

	// Handle implicit undefined initialization if needed
	if valueReg == nilRegister { // Check if still placeholder (only happens for `let x;`)
		undefReg := c.regAlloc.Alloc()
		c.emitLoadUndefined(undefReg, node.Name.Token.Line)
		valueReg = undefReg
		// Define the symbol ONLY if it wasn't a function defined above
		c.currentSymbolTable.Define(node.Name.Value, valueReg)
	} else if _, ok := node.Value.(*parser.FunctionLiteral); !ok {
		// If it wasn't a function, define the symbol now.
		// Function symbols were already defined/updated above.
		c.currentSymbolTable.Define(node.Name.Value, valueReg)
	}

	return nil
}

func (c *Compiler) compileConstStatement(node *parser.ConstStatement) error {
	// Const *must* have an initializer, parser should enforce this, or we check here.
	// Assuming parser guarantees node.Value is not nil for const.
	if node.Value == nil { // Defensive check, might not be needed if parser guarantees it
		return fmt.Errorf("line %d: constant '%s' must be initialized", node.Token.Line, node.Name.Value)
	}
	var valueReg Register = nilRegister // Placeholder for uninitialized
	var err error

	// Check if the value is a function literal
	if funcLit, ok := node.Value.(*parser.FunctionLiteral); ok {
		// --- Handle named function recursion ---
		c.currentSymbolTable.Define(node.Name.Value, nilRegister) // Define temporarily
		err = c.compileFunctionLiteral(funcLit, node.Name.Value)
		if err != nil {
			return err
		}
		valueReg = c.regAlloc.Current()
		c.currentSymbolTable.UpdateRegister(node.Name.Value, valueReg) // Update with correct register
	} else {
		// Compile other value types normally
		err = c.compileNode(node.Value)
		if err != nil {
			return err
		}
		valueReg = c.regAlloc.Current() // Get the register holding the computed value
	}
	// Define the variable
	c.currentSymbolTable.Define(node.Name.Value, valueReg) // Use currentSymbolTable
	return nil
}

func (c *Compiler) compileReturnStatement(node *parser.ReturnStatement) error {
	if node.ReturnValue != nil {
		err := c.compileNode(node.ReturnValue)
		if err != nil {
			return err
		}
		returnReg := c.regAlloc.Current() // Value to return is in the last allocated reg
		c.emitReturn(returnReg, node.Token.Line)
	} else {
		// Return undefined implicitly using the optimized opcode
		c.emitOpCode(bytecode.OpReturnUndefined, node.Token.Line)
		// No need to load undefined into a register first
		// undefReg := c.regAlloc.Alloc()
		// c.emitLoadUndefined(undefReg, node.Token.Line)
		// c.emitReturn(undefReg, node.Token.Line)
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
	// Compile left operand
	err := c.compileNode(node.Left)
	if err != nil {
		return err
	}
	leftReg := c.regAlloc.Current() // Register holding the left value

	// Compile right operand
	// IMPORTANT: For stack-like allocation, the right operand's result
	// will be in the *next* register.
	err = c.compileNode(node.Right)
	if err != nil {
		return err
	}
	rightReg := c.regAlloc.Current() // Register holding the right value

	// Allocate a register for the result of the infix operation
	destReg := c.regAlloc.Alloc()

	// Emit the corresponding binary opcode
	line := node.Token.Line // Use operator token line number
	switch node.Operator {
	case "+":
		c.emitAdd(destReg, leftReg, rightReg, line)
	case "-":
		c.emitSubtract(destReg, leftReg, rightReg, line)
	case "*":
		c.emitMultiply(destReg, leftReg, rightReg, line)
	case "/":
		c.emitDivide(destReg, leftReg, rightReg, line)
	case "==":
		c.emitEqual(destReg, leftReg, rightReg, line)
	case "!=":
		c.emitNotEqual(destReg, leftReg, rightReg, line)
	case "<":
		c.emitLess(destReg, leftReg, rightReg, line)
	case ">":
		c.emitGreater(destReg, leftReg, rightReg, line)
	case "<=":
		c.emitLessEqual(destReg, leftReg, rightReg, line)
	default:
		return fmt.Errorf("line %d: unknown infix operator '%s'", line, node.Operator)
	}

	// The result is now in destReg
	return nil
}

func (c *Compiler) compileFunctionLiteral(node *parser.FunctionLiteral, nameHint string) error {
	// 1. Create a new Compiler instance for the function body, linked to the current one
	functionCompiler := newFunctionCompiler(c) // Pass `c` as the enclosing compiler

	// 2. Define parameters in the function compiler's *enclosed* scope
	for _, param := range node.Parameters {
		reg := functionCompiler.regAlloc.Alloc()
		// Define in the function's own scope
		functionCompiler.currentSymbolTable.Define(param.Value, reg)
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
	funcName := nameHint                    // Use hint first
	if funcName == "" && node.Name != nil { // Fallback to node.Name if hint is empty and node has one
		funcName = node.Name.Value
	}
	funcObj := bytecode.Function{
		Arity:        len(node.Parameters),
		Chunk:        functionChunk,
		Name:         funcName,
		RegisterSize: int(regSize),
		// UpvalueCount: len(freeSymbols), // TODO: Add UpvalueCount field to bytecode.Function?
	}

	// 6. Add the function object to the *outer* compiler's constant pool.
	funcValue := value.NewFunction(&funcObj)   // Still use value.Function for the raw compiled code
	constIdx := c.chunk.AddConstant(funcValue) // Index of the function proto in outer chunk

	// 7. Emit OpClosure in the *outer* chunk.
	destReg := c.regAlloc.Alloc() // Register for the resulting closure object in the outer scope
	c.emitOpCode(bytecode.OpClosure, node.Token.Line)
	c.emitByte(byte(destReg))
	c.emitUint16(constIdx)             // Operand 1: Constant index of the function blueprint
	c.emitByte(byte(len(freeSymbols))) // Operand 2: Number of upvalues to capture

	// Emit operands for each upvalue
	for _, freeSym := range freeSymbols {
		// Resolve the symbol again in the *enclosing* compiler's context
		// to determine if it's local there or needs to be fetched from an outer upvalue.
		enclosingSymbol, enclosingTable, found := c.currentSymbolTable.Resolve(freeSym.Name)
		if !found {
			// This should theoretically not happen if it was resolved during body compilation
			// but handle defensively.
			panic(fmt.Sprintf("compiler internal error: free variable %s not found in enclosing scope during closure creation", freeSym.Name))
		}

		if enclosingTable == c.currentSymbolTable {
			// The free variable is local in the *direct* enclosing scope.
			c.emitByte(1) // isLocal = true
			// Check if this is the function capturing itself (recursion)
			if freeSym.Name == funcName {
				// Special case: Emit the *destination register* of OpClosure.
				// This signals the VM to capture the closure being created.
				c.emitByte(byte(destReg))
			} else {
				// Capture the value from the enclosing scope's actual register
				c.emitByte(byte(enclosingSymbol.Register)) // Index = register index
			}
		} else {
			// The free variable is also a free variable in the enclosing scope.
			// We need to capture it from the enclosing scope's upvalues.
			// We need the index of this symbol within the *enclosing* compiler's freeSymbols list.
			enclosingFreeIndex := c.addFreeSymbol(&enclosingSymbol) // Use the same helper
			c.emitByte(0)                                           // isLocal = false
			c.emitByte(byte(enclosingFreeIndex))                    // Index = upvalue index in enclosing scope
		}
	}

	// Note: OpLoadConst is no longer emitted here. OpClosure creates the runtime value.

	// Function definition using `let name = function(){}` or `const name = function(){}`
	// is handled by compileLetStatement/compileConstStatement which define the symbol
	// in the *outer* scope using the outer compiler's currentSymbolTable.
	// They will now store the closure created by OpClosure.
	// Only define the symbol here if the function was defined using `function name(){}` syntax.
	if nameHint == "" && node.Name != nil {
		// Define in the *outer* scope as well
		c.currentSymbolTable.Define(node.Name.Value, destReg) // destReg now holds the closure
	}

	return nil // Return nil even if there were body errors, errors are collected
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
	jumpIfFalsePos := c.emitPlaceholderJump(bytecode.OpJumpIfFalse, conditionReg, node.Token.Line)

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
		jumpElsePos := c.emitPlaceholderJump(bytecode.OpJump, 0, node.Consequence.Token.Line) // Use line of opening brace? Or token after consequence?

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

// --- Bytecode Emission Helpers ---

func (c *Compiler) emitOpCode(op bytecode.OpCode, line int) {
	c.chunk.WriteOpCode(op, line)
}

func (c *Compiler) emitByte(b byte) {
	c.chunk.WriteByte(b)
}

func (c *Compiler) emitUint16(val uint16) {
	c.chunk.WriteUint16(val)
}

func (c *Compiler) emitLoadConstant(dest Register, constIdx uint16, line int) {
	c.emitOpCode(bytecode.OpLoadConst, line)
	c.emitByte(byte(dest))
	c.emitUint16(constIdx)
}

func (c *Compiler) emitLoadNull(dest Register, line int) {
	c.emitOpCode(bytecode.OpLoadNull, line)
	c.emitByte(byte(dest))
}

func (c *Compiler) emitLoadUndefined(dest Register, line int) {
	c.emitOpCode(bytecode.OpLoadUndefined, line)
	c.emitByte(byte(dest))
}

func (c *Compiler) emitLoadTrue(dest Register, line int) {
	c.emitOpCode(bytecode.OpLoadTrue, line)
	c.emitByte(byte(dest))
}

func (c *Compiler) emitLoadFalse(dest Register, line int) {
	c.emitOpCode(bytecode.OpLoadFalse, line)
	c.emitByte(byte(dest))
}

func (c *Compiler) emitMove(dest, src Register, line int) {
	c.emitOpCode(bytecode.OpMove, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

func (c *Compiler) emitReturn(src Register, line int) {
	c.emitOpCode(bytecode.OpReturn, line)
	c.emitByte(byte(src))
}

func (c *Compiler) emitNegate(dest, src Register, line int) {
	c.emitOpCode(bytecode.OpNegate, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

func (c *Compiler) emitNot(dest, src Register, line int) {
	c.emitOpCode(bytecode.OpNot, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

func (c *Compiler) emitAdd(dest, left, right Register, line int) {
	c.emitOpCode(bytecode.OpAdd, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitSubtract(dest, left, right Register, line int) {
	c.emitOpCode(bytecode.OpSubtract, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitMultiply(dest, left, right Register, line int) {
	c.emitOpCode(bytecode.OpMultiply, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitDivide(dest, left, right Register, line int) {
	c.emitOpCode(bytecode.OpDivide, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitEqual(dest, left, right Register, line int) {
	c.emitOpCode(bytecode.OpEqual, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitNotEqual(dest, left, right Register, line int) {
	c.emitOpCode(bytecode.OpNotEqual, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitGreater(dest, left, right Register, line int) {
	c.emitOpCode(bytecode.OpGreater, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitLess(dest, left, right Register, line int) {
	c.emitOpCode(bytecode.OpLess, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitLessEqual(dest, left, right Register, line int) {
	c.emitOpCode(bytecode.OpLessEqual, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitCall(dest, funcReg Register, argCount byte, line int) {
	c.emitOpCode(bytecode.OpCall, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(funcReg))
	c.emitByte(argCount)
}

// emitFinalReturn adds the final OpReturnUndefined instruction.
func (c *Compiler) emitFinalReturn(line int) {
	// No need to load undefined first
	c.emitOpCode(bytecode.OpReturnUndefined, line)
}

// Overload or new function to handle adding constant and emitting load
func (c *Compiler) emitLoadNewConstant(dest Register, val value.Value, line int) {
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
func (c *Compiler) emitPlaceholderJump(op bytecode.OpCode, srcReg Register, line int) int {
	pos := len(c.chunk.Code)
	c.emitOpCode(op, line)
	if op == bytecode.OpJumpIfFalse {
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
	op := bytecode.OpCode(c.chunk.Code[placeholderPos])
	operandStartPos := placeholderPos + 1
	if op == bytecode.OpJumpIfFalse {
		operandStartPos = placeholderPos + 2 // Skip register byte
	}

	// Calculate offset from the position *after* the jump instruction
	jumpInstructionEndPos := operandStartPos + 2
	offset := len(c.chunk.Code) - jumpInstructionEndPos

	if offset > 65535 {
		// Handle error: jump offset too large
		panic("Compiler error: jump offset exceeds 16 bits")
	}

	// Write the 16-bit offset back into the placeholder bytes (Big Endian)
	c.chunk.Code[operandStartPos] = byte(offset >> 8)
	c.chunk.Code[operandStartPos+1] = byte(offset & 0xff)
}

// TODO: emitCall, etc.

package compiler

import (
	"fmt"
	"math"
	"paserati/pkg/errors"
	"paserati/pkg/parser"
	"paserati/pkg/vm"
)

func (c *Compiler) compileLetStatement(node *parser.LetStatement, hint Register) (Register, errors.PaseratiError) {
	// debug disabled
	var valueReg Register = nilRegister
	var err errors.PaseratiError
	isValueFunc := false // Flag to track if value is a function literal

	if funcLit, ok := node.Value.(*parser.FunctionLiteral); ok {
		isValueFunc = true
		// --- Handle let f = function g() {} or let f = function() {} ---
		// 1. Define the *variable name (f)* temporarily for potential recursion
		//    within the function body (e.g., recursive anonymous function).
		// debug disabled
		c.currentSymbolTable.Define(node.Name.Value, nilRegister)

		// 2. Compile the function literal body.
		//    Pass the variable name (f) as the hint for the function object's name
		//    if the function literal itself is anonymous.
		funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, node.Name.Value)
		if err != nil {
			// Error already added to c.errors by compileFunctionLiteral
			return BadRegister, nil // Return nil error here, main error is tracked
		}
		// 3. Create the closure object
		closureReg := c.regAlloc.Alloc()
		defer c.regAlloc.Free(closureReg)
		// debug disabled
		c.emitClosure(closureReg, funcConstIndex, funcLit, freeSymbols)

		// 4. Update the symbol table entry for the *variable name (f)* with the closure register.
		// debug disabled
		c.currentSymbolTable.UpdateRegister(node.Name.Value, closureReg)

		// The variable's value (the closure) is now set.
		// We don't need to assign to valueReg anymore for this path.

	} else if node.Value != nil {
		// Compile other value types normally
		// Use existing predefined register if present
		targetReg := valueReg
		if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.Register != nilRegister {
			targetReg = sym.Register
		} else {
			targetReg = c.regAlloc.Alloc()
			defer c.regAlloc.Free(targetReg)
		}
		_, err = c.compileNode(node.Value, targetReg)
		if err != nil {
			return BadRegister, err
		}
		valueReg = targetReg
	} // else: node.Value is nil (implicit undefined handled below)

	// Handle implicit undefined (`let x;`)
	if valueReg == nilRegister && !isValueFunc {
		undefReg := c.regAlloc.Alloc()
		defer c.regAlloc.Free(undefReg)
		c.emitLoadUndefined(undefReg, node.Name.Token.Line)
		valueReg = undefReg
		// Define symbol for the `let x;` case
		// debug disabled
		if c.enclosing == nil {
			// Top-level: use global variable
			globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
			c.emitSetGlobal(globalIdx, valueReg, node.Name.Token.Line)
			c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
		} else {
			// Function scope: use local symbol table
			if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.Register != nilRegister {
				c.emitMove(sym.Register, valueReg, node.Name.Token.Line)
			} else {
				c.currentSymbolTable.Define(node.Name.Value, valueReg)
				c.regAlloc.Pin(valueReg)
			}
		}
	} else if !isValueFunc {
		// Define symbol ONLY for non-function values.
		// Function assignments were handled above by UpdateRegister.
		// debug disabled
		if c.enclosing == nil {
			// Top-level: use global variable
			globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
			c.emitSetGlobal(globalIdx, valueReg, node.Name.Token.Line)
			c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
		} else {
			// Function scope: use local symbol table
			if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.Register != nilRegister {
				c.emitMove(sym.Register, valueReg, node.Name.Token.Line)
			} else {
				c.currentSymbolTable.Define(node.Name.Value, valueReg)
				c.regAlloc.Pin(valueReg)
			}
		}
	} else if c.enclosing == nil {
		// Top-level function: also set as global
		globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
		// Get the closure register from the symbol table
		symbolRef, _, found := c.currentSymbolTable.Resolve(node.Name.Value)
		if found && symbolRef.Register != nilRegister {
			c.emitSetGlobal(globalIdx, symbolRef.Register, node.Name.Token.Line)
			// Update the symbol to be global
			c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
			// Pin the register since function closures can be captured by upvalues
			c.regAlloc.Pin(symbolRef.Register)
		}
	}

	return BadRegister, nil
}

func (c *Compiler) compileVarStatement(node *parser.VarStatement, hint Register) (Register, errors.PaseratiError) {
	// Process all variable declarations in the statement
	for _, declarator := range node.Declarations {
		// Hoist function declarations in var statements similar to Annex B in sloppy mode.
		// If the initializer is a FunctionLiteral with an Identifier name, predefine the name in current scope
		// so that subsequent references within the same block use the local binding rather than a global.
		if funcLit, ok := declarator.Value.(*parser.FunctionLiteral); ok {
			if funcLit.Name != nil {
				// Predefine to enable using it before assignment in the block
				c.currentSymbolTable.Define(declarator.Name.Value, nilRegister)
			}
		}
		// Set current declarator in legacy fields for backward compatibility
		node.Name = declarator.Name
		node.Value = declarator.Value
		node.ComputedType = declarator.ComputedType

		// debug disabled
		var valueReg Register = nilRegister
		var err errors.PaseratiError
		isValueFunc := false // Flag to track if value is a function literal

		if funcLit, ok := declarator.Value.(*parser.FunctionLiteral); ok {
			isValueFunc = true
			// --- Handle var f = function g() {} or var f = function() {} ---
			// 1. Define the *variable name (f)* temporarily for potential recursion
			//    within the function body (e.g., recursive anonymous function).
			// debug disabled
			c.currentSymbolTable.Define(node.Name.Value, nilRegister)

			// 2. Compile the function literal body.
			//    Pass the variable name (f) as the hint for the function object's name
			//    if the function literal itself is anonymous.
			funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, node.Name.Value)
			if err != nil {
				// Error already added to c.errors by compileFunctionLiteral
				return BadRegister, nil // Return nil error here, main error is tracked
			}
			// 3. Create the closure object
			closureReg := c.regAlloc.Alloc()
			defer c.regAlloc.Free(closureReg)
			// debug disabled
			c.emitClosure(closureReg, funcConstIndex, funcLit, freeSymbols)

			// 4. Update the symbol table entry for the *variable name (f)* with the closure register.
			// debug disabled
			c.currentSymbolTable.UpdateRegister(node.Name.Value, closureReg)

			// The variable's value (the closure) is now set.
			// We don't need to assign to valueReg anymore for this path.

		} else if node.Value != nil {
			// Compile other value types normally
			valueReg = c.regAlloc.Alloc()
			defer c.regAlloc.Free(valueReg)
			_, err = c.compileNode(node.Value, valueReg)
			if err != nil {
				return BadRegister, err
			}
		} // else: node.Value is nil (implicit undefined handled below)

		// Handle implicit undefined (`var x;`)
		if valueReg == nilRegister && !isValueFunc {
			undefReg := c.regAlloc.Alloc()
			defer c.regAlloc.Free(undefReg)
			c.emitLoadUndefined(undefReg, node.Name.Token.Line)
			valueReg = undefReg
			// Define symbol for the `var x;` case
			// debug disabled
			if c.enclosing == nil {
				// Top-level: use global variable
				globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
				c.emitSetGlobal(globalIdx, valueReg, node.Name.Token.Line)
				c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
			} else {
				// Function scope: use local symbol table
				c.currentSymbolTable.Define(node.Name.Value, valueReg)
				// Pin the register since local variables can be captured by upvalues
				c.regAlloc.Pin(valueReg)
			}
		} else if !isValueFunc {
			// Define symbol ONLY for non-function values.
			// Function assignments were handled above by UpdateRegister.
			// debug disabled
			if c.enclosing == nil {
				// Top-level: use global variable
				globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
				c.emitSetGlobal(globalIdx, valueReg, node.Name.Token.Line)
				c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
			} else {
				// Function scope: use local symbol table
				c.currentSymbolTable.Define(node.Name.Value, valueReg)
				// Pin the register since local variables can be captured by upvalues
				c.regAlloc.Pin(valueReg)
			}
		} else if c.enclosing == nil {
			// Top-level function: also set as global
			globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
			// Get the closure register from the symbol table
			symbolRef, _, found := c.currentSymbolTable.Resolve(node.Name.Value)
			if found && symbolRef.Register != nilRegister {
				c.emitSetGlobal(globalIdx, symbolRef.Register, node.Name.Token.Line)
				// Update the symbol to be global
				c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
				// Pin the register since function closures can be captured by upvalues
				c.regAlloc.Pin(symbolRef.Register)
			}
		}
	}

	return BadRegister, nil
}

func (c *Compiler) compileConstStatement(node *parser.ConstStatement, hint Register) (Register, errors.PaseratiError) {
	if node.Value == nil {
		// Parser should prevent this, but defensive check
		return BadRegister, NewCompileError(node.Name, "const declarations require an initializer")
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
		funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, node.Name.Value)
		if err != nil {
			// Error already added to c.errors by compileFunctionLiteral
			return BadRegister, nil // Return nil error here, main error is tracked
		}
		// 3. Create the closure object
		closureReg := c.regAlloc.Alloc()
		defer c.regAlloc.Free(closureReg)
		c.emitClosure(closureReg, funcConstIndex, funcLit, freeSymbols)

		// 4. Update the temporary definition for the *const name (f)* with the closure register.
		c.currentSymbolTable.UpdateRegister(node.Name.Value, closureReg)

		// The constant's value (the closure) is now set.
		// We don't need to assign to valueReg anymore for this path.

	} else {
		// Compile other value types normally
		// Use existing predefined register if present
		targetReg := valueReg
		if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.Register != nilRegister {
			targetReg = sym.Register
		} else {
			targetReg = c.regAlloc.Alloc()
			defer c.regAlloc.Free(targetReg)
		}
		_, err = c.compileNode(node.Value, targetReg)
		if err != nil {
			return BadRegister, err
		}
		valueReg = targetReg
	}

	// Define symbol ONLY for non-function values.
	// Const function assignments were handled above by UpdateRegister.
	if !isValueFunc {
		// For non-functions, Define associates the name with the final value register.
		if c.enclosing == nil {
			// Top-level: use global variable
			globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
			c.emitSetGlobal(globalIdx, valueReg, node.Name.Token.Line)
			c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
		} else {
			// Function scope: use local symbol table
			if sym, _, found := c.currentSymbolTable.Resolve(node.Name.Value); found && sym.Register != nilRegister {
				c.emitMove(sym.Register, valueReg, node.Name.Token.Line)
			} else {
				c.currentSymbolTable.Define(node.Name.Value, valueReg)
				c.regAlloc.Pin(valueReg)
			}
		}
	} else if c.enclosing == nil {
		// Top-level function: also set as global
		globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
		// Get the closure register from the symbol table
		symbolRef, _, found := c.currentSymbolTable.Resolve(node.Name.Value)
		if found && symbolRef.Register != nilRegister {
			c.emitSetGlobal(globalIdx, symbolRef.Register, node.Name.Token.Line)
			// Update the symbol to be global
			c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
			// Pin the register since function closures can be captured by upvalues
			c.regAlloc.Pin(symbolRef.Register)
		}
	}
	return BadRegister, nil
}

func (c *Compiler) compileReturnStatement(node *parser.ReturnStatement, hint Register) (Register, errors.PaseratiError) {
	if node.ReturnValue != nil {
		var err errors.PaseratiError
		var returnReg Register
		// Check if the return value is a function literal itself
		if funcLit, ok := node.ReturnValue.(*parser.FunctionLiteral); ok {
			// Compile directly, bypassing the compileNode case for declarations.
			// Pass empty hint as it's an anonymous function expression here.
			funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, "")
			if err != nil {
				// Error already added to c.errors by compileFunctionLiteral
				return BadRegister, nil // Return nil error here, main error is tracked
			}
			// Create the closure object
			closureReg := c.regAlloc.Alloc()
			defer c.regAlloc.Free(closureReg)
			c.emitClosure(closureReg, funcConstIndex, funcLit, freeSymbols)
			returnReg = closureReg

		} else {
			// Compile other expression types normally via compileNode
			returnReg = c.regAlloc.Alloc()
			defer c.regAlloc.Free(returnReg)
			_, err = c.compileNode(node.ReturnValue, returnReg)
			if err != nil {
				return BadRegister, err
			}
		}

		// Error check should cover both paths now
		if err != nil {
			// This check might be redundant if errors are handled correctly above,
			// but keep for safety unless proven otherwise.
			return BadRegister, err
		}
		// Emit return using the register holding the final value (closure or other expression result)
		// Choose opcode based on whether we're in a finally block or try-with-finally context
		if c.inFinallyBlock || c.tryFinallyDepth > 0 {
			c.emitReturnFinally(returnReg, node.Token.Line)
		} else {
			c.emitReturn(returnReg, node.Token.Line)
		}
		return returnReg, nil // Return the register containing the returned value
	} else {
		// Return undefined implicitly using the optimized opcode
		if c.inFinallyBlock || c.tryFinallyDepth > 0 {
			// For finally blocks or try-with-finally contexts, we need to use OpReturnFinally even for undefined returns
			// Allocate a temporary register for undefined
			undefinedReg := c.regAlloc.Alloc()
			defer c.regAlloc.Free(undefinedReg)
			c.emitOpCode(vm.OpLoadUndefined, node.Token.Line)
			c.emitByte(byte(undefinedReg))
			c.emitReturnFinally(undefinedReg, node.Token.Line)
		} else {
			c.emitOpCode(vm.OpReturnUndefined, node.Token.Line)
		}
		// For undefined returns, we could allocate a register with undefined, but for now return BadRegister
		return BadRegister, nil
	}
}

// --- Loop Compilation (Updated) ---

func (c *Compiler) compileWhileStatement(node *parser.WhileStatement, hint Register) (Register, errors.PaseratiError) {
	return c.compileWhileStatementLabeled(node, "", hint)
}

func (c *Compiler) compileWhileStatementLabeled(node *parser.WhileStatement, label string, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line

	// Defer-safety: patch any outstanding placeholder jump to a valid anchor on early returns
	var (
		jumpToEndPlaceholderPos = -1
		patchedCondition        = false
	)
	defer func() {
		if jumpToEndPlaceholderPos >= 0 && !patchedCondition {
			c.patchJump(jumpToEndPlaceholderPos)
			patchedCondition = true
		}
	}()

	// --- Setup Loop Context ---
	loopStartPos := len(c.chunk.Code) // Position before condition evaluation
	loopContext := &LoopContext{
		Label:                      label,
		LoopStartPos:               loopStartPos,
		ContinueTargetPos:          loopStartPos, // Continue goes back to condition in while
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
	}
	c.loopContextStack = append(c.loopContextStack, loopContext)

	// --- Compile Condition ---
	conditionReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(conditionReg)
	_, err := c.compileNode(node.Condition, conditionReg)
	if err != nil {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1] // Pop context on error
		return BadRegister, NewCompileError(node, "error compiling while condition").CausedBy(err)
	}

	// --- Jump Out If False ---
	jumpToEndPlaceholderPos = c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, line)

	// --- Compile Body ---
	bodyReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(bodyReg)
	_, err = c.compileNode(node.Body, bodyReg)
	if err != nil {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1] // Pop context on error
		return BadRegister, NewCompileError(node, "error compiling while body").CausedBy(err)
	}

	// --- Jump Back To Start ---
	jumpBackInstructionEndPos := len(c.chunk.Code) + 1 + 2 // OpCode + 16bit offset
	backOffset := loopStartPos - jumpBackInstructionEndPos
	c.emitOpCode(vm.OpJump, line)
	c.emitUint16(uint16(int16(backOffset))) // Emit calculated signed offset

	// --- Finish Loop ---
	// Patch the initial conditional jump to land here (after the backward jump)
	c.patchJump(jumpToEndPlaceholderPos)
	patchedCondition = true

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
			return BadRegister, NewCompileError(node, fmt.Sprintf("internal compiler error: continue jump offset %d exceeds 16-bit limit", targetOffset))
		}
		// Manually write the 16-bit offset into the placeholder jump instruction
		c.chunk.Code[continuePos+1] = byte(int16(targetOffset) >> 8)   // High byte
		c.chunk.Code[continuePos+2] = byte(int16(targetOffset) & 0xFF) // Low byte
	}

	return BadRegister, nil
}

func (c *Compiler) compileForStatement(node *parser.ForStatement, hint Register) (Register, errors.PaseratiError) {
	return c.compileForStatementLabeled(node, "", hint)
}

func (c *Compiler) compileForStatementLabeled(node *parser.ForStatement, label string, hint Register) (Register, errors.PaseratiError) {
	// Defer-safety: patch any outstanding placeholder jump to a valid anchor on early returns
	var (
		conditionExitJumpPlaceholderPos = -1
		patchedConditionExit            = false
	)
	defer func() {
		if conditionExitJumpPlaceholderPos != -1 && !patchedConditionExit {
			c.patchJump(conditionExitJumpPlaceholderPos)
			patchedConditionExit = true
		}
	}()
	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Initializer
	if node.Initializer != nil {
		// If initializer is a var-declaration, predefine its bindings so Resolve() works in condition/update.
		if vs, ok := node.Initializer.(*parser.VarStatement); ok {
			// Prefer explicit declarations if present; otherwise fall back to legacy Name field
			if len(vs.Declarations) > 0 {
				for _, d := range vs.Declarations {
					if c.enclosing == nil {
						// Top-level: predefine as global so identifier resolves as global in condition/update
						globalIdx := c.GetOrAssignGlobalIndex(d.Name.Value)
						c.currentSymbolTable.DefineGlobal(d.Name.Value, globalIdx)
					} else {
						// Function scope: predefine local binding; register will be set by compileVarStatement
						c.currentSymbolTable.Define(d.Name.Value, nilRegister)
					}
				}
			} else if vs.Name != nil {
				name := vs.Name.Value
				if c.enclosing == nil {
					globalIdx := c.GetOrAssignGlobalIndex(name)
					c.currentSymbolTable.DefineGlobal(name, globalIdx)
				} else {
					c.currentSymbolTable.Define(name, nilRegister)
				}
			}
		}
		initReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, initReg)
		if _, err := c.compileNode(node.Initializer, initReg); err != nil {
			return BadRegister, err
		}
	}

	// --- Loop Start & Context Setup ---
	loopStartPos := len(c.chunk.Code) // Position before condition check
	loopContext := &LoopContext{
		Label:                      label,
		LoopStartPos:               loopStartPos,
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
	}
	c.loopContextStack = append(c.loopContextStack, loopContext)
	// Scope for body/vars is handled by compileNode for the BlockStatement

	// --- 2. Condition (Optional) ---
	if node.Condition != nil {
		conditionReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, conditionReg)
		_, err := c.compileNode(node.Condition, conditionReg)
		if err != nil {
			// Clean up loop context if condition compilation fails
			c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
			return BadRegister, err
		}
		conditionExitJumpPlaceholderPos = c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)
	} // If no condition, it's an infinite loop (handled by break/return)

	// --- 3. Body ---
	// Continue placeholders will be added to loopContext here
	bodyReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, bodyReg)
	if _, err := c.compileNode(node.Body, bodyReg); err != nil {
		// Clean up loop context if body compilation fails
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, err
	}

	// --- 4. Patch Continues & Compile Update ---

	// *** Patch Continue Jumps ***
	// Patch continue jumps to land here, *before* the update expression
	for _, continuePos := range loopContext.ContinuePlaceholderPosList { // Use context on stack
		c.patchJump(continuePos) // Patch placeholder to jump to current position
	}

	// *** Compile Update Expression (Optional) ***
	if node.Update != nil {
		updateReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, updateReg)
		if _, err := c.compileNode(node.Update, updateReg); err != nil {
			// Clean up loop context if update compilation fails
			c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
			return BadRegister, err
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

	// Pop loop context
	poppedContext := c.loopContextStack[len(c.loopContextStack)-1]
	c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]

	// Patch the condition exit jump (if there was a condition)
	// This needs to happen *at* the final position
	if conditionExitJumpPlaceholderPos != -1 {
		c.patchJump(conditionExitJumpPlaceholderPos) // Patch to jump to current position
		patchedConditionExit = true
	}

	// Patch all break jumps
	// This needs to happen *at* the final position
	for _, breakPos := range poppedContext.BreakPlaceholderPosList {
		c.patchJump(breakPos) // Patch to jump to current position
	}

	return BadRegister, nil
}

// --- New: Break/Continue Compilation ---

func (c *Compiler) compileBreakStatement(node *parser.BreakStatement, hint Register) (Register, errors.PaseratiError) {
	if len(c.loopContextStack) == 0 {
		return BadRegister, NewCompileError(node, "break statement not within a loop")
	}

	var targetContext *LoopContext

	if node.Label != nil {
		// Find the labeled context
		found := false
		for i := len(c.loopContextStack) - 1; i >= 0; i-- {
			if c.loopContextStack[i].Label == node.Label.Value {
				targetContext = c.loopContextStack[i]
				found = true
				break
			}
		}
		if !found {
			return BadRegister, NewCompileError(node, fmt.Sprintf("label '%s' not found", node.Label.Value))
		}
	} else {
		// Get current loop context (top of stack)
		targetContext = c.loopContextStack[len(c.loopContextStack)-1]
	}

	// Check if we need to emit iterator cleanup code before breaking
	if targetContext.IteratorCleanup != nil && targetContext.IteratorCleanup.UsesIteratorProtocol {
		c.emitIteratorCleanup(targetContext.IteratorCleanup.IteratorReg, node.Token.Line)
	}

	// Emit placeholder jump (OpJump) - Pass 0 for srcReg as it's ignored
	placeholderPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)

	// Add placeholder position to the target context's list for later patching
	targetContext.BreakPlaceholderPosList = append(targetContext.BreakPlaceholderPosList, placeholderPos)

	return BadRegister, nil
}

func (c *Compiler) compileEmptyStatement(node *parser.EmptyStatement, hint Register) (Register, errors.PaseratiError) {
	// Empty statements are no-ops - they generate no bytecode
	// This is perfectly valid: if (condition) ; else doSomething();
	return hint, nil
}

func (c *Compiler) compileContinueStatement(node *parser.ContinueStatement, hint Register) (Register, errors.PaseratiError) {
	if len(c.loopContextStack) == 0 {
		return BadRegister, NewCompileError(node, "continue statement not within a loop")
	}

	var targetContext *LoopContext

	if node.Label != nil {
		// Find the labeled context
		found := false
		for i := len(c.loopContextStack) - 1; i >= 0; i-- {
			if c.loopContextStack[i].Label == node.Label.Value {
				targetContext = c.loopContextStack[i]
				found = true
				break
			}
		}
		if !found {
			return BadRegister, NewCompileError(node, fmt.Sprintf("label '%s' not found", node.Label.Value))
		}

		// Check that the target is actually a loop (continue only works with loops)
		if targetContext.ContinueTargetPos == -1 {
			return BadRegister, NewCompileError(node, fmt.Sprintf("continue statement cannot target non-loop label '%s'", node.Label.Value))
		}
	} else {
		// Get current loop context (top of stack)
		targetContext = c.loopContextStack[len(c.loopContextStack)-1]
	}

	// Emit placeholder jump (OpJump) - Pass 0 for srcReg as it's ignored
	placeholderPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)

	// Add placeholder position to the target context's list for later patching
	targetContext.ContinuePlaceholderPosList = append(targetContext.ContinuePlaceholderPosList, placeholderPos)

	return BadRegister, nil
}

// --- New: Labeled Statement Compilation ---

func (c *Compiler) compileLabeledStatement(node *parser.LabeledStatement, hint Register) (Register, errors.PaseratiError) {
	// Compile the labeled statement
	// If it's a loop-like statement (while, for, do-while), we need to set the label in the LoopContext

	// For loop statements, we need to temporarily add a label context
	// and then call the normal compilation functions
	switch stmt := node.Statement.(type) {
	case *parser.WhileStatement:
		return c.compileWhileStatementLabeled(stmt, node.Label.Value, hint)
	case *parser.ForStatement:
		return c.compileForStatementLabeled(stmt, node.Label.Value, hint)
	case *parser.DoWhileStatement:
		return c.compileDoWhileStatementLabeled(stmt, node.Label.Value, hint)
	case *parser.ForOfStatement:
		return c.compileForOfStatementLabeled(stmt, node.Label.Value, hint)
	case *parser.ForInStatement:
		return c.compileForInStatementLabeled(stmt, node.Label.Value, hint)
	default:
		// For non-loop statements, we need to create a label context anyway
		// in case there are break statements that refer to this label
		labelContext := &LoopContext{
			Label:                      node.Label.Value,
			LoopStartPos:               -1, // Not applicable for non-loops
			ContinueTargetPos:          -1, // Not applicable for non-loops
			BreakPlaceholderPosList:    make([]int, 0),
			ContinuePlaceholderPosList: make([]int, 0),
		}
		c.loopContextStack = append(c.loopContextStack, labelContext)

		// Compile the statement
		result, err := c.compileNode(node.Statement, hint)

		// Pop the label context and patch any break statements
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		for _, placeholderPos := range labelContext.BreakPlaceholderPosList {
			c.patchJump(placeholderPos)
		}

		return result, err
	}
}

func (c *Compiler) compileDoWhileStatement(node *parser.DoWhileStatement, hint Register) (Register, errors.PaseratiError) {
	return c.compileDoWhileStatementLabeled(node, "", hint)
}

func (c *Compiler) compileDoWhileStatementLabeled(node *parser.DoWhileStatement, label string, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line

	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Mark Loop Start (before body)
	loopStartPos := len(c.chunk.Code)

	// 2. Setup Loop Context
	loopContext := &LoopContext{
		Label:                      label,
		LoopStartPos:               loopStartPos, // Continue jumps here
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
	}
	c.loopContextStack = append(c.loopContextStack, loopContext)

	// 3. Compile Body (executes at least once)
	bodyReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, bodyReg)
	if _, err := c.compileNode(node.Body, bodyReg); err != nil {
		// Pop context if body compilation fails
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, NewCompileError(node, "error compiling do-while body").CausedBy(err)
	}

	// 4. Mark Condition Position (for clarity, not used directly in jump calcs below)
	_ = len(c.chunk.Code) // conditionPos := len(c.chunk.Code)

	// 5. Compile Condition
	conditionReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, conditionReg)
	_, err := c.compileNode(node.Condition, conditionReg)
	if err != nil {
		// Pop context if condition compilation fails
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, NewCompileError(node, "error compiling do-while condition").CausedBy(err)
	}

	// 6. Conditional Jump back to Loop Start
	// We need OpJumpIfTrue, but we only have OpJumpIfFalse.
	// So, we invert the condition and use OpJumpIfFalse.
	invertedConditionReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, invertedConditionReg)
	c.emitNot(invertedConditionReg, conditionReg, line)

	// Now jump back if the *inverted* condition is FALSE (i.e., original was TRUE)
	jumpBackInstructionEndPos := len(c.chunk.Code) + 1 + 2 + 1 // OpCode + Reg + 16bit offset
	backOffset := loopStartPos - jumpBackInstructionEndPos
	if backOffset > math.MaxInt16 || backOffset < math.MinInt16 {
		return BadRegister, NewCompileError(node, fmt.Sprintf("internal compiler error: do-while loop jump offset %d exceeds 16-bit limit", backOffset))
	}
	c.emitOpCode(vm.OpJumpIfFalse, line)    // Use OpJumpIfFalse on inverted result
	c.emitByte(byte(invertedConditionReg))  // Jump based on the inverted condition
	c.emitUint16(uint16(int16(backOffset))) // Emit calculated signed offset

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
			return BadRegister, NewCompileError(node, fmt.Sprintf("internal compiler error: do-while continue jump offset %d exceeds 16-bit limit", targetOffset))
		}
		// Manually write the 16-bit offset into the placeholder OpJump instruction
		c.chunk.Code[continuePos+1] = byte(int16(targetOffset) >> 8)   // High byte
		c.chunk.Code[continuePos+2] = byte(int16(targetOffset) & 0xFF) // Low byte
	}

	return BadRegister, nil
}

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
func (c *Compiler) compileSwitchStatement(node *parser.SwitchStatement, hint Register) (Register, errors.PaseratiError) {
	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Compile the expression being switched on
	switchExprReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, switchExprReg)
	_, err := c.compileNode(node.Expression, switchExprReg)
	if err != nil {
		return BadRegister, err
	}
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
				return BadRegister, nil // Indicate error occurred
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
			caseCondReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, caseCondReg)
			_, err := c.compileNode(caseClause.Condition, caseCondReg)
			if err != nil {
				return BadRegister, err
			}

			// Compare switch expression value with case condition value
			matchReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, matchReg)
			c.emitStrictEqual(matchReg, switchExprReg, caseCondReg, caseLine)

			// If no match, jump to the next case test (or default/end)
			jumpPos := c.emitPlaceholderJump(vm.OpJumpIfFalse, matchReg, caseLine)
			caseTestFailJumps = append(caseTestFailJumps, jumpPos)

			// Record the start position of the body for potential jumps
			caseBodyStartPositions[i] = c.currentPosition()

			// Compile the case body
			caseBodyReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, caseBodyReg)
			_, err = c.compileNode(caseClause.Body, caseBodyReg)
			if err != nil {
				return BadRegister, err
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
			defaultBodyReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, defaultBodyReg)
			_, err = c.compileNode(caseClause.Body, defaultBodyReg)
			if err != nil {
				return BadRegister, err
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

	return BadRegister, nil
}

func (c *Compiler) compileForOfStatement(node *parser.ForOfStatement, hint Register) (Register, errors.PaseratiError) {
	return c.compileForOfStatementLabeled(node, "", hint)
}

func (c *Compiler) compileForOfStatementLabeled(node *parser.ForOfStatement, label string, hint Register) (Register, errors.PaseratiError) {
	// Defer-safety: ensure jumps get patched even if we return early
	var (
		iteratorPathJump        = -1
		conditionExitJumpPos    = -1
		iteratorExitJump        = -1
		skipIteratorPathJump    = -1
		patchedIteratorPath     = false
		patchedConditionExit    = false
		patchedIteratorExit     = false
		patchedSkipIteratorPath = false
	)
	defer func() {
		if iteratorPathJump >= 0 && !patchedIteratorPath {
			c.patchJump(iteratorPathJump)
			patchedIteratorPath = true
		}
		if conditionExitJumpPos >= 0 && !patchedConditionExit {
			c.patchJump(conditionExitJumpPos)
			patchedConditionExit = true
		}
		if iteratorExitJump >= 0 && !patchedIteratorExit {
			c.patchJump(iteratorExitJump)
			patchedIteratorExit = true
		}
		if skipIteratorPathJump >= 0 && !patchedSkipIteratorPath {
			c.patchJump(skipIteratorPathJump)
			patchedSkipIteratorPath = true
		}
	}()
	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Compile the iterable expression first
	iterableReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iterableReg)
	_, err := c.compileNode(node.Iterable, iterableReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. Runtime dispatch: check type and choose iteration strategy
	// We'll generate code that checks the object type and branches to the appropriate path

	// Check if iterable is an array (fast path)
	typeCheckReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, typeCheckReg)
	c.emitOpCode(vm.OpTypeof, node.Token.Line)
	c.emitByte(byte(typeCheckReg))
	c.emitByte(byte(iterableReg))

	// Compare with "array" type
	arrayTypeReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, arrayTypeReg)
	c.emitLoadNewConstant(arrayTypeReg, vm.String("array"), node.Token.Line)

	isArrayReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, isArrayReg)
	c.emitOpCode(vm.OpStrictEqual, node.Token.Line)
	c.emitByte(byte(isArrayReg))
	c.emitByte(byte(typeCheckReg))
	c.emitByte(byte(arrayTypeReg))

	// Jump to iterator protocol if not array
	iteratorPathJump = c.emitPlaceholderJump(vm.OpJumpIfFalse, isArrayReg, node.Token.Line)

	// === FAST PATH: Array iteration ===
	indexReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, indexReg)
	lengthReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, lengthReg)
	elementReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, elementReg)

	// Initialize index to 0
	c.emitLoadNewConstant(indexReg, vm.Number(0), node.Token.Line)

	// Get length of array
	c.emitOpCode(vm.OpGetProp, node.Token.Line)
	c.emitByte(byte(lengthReg))   // destination register
	c.emitByte(byte(iterableReg)) // object register
	lengthConstIdx := c.chunk.AddConstant(vm.String("length"))
	c.emitUint16(lengthConstIdx) // property name constant index

	// --- Loop Start & Context Setup ---
	loopStartPos := len(c.chunk.Code)
	loopContext := &LoopContext{
		Label:                      label,
		LoopStartPos:               loopStartPos,
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
		IteratorCleanup:            nil, // No cleanup needed for array fast path
	}
	c.loopContextStack = append(c.loopContextStack, loopContext)

	// 3. Check if index < length (loop condition)
	conditionReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, conditionReg)
	c.emitOpCode(vm.OpLess, node.Token.Line)
	c.emitByte(byte(conditionReg)) // destination
	c.emitByte(byte(indexReg))     // left operand (index)
	c.emitByte(byte(lengthReg))    // right operand (length)

	// Jump out of loop if condition is false
	conditionExitJumpPos = c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)

	// 4. Get current element: iterable[index]
	c.emitOpCode(vm.OpGetIndex, node.Token.Line)
	c.emitByte(byte(elementReg))  // destination
	c.emitByte(byte(iterableReg)) // array
	c.emitByte(byte(indexReg))    // index

	// 5. Assign element to loop variable
	if letStmt, ok := node.Variable.(*parser.LetStatement); ok {
		// Define the loop variable in symbol table
		symbol := c.currentSymbolTable.Define(letStmt.Name.Value, c.regAlloc.Alloc())
		// Pin the register since loop variables can be captured by closures in the loop body
		c.regAlloc.Pin(symbol.Register)
		// Store element value in the variable's register
		c.emitMove(symbol.Register, elementReg, node.Token.Line)
	} else if constStmt, ok := node.Variable.(*parser.ConstStatement); ok {
		// Define the loop variable in symbol table
		symbol := c.currentSymbolTable.Define(constStmt.Name.Value, c.regAlloc.Alloc())
		// Pin the register since loop variables can be captured by closures in the loop body
		c.regAlloc.Pin(symbol.Register)
		// Store element value in the variable's register
		c.emitMove(symbol.Register, elementReg, node.Token.Line)
	} else if arrayDestr, ok := node.Variable.(*parser.ArrayDestructuringDeclaration); ok {
		// Array destructuring: for(const [x, y] of arr)
		// Destructure elementReg into loop-scoped local variables
		for i, element := range arrayDestr.Elements {
			if element.Target == nil {
				continue
			}

			// Extract element[i] from elementReg
			indexReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, indexReg)
			extractedReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, extractedReg)

			c.emitLoadNewConstant(indexReg, vm.Number(float64(i)), node.Token.Line)
			c.emitOpCode(vm.OpGetIndex, node.Token.Line)
			c.emitByte(byte(extractedReg))
			c.emitByte(byte(elementReg))
			c.emitByte(byte(indexReg))

			// Define the variable as a local (like regular let/const in for-of)
			if ident, ok := element.Target.(*parser.Identifier); ok {
				symbol := c.currentSymbolTable.Define(ident.Value, c.regAlloc.Alloc())
				c.regAlloc.Pin(symbol.Register)
				c.emitMove(symbol.Register, extractedReg, node.Token.Line)
			}
		}
	} else if objDestr, ok := node.Variable.(*parser.ObjectDestructuringDeclaration); ok {
		// Object destructuring: for(const {x, y} of arr)
		// Destructure elementReg into loop-scoped local variables
		for _, prop := range objDestr.Properties {
			if prop.Target == nil {
				continue
			}

			// Extract property from elementReg
			extractedReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, extractedReg)

			propName := prop.Key.Value
			propConstIdx := c.chunk.AddConstant(vm.String(propName))
			c.emitGetProp(extractedReg, elementReg, propConstIdx, node.Token.Line)

			// Define the variable as a local
			if ident, ok := prop.Target.(*parser.Identifier); ok {
				symbol := c.currentSymbolTable.Define(ident.Value, c.regAlloc.Alloc())
				c.regAlloc.Pin(symbol.Register)
				c.emitMove(symbol.Register, extractedReg, node.Token.Line)
			}
		}
	} else if exprStmt, ok := node.Variable.(*parser.ExpressionStatement); ok {
		// This is an existing variable being assigned to
		if ident, ok := exprStmt.Expression.(*parser.Identifier); ok {
			symbolRef, definingTable, found := c.currentSymbolTable.Resolve(ident.Value)
			if !found {
				// Define a function/global-scoped binding (var semantics)
				target := c.currentSymbolTable
				for target.Outer != nil {
					target = target.Outer
				}
				reg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, reg)
				sym := target.Define(ident.Value, reg)
				c.regAlloc.Pin(sym.Register)
				c.emitMove(sym.Register, elementReg, node.Token.Line)
			} else {
				// Check if this is a global variable or local register
				if symbolRef.IsGlobal {
					// Store element value using OpSetGlobal for global variables
					c.emitSetGlobal(symbolRef.GlobalIndex, elementReg, node.Token.Line)
				} else {
					// Store element value in the existing variable's register for local variables
					_ = definingTable
					c.emitMove(symbolRef.Register, elementReg, node.Token.Line)
				}
			}
		}
	}

	// 6. Compile loop body
	bodyReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, bodyReg)
	if _, err := c.compileNode(node.Body, bodyReg); err != nil {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, err
	}

	// 7. Patch continue jumps to land here (before increment)
	for _, continuePos := range loopContext.ContinuePlaceholderPosList {
		c.patchJump(continuePos)
	}

	// 8. Increment index
	oneReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, oneReg)
	c.emitLoadNewConstant(oneReg, vm.Number(1), node.Token.Line)
	c.emitOpCode(vm.OpAdd, node.Token.Line)
	c.emitByte(byte(indexReg)) // destination (reuse indexReg)
	c.emitByte(byte(indexReg)) // left operand (current index)
	c.emitByte(byte(oneReg))   // right operand (1)

	// 9. Jump back to loop start
	jumpBackInstructionEndPos := len(c.chunk.Code) + 1 + 2
	backOffset := loopStartPos - jumpBackInstructionEndPos
	c.emitOpCode(vm.OpJump, node.Body.Token.Line)
	c.emitUint16(uint16(int16(backOffset)))

	// Jump to end after array iteration is complete
	skipIteratorPathJump = c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)

	// === ITERATOR PROTOCOL PATH: For generators, user-defined iterables ===
	c.patchJump(iteratorPathJump) // Patch to land here

	// Get Symbol.iterator via computed index to preserve the singleton identity
	iteratorMethodReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iteratorMethodReg)
	// Load global Symbol via unified global index
	symbolObjReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, symbolObjReg)
	symIdx := c.GetOrAssignGlobalIndex("Symbol")
	c.emitGetGlobal(symbolObjReg, symIdx, node.Token.Line)
	// Prepare "iterator" name BEFORE get
	propNameReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, propNameReg)
	c.emitLoadNewConstant(propNameReg, vm.String("iterator"), node.Token.Line)
	// iteratorKey = Symbol["iterator"]
	iteratorKeyReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iteratorKeyReg)
	c.emitOpCode(vm.OpGetIndex, node.Token.Line)
	c.emitByte(byte(iteratorKeyReg)) // Dest
	c.emitByte(byte(symbolObjReg))   // Base
	c.emitByte(byte(propNameReg))    // Key
	// method = iterable[iteratorKey]
	c.emitOpCode(vm.OpGetIndex, node.Token.Line)
	c.emitByte(byte(iteratorMethodReg)) // Dest
	c.emitByte(byte(iterableReg))       // Base
	c.emitByte(byte(iteratorKeyReg))    // Key

	// Call the iterator method to get iterator (use method call to preserve 'this' binding)
	iteratorObjReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iteratorObjReg)
	c.emitCallMethod(iteratorObjReg, iteratorMethodReg, iterableReg, 0, node.Token.Line)

	// Iterator loop setup - reuse loop context but update positions and set iterator cleanup
	iteratorLoopStart := len(c.chunk.Code)
	loopContext.LoopStartPos = iteratorLoopStart
	loopContext.IteratorCleanup = &IteratorCleanupInfo{
		IteratorReg:          iteratorObjReg,
		UsesIteratorProtocol: true,
	}

	// Call iterator.next()
	nextMethodReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, nextMethodReg)
	nextConstIdx := c.chunk.AddConstant(vm.String("next"))
	c.emitGetProp(nextMethodReg, iteratorObjReg, nextConstIdx, node.Token.Line)

	// Call next() to get {value, done} (use method call to preserve 'this' binding)
	resultReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, resultReg)
	c.emitCallMethod(resultReg, nextMethodReg, iteratorObjReg, 0, node.Token.Line)

	// Get result.done
	doneReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, doneReg)
	doneConstIdx := c.chunk.AddConstant(vm.String("done"))
	c.emitGetProp(doneReg, resultReg, doneConstIdx, node.Token.Line)

	// Exit loop if done is true (using same logic as yield*)
	notDoneReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, notDoneReg)
	c.emitOpCode(vm.OpNot, node.Token.Line)
	c.emitByte(byte(notDoneReg))
	c.emitByte(byte(doneReg))

	iteratorExitJump = c.emitPlaceholderJump(vm.OpJumpIfFalse, notDoneReg, node.Token.Line)

	// Get result.value for loop variable
	valueReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, valueReg)
	valueConstIdx := c.chunk.AddConstant(vm.String("value"))
	c.emitGetProp(valueReg, resultReg, valueConstIdx, node.Token.Line)

	// Assign value to loop variable (reuse the assignment logic from array path)
	if letStmt, ok := node.Variable.(*parser.LetStatement); ok {
		symbol := c.currentSymbolTable.Define(letStmt.Name.Value, c.regAlloc.Alloc())
		c.regAlloc.Pin(symbol.Register)
		c.emitMove(symbol.Register, valueReg, node.Token.Line)
	} else if constStmt, ok := node.Variable.(*parser.ConstStatement); ok {
		symbol := c.currentSymbolTable.Define(constStmt.Name.Value, c.regAlloc.Alloc())
		c.regAlloc.Pin(symbol.Register)
		c.emitMove(symbol.Register, valueReg, node.Token.Line)
	} else if arrayDestr, ok := node.Variable.(*parser.ArrayDestructuringDeclaration); ok {
		// Array destructuring in iterator path
		for i, element := range arrayDestr.Elements {
			if element.Target == nil {
				continue
			}

			indexReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, indexReg)
			extractedReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, extractedReg)

			c.emitLoadNewConstant(indexReg, vm.Number(float64(i)), node.Token.Line)
			c.emitOpCode(vm.OpGetIndex, node.Token.Line)
			c.emitByte(byte(extractedReg))
			c.emitByte(byte(valueReg))
			c.emitByte(byte(indexReg))

			if ident, ok := element.Target.(*parser.Identifier); ok {
				symbol := c.currentSymbolTable.Define(ident.Value, c.regAlloc.Alloc())
				c.regAlloc.Pin(symbol.Register)
				c.emitMove(symbol.Register, extractedReg, node.Token.Line)
			}
		}
	} else if objDestr, ok := node.Variable.(*parser.ObjectDestructuringDeclaration); ok {
		// Object destructuring in iterator path
		for _, prop := range objDestr.Properties {
			if prop.Target == nil {
				continue
			}

			extractedReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, extractedReg)

			propName := prop.Key.Value
			propConstIdx := c.chunk.AddConstant(vm.String(propName))
			c.emitGetProp(extractedReg, valueReg, propConstIdx, node.Token.Line)

			if ident, ok := prop.Target.(*parser.Identifier); ok {
				symbol := c.currentSymbolTable.Define(ident.Value, c.regAlloc.Alloc())
				c.regAlloc.Pin(symbol.Register)
				c.emitMove(symbol.Register, extractedReg, node.Token.Line)
			}
		}
	} else if exprStmt, ok := node.Variable.(*parser.ExpressionStatement); ok {
		if ident, ok := exprStmt.Expression.(*parser.Identifier); ok {
			symbolRef, _, found := c.currentSymbolTable.Resolve(ident.Value)
			if !found {
				return BadRegister, NewCompileError(ident, fmt.Sprintf("undefined variable '%s'", ident.Value))
			}
			if symbolRef.IsGlobal {
				c.emitSetGlobal(symbolRef.GlobalIndex, valueReg, node.Token.Line)
			} else {
				c.emitMove(symbolRef.Register, valueReg, node.Token.Line)
			}
		}
	}

	// Compile loop body for iterator path
	iteratorBodyReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iteratorBodyReg)
	if _, err := c.compileNode(node.Body, iteratorBodyReg); err != nil {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, err
	}

	// Patch continue jumps for iterator loop
	for _, continuePos := range loopContext.ContinuePlaceholderPosList {
		c.patchJump(continuePos)
	}

	// Jump back to iterator loop start
	iteratorJumpBackPos := len(c.chunk.Code) + 1 + 2
	iteratorBackOffset := iteratorLoopStart - iteratorJumpBackPos
	c.emitOpCode(vm.OpJump, node.Body.Token.Line)
	c.emitUint16(uint16(int16(iteratorBackOffset)))

	// Patch iterator exit jump
	c.patchJump(iteratorExitJump)
	patchedIteratorExit = true

	// Patch skip iterator path jump (from array completion)
	c.patchJump(skipIteratorPathJump)
	patchedSkipIteratorPath = true

	// 10. Clean up loop context and patch jumps
	poppedContext := c.loopContextStack[len(c.loopContextStack)-1]
	c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]

	// Patch condition exit jump (from array path)
	c.patchJump(conditionExitJumpPos)
	patchedConditionExit = true

	// Patch all break jumps
	for _, breakPos := range poppedContext.BreakPlaceholderPosList {
		c.patchJump(breakPos)
	}

	return BadRegister, nil
}

func (c *Compiler) compileForInStatement(node *parser.ForInStatement, hint Register) (Register, errors.PaseratiError) {
	return c.compileForInStatementLabeled(node, "", hint)
}

func (c *Compiler) compileForInStatementLabeled(node *parser.ForInStatement, label string, hint Register) (Register, errors.PaseratiError) {
	// Defer-safety: ensure condition exit is patched
	var (
		conditionExitJumpPos = -1
		patchedConditionExit = false
	)
	defer func() {
		if conditionExitJumpPos >= 0 && !patchedConditionExit {
			c.patchJump(conditionExitJumpPos)
			patchedConditionExit = true
		}
	}()
	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// Predefine var header bindings (top-level as global, function scope as local) so body Resolve works
	if vs, ok := node.Variable.(*parser.VarStatement); ok {
		if len(vs.Declarations) > 0 {
			for _, d := range vs.Declarations {
				if c.enclosing == nil {
					idx := c.GetOrAssignGlobalIndex(d.Name.Value)
					c.currentSymbolTable.DefineGlobal(d.Name.Value, idx)
					fmt.Printf("// [ForInPredefine] var %s as global idx=%d\n", d.Name.Value, idx)
				} else {
					c.currentSymbolTable.Define(d.Name.Value, nilRegister)
					fmt.Printf("// [ForInPredefine] var %s as local (nil reg)\n", d.Name.Value)
				}
			}
		} else if vs.Name != nil {
			name := vs.Name.Value
			if c.enclosing == nil {
				idx := c.GetOrAssignGlobalIndex(name)
				c.currentSymbolTable.DefineGlobal(name, idx)
				fmt.Printf("// [ForInPredefine] var %s as global idx=%d\n", name, idx)
			} else {
				c.currentSymbolTable.Define(name, nilRegister)
				fmt.Printf("// [ForInPredefine] var %s as local (nil reg)\n", name)
			}
		} else {
			fmt.Printf("// [ForInPredefine] VarStatement without Name/Declarations\n")
		}
	} else {
		switch node.Variable.(type) {
		case *parser.LetStatement:
			fmt.Printf("// [ForInHeader] let\n")
		case *parser.ConstStatement:
			fmt.Printf("// [ForInHeader] const\n")
		case *parser.ExpressionStatement:
			fmt.Printf("// [ForInHeader] bare identifier\n")
		default:
			fmt.Printf("// [ForInHeader] other type %T\n", node.Variable)
		}
	}

	// 1. Compile the object expression first
	objectReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, objectReg)
	_, err := c.compileNode(node.Object, objectReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. Get object keys using OpGetOwnKeys
	keysReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, keysReg)
	c.emitOpCode(vm.OpGetOwnKeys, node.Token.Line)
	c.emitByte(byte(keysReg))   // destination register
	c.emitByte(byte(objectReg)) // object register

	// 3. Set up iteration variables (mirroring for...of pattern)
	// We need a key index counter and keys array length
	keyIndexReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, keyIndexReg)
	lengthReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, lengthReg)
	currentKeyReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, currentKeyReg)

	// Initialize key index to 0
	c.emitLoadNewConstant(keyIndexReg, vm.Number(0), node.Token.Line)

	// Get length of keys array
	c.emitOpCode(vm.OpGetProp, node.Token.Line)
	c.emitByte(byte(lengthReg)) // destination register
	c.emitByte(byte(keysReg))   // keys array register
	lengthConstIdx := c.chunk.AddConstant(vm.String("length"))
	c.emitUint16(lengthConstIdx) // property name constant index

	// --- Loop Start & Context Setup ---
	loopStartPos := len(c.chunk.Code)
	loopContext := &LoopContext{
		Label:                      label,
		LoopStartPos:               loopStartPos,
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
	}
	c.loopContextStack = append(c.loopContextStack, loopContext)

	// 4. Check if keyIndex < length (loop condition)
	conditionReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, conditionReg)
	c.emitOpCode(vm.OpLess, node.Token.Line)
	c.emitByte(byte(conditionReg)) // destination
	c.emitByte(byte(keyIndexReg))  // left operand (key index)
	c.emitByte(byte(lengthReg))    // right operand (length)

	// Jump out of loop if condition is false
	conditionExitJumpPos = c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)

	// 5. Get current key: keys[keyIndex]
	c.emitOpCode(vm.OpGetIndex, node.Token.Line)
	c.emitByte(byte(currentKeyReg)) // destination
	c.emitByte(byte(keysReg))       // keys array
	c.emitByte(byte(keyIndexReg))   // index

	// 6. Assign current key to loop variable
	if letStmt, ok := node.Variable.(*parser.LetStatement); ok {
		// Define the loop variable in symbol table
		symbol := c.currentSymbolTable.Define(letStmt.Name.Value, c.regAlloc.Alloc())
		// Pin the register since loop variables can be captured by closures in the loop body
		c.regAlloc.Pin(symbol.Register)
		// Store key value in the variable's register
		c.emitMove(symbol.Register, currentKeyReg, node.Token.Line)
	} else if constStmt, ok := node.Variable.(*parser.ConstStatement); ok {
		// Define the loop variable in symbol table
		symbol := c.currentSymbolTable.Define(constStmt.Name.Value, c.regAlloc.Alloc())
		// Pin the register since loop variables can be captured by closures in the loop body
		c.regAlloc.Pin(symbol.Register)
		// Store key value in the variable's register
		c.emitMove(symbol.Register, currentKeyReg, node.Token.Line)
	} else if arrayDestr, ok := node.Variable.(*parser.ArrayDestructuringDeclaration); ok {
		// Array destructuring in for-in loop
		for i, element := range arrayDestr.Elements {
			if element.Target == nil {
				continue
			}

			indexReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, indexReg)
			extractedReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, extractedReg)

			c.emitLoadNewConstant(indexReg, vm.Number(float64(i)), node.Token.Line)
			c.emitOpCode(vm.OpGetIndex, node.Token.Line)
			c.emitByte(byte(extractedReg))
			c.emitByte(byte(currentKeyReg))
			c.emitByte(byte(indexReg))

			if ident, ok := element.Target.(*parser.Identifier); ok {
				symbol := c.currentSymbolTable.Define(ident.Value, c.regAlloc.Alloc())
				c.regAlloc.Pin(symbol.Register)
				c.emitMove(symbol.Register, extractedReg, node.Token.Line)
			}
		}
	} else if objDestr, ok := node.Variable.(*parser.ObjectDestructuringDeclaration); ok {
		// Object destructuring in for-in loop
		for _, prop := range objDestr.Properties {
			if prop.Target == nil {
				continue
			}

			extractedReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, extractedReg)

			propName := prop.Key.Value
			propConstIdx := c.chunk.AddConstant(vm.String(propName))
			c.emitGetProp(extractedReg, currentKeyReg, propConstIdx, node.Token.Line)

			if ident, ok := prop.Target.(*parser.Identifier); ok {
				symbol := c.currentSymbolTable.Define(ident.Value, c.regAlloc.Alloc())
				c.regAlloc.Pin(symbol.Register)
				c.emitMove(symbol.Register, extractedReg, node.Token.Line)
			}
		}
	} else if varStmt, ok := node.Variable.(*parser.VarStatement); ok {
		// Handle 'var k in obj' - assign into the pre-defined binding (global at top-level, local in function)
		if varStmt.Name != nil {
			name := varStmt.Name.Value
			fmt.Printf("// [ForInAssign] assigning to var %s\n", name)
			// Resolve the pre-defined binding from the header
			symbolRef, _, found := c.currentSymbolTable.Resolve(name)
			if !found {
				// As a fallback, predefine now mirroring regular for behavior
				if c.enclosing == nil {
					idx := c.GetOrAssignGlobalIndex(name)
					c.currentSymbolTable.DefineGlobal(name, idx)
					symbolRef, _, found = c.currentSymbolTable.Resolve(name)
				} else {
					c.currentSymbolTable.Define(name, nilRegister)
					symbolRef, _, found = c.currentSymbolTable.Resolve(name)
				}
			}
			if !found {
				return BadRegister, NewCompileError(varStmt, fmt.Sprintf("internal compiler error: failed to resolve var '%s' in for-in", name))
			}
			if symbolRef.IsGlobal {
				// Update global via OpSetGlobal using the assigned global index
				c.emitSetGlobal(symbolRef.GlobalIndex, currentKeyReg, node.Token.Line)
			} else {
				// Local (function-scoped) var: ensure it has a register then move
				if symbolRef.Register == nilRegister {
					reg := c.regAlloc.Alloc()
					// Pin register since it may be captured by closures
					c.regAlloc.Pin(reg)
					c.currentSymbolTable.UpdateRegister(name, reg)
					symbolRef.Register = reg
				}
				c.emitMove(symbolRef.Register, currentKeyReg, node.Token.Line)
			}
		}
	} else if exprStmt, ok := node.Variable.(*parser.ExpressionStatement); ok {
		// This is an existing variable being assigned to
		if ident, ok := exprStmt.Expression.(*parser.Identifier); ok {
			symbolRef, definingTable, found := c.currentSymbolTable.Resolve(ident.Value)
			if !found {
				fmt.Printf("// [ForInAssign] unresolved %s, defining var in outermost scope\n", ident.Value)
				// Define a function/global-scoped binding (var semantics)
				if c.enclosing == nil {
					idx := c.GetOrAssignGlobalIndex(ident.Value)
					c.currentSymbolTable.DefineGlobal(ident.Value, idx)
					c.emitSetGlobal(idx, currentKeyReg, node.Token.Line)
				} else {
					target := c.currentSymbolTable
					for target.Outer != nil {
						target = target.Outer
					}
					reg := c.regAlloc.Alloc()
					tempRegs = append(tempRegs, reg)
					sym := target.Define(ident.Value, reg)
					c.regAlloc.Pin(sym.Register)
					c.emitMove(sym.Register, currentKeyReg, node.Token.Line)
				}
			} else {
				// Check if this is a global variable or local register
				if symbolRef.IsGlobal {
					fmt.Printf("// [ForInAssign] writing global %s\n", ident.Value)
					c.emitSetGlobal(symbolRef.GlobalIndex, currentKeyReg, node.Token.Line)
				} else {
					// Store key value in the existing variable's register for local variables
					_ = definingTable
					fmt.Printf("// [ForInAssign] writing local %s R%d\n", ident.Value, symbolRef.Register)
					c.emitMove(symbolRef.Register, currentKeyReg, node.Token.Line)
				}
			}
		}
	}

	// 7. Compile loop body
	bodyReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, bodyReg)
	if _, err := c.compileNode(node.Body, bodyReg); err != nil {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, err
	}

	// 8. Patch continue jumps to land here (before increment)
	for _, continuePos := range loopContext.ContinuePlaceholderPosList {
		c.patchJump(continuePos)
	}

	// 9. Increment key index
	oneReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, oneReg)
	c.emitLoadNewConstant(oneReg, vm.Number(1), node.Token.Line)
	c.emitOpCode(vm.OpAdd, node.Token.Line)
	c.emitByte(byte(keyIndexReg)) // destination (reuse keyIndexReg)
	c.emitByte(byte(keyIndexReg)) // left operand (current key index)
	c.emitByte(byte(oneReg))      // right operand (1)

	// 10. Jump back to loop start
	jumpBackInstructionEndPos := len(c.chunk.Code) + 1 + 2
	backOffset := loopStartPos - jumpBackInstructionEndPos
	c.emitOpCode(vm.OpJump, node.Body.Token.Line)
	c.emitUint16(uint16(int16(backOffset)))

	// 11. Clean up loop context and patch jumps
	poppedContext := c.loopContextStack[len(c.loopContextStack)-1]
	c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]

	// Patch condition exit jump
	c.patchJump(conditionExitJumpPos)
	patchedConditionExit = true

	// Patch all break jumps
	for _, breakPos := range poppedContext.BreakPlaceholderPosList {
		c.patchJump(breakPos)
	}

	return BadRegister, nil
}

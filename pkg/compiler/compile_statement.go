package compiler

import (
	"fmt"
	"math"
	"paserati/pkg/errors"
	"paserati/pkg/parser"
	"paserati/pkg/vm"
)

func (c *Compiler) compileLetStatement(node *parser.LetStatement, hint Register) (Register, errors.PaseratiError) {
	debugPrintf("// DEBUG compileLetStatement: Defining '%s' (is top-level: %v)\n", node.Name.Value, c.enclosing == nil) // <<< ADDED
	var valueReg Register = nilRegister
	var err errors.PaseratiError
	isValueFunc := false // Flag to track if value is a function literal

	if funcLit, ok := node.Value.(*parser.FunctionLiteral); ok {
		isValueFunc = true
		// --- Handle let f = function g() {} or let f = function() {} ---
		// 1. Define the *variable name (f)* temporarily for potential recursion
		//    within the function body (e.g., recursive anonymous function).
		debugPrintf("// DEBUG compileLetStatement: Defining function '%s' temporarily with nilRegister\n", node.Name.Value) // <<< ADDED
		c.currentSymbolTable.Define(node.Name.Value, nilRegister)

		// 2. Compile the function literal body.
		//    Pass the variable name (f) as the hint for the function object's name
		//    if the function literal itself is anonymous.
		// <<< MODIFY Call Site >>>
		funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, node.Name.Value)
		if err != nil {
			// Error already added to c.errors by compileFunctionLiteral
			return BadRegister, nil // Return nil error here, main error is tracked
		}
		// 3. Create the closure object
		closureReg := c.regAlloc.Alloc()
		debugPrintf("// DEBUG compileLetStatement: Creating closure for '%s' in R%d with %d upvalues\n", node.Name.Value, closureReg, len(freeSymbols)) // <<< ADDED
		c.emitClosure(closureReg, funcConstIndex, funcLit, freeSymbols)                                                                                 // <<< Call emitClosure

		// 4. Update the symbol table entry for the *variable name (f)* with the closure register.
		debugPrintf("// DEBUG compileLetStatement: Updating symbol table for '%s' from nilRegister to R%d\n", node.Name.Value, closureReg) // <<< ADDED
		c.currentSymbolTable.UpdateRegister(node.Name.Value, closureReg)                                                                   // <<< Use closureReg

		// The variable's value (the closure) is now set.
		// We don't need to assign to valueReg anymore for this path.

	} else if node.Value != nil {
		// Compile other value types normally
		_, err = c.compileNode(node.Value, NoHint)
		if err != nil {
			return BadRegister, err
		}
		valueReg = c.regAlloc.Current()
	} // else: node.Value is nil (implicit undefined handled below)

	// Handle implicit undefined (`let x;`)
	if valueReg == nilRegister && !isValueFunc { // <<< Check !isValueFunc
		undefReg := c.regAlloc.Alloc()
		c.emitLoadUndefined(undefReg, node.Name.Token.Line)
		valueReg = undefReg
		// Define symbol for the `let x;` case
		debugPrintf("// DEBUG compileLetStatement: Defining '%s' with undefined value in R%d\n", node.Name.Value, valueReg) // <<< ADDED
		if c.enclosing == nil {
			// Top-level: use global variable
			globalIdx := c.getOrAssignGlobalIndex(node.Name.Value)
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
		debugPrintf("// DEBUG compileLetStatement: Defining '%s' with value in R%d\n", node.Name.Value, valueReg) // <<< ADDED
		if c.enclosing == nil {
			// Top-level: use global variable
			globalIdx := c.getOrAssignGlobalIndex(node.Name.Value)
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
		globalIdx := c.getOrAssignGlobalIndex(node.Name.Value)
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
		// <<< MODIFY Call Site >>>
		funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, node.Name.Value)
		if err != nil {
			// Error already added to c.errors by compileFunctionLiteral
			return BadRegister, nil // Return nil error here, main error is tracked
		}
		// 3. Create the closure object
		closureReg := c.regAlloc.Alloc()
		c.emitClosure(closureReg, funcConstIndex, funcLit, freeSymbols) // <<< Call emitClosure

		// 4. Update the temporary definition for the *const name (f)* with the closure register.
		c.currentSymbolTable.UpdateRegister(node.Name.Value, closureReg) // <<< Use closureReg

		// The constant's value (the closure) is now set.
		// We don't need to assign to valueReg anymore for this path.

	} else {
		// Compile other value types normally
		_, err = c.compileNode(node.Value, NoHint)
		if err != nil {
			return BadRegister, err
		}
		valueReg = c.regAlloc.Current()
	}

	// Define symbol ONLY for non-function values.
	// Const function assignments were handled above by UpdateRegister.
	if !isValueFunc {
		// For non-functions, Define associates the name with the final value register.
		if c.enclosing == nil {
			// Top-level: use global variable
			globalIdx := c.getOrAssignGlobalIndex(node.Name.Value)
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
		globalIdx := c.getOrAssignGlobalIndex(node.Name.Value)
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
			// <<< MODIFY Call Site >>>
			funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, "")
			if err != nil {
				// Error already added to c.errors by compileFunctionLiteral
				return BadRegister, nil // Return nil error here, main error is tracked
			}
			// Create the closure object
			closureReg := c.regAlloc.Alloc()
			c.emitClosure(closureReg, funcConstIndex, funcLit, freeSymbols) // <<< Call emitClosure
			returnReg = closureReg                                          // <<< Closure is the value to return

		} else {
			// Compile other expression types normally via compileNode
			_, err = c.compileNode(node.ReturnValue, NoHint)
			if err != nil {
				return BadRegister, err
			}
			returnReg = c.regAlloc.Current() // Value to return is in the last allocated reg
		}

		// Error check should cover both paths now
		if err != nil {
			// This check might be redundant if errors are handled correctly above,
			// but keep for safety unless proven otherwise.
			return BadRegister, err
		}
		// Emit return using the register holding the final value (closure or other expression result)
		c.emitReturn(returnReg, node.Token.Line) // <<< Use potentially updated returnReg
	} else {
		// Return undefined implicitly using the optimized opcode
		c.emitOpCode(vm.OpReturnUndefined, node.Token.Line)
	}
	return BadRegister, nil
}

// --- Loop Compilation (Updated) ---

func (c *Compiler) compileWhileStatement(node *parser.WhileStatement, hint Register) (Register, errors.PaseratiError) {
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
	_, err := c.compileNode(node.Condition, NoHint)
	if err != nil {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1] // Pop context on error
		return BadRegister, NewCompileError(node, "error compiling while condition").CausedBy(err)
	}
	conditionReg := c.regAlloc.Current()

	// --- Jump Out If False ---
	jumpToEndPlaceholderPos := c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, line)

	// --- Compile Body ---
	_, err = c.compileNode(node.Body, NoHint)
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
	// No new scope for initializer, it shares the outer scope

	// 1. Initializer
	if node.Initializer != nil {
		if _, err := c.compileNode(node.Initializer, NoHint); err != nil {
			return BadRegister, err
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
		if _, err := c.compileNode(node.Condition, NoHint); err != nil {
			// Clean up loop context if condition compilation fails
			c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
			return BadRegister, err
		}
		conditionReg := c.regAlloc.Current()
		conditionExitJumpPlaceholderPos = c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)
	} // If no condition, it's an infinite loop (handled by break/return)

	// --- 3. Body ---
	// Continue placeholders will be added to loopContext here
	if _, err := c.compileNode(node.Body, NoHint); err != nil {
		// Clean up loop context if body compilation fails
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, err
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
		if _, err := c.compileNode(node.Update, NoHint); err != nil {
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

	return BadRegister, nil
}

// --- New: Break/Continue Compilation ---

func (c *Compiler) compileBreakStatement(node *parser.BreakStatement, hint Register) (Register, errors.PaseratiError) {
	if len(c.loopContextStack) == 0 {
		return BadRegister, NewCompileError(node, "break statement not within a loop")
	}

	// Get current loop context (top of stack)
	currentLoopContext := c.loopContextStack[len(c.loopContextStack)-1]

	// Emit placeholder jump (OpJump) - Pass 0 for srcReg as it's ignored
	placeholderPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)

	// Add placeholder position to the context's list for later patching
	currentLoopContext.BreakPlaceholderPosList = append(currentLoopContext.BreakPlaceholderPosList, placeholderPos)

	return BadRegister, nil
}

func (c *Compiler) compileContinueStatement(node *parser.ContinueStatement, hint Register) (Register, errors.PaseratiError) {
	if len(c.loopContextStack) == 0 {
		return BadRegister, NewCompileError(node, "continue statement not within a loop")
	}

	// Get current loop context (top of stack)
	currentLoopContext := c.loopContextStack[len(c.loopContextStack)-1]

	// Emit placeholder jump (OpJump) - Pass 0 for srcReg as it's ignored
	placeholderPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)

	// Add placeholder position to the context's list for later patching
	currentLoopContext.ContinuePlaceholderPosList = append(currentLoopContext.ContinuePlaceholderPosList, placeholderPos)

	return BadRegister, nil
}

func (c *Compiler) compileDoWhileStatement(node *parser.DoWhileStatement, hint Register) (Register, errors.PaseratiError) {
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
	if _, err := c.compileNode(node.Body, NoHint); err != nil {
		// Pop context if body compilation fails
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, NewCompileError(node, "error compiling do-while body").CausedBy(err)
	}

	// 4. Mark Condition Position (for clarity, not used directly in jump calcs below)
	_ = len(c.chunk.Code) // conditionPos := len(c.chunk.Code)

	// 5. Compile Condition
	if _, err := c.compileNode(node.Condition, NoHint); err != nil {
		// Pop context if condition compilation fails
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, NewCompileError(node, "error compiling do-while condition").CausedBy(err)
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
		return BadRegister, NewCompileError(node, fmt.Sprintf("internal compiler error: do-while loop jump offset %d exceeds 16-bit limit", backOffset))
	}
	c.emitOpCode(vm.OpJumpIfFalse, line)    // Use OpJumpIfFalse on inverted result
	c.emitByte(byte(invertedConditionReg))  // Jump based on the inverted condition
	c.emitUint16(uint16(int16(backOffset))) // Emit calculated signed offset

	// Free the temporary inverted condition register
	c.regAlloc.Free(invertedConditionReg)

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
	// 1. Compile the expression being switched on
	_, err := c.compileNode(node.Expression, NoHint)
	if err != nil {
		return BadRegister, err
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
			_, err = c.compileNode(caseClause.Condition, NoHint)
			if err != nil {
				return BadRegister, err
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
			_, err = c.compileNode(caseClause.Body, NoHint)
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
			_, err = c.compileNode(caseClause.Body, NoHint)
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

	return BadRegister, nil
}

func (c *Compiler) compileForOfStatement(node *parser.ForOfStatement, hint Register) (Register, errors.PaseratiError) {
	// 1. Compile the iterable expression first
	if _, err := c.compileNode(node.Iterable, NoHint); err != nil {
		return BadRegister, err
	}
	iterableReg := c.regAlloc.Current()

	// 2. Set up iteration variables
	// For arrays: we need an index counter and length
	indexReg := c.regAlloc.Alloc()
	lengthReg := c.regAlloc.Alloc()
	elementReg := c.regAlloc.Alloc()

	// Initialize index to 0
	c.emitLoadNewConstant(indexReg, vm.Number(0), node.Token.Line)

	// Get length of iterable (for arrays, use .length property)
	// For now, assume it's an array and get its length
	c.emitOpCode(vm.OpGetProp, node.Token.Line)
	c.emitByte(byte(lengthReg))   // destination register
	c.emitByte(byte(iterableReg)) // object register
	lengthConstIdx := c.chunk.AddConstant(vm.String("length"))
	c.emitUint16(lengthConstIdx) // property name constant index

	// --- Loop Start & Context Setup ---
	loopStartPos := len(c.chunk.Code)
	loopContext := &LoopContext{
		LoopStartPos:               loopStartPos,
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
	}
	c.loopContextStack = append(c.loopContextStack, loopContext)

	// 3. Check if index < length (loop condition)
	conditionReg := c.regAlloc.Alloc()
	c.emitOpCode(vm.OpLess, node.Token.Line)
	c.emitByte(byte(conditionReg)) // destination
	c.emitByte(byte(indexReg))     // left operand (index)
	c.emitByte(byte(lengthReg))    // right operand (length)

	// Jump out of loop if condition is false
	conditionExitJumpPos := c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)

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
	} else if exprStmt, ok := node.Variable.(*parser.ExpressionStatement); ok {
		// This is an existing variable being assigned to
		if ident, ok := exprStmt.Expression.(*parser.Identifier); ok {
			symbolRef, _, found := c.currentSymbolTable.Resolve(ident.Value)
			if !found {
				return BadRegister, NewCompileError(ident, fmt.Sprintf("undefined variable '%s'", ident.Value))
			}
			// Store element value in the existing variable's register
			c.emitMove(symbolRef.Register, elementReg, node.Token.Line)
		}
	}

	// 6. Compile loop body
	if _, err := c.compileNode(node.Body, NoHint); err != nil {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, err
	}

	// 7. Patch continue jumps to land here (before increment)
	for _, continuePos := range loopContext.ContinuePlaceholderPosList {
		c.patchJump(continuePos)
	}

	// 8. Increment index
	oneReg := c.regAlloc.Alloc()
	c.emitLoadNewConstant(oneReg, vm.Number(1), node.Token.Line)
	c.emitOpCode(vm.OpAdd, node.Token.Line)
	c.emitByte(byte(indexReg)) // destination (reuse indexReg)
	c.emitByte(byte(indexReg)) // left operand (current index)
	c.emitByte(byte(oneReg))   // right operand (1)

	// Free the temporary constant register
	c.regAlloc.Free(oneReg)

	// 9. Jump back to loop start
	jumpBackInstructionEndPos := len(c.chunk.Code) + 1 + 2
	backOffset := loopStartPos - jumpBackInstructionEndPos
	c.emitOpCode(vm.OpJump, node.Body.Token.Line)
	c.emitUint16(uint16(int16(backOffset)))

	// 10. Clean up loop context and patch jumps
	poppedContext := c.loopContextStack[len(c.loopContextStack)-1]
	c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]

	// Patch condition exit jump
	c.patchJump(conditionExitJumpPos)

	// Patch all break jumps
	for _, breakPos := range poppedContext.BreakPlaceholderPosList {
		c.patchJump(breakPos)
	}

	return BadRegister, nil
}

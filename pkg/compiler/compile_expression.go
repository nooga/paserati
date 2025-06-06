package compiler

import (
	"fmt"
	"paserati/pkg/errors"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// Helper functions for detecting null and undefined literals
func isNullLiteral(node parser.Expression) bool {
	_, ok := node.(*parser.NullLiteral)
	return ok
}

func isUndefinedLiteral(node parser.Expression) bool {
	_, ok := node.(*parser.UndefinedLiteral)
	return ok
}

func (c *Compiler) compileNewExpression(node *parser.NewExpression, hint Register) (Register, errors.PaseratiError) {
	// 1. Compile the constructor expression
	constructorReg, err := c.compileNode(node.Constructor, NoHint)
	if err != nil {
		return BadRegister, err
	}

	// 2. Compile arguments
	argRegs := []Register{}
	for _, arg := range node.Arguments {
		argReg, err := c.compileNode(arg, NoHint)
		if err != nil {
			return BadRegister, err
		}
		argRegs = append(argRegs, argReg)
	}
	argCount := len(argRegs)

	// 3. Ensure arguments are in the correct registers for the call convention.
	// Convention: Args must be in registers constructorReg+1, constructorReg+2, ...
	if argCount > 0 {
		c.resolveRegisterMoves(argRegs, constructorReg+1, node.Token.Line)
		// DISABLED: Register freeing was corrupting constructor calls
		// Free the original argument registers after resolving moves
		// for _, argReg := range argRegs {
		// 	// Only free if it's different from the target register
		// 	targetReg := constructorReg + 1 + Register(argCount-1) // Last target register
		// 	if argReg > targetReg || argReg < constructorReg+1 {
		// 		c.regAlloc.Free(argReg)
		// 	}
		// }
	}

	// 4. Allocate register for the created instance
	resultReg := c.regAlloc.Alloc()

	// 5. Emit OpNew (constructor call)
	c.emitNew(resultReg, constructorReg, byte(argCount), node.Token.Line)

	// Free the constructor register after the operation
	c.regAlloc.Free(constructorReg)

	// The result of the new operation is now in resultReg
	return resultReg, nil
}

// compileMemberExpression compiles expressions like obj.prop or obj['prop'] (latter is future work)
func (c *Compiler) compileMemberExpression(node *parser.MemberExpression, hint Register) (Register, errors.PaseratiError) {
	// 1. Compile the object part
	objectReg, err := c.compileNode(node.Object, NoHint)
	if err != nil {
		return BadRegister, NewCompileError(node.Object, "error compiling object part of member expression").CausedBy(err)
	}

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
			return BadRegister, NewCompileError(node.Object, "compiler internal error: checker did not provide type for object in member expression")
		}

		// Widen the type to handle cases like `string | null` having `.length`
		widenedObjectType := types.GetWidenedType(objectStaticType)

		// Check if the widened type supports .length
		_, isArray := widenedObjectType.(*types.ArrayType)
		if isArray || widenedObjectType == types.String {
			// Emit specialized OpGetLength
			destReg := c.regAlloc.Alloc()
			c.emitGetLength(destReg, objectReg, node.Token.Line)
			// Free objectReg? Maybe not needed if GetLength copies or doesn't invalidate.
			// c.regAlloc.Free(objectReg)
			return destReg, nil // Handled by OpGetLength
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

	// DISABLED: Potentially problematic register freeing
	// Free the object register after the property access
	// c.regAlloc.Free(objectReg)

	// Result is now in destReg
	// Note: We don't need to set lastExprReg/Valid here, as compileNode will handle it.
	return destReg, nil
}

// compileOptionalChainingExpression compiles optional chaining property access (e.g., obj?.prop)
// This is similar to compileMemberExpression but adds null/undefined checks
func (c *Compiler) compileOptionalChainingExpression(node *parser.OptionalChainingExpression, hint Register) (Register, errors.PaseratiError) {
	// 1. Compile the object part
	objectReg, err := c.compileNode(node.Object, NoHint)
	if err != nil {
		return BadRegister, NewCompileError(node.Object, "error compiling object part of optional chaining expression").CausedBy(err)
	}

	// 2. Check if the object is null or undefined
	// If so, return undefined immediately

	// Allocate destination register first
	destReg := c.regAlloc.Alloc()

	// Use efficient nullish check opcode - much more register efficient!
	isNullishReg := c.regAlloc.Alloc()
	c.emitIsNullish(isNullishReg, objectReg, node.Token.Line)

	// If object is NOT nullish, jump to normal property access
	jumpToPropertyAccessPos := c.emitPlaceholderJump(vm.OpJumpIfFalse, isNullishReg, node.Token.Line)

	// Object IS nullish - return undefined
	c.emitLoadUndefined(destReg, node.Token.Line)
	endJumpPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)

	// Object is not null/undefined - do normal property access
	c.patchJump(jumpToPropertyAccessPos)

	// Free temporary register as it's no longer needed
	c.regAlloc.Free(isNullishReg)

	// 3. Get Property Name
	propertyName := node.Property.Value

	// 4. Special case for .length (same as regular member access)
	if propertyName == "length" {
		objectStaticType := node.Object.GetComputedType()
		if objectStaticType != nil {
			widenedObjectType := types.GetWidenedType(objectStaticType)
			_, isArray := widenedObjectType.(*types.ArrayType)
			if isArray || widenedObjectType == types.String {
				c.emitGetLength(destReg, objectReg, node.Token.Line)
				c.patchJump(endJumpPos)
				return destReg, nil
			}
		}
	}

	// 5. Add property name string to constant pool and emit OpGetProp
	nameConstIdx := c.chunk.AddConstant(vm.String(propertyName))
	c.emitGetProp(destReg, objectReg, nameConstIdx, node.Token.Line)

	// DISABLED: Potentially problematic register freeing
	// Free the object register after the property access
	// c.regAlloc.Free(objectReg)

	// 6. Patch the end jump
	c.patchJump(endJumpPos)

	// Result is now in destReg
	return destReg, nil
}

func (c *Compiler) compileIndexExpression(node *parser.IndexExpression, hint Register) (Register, errors.PaseratiError) {
	line := parser.GetTokenFromNode(node).Line                           // Use '[' token line
	debugPrintf(">>> Enter compileIndexExpression: %s\n", node.String()) // <<< DEBUG ENTRY

	// 1. Compile the expression being indexed (the base: array/object/string)
	debugPrintf("--- Compiling Base: %s\n", node.Left.String()) // <<< DEBUG
	arrayReg, err := c.compileNode(node.Left, NoHint)
	if err != nil {
		debugPrintf("<<< Exit compileIndexExpression (Error compiling base)\n") // <<< DEBUG EXIT
		return BadRegister, NewCompileError(node.Left, "error compiling base of index expression").CausedBy(err)
	}

	// 2. Compile the index expression
	debugPrintf("--- Compiling Index: %s\n", node.Index.String()) // <<< DEBUG
	indexReg, err := c.compileNode(node.Index, NoHint)
	if err != nil {
		debugPrintf("<<< Exit compileIndexExpression (Error compiling index)\n") // <<< DEBUG EXIT
		// Note: Need to consider freeing baseReg here if it was allocated and valid
		return BadRegister, NewCompileError(node.Index, "error compiling index part of index expression").CausedBy(err)
	}
	// <<< DEBUG INDEX RESULT >>>
	debugPrintf("--- Index Compiled. regAlloc.Current(): %s\n", indexReg)

	// 3. Allocate register for the result
	destReg := c.regAlloc.Alloc()
	debugPrintf("--- Allocated destReg = %s\n", destReg) // <<< DEBUG DEST REG

	// 4. Emit OpGetIndex
	debugPrintf("--- Emitting OpGetIndex %s, %s, %s (Dest, Base, Index)\n", destReg, arrayReg, indexReg) // <<< DEBUG EMIT
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(destReg))
	c.emitByte(byte(arrayReg)) // Using potentially wrong base register
	c.emitByte(byte(indexReg)) // Using potentially wrong index register

	// Free operand registers after they've been used
	// Make sure we don't free the result register
	if arrayReg != destReg {
		c.regAlloc.Free(arrayReg)
	}
	if indexReg != destReg && indexReg != arrayReg {
		c.regAlloc.Free(indexReg)
	}

	// Result is now in destReg
	debugPrintf("<<< Exit compileIndexExpression (Success)\n") // <<< DEBUG EXIT
	return destReg, nil
}

func (c *Compiler) compileUpdateExpression(node *parser.UpdateExpression, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line

	// Define types for different lvalue kinds
	type lvalueType int
	const (
		lvalueIdentifier lvalueType = iota
		lvalueMemberExpr
		lvalueIndexExpr
	)

	var lvalueKind lvalueType
	var currentValueReg Register // Register holding the current value before increment/decrement

	// Information needed for storing back to different lvalue types
	var identInfo struct {
		targetReg    Register
		isUpvalue    bool
		upvalueIndex uint8
		isGlobal     bool
		globalIndex  uint16
	}
	var memberInfo struct {
		objectReg    Register
		nameConstIdx uint16
	}
	var indexInfo struct {
		arrayReg Register
		indexReg Register
	}

	// 1. Determine lvalue type and load current value
	switch argNode := node.Argument.(type) {
	case *parser.Identifier:
		lvalueKind = lvalueIdentifier
		// Resolve identifier and determine if local or upvalue
		symbolRef, definingTable, found := c.currentSymbolTable.Resolve(argNode.Value)
		if !found {
			return BadRegister, NewCompileError(node, fmt.Sprintf("applying %s to undeclared variable '%s'", node.Operator, argNode.Value))
		}

		if symbolRef.IsGlobal {
			// Global variable: Load current value with OpGetGlobal
			identInfo.isGlobal = true
			identInfo.globalIndex = symbolRef.GlobalIndex
			currentValueReg = c.regAlloc.Alloc()
			c.emitGetGlobal(currentValueReg, symbolRef.GlobalIndex, line)
		} else if definingTable == c.currentSymbolTable {
			// Local variable: Get its register
			identInfo.targetReg = symbolRef.Register
			identInfo.isUpvalue = false
			identInfo.isGlobal = false
			currentValueReg = identInfo.targetReg // Current value is already in targetReg
		} else {
			// Upvalue: Get its index and load current value into a temporary register
			identInfo.isUpvalue = true
			identInfo.isGlobal = false
			identInfo.upvalueIndex = c.addFreeSymbol(node, &symbolRef)
			currentValueReg = c.regAlloc.Alloc()
			c.emitOpCode(vm.OpLoadFree, line)
			c.emitByte(byte(currentValueReg))
			c.emitByte(identInfo.upvalueIndex)
		}

	case *parser.MemberExpression:
		lvalueKind = lvalueMemberExpr
		// Compile the object part
		objectReg, err := c.compileNode(argNode.Object, NoHint)
		if err != nil {
			return BadRegister, NewCompileError(argNode.Object, "error compiling object part of member expression").CausedBy(err)
		}
		memberInfo.objectReg = objectReg

		// Get property name (assume identifier property for now: obj.prop)
		propIdent := argNode.Property
		propName := propIdent.Value
		memberInfo.nameConstIdx = c.chunk.AddConstant(vm.String(propName))

		// Load current property value
		currentValueReg = c.regAlloc.Alloc()
		c.emitGetProp(currentValueReg, memberInfo.objectReg, memberInfo.nameConstIdx, line)

	case *parser.IndexExpression:
		lvalueKind = lvalueIndexExpr
		// Compile array expression
		arrayReg, err := c.compileNode(argNode.Left, NoHint)
		if err != nil {
			return BadRegister, NewCompileError(argNode.Left, "error compiling array part of index expression").CausedBy(err)
		}
		indexInfo.arrayReg = arrayReg

		// Compile index expression
		indexReg, err := c.compileNode(argNode.Index, NoHint)
		if err != nil {
			return BadRegister, NewCompileError(argNode.Index, "error compiling index part of index expression").CausedBy(err)
		}
		indexInfo.indexReg = indexReg

		// Load the current value at the index
		currentValueReg = c.regAlloc.Alloc()
		c.emitOpCode(vm.OpGetIndex, line)
		c.emitByte(byte(currentValueReg))
		c.emitByte(byte(indexInfo.arrayReg))
		c.emitByte(byte(indexInfo.indexReg))

	default:
		return BadRegister, NewCompileError(node, fmt.Sprintf("invalid target for %s: expected identifier, member expression, or index expression, got %T", node.Operator, node.Argument))
	}

	// 2. Load constant 1
	constOneReg := c.regAlloc.Alloc()
	constOneIdx := c.chunk.AddConstant(vm.Number(1))
	c.emitLoadConstant(constOneReg, constOneIdx, line)

	// 3. Perform Pre/Post logic
	resultReg := c.regAlloc.Alloc() // Register to hold the expression's final result

	if node.Prefix {
		// Prefix (++x or --x):
		// a. Operate: currentValueReg = currentValueReg +/- constOneReg
		switch node.Operator {
		case "++":
			c.emitAdd(currentValueReg, currentValueReg, constOneReg, line)
		case "--":
			c.emitSubtract(currentValueReg, currentValueReg, constOneReg, line)
		}
		// b. Store back to lvalue
		c.storeToLvalue(int(lvalueKind), identInfo, memberInfo, indexInfo, currentValueReg, line)
		// c. Result of expression is the *new* value
		c.emitMove(resultReg, currentValueReg, line)

	} else {
		// Postfix (x++ or x--):
		// a. Save original value: resultReg = currentValueReg
		c.emitMove(resultReg, currentValueReg, line)
		// b. Operate: currentValueReg = currentValueReg +/- constOneReg
		switch node.Operator {
		case "++":
			c.emitAdd(currentValueReg, currentValueReg, constOneReg, line)
		case "--":
			c.emitSubtract(currentValueReg, currentValueReg, constOneReg, line)
		}
		// c. Store back to lvalue
		c.storeToLvalue(int(lvalueKind), identInfo, memberInfo, indexInfo, currentValueReg, line)
		// d. Result of expression is the *original* value (already saved in resultReg)
	}

	// 4. Clean up temporary registers
	c.regAlloc.Free(constOneReg)
	// Free the currentValueReg if it was allocated as temporary (for upvalues, member exprs, index exprs, globals)
	if lvalueKind != lvalueIdentifier || identInfo.isUpvalue || identInfo.isGlobal {
		c.regAlloc.Free(currentValueReg)
	}
	// Free object/array/index registers for member/index expressions
	if lvalueKind == lvalueMemberExpr {
		c.regAlloc.Free(memberInfo.objectReg)
	} else if lvalueKind == lvalueIndexExpr {
		c.regAlloc.Free(indexInfo.arrayReg)
		c.regAlloc.Free(indexInfo.indexReg)
	}

	return resultReg, nil
}

// compileTernaryExpression compiles condition ? consequence : alternative
func (c *Compiler) compileTernaryExpression(node *parser.TernaryExpression, hint Register) (Register, errors.PaseratiError) {
	// 1. Compile condition
	conditionReg, err := c.compileNode(node.Condition, NoHint)
	if err != nil {
		return BadRegister, err
	}

	// 2. Jump if false
	jumpFalsePos := c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)
	// Free condition register now that jump is emitted
	c.regAlloc.Free(conditionReg)

	// --- Consequence Path ---
	// 3. Compile consequence
	consequenceReg, err := c.compileNode(node.Consequence, NoHint)
	if err != nil {
		return BadRegister, err
	}

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
	alternativeReg, err := c.compileNode(node.Alternative, NoHint)
	if err != nil {
		return BadRegister, err
	}

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
	return finalReg, nil
}

func (c *Compiler) compileInfixExpression(node *parser.InfixExpression, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line // Use operator token line number

	// --- Standard binary operators (arithmetic, comparison, bitwise, shift) ---
	if node.Operator != "&&" && node.Operator != "||" && node.Operator != "??" {
		// --- OPTIMIZATION: Special handling for null/undefined comparisons ---
		if node.Operator == "===" || node.Operator == "!==" {
			// Check if one operand is a null or undefined literal
			leftIsNull := isNullLiteral(node.Left)
			leftIsUndefined := isUndefinedLiteral(node.Left)
			rightIsNull := isNullLiteral(node.Right)
			rightIsUndefined := isUndefinedLiteral(node.Right)

			if leftIsNull || leftIsUndefined || rightIsNull || rightIsUndefined {
				// One side is null/undefined literal - use efficient opcodes!
				var valueReg Register
				var err errors.PaseratiError
				var isNullCheck bool

				if leftIsNull || leftIsUndefined {
					// Compile the non-literal side (right)
					valueReg, err = c.compileNode(node.Right, NoHint)
					isNullCheck = leftIsNull
				} else {
					// Compile the non-literal side (left)
					valueReg, err = c.compileNode(node.Left, NoHint)
					isNullCheck = rightIsNull
				}

				if err != nil {
					return BadRegister, err
				}

				destReg := c.regAlloc.Alloc()

				if isNullCheck {
					c.emitIsNull(destReg, valueReg, line)
				} else {
					c.emitIsUndefined(destReg, valueReg, line)
				}

				// If this is !== (strict not equal), we need to negate the result
				if node.Operator == "!==" {
					c.emitNot(destReg, destReg, line)
				}

				return destReg, nil
			}
		}

		leftReg, err := c.compileNode(node.Left, NoHint)
		if err != nil {
			return BadRegister, err
		}

		rightReg, err := c.compileNode(node.Right, NoHint)
		if err != nil {
			return BadRegister, err
		}

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
			return BadRegister, NewCompileError(node, fmt.Sprintf("unknown standard infix operator '%s'", node.Operator))
		}
		// Result is now in destReg (allocator current is destReg)

		// DISABLED: Register freeing was too aggressive and causing correctness issues
		// Free operand registers after use (check against destReg for safety)
		// if leftReg != destReg {
		// 	c.regAlloc.Free(leftReg)
		// }
		// if rightReg != destReg {
		// 	c.regAlloc.Free(rightReg)
		// }

		return destReg, nil
	}

	// --- Logical Operators (&&, ||, ??) with Short-Circuiting ---
	// Allocate result register *before* compiling operands for logical ops too
	destReg := c.regAlloc.Alloc()

	if node.Operator == "||" { // a || b
		leftReg, err := c.compileNode(node.Left, NoHint)
		if err != nil {
			return BadRegister, err
		}

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
		rightReg, err := c.compileNode(node.Right, NoHint)
		if err != nil {
			return BadRegister, err
		}
		// Result is right, move to destReg
		c.emitMove(destReg, rightReg, line)
		// Free rightReg after moving its value
		c.regAlloc.Free(rightReg)

		// Patch jumpToEndPlaceholder to land here
		c.patchJump(jumpToEndPlaceholder)
		// Result is now unified in destReg
		return destReg, nil

	} else if node.Operator == "&&" { // a && b
		leftReg, err := c.compileNode(node.Left, NoHint)
		if err != nil {
			return BadRegister, err
		}

		// If left is FALSEY, jump to end, result is left
		jumpToEndPlaceholder := c.emitPlaceholderJump(vm.OpJumpIfFalse, leftReg, line)

		// If left was TRUTHY (didn't jump), compile right operand
		rightReg, err := c.compileNode(node.Right, NoHint)
		if err != nil {
			return BadRegister, err
		}
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
		return destReg, nil

	} else if node.Operator == "??" { // a ?? b
		leftReg, err := c.compileNode(node.Left, NoHint)
		if err != nil {
			return BadRegister, err
		}

		// Use efficient nullish check opcode - much more register efficient!
		isNullishReg := c.regAlloc.Alloc()
		c.emitIsNullish(isNullishReg, leftReg, line)

		// Jump if *NOT* nullish (jump if false) to skip the right side eval
		jumpSkipRightPlaceholder := c.emitPlaceholderJump(vm.OpJumpIfFalse, isNullishReg, line)

		// Free the check register early since we don't need it anymore
		c.regAlloc.Free(isNullishReg)

		// --- Eval Right Path ---
		// Compile right operand (only executed if left was nullish)
		rightReg, err := c.compileNode(node.Right, NoHint)
		if err != nil {
			return BadRegister, err
		}
		// Move result to destReg
		c.emitMove(destReg, rightReg, line)
		// Free rightReg after move
		c.regAlloc.Free(rightReg)

		// Jump over the skip-right landing pad
		jumpEndPlaceholder := c.emitPlaceholderJump(vm.OpJump, 0, line)

		// --- Skip Right Path ---
		// Land here if left was NOT nullish. Patch the jump from the nullish check.
		c.patchJump(jumpSkipRightPlaceholder)
		// Result is left (not nullish), move it to destReg.
		c.emitMove(destReg, leftReg, line)
		// Free leftReg after move
		c.regAlloc.Free(leftReg)

		// Land here after either path finishes. Patch the jump from the right-eval path.
		c.patchJump(jumpEndPlaceholder)

		// Unified result is now in destReg
		return destReg, nil
	}

	return BadRegister, NewCompileError(node, fmt.Sprintf("logical/coalescing operator '%s' compilation fell through", node.Operator))
}

func (c *Compiler) compilePrefixExpression(node *parser.PrefixExpression, hint Register) (Register, errors.PaseratiError) {
	// Compile the right operand first
	rightReg, err := c.compileNode(node.Right, NoHint)
	if err != nil {
		return BadRegister, err
	}

	// Allocate a new register for the result
	destReg := c.regAlloc.Alloc()

	// Emit the corresponding unary opcode
	switch node.Operator {
	case "!":
		c.emitNot(destReg, rightReg, node.Token.Line)
	case "-":
		c.emitNegate(destReg, rightReg, node.Token.Line)
	// --- NEW: Handle Unary Plus (+) ---
	case "+":
		// Unary plus converts operand to number using the dedicated OpToNumber instruction
		c.emitToNumber(destReg, rightReg, node.Token.Line)
	// --- NEW: Handle Void ---
	case "void":
		// void operator evaluates operand (for side effects) then returns undefined
		// Operand is already compiled above (rightReg), so we just load undefined
		c.emitLoadUndefined(destReg, node.Token.Line)
	// --- NEW ---
	case "~":
		c.emitBitwiseNot(destReg, rightReg, node.Token.Line)
	// --- END NEW ---
	default:
		return BadRegister, NewCompileError(node, fmt.Sprintf("unknown prefix operator '%s'", node.Operator))
	}
	// Free the operand register now that the result is in destReg
	c.regAlloc.Free(rightReg)

	// The result is now in destReg
	return destReg, nil
}

func (c *Compiler) compileTypeofExpression(node *parser.TypeofExpression, hint Register) (Register, errors.PaseratiError) {
	// Compile the operand being typeof
	exprReg, err := c.compileNode(node.Operand, NoHint)
	if err != nil {
		return BadRegister, err
	}

	// Allocate a new register for the result
	resultReg := c.regAlloc.Alloc()

	// Emit the OpTypeof instruction
	c.emitTypeof(resultReg, exprReg, node.Token.Line)

	// Free the operand register after the operation
	c.regAlloc.Free(exprReg)

	// The result of the typeof operation is now in resultReg
	return resultReg, nil
}

func (c *Compiler) compileCallExpression(node *parser.CallExpression, hint Register) (Register, errors.PaseratiError) {
	// Check if this is a method call (function is a member expression like obj.method())
	if memberExpr, isMethodCall := node.Function.(*parser.MemberExpression); isMethodCall {
		// Method call: obj.method(args...)
		// 1. Compile the object part
		thisReg, err := c.compileNode(memberExpr.Object, NoHint)
		if err != nil {
			return BadRegister, err
		}

		// 2. Compile the method property access to get the function
		funcReg, err := c.compileMemberExpression(memberExpr, NoHint)
		if err != nil {
			return BadRegister, err
		}

		// 3. Compile arguments and handle optional parameters
		argRegs, totalArgCount, err := c.compileArgumentsWithOptionalHandling(node)
		if err != nil {
			return BadRegister, err
		}

		// 4. Ensure arguments are in the correct registers for the call convention.
		// Convention: Args must be in registers funcReg+1, funcReg+2, ...
		// FIXED: Handle register cycles properly using a temporary register approach
		if totalArgCount > 0 {
			c.resolveRegisterMoves(argRegs, funcReg+1, node.Token.Line)
			// DISABLED: Register freeing was corrupting call expressions
			// Free the original argument registers after resolving moves
			// for _, argReg := range argRegs {
			// 	// Only free if it's different from the target register
			// 	targetReg := funcReg + 1 + Register(totalArgCount-1) // Last target register
			// 	if argReg > targetReg || argReg < funcReg+1 {
			// 		c.regAlloc.Free(argReg)
			// 	}
			// }
		}

		// 5. Allocate register for the return value
		resultReg := c.regAlloc.Alloc()

		// 6. Emit OpCallMethod (method call with 'this' context)
		c.emitCallMethod(resultReg, funcReg, thisReg, byte(totalArgCount), node.Token.Line)

		// The result of the method call is now in resultReg
		return resultReg, nil
	}

	// --- OPTIMIZED: Regular function call with register groups ---
	// Create a register group to manage all call-related registers
	callGroup := c.regAlloc.NewGroup()
	defer callGroup.Release() // Ensure cleanup even if there's an error

	// 1. Compile the expression being called (e.g., function name)
	funcReg, err := c.compileNode(node.Function, NoHint)
	if err != nil {
		return BadRegister, err
	}
	callGroup.Add(funcReg) // Add to group for automatic cleanup

	// 2. Compile arguments and handle optional parameters
	argRegs, totalArgCount, err := c.compileArgumentsWithOptionalHandling(node)
	if err != nil {
		return BadRegister, err
	}

	// Add all argument registers to the group
	for _, argReg := range argRegs {
		callGroup.Add(argReg)
	}

	// 3. Optimize argument layout using linearization
	if totalArgCount > 0 {
		// Create a subgroup for just the arguments
		argGroup := callGroup.SubGroup()
		for _, argReg := range argRegs {
			argGroup.Add(argReg)
		}

		// Try to linearize arguments for optimal register layout
		linearizedFirstArg, err := argGroup.Linearize()
		if err != nil {
			// Linearization failed, fall back to register moves
			c.resolveRegisterMoves(argRegs, funcReg+1, node.Token.Line)
		} else {
			// Check if linearization used existing registers or allocated new ones
			argumentsAlreadyContiguous := (len(argRegs) > 0 && argRegs[0] == linearizedFirstArg)

			if !argumentsAlreadyContiguous {
				// Linearization allocated NEW registers, we need to move our argument values there first
				for i := 0; i < totalArgCount; i++ {
					c.emitMove(linearizedFirstArg+Register(i), argRegs[i], node.Token.Line)
				}
			}

			// Now check if the linearized block is in the right position for the call convention
			if linearizedFirstArg != funcReg+1 {
				// Move the entire linearized argument block to the correct position
				for i := 0; i < totalArgCount; i++ {
					c.emitMove(funcReg+1+Register(i), linearizedFirstArg+Register(i), node.Token.Line)
				}
			}
		}
	}

	// 4. Allocate register for the return value (not managed by callGroup since it's the result)
	resultReg := c.regAlloc.Alloc()

	// 5. Emit OpCall (regular function call)
	c.emitCall(resultReg, funcReg, byte(totalArgCount), node.Token.Line)

	// The call group will be automatically released by defer, freeing all temporary registers
	// The result of the call is now in resultReg
	return resultReg, nil
}

func (c *Compiler) compileIfExpression(node *parser.IfExpression, hint Register) (Register, errors.PaseratiError) {
	// 1. Compile the condition
	conditionReg, err := c.compileNode(node.Condition, NoHint)
	if err != nil {
		return BadRegister, err
	}

	// 2. Emit placeholder jump for false condition
	jumpIfFalsePos := c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)

	// 3. Compile the consequence block
	// TODO: Handle block scope if needed later
	_, err = c.compileNode(node.Consequence, NoHint)
	if err != nil {
		return BadRegister, err
	}
	// TODO: How does an if-expr produce a value? Need convention.
	// Does the last expr statement value remain in a register?

	if node.Alternative != nil {
		// 4a. If there's an else, emit placeholder jump over the else block
		jumpElsePos := c.emitPlaceholderJump(vm.OpJump, 0, node.Consequence.Token.Line) // Use line of opening brace? Or token after consequence?

		// 5a. Backpatch the OpJumpIfFalse to jump *here* (start of else)
		c.patchJump(jumpIfFalsePos)

		// 6a. Compile the alternative block
		_, err = c.compileNode(node.Alternative, NoHint)
		if err != nil {
			return BadRegister, err
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
	return BadRegister, nil
}

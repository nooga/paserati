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
	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. For OpNew, we need constructor + arguments in contiguous registers
	// Allocate a contiguous block: [constructor, arg1, arg2, ...]
	argCount := len(node.Arguments)
	totalRegs := 1 + argCount // constructor + arguments
	
	constructorReg := c.regAlloc.AllocContiguous(totalRegs)
	// Add all registers to tempRegs for cleanup
	for i := 0; i < totalRegs; i++ {
		tempRegs = append(tempRegs, constructorReg+Register(i))
	}
	
	// Compile constructor into first register
	_, err := c.compileNode(node.Constructor, constructorReg)
	if err != nil {
		return BadRegister, err
	}
	
	// Compile arguments into subsequent registers
	for i, arg := range node.Arguments {
		argReg := constructorReg + 1 + Register(i)
		_, err := c.compileNode(arg, argReg)
		if err != nil {
			return BadRegister, err
		}
	}

	// Arguments are already in correct positions - no moves needed!

	// 4. Emit OpNew (constructor call) using hint as result register
	c.emitNew(hint, constructorReg, byte(argCount), node.Token.Line)

	return hint, nil
}

// compileMemberExpression compiles expressions like obj.prop or obj['prop'] (latter is future work)
func (c *Compiler) compileMemberExpression(node *parser.MemberExpression, hint Register) (Register, errors.PaseratiError) {
	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Compile the object part
	objectReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, objectReg)
	_, err := c.compileNode(node.Object, objectReg)
	if err != nil {
		return BadRegister, NewCompileError(node.Object, "error compiling object part of member expression").CausedBy(err)
	}

	// 2. Get Property Name (Assume Identifier for now: obj.prop)
	propIdent := node.Property
	propertyName := propIdent.Value

	// 3. <<< NEW: Special case for .length >>>
	if propertyName == "length" {
		// Check the static type provided by the checker
		objectStaticType := node.Object.GetComputedType()
		if objectStaticType == nil {
			// This can happen in finally blocks where type information may not be fully tracked
			// Fall through to generic OpGetProp instead of erroring
			debugPrintf("// DEBUG CompileMember: .length requested but object type is nil. Falling through to OpGetProp.\n")
		} else {

		// Widen the type to handle cases like `string | null` having `.length`
		widenedObjectType := types.GetWidenedType(objectStaticType)

		// Check if the widened type supports .length
		_, isArray := widenedObjectType.(*types.ArrayType)
		if isArray || widenedObjectType == types.String {
			// Emit specialized OpGetLength using hint as destination
			c.emitGetLength(hint, objectReg, node.Token.Line)
			return hint, nil // Handled by OpGetLength
		}
		// If type doesn't support .length, fall through to generic OpGetProp
		// The type checker *should* have caught this, but OpGetProp will likely return undefined/error at runtime.
		debugPrintf("// DEBUG CompileMember: .length requested on non-array/string type %s (widened from %s). Falling through to OpGetProp.\n",
			widenedObjectType.String(), objectStaticType.String())
		}
	}
	// --- END Special case for .length ---

	// 4. Add property name string to constant pool (for generic OpGetProp)
	nameConstIdx := c.chunk.AddConstant(vm.String(propertyName))

	// 5. Emit OpGetProp using hint as destination register
	c.emitGetProp(hint, objectReg, nameConstIdx, node.Token.Line) // Use '.' token line

	return hint, nil
}

// compileOptionalChainingExpression compiles optional chaining property access (e.g., obj?.prop)
// This is similar to compileMemberExpression but adds null/undefined checks
func (c *Compiler) compileOptionalChainingExpression(node *parser.OptionalChainingExpression, hint Register) (Register, errors.PaseratiError) {
	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Compile the object part
	objectReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, objectReg)
	_, err := c.compileNode(node.Object, objectReg)
	if err != nil {
		return BadRegister, NewCompileError(node.Object, "error compiling object part of optional chaining expression").CausedBy(err)
	}

	// 2. Check if the object is null or undefined
	// If so, return undefined immediately

	// Use efficient nullish check opcode
	isNullishReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, isNullishReg)
	c.emitIsNullish(isNullishReg, objectReg, node.Token.Line)

	// If object is NOT nullish, jump to normal property access
	jumpToPropertyAccessPos := c.emitPlaceholderJump(vm.OpJumpIfFalse, isNullishReg, node.Token.Line)

	// Object IS nullish - return undefined using hint register
	c.emitLoadUndefined(hint, node.Token.Line)
	endJumpPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)

	// Object is not null/undefined - do normal property access
	c.patchJump(jumpToPropertyAccessPos)

	// 3. Get Property Name
	propertyName := node.Property.Value

	// 4. Special case for .length (same as regular member access)
	if propertyName == "length" {
		objectStaticType := node.Object.GetComputedType()
		if objectStaticType != nil {
			widenedObjectType := types.GetWidenedType(objectStaticType)
			_, isArray := widenedObjectType.(*types.ArrayType)
			if isArray || widenedObjectType == types.String {
				c.emitGetLength(hint, objectReg, node.Token.Line)
				c.patchJump(endJumpPos)
				return hint, nil
			}
		}
	}

	// 5. Add property name string to constant pool and emit OpGetProp
	nameConstIdx := c.chunk.AddConstant(vm.String(propertyName))
	c.emitGetProp(hint, objectReg, nameConstIdx, node.Token.Line)

	// 6. Patch the end jump
	c.patchJump(endJumpPos)

	return hint, nil
}

func (c *Compiler) compileIndexExpression(node *parser.IndexExpression, hint Register) (Register, errors.PaseratiError) {
	line := parser.GetTokenFromNode(node).Line                           // Use '[' token line
	debugPrintf(">>> Enter compileIndexExpression: %s\n", node.String()) // <<< DEBUG ENTRY

	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Compile the expression being indexed (the base: array/object/string)
	debugPrintf("--- Compiling Base: %s\n", node.Left.String()) // <<< DEBUG
	arrayReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, arrayReg)
	_, err := c.compileNode(node.Left, arrayReg)
	if err != nil {
		debugPrintf("<<< Exit compileIndexExpression (Error compiling base)\n") // <<< DEBUG EXIT
		return BadRegister, NewCompileError(node.Left, "error compiling base of index expression").CausedBy(err)
	}

	// 2. Compile the index expression
	debugPrintf("--- Compiling Index: %s\n", node.Index.String()) // <<< DEBUG
	indexReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, indexReg)
	_, err = c.compileNode(node.Index, indexReg)
	if err != nil {
		debugPrintf("<<< Exit compileIndexExpression (Error compiling index)\n") // <<< DEBUG EXIT
		return BadRegister, NewCompileError(node.Index, "error compiling index part of index expression").CausedBy(err)
	}
	// <<< DEBUG INDEX RESULT >>>
	debugPrintf("--- Index Compiled. indexReg: %s\n", indexReg)

	debugPrintf("--- Using hint register = %s\n", hint) // <<< DEBUG DEST REG

	// 3. Emit OpGetIndex using hint as destination
	debugPrintf("--- Emitting OpGetIndex %s, %s, %s (Dest, Base, Index)\n", hint, arrayReg, indexReg) // <<< DEBUG EMIT
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(hint))
	c.emitByte(byte(arrayReg))
	c.emitByte(byte(indexReg))

	debugPrintf("<<< Exit compileIndexExpression (Success)\n") // <<< DEBUG EXIT
	return hint, nil
}

func (c *Compiler) compileUpdateExpression(node *parser.UpdateExpression, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line

	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

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
			tempRegs = append(tempRegs, currentValueReg)
			c.emitGetGlobal(currentValueReg, symbolRef.GlobalIndex, line)
		} else if definingTable == c.currentSymbolTable {
			// Local variable (including variables from immediate parent scope in same function)
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
			tempRegs = append(tempRegs, currentValueReg)
			c.emitOpCode(vm.OpLoadFree, line)
			c.emitByte(byte(currentValueReg))
			c.emitByte(identInfo.upvalueIndex)
		}

	case *parser.MemberExpression:
		lvalueKind = lvalueMemberExpr
		// Compile the object part
		objectReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, objectReg)
		_, err := c.compileNode(argNode.Object, objectReg)
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
		tempRegs = append(tempRegs, currentValueReg)
		c.emitGetProp(currentValueReg, memberInfo.objectReg, memberInfo.nameConstIdx, line)

	case *parser.IndexExpression:
		lvalueKind = lvalueIndexExpr
		// Compile array expression
		arrayReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, arrayReg)
		_, err := c.compileNode(argNode.Left, arrayReg)
		if err != nil {
			return BadRegister, NewCompileError(argNode.Left, "error compiling array part of index expression").CausedBy(err)
		}
		indexInfo.arrayReg = arrayReg

		// Compile index expression
		indexReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, indexReg)
		_, err = c.compileNode(argNode.Index, indexReg)
		if err != nil {
			return BadRegister, NewCompileError(argNode.Index, "error compiling index part of index expression").CausedBy(err)
		}
		indexInfo.indexReg = indexReg

		// Load the current value at the index
		currentValueReg = c.regAlloc.Alloc()
		tempRegs = append(tempRegs, currentValueReg)
		c.emitOpCode(vm.OpGetIndex, line)
		c.emitByte(byte(currentValueReg))
		c.emitByte(byte(indexInfo.arrayReg))
		c.emitByte(byte(indexInfo.indexReg))

	default:
		return BadRegister, NewCompileError(node, fmt.Sprintf("invalid target for %s: expected identifier, member expression, or index expression, got %T", node.Operator, node.Argument))
	}

	// 2. Load constant 1
	constOneReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, constOneReg)
	constOneIdx := c.chunk.AddConstant(vm.Number(1))
	c.emitLoadConstant(constOneReg, constOneIdx, line)

	// 3. Perform Pre/Post logic using hint as result register
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
		// c. Result of expression is the *new* value - move to hint
		c.emitMove(hint, currentValueReg, line)

	} else {
		// Postfix (x++ or x--):
		// a. Save original value: hint = currentValueReg
		c.emitMove(hint, currentValueReg, line)
		// b. Operate: currentValueReg = currentValueReg +/- constOneReg
		switch node.Operator {
		case "++":
			c.emitAdd(currentValueReg, currentValueReg, constOneReg, line)
		case "--":
			c.emitSubtract(currentValueReg, currentValueReg, constOneReg, line)
		}
		// c. Store back to lvalue
		c.storeToLvalue(int(lvalueKind), identInfo, memberInfo, indexInfo, currentValueReg, line)
		// d. Result of expression is the *original* value (already in hint)
	}

	return hint, nil
}

// compileTernaryExpression compiles condition ? consequence : alternative
func (c *Compiler) compileTernaryExpression(node *parser.TernaryExpression, hint Register) (Register, errors.PaseratiError) {
	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Compile condition
	conditionReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, conditionReg)
	_, err := c.compileNode(node.Condition, conditionReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. Jump if false
	jumpFalsePos := c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)

	// --- Consequence Path ---
	// 3. Compile consequence directly to hint
	_, err = c.compileNode(node.Consequence, hint)
	if err != nil {
		return BadRegister, err
	}

	// 4. Jump over alternative
	jumpEndPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)

	// --- Alternative Path ---
	// 5. Patch jumpFalse
	c.patchJump(jumpFalsePos)

	// 6. Compile alternative directly to hint (overwrites consequence result)
	_, err = c.compileNode(node.Alternative, hint)
	if err != nil {
		return BadRegister, err
	}

	// --- End ---
	// 7. Patch jumpEnd
	c.patchJump(jumpEndPos)

	// Regardless of path, hint now holds the correct value
	return hint, nil
}

func (c *Compiler) compileInfixExpression(node *parser.InfixExpression, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line // Use operator token line number

	// --- Standard binary operators (arithmetic, comparison, bitwise, shift) ---
	if node.Operator != "&&" && node.Operator != "||" && node.Operator != "??" {
		// Manage temporary registers with automatic cleanup
		var tempRegs []Register
		defer func() {
			for _, reg := range tempRegs {
				c.regAlloc.Free(reg)
			}
		}()

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
					valueReg = c.regAlloc.Alloc()
					tempRegs = append(tempRegs, valueReg)
					_, err = c.compileNode(node.Right, valueReg)
					isNullCheck = leftIsNull
				} else {
					// Compile the non-literal side (left)
					valueReg = c.regAlloc.Alloc()
					tempRegs = append(tempRegs, valueReg)
					_, err = c.compileNode(node.Left, valueReg)
					isNullCheck = rightIsNull
				}

				if err != nil {
					return BadRegister, err
				}

				if isNullCheck {
					c.emitIsNull(hint, valueReg, line)
				} else {
					c.emitIsUndefined(hint, valueReg, line)
				}

				// If this is !== (strict not equal), we need to negate the result
				if node.Operator == "!==" {
					c.emitNot(hint, hint, line)
				}

				return hint, nil
			}
		}

		leftReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, leftReg)
		_, err := c.compileNode(node.Left, leftReg)
		if err != nil {
			return BadRegister, err
		}

		rightReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, rightReg)
		_, err = c.compileNode(node.Right, rightReg)
		if err != nil {
			return BadRegister, err
		}

		switch node.Operator {
		// Arithmetic
		case "+":
			c.emitAdd(hint, leftReg, rightReg, line)
		case "-":
			c.emitSubtract(hint, leftReg, rightReg, line)
		case "*":
			c.emitMultiply(hint, leftReg, rightReg, line)
		case "/":
			c.emitDivide(hint, leftReg, rightReg, line)
		case "%":
			c.emitRemainder(hint, leftReg, rightReg, line)
		case "**":
			c.emitExponent(hint, leftReg, rightReg, line)

		// Comparison
		case "<=":
			c.emitLessEqual(hint, leftReg, rightReg, line)
		case ">=":
			// Implement as !(left < right)
			tempReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, tempReg)
			c.emitLess(tempReg, leftReg, rightReg, line)
			c.emitNot(hint, tempReg, line)
		case "==":
			c.emitEqual(hint, leftReg, rightReg, line)
		case "!=":
			c.emitNotEqual(hint, leftReg, rightReg, line)
		case "<":
			c.emitLess(hint, leftReg, rightReg, line)
		case ">":
			c.emitGreater(hint, leftReg, rightReg, line)
		case "in":
			c.emitIn(hint, leftReg, rightReg, line)
		case "instanceof":
			c.emitInstanceof(hint, leftReg, rightReg, line)
		case "===":
			c.emitStrictEqual(hint, leftReg, rightReg, line)
		case "!==":
			c.emitStrictNotEqual(hint, leftReg, rightReg, line)

		// --- NEW: Bitwise & Shift ---
		case "&":
			c.emitBitwiseAnd(hint, leftReg, rightReg, line)
		case "|":
			c.emitBitwiseOr(hint, leftReg, rightReg, line)
		case "^":
			c.emitBitwiseXor(hint, leftReg, rightReg, line)
		case "<<":
			c.emitShiftLeft(hint, leftReg, rightReg, line)
		case ">>":
			c.emitShiftRight(hint, leftReg, rightReg, line)
		case ">>>":
			c.emitUnsignedShiftRight(hint, leftReg, rightReg, line)
		// --- END NEW ---

		default:
			return BadRegister, NewCompileError(node, fmt.Sprintf("unknown standard infix operator '%s'", node.Operator))
		}

		return hint, nil
	}

	// --- Logical Operators (&&, ||, ??) with Short-Circuiting ---
	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	if node.Operator == "||" { // a || b
		leftReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, leftReg)
		_, err := c.compileNode(node.Left, leftReg)
		if err != nil {
			return BadRegister, err
		}

		// Jump to right eval if left is FALSEY
		jumpToRightPlaceholder := c.emitPlaceholderJump(vm.OpJumpIfFalse, leftReg, line)

		// If left was TRUTHY: result is left, move to hint and jump to end
		c.emitMove(hint, leftReg, line)
		jumpToEndPlaceholder := c.emitPlaceholderJump(vm.OpJump, 0, line)

		// Patch jumpToRightPlaceholder to land here (start of right operand eval)
		c.patchJump(jumpToRightPlaceholder)

		// Compile right operand directly to hint (only executed if left was falsey)
		_, err = c.compileNode(node.Right, hint)
		if err != nil {
			return BadRegister, err
		}

		// Patch jumpToEndPlaceholder to land here
		c.patchJump(jumpToEndPlaceholder)
		return hint, nil

	} else if node.Operator == "&&" { // a && b
		leftReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, leftReg)
		_, err := c.compileNode(node.Left, leftReg)
		if err != nil {
			return BadRegister, err
		}

		// If left is FALSEY, jump to end, result is left
		jumpToEndPlaceholder := c.emitPlaceholderJump(vm.OpJumpIfFalse, leftReg, line)

		// If left was TRUTHY (didn't jump), compile right operand directly to hint
		_, err = c.compileNode(node.Right, hint)
		if err != nil {
			return BadRegister, err
		}
		// Jump over the false path's move
		jumpSkipFalseMovePlaceholder := c.emitPlaceholderJump(vm.OpJump, 0, line)
		// Patch jumpToEndPlaceholder to land here (false path)
		c.patchJump(jumpToEndPlaceholder)
		// Result is left, move leftReg to hint
		c.emitMove(hint, leftReg, line)

		// Patch the skip jump
		c.patchJump(jumpSkipFalseMovePlaceholder)

		return hint, nil

	} else if node.Operator == "??" { // a ?? b
		leftReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, leftReg)
		_, err := c.compileNode(node.Left, leftReg)
		if err != nil {
			return BadRegister, err
		}

		// Use efficient nullish check opcode
		isNullishReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, isNullishReg)
		c.emitIsNullish(isNullishReg, leftReg, line)

		// Jump if *NOT* nullish (jump if false) to skip the right side eval
		jumpSkipRightPlaceholder := c.emitPlaceholderJump(vm.OpJumpIfFalse, isNullishReg, line)

		// --- Eval Right Path ---
		// Compile right operand directly to hint (only executed if left was nullish)
		_, err = c.compileNode(node.Right, hint)
		if err != nil {
			return BadRegister, err
		}

		// Jump over the skip-right landing pad
		jumpEndPlaceholder := c.emitPlaceholderJump(vm.OpJump, 0, line)

		// --- Skip Right Path ---
		// Land here if left was NOT nullish. Patch the jump from the nullish check.
		c.patchJump(jumpSkipRightPlaceholder)
		// Result is left (not nullish), move it to hint.
		c.emitMove(hint, leftReg, line)

		// Land here after either path finishes. Patch the jump from the right-eval path.
		c.patchJump(jumpEndPlaceholder)

		return hint, nil
	}

	return BadRegister, NewCompileError(node, fmt.Sprintf("logical/coalescing operator '%s' compilation fell through", node.Operator))
}

func (c *Compiler) compilePrefixExpression(node *parser.PrefixExpression, hint Register) (Register, errors.PaseratiError) {
	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// Compile the right operand first
	rightReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, rightReg)
	_, err := c.compileNode(node.Right, rightReg)
	if err != nil {
		return BadRegister, err
	}

	// Emit the corresponding unary opcode using hint as destination
	switch node.Operator {
	case "!":
		c.emitNot(hint, rightReg, node.Token.Line)
	case "-":
		c.emitNegate(hint, rightReg, node.Token.Line)
	// --- NEW: Handle Unary Plus (+) ---
	case "+":
		// Unary plus converts operand to number using the dedicated OpToNumber instruction
		c.emitToNumber(hint, rightReg, node.Token.Line)
	// --- NEW: Handle Void ---
	case "void":
		// void operator evaluates operand (for side effects) then returns undefined
		// Operand is already compiled above (rightReg), so we just load undefined
		c.emitLoadUndefined(hint, node.Token.Line)
	// --- NEW ---
	case "~":
		c.emitBitwiseNot(hint, rightReg, node.Token.Line)
	// --- NEW: Handle delete operator ---
	case "delete":
		// delete operator requires special handling based on the operand type
		switch operand := node.Right.(type) {
		case *parser.MemberExpression:
			// delete obj.prop
			objReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, objReg)
			_, err := c.compileNode(operand.Object, objReg)
			if err != nil {
				return BadRegister, err
			}
			// Get property name and add to constant pool
			propName := operand.Property.Value
			propIdx := c.chunk.AddConstant(vm.String(propName))
			c.emitDeleteProp(hint, objReg, propIdx, node.Token.Line)
			
		case *parser.IndexExpression:
			// delete obj[key]
			// For now, we'll handle this similarly to member expressions if the key is a string literal
			// Full dynamic property deletion would require a different opcode
			if strLit, ok := operand.Index.(*parser.StringLiteral); ok {
				objReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, objReg)
				_, err := c.compileNode(operand.Left, objReg)
				if err != nil {
					return BadRegister, err
				}
				propIdx := c.chunk.AddConstant(vm.String(strLit.Value))
				c.emitDeleteProp(hint, objReg, propIdx, node.Token.Line)
			} else {
				// For dynamic keys, we'd need a different approach
				// For now, compile error
				return BadRegister, NewCompileError(node, "delete with dynamic property keys not yet supported")
			}
			
		default:
			// For other expressions (like identifiers), the type checker should have caught this
			// But let's return a compile error just in case
			return BadRegister, NewCompileError(node, fmt.Sprintf("cannot delete %T", node.Right))
		}
	// --- END NEW ---
	default:
		return BadRegister, NewCompileError(node, fmt.Sprintf("unknown prefix operator '%s'", node.Operator))
	}

	return hint, nil
}

func (c *Compiler) compileTypeofExpression(node *parser.TypeofExpression, hint Register) (Register, errors.PaseratiError) {
	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// Compile the operand being typeof
	exprReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, exprReg)
	_, err := c.compileNode(node.Operand, exprReg)
	if err != nil {
		return BadRegister, err
	}

	// Emit the OpTypeof instruction using hint as destination
	c.emitTypeof(hint, exprReg, node.Token.Line)

	return hint, nil
}

// compileTypeAssertionExpression compiles type assertion expressions (value as Type)
// At runtime, type assertions are essentially no-ops since TypeScript type checking
// has already validated them at compile time.
func (c *Compiler) compileTypeAssertionExpression(node *parser.TypeAssertionExpression, hint Register) (Register, errors.PaseratiError) {
	// For type assertions, we simply compile the underlying expression
	// The type checking has already been done by the checker, so at runtime
	// this is just the value itself
	return c.compileNode(node.Expression, hint)
}

// calculateEffectiveArgCount calculates the effective number of arguments,
// expanding spread elements based on array literal lengths
func (c *Compiler) calculateEffectiveArgCount(arguments []parser.Expression) int {
	count := 0
	for _, arg := range arguments {
		if spreadElement, isSpread := arg.(*parser.SpreadElement); isSpread {
			// For direct array literals, count the elements
			if arrayLit, isArrayLit := spreadElement.Argument.(*parser.ArrayLiteral); isArrayLit {
				count += len(arrayLit.Elements)
			} else {
				// For non-literal arrays, we can't determine the exact count at compile time
				// This case should be handled by the type checker, but add 1 for error recovery
				count += 1
			}
		} else {
			// Regular argument
			count += 1
		}
	}
	return count
}

// hasSpreadArgument checks if any argument is a spread element
func (c *Compiler) hasSpreadArgument(arguments []parser.Expression) bool {
	for _, arg := range arguments {
		if _, isSpread := arg.(*parser.SpreadElement); isSpread {
			return true
		}
	}
	return false
}

// Helper function to determine total argument count including optional parameters
func (c *Compiler) determineTotalArgCount(node *parser.CallExpression) int {
	// Calculate effective argument count, expanding spread elements
	providedArgCount := c.calculateEffectiveArgCount(node.Arguments)

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

	return finalArgCount
}

func (c *Compiler) compileCallExpression(node *parser.CallExpression, hint Register) (Register, errors.PaseratiError) {
	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// Check if any argument uses spread syntax
	hasSpread := c.hasSpreadArgument(node.Arguments)
	if hasSpread {
		return c.compileSpreadCallExpression(node, hint, &tempRegs)
	}

	// Check if this is a method call (function is a member expression like obj.method())
	if memberExpr, isMethodCall := node.Function.(*parser.MemberExpression); isMethodCall {
		// Method call: obj.method(args...)
		// 1. Compile the object part (this value)
		thisReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, thisReg)
		_, err := c.compileNode(memberExpr.Object, thisReg)
		if err != nil {
			return BadRegister, err
		}

		// 2. Allocate contiguous block for function + all arguments (including optional parameters)
		totalArgCount := c.determineTotalArgCount(node)
		blockSize := 1 + totalArgCount // funcReg + arguments
		funcReg := c.regAlloc.AllocContiguous(blockSize)
		// Mark the entire block for cleanup
		for i := 0; i < blockSize; i++ {
			tempRegs = append(tempRegs, funcReg+Register(i))
		}

		// 3. OPTIMIZATION: Reuse thisReg for getting the property instead of compiling the object again
		propertyName := memberExpr.Property.Value
		nameConstIdx := c.chunk.AddConstant(vm.String(propertyName))
		c.emitGetProp(funcReg, thisReg, nameConstIdx, memberExpr.Token.Line)

		// 4. Compile arguments directly into their target positions (funcReg+1, funcReg+2, ...)
		_, actualArgCount, err := c.compileArgumentsWithOptionalHandling(node, funcReg+1)
		if err != nil {
			return BadRegister, err
		}

		// 5. Emit OpCallMethod using hint as result register
		c.emitCallMethod(hint, funcReg, thisReg, byte(actualArgCount), node.Token.Line)

		return hint, nil
	}

	// --- Regular function call ---
	// 1. Allocate contiguous block for function + all arguments (including optional parameters)
	totalArgCount := c.determineTotalArgCount(node)
	blockSize := 1 + totalArgCount // funcReg + arguments
	funcReg := c.regAlloc.AllocContiguous(blockSize)
	// Mark the entire block for cleanup
	for i := 0; i < blockSize; i++ {
		tempRegs = append(tempRegs, funcReg+Register(i))
	}

	// 2. Compile the expression being called (e.g., function name)
	_, err := c.compileNode(node.Function, funcReg)
	if err != nil {
		return BadRegister, err
	}

	// 3. Compile arguments directly into their target positions (funcReg+1, funcReg+2, ...)
	_, actualArgCount, err := c.compileArgumentsWithOptionalHandling(node, funcReg+1)
	if err != nil {
		return BadRegister, err
	}

	// Arguments are already in correct positions - no moves needed!

	// 4. Emit OpCall using hint as result register
	c.emitCall(hint, funcReg, byte(actualArgCount), node.Token.Line)

	return hint, nil
}

// compileSpreadCallExpression handles function calls that contain spread syntax
func (c *Compiler) compileSpreadCallExpression(node *parser.CallExpression, hint Register, tempRegs *[]Register) (Register, errors.PaseratiError) {
	// For now, only support calls with a single spread argument (the most common case)
	if len(node.Arguments) != 1 {
		return BadRegister, NewCompileError(node, "spread calls currently only support a single spread argument")
	}
	
	spreadElement, isSpread := node.Arguments[0].(*parser.SpreadElement)
	if !isSpread {
		return BadRegister, NewCompileError(node, "expected spread argument")
	}
	
	// Check if this is a method call
	if memberExpr, isMethodCall := node.Function.(*parser.MemberExpression); isMethodCall {
		// Method call with spread: obj.method(...args)
		
		// 1. Compile the object part (this value)
		thisReg := c.regAlloc.Alloc()
		*tempRegs = append(*tempRegs, thisReg)
		_, err := c.compileNode(memberExpr.Object, thisReg)
		if err != nil {
			return BadRegister, err
		}
		
		// 2. Compile the method function
		funcReg := c.regAlloc.Alloc()
		*tempRegs = append(*tempRegs, funcReg)
		propertyName := memberExpr.Property.Value
		nameConstIdx := c.chunk.AddConstant(vm.String(propertyName))
		c.emitGetProp(funcReg, thisReg, nameConstIdx, memberExpr.Token.Line)
		
		// 3. Compile the spread argument (array to spread)
		spreadArgReg := c.regAlloc.Alloc()
		*tempRegs = append(*tempRegs, spreadArgReg)
		_, err = c.compileNode(spreadElement.Argument, spreadArgReg)
		if err != nil {
			return BadRegister, err
		}
		
		// 4. Emit OpSpreadCallMethod
		c.emitSpreadCallMethod(hint, funcReg, thisReg, spreadArgReg, node.Token.Line)
		
		return hint, nil
	} else {
		// Regular function call with spread: func(...args)
		
		// 1. Compile the function
		funcReg := c.regAlloc.Alloc()
		*tempRegs = append(*tempRegs, funcReg)
		_, err := c.compileNode(node.Function, funcReg)
		if err != nil {
			return BadRegister, err
		}
		
		// 2. Compile the spread argument (array to spread)
		spreadArgReg := c.regAlloc.Alloc()
		*tempRegs = append(*tempRegs, spreadArgReg)
		_, err = c.compileNode(spreadElement.Argument, spreadArgReg)
		if err != nil {
			return BadRegister, err
		}
		
		// 3. Emit OpSpreadCall
		c.emitSpreadCall(hint, funcReg, spreadArgReg, node.Token.Line)
		
		return hint, nil
	}
}

func (c *Compiler) compileIfExpression(node *parser.IfExpression, hint Register) (Register, errors.PaseratiError) {
	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Compile the condition
	conditionReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, conditionReg)
	_, err := c.compileNode(node.Condition, conditionReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. Emit placeholder jump for false condition
	jumpIfFalsePos := c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)

	// 3. Compile the consequence block
	// Allocate temporary register for consequence compilation
	consequenceReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, consequenceReg)
	_, err = c.compileNode(node.Consequence, consequenceReg)
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
		// Allocate temporary register for alternative compilation
		alternativeReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, alternativeReg)
		_, err = c.compileNode(node.Alternative, alternativeReg)
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

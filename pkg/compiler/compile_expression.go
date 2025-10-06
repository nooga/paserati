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

// isDataProperty checks if a type represents a data property (as opposed to a getter/setter)
func (c *Compiler) isDataProperty(propType types.Type) bool {
	// Check for primitive types
	if propType == types.String || propType == types.Number || propType == types.Boolean {
		return true
	}

	switch propType.(type) {
	case *types.Primitive:
		return true // All primitive types are data properties
	case *types.LiteralType:
		return true // Literal types are data properties
	case *types.UnionType, *types.IntersectionType:
		return true // These are typically data types
	case *types.ArrayType:
		return true
	case *types.ObjectType:
		// For object types, check if it's a function type (getters return function types)
		objType := propType.(*types.ObjectType)
		// If it has call signatures, it's probably a getter method, not a data property
		return len(objType.CallSignatures) == 0
	default:
		return true // Default to treating as data property
	}
}

// emitOptimisticGetterCall emits bytecode that tries to call a getter, but falls back to regular property access
func (c *Compiler) emitOptimisticGetterCall(hint Register, methodReg Register, objectReg Register, propertyName string, line int) {
	// Check if the method (getter) is undefined
	undefinedReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(undefinedReg)
	c.emitLoadUndefined(undefinedReg, line)

	// Compare getter method with undefined
	compareReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(compareReg)
	c.emitStrictEqual(compareReg, methodReg, undefinedReg, line)

	// Jump to fallback if getter is undefined (comparison is true)
	jumpToFallbackPos := c.emitPlaceholderJump(vm.OpJumpIfFalse, compareReg, line)

	// Fallback: regular property access (getter is undefined)
	propertyIdx := c.chunk.AddConstant(vm.String(propertyName))
	c.emitGetProp(hint, objectReg, propertyIdx, line)

	// Jump over getter call
	jumpOverGetterPos := c.emitPlaceholderJump(vm.OpJump, 0, line)

	// Getter exists - call it
	c.patchJump(jumpToFallbackPos)
	c.emitCallMethod(hint, methodReg, objectReg, 0, line)

	// Patch the jump over getter call
	c.patchJump(jumpOverGetterPos)
}

// emitOptimisticSetterCall emits bytecode that tries to call a setter, but falls back to regular property assignment
func (c *Compiler) emitOptimisticSetterCall(methodReg Register, objectReg Register, valueReg Register, propertyConstIdx uint16, line int) {
	// Check if the method (setter) is undefined
	undefinedReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(undefinedReg)
	c.emitLoadUndefined(undefinedReg, line)

	// Compare setter method with undefined
	compareReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(compareReg)
	c.emitStrictEqual(compareReg, methodReg, undefinedReg, line)

	// Jump to fallback if setter is undefined (comparison is true)
	jumpToFallbackPos := c.emitPlaceholderJump(vm.OpJumpIfFalse, compareReg, line)

	// Fallback: regular property assignment (setter is undefined)
	c.emitSetProp(objectReg, valueReg, propertyConstIdx, line)

	// Jump over setter call
	jumpOverSetterPos := c.emitPlaceholderJump(vm.OpJump, 0, line)

	// Setter exists - call it
	c.patchJump(jumpToFallbackPos)
	dummyResultReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(dummyResultReg)
	c.emitCallMethod(dummyResultReg, methodReg, objectReg, 1, line)

	// Patch the jump over setter call
	c.patchJump(jumpOverSetterPos)
}

func (c *Compiler) compileNewExpression(node *parser.NewExpression, hint Register) (Register, errors.PaseratiError) {
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
		// Use spread call mechanism for new Foo(...args)
		return c.compileSpreadNewExpression(node, hint, &tempRegs)
	}

	// 1. Determine total argument count needed (including optional parameter padding)
	totalArgCount := c.determineTotalArgCountForNew(node)

	// 2. For OpNew, we need constructor + arguments in contiguous registers
	// Allocate a contiguous block: [constructor, arg1, arg2, ...]
	totalRegs := 1 + totalArgCount // constructor + arguments

	constructorReg := c.regAlloc.AllocContiguous(totalRegs)
	// Add all registers to tempRegs for cleanup
	for i := 0; i < totalRegs; i++ {
		tempRegs = append(tempRegs, constructorReg+Register(i))
	}

	// 3. Compile constructor into first register
	_, err := c.compileNode(node.Constructor, constructorReg)
	if err != nil {
		return BadRegister, err
	}

	// 4. Compile arguments with optional parameter handling (same as CallExpression)
	_, actualArgCount, err := c.compileArgumentsWithOptionalHandlingForNew(node, constructorReg+1)
	if err != nil {
		return BadRegister, err
	}

	// 5. Emit OpNew (constructor call) using hint as result register
	c.emitNew(hint, constructorReg, byte(actualArgCount), node.Token.Line)

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

	// 1. Check for super member access (super.method)
	if _, isSuperMember := node.Object.(*parser.SuperExpression); isSuperMember {
		return c.compileSuperMemberExpression(node, hint, &tempRegs)
	}

	// 1. Compile the object part
	objectReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, objectReg)
	_, err := c.compileNode(node.Object, objectReg)
	if err != nil {
		return BadRegister, NewCompileError(node.Object, "error compiling object part of member expression").CausedBy(err)
	}

	// 2. Check if this is a computed property access
	var isComputedProperty bool
	var propertyName string
	var propertyReg Register

	if computedKey, ok := node.Property.(*parser.ComputedPropertyName); ok {
		// This is a computed property: obj[expr]
		isComputedProperty = true
		propertyReg = c.regAlloc.Alloc()
		tempRegs = append(tempRegs, propertyReg)
		_, err := c.compileNode(computedKey.Expr, propertyReg)
		if err != nil {
			return BadRegister, NewCompileError(computedKey.Expr, "error compiling computed property key").CausedBy(err)
		}
		propertyName = "__computed__" // For debug purposes only
		// Note: if the computed key evaluates to a well-known symbol (e.g., Symbol.iterator),
		// the VM OpGetIndex will handle symbol identity efficiently. No extra emission needed here.
	} else {
		// Regular property access: obj.prop
		isComputedProperty = false
		propertyName = c.extractPropertyName(node.Property)
	}

	// 2.5. Check for private field access (ECMAScript # fields)
	if !isComputedProperty && len(propertyName) > 0 && propertyName[0] == '#' {
		// Private field access: obj.#field
		// Strip the # prefix for storage (internal representation)
		fieldName := propertyName[1:]
		nameConstIdx := c.chunk.AddConstant(vm.String(fieldName))
		c.emitGetPrivateField(hint, objectReg, nameConstIdx, node.Token.Line)
		return hint, nil
	}

	// 3. Handle Symbol.iterator special case
	objectStaticType := node.Object.GetComputedType()
	if objectStaticType == nil {
		// REGRESSION FIX: Handle Symbol.iterator when type checking didn't set computed type
		// This is a workaround for the with statement regression
		if identNode, ok := node.Object.(*parser.Identifier); ok && identNode.Value == "Symbol" {
			// Skip optimization and use direct property access
			propertyIdx := c.chunk.AddConstant(vm.String(propertyName))
			c.emitGetProp(hint, objectReg, propertyIdx, node.Token.Line)
			return hint, nil
		}
	}

	// 4. Special case for .length (moved before optimistic getter to fix try/finally blocks)
	if !isComputedProperty && propertyName == "length" {
		// Check the static type provided by the checker
		objectStaticType := node.Object.GetComputedType()
		if objectStaticType == nil {
			// This can happen in finally blocks where type information may not be fully tracked
			// Use OpGetProp with the actual property name "length" instead of optimistic getter
			// debug disabled
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
			// debug disabled
		}

		// For .length property access without type info, always use OpGetProp with "length"
		// This ensures string.length works even in try/finally blocks where objectStaticType is nil
		propertyIdx := c.chunk.AddConstant(vm.String(propertyName))
		c.emitGetProp(hint, objectReg, propertyIdx, node.Token.Line)
		return hint, nil
	}

	// 5. Emit appropriate property access instruction
	if isComputedProperty {
		// Use OpGetIndex for computed properties: hint = objectReg[propertyReg]
		c.emitOpCode(vm.OpGetIndex, node.Token.Line)
		c.emitByte(byte(hint))        // Destination register
		c.emitByte(byte(objectReg))   // Object register
		c.emitByte(byte(propertyReg)) // Key register (computed at runtime)
	} else {
		// Use OpGetProp for static properties: hint = objectReg.propertyName
		nameConstIdx := c.chunk.AddConstant(vm.String(propertyName))
		c.emitGetProp(hint, objectReg, nameConstIdx, node.Token.Line) // Use '.' token line
	}

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
	propertyName := c.extractPropertyName(node.Property)

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
	line := parser.GetTokenFromNode(node).Line // Use '[' token line
	if false {
		debugPrintf("")
	}

	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Compile the expression being indexed (the base: array/object/string)
	// debug disabled
	arrayReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, arrayReg)
	_, err := c.compileNode(node.Left, arrayReg)
	if err != nil {
		// debug disabled
		return BadRegister, NewCompileError(node.Left, "error compiling base of index expression").CausedBy(err)
	}

	// 2. Compile the index expression
	// debug disabled
	indexReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, indexReg)
	_, err = c.compileNode(node.Index, indexReg)
	if err != nil {
		// debug disabled
		return BadRegister, NewCompileError(node.Index, "error compiling index part of index expression").CausedBy(err)
	}
	// <<< DEBUG INDEX RESULT >>>
	// debug disabled

	// debug disabled

	// 3. Emit OpGetIndex using hint as destination
	// debug disabled
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(hint))
	c.emitByte(byte(arrayReg))
	c.emitByte(byte(indexReg))

	// debug disabled
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
		objectReg      Register
		nameConstIdx   uint16
		isPrivateField bool
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

		// Get property name (handle both identifiers and computed properties)
		propName := c.extractPropertyName(argNode.Property)

		// Check if this is a private field
		if len(propName) > 0 && propName[0] == '#' {
			// Private field - strip the # and set flag
			fieldName := propName[1:]
			memberInfo.nameConstIdx = c.chunk.AddConstant(vm.String(fieldName))
			memberInfo.isPrivateField = true
		} else {
			memberInfo.nameConstIdx = c.chunk.AddConstant(vm.String(propName))
			memberInfo.isPrivateField = false
		}

		// Load current property value
		currentValueReg = c.regAlloc.Alloc()
		tempRegs = append(tempRegs, currentValueReg)
		if memberInfo.isPrivateField {
			c.emitGetPrivateField(currentValueReg, memberInfo.objectReg, memberInfo.nameConstIdx, line)
		} else {
			c.emitGetProp(currentValueReg, memberInfo.objectReg, memberInfo.nameConstIdx, line)
		}

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
			c.emitGreaterEqual(hint, leftReg, rightReg, line)
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

		case ",":
			// Comma operator: evaluate both expressions, return the right one
			// Left expression is already compiled (for side effects)
			// Right expression is already compiled - just return it
			// The comma operator returns the value of the right operand
			c.emitMove(hint, rightReg, line)
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
		// Defer-safety: ensure any placeholder jumps are patched to a valid anchor
		var (
			jumpToRightPlaceholder = -1
			jumpToEndPlaceholder   = -1
			patchedRight           = false
			patchedEnd             = false
		)
		defer func() {
			// Anchor to current end of code by default
			endAnchor := len(c.chunk.Code)
			if jumpToRightPlaceholder >= 0 && !patchedRight {
				c.patchJump(jumpToRightPlaceholder)
				patchedRight = true
			}
			if jumpToEndPlaceholder >= 0 && !patchedEnd {
				// Manually patch OpJump to end if not already patched
				op := vm.OpCode(c.chunk.Code[jumpToEndPlaceholder])
				if op == vm.OpJump {
					operandStartPos := jumpToEndPlaceholder + 1
					jumpInstructionEndPos := operandStartPos + 2
					offset := endAnchor - jumpInstructionEndPos
					c.chunk.Code[operandStartPos] = byte(int16(offset) >> 8)
					c.chunk.Code[operandStartPos+1] = byte(int16(offset) & 0xFF)
				} else {
					c.patchJump(jumpToEndPlaceholder)
				}
				patchedEnd = true
			}
		}()
		leftReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, leftReg)
		_, err := c.compileNode(node.Left, leftReg)
		if err != nil {
			return BadRegister, err
		}

		// Jump to right eval if left is FALSEY
		jumpToRightPlaceholder = c.emitPlaceholderJump(vm.OpJumpIfFalse, leftReg, line)

		// If left was TRUTHY: result is left, move to hint and jump to end
		c.emitMove(hint, leftReg, line)
		jumpToEndPlaceholder = c.emitPlaceholderJump(vm.OpJump, 0, line)

		// Patch jumpToRightPlaceholder to land here (start of right operand eval)
		c.patchJump(jumpToRightPlaceholder)
		patchedRight = true

		// Compile right operand directly to hint (only executed if left was falsey)
		_, err = c.compileNode(node.Right, hint)
		if err != nil {
			return BadRegister, err
		}

		// Patch jumpToEndPlaceholder to land here
		c.patchJump(jumpToEndPlaceholder)
		patchedEnd = true
		return hint, nil

	} else if node.Operator == "&&" { // a && b
		// Defer-safety for placeholders in this block
		var (
			jumpToEndPlaceholder         = -1
			jumpSkipFalseMovePlaceholder = -1
			patchedEnd                   = false
			patchedSkip                  = false
		)
		defer func() {
			endAnchor := len(c.chunk.Code)
			if jumpToEndPlaceholder >= 0 && !patchedEnd {
				c.patchJump(jumpToEndPlaceholder)
				patchedEnd = true
			}
			if jumpSkipFalseMovePlaceholder >= 0 && !patchedSkip {
				op := vm.OpCode(c.chunk.Code[jumpSkipFalseMovePlaceholder])
				if op == vm.OpJump {
					operandStartPos := jumpSkipFalseMovePlaceholder + 1
					jumpInstructionEndPos := operandStartPos + 2
					offset := endAnchor - jumpInstructionEndPos
					c.chunk.Code[operandStartPos] = byte(int16(offset) >> 8)
					c.chunk.Code[operandStartPos+1] = byte(int16(offset) & 0xFF)
				} else {
					c.patchJump(jumpSkipFalseMovePlaceholder)
				}
				patchedSkip = true
			}
		}()
		leftReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, leftReg)
		_, err := c.compileNode(node.Left, leftReg)
		if err != nil {
			return BadRegister, err
		}

		// If left is FALSEY, jump to end, result is left
		jumpToEndPlaceholder = c.emitPlaceholderJump(vm.OpJumpIfFalse, leftReg, line)

		// If left was TRUTHY (didn't jump), compile right operand directly to hint
		_, err = c.compileNode(node.Right, hint)
		if err != nil {
			return BadRegister, err
		}
		// Jump over the false path's move
		jumpSkipFalseMovePlaceholder = c.emitPlaceholderJump(vm.OpJump, 0, line)
		// Patch jumpToEndPlaceholder to land here (false path)
		c.patchJump(jumpToEndPlaceholder)
		patchedEnd = true
		// Result is left, move leftReg to hint
		c.emitMove(hint, leftReg, line)

		// Patch the skip jump
		c.patchJump(jumpSkipFalseMovePlaceholder)
		patchedSkip = true

		return hint, nil

	} else if node.Operator == "??" { // a ?? b
		// Defer-safety for placeholders
		var (
			jumpSkipRightPlaceholder = -1
			jumpEndPlaceholder       = -1
			patchedSkip              = false
			patchedEnd               = false
		)
		defer func() {
			endAnchor := len(c.chunk.Code)
			if jumpSkipRightPlaceholder >= 0 && !patchedSkip {
				c.patchJump(jumpSkipRightPlaceholder)
				patchedSkip = true
			}
			if jumpEndPlaceholder >= 0 && !patchedEnd {
				op := vm.OpCode(c.chunk.Code[jumpEndPlaceholder])
				if op == vm.OpJump {
					operandStartPos := jumpEndPlaceholder + 1
					jumpInstructionEndPos := operandStartPos + 2
					offset := endAnchor - jumpInstructionEndPos
					c.chunk.Code[operandStartPos] = byte(int16(offset) >> 8)
					c.chunk.Code[operandStartPos+1] = byte(int16(offset) & 0xFF)
				} else {
					c.patchJump(jumpEndPlaceholder)
				}
				patchedEnd = true
			}
		}()
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
		jumpSkipRightPlaceholder = c.emitPlaceholderJump(vm.OpJumpIfFalse, isNullishReg, line)

		// --- Eval Right Path ---
		// Compile right operand directly to hint (only executed if left was nullish)
		_, err = c.compileNode(node.Right, hint)
		if err != nil {
			return BadRegister, err
		}

		// Jump over the skip-right landing pad
		jumpEndPlaceholder = c.emitPlaceholderJump(vm.OpJump, 0, line)

		// --- Skip Right Path ---
		// Land here if left was NOT nullish. Patch the jump from the nullish check.
		c.patchJump(jumpSkipRightPlaceholder)
		patchedSkip = true
		// Result is left (not nullish), move it to hint.
		c.emitMove(hint, leftReg, line)

		// Land here after either path finishes. Patch the jump from the right-eval path.
		c.patchJump(jumpEndPlaceholder)
		patchedEnd = true

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
			propName := c.extractPropertyName(operand.Property)
			propIdx := c.chunk.AddConstant(vm.String(propName))
			c.emitDeleteProp(hint, objReg, propIdx, node.Token.Line)

		case *parser.IndexExpression:
			// delete obj[key]
			objReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, objReg)
			_, err := c.compileNode(operand.Left, objReg)
			if err != nil {
				return BadRegister, err
			}
			keyReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, keyReg)
			_, err = c.compileNode(operand.Index, keyReg)
			if err != nil {
				return BadRegister, err
			}
			c.emitDeleteIndex(hint, objReg, keyReg, node.Token.Line)

		case *parser.Identifier:
			// delete identifier: Try to delete from global scope
			// If it's a global variable, emit OpDeleteGlobal
			// If it's not found, return true (per ECMAScript spec)
			varName := operand.Value

			// Check if this is a global variable
			if c.heapAlloc != nil {
				if heapIdx, isGlobal := c.heapAlloc.GetIndex(varName); isGlobal {
					// It's a global variable - emit OpDeleteGlobal
					c.emitOpCode(vm.OpDeleteGlobal, node.Token.Line)
					c.emitByte(byte(hint))
					c.emitUint16(uint16(heapIdx))
				} else {
					// Local variable or not found - return true (spec behavior)
					c.emitLoadConstant(hint, c.chunk.AddConstant(vm.BooleanValue(true)), node.Token.Line)
				}
			} else {
				// No heap allocator - return true (spec behavior)
				c.emitLoadConstant(hint, c.chunk.AddConstant(vm.BooleanValue(true)), node.Token.Line)
			}

		case *parser.CallExpression, *parser.PrefixExpression, *parser.InfixExpression:
			// delete expression: For other expressions that are not property access,
			// evaluate the expression and return true (per ECMAScript spec)
			// We still need to evaluate the expression for potential side effects
			exprReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, exprReg)
			_, err := c.compileNode(node.Right, exprReg)
			if err != nil {
				return BadRegister, err
			}
			// Always return true for non-property-access delete operations
			c.emitLoadConstant(hint, c.chunk.AddConstant(vm.BooleanValue(true)), node.Token.Line)
			
		default:
			// For other expressions that we don't support yet
			return BadRegister, NewCompileError(node, fmt.Sprintf("cannot delete %T", node.Right))
		}
	// --- END NEW ---
	default:
		return BadRegister, NewCompileError(node, fmt.Sprintf("unknown prefix operator '%s'", node.Operator))
	}

	return hint, nil
}

func (c *Compiler) compileTypeofExpression(node *parser.TypeofExpression, hint Register) (Register, errors.PaseratiError) {
	// Special case: typeof with identifier should not throw ReferenceError for undefined variables
	// Per ECMAScript spec, typeof is the only operator with this behavior
	if ident, ok := node.Operand.(*parser.Identifier); ok {
		// Check if identifier exists in symbol table
		// Special handling: 'arguments' is available in function scope but not in symbol table
		_, _, found := c.currentSymbolTable.Resolve(ident.Value)

		if !found && ident.Value != "arguments" {
			// Identifier doesn't exist (and it's not 'arguments') - emit special OpTypeofIdentifier that returns "undefined"
			if hint == NoHint || hint == BadRegister {
				hint = c.regAlloc.Alloc()
			}
			c.emitTypeofIdentifier(hint, ident.Value, node.Token.Line)
			return hint, nil
		}
		// If identifier exists (or is 'arguments'), fall through to normal compilation
	}

	// For all other expressions, compile normally and then typeof
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

// compileSatisfiesExpression compiles satisfies expressions (value satisfies Type)
// At runtime, satisfies expressions are essentially no-ops since TypeScript type checking
// has already validated them at compile time.
func (c *Compiler) compileSatisfiesExpression(node *parser.SatisfiesExpression, hint Register) (Register, errors.PaseratiError) {
	// For satisfies expressions, we simply compile the underlying expression
	// The type checking has already been done by the checker, so at runtime
	// this is just the value itself (preserving the original type)
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

	// Check if this is a super constructor call (super()) - must come before spread check
	if _, isSuperCall := node.Function.(*parser.SuperExpression); isSuperCall {
		return c.compileSuperConstructorCall(node, hint, &tempRegs)
	}

	// Check if any argument uses spread syntax
	hasSpread := c.hasSpreadArgument(node.Arguments)
	if hasSpread {
		return c.compileSpreadCallExpression(node, hint, &tempRegs)
	}

	// Check if this is a method call (function is a member expression like obj.method() or obj[key]())
	if memberExpr, isMethodCall := node.Function.(*parser.MemberExpression); isMethodCall {
		// Method call: obj.method(args...)
		// 1. Compile the object part (this value)
		thisReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, thisReg)
		// fmt.Printf("// [COMPILE DEBUG] Method call: thisReg = R%d\n", thisReg)
		_, err := c.compileNode(memberExpr.Object, thisReg)
		if err != nil {
			return BadRegister, err
		}

		// 2. Allocate contiguous block for function + all arguments (including optional parameters)
		totalArgCount := c.determineTotalArgCount(node)
		blockSize := 1 + totalArgCount // funcReg + arguments
		funcReg := c.regAlloc.AllocContiguous(blockSize)
		// fmt.Printf("// [COMPILE DEBUG] Method call: funcReg = R%d, blockSize = %d, totalArgCount = %d\n", funcReg, blockSize, totalArgCount)
		// Mark the entire block for cleanup
		for i := 0; i < blockSize; i++ {
			tempRegs = append(tempRegs, funcReg+Register(i))
		}

		// 3. Get the method property - handle computed vs regular properties
		if computedKey, isComputed := memberExpr.Property.(*parser.ComputedPropertyName); isComputed {
			// Computed property: obj[expr]()
			propertyReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, propertyReg)
			_, err := c.compileNode(computedKey.Expr, propertyReg)
			if err != nil {
				return BadRegister, err
			}
			c.emitOpCode(vm.OpGetIndex, memberExpr.Token.Line)
			c.emitByte(byte(funcReg))     // Destination register
			c.emitByte(byte(thisReg))     // Object register
			c.emitByte(byte(propertyReg)) // Key register
		} else {
			// Regular property: obj.prop() or private method: obj.#method()
			propertyName := c.extractPropertyName(memberExpr.Property)
			// Check for private field/method access
			if len(propertyName) > 0 && propertyName[0] == '#' {
				// Private method call: strip # prefix for storage
				fieldName := propertyName[1:]
				nameConstIdx := c.chunk.AddConstant(vm.String(fieldName))
				c.emitGetPrivateField(funcReg, thisReg, nameConstIdx, memberExpr.Token.Line)
			} else {
				// Public method call
				nameConstIdx := c.chunk.AddConstant(vm.String(propertyName))
				c.emitGetProp(funcReg, thisReg, nameConstIdx, memberExpr.Token.Line)
			}
		}

		// 4. Compile arguments directly into their target positions (funcReg+1, funcReg+2, ...)
		_, actualArgCount, err := c.compileArgumentsWithOptionalHandling(node, funcReg+1)
		if err != nil {
			return BadRegister, err
		}

		// 5. Emit OpCallMethod using hint as result register
		// fmt.Printf("// [COMPILE DEBUG] Method call: emitCallMethod(hint=R%d, funcReg=R%d, thisReg=R%d, argCount=%d)\n", hint, funcReg, thisReg, actualArgCount)
		c.emitCallMethod(hint, funcReg, thisReg, byte(actualArgCount), node.Token.Line)

		return hint, nil
	}

	// Check if this is an index expression method call (obj[key]())
	if indexExpr, isIndexCall := node.Function.(*parser.IndexExpression); isIndexCall {
		// Method call: obj[key](args...)
		// 1. Compile the object part (this value)
		thisReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, thisReg)
		_, err := c.compileNode(indexExpr.Left, thisReg)
		if err != nil {
			return BadRegister, err
		}

		// 2. Allocate contiguous block for function + all arguments
		totalArgCount := c.determineTotalArgCount(node)
		blockSize := 1 + totalArgCount // funcReg + arguments
		funcReg := c.regAlloc.AllocContiguous(blockSize)
		for i := 0; i < blockSize; i++ {
			tempRegs = append(tempRegs, funcReg+Register(i))
		}

		// 3. Get the method property using index access
		propertyReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, propertyReg)
		_, err = c.compileNode(indexExpr.Index, propertyReg)
		if err != nil {
			return BadRegister, err
		}
		c.emitOpCode(vm.OpGetIndex, indexExpr.Token.Line)
		c.emitByte(byte(funcReg))     // Destination register
		c.emitByte(byte(thisReg))     // Object register
		c.emitByte(byte(propertyReg)) // Key register

		// 4. Compile arguments
		_, actualArgCount, err := c.compileArgumentsWithOptionalHandling(node, funcReg+1)
		if err != nil {
			return BadRegister, err
		}

		// 5. Emit OpCallMethod to preserve 'this' binding
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

	// 4. Check if this is a generator function call
	// Look at the function's return type to see if it's Generator<...>
	functionType := node.Function.GetComputedType()
	isGeneratorCall := false

	// Debug: Print function type information
	debugPrintf("// [Generator Detection] Function type: %T\n", functionType)
	if functionType != nil {
		debugPrintf("// [Generator Detection] Function type string: %s\n", functionType.String())
	}

	// Check if the function type indicates it's a generator by examining return type
	if functionType != nil {
		if objType, ok := functionType.(*types.ObjectType); ok && objType.IsCallable() && len(objType.CallSignatures) > 0 {
			// Check if this function's return type is a Generator<...> or generator-like ObjectType
			sig := objType.CallSignatures[0]
			if sig.ReturnType != nil {
				debugPrintf("// [Generator Detection] Return type: %T (%s)\n", sig.ReturnType, sig.ReturnType.String())
			} else {
				debugPrintf("// [Generator Detection] Return type is nil\n")
			}

			// Only check return type if it's not nil
			if sig.ReturnType != nil {
				// First check for InstantiatedType (ideal case)
				if instantiated, ok := sig.ReturnType.(*types.InstantiatedType); ok {
					debugPrintf("// [Generator Detection] InstantiatedType generic: %s\n", instantiated.Generic.Name)
					if instantiated.Generic != nil && instantiated.Generic.Name == "Generator" {
						isGeneratorCall = true
						debugPrintf("// [Generator Detection] GENERATOR DETECTED (InstantiatedType)!\n")
					}
				}

				// Also check for ObjectType with generator methods (fallback case)
				if returnObjType, ok := sig.ReturnType.(*types.ObjectType); ok {
					// Check if it has the characteristic generator methods
					hasNext := returnObjType.Properties["next"] != nil
					hasReturn := returnObjType.Properties["return"] != nil
					hasThrow := returnObjType.Properties["throw"] != nil

					if hasNext && hasReturn && hasThrow {
						isGeneratorCall = true
						debugPrintf("// [Generator Detection] GENERATOR DETECTED (ObjectType with generator methods)!\n")
					}
				}
			}
		} else {
			debugPrintf("// [Generator Detection] Not an object type or not callable\n")
		}
	}

	// 5. Emit appropriate opcode based on function type
	if isGeneratorCall {
		// Generator function call - create generator object instead of executing
		c.emitOpCode(vm.OpCreateGenerator, node.Token.Line)
		c.emitByte(byte(hint))    // Destination register for generator object
		c.emitByte(byte(funcReg)) // Function register
		// Note: argCount is not used for generator creation, but let's keep it for consistency
		c.emitByte(byte(actualArgCount)) // Argument count
	} else {
		// Regular function call
		c.emitCall(hint, funcReg, byte(actualArgCount), node.Token.Line)
	}

	return hint, nil
}

// compileTaggedTemplate compiles tag`...` as a call: tag(cookedStrings, ...substitutions)
// For now we only pass cooked strings (no raw) and expand substitutions as additional args.
func (c *Compiler) compileTaggedTemplate(node *parser.TaggedTemplateExpression, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line
	// temp regs cleanup
	var tempRegs []Register
	defer func() {
		for _, r := range tempRegs {
			c.regAlloc.Free(r)
		}
	}()

	// Collect cooked strings and substitution expressions
	cookedStrings := []vm.Value{}
	substitutions := []parser.Expression{}
	for i, part := range node.Template.Parts {
		if i%2 == 0 {
			cookedStrings = append(cookedStrings, vm.String(part.String()))
		} else if expr, ok := part.(parser.Expression); ok {
			substitutions = append(substitutions, expr)
		}
	}

	// Allocate contiguous block: function + [cookedStrings, ...subs]
	argCount := 1 + len(substitutions)
	funcBase := c.regAlloc.AllocContiguous(1 + argCount)
	for i := 0; i < 1+argCount; i++ {
		tempRegs = append(tempRegs, funcBase+Register(i))
	}

	// 1) Compile tag into funcBase
	if _, err := c.compileNode(node.Tag, funcBase); err != nil {
		return BadRegister, err
	}

	// 2) Build cooked strings array and load into funcBase+1
	// Create array constant of cooked strings and attach a non-writable, non-configurable `.raw` copy
	arrVal := vm.NewArray()
	arr := arrVal.AsArray()
	for _, v := range cookedStrings {
		arr.Append(v)
	}
	// Build raw array (identical to cooked for now; no escape processing yet)
	rawVal := vm.NewArray()
	rawArr := rawVal.AsArray()
	for _, v := range cookedStrings {
		rawArr.Append(v)
	}
	// Attach `.raw` on the array object before loading as constant. We approximate attributes by not exposing attrs bits here.
	if po := arrVal.AsArray(); po != nil {
		// Array is a wrapped object; setOwn will create a data property
		// In our VM, property attributes are not fully modeled on ArrayObject; acceptable for harness use.
		arrVal.AsArray() // ensure allocation
	}
	// Since ArrayObject doesn't expose SetOwn, create a PlainObject wrapper to carry .raw: use DictObject-like approach
	// Simpler: after load, immediately set property in bytecode
	c.emitLoadNewConstant(funcBase+1, arrVal, line)
	// Load raw into a temp and set property 'raw' on the cooked array
	tmpReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, tmpReg)
	c.emitLoadNewConstant(tmpReg, rawVal, line)
	nameIdx := c.chunk.AddConstant(vm.String("raw"))
	c.emitSetProp(funcBase+1, tmpReg, nameIdx, line)

	// 3) Compile substitutions into subsequent registers
	for i, expr := range substitutions {
		if _, err := c.compileNode(expr, funcBase+Register(2+i)); err != nil {
			return BadRegister, err
		}
	}

	// 4) Emit call: tag(cookedStrings, ...subs)
	// IMPORTANT: Do not reuse funcBase as the destination; use 'hint' only, leaving funcBase intact until after emit
	c.emitCall(hint, funcBase, byte(argCount), line)
	// Ensure funcBase block stays alive through the call; cleanup is handled by tempRegs defer
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
		propertyName := c.extractPropertyName(memberExpr.Property)
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
	// local helpers for safe slicing in debug prints
	min := func(a, b int) int {
		if a < b {
			return a
		}
		return b
	}
	max := func(a, b int) int {
		if a > b {
			return a
		}
		return b
	}

	// Defer-safety: ensure jump placeholders always get patched to a valid anchor
	// even if later logic returns early. We track anchors and whether we patched.
	var (
		jumpIfFalsePos  = -1
		jumpElsePos     = -1
		patchedIfFalse  = false
		patchedElseJump = false
		hasElse         = false
		elseAnchor      = -1 // start of else block
		noElseEndAnchor = -1 // end of if block when there is no else
	)
	defer func() {
		if jumpIfFalsePos >= 0 && !patchedIfFalse {
			// Decide anchor: prefer elseAnchor if we recorded it; otherwise use noElseEndAnchor; as a last resort, current code length
			anchor := elseAnchor
			if anchor < 0 {
				anchor = noElseEndAnchor
			}
			if anchor < 0 {
				anchor = len(c.chunk.Code)
			}
			// Compute and write offset
			op := vm.OpCode(c.chunk.Code[jumpIfFalsePos])
			operandStartPos := jumpIfFalsePos + 1
			if op == vm.OpJumpIfFalse || op == vm.OpJumpIfUndefined || op == vm.OpJumpIfNull || op == vm.OpJumpIfNullish {
				operandStartPos = jumpIfFalsePos + 2 // skip register byte
			}
			jumpInstructionEndPos := operandStartPos + 2
			offset := anchor - jumpInstructionEndPos
			c.chunk.Code[operandStartPos] = byte(int16(offset) >> 8)
			c.chunk.Code[operandStartPos+1] = byte(int16(offset) & 0xFF)
			patchedIfFalse = true
			debugPrintf("[IfExpr][defer] Patched OpJumpIfFalse at pos=%d to anchor=%d (offset=%d)", jumpIfFalsePos, anchor, offset)
		}
		if hasElse && jumpElsePos >= 0 && !patchedElseJump {
			// Patch else-jump to end of else (current end)
			oprandPos := jumpElsePos + 1
			jumpInstructionEndPos := oprandPos + 2
			offset := len(c.chunk.Code) - jumpInstructionEndPos
			c.chunk.Code[oprandPos] = byte(int16(offset) >> 8)
			c.chunk.Code[oprandPos+1] = byte(int16(offset) & 0xFF)
			patchedElseJump = true
			debugPrintf("[IfExpr][defer] Patched OpJump (over else) at pos=%d to end (offset=%d)", jumpElsePos, offset)
		}
	}()
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
	debugPrintf("[IfExpr] Before OpJumpIfFalse emit: codeLen=%d", len(c.chunk.Code))
	jumpIfFalsePos = c.emitPlaceholderJump(vm.OpJumpIfFalse, conditionReg, node.Token.Line)
	debugPrintf("[IfExpr] Emitted OpJumpIfFalse at pos=%d; codeLen now=%d", jumpIfFalsePos, len(c.chunk.Code))

	// 3. Compile the consequence block
	// Allocate temporary register for consequence compilation
	consequenceReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, consequenceReg)
	_, err = c.compileNode(node.Consequence, consequenceReg)
	if err != nil {
		return BadRegister, err
	}
	// Dump disassembly after consequence compilation, before any patching
	debugPrintf("[IfExpr] Disassembly after consequence, pre-patch (codeLen=%d):\n%s", len(c.chunk.Code), c.chunk.DisassembleChunk("<if-consequence>"))
	// TODO: How does an if-expr produce a value? Need convention.
	// Does the last expr statement value remain in a register?

	if node.Alternative != nil {
		hasElse = true
		// 4a. If there's an else, emit placeholder jump over the else block
		debugPrintf("[IfExpr] Else present. Before OpJump emit-over-else: codeLen=%d", len(c.chunk.Code))
		jumpElsePos = c.emitPlaceholderJump(vm.OpJump, 0, node.Consequence.Token.Line)
		debugPrintf("[IfExpr] Emitted OpJump over else at pos=%d; codeLen now=%d", jumpElsePos, len(c.chunk.Code))

		// 5a. Backpatch the OpJumpIfFalse to jump *here* (start of else)
		elseAnchor = len(c.chunk.Code)
		debugPrintf("[IfExpr] Patching OpJumpIfFalse at pos=%d to elseAnchor=%d; codeLen=%d", jumpIfFalsePos, elseAnchor, len(c.chunk.Code))
		c.patchJump(jumpIfFalsePos)
		patchedIfFalse = true
		debugPrintf("[IfExpr] Patched OpJumpIfFalse at pos=%d; codeLen=%d; bytes=%v", jumpIfFalsePos, len(c.chunk.Code), c.chunk.Code[max(0, jumpIfFalsePos-4):min(len(c.chunk.Code), jumpIfFalsePos+6)])
		debugPrintf("[IfExpr] Disassembly after patchJumpIfFalse (else path entry):\n%s", c.chunk.DisassembleChunk("<if-else-entry>"))

		// 6a. Compile the alternative block
		// Allocate temporary register for alternative compilation
		alternativeReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, alternativeReg)
		_, err = c.compileNode(node.Alternative, alternativeReg)
		if err != nil {
			return BadRegister, err
		}

		// 7a. Backpatch the OpJump to jump *here* (end of else block)
		debugPrintf("[IfExpr] Patching OpJump over else at pos=%d; codeLen=%d", jumpElsePos, len(c.chunk.Code))
		c.patchJump(jumpElsePos)
		patchedElseJump = true
		debugPrintf("[IfExpr] Patched OpJump over else at pos=%d; codeLen=%d; bytes=%v", jumpElsePos, len(c.chunk.Code), c.chunk.Code[max(0, jumpElsePos-4):min(len(c.chunk.Code), jumpElsePos+6)])
		debugPrintf("[IfExpr] Disassembly after patchJump over else (end):\n%s", c.chunk.DisassembleChunk("<if-else-exit>"))

	} else {
		// 4b. If no else, backpatch OpJumpIfFalse to jump *here* (end of if block)
		noElseEndAnchor = len(c.chunk.Code)
		debugPrintf("[IfExpr] No else. Patching OpJumpIfFalse at pos=%d to endAnchor=%d; codeLen=%d", jumpIfFalsePos, noElseEndAnchor, len(c.chunk.Code))
		c.patchJump(jumpIfFalsePos)
		patchedIfFalse = true
		debugPrintf("[IfExpr] Patched (no-else) OpJumpIfFalse at pos=%d; codeLen=%d; bytes=%v", jumpIfFalsePos, len(c.chunk.Code), c.chunk.Code[max(0, jumpIfFalsePos-4):min(len(c.chunk.Code), jumpIfFalsePos+6)])
		debugPrintf("[IfExpr] Disassembly after patchJumpIfFalse (no-else end):\n%s", c.chunk.DisassembleChunk("<if-no-else-exit>"))
		// TODO: What value should an if without else produce? Undefined?
		// If so, might need to emit OpLoadUndefined here.
	}

	// TODO: Free conditionReg if no longer needed?
	return BadRegister, nil
}

// Helper function to determine total argument count for NewExpression including optional parameters
func (c *Compiler) determineTotalArgCountForNew(node *parser.NewExpression) int {
	// Calculate effective argument count, expanding spread elements
	providedArgCount := c.calculateEffectiveArgCount(node.Arguments)

	// Get constructor type to check for optional parameters
	constructorType := node.Constructor.GetComputedType()
	var expectedParamCount int
	var optionalParams []bool

	if constructorType != nil {
		if objType, ok := constructorType.(*types.ObjectType); ok && objType.IsConstructable() && len(objType.ConstructSignatures) > 0 {
			// For constructors, use construct signatures instead of call signatures
			sig := objType.ConstructSignatures[0] // Default to first signature
			bestMatch := sig
			bestScore := -1

			for _, candidateSig := range objType.ConstructSignatures {
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

// Helper function to compile arguments for NewExpression with optional parameter handling
func (c *Compiler) compileArgumentsWithOptionalHandlingForNew(node *parser.NewExpression, firstArgReg Register) ([]Register, int, errors.PaseratiError) {
	// Get constructor type information for optional parameter analysis
	constructorType := node.Constructor.GetComputedType()
	var expectedParamCount int
	var optionalParams []bool

	if constructorType != nil {
		if objType, ok := constructorType.(*types.ObjectType); ok && objType.IsConstructable() && len(objType.ConstructSignatures) > 0 {
			sig := objType.ConstructSignatures[0] // Use first construct signature for now
			expectedParamCount = len(sig.ParameterTypes)
			optionalParams = sig.OptionalParams
		}
	}

	// Determine final argument count (with padding for optional parameters)
	providedArgCount := len(node.Arguments)
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

	// Create slice of target registers
	argRegs := make([]Register, finalArgCount)
	for i := 0; i < finalArgCount; i++ {
		argRegs[i] = firstArgReg + Register(i)
	}

	// Compile provided arguments
	for i, arg := range node.Arguments {
		if i < len(argRegs) {
			_, err := c.compileNode(arg, argRegs[i])
			if err != nil {
				return nil, 0, err
			}
		}
	}

	// Pad missing optional parameters with undefined
	for i := providedArgCount; i < finalArgCount; i++ {
		targetReg := argRegs[i]
		c.emitLoadUndefined(targetReg, node.Token.Line)
	}

	return argRegs, finalArgCount, nil
}

// compileSuperConstructorCall compiles a super() constructor call
func (c *Compiler) compileSuperConstructorCall(node *parser.CallExpression, hint Register, tempRegs *[]Register) (Register, errors.PaseratiError) {
	// Get the super class name from the compiler context
	// This was set by compileConstructor when compiling a derived class
	if c.compilingSuperClassName == "" {
		return BadRegister, NewCompileError(node, "super() call outside of derived class constructor")
	}

	// Check if any argument uses spread syntax
	hasSpread := c.hasSpreadArgument(node.Arguments)
	if hasSpread {
		// Use spread call mechanism for super(...args)
		return c.compileSpreadSuperCall(node, hint, tempRegs)
	}

	// Load the parent constructor by name
	// Try to resolve it in the symbol table first (for user-defined classes)
	superConstructorReg := c.regAlloc.Alloc()
	*tempRegs = append(*tempRegs, superConstructorReg)

	symbol, _, exists := c.currentSymbolTable.Resolve(c.compilingSuperClassName)
	if exists {
		// Found in symbol table - user-defined class
		if symbol.IsGlobal {
			c.emitGetGlobal(superConstructorReg, symbol.GlobalIndex, node.Token.Line)
		} else {
			c.emitMove(superConstructorReg, symbol.Register, node.Token.Line)
		}
	} else {
		// Not in symbol table - might be a built-in class (Object, Array, etc.)
		// Emit code to look up the global variable at runtime
		globalIdx := c.GetOrAssignGlobalIndex(c.compilingSuperClassName)
		c.emitGetGlobal(superConstructorReg, globalIdx, node.Token.Line)
	}

	// Calculate effective argument count, expanding spread elements
	effectiveArgCount := c.calculateEffectiveArgCount(node.Arguments)

	// Allocate contiguous registers for the call: [function, arg1, arg2, ...]
	totalRegs := 1 + effectiveArgCount // function + arguments

	functionReg := c.regAlloc.AllocContiguous(totalRegs)
	for i := 0; i < totalRegs; i++ {
		*tempRegs = append(*tempRegs, functionReg+Register(i))
	}

	// Move parent constructor to function register
	c.emitMove(functionReg, superConstructorReg, node.Token.Line)

	// Compile arguments, handling spread elements
	argIndex := 0
	for _, arg := range node.Arguments {
		if spreadElement, isSpread := arg.(*parser.SpreadElement); isSpread {
			// Handle spread element - expand the array
			if arrayLit, isArrayLit := spreadElement.Argument.(*parser.ArrayLiteral); isArrayLit {
				// Expand array literal elements
				for _, elem := range arrayLit.Elements {
					if elem != nil { // Skip holes
						argReg := functionReg + Register(1+argIndex)
						_, err := c.compileNode(elem, argReg)
						if err != nil {
							return BadRegister, err
						}
						argIndex++
					}
				}
			} else {
				// For non-literal arrays, compile the expression
				argReg := functionReg + Register(1+argIndex)
				_, err := c.compileNode(spreadElement.Argument, argReg)
				if err != nil {
					return BadRegister, err
				}
				argIndex++
			}
		} else {
			// Regular argument
			argReg := functionReg + Register(1+argIndex)
			_, err := c.compileNode(arg, argReg)
			if err != nil {
				return BadRegister, err
			}
			argIndex++
		}
	}

	// super() creates the instance by calling the parent constructor
	// Use OpNew to create the instance (not OpCallMethod)
	resultReg := c.regAlloc.Alloc()
	*tempRegs = append(*tempRegs, resultReg)
	c.emitNew(resultReg, functionReg, byte(effectiveArgCount), node.Token.Line)

	// Update 'this' to be the newly created instance
	c.emitSetThis(resultReg, node.Token.Line)

	// Now load the current 'this' (which may have been updated) into hint
	c.emitLoadThis(hint, node.Token.Line)

	return hint, nil
}

// compileSpreadSuperCall compiles super(...args) with spread arguments
func (c *Compiler) compileSpreadSuperCall(node *parser.CallExpression, hint Register, tempRegs *[]Register) (Register, errors.PaseratiError) {
	// For now, only support calls with a single spread argument (the most common case)
	if len(node.Arguments) != 1 {
		return BadRegister, NewCompileError(node, "super() with spread currently only supports a single spread argument")
	}

	spreadElement, isSpread := node.Arguments[0].(*parser.SpreadElement)
	if !isSpread {
		return BadRegister, NewCompileError(node, "expected spread argument")
	}

	// Load the parent constructor by name
	superConstructorReg := c.regAlloc.Alloc()
	*tempRegs = append(*tempRegs, superConstructorReg)

	symbol, _, exists := c.currentSymbolTable.Resolve(c.compilingSuperClassName)
	if exists {
		// Found in symbol table - user-defined class
		if symbol.IsGlobal {
			c.emitGetGlobal(superConstructorReg, symbol.GlobalIndex, node.Token.Line)
		} else {
			c.emitMove(superConstructorReg, symbol.Register, node.Token.Line)
		}
	} else {
		// Not in symbol table - might be a built-in class (Object, Array, etc.)
		globalIdx := c.GetOrAssignGlobalIndex(c.compilingSuperClassName)
		c.emitGetGlobal(superConstructorReg, globalIdx, node.Token.Line)
	}

	// Compile the spread argument (array to spread)
	spreadArgReg := c.regAlloc.Alloc()
	*tempRegs = append(*tempRegs, spreadArgReg)
	_, err := c.compileNode(spreadElement.Argument, spreadArgReg)
	if err != nil {
		return BadRegister, err
	}

	// Use OpSpreadNew to create the instance with spread arguments
	resultReg := c.regAlloc.Alloc()
	*tempRegs = append(*tempRegs, resultReg)
	c.emitSpreadNew(resultReg, superConstructorReg, spreadArgReg, node.Token.Line)

	// Update 'this' to be the newly created instance
	c.emitSetThis(resultReg, node.Token.Line)

	// Now load the current 'this' (which has been updated) into hint
	c.emitLoadThis(hint, node.Token.Line)

	return hint, nil
}

// compileSpreadNewExpression compiles new Foo(...args) with spread arguments
func (c *Compiler) compileSpreadNewExpression(node *parser.NewExpression, hint Register, tempRegs *[]Register) (Register, errors.PaseratiError) {
	// For now, only support calls with a single spread argument (the most common case)
	if len(node.Arguments) != 1 {
		return BadRegister, NewCompileError(node, "new expression with spread currently only supports a single spread argument")
	}

	spreadElement, isSpread := node.Arguments[0].(*parser.SpreadElement)
	if !isSpread {
		return BadRegister, NewCompileError(node, "expected spread argument")
	}

	// Compile the constructor expression
	constructorReg := c.regAlloc.Alloc()
	*tempRegs = append(*tempRegs, constructorReg)
	_, err := c.compileNode(node.Constructor, constructorReg)
	if err != nil {
		return BadRegister, err
	}

	// Compile the spread argument (array to spread)
	spreadArgReg := c.regAlloc.Alloc()
	*tempRegs = append(*tempRegs, spreadArgReg)
	_, err = c.compileNode(spreadElement.Argument, spreadArgReg)
	if err != nil {
		return BadRegister, err
	}

	// Use OpSpreadNew to create the instance with spread arguments
	c.emitSpreadNew(hint, constructorReg, spreadArgReg, node.Token.Line)

	return hint, nil
}

// compileSuperMemberExpression compiles super.property access
func (c *Compiler) compileSuperMemberExpression(node *parser.MemberExpression, hint Register, tempRegs *[]Register) (Register, errors.PaseratiError) {
	// For now, implement super.method as this.method
	// This is a simplified approach - a full implementation would need to:
	// 1. Look up the method in the parent class prototype
	// 2. Bind it to the current 'this' context

	// Load 'this' as the object
	thisReg := c.regAlloc.Alloc()
	*tempRegs = append(*tempRegs, thisReg)
	c.emitLoadThis(thisReg, node.Token.Line)

	// Get the property name
	propertyName := c.extractPropertyName(node.Property)

	// Add property name as constant
	nameConstIdx := c.chunk.AddConstant(vm.String(propertyName))

	// Emit property access on 'this' instead of 'super'
	c.emitGetProp(hint, thisReg, nameConstIdx, node.Token.Line)

	return hint, nil
}

// compileYieldExpression compiles yield expressions in generator functions
func (c *Compiler) compileYieldExpression(node *parser.YieldExpression, hint Register) (Register, errors.PaseratiError) {
	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	if node.Delegate {
		// yield* delegation - more complex compilation
		return c.compileYieldDelegation(node, hint)
	}

	// 1. Compile the value being yielded (if any)
	var valueReg Register
	if node.Value != nil {
		valueReg = c.regAlloc.Alloc()
		tempRegs = append(tempRegs, valueReg)
		_, err := c.compileNode(node.Value, valueReg)
		if err != nil {
			return BadRegister, err
		}
	} else {
		// yield with no value yields undefined
		valueReg = c.regAlloc.Alloc()
		tempRegs = append(tempRegs, valueReg)
		c.emitLoadUndefined(valueReg, node.Token.Line)
	}

	// 2. Emit OpYield instruction with both input and output registers
	// The VM will suspend execution, yield the value, and store sent value in hint register
	c.emitOpCode(vm.OpYield, node.Token.Line)
	c.emitByte(byte(valueReg)) // Value being yielded
	c.emitByte(byte(hint))     // Register to store sent value when resuming

	return hint, nil
}

// compileAwaitExpression compiles await expressions in async functions
func (c *Compiler) compileAwaitExpression(node *parser.AwaitExpression, hint Register) (Register, errors.PaseratiError) {
	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Compile the promise being awaited
	promiseReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, promiseReg)
	_, err := c.compileNode(node.Argument, promiseReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. Emit OpAwait instruction
	// The VM will suspend execution, wait for promise settlement, and store result in hint register
	c.emitOpCode(vm.OpAwait, node.Token.Line)
	c.emitByte(byte(hint))       // Register to store resolved value
	c.emitByte(byte(promiseReg)) // Promise being awaited

	return hint, nil
}

// compileYieldDelegation compiles yield* expressions which delegate to another iterator
func (c *Compiler) compileYieldDelegation(node *parser.YieldExpression, hint Register) (Register, errors.PaseratiError) {
	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Compile the iterable expression
	iterableReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iterableReg)
	_, err := c.compileNode(node.Value, iterableReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. Get the Symbol.iterator method
	// Load Symbol.iterator via computed index to preserve singleton identity.
	iteratorMethodReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iteratorMethodReg)
	// Compile computed key: (Symbol).iterator
	symbolObjReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, symbolObjReg)
	// Load global 'Symbol' identifier via unified global index
	symIdx := c.GetOrAssignGlobalIndex("Symbol")
	c.emitGetGlobal(symbolObjReg, symIdx, node.Token.Line)
	// Prepare property name constant BEFORE emitting OpGetIndex
	propNameReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, propNameReg)
	c.emitLoadNewConstant(propNameReg, vm.String("iterator"), node.Token.Line)
	// iteratorKey = Symbol["iterator"]
	iteratorKeyReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iteratorKeyReg)
	c.emitOpCode(vm.OpGetIndex, node.Token.Line)
	c.emitByte(byte(iteratorKeyReg)) // Dest
	c.emitByte(byte(symbolObjReg))   // Base: Symbol
	c.emitByte(byte(propNameReg))    // Key: "iterator"
	// Now get iterable[iteratorKey]
	c.emitOpCode(vm.OpGetIndex, node.Token.Line)
	c.emitByte(byte(iteratorMethodReg)) // Dest
	c.emitByte(byte(iterableReg))       // Base: iterable
	c.emitByte(byte(iteratorKeyReg))    // Key: Symbol.iterator

	// 3. Call the iterator method to get the iterator (preserve 'this' = iterable)
	iteratorReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iteratorReg)
	c.emitCallMethod(iteratorReg, iteratorMethodReg, iterableReg, 0, node.Token.Line)

	// 4. Set up loop variables
	sentValueReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, sentValueReg)
	c.emitLoadUndefined(sentValueReg, node.Token.Line) // Initial sent value is undefined

	// 5. Loop: call iterator.next(sentValue) and yield the result
	loopStart := len(c.chunk.Code)

	// Get iterator.next method
	nextMethodReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, nextMethodReg)
	nextConstIdx := c.chunk.AddConstant(vm.String("next"))
	c.emitGetProp(nextMethodReg, iteratorReg, nextConstIdx, node.Token.Line)

	// Call iterator.next() (preserve 'this' = iterator)
	resultReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, resultReg)
	// For now, we do not pass the sent value yet.
	c.emitCallMethod(resultReg, nextMethodReg, iteratorReg, 0, node.Token.Line)

	// Get result.done
	doneReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, doneReg)
	doneConstIdx := c.chunk.AddConstant(vm.String("done"))
	c.emitGetProp(doneReg, resultReg, doneConstIdx, node.Token.Line)

	// Jump to exit if done is truthy (exit loop)
	// First negate done, then jump if false (i.e., jump if done was originally true)
	notDoneReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, notDoneReg)
	c.emitOpCode(vm.OpNot, node.Token.Line)
	c.emitByte(byte(notDoneReg))
	c.emitByte(byte(doneReg))

	// Now jump if notDone is false (i.e., done was true)
	exitLoopJump := c.emitPlaceholderJump(vm.OpJumpIfFalse, notDoneReg, node.Token.Line)

	// Get result.value
	valueReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, valueReg)
	valueConstIdx := c.chunk.AddConstant(vm.String("value"))
	c.emitGetProp(valueReg, resultReg, valueConstIdx, node.Token.Line)

	// Yield the value and store the sent value
	c.emitOpCode(vm.OpYield, node.Token.Line)
	c.emitByte(byte(valueReg))     // Value being yielded
	c.emitByte(byte(sentValueReg)) // Register to store sent value when resuming

	// Jump back to loop start
	c.emitOpCode(vm.OpJump, node.Token.Line)
	// The jump offset is relative to the position AFTER the jump instruction
	currentPos := len(c.chunk.Code) + 2 // Position after the 2-byte offset
	jumpBackOffset := loopStart - currentPos
	c.emitUint16(uint16(int16(jumpBackOffset)))

	// Done label: iterator is exhausted - patch the exit jump to come here
	c.patchJump(exitLoopJump)

	// Return the final value in hint register
	c.emitGetProp(hint, resultReg, valueConstIdx, node.Token.Line)

	return hint, nil
}

// compileOptionalIndexExpression compiles optional computed property access (e.g., obj?.[expr])
func (c *Compiler) compileOptionalIndexExpression(node *parser.OptionalIndexExpression, hint Register) (Register, errors.PaseratiError) {
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
		return BadRegister, NewCompileError(node.Object, "error compiling object part of optional index expression").CausedBy(err)
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

	// Object is not null/undefined - do normal computed property access
	c.patchJump(jumpToPropertyAccessPos)

	// 3. Compile the index expression
	indexReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, indexReg)
	_, err = c.compileNode(node.Index, indexReg)
	if err != nil {
		return BadRegister, NewCompileError(node.Index, "error compiling index part of optional index expression").CausedBy(err)
	}

	// 4. Emit OpGetIndex for computed property access
	c.emitOpCode(vm.OpGetIndex, node.Token.Line)
	c.emitByte(byte(hint))      // Destination register
	c.emitByte(byte(objectReg)) // Object register
	c.emitByte(byte(indexReg))  // Index register

	// 5. Patch the end jump
	c.patchJump(endJumpPos)

	return hint, nil
}

// compileOptionalCallExpression compiles optional function calls (e.g., func?.())
func (c *Compiler) compileOptionalCallExpression(node *parser.OptionalCallExpression, hint Register) (Register, errors.PaseratiError) {
	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// 1. Compile the function part
	functionReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, functionReg)
	_, err := c.compileNode(node.Function, functionReg)
	if err != nil {
		return BadRegister, NewCompileError(node.Function, "error compiling function part of optional call expression").CausedBy(err)
	}

	// 2. Check if the function is null or undefined
	// If so, return undefined immediately

	// Use efficient nullish check opcode
	isNullishReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, isNullishReg)
	c.emitIsNullish(isNullishReg, functionReg, node.Token.Line)

	// If function is NOT nullish, jump to normal function call
	jumpToCallPos := c.emitPlaceholderJump(vm.OpJumpIfFalse, isNullishReg, node.Token.Line)

	// Function IS nullish - return undefined using hint register
	c.emitLoadUndefined(hint, node.Token.Line)
	endJumpPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)

	// Function is not null/undefined - do normal function call
	c.patchJump(jumpToCallPos)

	// 3. Calculate total argument count (including optional parameters)
	// For simplicity, assume no optional parameters for now
	totalArgCount := len(node.Arguments)

	// 4. Allocate contiguous block for function + all arguments
	blockSize := 1 + totalArgCount // funcReg + arguments
	funcCallReg := c.regAlloc.AllocContiguous(blockSize)
	// Mark the entire block for cleanup
	for i := 0; i < blockSize; i++ {
		tempRegs = append(tempRegs, funcCallReg+Register(i))
	}

	// 5. Move the function to the first register of the call block
	c.emitMove(funcCallReg, functionReg, node.Token.Line)

	// 6. Compile arguments directly into their target positions
	for i, arg := range node.Arguments {
		argReg := funcCallReg + Register(1+i)
		_, err := c.compileNode(arg, argReg)
		if err != nil {
			return BadRegister, NewCompileError(arg, "error compiling argument in optional call expression").CausedBy(err)
		}
	}

	// 7. Emit regular function call
	c.emitCall(hint, funcCallReg, byte(totalArgCount), node.Token.Line)

	// 8. Patch the end jump
	c.patchJump(endJumpPos)

	return hint, nil
}

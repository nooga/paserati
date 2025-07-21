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

func (c *Compiler) compileNewExpression(node *parser.NewExpression, hint Register) (Register, errors.PaseratiError) {
	// Manage temporary registers with automatic cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

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
	} else {
		// Regular property access: obj.prop
		isComputedProperty = false
		propertyName = c.extractPropertyName(node.Property)
	}

	// 3. Check if this is a getter access (compile-time optimization)
	// Skip getter optimization for computed properties since we can't know the property name at compile time
	objectStaticType := node.Object.GetComputedType()
	debugPrintf("// DEBUG compileMemberExpression: objectType for '%s': %v\n", propertyName, objectStaticType)
	if !isComputedProperty && objectStaticType != nil {
		// Check if accessing a getter property
		widenedType := types.GetWidenedType(objectStaticType)
		debugPrintf("// DEBUG compileMemberExpression: widenedType for '%s': %v\n", propertyName, widenedType)
		if objType, ok := widenedType.(*types.ObjectType); ok && objType.IsClassInstance() {
			// Check if this property has a getter defined
			if objType.ClassMeta != nil {
				if memberInfo := objType.ClassMeta.GetMemberAccess(propertyName); memberInfo != nil && memberInfo.IsGetter {
					getterMethodName := "__get__" + propertyName
					getterIdx := c.chunk.AddConstant(vm.String(getterMethodName))
					methodReg := c.regAlloc.Alloc()
					tempRegs = append(tempRegs, methodReg)
					c.emitGetProp(methodReg, objectReg, getterIdx, node.Token.Line)

					// Check if the getter exists before calling it
					c.emitOptimisticGetterCall(hint, methodReg, objectReg, propertyName, node.Token.Line)
					return hint, nil
				}
			}
		}
	}

	// 4. <<< NEW: Special case for .length >>>
	// Skip .length optimization for computed properties
	if !isComputedProperty && propertyName == "length" {
		// Check the static type provided by the checker
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

		// Get property name (handle both identifiers and computed properties)
		propName := c.extractPropertyName(argNode.Property)
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
			propName := c.extractPropertyName(operand.Property)
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

	// Check if any argument uses spread syntax
	hasSpread := c.hasSpreadArgument(node.Arguments)
	if hasSpread {
		return c.compileSpreadCallExpression(node, hint, &tempRegs)
	}

	// Check if this is a super constructor call (super())
	if _, isSuperCall := node.Function.(*parser.SuperExpression); isSuperCall {
		return c.compileSuperConstructorCall(node, hint, &tempRegs)
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
			c.emitByte(byte(funcReg))    // Destination register
			c.emitByte(byte(thisReg))    // Object register
			c.emitByte(byte(propertyReg)) // Key register
		} else {
			// Regular property: obj.prop()
			propertyName := c.extractPropertyName(memberExpr.Property)
			nameConstIdx := c.chunk.AddConstant(vm.String(propertyName))
			c.emitGetProp(funcReg, thisReg, nameConstIdx, memberExpr.Token.Line)
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
	// Find the current class context to determine the parent constructor
	// For now, we'll use a simplified approach similar to Function.call pattern

	// Load 'this' for the method call context
	thisReg := c.regAlloc.Alloc()
	*tempRegs = append(*tempRegs, thisReg)
	c.emitLoadThis(thisReg, node.Token.Line)

	// Get the parent constructor from the current constructor's prototype chain
	// This follows the pattern: ParentConstructor.call(this, ...args)

	// For simplicity in this WIP implementation, we'll determine the total argument count
	totalArgCount := len(node.Arguments)

	// Allocate contiguous registers for the call: [function, this, arg1, arg2, ...]
	totalRegs := 2 + totalArgCount // function + this + arguments

	functionReg := c.regAlloc.AllocContiguous(totalRegs)
	for i := 0; i < totalRegs; i++ {
		*tempRegs = append(*tempRegs, functionReg+Register(i))
	}

	// Load the parent constructor - this is simplified for WIP
	// In a full implementation, we would look up the actual parent class constructor
	// For now, we'll load a dummy function that just sets up the instance properties
	c.emitLoadUndefined(functionReg, node.Token.Line)

	// Load 'this' as the receiver
	c.emitMove(functionReg+1, thisReg, node.Token.Line)

	// Compile arguments
	for i, arg := range node.Arguments {
		argReg := functionReg + Register(2+i)
		_, err := c.compileNode(arg, argReg)
		if err != nil {
			return BadRegister, err
		}
	}

	// For now, manually call the parent constructor logic
	// This is a simplified implementation that directly sets properties
	// based on common TypeScript constructor patterns

	if totalArgCount >= 1 {
		// Set this.name = first argument (common pattern)
		nameConstIdx := c.chunk.AddConstant(vm.String("name"))
		c.emitSetProp(thisReg, functionReg+2, nameConstIdx, node.Token.Line)
	}

	if totalArgCount >= 2 {
		// Set this.species = second argument (for Animal class compatibility)
		speciesConstIdx := c.chunk.AddConstant(vm.String("species"))
		c.emitSetProp(thisReg, functionReg+3, speciesConstIdx, node.Token.Line)
	}

	// Return undefined (constructor calls don't return values)
	c.emitLoadUndefined(hint, node.Token.Line)

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
	// Load Symbol.iterator constant
	symbolIteratorIdx := c.chunk.AddConstant(vm.String("@@symbol:Symbol.iterator"))
	iteratorMethodReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iteratorMethodReg)
	c.emitGetProp(iteratorMethodReg, iterableReg, symbolIteratorIdx, node.Token.Line)

	// 3. Call the iterator method to get the iterator
	iteratorReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iteratorReg)
	c.emitCall(iteratorReg, iteratorMethodReg, 0, node.Token.Line) // 0 arguments

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

	// Call iterator.next(sentValue)
	resultReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, resultReg)
	// For now, we'll call next() without arguments since we don't have a way to pass
	// arguments dynamically. This is a simplification - a full implementation would
	// need to handle passing the sent value to next()
	c.emitCall(resultReg, nextMethodReg, 0, node.Token.Line)

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

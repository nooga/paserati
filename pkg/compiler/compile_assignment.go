package compiler

import (
	"fmt"

	"paserati/pkg/errors"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

const debugAssignment = false // Enable debug output for assignment compilation

// compileAssignmentExpression compiles identifier = value OR indexExpr = value OR memberExpr = value
func (c *Compiler) compileAssignmentExpression(node *parser.AssignmentExpression, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line

	// --- Refactored LHS Handling ---
	var currentValueReg Register // Register holding the value BEFORE the assignment/operation
	var needsStore bool = true   // Assume we need to store back by default
	type lhsInfoType int
	const (
		lhsIsIdentifier lhsInfoType = iota
		lhsIsIndexExpr
		lhsIsMemberExpr
	)
	var lhsType lhsInfoType
	var identInfo struct { // Info needed to store back to identifier
		targetReg    Register
		isUpvalue    bool
		upvalueIndex uint8
		isGlobal     bool   // Track if this is a global variable
		globalIdx    uint16 // Direct global index instead of name constant index
	}
	var indexInfo struct { // Info needed to store back to index expr
		arrayReg Register
		indexReg Register
	}
	var memberInfo struct { // Info needed for member expr
		objectReg    Register
		nameConstIdx uint16
	}

	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		// Clean up all temporary registers
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	switch lhsNode := node.Left.(type) {
	case *parser.Identifier:
		lhsType = lhsIsIdentifier
		// Resolve the identifier
		symbolRef, definingTable, found := c.currentSymbolTable.Resolve(lhsNode.Value)
		if !found {
			// Variable not found in any scope, treat as global assignment
			identInfo.isGlobal = true
			identInfo.globalIdx = c.GetOrAssignGlobalIndex(lhsNode.Value)
			// For compound assignments, we need the current value
			if node.Operator != "=" {
				currentValueReg = c.regAlloc.Alloc()
				tempRegs = append(tempRegs, currentValueReg)
				c.emitGetGlobal(currentValueReg, identInfo.globalIdx, line)
			} else {
				currentValueReg = nilRegister // Not needed for simple assignment
			}
		} else {
			// Determine target register/upvalue index and load current value
			if symbolRef.IsGlobal {
				// Global variable
				identInfo.isGlobal = true
				identInfo.globalIdx = symbolRef.GlobalIndex
				// For compound assignments, we need the current value
				if node.Operator != "=" {
					currentValueReg = c.regAlloc.Alloc()
					tempRegs = append(tempRegs, currentValueReg)
					c.emitGetGlobal(currentValueReg, identInfo.globalIdx, line)
				} else {
					currentValueReg = nilRegister // Not needed for simple assignment
				}
			} else if definingTable == c.currentSymbolTable {
				// Local variable
				identInfo.targetReg = symbolRef.Register
				identInfo.isUpvalue = false
				identInfo.isGlobal = false
				currentValueReg = identInfo.targetReg // Current value is already in targetReg
			} else {
				// Upvalue (either non-global outer scope OR we're in a closure accessing global scope)
				identInfo.isUpvalue = true
				identInfo.isGlobal = false
				identInfo.upvalueIndex = c.addFreeSymbol(node, &symbolRef)
				currentValueReg = c.regAlloc.Alloc() // Allocate temporary reg for current value
				tempRegs = append(tempRegs, currentValueReg)
				c.emitOpCode(vm.OpLoadFree, line)
				c.emitByte(byte(currentValueReg))  // Destination register
				c.emitByte(identInfo.upvalueIndex) // Upvalue index
			}
		}

	case *parser.IndexExpression:
		lhsType = lhsIsIndexExpr
		// Compile array expression
		arrayReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, arrayReg)
		_, err := c.compileNode(lhsNode.Left, arrayReg)
		if err != nil {
			return BadRegister, err
		}
		indexInfo.arrayReg = arrayReg

		// Compile index expression
		indexReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, indexReg)
		_, err = c.compileNode(lhsNode.Index, indexReg)
		if err != nil {
			return BadRegister, err
		}
		indexInfo.indexReg = indexReg

		// Load the current value at the index
		currentValueReg = c.regAlloc.Alloc()
		tempRegs = append(tempRegs, currentValueReg)
		c.emitOpCode(vm.OpGetIndex, line)
		c.emitByte(byte(currentValueReg))
		c.emitByte(byte(indexInfo.arrayReg))
		c.emitByte(byte(indexInfo.indexReg))

	case *parser.MemberExpression:
		lhsType = lhsIsMemberExpr
		// Compile the object expression
		objectReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, objectReg)
		_, err := c.compileNode(lhsNode.Object, objectReg)
		if err != nil {
			return BadRegister, err
		}
		memberInfo.objectReg = objectReg

		// For now, assume property is an Identifier (obj.prop)
		propIdent := lhsNode.Property
		propName := propIdent.Value
		memberInfo.nameConstIdx = c.chunk.AddConstant(vm.String(propName))

		// If compound or logical assignment, load the current property value
		if node.Operator != "=" {
			currentValueReg = c.regAlloc.Alloc()
			tempRegs = append(tempRegs, currentValueReg)
			c.emitGetProp(currentValueReg, memberInfo.objectReg, memberInfo.nameConstIdx, line)
		} else {
			// For simple assignment '=', we don't need the current value
			currentValueReg = nilRegister
		}

	default:
		return BadRegister, NewCompileError(node, fmt.Sprintf("invalid assignment target, expected identifier, index expression, or member expression, got %T", node.Left))
	}

	// Track temporary registers used for operation results
	var operationTempRegs []Register

	var jumpPastStore int = -1
	var jumpToEnd int = -1

	// --- Logical Assignment Operators (&&=, ||=, ??=) ---
	if node.Operator == "&&=" || node.Operator == "||=" || node.Operator == "??=" {
		evaluatedRhs := false
		var rhsValueReg Register

		var jumpToEvalRhs int = -1

		switch node.Operator {
		case "&&=":
			// If FALSEY -> jumpToEnd (skip RHS eval AND store)
			jumpToEnd = c.emitPlaceholderJump(vm.OpJumpIfFalse, currentValueReg, line)
		case "||=":
			// If FALSEY -> jumpToEvalRhs
			jumpToEvalRhs = c.emitPlaceholderJump(vm.OpJumpIfFalse, currentValueReg, line)
			// If TRUTHY -> jumpToEnd (skip RHS eval AND store)
			jumpToEnd = c.emitPlaceholderJump(vm.OpJump, 0, line)
		case "??=":
			// Use efficient nullish check opcode
			isNullishReg := c.regAlloc.Alloc()
			operationTempRegs = append(operationTempRegs, isNullishReg)
			c.emitIsNullish(isNullishReg, currentValueReg, line)
			// If NOT nullish -> jumpToEnd (skip RHS eval AND store)
			jumpToEnd = c.emitPlaceholderJump(vm.OpJumpIfFalse, isNullishReg, line)
		}

		// --- Evaluate RHS Path ---
		if jumpToEvalRhs != -1 {
			c.patchJump(jumpToEvalRhs)
		}
		// This block is only reached if short-circuit didn't happen
		rhsValueReg = c.regAlloc.Alloc()
		operationTempRegs = append(operationTempRegs, rhsValueReg)
		_, err := c.compileNode(node.Value, rhsValueReg)
		if err != nil {
			return BadRegister, err
		}
		evaluatedRhs = true
		needsStore = true
		debugPrintf("// DEBUG Assign Logical RHS: Evaluated RHS. rhsValueReg=R%d, needsStore=%v\n", rhsValueReg, needsStore)

		// Move RHS result to hint register for final result
		if rhsValueReg != hint {
			c.emitMove(hint, rhsValueReg, line)
		}

		// Jump past merge logic
		jumpPastMerge := c.emitPlaceholderJump(vm.OpJump, 0, line)

		// --- Merge logic ---
		c.patchJump(jumpPastMerge)
		if !evaluatedRhs {
			needsStore = false
			// Move current value to hint register for final result
			if currentValueReg != hint {
				c.emitMove(hint, currentValueReg, line)
			}
			debugPrintf("// DEBUG Assign Logical ShortCircuit: needsStore=%v\n", needsStore)
			jumpPastStore = c.emitPlaceholderJump(vm.OpJump, 0, line)
		}

	} else { // --- Non-Logical Assignment ---
		// Compile RHS
		rhsValueReg := c.regAlloc.Alloc()
		operationTempRegs = append(operationTempRegs, rhsValueReg)
		_, err := c.compileNode(node.Value, rhsValueReg)
		if err != nil {
			return BadRegister, NewCompileError(node, "error compiling RHS").CausedBy(err)
		}

		needsStore = true

		switch node.Operator {
		// --- Compound Arithmetic ---
		case "+=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue && !identInfo.isGlobal && currentValueReg == identInfo.targetReg {
				c.emitAdd(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
			} else {
				c.emitAdd(hint, currentValueReg, rhsValueReg, line)
			}
		case "-=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue && !identInfo.isGlobal && currentValueReg == identInfo.targetReg {
				c.emitSubtract(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
			} else {
				c.emitSubtract(hint, currentValueReg, rhsValueReg, line)
			}
		case "*=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue && !identInfo.isGlobal && currentValueReg == identInfo.targetReg {
				c.emitMultiply(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
			} else {
				c.emitMultiply(hint, currentValueReg, rhsValueReg, line)
			}
		case "/=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue && !identInfo.isGlobal && currentValueReg == identInfo.targetReg {
				c.emitDivide(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
			} else {
				c.emitDivide(hint, currentValueReg, rhsValueReg, line)
			}
		case "%=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue && !identInfo.isGlobal && currentValueReg == identInfo.targetReg {
				c.emitRemainder(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
			} else {
				c.emitRemainder(hint, currentValueReg, rhsValueReg, line)
			}
		case "**=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue && !identInfo.isGlobal && currentValueReg == identInfo.targetReg {
				c.emitExponent(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
			} else {
				c.emitExponent(hint, currentValueReg, rhsValueReg, line)
			}

		// --- Compound Bitwise / Shift ---
		case "&=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue && !identInfo.isGlobal && currentValueReg == identInfo.targetReg {
				c.emitBitwiseAnd(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
			} else {
				c.emitBitwiseAnd(hint, currentValueReg, rhsValueReg, line)
			}
		case "|=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue && !identInfo.isGlobal && currentValueReg == identInfo.targetReg {
				c.emitBitwiseOr(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
			} else {
				c.emitBitwiseOr(hint, currentValueReg, rhsValueReg, line)
			}
		case "^=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue && !identInfo.isGlobal && currentValueReg == identInfo.targetReg {
				c.emitBitwiseXor(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
			} else {
				c.emitBitwiseXor(hint, currentValueReg, rhsValueReg, line)
			}
		case "<<=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue && !identInfo.isGlobal && currentValueReg == identInfo.targetReg {
				c.emitShiftLeft(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
			} else {
				c.emitShiftLeft(hint, currentValueReg, rhsValueReg, line)
			}
		case ">>=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue && !identInfo.isGlobal && currentValueReg == identInfo.targetReg {
				c.emitShiftRight(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
			} else {
				c.emitShiftRight(hint, currentValueReg, rhsValueReg, line)
			}
		case ">>>=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue && !identInfo.isGlobal && currentValueReg == identInfo.targetReg {
				c.emitUnsignedShiftRight(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				if currentValueReg != hint {
					c.emitMove(hint, currentValueReg, line)
				}
			} else {
				c.emitUnsignedShiftRight(hint, currentValueReg, rhsValueReg, line)
			}

		// --- Simple Assignment ---
		case "=":
			// Simple assignment: result is just the RHS value moved to hint
			if rhsValueReg != hint {
				c.emitMove(hint, rhsValueReg, line)
			}

		default:
			return BadRegister, NewCompileError(node, fmt.Sprintf("unsupported assignment operator '%s'", node.Operator))
		}
	}

	// Clean up operation temporary registers
	for _, reg := range operationTempRegs {
		c.regAlloc.Free(reg)
	}

	// --- Store Result Back to LHS ---
	if needsStore {
		switch lhsType {
		case lhsIsIdentifier:
			if identInfo.isGlobal {
				// Global variable assignment
				c.emitSetGlobal(identInfo.globalIdx, hint, line)
			} else if identInfo.isUpvalue {
				c.emitSetUpvalue(identInfo.upvalueIndex, hint, line)
			} else {
				if hint != identInfo.targetReg {
					debugPrintf("// DEBUG Assign Store Ident: Emitting Move R%d <- R%d\n", identInfo.targetReg, hint)
					c.emitMove(identInfo.targetReg, hint, line)
				} else {
					debugPrintf("// DEBUG Assign Store Ident: Skipping Move R%d <- R%d (already inplace)\n", identInfo.targetReg, hint)
				}
			}
		case lhsIsIndexExpr:
			debugPrintf("// DEBUG Assign Store Index: Emitting SetIndex [%d][%d] = R%d\n", indexInfo.arrayReg, indexInfo.indexReg, hint)
			c.emitOpCode(vm.OpSetIndex, line)
			c.emitByte(byte(indexInfo.arrayReg))
			c.emitByte(byte(indexInfo.indexReg))
			c.emitByte(byte(hint))
		case lhsIsMemberExpr:
			debugPrintf("// DEBUG Assign Store Member: Emitting SetProp R%d[%d] = R%d\n", memberInfo.objectReg, memberInfo.nameConstIdx, hint)
			c.emitSetProp(memberInfo.objectReg, hint, memberInfo.nameConstIdx, line)
		}
	} else {
		debugPrintf("// DEBUG Assign Store: Skipped store operation (needsStore=false)\n")
	}

	// --- Final Merge Point & Patching ---
	if jumpPastStore != -1 {
		c.patchJump(jumpPastStore)
	}
	if jumpToEnd != -1 {
		c.patchJump(jumpToEnd)
	}

	if debugAssignment {
		fmt.Printf("// DEBUG Assignment Finalize: hint=R%d\n", hint)
	}

	return hint, nil
}

// compileArrayDestructuringAssignment compiles array destructuring like [a, b, c] = expr
// Desugars into: temp = expr; a = temp[0]; b = temp[1]; c = temp[2];
func (c *Compiler) compileArrayDestructuringAssignment(node *parser.ArrayDestructuringAssignment, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line
	
	if debugAssignment {
		fmt.Printf("// [Assignment] Compiling array destructuring: %s\n", node.String())
	}

	// 1. Compile RHS expression into temp register
	tempReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(tempReg)
	
	_, err := c.compileNode(node.Value, tempReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. For each element, compile: target = temp[index] or target = temp.slice(index)
	for i, element := range node.Elements {
		if element.Target == nil {
			continue // Skip malformed elements
		}

		var valueReg Register
		
		if element.IsRest {
			// Rest element: compile temp.slice(i) to get remaining elements
			valueReg = c.regAlloc.Alloc()
			
			// Call temp.slice(i) to get the rest of the array
			err := c.compileArraySliceCall(tempReg, i, valueReg, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				return BadRegister, err
			}
		} else {
			// Regular element: compile temp[i]
			indexReg := c.regAlloc.Alloc()
			valueReg = c.regAlloc.Alloc()
			
			// Load the index as a constant
			indexConstIdx := c.chunk.AddConstant(vm.Number(float64(i)))
			c.emitLoadConstant(indexReg, indexConstIdx, line)
			
			// Get temp[i] using GetIndex operation
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(valueReg))  // destination register
			c.emitByte(byte(tempReg))   // array register
			c.emitByte(byte(indexReg))  // index register
			
			c.regAlloc.Free(indexReg)
		}
		
		// Handle assignment with potential default value
		if element.Default != nil {
			// Compile conditional assignment: target = valueReg !== undefined ? valueReg : default
			err := c.compileConditionalAssignment(element.Target, valueReg, element.Default, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				return BadRegister, err
			}
		} else {
			// Simple assignment: target = valueReg
			err := c.compileSimpleAssignment(element.Target, valueReg, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				return BadRegister, err
			}
		}
		
		// Clean up temporary registers
		c.regAlloc.Free(valueReg)
	}

	// 3. Return the original RHS value (like regular assignment)
	if hint != tempReg {
		c.emitMove(hint, tempReg, line)
	}
	
	return hint, nil
}

// compileArraySliceCall compiles an array slice operation for rest elements
func (c *Compiler) compileArraySliceCall(arrayReg Register, startIndex int, resultReg Register, line int) errors.PaseratiError {
	// This compiles: resultReg = arrayReg.slice(startIndex) using the specialized OpArraySlice opcode
	
	// Load the start index as a constant
	startIndexReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(startIndexReg)
	
	startConstIdx := c.chunk.AddConstant(vm.Number(float64(startIndex)))
	c.emitLoadConstant(startIndexReg, startConstIdx, line)
	
	// Emit the array slice opcode: OpArraySlice destReg arrayReg startIndexReg
	c.emitOpCode(vm.OpArraySlice, line)
	c.emitByte(byte(resultReg))      // destination register
	c.emitByte(byte(arrayReg))       // source array register
	c.emitByte(byte(startIndexReg))  // start index register
	
	return nil
}

// compileSimpleAssignment handles assignment to a single target (identifier only for Phase 1)
func (c *Compiler) compileSimpleAssignment(target parser.Expression, valueReg Register, line int) errors.PaseratiError {
	// For Phase 1, only support Identifier targets
	identTarget, ok := target.(*parser.Identifier)
	if !ok {
		return NewCompileError(target, "destructuring target must be an identifier")
	}

	// Resolve the identifier to determine how to store it
	symbol, _, found := c.currentSymbolTable.Resolve(identTarget.Value)
	if !found {
		return NewCompileError(identTarget, fmt.Sprintf("undefined variable '%s'", identTarget.Value))
	}

	// Generate appropriate store instruction based on symbol type
	if symbol.IsGlobal {
		c.emitSetGlobal(symbol.GlobalIndex, valueReg, line)
	} else {
		// For local variables, move to the allocated register
		if valueReg != symbol.Register {
			c.emitMove(symbol.Register, valueReg, line)
		}
	}

	return nil
}

// compileObjectDestructuringAssignment compiles object destructuring like {a, b} = expr
// Desugars into: temp = expr; a = temp.a; b = temp.b;
func (c *Compiler) compileObjectDestructuringAssignment(node *parser.ObjectDestructuringAssignment, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line
	
	if debugAssignment {
		fmt.Printf("// [Assignment] Compiling object destructuring: %s\n", node.String())
	}

	// 1. Compile RHS expression into temp register
	tempReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(tempReg)
	
	_, err := c.compileNode(node.Value, tempReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. For each property, compile: target = temp.propertyName
	for _, prop := range node.Properties {
		if prop.Target == nil {
			continue // Skip malformed properties
		}

		// Allocate register for extracted property value
		valueReg := c.regAlloc.Alloc()
		
		// Add property name as a constant
		propNameIdx := c.chunk.AddConstant(vm.String(prop.Key.Value))
		
		// Get temp.propertyName using GetProp operation
		c.emitGetProp(valueReg, tempReg, propNameIdx, line)
		
		// Handle assignment with potential default value
		if prop.Default != nil {
			// Compile conditional assignment: target = valueReg !== undefined ? valueReg : default
			err := c.compileConditionalAssignment(prop.Target, valueReg, prop.Default, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				return BadRegister, err
			}
		} else {
			// Simple assignment: target = valueReg
			err := c.compileSimpleAssignment(prop.Target, valueReg, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				return BadRegister, err
			}
		}
		
		// Clean up temporary register
		c.regAlloc.Free(valueReg)
	}

	// 2.5. Handle rest property if present
	if node.RestProperty != nil {
		err := c.compileObjectRestProperty(tempReg, node.Properties, node.RestProperty, line)
		if err != nil {
			return BadRegister, err
		}
	}

	// 3. Return the original RHS value (like regular assignment)
	if hint != tempReg {
		c.emitMove(hint, tempReg, line)
	}
	
	return hint, nil
}
// compileConditionalAssignment compiles: target = (valueReg !== undefined) ? valueReg : defaultExpr
func (c *Compiler) compileConditionalAssignment(target parser.Expression, valueReg Register, defaultExpr parser.Expression, line int) errors.PaseratiError {
	// This implements: target = valueReg !== undefined ? valueReg : defaultExpr
	
	// 1. Conditional jump: if undefined, jump to default value assignment
	jumpToDefault := c.emitPlaceholderJump(vm.OpJumpIfUndefined, valueReg, line)
	
	// 3. Path 1: Value is not undefined, assign valueReg to target
	err := c.compileSimpleAssignment(target, valueReg, line)
	if err != nil {
		return err
	}
	
	// Jump past the default assignment
	jumpPastDefault := c.emitPlaceholderJump(vm.OpJump, 0, line)
	
	// 4. Path 2: Value is undefined, evaluate and assign default
	c.patchJump(jumpToDefault)
	
	// Compile the default expression
	defaultReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(defaultReg)
	
	_, err = c.compileNode(defaultExpr, defaultReg)
	if err != nil {
		return err
	}
	
	// Assign default value to target
	err = c.compileSimpleAssignment(target, defaultReg, line)
	if err != nil {
		return err
	}
	
	// 5. Patch the jump past default
	c.patchJump(jumpPastDefault)
	
	return nil
}

// compileObjectRestProperty compiles rest property assignment for object destructuring
func (c *Compiler) compileObjectRestProperty(objReg Register, extractedProps []*parser.DestructuringProperty, restElement *parser.DestructuringElement, line int) errors.PaseratiError {
	// Use the new OpCopyObjectExcluding opcode for proper property filtering
	
	// Create array of property names to exclude
	excludeArrayReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(excludeArrayReg)
	
	// Create array with extracted property names
	excludeNames := make([]vm.Value, 0, len(extractedProps))
	for _, prop := range extractedProps {
		if prop.Key != nil {
			excludeNames = append(excludeNames, vm.String(prop.Key.Value))
		}
	}
	
	if len(excludeNames) == 0 {
		// No properties to exclude, just copy the whole object
		c.emitOpCode(vm.OpMakeEmptyObject, line)
		c.emitByte(byte(excludeArrayReg))
		
		// Create result register for the rest object
		restObjReg := c.regAlloc.Alloc()
		defer c.regAlloc.Free(restObjReg)
		
		c.emitMove(restObjReg, objReg, line)
		return c.compileSimpleAssignment(restElement.Target, restObjReg, line)
	}
	
	// Emit code to create the exclude array
	// First allocate contiguous registers for all elements
	startReg := c.regAlloc.Alloc()
	elementRegs := make([]Register, len(excludeNames))
	elementRegs[0] = startReg
	for i := 1; i < len(excludeNames); i++ {
		elementRegs[i] = c.regAlloc.Alloc()
		defer c.regAlloc.Free(elementRegs[i])
	}
	defer c.regAlloc.Free(startReg)
	
	// Load each string constant into consecutive registers
	for i, name := range excludeNames {
		nameConstIdx := c.chunk.AddConstant(name)
		c.emitLoadConstant(elementRegs[i], nameConstIdx, line)
	}
	
	// Create array from the element registers
	c.emitOpCode(vm.OpMakeArray, line)
	c.emitByte(byte(excludeArrayReg))      // destination register
	c.emitByte(byte(startReg))             // start register (first element)
	c.emitByte(byte(len(excludeNames)))    // element count
	
	// Create result register for the rest object
	restObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(restObjReg)
	
	// Use the new opcode: restObj = copyObjectExcluding(sourceObj, excludeArray)
	c.emitOpCode(vm.OpCopyObjectExcluding, line)
	c.emitByte(byte(restObjReg))     // destination register
	c.emitByte(byte(objReg))         // source object register
	c.emitByte(byte(excludeArrayReg)) // exclude array register
	
	// Assign rest object to the rest property target
	return c.compileSimpleAssignment(restElement.Target, restObjReg, line)
}

// compileObjectRestDeclaration compiles rest property declaration for object destructuring
func (c *Compiler) compileObjectRestDeclaration(objReg Register, extractedProps []*parser.DestructuringProperty, varName string, isConst bool, line int) errors.PaseratiError {
	// Create array of property names to exclude
	excludeArrayReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(excludeArrayReg)
	
	// Create array with extracted property names
	excludeNames := make([]vm.Value, 0, len(extractedProps))
	for _, prop := range extractedProps {
		if prop.Key != nil {
			excludeNames = append(excludeNames, vm.String(prop.Key.Value))
		}
	}
	
	if len(excludeNames) == 0 {
		// No properties to exclude, just copy the whole object
		c.emitOpCode(vm.OpMakeEmptyObject, line)
		c.emitByte(byte(excludeArrayReg))
		
		// Create result register for the rest object
		restObjReg := c.regAlloc.Alloc()
		defer c.regAlloc.Free(restObjReg)
		
		c.emitMove(restObjReg, objReg, line)
		return c.defineDestructuredVariableWithValue(varName, isConst, restObjReg, line)
	}
	
	// Emit code to create the exclude array
	// First allocate contiguous registers for all elements
	startReg := c.regAlloc.Alloc()
	elementRegs := make([]Register, len(excludeNames))
	elementRegs[0] = startReg
	for i := 1; i < len(excludeNames); i++ {
		elementRegs[i] = c.regAlloc.Alloc()
		defer c.regAlloc.Free(elementRegs[i])
	}
	defer c.regAlloc.Free(startReg)
	
	// Load each string constant into consecutive registers
	for i, name := range excludeNames {
		nameConstIdx := c.chunk.AddConstant(name)
		c.emitLoadConstant(elementRegs[i], nameConstIdx, line)
	}
	
	// Create array from the element registers
	c.emitOpCode(vm.OpMakeArray, line)
	c.emitByte(byte(excludeArrayReg))      // destination register
	c.emitByte(byte(startReg))             // start register (first element)
	c.emitByte(byte(len(excludeNames)))    // element count
	
	// Create result register for the rest object
	restObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(restObjReg)
	
	// Use the new opcode: restObj = copyObjectExcluding(sourceObj, excludeArray)
	c.emitOpCode(vm.OpCopyObjectExcluding, line)
	c.emitByte(byte(restObjReg))     // destination register
	c.emitByte(byte(objReg))         // source object register
	c.emitByte(byte(excludeArrayReg)) // exclude array register
	
	// Define the rest variable with the rest object
	return c.defineDestructuredVariableWithValue(varName, isConst, restObjReg, line)
}

// compileArrayDestructuringDeclaration compiles let/const [a, b] = expr declarations
func (c *Compiler) compileArrayDestructuringDeclaration(node *parser.ArrayDestructuringDeclaration, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line
	
	if debugAssignment {
		fmt.Printf("// [Assignment] Compiling array destructuring declaration: %s\n", node.String())
	}

	// If no initializer, assign undefined to all variables
	if node.Value == nil {
		for _, element := range node.Elements {
			if element.Target == nil {
				continue
			}
			
			if ident, ok := element.Target.(*parser.Identifier); ok {
				// Define variable with undefined value
				err := c.defineDestructuredVariable(ident.Value, node.IsConst, types.Undefined, line)
				if err != nil {
					return BadRegister, err
				}
			}
		}
		return BadRegister, nil
	}

	// 1. Compile RHS expression into temp register
	tempReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(tempReg)
	
	_, err := c.compileNode(node.Value, tempReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. For each element, compile: define target = temp[index]
	for i, element := range node.Elements {
		if element.Target == nil {
			continue // Skip malformed elements
		}

		// Only support identifier targets for now
		ident, ok := element.Target.(*parser.Identifier)
		if !ok {
			return BadRegister, NewCompileError(element.Target, "destructuring declaration target must be an identifier")
		}

		var valueReg Register
		
		if element.IsRest {
			// Rest element: compile temp.slice(i) to get remaining elements
			valueReg = c.regAlloc.Alloc()
			
			// Call temp.slice(i) to get the rest of the array
			err := c.compileArraySliceCall(tempReg, i, valueReg, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				return BadRegister, err
			}
		} else {
			// Regular element: compile temp[i]
			indexReg := c.regAlloc.Alloc()
			valueReg = c.regAlloc.Alloc()
			
			// Load the index as a constant
			indexConstIdx := c.chunk.AddConstant(vm.Number(float64(i)))
			c.emitLoadConstant(indexReg, indexConstIdx, line)
			
			// Get temp[i] using GetIndex operation
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(valueReg))  // destination register
			c.emitByte(byte(tempReg))   // array register
			c.emitByte(byte(indexReg))  // index register
			
			c.regAlloc.Free(indexReg)
		}
		
		// Handle default value if present
		if element.Default != nil {
			// Compile conditional assignment with default
			defaultReg := c.regAlloc.Alloc()
			_, err := c.compileNode(element.Default, defaultReg)
			if err != nil {
				c.regAlloc.Free(valueReg)
				c.regAlloc.Free(defaultReg)
				return BadRegister, err
			}
			
			// Jump to default if valueReg is undefined
			jumpToDefault := c.emitPlaceholderJump(vm.OpJumpIfUndefined, valueReg, line)
			
			// Define variable with extracted value
			err = c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, valueReg, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				c.regAlloc.Free(defaultReg)
				return BadRegister, err
			}
			
			// Jump past default
			jumpPastDefault := c.emitPlaceholderJump(vm.OpJump, 0, line)
			
			// Patch jump to default
			c.patchJump(jumpToDefault)
			
			// Define variable with default value
			err = c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, defaultReg, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				c.regAlloc.Free(defaultReg)
				return BadRegister, err
			}
			
			// Patch jump past default
			c.patchJump(jumpPastDefault)
			
			c.regAlloc.Free(defaultReg)
		} else {
			// Define variable with extracted value
			err := c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, valueReg, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				return BadRegister, err
			}
		}
		
		// Clean up temporary registers
		c.regAlloc.Free(valueReg)
	}
	
	return BadRegister, nil
}

// compileObjectDestructuringDeclaration compiles let/const {a, b} = expr declarations
func (c *Compiler) compileObjectDestructuringDeclaration(node *parser.ObjectDestructuringDeclaration, hint Register) (Register, errors.PaseratiError) {
	line := node.Token.Line
	
	if debugAssignment {
		fmt.Printf("// [Assignment] Compiling object destructuring declaration: %s\n", node.String())
	}

	// If no initializer, assign undefined to all variables
	if node.Value == nil {
		for _, prop := range node.Properties {
			if prop.Target == nil {
				continue
			}
			
			if ident, ok := prop.Target.(*parser.Identifier); ok {
				// Define variable with undefined value
				err := c.defineDestructuredVariable(ident.Value, node.IsConst, types.Undefined, line)
				if err != nil {
					return BadRegister, err
				}
			}
		}
		
		// Handle rest property without initializer
		if node.RestProperty != nil {
			if ident, ok := node.RestProperty.Target.(*parser.Identifier); ok {
				err := c.defineDestructuredVariable(ident.Value, node.IsConst, types.Undefined, line)
				if err != nil {
					return BadRegister, err
				}
			}
		}
		
		return BadRegister, nil
	}

	// 1. Compile RHS expression into temp register
	tempReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(tempReg)
	
	_, err := c.compileNode(node.Value, tempReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. For each property, compile: define target = temp.property
	for _, prop := range node.Properties {
		if prop.Key == nil || prop.Target == nil {
			continue // Skip malformed properties
		}

		// Only support identifier targets for now
		ident, ok := prop.Target.(*parser.Identifier)
		if !ok {
			return BadRegister, NewCompileError(prop.Target, "destructuring declaration target must be an identifier")
		}

		// Allocate register for extracted value
		valueReg := c.regAlloc.Alloc()
		
		// Get property from object
		propNameIdx := c.chunk.AddConstant(vm.String(prop.Key.Value))
		c.emitOpCode(vm.OpGetProp, line)
		c.emitByte(byte(valueReg))   // destination register
		c.emitByte(byte(tempReg))    // object register
		c.emitUint16(propNameIdx)    // property name constant index
		
		// Handle default value if present
		if prop.Default != nil {
			// Compile conditional assignment with default
			defaultReg := c.regAlloc.Alloc()
			_, err := c.compileNode(prop.Default, defaultReg)
			if err != nil {
				c.regAlloc.Free(valueReg)
				c.regAlloc.Free(defaultReg)
				return BadRegister, err
			}
			
			// Jump to default if valueReg is undefined
			jumpToDefault := c.emitPlaceholderJump(vm.OpJumpIfUndefined, valueReg, line)
			
			// Define variable with extracted value
			err = c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, valueReg, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				c.regAlloc.Free(defaultReg)
				return BadRegister, err
			}
			
			// Jump past default
			jumpPastDefault := c.emitPlaceholderJump(vm.OpJump, 0, line)
			
			// Patch jump to default
			c.patchJump(jumpToDefault)
			
			// Define variable with default value
			err = c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, defaultReg, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				c.regAlloc.Free(defaultReg)
				return BadRegister, err
			}
			
			// Patch jump past default
			c.patchJump(jumpPastDefault)
			
			c.regAlloc.Free(defaultReg)
		} else {
			// Define variable with extracted value
			err := c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, valueReg, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				return BadRegister, err
			}
		}
		
		// Clean up temporary register
		c.regAlloc.Free(valueReg)
	}
	
	// Handle rest property if present
	if node.RestProperty != nil {
		if ident, ok := node.RestProperty.Target.(*parser.Identifier); ok {
			// Create rest object with remaining properties
			err := c.compileObjectRestDeclaration(tempReg, node.Properties, ident.Value, node.IsConst, line)
			if err != nil {
				return BadRegister, err
			}
		}
	}
	
	return BadRegister, nil
}

// defineDestructuredVariable defines a new variable from destructuring (without value)
func (c *Compiler) defineDestructuredVariable(name string, isConst bool, valueType types.Type, line int) errors.PaseratiError {
	undefReg := c.regAlloc.Alloc()
	c.emitLoadUndefined(undefReg, line)
	
	err := c.defineDestructuredVariableWithValue(name, isConst, undefReg, line)
	if err != nil {
		c.regAlloc.Free(undefReg)
		return err
	}
	
	// Pin the register for local variables
	if c.enclosing != nil {
		c.regAlloc.Pin(undefReg)
	}
	
	return nil
}

// defineDestructuredVariableWithValue defines a new variable from destructuring with a specific value
func (c *Compiler) defineDestructuredVariableWithValue(name string, isConst bool, valueReg Register, line int) errors.PaseratiError {
	if c.enclosing == nil {
		// Top-level: use global variable
		globalIdx := c.GetOrAssignGlobalIndex(name)
		c.emitSetGlobal(globalIdx, valueReg, line)
		c.currentSymbolTable.DefineGlobal(name, globalIdx)
	} else {
		// Function scope: use local symbol table
		c.currentSymbolTable.Define(name, valueReg)
		// Pin the register since local variables can be captured by upvalues
		c.regAlloc.Pin(valueReg)
	}
	
	return nil
}
package compiler

import (
	"fmt"

	"paserati/pkg/errors"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)


const debugAssignment = false // Enable debug output for assignment compilation

// shouldUseWithProperty checks if an identifier should be treated as a with property
// This is an unobtrusive predicate that can be used throughout the compiler
func (c *Compiler) shouldUseWithProperty(ident *parser.Identifier) (Register, bool) {
	// Only check for with properties if we're actually inside a with statement
	// This prevents unnecessary constant pool corruption when no with statements are active
	if !c.currentSymbolTable.HasActiveWithObjects() {
		return BadRegister, false
	}
	
	debugPrintf("// DEBUG shouldUseWithProperty: Has active with objects, checking '%s'\n", ident.Value)
	
	// Check if this identifier is flagged as coming from a with object (by type checker)
	if ident.IsFromWith {
		debugPrintf("// DEBUG shouldUseWithProperty: '%s' is flagged as from with, checking resolution\n", ident.Value)
		if objReg, withFound := c.currentSymbolTable.ResolveWithProperty(ident.Value); withFound {
			debugPrintf("// DEBUG shouldUseWithProperty: '%s' found in with object, returning true\n", ident.Value)
			return objReg, true
		}
		debugPrintf("// DEBUG shouldUseWithProperty: '%s' not found in with object resolution\n", ident.Value)
	} else {
		debugPrintf("// DEBUG shouldUseWithProperty: '%s' not flagged as from with\n", ident.Value)
	}
	return BadRegister, false
}

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
		objectReg         Register
		nameConstIdx      uint16  // For static properties
		isComputed        bool    // True if this is a computed property
		keyReg           Register // For computed properties
		isPrivateField    bool    // True if this is a private field (#field)
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
		
		// First check if this is a with property (highest priority)
		if objReg, isWithProperty := c.shouldUseWithProperty(lhsNode); isWithProperty {
			// This is a with property assignment - treat as member assignment
			lhsType = lhsIsMemberExpr
			memberInfo.objectReg = objReg
			memberInfo.nameConstIdx = c.chunk.AddConstant(vm.String(lhsNode.Value))
			memberInfo.isComputed = false
			
			// For compound assignments, we need the current property value
			if node.Operator != "=" {
				currentValueReg = c.regAlloc.Alloc()
				tempRegs = append(tempRegs, currentValueReg)
				c.emitGetProp(currentValueReg, memberInfo.objectReg, memberInfo.nameConstIdx, line)
			} else {
				currentValueReg = nilRegister // Not needed for simple assignment
			}
		} else {
			// Regular identifier - resolve the identifier
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
				// Local variable (including variables from immediate parent scope in same function)
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

		// Check if this is a computed property
		if computedKey, ok := lhsNode.Property.(*parser.ComputedPropertyName); ok {
			// This is a computed property: obj[expr] = value
			memberInfo.isComputed = true
			memberInfo.keyReg = c.regAlloc.Alloc()
			tempRegs = append(tempRegs, memberInfo.keyReg)
			_, err := c.compileNode(computedKey.Expr, memberInfo.keyReg)
			if err != nil {
				return BadRegister, err
			}
		} else {
			// Regular property access: obj.prop = value
			memberInfo.isComputed = false
			propName := c.extractPropertyName(lhsNode.Property)

			// Check for private field (starts with #)
			if len(propName) > 0 && propName[0] == '#' {
				// Private field - store the field name without # prefix
				fieldName := propName[1:]
				memberInfo.nameConstIdx = c.chunk.AddConstant(vm.String(fieldName))
				memberInfo.isPrivateField = true
			} else {
				// Regular property
				memberInfo.nameConstIdx = c.chunk.AddConstant(vm.String(propName))
				memberInfo.isPrivateField = false
			}
		}

		// If compound or logical assignment, load the current property value
		if node.Operator != "=" {
			if memberInfo.isComputed {
				// For computed properties, always use OpGetIndex since we can't know property name at compile time
				currentValueReg = c.regAlloc.Alloc()
				tempRegs = append(tempRegs, currentValueReg)
				c.emitOpCode(vm.OpGetIndex, line)
				c.emitByte(byte(currentValueReg))       // Destination register
				c.emitByte(byte(memberInfo.objectReg))  // Object register
				c.emitByte(byte(memberInfo.keyReg))     // Key register (computed at runtime)
			} else if memberInfo.isPrivateField {
				// For private fields, use OpGetPrivateField
				currentValueReg = c.regAlloc.Alloc()
				tempRegs = append(tempRegs, currentValueReg)
				c.emitGetPrivateField(currentValueReg, memberInfo.objectReg, memberInfo.nameConstIdx, line)
			} else {
				// For static properties, use OpGetProp which handles getters automatically
				currentValueReg = c.regAlloc.Alloc()
				tempRegs = append(tempRegs, currentValueReg)
				c.emitGetProp(currentValueReg, memberInfo.objectReg, memberInfo.nameConstIdx, line)
			}
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
		var rhsValueReg Register

		var jumpToEvalRhs int = -1
		var jumpToShortCircuit int = -1

		switch node.Operator {
		case "&&=":
			// If FALSEY -> jumpToShortCircuit (skip RHS eval AND store)
			jumpToShortCircuit = c.emitPlaceholderJump(vm.OpJumpIfFalse, currentValueReg, line)
		case "||=":
			// If FALSEY -> jumpToEvalRhs
			jumpToEvalRhs = c.emitPlaceholderJump(vm.OpJumpIfFalse, currentValueReg, line)
			// If TRUTHY -> jumpToShortCircuit (skip RHS eval AND store)
			jumpToShortCircuit = c.emitPlaceholderJump(vm.OpJump, 0, line)
		case "??=":
			// Use efficient nullish check opcode
			isNullishReg := c.regAlloc.Alloc()
			operationTempRegs = append(operationTempRegs, isNullishReg)
			c.emitIsNullish(isNullishReg, currentValueReg, line)
			// If NOT nullish -> jumpToShortCircuit (skip RHS eval AND store)
			jumpToShortCircuit = c.emitPlaceholderJump(vm.OpJumpIfFalse, isNullishReg, line)
		}

		// --- Evaluate RHS Path ---
		if jumpToEvalRhs != -1 {
			c.patchJump(jumpToEvalRhs)
		}
		// This block is reached if short-circuit didn't happen
		rhsValueReg = c.regAlloc.Alloc()
		operationTempRegs = append(operationTempRegs, rhsValueReg)
		_, err := c.compileNode(node.Value, rhsValueReg)
		if err != nil {
			return BadRegister, err
		}
		debugPrintf("// DEBUG Assign Logical RHS: Evaluated RHS. rhsValueReg=R%d\n", rhsValueReg)

		// Move RHS result to hint register for final result
		if rhsValueReg != hint {
			c.emitMove(hint, rhsValueReg, line)
		}

		// Store hint to LHS (inline the store logic for RHS path)
		switch lhsType {
		case lhsIsIdentifier:
			if identInfo.isGlobal {
				c.emitSetGlobal(identInfo.globalIdx, hint, line)
			} else if identInfo.isUpvalue {
				c.emitSetUpvalue(identInfo.upvalueIndex, hint, line)
			} else {
				if hint != identInfo.targetReg {
					c.emitMove(identInfo.targetReg, hint, line)
				}
			}
		case lhsIsIndexExpr:
			c.emitOpCode(vm.OpSetIndex, line)
			c.emitByte(byte(indexInfo.arrayReg))
			c.emitByte(byte(indexInfo.indexReg))
			c.emitByte(byte(hint))
		case lhsIsMemberExpr:
			if memberInfo.isComputed {
				c.emitOpCode(vm.OpSetIndex, line)
				c.emitByte(byte(memberInfo.objectReg))
				c.emitByte(byte(memberInfo.keyReg))
				c.emitByte(byte(hint))
			} else if memberInfo.isPrivateField {
				c.emitSetPrivateField(memberInfo.objectReg, hint, memberInfo.nameConstIdx, line)
			} else {
				c.emitSetProp(memberInfo.objectReg, hint, memberInfo.nameConstIdx, line)
			}
		}

		// Jump past short-circuit path
		jumpToEnd = c.emitPlaceholderJump(vm.OpJump, 0, line)

		// --- Short-circuit path (currentValue is already the result) ---
		c.patchJump(jumpToShortCircuit)
		// Move current value to hint register for final result
		if currentValueReg != hint {
			c.emitMove(hint, currentValueReg, line)
		}
		debugPrintf("// DEBUG Assign Logical ShortCircuit: skipped store\n")
		// Short-circuit path doesn't store, just returns current value in hint
		// needsStore will be set to false below to skip the store logic

		needsStore = false // Skip the store logic below (we already handled both paths)

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
			// OpSetProp now properly handles accessor properties via OpDefineAccessor
			if memberInfo.isComputed {
				// Use OpSetIndex for computed properties: objectReg[keyReg] = hint
				debugPrintf("// DEBUG Assign Store Member: Emitting SetIndex R%d[R%d] = R%d\n", memberInfo.objectReg, memberInfo.keyReg, hint)
				c.emitOpCode(vm.OpSetIndex, line)
				c.emitByte(byte(memberInfo.objectReg)) // Object register
				c.emitByte(byte(memberInfo.keyReg))    // Key register (computed at runtime)
				c.emitByte(byte(hint))                 // Value register
			} else if memberInfo.isPrivateField {
				// Use OpSetPrivateField for private fields: objectReg.#field = hint
				debugPrintf("// DEBUG Assign Store Private Field: Emitting SetPrivateField R%d[%d] = R%d\n", memberInfo.objectReg, memberInfo.nameConstIdx, hint)
				c.emitSetPrivateField(memberInfo.objectReg, hint, memberInfo.nameConstIdx, line)
			} else {
				// Use OpSetProp for static properties: objectReg.nameConstIdx = hint
				// OpSetProp will invoke setters if the property is an accessor
				debugPrintf("// DEBUG Assign Store Member: Emitting SetProp R%d[%d] = R%d\n", memberInfo.objectReg, memberInfo.nameConstIdx, hint)
				c.emitSetProp(memberInfo.objectReg, hint, memberInfo.nameConstIdx, line)
			}
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

// compileSimpleAssignment handles assignment to a single target (supports nested patterns)
func (c *Compiler) compileSimpleAssignment(target parser.Expression, valueReg Register, line int) errors.PaseratiError {
	return c.compileRecursiveAssignment(target, valueReg, line)
}

// compileRecursiveAssignment handles assignment to various target types including nested patterns
func (c *Compiler) compileRecursiveAssignment(target parser.Expression, valueReg Register, line int) errors.PaseratiError {
	switch targetNode := target.(type) {
	case *parser.Identifier:
		// Simple variable assignment: a = value
		return c.compileIdentifierAssignment(targetNode, valueReg, line)
		
	case *parser.ArrayLiteral:
		// Nested array destructuring: [a, b] = value
		return c.compileNestedArrayDestructuring(targetNode, valueReg, line)
		
	case *parser.ObjectLiteral:
		// Nested object destructuring: {a, b} = value
		return c.compileNestedObjectDestructuring(targetNode, valueReg, line)
		
	default:
		return NewCompileError(target, fmt.Sprintf("unsupported destructuring target type: %T", target))
	}
}

// compileIdentifierAssignment handles simple variable assignment
func (c *Compiler) compileIdentifierAssignment(identTarget *parser.Identifier, valueReg Register, line int) errors.PaseratiError {
	// First check if this is from a with object (flagged by type checker)
	if identTarget.IsFromWith {
		if objReg, withFound := c.currentSymbolTable.ResolveWithProperty(identTarget.Value); withFound {
			// Emit property assignment bytecode: objReg[identTarget.Value] = valueReg
			propName := c.chunk.AddConstant(vm.String(identTarget.Value))
			c.emitSetProp(objReg, valueReg, propName, line)
			return nil
		}
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

// compileNestedArrayDestructuring compiles nested array patterns like [a, [b, c]] = value
func (c *Compiler) compileNestedArrayDestructuring(arrayLit *parser.ArrayLiteral, valueReg Register, line int) errors.PaseratiError {
	// Convert ArrayLiteral to ArrayDestructuringAssignment for reuse of existing logic
	destructureAssign := &parser.ArrayDestructuringAssignment{
		Token: arrayLit.Token,
		Value: nil, // Will not be used since we already have the valueReg
	}
	
	// Convert array elements to destructuring elements
	for i, element := range arrayLit.Elements {
		var target parser.Expression
		var defaultValue parser.Expression
		var isRest bool
		
		// Check if this element is a rest element (...rest)
		if spreadExpr, ok := element.(*parser.SpreadElement); ok {
			// This is a rest element: [...rest]
			target = spreadExpr.Argument
			defaultValue = nil
			isRest = true
		} else if assignExpr, ok := element.(*parser.AssignmentExpression); ok && assignExpr.Operator == "=" {
			// This is a default value: [a = 5]
			target = assignExpr.Left
			defaultValue = assignExpr.Value
			isRest = false
		} else {
			// This is a simple element: [a] or nested pattern: [[a, b]]
			target = element
			defaultValue = nil
			isRest = false
		}
		
		destElement := &parser.DestructuringElement{
			Target:  target,
			Default: defaultValue,
			IsRest:  isRest,
		}
		
		// Validate rest element placement
		if isRest && i != len(arrayLit.Elements)-1 {
			return NewCompileError(arrayLit, "rest element must be last element in destructuring pattern")
		}
		
		destructureAssign.Elements = append(destructureAssign.Elements, destElement)
	}
	
	// Reuse existing compilation logic but with direct value register
	return c.compileArrayDestructuringWithValueReg(destructureAssign, valueReg, line)
}

// compileNestedObjectDestructuring compiles nested object patterns like {user: {name, age}} = value
func (c *Compiler) compileNestedObjectDestructuring(objectLit *parser.ObjectLiteral, valueReg Register, line int) errors.PaseratiError {
	// Convert ObjectLiteral to ObjectDestructuringAssignment for reuse of existing logic
	destructureAssign := &parser.ObjectDestructuringAssignment{
		Token: objectLit.Token,
		Value: nil, // Will not be used since we already have the valueReg
	}
	
	// Convert object properties to destructuring properties
	for _, pair := range objectLit.Properties {
		// The key should be an identifier for simple property access
		keyIdent, ok := pair.Key.(*parser.Identifier)
		if !ok {
			return NewCompileError(objectLit, fmt.Sprintf("invalid destructuring property key: %s (only simple identifiers supported)", pair.Key.String()))
		}
		
		var target parser.Expression
		var defaultValue parser.Expression
		
		// Check for different patterns:
		// 1. {name} - shorthand without default
		// 2. {name = defaultVal} - shorthand with default (value is assignment expr)
		// 3. {name: localVar} - explicit target without default
		// 4. {name: localVar = defaultVal} - explicit target with default
		// 5. {name: [a, b]} - nested pattern target (NEW)
		// 6. {name: {x, y}} - nested pattern target (NEW)
		
		if valueIdent, ok := pair.Value.(*parser.Identifier); ok && valueIdent.Value == keyIdent.Value {
			// Pattern 1: Shorthand without default {name}
			target = keyIdent
			defaultValue = nil
		} else if assignExpr, ok := pair.Value.(*parser.AssignmentExpression); ok && assignExpr.Operator == "=" {
			// Check if this is shorthand with default or explicit with default
			if leftIdent, ok := assignExpr.Left.(*parser.Identifier); ok && leftIdent.Value == keyIdent.Value {
				// Pattern 2: Shorthand with default {name = defaultVal}
				target = keyIdent
				defaultValue = assignExpr.Value
			} else {
				// Pattern 4: Explicit target with default {name: localVar = defaultVal}
				target = assignExpr.Left
				defaultValue = assignExpr.Value
			}
		} else {
			// Pattern 3, 5, 6: Explicit target without default {name: localVar} or {name: [a, b]} or {name: {x, y}}
			target = pair.Value
			defaultValue = nil
		}
		
		destProperty := &parser.DestructuringProperty{
			Key:     keyIdent,
			Target:  target,
			Default: defaultValue,
		}
		
		destructureAssign.Properties = append(destructureAssign.Properties, destProperty)
	}
	
	// Reuse existing compilation logic but with direct value register
	return c.compileObjectDestructuringWithValueReg(destructureAssign, valueReg, line)
}

// compileArrayDestructuringWithValueReg compiles array destructuring using an existing value register
func (c *Compiler) compileArrayDestructuringWithValueReg(node *parser.ArrayDestructuringAssignment, valueReg Register, line int) errors.PaseratiError {
	// Reuse existing array destructuring logic but skip RHS compilation
	// 2. For each element, compile: target = valueReg[index] or target = valueReg.slice(index)
	for i, element := range node.Elements {
		if element.Target == nil {
			continue // Skip malformed elements
		}

		var extractedReg Register
		
		if element.IsRest {
			// Rest element: compile valueReg.slice(i) to get remaining elements
			extractedReg = c.regAlloc.Alloc()
			
			// Call valueReg.slice(i) to get the rest of the array
			err := c.compileArraySliceCall(valueReg, i, extractedReg, line)
			if err != nil {
				c.regAlloc.Free(extractedReg)
				return err
			}
		} else {
			// Regular element: compile valueReg[i]
			indexReg := c.regAlloc.Alloc()
			extractedReg = c.regAlloc.Alloc()
			
			// Load the index as a constant
			indexConstIdx := c.chunk.AddConstant(vm.Number(float64(i)))
			c.emitLoadConstant(indexReg, indexConstIdx, line)
			
			// Get valueReg[i] using GetIndex operation
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(extractedReg)) // destination register
			c.emitByte(byte(valueReg))     // array register
			c.emitByte(byte(indexReg))     // index register
			
			c.regAlloc.Free(indexReg)
		}
		
		// Handle assignment with potential default value (recursive assignment)
		if element.Default != nil {
			// Compile conditional assignment: target = extractedReg !== undefined ? extractedReg : default
			err := c.compileConditionalAssignment(element.Target, extractedReg, element.Default, line)
			if err != nil {
				c.regAlloc.Free(extractedReg)
				return err
			}
		} else {
			// Recursive assignment: target = extractedReg (may be nested pattern)
			err := c.compileRecursiveAssignment(element.Target, extractedReg, line)
			if err != nil {
				c.regAlloc.Free(extractedReg)
				return err
			}
		}
		
		// Clean up temporary registers
		c.regAlloc.Free(extractedReg)
	}
	
	return nil
}

// compileObjectDestructuringWithValueReg compiles object destructuring using an existing value register
func (c *Compiler) compileObjectDestructuringWithValueReg(node *parser.ObjectDestructuringAssignment, valueReg Register, line int) errors.PaseratiError {
	// Reuse existing object destructuring logic but skip RHS compilation
	// 2. For each property, compile: target = valueReg.propertyName
	for _, prop := range node.Properties {
		if prop.Target == nil {
			continue // Skip malformed properties
		}

		// Allocate register for extracted property value
		extractedReg := c.regAlloc.Alloc()

		// Handle property access (identifier or computed)
		if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
			// Static property key
			propNameIdx := c.chunk.AddConstant(vm.String(keyIdent.Value))
			c.emitGetProp(extractedReg, valueReg, propNameIdx, line)
		} else if computed, ok := prop.Key.(*parser.ComputedPropertyName); ok {
			// Computed property key - evaluate expression
			keyReg := c.regAlloc.Alloc()
			_, err := c.compileNode(computed.Expr, keyReg)
			if err != nil {
				c.regAlloc.Free(extractedReg)
				c.regAlloc.Free(keyReg)
				return err
			}
			// Use GetIndex for dynamic property access
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(extractedReg)) // Destination
			c.emitByte(byte(valueReg))     // Object
			c.emitByte(byte(keyReg))       // Key
			c.regAlloc.Free(keyReg)
		}
		
		// Handle assignment with potential default value (recursive assignment)
		if prop.Default != nil {
			// Compile conditional assignment: target = extractedReg !== undefined ? extractedReg : default
			err := c.compileConditionalAssignment(prop.Target, extractedReg, prop.Default, line)
			if err != nil {
				c.regAlloc.Free(extractedReg)
				return err
			}
		} else {
			// Recursive assignment: target = extractedReg (may be nested pattern)
			err := c.compileRecursiveAssignment(prop.Target, extractedReg, line)
			if err != nil {
				c.regAlloc.Free(extractedReg)
				return err
			}
		}
		
		// Clean up temporary register
		c.regAlloc.Free(extractedReg)
	}
	
	// TODO: Handle rest property if present (for future enhancement)
	
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

		// Handle property access (identifier or computed)
		if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
			propNameIdx := c.chunk.AddConstant(vm.String(keyIdent.Value))
			c.emitGetProp(valueReg, tempReg, propNameIdx, line)
		} else if computed, ok := prop.Key.(*parser.ComputedPropertyName); ok {
			keyReg := c.regAlloc.Alloc()
			_, err := c.compileNode(computed.Expr, keyReg)
			if err != nil {
				c.regAlloc.Free(valueReg)
				c.regAlloc.Free(keyReg)
				return BadRegister, err
			}
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(valueReg))
			c.emitByte(byte(tempReg))
			c.emitByte(byte(keyReg))
			c.regAlloc.Free(keyReg)
		}
		
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
		// CRITICAL: Must patch jump before returning on error!
		c.patchJump(jumpToDefault)
		return err
	}
	
	// Jump past the default assignment
	jumpPastDefault := c.emitPlaceholderJump(vm.OpJump, 0, line)
	
	// 4. Path 2: Value is undefined, evaluate and assign default
	c.patchJump(jumpToDefault)

	// Compile the default expression
	defaultReg := c.regAlloc.Alloc()
	// NOTE: Don't use defer here! When called in a loop (array destructuring),
	// defer would accumulate registers. We'll free it manually at the end.

	// Check if we should apply function name inference
	// Per ECMAScript spec: if target is an identifier and default is anonymous function, use target name
	var nameHint string
	if ident, ok := target.(*parser.Identifier); ok {
		nameHint = ident.Value
	}

	// Compile default with potential name hint for anonymous functions
	if nameHint != "" {
		if funcLit, ok := defaultExpr.(*parser.FunctionLiteral); ok && funcLit.Name == nil {
			// Anonymous function literal - use target name
			funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, nameHint)
			if err != nil {
				c.patchJump(jumpPastDefault)
				c.regAlloc.Free(defaultReg)
				return err
			}
			c.emitClosure(defaultReg, funcConstIndex, funcLit, freeSymbols)
		} else if classExpr, ok := defaultExpr.(*parser.ClassExpression); ok && classExpr.Name == nil {
			// Anonymous class expression - give it the target name temporarily
			// This allows function name inference per ECMAScript spec
			classExpr.Name = &parser.Identifier{
				Token: classExpr.Token,
				Value: nameHint,
			}
			_, err = c.compileNode(classExpr, defaultReg)
			// Restore to anonymous (though it doesn't matter since we're done compiling)
			classExpr.Name = nil
			if err != nil {
				c.patchJump(jumpPastDefault)
				c.regAlloc.Free(defaultReg)
				return err
			}
		} else if arrowFunc, ok := defaultExpr.(*parser.ArrowFunctionLiteral); ok {
			// Arrow function - compile with name hint by using compileArrowFunctionWithName
			funcConstIndex, freeSymbols, err := c.compileArrowFunctionWithName(arrowFunc, nameHint)
			if err != nil {
				c.patchJump(jumpPastDefault)
				c.regAlloc.Free(defaultReg)
				return err
			}
			// Create a minimal FunctionLiteral for emitClosure
			// Arrow function body can be an expression or block, wrap it appropriately
			var body *parser.BlockStatement
			if blockBody, ok := arrowFunc.Body.(*parser.BlockStatement); ok {
				body = blockBody
			} else {
				// Expression body - wrap in block for emitClosure
				body = &parser.BlockStatement{}
			}
			minimalFuncLit := &parser.FunctionLiteral{Body: body}
			c.emitClosure(defaultReg, funcConstIndex, minimalFuncLit, freeSymbols)
		} else {
			// Not a function, compile normally
			_, err = c.compileNode(defaultExpr, defaultReg)
			if err != nil {
				c.patchJump(jumpPastDefault)
				c.regAlloc.Free(defaultReg)
				return err
			}
		}
	} else {
		_, err = c.compileNode(defaultExpr, defaultReg)
		if err != nil {
			c.patchJump(jumpPastDefault)
			c.regAlloc.Free(defaultReg)
			return err
		}
	}

	// Assign default value to target
	err = c.compileSimpleAssignment(target, defaultReg, line)
	if err != nil {
		c.patchJump(jumpPastDefault)
		c.regAlloc.Free(defaultReg)
		return err
	}
	
	// 5. Patch the jump past default
	c.patchJump(jumpPastDefault)

	// Free the default register now that we're done with it
	c.regAlloc.Free(defaultReg)

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
		if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
			excludeNames = append(excludeNames, vm.String(keyIdent.Value))
		}
		// Skip computed properties (can't exclude statically)
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
		if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
			excludeNames = append(excludeNames, vm.String(keyIdent.Value))
			if debugAssignment {
				fmt.Printf("// [ObjectRest] Excluding property: %s\n", keyIdent.Value)
			}
		}
		// Skip computed properties (can't exclude statically)
	}
	if debugAssignment {
		fmt.Printf("// [ObjectRest] Total excludeNames: %d\n", len(excludeNames))
	}
	
	// Always use OpCopyObjectExcluding to ensure we only copy enumerable properties
	// Even when excludeNames is empty, we still need to filter out non-enumerable properties
	if len(excludeNames) == 0 {
		// Create empty array for exclude list
		c.emitOpCode(vm.OpMakeArray, line)
		c.emitByte(byte(excludeArrayReg))
		c.emitByte(0) // start register (unused for count=0)
		c.emitByte(0) // count: 0 elements
	} else {
		// Emit code to create the exclude array
		// Allocate contiguous registers for all array elements
		startReg := c.regAlloc.AllocContiguous(len(excludeNames))
		// Mark all registers for cleanup
		for i := 0; i < len(excludeNames); i++ {
			defer c.regAlloc.Free(startReg + Register(i))
		}
		if debugAssignment {
			fmt.Printf("// [ObjectRest] Allocated contiguous registers starting at %d for %d elements\n", startReg, len(excludeNames))
		}

		// Load each string constant into consecutive registers
		for i, name := range excludeNames {
			nameConstIdx := c.chunk.AddConstant(name)
			targetReg := startReg + Register(i)
			c.emitLoadConstant(targetReg, nameConstIdx, line)
			if debugAssignment {
				fmt.Printf("// [ObjectRest] Loading '%s' into reg %d (const idx %d)\n", name.AsString(), targetReg, nameConstIdx)
			}
		}

		// Create array from the element registers
		c.emitOpCode(vm.OpMakeArray, line)
		c.emitByte(byte(excludeArrayReg))      // destination register
		c.emitByte(byte(startReg))             // start register (first element)
		c.emitByte(byte(len(excludeNames)))    // element count
		if debugAssignment {
			fmt.Printf("// [ObjectRest] OpMakeArray: dest=%d, start=%d, count=%d\n", excludeArrayReg, startReg, len(excludeNames))
		}
	}

	// Create result register for the rest object
	restObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(restObjReg)

	// Use OpCopyObjectExcluding which filters out non-enumerable properties
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
	valueReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(valueReg)

	_, err := c.compileNode(node.Value, valueReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. Use iterator protocol for all destructuring
	// This works for arrays (which have Symbol.iterator built-in) AND custom iterables
	// Arrays' Symbol.iterator provides optimal performance by returning numeric indices
	err = c.compileArrayDestructuringIteratorPath(node, valueReg, line)
	if err != nil {
		return BadRegister, err
	}

	return BadRegister, nil
}

// compileArrayDestructuringFastPath compiles array destructuring using numeric indexing (fast path)
func (c *Compiler) compileArrayDestructuringFastPath(node *parser.ArrayDestructuringDeclaration, arrayReg Register, line int) errors.PaseratiError {
	// For each element, compile: define target = array[index]
	for i, element := range node.Elements {
		if element.Target == nil {
			continue // Skip elisions
		}

		var valueReg Register

		if element.IsRest {
			// Rest element: compile array.slice(i) to get remaining elements
			valueReg = c.regAlloc.Alloc()

			// Call array.slice(i) to get the rest of the array
			err := c.compileArraySliceCall(arrayReg, i, valueReg, line)
			if err != nil {
				c.regAlloc.Free(valueReg)
				return err
			}
		} else {
			// Regular element: compile array[i]
			indexReg := c.regAlloc.Alloc()
			valueReg = c.regAlloc.Alloc()

			// Load the index as a constant
			indexConstIdx := c.chunk.AddConstant(vm.Number(float64(i)))
			c.emitLoadConstant(indexReg, indexConstIdx, line)

			// Get array[i] using GetIndex operation
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(valueReg))   // destination register
			c.emitByte(byte(arrayReg))   // array register
			c.emitByte(byte(indexReg))   // index register

			c.regAlloc.Free(indexReg)
		}

		// Handle assignment based on target type (identifier vs nested pattern)
		if ident, ok := element.Target.(*parser.Identifier); ok {
			// Simple identifier target
			if element.Default != nil {
				// First, define the variable to reserve the name
				err := c.defineDestructuredVariable(ident.Value, node.IsConst, types.Any, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}

				// Get the target identifier for conditional assignment
				targetIdent := &parser.Identifier{
					Token: ident.Token,
					Value: ident.Value,
				}

				// Use conditional assignment: target = valueReg !== undefined ? valueReg : defaultExpr
				err = c.compileConditionalAssignment(targetIdent, valueReg, element.Default, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			} else {
				// Define variable with extracted value
				err := c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, valueReg, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			}
		} else {
			// Nested pattern target (ArrayLiteral or ObjectLiteral)
			if element.Default != nil {
				// Handle conditional assignment for nested patterns
				err := c.compileConditionalAssignmentForDeclaration(element.Target, valueReg, element.Default, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			} else {
				// Direct nested pattern assignment using recursive compilation
				err := c.compileNestedPatternDeclaration(element.Target, valueReg, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			}
		}

		// Clean up temporary registers
		c.regAlloc.Free(valueReg)
	}

	return nil
}

// compileArrayDestructuringIteratorPath compiles array destructuring using iterator protocol
func (c *Compiler) compileArrayDestructuringIteratorPath(node *parser.ArrayDestructuringDeclaration, iterableReg Register, line int) errors.PaseratiError {
	// Get Symbol.iterator via computed index
	iteratorMethodReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorMethodReg)

	// Load global Symbol
	symbolObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(symbolObjReg)
	symIdx := c.GetOrAssignGlobalIndex("Symbol")
	c.emitGetGlobal(symbolObjReg, symIdx, line)

	// Get Symbol.iterator
	propNameReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(propNameReg)
	c.emitLoadNewConstant(propNameReg, vm.String("iterator"), line)

	iteratorKeyReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorKeyReg)
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(iteratorKeyReg))
	c.emitByte(byte(symbolObjReg))
	c.emitByte(byte(propNameReg))

	// Get iterable[Symbol.iterator]
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(iteratorMethodReg))
	c.emitByte(byte(iterableReg))
	c.emitByte(byte(iteratorKeyReg))

	// Call the iterator method to get iterator object
	iteratorObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorObjReg)
	c.emitCallMethod(iteratorObjReg, iteratorMethodReg, iterableReg, 0, line)

	// Allocate register to track iterator.done state
	// We update this each time we call next(), then check it before calling iterator.return()
	doneReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(doneReg)
	// Initialize to false
	c.emitLoadFalse(doneReg, line)

	// Track how many elements we've consumed for rest elements
	elementIndex := 0

	// For each element, call iterator.next()
	for _, element := range node.Elements {
		if element.Target == nil {
			// Elision: consume iterator value but don't bind
			c.compileIteratorNext(iteratorObjReg, BadRegister, doneReg, line, true)
			elementIndex++
			continue
		}

		if element.IsRest {
			// Rest element: collect all remaining iterator values into an array
			// Rest elements exhaust the iterator, so we'll update done inside compileIteratorToArray
			restArrayReg := c.regAlloc.Alloc()
			err := c.compileIteratorToArray(iteratorObjReg, restArrayReg, line)
			if err != nil {
				c.regAlloc.Free(restArrayReg)
				return err
			}

			// Bind rest array to target
			if ident, ok := element.Target.(*parser.Identifier); ok {
				err := c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, restArrayReg, line)
				c.regAlloc.Free(restArrayReg)
				if err != nil {
					return err
				}
			} else {
				// Nested pattern: [...[x, y]] - destructure the rest array into the pattern
				err := c.compileNestedPatternDeclaration(element.Target, restArrayReg, node.IsConst, line)
				c.regAlloc.Free(restArrayReg)
				if err != nil {
					return err
				}
			}
			// Rest must be last, so we're done - the iterator is exhausted, set done=true
			c.emitLoadTrue(doneReg, line)
			break
		}

		// Regular element: get next value from iterator
		valueReg := c.regAlloc.Alloc()
		c.compileIteratorNext(iteratorObjReg, valueReg, doneReg, line, false)

		// Handle assignment based on target type
		if ident, ok := element.Target.(*parser.Identifier); ok {
			if element.Default != nil {
				// Define variable first
				err := c.defineDestructuredVariable(ident.Value, node.IsConst, types.Any, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}

				// Conditional assignment with default
				targetIdent := &parser.Identifier{Token: ident.Token, Value: ident.Value}
				err = c.compileConditionalAssignment(targetIdent, valueReg, element.Default, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			} else {
				// Define variable with value
				err := c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, valueReg, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			}
		} else {
			// Nested pattern
			if element.Default != nil {
				err := c.compileConditionalAssignmentForDeclaration(element.Target, valueReg, element.Default, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			} else {
				err := c.compileNestedPatternDeclaration(element.Target, valueReg, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return err
				}
			}
		}

		c.regAlloc.Free(valueReg)
		elementIndex++
	}

	// Call IteratorClose (iterator.return if it exists AND iterator is not done)
	c.emitIteratorCleanupWithDone(iteratorObjReg, doneReg, line)

	return nil
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

	// ECMAScript compliance: Throw TypeError if destructuring null or undefined
	// This is required at runtime even if type checker catches it at compile time
	// We need to check: if (tempReg === null || tempReg === undefined) throw TypeError

	// Allocate register for null/undefined checks
	checkReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(checkReg)

	// Check if tempReg is null
	nullConstIdx := c.chunk.AddConstant(vm.Null)
	c.emitLoadConstant(checkReg, nullConstIdx, line)
	c.emitOpCode(vm.OpEqual, line)
	c.emitByte(byte(checkReg))  // result register
	c.emitByte(byte(tempReg))   // left operand
	c.emitByte(byte(checkReg))  // right operand (null)

	// Jump past error if not null
	notNullJump := c.emitPlaceholderJump(vm.OpJumpIfFalse, checkReg, line)

	// Throw TypeError: Cannot destructure null
	errorReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(errorReg)
	typeErrorIdx := c.chunk.AddConstant(vm.String("TypeError"))
	c.emitGetGlobal(errorReg, typeErrorIdx, line)

	msgReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(msgReg)
	msgConstIdx := c.chunk.AddConstant(vm.String("Cannot destructure 'null'"))
	c.emitLoadConstant(msgReg, msgConstIdx, line)

	resultReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(resultReg)
	c.emitCall(resultReg, errorReg, 1, line)  // Call TypeError constructor with message
	c.emitOpCode(vm.OpThrow, line)
	c.emitByte(byte(resultReg))

	// Patch jump for not-null case
	c.patchJump(notNullJump)

	// Check if tempReg is undefined
	undefConstIdx := c.chunk.AddConstant(vm.Undefined)
	c.emitLoadConstant(checkReg, undefConstIdx, line)
	c.emitOpCode(vm.OpEqual, line)
	c.emitByte(byte(checkReg))  // result register
	c.emitByte(byte(tempReg))   // left operand
	c.emitByte(byte(checkReg))  // right operand (undefined)

	// Jump past error if not undefined
	notUndefJump := c.emitPlaceholderJump(vm.OpJumpIfFalse, checkReg, line)

	// Throw TypeError: Cannot destructure undefined
	c.emitGetGlobal(errorReg, typeErrorIdx, line)
	msgConstIdx = c.chunk.AddConstant(vm.String("Cannot destructure 'undefined'"))
	c.emitLoadConstant(msgReg, msgConstIdx, line)
	c.emitCall(resultReg, errorReg, 1, line)  // Call TypeError constructor with message
	c.emitOpCode(vm.OpThrow, line)
	c.emitByte(byte(resultReg))

	// Patch jump for not-undefined case
	c.patchJump(notUndefJump)

	// 2. For each property, compile: define target = temp.property
	for _, prop := range node.Properties {
		if prop.Key == nil || prop.Target == nil {
			continue // Skip malformed properties
		}

		// Support both identifier and nested pattern targets

		// Allocate register for extracted value
		valueReg := c.regAlloc.Alloc()

		// Handle property access (identifier or computed)
		if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
			// Check if the property name is numeric (for array index access)
			// This handles cases like {0: x, 1: y} destructuring from arrays
			isNumeric := false
			for _, ch := range keyIdent.Value {
				if ch < '0' || ch > '9' {
					isNumeric = false
					break
				}
				isNumeric = true
			}

			if isNumeric && len(keyIdent.Value) > 0 {
				// Use OpGetIndex for numeric properties (array elements)
				// Convert string to number for proper array indexing
				var indexNum float64
				fmt.Sscanf(keyIdent.Value, "%f", &indexNum)
				indexConstIdx := c.chunk.AddConstant(vm.Number(indexNum))
				indexReg := c.regAlloc.Alloc()
				c.emitLoadConstant(indexReg, indexConstIdx, line)
				c.emitOpCode(vm.OpGetIndex, line)
				c.emitByte(byte(valueReg))
				c.emitByte(byte(tempReg))
				c.emitByte(byte(indexReg))
				c.regAlloc.Free(indexReg)
			} else {
				// Use OpGetProp for regular string properties
				propNameIdx := c.chunk.AddConstant(vm.String(keyIdent.Value))
				c.emitOpCode(vm.OpGetProp, line)
				c.emitByte(byte(valueReg))   // destination register
				c.emitByte(byte(tempReg))    // object register
				c.emitUint16(propNameIdx)    // property name constant index
			}
		} else if numLit, ok := prop.Key.(*parser.NumberLiteral); ok {
			// Number literal key: convert to string property name
			propName := numLit.Token.Literal
			propNameIdx := c.chunk.AddConstant(vm.String(propName))
			c.emitOpCode(vm.OpGetProp, line)
			c.emitByte(byte(valueReg))   // destination register
			c.emitByte(byte(tempReg))    // object register
			c.emitUint16(propNameIdx)    // property name constant index
		} else if bigIntLit, ok := prop.Key.(*parser.BigIntLiteral); ok {
			// BigInt literal key: convert to string property name (numeric part without 'n')
			propName := bigIntLit.Value
			propNameIdx := c.chunk.AddConstant(vm.String(propName))
			c.emitOpCode(vm.OpGetProp, line)
			c.emitByte(byte(valueReg))   // destination register
			c.emitByte(byte(tempReg))    // object register
			c.emitUint16(propNameIdx)    // property name constant index
		} else if computed, ok := prop.Key.(*parser.ComputedPropertyName); ok {
			keyReg := c.regAlloc.Alloc()
			_, err := c.compileNode(computed.Expr, keyReg)
			if err != nil {
				c.regAlloc.Free(valueReg)
				c.regAlloc.Free(keyReg)
				return BadRegister, err
			}
			c.emitOpCode(vm.OpGetIndex, line)
			c.emitByte(byte(valueReg))
			c.emitByte(byte(tempReg))
			c.emitByte(byte(keyReg))
			c.regAlloc.Free(keyReg)
		}

		// Handle assignment based on target type (identifier vs nested pattern)
		if ident, ok := prop.Target.(*parser.Identifier); ok {
			// Simple identifier target
			if prop.Default != nil {
				// First, define the variable to reserve the name and get the target register
				err := c.defineDestructuredVariable(ident.Value, node.IsConst, types.Any, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return BadRegister, err
				}
				
				// Get the target identifier for conditional assignment
				targetIdent := &parser.Identifier{
					Token: ident.Token,
					Value: ident.Value,
				}
				
				// Use conditional assignment: target = valueReg !== undefined ? valueReg : defaultExpr
				err = c.compileConditionalAssignment(targetIdent, valueReg, prop.Default, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return BadRegister, err
				}
			} else {
				// Define variable with extracted value
				err := c.defineDestructuredVariableWithValue(ident.Value, node.IsConst, valueReg, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return BadRegister, err
				}
			}
		} else {
			// Nested pattern target (ArrayLiteral or ObjectLiteral)
			if prop.Default != nil {
				// Handle conditional assignment for nested patterns
				err := c.compileConditionalAssignmentForDeclaration(prop.Target, valueReg, prop.Default, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return BadRegister, err
				}
			} else {
				// Direct nested pattern assignment using recursive compilation
				err := c.compileNestedPatternDeclaration(prop.Target, valueReg, node.IsConst, line)
				if err != nil {
					c.regAlloc.Free(valueReg)
					return BadRegister, err
				}
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
// compileAssignmentToMember compiles assignment to a member expression: obj.prop = valueReg or obj[key] = valueReg
func (c *Compiler) compileAssignmentToMember(memberExpr *parser.MemberExpression, valueReg Register, line int) errors.PaseratiError {
	// Compile the object expression
	objectReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(objectReg)

	_, err := c.compileNode(memberExpr.Object, objectReg)
	if err != nil {
		return err
	}

	// Check if this is a computed property
	if computedKey, ok := memberExpr.Property.(*parser.ComputedPropertyName); ok {
		// Computed property: obj[expr] = value
		keyReg := c.regAlloc.Alloc()
		defer c.regAlloc.Free(keyReg)

		_, err := c.compileNode(computedKey.Expr, keyReg)
		if err != nil {
			return err
		}

		// Emit OpSetIndex: objectReg[keyReg] = valueReg
		c.emitOpCode(vm.OpSetIndex, line)
		c.emitByte(byte(objectReg))
		c.emitByte(byte(keyReg))
		c.emitByte(byte(valueReg))
	} else {
		// Regular property: obj.prop = value
		propName := c.extractPropertyName(memberExpr.Property)

		// Check for private field (starts with #)
		if len(propName) > 0 && propName[0] == '#' {
			// Private field
			fieldName := propName[1:]
			nameConstIdx := c.chunk.AddConstant(vm.String(fieldName))
			c.emitSetPrivateField(objectReg, valueReg, nameConstIdx, line)
		} else {
			// Regular property
			nameConstIdx := c.chunk.AddConstant(vm.String(propName))
			c.emitSetProp(objectReg, valueReg, nameConstIdx, line)
		}
	}

	return nil
}

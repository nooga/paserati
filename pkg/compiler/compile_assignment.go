package compiler

import (
	"fmt"

	"paserati/pkg/errors"
	"paserati/pkg/parser"
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
			identInfo.globalIdx = c.getOrAssignGlobalIndex(lhsNode.Value)
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

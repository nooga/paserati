package compiler

import (
	"fmt"

	"paserati/pkg/errors"
	"paserati/pkg/parser"
	"paserati/pkg/vm"
)

// compileAssignmentExpression compiles identifier = value OR indexExpr = value
func (c *Compiler) compileAssignmentExpression(node *parser.AssignmentExpression) errors.PaseratiError {
	line := node.Token.Line

	// --- Refactored LHS Handling ---
	var currentValueReg Register // Register holding the value BEFORE the assignment/operation
	var needsStore bool = true   // Assume we need to store back by default
	type lhsInfoType int
	const (
		lhsIsIdentifier lhsInfoType = iota
		lhsIsIndexExpr
		// lhsIsMemberExpr // Placeholder for future
	)
	var lhsType lhsInfoType
	var identInfo struct { // Info needed to store back to identifier
		targetReg    Register
		isUpvalue    bool
		upvalueIndex uint8
	}
	var indexInfo struct { // Info needed to store back to index expr
		arrayReg Register
		indexReg Register
	}

	// ... (existing switch lhsNode := node.Left.(type) block remains unchanged) ...
	switch lhsNode := node.Left.(type) {
	case *parser.Identifier:
		lhsType = lhsIsIdentifier
		// Resolve the identifier
		symbolRef, definingTable, found := c.currentSymbolTable.Resolve(lhsNode.Value)
		if !found {
			return NewCompileError(node, fmt.Sprintf("assignment to undeclared variable '%s'", lhsNode.Value))
		}

		// Determine target register/upvalue index and load current value
		if definingTable == c.currentSymbolTable {
			// Local variable
			identInfo.targetReg = symbolRef.Register
			identInfo.isUpvalue = false
			currentValueReg = identInfo.targetReg // Current value is already in targetReg
		} else {
			// Upvalue
			identInfo.isUpvalue = true
			identInfo.upvalueIndex = c.addFreeSymbol(node, &symbolRef)
			currentValueReg = c.regAlloc.Alloc() // Allocate temporary reg for current value
			c.emitOpCode(vm.OpLoadFree, line)
			c.emitByte(byte(currentValueReg))  // Destination register
			c.emitByte(identInfo.upvalueIndex) // Upvalue index
		}
		// If currentValueReg is nilRegister here, it's an internal error (should be targetReg or newly allocated)
		if currentValueReg == nilRegister {
			panic(fmt.Sprintf("Internal compiler error: currentValueReg is nilRegister for identifier '%s'", lhsNode.Value))
		}

	case *parser.IndexExpression:
		lhsType = lhsIsIndexExpr
		// Compile array expression
		err := c.compileNode(lhsNode.Left)
		if err != nil {
			return err
		}
		indexInfo.arrayReg = c.regAlloc.Current()

		// Compile index expression
		err = c.compileNode(lhsNode.Index)
		if err != nil {
			// TODO: Consider freeing arrayReg if allocated?
			return err
		}
		indexInfo.indexReg = c.regAlloc.Current()

		// Load the current value at the index
		currentValueReg = c.regAlloc.Alloc()
		c.emitOpCode(vm.OpGetIndex, line) // Use assignment token line
		c.emitByte(byte(currentValueReg))
		c.emitByte(byte(indexInfo.arrayReg))
		c.emitByte(byte(indexInfo.indexReg))
		// Keep arrayReg and indexReg allocated for the potential SetIndex later

	// case *parser.MemberExpression: // TODO: Add later
	// 	lhsType = lhsIsMemberExpr
	// 	// ... compile object, load property value, store info ...

	default:
		return NewCompileError(node, fmt.Sprintf("invalid assignment target, expected identifier or index expression, got %T", node.Left))
	}
	// --- End Refactored LHS Handling ---

	// This register will hold the final value of the assignment expression
	// (either the original LHS value or the RHS value depending on the operator and short-circuiting)
	var storeOpTargetReg Register
	// needsStore init'd true

	var jumpPastStore int = -1 // New jump placeholder

	var jumpToEnd int = -1 // Jumps past RHS eval *and* the store block

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
			isNullReg := c.regAlloc.Alloc()
			isUndefReg := c.regAlloc.Alloc()
			nullConstReg := c.regAlloc.Alloc()
			undefConstReg := c.regAlloc.Alloc()
			c.emitLoadNewConstant(nullConstReg, vm.Null(), line)
			c.emitLoadNewConstant(undefConstReg, vm.Undefined(), line)
			c.emitStrictEqual(isNullReg, currentValueReg, nullConstReg, line)
			jumpIfNotNull := c.emitPlaceholderJump(vm.OpJumpIfFalse, isNullReg, line)
			// If IS null -> jumpToEvalRhs
			jumpToEvalRhs = c.emitPlaceholderJump(vm.OpJump, 0, line)
			c.patchJump(jumpIfNotNull)
			c.emitStrictEqual(isUndefReg, currentValueReg, undefConstReg, line)
			// If NOT nullish -> jumpToEnd (skip RHS eval AND store)
			jumpToEnd = c.emitPlaceholderJump(vm.OpJumpIfFalse, isUndefReg, line)
			c.patchJump(jumpToEvalRhs) // Patch jump from null check TO start of RHS eval
			c.regAlloc.Free(isNullReg)
			c.regAlloc.Free(isUndefReg)
			c.regAlloc.Free(nullConstReg)
			c.regAlloc.Free(undefConstReg)
		}

		// --- Evaluate RHS Path ---
		if jumpToEvalRhs != -1 {
			c.patchJump(jumpToEvalRhs) // Patch jumps that lead here
		}
		// This block is only reached if short-circuit didn't happen
		err := c.compileNode(node.Value)
		if err != nil {
			return err
		}
		evaluatedRhs = true
		rhsValueReg = c.regAlloc.Current()
		storeOpTargetReg = rhsValueReg // Result is RHS
		needsStore = true              // Store IS needed
		fmt.Printf("// DEBUG Assign Logical RHS: Evaluated RHS. rhsValueReg=R%d, storeOpTargetReg=R%d, needsStore=%v\n", rhsValueReg, storeOpTargetReg, needsStore)
		// Jump unconditionally past the merge/short-circuit logic block
		jumpPastMerge := c.emitPlaceholderJump(vm.OpJump, 0, line)

		// --- Skip RHS Path --- // This comment block is now conceptually skipped by the jump above

		// --- Merge BEFORE Store (for Logical Ops only) ---
		c.patchJump(jumpPastMerge) // Patch the jump from the RHS path to land AFTER this block
		// Determine final state based on path, but store happens later
		if !evaluatedRhs { // This block is now only reachable via short-circuit
			storeOpTargetReg = currentValueReg // Result is original LHS value
			needsStore = false                 // Store is NOT needed
			fmt.Printf("// DEBUG Assign Logical ShortCircuit: Path taken. currentValueReg=R%d, storeOpTargetReg set to R%d, needsStore=%v\n", currentValueReg, storeOpTargetReg, needsStore)
			// If store is not needed, we MUST jump past the store block
			jumpPastStore = c.emitPlaceholderJump(vm.OpJump, 0, line) // Jump past store
		}
		fmt.Printf("// DEBUG Assign Logical End: Final decision. storeOpTargetReg=R%d, needsStore=%v\n", storeOpTargetReg, needsStore)

		// Free RHS reg if evaluated and not needed for store (maybe?)
		if evaluatedRhs && rhsValueReg != storeOpTargetReg {
			// c.regAlloc.Free(rhsValueReg) // Revisit freeing
		}

	} else { // --- Non-Logical Assignment ---
		// Compile RHS
		err := c.compileNode(node.Value)
		if err != nil {
			// TODO: Free registers allocated for LHS?
			return NewCompileError(node, "error compiling RHS").CausedBy(err)
		}
		rhsValueReg := c.regAlloc.Current() // RHS Value is in this register

		// Determine result register: in-place for local vars, new reg otherwise
		var resultReg Register // Will hold result if not calculated in-place
		needsStore = true      // Non-logical assignments always need storing

		switch node.Operator {
		// --- Compound Arithmetic ---
		case "+=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue {
				c.emitAdd(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				storeOpTargetReg = currentValueReg
			} else {
				resultReg = c.regAlloc.Alloc()
				c.emitAdd(resultReg, currentValueReg, rhsValueReg, line) // To new reg
				storeOpTargetReg = resultReg
			}
		case "-=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue {
				c.emitSubtract(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				storeOpTargetReg = currentValueReg
			} else {
				resultReg = c.regAlloc.Alloc()
				c.emitSubtract(resultReg, currentValueReg, rhsValueReg, line) // To new reg
				storeOpTargetReg = resultReg
			}
		case "*=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue {
				c.emitMultiply(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				storeOpTargetReg = currentValueReg
			} else {
				resultReg = c.regAlloc.Alloc()
				c.emitMultiply(resultReg, currentValueReg, rhsValueReg, line) // To new reg
				storeOpTargetReg = resultReg
			}
		case "/=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue {
				c.emitDivide(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				storeOpTargetReg = currentValueReg
			} else {
				resultReg = c.regAlloc.Alloc()
				c.emitDivide(resultReg, currentValueReg, rhsValueReg, line) // To new reg
				storeOpTargetReg = resultReg
			}
		case "%=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue {
				c.emitRemainder(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				storeOpTargetReg = currentValueReg
			} else {
				resultReg = c.regAlloc.Alloc()
				c.emitRemainder(resultReg, currentValueReg, rhsValueReg, line) // To new reg
				storeOpTargetReg = resultReg
			}
		case "**=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue {
				c.emitExponent(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				storeOpTargetReg = currentValueReg
			} else {
				resultReg = c.regAlloc.Alloc()
				c.emitExponent(resultReg, currentValueReg, rhsValueReg, line) // To new reg
				storeOpTargetReg = resultReg
			}

		// --- Compound Bitwise / Shift ---
		case "&=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue {
				c.emitBitwiseAnd(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				storeOpTargetReg = currentValueReg
			} else {
				resultReg = c.regAlloc.Alloc()
				c.emitBitwiseAnd(resultReg, currentValueReg, rhsValueReg, line) // To new reg
				storeOpTargetReg = resultReg
			}
		case "|=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue {
				c.emitBitwiseOr(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				storeOpTargetReg = currentValueReg
			} else {
				resultReg = c.regAlloc.Alloc()
				c.emitBitwiseOr(resultReg, currentValueReg, rhsValueReg, line) // To new reg
				storeOpTargetReg = resultReg
			}
		case "^=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue {
				c.emitBitwiseXor(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				storeOpTargetReg = currentValueReg
			} else {
				resultReg = c.regAlloc.Alloc()
				c.emitBitwiseXor(resultReg, currentValueReg, rhsValueReg, line) // To new reg
				storeOpTargetReg = resultReg
			}
		case "<<=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue {
				c.emitShiftLeft(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				storeOpTargetReg = currentValueReg
			} else {
				resultReg = c.regAlloc.Alloc()
				c.emitShiftLeft(resultReg, currentValueReg, rhsValueReg, line) // To new reg
				storeOpTargetReg = resultReg
			}
		case ">>=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue {
				c.emitShiftRight(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				storeOpTargetReg = currentValueReg
			} else {
				resultReg = c.regAlloc.Alloc()
				c.emitShiftRight(resultReg, currentValueReg, rhsValueReg, line) // To new reg
				storeOpTargetReg = resultReg
			}
		case ">>>=":
			if lhsType == lhsIsIdentifier && !identInfo.isUpvalue {
				c.emitUnsignedShiftRight(currentValueReg, currentValueReg, rhsValueReg, line) // In-place
				storeOpTargetReg = currentValueReg
			} else {
				resultReg = c.regAlloc.Alloc()
				c.emitUnsignedShiftRight(resultReg, currentValueReg, rhsValueReg, line) // To new reg
				storeOpTargetReg = resultReg
			}

		// --- Simple Assignment ---
		case "=":
			// Simple assignment: result is just the RHS value.
			// We use a new register for the result to simplify the store step.
			resultReg = c.regAlloc.Alloc()
			c.emitMove(resultReg, rhsValueReg, line)
			storeOpTargetReg = resultReg

		default:
			// TODO: Free registers?
			return NewCompileError(node, fmt.Sprintf("unsupported assignment operator '%s'", node.Operator))
		}
		// The final value to be stored is now in storeOpTargetReg

		// Free the intermediate resultReg if it was used and is different from storeOpTargetReg
		if resultReg != 0 && resultReg != storeOpTargetReg {
			c.regAlloc.Free(resultReg)
		}
		// Free the RHS register if it's different from the final stored value
		if rhsValueReg != storeOpTargetReg {
			c.regAlloc.Free(rhsValueReg)
		}
	}
	// --- End Operator Logic ---

	// --- Store Result Back to LHS ---
	// This block is now potentially skipped by a jump if needsStore is false
	if needsStore { // Check flag again just before emitting store code
		switch lhsType {
		case lhsIsIdentifier:
			if identInfo.isUpvalue {
				c.emitSetUpvalue(identInfo.upvalueIndex, storeOpTargetReg, line)
			} else {
				if storeOpTargetReg != identInfo.targetReg {
					fmt.Printf("// DEBUG Assign Store: Emitting Move R%d <- R%d\n", identInfo.targetReg, storeOpTargetReg)
					c.emitMove(identInfo.targetReg, storeOpTargetReg, line)
				} else {
					fmt.Printf("// DEBUG Assign Store: Skipping Move R%d <- R%d (already inplace)\n", identInfo.targetReg, storeOpTargetReg)
				}
			}
		case lhsIsIndexExpr:
			fmt.Printf("// DEBUG Assign Store: Emitting SetIndex [%d][%d] = R%d\n", indexInfo.arrayReg, indexInfo.indexReg, storeOpTargetReg)
			c.emitOpCode(vm.OpSetIndex, line)
			c.emitByte(byte(indexInfo.arrayReg))
			c.emitByte(byte(indexInfo.indexReg))
			c.emitByte(byte(storeOpTargetReg))
		}
	} else {
		fmt.Printf("// DEBUG Assign Store: Skipped store operation (needsStore=false)\n")
	}

	// --- Final Merge Point & Patching ---
	// Patch jumps that needed to skip the store block
	if jumpPastStore != -1 {
		c.patchJump(jumpPastStore)
	}
	// Patch jumps from initial short-circuit checks that needed to skip everything (RHS eval AND store)
	if jumpToEnd != -1 {
		c.patchJump(jumpToEnd)
	}

	// --- Finalize ---
	c.regAlloc.SetCurrent(storeOpTargetReg)
	// ... freeing logic ...

	return nil
}

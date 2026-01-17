package compiler

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

// compileForOfStatementLabeled compiles for-of loops using iterator protocol uniformly
func (c *Compiler) compileForOfStatementLabeled(node *parser.ForOfStatement, label string, hint Register) (Register, errors.PaseratiError) {
	// Track temporary registers for cleanup
	var tempRegs []Register
	defer func() {
		for _, reg := range tempRegs {
			c.regAlloc.Free(reg)
		}
	}()

	// Check if this is a lexical binding (let/const) that needs its own scope
	// This ensures loop variables shadow outer variables with the same name
	var hasLexicalDecl bool
	var prevSymbolTable *SymbolTable
	switch v := node.Variable.(type) {
	case *parser.LetStatement, *parser.ConstStatement:
		hasLexicalDecl = true
	case *parser.ArrayDestructuringDeclaration:
		hasLexicalDecl = v.Token.Type == lexer.LET || v.Token.Type == lexer.CONST
	case *parser.ObjectDestructuringDeclaration:
		hasLexicalDecl = v.Token.Type == lexer.LET || v.Token.Type == lexer.CONST
	}
	if hasLexicalDecl {
		prevSymbolTable = c.currentSymbolTable
		c.currentSymbolTable = NewEnclosedSymbolTable(c.currentSymbolTable)
	}
	// Ensure we restore the scope when done
	defer func() {
		if prevSymbolTable != nil {
			c.currentSymbolTable = prevSymbolTable
		}
	}()

	// 1. Compile the iterable expression
	iterableReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iterableReg)
	_, err := c.compileNode(node.Iterable, iterableReg)
	if err != nil {
		return BadRegister, err
	}

	// 2. Get Symbol.iterator or Symbol.asyncIterator from global Symbol
	symbolObjReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, symbolObjReg)
	symIdx := c.GetOrAssignGlobalIndex("Symbol")
	c.emitGetGlobal(symbolObjReg, symIdx, node.Token.Line)

	// 3. Get Symbol.iterator or Symbol.asyncIterator property
	propNameReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, propNameReg)
	if node.IsAsync {
		c.emitLoadNewConstant(propNameReg, vm.String("asyncIterator"), node.Token.Line)
	} else {
		c.emitLoadNewConstant(propNameReg, vm.String("iterator"), node.Token.Line)
	}

	iteratorKeyReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iteratorKeyReg)
	c.emitOpCode(vm.OpGetIndex, node.Token.Line)
	c.emitByte(byte(iteratorKeyReg))
	c.emitByte(byte(symbolObjReg))
	c.emitByte(byte(propNameReg))

	// 4. Get iterable[Symbol.iterator]
	iteratorMethodReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iteratorMethodReg)
	c.emitOpCode(vm.OpGetIndex, node.Token.Line)
	c.emitByte(byte(iteratorMethodReg))
	c.emitByte(byte(iterableReg))
	c.emitByte(byte(iteratorKeyReg))

	// 5. Call the iterator method to get iterator object
	iteratorObjReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, iteratorObjReg)
	c.emitCallMethod(iteratorObjReg, iteratorMethodReg, iterableReg, 0, node.Token.Line)

	// 5a. Validate that the iterator object is actually an object (not null/undefined/primitive)
	// ECMAScript spec requires iterator to be an object
	c.emitOpCode(vm.OpTypeGuardIteratorReturn, node.Token.Line)
	c.emitByte(byte(iteratorObjReg))

	// 6. Get iterator.next method (once, outside loop)
	nextMethodReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, nextMethodReg)
	nextConstIdx := c.chunk.AddConstant(vm.String("next"))
	c.emitGetProp(nextMethodReg, iteratorObjReg, nextConstIdx, node.Token.Line)

	// Per ECMAScript spec, initialize completion value V = undefined
	c.emitLoadUndefined(hint, node.Token.Line)

	// 7. Loop start & context setup
	loopStartPos := len(c.chunk.Code)
	loopContext := &LoopContext{
		Label:                      label,
		LoopStartPos:               loopStartPos,
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
		IteratorCleanup: &IteratorCleanupInfo{
			IteratorReg:          iteratorObjReg,
			UsesIteratorProtocol: true,
		},
		CompletionReg: hint, // For break/continue to update via UpdateEmpty
	}
	c.loopContextStack = append(c.loopContextStack, loopContext)

	// 8. Call iterator.next() to get {value, done} (or Promise<{value, done}> for async)
	resultReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, resultReg)
	c.emitCallMethod(resultReg, nextMethodReg, iteratorObjReg, 0, node.Token.Line)

	// 8a. For async iterators, await the promise
	if node.IsAsync {
		awaitedReg := c.regAlloc.Alloc()
		tempRegs = append(tempRegs, awaitedReg)
		c.emitOpCode(vm.OpAwait, node.Token.Line)
		c.emitByte(byte(awaitedReg))
		c.emitByte(byte(resultReg))
		resultReg = awaitedReg // Use awaited result
	}

	// 8b. Validate that iterator result is an object (required by ECMAScript spec)
	// If not an object, throw TypeError
	c.emitOpCode(vm.OpTypeGuardIteratorReturn, node.Token.Line)
	c.emitByte(byte(resultReg))

	// 9. Get result.done
	doneReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, doneReg)
	doneConstIdx := c.chunk.AddConstant(vm.String("done"))
	c.emitGetProp(doneReg, resultReg, doneConstIdx, node.Token.Line)

	// 10. Negate done to check if NOT done (continue looping)
	notDoneReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, notDoneReg)
	c.emitOpCode(vm.OpNot, node.Token.Line)
	c.emitByte(byte(notDoneReg))
	c.emitByte(byte(doneReg))

	// 11. Exit loop if NOT not-done (i.e., if done)
	exitJump := c.emitPlaceholderJump(vm.OpJumpIfFalse, notDoneReg, node.Token.Line)

	// 12. Get result.value
	valueReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, valueReg)
	valueConstIdx := c.chunk.AddConstant(vm.String("value"))
	c.emitGetProp(valueReg, resultReg, valueConstIdx, node.Token.Line)

	// 13. Assign value to loop variable
	// Track per-iteration binding registers for let/const loop variables (not var - var is function-scoped)
	var perIterationRegs []Register
	if letStmt, ok := node.Variable.(*parser.LetStatement); ok {
		symbol := c.currentSymbolTable.Define(letStmt.Name.Value, c.regAlloc.Alloc())
		c.regAlloc.Pin(symbol.Register)
		perIterationRegs = append(perIterationRegs, symbol.Register)
		c.emitMove(symbol.Register, valueReg, node.Token.Line)
	} else if constStmt, ok := node.Variable.(*parser.ConstStatement); ok {
		// Use DefineConst (not TDZ) - variable is immediately initialized in for-of
		symbol := c.currentSymbolTable.DefineConst(constStmt.Name.Value, c.regAlloc.Alloc())
		c.regAlloc.Pin(symbol.Register)
		perIterationRegs = append(perIterationRegs, symbol.Register)
		c.emitMove(symbol.Register, valueReg, node.Token.Line)
	} else if varStmt, ok := node.Variable.(*parser.VarStatement); ok {
		symbol := c.currentSymbolTable.Define(varStmt.Name.Value, c.regAlloc.Alloc())
		c.regAlloc.Pin(symbol.Register)
		// Note: var is function-scoped, so no per-iteration binding needed
		c.emitMove(symbol.Register, valueReg, node.Token.Line)
	} else if arrayDestr, ok := node.Variable.(*parser.ArrayDestructuringDeclaration); ok {
		// Array destructuring: for(const [x, y] of arr)
		// Use iterator protocol to destructure the value
		isConst := arrayDestr.IsConst
		err := c.compileForOfArrayDestructuring(arrayDestr, valueReg, isConst, node.Token.Line)
		if err != nil {
			return BadRegister, err
		}
	} else if objDestr, ok := node.Variable.(*parser.ObjectDestructuringDeclaration); ok {
		// Object destructuring: for(const {x, y} of arr)
		for _, prop := range objDestr.Properties {
			if prop.Target == nil {
				continue
			}
			extractedReg := c.regAlloc.Alloc()
			tempRegs = append(tempRegs, extractedReg)

			if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
				propConstIdx := c.chunk.AddConstant(vm.String(keyIdent.Value))
				c.emitGetProp(extractedReg, valueReg, propConstIdx, node.Token.Line)
			} else if computed, ok := prop.Key.(*parser.ComputedPropertyName); ok {
				keyReg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, keyReg)
				_, err := c.compileNode(computed.Expr, keyReg)
				if err != nil {
					return BadRegister, err
				}
				c.emitOpCode(vm.OpGetIndex, node.Token.Line)
				c.emitByte(byte(extractedReg))
				c.emitByte(byte(valueReg))
				c.emitByte(byte(keyReg))
			}

			// Handle assignment with potential default value
			if ident, ok := prop.Target.(*parser.Identifier); ok {
				// Simple identifier target
				symbol := c.currentSymbolTable.Define(ident.Value, c.regAlloc.Alloc())
				c.regAlloc.Pin(symbol.Register)

				if prop.Default != nil {
					// Use conditional assignment: target = extractedReg !== undefined ? extractedReg : default
					targetIdent := &parser.Identifier{Token: ident.Token, Value: ident.Value}
					err := c.compileConditionalAssignment(targetIdent, extractedReg, prop.Default, node.Token.Line)
					if err != nil {
						return BadRegister, err
					}
				} else {
					c.emitMove(symbol.Register, extractedReg, node.Token.Line)
				}
			} else {
				// Nested pattern target (ArrayLiteral or ObjectLiteral)
				isConst := true // default for const
				if _, ok := node.Variable.(*parser.VarStatement); ok {
					isConst = false
				} else if _, ok := node.Variable.(*parser.LetStatement); ok {
					isConst = false
				}

				if prop.Default != nil {
					// Handle conditional assignment for nested patterns
					err := c.compileConditionalAssignmentForDeclaration(prop.Target, extractedReg, prop.Default, isConst, node.Token.Line)
					if err != nil {
						return BadRegister, err
					}
				} else {
					// Direct nested pattern assignment
					err := c.compileNestedPatternDeclaration(prop.Target, extractedReg, isConst, node.Token.Line)
					if err != nil {
						return BadRegister, err
					}
				}
			}
		}

		// Handle rest property if present
		if objDestr.RestProperty != nil {
			if ident, ok := objDestr.RestProperty.Target.(*parser.Identifier); ok {
				// Create rest object with remaining properties
				err := c.compileObjectRestDeclaration(valueReg, objDestr.Properties, ident.Value, true, node.Token.Line)
				if err != nil {
					return BadRegister, err
				}
			}
		}
	} else if exprStmt, ok := node.Variable.(*parser.ExpressionStatement); ok {
		// This is an existing variable/pattern being assigned to
		switch target := exprStmt.Expression.(type) {
		case *parser.Identifier:
			symbolRef, definingTable, found := c.currentSymbolTable.Resolve(target.Value)
			if !found {
				// Define a function/global-scoped binding (var semantics)
				scope := c.currentSymbolTable
				for scope.Outer != nil {
					scope = scope.Outer
				}
				reg := c.regAlloc.Alloc()
				tempRegs = append(tempRegs, reg)
				sym := scope.Define(target.Value, reg)
				c.regAlloc.Pin(sym.Register)
				c.emitMove(sym.Register, valueReg, node.Token.Line)
			} else {
				if symbolRef.IsGlobal {
					c.emitSetGlobal(symbolRef.GlobalIndex, valueReg, node.Token.Line)
				} else {
					_ = definingTable
					c.emitMove(symbolRef.Register, valueReg, node.Token.Line)
				}
			}
		case *parser.ArrayLiteral:
			// Array destructuring assignment: for ([x, y] of items)
			// Per ECMAScript spec, array destructuring in for-of MUST use iterator protocol
			if err := c.compileForOfArrayAssignmentWithIterator(target, valueReg, node.Token.Line); err != nil {
				return BadRegister, err
			}
		case *parser.ObjectLiteral:
			// Object destructuring assignment: for ({a, b} of items)
			if err := c.compileNestedObjectDestructuring(target, valueReg, node.Token.Line); err != nil {
				return BadRegister, err
			}
		case *parser.MemberExpression:
			// Member expression: for (obj.x of items) or for (obj[key] of items)
			if err := c.compileAssignmentToMember(target, valueReg, node.Token.Line); err != nil {
				return BadRegister, err
			}
		case *parser.IndexExpression:
			// Index expression: for (arr[idx] of items) or for ([let][1] of items)
			if err := c.compileAssignmentToIndex(target, valueReg, node.Token.Line); err != nil {
				return BadRegister, err
			}
		}
	}

	// 14. Compile loop body
	bodyReg := c.regAlloc.Alloc()
	tempRegs = append(tempRegs, bodyReg)
	bodyResultReg, err := c.compileNode(node.Body, bodyReg)
	if err != nil {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
		return BadRegister, err
	}
	// If the body produced a value, update completion value V
	if bodyResultReg != BadRegister {
		c.emitMove(hint, bodyResultReg, node.Token.Line)
	}

	// 15. Patch continue jumps to land here (before next iteration)
	for _, continuePos := range loopContext.ContinuePlaceholderPosList {
		c.patchJump(continuePos)
	}

	// 15a. Per-iteration bindings: close upvalues for let/const loop variables
	// ECMAScript spec: each iteration gets fresh bindings. This ensures closures
	// capture the value at this iteration, not the final value.
	for _, reg := range perIterationRegs {
		c.emitCloseUpvalue(reg, node.Token.Line)
	}

	// 16. Jump back to loop start
	jumpBackPos := len(c.chunk.Code) + 1 + 2
	backOffset := loopStartPos - jumpBackPos
	c.emitOpCode(vm.OpJump, node.Token.Line)
	c.emitUint16(uint16(int16(backOffset)))

	// 17. Patch exit jump - this is where loop exits normally when done=true
	c.patchJump(exitJump)

	// 18. Call iterator.return() after normal loop completion
	// Note: We don't call it here because when done=true, the iterator is exhausted
	// and per spec, we only call return() on abrupt completion (break, throw, etc)
	// which is handled by IteratorCleanup in the loop context

	// 19. Clean up loop context and patch break jumps
	poppedContext := c.loopContextStack[len(c.loopContextStack)-1]
	c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]

	for _, breakPos := range poppedContext.BreakPlaceholderPosList {
		c.patchJump(breakPos)
	}

	// Return completion value
	return hint, nil
}

// compileForOfArrayAssignmentWithIterator uses iterator protocol for array assignment destructuring in for-of
// This handles: for ([x, y] of items) where x, y are already declared
// Per ECMAScript spec 13.7.5.13, array assignment patterns MUST use iterator protocol
func (c *Compiler) compileForOfArrayAssignmentWithIterator(arrayLit *parser.ArrayLiteral, valueReg Register, line int) errors.PaseratiError {
	// Get Symbol.iterator from the value
	symbolObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(symbolObjReg)
	symIdx := c.GetOrAssignGlobalIndex("Symbol")
	c.emitGetGlobal(symbolObjReg, symIdx, line)

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
	iteratorMethodReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorMethodReg)
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(iteratorMethodReg))
	c.emitByte(byte(valueReg))
	c.emitByte(byte(iteratorKeyReg))

	// Call the iterator method
	iteratorObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorObjReg)
	c.emitCallMethod(iteratorObjReg, iteratorMethodReg, valueReg, 0, line)

	// Track done state
	doneReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(doneReg)
	c.emitLoadFalse(doneReg, line)

	// Process each element in the array pattern
	for i, element := range arrayLit.Elements {
		if element == nil {
			// Elision - consume but don't assign
			c.compileIteratorNext(iteratorObjReg, BadRegister, doneReg, line, true)
			continue
		}

		// Check for rest element
		if spreadExpr, ok := element.(*parser.SpreadElement); ok {
			if i != len(arrayLit.Elements)-1 {
				return NewCompileError(arrayLit, "rest element must be last in destructuring pattern")
			}
			// Collect remaining values into array
			restArrayReg := c.regAlloc.Alloc()
			err := c.compileIteratorToArray(iteratorObjReg, restArrayReg, line)
			if err != nil {
				c.regAlloc.Free(restArrayReg)
				return err
			}
			// Assign to target
			err = c.compileRecursiveAssignment(spreadExpr.Argument, restArrayReg, line)
			c.regAlloc.Free(restArrayReg)
			if err != nil {
				return err
			}
			c.emitLoadTrue(doneReg, line)
			break
		}

		// Regular element - extract value from iterator
		extractedReg := c.regAlloc.Alloc()
		c.compileIteratorNext(iteratorObjReg, extractedReg, doneReg, line, false)

		// Handle assignment with default value
		var target parser.Expression
		var defaultExpr parser.Expression

		if assignExpr, ok := element.(*parser.AssignmentExpression); ok && assignExpr.Operator == "=" {
			target = assignExpr.Left
			defaultExpr = assignExpr.Value
		} else {
			target = element
		}

		// Assign to target (with optional default)
		if defaultExpr != nil {
			err := c.compileConditionalAssignment(target, extractedReg, defaultExpr, line)
			c.regAlloc.Free(extractedReg)
			if err != nil {
				return err
			}
		} else {
			err := c.compileRecursiveAssignment(target, extractedReg, line)
			c.regAlloc.Free(extractedReg)
			if err != nil {
				return err
			}
		}
	}

	// Call iterator cleanup (this will validate return() result is an object)
	c.emitIteratorCleanupWithDone(iteratorObjReg, doneReg, line)

	return nil
}

// compileForOfArrayDestructuring uses iterator protocol to destructure array patterns in for-of loops
func (c *Compiler) compileForOfArrayDestructuring(arrayDestr *parser.ArrayDestructuringDeclaration, valueReg Register, isConst bool, line int) errors.PaseratiError {
	// Get Symbol.iterator from the value using the same pattern as compileArrayDestructuringIteratorPath

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

	// Get value[Symbol.iterator]
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(iteratorMethodReg))
	c.emitByte(byte(valueReg))
	c.emitByte(byte(iteratorKeyReg))

	// Call the iterator method to get iterator object
	iteratorObjReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorObjReg)
	c.emitCallMethod(iteratorObjReg, iteratorMethodReg, valueReg, 0, line)

	// Allocate register to track iterator.done state
	doneReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(doneReg)
	c.emitLoadFalse(doneReg, line)

	// For each element, call iterator.next()
	for _, element := range arrayDestr.Elements {
		if element.Target == nil {
			// Elision: consume iterator value but don't bind
			c.compileIteratorNext(iteratorObjReg, BadRegister, doneReg, line, true)
			continue
		}

		if element.IsRest {
			// Rest element: collect all remaining iterator values into an array
			restArrayReg := c.regAlloc.Alloc()
			err := c.compileIteratorToArray(iteratorObjReg, restArrayReg, line)
			if err != nil {
				c.regAlloc.Free(restArrayReg)
				return err
			}

			// Bind rest array to target
			if ident, ok := element.Target.(*parser.Identifier); ok {
				symbol := c.currentSymbolTable.Define(ident.Value, c.regAlloc.Alloc())
				c.regAlloc.Pin(symbol.Register)
				c.emitMove(symbol.Register, restArrayReg, line)
				c.regAlloc.Free(restArrayReg)
			} else {
				// Nested pattern: [...[x, y]]
				err := c.compileNestedPatternDeclaration(element.Target, restArrayReg, isConst, line)
				c.regAlloc.Free(restArrayReg)
				if err != nil {
					return err
				}
			}
			// Rest is last, iterator is exhausted
			c.emitLoadTrue(doneReg, line)
			break
		}

		// Regular element: get next value from iterator
		extractedReg := c.regAlloc.Alloc()
		c.compileIteratorNext(iteratorObjReg, extractedReg, doneReg, line, false)

		// Handle assignment based on target type
		if ident, ok := element.Target.(*parser.Identifier); ok {
			// Simple identifier target
			symbol := c.currentSymbolTable.Define(ident.Value, c.regAlloc.Alloc())
			c.regAlloc.Pin(symbol.Register)

			if element.Default != nil {
				// Conditional assignment with default
				targetIdent := &parser.Identifier{Token: ident.Token, Value: ident.Value}
				err := c.compileConditionalAssignment(targetIdent, extractedReg, element.Default, line)
				c.regAlloc.Free(extractedReg)
				if err != nil {
					return err
				}
			} else {
				// Simple move
				c.emitMove(symbol.Register, extractedReg, line)
				c.regAlloc.Free(extractedReg)
			}
		} else {
			// Nested pattern
			if element.Default != nil {
				err := c.compileConditionalAssignmentForDeclaration(element.Target, extractedReg, element.Default, isConst, line)
				c.regAlloc.Free(extractedReg)
				if err != nil {
					return err
				}
			} else {
				err := c.compileNestedPatternDeclaration(element.Target, extractedReg, isConst, line)
				c.regAlloc.Free(extractedReg)
				if err != nil {
					return err
				}
			}
		}
	}

	// Close the iterator if not done
	c.emitIteratorCleanupWithDone(iteratorObjReg, doneReg, line)

	return nil
}

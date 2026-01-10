package compiler

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

// --- Exception Handling Compilation ---

// compileTryStatement compiles a try/catch/finally statement (Phase 3 design)
func (c *Compiler) compileTryStatement(node *parser.TryStatement, hint Register) (Register, errors.PaseratiError) {
	tryStart := len(c.chunk.Code)

	// Track try depth (used to disable tail calls inside try blocks)
	c.tryDepth++
	defer func() { c.tryDepth-- }()

	// Track finally depth for return statement handling
	// Also push a FinallyContext so break/continue/return know where to jump
	var finallyCtx *FinallyContext
	if node.FinallyBlock != nil {
		c.tryFinallyDepth++
		// Push finally context with placeholder PC (will be filled in later)
		finallyCtx = &FinallyContext{
			FinallyPC:                 -1, // Placeholder, will be set when compiling finally block
			JumpToFinallyPlaceholders: make([]int, 0),
			LoopStackDepthAtCreation:  len(c.loopContextStack), // Record current loop depth
		}
		c.finallyContextStack = append(c.finallyContextStack, finallyCtx)
		defer func() {
			c.tryFinallyDepth--
			// Pop finally context
			c.finallyContextStack = c.finallyContextStack[:len(c.finallyContextStack)-1]
		}()
	}

	// Per ECMAScript spec, try statement completion value is undefined if the block is empty
	// or the last statement doesn't produce a value. Load undefined as the default.
	c.emitLoadUndefined(hint, node.Token.Line)

	// Compile try body - we need to track completion values from statements
	// instead of using the block compilation which discards them
	// Also need to preserve block scoping for let/const declarations
	if node.Body != nil {
		// Create enclosed scope for the try block (like BlockStatement does)
		previousSymbolTable := c.currentSymbolTable
		c.currentSymbolTable = NewEnclosedSymbolTable(previousSymbolTable)

		// Pre-define let/const variables in the block scope
		for _, stmt := range node.Body.Statements {
			switch s := stmt.(type) {
			case *parser.LetStatement:
				if s.Name != nil {
					if _, alreadyInCurrentScope := c.currentSymbolTable.store[s.Name.Value]; !alreadyInCurrentScope {
						reg := c.regAlloc.Alloc()
						c.currentSymbolTable.Define(s.Name.Value, reg)
						c.regAlloc.Pin(reg)
					}
				}
			case *parser.ConstStatement:
				if s.Name != nil {
					if _, alreadyInCurrentScope := c.currentSymbolTable.store[s.Name.Value]; !alreadyInCurrentScope {
						reg := c.regAlloc.Alloc()
						c.currentSymbolTable.Define(s.Name.Value, reg)
						c.regAlloc.Pin(reg)
					}
				}
			}
		}

		// Compile statements
		for _, stmt := range node.Body.Statements {
			stmtReg, err := c.compileNode(stmt, hint)
			if err != nil {
				c.currentSymbolTable = previousSymbolTable
				return BadRegister, err
			}
			// If the statement produced a value (expression statement), it's in hint
			// If not (declaration, etc.), hint still has the previous value or undefined
			if stmtReg != BadRegister && stmtReg != hint {
				// Move the result to hint if it ended up in a different register
				c.emitMove(hint, stmtReg, node.Token.Line)
			}
		}

		// Restore previous scope
		c.currentSymbolTable = previousSymbolTable
	}

	// Strategy: If finally exists, ALL exits must go through it
	var finallyPC int
	var catchAfterJump int
	var normalExitJump int

	if node.FinallyBlock != nil {
		// With finally: try → catch (if present) → finally → continue
		normalExitJump = c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
		tryEnd := len(c.chunk.Code)

		// Compile catch if present
		if node.CatchClause != nil {
			catchPC := len(c.chunk.Code)

			// Allocate register for exception
			catchReg := c.regAlloc.Alloc()
			defer c.regAlloc.Free(catchReg)

			if node.CatchClause.Parameter != nil {
				// Create an enclosed scope for the catch parameter
				// This ensures the catch parameter shadows (but doesn't replace) any outer variable with the same name
				previousSymbolTable := c.currentSymbolTable
				c.currentSymbolTable = NewEnclosedSymbolTable(previousSymbolTable)

				// Define catch parameter in the catch scope
				switch param := node.CatchClause.Parameter.(type) {
				case *parser.Identifier:
					c.currentSymbolTable.Define(param.Value, catchReg)
				case *parser.ArrayParameterPattern:
					// Convert ArrayParameterPattern to ArrayDestructuringDeclaration
					decl := &parser.ArrayDestructuringDeclaration{
						Token:    param.Token,
						IsConst:  false, // catch parameters are not const
						Elements: param.Elements,
						Value:    nil, // value already in catchReg
					}
					if err := c.compileArrayDestructuringDeclarationWithValueReg(decl, catchReg, node.Token.Line); err != nil {
						c.currentSymbolTable = previousSymbolTable
						return BadRegister, err
					}
				case *parser.ObjectParameterPattern:
					// Convert ObjectParameterPattern to ObjectDestructuringDeclaration
					decl := &parser.ObjectDestructuringDeclaration{
						Token:        param.Token,
						IsConst:      false, // catch parameters are not const
						Properties:   param.Properties,
						RestProperty: param.RestProperty,
						Value:        nil, // value already in catchReg
					}
					if err := c.compileObjectDestructuringDeclarationWithValueReg(decl, catchReg, node.Token.Line); err != nil {
						c.currentSymbolTable = previousSymbolTable
						return BadRegister, err
					}
				default:
					c.currentSymbolTable = previousSymbolTable
					return BadRegister, NewCompileError(node, fmt.Sprintf("unexpected catch parameter type: %T", param))
				}

				// Pre-define let/const in the catch scope before compiling statements
				for _, stmt := range node.CatchClause.Body.Statements {
					switch s := stmt.(type) {
					case *parser.LetStatement:
						if s.Name != nil {
							if _, alreadyInCurrentScope := c.currentSymbolTable.store[s.Name.Value]; !alreadyInCurrentScope {
								reg := c.regAlloc.Alloc()
								c.currentSymbolTable.Define(s.Name.Value, reg)
								c.regAlloc.Pin(reg)
							}
						}
					case *parser.ConstStatement:
						if s.Name != nil {
							if _, alreadyInCurrentScope := c.currentSymbolTable.store[s.Name.Value]; !alreadyInCurrentScope {
								reg := c.regAlloc.Alloc()
								c.currentSymbolTable.Define(s.Name.Value, reg)
								c.regAlloc.Pin(reg)
							}
						}
					}
				}

				// Compile catch body - track completion value in hint
				for _, stmt := range node.CatchClause.Body.Statements {
					stmtReg, err := c.compileNode(stmt, hint)
					if err != nil {
						c.currentSymbolTable = previousSymbolTable
						return BadRegister, err
					}
					if stmtReg != BadRegister && stmtReg != hint {
						c.emitMove(hint, stmtReg, node.Token.Line)
					}
				}

				// Restore the previous symbol table
				c.currentSymbolTable = previousSymbolTable
			} else {
				// Catch without parameter (ES2019+) - still need enclosed scope for block scoping
				catchScopePrev := c.currentSymbolTable
				c.currentSymbolTable = NewEnclosedSymbolTable(catchScopePrev)

				// Pre-define let/const in the catch scope
				for _, stmt := range node.CatchClause.Body.Statements {
					switch s := stmt.(type) {
					case *parser.LetStatement:
						if s.Name != nil {
							if _, alreadyInCurrentScope := c.currentSymbolTable.store[s.Name.Value]; !alreadyInCurrentScope {
								reg := c.regAlloc.Alloc()
								c.currentSymbolTable.Define(s.Name.Value, reg)
								c.regAlloc.Pin(reg)
							}
						}
					case *parser.ConstStatement:
						if s.Name != nil {
							if _, alreadyInCurrentScope := c.currentSymbolTable.store[s.Name.Value]; !alreadyInCurrentScope {
								reg := c.regAlloc.Alloc()
								c.currentSymbolTable.Define(s.Name.Value, reg)
								c.regAlloc.Pin(reg)
							}
						}
					}
				}

				// Compile catch body
				for _, stmt := range node.CatchClause.Body.Statements {
					stmtReg, err := c.compileNode(stmt, hint)
					if err != nil {
						c.currentSymbolTable = catchScopePrev
						return BadRegister, err
					}
					if stmtReg != BadRegister && stmtReg != hint {
						c.emitMove(hint, stmtReg, node.Token.Line)
					}
				}

				// Restore scope
				c.currentSymbolTable = catchScopePrev
			}

			// After catch, jump to finally
			catchAfterJump = c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)

			// Add catch handler to exception table
			catchHandler := vm.ExceptionHandler{
				TryStart:   tryStart,
				TryEnd:     tryEnd,
				HandlerPC:  catchPC,
				CatchReg:   int(catchReg),
				IsCatch:    true,
				IsFinally:  false,
				FinallyReg: -1,
			}
			c.chunk.ExceptionTable = append(c.chunk.ExceptionTable, catchHandler)
		}

		// Compile finally block
		finallyPC = len(c.chunk.Code)

		// Update the FinallyContext with the actual finally block PC
		if finallyCtx != nil {
			finallyCtx.FinallyPC = finallyPC
			// Patch all jumps from break/continue/return statements to point to finally
			for _, placeholderPos := range finallyCtx.JumpToFinallyPlaceholders {
				// Use the existing patch logic which calculates the offset
				// to patch the jump to the finallyPC
				c.patchJumpToTarget(placeholderPos, finallyPC)
			}
		}

		// Patch jumps to finally BEFORE compiling the finally block
		c.patchJump(normalExitJump)
		if catchAfterJump != 0 {
			c.patchJump(catchAfterJump)
		}

		// Set finally block context before compilation
		prevInFinally := c.inFinallyBlock
		c.inFinallyBlock = true

		// Per ECMAScript spec:
		// - If finally completes normally, the try/catch completion value is used
		// - If finally completes abnormally (break/continue/return/throw), the finally's
		//   completion value (with UpdateEmpty applied) becomes the result
		//
		// Save try/catch completion value, then initialize hint to undefined for finally's
		// own statement list evaluation (UpdateEmpty semantics).
		tryCatchValueReg := c.regAlloc.Alloc()
		c.emitMove(tryCatchValueReg, hint, node.Token.Line)
		c.emitLoadUndefined(hint, node.Token.Line)

		// Compile finally block with hint for its own completion tracking
		_, err := c.compileNode(node.FinallyBlock, hint)
		c.inFinallyBlock = prevInFinally // Restore previous context

		if err != nil {
			c.regAlloc.Free(tryCatchValueReg)
			return BadRegister, err
		}

		// If we reach here, finally completed normally (no break/continue/return/throw)
		// Restore the try/catch completion value per ECMAScript spec
		c.emitMove(hint, tryCatchValueReg, node.Token.Line)
		c.regAlloc.Free(tryCatchValueReg)

		// ALWAYS emit instruction to handle pending actions after finally block
		// This ensures that even empty finally blocks properly handle pending returns/throws
		c.emitHandlePendingAction(node.Token.Line)

		// The finally handler should cover the try/catch blocks but NOT the finally block itself
		// This ensures that exceptions in try/catch trigger finally, but exceptions in finally
		// don't recursively trigger the same finally block

		// Add finally handler to exception table (covers try/catch but not finally itself)
		finallyHandler := vm.ExceptionHandler{
			TryStart:   tryStart,
			TryEnd:     finallyPC, // Only cover up to start of finally block
			HandlerPC:  finallyPC,
			CatchReg:   -1,
			IsCatch:    false,
			IsFinally:  true,
			FinallyReg: -1, // For now, no pending action storage
		}
		c.chunk.ExceptionTable = append(c.chunk.ExceptionTable, finallyHandler)
	} else {
		// Without finally: Original Phase 1 logic
		normalExitJump = c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
		tryEnd := len(c.chunk.Code)

		// Compile catch if present
		if node.CatchClause != nil {
			catchPC := len(c.chunk.Code)

			// Allocate register for exception
			catchReg := c.regAlloc.Alloc()
			defer c.regAlloc.Free(catchReg)

			if node.CatchClause.Parameter != nil {
				// Create an enclosed scope for the catch parameter
				// This ensures the catch parameter shadows (but doesn't replace) any outer variable with the same name
				previousSymbolTable := c.currentSymbolTable
				c.currentSymbolTable = NewEnclosedSymbolTable(previousSymbolTable)

				// Define catch parameter in the catch scope
				switch param := node.CatchClause.Parameter.(type) {
				case *parser.Identifier:
					c.currentSymbolTable.Define(param.Value, catchReg)
				case *parser.ArrayParameterPattern:
					// Convert ArrayParameterPattern to ArrayDestructuringDeclaration
					decl := &parser.ArrayDestructuringDeclaration{
						Token:    param.Token,
						IsConst:  false, // catch parameters are not const
						Elements: param.Elements,
						Value:    nil, // value already in catchReg
					}
					if err := c.compileArrayDestructuringDeclarationWithValueReg(decl, catchReg, node.Token.Line); err != nil {
						c.currentSymbolTable = previousSymbolTable
						return BadRegister, err
					}
				case *parser.ObjectParameterPattern:
					// Convert ObjectParameterPattern to ObjectDestructuringDeclaration
					decl := &parser.ObjectDestructuringDeclaration{
						Token:        param.Token,
						IsConst:      false, // catch parameters are not const
						Properties:   param.Properties,
						RestProperty: param.RestProperty,
						Value:        nil, // value already in catchReg
					}
					if err := c.compileObjectDestructuringDeclarationWithValueReg(decl, catchReg, node.Token.Line); err != nil {
						c.currentSymbolTable = previousSymbolTable
						return BadRegister, err
					}
				default:
					c.currentSymbolTable = previousSymbolTable
					return BadRegister, NewCompileError(node, fmt.Sprintf("unexpected catch parameter type: %T", param))
				}

				// Pre-define let/const in the catch scope before compiling statements
				for _, stmt := range node.CatchClause.Body.Statements {
					switch s := stmt.(type) {
					case *parser.LetStatement:
						if s.Name != nil {
							if _, alreadyInCurrentScope := c.currentSymbolTable.store[s.Name.Value]; !alreadyInCurrentScope {
								reg := c.regAlloc.Alloc()
								c.currentSymbolTable.Define(s.Name.Value, reg)
								c.regAlloc.Pin(reg)
							}
						}
					case *parser.ConstStatement:
						if s.Name != nil {
							if _, alreadyInCurrentScope := c.currentSymbolTable.store[s.Name.Value]; !alreadyInCurrentScope {
								reg := c.regAlloc.Alloc()
								c.currentSymbolTable.Define(s.Name.Value, reg)
								c.regAlloc.Pin(reg)
							}
						}
					}
				}

				// Compile catch body - track completion value in hint
				for _, stmt := range node.CatchClause.Body.Statements {
					stmtReg, err := c.compileNode(stmt, hint)
					if err != nil {
						c.currentSymbolTable = previousSymbolTable
						return BadRegister, err
					}
					if stmtReg != BadRegister && stmtReg != hint {
						c.emitMove(hint, stmtReg, node.Token.Line)
					}
				}

				// Restore the previous symbol table
				c.currentSymbolTable = previousSymbolTable
			} else {
				// Catch without parameter (ES2019+) - still need enclosed scope for block scoping
				catchScopePrev := c.currentSymbolTable
				c.currentSymbolTable = NewEnclosedSymbolTable(catchScopePrev)

				// Pre-define let/const in the catch scope
				for _, stmt := range node.CatchClause.Body.Statements {
					switch s := stmt.(type) {
					case *parser.LetStatement:
						if s.Name != nil {
							if _, alreadyInCurrentScope := c.currentSymbolTable.store[s.Name.Value]; !alreadyInCurrentScope {
								reg := c.regAlloc.Alloc()
								c.currentSymbolTable.Define(s.Name.Value, reg)
								c.regAlloc.Pin(reg)
							}
						}
					case *parser.ConstStatement:
						if s.Name != nil {
							if _, alreadyInCurrentScope := c.currentSymbolTable.store[s.Name.Value]; !alreadyInCurrentScope {
								reg := c.regAlloc.Alloc()
								c.currentSymbolTable.Define(s.Name.Value, reg)
								c.regAlloc.Pin(reg)
							}
						}
					}
				}

				// Compile catch body
				for _, stmt := range node.CatchClause.Body.Statements {
					stmtReg, err := c.compileNode(stmt, hint)
					if err != nil {
						c.currentSymbolTable = catchScopePrev
						return BadRegister, err
					}
					if stmtReg != BadRegister && stmtReg != hint {
						c.emitMove(hint, stmtReg, node.Token.Line)
					}
				}

				// Restore scope
				c.currentSymbolTable = catchScopePrev
			}

			// Add catch handler to exception table
			catchHandler := vm.ExceptionHandler{
				TryStart:   tryStart,
				TryEnd:     tryEnd,
				HandlerPC:  catchPC,
				CatchReg:   int(catchReg),
				IsCatch:    true,
				IsFinally:  false,
				FinallyReg: -1,
			}
			c.chunk.ExceptionTable = append(c.chunk.ExceptionTable, catchHandler)
		}

		c.patchJump(normalExitJump)
	}

	return hint, nil
}

// compileThrowStatement compiles a throw statement
func (c *Compiler) compileThrowStatement(node *parser.ThrowStatement, hint Register) (Register, errors.PaseratiError) {
	// Compile the expression being thrown
	valueReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(valueReg)

	if _, err := c.compileNode(node.Value, valueReg); err != nil {
		return BadRegister, err
	}

	// Clean up any active iterators before throwing from within a loop
	for i := len(c.loopContextStack) - 1; i >= 0; i-- {
		ctx := c.loopContextStack[i]
		if ctx.IteratorCleanup != nil && ctx.IteratorCleanup.UsesIteratorProtocol {
			c.emitIteratorCleanup(ctx.IteratorCleanup.IteratorReg, node.Token.Line)
		}
	}

	// Emit OpThrow instruction using the value register directly
	// Note: Previously this moved to R0, but that corrupts function parameters in R0
	c.emitOpCode(vm.OpThrow, node.Token.Line)
	c.emitByte(byte(valueReg))

	return BadRegister, nil
}

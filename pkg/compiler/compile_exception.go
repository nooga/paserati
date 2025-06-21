package compiler

import (
	"paserati/pkg/errors"
	"paserati/pkg/parser"
	"paserati/pkg/vm"
)

// --- Exception Handling Compilation ---

// compileTryStatement compiles a try/catch/finally statement (Phase 3 design)
func (c *Compiler) compileTryStatement(node *parser.TryStatement, hint Register) (Register, errors.PaseratiError) {
	tryStart := len(c.chunk.Code)
	
	// Compile try body
	bodyReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(bodyReg)
	
	if _, err := c.compileNode(node.Body, bodyReg); err != nil {
		return BadRegister, err
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
				// Define catch parameter in a new scope
				prevSymbolTable := c.currentSymbolTable
				c.currentSymbolTable = NewEnclosedSymbolTable(prevSymbolTable)
				c.currentSymbolTable.Define(node.CatchClause.Parameter.Value, catchReg)
				
				// Compile catch body
				if _, err := c.compileNode(node.CatchClause.Body, bodyReg); err != nil {
					c.currentSymbolTable = prevSymbolTable
					return BadRegister, err
				}
				
				// Restore symbol table
				c.currentSymbolTable = prevSymbolTable
			} else {
				// Catch without parameter (ES2019+)
				if _, err := c.compileNode(node.CatchClause.Body, bodyReg); err != nil {
					return BadRegister, err
				}
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
		
		// Patch jumps to finally BEFORE compiling the finally block
		c.patchJump(normalExitJump)
		if catchAfterJump != 0 {
			c.patchJump(catchAfterJump)
		}
		
		if _, err := c.compileNode(node.FinallyBlock, bodyReg); err != nil {
			return BadRegister, err
		}
		
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
				// Define catch parameter in a new scope
				prevSymbolTable := c.currentSymbolTable
				c.currentSymbolTable = NewEnclosedSymbolTable(prevSymbolTable)
				c.currentSymbolTable.Define(node.CatchClause.Parameter.Value, catchReg)
				
				// Compile catch body
				if _, err := c.compileNode(node.CatchClause.Body, bodyReg); err != nil {
					c.currentSymbolTable = prevSymbolTable
					return BadRegister, err
				}
				
				// Restore symbol table
				c.currentSymbolTable = prevSymbolTable
			} else {
				// Catch without parameter (ES2019+)
				if _, err := c.compileNode(node.CatchClause.Body, bodyReg); err != nil {
					return BadRegister, err
				}
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
	
	return BadRegister, nil
}

// compileThrowStatement compiles a throw statement
func (c *Compiler) compileThrowStatement(node *parser.ThrowStatement, hint Register) (Register, errors.PaseratiError) {
	// Compile the expression being thrown
	throwReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(throwReg)
	
	if _, err := c.compileNode(node.Value, throwReg); err != nil {
		return BadRegister, err
	}
	
	// Emit OpThrow instruction
	c.emitOpCode(vm.OpThrow, node.Token.Line)
	c.emitByte(byte(throwReg))
	
	return BadRegister, nil
}
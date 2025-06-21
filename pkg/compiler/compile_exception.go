package compiler

import (
	"paserati/pkg/errors"
	"paserati/pkg/parser"
	"paserati/pkg/vm"
)

// --- Exception Handling Compilation ---

// compileTryStatement compiles a try/catch statement according to Phase 1 design
func (c *Compiler) compileTryStatement(node *parser.TryStatement, hint Register) (Register, errors.PaseratiError) {
	tryStart := len(c.chunk.Code)
	
	// Compile try body
	bodyReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(bodyReg)
	
	if _, err := c.compileNode(node.Body, bodyReg); err != nil {
		return BadRegister, err
	}
	
	// The tryEnd should include the jump instruction that exits the try block
	// This ensures that function calls within the try block that return to 
	// the instruction after the call are still covered by the exception handler
	normalExit := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
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
		
		// Add to exception table
		handler := vm.ExceptionHandler{
			TryStart:  tryStart,
			TryEnd:    tryEnd,
			HandlerPC: catchPC,
			CatchReg:  int(catchReg),
			IsCatch:   true,
		}
		c.chunk.ExceptionTable = append(c.chunk.ExceptionTable, handler)
	}
	
	c.patchJump(normalExit)
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
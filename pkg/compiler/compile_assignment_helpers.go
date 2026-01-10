package compiler

import (
	"fmt"

	"github.com/nooga/paserati/pkg/vm"
)

// emitDestructuringNullCheck emits bytecode to check if valueReg is null or undefined
// and throws TypeError if so. This is required by ECMAScript for destructuring operations.
//
// The check is done at runtime even if type checker catches it at compile time,
// because JavaScript allows null/undefined to be passed despite type annotations.
func (c *Compiler) emitDestructuringNullCheck(valueReg Register, line int) {
	if debugCompiler {
		fmt.Printf("// [emitDestructuringNullCheck] Emitting null/undefined check for R%d\n", valueReg)
	}

	// Allocate register for null/undefined checks
	checkReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(checkReg)

	// Check if valueReg is null
	nullConstIdx := c.chunk.AddConstant(vm.Null)
	c.emitLoadConstant(checkReg, nullConstIdx, line)
	c.emitOpCode(vm.OpEqual, line)
	c.emitByte(byte(checkReg)) // result register
	c.emitByte(byte(valueReg)) // left operand
	c.emitByte(byte(checkReg)) // right operand (null)

	// Jump past error if not null
	notNullJump := c.emitPlaceholderJump(vm.OpJumpIfFalse, checkReg, line)

	// Throw TypeError: Cannot destructure null
	errorReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(errorReg)
	typeErrorGlobalIdx := c.GetOrAssignGlobalIndex("TypeError")
	c.emitGetGlobal(errorReg, typeErrorGlobalIdx, line)

	msgReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(msgReg)
	msgConstIdx := c.chunk.AddConstant(vm.String("Cannot destructure 'null'"))
	c.emitLoadConstant(msgReg, msgConstIdx, line)

	resultReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(resultReg)
	c.emitCall(resultReg, errorReg, 1, line) // Call TypeError constructor with message
	c.emitOpCode(vm.OpThrow, line)
	c.emitByte(byte(resultReg))

	// Patch jump for not-null case
	c.patchJump(notNullJump)

	// Check if valueReg is undefined
	undefConstIdx := c.chunk.AddConstant(vm.Undefined)
	c.emitLoadConstant(checkReg, undefConstIdx, line)
	c.emitOpCode(vm.OpEqual, line)
	c.emitByte(byte(checkReg)) // result register
	c.emitByte(byte(valueReg)) // left operand
	c.emitByte(byte(checkReg)) // right operand (undefined)

	// Jump past error if not undefined
	notUndefJump := c.emitPlaceholderJump(vm.OpJumpIfFalse, checkReg, line)

	// Throw TypeError: Cannot destructure undefined
	c.emitGetGlobal(errorReg, typeErrorGlobalIdx, line)
	msgConstIdx = c.chunk.AddConstant(vm.String("Cannot destructure 'undefined'"))
	c.emitLoadConstant(msgReg, msgConstIdx, line)
	c.emitCall(resultReg, errorReg, 1, line) // Call TypeError constructor with message
	c.emitOpCode(vm.OpThrow, line)
	c.emitByte(byte(resultReg))

	// Patch jump for not-undefined case
	c.patchJump(notUndefJump)
}

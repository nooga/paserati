package compiler

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/vm"
)

// compileIteratorNext calls iterator.next() and extracts the value
// If destReg is BadRegister, the value is discarded (for elisions)
// If discardValue is true, we only care about advancing the iterator
// If doneReg is not BadRegister, also extracts and stores result.done
func (c *Compiler) compileIteratorNext(iteratorReg Register, destReg Register, doneReg Register, line int, discardValue bool) {
	// Get iterator.next method
	nextMethodReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(nextMethodReg)
	nextConstIdx := c.chunk.AddConstant(vm.String("next"))
	c.emitGetProp(nextMethodReg, iteratorReg, nextConstIdx, line)

	// Call iterator.next() to get {value, done}
	resultReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(resultReg)
	c.emitCallMethod(resultReg, nextMethodReg, iteratorReg, 0, line)

	// Extract result.value if needed
	if !discardValue && destReg != BadRegister {
		valueConstIdx := c.chunk.AddConstant(vm.String("value"))
		c.emitGetProp(destReg, resultReg, valueConstIdx, line)
	}

	// Extract result.done if tracking
	if doneReg != BadRegister {
		doneConstIdx := c.chunk.AddConstant(vm.String("done"))
		c.emitGetProp(doneReg, resultReg, doneConstIdx, line)
	}
}

// compileIteratorToArray collects all remaining values from iterator into an array
// Used for rest elements: let [...rest] = iterable
func (c *Compiler) compileIteratorToArray(iteratorReg Register, destReg Register, line int) errors.PaseratiError {
	// Create empty array using OpMakeArray
	// Format: OpMakeArray destReg, startReg, count
	// For empty array: use any register as start (we use 0) with count 0
	c.emitOpCode(vm.OpMakeArray, line)
	c.emitByte(byte(destReg))
	c.emitByte(0) // start register (unused for count=0)
	c.emitByte(0) // count: 0 elements

	// Get iterator.next method once (optimization)
	nextMethodReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(nextMethodReg)
	nextConstIdx := c.chunk.AddConstant(vm.String("next"))
	c.emitGetProp(nextMethodReg, iteratorReg, nextConstIdx, line)

	// Loop: while (!result.done) { array.push(result.value); }
	loopStart := len(c.chunk.Code)

	// Call iterator.next()
	resultReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(resultReg)
	c.emitCallMethod(resultReg, nextMethodReg, iteratorReg, 0, line)

	// Get result.done
	doneReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(doneReg)
	doneConstIdx := c.chunk.AddConstant(vm.String("done"))
	c.emitGetProp(doneReg, resultReg, doneConstIdx, line)

	// Negate done to check if NOT done (continue looping)
	notDoneReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(notDoneReg)
	c.emitOpCode(vm.OpNot, line)
	c.emitByte(byte(notDoneReg))
	c.emitByte(byte(doneReg))

	// Exit loop if NOT not-done (i.e., if done)
	exitJump := c.emitPlaceholderJump(vm.OpJumpIfFalse, notDoneReg, line)

	// Get result.value
	valueReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(valueReg)
	valueConstIdx := c.chunk.AddConstant(vm.String("value"))
	c.emitGetProp(valueReg, resultReg, valueConstIdx, line)

	// Call array.push(value)
	// For OpCallMethod, arguments must be in consecutive registers starting at funcReg+1
	// Use AllocContiguous to ensure we get consecutive registers
	pushMethodReg := c.regAlloc.AllocContiguous(3)
	pushArgReg := pushMethodReg + 1 // Must be pushMethodReg+1 for OpCallMethod
	pushResultReg := pushMethodReg + 2

	pushConstIdx := c.chunk.AddConstant(vm.String("push"))
	c.emitGetProp(pushMethodReg, destReg, pushConstIdx, line)

	// Move value to argument position (pushMethodReg+1)
	c.emitMove(pushArgReg, valueReg, line)

	// Call push method with 1 argument
	c.emitCallMethod(pushResultReg, pushMethodReg, destReg, 1, line)

	// Free immediately - don't wait for defer at end of function
	c.regAlloc.Free(pushResultReg)
	c.regAlloc.Free(pushArgReg)
	c.regAlloc.Free(pushMethodReg)

	// Jump back to loop start
	jumpBackPos := len(c.chunk.Code) + 1 + 2
	backOffset := loopStart - jumpBackPos
	c.emitOpCode(vm.OpJump, line)
	c.emitUint16(uint16(int16(backOffset)))

	// Patch exit jump
	c.patchJump(exitJump)

	return nil
}

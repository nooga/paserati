package compiler

import "github.com/nooga/paserati/pkg/vm"

// emitGetAsyncIterator emits GetIterator(iterable, async) into destReg:
// iterable[Symbol.asyncIterator]() when that method is present, otherwise
// CreateAsyncFromSyncIterator(iterable[Symbol.iterator]()). Both results
// must be objects.
func (c *Compiler) emitGetAsyncIterator(iterableReg, destReg Register, line int) {
	tmp := c.regAlloc.Alloc()
	keyReg := c.regAlloc.Alloc()
	methodReg := c.regAlloc.Alloc()
	cmpReg := c.regAlloc.Alloc()
	defer func() {
		c.regAlloc.Free(cmpReg)
		c.regAlloc.Free(methodReg)
		c.regAlloc.Free(keyReg)
		c.regAlloc.Free(tmp)
	}()

	loadWellKnown := func(name string) {
		c.emitGetGlobal(tmp, c.GetOrAssignGlobalIndex("Symbol"), line)
		c.emitLoadNewConstant(keyReg, vm.String(name), line)
		c.emitOpCode(vm.OpGetIndex, line)
		c.emitByte(byte(keyReg))
		c.emitByte(byte(tmp))
		c.emitByte(byte(keyReg))
	}

	// method = GetMethod(iterable, @@asyncIterator)
	loadWellKnown("asyncIterator")
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(methodReg))
	c.emitByte(byte(iterableReg))
	c.emitByte(byte(keyReg))

	// undefined or null (loose equality) falls back to the sync iterator
	c.emitLoadUndefined(tmp, line)
	c.emitOpCode(vm.OpEqual, line)
	c.emitByte(byte(cmpReg))
	c.emitByte(byte(methodReg))
	c.emitByte(byte(tmp))
	toAsync := c.emitPlaceholderJump(vm.OpJumpIfFalse, cmpReg, line)

	loadWellKnown("iterator")
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(methodReg))
	c.emitByte(byte(iterableReg))
	c.emitByte(byte(keyReg))
	c.emitCallMethod(destReg, methodReg, iterableReg, 0, line)
	c.emitOpCode(vm.OpTypeGuardIteratorReturn, line)
	c.emitByte(byte(destReg))
	c.emitOpCode(vm.OpAsyncFromSyncIterator, line)
	c.emitByte(byte(destReg))
	c.emitByte(byte(destReg))
	toDone := c.emitPlaceholderJump(vm.OpJump, 0, line)

	c.patchJump(toAsync)
	c.emitCallMethod(destReg, methodReg, iterableReg, 0, line)
	c.emitOpCode(vm.OpTypeGuardIteratorReturn, line)
	c.emitByte(byte(destReg))

	c.patchJump(toDone)
}

// compileAsyncYieldDelegation compiles yield* in an async generator. The
// delegation loop itself runs in the VM's async generator driver
// (OpAsyncYieldStar), which awaits each inner result and forwards
// next/throw/return requests to the inner iterator.
func (c *Compiler) compileAsyncYieldDelegation(iterableReg Register, hint Register, line int) Register {
	iterReg := c.regAlloc.Alloc()
	nextReg := c.regAlloc.Alloc()
	defer func() {
		c.regAlloc.Free(nextReg)
		c.regAlloc.Free(iterReg)
	}()
	c.emitGetAsyncIterator(iterableReg, iterReg, line)
	c.emitGetProp(nextReg, iterReg, c.chunk.AddConstant(vm.String("next")), line)
	c.emitOpCode(vm.OpAsyncYieldStar, line)
	c.emitByte(byte(hint))
	c.emitByte(byte(iterReg))
	c.emitByte(byte(nextReg))
	return hint
}

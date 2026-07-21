package compiler

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

// compileIteratorNext calls iterator.next() and extracts the value
// If destReg is BadRegister, the value is discarded (for elisions)
// If discardValue is true, we only care about advancing the iterator
// If doneReg is not BadRegister, also extracts and stores result.done
//
// Per ECMAScript spec (IteratorStep), if next() throws, done should be set to true
// BEFORE the exception propagates. We achieve this by setting done=true before the
// call and only updating it to the actual result.done after a successful call.
func (c *Compiler) compileIteratorNext(iteratorReg Register, destReg Register, doneReg Register, line int, discardValue bool) {
	// Get iterator.next method
	nextMethodReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(nextMethodReg)
	nextConstIdx := c.chunk.AddConstant(vm.String("next"))
	c.emitGetProp(nextMethodReg, iteratorReg, nextConstIdx, line)

	// Per ECMAScript spec IteratorStep: if next() throws, set done=true first.
	// We do this pessimistically: set done=true BEFORE the call, then update
	// it to the actual value if the call succeeds.
	if doneReg != BadRegister {
		c.emitLoadTrue(doneReg, line)
	}

	// Call iterator.next() to get {value, done}
	resultReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(resultReg)
	c.emitCallMethod(resultReg, nextMethodReg, iteratorReg, 0, line)

	// Extract result.value if needed
	if !discardValue && destReg != BadRegister {
		valueConstIdx := c.chunk.AddConstant(vm.String("value"))
		c.emitGetProp(destReg, resultReg, valueConstIdx, line)
	}

	// Extract result.done if tracking - this overwrites the pessimistic true
	// with the actual value after successful next() call
	if doneReg != BadRegister {
		doneConstIdx := c.chunk.AddConstant(vm.String("done"))
		c.emitGetProp(doneReg, resultReg, doneConstIdx, line)
	}
}

// arrayDestructState threads the registers used by an array-destructuring site.
// When fastEnabled is true, a runtime check (fastFlagReg) selects per element
// between reading the source array by index (no iterator, no next() call, no
// {value, done} allocation) and the generic iterator protocol. When false, only
// the generic protocol is emitted - byte-for-byte the pre-fast-path behaviour.
type arrayDestructState struct {
	fastEnabled bool
	fastFlagReg Register // BadRegister when !fastEnabled
	iterObjReg  Register // the iterator object (generic arm only; undefined on the fast arm)
	doneReg     Register // iterator done flag; forced true on the fast arm so cleanup is a no-op
}

// emitBuildArrayIterator resolves src[Symbol.iterator], calls it, and leaves the
// iterator object in iterObjReg. Factored out of the destructuring paths, which
// each open-coded this identical sequence.
func (c *Compiler) emitBuildArrayIterator(iterObjReg Register, src Register, line int) {
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

	iteratorMethodReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(iteratorMethodReg)
	c.emitOpCode(vm.OpGetIndex, line)
	c.emitByte(byte(iteratorMethodReg))
	c.emitByte(byte(src))
	c.emitByte(byte(iteratorKeyReg))

	c.emitCallMethod(iterObjReg, iteratorMethodReg, src, 0, line)
}

// beginArrayDestruct sets up an array-destructuring site over srcReg. It
// allocates iterObjReg and doneReg (and, when enableFast, fastFlagReg) and emits
// the branched setup: the fast arm skips iterator creation and marks done=true;
// the generic arm builds the iterator and sets done=false. The caller must pair
// this with endArrayDestruct to free the registers.
func (c *Compiler) beginArrayDestruct(srcReg Register, enableFast bool, line int) arrayDestructState {
	st := arrayDestructState{fastEnabled: enableFast, fastFlagReg: BadRegister}
	st.iterObjReg = c.regAlloc.Alloc()
	st.doneReg = c.regAlloc.Alloc()

	if !enableFast {
		c.emitBuildArrayIterator(st.iterObjReg, srcReg, line)
		c.emitLoadFalse(st.doneReg, line)
		return st
	}

	st.fastFlagReg = c.regAlloc.Alloc()
	c.emitArrayDestructFastCheck(st.fastFlagReg, srcReg, line)
	// The fast arm never dereferences iterObjReg (done=true skips cleanup), but
	// give it a defined value so the register is never read while uninitialized.
	c.emitLoadUndefined(st.iterObjReg, line)

	toGeneric := c.emitPlaceholderJump(vm.OpJumpIfFalse, st.fastFlagReg, line)
	c.emitLoadTrue(st.doneReg, line) // fast arm: no iterator to close
	pastSetup := c.emitPlaceholderJump(vm.OpJump, 0, line)

	c.patchJump(toGeneric)
	c.emitBuildArrayIterator(st.iterObjReg, srcReg, line)
	c.emitLoadFalse(st.doneReg, line)
	c.patchJump(pastSetup)
	return st
}

func (c *Compiler) endArrayDestruct(st arrayDestructState) {
	if st.fastFlagReg != BadRegister {
		c.regAlloc.Free(st.fastFlagReg)
	}
	c.regAlloc.Free(st.doneReg)
	c.regAlloc.Free(st.iterObjReg)
}

// compileDestructNext produces the idx-th destructured value into destReg,
// advancing the iterator by one. idx is the element's position in the pattern
// (counting elisions). On the fast arm the value is read directly from the
// source array; on the generic arm it comes from iterator.next(). When discard
// is true (elision), the fast arm does nothing and the generic arm still steps
// the iterator to stay in sync.
func (c *Compiler) compileDestructNext(st arrayDestructState, srcReg Register, idx int, destReg Register, discard bool, line int) {
	if !st.fastEnabled {
		c.compileIteratorNext(st.iterObjReg, destReg, st.doneReg, line, discard)
		return
	}
	toGeneric := c.emitPlaceholderJump(vm.OpJumpIfFalse, st.fastFlagReg, line)
	if !discard && destReg != BadRegister {
		c.emitArrayRawGetInt(destReg, srcReg, idx, line)
	}
	pastGeneric := c.emitPlaceholderJump(vm.OpJump, 0, line)
	c.patchJump(toGeneric)
	c.compileIteratorNext(st.iterObjReg, destReg, st.doneReg, line, discard)
	c.patchJump(pastGeneric)
}

// arrayDeclPatternFastEligible reports whether an array pattern can use the fast
// path: no rest element (rest still exhausts the iterator via the generic path).
func arrayDeclPatternFastEligible(elements []*parser.DestructuringElement) bool {
	for _, el := range elements {
		if el.IsRest {
			return false
		}
	}
	return true
}

// compileIteratorToArray collects all remaining values from iterator into an array
// Used for rest elements: let [...rest] = iterable
// This version doesn't update an external done register. Use compileIteratorToArrayWithDone
// when you need to track done state for exception handling.
func (c *Compiler) compileIteratorToArray(iteratorReg Register, destReg Register, line int) errors.PaseratiError {
	return c.compileIteratorToArrayWithDone(iteratorReg, destReg, BadRegister, line)
}

// compileIteratorToArrayWithDone collects all remaining values from iterator into an array
// and optionally updates an external done register for exception handling purposes.
// If externalDoneReg is not BadRegister, it will be set to true before each next() call
// and updated with the actual result.done after successful calls.
func (c *Compiler) compileIteratorToArrayWithDone(iteratorReg Register, destReg Register, externalDoneReg Register, line int) errors.PaseratiError {
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

	// Per ECMAScript spec: if next() throws, done should be true.
	// Set externalDoneReg=true pessimistically before the call.
	if externalDoneReg != BadRegister {
		c.emitLoadTrue(externalDoneReg, line)
	}

	// Call iterator.next()
	resultReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(resultReg)
	c.emitCallMethod(resultReg, nextMethodReg, iteratorReg, 0, line)

	// Get result.done
	localDoneReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(localDoneReg)
	doneConstIdx := c.chunk.AddConstant(vm.String("done"))
	c.emitGetProp(localDoneReg, resultReg, doneConstIdx, line)

	// Update external done register with actual value after successful next()
	if externalDoneReg != BadRegister {
		c.emitMove(externalDoneReg, localDoneReg, line)
	}

	// Negate done to check if NOT done (continue looping)
	notDoneReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(notDoneReg)
	c.emitOpCode(vm.OpNot, line)
	c.emitByte(byte(notDoneReg))
	c.emitByte(byte(localDoneReg))

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

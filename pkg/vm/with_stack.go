package vm

// The with-object stack holds the objects of the `with` statements being
// executed. Each entry records the frame depth (vm.frameCount) that pushed
// it, so a frame sees only its own entries - plus those its closure captured
// lexically (ClosureObject.WithObjects) - and entries outlive neither the
// frame that pushed them (return, throw out of the frame, generator
// suspension) nor the block (break/continue/catch, via OpTruncateWith).
//
// Entries of exited frames are dropped lazily: before a frame is pushed (so a
// new frame at the same depth never inherits them) and before the stack is
// read or pushed. Depths therefore never decrease towards the top.

// trimFrameStacks drops with-stack and private-environment entries pushed
// by frames that have exited. Called before every frame push.
func (vm *VM) trimFrameStacks() {
	if n := len(vm.withObjectDepth); n != 0 && vm.withObjectDepth[n-1] > vm.frameCount {
		vm.trimWithStackSlow()
	}
	if n := len(vm.privateEnvStack); n != 0 && vm.privateEnvStack[n-1].depth > vm.frameCount {
		vm.trimPrivateEnvStack()
	}
}

func (vm *VM) trimWithStackSlow() {
	n := len(vm.withObjectDepth)
	keep := n
	for keep > 0 && vm.withObjectDepth[keep-1] > vm.frameCount {
		keep--
	}
	clear(vm.withObjectStack[keep:n])
	vm.withObjectStack = vm.withObjectStack[:keep]
	vm.withObjectDepth = vm.withObjectDepth[:keep]
}

// frameWithObjects returns the current frame's with objects, innermost last.
func (vm *VM) frameWithObjects() []Value {
	vm.trimFrameStacks()
	n := len(vm.withObjectDepth)
	i := n
	for i > 0 && vm.withObjectDepth[i-1] == vm.frameCount {
		i--
	}
	return vm.withObjectStack[i:n]
}

// pushWithObject pushes obj for the current frame.
func (vm *VM) pushWithObject(obj Value) {
	vm.trimFrameStacks()
	vm.withObjectStack = append(vm.withObjectStack, obj)
	vm.withObjectDepth = append(vm.withObjectDepth, vm.frameCount)
}

// truncateFrameWithObjects keeps the current frame's first keep entries.
func (vm *VM) truncateFrameWithObjects(keep int) {
	own := vm.frameWithObjects()
	if keep < 0 || keep >= len(own) {
		return
	}
	n := len(vm.withObjectStack) - (len(own) - keep)
	clear(vm.withObjectStack[n:])
	vm.withObjectStack = vm.withObjectStack[:n]
	vm.withObjectDepth = vm.withObjectDepth[:n]
}

// saveFrameWithObjects copies the current frame's entries for a generator or
// async function that is about to suspend.
func (vm *VM) saveFrameWithObjects() []Value {
	own := vm.frameWithObjects()
	if len(own) == 0 {
		return nil
	}
	return append([]Value(nil), own...)
}

// restoreFrameWithObjects re-pushes a resumed frame's saved entries; the
// frame must already be the current one.
func (vm *VM) restoreFrameWithObjects(saved []Value) {
	for _, obj := range saved {
		vm.pushWithObject(obj)
	}
}

// closureWithObjects is what a closure created now captures: its creator's
// captured with objects followed by the creator frame's own.
func (vm *VM) closureWithObjects(creator *ClosureObject) []Value {
	own := vm.frameWithObjects()
	var outer []Value
	if creator != nil {
		outer = creator.WithObjects
	}
	if len(own) == 0 {
		return outer
	}
	captured := make([]Value, 0, len(outer)+len(own))
	captured = append(captured, outer...)
	return append(captured, own...)
}

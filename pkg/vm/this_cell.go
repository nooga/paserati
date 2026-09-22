package vm

// thisCell is a derived constructor's `this` binding, shared by reference
// with the arrow functions that close over it.
//
// An arrow resolves `this` through its enclosing function's environment
// record when it runs, not when it is created (ECMAScript 9.4.4
// ResolveThisBinding). Arrows used to snapshot the value into CapturedThis
// at creation, so an arrow created in a derived constructor before super()
// kept the TypeUninitialized sentinel forever and threw "Must call super
// constructor..." even when called long after super() returned
// (paserati#520 - tar's Unpack sets `t.ondone = () => {...this...}` before
// super(t)).
//
// A cell is only allocated for that case: when an arrow is created while the
// enclosing `this` is still uninitialized. The constructor frame owns it
// (CallFrame.thisCell) and arrows reference it (ClosureObject.
// CapturedThisCell); an arrow created inside such an arrow shares the same
// cell. super() - in the constructor or in any of those arrows - writes it.
// Every other function keeps the plain by-value capture.
//
// CallFrame.thisCell is only ever read while that frame's `this` is still
// uninitialized, which only a derived-constructor frame's can be, so only the
// constructor push sites reset it.
type thisCell struct {
	value Value
}

// lexicalThisCell is the cell frame's `this` resolves through, or nil.
func (frame *CallFrame) lexicalThisCell() *thisCell {
	if frame.closure != nil && frame.closure.Fn != nil && frame.closure.Fn.IsArrowFunction {
		return frame.closure.CapturedThisCell
	}
	return frame.thisCell
}

// currentThis is frame's `this` binding as of now. While frame.thisValue is
// still uninitialized it re-reads the shared cell, since super() may have
// run since the frame (or its arrow closure) captured the value.
func (frame *CallFrame) currentThis() Value {
	if frame.thisValue.typ == TypeUninitialized {
		if c := frame.lexicalThisCell(); c != nil && c.value.typ != TypeUninitialized {
			frame.thisValue = c.value
		}
	}
	return frame.thisValue
}

// captureArrowThis records the enclosing `this` on a new arrow closure: by
// value once it is initialized, otherwise through the shared cell,
// allocating it on the constructor frame the first time.
func (frame *CallFrame) captureArrowThis(cl *ClosureObject) {
	this := frame.currentThis()
	cl.CapturedThis = this
	if this.typ != TypeUninitialized {
		return
	}
	c := frame.lexicalThisCell()
	if c == nil {
		if frame.closure != nil && frame.closure.Fn != nil && frame.closure.Fn.IsArrowFunction {
			// An uninitialized arrow frame without a cell can't arise from
			// bytecode; keep the old by-value behavior rather than invent one.
			return
		}
		c = &thisCell{value: Uninitialized}
		frame.thisCell = c
	}
	cl.CapturedThisCell = c
}

// bindThisFromArrowSuper handles super() called inside an arrow whose `this`
// is a shared cell. It reports false when the binding is already
// initialized ("super() already called"). The owning constructor frame, if
// still on the stack, gets the value directly, so its own return paths see
// it without consulting the cell.
func (vm *VM) bindThisFromArrowSuper(frame *CallFrame, cell *thisCell, v Value) bool {
	if cell.value.typ != TypeUninitialized {
		return false
	}
	for i := vm.frameCount - 2; i >= 0; i-- {
		owner := &vm.frames[i]
		if owner.thisCell == cell && !owner.closureIsArrow() {
			if owner.thisValue.typ != TypeUninitialized {
				return false
			}
			owner.thisValue = v
			break
		}
	}
	cell.value = v
	frame.thisValue = v
	return true
}

func (frame *CallFrame) closureIsArrow() bool {
	return frame.closure != nil && frame.closure.Fn != nil && frame.closure.Fn.IsArrowFunction
}

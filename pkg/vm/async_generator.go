package vm

// Async generators (ECMAScript 27.6).
//
// The body runs on the ordinary generator machinery (executeGenerator and
// friends, OpYield), but it is driven from here rather than by the caller of
// next(): every request goes into [[AsyncGeneratorQueue]] and gets its own
// promise, the body suspends at `await` (OpAwait parks the frame and sets
// asyncAwaiting) as well as at `yield`, and yield* delegation (OpAsyncYieldStar)
// is run by agDelegate, which awaits the inner iterator's results.

// asyncGenState is [[AsyncGeneratorState]].
type asyncGenState uint8

const (
	agSuspendedStart asyncGenState = iota
	agSuspendedYield
	agExecuting
	agDrainingQueue
	agCompleted
)

// asyncGenKind is the completion type of a queued request or resumption.
type asyncGenKind uint8

const (
	agNormal asyncGenKind = iota
	agThrow
	agReturn
)

// asyncGenRequest is an AsyncGeneratorRequest record.
type asyncGenRequest struct {
	kind    asyncGenKind
	value   Value
	promise *PromiseObject
}

// asyncGenPending is the finally-block bookkeeping (vm.pendingAction and
// friends) that belongs to a suspended async generator body. Those are VM
// globals, so they are swapped in and out around every run of the body.
type asyncGenPending struct {
	action       PendingAction
	value        Value
	finallyDepth int
}

func isObjectLike(v Value) bool { return v.IsObject() || v.IsCallable() }

func (vm *VM) newIntrinsicPromise() *PromiseObject {
	return vm.NewPendingPromise().AsPromise()
}

func (vm *VM) newIterResult(value Value, done bool) Value {
	o := NewObject(vm.ObjectPrototype).AsPlainObject()
	o.SetOwn("value", value)
	o.SetOwn("done", BooleanValue(done))
	return NewValueFromPlainObject(o)
}

func (vm *VM) typeErrorValue(msg string) Value {
	return vm.ExceptionValueFromError(vm.NewTypeError(msg))
}

// awaitPromiseResolve is PromiseResolve(%Promise%, v), the first step of
// Await: a native promise whose constructor is %Promise% is used as is,
// anything else is wrapped in a new promise resolved with it.
func (vm *VM) awaitPromiseResolve(v Value) (*PromiseObject, error) {
	if v.Type() == TypePromise {
		if vm.PromiseConstructor.Type() == TypeUndefined {
			return v.AsPromise(), nil
		}
		ctor, err := vm.GetProperty(v, "constructor")
		if err != nil {
			return nil, err
		}
		if ctor.Is(vm.PromiseConstructor) {
			return v.AsPromise(), nil
		}
	}
	p := vm.newIntrinsicPromise()
	vm.resolvePromise(p, v)
	return p, nil
}

// thenGo is PerformPromiseThen with Go reactions and no result promise.
func (vm *VM) thenGo(p *PromiseObject, onFulfilled, onRejected func(Value)) {
	st := p.addAwaitReactions(
		PromiseReaction{Handler: Undefined, Resolve: onFulfilled, Reject: func(Value) {}},
		PromiseReaction{Handler: Undefined, Resolve: func(Value) {}, Reject: onRejected},
	)
	switch st {
	case PromiseFulfilled:
		vm.triggerPromiseReactions(p, true)
	case PromiseRejected:
		vm.triggerPromiseReactions(p, false)
	}
}

// AwaitThen is Await(v) for native code written in continuation-passing
// style: exactly one of onFulfilled/onRejected runs in a later job.
func (vm *VM) AwaitThen(v Value, onFulfilled, onRejected func(Value)) {
	p, err := vm.awaitPromiseResolve(v)
	if err != nil {
		vm.ClearUnwindingState()
		reason := vm.ExceptionValueFromError(err)
		// Await throws synchronously here, but a caller in CPS still expects
		// the rejection asynchronously; deliver it through a settled promise.
		p = vm.newIntrinsicPromise()
		vm.rejectPromise(p, reason)
	}
	vm.thenGo(p, onFulfilled, onRejected)
}

// getMethod is GetMethod(v, name): undefined/null yield Undefined, anything
// else non-callable is a TypeError.
func (vm *VM) getMethod(v Value, name string) (Value, error) {
	m, err := vm.GetProperty(v, name)
	if err != nil {
		return Undefined, err
	}
	if m.Type() == TypeUndefined || m.Type() == TypeNull {
		return Undefined, nil
	}
	if !m.IsCallable() {
		return Undefined, vm.NewTypeError(name + " is not a function")
	}
	return m, nil
}

// saveGeneratorFrame parks the running generator frame in genObj.Frame, the
// same way OpYield does, so the body can be resumed at ip later.
func (vm *VM) saveGeneratorFrame(genObj *GeneratorObject, frame *CallFrame, registers []Value, ip, suspendPC int, outputReg byte) {
	if genObj.Frame == nil || len(genObj.Frame.registers) != len(registers) {
		old := genObj.Frame
		genObj.Frame = &GeneratorFrame{registers: make([]Value, len(registers)), locals: make([]Value, 0)}
		if old != nil {
			genObj.Frame.stackBase = old.stackBase
		}
	}
	f := genObj.Frame
	f.pc = ip
	f.suspendPC = suspendPC
	f.outputReg = outputReg
	f.thisValue = frame.thisValue
	f.homeObject = frame.homeObject
	copy(f.registers, registers)
	f.openUpvalues = frame.openUpvalues
	relocateOpenUpvalues(f.openUpvalues, registers, f.registers)
	f.spillSlots = frame.spillSlots
}

func (vm *VM) asAsyncGen(this Value) (*GeneratorObject, bool) {
	if this.Type() != TypeAsyncGenerator {
		return nil, false
	}
	return (*GeneratorObject)(this.AsAsyncGenerator()), true
}

// AsyncGeneratorNext is %AsyncGeneratorPrototype%.next.
func (vm *VM) AsyncGeneratorNext(this Value, value Value) Value {
	promise := vm.newIntrinsicPromise()
	g, ok := vm.asAsyncGen(this)
	if !ok {
		vm.rejectPromise(promise, vm.typeErrorValue("AsyncGenerator.prototype.next called on incompatible receiver"))
		return promiseValue(promise)
	}
	state := g.asyncState
	if state == agCompleted {
		vm.resolvePromise(promise, vm.newIterResult(Undefined, true))
		return promiseValue(promise)
	}
	g.asyncQueue = append(g.asyncQueue, asyncGenRequest{kind: agNormal, value: value, promise: promise})
	if state == agSuspendedStart || state == agSuspendedYield {
		vm.agResume(g, agNormal, value)
	}
	return promiseValue(promise)
}

// AsyncGeneratorReturn is %AsyncGeneratorPrototype%.return.
func (vm *VM) AsyncGeneratorReturn(this Value, value Value) Value {
	promise := vm.newIntrinsicPromise()
	g, ok := vm.asAsyncGen(this)
	if !ok {
		vm.rejectPromise(promise, vm.typeErrorValue("AsyncGenerator.prototype.return called on incompatible receiver"))
		return promiseValue(promise)
	}
	state := g.asyncState
	g.asyncQueue = append(g.asyncQueue, asyncGenRequest{kind: agReturn, value: value, promise: promise})
	switch state {
	case agSuspendedStart, agCompleted:
		vm.agFinishBody(g)
		g.asyncState = agDrainingQueue
		vm.agAwaitReturn(g)
	case agSuspendedYield:
		vm.agResume(g, agReturn, value)
	}
	return promiseValue(promise)
}

// AsyncGeneratorThrow is %AsyncGeneratorPrototype%.throw.
func (vm *VM) AsyncGeneratorThrow(this Value, value Value) Value {
	promise := vm.newIntrinsicPromise()
	g, ok := vm.asAsyncGen(this)
	if !ok {
		vm.rejectPromise(promise, vm.typeErrorValue("AsyncGenerator.prototype.throw called on incompatible receiver"))
		return promiseValue(promise)
	}
	state := g.asyncState
	if state == agSuspendedStart {
		vm.agFinishBody(g)
		g.asyncState = agCompleted
		state = agCompleted
	}
	if state == agCompleted {
		vm.rejectPromise(promise, value)
		return promiseValue(promise)
	}
	g.asyncQueue = append(g.asyncQueue, asyncGenRequest{kind: agThrow, value: value, promise: promise})
	if state == agSuspendedYield {
		vm.agResume(g, agThrow, value)
	}
	return promiseValue(promise)
}

func promiseValue(p *PromiseObject) Value {
	return Value{typ: TypePromise, obj: promiseToUnsafe(p)}
}

// agFinishBody discards the body of a generator that will never run again.
func (vm *VM) agFinishBody(g *GeneratorObject) {
	g.State = GeneratorCompleted
	g.Done = true
	g.Frame = nil
	g.asyncDelegate = Undefined
	g.asyncDelegateNext = Undefined
	g.asyncDelegating = false
	g.asyncPending = asyncGenPending{}
}

// agResume resumes a suspended-start or suspended-yield generator with a
// completion (AsyncGeneratorResume). A return completion at a yield is
// awaited first (AsyncGeneratorUnwrapYieldResumption); inside yield* the
// completion goes to the delegate instead of the body.
func (vm *VM) agResume(g *GeneratorObject, kind asyncGenKind, value Value) {
	g.asyncState = agExecuting
	if kind == agReturn {
		vm.agAwaitThen(g, value, func(v Value) {
			if g.asyncDelegating {
				vm.agDelegate(g, agReturn, v)
				return
			}
			vm.agRun(g, agReturn, v)
		}, func(r Value) {
			if g.asyncDelegating {
				vm.agDelegate(g, agThrow, r)
				return
			}
			vm.agRun(g, agThrow, r)
		})
		return
	}
	if g.asyncDelegating {
		vm.agDelegate(g, kind, value)
		return
	}
	vm.agRun(g, kind, value)
}

// agAwaitThen is Await(v) outside the body's own frame: onFulfilled or
// onRejected runs in a later job.
func (vm *VM) agAwaitThen(g *GeneratorObject, v Value, onFulfilled, onRejected func(Value)) {
	p, err := vm.awaitPromiseResolve(v)
	if err != nil {
		vm.ClearUnwindingState()
		onRejected(vm.ExceptionValueFromError(err))
		return
	}
	vm.thenGo(p, onFulfilled, onRejected)
}

// agRun runs the body from its suspension point with the given completion
// and dispatches on where it stopped: an await, a yield, a yield*, a return
// or a throw.
func (vm *VM) agRun(g *GeneratorObject, kind asyncGenKind, value Value) {
	g.asyncState = agExecuting

	callerPending := asyncGenPending{vm.pendingAction, vm.pendingValue, vm.finallyDepth}
	vm.pendingAction, vm.pendingValue, vm.finallyDepth = g.asyncPending.action, g.asyncPending.value, g.asyncPending.finallyDepth
	if vm.pendingValue.Type() == 0 {
		vm.pendingValue = Undefined
	}
	fc, mark := vm.frameCount, vm.regDir.mark()

	var result Value
	var err error
	switch kind {
	case agNormal:
		result, err = vm.executeGenerator(g, value)
	case agThrow:
		result, err = vm.executeGeneratorWithException(g, value)
	case agReturn:
		result, err = vm.resumeGeneratorWithReturn(g, value)
	}

	if vm.frameCount > fc {
		vm.frameCount = fc
		vm.regDir.popTo(mark)
	}
	suspended := err == nil && g.State == GeneratorSuspendedYield
	if suspended {
		g.asyncPending = asyncGenPending{vm.pendingAction, vm.pendingValue, vm.finallyDepth}
	} else {
		g.asyncPending = asyncGenPending{}
	}
	vm.pendingAction, vm.pendingValue, vm.finallyDepth = callerPending.action, callerPending.value, callerPending.finallyDepth

	if err != nil {
		vm.ClearUnwindingState()
		vm.ClearErrors()
		vm.agFinishBody(g)
		g.asyncState = agDrainingQueue
		vm.agCompleteStep(g, agThrow, vm.ExceptionValueFromError(err), true)
		vm.agDrainQueue(g)
		return
	}
	if !suspended {
		retVal := Undefined
		if result.Type() == TypeObject {
			if v, ok := result.AsPlainObject().GetOwn("value"); ok {
				retVal = v
			}
		}
		vm.agFinishBody(g)
		g.asyncState = agDrainingQueue
		vm.agCompleteStep(g, agNormal, retVal, true)
		vm.agDrainQueue(g)
		return
	}
	if p := g.asyncAwaiting; p != nil {
		g.asyncAwaiting = nil
		vm.thenGo(p, func(v Value) { vm.agRun(g, agNormal, v) }, func(r Value) { vm.agRun(g, agThrow, r) })
		return
	}
	if g.asyncDelegating {
		vm.agDelegate(g, agNormal, Undefined)
		return
	}
	vm.agYield(g, g.YieldedValue)
}

// agYield is AsyncGeneratorYield: settle the oldest request with the value,
// then carry straight on with the next queued request, if any.
func (vm *VM) agYield(g *GeneratorObject, value Value) {
	vm.agCompleteStep(g, agNormal, value, false)
	if len(g.asyncQueue) > 0 {
		next := g.asyncQueue[0]
		vm.agResume(g, next.kind, next.value)
		return
	}
	g.asyncState = agSuspendedYield
}

// agCompleteStep is AsyncGeneratorCompleteStep.
func (vm *VM) agCompleteStep(g *GeneratorObject, kind asyncGenKind, value Value, done bool) {
	if len(g.asyncQueue) == 0 {
		return
	}
	next := g.asyncQueue[0]
	g.asyncQueue[0] = asyncGenRequest{}
	g.asyncQueue = g.asyncQueue[1:]
	if kind == agThrow {
		vm.rejectPromise(next.promise, value)
		return
	}
	vm.resolvePromise(next.promise, vm.newIterResult(value, done))
}

// agDrainQueue is AsyncGeneratorDrainQueue.
func (vm *VM) agDrainQueue(g *GeneratorObject) {
	for {
		if len(g.asyncQueue) == 0 {
			g.asyncState = agCompleted
			return
		}
		next := g.asyncQueue[0]
		if next.kind == agReturn {
			vm.agAwaitReturn(g)
			return
		}
		v := next.value
		if next.kind == agNormal {
			v = Undefined
		}
		vm.agCompleteStep(g, next.kind, v, true)
	}
}

// agAwaitReturn is AsyncGeneratorAwaitReturn.
func (vm *VM) agAwaitReturn(g *GeneratorObject) {
	next := g.asyncQueue[0]
	vm.agAwaitThen(g, next.value, func(v Value) {
		vm.agCompleteStep(g, agNormal, v, true)
		vm.agDrainQueue(g)
	}, func(r Value) {
		vm.agCompleteStep(g, agThrow, r, true)
		vm.agDrainQueue(g)
	})
}

// agDelegate runs one step of yield* (14.4.14, generatorKind async) for the
// received completion, awaiting what the inner iterator returns.
func (vm *VM) agDelegate(g *GeneratorObject, kind asyncGenKind, value Value) {
	g.asyncState = agExecuting
	iter := g.asyncDelegate
	fail := func(e Value) {
		vm.agEndDelegate(g)
		vm.agRun(g, agThrow, e)
	}
	failErr := func(err error) {
		vm.ClearUnwindingState()
		fail(vm.ExceptionValueFromError(err))
	}
	// awaitResult awaits an inner result and hands it to then.
	awaitResult := func(res Value, then func(Value)) {
		p, err := vm.awaitPromiseResolve(res)
		if err != nil {
			failErr(err)
			return
		}
		vm.thenGo(p, then, fail)
	}

	switch kind {
	case agNormal:
		if !g.asyncDelegateNext.IsCallable() {
			fail(vm.typeErrorValue("The iterator's next method is not a function"))
			return
		}
		res, err := vm.Call(g.asyncDelegateNext, iter, []Value{value})
		if err != nil {
			failErr(err)
			return
		}
		awaitResult(res, func(r Value) { vm.agDelegateResult(g, r, false) })

	case agThrow:
		throwMethod, err := vm.getMethod(iter, "throw")
		if err != nil {
			failErr(err)
			return
		}
		if throwMethod.Type() != TypeUndefined {
			res, err := vm.Call(throwMethod, iter, []Value{value})
			if err != nil {
				failErr(err)
				return
			}
			awaitResult(res, func(r Value) { vm.agDelegateResult(g, r, false) })
			return
		}
		// No throw method: AsyncIteratorClose(iterator, normal completion),
		// then throw a TypeError for the protocol violation.
		protocolErr := func() { fail(vm.typeErrorValue("The iterator does not provide a 'throw' method")) }
		returnMethod, err := vm.getMethod(iter, "return")
		if err != nil {
			failErr(err)
			return
		}
		if returnMethod.Type() == TypeUndefined {
			protocolErr()
			return
		}
		res, err := vm.Call(returnMethod, iter, nil)
		if err != nil {
			failErr(err)
			return
		}
		awaitResult(res, func(r Value) {
			if !isObjectLike(r) {
				fail(vm.typeErrorValue("Iterator result is not an object"))
				return
			}
			protocolErr()
		})

	case agReturn:
		returnMethod, err := vm.getMethod(iter, "return")
		if err != nil {
			failErr(err)
			return
		}
		if returnMethod.Type() == TypeUndefined {
			vm.agEndDelegate(g)
			vm.agAwaitThen(g, value, func(v Value) { vm.agRun(g, agReturn, v) }, func(r Value) { vm.agRun(g, agThrow, r) })
			return
		}
		res, err := vm.Call(returnMethod, iter, []Value{value})
		if err != nil {
			failErr(err)
			return
		}
		awaitResult(res, func(r Value) { vm.agDelegateResult(g, r, true) })
	}
}

// agDelegateResult handles an awaited inner iterator result during yield*.
func (vm *VM) agDelegateResult(g *GeneratorObject, r Value, isReturn bool) {
	fail := func(err error) {
		vm.ClearUnwindingState()
		vm.agEndDelegate(g)
		vm.agRun(g, agThrow, vm.ExceptionValueFromError(err))
	}
	if !isObjectLike(r) {
		vm.agEndDelegate(g)
		vm.agRun(g, agThrow, vm.typeErrorValue("Iterator result "+r.ToString()+" is not an object"))
		return
	}
	doneVal, err := vm.GetProperty(r, "done")
	if err != nil {
		fail(err)
		return
	}
	if doneVal.IsTruthy() {
		v, err := vm.GetProperty(r, "value")
		if err != nil {
			fail(err)
			return
		}
		vm.agEndDelegate(g)
		if isReturn {
			vm.agAwaitThen(g, v, func(av Value) { vm.agRun(g, agReturn, av) }, func(e Value) { vm.agRun(g, agThrow, e) })
			return
		}
		vm.agRun(g, agNormal, v)
		return
	}
	v, err := vm.GetProperty(r, "value")
	if err != nil {
		fail(err)
		return
	}
	vm.agYield(g, v)
}

func (vm *VM) agEndDelegate(g *GeneratorObject) {
	g.asyncDelegating = false
	g.asyncDelegate = Undefined
	g.asyncDelegateNext = Undefined
}

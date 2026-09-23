package builtins

import (
	"math"

	"github.com/nooga/paserati/pkg/vm"
)

// This file implements the Iterator Record abstract operations (ECMA-262 7.4)
// and everything built on them: the Iterator Helper objects returned by
// Iterator.prototype.{map,filter,take,drop,flatMap} and Iterator.{concat,zip,
// zipKeyed}, and the consuming Iterator.prototype methods.

// iteratorRecord is the spec's Iterator Record.
type iteratorRecord struct {
	iter vm.Value
	next vm.Value
	done bool
}

func isObjectValue(v vm.Value) bool { return v.IsObject() || v.IsCallable() }

func createIterResultObject(vmi *vm.VM, value vm.Value, done bool) vm.Value {
	result := vm.NewObject(vmi.ObjectPrototype).AsPlainObject()
	result.SetOwn("value", value)
	result.SetOwn("done", vm.BooleanValue(done))
	return vm.NewValueFromPlainObject(result)
}

// getIteratorDirect reads obj.next once; its callability is checked per step.
func getIteratorDirect(vmi *vm.VM, obj vm.Value) (*iteratorRecord, error) {
	next, err := vmi.GetProperty(obj, "next")
	if err != nil {
		return nil, err
	}
	return &iteratorRecord{iter: obj, next: next}, nil
}

// getSymbolMethod is GetMethod(v, @@sym): undefined for a nullish property,
// TypeError for a non-callable one.
func getSymbolMethod(vmi *vm.VM, v vm.Value, sym vm.Value) (vm.Value, error) {
	method, err := vmi.ReflectGetSymbolProperty(v, sym)
	if err != nil {
		return vm.Undefined, err
	}
	if method.IsUndefined() || method.Type() == vm.TypeNull {
		return vm.Undefined, nil
	}
	if !method.IsCallable() {
		return vm.Undefined, vmi.NewTypeError("Symbol.iterator is not a function")
	}
	return method, nil
}

// getIterator is GetIterator(obj, sync).
func getIterator(vmi *vm.VM, obj vm.Value) (*iteratorRecord, error) {
	method, err := getSymbolMethod(vmi, obj, SymbolIterator)
	if err != nil {
		return nil, err
	}
	if method.IsUndefined() {
		return nil, vmi.NewTypeError("object is not iterable")
	}
	iter, err := vmi.Call(method, obj, nil)
	if err != nil {
		return nil, err
	}
	if !isObjectValue(iter) {
		return nil, vmi.NewTypeError("Result of the Symbol.iterator method is not an object")
	}
	return getIteratorDirect(vmi, iter)
}

// getIteratorFlattenable is GetIteratorFlattenable(obj, reject-primitives).
func getIteratorFlattenable(vmi *vm.VM, obj vm.Value) (*iteratorRecord, error) {
	if !isObjectValue(obj) {
		return nil, vmi.NewTypeError("value is not an object")
	}
	method, err := getSymbolMethod(vmi, obj, SymbolIterator)
	if err != nil {
		return nil, err
	}
	iter := obj
	if !method.IsUndefined() {
		if iter, err = vmi.Call(method, obj, nil); err != nil {
			return nil, err
		}
	}
	if !isObjectValue(iter) {
		return nil, vmi.NewTypeError("iterator is not an object")
	}
	return getIteratorDirect(vmi, iter)
}

// iteratorStep is IteratorStep: it returns the result object, or done=true.
// Any abrupt completion marks the record done.
func iteratorStep(vmi *vm.VM, rec *iteratorRecord) (vm.Value, bool, error) {
	if !rec.next.IsCallable() {
		rec.done = true
		return vm.Undefined, false, vmi.NewTypeError("iterator.next is not a function")
	}
	result, err := vmi.Call(rec.next, rec.iter, nil)
	if err != nil {
		rec.done = true
		return vm.Undefined, false, err
	}
	if !isObjectValue(result) {
		rec.done = true
		return vm.Undefined, false, vmi.NewTypeError("Iterator result " + result.ToString() + " is not an object")
	}
	done, err := vmi.GetProperty(result, "done")
	if err != nil {
		rec.done = true
		return vm.Undefined, false, err
	}
	if done.IsTruthy() {
		rec.done = true
		return vm.Undefined, true, nil
	}
	return result, false, nil
}

// iteratorStepValue is IteratorStepValue: the next value, or done=true.
func iteratorStepValue(vmi *vm.VM, rec *iteratorRecord) (vm.Value, bool, error) {
	result, done, err := iteratorStep(vmi, rec)
	if err != nil || done {
		return vm.Undefined, done, err
	}
	value, err := vmi.GetProperty(result, "value")
	if err != nil {
		rec.done = true
		return vm.Undefined, false, err
	}
	return value, false, nil
}

// iteratorClose is IteratorClose(iter, completion). completion is the pending
// throw, or nil for a normal or return completion - in which case errors from
// getting or calling "return", and a non-object result, are what's returned.
func iteratorClose(vmi *vm.VM, iter vm.Value, completion error) error {
	ret, err := vmi.GetProperty(iter, "return")
	if err == nil && !ret.IsUndefined() && ret.Type() != vm.TypeNull && !ret.IsCallable() {
		err = vmi.NewTypeError("iterator.return is not a function")
	}
	if err != nil {
		if completion != nil {
			return completion
		}
		return err
	}
	if ret.IsUndefined() || ret.Type() == vm.TypeNull {
		return completion
	}
	result, err := vmi.Call(ret, iter, nil)
	if completion != nil {
		return completion
	}
	if err != nil {
		return err
	}
	if !isObjectValue(result) {
		return vmi.NewTypeError("iterator.return() result is not an object")
	}
	return nil
}

// iteratorCloseAll is IteratorCloseAll: closes recs in reverse order, each
// seeing the completion left by the previous close.
func iteratorCloseAll(vmi *vm.VM, recs []*iteratorRecord, completion error) error {
	for i := len(recs) - 1; i >= 0; i-- {
		completion = iteratorClose(vmi, recs[i].iter, completion)
	}
	return completion
}

type helperState uint8

const (
	helperSuspendedStart helperState = iota
	helperSuspendedYield
	helperExecuting
	helperCompleted
)

// iteratorHelper is an Iterator Helper object's internal state: the
// [[GeneratorState]] of the generator CreateIteratorFromClosure would make,
// and that generator's closure written as a resumable Go state machine.
type iteratorHelper struct {
	state helperState
	// underlying is [[UnderlyingIterators]], closed by return() from
	// suspended-start.
	underlying []*iteratorRecord
	// resume runs the closure to its next Yield (value, false, nil), to its
	// end (_, true, nil), or until it throws.
	resume func() (vm.Value, bool, error)
	// abort resumes the closure at its current Yield with a return
	// completion, closing the iterators it holds open.
	abort func() error
}

func newIteratorHelper(vmi *vm.VM, h *iteratorHelper) vm.Value {
	obj := vm.NewObject(vmi.IteratorHelperPrototype).AsPlainObject()
	obj.SetInternalSlots(h)
	return vm.NewValueFromPlainObject(obj)
}

func thisIteratorHelper(v vm.Value) *iteratorHelper {
	if v.Type() != vm.TypeObject {
		return nil
	}
	h, _ := v.AsPlainObject().InternalSlots().(*iteratorHelper)
	return h
}

// installIteratorHelperPrototype defines next and return on
// %IteratorHelperPrototype%.
func installIteratorHelperPrototype(vmi *vm.VM, proto *vm.PlainObject) {
	proto.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		// GeneratorResume(this, undefined, "Iterator Helper")
		h := thisIteratorHelper(vmi.GetThis())
		if h == nil {
			return vm.Undefined, vmi.NewTypeError("Iterator Helper next called on incompatible receiver")
		}
		switch h.state {
		case helperExecuting:
			return vm.Undefined, vmi.NewTypeError("Generator is already running")
		case helperCompleted:
			return createIterResultObject(vmi, vm.Undefined, true), nil
		}
		h.state = helperExecuting
		value, done, err := h.resume()
		if err != nil || done {
			h.state = helperCompleted
			if err != nil {
				return vm.Undefined, err
			}
			return createIterResultObject(vmi, vm.Undefined, true), nil
		}
		h.state = helperSuspendedYield
		return createIterResultObject(vmi, value, false), nil
	}))

	proto.SetOwnNonEnumerable("return", vm.NewNativeFunction(0, false, "return", func(args []vm.Value) (vm.Value, error) {
		h := thisIteratorHelper(vmi.GetThis())
		if h == nil {
			return vm.Undefined, vmi.NewTypeError("Iterator Helper return called on incompatible receiver")
		}
		switch h.state {
		case helperSuspendedStart:
			h.state = helperCompleted
			if err := iteratorCloseAll(vmi, h.underlying, nil); err != nil {
				return vm.Undefined, err
			}
			return createIterResultObject(vmi, vm.Undefined, true), nil
		case helperExecuting:
			return vm.Undefined, vmi.NewTypeError("Generator is already running")
		case helperCompleted:
			return createIterResultObject(vmi, vm.Undefined, true), nil
		}
		// GeneratorResumeAbrupt with ReturnCompletion(undefined)
		h.state = helperExecuting
		err := h.abort()
		h.state = helperCompleted
		if err != nil {
			return vm.Undefined, err
		}
		return createIterResultObject(vmi, vm.Undefined, true), nil
	}))
}

// requireIteratorThis returns this for an Iterator.prototype method, or a
// TypeError if it isn't an object.
func requireIteratorThis(vmi *vm.VM, method string) (vm.Value, error) {
	this := vmi.GetThis()
	if !isObjectValue(this) {
		return vm.Undefined, vmi.NewTypeError("Iterator.prototype." + method + " called on non-object")
	}
	return this, nil
}

// iteratorCallbackPrologue is the shared start of the callback-taking
// Iterator.prototype methods: the receiver check, then (closing the receiver
// on failure) the callability check, then GetIteratorDirect.
func iteratorCallbackPrologue(vmi *vm.VM, method string, args []vm.Value) (*iteratorRecord, vm.Value, error) {
	this, err := requireIteratorThis(vmi, method)
	if err != nil {
		return nil, vm.Undefined, err
	}
	fn := vm.Undefined
	if len(args) > 0 {
		fn = args[0]
	}
	if !fn.IsCallable() {
		return nil, vm.Undefined, iteratorClose(vmi, this, vmi.NewTypeError("Iterator.prototype."+method+": argument is not a function"))
	}
	rec, err := getIteratorDirect(vmi, this)
	if err != nil {
		return nil, vm.Undefined, err
	}
	return rec, fn, nil
}

// iteratorLimitPrologue is the shared start of take and drop: the receiver
// check, then ToNumber(limit) and its range checks (closing the receiver on
// failure), then GetIteratorDirect. The limit may be +Infinity.
func iteratorLimitPrologue(vmi *vm.VM, method string, args []vm.Value) (*iteratorRecord, float64, error) {
	this, err := requireIteratorThis(vmi, method)
	if err != nil {
		return nil, 0, err
	}
	limit := vm.Undefined
	if len(args) > 0 {
		limit = args[0]
	}
	num, err := toNumberWithVM(vmi, limit)
	if err == ErrVMUnwinding {
		// ToPrimitive threw and the VM is already unwinding it.
		_ = iteratorClose(vmi, this, err)
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, iteratorClose(vmi, this, err)
	}
	if math.IsNaN(num) {
		return nil, 0, iteratorClose(vmi, this, vmi.NewRangeError("Iterator.prototype."+method+": limit must be a number"))
	}
	num = math.Trunc(num) // ToIntegerOrInfinity
	if num < 0 {
		return nil, 0, iteratorClose(vmi, this, vmi.NewRangeError("Iterator.prototype."+method+": limit must be non-negative"))
	}
	rec, err := getIteratorDirect(vmi, this)
	if err != nil {
		return nil, 0, err
	}
	return rec, num, nil
}

// installIteratorPrototypeMethods defines the helper-producing and consuming
// methods of %Iterator.prototype%.
func installIteratorPrototypeMethods(vmi *vm.VM, proto *vm.PlainObject) {
	proto.SetOwnNonEnumerable("map", vm.NewNativeFunction(1, false, "map", func(args []vm.Value) (vm.Value, error) {
		iterated, mapper, err := iteratorCallbackPrologue(vmi, "map", args)
		if iterated == nil {
			return vm.Undefined, err
		}
		counter := 0
		return newIteratorHelper(vmi, &iteratorHelper{
			underlying: []*iteratorRecord{iterated},
			resume: func() (vm.Value, bool, error) {
				value, done, err := iteratorStepValue(vmi, iterated)
				if err != nil || done {
					return vm.Undefined, done, err
				}
				mapped, err := vmi.CallArgs2(mapper, vm.Undefined, value, vm.NumberValue(float64(counter)))
				if err != nil {
					return vm.Undefined, false, iteratorClose(vmi, iterated.iter, err)
				}
				counter++
				return mapped, false, nil
			},
			abort: func() error { return iteratorClose(vmi, iterated.iter, nil) },
		}), nil
	}))

	proto.SetOwnNonEnumerable("filter", vm.NewNativeFunction(1, false, "filter", func(args []vm.Value) (vm.Value, error) {
		iterated, predicate, err := iteratorCallbackPrologue(vmi, "filter", args)
		if iterated == nil {
			return vm.Undefined, err
		}
		counter := 0
		return newIteratorHelper(vmi, &iteratorHelper{
			underlying: []*iteratorRecord{iterated},
			resume: func() (vm.Value, bool, error) {
				for {
					value, done, err := iteratorStepValue(vmi, iterated)
					if err != nil || done {
						return vm.Undefined, done, err
					}
					selected, err := vmi.CallArgs2(predicate, vm.Undefined, value, vm.NumberValue(float64(counter)))
					if err != nil {
						return vm.Undefined, false, iteratorClose(vmi, iterated.iter, err)
					}
					counter++
					if selected.IsTruthy() {
						return value, false, nil
					}
				}
			},
			abort: func() error { return iteratorClose(vmi, iterated.iter, nil) },
		}), nil
	}))

	proto.SetOwnNonEnumerable("take", vm.NewNativeFunction(1, false, "take", func(args []vm.Value) (vm.Value, error) {
		iterated, remaining, err := iteratorLimitPrologue(vmi, "take", args)
		if iterated == nil {
			return vm.Undefined, err
		}
		return newIteratorHelper(vmi, &iteratorHelper{
			underlying: []*iteratorRecord{iterated},
			resume: func() (vm.Value, bool, error) {
				if remaining == 0 {
					return vm.Undefined, true, iteratorClose(vmi, iterated.iter, nil)
				}
				if !math.IsInf(remaining, 1) {
					remaining--
				}
				return iteratorStepValue(vmi, iterated)
			},
			abort: func() error { return iteratorClose(vmi, iterated.iter, nil) },
		}), nil
	}))

	proto.SetOwnNonEnumerable("drop", vm.NewNativeFunction(1, false, "drop", func(args []vm.Value) (vm.Value, error) {
		iterated, remaining, err := iteratorLimitPrologue(vmi, "drop", args)
		if iterated == nil {
			return vm.Undefined, err
		}
		return newIteratorHelper(vmi, &iteratorHelper{
			underlying: []*iteratorRecord{iterated},
			resume: func() (vm.Value, bool, error) {
				for remaining > 0 {
					if !math.IsInf(remaining, 1) {
						remaining--
					}
					if _, done, err := iteratorStep(vmi, iterated); err != nil || done {
						return vm.Undefined, done, err
					}
				}
				return iteratorStepValue(vmi, iterated)
			},
			abort: func() error { return iteratorClose(vmi, iterated.iter, nil) },
		}), nil
	}))

	proto.SetOwnNonEnumerable("flatMap", vm.NewNativeFunction(1, false, "flatMap", func(args []vm.Value) (vm.Value, error) {
		iterated, mapper, err := iteratorCallbackPrologue(vmi, "flatMap", args)
		if iterated == nil {
			return vm.Undefined, err
		}
		counter := 0
		var inner *iteratorRecord
		return newIteratorHelper(vmi, &iteratorHelper{
			underlying: []*iteratorRecord{iterated},
			resume: func() (vm.Value, bool, error) {
				for {
					if inner != nil {
						value, done, err := iteratorStepValue(vmi, inner)
						if err != nil {
							return vm.Undefined, false, iteratorClose(vmi, iterated.iter, err)
						}
						if !done {
							return value, false, nil
						}
						inner = nil
					}
					value, done, err := iteratorStepValue(vmi, iterated)
					if err != nil || done {
						return vm.Undefined, done, err
					}
					mapped, err := vmi.CallArgs2(mapper, vm.Undefined, value, vm.NumberValue(float64(counter)))
					if err != nil {
						return vm.Undefined, false, iteratorClose(vmi, iterated.iter, err)
					}
					if inner, err = getIteratorFlattenable(vmi, mapped); err != nil {
						return vm.Undefined, false, iteratorClose(vmi, iterated.iter, err)
					}
					counter++
				}
			},
			abort: func() error {
				if inner != nil {
					if err := iteratorClose(vmi, inner.iter, nil); err != nil {
						return iteratorClose(vmi, iterated.iter, err)
					}
				}
				return iteratorClose(vmi, iterated.iter, nil)
			},
		}), nil
	}))

	proto.SetOwnNonEnumerable("toArray", vm.NewNativeFunction(0, false, "toArray", func(args []vm.Value) (vm.Value, error) {
		this, err := requireIteratorThis(vmi, "toArray")
		if err != nil {
			return vm.Undefined, err
		}
		iterated, err := getIteratorDirect(vmi, this)
		if err != nil {
			return vm.Undefined, err
		}
		result := vm.NewArray()
		arr := result.AsArray()
		for {
			value, done, err := iteratorStepValue(vmi, iterated)
			if err != nil {
				return vm.Undefined, err
			}
			if done {
				return result, nil
			}
			arr.Append(value)
		}
	}))

	proto.SetOwnNonEnumerable("forEach", vm.NewNativeFunction(1, false, "forEach", func(args []vm.Value) (vm.Value, error) {
		iterated, fn, err := iteratorCallbackPrologue(vmi, "forEach", args)
		if iterated == nil {
			return vm.Undefined, err
		}
		for counter := 0; ; counter++ {
			value, done, err := iteratorStepValue(vmi, iterated)
			if err != nil || done {
				return vm.Undefined, err
			}
			if _, err := vmi.CallArgs2(fn, vm.Undefined, value, vm.NumberValue(float64(counter))); err != nil {
				return vm.Undefined, iteratorClose(vmi, iterated.iter, err)
			}
		}
	}))

	proto.SetOwnNonEnumerable("reduce", vm.NewNativeFunction(1, false, "reduce", func(args []vm.Value) (vm.Value, error) {
		iterated, reducer, err := iteratorCallbackPrologue(vmi, "reduce", args)
		if iterated == nil {
			return vm.Undefined, err
		}
		var accumulator vm.Value
		counter := 0
		if len(args) >= 2 {
			accumulator = args[1]
		} else {
			value, done, err := iteratorStepValue(vmi, iterated)
			if err != nil {
				return vm.Undefined, err
			}
			if done {
				return vm.Undefined, vmi.NewTypeError("Reduce of empty iterator with no initial value")
			}
			accumulator = value
			counter = 1
		}
		for ; ; counter++ {
			value, done, err := iteratorStepValue(vmi, iterated)
			if err != nil {
				return vm.Undefined, err
			}
			if done {
				return accumulator, nil
			}
			accumulator, err = vmi.CallArgs3(reducer, vm.Undefined, accumulator, value, vm.NumberValue(float64(counter)))
			if err != nil {
				return vm.Undefined, iteratorClose(vmi, iterated.iter, err)
			}
		}
	}))

	// some, every and find run the predicate until stop(value, result) says
	// to answer early, closing the iterator with that answer.
	searchMethod := func(name string, stop func(value, selected vm.Value) (vm.Value, bool), exhausted vm.Value) {
		proto.SetOwnNonEnumerable(name, vm.NewNativeFunction(1, false, name, func(args []vm.Value) (vm.Value, error) {
			iterated, predicate, err := iteratorCallbackPrologue(vmi, name, args)
			if iterated == nil {
				return vm.Undefined, err
			}
			for counter := 0; ; counter++ {
				value, done, err := iteratorStepValue(vmi, iterated)
				if err != nil {
					return vm.Undefined, err
				}
				if done {
					return exhausted, nil
				}
				selected, err := vmi.CallArgs2(predicate, vm.Undefined, value, vm.NumberValue(float64(counter)))
				if err != nil {
					return vm.Undefined, iteratorClose(vmi, iterated.iter, err)
				}
				if answer, ok := stop(value, selected); ok {
					if err := iteratorClose(vmi, iterated.iter, nil); err != nil {
						return vm.Undefined, err
					}
					return answer, nil
				}
			}
		}))
	}
	searchMethod("some", func(_, selected vm.Value) (vm.Value, bool) {
		return vm.BooleanValue(true), selected.IsTruthy()
	}, vm.BooleanValue(false))
	searchMethod("every", func(_, selected vm.Value) (vm.Value, bool) {
		return vm.BooleanValue(false), !selected.IsTruthy()
	}, vm.BooleanValue(true))
	searchMethod("find", func(value, selected vm.Value) (vm.Value, bool) {
		return value, selected.IsTruthy()
	}, vm.Undefined)
}

// iteratorZip is IteratorZip: a helper yielding finish(results) for each
// round of stepping iters, ending per mode ("shortest", "longest", "strict").
func iteratorZip(vmi *vm.VM, iters []*iteratorRecord, mode string, padding []vm.Value, finish func([]vm.Value) vm.Value) vm.Value {
	iterCount := len(iters)
	openIters := append([]*iteratorRecord(nil), iters...)
	slots := append([]*iteratorRecord(nil), iters...) // nil once exhausted in "longest" mode
	removeOpen := func(rec *iteratorRecord) {
		for i, r := range openIters {
			if r == rec {
				openIters = append(openIters[:i], openIters[i+1:]...)
				return
			}
		}
	}
	lengthMismatch := func() error {
		return vmi.NewTypeError("Iterator.zip: iterators have different lengths in strict mode")
	}
	return newIteratorHelper(vmi, &iteratorHelper{
		underlying: iters,
		resume: func() (vm.Value, bool, error) {
			if iterCount == 0 {
				return vm.Undefined, true, nil
			}
			results := make([]vm.Value, iterCount)
			for i, rec := range slots {
				if rec == nil {
					results[i] = padding[i]
					continue
				}
				value, done, err := iteratorStepValue(vmi, rec)
				if err != nil {
					removeOpen(rec)
					return vm.Undefined, false, iteratorCloseAll(vmi, openIters, err)
				}
				if done {
					removeOpen(rec)
					switch mode {
					case "shortest":
						return vm.Undefined, true, iteratorCloseAll(vmi, openIters, nil)
					case "strict":
						if i != 0 {
							return vm.Undefined, false, iteratorCloseAll(vmi, openIters, lengthMismatch())
						}
						for _, other := range slots[1:] {
							_, done, err := iteratorStep(vmi, other)
							if err != nil {
								removeOpen(other)
								return vm.Undefined, false, iteratorCloseAll(vmi, openIters, err)
							}
							if !done {
								return vm.Undefined, false, iteratorCloseAll(vmi, openIters, lengthMismatch())
							}
							removeOpen(other)
						}
						return vm.Undefined, true, nil
					default: // "longest"
						if len(openIters) == 0 {
							return vm.Undefined, true, nil
						}
						slots[i] = nil
						value = padding[i]
					}
				}
				results[i] = value
			}
			return finish(results), false, nil
		},
		abort: func() error { return iteratorCloseAll(vmi, openIters, nil) },
	})
}

// zipOptions reads the mode and padding options of Iterator.zip/zipKeyed
// (GetOptionsObject, then steps 3-7).
func zipOptions(vmi *vm.VM, name string, args []vm.Value) (string, vm.Value, error) {
	options := vm.Undefined
	if len(args) > 1 {
		options = args[1]
	}
	if options.IsUndefined() {
		return "shortest", vm.Undefined, nil
	}
	if !isObjectValue(options) {
		return "", vm.Undefined, vmi.NewTypeError(name + ": options must be an object")
	}
	modeVal, err := vmi.GetProperty(options, "mode")
	if err != nil {
		return "", vm.Undefined, err
	}
	mode := "shortest"
	if !modeVal.IsUndefined() {
		if modeVal.IsString() {
			mode = modeVal.ToString()
		}
		if !modeVal.IsString() || (mode != "shortest" && mode != "longest" && mode != "strict") {
			return "", vm.Undefined, vmi.NewTypeError(name + `: mode must be "shortest", "longest", or "strict"`)
		}
	}
	padding := vm.Undefined
	if mode == "longest" {
		if padding, err = vmi.GetProperty(options, "padding"); err != nil {
			return "", vm.Undefined, err
		}
		if !padding.IsUndefined() && !isObjectValue(padding) {
			return "", vm.Undefined, vmi.NewTypeError(name + ": padding must be an object")
		}
	}
	return mode, padding, nil
}

// getByKey is Get(obj, key) for a string or symbol key.
func getByKey(vmi *vm.VM, obj vm.Value, key vm.Value) (vm.Value, error) {
	if key.Type() == vm.TypeSymbol {
		return vmi.ReflectGetSymbolProperty(obj, key)
	}
	return vmi.GetProperty(obj, key.ToString())
}

// installIteratorStatics defines Iterator.concat, Iterator.zip and
// Iterator.zipKeyed.
func installIteratorStatics(vmi *vm.VM, ctor *vm.PlainObject) {
	ctor.SetOwnNonEnumerable("concat", vm.NewNativeFunction(0, true, "concat", func(args []vm.Value) (vm.Value, error) {
		type iterable struct{ method, value vm.Value }
		iterables := make([]iterable, 0, len(args))
		for _, item := range args {
			if !isObjectValue(item) {
				return vm.Undefined, vmi.NewTypeError("Iterator.concat: argument is not an object")
			}
			method, err := getSymbolMethod(vmi, item, SymbolIterator)
			if err != nil {
				return vm.Undefined, err
			}
			if method.IsUndefined() {
				return vm.Undefined, vmi.NewTypeError("Iterator.concat: argument is not iterable")
			}
			iterables = append(iterables, iterable{method, item})
		}
		next := 0
		var current *iteratorRecord
		return newIteratorHelper(vmi, &iteratorHelper{
			resume: func() (vm.Value, bool, error) {
				for {
					if current != nil {
						value, done, err := iteratorStepValue(vmi, current)
						if err != nil || !done {
							return value, false, err
						}
						current = nil
					}
					if next == len(iterables) {
						return vm.Undefined, true, nil
					}
					it := iterables[next]
					next++
					iter, err := vmi.Call(it.method, it.value, nil)
					if err != nil {
						return vm.Undefined, false, err
					}
					if !isObjectValue(iter) {
						return vm.Undefined, false, vmi.NewTypeError("Iterator.concat: Symbol.iterator result is not an object")
					}
					if current, err = getIteratorDirect(vmi, iter); err != nil {
						return vm.Undefined, false, err
					}
				}
			},
			abort: func() error {
				if current == nil {
					return nil
				}
				return iteratorClose(vmi, current.iter, nil)
			},
		}), nil
	}))

	ctor.SetOwnNonEnumerable("zip", vm.NewNativeFunction(1, false, "zip", func(args []vm.Value) (vm.Value, error) {
		iterables := vm.Undefined
		if len(args) > 0 {
			iterables = args[0]
		}
		if !isObjectValue(iterables) {
			return vm.Undefined, vmi.NewTypeError("Iterator.zip: iterables is not an object")
		}
		mode, paddingOption, err := zipOptions(vmi, "Iterator.zip", args)
		if err != nil {
			return vm.Undefined, err
		}
		var iters []*iteratorRecord
		inputIter, err := getIterator(vmi, iterables)
		if err != nil {
			return vm.Undefined, err
		}
		for {
			next, done, err := iteratorStepValue(vmi, inputIter)
			if err != nil {
				return vm.Undefined, iteratorCloseAll(vmi, iters, err)
			}
			if done {
				break
			}
			iter, err := getIteratorFlattenable(vmi, next)
			if err != nil {
				return vm.Undefined, iteratorCloseAll(vmi, append([]*iteratorRecord{inputIter}, iters...), err)
			}
			iters = append(iters, iter)
		}
		padding := make([]vm.Value, len(iters))
		for i := range padding {
			padding[i] = vm.Undefined
		}
		if mode == "longest" && !paddingOption.IsUndefined() {
			paddingIter, err := getIterator(vmi, paddingOption)
			if err != nil {
				return vm.Undefined, iteratorCloseAll(vmi, iters, err)
			}
			usingIterator := true
			for i := range padding {
				value, done, err := iteratorStepValue(vmi, paddingIter)
				if err != nil {
					return vm.Undefined, iteratorCloseAll(vmi, iters, err)
				}
				if done {
					usingIterator = false
					break
				}
				padding[i] = value
			}
			if usingIterator {
				if err := iteratorClose(vmi, paddingIter.iter, nil); err != nil {
					return vm.Undefined, iteratorCloseAll(vmi, iters, err)
				}
			}
		}
		return iteratorZip(vmi, iters, mode, padding, func(results []vm.Value) vm.Value {
			arr := vm.NewArray()
			for _, v := range results {
				arr.AsArray().Append(v)
			}
			return arr
		}), nil
	}))

	ctor.SetOwnNonEnumerable("zipKeyed", vm.NewNativeFunction(1, false, "zipKeyed", func(args []vm.Value) (vm.Value, error) {
		iterables := vm.Undefined
		if len(args) > 0 {
			iterables = args[0]
		}
		if !isObjectValue(iterables) {
			return vm.Undefined, vmi.NewTypeError("Iterator.zipKeyed: iterables is not an object")
		}
		mode, paddingOption, err := zipOptions(vmi, "Iterator.zipKeyed", args)
		if err != nil {
			return vm.Undefined, err
		}
		allKeys, err := reflectOwnKeysWithVM(vmi, iterables)
		if err != nil {
			return vm.Undefined, err
		}
		var keys []vm.Value
		var iters []*iteratorRecord
		keyList := allKeys.AsArray()
		for k := 0; k < keyList.Length(); k++ {
			key := keyList.Get(k)
			desc, err := objectGetOwnPropertyDescriptorWithVM(vmi, []vm.Value{iterables, key})
			if err != nil {
				return vm.Undefined, iteratorCloseAll(vmi, iters, err)
			}
			if desc.IsUndefined() {
				continue
			}
			enumerable, err := vmi.GetProperty(desc, "enumerable")
			if err != nil {
				return vm.Undefined, iteratorCloseAll(vmi, iters, err)
			}
			if !enumerable.IsTruthy() {
				continue
			}
			value, err := getByKey(vmi, iterables, key)
			if err != nil {
				return vm.Undefined, iteratorCloseAll(vmi, iters, err)
			}
			if value.IsUndefined() {
				continue
			}
			iter, err := getIteratorFlattenable(vmi, value)
			if err != nil {
				return vm.Undefined, iteratorCloseAll(vmi, iters, err)
			}
			keys = append(keys, key)
			iters = append(iters, iter)
		}
		padding := make([]vm.Value, len(iters))
		for i := range padding {
			padding[i] = vm.Undefined
		}
		if mode == "longest" && !paddingOption.IsUndefined() {
			for i, key := range keys {
				if padding[i], err = getByKey(vmi, paddingOption, key); err != nil {
					return vm.Undefined, iteratorCloseAll(vmi, iters, err)
				}
			}
		}
		t := true
		return iteratorZip(vmi, iters, mode, padding, func(results []vm.Value) vm.Value {
			obj := vm.NewObject(vm.Null).AsPlainObject()
			for i, key := range keys {
				if key.Type() == vm.TypeSymbol {
					obj.DefineOwnPropertyByKey(vm.NewSymbolKey(key), results[i], &t, &t, &t)
				} else {
					obj.DefineOwnProperty(key.ToString(), results[i], &t, &t, &t)
				}
			}
			return vm.NewValueFromPlainObject(obj)
		}), nil
	}))
}

// installWrapForValidIteratorPrototype defines next and return on
// %WrapForValidIteratorPrototype%, whose instances (from Iterator.from) hold
// their [[Iterated]] Iterator Record as internal slots.
func installWrapForValidIteratorPrototype(vmi *vm.VM, proto *vm.PlainObject) {
	thisIterated := func(method string) (*iteratorRecord, error) {
		this := vmi.GetThis()
		if this.Type() == vm.TypeObject {
			if rec, ok := this.AsPlainObject().InternalSlots().(*iteratorRecord); ok {
				return rec, nil
			}
		}
		return nil, vmi.NewTypeError("%WrapForValidIteratorPrototype%." + method + " called on incompatible receiver")
	}
	proto.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		rec, err := thisIterated("next")
		if err != nil {
			return vm.Undefined, err
		}
		if !rec.next.IsCallable() {
			return vm.Undefined, vmi.NewTypeError("iterator.next is not a function")
		}
		return vmi.Call(rec.next, rec.iter, nil)
	}))
	proto.SetOwnNonEnumerable("return", vm.NewNativeFunction(0, false, "return", func(args []vm.Value) (vm.Value, error) {
		rec, err := thisIterated("return")
		if err != nil {
			return vm.Undefined, err
		}
		ret, err := vmi.GetProperty(rec.iter, "return")
		if err != nil {
			return vm.Undefined, err
		}
		if ret.IsUndefined() || ret.Type() == vm.TypeNull {
			return createIterResultObject(vmi, vm.Undefined, true), nil
		}
		if !ret.IsCallable() {
			return vm.Undefined, vmi.NewTypeError("iterator.return is not a function")
		}
		return vmi.Call(ret, rec.iter, nil)
	}))
}

// iteratorFrom is Iterator.from(O); iteratorCtor is %Iterator%.
func iteratorFrom(vmi *vm.VM, iteratorCtor vm.Value, args []vm.Value) (vm.Value, error) {
	obj := vm.Undefined
	if len(args) > 0 {
		obj = args[0]
	}
	// GetIteratorFlattenable(O, iterate-string-primitives)
	if !isObjectValue(obj) && !obj.IsString() {
		return vm.Undefined, vmi.NewTypeError("Iterator.from: argument is not an object or string")
	}
	method, err := getSymbolMethod(vmi, obj, SymbolIterator)
	if err != nil {
		return vm.Undefined, err
	}
	iter := obj
	if !method.IsUndefined() {
		if iter, err = vmi.Call(method, obj, nil); err != nil {
			return vm.Undefined, err
		}
	}
	if !isObjectValue(iter) {
		return vm.Undefined, vmi.NewTypeError("Iterator.from: iterator is not an object")
	}
	rec, err := getIteratorDirect(vmi, iter)
	if err != nil {
		return vm.Undefined, err
	}
	hasInstance, err := functionHasInstanceImpl(vmi, iteratorCtor, iter)
	if err != nil {
		return vm.Undefined, err
	}
	if hasInstance.IsTruthy() {
		return iter, nil
	}
	wrapper := vm.NewObject(vmi.WrapForValidIteratorPrototype).AsPlainObject()
	wrapper.SetInternalSlots(rec)
	return vm.NewValueFromPlainObject(wrapper), nil
}

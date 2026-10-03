package vm

import (
	"math"
	"strconv"
)

// --- V8 Stack Trace API (#492) ---
//
// Real code (typescript-eslint, several error-reporting/telemetry libraries,
// source-map remapping) sets Error.prepareStackTrace to a custom formatter
// and calls Error.captureStackTrace expecting it to receive an array of
// CallSite objects, not a plain string - see https://v8.dev/docs/stack-trace-api.
// This file wires Error.stackTraceLimit and Error.prepareStackTrace into the
// stack capture path shared by `new Error(...)`, the NativeError family, and
// Error.captureStackTrace.

// errorCtorProp reads an own property off the global Error constructor
// object, where Error.stackTraceLimit / Error.prepareStackTrace live.
// Returns (Undefined, false) if Error hasn't been initialized yet.
func (vm *VM) errorCtorProp(name string) (Value, bool) {
	if vm.ErrorConstructor.Type() != TypeNativeFunctionWithProps {
		return Undefined, false
	}
	nfp := vm.ErrorConstructor.AsNativeFunctionWithProps()
	if nfp == nil || nfp.Properties == nil {
		return Undefined, false
	}
	return nfp.Properties.GetOwn(name)
}

// stackTraceLimit reads Error.stackTraceLimit, mirroring V8: it defaults to
// 10 frames, a non-positive or NaN value captures nothing, and +Infinity (or
// any value ToNumber can't turn into a finite non-negative number by way of
// those two cases) means unlimited. Returns -1 for "unlimited".
func (vm *VM) stackTraceLimit() int {
	v, ok := vm.errorCtorProp("stackTraceLimit")
	if !ok || v.Type() == TypeUndefined {
		return 10
	}
	f := v.ToFloat()
	if math.IsNaN(f) {
		return 0
	}
	if math.IsInf(f, 1) {
		return -1
	}
	if f < 0 {
		return 0
	}
	return int(f)
}

// CaptureStackValue is CaptureStackValueExcluding with no excluded target.
func (vm *VM) CaptureStackValue(errorObject Value) (Value, error) {
	return vm.CaptureStackValueExcluding(errorObject, nil)
}

// CaptureStackValueExcluding computes the value to assign to an Error
// object's "stack" property, mirroring V8's Stack Trace API:
//
//   - The number of frames considered is capped by Error.stackTraceLimit
//     (default 10).
//   - If Error.prepareStackTrace has been set to a callable, it is invoked
//     as prepareStackTrace(errorObject, structuredStackTrace) - where
//     structuredStackTrace is an array of CallSite objects - and whatever it
//     returns becomes the stack value verbatim; V8 doesn't require the
//     result to be a string, and callers routinely return the raw array.
//   - Otherwise, the frames are formatted into the usual multi-line string.
//
// `target`, if non-nil, excludes its own frame and everything more recent
// than it - see getStackFramesExcluding, which this delegates to.
func (vm *VM) CaptureStackValueExcluding(errorObject Value, target *FunctionObject) (Value, error) {
	frames := vm.getStackFramesExcludingLimited(target, vm.stackTraceLimit())

	if prepare, ok := vm.errorCtorProp("prepareStackTrace"); ok && prepare.IsCallable() {
		return vm.Call(prepare, Undefined, []Value{errorObject, vm.buildCallSites(frames)})
	}

	return NewString(vm.formatStackFrames(frames)), nil
}

// buildCallSites builds a real JS Array of CallSite-like objects from raw
// stack frames, for consumption by a custom Error.prepareStackTrace hook.
func (vm *VM) buildCallSites(frames []StackFrame) Value {
	arr := NewArray()
	arrObj := arr.AsArray()
	for _, frame := range frames {
		arrObj.Append(vm.newCallSite(frame))
	}
	return arr
}

// newCallSite builds one V8-shaped CallSite object. Paserati doesn't track
// enough per-frame detail to make most of the boolean predicates (isNative,
// isConstructor, isEval, ...) meaningful, so they report conservative
// constant answers rather than being left off entirely - real callers (e.g.
// typescript-eslint's own stack walker) call these unconditionally and
// expect a function back, not `undefined`.
func (vm *VM) newCallSite(frame StackFrame) Value {
	obj := NewObject(vm.callSitePrototype()).AsPlainObject()

	nameValue := Null
	if frame.FunctionName != "" && frame.FunctionName != "<anonymous>" {
		nameValue = NewString(frame.FunctionName)
	}
	fileValue := Null
	if frame.FileName != "" && frame.FileName != "<script>" {
		fileValue = NewString(frame.FileName)
	}
	// Per-frame data lives in a hidden slot (not a property: V8's CallSite
	// instances have no own properties) read back by the shared prototype's
	// methods through `this`.
	data := NewArray()
	d := data.AsArray()
	d.Append(nameValue)
	d.Append(fileValue)
	d.Append(NumberValue(float64(frame.Line)))
	d.Append(NumberValue(float64(frame.Column)))
	d.Append(NewString(frame.FunctionName))
	d.Append(NewString(frame.FileName))
	obj.SetPrivateField(callSiteSlot, data)

	return NewValueFromPlainObject(obj)
}

// callSiteSlot is not a valid identifier, so it can't collide with a
// user-visible #private name.
const callSiteSlot = "[[CallSite]]"

// callSiteData returns the hidden per-frame slot of a CallSite receiver.
func callSiteData(vm *VM, this Value) (*ArrayObject, error) {
	if this.Type() == TypeObject {
		if v, ok := this.AsPlainObject().GetPrivateField(callSiteSlot); ok && v.Type() == TypeArray {
			return v.AsArray(), nil
		}
	}
	return nil, vm.NewTypeError("CallSite method called on incompatible receiver")
}

// callSitePrototype lazily builds the shared CallSite.prototype (and its
// CallSite constructor, for `constructor.name`). V8 keeps the methods there,
// and source-map-support / vite-node clone frames by walking
// Object.getOwnPropertyNames(Object.getPrototypeOf(frame)) (#577).
//
// Paserati doesn't track enough per-frame detail to make most of the boolean
// predicates (isNative, isConstructor, isEval, ...) meaningful, so they report
// conservative constant answers rather than being left off - real callers
// (e.g. typescript-eslint's stack walker) call them unconditionally.
func (vm *VM) callSitePrototype() Value {
	if vm.callSiteProto.Type() == TypeObject {
		return vm.callSiteProto
	}
	proto := NewObject(vm.ObjectPrototype).AsPlainObject()
	protoVal := NewValueFromPlainObject(proto)

	ctor := NewConstructorWithProps(0, false, "CallSite", func(args []Value) (Value, error) {
		return Undefined, vm.NewTypeError("Illegal constructor")
	})
	ctor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", protoVal)
	proto.SetOwnNonEnumerable("constructor", ctor)

	method := func(name string, fn func(vm *VM, d *ArrayObject) Value) {
		proto.SetOwnNonEnumerable(name, NewNativeFunction(0, false, name, func(args []Value) (Value, error) {
			d, err := callSiteData(vm, vm.GetThis())
			if err != nil {
				return Undefined, err
			}
			return fn(vm, d), nil
		}))
	}
	constant := func(name string, v Value) {
		method(name, func(*VM, *ArrayObject) Value { return v })
	}

	method("getFileName", func(_ *VM, d *ArrayObject) Value { return d.Get(1) })
	method("getScriptNameOrSourceURL", func(_ *VM, d *ArrayObject) Value { return d.Get(1) })
	method("getFunctionName", func(_ *VM, d *ArrayObject) Value { return d.Get(0) })
	method("getLineNumber", func(_ *VM, d *ArrayObject) Value { return d.Get(2) })
	method("getColumnNumber", func(_ *VM, d *ArrayObject) Value { return d.Get(3) })
	constant("getMethodName", Null)
	constant("getTypeName", Null)
	constant("getThis", Undefined)
	constant("getFunction", Undefined)
	constant("getEvalOrigin", Undefined)
	constant("getPromiseIndex", Null)
	for _, n := range []string{"isToplevel", "isEval", "isNative", "isConstructor", "isAsync", "isPromiseAll", "isPromiseAny"} {
		constant(n, BooleanValue(false))
	}
	method("toString", func(_ *VM, d *ArrayObject) Value {
		name := d.Get(4).ToString()
		if name == "" {
			name = "<anonymous>"
		}
		return NewString(name + " (" + d.Get(5).ToString() + ":" + strconv.Itoa(int(d.Get(2).ToFloat())) + ":" + strconv.Itoa(int(d.Get(3).ToFloat())) + ")")
	})

	vm.callSiteProto = protoVal
	return protoVal
}

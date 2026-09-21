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
	frames := vm.getStackFramesExcluding(target)

	if limit := vm.stackTraceLimit(); limit >= 0 && len(frames) > limit {
		frames = frames[:limit]
	}

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
	obj := NewObject(vm.ObjectPrototype).AsPlainObject()

	nameValue := Null
	if frame.FunctionName != "" && frame.FunctionName != "<anonymous>" {
		nameValue = NewString(frame.FunctionName)
	}

	fileValue := Null
	if frame.FileName != "" && frame.FileName != "<script>" {
		fileValue = NewString(frame.FileName)
	}

	line, column := frame.Line, frame.Column

	method := func(name string, fn func(args []Value) (Value, error)) {
		obj.SetOwnNonEnumerable(name, NewNativeFunction(0, false, name, fn))
	}

	method("getFileName", func(args []Value) (Value, error) { return fileValue, nil })
	method("getScriptNameOrSourceURL", func(args []Value) (Value, error) { return fileValue, nil })
	method("getFunctionName", func(args []Value) (Value, error) { return nameValue, nil })
	method("getMethodName", func(args []Value) (Value, error) { return Null, nil })
	method("getTypeName", func(args []Value) (Value, error) { return Null, nil })
	method("getThis", func(args []Value) (Value, error) { return Undefined, nil })
	method("getFunction", func(args []Value) (Value, error) { return Undefined, nil })
	method("getLineNumber", func(args []Value) (Value, error) { return NumberValue(float64(line)), nil })
	method("getColumnNumber", func(args []Value) (Value, error) { return NumberValue(float64(column)), nil })
	method("getEvalOrigin", func(args []Value) (Value, error) { return Undefined, nil })
	method("getPromiseIndex", func(args []Value) (Value, error) { return Null, nil })
	method("isToplevel", func(args []Value) (Value, error) { return BooleanValue(false), nil })
	method("isEval", func(args []Value) (Value, error) { return BooleanValue(false), nil })
	method("isNative", func(args []Value) (Value, error) { return BooleanValue(false), nil })
	method("isConstructor", func(args []Value) (Value, error) { return BooleanValue(false), nil })
	method("isAsync", func(args []Value) (Value, error) { return BooleanValue(false), nil })
	method("isPromiseAll", func(args []Value) (Value, error) { return BooleanValue(false), nil })
	method("isPromiseAny", func(args []Value) (Value, error) { return BooleanValue(false), nil })
	method("toString", func(args []Value) (Value, error) {
		name := frame.FunctionName
		if name == "" {
			name = "<anonymous>"
		}
		return NewString(name + " (" + frame.FileName + ":" + strconv.Itoa(line) + ":" + strconv.Itoa(column) + ")"), nil
	})

	return NewValueFromPlainObject(obj)
}

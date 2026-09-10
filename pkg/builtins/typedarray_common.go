package builtins

import (
	"fmt"
	"math"
	"math/big"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

func typedArrayInstanceType(elementType types.Type) *types.ObjectType {
	self := types.NewObjectType()
	elementOrUndefined := types.NewUnionType(elementType, types.Undefined)
	predicateType := types.NewOptionalFunction(
		[]types.Type{elementType, types.Number, self},
		types.Boolean,
		[]bool{false, true, true},
	)
	mapperType := types.NewOptionalFunction(
		[]types.Type{elementType, types.Number, self},
		types.Any,
		[]bool{false, true, true},
	)

	return self.
		WithProperty("buffer", types.Any).
		WithProperty("byteLength", types.Number).
		WithProperty("byteOffset", types.Number).
		WithProperty("length", types.Number).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("set", types.NewOptionalFunction([]types.Type{types.Any, types.Number}, types.Undefined, []bool{false, true})).
		WithProperty("subarray", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, self, []bool{true, true})).
		WithProperty("slice", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, self, []bool{true, true})).
		WithProperty("at", types.NewSimpleFunction([]types.Type{types.Number}, elementOrUndefined)).
		WithProperty("includes", types.NewOptionalFunction([]types.Type{elementType, types.Number}, types.Boolean, []bool{false, true})).
		WithProperty("indexOf", types.NewOptionalFunction([]types.Type{elementType, types.Number}, types.Number, []bool{false, true})).
		WithProperty("lastIndexOf", types.NewOptionalFunction([]types.Type{elementType, types.Number}, types.Number, []bool{false, true})).
		WithProperty("find", types.NewSimpleFunction([]types.Type{predicateType}, elementOrUndefined)).
		WithProperty("findIndex", types.NewSimpleFunction([]types.Type{predicateType}, types.Number)).
		WithProperty("findLast", types.NewSimpleFunction([]types.Type{predicateType}, elementOrUndefined)).
		WithProperty("findLastIndex", types.NewSimpleFunction([]types.Type{predicateType}, types.Number)).
		WithProperty("forEach", types.NewSimpleFunction([]types.Type{mapperType}, types.Undefined)).
		WithProperty("map", types.NewSimpleFunction([]types.Type{mapperType}, self)).
		WithProperty("filter", types.NewSimpleFunction([]types.Type{predicateType}, self)).
		WithProperty("every", types.NewSimpleFunction([]types.Type{predicateType}, types.Boolean)).
		WithProperty("some", types.NewSimpleFunction([]types.Type{predicateType}, types.Boolean)).
		WithProperty("reduce", types.NewOptionalFunction([]types.Type{types.Any, types.Any}, types.Any, []bool{false, true})).
		WithProperty("reduceRight", types.NewOptionalFunction([]types.Type{types.Any, types.Any}, types.Any, []bool{false, true})).
		WithProperty("copyWithin", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Number}, self, []bool{false, false, true})).
		WithProperty("fill", types.NewOptionalFunction([]types.Type{elementType, types.Number, types.Number}, self, []bool{false, true, true})).
		WithProperty("reverse", types.NewSimpleFunction([]types.Type{}, self)).
		WithProperty("sort", types.NewOptionalFunction([]types.Type{types.Any}, self, []bool{true})).
		WithProperty("toReversed", types.NewSimpleFunction([]types.Type{}, self)).
		WithProperty("toSorted", types.NewOptionalFunction([]types.Type{types.Any}, self, []bool{true})).
		WithProperty("with", types.NewSimpleFunction([]types.Type{types.Number, elementType}, self)).
		WithProperty("join", types.NewOptionalFunction([]types.Type{types.String}, types.String, []bool{true})).
		WithProperty("toLocaleString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("entries", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("keys", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("values", types.NewSimpleFunction([]types.Type{}, types.Any))
}

// ValidateTypedArrayByteOffset converts byteOffset to integer (calling valueOf if needed)
// and validates that it's aligned to the element size.
// Per ECMAScript spec 22.2.4.5:
// - Step 7: Let offset be ? ToInteger(byteOffset)
// - Step 10: If offset modulo elementSize ≠ 0, throw a RangeError
func ValidateTypedArrayByteOffset(vmInstance *vm.VM, byteOffsetArg vm.Value, elementSize int) (int, error) {
	// If undefined, default to 0
	if byteOffsetArg.IsUndefined() {
		return 0, nil
	}

	// Call ToInteger which properly invokes valueOf() through ToPrimitive
	offset, err := toIntegerOrInfinityWithVM(vmInstance, byteOffsetArg)
	if err != nil {
		return 0, err
	}

	// Step 8: If offset < 0, throw a RangeError
	if offset < 0 {
		return 0, vmInstance.NewRangeError("Start offset is negative")
	}

	// Step 10: If offset modulo elementSize ≠ 0, throw a RangeError
	if elementSize > 1 && offset%elementSize != 0 {
		return 0, vmInstance.NewRangeError(fmt.Sprintf("Start offset of %s should be a multiple of %d", getTypedArrayNameFromElementSize(elementSize), elementSize))
	}

	return offset, nil
}

// getTypedArrayNameFromElementSize returns the TypedArray name based on element size
func getTypedArrayNameFromElementSize(elementSize int) string {
	switch elementSize {
	case 1:
		return "Int8Array" // or Uint8Array/Uint8ClampedArray
	case 2:
		return "Int16Array" // or Uint16Array
	case 4:
		return "Int32Array" // or Uint32Array/Float32Array
	case 8:
		return "Float64Array" // or BigInt64Array/BigUint64Array
	default:
		return "TypedArray"
	}
}

// ValidateTypedArrayBufferAlignment checks that when length is undefined (auto-calculate),
// the remaining buffer bytes after byteOffset is aligned to elementSize.
// Per ECMAScript spec 22.2.4.5 step 13a:
// - If bufferByteLength modulo elementSize ≠ 0, throw a RangeError
func ValidateTypedArrayBufferAlignment(vmInstance *vm.VM, buffer *vm.ArrayBufferObject, byteOffset, elementSize int) error {
	bufferByteLength := len(buffer.GetData())
	remainingBytes := bufferByteLength - byteOffset

	if remainingBytes < 0 {
		return vmInstance.NewRangeError("Start offset is outside the bounds of the buffer")
	}

	if remainingBytes%elementSize != 0 {
		return vmInstance.NewRangeError(fmt.Sprintf("Byte length of %s should be a multiple of %d", getTypedArrayNameFromElementSize(elementSize), elementSize))
	}

	return nil
}

// ValidateTypedArrayByteOffsetShared is the SharedArrayBuffer version of ValidateTypedArrayByteOffset
func ValidateTypedArrayByteOffsetShared(vmInstance *vm.VM, byteOffsetArg vm.Value, elementSize int) (int, error) {
	// If undefined, default to 0
	if byteOffsetArg.IsUndefined() {
		return 0, nil
	}

	// Call ToInteger which properly invokes valueOf() through ToPrimitive
	offset, err := toIntegerOrInfinityWithVM(vmInstance, byteOffsetArg)
	if err != nil {
		return 0, err
	}

	// Step 8: If offset < 0, throw a RangeError
	if offset < 0 {
		return 0, vmInstance.NewRangeError("Start offset is negative")
	}

	// Step 10: If offset modulo elementSize ≠ 0, throw a RangeError
	if elementSize > 1 && offset%elementSize != 0 {
		return 0, vmInstance.NewRangeError(fmt.Sprintf("Start offset of %s should be a multiple of %d", getTypedArrayNameFromElementSize(elementSize), elementSize))
	}

	return offset, nil
}

// ValidateTypedArrayBufferAlignmentShared is the SharedArrayBuffer version of ValidateTypedArrayBufferAlignment
func ValidateTypedArrayBufferAlignmentShared(vmInstance *vm.VM, buffer *vm.SharedArrayBufferObject, byteOffset, elementSize int) error {
	bufferByteLength := len(buffer.GetData())
	remainingBytes := bufferByteLength - byteOffset

	if remainingBytes < 0 {
		return vmInstance.NewRangeError("Start offset is outside the bounds of the buffer")
	}

	if remainingBytes%elementSize != 0 {
		return vmInstance.NewRangeError(fmt.Sprintf("Byte length of %s should be a multiple of %d", getTypedArrayNameFromElementSize(elementSize), elementSize))
	}

	return nil
}

// TypedArrayRequireNewTarget implements 22.2.5.1 step 1: "If NewTarget is
// undefined, throw a TypeError exception." This check has no observable side
// effects and per spec must run before any argument-specific processing
// (ToIndex on a Symbol/BigInt argument throws TypeError too, buffer
// validation can throw RangeError, etc.) - unlike the prototype lookup in
// TypedArrayGPFC below, which runs a user-observable "prototype" getter on
// NewTarget and per spec (AllocateTypedArray) must happen *last*, after
// that argument processing, not before it.
func TypedArrayRequireNewTarget(vmInstance *vm.VM) error {
	if vmInstance.GetNewTarget().IsUndefined() {
		return vmInstance.NewTypeError("Constructor TypedArray requires 'new'")
	}
	return nil
}

// TypedArrayGPFC implements AllocateTypedArray's
// GetPrototypeFromConstructor(newTarget, defaultProto) step. Callers must
// have already checked TypedArrayRequireNewTarget and finished any
// argument-specific processing that can throw (ToIndex, buffer bounds,
// etc.) before calling this, since GetPrototypeFromConstructor can run
// user-observable code (a custom "prototype" getter on NewTarget) and per
// spec that must happen after argument validation, not before it. The
// caller must apply the returned prototype to the newly created typed array
// itself (GetPrototypeFromConstructor only computes [[Prototype]]; it
// doesn't know where to install it).
func TypedArrayGPFC(vmInstance *vm.VM, defaultProtoIntrinsic string) (vm.Value, error) {
	newTarget := vmInstance.GetNewTarget()
	if newTarget.IsUndefined() {
		return vm.Undefined, vmInstance.NewTypeError("Constructor TypedArray requires 'new'")
	}
	return vmInstance.GetPrototypeFromConstructor(newTarget, defaultProtoIntrinsic)
}

// typedArrayEffectivePrototype resolves the prototype a TypedArray instance
// actually uses: its per-instance override if one was set (subclassing, or a
// custom newTarget via Reflect.construct), else the intrinsic prototype for
// its element kind. Returns vm.Undefined if val isn't a TypedArray.
func typedArrayEffectivePrototype(vmInstance *vm.VM, val vm.Value) vm.Value {
	ta := val.AsTypedArray()
	if ta == nil {
		return vm.Undefined
	}
	if p := ta.GetPrototype(); p.IsObject() {
		return p
	}
	switch ta.GetElementType() {
	case vm.TypedArrayInt8:
		return vmInstance.Int8ArrayPrototype
	case vm.TypedArrayUint8:
		return vmInstance.Uint8ArrayPrototype
	case vm.TypedArrayUint8Clamped:
		return vmInstance.Uint8ClampedArrayPrototype
	case vm.TypedArrayInt16:
		return vmInstance.Int16ArrayPrototype
	case vm.TypedArrayUint16:
		return vmInstance.Uint16ArrayPrototype
	case vm.TypedArrayInt32:
		return vmInstance.Int32ArrayPrototype
	case vm.TypedArrayUint32:
		return vmInstance.Uint32ArrayPrototype
	case vm.TypedArrayFloat16:
		return vmInstance.Float16ArrayPrototype
	case vm.TypedArrayFloat32:
		return vmInstance.Float32ArrayPrototype
	case vm.TypedArrayFloat64:
		return vmInstance.Float64ArrayPrototype
	case vm.TypedArrayBigInt64:
		return vmInstance.BigInt64ArrayPrototype
	case vm.TypedArrayBigUint64:
		return vmInstance.BigUint64ArrayPrototype
	default:
		return vmInstance.TypedArrayPrototype
	}
}

// typedArrayIntrinsicProtoName maps a TypedArrayKind to its %Foo.prototype%
// intrinsic name, for GetPrototypeFromConstructor's cross-realm default.
func typedArrayIntrinsicProtoName(kind vm.TypedArrayKind) string {
	switch kind {
	case vm.TypedArrayUint8:
		return "%Uint8Array.prototype%"
	case vm.TypedArrayUint8Clamped:
		return "%Uint8ClampedArray.prototype%"
	case vm.TypedArrayInt8:
		return "%Int8Array.prototype%"
	case vm.TypedArrayInt16:
		return "%Int16Array.prototype%"
	case vm.TypedArrayUint16:
		return "%Uint16Array.prototype%"
	case vm.TypedArrayUint32:
		return "%Uint32Array.prototype%"
	case vm.TypedArrayInt32:
		return "%Int32Array.prototype%"
	case vm.TypedArrayFloat16:
		return "%Float16Array.prototype%"
	case vm.TypedArrayFloat32:
		return "%Float32Array.prototype%"
	case vm.TypedArrayFloat64:
		return "%Float64Array.prototype%"
	case vm.TypedArrayBigInt64:
		return "%BigInt64Array.prototype%"
	case vm.TypedArrayBigUint64:
		return "%BigUint64Array.prototype%"
	default:
		return "%TypedArrayPrototype%"
	}
}

// applyGPFCPrototype installs the prototype computed by TypedArrayGPFC onto
// a freshly created typed array value (a per-instance override; see
// TypedArrayObject.SetPrototype).
func applyGPFCPrototype(result vm.Value, proto vm.Value) vm.Value {
	if ta := result.AsTypedArray(); ta != nil {
		ta.SetPrototype(proto)
	}
	return result
}

// TypedArrayToIndex validates a non-object argument for TypedArray constructors.
// Per spec, ToIndex calls ToNumber which throws TypeError for Symbol and BigInt.
func TypedArrayToIndex(vmInstance *vm.VM, arg vm.Value) (int, error) {
	if arg.Type() == vm.TypeSymbol {
		return 0, vmInstance.NewTypeError("Cannot convert a Symbol value to a number")
	}
	if arg.Type() == vm.TypeBigInt {
		return 0, vmInstance.NewTypeError("Cannot convert a BigInt value to a number")
	}
	n := arg.ToFloat()
	// ToIndex (7.1.22): first ToIntegerOrInfinity - NaN -> 0, otherwise
	// Math.trunc (this must happen *before* the sign/range check below: a
	// value like -0.1 truncates to 0, which is in range, not "negative").
	// +Infinity (and anything beyond the safe-integer range) must also be
	// rejected here - converting an out-of-range float straight to int is
	// undefined territory in Go and previously fed a garbage length into
	// make([]Value, l), panicking instead of throwing the spec'd RangeError.
	if math.IsNaN(n) {
		n = 0
	} else {
		n = math.Trunc(n)
	}
	if math.IsInf(n, 0) || n < 0 || n > maxSafeInteger {
		return 0, vmInstance.NewRangeError("Invalid typed array length")
	}
	l := int(n)
	if l < 0 {
		return 0, vmInstance.NewRangeError("Invalid typed array length")
	}
	return l, nil
}

// TypedArrayLengthToIndex validates the optional "length" argument of a
// TypedArray-from-buffer constructor (22.2.5.1.4/1.5 step "newLength = ?
// ToIndex(length)"). Unlike TypedArrayToIndex, the argument here may be an
// object (e.g. `new Int8Array(buffer, 0, {valueOf(){...}})`), so this goes
// through the VM-aware toIntegerOrInfinityWithVM (which calls ToPrimitive and
// properly propagates an exception thrown from valueOf/toString) rather than
// the bare Value.ToFloat() used by TypedArrayToIndex's non-object callers.
func TypedArrayLengthToIndex(vmInstance *vm.VM, arg vm.Value) (int, error) {
	if arg.Type() == vm.TypeBigInt {
		return 0, vmInstance.NewTypeError("Cannot convert a BigInt value to a number")
	}
	n, err := toIntegerOrInfinityWithVM(vmInstance, arg)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, vmInstance.NewRangeError("Invalid typed array length")
	}
	return n, nil
}

// computeBufferViewOffsetLength implements the byteOffset/length portion of
// InitializeTypedArrayFromArrayBuffer (22.2.5.1.4) shared by every
// %TypedArray% subclass constructor's ArrayBuffer overload: ToIndex(byteOffset)
// and its alignment check, then ToIndex(length) if given. Per spec the
// detached-buffer check happens AFTER both coercions (either of which may run
// arbitrary user code via valueOf/toString that detaches buf), followed by
// alignment (length omitted) or a bounds check (length given). Returns
// length == -1 when the resulting view should auto/length-track the buffer.
func computeBufferViewOffsetLength(vmInstance *vm.VM, buf *vm.ArrayBufferObject, args []vm.Value, elementSize int) (off int, length int, err error) {
	byteOffsetArg := vm.Undefined
	if len(args) > 1 {
		byteOffsetArg = args[1]
	}
	off, err = ValidateTypedArrayByteOffset(vmInstance, byteOffsetArg, elementSize)
	if err != nil {
		return 0, 0, err
	}
	length = -1
	if len(args) > 2 && !args[2].IsUndefined() {
		length, err = TypedArrayLengthToIndex(vmInstance, args[2])
		if err != nil {
			return 0, 0, err
		}
	}
	if buf.IsDetached() {
		return 0, 0, vmInstance.NewTypeError("Cannot perform operation on a detached ArrayBuffer")
	}
	if length == -1 {
		if err := ValidateTypedArrayBufferAlignment(vmInstance, buf, off, elementSize); err != nil {
			return 0, 0, err
		}
		return off, -1, nil
	}
	if off+length*elementSize > len(buf.GetData()) {
		return 0, 0, vmInstance.NewRangeError("Invalid typed array length")
	}
	return off, length, nil
}

// computeSharedBufferViewOffsetLength is the SharedArrayBuffer counterpart of
// computeBufferViewOffsetLength. SharedArrayBuffers cannot be detached, so
// there is no post-coercion detach recheck.
func computeSharedBufferViewOffsetLength(vmInstance *vm.VM, sab *vm.SharedArrayBufferObject, args []vm.Value, elementSize int) (off int, length int, err error) {
	byteOffsetArg := vm.Undefined
	if len(args) > 1 {
		byteOffsetArg = args[1]
	}
	off, err = ValidateTypedArrayByteOffsetShared(vmInstance, byteOffsetArg, elementSize)
	if err != nil {
		return 0, 0, err
	}
	length = -1
	if len(args) > 2 && !args[2].IsUndefined() {
		length, err = TypedArrayLengthToIndex(vmInstance, args[2])
		if err != nil {
			return 0, 0, err
		}
	}
	if length == -1 {
		if err := ValidateTypedArrayBufferAlignmentShared(vmInstance, sab, off, elementSize); err != nil {
			return 0, 0, err
		}
		return off, -1, nil
	}
	if off+length*elementSize > len(sab.GetData()) {
		return 0, 0, vmInstance.NewRangeError("Invalid typed array length")
	}
	return off, length, nil
}

// TypedArrayElementKind describes what differs between numeric %TypedArray%
// subclass constructors (Int8Array, ..., BigInt64Array, BigUint64Array); the
// shared body in NumericTypedArrayCtorBody implements everything else once.
type TypedArrayElementKind struct {
	Kind        vm.TypedArrayKind
	ElementSize int
	IsBigInt    bool
	Unsigned    bool // for IsBigInt only: BigUint64Array vs BigInt64Array
}

// coerceTypedArrayElement converts a plain JS value (as found in a source
// array/iterable) to the representation a typed array of this element kind
// stores: BigInt arrays require an actual BigInt, converting a Number via
// truncation (matching the pre-existing per-file behavior, signed or
// unsigned per Kind); everything else is stored as-is.
func (ek TypedArrayElementKind) coerceElement(v vm.Value) vm.Value {
	if ek.IsBigInt && !v.IsBigInt() {
		if ek.Unsigned {
			return vm.NewBigInt(new(big.Int).SetUint64(uint64(v.ToFloat())))
		}
		return vm.NewBigInt(big.NewInt(int64(v.ToFloat())))
	}
	return v
}

// NumericTypedArrayCtorBody returns the native constructor function body
// shared by every numeric/BigInt %TypedArray% subclass, dispatching on the
// first argument per ECMAScript 23.2.5 %TypedArray%(...): no args, a length,
// an ArrayBuffer/SharedArrayBuffer (with optional byteOffset/length), an
// array-like, or a coercible primitive.
func NumericTypedArrayCtorBody(vmInstance *vm.VM, ek TypedArrayElementKind) func(args []vm.Value) (vm.Value, error) {
	kind := ek.Kind
	elementSize := ek.ElementSize
	protoName := typedArrayIntrinsicProtoName(kind)
	return func(args []vm.Value) (vm.Value, error) {
		// Step 1: NewTarget undefined check, before any argument processing.
		if err := TypedArrayRequireNewTarget(vmInstance); err != nil {
			return vm.Undefined, err
		}
		if len(args) == 0 {
			proto, err := TypedArrayGPFC(vmInstance, protoName)
			if err != nil {
				return vm.Undefined, err
			}
			return applyGPFCPrototype(vm.NewTypedArray(kind, 0, 0, 0), proto), nil
		}
		arg := args[0]
		if arg.IsNumber() {
			// ToIndex(length) (step 3) before GPFC's prototype lookup: that
			// lookup can run a user-observable getter and per spec
			// (AllocateTypedArray) happens after argument processing, not
			// before it. NewTarget-undefined was already checked above.
			l, err := TypedArrayToIndex(vmInstance, arg)
			if err != nil {
				return vm.Undefined, err
			}
			proto, err := TypedArrayGPFC(vmInstance, protoName)
			if err != nil {
				return vm.Undefined, err
			}
			return applyGPFCPrototype(vm.NewTypedArray(kind, l, 0, 0), proto), nil
		}
		if buf := arg.AsArrayBuffer(); buf != nil {
			proto, err := TypedArrayGPFC(vmInstance, protoName)
			if err != nil {
				return vm.Undefined, err
			}
			off, ln, err := computeBufferViewOffsetLength(vmInstance, buf, args, elementSize)
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			return applyGPFCPrototype(vm.NewTypedArray(kind, buf, off, ln), proto), nil
		}
		if sab := arg.AsSharedArrayBuffer(); sab != nil {
			proto, err := TypedArrayGPFC(vmInstance, protoName)
			if err != nil {
				return vm.Undefined, err
			}
			off, ln, err := computeSharedBufferViewOffsetLength(vmInstance, sab, args, elementSize)
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			return applyGPFCPrototype(vm.NewTypedArray(kind, sab, off, ln), proto), nil
		}
		if arg.Type() == vm.TypeArray {
			arr := arg.AsArray()
			proto, err := TypedArrayGPFC(vmInstance, protoName)
			if err != nil {
				return vm.Undefined, err
			}
			vals := make([]vm.Value, arr.Length())
			for i := 0; i < arr.Length(); i++ {
				vals[i] = ek.coerceElement(arr.Get(i))
			}
			return applyGPFCPrototype(vm.NewTypedArray(kind, vals, 0, 0), proto), nil
		}
		if srcTA := arg.AsTypedArray(); srcTA != nil {
			// %TypedArray%(typedArray): copy/reinterpret the source's
			// elements into a freshly allocated typed array of this kind.
			// Per InitializeTypedArrayFromTypedArray: source and target must
			// agree on BigInt-ness (Number and BigInt content types never mix).
			srcIsBigInt := srcTA.GetElementType() == vm.TypedArrayBigInt64 || srcTA.GetElementType() == vm.TypedArrayBigUint64
			if srcIsBigInt != ek.IsBigInt {
				return vm.Undefined, vmInstance.NewTypeError("Cannot mix BigInt and other types, use explicit conversions")
			}
			proto, err := TypedArrayGPFC(vmInstance, protoName)
			if err != nil {
				return vm.Undefined, err
			}
			srcLen := srcTA.GetLength()
			vals := make([]vm.Value, srcLen)
			for i := 0; i < srcLen; i++ {
				vals[i] = ek.coerceElement(srcTA.GetElement(i))
			}
			return applyGPFCPrototype(vm.NewTypedArray(kind, vals, 0, 0), proto), nil
		}
		if arg.IsObject() {
			proto, err := TypedArrayGPFC(vmInstance, protoName)
			if err != nil {
				return vm.Undefined, err
			}
			return applyGPFCPrototype(vm.NewTypedArray(kind, 0, 0, 0), proto), nil
		}
		// Non-number, non-object (e.g. string, Symbol, BigInt): ToIndex
		// before GPFC's prototype lookup, matching the number branch above.
		l, err := TypedArrayToIndex(vmInstance, arg)
		if err != nil {
			return vm.Undefined, err
		}
		proto, err := TypedArrayGPFC(vmInstance, protoName)
		if err != nil {
			return vm.Undefined, err
		}
		return applyGPFCPrototype(vm.NewTypedArray(kind, l, 0, 0), proto), nil
	}
}

// TypedArrayFromArrayLike implements the shared body of the legacy (non-spec,
// array-only) %TypedArray%.from used by each subclass's own "from" method.
func TypedArrayFromArrayLike(ek TypedArrayElementKind, args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.NewTypedArray(ek.Kind, 0, 0, 0)
	}
	source := args[0]
	if source.Type() == vm.TypeArray {
		sourceArray := source.AsArray()
		values := make([]vm.Value, sourceArray.Length())
		for i := 0; i < sourceArray.Length(); i++ {
			values[i] = ek.coerceElement(sourceArray.Get(i))
		}
		return vm.NewTypedArray(ek.Kind, values, 0, 0)
	}
	return vm.NewTypedArray(ek.Kind, 0, 0, 0)
}

// TypedArrayOfValues implements the shared body of each subclass's "of" method.
func TypedArrayOfValues(ek TypedArrayElementKind, args []vm.Value) vm.Value {
	if !ek.IsBigInt {
		return vm.NewTypedArray(ek.Kind, args, 0, 0)
	}
	values := make([]vm.Value, len(args))
	for i, v := range args {
		values[i] = ek.coerceElement(v)
	}
	return vm.NewTypedArray(ek.Kind, values, 0, 0)
}

// SetupTypedArrayConstructorProperties sets up the constructor properties with correct descriptors.
// Per ECMAScript spec:
// - BYTES_PER_ELEMENT: { writable: false, enumerable: false, configurable: false }
// - prototype: { writable: false, enumerable: false, configurable: false }
// - length: { writable: false, enumerable: false, configurable: true } - value is 3
func SetupTypedArrayConstructorProperties(ctor vm.Value, proto *vm.PlainObject, bytesPerElement int) {
	props := ctor.AsNativeFunctionWithProps().Properties

	// BYTES_PER_ELEMENT: { writable: false, enumerable: false, configurable: false }
	w, e, c := false, false, false

	props.DefineOwnProperty("BYTES_PER_ELEMENT", vm.Number(float64(bytesPerElement)), &w, &e, &c)

	// prototype: { writable: false, enumerable: false, configurable: false }
	props.DefineOwnProperty("prototype", vm.NewValueFromPlainObject(proto), &w, &e, &c)
}

// SetupTypedArrayPrototypeProperties sets up the prototype properties with correct descriptors.
// Per ECMAScript spec:
// - BYTES_PER_ELEMENT: { writable: false, enumerable: false, configurable: false }
// Note: buffer, byteLength, byteOffset, length accessors are on %TypedArray%.prototype (base),
// not on each subtype prototype. See typedarray_base_init.go.
func SetupTypedArrayPrototypeProperties(proto *vm.PlainObject, vmInstance *vm.VM, bytesPerElement int) {
	// BYTES_PER_ELEMENT: { writable: false, enumerable: false, configurable: false }
	w, e, c := false, false, false
	proto.DefineOwnProperty("BYTES_PER_ELEMENT", vm.Number(float64(bytesPerElement)), &w, &e, &c)
}

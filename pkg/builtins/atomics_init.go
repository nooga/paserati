package builtins

import (
	"encoding/binary"
	"math"
	"math/big"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type AtomicsInitializer struct{}

func (a *AtomicsInitializer) Name() string {
	return "Atomics"
}

func (a *AtomicsInitializer) Priority() int {
	return PriorityMath + 5 // After Math, after SharedArrayBuffer/TypedArrays
}

func (a *AtomicsInitializer) InitTypes(ctx *TypeContext) error {
	// Create Atomics namespace type with all methods
	atomicsType := types.NewObjectType().
		WithProperty("add", types.NewSimpleFunction([]types.Type{types.Any, types.Number, types.Number}, types.Number)).
		WithProperty("and", types.NewSimpleFunction([]types.Type{types.Any, types.Number, types.Number}, types.Number)).
		WithProperty("compareExchange", types.NewSimpleFunction([]types.Type{types.Any, types.Number, types.Number, types.Number}, types.Number)).
		WithProperty("exchange", types.NewSimpleFunction([]types.Type{types.Any, types.Number, types.Number}, types.Number)).
		WithProperty("isLockFree", types.NewSimpleFunction([]types.Type{types.Number}, types.Boolean)).
		WithProperty("load", types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Number)).
		WithProperty("notify", types.NewSimpleFunction([]types.Type{types.Any, types.Number, types.Number}, types.Number)).
		WithProperty("or", types.NewSimpleFunction([]types.Type{types.Any, types.Number, types.Number}, types.Number)).
		WithProperty("pause", types.NewOptionalFunction([]types.Type{types.Number}, types.Undefined, []bool{true})).
		WithProperty("store", types.NewSimpleFunction([]types.Type{types.Any, types.Number, types.Number}, types.Number)).
		WithProperty("sub", types.NewSimpleFunction([]types.Type{types.Any, types.Number, types.Number}, types.Number)).
		WithProperty("wait", types.NewSimpleFunction([]types.Type{types.Any, types.Number, types.Number, types.Number}, types.String)).
		WithProperty("waitAsync", types.NewSimpleFunction([]types.Type{types.Any, types.Number, types.Number, types.Number}, types.Any)).
		WithProperty("xor", types.NewSimpleFunction([]types.Type{types.Any, types.Number, types.Number}, types.Number))

	return ctx.DefineGlobal("Atomics", atomicsType)
}

func (a *AtomicsInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Atomics object with Object.prototype as its prototype
	atomicsObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Set @@toStringTag to "Atomics"
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		falseVal := false
		trueVal := true
		atomicsObj.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Atomics"),
			&falseVal, // writable: false
			&falseVal, // enumerable: false
			&trueVal,  // configurable: true (per ECMAScript spec)
		)
	}

	// Helper function to validate and get TypedArray for atomic operations
	// Returns the TypedArray, byte offset for the element, and error
	validateAtomicAccess := func(typedArrayArg, indexArg vm.Value) (*vm.TypedArrayObject, int, error) {
		ta := typedArrayArg.AsTypedArray()
		if ta == nil {
			return nil, 0, vmInstance.NewTypeError("Atomics operation requires a TypedArray")
		}

		// Check for valid TypedArray types (not Float32, Float64, or Uint8Clamped)
		elemType := ta.GetElementType()
		if elemType == vm.TypedArrayFloat32 || elemType == vm.TypedArrayFloat64 || elemType == vm.TypedArrayUint8Clamped {
			return nil, 0, vmInstance.NewTypeError("Atomics operations are not allowed on Float32Array, Float64Array, or Uint8ClampedArray")
		}

		// Get the index
		index := int(vmInstance.ToNumber(indexArg))

		// Validate index bounds
		if index < 0 || index >= ta.GetLength() {
			return nil, 0, vmInstance.NewRangeError("Invalid atomic access index")
		}

		// Calculate byte offset
		byteOffset := ta.GetByteOffset() + index*ta.GetBytesPerElement()

		return ta, byteOffset, nil
	}

	// atomicLoad reads a value atomically from the TypedArray
	atomicLoad := func(ta *vm.TypedArrayObject, byteOffset int) vm.Value {
		data := ta.GetBufferData().GetData()[byteOffset:]
		elemType := ta.GetElementType()

		switch elemType {
		case vm.TypedArrayInt8:
			return vm.Number(float64(int8(data[0])))
		case vm.TypedArrayUint8:
			return vm.Number(float64(data[0]))
		case vm.TypedArrayInt16:
			return vm.Number(float64(int16(binary.LittleEndian.Uint16(data))))
		case vm.TypedArrayUint16:
			return vm.Number(float64(binary.LittleEndian.Uint16(data)))
		case vm.TypedArrayInt32:
			return vm.Number(float64(int32(binary.LittleEndian.Uint32(data))))
		case vm.TypedArrayUint32:
			return vm.Number(float64(binary.LittleEndian.Uint32(data)))
		case vm.TypedArrayBigInt64:
			i64 := int64(binary.LittleEndian.Uint64(data))
			return vm.NewBigInt(big.NewInt(i64))
		case vm.TypedArrayBigUint64:
			u64 := binary.LittleEndian.Uint64(data)
			return vm.NewBigInt(new(big.Int).SetUint64(u64))
		default:
			return vm.Undefined
		}
	}

	// atomicStore writes a value atomically to the TypedArray
	atomicStore := func(ta *vm.TypedArrayObject, byteOffset int, value vm.Value) {
		data := ta.GetBufferData().GetData()[byteOffset:]
		elemType := ta.GetElementType()

		switch elemType {
		case vm.TypedArrayInt8, vm.TypedArrayUint8:
			data[0] = byte(int8(vmInstance.ToNumber(value)))
		case vm.TypedArrayInt16, vm.TypedArrayUint16:
			binary.LittleEndian.PutUint16(data, uint16(int16(vmInstance.ToNumber(value))))
		case vm.TypedArrayInt32, vm.TypedArrayUint32:
			binary.LittleEndian.PutUint32(data, uint32(int32(vmInstance.ToNumber(value))))
		case vm.TypedArrayBigInt64:
			if value.IsBigInt() {
				binary.LittleEndian.PutUint64(data, uint64(value.AsBigInt().Int64()))
			} else {
				binary.LittleEndian.PutUint64(data, uint64(int64(vmInstance.ToNumber(value))))
			}
		case vm.TypedArrayBigUint64:
			if value.IsBigInt() {
				binary.LittleEndian.PutUint64(data, value.AsBigInt().Uint64())
			} else {
				binary.LittleEndian.PutUint64(data, uint64(vmInstance.ToNumber(value)))
			}
		}
	}

	// atomicReadInt64 reads raw 64-bit value (for atomic operations)
	atomicReadInt64 := func(ta *vm.TypedArrayObject, byteOffset int) int64 {
		data := ta.GetBufferData().GetData()[byteOffset:]
		elemType := ta.GetElementType()

		switch elemType {
		case vm.TypedArrayInt8:
			return int64(int8(data[0]))
		case vm.TypedArrayUint8:
			return int64(data[0])
		case vm.TypedArrayInt16:
			return int64(int16(binary.LittleEndian.Uint16(data)))
		case vm.TypedArrayUint16:
			return int64(binary.LittleEndian.Uint16(data))
		case vm.TypedArrayInt32:
			return int64(int32(binary.LittleEndian.Uint32(data)))
		case vm.TypedArrayUint32:
			return int64(binary.LittleEndian.Uint32(data))
		case vm.TypedArrayBigInt64:
			return int64(binary.LittleEndian.Uint64(data))
		case vm.TypedArrayBigUint64:
			return int64(binary.LittleEndian.Uint64(data))
		default:
			return 0
		}
	}

	// atomicWriteInt64 writes raw 64-bit value (for atomic operations)
	atomicWriteInt64 := func(ta *vm.TypedArrayObject, byteOffset int, val int64) {
		data := ta.GetBufferData().GetData()[byteOffset:]
		elemType := ta.GetElementType()

		switch elemType {
		case vm.TypedArrayInt8, vm.TypedArrayUint8:
			data[0] = byte(val)
		case vm.TypedArrayInt16, vm.TypedArrayUint16:
			binary.LittleEndian.PutUint16(data, uint16(val))
		case vm.TypedArrayInt32, vm.TypedArrayUint32:
			binary.LittleEndian.PutUint32(data, uint32(val))
		case vm.TypedArrayBigInt64, vm.TypedArrayBigUint64:
			binary.LittleEndian.PutUint64(data, uint64(val))
		}
	}

	// Convert value to appropriate integer for atomic operations
	toAtomicValue := func(ta *vm.TypedArrayObject, value vm.Value) int64 {
		elemType := ta.GetElementType()
		if elemType == vm.TypedArrayBigInt64 || elemType == vm.TypedArrayBigUint64 {
			if value.IsBigInt() {
				return value.AsBigInt().Int64()
			}
		}
		return int64(vmInstance.ToNumber(value))
	}

	// Convert int64 back to appropriate Value
	fromAtomicValue := func(ta *vm.TypedArrayObject, val int64) vm.Value {
		elemType := ta.GetElementType()
		switch elemType {
		case vm.TypedArrayInt8:
			return vm.Number(float64(int8(val)))
		case vm.TypedArrayUint8:
			return vm.Number(float64(uint8(val)))
		case vm.TypedArrayInt16:
			return vm.Number(float64(int16(val)))
		case vm.TypedArrayUint16:
			return vm.Number(float64(uint16(val)))
		case vm.TypedArrayInt32:
			return vm.Number(float64(int32(val)))
		case vm.TypedArrayUint32:
			return vm.Number(float64(uint32(val)))
		case vm.TypedArrayBigInt64:
			return vm.NewBigInt(big.NewInt(val))
		case vm.TypedArrayBigUint64:
			return vm.NewBigInt(new(big.Int).SetUint64(uint64(val)))
		default:
			return vm.Undefined
		}
	}

	// Atomics.add(typedArray, index, value)
	atomicsObj.SetOwnNonEnumerable("add", vm.NewNativeFunction(3, false, "add", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 3 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.add requires 3 arguments")
		}

		ta, byteOffset, err := validateAtomicAccess(args[0], args[1])
		if err != nil {
			return vm.Undefined, err
		}

		oldVal := atomicReadInt64(ta, byteOffset)
		addVal := toAtomicValue(ta, args[2])
		newVal := oldVal + addVal
		atomicWriteInt64(ta, byteOffset, newVal)

		return fromAtomicValue(ta, oldVal), nil
	}))

	// Atomics.and(typedArray, index, value)
	atomicsObj.SetOwnNonEnumerable("and", vm.NewNativeFunction(3, false, "and", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 3 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.and requires 3 arguments")
		}

		ta, byteOffset, err := validateAtomicAccess(args[0], args[1])
		if err != nil {
			return vm.Undefined, err
		}

		oldVal := atomicReadInt64(ta, byteOffset)
		andVal := toAtomicValue(ta, args[2])
		newVal := oldVal & andVal
		atomicWriteInt64(ta, byteOffset, newVal)

		return fromAtomicValue(ta, oldVal), nil
	}))

	// Atomics.compareExchange(typedArray, index, expectedValue, replacementValue)
	atomicsObj.SetOwnNonEnumerable("compareExchange", vm.NewNativeFunction(4, false, "compareExchange", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 4 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.compareExchange requires 4 arguments")
		}

		ta, byteOffset, err := validateAtomicAccess(args[0], args[1])
		if err != nil {
			return vm.Undefined, err
		}

		oldVal := atomicReadInt64(ta, byteOffset)
		expectedVal := toAtomicValue(ta, args[2])
		replacementVal := toAtomicValue(ta, args[3])

		if oldVal == expectedVal {
			atomicWriteInt64(ta, byteOffset, replacementVal)
		}

		return fromAtomicValue(ta, oldVal), nil
	}))

	// Atomics.exchange(typedArray, index, value)
	atomicsObj.SetOwnNonEnumerable("exchange", vm.NewNativeFunction(3, false, "exchange", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 3 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.exchange requires 3 arguments")
		}

		ta, byteOffset, err := validateAtomicAccess(args[0], args[1])
		if err != nil {
			return vm.Undefined, err
		}

		oldVal := atomicReadInt64(ta, byteOffset)
		newVal := toAtomicValue(ta, args[2])
		atomicWriteInt64(ta, byteOffset, newVal)

		return fromAtomicValue(ta, oldVal), nil
	}))

	// Atomics.isLockFree(size)
	atomicsObj.SetOwnNonEnumerable("isLockFree", vm.NewNativeFunction(1, false, "isLockFree", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.BooleanValue(false), nil
		}

		size := int(vmInstance.ToNumber(args[0]))

		// Per ECMAScript spec:
		// - Size 4 MUST return true
		// - Sizes 1, 2, 8 return implementation-dependent boolean
		// - All other sizes return false
		// On modern platforms, all these sizes are typically lock-free
		switch size {
		case 1, 2, 4, 8:
			return vm.BooleanValue(true), nil
		default:
			return vm.BooleanValue(false), nil
		}
	}))

	// Atomics.load(typedArray, index)
	atomicsObj.SetOwnNonEnumerable("load", vm.NewNativeFunction(2, false, "load", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.load requires 2 arguments")
		}

		ta, byteOffset, err := validateAtomicAccess(args[0], args[1])
		if err != nil {
			return vm.Undefined, err
		}

		return atomicLoad(ta, byteOffset), nil
	}))

	// Atomics.notify(typedArray, index, count)
	// Per ECMAScript spec:
	// 1. Let buffer be ? ValidateSharedIntegerTypedArray(typedArray, true).
	// 2. Let i be ? ValidateAtomicAccess(typedArray, index).
	// 3. If count is undefined, let c be +∞.
	// 4. Else, let intCount be ? ToInteger(count). (Symbols throw TypeError)
	atomicsObj.SetOwnNonEnumerable("notify", vm.NewNativeFunction(3, false, "notify", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.notify requires at least 2 arguments")
		}

		// Step 1: ValidateSharedIntegerTypedArray
		ta := args[0].AsTypedArray()
		if ta == nil {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.notify requires an Int32Array or BigInt64Array")
		}

		// Atomics.notify only works with Int32Array and BigInt64Array
		elemType := ta.GetElementType()
		if elemType != vm.TypedArrayInt32 && elemType != vm.TypedArrayBigInt64 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.notify requires an Int32Array or BigInt64Array")
		}

		// Step 2: ValidateAtomicAccess (validate index)
		index := int(vmInstance.ToNumber(args[1]))
		if index < 0 || index >= ta.GetLength() {
			return vm.Undefined, vmInstance.NewRangeError("Invalid atomic access index")
		}

		// Step 3-4: Validate count argument
		// If count is provided and not undefined, it must be convertible to a number
		// Symbols throw TypeError when converted to number
		if len(args) > 2 && !args[2].IsUndefined() {
			if args[2].Type() == vm.TypeSymbol {
				return vm.Undefined, vmInstance.NewTypeError("Cannot convert a Symbol value to a number")
			}
			// Convert to number to validate
			_ = vmInstance.ToNumber(args[2])
		}

		// In single-threaded execution, no agents are waiting, so return 0
		return vm.Number(0), nil
	}))

	// Atomics.or(typedArray, index, value)
	atomicsObj.SetOwnNonEnumerable("or", vm.NewNativeFunction(3, false, "or", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 3 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.or requires 3 arguments")
		}

		ta, byteOffset, err := validateAtomicAccess(args[0], args[1])
		if err != nil {
			return vm.Undefined, err
		}

		oldVal := atomicReadInt64(ta, byteOffset)
		orVal := toAtomicValue(ta, args[2])
		newVal := oldVal | orVal
		atomicWriteInt64(ta, byteOffset, newVal)

		return fromAtomicValue(ta, oldVal), nil
	}))

	// Atomics.pause([iterationNumber])
	// Per ECMAScript spec:
	// - If iterationNumber is present and not undefined, it must be an integral Number
	// - Throws TypeError for non-Number types or non-integral numbers
	// - length property is 0 (optional argument)
	atomicsObj.SetOwnNonEnumerable("pause", vm.NewNativeFunction(0, false, "pause", func(args []vm.Value) (vm.Value, error) {
		if len(args) > 0 && !args[0].IsUndefined() {
			arg := args[0]
			// Must be a Number (not BigInt, not other types)
			if !arg.IsNumber() {
				return vm.Undefined, vmInstance.NewTypeError("Atomics.pause iterationNumber must be a non-negative integer")
			}
			num := arg.ToFloat()
			// Must be a non-negative integral number
			if math.IsNaN(num) || math.IsInf(num, 0) || num != math.Trunc(num) || num < 0 {
				return vm.Undefined, vmInstance.NewTypeError("Atomics.pause iterationNumber must be a non-negative integer")
			}
		}
		// Atomics.pause is a no-op hint to the CPU
		return vm.Undefined, nil
	}))

	// Atomics.store(typedArray, index, value)
	atomicsObj.SetOwnNonEnumerable("store", vm.NewNativeFunction(3, false, "store", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 3 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.store requires 3 arguments")
		}

		ta, byteOffset, err := validateAtomicAccess(args[0], args[1])
		if err != nil {
			return vm.Undefined, err
		}

		value := args[2]
		atomicStore(ta, byteOffset, value)

		// Atomics.store returns the value that was stored (converted to number/bigint)
		elemType := ta.GetElementType()
		if elemType == vm.TypedArrayBigInt64 || elemType == vm.TypedArrayBigUint64 {
			if value.IsBigInt() {
				return value, nil
			}
			return vm.NewBigInt(big.NewInt(int64(vmInstance.ToNumber(value)))), nil
		}

		// For non-BigInt arrays, return ToInteger(value)
		num := vmInstance.ToNumber(value)
		if math.IsNaN(num) {
			return vm.Number(0), nil
		}
		if num == 0 || math.IsInf(num, 0) {
			return vm.Number(num), nil
		}
		if num < 0 {
			return vm.Number(-math.Floor(math.Abs(num))), nil
		}
		return vm.Number(math.Floor(num)), nil
	}))

	// Atomics.sub(typedArray, index, value)
	atomicsObj.SetOwnNonEnumerable("sub", vm.NewNativeFunction(3, false, "sub", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 3 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.sub requires 3 arguments")
		}

		ta, byteOffset, err := validateAtomicAccess(args[0], args[1])
		if err != nil {
			return vm.Undefined, err
		}

		oldVal := atomicReadInt64(ta, byteOffset)
		subVal := toAtomicValue(ta, args[2])
		newVal := oldVal - subVal
		atomicWriteInt64(ta, byteOffset, newVal)

		return fromAtomicValue(ta, oldVal), nil
	}))

	// Atomics.wait(typedArray, index, value, timeout)
	// Per ECMAScript spec, validation order is:
	// 1. ValidateSharedIntegerTypedArray(typedArray, true)
	// 2. ValidateAtomicAccess(typedArray, index)
	// 3. ToInt32(value) or ToBigInt64(value)
	// 4. ToNumber(timeout)
	// 5. Check AgentCanSuspend() - throw if false
	atomicsObj.SetOwnNonEnumerable("wait", vm.NewNativeFunction(4, false, "wait", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 3 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.wait requires at least 3 arguments")
		}

		// Step 1: ValidateSharedIntegerTypedArray
		ta := args[0].AsTypedArray()
		if ta == nil {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.wait requires an Int32Array or BigInt64Array")
		}

		// Atomics.wait only works with Int32Array and BigInt64Array
		elemType := ta.GetElementType()
		if elemType != vm.TypedArrayInt32 && elemType != vm.TypedArrayBigInt64 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.wait requires an Int32Array or BigInt64Array")
		}

		// Step 2: ValidateAtomicAccess (validate index)
		index := int(vmInstance.ToNumber(args[1]))
		if index < 0 || index >= ta.GetLength() {
			return vm.Undefined, vmInstance.NewRangeError("Invalid atomic access index")
		}

		// Step 3: Coerce value (ToInt32 or ToBigInt64)
		// This validates the value argument
		_ = toAtomicValue(ta, args[2])

		// Step 4: ToNumber(timeout) - validates timeout argument
		if len(args) > 3 && !args[3].IsUndefined() {
			_ = vmInstance.ToNumber(args[3])
		}

		// Step 5: AgentCanSuspend() returns false in main thread
		// In single-threaded environment, we simulate this behavior
		// Since we're always "main thread", we should throw
		return vm.Undefined, vmInstance.NewTypeError("Atomics.wait cannot be called in the main thread")
	}))

	// Atomics.waitAsync(typedArray, index, value, timeout)
	// Returns a promise that resolves when the wait completes
	atomicsObj.SetOwnNonEnumerable("waitAsync", vm.NewNativeFunction(4, false, "waitAsync", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 3 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.waitAsync requires at least 3 arguments")
		}

		ta := args[0].AsTypedArray()
		if ta == nil {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.waitAsync requires an Int32Array or BigInt64Array")
		}

		// Atomics.waitAsync only works with Int32Array and BigInt64Array
		elemType := ta.GetElementType()
		if elemType != vm.TypedArrayInt32 && elemType != vm.TypedArrayBigInt64 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.waitAsync requires an Int32Array or BigInt64Array")
		}

		// Validate index
		index := int(vmInstance.ToNumber(args[1]))
		if index < 0 || index >= ta.GetLength() {
			return vm.Undefined, vmInstance.NewRangeError("Invalid atomic access index")
		}

		byteOffset := ta.GetByteOffset() + index*ta.GetBytesPerElement()

		// Read the current value
		currentVal := atomicReadInt64(ta, byteOffset)
		expectedVal := toAtomicValue(ta, args[2])

		// Create result object with { async: false, value: "not-equal" | "ok" }
		resultObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

		if currentVal != expectedVal {
			// Value doesn't match - return immediately with "not-equal"
			resultObj.SetOwn("async", vm.BooleanValue(false))
			resultObj.SetOwn("value", vm.NewString("not-equal"))
		} else {
			// In single-threaded environment, we can never wake up, so timeout immediately
			// Per spec, if timeout is 0 or negative, return "timed-out" synchronously
			resultObj.SetOwn("async", vm.BooleanValue(false))
			resultObj.SetOwn("value", vm.NewString("timed-out"))
		}

		return vm.NewValueFromPlainObject(resultObj), nil
	}))

	// Atomics.xor(typedArray, index, value)
	atomicsObj.SetOwnNonEnumerable("xor", vm.NewNativeFunction(3, false, "xor", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 3 {
			return vm.Undefined, vmInstance.NewTypeError("Atomics.xor requires 3 arguments")
		}

		ta, byteOffset, err := validateAtomicAccess(args[0], args[1])
		if err != nil {
			return vm.Undefined, err
		}

		oldVal := atomicReadInt64(ta, byteOffset)
		xorVal := toAtomicValue(ta, args[2])
		newVal := oldVal ^ xorVal
		atomicWriteInt64(ta, byteOffset, newVal)

		return fromAtomicValue(ta, oldVal), nil
	}))

	// Register Atomics object as global
	return ctx.DefineGlobal("Atomics", vm.NewValueFromPlainObject(atomicsObj))
}

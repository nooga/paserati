package builtins

import (
	"math"
	"math/big"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type DataViewInitializer struct{}

func (d *DataViewInitializer) Name() string {
	return "DataView"
}

func (d *DataViewInitializer) Priority() int {
	return 415 // After ArrayBuffer, before TypedArrays
}

func (d *DataViewInitializer) InitTypes(ctx *TypeContext) error {
	// Create DataView.prototype type
	dataViewProtoType := types.NewObjectType().
		WithProperty("buffer", types.Any).
		WithProperty("byteLength", types.Number).
		WithProperty("byteOffset", types.Number).
		WithProperty("getInt8", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("getUint8", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("getInt16", types.NewOptionalFunction([]types.Type{types.Number, types.Boolean}, types.Number, []bool{false, true})).
		WithProperty("getUint16", types.NewOptionalFunction([]types.Type{types.Number, types.Boolean}, types.Number, []bool{false, true})).
		WithProperty("getInt32", types.NewOptionalFunction([]types.Type{types.Number, types.Boolean}, types.Number, []bool{false, true})).
		WithProperty("getUint32", types.NewOptionalFunction([]types.Type{types.Number, types.Boolean}, types.Number, []bool{false, true})).
		WithProperty("getFloat32", types.NewOptionalFunction([]types.Type{types.Number, types.Boolean}, types.Number, []bool{false, true})).
		WithProperty("getFloat64", types.NewOptionalFunction([]types.Type{types.Number, types.Boolean}, types.Number, []bool{false, true})).
		WithProperty("getBigInt64", types.NewOptionalFunction([]types.Type{types.Number, types.Boolean}, types.BigInt, []bool{false, true})).
		WithProperty("getBigUint64", types.NewOptionalFunction([]types.Type{types.Number, types.Boolean}, types.BigInt, []bool{false, true})).
		WithProperty("setInt8", types.NewSimpleFunction([]types.Type{types.Number, types.Number}, types.Undefined)).
		WithProperty("setUint8", types.NewSimpleFunction([]types.Type{types.Number, types.Number}, types.Undefined)).
		WithProperty("setInt16", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Boolean}, types.Undefined, []bool{false, false, true})).
		WithProperty("setUint16", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Boolean}, types.Undefined, []bool{false, false, true})).
		WithProperty("setInt32", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Boolean}, types.Undefined, []bool{false, false, true})).
		WithProperty("setUint32", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Boolean}, types.Undefined, []bool{false, false, true})).
		WithProperty("setFloat32", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Boolean}, types.Undefined, []bool{false, false, true})).
		WithProperty("setFloat64", types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Boolean}, types.Undefined, []bool{false, false, true})).
		WithProperty("setBigInt64", types.NewOptionalFunction([]types.Type{types.Number, types.BigInt, types.Boolean}, types.Undefined, []bool{false, false, true})).
		WithProperty("setBigUint64", types.NewOptionalFunction([]types.Type{types.Number, types.BigInt, types.Boolean}, types.Undefined, []bool{false, false, true}))

	// Create DataView constructor type
	dataViewCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Any, types.Number, types.Number}, dataViewProtoType).
		WithProperty("prototype", dataViewProtoType)

	return ctx.DefineGlobal("DataView", dataViewCtorType)
}

func (d *DataViewInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create DataView.prototype inheriting from Object.prototype
	dataViewProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Set @@toStringTag
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		falseVal := false
		trueVal := true
		dataViewProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("DataView"),
			&falseVal, // writable: false
			&falseVal, // enumerable: false
			&trueVal,  // configurable: true
		)
	}

	// Helper to validate DataView and get byte offset
	validateDataViewAccess := func(thisVal vm.Value, byteOffsetArg vm.Value, elementSize int) (*vm.DataViewObject, int, error) {
		dv := thisVal.AsDataView()
		if dv == nil {
			return nil, 0, vmInstance.NewTypeError("Method called on incompatible receiver")
		}

		// Check if buffer is detached
		if dv.GetBufferData().IsDetached() {
			return nil, 0, vmInstance.NewTypeError("Cannot perform operation on a detached ArrayBuffer")
		}

		// Get byte offset
		byteOffset := int(vmInstance.ToNumber(byteOffsetArg))
		if byteOffset < 0 {
			return nil, 0, vmInstance.NewRangeError("Offset is outside the bounds of the DataView")
		}

		// Check bounds
		if byteOffset+elementSize > dv.GetByteLength() {
			return nil, 0, vmInstance.NewRangeError("Offset is outside the bounds of the DataView")
		}

		return dv, byteOffset, nil
	}

	// buffer getter
	e := false
	c := true
	bufferGetter := vm.NewNativeFunction(0, false, "get buffer", func(args []vm.Value) (vm.Value, error) {
		dv := vmInstance.GetThis().AsDataView()
		if dv == nil {
			return vm.Undefined, vmInstance.NewTypeError("get DataView.prototype.buffer called on incompatible receiver")
		}
		if dv.IsSharedBuffer() {
			return vm.NewSharedArrayBufferFromObject(dv.GetSharedBuffer()), nil
		}
		return vm.NewArrayBufferFromObject(dv.GetBuffer()), nil
	})
	dataViewProto.DefineAccessorProperty("buffer", bufferGetter, true, vm.Undefined, false, &e, &c)

	// byteLength getter
	byteLengthGetter := vm.NewNativeFunction(0, false, "get byteLength", func(args []vm.Value) (vm.Value, error) {
		dv := vmInstance.GetThis().AsDataView()
		if dv == nil {
			return vm.Undefined, vmInstance.NewTypeError("get DataView.prototype.byteLength called on incompatible receiver")
		}
		if dv.GetBufferData().IsDetached() {
			return vm.Undefined, vmInstance.NewTypeError("Cannot get byteLength on a detached ArrayBuffer")
		}
		return vm.Number(float64(dv.GetByteLength())), nil
	})
	dataViewProto.DefineAccessorProperty("byteLength", byteLengthGetter, true, vm.Undefined, false, &e, &c)

	// byteOffset getter
	byteOffsetGetter := vm.NewNativeFunction(0, false, "get byteOffset", func(args []vm.Value) (vm.Value, error) {
		dv := vmInstance.GetThis().AsDataView()
		if dv == nil {
			return vm.Undefined, vmInstance.NewTypeError("get DataView.prototype.byteOffset called on incompatible receiver")
		}
		if dv.GetBufferData().IsDetached() {
			return vm.Undefined, vmInstance.NewTypeError("Cannot get byteOffset on a detached ArrayBuffer")
		}
		return vm.Number(float64(dv.GetByteOffset())), nil
	})
	dataViewProto.DefineAccessorProperty("byteOffset", byteOffsetGetter, true, vm.Undefined, false, &e, &c)

	// getInt8
	dataViewProto.SetOwnNonEnumerable("getInt8", vm.NewNativeFunction(1, false, "getInt8", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("getInt8 requires 1 argument")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 1)
		if err != nil {
			return vm.Undefined, err
		}
		val, _ := dv.GetInt8(byteOffset)
		return vm.Number(float64(val)), nil
	}))

	// getUint8
	dataViewProto.SetOwnNonEnumerable("getUint8", vm.NewNativeFunction(1, false, "getUint8", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("getUint8 requires 1 argument")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 1)
		if err != nil {
			return vm.Undefined, err
		}
		val, _ := dv.GetUint8(byteOffset)
		return vm.Number(float64(val)), nil
	}))

	// getInt16
	dataViewProto.SetOwnNonEnumerable("getInt16", vm.NewNativeFunction(1, false, "getInt16", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("getInt16 requires 1 argument")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 2)
		if err != nil {
			return vm.Undefined, err
		}
		littleEndian := len(args) > 1 && args[1].IsTruthy()
		val, _ := dv.GetInt16(byteOffset, littleEndian)
		return vm.Number(float64(val)), nil
	}))

	// getUint16
	dataViewProto.SetOwnNonEnumerable("getUint16", vm.NewNativeFunction(1, false, "getUint16", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("getUint16 requires 1 argument")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 2)
		if err != nil {
			return vm.Undefined, err
		}
		littleEndian := len(args) > 1 && args[1].IsTruthy()
		val, _ := dv.GetUint16(byteOffset, littleEndian)
		return vm.Number(float64(val)), nil
	}))

	// getInt32
	dataViewProto.SetOwnNonEnumerable("getInt32", vm.NewNativeFunction(1, false, "getInt32", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("getInt32 requires 1 argument")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 4)
		if err != nil {
			return vm.Undefined, err
		}
		littleEndian := len(args) > 1 && args[1].IsTruthy()
		val, _ := dv.GetInt32(byteOffset, littleEndian)
		return vm.Number(float64(val)), nil
	}))

	// getUint32
	dataViewProto.SetOwnNonEnumerable("getUint32", vm.NewNativeFunction(1, false, "getUint32", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("getUint32 requires 1 argument")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 4)
		if err != nil {
			return vm.Undefined, err
		}
		littleEndian := len(args) > 1 && args[1].IsTruthy()
		val, _ := dv.GetUint32(byteOffset, littleEndian)
		return vm.Number(float64(val)), nil
	}))

	// getFloat32
	dataViewProto.SetOwnNonEnumerable("getFloat32", vm.NewNativeFunction(1, false, "getFloat32", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("getFloat32 requires 1 argument")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 4)
		if err != nil {
			return vm.Undefined, err
		}
		littleEndian := len(args) > 1 && args[1].IsTruthy()
		val, _ := dv.GetFloat32(byteOffset, littleEndian)
		return vm.Number(float64(val)), nil
	}))

	// getFloat64
	dataViewProto.SetOwnNonEnumerable("getFloat64", vm.NewNativeFunction(1, false, "getFloat64", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("getFloat64 requires 1 argument")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 8)
		if err != nil {
			return vm.Undefined, err
		}
		littleEndian := len(args) > 1 && args[1].IsTruthy()
		val, _ := dv.GetFloat64(byteOffset, littleEndian)
		return vm.Number(val), nil
	}))

	// getBigInt64
	dataViewProto.SetOwnNonEnumerable("getBigInt64", vm.NewNativeFunction(1, false, "getBigInt64", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("getBigInt64 requires 1 argument")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 8)
		if err != nil {
			return vm.Undefined, err
		}
		littleEndian := len(args) > 1 && args[1].IsTruthy()
		val, _ := dv.GetBigInt64(byteOffset, littleEndian)
		return vm.NewBigInt(val), nil
	}))

	// getBigUint64
	dataViewProto.SetOwnNonEnumerable("getBigUint64", vm.NewNativeFunction(1, false, "getBigUint64", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("getBigUint64 requires 1 argument")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 8)
		if err != nil {
			return vm.Undefined, err
		}
		littleEndian := len(args) > 1 && args[1].IsTruthy()
		val, _ := dv.GetBigUint64(byteOffset, littleEndian)
		return vm.NewBigInt(val), nil
	}))

	// setInt8
	dataViewProto.SetOwnNonEnumerable("setInt8", vm.NewNativeFunction(2, false, "setInt8", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("setInt8 requires 2 arguments")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 1)
		if err != nil {
			return vm.Undefined, err
		}
		value := int8(vmInstance.ToNumber(args[1]))
		dv.SetInt8(byteOffset, value)
		return vm.Undefined, nil
	}))

	// setUint8
	dataViewProto.SetOwnNonEnumerable("setUint8", vm.NewNativeFunction(2, false, "setUint8", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("setUint8 requires 2 arguments")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 1)
		if err != nil {
			return vm.Undefined, err
		}
		value := uint8(vmInstance.ToNumber(args[1]))
		dv.SetUint8(byteOffset, value)
		return vm.Undefined, nil
	}))

	// setInt16
	dataViewProto.SetOwnNonEnumerable("setInt16", vm.NewNativeFunction(2, false, "setInt16", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("setInt16 requires 2 arguments")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 2)
		if err != nil {
			return vm.Undefined, err
		}
		value := int16(vmInstance.ToNumber(args[1]))
		littleEndian := len(args) > 2 && args[2].IsTruthy()
		dv.SetInt16(byteOffset, value, littleEndian)
		return vm.Undefined, nil
	}))

	// setUint16
	dataViewProto.SetOwnNonEnumerable("setUint16", vm.NewNativeFunction(2, false, "setUint16", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("setUint16 requires 2 arguments")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 2)
		if err != nil {
			return vm.Undefined, err
		}
		value := uint16(vmInstance.ToNumber(args[1]))
		littleEndian := len(args) > 2 && args[2].IsTruthy()
		dv.SetUint16(byteOffset, value, littleEndian)
		return vm.Undefined, nil
	}))

	// setInt32
	dataViewProto.SetOwnNonEnumerable("setInt32", vm.NewNativeFunction(2, false, "setInt32", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("setInt32 requires 2 arguments")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 4)
		if err != nil {
			return vm.Undefined, err
		}
		value := int32(vmInstance.ToNumber(args[1]))
		littleEndian := len(args) > 2 && args[2].IsTruthy()
		dv.SetInt32(byteOffset, value, littleEndian)
		return vm.Undefined, nil
	}))

	// setUint32
	dataViewProto.SetOwnNonEnumerable("setUint32", vm.NewNativeFunction(2, false, "setUint32", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("setUint32 requires 2 arguments")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 4)
		if err != nil {
			return vm.Undefined, err
		}
		value := uint32(vmInstance.ToNumber(args[1]))
		littleEndian := len(args) > 2 && args[2].IsTruthy()
		dv.SetUint32(byteOffset, value, littleEndian)
		return vm.Undefined, nil
	}))

	// setFloat32
	dataViewProto.SetOwnNonEnumerable("setFloat32", vm.NewNativeFunction(2, false, "setFloat32", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("setFloat32 requires 2 arguments")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 4)
		if err != nil {
			return vm.Undefined, err
		}
		value := float32(vmInstance.ToNumber(args[1]))
		littleEndian := len(args) > 2 && args[2].IsTruthy()
		dv.SetFloat32(byteOffset, value, littleEndian)
		return vm.Undefined, nil
	}))

	// setFloat64
	dataViewProto.SetOwnNonEnumerable("setFloat64", vm.NewNativeFunction(2, false, "setFloat64", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("setFloat64 requires 2 arguments")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 8)
		if err != nil {
			return vm.Undefined, err
		}
		value := vmInstance.ToNumber(args[1])
		littleEndian := len(args) > 2 && args[2].IsTruthy()
		dv.SetFloat64(byteOffset, value, littleEndian)
		return vm.Undefined, nil
	}))

	// setBigInt64
	dataViewProto.SetOwnNonEnumerable("setBigInt64", vm.NewNativeFunction(2, false, "setBigInt64", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("setBigInt64 requires 2 arguments")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 8)
		if err != nil {
			return vm.Undefined, err
		}
		var value *big.Int
		if args[1].IsBigInt() {
			value = args[1].AsBigInt()
		} else {
			value = big.NewInt(int64(vmInstance.ToNumber(args[1])))
		}
		littleEndian := len(args) > 2 && args[2].IsTruthy()
		dv.SetBigInt64(byteOffset, value, littleEndian)
		return vm.Undefined, nil
	}))

	// setBigUint64
	dataViewProto.SetOwnNonEnumerable("setBigUint64", vm.NewNativeFunction(2, false, "setBigUint64", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("setBigUint64 requires 2 arguments")
		}
		dv, byteOffset, err := validateDataViewAccess(vmInstance.GetThis(), args[0], 8)
		if err != nil {
			return vm.Undefined, err
		}
		var value *big.Int
		if args[1].IsBigInt() {
			value = args[1].AsBigInt()
		} else {
			value = big.NewInt(int64(vmInstance.ToNumber(args[1])))
		}
		littleEndian := len(args) > 2 && args[2].IsTruthy()
		dv.SetBigUint64(byteOffset, value, littleEndian)
		return vm.Undefined, nil
	}))

	// Create DataView constructor
	ctorWithProps := vm.NewConstructorWithProps(3, true, "DataView", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("DataView constructor requires an ArrayBuffer or SharedArrayBuffer")
		}

		// Get the buffer argument
		bufferArg := args[0]
		var buffer vm.BufferData
		var bufferByteLength int

		if ab := bufferArg.AsArrayBuffer(); ab != nil {
			if ab.IsDetached() {
				return vm.Undefined, vmInstance.NewTypeError("Cannot construct DataView on a detached ArrayBuffer")
			}
			buffer = ab
			bufferByteLength = len(ab.GetData())
		} else if sab := bufferArg.AsSharedArrayBuffer(); sab != nil {
			buffer = sab
			bufferByteLength = len(sab.GetData())
		} else {
			return vm.Undefined, vmInstance.NewTypeError("First argument to DataView must be an ArrayBuffer or SharedArrayBuffer")
		}

		// Parse byteOffset
		byteOffset := 0
		if len(args) > 1 && !args[1].IsUndefined() {
			offset := vmInstance.ToNumber(args[1])
			if math.IsNaN(offset) || math.IsInf(offset, 0) {
				return vm.Undefined, vmInstance.NewRangeError("Invalid DataView byteOffset")
			}
			byteOffset = int(offset)
			if byteOffset < 0 {
				return vm.Undefined, vmInstance.NewRangeError("Start offset is negative")
			}
			if byteOffset > bufferByteLength {
				return vm.Undefined, vmInstance.NewRangeError("Start offset is outside the bounds of the buffer")
			}
		}

		// Parse byteLength
		byteLength := bufferByteLength - byteOffset
		if len(args) > 2 && !args[2].IsUndefined() {
			length := vmInstance.ToNumber(args[2])
			if math.IsNaN(length) || math.IsInf(length, 0) {
				return vm.Undefined, vmInstance.NewRangeError("Invalid DataView byteLength")
			}
			byteLength = int(length)
			if byteLength < 0 {
				return vm.Undefined, vmInstance.NewRangeError("Invalid DataView byteLength")
			}
			if byteOffset+byteLength > bufferByteLength {
				return vm.Undefined, vmInstance.NewRangeError("Start offset plus length is outside the bounds of the buffer")
			}
		}

		return vm.NewDataView(buffer, byteOffset, byteLength), nil
	})

	// Add prototype property
	w := false
	ctorWithProps.AsNativeFunctionWithProps().Properties.DefineOwnProperty("prototype", vm.NewValueFromPlainObject(dataViewProto), &w, &e, &c)

	// Set constructor property on prototype
	dataViewProto.SetOwnNonEnumerable("constructor", ctorWithProps)

	// Set DataView prototype in VM
	vmInstance.DataViewPrototype = vm.NewValueFromPlainObject(dataViewProto)

	// Register DataView constructor as global
	return ctx.DefineGlobal("DataView", ctorWithProps)
}

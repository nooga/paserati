package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type SharedArrayBufferInitializer struct{}

func (s *SharedArrayBufferInitializer) Name() string {
	return "SharedArrayBuffer"
}

func (s *SharedArrayBufferInitializer) Priority() int {
	return 411 // After ArrayBuffer, before typed arrays
}

func (s *SharedArrayBufferInitializer) InitTypes(ctx *TypeContext) error {
	// Create SharedArrayBuffer.prototype type
	sharedArrayBufferProtoType := types.NewObjectType().
		WithProperty("byteLength", types.Number).
		WithProperty("maxByteLength", types.Number).
		WithProperty("growable", types.Boolean).
		WithProperty("grow", types.NewSimpleFunction([]types.Type{types.Number}, types.Undefined)).
		WithProperty("slice", types.NewSimpleFunction([]types.Type{types.Number, types.Number}, types.Any)) // Returns new SharedArrayBuffer

	// Create SharedArrayBuffer constructor type: new SharedArrayBuffer(length, options?)
	ctorSig := &types.Signature{
		ParameterTypes: []types.Type{types.Number, types.Any},
		ReturnType:     sharedArrayBufferProtoType,
		OptionalParams: []bool{false, true},
	}
	sharedArrayBufferCtorType := types.NewObjectType().
		WithCallSignature(ctorSig).
		WithProperty("prototype", sharedArrayBufferProtoType)

	return ctx.DefineGlobal("SharedArrayBuffer", sharedArrayBufferCtorType)
}

func (s *SharedArrayBufferInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create SharedArrayBuffer.prototype inheriting from Object.prototype
	sharedArrayBufferProto := vm.NewObject(objectProto).AsPlainObject()

	// Add byteLength getter
	// Per spec, byteLength is an accessor property
	byteLengthGetter := vm.NewNativeFunction(0, false, "get byteLength", func(args []vm.Value) (vm.Value, error) {
		thisBuffer := vmInstance.GetThis()
		buffer := thisBuffer.AsSharedArrayBuffer()
		if buffer == nil {
			return vm.Undefined, vmInstance.NewTypeError("SharedArrayBuffer.prototype.byteLength called on incompatible receiver")
		}
		return vm.Number(float64(buffer.ByteLength())), nil
	})
	e := false
	c := false
	sharedArrayBufferProto.DefineAccessorProperty("byteLength", byteLengthGetter, true, vm.Undefined, false, &e, &c)

	// maxByteLength/growable getters (ES2024 growable SharedArrayBuffer).
	// These exist mainly for Object.getOwnPropertyDescriptor/reflection -
	// ordinary property reads are already served by the handleSpecialProperties
	// fast path in pkg/vm/property_helpers.go.
	accEnum, accConf := false, true
	maxByteLengthGetter := vm.NewNativeFunction(0, false, "get maxByteLength", func(args []vm.Value) (vm.Value, error) {
		thisBuffer := vmInstance.GetThis()
		buffer := thisBuffer.AsSharedArrayBuffer()
		if buffer == nil {
			return vm.Undefined, vmInstance.NewTypeError("SharedArrayBuffer.prototype.maxByteLength called on incompatible receiver")
		}
		if buffer.IsGrowable() {
			return vm.Number(float64(buffer.MaxByteLength())), nil
		}
		return vm.Number(float64(buffer.ByteLength())), nil
	})
	sharedArrayBufferProto.DefineAccessorProperty("maxByteLength", maxByteLengthGetter, true, vm.Undefined, false, &accEnum, &accConf)

	growableGetter := vm.NewNativeFunction(0, false, "get growable", func(args []vm.Value) (vm.Value, error) {
		thisBuffer := vmInstance.GetThis()
		buffer := thisBuffer.AsSharedArrayBuffer()
		if buffer == nil {
			return vm.Undefined, vmInstance.NewTypeError("SharedArrayBuffer.prototype.growable called on incompatible receiver")
		}
		return vm.BooleanValue(buffer.IsGrowable()), nil
	})
	sharedArrayBufferProto.DefineAccessorProperty("growable", growableGetter, true, vm.Undefined, false, &accEnum, &accConf)

	// Add slice method
	sharedArrayBufferProto.SetOwnNonEnumerable("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) (vm.Value, error) {
		thisBuffer := vmInstance.GetThis()
		buffer := thisBuffer.AsSharedArrayBuffer()
		if buffer == nil {
			return vm.Undefined, vmInstance.NewTypeError("SharedArrayBuffer.prototype.slice called on incompatible receiver")
		}

		data := buffer.GetData()
		length := len(data)

		// Parse start argument
		start := 0
		if len(args) > 0 && !args[0].IsUndefined() {
			start = int(args[0].ToFloat())
			if start < 0 {
				start = length + start
			}
			if start < 0 {
				start = 0
			}
			if start > length {
				start = length
			}
		}

		// Parse end argument
		end := length
		if len(args) > 1 && !args[1].IsUndefined() {
			end = int(args[1].ToFloat())
			if end < 0 {
				end = length + end
			}
			if end < 0 {
				end = 0
			}
			if end > length {
				end = length
			}
		}

		// Ensure start <= end
		if start > end {
			start = end
		}

		// Create new SharedArrayBuffer with sliced data
		sliceLength := end - start
		newBuffer := vm.NewSharedArrayBuffer(sliceLength)
		if newBufferObj := newBuffer.AsSharedArrayBuffer(); newBufferObj != nil {
			copy(newBufferObj.GetData(), data[start:end])
		}

		return newBuffer, nil
	}))

	// grow(newLength) - ES2024 growable SharedArrayBuffer
	sharedArrayBufferProto.SetOwnNonEnumerable("grow", vm.NewNativeFunction(1, false, "grow", func(args []vm.Value) (vm.Value, error) {
		thisBuffer := vmInstance.GetThis()
		buffer := thisBuffer.AsSharedArrayBuffer()
		if buffer == nil {
			return vm.Undefined, vmInstance.NewTypeError("SharedArrayBuffer.prototype.grow called on incompatible receiver")
		}
		if !buffer.IsGrowable() {
			return vm.Undefined, vmInstance.NewTypeError("SharedArrayBuffer.prototype.grow called on a non-growable SharedArrayBuffer")
		}
		newLen := 0
		if len(args) > 0 {
			var lenErr error
			newLen, lenErr = validatedArrayBufferLength(vmInstance, args[0], "Invalid array buffer length")
			if lenErr != nil {
				return vm.Undefined, lenErr
			}
		}
		if err := buffer.Grow(newLen); err != nil {
			return vm.Undefined, vmInstance.NewRangeError(err.Error())
		}
		return vm.Undefined, nil
	}))

	// Add @@toStringTag
	sharedArrayBufferProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolToStringTag), vm.NewString("SharedArrayBuffer"), nil, nil, nil)

	// Create SharedArrayBuffer constructor
	// Per spec, SharedArrayBuffer constructor length is 1
	ctorWithProps := vm.NewConstructorWithProps(1, true, "SharedArrayBuffer", func(args []vm.Value) (vm.Value, error) {
		// SharedArrayBuffer cannot be called without new
		// The NewConstructorWithProps handles this for us

		// Per ECMAScript spec: OrdinaryCreateFromConstructor(NewTarget, "%SharedArrayBuffer.prototype%")
		if newTarget := vmInstance.GetNewTarget(); !newTarget.IsUndefined() {
			_, gpfcErr := vmInstance.GetPrototypeFromConstructor(newTarget, "%ObjectPrototype%")
			if gpfcErr != nil {
				return vm.Undefined, gpfcErr
			}
		}

		if len(args) == 0 {
			return vm.NewSharedArrayBuffer(0), nil
		}

		size, err := validatedArrayBufferLength(vmInstance, args[0], "Invalid shared array buffer length")
		if err != nil {
			return vm.Undefined, err
		}

		maxByteLength := -1
		if len(args) > 1 {
			var err error
			maxByteLength, err = arrayBufferMaxByteLengthOption(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
		}
		if maxByteLength >= 0 {
			if size > maxByteLength {
				return vm.Undefined, vmInstance.NewRangeError("Invalid array buffer length: maxByteLength must not be smaller than the initial byteLength")
			}
			return vm.NewGrowableSharedArrayBuffer(size, maxByteLength), nil
		}

		return vm.NewSharedArrayBuffer(size), nil
	})

	// Add prototype property (non-writable, non-enumerable, non-configurable)
	w := false
	ctorWithProps.AsNativeFunctionWithProps().Properties.DefineOwnProperty("prototype", vm.NewValueFromPlainObject(sharedArrayBufferProto), &w, &e, &c)

	// Set constructor property on prototype
	sharedArrayBufferProto.SetOwnNonEnumerable("constructor", ctorWithProps)

	// Set SharedArrayBuffer prototype in VM for proper prototype chain lookups
	vmInstance.SharedArrayBufferPrototype = vm.NewValueFromPlainObject(sharedArrayBufferProto)

	// Register SharedArrayBuffer constructor as global
	return ctx.DefineGlobal("SharedArrayBuffer", ctorWithProps)
}

package builtins

import (
	"fmt"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type ArrayBufferInitializer struct{}

func (a *ArrayBufferInitializer) Name() string {
	return "ArrayBuffer"
}

func (a *ArrayBufferInitializer) Priority() int {
	return 410 // After basic types, before typed arrays
}

func (a *ArrayBufferInitializer) InitTypes(ctx *TypeContext) error {
	// Create ArrayBuffer.prototype type
	arrayBufferProtoType := types.NewObjectType().
		WithProperty("byteLength", types.Number).
		WithProperty("slice", types.NewSimpleFunction([]types.Type{types.Number, types.Number}, types.Any)) // Returns new ArrayBuffer

	// Create ArrayBuffer constructor type
	arrayBufferCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, arrayBufferProtoType). // ArrayBuffer(length) -> ArrayBuffer
		WithProperty("isView", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("prototype", arrayBufferProtoType)

	return ctx.DefineGlobal("ArrayBuffer", arrayBufferCtorType)
}

func (a *ArrayBufferInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create ArrayBuffer.prototype inheriting from Object.prototype
	arrayBufferProto := vm.NewObject(objectProto).AsPlainObject()

	// Add ArrayBuffer prototype properties and methods
	arrayBufferProto.SetOwn("byteLength", vm.NewNativeFunction(0, false, "get byteLength", func(args []vm.Value) (vm.Value, error) {
		thisBuffer := vmInstance.GetThis()
		if buffer := thisBuffer.AsArrayBuffer(); buffer != nil {
			return vm.Number(float64(len(buffer.GetData()))), nil
		}
		return vm.Undefined, nil
	}))

	arrayBufferProto.SetOwn("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) (vm.Value, error) {
		thisBuffer := vmInstance.GetThis()
		buffer := thisBuffer.AsArrayBuffer()
		if buffer == nil {
			return vm.Undefined, nil
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

		// Create new ArrayBuffer with sliced data
		sliceLength := end - start
		newBuffer := vm.NewArrayBuffer(sliceLength)
		if newBufferObj := newBuffer.AsArrayBuffer(); newBufferObj != nil {
			copy(newBufferObj.GetData(), data[start:end])
		}

		return newBuffer, nil
	}))

	// Create ArrayBuffer constructor
	ctorWithProps := vm.NewNativeFunctionWithProps(1, true, "ArrayBuffer", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewArrayBuffer(0), nil
		}

		size := int(args[0].ToFloat())
		if size < 0 {
			// Proper error handling instead of returning Undefined
			return vm.Undefined, fmt.Errorf("Invalid ArrayBuffer length")
		}

		return vm.NewArrayBuffer(size), nil
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("prototype", vm.NewValueFromPlainObject(arrayBufferProto))

	// Add static methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("isView", vm.NewNativeFunction(1, false, "isView", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.BooleanValue(false), nil
		}

		// Check if argument is a TypedArray or DataView
		arg := args[0]
		return vm.BooleanValue(arg.Type() == vm.TypeTypedArray), nil
	}))

	// Set constructor property on prototype
	arrayBufferProto.SetOwn("constructor", ctorWithProps)

	// Register ArrayBuffer constructor as global
	return ctx.DefineGlobal("ArrayBuffer", ctorWithProps)
}
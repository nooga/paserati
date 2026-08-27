package builtins

import (
	"math"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
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
		WithProperty("maxByteLength", types.Number).
		WithProperty("resizable", types.Boolean).
		WithProperty("resize", types.NewSimpleFunction([]types.Type{types.Number}, types.Undefined)).
		WithProperty("transfer", types.NewOptionalFunction([]types.Type{types.Number}, types.Any, []bool{true})).
		WithProperty("transferToFixedLength", types.NewOptionalFunction([]types.Type{types.Number}, types.Any, []bool{true})).
		WithProperty("slice", types.NewSimpleFunction([]types.Type{types.Number, types.Number}, types.Any)) // Returns new ArrayBuffer

	// Create ArrayBuffer constructor type: new ArrayBuffer(length, options?)
	ctorSig := &types.Signature{
		ParameterTypes: []types.Type{types.Number, types.Any},
		ReturnType:     arrayBufferProtoType,
		OptionalParams: []bool{false, true},
	}
	arrayBufferCtorType := types.NewObjectType().
		WithCallSignature(ctorSig).
		WithProperty("isView", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)).
		WithProperty("prototype", arrayBufferProtoType)

	return ctx.DefineGlobal("ArrayBuffer", arrayBufferCtorType)
}

// maxArrayBufferByteLength is an implementation-defined cap on a resizable/
// growable buffer's maxByteLength (and on ArrayBuffer.prototype.transfer's
// requested length). Resizable/growable buffers preallocate capacity up to
// maxByteLength immediately (see NewResizableArrayBuffer/
// NewGrowableSharedArrayBuffer), so without a cap well below ToIndex's
// 2^53-1 ceiling, a script-supplied value would make() an enormous slice and
// crash the process instead of raising the RangeError the spec allows
// ("if it is not possible to create a Data Block of size byteLength, throw a
// RangeError"). 1<<32 (4 GiB) is generous for any real use and still cheap
// to reject.
const maxArrayBufferByteLength = 1 << 32

// arrayBufferMaxByteLengthOption reads the { maxByteLength } option from a
// constructor options bag argument, per ECMAScript GetArrayBufferMaxByteLengthOption.
// Returns (-1, nil) if the option is absent/undefined (buffer is not resizable).
func arrayBufferMaxByteLengthOption(vmInstance *vm.VM, optionsArg vm.Value) (int, error) {
	if !optionsArg.IsObject() {
		return -1, nil
	}
	maxVal, err := vmInstance.GetProperty(optionsArg, "maxByteLength")
	if err != nil {
		return -1, err
	}
	if maxVal.IsUndefined() {
		return -1, nil
	}
	return validatedArrayBufferLength(vmInstance, maxVal, "Invalid maxByteLength option")
}

// validatedArrayBufferLength implements enough of ToIndex to safely reject a
// script-supplied byte length before it reaches a make([]byte, ...) call:
// it must be a non-negative integer no greater than 2^53-1 (ToIndex's
// ceiling), and additionally no greater than maxArrayBufferByteLength (our
// implementation limit, since preallocating up to a ToIndex-legal but
// multi-petabyte value would crash the process rather than raise a
// RangeError).
func validatedArrayBufferLength(vmInstance *vm.VM, val vm.Value, errMsg string) (int, error) {
	f := val.ToFloat()
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f > (1<<53-1) {
		return -1, vmInstance.NewRangeError(errMsg)
	}
	if f > maxArrayBufferByteLength {
		return -1, vmInstance.NewRangeError(errMsg + ": exceeds implementation limit")
	}
	return int(f), nil
}

func (a *ArrayBufferInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create ArrayBuffer.prototype inheriting from Object.prototype
	arrayBufferProto := vm.NewObject(objectProto).AsPlainObject()

	// Add ArrayBuffer prototype properties and methods
	arrayBufferProto.SetOwnNonEnumerable("byteLength", vm.NewNativeFunction(0, false, "get byteLength", func(args []vm.Value) (vm.Value, error) {
		thisBuffer := vmInstance.GetThis()
		buffer := thisBuffer.AsArrayBuffer()
		if buffer == nil {
			return vm.Undefined, vmInstance.NewTypeError("ArrayBuffer.prototype.byteLength called on incompatible receiver")
		}
		// Return 0 for detached buffers per spec
		if buffer.IsDetached() {
			return vm.Number(0), nil
		}
		return vm.Number(float64(len(buffer.GetData()))), nil
	}))

	// maxByteLength/resizable getters (ES2024 resizable ArrayBuffer). These
	// exist mainly for Object.getOwnPropertyDescriptor/reflection - ordinary
	// property reads are already served by the handleSpecialProperties fast
	// path in pkg/vm/property_helpers.go.
	accEnum, accConf := false, true
	maxByteLengthGetter := vm.NewNativeFunction(0, false, "get maxByteLength", func(args []vm.Value) (vm.Value, error) {
		thisBuffer := vmInstance.GetThis()
		buffer := thisBuffer.AsArrayBuffer()
		if buffer == nil {
			return vm.Undefined, vmInstance.NewTypeError("ArrayBuffer.prototype.maxByteLength called on incompatible receiver")
		}
		if buffer.IsDetached() {
			return vm.Number(0), nil
		}
		if buffer.IsResizable() {
			return vm.Number(float64(buffer.MaxByteLength())), nil
		}
		return vm.Number(float64(len(buffer.GetData()))), nil
	})
	arrayBufferProto.DefineAccessorProperty("maxByteLength", maxByteLengthGetter, true, vm.Undefined, false, &accEnum, &accConf)

	resizableGetter := vm.NewNativeFunction(0, false, "get resizable", func(args []vm.Value) (vm.Value, error) {
		thisBuffer := vmInstance.GetThis()
		buffer := thisBuffer.AsArrayBuffer()
		if buffer == nil {
			return vm.Undefined, vmInstance.NewTypeError("ArrayBuffer.prototype.resizable called on incompatible receiver")
		}
		return vm.BooleanValue(buffer.IsResizable()), nil
	})
	arrayBufferProto.DefineAccessorProperty("resizable", resizableGetter, true, vm.Undefined, false, &accEnum, &accConf)

	arrayBufferProto.SetOwnNonEnumerable("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) (vm.Value, error) {
		thisBuffer := vmInstance.GetThis()
		buffer := thisBuffer.AsArrayBuffer()
		if buffer == nil {
			return vm.Undefined, vmInstance.NewTypeError("ArrayBuffer.prototype.slice called on incompatible receiver")
		}

		// Throw TypeError if buffer is detached
		if buffer.IsDetached() {
			return vm.Undefined, vmInstance.NewTypeError("Cannot perform slice on a detached ArrayBuffer")
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

		sliceLength := end - start

		// SpeciesConstructor algorithm (ECMAScript 7.3.20)
		// Step 1-2: Get constructor property
		var ctor vm.Value
		if buffer.HasOwnProperty("constructor") {
			ctor, _ = buffer.GetOwnProperty("constructor")
		} else {
			// Check prototype chain for constructor
			ctor, _ = vmInstance.GetProperty(thisBuffer, "constructor")
		}

		// Step 4: If C is undefined, use default constructor
		if ctor.IsUndefined() {
			// Use default ArrayBuffer constructor
			newBuffer := vm.NewArrayBuffer(sliceLength)
			if newBufferObj := newBuffer.AsArrayBuffer(); newBufferObj != nil {
				copy(newBufferObj.GetData(), data[start:end])
			}
			return newBuffer, nil
		}

		// Step 5: If Type(C) is not Object, throw TypeError
		if !ctor.IsObject() && !ctor.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("ArrayBuffer.prototype.slice: constructor property is not an object")
		}

		// Step 6: Get [Symbol.species] from constructor
		var species vm.Value
		if ctor.IsObject() {
			if ctor.Type() == vm.TypeObject {
				po := ctor.AsPlainObject()
				species, _ = po.GetOwnByKey(vm.NewSymbolKey(vmInstance.SymbolSpecies))
			}
		}
		if species.Type() == 0 || species.IsUndefined() {
			// Try to get Symbol.species via GetProperty which handles symbol lookup
			species, _ = vmInstance.GetProperty(ctor, string(vmInstance.SymbolSpecies.AsSymbol()))
		}

		// Step 7: If species is null or undefined, use default constructor
		if species.Type() == 0 || species.IsUndefined() || species.Type() == vm.TypeNull {
			newBuffer := vm.NewArrayBuffer(sliceLength)
			if newBufferObj := newBuffer.AsArrayBuffer(); newBufferObj != nil {
				copy(newBufferObj.GetData(), data[start:end])
			}
			return newBuffer, nil
		}

		// Step 8: If species is not a constructor, throw TypeError
		if !vmInstance.IsConstructor(species) {
			return vm.Undefined, vmInstance.NewTypeError("ArrayBuffer.prototype.slice: @@species is not a constructor")
		}

		// Step 9: Call the species constructor with sliceLength
		newBuffer, err := vmInstance.Construct(species, []vm.Value{vm.Number(float64(sliceLength))})
		if err != nil {
			return vm.Undefined, err
		}

		// Validate that the result is an ArrayBuffer
		newBufferObj := newBuffer.AsArrayBuffer()
		if newBufferObj == nil {
			return vm.Undefined, vmInstance.NewTypeError("ArrayBuffer.prototype.slice: species constructor did not return an ArrayBuffer")
		}

		// Check that we didn't get the same buffer back
		if newBufferObj == buffer {
			return vm.Undefined, vmInstance.NewTypeError("ArrayBuffer.prototype.slice: species constructor returned same buffer")
		}

		// Check that the new buffer is big enough
		if len(newBufferObj.GetData()) < sliceLength {
			return vm.Undefined, vmInstance.NewTypeError("ArrayBuffer.prototype.slice: species constructor returned a buffer that is too small")
		}

		// Copy the data
		copy(newBufferObj.GetData(), data[start:end])

		return newBuffer, nil
	}))

	// resize(newLength) - ES2024 resizable ArrayBuffer
	arrayBufferProto.SetOwnNonEnumerable("resize", vm.NewNativeFunction(1, false, "resize", func(args []vm.Value) (vm.Value, error) {
		thisBuffer := vmInstance.GetThis()
		buffer := thisBuffer.AsArrayBuffer()
		if buffer == nil {
			return vm.Undefined, vmInstance.NewTypeError("ArrayBuffer.prototype.resize called on incompatible receiver")
		}
		if !buffer.IsResizable() {
			return vm.Undefined, vmInstance.NewTypeError("ArrayBuffer.prototype.resize called on a non-resizable ArrayBuffer")
		}
		newLen := 0
		if len(args) > 0 {
			var lenErr error
			newLen, lenErr = validatedArrayBufferLength(vmInstance, args[0], "Invalid array buffer length")
			if lenErr != nil {
				return vm.Undefined, lenErr
			}
		}
		if err := buffer.Resize(newLen); err != nil {
			if newLen > buffer.MaxByteLength() || newLen < 0 {
				return vm.Undefined, vmInstance.NewRangeError(err.Error())
			}
			return vm.Undefined, vmInstance.NewTypeError(err.Error())
		}
		return vm.Undefined, nil
	}))

	// Shared implementation for transfer/transferToFixedLength.
	transferImpl := func(preserveResizability bool) func(args []vm.Value) (vm.Value, error) {
		return func(args []vm.Value) (vm.Value, error) {
			thisBuffer := vmInstance.GetThis()
			buffer := thisBuffer.AsArrayBuffer()
			if buffer == nil {
				return vm.Undefined, vmInstance.NewTypeError("ArrayBuffer.prototype.transfer called on incompatible receiver")
			}
			newLen := -1
			if len(args) > 0 && !args[0].IsUndefined() {
				var lenErr error
				newLen, lenErr = validatedArrayBufferLength(vmInstance, args[0], "Invalid array buffer length")
				if lenErr != nil {
					return vm.Undefined, lenErr
				}
			}
			newBuffer, err := buffer.Transfer(newLen, preserveResizability)
			if err != nil {
				if buffer.IsDetached() {
					return vm.Undefined, vmInstance.NewTypeError("Cannot transfer a detached ArrayBuffer")
				}
				return vm.Undefined, vmInstance.NewRangeError(err.Error())
			}
			return vm.NewArrayBufferFromObject(newBuffer), nil
		}
	}
	arrayBufferProto.SetOwnNonEnumerable("transfer", vm.NewNativeFunction(0, false, "transfer", transferImpl(true)))
	arrayBufferProto.SetOwnNonEnumerable("transferToFixedLength", vm.NewNativeFunction(0, false, "transferToFixedLength", transferImpl(false)))

	// Create ArrayBuffer constructor
	ctorWithProps := vm.NewConstructorWithProps(1, true, "ArrayBuffer", func(args []vm.Value) (vm.Value, error) {
		// Per ECMAScript spec: OrdinaryCreateFromConstructor(NewTarget, "%ArrayBuffer.prototype%")
		if newTarget := vmInstance.GetNewTarget(); !newTarget.IsUndefined() {
			_, gpfcErr := vmInstance.GetPrototypeFromConstructor(newTarget, "%ObjectPrototype%")
			if gpfcErr != nil {
				return vm.Undefined, gpfcErr
			}
		}

		if len(args) == 0 {
			return vm.NewArrayBuffer(0), nil
		}

		size, err := validatedArrayBufferLength(vmInstance, args[0], "Invalid array buffer length")
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
			return vm.NewResizableArrayBuffer(size, maxByteLength), nil
		}

		return vm.NewArrayBuffer(size), nil
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(arrayBufferProto))

	// Add static methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("isView", vm.NewNativeFunction(1, false, "isView", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.BooleanValue(false), nil
		}

		// Check if argument is a TypedArray or DataView
		arg := args[0]
		return vm.BooleanValue(arg.Type() == vm.TypeTypedArray), nil
	}))

	// Set constructor property on prototype
	arrayBufferProto.SetOwnNonEnumerable("constructor", ctorWithProps)

	// Add ArrayBuffer.prototype[@@toStringTag] = "ArrayBuffer" (writable: false, enumerable: false, configurable: true)
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		wFalse, eFalse, cTrue := false, false, true
		arrayBufferProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("ArrayBuffer"),
			&wFalse, &eFalse, &cTrue,
		)
	}

	// Set ArrayBuffer prototype in VM for proper prototype chain lookups
	vmInstance.ArrayBufferPrototype = vm.NewValueFromPlainObject(arrayBufferProto)

	// Register ArrayBuffer constructor as global
	return ctx.DefineGlobal("ArrayBuffer", ctorWithProps)
}

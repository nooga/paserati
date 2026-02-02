package builtins

import (
	"encoding/base64"
	"encoding/hex"
	"strings"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type Uint8ArrayInitializer struct{}

func (u *Uint8ArrayInitializer) Name() string {
	return "Uint8Array"
}

func (u *Uint8ArrayInitializer) Priority() int {
	return 420 // After ArrayBuffer
}

func (u *Uint8ArrayInitializer) InitTypes(ctx *TypeContext) error {
	// Create Uint8Array.prototype type
	uint8ArrayProtoType := types.NewObjectType().
		WithProperty("buffer", types.Any). // Reference to underlying ArrayBuffer
		WithProperty("byteLength", types.Number).
		WithProperty("byteOffset", types.Number).
		WithProperty("length", types.Number).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("set", types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Undefined)).
		WithProperty("subarray", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true})).
		WithProperty("slice", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true}))

	// Create Uint8Array constructor type with multiple overloads
	uint8ArrayCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, uint8ArrayProtoType).                                // Uint8Array(length)
		WithSimpleCallSignature([]types.Type{types.Any}, uint8ArrayProtoType).                                   // Uint8Array(buffer, byteOffset?, length?)
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.Number}}, uint8ArrayProtoType). // Uint8Array(array)
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, uint8ArrayProtoType)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, uint8ArrayProtoType)).
		WithProperty("prototype", uint8ArrayProtoType)

	return ctx.DefineGlobal("Uint8Array", uint8ArrayCtorType)
}

func (u *Uint8ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create Uint8Array.prototype inheriting from TypedArray.prototype
	uint8ArrayProto := vm.NewObject(vmInstance.TypedArrayPrototype).AsPlainObject()

	// Set up prototype properties with correct descriptors (BYTES_PER_ELEMENT, buffer, byteLength, byteOffset, length)
	SetupTypedArrayPrototypeProperties(uint8ArrayProto, vmInstance, 1)
	// Note: set, fill, subarray, slice, and Symbol.toStringTag are inherited from %TypedArray%.prototype

	// ES2024: Uint8Array.prototype.toBase64(options?)
	uint8ArrayProto.SetOwnNonEnumerable("toBase64", vm.NewNativeFunction(0, false, "toBase64", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		ta := thisVal.AsTypedArray()
		if ta == nil || ta.GetElementType() != vm.TypedArrayUint8 {
			return vm.Undefined, vmInstance.NewTypeError("toBase64 called on non-Uint8Array")
		}

		// Get options FIRST (spec requires side effects before detached check)
		alphabet := "base64" // default
		omitPadding := false
		if len(args) > 0 && args[0].IsObject() {
			opts := args[0]
			if alphaVal, err := vmInstance.GetProperty(opts, "alphabet"); err == nil && alphaVal.Type() == vm.TypeString {
				alphabet = alphaVal.AsString()
				if alphabet != "base64" && alphabet != "base64url" {
					return vm.Undefined, vmInstance.NewTypeError("Invalid alphabet: must be 'base64' or 'base64url'")
				}
			}
			if omitVal, err := vmInstance.GetProperty(opts, "omitPadding"); err == nil {
				omitPadding = omitVal.IsTruthy()
			}
		}

		// Check for detached buffer AFTER reading options
		if buf := ta.GetBuffer(); buf != nil && buf.IsDetached() {
			return vm.Undefined, vmInstance.NewTypeError("Cannot perform toBase64 on a detached ArrayBuffer")
		}

		// Get bytes from typed array
		data := make([]byte, ta.GetLength())
		for i := 0; i < ta.GetLength(); i++ {
			data[i] = byte(ta.GetElement(i).ToFloat())
		}

		// Encode
		var encoded string
		if alphabet == "base64url" {
			encoded = base64.URLEncoding.EncodeToString(data)
		} else {
			encoded = base64.StdEncoding.EncodeToString(data)
		}

		if omitPadding {
			encoded = strings.TrimRight(encoded, "=")
		}

		return vm.NewString(encoded), nil
	}))

	// ES2024: Uint8Array.prototype.toHex()
	uint8ArrayProto.SetOwnNonEnumerable("toHex", vm.NewNativeFunction(0, false, "toHex", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		ta := thisVal.AsTypedArray()
		if ta == nil || ta.GetElementType() != vm.TypedArrayUint8 {
			return vm.Undefined, vmInstance.NewTypeError("toHex called on non-Uint8Array")
		}

		// Check for detached buffer
		if buf := ta.GetBuffer(); buf != nil && buf.IsDetached() {
			return vm.Undefined, vmInstance.NewTypeError("Cannot perform toHex on a detached ArrayBuffer")
		}

		// Get bytes from typed array
		data := make([]byte, ta.GetLength())
		for i := 0; i < ta.GetLength(); i++ {
			data[i] = byte(ta.GetElement(i).ToFloat())
		}

		return vm.NewString(hex.EncodeToString(data)), nil
	}))

	// ES2024: Uint8Array.prototype.setFromBase64(string, options?)
	uint8ArrayProto.SetOwnNonEnumerable("setFromBase64", vm.NewNativeFunction(1, false, "setFromBase64", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		ta := thisVal.AsTypedArray()
		if ta == nil || ta.GetElementType() != vm.TypedArrayUint8 {
			return vm.Undefined, vmInstance.NewTypeError("setFromBase64 called on non-Uint8Array")
		}

		if len(args) < 1 || args[0].Type() != vm.TypeString {
			return vm.Undefined, vmInstance.NewTypeError("setFromBase64 requires a string argument")
		}

		input := args[0].AsString()

		// Get options FIRST (spec requires side effects before detached check)
		alphabet := "base64"
		lastChunkHandling := "loose"
		if len(args) > 1 && args[1].IsObject() {
			opts := args[1]
			if alphaVal, err := vmInstance.GetProperty(opts, "alphabet"); err == nil && alphaVal.Type() == vm.TypeString {
				alphabet = alphaVal.AsString()
				if alphabet != "base64" && alphabet != "base64url" {
					return vm.Undefined, vmInstance.NewTypeError("Invalid alphabet: must be 'base64' or 'base64url'")
				}
			}
			if lchVal, err := vmInstance.GetProperty(opts, "lastChunkHandling"); err == nil && lchVal.Type() == vm.TypeString {
				lastChunkHandling = lchVal.AsString()
				if lastChunkHandling != "loose" && lastChunkHandling != "strict" && lastChunkHandling != "stop-before-partial" {
					return vm.Undefined, vmInstance.NewTypeError("Invalid lastChunkHandling")
				}
			}
		}

		// Check for detached buffer AFTER reading options
		if buf := ta.GetBuffer(); buf != nil && buf.IsDetached() {
			return vm.Undefined, vmInstance.NewTypeError("Cannot perform setFromBase64 on a detached ArrayBuffer")
		}

		// Remove whitespace
		input = strings.Map(func(r rune) rune {
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' {
				return -1
			}
			return r
		}, input)

		// Decode
		var decoded []byte
		var err error
		if alphabet == "base64url" {
			// Add padding if needed for strict decoding
			if lastChunkHandling != "stop-before-partial" {
				for len(input)%4 != 0 {
					input += "="
				}
			}
			decoded, err = base64.URLEncoding.DecodeString(input)
		} else {
			if lastChunkHandling != "stop-before-partial" {
				for len(input)%4 != 0 {
					input += "="
				}
			}
			decoded, err = base64.StdEncoding.DecodeString(input)
		}

		if err != nil {
			return vm.Undefined, vmInstance.NewSyntaxError("Invalid base64 string: " + err.Error())
		}

		// Write to typed array
		written := 0
		for i := 0; i < len(decoded) && i < ta.GetLength(); i++ {
			ta.SetElement(i, vm.NumberValue(float64(decoded[i])))
			written++
		}

		// Return result object { read, written }
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		result.SetOwn("read", vm.IntegerValue(int32(len(input))))
		result.SetOwn("written", vm.IntegerValue(int32(written)))
		return vm.NewValueFromPlainObject(result), nil
	}))

	// ES2024: Uint8Array.prototype.setFromHex(string)
	uint8ArrayProto.SetOwnNonEnumerable("setFromHex", vm.NewNativeFunction(1, false, "setFromHex", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		ta := thisVal.AsTypedArray()
		if ta == nil || ta.GetElementType() != vm.TypedArrayUint8 {
			return vm.Undefined, vmInstance.NewTypeError("setFromHex called on non-Uint8Array")
		}

		// Check for detached buffer
		if buf := ta.GetBuffer(); buf != nil && buf.IsDetached() {
			return vm.Undefined, vmInstance.NewTypeError("Cannot perform setFromHex on a detached ArrayBuffer")
		}

		if len(args) < 1 || args[0].Type() != vm.TypeString {
			return vm.Undefined, vmInstance.NewTypeError("setFromHex requires a string argument")
		}

		input := args[0].AsString()

		// Validate hex characters
		for _, c := range input {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return vm.Undefined, vmInstance.NewSyntaxError("Invalid hex character")
			}
		}

		// Decode
		decoded, err := hex.DecodeString(input)
		if err != nil {
			return vm.Undefined, vmInstance.NewSyntaxError("Invalid hex string: " + err.Error())
		}

		// Write to typed array
		written := 0
		for i := 0; i < len(decoded) && i < ta.GetLength(); i++ {
			ta.SetElement(i, vm.NumberValue(float64(decoded[i])))
			written++
		}

		// Return result object { read, written }
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		result.SetOwn("read", vm.IntegerValue(int32(len(input))))
		result.SetOwn("written", vm.IntegerValue(int32(written)))
		return vm.NewValueFromPlainObject(result), nil
	}))

	// Create Uint8Array constructor (length is 3 per ECMAScript spec)
	ctorWithProps := vm.NewConstructorWithProps(3, true, "Uint8Array", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayUint8, 0, 0, 0), nil
		}

		arg := args[0]

		// Handle different constructor patterns
		if arg.IsNumber() {
			// Uint8Array(length)
			length := int(arg.ToFloat())
			if length < 0 {
				// Should throw RangeError
				return vm.Undefined, nil
			}
			return vm.NewTypedArray(vm.TypedArrayUint8, length, 0, 0), nil
		}

		if buffer := arg.AsArrayBuffer(); buffer != nil {
			// Uint8Array(buffer, byteOffset?, length?)
			byteOffset := 0
			if len(args) > 1 {
				var err error
				byteOffset, err = ValidateTypedArrayByteOffset(vmInstance, args[1], 1)
				if err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}

			length := -1 // Use remaining buffer
			if len(args) > 2 && !args[2].IsUndefined() {
				length = int(args[2].ToFloat())
			}

			// If length is auto-calculated, validate buffer alignment
			if length == -1 {
				if err := ValidateTypedArrayBufferAlignment(vmInstance, buffer, byteOffset, 1); err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}

			return vm.NewTypedArray(vm.TypedArrayUint8, buffer, byteOffset, length), nil
		}

		if sab := arg.AsSharedArrayBuffer(); sab != nil {
			byteOffset := 0
			if len(args) > 1 {
				var err error
				byteOffset, err = ValidateTypedArrayByteOffsetShared(vmInstance, args[1], 1)
				if err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}
			length := -1
			if len(args) > 2 && !args[2].IsUndefined() {
				length = int(args[2].ToFloat())
			}
			if length == -1 {
				if err := ValidateTypedArrayBufferAlignmentShared(vmInstance, sab, byteOffset, 1); err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}
			return vm.NewTypedArray(vm.TypedArrayUint8, sab, byteOffset, length), nil
		}

		if sourceArray := arg.AsArray(); sourceArray != nil {
			// Uint8Array(array)
			values := make([]vm.Value, sourceArray.Length())
			for i := 0; i < sourceArray.Length(); i++ {
				values[i] = sourceArray.Get(i)
			}
			return vm.NewTypedArray(vm.TypedArrayUint8, values, 0, 0), nil
		}

		// Default case
		return vm.NewTypedArray(vm.TypedArrayUint8, 0, 0, 0), nil
	})

	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctorWithProps, uint8ArrayProto, 1)

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayUint8, 0, 0, 0), nil
		}

		source := args[0]
		if sourceArray := source.AsArray(); sourceArray != nil {
			values := make([]vm.Value, sourceArray.Length())
			for i := 0; i < sourceArray.Length(); i++ {
				values[i] = sourceArray.Get(i)
			}
			return vm.NewTypedArray(vm.TypedArrayUint8, values, 0, 0), nil
		}

		return vm.NewTypedArray(vm.TypedArrayUint8, 0, 0, 0), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		return vm.NewTypedArray(vm.TypedArrayUint8, args, 0, 0), nil
	}))

	// ES2024: Uint8Array.fromBase64(string, options?)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("fromBase64", vm.NewNativeFunction(1, false, "fromBase64", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 || args[0].Type() != vm.TypeString {
			return vm.Undefined, vmInstance.NewTypeError("fromBase64 requires a string argument")
		}

		input := args[0].AsString()

		// Get options
		alphabet := "base64"
		lastChunkHandling := "loose"
		if len(args) > 1 && args[1].IsObject() {
			opts := args[1]
			if alphaVal, err := vmInstance.GetProperty(opts, "alphabet"); err == nil && alphaVal.Type() == vm.TypeString {
				alphabet = alphaVal.AsString()
				if alphabet != "base64" && alphabet != "base64url" {
					return vm.Undefined, vmInstance.NewTypeError("Invalid alphabet: must be 'base64' or 'base64url'")
				}
			}
			if lchVal, err := vmInstance.GetProperty(opts, "lastChunkHandling"); err == nil && lchVal.Type() == vm.TypeString {
				lastChunkHandling = lchVal.AsString()
				if lastChunkHandling != "loose" && lastChunkHandling != "strict" && lastChunkHandling != "stop-before-partial" {
					return vm.Undefined, vmInstance.NewTypeError("Invalid lastChunkHandling")
				}
			}
		}

		// Remove whitespace
		input = strings.Map(func(r rune) rune {
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' {
				return -1
			}
			return r
		}, input)

		// Decode
		var decoded []byte
		var err error
		if alphabet == "base64url" {
			if lastChunkHandling != "stop-before-partial" {
				for len(input)%4 != 0 {
					input += "="
				}
			}
			decoded, err = base64.URLEncoding.DecodeString(input)
		} else {
			if lastChunkHandling != "stop-before-partial" {
				for len(input)%4 != 0 {
					input += "="
				}
			}
			decoded, err = base64.StdEncoding.DecodeString(input)
		}

		if err != nil {
			return vm.Undefined, vmInstance.NewSyntaxError("Invalid base64 string: " + err.Error())
		}

		// Create Uint8Array from decoded bytes
		values := make([]vm.Value, len(decoded))
		for i, b := range decoded {
			values[i] = vm.NumberValue(float64(b))
		}
		return vm.NewTypedArray(vm.TypedArrayUint8, values, 0, 0), nil
	}))

	// ES2024: Uint8Array.fromHex(string)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("fromHex", vm.NewNativeFunction(1, false, "fromHex", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 || args[0].Type() != vm.TypeString {
			return vm.Undefined, vmInstance.NewTypeError("fromHex requires a string argument")
		}

		input := args[0].AsString()

		// Validate hex characters
		for _, c := range input {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return vm.Undefined, vmInstance.NewSyntaxError("Invalid hex character")
			}
		}

		// Odd length is an error
		if len(input)%2 != 0 {
			return vm.Undefined, vmInstance.NewSyntaxError("Hex string must have even length")
		}

		decoded, err := hex.DecodeString(input)
		if err != nil {
			return vm.Undefined, vmInstance.NewSyntaxError("Invalid hex string: " + err.Error())
		}

		// Create Uint8Array from decoded bytes
		values := make([]vm.Value, len(decoded))
		for i, b := range decoded {
			values[i] = vm.NumberValue(float64(b))
		}
		return vm.NewTypedArray(vm.TypedArrayUint8, values, 0, 0), nil
	}))

	// Set constructor property on prototype
	uint8ArrayProto.SetOwnNonEnumerable("constructor", ctorWithProps)

	// Set the constructor's [[Prototype]] to TypedArray (for proper inheritance chain)
	// This makes Object.getPrototypeOf(Uint8Array) === TypedArray
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetPrototype(vmInstance.TypedArrayConstructor)

	// Set Uint8Array prototype in VM
	vmInstance.Uint8ArrayPrototype = vm.NewValueFromPlainObject(uint8ArrayProto)

	// Register Uint8Array constructor as global
	return ctx.DefineGlobal("Uint8Array", ctorWithProps)
}

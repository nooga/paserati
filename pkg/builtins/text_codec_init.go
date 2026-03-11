package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// TextEncoderInitializer implements TextEncoder constructor and prototype
type TextEncoderInitializer struct{}

func (t *TextEncoderInitializer) Name() string  { return "TextEncoder" }
func (t *TextEncoderInitializer) Priority() int { return 425 } // After Uint8Array (420)

func (t *TextEncoderInitializer) InitTypes(ctx *TypeContext) error {
	// Get the actual Uint8Array instance type for encode() return
	var encodeReturnType types.Type = types.Any
	if uint8ArrayCtor, ok := ctx.GetType("Uint8Array"); ok {
		if objType, ok2 := uint8ArrayCtor.(*types.ObjectType); ok2 {
			// Use the constructor's call signature return type (the instance type)
			if len(objType.CallSignatures) > 0 && objType.CallSignatures[0].ReturnType != nil {
				encodeReturnType = objType.CallSignatures[0].ReturnType
			}
		}
	}

	// TextEncoder.prototype type
	protoType := types.NewObjectType().
		WithProperty("encoding", types.String).
		WithProperty("encode", types.NewSimpleFunction([]types.Type{types.String}, encodeReturnType)).
		WithProperty("encodeInto", types.NewSimpleFunction([]types.Type{types.String, types.Any}, types.NewObjectType().
			WithProperty("read", types.Number).
			WithProperty("written", types.Number)))

	// TextEncoder constructor type
	ctorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, protoType).
		WithSimpleConstructSignature([]types.Type{}, protoType).
		WithProperty("prototype", protoType)

	return ctx.DefineGlobal("TextEncoder", ctorType)
}

func (t *TextEncoderInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	encoderProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	encoderProto.SetOwnNonEnumerable("encoding", vm.NewString("utf-8"))

	encoderProto.SetOwnNonEnumerable("encode", vm.NewNativeFunction(1, false, "encode", func(args []vm.Value) (vm.Value, error) {
		input := ""
		if len(args) > 0 {
			input = args[0].ToString()
		}
		data := []byte(input)
		arr := vm.NewArray()
		arrObj := arr.AsArray()
		for _, b := range data {
			arrObj.Append(vm.NumberValue(float64(b)))
		}
		return arr, nil
	}))

	ctor := vm.NewNativeFunction(0, true, "TextEncoder", func(args []vm.Value) (vm.Value, error) {
		inst := vm.NewObject(vm.NewValueFromPlainObject(encoderProto))
		return inst, nil
	})

	if nf := ctor.AsNativeFunction(); nf != nil {
		withProps := vm.NewConstructorWithProps(nf.Arity, nf.Variadic, nf.Name, nf.Fn)
		ctorProps := withProps.AsNativeFunctionWithProps()
		ctorProps.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(encoderProto))
		encoderProto.SetOwnNonEnumerable("constructor", withProps)
		return ctx.DefineGlobal("TextEncoder", withProps)
	}

	return ctx.DefineGlobal("TextEncoder", ctor)
}

// TextDecoderInitializer implements TextDecoder constructor and prototype
type TextDecoderInitializer struct{}

func (t *TextDecoderInitializer) Name() string  { return "TextDecoder" }
func (t *TextDecoderInitializer) Priority() int { return 425 } // After Uint8Array (420)

func (t *TextDecoderInitializer) InitTypes(ctx *TypeContext) error {
	// TextDecoder.prototype type
	protoType := types.NewObjectType().
		WithProperty("encoding", types.String).
		WithProperty("fatal", types.Boolean).
		WithProperty("ignoreBOM", types.Boolean).
		WithProperty("decode", types.NewSimpleFunction([]types.Type{}, types.String))

	// TextDecoder constructor type - takes optional encoding string
	ctorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, protoType).
		WithSimpleCallSignature([]types.Type{types.String}, protoType).
		WithSimpleConstructSignature([]types.Type{}, protoType).
		WithSimpleConstructSignature([]types.Type{types.String}, protoType).
		WithProperty("prototype", protoType)

	return ctx.DefineGlobal("TextDecoder", ctorType)
}

func (t *TextDecoderInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	decoderProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	decoderProto.SetOwnNonEnumerable("encoding", vm.NewString("utf-8"))
	decoderProto.SetOwnNonEnumerable("fatal", vm.BooleanValue(false))
	decoderProto.SetOwnNonEnumerable("ignoreBOM", vm.BooleanValue(false))

	decoderProto.SetOwnNonEnumerable("decode", vm.NewNativeFunction(1, false, "decode", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewString(""), nil
		}
		input := args[0]
		// Handle ArrayObject (Uint8Array-like)
		if arrObj := input.AsArray(); arrObj != nil {
			bytes := make([]byte, arrObj.Length())
			for i := 0; i < arrObj.Length(); i++ {
				val := arrObj.Get(i)
				bytes[i] = byte(val.ToFloat())
			}
			return vm.NewString(string(bytes)), nil
		}
		return vm.NewString(input.ToString()), nil
	}))

	ctor := vm.NewNativeFunction(0, true, "TextDecoder", func(args []vm.Value) (vm.Value, error) {
		inst := vm.NewObject(vm.NewValueFromPlainObject(decoderProto)).AsPlainObject()
		encoding := "utf-8"
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined {
			encoding = args[0].ToString()
		}
		inst.SetOwn("encoding", vm.NewString(encoding))
		inst.SetOwn("fatal", vm.BooleanValue(false))
		inst.SetOwn("ignoreBOM", vm.BooleanValue(false))
		return vm.NewValueFromPlainObject(inst), nil
	})

	if nf := ctor.AsNativeFunction(); nf != nil {
		withProps := vm.NewConstructorWithProps(nf.Arity, nf.Variadic, nf.Name, nf.Fn)
		ctorProps := withProps.AsNativeFunctionWithProps()
		ctorProps.Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(decoderProto))
		decoderProto.SetOwnNonEnumerable("constructor", withProps)
		return ctx.DefineGlobal("TextDecoder", withProps)
	}

	return ctx.DefineGlobal("TextDecoder", ctor)
}

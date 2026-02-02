package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type Int8ArrayInitializer struct{}

func (i *Int8ArrayInitializer) Name() string  { return "Int8Array" }
func (i *Int8ArrayInitializer) Priority() int { return 421 }

func (i *Int8ArrayInitializer) InitTypes(ctx *TypeContext) error {
	int8ArrayProtoType := types.NewObjectType().
		WithProperty("buffer", types.Any).
		WithProperty("byteLength", types.Number).
		WithProperty("byteOffset", types.Number).
		WithProperty("length", types.Number).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("set", types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Undefined)).
		WithProperty("subarray", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true})).
		WithProperty("slice", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true})).
		WithProperty("fill", types.NewOptionalFunction([]types.Type{types.Any, types.Number, types.Number}, types.Any, []bool{true, true, true}))

	ctorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, int8ArrayProtoType).
		WithSimpleCallSignature([]types.Type{types.Any}, int8ArrayProtoType).
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.Number}}, int8ArrayProtoType).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, int8ArrayProtoType)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, int8ArrayProtoType)).
		WithProperty("prototype", int8ArrayProtoType)

	return ctx.DefineGlobal("Int8Array", ctorType)
}

func (i *Int8ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmx := ctx.VM
	proto := vm.NewObject(vmx.TypedArrayPrototype).AsPlainObject()

	// Set up prototype properties with correct descriptors (BYTES_PER_ELEMENT, buffer, byteLength, byteOffset, length)
	SetupTypedArrayPrototypeProperties(proto, vmx, 1)

	// set
	proto.SetOwnNonEnumerable("set", vm.NewNativeFunction(2, false, "set", func(args []vm.Value) (vm.Value, error) {
		thisArr := vmx.GetThis()
		ta := thisArr.AsTypedArray()
		if ta == nil {
			return vm.Undefined, nil
		}
		offset := 0
		if len(args) > 1 {
			offset = int(args[1].ToFloat())
		}
		source := args[0]
		if arr := source.AsArray(); arr != nil {
			for i := 0; i < arr.Length() && offset+i < ta.GetLength(); i++ {
				ta.SetElement(offset+i, arr.Get(i))
			}
		} else if sa := source.AsTypedArray(); sa != nil {
			for i := 0; i < sa.GetLength() && offset+i < ta.GetLength(); i++ {
				ta.SetElement(offset+i, sa.GetElement(i))
			}
		}
		return vm.Undefined, nil
	}))

	// subarray
	proto.SetOwnNonEnumerable("subarray", vm.NewNativeFunction(2, false, "subarray", func(args []vm.Value) (vm.Value, error) {
		if ta := vmx.GetThis().AsTypedArray(); ta != nil {
			start, end := 0, ta.GetLength()
			if len(args) > 0 && !args[0].IsUndefined() {
				start = int(args[0].ToFloat())
				if start < 0 {
					start = ta.GetLength() + start
				}
				if start < 0 {
					start = 0
				}
				if start > ta.GetLength() {
					start = ta.GetLength()
				}
			}
			if len(args) > 1 && !args[1].IsUndefined() {
				end = int(args[1].ToFloat())
				if end < 0 {
					end = ta.GetLength() + end
				}
				if end < 0 {
					end = 0
				}
				if end > ta.GetLength() {
					end = ta.GetLength()
				}
			}
			if start > end {
				start = end
			}
			return vm.NewTypedArray(vm.TypedArrayInt8, ta.GetBuffer(), ta.GetByteOffset()+start, end-start), nil
		}
		return vm.Undefined, nil
	}))

	// slice
	proto.SetOwnNonEnumerable("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) (vm.Value, error) {
		if ta := vmx.GetThis().AsTypedArray(); ta != nil {
			start, end := 0, ta.GetLength()
			if len(args) > 0 && !args[0].IsUndefined() {
				start = int(args[0].ToFloat())
				if start < 0 {
					start = ta.GetLength() + start
				}
				if start < 0 {
					start = 0
				}
				if start > ta.GetLength() {
					start = ta.GetLength()
				}
			}
			if len(args) > 1 && !args[1].IsUndefined() {
				end = int(args[1].ToFloat())
				if end < 0 {
					end = ta.GetLength() + end
				}
				if end < 0 {
					end = 0
				}
				if end > ta.GetLength() {
					end = ta.GetLength()
				}
			}
			if start > end {
				start = end
			}
			length := end - start
			newArr := vm.NewTypedArray(vm.TypedArrayInt8, length, 0, 0)
			if n := newArr.AsTypedArray(); n != nil {
				for i := 0; i < length; i++ {
					n.SetElement(i, ta.GetElement(start+i))
				}
			}
			return newArr, nil
		}
		return vm.Undefined, nil
	}))

	// fill
	proto.SetOwnNonEnumerable("fill", vm.NewNativeFunction(3, false, "fill", func(args []vm.Value) (vm.Value, error) {
		if ta := vmx.GetThis().AsTypedArray(); ta != nil {
			value := vm.Undefined
			if len(args) > 0 {
				value = args[0]
			}
			start := 0
			end := ta.GetLength()
			if len(args) > 1 && !args[1].IsUndefined() {
				start = int(args[1].ToFloat())
				if start < 0 {
					start = ta.GetLength() + start
				}
				if start < 0 {
					start = 0
				}
			}
			if len(args) > 2 && !args[2].IsUndefined() {
				end = int(args[2].ToFloat())
				if end < 0 {
					end = ta.GetLength() + end
				}
				if end < 0 {
					end = 0
				}
				if end > ta.GetLength() {
					end = ta.GetLength()
				}
			}
			for i := start; i < end; i++ {
				ta.SetElement(i, value)
			}
			return vmx.GetThis(), nil
		}
		return vm.Undefined, nil
	}))

	// constructor (length is 3 per ECMAScript spec)
	ctor := vm.NewConstructorWithProps(3, true, "Int8Array", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayInt8, 0, 0, 0), nil
		}
		arg := args[0]
		if arg.IsNumber() {
			l := int(arg.ToFloat())
			if l < 0 {
				return vm.Undefined, nil
			}
			return vm.NewTypedArray(vm.TypedArrayInt8, l, 0, 0), nil
		}
		if buf := arg.AsArrayBuffer(); buf != nil {
			off := 0
			if len(args) > 1 {
				var err error
				off, err = ValidateTypedArrayByteOffset(vmx, args[1], 1)
				if err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}
			ln := -1
			if len(args) > 2 && !args[2].IsUndefined() {
				ln = int(args[2].ToFloat())
			}
			// If length is auto-calculated, validate buffer alignment
			if ln == -1 {
				if err := ValidateTypedArrayBufferAlignment(vmx, buf, off, 1); err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}
			return vm.NewTypedArray(vm.TypedArrayInt8, buf, off, ln), nil
		}
		if sab := arg.AsSharedArrayBuffer(); sab != nil {
			off := 0
			if len(args) > 1 {
				var err error
				off, err = ValidateTypedArrayByteOffsetShared(vmx, args[1], 1)
				if err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}
			ln := -1
			if len(args) > 2 && !args[2].IsUndefined() {
				ln = int(args[2].ToFloat())
			}
			if ln == -1 {
				if err := ValidateTypedArrayBufferAlignmentShared(vmx, sab, off, 1); err != nil {
					if err == ErrVMUnwinding {
						return vm.Undefined, nil
					}
					return vm.Undefined, err
				}
			}
			return vm.NewTypedArray(vm.TypedArrayInt8, sab, off, ln), nil
		}
		if arr := arg.AsArray(); arr != nil {
			vals := make([]vm.Value, arr.Length())
			for i := 0; i < arr.Length(); i++ {
				vals[i] = arr.Get(i)
			}
			return vm.NewTypedArray(vm.TypedArrayInt8, vals, 0, 0), nil
		}
		return vm.NewTypedArray(vm.TypedArrayInt8, 0, 0, 0), nil
	})
	// Set up constructor properties with correct descriptors (BYTES_PER_ELEMENT, prototype)
	SetupTypedArrayConstructorProperties(ctor, proto, 1)
	ctor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayInt8, 0, 0, 0), nil
		}
		src := args[0]
		if a := src.AsArray(); a != nil {
			vals := make([]vm.Value, a.Length())
			for i := 0; i < a.Length(); i++ {
				vals[i] = a.Get(i)
			}
			return vm.NewTypedArray(vm.TypedArrayInt8, vals, 0, 0), nil
		}
		return vm.NewTypedArray(vm.TypedArrayInt8, 0, 0, 0), nil
	}))
	ctor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) { return vm.NewTypedArray(vm.TypedArrayInt8, args, 0, 0), nil }))

	proto.SetOwnNonEnumerable("constructor", ctor)

	// Set the constructor's [[Prototype]] to TypedArray (for proper inheritance chain)
	// This makes Object.getPrototypeOf(Int8Array) === TypedArray
	ctor.AsNativeFunctionWithProps().Properties.SetPrototype(vmx.TypedArrayConstructor)

	vmx.Int8ArrayPrototype = vm.NewValueFromPlainObject(proto)
	return ctx.DefineGlobal("Int8Array", ctor)
}

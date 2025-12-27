package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type Uint32ArrayInitializer struct{}

func (u *Uint32ArrayInitializer) Name() string  { return "Uint32Array" }
func (u *Uint32ArrayInitializer) Priority() int { return 423 }

func (u *Uint32ArrayInitializer) InitTypes(ctx *TypeContext) error {
	proto := types.NewObjectType().
		WithProperty("buffer", types.Any).
		WithProperty("byteLength", types.Number).
		WithProperty("byteOffset", types.Number).
		WithProperty("length", types.Number).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("set", types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Undefined)).
		WithProperty("subarray", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true})).
		WithProperty("slice", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.Any, []bool{true, true})).
		WithProperty("fill", types.NewOptionalFunction([]types.Type{types.Any, types.Number, types.Number}, types.Any, []bool{true, true, true}))

	ctor := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{types.Number}, proto).
		WithSimpleCallSignature([]types.Type{types.Any}, proto).
		WithSimpleCallSignature([]types.Type{&types.ArrayType{ElementType: types.Number}}, proto).
		WithProperty("BYTES_PER_ELEMENT", types.Number).
		WithProperty("from", types.NewSimpleFunction([]types.Type{types.Any}, proto)).
		WithProperty("of", types.NewSimpleFunction([]types.Type{}, proto)).
		WithProperty("prototype", proto)

	return ctx.DefineGlobal("Uint32Array", ctor)
}

func (u *Uint32ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmx := ctx.VM
	proto := vm.NewObject(ctx.ObjectPrototype).AsPlainObject()

	proto.SetOwnNonEnumerable("set", vm.NewNativeFunction(2, false, "set", func(args []vm.Value) (vm.Value, error) {
		ta := vmx.GetThis().AsTypedArray()
		if ta == nil {
			return vm.Undefined, nil
		}
		offset := 0
		if len(args) > 1 {
			offset = int(args[1].ToFloat())
		}
		src := args[0]
		if arr := src.AsArray(); arr != nil {
			for i := 0; i < arr.Length() && offset+i < ta.GetLength(); i++ {
				ta.SetElement(offset+i, arr.Get(i))
			}
		} else if sa := src.AsTypedArray(); sa != nil {
			for i := 0; i < sa.GetLength() && offset+i < ta.GetLength(); i++ {
				ta.SetElement(offset+i, sa.GetElement(i))
			}
		}
		return vm.Undefined, nil
	}))

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
			return vm.NewTypedArray(vm.TypedArrayUint32, ta.GetBuffer(), ta.GetByteOffset()+start, end-start), nil
		}
		return vm.Undefined, nil
	}))

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
			out := vm.NewTypedArray(vm.TypedArrayUint32, length, 0, 0)
			if n := out.AsTypedArray(); n != nil {
				for i := 0; i < length; i++ {
					n.SetElement(i, ta.GetElement(start+i))
				}
			}
			return out, nil
		}
		return vm.Undefined, nil
	}))

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

	ctor := vm.NewConstructorWithProps(-1, true, "Uint32Array", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewTypedArray(vm.TypedArrayUint32, 0, 0, 0), nil
		}
		arg := args[0]
		if arg.IsNumber() {
			l := int(arg.ToFloat())
			if l < 0 {
				return vm.Undefined, nil
			}
			return vm.NewTypedArray(vm.TypedArrayUint32, l, 0, 0), nil
		}
		if buf := arg.AsArrayBuffer(); buf != nil {
			off := 0
			if len(args) > 1 {
				off = int(args[1].ToFloat())
			}
			ln := -1
			if len(args) > 2 {
				ln = int(args[2].ToFloat())
			}
			return vm.NewTypedArray(vm.TypedArrayUint32, buf, off, ln), nil
		}
		if arr := arg.AsArray(); arr != nil {
			vals := make([]vm.Value, arr.Length())
			for i := 0; i < arr.Length(); i++ {
				vals[i] = arr.Get(i)
			}
			return vm.NewTypedArray(vm.TypedArrayUint32, vals, 0, 0), nil
		}
		return vm.NewTypedArray(vm.TypedArrayUint32, 0, 0, 0), nil
	})
	ctor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(proto))
	return ctx.DefineGlobal("Uint32Array", ctor)
}

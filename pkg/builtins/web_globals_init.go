package builtins

import (
	"encoding/base64"
	"strings"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// WebGlobalsInitializer provides the HTML/WinterCG globals atob, btoa and
// structuredClone (#624).
type WebGlobalsInitializer struct{}

func (w *WebGlobalsInitializer) Name() string  { return "WebGlobals" }
func (w *WebGlobalsInitializer) Priority() int { return 430 } // after typed arrays, Map/Set, Date, errors

func (w *WebGlobalsInitializer) InitTypes(ctx *TypeContext) error {
	strFn := types.NewSimpleFunction([]types.Type{types.String}, types.String)
	if err := ctx.DefineGlobal("atob", strFn); err != nil {
		return err
	}
	if err := ctx.DefineGlobal("btoa", strFn); err != nil {
		return err
	}
	// structuredClone<T>(value: T, options?): T, approximated as any -> any.
	return ctx.DefineGlobal("structuredClone", types.NewFunctionType(&types.Signature{
		ParameterTypes: []types.Type{types.Any, types.Any},
		ReturnType:     types.Any,
		OptionalParams: []bool{false, true},
	}))
}

func (w *WebGlobalsInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	atob := vm.NewNativeFunction(1, false, "atob", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("atob: 1 argument required, but only 0 present.")
		}
		decoded, ok := forgivingBase64Decode(args[0].ToString())
		if !ok {
			return vm.Undefined, domException(vmInstance, "InvalidCharacterError", "The string to be decoded is not correctly encoded.")
		}
		// Each byte becomes one code unit (a "binary string").
		units := make([]uint16, len(decoded))
		for i, b := range decoded {
			units[i] = uint16(b)
		}
		return vm.NewString(vm.UTF16ToString(units)), nil
	})
	if err := ctx.DefineGlobal("atob", atob); err != nil {
		return err
	}

	btoa := vm.NewNativeFunction(1, false, "btoa", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("btoa: 1 argument required, but only 0 present.")
		}
		units := vm.StringToUTF16(args[0].ToString())
		data := make([]byte, len(units))
		for i, u := range units {
			if u > 0xFF {
				return vm.Undefined, domException(vmInstance, "InvalidCharacterError", "The string to be encoded contains characters outside of the Latin1 range.")
			}
			data[i] = byte(u)
		}
		return vm.NewString(base64.StdEncoding.EncodeToString(data)), nil
	})
	if err := ctx.DefineGlobal("btoa", btoa); err != nil {
		return err
	}

	structuredClone := vm.NewNativeFunction(1, false, "structuredClone", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("structuredClone: 1 argument required, but only 0 present.")
		}
		c := &cloner{vm: vmInstance, memo: make(map[vm.Value]vm.Value)}
		return c.clone(args[0])
	})
	return ctx.DefineGlobal("structuredClone", structuredClone)
}

// forgivingBase64Decode is the WHATWG forgiving-base64 decode: ASCII
// whitespace is ignored, padding is optional, and anything else malformed
// fails.
func forgivingBase64Decode(s string) ([]byte, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case ' ', '\t', '\n', '\f', '\r':
		default:
			b.WriteByte(c)
		}
	}
	data := b.String()
	if len(data)%4 == 0 {
		data = strings.TrimSuffix(data, "=")
		data = strings.TrimSuffix(data, "=")
	}
	if len(data)%4 == 1 {
		return nil, false
	}
	for i := 0; i < len(data); i++ {
		c := data[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '/') {
			return nil, false
		}
	}
	// The default (non-strict) decoder ignores non-zero trailing bits, as
	// forgiving-base64 does.
	out, err := base64.RawStdEncoding.DecodeString(data)
	if err != nil {
		return nil, false
	}
	return out, true
}

// domException is the closest this runtime has to a DOMException: an Error
// whose name is the DOMException name (there is no DOMException class).
func domException(vmInstance *vm.VM, name, message string) error {
	if ctor, ok := vmInstance.GetGlobal("Error"); ok {
		if v, err := vmInstance.Call(ctor, vm.Undefined, []vm.Value{vm.NewString(message)}); err == nil && v.IsObject() {
			v.AsPlainObject().SetOwnNonEnumerable("name", vm.NewString(name))
			return vmInstance.NewExceptionError(v)
		}
	}
	return vmInstance.NewTypeError(name + ": " + message)
}

// cloner implements StructuredSerialize + StructuredDeserialize in one pass,
// with memo preserving shared and circular references.
type cloner struct {
	vm   *vm.VM
	memo map[vm.Value]vm.Value // keyed by identity: a Value compares by its object pointer
}

func (c *cloner) uncloneable(what string) error {
	return domException(c.vm, "DataCloneError", what+" could not be cloned.")
}

func (c *cloner) clone(v vm.Value) (vm.Value, error) {
	switch v.Type() {
	case vm.TypeUndefined, vm.TypeNull, vm.TypeBoolean, vm.TypeIntegerNumber, vm.TypeFloatNumber, vm.TypeBigInt, vm.TypeString:
		return v, nil
	case vm.TypeSymbol:
		return vm.Undefined, c.uncloneable("Symbol()")
	}
	if v.IsCallable() {
		return vm.Undefined, c.uncloneable(v.ToString())
	}
	key := v
	if done, ok := c.memo[key]; ok {
		return done, nil
	}

	switch v.Type() {
	case vm.TypeArray:
		src := v.AsArray()
		out := vm.NewArray()
		c.memo[key] = out
		dst := out.AsArray()
		for i := 0; i < src.Length(); i++ {
			if !src.HasIndex(i) {
				dst.SetLength(i + 1)
				continue
			}
			e, err := c.clone(src.Get(i))
			if err != nil {
				return vm.Undefined, err
			}
			dst.Set(i, e)
		}
		dst.SetLength(src.Length())
		return out, nil
	case vm.TypeMap:
		out := vm.NewMap()
		out.AsMap().SetPrototype(c.vm.MapPrototype)
		c.memo[key] = out
		var err error
		v.AsMap().ForEach(func(k, val vm.Value) {
			if err != nil {
				return
			}
			var ck, cv vm.Value
			if ck, err = c.clone(k); err != nil {
				return
			}
			if cv, err = c.clone(val); err != nil {
				return
			}
			out.AsMap().Set(ck, cv)
		})
		return out, err
	case vm.TypeSet:
		out := vm.NewSet()
		out.AsSet().SetPrototype(c.vm.SetPrototype)
		c.memo[key] = out
		var err error
		v.AsSet().ForEach(func(val vm.Value) {
			if err != nil {
				return
			}
			var cv vm.Value
			if cv, err = c.clone(val); err != nil {
				return
			}
			out.AsSet().Add(cv)
		})
		return out, err
	case vm.TypeRegExp:
		re := v.AsRegExpObject()
		out, err := vm.NewRegExp(re.GetSource(), re.GetFlags())
		if err != nil {
			return vm.Undefined, err
		}
		c.memo[key] = out
		return out, nil
	case vm.TypeArrayBuffer:
		src := v.AsArrayBuffer()
		if src.IsDetached() {
			return vm.Undefined, c.uncloneable("ArrayBuffer (detached)")
		}
		out := vm.NewArrayBuffer(len(src.GetData()))
		copy(out.AsArrayBuffer().GetData(), src.GetData())
		c.memo[key] = out
		return out, nil
	case vm.TypeTypedArray:
		ta := v.AsTypedArray()
		buf, err := c.clone(vm.NewArrayBufferFromObject(ta.GetBuffer()))
		if err != nil {
			return vm.Undefined, err
		}
		out := vm.NewTypedArray(ta.GetElementType(), buf.AsArrayBuffer(), ta.GetByteOffset(), ta.GetLength())
		c.memo[key] = out
		return out, nil
	case vm.TypeObject:
		return c.cloneObject(v, key)
	}
	return vm.Undefined, c.uncloneable(v.TypeName())
}

// cloneObject clones a plain object: Date, primitive wrappers and errors by
// their internal data, anything else as an ordinary object of its own
// enumerable string-keyed properties (class instances lose their class, as
// in the spec).
func (c *cloner) cloneObject(v vm.Value, key vm.Value) (vm.Value, error) {
	obj := v.AsPlainObject()
	if ts, ok := obj.GetInternal("__timestamp__"); ok {
		d := vm.NewObject(c.vm.DatePrototype)
		d.AsPlainObject().SetInternal("__timestamp__", ts)
		c.memo[key] = d
		return d, nil
	}
	if prim, ok := obj.GetInternal("[[PrimitiveValue]]"); ok {
		var out vm.Value
		switch prim.Type() {
		case vm.TypeBoolean:
			out = c.vm.NewBooleanObject(prim.AsBoolean())
		case vm.TypeString:
			out = c.vm.NewStringObject(prim.ToString())
		case vm.TypeIntegerNumber, vm.TypeFloatNumber:
			out = c.vm.NewNumberObject(prim.ToFloat())
		default:
			return vm.Undefined, c.uncloneable("object")
		}
		c.memo[key] = out
		return out, nil
	}
	if _, isError := obj.GetInternal("[[ErrorData]]"); isError {
		return c.cloneError(v, key)
	}

	out := vm.NewObject(c.vm.ObjectPrototype)
	c.memo[key] = out
	keys, err := objectKeysWithVM(c.vm, []vm.Value{v})
	if err != nil {
		return vm.Undefined, err
	}
	dst := out.AsPlainObject()
	ka := keys.AsArray()
	for i := 0; i < ka.Length(); i++ {
		name := ka.Get(i).ToString()
		val, err := c.vm.GetProperty(v, name)
		if err != nil {
			return vm.Undefined, err
		}
		cv, err := c.clone(val)
		if err != nil {
			return vm.Undefined, err
		}
		dst.SetOwn(name, cv)
	}
	return out, nil
}

// cloneError rebuilds an error through its native constructor (name one of
// the standard error types, else Error), carrying message, stack and cause.
func (c *cloner) cloneError(v vm.Value, key vm.Value) (vm.Value, error) {
	name := "Error"
	if n, err := c.vm.GetProperty(v, "name"); err == nil && n.IsString() {
		switch s := n.ToString(); s {
		case "EvalError", "RangeError", "ReferenceError", "SyntaxError", "TypeError", "URIError":
			name = s
		}
	}
	ctor, ok := c.vm.GetGlobal(name)
	if !ok {
		return vm.Undefined, c.uncloneable("Error")
	}
	var args []vm.Value
	if msg, ok := v.AsPlainObject().GetOwn("message"); ok {
		args = append(args, vm.NewString(msg.ToString()))
	}
	out, err := c.vm.Call(ctor, vm.Undefined, args)
	if err != nil {
		return vm.Undefined, err
	}
	c.memo[key] = out
	dst := out.AsPlainObject()
	if stack, ok := v.AsPlainObject().GetOwn("stack"); ok && stack.IsString() {
		dst.SetOwnNonEnumerable("stack", stack)
	}
	if cause, ok := v.AsPlainObject().GetOwn("cause"); ok {
		cc, err := c.clone(cause)
		if err != nil {
			return vm.Undefined, err
		}
		dst.SetOwnNonEnumerable("cause", cc)
	}
	return out, nil
}

package driver

import (
	"fmt"
	"math/big"
	"reflect"
	"sort"

	"github.com/nooga/paserati/pkg/vm"
)

// goConverter converts between Go values (via reflection) and VM values for
// native modules. vm may be nil (the package-level helpers below): objects
// then get no Object.prototype, and a JS function can't become a Go func.
type goConverter struct {
	vm *vm.VM
}

// jsCallbackPanic carries a JS exception out of a Go func parameter whose
// type has no error result; the native function wrapper recovers it and
// rethrows it into JS.
type jsCallbackPanic struct{ err error }

var (
	vmValueType = reflect.TypeOf(vm.Value{})
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
)

// reflectValueToVM converts a Go value without a VM at hand.
func reflectValueToVM(reflectVal reflect.Value) vm.Value {
	return (&goConverter{}).toVM(reflectVal)
}

// vmValueToReflectValue converts a VM value to targetType without a VM at hand.
func vmValueToReflectValue(vmVal vm.Value, targetType reflect.Type) reflect.Value {
	return (&goConverter{}).fromVM(vmVal, targetType)
}

// vmValueToInterface converts a VM value to its natural Go form.
func vmValueToInterface(vmVal vm.Value) interface{} {
	return (&goConverter{}).toInterface(vmVal)
}

func (c *goConverter) newObject() vm.Value {
	if c.vm != nil {
		return vm.NewObject(c.vm.ObjectPrototype)
	}
	return vm.NewObject(vm.Undefined)
}

// valueToVM converts an arbitrary Go value.
func (c *goConverter) valueToVM(value interface{}) vm.Value {
	if value == nil {
		return vm.Null
	}
	return c.toVM(reflect.ValueOf(value))
}

// toVM converts a Go value to a VM value: strings, numbers and booleans to
// primitives, maps with string-like keys and structs to objects, slices to
// arrays ([]byte to a Uint8Array), interfaces and pointers by what they hold
// (nil to null), funcs to native functions, and a vm.Value as itself.
func (c *goConverter) toVM(reflectVal reflect.Value) vm.Value {
	if !reflectVal.IsValid() {
		return vm.Undefined
	}
	if reflectVal.Type() == vmValueType {
		return reflectVal.Interface().(vm.Value)
	}
	if reflectVal.CanInterface() {
		if b, ok := reflectVal.Interface().(*big.Int); ok && b != nil {
			return vm.NewBigInt(new(big.Int).Set(b))
		}
	}

	switch reflectVal.Kind() {
	case reflect.String:
		return vm.NewString(reflectVal.String())
	case reflect.Float64, reflect.Float32:
		return vm.NumberValue(reflectVal.Float())
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		return vm.NumberValue(float64(reflectVal.Int()))
	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8, reflect.Uintptr:
		return vm.NumberValue(float64(reflectVal.Uint()))
	case reflect.Bool:
		return vm.BooleanValue(reflectVal.Bool())
	case reflect.Interface:
		if reflectVal.IsNil() {
			return vm.Null
		}
		return c.toVM(reflectVal.Elem())
	case reflect.Map:
		if reflectVal.IsNil() {
			return vm.Null
		}
		// Keys in sorted order, as encoding/json does: Go's random map
		// order would give each conversion its own property order (and its
		// own chain of shapes).
		type entry struct {
			name  string
			value reflect.Value
		}
		entries := make([]entry, 0, reflectVal.Len())
		for _, key := range reflectVal.MapKeys() {
			entries = append(entries, entry{c.toVM(key).ToString(), reflectVal.MapIndex(key)})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
		obj := c.newObject()
		objPtr := obj.AsPlainObject()
		for _, e := range entries {
			objPtr.SetOwn(e.name, c.toVM(e.value))
		}
		return obj
	case reflect.Ptr:
		if reflectVal.IsNil() {
			return vm.Null
		}
		if reflectVal.Elem().Kind() == reflect.Struct {
			return c.structToVM(reflectVal)
		}
		return c.toVM(reflectVal.Elem())
	case reflect.Struct:
		// A struct returned by value: bind a copy, as for a pointer.
		ptr := reflect.New(reflectVal.Type())
		ptr.Elem().Set(reflectVal)
		return c.structToVM(ptr)
	case reflect.Slice, reflect.Array:
		if reflectVal.Kind() == reflect.Slice && reflectVal.IsNil() {
			return vm.Null
		}
		if reflectVal.Type().Elem().Kind() == reflect.Uint8 {
			goBytes := make([]byte, reflectVal.Len())
			reflect.Copy(reflect.ValueOf(goBytes), reflectVal)
			arrayBufferValue := vm.NewArrayBuffer(len(goBytes))
			if buffer := arrayBufferValue.AsArrayBuffer(); buffer != nil {
				copy(buffer.GetData(), goBytes)
				return vm.NewTypedArray(vm.TypedArrayUint8, buffer, 0, 0)
			}
			return vm.Undefined
		}
		arr := vm.NewArray()
		arrayObj := arr.AsArray()
		for i := 0; i < reflectVal.Len(); i++ {
			arrayObj.Set(i, c.toVM(reflectVal.Index(i)))
		}
		return arr
	case reflect.Func:
		if reflectVal.IsNil() {
			return vm.Null
		}
		return c.functionToVM(reflectVal.Interface())
	default:
		return vm.Undefined
	}
}

// structToVM exposes a struct (through a pointer to it) as an object with
// accessor properties for its fields (by JSON name) and its methods.
func (c *goConverter) structToVM(ptr reflect.Value) vm.Value {
	instance := c.newObject()
	mb := &ModuleBuilder{vm: c.vm}
	mb.bindStructMethods(instance.AsPlainObject(), ptr, ptr.Elem().Type())
	return instance
}

// fromVM converts a VM value to a Go value of targetType, for arguments to
// a Go function. A value that doesn't fit becomes targetType's zero value.
func (c *goConverter) fromVM(vmVal vm.Value, targetType reflect.Type) reflect.Value {
	// A Go parameter typed vm.Value itself wants the raw argument, untouched.
	if targetType == vmValueType {
		return reflect.ValueOf(vmVal)
	}
	switch targetType.Kind() {
	case reflect.String:
		if vmVal.IsString() {
			return reflect.ValueOf(vmVal.AsString()).Convert(targetType)
		}
		return reflect.ValueOf(vmVal.ToString()).Convert(targetType)
	case reflect.Float64, reflect.Float32:
		if vmVal.IsNumber() {
			return reflect.ValueOf(vmVal.ToFloat()).Convert(targetType)
		}
		return reflect.Zero(targetType)
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		if vmVal.IsNumber() {
			return reflect.ValueOf(int64(vmVal.ToFloat())).Convert(targetType)
		}
		return reflect.Zero(targetType)
	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8, reflect.Uintptr:
		if vmVal.IsNumber() {
			if f := vmVal.ToFloat(); f > 0 {
				return reflect.ValueOf(uint64(f)).Convert(targetType)
			}
		}
		return reflect.Zero(targetType)
	case reflect.Bool:
		if vmVal.IsBoolean() {
			return reflect.ValueOf(vmVal.AsBoolean()).Convert(targetType)
		}
		return reflect.ValueOf(vmVal.IsTruthy()).Convert(targetType)
	case reflect.Map:
		if targetType.Key().Kind() != reflect.String {
			return reflect.Zero(targetType)
		}
		keys, get, ok := ownEntries(vmVal)
		if !ok {
			return reflect.Zero(targetType)
		}
		newMap := reflect.MakeMap(targetType)
		for _, key := range keys {
			if val, ok := get(key); ok {
				newMap.SetMapIndex(reflect.ValueOf(key).Convert(targetType.Key()), c.fromVM(val, targetType.Elem()))
			}
		}
		return newMap
	case reflect.Slice:
		if targetType.Elem().Kind() == reflect.Uint8 && vmVal.Type() == vm.TypeTypedArray {
			ta := vmVal.AsTypedArray()
			data := ta.GetBuffer().GetData()
			start := ta.GetByteOffset()
			out := make([]byte, ta.GetByteLength())
			copy(out, data[start:start+len(out)])
			return reflect.ValueOf(out).Convert(targetType)
		}
		if vmVal.Type() != vm.TypeArray {
			return reflect.Zero(targetType)
		}
		arr := vmVal.AsArray()
		out := reflect.MakeSlice(targetType, arr.Length(), arr.Length())
		for i := 0; i < arr.Length(); i++ {
			out.Index(i).Set(c.fromVM(arr.Get(i), targetType.Elem()))
		}
		return out
	case reflect.Interface:
		if v := c.toInterface(vmVal); v != nil {
			rv := reflect.ValueOf(v)
			if rv.Type().AssignableTo(targetType) {
				return rv
			}
		}
		return reflect.Zero(targetType)
	case reflect.Func:
		if !vmVal.IsCallable() {
			return reflect.Zero(targetType)
		}
		return c.jsFunctionToGo(vmVal, targetType)
	case reflect.Ptr:
		// null and undefined (a missing argument) are nil; anything else
		// converts to what the pointer holds.
		if vmVal.Type() == vm.TypeNull || vmVal.Type() == vm.TypeUndefined {
			return reflect.Zero(targetType)
		}
		// An instance of a Go-backed class is passed as itself.
		if inst, ok := goInstanceFromThis(vmVal); ok && inst.Type() == targetType {
			return inst
		}
		out := reflect.New(targetType.Elem())
		out.Elem().Set(c.fromVM(vmVal, targetType.Elem()))
		return out
	case reflect.Struct:
		// An object fills the exported fields named (by JSON name, as
		// goTypeToTSType types them) by its own properties.
		keys, get, ok := ownEntries(vmVal)
		out := reflect.New(targetType).Elem()
		if !ok {
			return out
		}
		fields := structFieldsByJSONName(targetType)
		for _, key := range keys {
			idx, known := fields[key]
			if !known {
				continue
			}
			if val, ok := get(key); ok {
				out.Field(idx).Set(c.fromVM(val, targetType.Field(idx).Type))
			}
		}
		return out
	default:
		return reflect.Zero(targetType)
	}
}

// structFieldsByJSONName maps a struct's exported fields' JSON names to
// their indices.
func structFieldsByJSONName(t reflect.Type) map[string]int {
	mb := &ModuleBuilder{}
	fields := make(map[string]int, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if name := mb.getJSONPropertyName(f); name != "" {
			fields[name] = i
		}
	}
	return fields
}

// jsFunctionToGo wraps a JS function as a Go func of type fnType. Its
// results are converted to fnType's; a JS exception becomes the error
// result when fnType's last result is error, and otherwise propagates back
// out through the native function that received the callback.
func (c *goConverter) jsFunctionToGo(fn vm.Value, fnType reflect.Type) reflect.Value {
	numOut := fnType.NumOut()
	hasErr := numOut > 0 && fnType.Out(numOut-1) == errorType
	return reflect.MakeFunc(fnType, func(in []reflect.Value) []reflect.Value {
		out := make([]reflect.Value, numOut)
		for i := range out {
			out[i] = reflect.Zero(fnType.Out(i))
		}
		fail := func(err error) []reflect.Value {
			if hasErr {
				out[numOut-1] = reflect.ValueOf(&err).Elem()
				return out
			}
			panic(jsCallbackPanic{err})
		}
		if c.vm == nil {
			return fail(fmt.Errorf("JS callback called without a VM"))
		}
		args := make([]vm.Value, len(in))
		for i, a := range in {
			args[i] = c.toVM(a)
		}
		res, err := c.vm.Call(fn, vm.Undefined, args)
		if err != nil {
			return fail(err)
		}
		if numOut > 0 && !(hasErr && numOut == 1) {
			out[0] = c.fromVM(res, fnType.Out(0))
		}
		return out
	})
}

// toInterface converts a VM value to its natural Go form: string, float64,
// bool, nil (null/undefined), *big.Int, []interface{} for arrays and
// map[string]interface{} for objects. Functions and other exotic values
// become nil.
func (c *goConverter) toInterface(vmVal vm.Value) interface{} {
	switch {
	case vmVal.IsString():
		return vmVal.AsString()
	case vmVal.IsNumber():
		return vmVal.ToFloat()
	case vmVal.IsBoolean():
		return vmVal.AsBoolean()
	case vmVal.Type() == vm.TypeNull, vmVal.Type() == vm.TypeUndefined:
		return nil
	case vmVal.Type() == vm.TypeBigInt:
		return new(big.Int).Set(vmVal.AsBigInt())
	case vmVal.Type() == vm.TypeArray:
		arr := vmVal.AsArray()
		out := make([]interface{}, arr.Length())
		for i := range out {
			out[i] = c.toInterface(arr.Get(i))
		}
		return out
	}
	keys, get, ok := ownEntries(vmVal)
	if !ok {
		return nil
	}
	result := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		if val, ok := get(key); ok {
			result[key] = c.toInterface(val)
		}
	}
	return result
}

// ownEntries gives the own string keys and a getter of a plain or dict
// object.
func ownEntries(v vm.Value) ([]string, func(string) (vm.Value, bool), bool) {
	switch {
	case v.IsObject():
		o := v.AsPlainObject()
		return o.OwnKeys(), o.GetOwn, true
	case v.IsDictObject():
		o := v.AsDictObject()
		return o.OwnKeys(), o.GetOwn, true
	}
	return nil, nil, false
}

// ToValue converts a Go value to a JS value on this session's VM, the way
// native module functions convert their results: maps and structs become
// objects, slices arrays ([]byte a Uint8Array), funcs callable functions,
// interfaces and pointers what they hold, nil null.
func (p *Paserati) ToValue(v interface{}) vm.Value {
	return (&goConverter{vm: p.vmInstance}).valueToVM(v)
}

// Export converts a JS value to its natural Go form: string, float64, bool,
// nil (null/undefined), *big.Int, []interface{} for arrays and
// map[string]interface{} for objects.
func (p *Paserati) Export(v vm.Value) interface{} {
	return (&goConverter{vm: p.vmInstance}).toInterface(v)
}

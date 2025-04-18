package values

import (
	"fmt"
	"paserati/pkg/vm"
	"unsafe"
)

type FunctionObject struct {
	Object
	arity        int
	variadic     bool
	chunk        *vm.Chunk
	name         string
	upvalueCount int
	registerSize int
}

type Upvalue struct {
	Location *Value
	Closed   Value
	next     *Upvalue
}

func (uv *Upvalue) Close() {
	if uv.Location != nil {
		uv.Closed = *uv.Location
		uv.Location = nil
	}
}

func (uv *Upvalue) Resolve() *Value {
	if uv.Location == nil {
		return &uv.Closed
	}
	return uv.Location
}

type ClosureObject struct {
	Object
	fn       *FunctionObject
	upvalues []*Upvalue
}

type NativeFunctionObject struct {
	Object
	arity    int
	variadic bool
	name     string
	fn       func(args []Value) Value
}

func NewFunction(arity, upvalueCount, registerSize int, variadic bool, name string, chunk *vm.Chunk) Value {
	fnObj := &FunctionObject{
		arity:        arity,
		variadic:     variadic,
		chunk:        chunk,
		name:         name,
		upvalueCount: upvalueCount,
		registerSize: registerSize,
	}
	return Value{typ: TypeFunction, obj: unsafe.Pointer(fnObj)}
}

func NewClosure(fn *FunctionObject, upvalues []*Upvalue) Value {
	if fn == nil {
		panic("Cannot create Closure with a nil FunctionObject")
	}
	if len(upvalues) != fn.upvalueCount {
		panic(fmt.Sprintf("Incorrect number of upvalues provided for closure: expected %d, got %d", fn.upvalueCount, len(upvalues)))
	}
	closureObj := &ClosureObject{
		fn:       fn,
		upvalues: upvalues,
	}
	return Value{typ: TypeClosure, obj: unsafe.Pointer(closureObj)}
}

func NewNativeFunction(arity int, variadic bool, name string, fn func(args []Value) Value) Value {
	return Value{typ: TypeNativeFunction, obj: unsafe.Pointer(&NativeFunctionObject{arity: arity, variadic: variadic, name: name, fn: fn})}
}

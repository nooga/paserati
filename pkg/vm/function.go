package vm

import (
	"fmt"
	"unsafe"
)

type FunctionObject struct {
   Object
   Arity        int
   Variadic     bool
   Chunk        *Chunk
   Name         string
   UpvalueCount int
   RegisterSize int
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
   Fn       *FunctionObject
   Upvalues []*Upvalue
}

// NativeFunctionObject represents a native Go function callable from Paserati.
type NativeFunctionObject struct {
   Object
   Arity    int
   Variadic bool
   Name     string
   Fn       func(args []Value) Value
}

func NewFunction(arity, upvalueCount, registerSize int, variadic bool, name string, chunk *Chunk) Value {
   fnObj := &FunctionObject{
       Arity:        arity,
       Variadic:     variadic,
       Chunk:        chunk,
       Name:         name,
       UpvalueCount: upvalueCount,
       RegisterSize: registerSize,
   }
	return Value{typ: TypeFunction, obj: unsafe.Pointer(fnObj)}
}

func NewClosure(fn *FunctionObject, upvalues []*Upvalue) Value {
	if fn == nil {
		panic("Cannot create Closure with a nil FunctionObject")
	}
   if len(upvalues) != fn.UpvalueCount {
       panic(fmt.Sprintf("Incorrect number of upvalues provided for closure: expected %d, got %d", fn.UpvalueCount, len(upvalues)))
   }
   closureObj := &ClosureObject{
       Fn:       fn,
       Upvalues: upvalues,
   }
	return Value{typ: TypeClosure, obj: unsafe.Pointer(closureObj)}
}

func NewNativeFunction(arity int, variadic bool, name string, fn func(args []Value) Value) Value {
   return Value{typ: TypeNativeFunction, obj: unsafe.Pointer(&NativeFunctionObject{
       Arity:    arity,
       Variadic: variadic,
       Name:     name,
       Fn:       fn,
   })}
}

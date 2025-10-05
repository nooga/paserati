package vm

import (
	"fmt"
	"unsafe"
)

type FunctionObject struct {
	Object
	Arity               int
	Variadic            bool
	Chunk               *Chunk
	Name                string
	UpvalueCount        int
	RegisterSize        int
	IsGenerator         bool         // True for generator functions (function*)
	IsAsync             bool         // True for async functions
	IsArrowFunction     bool         // True for arrow functions (cannot be used as constructors)
	IsDerivedConstructor bool        // True for derived class constructors (must call super())
	Properties          *PlainObject // For properties like .prototype (created lazily)
	Prototype           Value        // [[Prototype]] - the function's prototype (usually Function.prototype)
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
	Fn       func(args []Value) (Value, error)
}

// BoundNativeFunctionObject represents a native function bound to a 'this' value
type BoundNativeFunctionObject struct {
	Object
	ThisValue  Value
	NativeFunc *NativeFunctionObject
	Name       string
}

// BoundFunctionObject represents any function bound to a 'this' value and optional partial arguments
type BoundFunctionObject struct {
	Object
	OriginalFunction Value   // The function being bound (can be any callable type)
	BoundThis        Value   // The 'this' value to use when calling
	PartialArgs      []Value // Arguments to prepend to call arguments
	Name             string  // For debugging/inspection
}

// NativeFunctionObjectWithProps represents a native function that can also have properties
// This is useful for constructors that need static methods (like String.fromCharCode)
type NativeFunctionObjectWithProps struct {
	Object
	Arity      int
	Variadic   bool
	Name       string
	Fn         func(args []Value) (Value, error)
	Properties *PlainObject // Can have properties like static methods
}

// AsyncNativeFunctionObject represents a native function that can call bytecode functions
// This uses Go channels for async communication with the VM
type AsyncNativeFunctionObject struct {
	Object
	Arity      int
	Variadic   bool
	Name       string
	// AsyncFn receives a VMCaller interface that can call bytecode functions
	AsyncFn    func(caller VMCaller, args []Value) Value
}

// VMCaller provides an interface for native functions to call bytecode functions
type VMCaller interface {
	CallBytecode(fn Value, thisValue Value, args []Value) Value
}

func NewFunction(arity, upvalueCount, registerSize int, variadic bool, name string, chunk *Chunk, isGenerator bool, isAsync bool, isArrowFunction bool) Value {
	fnObj := &FunctionObject{
		Arity:        arity,
		Variadic:     variadic,
		Chunk:        chunk,
		Name:         name,
		UpvalueCount: upvalueCount,
		RegisterSize: registerSize,
		IsGenerator:  isGenerator,
		IsAsync:      isAsync,
		IsArrowFunction: isArrowFunction,
		Properties:   nil, // Start with nil - create lazily
	}
	return Value{typ: TypeFunction, obj: unsafe.Pointer(fnObj)}
}

// getOrCreatePrototype lazily creates and returns the function's prototype property
func (fn *FunctionObject) getOrCreatePrototype() Value {
	return fn.getOrCreatePrototypeWithVM(nil)
}

func (fn *FunctionObject) getOrCreatePrototypeWithVM(vm *VM) Value {
	// Ensure Properties object exists
	if fn.Properties == nil {
		fn.Properties = NewObject(Undefined).AsPlainObject()
	}

	// Check if prototype already exists
	if proto, exists := fn.Properties.GetOwn("prototype"); exists {
		return proto
	}

	// Determine the correct prototype parent based on function type
	var prototypeParent Value = DefaultObjectPrototype

	// For generator and async generator functions, use their specific prototypes
	if vm != nil {
		if fn.IsAsync && fn.IsGenerator {
			// Async generator function's .prototype should inherit from AsyncGeneratorPrototype
			prototypeParent = vm.AsyncGeneratorPrototype
		} else if fn.IsGenerator {
			// Generator function's .prototype should inherit from GeneratorPrototype
			prototypeParent = vm.GeneratorPrototype
		}
	}

	// Create prototype lazily with the appropriate parent
	prototypeObj := NewObject(prototypeParent)
	fn.Properties.SetOwn("prototype", prototypeObj)

	// Set constructor property on prototype (circular reference)
	if prototypeObj.IsObject() {
		protoPlain := prototypeObj.AsPlainObject()
		constructorVal := Value{typ: TypeFunction, obj: unsafe.Pointer(fn)}
		protoPlain.SetOwn("constructor", constructorVal)
	}

	return prototypeObj
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

func NewNativeFunction(arity int, variadic bool, name string, fn func(args []Value) (Value, error)) Value {
	return Value{typ: TypeNativeFunction, obj: unsafe.Pointer(&NativeFunctionObject{
		Arity:    arity,
		Variadic: variadic,
		Name:     name,
		Fn:       fn,
	})}
}

func NewNativeFunctionWithProps(arity int, variadic bool, name string, fn func(args []Value) (Value, error)) Value {
	props := NewObject(Undefined).AsPlainObject()
	return Value{typ: TypeNativeFunctionWithProps, obj: unsafe.Pointer(&NativeFunctionObjectWithProps{
		Arity:      arity,
		Variadic:   variadic,
		Name:       name,
		Fn:         fn,
		Properties: props,
	})}
}

func NewAsyncNativeFunction(arity int, variadic bool, name string, asyncFn func(caller VMCaller, args []Value) Value) Value {
	return Value{typ: TypeAsyncNativeFunction, obj: unsafe.Pointer(&AsyncNativeFunctionObject{
		Arity:    arity,
		Variadic: variadic,
		Name:     name,
		AsyncFn:  asyncFn,
	})}
}

func NewBoundFunction(originalFunction Value, boundThis Value, partialArgs []Value, name string) Value {
	// Copy partial args to avoid aliasing issues
	argsCopy := make([]Value, len(partialArgs))
	copy(argsCopy, partialArgs)
	
	return Value{typ: TypeBoundFunction, obj: unsafe.Pointer(&BoundFunctionObject{
		OriginalFunction: originalFunction,
		BoundThis:        boundThis,
		PartialArgs:      argsCopy,
		Name:             name,
	})}
}

package vm

import (
	"fmt"
	"unsafe"
)

type FunctionObject struct {
	Object
	Arity                int          // Number of declared parameters (used for VM register allocation)
	Length               int          // ECMAScript length property (params before first default, per spec)
	Variadic             bool
	Chunk                *Chunk
	Name                 string
	UpvalueCount         int
	RegisterSize         int
	IsGenerator          bool         // True for generator functions (function*)
	IsAsync              bool         // True for async functions
	IsArrowFunction      bool         // True for arrow functions (cannot be used as constructors)
	IsDerivedConstructor bool         // True for derived class constructors (must call super())
	IsClassConstructor   bool         // True for class constructors (calling without 'new' throws TypeError)
	Properties           *PlainObject // For properties like .prototype (created lazily)
	Prototype            Value        // [[Prototype]] - the function's prototype (usually Function.prototype)
	HomeObject           Value        // [[HomeObject]] - object where method is defined (for super property access)
	HomeRealm            *Realm       // [[Realm]] - the realm where this function was created
	NameBindingRegister  int          // For named function expressions: register to initialize with closure (-1 if not used)

	// Deleted intrinsic property tracking - these are configurable:true so can be deleted
	DeletedName   bool // True if the 'name' property has been deleted
	DeletedLength bool // True if the 'length' property has been deleted

	// HasLocalCaptures indicates if any nested closure captures locals from this function.
	// When false, closeUpvalues can be skipped entirely on return (major performance win).
	// Set at compile time when emitting OpClosure with CaptureFromRegister or CaptureFromSpill.
	HasLocalCaptures bool

	// cachedClosure is used to avoid per-call allocations when invoking TypeFunction values.
	// Most runtime calls should operate on TypeClosure, but some compilation paths may leave
	// no-capture functions as TypeFunction. In that case we can reuse this closure wrapper.
	cachedClosure *ClosureObject
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
	Fn                       *FunctionObject
	Upvalues                 []*Upvalue
	WithObjects              []Value // Captured with-object stack from enclosing with statements
	CapturedThis             Value   // Captured 'this' for arrow functions (lexical this binding)
	CapturedSuperConstructor Value   // Captured super constructor for arrow functions with super() calls
	CapturedArguments        Value   // Captured 'arguments' for arrow functions (lexical arguments binding)
	CapturedNewTarget        Value   // Captured 'new.target' for arrow functions (lexical new.target binding)
	CapturedHomeObject       Value   // Captured [[HomeObject]] for arrow functions (for super property access)
	Properties               *PlainObject // Per-closure properties like .prototype (created lazily, shadows Fn.Properties)
	constructorFixed         bool         // True after we've fixed the constructor property to point to this closure
}

// NativeFunctionObject represents a native Go function callable from Paserati.
type NativeFunctionObject struct {
	Object
	Arity         int
	Variadic      bool
	Name          string
	Fn            func(args []Value) (Value, error)
	IsConstructor bool         // If true, can be used with 'new'; false by default for most native functions
	Properties    *PlainObject // Lazily created when user code sets properties on this function
	HomeRealm     *Realm       // [[Realm]] - the realm where this function was created
	DeletedName   bool         // True if the 'name' property has been deleted
	DeletedLength bool         // True if the 'length' property has been deleted
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
	OriginalFunction Value        // The function being bound (can be any callable type)
	BoundThis        Value        // The 'this' value to use when calling
	PartialArgs      []Value      // Arguments to prepend to call arguments
	Name             string       // For debugging/inspection
	Properties       *PlainObject // Per ECMAScript, bound functions can have properties
}

// NativeFunctionObjectWithProps represents a native function that can also have properties
// This is useful for constructors that need static methods (like String.fromCharCode)
type NativeFunctionObjectWithProps struct {
	Object
	Arity         int
	Variadic      bool
	Name          string
	Fn            func(args []Value) (Value, error)
	Properties    *PlainObject // Can have properties like static methods
	IsConstructor bool         // If true, can be used with 'new'; most built-in constructors set this to true
	HomeRealm     *Realm       // [[Realm]] - the realm where this function was created
	DeletedName   bool         // True if the 'name' property has been deleted
	DeletedLength bool         // True if the 'length' property has been deleted
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

func NewFunction(arity, length, upvalueCount, registerSize int, variadic bool, name string, chunk *Chunk, isGenerator bool, isAsync bool, isArrowFunction bool, hasLocalCaptures bool) Value {
	fnObj := &FunctionObject{
		Arity:               arity,
		Length:              length,
		Variadic:            variadic,
		Chunk:               chunk,
		Name:                name,
		UpvalueCount:        upvalueCount,
		RegisterSize:        registerSize,
		IsGenerator:         isGenerator,
		IsAsync:             isAsync,
		IsArrowFunction:     isArrowFunction,
		HasLocalCaptures:    hasLocalCaptures,
		NameBindingRegister: -1,  // Default: no name binding
		Properties:          nil, // Start with nil - create lazily
	}
	return Value{typ: TypeFunction, obj: unsafe.Pointer(fnObj)}
}

// GetOrCreatePrototype lazily creates and returns the function's prototype property
func (fn *FunctionObject) GetOrCreatePrototype() Value {
	return fn.GetOrCreatePrototypeWithVM(nil)
}

// GetOrCreatePrototypeWithVM lazily creates and returns the function's prototype property,
// using the VM's prototypes for proper inheritance chain setup.
func (fn *FunctionObject) GetOrCreatePrototypeWithVM(vm *VM) Value {
	// NUCLEAR DEBUG

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

	// When we have a VM, use the proper prototypes
	if vm != nil {
		if fn.IsAsync && fn.IsGenerator {
			// Async generator function's .prototype should inherit from AsyncGeneratorPrototype
			prototypeParent = vm.AsyncGeneratorPrototype
		} else if fn.IsGenerator {
			// Generator function's .prototype should inherit from GeneratorPrototype
			prototypeParent = vm.GeneratorPrototype
		} else {
			// Regular function's .prototype should inherit from Object.prototype
			// This ensures user-defined constructor prototypes have toString(), valueOf(), etc.
			prototypeParent = vm.ObjectPrototype
		}
	}

	// Create prototype lazily with the appropriate parent
	prototypeObj := NewObject(prototypeParent)
	// Per ECMAScript spec, function.prototype is: writable=true, enumerable=false, configurable=false
	// (This applies to both regular functions and generator functions)
	w, e, c := true, false, false
	fn.Properties.DefineOwnProperty("prototype", prototypeObj, &w, &e, &c)

	// Set constructor property on prototype (circular reference)
	// IMPORTANT: Generator and async generator function prototypes should NOT have a constructor property
	// per ECMAScript spec (they should be plain empty objects)
	// Per ECMAScript, constructor should be: writable=true, enumerable=false, configurable=true
	if prototypeObj.IsObject() && !fn.IsGenerator {
		protoPlain := prototypeObj.AsPlainObject()
		constructorVal := Value{typ: TypeFunction, obj: unsafe.Pointer(fn)}
		w, e, c := true, false, true // writable=true, enumerable=false, configurable=true
		protoPlain.DefineOwnProperty("constructor", constructorVal, &w, &e, &c)
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

// GetPrototypeWithVM returns the prototype to use for instances created with this closure.
// It first checks the closure's own Properties for a "prototype" property (set via assignment),
// then falls back to the underlying FunctionObject's prototype.
// IMPORTANT: When using the function's prototype, we update the constructor property to point
// to this closure, ensuring `new MyFunc().constructor === MyFunc` works correctly.
func (c *ClosureObject) GetPrototypeWithVM(vm *VM) Value {
	// First check closure's own properties (set via `Inner.prototype = proto`)
	if c.Properties != nil {
		if proto, exists := c.Properties.GetOwn("prototype"); exists {
			return proto
		}
	}
	// Fall back to the underlying function's prototype
	proto := c.Fn.GetOrCreatePrototypeWithVM(vm)

	// Fix the constructor property ONCE to point to this closure instead of the underlying function
	// This ensures `new MyFunc().constructor === MyFunc` works correctly
	if !c.constructorFixed && proto.IsObject() && !c.Fn.IsGenerator {
		protoObj := proto.AsPlainObject()
		// Create a closure value for this closure object
		closureVal := Value{typ: TypeClosure, obj: unsafe.Pointer(c)}
		w, e, cfg := true, false, true
		protoObj.DefineOwnProperty("constructor", closureVal, &w, &e, &cfg)
		c.constructorFixed = true
	}

	return proto
}

func NewNativeFunction(arity int, variadic bool, name string, fn func(args []Value) (Value, error)) Value {
	return Value{typ: TypeNativeFunction, obj: unsafe.Pointer(&NativeFunctionObject{
		Arity:    arity,
		Variadic: variadic,
		Name:     name,
		Fn:       fn,
	})}
}

// NewNativeConstructor creates a native function that can be used as a constructor.
// This is the same as NewNativeFunction but with IsConstructor set to true.
func NewNativeConstructor(arity int, variadic bool, name string, fn func(args []Value) (Value, error)) Value {
	return Value{typ: TypeNativeFunction, obj: unsafe.Pointer(&NativeFunctionObject{
		Arity:         arity,
		Variadic:      variadic,
		Name:          name,
		Fn:            fn,
		IsConstructor: true,
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

// NewConstructorWithProps creates a native function with properties that can be used as a constructor.
// This is the same as NewNativeFunctionWithProps but with IsConstructor set to true.
func NewConstructorWithProps(arity int, variadic bool, name string, fn func(args []Value) (Value, error)) Value {
	props := NewObject(Undefined).AsPlainObject()
	return Value{typ: TypeNativeFunctionWithProps, obj: unsafe.Pointer(&NativeFunctionObjectWithProps{
		Arity:         arity,
		Variadic:      variadic,
		Name:          name,
		Fn:            fn,
		Properties:    props,
		IsConstructor: true,
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

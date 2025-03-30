package value

import (
	"fmt"
	// "paseratti2/pkg/bytecode" // REMOVED to break import cycle
	"strconv"
)

// ValueType represents the type of a Value.
type ValueType uint8

const (
	TypeUndefined ValueType = iota // Default/uninitialized/implicit return
	TypeNull                       // Explicit null value
	TypeBool
	TypeNumber
	TypeString
	TypeFunction // Represents *bytecode.Function
	TypeClosure  // Represents *value.Closure
	// Add TypeObject later
)

// // Function struct moved to pkg/bytecode to break import cycle
// type Function struct { ... }

// // Minimal interface to access Function details needed by value.String()
// // This avoids importing bytecode but requires the actual function object
// // passed to NewFunction to implement this interface.
// type FunctionInfo interface {
//  GetName() string
//  GetArity() int
// }

// Value represents a value in the VM.
// We use a tagged union approach for performance.
type Value struct {
	Type ValueType
	// Using a union of fields instead of interface{}
	// can be more performant for primitive types.
	as struct {
		boolean bool
		number  float64
		str     string
		obj     interface{} // Use interface{} to store complex types like Function, Closure, Object
		// without creating direct import cycles.
	}
}

// Constructors

func Undefined() Value {
	return Value{Type: TypeUndefined}
}

func Null() Value {
	return Value{Type: TypeNull}
}

func Bool(value bool) Value {
	v := Value{Type: TypeBool}
	v.as.boolean = value
	return v
}

func Number(value float64) Value {
	v := Value{Type: TypeNumber}
	v.as.number = value
	return v
}

func String(value string) Value {
	v := Value{Type: TypeString}
	v.as.str = value
	return v
}

// NewFunction creates a Function value.
// The `fn` argument must be the actual function object (e.g., *bytecode.Function)
// which will be stored in the `obj` field.
func NewFunction(fn interface{}) Value {
	// Could add type assertion here to ensure fn is a pointer type if needed
	// _, ok := fn.(FunctionInfo) // Check if it implements the interface if using interface approach
	// if !ok { panic("Object passed to NewFunction is not a FunctionInfo") }
	v := Value{Type: TypeFunction}
	v.as.obj = fn
	return v
}

// NewClosure creates a Closure value.
// `fn` must be the *bytecode.Function.
// `upvalues` is the slice of pointers to Upvalue objects created during closure creation.
func NewClosure(fn interface{}, upvalues []*Upvalue) Value {
	closure := &Closure{
		Fn:       fn,
		Upvalues: upvalues,
	}
	v := Value{Type: TypeClosure}
	v.as.obj = closure
	return v
}

// ClosureV creates a Closure value directly from a *Closure pointer.
// Used by the VM during OpClosure execution.
func ClosureV(closure *Closure) Value {
	if closure == nil {
		// Handle nil closure pointer gracefully, perhaps return Undefined or panic
		panic("Attempted to create Value from nil Closure pointer")
	}
	v := Value{Type: TypeClosure}
	v.as.obj = closure
	return v
}

// Type Checkers

func IsUndefined(v Value) bool {
	return v.Type == TypeUndefined
}

func IsNull(v Value) bool {
	return v.Type == TypeNull
}

func IsBool(v Value) bool {
	return v.Type == TypeBool
}

func IsNumber(v Value) bool {
	return v.Type == TypeNumber
}

func IsString(v Value) bool {
	return v.Type == TypeString
}

func IsFunction(v Value) bool {
	return v.Type == TypeFunction
}

func IsClosure(v Value) bool {
	return v.Type == TypeClosure
}

// Accessors (with type checking)

func AsBool(v Value) bool {
	if !IsBool(v) {
		// In a real implementation, might panic or return an error
		panic("value is not a bool")
	}
	return v.as.boolean
}

func AsNumber(v Value) float64 {
	if !IsNumber(v) {
		panic("value is not a number")
	}
	return v.as.number
}

func AsString(v Value) string {
	if !IsString(v) {
		panic("value is not a string")
	}
	return v.as.str
}

// AsFunction performs a type assertion to retrieve the underlying function object.
// The caller must know the actual type stored (e.g., *bytecode.Function).
func AsFunction(v Value) interface{} { // Returns interface{}, caller asserts
	if !IsFunction(v) {
		panic("value is not a function")
	}
	return v.as.obj
}

// AsClosure performs a type assertion to retrieve the underlying *Closure object.
func AsClosure(v Value) *Closure { // Returns concrete type
	if !IsClosure(v) {
		panic("value is not a closure")
	}
	// Type assertion is safe due to the check above
	return v.as.obj.(*Closure)
}

// String representation for debugging/printing

func (v Value) String() string {
	switch v.Type {
	case TypeUndefined:
		return "undefined"
	case TypeNull:
		return "null"
	case TypeBool:
		return strconv.FormatBool(v.as.boolean)
	case TypeNumber:
		// Format float nicely
		return strconv.FormatFloat(v.as.number, 'f', -1, 64)
	case TypeString:
		// Could add quotes for clarity
		return v.as.str
	case TypeFunction:
		// Attempt to get name from the underlying *bytecode.Function
		// This requires reflection or careful interface design.
		// Let's keep it simple for now.
		return "<fn>"
	case TypeClosure:
		// Similarly, try to get the underlying function's name for a better representation.
		// For now, just indicate it's a closure.
		// closure := AsClosure(v)
		// Accessing closure.Fn would require reflection/assertion again.
		return "<closure>"
	default:
		return fmt.Sprintf("Unknown ValueType: %d", v.Type)
	}
}

// Consider adding methods for equality, comparison, etc. later

// Upvalue represents a variable captured by a closure.
// It holds a pointer to the variable on the stack or another upvalue
// so that assignments to closed-over variables work correctly.
type Upvalue struct {
	Location *Value // Pointer to stack slot (if open) OR nil (if closed)
	Closed   Value  // Holds the value after the stack slot is invalid (if closed)
}

// Close closes an open upvalue by copying the value from its stack location
// into its Closed field and setting Location to nil.
func (uv *Upvalue) Close(val Value) {
	uv.Closed = val
	uv.Location = nil
}

// Closure represents a function object combined with its captured upvalues.
type Closure struct {
	Fn       interface{} // Holds the *bytecode.Function object
	Upvalues []*Upvalue  // Slice of pointers to captured variables
}

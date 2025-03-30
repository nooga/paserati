package vm

import (
	"fmt"
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
	TypeFunction // Represents *Function
	TypeClosure  // Represents *Closure
	// Add TypeObject later
)

// Function represents a compiled function.
// Moved here from old bytecode package.
type Function struct {
	Arity        int    // Number of parameters expected
	Chunk        *Chunk // Bytecode for the function body (Chunk is defined in bytecode.go)
	Name         string // Optional name for debugging
	RegisterSize int    // Number of registers needed for this function's frame
	// TODO: Add UpvalueCount later for closures
}

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
		fn      *Function // Direct pointer for functions
		closure *Closure  // Direct pointer for closures
		// obj     interface{} // Keep for other potential object types? Or remove if only Fn/Closure?
		// For now, removing obj, assuming only these complex types for now.
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
// The `fn` argument must be a *Function pointer.
func NewFunction(fn *Function) Value {
	if fn == nil {
		panic("Attempted to create Value from nil Function pointer")
	}
	v := Value{Type: TypeFunction}
	v.as.fn = fn
	return v
}

// NewClosure creates a new Closure object and returns a Closure value.
// `fn` must be the *Function.
// `upvalues` is the slice of pointers to Upvalue objects.
func NewClosure(fn *Function, upvalues []*Upvalue) Value {
	if fn == nil {
		panic("Cannot create Closure with a nil Function pointer")
	}
	closure := &Closure{
		Fn:       fn,
		Upvalues: upvalues,
	}
	v := Value{Type: TypeClosure}
	v.as.closure = closure
	return v
}

// ClosureV creates a Closure value directly from a *Closure pointer.
// Used by the VM during OpClosure execution.
func ClosureV(closure *Closure) Value {
	if closure == nil {
		panic("Attempted to create Value from nil Closure pointer")
	}
	v := Value{Type: TypeClosure}
	v.as.closure = closure
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

// AsFunction returns the underlying *Function pointer.
func AsFunction(v Value) *Function {
	if !IsFunction(v) {
		panic("value is not a function")
	}
	return v.as.fn
}

// AsClosure returns the underlying *Closure pointer.
func AsClosure(v Value) *Closure {
	if !IsClosure(v) {
		panic("value is not a closure")
	}
	return v.as.closure
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
		return strconv.FormatFloat(v.as.number, 'f', -1, 64)
	case TypeString:
		return v.as.str
	case TypeFunction:
		fn := v.as.fn // No assertion needed
		if fn.Name != "" {
			return fmt.Sprintf("<fn %s>", fn.Name)
		} else {
			return "<script>" // Or <fn> ?
		}
	case TypeClosure:
		closure := v.as.closure // No assertion needed
		if closure.Fn != nil && closure.Fn.Name != "" {
			return fmt.Sprintf("<closure %s>", closure.Fn.Name)
		} else {
			return "<closure>"
		}
	default:
		return fmt.Sprintf("Unknown ValueType: %d", v.Type)
	}
}

// Upvalue represents a variable captured by a closure.
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
	Fn       *Function  // Now holds a direct pointer to the Function object
	Upvalues []*Upvalue // Slice of pointers to captured variables
}

// isFalsey determines the truthiness of a value according to common dynamic language rules.
// null and false are falsey, everything else is truthy.
func isFalsey(v Value) bool {
	return IsNull(v) || (IsBool(v) && !AsBool(v))
}

// valuesEqual checks if two values are equal.
// Handles different types appropriately (e.g., number vs string comparison is false).
func valuesEqual(a, b Value) bool {
	if a.Type != b.Type {
		return false // Different types are never equal
	}
	switch a.Type {
	case TypeUndefined:
		return true // undefined == undefined
	case TypeNull:
		return true // null == null
	case TypeBool:
		return AsBool(a) == AsBool(b)
	case TypeNumber:
		return AsNumber(a) == AsNumber(b)
	case TypeString:
		return AsString(a) == AsString(b)
	case TypeFunction:
		// Function equality is typically by reference
		return AsFunction(a) == AsFunction(b)
	case TypeClosure:
		// Closure equality is typically by reference
		return AsClosure(a) == AsClosure(b)
	default:
		return false // Should not happen
	}
}

package vm

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ValueType represents the type of a Value.
type ValueType uint8

const (
	TypeUndefined ValueType = iota // Default/uninitialized/implicit return
	TypeNull                       // Explicit null value
	TypeBool
	TypeNumber
	TypeString
	TypeFunction    // Represents *Function (compiled Paserati code)
	TypeClosure     // Represents *Closure (compiled Paserati code)
	TypeArray       // Represents *Array (Added)
	TypeObject      // Represents *Object (NEW)
	TypeBuiltinFunc // Represents *BuiltinFunc (native Go function)
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

// --- NEW: BuiltinFunc Struct ---
// BuiltinFunc represents a native Go function callable from Paserati.
type BuiltinFunc struct {
	Name  string
	Func  func(args []Value) (Value, error) // Explicitly takes/returns vm.Value
	Arity int                               // -1 for variadic
}

// --- END NEW ---

// --- NEW: Object Struct (Phase 1: Map-based) ---
type Object struct {
	Properties map[string]Value // Simple map for properties in Phase 1
}

// --- END NEW ---

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
		fn      *Function    // Direct pointer for functions
		closure *Closure     // Direct pointer for closures
		arr     *Array       // Direct pointer for arrays (Added)
		obj     *Object      // Direct pointer for objects (NEW)
		builtin *BuiltinFunc // Direct pointer for built-in funcs
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

// --- NEW: Object Constructor ---
func ObjectV(obj *Object) Value {
	if obj == nil {
		panic("Attempted to create Value from nil Object pointer")
	}
	v := Value{Type: TypeObject}
	v.as.obj = obj
	return v
}

// --- END NEW ---

// --- NEW: BuiltinFunc Constructor ---
func NewBuiltinFunc(bf *BuiltinFunc) Value {
	if bf == nil {
		panic("Attempted to create Value from nil BuiltinFunc pointer")
	}
	v := Value{Type: TypeBuiltinFunc}
	v.as.builtin = bf
	return v
}

// --- END NEW ---

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

// --- NEW: Object Type Checker ---
func IsObject(v Value) bool {
	return v.Type == TypeObject
}

// --- END NEW ---

// --- NEW: BuiltinFunc Type Checker ---
func IsBuiltinFunc(v Value) bool {
	return v.Type == TypeBuiltinFunc
}

// --- END NEW ---

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

// --- NEW: Object Accessor ---
func AsObject(v Value) *Object {
	if !IsObject(v) {
		panic("value is not an object")
	}
	return v.as.obj
}

// --- END NEW ---

// --- NEW: BuiltinFunc Accessor ---
func AsBuiltinFunc(v Value) *BuiltinFunc {
	if !IsBuiltinFunc(v) {
		panic("value is not a built-in function")
	}
	return v.as.builtin
}

// --- END NEW ---

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
		num := v.as.number
		// Handle negative zero display explicitly
		if num == 0 && math.Signbit(num) {
			return "0" // Display negative zero as just "0"
		}
		return strconv.FormatFloat(num, 'f', -1, 64)
	case TypeString:
		// Represent strings more explicitly for debugging object keys
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
	case TypeArray:
		arr := v.as.arr
		var builder strings.Builder
		builder.WriteString("[")
		for i, elem := range arr.Elements {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(elem.String()) // Recursively call String()
		}
		builder.WriteString("]")
		return builder.String()
	// --- NEW: Object String Representation ---
	case TypeObject:
		obj := v.as.obj
		var builder strings.Builder
		builder.WriteString("{")
		count := 0
		for key, val := range obj.Properties {
			if count > 0 {
				builder.WriteString(", ")
			}
			// Ensure keys are represented as quoted strings if they contain spaces or special chars,
			// although map keys are strings here anyway.
			builder.WriteString(strconv.Quote(key))
			builder.WriteString(": ")
			builder.WriteString(val.String()) // Recursively call String()
			count++
		}
		builder.WriteString("}")
		return builder.String()
	// --- END NEW ---
	case TypeBuiltinFunc:
		bf := v.as.builtin // No assertion needed due to switch
		if bf != nil && bf.Name != "" {
			return fmt.Sprintf("<builtin fn %s>", bf.Name)
		} else {
			return "<builtin fn>"
		}
	default:
		return fmt.Sprintf("Unknown ValueType: %d", v.Type)
	}
}

func (v Value) TypeName() string {
	switch v.Type {
	case TypeUndefined:
		return "undefined"
	case TypeNull:
		return "null"
	case TypeBool:
		return "boolean"
	case TypeNumber:
		return "number"
	case TypeString:
		return "string"
	case TypeFunction:
		return "function"
	case TypeClosure:
		return "closure"
	case TypeArray:
		return "array"
	case TypeObject:
		return "object"
	case TypeBuiltinFunc:
		return "function"
	default:
		return fmt.Sprintf("<unknown type: %d>", v.Type)
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
// false, null, undefined, 0, and "" are falsey. Everything else is truthy.
func isFalsey(v Value) bool {
	switch v.Type {
	case TypeNull:
		return true
	case TypeUndefined:
		return true
	case TypeBool:
		return !AsBool(v)
	case TypeNumber:
		return AsNumber(v) == 0
	case TypeString:
		return AsString(v) == ""
	default: // Including TypeObject, TypeFunction, TypeClosure, TypeArray
		return false // Objects, functions, arrays are generally truthy
	}
}

// valuesEqual checks if two values are equal.
// Handles different types appropriately (e.g., number vs string comparison is false).
func valuesEqual(a, b Value) bool {
	if a.Type != b.Type {
		// --- Phase 1: Strict type equality ---
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
	case TypeArray:
		// Array equality is by reference
		return AsArray(a) == AsArray(b)
	// --- NEW: Object Equality ---
	case TypeObject:
		// Object equality is by reference
		return AsObject(a) == AsObject(b)
	// --- END NEW ---
	case TypeBuiltinFunc:
		// Builtin function equality is by reference
		return AsBuiltinFunc(a) == AsBuiltinFunc(b)
	default:
		return false // Should not happen
	}
}

// --- NEW: Array Type --- (Added)

// Array represents a runtime array value.
type Array struct {
	Elements []Value
}

// --- NEW: Array Constructor/Checker/Accessor --- (Added)

// NewArray creates a new Array value containing the given elements.
func NewArray(elements []Value) Value {
	arr := &Array{Elements: elements}
	v := Value{Type: TypeArray}
	v.as.arr = arr
	return v
}

func IsArray(v Value) bool {
	return v.Type == TypeArray
}

func AsArray(v Value) *Array {
	if !IsArray(v) {
		panic("value is not an array")
	}
	return v.as.arr
}

// --- END NEW ---

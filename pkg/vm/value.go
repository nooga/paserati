package vm

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unsafe"
)

type ValueType uint8

const (
	TypeNull ValueType = iota
	TypeUndefined

	TypeString
	TypeSymbol

	TypeFloatNumber
	TypeIntegerNumber
	TypeBigInt

	TypeBoolean

	TypeFunction
	TypeNativeFunction
	TypeClosure

	TypeObject
	TypeDictObject

	TypeArray
)

type StringObject struct {
	Object
	value string
}

type SymbolObject struct {
	Object
	value string
}

type ArrayObject struct {
	Object
	length   int
	elements []Value
}

type BigIntObject struct {
	Object
	value *big.Int
}

type Value struct {
	typ     ValueType
	payload uint64
	obj     unsafe.Pointer
}

var (
	Undefined = Value{typ: TypeUndefined}
	Null      = Value{typ: TypeNull}
	True      = Value{typ: TypeBoolean, payload: 1}
	False     = Value{typ: TypeBoolean, payload: 0}
	NaN       = Value{typ: TypeFloatNumber, payload: math.Float64bits(math.NaN())}
)

func NumberValue(value float64) Value {
	return Value{typ: TypeFloatNumber, payload: uint64(math.Float64bits(value))}
}

func IntegerValue(value int32) Value {
	return Value{typ: TypeIntegerNumber, payload: uint64(int64(value))}
}

func BooleanValue(value bool) Value {
	if value {
		return True
	}
	return False
}

func NewBigInt(value *big.Int) Value {
	return Value{typ: TypeBigInt, obj: unsafe.Pointer(&BigIntObject{value: value})}
}

func NewString(value string) Value {
	return Value{typ: TypeString, obj: unsafe.Pointer(&StringObject{value: value})}
}

func NewSymbol(value string) Value {
	return Value{typ: TypeSymbol, obj: unsafe.Pointer(&SymbolObject{value: value})}
}

func NewArray() Value {
	return Value{typ: TypeArray, obj: unsafe.Pointer(&ArrayObject{})}
}

// NewArrayWithArgs creates an array based on the Array constructor arguments:
// - No args: empty array
// - Single numeric arg: array with that length (filled with undefined)
// - Multiple args: array with those elements
func NewArrayWithArgs(args []Value) Value {
	arr := NewArray()
	arrayObj := arr.AsArray()

	if len(args) == 0 {
		// Array() - empty array
		return arr
	} else if len(args) == 1 && args[0].IsNumber() {
		// Array(length) - array with specified length
		length := int(args[0].ToFloat())
		if length < 0 {
			length = 0
		}
		arrayObj.SetLength(length)
		return arr
	} else {
		// Array(element1, element2, ...) - array with specified elements
		arrayObj.SetElements(args)
		return arr
	}
}

func (v Value) IsNumber() bool {
	return v.typ == TypeFloatNumber || v.typ == TypeIntegerNumber
}

func (v Value) IsFloatNumber() bool {
	return v.typ == TypeFloatNumber
}

func (v Value) IsIntegerNumber() bool {
	return v.typ == TypeIntegerNumber
}

func (v Value) IsBigInt() bool {
	return v.typ == TypeBigInt
}

func (v Value) IsString() bool {
	return v.typ == TypeString
}

func (v Value) IsSymbol() bool {
	return v.typ == TypeSymbol
}

func (v Value) IsBoolean() bool {
	return v.typ == TypeBoolean
}

func (v Value) IsObject() bool {
	return v.typ == TypeObject || v.typ == TypeDictObject
}

func (v Value) IsDictObject() bool {
	return v.typ == TypeDictObject
}

func (v Value) IsArray() bool {
	return v.typ == TypeArray
}

func (v Value) IsCallable() bool {
	return v.typ == TypeFunction || v.typ == TypeNativeFunction || v.typ == TypeClosure
}

func (v Value) IsFunction() bool {
	return v.typ == TypeFunction
}

func (v Value) IsClosure() bool {
	return v.typ == TypeClosure
}

func (v Value) IsNativeFunction() bool {
	return v.typ == TypeNativeFunction
}

func (v Value) Type() ValueType {
	return v.typ
}

func (v Value) TypeName() string {
	switch v.typ {
	case TypeUndefined:
		return "undefined"
	case TypeNull:
		return "null"
	case TypeBoolean:
		return "boolean"
	case TypeFloatNumber, TypeIntegerNumber:
		return "number"
	case TypeBigInt:
		return "bigint"
	case TypeString:
		return "string"
	case TypeSymbol:
		return "symbol"
	case TypeFunction, TypeClosure, TypeNativeFunction:
		return "function"
	case TypeObject, TypeDictObject, TypeArray:
		return "object"
	default:
		return fmt.Sprintf("<unknown type: %d>", v.typ)
	}
}

func (v Value) AsFloat() float64 {
	if v.typ != TypeFloatNumber {
		panic("value is not a float")
	}
	return math.Float64frombits(uint64(v.payload))
}

func (v Value) AsInteger() int32 {
	if v.typ != TypeIntegerNumber {
		panic("value is not an integer")
	}
	return int32(v.payload)
}

func (v Value) AsBigInt() *big.Int {
	if v.typ != TypeBigInt {
		panic("value is not a big int")
	}
	return (*BigIntObject)(v.obj).value
}

func (v Value) AsString() string {
	if v.typ != TypeString {
		panic("value is not a string")
	}
	return (*StringObject)(v.obj).value
}

func (v Value) AsSymbol() string {
	if v.typ != TypeSymbol {
		panic("value is not a symbol")
	}
	return (*SymbolObject)(v.obj).value
}

func (v Value) AsObject() *Object {
	if v.typ != TypeObject {
		panic("value is not an object")
	}
	return (*Object)(v.obj)
}

func (v Value) AsPlainObject() *PlainObject {
	if v.typ != TypeObject {
		panic("value is not an object")
	}
	return (*PlainObject)(v.obj)
}

func (v Value) AsDictObject() *DictObject {
	if v.typ != TypeDictObject {
		panic("value is not a dict object")
	}
	return (*DictObject)(v.obj)
}

func (v Value) AsArray() *ArrayObject {
	if v.typ != TypeArray {
		panic("value is not an array")
	}
	return (*ArrayObject)(v.obj)
}

func (v Value) AsFunction() *FunctionObject {
	if v.typ != TypeFunction {
		panic("value is not a function template")
	}
	return (*FunctionObject)(v.obj)
}

func (v Value) AsClosure() *ClosureObject {
	if v.typ != TypeClosure {
		panic("value is not a closure")
	}
	return (*ClosureObject)(v.obj)
}

func (v Value) AsNativeFunction() *NativeFunctionObject {
	if v.typ != TypeNativeFunction {
		panic("value is not a native function")
	}
	return (*NativeFunctionObject)(v.obj)
}

func (v Value) AsBoolean() bool {
	if v.typ != TypeBoolean {
		panic("value is not a boolean")
	}
	return v.payload == 1
}

func (v Value) ToString() string {
	switch v.typ {
	case TypeString:
		return (*StringObject)(v.obj).value
	case TypeSymbol:
		return fmt.Sprintf("Symbol(%s)", (*SymbolObject)(v.obj).value)
	case TypeFloatNumber:
		return strconv.FormatFloat(v.AsFloat(), 'f', -1, 64)
	case TypeIntegerNumber:
		return strconv.FormatInt(int64(v.AsInteger()), 10)
	case TypeBigInt:
		return v.AsBigInt().String() + "n"
	case TypeBoolean:
		if v.AsBoolean() {
			return "true"
		}
		return "false"
	case TypeFunction:
		fn := (*FunctionObject)(v.obj)
		if fn.Name != "" {
			return fmt.Sprintf("<function %s>", fn.Name)
		}
		return "<function>"
	case TypeClosure:
		closure := (*ClosureObject)(v.obj)
		if closure.Fn != nil && closure.Fn.Name != "" {
			return fmt.Sprintf("<closure %s>", closure.Fn.Name)
		}
		return "<closure>"
	case TypeNativeFunction:
		nativeFn := (*NativeFunctionObject)(v.obj)
		if nativeFn.Name != "" {
			return fmt.Sprintf("<native function %s>", nativeFn.Name)
		}
		return "<native function>"
	case TypeObject, TypeDictObject:
		return "[object Object]"
	case TypeArray:
		// JS Array.prototype.toString -> join with commas
		arr := v.AsArray()
		parts := make([]string, len(arr.elements))
		for i, el := range arr.elements {
			parts[i] = el.ToString()
		}
		return strings.Join(parts, ",")
	case TypeNull:
		return "null"
	case TypeUndefined:
		return "undefined"
	}
	return fmt.Sprintf("<unknown type %d>", v.typ)
}

func (v Value) ToFloat() float64 {
	switch v.typ {
	case TypeIntegerNumber:
		return float64(v.AsInteger())
	case TypeFloatNumber:
		return v.AsFloat()
	case TypeBigInt:
		f, _ := v.AsBigInt().Float64()
		return f
	case TypeBoolean:
		if v.AsBoolean() {
			return 1
		}
		return 0
	case TypeString:
		str := strings.TrimSpace(v.AsString())
		if str == "" {
			return 0
		}
		f, err := strconv.ParseFloat(str, 64)
		if err == nil {
			return f
		}
		return math.NaN()
	default:
		return math.NaN()
	}
}

func (v Value) ToInteger() int32 {
	switch v.typ {
	case TypeIntegerNumber:
		return v.AsInteger()
	case TypeFloatNumber:
		f := v.AsFloat()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0
		}
		return int32(f)
	case TypeBigInt:
		if v.AsBigInt().IsInt64() {
			i64 := v.AsBigInt().Int64()
			if i64 >= math.MinInt32 && i64 <= math.MaxInt32 {
				return int32(i64)
			}
		}
		return 0
	case TypeBoolean:
		if v.AsBoolean() {
			return 1
		}
		return 0
	case TypeString:
		str := strings.TrimSpace(v.AsString())
		if str == "" {
			return 0
		}
		i, err := strconv.ParseInt(str, 0, 32)
		if err == nil {
			return int32(i)
		}
		f, err := strconv.ParseFloat(str, 64)
		if err == nil {
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return 0
			}
			return int32(f)
		}
		return 0
	default:
		return 0
	}
}

// Inspect returns a developer-friendly representation of Value, similar to a REPL.
func (v Value) Inspect() string {
	switch v.typ {
	case TypeString:
		return v.AsString()
	case TypeSymbol:
		return fmt.Sprintf("Symbol(%s)", v.AsSymbol())
	case TypeFloatNumber:
		return strconv.FormatFloat(v.AsFloat(), 'f', -1, 64)
	case TypeIntegerNumber:
		return strconv.FormatInt(int64(v.AsInteger()), 10)
	case TypeBigInt:
		return v.AsBigInt().String() + "n"
	case TypeBoolean:
		if v.AsBoolean() {
			return "true"
		}
		return "false"
	case TypeFunction:
		fn := (*FunctionObject)(v.obj)
		if fn.Name != "" {
			return fmt.Sprintf("[Function: %s]", fn.Name)
		}
		return "[Function (anonymous)]"
	case TypeClosure:
		closure := (*ClosureObject)(v.obj)
		if closure.Fn != nil && closure.Fn.Name != "" {
			return fmt.Sprintf("[Function: %s]", closure.Fn.Name)
		}
		return "[Function (anonymous)]"
	case TypeNativeFunction:
		nativeFn := (*NativeFunctionObject)(v.obj)
		if nativeFn.Name != "" {
			return fmt.Sprintf("[Function: %s]", nativeFn.Name)
		}
		return "[Function (anonymous)]"
	case TypeObject:
		// Plain object literal inspect
		obj := v.AsPlainObject()
		var b strings.Builder
		b.WriteString("{")
		for i, field := range obj.shape.fields {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(field.name)
			b.WriteString(": ")
			b.WriteString(obj.properties[i].Inspect())
		}
		b.WriteString("}")
		return b.String()
	case TypeDictObject:
		// Dictionary object literal inspect (sorted keys)
		dict := v.AsDictObject()
		keys := make([]string, 0, len(dict.properties))
		for k := range dict.properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = k + ": " + dict.properties[k].Inspect()
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case TypeArray:
		// Array literal inspect
		arr := v.AsArray()
		elems := make([]string, len(arr.elements))
		for i, el := range arr.elements {
			elems[i] = el.Inspect()
		}
		return "[" + strings.Join(elems, ", ") + "]"
	case TypeNull:
		return "null"
	case TypeUndefined:
		return "undefined"
	default:
		return fmt.Sprintf("<unknown %d>", v.typ)
	}
}

// --- Truthiness ---

var bigZero = big.NewInt(0) // Cache zero BigInt

// IsFalsey checks if the value is considered falsey according to ECMAScript rules.
// null, undefined, false, +0, -0, NaN, 0n, "" are falsey. Everything else is truthy.
func (v Value) IsFalsey() bool {
	switch v.typ {
	case TypeNull, TypeUndefined:
		return true
	case TypeBoolean:
		return !v.AsBoolean()
	case TypeFloatNumber:
		f := v.AsFloat()
		return f == 0 || math.IsNaN(f) // Catches +0, -0, NaN
	case TypeIntegerNumber:
		return v.AsInteger() == 0
	case TypeBigInt:
		return v.AsBigInt().Cmp(bigZero) == 0
	case TypeString:
		return v.AsString() == ""
	case TypeSymbol, TypeObject, TypeArray, TypeFunction, TypeClosure, TypeNativeFunction:
		// All object types (including symbols) are truthy
		return false
	default:
		return true // Unknown types assumed truthy? Or panic? Let's assume truthy.
	}
}

// IsTruthy checks if the value is considered truthy (opposite of IsFalsey).
func (v Value) IsTruthy() bool {
	return !v.IsFalsey()
}

// --- Equality ---

// Is compares two values for equality based on the ECMAScript SameValueZero algorithm.
// NaN === NaN is true, +0 === -0 is true. Useful for collections.
func (v Value) Is(other Value) bool {
	if v.typ != other.typ {
		// Handle cross-Number type comparisons for SameValueZero
		if v.IsNumber() && other.IsNumber() {
			vf := v.ToFloat() // Coerce both to float for comparison
			of := other.ToFloat()
			if math.IsNaN(vf) && math.IsNaN(of) {
				return true // NaN is NaN
			}
			// Float comparison handles +0/-0 correctly
			return vf == of
		}
		// Note: SameValueZero doesn't coerce between BigInt and Number
		return false
	}

	// Types are the same
	switch v.typ {
	case TypeUndefined, TypeNull:
		return true // Singleton types are always equal to themselves
	case TypeBoolean:
		return v.AsBoolean() == other.AsBoolean() // Compare boolean payloads directly
	case TypeIntegerNumber:
		return v.AsInteger() == other.AsInteger() // Compare integer payloads
	case TypeFloatNumber:
		vf := v.AsFloat()
		of := other.AsFloat()
		if math.IsNaN(vf) && math.IsNaN(of) {
			return true // NaN is NaN
		}
		// Standard float comparison handles +0/-0
		return vf == of
	case TypeBigInt:
		return v.AsBigInt().Cmp(other.AsBigInt()) == 0
	case TypeString:
		// String comparison by value
		return v.AsString() == other.AsString()
	case TypeSymbol:
		// Symbols are only equal if they are the *same* object (reference)
		return v.obj == other.obj
	case TypeObject, TypeArray, TypeFunction, TypeClosure, TypeNativeFunction:
		// Objects (including arrays, functions, etc.) are equal only by reference
		return v.obj == other.obj
	default:
		panic(fmt.Sprintf("Unhandled type in Is comparison: %v", v.typ)) // Should not happen
	}
}

// StrictlyEquals compares two values using the ECMAScript Strict Equality Comparison (`===`).
// Types must match, no coercion. NaN !== NaN. +0 === -0.
func (v Value) StrictlyEquals(other Value) bool {
	if v.typ != other.typ {
		return false // Different types are never strictly equal
	}

	// Types are the same
	switch v.typ {
	case TypeUndefined, TypeNull:
		return true // Singleton types are always equal to themselves
	case TypeBoolean:
		return v.AsBoolean() == other.AsBoolean()
	case TypeIntegerNumber:
		return v.AsInteger() == other.AsInteger()
	case TypeFloatNumber:
		vf := v.AsFloat()
		of := other.AsFloat()
		// Strict equality: NaN !== NaN
		if math.IsNaN(vf) || math.IsNaN(of) {
			return false
		}
		// Standard float comparison handles +0/-0 correctly
		return vf == of
	case TypeBigInt:
		return v.AsBigInt().Cmp(other.AsBigInt()) == 0
	case TypeString:
		return v.AsString() == other.AsString()
	case TypeSymbol:
		// Symbols are only equal if they are the *same* object (reference)
		return v.obj == other.obj
	case TypeObject, TypeArray, TypeFunction, TypeClosure, TypeNativeFunction:
		// Objects (including arrays, functions, etc.) are equal only by reference
		return v.obj == other.obj
	default:
		panic(fmt.Sprintf("Unhandled type in StrictlyEquals comparison: %v", v.typ))
	}
}

// Equals compares two values using the ECMAScript Abstract Equality Comparison (`==`).
// Handles type coercion according to the spec (simplified version).
// See: https://tc39.es/ecma262/multipage/abstract-operations.html#sec-abstract-equality-comparison
func (v Value) Equals(other Value) bool {
	// Loop to handle boolean recursion without actual function calls
	for {
		// 1. If Type(x) is the same as Type(y), then return StrictEquality(x, y)
		if v.typ == other.typ {
			return v.StrictlyEquals(other) // Use === logic if types match
		}

		// Check cross-numeric types *before* other coercions
		// (Handles case where boolean was coerced, leading to Number==Number check)
		if v.IsNumber() && other.IsNumber() {
			// Compare Float/Int, Int/Float as numbers (NaN handled by ToFloat)
			return v.ToFloat() == other.ToFloat()
		}
		if v.typ == TypeBigInt && other.IsNumber() {
			return compareBigIntAndNumber(v.AsBigInt(), other)
		}
		if v.IsNumber() && other.typ == TypeBigInt {
			return compareBigIntAndNumber(other.AsBigInt(), v)
		}

		// 2. If x is null and y is undefined, return true.
		// 3. If x is undefined and y is null, return true.
		if (v.typ == TypeNull && other.typ == TypeUndefined) || (v.typ == TypeUndefined && other.typ == TypeNull) {
			return true
		}

		// 4. If Type(x) is Number and Type(y) is String, return x == ToNumber(y).
		// 5. If Type(x) is String and Type(y) is Number, return ToNumber(x) == y.
		if v.IsNumber() && other.typ == TypeString {
			otherNum := other.ToFloat()    // ToNumber(string) -> float
			return v.ToFloat() == otherNum // Handles Int/Float vs String->Float
		}
		if v.typ == TypeString && other.IsNumber() {
			vNum := v.ToFloat()            // ToNumber(string) -> float
			return vNum == other.ToFloat() // Handles String->Float vs Int/Float
		}

		// 6. If Type(x) is BigInt and Type(y) is String, return x == StringToBigInt(y).
		if v.typ == TypeBigInt && other.typ == TypeString {
			otherBig, ok := stringToBigInt(other.AsString())
			if !ok {
				return false
			}
			return v.AsBigInt().Cmp(otherBig) == 0
		}
		// 7. If Type(x) is String and Type(y) is BigInt, return StringToBigInt(x) == y.
		if v.typ == TypeString && other.typ == TypeBigInt {
			vBig, ok := stringToBigInt(v.AsString())
			if !ok {
				return false
			}
			return vBig.Cmp(other.AsBigInt()) == 0
		}

		// 8. If Type(x) is Boolean, return ToNumber(x) == y.
		// 9. If Type(y) is Boolean, return x == ToNumber(y).
		// Use loop instead of recursion: convert boolean and restart comparison
		if v.typ == TypeBoolean {
			v = NumberValue(v.ToFloat()) // Convert v to Number(0 or 1)
			continue                     // Restart loop with converted v
		}
		if other.typ == TypeBoolean {
			other = NumberValue(other.ToFloat()) // Convert other to Number(0 or 1)
			continue                             // Restart loop with converted other
		}

		// 10. If Type(x) is Object and Type(y) is String/Number/BigInt/Symbol, return ToPrimitive(x) == y.
		// 11. If Type(y) is Object and Type(x) is String/Number/BigInt/Symbol, return x == ToPrimitive(y).
		// (Skipping ToPrimitive for now)

		// 12. BigInt/Number handled earlier

		// 13. Return false
		return false
	}
}

// --- VM API Helpers ---
// IsNumber returns true if the value is a JS Number (float or integer).
func IsNumber(v Value) bool { return v.IsNumber() }

// AsNumber returns the numeric value (float64) of a Number value.
// For integer numbers, it converts to float64.
func AsNumber(v Value) float64 { return v.ToFloat() }

// Number creates a Number value from float64.
func Number(f float64) Value { return NumberValue(f) }

// IsString reports whether the value is a String.
func IsString(v Value) bool { return v.IsString() }

// AsString returns the Go string of a String value.
func AsString(v Value) string { return v.AsString() }

// String creates a String value.
func String(s string) Value { return NewString(s) }

// isFalsey returns true if the value is considered falsey.
func isFalsey(v Value) bool { return v.IsFalsey() }

// AsClosure returns the ClosureObject pointer from a Closure value.
func AsClosure(v Value) *ClosureObject { return v.AsClosure() }

// AsPlainObject returns the PlainObject pointer from an Object value.
func AsPlainObject(v Value) *PlainObject { return v.AsPlainObject() }

// AsDictObject returns the DictObject pointer from a DictObject value.
func AsDictObject(v Value) *DictObject { return v.AsDictObject() }

// AsArray returns the ArrayObject pointer from an Array value.
func AsArray(v Value) *ArrayObject { return v.AsArray() }

// valuesEqual compares two values using ECMAScript SameValueZero (NaN===NaN, +0===-0).
func valuesEqual(a, b Value) bool { return a.Is(b) }

// valuesStrictEqual compares two values using ECMAScript Strict Equality (===).
func valuesStrictEqual(a, b Value) bool { return a.StrictlyEquals(b) }

// AsNativeFunction returns the NativeFunctionObject pointer from a native function value.
func AsNativeFunction(v Value) *NativeFunctionObject { return v.AsNativeFunction() }

// IsFunction reports whether the value is a function (FunctionObject or ClosureObject or NativeFunctionObject).
func IsFunction(v Value) bool { return v.IsFunction() }

// AsFunction returns the FunctionObject pointer from a function template value.
func AsFunction(v Value) *FunctionObject { return v.AsFunction() }

// Helper for BigInt == String coercion - Now stricter
func stringToBigInt(s string) (*big.Int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false // Empty string is not a valid BigInt for == comparison (unlike ToNumber)
	}
	// Use SetString with base 0 to detect 0x/0b/0o prefixes
	i := new(big.Int)
	_, ok := i.SetString(s, 0) // base 0 auto-detects prefix
	if !ok {
		// Check if it might look like a float (decimal/exponent) - these are invalid for BigInt string comparison
		if strings.ContainsAny(s, ".eE") {
			return nil, false
		}
		// It might be just non-numeric
		return nil, false
	}
	// SetString succeeded, return the parsed BigInt
	return i, true
}

// Helper for BigInt == Number comparison
func compareBigIntAndNumber(bi *big.Int, numVal Value) bool {
	if numVal.IsIntegerNumber() {
		return bi.Cmp(big.NewInt(int64(numVal.AsInteger()))) == 0
	} else if numVal.IsFloatNumber() {
		f := numVal.AsFloat()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return false
		}
		// Use big.Float comparison for better accuracy
		bf := new(big.Float).SetInt(bi)
		otherBf := new(big.Float).SetFloat64(f)
		return bf.Cmp(otherBf) == 0 // Compare using big.Float
	}
	return false
}

// Length returns the length of the array
func (a *ArrayObject) Length() int {
	return a.length
}

// SetLength sets the length of the array, expanding or truncating as needed
func (a *ArrayObject) SetLength(newLength int) {
	if newLength < 0 {
		newLength = 0
	}

	if newLength < len(a.elements) {
		// Truncate array
		a.elements = a.elements[:newLength]
	} else if newLength > len(a.elements) {
		// Expand array with undefined values
		for i := len(a.elements); i < newLength; i++ {
			a.elements = append(a.elements, Undefined)
		}
	}
	a.length = newLength
}

// Get returns the element at the given index, or Undefined if out of bounds
func (a *ArrayObject) Get(index int) Value {
	if index < 0 || index >= len(a.elements) {
		return Undefined
	}
	return a.elements[index]
}

// Set sets the element at the given index, expanding the array if necessary
func (a *ArrayObject) Set(index int, value Value) {
	if index < 0 {
		return // Ignore negative indices
	}

	// Expand array if necessary
	if index >= len(a.elements) {
		for i := len(a.elements); i <= index; i++ {
			a.elements = append(a.elements, Undefined)
		}
	}

	a.elements[index] = value
	if index >= a.length {
		a.length = index + 1
	}
}

// SetElements sets all elements at once and updates length
func (a *ArrayObject) SetElements(elements []Value) {
	a.elements = make([]Value, len(elements))
	copy(a.elements, elements)
	a.length = len(elements)
}

// Append adds a value to the end of the array
func (a *ArrayObject) Append(value Value) {
	a.elements = append(a.elements, value)
	a.length++
}

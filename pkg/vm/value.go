package vm

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
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
	TypeNativeFunctionWithProps
	TypeAsyncNativeFunction
	TypeClosure
	TypeBoundFunction

	TypeObject
	TypeDictObject

	TypeArray
	TypeArguments
	TypeGenerator
	TypeRegExp
	TypeMap
	TypeSet
	TypeArrayBuffer
	TypeTypedArray
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

type ArgumentsObject struct {
	Object
	length int
	args   []Value
}

// GeneratorState represents the execution state of a generator
type GeneratorState int

const (
	GeneratorSuspendedStart GeneratorState = iota // Initial state, not yet started
	GeneratorSuspendedYield                       // Suspended at a yield expression
	GeneratorExecuting                            // Currently executing
	GeneratorCompleted                            // Completed (returned or threw)
)

// GeneratorFrame stores the execution state of a suspended generator
// This allows the generator to resume execution from where it left off
type GeneratorFrame struct {
	pc        int       // Program counter - next instruction to execute
	registers []Value   // Register state at suspension point
	locals    []Value   // Local variable state
	stackBase int       // Base of this frame's stack
	yieldPC   int       // PC of the yield instruction (for resumption)
}

// GeneratorObject represents a JavaScript generator instance
// Based on the design from generators-implementation-plan.md
type GeneratorObject struct {
	Object
	Function     Value              // The generator function
	State        GeneratorState     // Current state (suspended/completed/executing)
	Frame        *GeneratorFrame    // Execution frame (nil if completed)
	YieldedValue Value              // Last yielded value
	ReturnValue  Value              // Final return value (when completed)
	Done         bool               // True when generator is exhausted
	Args         []Value            // Arguments passed when the generator was created
}

type MapObject struct {
	Object
	size    int
	entries map[string]Value      // key -> value
	keys    map[string]Value      // key -> original key (for key iteration)
}

type SetObject struct {
	Object
	size   int
	values map[string]Value  // key -> original value (for value iteration)
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

func NewArguments(args []Value) Value {
	argObj := &ArgumentsObject{
		length: len(args),
		args:   make([]Value, len(args)),
	}
	copy(argObj.args, args)
	return Value{typ: TypeArguments, obj: unsafe.Pointer(argObj)}
}

// NewGenerator creates a new generator object with the given generator function
func NewGenerator(function Value) Value {
	genObj := &GeneratorObject{
		Function:     function,
		State:        GeneratorSuspendedStart,
		Frame:        nil, // Will be created when generator starts
		YieldedValue: Undefined,
		ReturnValue:  Undefined,
		Done:         false,
	}
	return Value{typ: TypeGenerator, obj: unsafe.Pointer(genObj)}
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

func NewMap() Value {
	mapObj := &MapObject{
		size:    0,
		entries: make(map[string]Value),
		keys:    make(map[string]Value),
	}
	return Value{typ: TypeMap, obj: unsafe.Pointer(mapObj)}
}

func NewSet() Value {
	setObj := &SetObject{
		size:   0,
		values: make(map[string]Value),
	}
	return Value{typ: TypeSet, obj: unsafe.Pointer(setObj)}
}

// hashKey creates a unique string key for any JavaScript value
// Uses SameValueZero equality (NaN === NaN, +0 === -0)
func hashKey(v Value) string {
	switch v.Type() {
	case TypeNull:
		return "null"
	case TypeUndefined:
		return "undefined"
	case TypeString:
		return "s:" + v.ToString()
	case TypeBoolean:
		if v.AsBoolean() {
			return "b:true"
		}
		return "b:false"
	case TypeFloatNumber, TypeIntegerNumber:
		f := v.ToFloat()
		if math.IsNaN(f) {
			return "n:NaN"
		}
		if f == 0 {
			return "n:0" // Treat +0 and -0 as same (SameValueZero)
		}
		return "n:" + strconv.FormatFloat(f, 'g', -1, 64)
	default:
		// For objects, use pointer address as unique key
		return "o:" + fmt.Sprintf("%p", v.obj)
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
	return v.typ == TypeObject || v.typ == TypeDictObject || v.typ == TypeArray || v.typ == TypeArguments || v.typ == TypeGenerator || v.typ == TypeRegExp
}

func (v Value) IsDictObject() bool {
	return v.typ == TypeDictObject
}

func (v Value) IsArray() bool {
	return v.typ == TypeArray
}

func (v Value) IsArguments() bool {
	return v.typ == TypeArguments
}

func (v Value) IsGenerator() bool {
	return v.typ == TypeGenerator
}

func (v Value) IsCallable() bool {
	return v.typ == TypeFunction || v.typ == TypeNativeFunction || v.typ == TypeNativeFunctionWithProps || v.typ == TypeClosure || v.typ == TypeBoundFunction
}

func (v Value) IsFunction() bool {
	return v.typ == TypeFunction
}

func (v Value) IsClosure() bool {
	return v.typ == TypeClosure
}

func (v Value) IsNativeFunction() bool {
	return v.typ == TypeNativeFunction || v.typ == TypeNativeFunctionWithProps || v.typ == TypeAsyncNativeFunction
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
	case TypeFunction, TypeClosure, TypeNativeFunction, TypeNativeFunctionWithProps, TypeAsyncNativeFunction:
		return "function"
	case TypeObject, TypeDictObject, TypeArray, TypeArguments, TypeRegExp:
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

func (v Value) AsArguments() *ArgumentsObject {
	if v.typ != TypeArguments {
		panic("value is not an arguments object")
	}
	return (*ArgumentsObject)(v.obj)
}

func (v Value) AsGenerator() *GeneratorObject {
	if v.typ != TypeGenerator {
		panic("value is not a generator")
	}
	return (*GeneratorObject)(v.obj)
}

func (v Value) AsMap() *MapObject {
	if v.typ != TypeMap {
		panic("value is not a map")
	}
	return (*MapObject)(v.obj)
}

func (v Value) AsSet() *SetObject {
	if v.typ != TypeSet {
		panic("value is not a set")
	}
	return (*SetObject)(v.obj)
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

func (v Value) AsNativeFunctionWithProps() *NativeFunctionObjectWithProps {
	if v.typ != TypeNativeFunctionWithProps {
		panic("value is not a native function with props")
	}
	return (*NativeFunctionObjectWithProps)(v.obj)
}

func (v Value) AsAsyncNativeFunction() *AsyncNativeFunctionObject {
	if v.typ != TypeAsyncNativeFunction {
		panic("value is not an async native function")
	}
	return (*AsyncNativeFunctionObject)(v.obj)
}

func (v Value) AsBoundFunction() *BoundFunctionObject {
	if v.typ != TypeBoundFunction {
		panic("value is not a bound function")
	}
	return (*BoundFunctionObject)(v.obj)
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
	case TypeNativeFunctionWithProps:
		nativeFn := (*NativeFunctionObjectWithProps)(v.obj)
		if nativeFn.Name != "" {
			return fmt.Sprintf("<native function %s>", nativeFn.Name)
		}
		return "<native function>"
	case TypeAsyncNativeFunction:
		asyncFn := (*AsyncNativeFunctionObject)(v.obj)
		if asyncFn.Name != "" {
			return fmt.Sprintf("<async native function %s>", asyncFn.Name)
		}
		return "<async native function>"
	case TypeBoundFunction:
		boundFn := (*BoundFunctionObject)(v.obj)
		if boundFn.Name != "" {
			return fmt.Sprintf("<bound function %s>", boundFn.Name)
		}
		return "<bound function>"
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
	case TypeArguments:
		// Arguments object toString -> [object Arguments]
		return "[object Arguments]"
	case TypeGenerator:
		// Generator object toString -> [object Generator]
		return "[object Generator]"
	case TypeNull:
		return "null"
	case TypeUndefined:
		return "undefined"
	case TypeRegExp:
		regex := v.AsRegExpObject()
		if regex != nil {
			return "/" + regex.source + "/" + regex.flags
		}
		return "/(?:)/"
	case TypeArrayBuffer:
		return "[object ArrayBuffer]"
	case TypeTypedArray:
		ta := v.AsTypedArray()
		if ta != nil {
			switch ta.elementType {
			case TypedArrayInt8:
				return "[object Int8Array]"
			case TypedArrayUint8:
				return "[object Uint8Array]"
			case TypedArrayUint8Clamped:
				return "[object Uint8ClampedArray]"
			case TypedArrayInt16:
				return "[object Int16Array]"
			case TypedArrayUint16:
				return "[object Uint16Array]"
			case TypedArrayInt32:
				return "[object Int32Array]"
			case TypedArrayUint32:
				return "[object Uint32Array]"
			case TypedArrayFloat32:
				return "[object Float32Array]"
			case TypedArrayFloat64:
				return "[object Float64Array]"
			default:
				return "[object TypedArray]"
			}
		}
		return "[object TypedArray]"
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
	case TypeObject, TypeDictObject, TypeArray, TypeArguments, TypeRegExp, TypeMap, TypeSet, TypeArrayBuffer, TypeTypedArray:
		// Special case for Date objects - directly get timestamp
		if obj := v.AsPlainObject(); obj != nil {
			if timestampValue, exists := obj.GetOwn("__timestamp__"); exists {
				return timestampValue.ToFloat()
			}
		}
		// For other objects, try to convert to primitive using valueOf
		if prim := v.ToPrimitive("number"); prim.typ != v.typ {
			// Successfully converted to primitive, now convert that to number
			return prim.ToFloat()
		}
		return math.NaN()
	default:
		return math.NaN()
	}
}

// ToPrimitive converts a value to a primitive type following ECMAScript specification
// hint can be "number", "string", or "default"
func (v Value) ToPrimitive(hint string) Value {
	// If already primitive, return as-is
	if !v.IsObject() && v.typ != TypeArray && v.typ != TypeArguments && v.typ != TypeRegExp && v.typ != TypeMap && v.typ != TypeSet {
		return v
	}

	// For objects, we don't have proper valueOf/toString calling implemented yet
	// So just return the original value. Date objects are handled specially in ToFloat()
	return v
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
		// Proper 32-bit wrapping instead of clamping
		// Convert to int64 first to avoid overflow, then wrap to 32-bit range
		i64 := int64(f)
		// Use bitwise AND to wrap to 32-bit range (equivalent to modulo 2^32 for unsigned)
		// Then convert to signed int32
		return int32(uint32(i64))
	case TypeBigInt:
		if v.AsBigInt().IsInt64() {
			i64 := v.AsBigInt().Int64()
			// Apply the same wrapping logic for BigInt
			return int32(uint32(i64))
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
			// Apply the same wrapping logic for string-parsed floats
			i64 := int64(f)
			return int32(uint32(i64))
		}
		return 0
	default:
		return 0
	}
}

// Inspect returns a developer-friendly representation of Value, similar to a REPL.
func (v Value) Inspect() string {
	return v.inspectWithContext(false) // Top-level call, strings unquoted
}

// InspectNested is used for nested contexts where strings should be quoted
func (v Value) InspectNested() string {
	return v.inspectWithContext(true) // Nested call, strings quoted
}

func (v Value) inspectWithContext(nested bool) string {
	switch v.typ {
	case TypeString:
		if nested {
			// In nested context (like inside objects/arrays), quote the string
			return fmt.Sprintf(`"%s"`, v.AsString())
		}
		// At top level, return unquoted string
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
	case TypeNativeFunctionWithProps:
		nativeFn := (*NativeFunctionObjectWithProps)(v.obj)
		if nativeFn.Name != "" {
			return fmt.Sprintf("[Function: %s]", nativeFn.Name)
		}
		return "[Function (anonymous)]"
	case TypeAsyncNativeFunction:
		asyncFn := (*AsyncNativeFunctionObject)(v.obj)
		if asyncFn.Name != "" {
			return fmt.Sprintf("[Function: %s]", asyncFn.Name)
		}
		return "[Function (anonymous)]"
	case TypeObject:
		// Plain object literal inspect - check for toString() first
		obj := v.AsPlainObject()
		
		// Fast path: Check for special built-in objects with known patterns
		if toStringResult := tryBuiltinToString(obj); toStringResult != "" {
			// In nested context, quote the toString result like a string
			if nested {
				return fmt.Sprintf(`"%s"`, toStringResult)
			}
			return toStringResult
		}
		
		// General path: Look for toString method in prototype chain
		if toStringMethod := findToStringMethod(obj); toStringMethod.Type() != TypeUndefined && toStringMethod.IsFunction() {
			// For now, we'll handle specific cases rather than calling the method
			// to avoid VM state complications in the inspect method
			if dateString := tryFormatAsDate(obj); dateString != "" {
				// In nested context, quote the date string like a string
				if nested {
					return fmt.Sprintf(`"%s"`, dateString)
				}
				return dateString
			}
		}
		
		// Default object literal representation
		var b strings.Builder
		b.WriteString("{")
		for i, field := range obj.shape.fields {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(field.name)
			b.WriteString(": ")
			b.WriteString(obj.properties[i].InspectNested()) // Use nested context
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
			parts[i] = k + ": " + dict.properties[k].InspectNested() // Use nested context
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case TypeArray:
		// Array literal inspect
		arr := v.AsArray()
		elems := make([]string, len(arr.elements))
		for i, el := range arr.elements {
			elems[i] = el.InspectNested() // Use nested context
		}
		return "[" + strings.Join(elems, ", ") + "]"
	case TypeArguments:
		// Arguments object inspect - show like array but with [Arguments] tag
		args := v.AsArguments()
		elems := make([]string, len(args.args))
		for i, el := range args.args {
			elems[i] = el.InspectNested() // Use nested context
		}
		return "[Arguments] { 0: " + strings.Join(elems, ", ") + " }"
	case TypeMap:
		// Map object inspect - show as Map { key => value, ... }
		mapObj := v.AsMap()
		if mapObj.Size() == 0 {
			return "Map {}"
		}
		var parts []string
		for keyStr, value := range mapObj.entries {
			key := mapObj.keys[keyStr]
			parts = append(parts, key.InspectNested()+" => "+value.InspectNested())
		}
		return "Map { " + strings.Join(parts, ", ") + " }"
	case TypeSet:
		// Set object inspect - show as Set { value1, value2, ... }
		setObj := v.AsSet()
		if setObj.Size() == 0 {
			return "Set {}"
		}
		var parts []string
		for _, value := range setObj.values {
			parts = append(parts, value.InspectNested())
		}
		return "Set { " + strings.Join(parts, ", ") + " }"
	case TypeNull:
		return "null"
	case TypeUndefined:
		return "undefined"
	case TypeRegExp:
		regex := v.AsRegExpObject()
		if regex != nil {
			return "/" + regex.source + "/" + regex.flags
		}
		return "/(?:)/"
	case TypeArrayBuffer:
		buffer := v.AsArrayBuffer()
		if buffer != nil {
			return fmt.Sprintf("ArrayBuffer { [Uint8Contents]: <%d bytes> }", len(buffer.data))
		}
		return "ArrayBuffer {}"
	case TypeTypedArray:
		ta := v.AsTypedArray()
		if ta != nil {
			typeName := ""
			switch ta.elementType {
			case TypedArrayInt8:
				typeName = "Int8Array"
			case TypedArrayUint8:
				typeName = "Uint8Array"
			case TypedArrayUint8Clamped:
				typeName = "Uint8ClampedArray"
			case TypedArrayInt16:
				typeName = "Int16Array"
			case TypedArrayUint16:
				typeName = "Uint16Array"
			case TypedArrayInt32:
				typeName = "Int32Array"
			case TypedArrayUint32:
				typeName = "Uint32Array"
			case TypedArrayFloat32:
				typeName = "Float32Array"
			case TypedArrayFloat64:
				typeName = "Float64Array"
			default:
				typeName = "TypedArray"
			}
			return fmt.Sprintf("%s { length: %d }", typeName, ta.length)
		}
		return "TypedArray {}"
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
	case TypeSymbol, TypeObject, TypeArray, TypeArguments, TypeFunction, TypeClosure, TypeNativeFunction, TypeRegExp:
		// All object types (including symbols and regex) are truthy
		return false
	default:
		return true // Unknown types assumed truthy? Or panic? Let's assume truthy.
	}
}

// IsTruthy checks if the value is considered truthy (opposite of IsFalsey).
func (v Value) IsTruthy() bool {
	return !v.IsFalsey()
}

// IsUndefined checks if the value is undefined
func (v Value) IsUndefined() bool {
	return v.typ == TypeUndefined
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
	case TypeObject, TypeArray, TypeArguments, TypeFunction, TypeClosure, TypeNativeFunction, TypeNativeFunctionWithProps, TypeBoundFunction, TypeRegExp, TypeMap, TypeSet:
		// Objects (including arrays, functions, regex, maps, sets, etc.) are equal only by reference
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
	case TypeObject, TypeArray, TypeArguments, TypeFunction, TypeClosure, TypeNativeFunction, TypeNativeFunctionWithProps, TypeBoundFunction, TypeRegExp, TypeMap, TypeSet:
		// Objects (including arrays, functions, regex, maps, sets, etc.) are equal only by reference
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

// AsArguments returns the ArgumentsObject pointer from an Arguments value.
func AsArguments(v Value) *ArgumentsObject { return v.AsArguments() }
func AsMap(v Value) *MapObject { return v.AsMap() }
func AsSet(v Value) *SetObject { return v.AsSet() }

// valuesEqual compares two values using ECMAScript SameValueZero (NaN===NaN, +0===-0).
func valuesEqual(a, b Value) bool { return a.Is(b) }

// valuesStrictEqual compares two values using ECMAScript Strict Equality (===).
func valuesStrictEqual(a, b Value) bool { return a.StrictlyEquals(b) }

// AsNativeFunction returns the NativeFunctionObject pointer from a native function value.
func AsNativeFunction(v Value) *NativeFunctionObject { return v.AsNativeFunction() }

// AsBoundFunction returns the BoundFunctionObject pointer from a bound function value.
func AsBoundFunction(v Value) *BoundFunctionObject { return v.AsBoundFunction() }

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

// ArgumentsObject methods
func (a *ArgumentsObject) Length() int {
	return a.length
}

func (a *ArgumentsObject) Get(index int) Value {
	if index < 0 || index >= len(a.args) {
		return Undefined
	}
	return a.args[index]
}

func (a *ArgumentsObject) Set(index int, value Value) {
	if index < 0 || index >= len(a.args) {
		return // Arguments object doesn't expand like arrays
	}
	a.args[index] = value
}

// MapObject methods
func (m *MapObject) Set(key, value Value) {
	keyStr := hashKey(key)
	if _, exists := m.entries[keyStr]; !exists {
		m.size++
	}
	m.entries[keyStr] = value
	m.keys[keyStr] = key
}

func (m *MapObject) Get(key Value) Value {
	keyStr := hashKey(key)
	if value, exists := m.entries[keyStr]; exists {
		return value
	}
	return Undefined
}

func (m *MapObject) Has(key Value) bool {
	keyStr := hashKey(key)
	_, exists := m.entries[keyStr]
	return exists
}

func (m *MapObject) Delete(key Value) bool {
	keyStr := hashKey(key)
	if _, exists := m.entries[keyStr]; exists {
		delete(m.entries, keyStr)
		delete(m.keys, keyStr)
		m.size--
		return true
	}
	return false
}

func (m *MapObject) Clear() {
	m.entries = make(map[string]Value)
	m.keys = make(map[string]Value)
	m.size = 0
}

func (m *MapObject) Size() int {
	return m.size
}

// SetObject methods
func (s *SetObject) Add(value Value) {
	keyStr := hashKey(value)
	if _, exists := s.values[keyStr]; !exists {
		s.size++
	}
	s.values[keyStr] = value
}

func (s *SetObject) Has(value Value) bool {
	keyStr := hashKey(value)
	_, exists := s.values[keyStr]
	return exists
}

func (s *SetObject) Delete(value Value) bool {
	keyStr := hashKey(value)
	if _, exists := s.values[keyStr]; exists {
		delete(s.values, keyStr)
		s.size--
		return true
	}
	return false
}

func (s *SetObject) Clear() {
	s.values = make(map[string]Value)
	s.size = 0
}

func (s *SetObject) Size() int {
	return s.size
}

// NewValueFromPlainObject creates a Value from a PlainObject pointer
// This is useful for returning prototype objects from built-in functions
func NewValueFromPlainObject(plainObj *PlainObject) Value {
	return Value{typ: TypeObject, obj: unsafe.Pointer(plainObj)}
}

// GetArity returns the arity (number of parameters) for callable values
func (v Value) GetArity() int {
	switch v.typ {
	case TypeFunction:
		return v.AsFunction().Arity
	case TypeClosure:
		return v.AsClosure().Fn.Arity
	case TypeNativeFunction:
		return v.AsNativeFunction().Arity
	case TypeNativeFunctionWithProps:
		return v.AsNativeFunctionWithProps().Arity
	case TypeAsyncNativeFunction:
		return v.AsAsyncNativeFunction().Arity
	case TypeBoundFunction:
		boundFn := v.AsBoundFunction()
		originalArity := boundFn.OriginalFunction.GetArity()
		newArity := originalArity - len(boundFn.PartialArgs)
		if newArity < 0 {
			newArity = 0
		}
		return newArity
	default:
		panic("value is not callable")
	}
}

// Helper functions for toString() method resolution and built-in object formatting

// tryBuiltinToString checks for specific built-in object patterns and formats them
func tryBuiltinToString(obj *PlainObject) string {
	// Check for Date objects with __timestamp__ property
	if timestampValue, exists := obj.GetOwn("__timestamp__"); exists && timestampValue.IsNumber() {
		return formatDateTimestamp(timestampValue.ToFloat())
	}
	
	// Add other built-in object patterns here as needed
	// e.g., RegExp, Error objects, etc.
	
	return ""
}

// findToStringMethod looks for toString method in the prototype chain
func findToStringMethod(obj *PlainObject) Value {
	// Check own properties first
	if toStringMethod, exists := obj.GetOwn("toString"); exists {
		return toStringMethod
	}
	
	// Walk the prototype chain
	current := obj
	depth := 0
	
	for current != nil && depth < 10 { // Prevent infinite loops
		// Move up the prototype chain
		protoVal := current.GetPrototype()
		if !protoVal.IsObject() {
			break
		}
		
		current = protoVal.AsPlainObject()
		if current != nil {
			if toStringMethod, exists := current.GetOwn("toString"); exists {
				return toStringMethod
			}
		}
		depth++
	}
	
	return Undefined
}

// tryFormatAsDate attempts to format an object as a Date if it has the right structure
func tryFormatAsDate(obj *PlainObject) string {
	if timestampValue, exists := obj.GetOwn("__timestamp__"); exists && timestampValue.IsNumber() {
		return formatDateTimestamp(timestampValue.ToFloat())
	}
	return ""
}

// formatDateTimestamp formats a timestamp like JavaScript Date.toString()
func formatDateTimestamp(timestamp float64) string {
	// Use Go's time package to format the timestamp
	// This should match the format used in date_init.go
	t := time.UnixMilli(int64(timestamp))
	return t.Format("Mon Jan 02 2006 15:04:05 GMT-0700 (MST)")
}

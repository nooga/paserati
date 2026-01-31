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
	"weak"
)

// cleanExponentialFormat removes leading zeros from exponent to match JS format
// e.g., "1e-07" -> "1e-7", "1e+25" -> "1e+25"
func cleanExponentialFormat(s string) string {
	// Find the 'e' or 'E'
	for i := 0; i < len(s); i++ {
		if s[i] == 'e' || s[i] == 'E' {
			// Check if next char is + or -
			if i+1 < len(s) && (s[i+1] == '+' || s[i+1] == '-') {
				sign := s[i+1]
				// Remove leading zeros from exponent
				expStart := i + 2
				j := expStart
				for j < len(s) && s[j] == '0' {
					j++
				}
				// If all zeros or no digits after sign, keep one zero
				if j >= len(s) {
					return s[:i+2] + "0"
				}
				// Reconstruct: mantissa + e + sign + trimmed exponent
				return s[:i+1] + string(sign) + s[j:]
			}
			break
		}
	}
	return s
}

type ValueType uint8

const (
	TypeUndefined ValueType = iota
	TypeNull

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
	TypeAsyncGenerator
	TypePromise
	TypeRegExp
	TypeMap
	TypeSet
	TypeWeakMap
	TypeWeakSet
	TypeArrayBuffer
	TypeTypedArray
	TypeProxy
	TypeHole          // Internal marker for array holes (sparse arrays)
	TypeUninitialized // TDZ marker for let/const before initialization
)

// String returns a human-readable string representation of the ValueType
func (vt ValueType) String() string {
	switch vt {
	case TypeNull:
		return "null"
	case TypeUndefined:
		return "undefined"
	case TypeString:
		return "string"
	case TypeSymbol:
		return "symbol"
	case TypeFloatNumber, TypeIntegerNumber:
		return "number"
	case TypeBigInt:
		return "bigint"
	case TypeBoolean:
		return "boolean"
	case TypeFunction:
		return "function"
	case TypeNativeFunction, TypeNativeFunctionWithProps, TypeAsyncNativeFunction:
		return "native function"
	case TypeClosure:
		return "closure"
	case TypeBoundFunction:
		return "bound function"
	case TypeObject:
		return "object"
	case TypeDictObject:
		return "dict object"
	case TypeArray:
		return "array"
	case TypeArguments:
		return "arguments"
	case TypeGenerator:
		return "generator"
	case TypeAsyncGenerator:
		return "async generator"
	case TypePromise:
		return "promise"
	case TypeRegExp:
		return "regexp"
	case TypeMap:
		return "map"
	case TypeSet:
		return "set"
	case TypeWeakMap:
		return "weakmap"
	case TypeWeakSet:
		return "weakset"
	case TypeArrayBuffer:
		return "arraybuffer"
	case TypeTypedArray:
		return "typed array"
	case TypeProxy:
		return "proxy"
	default:
		return "unknown"
	}
}

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
	length       int
	elements     []Value
	properties   map[string]Value           // Named properties (e.g., "index", "input" for match results)
	propertyDesc map[string]PropertyDesc    // Property descriptors for named properties
	symbolProps  map[*SymbolObject]Value    // Symbol-keyed properties (e.g., Symbol.iterator override)
	extensible   bool                       // When false, no new properties can be added (for Object.freeze/seal)
	frozen       bool                       // When true, elements are also non-writable and non-configurable
}

// PropertyDesc stores property descriptor attributes
type PropertyDesc struct {
	Writable     bool
	Enumerable   bool
	Configurable bool
}

type ArgumentsObject struct {
	Object
	length      int
	args        []Value
	callee      Value                   // The function that created this arguments object
	isStrict    bool                    // Whether the arguments object is from strict mode code
	symbolProps map[*SymbolObject]Value // Symbol-keyed properties (e.g., Symbol.iterator)
	namedProps  map[string]Value        // Overflow storage for arbitrary named properties
	mappedRegs  []Value                 // Shared slice into frame registers for mapped arguments (sloppy mode)
	numMapped   int                     // Number of mapped parameters (0 = unmapped)
}

// GeneratorState represents the execution state of a generator
// This allows the generator to resume execution from where it left off
type GeneratorState int

const (
	GeneratorStart          GeneratorState = iota // Just created, prologue not yet executed
	GeneratorSuspendedStart                       // Prologue executed, ready for first .next()
	GeneratorSuspendedYield                       // Suspended at a yield expression
	GeneratorExecuting                            // Currently executing
	GeneratorCompleted                            // Completed (returned or threw)
)

// String returns a human-readable name for the generator state
func (gs GeneratorState) String() string {
	switch gs {
	case GeneratorStart:
		return "Start"
	case GeneratorSuspendedStart:
		return "SuspendedStart"
	case GeneratorSuspendedYield:
		return "SuspendedYield"
	case GeneratorExecuting:
		return "Executing"
	case GeneratorCompleted:
		return "Completed"
	default:
		return "Unknown"
	}
}

// GeneratorFrame stores the execution state of a suspended generator
// This allows the generator to resume execution from where it left off
// SuspendedFrame stores execution state when a function is suspended
// Used by both generators (yield) and async functions (await)
type SuspendedFrame struct {
	pc         int     // Program counter - next instruction to execute
	registers  []Value // Register state at suspension point
	locals     []Value // Local variable state
	stackBase  int     // Base of this frame's stack
	suspendPC  int     // PC of the suspend instruction (yield/await) for resumption
	outputReg  byte    // Register where sent/resolved value should be stored on resumption
	thisValue  Value   // The 'this' value for this frame (must be preserved across suspensions)
	homeObject Value   // The [[HomeObject]] for super property access (must be preserved for object literal methods)
}

// GeneratorFrame is an alias for backwards compatibility
type GeneratorFrame = SuspendedFrame

// GeneratorObject represents a JavaScript generator instance
// Based on the design from generators-implementation-plan.md
type GeneratorObject struct {
	Object
	Function          Value           // The generator function
	State             GeneratorState  // Current state (suspended/completed/executing)
	Frame             *SuspendedFrame // Execution frame (nil if completed)
	YieldedValue      Value           // Last yielded value
	ReturnValue       Value           // Final return value (when completed)
	Done              bool            // True when generator is exhausted
	Args              []Value         // Arguments passed when the generator was created
	This              Value           // The 'this' value for the generator context
	Prototype         *PlainObject    // Custom prototype (if set via function.prototype)
	DelegatedIterator     Value // Iterator being delegated to (for yield* forwarding of .return()/.throw())
	DelegationResult      Value // Result value when delegation completed via external throw/return with done:true
	DelegationResultReady bool  // Flag indicating DelegationResult is set (needed because result could be undefined)
}

type AsyncGeneratorObject GeneratorObject

type MapObject struct {
	Object
	size       int
	entries    map[string]Value // key -> value
	keys       map[string]Value // key -> original key (for key iteration)
	order      []string         // insertion order of keys (for deterministic iteration)
	tombstones map[string]bool  // keys that were deleted but still in order (for live iteration)
	Properties *PlainObject     // User-defined properties on the Map object
	prototype  Value            // Map prototype
}

type SetObject struct {
	Object
	size       int
	values     map[string]Value // key -> original value (for value iteration)
	order      []string         // insertion order of values (for deterministic iteration)
	tombstones map[string]bool  // values that were deleted but still in order (for live iteration)
	Properties *PlainObject     // User-defined properties on the Set object
	prototype  Value            // Set prototype
}

// WeakMapEntry holds a weak reference to the key and a strong reference to the value
type WeakMapEntry struct {
	keyWeak weak.Pointer[byte] // Weak reference to check if key is still alive
	value   Value              // Strong reference to the value
}

// WeakMapObject implements ECMAScript WeakMap using Go's weak package.
// Keys must be objects (not primitives) and are held weakly, allowing GC.
type WeakMapObject struct {
	Object
	entries map[uintptr]*WeakMapEntry // pointer address -> entry
}

// WeakSetEntry holds a weak reference to a value
type WeakSetEntry struct {
	valueWeak weak.Pointer[byte] // Weak reference to check if value is still alive
}

// WeakSetObject implements ECMAScript WeakSet using Go's weak package.
// Values must be objects and are held weakly, allowing GC.
type WeakSetObject struct {
	Object
	entries map[uintptr]*WeakSetEntry // pointer address -> entry
}

type ProxyObject struct {
	Object
	target  Value // The target object being proxied
	handler Value // The handler object with traps
	Revoked bool  // Whether the proxy has been revoked
}

// Target returns the proxy's target object
func (p *ProxyObject) Target() Value {
	return p.target
}

// Handler returns the proxy's handler object
func (p *ProxyObject) Handler() Value {
	return p.handler
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

// NewTypeError constructs a TypeError exception error for builtin helpers to return
func (vm *VM) NewTypeError(message string) error {
	ctor, _ := vm.GetGlobal("TypeError")
	if ctor != Undefined {
		errObj, _ := vm.Call(ctor, Undefined, []Value{NewString(message)})
		return exceptionError{exception: errObj}
	}
	// Fallback generic error object
	obj := NewObject(Null).AsPlainObject()
	obj.SetOwn("name", NewString("TypeError"))
	obj.SetOwn("message", NewString(message))
	return exceptionError{exception: NewValueFromPlainObject(obj)}
}

// NewReferenceError constructs a ReferenceError exception error for builtin helpers to return
func (vm *VM) NewReferenceError(message string) error {
	ctor, _ := vm.GetGlobal("ReferenceError")
	if ctor != Undefined {
		errObj, _ := vm.Call(ctor, Undefined, []Value{NewString(message)})
		return exceptionError{exception: errObj}
	}
	// Fallback generic error object
	obj := NewObject(Null).AsPlainObject()
	obj.SetOwn("name", NewString("ReferenceError"))
	obj.SetOwn("message", NewString(message))
	return exceptionError{exception: NewValueFromPlainObject(obj)}
}

// NewRangeError constructs a RangeError exception error for builtin helpers to return
func (vm *VM) NewRangeError(message string) error {
	ctor, _ := vm.GetGlobal("RangeError")
	if ctor != Undefined {
		errObj, _ := vm.Call(ctor, Undefined, []Value{NewString(message)})
		return exceptionError{exception: errObj}
	}
	// Fallback generic error object
	obj := NewObject(Null).AsPlainObject()
	obj.SetOwn("name", NewString("RangeError"))
	obj.SetOwn("message", NewString(message))
	return exceptionError{exception: NewValueFromPlainObject(obj)}
}

// NewSyntaxError constructs a SyntaxError exception error for builtin helpers to return
func (vm *VM) NewSyntaxError(message string) error {
	ctor, _ := vm.GetGlobal("SyntaxError")
	if ctor != Undefined {
		errObj, _ := vm.Call(ctor, Undefined, []Value{NewString(message)})
		return exceptionError{exception: errObj}
	}
	// Fallback generic error object
	obj := NewObject(Null).AsPlainObject()
	obj.SetOwn("name", NewString("SyntaxError"))
	obj.SetOwn("message", NewString(message))
	return exceptionError{exception: NewValueFromPlainObject(obj)}
}

var (
	Undefined     = Value{typ: TypeUndefined}
	Null          = Value{typ: TypeNull}
	Hole          = Value{typ: TypeHole}          // Internal marker for array holes (sparse arrays)
	Uninitialized = Value{typ: TypeUninitialized} // TDZ marker for let/const before initialization
	True          = Value{typ: TypeBoolean, payload: 1}
	False         = Value{typ: TypeBoolean, payload: 0}
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
	return Value{typ: TypeArray, obj: unsafe.Pointer(&ArrayObject{extensible: true})}
}

func NewArguments(args []Value, callee Value, isStrict bool) Value {
	argObj := &ArgumentsObject{
		length:   len(args),
		args:     make([]Value, len(args)),
		callee:   callee,
		isStrict: isStrict,
	}
	copy(argObj.args, args)
	return Value{typ: TypeArguments, obj: unsafe.Pointer(argObj)}
}

// NewGenerator creates a new generator object with the given generator function
func NewGenerator(function Value) Value {
	genObj := &GeneratorObject{
		Function:     function,
		State:        GeneratorStart, // Start state - prologue not yet executed
		Frame:        nil,            // Will be created during prologue execution
		YieldedValue: Undefined,
		ReturnValue:  Undefined,
		Done:         false,
	}
	return Value{typ: TypeGenerator, obj: unsafe.Pointer(genObj)}
}

func NewAsyncGenerator(function Value) Value {
	genObj := &AsyncGeneratorObject{
		Function:     function,
		State:        GeneratorStart,
		Frame:        nil, // Will be created when generator starts
		YieldedValue: Undefined,
		ReturnValue:  Undefined,
		Done:         false,
	}
	return Value{typ: TypeAsyncGenerator, obj: unsafe.Pointer(genObj)}
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

// NewArrayWithLength creates an array with the specified length
func NewArrayWithLength(length int) Value {
	arr := NewArray()
	if length > 0 {
		arr.AsArray().SetLength(length)
	}
	return arr
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

func NewProxy(target Value, handler Value) Value {
	proxyObj := &ProxyObject{
		target:  target,
		handler: handler,
		Revoked: false,
	}
	return Value{typ: TypeProxy, obj: unsafe.Pointer(proxyObj)}
}

// NewWeakMap creates a new WeakMap object
func NewWeakMap() Value {
	wmObj := &WeakMapObject{
		entries: make(map[uintptr]*WeakMapEntry),
	}
	return Value{typ: TypeWeakMap, obj: unsafe.Pointer(wmObj)}
}

// NewWeakSet creates a new WeakSet object
func NewWeakSet() Value {
	wsObj := &WeakSetObject{
		entries: make(map[uintptr]*WeakSetEntry),
	}
	return Value{typ: TypeWeakSet, obj: unsafe.Pointer(wsObj)}
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
	return v.typ == TypeObject || v.typ == TypeDictObject || v.typ == TypeArray || v.typ == TypeArguments || v.typ == TypeGenerator || v.typ == TypeAsyncGenerator || v.typ == TypePromise || v.typ == TypeRegExp || v.typ == TypeTypedArray || v.typ == TypeArrayBuffer || v.typ == TypeProxy || v.typ == TypeMap || v.typ == TypeSet || v.typ == TypeWeakMap || v.typ == TypeWeakSet
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
	case TypeFunction, TypeClosure, TypeNativeFunction, TypeNativeFunctionWithProps, TypeAsyncNativeFunction, TypeBoundFunction:
		return "function"
	case TypeProxy:
		// Proxy typeof depends on whether the target is callable
		proxy := v.AsProxy()
		if proxy.target.IsCallable() {
			return "function"
		}
		return "object"
	case TypeObject, TypeDictObject, TypeArray, TypeArguments, TypeRegExp, TypeTypedArray,
		TypeGenerator, TypeAsyncGenerator, TypePromise, TypeMap, TypeSet, TypeArrayBuffer,
		TypeWeakMap, TypeWeakSet:
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

// AsSymbolObject returns the underlying SymbolObject pointer for symbol values
func (v Value) AsSymbolObject() *SymbolObject {
	if v.typ != TypeSymbol {
		panic("value is not a symbol")
	}
	return (*SymbolObject)(v.obj)
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

func (v Value) AsAsyncGenerator() *AsyncGeneratorObject {
	if v.typ != TypeAsyncGenerator {
		panic("value is not an async generator")
	}
	return (*AsyncGeneratorObject)(v.obj)
}

func (v Value) AsPromise() *PromiseObject {
	if v.typ != TypePromise {
		return nil
	}
	return (*PromiseObject)(v.obj)
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

func (v Value) AsProxy() *ProxyObject {
	if v.typ != TypeProxy {
		panic("value is not a proxy")
	}
	return (*ProxyObject)(v.obj)
}

func (v Value) AsWeakMap() *WeakMapObject {
	if v.typ != TypeWeakMap {
		panic("value is not a weakmap")
	}
	return (*WeakMapObject)(v.obj)
}

func (v Value) AsWeakSet() *WeakSetObject {
	if v.typ != TypeWeakSet {
		panic("value is not a weakset")
	}
	return (*WeakSetObject)(v.obj)
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
		f := v.AsFloat()
		// ECMAScript ToString specification (7.1.12.1):
		// Handle special cases first
		if math.IsNaN(f) {
			return "NaN"
		}
		if math.IsInf(f, 1) {
			return "Infinity"
		}
		if math.IsInf(f, -1) {
			return "-Infinity"
		}
		// Handle -0 (convert to 0)
		if f == 0 && math.Signbit(f) {
			return "0"
		}
		// Use exponential notation for very small or very large numbers
		absF := f
		if absF < 0 {
			absF = -absF
		}
		// If |f| < 1e-6 or |f| >= 1e21, use exponential notation
		// Otherwise use fixed notation
		if absF != 0 && (absF < 1e-6 || absF >= 1e21) {
			// Use exponential notation and clean up format (remove leading zeros from exponent)
			exp := strconv.FormatFloat(f, 'e', -1, 64)
			return cleanExponentialFormat(exp)
		}
		// Use fixed notation
		return strconv.FormatFloat(f, 'f', -1, 64)
	case TypeIntegerNumber:
		return strconv.FormatInt(int64(v.AsInteger()), 10)
	case TypeBigInt:
		return v.AsBigInt().String() // No "n" suffix for string conversion
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
	case TypeAsyncGenerator:
		// AsyncGenerator object toString -> [object AsyncGenerator]
		return "[object AsyncGenerator]"
	case TypePromise:
		// Promise object toString -> [object Promise]
		return "[object Promise]"
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

// parseStringToNumber converts a string to a number following ECMAScript rules
// Handles hex (0x), octal (0o), binary (0b), and decimal (including scientific notation)
func parseStringToNumber(s string) float64 {
	str := strings.TrimSpace(s)
	if str == "" {
		return 0
	}

	// Handle hex (0x or 0X)
	if len(str) >= 2 && (strings.HasPrefix(str, "0x") || strings.HasPrefix(str, "0X")) {
		if i, err := strconv.ParseInt(str[2:], 16, 64); err == nil {
			return float64(i)
		}
		return math.NaN()
	}

	// Handle binary (0b or 0B)
	if len(str) >= 2 && (strings.HasPrefix(str, "0b") || strings.HasPrefix(str, "0B")) {
		if i, err := strconv.ParseInt(str[2:], 2, 64); err == nil {
			return float64(i)
		}
		return math.NaN()
	}

	// Handle octal (0o or 0O)
	if len(str) >= 2 && (strings.HasPrefix(str, "0o") || strings.HasPrefix(str, "0O")) {
		if i, err := strconv.ParseInt(str[2:], 8, 64); err == nil {
			return float64(i)
		}
		return math.NaN()
	}

	// Handle decimal (including scientific notation, Infinity, -Infinity)
	// Per ECMAScript spec, "Infinity" is case-sensitive (unlike Go's ParseFloat)
	if str == "Infinity" || str == "+Infinity" {
		return math.Inf(1)
	}
	if str == "-Infinity" {
		return math.Inf(-1)
	}

	// Check for invalid case variations (e.g., "INFINITY", "infinity")
	// Go's ParseFloat accepts these but ECMAScript does not
	strLower := strings.ToLower(str)
	if strLower == "infinity" || strLower == "+infinity" || strLower == "-infinity" {
		return math.NaN()
	}

	// Try parsing as float
	if f, err := strconv.ParseFloat(str, 64); err == nil {
		return f
	}

	return math.NaN()
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
	case TypeNull:
		return 0 // null converts to 0 in JavaScript
	case TypeUndefined:
		return math.NaN() // undefined converts to NaN in JavaScript
	case TypeBoolean:
		if v.AsBoolean() {
			return 1
		}
		return 0
	case TypeString:
		return parseStringToNumber(v.AsString())
	case TypeObject, TypeDictObject, TypeArray, TypeArguments, TypeRegExp, TypeMap, TypeSet, TypeArrayBuffer, TypeTypedArray, TypeProxy:
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
	if !v.IsObject() && v.typ != TypeArray && v.typ != TypeArguments && v.typ != TypeRegExp && v.typ != TypeMap && v.typ != TypeSet && v.typ != TypeProxy {
		return v
	}

	// For objects, we need to call valueOf/toString methods
	// Since we don't have VM instance here, we use a simplified approach
	// This will be enhanced when we have access to VM in this context
	if po := v.AsPlainObject(); po != nil {
		// For built-in wrappers, we can extract the primitive value directly
		switch v.typ {
		case TypeBoolean:
			return BooleanValue(v.AsBoolean())
		case TypeFloatNumber, TypeIntegerNumber:
			return NumberValue(v.ToFloat())
		case TypeString:
			return NewString(v.ToString())
		case TypeBigInt:
			// For BigInt object wrappers, extract the primitive value from [[BigIntData]]
			if po := v.AsPlainObject(); po != nil {
				if dataVal, exists := po.GetOwn("[[BigIntData]]"); exists {
					return dataVal
				}
			}
			// If no [[BigIntData]], return as-is (shouldn't happen for valid BigInt objects)
			return v
		}
	}

	// For other objects, return string representation as fallback
	return NewString(v.ToString())
}

func (v Value) ToInteger() int32 {
	// First apply ToPrimitive if it's an object
	if v.IsObject() || v.typ == TypeArray || v.typ == TypeArguments || v.typ == TypeRegExp || v.typ == TypeMap || v.typ == TypeSet || v.typ == TypeProxy {
		v = v.ToPrimitive("number")
	}

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
	return v.inspectWithDepth(false, 0, 64) // Top-level call, depth-limited to avoid stack overflow
}

// InspectNested is used for nested contexts where strings should be quoted
func (v Value) InspectNested() string {
	return v.inspectWithDepth(true, 0, 64) // Nested call, depth-limited
}

// inspectWithDepth is a depth-limited inspector to prevent runaway recursion in debug prints.
func (v Value) inspectWithDepth(nested bool, depth int, maxDepth int) string {
	if depth >= maxDepth {
		return "<…>"
	}
	switch v.typ {
	case TypeString:
		if nested {
			return fmt.Sprintf(`"%s"`, v.AsString())
		}
		return v.AsString()
	case TypeSymbol:
		return fmt.Sprintf("Symbol(%s)", v.AsSymbol())
	case TypeFloatNumber:
		return strconv.FormatFloat(v.AsFloat(), 'f', -1, 64)
	case TypeIntegerNumber:
		return strconv.FormatInt(int64(v.AsInteger()), 10)
	case TypeBigInt:
		return v.AsBigInt().String() + "n" // Include "n" suffix for display
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
	case TypeBoundFunction:
		boundFn := (*BoundFunctionObject)(v.obj)
		if boundFn.Name != "" {
			return fmt.Sprintf("[Function: %s]", boundFn.Name)
		}
		return "[Function (anonymous)]"
	case TypeObject:
		obj := v.AsPlainObject()
		if toStringResult := tryBuiltinToString(obj); toStringResult != "" {
			if nested {
				return fmt.Sprintf(`"%s"`, toStringResult)
			}
			return toStringResult
		}
		if toStringMethod := findToStringMethod(obj); toStringMethod.Type() != TypeUndefined && toStringMethod.IsFunction() {
			if dateString := tryFormatAsDate(obj); dateString != "" {
				if nested {
					return fmt.Sprintf(`"%s"`, dateString)
				}
				return dateString
			}
		}
		var b strings.Builder
		b.WriteString("{")
		for i, field := range obj.shape.fields {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(field.name)
			b.WriteString(": ")
			b.WriteString(obj.properties[i].inspectWithDepth(true, depth+1, maxDepth))
		}
		b.WriteString("}")
		return b.String()
	case TypeProxy:
		proxy := v.AsProxy()
		if proxy.Revoked {
			return "[Proxy (revoked)]"
		}
		return fmt.Sprintf("[Proxy target=%s handler=%s]", proxy.target.Inspect(), proxy.handler.Inspect())
	case TypeDictObject:
		dict := v.AsDictObject()
		keys := make([]string, 0, len(dict.properties))
		for k := range dict.properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = k + ": " + dict.properties[k].inspectWithDepth(true, depth+1, maxDepth)
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case TypeArray:
		arr := v.AsArray()
		elems := make([]string, len(arr.elements))
		for i, el := range arr.elements {
			elems[i] = el.inspectWithDepth(true, depth+1, maxDepth)
		}
		return "[" + strings.Join(elems, ", ") + "]"
	case TypeArguments:
		args := v.AsArguments()
		elems := make([]string, len(args.args))
		for i, el := range args.args {
			elems[i] = el.inspectWithDepth(true, depth+1, maxDepth)
		}
		return "[Arguments] { 0: " + strings.Join(elems, ", ") + " }"
	case TypeMap:
		mapObj := v.AsMap()
		if mapObj.Size() == 0 {
			return "Map {}"
		}
		var parts []string
		for keyStr, value := range mapObj.entries {
			key := mapObj.keys[keyStr]
			parts = append(parts, key.inspectWithDepth(true, depth+1, maxDepth)+" => "+value.inspectWithDepth(true, depth+1, maxDepth))
		}
		return "Map { " + strings.Join(parts, ", ") + " }"
	case TypeSet:
		setObj := v.AsSet()
		if setObj.Size() == 0 {
			return "Set {}"
		}
		var parts []string
		for _, value := range setObj.values {
			parts = append(parts, value.inspectWithDepth(true, depth+1, maxDepth))
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
	case TypePromise:
		promise := v.AsPromise()
		if promise != nil {
			switch promise.State {
			case PromisePending:
				return "Promise { <pending> }"
			case PromiseFulfilled:
				return fmt.Sprintf("Promise { %s }", promise.Result.inspectWithDepth(false, depth+1, maxDepth))
			case PromiseRejected:
				return fmt.Sprintf("Promise { <rejected> %s }", promise.Result.inspectWithDepth(false, depth+1, maxDepth))
			default:
				return "Promise { <unknown state> }"
			}
		}
		return "Promise {}"
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
	case TypeSymbol, TypeObject, TypeArray, TypeArguments, TypeFunction, TypeClosure, TypeNativeFunction, TypeRegExp, TypeProxy, TypePromise, TypeMap, TypeSet, TypeDictObject, TypeBoundFunction, TypeNativeFunctionWithProps, TypeAsyncNativeFunction, TypeGenerator, TypeAsyncGenerator, TypeArrayBuffer, TypeTypedArray:
		// All object types (including symbols, regex, proxies, promises, maps, sets, etc.) are truthy
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
		// String comparison by UTF-16 code units (not raw bytes)
		// This is needed because the same JS string can have different Go representations:
		// - Literal astral chars (e.g., "𐐨") are stored as UTF-8 (4 bytes)
		// - Escape sequences (e.g., "\ud801\udc28") are stored as WTF-8 (6 bytes)
		a, b := v.AsString(), other.AsString()
		// Fast path: if byte strings are equal, they're the same JS string
		if a == b {
			return true
		}
		// Medium path: if lengths are equal, bytes are different, strings are different
		// (same UTF-16 content can only have different byte lengths when mixing UTF-8/WTF-8)
		if len(a) == len(b) {
			return false
		}
		// Check if either string could have surrogates/astral chars that affect comparison
		if !stringNeedsUTF16Comparison(a) && !stringNeedsUTF16Comparison(b) {
			return false
		}
		// Slow path: compare by UTF-16 code units for mixed representations
		return compareStringsUTF16(a, b) == 0
	case TypeSymbol:
		// Symbols are only equal if they are the *same* object (reference)
		return v.obj == other.obj
	case TypeObject, TypeArray, TypeArguments, TypeFunction, TypeClosure, TypeNativeFunction, TypeNativeFunctionWithProps, TypeBoundFunction, TypeRegExp, TypeMap, TypeSet, TypeProxy:
		// Objects (including arrays, functions, regex, maps, sets, proxies, etc.) are equal only by reference
		return v.obj == other.obj
	default:
		panic(fmt.Sprintf("Unhandled type in Is comparison: %v", v.typ)) // Should not happen
	}
}

// StrictlyEquals compares two values using the ECMAScript Strict Equality Comparison (`===`).
// Types must match, no coercion. NaN !== NaN. +0 === -0.
func (v Value) StrictlyEquals(other Value) bool {
	// Handle cross-numeric comparison: IntegerNumber and FloatNumber are both JavaScript "number" type
	if v.IsNumber() && other.IsNumber() {
		vf := v.ToFloat()
		of := other.ToFloat()
		// Strict equality: NaN !== NaN
		if math.IsNaN(vf) || math.IsNaN(of) {
			return false
		}
		return vf == of
	}

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
		// String comparison by UTF-16 code units (not raw bytes)
		// This is needed because the same JS string can have different Go representations:
		// - Literal astral chars (e.g., "𐐨") are stored as UTF-8 (4 bytes)
		// - Escape sequences (e.g., "\ud801\udc28") are stored as WTF-8 (6 bytes)
		a, b := v.AsString(), other.AsString()
		// Fast path: if byte strings are equal, they're the same JS string
		if a == b {
			return true
		}
		// Medium path: if lengths are equal, bytes are different, strings are different
		// (same UTF-16 content can only have different byte lengths when mixing UTF-8/WTF-8)
		if len(a) == len(b) {
			return false
		}
		// Check if either string could have surrogates/astral chars that affect comparison
		if !stringNeedsUTF16Comparison(a) && !stringNeedsUTF16Comparison(b) {
			return false
		}
		// Slow path: compare by UTF-16 code units for mixed representations
		return compareStringsUTF16(a, b) == 0
	case TypeSymbol:
		// Symbols are only equal if they are the *same* object (reference)
		return v.obj == other.obj
	case TypeObject, TypeArray, TypeArguments, TypeFunction, TypeClosure, TypeNativeFunction, TypeNativeFunctionWithProps, TypeBoundFunction, TypeRegExp, TypeMap, TypeSet, TypeProxy:
		// Objects (including arrays, functions, regex, maps, sets, proxies, etc.) are equal only by reference
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
func AsMap(v Value) *MapObject             { return v.AsMap() }
func AsSet(v Value) *SetObject             { return v.AsSet() }

// valuesEqual compares two values using ECMAScript SameValueZero (NaN===NaN, +0===-0).
func valuesEqual(a, b Value) bool { return a.Is(b) }

// valuesStrictEqual compares two values using ECMAScript Strict Equality (===).
func valuesStrictEqual(a, b Value) bool { return a.StrictlyEquals(b) }

// valuesAbstractEqual implements the ECMAScript Abstract Equality Comparison (==)
// with a pragmatic subset sufficient for Test262 harness helpers:
// - undefined == null is true
// - number == string performs ToNumber on string
// - boolean == x compares ToNumber(boolean) to x
// - bigint == string parses string as BigInt if possible
// - number == bigint compares only when number is finite integral and within int64 range
// - otherwise falls back to Strict Equality when types match, or false
func valuesAbstractEqual(a, b Value) bool {
	// If types are identical, use strict equality
	if a.Type() == b.Type() {
		return a.StrictlyEquals(b)
	}

	// null/undefined
	if (a.Type() == TypeNull && b.Type() == TypeUndefined) || (a.Type() == TypeUndefined && b.Type() == TypeNull) {
		return true
	}

	// number and string
	if IsNumber(a) && b.Type() == TypeString {
		return AsNumber(a) == b.ToFloat()
	}
	if a.Type() == TypeString && IsNumber(b) {
		return a.ToFloat() == AsNumber(b)
	}

	// boolean compared to anything -> compare ToNumber(boolean) to other via abstract again
	if a.Type() == TypeBoolean {
		num := 0.0
		if a.AsBoolean() {
			num = 1.0
		}
		return valuesAbstractEqual(Number(num), b)
	}
	if b.Type() == TypeBoolean {
		num := 0.0
		if b.AsBoolean() {
			num = 1.0
		}
		return valuesAbstractEqual(a, Number(num))
	}

	// bigint and string
	if a.IsBigInt() && b.Type() == TypeString {
		if bi, ok := stringToBigInt(b.ToString()); ok {
			return a.AsBigInt().Cmp(bi) == 0
		}
		return false
	}
	if b.IsBigInt() && a.Type() == TypeString {
		if bi, ok := stringToBigInt(a.ToString()); ok {
			return b.AsBigInt().Cmp(bi) == 0
		}
		return false
	}

	// number and bigint
	if IsNumber(a) && b.IsBigInt() {
		n := a.ToFloat()
		if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) {
			return false
		}
		// Limit to int64 for now
		if n < math.MinInt64 || n > math.MaxInt64 {
			return false
		}
		ni := int64(n)
		return new(big.Int).SetInt64(ni).Cmp(b.AsBigInt()) == 0
	}
	if a.IsBigInt() && IsNumber(b) {
		n := b.ToFloat()
		if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) {
			return false
		}
		if n < math.MinInt64 || n > math.MaxInt64 {
			return false
		}
		ni := int64(n)
		return a.AsBigInt().Cmp(new(big.Int).SetInt64(ni)) == 0
	}

	// TODO: object-to-primitive cases (ToPrimitive) if needed

	// Default: not equal
	return false
}

// AsNativeFunction returns the NativeFunctionObject pointer from a native function value.
func AsNativeFunction(v Value) *NativeFunctionObject { return v.AsNativeFunction() }

// AsBoundFunction returns the BoundFunctionObject pointer from a bound function value.
func AsBoundFunction(v Value) *BoundFunctionObject { return v.AsBoundFunction() }

// IsFunction reports whether the value is a function (FunctionObject or ClosureObject or NativeFunctionObject).
func IsFunction(v Value) bool { return v.IsFunction() }

// AsFunction returns the FunctionObject pointer from a function template value.
func AsFunction(v Value) *FunctionObject { return v.AsFunction() }

// Helper for BigInt == String coercion per ECMAScript StringToBigInt
func stringToBigInt(s string) (*big.Int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		// Per ECMAScript spec, empty string (or whitespace-only) converts to 0n
		return big.NewInt(0), true
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
	}
	// Don't expand elements slice - JavaScript arrays are sparse
	// Elements beyond len(a.elements) will return undefined when accessed
	a.length = newLength
}

// Get returns the element at the given index, or Undefined if out of bounds
func (a *ArrayObject) Get(index int) Value {
	if index < 0 || index >= len(a.elements) {
		return Undefined
	}
	elem := a.elements[index]
	// Holes should appear as undefined when accessed
	if elem.typ == TypeHole {
		return Undefined
	}
	return elem
}

// Set sets the element at the given index, expanding the array if necessary
func (a *ArrayObject) Set(index int, value Value) {
	if index < 0 {
		return // Ignore negative indices
	}

	// Expand array if necessary
	if index >= len(a.elements) {
		for i := len(a.elements); i < index; i++ {
			a.elements = append(a.elements, Hole) // Use Hole marker for gaps
		}
		a.elements = append(a.elements, value) // Append the actual value at index
	} else {
		a.elements[index] = value
	}
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

// HasIndex returns true if the index has an actual value (not a hole in sparse array)
func (a *ArrayObject) HasIndex(index int) bool {
	// Check bounds first
	if index < 0 || index >= len(a.elements) {
		return false
	}
	// Check if it's a hole (using the special Hole marker)
	return a.elements[index].typ != TypeHole
}

// GetOwn returns a named property from the array (e.g., "index", "input" for match results)
func (a *ArrayObject) GetOwn(name string) (Value, bool) {
	if a.properties == nil {
		return Undefined, false
	}
	v, ok := a.properties[name]
	return v, ok
}

// SetOwn sets a named property on the array (e.g., "index", "input" for match results)
func (a *ArrayObject) SetOwn(name string, value Value) {
	if a.properties == nil {
		a.properties = make(map[string]Value)
	}
	a.properties[name] = value
}

// DefineOwnProperty sets a named property with specified descriptor attributes
func (a *ArrayObject) DefineOwnProperty(name string, value Value, writable, enumerable, configurable bool) {
	if a.properties == nil {
		a.properties = make(map[string]Value)
	}
	if a.propertyDesc == nil {
		a.propertyDesc = make(map[string]PropertyDesc)
	}
	a.properties[name] = value
	a.propertyDesc[name] = PropertyDesc{
		Writable:     writable,
		Enumerable:   enumerable,
		Configurable: configurable,
	}
}

// GetOwnPropertyDescriptor returns the descriptor for a named property
func (a *ArrayObject) GetOwnPropertyDescriptor(name string) (Value, PropertyDesc, bool) {
	if a.properties == nil {
		return Undefined, PropertyDesc{}, false
	}
	v, ok := a.properties[name]
	if !ok {
		return Undefined, PropertyDesc{}, false
	}
	// Check if we have a stored descriptor
	if a.propertyDesc != nil {
		if desc, hasDesc := a.propertyDesc[name]; hasDesc {
			return v, desc, true
		}
	}
	// Default descriptor if no explicit one was set
	return v, PropertyDesc{Writable: true, Enumerable: true, Configurable: true}, true
}

// IsFrozen returns whether this array is frozen
func (a *ArrayObject) IsFrozen() bool {
	return a.frozen
}

// SetFrozen sets whether this array is frozen (elements non-writable/non-configurable)
func (a *ArrayObject) SetFrozen(frozen bool) {
	a.frozen = frozen
}

// IsExtensible returns whether new properties can be added to this array
func (a *ArrayObject) IsExtensible() bool {
	return a.extensible
}

// SetExtensible sets whether new properties can be added to this array
func (a *ArrayObject) SetExtensible(extensible bool) {
	a.extensible = extensible
}

// GetSymbolProp returns a symbol-keyed property from the array object
func (a *ArrayObject) GetSymbolProp(sym *SymbolObject) (Value, bool) {
	if a.symbolProps == nil {
		return Undefined, false
	}
	v, ok := a.symbolProps[sym]
	return v, ok
}

// SetSymbolProp sets a symbol-keyed property on the array object
func (a *ArrayObject) SetSymbolProp(sym *SymbolObject, val Value) {
	if a.symbolProps == nil {
		a.symbolProps = make(map[*SymbolObject]Value)
	}
	a.symbolProps[sym] = val
}

// HasOwnSymbolProp checks if the array object has an own symbol property
func (a *ArrayObject) HasOwnSymbolProp(sym *SymbolObject) bool {
	if a.symbolProps == nil {
		return false
	}
	_, ok := a.symbolProps[sym]
	return ok
}

// ArgumentsObject methods
func (a *ArgumentsObject) Length() int {
	return a.length
}

func (a *ArgumentsObject) Get(index int) Value {
	// For mapped arguments (sloppy mode), read directly from the register
	if index >= 0 && index < a.numMapped && a.mappedRegs != nil {
		return a.mappedRegs[index]
	}
	if index >= 0 && index < len(a.args) {
		return a.args[index]
	}
	// Check namedProps for indices beyond original length
	// (per ECMAScript, arguments objects can have arbitrary properties added)
	if index >= 0 && a.namedProps != nil {
		if v, ok := a.namedProps[strconv.Itoa(index)]; ok {
			return v
		}
	}
	return Undefined
}

func (a *ArgumentsObject) Set(index int, value Value) {
	// For mapped arguments (sloppy mode), write directly to the register
	if index >= 0 && index < a.numMapped && a.mappedRegs != nil {
		a.mappedRegs[index] = value
		return
	}
	if index < 0 || index >= len(a.args) {
		return // Arguments object doesn't expand like arrays
	}
	a.args[index] = value
}

// Callee returns the function that created this arguments object
func (a *ArgumentsObject) Callee() Value {
	return a.callee
}

// IsStrict returns whether this arguments object is from strict mode code
func (a *ArgumentsObject) IsStrict() bool {
	return a.isStrict
}

// GetSymbolProp returns a symbol-keyed property from the arguments object
func (a *ArgumentsObject) GetSymbolProp(sym *SymbolObject) (Value, bool) {
	if a.symbolProps == nil {
		return Undefined, false
	}
	v, ok := a.symbolProps[sym]
	return v, ok
}

// SetSymbolProp sets a symbol-keyed property on the arguments object
func (a *ArgumentsObject) SetSymbolProp(sym *SymbolObject, val Value) {
	if a.symbolProps == nil {
		a.symbolProps = make(map[*SymbolObject]Value)
	}
	a.symbolProps[sym] = val
}

// HasOwnSymbolProp checks if the arguments object has an own symbol property
func (a *ArgumentsObject) HasOwnSymbolProp(sym *SymbolObject) bool {
	if a.symbolProps == nil {
		return false
	}
	_, ok := a.symbolProps[sym]
	return ok
}

// SetCallee sets the callee property on the arguments object
func (a *ArgumentsObject) SetCallee(val Value) {
	a.callee = val
}

// SetLength sets the length property on the arguments object
func (a *ArgumentsObject) SetLength(val int) {
	a.length = val
}

// GetNamedProp returns a named property from the arguments object's overflow storage
func (a *ArgumentsObject) GetNamedProp(name string) (Value, bool) {
	if a.namedProps == nil {
		return Undefined, false
	}
	v, ok := a.namedProps[name]
	return v, ok
}

// SetNamedProp sets a named property on the arguments object's overflow storage
func (a *ArgumentsObject) SetNamedProp(name string, val Value) {
	if a.namedProps == nil {
		a.namedProps = make(map[string]Value)
	}
	a.namedProps[name] = val
}

// HasNamedProp checks if the arguments object has a named property in overflow storage
func (a *ArgumentsObject) HasNamedProp(name string) bool {
	if a.namedProps == nil {
		return false
	}
	_, ok := a.namedProps[name]
	return ok
}

// MapObject methods
func (m *MapObject) Set(key, value Value) {
	keyStr := hashKey(key)
	// fmt.Printf("[DBG Map.set] m=%p key=%s (%s) -> %s\n", m, keyStr, key.TypeName(), value.Inspect())
	if _, exists := m.entries[keyStr]; !exists {
		// Check if this key was deleted (has tombstone)
		if m.tombstones != nil && m.tombstones[keyStr] {
			// Revive the tombstone - don't add to order, just remove from tombstones
			delete(m.tombstones, keyStr)
		} else {
			// Truly new key - add to order
			m.order = append(m.order, keyStr)
		}
		m.size++
	}
	m.entries[keyStr] = value
	m.keys[keyStr] = key
}

func (m *MapObject) Get(key Value) Value {
	keyStr := hashKey(key)
	// fmt.Printf("[DBG Map.get] m=%p key=%s (%s) -> %v\n", m, keyStr, key.TypeName(), m.entries[keyStr])
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
		// NOTE: We intentionally do NOT remove from m.order here.
		// Mark this key as a tombstone so live iterators skip it,
		// but re-insertion can revive the entry at its original position.
		if m.tombstones == nil {
			m.tombstones = make(map[string]bool)
		}
		m.tombstones[keyStr] = true
		m.size--
		return true
	}
	return false
}

func (m *MapObject) Clear() {
	m.entries = make(map[string]Value)
	m.keys = make(map[string]Value)
	m.order = nil        // Reset insertion order
	m.tombstones = nil   // Clear tombstones
	m.size = 0
}

func (m *MapObject) Size() int {
	return m.size
}

// ForEach calls fn for each entry in the map in insertion order.
// Skips entries that have been deleted (tombstones in order array).
func (m *MapObject) ForEach(fn func(key Value, value Value)) {
	for _, keyStr := range m.order {
		// Check if this entry is a tombstone (deleted)
		if m.tombstones != nil && m.tombstones[keyStr] {
			continue
		}
		value, exists := m.entries[keyStr]
		if !exists {
			continue
		}
		if originalKey, ok := m.keys[keyStr]; ok {
			fn(originalKey, value)
		} else {
			// Fallback: synthesize string key
			fn(NewString(keyStr), value)
		}
	}
}

// OrderLen returns the length of the order array (including tombstones).
// Used by live iterators.
func (m *MapObject) OrderLen() int {
	return len(m.order)
}

// GetEntryAt returns the key-value pair at the given index in insertion order.
// Returns (key, value, true) if the entry exists, or (Undefined, Undefined, false)
// if the index is out of bounds or the entry was deleted.
// Used by live iterators.
func (m *MapObject) GetEntryAt(index int) (Value, Value, bool) {
	if index < 0 || index >= len(m.order) {
		return Undefined, Undefined, false
	}
	keyStr := m.order[index]
	// Check if this entry is a tombstone (deleted)
	if m.tombstones != nil && m.tombstones[keyStr] {
		return Undefined, Undefined, false
	}
	value, exists := m.entries[keyStr]
	if !exists {
		return Undefined, Undefined, false
	}
	if originalKey, ok := m.keys[keyStr]; ok {
		return originalKey, value, true
	}
	// Fallback: synthesize string key
	return NewString(keyStr), value, true
}

// SetObject methods
func (s *SetObject) Add(value Value) {
	keyStr := hashKey(value)
	if _, exists := s.values[keyStr]; !exists {
		// Check if this value was deleted (has tombstone)
		if s.tombstones != nil && s.tombstones[keyStr] {
			// Revive the tombstone - don't add to order, just remove from tombstones
			delete(s.tombstones, keyStr)
		} else {
			// Truly new value - add to order
			s.order = append(s.order, keyStr)
		}
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
		// Mark as tombstone for live iteration
		if s.tombstones == nil {
			s.tombstones = make(map[string]bool)
		}
		s.tombstones[keyStr] = true
		s.size--
		return true
	}
	return false
}

func (s *SetObject) Clear() {
	s.values = make(map[string]Value)
	s.order = nil      // Reset insertion order
	s.tombstones = nil // Clear tombstones
	s.size = 0
}

func (s *SetObject) Size() int {
	return s.size
}

// ForEach calls fn for each value in the set in insertion order.
// Skips tombstones (deleted values).
func (s *SetObject) ForEach(fn func(value Value)) {
	for _, keyStr := range s.order {
		// Check if this entry is a tombstone (deleted)
		if s.tombstones != nil && s.tombstones[keyStr] {
			continue
		}
		if val, exists := s.values[keyStr]; exists {
			fn(val)
		}
	}
}

// OrderLen returns the length of the order array (including tombstones).
// Used by live iterators.
func (s *SetObject) OrderLen() int {
	return len(s.order)
}

// GetValueAt returns the value at the given index in insertion order.
// Returns (value, true) if the entry exists, or (Undefined, false)
// if the index is out of bounds or the entry was deleted.
// Used by live iterators.
func (s *SetObject) GetValueAt(index int) (Value, bool) {
	if index < 0 || index >= len(s.order) {
		return Undefined, false
	}
	keyStr := s.order[index]
	// Check if this entry is a tombstone (deleted)
	if s.tombstones != nil && s.tombstones[keyStr] {
		return Undefined, false
	}
	if val, exists := s.values[keyStr]; exists {
		return val, true
	}
	return Undefined, false
}

// WeakMapObject methods - implements ECMAScript WeakMap using Go's weak package
// Keys must be objects (not primitives) and are held weakly, allowing GC.

// Set adds or updates a key-value pair in the WeakMap
// Returns false if the key is not a valid object type
func (wm *WeakMapObject) Set(key, value Value) bool {
	// WeakMap keys must be objects
	if !key.IsObject() {
		return false
	}

	ptr := uintptr(key.obj)
	// Create weak reference - cast to *byte for the weak pointer
	weakPtr := weak.Make((*byte)(key.obj))

	wm.entries[ptr] = &WeakMapEntry{
		keyWeak: weakPtr,
		value:   value,
	}
	return true
}

// Get retrieves a value from the WeakMap by key
// Returns (Undefined, false) if key not found or key has been GC'd
func (wm *WeakMapObject) Get(key Value) (Value, bool) {
	if !key.IsObject() {
		return Undefined, false
	}

	ptr := uintptr(key.obj)
	entry, exists := wm.entries[ptr]
	if !exists {
		return Undefined, false
	}

	// Check if the key is still alive
	if entry.keyWeak.Value() == nil {
		// Key has been GC'd, clean up the entry
		delete(wm.entries, ptr)
		return Undefined, false
	}

	return entry.value, true
}

// Has checks if a key exists in the WeakMap
func (wm *WeakMapObject) Has(key Value) bool {
	if !key.IsObject() {
		return false
	}

	ptr := uintptr(key.obj)
	entry, exists := wm.entries[ptr]
	if !exists {
		return false
	}

	// Check if the key is still alive
	if entry.keyWeak.Value() == nil {
		// Key has been GC'd, clean up the entry
		delete(wm.entries, ptr)
		return false
	}

	return true
}

// Delete removes a key-value pair from the WeakMap
// Returns true if the key was found and deleted
func (wm *WeakMapObject) Delete(key Value) bool {
	if !key.IsObject() {
		return false
	}

	ptr := uintptr(key.obj)
	if _, exists := wm.entries[ptr]; exists {
		delete(wm.entries, ptr)
		return true
	}
	return false
}

// WeakSetObject methods - implements ECMAScript WeakSet using Go's weak package
// Values must be objects and are held weakly, allowing GC.

// Add adds a value to the WeakSet
// Returns false if the value is not a valid object type
func (ws *WeakSetObject) Add(value Value) bool {
	// WeakSet values must be objects
	if !value.IsObject() {
		return false
	}

	ptr := uintptr(value.obj)
	// Create weak reference
	weakPtr := weak.Make((*byte)(value.obj))

	ws.entries[ptr] = &WeakSetEntry{
		valueWeak: weakPtr,
	}
	return true
}

// Has checks if a value exists in the WeakSet
func (ws *WeakSetObject) Has(value Value) bool {
	if !value.IsObject() {
		return false
	}

	ptr := uintptr(value.obj)
	entry, exists := ws.entries[ptr]
	if !exists {
		return false
	}

	// Check if the value is still alive
	if entry.valueWeak.Value() == nil {
		// Value has been GC'd, clean up the entry
		delete(ws.entries, ptr)
		return false
	}

	return true
}

// Delete removes a value from the WeakSet
// Returns true if the value was found and deleted
func (ws *WeakSetObject) Delete(value Value) bool {
	if !value.IsObject() {
		return false
	}

	ptr := uintptr(value.obj)
	if _, exists := ws.entries[ptr]; exists {
		delete(ws.entries, ptr)
		return true
	}
	return false
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

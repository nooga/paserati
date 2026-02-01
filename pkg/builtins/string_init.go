package builtins

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"

	"golang.org/x/text/unicode/norm"
)

// ErrVMUnwinding is a sentinel error that indicates the VM is unwinding
// with an exception. When this error is returned, the caller should abort
// and return vm.Undefined immediately (without returning an error) to let
// the exception continue to propagate.
var ErrVMUnwinding = errors.New("VM unwinding")

// requireObjectCoercible checks if a value can be converted to an object (not null or undefined).
// Returns an error if the value is null or undefined, nil otherwise.
func requireObjectCoercible(vmInstance *vm.VM, val vm.Value, methodName string) error {
	if val.Type() == vm.TypeNull {
		return vmInstance.NewTypeError(fmt.Sprintf("String.prototype.%s called on null or undefined", methodName))
	}
	if val.Type() == vm.TypeUndefined {
		return vmInstance.NewTypeError(fmt.Sprintf("String.prototype.%s called on null or undefined", methodName))
	}
	return nil
}

// isRegExp checks if a value is a RegExp object
func isRegExp(val vm.Value) bool {
	return val.IsRegExp()
}

// getStringValueWithVM extracts the string value from a value using the VM.
// For primitive strings, returns the string directly.
// For String wrapper objects, extracts the [[PrimitiveValue]].
// For other objects, calls ToPrimitive with "string" hint to get proper conversion.
// Returns an error if a Symbol is passed (cannot convert Symbol to string).
// Also propagates any exceptions thrown during ToPrimitive conversion.
func getStringValueWithVM(vmInstance *vm.VM, val vm.Value) (string, error) {
	// Symbols cannot be converted to strings implicitly
	if val.Type() == vm.TypeSymbol {
		return "", vmInstance.NewTypeError("Cannot convert a Symbol value to a string")
	}

	// If it's a primitive string, return it directly
	if val.Type() == vm.TypeString {
		return val.ToString(), nil
	}

	// If it's an object, check for [[PrimitiveValue]] (String wrapper)
	if val.IsObject() {
		if plainObj := val.AsPlainObject(); plainObj != nil {
			if primitiveVal, exists := plainObj.GetOwn("[[PrimitiveValue]]"); exists {
				if primitiveVal.Type() == vm.TypeString {
					return primitiveVal.ToString(), nil
				}
				// Check if [[PrimitiveValue]] is a Symbol - can't convert to string
				if primitiveVal.Type() == vm.TypeSymbol {
					return "", vmInstance.NewTypeError("Cannot convert a Symbol value to a string")
				}
			}
		}
		// For other objects, use ToPrimitive to properly call toString/valueOf
		// Track that we're in a helper call so exception handlers can be found
		vmInstance.EnterHelperCall()
		primVal := vmInstance.ToPrimitive(val, "string")
		vmInstance.ExitHelperCall()
		// Check if ToPrimitive threw an exception - if so, the VM is unwinding
		// and we should return ErrVMUnwinding to signal the caller to abort
		if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
			return "", ErrVMUnwinding
		}
		// Check if ToPrimitive returned a Symbol
		if primVal.Type() == vm.TypeSymbol {
			return "", vmInstance.NewTypeError("Cannot convert a Symbol value to a string")
		}
		return primVal.ToString(), nil
	}

	// Fall back to ToString() for other types
	return val.ToString(), nil
}

// getStringValue is a simple version without VM access (for backward compatibility)
func getStringValue(val vm.Value) string {
	// If it's a primitive string, return it directly
	if val.Type() == vm.TypeString {
		return val.ToString()
	}

	// If it's an object, check for [[PrimitiveValue]] (String wrapper)
	if val.IsObject() {
		if plainObj := val.AsPlainObject(); plainObj != nil {
			if primitiveVal, exists := plainObj.GetOwn("[[PrimitiveValue]]"); exists {
				if primitiveVal.Type() == vm.TypeString {
					return primitiveVal.ToString()
				}
			}
		}
	}

	// Fall back to ToString() for other types
	return val.ToString()
}

// isECMAScriptWhitespace checks if a rune is considered whitespace by ECMAScript
// This includes all Unicode whitespace plus BOM (U+FEFF) and line terminators
func isECMAScriptWhitespace(r rune) bool {
	// ECMAScript 5.1 - 7.2 White Space + 7.3 Line Terminators
	switch r {
	case '\t', // TAB
		'\n',     // LF (Line Feed)
		'\v',     // VT (Vertical Tab)
		'\f',     // FF (Form Feed)
		'\r',     // CR (Carriage Return)
		' ',      // Space
		'\u00A0', // No-Break Space
		'\u1680', // Ogham Space Mark
		'\u2000', // En Quad
		'\u2001', // Em Quad
		'\u2002', // En Space
		'\u2003', // Em Space
		'\u2004', // Three-Per-Em Space
		'\u2005', // Four-Per-Em Space
		'\u2006', // Six-Per-Em Space
		'\u2007', // Figure Space
		'\u2008', // Punctuation Space
		'\u2009', // Thin Space
		'\u200A', // Hair Space
		'\u2028', // Line Separator
		'\u2029', // Paragraph Separator
		'\u202F', // Narrow No-Break Space
		'\u205F', // Medium Mathematical Space
		'\u3000', // Ideographic Space
		'\uFEFF': // Zero Width No-Break Space (BOM)
		return true
	}
	return false
}

// trimECMAScriptWhitespace trims ECMAScript whitespace from both ends of a string
func trimECMAScriptWhitespace(s string) string {
	return strings.TrimFunc(s, isECMAScriptWhitespace)
}

// trimLeftECMAScriptWhitespace trims ECMAScript whitespace from the left of a string
func trimLeftECMAScriptWhitespace(s string) string {
	return strings.TrimLeftFunc(s, isECMAScriptWhitespace)
}

// trimRightECMAScriptWhitespace trims ECMAScript whitespace from the right of a string
func trimRightECMAScriptWhitespace(s string) string {
	return strings.TrimRightFunc(s, isECMAScriptWhitespace)
}

// toNumberWithVM converts a value to a number, throwing TypeError for Symbols and BigInt.
// This implements ECMAScript's ToNumber abstract operation with proper Symbol/BigInt handling.
// It also propagates any exceptions thrown during ToPrimitive conversion.
func toNumberWithVM(vmInstance *vm.VM, val vm.Value) (float64, error) {
	if val.Type() == vm.TypeSymbol {
		return 0, vmInstance.NewTypeError("Cannot convert a Symbol value to a number")
	}
	if val.Type() == vm.TypeBigInt {
		return 0, vmInstance.NewTypeError("Cannot convert a BigInt value to a number")
	}
	// For objects, use ToPrimitive first
	if val.IsObject() || val.IsCallable() {
		// Track that we're in a helper call so exception handlers can be found
		vmInstance.EnterHelperCall()
		val = vmInstance.ToPrimitive(val, "number")
		vmInstance.ExitHelperCall()
		// Check if ToPrimitive threw an exception - if so, the VM is unwinding
		// and we should return ErrVMUnwinding to signal the caller to abort
		if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
			return 0, ErrVMUnwinding
		}
		// Check again after ToPrimitive
		if val.Type() == vm.TypeSymbol {
			return 0, vmInstance.NewTypeError("Cannot convert a Symbol value to a number")
		}
		if val.Type() == vm.TypeBigInt {
			return 0, vmInstance.NewTypeError("Cannot convert a BigInt value to a number")
		}
	}
	return val.ToFloat(), nil
}

// toIntegerWithVM converts a value to an integer, throwing TypeError for Symbols.
// This implements ECMAScript's ToInteger abstract operation with proper Symbol handling.
func toIntegerWithVM(vmInstance *vm.VM, val vm.Value) (int, error) {
	n, err := toNumberWithVM(vmInstance, val)
	if err != nil {
		return 0, err
	}
	if n != n { // NaN
		return 0, nil
	}
	if n == 0 {
		return 0, nil
	}
	// Handle infinity
	if n > 0 && n > float64(1<<31-1) {
		return 1<<31 - 1, nil
	}
	if n < 0 && n < float64(-(1<<31)) {
		return -(1 << 31), nil
	}
	// Truncate towards zero (ECMAScript ToInteger)
	// int() in Go truncates towards zero, which is correct
	return int(n), nil
}

// processStringReplacementPattern processes $ patterns in replacement strings for string search
// $$ -> $
// $& -> matched substring
// $` -> portion before match
// $' -> portion after match
// $n and $nn are NOT supported for string search (no capture groups)
func processStringReplacementPattern(str string, match []int, replacement string) string {
	var result strings.Builder
	i := 0
	for i < len(replacement) {
		if replacement[i] == '$' && i+1 < len(replacement) {
			switch replacement[i+1] {
			case '$':
				// $$ -> $
				result.WriteByte('$')
				i += 2
			case '&':
				// $& -> matched substring
				result.WriteString(str[match[0]:match[1]])
				i += 2
			case '`':
				// $` -> portion before match
				result.WriteString(str[:match[0]])
				i += 2
			case '\'':
				// $' -> portion after match
				result.WriteString(str[match[1]:])
				i += 2
			default:
				// For string search, $n patterns are output literally (no capture groups)
				result.WriteByte('$')
				i++
			}
		} else {
			result.WriteByte(replacement[i])
			i++
		}
	}
	return result.String()
}

type StringInitializer struct{}

func (s *StringInitializer) Name() string {
	return "String"
}

func (s *StringInitializer) Priority() int {
	return 300 // After Object (100) and Function (200)
}

func (s *StringInitializer) InitTypes(ctx *TypeContext) error {
	// Create String constructor type first (needed for constructor property)
	stringCtorType := types.NewSimpleFunction([]types.Type{types.Any}, types.String).
		WithProperty("fromCharCode", types.NewVariadicFunction([]types.Type{}, types.String, &types.ArrayType{ElementType: types.Number})).
		WithProperty("fromCodePoint", types.NewVariadicFunction([]types.Type{}, types.String, &types.ArrayType{ElementType: types.Number})).
		WithProperty("raw", types.NewVariadicFunction([]types.Type{types.Any}, types.String, &types.ArrayType{ElementType: types.Any}))

	// Create String.prototype type with all methods
	// Note: 'this' is implicit and not included in type signatures
	stringProtoType := types.NewObjectType().
		WithProperty("at", types.NewOptionalFunction([]types.Type{types.Number}, types.NewUnionType(types.String, types.Undefined), []bool{true})).
		WithProperty("charAt", types.NewOptionalFunction([]types.Type{types.Number}, types.String, []bool{true})).
		WithProperty("charCodeAt", types.NewOptionalFunction([]types.Type{types.Number}, types.Number, []bool{true})).
		WithProperty("codePointAt", types.NewOptionalFunction([]types.Type{types.Number}, types.NewUnionType(types.Number, types.Undefined), []bool{true})).
		WithProperty("slice", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.String, []bool{false, true})).
		WithProperty("substring", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.String, []bool{false, true})).
		WithProperty("substr", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.String, []bool{false, true})).
		WithProperty("indexOf", types.NewOptionalFunction([]types.Type{types.String, types.Number}, types.Number, []bool{false, true})).
		WithProperty("lastIndexOf", types.NewOptionalFunction([]types.Type{types.String, types.Number}, types.Number, []bool{false, true})).
		WithProperty("includes", types.NewOptionalFunction([]types.Type{types.String, types.Number}, types.Boolean, []bool{false, true})).
		WithProperty("startsWith", types.NewOptionalFunction([]types.Type{types.String, types.Number}, types.Boolean, []bool{false, true})).
		WithProperty("endsWith", types.NewOptionalFunction([]types.Type{types.String, types.Number}, types.Boolean, []bool{false, true})).
		WithProperty("toLowerCase", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toUpperCase", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toLocaleLowerCase", types.NewOptionalFunction([]types.Type{types.String}, types.String, []bool{true})).
		WithProperty("toLocaleUpperCase", types.NewOptionalFunction([]types.Type{types.String}, types.String, []bool{true})).
		WithProperty("normalize", types.NewOptionalFunction([]types.Type{types.String}, types.String, []bool{true})).
		WithProperty("localeCompare", types.NewSimpleFunction([]types.Type{types.String}, types.Number)).
		WithProperty("trim", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("trimStart", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("trimEnd", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("repeat", types.NewSimpleFunction([]types.Type{types.Number}, types.String)).
		WithProperty("padStart", types.NewOptionalFunction([]types.Type{types.Number, types.String}, types.String, []bool{false, true})).
		WithProperty("padEnd", types.NewOptionalFunction([]types.Type{types.Number, types.String}, types.String, []bool{false, true})).
		WithProperty("concat", types.NewVariadicFunction([]types.Type{}, types.String, types.String)).
		WithProperty("split", types.NewOptionalFunction([]types.Type{types.NewUnionType(types.String, types.RegExp), types.Number}, &types.ArrayType{ElementType: types.String}, []bool{false, true})).
		WithProperty("replace", types.NewSimpleFunction([]types.Type{types.NewUnionType(types.String, types.RegExp), types.String}, types.String)).
		WithProperty("replaceAll", types.NewSimpleFunction([]types.Type{types.NewUnionType(types.String, types.RegExp), types.String}, types.String)).
		WithProperty("match", types.NewSimpleFunction([]types.Type{types.NewUnionType(types.String, types.RegExp)}, types.NewUnionType(&types.ArrayType{ElementType: types.String}, types.Null))).
		WithProperty("matchAll", types.NewSimpleFunction([]types.Type{types.NewUnionType(types.String, types.RegExp)}, types.Any)). // Returns IterableIterator<RegExpMatchArray>
		WithProperty("search", types.NewSimpleFunction([]types.Type{types.NewUnionType(types.String, types.RegExp)}, types.Number)).
		WithProperty("constructor", types.Any) // Avoid circular reference, use Any for constructor property

	// Add Symbol.iterator to string prototype type to make strings iterable
	// Get the Iterator<T> generic type if available (use internal name)
	if iteratorType, found := ctx.GetType("__IteratorGeneric__"); found {
		if iteratorGeneric, ok := iteratorType.(*types.GenericType); ok {
			// Create Iterator<string> type for strings
			iteratorOfString := &types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{types.String},
			}
			// Add [Symbol.iterator](): Iterator<string> method (computed symbol key in types)
			stringProtoType = stringProtoType.WithProperty("__COMPUTED_PROPERTY__",
				types.NewSimpleFunction([]types.Type{}, iteratorOfString.Substitute()))
		}
	}

	// Register string primitive prototype
	ctx.SetPrimitivePrototype("string", stringProtoType)

	// Add prototype property to constructor
	stringCtorType = stringCtorType.WithProperty("prototype", stringProtoType)

	// Define String constructor in global environment
	return ctx.DefineGlobal("String", stringCtorType)
}

func (s *StringInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create String.prototype inheriting from Object.prototype
	// String.prototype itself has [[PrimitiveValue]] = "" per ES spec
	stringProto := vm.NewObject(objectProto).AsPlainObject()
	stringProto.SetOwn("[[PrimitiveValue]]", vm.NewString(""))
	// String.prototype.length should be 0 (length of empty string "")
	stringProto.SetOwnNonEnumerable("length", vm.NumberValue(0))

	// Add String prototype methods
	stringProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis()

		// If this is a primitive string, return it
		if thisStr.Type() == vm.TypeString {
			return thisStr, nil
		}

		// If this is a String wrapper object, extract [[PrimitiveValue]]
		if thisStr.IsObject() {
			if primitiveVal, exists := thisStr.AsPlainObject().GetOwn("[[PrimitiveValue]]"); exists {
				return primitiveVal, nil
			}
		}

		// TypeError: String.prototype.valueOf requires that 'this' be a String
		return vm.Undefined, vmInstance.NewTypeError("String.prototype.valueOf requires that 'this' be a String")
	}))

	stringProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis()

		// If this is a primitive string, return it
		if thisStr.Type() == vm.TypeString {
			return thisStr, nil
		}

		// If this is a String wrapper object, extract [[PrimitiveValue]]
		if thisStr.IsObject() {
			if primitiveVal, exists := thisStr.AsPlainObject().GetOwn("[[PrimitiveValue]]"); exists {
				if primitiveVal.Type() == vm.TypeString {
					return primitiveVal, nil
				}
			}
		}

		// TypeError: String.prototype.toString requires that 'this' be a String
		return vm.Undefined, vmInstance.NewTypeError("String.prototype.toString requires that 'this' be a String")
	}))

	stringProto.SetOwnNonEnumerable("charAt", vm.NewNativeFunction(1, false, "charAt", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "charAt"); err != nil {
			return vm.Undefined, err
		}
		// Get string value - for String wrapper objects, extract [[PrimitiveValue]]
		// For other objects, call ToPrimitive("string") to get proper conversion
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// Default to index 0 if no argument provided, using proper ToInteger conversion
		// that throws TypeError for Symbols
		index := 0
		if len(args) >= 1 {
			index, err = toIntegerWithVM(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		// Convert to UTF-16 code units for proper JavaScript string semantics
		utf16Units := vm.StringToUTF16(thisStr)
		if index < 0 || index >= len(utf16Units) {
			return vm.NewString(""), nil
		}
		// Return the character at the UTF-16 index
		return vm.NewString(string(rune(utf16Units[index]))), nil
	}))

	stringProto.SetOwnNonEnumerable("charCodeAt", vm.NewNativeFunction(1, false, "charCodeAt", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "charCodeAt"); err != nil {
			return vm.Undefined, err
		}
		// Get string value - for String wrapper objects, extract [[PrimitiveValue]]
		// For other objects, call ToPrimitive("string") to get proper conversion
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// Default to index 0 if no argument provided, using proper ToInteger conversion
		// that throws TypeError for Symbols
		index := 0
		if len(args) >= 1 {
			index, err = toIntegerWithVM(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		// Convert to UTF-16 code units for proper JavaScript string semantics
		utf16Units := vm.StringToUTF16(thisStr)
		if index < 0 || index >= len(utf16Units) {
			return vm.NaN, nil // Return NaN for out of bounds
		}
		return vm.NumberValue(float64(utf16Units[index])), nil
	}))

	// String.prototype.at - returns character at relative index (supports negative indices)
	stringProto.SetOwnNonEnumerable("at", vm.NewNativeFunction(1, false, "at", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "at"); err != nil {
			return vm.Undefined, err
		}
		// Get string value - for String wrapper objects, extract [[PrimitiveValue]]
		// For other objects, call ToPrimitive("string") to get proper conversion
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// Convert to UTF-16 code units for proper JavaScript string semantics
		utf16Units := vm.StringToUTF16(thisStr)
		length := len(utf16Units)

		// Default to 0 if no argument provided, using proper ToInteger conversion
		// that throws TypeError for Symbols
		index := 0
		if len(args) >= 1 {
			var err error
			index, err = toIntegerWithVM(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}

		// Handle negative indices (relative to end)
		if index < 0 {
			index = length + index
		}

		// Return undefined if out of bounds
		if index < 0 || index >= length {
			return vm.Undefined, nil
		}

		// Return the character at the UTF-16 index
		return vm.NewString(string(rune(utf16Units[index]))), nil
	}))

	// String.prototype.codePointAt - returns code point at position (handles surrogate pairs)
	stringProto.SetOwnNonEnumerable("codePointAt", vm.NewNativeFunction(1, false, "codePointAt", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "codePointAt"); err != nil {
			return vm.Undefined, err
		}
		// Get string value - for String wrapper objects, extract [[PrimitiveValue]]
		// For other objects, call ToPrimitive("string") to get proper conversion
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// Convert to UTF-16 code units for proper JavaScript string semantics
		utf16Units := vm.StringToUTF16(thisStr)
		size := len(utf16Units)

		// Default to 0 if no argument provided, using proper ToInteger conversion
		// that throws TypeError for Symbols
		position := 0
		if len(args) >= 1 {
			var err error
			position, err = toIntegerWithVM(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}

		// Return undefined if out of bounds
		if position < 0 || position >= size {
			return vm.Undefined, nil
		}

		// Get the first code unit
		first := utf16Units[position]

		// If not a lead surrogate, or at end of string, return the code unit
		if first < 0xD800 || first > 0xDBFF || position+1 >= size {
			return vm.NumberValue(float64(first)), nil
		}

		// Get the second code unit
		second := utf16Units[position+1]

		// If not a trail surrogate, return the lead surrogate
		if second < 0xDC00 || second > 0xDFFF {
			return vm.NumberValue(float64(first)), nil
		}

		// Compute the full code point from surrogate pair
		// Formula: (first - 0xD800) * 0x400 + (second - 0xDC00) + 0x10000
		codePoint := (int(first)-0xD800)*0x400 + (int(second) - 0xDC00) + 0x10000
		return vm.NumberValue(float64(codePoint)), nil
	}))

	stringProto.SetOwnNonEnumerable("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "slice"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		length := len(thisStr)

		// Convert start argument via ToInteger (which calls ToPrimitive for objects)
		// ToInteger(undefined) = 0
		var start int
		if len(args) >= 1 && args[0].Type() != vm.TypeUndefined {
			startArg := args[0]
			if startArg.IsObject() {
				vmInstance.EnterHelperCall()
				startArg = vmInstance.ToPrimitive(startArg, "number")
				vmInstance.ExitHelperCall()
				if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
					return vm.Undefined, nil
				}
			}
			startFloat := startArg.ToFloat()
			if math.IsNaN(startFloat) {
				start = 0
			} else {
				start = int(startFloat)
			}
		}
		if start < 0 {
			start = length + start
			if start < 0 {
				start = 0
			}
		} else if start > length {
			start = length
		}

		end := length
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			// Convert end argument via ToInteger (which calls ToPrimitive for objects)
			endArg := args[1]
			if endArg.IsObject() {
				vmInstance.EnterHelperCall()
				endArg = vmInstance.ToPrimitive(endArg, "number")
				vmInstance.ExitHelperCall()
				if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
					return vm.Undefined, nil
				}
			}
			end = int(endArg.ToFloat())
			if math.IsNaN(endArg.ToFloat()) {
				end = 0
			}
			if end < 0 {
				end = length + end
				if end < 0 {
					end = 0
				}
			} else if end > length {
				end = length
			}
		}
		if start >= end {
			return vm.NewString(""), nil
		}
		return vm.NewString(thisStr[start:end]), nil
	}))

	stringProto.SetOwnNonEnumerable("substring", vm.NewNativeFunction(2, false, "substring", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "substring"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		length := len(thisStr)
		if len(args) < 1 || args[0].Type() == vm.TypeUndefined {
			return vm.NewString(thisStr), nil
		}
		start := int(args[0].ToFloat())
		if start < 0 {
			start = 0
		} else if start > length {
			start = length
		}
		end := length
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			end = int(args[1].ToFloat())
			if end < 0 {
				end = 0
			} else if end > length {
				end = length
			}
		}
		// substring swaps start and end if start > end
		if start > end {
			start, end = end, start
		}
		return vm.NewString(thisStr[start:end]), nil
	}))

	stringProto.SetOwnNonEnumerable("substr", vm.NewNativeFunction(2, false, "substr", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "substr"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		length := len(thisStr)
		if len(args) < 1 || args[0].Type() == vm.TypeUndefined {
			return vm.NewString(thisStr), nil
		}
		start := int(args[0].ToFloat())
		if start < 0 {
			start = length + start
			if start < 0 {
				start = 0
			}
		} else if start >= length {
			return vm.NewString(""), nil
		}
		substrLength := length - start
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			substrLength = int(args[1].ToFloat())
			if substrLength < 0 {
				return vm.NewString(""), nil
			}
		}
		end := start + substrLength
		if end > length {
			end = length
		}
		return vm.NewString(thisStr[start:end]), nil
	}))

	stringProto.SetOwnNonEnumerable("indexOf", vm.NewNativeFunction(1, false, "indexOf", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "indexOf"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			if err == ErrVMUnwinding {
				return vm.Undefined, nil // Abort: let VM continue unwinding
			}
			return vm.Undefined, err
		}
		// indexOf(undefined) searches for "undefined"
		// Convert searchString - throws TypeError for Symbols
		searchStr := "undefined"
		if len(args) >= 1 && args[0].Type() != vm.TypeUndefined {
			searchStr, err = getStringValueWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil // Abort: let VM continue unwinding
				}
				return vm.Undefined, err
			}
		}
		position := 0
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			// Convert position - throws TypeError for Symbols
			positionF, err := toNumberWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil // Abort: let VM continue unwinding
				}
				return vm.Undefined, err
			}
			position = int(positionF)
			if position < 0 {
				position = 0
			}
		}
		if position >= len(thisStr) {
			return vm.NumberValue(-1), nil
		}
		index := strings.Index(thisStr[position:], searchStr)
		if index == -1 {
			return vm.NumberValue(-1), nil
		}
		return vm.NumberValue(float64(position + index)), nil
	}))

	stringProto.SetOwnNonEnumerable("lastIndexOf", vm.NewNativeFunction(1, false, "lastIndexOf", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "lastIndexOf"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// lastIndexOf(undefined) searches for "undefined"
		// Convert searchString - throws TypeError for Symbols
		searchStr := "undefined"
		if len(args) >= 1 && args[0].Type() != vm.TypeUndefined {
			searchStr, err = getStringValueWithVM(vmInstance, args[0])
			if err != nil {
				return vm.Undefined, err
			}
		}
		position := len(thisStr)
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			// Convert position - throws TypeError for Symbols
			positionF, err := toNumberWithVM(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
			position = int(positionF)
			if position < 0 {
				position = 0
			} else if position > len(thisStr) {
				position = len(thisStr)
			}
		}
		// Ensure we don't go past the end of the string
		endPos := position + len(searchStr)
		if endPos > len(thisStr) {
			endPos = len(thisStr)
		}
		index := strings.LastIndex(thisStr[:endPos], searchStr)
		return vm.NumberValue(float64(index)), nil
	}))

	stringProto.SetOwnNonEnumerable("includes", vm.NewNativeFunction(1, false, "includes", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "includes"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) < 1 || args[0].Type() == vm.TypeUndefined {
			// includes(undefined) should search for "undefined"
			return vm.BooleanValue(strings.Contains(thisStr, "undefined")), nil
		}
		// Reject RegExp arguments
		if isRegExp(args[0]) {
			return vm.Undefined, vmInstance.NewTypeError("First argument to String.prototype.includes must not be a regular expression")
		}
		// Convert searchString - throws TypeError for Symbols
		searchStr, err := getStringValueWithVM(vmInstance, args[0])
		if err != nil {
			return vm.Undefined, err
		}
		position := 0
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			// Convert position - throws TypeError for Symbols
			positionF, err := toNumberWithVM(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
			position = int(positionF)
			if position < 0 {
				position = 0
			}
		}
		// Empty string is found at any position (including at the end)
		if searchStr == "" {
			return vm.BooleanValue(true), nil
		}
		if position >= len(thisStr) {
			return vm.BooleanValue(false), nil
		}
		return vm.BooleanValue(strings.Contains(thisStr[position:], searchStr)), nil
	}))

	stringProto.SetOwnNonEnumerable("startsWith", vm.NewNativeFunction(1, false, "startsWith", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "startsWith"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) < 1 || args[0].Type() == vm.TypeUndefined {
			// startsWith(undefined) should search for "undefined"
			return vm.BooleanValue(strings.HasPrefix(thisStr, "undefined")), nil
		}
		// Reject RegExp arguments
		if isRegExp(args[0]) {
			return vm.Undefined, vmInstance.NewTypeError("First argument to String.prototype.startsWith must not be a regular expression")
		}
		// Convert searchString - throws TypeError for Symbols
		searchStr, err := getStringValueWithVM(vmInstance, args[0])
		if err != nil {
			return vm.Undefined, err
		}
		position := 0
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			// Convert position - throws TypeError for Symbols
			positionF, err := toNumberWithVM(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
			position = int(positionF)
			if position < 0 {
				position = 0
			}
		}
		if position >= len(thisStr) {
			return vm.BooleanValue(false), nil
		}
		return vm.BooleanValue(strings.HasPrefix(thisStr[position:], searchStr)), nil
	}))

	stringProto.SetOwnNonEnumerable("endsWith", vm.NewNativeFunction(1, false, "endsWith", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "endsWith"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) < 1 || args[0].Type() == vm.TypeUndefined {
			// endsWith(undefined) should search for "undefined"
			return vm.BooleanValue(strings.HasSuffix(thisStr, "undefined")), nil
		}
		// Reject RegExp arguments
		if isRegExp(args[0]) {
			return vm.Undefined, vmInstance.NewTypeError("First argument to String.prototype.endsWith must not be a regular expression")
		}
		// Convert searchString - throws TypeError for Symbols
		searchStr, err := getStringValueWithVM(vmInstance, args[0])
		if err != nil {
			return vm.Undefined, err
		}
		length := len(thisStr)
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			// Convert position - throws TypeError for Symbols
			lengthF, err := toNumberWithVM(vmInstance, args[1])
			if err != nil {
				return vm.Undefined, err
			}
			length = int(lengthF)
			if length < 0 {
				length = 0
			} else if length > len(thisStr) {
				length = len(thisStr)
			}
		}
		if length < len(searchStr) {
			return vm.BooleanValue(false), nil
		}
		return vm.BooleanValue(strings.HasSuffix(thisStr[:length], searchStr)), nil
	}))

	stringProto.SetOwnNonEnumerable("toLowerCase", vm.NewNativeFunction(0, false, "toLowerCase", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "toLowerCase"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(strings.ToLower(thisStr)), nil
	}))

	stringProto.SetOwnNonEnumerable("toUpperCase", vm.NewNativeFunction(0, false, "toUpperCase", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "toUpperCase"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(strings.ToUpper(thisStr)), nil
	}))

	// String.prototype.toLocaleLowerCase - returns string converted to lower case, according to locale
	stringProto.SetOwnNonEnumerable("toLocaleLowerCase", vm.NewNativeFunction(0, false, "toLocaleLowerCase", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "toLocaleLowerCase"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// For now, use simple lower case (proper locale support requires Intl)
		return vm.NewString(strings.ToLower(thisStr)), nil
	}))

	// String.prototype.toLocaleUpperCase - returns string converted to upper case, according to locale
	stringProto.SetOwnNonEnumerable("toLocaleUpperCase", vm.NewNativeFunction(0, false, "toLocaleUpperCase", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "toLocaleUpperCase"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// For now, use simple upper case (proper locale support requires Intl)
		return vm.NewString(strings.ToUpper(thisStr)), nil
	}))

	// String.prototype.normalize - returns Unicode Normalization Form of the string
	stringProto.SetOwnNonEnumerable("normalize", vm.NewNativeFunction(0, false, "normalize", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "normalize"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// Default form is "NFC"
		formName := "NFC"
		if len(args) >= 1 && args[0].Type() != vm.TypeUndefined {
			formName = args[0].ToString()
		}

		var result string
		switch formName {
		case "NFC":
			result = norm.NFC.String(thisStr)
		case "NFD":
			result = norm.NFD.String(thisStr)
		case "NFKC":
			result = norm.NFKC.String(thisStr)
		case "NFKD":
			result = norm.NFKD.String(thisStr)
		default:
			return vm.Undefined, vmInstance.NewRangeError("The normalization form should be one of NFC, NFD, NFKC, NFKD")
		}
		return vm.NewString(result), nil
	}))

	// String.prototype.localeCompare - compares two strings in the current locale
	stringProto.SetOwnNonEnumerable("localeCompare", vm.NewNativeFunction(1, false, "localeCompare", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "localeCompare"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		if len(args) < 1 {
			return vm.NumberValue(0), nil
		}

		compareStr := args[0].ToString()

		// Simple comparison (proper locale support requires Intl.Collator)
		if thisStr < compareStr {
			return vm.NumberValue(-1), nil
		} else if thisStr > compareStr {
			return vm.NumberValue(1), nil
		}
		return vm.NumberValue(0), nil
	}))

	stringProto.SetOwnNonEnumerable("trim", vm.NewNativeFunction(0, false, "trim", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "trim"); err != nil {
			return vm.Undefined, err
		}
		// Use proper string conversion (ToPrimitive for objects)
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(trimECMAScriptWhitespace(thisStr)), nil
	}))

	stringProto.SetOwnNonEnumerable("trimStart", vm.NewNativeFunction(0, false, "trimStart", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "trimStart"); err != nil {
			return vm.Undefined, err
		}
		// Use proper string conversion (ToPrimitive for objects)
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(trimLeftECMAScriptWhitespace(thisStr)), nil
	}))

	stringProto.SetOwnNonEnumerable("trimEnd", vm.NewNativeFunction(0, false, "trimEnd", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "trimEnd"); err != nil {
			return vm.Undefined, err
		}
		// Use proper string conversion (ToPrimitive for objects)
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(trimRightECMAScriptWhitespace(thisStr)), nil
	}))

	// String.prototype.isWellFormed - checks if string has no lone surrogates (ES2024)
	stringProto.SetOwnNonEnumerable("isWellFormed", vm.NewNativeFunction(0, false, "isWellFormed", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "isWellFormed"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// Convert to UTF-16 and check for lone surrogates
		utf16Units := vm.StringToUTF16(thisStr)
		for i := 0; i < len(utf16Units); i++ {
			c := utf16Units[i]
			// Check if it's a high surrogate (0xD800-0xDBFF)
			if c >= 0xD800 && c <= 0xDBFF {
				// Must be followed by a low surrogate
				if i+1 >= len(utf16Units) || utf16Units[i+1] < 0xDC00 || utf16Units[i+1] > 0xDFFF {
					return vm.BooleanValue(false), nil
				}
				i++ // Skip the low surrogate
			} else if c >= 0xDC00 && c <= 0xDFFF {
				// Low surrogate without preceding high surrogate
				return vm.BooleanValue(false), nil
			}
		}
		return vm.BooleanValue(true), nil
	}))

	// String.prototype.toWellFormed - replaces lone surrogates with U+FFFD (ES2024)
	stringProto.SetOwnNonEnumerable("toWellFormed", vm.NewNativeFunction(0, false, "toWellFormed", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "toWellFormed"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// Convert to UTF-16 and replace lone surrogates with U+FFFD
		utf16Units := vm.StringToUTF16(thisStr)
		result := make([]uint16, 0, len(utf16Units))

		for i := 0; i < len(utf16Units); i++ {
			c := utf16Units[i]
			// Check if it's a high surrogate (0xD800-0xDBFF)
			if c >= 0xD800 && c <= 0xDBFF {
				// Check if followed by low surrogate
				if i+1 < len(utf16Units) && utf16Units[i+1] >= 0xDC00 && utf16Units[i+1] <= 0xDFFF {
					// Valid surrogate pair - keep both
					result = append(result, c, utf16Units[i+1])
					i++ // Skip the low surrogate
				} else {
					// Lone high surrogate - replace with U+FFFD
					result = append(result, 0xFFFD)
				}
			} else if c >= 0xDC00 && c <= 0xDFFF {
				// Low surrogate without preceding high surrogate - replace with U+FFFD
				result = append(result, 0xFFFD)
			} else {
				// Normal character
				result = append(result, c)
			}
		}

		return vm.NewString(vm.UTF16ToString(result)), nil
	}))

	stringProto.SetOwnNonEnumerable("repeat", vm.NewNativeFunction(1, false, "repeat", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "repeat"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) < 1 {
			return vm.NewString(""), nil
		}
		count := int(args[0].ToFloat())
		if count < 0 {
			// TODO: Should throw RangeError
			return vm.NewString(""), nil
		}
		if count == 0 || thisStr == "" {
			return vm.NewString(""), nil
		}
		return vm.NewString(strings.Repeat(thisStr, count)), nil
	}))

	// String.prototype.padStart - pads string at the start to reach target length
	stringProto.SetOwnNonEnumerable("padStart", vm.NewNativeFunction(1, false, "padStart", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "padStart"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		utf16Units := vm.StringToUTF16(thisStr)
		stringLength := len(utf16Units)

		// Get target length
		targetLength := 0
		if len(args) >= 1 {
			targetLength = vmInstance.ToInteger(args[0])
		}

		// If target length is less than or equal to current length, return original
		if targetLength <= stringLength {
			return vm.NewString(thisStr), nil
		}

		// Get pad string (default to space)
		padString := " "
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			var padErr error
			padString, padErr = getStringValueWithVM(vmInstance, args[1])
			if padErr != nil {
				return vm.Undefined, padErr
			}
		}

		// If pad string is empty, return original
		if padString == "" {
			return vm.NewString(thisStr), nil
		}

		// Calculate fill length and build result
		fillLength := targetLength - stringLength
		padUtf16 := vm.StringToUTF16(padString)

		// Build the padding
		var result strings.Builder
		for result.Len() < fillLength {
			for _, unit := range padUtf16 {
				if result.Len() >= fillLength {
					break
				}
				result.WriteRune(rune(unit))
			}
		}
		// Truncate if necessary and append original string
		padding := result.String()
		if len(vm.StringToUTF16(padding)) > fillLength {
			// Truncate to exact fill length
			padRunes := []rune(padding)
			padding = string(padRunes[:fillLength])
		}
		return vm.NewString(padding + thisStr), nil
	}))

	// String.prototype.padEnd - pads string at the end to reach target length
	stringProto.SetOwnNonEnumerable("padEnd", vm.NewNativeFunction(1, false, "padEnd", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "padEnd"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		utf16Units := vm.StringToUTF16(thisStr)
		stringLength := len(utf16Units)

		// Get target length
		targetLength := 0
		if len(args) >= 1 {
			targetLength = vmInstance.ToInteger(args[0])
		}

		// If target length is less than or equal to current length, return original
		if targetLength <= stringLength {
			return vm.NewString(thisStr), nil
		}

		// Get pad string (default to space)
		padString := " "
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			var padErr error
			padString, padErr = getStringValueWithVM(vmInstance, args[1])
			if padErr != nil {
				return vm.Undefined, padErr
			}
		}

		// If pad string is empty, return original
		if padString == "" {
			return vm.NewString(thisStr), nil
		}

		// Calculate fill length and build result
		fillLength := targetLength - stringLength
		padUtf16 := vm.StringToUTF16(padString)

		// Build the padding
		var result strings.Builder
		for result.Len() < fillLength {
			for _, unit := range padUtf16 {
				if result.Len() >= fillLength {
					break
				}
				result.WriteRune(rune(unit))
			}
		}
		// Truncate if necessary and prepend original string
		padding := result.String()
		if len(vm.StringToUTF16(padding)) > fillLength {
			// Truncate to exact fill length
			padRunes := []rune(padding)
			padding = string(padRunes[:fillLength])
		}
		return vm.NewString(thisStr + padding), nil
	}))

	stringProto.SetOwnNonEnumerable("concat", vm.NewNativeFunction(1, true, "concat", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "concat"); err != nil {
			return vm.Undefined, err
		}
		// Get string value - for String wrapper objects, extract [[PrimitiveValue]]
		// For other objects, call ToPrimitive("string") to get proper conversion
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		result := thisStr
		for i := 0; i < len(args); i++ {
			// Also convert arguments using ToPrimitive for proper object handling
			argStr, argErr := getStringValueWithVM(vmInstance, args[i])
			if argErr != nil {
				return vm.Undefined, argErr
			}
			result += argStr
		}
		return vm.NewString(result), nil
	}))

	stringProto.SetOwnNonEnumerable("split", vm.NewNativeFunction(2, false, "split", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "split"); err != nil {
			return vm.Undefined, err
		}

		// Get separator and limit arguments
		var separatorArg vm.Value = vm.Undefined
		var limitArg vm.Value = vm.Undefined
		if len(args) > 0 {
			separatorArg = args[0]
		}
		if len(args) > 1 {
			limitArg = args[1]
		}

		// ECMAScript step 2: If separator is neither undefined nor null,
		// check for Symbol.split method and call it if exists
		if separatorArg.Type() != vm.TypeUndefined && separatorArg.Type() != vm.TypeNull {
			// Check for Symbol.split method (use GetSymbolPropertyWithGetter to handle accessor properties)
			vmInstance.EnterHelperCall()
			splitter, ok, err := vmInstance.GetSymbolPropertyWithGetter(separatorArg, SymbolSplit)
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}
			if err != nil {
				return vm.Undefined, err
			}
			if ok && splitter.Type() != vm.TypeUndefined {
				// Check if splitter is callable
				if splitter.IsCallable() {
					// Call splitter with separator as 'this' and (O, limit) as arguments
					vmInstance.EnterHelperCall()
					result, err := vmInstance.Call(splitter, separatorArg, []vm.Value{thisVal, limitArg})
					vmInstance.ExitHelperCall()
					if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
						return vm.Undefined, nil
					}
					if err != nil {
						return vm.Undefined, err
					}
					return result, nil
				}
			}
		}

		// ECMAScript step 3: Let S be ? ToString(O).
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) == 0 {
			// No separator - return array with whole string
			return vm.NewArrayWithArgs([]vm.Value{vm.NewString(thisStr)}), nil
		}

		// ECMAScript: Let lim be ? ToUint32(limit).
		// If limit is undefined, use 2^32-1 (effectively unlimited)
		var limit uint32 = 0xFFFFFFFF // 2^32 - 1 (unlimited)
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			limitArg := args[1]
			// For objects, call ToPrimitive to invoke valueOf
			if limitArg.IsObject() {
				vmInstance.EnterHelperCall()
				limitArg = vmInstance.ToPrimitive(limitArg, "number")
				vmInstance.ExitHelperCall()
				if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
					return vm.Undefined, nil
				}
			}
			limitVal := limitArg.ToFloat()
			// ToUint32: NaN, +0, -0, +∞, -∞ all convert to 0
			if math.IsNaN(limitVal) || math.IsInf(limitVal, 0) || limitVal == 0 {
				limit = 0
			} else {
				// Truncate toward zero and take modulo 2^32
				truncated := math.Trunc(limitVal)
				if truncated < 0 {
					// For negative values, add 2^32 to get the unsigned equivalent
					limit = uint32(int64(truncated) & 0xFFFFFFFF)
				} else if truncated > math.MaxUint32 {
					limit = uint32(int64(truncated) & 0xFFFFFFFF)
				} else {
					limit = uint32(truncated)
				}
			}
		}
		// Check if separator is a RegExp (before any conversion)
		isRegExp := separatorArg.IsRegExp()
		separatorIsUndefined := separatorArg.Type() == vm.TypeUndefined

		// ECMAScript step 7: Let R be ? ToString(separator).
		// For non-RegExp separators, convert to string (must happen before limit=0 check)
		var separator string
		if !isRegExp && !separatorIsUndefined {
			// For objects, call ToPrimitive to invoke custom toString
			sepVal := separatorArg
			if sepVal.IsObject() {
				vmInstance.EnterHelperCall()
				sepVal = vmInstance.ToPrimitive(sepVal, "string")
				vmInstance.ExitHelperCall()
				if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
					return vm.Undefined, nil
				}
			}
			separator = sepVal.ToString()
		}

		// ECMAScript step 8: If lim = 0, return empty array
		if limit == 0 {
			return vm.NewArray(), nil
		}

		// ECMAScript step 9: if separator is undefined, return array with whole string
		if separatorIsUndefined {
			return vm.NewArrayWithArgs([]vm.Value{vm.NewString(thisStr)}), nil
		}

		if isRegExp {
			// RegExp separator
			regex := separatorArg.AsRegExpObject()
			// Check for deferred compile error
			if regex.HasCompileError() {
				return vm.Undefined, vmInstance.NewSyntaxError("Invalid regular expression: " + regex.GetCompileError())
			}

			parts := regex.Split(thisStr, -1)
			if uint32(len(parts)) > limit {
				parts = parts[:limit]
			}
			elements := make([]vm.Value, len(parts))
			for i, part := range parts {
				elements[i] = vm.NewString(part)
			}
			return vm.NewArrayWithArgs(elements), nil
		} else {
			// String separator
			if separator == "" {
				// Split into individual characters
				runes := []rune(thisStr)
				count := uint32(len(runes))
				if limit < count {
					count = limit
				}
				elements := make([]vm.Value, count)
				for i := uint32(0); i < count; i++ {
					elements[i] = vm.NewString(string(runes[i]))
				}
				return vm.NewArrayWithArgs(elements), nil
			}

			// Normal string split
			parts := strings.Split(thisStr, separator)
			if uint32(len(parts)) > limit {
				parts = parts[:limit]
			}
			elements := make([]vm.Value, len(parts))
			for i, part := range parts {
				elements[i] = vm.NewString(part)
			}
			return vm.NewArrayWithArgs(elements), nil
		}
	}))

	stringProto.SetOwnNonEnumerable("replace", vm.NewNativeFunction(2, false, "replace", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "replace"); err != nil {
			return vm.Undefined, err
		}

		// Get searchValue and replaceValue from args
		var searchArg vm.Value
		var replaceArg vm.Value
		if len(args) > 0 {
			searchArg = args[0]
		} else {
			searchArg = vm.Undefined
		}
		if len(args) > 1 {
			replaceArg = args[1]
		} else {
			replaceArg = vm.Undefined
		}

		// Step 2: If searchValue is not null/undefined, check for Symbol.replace
		if searchArg.Type() != vm.TypeUndefined && searchArg.Type() != vm.TypeNull {
			vmInstance.EnterHelperCall()
			replacer, ok, err := vmInstance.GetSymbolPropertyWithGetter(searchArg, SymbolReplace)
			vmInstance.ExitHelperCall()
			if err != nil {
				return vm.Undefined, err
			}
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}

			// If replacer exists and is callable, invoke it
			if ok && replacer.Type() != vm.TypeUndefined && replacer.Type() != vm.TypeNull {
				if replacer.IsCallable() {
					vmInstance.EnterHelperCall()
					result, err := vmInstance.Call(replacer, searchArg, []vm.Value{thisVal, replaceArg})
					vmInstance.ExitHelperCall()
					if err != nil {
						return vm.Undefined, err
					}
					if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
						return vm.Undefined, nil
					}
					return vm.NewString(result.ToString()), nil
				}
			}
		}

		// Fallback: Convert this to string
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// Convert searchArg to string
		searchValue := searchArg.ToString()

		// Check if replaceArg is callable
		isCallable := replaceArg.IsCallable()

		// Find first occurrence
		idx := strings.Index(thisStr, searchValue)
		if idx == -1 {
			return vm.NewString(thisStr), nil
		}

		var replacement string
		if isCallable {
			// Call replacer function with (matched, position, string)
			vmInstance.EnterHelperCall()
			res, err := vmInstance.Call(replaceArg, vm.Undefined, []vm.Value{
				vm.NewString(searchValue),
				vm.NumberValue(float64(idx)),
				vm.NewString(thisStr),
			})
			vmInstance.ExitHelperCall()
			if err != nil {
				return vm.Undefined, err
			}
			replacement = res.ToString()
		} else {
			// Process $ patterns in replacement string
			replaceStr := replaceArg.ToString()
			// For simple string search, we only have the match itself (no groups)
			// Build a pseudo match slice: [start, end]
			match := []int{idx, idx + len(searchValue)}
			replacement = processStringReplacementPattern(thisStr, match, replaceStr)
		}

		result := thisStr[:idx] + replacement + thisStr[idx+len(searchValue):]
		return vm.NewString(result), nil
	}))

	// String.prototype.replaceAll - replaces all occurrences of a search string/pattern
	stringProto.SetOwnNonEnumerable("replaceAll", vm.NewNativeFunction(2, false, "replaceAll", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "replaceAll"); err != nil {
			return vm.Undefined, err
		}

		// Get searchValue and replaceValue from args
		var searchArg vm.Value
		var replaceArg vm.Value
		if len(args) > 0 {
			searchArg = args[0]
		} else {
			searchArg = vm.Undefined
		}
		if len(args) > 1 {
			replaceArg = args[1]
		} else {
			replaceArg = vm.Undefined
		}

		// Step 2: If searchValue is not null/undefined, check for Symbol.replace
		if searchArg.Type() != vm.TypeUndefined && searchArg.Type() != vm.TypeNull {
			// If it's a RegExp, check if it's global
			if searchArg.IsRegExp() {
				regexObj := searchArg.AsRegExpObject()
				if !regexObj.IsGlobal() {
					return vm.Undefined, vmInstance.NewTypeError("String.prototype.replaceAll called with a non-global RegExp argument")
				}
			}

			vmInstance.EnterHelperCall()
			replacer, ok, err := vmInstance.GetSymbolPropertyWithGetter(searchArg, SymbolReplace)
			vmInstance.ExitHelperCall()
			if err != nil {
				return vm.Undefined, err
			}
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}

			// If replacer exists and is callable, invoke it
			if ok && replacer.Type() != vm.TypeUndefined && replacer.Type() != vm.TypeNull {
				if replacer.IsCallable() {
					vmInstance.EnterHelperCall()
					result, err := vmInstance.Call(replacer, searchArg, []vm.Value{thisVal, replaceArg})
					vmInstance.ExitHelperCall()
					if err != nil {
						return vm.Undefined, err
					}
					if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
						return vm.Undefined, nil
					}
					return vm.NewString(result.ToString()), nil
				}
			}
		}

		// Fallback: Convert this to string
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// Convert searchArg to string
		searchValue := searchArg.ToString()

		// Check if replaceArg is callable
		isCallable := replaceArg.IsCallable()

		// Handle empty string search
		if searchValue == "" {
			// Empty string matches between every character
			var result strings.Builder
			runes := []rune(thisStr)
			for i := 0; i <= len(runes); i++ {
				var replacement string
				if isCallable {
					vmInstance.EnterHelperCall()
					res, err := vmInstance.Call(replaceArg, vm.Undefined, []vm.Value{
						vm.NewString(""),
						vm.NumberValue(float64(i)),
						vm.NewString(thisStr),
					})
					vmInstance.ExitHelperCall()
					if err != nil {
						return vm.Undefined, err
					}
					replacement = res.ToString()
				} else {
					match := []int{i, i}
					replacement = processStringReplacementPattern(thisStr, match, replaceArg.ToString())
				}
				result.WriteString(replacement)
				if i < len(runes) {
					result.WriteRune(runes[i])
				}
			}
			return vm.NewString(result.String()), nil
		}

		// Find all occurrences and replace
		var result strings.Builder
		lastIndex := 0
		for {
			idx := strings.Index(thisStr[lastIndex:], searchValue)
			if idx == -1 {
				break
			}
			idx += lastIndex // Convert to absolute index

			// Add the part before the match
			result.WriteString(thisStr[lastIndex:idx])

			var replacement string
			if isCallable {
				vmInstance.EnterHelperCall()
				res, err := vmInstance.Call(replaceArg, vm.Undefined, []vm.Value{
					vm.NewString(searchValue),
					vm.NumberValue(float64(idx)),
					vm.NewString(thisStr),
				})
				vmInstance.ExitHelperCall()
				if err != nil {
					return vm.Undefined, err
				}
				replacement = res.ToString()
			} else {
				match := []int{idx, idx + len(searchValue)}
				replacement = processStringReplacementPattern(thisStr, match, replaceArg.ToString())
			}

			result.WriteString(replacement)
			lastIndex = idx + len(searchValue)
		}

		// Add the rest of the string
		result.WriteString(thisStr[lastIndex:])
		return vm.NewString(result.String()), nil
	}))

	stringProto.SetOwnNonEnumerable("match", vm.NewNativeFunction(1, false, "match", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "match"); err != nil {
			return vm.Undefined, err
		}

		// Get regexp argument
		var regexpArg vm.Value = vm.Undefined
		if len(args) >= 1 {
			regexpArg = args[0]
		}

		// ECMAScript step 2: If regexp is neither undefined nor null,
		// check for Symbol.match method and call it if exists
		if regexpArg.Type() != vm.TypeUndefined && regexpArg.Type() != vm.TypeNull {
			// Check for Symbol.match method
			vmInstance.EnterHelperCall()
			matcher, ok, err := vmInstance.GetSymbolPropertyWithGetter(regexpArg, SymbolMatch)
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}
			if err != nil {
				return vm.Undefined, err
			}
			if ok && matcher.Type() != vm.TypeUndefined && matcher.Type() != vm.TypeNull {
				if matcher.IsCallable() {
					// Call matcher with regexp as 'this' and (O) as argument
					vmInstance.EnterHelperCall()
					result, err := vmInstance.Call(matcher, regexpArg, []vm.Value{thisVal})
					vmInstance.ExitHelperCall()
					if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
						return vm.Undefined, nil
					}
					if err != nil {
						return vm.Undefined, err
					}
					return result, nil
				}
			}
		}

		// ECMAScript step 3: Let string be ? ToString(O).
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// ECMAScript step 4: Let rx be ? RegExpCreate(regexp, undefined).
		// ECMAScript step 5: Return ? Invoke(rx, @@match, « string »).
		var pattern string
		if regexpArg.Type() == vm.TypeUndefined {
			pattern = ""
		} else if regexpArg.IsRegExp() {
			// Use existing RegExp's pattern
			regexObj := regexpArg.AsRegExpObject()
			pattern = regexObj.GetSource()
		} else if regexpArg.IsObject() {
			vmInstance.EnterHelperCall()
			primVal := vmInstance.ToPrimitive(regexpArg, "string")
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}
			pattern = primVal.ToString()
		} else {
			pattern = regexpArg.ToString()
		}

		// Create a new RegExp
		rx, regexpErr := vm.NewRegExp(pattern, "")
		if regexpErr != nil {
			return vm.Undefined, vmInstance.NewSyntaxError("Invalid regular expression: " + regexpErr.Error())
		}

		// Invoke rx[@@match](string)
		vmInstance.EnterHelperCall()
		matchMethod, ok, err := vmInstance.GetSymbolPropertyWithGetter(rx, SymbolMatch)
		vmInstance.ExitHelperCall()
		if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
			return vm.Undefined, nil
		}
		if err != nil {
			return vm.Undefined, err
		}
		if ok && matchMethod.IsCallable() {
			vmInstance.EnterHelperCall()
			result, err := vmInstance.Call(matchMethod, rx, []vm.Value{vm.NewString(thisStr)})
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}
			if err != nil {
				return vm.Undefined, err
			}
			return result, nil
		}

		// Fallback: no Symbol.match method
		return vm.Null, nil
	}))

	// String.prototype.matchAll - returns an iterator of all matches
	stringProto.SetOwnNonEnumerable("matchAll", vm.NewNativeFunction(1, false, "matchAll", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "matchAll"); err != nil {
			return vm.Undefined, err
		}

		// Get regexp argument
		var regexpArg vm.Value = vm.Undefined
		if len(args) >= 1 {
			regexpArg = args[0]
		}

		// ECMAScript step 2: If regexp is neither undefined nor null,
		// check for Symbol.matchAll method and call it if exists
		if regexpArg.Type() != vm.TypeUndefined && regexpArg.Type() != vm.TypeNull {
			// First check: if regexp is a RegExp, check if it's global (required for matchAll)
			if regexpArg.IsRegExp() {
				regexObj := regexpArg.AsRegExpObject()
				if !regexObj.IsGlobal() {
					return vm.Undefined, vmInstance.NewTypeError("String.prototype.matchAll called with a non-global RegExp argument")
				}
			}

			// Check for Symbol.matchAll method
			vmInstance.EnterHelperCall()
			matcher, ok, err := vmInstance.GetSymbolPropertyWithGetter(regexpArg, SymbolMatchAll)
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}
			if err != nil {
				return vm.Undefined, err
			}
			if ok && matcher.Type() != vm.TypeUndefined && matcher.Type() != vm.TypeNull {
				if matcher.IsCallable() {
					// Call matcher with regexp as 'this' and (O) as argument
					vmInstance.EnterHelperCall()
					result, err := vmInstance.Call(matcher, regexpArg, []vm.Value{thisVal})
					vmInstance.ExitHelperCall()
					if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
						return vm.Undefined, nil
					}
					if err != nil {
						return vm.Undefined, err
					}
					return result, nil
				}
			}
		}

		// ECMAScript step 3: Let string be ? ToString(O).
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// ECMAScript step 4: Let rx be ? RegExpCreate(regexp, "g").
		// ECMAScript step 5: Return ? Invoke(rx, @@matchAll, « string »).
		// Always create a NEW RegExp to avoid inheriting overridden Symbol.matchAll from the instance
		var pattern string
		var flags string = "g"
		if regexpArg.IsRegExp() {
			// Get pattern and flags from existing RegExp
			regexObj := regexpArg.AsRegExpObject()
			pattern = regexObj.GetSource()
			flags = regexObj.GetFlags()
			if !strings.Contains(flags, "g") {
				flags = flags + "g"
			}
		} else if regexpArg.Type() == vm.TypeUndefined {
			pattern = ""
		} else if regexpArg.IsObject() {
			vmInstance.EnterHelperCall()
			primVal := vmInstance.ToPrimitive(regexpArg, "string")
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}
			pattern = primVal.ToString()
		} else {
			pattern = regexpArg.ToString()
		}
		// Create a new global RegExp (fresh object without overridden properties)
		rx, regexpErr := vm.NewRegExp(pattern, flags)
		if regexpErr != nil {
			return vm.Undefined, vmInstance.NewSyntaxError("Invalid regular expression: " + regexpErr.Error())
		}

		// Invoke rx[@@matchAll](string)
		vmInstance.EnterHelperCall()
		matchAllMethod, ok, err := vmInstance.GetSymbolPropertyWithGetter(rx, SymbolMatchAll)
		vmInstance.ExitHelperCall()
		if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
			return vm.Undefined, nil
		}
		if err != nil {
			return vm.Undefined, err
		}
		if ok && matchAllMethod.IsCallable() {
			vmInstance.EnterHelperCall()
			result, err := vmInstance.Call(matchAllMethod, rx, []vm.Value{vm.NewString(thisStr)})
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}
			if err != nil {
				return vm.Undefined, err
			}
			return result, nil
		}

		// Fallback: no Symbol.matchAll method
		return vm.Undefined, vmInstance.NewTypeError("RegExp.prototype[@@matchAll] is not a function")
	}))

	stringProto.SetOwnNonEnumerable("search", vm.NewNativeFunction(1, false, "search", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "search"); err != nil {
			return vm.Undefined, err
		}

		// Get regexp argument
		var regexpArg vm.Value = vm.Undefined
		if len(args) >= 1 {
			regexpArg = args[0]
		}

		// ECMAScript step 2: If regexp is neither undefined nor null,
		// check for Symbol.search method and call it if exists
		if regexpArg.Type() != vm.TypeUndefined && regexpArg.Type() != vm.TypeNull {
			// Check for Symbol.search method
			vmInstance.EnterHelperCall()
			searcher, ok, err := vmInstance.GetSymbolPropertyWithGetter(regexpArg, SymbolSearch)
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}
			if err != nil {
				return vm.Undefined, err
			}
			if ok && searcher.Type() != vm.TypeUndefined && searcher.Type() != vm.TypeNull {
				// Check if searcher is callable (GetMethod returns undefined if null)
				if searcher.IsCallable() {
					// Call searcher with regexp as 'this' and (O) as argument
					vmInstance.EnterHelperCall()
					result, err := vmInstance.Call(searcher, regexpArg, []vm.Value{thisVal})
					vmInstance.ExitHelperCall()
					if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
						return vm.Undefined, nil
					}
					if err != nil {
						return vm.Undefined, err
					}
					return result, nil
				}
			}
		}

		// ECMAScript step 3: Let string be ? ToString(O).
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// ECMAScript step 4: Let rx be ? RegExpCreate(regexp, undefined).
		// ECMAScript step 5: Return ? Invoke(rx, @@search, « string »).
		var rx vm.Value
		if regexpArg.IsRegExp() {
			// RegExp argument - use directly
			rx = regexpArg
		} else {
			// Convert to pattern string and create a RegExp
			var pattern string
			if regexpArg.Type() == vm.TypeUndefined {
				// When undefined, search for "" (empty regex matches at position 0)
				pattern = ""
			} else if regexpArg.IsObject() {
				// Call ToPrimitive for objects
				vmInstance.EnterHelperCall()
				primVal := vmInstance.ToPrimitive(regexpArg, "string")
				vmInstance.ExitHelperCall()
				if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
					return vm.Undefined, nil
				}
				pattern = primVal.ToString()
			} else {
				pattern = regexpArg.ToString()
			}
			// Create RegExp object (this will inherit Symbol.search from RegExp.prototype)
			var regexpErr error
			rx, regexpErr = vm.NewRegExp(pattern, "")
			if regexpErr != nil {
				// Invalid regex pattern - return SyntaxError
				return vm.Undefined, vmInstance.NewSyntaxError("Invalid regular expression: " + regexpErr.Error())
			}
		}

		// Invoke rx[@@search](string) - this calls Symbol.search on the regex
		vmInstance.EnterHelperCall()
		searchMethod, ok, err := vmInstance.GetSymbolPropertyWithGetter(rx, SymbolSearch)
		vmInstance.ExitHelperCall()
		if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
			return vm.Undefined, nil
		}
		if err != nil {
			return vm.Undefined, err
		}
		if ok && searchMethod.IsCallable() {
			vmInstance.EnterHelperCall()
			result, err := vmInstance.Call(searchMethod, rx, []vm.Value{vm.NewString(thisStr)})
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}
			if err != nil {
				return vm.Undefined, err
			}
			return result, nil
		}

		// Fallback if no Symbol.search (shouldn't happen for RegExp)
		return vm.NumberValue(-1), nil
	}))

	// Declare stringCtor - will be assigned after ctorWithProps is created
	var stringCtor vm.Value

	// Create String constructor with static methods
	ctorWithProps := vm.NewConstructorWithProps(1, true, "String", func(args []vm.Value) (vm.Value, error) {
		// Determine the primitive string value
		var primitiveValue string
		if len(args) == 0 {
			primitiveValue = ""
		} else {
			arg := args[0]

			// ECMAScript 22.1.1.1: String(value)
			// If NewTarget is undefined (called as function) AND value is a Symbol,
			// return SymbolDescriptiveString(value)
			// If NewTarget is defined (called with new) AND value is a Symbol,
			// throw TypeError (ToString on Symbol throws)
			if arg.Type() == vm.TypeSymbol {
				if vmInstance.IsConstructorCall() {
					// new String(symbol) - throws TypeError
					return vm.Undefined, vmInstance.NewTypeError("Cannot convert a Symbol value to a string")
				}
				// String(symbol) - returns descriptive string
				return vm.NewString(arg.ToString()), nil
			}

			// For objects and functions, use ToPrimitive with hint "string"
			// Functions in ECMAScript are objects and may have custom toString properties
			if arg.IsObject() || arg.IsFunction() || arg.IsClosure() || arg.IsNativeFunction() {
				// Track that we're in a helper call so exception handlers can be found
				vmInstance.EnterHelperCall()
				primVal := vmInstance.ToPrimitive(arg, "string")
				vmInstance.ExitHelperCall()
				// Check if ToPrimitive threw an exception
				// Return nil (not ErrVMUnwinding) to let call.go's unwinding check handle it
				if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
					return vm.Undefined, nil
				}
				// Check if ToPrimitive returned a Symbol
				if primVal.Type() == vm.TypeSymbol {
					if vmInstance.IsConstructorCall() {
						return vm.Undefined, vmInstance.NewTypeError("Cannot convert a Symbol value to a string")
					}
					return vm.NewString(primVal.ToString()), nil
				}
				primitiveValue = primVal.ToString()
			} else {
				primitiveValue = arg.ToString()
			}
		}

		// If called with 'new', return a String wrapper object
		if vmInstance.IsConstructorCall() {
			return vmInstance.NewStringObject(primitiveValue), nil
		}
		// Otherwise, return primitive string (type coercion)
		return vm.NewString(primitiveValue), nil
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(stringProto))

	// Add static methods
	// ECMAScript spec: String.fromCharCode.length = 1
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("fromCharCode", vm.NewNativeFunction(1, true, "fromCharCode", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewString(""), nil
		}
		// Use runes to properly handle Unicode code points
		result := make([]rune, len(args))
		for i, arg := range args {
			// BigInt to Number throws TypeError per ECMAScript
			if arg.Type() == vm.TypeBigInt {
				return vm.Undefined, vmInstance.NewTypeError("Cannot convert a BigInt value to a number")
			}
			// Use ToNumber to properly call ToPrimitive for objects
			vmInstance.EnterHelperCall()
			num := vmInstance.ToNumber(arg)
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.Undefined, nil
			}
			// ToUint16: If number is not finite (NaN, +∞, or -∞), return +0
			var code int
			if math.IsNaN(num) || math.IsInf(num, 0) {
				code = 0
			} else {
				// Truncate to integer and mask to 16 bits
				code = int(math.Trunc(num)) & 0xFFFF
			}
			result[i] = rune(code)
		}
		return vm.NewString(string(result)), nil
	}))

	// String.fromCodePoint - creates string from code points (supports full Unicode range)
	// ECMAScript spec: String.fromCodePoint.length = 1
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("fromCodePoint", vm.NewNativeFunction(1, true, "fromCodePoint", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewString(""), nil
		}
		var result strings.Builder
		for _, arg := range args {
			// Use toNumberWithVM to throw TypeError for Symbols
			codePointF, err := toNumberWithVM(vmInstance, arg)
			if err != nil {
				return vm.Undefined, err
			}
			// Check for NaN - not an integral number
			if codePointF != codePointF { // NaN check
				return vm.Undefined, vmInstance.NewRangeError("Invalid code point NaN")
			}
			// Check for Infinity - not an integral number
			if codePointF > 1e20 || codePointF < -1e20 {
				return vm.Undefined, vmInstance.NewRangeError("Invalid code point Infinity")
			}
			// Check if it's an integer (no decimal part)
			codePoint := int(codePointF)
			if float64(codePoint) != codePointF {
				return vm.Undefined, vmInstance.NewRangeError("Invalid code point " + fmt.Sprintf("%v", codePointF))
			}
			// Check for valid code point range
			if codePoint < 0 || codePoint > 0x10FFFF {
				return vm.Undefined, vmInstance.NewRangeError("Invalid code point " + fmt.Sprintf("%d", codePoint))
			}
			// Convert code point to string (handles surrogate pairs automatically)
			result.WriteRune(rune(codePoint))
		}
		return vm.NewString(result.String()), nil
	}))

	// String.raw - template tag function for raw string literals
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("raw", vm.NewNativeFunction(1, true, "raw", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}

		// Get the template object (first argument)
		template := args[0]
		if !template.IsObject() && !template.IsArray() {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}

		// Get the 'raw' property from template
		var rawArray vm.Value
		if template.IsObject() {
			if po := template.AsPlainObject(); po != nil {
				if rawProp, exists := po.Get("raw"); exists {
					rawArray = rawProp
				} else {
					return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
				}
			} else {
				return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
			}
		} else {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}

		// Get length and elements from raw array
		var rawElements []vm.Value
		if rawArray.IsArray() {
			arr := rawArray.AsArray()
			if arr != nil {
				rawLength := arr.Length()
				rawElements = make([]vm.Value, rawLength)
				for i := 0; i < rawLength; i++ {
					rawElements[i] = arr.Get(i)
				}
			}
		} else if rawArray.IsObject() {
			// Could be array-like object
			if po := rawArray.AsPlainObject(); po != nil {
				if lengthVal, exists := po.Get("length"); exists {
					rawLength := int(lengthVal.ToFloat())
					rawElements = make([]vm.Value, rawLength)
					for i := 0; i < rawLength; i++ {
						if elem, exists := po.Get(fmt.Sprintf("%d", i)); exists {
							rawElements[i] = elem
						} else {
							rawElements[i] = vm.Undefined
						}
					}
				}
			}
		}

		if len(rawElements) == 0 {
			return vm.NewString(""), nil
		}

		// Build the result by interleaving raw strings and substitutions
		var result strings.Builder
		substitutions := args[1:] // Rest of args are substitutions

		for i, rawElement := range rawElements {
			// Convert raw element to string
			result.WriteString(rawElement.ToString())

			// Add substitution if available (between raw strings)
			if i < len(rawElements)-1 && i < len(substitutions) {
				result.WriteString(substitutions[i].ToString())
			}
		}

		return vm.NewString(result.String()), nil
	}))

	stringCtor = ctorWithProps

	// Set constructor property on prototype
	stringProto.SetOwnNonEnumerable("constructor", stringCtor)

	// Add Symbol.iterator implementation for strings (native symbol key)
	strIterFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "[Symbol.iterator]"); err != nil {
			return vm.Undefined, err
		}
		// Get string value - for String wrapper objects, extract [[PrimitiveValue]]
		// For other objects, call ToPrimitive("string") to get proper conversion
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// Create a string iterator object
		return createStringIterator(vmInstance, thisStr), nil
	})
	stringProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), strIterFn, nil, nil, nil)

	// Set String prototype in VM
	vmInstance.StringPrototype = vm.NewValueFromPlainObject(stringProto)

	// Register String constructor as global
	return ctx.DefineGlobal("String", stringCtor)
}

// createMatchAllIterator creates an iterator for String.prototype.matchAll
func createMatchAllIterator(vmInstance *vm.VM, str string, allMatches [][]int) vm.Value {
	// Create iterator object inheriting from %RegExpStringIteratorPrototype%
	iterator := vm.NewObject(vmInstance.RegExpStringIteratorPrototype).AsPlainObject()

	// Iterator state: current match index
	currentMatchIndex := 0

	// Add next() method to iterator
	iterator.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		// Create iterator result object {value, done}
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

		if currentMatchIndex >= len(allMatches) {
			// Iterator is exhausted
			result.SetOwn("value", vm.Undefined)
			result.SetOwn("done", vm.BooleanValue(true))
		} else {
			// Get current match indices
			matchIndices := allMatches[currentMatchIndex]

			// Create match result array (similar to RegExp.exec result)
			matchResult := vm.NewArray()
			arr := matchResult.AsArray()

			// Add full match first
			if matchIndices[0] >= 0 && matchIndices[1] >= 0 {
				arr.Append(vm.NewString(str[matchIndices[0]:matchIndices[1]]))
			} else {
				arr.Append(vm.Undefined)
			}

			// Add capture groups
			for i := 2; i < len(matchIndices); i += 2 {
				if matchIndices[i] >= 0 && matchIndices[i+1] >= 0 {
					arr.Append(vm.NewString(str[matchIndices[i]:matchIndices[i+1]]))
				} else {
					arr.Append(vm.Undefined)
				}
			}

			// Add index property
			arr.SetOwn("index", vm.NumberValue(float64(matchIndices[0])))
			// Add input property
			arr.SetOwn("input", vm.NewString(str))

			result.SetOwn("value", matchResult)
			result.SetOwn("done", vm.BooleanValue(false))
			currentMatchIndex++
		}

		return vm.NewValueFromPlainObject(result), nil
	}))

	return vm.NewValueFromPlainObject(iterator)
}

// createStringIterator creates an iterator object for string iteration
func createStringIterator(vmInstance *vm.VM, str string) vm.Value {
	// Create iterator object inheriting from StringIteratorPrototype
	iterator := vm.NewObject(vmInstance.StringIteratorPrototype).AsPlainObject()

	// Convert string to UTF-16 code units for proper JavaScript semantics
	// JavaScript strings are UTF-16 encoded, so we need to iterate by code points,
	// which may span two UTF-16 code units (surrogate pairs)
	utf16Units := vm.StringToUTF16(str)

	// Iterator state: current index into UTF-16 code units
	currentIndex := 0

	// Add next() method to iterator
	iterator.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		// Create iterator result object {value, done}
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

		if currentIndex >= len(utf16Units) {
			// Iterator is exhausted
			result.SetOwnNonEnumerable("value", vm.Undefined)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
		} else {
			// Return current code point as a string and advance
			// ECMAScript string iteration yields code points, not UTF-16 code units
			// Check for surrogate pairs and combine them into a single code point
			c := utf16Units[currentIndex]
			var codeUnits []uint16

			// Check if this is a high surrogate (0xD800-0xDBFF)
			if c >= 0xD800 && c <= 0xDBFF && currentIndex+1 < len(utf16Units) {
				// Check if next is a low surrogate (0xDC00-0xDFFF)
				low := utf16Units[currentIndex+1]
				if low >= 0xDC00 && low <= 0xDFFF {
					// Valid surrogate pair - yield both as a single code point
					codeUnits = []uint16{c, low}
					currentIndex += 2 // Advance past both surrogates
				} else {
					// Lone high surrogate - yield as-is (use WTF-8 encoding)
					codeUnits = []uint16{c}
					currentIndex++
				}
			} else {
				// Regular BMP character or lone low surrogate
				// Use WTF-8 encoding to preserve lone surrogates
				codeUnits = []uint16{c}
				currentIndex++
			}

			// Convert UTF-16 code units back to a Go string (preserves surrogates via WTF-8)
			result.SetOwnNonEnumerable("value", vm.NewString(vm.UTF16ToString(codeUnits)))
			result.SetOwnNonEnumerable("done", vm.BooleanValue(false))
		}

		return vm.NewValueFromPlainObject(result), nil
	}))

	return vm.NewValueFromPlainObject(iterator)
}

package builtins

import (
	"fmt"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

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
			}
		}
		// For other objects, use ToPrimitive to properly call toString/valueOf
		primVal := vmInstance.ToPrimitive(val, "string")
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
		'\n', // LF (Line Feed)
		'\v', // VT (Vertical Tab)
		'\f', // FF (Form Feed)
		'\r', // CR (Carriage Return)
		' ',  // Space
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
	// Get the Iterator<T> generic type if available
	if iteratorType, found := ctx.GetType("Iterator"); found {
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
		return vm.Undefined, fmt.Errorf("String.prototype.valueOf requires that 'this' be a String")
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
		return vm.Undefined, fmt.Errorf("String.prototype.toString requires that 'this' be a String")
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
		index := 0
		if len(args) >= 1 {
			index = vmInstance.ToInteger(args[0])
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
		index := 0
		if len(args) >= 1 {
			index = vmInstance.ToInteger(args[0])
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
		index := 0
		if len(args) >= 1 {
			index = vmInstance.ToInteger(args[0])
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
		position := 0
		if len(args) >= 1 {
			position = vmInstance.ToInteger(args[0])
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
		if len(args) < 1 || args[0].Type() == vm.TypeUndefined {
			return vm.NewString(thisStr), nil
		}
		start := int(args[0].ToFloat())
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
			end = int(args[1].ToFloat())
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

	stringProto.SetOwnNonEnumerable("indexOf", vm.NewNativeFunction(2, false, "indexOf", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "indexOf"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// indexOf(undefined) searches for "undefined"
		searchStr := "undefined"
		if len(args) >= 1 && args[0].Type() != vm.TypeUndefined {
			searchStr = args[0].ToString()
		}
		position := 0
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			position = int(args[1].ToFloat())
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

	stringProto.SetOwnNonEnumerable("lastIndexOf", vm.NewNativeFunction(2, false, "lastIndexOf", func(args []vm.Value) (vm.Value, error) {
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
		searchStr := "undefined"
		if len(args) >= 1 && args[0].Type() != vm.TypeUndefined {
			searchStr = args[0].ToString()
		}
		position := len(thisStr)
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			position = int(args[1].ToFloat())
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
		searchStr := args[0].ToString()
		position := 0
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			position = int(args[1].ToFloat())
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
		searchStr := args[0].ToString()
		position := 0
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			position = int(args[1].ToFloat())
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
		searchStr := args[0].ToString()
		length := len(thisStr)
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			length = int(args[1].ToFloat())
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
	stringProto.SetOwnNonEnumerable("normalize", vm.NewNativeFunction(1, false, "normalize", func(args []vm.Value) (vm.Value, error) {
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
	stringProto.SetOwnNonEnumerable("padStart", vm.NewNativeFunction(2, false, "padStart", func(args []vm.Value) (vm.Value, error) {
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
	stringProto.SetOwnNonEnumerable("padEnd", vm.NewNativeFunction(2, false, "padEnd", func(args []vm.Value) (vm.Value, error) {
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

	stringProto.SetOwnNonEnumerable("concat", vm.NewNativeFunction(0, true, "concat", func(args []vm.Value) (vm.Value, error) {
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
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) == 0 {
			// No separator - return array with whole string
			return vm.NewArrayWithArgs([]vm.Value{vm.NewString(thisStr)}), nil
		}

		separatorArg := args[0]
		limit := -1
		if len(args) >= 2 {
			limitVal := args[1].ToFloat()
			if limitVal > 0 {
				limit = int(limitVal)
			} else if limitVal <= 0 {
				return vm.NewArray(), nil
			}
		}

		if separatorArg.IsRegExp() {
			// RegExp separator
			regex := separatorArg.AsRegExpObject()
			compiledRegex := regex.GetCompiledRegex()

			parts := compiledRegex.Split(thisStr, -1)
			if limit > 0 && len(parts) > limit {
				parts = parts[:limit]
			}
			elements := make([]vm.Value, len(parts))
			for i, part := range parts {
				elements[i] = vm.NewString(part)
			}
			return vm.NewArrayWithArgs(elements), nil
		} else {
			// String separator
			separator := separatorArg.ToString()
			if separator == "" {
				// Split into individual characters
				runes := []rune(thisStr)
				count := len(runes)
				if limit > 0 && limit < count {
					count = limit
				}
				elements := make([]vm.Value, count)
				for i := 0; i < count; i++ {
					elements[i] = vm.NewString(string(runes[i]))
				}
				return vm.NewArrayWithArgs(elements), nil
			}

			// Normal string split
			parts := strings.Split(thisStr, separator)
			if limit > 0 && len(parts) > limit {
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
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) < 2 {
			return vm.NewString(thisStr), nil
		}

		searchArg := args[0]
		replaceValue := args[1].ToString()

		if searchArg.IsRegExp() {
			// RegExp argument
			regex := searchArg.AsRegExpObject()
			compiledRegex := regex.GetCompiledRegex()

			if regex.IsGlobal() {
				// Global replace: replace all matches
				result := compiledRegex.ReplaceAllString(thisStr, replaceValue)
				return vm.NewString(result), nil
			} else {
				// Non-global: replace first match only
				if loc := compiledRegex.FindStringIndex(thisStr); loc != nil {
					// Replace only the first match
					result := thisStr[:loc[0]] + replaceValue + thisStr[loc[1]:]
					return vm.NewString(result), nil
				}
				return vm.NewString(thisStr), nil
			}
		} else {
			// String argument - legacy behavior (replace first occurrence only)
			searchValue := searchArg.ToString()
			result := strings.Replace(thisStr, searchValue, replaceValue, 1)
			return vm.NewString(result), nil
		}
	}))

	// String.prototype.replaceAll - replaces all occurrences of a search string/pattern
	stringProto.SetOwnNonEnumerable("replaceAll", vm.NewNativeFunction(2, false, "replaceAll", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "replaceAll"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) < 2 {
			return vm.NewString(thisStr), nil
		}

		searchArg := args[0]
		replaceValue := args[1].ToString()

		if searchArg.IsRegExp() {
			// RegExp argument - must be global
			regexObj := searchArg.AsRegExpObject()
			if !regexObj.IsGlobal() {
				return vm.Undefined, vmInstance.NewTypeError("String.prototype.replaceAll called with a non-global RegExp argument")
			}
			compiledRegex := regexObj.GetCompiledRegex()
			result := compiledRegex.ReplaceAllString(thisStr, replaceValue)
			return vm.NewString(result), nil
		} else {
			// String argument - replace all occurrences
			searchValue := searchArg.ToString()
			result := strings.ReplaceAll(thisStr, searchValue, replaceValue)
			return vm.NewString(result), nil
		}
	}))

	stringProto.SetOwnNonEnumerable("match", vm.NewNativeFunction(1, false, "match", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "match"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) < 1 {
			return vm.Null, nil
		}

		arg := args[0]
		if arg.IsRegExp() {
			// RegExp argument
			regex := arg.AsRegExpObject()
			compiledRegex := regex.GetCompiledRegex()

			if regex.IsGlobal() {
				// Global match: find all matches
				matches := compiledRegex.FindAllString(thisStr, -1)
				if len(matches) == 0 {
					return vm.Null, nil
				}
				result := vm.NewArray()
				arr := result.AsArray()
				for _, match := range matches {
					arr.Append(vm.NewString(match))
				}
				return result, nil
			} else {
				// Non-global: find first match with capture groups
				matches := compiledRegex.FindStringSubmatch(thisStr)
				if matches == nil {
					return vm.Null, nil
				}
				result := vm.NewArray()
				arr := result.AsArray()
				for _, match := range matches {
					arr.Append(vm.NewString(match))
				}
				return result, nil
			}
		} else {
			// String argument - legacy behavior
			pattern := arg.ToString()
			if strings.Contains(thisStr, pattern) {
				return vm.NewArrayWithArgs([]vm.Value{vm.NewString(pattern)}), nil
			}
			return vm.Null, nil
		}
	}))

	// String.prototype.matchAll - returns an iterator of all matches
	stringProto.SetOwnNonEnumerable("matchAll", vm.NewNativeFunction(1, false, "matchAll", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "matchAll"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("String.prototype.matchAll called with undefined or null argument")
		}

		arg := args[0]
		var compiledRegex *regexp.Regexp

		if arg.IsRegExp() {
			// RegExp argument - must be global
			regexObj := arg.AsRegExpObject()
			if !regexObj.IsGlobal() {
				return vm.Undefined, vmInstance.NewTypeError("String.prototype.matchAll called with a non-global RegExp argument")
			}
			compiledRegex = regexObj.GetCompiledRegex()
		} else {
			// String argument - create a global RegExp from it
			pattern := regexp.QuoteMeta(arg.ToString())
			var err error
			compiledRegex, err = regexp.Compile(pattern)
			if err != nil {
				return vm.Undefined, fmt.Errorf("Invalid regular expression: %s", err.Error())
			}
		}

		// Find all matches with indices
		allMatches := compiledRegex.FindAllStringSubmatchIndex(thisStr, -1)

		// Create iterator that returns match results
		return createMatchAllIterator(vmInstance, thisStr, allMatches, compiledRegex), nil
	}))

	stringProto.SetOwnNonEnumerable("search", vm.NewNativeFunction(1, false, "search", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		// RequireObjectCoercible: throw TypeError for null/undefined
		if err := requireObjectCoercible(vmInstance, thisVal, "search"); err != nil {
			return vm.Undefined, err
		}
		thisStr, err := getStringValueWithVM(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if len(args) < 1 {
			return vm.NumberValue(-1), nil
		}

		arg := args[0]
		if arg.IsRegExp() {
			// RegExp argument
			regex := arg.AsRegExpObject()
			compiledRegex := regex.GetCompiledRegex()

			loc := compiledRegex.FindStringIndex(thisStr)
			if loc == nil {
				return vm.NumberValue(-1), nil
			}
			return vm.NumberValue(float64(loc[0])), nil
		} else {
			// String argument - legacy behavior
			searchValue := arg.ToString()
			index := strings.Index(thisStr, searchValue)
			return vm.NumberValue(float64(index)), nil
		}
	}))

	// Create String constructor
	stringCtor := vm.NewNativeFunction(-1, true, "String", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewString(""), nil
		}
		return vm.NewString(args[0].ToString()), nil
	})

	// Make it a proper constructor with static methods
	ctorWithProps := vm.NewConstructorWithProps(1, true, "String", func(args []vm.Value) (vm.Value, error) {
		// Determine the primitive string value
		var primitiveValue string
		if len(args) == 0 {
			primitiveValue = ""
		} else {
			arg := args[0]
			// For objects, use ToPrimitive with hint "string"
			if arg.IsObject() {
				// Try calling toString() first, then valueOf()
				if toStringMethod, err := vmInstance.GetProperty(arg, "toString"); err == nil && toStringMethod.IsCallable() {
					if result, err := vmInstance.Call(toStringMethod, arg, []vm.Value{}); err == nil {
						if !result.IsObject() {
							primitiveValue = result.ToString()
						} else {
							// toString() returned an object, try valueOf()
							if valueOfMethod, err := vmInstance.GetProperty(arg, "valueOf"); err == nil && valueOfMethod.IsCallable() {
								if result, err := vmInstance.Call(valueOfMethod, arg, []vm.Value{}); err == nil {
									if !result.IsObject() {
										primitiveValue = result.ToString()
									} else {
										// Both returned objects, fall back to default
										primitiveValue = arg.ToString()
									}
								} else {
									primitiveValue = arg.ToString()
								}
							} else {
								primitiveValue = arg.ToString()
							}
						}
					} else {
						primitiveValue = arg.ToString()
					}
				} else {
					primitiveValue = arg.ToString()
				}
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
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("fromCharCode", vm.NewNativeFunction(0, true, "fromCharCode", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewString(""), nil
		}
		// Use runes to properly handle Unicode code points
		result := make([]rune, len(args))
		for i, arg := range args {
			// Use ToNumber to properly call ToPrimitive for objects
			code := int(vmInstance.ToNumber(arg)) & 0xFFFF // Mask to 16 bits like JS
			result[i] = rune(code)
		}
		return vm.NewString(string(result)), nil
	}))

	// String.fromCodePoint - creates string from code points (supports full Unicode range)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("fromCodePoint", vm.NewNativeFunction(0, true, "fromCodePoint", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewString(""), nil
		}
		var result strings.Builder
		for _, arg := range args {
			// Use ToNumber to properly call ToPrimitive for objects
			codePoint := int(vmInstance.ToNumber(arg))
			// Check for valid code point range
			if codePoint < 0 || codePoint > 0x10FFFF {
				return vm.Undefined, vmInstance.NewRangeError("Invalid code point")
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
		thisStr := vmInstance.GetThis().ToString()

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
func createMatchAllIterator(vmInstance *vm.VM, str string, allMatches [][]int, compiledRegex *regexp.Regexp) vm.Value {
	// Create iterator object inheriting from Object.prototype
	iterator := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

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
			matchResult.AsPlainObject().SetOwn("index", vm.NumberValue(float64(matchIndices[0])))
			// Add input property
			matchResult.AsPlainObject().SetOwn("input", vm.NewString(str))

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
	// Create iterator object inheriting from Object.prototype
	iterator := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Iterator state: current index
	currentIndex := 0

	// Add next() method to iterator
	iterator.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		// Create iterator result object {value, done}
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

		if currentIndex >= len(str) {
			// Iterator is exhausted
			result.SetOwnNonEnumerable("value", vm.Undefined)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
		} else {
			// Return current character and advance
			char := string(str[currentIndex])
			result.SetOwnNonEnumerable("value", vm.NewString(char))
			result.SetOwnNonEnumerable("done", vm.BooleanValue(false))
			currentIndex++
		}

		return vm.NewValueFromPlainObject(result), nil
	}))

	return vm.NewValueFromPlainObject(iterator)
}

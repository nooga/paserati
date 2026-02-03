package builtins

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
	"unsafe"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type JSONInitializer struct{}

// rawJSONObjects tracks objects created by JSON.rawJSON for isRawJSON checks
// We use a map with PlainObject pointers as keys
var rawJSONObjects = make(map[*vm.PlainObject]bool)

func (j *JSONInitializer) Name() string {
	return "JSON"
}

func (j *JSONInitializer) Priority() int {
	return PriorityJSON // 101 - After Math
}

func (j *JSONInitializer) InitTypes(ctx *TypeContext) error {
	// Create JSON namespace type with parse and stringify methods
	jsonType := types.NewObjectType().
		WithProperty("parse", types.NewSimpleFunction([]types.Type{types.String}, types.Any)).
		WithProperty("stringify", types.NewOptionalFunction(
			[]types.Type{types.Any, types.Any, types.Any}, // value, replacer, space
			types.String,
			[]bool{false, true, true}, // value is required, replacer and space are optional
		))

	// Define JSON namespace in global environment
	return ctx.DefineGlobal("JSON", jsonType)
}

func (j *JSONInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Create JSON object with Object.prototype as its prototype (ECMAScript spec)
	jsonObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Set @@toStringTag to "JSON" so Object.prototype.toString.call(JSON) returns "[object JSON]"
	// Per ECMAScript 25.5: { [[Writable]]: false, [[Enumerable]]: false, [[Configurable]]: true }
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		falseVal := false
		trueVal := true
		jsonObj.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("JSON"),
			&falseVal, // writable: false
			&falseVal, // enumerable: false
			&trueVal,  // configurable: true
		)
	}

	// Add parse method - Per ECMAScript spec, JSON.parse.length = 2 (text, reviver)
	jsonObj.SetOwnNonEnumerable("parse", vm.NewNativeFunction(2, false, "parse", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			// Throw SyntaxError for missing argument
			return vm.Undefined, ctx.VM.NewSyntaxError("Unexpected end of JSON input")
		}

		// Per spec: Let JText be ? ToString(text)
		// This calls ToPrimitive with string hint for objects
		textArg := args[0]
		if textArg.IsObject() || textArg.IsCallable() {
			textArg = vmInstance.ToPrimitive(textArg, "string")
		}
		text := textArg.ToString()

		// If reviver is provided and callable, use source-tracking parser
		if len(args) >= 2 && args[1].IsCallable() {
			reviver := args[1]
			// Parse with source tracking for json-parse-with-source feature
			val, sourceMap, err := parseJSONWithSource(vmInstance, text)
			if err != nil {
				// Wrap parse error as SyntaxError exception
				ctor, _ := ctx.VM.GetGlobal("SyntaxError")
				if ctor != vm.Undefined {
					errObj, _ := ctx.VM.Call(ctor, vm.Undefined, []vm.Value{vm.NewString(err.Error())})
					return vm.Undefined, ctx.VM.NewExceptionError(errObj)
				}
				return vm.Undefined, err
			}

			// Create root object with empty string key holding the parsed value
			root := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
			root.SetOwn("", val)
			rootVal := vm.NewValueFromPlainObject(root)
			// Apply reviver recursively with source map
			return internalizeJSONProperty(vmInstance, rootVal, "", reviver, sourceMap, "")
		}

		// No reviver - use standard parser (faster)
		val, err := parseJSONToValueWithPrototypes(vmInstance, text)
		if err != nil {
			// Wrap parse error as SyntaxError exception
			ctor, _ := ctx.VM.GetGlobal("SyntaxError")
			if ctor != vm.Undefined {
				errObj, _ := ctx.VM.Call(ctor, vm.Undefined, []vm.Value{vm.NewString(err.Error())})
				return vm.Undefined, ctx.VM.NewExceptionError(errObj)
			}
			return vm.Undefined, err
		}

		return val, nil
	}))

	// Add stringify method (supports optional replacer and space parameters)
	// Per ECMAScript spec, JSON.stringify.length = 3 (value, replacer, space)
	jsonObj.SetOwnNonEnumerable("stringify", vm.NewNativeFunction(3, true, "stringify", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, nil
		}

		value := args[0]

		// Process replacer parameter (args[1])
		var replacerFunc vm.Value
		var propertyList []string // nil means no filtering, empty slice means filter all
		if len(args) >= 2 && args[1] != vm.Undefined && args[1] != vm.Null {
			replacer := args[1]

			if replacer.IsCallable() {
				// Replacer is a function
				replacerFunc = replacer
			} else if replacer.Type() == vm.TypeArray {
				// Replacer is an array - build property whitelist (even if empty)
				propertyList = []string{} // Initialize as empty slice, not nil
				arr := replacer.AsArray()
				seen := make(map[string]bool)
				for i := 0; i < arr.Length(); i++ {
					elem := arr.Get(i)
					var item string

					// Per ECMAScript spec: If v is String or Number primitive, use as-is
					// If v is Object with [[StringData]] or [[NumberData]], call ToString(v)
					if elem.Type() == vm.TypeString {
						item = elem.ToString()
					} else if elem.Type() == vm.TypeFloatNumber || elem.Type() == vm.TypeIntegerNumber {
						num := elem.ToFloat()
						if math.IsNaN(num) {
							item = "NaN"
						} else if math.IsInf(num, 1) {
							item = "Infinity"
						} else if math.IsInf(num, -1) {
							item = "-Infinity"
						} else if num == 0 && math.Signbit(num) {
							// Negative zero should be "0" not "-0"
							item = "0"
						} else {
							item = strconv.FormatFloat(num, 'f', -1, 64)
						}
					} else if elem.Type() == vm.TypeObject {
						// Check if it's a String or Number wrapper object
						if obj := elem.AsPlainObject(); obj != nil {
							if _, ok := obj.GetOwn("[[PrimitiveValue]]"); ok {
								// Has [[PrimitiveValue]] - it's a wrapper, call ToString via ToPrimitive
								primVal := vmInstance.ToPrimitive(elem, "string")
								item = primVal.ToString()
							}
						}
					}

					// Only add if not already in list (deduplication)
					if item != "" && !seen[item] {
						propertyList = append(propertyList, item)
						seen[item] = true
					}
				}
			}
		}

		// Process space parameter (args[2])
		var gap string
		if len(args) >= 3 && args[2] != vm.Undefined && args[2] != vm.Null {
			space := args[2]

			// Handle Number/String objects per spec:
			// - If space has [[NumberData]] internal slot, call ToNumber (which calls valueOf)
			// - If space has [[StringData]] internal slot, call ToString (which calls toString)
			if space.Type() == vm.TypeObject {
				obj := space.AsPlainObject()
				// Check if it has [[PrimitiveValue]] property (our representation of boxed primitives)
				if pv, ok := obj.GetOwn("[[PrimitiveValue]]"); ok {
					// Determine if it's a Number or String object based on primitive type
					if pv.Type() == vm.TypeFloatNumber || pv.Type() == vm.TypeIntegerNumber {
						// Number object - call ToNumber via ToPrimitive with number hint
						space = vmInstance.ToPrimitive(space, "number")
					} else if pv.Type() == vm.TypeString {
						// String object - call ToString via ToPrimitive with string hint
						space = vmInstance.ToPrimitive(space, "string")
					}
				}
			}

			if space.Type() == vm.TypeFloatNumber || space.Type() == vm.TypeIntegerNumber {
				// Number space: create string of that many spaces (max 10)
				numSpaces := int(space.ToFloat())
				if numSpaces < 0 {
					numSpaces = 0
				}
				if numSpaces > 10 {
					numSpaces = 10
				}
				for i := 0; i < numSpaces; i++ {
					gap += " "
				}
			} else if space.Type() == vm.TypeString {
				// String space: use first 10 characters
				gap = space.ToString()
				if len(gap) > 10 {
					gap = gap[:10]
				}
			}
		}

		// Wrap the value in a wrapper object for initial call
		// Per spec: wrapper = ObjectCreate(%ObjectPrototype%) with CreateDataProperty(wrapper, "", value)
		wrapper := vm.NewObject(vmInstance.ObjectPrototype)
		wrapper.AsPlainObject().SetOwn("", value) // Enumerable, writable, configurable

		visited := make(map[uintptr]bool)
		result, err := stringifyValueToJSONWithVisited(ctx.VM, value, visited, gap, "", "", wrapper, replacerFunc, propertyList)
		if err != nil {
			return vm.Undefined, err
		}
		if result == "" {
			return vm.Undefined, nil
		}
		return vm.NewString(result), nil
	}))

	// Add rawJSON method (ES2024)
	// JSON.rawJSON(text) creates a frozen object with rawJSON property containing the text
	jsonObj.SetOwnNonEnumerable("rawJSON", vm.NewNativeFunction(1, false, "rawJSON", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, vmInstance.NewTypeError("JSON.rawJSON requires a string argument")
		}

		// Step 1: Let jsonString be ? ToString(text)
		// Note: Symbol cannot be converted to string, throws TypeError
		textArg := args[0]
		if textArg.Type() == vm.TypeSymbol {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert a Symbol value to a string")
		}
		if textArg.IsObject() || textArg.IsCallable() {
			textArg = vmInstance.ToPrimitive(textArg, "string")
		}
		jsonString := textArg.ToString()

		// Step 2: Throw SyntaxError if empty string, or if first/last code unit is whitespace
		if len(jsonString) == 0 {
			return vm.Undefined, vmInstance.NewSyntaxError("JSON.rawJSON: cannot be empty string")
		}
		firstChar := jsonString[0]
		lastChar := jsonString[len(jsonString)-1]
		// Check for illegal start/end whitespace characters: tab (0x09), LF (0x0A), CR (0x0D), space (0x20)
		if firstChar == '\t' || firstChar == '\n' || firstChar == '\r' || firstChar == ' ' ||
			lastChar == '\t' || lastChar == '\n' || lastChar == '\r' || lastChar == ' ' {
			return vm.Undefined, vmInstance.NewSyntaxError("JSON.rawJSON: JSON text may not start or end with whitespace")
		}

		// Step 3: Parse to validate it's valid JSON text for a primitive
		// We need to validate that it's a valid JSON primitive value
		dec := json.NewDecoder(strings.NewReader(jsonString))
		token, err := dec.Token()
		if err != nil {
			return vm.Undefined, vmInstance.NewSyntaxError("JSON.rawJSON: invalid JSON text")
		}
		// Check it's not an object or array start
		if delim, ok := token.(json.Delim); ok {
			if delim == '{' || delim == '[' {
				return vm.Undefined, vmInstance.NewSyntaxError("JSON.rawJSON text must be a JSON primitive value")
			}
		}
		// Check no extra content
		if dec.More() {
			return vm.Undefined, vmInstance.NewSyntaxError("JSON.rawJSON: unexpected content after JSON value")
		}

		// Step 4-5: Create object with null prototype
		// We use a special non-enumerable, non-configurable marker property that stringify can check
		rawObj := vm.NewObject(vm.Null).AsPlainObject()

		// Step 6: Create data property "rawJSON" with the string
		// Per spec: { [[Writable]]: false, [[Enumerable]]: true, [[Configurable]]: false }
		falseVal := false
		trueVal := true
		rawObj.DefineOwnProperty("rawJSON", vm.NewString(jsonString), &falseVal, &trueVal, &falseVal)

		// Step 7: Make object non-extensible (frozen)
		rawObj.SetExtensible(false)

		// Store a reference in our tracking map for isRawJSON checks
		rawJSONObjects[rawObj] = true

		return vm.NewValueFromPlainObject(rawObj), nil
	}))

	// Add isRawJSON method (ES2024)
	// JSON.isRawJSON(value) returns true if value has [[IsRawJSON]] internal slot
	jsonObj.SetOwnNonEnumerable("isRawJSON", vm.NewNativeFunction(1, false, "isRawJSON", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.BooleanValue(false), nil
		}

		value := args[0]
		// Check if it's an object tracked in our rawJSONObjects map
		if value.Type() == vm.TypeObject {
			obj := value.AsPlainObject()
			if obj != nil && rawJSONObjects[obj] {
				return vm.BooleanValue(true), nil
			}
		}
		return vm.BooleanValue(false), nil
	}))

	// Register JSON object as global
	return ctx.DefineGlobal("JSON", vm.NewValueFromPlainObject(jsonObj))
}

// parseJSONToValue converts a JSON string to a VM Value, preserving object key order
func parseJSONToValue(text string) (vm.Value, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber() // Use json.Number to preserve number precision
	val, err := parseJSONValueFromDecoder(dec, nil)
	if err != nil {
		return vm.Undefined, err
	}
	// Check for trailing content (JSON should have exactly one value)
	if dec.More() {
		return vm.Undefined, errors.New("unexpected token after JSON")
	}
	return val, nil
}

// parseJSONToValueWithPrototypes converts a JSON string to a VM Value with proper prototypes
func parseJSONToValueWithPrototypes(vmInstance *vm.VM, text string) (vm.Value, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber() // Use json.Number to preserve number precision
	val, err := parseJSONValueFromDecoder(dec, vmInstance)
	if err != nil {
		return vm.Undefined, err
	}
	// Check for trailing content (JSON should have exactly one value)
	if dec.More() {
		return vm.Undefined, errors.New("unexpected token after JSON")
	}
	return val, nil
}

// jsonSourceMap maps parsed values (by identity) to their source text
// For primitives, we use a path-based key since primitives don't have identity
type jsonSourceMap map[string]string

// parseJSONWithSource parses JSON and tracks source text for each value
// Returns the parsed value and a source map for primitive values
func parseJSONWithSource(vmInstance *vm.VM, text string) (vm.Value, jsonSourceMap, error) {
	sourceMap := make(jsonSourceMap)
	val, err := parseJSONWithSourceRecursive(vmInstance, text, "", sourceMap)
	if err != nil {
		return vm.Undefined, nil, err
	}
	return val, sourceMap, nil
}

// parseJSONWithSourceRecursive parses JSON while tracking source positions
func parseJSONWithSourceRecursive(vmInstance *vm.VM, text string, path string, sourceMap jsonSourceMap) (vm.Value, error) {
	text = strings.TrimLeft(text, " \t\r\n")
	if len(text) == 0 {
		return vm.Undefined, errors.New("unexpected end of JSON input")
	}

	switch text[0] {
	case '{':
		// Parse object
		var proto vm.Value
		if vmInstance != nil {
			proto = vmInstance.ObjectPrototype
		} else {
			proto = vm.Null
		}
		obj := vm.NewObject(proto).AsPlainObject()

		// Skip '{'
		text = strings.TrimLeft(text[1:], " \t\r\n")
		if len(text) == 0 {
			return vm.Undefined, errors.New("unexpected end of JSON input in object")
		}

		if text[0] == '}' {
			// Empty object - no source for objects
			return vm.NewValueFromPlainObject(obj), nil
		}

		for {
			// Parse key
			text = strings.TrimLeft(text, " \t\r\n")
			if len(text) == 0 || text[0] != '"' {
				return vm.Undefined, errors.New("expected string key in object")
			}
			key, keyEnd, err := parseJSONString(text)
			if err != nil {
				return vm.Undefined, err
			}
			text = strings.TrimLeft(text[keyEnd:], " \t\r\n")

			// Expect ':'
			if len(text) == 0 || text[0] != ':' {
				return vm.Undefined, errors.New("expected ':' after key in object")
			}
			text = strings.TrimLeft(text[1:], " \t\r\n")

			// Find the source for this value
			childPath := path + "/" + key

			// Parse value (recursive)
			val, remaining, source, err := parseJSONValueWithSource(vmInstance, text, childPath, sourceMap)
			if err != nil {
				return vm.Undefined, err
			}
			text = remaining

			// Store source for primitives
			if source != "" {
				sourceMap[childPath] = source
			}

			obj.SetOwn(key, val)

			text = strings.TrimLeft(text, " \t\r\n")
			if len(text) == 0 {
				return vm.Undefined, errors.New("unexpected end of JSON input in object")
			}

			if text[0] == '}' {
				text = text[1:]
				break
			}
			if text[0] == ',' {
				text = strings.TrimLeft(text[1:], " \t\r\n")
				continue
			}
			return vm.Undefined, errors.New("expected ',' or '}' in object")
		}

		return vm.NewValueFromPlainObject(obj), nil

	case '[':
		// Parse array
		var elements []vm.Value

		text = strings.TrimLeft(text[1:], " \t\r\n")
		if len(text) == 0 {
			return vm.Undefined, errors.New("unexpected end of JSON input in array")
		}

		if text[0] == ']' {
			// Empty array
			return vm.NewArray(), nil
		}

		idx := 0
		for {
			childPath := path + "/" + strconv.Itoa(idx)

			// Parse element
			val, remaining, source, err := parseJSONValueWithSource(vmInstance, text, childPath, sourceMap)
			if err != nil {
				return vm.Undefined, err
			}
			text = remaining

			// Store source for primitives
			if source != "" {
				sourceMap[childPath] = source
			}

			elements = append(elements, val)
			idx++

			text = strings.TrimLeft(text, " \t\r\n")
			if len(text) == 0 {
				return vm.Undefined, errors.New("unexpected end of JSON input in array")
			}

			if text[0] == ']' {
				text = text[1:]
				break
			}
			if text[0] == ',' {
				text = strings.TrimLeft(text[1:], " \t\r\n")
				continue
			}
			return vm.Undefined, errors.New("expected ',' or ']' in array")
		}

		// Create array with elements directly (don't use NewArrayWithArgs which has Array() constructor semantics)
		arr := vm.NewArray()
		arr.AsArray().SetElements(elements)
		return arr, nil

	default:
		// Parse primitive
		val, _, source, err := parseJSONValueWithSource(vmInstance, text, path, sourceMap)
		if err != nil {
			return vm.Undefined, err
		}
		if source != "" {
			sourceMap[path] = source
		}
		return val, nil
	}
}

// parseJSONValueWithSource parses a single JSON value and returns the value, remaining text, and source
func parseJSONValueWithSource(vmInstance *vm.VM, text string, path string, sourceMap jsonSourceMap) (vm.Value, string, string, error) {
	text = strings.TrimLeft(text, " \t\r\n")
	if len(text) == 0 {
		return vm.Undefined, "", "", errors.New("unexpected end of JSON input")
	}

	switch text[0] {
	case '{':
		// Objects don't have source - recursively parse
		val, err := parseJSONWithSourceRecursive(vmInstance, text, path, sourceMap)
		if err != nil {
			return vm.Undefined, "", "", err
		}
		// Find end of object
		remaining := skipJSONValue(text)
		return val, remaining, "", nil

	case '[':
		// Arrays don't have source - recursively parse
		val, err := parseJSONWithSourceRecursive(vmInstance, text, path, sourceMap)
		if err != nil {
			return vm.Undefined, "", "", err
		}
		// Find end of array
		remaining := skipJSONValue(text)
		return val, remaining, "", nil

	case '"':
		// String - capture source exactly
		str, end, err := parseJSONString(text)
		if err != nil {
			return vm.Undefined, "", "", err
		}
		source := text[:end]
		return vm.NewString(str), text[end:], source, nil

	case 't':
		// true
		if strings.HasPrefix(text, "true") {
			return vm.BooleanValue(true), text[4:], "true", nil
		}
		return vm.Undefined, "", "", errors.New("invalid JSON: expected 'true'")

	case 'f':
		// false
		if strings.HasPrefix(text, "false") {
			return vm.BooleanValue(false), text[5:], "false", nil
		}
		return vm.Undefined, "", "", errors.New("invalid JSON: expected 'false'")

	case 'n':
		// null
		if strings.HasPrefix(text, "null") {
			return vm.Null, text[4:], "null", nil
		}
		return vm.Undefined, "", "", errors.New("invalid JSON: expected 'null'")

	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		// Number - capture source exactly
		end := 0
		for end < len(text) {
			c := text[end]
			if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
				end++
			} else {
				break
			}
		}
		source := text[:end]
		f, err := strconv.ParseFloat(source, 64)
		if err != nil {
			return vm.Undefined, "", "", err
		}
		return vm.NumberValue(f), text[end:], source, nil

	default:
		return vm.Undefined, "", "", errors.New("invalid JSON value")
	}
}

// parseJSONString parses a JSON string and returns the string value and the end position (after closing quote)
func parseJSONString(text string) (string, int, error) {
	if len(text) < 2 || text[0] != '"' {
		return "", 0, errors.New("expected string")
	}

	var result strings.Builder
	i := 1 // Skip opening quote
	for i < len(text) {
		if text[i] == '"' {
			return result.String(), i + 1, nil
		}
		if text[i] == '\\' {
			if i+1 >= len(text) {
				return "", 0, errors.New("unexpected end of string")
			}
			i++
			switch text[i] {
			case '"', '\\', '/':
				result.WriteByte(text[i])
			case 'b':
				result.WriteByte('\b')
			case 'f':
				result.WriteByte('\f')
			case 'n':
				result.WriteByte('\n')
			case 'r':
				result.WriteByte('\r')
			case 't':
				result.WriteByte('\t')
			case 'u':
				if i+4 >= len(text) {
					return "", 0, errors.New("invalid unicode escape")
				}
				code, err := strconv.ParseUint(text[i+1:i+5], 16, 16)
				if err != nil {
					return "", 0, errors.New("invalid unicode escape")
				}
				result.WriteRune(rune(code))
				i += 4
			default:
				return "", 0, errors.New("invalid escape sequence")
			}
		} else {
			result.WriteByte(text[i])
		}
		i++
	}
	return "", 0, errors.New("unterminated string")
}

// skipJSONValue skips over a JSON value and returns the remaining text
func skipJSONValue(text string) string {
	text = strings.TrimLeft(text, " \t\r\n")
	if len(text) == 0 {
		return text
	}

	switch text[0] {
	case '{':
		depth := 1
		i := 1
		inString := false
		for i < len(text) && depth > 0 {
			if inString {
				if text[i] == '\\' {
					i++
				} else if text[i] == '"' {
					inString = false
				}
			} else {
				switch text[i] {
				case '"':
					inString = true
				case '{':
					depth++
				case '}':
					depth--
				}
			}
			i++
		}
		return text[i:]

	case '[':
		depth := 1
		i := 1
		inString := false
		for i < len(text) && depth > 0 {
			if inString {
				if text[i] == '\\' {
					i++
				} else if text[i] == '"' {
					inString = false
				}
			} else {
				switch text[i] {
				case '"':
					inString = true
				case '[':
					depth++
				case ']':
					depth--
				}
			}
			i++
		}
		return text[i:]

	case '"':
		_, end, _ := parseJSONString(text)
		return text[end:]

	case 't':
		return text[4:]
	case 'f':
		return text[5:]
	case 'n':
		return text[4:]

	default:
		// Number
		i := 0
		for i < len(text) {
			c := text[i]
			if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
				i++
			} else {
				break
			}
		}
		return text[i:]
	}
}

// internalizeJSONProperty implements the abstract operation InternalizeJSONProperty
// It recursively walks the parsed value and calls the reviver function for each property
// path is used to look up source text in the sourceMap for the json-parse-with-source feature
func internalizeJSONProperty(vmInstance *vm.VM, holder vm.Value, name string, reviver vm.Value, sourceMap jsonSourceMap, path string) (vm.Value, error) {
	// Get the value at holder[name]
	val, err := vmInstance.GetProperty(holder, name)
	if err != nil {
		return vm.Undefined, err
	}

	// If val is an object, recursively process its properties
	if val.Type() == vm.TypeObject || val.Type() == vm.TypeDictObject {
		obj := val.AsPlainObject()
		if obj != nil {
			keys := obj.OwnKeys()
			for _, key := range keys {
				childPath := path + "/" + key
				newElement, err := internalizeJSONProperty(vmInstance, val, key, reviver, sourceMap, childPath)
				if err != nil {
					return vm.Undefined, err
				}
				if newElement == vm.Undefined {
					// Delete the property if reviver returns undefined (silently fail if non-configurable)
					obj.DeleteOwn(key)
				} else {
					// CreateDataProperty semantics: silently fail if property is non-configurable
					exists, nonConfigurable := obj.IsOwnPropertyNonConfigurable(key)
					if !exists || !nonConfigurable {
						obj.SetOwn(key, newElement)
					}
					// If non-configurable, silently skip the update per spec
				}
			}
		}
	} else if val.Type() == vm.TypeArray {
		arr := val.AsArray()
		if arr != nil {
			for i := 0; i < arr.Length(); i++ {
				key := strconv.Itoa(i)
				childPath := path + "/" + key
				newElement, err := internalizeJSONProperty(vmInstance, val, key, reviver, sourceMap, childPath)
				if err != nil {
					return vm.Undefined, err
				}
				if newElement == vm.Undefined {
					// For arrays, set to undefined rather than deleting (keeps sparse array)
					arr.Set(i, vm.Undefined)
				} else {
					arr.Set(i, newElement)
				}
			}
		}
	}

	// Create context object for reviver (json-parse-with-source feature)
	// Per spec: context is a plain object with Object.prototype
	// For primitives, it has a "source" property with the original JSON text
	// For objects/arrays, it has no properties
	context := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	if sourceMap != nil {
		if source, ok := sourceMap[path]; ok {
			// Primitive value - add source property
			// Per spec: { [[Writable]]: true, [[Enumerable]]: true, [[Configurable]]: true }
			context.SetOwn("source", vm.NewString(source))
		}
		// Objects and arrays get empty context (no source property)
	}

	// Call the reviver function with (holder, name, val, context)
	return vmInstance.Call(reviver, holder, []vm.Value{vm.NewString(name), val, vm.NewValueFromPlainObject(context)})
}

// parseJSONValueFromDecoder reads a JSON value from a decoder, preserving object key order
// If vmInstance is provided, objects will use Object.prototype and arrays will use Array.prototype
func parseJSONValueFromDecoder(dec *json.Decoder, vmInstance *vm.VM) (vm.Value, error) {
	token, err := dec.Token()
	if err != nil {
		return vm.Undefined, err
	}

	switch t := token.(type) {
	case nil:
		return vm.Null, nil
	case bool:
		return vm.BooleanValue(t), nil
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(f), nil
	case string:
		return vm.NewString(t), nil
	case json.Delim:
		switch t {
		case '{':
			// Parse object, preserving key order
			// Use Object.prototype if vmInstance is provided, otherwise null
			var proto vm.Value
			if vmInstance != nil {
				proto = vmInstance.ObjectPrototype
			} else {
				proto = vm.Null
			}
			obj := vm.NewObject(proto).AsPlainObject()
			for dec.More() {
				// Read key
				keyToken, err := dec.Token()
				if err != nil {
					return vm.Undefined, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return vm.Undefined, errors.New("expected string key in object")
				}
				// Read value
				value, err := parseJSONValueFromDecoder(dec, vmInstance)
				if err != nil {
					return vm.Undefined, err
				}
				// JSON parsed properties should be enumerable, writable, and configurable
				obj.SetOwn(key, value)
			}
			// Consume closing '}'
			if _, err := dec.Token(); err != nil {
				return vm.Undefined, err
			}
			return vm.NewValueFromPlainObject(obj), nil
		case '[':
			// Parse array
			var elements []vm.Value
			for dec.More() {
				elem, err := parseJSONValueFromDecoder(dec, vmInstance)
				if err != nil {
					return vm.Undefined, err
				}
				elements = append(elements, elem)
			}
			// Consume closing ']'
			if _, err := dec.Token(); err != nil {
				return vm.Undefined, err
			}
			return vm.NewArrayWithArgs(elements), nil
		}
	}

	return vm.Undefined, errors.New("unexpected JSON token")
}

// convertJSONValue converts a Go interface{} from json.Unmarshal to a VM Value
func convertJSONValue(value interface{}) vm.Value {
	switch v := value.(type) {
	case nil:
		return vm.Null
	case bool:
		return vm.BooleanValue(v)
	case float64:
		return vm.NumberValue(v)
	case string:
		return vm.NewString(v)
	case []interface{}:
		// Convert array
		elements := make([]vm.Value, len(v))
		for i, elem := range v {
			elements[i] = convertJSONValue(elem)
		}
		return vm.NewArrayWithArgs(elements)
	case map[string]interface{}:
		// Convert object
		obj := vm.NewObject(vm.Null).AsPlainObject()
		for key, val := range v {
			obj.SetOwnNonEnumerable(key, convertJSONValue(val))
		}
		return vm.NewValueFromPlainObject(obj)
	default:
		return vm.Undefined
	}
}

// sortJSONKeys sorts keys per ECMAScript spec: array indices first (sorted numerically), then strings (original order)
func sortJSONKeys(keys []string) []string {
	var numericKeys []string
	var stringKeys []string

	for _, key := range keys {
		// Check if key is a valid array index (non-negative integer)
		if num, err := strconv.ParseUint(key, 10, 32); err == nil && strconv.FormatUint(num, 10) == key {
			numericKeys = append(numericKeys, key)
		} else {
			stringKeys = append(stringKeys, key)
		}
	}

	// Sort numeric keys numerically
	sort.Slice(numericKeys, func(i, j int) bool {
		a, _ := strconv.ParseUint(numericKeys[i], 10, 32)
		b, _ := strconv.ParseUint(numericKeys[j], 10, 32)
		return a < b
	})

	// Combine: numeric keys first, then string keys (preserve original order)
	return append(numericKeys, stringKeys...)
}

// isProxyForArray recursively checks if a proxy's target is an array
// Per ECMAScript spec, IsArray() on a Proxy must check the target recursively
func isProxyForArray(proxy *vm.ProxyObject) bool {
	if proxy == nil || proxy.Revoked {
		return false
	}
	target := proxy.Target()
	if target.Type() == vm.TypeArray {
		return true
	}
	// Recursively check if target is also a proxy for an array
	if target.Type() == vm.TypeProxy {
		return isProxyForArray(target.AsProxy())
	}
	return false
}

// getProxyOwnKeys gets the own enumerable string keys from a proxy, handling proxy chains
// Per ECMAScript, ownKeys trap is called on the proxy's handler, but if no trap exists,
// we fall back to the target's [[OwnPropertyKeys]] which may itself be a proxy
func getProxyOwnKeys(vmInstance *vm.VM, proxy *vm.ProxyObject) ([]string, error) {
	if proxy == nil || proxy.Revoked {
		return nil, nil
	}

	handler := proxy.Handler()
	if handler.Type() == vm.TypeObject {
		handlerObj := handler.AsPlainObject()
		ownKeysTrap, hasOwnKeysTrap := handlerObj.GetOwn("ownKeys")
		if hasOwnKeysTrap && ownKeysTrap.IsCallable() {
			// Call ownKeys trap: handler.ownKeys(target)
			keysResult, err := vmInstance.Call(ownKeysTrap, handler, []vm.Value{proxy.Target()})
			if err != nil {
				return nil, err
			}
			// Extract keys from result array
			var keys []string
			if keysResult.Type() == vm.TypeArray {
				arr := keysResult.AsArray()
				for i := 0; i < arr.Length(); i++ {
					keyVal := arr.Get(i)
					if keyVal.Type() == vm.TypeString {
						keys = append(keys, keyVal.ToString())
					}
				}
			}
			return keys, nil
		}
	}

	// No ownKeys trap - recurse into target
	target := proxy.Target()
	switch target.Type() {
	case vm.TypeProxy:
		// Target is another proxy - recurse
		return getProxyOwnKeys(vmInstance, target.AsProxy())
	case vm.TypeObject:
		return target.AsPlainObject().OwnKeys(), nil
	case vm.TypeDictObject:
		return target.AsDictObject().OwnKeys(), nil
	case vm.TypeArray:
		// Array - return numeric indices
		arr := target.AsArray()
		var keys []string
		for i := 0; i < arr.Length(); i++ {
			keys = append(keys, strconv.Itoa(i))
		}
		return keys, nil
	default:
		return nil, nil
	}
}

// escapeJSONString escapes a string for JSON output, preserving lone surrogates
// Go's json.Marshal replaces invalid UTF-8 (including lone surrogates) with replacement chars,
// but ECMAScript requires preserving surrogates as \uXXXX escapes
func escapeJSONString(s string) string {
	var buf strings.Builder
	buf.WriteByte('"')

	for i := 0; i < len(s); {
		b := s[i]
		// Check for characters that need escaping
		switch b {
		case '"':
			buf.WriteString("\\\"")
			i++
		case '\\':
			buf.WriteString("\\\\")
			i++
		case '\b':
			buf.WriteString("\\b")
			i++
		case '\f':
			buf.WriteString("\\f")
			i++
		case '\n':
			buf.WriteString("\\n")
			i++
		case '\r':
			buf.WriteString("\\r")
			i++
		case '\t':
			buf.WriteString("\\t")
			i++
		default:
			if b < 0x20 {
				// Control characters need \uXXXX escaping
				buf.WriteString(fmt.Sprintf("\\u%04x", b))
				i++
			} else if b < 0x80 {
				// Regular ASCII
				buf.WriteByte(b)
				i++
			} else {
				// UTF-8 sequence - decode it
				// Note: Surrogates (U+D800-U+DFFF) are encoded as UTF-8 in JS strings
				// even though they're technically invalid UTF-8. We need to detect them
				// and handle them specially.
				r, size := utf8.DecodeRuneInString(s[i:])
				if r == utf8.RuneError && size == 1 {
					// Invalid UTF-8 byte - check if it's part of a surrogate sequence
					// UTF-8 encoding of surrogates (U+D800-U+DFFF) uses bytes ED A0-BF 80-BF
					if b == 0xED && i+2 < len(s) {
						b2 := s[i+1]
						b3 := s[i+2]
						// Check if this forms a surrogate code point (U+D800-U+DFFF)
						if b2 >= 0xA0 && b2 <= 0xBF && b3 >= 0x80 && b3 <= 0xBF {
							// Decode the surrogate value
							highSurr := rune(b&0x0F)<<12 | rune(b2&0x3F)<<6 | rune(b3&0x3F)

							// Check if this is a high surrogate (D800-DBFF) followed by a low surrogate
							if highSurr >= 0xD800 && highSurr <= 0xDBFF && i+5 < len(s) {
								// Check for following low surrogate
								if s[i+3] == 0xED {
									b5 := s[i+4]
									b6 := s[i+5]
									if b5 >= 0xB0 && b5 <= 0xBF && b6 >= 0x80 && b6 <= 0xBF {
										// Low surrogate found - decode it
										lowSurr := rune(s[i+3]&0x0F)<<12 | rune(b5&0x3F)<<6 | rune(b6&0x3F)
										if lowSurr >= 0xDC00 && lowSurr <= 0xDFFF {
											// Valid surrogate pair - combine into single character
											codepoint := 0x10000 + ((highSurr - 0xD800) << 10) + (lowSurr - 0xDC00)
											buf.WriteRune(codepoint)
											i += 6
											continue
										}
									}
								}
							}

							// Lone surrogate - escape as \uXXXX
							buf.WriteString(fmt.Sprintf("\\u%04x", highSurr))
							i += 3
							continue
						}
					}
					// Not a surrogate - escape as \u00XX
					buf.WriteString(fmt.Sprintf("\\u%04x", b))
					i++
				} else if r >= 0xD800 && r <= 0xDFFF {
					// Surrogate code point decoded successfully (shouldn't happen, but handle it)
					buf.WriteString(fmt.Sprintf("\\u%04x", r))
					i += size
				} else {
					// Valid Unicode - write as-is (it's valid UTF-8)
					buf.WriteString(s[i : i+size])
					i += size
				}
			}
		}
	}

	buf.WriteByte('"')
	return buf.String()
}

// stringifyProxyArray serializes a proxy that wraps an array using length and numeric indices
func stringifyProxyArray(vmInstance *vm.VM, value vm.Value, visited map[uintptr]bool, gap string, indent string, key string, holder vm.Value, replacerFunc vm.Value, propertyList []string) (string, error) {
	proxy := value.AsProxy()
	if proxy == nil {
		return "null", nil
	}

	// Check for circular reference
	ptr := uintptr(unsafe.Pointer(proxy))
	if visited[ptr] {
		if vmInstance != nil {
			return "", vmInstance.NewTypeError("Converting circular structure to JSON")
		}
		return "", errors.New("TypeError: Converting circular structure to JSON")
	}
	visited[ptr] = true
	defer delete(visited, ptr)

	// Get length via get trap
	var length int
	if vmInstance != nil {
		lengthVal, err := vmInstance.GetProperty(value, "length")
		if err != nil {
			return "", err
		}
		length = int(lengthVal.ToFloat())
	} else {
		length = 0
	}

	if length == 0 {
		return "[]", nil
	}

	// Pretty printing with gap
	if gap != "" {
		stepIndent := indent + gap
		result := "["
		for i := 0; i < length; i++ {
			result += "\n" + stepIndent
			// Get element via get trap
			var elem vm.Value
			if vmInstance != nil {
				var err error
				elem, err = vmInstance.GetProperty(value, strconv.Itoa(i))
				if err != nil {
					return "", err
				}
			} else {
				elem = vm.Undefined
			}
			elemKey := strconv.Itoa(i)
			elemJSON, err := stringifyValueToJSONWithVisited(vmInstance, elem, visited, gap, stepIndent, elemKey, value, replacerFunc, propertyList)
			if err != nil {
				return "", err
			}
			// In arrays, undefined/functions/symbols become "null"
			if elemJSON == "" {
				elemJSON = "null"
			}
			result += elemJSON
			if i < length-1 {
				result += ","
			}
		}
		result += "\n" + indent + "]"
		return result, nil
	}

	// Compact formatting (no gap)
	result := "["
	for i := 0; i < length; i++ {
		if i > 0 {
			result += ","
		}
		// Get element via get trap
		var elem vm.Value
		if vmInstance != nil {
			var err error
			elem, err = vmInstance.GetProperty(value, strconv.Itoa(i))
			if err != nil {
				return "", err
			}
		} else {
			elem = vm.Undefined
		}
		elemKey := strconv.Itoa(i)
		elemJSON, err := stringifyValueToJSONWithVisited(vmInstance, elem, visited, gap, indent, elemKey, value, replacerFunc, propertyList)
		if err != nil {
			return "", err
		}
		// In arrays, undefined/functions/symbols become "null"
		if elemJSON == "" {
			elemJSON = "null"
		}
		result += elemJSON
	}
	result += "]"
	return result, nil
}

// stringifyValueToJSON converts a VM Value to a JSON string (legacy, no circular check)
func stringifyValueToJSON(value vm.Value) string {
	visited := make(map[uintptr]bool)
	wrapper := vm.NewObject(vm.Null)
	result, _ := stringifyValueToJSONWithVisited(nil, value, visited, "", "", "", wrapper, vm.Undefined, nil)
	return result
}

// stringifyValueToJSONWithVisited converts a VM Value to a JSON string with circular reference detection
// gap is the indentation string (e.g., "  " for 2 spaces), indent is the current indentation level
// key is the property key (for toJSON method calls)
// holder is the parent object containing this value
// replacerFunc is the replacer function (if any)
// propertyList is the property whitelist (if replacer is an array)
func stringifyValueToJSONWithVisited(vmInstance *vm.VM, value vm.Value, visited map[uintptr]bool, gap string, indent string, key string, holder vm.Value, replacerFunc vm.Value, propertyList []string) (string, error) {
	// Step 0: Handle rawJSON objects (ES2024) - return rawJSON property directly
	if value.Type() == vm.TypeObject {
		obj := value.AsPlainObject()
		if obj != nil && rawJSONObjects[obj] {
			if rawJSON, ok := obj.GetOwn("rawJSON"); ok {
				return rawJSON.ToString(), nil
			}
		}
	}

	// Step 1: Handle toJSON method if present (objects, arrays, proxies, and BigInt)
	// Per ECMAScript spec: If Type(value) is Object or BigInt, check for toJSON
	if value.Type() == vm.TypeObject || value.Type() == vm.TypeDictObject || value.Type() == vm.TypeArray || value.Type() == vm.TypeProxy || value.Type() == vm.TypeBigInt {
		var toJSON vm.Value
		var err error

		// Use GetProperty to properly invoke getters (which may throw)
		if vmInstance != nil {
			toJSON, err = vmInstance.GetProperty(value, "toJSON")
			if err != nil {
				return "", err
			}
		} else {
			// Fallback if no VM instance
			var ok bool
			if value.Type() == vm.TypeObject {
				toJSON, ok = value.AsPlainObject().GetOwn("toJSON")
			} else if value.Type() == vm.TypeDictObject {
				toJSON, ok = value.AsDictObject().GetOwn("toJSON")
			} else if value.Type() == vm.TypeArray {
				toJSON, ok = value.AsArray().GetOwn("toJSON")
			}
			if !ok {
				toJSON = vm.Undefined
			}
		}

		if toJSON != vm.Undefined && toJSON.IsCallable() {
			// Call toJSON method with key as argument
			if vmInstance != nil {
				result, err := vmInstance.Call(toJSON, value, []vm.Value{vm.NewString(key)})
				if err != nil {
					return "", err
				}
				value = result
			}
		}
	}

	// Step 2: Apply replacer function if present
	if replacerFunc != vm.Undefined && replacerFunc.IsCallable() && vmInstance != nil {
		result, err := vmInstance.Call(replacerFunc, holder, []vm.Value{vm.NewString(key), value})
		if err != nil {
			return "", err
		}
		value = result

		// Check if replacer returned a rawJSON object (ES2024)
		if value.Type() == vm.TypeObject {
			obj := value.AsPlainObject()
			if obj != nil && rawJSONObjects[obj] {
				if rawJSON, ok := obj.GetOwn("rawJSON"); ok {
					return rawJSON.ToString(), nil
				}
			}
		}
	}

	// Step 3: Handle boxed primitives (Boolean, Number, String, BigInt objects)
	// Per spec: ToNumber for Number objects, ToString for String objects, value for Boolean objects
	// BigInt objects throw TypeError
	if value.Type() == vm.TypeObject && vmInstance != nil {
		obj := value.AsPlainObject()
		if pv, ok := obj.GetOwn("[[PrimitiveValue]]"); ok {
			// This is a boxed primitive - convert using ToPrimitive
			switch pv.Type() {
			case vm.TypeFloatNumber, vm.TypeIntegerNumber:
				// Number object - call ToNumber via ToPrimitive with number hint
				value = vmInstance.ToPrimitive(value, "number")
			case vm.TypeString:
				// String object - call ToString via ToPrimitive with string hint
				value = vmInstance.ToPrimitive(value, "string")
			case vm.TypeBoolean:
				// Boolean object - just use the primitive value
				value = pv
			case vm.TypeBigInt:
				// BigInt objects cannot be serialized - throw TypeError
				return "", vmInstance.NewTypeError("Do not know how to serialize a BigInt")
			default:
				value = pv
			}
		}
	}

	switch value.Type() {
	case vm.TypeNull:
		return "null", nil
	case vm.TypeUndefined:
		return "", nil // JSON.stringify(undefined) returns undefined (empty string here)
	case vm.TypeSymbol:
		return "", nil // Symbols are not serializable
	case vm.TypeFunction, vm.TypeClosure, vm.TypeNativeFunction, vm.TypeNativeFunctionWithProps, vm.TypeBoundFunction, vm.TypeAsyncNativeFunction:
		return "", nil // Functions are not serializable
	case vm.TypeBigInt:
		// BigInt cannot be serialized in JSON - must throw TypeError
		if vmInstance != nil {
			return "", vmInstance.NewTypeError("Do not know how to serialize a BigInt")
		}
		return "", errors.New("TypeError: Do not know how to serialize a BigInt")
	case vm.TypeBoolean:
		if value.IsTruthy() {
			return "true", nil
		}
		return "false", nil
	case vm.TypeFloatNumber, vm.TypeIntegerNumber:
		num := value.ToFloat()
		// Handle special cases
		if math.IsNaN(num) { // NaN
			return "null", nil
		}
		if math.IsInf(num, 0) { // Infinity
			return "null", nil
		}
		// Handle negative zero - should serialize as "0" not "-0"
		if num == 0 {
			return "0", nil
		}
		return strconv.FormatFloat(num, 'f', -1, 64), nil
	case vm.TypeString:
		// Use custom escaping to preserve lone surrogates
		return escapeJSONString(value.ToString()), nil
	case vm.TypeArray:
		arr := value.AsArray()
		if arr.Length() == 0 {
			return "[]", nil
		}

		// Check for circular reference
		ptr := uintptr(unsafe.Pointer(arr))
		if visited[ptr] {
			// Throw TypeError for circular reference
			if vmInstance != nil {
				return "", vmInstance.NewTypeError("Converting circular structure to JSON")
			}
			return "", errors.New("TypeError: Converting circular structure to JSON")
		}

		// Mark as visited
		visited[ptr] = true
		defer delete(visited, ptr) // Remove after processing to allow same object in different branches

		// Pretty printing with gap
		if gap != "" {
			stepIndent := indent + gap
			result := "["
			for i := 0; i < arr.Length(); i++ {
				result += "\n" + stepIndent
				elem := arr.Get(i)
				elemKey := strconv.Itoa(i)
				elemJSON, err := stringifyValueToJSONWithVisited(vmInstance, elem, visited, gap, stepIndent, elemKey, value, replacerFunc, propertyList)
				if err != nil {
					return "", err
				}
				// In arrays, undefined/functions/symbols become "null"
				if elemJSON == "" {
					elemJSON = "null"
				}
				result += elemJSON
				if i < arr.Length()-1 {
					result += ","
				}
			}
			result += "\n" + indent + "]"
			return result, nil
		}

		// Compact formatting (no gap)
		result := "["
		for i := 0; i < arr.Length(); i++ {
			if i > 0 {
				result += ","
			}
			elem := arr.Get(i)
			elemKey := strconv.Itoa(i)
			elemJSON, err := stringifyValueToJSONWithVisited(vmInstance, elem, visited, gap, indent, elemKey, value, replacerFunc, propertyList)
			if err != nil {
				return "", err
			}
			// In arrays, undefined/functions/symbols become "null"
			if elemJSON == "" {
				elemJSON = "null"
			}
			result += elemJSON
		}
		result += "]"
		return result, nil
	case vm.TypeRegExp:
		// RegExp objects serialize as empty objects {}
		return "{}", nil
	case vm.TypeProxy:
		// Check if proxy is for an array (IsArray check)
		proxy := value.AsProxy()
		if proxy == nil {
			return "null", nil
		}
		if proxy.Revoked {
			if vmInstance != nil {
				return "", vmInstance.NewTypeError("Cannot perform 'ownKeys' on a proxy that has been revoked")
			}
			return "", errors.New("TypeError: Cannot perform 'ownKeys' on a proxy that has been revoked")
		}
		// Check if target is an array (recursively for proxy chains)
		if isProxyForArray(proxy) {
			// Serialize as an array using length and numeric indices
			return stringifyProxyArray(vmInstance, value, visited, gap, indent, key, holder, replacerFunc, propertyList)
		}
		// Fall through to object handling
		fallthrough
	case vm.TypeObject, vm.TypeDictObject:
		// Get object pointer for circular reference check
		var ptr uintptr
		if value.Type() == vm.TypeObject {
			ptr = uintptr(unsafe.Pointer(value.AsPlainObject()))
		} else if value.Type() == vm.TypeDictObject {
			ptr = uintptr(unsafe.Pointer(value.AsDictObject()))
		} else if value.Type() == vm.TypeProxy {
			proxy := value.AsProxy()
			ptr = uintptr(unsafe.Pointer(proxy))
			// Already checked for revoked above
			if proxy != nil && proxy.Revoked {
				if vmInstance != nil {
					return "", vmInstance.NewTypeError("Cannot perform 'ownKeys' on a proxy that has been revoked")
				}
				return "", errors.New("TypeError: Cannot perform 'ownKeys' on a proxy that has been revoked")
			}
		}

		// Check for circular reference
		if visited[ptr] {
			// Throw TypeError for circular reference
			if vmInstance != nil {
				return "", vmInstance.NewTypeError("Converting circular structure to JSON")
			}
			return "", errors.New("TypeError: Converting circular structure to JSON")
		}

		// Mark as visited
		visited[ptr] = true
		defer delete(visited, ptr) // Remove after processing to allow same object in different branches

		// Get keys for the object
		var keys []string
		if propertyList != nil {
			// If replacer array provided, use its order and only include those keys
			keys = propertyList
		} else {
			// Get keys based on object type
			if value.Type() == vm.TypeObject {
				keys = sortJSONKeys(value.AsPlainObject().OwnKeys())
			} else if value.Type() == vm.TypeDictObject {
				keys = sortJSONKeys(value.AsDictObject().OwnKeys())
			} else if value.Type() == vm.TypeProxy && vmInstance != nil {
				// For proxies, recursively get keys handling proxy chains
				proxy := value.AsProxy()
				if proxy != nil && !proxy.Revoked {
					proxyKeys, err := getProxyOwnKeys(vmInstance, proxy)
					if err != nil {
						return "", err
					}
					keys = sortJSONKeys(proxyKeys)
				}
			}
		}

		// Pretty printing with gap
		if gap != "" {
			stepIndent := indent + gap
			result := "{"
			first := true
			for _, key := range keys {
				// Use vm.GetProperty to properly invoke getters
				// Per spec, we must call the replacer for all keys in K, even if the property
				// has been deleted (returns undefined). The replacer may transform undefined to a value.
				var prop vm.Value
				if vmInstance != nil {
					var err error
					prop, err = vmInstance.GetProperty(value, key)
					if err != nil {
						return "", err
					}
				} else {
					// Fallback if no VM instance - get property directly from object
					if value.Type() == vm.TypeObject {
						prop, _ = value.AsPlainObject().GetOwn(key)
					} else if value.Type() == vm.TypeDictObject {
						prop, _ = value.AsDictObject().GetOwn(key)
					}
				}

				propJSON, err := stringifyValueToJSONWithVisited(vmInstance, prop, visited, gap, stepIndent, key, value, replacerFunc, propertyList)
				if err != nil {
					return "", err
				}
				if propJSON != "" { // Skip undefined properties (after replacer is called)
					if !first {
						result += ","
					}
					first = false
					result += "\n" + stepIndent
					keyBytes, _ := json.Marshal(key)
					result += string(keyBytes) + ": " + propJSON
				}
			}
			if !first {
				result += "\n" + indent
			}
			result += "}"
			return result, nil
		}

		// Compact formatting (no gap)
		result := "{"
		first := true
		for _, key := range keys {
			// Use vm.GetProperty to properly invoke getters
			// Per spec, we must call the replacer for all keys in K, even if the property
			// has been deleted (returns undefined). The replacer may transform undefined to a value.
			var prop vm.Value
			if vmInstance != nil {
				var err error
				prop, err = vmInstance.GetProperty(value, key)
				if err != nil {
					return "", err
				}
			} else {
				// Fallback if no VM instance - get property directly from object
				if value.Type() == vm.TypeObject {
					prop, _ = value.AsPlainObject().GetOwn(key)
				} else if value.Type() == vm.TypeDictObject {
					prop, _ = value.AsDictObject().GetOwn(key)
				}
			}

			propJSON, err := stringifyValueToJSONWithVisited(vmInstance, prop, visited, gap, indent, key, value, replacerFunc, propertyList)
			if err != nil {
				return "", err
			}
			if propJSON != "" { // Skip undefined properties (after replacer is called)
				if !first {
					result += ","
				}
				first = false
				keyBytes, _ := json.Marshal(key)
				result += string(keyBytes) + ":" + propJSON
			}
		}
		result += "}"
		return result, nil
	default:
		return "null", nil
	}
}

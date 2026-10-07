package builtins

import (
	"encoding/json"
	"errors"
	"github.com/nooga/paserati/pkg/wtf8"
	"math"
	"sort"
	"strconv"
	"strings"
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
		WithProperty("parse", types.NewOptionalFunction([]types.Type{types.String, types.Any}, types.Any, []bool{false, true})).
		WithProperty("stringify", types.NewOptionalFunction(
			[]types.Type{types.Any, types.Any, types.Any}, // value, replacer, space
			types.String,
			[]bool{false, true, true}, // value is required, replacer and space are optional
		)).
		WithProperty("rawJSON", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("isRawJSON", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean))

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

		return jsonParseText(vmInstance, text)
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
						if elem.Type() == vm.TypeObject {
							obj := elem.AsPlainObject()
							if _, ok := obj.GetInternal("[[PrimitiveValue]]"); ok {
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
				if pv, ok := obj.GetInternal("[[PrimitiveValue]]"); ok {
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
		var out []byte
		ok, err := jsonAppendValue(ctx.VM, value, visited, gap, "", "", wrapper, replacerFunc, propertyList, &out)
		if err != nil {
			return vm.Undefined, err
		}
		if !ok {
			return vm.Undefined, nil
		}
		return vm.NewString(string(out)), nil
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

// jsonParseText is JSON.parse(text) with no reviver: source key order,
// Object.prototype on every object, a "__proto__" key kept as an own data
// property, lone surrogate escapes preserved, and a parse failure returned as
// a thrown SyntaxError. Anything that turns JSON text into JS values
// (Response.json(), Request.json()) goes through this rather than its own
// decoder, so it can't drift from JSON.parse (paserati#522).
func jsonParseText(vmInstance *vm.VM, text string) (vm.Value, error) {
	// The standard parser is faster, but Go's decoder turns a lone surrogate
	// escape into U+FFFD, so text that may hold one goes through the
	// WTF-8-preserving manual parser instead.
	var val vm.Value
	var err error
	if jsonMayContainSurrogateEscape(text) || wtf8.HasSurrogate(text) {
		val, _, err = parseJSONWithSource(vmInstance, text)
	} else {
		val, err = parseJSONToValueWithPrototypes(vmInstance, text)
	}
	if err != nil {
		if cerr := vmInstance.CheckCancelled(); cerr != nil {
			return vm.Undefined, cerr // not a parse error
		}
		// Wrap parse error as SyntaxError exception
		if ctor, _ := vmInstance.GetGlobal("SyntaxError"); ctor != vm.Undefined {
			errObj, _ := vmInstance.Call(ctor, vm.Undefined, []vm.Value{vm.NewString(err.Error())})
			return vm.Undefined, vmInstance.NewExceptionError(errObj)
		}
		return vm.Undefined, err
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
	if vmInstance != nil {
		if err := vmInstance.CheckCancelled(); err != nil {
			return vm.Undefined, err
		}
	}
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
			return wtf8.JoinSurrogatePairs(result.String()), i + 1, nil
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
				// \uXXXX names a UTF-16 code unit: surrogates must survive as WTF-8
				// (WriteRune would emit U+FFFD) and pairs are joined on return.
				wtf8.WriteCodeUnit(&result, uint16(code))
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
	return vmInstance.CallArgs3(reviver, holder, vm.NewString(name), val, vm.NewValueFromPlainObject(context))
}

// parseJSONValueFromDecoder reads a JSON value from a decoder, preserving object key order
// If vmInstance is provided, objects will use Object.prototype and arrays will use Array.prototype
func parseJSONValueFromDecoder(dec *json.Decoder, vmInstance *vm.VM) (vm.Value, error) {
	if vmInstance != nil {
		if err := vmInstance.CheckCancelled(); err != nil {
			return vm.Undefined, err
		}
	}
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
	// OwnKeys already lists index-like keys first, in order; the only keys this
	// has to move are canonical array indices, which all start with a digit.
	// Most objects have none, so skip the partition (and its allocations).
	hasDigitKey := false
	for _, k := range keys {
		if len(k) > 0 && k[0] >= '0' && k[0] <= '9' {
			hasDigitKey = true
			break
		}
	}
	if !hasDigitKey {
		return keys
	}
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
	// GetMethod(handler, "ownKeys") per spec: an inherited trap counts, not
	// just an own one, and a TypeDictObject handler (a TS enum or module
	// namespace value at runtime) must still be checked for the trap
	// instead of being treated as trap-less outright -
	// vmInstance.ProxyGetTrap, not the previous
	// `if handler.Type() == vm.TypeObject { ...GetOwn... }` which silently
	// answered "no trap" for any other handler kind.
	ownKeysTrap, hasOwnKeysTrap := vmInstance.ProxyGetTrap(handler, "ownKeys")
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

// jsonMayContainSurrogateEscape reports whether text contains a \uXXXX escape
// that could name a UTF-16 surrogate (\uD800..\uDFFF, any hex case). It is a
// cheap over-approximation (퀀..퟿ also match) used, together with a
// check for raw WTF-8 surrogate bytes in the text, only to pick the parser
// that preserves lone surrogates (encoding/json would emit U+FFFD).
func jsonMayContainSurrogateEscape(text string) bool {
	for i := strings.Index(text, "\\u"); i >= 0; {
		if i+2 < len(text) && (text[i+2] == 'd' || text[i+2] == 'D') {
			return true
		}
		j := strings.Index(text[i+2:], "\\u")
		if j < 0 {
			return false
		}
		i += 2 + j
	}
	return false
}

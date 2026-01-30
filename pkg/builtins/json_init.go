package builtins

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type JSONInitializer struct{}

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
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		falseVal := false
		jsonObj.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("JSON"),
			&falseVal, // writable: false
			&falseVal, // enumerable: false
			&falseVal, // configurable: false (per ECMAScript spec 25.5)
		)
	}

	// Add parse method
	jsonObj.SetOwnNonEnumerable("parse", vm.NewNativeFunction(1, false, "parse", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			// Throw SyntaxError for missing argument
			return vm.Undefined, ctx.VM.NewSyntaxError("Unexpected end of JSON input")
		}

		text := args[0].ToString()
		val, err := parseJSONToValue(text)
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
	jsonObj.SetOwnNonEnumerable("stringify", vm.NewNativeFunction(1, true, "stringify", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, nil
		}

		value := args[0]

		// Process replacer parameter (args[1])
		var replacerFunc vm.Value
		var propertyList []string // nil means no filtering, empty slice means filter all
		if len(args) >= 2 && args[1] != vm.Undefined && args[1] != vm.Null {
			replacer := args[1]

			if replacer.Type() == vm.TypeFunction {
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

					// Handle String/Number objects
					if elem.Type() == vm.TypeObject {
						if pv, ok := elem.AsPlainObject().GetOwn("[[PrimitiveValue]]"); ok {
							elem = pv
						}
					}

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

			// Handle Number/String objects: ToNumber/ToString per spec
			// Check if it's a Number object (has [[NumberData]] internal slot)
			if space.Type() == vm.TypeObject {
				obj := space.AsPlainObject()
				// Check if it has [[PrimitiveValue]] property (our representation of boxed primitives)
				if pv, ok := obj.GetOwn("[[PrimitiveValue]]"); ok {
					space = pv
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
		wrapper := vm.NewObject(vm.Null)
		wrapper.AsPlainObject().SetOwnNonEnumerable("", value)

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

	// Register JSON object as global
	return ctx.DefineGlobal("JSON", vm.NewValueFromPlainObject(jsonObj))
}

// parseJSONToValue converts a JSON string to a VM Value, preserving object key order
func parseJSONToValue(text string) (vm.Value, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber() // Use json.Number to preserve number precision
	val, err := parseJSONValueFromDecoder(dec)
	if err != nil {
		return vm.Undefined, err
	}
	// Check for trailing content (JSON should have exactly one value)
	if dec.More() {
		return vm.Undefined, errors.New("unexpected token after JSON")
	}
	return val, nil
}

// parseJSONValueFromDecoder reads a JSON value from a decoder, preserving object key order
func parseJSONValueFromDecoder(dec *json.Decoder) (vm.Value, error) {
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
			obj := vm.NewObject(vm.Null).AsPlainObject()
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
				value, err := parseJSONValueFromDecoder(dec)
				if err != nil {
					return vm.Undefined, err
				}
				obj.SetOwnNonEnumerable(key, value)
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
				elem, err := parseJSONValueFromDecoder(dec)
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
	// Step 1: Handle toJSON method if present (only for objects, not arrays for now)
	if value.Type() == vm.TypeObject || value.Type() == vm.TypeDictObject {
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
			}
			if !ok {
				toJSON = vm.Undefined
			}
		}

		if toJSON != vm.Undefined && toJSON.Type() == vm.TypeFunction {
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
	if replacerFunc != vm.Undefined && replacerFunc.Type() == vm.TypeFunction && vmInstance != nil {
		result, err := vmInstance.Call(replacerFunc, holder, []vm.Value{vm.NewString(key), value})
		if err != nil {
			return "", err
		}
		value = result
	}

	// Step 3: Handle boxed primitives (Boolean, Number, String objects) - extract [[PrimitiveValue]]
	if value.Type() == vm.TypeObject {
		obj := value.AsPlainObject()
		if pv, ok := obj.GetOwn("[[PrimitiveValue]]"); ok {
			// This is a boxed primitive - use the primitive value instead
			value = pv
		}
	}

	switch value.Type() {
	case vm.TypeNull:
		return "null", nil
	case vm.TypeUndefined:
		return "", nil // JSON.stringify(undefined) returns undefined (empty string here)
	case vm.TypeFunction, vm.TypeSymbol:
		return "", nil // Functions and Symbols are not serializable
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
		bytes, _ := json.Marshal(value.ToString()) // Proper JSON string escaping
		return string(bytes), nil
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
	case vm.TypeObject, vm.TypeDictObject:
		// Get object pointer for circular reference check
		var ptr uintptr
		if value.Type() == vm.TypeObject {
			ptr = uintptr(unsafe.Pointer(value.AsPlainObject()))
		} else {
			ptr = uintptr(unsafe.Pointer(value.AsDictObject()))
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

		// Get object interface
		var obj interface {
			OwnKeys() []string
			GetOwn(string) (vm.Value, bool)
		}

		if value.Type() == vm.TypeObject {
			obj = value.AsPlainObject()
		} else {
			obj = value.AsDictObject()
		}

		// Determine key order
		var keys []string
		if propertyList != nil {
			// If replacer array provided, use its order and only include those keys
			keys = propertyList
		} else {
			// Sort keys per ECMAScript spec: numeric indices first (sorted numerically), then strings (insertion order)
			keys = sortJSONKeys(obj.OwnKeys())
		}

		// Pretty printing with gap
		if gap != "" {
			stepIndent := indent + gap
			result := "{"
			first := true
			for _, key := range keys {
				// Use vm.GetProperty to properly invoke getters
				var prop vm.Value
				var ok bool
				if vmInstance != nil {
					var err error
					prop, err = vmInstance.GetProperty(value, key)
					if err != nil {
						return "", err
					}
					ok = (prop != vm.Undefined)
				} else {
					// Fallback if no VM instance
					prop, ok = obj.GetOwn(key)
				}

				if ok {
					propJSON, err := stringifyValueToJSONWithVisited(vmInstance, prop, visited, gap, stepIndent, key, value, replacerFunc, propertyList)
					if err != nil {
						return "", err
					}
					if propJSON != "" { // Skip undefined properties
						if !first {
							result += ","
						}
						first = false
						result += "\n" + stepIndent
						keyBytes, _ := json.Marshal(key)
						result += string(keyBytes) + ": " + propJSON
					}
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
			var prop vm.Value
			var ok bool
			if vmInstance != nil {
				var err error
				prop, err = vmInstance.GetProperty(value, key)
				if err != nil {
					return "", err
				}
				ok = (prop != vm.Undefined)
			} else {
				// Fallback if no VM instance
				prop, ok = obj.GetOwn(key)
			}

			if ok {
				propJSON, err := stringifyValueToJSONWithVisited(vmInstance, prop, visited, gap, indent, key, value, replacerFunc, propertyList)
				if err != nil {
					return "", err
				}
				if propJSON != "" { // Skip undefined properties
					if !first {
						result += ","
					}
					first = false
					keyBytes, _ := json.Marshal(key)
					result += string(keyBytes) + ":" + propJSON
				}
			}
		}
		result += "}"
		return result, nil
	default:
		return "null", nil
	}
}

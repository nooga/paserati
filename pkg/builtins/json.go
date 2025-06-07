package builtins

import (
	"encoding/json"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"strings"
)

// registerJSON creates and registers the JSON object with its methods
func registerJSON() {
	// Create the JSON object as a PlainObject
	jsonObj := vm.NewObject(vm.Undefined)
	jsonObject := jsonObj.AsPlainObject()

	// Register JSON methods
	jsonObject.SetOwn("parse", vm.NewNativeFunction(1, false, "parse", jsonParseImpl))
	jsonObject.SetOwn("stringify", vm.NewNativeFunction(-1, true, "stringify", jsonStringifyImpl))

	// Define the type for JSON object with all methods
	jsonType := &types.ObjectType{
		Properties: map[string]types.Type{
			"parse": &types.FunctionType{
				ParameterTypes: []types.Type{types.String},
				ReturnType:     types.Any,
				IsVariadic:     false,
			},
			"stringify": &types.FunctionType{
				ParameterTypes:    []types.Type{types.Any}, // First parameter: value to stringify
				ReturnType:        types.String,            // Always returns string in our implementation
				IsVariadic:        true,
				RestParameterType: &types.ArrayType{ElementType: types.Any}, // Optional replacer and space params
			},
		},
	}

	// Register the JSON object
	registerObject("JSON", jsonObj, jsonType)
}

// --- JSON Method Implementations ---

// jsonParseImpl implements JSON.parse(text)
func jsonParseImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		// Throw SyntaxError in real JS, but we'll just return undefined for now
		return vm.Undefined
	}

	text := args[0].ToString()

	// Parse the JSON string using Go's json package
	var result any
	err := json.Unmarshal([]byte(text), &result)
	if err != nil {
		// In real JavaScript this would throw a SyntaxError
		// For now, we'll return undefined
		return vm.Undefined
	}

	// Convert the Go value to a VM value
	return convertGoValueToVMValue(result)
}

// jsonStringifyImpl implements JSON.stringify(value, replacer?, space?)
func jsonStringifyImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.NewString("undefined")
	}

	value := args[0]

	// Convert VM value to Go value for JSON marshaling
	goValue := convertVMValueToGoValue(value)

	// Marshal to JSON bytes
	jsonBytes, err := json.Marshal(goValue)
	if err != nil {
		// In case of circular references or other issues, return "null"
		return vm.NewString("null")
	}

	return vm.NewString(string(jsonBytes))
}

// convertGoValueToVMValue converts a Go any value from json.Unmarshal to a VM value
func convertGoValueToVMValue(value any) vm.Value {
	switch v := value.(type) {
	case nil:
		return vm.Null
	case bool:
		return vm.BooleanValue(v)
	case float64:
		return vm.NumberValue(v)
	case string:
		return vm.NewString(v)
	case []any:
		// Create an array
		arr := vm.NewArray()
		arrayObj := arr.AsArray()
		for i, elem := range v {
			arrayObj.Set(i, convertGoValueToVMValue(elem))
		}
		return arr
	case map[string]any:
		// Create an object
		obj := vm.NewObject(vm.Undefined)
		plainObj := obj.AsPlainObject()
		for key, val := range v {
			plainObj.SetOwn(key, convertGoValueToVMValue(val))
		}
		return obj
	default:
		// Fallback for unknown types
		return vm.Undefined
	}
}

// convertVMValueToGoValue converts a VM value to a Go any for JSON marshaling
func convertVMValueToGoValue(value vm.Value) any {
	switch value.Type() {
	case vm.TypeNull:
		return nil
	case vm.TypeUndefined:
		return nil // JSON doesn't have undefined, so convert to null
	case vm.TypeBoolean:
		return value.AsBoolean()
	case vm.TypeFloatNumber, vm.TypeIntegerNumber:
		num := value.ToFloat()
		// Check if it's an integer
		if num == float64(int64(num)) {
			return int64(num)
		}
		return num
	case vm.TypeString:
		return value.ToString()
	case vm.TypeArray:
		arr := value.AsArray()
		length := arr.Length()
		result := make([]any, length)
		for i := 0; i < length; i++ {
			elem := arr.Get(i)
			result[i] = convertVMValueToGoValue(elem)
		}
		return result
	case vm.TypeObject:
		obj := value.AsPlainObject()
		result := make(map[string]any)

		// Get all own properties using public API
		keys := obj.OwnKeys()
		for _, key := range keys {
			if prop, exists := obj.GetOwn(key); exists {
				result[key] = convertVMValueToGoValue(prop)
			}
		}
		return result
	default:
		// For functions and other non-serializable types, return null
		return nil
	}
}

// Helper function to format JSON with proper indentation (for future use with space parameter)
func formatJSONWithSpacing(jsonStr string, space any) string {
	// This is a simplified version - real JSON.stringify space parameter is more complex
	switch s := space.(type) {
	case float64:
		if s > 0 && s <= 10 {
			return formatJSONWithIndent(jsonStr, strings.Repeat(" ", int(s)))
		}
	case string:
		if len(s) > 0 && len(s) <= 10 {
			return formatJSONWithIndent(jsonStr, s)
		}
	}
	return jsonStr
}

// Helper to format JSON with a specific indent string
func formatJSONWithIndent(jsonStr, indent string) string {
	// This is a very basic implementation
	// A full implementation would need to properly parse and reformat the JSON
	var result strings.Builder
	var level int
	inString := false
	escapeNext := false

	for i, char := range jsonStr {
		switch char {
		case '"':
			if !escapeNext {
				inString = !inString
			}
			result.WriteRune(char)
			escapeNext = false
		case '\\':
			result.WriteRune(char)
			if inString {
				escapeNext = !escapeNext
			}
		case '{', '[':
			result.WriteRune(char)
			if !inString {
				level++
				if i < len(jsonStr)-1 {
					result.WriteRune('\n')
					result.WriteString(strings.Repeat(indent, level))
				}
			}
			escapeNext = false
		case '}', ']':
			if !inString {
				level--
				result.WriteRune('\n')
				result.WriteString(strings.Repeat(indent, level))
			}
			result.WriteRune(char)
			escapeNext = false
		case ',':
			result.WriteRune(char)
			if !inString {
				result.WriteRune('\n')
				result.WriteString(strings.Repeat(indent, level))
			}
			escapeNext = false
		case ':':
			result.WriteRune(char)
			if !inString {
				result.WriteRune(' ')
			}
			escapeNext = false
		default:
			result.WriteRune(char)
			escapeNext = false
		}
	}

	return result.String()
}

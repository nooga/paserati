package builtins

import (
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

	// Define the type for JSON object using the smart constructor pattern
	jsonType := types.NewObjectType().
		WithProperty("parse", types.NewSimpleFunction([]types.Type{types.String}, types.Any)).
		WithProperty("stringify", types.NewVariadicFunction([]types.Type{types.Any}, types.String, &types.ArrayType{ElementType: types.Any}))

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

	// Use the efficient UnmarshalJSON method directly on vm.Value
	var result vm.Value
	err := result.UnmarshalJSON([]byte(text))
	if err != nil {
		// In real JavaScript this would throw a SyntaxError
		// For now, we'll return undefined
		return vm.Undefined
	}

	return result
}

// jsonStringifyImpl implements JSON.stringify(value, replacer?, space?)
func jsonStringifyImpl(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.NewString("undefined")
	}

	value := args[0]

	// Use the efficient MarshalJSON method directly on vm.Value
	jsonBytes, err := value.MarshalJSON()
	if err != nil {
		// In case of circular references or other issues, return "null"
		return vm.NewString("null")
	}

	return vm.NewString(string(jsonBytes))
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

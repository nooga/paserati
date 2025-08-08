package builtins

import (
	"encoding/json"
	"fmt"
	"math"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"strconv"
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
	// Create JSON object
	jsonObj := vm.NewObject(vm.Null).AsPlainObject()

	// Add parse method
	jsonObj.SetOwn("parse", vm.NewNativeFunction(1, false, "parse", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			// Throw SyntaxError for missing argument
			return vm.Undefined, fmt.Errorf("SyntaxError: Unexpected end of JSON input")
		}
		
		text := args[0].ToString()
		return parseJSONToValue(text)
	}))

	// Add stringify method (supports optional replacer and space parameters)
	jsonObj.SetOwn("stringify", vm.NewNativeFunction(1, true, "stringify", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, nil
		}
		
		value := args[0]
		// TODO: Handle replacer (args[1]) and space (args[2]) parameters
		result := stringifyValueToJSON(value)
		if result == "" {
			return vm.Undefined, nil
		}
		return vm.NewString(result), nil
	}))

	// Register JSON object as global
	return ctx.DefineGlobal("JSON", vm.NewValueFromPlainObject(jsonObj))
}

// parseJSONToValue converts a JSON string to a VM Value
func parseJSONToValue(text string) (vm.Value, error) {
	var jsonValue interface{}
	if err := json.Unmarshal([]byte(text), &jsonValue); err != nil {
		// Return proper SyntaxError for invalid JSON
		return vm.Undefined, err
	}
	
	return convertJSONValue(jsonValue), nil
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
			obj.SetOwn(key, convertJSONValue(val))
		}
		return vm.NewValueFromPlainObject(obj)
	default:
		return vm.Undefined
	}
}

// stringifyValueToJSON converts a VM Value to a JSON string
func stringifyValueToJSON(value vm.Value) string {
	switch value.Type() {
	case vm.TypeNull:
		return "null"
	case vm.TypeUndefined:
		return "" // JSON.stringify(undefined) returns undefined (empty string here)
	case vm.TypeBoolean:
		if value.IsTruthy() {
			return "true"
		}
		return "false"
	case vm.TypeFloatNumber, vm.TypeIntegerNumber:
		num := value.ToFloat()
		// Handle special cases
		if math.IsNaN(num) { // NaN
			return "null"
		}
		if math.IsInf(num, 0) { // Infinity
			return "null"
		}
		return strconv.FormatFloat(num, 'f', -1, 64)
	case vm.TypeString:
		bytes, _ := json.Marshal(value.ToString()) // Proper JSON string escaping
		return string(bytes)
	case vm.TypeArray:
		arr := value.AsArray()
		if arr.Length() == 0 {
			return "[]"
		}
		result := "["
		for i := 0; i < arr.Length(); i++ {
			if i > 0 {
				result += ","
			}
			elem := arr.Get(i)
			result += stringifyValueToJSON(elem)
		}
		result += "]"
		return result
	case vm.TypeObject, vm.TypeDictObject:
		// Handle objects
		result := "{"
		first := true
		var obj interface{ OwnKeys() []string; GetOwn(string) (vm.Value, bool) }
		
		if value.Type() == vm.TypeObject {
			obj = value.AsPlainObject()
		} else {
			obj = value.AsDictObject()
		}
		
		for _, key := range obj.OwnKeys() {
			if prop, ok := obj.GetOwn(key); ok {
				propJSON := stringifyValueToJSON(prop)
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
		return result
	default:
		return "null"
	}
}
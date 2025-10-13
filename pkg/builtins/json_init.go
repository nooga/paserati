package builtins

import (
	"encoding/json"
	"fmt"
	"math"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"strconv"
	"unsafe"
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
			// Create a real SyntaxError instance and return it as an exception
			ctor, _ := ctx.VM.GetGlobal("SyntaxError")
			if ctor != vm.Undefined {
				errObj, _ := ctx.VM.Call(ctor, vm.Undefined, []vm.Value{vm.NewString("Unexpected end of JSON input")})
				return vm.Undefined, ctx.VM.NewExceptionError(errObj)
			}
			return vm.Undefined, fmt.Errorf("SyntaxError: Unexpected end of JSON input")
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
	jsonObj.SetOwn("stringify", vm.NewNativeFunction(1, true, "stringify", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, nil
		}

		value := args[0]
		// TODO: Handle replacer (args[1]) parameter

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

		visited := make(map[uintptr]bool)
		result, err := stringifyValueToJSONWithVisited(ctx.VM, value, visited, gap, "", "")
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

// parseJSONToValue converts a JSON string to a VM Value
func parseJSONToValue(text string) (vm.Value, error) {
	var jsonValue interface{}
	if err := json.Unmarshal([]byte(text), &jsonValue); err != nil {
		// Return error up so caller can wrap as SyntaxError
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

// stringifyValueToJSON converts a VM Value to a JSON string (legacy, no circular check)
func stringifyValueToJSON(value vm.Value) string {
	visited := make(map[uintptr]bool)
	result, _ := stringifyValueToJSONWithVisited(nil, value, visited, "", "", "")
	return result
}

// stringifyValueToJSONWithVisited converts a VM Value to a JSON string with circular reference detection
// gap is the indentation string (e.g., "  " for 2 spaces), indent is the current indentation level
// key is the property key (for toJSON method calls)
func stringifyValueToJSONWithVisited(vmInstance *vm.VM, value vm.Value, visited map[uintptr]bool, gap string, indent string, key string) (string, error) {
	// Handle toJSON method if present (only for objects, not arrays for now)
	if value.Type() == vm.TypeObject || value.Type() == vm.TypeDictObject {
		var toJSON vm.Value
		var ok bool

		if value.Type() == vm.TypeObject {
			toJSON, ok = value.AsPlainObject().GetOwn("toJSON")
		} else if value.Type() == vm.TypeDictObject {
			toJSON, ok = value.AsDictObject().GetOwn("toJSON")
		}

		if ok && toJSON.Type() == vm.TypeFunction {
			// Call toJSON method with key as argument
			if vmInstance != nil {
				result, err := vmInstance.Call(toJSON, value, []vm.Value{vm.NewString(key)})
				if err != nil {
					return "", err
				}
				// Recursively stringify the result (but without calling toJSON again)
				value = result
			}
		}
	}

	switch value.Type() {
	case vm.TypeNull:
		return "null", nil
	case vm.TypeUndefined:
		return "", nil // JSON.stringify(undefined) returns undefined (empty string here)
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
				ctor, _ := vmInstance.GetGlobal("TypeError")
				if ctor != vm.Undefined {
					errObj, _ := vmInstance.Call(ctor, vm.Undefined, []vm.Value{vm.NewString("Converting circular structure to JSON")})
					return "", vmInstance.NewExceptionError(errObj)
				}
			}
			return "", fmt.Errorf("TypeError: Converting circular structure to JSON")
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
				elemJSON, err := stringifyValueToJSONWithVisited(vmInstance, elem, visited, gap, stepIndent, elemKey)
				if err != nil {
					return "", err
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
			elemJSON, err := stringifyValueToJSONWithVisited(vmInstance, elem, visited, gap, indent, elemKey)
			if err != nil {
				return "", err
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
				ctor, _ := vmInstance.GetGlobal("TypeError")
				if ctor != vm.Undefined {
					errObj, _ := vmInstance.Call(ctor, vm.Undefined, []vm.Value{vm.NewString("Converting circular structure to JSON")})
					return "", vmInstance.NewExceptionError(errObj)
				}
			}
			return "", fmt.Errorf("TypeError: Converting circular structure to JSON")
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

		// Pretty printing with gap
		if gap != "" {
			stepIndent := indent + gap
			result := "{"
			first := true
			for _, key := range obj.OwnKeys() {
				if prop, ok := obj.GetOwn(key); ok {
					propJSON, err := stringifyValueToJSONWithVisited(vmInstance, prop, visited, gap, stepIndent, key)
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
		for _, key := range obj.OwnKeys() {
			if prop, ok := obj.GetOwn(key); ok {
				propJSON, err := stringifyValueToJSONWithVisited(vmInstance, prop, visited, gap, indent, key)
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

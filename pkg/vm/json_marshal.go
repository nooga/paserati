package vm

import (
	"encoding/json"
	"strconv"
	"strings"
)

// MarshalJSON implements json.Marshaler interface for vm.Value
// This allows direct JSON marshaling without intermediate conversions
func (v Value) MarshalJSON() ([]byte, error) {
	switch v.Type() {
	case TypeNull:
		return []byte("null"), nil
	case TypeUndefined:
		return []byte("null"), nil // JSON doesn't have undefined, so convert to null
	case TypeBoolean:
		if v.AsBoolean() {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case TypeFloatNumber, TypeIntegerNumber:
		num := v.ToFloat()
		// Check if it's an integer
		if num == float64(int64(num)) {
			return []byte(strconv.FormatInt(int64(num), 10)), nil
		}
		return []byte(strconv.FormatFloat(num, 'g', -1, 64)), nil
	case TypeString:
		// Use Go's json.Marshal for proper string escaping
		return json.Marshal(v.ToString())
	case TypeArray:
		arr := v.AsArray()
		length := arr.Length()

		var builder strings.Builder
		builder.WriteByte('[')

		for i := 0; i < length; i++ {
			if i > 0 {
				builder.WriteByte(',')
			}
			elem := arr.Get(i)
			elemJSON, err := elem.MarshalJSON()
			if err != nil {
				return nil, err
			}
			builder.Write(elemJSON)
		}

		builder.WriteByte(']')
		return []byte(builder.String()), nil
	case TypeObject:
		obj := v.AsPlainObject()

		var builder strings.Builder
		builder.WriteByte('{')

		keys := obj.OwnKeys()
		for i, key := range keys {
			if i > 0 {
				builder.WriteByte(',')
			}

			// Marshal the key (always a string in JSON)
			keyJSON, err := json.Marshal(key)
			if err != nil {
				return nil, err
			}
			builder.Write(keyJSON)
			builder.WriteByte(':')

			// Marshal the value
			if prop, exists := obj.GetOwn(key); exists {
				propJSON, err := prop.MarshalJSON()
				if err != nil {
					return nil, err
				}
				builder.Write(propJSON)
			} else {
				builder.WriteString("null")
			}
		}

		builder.WriteByte('}')
		return []byte(builder.String()), nil
	default:
		// For functions and other non-serializable types, return null
		return []byte("null"), nil
	}
}

// UnmarshalJSON implements json.Unmarshaler interface for vm.Value
// This allows direct JSON unmarshaling without intermediate conversions
func (v *Value) UnmarshalJSON(data []byte) error {
	// Parse the JSON using Go's json package first to determine the type
	var intermediate any
	if err := json.Unmarshal(data, &intermediate); err != nil {
		return err
	}

	// Convert the parsed Go value to a VM value
	*v = convertGoValueToVMValue(intermediate)
	return nil
}

// convertGoValueToVMValue converts a Go any value from json.Unmarshal to a VM value
// This is a helper function used by UnmarshalJSON
func convertGoValueToVMValue(value any) Value {
	switch v := value.(type) {
	case nil:
		return Null
	case bool:
		return BooleanValue(v)
	case float64:
		return NumberValue(v)
	case string:
		return NewString(v)
	case []any:
		// Create an array
		arr := NewArray()
		arrayObj := arr.AsArray()
		for i, elem := range v {
			arrayObj.Set(i, convertGoValueToVMValue(elem))
		}
		return arr
	case map[string]any:
		// Create an object
		obj := NewObject(Undefined)
		plainObj := obj.AsPlainObject()
		for key, val := range v {
			plainObj.SetOwn(key, convertGoValueToVMValue(val))
		}
		return obj
	default:
		// Fallback for unknown types
		return Undefined
	}
}

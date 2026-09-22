package vm

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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
		return []byte(QuoteJSONString(v.ToString())), nil
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
			builder.WriteString(QuoteJSONString(key))
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

// UnmarshalJSON implements json.Unmarshaler for vm.Value.
//
// Object keys keep their source order (paserati#522): this used to decode
// into a Go map[string]any, whose iteration order is randomized, so the
// resulting object's property order changed from run to run. It now reads
// the token stream directly.
//
// There is no realm here, so objects get no [[Prototype]]. Code that has a
// VM and wants JSON.parse's result - Object.prototype, lone surrogate
// escapes preserved - should call JSON.parse's implementation instead, as
// Response.json() does.
func (v *Value) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	val, err := decodeJSONValue(dec)
	if err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("unexpected data after top-level JSON value")
	}
	*v = val
	return nil
}

// decodeJSONValue reads one JSON value from dec's token stream.
func decodeJSONValue(dec *json.Decoder) (Value, error) {
	token, err := dec.Token()
	if err != nil {
		return Undefined, err
	}
	switch t := token.(type) {
	case nil:
		return Null, nil
	case bool:
		return BooleanValue(t), nil
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return Undefined, err
		}
		return NumberValue(f), nil
	case string:
		return NewString(t), nil
	case json.Delim:
		switch t {
		case '{':
			obj := NewObject(Undefined)
			plainObj := obj.AsPlainObject()
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return Undefined, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return Undefined, errors.New("expected string key in JSON object")
				}
				val, err := decodeJSONValue(dec)
				if err != nil {
					return Undefined, err
				}
				plainObj.SetOwn(key, val)
			}
			if _, err := dec.Token(); err != nil { // closing '}'
				return Undefined, err
			}
			return obj, nil
		case '[':
			arr := NewArray()
			arrayObj := arr.AsArray()
			for i := 0; dec.More(); i++ {
				elem, err := decodeJSONValue(dec)
				if err != nil {
					return Undefined, err
				}
				arrayObj.Set(i, elem)
			}
			if _, err := dec.Token(); err != nil { // closing ']'
				return Undefined, err
			}
			return arr, nil
		}
	}
	return Undefined, errors.New("unexpected JSON token")
}

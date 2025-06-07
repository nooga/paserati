package vm

import (
	"encoding/json"
	"testing"
)

func TestValueImplementsJSONInterfaces(t *testing.T) {
	// Test that Value implements json.Marshaler
	var _ json.Marshaler = Value{}

	// Test that *Value implements json.Unmarshaler
	var _ json.Unmarshaler = (*Value)(nil)
}

func TestDirectJSONMarshaling(t *testing.T) {
	// Test various value types using direct JSON marshaling
	tests := []struct {
		name     string
		value    Value
		expected string
	}{
		{"null", Null, "null"},
		{"undefined", Undefined, "null"},
		{"true", BooleanValue(true), "true"},
		{"false", BooleanValue(false), "false"},
		{"integer", NumberValue(42), "42"},
		{"float", NumberValue(3.14), "3.14"},
		{"string", NewString("hello"), `"hello"`},
		{"empty array", NewArray(), "[]"},
		{"empty object", NewObject(Undefined), "{}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use Go's json.Marshal which will call our MarshalJSON method
			result, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(result))
			}
		})
	}
}

func TestDirectJSONUnmarshaling(t *testing.T) {
	// Test unmarshaling various JSON types
	tests := []struct {
		name           string
		input          string
		expected       string // We'll check by re-marshaling
		skipExactMatch bool   // For objects where property order may differ
	}{
		{"null", "null", "null", false},
		{"true", "true", "true", false},
		{"false", "false", "false", false},
		{"integer", "42", "42", false},
		{"float", "3.14", "3.14", false},
		{"string", `"hello"`, `"hello"`, false},
		{"array", `[1,2,3]`, "[1,2,3]", false},
		{"object", `{"a":1,"b":"test"}`, `{"a":1,"b":"test"}`, true}, // Skip exact match due to property order
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var value Value

			// Use Go's json.Unmarshal which will call our UnmarshalJSON method
			err := json.Unmarshal([]byte(tt.input), &value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.skipExactMatch {
				// For objects, just verify it's an object and has the right properties
				if value.Type() != TypeObject {
					t.Errorf("expected object type")
					return
				}
				obj := value.AsPlainObject()
				if aProp, exists := obj.GetOwn("a"); !exists || aProp.ToFloat() != 1 {
					t.Errorf("object missing or incorrect 'a' property")
				}
				if bProp, exists := obj.GetOwn("b"); !exists || bProp.ToString() != "test" {
					t.Errorf("object missing or incorrect 'b' property")
				}
			} else {
				// Marshal it back to verify it was parsed correctly
				result, err := json.Marshal(value)
				if err != nil {
					t.Fatalf("unexpected error re-marshaling: %v", err)
				}

				if string(result) != tt.expected {
					t.Errorf("expected %s, got %s", tt.expected, string(result))
				}
			}
		})
	}
}

func TestJSONRoundtrip(t *testing.T) {
	// Create a complex value
	arr := NewArray()
	arrObj := arr.AsArray()
	arrObj.Set(0, NumberValue(1))
	arrObj.Set(1, NewString("test"))
	arrObj.Set(2, BooleanValue(true))

	obj := NewObject(Undefined)
	plainObj := obj.AsPlainObject()
	plainObj.SetOwn("number", NumberValue(42))
	plainObj.SetOwn("string", NewString("hello"))
	plainObj.SetOwn("array", arr)

	// Marshal using Go's json.Marshal (which uses our MarshalJSON)
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Unmarshal using Go's json.Unmarshal (which uses our UnmarshalJSON)
	var restored Value
	err = json.Unmarshal(jsonBytes, &restored)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Verify the structure is correct
	if restored.Type() != TypeObject {
		t.Fatalf("expected object, got %v", restored.Type())
	}

	restoredObj := restored.AsPlainObject()

	// Check number property
	if numProp, exists := restoredObj.GetOwn("number"); !exists || numProp.ToFloat() != 42 {
		t.Errorf("number property not correctly restored")
	}

	// Check string property
	if strProp, exists := restoredObj.GetOwn("string"); !exists || strProp.ToString() != "hello" {
		t.Errorf("string property not correctly restored")
	}

	// Check array property
	if arrProp, exists := restoredObj.GetOwn("array"); !exists || arrProp.Type() != TypeArray {
		t.Errorf("array property not correctly restored")
	} else {
		restoredArr := arrProp.AsArray()
		if restoredArr.Length() != 3 {
			t.Errorf("array length not correctly restored")
		}
		if restoredArr.Get(0).ToFloat() != 1 {
			t.Errorf("array element 0 not correctly restored")
		}
		if restoredArr.Get(1).ToString() != "test" {
			t.Errorf("array element 1 not correctly restored")
		}
		if !restoredArr.Get(2).AsBoolean() {
			t.Errorf("array element 2 not correctly restored")
		}
	}
}

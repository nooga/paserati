package types

import (
	"fmt"
	"strings"
)

// --- Array Types ---

// ArrayType represents the type of an array.
type ArrayType struct {
	ElementType Type
}

func (at *ArrayType) String() string {
	elemTypeStr := "<nil>"
	if at.ElementType != nil { // Add nil check
		elemTypeStr = at.ElementType.String()
	}
	return fmt.Sprintf("%s[]", elemTypeStr)
}
func (at *ArrayType) typeNode() {}
func (at *ArrayType) Equals(other Type) bool {
	otherAt, ok := other.(*ArrayType)
	if !ok {
		return false // Not an ArrayType
	}
	if at == nil || otherAt == nil {
		return at == otherAt // Both must be nil or non-nil
	}
	// Check element type equality
	if (at.ElementType == nil) != (otherAt.ElementType == nil) {
		return false
	} // One nil, one not
	if at.ElementType != nil && !at.ElementType.Equals(otherAt.ElementType) {
		return false // Element types differ
	}
	return true
}

// --- Tuple Types ---

// TupleType represents a tuple type with fixed-length, ordered elements.
// Design mirrors FunctionType's parameter structure for compatibility with spread syntax.
type TupleType struct {
	ElementTypes     []Type // Types of each tuple element (like ParameterTypes in FunctionType)
	OptionalElements []bool // Which elements are optional [string, number?] (like OptionalParams in FunctionType)
	RestElementType  Type   // Type for rest elements [string, ...number[]] (like RestParameterType in FunctionType)
}

func (tt *TupleType) String() string {
	var elements strings.Builder
	elements.WriteString("[")

	numElements := len(tt.ElementTypes)
	for i, elemType := range tt.ElementTypes {
		if elemType != nil {
			elements.WriteString(elemType.String())
		} else {
			elements.WriteString("<nil>")
		}

		// Add optional marker if this element is optional
		if i < len(tt.OptionalElements) && tt.OptionalElements[i] {
			elements.WriteString("?")
		}

		if i < numElements-1 {
			elements.WriteString(", ")
		}
	}

	// Add rest element if present
	if tt.RestElementType != nil {
		if numElements > 0 {
			elements.WriteString(", ")
		}
		elements.WriteString("...")
		elements.WriteString(tt.RestElementType.String())
	}

	elements.WriteString("]")
	return elements.String()
}

func (tt *TupleType) typeNode() {}

func (tt *TupleType) Equals(other Type) bool {
	otherTt, ok := other.(*TupleType)
	if !ok {
		return false // Not a TupleType
	}
	if tt == nil || otherTt == nil {
		return tt == otherTt // Both must be nil or non-nil
	}

	// Check element types
	if len(tt.ElementTypes) != len(otherTt.ElementTypes) {
		return false // Different number of elements
	}
	if len(tt.OptionalElements) != len(otherTt.OptionalElements) {
		return false // Different optional element tracking
	}

	// Check element types (invariant check for simplicity)
	for i, elem1 := range tt.ElementTypes {
		elem2 := otherTt.ElementTypes[i]
		if (elem1 == nil) != (elem2 == nil) {
			return false // One nil, one not
		}
		if elem1 != nil && !elem1.Equals(elem2) {
			return false // Element types differ
		}
	}

	// Check optional element markers
	for i, opt1 := range tt.OptionalElements {
		if opt1 != otherTt.OptionalElements[i] {
			return false // Optional element markers differ
		}
	}

	// Check rest element type
	if (tt.RestElementType == nil) != (otherTt.RestElementType == nil) {
		return false // One has rest element, other doesn't
	}
	if tt.RestElementType != nil && !tt.RestElementType.Equals(otherTt.RestElementType) {
		return false // Rest element types differ
	}

	return true // All checks passed
}

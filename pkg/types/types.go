package types

import (
	"fmt"
	"strings"
)

// Type is the interface implemented by all type representations.
type Type interface {
	// String returns a string representation of the type, suitable for debugging or printing.
	String() string
	// Equals checks if this type is structurally equivalent to another type.
	Equals(other Type) bool
	// TODO: Add IsAssignableTo(target Type) bool

	// typeNode() is a marker method to ensure only types defined in this package
	// can be assigned to the Type interface. This prevents accidental implementation
	// elsewhere and makes the type system closed for now.
	typeNode()
}

// ForwardReferenceType represents a forward reference to a generic class being defined
type ForwardReferenceType struct {
	ClassName      string
	TypeParameters []*TypeParameter
}

func (frt *ForwardReferenceType) String() string {
	return frt.ClassName
}

func (frt *ForwardReferenceType) Equals(other Type) bool {
	if otherFrt, ok := other.(*ForwardReferenceType); ok {
		return frt.ClassName == otherFrt.ClassName
	}
	return false
}

func (frt *ForwardReferenceType) typeNode() {}

// MappedType represents a mapped type like { [P in K]: T }
// This is used for utility types like Partial<T>, Readonly<T>, etc.
type MappedType struct {
	TypeParameter  string // The iteration variable (e.g., "P" in [P in K])
	ConstraintType Type   // The type being iterated over (e.g., K in [P in K])
	ValueType      Type   // The resulting value type for each property
	
	// Modifiers for the mapped type
	ReadonlyModifier string // "+", "-", or "" (for readonly modifier)
	OptionalModifier string // "+", "-", or "" (for optional modifier)
}

func (mt *MappedType) String() string {
	var modifiers []string
	if mt.ReadonlyModifier == "+" {
		modifiers = append(modifiers, "readonly")
	} else if mt.ReadonlyModifier == "-" {
		modifiers = append(modifiers, "-readonly")
	}
	
	optionalMark := ""
	if mt.OptionalModifier == "+" {
		optionalMark = "?"
	} else if mt.OptionalModifier == "-" {
		optionalMark = "-?"
	}
	
	modifierStr := ""
	if len(modifiers) > 0 {
		modifierStr = strings.Join(modifiers, " ") + " "
	}
	
	constraintStr := "unknown"
	if mt.ConstraintType != nil {
		constraintStr = mt.ConstraintType.String()
	}
	
	valueStr := "unknown"
	if mt.ValueType != nil {
		valueStr = mt.ValueType.String()
	}
	
	return fmt.Sprintf("{ %s[%s in %s]%s: %s }", modifierStr, mt.TypeParameter, constraintStr, optionalMark, valueStr)
}

func (mt *MappedType) Equals(other Type) bool {
	otherMt, ok := other.(*MappedType)
	if !ok {
		return false
	}
	
	if mt.TypeParameter != otherMt.TypeParameter {
		return false
	}
	
	if mt.ReadonlyModifier != otherMt.ReadonlyModifier {
		return false
	}
	
	if mt.OptionalModifier != otherMt.OptionalModifier {
		return false
	}
	
	if !mt.ConstraintType.Equals(otherMt.ConstraintType) {
		return false
	}
	
	return mt.ValueType.Equals(otherMt.ValueType)
}

func (mt *MappedType) typeNode() {}

// KeyofType represents a keyof type operator like keyof T
// This evaluates to a union of string literal types representing the keys of the operand type
type KeyofType struct {
	OperandType Type // The type we're getting keys from
}

func (kt *KeyofType) String() string {
	operandStr := "unknown"
	if kt.OperandType != nil {
		operandStr = kt.OperandType.String()
	}
	return fmt.Sprintf("keyof %s", operandStr)
}

func (kt *KeyofType) Equals(other Type) bool {
	otherKt, ok := other.(*KeyofType)
	if !ok {
		return false
	}
	return kt.OperandType.Equals(otherKt.OperandType)
}

func (kt *KeyofType) typeNode() {}

// TypePredicateType represents a type predicate like 'x is string'
// This is used in function return types to indicate type guards
type TypePredicateType struct {
	ParameterName string // The parameter being tested (e.g., "x" in "x is string")
	Type          Type   // The type being tested for
}

func (tpt *TypePredicateType) String() string {
	typeStr := "unknown"
	if tpt.Type != nil {
		typeStr = tpt.Type.String()
	}
	return fmt.Sprintf("%s is %s", tpt.ParameterName, typeStr)
}

func (tpt *TypePredicateType) Equals(other Type) bool {
	otherTpt, ok := other.(*TypePredicateType)
	if !ok {
		return false
	}
	if tpt.ParameterName != otherTpt.ParameterName {
		return false
	}
	return tpt.Type.Equals(otherTpt.Type)
}

func (tpt *TypePredicateType) typeNode() {}

// IndexedAccessType represents an indexed access type like T[K]
// This is used to access properties of a type using a key type
type IndexedAccessType struct {
	ObjectType Type // The type we're indexing into (e.g., T in T[K])
	IndexType  Type // The key type used for indexing (e.g., K in T[K])
}

func (iat *IndexedAccessType) String() string {
	objectStr := "unknown"
	if iat.ObjectType != nil {
		objectStr = iat.ObjectType.String()
	}
	indexStr := "unknown"
	if iat.IndexType != nil {
		indexStr = iat.IndexType.String()
	}
	return fmt.Sprintf("%s[%s]", objectStr, indexStr)
}

func (iat *IndexedAccessType) Equals(other Type) bool {
	otherIat, ok := other.(*IndexedAccessType)
	if !ok {
		return false
	}
	return iat.ObjectType.Equals(otherIat.ObjectType) && iat.IndexType.Equals(otherIat.IndexType)
}

func (iat *IndexedAccessType) typeNode() {}

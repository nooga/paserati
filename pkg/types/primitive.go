package types

import (
	"paserati/pkg/vm"
)

// --- Primitive Types ---

// Primitive represents a fundamental, non-composite type.
type Primitive struct {
	Name string
}

func (p *Primitive) String() string {
	return p.Name
}
func (p *Primitive) typeNode() {}
func (p *Primitive) Equals(other Type) bool {
	// Primitives are singletons, so pointer equality is sufficient.
	return p == other
}

// Pre-defined instances for common primitive types
var (
	Number    = &Primitive{Name: "number"}
	String    = &Primitive{Name: "string"}
	Boolean   = &Primitive{Name: "boolean"}
	Null      = &Primitive{Name: "null"}
	Undefined = &Primitive{Name: "undefined"}
	Any       = &Primitive{Name: "any"}
	Unknown   = &Primitive{Name: "unknown"}
	Never     = &Primitive{Name: "never"}
	Void      = &Primitive{Name: "void"}
)

// TypeofResultType represents the union of all possible string literals that the typeof operator can return
var TypeofResultType = NewUnionType(
	&LiteralType{Value: vm.String("undefined")},
	&LiteralType{Value: vm.String("boolean")},
	&LiteralType{Value: vm.String("number")},
	&LiteralType{Value: vm.String("string")},
	&LiteralType{Value: vm.String("function")},
	&LiteralType{Value: vm.String("object")},
	// Note: In TypeScript/JavaScript, typeof can also return "symbol" and "bigint" in newer versions
	// but for now we'll stick to the basic set that our VM supports
)

// --- Literal Types ---

// LiteralType represents a specific literal value used as a type.
type LiteralType struct {
	Value vm.Value // Holds the literal value (e.g., vm.Number(5), vm.String("hello"))
}

func (lt *LiteralType) isType()        {}
func (lt *LiteralType) Name() string   { return lt.Value.ToString() }
func (lt *LiteralType) String() string { return lt.Value.ToString() }
func (lt *LiteralType) typeNode()      {}
func (lt *LiteralType) Equals(other Type) bool {
	otherLt, ok := other.(*LiteralType)
	if !ok {
		return false // Not a LiteralType
	}
	if lt == nil || otherLt == nil {
		return lt == otherLt // Both must be nil or non-nil
	}

	// Check if underlying vm.Value types are the same
	if lt.Value.Type() != otherLt.Value.Type() {
		return false
	}

	// Compare the actual values based on type
	switch lt.Value.Type() {
	case vm.TypeFloatNumber, vm.TypeIntegerNumber:
		return vm.AsNumber(lt.Value) == vm.AsNumber(otherLt.Value)
	case vm.TypeString:
		return vm.AsString(lt.Value) == vm.AsString(otherLt.Value)
	case vm.TypeBoolean:
		return lt.Value.AsBoolean() == otherLt.Value.AsBoolean()
	case vm.TypeNull, vm.TypeUndefined:
		return true // Already checked type match
	default:
		return false // Should not happen for literal types
	}
}

// --- Type Widening ---

// GetWidenedType converts literal types to their corresponding primitive base types.
// Other types are returned unchanged.
func GetWidenedType(t Type) Type {
	if litType, ok := t.(*LiteralType); ok {
		switch litType.Value.Type() {
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			return Number
		case vm.TypeString:
			return String
		case vm.TypeBoolean:
			return Boolean
		case vm.TypeNull:
			return Null // Null widens to null
		case vm.TypeUndefined:
			return Undefined // Undefined widens to undefined
		default:
			// Should not happen for valid literal types (like Function/Closure)
			return t // Return original if unexpected underlying type
		}
	}
	// TODO: Should unions containing only literals of the same base type also widen?
	// e.g., should (1 | 2 | 3) widen to number? Probably.
	// This would require more complex logic here or in NewUnionType.
	return t // Not a literal type, return as is
}

// WidenType converts literal types to their primitive equivalents
func WidenType(t Type) Type {
	return GetWidenedType(t) // Use existing function
}

// IsPrimitive returns true if the type is a primitive type
func IsPrimitive(t Type) bool {
	_, ok := t.(*Primitive)
	return ok
}

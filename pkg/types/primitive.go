package types

import (
	"github.com/nooga/paserati/pkg/vm"
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
	Symbol    = &Primitive{Name: "symbol"}
	BigInt    = &Primitive{Name: "bigint"}
	Null      = &Primitive{Name: "null"}
	Undefined = &Primitive{Name: "undefined"}
	Any       = &Primitive{Name: "any"}
	Unknown   = &Primitive{Name: "unknown"}
	Never     = &Primitive{Name: "never"}
	Void      = &Primitive{Name: "void"}
	RegExp    = &Primitive{Name: "RegExp"}
)

// TypeofResultType represents the union of all possible string literals that the typeof operator can return
var TypeofResultType = NewUnionType(
	&LiteralType{Value: vm.String("undefined")},
	&LiteralType{Value: vm.String("boolean")},
	&LiteralType{Value: vm.String("number")},
	&LiteralType{Value: vm.String("bigint")},
	&LiteralType{Value: vm.String("string")},
	&LiteralType{Value: vm.String("function")},
	&LiteralType{Value: vm.String("object")},
	// Note: In TypeScript/JavaScript, typeof can also return "symbol" in newer versions
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
	case vm.TypeBigInt:
		return lt.Value.AsBigInt().Cmp(otherLt.Value.AsBigInt()) == 0
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

// IsPrimitive returns true if the type is a primitive type
func IsPrimitive(t Type) bool {
	_, ok := t.(*Primitive)
	return ok
}

// IsNullOrUndefined returns true if the type is null or undefined
func IsNullOrUndefined(t Type) bool {
	return t == Null || t == Undefined
}

// IsLiteral returns true if the type is a literal type
func IsLiteral(t Type) bool {
	_, ok := t.(*LiteralType)
	return ok
}

// GetTypeofResult returns the TypeScript-compatible string literal representing
// the result of the typeof operator when applied to a value of the given type
func GetTypeofResult(t Type) Type {
	// Widen the type first
	widened := GetWidenedType(t)

	switch widened {
	case String:
		return &LiteralType{Value: vm.String("string")}
	case Number:
		return &LiteralType{Value: vm.String("number")}
	case BigInt:
		return &LiteralType{Value: vm.String("bigint")}
	case Boolean:
		return &LiteralType{Value: vm.String("boolean")}
	case Symbol:
		return &LiteralType{Value: vm.String("symbol")}
	case Undefined:
		return &LiteralType{Value: vm.String("undefined")}
	case Null:
		return &LiteralType{Value: vm.String("object")} // typeof null === "object" in JS
	}

	// Handle other types
	switch widened.(type) {
	case *ObjectType:
		if ObjectTypeIsCallable(widened.(*ObjectType)) {
			return &LiteralType{Value: vm.String("function")}
		}
		return &LiteralType{Value: vm.String("object")}
	case *ArrayType:
		return &LiteralType{Value: vm.String("object")}
	default:
		// For unknown types, default to "object" (most conservative approach)
		return &LiteralType{Value: vm.String("object")}
	}
}

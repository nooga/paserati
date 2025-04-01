package types

import "fmt"

// TODO: Define type system (interfaces, structs for primitive types, object types, etc.)

// Type is the interface implemented by all type representations.
type Type interface {
	// String returns a string representation of the type, suitable for debugging or printing.
	String() string
	// TODO: Add Equals(other Type) bool
	// TODO: Add IsAssignableTo(target Type) bool

	// typeNode() is a marker method to ensure only types defined in this package
	// can be assigned to the Type interface. This prevents accidental implementation
	// elsewhere and makes the type system closed for now.
	typeNode()
}

// --- Primitive Types ---

// Primitive represents a fundamental, non-composite type.
type Primitive struct {
	Name string
}

func (p *Primitive) String() string {
	return p.Name
}
func (p *Primitive) typeNode() {}

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
	// Add Void later if needed for function returns? TS uses undefined.
)

// --- Simple Composite Types (Placeholder Structs) ---

// FunctionType represents the type of a function.
type FunctionType struct {
	ParameterTypes []Type // TODO: Parameter names?
	ReturnType     Type
}

func (ft *FunctionType) String() string {
	// Basic representation for now
	params := ""
	for i, p := range ft.ParameterTypes {
		if i > 0 {
			params += ", "
		}
		params += p.String()
	}
	return fmt.Sprintf("(%s) => %s", params, ft.ReturnType.String())
}
func (ft *FunctionType) typeNode() {}

// ArrayType represents the type of an array.
type ArrayType struct {
	ElementType Type
}

func (at *ArrayType) String() string {
	return fmt.Sprintf("%s[]", at.ElementType.String()) // Or use Array<T> syntax?
}
func (at *ArrayType) typeNode() {}

// ObjectType represents the type of an object literal or interface.
type ObjectType struct {
	// Using a map for simplicity now. Consider ordered map or slice for stability.
	Properties map[string]Type
	// TODO: Index Signatures?
}

func (ot *ObjectType) String() string {
	// Simple representation
	props := ""
	i := 0
	for name, typ := range ot.Properties {
		if i > 0 {
			props += "; "
		}
		props += fmt.Sprintf("%s: %s", name, typ.String())
		i++
	}
	return fmt.Sprintf("{ %s }", props)
}
func (ot *ObjectType) typeNode() {}

// AliasType represents a named type alias.
type AliasType struct {
	Name         string
	ResolvedType Type // The actual type this alias points to after resolution
}

func (at *AliasType) String() string {
	return at.Name // Simply return the alias name
}
func (at *AliasType) typeNode() {}

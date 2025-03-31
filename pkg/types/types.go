package types

import (
	"fmt"
	"paserati/pkg/vm"
	"strings"
)

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
	Void      = &Primitive{Name: "void"}
	// Add Void later if needed for function returns? TS uses undefined.
)

// --- Simple Composite Types (Placeholder Structs) ---

// FunctionType represents the type of a function.
type FunctionType struct {
	ParameterTypes []Type
	ReturnType     Type
}

func (ft *FunctionType) String() string {
	params := ""
	for i, p := range ft.ParameterTypes {
		if i > 0 {
			params += ", "
		}
		if p != nil { // Add nil check
			params += p.String()
		} else {
			params += "<nil>"
		}
	}
	retTypeStr := "<nil>"
	if ft.ReturnType != nil { // Add nil check
		retTypeStr = ft.ReturnType.String()
	}
	return fmt.Sprintf("(%s) => %s", params, retTypeStr)
}
func (ft *FunctionType) typeNode() {}

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

// ObjectType represents the type of an object literal or interface.
type ObjectType struct {
	// Using a map for simplicity now. Consider ordered map or slice for stability.
	Properties map[string]Type
	// TODO: Index Signatures?
}

func (ot *ObjectType) String() string {
	props := ""
	i := 0
	for name, typ := range ot.Properties {
		if i > 0 {
			props += "; "
		}
		typStr := "<nil>"
		if typ != nil { // Add nil check
			typStr = typ.String()
		}
		props += fmt.Sprintf("%s: %s", name, typStr)
		i++
	}
	return fmt.Sprintf("{ %s }", props)
}
func (ot *ObjectType) typeNode() {}

// --- NEW: UnionType ---

// UnionType represents a union of multiple types (e.g., string | number).
// Stores constituent types in a slice.
type UnionType struct {
	Types []Type // Slice holding the types in the union
	// TODO: Consider storing unique types or a canonical representation?
}

func (ut *UnionType) String() string {
	strs := make([]string, len(ut.Types))
	for i, t := range ut.Types {
		if t != nil {
			strs[i] = t.String()
		} else {
			strs[i] = "<nil>"
		}
	}
	// TODO: Consider sorting for canonical representation?
	return strings.Join(strs, " | ")
}
func (ut *UnionType) typeNode() {}

// --- NEW: LiteralType ---

// LiteralType represents a specific literal value used as a type.
type LiteralType struct {
	Value vm.Value // Store the actual VM value (String, Number, Bool)
}

func (lt *LiteralType) String() string {
	// Use the String() method of the underlying vm.Value
	return lt.Value.String()
}
func (lt *LiteralType) typeNode() {}

// --- NEW: UnionType Constructor ---

// NewUnionType creates a new union type from the given types.
// It flattens nested unions and removes duplicate types.
// If the resulting union has only one type, it returns that type directly.
func NewUnionType(ts ...Type) Type {
	uniqueTypes := make(map[Type]bool)
	flattenedTypes := make([]Type, 0, len(ts))

	var collectTypes func(t Type)
	collectTypes = func(t Type) {
		if t == nil { // Should not happen, but be safe
			return
		}
		if union, ok := t.(*UnionType); ok {
			// If it's a union, recursively collect its members
			for _, member := range union.Types {
				collectTypes(member)
			}
		} else {
			// If it's not a union, add it if unique
			if !uniqueTypes[t] {
				uniqueTypes[t] = true
				flattenedTypes = append(flattenedTypes, t)
			}
		}
	}

	// Collect types from the input slice
	for _, t := range ts {
		collectTypes(t)
	}

	// Handle simplification
	if len(flattenedTypes) == 0 {
		// Should this happen? Maybe return Never or Any?
		// Let's return Never for an empty union.
		return Never
	} else if len(flattenedTypes) == 1 {
		// If only one unique type remains, return it directly
		return flattenedTypes[0]
	}

	// TODO: Sort flattenedTypes for a canonical representation?
	return &UnionType{Types: flattenedTypes}
}

// AliasType represents a named type alias.
type AliasType struct {
	Name         string
	ResolvedType Type // The actual type this alias points to after resolution
}

func (at *AliasType) String() string {
	return at.Name
}
func (at *AliasType) typeNode() {}

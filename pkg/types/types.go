package types

import (
	"fmt"
	"paserati/pkg/vm"
	"sort"
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
	// Add Void later if needed for function returns? TS uses undefined.
)

// --- Simple Composite Types (Placeholder Structs) ---

// FunctionType represents the type of a function.
type FunctionType struct {
	ParameterTypes []Type
	ReturnType     Type
	IsVariadic     bool // Indicates if the function accepts variable arguments
}

func (ft *FunctionType) String() string {
	var params strings.Builder // Use strings.Builder for efficiency
	params.WriteString("(")
	numParams := len(ft.ParameterTypes)
	for i, p := range ft.ParameterTypes {
		if ft.IsVariadic && i == numParams-1 {
			// For variadic, assume last param type describes the rest elements
			params.WriteString("...")
			if p != nil {
				params.WriteString(p.String()) // e.g., ...any[] or ...number[]
			} else {
				params.WriteString("<nil>") // Default if type is nil? Or maybe error?
			}
		} else {
			if p != nil {
				params.WriteString(p.String())
			} else {
				params.WriteString("<nil>") // Represent nil param type as any?
			}
		}
		if i < numParams-1 {
			params.WriteString(", ")
		}
	}
	params.WriteString(")")

	retTypeStr := "void" // Default to void if nil? Or unknown?
	if ft.ReturnType != nil {
		retTypeStr = ft.ReturnType.String()
	}

	return fmt.Sprintf("%s => %s", params.String(), retTypeStr)
}
func (ft *FunctionType) typeNode() {}
func (ft *FunctionType) Equals(other Type) bool {
	otherFt, ok := other.(*FunctionType)
	if !ok {
		return false // Not even a FunctionType
	}
	if ft == nil || otherFt == nil {
		return ft == otherFt // Both must be nil or non-nil
	}
	if len(ft.ParameterTypes) != len(otherFt.ParameterTypes) {
		return false // Different number of parameters
	}
	if ft.IsVariadic != otherFt.IsVariadic {
		return false
	}

	// Check parameter types (invariant check for simplicity)
	for i, p1 := range ft.ParameterTypes {
		p2 := otherFt.ParameterTypes[i]
		if (p1 == nil) != (p2 == nil) {
			return false
		} // One nil, one not
		if p1 != nil && !p1.Equals(p2) {
			return false // Parameter types differ
		}
	}
	// Check return type
	if (ft.ReturnType == nil) != (otherFt.ReturnType == nil) {
		return false
	} // One nil, one not
	if ft.ReturnType != nil && !ft.ReturnType.Equals(otherFt.ReturnType) {
		return false // Return types differ
	}
	return true // All checks passed
}

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
func (ot *ObjectType) Equals(other Type) bool {
	otherOt, ok := other.(*ObjectType)
	if !ok {
		return false // Not an ObjectType
	}
	if ot == nil || otherOt == nil {
		return ot == otherOt // Both must be nil or non-nil
	}

	// Use reflect.DeepEqual for comparing the maps of properties recursively.
	// This relies on the Equals methods of the contained types.
	// Note: This assumes map iteration order doesn't matter for equality.
	// It also handles nil maps correctly.
	if len(ot.Properties) != len(otherOt.Properties) {
		return false // Different number of properties
	}
	if len(ot.Properties) == 0 {
		return true // Both are empty objects
	}

	// Check each property
	for key, t1 := range ot.Properties {
		t2, exists := otherOt.Properties[key]
		if !exists {
			return false // Key missing in other object
		}
		if (t1 == nil) != (t2 == nil) {
			return false
		} // One type is nil, the other isn't
		if t1 != nil && !t1.Equals(t2) {
			return false // Property types are not equal
		}
	}
	return true // All properties matched
}

// --- NEW: UnionType ---

// UnionType represents a union of multiple types (e.g., string | number).
// Stores constituent types in a slice.
type UnionType struct {
	Types []Type // Slice holding the types in the union
	// TODO: Consider storing unique types or a canonical representation?
}

func (ut *UnionType) String() string {
	typesStr := ""
	for i, t := range ut.Types {
		if i > 0 {
			typesStr += " | "
		}
		typesStr += t.String()
	}
	return typesStr
}
func (ut *UnionType) typeNode() {}
func (ut *UnionType) Equals(other Type) bool {
	otherUt, ok := other.(*UnionType)
	if !ok {
		return false // Not a UnionType
	}
	if ut == nil || otherUt == nil {
		return ut == otherUt // Both must be nil or non-nil
	}

	// Unions are equal if they contain the same set of unique types, regardless of order.
	if len(ut.Types) != len(otherUt.Types) {
		return false // Must have the same number of unique constituent types
	}

	// Create boolean maps to track matches
	matched1 := make([]bool, len(ut.Types))
	matched2 := make([]bool, len(otherUt.Types))

	for i, t1 := range ut.Types {
		foundMatch := false
		for j, t2 := range otherUt.Types {
			if !matched2[j] && t1.Equals(t2) {
				matched1[i] = true
				matched2[j] = true
				foundMatch = true
				break
			}
		}
		if !foundMatch {
			return false // Type t1 from first union not found in second
		}
	}

	// We only need to check one way because we already verified lengths are equal.
	// If every element in ut.Types has a unique match in otherUt.Types, they are equivalent sets.
	return true
}

// --- NEW: LiteralType ---

// LiteralType represents a specific literal value used as a type.
type LiteralType struct {
	Value vm.Value // Holds the literal value (e.g., vm.Number(5), vm.String("hello"))
}

func (lt *LiteralType) isType()        {}
func (lt *LiteralType) Name() string   { return lt.Value.String() }
func (lt *LiteralType) String() string { return lt.Value.String() }
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
	if lt.Value.Type != otherLt.Value.Type {
		return false
	}

	// Compare the actual values based on type
	// Using reflect.DeepEqual is generally safe for the underlying value types (float64, string, bool)
	// Note: This won't work correctly if vm.Value.Value can hold pointers that aren't comparable this way,
	// but for primitives it should be fine.
	//return reflect.DeepEqual(lt.Value.Value, otherLt.Value.Value)
	//Alternative: Use vm specific comparisons if DeepEqual isn't appropriate
	switch lt.Value.Type {
	case vm.TypeNumber:
		return vm.AsNumber(lt.Value) == vm.AsNumber(otherLt.Value)
	case vm.TypeString:
		return vm.AsString(lt.Value) == vm.AsString(otherLt.Value)
	case vm.TypeBool:
		return vm.AsBool(lt.Value) == vm.AsBool(otherLt.Value)
	case vm.TypeNull, vm.TypeUndefined:
		return true // Already checked type match
	default:
		return false // Should not happen for literal types
	}
}

// --- NEW: UnionType Constructor ---

// NewUnionType creates a new union type from the given types.
// It flattens nested unions and removes duplicate types using structural equality.
func NewUnionType(ts ...Type) Type {
	// Use a slice to collect potential members after flattening
	potentialMembers := make([]Type, 0, len(ts))

	var collectTypes func(t Type)
	collectTypes = func(t Type) {
		if t == nil {
			return
		}
		if union, ok := t.(*UnionType); ok {
			// Flatten nested unions
			for _, member := range union.Types {
				collectTypes(member)
			}
		} else if t != Never { // Don't include Never in unions unless it's the only type
			potentialMembers = append(potentialMembers, t)
		}
	}

	// Collect and flatten all input types
	for _, t := range ts {
		collectTypes(t)
	}

	// Filter for unique types using the Equals method
	uniqueMembers := make([]Type, 0, len(potentialMembers))
	for _, pm := range potentialMembers {
		isDuplicate := false
		for _, um := range uniqueMembers {
			if pm.Equals(um) { // <<< USE Equals METHOD
				isDuplicate = true
				break
			}
		}
		if !isDuplicate {
			uniqueMembers = append(uniqueMembers, pm)
		}
	}

	// Handle simplification
	if len(uniqueMembers) == 0 {
		// If only Never types were input, or input was empty
		return Never
	} else if len(uniqueMembers) == 1 {
		// If only one unique type remains, return it directly
		return uniqueMembers[0]
	}

	// Sort the unique types for a canonical string representation (optional but good)
	sort.SliceStable(uniqueMembers, func(i, j int) bool {
		// Basic sort by string representation for consistency
		return uniqueMembers[i].String() < uniqueMembers[j].String()
	})

	return &UnionType{Types: uniqueMembers}
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
func (at *AliasType) Equals(other Type) bool {
	// An alias type should probably be considered equal to its resolved type
	// for the purposes of structural comparison. Comparing two aliases directly
	// might be less useful than comparing what they resolve to.
	// However, resolving cycles needs care. Let's compare the resolved type for now.

	// Avoid infinite recursion if an alias points to itself or has cycles
	// TODO: Implement proper cycle detection if needed. For now, assume simple cases.

	if at == nil || other == nil {
		return at == other
	}

	// Check if the other type IS the same alias by pointer
	if otherAt, ok := other.(*AliasType); ok && at == otherAt {
		return true
	}

	// Compare based on the *resolved* type of the alias
	// Note: Ensure ResolvedType is populated before Equals is called.
	if at.ResolvedType == nil {
		// Cannot compare if not resolved
		return false // Or panic? Indicate internal error.
	}
	// Recursively call Equals on the resolved type against the other type
	return at.ResolvedType.Equals(other)

	// Alternative: Consider two different AliasTypes equal if their names
	// AND resolved types are equal?
	// otherAt, ok := other.(*AliasType)
	// if !ok { return false } // Not an AliasType
	// if at.Name != otherAt.Name { return false } // Different names
	// if (at.ResolvedType == nil) != (otherAt.ResolvedType == nil) { return false }
	// if at.ResolvedType != nil && !at.ResolvedType.Equals(otherAt.ResolvedType) { return false }
	// return true
}

// --- NEW: GetWidenedType Function ---

// GetWidenedType converts literal types to their corresponding primitive base types.
// Other types are returned unchanged.
func GetWidenedType(t Type) Type {
	if litType, ok := t.(*LiteralType); ok {
		switch litType.Value.Type {
		case vm.TypeNumber:
			return Number
		case vm.TypeString:
			return String
		case vm.TypeBool:
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

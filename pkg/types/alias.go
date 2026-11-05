package types

import "strings"

// --- Alias Types ---

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

// GetEffectiveType resolves aliases and returns the actual type
func GetEffectiveType(t Type) Type {
	if aliasType, ok := t.(*AliasType); ok && aliasType.ResolvedType != nil {
		return GetEffectiveType(aliasType.ResolvedType)
	}
	return t
}

// AliasRef names a type by the type alias it was made from (`YesNo`,
// `Partial<T>`), as tsc's aliasSymbol and aliasTypeArguments do. It affects
// only printing: a union, intersection, mapped or conditional type made by
// an alias declaration or instantiation prints as the alias.
type AliasRef struct {
	Name string
	Args []Type
}

// String prints the alias reference: `Name` or `Name<A, B>`.
func (a *AliasRef) String() string {
	if len(a.Args) == 0 {
		return a.Name
	}
	args := make([]string, len(a.Args))
	for i, t := range a.Args {
		if t == nil {
			args[i] = "any"
		} else {
			args[i] = t.String()
		}
	}
	return a.Name + "<" + strings.Join(args, ", ") + ">"
}

// TypeAliasOf returns the alias a type prints as, if any.
func TypeAliasOf(t Type) *AliasRef {
	switch tt := t.(type) {
	case *UnionType:
		return tt.Alias
	case *IntersectionType:
		return tt.Alias
	case *MappedType:
		return tt.Alias
	case *ConditionalType:
		return tt.Alias
	case *ObjectType:
		return tt.Alias
	}
	return nil
}

// SetTypeAlias makes t print as alias. It is a no-op for kinds that do not
// carry an alias, and for a type that is already named.
func SetTypeAlias(t Type, alias *AliasRef) {
	switch tt := t.(type) {
	case *UnionType:
		if tt.Alias == nil {
			tt.Alias = alias
		}
	case *IntersectionType:
		if tt.Alias == nil {
			tt.Alias = alias
		}
	case *MappedType:
		if tt.Alias == nil {
			tt.Alias = alias
		}
	case *ConditionalType:
		if tt.Alias == nil {
			tt.Alias = alias
		}
	case *ObjectType:
		if tt.Alias == nil && tt.DisplayName == "" && tt.GenericName == "" && tt.LibName == "" && tt.ClassMeta == nil {
			tt.Alias = alias
		}
	}
}

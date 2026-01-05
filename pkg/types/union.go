package types

import (
	"sort"
)

// --- Union Types ---

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

// ContainsType checks if the union contains a type that equals the given type
func (ut *UnionType) ContainsType(target Type) bool {
	for _, t := range ut.Types {
		if t.Equals(target) {
			return true
		}
	}
	return false
}

// RemoveType returns a new union with the specified type removed
// Returns the modified union type, or the single remaining type if only one remains
func (ut *UnionType) RemoveType(target Type) Type {
	var remainingTypes []Type

	for _, t := range ut.Types {
		if !t.Equals(target) {
			remainingTypes = append(remainingTypes, t)
		}
	}

	// Use NewUnionType to handle simplification (single type, etc.)
	return NewUnionType(remainingTypes...)
}

// --- Union Type Constructor ---

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

// RemoveNullUndefined removes null and undefined from a type.
// If the type is a union, it filters out null and undefined members.
// If the type is exactly null or undefined, returns never.
// Otherwise returns the original type unchanged.
func RemoveNullUndefined(t Type) Type {
	if t == nil {
		return t
	}

	// If it's exactly null or undefined, return never (or the original if you prefer)
	if t.Equals(Null) || t.Equals(Undefined) {
		return Never
	}

	// If it's a union, filter out null and undefined
	if union, ok := t.(*UnionType); ok {
		var filtered []Type
		for _, member := range union.Types {
			if !member.Equals(Null) && !member.Equals(Undefined) {
				filtered = append(filtered, member)
			}
		}
		// Use NewUnionType to handle simplification
		if len(filtered) == 0 {
			return Never
		}
		return NewUnionType(filtered...)
	}

	// Not a union and not null/undefined, return as-is
	return t
}

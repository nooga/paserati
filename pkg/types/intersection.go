package types

import (
	"github.com/nooga/paserati/pkg/vm"
)

// --- Intersection Types ---

// IntersectionType represents an intersection of multiple types (e.g., A & B).
// A value of intersection type must satisfy ALL constituent types simultaneously.
type IntersectionType struct {
	Types []Type // Slice holding the types in the intersection
}

func (it *IntersectionType) String() string {
	typesStr := ""
	for i, t := range it.Types {
		if i > 0 {
			typesStr += " & "
		}
		if _, isUnion := t.(*UnionType); isUnion {
			typesStr += "(" + t.String() + ")"
			continue
		}
		typesStr += t.String()
	}
	return typesStr
}
func (it *IntersectionType) typeNode() {}
func (it *IntersectionType) Equals(other Type) bool {
	return it.equalsIn(other, nil)
}

func (it *IntersectionType) equalsIn(other Type, s *equalsState) bool {
	otherIt, ok := other.(*IntersectionType)
	if !ok {
		return false // Not an IntersectionType
	}
	if it == nil || otherIt == nil {
		return it == otherIt // Both must be nil or non-nil
	}

	// Intersections are equal if they contain the same set of unique types, regardless of order.
	if len(it.Types) != len(otherIt.Types) {
		return false // Must have the same number of unique constituent types
	}

	// Create boolean maps to track matches
	matched1 := make([]bool, len(it.Types))
	matched2 := make([]bool, len(otherIt.Types))

	for i, t1 := range it.Types {
		foundMatch := false
		for j, t2 := range otherIt.Types {
			if !matched2[j] && equalsIn(t1, t2, s) {
				matched1[i] = true
				matched2[j] = true
				foundMatch = true
				break
			}
		}
		if !foundMatch {
			return false // Type t1 from first intersection not found in second
		}
	}

	// We only need to check one way because we already verified lengths are equal.
	return true
}

// --- Intersection Type Constructor ---

// NewIntersectionType creates a new intersection type from the given types.
// It flattens nested intersections and handles simplifications.
func NewIntersectionType(ts ...Type) Type {
	// Use a slice to collect potential members after flattening
	potentialMembers := make([]Type, 0, len(ts))

	var collectTypes func(t Type)
	collectTypes = func(t Type) {
		if t == nil {
			return
		}
		if intersection, ok := t.(*IntersectionType); ok {
			// Flatten nested intersections
			for _, member := range intersection.Types {
				collectTypes(member)
			}
		} else {
			potentialMembers = append(potentialMembers, t)
		}
	}

	// Collect and flatten all input types
	for _, t := range ts {
		collectTypes(t)
	}

	if d := distributePrimitiveUnions(potentialMembers); d != nil {
		return d
	}

	// Filter for unique types using the Equals method
	uniqueMembers := make([]Type, 0, len(potentialMembers))
	for _, pm := range potentialMembers {
		isDuplicate := false
		for _, um := range uniqueMembers {
			if pm.Equals(um) {
				isDuplicate = true
				break
			}
		}
		if !isDuplicate {
			uniqueMembers = append(uniqueMembers, pm)
		}
	}

	// unknown & T is T.
	if len(uniqueMembers) > 1 {
		known := uniqueMembers[:0:0]
		for _, m := range uniqueMembers {
			if m != Unknown {
				known = append(known, m)
			}
		}
		if len(known) == 0 {
			return Unknown
		}
		uniqueMembers = known
	}
	// A literal absorbs its own primitive: "foo" & string is "foo".
	if len(uniqueMembers) > 1 {
		literalKinds := map[string]bool{}
		for _, m := range uniqueMembers {
			if _, isLit := m.(*LiteralType); isLit {
				literalKinds[primitiveKind(m)] = true
			}
		}
		if len(literalKinds) > 0 {
			kept := uniqueMembers[:0:0]
			for _, m := range uniqueMembers {
				if _, isLit := m.(*LiteralType); !isLit && literalKinds[primitiveKind(m)] && primitiveKind(m) != "" {
					continue
				}
				kept = append(kept, m)
			}
			uniqueMembers = kept
		}
	}
	// Disjoint primitives are never: "a" & "b", string & number, "a" & number.
	for i, a := range uniqueMembers {
		for _, b := range uniqueMembers[i+1:] {
			if disjointPrimitives(a, b) {
				return Never
			}
		}
	}

	// Handle simplification rules
	if len(uniqueMembers) == 0 {
		// Empty intersection (should not happen normally)
		return Any
	} else if len(uniqueMembers) == 1 {
		// If only one unique type remains, return it directly
		return uniqueMembers[0]
	}

	// Check for any & T = any (any absorbs everything in intersections)
	for _, member := range uniqueMembers {
		if member == Any {
			return Any
		}
	}

	// Check for never & T = never (never propagates in intersections)
	for _, member := range uniqueMembers {
		if member == Never {
			return Never
		}
	}

	// TODO: Add more sophisticated conflict detection for incompatible types
	// For now, let the type checker handle conflicts during assignability checks

	// Members keep their written order, as tsc's do: the order of an
	// intersection's call signatures is its overload order. (Sorting them
	// by String() printed csstype's huge types over and over.)
	return &IntersectionType{Types: uniqueMembers}
}

// primitiveKind is the primitive a type is or belongs to ("string" for string
// and string literals), or "" for anything else.
func primitiveKind(t Type) string {
	switch t {
	case String:
		return "string"
	case Number:
		return "number"
	case Boolean:
		return "boolean"
	case BigInt:
		return "bigint"
	case Symbol:
		return "symbol"
	case Null:
		return "null"
	case Undefined:
		return "undefined"
	}
	if lit, ok := t.(*LiteralType); ok {
		switch lit.Value.Type() {
		case vm.TypeString:
			return "string"
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			return "number"
		case vm.TypeBoolean:
			return "boolean"
		case vm.TypeBigInt:
			return "bigint"
		}
	}
	return ""
}

// disjointPrimitives reports whether a & b is empty because both are
// primitives (or literals) with no value in common.
func disjointPrimitives(a, b Type) bool {
	ka, kb := primitiveKind(a), primitiveKind(b)
	if ka == "" || kb == "" {
		return false
	}
	if ka != kb {
		return true
	}
	la, aLit := a.(*LiteralType)
	lb, bLit := b.(*LiteralType)
	return aLit && bLit && !la.Equals(lb)
}

// distributePrimitiveUnions is the union an intersection of primitive unions
// reduces to, distributed as tsc does: (number | string | undefined) &
// (number | string) is number | string. nil when some member is not a
// primitive, a literal or a union of them, when no member is a union, or
// when the cross product would be large.
func distributePrimitiveUnions(members []Type) Type {
	const maxCombinations = 64
	hasUnion := false
	combinations := 1
	for _, m := range members {
		if u, ok := m.(*UnionType); ok {
			for _, um := range u.Types {
				if primitiveKind(um) == "" {
					return nil
				}
			}
			hasUnion = true
			combinations *= len(u.Types)
			if combinations > maxCombinations {
				return nil
			}
		} else if primitiveKind(m) == "" {
			return nil
		}
	}
	if !hasUnion || len(members) < 2 {
		return nil
	}
	var results []Type
	var walk func(i int, picked []Type)
	walk = func(i int, picked []Type) {
		if i == len(members) {
			if r := NewIntersectionType(picked...); r != Never {
				results = append(results, r)
			}
			return
		}
		if u, ok := members[i].(*UnionType); ok {
			for _, um := range u.Types {
				walk(i+1, append(picked, um))
			}
			return
		}
		walk(i+1, append(picked, members[i]))
	}
	walk(0, make([]Type, 0, len(members)))
	if len(results) == 0 {
		return Never
	}
	return NewUnionType(results...)
}

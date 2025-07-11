package types

import (
	"fmt"
	"strings"
)

// EnumType represents an enum type (e.g., Color with members Red, Green, Blue)
type EnumType struct {
	Name      string
	Members   map[string]*EnumMemberType // Map of member name to member type
	IsConst   bool                       // True for const enums
	IsNumeric bool                       // True if all members are numeric
}

// EnumMemberType represents a specific enum member literal type (e.g., Color.Red)
type EnumMemberType struct {
	EnumName   string      // Parent enum name
	MemberName string      // Member name
	Value      interface{} // Runtime value (int or string)
}

// String returns the string representation of the enum type
func (e *EnumType) String() string {
	if e.IsConst {
		return fmt.Sprintf("const enum %s", e.Name)
	}
	return fmt.Sprintf("enum %s", e.Name)
}

// TypeString returns the type string representation
func (e *EnumType) TypeString() string {
	// For type contexts, enum type is the union of all its members
	var memberTypes []string
	for _, member := range e.Members {
		memberTypes = append(memberTypes, member.String())
	}
	return strings.Join(memberTypes, " | ")
}

// GetName returns the enum name
func (e *EnumType) GetName() string {
	return e.Name
}

// String returns the string representation of the enum member type
func (em *EnumMemberType) String() string {
	return fmt.Sprintf("%s.%s", em.EnumName, em.MemberName)
}

// TypeString returns the type string representation
func (em *EnumMemberType) TypeString() string {
	return em.String()
}

// GetName returns the member name (for consistency with other types)
func (em *EnumMemberType) GetName() string {
	return em.MemberName
}

// IsEnumType checks if a type is an EnumType
func IsEnumType(t Type) bool {
	_, ok := t.(*EnumType)
	return ok
}

// IsEnumMemberType checks if a type is an EnumMemberType
func IsEnumMemberType(t Type) bool {
	_, ok := t.(*EnumMemberType)
	return ok
}

// GetEnumMemberValue returns the value of an enum member if it's an enum member type
func GetEnumMemberValue(t Type) (interface{}, bool) {
	if em, ok := t.(*EnumMemberType); ok {
		return em.Value, true
	}
	return nil, false
}

// typeNode implements the Type interface marker method
func (e *EnumType) typeNode() {}

// Equals checks if this enum type equals another type
func (e *EnumType) Equals(other Type) bool {
	if other == nil {
		return false
	}
	o, ok := other.(*EnumType)
	if !ok {
		return false
	}
	return e.Name == o.Name && e.IsConst == o.IsConst
}

// typeNode implements the Type interface marker method
func (em *EnumMemberType) typeNode() {}

// Equals checks if this enum member type equals another type
func (em *EnumMemberType) Equals(other Type) bool {
	if other == nil {
		return false
	}
	o, ok := other.(*EnumMemberType)
	if !ok {
		return false
	}
	return em.EnumName == o.EnumName && em.MemberName == o.MemberName
}
package types

import (
	"fmt"
	"strings"
)

// NamespaceType represents a TypeScript namespace as a type-level entity. It carries:
//
//   - ValueShape: the runtime ObjectType describing the namespace's exported VALUE
//     bindings (var/let/const/function/class/enum). This is also the type bound to
//     the namespace name in the VALUE environment, so ordinary `N.x` member access
//     resolves through normal ObjectType property lookup.
//
//   - TypeMembers: exported TYPE bindings (interfaces, type aliases, nested
//     namespaces, enum types). These are consulted when resolving namespace-
//     qualified type names like `N.I` in type position.
//
// A NamespaceType is itself bound in the TYPE environment for the namespace name,
// so type-name resolution sees it. Multiple `namespace N { ... }` declarations in
// the same scope MERGE into the same NamespaceType.
type NamespaceType struct {
	Name        string
	ValueShape  *ObjectType
	TypeMembers map[string]Type
	// Declare is true if this namespace was introduced by `declare namespace ...`
	// (no runtime emission, type-level only).
	Declare bool
}

// NewNamespaceType creates an empty namespace type.
func NewNamespaceType(name string) *NamespaceType {
	return &NamespaceType{
		Name:        name,
		ValueShape:  NewObjectType(),
		TypeMembers: make(map[string]Type),
	}
}

func (n *NamespaceType) typeNode() {}

func (n *NamespaceType) String() string {
	parts := []string{}
	if n.ValueShape != nil {
		for k := range n.ValueShape.Properties {
			parts = append(parts, k)
		}
	}
	for k := range n.TypeMembers {
		parts = append(parts, "type "+k)
	}
	return fmt.Sprintf("namespace %s { %s }", n.Name, strings.Join(parts, "; "))
}

func (n *NamespaceType) Equals(other Type) bool {
	o, ok := other.(*NamespaceType)
	if !ok {
		return false
	}
	return n == o
}

// LookupTypeMember resolves a type-position member like `N.X`. It first checks
// TypeMembers, then falls back to ValueShape properties whose type is itself a
// type-shaped binding (e.g. a class's instance type, an enum). Returns nil if
// not found.
func (n *NamespaceType) LookupTypeMember(name string) Type {
	if t, ok := n.TypeMembers[name]; ok {
		return t
	}
	if n.ValueShape != nil {
		if t, ok := n.ValueShape.Properties[name]; ok {
			// For classes/enums stored as values, return their instance/value type.
			if cls, isClass := t.(*ClassType); isClass {
				return cls.InstanceType
			}
			if _, isEnum := t.(*EnumType); isEnum {
				return t
			}
		}
	}
	return nil
}

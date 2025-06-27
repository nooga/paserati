package types

import "fmt"

// ReadonlyType represents a readonly wrapper around another type
// This allows `readonly foo: number` to unify with `foo: Readonly<number>`
type ReadonlyType struct {
	InnerType Type // The wrapped type that becomes readonly
}

func (r *ReadonlyType) String() string {
	if r.InnerType == nil {
		return "readonly <nil>"
	}
	return fmt.Sprintf("readonly %s", r.InnerType.String())
}

func (r *ReadonlyType) Equals(other Type) bool {
	// readonly T equals readonly U if T equals U
	if otherReadonly, ok := other.(*ReadonlyType); ok {
		if r.InnerType == nil && otherReadonly.InnerType == nil {
			return true
		}
		if r.InnerType == nil || otherReadonly.InnerType == nil {
			return false
		}
		return r.InnerType.Equals(otherReadonly.InnerType)
	}
	return false
}

func (r *ReadonlyType) typeNode() {}

// NewReadonlyType creates a new readonly wrapper around a type
func NewReadonlyType(innerType Type) *ReadonlyType {
	return &ReadonlyType{InnerType: innerType}
}

// GetReadonlyInnerType extracts the inner type from a readonly type
// Returns the type itself if it's not readonly
func GetReadonlyInnerType(t Type) Type {
	if readonly, ok := t.(*ReadonlyType); ok {
		return readonly.InnerType
	}
	return t
}

// IsReadonlyType checks if a type is a readonly type
func IsReadonlyType(t Type) bool {
	_, ok := t.(*ReadonlyType)
	return ok
}
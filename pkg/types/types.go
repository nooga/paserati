package types

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

// ForwardReferenceType represents a forward reference to a generic class being defined
type ForwardReferenceType struct {
	ClassName      string
	TypeParameters []*TypeParameter
}

func (frt *ForwardReferenceType) String() string {
	return frt.ClassName
}

func (frt *ForwardReferenceType) Equals(other Type) bool {
	if otherFrt, ok := other.(*ForwardReferenceType); ok {
		return frt.ClassName == otherFrt.ClassName
	}
	return false
}

func (frt *ForwardReferenceType) typeNode() {}

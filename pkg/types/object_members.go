package types

// Reads of an ObjectType's members go through these accessors, so that a
// type can fill its members in when they are first read: substitution makes
// instantiations whose members are substituted only if something reads them.

// SetLazyMembers makes fill fill in ot's members when one is first read or
// written. fill runs once.
func (ot *ObjectType) SetLazyMembers(fill func(*ObjectType)) { ot.lazy = fill }

// Materialize fills in members not filled in yet. Accessors call it; a plain
// write to a member field calls it first.
func (ot *ObjectType) Materialize() {
	if fill := ot.lazy; fill != nil {
		ot.lazy = nil
		fill(ot)
	}
}

// Props returns the own properties by name.
func (ot *ObjectType) Props() map[string]Type {
	if ot.lazy != nil {
		ot.Materialize()
	}
	return ot.Properties
}

// OptionalProps returns which own properties are optional.
func (ot *ObjectType) OptionalProps() map[string]bool {
	if ot.lazy != nil {
		ot.Materialize()
	}
	return ot.OptionalProperties
}

// ReadOnlyProps returns which own properties are readonly.
func (ot *ObjectType) ReadOnlyProps() map[string]bool {
	if ot.lazy != nil {
		ot.Materialize()
	}
	return ot.ReadOnlyProperties
}

// PropOrder returns the order own properties were first added in.
func (ot *ObjectType) PropOrder() []string {
	if ot.lazy != nil {
		ot.Materialize()
	}
	return ot.PropertyOrder
}

// CallSigs returns the call signatures.
func (ot *ObjectType) CallSigs() []*Signature {
	if ot.lazy != nil {
		ot.Materialize()
	}
	return ot.CallSignatures
}

// ConstructSigs returns the construct signatures.
func (ot *ObjectType) ConstructSigs() []*Signature {
	if ot.lazy != nil {
		ot.Materialize()
	}
	return ot.ConstructSignatures
}

// IndexSigs returns the index signatures.
func (ot *ObjectType) IndexSigs() []*IndexSignature {
	if ot.lazy != nil {
		ot.Materialize()
	}
	return ot.IndexSignatures
}

// Bases returns the base types (heritage of a class or interface).
func (ot *ObjectType) Bases() []Type {
	if ot.lazy != nil {
		ot.Materialize()
	}
	return ot.BaseTypes
}

package types

// Reads of an ObjectType's members go through these accessors, so that a
// type can fill its members in when they are first read.

// Props returns the own properties by name.
func (ot *ObjectType) Props() map[string]Type { return ot.Properties }

// OptionalProps returns which own properties are optional.
func (ot *ObjectType) OptionalProps() map[string]bool { return ot.OptionalProperties }

// ReadOnlyProps returns which own properties are readonly.
func (ot *ObjectType) ReadOnlyProps() map[string]bool { return ot.ReadOnlyProperties }

// PropOrder returns the order own properties were first added in.
func (ot *ObjectType) PropOrder() []string { return ot.PropertyOrder }

// CallSigs returns the call signatures.
func (ot *ObjectType) CallSigs() []*Signature { return ot.CallSignatures }

// ConstructSigs returns the construct signatures.
func (ot *ObjectType) ConstructSigs() []*Signature { return ot.ConstructSignatures }

// IndexSigs returns the index signatures.
func (ot *ObjectType) IndexSigs() []*IndexSignature { return ot.IndexSignatures }

// Bases returns the base types (heritage of a class or interface).
func (ot *ObjectType) Bases() []Type { return ot.BaseTypes }

// Materialize fills in members not filled in yet, before a write to one.
func (ot *ObjectType) Materialize() {}

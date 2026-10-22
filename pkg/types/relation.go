package types

// Relation is one checker's view of the type relations (assignability,
// comparability, widening, apparent members): the compiler options they
// depend on, the hooks into that checker's built-ins, and the state that
// bounds their recursion. Every checker has its own, so independent checkers
// can run on separate goroutines (#606) with their own options. A Relation is
// used by one goroutine at a time.
type Relation struct {
	// StrictNullChecks mirrors --strictNullChecks. When false null and
	// undefined are assignable to every type and widen to any, as in
	// TypeScript without the flag.
	StrictNullChecks bool

	// StrictFunctionTypes mirrors --strictFunctionTypes: when set, parameters
	// of signatures declared as function types (not methods) are compared
	// contravariantly instead of bivariantly.
	StrictFunctionTypes bool

	// PrimitivePrototypeMembers supplies the members of a primitive's wrapper
	// prototype ("string", "number", "boolean", "bigint", "array").
	PrimitivePrototypeMembers func(kind string) map[string]Type

	// PrototypeMethods resolves a method on a primitive's prototype
	// ("string", "number", "array", "object", ...).
	PrototypeMethods GetMethodType

	// ExpandMapped expands a mapped type over concrete keys (Partial<Base>)
	// to the object type it maps to, or returns it as it is. The checker
	// supplies it; a mapped type nested in a member (`{ e: Partial<Base> }`)
	// reaches the relation unexpanded.
	ExpandMapped func(Type) Type

	// ApparentMapped is tsc's getResolvedApparentTypeOfMappedType: a
	// homomorphic mapped type over a type parameter constrained to arrays or
	// tuples is related as its mapping of that constraint (or nil). The
	// checker supplies it.
	ApparentMapped func(*MappedType) Type
	// IndexedConstraint is the base constraint of a generic indexed access
	// (TRouter["_def"]["errorShape"] through TRouter's constraint), or nil:
	// an indexed access no other rule relates is related through it.
	IndexedConstraint func(*IndexedAccessType) Type

	// ConditionalConstraint is what a deferred conditional type is known to
	// be (tsc's default constraint of a conditional type: its branches'
	// constraints), or nil when nothing is known. The checker supplies it.
	ConditionalConstraint func(*ConditionalType) Type

	// ResolveClassRef resolves a reference a generic class makes to itself
	// or another class in its own body (List<List<T>>) to the instance type
	// it names. The checker supplies it.
	ResolveClassRef func(*ParameterizedForwardReferenceType) Type

	// ResolveTypeof resolves a `typeof x` query written before x was typed
	// (a hoisted function's annotation), or returns it unchanged. The
	// checker supplies it.
	ResolveTypeof func(*TypeofType) Type

	// maybe holds, like tsc's maybeKeys, the (source, target) pairs
	// isAssignable is relating or has related within the current top-level
	// query, with their index in maybeStack. A pair in it relates by
	// assumption: a recursive type relates to itself, and a pair met again
	// is not related again. A pair that fails drops everything pushed after
	// it, since that may have assumed it; the query's end clears it all.
	maybe      map[assignPair]int
	maybeStack []assignPair
	depth      int
	// cutOff records that a nesting cut-off (maxRelatedNesting) assumed a
	// relation inside the pair being related, which is then not kept.
	cutOff bool

	// relatingInstances counts, per generic, the instantiations of it that
	// objectAssignable is relating (source and target sides together).
	relatingInstances map[*GenericType]int

	// expandingInstances counts, per generic, the lazily instantiated
	// references to it (InstantiatedType) isAssignable is expanding.
	expandingInstances map[*GenericType]int
}

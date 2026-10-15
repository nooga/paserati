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

	// visited holds the (source, target) pairs isAssignable is relating, so a
	// recursive type relates to itself by assumption.
	visited map[assignPair]bool

	// relatingInstances counts, per generic, the instantiations of it that
	// objectAssignable is relating (source and target sides together).
	relatingInstances map[*GenericType]int
}

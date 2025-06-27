package builtins

type UtilityTypesInitializer struct{}

func (u *UtilityTypesInitializer) Name() string {
	return "UtilityTypes"
}

func (u *UtilityTypesInitializer) Priority() int {
	return PriorityObject - 1 // Initialize before Object so other types can use utility types
}

func (u *UtilityTypesInitializer) InitTypes(ctx *TypeContext) error {
	// Utility types like Readonly<T> are handled directly in the type checker
	// (see pkg/checker/resolve.go) as special cases like Array<T> and Promise<T>
	// This is because they need to be available as type constructors, not values
	return nil
}

func (u *UtilityTypesInitializer) InitRuntime(ctx *RuntimeContext) error {
	// Utility types are compile-time only, no runtime representation needed
	return nil
}
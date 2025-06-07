package checker

import (
	"paserati/pkg/builtins"
	"paserati/pkg/types"
)

// Initialize the prototype method resolver
func init() {
	// Set the prototype method resolver in the types package
	types.SetPrototypeMethodResolver(builtins.GetPrototypeMethodType)
}

// getBuiltinType looks up a builtin type by name (e.g., "String.fromCharCode")
func (c *Checker) getBuiltinType(name string) types.Type {
	return builtins.GetType(name)
}

// getPropertyTypeFromType returns the type of a property access on the given type
// isOptionalChaining determines whether to be permissive about missing properties
// This is now a wrapper around the implementation in the types package.
func (c *Checker) getPropertyTypeFromType(objectType types.Type, propertyName string, isOptionalChaining bool) types.Type {
	return types.GetPropertyType(objectType, propertyName, isOptionalChaining)
}

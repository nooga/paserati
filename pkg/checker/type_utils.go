package checker

import (
	"paserati/pkg/types"
)

// Initialize the prototype method resolver
func init() {
	// Set the prototype method resolver to use our new environment-based approach
	types.SetPrototypeMethodResolver(getPrototypeMethodTypeFromGlobalEnv)
}

// getPrototypeMethodTypeFromGlobalEnv is the new prototype method resolver
// that uses the environment's primitive prototype registry
func getPrototypeMethodTypeFromGlobalEnv(primitiveName, methodName string) types.Type {
	if globalEnvironment != nil {
		return globalEnvironment.GetPrimitivePrototypeMethodType(primitiveName, methodName)
	}
	return nil
}

// getBuiltinType looks up a builtin type by name in the global environment
func (c *Checker) getBuiltinType(name string) types.Type {
	if typ, _, found := c.env.Resolve(name); found {
		return typ
	}
	return nil
}


// getPropertyTypeFromType returns the type of a property access on the given type
// isOptionalChaining determines whether to be permissive about missing properties
// This is now a wrapper around the implementation in the types package.
func (c *Checker) getPropertyTypeFromType(objectType types.Type, propertyName string, isOptionalChaining bool) types.Type {
	return types.GetPropertyType(objectType, propertyName, isOptionalChaining)
}

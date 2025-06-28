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

// validateIndexSignatures checks if a source object type satisfies the index signature constraints of a target type
// This is used when assigning object literals to types with index signatures
func (c *Checker) validateIndexSignatures(sourceType, targetType types.Type) []IndexSignatureError {
	var errors []IndexSignatureError
	
	sourceObj, sourceIsObj := sourceType.(*types.ObjectType)
	targetObj, targetIsObj := targetType.(*types.ObjectType)
	
	// Only validate if both are object types and target has index signatures
	if !sourceIsObj || !targetIsObj || len(targetObj.IndexSignatures) == 0 {
		return errors
	}
	
	// Check each property in source against all index signatures in target
	for propName, propType := range sourceObj.Properties {
		errors = append(errors, c.validatePropertyAgainstIndexSignatures(propName, propType, targetObj.IndexSignatures)...)
	}
	
	return errors
}

// validatePropertyAgainstIndexSignatures checks if a single property satisfies index signature constraints
func (c *Checker) validatePropertyAgainstIndexSignatures(propName string, propType types.Type, indexSignatures []*types.IndexSignature) []IndexSignatureError {
	var errors []IndexSignatureError
	
	for _, indexSig := range indexSignatures {
		if c.propertyMatchesIndexSignature(propName, indexSig) {
			// Property matches this index signature's key pattern, validate value type
			if !types.IsAssignable(propType, indexSig.ValueType) {
				errors = append(errors, IndexSignatureError{
					PropertyName: propName,
					PropertyType: propType,
					ExpectedType: indexSig.ValueType,
					KeyType:      indexSig.KeyType,
				})
			}
		}
	}
	
	return errors
}

// propertyMatchesIndexSignature determines if a property name matches an index signature's key type
func (c *Checker) propertyMatchesIndexSignature(propName string, indexSig *types.IndexSignature) bool {
	// For now, we only support string and number key types
	switch indexSig.KeyType {
	case types.String:
		// All string property names match string index signatures
		return true
	case types.Number:
		// Only numeric property names match number index signatures
		// In JavaScript, array indices and numeric properties are treated as numbers
		// For simplicity, we'll check if the property name looks numeric
		for _, char := range propName {
			if char < '0' || char > '9' {
				return false
			}
		}
		return len(propName) > 0
	default:
		// For other key types (like union types), we'd need more sophisticated matching
		return false
	}
}

// IndexSignatureError represents an error when a property doesn't match index signature constraints
type IndexSignatureError struct {
	PropertyName string
	PropertyType types.Type
	ExpectedType types.Type
	KeyType      types.Type
}

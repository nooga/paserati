package types

import (
	"fmt"
	"os"
)

// Debug flag for property type resolution
const propsDebug = false

func propsDebugPrintf(format string, args ...interface{}) {
	if propsDebug {
		fmt.Fprintf(os.Stderr, "[PROPS DEBUG] "+format, args...)
	}
}

// --- Property Type Access ---

// GetMethodType is a function type for resolving prototype methods
// This type is used to decouple the types package from builtins
type GetMethodType func(objectType string, methodName string) Type

// prototypeMethodResolver is used to resolve prototype methods
// This will be set by the checker package to avoid circular imports
var prototypeMethodResolver GetMethodType

// SetPrototypeMethodResolver sets the resolver for prototype methods
// This should be called by the checker package during initialization
func SetPrototypeMethodResolver(resolver GetMethodType) {
	prototypeMethodResolver = resolver
}

// GetPropertyType returns the type of a property access on the given type
// isOptionalChaining determines whether to be permissive about missing properties
func GetPropertyType(objectType Type, propertyName string, isOptionalChaining bool) Type {
	// Widen the object type for checks
	widenedObjectType := GetWidenedType(objectType)
	propsDebugPrintf("GetPropertyType: objectType=%T (%s), propertyName=%s, widened=%T (%s)\n",
		objectType, objectType.String(), propertyName, widenedObjectType, widenedObjectType.String())

	if widenedObjectType == Any {
		return Any // Property access on 'any' results in 'any'
	} else if widenedObjectType == Null || widenedObjectType == Undefined {
		return Undefined
	} else if widenedObjectType == String {
		if propertyName == "length" {
			return Number // string.length is number
		} else {
			// Check prototype registry for String methods
			if prototypeMethodResolver != nil {
				if methodType := prototypeMethodResolver("string", propertyName); methodType != nil {
					return methodType
				}
			}

			if !isOptionalChaining {
				// Only add error for regular member access, not optional chaining
				// Note: We can't add error here since we don't have the node, but that's ok for union handling
			}
			return Never
		}
	} else {
		// Use a type switch for struct-based types
		switch obj := widenedObjectType.(type) {
		case *ArrayType:
			if propertyName == "length" {
				return Number // Array.length is number
			} else {
				// Check prototype registry for Array methods
				if prototypeMethodResolver != nil {
					if methodType := prototypeMethodResolver("array", propertyName); methodType != nil {
						return methodType
					}
				}
				return Never
			}
		case *ObjectType:
			// Look for the property in the object's fields
			fieldType, exists := obj.Properties[propertyName]
			if exists {
				// Property found
				if fieldType == nil { // Should ideally not happen if checker populates correctly
					return Never
				} else {
					return fieldType
				}
			} else {
				// Property not found in object's own properties, check Object.prototype
				if prototypeMethodResolver != nil {
					if prototypeMethodType := prototypeMethodResolver("object", propertyName); prototypeMethodType != nil {
						return prototypeMethodType
					}
				}
				
				// Property not found
				if isOptionalChaining {
					return Undefined
				} else {
					return Never
				}
			}
		// Legacy FunctionType and CallableType cases removed - use ObjectType with CallSignatures instead
		case *IntersectionType:
			// Handle property access on intersection types
			return GetPropertyTypeFromIntersection(obj, propertyName)
		case *InstantiatedType:
			// Handle property access on instantiated generic types (e.g., Iterator<number>)
			// Substitute the type parameters and recursively get the property
			substituted := obj.Substitute()
			return GetPropertyType(substituted, propertyName, isOptionalChaining)
		default:
			// This covers cases where widenedObjectType was not String, Any, ArrayType, ObjectType, etc.
			return Never
		}
	}
}

// GetPropertyTypeFromIntersection returns the type of a property accessed on an intersection type.
// The property exists if it exists on ANY of the constituent types.
// The resulting type is the intersection of the property types from all types that have it.
func GetPropertyTypeFromIntersection(intersection *IntersectionType, propertyName string) Type {
	var resultTypes []Type

	for _, member := range intersection.Types {
		propType := GetPropertyType(member, propertyName, false)
		if propType != Never {
			// Property exists on this member - collect its type
			resultTypes = append(resultTypes, propType)
		}
		// If property doesn't exist on this member, that's okay for intersections
	}

	if len(resultTypes) == 0 {
		// Property doesn't exist on any member
		return Never
	}

	// If only one member has the property, return that type
	if len(resultTypes) == 1 {
		return resultTypes[0]
	}

	// Check if all types are the same
	firstType := resultTypes[0]
	allSame := true
	for _, t := range resultTypes[1:] {
		if !firstType.Equals(t) {
			allSame = false
			break
		}
	}

	if allSame {
		return firstType
	}

	// Create intersection of property types from all members that have it
	return NewIntersectionType(resultTypes...)
}

// --- Intersection Type Helpers ---

// SimplifyIntersectionWithObjects attempts to merge object types in an intersection.
// This is one of the most complex parts of intersection type handling.
func SimplifyIntersectionWithObjects(intersection *IntersectionType) Type {
	var objectTypes []*ObjectType
	var otherTypes []Type

	// Separate object types from other types
	for _, member := range intersection.Types {
		if objType, isObj := member.(*ObjectType); isObj {
			objectTypes = append(objectTypes, objType)
		} else {
			otherTypes = append(otherTypes, member)
		}
	}

	// If we have object types, merge them
	if len(objectTypes) > 0 {
		mergedObject := MergeObjectTypes(objectTypes)
		if mergedObject != nil {
			if len(otherTypes) == 0 {
				// Only object types - return the merged object
				return mergedObject
			} else {
				// Add the merged object back to other types
				allTypes := append(otherTypes, mergedObject)
				return NewIntersectionType(allTypes...)
			}
		}
	}

	// No simplification possible, return original intersection
	return intersection
}

// MergeObjectTypes merges multiple object types into a single object type.
// This handles property conflicts and optional properties.
func MergeObjectTypes(objectTypes []*ObjectType) *ObjectType {
	if len(objectTypes) == 0 {
		return nil
	}

	if len(objectTypes) == 1 {
		return objectTypes[0]
	}

	// Merge all properties
	mergedProperties := make(map[string]Type)
	mergedOptional := make(map[string]bool)

	for _, objType := range objectTypes {
		for propName, propType := range objType.Properties {
			if existingType, exists := mergedProperties[propName]; exists {
				// Property exists in multiple objects - intersect the types
				mergedProperties[propName] = NewIntersectionType(existingType, propType)
			} else {
				mergedProperties[propName] = propType
			}

			// Property is optional only if it's optional in ALL objects that have it
			isOptionalInThis := objType.OptionalProperties != nil && objType.OptionalProperties[propName]
			if !isOptionalInThis {
				// Property is required in this object, so it's required in the intersection
				mergedOptional[propName] = false
			} else if _, alreadySet := mergedOptional[propName]; !alreadySet {
				// First time seeing this property, and it's optional
				mergedOptional[propName] = true
			}
		}
	}

	return &ObjectType{
		Properties:         mergedProperties,
		OptionalProperties: mergedOptional,
	}
}

// ObjectTypeIsCallable checks if an object type has call signatures
func ObjectTypeIsCallable(objType *ObjectType) bool {
	// For now, this is a placeholder - will be implemented in the unified type system
	return false
}

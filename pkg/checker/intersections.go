package checker

import (
	"paserati/pkg/types"
)

// isAssignableToIntersection checks if a source type can be assigned to an intersection type.
// For assignment to succeed, the source must be assignable to ALL types in the intersection.
func (c *Checker) isAssignableToIntersection(source types.Type, intersection *types.IntersectionType) bool {
	// Source must be assignable to ALL members of the intersection
	for _, member := range intersection.Types {
		if !c.isAssignable(source, member) {
			return false
		}
	}
	return true
}

// isIntersectionAssignableTo checks if an intersection type can be assigned to a target type.
// For assignment to succeed, AT LEAST ONE type in the intersection must be assignable to target.
func (c *Checker) isIntersectionAssignableTo(intersection *types.IntersectionType, target types.Type) bool {
	// At least one member of the intersection must be assignable to target
	for _, member := range intersection.Types {
		if c.isAssignable(member, target) {
			return true
		}
	}
	return false
}

// getPropertyTypeFromIntersection returns the type of a property accessed on an intersection type.
// The property exists if it exists on ANY of the constituent types.
// The resulting type is the intersection of the property types from all types that have it.
func (c *Checker) getPropertyTypeFromIntersection(intersection *types.IntersectionType, propertyName string) types.Type {
	var resultTypes []types.Type

	for _, member := range intersection.Types {
		propType := c.getPropertyTypeFromType(member, propertyName, false)
		if propType != types.Never {
			// Property exists on this member - collect its type
			resultTypes = append(resultTypes, propType)
		}
		// If property doesn't exist on this member, that's okay for intersections
	}

	if len(resultTypes) == 0 {
		// Property doesn't exist on any member
		return types.Never
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
	return types.NewIntersectionType(resultTypes...)
}

// intersectionHasCallSignature checks if an intersection type can be called.
// It returns true if any member of the intersection is callable.
func (c *Checker) intersectionHasCallSignature(intersection *types.IntersectionType) (*types.FunctionType, bool) {
	for _, member := range intersection.Types {
		switch mt := member.(type) {
		case *types.FunctionType:
			return mt, true
		case *types.CallableType:
			return mt.CallSignature, true
		case *types.OverloadedFunctionType:
			return mt.Implementation, true
		}
	}
	return nil, false
}

// simplifyIntersectionWithObjects attempts to merge object types in an intersection.
// This is one of the most complex parts of intersection type handling.
func (c *Checker) simplifyIntersectionWithObjects(intersection *types.IntersectionType) types.Type {
	var objectTypes []*types.ObjectType
	var otherTypes []types.Type

	// Separate object types from other types
	for _, member := range intersection.Types {
		if objType, isObj := member.(*types.ObjectType); isObj {
			objectTypes = append(objectTypes, objType)
		} else {
			otherTypes = append(otherTypes, member)
		}
	}

	// If we have object types, merge them
	if len(objectTypes) > 0 {
		mergedObject := c.mergeObjectTypes(objectTypes)
		if mergedObject != nil {
			if len(otherTypes) == 0 {
				// Only object types - return the merged object
				return mergedObject
			} else {
				// Add the merged object back to other types
				allTypes := append(otherTypes, mergedObject)
				return types.NewIntersectionType(allTypes...)
			}
		}
	}

	// No simplification possible, return original intersection
	return intersection
}

// mergeObjectTypes merges multiple object types into a single object type.
// This handles property conflicts and optional properties.
func (c *Checker) mergeObjectTypes(objectTypes []*types.ObjectType) *types.ObjectType {
	if len(objectTypes) == 0 {
		return nil
	}

	if len(objectTypes) == 1 {
		return objectTypes[0]
	}

	// Merge all properties
	mergedProperties := make(map[string]types.Type)
	mergedOptional := make(map[string]bool)

	for _, objType := range objectTypes {
		for propName, propType := range objType.Properties {
			if existingType, exists := mergedProperties[propName]; exists {
				// Property exists in multiple objects - intersect the types
				mergedProperties[propName] = types.NewIntersectionType(existingType, propType)
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

	return &types.ObjectType{
		Properties:         mergedProperties,
		OptionalProperties: mergedOptional,
	}
}

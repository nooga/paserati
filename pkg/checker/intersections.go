package checker

import (
	"paserati/pkg/types"
)

// isAssignableToIntersection checks if a source type can be assigned to an intersection type.
// For assignment to succeed, the source must be assignable to ALL types in the intersection.
// This is now just a wrapper around the implementation in the types package.
func (c *Checker) isAssignableToIntersection(source types.Type, intersection *types.IntersectionType) bool {
	// Source must be assignable to ALL members of the intersection
	for _, member := range intersection.Types {
		if !types.IsAssignable(source, member) {
			return false
		}
	}
	return true
}

// isIntersectionAssignableTo checks if an intersection type can be assigned to a target type.
// For assignment to succeed, AT LEAST ONE type in the intersection must be assignable to target.
// This is now just a wrapper around the implementation in the types package.
func (c *Checker) isIntersectionAssignableTo(intersection *types.IntersectionType, target types.Type) bool {
	// At least one member of the intersection must be assignable to target
	for _, member := range intersection.Types {
		if types.IsAssignable(member, target) {
			return true
		}
	}
	return false
}

// getPropertyTypeFromIntersection returns the type of a property accessed on an intersection type.
// This is now a wrapper around the implementation in the types package.
func (c *Checker) getPropertyTypeFromIntersection(intersection *types.IntersectionType, propertyName string) types.Type {
	return types.GetPropertyTypeFromIntersection(intersection, propertyName)
}

// intersectionHasCallSignature checks if an intersection type can be called.
// This is now a wrapper around the implementation in the types package.
// func (c *Checker) intersectionHasCallSignature(intersection *types.IntersectionType) (*types.FunctionType, bool) {
// 	return types.IntersectionHasCallSignature(intersection)
// }

// simplifyIntersectionWithObjects attempts to merge object types in an intersection.
// This is now a wrapper around the implementation in the types package.
func (c *Checker) simplifyIntersectionWithObjects(intersection *types.IntersectionType) types.Type {
	return types.SimplifyIntersectionWithObjects(intersection)
}

// mergeObjectTypes merges multiple object types into a single object type.
// This is now a wrapper around the implementation in the types package.
func (c *Checker) mergeObjectTypes(objectTypes []*types.ObjectType) *types.ObjectType {
	return types.MergeObjectTypes(objectTypes)
}

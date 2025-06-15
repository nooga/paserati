package types

import (
	"paserati/pkg/vm"
)

// --- Type Assignability ---

// IsAssignable checks if a value of type `source` can be assigned to a variable
// of type `target`. This is moved from checker to types package for clean separation.
func IsAssignable(source, target Type) bool {
	if source == nil || target == nil {
		return false
	}

	// Basic rules:
	if target == Any || source == Any {
		return true
	}

	if target == Unknown {
		return true
	}
	if source == Unknown {
		return target == Unknown
	}

	if source == Never {
		return true
	}

	// Check for identical types
	if source == target {
		return true
	}

	// Check using type-specific Equals method for complex types
	if source.Equals(target) {
		return true
	}

	// Union type handling
	sourceUnion, sourceIsUnion := source.(*UnionType)
	targetUnion, targetIsUnion := target.(*UnionType)

	if targetIsUnion {
		if sourceIsUnion {
			// Union to union: every type in source must be assignable to at least one in target
			for _, sType := range sourceUnion.Types {
				assignable := false
				for _, tType := range targetUnion.Types {
					if IsAssignable(sType, tType) {
						assignable = true
						break
					}
				}
				if !assignable {
					return false
				}
			}
			return true
		} else {
			// Non-union to union: source must be assignable to at least one type in target
			for _, tType := range targetUnion.Types {
				if IsAssignable(source, tType) {
					return true
				}
			}
			return false
		}
	} else if sourceIsUnion {
		// Union to non-union: every type in source must be assignable to target
		for _, sType := range sourceUnion.Types {
			if !IsAssignable(sType, target) {
				return false
			}
		}
		return true
	}

	// Intersection type handling
	sourceIntersection, sourceIsIntersection := source.(*IntersectionType)
	targetIntersection, targetIsIntersection := target.(*IntersectionType)

	if targetIsIntersection {
		// Source must be assignable to ALL types in target intersection
		for _, tType := range targetIntersection.Types {
			if !IsAssignable(source, tType) {
				return false
			}
		}
		return true
	} else if sourceIsIntersection {
		// At least one type in source intersection must be assignable to target
		for _, sType := range sourceIntersection.Types {
			if IsAssignable(sType, target) {
				return true
			}
		}
		return false
	}

	// Literal type handling
	sourceLiteral, sourceIsLiteral := source.(*LiteralType)
	targetLiteral, targetIsLiteral := target.(*LiteralType)

	if sourceIsLiteral && targetIsLiteral {
		// Both literals: values must be equal
		if sourceLiteral.Value.Type() != targetLiteral.Value.Type() {
			return false
		}
		switch sourceLiteral.Value.Type() {
		case vm.TypeNull, vm.TypeUndefined:
			return true
		case vm.TypeBoolean:
			return sourceLiteral.Value.AsBoolean() == targetLiteral.Value.AsBoolean()
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			return vm.AsNumber(sourceLiteral.Value) == vm.AsNumber(targetLiteral.Value)
		case vm.TypeString:
			return vm.AsString(sourceLiteral.Value) == vm.AsString(targetLiteral.Value)
		default:
			return false
		}
	} else if sourceIsLiteral {
		// Literal to non-literal: check if literal's primitive type is assignable
		var primitiveType Type
		switch sourceLiteral.Value.Type() {
		case vm.TypeString:
			primitiveType = String
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			primitiveType = Number
		case vm.TypeBoolean:
			primitiveType = Boolean
		default:
			return false
		}
		return IsAssignable(primitiveType, target)
	} else if targetIsLiteral {
		// Non-literal to literal: generally false except for special cases
		return false
	}

	// Array type handling
	sourceArray, sourceIsArray := source.(*ArrayType)
	targetArray, targetIsArray := target.(*ArrayType)

	if sourceIsArray && targetIsArray {
		if sourceArray.ElementType == nil || targetArray.ElementType == nil {
			return false
		}
		return IsAssignable(sourceArray.ElementType, targetArray.ElementType)
	}

	// Tuple type handling
	sourceTuple, sourceIsTuple := source.(*TupleType)
	targetTuple, targetIsTuple := target.(*TupleType)

	if sourceIsTuple && targetIsTuple {
		sourceLen := len(sourceTuple.ElementTypes)
		targetLen := len(targetTuple.ElementTypes)

		// Check each target element against source
		for i := 0; i < targetLen; i++ {
			targetElementType := targetTuple.ElementTypes[i]
			targetIsOptional := i < len(targetTuple.OptionalElements) && targetTuple.OptionalElements[i]

			if i < sourceLen {
				sourceElementType := sourceTuple.ElementTypes[i]
				if !IsAssignable(sourceElementType, targetElementType) {
					return false
				}
			} else if !targetIsOptional {
				// Target element is required but source doesn't have it
				return false
			}
		}

		// Check if source has extra elements that target can't handle
		if sourceLen > targetLen && targetTuple.RestElementType == nil {
			return false
		}

		return true
	}

	// Object type handling
	sourceObj, sourceIsObj := source.(*ObjectType)
	targetObj, targetIsObj := target.(*ObjectType)

	if sourceIsObj && targetIsObj {
		// Check that all required properties in target exist in source and are assignable
		targetProps := targetObj.GetEffectiveProperties()
		sourceProps := sourceObj.GetEffectiveProperties()

		for propName, targetPropType := range targetProps {
			sourcePropType, exists := sourceProps[propName]
			if !exists {
				// Check if property is optional in target
				isOptional := targetObj.OptionalProperties != nil && targetObj.OptionalProperties[propName]
				if !isOptional {
					return false
				}
			} else {
				if !IsAssignable(sourcePropType, targetPropType) {
					return false
				}
			}
		}

		// Check call signatures
		if len(targetObj.CallSignatures) > 0 {
			if len(sourceObj.CallSignatures) == 0 {
				return false
			}
			// For now, require at least one compatible signature
			// TODO: More sophisticated overload matching
			compatible := false
			for _, targetSig := range targetObj.CallSignatures {
				for _, sourceSig := range sourceObj.CallSignatures {
					if isSignatureAssignable(sourceSig, targetSig) {
						compatible = true
						break
					}
				}
				if compatible {
					break
				}
			}
			if !compatible {
				return false
			}
		}

		return true
	}

	// Legacy FunctionType compatibility removed - use ObjectType with CallSignatures instead

	return false
}

// Helper function to check signature assignability
func isSignatureAssignable(source, target *Signature) bool {
	if source == nil || target == nil {
		return source == target
	}

	// Check parameter count compatibility
	sourceParamCount := len(source.ParameterTypes)
	targetParamCount := len(target.ParameterTypes)

	// For now, require exact parameter count match (can be relaxed later)
	if sourceParamCount != targetParamCount {
		return false
	}

	// Check parameter types (contravariant)
	for i, targetParam := range target.ParameterTypes {
		sourceParam := source.ParameterTypes[i]
		if !IsAssignable(targetParam, sourceParam) { // Note: reversed for contravariance
			return false
		}
	}

	// Check return type (covariant)
	return IsAssignable(source.ReturnType, target.ReturnType)
}

// Helper function removed - FunctionType deprecated, use ObjectType with CallSignatures

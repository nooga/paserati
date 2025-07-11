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

	// Handle forward references - they should be treated as equivalent to each other
	// This is a simple approach for now - in a full implementation, we'd resolve them properly
	if sourceRef, ok := source.(*TypeAliasForwardReference); ok {
		if targetRef, ok := target.(*TypeAliasForwardReference); ok {
			return sourceRef.AliasName == targetRef.AliasName
		}
		// For now, we'll be permissive with forward references in one direction
		// In a full implementation, we'd resolve the forward reference first
		return true
	}
	if _, ok := target.(*TypeAliasForwardReference); ok {
		// Target is a forward reference - be permissive for now
		return true
	}

	// Handle generic forward references
	if sourceGenRef, ok := source.(*GenericTypeAliasForwardReference); ok {
		if targetGenRef, ok := target.(*GenericTypeAliasForwardReference); ok {
			return sourceGenRef.AliasName == targetGenRef.AliasName
		}
		// For now, be permissive with generic forward references
		return true
	}
	if _, ok := target.(*GenericTypeAliasForwardReference); ok {
		// Target is a generic forward reference - be permissive for now
		return true
	}

	// Handle parameterized forward references (for recursive generic classes)
	if sourceParamRef, ok := source.(*ParameterizedForwardReferenceType); ok {
		if targetParamRef, ok := target.(*ParameterizedForwardReferenceType); ok {
			// Both are parameterized forward references - check if they're the same class with same type args
			if sourceParamRef.ClassName != targetParamRef.ClassName {
				return false
			}
			if len(sourceParamRef.TypeArguments) != len(targetParamRef.TypeArguments) {
				return false
			}
			for i := range sourceParamRef.TypeArguments {
				if !IsAssignable(sourceParamRef.TypeArguments[i], targetParamRef.TypeArguments[i]) {
					return false
				}
			}
			return true
		}
		// For now, be permissive when source is parameterized forward reference
		return true
	}
	if _, ok := target.(*ParameterizedForwardReferenceType); ok {
		// Target is a parameterized forward reference - be permissive for now
		// This allows object types to be assigned to forward references
		return true
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

	// TypeScript compatibility: undefined is assignable to void
	if target == Void && source == Undefined {
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

	// Handle tuple to array assignability: [string, number] should be assignable to any[]
	if sourceIsTuple && targetIsArray {
		if targetArray.ElementType == nil {
			return false
		}
		// All tuple elements must be assignable to the array element type
		for _, tupleElementType := range sourceTuple.ElementTypes {
			if !IsAssignable(tupleElementType, targetArray.ElementType) {
				return false
			}
		}
		return true
	}

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

	// Readonly type handling
	sourceReadonly, sourceIsReadonly := source.(*ReadonlyType)
	targetReadonly, targetIsReadonly := target.(*ReadonlyType)

	if sourceIsReadonly && targetIsReadonly {
		// readonly T to readonly U: T must be assignable to U
		return IsAssignable(sourceReadonly.InnerType, targetReadonly.InnerType)
	} else if sourceIsReadonly && !targetIsReadonly {
		// readonly T to T: allowed (covariance)
		return IsAssignable(sourceReadonly.InnerType, target)
	} else if !sourceIsReadonly && targetIsReadonly {
		// T to readonly T: allowed (source is assignable to target inner type)
		// This is safe because we're making something more restrictive
		return IsAssignable(source, targetReadonly.InnerType)
	}

	// TypeParameterType handling - type parameters with the same identity are assignable
	sourceTypeParam, sourceIsTypeParam := source.(*TypeParameterType)
	targetTypeParam, targetIsTypeParam := target.(*TypeParameterType)
	
	if sourceIsTypeParam && targetIsTypeParam {
		// Type parameters are assignable if they refer to the same type parameter
		// Since we might have different instances of the same logical type parameter,
		// compare by name as a fallback (this is a simplification - in a full implementation
		// we'd track scoping more carefully)
		if sourceTypeParam.Parameter == targetTypeParam.Parameter {
			return true
		}
		
		// Fallback: compare by name if they're different instances
		sourceName := sourceTypeParam.Parameter.Name
		targetName := targetTypeParam.Parameter.Name
		if sourceName == targetName {
			// fmt.Printf("// [TypeParam Debug] Allowing name-based match: '%s'\n", sourceName)
			return true
		}
		
		return false
	}
	
	// Handle case where source is a type parameter and target is a concrete type
	if sourceIsTypeParam && !targetIsTypeParam {
		// Check if the source type parameter's constraint is assignable to the target
		// This handles cases like: U extends Date should be assignable to Date
		if sourceTypeParam.Parameter.Constraint != nil {
			return IsAssignable(sourceTypeParam.Parameter.Constraint, target)
		}
		// If no constraint, fall back to checking if the type parameter itself can be assigned
		// (this would typically be false for concrete types)
		return false
	}
	
	// Note: Readonly<T> utility type is now handled via mapped type expansion
	// The expandMappedType system will convert Readonly<T> to concrete object types
	// so no special handling is needed here

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

	// TypeScript allows functions with fewer parameters to be assigned to functions expecting more
	// This is because JavaScript allows ignoring extra parameters when calling a function
	// Example: (a, b) => a + b can be assigned to (a, b, c, d) => number
	
	// The key rule: A function with fewer parameters can be assigned to one expecting more parameters
	// We only need to check that the parameters the source DOES have are compatible with
	// the corresponding parameters in the target
	
	// No minimum parameter checking needed - source can have 0 parameters and still be valid!

	// Check parameter types (contravariant) for the parameters that source provides
	checkParamCount := sourceParamCount
	if targetParamCount < sourceParamCount {
		checkParamCount = targetParamCount
	}
	
	for i := 0; i < checkParamCount; i++ {
		targetParam := target.ParameterTypes[i]
		sourceParam := source.ParameterTypes[i]
		if !IsAssignable(targetParam, sourceParam) { // Note: reversed for contravariance
			return false
		}
	}

	// Check return type (covariant)
	return IsAssignable(source.ReturnType, target.ReturnType)
}

// Helper function removed - FunctionType deprecated, use ObjectType with CallSignatures

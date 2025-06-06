package checker

import (
	"paserati/pkg/builtins"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// isAssignable checks if a value of type `source` can be assigned to a variable
// of type `target`.
// TODO: Expand significantly for structural typing, unions, intersections etc.
func (c *Checker) isAssignable(source, target types.Type) bool {
	if source == nil || target == nil {
		// Cannot determine assignability if one type is unknown/error
		// Defaulting to true might hide errors, false seems safer.
		return false
	}

	// Basic rules:
	if target == types.Any || source == types.Any {
		return true // Any accepts anything, anything goes into Any
	}

	if target == types.Unknown {
		return true // Anything can be assigned to Unknown
	}
	if source == types.Unknown {
		// Unknown can only be assigned to Unknown or Any (already handled)
		return target == types.Unknown
	}

	if source == types.Never {
		return true // Never type is assignable to anything
	}

	// Check for identical types (using pointer equality for primitives)
	if source == target {
		return true
	}

	// --- NEW: Union Type Handling ---
	sourceUnion, sourceIsUnion := source.(*types.UnionType)
	targetUnion, targetIsUnion := target.(*types.UnionType)

	if targetIsUnion {
		// Assigning TO a union: Source must be assignable to at least one type in the target union.
		if sourceIsUnion {
			// Assigning UNION to UNION (S_union to T_union):
			// Every type in S_union must be assignable to at least one type in T_union.
			for _, sType := range sourceUnion.Types {
				assignableToOneInTarget := false
				for _, tType := range targetUnion.Types {
					if c.isAssignable(sType, tType) {
						assignableToOneInTarget = true
						break
					}
				}
				if !assignableToOneInTarget {
					return false // Found a type in source union not assignable to any in target union
				}
			}
			return true // All types in source union were assignable to the target union
		} else {
			// Assigning NON-UNION to UNION (S to T_union):
			// S must be assignable to at least one type in T_union.
			for _, tType := range targetUnion.Types {
				if c.isAssignable(source, tType) {
					return true // Found a compatible type in the union
				}
			}
			return false // Source not assignable to any type in the target union
		}
	} else if sourceIsUnion {
		// Assigning FROM a union TO a non-union (S_union to T):
		// Every type in S_union must be assignable to T.
		for _, sType := range sourceUnion.Types {
			if !c.isAssignable(sType, target) {
				return false // Found a type in the source union not assignable to the target
			}
		}
		return true // All types in source union were assignable to target
	}

	// --- End Union Type Handling ---

	// --- NEW: Literal Type Handling ---
	sourceLiteral, sourceIsLiteral := source.(*types.LiteralType)
	targetLiteral, targetIsLiteral := target.(*types.LiteralType)

	if sourceIsLiteral && targetIsLiteral {
		// Assigning LiteralType to LiteralType: Values must be strictly equal
		// Use vm.valuesEqual (unexported) logic for now.
		// Types must match AND values must match.
		if sourceLiteral.Value.Type() != targetLiteral.Value.Type() {
			return false
		}
		// Use the existing loose equality check from VM package
		// as types are already confirmed to be the same.
		// Need to export valuesEqual or replicate logic.
		// Let's replicate the core logic here for simplicity for now.
		switch sourceLiteral.Value.Type() {
		case vm.TypeNull:
			return true // null === null
		case vm.TypeUndefined:
			return true // undefined === undefined
		case vm.TypeBoolean:
			return sourceLiteral.Value.AsBoolean() == targetLiteral.Value.AsBoolean()
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			return vm.AsNumber(sourceLiteral.Value) == vm.AsNumber(targetLiteral.Value)
		case vm.TypeString:
			return vm.AsString(sourceLiteral.Value) == vm.AsString(targetLiteral.Value)
		default:
			return false // Literal types cannot be functions/closures/etc.
		}
		// return vm.valuesEqual(sourceLiteral.Value, targetLiteral.Value) // If vm.valuesEqual was exported
	} else if sourceIsLiteral {
		// Assigning LiteralType TO Non-LiteralType (target):
		// Check if the literal's underlying primitive type is assignable to the target.
		// e.g., LiteralType{"hello"} -> string (true)
		// e.g., LiteralType{123} -> number (true)
		// e.g., LiteralType{true} -> boolean (true)
		// e.g., LiteralType{"hello"} -> number (false)
		var underlyingPrimitiveType types.Type
		switch sourceLiteral.Value.Type() {
		case vm.TypeString:
			underlyingPrimitiveType = types.String
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			underlyingPrimitiveType = types.Number
		case vm.TypeBoolean:
			underlyingPrimitiveType = types.Boolean
		// Cannot have literal types for null/undefined/functions/etc.
		default:
			return false // Or maybe Any/Unknown?
		}
		// Check assignability between the underlying primitive and the target
		return c.isAssignable(underlyingPrimitiveType, target)
	} else if targetIsLiteral {
		// Assigning Non-LiteralType (source) TO LiteralType:
		// This is generally only possible if the source is also a literal type
		// with the exact same value, which is covered by the first case (sourceIsLiteral && targetIsLiteral).
		// Or if the source is 'any'/'unknown' (already handled).
		// Or if the source is 'never' (already handled).
		return false
	}
	// --- End Literal Type Handling ---

	// --- NEW: Array Type Assignability ---
	sourceArray, sourceIsArray := source.(*types.ArrayType)
	targetArray, targetIsArray := target.(*types.ArrayType)

	if sourceIsArray && targetIsArray {
		// Both are arrays. Check if source element type is assignable to target element type.
		// This is a basic covariance check. Stricter checks might be needed later.
		if sourceArray.ElementType == nil || targetArray.ElementType == nil {
			// If either element type is unknown (shouldn't happen?), consider it not assignable for safety.
			return false
		}
		return c.isAssignable(sourceArray.ElementType, targetArray.ElementType)
	}

	// --- End Array Type Handling ---

	// --- NEW: Function Type Assignability (including CallableType) ---
	sourceFunc, sourceIsFunc := source.(*types.FunctionType)
	targetFunc, targetIsFunc := target.(*types.FunctionType)
	sourceCallable, sourceIsCallable := source.(*types.CallableType)
	targetCallable, targetIsCallable := target.(*types.CallableType)
	sourceOverloaded, sourceIsOverloaded := source.(*types.OverloadedFunctionType)
	targetOverloaded, targetIsOverloaded := target.(*types.OverloadedFunctionType)

	if targetIsFunc {
		if sourceIsFunc {
			// Assigning Function to Function

			// Check Arity
			if len(sourceFunc.ParameterTypes) != len(targetFunc.ParameterTypes) {
				return false // Arity mismatch
			}

			// Check Parameter Types (Contravariance - target param assignable to source param)
			// For simplicity now, let's check invariance: source param assignable to target param
			for i, targetParamType := range targetFunc.ParameterTypes {
				sourceParamType := sourceFunc.ParameterTypes[i]
				// if !c.isAssignable(targetParamType, sourceParamType) { // Contravariant check
				if !c.isAssignable(sourceParamType, targetParamType) { // Invariant check (simpler)
					return false // Parameter type mismatch
				}
			}

			// Check Return Type (Covariance - source return assignable to target return)
			if !c.isAssignable(sourceFunc.ReturnType, targetFunc.ReturnType) {
				return false // Return type mismatch
			}

			// All checks passed
			return true
		} else if sourceIsCallable {
			// Assigning CallableType to FunctionType: use call signature
			if sourceCallable.CallSignature == nil {
				return false
			}
			return c.isAssignable(sourceCallable.CallSignature, targetFunc)
		} else {
			// Assigning Non-Function to Function Target: Generally false
			// (Unless source is Any/Unknown/Never, handled earlier)
			return false
		}
	} else if targetIsOverloaded {
		// Assigning to OverloadedFunctionType
		if sourceIsFunc {
			// Assigning a regular function to an overloaded function type
			// The source function must be compatible with ALL overloads
			for _, overload := range targetOverloaded.Overloads {
				if !c.isImplementationCompatible(sourceFunc, overload) {
					return false // Source function not compatible with this overload
				}
			}
			return true // Source function is compatible with all overloads
		} else if sourceIsOverloaded {
			// Assigning OverloadedFunctionType to OverloadedFunctionType
			// For now, require exact match
			return sourceOverloaded.Equals(targetOverloaded)
		} else {
			// Assigning Non-Function to OverloadedFunctionType: Generally false
			return false
		}
	} else if sourceIsOverloaded {
		// Assigning OverloadedFunctionType to something else
		if targetIsFunc {
			// Can an overloaded function be assigned to a single function type?
			// This is complex - for now, let's be strict and disallow it
			return false
		} else {
			// Assigning OverloadedFunctionType to non-function type: false
			return false
		}
	} else if targetIsCallable {
		// Assigning to CallableType
		if sourceIsFunc {
			// Assigning FunctionType to CallableType: check against call signature
			if targetCallable.CallSignature == nil {
				return false
			}
			return c.isAssignable(sourceFunc, targetCallable.CallSignature)
		} else if sourceIsCallable {
			// Assigning CallableType to CallableType: check both call signature and properties
			// Call signatures must be compatible
			if (sourceCallable.CallSignature == nil) != (targetCallable.CallSignature == nil) {
				return false
			}
			if sourceCallable.CallSignature != nil && !c.isAssignable(sourceCallable.CallSignature, targetCallable.CallSignature) {
				return false
			}
			// All target properties must exist in source with compatible types
			for propName, targetPropType := range targetCallable.Properties {
				sourcePropType, exists := sourceCallable.Properties[propName]
				if !exists {
					return false // Target requires property that source doesn't have
				}
				if !c.isAssignable(sourcePropType, targetPropType) {
					return false // Property type mismatch
				}
			}
			return true
		} else {
			// Assigning Non-Callable to CallableType: false
			return false
		}
	} else if sourceIsCallable {
		// Assigning CallableType to something else (not function or callable)
		// Maybe check if target is an ObjectType that matches the properties?
		if targetObj, ok := target.(*types.ObjectType); ok {
			// Check if CallableType's properties are assignable to ObjectType
			for propName, targetPropType := range targetObj.Properties {
				sourcePropType, exists := sourceCallable.Properties[propName]
				if !exists {
					// Check if this property is optional in the target
					isOptional := targetObj.OptionalProperties != nil && targetObj.OptionalProperties[propName]
					if !isOptional {
						return false // Target requires property that source doesn't have
					}
					continue
				}
				if !c.isAssignable(sourcePropType, targetPropType) {
					return false // Property type mismatch
				}
			}
			return true // CallableType properties are compatible with ObjectType
		} else {
			// Assigning CallableType to other types: false
			return false
		}
	}
	// --- End Function Type Handling ---

	// --- NEW: Object Type Assignability ---
	sourceObject, sourceIsObject := source.(*types.ObjectType)
	targetObject, targetIsObject := target.(*types.ObjectType)

	if targetIsObject && sourceIsObject {
		// Both are objects. Check structural compatibility.
		// For source to be assignable to target:
		// - All REQUIRED properties in target must exist in source with compatible types
		// - Optional properties in target don't need to exist in source
		// - Source can have additional properties (structural typing)

		for targetPropName, targetPropType := range targetObject.Properties {
			sourcePropType, exists := sourceObject.Properties[targetPropName]
			if !exists {
				// Check if this property is optional in the target
				isOptional := targetObject.OptionalProperties != nil && targetObject.OptionalProperties[targetPropName]
				if !isOptional {
					// Target requires property that source doesn't have
					return false
				}
				// Property is optional, so it's okay that source doesn't have it
				continue
			}
			if !c.isAssignable(sourcePropType, targetPropType) {
				// Property type mismatch
				return false
			}
		}
		// All required target properties found and compatible in source
		return true
	}
	// --- End Object Type Handling ---

	// TODO: Handle null/undefined assignability based on strict flags later.
	// For now, let's be strict unless target is Any/Unknown/Union.
	if source == types.Null && target != types.Null { // Allow null -> T | null
		return false
	}
	if source == types.Undefined && target != types.Undefined { // Allow undefined -> T | undefined
		return false
	}

	// TODO: Add structural checks for objects/arrays
	// TODO: Add checks for function type compatibility
	// TODO: Add checks for intersections

	// Default: not assignable
	return false
}

// deeplyWidenObjectType creates a new ObjectType where literal property types are widened.
// Returns the original type if it's not an ObjectType.
func deeplyWidenType(t types.Type) types.Type {
	// Widen top-level literals first
	widenedT := types.GetWidenedType(t)

	// If it's an object after top-level widening, widen its properties
	if objType, ok := widenedT.(*types.ObjectType); ok {
		newFields := make(map[string]types.Type, len(objType.Properties))
		for name, propType := range objType.Properties {
			// Recursively deeply widen property types? For now, just one level.
			newFields[name] = types.GetWidenedType(propType)
		}
		return &types.ObjectType{Properties: newFields}
	}

	// If it was an array, maybe deeply widen its element type?
	if arrType, ok := widenedT.(*types.ArrayType); ok {
		// Avoid infinite recursion for recursive types: Check if elem type is same as t?
		// For now, let's not recurse into arrays here, only objects.
		// return &types.ArrayType{ElementType: deeplyWidenType(arrType.ElementType)}
		return arrType // Return array type as is for now
	}

	// Return the (potentially top-level widened) type if not an object
	return widenedT
}

// getBuiltinType looks up a builtin type by name (e.g., "String.fromCharCode")
func (c *Checker) getBuiltinType(name string) types.Type {
	return builtins.GetType(name)
}

// getPropertyTypeFromType returns the type of a property access on the given type
// isOptionalChaining determines whether to be permissive about missing properties
func (c *Checker) getPropertyTypeFromType(objectType types.Type, propertyName string, isOptionalChaining bool) types.Type {
	// Widen the object type for checks
	widenedObjectType := types.GetWidenedType(objectType)

	if widenedObjectType == types.Any {
		return types.Any // Property access on 'any' results in 'any'
	} else if widenedObjectType == types.Null || widenedObjectType == types.Undefined {
		return types.Undefined
	} else if widenedObjectType == types.String {
		if propertyName == "length" {
			return types.Number // string.length is number
		} else {
			// Check prototype registry for String methods
			if methodType := builtins.GetPrototypeMethodType("string", propertyName); methodType != nil {
				return methodType
			} else {
				if !isOptionalChaining {
					// Only add error for regular member access, not optional chaining
					// Note: We can't add error here since we don't have the node, but that's ok for union handling
				}
				return types.Never
			}
		}
	} else {
		// Use a type switch for struct-based types
		switch obj := widenedObjectType.(type) {
		case *types.ArrayType:
			if propertyName == "length" {
				return types.Number // Array.length is number
			} else {
				// Check prototype registry for Array methods
				if methodType := builtins.GetPrototypeMethodType("array", propertyName); methodType != nil {
					return methodType
				} else {
					return types.Never
				}
			}
		case *types.ObjectType:
			// Look for the property in the object's fields
			fieldType, exists := obj.Properties[propertyName]
			if exists {
				// Property found
				if fieldType == nil { // Should ideally not happen if checker populates correctly
					return types.Never
				} else {
					return fieldType
				}
			} else {
				// Property not found
				if isOptionalChaining {
					return types.Undefined
				} else {
					return types.Never
				}
			}
		case *types.FunctionType:
			// Regular function types don't have properties
			return types.Never
		case *types.CallableType:
			// Handle property access on callable types
			if propType, exists := obj.Properties[propertyName]; exists {
				return propType
			} else {
				if isOptionalChaining {
					return types.Undefined
				} else {
					return types.Never
				}
			}
		default:
			// This covers cases where widenedObjectType was not String, Any, ArrayType, ObjectType, etc.
			return types.Never
		}
	}
}

package types

import "github.com/nooga/paserati/pkg/vm"

// --- Type Widening ---

// GetWidenedType converts literal types to their corresponding primitive base types.
// Other types are returned unchanged.
func GetWidenedType(t Type) Type {
	// Without strictNullChecks the null and undefined types widen to any
	// (an initializer of `null` declares an `any` variable).
	if !StrictNullChecks && (t == Null || t == Undefined) {
		return Any
	}
	if litType, ok := t.(*LiteralType); ok {
		switch litType.Value.Type() {
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			return Number
		case vm.TypeBigInt:
			return BigInt
		case vm.TypeString:
			return String
		case vm.TypeBoolean:
			return Boolean
		case vm.TypeNull:
			return Null // Null widens to null
		case vm.TypeUndefined:
			return Undefined // Undefined widens to undefined
		default:
			// Should not happen for valid literal types (like Function/Closure)
			return t // Return original if unexpected underlying type
		}
	}

	// Handle type parameters with constraints for arithmetic operations
	if typeParam, ok := t.(*TypeParameterType); ok {
		if typeParam.Parameter.Constraint != nil {
			// For arithmetic operations, widen to the constraint
			// e.g., T extends number -> number for + operations
			return GetWidenedType(typeParam.Parameter.Constraint)
		}
	}

	// TODO: Should unions containing only literals of the same base type also widen?
	// e.g., should (1 | 2 | 3) widen to number? Probably.
	// This would require more complex logic here or in NewUnionType.
	return t // Not a literal type, return as is
}

// WidenType converts literal types to their primitive equivalents
func WidenType(t Type) Type {
	return GetWidenedType(t) // Use existing function
}

// deeplyWidenObjectType creates a new ObjectType where literal property types are widened.
// Returns the original type if it's not an ObjectType.
func DeeplyWidenType(t Type) Type {
	// Widen top-level literals first
	widenedT := GetWidenedType(t)

	// If it's an object after top-level widening, widen its properties
	if objType, ok := widenedT.(*ObjectType); ok {
		newFields := make(map[string]Type, len(objType.Properties))
		for _, name := range SortedPropertyNames(objType.Properties) {
			propType := objType.Properties[name]
			if objType.ReadOnlyProperties[name] {
				// A readonly property keeps its literal type.
				newFields[name] = propType
				continue
			}
			newFields[name] = widenNested(propType, 0)
		}
		return &ObjectType{
			Properties:          newFields,
			OptionalProperties:  objType.OptionalProperties,
			CallSignatures:      objType.CallSignatures,
			ConstructSignatures: objType.ConstructSignatures,
			BaseTypes:           objType.BaseTypes,
			ClassMeta:           objType.ClassMeta,       // Preserve class metadata
			IndexSignatures:     objType.IndexSignatures, // Preserve index signatures
		}
	}

	// If it was an array, maybe deeply widen its element type?
	if arrType, ok := widenedT.(*ArrayType); ok {
		// Avoid infinite recursion for recursive types: Check if elem type is same as t?
		// For now, let's not recurse into arrays here, only objects.
		// return &types.ArrayType{ElementType: deeplyWidenType(arrType.ElementType)}
		return arrType // Return array type as is for now
	}

	// Return the (potentially top-level widened) type if not an object
	return widenedT
}

// WidenEnumMember widens an enum member type to its enum (`let t = Color.Red`
// declares a Color), as tsc does for a mutable declaration's inferred type.
// Other types are returned unchanged.
func WidenEnumMember(t Type) Type {
	if em, ok := t.(*EnumMemberType); ok && em.Parent != nil {
		return em.Parent.UnionOfMembers()
	}
	return t
}

// widenNested widens literal types in a property position: literals become
// their base type and plain nested object (literal) types are widened the same
// way. Callable types, class instances and everything else are left alone.
func widenNested(t Type, depth int) Type {
	if depth > 4 {
		return t
	}
	t = GetWidenedType(t)
	obj, ok := t.(*ObjectType)
	if !ok || obj.IsCallable() || len(obj.ConstructSignatures) > 0 || obj.ClassMeta != nil || obj.IsInterface {
		return t
	}
	changed := false
	props := make(map[string]Type, len(obj.Properties))
	for _, name := range SortedPropertyNames(obj.Properties) {
		pt := obj.Properties[name]
		w := widenNested(pt, depth+1)
		if w != pt {
			changed = true
		}
		props[name] = w
	}
	if !changed {
		return t
	}
	cp := *obj
	cp.Properties = props
	return &cp
}

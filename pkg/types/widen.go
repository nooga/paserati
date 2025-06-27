package types

import "paserati/pkg/vm"

// --- Type Widening ---

// GetWidenedType converts literal types to their corresponding primitive base types.
// Other types are returned unchanged.
func GetWidenedType(t Type) Type {
	if litType, ok := t.(*LiteralType); ok {
		switch litType.Value.Type() {
		case vm.TypeFloatNumber, vm.TypeIntegerNumber:
			return Number
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
		for name, propType := range objType.Properties {
			// Recursively deeply widen property types? For now, just one level.
			newFields[name] = GetWidenedType(propType)
		}
		return &ObjectType{
			Properties:          newFields,
			OptionalProperties:  objType.OptionalProperties,
			CallSignatures:      objType.CallSignatures,
			ConstructSignatures: objType.ConstructSignatures,
			BaseTypes:           objType.BaseTypes,
			ClassMeta:           objType.ClassMeta, // Preserve class metadata
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

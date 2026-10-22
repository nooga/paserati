package types

import "github.com/nooga/paserati/pkg/vm"

// --- Type Widening ---

// GetWidenedType converts literal types to their corresponding primitive base types.
// Other types are returned unchanged.
func (r *Relation) GetWidenedType(t Type) Type {
	// Without strictNullChecks the null and undefined types widen to any
	// (an initializer of `null` declares an `any` variable).
	if !r.StrictNullChecks && (t == Null || t == Undefined) {
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
			return r.GetWidenedType(typeParam.Parameter.Constraint)
		}
	}

	// TODO: Should unions containing only literals of the same base type also widen?
	// e.g., should (1 | 2 | 3) widen to number? Probably.
	// This would require more complex logic here or in NewUnionType.
	return t // Not a literal type, return as is
}

// WidenType converts literal types to their primitive equivalents
func (r *Relation) WidenType(t Type) Type {
	return r.GetWidenedType(t) // Use existing function
}

// deeplyWidenObjectType creates a new ObjectType where literal property types are widened.
// Returns the original type if it's not an ObjectType.
func (r *Relation) DeeplyWidenType(t Type) Type {
	// FIXME: `let y = x` with x: T declares T in tsc, but a type parameter
	// widens to its constraint here (GetWidenedType's arithmetic rule).
	// Keeping T exposes type parameters an inner generic call leaves
	// uninferred (tRPC's source.pipe(distinctUntilChanged()), which tsc
	// infers from pipe's parameter), so that has to be fixed first.
	if tt, ok := t.(*IntersectionType); ok && intersectsTypeParameter(tt) {
		// `{ ...x, q: 1 }` is T & { q: number }: each part widens. Other
		// intersections are declared types (Nominal<'A', string>) and keep
		// their literals.
		parts := make([]Type, len(tt.Types))
		changed := false
		for i, part := range tt.Types {
			parts[i] = part
			if _, isTP := part.(*TypeParameterType); !isTP {
				parts[i] = r.DeeplyWidenType(part)
			}
			changed = changed || parts[i] != part
		}
		if !changed {
			return t
		}
		return NewIntersectionType(parts...)
	}
	// Widen top-level literals first
	widenedT := r.GetWidenedType(t)

	// If it's an object after top-level widening, widen its properties. A
	// declared type (an interface, a class instance) is never widened: only
	// an object literal's literal properties are (`b: 1` in interface B
	// stays 1, or id(b) would no longer be a B).
	if objType, ok := widenedT.(*ObjectType); ok && !objType.IsInterface && objType.ClassMeta == nil {
		newFields := make(map[string]Type, len(objType.Props()))
		for _, name := range SortedPropertyNames(objType.Props()) {
			propType := objType.Props()[name]
			if objType.ReadOnlyProps()[name] {
				// A readonly property keeps its literal type.
				newFields[name] = propType
				continue
			}
			newFields[name] = r.widenNested(propType, 0)
		}
		return &ObjectType{
			Properties:          newFields,
			PropertyOrder:       objType.PropOrder(),
			PropertyDocs:        objType.PropertyDocs,
			Doc:                 objType.Doc,
			DisplayName:         objType.DisplayName,
			GenericName:         objType.GenericName,
			GenericArgs:         objType.GenericArgs,
			OptionalProperties:  objType.OptionalProps(),
			ReadOnlyProperties:  objType.ReadOnlyProps(),
			CallSignatures:      objType.CallSigs(),
			ConstructSignatures: objType.ConstructSigs(),
			BaseTypes:           objType.Bases(),
			ClassMeta:           objType.ClassMeta,   // Preserve class metadata
			IndexSignatures:     objType.IndexSigs(), // Preserve index signatures
		}
	}

	// If it was an array, maybe deeply widen its element type?
	if arrType, ok := widenedT.(*ArrayType); ok {
		// Avoid infinite recursion for recursive types: Check if elem type is same as t?
		// For now, let's not recurse into arrays here, only objects.
		// return &types.ArrayType{ElementType: deeplyWidenType(arrType.ElementType)}
		return arrType // Return array type as is for now
	}

	// Without strictNullChecks a tuple's null and undefined elements widen
	// to any, as an array literal's do (`var y = foo([undefined])` with
	// foo<T extends [any]> declares [any]).
	if tuple, ok := widenedT.(*TupleType); ok && !r.StrictNullChecks {
		var elems []Type
		for i, e := range tuple.ElementTypes {
			if e == Null || e == Undefined {
				if elems == nil {
					elems = append([]Type(nil), tuple.ElementTypes...)
				}
				elems[i] = Any
			}
		}
		if elems != nil {
			cp := *tuple
			cp.ElementTypes = elems
			return &cp
		}
	}

	// Return the (potentially top-level widened) type if not an object
	return widenedT
}

// WidenEnumMember widens null and undefined to any (see below) and an enum member type to its enum (`let t = Color.Red`
// declares a Color), as tsc does for a mutable declaration's inferred type.
// Other types are returned unchanged.
func WidenEnumMember(t Type) Type {
	// A mutable `let x = null` has no useful declared type: tsc evolves it
	// with the assignments that follow, which any approximates.
	if t == Null || t == Undefined {
		return Any
	}
	if em, ok := t.(*EnumMemberType); ok && em.Parent != nil {
		return em.Parent.UnionOfMembers()
	}
	return t
}

// widenNested widens literal types in a property position: literals become
// their base type and plain nested object (literal) types are widened the same
// way. Callable types, class instances and everything else are left alone.
func (r *Relation) widenNested(t Type, depth int) Type {
	if depth > 4 {
		return t
	}
	t = r.GetWidenedType(t)
	obj, ok := t.(*ObjectType)
	if !ok || obj.IsCallable() || len(obj.ConstructSigs()) > 0 || obj.ClassMeta != nil || obj.IsInterface {
		return t
	}
	changed := false
	props := make(map[string]Type, len(obj.Props()))
	for _, name := range SortedPropertyNames(obj.Props()) {
		pt := obj.Props()[name]
		w := r.widenNested(pt, depth+1)
		if w != pt {
			changed = true
		}
		props[name] = w
	}
	if !changed {
		return t
	}
	cp := *obj
	cp.Materialize()
	cp.Properties = props
	return &cp
}

// intersectsTypeParameter reports whether one of t's parts is a type
// parameter, as in the type of `{ ...x, q: 1 }` with x: T.
func intersectsTypeParameter(t *IntersectionType) bool {
	for _, part := range t.Types {
		if _, ok := part.(*TypeParameterType); ok {
			return true
		}
	}
	return false
}

package types

// IsComparable approximates TypeScript's comparable relation, which type
// assertions use: S is comparable to T when S is assignable to T, or when a
// union source has a constituent that is comparable, or (for objects) when each
// property the two types share is comparable. Types whose structure is not
// modelled here (type parameters, mapped and conditional types, ...) are
// treated as comparable so that an assertion is only rejected when the types
// are clearly unrelated.
func (r *Relation) IsComparable(source, target Type) bool {
	return r.comparable(source, target, 0)
}

func (r *Relation) comparable(s, t Type, depth int) bool {
	if s == nil || t == nil || depth > 6 {
		return true
	}
	if r.isAssignable(s, t) {
		return true
	}
	if s == Any || t == Any || s == Unknown || t == Unknown {
		return true
	}
	// Simple (leaf) types are comparable when related in either direction.
	if isLeafType(s) && isLeafType(t) {
		return r.isAssignable(t, s)
	}
	if su, ok := s.(*UnionType); ok {
		for _, m := range su.Types {
			if r.comparable(m, t, depth+1) {
				return true
			}
		}
		return false
	}
	if tu, ok := t.(*UnionType); ok {
		for _, m := range tu.Types {
			if r.comparable(s, m, depth+1) {
				return true
			}
		}
		return false
	}
	if !isSimpleStructure(s) || !isSimpleStructure(t) {
		return true
	}
	switch sv := s.(type) {
	case *ArrayType:
		if tv, ok := t.(*ArrayType); ok {
			return r.comparable(sv.ElementType, tv.ElementType, depth+1)
		}
		if tv, ok := t.(*TupleType); ok {
			for _, e := range tv.ElementTypes {
				if !r.comparable(sv.ElementType, e, depth+1) {
					return false
				}
			}
			return true
		}
	case *TupleType:
		if tv, ok := t.(*ArrayType); ok {
			for _, e := range sv.ElementTypes {
				if !r.comparable(e, tv.ElementType, depth+1) {
					return false
				}
			}
			return true
		}
		if tv, ok := t.(*TupleType); ok {
			if len(sv.ElementTypes) != len(tv.ElementTypes) && sv.RestElementType == nil && tv.RestElementType == nil {
				return false
			}
			for i := 0; i < len(sv.ElementTypes) && i < len(tv.ElementTypes); i++ {
				if !r.comparable(sv.ElementTypes[i], tv.ElementTypes[i], depth+1) {
					return false
				}
			}
			return true
		}
	case *ObjectType:
		if tv, ok := t.(*ObjectType); ok {
			return r.comparableObjects(sv, tv, depth)
		}
	}
	return false
}

// isSimpleStructure reports whether a type's structure is modelled well
// enough to reject a comparison on.
func isSimpleStructure(t Type) bool {
	switch t.(type) {
	case *Primitive, *LiteralType, *EnumMemberType, *ArrayType, *TupleType, *ObjectType:
		return true
	}
	return false
}

// comparableObjects relates two object types: every property of the target
// must exist in the source (unless optional) and be comparable.
func (r *Relation) comparableObjects(s, t *ObjectType, depth int) bool {
	sProps := s.GetEffectiveProperties()
	for name, tp := range t.GetEffectiveProperties() {
		sp, ok := sProps[name]
		if !ok {
			if t.IsPropertyOptional(name) {
				continue
			}
			return false
		}
		if r.StrictNullChecks {
			if s.IsPropertyOptional(name) {
				sp = NewUnionType(sp, Undefined)
			}
			if t.IsPropertyOptional(name) {
				tp = NewUnionType(tp, Undefined)
			}
		}
		if !r.comparable(sp, tp, depth+1) {
			return false
		}
	}
	if len(t.CallSigs()) > 0 && len(s.CallSigs()) == 0 {
		return false
	}
	if len(t.ConstructSigs()) > 0 && len(s.ConstructSigs()) == 0 {
		return false
	}
	for _, ts := range t.CallSigs() {
		matched := false
		for _, ss := range s.CallSigs() {
			if r.comparableSignatures(ss, ts, depth) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// comparableSignatures relates two signatures loosely: the source may not
// require more arguments than the target supplies, parameters need only be
// comparable in either direction, and the returns must be comparable.
func (r *Relation) comparableSignatures(s, t *Signature, depth int) bool {
	if t.RestParameterType == nil && sigMinArgs(s) > len(t.ParameterTypes) {
		return false
	}
	n := len(s.ParameterTypes)
	if len(t.ParameterTypes) < n {
		n = len(t.ParameterTypes)
	}
	for i := 0; i < n; i++ {
		if !r.comparable(s.ParameterTypes[i], t.ParameterTypes[i], depth+1) && !r.comparable(t.ParameterTypes[i], s.ParameterTypes[i], depth+1) {
			return false
		}
	}
	if t.ReturnType == Void || t.ReturnType == nil || s.ReturnType == nil {
		return true
	}
	return r.comparable(s.ReturnType, t.ReturnType, depth+1)
}

func isLeafType(t Type) bool {
	switch t.(type) {
	case *Primitive, *LiteralType, *EnumMemberType:
		return true
	}
	return false
}

// IsIdenticalType approximates TypeScript's identity relation (used for
// "subsequent variable declarations must have the same type"): the types are
// assignable in both directions, with any and unknown identical only to
// themselves.
func (r *Relation) IsIdenticalType(a, b Type) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a == Any || b == Any || a == Unknown || b == Unknown {
		return false
	}
	return r.isAssignable(a, b) && r.isAssignable(b, a)
}

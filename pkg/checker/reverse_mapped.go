package checker

import (
	"github.com/nooga/paserati/pkg/types"
)

// Inference to a homomorphic mapped type `{ [K in keyof T]: Template }` over
// a type parameter T infers T itself from the argument, as tsc's reverse
// mapped types do: each member of the argument is read back through the
// template (Box<T[K]> against Box<number> gives number for that member).

// reverseMappedConstraint infers T from argType when pType maps over keyof T.
func (c *Checker) reverseMappedConstraint(pType *types.MappedType, argType types.Type) (TypeParameterConstraint, bool) {
	keyof, ok := pType.ConstraintType.(*types.KeyofType)
	if !ok {
		return TypeParameterConstraint{}, false
	}
	tp, ok := keyof.OperandType.(*types.TypeParameterType)
	if !ok || tp.Parameter == nil {
		return TypeParameterConstraint{}, false
	}
	reversed := c.reverseMappedType(pType, tp.Parameter, argType)
	if reversed == nil {
		return TypeParameterConstraint{}, false
	}
	return TypeParameterConstraint{TypeParameter: tp.Parameter, InferredType: reversed, Confidence: 100}, true
}

// reverseMappedType is the T whose mapping through pType gives source, or
// nil when source is not an object, array or tuple.
func (c *Checker) reverseMappedType(pType *types.MappedType, t *types.TypeParameter, source types.Type) types.Type {
	switch src := source.(type) {
	case *types.ArrayType:
		return &types.ArrayType{ElementType: c.reverseMappedMember(pType, t, src.ElementType)}
	case *types.TupleType:
		elems := make([]types.Type, len(src.ElementTypes))
		for i, e := range src.ElementTypes {
			elems[i] = c.reverseMappedMember(pType, t, e)
		}
		var rest types.Type
		if src.RestElementType != nil {
			rest = c.reverseMappedMember(pType, t, src.RestElementType)
		}
		return &types.TupleType{ElementTypes: elems, OptionalElements: src.OptionalElements, RestElementType: rest}
	}
	obj, ok := c.structuralObject(source)
	if !ok || obj == nil || len(obj.CallSigs()) > 0 || len(obj.ConstructSigs()) > 0 {
		return nil
	}
	result := types.NewObjectType()
	for _, name := range obj.PropertyNames() {
		propType := obj.Props()[name]
		member := c.reverseMappedMember(pType, t, propType)
		optional := obj.OptionalProps()[name] && pType.OptionalModifier != "+"
		switch {
		case optional:
			result.WithOptionalProperty(name, member)
		case obj.IsReadOnly(name):
			result.WithReadOnlyProperty(name, member)
		default:
			result.WithProperty(name, member)
		}
	}
	return result
}

// reverseMappedMember infers T[K] for one member: the template with T[K]
// replaced by a fresh type parameter, inferred against the member's type.
func (c *Checker) reverseMappedMember(pType *types.MappedType, t *types.TypeParameter, member types.Type) types.Type {
	if c.isMappedIndexedAccess(pType.ValueType, pType, t) {
		return member
	}
	placeholder := &types.TypeParameter{Name: "__reverse_" + t.Name}
	template := c.replaceMappedIndexedAccess(pType.ValueType, pType, t, &types.TypeParameterType{Parameter: placeholder}, map[types.Type]types.Type{})
	var found []types.Type
	for _, con := range c.collectConstraintsFromTypeSeen(template, member, map[constraintTypePair]bool{}) {
		if con.TypeParameter == placeholder && con.InferredType != nil {
			found = append(found, con.InferredType)
		}
	}
	switch len(found) {
	case 0:
		return types.Unknown
	case 1:
		return found[0]
	}
	return types.NewUnionType(found...)
}

// isMappedIndexedAccess reports whether x is T[K] for the mapped type's K.
func (c *Checker) isMappedIndexedAccess(x types.Type, pType *types.MappedType, t *types.TypeParameter) bool {
	ia, ok := x.(*types.IndexedAccessType)
	if !ok {
		return false
	}
	obj, ok := ia.ObjectType.(*types.TypeParameterType)
	if !ok || obj.Parameter != t {
		return false
	}
	idx, ok := ia.IndexType.(*types.TypeParameterType)
	return ok && idx.Parameter != nil && idx.Parameter.Name == pType.TypeParameter
}

// replaceMappedIndexedAccess replaces T[K] in a template by with.
func (c *Checker) replaceMappedIndexedAccess(x types.Type, pType *types.MappedType, t *types.TypeParameter, with types.Type, memo map[types.Type]types.Type) types.Type {
	if x == nil {
		return nil
	}
	if c.isMappedIndexedAccess(x, pType, t) {
		return with
	}
	if done, ok := memo[x]; ok {
		return done
	}
	rec := func(y types.Type) types.Type { return c.replaceMappedIndexedAccess(y, pType, t, with, memo) }
	switch tt := x.(type) {
	case *types.ArrayType:
		return &types.ArrayType{ElementType: rec(tt.ElementType)}
	case *types.UnionType:
		members := make([]types.Type, len(tt.Types))
		for i, m := range tt.Types {
			members[i] = rec(m)
		}
		return types.NewUnionType(members...)
	case *types.TupleType:
		elems := make([]types.Type, len(tt.ElementTypes))
		for i, e := range tt.ElementTypes {
			elems[i] = rec(e)
		}
		var rest types.Type
		if tt.RestElementType != nil {
			rest = rec(tt.RestElementType)
		}
		return &types.TupleType{ElementTypes: elems, OptionalElements: tt.OptionalElements, RestElementType: rest}
	case *types.InstantiatedType:
		args := make([]types.Type, len(tt.TypeArguments))
		for i, a := range tt.TypeArguments {
			args[i] = rec(a)
		}
		return &types.InstantiatedType{Generic: tt.Generic, TypeArguments: args}
	case *types.ObjectType:
		cp := types.NewObjectType()
		memo[x] = cp
		cp.Instantiates = tt.Instantiates
		cp.GenericArgs = make([]types.Type, len(tt.GenericArgs))
		for i, a := range tt.GenericArgs {
			cp.GenericArgs[i] = rec(a)
		}
		for _, name := range tt.PropertyNames() {
			p := rec(tt.Props()[name])
			switch {
			case tt.OptionalProps()[name]:
				cp.WithOptionalProperty(name, p)
			case tt.IsReadOnly(name):
				cp.WithReadOnlyProperty(name, p)
			default:
				cp.WithProperty(name, p)
			}
		}
		for _, sig := range tt.CallSigs() {
			cp.WithCallSignature(c.replaceInSignature(sig, rec))
		}
		return cp
	}
	return x
}

func (c *Checker) replaceInSignature(sig *types.Signature, rec func(types.Type) types.Type) *types.Signature {
	cp := *sig
	cp.ParameterTypes = make([]types.Type, len(sig.ParameterTypes))
	for i, p := range sig.ParameterTypes {
		cp.ParameterTypes[i] = rec(p)
	}
	cp.ReturnType = rec(sig.ReturnType)
	if sig.RestParameterType != nil {
		cp.RestParameterType = rec(sig.RestParameterType)
	}
	return &cp
}

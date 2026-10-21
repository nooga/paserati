package checker

import (
	"github.com/nooga/paserati/pkg/types"
)

// withTrueBranchConstraint runs fn (resolving the true branch of a
// conditional type) with the check type parameter known to also satisfy the
// extends type: in `T extends X ? A<T> : B`, T is a T & X in A<T>, as tsc's
// substitution types make it.
func (c *Checker) withTrueBranchConstraint(checkType, extendsType types.Type, fn func()) {
	tp, ok := checkType.(*types.TypeParameterType)
	if !ok || tp.Parameter == nil || extendsType == nil || mentionsInfer(extendsType) {
		fn()
		return
	}
	if c.trueBranchConstraints == nil {
		c.trueBranchConstraints = map[*types.TypeParameter][]types.Type{}
	}
	p := tp.Parameter
	c.trueBranchConstraints[p] = append(c.trueBranchConstraints[p], extendsType)
	defer func() { c.trueBranchConstraints[p] = c.trueBranchConstraints[p][:len(c.trueBranchConstraints[p])-1] }()
	fn()
}

// mentionsInfer reports whether t is or directly holds an `infer` placeholder.
func mentionsInfer(t types.Type) bool {
	switch tt := t.(type) {
	case *types.InferType:
		return true
	case *types.UnionType:
		for _, m := range tt.Types {
			if mentionsInfer(m) {
				return true
			}
		}
	case *types.IntersectionType:
		for _, m := range tt.Types {
			if mentionsInfer(m) {
				return true
			}
		}
	}
	return false
}

// baseConstraint is what a generic type argument is known to be, for checking
// it against a type parameter's constraint (tsc's base constraint): a type
// parameter's constraint (and the extends types of the conditional true
// branches it is in), an indexed access into such a type, and either branch
// of a deferred conditional type. It returns nil when nothing is known that
// could fail a constraint (an `infer` placeholder, whose implied constraint
// is not computed here), and t itself when t is not generic.
func (c *Checker) baseConstraint(t types.Type, depth int) types.Type {
	if t == nil || depth > 8 {
		return t
	}
	switch tt := t.(type) {
	case *types.InferType:
		return nil
	case *types.TypeParameterType:
		if tt.Parameter == nil {
			return t
		}
		var parts []types.Type
		if tt.Parameter.Constraint != nil {
			parts = append(parts, tt.Parameter.Constraint)
		}
		parts = append(parts, c.trueBranchConstraints[tt.Parameter]...)
		switch len(parts) {
		case 0:
			return types.Unknown
		case 1:
			return c.baseConstraint(parts[0], depth+1)
		}
		return c.baseConstraint(types.NewIntersectionType(parts...), depth+1)
	case *types.IndexedAccessType:
		obj := c.baseConstraint(tt.ObjectType, depth+1)
		idx := c.baseConstraint(tt.IndexType, depth+1)
		if obj == nil || idx == nil {
			return nil
		}
		if obj == tt.ObjectType && idx == tt.IndexType {
			return t
		}
		if obj == types.Any {
			return types.Any // any[K] is any
		}
		if r := c.computeIndexedAccessType(c.expandIfMappedType(obj), idx); r != nil {
			if _, still := r.(*types.IndexedAccessType); still {
				return c.baseConstraint(r, depth+1)
			}
			return r
		}
		return &types.IndexedAccessType{ObjectType: obj, IndexType: idx}
	case *types.ConditionalType:
		tb := c.baseConstraint(tt.TrueType, depth+1)
		fb := c.baseConstraint(tt.FalseType, depth+1)
		if tb == nil || fb == nil {
			return nil
		}
		return types.NewUnionType(tb, fb)
	case *types.UnionType:
		members := make([]types.Type, len(tt.Types))
		changed := false
		for i, m := range tt.Types {
			members[i] = c.baseConstraint(m, depth+1)
			if members[i] == nil {
				return nil
			}
			changed = changed || members[i] != m
		}
		if changed {
			return types.NewUnionType(members...)
		}
	}
	return t
}

// satisfiesTypeArgConstraint reports whether a type argument satisfies its
// parameter's constraint, directly or through its base constraint.
func (c *Checker) satisfiesTypeArgConstraint(arg, constraint types.Type) func() bool {
	return func() bool {
		// Computed when asked: a deferred check runs once the types the
		// base constraint goes through are declared.
		base := c.baseConstraint(arg, 0)
		if base == nil || c.isAssignableWithExpansion(arg, constraint) {
			return true
		}
		return base != arg && c.isAssignableWithExpansion(base, constraint)
	}
}

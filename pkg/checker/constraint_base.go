package checker

import (
	"reflect"

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
		if tt.Constraint != nil {
			return c.baseConstraint(tt.Constraint, depth+1)
		}
		return nil
	case *types.TypeParameterType:
		if tt.Parameter == nil {
			return t
		}
		var parts []types.Type
		constraint := tt.Parameter.Constraint
		if c.apparentDepth > 0 {
			constraint = c.constraintOf(tt.Parameter)
		}
		if constraint != nil {
			parts = append(parts, constraint)
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
		if r := c.indexedMappedConstraint(t); r != t {
			return c.baseConstraint(r, depth+1)
		}
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
		// Opts[keyof T] (T[K] of `[K in keyof T]` with T extends Opts) reads
		// Opts's string index signature.
		if _, isKeyof := idx.(*types.KeyofType); isKeyof {
			if o, ok := obj.(*types.ObjectType); ok {
				for _, sig := range o.IndexSigs() {
					if sig.KeyType == types.String {
						return sig.ValueType
					}
				}
			}
		}
		return &types.IndexedAccessType{ObjectType: obj, IndexType: idx}
	case *types.ConditionalType:
		// tsc's default constraint of a conditional type: the true branch
		// with its `infer` placeholders inferred from the check type's
		// constraint, and a branch that is any leaves the other alone
		// (`TOptions['errorFormatter'] extends ErrorFormatter<any, infer S>
		// ? S : DefaultErrorShape` against an ErrorFormatter<C, any>).
		trueType := tt.TrueType
		if !mentionsInfer(trueType) {
			// Nothing to infer (and inferring is costly).
		} else if inferred, done := c.conditionalTrueConstraints[tt]; done {
			trueType = inferred
		} else if check := c.baseConstraint(tt.CheckType, depth+1); check != nil && check != tt.CheckType {
			members := []types.Type{check}
			if u, ok := check.(*types.UnionType); ok {
				members = u.Types
			}
			inferences := map[string]types.Type{}
			for _, m := range members {
				c.inferFromConditional(m, tt.ExtendsType, inferences)
			}
			if len(inferences) > 0 {
				trueType = c.substituteInferredTypes(trueType, inferences)
				// Kept once something is inferred: a check type whose
				// constraint is not known yet (an import cycle still
				// declaring it) is asked again later.
				if c.conditionalTrueConstraints == nil {
					c.conditionalTrueConstraints = map[*types.ConditionalType]types.Type{}
				}
				c.conditionalTrueConstraints[tt] = trueType
			}
		}
		var tb types.Type
		if (trueType == tt.CheckType || trueType.Equals(tt.CheckType)) && !mentionsInfer(tt.ExtendsType) {
			// The check type in its own true branch is known to extend the
			// extends type (tsc's substitution type): `V extends Proc ? V`
			// is a Proc.
			tb = c.baseConstraint(tt.ExtendsType, depth+1)
			if check := c.baseConstraint(tt.CheckType, depth+1); check != nil && tb != nil && check != types.Unknown {
				tb = types.NewIntersectionType(check, tb)
			}
		} else {
			tb = c.baseConstraint(trueType, depth+1)
		}
		fb := c.baseConstraint(tt.FalseType, depth+1)
		if tb == nil || fb == nil {
			return nil
		}
		if tb == types.Any {
			return fb
		}
		if fb == types.Any {
			return tb
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

// apparentContextualType is a contextual type with its conditional and type
// parameter members replaced by their base constraints (when known), as
// tsc's getApparentTypeOfContextualType maps them.
func (c *Checker) apparentContextualType(t types.Type) types.Type {
	switch tt := t.(type) {
	case *types.ConditionalType, *types.TypeParameterType:
		// Only here (and for an uninferred type parameter) are the call's
		// outer-substituted constraints used: relating with them is costly.
		c.apparentDepth++
		base := c.baseConstraint(t, 0)
		c.apparentDepth--
		if base != nil && base != types.Unknown {
			return base
		}
	case *types.UnionType:
		members := make([]types.Type, len(tt.Types))
		changed := false
		for i, m := range tt.Types {
			members[i] = c.apparentContextualType(m)
			changed = changed || members[i] != m
		}
		if changed {
			return types.NewUnionType(members...)
		}
	}
	return t
}

// conditionalMembersConstrained replaces the conditional members of an
// intersection with their base constraints, the apparent type a member is
// read from: `InstanceType<Response> & { req: ... }` (@types/node's
// RequestListener) while Response is not inferred yet reads Response's
// constraint's instances.
func (c *Checker) conditionalMembersConstrained(t types.Type) types.Type {
	it, ok := t.(*types.IntersectionType)
	if !ok {
		return t
	}
	members := make([]types.Type, len(it.Types))
	changed := false
	for i, m := range it.Types {
		members[i] = m
		if _, isCond := m.(*types.ConditionalType); isCond {
			if base := c.baseConstraint(m, 0); base != nil && base != types.Unknown {
				members[i] = base
				changed = true
			}
		}
	}
	if !changed {
		return t
	}
	return types.NewIntersectionType(members...)
}

// outerConstraintKey is a type parameter and the outer substitution (by map
// identity) its constraint is substituted by.
type outerConstraintKey struct {
	tp    *types.TypeParameter
	outer uintptr
}

// constraintOf is tp's constraint, substituted by the outer substitution of
// the call being inferred when tp is that call's own type parameter (a method
// of an instantiated generic type: Signature.OuterSubstitution).
func (c *Checker) constraintOf(tp *types.TypeParameter) types.Type {
	if tp == nil || tp.Constraint == nil {
		return nil
	}
	if outer := c.callOuterSubstitution[tp]; outer != nil {
		key := outerConstraintKey{tp, reflect.ValueOf(outer).Pointer()}
		if done, ok := c.outerConstraints[key]; ok {
			return done
		}
		if c.outerConstraints == nil {
			c.outerConstraints = map[outerConstraintKey]types.Type{}
		}
		c.outerConstraints[key] = tp.Constraint // while substituting
		substituted := c.substituteTypes(tp.Constraint, outer)
		c.outerConstraints[key] = substituted
		return substituted
	}
	return tp.Constraint
}

// withCallOuterSubstitution runs fn with sig's type parameters' constraints
// substituted by its outer substitution.
func (c *Checker) withCallOuterSubstitution(sig *types.Signature, fn func()) {
	if sig == nil || sig.OuterSubstitution == nil {
		fn()
		return
	}
	saved := c.callOuterSubstitution
	c.callOuterSubstitution = make(map[*types.TypeParameter]map[string]types.Type, len(saved)+len(sig.TypeParameters))
	for tp, outer := range saved {
		c.callOuterSubstitution[tp] = outer
	}
	for _, tp := range sig.TypeParameters {
		c.callOuterSubstitution[tp] = sig.OuterSubstitution
	}
	defer func() { c.callOuterSubstitution = saved }()
	fn()
}

// indexedAccessConstraint is the relation's IndexedConstraint: ia's base
// constraint, worked out once at a time (computing it relates types too).
func (c *Checker) indexedAccessConstraint(ia *types.IndexedAccessType) types.Type {
	if c.inIndexedConstraint {
		return nil
	}
	c.inIndexedConstraint = true
	defer func() { c.inIndexedConstraint = false }()
	return c.baseConstraint(ia, 0)
}

// suspendIndexedConstraint keeps the relation from relating indexed accesses
// through their constraints until the returned func runs: a conditional
// type is decided for every instantiation (tsc's restrictive instantiation),
// so `TRoot['transformer'] extends true` stays deferred for TRoot extends
// { transformer: any }.
func (c *Checker) suspendIndexedConstraint() func() {
	saved := c.inIndexedConstraint
	c.inIndexedConstraint = true
	return func() { c.inIndexedConstraint = saved }
}

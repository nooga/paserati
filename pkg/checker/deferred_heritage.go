package checker

import (
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// An interface may extend a member of a namespace declared further down the
// file (`interface EventEmitter<T> extends NodeJS.EventEmitter<T> {}` above
// @types/node's `declare global { namespace NodeJS { ... } }`). tsc binds
// every declaration before resolving any; Pass 1 here declares in source
// order, so such a base is resolved once Pass 1 is done, in the scope it was
// written in.

type deferredHeritage struct {
	target *types.ObjectType
	expr   parser.Expression
	env    *Environment
	// generic is the generic interface whose body target is: its instances
	// made meanwhile get the new members too.
	generic *types.GenericType
}

// deferQualifiedBase queues expr, a base of target, when it is qualified by
// a name nothing declares yet. It reports whether it did.
func (c *Checker) deferQualifiedBase(target *types.ObjectType, generic *types.GenericType, expr parser.Expression) bool {
	if c.resolvingDeferredHeritage {
		return false
	}
	root := qualifierRoot(expr)
	if root == nil {
		return false
	}
	if _, found := c.env.ResolveTypeParameter(root.Value); found {
		return false
	}
	if rootType, found := c.env.ResolveType(root.Value); found {
		// A namespace declared already may still gain the member further
		// down (a global NodeJS namespace merged into by a later block).
		if _, isNS := rootType.(*types.NamespaceType); !isNS || c.resolveQualifiedTypeQuietly(qualifiedName(expr)) != nil {
			return false
		}
	} else if _, _, found := c.env.Resolve(root.Value); found {
		return false
	}
	c.deferredHeritages = append(c.deferredHeritages, deferredHeritage{target: target, expr: expr, env: c.env, generic: generic})
	return true
}

// qualifierRoot is the leftmost name of a qualified type reference (N in
// N.M.T or N.M.T<X>), or nil when expr is not qualified.
func qualifierRoot(expr parser.Expression) *parser.Identifier {
	switch e := expr.(type) {
	case *parser.GenericTypeRef:
		if e.Qualifier == nil {
			return nil
		}
		return leftmostIdentifier(e.Qualifier)
	case *parser.MemberExpression:
		return leftmostIdentifier(e)
	}
	return nil
}

// qualifiedName is the qualified name of a type reference, without type
// arguments: N.M.T for N.M.T<X>.
func qualifiedName(expr parser.Expression) *parser.MemberExpression {
	switch e := expr.(type) {
	case *parser.GenericTypeRef:
		return &parser.MemberExpression{Object: e.Qualifier, Property: e.Name}
	case *parser.MemberExpression:
		return e
	}
	return nil
}

func leftmostIdentifier(expr parser.Expression) *parser.Identifier {
	for {
		switch e := expr.(type) {
		case *parser.Identifier:
			return e
		case *parser.MemberExpression:
			expr = e.Object
		default:
			return nil
		}
	}
}

// resolveDeferredHeritages gives each queued interface the members of its
// base that it does not declare itself.
func (c *Checker) resolveDeferredHeritages() {
	pending := c.deferredHeritages
	c.deferredHeritages = nil
	if len(pending) == 0 {
		return
	}
	savedEnv, savedThis := c.env, c.currentThisType
	c.resolvingDeferredHeritage = true
	defer func() {
		c.env, c.currentThisType = savedEnv, savedThis
		c.resolvingDeferredHeritage = false
	}()
	for _, d := range pending {
		c.env, c.currentThisType = d.env, d.target
		base, ok := c.resolveExtendedInterfaceObjectType(c.resolveTypeAnnotation(d.expr))
		if !ok || base == nil {
			continue
		}
		c.recordDerived(base, d.target)
		c.keepClassBaseForAccess(d.target, base)
		rb := newThisRebind(base, d.target)
		forEachEffectiveMember(base, func(owner *types.ObjectType, name string) {
			if _, own := d.target.Props()[name]; own {
				return
			}
			d.target.SetProperty(name, c.rebindThisType(owner.Props()[name], rb))
			if d.target.ClassMeta != nil {
				// A class merged into the interface since (its instance type).
				d.target.ClassMeta.AddMember(name, types.AccessPublic, false, false)
			}
			d.target.SetPropertyDoc(name, owner.PropertyDoc(name))
			setReadonlyProperty(d.target, name, owner.ReadOnlyProps()[name])
			if owner.OptionalProps()[name] {
				d.target.OptionalProps()[name] = true
			}
		})
		if d.generic != nil {
			c.refillInstances(d.generic)
		}
	}
}

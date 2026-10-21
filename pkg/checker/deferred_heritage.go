package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// An interface may extend an interface declared further down the file, or a
// member of a namespace declared further down (`interface EventEmitter<T>
// extends NodeJS.EventEmitter<T> {}` above @types/node's `declare global {
// namespace NodeJS { ... } }`). tsc binds every declaration before resolving
// any; Pass 1 here declares in source order, so such a base is resolved
// once Pass 1 is done, in the scope it was written in.

type deferredHeritage struct {
	node   *parser.InterfaceDeclaration
	index  int // of the base in node.Extends
	target *types.ObjectType
	env    *Environment
	// generic is the generic interface whose body target is: its instances
	// made meanwhile get the new members too.
	generic *types.GenericType
}

// deferUndeclaredBase queues the index'th base of node when it names an
// interface, class or namespace member not declared yet. It reports whether
// it did.
func (c *Checker) deferUndeclaredBase(node *parser.InterfaceDeclaration, index int, target *types.ObjectType, generic *types.GenericType) bool {
	if c.resolvingDeferredHeritage || !c.baseNotDeclaredYet(node.Extends[index]) {
		return false
	}
	c.deferredHeritages = append(c.deferredHeritages, deferredHeritage{node: node, index: index, target: target, env: c.env, generic: generic})
	return true
}

// baseNotDeclaredYet reports whether the type reference expr names nothing
// declared so far: an unknown name (or one only pre-registered in Pass 0),
// or a namespace without the member yet.
func (c *Checker) baseNotDeclaredYet(expr parser.Expression) bool {
	if root := qualifierRoot(expr); root != nil {
		if _, found := c.env.ResolveTypeParameter(root.Value); found {
			return false
		}
		if rootType, found := c.env.ResolveType(root.Value); found {
			// A namespace declared already may still gain the member further
			// down (a global NodeJS namespace merged into by a later block).
			_, isNS := rootType.(*types.NamespaceType)
			return isNS && c.resolveQualifiedTypeQuietly(qualifiedName(expr)) == nil &&
				c.env.ResolveMergedNamespace(root.Value) == nil
		}
		_, _, found := c.env.Resolve(root.Value)
		return !found
	}
	var name string
	switch e := expr.(type) {
	case *parser.Identifier:
		name = e.Value
	case *parser.GenericTypeRef:
		name = e.Name.Value
	default:
		return false
	}
	if _, found := c.env.ResolveTypeParameter(name); found {
		return false
	}
	if t, found := c.env.ResolveType(name); found {
		obj, isObj := t.(*types.ObjectType)
		return isObj && obj.Pending
	}
	_, _, found := c.env.Resolve(name)
	return !found
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

// resolveDeferredHeritages gives each queued interface what its base
// brings: the members it does not declare itself, the base's call, construct
// and index signatures, and the TS2430 check against it.
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
		expr := d.node.Extends[d.index]
		resolved := c.resolveTypeAnnotation(expr)
		if resolved == nil {
			continue
		}
		base, ok := c.resolveExtendedInterfaceObjectType(resolved)
		if !ok || base == nil {
			if _, isAny := resolved.(*types.Primitive); !isAny {
				c.reportInvalidInterfaceBase(expr, resolved)
			}
			continue
		}
		c.inheritDeferredBase(d, base)
		if d.generic != nil {
			c.refillInstances(d.generic)
		}
	}
}

func (c *Checker) inheritDeferredBase(d deferredHeritage, base *types.ObjectType) {
	target := d.target
	c.recordDerived(base, target)
	c.keepClassBaseForAccess(target, base)
	rb := newThisRebind(base, target)
	forEachEffectiveMember(base, func(owner *types.ObjectType, name string) {
		if _, own := target.Props()[name]; own {
			return
		}
		target.SetProperty(name, c.rebindThisType(owner.Props()[name], rb))
		if target.ClassMeta != nil {
			// A class merged into the interface since (its instance type).
			target.ClassMeta.AddMember(name, types.AccessPublic, false, false)
		}
		target.SetPropertyDoc(name, owner.PropertyDoc(name))
		setReadonlyProperty(target, name, owner.ReadOnlyProps()[name])
		if owner.OptionalProps()[name] {
			target.OptionalProps()[name] = true
		}
	})
	target.Materialize()
	target.IndexSignatures = inheritIndexSignatures(target.IndexSigs(), []*types.ObjectType{base})
	target.CallSignatures = appendNewSignatures(target.CallSigs(), base.CallSigs())
	target.ConstructSignatures = appendNewSignatures(target.ConstructSigs(), base.ConstructSigs())
	c.checkDeferredInterfaceExtends(d, base)
}

// checkDeferredInterfaceExtends is checkInterfaceExtends for one deferred
// base.
func (c *Checker) checkDeferredInterfaceExtends(d deferredHeritage, base *types.ObjectType) {
	node := d.node
	// A class merged into the interface since brings members of its own,
	// which are not the declaration's.
	if node.Name == nil || c.skipValidation() || d.target.ClassMeta != nil {
		return
	}
	if !redeclaresNonPublicMember(node, base) && c.isAssignableWithExpansion(membersView(d.target), membersView(base)) {
		return
	}
	if ref, ok := node.Extends[d.index].(*parser.GenericTypeRef); ok && typeArgsReference(ref.TypeArguments, node.Name.Value) {
		return
	}
	c.addErrorAtStart(node.Name, errors.TS2430, fmt.Sprintf(
		"Interface '%s' incorrectly extends interface '%s'.", interfaceDisplayName(node), node.Extends[d.index].String()))
}

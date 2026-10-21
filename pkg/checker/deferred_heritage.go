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
	if c.resolvingDeferredHeritage || !c.baseNotDeclaredYet(node.Extends[index]) && !c.baseAwaitsDeferred(node.Extends[index]) {
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
		if c.awaitsCycleImport(name, t) {
			return true
		}
		obj, isObj := t.(*types.ObjectType)
		return isObj && obj.Pending
	}
	_, _, found := c.env.Resolve(name)
	return !found
}

// awaitsCycleImport reports whether the type name names, t, is an import
// from a module in an import cycle that has not declared it yet.
func (c *Checker) awaitsCycleImport(name string, t types.Type) bool {
	if c.moduleEnv == nil {
		return false
	}
	if b, ok := c.moduleEnv.ImportedNames[name]; !ok || !b.Early {
		return false
	}
	switch tt := t.(type) {
	case *types.GenericTypeAliasForwardReference:
		if tt.Scope == nil {
			return false
		}
		resolved, _ := tt.Scope(tt.AliasName)
		return resolved == types.Any
	case *types.ObjectType:
		return tt.Pending
	}
	return false
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
	// A base that is itself waiting for a base of its own goes first, so an
	// interface copies its base's members once they are all there
	// (ZString extends ZType<string> extends Base<string>, declared bottom
	// up). A base waiting for an import cycle keeps what extends it waiting.
	waiting := make(map[*types.ObjectType]bool, len(pending))
	for _, d := range pending {
		waiting[d.target] = true
	}
	order := make([]deferredHeritage, 0, len(pending))
	held := map[*types.ObjectType]bool{}
	done := make([]bool, len(pending))
	for progress := true; progress; {
		progress = false
		for i, d := range pending {
			if done[i] {
				continue
			}
			c.env = d.env
			expr := d.node.Extends[d.index]
			if base := c.heritageBaseObject(expr); base != nil && base != d.target && waiting[base] && !held[base] {
				continue
			}
			done[i], progress = true, true
			if c.baseNotDeclaredYet(expr) && c.basesAwaitCycle(expr) || c.heritageHeld(expr, held) {
				// Declared by a module in an import cycle that has not got
				// to it yet: resolved once this module's imports are (see
				// refreshEarlyImports).
				held[d.target] = true
				c.deferredHeritages = append(c.deferredHeritages, d)
				continue
			}
			order = append(order, d)
			waiting[d.target] = false
		}
	}
	for i, d := range pending {
		if !done[i] {
			order = append(order, d) // bases extending each other: as written
		}
	}
	for _, d := range order {
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

// basesAwaitCycle reports whether the base expr names an import still
// awaited from a module in an import cycle.
func (c *Checker) basesAwaitCycle(expr parser.Expression) bool {
	name := ""
	if root := qualifierRoot(expr); root != nil {
		name = root.Value
	} else if id, ok := expr.(*parser.Identifier); ok {
		name = id.Value
	} else if ref, ok := expr.(*parser.GenericTypeRef); ok {
		name = ref.Name.Value
	}
	if name == "" || c.moduleEnv == nil {
		return false
	}
	b, ok := c.moduleEnv.ImportedNames[name]
	return ok && b.Early
}

// heritageBaseObject is the interface (or generic interface's body) the
// unqualified base expr names, or nil.
func (c *Checker) heritageBaseObject(expr parser.Expression) *types.ObjectType {
	var name string
	switch e := expr.(type) {
	case *parser.Identifier:
		name = e.Value
	case *parser.GenericTypeRef:
		if e.Qualifier != nil {
			return nil
		}
		name = e.Name.Value
	default:
		return nil
	}
	t, found := c.env.ResolveType(name)
	if !found {
		return nil
	}
	switch tt := t.(type) {
	case *types.GenericType:
		obj, _ := tt.Body.(*types.ObjectType)
		return obj
	case *types.ObjectType:
		return tt
	}
	return nil
}

// heritageHeld reports whether the base expr names is held back for an
// import cycle.
func (c *Checker) heritageHeld(expr parser.Expression, held map[*types.ObjectType]bool) bool {
	base := c.heritageBaseObject(expr)
	return base != nil && held[base]
}

// baseAwaitsDeferred reports whether the base expr names is itself still
// waiting for a base of its own: copying its members now would miss those.
func (c *Checker) baseAwaitsDeferred(expr parser.Expression) bool {
	if len(c.deferredHeritages) == 0 {
		return false
	}
	base := c.heritageBaseObject(expr)
	if base == nil {
		return false
	}
	for _, d := range c.deferredHeritages {
		if d.target == base {
			return true
		}
	}
	return false
}

package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// checkClassMembersAgainstBase reports TS2416 for an instance member whose
// type is not assignable to the member of the same name in the base class
// (checkKindsOfPropertyMemberOverrides / checkTypeAssignableTo on the
// property types). Private members and accessors are left alone.
func (c *Checker) checkClassMembersAgainstBase(className string, body *parser.ClassBody, instanceType *types.ObjectType) {
	if body == nil || instanceType == nil || len(instanceType.Bases()) == 0 {
		return
	}
	baseObj, ok := c.resolveStructural(instanceType.Bases()[0]).(*types.ObjectType)
	if !ok || baseObj.ClassMeta == nil {
		return
	}
	baseName := baseObj.ClassMeta.ClassName
	if baseName == "" {
		baseName = baseObj.String()
	}
	baseProps := baseObj.GetEffectiveProperties()

	check := func(key parser.Expression, isStatic, isPrivate bool, ownOptional bool) {
		if isStatic || isPrivate || key == nil {
			return
		}
		ident, isIdent := key.(*parser.Identifier)
		if !isIdent {
			return
		}
		name := ident.Value
		if len(name) > 0 && name[0] == '#' {
			return // #private names are scoped to their own class
		}
		baseType, exists := baseProps[name]
		if !exists || baseType == nil {
			return
		}
		ownType, ok := instanceType.Props()[name]
		if !ok || ownType == nil {
			return
		}
		ownType = c.resolveTypeofDeep(ownType, 0)
		baseType = c.resolveTypeofDeep(baseType, 0)
		if c.rel.StrictNullChecks && ownOptional && !baseObj.IsPropertyOptional(name) {
			ownType = types.NewUnionType(ownType, types.Undefined)
		}
		if c.isAssignableWithExpansion(ownType, baseType) {
			return
		}
		c.addErrorAtStart(ident, errors.TS2416, fmt.Sprintf(
			"Property '%s' in type '%s' is not assignable to the same property in base type '%s'.",
			name, className, baseName))
	}

	for _, prop := range body.Properties {
		check(prop.Key, prop.IsStatic, prop.IsPrivate, prop.Optional)
	}
	for _, method := range body.Methods {
		if method.Kind != "method" {
			continue
		}
		check(method.Key, method.IsStatic, method.IsPrivate, false)
	}
}

// deferredRelationCheck is a relation check postponed until all declarations
// of the program have been resolved.
type deferredRelationCheck struct {
	env *Environment
	run func()
}

// deferRelationCheck registers a check to run once the main passes are done.
func (c *Checker) deferRelationCheck(run func()) {
	if c.skipValidation() {
		return
	}
	c.deferredRelationChecks = append(c.deferredRelationChecks, deferredRelationCheck{env: c.env, run: run})
}

// runDeferredRelationChecks runs and clears the deferred relation checks in
// the environment each was registered in.
func (c *Checker) runDeferredRelationChecks() {
	pending := c.deferredRelationChecks
	c.deferredRelationChecks = nil
	saved := c.env
	for _, check := range pending {
		c.env = check.env
		check.run()
	}
	c.env = saved
}

// resolveTypeofDeep replaces `typeof x` placeholders nested in function
// parameter, return, property, array and union positions by the declared type
// of x. Types without placeholders are returned unchanged.
func (c *Checker) resolveTypeofDeep(t types.Type, depth int) types.Type {
	if t == nil || depth > 6 {
		return t
	}
	switch tt := t.(type) {
	case *types.TypeofType:
		return c.resolveTypeofTypeIfNeeded(tt)
	case *types.ArrayType:
		if el := c.resolveTypeofDeep(tt.ElementType, depth+1); el != tt.ElementType {
			return &types.ArrayType{ElementType: el}
		}
	case *types.UnionType:
		changed := false
		members := make([]types.Type, len(tt.Types))
		for i, m := range tt.Types {
			members[i] = c.resolveTypeofDeep(m, depth+1)
			if members[i] != m {
				changed = true
			}
		}
		if changed {
			return types.NewUnionType(members...)
		}
	case *types.ObjectType:
		changed := false
		props := make(map[string]types.Type, len(tt.Props()))
		for name, pt := range tt.Props() {
			props[name] = c.resolveTypeofDeep(pt, depth+1)
			if props[name] != pt {
				changed = true
			}
		}
		resolveSigs := func(sigs []*types.Signature) []*types.Signature {
			out := make([]*types.Signature, len(sigs))
			for i, sig := range sigs {
				cp := *sig
				cp.ParameterTypes = make([]types.Type, len(sig.ParameterTypes))
				for j, pt := range sig.ParameterTypes {
					cp.ParameterTypes[j] = c.resolveTypeofDeep(pt, depth+1)
					if cp.ParameterTypes[j] != pt {
						changed = true
					}
				}
				cp.ReturnType = c.resolveTypeofDeep(sig.ReturnType, depth+1)
				if cp.ReturnType != sig.ReturnType {
					changed = true
				}
				out[i] = &cp
			}
			return out
		}
		calls := resolveSigs(tt.CallSigs())
		ctors := resolveSigs(tt.ConstructSigs())
		if changed {
			cp := *tt
			cp.Materialize()
			cp.Properties = props
			cp.Materialize()
			cp.CallSignatures = calls
			cp.Materialize()
			cp.ConstructSignatures = ctors
			return &cp
		}
	}
	return t
}

// memberAccessThroughBases returns the access level a class instance type
// gives a member, looking through its base types.
func (c *Checker) memberAccessThroughBases(obj *types.ObjectType, name string, depth int) (types.AccessModifier, bool) {
	if obj == nil || depth > 8 {
		return types.AccessPublic, false
	}
	if obj.ClassMeta != nil {
		if info := obj.ClassMeta.GetMemberAccess(name); info != nil && !info.IsStatic {
			return info.AccessLevel, true
		}
	}
	for _, base := range obj.Bases() {
		if bo, ok := c.resolveStructural(base).(*types.ObjectType); ok {
			if level, found := c.memberAccessThroughBases(bo, name, depth+1); found {
				return level, true
			}
		}
	}
	return types.AccessPublic, false
}

// checkClassHeritageAccessibility reports TS2415 when a derived class makes an
// inherited member less accessible than in its base class (a private member
// shadowing a public or protected one, a protected one shadowing a public
// one) or redeclares a private member of the base.
func (c *Checker) checkClassHeritageAccessibility(node *parser.ClassDeclaration, instanceType *types.ObjectType) {
	if node == nil || node.Name == nil || node.Body == nil || instanceType == nil || len(instanceType.Bases()) == 0 || instanceType.ClassMeta == nil {
		return
	}
	baseObj, ok := c.resolveStructural(instanceType.Bases()[0]).(*types.ObjectType)
	if !ok || baseObj.ClassMeta == nil {
		return
	}
	baseName := baseObj.ClassMeta.ClassName
	if baseName == "" {
		baseName = node.SuperClass.String()
	}
	incompatible := func(name string) bool {
		if len(name) > 0 && name[0] == '#' {
			return false // #private names are scoped to their own class
		}
		own := instanceType.ClassMeta.GetMemberAccess(name)
		if own == nil || own.IsStatic {
			return false
		}
		baseLevel, found := c.memberAccessThroughBases(baseObj, name, 0)
		if !found {
			return false
		}
		switch {
		case baseLevel == types.AccessPrivate:
			return true // separate declarations of a private property
		case baseLevel == types.AccessProtected && own.AccessLevel == types.AccessPrivate:
			return true
		case baseLevel == types.AccessPublic && own.AccessLevel != types.AccessPublic:
			return true
		}
		return false
	}
	report := func() {
		c.addErrorAtStart(node.Name, errors.TS2415, fmt.Sprintf(
			"Class '%s' incorrectly extends base class '%s'.", node.Name.Value, baseName))
	}
	for _, prop := range node.Body.Properties {
		if id, ok := prop.Key.(*parser.Identifier); ok && !prop.IsStatic && incompatible(id.Value) {
			report()
			return
		}
	}
	for _, method := range node.Body.Methods {
		if id, ok := method.Key.(*parser.Identifier); ok && !method.IsStatic && method.Kind != "constructor" && incompatible(id.Value) {
			report()
			return
		}
	}
}

// checkClassStaticHeritageAccessibility is TS2415's static side, TS2417:
// a static member less accessible than the base class static member it
// shadows, or a redeclared private static member.
func (c *Checker) checkClassStaticHeritageAccessibility(node *parser.ClassDeclaration, instanceType *types.ObjectType) {
	if node == nil || node.Name == nil || node.Body == nil || instanceType == nil || len(instanceType.Bases()) == 0 {
		return
	}
	baseObj, ok := c.resolveStructural(instanceType.Bases()[0]).(*types.ObjectType)
	if !ok || baseObj.ClassMeta == nil {
		return
	}
	baseName := baseObj.ClassMeta.ClassName
	if baseName == "" {
		return
	}
	baseIdent, isIdent := node.SuperClass.(*parser.Identifier)
	if !isIdent {
		return
	}
	baseCtorType, _, found := c.env.Resolve(baseIdent.Value)
	if !found {
		return
	}
	baseCtor, isObj := baseCtorType.(*types.ObjectType)
	if !isObj {
		return
	}
	baseStaticLevel := func(name string) (types.AccessModifier, bool) {
		if info := baseCtor.GetMemberAccessInfo(name); info != nil {
			return info.AccessLevel, true
		}
		return types.AccessPublic, false
	}
	incompatible := func(name string, private, protected bool) bool {
		if len(name) > 0 && name[0] == '#' {
			return false
		}
		baseLevel, found := baseStaticLevel(name)
		if !found {
			return false
		}
		switch {
		case baseLevel == types.AccessPrivate:
			return true
		case baseLevel == types.AccessProtected && private:
			return true
		case baseLevel == types.AccessPublic && (private || protected):
			return true
		}
		return false
	}
	report := func() {
		c.addErrorAtStart(node.Name, errors.TS2417, fmt.Sprintf(
			"Class static side 'typeof %s' incorrectly extends base class static side 'typeof %s'.", node.Name.Value, baseName))
	}
	for _, prop := range node.Body.Properties {
		if id, ok := prop.Key.(*parser.Identifier); ok && prop.IsStatic && incompatible(id.Value, prop.IsPrivate, prop.IsProtected) {
			report()
			return
		}
	}
	for _, method := range node.Body.Methods {
		if id, ok := method.Key.(*parser.Identifier); ok && method.IsStatic && incompatible(id.Value, method.IsPrivate, method.IsProtected) {
			report()
			return
		}
	}
}

// classMemberKind is how a class declares an instance member: as a property
// or as an accessor (get/set or `accessor`), and whether it is abstract.
type classMemberKind struct {
	accessor bool
	abstract bool
	name     *parser.Identifier
}

func classMemberName(key parser.Expression) (string, *parser.Identifier) {
	switch k := key.(type) {
	case *parser.Identifier:
		if len(k.Value) > 0 && k.Value[0] == '#' {
			return "", nil
		}
		return k.Value, k
	case *parser.StringLiteral:
		return k.Value, nil
	}
	return "", nil
}

// recordClassMemberKinds notes the kind of each public or protected
// instance member body declares, for checkPropertyAccessorOverrides.
func (c *Checker) recordClassMemberKinds(body *parser.ClassBody, instanceType *types.ObjectType) {
	if body == nil || instanceType == nil || instanceType.ClassMeta == nil {
		return
	}
	kinds := map[string]classMemberKind{}
	add := func(key parser.Expression, k classMemberKind) {
		name, ident := classMemberName(key)
		if name == "" {
			return
		}
		if _, seen := kinds[name]; seen {
			return
		}
		k.name = ident
		kinds[name] = k
	}
	for _, prop := range body.Properties {
		if prop != nil && !prop.IsStatic && !prop.IsPrivate {
			add(prop.Key, classMemberKind{accessor: prop.IsAccessor, abstract: prop.IsAbstract})
		}
	}
	for _, m := range body.Methods {
		if m != nil && !m.IsStatic && !m.IsPrivate && (m.Kind == "getter" || m.Kind == "setter") {
			add(m.Key, classMemberKind{accessor: true})
		}
	}
	for _, m := range body.MethodSigs {
		if m != nil && !m.IsStatic && !m.IsPrivate && (m.Kind == "getter" || m.Kind == "setter") {
			add(m.Key, classMemberKind{accessor: true, abstract: m.IsAbstract})
		}
	}
	if c.classMemberKinds == nil {
		c.classMemberKinds = map[*types.ClassMetadata]map[string]classMemberKind{}
	}
	c.classMemberKinds[instanceType.ClassMeta] = kinds
}

// checkPropertyAccessorOverrides reports a property overriding a base
// accessor (TS2610) and an accessor overriding a base property (TS2611);
// abstract base members may be overridden either way.
func (c *Checker) checkPropertyAccessorOverrides(className string, body *parser.ClassBody, instanceType *types.ObjectType) {
	if body == nil || instanceType == nil || instanceType.ClassMeta == nil || len(instanceType.Bases()) == 0 {
		return
	}
	own := c.classMemberKinds[instanceType.ClassMeta]
	baseObj, ok := c.resolveStructural(instanceType.Bases()[0]).(*types.ObjectType)
	if !ok || baseObj.ClassMeta == nil {
		return
	}
	baseName := baseObj.ClassMeta.ClassName
	if baseName == "" {
		return
	}
	lookup := func(name string) (classMemberKind, bool) {
		obj := baseObj
		for depth := 0; obj != nil && obj.ClassMeta != nil && depth < 32; depth++ {
			if k, ok := c.classMemberKinds[obj.ClassMeta][name]; ok {
				return k, true
			}
			if obj.ClassMeta.GetMemberAccess(name) != nil || len(obj.Bases()) == 0 {
				return classMemberKind{}, false
			}
			obj, _ = c.resolveStructural(obj.Bases()[0]).(*types.ObjectType)
		}
		return classMemberKind{}, false
	}
	for name, k := range own {
		if k.name == nil || k.abstract {
			continue
		}
		base, ok := lookup(name)
		if !ok || base.abstract || base.accessor == k.accessor {
			continue
		}
		if !k.accessor {
			c.addErrorAtStart(k.name, errors.TS2610, fmt.Sprintf(
				"'%s' is defined as an accessor in class '%s', but is overridden here in '%s' as an instance property.",
				name, baseName, className))
		} else {
			c.addErrorAtStart(k.name, errors.TS2611, fmt.Sprintf(
				"'%s' is defined as a property in class '%s', but is overridden here in '%s' as an accessor.",
				name, baseName, className))
		}
	}
}

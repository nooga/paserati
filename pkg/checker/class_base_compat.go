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
	if body == nil || instanceType == nil || len(instanceType.BaseTypes) == 0 {
		return
	}
	baseObj, ok := c.resolveStructural(instanceType.BaseTypes[0]).(*types.ObjectType)
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
		ownType, ok := instanceType.Properties[name]
		if !ok || ownType == nil {
			return
		}
		ownType = c.resolveTypeofDeep(ownType, 0)
		baseType = c.resolveTypeofDeep(baseType, 0)
		if types.StrictNullChecks && ownOptional && !baseObj.IsPropertyOptional(name) {
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
		props := make(map[string]types.Type, len(tt.Properties))
		for name, pt := range tt.Properties {
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
		calls := resolveSigs(tt.CallSignatures)
		ctors := resolveSigs(tt.ConstructSignatures)
		if changed {
			cp := *tt
			cp.Properties = props
			cp.CallSignatures = calls
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
	for _, base := range obj.BaseTypes {
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
	if node == nil || node.Name == nil || node.Body == nil || instanceType == nil || len(instanceType.BaseTypes) == 0 || instanceType.ClassMeta == nil {
		return
	}
	baseObj, ok := c.resolveStructural(instanceType.BaseTypes[0]).(*types.ObjectType)
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

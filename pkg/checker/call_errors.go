package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// reportNotCallable reports the diagnostic TypeScript gives when the callee of
// a call expression has no call signatures. It is reported at the callee.
func (c *Checker) reportNotCallable(node *parser.CallExpression, calleeType types.Type) {
	callee := leftmostExpression(node.Function)
	if calleeType == types.Unknown && c.strictNullChecks {
		if name := entityNameText(node.Function); name != "" && len(name) < 100 {
			c.addErrorWithCode(callee, errors.TS18046, fmt.Sprintf("'%s' is of type 'unknown'.", name))
		} else {
			c.addErrorWithCode(callee, errors.TS2571, "Object is of type 'unknown'.")
		}
		return
	}
	if obj, ok := calleeType.(*types.ObjectType); ok && len(obj.ConstructSigs()) > 0 && len(obj.CallSigs()) == 0 {
		c.addErrorWithCode(callee, errors.TS2348, fmt.Sprintf("Value of type '%s' is not callable. Did you mean to include 'new'?", calleeType.String()))
		return
	}
	if c.calleeIsGetAccessor(node) {
		c.addErrorWithCode(callee, errors.TS6234, "This expression is not callable because it is a 'get' accessor. Did you mean to use it without '()'?")
		return
	}
	c.addErrorWithCode(callee, errors.TS2349, "This expression is not callable.")
}

// calleeIsGetAccessor mirrors invocationErrorDetails: a call without
// arguments whose callee is `obj.name`, with name a class get accessor.
func (c *Checker) calleeIsGetAccessor(node *parser.CallExpression) bool {
	if len(node.Arguments) != 0 {
		return false
	}
	me, ok := node.Function.(*parser.MemberExpression)
	if !ok {
		return false
	}
	prop, ok := me.Property.(*parser.Identifier)
	if !ok || me.Object == nil || me.Object.GetComputedType() == nil {
		return false
	}
	obj, _ := c.resolveStructural(me.Object.GetComputedType()).(*types.ObjectType)
	if obj == nil {
		return false
	}
	info := obj.GetMemberAccessInfo(prop.Value)
	return info != nil && info.IsGetter
}

// stripNullishCallee mirrors TypeScript's checkNonNullType on the callee of a
// call: a null/undefined member of the callee type is reported (TS2721/2722/2723)
// and removed so the rest of the call is checked against what remains.
func (c *Checker) stripNullishCallee(node *parser.CallExpression, calleeType types.Type) types.Type {
	if !c.strictNullChecks {
		return calleeType
	}
	callee := leftmostExpression(node.Function)
	if calleeType == types.Undefined {
		c.addErrorWithCode(callee, errors.TS2722, "Cannot invoke an object which is possibly 'undefined'.")
		return types.Any
	}
	if calleeType == types.Null {
		c.addErrorWithCode(callee, errors.TS2721, "Cannot invoke an object which is possibly 'null'.")
		return types.Any
	}
	union, ok := calleeType.(*types.UnionType)
	if !ok {
		return calleeType
	}
	hasNull, hasUndefined := false, false
	var rest []types.Type
	for _, member := range union.Types {
		switch member {
		case types.Null:
			hasNull = true
		case types.Undefined:
			hasUndefined = true
		default:
			rest = append(rest, member)
		}
	}
	if !hasNull && !hasUndefined {
		return calleeType
	}
	switch {
	case hasNull && hasUndefined:
		c.addErrorWithCode(callee, errors.TS2723, "Cannot invoke an object which is possibly 'null' or 'undefined'.")
	case hasUndefined:
		c.addErrorWithCode(callee, errors.TS2722, "Cannot invoke an object which is possibly 'undefined'.")
	default:
		c.addErrorWithCode(callee, errors.TS2721, "Cannot invoke an object which is possibly 'null'.")
	}
	if len(rest) == 0 {
		return types.Any
	}
	return types.NewUnionType(rest...)
}

// reportSuperWithoutBase reports a `super` reference in a class that has no
// resolved base class. A class with no extends clause gets TS2335; `extends
// null` gets TS17005 for a super call. A class whose extends clause failed to
// resolve already has its own diagnostic, so nothing more is reported.
func (c *Checker) reportSuperWithoutBase(superExpr parser.Node, classType *types.ObjectType, isCall bool) {
	meta := classType.ClassMeta
	switch {
	case meta != nil && meta.ExtendsNull:
		if isCall {
			c.addErrorWithCode(superExpr, errors.TS17005, "A constructor cannot contain a 'super' call when its class extends 'null'.")
		}
	case meta != nil && meta.HasExtendsClause:
		// extends clause already reported
	default:
		c.addErrorWithCode(superExpr, errors.TS2335, "'super' can only be referenced in a derived class.")
	}
}

// memberKindsOf returns the member kinds recorded for a class. Instantiating a
// generic class copies its ClassMetadata, so a copy is matched to the recorded
// original by declaration site.
func (c *Checker) memberKindsOf(meta *types.ClassMetadata) map[string]classMemberKind {
	if kinds, ok := c.classMemberKinds[meta]; ok {
		return kinds
	}
	if meta.DeclEnd == 0 {
		return nil
	}
	for m, kinds := range c.classMemberKinds {
		if m.ClassName == meta.ClassName && m.DeclFile == meta.DeclFile && m.DeclStart == meta.DeclStart && m.DeclEnd == meta.DeclEnd {
			return kinds
		}
	}
	return nil
}

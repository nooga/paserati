package checker

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// reportUnknownObject reports property access on a value of type `unknown`
// (TS18046, or TS2571 when the operand has no entity name).
func (c *Checker) reportUnknownObject(operand parser.Expression) {
	if !c.strictNullChecks {
		return
	}
	if name := entityNameText(operand); name != "" && len(name) < 100 {
		c.addErrorWithCode(operand, errors.TS18046, "'"+name+"' is of type 'unknown'.")
		return
	}
	c.addErrorWithCode(operand, errors.TS2571, "Object is of type 'unknown'.")
}

// primitivePropertyType resolves a property read on a primitive-like type that
// the main member-access switch has no case for (boolean, bigint, void, never,
// the non-primitive `object`). Known prototype members resolve; anything else
// is TS2339 at the property name.
func (c *Checker) primitivePropertyType(t types.Type, propertyName string, node *parser.MemberExpression) types.Type {
	prototypeName := ""
	switch t {
	case types.Boolean:
		prototypeName = "boolean"
	case types.BigInt:
		prototypeName = "bigint"
	}
	if prototypeName != "" {
		if methodType := c.env.GetPrimitivePrototypeMethodType(prototypeName, propertyName); methodType != nil {
			return methodType
		}
	}
	switch t.(type) {
	case *types.Primitive, *types.LiteralType:
		c.addErrorWithCode(node.Property, errors.TS2339, "Property '"+propertyName+"' does not exist on type '"+t.String()+"'.")
		return types.Any
	}
	c.addError(node.Object, "property access is not supported on type "+t.String())
	return types.Any
}

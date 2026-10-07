package checker

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// entityNameText returns the dotted text of an identifier or property-access
// chain of identifiers (TypeScript's isEntityNameExpression), or "" when the
// expression is not one.
func entityNameText(expr parser.Expression) string {
	switch e := expr.(type) {
	case *parser.Identifier:
		return e.Value
	case *parser.MemberExpression:
		if id, ok := e.Property.(*parser.Identifier); ok {
			if left := entityNameText(e.Object); left != "" {
				return left + "." + id.Value
			}
		}
	}
	return ""
}

// nullishFacts reports whether a type is, or is a union containing, null and
// undefined, and returns the type with those members removed (types.Never when
// nothing else is left).
func nullishFacts(t types.Type) (hasNull, hasUndefined bool, rest types.Type) {
	switch t {
	case types.Null:
		return true, false, types.Never
	case types.Undefined:
		return false, true, types.Never
	}
	union, ok := t.(*types.UnionType)
	if !ok {
		return false, false, t
	}
	var members []types.Type
	for _, member := range union.Types {
		switch member {
		case types.Null:
			hasNull = true
		case types.Undefined:
			hasUndefined = true
		default:
			members = append(members, member)
		}
	}
	if !hasNull && !hasUndefined {
		return false, false, t
	}
	if len(members) == 0 {
		return hasNull, hasUndefined, types.Never
	}
	return hasNull, hasUndefined, types.NewUnionType(members...)
}

// reportNullishObject mirrors TypeScript's reportObjectPossiblyNullOrUndefinedError
// for an operand whose type is, or includes, `null` or `undefined`. Without
// strictNullChecks nothing is reported (the operand is just `any`-like).
func (c *Checker) reportNullishObject(operand parser.Expression, t types.Type) {
	hasNull, hasUndefined, _ := nullishFacts(t)
	c.reportNullishFacts(operand, hasNull, hasUndefined)
}

func (c *Checker) reportNullishFacts(operand parser.Expression, hasNull, hasUndefined bool) {
	if !c.strictNullChecks || (!hasNull && !hasUndefined) {
		return
	}
	if _, ok := operand.(*parser.NullLiteral); ok {
		c.addErrorWithCode(operand, errors.TS18050, "The value 'null' cannot be used here.")
		return
	}
	name := entityNameText(operand)
	if name != "" && len(name) < 100 {
		switch {
		case name == "undefined":
			c.addErrorWithCode(operand, errors.TS18050, "The value 'undefined' cannot be used here.")
		case hasNull && hasUndefined:
			c.addErrorWithCode(operand, errors.TS18049, "'"+name+"' is possibly 'null' or 'undefined'.")
		case hasNull:
			c.addErrorWithCode(operand, errors.TS18047, "'"+name+"' is possibly 'null'.")
		default:
			c.addErrorWithCode(operand, errors.TS18048, "'"+name+"' is possibly 'undefined'.")
		}
		return
	}
	switch {
	case hasNull && hasUndefined:
		c.addErrorWithCode(operand, errors.TS2533, "Object is possibly 'null' or 'undefined'.")
	case hasNull:
		c.addErrorWithCode(operand, errors.TS2531, "Object is possibly 'null'.")
	default:
		c.addErrorWithCode(operand, errors.TS2532, "Object is possibly 'undefined'.")
	}
}

// stripNullishObject reports a nullish part of an object operand (TS2531,
// TS2532, TS2533, TS18047-TS18049) and returns the type without it, mirroring
// TypeScript's checkNonNullType. The second result is false when nothing but
// null/undefined was there, in which case the access yields `any`.
func (c *Checker) stripNullishObject(operand parser.Expression, t types.Type) (types.Type, bool) {
	hasNull, hasUndefined, rest := nullishFacts(t)
	if !hasNull && !hasUndefined {
		return t, true
	}
	c.reportNullishFacts(operand, hasNull, hasUndefined)
	return rest, rest != types.Never
}

// reportNonIndexable reports why `left[index]` could not be resolved when the
// base type has no indexing behaviour we model. Only the cases TypeScript
// reports the same way are diagnosed: `unknown`, and bases that include `null`
// or `undefined`. Anything else (mapped, generic and similarly deferred types)
// is left unreported rather than risk a diagnostic TypeScript would not give.
func (c *Checker) reportNonIndexable(node *parser.IndexExpression, leftType types.Type) {
	if leftType == types.Unknown && c.strictNullChecks {
		if name := entityNameText(node.Left); name != "" && len(name) < 100 {
			c.addErrorWithCode(node.Left, errors.TS18046, "'"+name+"' is of type 'unknown'.")
		} else {
			c.addErrorWithCode(node.Left, errors.TS2571, "Object is of type 'unknown'.")
		}
		return
	}
	c.stripNullishObject(node.Left, leftType)
}

// isDefinitelyInvalidIndexType reports whether a type can certainly not be used
// as an index: booleans, bigint, null/undefined/void and object types. String,
// number, symbol, enums, their literals and anything we cannot judge are fine.
func isDefinitelyInvalidIndexType(t types.Type) bool {
	switch tt := t.(type) {
	case *types.Primitive:
		switch tt {
		case types.Boolean, types.BigInt, types.Null, types.Undefined, types.Void:
			return true
		}
		return false
	case *types.LiteralType:
		return types.IsAssignable(tt, types.Boolean)
	case *types.ObjectType:
		return true
	case *types.UnionType:
		for _, member := range tt.Types {
			if isDefinitelyInvalidIndexType(member) {
				return true
			}
		}
	}
	return false
}

// reportInvalidIndexType reports TS2538 for an index expression whose key type
// cannot be used to index an object.
func (c *Checker) reportInvalidIndexType(node *parser.IndexExpression, indexType types.Type) {
	if !isDefinitelyInvalidIndexType(indexType) {
		return
	}
	c.addErrorWithCode(node.Index, errors.TS2538, "Type '"+indexType.String()+"' cannot be used as an index type.")
}

// reportBadObjectDestructure handles an object pattern whose source type is not
// an object. TypeScript treats the pattern like property reads: null and
// undefined are reported as such (TS2531/TS2532/TS18047...), while other
// primitives simply look their properties up on the apparent type, which the
// binding code already does, so nothing else is reported here.
func (c *Checker) reportBadObjectDestructure(node parser.Node, t types.Type) {
	if expr, ok := node.(parser.Expression); ok {
		c.reportNullishObject(expr, t)
		return
	}
	hasNull, hasUndefined, _ := nullishFacts(t)
	if c.strictNullChecks && (hasNull || hasUndefined) {
		switch {
		case hasNull && hasUndefined:
			c.addErrorWithCode(node, errors.TS2533, "Object is possibly 'null' or 'undefined'.")
		case hasNull:
			c.addErrorWithCode(node, errors.TS2531, "Object is possibly 'null'.")
		default:
			c.addErrorWithCode(node, errors.TS2532, "Object is possibly 'undefined'.")
		}
	}
}

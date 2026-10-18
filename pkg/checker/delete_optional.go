package checker

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// checkDeleteOperandOptional reports TS2790 when `delete` removes a declared
// property whose type does not admit undefined (tsc's
// checkDeleteExpressionMustBeOptional). Only strictNullChecks makes the
// distinction, and an operand without a resolved property (an index
// signature, a computed key) is not checked.
func (c *Checker) checkDeleteOperandOptional(operand parser.Expression) {
	report := operand
	if !c.strictNullChecks {
		return
	}
	// In `a?.b.c` the operand is the chain and `.c` its continuation.
	for {
		if oc, isChain := operand.(*parser.OptionalChainingExpression); isChain && oc.Continuation != nil {
			operand, report = oc.Continuation, operand
		} else if oi, isChain := operand.(*parser.OptionalIndexExpression); isChain && oi.Continuation != nil {
			operand, report = oi.Continuation, operand
		} else {
			break
		}
	}
	var object parser.Expression
	name, ok := "", false
	switch e := operand.(type) {
	case *parser.MemberExpression:
		object = e.Object
		if id, isIdent := e.Property.(*parser.Identifier); isIdent {
			name, ok = id.Value, true
		}
	case *parser.OptionalChainingExpression:
		object = e.Object
		if id, isIdent := e.Property.(*parser.Identifier); isIdent {
			name, ok = id.Value, true
		}
	case *parser.IndexExpression:
		object = e.Left
		name, ok = staticPropertyName(e.Index)
	case *parser.OptionalIndexExpression:
		object = e.Object
		name, ok = staticPropertyName(e.Index)
	}
	if !ok {
		return
	}
	var objType types.Type
	if object == nil {
		objType = c.chainLinkReceivers[operand]
	} else {
		objType = object.GetComputedType()
		if objType == nil {
			// A narrowed member read skips typing its object.
			if key := expressionToNarrowingKey(object); key != "" {
				objType = c.declaredTypeOfRef(key)
			}
		}
	}
	if objType == nil {
		return
	}
	var found []types.Type
	optional := false
	for _, obj := range c.targetObjects(objType) {
		pt, has := obj.EffectiveProperty(name)
		if !has {
			continue
		}
		found = append(found, pt)
		if obj.IsPropertyOptional(name) {
			optional = true
		}
	}
	if len(found) == 0 || optional {
		return
	}
	for _, t := range found {
		if t == types.Never || admitsUndefined(t) {
			return
		}
	}
	debugPrintf("")
	c.addErrorWithCode(report, errors.TS2790, "The operand of a 'delete' operator must be optional.")
}

// admitsUndefined reports whether undefined is among a type's values (a type
// parameter counts, since its constraint may allow it).
func admitsUndefined(t types.Type) bool {
	switch tt := t.(type) {
	case *types.UnionType:
		for _, m := range tt.Types {
			if admitsUndefined(m) {
				return true
			}
		}
		return false
	case *types.TypeParameterType:
		return true
	}
	return t == types.Undefined || t == types.Void || t == types.Any || t == types.Unknown
}

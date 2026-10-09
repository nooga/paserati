package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// leftmostExpression returns the expression whose first token starts expr, so
// diagnostics TypeScript reports "at the node" land on the same line.
func leftmostExpression(expr parser.Expression) parser.Expression {
	for {
		switch e := expr.(type) {
		case *parser.CallExpression:
			expr = e.Function
		case *parser.MemberExpression:
			expr = e.Object
		case *parser.IndexExpression:
			expr = e.Left
		case *parser.InfixExpression:
			expr = e.Left
		default:
			return expr
		}
	}
}

// signatureTypeArgumentRange returns the minimum and maximum number of type
// arguments a signature accepts, and whether that is known. A signature whose
// type parameters are implicit (generic only by containing type parameters) is
// unknown.
func (c *Checker) signatureTypeArgumentRange(sig *types.Signature) (min, max int, known bool) {
	if len(sig.TypeParameters) == 0 {
		if c.isGenericSignature(sig) {
			return 0, 0, false
		}
		return 0, 0, true
	}
	max = len(sig.TypeParameters)
	min = max
	for min > 0 && sig.TypeParameters[min-1].Default != nil {
		min--
	}
	return min, max, true
}

// checkCallTypeArguments reports TypeScript's diagnostics for explicit type
// arguments on a call whose target cannot take them:
//   - TS2347 when the callee is `any` (an untyped call)
//   - TS2558 when no call signature accepts that many type arguments
func (c *Checker) checkCallTypeArguments(node *parser.CallExpression, funcType types.Type) bool {
	typeArgs := node.TypeArguments
	if len(typeArgs) == 0 {
		return false
	}
	if funcType == types.Any {
		c.addErrorWithCode(leftmostExpression(node), errors.TS2347, "Untyped function calls may not accept type arguments.")
		return false
	}
	obj, ok := funcType.(*types.ObjectType)
	if !ok || len(obj.CallSignatures) == 0 {
		return false
	}
	argCount := len(typeArgs)
	belowArgCount, aboveArgCount := -1, -1
	for _, sig := range obj.CallSignatures {
		min, max, known := c.signatureTypeArgumentRange(sig)
		if !known {
			return false
		}
		if argCount >= min && argCount <= max {
			return false
		}
		if min > argCount {
			if aboveArgCount == -1 || min < aboveArgCount {
				aboveArgCount = min
			}
		} else if max < argCount {
			if max > belowArgCount {
				belowArgCount = max
			}
		}
	}
	if len(obj.CallSignatures) == 1 {
		min, max, _ := c.signatureTypeArgumentRange(obj.CallSignatures[0])
		expected := fmt.Sprintf("%d", min)
		if min < max {
			expected = fmt.Sprintf("%d-%d", min, max)
		}
		c.addErrorWithCode(typeArgs[0], errors.TS2558, fmt.Sprintf("Expected %s type arguments, but got %d.", expected, argCount))
		return true
	}
	if belowArgCount != -1 && aboveArgCount != -1 {
		c.addErrorWithCode(typeArgs[0], errors.TS2743, fmt.Sprintf("No overload expects %d type arguments, but overloads do exist that expect either %d or %d type arguments.", argCount, belowArgCount, aboveArgCount))
		return true
	}
	expected := belowArgCount
	if belowArgCount == -1 {
		expected = aboveArgCount
	}
	c.addErrorWithCode(typeArgs[0], errors.TS2558, fmt.Sprintf("Expected %d type arguments, but got %d.", expected, argCount))
	return true
}

// instantiatedConstraint is tp's constraint with the solution's type arguments
// substituted in, or nil when tp is unconstrained.
func (c *Checker) instantiatedConstraint(tp *types.TypeParameter, solution map[*types.TypeParameter]types.Type) types.Type {
	if tp == nil || tp.Constraint == nil || tp.Constraint == types.Any {
		return nil
	}
	byName := make(map[string]types.Type, len(solution))
	for p, t := range solution {
		byName[p.Name] = t
	}
	return c.substituteTypes(tp.Constraint, byName)
}

// checkExplicitTypeArgConstraints reports TS2344 for each written type
// argument that does not satisfy its type parameter's constraint.
func (c *Checker) checkExplicitTypeArgConstraints(typeParams []*types.TypeParameter, typeArgNodes []parser.Expression, solution map[*types.TypeParameter]types.Type) {
	for i, node := range typeArgNodes {
		if i >= len(typeParams) {
			break
		}
		constraint := c.instantiatedConstraint(typeParams[i], solution)
		arg := solution[typeParams[i]]
		if constraint == nil || arg == nil || arg == types.Any || c.satisfiesConstraint(arg, constraint) {
			continue
		}
		c.addErrorWithCode(node, errors.TS2344, fmt.Sprintf(
			"Type '%s' does not satisfy the constraint '%s'.", arg.String(), constraint.String()))
	}
}

// fallBackToConstraints replaces an inferred type argument that violates its
// parameter's constraint with the constraint itself, as tsc does; the argument
// that produced the candidate then fails against the constraint at the call.
func (c *Checker) fallBackToConstraints(solution map[*types.TypeParameter]types.Type) {
	for tp, inferred := range solution {
		constraint := c.instantiatedConstraint(tp, solution)
		if constraint == nil || inferred == types.Any || c.typeContainsTypeParameter(constraint) || c.satisfiesConstraint(inferred, constraint) {
			continue
		}
		solution[tp] = constraint
	}
}

// satisfiesConstraint is assignability for a type argument against its
// constraint. A primitive or array has the members of its wrapper type, which
// plain structural assignability does not see (`string` satisfies
// `{ length: number }`).
func (c *Checker) satisfiesConstraint(arg, constraint types.Type) bool {
	if types.IsAssignable(arg, constraint) {
		return true
	}
	if union, ok := arg.(*types.UnionType); ok {
		for _, m := range union.Types {
			if !c.satisfiesConstraint(m, constraint) {
				return false
			}
		}
		return true
	}
	target, ok := c.resolveStructural(constraint).(*types.ObjectType)
	if !ok || target.IsCallable() || len(target.ConstructSignatures) > 0 {
		return false
	}
	var protoName string
	switch w := types.GetWidenedType(arg).(type) {
	case *types.Primitive:
		switch w {
		case types.String:
			protoName = "string"
		case types.Number:
			protoName = "number"
		default:
			return false
		}
	case *types.ArrayType, *types.TupleType:
		protoName = "array"
	default:
		return false
	}
	for name, want := range target.GetEffectiveProperties() {
		if target.IsPropertyOptional(name) {
			continue
		}
		var have types.Type
		if name == "length" && protoName != "number" {
			have = types.Number
		} else {
			have = c.env.GetPrimitivePrototypeMethodType(protoName, name)
		}
		if have == nil {
			return false
		}
		if want != nil && !types.IsAssignable(have, want) && !types.IsAssignable(c.instantiateGenericMethod(have, types.Any), want) {
			return false
		}
	}
	return true
}

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

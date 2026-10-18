package checker

import (
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// compoundAssignmentResult applies TypeScript's operator rules to a compound
// assignment such as `x += y` or `x -= y` (the same checkBinaryLikeExpression
// path as the binary operator), reporting TS18050/TS2362/TS2363/TS2365 for
// operands the operator cannot take. It returns the type the operation
// produces (which is what must be assignable to the target) and whether the
// operands were acceptable.
func (c *Checker) compoundAssignmentResult(node *parser.AssignmentExpression, lhs, rhs types.Type) (types.Type, bool) {
	b := binaryOperands{op: node.Operator, left: node.Left, right: node.Value, node: node}
	n := len(c.errors)
	if node.Operator == "+=" {
		if !c.assignableToKind(lhs, kindStringLike, false) && !c.assignableToKind(rhs, kindStringLike, false) {
			if c.checkNullishOperand(node.Value) {
				rhs = types.Any
			}
		}
		t := c.checkAdditionOperator(b, lhs, rhs)
		return t, len(c.errors) == n
	}
	if c.checkNullishOperand(node.Value) {
		rhs = types.Any
	}
	t := c.checkArithmeticOperator(b, lhs, rhs)
	return t, len(c.errors) == n
}

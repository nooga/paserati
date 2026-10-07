package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// isJudgeableOperandType reports whether a type is simple enough for us to
// trust a verdict about it: primitives, literals and plain object types.
// Unions, type parameters and the like are left alone rather than risk a
// diagnostic TypeScript would not give.
func isJudgeableOperandType(t types.Type) bool {
	switch types.GetWidenedType(t).(type) {
	case *types.Primitive, *types.LiteralType, *types.ObjectType:
		return true
	}
	return false
}

// compoundAssignmentResult applies TypeScript's operator rules to a compound
// assignment such as `x += y` or `x -= y`, reporting TS2362/TS2363/TS2365 for
// operands the operator cannot take. It returns the type the operation
// produces (which is what must be assignable to the target) and whether the
// operands were acceptable.
func (c *Checker) compoundAssignmentResult(node *parser.AssignmentExpression, lhs, rhs types.Type) (types.Type, bool) {
	wl, wr := types.GetWidenedType(lhs), types.GetWidenedType(rhs)
	if wl == types.Any || wr == types.Any {
		return types.Any, true
	}
	if !isJudgeableOperandType(lhs) || !isJudgeableOperandType(rhs) {
		// Cannot judge; assume the operation is fine and yields the target type.
		return lhs, true
	}
	leftNumeric := isArithmeticOperandType(lhs) && wl != types.BigInt
	rightNumeric := isArithmeticOperandType(rhs) && wr != types.BigInt
	bothNumeric := leftNumeric && rightNumeric
	bothBigInt := wl == types.BigInt && wr == types.BigInt
	notApplicable := func() (types.Type, bool) {
		c.addErrorWithCode(leftmostExpression(node.Left), errors.TS2365, fmt.Sprintf(
			"Operator '%s' cannot be applied to types '%s' and '%s'.", node.Operator, wl.String(), wr.String()))
		return types.Any, false
	}
	if node.Operator == "+=" {
		switch {
		case bothNumeric:
			return types.Number, true
		case bothBigInt:
			return types.BigInt, true
		case wl == types.String || wr == types.String:
			return types.String, true
		}
		return notApplicable()
	}
	// -=, *=, /=, %=, **=, &=, |=, ^=, <<=, >>=, >>>= need numeric operands.
	leftOk := isArithmeticOperandType(lhs)
	rightOk := isArithmeticOperandType(rhs)
	if leftOk && rightOk {
		if bothBigInt {
			return types.BigInt, true
		}
		if bothNumeric {
			return types.Number, true
		}
		return notApplicable()
	}
	if !leftOk {
		c.addErrorWithCode(node.Left, errors.TS2362,
			"The left-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type.")
	}
	if !rightOk {
		c.addErrorWithCode(node.Value, errors.TS2363,
			"The right-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type.")
	}
	return types.Any, false
}

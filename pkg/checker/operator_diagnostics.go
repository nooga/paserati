package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// TypeScript splits "these operands do not work with this operator" across
// three codes, by operator:
//
//   - TS2365 for `+` and the relational operators, naming both operand types
//   - TS2362/TS2363 for the arithmetic, bitwise and shift operators, which
//     TypeScript checks one operand at a time and reports at the first bad one
//
// The distinction is TypeScript's, not ours: `+` and `<` accept several
// combinations of types, so only the pair is meaningful, whereas `-` and `&`
// require each operand to be numeric independently.

// reportOperatorNotApplicable reports TS2365 for an operator that has no
// meaning for this combination of operand types.
func (c *Checker) reportOperatorNotApplicable(node *parser.InfixExpression, left, right types.Type) {
	c.addErrorWithCode(node.Right, errors.TS2365, fmt.Sprintf(
		"Operator '%s' cannot be applied to types '%s' and '%s'.",
		node.Operator, left.String(), right.String()))
}

// reportArithmeticOperandType reports TS2362 and/or TS2363 for an operator that
// needs each operand to be numeric on its own. TypeScript checks the two
// operands independently, so both diagnostics appear when both are wrong.
func (c *Checker) reportArithmeticOperandType(node *parser.InfixExpression, left, right types.Type) {
	leftBad := !c.isArithmeticOperandType(left)
	rightBad := !c.isArithmeticOperandType(right)
	if leftBad {
		c.addErrorWithCode(node.Left, errors.TS2362,
			"The left-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type.")
	}
	if rightBad || !leftBad {
		c.addErrorWithCode(node.Right, errors.TS2363,
			"The right-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type.")
	}
}

// isArithmeticOperandType reports whether a type is one an arithmetic operator
// accepts on its own: any, number, bigint, or a numeric enum.
func (c *Checker) isArithmeticOperandType(t types.Type) bool {
	if t == nil {
		return false
	}
	widened := c.rel.GetWidenedType(t)
	return widened == types.Any || widened == types.Number || widened == types.BigInt ||
		types.IsNumericEnumLikeType(t)
}

// reportSymbolUnaryOperand reports TS2469 for a unary +, - or ~ applied to a
// symbol, the only operand type TypeScript refuses for those operators.
func (c *Checker) reportSymbolUnaryOperand(node *parser.PrefixExpression, operand types.Type) {
	if operand != types.Symbol {
		return
	}
	c.addErrorWithCode(node.Right, errors.TS2469, fmt.Sprintf("The '%s' operator cannot be applied to type 'symbol'.", node.Operator))
}

// isPrimitiveOperandType reports whether every constituent of t is a primitive
// (string, number, boolean, bigint, symbol, null, undefined, void or a literal
// of one of them), i.e. a value that can never be an object instance.
func (c *Checker) isPrimitiveOperandType(t types.Type) bool {
	t = c.rel.GetWidenedType(t)
	switch tt := t.(type) {
	case *types.Primitive:
		switch tt {
		case types.String, types.Number, types.Boolean, types.BigInt, types.Symbol, types.Null, types.Undefined, types.Void:
			return true
		}
		return false
	case *types.LiteralType:
		return true
	case *types.UnionType:
		for _, member := range tt.Types {
			if !c.isPrimitiveOperandType(member) {
				return false
			}
		}
		return len(tt.Types) > 0
	}
	return false
}

// checkArithmeticOperator checks an arithmetic, bitwise or shift operator the
// way tsc's checkBinaryLikeExpression does: `&`, `|` and `^` on two booleans
// suggest the logical operator (TS2447); otherwise each operand must on its own
// be assignable to number | bigint (TS2362/TS2363), and bigint operands must
// not mix with number ones (TS2365).
func (c *Checker) checkArithmeticOperator(node *parser.InfixExpression, leftType, rightType types.Type) types.Type {
	if leftType == nil {
		leftType = types.Any
	}
	if rightType == nil {
		rightType = types.Any
	}
	switch node.Operator {
	case "&", "|", "^":
		if c.isBooleanLikeOperand(leftType) && c.isBooleanLikeOperand(rightType) {
			suggest := map[string]string{"&": "&&", "|": "||", "^": "!=="}[node.Operator]
			c.addErrorWithCode(node.Left, errors.TS2447, fmt.Sprintf(
				"The '%s' operator is not allowed for boolean types. Consider using '%s' instead.", node.Operator, suggest))
			return types.Number
		}
	}
	numberOrBigInt := types.NewUnionType(types.Number, types.BigInt)
	leftOk := c.arithmeticOperandOk(leftType, numberOrBigInt)
	rightOk := c.arithmeticOperandOk(rightType, numberOrBigInt)
	if !leftOk {
		c.addErrorWithCode(node.Left, errors.TS2362,
			"The left-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type.")
	}
	if !rightOk {
		c.addErrorWithCode(node.Right, errors.TS2363,
			"The right-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type.")
	}
	if (isAnyOrUnknownType(leftType) && isAnyOrUnknownType(rightType)) ||
		!(c.maybeBigIntLike(leftType) || c.maybeBigIntLike(rightType)) {
		return types.Number
	}
	if c.isBigIntLike(leftType) && c.isBigIntLike(rightType) {
		if node.Operator == ">>>" {
			c.reportOperatorNotApplicable(node, c.rel.GetWidenedType(leftType), c.rel.GetWidenedType(rightType))
		}
		return types.BigInt
	}
	if leftOk && rightOk {
		c.reportOperatorNotApplicable(node, c.rel.GetWidenedType(leftType), c.rel.GetWidenedType(rightType))
	}
	return types.Any
}

// arithmeticOperandOk mirrors checkArithmeticOperandType: the operand must be
// assignable to number | bigint. unknown is reported elsewhere (TS18046).
func (c *Checker) arithmeticOperandOk(t, numberOrBigInt types.Type) bool {
	if isAnyOrUnknownType(t) {
		return true
	}
	return c.rel.IsAssignable(t, numberOrBigInt)
}

func isAnyOrUnknownType(t types.Type) bool {
	return t == types.Any || t == types.Unknown
}

// isBigIntLike reports whether t is bigint, a bigint literal, or a union of them.
func (c *Checker) isBigIntLike(t types.Type) bool {
	if u, ok := t.(*types.UnionType); ok {
		for _, m := range u.Types {
			if !c.isBigIntLike(m) {
				return false
			}
		}
		return len(u.Types) > 0
	}
	return t == types.Never || c.rel.GetWidenedType(t) == types.BigInt
}

// maybeBigIntLike reports whether any constituent of t is bigint-like.
func (c *Checker) maybeBigIntLike(t types.Type) bool {
	switch tt := t.(type) {
	case *types.UnionType:
		for _, m := range tt.Types {
			if c.maybeBigIntLike(m) {
				return true
			}
		}
		return false
	case *types.IntersectionType:
		for _, m := range tt.Types {
			if c.maybeBigIntLike(m) {
				return true
			}
		}
		return false
	}
	return t != types.Never && c.rel.GetWidenedType(t) == types.BigInt
}

// isBooleanLikeOperand reports whether t is boolean, a boolean literal, or a
// union of them.
func (c *Checker) isBooleanLikeOperand(t types.Type) bool {
	if u, ok := t.(*types.UnionType); ok {
		for _, m := range u.Types {
			if !c.isBooleanLikeOperand(m) {
				return false
			}
		}
		return len(u.Types) > 0
	}
	return c.rel.GetWidenedType(t) == types.Boolean
}

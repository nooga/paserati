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
	leftBad := !isArithmeticOperandType(left)
	rightBad := !isArithmeticOperandType(right)
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
func isArithmeticOperandType(t types.Type) bool {
	if t == nil {
		return false
	}
	widened := types.GetWidenedType(t)
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
func isPrimitiveOperandType(t types.Type) bool {
	t = types.GetWidenedType(t)
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
			if !isPrimitiveOperandType(member) {
				return false
			}
		}
		return len(tt.Types) > 0
	}
	return false
}

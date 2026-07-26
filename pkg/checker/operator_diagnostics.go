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

// reportArithmeticOperandType reports TS2362 or TS2363 for an operator that
// needs each operand to be numeric on its own. TypeScript checks the left
// operand first and stops there if it fails, so only one diagnostic is
// produced even when both operands are wrong.
func (c *Checker) reportArithmeticOperandType(node *parser.InfixExpression, left types.Type) {
	if !isArithmeticOperandType(left) {
		c.addErrorWithCode(node.Left, errors.TS2362,
			"The left-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type.")
		return
	}
	c.addErrorWithCode(node.Right, errors.TS2363,
		"The right-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type.")
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

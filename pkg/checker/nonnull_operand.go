package checker

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// TS18050 — "The value 'null' cannot be used here."
//
// TypeScript runs its arithmetic, bitwise and relational operands through
// checkNonNullType. When the operand is written as the literal `null` keyword
// or the identifier `undefined`, that reports TS18050 at the operand and the
// operand is then treated as non-nullable, so the usual TS2362/TS2363/TS2365
// complaint about the operator never follows.
//
// Operands that are merely *typed* null or undefined get a different
// diagnostic in TypeScript (TS18047/TS18048/TS18049, "'x' is possibly 'null'"),
// which we do not report yet — hence the syntactic test here rather than a
// type test.

// nullishOperandName returns the name TypeScript uses in TS18050 for an operand
// written as a nullish literal, and whether the operand is one.
func nullishOperandName(operand parser.Expression) (string, bool) {
	switch operand.(type) {
	case *parser.NullLiteral:
		return "null", true
	case *parser.UndefinedLiteral:
		return "undefined", true
	}
	return "", false
}

// checkNullishOperand reports TS18050 if the operand is written as `null` or
// `undefined`. It returns true when it reported, so callers can skip the
// operator diagnostic that TypeScript suppresses in the same situation.
func (c *Checker) checkNullishOperand(operand parser.Expression) bool {
	name, ok := nullishOperandName(operand)
	if !ok {
		return false
	}
	c.addErrorWithCode(operand, errors.TS18050, "The value '"+name+"' cannot be used here.")
	return true
}

// checkNullishOperandsForOperator applies the TS18050 check to both operands of
// a binary operator, following TypeScript's rules about which operators run
// their operands through checkNonNullType.
//
// It returns true if either operand was reported, meaning the caller should
// suppress its own operator-level diagnostic.
func (c *Checker) checkNullishOperandsForOperator(node *parser.InfixExpression, leftType, rightType types.Type) bool {
	// Without strictNullChecks, `null` and `undefined` are assignable to
	// everything, so TypeScript never singles out a nullish operand — it falls
	// through to TS2365 about the operand pair instead.
	if !c.strictNullChecks {
		return false
	}
	switch node.Operator {
	case "-", "*", "/", "%", "**", "<<", ">>", ">>>", "&", "|", "^", "<", ">", "<=", ">=":
		// Both operands are always checked.
	case "+":
		// TypeScript only null-checks the operands of `+` when neither side
		// could be a string — otherwise this is concatenation, where `null` and
		// `undefined` are legitimate values that stringify.
		if isStringLikeForAddition(leftType) || isStringLikeForAddition(rightType) {
			return false
		}
	default:
		// Equality, logical, `in`, `instanceof` and the rest never null-check
		// their operands.
		return false
	}

	// Both sides are reported when both are nullish literals, so evaluate each
	// rather than short-circuiting.
	reportedLeft := c.checkNullishOperand(node.Left)
	reportedRight := c.checkNullishOperand(node.Right)
	return reportedLeft || reportedRight
}

// nullishOperatorResultType is the type an operator produces once TS18050 has
// been reported for one of its operands. TypeScript strips the nullish part of
// the operand type before applying the operator, which leaves the operator's
// ordinary result.
func nullishOperatorResultType(operator string) types.Type {
	switch operator {
	case "<", ">", "<=", ">=":
		return types.Boolean
	default:
		return types.Number
	}
}

// isStringLikeForAddition reports whether a `+` operand could be a string, in
// which case the expression is concatenation and nullish operands are allowed.
// `any` counts, matching TypeScript's assignability-based test.
func isStringLikeForAddition(t types.Type) bool {
	if t == nil {
		return false
	}
	widened := types.GetWidenedType(t)
	if widened == types.String || widened == types.Any {
		return true
	}
	if union, ok := widened.(*types.UnionType); ok {
		for _, member := range union.Types {
			if isStringLikeForAddition(member) {
				return true
			}
		}
	}
	return false
}

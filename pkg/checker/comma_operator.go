package checker

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
)

// TS2695 — "Left side of comma operator is unused and has no side effects."
//
// The comma operator discards its left operand, so writing one that cannot do
// anything is always a mistake — usually a `;` that was typed as a `,`.
// TypeScript reports this at the left operand whenever that operand is
// side-effect free.

// checkCommaOperator reports TS2695 when the discarded left operand of a comma
// expression can have no effect.
func (c *Checker) checkCommaOperator(node *parser.InfixExpression) {
	if c.allowUnreachableCode || !isSideEffectFree(node.Left) || c.isIndirectCallCallee(node) {
		return
	}
	c.addErrorWithCode(node.Left, errors.TS2695,
		"Left side of comma operator is unused and has no side effects.")
}

// noteIndirectCallCallee records a comma expression that is being called, so
// checkCommaOperator can recognise `(0, f)(...)`. Callers invoke it before
// visiting the callee.
func (c *Checker) noteIndirectCallCallee(callee parser.Expression) {
	infix, ok := callee.(*parser.InfixExpression)
	if !ok || infix.Operator != "," {
		return
	}
	if c.indirectCallCallees == nil {
		c.indirectCallCallees = make(map[*parser.InfixExpression]bool)
	}
	c.indirectCallCallees[infix] = true
}

// isIndirectCallCallee reports whether a comma expression is the `(0, f)` used
// to call `f` with an undefined `this`. It is the standard way to reach global
// `eval` or to strip a method from its receiver, so TypeScript exempts it from
// TS2695 even though the literal `0` is plainly inert.
func (c *Checker) isIndirectCallCallee(node *parser.InfixExpression) bool {
	if !c.indirectCallCallees[node] {
		return false
	}
	zero, ok := node.Left.(*parser.NumberLiteral)
	return ok && zero.Value == 0
}

// isSideEffectFree reports whether evaluating an expression can be observed.
// It mirrors TypeScript's isSideEffectFree: the listed forms are known to be
// inert, and everything else — calls, `new`, assignments, `await`, `yield`,
// `delete`, increments, property access (which can run a getter) — is assumed
// to do something.
func isSideEffectFree(node parser.Expression) bool {
	switch n := node.(type) {
	case *parser.Identifier,
		*parser.StringLiteral,
		*parser.NumberLiteral,
		*parser.BigIntLiteral,
		*parser.BooleanLiteral,
		*parser.RegexLiteral,
		*parser.NullLiteral,
		*parser.UndefinedLiteral,
		*parser.FunctionLiteral,
		*parser.ArrowFunctionLiteral,
		*parser.ClassExpression,
		*parser.ArrayLiteral,
		*parser.ObjectLiteral,
		*parser.TemplateLiteral,
		*parser.TypeofExpression,
		*parser.NonNullExpression:
		return true

	case *parser.TernaryExpression:
		return isSideEffectFree(n.Consequence) && isSideEffectFree(n.Alternative)

	case *parser.InfixExpression:
		// Assignment operators are handled by AssignmentExpression; any other
		// binary operator is inert exactly when both its operands are.
		return isSideEffectFree(n.Left) && isSideEffectFree(n.Right)

	case *parser.PrefixExpression:
		// TypeScript treats `!x`, `+x`, `-x` and `~x` as inert on the strength
		// of the operator alone, without looking at the operand — so `-foo()`
		// counts as side-effect free. `void`, `delete` and `await` do not, and
		// neither do `++`/`--`, which parse as UpdateExpression.
		switch n.Operator {
		case "!", "+", "-", "~":
			return true
		}
		return false
	}
	return false
}

package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// Assignment and increment/decrement targets, checked the way tsc does.
//
// tsc's parser accepts any expression on the left of `=` or as the operand of
// `++`/`--`; the checker then requires a variable or property access
// (checkReferenceExpression) and refuses names that are not variables:
// classes, enums and namespaces (checkIdentifier with an assignment target).
// Paserati's parser rejects invalid targets itself unless the checker is told
// to report them (parser.SetTscDiagnostics), as the conformance
// harness does.

// checkReferenceExpression reports code (or optCode for an optional chain) when
// expr, stripped of type assertions, is not an identifier or property access.
func (c *Checker) checkReferenceExpression(expr parser.Expression, code, msg, optCode, optMsg string) bool {
	switch parser.UnwrapTypeAssertions(expr).(type) {
	case *parser.Identifier, *parser.UndefinedLiteral, *parser.MemberExpression, *parser.IndexExpression:
		return true
	case *parser.OptionalChainingExpression, *parser.OptionalIndexExpression:
		c.addErrorWithCode(expr, optCode, optMsg)
		return false
	}
	c.addErrorWithCode(expr, code, msg)
	return false
}

// checkAssignmentTargetName reports an assignment to a name that is not a
// variable: `undefined` (TS2539), an enum (TS2628), a class (TS2629) or a
// namespace (TS2631). It returns true when it reported one; tsc then treats the
// target as the error type, which silences the operator's other checks.
func (c *Checker) checkAssignmentTargetName(expr parser.Expression) bool {
	switch t := parser.UnwrapTypeAssertions(expr).(type) {
	case *parser.UndefinedLiteral:
		c.addErrorWithCode(t, errors.TS2539, "Cannot assign to 'undefined' because it is not a variable.")
		return true
	case *parser.Identifier:
		var code, what string
		switch c.env.ResolveBindingKind(t.Value) {
		case BindingEnum:
			code, what = errors.TS2628, "an enum"
		case BindingClass:
			code, what = errors.TS2629, "a class"
		case BindingNamespace:
			code, what = errors.TS2631, "a namespace"
		default:
			return false
		}
		c.addErrorWithCode(t, code, fmt.Sprintf("Cannot assign to '%s' because it is %s.", t.Value, what))
		return true
	}
	return false
}

// checkUpdateOperand checks the operand of `++`/`--` (checkPrefixUnaryExpression
// and checkPostfixUnaryExpression): it must be numeric (TS2356) and, if it is,
// a variable or property access (TS2357/TS2777). Returns the result type.
func (c *Checker) checkUpdateOperand(operand parser.Expression, operandType types.Type) types.Type {
	if c.checkAssignmentTargetName(operand) {
		return types.Number
	}
	if id, ok := parser.UnwrapTypeAssertions(operand).(*parser.Identifier); ok && (id.Value == "eval" || id.Value == "arguments") {
		// Reported as a strict-mode or function-assignment error instead.
		return types.Number
	}
	if operandType == nil {
		operandType = types.Any
	}
	// checkNonNullType: a `null`/`undefined` literal is TS18050 (with or
	// without strictNullChecks) and then counts as non-nullable; a
	// possibly-nullish type is checked without its nullish part (tsc's
	// TS18047/TS18048 for it are not reported yet).
	if c.checkNullishOperand(parser.UnwrapTypeAssertions(operand)) {
		operandType = types.Never
	} else if t := types.RemoveNullUndefined(operandType); t != nil {
		operandType = t
	}
	if !c.arithmeticOperandOk(operandType, types.NewUnionType(types.Number, types.BigInt)) {
		c.addErrorWithCode(operand, errors.TS2356, "An arithmetic operand must be of type 'any', 'number', 'bigint' or an enum type.")
		return types.Number
	}
	c.checkReferenceExpression(operand,
		errors.TS2357, "The operand of an increment or decrement operator must be a variable or a property access.",
		errors.TS2777, "The operand of an increment or decrement operator may not be an optional property access.")
	if c.isBigIntLike(operandType) && !isAnyOrUnknownType(operandType) {
		return types.BigInt
	}
	return types.Number
}

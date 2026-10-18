package checker

import (
	"fmt"
	"strings"

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

// binaryOperands is an operator applied to two operands, as a binary
// expression or a compound assignment (whose node is the assignment).
type binaryOperands struct {
	op          string
	left, right parser.Expression
	node        parser.Node
}

func infixOperands(n *parser.InfixExpression) binaryOperands {
	return binaryOperands{op: n.Operator, left: n.Left, right: n.Right, node: n}
}

// reportOperatorNotApplicable reports TS2365 for an operator that has no
// meaning for this combination of operand types.
func (c *Checker) reportOperatorNotApplicable(b binaryOperands, left, right types.Type) {
	c.addErrorAtStart(b.node, errors.TS2365, fmt.Sprintf(
		"Operator '%s' cannot be applied to types '%s' and '%s'.",
		b.op, left.String(), right.String()))
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
func (c *Checker) checkArithmeticOperator(b binaryOperands, leftType, rightType types.Type) types.Type {
	if leftType == nil {
		leftType = types.Any
	}
	if rightType == nil {
		rightType = types.Any
	}
	op := strings.TrimSuffix(b.op, "=") // `x &= y` checks as `x & y`
	switch op {
	case "&", "|", "^":
		if c.isBooleanLikeOperand(leftType) && c.isBooleanLikeOperand(rightType) {
			suggest := map[string]string{"&": "&&", "|": "||", "^": "!=="}[op]
			c.addErrorWithCode(b.left, errors.TS2447, fmt.Sprintf(
				"The '%s' operator is not allowed for boolean types. Consider using '%s' instead.", b.op, suggest))
			return types.Number
		}
	}
	numberOrBigInt := types.NewUnionType(types.Number, types.BigInt)
	leftOk := c.arithmeticOperandOk(leftType, numberOrBigInt)
	rightOk := c.arithmeticOperandOk(rightType, numberOrBigInt)
	if !leftOk {
		c.addErrorWithCode(b.left, errors.TS2362,
			"The left-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type.")
	}
	if !rightOk {
		c.addErrorWithCode(b.right, errors.TS2363,
			"The right-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type.")
	}
	if (isAnyOrUnknownType(leftType) && isAnyOrUnknownType(rightType)) ||
		!(c.maybeBigIntLike(leftType) || c.maybeBigIntLike(rightType)) {
		return types.Number
	}
	if c.isBigIntLike(leftType) && c.isBigIntLike(rightType) {
		if op == ">>>" {
			c.reportOperatorNotApplicable(b, c.rel.GetWidenedType(leftType), c.rel.GetWidenedType(rightType))
		}
		return types.BigInt
	}
	if leftOk && rightOk {
		c.reportOperatorNotApplicable(b, c.rel.GetWidenedType(leftType), c.rel.GetWidenedType(rightType))
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

// checkRelationalOperator checks `<`, `>`, `<=` and `>=` as tsc does: a symbol
// operand is TS2469; otherwise the operands (literal types widened, nullish
// parts dropped) must both be numeric, or both non-numeric and comparable in
// either direction, or else the pair is TS2365.
func (c *Checker) checkRelationalOperator(b binaryOperands, leftType, rightType types.Type) {
	if leftType == nil || rightType == nil {
		return
	}
	if c.reportSymbolBinaryOperand(b, leftType, rightType) {
		return
	}
	l := c.comparisonBaseType(leftType)
	r := c.comparisonBaseType(rightType)
	if l == types.Any || r == types.Any {
		return
	}
	numberOrBigInt := types.NewUnionType(types.Number, types.BigInt)
	la := c.rel.IsAssignable(l, numberOrBigInt)
	ra := c.rel.IsAssignable(r, numberOrBigInt)
	if la && ra {
		return
	}
	if !la && !ra && (c.rel.IsComparable(l, r) || c.rel.IsComparable(r, l)) {
		return
	}
	c.reportOperatorNotApplicable(b, l, r)
}

// comparisonBaseType is getBaseTypeOfLiteralTypeForComparison applied after
// checkNonNullType: literals widen to their primitive and null/undefined
// constituents are dropped.
func (c *Checker) comparisonBaseType(t types.Type) types.Type {
	if stripped := types.RemoveNullUndefined(t); stripped != nil && stripped != types.Never {
		t = stripped
	}
	return c.widenLiteralUnion(c.rel.GetWidenedType(t))
}

// reportSymbolBinaryOperand is checkForDisallowedESSymbolOperand: a symbol
// operand of an arithmetic-like operator is TS2469, reported at the left
// operand if it may be a symbol, else at the right one.
func (c *Checker) reportSymbolBinaryOperand(b binaryOperands, leftType, rightType types.Type) bool {
	var offending parser.Expression
	switch {
	case maybeSymbolLike(leftType):
		offending = b.left
	case maybeSymbolLike(rightType):
		offending = b.right
	default:
		return false
	}
	c.addErrorWithCode(offending, errors.TS2469, fmt.Sprintf("The '%s' operator cannot be applied to type 'symbol'.", b.op))
	return true
}

func maybeSymbolLike(t types.Type) bool {
	switch tt := t.(type) {
	case *types.UnionType:
		for _, m := range tt.Types {
			if maybeSymbolLike(m) {
				return true
			}
		}
		return false
	case *types.IntersectionType:
		for _, m := range tt.Types {
			if maybeSymbolLike(m) {
				return true
			}
		}
		return false
	}
	return t == types.Symbol
}

// Type kinds for isTypeAssignableToKind.
const (
	kindNumberLike = 1 << iota
	kindBigIntLike
	kindStringLike
)

// assignableToKind is tsc's isTypeAssignableToKind: t is of one of the kinds
// outright, or (unless strict excludes any/unknown/void/undefined/null)
// assignable to the corresponding primitive.
func (c *Checker) assignableToKind(t types.Type, kinds int, strict bool) bool {
	if t == nil {
		return false
	}
	w := c.rel.GetWidenedType(t)
	if kinds&kindNumberLike != 0 && (w == types.Number && !isUnionType(t) || types.IsNumericEnumLikeType(t) && !isUnionType(t)) {
		return true
	}
	if kinds&kindBigIntLike != 0 && w == types.BigInt && !isUnionType(t) {
		return true
	}
	if kinds&kindStringLike != 0 && w == types.String && !isUnionType(t) {
		return true
	}
	if strict {
		switch t {
		case types.Any, types.Unknown, types.Void, types.Undefined, types.Null:
			return false
		}
	}
	return kinds&kindNumberLike != 0 && c.rel.IsAssignable(t, types.Number) ||
		kinds&kindBigIntLike != 0 && c.rel.IsAssignable(t, types.BigInt) ||
		kinds&kindStringLike != 0 && c.rel.IsAssignable(t, types.String)
}

func isUnionType(t types.Type) bool {
	_, ok := t.(*types.UnionType)
	return ok
}

// checkAdditionOperator checks `+` as tsc does. Unless either side is
// string-like, nullish parts are dropped (checkNonNullType). Two numbers add
// to number, two bigints to bigint, a string on either side concatenates, and
// an any operand gives any; anything else is TS2365.
func (c *Checker) checkAdditionOperator(b binaryOperands, leftType, rightType types.Type) types.Type {
	if leftType == nil {
		leftType = types.Any
	}
	if rightType == nil {
		rightType = types.Any
	}
	l, r := leftType, rightType
	if !c.assignableToKind(l, kindStringLike, false) && !c.assignableToKind(r, kindStringLike, false) {
		l, r = nonNullOperandType(l), nonNullOperandType(r)
	}
	var result types.Type
	switch {
	case c.assignableToKind(l, kindNumberLike, true) && c.assignableToKind(r, kindNumberLike, true):
		result = types.Number
	case c.assignableToKind(l, kindBigIntLike, true) && c.assignableToKind(r, kindBigIntLike, true):
		result = types.BigInt
	case c.assignableToKind(l, kindStringLike, true) || c.assignableToKind(r, kindStringLike, true):
		result = types.String
	case l == types.Any || r == types.Any:
		result = types.Any
	}
	if result != nil {
		c.reportSymbolBinaryOperand(b, l, r)
		return result
	}
	// reportOperatorError(isRelated): literal types are shown as their base
	// types unless the pair is "close enough" (getBaseTypesIfUnrelated).
	closeEnough := func(t types.Type) bool {
		return isAnyOrUnknownType(t) || c.assignableToKind(t, kindNumberLike|kindBigIntLike|kindStringLike, false)
	}
	if !closeEnough(c.literalBaseType(l)) || !closeEnough(c.literalBaseType(r)) {
		l, r = c.literalBaseType(l), c.literalBaseType(r)
	}
	c.reportOperatorNotApplicable(b, l, r)
	return types.Any
}

// nonNullOperandType drops the null and undefined constituents of an operand
// type, as checkNonNullType does once it has reported them.
func nonNullOperandType(t types.Type) types.Type {
	if stripped := types.RemoveNullUndefined(t); stripped != nil && stripped != types.Never {
		return stripped
	}
	return t
}

// literalBaseType is getBaseTypeOfLiteralType: a literal type (or a union's
// literal constituents) becomes its primitive; anything else is unchanged.
func (c *Checker) literalBaseType(t types.Type) types.Type {
	switch tt := t.(type) {
	case *types.LiteralType:
		return c.rel.GetWidenedType(tt)
	case *types.UnionType:
		members := make([]types.Type, len(tt.Types))
		for i, m := range tt.Types {
			members[i] = c.literalBaseType(m)
		}
		return types.NewUnionType(members...)
	}
	return t
}

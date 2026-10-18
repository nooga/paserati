package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// checkInfixExpression type-checks a binary expression. rightContext, when
// non-nil, is contextual type information to apply to the right operand —
// used so that e.g. `flag && (s => s)` infers its arrow parameter type from
// a surrounding call argument's expected type, the same way the arrow would
// if it were the argument directly.
func (c *Checker) checkInfixExpression(node *parser.InfixExpression, rightContext *ContextualType) {
	c.visit(node.Left)
	if c.noImplicitAny && (node.Operator == "||" || node.Operator == "??") {
		// `f || (x => ...)`: the left operand types the right one.
		if lt := node.Left.GetComputedType(); lt != nil && lt != types.Any && c.parameterTypeIsFunctionLike(types.RemoveNullishTypes(lt)) {
			c.markFunctionsUnder(node.Right, 0)
		}
	}

	// For && expressions, apply narrowing from left operand before checking right
	// This ensures that in `isObjectRecord(node) && node["fn"]`, the right side
	// sees `node` narrowed by the type predicate on the left side.
	// For || expressions, apply inverted narrowing (left is falsy, so right sees negation)
	// This ensures that in `!c.items || c.items.length === 0`, the right side
	// sees `c.items` narrowed to non-null/non-undefined.
	var savedEnvForLogical *Environment
	if node.Operator == "&&" {
		savedEnvForLogical = c.env
		narrowedEnv := c.applyTypeNarrowingFromCondition(node.Left)
		if narrowedEnv != nil {
			c.env = narrowedEnv
		}
	} else if node.Operator == "||" {
		// The right operand only runs when every `||` operand to its left was
		// falsy (`total === null || total <= 0`), so the left's guards are
		// applied inverted; this composes onto c.env.
		savedEnvForLogical = c.env
		c.applyInvertedOrNarrowing(node.Left)
	}

	if rightContext != nil {
		c.visitWithContext(node.Right, rightContext)
	} else {
		c.visit(node.Right)
	}

	// Restore environment after && narrowing
	if savedEnvForLogical != nil {
		c.env = savedEnvForLogical
	}

	leftType := node.Left.GetComputedType()

	if leftType == nil {
		leftType = types.Any
	}
	rightType := node.Right.GetComputedType()

	if rightType == nil {
		rightType = types.Any
	}

	widenedLeftType := c.rel.GetWidenedType(leftType)
	widenedRightType := c.rel.GetWidenedType(rightType)
	switch node.Operator {
	case "+", "-", "*", "/", "%", "**", "<", ">", "<=", ">=", "&", "|", "^", "<<", ">>", ">>>":
		// A union of same-kind literals (`flag ? 1 : 0`) operates as its primitive.
		widenedLeftType = c.widenLiteralUnion(widenedLeftType)
		widenedRightType = c.widenLiteralUnion(widenedRightType)
	}

	debugPrintf("// [Checker Infix Pre-Check] Left : %T (%v)\n", leftType, leftType)
	debugPrintf("// [Checker Infix Pre-Check] Right: %T (%v)\n", rightType, rightType)
	debugPrintf("// [Checker Infix Pre-Check] Widened Left : %T (%v)\n", widenedLeftType, widenedLeftType)
	debugPrintf("// [Checker Infix Pre-Check] Widened Right: %T (%v)\n", widenedRightType, widenedRightType)
	debugPrintf("// [Checker Infix Pre-Check] Check Condition: %v\n", widenedLeftType != nil && widenedRightType != nil)

	var resultType types.Type = types.Any // Default to Any on error

	// A `null` or `undefined` written directly as an operand of an
	// arithmetic, bitwise or relational operator is TS18050. TypeScript
	// reports it at the operand and then treats the operand as
	// non-nullable, so none of the operator-level diagnostics below fire.
	if c.checkNullishOperandsForOperator(node, leftType, rightType) {
		switch node.Operator {
		case "-", "*", "/", "%", "**", "&", "|", "^", "<<", ">>", ">>>":
			// The nullish side counts as never, so the other operand is
			// still checked (`null ** true` is TS18050 and TS2363).
			l, r := leftType, rightType
			if _, ok := nullishOperandName(node.Left); ok || l == nil {
				l = types.Never
			}
			if _, ok := nullishOperandName(node.Right); ok || r == nil {
				r = types.Never
			}
			c.checkArithmeticOperator(infixOperands(node), l, r)
		}
		node.SetComputedType(nullishOperatorResultType(node.Operator))
		return
	}

	if widenedLeftType != nil && widenedRightType != nil {
		debugPrintf("// [Checker Infix Pre-Check] Proceeding with operator: %s\n", node.Operator)
		switch node.Operator {
		case "+":
			resultType = c.checkAdditionOperator(infixOperands(node), leftType, rightType)
		case "-", "*", "/", "%", "**", "&", "|", "^", "<<", ">>", ">>>":
			resultType = c.checkArithmeticOperator(infixOperands(node), leftType, rightType)

		case "<", ">", "<=", ">=":
			c.checkRelationalOperator(infixOperands(node), leftType, rightType)
			resultType = types.Boolean
		case "==", "!=", "===", "!==":
			// Check for impossible comparisons before setting result type
			c.checkImpossibleComparison(leftType, rightType, node.Operator, node)
			// Comparison always results in boolean, even with 'any'
			resultType = types.Boolean
		case "in":
			// Property existence check: "prop" in obj
			c.checkInOperator(leftType, rightType, node)
			resultType = types.Boolean
		case "instanceof":
			// Instance check: obj instanceof Constructor
			c.checkInstanceofOperator(leftType, rightType, node)
			resultType = types.Boolean
		case "&&", "||", "??":
			if node.Operator != "&&" && isEmptyArrayLiteral(node.Right) {
				// A bare `[]` fallback is tsc's `never[]`, which subtype
				// reduction drops against the left operand's array type:
				// `xs || []` is `T[]`, not `unknown[]` (#636).
				rightType = &types.ArrayType{ElementType: types.Never}
			}
			resultType = c.logicalOperatorResultType(node.Operator, leftType, rightType)
		case ",":
			// Comma operator: evaluates both expressions but returns the type of the right expression
			// (left, right) -> right_type
			c.checkCommaOperator(node)
			resultType = rightType
			if checkerDebug {
				debugPrintf("// [Checker Comma] Left evaluated but discarded, result type: %s\n", rightType.String())
			}
		default:
			debugPrintf("// [Checker Infix Pre-Check] Proceeding with operator: %s\n", node.Operator)
			c.addError(node.Right, fmt.Sprintf("unsupported infix operator: %s", node.Operator))
		}
	} // else: Error already reported during operand check or types were nil

	debugPrintf("// [Checker Infix] Node: %p (%s), Determined ResultType: %T (%v)\n", node, node.Operator, resultType, resultType)
	node.SetComputedType(resultType)
}

// logicalOperatorResultType implements TypeScript's typing rule for &&, ||
// and ??: each operator unions the retained half of its left operand with
// the type of its right operand, with a short-circuit special case when the
// left operand alone decides the result.
func (c *Checker) logicalOperatorResultType(operator string, left, right types.Type) types.Type {
	if left == nil || right == nil {
		return types.Any
	}
	if c.rel.GetWidenedType(left) == types.Any {
		return types.Any
	}
	retained := retainedOperandType(operator, left)
	if retained == types.Never {
		// The left operand can never take the branch that would retain it,
		// so the right operand always decides the result.
		return right
	}
	if shortCircuitsToLeft(operator, left) {
		// The left operand always takes the branch that retains it, so the
		// right operand is unreachable.
		return left
	}
	if operator == "&&" {
		return types.NewUnionType(retained, right)
	}
	return c.rel.UnionWithSubtypeReduction(retained, right)
}

// retainedOperandType returns the part of the left operand that survives
// when the operator short-circuits on it.
func retainedOperandType(operator string, left types.Type) types.Type {
	switch operator {
	case "&&":
		return types.ExtractFalsyTypes(left)
	case "||":
		return types.RemoveFalsyTypes(left)
	case "??":
		return types.RemoveNullishTypes(left)
	}
	return left
}

// shortCircuitsToLeft reports whether the left operand always decides the
// result, making the right operand unreachable.
func shortCircuitsToLeft(operator string, left types.Type) bool {
	switch operator {
	case "&&":
		return types.RemoveFalsyTypes(left) == types.Never
	case "||":
		return types.ExtractFalsyTypes(left) == types.Never
	case "??":
		return types.ExtractNullishTypes(left) == types.Never
	}
	return false
}

// isEmptyArrayLiteral reports whether e is `[]`.
func isEmptyArrayLiteral(e parser.Expression) bool {
	arr, ok := e.(*parser.ArrayLiteral)
	return ok && len(arr.Elements) == 0
}

// widenLiteralUnion widens a union whose members are all literals of one
// primitive kind (`0 | 1`) to that primitive. Other types are unchanged.
func (c *Checker) widenLiteralUnion(t types.Type) types.Type {
	union, ok := t.(*types.UnionType)
	if !ok || len(union.Types) == 0 {
		return t
	}
	var kind types.Type
	for _, member := range union.Types {
		if _, isLit := member.(*types.LiteralType); !isLit {
			return t
		}
		w := c.rel.GetWidenedType(member)
		if kind == nil {
			kind = w
		} else if kind != w {
			return t
		}
	}
	return kind
}

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
		savedEnvForLogical = c.env
		invertedEnv := c.applyInvertedTruthinessNarrowing(node.Left)
		if invertedEnv != nil {
			c.env = invertedEnv
		}
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

	widenedLeftType := types.GetWidenedType(leftType)
	widenedRightType := types.GetWidenedType(rightType)

	debugPrintf("// [Checker Infix Pre-Check] Left : %T (%v)\n", leftType, leftType)
	debugPrintf("// [Checker Infix Pre-Check] Right: %T (%v)\n", rightType, rightType)
	debugPrintf("// [Checker Infix Pre-Check] Widened Left : %T (%v)\n", widenedLeftType, widenedLeftType)
	debugPrintf("// [Checker Infix Pre-Check] Widened Right: %T (%v)\n", widenedRightType, widenedRightType)
	debugPrintf("// [Checker Infix Pre-Check] Check Condition: %v\n", widenedLeftType != nil && widenedRightType != nil)

	var resultType types.Type = types.Any // Default to Any on error
	isAnyOperand := widenedLeftType == types.Any || widenedRightType == types.Any

	// A `null` or `undefined` written directly as an operand of an
	// arithmetic, bitwise or relational operator is TS18050. TypeScript
	// reports it at the operand and then treats the operand as
	// non-nullable, so none of the operator-level diagnostics below fire.
	if c.checkNullishOperandsForOperator(node, leftType, rightType) {
		node.SetComputedType(nullishOperatorResultType(node.Operator))
		return
	}

	if widenedLeftType != nil && widenedRightType != nil {
		debugPrintf("// [Checker Infix Pre-Check] Proceeding with operator: %s\n", node.Operator)
		switch node.Operator {
		case "+":
			leftIsNumeric := widenedLeftType == types.Number || types.IsNumericEnumLikeType(leftType)
			rightIsNumeric := widenedRightType == types.Number || types.IsNumericEnumLikeType(rightType)
			if isAnyOperand {
				resultType = types.Any
			} else if leftIsNumeric && rightIsNumeric {
				resultType = types.Number
			} else if widenedLeftType == types.BigInt && widenedRightType == types.BigInt {
				resultType = types.BigInt
			} else if (widenedLeftType == types.BigInt && widenedRightType == types.Number) ||
				(widenedLeftType == types.Number && widenedRightType == types.BigInt) {
				c.reportOperatorNotApplicable(node, widenedLeftType, widenedRightType)
				// Keep resultType = types.Any (default)
			} else if widenedLeftType == types.String && widenedRightType == types.String {
				resultType = types.String
				// <<< NEW: Handle String + Number/BigInt Coercion >>>
			} else if (widenedLeftType == types.String && widenedRightType == types.Number) ||
				(widenedLeftType == types.Number && widenedRightType == types.String) {
				resultType = types.String
			} else if (widenedLeftType == types.String && widenedRightType == types.BigInt) ||
				(widenedLeftType == types.BigInt && widenedRightType == types.String) {
				resultType = types.String
			} else if (widenedLeftType == types.String && widenedRightType == types.Boolean) ||
				(widenedLeftType == types.Boolean && widenedRightType == types.String) {
				resultType = types.String
			} else if (widenedLeftType == types.String && c.isStringConcatenatable(rightType)) ||
				(c.isStringConcatenatable(leftType) && widenedRightType == types.String) {
				// TypeScript allows string concatenation with most types (including unions)
				resultType = types.String
			} else if (widenedLeftType == types.Boolean || widenedLeftType == types.Null || widenedLeftType == types.Undefined) &&
				(widenedRightType == types.Boolean || widenedRightType == types.Null || widenedRightType == types.Undefined || widenedRightType == types.Number) {
				// JavaScript allows boolean/null/undefined in addition, they're coerced to numbers
				// true → 1, false → 0, null → 0, undefined → NaN
				resultType = types.Number
			} else if (widenedRightType == types.Boolean || widenedRightType == types.Null || widenedRightType == types.Undefined) &&
				(widenedLeftType == types.Number) {
				// Number + boolean/null/undefined → number
				resultType = types.Number
			} else if c.isObjectType(widenedLeftType) || c.isObjectType(widenedRightType) {
				// JavaScript allows objects in addition via ToPrimitive conversion
				// Object + anything or anything + Object → depends on ToPrimitive result
				// If ToPrimitive returns string, result is string; otherwise number
				// Conservative: assume string since that's most common for objects
				resultType = types.String
			} else {
				c.reportOperatorNotApplicable(node, widenedLeftType, widenedRightType)
				// Keep resultType = types.Any (default)
			}
		case "-", "*", "/":
			leftIsNumeric := widenedLeftType == types.Number || types.IsNumericEnumLikeType(leftType)
			rightIsNumeric := widenedRightType == types.Number || types.IsNumericEnumLikeType(rightType)
			if isAnyOperand {
				resultType = types.Any
			} else if leftIsNumeric && rightIsNumeric {
				resultType = types.Number
			} else if widenedLeftType == types.BigInt && widenedRightType == types.BigInt {
				resultType = types.BigInt
			} else if (widenedLeftType == types.BigInt && widenedRightType == types.Number) ||
				(widenedLeftType == types.Number && widenedRightType == types.BigInt) {
				c.reportOperatorNotApplicable(node, widenedLeftType, widenedRightType)
			} else if (widenedLeftType == types.String && widenedRightType == types.Number) ||
				(widenedLeftType == types.Number && widenedRightType == types.String) {
				resultType = types.Number
			} else if (widenedLeftType == types.String && widenedRightType == types.BigInt) ||
				(widenedLeftType == types.BigInt && widenedRightType == types.String) {
				resultType = types.BigInt
			} else {
				c.reportArithmeticOperandType(node, widenedLeftType)
				// Keep resultType = types.Any (default)
			}
		// --- Handle % and ** type checking ---
		case "%", "**":
			leftIsNumeric := widenedLeftType == types.Number || types.IsNumericEnumLikeType(leftType)
			rightIsNumeric := widenedRightType == types.Number || types.IsNumericEnumLikeType(rightType)
			if isAnyOperand {
				resultType = types.Any
			} else if leftIsNumeric && rightIsNumeric {
				resultType = types.Number
			} else if widenedLeftType == types.BigInt && widenedRightType == types.BigInt {
				resultType = types.BigInt
			} else if (widenedLeftType == types.BigInt && widenedRightType == types.Number) ||
				(widenedLeftType == types.Number && widenedRightType == types.BigInt) {
				c.reportOperatorNotApplicable(node, widenedLeftType, widenedRightType)
			} else {
				c.reportArithmeticOperandType(node, widenedLeftType)
			}

		// --- Handle Bitwise/Shift Operators ---
		case "&", "|", "^", "<<", ">>", ">>>":
			leftIsNumericBit := widenedLeftType == types.Number || types.IsNumericEnumLikeType(leftType)
			rightIsNumericBit := widenedRightType == types.Number || types.IsNumericEnumLikeType(rightType)
			if isAnyOperand {
				resultType = types.Number
			} else if leftIsNumericBit && rightIsNumericBit {
				resultType = types.Number
			} else if widenedLeftType == types.BigInt && widenedRightType == types.BigInt {
				// Both operands are BigInt, result is BigInt.
				resultType = types.BigInt
			} else if (widenedLeftType == types.BigInt && widenedRightType == types.Number) ||
				(widenedLeftType == types.Number && widenedRightType == types.BigInt) {
				// Mixing BigInt and Number is not allowed for bitwise/shift operations
				c.reportOperatorNotApplicable(node, widenedLeftType, widenedRightType)
				// Keep resultType = types.Any (default)
			} else {
				// Check if either operand is an object type (can be converted via valueOf/toString)
				leftIsObject := false
				rightIsObject := false

				switch widenedLeftType.(type) {
				case *types.ObjectType:
					leftIsObject = true
				}

				switch widenedRightType.(type) {
				case *types.ObjectType:
					rightIsObject = true
				}

				if leftIsObject && rightIsObject {
					// Both operands are objects (can be converted via valueOf/toString)
					resultType = types.Number
				} else if leftIsObject && widenedRightType == types.Number {
					// Left operand is object, right is number
					resultType = types.Number
				} else if widenedLeftType == types.Number && rightIsObject {
					// Left operand is number, right is object
					resultType = types.Number
				} else {
					// Operands are not compatible types for bitwise/shift operations.
					c.reportArithmeticOperandType(node, widenedLeftType)
					// Keep resultType = types.Any (default)
				}
			}
		// --- END NEW ---

		case "<", ">", "<=", ">=":
			leftIsNumericCmp := widenedLeftType == types.Number || types.IsNumericEnumLikeType(leftType)
			rightIsNumericCmp := widenedRightType == types.Number || types.IsNumericEnumLikeType(rightType)
			if isAnyOperand {
				resultType = types.Boolean
			} else if leftIsNumericCmp && rightIsNumericCmp {
				resultType = types.Boolean
			} else if widenedLeftType == types.String && widenedRightType == types.String {
				resultType = types.Boolean
			} else {
				c.reportOperatorNotApplicable(node, widenedLeftType, widenedRightType)
				resultType = types.Boolean
			}
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
			resultType = c.logicalOperatorResultType(node.Operator, leftType, rightType)
		case ",":
			// Comma operator: evaluates both expressions but returns the type of the right expression
			// (left, right) -> right_type
			c.checkCommaOperator(node)
			resultType = rightType
			debugPrintf("// [Checker Comma] Left evaluated but discarded, result type: %s\n", rightType.String())
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
	if types.GetWidenedType(left) == types.Any {
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
	return types.UnionWithSubtypeReduction(retained, right)
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

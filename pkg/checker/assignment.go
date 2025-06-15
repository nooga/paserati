package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

func (c *Checker) checkAssignmentExpression(node *parser.AssignmentExpression) {
	// Visit LHS (Identifier, IndexExpr, MemberExpr)
	c.visit(node.Left)
	lhsType := node.Left.GetComputedType()
	if lhsType == nil {
		lhsType = types.Any
	} // Handle nil from error

	// Visit RHS value
	c.visit(node.Value)
	rhsType := node.Value.GetComputedType()
	if rhsType == nil {
		rhsType = types.Any
	} // Handle nil from error

	// Widen types for operator checks
	widenedLhsType := types.GetWidenedType(lhsType) // Needed for operator checks AND assignability target
	widenedRhsType := types.GetWidenedType(rhsType)
	isAnyLhs := widenedLhsType == types.Any
	isAnyRhs := widenedRhsType == types.Any

	// Operator-Specific Pre-Checks
	validOperands := true
	switch node.Operator {
	// Arithmetic Compound Assignments (Check if LHS/RHS are numeric)
	case "+=", "-=", "*=", "/=", "%=", "**=":
		if !isAnyLhs && widenedLhsType != types.Number {
			// Exception: Allow string += any
			if !(node.Operator == "+=" && widenedLhsType == types.String) {
				c.addError(node.Left, fmt.Sprintf("operator '%s' requires LHS operand of type 'number' or 'any', got '%s'", node.Operator, widenedLhsType.String()))
				validOperands = false
			}
		}
		if !isAnyRhs && widenedRhsType != types.Number {
			// Exception: Allow string += any or number += string
			if !(node.Operator == "+=" && (widenedLhsType == types.String || widenedRhsType == types.String || isAnyRhs)) { // Adjusted check for RHS in +=
				c.addError(node.Value, fmt.Sprintf("operator '%s' requires RHS operand of type 'number', 'string' (if LHS is string), or 'any', got '%s'", node.Operator, widenedRhsType.String()))
				validOperands = false
			}
		}
		// Note: += specifically allows string concatenation, checks adjusted slightly.

	// Bitwise/Shift Compound Assignments (Require numeric operands)
	case "&=", "|=", "^=", "<<=", ">>=", ">>>=":
		if !isAnyLhs && widenedLhsType != types.Number {
			c.addError(node.Left, fmt.Sprintf("operator '%s' requires LHS operand of type 'number' or 'any', got '%s'", node.Operator, widenedLhsType.String()))
			validOperands = false
		}
		if !isAnyRhs && widenedRhsType != types.Number {
			c.addError(node.Value, fmt.Sprintf("operator '%s' requires RHS operand of type 'number' or 'any', got '%s'", node.Operator, widenedRhsType.String()))
			validOperands = false
		}

	// Logical/Coalesce Compound Assignments (No extra numeric checks needed)
	case "&&=", "||=", "??=":
		break // Handled by assignability check below

	case "=":
		// Simple assignment, no extra operator checks needed here.
		break

	default:
		c.addError(node, fmt.Sprintf("internal checker error: unhandled assignment operator %s", node.Operator))
		validOperands = false
	}

	// --- Check LHS const status ---
	// ... (keep existing const check) ...
	if identLHS, ok := node.Left.(*parser.Identifier); ok {
		_, isConst, found := c.env.Resolve(identLHS.Value)
		if found && isConst {
			c.addError(node.Left, fmt.Sprintf("cannot assign to constant variable '%s'", identLHS.Value))
			// Still proceed to check assignability for more errors
		}
	}
	// TODO: Check if MemberExpression LHS refers to a const property?

	// --- Final Assignability Check ---
	if validOperands {
		// <<< USE WIDENED LHS TYPE AS TARGET for assignability check >>>
		targetType := widenedLhsType
		// For simple identifiers, if a declared type exists, we should respect that *exact* type
		// instead of widening it for the target check.
		if identLHS, isIdent := node.Left.(*parser.Identifier); isIdent {
			resolvedType, _, found := c.env.Resolve(identLHS.Value)
			// Check if the *original* lhsType came directly from a declared type (annotation)
			// This is tricky to track perfectly. A simpler heuristic: if the original lhsType
			// isn't a literal type, maybe it came from an annotation or inference, so respect it.
			if found && resolvedType != nil {
				// If the resolved type is NOT a literal type, prefer it over the widened type.
				// This preserves stricter checking for annotated variables.
				if _, isLiteral := resolvedType.(*types.LiteralType); !isLiteral {
					targetType = resolvedType
				}
			}
		}

		if !types.IsAssignable(rhsType, targetType) { // <<< Use targetType (usually widened LHS)
			// Special case for ??= handled within isAssignable now?
			// Let's keep the explicit check here for clarity just for ??=
			allowAssignment := false
			if node.Operator == "??=" && (lhsType == types.Null || lhsType == types.Undefined) {
				// Allow ??= if LHS is null/undefined, check if RHS assignable to WIDENED LHS
				if types.IsAssignable(rhsType, widenedLhsType) { // Check assignability to widened target
					allowAssignment = true
				}
				// If RHS is not assignable even to widened LHS, error will be reported below
			}

			if !allowAssignment {
				leftDesc := "location"
				if ident, ok := node.Left.(*parser.Identifier); ok {
					leftDesc = fmt.Sprintf("variable '%s'", ident.Value)
				} else if _, ok := node.Left.(*parser.MemberExpression); ok {
					leftDesc = "property"
				} else if _, ok := node.Left.(*parser.IndexExpression); ok {
					leftDesc = "element"
				}
				// Report error comparing RHS to the potentially stricter targetType
				c.addError(node.Value, fmt.Sprintf("type '%s' is not assignable to %s of type '%s'", rhsType.String(), leftDesc, targetType.String()))
			}
		}
	}

	// Set computed type for the overall assignment expression (evaluates to RHS value)
	node.SetComputedType(rhsType)
}

// checkArrayDestructuringAssignment handles array destructuring assignments like [a, b, c] = expr
func (c *Checker) checkArrayDestructuringAssignment(node *parser.ArrayDestructuringAssignment) {
	// 1. Check RHS expression 
	c.visit(node.Value)
	rhsType := node.Value.GetComputedType()
	if rhsType == nil {
		rhsType = types.Any
	}

	// 2. Validate that RHS is array-like type
	widenedRhsType := types.GetWidenedType(rhsType)
	var elementType types.Type
	
	if arrayType, ok := widenedRhsType.(*types.ArrayType); ok {
		// Standard array type T[]
		elementType = arrayType.ElementType
	} else if tupleType, ok := widenedRhsType.(*types.TupleType); ok {
		// Tuple type [T1, T2, T3] - handle with precise type checking
		c.checkArrayDestructuringWithTuple(node, tupleType, rhsType)
		return
	} else if widenedRhsType == types.Any {
		// Allow destructuring of 'any' type
		elementType = types.Any
	} else {
		// RHS is not array-like
		c.addError(node.Value, fmt.Sprintf("type '%s' is not array-like and cannot be destructured", rhsType.String()))
		elementType = types.Any // Continue with Any to avoid cascading errors
	}

	// 3. For each destructuring element, assign appropriate type
	for i, element := range node.Elements {
		if element.Target == nil {
			continue // Skip malformed elements
		}

		// For Phase 1, only support Identifier targets
		if identTarget, ok := element.Target.(*parser.Identifier); ok {
			// Check if target variable is const
			_, isConst, found := c.env.Resolve(identTarget.Value)
			if found && isConst {
				c.addError(identTarget, fmt.Sprintf("cannot assign to constant variable '%s'", identTarget.Value))
			}

			// Set the computed type for the target
			identTarget.SetComputedType(elementType)
			
			// Update the variable's type in the environment
			c.env.Update(identTarget.Value, elementType)
			
		} else {
			// Only identifiers supported in Phase 1
			c.addError(element.Target, fmt.Sprintf("destructuring target at position %d must be an identifier", i))
		}
	}

	// 4. Set computed type for the overall expression (evaluates to RHS value)
	node.SetComputedType(rhsType)
}

// checkArrayDestructuringWithTuple handles array destructuring with tuple types for precise checking
func (c *Checker) checkArrayDestructuringWithTuple(node *parser.ArrayDestructuringAssignment, tupleType *types.TupleType, rhsType types.Type) {
	// For each destructuring element, assign the corresponding tuple element type
	for i, element := range node.Elements {
		if element.Target == nil {
			continue
		}

		var targetType types.Type
		if i < len(tupleType.ElementTypes) {
			// Use the precise type from the tuple
			targetType = tupleType.ElementTypes[i]
		} else {
			// More destructuring targets than tuple elements - undefined
			targetType = types.Undefined
		}

		// For Phase 1, only support Identifier targets
		if identTarget, ok := element.Target.(*parser.Identifier); ok {
			// Check if target variable is const
			_, isConst, found := c.env.Resolve(identTarget.Value)
			if found && isConst {
				c.addError(identTarget, fmt.Sprintf("cannot assign to constant variable '%s'", identTarget.Value))
			}

			// Set the computed type for the target
			identTarget.SetComputedType(targetType)
			
			// Update the variable's type in the environment
			c.env.Update(identTarget.Value, targetType)
			
		} else {
			// Only identifiers supported in Phase 1
			c.addError(element.Target, fmt.Sprintf("destructuring target at position %d must be an identifier", i))
		}
	}

	// Set computed type for the overall expression (evaluates to RHS value)
	node.SetComputedType(rhsType)
}

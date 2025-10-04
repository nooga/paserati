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

	// Visit RHS value with contextual typing if applicable
	if arrayLit, isArrayLit := node.Value.(*parser.ArrayLiteral); isArrayLit {
		// For array literals, provide contextual typing from the LHS
		contextualType := &ContextualType{ExpectedType: lhsType}
		c.checkArrayLiteralWithContext(arrayLit, contextualType)
	} else {
		c.visit(node.Value)
	}
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
	
	// --- Check LHS readonly status ---
	if memberLHS, ok := node.Left.(*parser.MemberExpression); ok {
		c.checkReadonlyPropertyAssignment(memberLHS)
	}

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

// checkReadonlyPropertyAssignment checks if a property assignment violates readonly constraints
func (c *Checker) checkReadonlyPropertyAssignment(memberExpr *parser.MemberExpression) {
	// Get the object type
	objectType := memberExpr.Object.GetComputedType()
	if objectType == nil {
		return // Can't check readonly if we don't know the object type
	}
	
	// Get the property name
	if memberExpr.Property == nil {
		return // No property to check
	}
	propertyName := c.extractPropertyName(memberExpr.Property)
	
	// Check if the object type has this property and if it's readonly
	if objType, ok := objectType.(*types.ObjectType); ok {
		if propType, exists := objType.Properties[propertyName]; exists {
			if types.IsReadonlyType(propType) {
				// In TypeScript, readonly properties can be assigned in constructors
				// Check if we're in a constructor context and assigning to 'this'
				if c.currentClassContext != nil && 
				   c.currentClassContext.ContextType == types.AccessContextConstructor &&
				   c.isThisExpression(memberExpr.Object) {
					// Allow readonly assignment in constructor when assigning to 'this'
					return
				}
				
				c.addError(memberExpr, fmt.Sprintf("cannot assign to readonly property '%s'", propertyName))
			}
		}
	}
}

// isThisExpression checks if an expression is a 'this' expression
func (c *Checker) isThisExpression(expr parser.Expression) bool {
	_, isThis := expr.(*parser.ThisExpression)
	return isThis
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

		var targetType types.Type
		if element.IsRest {
			// Rest element gets an array type containing the remaining elements
			targetType = &types.ArrayType{ElementType: elementType}
		} else {
			// Regular element gets the element type
			targetType = elementType
		}

		// Recursively check the target (supports nested patterns)
		c.checkDestructuringTarget(element.Target, targetType, i)
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
		if element.IsRest {
			// Rest element gets an array of remaining tuple elements
			if i < len(tupleType.ElementTypes) {
				// Create an array type with union of remaining elements
				remainingTypes := tupleType.ElementTypes[i:]
				if len(remainingTypes) == 0 {
					// No remaining elements, empty array
					targetType = &types.ArrayType{ElementType: types.Never}
				} else if len(remainingTypes) == 1 {
					// Single remaining type
					targetType = &types.ArrayType{ElementType: remainingTypes[0]}
				} else {
					// Multiple remaining types - create a union
					unionType := &types.UnionType{Types: remainingTypes}
					targetType = &types.ArrayType{ElementType: unionType}
				}
			} else {
				// Rest element beyond tuple length - empty array
				targetType = &types.ArrayType{ElementType: types.Never}
			}
		} else if i < len(tupleType.ElementTypes) {
			// Use the precise type from the tuple
			targetType = tupleType.ElementTypes[i]
		} else {
			// More destructuring targets than tuple elements - undefined
			targetType = types.Undefined
		}

		// Handle default values if present
		var finalTargetType types.Type = targetType
		if element.Default != nil {
			// Type-check the default value
			c.visit(element.Default)
			defaultType := element.Default.GetComputedType()
			if defaultType == nil {
				defaultType = types.Any
			}
			
			// Check that default value is assignable to expected element type
			if targetType != types.Undefined && targetType != types.Any {
				if !types.IsAssignable(defaultType, targetType) {
					c.addError(element.Default, fmt.Sprintf("default value type '%s' is not assignable to expected element type '%s'", defaultType.String(), targetType.String()))
				}
			}
			
			// Final type is the union of element type and default type (excluding undefined)
			if targetType == types.Undefined {
				// Element doesn't exist, so target gets default type
				finalTargetType = defaultType
			} else {
				// Element exists, so target could be either element type or default type
				// For simplicity, use the element type since default is only used when undefined
				finalTargetType = targetType
			}
		}

		// Recursively check the target (supports nested patterns)
		c.checkDestructuringTarget(element.Target, finalTargetType, i)
	}

	// Set computed type for the overall expression (evaluates to RHS value)
	node.SetComputedType(rhsType)
}

// checkObjectDestructuringAssignment handles object destructuring assignments like {a, b} = expr
func (c *Checker) checkObjectDestructuringAssignment(node *parser.ObjectDestructuringAssignment) {
	// 1. Check RHS expression 
	c.visit(node.Value)
	rhsType := node.Value.GetComputedType()
	if rhsType == nil {
		rhsType = types.Any
	}

	// 2. Widen the RHS type for compatibility checking
	widenedRhsType := types.GetWidenedType(rhsType)

	// 3. Validate that RHS is object-like (has properties)
	if widenedRhsType != types.Any {
		switch rhsType := rhsType.(type) {
		case *types.ObjectType:
			// Valid: object type with known properties (includes interfaces)
		default:
			// For Phase 2, we require object types or Any
			c.addError(node.Value, fmt.Sprintf("object destructuring requires RHS to be object-like, got '%s'", rhsType.String()))
			return
		}
	}

	// 4. For each destructuring property, check if it exists on RHS and infer target type
	for _, prop := range node.Properties {
		if prop.Target == nil {
			continue // Skip malformed properties
		}

		// Get the property name (skip computed properties for now)
		keyIdent, ok := prop.Key.(*parser.Identifier)
		if !ok {
			// Computed property - can't check statically
			continue
		}
		propName := keyIdent.Value

		// Determine the property type from RHS
		var propType types.Type = types.Any // Default fallback

		if widenedRhsType != types.Any {
			switch rhsType := rhsType.(type) {
			case *types.ObjectType:
				if foundPropType, exists := rhsType.Properties[propName]; exists {
					propType = foundPropType
				} else {
					// Property doesn't exist on object - will be undefined at runtime
					// In TypeScript, this is allowed but results in undefined
					propType = types.Undefined
				}
			}
		}

		// Handle default values if present
		var finalPropType types.Type = propType
		if prop.Default != nil {
			// Type-check the default value
			c.visit(prop.Default)
			defaultType := prop.Default.GetComputedType()
			if defaultType == nil {
				defaultType = types.Any
			}
			
			// Check that default value is assignable to expected property type
			if propType != types.Undefined && propType != types.Any {
				if !types.IsAssignable(defaultType, propType) {
					c.addError(prop.Default, fmt.Sprintf("default value type '%s' is not assignable to expected property type '%s'", defaultType.String(), propType.String()))
				}
			}
			
			// Final type is the union of property type and default type (excluding undefined)
			if propType == types.Undefined {
				// Property doesn't exist, so target gets default type
				finalPropType = defaultType
			} else {
				// Property exists, so target could be either property type or default type
				// For simplicity, use the property type since default is only used when undefined
				finalPropType = propType
			}
		}

		// Recursively check the target (supports nested patterns)
		c.checkDestructuringTargetForProperty(prop.Target, finalPropType, propName)
	}

	// 5. Handle rest property if present
	if node.RestProperty != nil {
		// Rest property gets an object type containing all remaining properties
		var restType types.Type
		
		if widenedRhsType == types.Any {
			// If RHS is Any, rest property is also Any
			restType = types.Any
		} else if objType, ok := rhsType.(*types.ObjectType); ok {
			// Create a new object type excluding the destructured properties
			extractedProps := make(map[string]struct{})
			for _, prop := range node.Properties {
				if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
					extractedProps[keyIdent.Value] = struct{}{}
				}
				// Skip computed properties (can't determine statically)
			}
			
			// Build remaining properties map
			remainingProps := make(map[string]types.Type)
			for propName, propType := range objType.Properties {
				if _, wasExtracted := extractedProps[propName]; !wasExtracted {
					remainingProps[propName] = propType
				}
			}
			
			// Create object type with remaining properties
			restType = &types.ObjectType{Properties: remainingProps}
		} else {
			// For other types, rest gets an empty object type
			restType = &types.ObjectType{Properties: make(map[string]types.Type)}
		}
		
		// Set type for rest property target
		if identTarget, ok := node.RestProperty.Target.(*parser.Identifier); ok {
			identTarget.SetComputedType(restType)
			c.env.Update(identTarget.Value, restType)
		}
	}

	// Set computed type for the overall expression (evaluates to RHS value)
	node.SetComputedType(rhsType)
}

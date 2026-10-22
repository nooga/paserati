package checker

import (
	"fmt"
	"github.com/nooga/paserati/pkg/errors"
	"slices"
	"strings"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

func (c *Checker) checkAssignmentExpression(node *parser.AssignmentExpression) {
	expando, expandoName := c.expandoAssignmentTarget(node)
	if expando != nil {
		// The property exists from its first assignment on; its type is
		// filled in below from the assigned value.
		if _, has := expando.Props()[expandoName]; !has {
			expando.WithProperty(expandoName, types.Any)
		}
	}

	// Visit LHS (Identifier, IndexExpr, MemberExpr)
	c.visit(node.Left)
	lhsType := node.Left.GetComputedType()
	if lhsType == nil {
		lhsType = types.Any
	} // Handle nil from error

	// Visit RHS value with contextual typing from the LHS. A narrowed variable
	// (`if (!b) { b = ... }`) is typed by its declared type, not the narrowed one.
	contextType := lhsType
	if identLHS, isIdent := node.Left.(*parser.Identifier); isIdent && node.Operator == "=" {
		if declared := c.env.ResolveDeclaredType(identLHS.Value); declared != nil {
			contextType = declared
		}
	}
	if expando != nil {
		contextType = nil
	}
	if contextType != nil {
		c.visitWithContext(node.Value, &ContextualType{ExpectedType: contextType})
	} else {
		c.visit(node.Value)
	}
	rhsType := node.Value.GetComputedType()
	if rhsType == nil {
		rhsType = types.Any
	} // Handle nil from error
	if expando != nil {
		widened := c.rel.DeeplyWidenType(rhsType)
		if prev := expando.Props()[expandoName]; prev != nil && prev != types.Any {
			widened = types.NewUnionType(prev, widened)
		}
		expando.WithProperty(expandoName, widened)
		node.SetComputedType(rhsType)
		return
	}

	// Widen types for operator checks
	widenedLhsType := c.rel.GetWidenedType(lhsType) // Needed for operator checks AND assignability target

	// Operator-Specific Pre-Checks
	validOperands := true
	assignedType := rhsType // what the operation hands to the target
	switch node.Operator {
	// Arithmetic, bitwise and shift compound assignments: TypeScript checks the
	// operator on the two operands and assigns the operation's result.
	case "+=", "-=", "*=", "/=", "%=", "**=", "&=", "|=", "^=", "<<=", ">>=", ">>>=":
		var resultType types.Type
		resultType, validOperands = c.compoundAssignmentResult(node, lhsType, rhsType)
		if validOperands {
			assignedType = resultType
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

	// checkAssignmentOperator: the target must be a variable or property
	// access, and a name that is not a variable is an error of its own.
	// tsc only gets here for compound arithmetic when both operands were ok.
	if c.checkAssignmentTargetName(node.Left) {
		validOperands = false
	} else if validOperands && !c.checkReferenceExpression(node.Left,
		errors.TS2364, "The left-hand side of an assignment expression must be a variable or a property access.",
		errors.TS2779, "The left-hand side of an assignment expression may not be an optional property access.") {
		validOperands = false
	}

	// --- Check LHS const status ---
	// ... (keep existing const check) ...
	if identLHS, ok := node.Left.(*parser.Identifier); ok {
		_, isConst, found := c.env.Resolve(identLHS.Value)
		if found && isConst {
			c.addErrorWithCode(node.Left, errors.TS2588, fmt.Sprintf("Cannot assign to '%s' because it is a constant.", identLHS.Value))
			// tsc types the target as an error type: nothing more is reported.
			validOperands = false
		}
	}

	// --- Check LHS readonly status ---
	// A write to a read-only target is reported as that alone, as tsc does.
	if c.checkReadonlyAssignmentTarget(node.Left) {
		validOperands = false
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
				} else if declared := c.env.ResolveDeclaredType(identLHS.Value); declared != nil {
					// A literal here may be a narrowing; the declared type is
					// the target (`let k: "a"` accepts only "a").
					targetType = declared
				}
			}
		}

		// A readonly member (writable only in its class's constructor) keeps
		// its literal type: `readonly n = 1` accepts only 1.
		if memberLHS, isMember := node.Left.(*parser.MemberExpression); isMember && memberLHS.Property != nil {
			if objT := memberLHS.Object.GetComputedType(); objT != nil && c.isReadonlyProperty(objT, c.extractPropertyName(memberLHS.Property), 0) {
				targetType = lhsType
			}
		}

		// An element access without a narrowing keeps its declared type,
		// literal or not (`[n: number]: 42` accepts only 42).
		if indexLHS, isIndex := node.Left.(*parser.IndexExpression); isIndex {
			if key := expressionToNarrowingKey(indexLHS); key == "" || c.resolveMemberExpressionOriginalType(key) == nil {
				targetType = lhsType
			}
		}

		// For member expressions, if the property has been narrowed (e.g., this._tools
		// narrowed from string[] | null to null), check assignment against the original
		// declared property type, not the narrowed type.
		if memberLHS, isMember := node.Left.(*parser.MemberExpression); isMember {
			key := expressionToNarrowingKey(memberLHS)
			if key != "" {
				// Check if there's a narrowing active for this member expression
				if origType := c.resolveMemberExpressionOriginalType(key); origType != nil {
					// Only use original type if it's wider than the narrowed type
					if !c.typesEqual(origType, lhsType) {
						targetType = origType
					}
				}
			}
		}

		// A property declared `Required<T>`/`Partial<T>` is still a mapped type here.
		targetType = c.expandIfMappedType(targetType)

		excessReported := false
		if node.Operator == "=" && c.rel.IsAssignable(assignedType, targetType) && c.findExcessProperty(node.Value, targetType) != nil {
			c.reportNotAssignable(node.Left, node.Value, assignedType, targetType, headAssign)
			excessReported = true
		}
		if !excessReported && !c.rel.IsAssignable(assignedType, targetType) { // <<< Use targetType (usually widened LHS)
			// If the resolved type rejected the RHS, check if we're in a narrowing scope
			// where the declared type is wider and would accept the assignment.
			// This handles: if (x === null) { x = "default"; } where x: string | null
			// is narrowed to null, but "default" is assignable to the declared string | null.
			allowAssignment := false
			if identLHS, isIdent := node.Left.(*parser.Identifier); isIdent {
				declaredType := c.env.ResolveDeclaredType(identLHS.Value)
				if declaredType != nil && declaredType != targetType {
					if c.rel.IsAssignable(assignedType, declaredType) {
						allowAssignment = true
					}
				}
			}

			// Special case for ??=
			if !allowAssignment && node.Operator == "??=" && (lhsType == types.Null || lhsType == types.Undefined) {
				if c.rel.IsAssignable(assignedType, widenedLhsType) {
					allowAssignment = true
				}
			}

			if !allowAssignment {
				// Report error comparing RHS to the potentially stricter targetType
				c.reportNotAssignable(node.Left, node.Value, assignedType, targetType, headAssign)
			}
		}
	}

	// After assignment, update the variable's type in the scope chain.
	// This enables narrowing after assignments: if (x === null) { x = value; }
	// should narrow x to the RHS type within the current scope.
	// We use UpdateInChain to walk up through block scopes and narrowing scopes
	// to find and update the nearest definition of the variable.
	if node.Operator == "=" {
		if identLHS, ok := node.Left.(*parser.Identifier); ok {
			if rhsType != nil && rhsType != types.Any {
				_, isConst, found := c.env.Resolve(identLHS.Value)
				// A rejected assignment leaves the declared type in place.
				if declared := c.env.ResolveDeclaredType(identLHS.Value); declared != nil && !c.rel.IsAssignable(c.rel.GetWidenedType(rhsType), declared) {
					found = false
				} else if declared == types.Any || declared == types.Unknown {
					// A declared any (or unknown) is never narrowed by assignment.
					found = false
				}
				if found && !isConst {
					// Update the type in the nearest defining scope to the widened RHS type
					widenedRhs := c.assignedWidened(rhsType)
					// The declared union reduced to what the value can be
					// (`r.D = {}` leaves the object member, not `{}`).
					if declared := c.env.ResolveDeclaredType(identLHS.Value); declared != nil {
						if reduced := c.assignmentReducedType(declared, widenedRhs); reduced != nil {
							widenedRhs = reduced
						}
					}
					c.env.UpdateInChain(identLHS.Value, widenedRhs)
					c.dropMemberNarrowings(identLHS.Value)
					if checkerDebug {
						debugPrintf("// [Checker Assignment] Updated '%s' type to '%s' after assignment\n",
							identLHS.Value, widenedRhs.String())
					}
				}
			}
		}

		// For member expression assignments (e.g., this._tools = [...]),
		// update the narrowings map so post-if merging can see the assignment effect.
		if memberLHS, ok := node.Left.(*parser.MemberExpression); ok {
			if rhsType != nil {
				key := expressionToNarrowingKey(memberLHS)
				if key != "" {
					widenedRhs := c.assignedWidened(rhsType)
					if c.hasNarrowingInChain(key) {
						if declared := c.resolveMemberExpressionOriginalType(key); declared != nil {
							if reduced := c.assignmentReducedType(declared, widenedRhs); reduced != nil {
								widenedRhs = reduced
							}
						}
						c.updateNarrowingInChain(key, widenedRhs)
					} else if narrowed := c.assignmentReducedType(memberLHS.GetComputedType(), widenedRhs); narrowed != nil {
						// `st.n = (st.n || 0) + 1` leaves st.n a number (#635).
						c.env.narrowings[key] = narrowed
					}
				}
			}
		}
	}

	// `x ??= v` leaves x non-nullish: its old value when that was non-nullish,
	// else v.
	if (node.Operator == "??=" || node.Operator == "||=") && validOperands && rhsType != nil && lhsType != nil {
		kept := types.RemoveNullishTypes(lhsType)
		if node.Operator == "||=" {
			kept = types.RemoveNullishTypes(kept)
		}
		if kept != nil && kept != types.Never && kept != types.Any && rhsType != types.Any {
			joined := types.NewUnionType(kept, c.rel.GetWidenedType(rhsType))
			if ident, ok := node.Left.(*parser.Identifier); ok {
				if _, isConst, found := c.env.Resolve(ident.Value); found && !isConst {
					c.env.UpdateInChain(ident.Value, joined)
					c.dropMemberNarrowings(ident.Value)
				}
			} else if member, ok := node.Left.(*parser.MemberExpression); ok {
				if key := expressionToNarrowingKey(member); key != "" {
					if c.hasNarrowingInChain(key) {
						c.updateNarrowingInChain(key, joined)
					} else {
						c.env.narrowings[key] = joined
					}
				}
			}
		}
	}

	// Set computed type for the overall assignment expression (evaluates to RHS value)
	node.SetComputedType(rhsType)
	switch node.Operator {
	case "&&=", "||=", "??=":
		// A logical assignment evaluates like the logical operator: the
		// retained part of the old value or the assigned value.
		if lhsType != nil && rhsType != nil {
			if isEmptyArrayLiteral(node.Value) {
				// A bare `[]` is tsc's `never[]` (see the logical operators).
				rhsType = &types.ArrayType{ElementType: types.Never}
			}
			node.SetComputedType(c.logicalOperatorResultType(strings.TrimSuffix(node.Operator, "="), lhsType, rhsType))
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
	saved := c.inDestructuringAssignment
	c.inDestructuringAssignment = true
	defer func() { c.inDestructuringAssignment = saved }()
	// 1. Check RHS expression
	c.visit(node.Value)
	rhsType := node.Value.GetComputedType()
	if rhsType == nil {
		rhsType = types.Any
	}

	// 2. Validate that RHS is array-like type
	widenedRhsType := c.rel.GetWidenedType(rhsType)
	if lit, ok := node.Value.(*parser.ArrayLiteral); ok {
		// An array literal destructured in place is a tuple of its elements.
		if tuple := c.arrayLiteralTuple(lit); tuple != nil {
			widenedRhsType = tuple
		}
	}
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
	} else if c.isGeneratorType(widenedRhsType) || c.isSpreadableIterableType(widenedRhsType) {
		elementType = c.getSpreadElementType(widenedRhsType)
	} else {
		// RHS is not array-like
		c.reportNotIterable(node.Value, rhsType)
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
			// Rest element gets the remaining tuple elements
			if tupleType.RestElementType == nil && !slices.Contains(tupleType.OptionalElements, true) {
				remaining := tupleType.ElementTypes[min(i, len(tupleType.ElementTypes)):]
				targetType = &types.TupleType{ElementTypes: append([]types.Type(nil), remaining...)}
			} else if i < len(tupleType.ElementTypes) {
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
				if !c.rel.IsAssignable(defaultType, targetType) {
					c.addErrorWithCode(element.Default, errors.TS2322, fmt.Sprintf("Type '%s' is not assignable to type '%s'.", defaultType.String(), targetType.String()))
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
	saved := c.inDestructuringAssignment
	c.inDestructuringAssignment = true
	defer func() { c.inDestructuringAssignment = saved }()
	// 1. Check RHS expression
	c.visit(node.Value)
	rhsType := node.Value.GetComputedType()
	if rhsType == nil {
		rhsType = types.Any
	}

	c.checkDestructuringExcess(node.Properties, node.RestProperty != nil, node.Value)
	rhsType = c.destructurableObject(rhsType)

	// 2. Widen the RHS type for compatibility checking
	widenedRhsType := c.rel.GetWidenedType(rhsType)

	// 3. Validate that RHS is object-like (has properties)
	if widenedRhsType != types.Any {
		switch rhsType := rhsType.(type) {
		case *types.ObjectType:
			// Valid: object type with known properties (includes interfaces)
		default:
			// For Phase 2, we require object types or Any
			c.reportBadObjectDestructure(node.Value, rhsType)
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
				if foundPropType, exists := c.resolveObjectMemberForDestructuring(rhsType, propName); exists {
					propType = foundPropType
				} else if prop.Default == nil {
					c.addErrorWithCode(prop.Key, errors.TS2339, fmt.Sprintf("Property '%s' does not exist on type '%s'.", propName, rhsType.String()))
				} else {
					// Property doesn't exist on object - will be undefined at runtime
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
				if !c.rel.IsAssignable(defaultType, propType) {
					c.addErrorWithCode(prop.Default, errors.TS2322, fmt.Sprintf("Type '%s' is not assignable to type '%s'.", defaultType.String(), propType.String()))
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
			for propName, propType := range objType.Props() {
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

		// The remaining properties are assigned to the rest target, which
		// keeps its declared type.
		switch target := node.RestProperty.Target.(type) {
		case *parser.Identifier:
			c.checkIdentifierTarget(target, restType)
		case *parser.MemberExpression, *parser.IndexExpression:
			c.visit(target)
		}
	}

	// Set computed type for the overall expression (evaluates to RHS value)
	node.SetComputedType(rhsType)
}

// assignmentReducedType is the declared union narrowed by an assignment: the
// members the assigned type can be. It returns nil when nothing narrows.
// assignedWidened widens an assigned literal for assignment narrowing. A
// type parameter stays itself: GetWidenedType's step to its constraint is
// for arithmetic, and `v = r` of `let v: R | symbol` narrows v to R.
func (c *Checker) assignedWidened(t types.Type) types.Type {
	if _, isParam := t.(*types.TypeParameterType); isParam {
		return t
	}
	return c.rel.GetWidenedType(t)
}

func (c *Checker) assignmentReducedType(declared, assigned types.Type) types.Type {
	union, ok := declared.(*types.UnionType)
	if !ok || assigned == nil || assigned == types.Any {
		return nil
	}
	assignedMembers := []types.Type{assigned}
	if au, ok := assigned.(*types.UnionType); ok {
		assignedMembers = au.Types
	}
	var kept []types.Type
	for _, m := range union.Types {
		for _, a := range assignedMembers {
			// A lib interface Paserati does not model (IteratorResult<T>)
			// is an object type: it narrows to the declared object members,
			// never to null, undefined or a primitive.
			if ref, ok := a.(*types.GenericTypeAliasForwardReference); ok && c.instantiateAliasReference(ref) == nil {
				if !isPrimitiveLike(m) {
					kept = append(kept, m)
					break
				}
				continue
			}
			if c.rel.IsAssignable(a, m) {
				kept = append(kept, m)
				break
			}
		}
	}
	if len(kept) == 0 || len(kept) == len(union.Types) {
		return nil
	}
	return types.NewUnionType(kept...)
}

// hasNarrowingInChain reports whether some enclosing scope narrows key.
func (c *Checker) hasNarrowingInChain(key string) bool {
	for e := c.env; e != nil; e = e.outer {
		if _, ok := e.narrowings[key]; ok {
			return true
		}
	}
	return false
}

// dropMemberNarrowings forgets the property narrowings under a reassigned
// binding: after `st = other`, what was known about `st.n` no longer holds.
func (c *Checker) dropMemberNarrowings(name string) {
	prefix := name + "."
	for e := c.env; e != nil; e = e.outer {
		for k := range e.narrowings {
			if strings.HasPrefix(k, prefix) {
				delete(e.narrowings, k)
			}
		}
	}
}

// arrayLiteralTuple is the tuple type of an array literal without spreads or
// holes, with widened element types; nil otherwise.
func (c *Checker) arrayLiteralTuple(lit *parser.ArrayLiteral) *types.TupleType {
	elems := make([]types.Type, 0, len(lit.Elements))
	for _, e := range lit.Elements {
		if isNilNode(e) {
			return nil
		}
		if _, spread := e.(*parser.SpreadElement); spread {
			return nil
		}
		t := e.GetComputedType()
		if t == nil {
			return nil
		}
		elems = append(elems, c.rel.GetWidenedType(t))
	}
	return &types.TupleType{ElementTypes: elems}
}

// collectExpandoNames returns the names of function declarations and of
// consts initialized by a function or arrow function anywhere in program.
func collectExpandoNames(program *parser.Program) map[string]map[string]bool {
	names := map[string]map[string]bool{}
	assigned := map[string]map[string]bool{}
	declare := func(name string) {
		if names[name] == nil {
			names[name] = map[string]bool{}
		}
	}
	var walk func(n parser.Node)
	walk = func(n parser.Node) {
		switch d := n.(type) {
		case *parser.ExportDefaultDeclaration:
			if fn, ok := d.Declaration.(*parser.FunctionLiteral); ok && d.IsDeclaration && fn.Name != nil {
				declare(fn.Name.Value)
			}
		case *parser.NamespaceDeclaration:
			if d.Body != nil {
				walk(d.Body)
			}
			return
		case *parser.ExpressionStatement:
			if fn, ok := d.Expression.(*parser.FunctionLiteral); ok && fn.Name != nil {
				declare(fn.Name.Value)
			}
			if ns, ok := d.Expression.(*parser.NamespaceDeclaration); ok {
				walk(ns)
				return
			}
			if as, ok := d.Expression.(*parser.AssignmentExpression); ok && as.Operator == "=" {
				if m, ok := as.Left.(*parser.MemberExpression); ok {
					obj, ok1 := m.Object.(*parser.Identifier)
					prop, ok2 := m.Property.(*parser.Identifier)
					if ok1 && ok2 {
						if assigned[obj.Value] == nil {
							assigned[obj.Value] = map[string]bool{}
						}
						assigned[obj.Value][prop.Value] = true
					}
				}
				// F["prop"] = ... declares prop like F.prop = ... does.
				if ix, ok := as.Left.(*parser.IndexExpression); ok {
					obj, ok1 := ix.Left.(*parser.Identifier)
					key, ok2 := ix.Index.(*parser.StringLiteral)
					if ok1 && ok2 {
						if assigned[obj.Value] == nil {
							assigned[obj.Value] = map[string]bool{}
						}
						assigned[obj.Value][key.Value] = true
					}
				}
			}
		case *parser.ConstStatement:
			for _, decl := range d.Declarations {
				switch decl.Value.(type) {
				case *parser.FunctionLiteral, *parser.ArrowFunctionLiteral:
					if decl.Name != nil {
						declare(decl.Name.Value)
					}
				}
			}
		}
		forEachChildInScope(n, walk)
	}
	walk(program)
	for name, props := range names {
		for p := range assigned[name] {
			props[p] = true
		}
	}
	return names
}

// declareExpandoRead gives a function's expando property a placeholder
// before its assignment is checked, for reads checked earlier (a hoisted
// function body reading its own expando property).
func (c *Checker) declareExpandoRead(node parser.Expression) {
	objName, propName, ok := expandoMember(node)
	if !ok || !c.expandoNames[objName][propName] {
		return
	}
	t, _, found := c.env.Resolve(objName)
	if !found {
		return
	}
	ot, ok := t.(*types.ObjectType)
	if !ok || !ot.IsCallable() || ot.IsClassConstructor() {
		return
	}
	if _, has := ot.Props()[propName]; !has {
		ot.WithProperty(propName, types.Any)
		if c.expandoProps == nil {
			c.expandoProps = map[expandoKey]bool{}
		}
		c.expandoProps[expandoKey{ot, propName}] = true
	}
}

// expandoAssignmentTarget returns the function type and property name of an
// expando assignment `f.prop = value`, where f is a function declaration or
// a const initialized by a function and prop is not one of its declared
// members.
func (c *Checker) expandoAssignmentTarget(node *parser.AssignmentExpression) (*types.ObjectType, string) {
	if node.Operator != "=" || len(c.expandoNames) == 0 {
		return nil, ""
	}
	objName, propName, ok := expandoMember(node.Left)
	if !ok {
		return nil, ""
	}
	if c.expandoNames[objName] == nil {
		return nil, ""
	}
	t, _, found := c.env.Resolve(objName)
	if !found {
		return nil, ""
	}
	ot, ok := t.(*types.ObjectType)
	if !ok || !ot.IsCallable() || ot.IsClassConstructor() {
		return nil, ""
	}
	if pt, has := ot.Props()[propName]; has && pt != types.Any && !c.expandoProps[expandoKey{ot, propName}] {
		return nil, ""
	}
	if c.expandoProps == nil {
		c.expandoProps = map[expandoKey]bool{}
	}
	c.expandoProps[expandoKey{ot, propName}] = true
	return ot, propName
}

// expandoMember splits an expando property reference, `f.prop` or
// `f["prop"]`, into the function's and the property's names.
func expandoMember(e parser.Expression) (obj, prop string, ok bool) {
	switch m := e.(type) {
	case *parser.MemberExpression:
		o, ok1 := m.Object.(*parser.Identifier)
		p, ok2 := m.Property.(*parser.Identifier)
		if ok1 && ok2 {
			return o.Value, p.Value, true
		}
	case *parser.IndexExpression:
		o, ok1 := m.Left.(*parser.Identifier)
		p, ok2 := m.Index.(*parser.StringLiteral)
		if ok1 && ok2 {
			return o.Value, p.Value, true
		}
	}
	return "", "", false
}

type expandoKey struct {
	fn   *types.ObjectType
	name string
}

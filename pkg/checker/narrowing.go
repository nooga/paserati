package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// TypeGuard represents a detected type guard pattern
type TypeGuard struct {
	VariableName string     // The variable being narrowed (e.g., "x")
	NarrowedType types.Type // The type it's narrowed to (e.g., types.String)
	IsNegated    bool       // true for !== checks, false for === checks
}

// detectTypeGuard analyzes a condition expression to detect type guard patterns like:
// typeof x === "string"
// typeof obj === "number"
// x === "foo" (literal narrowing)
// "bar" === y (literal narrowing)
// isString(x) (type predicate function calls)
func (c *Checker) detectTypeGuard(condition parser.Expression) *TypeGuard {
	// Pattern 0: Type predicate function calls like isString(x)
	if callExpr, ok := condition.(*parser.CallExpression); ok {
		// Check if this is a single-argument call to a function with type predicate return
		if len(callExpr.Arguments) == 1 {
			if ident, ok := callExpr.Arguments[0].(*parser.Identifier); ok {
				// Check if the function being called has a type predicate return type
				functionType := callExpr.Function.GetComputedType()
				if functionType != nil {
					if objType, ok := functionType.(*types.ObjectType); ok {
						if len(objType.CallSignatures) > 0 {
							returnType := objType.CallSignatures[0].ReturnType
							if predType, ok := returnType.(*types.TypePredicateType); ok {
								// This is a type predicate function call!
								// Extract the type being tested for
								return &TypeGuard{
									VariableName: ident.Value,
									NarrowedType: predType.Type,
									IsNegated:    false, // Type predicate calls are always positive
								}
							}
						}
					}
				}
			}
		}
	}

	// Look for infix comparison patterns
	if infix, ok := condition.(*parser.InfixExpression); ok {
		isPositive := infix.Operator == "===" || infix.Operator == "=="
		isNegative := infix.Operator == "!==" || infix.Operator == "!="
		
		if isPositive || isNegative {

			// Pattern 1: typeof identifier === "literal"
			if typeofExpr, ok := infix.Left.(*parser.TypeofExpression); ok {
				if ident, ok := typeofExpr.Operand.(*parser.Identifier); ok {
					if stringLit, ok := infix.Right.(*parser.StringLiteral); ok {
						// Map string literal to corresponding type
						var narrowedType types.Type
						switch stringLit.Value {
						case "string":
							narrowedType = types.String
						case "number":
							narrowedType = types.Number
						case "boolean":
							narrowedType = types.Boolean
						case "object":
							// For simplicity, we'll skip object narrowing for now
							// as it's more complex (could be null, array, etc.)
							return nil
						case "function":
							// Create a generic function type for typeof narrowing
							narrowedType = &types.ObjectType{
								CallSignatures: []*types.Signature{
									{
										ParameterTypes: []types.Type{},
										ReturnType:     types.Any,
										OptionalParams: []bool{},
										IsVariadic:     false,
									},
								},
							}
						case "undefined":
							narrowedType = types.Undefined
						default:
							return nil // Unknown type string
						}

						return &TypeGuard{
							VariableName: ident.Value,
							NarrowedType: narrowedType,
							IsNegated:    isNegative,
						}
					}
				}
			}

			// Pattern 2: identifier === literal (e.g., x === "foo")
			if ident, ok := infix.Left.(*parser.Identifier); ok {
				if narrowedType := c.literalToType(infix.Right); narrowedType != nil {
					return &TypeGuard{
						VariableName: ident.Value,
						NarrowedType: narrowedType,
						IsNegated:    isNegative,
					}
				}
			}

			// Pattern 3: literal === identifier (e.g., "foo" === x)
			if ident, ok := infix.Right.(*parser.Identifier); ok {
				if narrowedType := c.literalToType(infix.Left); narrowedType != nil {
					return &TypeGuard{
						VariableName: ident.Value,
						NarrowedType: narrowedType,
						IsNegated:    isNegative,
					}
				}
			}
		}
	}
	return nil
}

// literalToType converts a literal expression to its corresponding literal type
func (c *Checker) literalToType(expr parser.Expression) types.Type {
	switch lit := expr.(type) {
	case *parser.StringLiteral:
		return &types.LiteralType{Value: vm.NewString(lit.Value)}
	case *parser.NumberLiteral:
		return &types.LiteralType{Value: vm.NumberValue(lit.Value)}
	case *parser.BooleanLiteral:
		return &types.LiteralType{Value: vm.BooleanValue(lit.Value)}
	case *parser.NullLiteral:
		return types.Null
	case *parser.UndefinedLiteral:
		return types.Undefined
	default:
		return nil // Not a literal we can narrow to
	}
}

// applyTypeNarrowing applies type narrowing to the current environment
// Returns a new environment with the narrowed types, or nil if no narrowing was applied
// For negated type guards, this applies inverted narrowing
func (c *Checker) applyTypeNarrowing(guard *TypeGuard) *Environment {
	if guard == nil {
		return nil
	}
	
	// If the guard is negated (e.g., !== check), apply inverted narrowing instead
	if guard.IsNegated {
		return c.applyInvertedTypeNarrowing(guard)
	}

	return c.applyPositiveTypeNarrowing(guard)
}

// applyPositiveTypeNarrowing performs the actual positive type narrowing logic
func (c *Checker) applyPositiveTypeNarrowing(guard *TypeGuard) *Environment {
	// Check if the variable exists in the current environment
	originalType, isConst, found := c.env.Resolve(guard.VariableName)
	if !found {
		debugPrintf("// [TypeNarrowing] Variable '%s' not found for narrowing\n", guard.VariableName)
		return nil
	}

	// Handle unknown types and union types
	var canNarrow bool
	var narrowedType types.Type

	if originalType == types.Unknown {
		// Unknown can be narrowed to any specific type
		canNarrow = true
		narrowedType = guard.NarrowedType
	} else if unionType, ok := originalType.(*types.UnionType); ok {
		// For union types, check if we can narrow based on type compatibility
		if guard.NarrowedType != nil {
			// For typeof "function" checks, find callable members in the union
			if objType, ok := guard.NarrowedType.(*types.ObjectType); ok && objType.IsCallable() {
				var callableMembers []types.Type
				debugPrintf("// [TypeNarrowing] Checking union members for callable types\n")
				for _, memberType := range unionType.Types {
					debugPrintf("// [TypeNarrowing] Checking member: %s (type: %T)\n", memberType.String(), memberType)
					
					// Resolve type aliases to their underlying types
					resolvedType := c.resolveTypeAlias(memberType)
					debugPrintf("// [TypeNarrowing] Resolved member to: %s (type: %T)\n", resolvedType.String(), resolvedType)
					
					if memberObj, ok := resolvedType.(*types.ObjectType); ok && memberObj.IsCallable() {
						callableMembers = append(callableMembers, memberType) // Keep original for narrowed type
						debugPrintf("// [TypeNarrowing] Found callable member: %s\n", memberType.String())
					}
				}
				
				if len(callableMembers) > 0 {
					canNarrow = true
					if len(callableMembers) == 1 {
						narrowedType = callableMembers[0]
					} else {
						narrowedType = types.NewUnionType(callableMembers...)
					}
					debugPrintf("// [TypeNarrowing] Narrowed to callable types: %s\n", narrowedType.String())
				} else {
					debugPrintf("// [TypeNarrowing] No callable members found in union\n")
					return nil
				}
			} else if unionType.ContainsType(guard.NarrowedType) {
				// Regular type narrowing - union contains the exact target type
				canNarrow = true
				narrowedType = guard.NarrowedType
			} else {
				debugPrintf("// [TypeNarrowing] Union '%s' does not contain type '%s' - skipping narrowing\n",
					originalType.String(), guard.NarrowedType.String())
				return nil
			}
		}
	} else if types.IsAssignable(guard.NarrowedType, originalType) {
		// Allow narrowing if the narrowed type is assignable to the original type
		// This handles cases like narrowing 'string' to '"foo"' (literal type)
		canNarrow = true
		narrowedType = guard.NarrowedType
		debugPrintf("// [TypeNarrowing] Narrowing '%s' from '%s' to more specific type '%s'\n",
			guard.VariableName, originalType.String(), guard.NarrowedType.String())
	} else {
		debugPrintf("// [TypeNarrowing] Variable '%s' has type '%s', cannot narrow to '%s'\n",
			guard.VariableName, originalType.String(), guard.NarrowedType.String())
		return nil
	}

	if !canNarrow {
		return nil
	}

	// Create a new environment that inherits from the current one
	narrowedEnv := NewEnclosedEnvironment(c.env)

	// Define the variable with the narrowed type in the new environment
	// We redefine rather than update to shadow the outer scope
	success := narrowedEnv.Define(guard.VariableName, narrowedType, isConst)
	if !success {
		debugPrintf("// [TypeNarrowing] Failed to define narrowed type for '%s'\n", guard.VariableName)
		return nil
	}

	debugPrintf("// [TypeNarrowing] Narrowed variable '%s' from '%s' to '%s'\n",
		guard.VariableName, originalType.String(), narrowedType.String())

	return narrowedEnv
}

// applyInvertedTypeNarrowing creates an environment for the else branch with inverted type constraints
// For "if (typeof x === 'string')", the else branch knows x is NOT a string
// For negated guards (e.g., !== check), this applies positive narrowing instead
func (c *Checker) applyInvertedTypeNarrowing(guard *TypeGuard) *Environment {
	if guard == nil {
		return nil
	}
	
	// If the guard is negated (e.g., !== check), apply positive narrowing instead
	if guard.IsNegated {
		return c.applyPositiveTypeNarrowing(guard)
	}

	// Check if the variable exists in the current environment
	originalType, isConst, found := c.env.Resolve(guard.VariableName)
	if !found {
		debugPrintf("// [InvertedTypeNarrowing] Variable '%s' not found\n", guard.VariableName)
		return nil
	}

	// Handle union types: remove the narrowed type from the union
	if unionType, ok := originalType.(*types.UnionType); ok {
		if unionType.ContainsType(guard.NarrowedType) {
			remainingType := unionType.RemoveType(guard.NarrowedType)

			// Create environment with the remaining type(s)
			narrowedEnv := NewEnclosedEnvironment(c.env)
			success := narrowedEnv.Define(guard.VariableName, remainingType, isConst)
			if !success {
				debugPrintf("// [InvertedTypeNarrowing] Failed to define inverted narrowed type for '%s'\n", guard.VariableName)
				return nil
			}

			debugPrintf("// [InvertedTypeNarrowing] Variable '%s' narrowed from '%s' to '%s' in else branch\n",
				guard.VariableName, originalType.String(), remainingType.String())
			return narrowedEnv
		} else {
			debugPrintf("// [InvertedTypeNarrowing] Union '%s' does not contain type '%s' - no inverted narrowing\n",
				originalType.String(), guard.NarrowedType.String())
			return nil
		}
	}

	// For literal narrowing on non-union types, the else branch doesn't provide useful narrowing
	// (if x is string and we check x === "foo", in the else branch x is still string, just not "foo")
	// But for typeof narrowing on unknown, the else branch is still useful
	if originalType == types.Unknown {
		debugPrintf("// [InvertedTypeNarrowing] Variable '%s' remains unknown in else branch (but not %s)\n",
			guard.VariableName, guard.NarrowedType.String())
		return nil // No environment change needed for unknown
	}

	debugPrintf("// [InvertedTypeNarrowing] No inverted narrowing applied for type '%s'\n", originalType.String())
	return nil
}

// checkImpossibleComparison detects when two types have no overlap and comparison is impossible
// For example: comparing literal "foo" with literal "bar", or string with number
func (c *Checker) checkImpossibleComparison(leftType, rightType types.Type, operator string, node parser.Node) {
	// Only check strict equality and inequality operators
	if operator != "===" && operator != "!==" && operator != "==" && operator != "!=" {
		return
	}

	// Skip if either type is Any - anything can be compared to Any
	if leftType == types.Any || rightType == types.Any || leftType == types.Unknown || rightType == types.Unknown {
		return
	}

	// Check if the types have any overlap
	if !c.typesHaveOverlap(leftType, rightType) {
		c.addError(node, fmt.Sprintf("This comparison appears to be unintentional because the types '%s' and '%s' have no overlap.", leftType.String(), rightType.String()))
	}
}

// typesHaveOverlap checks if two types have any possible overlap
func (c *Checker) typesHaveOverlap(type1, type2 types.Type) bool {
	// Same types always overlap
	if type1.Equals(type2) {
		return true
	}

	// Any and Unknown overlap with everything
	if type1 == types.Any || type2 == types.Any || type1 == types.Unknown || type2 == types.Unknown {
		return true
	}
	
	// Special case for typeof checks: always allow checking against string literals "string", "number", etc.
	// This is a common pattern in TypeScript: typeof x === "string"
	if lit1, isLit1 := type1.(*types.LiteralType); isLit1 && lit1.Value.IsString() {
		strValue := lit1.Value.ToString()
		if strValue == "string" || strValue == "number" || strValue == "boolean" || 
		   strValue == "undefined" || strValue == "function" || strValue == "object" {
			return true // Allow typeof pattern
		}
	}
	
	if lit2, isLit2 := type2.(*types.LiteralType); isLit2 && lit2.Value.IsString() {
		strValue := lit2.Value.ToString()
		if strValue == "string" || strValue == "number" || strValue == "boolean" || 
		   strValue == "undefined" || strValue == "function" || strValue == "object" {
			return true // Allow typeof pattern
		}
	}

	// Handle union types - check if any member of one union overlaps with the other type
	if union1, ok := type1.(*types.UnionType); ok {
		for _, memberType := range union1.Types {
			if c.typesHaveOverlap(memberType, type2) {
				return true
			}
		}
		return false
	}

	if union2, ok := type2.(*types.UnionType); ok {
		for _, memberType := range union2.Types {
			if c.typesHaveOverlap(type1, memberType) {
				return true
			}
		}
		return false
	}

	// Handle literal types
	isLiteral1 := types.IsLiteral(type1)
	isLiteral2 := types.IsLiteral(type2)

	if isLiteral1 && isLiteral2 {
		// Both are literal types - they overlap only if they have the same value
		return type1.Equals(type2)
	}

	if isLiteral1 {
		// Check if literal type1 is assignable to type2
		return types.IsAssignable(type1, type2)
	}

	if isLiteral2 {
		// Check if literal type2 is assignable to type1
		return types.IsAssignable(type2, type1)
	}

	// Handle basic types - for strict equality, different primitive types don't overlap
	// Exception: allow comparisons with null/undefined as these are common runtime checks
	if type1 == types.Null || type1 == types.Undefined || type2 == types.Null || type2 == types.Undefined {
		return true // Allow comparisons with null/undefined
	}

	// Special case: Allow string comparison with object for typeof checks (common pattern)
	_, isObject1 := type1.(*types.ObjectType)
	_, isObject2 := type2.(*types.ObjectType)
	if (isObject1 && type2 == types.String) || (isObject2 && type1 == types.String) {
		return true // Allow object to be compared with string (for typeof checks)
	}
	
	// Special case: Allow number comparison with object for typeof checks (common pattern)
	if (isObject1 && type2 == types.Number) || (isObject2 && type1 == types.Number) {
		return true // Allow object to be compared with number (for typeof checks)
	}
	
	// Special case: Allow boolean comparison with object for typeof checks (common pattern)
	if (isObject1 && type2 == types.Boolean) || (isObject2 && type1 == types.Boolean) {
		return true // Allow object to be compared with boolean (for typeof checks)
	}
	
	widenedType1 := types.GetWidenedType(type1)
	widenedType2 := types.GetWidenedType(type2)
	return widenedType1 == widenedType2
}

// isLiteralAssignableToType checks if a literal type is assignable to another type
func (c *Checker) isLiteralAssignableToType(literal *types.LiteralType, targetType types.Type) bool {
	// A string literal is assignable to string type
	if literal.Value.IsString() && targetType == types.String {
		return true
	}
	// A number literal is assignable to number type
	if literal.Value.IsNumber() && targetType == types.Number {
		return true
	}
	// A boolean literal is assignable to boolean type
	if literal.Value.IsBoolean() && targetType == types.Boolean {
		return true
	}
	return false
}

// areLiteralValuesEqual checks if two literal values are equal
func (c *Checker) areLiteralValuesEqual(val1, val2 vm.Value) bool {
	// Use the vm.Value's built-in comparison methods
	return val1.StrictlyEquals(val2)
}

// resolveTypeAlias recursively resolves type aliases to their underlying types
func (c *Checker) resolveTypeAlias(t types.Type) types.Type {
	// Use the existing GetEffectiveType function which handles AliasType resolution
	effective := types.GetEffectiveType(t)
	if effective != t {
		debugPrintf("// [TypeNarrowing] Resolved alias %s -> %s\n", t.String(), effective.String())
		return effective
	}
	
	// Handle different type structures
	switch typ := t.(type) {
	case *types.InstantiatedType:
		// For instantiated generic types, try to substitute and resolve
		debugPrintf("// [TypeNarrowing] Resolving InstantiatedType: %s\n", typ.String())
		substituted := typ.Substitute()
		debugPrintf("// [TypeNarrowing] InstantiatedType substituted to: %s\n", substituted.String())
		return c.resolveTypeAlias(substituted) // Recursively resolve the result
	case *types.GenericTypeAliasForwardReference:
		// Try to resolve forward references by looking up the alias name
		debugPrintf("// [TypeNarrowing] Attempting to resolve GenericTypeAliasForwardReference: %s\n", typ.AliasName)
		if resolvedType, _, found := c.env.Resolve(typ.AliasName); found {
			debugPrintf("// [TypeNarrowing] Found type alias '%s' in environment: %s (type: %T)\n", typ.AliasName, resolvedType.String(), resolvedType)
			// If it's a generic type and we have type arguments, try to instantiate it
			if genericType, ok := resolvedType.(*types.GenericType); ok && len(typ.TypeArguments) > 0 {
				debugPrintf("// [TypeNarrowing] Instantiating generic type with %d args\n", len(typ.TypeArguments))
				instantiated := types.NewInstantiatedType(genericType, typ.TypeArguments)
				return c.resolveTypeAlias(instantiated.Substitute())
			}
			// Otherwise, just resolve the found type
			return c.resolveTypeAlias(resolvedType)
		} else {
			debugPrintf("// [TypeNarrowing] Could not resolve GenericTypeAliasForwardReference: %s\n", typ.AliasName)
		}
	default:
		// Not a resolvable type, return as-is
		return t
	}
	
	return t
}

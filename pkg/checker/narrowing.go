package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// TypeGuard represents a detected type guard pattern
type TypeGuard struct {
	VariableName      string     // The variable being narrowed (e.g., "x" or "this.value" or "obj.prop")
	NarrowedType      types.Type // The type it's narrowed to (e.g., types.String)
	IsNegated         bool       // true for !== checks, false for === checks
	DiscriminantProp  string     // For discriminated unions: the property being checked (e.g., "kind")
	DiscriminantValue types.Type // For discriminated unions: the value being compared (e.g., literal "num")
}

// expressionToNarrowingKey converts an expression to a string key for narrowing.
// Supports identifiers (e.g., "x") and member expressions (e.g., "this.value", "obj.prop").
func expressionToNarrowingKey(expr parser.Expression) string {
	switch e := expr.(type) {
	case *parser.Identifier:
		return e.Value
	case *parser.ThisExpression:
		return "this"
	case *parser.MemberExpression:
		// Recursively build the key for nested member expressions
		objectKey := expressionToNarrowingKey(e.Object)
		debugPrintf("// [expressionToNarrowingKey] MemberExpr: objectKey=%s, objectType=%T\n", objectKey, e.Object)
		if objectKey == "" {
			return ""
		}
		// Only handle simple property access (not computed properties like obj[expr])
		if propIdent, ok := e.Property.(*parser.Identifier); ok {
			return objectKey + "." + propIdent.Value
		}
		return ""
	default:
		debugPrintf("// [expressionToNarrowingKey] Unsupported expression type: %T\n", expr)
		return ""
	}
}

// detectTypeGuard analyzes a condition expression to detect type guard patterns like:
// typeof x === "string"
// typeof obj === "number"
// x === "foo" (literal narrowing)
// "bar" === y (literal narrowing)
// isString(x) (type predicate function calls)
// x && typeof x === "object" (compound conditions)
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

			// Pattern 1: typeof <expression> === "literal"
			// Now supports both identifiers (x) and member expressions (this.value, obj.prop)
			if typeofExpr, ok := infix.Left.(*parser.TypeofExpression); ok {
				narrowingKey := expressionToNarrowingKey(typeofExpr.Operand)
				debugPrintf("// [detectTypeGuard] typeof check detected, operand type: %T, key: %s\n", typeofExpr.Operand, narrowingKey)
				if narrowingKey != "" {
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
							// For typeof === "object", we need a special marker
							// We'll handle this in the narrowing logic to filter to object types only
							narrowedType = &types.ObjectTypeMarker{}
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
							VariableName: narrowingKey,
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

			// Pattern 3.5: member.property === literal (discriminated union narrowing)
			// e.g., expr.kind === "num" narrows expr to the union member where kind is "num"
			if memberExpr, ok := infix.Left.(*parser.MemberExpression); ok {
				if propIdent, ok := memberExpr.Property.(*parser.Identifier); ok {
					if discriminantValue := c.literalToType(infix.Right); discriminantValue != nil {
						baseKey := expressionToNarrowingKey(memberExpr.Object)
						if baseKey != "" {
							debugPrintf("// [detectTypeGuard] Discriminated union pattern: %s.%s === %s\n",
								baseKey, propIdent.Value, discriminantValue.String())
							return &TypeGuard{
								VariableName:      baseKey,
								NarrowedType:      nil, // Will be computed during narrowing
								IsNegated:         isNegative,
								DiscriminantProp:  propIdent.Value,
								DiscriminantValue: discriminantValue,
							}
						}
					}
				}
			}

			// Pattern 3.6: literal === member.property (discriminated union, reversed)
			if memberExpr, ok := infix.Right.(*parser.MemberExpression); ok {
				if propIdent, ok := memberExpr.Property.(*parser.Identifier); ok {
					if discriminantValue := c.literalToType(infix.Left); discriminantValue != nil {
						baseKey := expressionToNarrowingKey(memberExpr.Object)
						if baseKey != "" {
							debugPrintf("// [detectTypeGuard] Discriminated union pattern (reversed): %s === %s.%s\n",
								discriminantValue.String(), baseKey, propIdent.Value)
							return &TypeGuard{
								VariableName:      baseKey,
								NarrowedType:      nil, // Will be computed during narrowing
								IsNegated:         isNegative,
								DiscriminantProp:  propIdent.Value,
								DiscriminantValue: discriminantValue,
							}
						}
					}
				}
			}
		}

		// Pattern 4: "property" in identifier (e.g., "foo" in obj)
		if infix.Operator == "in" {
			if propLit, ok := infix.Left.(*parser.StringLiteral); ok {
				if ident, ok := infix.Right.(*parser.Identifier); ok {
					// Create a property existence marker
					return &TypeGuard{
						VariableName: ident.Value,
						NarrowedType: &types.PropertyExistenceMarker{PropertyName: propLit.Value},
						IsNegated:    false,
					}
				}
			}
			// Also handle Symbol in identifier (e.g., symbol in obj)
			if ident, ok := infix.Right.(*parser.Identifier); ok {
				// For symbol properties, we need to check if the left side is a symbol
				// For now, we'll create a general property existence check
				return &TypeGuard{
					VariableName: ident.Value,
					NarrowedType: &types.PropertyExistenceMarker{PropertyName: "[[Symbol]]"},
					IsNegated:    false,
				}
			}
		}

		// Pattern 5: identifier instanceof Constructor (e.g., date instanceof Date)
		if infix.Operator == "instanceof" {
			if ident, ok := infix.Left.(*parser.Identifier); ok {
				// For instanceof, we need to determine what type the constructor produces
				if constructorIdent, ok := infix.Right.(*parser.Identifier); ok {
					switch constructorIdent.Value {
					case "Date":
						// For Date instanceof, narrow to the Date instance type
						// Look up the actual Date constructor and get its instance type
						if dateType, _, found := c.env.Resolve("Date"); found {
							if objType, ok := dateType.(*types.ObjectType); ok && len(objType.CallSignatures) > 0 {
								// Use the return type of the constructor (the instance type)
								instanceType := objType.CallSignatures[0].ReturnType
								return &TypeGuard{
									VariableName: ident.Value,
									NarrowedType: instanceType,
									IsNegated:    false,
								}
							}
						}
					case "Array":
						// For Array, narrow to a generic array type
						return &TypeGuard{
							VariableName: ident.Value,
							NarrowedType: &types.ArrayType{ElementType: types.Any},
							IsNegated:    false,
						}
					case "Object":
						// For Object, narrow to a generic object type
						return &TypeGuard{
							VariableName: ident.Value,
							NarrowedType: types.NewObjectType(),
							IsNegated:    false,
						}
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

// applyTypeNarrowingWithFallback tries the old approach first, then falls back to compound narrowing
func (c *Checker) applyTypeNarrowingWithFallback(condition parser.Expression) *Environment {
	// First try the traditional single type guard approach
	guard := c.detectTypeGuard(condition)
	env := c.applyTypeNarrowing(guard)
	if env != nil {
		debugPrintf("// [TypeNarrowing] Used traditional approach\n")
		return env
	}

	// If that didn't work, try the compound approach
	debugPrintf("// [TypeNarrowing] Trying compound approach for condition: %T\n", condition)
	return c.applyTypeNarrowingFromCondition(condition)
}

// applyPositiveTypeNarrowing performs the actual positive type narrowing logic
func (c *Checker) applyPositiveTypeNarrowing(guard *TypeGuard) *Environment {
	// Check if this is a member expression narrowing (contains a dot)
	isMemberExpression := false
	for _, ch := range guard.VariableName {
		if ch == '.' {
			isMemberExpression = true
			break
		}
	}

	var originalType types.Type
	var isConst bool
	var found bool

	if isMemberExpression {
		// For member expressions like "this.value" or "obj.prop", we store the narrowing
		// in a separate map that will be consulted when checking member expressions

		var narrowedType types.Type

		// Check if this is a discriminant guard on a member expression
		if guard.DiscriminantProp != "" && guard.DiscriminantValue != nil {
			// For discriminant guards, we need to compute the narrowed type
			// First, get the original type of the member expression from the narrowings or by resolving
			var originalType types.Type
			if existingNarrowing, exists := c.env.narrowings[guard.VariableName]; exists {
				originalType = existingNarrowing
			} else {
				// Try to resolve the root identifier to infer the member's type
				// For now, we'll return nil - discriminant narrowing on member expressions
				// requires more complex resolution
				debugPrintf("// [TypeNarrowing] Discriminant guard on member expression %s.%s, but cannot resolve original type\n",
					guard.VariableName, guard.DiscriminantProp)
				return nil
			}

			// The original type must be a union for discriminated narrowing
			if unionType, ok := originalType.(*types.UnionType); ok {
				var matchingMembers []types.Type
				for _, memberType := range unionType.Types {
					if c.memberMatchesDiscriminant(memberType, guard.DiscriminantProp, guard.DiscriminantValue) {
						matchingMembers = append(matchingMembers, memberType)
						debugPrintf("// [TypeNarrowing] Member expr union member matches discriminant: %s\n", memberType.String())
					}
				}

				if len(matchingMembers) > 0 {
					if len(matchingMembers) == 1 {
						narrowedType = matchingMembers[0]
					} else {
						narrowedType = types.NewUnionType(matchingMembers...)
					}
				}
			}

			if narrowedType == nil {
				debugPrintf("// [TypeNarrowing] Member expr discriminant narrowing failed for %s\n", guard.VariableName)
				return nil
			}
		} else {
			narrowedType = guard.NarrowedType
			if narrowedType == nil {
				debugPrintf("// [TypeNarrowing] Member expression narrowing has nil type for %s\n", guard.VariableName)
				return nil
			}
		}

		debugPrintf("// [TypeNarrowing] Member expression narrowing detected: %s -> %s\n", guard.VariableName, narrowedType.String())

		// Create a new environment with the narrowing
		newEnv := NewEnclosedEnvironment(c.env)
		// Copy existing narrowings from the current environment
		for k, v := range c.env.narrowings {
			newEnv.narrowings[k] = v
		}
		// Also copy from outer environments if they have narrowings
		if c.env.outer != nil && c.env.outer.narrowings != nil {
			for k, v := range c.env.outer.narrowings {
				if _, exists := newEnv.narrowings[k]; !exists {
					newEnv.narrowings[k] = v
				}
			}
		}
		// Add the new narrowing
		newEnv.narrowings[guard.VariableName] = narrowedType
		debugPrintf("// [TypeNarrowing] Stored member narrowing in environment\n")
		return newEnv
	}

	// Regular identifier narrowing
	originalType, isConst, found = c.env.Resolve(guard.VariableName)
	if !found {
		debugPrintf("// [TypeNarrowing] Variable '%s' not found for narrowing\n", guard.VariableName)
		return nil
	}

	// Handle discriminated union narrowing (e.g., expr.kind === "num")
	if guard.DiscriminantProp != "" && guard.DiscriminantValue != nil {
		debugPrintf("// [TypeNarrowing] Discriminated union narrowing: %s.%s === %s\n",
			guard.VariableName, guard.DiscriminantProp, guard.DiscriminantValue.String())

		// The original type must be a union for discriminated narrowing
		if unionType, ok := originalType.(*types.UnionType); ok {
			var matchingMembers []types.Type
			for _, memberType := range unionType.Types {
				// Check if this union member has the discriminant property with a matching value
				if c.memberMatchesDiscriminant(memberType, guard.DiscriminantProp, guard.DiscriminantValue) {
					matchingMembers = append(matchingMembers, memberType)
					debugPrintf("// [TypeNarrowing] Union member matches discriminant: %s\n", memberType.String())
				}
			}

			if len(matchingMembers) > 0 {
				var narrowedType types.Type
				if len(matchingMembers) == 1 {
					narrowedType = matchingMembers[0]
				} else {
					narrowedType = types.NewUnionType(matchingMembers...)
				}

				debugPrintf("// [TypeNarrowing] Narrowed '%s' to discriminated type: %s\n",
					guard.VariableName, narrowedType.String())

				narrowedEnv := NewEnclosedEnvironment(c.env)
				narrowedEnv.Define(guard.VariableName, narrowedType, isConst)
				return narrowedEnv
			}
		}

		debugPrintf("// [TypeNarrowing] Discriminated union narrowing failed for '%s'\n", guard.VariableName)
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
			// Handle special marker types for compound type narrowing
			if _, ok := guard.NarrowedType.(*types.ObjectTypeMarker); ok {
				// typeof === "object" - keep only object-like types (objects, arrays, but not primitives)
				var objectMembers []types.Type
				for _, memberType := range unionType.Types {
					if c.isObjectLikeType(memberType) {
						objectMembers = append(objectMembers, memberType)
					}
				}
				if len(objectMembers) > 0 {
					canNarrow = true
					if len(objectMembers) == 1 {
						narrowedType = objectMembers[0]
					} else {
						narrowedType = types.NewUnionType(objectMembers...)
					}
					debugPrintf("// [TypeNarrowing] Narrowed to object types: %s\n", narrowedType.String())
				} else {
					debugPrintf("// [TypeNarrowing] No object types found in union\n")
					return nil
				}
			} else if propMarker, ok := guard.NarrowedType.(*types.PropertyExistenceMarker); ok {
				// "prop" in obj - keep only types that have the property
				var validMembers []types.Type
				for _, memberType := range unionType.Types {
					if c.typeHasProperty(memberType, propMarker.PropertyName) {
						validMembers = append(validMembers, memberType)
					}
				}
				if len(validMembers) > 0 {
					canNarrow = true
					if len(validMembers) == 1 {
						narrowedType = validMembers[0]
					} else {
						narrowedType = types.NewUnionType(validMembers...)
					}
					debugPrintf("// [TypeNarrowing] Narrowed to types with property '%s': %s\n", propMarker.PropertyName, narrowedType.String())
				} else {
					debugPrintf("// [TypeNarrowing] No types with property '%s' found in union\n", propMarker.PropertyName)
					return nil
				}
				// For typeof "function" checks, find callable members in the union
			} else if objType, ok := guard.NarrowedType.(*types.ObjectType); ok && objType.IsCallable() {
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
				// For instanceof narrowing, filter union members that are compatible with the narrowed type
				var compatibleMembers []types.Type
				for _, memberType := range unionType.Types {
					// Check if this member is compatible with the narrowed type
					if c.isTypeCompatibleForInstanceof(memberType, guard.NarrowedType) {
						compatibleMembers = append(compatibleMembers, memberType)
					}
				}

				if len(compatibleMembers) > 0 {
					canNarrow = true
					if len(compatibleMembers) == 1 {
						narrowedType = compatibleMembers[0]
					} else {
						narrowedType = types.NewUnionType(compatibleMembers...)
					}
					debugPrintf("// [TypeNarrowing] Narrowed to compatible types: %s\n", narrowedType.String())
				} else {
					debugPrintf("// [TypeNarrowing] Union '%s' does not contain type '%s' - skipping narrowing\n",
						originalType.String(), guard.NarrowedType.String())
					return nil
				}
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

	// Check if this is a member expression narrowing
	isMemberExpression := false
	for _, ch := range guard.VariableName {
		if ch == '.' {
			isMemberExpression = true
			break
		}
	}

	if isMemberExpression {
		// For member expressions, we need to apply complement narrowing
		// We need to get the original type to compute the complement
		// For typeof checks on union types, the complement is "union minus the narrowed type"

		// Check if this is a discriminant guard on a member expression
		if guard.DiscriminantProp != "" && guard.DiscriminantValue != nil {
			// For discriminant guards on member expressions, we need to compute the complement
			var originalType types.Type
			if existingNarrowing, exists := c.env.narrowings[guard.VariableName]; exists {
				originalType = existingNarrowing
			} else {
				debugPrintf("// [InvertedTypeNarrowing] Discriminant guard on member expression %s.%s, but cannot resolve original type\n",
					guard.VariableName, guard.DiscriminantProp)
				return nil
			}

			if unionType, ok := originalType.(*types.UnionType); ok {
				var nonMatchingMembers []types.Type
				for _, memberType := range unionType.Types {
					if !c.memberMatchesDiscriminant(memberType, guard.DiscriminantProp, guard.DiscriminantValue) {
						nonMatchingMembers = append(nonMatchingMembers, memberType)
						debugPrintf("// [InvertedTypeNarrowing] Member expr union member does not match: %s\n", memberType.String())
					}
				}

				if len(nonMatchingMembers) > 0 {
					var narrowedType types.Type
					if len(nonMatchingMembers) == 1 {
						narrowedType = nonMatchingMembers[0]
					} else {
						narrowedType = types.NewUnionType(nonMatchingMembers...)
					}

					newEnv := NewEnclosedEnvironment(c.env)
					for k, v := range c.env.narrowings {
						newEnv.narrowings[k] = v
					}
					if c.env.outer != nil && c.env.outer.narrowings != nil {
						for k, v := range c.env.outer.narrowings {
							if _, exists := newEnv.narrowings[k]; !exists {
								newEnv.narrowings[k] = v
							}
						}
					}
					newEnv.narrowings[guard.VariableName] = narrowedType
					debugPrintf("// [InvertedTypeNarrowing] Member expr discriminant else narrowing: %s -> %s\n",
						guard.VariableName, narrowedType.String())
					return newEnv
				}
			}

			debugPrintf("// [InvertedTypeNarrowing] Member expr discriminant else narrowing failed for %s\n", guard.VariableName)
			return nil
		}

		// Non-discriminant member expression complement narrowing
		if guard.NarrowedType == nil {
			debugPrintf("// [InvertedTypeNarrowing] Member expression complement narrowing has nil type for %s\n", guard.VariableName)
			return nil
		}

		debugPrintf("// [InvertedTypeNarrowing] Member expression complement narrowing: %s\n", guard.VariableName)

		// Create a special marker indicating this is a complement narrowing
		// The actual narrowing will be applied when checking the member expression
		// For now, we store the guard with IsNegated=true to indicate complement
		newEnv := NewEnclosedEnvironment(c.env)
		// Copy existing narrowings
		for k, v := range c.env.narrowings {
			newEnv.narrowings[k] = v
		}
		// Also copy from outer environments
		if c.env.outer != nil && c.env.outer.narrowings != nil {
			for k, v := range c.env.outer.narrowings {
				if _, exists := newEnv.narrowings[k]; !exists {
					newEnv.narrowings[k] = v
				}
			}
		}
		// Store a complement narrowing marker (we'll handle this specially in checkMemberExpression)
		// For typeof checks, we can create a complement marker
		// This is a simplified approach: just mark it as "not the narrowed type"
		newEnv.narrowings[guard.VariableName+"__complement"] = guard.NarrowedType
		debugPrintf("// [InvertedTypeNarrowing] Stored complement marker for %s\n", guard.VariableName)
		return newEnv
	}

	// Check if the variable exists in the current environment
	originalType, isConst, found := c.env.Resolve(guard.VariableName)
	if !found {
		debugPrintf("// [InvertedTypeNarrowing] Variable '%s' not found\n", guard.VariableName)
		return nil
	}

	// Handle discriminated union narrowing in else branch
	// e.g., after "if (expr.kind === 'num')", the else branch has all members except those with kind='num'
	if guard.DiscriminantProp != "" && guard.DiscriminantValue != nil {
		debugPrintf("// [InvertedTypeNarrowing] Discriminated union else branch: %s.%s !== %s\n",
			guard.VariableName, guard.DiscriminantProp, guard.DiscriminantValue.String())

		if unionType, ok := originalType.(*types.UnionType); ok {
			var nonMatchingMembers []types.Type
			for _, memberType := range unionType.Types {
				if !c.memberMatchesDiscriminant(memberType, guard.DiscriminantProp, guard.DiscriminantValue) {
					nonMatchingMembers = append(nonMatchingMembers, memberType)
					debugPrintf("// [InvertedTypeNarrowing] Union member does not match: %s\n", memberType.String())
				}
			}

			if len(nonMatchingMembers) > 0 {
				var narrowedType types.Type
				if len(nonMatchingMembers) == 1 {
					narrowedType = nonMatchingMembers[0]
				} else {
					narrowedType = types.NewUnionType(nonMatchingMembers...)
				}

				debugPrintf("// [InvertedTypeNarrowing] Narrowed '%s' to non-matching types: %s\n",
					guard.VariableName, narrowedType.String())

				narrowedEnv := NewEnclosedEnvironment(c.env)
				narrowedEnv.Define(guard.VariableName, narrowedType, isConst)
				return narrowedEnv
			}
		}

		debugPrintf("// [InvertedTypeNarrowing] Discriminated union else branch failed for '%s'\n", guard.VariableName)
		return nil
	}

	// Handle union types: remove the narrowed type from the union
	if unionType, ok := originalType.(*types.UnionType); ok {
		if guard.NarrowedType != nil && unionType.ContainsType(guard.NarrowedType) {
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

	// Handle enum member types - they overlap with their underlying values
	if enumMember1, ok := type1.(*types.EnumMemberType); ok {
		// Check if enum member's value overlaps with type2
		if enumMember1.Value != nil {
			switch val := enumMember1.Value.(type) {
			case int:
				// Numeric enum member overlaps with number literals and number type
				if lit2, isLit2 := type2.(*types.LiteralType); isLit2 && lit2.Value.IsNumber() {
					if lit2.Value.IsIntegerNumber() {
						return int(lit2.Value.AsInteger()) == val
					} else {
						return int(lit2.Value.AsFloat()) == val
					}
				}
				return type2 == types.Number
			case string:
				// String enum member overlaps with string literals and string type
				if lit2, isLit2 := type2.(*types.LiteralType); isLit2 && lit2.Value.IsString() {
					return lit2.Value.ToString() == val
				}
				return type2 == types.String
			}
		}
	}

	if enumMember2, ok := type2.(*types.EnumMemberType); ok {
		// Check if enum member's value overlaps with type1
		if enumMember2.Value != nil {
			switch val := enumMember2.Value.(type) {
			case int:
				// Numeric enum member overlaps with number literals and number type
				if lit1, isLit1 := type1.(*types.LiteralType); isLit1 && lit1.Value.IsNumber() {
					if lit1.Value.IsIntegerNumber() {
						return int(lit1.Value.AsInteger()) == val
					} else {
						return int(lit1.Value.AsFloat()) == val
					}
				}
				return type1 == types.Number
			case string:
				// String enum member overlaps with string literals and string type
				if lit1, isLit1 := type1.(*types.LiteralType); isLit1 && lit1.Value.IsString() {
					return lit1.Value.ToString() == val
				}
				return type1 == types.String
			}
		}
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

// applyTypeNarrowingFromCondition recursively walks logical expressions and composes narrowed environments
// This handles compound conditions like "a && b && c" by applying each constraint in sequence
func (c *Checker) applyTypeNarrowingFromCondition(condition parser.Expression) *Environment {
	// Handle logical AND expressions by composing environments
	if infixExpr, ok := condition.(*parser.InfixExpression); ok && infixExpr.Operator == "&&" {
		// For "a && b", first apply narrowing from "a", then apply "b" in that narrowed context

		// Apply narrowing from left side
		leftEnv := c.applyTypeNarrowingFromCondition(infixExpr.Left)
		if leftEnv == nil {
			// If left side doesn't narrow, try just the right side
			return c.applyTypeNarrowingFromCondition(infixExpr.Right)
		}

		// Save current environment and switch to the narrowed one
		originalEnv := c.env
		c.env = leftEnv

		// Apply narrowing from right side in the already-narrowed environment
		rightEnv := c.applyTypeNarrowingFromCondition(infixExpr.Right)

		// Restore original environment
		c.env = originalEnv

		// Return the composed environment (rightEnv already includes leftEnv constraints)
		if rightEnv != nil {
			return rightEnv
		}
		return leftEnv
	}

	// Handle logical OR expressions - for "a || b", we can't really narrow much
	// (would need union of constraints, which is complex)
	if infixExpr, ok := condition.(*parser.InfixExpression); ok && infixExpr.Operator == "||" {
		// For now, don't handle OR expressions in type narrowing
		return nil
	}

	// Handle truthiness checks (bare identifiers)
	if ident, ok := condition.(*parser.Identifier); ok {
		// This is a truthiness check like "if (x)" - eliminates null and undefined
		return c.applyTruthinessNarrowing(ident.Value)
	}

	// Handle single type guard expressions
	guard := c.detectTypeGuard(condition)
	return c.applyTypeNarrowing(guard)
}

// applyTruthinessNarrowing handles bare identifier checks like "if (x)"
// This eliminates null and undefined from union types
func (c *Checker) applyTruthinessNarrowing(varName string) *Environment {
	originalType, isConst, found := c.env.Resolve(varName)
	if !found {
		return nil
	}

	// If it's a union type, remove null and undefined
	if unionType, ok := originalType.(*types.UnionType); ok {
		var truthyMembers []types.Type
		for _, member := range unionType.Types {
			if member != types.Null && member != types.Undefined {
				truthyMembers = append(truthyMembers, member)
			}
		}

		if len(truthyMembers) < len(unionType.Types) {
			// We actually narrowed something
			var narrowedType types.Type
			if len(truthyMembers) == 0 {
				return nil // Would be never, but that's likely an error
			} else if len(truthyMembers) == 1 {
				narrowedType = truthyMembers[0]
			} else {
				narrowedType = types.NewUnionType(truthyMembers...)
			}

			// Create narrowed environment
			narrowedEnv := NewEnclosedEnvironment(c.env)
			if narrowedEnv.Define(varName, narrowedType, isConst) {
				debugPrintf("// [TypeNarrowing] Truthiness check narrowed '%s' from '%s' to '%s'\n",
					varName, originalType.String(), narrowedType.String())
				return narrowedEnv
			}
		}
	}

	return nil
}

// isObjectLikeType checks if a type represents an object-like value (objects, arrays, functions)
// This is used for "typeof x === 'object'" narrowing
func (c *Checker) isObjectLikeType(t types.Type) bool {
	switch t.(type) {
	case *types.ObjectType, *types.ArrayType:
		return true
	default:
		// Primitives like string, number, boolean are not object-like
		return false
	}
}

// isTypeCompatibleForInstanceof checks if a type could be an instance of the target type
// This is used for instanceof narrowing with union types
func (c *Checker) isTypeCompatibleForInstanceof(memberType, targetType types.Type) bool {
	// If they're exactly the same, they're compatible
	if memberType.Equals(targetType) {
		return true
	}

	// Check if memberType is assignable to targetType or vice versa
	if types.IsAssignable(memberType, targetType) || types.IsAssignable(targetType, memberType) {
		return true
	}

	// Primitives like number, string are not compatible with Date instances
	switch memberType {
	case types.Number, types.String, types.Boolean, types.Null, types.Undefined:
		return false
	}

	// Function types (with call signatures) are not compatible with Date instances
	if memberObj, ok := memberType.(*types.ObjectType); ok {
		if targetObj, ok := targetType.(*types.ObjectType); ok {
			// If memberType has call signatures but targetType doesn't, they're incompatible
			// This prevents ContextFn<T> from being considered compatible with Date instances
			if len(memberObj.CallSignatures) > 0 && len(targetObj.CallSignatures) == 0 {
				return false
			}
			// If both are object types without call signatures, they might be compatible
			if len(memberObj.CallSignatures) == 0 && len(targetObj.CallSignatures) == 0 {
				return true
			}
		}
	}

	return false
}

// memberMatchesDiscriminant checks if a union member has a discriminant property
// with a value that matches the expected literal type.
// Used for discriminated union narrowing like: if (expr.kind === "num")
func (c *Checker) memberMatchesDiscriminant(memberType types.Type, propName string, expectedValue types.Type) bool {
	// Get the object type (resolve forward references if needed)
	var objType *types.ObjectType

	switch t := memberType.(type) {
	case *types.ObjectType:
		objType = t
	case *types.TypeAliasForwardReference:
		// Resolve forward reference
		if resolved, found := c.env.ResolveType(t.AliasName); found {
			if obj, ok := resolved.(*types.ObjectType); ok {
				objType = obj
			}
		}
	default:
		return false
	}

	if objType == nil {
		return false
	}

	// Check if the object has the discriminant property
	propType, exists := objType.Properties[propName]
	if !exists {
		return false
	}

	// Check if the property type matches the expected value
	// For literal types, we need to compare the literal values using StrictlyEquals
	if expectedLit, ok := expectedValue.(*types.LiteralType); ok {
		if propLit, ok := propType.(*types.LiteralType); ok {
			// Compare literal values using VM's StrictlyEquals
			return propLit.Value.StrictlyEquals(expectedLit.Value)
		}
	}

	// For general type comparison
	return types.IsAssignable(expectedValue, propType)
}

// typeHasProperty checks if a type has a specific property
// This is used for "prop in obj" narrowing
func (c *Checker) typeHasProperty(t types.Type, propertyName string) bool {
	switch typ := t.(type) {
	case *types.ObjectType:
		// Check if the object type has this property
		_, exists := typ.Properties[propertyName]
		return exists
	case *types.ArrayType:
		// Arrays have "length" property and some built-in methods
		if propertyName == "length" {
			return true
		}
		// For other properties, check if it's a known array method
		// We can use the existing prototype system for this
		if methodType := c.env.GetPrimitivePrototypeMethodType("array", propertyName); methodType != nil {
			return true
		}
		return false
	default:
		// For primitives, check their prototype methods
		var primitiveKey string
		switch t {
		case types.String:
			primitiveKey = "string"
		case types.Number:
			primitiveKey = "number"
		case types.Boolean:
			primitiveKey = "boolean"
		case types.Symbol:
			primitiveKey = "symbol"
		default:
			return false
		}

		if methodType := c.env.GetPrimitivePrototypeMethodType(primitiveKey, propertyName); methodType != nil {
			return true
		}
		return false
	}
}

// blockAlwaysTerminates checks if a block or statement always terminates control flow
// (via return, throw, break, continue). This is used for control flow narrowing.
func blockAlwaysTerminates(node parser.Node) bool {
	if node == nil {
		return false
	}

	switch n := node.(type) {
	case *parser.ReturnStatement:
		return true
	case *parser.ThrowStatement:
		return true
	case *parser.BreakStatement:
		return true
	case *parser.ContinueStatement:
		return true
	case *parser.BlockStatement:
		// A block terminates if its last statement terminates
		if len(n.Statements) == 0 {
			return false
		}
		return blockAlwaysTerminates(n.Statements[len(n.Statements)-1])
	case *parser.IfStatement:
		// If statement terminates only if BOTH branches terminate
		if n.Alternative == nil {
			return false
		}
		return blockAlwaysTerminates(n.Consequence) && blockAlwaysTerminates(n.Alternative)
	default:
		return false
	}
}

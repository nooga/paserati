package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

// calculateEffectiveArgCount calculates the effective number of arguments,
// expanding spread elements based on tuple types (following TypeScript behavior)
func (c *Checker) calculateEffectiveArgCount(arguments []parser.Expression) int {
	count := 0
	for i, arg := range arguments {
		if spreadElement, isSpread := arg.(*parser.SpreadElement); isSpread {
			debugPrintf("// [Checker EffectiveArgCount] Found spread element at index %d\n", i)
			// Visit the spread argument to get its type
			c.visit(spreadElement.Argument)
			argType := spreadElement.Argument.GetComputedType()
			debugPrintf("// [Checker EffectiveArgCount] Spread argument type: %T (%v)\n", argType, argType)
			
			if argType != nil {
				// Check for tuple types first (they have known length)
				if tupleType, isTuple := argType.(*types.TupleType); isTuple {
					elemCount := len(tupleType.ElementTypes)
					debugPrintf("// [Checker EffectiveArgCount] Tuple type with %d elements\n", elemCount)
					count += elemCount
				} else if _, isArray := argType.(*types.ArrayType); isArray {
					// For direct array literals, TypeScript allows them (infers as tuple)
					if arrayLit, isArrayLit := spreadElement.Argument.(*parser.ArrayLiteral); isArrayLit {
						elemCount := len(arrayLit.Elements)
						debugPrintf("// [Checker EffectiveArgCount] Array literal with %d elements (treated as tuple)\n", elemCount)
						count += elemCount
					} else {
						// For array variables/expressions, this should be an error
						// But for counting purposes, we can't determine length
						debugPrintf("// [Checker EffectiveArgCount] Array type (not tuple) - will error later\n")
						count += 1 // Conservative estimate for error checking
					}
				} else {
					// Spread on non-array/tuple type - error
					debugPrintf("// [Checker EffectiveArgCount] Spread on non-array type %T, adding 1\n", argType)
					count += 1
				}
			} else {
				// Error getting type - count as 1 to continue error checking
				debugPrintf("// [Checker EffectiveArgCount] Error getting spread type, adding 1\n")
				count += 1
			}
		} else {
			// Regular argument
			debugPrintf("// [Checker EffectiveArgCount] Regular argument at index %d\n", i)
			count += 1
		}
	}
	debugPrintf("// [Checker EffectiveArgCount] Total effective count: %d\n", count)
	return count
}

// validateSpreadArgument checks if a spread argument is valid according to TypeScript rules
// It now supports contextual typing to infer array literals as tuples when appropriate
func (c *Checker) validateSpreadArgument(spreadElement *parser.SpreadElement, isVariadicFunction bool, expectedParamTypes []types.Type, currentParamIndex int) bool {
	// Try contextual typing for array literals in non-variadic functions
	if !isVariadicFunction {
		if arrayLit, isArrayLit := spreadElement.Argument.(*parser.ArrayLiteral); isArrayLit {
			// Calculate how many parameters remain from current position
			remainingParamCount := len(expectedParamTypes) - currentParamIndex
			if remainingParamCount > 0 && len(arrayLit.Elements) <= remainingParamCount {
				// Create a tuple type from the remaining parameter types
				var tupleElementTypes []types.Type
				for i := 0; i < len(arrayLit.Elements) && currentParamIndex+i < len(expectedParamTypes); i++ {
					tupleElementTypes = append(tupleElementTypes, expectedParamTypes[currentParamIndex+i])
				}
				
				if len(tupleElementTypes) == len(arrayLit.Elements) {
					// Create tuple type for contextual typing
					expectedTupleType := &types.TupleType{
						ElementTypes: tupleElementTypes,
					}
					
					debugPrintf("// [Checker SpreadValidation] Using contextual tuple type: %s for array literal\n", expectedTupleType.String())
					
					// Use contextual typing to check the array literal
					c.visitWithContext(spreadElement.Argument, &ContextualType{
						ExpectedType: expectedTupleType,
						IsContextual: true,
					})
					
					argType := spreadElement.Argument.GetComputedType()
					if argType != nil {
						if _, isTuple := argType.(*types.TupleType); isTuple {
							return true // Successfully inferred as tuple
						}
					}
				}
			}
		}
	}
	
	// Fallback: visit normally and check traditional rules
	c.visit(spreadElement.Argument)
	argType := spreadElement.Argument.GetComputedType()
	
	if argType == nil {
		return false // Error already reported
	}
	
	// Check if it's a tuple type (always valid)
	if _, isTuple := argType.(*types.TupleType); isTuple {
		return true
	}
	
	// Check if it's a direct array literal (TypeScript treats as tuple)
	if _, isArray := argType.(*types.ArrayType); isArray {
		if _, isArrayLit := spreadElement.Argument.(*parser.ArrayLiteral); isArrayLit {
			return true // Direct array literals are treated as tuples
		}
	}
	
	// If it's a variadic function, arrays are allowed (rest parameters)
	if isVariadicFunction {
		if _, isArray := argType.(*types.ArrayType); isArray {
			return true
		}
	}
	
	// Otherwise, this is an error - spread must have tuple type or be passed to rest parameter
	if _, isArray := argType.(*types.ArrayType); isArray {
		c.addError(spreadElement, "A spread argument must either have a tuple type or be passed to a rest parameter")
	} else {
		c.addError(spreadElement, fmt.Sprintf("spread syntax can only be applied to arrays or tuples, got '%s'", argType.String()))
	}
	
	return false
}

// checkFixedArgumentsWithSpread checks fixed parameters against arguments,
// properly expanding spread elements
func (c *Checker) checkFixedArgumentsWithSpread(arguments []parser.Expression, paramTypes []types.Type, isVariadicFunction bool) bool {
	allOk := true
	effectiveArgIndex := 0
	
	for _, argNode := range arguments {
		if spreadElement, isSpread := argNode.(*parser.SpreadElement); isSpread {
			// Validate the spread argument first
			if !c.validateSpreadArgument(spreadElement, isVariadicFunction, paramTypes, effectiveArgIndex) {
				allOk = false
				effectiveArgIndex += 1 // Conservative estimate for error recovery
				continue
			}
			
			// Handle spread element type checking
			argType := spreadElement.Argument.GetComputedType()
			
			if tupleType, isTuple := argType.(*types.TupleType); isTuple {
				// Check each tuple element against the corresponding parameter
				for j, elemType := range tupleType.ElementTypes {
					if effectiveArgIndex+j < len(paramTypes) {
						paramType := paramTypes[effectiveArgIndex+j]
						if !types.IsAssignable(elemType, paramType) {
							c.addError(spreadElement, fmt.Sprintf("spread element %d: cannot assign type '%s' to parameter of type '%s'", j+1, elemType.String(), paramType.String()))
							allOk = false
						}
					}
				}
				effectiveArgIndex += len(tupleType.ElementTypes)
			} else if _, isArray := argType.(*types.ArrayType); isArray {
				// Must be a direct array literal (validated above)
				if arrayLit, isArrayLit := spreadElement.Argument.(*parser.ArrayLiteral); isArrayLit {
					// Check each element against the corresponding parameter
					for j, element := range arrayLit.Elements {
						if effectiveArgIndex+j < len(paramTypes) {
							c.visit(element)
							elemType := element.GetComputedType()
							paramType := paramTypes[effectiveArgIndex+j]
							
							if elemType != nil && !types.IsAssignable(elemType, paramType) {
								c.addError(element, fmt.Sprintf("spread element %d: cannot assign type '%s' to parameter of type '%s'", j+1, elemType.String(), paramType.String()))
								allOk = false
							}
						}
					}
					effectiveArgIndex += len(arrayLit.Elements)
				} else {
					// This should not happen due to validation above
					allOk = false
					effectiveArgIndex += 1
				}
			} else {
				// This should not happen due to validation above
				allOk = false
				effectiveArgIndex += 1
			}
		} else {
			// Regular argument
			c.visit(argNode)
			argType := argNode.GetComputedType()
			
			if effectiveArgIndex < len(paramTypes) {
				paramType := paramTypes[effectiveArgIndex]
				if argType != nil && !c.isAssignableWithExpansion(argType, paramType) {
					c.addError(argNode, fmt.Sprintf("argument %d: cannot assign type '%s' to parameter of type '%s'", effectiveArgIndex+1, argType.String(), paramType.String()))
					allOk = false
				}
			}
			effectiveArgIndex += 1
		}
	}
	
	return allOk
}

func (c *Checker) checkCallExpression(node *parser.CallExpression) {
	// --- UPDATED: Handle CallExpression with Overload Support ---
	// 1. Check the expression being called
	debugPrintf("// [Checker CallExpr] Checking call at line %d\n", node.Token.Line)
	c.visit(node.Function)
	funcNodeType := node.Function.GetComputedType()
	debugPrintf("// [Checker CallExpr] Function type resolved to: %T (%v)\n", funcNodeType, funcNodeType)

	if funcNodeType == nil {
		// Error visiting the function expression itself
		funcIdent, isIdent := node.Function.(*parser.Identifier)
		errMsg := "cannot determine type of called expression"
		if isIdent {
			errMsg = fmt.Sprintf("cannot determine type of called identifier '%s'", funcIdent.Value)
		}
		c.addError(node, errMsg)
		node.SetComputedType(types.Any)
		return
	}

	if funcNodeType == types.Any {
		// Allow calling 'any', result is 'any'. Check args against 'any'.
		for _, argNode := range node.Arguments {
			c.visit(argNode) // Visit args even if function is 'any'
		}
		node.SetComputedType(types.Any)
		return
	}

	// Handle callable ObjectType with unified approach
	objType, ok := funcNodeType.(*types.ObjectType)
	if !ok {
		c.addError(node, fmt.Sprintf("cannot call value of type '%s'", funcNodeType.String()))
		node.SetComputedType(types.Any)
		return
	}
	
	if !objType.IsCallable() {
		c.addError(node, fmt.Sprintf("cannot call value of type '%s'", funcNodeType.String()))
		node.SetComputedType(types.Any)
		return
	}

	if len(objType.CallSignatures) == 0 {
		c.addError(node, fmt.Sprintf("callable object has no call signatures"))
		node.SetComputedType(types.Any)
		return
	}

	// Handle overloaded functions (multiple call signatures)
	if len(objType.CallSignatures) > 1 {
		debugPrintf("// [Checker CallExpr] Processing overloaded function with %d signatures\n", len(objType.CallSignatures))
		c.checkOverloadedCallUnified(node, objType)
		return
	}

	// Handle single call signature
	funcSignature := objType.CallSignatures[0]

	debugPrintf("// [Checker CallExpr] Processing function signature: %s, IsVariadic: %t\n", funcSignature.String(), funcSignature.IsVariadic)

	// Check if this is a generic function call that needs type inference
	if c.isGenericSignature(funcSignature) {
		debugPrintf("// [Checker CallExpr] Detected generic function call, attempting type inference\n")
		inferredSignature := c.inferGenericFunctionCall(node, funcSignature)
		if inferredSignature != nil {
			debugPrintf("// [Checker CallExpr] Type inference successful, using inferred signature: %s\n", inferredSignature.String())
			funcSignature = inferredSignature
		} else {
			debugPrintf("// [Checker CallExpr] Type inference failed, using original signature with type parameters\n")
		}
	}

	// --- MODIFIED Arity and Argument Type Checking ---
	// First validate all spread arguments
	hasSpreadErrors := false
	currentArgIndex := 0
	for _, arg := range node.Arguments {
		if spreadElement, isSpread := arg.(*parser.SpreadElement); isSpread {
			if !c.validateSpreadArgument(spreadElement, funcSignature.IsVariadic, funcSignature.ParameterTypes, currentArgIndex) {
				hasSpreadErrors = true
			}
		}
		// For simplicity, assume each argument takes one slot for this validation pass
		currentArgIndex += 1
	}
	
	// If there are spread errors, skip detailed arity checking as it's meaningless
	if hasSpreadErrors {
		node.SetComputedType(funcSignature.ReturnType)
		return
	}
	
	// Calculate effective argument count, expanding spread elements
	actualArgCount := c.calculateEffectiveArgCount(node.Arguments)
	debugPrintf("// [Checker CallExpr] actualArgCount after spread expansion: %d (from %d raw arguments)\n", actualArgCount, len(node.Arguments))

	if funcSignature.IsVariadic {
		// --- Variadic Function Check ---
		debugPrintf("// [Checker CallExpr] Checking variadic function, RestParameterType: %T (%v)\n", funcSignature.RestParameterType, funcSignature.RestParameterType)
		if funcSignature.RestParameterType == nil {
			c.addError(node, "internal checker error: variadic function type must have a rest parameter type")
			node.SetComputedType(types.Any) // Error state
			return
		}

		// Calculate minimum required arguments (excluding optional parameters)
		minExpectedArgs := len(funcSignature.ParameterTypes)
		if len(funcSignature.OptionalParams) == len(funcSignature.ParameterTypes) {
			// Count required parameters from the end
			for i := len(funcSignature.ParameterTypes) - 1; i >= 0; i-- {
				if funcSignature.OptionalParams[i] {
					minExpectedArgs--
				} else {
					break // Stop at first required parameter from the end
				}
			}
		}

		if actualArgCount < minExpectedArgs {
			c.addError(node, fmt.Sprintf("expected at least %d arguments for variadic function, but got %d", minExpectedArgs, actualArgCount))
			// Don't check args if minimum count isn't met.
		} else {
			// Check fixed arguments with spread expansion support
			fixedArgsOk := c.checkFixedArgumentsWithSpread(node.Arguments, funcSignature.ParameterTypes, funcSignature.IsVariadic)

			// Check variadic arguments
			if fixedArgsOk { // Only check variadic part if fixed part was okay
				variadicParamType := funcSignature.RestParameterType
				arrayType, isArray := variadicParamType.(*types.ArrayType)
				if !isArray {
					c.addError(node, fmt.Sprintf("internal checker error: variadic parameter type must be an array type, got %s", variadicParamType.String()))
				} else {
					variadicElementType := arrayType.ElementType
					if variadicElementType == nil { // Should not happen with valid types
						variadicElementType = types.Any
					}
					// Check remaining arguments against the element type - start after all fixed parameters
					for i := len(funcSignature.ParameterTypes); i < len(node.Arguments); i++ {
						argNode := node.Arguments[i]
						
						// --- Handle spread elements in variadic functions ---
						if spreadElement, isSpread := argNode.(*parser.SpreadElement); isSpread {
							// Validate the spread argument for variadic functions
							// For variadic functions, pass empty slice since we don't use contextual typing
							if !c.validateSpreadArgument(spreadElement, true, []types.Type{}, 0) {
								continue // Error already reported
							}
							
							c.visit(spreadElement.Argument)
							argType := spreadElement.Argument.GetComputedType()
							if argType == nil {
								continue
							}
							
							// For variadic functions, spread arrays should match the rest parameter type
							if !types.IsAssignable(argType, variadicParamType) {
								c.addError(spreadElement, fmt.Sprintf("spread argument: cannot assign type '%s' to rest parameter type '%s'", argType.String(), variadicParamType.String()))
							}
						} else {
							// Regular arguments in variadic part
							c.visit(argNode)
							argType := argNode.GetComputedType()
							if argType == nil {
								continue
							}
							
							if !types.IsAssignable(argType, variadicElementType) {
								c.addError(argNode, fmt.Sprintf("variadic argument %d: cannot assign type '%s' to parameter element type '%s'", i+1, argType.String(), variadicElementType.String()))
							}
						}
					}
				}
			}
		}
	} else {
		// --- Non-Variadic Function Check (Updated for Optional Parameters) ---
		expectedArgCount := len(funcSignature.ParameterTypes)

		// Calculate minimum required arguments (non-optional parameters)
		minRequiredArgs := expectedArgCount
		if len(funcSignature.OptionalParams) == expectedArgCount {
			// Count required parameters from the end
			for i := expectedArgCount - 1; i >= 0; i-- {
				if funcSignature.OptionalParams[i] {
					minRequiredArgs--
				} else {
					break // Stop at first required parameter from the end
				}
			}
		}

		if actualArgCount < minRequiredArgs {
			c.addError(node, fmt.Sprintf("expected at least %d arguments, but got %d", minRequiredArgs, actualArgCount))
			// Continue checking assignable args anyway? Let's stop if arity wrong.
		} else if actualArgCount > expectedArgCount {
			c.addError(node, fmt.Sprintf("expected at most %d arguments, but got %d", expectedArgCount, actualArgCount))
		} else {
			// Check argument types with spread expansion support
			c.checkFixedArgumentsWithSpread(node.Arguments, funcSignature.ParameterTypes, funcSignature.IsVariadic)
		}
	}
	// --- END MODIFIED Checking ---

	// Set Result Type - check if we need to resolve parameterized forward references
	resultType := funcSignature.ReturnType
	
	// If the result type is a ParameterizedForwardReferenceType with concrete type arguments,
	// we need to instantiate the generic type to get a proper object type
	if paramRef, ok := resultType.(*types.ParameterizedForwardReferenceType); ok {
		debugPrintf("// [Checker CallExpr] Result type is parameterized forward reference: %s\n", paramRef.String())
		
		// Try to resolve the generic class and instantiate it
		if resolvedType := c.resolveParameterizedForwardReference(paramRef); resolvedType != nil {
			debugPrintf("// [Checker CallExpr] Resolved parameterized type to: %T (%s)\n", resolvedType, resolvedType.String())
			resultType = resolvedType
		}
	}
	
	debugPrintf("// [Checker CallExpr] Setting result type from func '%s'. ReturnType from Sig: %T (%v)\n", node.Function.String(), resultType, resultType)
	node.SetComputedType(resultType)

}

// checkOverloadedCallUnified handles function calls to overloaded functions using unified ObjectType
func (c *Checker) checkOverloadedCallUnified(node *parser.CallExpression, objType *types.ObjectType) {
	// Visit all arguments first
	var argTypes []types.Type
	for _, argNode := range node.Arguments {
		c.visit(argNode)
		argType := argNode.GetComputedType()
		if argType == nil {
			argType = types.Any
		}
		argTypes = append(argTypes, argType)
	}

	// Try to find the best matching signature
	signatureIndex := -1
	var resultType types.Type

	for i, signature := range objType.CallSignatures {
		// Check if this signature can accept the given arguments
		var isMatching bool

		if signature.IsVariadic {
			// For variadic signatures, check minimum required arguments (fixed parameters)
			minRequiredArgs := len(signature.ParameterTypes)
			if len(argTypes) >= minRequiredArgs {
				// Check fixed parameters first
				fixedMatch := true
				for j := 0; j < minRequiredArgs; j++ {
					if !types.IsAssignable(argTypes[j], signature.ParameterTypes[j]) {
						fixedMatch = false
						break
					}
				}

				if fixedMatch {
					// Check remaining arguments against rest parameter type
					if signature.RestParameterType != nil {
						// Extract element type from rest parameter array type
						var elementType types.Type = types.Any
						if arrayType, ok := signature.RestParameterType.(*types.ArrayType); ok {
							elementType = arrayType.ElementType
						}

						// Check all remaining arguments against element type
						variadicMatch := true
						for j := minRequiredArgs; j < len(argTypes); j++ {
							if !types.IsAssignable(argTypes[j], elementType) {
								variadicMatch = false
								break
							}
						}
						isMatching = variadicMatch
					} else {
						isMatching = true // No rest parameter type specified, assume compatible
					}
				}
			}
		} else {
			// For non-variadic signatures, argument count must match exactly
			if len(argTypes) != len(signature.ParameterTypes) {
				continue // Argument count mismatch
			}

			// Check if all argument types are assignable to parameter types
			allMatch := true
			for j, argType := range argTypes {
				paramType := signature.ParameterTypes[j]
				if !types.IsAssignable(argType, paramType) {
					allMatch = false
					break
				}
			}
			isMatching = allMatch
		}

		if isMatching {
			signatureIndex = i
			resultType = signature.ReturnType
			break // Found the first matching signature
		}
	}

	if signatureIndex == -1 {
		// No matching signature found
		var signatureStrs []string
		for _, signature := range objType.CallSignatures {
			signatureStrs = append(signatureStrs, signature.String())
		}

		// Build argument type string for error message
		var argTypeStrs []string
		for _, argType := range argTypes {
			argTypeStrs = append(argTypeStrs, argType.String())
		}

		// Format signatures nicely - each on its own line with proper indentation
		signatureList := ""
		for i, sig := range signatureStrs {
			if i > 0 {
				signatureList += "\n"
			}
			signatureList += "  " + sig
		}

		c.addError(node, fmt.Sprintf("no overload matches call with arguments (%v). Available signatures:\n%s",
			argTypeStrs, signatureList))

		node.SetComputedType(types.Any)
		return
	}

	// Found a matching signature
	matchedSignature := objType.CallSignatures[signatureIndex]
	debugPrintf("// [Checker OverloadCall] Found matching signature %d: %s for call with args (%v)\n",
		signatureIndex, matchedSignature.String(), argTypes)

	// Set the result type from the matched signature
	node.SetComputedType(resultType)
	debugPrintf("// [Checker OverloadCall] Set result type to: %s\n", resultType.String())
}

// isGenericSignature checks if a function signature contains unresolved type parameters
func (c *Checker) isGenericSignature(sig *types.Signature) bool {
	// Helper function to check if a type contains type parameters
	var containsTypeParameters func(t types.Type) bool
	containsTypeParameters = func(t types.Type) bool {
		switch typ := t.(type) {
		case *types.TypeParameterType:
			return true
		case *types.ArrayType:
			return containsTypeParameters(typ.ElementType)
		case *types.UnionType:
			for _, memberType := range typ.Types {
				if containsTypeParameters(memberType) {
					return true
				}
			}
			return false
		case *types.ObjectType:
			for _, propType := range typ.Properties {
				if containsTypeParameters(propType) {
					return true
				}
			}
			return false
		// Add more type cases as needed
		default:
			return false
		}
	}
	
	// Check parameter types
	for _, paramType := range sig.ParameterTypes {
		if containsTypeParameters(paramType) {
			return true
		}
	}
	
	// Check return type
	if sig.ReturnType != nil && containsTypeParameters(sig.ReturnType) {
		return true
	}
	
	// Check rest parameter type
	if sig.RestParameterType != nil && containsTypeParameters(sig.RestParameterType) {
		return true
	}
	
	return false
}

// inferGenericFunctionCall attempts to infer type arguments for a generic function call
func (c *Checker) inferGenericFunctionCall(callNode *parser.CallExpression, genericSig *types.Signature) *types.Signature {
	debugPrintf("// [Checker Inference] Starting type inference for generic function call\n")
	
	// First, visit all arguments to get their types
	var argTypes []types.Type
	for i, argNode := range callNode.Arguments {
		c.visit(argNode)
		argType := argNode.GetComputedType()
		if argType == nil {
			argType = types.Any
		}
		argTypes = append(argTypes, argType)
		debugPrintf("// [Checker Inference] Argument %d type: %s\n", i, argType.String())
	}
	
	// Collect type parameter constraints
	constraints := c.collectTypeParameterConstraints(genericSig, argTypes)
	debugPrintf("// [Checker Inference] Collected %d type parameter constraints\n", len(constraints))
	
	// Solve constraints to get type parameter bindings
	solution := c.solveTypeParameterConstraints(constraints)
	debugPrintf("// [Checker Inference] Solved %d type parameter bindings\n", len(solution))
	
	if len(solution) == 0 {
		debugPrintf("// [Checker Inference] No type parameters could be inferred\n")
		return nil // Inference failed
	}
	
	// Substitute type parameters with inferred types
	inferredSig := c.substituteTypeParameters(genericSig, solution)
	debugPrintf("// [Checker Inference] Created inferred signature: %s\n", inferredSig.String())
	
	return inferredSig
}

// TypeParameterConstraint represents a constraint on a type parameter
type TypeParameterConstraint struct {
	TypeParameter *types.TypeParameter
	InferredType  types.Type
	Confidence    int // Higher = more confident
}

// collectTypeParameterConstraints analyzes arguments to build constraints for type parameters
func (c *Checker) collectTypeParameterConstraints(sig *types.Signature, argTypes []types.Type) []TypeParameterConstraint {
	var constraints []TypeParameterConstraint
	
	// For each parameter, if it contains type parameters, create constraints based on the argument type
	for i, paramType := range sig.ParameterTypes {
		if i >= len(argTypes) {
			break // No more arguments
		}
		argType := argTypes[i]
		
		// Collect constraints from this parameter-argument pair
		paramConstraints := c.collectConstraintsFromType(paramType, argType)
		constraints = append(constraints, paramConstraints...)
	}
	
	return constraints
}

// collectConstraintsFromType recursively collects constraints by matching parameter and argument types
func (c *Checker) collectConstraintsFromType(paramType, argType types.Type) []TypeParameterConstraint {
	var constraints []TypeParameterConstraint
	
	switch pType := paramType.(type) {
	case *types.TypeParameterType:
		// Direct constraint: T should be inferred as argType
		constraints = append(constraints, TypeParameterConstraint{
			TypeParameter: pType.Parameter,
			InferredType:  argType,
			Confidence:    100, // High confidence for direct matches
		})
		debugPrintf("// [Checker Constraints] Direct constraint: %s = %s\n", pType.Parameter.Name, argType.String())
		
	case *types.ArrayType:
		// Array<T> matched against Array<U> or U[]
		if aType, isArray := argType.(*types.ArrayType); isArray {
			// Recurse into element types
			elemConstraints := c.collectConstraintsFromType(pType.ElementType, aType.ElementType)
			constraints = append(constraints, elemConstraints...)
		}
		
	// Add more cases for other generic type constructs as needed
	}
	
	return constraints
}

// solveTypeParameterConstraints attempts to solve the collected constraints
func (c *Checker) solveTypeParameterConstraints(constraints []TypeParameterConstraint) map[*types.TypeParameter]types.Type {
	solution := make(map[*types.TypeParameter]types.Type)
	
	// Simple solver: for each type parameter, pick the constraint with highest confidence
	// In the future, this could be much more sophisticated (unification, etc.)
	
	type bestConstraint struct {
		constraint TypeParameterConstraint
		confidence int
	}
	
	best := make(map[*types.TypeParameter]bestConstraint)
	
	for _, constraint := range constraints {
		existing, exists := best[constraint.TypeParameter]
		if !exists || constraint.Confidence > existing.confidence {
			best[constraint.TypeParameter] = bestConstraint{
				constraint: constraint,
				confidence: constraint.Confidence,
			}
		}
	}
	
	// Convert best constraints to solution
	for typeParam, bestConstr := range best {
		solution[typeParam] = bestConstr.constraint.InferredType
		debugPrintf("// [Checker Solve] %s = %s (confidence: %d)\n", 
			typeParam.Name, bestConstr.constraint.InferredType.String(), bestConstr.confidence)
	}
	
	return solution
}

// substituteTypeParameters creates a new signature with type parameters replaced by inferred types
func (c *Checker) substituteTypeParameters(sig *types.Signature, solution map[*types.TypeParameter]types.Type) *types.Signature {
	// Helper function to substitute type parameters in a type
	var substitute func(t types.Type) types.Type
	substitute = func(t types.Type) types.Type {
		switch typ := t.(type) {
		case *types.TypeParameterType:
			if inferredType, found := solution[typ.Parameter]; found {
				return inferredType
			}
			return typ // Keep unresolved type parameters
		case *types.ArrayType:
			return &types.ArrayType{ElementType: substitute(typ.ElementType)}
		case *types.UnionType:
			var newTypes []types.Type
			for _, memberType := range typ.Types {
				newTypes = append(newTypes, substitute(memberType))
			}
			return types.NewUnionType(newTypes...)
		case *types.ParameterizedForwardReferenceType:
			// Substitute type parameters in the type arguments
			var newTypeArgs []types.Type
			for _, typeArg := range typ.TypeArguments {
				newTypeArgs = append(newTypeArgs, substitute(typeArg))
			}
			// Create a new parameterized forward reference with substituted type arguments
			return &types.ParameterizedForwardReferenceType{
				ClassName:      typ.ClassName,
				TypeArguments:  newTypeArgs,
			}
		case *types.ObjectType:
			// For ObjectType, we need to substitute in properties and signatures
			newObj := &types.ObjectType{
				Properties:     make(map[string]types.Type),
				OptionalProperties: typ.OptionalProperties, // Copy as-is
				CallSignatures: nil,
				ConstructSignatures: nil,
				BaseTypes: typ.BaseTypes, // Copy as-is for now
				IndexSignatures: typ.IndexSignatures, // Copy as-is for now
			}
			
			// Substitute in properties
			for name, propType := range typ.Properties {
				newObj.Properties[name] = substitute(propType)
			}
			
			// Substitute in call signatures
			for _, sig := range typ.CallSignatures {
				newSig := c.substituteInSignature(sig, solution, substitute)
				newObj.CallSignatures = append(newObj.CallSignatures, newSig)
			}
			
			// Substitute in construct signatures
			for _, sig := range typ.ConstructSignatures {
				newSig := c.substituteInSignature(sig, solution, substitute)
				newObj.ConstructSignatures = append(newObj.ConstructSignatures, newSig)
			}
			
			return newObj
		// Add more cases as needed
		default:
			return typ
		}
	}
	
	// Substitute in parameter types
	var newParamTypes []types.Type
	for _, paramType := range sig.ParameterTypes {
		newParamTypes = append(newParamTypes, substitute(paramType))
	}
	
	// Substitute in return type
	var newReturnType types.Type
	if sig.ReturnType != nil {
		newReturnType = substitute(sig.ReturnType)
	}
	
	// Substitute in rest parameter type
	var newRestParamType types.Type
	if sig.RestParameterType != nil {
		newRestParamType = substitute(sig.RestParameterType)
	}
	
	return &types.Signature{
		ParameterTypes:    newParamTypes,
		ReturnType:        newReturnType,
		OptionalParams:    sig.OptionalParams, // Copy as-is
		IsVariadic:        sig.IsVariadic,
		RestParameterType: newRestParamType,
	}
}

// substituteInSignature is a helper to substitute type parameters in a signature
func (c *Checker) substituteInSignature(sig *types.Signature, solution map[*types.TypeParameter]types.Type, substitute func(types.Type) types.Type) *types.Signature {
	// Substitute in parameter types
	var newParamTypes []types.Type
	for _, paramType := range sig.ParameterTypes {
		newParamTypes = append(newParamTypes, substitute(paramType))
	}
	
	// Substitute in return type
	var newReturnType types.Type
	if sig.ReturnType != nil {
		newReturnType = substitute(sig.ReturnType)
	}
	
	// Substitute in rest parameter type
	var newRestParamType types.Type
	if sig.RestParameterType != nil {
		newRestParamType = substitute(sig.RestParameterType)
	}
	
	return &types.Signature{
		ParameterTypes:    newParamTypes,
		ReturnType:        newReturnType,
		OptionalParams:    sig.OptionalParams, // Copy as-is
		IsVariadic:        sig.IsVariadic,
		RestParameterType: newRestParamType,
	}
}

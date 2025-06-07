package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

func (c *Checker) checkArrowFunctionLiteral(node *parser.ArrowFunctionLiteral) {
	// --- UPDATED: Handle ArrowFunctionLiteral (Similar to FunctionLiteral) ---
	// 1. Save outer context
	outerExpectedReturnType := c.currentExpectedReturnType
	outerInferredReturnTypes := c.currentInferredReturnTypes

	// 2. Resolve Parameter Types
	paramTypes := []types.Type{}
	paramNames := []*parser.Identifier{}
	for _, param := range node.Parameters {
		var paramType types.Type = types.Any // Default, USE INTERFACE TYPE
		if param.TypeAnnotation != nil {
			resolvedParamType := c.resolveTypeAnnotation(param.TypeAnnotation)
			if resolvedParamType != nil {
				paramType = resolvedParamType // Assign interface{} to interface{}
			}
		}
		paramTypes = append(paramTypes, paramType)
		paramNames = append(paramNames, param.Name)
		param.ComputedType = paramType
	}

	// Handle rest parameter if present
	var restParameterType types.Type
	var restParameterName *parser.Identifier
	if node.RestParameter != nil {
		var resolvedRestType types.Type
		if node.RestParameter.TypeAnnotation != nil {
			resolvedRestType = c.resolveTypeAnnotation(node.RestParameter.TypeAnnotation)

			// Rest parameter type should be an array type
			if resolvedRestType != nil {
				if _, isArrayType := resolvedRestType.(*types.ArrayType); !isArrayType {
					c.addError(node.RestParameter.TypeAnnotation, fmt.Sprintf("rest parameter type must be an array type, got '%s'", resolvedRestType.String()))
					resolvedRestType = &types.ArrayType{ElementType: types.Any}
				}
			}
		}

		if resolvedRestType == nil {
			// Default to any[] if no annotation
			resolvedRestType = &types.ArrayType{ElementType: types.Any}
		}

		restParameterType = resolvedRestType
		restParameterName = node.RestParameter.Name
		node.RestParameter.ComputedType = restParameterType
	}

	// --- NEW: Resolve Return Type Annotation ---
	expectedReturnType := c.resolveTypeAnnotation(node.ReturnTypeAnnotation)

	// --- UPDATED: Set Context using expected type ---
	c.currentExpectedReturnType = expectedReturnType // Use resolved annotation (can be nil)
	c.currentInferredReturnTypes = nil               // Reset for this function
	if expectedReturnType == nil {
		// Only allocate if we need to infer
		c.currentInferredReturnTypes = []types.Type{}
	}

	// 4. Create function scope & define parameters
	// --- DEBUG ---
	debugPrintf("// [Checker Visit ArrowFunc] Creating Func Scope. Current Env: %p\n", c.env)
	if c.env == nil {
		panic("Checker env is nil before creating arrow scope!")
	}
	// --- END DEBUG ---
	originalEnv := c.env
	funcEnv := NewEnclosedEnvironment(originalEnv)
	c.env = funcEnv
	for i, nameNode := range paramNames {
		if !funcEnv.Define(nameNode.Value, paramTypes[i], false) {
			c.addError(nameNode, fmt.Sprintf("duplicate parameter name: %s", nameNode.Value))
		}
	}

	// Define rest parameter if present
	if restParameterName != nil && restParameterType != nil {
		if !funcEnv.Define(restParameterName.Value, restParameterType, false) {
			c.addError(restParameterName, fmt.Sprintf("duplicate parameter name: %s", restParameterName.Value))
		}
	}

	// 5. Visit Body
	c.visit(node.Body)
	var bodyType types.Type = types.Any
	isExprBody := false
	// Special handling for expression body
	if exprBody, ok := node.Body.(parser.Expression); ok {
		isExprBody = true
		bodyType = exprBody.GetComputedType()
		// If body is an expression, its type *is* the single inferred return type,
		// unless overridden by an annotation.
		if c.currentInferredReturnTypes != nil { // Only append if inference is active
			c.currentInferredReturnTypes = append(c.currentInferredReturnTypes, bodyType)
		}

		// --- NEW: Check expression body type against annotation ---
		if expectedReturnType != nil {
			if !types.IsAssignable(bodyType, expectedReturnType) {
				// TODO: Get line number from exprBody token if possible
				c.addError(exprBody, fmt.Sprintf("cannot return expression of type '%s' from arrow function with return type annotation '%s'", bodyType.String(), expectedReturnType.String()))
			}
		}
		// --- END NEW ---
	} // Else: Body is BlockStatement, returns handled by ReturnStatement visitor

	// 6. Determine Final Return Type (Inference or Annotation)
	var finalReturnType types.Type = expectedReturnType // Start with annotation
	if finalReturnType == nil {                         // Infer ONLY if no annotation
		if len(c.currentInferredReturnTypes) == 0 {
			// If it was an expression body, we should have added its type already.
			// If it's a block body with no returns, it's Undefined.
			if isExprBody {
				// Should have been added above, but double check
				finalReturnType = bodyType
			} else {
				finalReturnType = types.Undefined // No returns in block, infer Undefined
			}
		} else {
			// Inference logic for multiple returns (existing logic seems okay)
			if len(c.currentInferredReturnTypes) == 1 {
				finalReturnType = c.currentInferredReturnTypes[0]
			} else {
				firstType := c.currentInferredReturnTypes[0]
				allSame := true
				for _, typ := range c.currentInferredReturnTypes[1:] {
					// TODO: Use proper type equality check later
					if typ != firstType { // Basic check
						allSame = false
						break
					}
				}
				if allSame {
					finalReturnType = firstType
				} else {
					// TODO: Implement Union type, fallback to Any for now
					finalReturnType = types.Any
				}
			}
		}
	} // else: Annotation exists. ReturnStatement visitor handles checks for block bodies. Expression body check done above.

	if finalReturnType == nil { // Safety check
		finalReturnType = types.Any
	}

	// 7. Create FunctionType
	optionalParams := make([]bool, len(node.Parameters))
	for i, param := range node.Parameters {
		optionalParams[i] = param.Optional || (param.DefaultValue != nil)
	}

	funcType := &types.FunctionType{
		ParameterTypes:    paramTypes,
		ReturnType:        finalReturnType,
		OptionalParams:    optionalParams,
		IsVariadic:        node.RestParameter != nil,
		RestParameterType: restParameterType,
	}

	// --- DEBUG: Log type before setting ---
	debugPrintf("// [Checker ArrowFunc] Computed funcType: %s\n", funcType.String())
	// --- END DEBUG ---

	// 8. Set ComputedType on the ArrowFunctionLiteral node
	// ... calculate funcType ...
	debugPrintf("// [Checker ArrowFunc] ABOUT TO SET Computed funcType: %#v, ReturnType: %#v\n", funcType, funcType.ReturnType)
	node.SetComputedType(funcType)

	// 9. Restore outer environment and context
	// --- DEBUG ---
	debugPrintf("// [Checker Visit ArrowFunc] Exiting Arrow Func. Restoring Env: %p (from current %p)\n", originalEnv, c.env)
	if originalEnv == nil {
		panic("Checker originalEnv is nil before restoring arrow scope!")
	}
	// --- END DEBUG ---
	c.env = originalEnv
	c.currentExpectedReturnType = outerExpectedReturnType
	c.currentInferredReturnTypes = outerInferredReturnTypes

}

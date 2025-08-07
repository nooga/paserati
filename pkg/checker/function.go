package checker

import (
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

func (c *Checker) checkFunctionLiteral(node *parser.FunctionLiteral) {
	// Use the unified function checking approach
	funcName := "<anonymous>"
	if node.Name != nil {
		funcName = node.Name.Value
	}
	
	// Create function check context
	ctx := &FunctionCheckContext{
		FunctionName:              funcName,
		TypeParameters:            node.TypeParameters, // Support for generic functions
		Parameters:                node.Parameters,
		RestParameter:             node.RestParameter,
		ReturnTypeAnnotation:      node.ReturnTypeAnnotation,
		Body:                      node.Body,
		IsArrow:                   false,
		IsGenerator:               node.IsGenerator, // Detect generator functions
		AllowSelfReference:        node.Name != nil, // Allow recursion for named functions
		AllowOverloadCompletion:   node.Name != nil, // Check for overloads for named functions
	}

	// 1. Resolve parameters and signature
	preliminarySignature, paramTypes, paramNames, restParameterType, restParameterName, typeParamEnv := c.resolveFunctionParameters(ctx)

	// 2. Setup function environment
	originalEnv := c.setupFunctionEnvironment(ctx, paramTypes, paramNames, restParameterType, restParameterName, preliminarySignature, typeParamEnv)

	// 3. Check function body and determine return type
	// For generator functions, we need to check the body against the inner return type,
	// not the full Generator<T, TReturn, TNext> type
	expectedReturnTypeForBody := preliminarySignature.ReturnType
	var innerReturnTypeFromAnnotation types.Type
	
	if node.IsGenerator {
		// For generators, we want to check the body against the inner return type
		if preliminarySignature.ReturnType != nil {
			// If there's an explicit return type annotation, extract TReturn from Generator<T, TReturn, TNext>
			if instType, ok := preliminarySignature.ReturnType.(*types.InstantiatedType); ok {
				if instType.Generic.Name == "Generator" && len(instType.TypeArguments) >= 2 {
					innerReturnTypeFromAnnotation = instType.TypeArguments[1] // TReturn parameter
					expectedReturnTypeForBody = innerReturnTypeFromAnnotation
					debugPrintf("// [Checker FuncLit] Generator function: using TReturn type %s for body checking\n", 
						expectedReturnTypeForBody.String())
				}
			}
		} else {
			// No explicit return type annotation - use nil to allow inference
			expectedReturnTypeForBody = nil
			debugPrintf("// [Checker FuncLit] Generator function: no explicit return type, allowing inference\n")
		}
	}
	
	finalReturnType := c.checkFunctionBody(ctx, expectedReturnTypeForBody)

	// 4. Handle generator functions - wrap return type in Generator<T, TReturn, TNext>
	if node.IsGenerator {
		// For generator functions, the actual return type becomes Generator<YieldType, ReturnType, any>
		debugPrintf("// [Checker FuncLit] Generator function detected, wrapping return type\n")
		
		// Use the collected yield types to create the proper Generator<T, TReturn, TNext> type
		generatorType := c.createGeneratorType(finalReturnType, c.currentInferredYieldTypes)
		finalReturnType = generatorType
		debugPrintf("// [Checker FuncLit] Created generator type: %s\n", finalReturnType.String())
	}

	// 5. Create final function type
	finalFuncType := c.createFinalFunctionType(ctx, paramTypes, finalReturnType, restParameterType)

	// 6. Set computed type on the FunctionLiteral node
	debugPrintf("// [Checker FuncLit] Setting computed type: %s\n", finalFuncType.String())
	node.SetComputedType(finalFuncType)

	// 6. Check for overload completion if this is a named function
	if ctx.AllowOverloadCompletion && len(c.env.GetPendingOverloads(funcName)) > 0 {
		c.completeOverloadedFunction(funcName, finalFuncType)
	}

	// 7. Restore environment
	c.env = originalEnv
	debugPrintf("// [Checker FuncLit] Restored environment to: %p\n", originalEnv)
}

// createGeneratorType creates an instantiated Generator<T, TReturn, TNext> type
// based on the yielded and returned types from the generator function
func (c *Checker) createGeneratorType(returnType types.Type, yieldTypes []types.Type) types.Type {
	// Handle nil return type (generators without explicit return default to void)  
	if returnType == nil {
		returnType = types.Void
	}
	
	// Determine the yield type (T) from collected yield expressions
	var yieldType types.Type
	if len(yieldTypes) == 0 {
		// No yield expressions found, use undefined
		yieldType = types.Undefined
	} else if len(yieldTypes) == 1 {
		// Single yield type
		yieldType = yieldTypes[0]
	} else {
		// Multiple yield types, create a union
		yieldType = types.NewUnionType(yieldTypes...)
	}
	
	// Create an instantiated Generator<T, TReturn, TNext> type
	if types.GeneratorGeneric != nil {
		return types.NewInstantiatedType(types.GeneratorGeneric, []types.Type{
			yieldType,        // T (yield type) - inferred from yield expressions
			returnType,       // TReturn (return type) 
			types.Unknown,    // TNext (sent type) - TypeScript uses unknown for this
		})
	}
	
	// Fallback if GeneratorGeneric is not initialized
	generatorObj := types.NewObjectType()
	generatorObj.WithProperty("next", types.NewOptionalFunction([]types.Type{types.Unknown}, types.Any, []bool{true}))
	generatorObj.WithProperty("return", types.NewOptionalFunction([]types.Type{returnType}, types.Any, []bool{true}))
	generatorObj.WithProperty("throw", types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{true}))
	return generatorObj
}

func (c *Checker) checkMethodDefinition(node *parser.MethodDefinition) {
	// Method definitions are essentially function literals with different kinds
	// Visit the underlying function literal first
	if node.Value != nil {
		c.visit(node.Value)
		// Copy the computed type from the function literal to the method definition
		if funcType := node.Value.GetComputedType(); funcType != nil {
			node.SetComputedType(funcType)
		} else {
			node.SetComputedType(types.Any)
		}
	} else {
		// No function value (shouldn't happen in practice)
		node.SetComputedType(types.Any)
	}
}

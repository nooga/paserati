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
		IsAsync:                   node.IsAsync,     // Detect async functions
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

	// 4.5. Handle async functions - wrap return type in Promise<T>
	if node.IsAsync {
		// For async functions, the actual return type becomes Promise<T>
		debugPrintf("// [Checker FuncLit] Async function detected, wrapping return type in Promise\n")

		// Handle nil return type (async functions without explicit return default to Promise<void>)
		innerType := finalReturnType
		if innerType == nil {
			innerType = types.Void
		}

		// Create Promise<T> type
		promiseType := c.createPromiseType(innerType)
		finalReturnType = promiseType
		debugPrintf("// [Checker FuncLit] Created Promise type: %s\n", finalReturnType.String())
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

// createPromiseType creates an instantiated Promise<T> type
func (c *Checker) createPromiseType(valueType types.Type) types.Type {
	// Handle nil value type (shouldn't happen, but default to void)
	if valueType == nil {
		valueType = types.Void
	}

	// Use the global PromiseGeneric type (set by builtins) - same pattern as generators
	if types.PromiseGeneric != nil {
		return types.NewInstantiatedType(types.PromiseGeneric, []types.Type{valueType})
	}

	// Fallback if PromiseGeneric is not initialized
	// Create a simple object type representing Promise<T>
	promiseObj := types.NewObjectType()

	// Add then method: then<TResult1 = T, TResult2 = never>(
	//   onfulfilled?: (value: T) => TResult1 | PromiseLike<TResult1>,
	//   onrejected?: (reason: any) => TResult2 | PromiseLike<TResult2>
	// ): Promise<TResult1 | TResult2>
	promiseObj.WithProperty("then", types.NewOptionalFunction(
		[]types.Type{
			types.NewOptionalFunction([]types.Type{valueType}, types.Any, []bool{false}),
			types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{false}),
		},
		promiseObj, // Returns another Promise
		[]bool{true, true}, // Both parameters optional
	))

	// Add catch method: catch<TResult = never>(onrejected?: (reason: any) => TResult): Promise<T | TResult>
	promiseObj.WithProperty("catch", types.NewOptionalFunction(
		[]types.Type{types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{false})},
		promiseObj,
		[]bool{true},
	))

	// Add finally method: finally(onfinally?: () => void): Promise<T>
	promiseObj.WithProperty("finally", types.NewOptionalFunction(
		[]types.Type{types.NewOptionalFunction([]types.Type{}, types.Void, []bool{})},
		promiseObj,
		[]bool{true},
	))

	return promiseObj
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

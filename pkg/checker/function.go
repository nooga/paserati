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
	finalReturnType := c.checkFunctionBody(ctx, preliminarySignature.ReturnType)

	// 4. Handle generator functions - wrap return type in Generator<T, TReturn, TNext>
	if node.IsGenerator {
		// For generator functions, the actual return type becomes Generator<YieldType, ReturnType, any>
		// For now, we'll use a simplified approach and set it to Any
		// TODO: Implement proper Generator<T, TReturn, TNext> type construction
		debugPrintf("// [Checker FuncLit] Generator function detected, wrapping return type\n")
		
		// For basic functionality, we'll create a generic Generator type
		// In a full implementation, this would be Generator<YieldType, ReturnType, TNext>
		// For now, we'll use a placeholder that has the .next() method
		generatorType := c.createGeneratorType(finalReturnType)
		finalReturnType = generatorType
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
func (c *Checker) createGeneratorType(returnType types.Type) types.Type {
	// For now, we'll use a simplified approach:
	// - T (yield type): types.Any for now (would need to analyze yield expressions)
	// - TReturn (return type): the actual return type from function body
	// - TNext (sent type): types.Any (values sent via .next())
	
	// Temporarily use ObjectType instead of InstantiatedType to avoid GeneratorGeneric panic
	// TODO: Fix GeneratorGeneric initialization issue
	generatorObj := types.NewObjectType()
	generatorObj.WithProperty("next", types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{true}))
	generatorObj.WithProperty("return", types.NewOptionalFunction([]types.Type{returnType}, types.Any, []bool{true}))
	generatorObj.WithProperty("throw", types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{true}))
	return generatorObj
}

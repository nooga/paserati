package checker

import (
	"paserati/pkg/parser"
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
		AllowSelfReference:        node.Name != nil, // Allow recursion for named functions
		AllowOverloadCompletion:   node.Name != nil, // Check for overloads for named functions
	}

	// 1. Resolve parameters and signature
	preliminarySignature, paramTypes, paramNames, restParameterType, restParameterName, typeParamEnv := c.resolveFunctionParameters(ctx)

	// 2. Setup function environment
	originalEnv := c.setupFunctionEnvironment(ctx, paramTypes, paramNames, restParameterType, restParameterName, preliminarySignature, typeParamEnv)

	// 3. Check function body and determine return type
	finalReturnType := c.checkFunctionBody(ctx, preliminarySignature.ReturnType)

	// 4. Create final function type
	finalFuncType := c.createFinalFunctionType(ctx, paramTypes, finalReturnType, restParameterType)

	// 5. Set computed type on the FunctionLiteral node
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

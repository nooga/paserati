package checker

import (
	"paserati/pkg/parser"
)

func (c *Checker) checkArrowFunctionLiteral(node *parser.ArrowFunctionLiteral) {
	// Create function check context
	ctx := &FunctionCheckContext{
		FunctionName:              "<arrow>", // Arrow functions are anonymous
		Parameters:                node.Parameters,
		RestParameter:             node.RestParameter,
		ReturnTypeAnnotation:      node.ReturnTypeAnnotation,
		Body:                      node.Body,
		IsArrow:                   true,
		AllowSelfReference:        false, // Arrow functions don't have self-reference
		AllowOverloadCompletion:   false, // Arrow functions don't support overloads
	}

	// 1. Resolve parameters and signature
	preliminarySignature, paramTypes, paramNames, restParameterType, restParameterName := c.resolveFunctionParameters(ctx)

	// 2. Setup function environment
	originalEnv := c.setupFunctionEnvironment(ctx, paramTypes, paramNames, restParameterType, restParameterName, preliminarySignature)

	// 3. Check function body and determine return type
	finalReturnType := c.checkFunctionBody(ctx, preliminarySignature.ReturnType)

	// 4. Create final function type
	finalFuncType := c.createFinalFunctionType(ctx, paramTypes, finalReturnType, restParameterType)

	// 5. Set computed type on the ArrowFunctionLiteral node
	debugPrintf("// [Checker ArrowFunc] Setting computed type: %s\n", finalFuncType.String())
	node.SetComputedType(finalFuncType)

	// 6. Restore environment
	c.env = originalEnv
	debugPrintf("// [Checker ArrowFunc] Restored environment to: %p\n", originalEnv)
}

package checker

import (
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

func (c *Checker) checkArrowFunctionLiteral(node *parser.ArrowFunctionLiteral) {
	// Create function check context
	ctx := &FunctionCheckContext{
		FunctionName:              "<arrow>", // Arrow functions are anonymous
		TypeParameters:            node.TypeParameters, // Support for generic arrow functions
		Parameters:                node.Parameters,
		RestParameter:             node.RestParameter,
		ReturnTypeAnnotation:      node.ReturnTypeAnnotation,
		Body:                      node.Body,
		IsArrow:                   true,
		IsGenerator:               false,       // Arrow functions cannot be generators
		IsAsync:                   node.IsAsync, // Support async arrow functions
		AllowSelfReference:        false,       // Arrow functions don't have self-reference
		AllowOverloadCompletion:   false,       // Arrow functions don't support overloads
	}

	// 1. Resolve parameters and signature
	preliminarySignature, paramTypes, paramNames, restParameterType, restParameterName, typeParamEnv := c.resolveFunctionParameters(ctx)

	// 2. Setup function environment
	originalEnv := c.setupFunctionEnvironment(ctx, paramTypes, paramNames, restParameterType, restParameterName, preliminarySignature, typeParamEnv)

	// 3. Check function body and determine return type
	finalReturnType := c.checkFunctionBody(ctx, preliminarySignature.ReturnType)

	// 4. Handle async arrow functions - wrap return type in Promise<T>
	if node.IsAsync {
		debugPrintf("// [Checker ArrowFunc] Async arrow function detected, wrapping return type in Promise\n")

		// Handle nil return type (async functions without explicit return default to Promise<void>)
		innerType := finalReturnType
		if innerType == nil {
			innerType = types.Void
		}

		// Create Promise<T> type
		promiseType := c.createPromiseType(innerType)
		finalReturnType = promiseType
		debugPrintf("// [Checker ArrowFunc] Created Promise type: %s\n", finalReturnType.String())
	}

	// 5. Create final function type
	finalFuncType := c.createFinalFunctionType(ctx, paramTypes, finalReturnType, restParameterType)

	// 6. Set computed type on the ArrowFunctionLiteral node
	debugPrintf("// [Checker ArrowFunc] Setting computed type: %s\n", finalFuncType.String())
	node.SetComputedType(finalFuncType)

	// 7. Restore environment
	c.env = originalEnv
	debugPrintf("// [Checker ArrowFunc] Restored environment to: %p\n", originalEnv)
}

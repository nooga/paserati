package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

// FunctionCheckContext holds the common context for function checking
type FunctionCheckContext struct {
	FunctionName              string              // For logging and recursion
	Parameters                []*parser.Parameter // Parameter nodes
	RestParameter             *parser.RestParameter // Rest parameter node (if any)
	ReturnTypeAnnotation      parser.Expression   // Return type annotation (if any)
	Body                      parser.Node         // Function body (block or expression)
	IsArrow                   bool                // Whether this is an arrow function
	AllowSelfReference        bool                // Whether to allow recursive self-reference
	AllowOverloadCompletion   bool                // Whether to check for overload completion
}

// resolveFunctionParameters resolves parameter types and creates the parameter environment setup
func (c *Checker) resolveFunctionParameters(ctx *FunctionCheckContext) (*types.Signature, []types.Type, []*parser.Identifier, types.Type, *parser.Identifier) {
	var paramTypes []types.Type
	var paramNames []*parser.Identifier
	var restParameterType types.Type
	var restParameterName *parser.Identifier
	
	// Resolve regular parameters
	for _, param := range ctx.Parameters {
		var paramType types.Type = types.Any
		if param.TypeAnnotation != nil {
			resolvedParamType := c.resolveTypeAnnotation(param.TypeAnnotation)
			if resolvedParamType != nil {
				paramType = resolvedParamType
			}
		}
		paramTypes = append(paramTypes, paramType)
		paramNames = append(paramNames, param.Name)
		param.ComputedType = paramType
	}
	
	// Handle rest parameter if present
	if ctx.RestParameter != nil {
		var resolvedRestType types.Type
		if ctx.RestParameter.TypeAnnotation != nil {
			resolvedRestType = c.resolveTypeAnnotation(ctx.RestParameter.TypeAnnotation)
			
			// Rest parameter type should be an array type
			if resolvedRestType != nil {
				if _, isArrayType := resolvedRestType.(*types.ArrayType); !isArrayType {
					c.addError(ctx.RestParameter.TypeAnnotation, fmt.Sprintf("rest parameter type must be an array type, got '%s'", resolvedRestType.String()))
					resolvedRestType = &types.ArrayType{ElementType: types.Any}
				}
			}
		}
		
		if resolvedRestType == nil {
			// Default to any[] if no annotation
			resolvedRestType = &types.ArrayType{ElementType: types.Any}
		}
		
		restParameterType = resolvedRestType
		restParameterName = ctx.RestParameter.Name
		ctx.RestParameter.ComputedType = restParameterType
	}
	
	// Resolve return type annotation
	var expectedReturnType types.Type
	if ctx.ReturnTypeAnnotation != nil {
		expectedReturnType = c.resolveTypeAnnotation(ctx.ReturnTypeAnnotation)
	}
	
	// Create preliminary signature
	optionalParams := make([]bool, len(ctx.Parameters))
	for i, param := range ctx.Parameters {
		optionalParams[i] = param.Optional || (param.DefaultValue != nil)
	}
	
	signature := &types.Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        expectedReturnType,
		OptionalParams:    optionalParams,
		IsVariadic:        ctx.RestParameter != nil,
		RestParameterType: restParameterType,
	}
	
	return signature, paramTypes, paramNames, restParameterType, restParameterName
}

// setupFunctionEnvironment creates the function scope and defines parameters
func (c *Checker) setupFunctionEnvironment(ctx *FunctionCheckContext, paramTypes []types.Type, paramNames []*parser.Identifier, restParameterType types.Type, restParameterName *parser.Identifier, preliminarySignature *types.Signature) *Environment {
	debugPrintf("// [Checker Function Common] Creating scope for '%s'. Current Env: %p\n", ctx.FunctionName, c.env)
	originalEnv := c.env
	funcEnv := NewEnclosedEnvironment(originalEnv)
	c.env = funcEnv
	
	// Define regular parameters
	for i, nameNode := range paramNames {
		if i < len(paramTypes) {
			if !funcEnv.Define(nameNode.Value, paramTypes[i], false) {
				c.addError(nameNode, fmt.Sprintf("duplicate parameter name: %s", nameNode.Value))
			}
		}
	}
	
	// Define rest parameter if present
	if restParameterName != nil && restParameterType != nil {
		if !funcEnv.Define(restParameterName.Value, restParameterType, false) {
			c.addError(restParameterName, fmt.Sprintf("duplicate parameter name: %s", restParameterName.Value))
		}
		debugPrintf("// [Checker Function Common] Defined rest parameter '%s' with type: %s\n", restParameterName.Value, restParameterType.String())
	}
	
	// Define function itself for recursion if allowed and named
	if ctx.AllowSelfReference && ctx.FunctionName != "<anonymous>" {
		tempFuncTypeForRecursion := types.NewFunctionType(preliminarySignature)
		if !funcEnv.Define(ctx.FunctionName, tempFuncTypeForRecursion, false) {
			// This might happen if a param has the same name - parser should likely prevent this
			debugPrintf("// [Checker Function Common] WARNING: function name '%s' conflicts with a parameter\n", ctx.FunctionName)
		}
	}
	
	return originalEnv
}

// checkFunctionBody visits the function body and handles return type inference
func (c *Checker) checkFunctionBody(ctx *FunctionCheckContext, expectedReturnType types.Type) types.Type {
	// Set return context
	outerExpectedReturnType := c.currentExpectedReturnType
	outerInferredReturnTypes := c.currentInferredReturnTypes
	
	c.currentExpectedReturnType = expectedReturnType
	c.currentInferredReturnTypes = nil
	if expectedReturnType == nil {
		c.currentInferredReturnTypes = []types.Type{}
	}
	
	var finalReturnType types.Type
	
	// Visit body
	c.visit(ctx.Body)
	
	// Handle different body types
	if ctx.IsArrow {
		// Special handling for arrow function expression bodies
		if exprBody, ok := ctx.Body.(parser.Expression); ok {
			bodyType := exprBody.GetComputedType()
			if bodyType == nil {
				bodyType = types.Any
			}
			
			// For expression bodies, the body type is the return type
			if c.currentInferredReturnTypes != nil {
				c.currentInferredReturnTypes = append(c.currentInferredReturnTypes, bodyType)
			}
			
			// Check expression body type against annotation
			if expectedReturnType != nil {
				if !types.IsAssignable(bodyType, expectedReturnType) {
					c.addError(exprBody, fmt.Sprintf("cannot return expression of type '%s' from arrow function with return type annotation '%s'", bodyType.String(), expectedReturnType.String()))
				}
				finalReturnType = expectedReturnType
			} else {
				finalReturnType = bodyType
			}
		} else {
			// Block body for arrow function - use normal inference
			finalReturnType = c.inferFinalReturnType(expectedReturnType, ctx.FunctionName)
		}
	} else {
		// Regular function - use normal inference
		finalReturnType = c.inferFinalReturnType(expectedReturnType, ctx.FunctionName)
	}
	
	// Restore return context
	c.currentExpectedReturnType = outerExpectedReturnType
	c.currentInferredReturnTypes = outerInferredReturnTypes
	
	return finalReturnType
}

// inferFinalReturnType handles return type inference logic
func (c *Checker) inferFinalReturnType(expectedReturnType types.Type, functionName string) types.Type {
	if expectedReturnType != nil {
		return expectedReturnType
	}
	
	// Infer from collected return types
	if len(c.currentInferredReturnTypes) == 0 {
		return types.Undefined
	}
	
	// Use NewUnionType to combine inferred return types
	finalType := types.NewUnionType(c.currentInferredReturnTypes...)
	debugPrintf("// [Checker Function Common] Inferred return type for '%s': %s\n", functionName, finalType.String())
	return finalType
}

// createFinalFunctionType creates the final unified ObjectType for the function
func (c *Checker) createFinalFunctionType(ctx *FunctionCheckContext, paramTypes []types.Type, finalReturnType types.Type, restParameterType types.Type) *types.ObjectType {
	optionalParams := make([]bool, len(ctx.Parameters))
	for i, param := range ctx.Parameters {
		optionalParams[i] = param.Optional || (param.DefaultValue != nil)
	}
	
	// Create final signature
	sig := &types.Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        finalReturnType,
		OptionalParams:    optionalParams,
		IsVariadic:        ctx.RestParameter != nil,
		RestParameterType: restParameterType,
	}
	
	// Create unified ObjectType with call signature
	return types.NewFunctionType(sig)
}
package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

func (c *Checker) checkFunctionLiteral(node *parser.FunctionLiteral) {
	// 1. Resolve explicit annotations FIRST to get the signature contract.
	//    Use resolveFunctionLiteralType helper, but pass the *current* env.
	//    This gives us the expected parameter types and *potential* return type.
	resolvedSignature := c.resolveFunctionLiteralType(node, c.env) // Pass c.env
	if resolvedSignature == nil {
		// Error resolving annotations (e.g., unknown type name)
		// Error should have been added by resolve helper.
		// Create a dummy Any signature to proceed safely.
		paramTypes := make([]types.Type, len(node.Parameters))
		for i := range paramTypes {
			paramTypes[i] = types.Any
		}
		resolvedSignature = &types.FunctionType{ParameterTypes: paramTypes, ReturnType: types.Any}
		// Set dummy type on node immediately to prevent nil checks later if we proceeded?
		// No, let's calculate the final type below and set it once.
	}

	// 2. Save outer return context
	outerExpectedReturnType := c.currentExpectedReturnType
	outerInferredReturnTypes := c.currentInferredReturnTypes

	// 3. Set context for BODY CHECK based ONLY on explicit return annotation.
	//    resolvedSignature.ReturnType can be nil if not annotated.
	c.currentExpectedReturnType = resolvedSignature.ReturnType // Use ReturnType from resolved signature
	c.currentInferredReturnTypes = nil                         // Reset inferred list for this function body
	if c.currentExpectedReturnType == nil {                    // Allocate ONLY if inference is actually needed
		c.currentInferredReturnTypes = []types.Type{}
	}

	// 4. Create function's inner scope & define parameters using resolved signature param types
	funcNameForLog := "<anonymous>"
	if node.Name != nil {
		funcNameForLog = node.Name.Value
	}
	debugPrintf("// [Checker Visit FuncLit] Creating INNER scope for '%s'. Current Env: %p\n", funcNameForLog, c.env)
	originalEnv := c.env
	funcEnv := NewEnclosedEnvironment(originalEnv)
	c.env = funcEnv
	for i, paramNode := range node.Parameters { // Iterate over parser nodes
		if i < len(resolvedSignature.ParameterTypes) { // Safety check using resolved signature
			paramType := resolvedSignature.ParameterTypes[i]
			if !funcEnv.Define(paramNode.Name.Value, paramType, false) {
				c.addError(paramNode.Name, fmt.Sprintf("duplicate parameter name: %s", paramNode.Name.Value))
			}
			// Set computed type on the Parameter node itself
			paramNode.ComputedType = paramType
		} else {
			// Mismatch between AST params and resolved signature params - internal error?
			debugPrintf("// [Checker FuncLit Visit] ERROR: Mismatch in param count for func '%s'\n", funcNameForLog)
		}
	}

	// --- NEW: Define rest parameter if present ---
	if node.RestParameter != nil && resolvedSignature.RestParameterType != nil {
		if !funcEnv.Define(node.RestParameter.Name.Value, resolvedSignature.RestParameterType, false) {
			c.addError(node.RestParameter.Name, fmt.Sprintf("duplicate parameter name: %s", node.RestParameter.Name.Value))
		}
		// Set computed type on the RestParameter node itself
		node.RestParameter.ComputedType = resolvedSignature.RestParameterType
		debugPrintf("// [Checker FuncLit Visit] Defined rest parameter '%s' with type: %s\n", node.RestParameter.Name.Value, resolvedSignature.RestParameterType.String())
	}
	// --- END NEW ---

	// --- Function name self-definition for recursion (if named) ---
	// Hoisting handles top-level/block-level, but let/const needs this.
	if node.Name != nil {
		// Re-use the resolvedSignature for the temporary definition
		// (ReturnType might still be nil here if not annotated)
		tempFuncTypeForRecursion := &types.FunctionType{
			ParameterTypes: resolvedSignature.ParameterTypes,
			ReturnType:     resolvedSignature.ReturnType, // Use potentially nil return type
		}
		if !funcEnv.Define(node.Name.Value, tempFuncTypeForRecursion, false) {
			// This might happen if a param has the same name - parser should likely prevent this
			c.addError(node.Name, fmt.Sprintf("function name '%s' conflicts with a parameter", node.Name.Value))
		}
	}
	// --- END Function name self-definition ---

	// 5. Visit Body (only if it exists - function signatures have nil body)
	if node.Body != nil {
		c.visit(node.Body)
	}

	// 6. Determine Final ACTUAL Return Type of the function body
	var actualReturnType types.Type
	if resolvedSignature.ReturnType != nil {
		// Annotation exists, use that as the final actual type.
		// Checks against this type happened during ReturnStatement visits.
		actualReturnType = resolvedSignature.ReturnType
	} else {
		// No annotation, INFER the return type from collected returns.
		if len(c.currentInferredReturnTypes) == 0 {
			actualReturnType = types.Undefined // No returns -> undefined
		} else {
			// Use NewUnionType to combine inferred return types
			actualReturnType = types.NewUnionType(c.currentInferredReturnTypes...)
		}
		debugPrintf("// [Checker FuncLit Visit] Inferred return type for '%s': %s\n", funcNameForLog, actualReturnType.String())
	}

	// --- Update self-definition if name existed and return type was inferred ---
	if node.Name != nil && resolvedSignature.ReturnType == nil {
		optionalParams := make([]bool, len(node.Parameters))
		for i, param := range node.Parameters {
			optionalParams[i] = param.Optional || (param.DefaultValue != nil)
		}

		finalFuncTypeForRecursion := &types.FunctionType{
			ParameterTypes: resolvedSignature.ParameterTypes,
			ReturnType:     actualReturnType, // Use the inferred type now
			OptionalParams: optionalParams,
		}
		// Update the function's own entry in its scope
		if !funcEnv.Update(node.Name.Value, finalFuncTypeForRecursion) {
			debugPrintf("// [Checker FuncLit Visit] WARNING: Failed to update self-definition for '%s'\n", node.Name.Value)
		}
	}
	// --- END Update self-definition ---

	// 7. Create the FINAL FunctionType representing this literal
	optionalParams := make([]bool, len(node.Parameters))
	for i, param := range node.Parameters {
		optionalParams[i] = param.Optional || (param.DefaultValue != nil)
	}

	finalFuncType := &types.FunctionType{
		ParameterTypes:    resolvedSignature.ParameterTypes, // Use types from annotation/defaults
		ReturnType:        actualReturnType,                 // Use the explicit or inferred return type
		OptionalParams:    optionalParams,
		IsVariadic:        resolvedSignature.IsVariadic,        // Add variadic info
		RestParameterType: resolvedSignature.RestParameterType, // Add rest parameter type
	}

	// 8. *** ALWAYS Set the Computed Type on the FunctionLiteral node ***
	debugPrintf("// [Checker FuncLit Visit] SETTING final computed type for '%s': %s\n", funcNameForLog, finalFuncType.String())
	node.SetComputedType(finalFuncType) // <<< THIS IS THE KEY FIX

	// --- NEW: Check for overload completion ---
	if node.Name != nil && len(c.env.GetPendingOverloads(node.Name.Value)) > 0 {
		c.completeOverloadedFunction(node.Name.Value, finalFuncType)
	}
	// --- END NEW ---

	// 9. Restore outer environment and context
	debugPrintf("// [Checker Visit FuncLit] Exiting '%s'. Restoring Env: %p (from current %p)\n", funcNameForLog, originalEnv, c.env)
	c.env = originalEnv
	c.currentExpectedReturnType = outerExpectedReturnType
	c.currentInferredReturnTypes = outerInferredReturnTypes
}

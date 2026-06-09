package checker

import (
	"fmt"
	"strings"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// processFunctionSignature handles function overload signatures by collecting them
// and checking their types.
func (c *Checker) processFunctionSignature(node *parser.FunctionSignature) {
	if node.Name == nil {
		c.addError(node, "function signature must have a name")
		return
	}

	functionName := node.Name.Value
	ctx := &FunctionCheckContext{
		FunctionName:         functionName,
		TypeParameters:       node.TypeParameters,
		Parameters:           node.Parameters,
		RestParameter:        node.RestParameter,
		ReturnTypeAnnotation: node.ReturnTypeAnnotation,
		Body:                 nil,
		IsArrow:              false,
		AllowSelfReference:   false,
	}
	sig, _, _, _, _, _ := c.resolveFunctionParameters(ctx)
	if sig == nil {
		c.addError(node, "invalid function signature")
		return
	}
	if sig.ReturnType == nil {
		if node.Declare {
			sig.ReturnType = types.Any
		} else {
			sig.ReturnType = types.Void
		}
	}

	// Create the function type for this signature
	funcType := types.NewFunctionType(sig)

	// For backward compatibility, create legacy FunctionType
	// funcType := &types.FunctionType{
	// 	ParameterTypes: paramTypes,
	// 	ReturnType:     returnType,
	// 	OptionalParams: optionalParams,
	// }

	// Set the computed type on the signature node
	// For now, continue using FunctionType for overloads until we update the entire overload system
	node.SetComputedType(funcType)
	if node.Declare {
		c.env.Define(functionName, funcType, false)
		debugPrintf("// [Checker] Added ambient function signature for '%s': %s\n", functionName, funcType.String())
		return
	}

	// Add the signature to pending overloads in the environment
	c.env.AddOverloadSignature(functionName, node)
	debugPrintf("// [Checker processFunctionSignature] Added to env %p\n", c.env)
	debugPrintf("// [Checker] Added overload signature for '%s': %s\n", functionName, funcType.String())
}

// completeOverloadedFunction creates an ObjectType with multiple call signatures when we encounter
// a function implementation that has pending overload signatures.
func (c *Checker) completeOverloadedFunction(functionName string, implementation *types.ObjectType) {
	debugPrintf("// [Checker completeOverloadedFunction] Starting completion for '%s'\n", functionName)
	debugPrintf("// [Checker completeOverloadedFunction] Checking env %p\n", c.env)

	// Get pending overload signatures from the GLOBAL environment (not current env)
	// because overload signatures are added during Pass 2 in the global scope
	globalEnv := c.env
	for globalEnv.outer != nil {
		globalEnv = globalEnv.outer
	}
	debugPrintf("// [Checker completeOverloadedFunction] Using global env %p\n", globalEnv)

	pendingSignatures := globalEnv.GetPendingOverloads(functionName)
	if len(pendingSignatures) == 0 {
		debugPrintf("// [Checker completeOverloadedFunction] No pending overloads for '%s'\n", functionName)
		return // No pending overloads
	}

	debugPrintf("// [Checker completeOverloadedFunction] Found %d pending overloads for '%s'\n", len(pendingSignatures), functionName)

	// Convert signatures to call signatures for unified ObjectType
	var overloadSignatures []*types.Signature
	for _, sig := range pendingSignatures {
		if sigType := sig.GetComputedType(); sigType != nil {
			if objType, ok := sigType.(*types.ObjectType); ok && objType.IsCallable() {
				// Extract call signatures from unified ObjectType
				if len(objType.CallSignatures) > 0 {
					overloadSignatures = append(overloadSignatures, objType.CallSignatures[0])
					debugPrintf("// [Checker completeOverloadedFunction] Added overload signature: %s\n", objType.CallSignatures[0].String())
				}
			}
		}
	}

	debugPrintf("// [Checker completeOverloadedFunction] Converted %d overload signatures\n", len(overloadSignatures))

	// Convert implementation ObjectType to Signature
	if len(implementation.CallSignatures) == 0 {
		debugPrintf("// [Checker completeOverloadedFunction] Implementation has no call signatures\n")
		return
	}
	implementationSig := implementation.CallSignatures[0] // Use first call signature

	// Validate that implementation is compatible with all overloads
	for i, overloadSig := range overloadSignatures {
		if !c.isSignatureCompatible(implementationSig, overloadSig) {
			sig := pendingSignatures[i]
			c.addError(sig, fmt.Sprintf("function implementation signature '%s' is not compatible with overload signature '%s'",
				implementationSig.String(), overloadSig.String()))
		}
	}

	// Complete the overloaded function in the environment using unified approach
	if globalEnv.CompleteOverloadedFunctionUTS(functionName, overloadSignatures, implementationSig) {
		debugPrintf("// [Checker] Completed unified overloaded function '%s' with %d overloads\n",
			functionName, len(overloadSignatures))
	} else {
		debugPrintf("// [Checker] FAILED to complete unified overloaded function '%s'\n", functionName)
		c.addError(nil, fmt.Sprintf("failed to complete overloaded function '%s'", functionName))
	}
}

// isImplementationCompatible checks if an implementation signature is compatible
// with an overload signature. This is a simplified check.
// DEPRECATED: Use isSignatureCompatible instead for unified ObjectType system.
func (c *Checker) isImplementationCompatible(implementation, overload *types.ObjectType) bool {
	debugPrintf("// [Checker isImplementationCompatible] Checking implementation %s against overload %s\n", implementation.String(), overload.String())

	// Extract call signatures from ObjectTypes
	if len(implementation.CallSignatures) == 0 || len(overload.CallSignatures) == 0 {
		debugPrintf("// [Checker isImplementationCompatible] Missing call signatures\n")
		return false
	}

	implSig := implementation.CallSignatures[0]
	overloadSig := overload.CallSignatures[0]

	// Delegate to unified signature compatibility check
	return c.isSignatureCompatible(implSig, overloadSig)

}

// isSignatureCompatible checks if an implementation signature is compatible
// with an overload signature. This is the unified version for Signature types.
func (c *Checker) isSignatureCompatible(implementation, overload *types.Signature) bool {
	debugPrintf("// [Checker isSignatureCompatible] Checking implementation %s against overload %s\n", implementation.String(), overload.String())

	implMin := requiredParameterCount(implementation)
	overloadMin := requiredParameterCount(overload)
	implMax := len(implementation.ParameterTypes)
	overloadMax := len(overload.ParameterTypes)

	// The implementation signature must accept every call accepted by the overload.
	if implMin > overloadMin || (!implementation.IsVariadic && implMax < overloadMax) {
		debugPrintf("// [Checker isSignatureCompatible] Parameter count range mismatch: impl %d..%d vs overload %d..%d\n", implMin, implMax, overloadMin, overloadMax)
		return false
	}

	// Check that each overload parameter type is assignable to the corresponding implementation parameter
	for i, overloadParam := range overload.ParameterTypes {
		if i >= len(implementation.ParameterTypes) {
			if implementation.IsVariadic && implementation.RestParameterType != nil {
				continue
			}
			return false
		}
		implParam := implementation.ParameterTypes[i]
		debugPrintf("// [Checker isSignatureCompatible] Checking param %d: overload %s assignable to impl %s\n", i, overloadParam.String(), implParam.String())
		if !types.IsAssignable(overloadParam, implParam) {
			debugPrintf("// [Checker isSignatureCompatible] Parameter %d incompatible: %s not assignable to %s\n", i, overloadParam.String(), implParam.String())
			return false
		}
		debugPrintf("// [Checker isSignatureCompatible] Parameter %d compatible\n", i)
	}

	// Check return type compatibility
	debugPrintf("// [Checker isSignatureCompatible] Checking return types: impl %s vs overload %s\n", implementation.ReturnType.String(), overload.ReturnType.String())

	// TypeScript rule: an overload with return type 'void' is compatible with any implementation
	// return type (the void overload means "callers don't use the return value").
	if overload.ReturnType == types.Void {
		return true
	}

	// For overloads, if the implementation return type is a union, check if the overload return type is one of the union members
	if implUnion, isUnion := implementation.ReturnType.(*types.UnionType); isUnion {
		// Check if the overload return type is assignable to any of the union types
		for _, unionMember := range implUnion.Types {
			if types.IsAssignable(overload.ReturnType, unionMember) {
				debugPrintf("// [Checker isSignatureCompatible] Return type compatible via union member %s\n", unionMember.String())
				return true
			}
		}
		debugPrintf("// [Checker isSignatureCompatible] Return type incompatible: overload %s not found in union %s\n", overload.ReturnType.String(), implUnion.String())
		return false
	} else {
		// Non-union implementation return type - use standard assignability
		result := types.IsAssignable(implementation.ReturnType, overload.ReturnType)
		debugPrintf("// [Checker isSignatureCompatible] Return type compatible: %t\n", result)
		return result
	}
}

func requiredParameterCount(sig *types.Signature) int {
	if sig == nil {
		return 0
	}

	count := len(sig.ParameterTypes)
	if len(sig.OptionalParams) == len(sig.ParameterTypes) {
		for i := len(sig.ParameterTypes) - 1; i >= 0; i-- {
			if sig.OptionalParams[i] {
				count--
			} else {
				break
			}
		}
	}

	return count
}

func (c *Checker) reportDuplicateIndexSignature(node parser.Node, indexSignatures []*types.IndexSignature, keyType types.Type) {
	if keyType == nil {
		return
	}
	for _, existing := range indexSignatures {
		if existing != nil && existing.KeyType != nil && existing.KeyType.String() == keyType.String() {
			c.addError(node, fmt.Sprintf("Duplicate index signature for type '%s'.", keyType.String()))
			c.addError(node, fmt.Sprintf("Duplicate index signature for type '%s'.", keyType.String()))
			return
		}
	}
}

// checkOverloadedCall handles function calls to overloaded functions by finding
// the best matching overload and using its return type.
// DEPRECATED: This function is replaced by checkOverloadedCallUnified in call.go
func (c *Checker) checkOverloadedCall(node *parser.CallExpression, overloadedFunc *types.ObjectType) {
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

	// Try to find the best matching overload using checker's isAssignable method
	overloadIndex := -1
	var resultType types.Type

	for i, overload := range overloadedFunc.CallSignatures {
		// Check if this overload can accept the given arguments
		var isMatching bool

		if overload.IsVariadic {
			// For variadic overloads, check minimum required arguments (fixed parameters)
			minRequiredArgs := len(overload.ParameterTypes)
			if len(argTypes) >= minRequiredArgs {
				// Check fixed parameters first
				fixedMatch := true
				for j := 0; j < minRequiredArgs; j++ {
					if !types.IsAssignable(argTypes[j], overload.ParameterTypes[j]) {
						fixedMatch = false
						break
					}
				}

				if fixedMatch {
					// Check remaining arguments against rest parameter type
					if overload.RestParameterType != nil {
						// Extract element type from rest parameter array type
						var elementType types.Type = types.Any
						if arrayType, ok := overload.RestParameterType.(*types.ArrayType); ok {
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
			minRequiredArgs := requiredParameterCount(overload)
			if len(argTypes) < minRequiredArgs || len(argTypes) > len(overload.ParameterTypes) {
				continue // Argument count mismatch
			}

			// Check if all argument types are assignable to parameter types
			allMatch := true
			for j, argType := range argTypes {
				paramType := overload.ParameterTypes[j]
				if !types.IsAssignable(argType, paramType) {
					allMatch = false
					break
				}
			}
			isMatching = allMatch
		}

		if isMatching {
			overloadIndex = i
			resultType = overload.ReturnType
			break // Found the first matching overload
		}
	}

	if overloadIndex == -1 {
		// No matching overload found
		var overloadSigs []string
		for _, overload := range overloadedFunc.CallSignatures {
			overloadSigs = append(overloadSigs, overload.String())
		}

		// Build argument type string for error message
		var argTypeStrs []string
		for _, argType := range argTypes {
			argTypeStrs = append(argTypeStrs, argType.String())
		}

		// Format overloads nicely - each on its own line with proper indentation
		overloadList := ""
		for i, sig := range overloadSigs {
			if i > 0 {
				overloadList += "\n"
			}
			overloadList += "  " + sig
		}

		c.addError(node, fmt.Sprintf("no overload matches call with arguments (%s). Available overloads:\n%s",
			"["+strings.Join(argTypeStrs, ", ")+"]", // Clean argument list
			overloadList)) // Each overload on its own line

		node.SetComputedType(types.Any)
		return
	}

	// Found a matching overload
	matchedOverload := overloadedFunc.CallSignatures[overloadIndex]
	debugPrintf("// [Checker OverloadCall] Found matching overload %d: %s for call with args (%v)\n",
		overloadIndex, matchedOverload.String(), argTypes)

	// Perform detailed argument type checking for the matched overload
	if matchedOverload.IsVariadic {
		// For variadic overloads, validate fixed parameters and rest parameters separately
		fixedParamCount := len(matchedOverload.ParameterTypes)

		// Check fixed parameters
		for i := 0; i < fixedParamCount; i++ {
			if i < len(argTypes) {
				argType := argTypes[i]
				paramType := matchedOverload.ParameterTypes[i]
				if !types.IsAssignable(argType, paramType) {
					argNode := node.Arguments[i]
					c.addError(argNode, fmt.Sprintf("argument %d: cannot assign type '%s' to parameter of type '%s'",
						i+1, argType.String(), paramType.String()))
				}
			}
		}

		// Check rest parameters if any
		if len(argTypes) > fixedParamCount && matchedOverload.RestParameterType != nil {
			var elementType types.Type = types.Any
			if arrayType, ok := matchedOverload.RestParameterType.(*types.ArrayType); ok {
				elementType = arrayType.ElementType
			}

			for i := fixedParamCount; i < len(argTypes); i++ {
				argType := argTypes[i]
				if !types.IsAssignable(argType, elementType) {
					argNode := node.Arguments[i]
					c.addError(argNode, fmt.Sprintf("variadic argument %d: cannot assign type '%s' to rest parameter element type '%s'",
						i+1, argType.String(), elementType.String()))
				}
			}
		}
	} else {
		// For non-variadic overloads, use the original validation logic
		if len(argTypes) != len(matchedOverload.ParameterTypes) {
			c.addError(node, fmt.Sprintf("internal error: matched overload has different arity"))
			node.SetComputedType(types.Any)
			return
		}

		for i, argType := range argTypes {
			paramType := matchedOverload.ParameterTypes[i]
			if !types.IsAssignable(argType, paramType) {
				// This shouldn't happen if overload matching worked correctly
				argNode := node.Arguments[i]
				c.addError(argNode, fmt.Sprintf("argument %d: cannot assign type '%s' to parameter of type '%s'",
					i+1, argType.String(), paramType.String()))
			}
		}
	}

	// Set the result type from the matched overload
	node.SetComputedType(resultType)
	debugPrintf("// [Checker OverloadCall] Set result type to: %s\n", resultType.String())
}

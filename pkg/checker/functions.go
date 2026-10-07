package checker

import (
	"fmt"
	"github.com/nooga/paserati/pkg/errors"

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
		if !c.env.Define(functionName, funcType, false) {
			c.mergeAmbientOverload(functionName, sig)
		}
		debugPrintf("// [Checker] Added ambient function signature for '%s': %s\n", functionName, funcType.String())
		return
	}

	// Add the signature to pending overloads in the environment
	c.env.AddOverloadSignature(functionName, node)
	debugPrintf("// [Checker processFunctionSignature] Added to env %p\n", c.env)
	debugPrintf("// [Checker] Added overload signature for '%s': %s\n", functionName, funcType.String())
}

func (c *Checker) hasPendingOverloads(functionName string) bool {
	for env := c.env; env != nil; env = env.outer {
		if len(env.GetPendingOverloads(functionName)) > 0 {
			return true
		}
	}
	return false
}

// completeOverloadedFunction creates an ObjectType with multiple call signatures when we encounter
// a function implementation that has pending overload signatures.
func (c *Checker) completeOverloadedFunction(functionName string, implementation *types.ObjectType) {
	debugPrintf("// [Checker completeOverloadedFunction] Starting completion for '%s'\n", functionName)
	debugPrintf("// [Checker completeOverloadedFunction] Checking env %p\n", c.env)

	// Complete overloads in the scope where their signatures were collected.
	// Top-level overloads live in the global environment; nested overloads live
	// in their block/function environment and must be cleared there.
	overloadEnv := c.env
	for overloadEnv != nil && len(overloadEnv.GetPendingOverloads(functionName)) == 0 {
		overloadEnv = overloadEnv.outer
	}
	if overloadEnv == nil {
		debugPrintf("// [Checker completeOverloadedFunction] No pending overload scope for '%s'\n", functionName)
		return
	}
	debugPrintf("// [Checker completeOverloadedFunction] Using overload env %p\n", overloadEnv)

	pendingSignatures := overloadEnv.GetPendingOverloads(functionName)
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
			c.addErrorWithCode(sig, errors.TS2394, "This overload signature is not compatible with its implementation signature.")
		}
	}

	// Complete the overloaded function in the environment using unified approach
	if overloadEnv.CompleteOverloadedFunctionUTS(functionName, overloadSignatures, implementationSig) {
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

	// Mirrors TypeScript's isImplementationCompatibleWithOverload: type
	// parameters are erased to any, the return types must be related in either
	// direction (or the overload returns void), and the implementation must be
	// assignable to the overload ignoring return types - i.e. it may declare
	// fewer parameters but never require more than the overload offers, and
	// parameter types are compared bivariantly.
	erase := func(t types.Type) types.Type {
		if t == nil || c.typeContainsTypeParameter(t) {
			return types.Any
		}
		return t
	}

	if overload.ReturnType != types.Void {
		implReturn, overloadReturn := erase(implementation.ReturnType), erase(overload.ReturnType)
		if !types.IsAssignable(overloadReturn, implReturn) && !types.IsAssignable(implReturn, overloadReturn) {
			return false
		}
	}

	if !overload.IsVariadic && requiredParameterCount(implementation) > len(overload.ParameterTypes) {
		return false
	}

	for i, overloadParam := range overload.ParameterTypes {
		if i >= len(implementation.ParameterTypes) {
			continue // extra overload parameters are simply ignored by a shorter implementation
		}
		implParam := implementation.ParameterTypes[i]
		overloadParam, implParam = erase(overloadParam), erase(implParam)
		if !types.IsAssignable(overloadParam, implParam) && !types.IsAssignable(implParam, overloadParam) {
			return false
		}
	}
	return true
}

// typeContainsTypeParameter reports whether t mentions a type parameter.
func (c *Checker) typeContainsTypeParameter(t types.Type) bool {
	sig := &types.Signature{ParameterTypes: []types.Type{t}}
	return c.isGenericSignature(sig)
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
			c.addErrorWithCode(node, errors.TS2374, fmt.Sprintf("Duplicate index signature for type '%s'.", keyType.String()))
			c.addErrorWithCode(node, errors.TS2374, fmt.Sprintf("Duplicate index signature for type '%s'.", keyType.String()))
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
		c.reportOverloadFailure(node, argTypes, overloadedFunc.CallSignatures)

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
					c.addErrorWithCode(argNode, errors.TS2345, fmt.Sprintf("Argument of type '%s' is not assignable to parameter of type '%s'.",
						argType.String(), paramType.String()))
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
					c.addErrorWithCode(argNode, errors.TS2345, fmt.Sprintf("Argument of type '%s' is not assignable to parameter of type '%s'.",
						argType.String(), elementType.String()))
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
				c.addErrorWithCode(argNode, errors.TS2345, fmt.Sprintf("Argument of type '%s' is not assignable to parameter of type '%s'.",
					argType.String(), paramType.String()))
			}
		}
	}

	// Set the result type from the matched overload
	node.SetComputedType(resultType)
	debugPrintf("// [Checker OverloadCall] Set result type to: %s\n", resultType.String())
}

// mergeAmbientOverload adds another `declare function f(...)` signature to the
// function type already bound to f in this scope, so that all the overloads of
// an ambient function are visible (the same signature is not added twice).
func (c *Checker) mergeAmbientOverload(name string, sig *types.Signature) {
	existing, _, found := c.env.Resolve(name)
	if !found {
		return
	}
	existingObj, ok := existing.(*types.ObjectType)
	if !ok || !existingObj.IsCallable() || len(existingObj.Properties) != 0 {
		return
	}
	for _, have := range existingObj.CallSignatures {
		if have.String() == sig.String() {
			return
		}
	}
	merged := &types.ObjectType{}
	merged.CallSignatures = append(append([]*types.Signature(nil), existingObj.CallSignatures...), sig)
	c.env.Update(name, merged)
}

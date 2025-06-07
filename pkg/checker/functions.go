package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"strings"
)

// processFunctionSignature handles function overload signatures by collecting them
// and checking their types.
func (c *Checker) processFunctionSignature(node *parser.FunctionSignature) {
	if node.Name == nil {
		c.addError(node, "function signature must have a name")
		return
	}

	functionName := node.Name.Value

	// Validate parameter types
	for _, param := range node.Parameters {
		if param.TypeAnnotation != nil {
			paramType := c.resolveTypeAnnotation(param.TypeAnnotation)
			param.ComputedType = paramType
		} else {
			c.addError(param.Name, fmt.Sprintf("function overload parameter '%s' must have type annotation", param.Name.Value))
		}
	}

	// Check return type annotation (required for signatures)
	if node.ReturnTypeAnnotation == nil {
		c.addError(node, "function signature must have return type annotation")
		return
	}

	returnType := c.resolveTypeAnnotation(node.ReturnTypeAnnotation)
	if returnType == nil {
		c.addError(node.ReturnTypeAnnotation, "invalid return type annotation")
		return
	}

	// Add the signature to pending overloads in the environment
	c.env.AddOverloadSignature(functionName, node)
	debugPrintf("// [Checker processFunctionSignature] Added to env %p\n", c.env)

	// Create the function type for this signature
	var paramTypes []types.Type
	var optionalParams []bool
	for _, param := range node.Parameters {
		if param.ComputedType != nil {
			paramTypes = append(paramTypes, param.ComputedType)
		} else {
			paramTypes = append(paramTypes, types.Any) // Fallback for error cases
		}
		optionalParams = append(optionalParams, param.Optional || (param.DefaultValue != nil))
	}

	funcType := &types.FunctionType{
		ParameterTypes: paramTypes,
		ReturnType:     returnType,
		OptionalParams: optionalParams,
	}

	// Set the computed type on the signature node
	node.SetComputedType(funcType)
	debugPrintf("// [Checker] Added overload signature for '%s': %s\n", functionName, funcType.String())
}

// completeOverloadedFunction creates an OverloadedFunctionType when we encounter
// a function implementation that has pending overload signatures.
func (c *Checker) completeOverloadedFunction(functionName string, implementation *types.FunctionType) {
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

	// Convert signatures to function types
	var overloadTypes []*types.FunctionType
	for _, sig := range pendingSignatures {
		if sigType := sig.GetComputedType(); sigType != nil {
			if funcType, ok := sigType.(*types.FunctionType); ok {
				overloadTypes = append(overloadTypes, funcType)
				debugPrintf("// [Checker completeOverloadedFunction] Added overload type: %s\n", funcType.String())
			}
		}
	}

	debugPrintf("// [Checker completeOverloadedFunction] Converted %d overload types\n", len(overloadTypes))

	// Validate that implementation is compatible with all overloads
	for i, overloadType := range overloadTypes {
		if !c.isImplementationCompatible(implementation, overloadType) {
			sig := pendingSignatures[i]
			c.addError(sig, fmt.Sprintf("function implementation signature '%s' is not compatible with overload signature '%s'",
				implementation.String(), overloadType.String()))
		}
	}

	// Complete the overloaded function in the environment
	if globalEnv.CompleteOverloadedFunction(functionName, overloadTypes, implementation) {
		debugPrintf("// [Checker] Completed overloaded function '%s' with %d overloads\n",
			functionName, len(overloadTypes))
	} else {
		debugPrintf("// [Checker] FAILED to complete overloaded function '%s'\n", functionName)
		c.addError(nil, fmt.Sprintf("failed to complete overloaded function '%s'", functionName))
	}
}

// isImplementationCompatible checks if an implementation signature is compatible
// with an overload signature. This is a simplified check.
func (c *Checker) isImplementationCompatible(implementation, overload *types.FunctionType) bool {
	debugPrintf("// [Checker isImplementationCompatible] Checking implementation %s against overload %s\n", implementation.String(), overload.String())

	// The implementation must be able to accept all the parameter types from the overload
	// For now, we'll use a simple check that the implementation has compatible parameter types

	if len(implementation.ParameterTypes) != len(overload.ParameterTypes) {
		debugPrintf("// [Checker isImplementationCompatible] Parameter count mismatch: impl %d vs overload %d\n", len(implementation.ParameterTypes), len(overload.ParameterTypes))
		return false
	}

	// Check that each overload parameter type is assignable to the corresponding implementation parameter
	// For overloaded functions, the implementation parameter must be able to accept the overload parameter
	for i, overloadParam := range overload.ParameterTypes {
		implParam := implementation.ParameterTypes[i]
		debugPrintf("// [Checker isImplementationCompatible] Checking param %d: overload %s assignable to impl %s\n", i, overloadParam.String(), implParam.String())
		// The overload parameter must be assignable to the implementation parameter
		// This means the implementation parameter is a union or supertype that can handle the overload parameter
		if !types.IsAssignable(overloadParam, implParam) {
			debugPrintf("// [Checker isImplementationCompatible] Parameter %d incompatible: %s not assignable to %s\n", i, overloadParam.String(), implParam.String())
			return false
		}
		debugPrintf("// [Checker isImplementationCompatible] Parameter %d compatible\n", i)
	}

	// Check return type compatibility - for overloads, we need more flexible checking
	// The implementation return type can be a union that includes the overload return type
	debugPrintf("// [Checker isImplementationCompatible] Checking return types: impl %s vs overload %s\n", implementation.ReturnType.String(), overload.ReturnType.String())

	// For overloads, if the implementation return type is a union, check if the overload return type is one of the union members
	if implUnion, isUnion := implementation.ReturnType.(*types.UnionType); isUnion {
		// Check if the overload return type is assignable to any of the union types
		for _, unionMember := range implUnion.Types {
			if types.IsAssignable(overload.ReturnType, unionMember) {
				debugPrintf("// [Checker isImplementationCompatible] Return type compatible via union member %s\n", unionMember.String())
				return true
			}
		}
		// If no union member is compatible, it's not compatible
		debugPrintf("// [Checker isImplementationCompatible] Return type incompatible: overload %s not found in union %s\n", overload.ReturnType.String(), implUnion.String())
		return false
	} else {
		// Non-union implementation return type - use standard assignability
		result := types.IsAssignable(implementation.ReturnType, overload.ReturnType)
		debugPrintf("// [Checker isImplementationCompatible] Return type compatible: %t\n", result)
		return result
	}
}

// checkOverloadedCall handles function calls to overloaded functions by finding
// the best matching overload and using its return type.
func (c *Checker) checkOverloadedCall(node *parser.CallExpression, overloadedFunc *types.OverloadedFunctionType) {
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

	for i, overload := range overloadedFunc.Overloads {
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
			// For non-variadic overloads, argument count must match exactly
			if len(argTypes) != len(overload.ParameterTypes) {
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
		for _, overload := range overloadedFunc.Overloads {
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
	matchedOverload := overloadedFunc.Overloads[overloadIndex]
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

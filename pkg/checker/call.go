package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

func (c *Checker) checkCallExpression(node *parser.CallExpression) {
	// --- UPDATED: Handle CallExpression with Overload Support ---
	// 1. Check the expression being called
	debugPrintf("// [Checker CallExpr] Checking call at line %d\n", node.Token.Line)
	c.visit(node.Function)
	funcNodeType := node.Function.GetComputedType()
	debugPrintf("// [Checker CallExpr] Function type resolved to: %T (%v)\n", funcNodeType, funcNodeType)

	if funcNodeType == nil {
		// Error visiting the function expression itself
		funcIdent, isIdent := node.Function.(*parser.Identifier)
		errMsg := "cannot determine type of called expression"
		if isIdent {
			errMsg = fmt.Sprintf("cannot determine type of called identifier '%s'", funcIdent.Value)
		}
		c.addError(node, errMsg)
		node.SetComputedType(types.Any)
		return
	}

	if funcNodeType == types.Any {
		// Allow calling 'any', result is 'any'. Check args against 'any'.
		for _, argNode := range node.Arguments {
			c.visit(argNode) // Visit args even if function is 'any'
		}
		node.SetComputedType(types.Any)
		return
	}

	// --- NEW: Check for overloaded functions ---
	if overloadedFunc, ok := funcNodeType.(*types.OverloadedFunctionType); ok {
		debugPrintf("// [Checker CallExpr] Processing overloaded function: %s\n", overloadedFunc.Name)
		c.checkOverloadedCall(node, overloadedFunc)
		return
	}
	// --- END NEW ---

	// Handle both FunctionType and CallableType
	var funcType *types.FunctionType
	if ft, ok := funcNodeType.(*types.FunctionType); ok {
		funcType = ft
	} else if ct, ok := funcNodeType.(*types.CallableType); ok {
		// Use the call signature of the callable type
		funcType = ct.CallSignature
		if funcType == nil {
			c.addError(node, fmt.Sprintf("callable type has no call signature"))
			node.SetComputedType(types.Any)
			return
		}
	} else {
		c.addError(node, fmt.Sprintf("cannot call value of type '%s'", funcNodeType.String()))
		node.SetComputedType(types.Any) // Result type is unknown/error
		return
	}

	debugPrintf("// [Checker CallExpr] Processing regular function type: %s, IsVariadic: %t\n", funcType.String(), funcType.IsVariadic)

	// --- MODIFIED Arity and Argument Type Checking ---
	actualArgCount := len(node.Arguments)

	if funcType.IsVariadic {
		// --- Variadic Function Check ---
		debugPrintf("// [Checker CallExpr] Checking variadic function, RestParameterType: %T (%v)\n", funcType.RestParameterType, funcType.RestParameterType)
		if funcType.RestParameterType == nil {
			c.addError(node, "internal checker error: variadic function type must have a rest parameter type")
			node.SetComputedType(types.Any) // Error state
			return
		}

		// Calculate minimum required arguments (excluding optional parameters)
		minExpectedArgs := len(funcType.ParameterTypes)
		if len(funcType.OptionalParams) == len(funcType.ParameterTypes) {
			// Count required parameters from the end
			for i := len(funcType.ParameterTypes) - 1; i >= 0; i-- {
				if funcType.OptionalParams[i] {
					minExpectedArgs--
				} else {
					break // Stop at first required parameter from the end
				}
			}
		}

		if actualArgCount < minExpectedArgs {
			c.addError(node, fmt.Sprintf("expected at least %d arguments for variadic function, but got %d", minExpectedArgs, actualArgCount))
			// Don't check args if minimum count isn't met.
		} else {
			// Check fixed arguments - check all provided fixed parameters, not just minimum required
			fixedArgsOk := true
			fixedArgCount := len(funcType.ParameterTypes)
			if actualArgCount < fixedArgCount {
				fixedArgCount = actualArgCount // Don't check more arguments than provided
			}

			for i := 0; i < fixedArgCount; i++ {
				argNode := node.Arguments[i]
				c.visit(argNode)
				argType := argNode.GetComputedType()
				paramType := funcType.ParameterTypes[i]
				if argType == nil { // Error visiting arg
					fixedArgsOk = false
					continue
				}
				if !c.isAssignable(argType, paramType) {
					c.addError(argNode, fmt.Sprintf("argument %d: cannot assign type '%s' to parameter of type '%s'", i+1, argType.String(), paramType.String()))
					fixedArgsOk = false
				}
			}

			// Check variadic arguments
			if fixedArgsOk { // Only check variadic part if fixed part was okay
				variadicParamType := funcType.RestParameterType
				arrayType, isArray := variadicParamType.(*types.ArrayType)
				if !isArray {
					c.addError(node, fmt.Sprintf("internal checker error: variadic parameter type must be an array type, got %s", variadicParamType.String()))
				} else {
					variadicElementType := arrayType.ElementType
					if variadicElementType == nil { // Should not happen with valid types
						variadicElementType = types.Any
					}
					// Check remaining arguments against the element type - start after all fixed parameters
					for i := len(funcType.ParameterTypes); i < actualArgCount; i++ {
						argNode := node.Arguments[i]
						c.visit(argNode)
						argType := argNode.GetComputedType()
						if argType == nil { // Error visiting arg
							continue
						}

						// --- NEW: Handle spread elements specially ---
						if spreadElement, isSpread := argNode.(*parser.SpreadElement); isSpread {
							// For spread elements, check that the spread argument is assignable to the rest parameter type
							if !c.isAssignable(argType, variadicParamType) {
								c.addError(spreadElement, fmt.Sprintf("spread argument: cannot assign type '%s' to rest parameter type '%s'", argType.String(), variadicParamType.String()))
							}
						} else {
							// For regular arguments, check against element type
							if !c.isAssignable(argType, variadicElementType) {
								c.addError(argNode, fmt.Sprintf("variadic argument %d: cannot assign type '%s' to parameter element type '%s'", i+1, argType.String(), variadicElementType.String()))
							}
						}
						// --- END NEW ---
					}
				}
			}
		}
	} else {
		// --- Non-Variadic Function Check (Updated for Optional Parameters) ---
		expectedArgCount := len(funcType.ParameterTypes)

		// Calculate minimum required arguments (non-optional parameters)
		minRequiredArgs := expectedArgCount
		if len(funcType.OptionalParams) == expectedArgCount {
			// Count required parameters from the end
			for i := expectedArgCount - 1; i >= 0; i-- {
				if funcType.OptionalParams[i] {
					minRequiredArgs--
				} else {
					break // Stop at first required parameter from the end
				}
			}
		}

		if actualArgCount < minRequiredArgs {
			c.addError(node, fmt.Sprintf("expected at least %d arguments, but got %d", minRequiredArgs, actualArgCount))
			// Continue checking assignable args anyway? Let's stop if arity wrong.
		} else if actualArgCount > expectedArgCount {
			c.addError(node, fmt.Sprintf("expected at most %d arguments, but got %d", expectedArgCount, actualArgCount))
		} else {
			// Check Argument Types (only for provided arguments)
			for i, argNode := range node.Arguments {
				c.visit(argNode) // Visit argument to compute its type
				argType := argNode.GetComputedType()
				paramType := funcType.ParameterTypes[i]

				if argType == nil {
					// Error computing argument type, can't check assignability
					continue
				}

				if !c.isAssignable(argType, paramType) {
					c.addError(argNode, fmt.Sprintf("argument %d: cannot assign type '%s' to parameter of type '%s'", i+1, argType.String(), paramType.String()))
				}
			}
		}
	}
	// --- END MODIFIED Checking ---

	// Set Result Type (unchanged)
	debugPrintf("// [Checker CallExpr] Setting result type from func '%s'. ReturnType from Sig: %T (%v)\n", node.Function.String(), funcType.ReturnType, funcType.ReturnType)
	node.SetComputedType(funcType.ReturnType)

}

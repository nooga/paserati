package checker

import (
	"fmt"
	"github.com/nooga/paserati/pkg/errors"
	"strings"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// iifeParameters returns the parameters of a function or arrow literal that
// is called directly (an IIFE), or nil.
func iifeParameters(callee parser.Expression) []*parser.Parameter {
	switch fn := callee.(type) {
	case *parser.FunctionLiteral:
		return fn.Parameters
	case *parser.ArrowFunctionLiteral:
		return fn.Parameters
	}
	return nil
}

// formatArityError renders a TypeScript-style TS2554 "Expected N arguments,
// but got M." message. When min < max (some trailing parameters are
// optional), the expected count is rendered as a "min-max" range, matching
// how TypeScript reports arity errors for signatures with optional
// parameters (e.g. "Expected 1-2 arguments, but got 0.").
func formatArityError(min, max, actual int) string {
	if min < max {
		return fmt.Sprintf("Expected %d-%d arguments, but got %d.", min, max, actual)
	}
	return fmt.Sprintf("Expected %d arguments, but got %d.", min, actual)
}

// calculateEffectiveArgCount calculates the effective number of arguments,
// expanding spread elements based on tuple types (following TypeScript behavior)
func (c *Checker) calculateEffectiveArgCount(arguments []parser.Expression) int {
	count := 0
	for i, arg := range arguments {
		if spreadElement, isSpread := arg.(*parser.SpreadElement); isSpread {
			debugPrintf("// [Checker EffectiveArgCount] Found spread element at index %d\n", i)
			// Visit the spread argument to get its type
			c.visit(spreadElement.Argument)
			argType := spreadElement.Argument.GetComputedType()
			debugPrintf("// [Checker EffectiveArgCount] Spread argument type: %T (%v)\n", argType, argType)

			if argType != nil {
				// Check for tuple types first (they have known length)
				if tupleType, isTuple := argType.(*types.TupleType); isTuple {
					elemCount := len(tupleType.ElementTypes)
					debugPrintf("// [Checker EffectiveArgCount] Tuple type with %d elements\n", elemCount)
					count += elemCount
				} else if _, isArray := argType.(*types.ArrayType); isArray {
					// For direct array literals, TypeScript allows them (infers as tuple)
					if arrayLit, isArrayLit := spreadElement.Argument.(*parser.ArrayLiteral); isArrayLit {
						elemCount := len(arrayLit.Elements)
						debugPrintf("// [Checker EffectiveArgCount] Array literal with %d elements (treated as tuple)\n", elemCount)
						count += elemCount
					} else {
						// For array variables/expressions, we can't determine length at compile time
						// Return -1 to signal that arity checking should be skipped
						debugPrintf("// [Checker EffectiveArgCount] Array type (not tuple) - unknown length, skipping arity check\n")
						return -1 // Signal: arity check should be skipped
					}
				} else if c.isSpreadableIterableType(argType) {
					debugPrintf("// [Checker EffectiveArgCount] Iterable type - unknown length, skipping arity check\n")
					return -1
				} else {
					// Spread on non-array/tuple type - error
					debugPrintf("// [Checker EffectiveArgCount] Spread on non-array type %T, adding 1\n", argType)
					count += 1
				}
			} else {
				// Error getting type - count as 1 to continue error checking
				debugPrintf("// [Checker EffectiveArgCount] Error getting spread type, adding 1\n")
				count += 1
			}
		} else {
			// Regular argument
			debugPrintf("// [Checker EffectiveArgCount] Regular argument at index %d\n", i)
			count += 1
		}
	}
	debugPrintf("// [Checker EffectiveArgCount] Total effective count: %d\n", count)
	return count
}

// validateSpreadArgument checks if a spread argument is valid according to TypeScript rules
// It now supports contextual typing to infer array literals as tuples when appropriate
func (c *Checker) validateSpreadArgument(spreadElement *parser.SpreadElement, isVariadicFunction bool, expectedParamTypes []types.Type, currentParamIndex int) bool {
	// Try contextual typing for array literals in non-variadic functions
	if !isVariadicFunction {
		if arrayLit, isArrayLit := spreadElement.Argument.(*parser.ArrayLiteral); isArrayLit {
			// Calculate how many parameters remain from current position
			remainingParamCount := len(expectedParamTypes) - currentParamIndex
			if remainingParamCount > 0 && len(arrayLit.Elements) <= remainingParamCount {
				// Create a tuple type from the remaining parameter types
				var tupleElementTypes []types.Type
				for i := 0; i < len(arrayLit.Elements) && currentParamIndex+i < len(expectedParamTypes); i++ {
					tupleElementTypes = append(tupleElementTypes, expectedParamTypes[currentParamIndex+i])
				}

				if len(tupleElementTypes) == len(arrayLit.Elements) {
					// Create tuple type for contextual typing
					expectedTupleType := &types.TupleType{
						ElementTypes: tupleElementTypes,
					}

					if checkerDebug {
						debugPrintf("// [Checker SpreadValidation] Using contextual tuple type: %s for array literal\n", expectedTupleType.String())
					}

					// Use contextual typing to check the array literal
					c.visitWithContext(spreadElement.Argument, &ContextualType{
						ExpectedType: expectedTupleType,
						IsContextual: true,
					})

					argType := spreadElement.Argument.GetComputedType()
					if argType != nil {
						if _, isTuple := argType.(*types.TupleType); isTuple {
							return true // Successfully inferred as tuple
						}
					}
				}
			}
		}
	}

	// Fallback: visit normally and check traditional rules
	c.visit(spreadElement.Argument)
	argType := spreadElement.Argument.GetComputedType()

	if argType == nil {
		return false // Error already reported
	}

	// Check if it's a tuple type (always valid)
	if _, isTuple := argType.(*types.TupleType); isTuple {
		return true
	}

	// Check if it's a direct array literal (TypeScript treats as tuple)
	if _, isArray := argType.(*types.ArrayType); isArray {
		if _, isArrayLit := spreadElement.Argument.(*parser.ArrayLiteral); isArrayLit {
			return true // Direct array literals are treated as tuples
		}
	}

	// If it's a variadic function, arrays are allowed (rest parameters)
	if isVariadicFunction {
		if _, isArray := argType.(*types.ArrayType); isArray {
			return true
		}
	}

	// Arrays can be spread into fixed parameters - JavaScript allows this
	// The runtime will use as many elements as there are parameters
	if _, isArray := argType.(*types.ArrayType); isArray {
		return true // Allow spreading arrays into fixed parameters
	}

	if isVariadicFunction && currentParamIndex >= len(expectedParamTypes) && c.isSpreadableIterableType(argType) {
		return true
	}

	// Only reject if it's not an array or tuple at all
	c.reportBadSpreadArgument(spreadElement, argType)
	return false
}

// checkFixedArgumentsWithSpread checks fixed parameters against arguments,
// properly expanding spread elements
func (c *Checker) checkFixedArgumentsWithSpread(arguments []parser.Expression, paramTypes []types.Type, isVariadicFunction bool, optionalParams ...[]bool) bool {
	// Extract optional params info if provided
	var isOptional []bool
	if len(optionalParams) > 0 {
		isOptional = optionalParams[0]
	}
	allOk := true
	effectiveArgIndex := 0
	visited := c.skipArgContexts
	c.skipArgContexts = nil

	for _, argNode := range arguments {
		if spreadElement, isSpread := argNode.(*parser.SpreadElement); isSpread {
			// Validate the spread argument first
			if !c.validateSpreadArgument(spreadElement, isVariadicFunction, paramTypes, effectiveArgIndex) {
				allOk = false
				effectiveArgIndex += 1 // Conservative estimate for error recovery
				continue
			}

			// Handle spread element type checking
			argType := spreadElement.Argument.GetComputedType()

			if tupleType, isTuple := argType.(*types.TupleType); isTuple {
				// Check each tuple element against the corresponding parameter
				for j, elemType := range tupleType.ElementTypes {
					if effectiveArgIndex+j < len(paramTypes) {
						paramType := paramTypes[effectiveArgIndex+j]
						if !c.rel.IsAssignable(elemType, paramType) {
							c.addErrorWithCode(spreadElement, errors.TS2345, fmt.Sprintf("Argument of type '%s' is not assignable to parameter of type '%s'.", elemType.String(), paramType.String()))
							allOk = false
						}
					}
				}
				effectiveArgIndex += len(tupleType.ElementTypes)
			} else if _, isArray := argType.(*types.ArrayType); isArray {
				// Must be a direct array literal (validated above)
				if arrayLit, isArrayLit := spreadElement.Argument.(*parser.ArrayLiteral); isArrayLit {
					// Check each element against the corresponding parameter
					for j, element := range arrayLit.Elements {
						if effectiveArgIndex+j < len(paramTypes) {
							c.visit(element)
							elemType := element.GetComputedType()
							paramType := paramTypes[effectiveArgIndex+j]

							if elemType != nil && !c.rel.IsAssignable(elemType, paramType) {
								c.addError(element, fmt.Sprintf("spread element %d: cannot assign type '%s' to parameter of type '%s'", j+1, elemType.String(), paramType.String()))
								allOk = false
							}
						}
					}
					effectiveArgIndex += len(arrayLit.Elements)
				} else {
					// This should not happen due to validation above
					allOk = false
					effectiveArgIndex += 1
				}
			} else if c.isSpreadableIterableType(argType) {
				// Iterator protocol values have unknown length and element type here.
				effectiveArgIndex = len(paramTypes)
			} else {
				// This should not happen due to validation above
				allOk = false
				effectiveArgIndex += 1
			}
		} else {
			// Regular argument
			// Use contextual typing if we have a matching parameter type
			if ctx, seen := visited[argNode]; seen && effectiveArgIndex < len(paramTypes) &&
				sameArgContext(ctx, argNode.GetComputedType(), paramTypes[effectiveArgIndex]) {
				// Inference already visited it in this very context, or
				// the parameter is the argument's own type.
			} else if effectiveArgIndex < len(paramTypes) {
				paramType := paramTypes[effectiveArgIndex]
				// Use contextual typing for the argument
				c.visitWithContext(argNode, &ContextualType{
					ExpectedType: paramType,
					IsContextual: true,
				})
			} else if !isVariadicFunction {
				// No corresponding parameter type and function is not variadic.
				// Visit so the excess argument still gets type info / errors.
				// For variadic functions, the caller's variadic loop owns these
				// arguments — visiting here would double-emit checker errors.
				c.visit(argNode)
			}

			argType := argNode.GetComputedType()

			if effectiveArgIndex < len(paramTypes) {
				paramType := paramTypes[effectiveArgIndex]
				// Optional parameters implicitly accept undefined
				paramIsOptional := effectiveArgIndex < len(isOptional) && isOptional[effectiveArgIndex]
				if paramIsOptional {
					paramType = types.NewUnionType(paramType, types.Undefined)
				}
				if argType != nil && (!c.isArgumentAssignableWithExpansion(argType, paramType) || c.findExcessProperty(argNode, paramType) != nil) {
					// Like TypeScript, only the first argument that does not
					// fit is reported (getSignatureApplicabilityError).
					if allOk {
						if tag, isJSX := c.jsxAttributes[argNode]; isJSX {
							// A JSX element's attributes are reported as an
							// assignment, at the tag name.
							c.reportNotAssignable(tag, argNode, argType, paramType, headAssign)
						} else {
							c.reportNotAssignable(argNode, argNode, argType, paramType, headArgument)
						}
					}
					allOk = false
				}
			}
			effectiveArgIndex += 1
		}
	}

	return allOk
}

// sameArgContext reports whether visiting an argument against param can
// change nothing: inference already visited it in that context, or param is
// (a copy of) the argument's own type.
func sameArgContext(visitedCtx, argType, param types.Type) bool {
	if param == nil {
		return false
	}
	if visitedCtx != nil && (visitedCtx == param || visitedCtx.Equals(param)) {
		return true
	}
	return argType != nil && (argType == param || argType.Equals(param))
}

// checkSuperCallExpression handles super() constructor calls
func (c *Checker) checkSuperCallExpression(node *parser.CallExpression, superExpr *parser.SuperExpression) {
	debugPrintf("// [Checker CallExpr] Handling super() call\n")

	// Super calls are only valid in constructors
	if c.currentClassContext == nil || c.currentClassContext.ContextType != types.AccessContextConstructor {
		c.addErrorWithCode(node, errors.TS2337, "Super calls are not permitted outside constructors or in nested functions inside constructors.")
		node.SetComputedType(types.Any)
		return
	}

	// Get the current class instance type from the checker state
	classInstanceType := c.currentClassInstanceType
	if classInstanceType == nil || classInstanceType.ClassMeta == nil {
		c.addError(node, "super() can only be used within a class context")
		node.SetComputedType(types.Any)
		return
	}

	// Check if the current class has a superclass
	superClassName := classInstanceType.ClassMeta.SuperClassName
	if superClassName == "" {
		c.reportSuperWithoutBase(node, classInstanceType, true)
		for _, argNode := range node.Arguments {
			c.visit(argNode)
		}
		node.SetComputedType(types.Any)
		return
	}

	// Get the superclass constructor - handle parameterized types properly
	var superConstructor types.Type
	var exists bool

	// Check if superclass name contains type parameters (e.g., "Comparable<T[]>")
	if strings.Contains(superClassName, "<") {
		debugPrintf("// [Checker CallExpr] Detected parameterized superclass: %s\n", superClassName)
		// Extract base class name (everything before '<')
		baseClassName := strings.Split(superClassName, "<")[0]
		debugPrintf("// [Checker CallExpr] Extracting base class: %s\n", baseClassName)

		// Resolve the base class constructor
		superConstructor, _, exists = c.env.Resolve(baseClassName)
		if !exists {
			c.addError(node, fmt.Sprintf("could not resolve base superclass constructor '%s'", baseClassName))
			node.SetComputedType(types.Any)
			return
		}

		// For generic constructors, we need the instantiated constructor from the current class's superclass type
		// Look it up in the BaseTypes of the current class instance
		if len(classInstanceType.Bases()) > 0 {
			// The first base type should be the superclass instance type
			if _, ok := classInstanceType.Bases()[0].(*types.ObjectType); ok {
				// Find the constructor type that matches this instance type
				debugPrintf("// [Checker CallExpr] Found superclass instance type, will use its constructor\n")
				// For now, use a simplified approach - the generic case is handled during inheritance setup
				// The BaseTypes[0] should already be the correctly instantiated superclass type
			}
		}
	} else {
		// Simple case - resolve the constructor directly
		superConstructor, _, exists = c.env.Resolve(superClassName)
		if !exists {
			c.addError(node, fmt.Sprintf("could not resolve superclass constructor '%s'", superClassName))
			node.SetComputedType(types.Any)
			return
		}
	}

	// Handle different types of superclass constructors
	var finalConstructor *types.ObjectType

	if objType, ok := superConstructor.(*types.ObjectType); ok && len(objType.ConstructSigs()) > 0 {
		finalConstructor = objType
	} else if _, ok := superConstructor.(*types.GenericType); ok {
		debugPrintf("// [Checker CallExpr] Superclass constructor is generic, need to instantiate it\n")
		// For parameterized superclasses, we need to use the already instantiated constructor
		// that was resolved during inheritance setup. It should be stored in SuperConstructorType.
		if classInstanceType.ClassMeta.SuperConstructorType != nil {
			if objType, ok := classInstanceType.ClassMeta.SuperConstructorType.(*types.ObjectType); ok && len(objType.ConstructSigs()) > 0 {
				finalConstructor = objType
				debugPrintf("// [Checker CallExpr] Using pre-instantiated superclass constructor\n")
			}
		}

		if finalConstructor == nil {
			// Fallback: this shouldn't happen if inheritance was set up correctly
			c.addError(node, fmt.Sprintf("could not instantiate generic superclass constructor '%s'", superClassName))
			node.SetComputedType(types.Any)
			return
		}
	} else {
		c.addError(node, fmt.Sprintf("superclass '%s' does not have a constructor", superClassName))
		node.SetComputedType(types.Any)
		return
	}

	// Type-check and validate arguments against constructor signature
	if len(finalConstructor.ConstructSigs()) > 0 {
		constructorSig := finalConstructor.ConstructSigs()[0]

		// First validate all spread arguments
		hasSpreadErrors := false
		currentArgIndex := 0
		for _, arg := range node.Arguments {
			if spreadElement, isSpread := arg.(*parser.SpreadElement); isSpread {
				if !c.validateSpreadArgument(spreadElement, constructorSig.IsVariadic, constructorSig.ParameterTypes, currentArgIndex) {
					hasSpreadErrors = true
				}
			}
			currentArgIndex += 1
		}

		// If there are spread errors, skip detailed arity checking
		if !hasSpreadErrors {
			// Calculate effective argument count, expanding spread elements
			actualArgCount := c.calculateEffectiveArgCount(node.Arguments)
			skipArityCheck := actualArgCount == -1 // -1 signals unknown length arrays in spreads

			if constructorSig.IsVariadic {
				// Variadic constructor - check minimum required args
				minExpectedArgs := len(constructorSig.ParameterTypes)
				if len(constructorSig.OptionalParams) == len(constructorSig.ParameterTypes) {
					for i := len(constructorSig.ParameterTypes) - 1; i >= 0; i-- {
						if constructorSig.OptionalParams[i] {
							minExpectedArgs--
						} else {
							break
						}
					}
				}

				if !skipArityCheck && actualArgCount < minExpectedArgs {
					c.addError(node, fmt.Sprintf("Constructor expected at least %d arguments but got %d.", minExpectedArgs, actualArgCount))
				} else {
					// Check fixed arguments
					fixedArgsOk := c.checkFixedArgumentsWithSpread(node.Arguments, constructorSig.ParameterTypes, constructorSig.IsVariadic)

					// Check variadic arguments
					if fixedArgsOk && constructorSig.RestParameterType != nil {
						arrayType, isArray := constructorSig.RestParameterType.(*types.ArrayType)
						if !isArray {
							c.visitRemainingArguments(node.Arguments, len(constructorSig.ParameterTypes))
						} else {
							variadicElementType := arrayType.ElementType
							if variadicElementType == nil {
								variadicElementType = types.Any
							}
							// Check remaining arguments against the element type
							for i := len(constructorSig.ParameterTypes); i < len(node.Arguments); i++ {
								argNode := node.Arguments[i]

								if spreadElement, isSpread := argNode.(*parser.SpreadElement); isSpread {
									if !c.validateSpreadArgument(spreadElement, true, []types.Type{}, 0) {
										continue
									}

									c.visit(spreadElement.Argument)
									argType := spreadElement.Argument.GetComputedType()
									if argType == nil {
										continue
									}

									if !c.rel.IsAssignable(argType, constructorSig.RestParameterType) {
										c.addErrorWithCode(spreadElement, errors.TS2345, fmt.Sprintf("Argument of type '%s' is not assignable to parameter of type '%s'.", argType.String(), constructorSig.RestParameterType.String()))
									}
								} else {
									c.visitWithContext(argNode, &ContextualType{
										ExpectedType: variadicElementType,
										IsContextual: true,
									})

									argType := argNode.GetComputedType()
									if argType == nil {
										continue
									}

									if !c.rel.IsAssignable(argType, variadicElementType) {
										c.addErrorWithCode(argNode, errors.TS2345, fmt.Sprintf("Argument of type '%s' is not assignable to parameter of type '%s'.", argType.String(), variadicElementType.String()))
									}
								}
							}
						}
					}
				}
			} else {
				// Non-variadic constructor
				expectedArgCount := len(constructorSig.ParameterTypes)
				minRequiredArgs := expectedArgCount
				if len(constructorSig.OptionalParams) == expectedArgCount {
					for i := expectedArgCount - 1; i >= 0; i-- {
						if constructorSig.OptionalParams[i] {
							minRequiredArgs--
						} else {
							break
						}
					}
				}

				if !skipArityCheck && actualArgCount < minRequiredArgs {
					c.addErrorWithCode(node, errors.TS2554, formatArityError(minRequiredArgs, expectedArgCount, actualArgCount))
				} else if !skipArityCheck && actualArgCount > expectedArgCount && expectedArgCount > 0 {
					// Only enforce max args if constructor has declared parameters
					// (allow extra args for constructors with no params - they may use 'arguments')
					c.addErrorWithCode(node, errors.TS2554, formatArityError(minRequiredArgs, expectedArgCount, actualArgCount))
				} else {
					c.checkFixedArgumentsWithSpread(node.Arguments, constructorSig.ParameterTypes, constructorSig.IsVariadic)
				}
			}
		}
	} else {
		// No constructor signature - just visit args
		for _, arg := range node.Arguments {
			c.visit(arg)
		}
	}

	// The result type is void (constructors don't return values in the normal sense)
	node.SetComputedType(types.Void)
}

func (c *Checker) checkCallExpression(node *parser.CallExpression) {
	c.markArgumentGenerators(node.Arguments)
	// --- UPDATED: Handle CallExpression with Overload Support ---
	// 1. Check the expression being called
	debugPrintf("// [Checker CallExpr] Checking call at line %d\n", node.Token.Line)

	// Special handling for Paserati.reflect<T>() intrinsic
	if c.isPaseratiReflectCall(node) {
		c.handlePaseratiReflect(node)
		return
	}

	// Special handling for super() calls
	if superExpr, isSuper := node.Function.(*parser.SuperExpression); isSuper {
		c.checkSuperCallExpression(node, superExpr)
		return
	}

	c.noteIndirectCallCallee(node.Function)
	c.visit(node.Function)
	funcNodeType := node.Function.GetComputedType()

	// The receiver a method call passes as `this`, for signatures that
	// declare an explicit `this` parameter.
	savedReceiver := c.callReceiver
	c.callReceiver = nil
	switch fn := node.Function.(type) {
	case *parser.MemberExpression:
		if fn.Object != nil {
			c.callReceiver = fn.Object.GetComputedType()
		}
	case *parser.ChainBase:
		if fn.Receiver != nil {
			c.callReceiver = fn.Receiver.GetComputedType()
		}
	}
	if c.callReceiver != nil {
		c.callReceiver = types.RemoveNullishTypes(c.callReceiver)
	}
	defer func() { c.callReceiver = savedReceiver }()
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

	if c.checkCallTypeArguments(node, funcNodeType) {
		// TypeScript resolves a call with a bad type-argument list as an
		// error call: arguments are still checked, but nothing is matched.
		for _, argNode := range node.Arguments {
			c.visit(argNode)
		}
		node.SetComputedType(types.Any)
		return
	}

	funcNodeType = c.stripNullishCallee(node, funcNodeType)
	c.markContextuallyTypedArguments(node.Arguments, funcNodeType)
	if c.noImplicitAny {
		c.markFunctionsUnder(node.Function, 5) // an immediately invoked function takes its parameter types from the arguments
	}

	if funcNodeType == types.Any {
		// Allow calling 'any', result is 'any'. Check args against 'any'.
		for _, argNode := range node.Arguments {
			c.visit(argNode) // Visit args even if function is 'any'
		}
		node.SetComputedType(types.Any)
		return
	}

	// Handle TypeParameterType by checking its constraint
	if typeParamType, ok := funcNodeType.(*types.TypeParameterType); ok {
		// Try to resolve the type parameter to its constraint or a concrete type
		if typeParamType.Parameter != nil && typeParamType.Parameter.Constraint != nil {
			constraintType := typeParamType.Parameter.Constraint
			if checkerDebug {
				debugPrintf("// [Checker CallExpr] Type parameter '%s' has constraint: %s\n",
					typeParamType.Parameter.Name, constraintType.String())
			}

			// If the constraint is callable, use it for the call
			if objType, ok := constraintType.(*types.ObjectType); ok && objType.IsCallable() {
				// Use the constraint type instead of the type parameter
				funcNodeType = constraintType
				if checkerDebug {
					debugPrintf("// [Checker CallExpr] Using constraint type for type parameter call: %s\n", constraintType.String())
				}
			} else {
				// If constraint is not callable, this is an error
				c.reportNotCallable(node, funcNodeType)
				node.SetComputedType(types.Any)
				return
			}
		} else {
			// Type parameter without constraint or with 'any' constraint - assume it could be callable
			// but we can't verify at compile time
			c.reportNotCallable(node, funcNodeType)
			node.SetComputedType(types.Any)
			return
		}
	}

	// Handle GenericType (for generic methods in interfaces)
	// When calling a generic method, extract the body (the callable function type)
	if genericType, ok := funcNodeType.(*types.GenericType); ok {
		// The body should be the callable function type
		if bodyObj, ok := genericType.Body.(*types.ObjectType); ok && bodyObj.IsCallable() {
			if checkerDebug {
				debugPrintf("// [Checker CallExpr] Calling generic method, using body type: %s\n", bodyObj.String())
			}
			funcNodeType = bodyObj
		} else {
			c.addError(node, fmt.Sprintf("cannot call generic type '%s' (body is not callable)", funcNodeType.String()))
			node.SetComputedType(types.Any)
			return
		}
	}

	// An intersection is callable with the call signatures of its members
	// (vitest's `test` is `ChainableTestAPI & ExtendedAPI & Hooks & {...}`).
	if inter, ok := funcNodeType.(*types.IntersectionType); ok && !c.hasConflictingDiscriminant(inter) {
		if sigs := c.intersectionCallSignatures(inter, 0); len(sigs) > 0 {
			callable := types.NewObjectType()
			callable.Materialize()
			callable.CallSignatures = sigs
			funcNodeType = callable
		}
	}

	// Handle union of callable types - e.g. string.includes | Uint8Array.includes
	if unionType, isUnion := funcNodeType.(*types.UnionType); isUnion {
		// Visit arguments
		for _, argNode := range node.Arguments {
			c.visit(argNode)
		}
		// Check that all members are callable and collect return types
		var returnTypes []types.Type
		allCallable := true
		for _, member := range unionType.Types {
			if objMember, ok := member.(*types.ObjectType); ok && objMember.IsCallable() {
				if len(objMember.CallSigs()) > 0 {
					returnTypes = append(returnTypes, objMember.CallSigs()[0].ReturnType)
				}
			} else {
				allCallable = false
				break
			}
		}
		if allCallable && len(returnTypes) > 0 {
			if len(returnTypes) == 1 {
				node.SetComputedType(returnTypes[0])
			} else {
				node.SetComputedType(types.NewUnionType(returnTypes...))
			}
			return
		}
		// Fall through to error if not all callable
		c.reportNotCallable(node, funcNodeType)
		for _, argNode := range node.Arguments {
			c.visit(argNode)
		}
		node.SetComputedType(types.Any)
		return
	}

	// Handle callable ObjectType with unified approach
	objType, ok := funcNodeType.(*types.ObjectType)
	if !ok {
		c.reportNotCallable(node, funcNodeType)
		for _, argNode := range node.Arguments {
			c.visit(argNode)
		}
		node.SetComputedType(types.Any)
		return
	}

	if !objType.IsCallable() {
		if c.isUntypedCallTarget(objType) {
			// tsc's untyped call: a Function value takes any arguments.
			for _, argNode := range node.Arguments {
				c.visit(argNode)
			}
			node.SetComputedType(types.Any)
			return
		}
		c.reportNotCallable(node, funcNodeType)
		for _, argNode := range node.Arguments {
			c.visit(argNode)
		}
		node.SetComputedType(types.Any)
		return
	}

	if len(objType.CallSigs()) == 0 {
		c.reportNotCallable(node, funcNodeType)
		node.SetComputedType(types.Any)
		return
	}

	// Handle overloaded functions (multiple call signatures)
	if len(objType.CallSigs()) > 1 {
		if checkerDebug {
			debugPrintf("// [Checker CallExpr] Processing overloaded function with %d signatures\n", len(objType.CallSigs()))
		}
		c.checkOverloadedCallUnified(node, objType)
		return
	}

	// Handle single call signature
	funcSignature := objType.CallSigs()[0]

	if checkerDebug {
		debugPrintf("// [Checker CallExpr] Processing function signature: %s, IsVariadic: %t\n", funcSignature.String(), funcSignature.IsVariadic)
	}

	// Check if this is a generic function call that needs type inference
	c.fixInScopeTypeParams = true
	isGeneric := c.isGenericSignature(funcSignature)
	c.fixInScopeTypeParams = false
	if checkerDebug {
		debugPrintf("// [Checker CallExpr] isGenericSignature returned: %t for signature: %s\n", isGeneric, funcSignature.String())
	}
	if isGeneric {
		debugPrintf("// [Checker CallExpr] Detected generic function call, attempting type inference\n")
		inferredSignature := c.inferGenericFunctionCall(node, funcSignature)
		argContexts := c.lastArgContexts
		c.lastArgContexts = nil
		if inferredSignature != nil {
			c.skipArgContexts = argContexts
			if checkerDebug {
				debugPrintf("// [Checker CallExpr] Type inference successful, using inferred signature: %s\n", inferredSignature.String())
			}
			funcSignature = inferredSignature
		} else {
			debugPrintf("// [Checker CallExpr] Type inference failed, using original signature with type parameters\n")
		}
	}

	// --- MODIFIED Arity and Argument Type Checking ---
	// First validate all spread arguments
	hasSpreadErrors := false
	currentArgIndex := 0
	for _, arg := range node.Arguments {
		if spreadElement, isSpread := arg.(*parser.SpreadElement); isSpread {
			if !c.validateSpreadArgument(spreadElement, funcSignature.IsVariadic, funcSignature.ParameterTypes, currentArgIndex) {
				hasSpreadErrors = true
			}
		}
		// For simplicity, assume each argument takes one slot for this validation pass
		currentArgIndex += 1
	}

	// If there are spread errors, skip detailed arity checking as it's meaningless
	if hasSpreadErrors {
		c.skipArgContexts = nil
		node.SetComputedType(funcSignature.ReturnType)
		return
	}

	// Calculate effective argument count, expanding spread elements
	actualArgCount := c.calculateEffectiveArgCount(node.Arguments)
	skipArityCheck := actualArgCount == -1 // -1 signals unknown length arrays in spreads
	if skipArityCheck {
		debugPrintf("// [Checker CallExpr] Skipping arity check due to unknown-length array spreads\n")
	} else {
		if checkerDebug {
			debugPrintf("// [Checker CallExpr] actualArgCount after spread expansion: %d (from %d raw arguments)\n", actualArgCount, len(node.Arguments))
		}
	}

	if funcSignature.IsVariadic {
		// --- Variadic Function Check ---
		debugPrintf("// [Checker CallExpr] Checking variadic function, RestParameterType: %T (%v)\n", funcSignature.RestParameterType, funcSignature.RestParameterType)
		if funcSignature.RestParameterType == nil {
			c.addError(node, "internal checker error: variadic function type must have a rest parameter type")
			node.SetComputedType(types.Any) // Error state
			return
		}

		// Calculate minimum required arguments (excluding optional parameters)
		minExpectedArgs := len(funcSignature.ParameterTypes)
		if len(funcSignature.OptionalParams) == len(funcSignature.ParameterTypes) {
			// Count required parameters from the end
			for i := len(funcSignature.ParameterTypes) - 1; i >= 0; i-- {
				if funcSignature.OptionalParams[i] {
					minExpectedArgs--
				} else {
					break // Stop at first required parameter from the end
				}
			}
		}

		if !skipArityCheck && actualArgCount < minExpectedArgs {
			c.addErrorWithCode(node, errors.TS2555, fmt.Sprintf("Expected at least %d arguments, but got %d.", minExpectedArgs, actualArgCount))
			// Don't check args if minimum count isn't met.
		} else {
			// Check fixed arguments with spread expansion support
			// tsc reports only the first argument that does not fit.
			argErr := !c.checkFixedArgumentsWithSpread(node.Arguments, funcSignature.ParameterTypes, funcSignature.IsVariadic, funcSignature.OptionalParams)

			// Check variadic arguments — always, even when the fixed-args
			// check reported errors. Variadic args own their own visit (since
			// checkFixedArgumentsWithSpread skips visiting beyond fixed params
			// for variadic functions to avoid double-emission); if we skipped
			// this branch on fixed-args failure, errors in variadic args would
			// be silently dropped.
			{
				variadicParamType := funcSignature.RestParameterType
				arrayType, isArray := variadicParamType.(*types.ArrayType)
				if !isArray {
					c.visitRemainingArguments(node.Arguments, len(funcSignature.ParameterTypes))
				} else {
					variadicElementType := arrayType.ElementType
					if variadicElementType == nil { // Should not happen with valid types
						variadicElementType = types.Any
					}
					// Check remaining arguments against the element type - start after all fixed parameters
					for i := len(funcSignature.ParameterTypes); i < len(node.Arguments); i++ {
						argNode := node.Arguments[i]

						// --- Handle spread elements in variadic functions ---
						if spreadElement, isSpread := argNode.(*parser.SpreadElement); isSpread {
							// Validate the spread argument for variadic functions
							// For variadic functions, pass empty slice since we don't use contextual typing
							if !c.validateSpreadArgument(spreadElement, true, []types.Type{}, 0) {
								continue // Error already reported
							}

							c.visit(spreadElement.Argument)
							argType := spreadElement.Argument.GetComputedType()
							if argType == nil {
								continue
							}

							if c.isSpreadableIterableType(argType) {
								spreadElementType := c.getSpreadElementType(argType)
								if !argErr && !c.rel.IsAssignable(spreadElementType, variadicElementType) {
									argErr = true
									c.addErrorWithCode(spreadElement, errors.TS2345, fmt.Sprintf("Argument of type '%s' is not assignable to parameter of type '%s'.", spreadElementType.String(), variadicElementType.String()))
								}
								continue
							}
							if !argErr && !c.rel.IsAssignable(argType, variadicParamType) {
								argErr = true
								c.addErrorWithCode(spreadElement, errors.TS2345, fmt.Sprintf("Argument of type '%s' is not assignable to parameter of type '%s'.", argType.String(), variadicParamType.String()))
							}
						} else {
							// Regular arguments in variadic part
							// Use contextual typing with the variadic element type
							c.visitWithContext(argNode, &ContextualType{
								ExpectedType: variadicElementType,
								IsContextual: true,
							})

							argType := argNode.GetComputedType()
							if argType == nil {
								continue
							}

							if !argErr && !c.rel.IsAssignable(argType, variadicElementType) {
								argErr = true
								c.addErrorWithCode(argNode, errors.TS2345, fmt.Sprintf("Argument of type '%s' is not assignable to parameter of type '%s'.", argType.String(), variadicElementType.String()))
							}
						}
					}
				}
			}
		}
	} else {
		// --- Non-Variadic Function Check (Updated for Optional Parameters) ---
		expectedArgCount := len(funcSignature.ParameterTypes)

		// Calculate minimum required arguments (non-optional parameters)
		minRequiredArgs := expectedArgCount
		if len(funcSignature.OptionalParams) == expectedArgCount {
			// Count required parameters from the end
			for i := expectedArgCount - 1; i >= 0; i-- {
				if funcSignature.OptionalParams[i] {
					minRequiredArgs--
				} else {
					break // Stop at first required parameter from the end
				}
			}
		}

		if params := iifeParameters(node.Function); params != nil && len(params) == expectedArgCount {
			// An immediately invoked function's untyped parameters past the
			// supplied arguments are optional (tsc's isOptionalParameter).
			required := 0
			for i, p := range params {
				optional := i < len(funcSignature.OptionalParams) && funcSignature.OptionalParams[i]
				if !optional && !(p.TypeAnnotation == nil && i >= actualArgCount) {
					required = i + 1
				}
			}
			minRequiredArgs = required
		}
		if !skipArityCheck && actualArgCount < minRequiredArgs {
			c.addErrorWithCode(node, errors.TS2554, formatArityError(minRequiredArgs, expectedArgCount, actualArgCount))
			// Continue checking assignable args anyway? Let's stop if arity wrong.
		} else if !skipArityCheck && actualArgCount > expectedArgCount {
			// Too many arguments for a non-variadic signature. Mirrors the
			// too-many check already done for super() and `new` calls above.
			c.addErrorWithCode(node, errors.TS2554, formatArityError(minRequiredArgs, expectedArgCount, actualArgCount))
		} else {
			// Check argument types with spread expansion support
			c.checkFixedArgumentsWithSpread(node.Arguments, funcSignature.ParameterTypes, funcSignature.IsVariadic, funcSignature.OptionalParams)
		}
	}
	// --- END MODIFIED Checking ---
	c.skipArgContexts = nil

	// Set Result Type - check if we need to resolve parameterized forward references
	resultType := funcSignature.ReturnType

	// If the result type is a ParameterizedForwardReferenceType with concrete type arguments,
	// we need to instantiate the generic type to get a proper object type
	if paramRef, ok := resultType.(*types.ParameterizedForwardReferenceType); ok {
		if checkerDebug {
			debugPrintf("// [Checker CallExpr] Result type is parameterized forward reference: %s\n", paramRef.String())
		}

		// Try to resolve the generic class and instantiate it
		if resolvedType := c.resolveParameterizedForwardReference(paramRef); resolvedType != nil {
			if checkerDebug {
				debugPrintf("// [Checker CallExpr] Resolved parameterized type to: %T (%s)\n", resolvedType, resolvedType.String())
			}
			resultType = resolvedType
		}
	}

	resultType = c.narrowFilterResult(node, resultType)

	if checkerDebug {
		debugPrintf("// [Checker CallExpr] Setting result type from func '%s'. ReturnType from Sig: %T (%v)\n", node.Function.String(), resultType, resultType)
	}
	node.SetComputedType(resultType)

}

func (c *Checker) callTargetAcceptsTypeArguments(funcNodeType types.Type) bool {
	switch typ := funcNodeType.(type) {
	case *types.GenericType:
		return true
	case *types.ObjectType:
		for _, sig := range typ.CallSigs() {
			if c.isGenericSignature(sig) {
				return true
			}
		}
		return false
	case *types.TypeParameterType:
		if typ.Parameter != nil && typ.Parameter.Constraint != nil {
			return c.callTargetAcceptsTypeArguments(typ.Parameter.Constraint)
		}
		return false
	default:
		return false
	}
}

// checkOverloadedCallUnified handles function calls to overloaded functions using unified ObjectType
func (c *Checker) checkOverloadedCallUnified(node *parser.CallExpression, objType *types.ObjectType) {
	// Visit all arguments first
	var argTypes []types.Type
	for _, argNode := range node.Arguments {
		c.visit(argNode)
		argType := argNode.GetComputedType()
		if argType == nil {
			argType = types.Any
		}
		// An empty array literal has no context here to give it an element
		// type; like tsc's `never[]` it fits any array parameter.
		if lit, ok := argNode.(*parser.ArrayLiteral); ok && len(lit.Elements) == 0 {
			if arr, ok := argType.(*types.ArrayType); ok && arr.ElementType == types.Unknown {
				argType = &types.ArrayType{ElementType: types.Never}
			}
		}
		argTypes = append(argTypes, argType)
	}

	// Explicit type arguments select the overloads that take that many and
	// instantiate them, as tsc does; the others are not candidates.
	if len(node.TypeArguments) > 0 {
		var candidates []*types.Signature
		for _, sig := range objType.CallSigs() {
			min, max, known := c.signatureTypeArgumentRange(sig)
			if !known || len(node.TypeArguments) < min || len(node.TypeArguments) > max {
				continue
			}
			typeParams := c.extractTypeParametersFromSignature(sig)
			solution := make(map[*types.TypeParameter]types.Type, len(node.TypeArguments))
			for i, typeArgExpr := range node.TypeArguments {
				typeArg := c.resolveTypeAnnotation(typeArgExpr)
				if typeArg == nil {
					typeArg = types.Any
				}
				if i < len(typeParams) {
					solution[typeParams[i]] = typeArg
				}
			}
			if len(objType.CallSigs()) == 1 {
				c.checkExplicitTypeArgConstraints(typeParams, node.TypeArguments, solution)
			}
			candidates = append(candidates, c.substituteTypeParameters(sig, solution))
		}
		if len(candidates) > 0 {
			objType = types.NewObjectType()
			objType.Materialize()
			objType.CallSignatures = candidates
		}
	}

	// Generic overloads called without explicit type arguments are matched
	// through the signature their type parameters infer from the arguments.
	if len(node.TypeArguments) == 0 {
		var inferred []*types.Signature
		for i, sig := range objType.CallSigs() {
			if !c.isGenericSignature(sig) {
				continue
			}
			constraints := c.collectTypeParameterConstraints(sig, argTypes)
			solution := c.solveTypeParameterConstraints(constraints)
			if len(solution) == 0 {
				continue
			}
			if inferred == nil {
				inferred = append([]*types.Signature(nil), objType.CallSigs()...)
			}
			inferred[i] = c.substituteTypeParameters(sig, solution)
		}
		if inferred != nil {
			objType = types.NewObjectType()
			objType.Materialize()
			objType.CallSignatures = inferred
		}
	}

	// Try to find the best matching signature
	signatureIndex := -1
	var resultType types.Type

	// chooseOverload: with several candidates a first pass requires arguments
	// to be subtypes of the parameters (an `any` argument only fits an `any`
	// or `unknown` parameter); only if that finds nothing are plain
	// assignability and the first fitting candidate used.
	passes := 1
	if len(objType.CallSigs()) > 1 {
		passes = 2
	}
	for pass := 0; pass < passes && signatureIndex == -1; pass++ {
		c.overloadStrictAny = passes == 2 && pass == 0
		for i, signature := range objType.CallSigs() {
			// Check if this signature can accept the given arguments
			var isMatching bool

			if signature.IsVariadic {
				// For variadic signatures, check minimum required arguments (fixed parameters)
				minRequiredArgs := len(signature.ParameterTypes)
				if len(argTypes) >= minRequiredArgs {
					// Check fixed parameters first
					fixedMatch := true
					for j := 0; j < minRequiredArgs; j++ {
						if !c.overloadArgMatches(node, j, argTypes[j], signature.ParameterTypes[j]) {
							fixedMatch = false
							break
						}
					}

					if fixedMatch {
						// Check remaining arguments against rest parameter type
						if signature.RestParameterType != nil {
							// Extract element type from rest parameter array type
							var elementType types.Type = types.Any
							if arrayType, ok := signature.RestParameterType.(*types.ArrayType); ok {
								elementType = arrayType.ElementType
							}

							// Check all remaining arguments against element type
							variadicMatch := true
							for j := minRequiredArgs; j < len(argTypes); j++ {
								if !c.overloadArgMatches(node, j, argTypes[j], elementType) {
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
				minRequiredArgs := requiredParameterCount(signature)
				if len(argTypes) < minRequiredArgs || len(argTypes) > len(signature.ParameterTypes) {
					continue // Argument count mismatch
				}

				// Check if all argument types are assignable to parameter types
				allMatch := true
				for j, argType := range argTypes {
					paramType := signature.ParameterTypes[j]
					if !c.overloadArgMatches(node, j, argType, paramType) {
						allMatch = false
						break
					}
				}
				isMatching = allMatch
			}

			if isMatching {
				signatureIndex = i
				resultType = signature.ReturnType
				break // Found the first matching signature
			}
		}
	}
	c.overloadStrictAny = false

	if signatureIndex == -1 {
		// No matching signature found
		c.reportOverloadFailure(node, argTypes, objType.CallSigs())

		node.SetComputedType(types.Any)
		return
	}

	// Found a matching signature
	matchedSignature := objType.CallSigs()[signatureIndex]
	if checkerDebug {
		debugPrintf("// [Checker OverloadCall] Found matching signature %d: %s for call with args (%v)\n",
			signatureIndex, matchedSignature.String(), argTypes)
	}

	// Set the result type from the matched signature
	node.SetComputedType(resultType)
	if checkerDebug {
		debugPrintf("// [Checker OverloadCall] Set result type to: %s\n", resultType.String())
	}
}

// isGenericSignature checks if a function signature contains unresolved type parameters
func (c *Checker) isGenericSignature(sig *types.Signature) bool {
	if len(sig.TypeParameters) > 0 {
		return true
	}

	// Track visited types to prevent infinite recursion in self-referencing types
	visited := make(map[types.Type]bool)

	// Helper function to check if a type contains type parameters
	var containsTypeParameters func(t types.Type) bool
	containsTypeParameters = func(t types.Type) bool {
		if t == nil {
			return false
		}
		// Check if we've already visited this type (cycle detection)
		if visited[t] {
			return false
		}
		visited[t] = true

		switch typ := t.(type) {
		case *types.TypeParameterType:
			// The enclosing declaration's own type parameter is fixed at a
			// call, not inferred from its arguments.
			if c.fixInScopeTypeParams && typ.Parameter != nil {
				if inScope, ok := c.env.ResolveTypeParameter(typ.Parameter.Name); ok && inScope == typ.Parameter {
					return false
				}
			}
			return true
		case *types.ArrayType:
			return containsTypeParameters(typ.ElementType)
		case *types.UnionType:
			for _, memberType := range typ.Types {
				if containsTypeParameters(memberType) {
					return true
				}
			}
			return false
		case *types.ObjectType:
			if typ.ClosedInstance {
				for _, a := range typ.GenericArgs {
					if containsTypeParameters(a) {
						return true
					}
				}
				return false
			}
			for _, __name := range types.SortedPropertyNames(typ.Props()) {
				propType := typ.Props()[__name]
				if containsTypeParameters(propType) {
					return true
				}
			}
			// Check call and construct signatures for type parameters
			for _, sig := range append(append([]*types.Signature(nil), typ.CallSigs()...), typ.ConstructSigs()...) {
				for _, paramType := range sig.ParameterTypes {
					if containsTypeParameters(paramType) {
						return true
					}
				}
				if sig.ReturnType != nil && containsTypeParameters(sig.ReturnType) {
					return true
				}
				if sig.RestParameterType != nil && containsTypeParameters(sig.RestParameterType) {
					return true
				}
			}
			return false
		case *types.ParameterizedForwardReferenceType:
			// Check type arguments for type parameters
			for _, typeArg := range typ.TypeArguments {
				if containsTypeParameters(typeArg) {
					return true
				}
			}
			return false
		// Add more type cases as needed
		default:
			return false
		}
	}

	// Check parameter types
	for _, paramType := range sig.ParameterTypes {
		if containsTypeParameters(paramType) {
			return true
		}
	}

	// Check return type
	if sig.ReturnType != nil && containsTypeParameters(sig.ReturnType) {
		return true
	}

	// Check rest parameter type
	if sig.RestParameterType != nil && containsTypeParameters(sig.RestParameterType) {
		return true
	}

	return false
}

// isLikelyFunctionArgument checks if an argument node is likely a function (for two-phase inference)
func (c *Checker) isLikelyFunctionArgument(argNode parser.Expression) bool {
	switch argNode.(type) {
	case *parser.FunctionLiteral, *parser.ArrowFunctionLiteral:
		return true
	default:
		return false
	}
}

// collectTypeParameterConstraintsPhase1 collects constraints only from non-nil argument types
func (c *Checker) collectTypeParameterConstraintsPhase1(sig *types.Signature, argTypes []types.Type) []TypeParameterConstraint {
	var constraints []TypeParameterConstraint
	if sig.ThisType != nil && c.callReceiver != nil {
		constraints = append(constraints, c.collectConstraintsFromType(sig.ThisType, c.callReceiver)...)
	}

	// For each parameter, if it contains type parameters, create constraints based on the argument type
	for i, paramType := range sig.ParameterTypes {
		if i >= len(argTypes) || argTypes[i] == nil {
			continue // Skip nil placeholders from phase 1
		}
		argType := argTypes[i]

		// Collect constraints from this parameter-argument pair
		savedTop, savedSig := c.inferTopParam, c.inferSig
		c.inferTopParam, c.inferSig = paramType, sig
		paramConstraints := c.collectConstraintsFromType(paramType, argType)
		c.inferTopParam, c.inferSig = savedTop, savedSig
		constraints = append(constraints, paramConstraints...)
	}

	if sig.RestParameterType != nil {
		for i := len(sig.ParameterTypes); i < len(argTypes); i++ {
			if argTypes[i] == nil {
				continue
			}
			constraints = append(constraints, c.collectConstraintsFromType(sig.RestParameterType, argTypes[i])...)
		}
	}

	return constraints
}

// inferGenericFunctionCall attempts to infer type arguments for a generic function call
func (c *Checker) inferGenericFunctionCall(callNode *parser.CallExpression, genericSig *types.Signature) *types.Signature {
	debugPrintf("// [Checker Inference] Starting type inference for generic function call\n")
	argContexts := make(map[parser.Expression]types.Type, len(callNode.Arguments))
	defer func() { c.lastArgContexts = argContexts }()

	// === PHASE 0: Check for explicit type arguments ===
	if len(callNode.TypeArguments) > 0 {
		if checkerDebug {
			debugPrintf("// [Checker Inference] Found %d explicit type arguments\n", len(callNode.TypeArguments))
		}

		// Extract type parameters from the signature
		typeParams := c.extractTypeParametersFromSignature(genericSig)

		minTypeArgs := len(typeParams)
		for minTypeArgs > 0 && typeParams[minTypeArgs-1].Default != nil {
			minTypeArgs--
		}
		if n := len(callNode.TypeArguments); n < minTypeArgs || n > len(typeParams) {
			expected := fmt.Sprintf("%d", minTypeArgs)
			if minTypeArgs < len(typeParams) {
				expected = fmt.Sprintf("%d-%d", minTypeArgs, len(typeParams))
			}
			c.addErrorWithCode(callNode.TypeArguments[0], errors.TS2558, fmt.Sprintf("Expected %s type arguments, but got %d.", expected, n))
			return nil
		}

		// Build solution from explicit type arguments
		solution := make(map[*types.TypeParameter]types.Type)
		for i, typeArgExpr := range callNode.TypeArguments {
			typeArg := c.resolveTypeAnnotation(typeArgExpr)
			if typeArg == nil {
				typeArg = types.Any
			}
			if i < len(typeParams) {
				solution[typeParams[i]] = typeArg
				if checkerDebug {
					debugPrintf("// [Checker Inference] Explicit type argument: %s = %s\n", typeParams[i].Name, typeArg.String())
				}
			}
		}

		c.checkExplicitTypeArgConstraints(typeParams, callNode.TypeArguments, solution)

		// Substitute type parameters to create concrete signature
		inferredSig := c.substituteTypeParameters(genericSig, solution)
		if checkerDebug {
			debugPrintf("// [Checker Inference] Created signature from explicit type args: %s\n", inferredSig.String())
		}

		// Now visit arguments with contextual typing from the concrete signature
		for i, argNode := range callNode.Arguments {
			if i < len(inferredSig.ParameterTypes) {
				paramType := inferredSig.ParameterTypes[i]
				argContexts[argNode] = paramType
				c.visitWithContext(argNode, &ContextualType{
					ExpectedType: paramType,
					IsContextual: true,
				})
			} else {
				argContexts[argNode] = nil
				c.visit(argNode)
			}
		}

		return inferredSig
	}

	// === PHASE 1: Infer type parameters from non-function arguments ===
	var argTypes []types.Type
	for i, argNode := range callNode.Arguments {
		// Skip function arguments in phase 1
		if c.isLikelyFunctionArgument(argNode) {
			argTypes = append(argTypes, nil) // Placeholder
			debugPrintf("// [Checker Inference] Phase 1: Skipping function argument %d\n", i)
			continue
		}

		// Visit non-function arguments without contextual typing to get their natural types
		argContexts[argNode] = nil
		c.visit(argNode)
		argType := argNode.GetComputedType()
		if argType == nil {
			argType = types.Any
		}
		if spreadElement, isSpread := argNode.(*parser.SpreadElement); isSpread && c.isSpreadableIterableType(argType) {
			argType = &types.ArrayType{ElementType: c.getSpreadElementType(argType)}
			spreadElement.SetComputedType(argType)
		}
		argTypes = append(argTypes, argType)
		if checkerDebug {
			debugPrintf("// [Checker Inference] Phase 1: Argument %d type: %s\n", i, argType.String())
		}
	}

	// Collect constraints from non-function arguments only
	constraints := c.collectTypeParameterConstraintsPhase1(genericSig, argTypes)
	if checkerDebug {
		debugPrintf("// [Checker Inference] Phase 1: Collected %d type parameter constraints\n", len(constraints))
	}

	// Solve constraints from phase 1
	partialSolution := c.solveTypeParameterConstraints(constraints)
	if checkerDebug {
		debugPrintf("// [Checker Inference] Phase 1: Solved %d type parameter bindings\n", len(partialSolution))
	}

	// === PHASE 2: Re-visit function arguments with inferred types ===
	if len(partialSolution) > 0 {
		// Create partially instantiated signature for contextual typing
		// Only the parameter types give the remaining arguments context;
		// substituting the return type too (a whole router type) is waste.
		paramsOnly := *genericSig
		paramsOnly.ReturnType = nil
		partialSig := c.substituteTypeParameters(&paramsOnly, partialSolution)
		if checkerDebug {
			debugPrintf("// [Checker Inference] Phase 2: Using partially inferred signature: %s\n", partialSig.String())
		}

		// Re-visit function arguments with better contextual typing
		for i, argNode := range callNode.Arguments {
			if argTypes[i] == nil { // This was a function argument skipped in phase 1
				if i < len(partialSig.ParameterTypes) {
					paramType := partialSig.ParameterTypes[i]
					if checkerDebug {
						debugPrintf("// [Checker Inference] Phase 2: Re-visiting function argument %d with context: %s\n", i, paramType.String())
					}
					argContexts[argNode] = paramType
					c.visitWithContext(argNode, &ContextualType{
						ExpectedType: paramType,
						IsContextual: true,
					})
				} else {
					argContexts[argNode] = nil
					c.visit(argNode)
				}

				argType := argNode.GetComputedType()
				if argType == nil {
					argType = types.Any
				}
				argTypes[i] = argType
				if checkerDebug {
					debugPrintf("// [Checker Inference] Phase 2: Function argument %d type: %s\n", i, argType.String())
				}
			}
		}

		// Collect additional constraints from function arguments if needed
		additionalConstraints := c.collectTypeParameterConstraints(partialSig, argTypes)
		allConstraints := append(constraints, additionalConstraints...)

		// Solve all constraints together. Inferences from the function
		// arguments (context sensitive, checked with the phase 1 solution as
		// context) do not override what the other arguments fixed.
		finalSolution := c.solveTypeParameterConstraints(allConstraints)
		for tp, t := range partialSolution {
			finalSolution[tp] = t
		}
		if checkerDebug {
			debugPrintf("// [Checker Inference] Phase 2: Final solution with %d type parameter bindings\n", len(finalSolution))
		}

		c.applyTypeParameterDefaults(genericSig, finalSolution)
		if len(finalSolution) > 0 {
			c.fallBackToConstraints(finalSolution)
			inferredSig := c.substituteTypeParameters(genericSig, finalSolution)
			if checkerDebug {
				debugPrintf("// [Checker Inference] Created final inferred signature: %s\n", inferredSig.String())
			}
			return inferredSig
		}
	}

	// Fallback: If phase 1 didn't infer anything, try the original approach.
	// A function argument's parameters fix the type parameters they mention:
	// with no inference yet, a constrained one is its constraint.
	ctxParams := genericSig.ParameterTypes
	if fixedToConstraints := constrainedTypeParams(genericSig.TypeParameters); len(fixedToConstraints) > 0 {
		paramsOnly := *genericSig
		paramsOnly.ReturnType = nil
		ctxParams = c.substituteTypeParameters(&paramsOnly, fixedToConstraints).ParameterTypes
	}
	for i, argNode := range callNode.Arguments {
		if argTypes[i] == nil {
			if i < len(genericSig.ParameterTypes) {
				paramType := genericSig.ParameterTypes[i]
				if _, isFn := argNode.(*parser.ArrowFunctionLiteral); isFn && i < len(ctxParams) {
					paramType = ctxParams[i]
				}
				argContexts[argNode] = paramType
				c.visitWithContext(argNode, &ContextualType{
					ExpectedType: paramType,
					IsContextual: true,
				})
			} else {
				argContexts[argNode] = nil
				c.visit(argNode)
			}

			argType := argNode.GetComputedType()
			if argType == nil {
				argType = types.Any
			}
			argTypes[i] = argType
		}
	}

	// Final attempt with all arguments
	allConstraints := c.collectTypeParameterConstraints(genericSig, argTypes)
	solution := c.solveTypeParameterConstraints(allConstraints)

	// Type parameters no argument determines are inferred from the type the
	// call's result is expected to have, at lower priority than arguments.
	if expected := c.returnContexts[callNode]; expected != nil && expected != types.Any && genericSig.ReturnType != nil {
		retSig := &types.Signature{ParameterTypes: []types.Type{genericSig.ReturnType}}
		retConstraints := c.collectTypeParameterConstraints(retSig, []types.Type{stripReadonlyArrays(expected)})
		for tp, t := range c.solveTypeParameterConstraints(retConstraints) {
			if _, have := solution[tp]; !have {
				if solution == nil {
					solution = map[*types.TypeParameter]types.Type{}
				}
				solution[tp] = t
			}
		}
	}

	if solution == nil {
		solution = map[*types.TypeParameter]types.Type{}
	}
	c.applyTypeParameterDefaults(genericSig, solution)
	if len(solution) == 0 {
		debugPrintf("// [Checker Inference] No type parameters could be inferred\n")
		return nil // Inference failed
	}

	c.fallBackToConstraints(solution)
	inferredSig := c.substituteTypeParameters(genericSig, solution)
	if checkerDebug {
		debugPrintf("// [Checker Inference] Created fallback inferred signature: %s\n", inferredSig.String())
	}

	return inferredSig
}

// applyTypeParameterDefaults gives the type parameters of sig that nothing
// inferred their defaults (`konn<C = NoContext>()`), as tsc does.
func (c *Checker) applyTypeParameterDefaults(sig *types.Signature, solution map[*types.TypeParameter]types.Type) {
	// Only the declared parameters: walking the signature for others would
	// expand self-growing generics (Grow<T>'s next(): Grow<Wrap<T>>) forever.
	for _, tp := range sig.TypeParameters {
		if _, inferred := solution[tp]; !inferred && tp.Default != nil && !c.typeContainsTypeParameter(tp.Default) {
			solution[tp] = tp.Default
		}
	}
}

// TypeParameterConstraint represents a constraint on a type parameter
type TypeParameterConstraint struct {
	TypeParameter *types.TypeParameter
	InferredType  types.Type
	Confidence    int // Higher = more confident
	Combine       bool
}

type constraintTypePair struct {
	param types.Type
	arg   types.Type
}

// collectTypeParameterConstraints analyzes arguments to build constraints for type parameters
func (c *Checker) collectTypeParameterConstraints(sig *types.Signature, argTypes []types.Type) []TypeParameterConstraint {
	var constraints []TypeParameterConstraint
	if sig.ThisType != nil && c.callReceiver != nil {
		constraints = append(constraints, c.collectConstraintsFromType(sig.ThisType, c.callReceiver)...)
	}

	// For each parameter, if it contains type parameters, create constraints based on the argument type
	for i, paramType := range sig.ParameterTypes {
		if i >= len(argTypes) {
			break // No more arguments
		}
		argType := argTypes[i]

		// Collect constraints from this parameter-argument pair
		savedTop, savedSig := c.inferTopParam, c.inferSig
		c.inferTopParam, c.inferSig = paramType, sig
		paramConstraints := c.collectConstraintsFromType(paramType, argType)
		c.inferTopParam, c.inferSig = savedTop, savedSig
		constraints = append(constraints, paramConstraints...)
	}

	if sig.RestParameterType != nil {
		for i := len(sig.ParameterTypes); i < len(argTypes); i++ {
			if i >= len(argTypes) || argTypes[i] == nil {
				continue
			}
			constraints = append(constraints, c.collectConstraintsFromType(sig.RestParameterType, argTypes[i])...)
		}
	}

	return constraints
}

// isNumericValue checks if a vm.Value represents a numeric type
func isNumericValue(value vm.Value) bool {
	valueType := value.Type()
	switch valueType {
	case vm.TypeFloatNumber, vm.TypeIntegerNumber, vm.TypeBigInt:
		return true
	default:
		return false
	}
}

// shouldWidenForAccumulator determines if a type parameter should be widened for accumulator patterns
func shouldWidenForAccumulator(typeParamName string, argType types.Type) bool {
	// Common accumulator type parameter names that should be widened
	accumulatorNames := []string{"TResult", "TAcc", "TAccumulator", "TReduce"}

	for _, name := range accumulatorNames {
		if typeParamName == name {
			// Only widen if the argument type has literal types that can be widened
			switch objType := argType.(type) {
			case *types.ObjectType:
				// Check if any properties have literal types that would benefit from widening
				for _, __name := range types.SortedPropertyNames(objType.Props()) {
					propType := objType.Props()[__name]
					if literalType, isLiteral := propType.(*types.LiteralType); isLiteral {
						// Check if it's a numeric literal using vm.Value type
						if isNumericValue(literalType.Value) {
							return true
						}
					}
				}
			case *types.LiteralType:
				// Direct literal type
				if isNumericValue(argType.(*types.LiteralType).Value) {
					return true
				}
			}
		}
	}
	return false
}

// collectConstraintsFromType recursively collects constraints by matching parameter and argument types
func (c *Checker) collectConstraintsFromType(paramType, argType types.Type) []TypeParameterConstraint {
	return c.collectConstraintsFromTypeSeen(paramType, argType, make(map[constraintTypePair]bool))
}

// maxInferNesting is how deep one generic's instantiations may nest on each
// side of collectConstraintsFromTypeSeen; maxInferWalkDepth bounds the walk.
const (
	maxInferNesting   = 3
	maxInferWalkDepth = 100
)

func (c *Checker) collectConstraintsFromTypeSeen(paramType, argType types.Type, seen map[constraintTypePair]bool) []TypeParameterConstraint {
	var constraints []TypeParameterConstraint
	if paramType == nil || argType == nil {
		return constraints
	}
	pair := constraintTypePair{param: paramType, arg: argType}
	if seen[pair] {
		return constraints
	}
	seen[pair] = true
	// Recursive generics lead the walk to ever deeper instantiations
	// (Grow<A>'s next(): Grow<Wrap<A>> against an argument's). Like tsc's
	// isDeeplyNestedType, stop once one generic's instantiations nest several
	// levels deep on both sides, and in any case past a depth no real type
	// reaches.
	if c.inferNesting == nil {
		c.inferNesting = map[*types.GenericType]int{}
	}
	if c.inferWalkDepth >= maxInferWalkDepth {
		return constraints
	}
	c.inferWalkDepth++
	defer func() { c.inferWalkDepth-- }()
	pObj, _ := paramType.(*types.ObjectType)
	aObj, _ := argType.(*types.ObjectType)
	if pObj != nil && aObj != nil && pObj.Instantiates != nil && aObj.Instantiates != nil {
		pg, ag := pObj.Instantiates, aObj.Instantiates
		if c.inferNesting[pg] >= maxInferNesting && c.inferNesting[ag] >= maxInferNesting {
			return constraints
		}
		c.inferNesting[pg]++
		c.inferNesting[ag]++
		defer func() {
			c.inferNesting[pg]--
			c.inferNesting[ag]--
		}()
	}

	// Readonly arrays and tuples infer through their element structure:
	// `[A[], B[]]` against `readonly [T1[], T2[]]`.
	if ro, ok := paramType.(*types.ReadonlyType); ok && types.IsReadonlyArrayLike(ro) {
		return c.collectConstraintsFromTypeSeen(ro.InnerType, stripReadonlyArrays(argType), seen)
	}
	if _, isTP := paramType.(*types.TypeParameterType); !isTP && types.IsReadonlyArrayLike(argType) {
		return c.collectConstraintsFromTypeSeen(paramType, stripReadonlyArrays(argType), seen)
	}

	switch pType := paramType.(type) {
	case *types.TypeParameterType:
		// Direct constraint: T should be inferred as argType
		// TypeScript widens literal types when inferring type arguments by default
		// e.g., createSignal(0) should infer T = number, not T = 0
		// Use DeeplyWidenType to also widen object literal properties
		// e.g., { count: 0 } should infer T = { count: number }, not T = { count: 0 }
		inferredType := c.rel.DeeplyWidenType(argType)
		keepsLiteral := false
		_, isLit := argType.(*types.LiteralType)
		if isLit && hasPrimitiveConstraint(pType.Parameter.Constraint) {
			// `T extends string` / `K extends keyof O` keep the literal
			// ("a"), as tsc does for a primitive-constrained parameter.
			inferredType = argType
			keepsLiteral = true
		} else if isLit && c.inferSig != nil && c.keepsLiteralCandidate(pType.Parameter, paramType) {
			inferredType = argType
			keepsLiteral = true
			if paramType == c.inferTopParam && !c.isWideningLiteral(argType) {
				lit := argType.(*types.LiteralType)
				w := &types.LiteralType{Value: lit.Value}
				if c.wideningLiterals == nil {
					c.wideningLiterals = map[*types.LiteralType]bool{}
				}
				c.wideningLiterals[w] = true
				inferredType = w
			}
		}
		if argTP, argIsTP := argType.(*types.TypeParameterType); argIsTP && argTP.Parameter != nil &&
			argTP.Parameter.Constraint != nil && argTP.Parameter.Constraint != types.Any {
			// A constrained type parameter infers as itself, not as its
			// constraint (`T extends {..}`, `list.slice()` is `T[]`).
			inferredType = argType
		}
		if checkerDebug {
			debugPrintf("// [Checker Constraints] Widening type for %s: %s -> %s\n",
				pType.Parameter.Name, argType.String(), inferredType.String())
		}

		constraints = append(constraints, TypeParameterConstraint{
			TypeParameter: pType.Parameter,
			InferredType:  inferredType,
			Confidence:    100, // High confidence for direct matches
			Combine:       keepsLiteral,
		})
		if checkerDebug {
			debugPrintf("// [Checker Constraints] Direct constraint: %s = %s\n", pType.Parameter.Name, inferredType.String())
		}

	case *types.ArrayType:
		// Array<T> matched against Array<U> or U[]
		if aType, isArray := argType.(*types.ArrayType); isArray && aType.ElementType != types.Unknown {
			// Recurse into element types (an empty literal `[]` carries no
			// candidates: it is never[] and so never constrains T)
			elemConstraints := c.collectConstraintsFromTypeSeen(pType.ElementType, aType.ElementType, seen)
			constraints = append(constraints, elemConstraints...)
		}

	case *types.TupleType:
		// Tuple [A, B, C] matched against tuple [X, Y, Z]
		if aTuple, isTuple := argType.(*types.TupleType); isTuple {
			// Match element-by-element
			minLen := len(pType.ElementTypes)
			if len(aTuple.ElementTypes) < minLen {
				minLen = len(aTuple.ElementTypes)
			}
			for i := 0; i < minLen; i++ {
				elemConstraints := c.collectConstraintsFromTypeSeen(pType.ElementTypes[i], aTuple.ElementTypes[i], seen)
				constraints = append(constraints, elemConstraints...)
			}
			// Handle rest element type if present
			if pType.RestElementType != nil && aTuple.RestElementType != nil {
				restConstraints := c.collectConstraintsFromTypeSeen(pType.RestElementType, aTuple.RestElementType, seen)
				constraints = append(constraints, restConstraints...)
			}
		}

	case *types.ObjectType:
		// Two instantiations of one closed generic infer from their type
		// arguments, as tsc does for two references to one type: their
		// members are the generic's with those arguments, and walking them
		// follows `next(): Grow<Wrap<T>>` to ever deeper instantiations.
		if aType, isObject := argType.(*types.ObjectType); isObject && pType.ClosedInstance && aType.ClosedInstance &&
			pType.Instantiates == aType.Instantiates && len(pType.GenericArgs) == len(aType.GenericArgs) {
			for i, pa := range pType.GenericArgs {
				constraints = append(constraints, c.collectConstraintsFromTypeSeen(pa, aType.GenericArgs[i], seen)...)
			}
			return constraints
		}
		// Handle function types: (T) => U matched against (A) => B
		if aType, isObject := argType.(*types.ObjectType); isObject {
			// Check if both are function types (have call signatures)
			sigPairs := [][2][]*types.Signature{
				{pType.CallSigs(), aType.CallSigs()},
				{pType.ConstructSigs(), aType.ConstructSigs()},
			}
			for _, pair := range sigPairs {
				if len(pair[0]) == 0 || len(pair[1]) == 0 {
					continue
				}
				// Compare the first signature (most common case)
				pSig := pair[0][0]
				aSig := pair[1][0]

				// Collect constraints from parameter types (contravariant)
				minParams := len(pSig.ParameterTypes)
				if len(aSig.ParameterTypes) < minParams {
					minParams = len(aSig.ParameterTypes)
				}
				for i := 0; i < minParams; i++ {
					paramConstraints := c.collectConstraintsFromTypeSeen(pSig.ParameterTypes[i], aSig.ParameterTypes[i], seen)
					constraints = append(constraints, paramConstraints...)
				}

				// Collect constraints from return type (covariant)
				if pSig.ReturnType != nil && aSig.ReturnType != nil {
					returnConstraints := c.collectConstraintsFromTypeSeen(pSig.ReturnType, aSig.ReturnType, seen)
					constraints = append(constraints, returnConstraints...)
				}
			}

			// Also handle regular object types with properties that may contain type parameters
			// e.g., { value: T } matched against { value: "hello" } should infer T = string
			if len(pType.Props()) > 0 {
				// Members inherited from base types (e.g. a generic base class) count too
				argEffectiveProps := aType.GetEffectiveProperties()
				for _, propName := range types.SortedPropertyNames(pType.Props()) {
					paramPropType := pType.Props()[propName]
					if argPropType, exists := argEffectiveProps[propName]; exists {
						propConstraints := c.collectConstraintsFromTypeSeen(paramPropType, argPropType, seen)
						constraints = append(constraints, propConstraints...)
					}
				}
			}

			if len(pType.IndexSigs()) > 0 {
				for _, idxSig := range pType.IndexSigs() {
					if idxSig == nil || idxSig.ValueType == nil {
						continue
					}

					for _, propName := range types.SortedPropertyNames(aType.Props()) {
						argPropType := aType.Props()[propName]
						if propertyMatchesIndexSignatureKey(propName, idxSig.KeyType) {
							propConstraints := c.collectConstraintsFromTypeSeen(idxSig.ValueType, argPropType, seen)
							markConstraintsCombine(propConstraints)
							constraints = append(constraints, propConstraints...)
						}
					}

					for _, argIdxSig := range aType.IndexSigs() {
						if argIdxSig == nil || argIdxSig.ValueType == nil {
							continue
						}
						if indexSignatureKeysOverlap(idxSig.KeyType, argIdxSig.KeyType) {
							indexConstraints := c.collectConstraintsFromTypeSeen(idxSig.ValueType, argIdxSig.ValueType, seen)
							markConstraintsCombine(indexConstraints)
							constraints = append(constraints, indexConstraints...)
						}
					}
				}
			}
		}

	case *types.MappedType:
		// Handle mapped types like { [P in K]: V } (e.g., Record<K, V>)
		// When matching against an object literal, infer K from keys and V from values
		if aType, isObject := argType.(*types.ObjectType); isObject {
			debugPrintf("// [Checker Constraints] Matching MappedType against ObjectType\n")
			debugPrintf("// [Checker Constraints] MappedType.ConstraintType: %v\n", pType.ConstraintType)
			debugPrintf("// [Checker Constraints] MappedType.ValueType: %v\n", pType.ValueType)

			// Collect keys from the object literal to infer K
			if constraintTypeParam, isTypeParam := pType.ConstraintType.(*types.TypeParameterType); isTypeParam {
				var keyTypes []types.Type
				for _, propName := range types.SortedPropertyNames(aType.Props()) {
					keyTypes = append(keyTypes, &types.LiteralType{Value: vm.String(propName)})
				}
				if len(keyTypes) > 0 {
					var inferredKeyType types.Type
					if len(keyTypes) == 1 {
						inferredKeyType = keyTypes[0]
					} else {
						inferredKeyType = types.NewUnionType(keyTypes...)
					}
					constraints = append(constraints, TypeParameterConstraint{
						TypeParameter: constraintTypeParam.Parameter,
						InferredType:  inferredKeyType,
						Confidence:    90, // Slightly lower confidence for mapped type inference
					})
					if checkerDebug {
						debugPrintf("// [Checker Constraints] Mapped type key constraint: %s = %s\n",
							constraintTypeParam.Parameter.Name, inferredKeyType.String())
					}
				}
			}

			// Collect constraints from values - recursively handle complex value types
			// This handles cases like { [P in K]: [B, C] } where B and C need to be inferred
			for _, __name := range types.SortedPropertyNames(aType.Props()) {
				propType := aType.Props()[__name]
				valueConstraints := c.collectConstraintsFromTypeSeen(pType.ValueType, propType, seen)
				constraints = append(constraints, valueConstraints...)
			}
		}

	case *types.IntersectionType:
		// Infer to every member of an intersection, as tsc does
		// (`IntrinsicAttributes & { items: T[] }`).
		for _, memberType := range pType.Types {
			constraints = append(constraints, c.collectConstraintsFromTypeSeen(memberType, argType, seen)...)
		}

	case *types.UnionType:
		// T | undefined (one naked type parameter beside fixed members): the
		// argument's members matching a fixed member are left out, and the
		// rest infers T, as tsc's inferFromMatchingTypes does.
		if tp, fixed := c.nakedTypeParamUnion(pType); tp != nil {
			var rest []types.Type
			argMembers := []types.Type{argType}
			if au, ok := argType.(*types.UnionType); ok {
				argMembers = au.Types
			}
			for _, m := range argMembers {
				matched := false
				for _, f := range fixed {
					if c.typesEqual(m, f) {
						matched = true
						break
					}
				}
				if !matched {
					rest = append(rest, m)
				}
			}
			if len(rest) > 0 {
				var src types.Type = rest[0]
				if len(rest) > 1 {
					src = types.NewUnionType(rest...)
				}
				constraints = append(constraints, c.collectConstraintsFromTypeSeen(tp, src, seen)...)
			}
			break
		}
		// For union parameter types like T | Getter<T> | Node<T>,
		// try matching argType against each union member and collect constraints from the best match.
		// The member that produces the most constraints (deepest structural match) wins.
		var bestMemberConstraints []TypeParameterConstraint
		for _, memberType := range pType.Types {
			memberConstraints := c.collectConstraintsFromTypeSeen(memberType, argType, seen)
			if len(memberConstraints) > len(bestMemberConstraints) {
				bestMemberConstraints = memberConstraints
			}
		}
		if len(bestMemberConstraints) > 0 {
			constraints = append(constraints, bestMemberConstraints...)
			if checkerDebug {
				debugPrintf("// [Checker Constraints] Union type: collected %d constraints from best-matching member\n", len(bestMemberConstraints))
			}
		}

		// Add more cases for other generic type constructs as needed
	}

	return constraints
}

// solveTypeParameterConstraints attempts to solve the collected constraints
func (c *Checker) solveTypeParameterConstraints(constraints []TypeParameterConstraint) map[*types.TypeParameter]types.Type {
	solution := make(map[*types.TypeParameter]types.Type)

	bestConfidence := make(map[*types.TypeParameter]int)
	bestTypes := make(map[*types.TypeParameter][]types.Type)
	bestCombine := make(map[*types.TypeParameter]bool)

	for _, constraint := range constraints {
		existing, exists := bestConfidence[constraint.TypeParameter]
		if !exists || constraint.Confidence > existing {
			bestConfidence[constraint.TypeParameter] = constraint.Confidence
			bestTypes[constraint.TypeParameter] = []types.Type{constraint.InferredType}
			bestCombine[constraint.TypeParameter] = constraint.Combine
		} else if constraint.Confidence == existing {
			if bestCombine[constraint.TypeParameter] && constraint.Combine {
				bestTypes[constraint.TypeParameter] = appendUniqueType(bestTypes[constraint.TypeParameter], constraint.InferredType)
			} else if cur := bestTypes[constraint.TypeParameter]; len(cur) == 1 && isObjectTypeCandidate(cur[0]) && isObjectTypeCandidate(constraint.InferredType) &&
				// Among equally good object candidates keep the first unless a
				// later one is a strict supertype of it (getCommonSupertype).
				c.rel.IsAssignable(cur[0], constraint.InferredType) && !c.rel.IsAssignable(constraint.InferredType, cur[0]) {
				bestTypes[constraint.TypeParameter] = []types.Type{constraint.InferredType}
			}
		}
	}

	// Convert best constraints to solution
	for typeParam, inferredTypes := range bestTypes {
		var inferredType types.Type
		if len(inferredTypes) == 1 {
			inferredType = inferredTypes[0]
		} else {
			inferredType = types.NewUnionType(inferredTypes...)
		}
		solution[typeParam] = inferredType
		if checkerDebug {
			debugPrintf("// [Checker Solve] %s = %s (confidence: %d)\n",
				typeParam.Name, inferredType.String(), bestConfidence[typeParam])
		}
	}

	return solution
}

func markConstraintsCombine(constraints []TypeParameterConstraint) {
	for i := range constraints {
		constraints[i].Combine = true
	}
}

func appendUniqueType(typeList []types.Type, typ types.Type) []types.Type {
	if typ == nil {
		return typeList
	}
	for _, existing := range typeList {
		if existing != nil && existing.Equals(typ) {
			return typeList
		}
	}
	return append(typeList, typ)
}

func propertyMatchesIndexSignatureKey(propName string, keyType types.Type) bool {
	switch keyType {
	case types.Any, types.String:
		return true
	case types.Number:
		return isNumericPropertyName(propName)
	default:
		return false
	}
}

func indexSignatureKeysOverlap(targetKeyType, sourceKeyType types.Type) bool {
	if targetKeyType == types.Any || sourceKeyType == types.Any {
		return true
	}
	if targetKeyType == types.String {
		return sourceKeyType == types.String || sourceKeyType == types.Number
	}
	return targetKeyType == sourceKeyType
}

func isNumericPropertyName(propName string) bool {
	if propName == "" {
		return false
	}
	for _, char := range propName {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

// substituteTypeParameters creates a new signature with type parameters replaced by inferred types
func (c *Checker) substituteTypeParameters(sig *types.Signature, solution map[*types.TypeParameter]types.Type) *types.Signature {
	memo := make(map[types.Type]types.Type)

	// Helper function to substitute type parameters in a type. Every result
	// is memoized: router types share subterms, and substituting a shared
	// union or mapped type once per path was exponential.
	var substitute, substituteUncached func(t types.Type) types.Type
	substitute = func(t types.Type) types.Type {
		if t == nil {
			return nil
		}
		if substituted, ok := memo[t]; ok {
			return substituted
		}
		r := substituteUncached(t)
		memo[t] = r
		return r
	}
	substituteUncached = func(t types.Type) types.Type {
		switch typ := t.(type) {
		case *types.TypeParameterType:
			if inferredType, found := solution[typ.Parameter]; found {
				return inferredType
			}
			return typ // Keep unresolved type parameters
		case *types.ArrayType:
			return &types.ArrayType{ElementType: substitute(typ.ElementType)}
		case *types.ReadonlyType:
			return types.NewReadonlyType(substitute(typ.InnerType))
		case *types.UnionType:
			var newTypes []types.Type
			for _, memberType := range typ.Types {
				newTypes = append(newTypes, substitute(memberType))
			}
			return types.NewUnionType(newTypes...)
		case *types.IntersectionType:
			newTypes := make([]types.Type, len(typ.Types))
			for i, memberType := range typ.Types {
				newTypes[i] = substitute(memberType)
			}
			return types.NewIntersectionType(newTypes...)
		case *types.InstantiatedType:
			newArgs := make([]types.Type, len(typ.TypeArguments))
			for i, a := range typ.TypeArguments {
				newArgs[i] = substitute(a)
			}
			return types.NewInstantiatedType(typ.Generic, newArgs)
		case *types.ParameterizedForwardReferenceType:
			// Substitute type parameters in the type arguments
			var newTypeArgs []types.Type
			for _, typeArg := range typ.TypeArguments {
				newTypeArgs = append(newTypeArgs, substitute(typeArg))
			}
			// Create a new parameterized forward reference with substituted type arguments
			return &types.ParameterizedForwardReferenceType{
				ClassName:     typ.ClassName,
				TypeArguments: newTypeArgs,
				Scope:         typ.Scope,
			}
		case *types.ObjectType:
			// An object that mentions none of the inferred parameters is kept:
			// copying zod's class types (and expanding every mapped type in
			// their methods) per call was exponential.
			if !typ.Pending && !types.MentionsAnyTypeParameter(typ, solution) {
				memo[t] = typ
				return typ
			}
			// An instantiation of a generic (ZodObject<Shape, ...>) is
			// re-instantiated with substituted arguments: the cached
			// instantiation, without copying and expanding every member.
			if g := typ.Instantiates; g != nil && len(typ.GenericArgs) == len(g.TypeParameters) {
				args := make([]types.Type, len(typ.GenericArgs))
				for i, a := range typ.GenericArgs {
					args[i] = substitute(a)
				}
				inst := c.instantiateGenericType(g, args, nil)
				memo[t] = inst
				return inst
			}

			// For ObjectType, we need to substitute in properties and signatures
			newObj := &types.ObjectType{
				Properties:          make(map[string]types.Type),
				PropertyDocs:        typ.PropertyDocs,
				Doc:                 typ.Doc,
				OptionalProperties:  typ.OptionalProps(), // Copy as-is
				ReadOnlyProperties:  typ.ReadOnlyProps(), // Copy as-is
				CallSignatures:      nil,
				ConstructSignatures: nil,
				BaseTypes:           typ.Bases(), // Copy as-is for now
				IndexSignatures:     nil,
				ClassMeta:           typ.ClassMeta,
				IsReflectIntrinsic:  typ.IsReflectIntrinsic,
			}
			memo[t] = newObj
			typ.CopyDisplay(newObj, substitute)
			if len(typ.Bases()) > 0 {
				// Inherited members are read through the bases.
				newObj.Materialize()
				newObj.BaseTypes = make([]types.Type, len(typ.Bases()))
				for i, base := range typ.Bases() {
					newObj.Bases()[i] = substitute(base)
				}
			}

			// Substitute in properties
			for _, name := range typ.PropertyNames() {
				propType := typ.Props()[name]
				newObj.SetProperty(name, substitute(propType))
			}

			for _, indexSig := range typ.IndexSigs() {
				if indexSig == nil {
					continue
				}
				newObj.Materialize()
				newObj.IndexSignatures = append(newObj.IndexSigs(), &types.IndexSignature{
					KeyType:        substitute(indexSig.KeyType),
					ValueType:      substitute(indexSig.ValueType),
					IsMapped:       indexSig.IsMapped,
					TypeParameter:  indexSig.TypeParameter,
					ConstraintType: substitute(indexSig.ConstraintType),
				})
			}

			// Substitute in call signatures
			for _, sig := range typ.CallSigs() {
				newSig := c.substituteMemberSignature(sig, solution, substitute)
				newObj.Materialize()
				newObj.CallSignatures = append(newObj.CallSigs(), newSig)
			}

			// Substitute in construct signatures
			for _, sig := range typ.ConstructSigs() {
				newSig := c.substituteMemberSignature(sig, solution, substitute)
				newObj.Materialize()
				newObj.ConstructSignatures = append(newObj.ConstructSigs(), newSig)
			}

			return newObj
		case *types.TupleType:
			// Substitute type parameters in tuple element types
			var newElementTypes []types.Type
			for _, elemType := range typ.ElementTypes {
				newElementTypes = append(newElementTypes, substitute(elemType))
			}
			var newRestType types.Type
			if typ.RestElementType != nil {
				newRestType = substitute(typ.RestElementType)
			}
			return &types.TupleType{
				ElementTypes:     newElementTypes,
				OptionalElements: typ.OptionalElements,
				RestElementType:  newRestType,
			}
		case *types.MappedType:
			// Substitute type parameters in the constraint and value types
			constraint := typ.ConstraintType
			if keyof, ok := constraint.(*types.KeyofType); ok {
				// Homomorphic: `[k in keyof T]` keeps T's `?` and `readonly`
				// once T is a concrete object (zod's flatten).
				operand := substitute(keyof.OperandType)
				if _, ok := c.structuralObject(operand); ok {
					constraint = &types.KeyofType{OperandType: operand}
				} else {
					constraint = c.computeKeyofType(operand)
				}
			} else {
				constraint = substitute(constraint)
			}
			substitutedMapped := &types.MappedType{
				TypeParameter:    typ.TypeParameter,
				ConstraintType:   constraint,
				ValueType:        substitute(typ.ValueType),
				ReadonlyModifier: typ.ReadonlyModifier,
				OptionalModifier: typ.OptionalModifier,
			}
			// After substitution, expand the mapped type to a concrete ObjectType
			expanded := c.expandMappedType(substitutedMapped)
			if expanded == nil {
				return substitutedMapped
			}
			if checkerDebug {
				debugPrintf("// [Checker Substitute] Expanded mapped type: %s -> %s\n",
					substitutedMapped.String(), expanded.String())
			}
			return expanded
		case *types.KeyofType:
			return c.computeKeyofType(substitute(typ.OperandType))
		case *types.IndexedAccessType:
			objectType, indexType := substitute(typ.ObjectType), substitute(typ.IndexType)
			if resolved := c.computeIndexedAccessType(objectType, indexType); resolved != nil {
				return resolved
			}
			return &types.IndexedAccessType{ObjectType: objectType, IndexType: indexType}
		case *types.ConditionalType:
			// `undefined extends T[k] ? never : k` in a return type: decided
			// once T is inferred (zod's optionalKeys).
			check, ext := substitute(typ.CheckType), substitute(typ.ExtendsType)
			trueT, falseT := substitute(typ.TrueType), substitute(typ.FalseType)
			if resolved := c.computeConditionalType(check, ext, trueT, falseT); resolved != nil {
				return resolved
			}
			return &types.ConditionalType{CheckType: check, ExtendsType: ext, TrueType: trueT, FalseType: falseT}
		case *types.GenericTypeAliasForwardReference:
			args := make([]types.Type, len(typ.TypeArguments))
			for i, a := range typ.TypeArguments {
				args[i] = substitute(a)
			}
			ref := &types.GenericTypeAliasForwardReference{AliasName: typ.AliasName, TypeArguments: args, Scope: typ.Scope}
			if inst := c.instantiateAliasReference(ref); inst != nil {
				return inst
			}
			return ref
		// Add more cases as needed
		default:
			return typ
		}
	}

	// Substitute in parameter types
	var newParamTypes []types.Type
	for _, paramType := range sig.ParameterTypes {
		newParamTypes = append(newParamTypes, substitute(paramType))
	}

	// Substitute in return type
	var newReturnType types.Type
	if sig.ReturnType != nil {
		newReturnType = substitute(sig.ReturnType)
	}

	// Substitute in rest parameter type
	var newRestParamType types.Type
	if sig.RestParameterType != nil {
		newRestParamType = substitute(sig.RestParameterType)
	}

	return &types.Signature{
		TypeParameters:    sig.TypeParameters,
		ParameterNames:    sig.ParameterNames,
		ParameterTypes:    newParamTypes,
		ReturnType:        newReturnType,
		OptionalParams:    sig.OptionalParams, // Copy as-is
		IsVariadic:        sig.IsVariadic,
		StrictVariance:    sig.StrictVariance,
		RestParameterType: newRestParamType,
	}
}

// substituteMemberSignature substitutes in a signature that is a member of an
// object type. The signature's own type parameters shadow solution entries for
// the same parameters, so those are left alone.
func (c *Checker) substituteMemberSignature(sig *types.Signature, solution map[*types.TypeParameter]types.Type, substitute func(types.Type) types.Type) *types.Signature {
	var inner map[*types.TypeParameter]types.Type
	for _, tp := range sig.TypeParameters {
		if _, ok := solution[tp]; ok {
			if inner == nil {
				inner = make(map[*types.TypeParameter]types.Type, len(solution))
				for k, v := range solution {
					inner[k] = v
				}
			}
			delete(inner, tp)
		}
	}
	if inner == nil {
		return c.substituteInSignature(sig, solution, substitute)
	}
	return c.substituteTypeParameters(sig, inner)
}

// substituteInSignature is a helper to substitute type parameters in a signature
func (c *Checker) substituteInSignature(sig *types.Signature, solution map[*types.TypeParameter]types.Type, substitute func(types.Type) types.Type) *types.Signature {
	// Substitute in parameter types
	var newParamTypes []types.Type
	for _, paramType := range sig.ParameterTypes {
		newParamTypes = append(newParamTypes, substitute(paramType))
	}

	// Substitute in return type
	var newReturnType types.Type
	if sig.ReturnType != nil {
		newReturnType = substitute(sig.ReturnType)
	}

	// Substitute in rest parameter type
	var newRestParamType types.Type
	if sig.RestParameterType != nil {
		newRestParamType = substitute(sig.RestParameterType)
	}

	return &types.Signature{
		TypeParameters:    sig.TypeParameters,
		ParameterNames:    sig.ParameterNames,
		ParameterTypes:    newParamTypes,
		ReturnType:        newReturnType,
		OptionalParams:    sig.OptionalParams, // Copy as-is
		IsVariadic:        sig.IsVariadic,
		StrictVariance:    sig.StrictVariance,
		RestParameterType: newRestParamType,
	}
}

// isPaseratiReflectCall checks if this is a Paserati.reflect<T>() call
func (c *Checker) isPaseratiReflectCall(node *parser.CallExpression) bool {
	// Check if the callee is Paserati.reflect
	memberExpr, ok := node.Function.(*parser.MemberExpression)
	if !ok {
		return false
	}

	// Check if the object is the Paserati identifier
	objIdent, ok := memberExpr.Object.(*parser.Identifier)
	if !ok || objIdent.Value != "Paserati" {
		return false
	}

	// Check if the property is "reflect"
	propIdent, ok := memberExpr.Property.(*parser.Identifier)
	if !ok || propIdent.Value != "reflect" {
		return false
	}

	// Must have exactly one type argument
	if len(node.TypeArguments) != 1 {
		return false
	}

	return true
}

// handlePaseratiReflect processes Paserati.reflect<T>() intrinsic calls
// It resolves the type argument T and stores it on the AST node for the compiler
func (c *Checker) handlePaseratiReflect(node *parser.CallExpression) {
	debugPrintf("// [Checker] Handling Paserati.reflect<T>() intrinsic\n")

	// Resolve the type argument
	if len(node.TypeArguments) != 1 {
		c.addError(node, "Paserati.reflect requires exactly one type argument")
		node.SetComputedType(types.Any)
		return
	}

	typeArg := c.resolveTypeAnnotation(node.TypeArguments[0])
	if typeArg == nil {
		c.addError(node, "could not resolve type argument for Paserati.reflect")
		node.SetComputedType(types.Any)
		return
	}

	if checkerDebug {
		debugPrintf("// [Checker] Paserati.reflect<T>() resolved T to: %s\n", typeArg.String())
	}

	// Store the resolved type on the AST node for the compiler
	node.ResolvedReflectType = typeArg

	// The return type is the Type interface (an object describing the type)
	// Create toJSONSchema method type: () => any
	toJSONSchemaType := types.NewFunctionType(&types.Signature{
		ParameterTypes: []types.Type{},
		ReturnType:     types.Any,
	})

	typeDescriptorType := types.NewObjectType().
		WithProperty("kind", types.String).
		WithOptionalProperty("name", types.String).
		WithOptionalProperty("properties", types.Any).
		WithOptionalProperty("elementType", types.Any).
		WithOptionalProperty("types", types.Any).
		WithOptionalProperty("parameters", types.Any).
		WithOptionalProperty("returnType", types.Any).
		WithProperty("toJSONSchema", toJSONSchemaType)

	node.SetComputedType(typeDescriptorType)
}

func isObjectTypeCandidate(t types.Type) bool {
	_, ok := t.(*types.ObjectType)
	return ok
}

// overloadArgMatches reports whether argument j of the call is acceptable for
// a candidate parameter type: assignable, and (for an object literal written
// in place) free of excess properties.
func (c *Checker) overloadArgMatches(node *parser.CallExpression, j int, argType, paramType types.Type) bool {
	if !c.rel.IsAssignable(argType, paramType) {
		return false
	}
	if c.overloadStrictAny && argType == types.Any && paramType != types.Any && paramType != types.Unknown {
		if _, isTypeParam := paramType.(*types.TypeParameterType); !isTypeParam {
			return false
		}
	}
	if j < len(node.Arguments) && node.Arguments[j] != nil {
		if _, spread := node.Arguments[j].(*parser.SpreadElement); !spread &&
			c.findExcessProperty(node.Arguments[j], paramType) != nil {
			return false
		}
	}
	return true
}

// hasPrimitiveConstraint reports whether a type parameter's constraint admits
// primitive values (a primitive, literal, keyof or template literal type, or a
// union containing one), which stops inference widening its literal candidates.
func hasPrimitiveConstraint(constraint types.Type) bool {
	switch t := constraint.(type) {
	case nil:
		return false
	case *types.KeyofType, *types.LiteralType, *types.TemplateLiteralType:
		return true
	case *types.UnionType:
		for _, m := range t.Types {
			if hasPrimitiveConstraint(m) {
				return true
			}
		}
		return false
	}
	return constraint == types.String || constraint == types.Number || constraint == types.Boolean ||
		constraint == types.BigInt || constraint == types.Symbol
}

// markContextuallyTypedArguments records the unannotated parameters of function
// arguments that sit in a function-typed parameter position of the callee: they
// are contextually typed, so noImplicitAny does not apply to them even when the
// built-in's own typing leaves them as any.
func (c *Checker) markContextuallyTypedArguments(args []parser.Expression, callee types.Type) {
	if !c.noImplicitAny {
		return
	}
	var sigs []*types.Signature
	untyped := callee == types.Any
	switch t := callee.(type) {
	case *types.ObjectType:
		sigs = append(append(sigs, t.CallSigs()...), t.ConstructSigs()...)
	case *types.GenericType:
		if body, ok := t.Body.(*types.ObjectType); ok {
			sigs = append(append(sigs, body.CallSigs()...), body.ConstructSigs()...)
		}
	}
	for i, arg := range args {
		var params []*parser.Parameter
		var rest *parser.RestParameter
		switch fn := arg.(type) {
		case *parser.ArrowFunctionLiteral:
			params, rest = fn.Parameters, fn.RestParameter
		case *parser.FunctionLiteral:
			params, rest = fn.Parameters, fn.RestParameter
		default:
			continue
		}
		if untyped {
			// A callee we could not type (an import(), a loosely typed
			// built-in) may well give its callbacks a signature.
			c.markParametersContextual(params, rest)
			continue
		}
		for _, sig := range sigs {
			var pt types.Type
			switch {
			case i < len(sig.ParameterTypes):
				pt = sig.ParameterTypes[i]
			case sig.IsVariadic && sig.RestParameterType != nil:
				if arr, ok := sig.RestParameterType.(*types.ArrayType); ok {
					pt = arr.ElementType
				}
			}
			if pt == nil {
				continue
			}
			// An `any` parameter counts too: built-in callbacks are often
			// typed that loosely, and flagging their arguments would be noise.
			if pt == types.Any || c.parameterTypeIsFunctionLike(pt) {
				c.markParametersContextual(params, rest)
				break
			}
		}
	}
}

// parameterTypeIsFunctionLike reports whether a parameter type can supply a
// contextual signature: a callable type, a type parameter, or a union of such.
func (c *Checker) parameterTypeIsFunctionLike(t types.Type) bool {
	switch tt := t.(type) {
	case *types.ObjectType:
		return tt.IsCallable()
	case *types.GenericType:
		return c.parameterTypeIsFunctionLike(tt.Body)
	case *types.TypeParameterType:
		return true
	case *types.UnionType:
		for _, m := range tt.Types {
			if c.parameterTypeIsFunctionLike(m) {
				return true
			}
		}
	}
	return false
}

// markContextuallyTypedFunction marks the unannotated parameters of the function
// expressions an expression contains in the positions a contextual type reaches
// (the expression itself, object literal members, array elements, either side
// of `??`, `||`, `&&` and `?:`), for noImplicitAny.
func (c *Checker) markContextuallyTypedFunction(node parser.Node, expected types.Type) {
	if !c.noImplicitAny || expected == nil || expected == types.Any || expected == types.Unknown {
		return
	}
	c.markFunctionsAgainst(node, expected, 0)
}

// markFunctionsAgainst marks the functions under node that expected gives a
// contextual signature, following object literal members and array elements
// into the matching parts of expected. A function whose contextual type
// cannot be callable (unknown, a primitive, a non-callable object, a member
// expected does not have) keeps its implicit any, as in tsc. A nil expected
// means the type could not be taken apart here, and everything under node is
// marked.
func (c *Checker) markFunctionsAgainst(node parser.Node, expected types.Type, depth int) {
	if node == nil || depth > 6 {
		return
	}
	if expected == nil {
		c.markFunctionsUnder(node, depth)
		return
	}
	switch n := node.(type) {
	case *parser.ArrowFunctionLiteral, *parser.FunctionLiteral, *parser.ShorthandMethod, *parser.MethodDefinition:
		if mayGiveSignature(expected) {
			c.markFunctionsUnder(n, depth)
		}
	case *parser.ObjectLiteral:
		for _, prop := range n.Properties {
			if t, known := c.contextualMemberType(expected, prop.Key); !known {
				c.markFunctionsUnder(prop.Value, depth+1)
			} else if t != nil {
				c.markFunctionsAgainst(prop.Value, t, depth+1)
			}
		}
	case *parser.ArrayLiteral:
		var elem types.Type
		if arr, ok := expected.(*types.ArrayType); ok {
			elem = arr.ElementType
		}
		for _, e := range n.Elements {
			c.markFunctionsAgainst(e, elem, depth+1)
		}
	case *parser.TernaryExpression:
		c.markFunctionsAgainst(n.Consequence, expected, depth+1)
		c.markFunctionsAgainst(n.Alternative, expected, depth+1)
	case *parser.InfixExpression:
		switch n.Operator {
		case "??", "||":
			c.markFunctionsAgainst(n.Left, expected, depth+1)
			c.markFunctionsAgainst(n.Right, expected, depth+1)
		case "&&", ",":
			c.markFunctionsAgainst(n.Right, expected, depth+1)
		}
	default:
		c.markFunctionsUnder(node, depth)
	}
}

// mayGiveSignature reports whether a contextual type may supply a function
// with a signature; only types that plainly cannot are ruled out.
func mayGiveSignature(t types.Type) bool {
	switch tt := t.(type) {
	case *types.Primitive:
		return tt == types.Any
	case *types.LiteralType:
		return false
	case *types.ObjectType:
		return tt.IsCallable()
	case *types.UnionType:
		for _, m := range tt.Types {
			if mayGiveSignature(m) {
				return true
			}
		}
		return false
	case *types.IntersectionType:
		for _, m := range tt.Types {
			if mayGiveSignature(m) {
				return true
			}
		}
		return false
	}
	return true
}

// contextualMemberType is the contextual type expected gives the object
// literal member with the given key: a property, else an index signature's
// value. known is false when expected cannot be taken apart here; a nil type
// with known true means the member has no contextual type.
func (c *Checker) contextualMemberType(expected types.Type, key parser.Expression) (t types.Type, known bool) {
	name, numeric := "", false
	switch k := key.(type) {
	case *parser.Identifier:
		name = k.Value
	case *parser.StringLiteral:
		name = k.Value
	case *parser.NumberLiteral:
		name, numeric = fmt.Sprintf("%v", k.Value), true
	default:
		return nil, false
	}
	if mt, ok := expected.(*types.MappedType); ok {
		expected = c.expandMappedType(mt)
	}
	switch e := expected.(type) {
	case *types.Primitive:
		if e == types.Any {
			return nil, false
		}
		return nil, true
	case *types.ObjectType:
		if pt, ok := e.EffectiveProperty(name); ok {
			return pt, true
		}
		for _, is := range e.IndexSignatures {
			if is == nil || is.IsMapped {
				return nil, false
			}
			if is.KeyType == types.String || numeric && is.KeyType == types.Number {
				return is.ValueType, true
			}
		}
		return nil, true
	case *types.IntersectionType, *types.UnionType:
		var members []types.Type
		it, isIntersection := e.(*types.IntersectionType)
		if isIntersection {
			members = it.Types
		} else {
			members = e.(*types.UnionType).Types
		}
		var found []types.Type
		for _, m := range members {
			mt, k := c.contextualMemberType(m, key)
			if !k {
				return nil, false
			}
			if mt != nil && (mt != types.Unknown || !isIntersection) {
				found = append(found, mt)
			}
		}
		switch len(found) {
		case 0:
			return nil, true
		case 1:
			return found[0], true
		}
		if isIntersection {
			return &types.IntersectionType{Types: found}, true
		}
		return types.NewUnionType(found...), true
	}
	return nil, false
}

func (c *Checker) markFunctionsUnder(node parser.Node, depth int) {
	if node == nil || depth > 6 {
		return
	}
	switch n := node.(type) {
	case *parser.ArrowFunctionLiteral:
		c.markParametersContextual(n.Parameters, n.RestParameter)
	case *parser.FunctionLiteral:
		c.markParametersContextual(n.Parameters, n.RestParameter)
	case *parser.ShorthandMethod:
		c.markParametersContextual(n.Parameters, n.RestParameter)
	case *parser.MethodDefinition:
		if n.Value != nil {
			c.markParametersContextual(n.Value.Parameters, n.Value.RestParameter)
		}
	case *parser.ObjectLiteral:
		for _, prop := range n.Properties {
			c.markFunctionsUnder(prop.Value, depth+1)
		}
	case *parser.ArrayLiteral:
		for _, elem := range n.Elements {
			c.markFunctionsUnder(elem, depth+1)
		}
	case *parser.TernaryExpression:
		c.markFunctionsUnder(n.Consequence, depth+1)
		c.markFunctionsUnder(n.Alternative, depth+1)
	case *parser.InfixExpression:
		// tsc's getContextualTypeForBinaryOperand: `||` and `??` pass the
		// context to both operands, `&&` and `,` only to the right one.
		switch n.Operator {
		case "??", "||":
			c.markFunctionsUnder(n.Left, depth+1)
			c.markFunctionsUnder(n.Right, depth+1)
		case "&&", ",":
			c.markFunctionsUnder(n.Right, depth+1)
		}
	case *parser.TypeAssertionExpression:
		c.markFunctionsUnder(n.Expression, depth+1)
	}
}

func (c *Checker) markParametersContextual(params []*parser.Parameter, rest *parser.RestParameter) {
	if c.contextualParams == nil {
		c.contextualParams = make(map[parser.Node]bool)
	}
	for _, p := range params {
		c.contextualParams[p] = true
		delete(c.implicitAnyParams, p) // it may have been checked already
	}
	if rest != nil {
		c.contextualParams[rest] = true
		delete(c.implicitAnyParams, rest)
	}
}

// narrowFilterResult gives `xs.filter(isT)` the type T[] when the callback is a
// type predicate (`(x): x is T => ...`), as the S-returning overload of
// Array.prototype.filter does.
func (c *Checker) narrowFilterResult(node *parser.CallExpression, result types.Type) types.Type {
	member, ok := node.Function.(*parser.MemberExpression)
	if !ok || len(node.Arguments) == 0 || c.extractPropertyName(member.Property) != "filter" {
		return result
	}
	arr, ok := result.(*types.ArrayType)
	if !ok {
		return result
	}
	callback, ok := node.Arguments[0].GetComputedType().(*types.ObjectType)
	if !ok || len(callback.CallSigs()) == 0 {
		return result
	}
	pred, ok := callback.CallSigs()[0].ReturnType.(*types.TypePredicateType)
	if !ok || pred.Asserts || pred.Type == nil || !c.rel.IsAssignable(pred.Type, arr.ElementType) {
		return result
	}
	return &types.ArrayType{ElementType: pred.Type}
}

// intersectionCallSignatures collects the call signatures of an
// intersection's members, in order.
func (c *Checker) intersectionCallSignatures(inter *types.IntersectionType, depth int) []*types.Signature {
	if depth > 8 {
		return nil
	}
	var sigs []*types.Signature
	for _, member := range inter.Types {
		member = types.GetEffectiveType(member)
		if inst, ok := member.(*types.InstantiatedType); ok {
			member = types.GetEffectiveType(inst.Substitute())
		}
		switch m := member.(type) {
		case *types.ObjectType:
			sigs = append(sigs, m.CallSigs()...)
		case *types.IntersectionType:
			sigs = append(sigs, c.intersectionCallSignatures(m, depth+1)...)
		}
	}
	return sigs
}

// hasConflictingDiscriminant reports an intersection TypeScript reduces to
// never: two members give a property disjoint types, one of them a unit type
// (`{ a: "" } & { a: number }`).
func (c *Checker) hasConflictingDiscriminant(inter *types.IntersectionType) bool {
	seen := map[string]types.Type{}
	for _, member := range inter.Types {
		obj, ok := types.GetEffectiveType(member).(*types.ObjectType)
		if !ok {
			continue
		}
		for name, prop := range obj.Props() {
			prev, exists := seen[name]
			if !exists {
				seen[name] = prop
				continue
			}
			if (isUnitType(prev) || isUnitType(prop)) && !c.rel.IsAssignable(prev, prop) && !c.rel.IsAssignable(prop, prev) {
				return true
			}
		}
	}
	return false
}

// typeParamAtTopLevel reports whether t is tp, or a union containing it.
func typeParamAtTopLevel(t types.Type, tp *types.TypeParameter) bool {
	switch tt := t.(type) {
	case *types.TypeParameterType:
		return tt.Parameter == tp
	case *types.UnionType:
		for _, m := range tt.Types {
			if typeParamAtTopLevel(m, tp) {
				return true
			}
		}
	}
	return false
}

// keepsLiteralCandidate is tsc's getCovariantInference widening rule for a
// literal candidate of tp inferred at paramType: it is widened when inferred
// directly for a parameter typed tp, unless tp is a top-level return type and
// is not fixed early by a callback parameter. Literals nested in an array or
// tuple parameter come from declared types and are kept.
func (c *Checker) keepsLiteralCandidate(tp *types.TypeParameter, paramType types.Type) bool {
	if paramType != c.inferTopParam {
		switch c.inferTopParam.(type) {
		case *types.ArrayType, *types.TupleType, *types.ReadonlyType:
			return true
		}
		return false
	}
	if !typeParamAtTopLevel(c.inferSig.ReturnType, tp) {
		return false
	}
	params := append([]types.Type(nil), c.inferSig.ParameterTypes...)
	if c.inferSig.RestParameterType != nil {
		params = append(params, c.inferSig.RestParameterType)
	}
	for _, p := range params {
		if fn, ok := p.(*types.ObjectType); ok {
			for _, cs := range fn.CallSigs() {
				for _, pt := range cs.ParameterTypes {
					if c.typeContainsTypeParameter(pt) {
						return false
					}
				}
			}
		}
	}
	return true
}

// nakedTypeParamUnion returns the one naked type parameter of u and its other
// members, when those mention no type parameter.
func (c *Checker) nakedTypeParamUnion(u *types.UnionType) (*types.TypeParameterType, []types.Type) {
	var tp *types.TypeParameterType
	var fixed []types.Type
	for _, m := range u.Types {
		if t, ok := m.(*types.TypeParameterType); ok {
			if tp != nil {
				return nil, nil
			}
			tp = t
			continue
		}
		if c.typeContainsTypeParameter(m) {
			return nil, nil
		}
		fixed = append(fixed, m)
	}
	if len(fixed) == 0 {
		return nil, nil
	}
	return tp, fixed
}

// constrainedTypeParams maps each type parameter with a literal-like
// constraint to that constraint.
func constrainedTypeParams(tps []*types.TypeParameter) map[*types.TypeParameter]types.Type {
	m := map[*types.TypeParameter]types.Type{}
	for _, tp := range tps {
		if tp != nil && tp.Constraint != nil && hasPrimitiveConstraint(tp.Constraint) {
			m[tp] = tp.Constraint
		}
	}
	return m
}

// strictCallApplyType types fn.call and fn.apply by fn's signature, as
// tsc's CallableFunction does under strictBindCallApply: call takes the
// receiver and then fn's parameters, apply the receiver and a tuple of
// them. Only a single non-generic call signature is modelled; anything
// else keeps Function's methods.
func (c *Checker) strictCallApplyType(fn *types.ObjectType, name string) types.Type {
	if !c.strictBindCallApply || (name != "call" && name != "apply") {
		return nil
	}
	sigs := fn.CallSigs()
	if len(sigs) != 1 || len(sigs[0].TypeParameters) > 0 || sigs[0].IsVariadic && sigs[0].RestParameterType == nil {
		return nil
	}
	sig := sigs[0]
	if sig.RestParameterType != nil {
		if _, isArray := sig.RestParameterType.(*types.ArrayType); !isArray {
			return nil // a generic rest (`...args: A`) is not modelled
		}
	}
	thisType := sig.ThisType
	if thisType == nil {
		thisType = types.Any
	}
	ret := sig.ReturnType
	if ret == nil {
		ret = types.Any
	}
	params := sig.ParameterTypes
	optional := make([]bool, len(params))
	copy(optional, sig.OptionalParams)
	if name == "call" {
		return types.NewFunctionType(&types.Signature{
			ParameterNames:    append([]string{"thisArg"}, sig.ParameterNames...),
			ParameterTypes:    append([]types.Type{thisType}, params...),
			OptionalParams:    append([]bool{false}, optional...),
			RestParameterType: sig.RestParameterType,
			IsVariadic:        sig.IsVariadic,
			ReturnType:        ret,
		})
	}
	tuple := &types.TupleType{ElementTypes: append([]types.Type(nil), params...), OptionalElements: optional}
	if r, ok := sig.RestParameterType.(*types.ArrayType); ok {
		tuple.RestElementType = r.ElementType
	}
	allOptional := true
	for i := range params {
		if !optional[i] {
			allOptional = false
		}
	}
	return types.NewFunctionType(&types.Signature{
		ParameterNames: []string{"thisArg", "args"},
		ParameterTypes: []types.Type{thisType, tuple},
		OptionalParams: []bool{false, allOptional},
		ReturnType:     ret,
	})
}

// isUntypedCallTarget reports whether a value without signatures may still be
// called or constructed, untyped: tsc allows it when the type is assignable
// to the global Function interface.
func (c *Checker) isUntypedCallTarget(t types.Type) bool {
	obj, ok := t.(*types.ObjectType)
	if !ok || len(obj.CallSigs()) > 0 || len(obj.ConstructSigs()) > 0 {
		return false
	}
	if obj.IsFunctionInterface {
		return true
	}
	fn, ok := c.env.ResolveType("Function")
	return ok && fn != nil && c.rel.IsAssignable(obj, fn)
}

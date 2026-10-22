package checker

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// isValidRestParameterType checks if a type is valid for a rest parameter.
// Valid types are array types and tuple types (including variadic tuples).
func isValidRestParameterType(t types.Type) bool {
	t = types.GetEffectiveType(t)
	switch tt := t.(type) {
	case *types.ArrayType, *types.TupleType:
		return true
	case *types.TypeParameterType:
		// A type parameter is fine when its constraint is an array/tuple type
		// (e.g. Args extends any[]); an unconstrained one is not.
		if tt.Parameter != nil && tt.Parameter.Constraint != nil {
			return isValidRestParameterType(tt.Parameter.Constraint)
		}
		return false
	case *types.UnionType:
		for _, member := range tt.Types {
			if !isValidRestParameterType(member) {
				return false
			}
		}
		return true
	case *types.IntersectionType:
		for _, member := range tt.Types {
			if isValidRestParameterType(member) {
				return true
			}
		}
		return false
	case *types.Primitive:
		// `any` (and never) are assignable to any array type.
		return tt == types.Any || tt == types.Never
	case *types.LiteralType:
		return false
	case *types.ObjectType:
		// Interfaces extending Array carry no members here, so an empty object
		// type gets the benefit of the doubt; one with members is not an array.
		if len(tt.CallSigs()) != 0 || len(tt.ConstructSigs()) != 0 {
			return false
		}
		if len(tt.Props()) == 0 {
			return true
		}
		// An interface extending Array or ReadonlyArray inherits their members.
		props := tt.Props()
		for _, name := range []string{"length", "concat", "slice", "forEach"} {
			if _, ok := props[name]; !ok {
				return false
			}
		}
		return true
	}
	// Conditional, indexed-access, infer and other deferred types cannot be
	// judged here.
	return true
}

// getAwaitedType implements the checker-side part of TypeScript's async return
// comparison: Promise<T> and promise-like objects contribute their fulfillment
// type, while non-promise values are used as-is.
func (c *Checker) getAwaitedType(t types.Type) types.Type {
	if t == nil {
		return types.Any
	}

	t = types.GetEffectiveType(t)

	// Awaited distributes over a union: `T | Promise<T>` awaits to T.
	if u, ok := t.(*types.UnionType); ok {
		members := make([]types.Type, len(u.Types))
		changed := false
		for i, m := range u.Types {
			members[i] = c.getAwaitedType(m)
			changed = changed || members[i] != m
		}
		if changed {
			return types.NewUnionType(members...)
		}
		return t
	}

	if instType, ok := t.(*types.InstantiatedType); ok {
		if instType.Generic != nil && instType.Generic.Name == "Promise" && len(instType.TypeArguments) > 0 {
			return c.getAwaitedType(instType.TypeArguments[0])
		}

		substituted := instType.Substitute()
		if substituted != nil && substituted != t {
			return c.getAwaitedType(substituted)
		}
	}

	if objType, ok := t.(*types.ObjectType); ok {
		if thenProp, hasThen := objType.EffectiveProperty("then"); hasThen {
			// A generic then (`then<TResult1, TResult2>(...)`) is its body.
			if g, isGeneric := thenProp.(*types.GenericType); isGeneric {
				thenProp = g.Body
			}
			if thenFuncType, ok := thenProp.(*types.ObjectType); ok && len(thenFuncType.CallSigs()) > 0 {
				sig := thenFuncType.CallSigs()[0]
				if len(sig.ParameterTypes) > 0 {
					// onfulfilled may be optional: `(value: T) => ... | undefined | null`.
					callback := types.RemoveNullishTypes(sig.ParameterTypes[0])
					if callbackFuncType, ok := callback.(*types.ObjectType); ok && len(callbackFuncType.CallSigs()) > 0 {
						callbackSig := callbackFuncType.CallSigs()[0]
						if len(callbackSig.ParameterTypes) > 0 {
							if value := callbackSig.ParameterTypes[0]; value != t {
								return c.getAwaitedType(value)
							}
							return callbackSig.ParameterTypes[0]
						}
					}
				}
			}
		}
	}

	return t
}

// FunctionCheckContext holds the common context for function checking
type FunctionCheckContext struct {
	FunctionName             string                  // For logging and recursion
	TypeParameters           []*parser.TypeParameter // Generic type parameters (if any)
	Parameters               []*parser.Parameter     // Parameter nodes
	RestParameter            *parser.RestParameter   // Rest parameter node (if any)
	ReturnTypeAnnotation     parser.Expression       // Return type annotation (if any)
	Body                     parser.Node             // Function body (block or expression)
	IsArrow                  bool                    // Whether this is an arrow function
	IsGenerator              bool                    // Whether this is a generator function (function*)
	IsAsync                  bool                    // Whether this is an async function
	AllowSelfReference       bool                    // Whether to allow recursive self-reference
	AllowOverloadCompletion  bool                    // Whether to check for overload completion
	ContextualParameterTypes []types.Type            // Contextual parameter types from expected signature
	ContextualReturnType     types.Type              // Contextual return type (nil for generic inference)
	// InferUnderContext: the return expressions are contextually typed by
	// the expected return type but not checked against it, and the return
	// type is inferred from them (tsc does not elaborate a block-bodied or
	// conditional-arm arrow into its returns).
	InferUnderContext bool
	// MayReturnNever: a body that cannot complete and has no return
	// statement returns never (see inferBodyReturnType).
	MayReturnNever bool
}

// resolveFunctionParameters resolves parameter types and creates the parameter environment setup
// Returns: signature, paramTypes, paramNames, restParameterType, restParameterName, typeParamEnv
// checkOptionalBindingParameters reports TS2463 once for each optional
// binding pattern parameter of a function with a body (the parser rewrites
// those parameters and marks them OptionalPattern).
func (c *Checker) checkOptionalBindingParameters(params []*parser.Parameter) {
	for _, param := range params {
		if param == nil || !param.OptionalPattern || c.reportedOptionalPatterns[param] {
			continue
		}
		if c.reportedOptionalPatterns == nil {
			c.reportedOptionalPatterns = map[*parser.Parameter]bool{}
		}
		c.reportedOptionalPatterns[param] = true
		c.addErrorWithCode(param, errors.TS2463, "A binding pattern parameter cannot be optional in an implementation signature.")
	}
}

func (c *Checker) resolveFunctionParameters(ctx *FunctionCheckContext) (*types.Signature, []types.Type, []*parser.Identifier, types.Type, *parser.Identifier, *Environment) {
	c.checkOptionalBindingParameters(ctx.Parameters)
	// 1. First, create an environment for type parameters if this is a generic function
	var typeParamEnv *Environment = c.env

	if len(ctx.TypeParameters) > 0 {
		// Create a new environment that includes type parameters
		typeParamEnv = NewEnclosedEnvironment(c.env)

		typeParams := make([]*types.TypeParameter, len(ctx.TypeParameters))
		for i, typeParamNode := range ctx.TypeParameters {
			typeParam := &types.TypeParameter{
				Name:       typeParamNode.Name.Value,
				Constraint: types.Any,
				Index:      i,
			}

			if !typeParamEnv.DefineTypeParameter(typeParam.Name, typeParam) {
				c.redeclarationReportedByBinder()
			}

			typeParams[i] = typeParam
			typeParamNode.SetComputedType(&types.TypeParameterType{Parameter: typeParam})
		}

		// Resolve constraints/defaults after every type parameter is in scope.
		// TypeScript permits later parameters in earlier constraints, e.g. <U extends T, T>.
		for i, typeParamNode := range ctx.TypeParameters {
			typeParam := typeParams[i]

			var constraintType types.Type
			if typeParamNode.Constraint != nil {
				originalEnv := c.env
				c.env = typeParamEnv // Use the type param environment for constraint resolution
				constraintType = c.resolveTypeAnnotation(typeParamNode.Constraint)
				c.env = originalEnv
				if constraintType == nil {
					constraintType = types.Any // Default constraint
				}
			} else {
				constraintType = types.Any // Default constraint
			}

			// Resolve default type if present
			var defaultType types.Type
			if typeParamNode.DefaultType != nil {
				originalEnv := c.env
				c.env = typeParamEnv // Use the type param environment for default type resolution
				defaultType = c.resolveTypeAnnotation(typeParamNode.DefaultType)
				c.env = originalEnv

				// Validate that default type satisfies constraint if both are present
				if defaultType != nil && constraintType != types.Any {
					// Special case: if default type is a type parameter, be permissive
					if _, ok := defaultType.(*types.TypeParameterType); ok {
						// For type parameters as defaults, TypeScript allows them if there exists
						// some valid instantiation that would satisfy the constraint.
						// For now, we're permissive and allow all type parameter defaults.
						// TODO: Implement proper constraint satisfaction checking
					} else if !c.satisfiesTypeArgConstraint(defaultType, constraintType)() {
						c.addError(typeParamNode.DefaultType, fmt.Sprintf("default type '%s' does not satisfy constraint '%s'", defaultType.String(), constraintType.String()))
					}
				}
			}

			typeParam.Constraint = constraintType
			typeParam.Default = defaultType

			if checkerDebug {
				debugPrintf("// [Checker Function Common] Defined type parameter '%s' with constraint %s\n",
					typeParam.Name, constraintType.String())
			}
		}
	}

	var paramTypes []types.Type
	var paramNames []*parser.Identifier
	var restParameterType types.Type
	var restParameterName *parser.Identifier

	// 2. Resolve regular parameters using the type parameter environment
	for _, param := range ctx.Parameters {
		var paramType types.Type = types.Any
		if param.TypeAnnotation != nil {
			originalEnv := c.env
			c.env = typeParamEnv // Use environment that includes type parameters
			resolvedParamType := c.resolveTypeAnnotation(param.TypeAnnotation)
			c.env = originalEnv
			if resolvedParamType != nil {
				paramType = resolvedParamType
			}
		}
		paramTypes = append(paramTypes, paramType)

		// For 'this' parameters, include in signature but don't add to paramNames (no variable)
		if param.IsThis {
			paramNames = append(paramNames, nil) // Placeholder to keep indices aligned
		} else {
			paramNames = append(paramNames, param.Name)
		}
		param.ComputedType = paramType
	}

	// 3. Handle rest parameter if present using the type parameter environment
	if ctx.RestParameter != nil {
		var resolvedRestType types.Type
		if ctx.RestParameter.TypeAnnotation != nil {
			originalEnv := c.env
			c.env = typeParamEnv // Use environment that includes type parameters
			resolvedRestType = c.resolveTypeAnnotation(ctx.RestParameter.TypeAnnotation)
			c.env = originalEnv

			// Rest parameter type should be an array or tuple type
			if resolvedRestType != nil {
				if !isValidRestParameterType(resolvedRestType) {
					c.addErrorWithCode(ctx.RestParameter.TypeAnnotation, errors.TS2370, "A rest parameter must be of an array type.")
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

	// 4. Resolve return type annotation using the type parameter environment
	var expectedReturnType types.Type
	if ctx.ReturnTypeAnnotation != nil {
		originalEnv := c.env
		c.env = c.returnTypeEnv(typeParamEnv, ctx.Parameters, paramTypes) // type parameters, and the parameters for `typeof p`
		expectedReturnType = c.resolveTypeAnnotation(ctx.ReturnTypeAnnotation)
		c.env = originalEnv
	}

	// Create preliminary signature
	optionalParams := make([]bool, len(ctx.Parameters))
	for i, param := range ctx.Parameters {
		optionalParams[i] = param.Optional || (param.DefaultValue != nil)
	}

	signature := &types.Signature{
		TypeParameters:    c.signatureTypeParameters(ctx.TypeParameters),
		ParameterNames:    parameterNameStrings(ctx.Parameters),
		ParameterTypes:    paramTypes,
		ReturnType:        expectedReturnType,
		OptionalParams:    optionalParams,
		IsVariadic:        ctx.RestParameter != nil,
		RestParameterType: restParameterType,
	}
	dropThisParam(signature, ctx.Parameters)

	return signature, paramTypes, paramNames, restParameterType, restParameterName, typeParamEnv
}

// resolveFunctionParametersWithContext resolves parameter types using contextual type information
// This version prioritizes contextual parameter types over explicit annotations for type inference
func (c *Checker) resolveFunctionParametersWithContext(ctx *FunctionCheckContext) (*types.Signature, []types.Type, []*parser.Identifier, types.Type, *parser.Identifier, *Environment) {
	// If no contextual parameter types, fall back to regular resolution
	if len(ctx.ContextualParameterTypes) == 0 {
		sig, paramTypes, paramNames, restType, restName, env := c.resolveFunctionParameters(ctx)
		// A parameterless callback still takes the contextual return type.
		if sig != nil && ctx.ReturnTypeAnnotation == nil && ctx.ContextualReturnType != nil && sig.ReturnType == nil {
			cp := *sig
			cp.ReturnType = ctx.ContextualReturnType
			sig = &cp
		}
		return sig, paramTypes, paramNames, restType, restName, env
	}
	c.checkOptionalBindingParameters(ctx.Parameters)

	// 1. First, create an environment for type parameters if this is a generic function
	var typeParamEnv *Environment = c.env

	if len(ctx.TypeParameters) > 0 {
		// Create a new environment that includes type parameters
		typeParamEnv = NewEnclosedEnvironment(c.env)

		// Define each type parameter in the environment
		for _, typeParamNode := range ctx.TypeParameters {
			// Resolve constraint if present
			var constraintType types.Type
			if typeParamNode.Constraint != nil {
				originalEnv := c.env
				c.env = typeParamEnv // Use the type param environment for constraint resolution
				constraintType = c.resolveTypeAnnotation(typeParamNode.Constraint)
				c.env = originalEnv
				if constraintType == nil {
					constraintType = types.Any // Default constraint
				}
			} else {
				constraintType = types.Any // Default constraint
			}

			// Resolve default type if present
			var defaultType types.Type
			if typeParamNode.DefaultType != nil {
				originalEnv := c.env
				c.env = typeParamEnv // Use the type param environment for default type resolution
				defaultType = c.resolveTypeAnnotation(typeParamNode.DefaultType)
				c.env = originalEnv

				// Validate that default type satisfies constraint if both are present
				if defaultType != nil && constraintType != types.Any && !c.satisfiesTypeArgConstraint(defaultType, constraintType)() {
					c.addError(typeParamNode.DefaultType, fmt.Sprintf("default type '%s' does not satisfy constraint '%s'", defaultType.String(), constraintType.String()))
				}
			}

			// Create the type parameter
			typeParam := &types.TypeParameter{
				Name:       typeParamNode.Name.Value,
				Constraint: constraintType,
				Default:    defaultType,
			}

			debugPrintf("// [Checker Function Common] Defined type parameter '%s' with constraint any\n", typeParam.Name)
			typeParamEnv.DefineTypeAlias(typeParam.Name, &types.TypeParameterType{Parameter: typeParam})
		}
	}

	// 2. Resolve parameter types, using contextual types when available
	var paramTypes []types.Type
	var paramNames []*parser.Identifier
	var optionalParams []bool

	for i, param := range ctx.Parameters {
		paramNames = append(paramNames, param.Name)

		var paramType types.Type

		// Contextual parameter types do not count an explicit `this` parameter.
		ctxIdx := i
		if len(ctx.Parameters) > 0 && ctx.Parameters[0].IsThis {
			ctxIdx--
		}

		// Use contextual type if available and no explicit annotation
		if ctxIdx >= 0 && ctxIdx < len(ctx.ContextualParameterTypes) && param.TypeAnnotation == nil && !param.IsThis {
			paramType = ctx.ContextualParameterTypes[ctxIdx]
			if checkerDebug {
				debugPrintf("// [Checker FuncContextual] Using contextual type for param '%s': %s\n", param.Name.Value, paramType.String())
			}
		} else if param.TypeAnnotation != nil {
			// Use explicit annotation
			originalEnv := c.env
			c.env = typeParamEnv // Use type param env for parameter type resolution
			paramType = c.resolveTypeAnnotation(param.TypeAnnotation)
			c.env = originalEnv
			if paramType == nil {
				paramType = types.Any
			}
		} else {
			// No contextual type and no annotation - use any
			paramType = types.Any
		}

		paramTypes = append(paramTypes, paramType)
		optionalParams = append(optionalParams, param.DefaultValue != nil)
		if param.IsThis {
			param.ComputedType = paramType
		}
	}

	// 3. Handle rest parameter
	var restParameterType types.Type
	var restParameterName *parser.Identifier
	if ctx.RestParameter != nil {
		restParameterName = ctx.RestParameter.Name

		if ctx.RestParameter.TypeAnnotation != nil {
			originalEnv := c.env
			c.env = typeParamEnv
			restParameterType = c.resolveTypeAnnotation(ctx.RestParameter.TypeAnnotation)
			c.env = originalEnv
		}
		if restParameterType == nil {
			// Default rest parameter type
			restParameterType = &types.ArrayType{ElementType: types.Any}
		}
	}

	// 4. Resolve return type
	var returnType types.Type
	if ctx.ReturnTypeAnnotation != nil {
		originalEnv := c.env
		c.env = c.returnTypeEnv(typeParamEnv, ctx.Parameters, paramTypes)
		returnType = c.resolveTypeAnnotation(ctx.ReturnTypeAnnotation)
		c.env = originalEnv
	} else if ctx.ContextualReturnType != nil {
		// Use contextual return type if provided and no explicit annotation
		returnType = ctx.ContextualReturnType
	}
	if returnType == nil {
		returnType = types.Any // Will be inferred during body checking
	}

	// 5. Create preliminary signature
	signature := &types.Signature{
		ParameterNames:    parameterNameStrings(ctx.Parameters),
		ParameterTypes:    paramTypes,
		ReturnType:        returnType,
		OptionalParams:    optionalParams,
		IsVariadic:        ctx.RestParameter != nil,
		RestParameterType: restParameterType,
	}
	dropThisParam(signature, ctx.Parameters)

	return signature, paramTypes, paramNames, restParameterType, restParameterName, typeParamEnv
}

// setupFunctionEnvironment creates the function scope and defines parameters
func (c *Checker) setupFunctionEnvironment(ctx *FunctionCheckContext, paramTypes []types.Type, paramNames []*parser.Identifier, restParameterType types.Type, restParameterName *parser.Identifier, preliminarySignature *types.Signature, typeParamEnv *Environment) *Environment {
	debugPrintf("// [Checker Function Common] Creating scope for '%s'. Current Env: %p\n", ctx.FunctionName, c.env)
	originalEnv := c.env
	// Use the type parameter environment as the base for the function body
	// environment. NewFunctionEnvironment, not NewEnclosedEnvironment: this is a
	// function scope, and GetFunctionScope (which is where every `var` binding
	// lands) walks up until it finds one. A block-scoped env here let a `var` in
	// any function body escape to the *enclosing* function or global scope -
	// `function outer(){ function inner(){ var w = 1; } return w; }` resolved w.
	funcEnv := NewFunctionEnvironment(typeParamEnv)
	c.env = funcEnv

	// Define regular parameters (skip 'this' parameters which have nil nameNode)
	for i, nameNode := range paramNames {
		if i < len(paramTypes) && nameNode != nil {
			if !funcEnv.Define(nameNode.Value, paramTypes[i], false) {
				c.redeclarationReportedByBinder()
			}
		}
	}

	// Define rest parameter if present
	if restParameterName != nil && restParameterType != nil {
		if !funcEnv.Define(restParameterName.Value, restParameterType, false) {
			c.redeclarationReportedByBinder()
		}
		if checkerDebug {
			debugPrintf("// [Checker Function Common] Defined rest parameter '%s' with type: %s\n", restParameterName.Value, restParameterType.String())
		}
	}

	// Define function itself for recursion if allowed and named
	if ctx.AllowSelfReference && ctx.FunctionName != "<anonymous>" {
		tempFuncTypeForRecursion := types.NewFunctionType(preliminarySignature)
		if !funcEnv.Define(ctx.FunctionName, tempFuncTypeForRecursion, false) {
			// This might happen if a param has the same name - parser should likely prevent this
			debugPrintf("// [Checker Function Common] WARNING: function name '%s' conflicts with a parameter\n", ctx.FunctionName)
		}
	}

	c.checkParameterDefaults(ctx, paramTypes)

	return originalEnv
}

// checkParameterDefaults checks the default value of each annotated parameter
// against its declared type, reporting TS2322 on the parameter name (like a
// variable declaration). Each parameter node is checked once.
func (c *Checker) checkParameterDefaults(ctx *FunctionCheckContext, paramTypes []types.Type) {
	for i, param := range ctx.Parameters {
		if param == nil || param.DefaultValue == nil || param.TypeAnnotation == nil || i >= len(paramTypes) || paramTypes[i] == nil {
			continue
		}
		if c.checkedParameterDefaults == nil {
			c.checkedParameterDefaults = make(map[*parser.Parameter]bool)
		}
		if c.checkedParameterDefaults[param] {
			continue
		}
		c.checkedParameterDefaults[param] = true
		c.visitWithContext(param.DefaultValue, &ContextualType{ExpectedType: paramTypes[i], IsContextual: true})
		defaultType := param.DefaultValue.GetComputedType()
		if defaultType == nil || c.assignableToFresh(param.DefaultValue, defaultType, paramTypes[i]) {
			continue
		}
		var errNode parser.Node = param.DefaultValue
		if param.Name != nil {
			errNode = param.Name
		}
		c.reportNotAssignable(errNode, param.DefaultValue, defaultType, paramTypes[i], headAssign)
	}
}

// checkFunctionBody visits the function body and handles return type inference
func (c *Checker) checkFunctionBody(ctx *FunctionCheckContext, expectedReturnType types.Type) types.Type {
	c.validateParamListBasic(ctx.Parameters)
	c.noteImplicitAnyParameters(ctx)

	// Set return context
	outerExpectedReturnType := c.currentExpectedReturnType
	outerInferredReturnTypes := c.currentInferredReturnTypes
	outerInferredYieldTypes := c.currentInferredYieldTypes

	outerStableParams := c.stableParams
	c.stableParams = stableParameters(ctx.Parameters, ctx.Body)
	defer func() { c.stableParams = outerStableParams }()

	c.currentExpectedReturnType = expectedReturnType
	c.currentInferredReturnTypes = nil
	outerUnchecked := c.uncheckedReturns
	c.uncheckedReturns = ctx.InferUnderContext && expectedReturnType != nil
	defer func() { c.uncheckedReturns = outerUnchecked }()
	if c.uncheckedReturns {
		c.currentInferredReturnTypes = []types.Type{}
	}
	c.currentInferredYieldTypes = []types.Type{} // Always collect yield types for generators
	if expectedReturnType == nil {
		c.currentInferredReturnTypes = []types.Type{}
	}

	// Set async/generator context
	outerInAsyncFunction := c.inAsyncFunction
	outerInGeneratorFunction := c.inGeneratorFunction
	c.inAsyncFunction = ctx.IsAsync
	c.inGeneratorFunction = ctx.IsGenerator
	outerNextTyped := c.generatorNextTyped
	c.generatorNextTyped = ctx.ReturnTypeAnnotation != nil || (ctx.ContextualReturnType != nil && ctx.ContextualReturnType != types.Any) ||
		(expectedReturnType != nil && expectedReturnType != types.Any) || c.argumentGenerators[ctx.Body]
	defer func() { c.generatorNextTyped = outerNextTyped }()

	// Reset loop/switch/label context for new function scope
	outerLoopDepth := c.loopDepth
	outerSwitchDepth := c.switchDepth
	outerActiveLabels := c.activeLabels
	outerCrossTargets := c.crossFunctionTargets
	c.crossFunctionTargets = outerCrossTargets || outerLoopDepth > 0 || outerSwitchDepth > 0 || len(outerActiveLabels) > 0
	c.loopDepth = 0
	c.switchDepth = 0
	c.activeLabels = make(map[string]bool)
	c.functionNestingDepth++
	if !ctx.IsArrow {
		c.nonArrowFunctionDepth++
	}
	outerInArrow := c.innermostFunctionIsArrow
	c.innermostFunctionIsArrow = ctx.IsArrow

	// Set up 'this' context - check for explicit 'this' parameter
	outerThisType := c.currentThisType
	outerImplicitThis := c.implicitAnyThis
	if !ctx.IsArrow {
		c.implicitAnyThis = false
	}
	hasExplicitThisParam := false
	for _, param := range ctx.Parameters {
		if param.IsThis {
			hasExplicitThisParam = true
			// Use the resolved type from parameter resolution
			if param.ComputedType != nil {
				c.currentThisType = param.ComputedType
				if checkerDebug {
					debugPrintf("// [Checker Function Body] Using explicit this parameter type: %s\n", param.ComputedType.String())
				}
			} else {
				c.currentThisType = types.Any
			}
			break
		}
	}

	if !hasExplicitThisParam {
		// No explicit 'this' parameter - only set to 'any' if not already set by calling context
		// (an arrow at the top level keeps the file's `this`).
		if c.currentThisType == nil && !ctx.IsArrow {
			c.currentThisType = types.Any
			// A function with no this of its own: reading it is TS2683.
			c.implicitAnyThis = !ctx.IsArrow && !c.hasContextualThis(ctx.Body)
			debugPrintf("// [Checker Function Body] No explicit this parameter and no context, setting this to any for function literal\n")
		} else {
			if checkerDebug {
				debugPrintf("// [Checker Function Body] No explicit this parameter, but context already set this to: %s\n", c.currentThisType.String())
			}
		}
	}

	var finalReturnType types.Type

	// Visit body
	c.hoistFunctionBodyVars(ctx.Body)
	if exprBody, ok := ctx.Body.(parser.Expression); ok && ctx.IsArrow && expectedReturnType != nil {
		// An expression body is contextually typed by the return type, like a
		// return statement's value: `(n): [number, number] => [n, n]`.
		contextType := expectedReturnType
		if ctx.IsAsync {
			contextType = c.getAwaitedType(contextType)
		}
		c.visitWithContext(exprBody, &ContextualType{ExpectedType: contextType})
	} else {
		c.visit(ctx.Body)
	}
	if block, ok := ctx.Body.(*parser.BlockStatement); ok && !c.uncheckedReturns {
		c.checkMissingReturn(ctx.ReturnTypeAnnotation, expectedReturnType, block, ctx.IsAsync, ctx.IsGenerator)
	}
	if c.uncheckedReturns {
		var inferred types.Type
		if exprBody, ok := ctx.Body.(parser.Expression); ok {
			inferred = exprBody.GetComputedType()
		} else {
			inferred = c.inferBodyReturnType(ctx, nil)
		}
		if inferred == nil {
			inferred = types.Any
		}
		// The inferred type stands in for the contextual one when it fits,
		// unless it is any or the context still names type parameters being
		// inferred: then the body's own type is what inference needs.
		if inferred != types.Any && !c.typeContainsTypeParameter(expectedReturnType) && c.rel.IsAssignable(inferred, expectedReturnType) {
			inferred = expectedReturnType
		}
		finalReturnType = inferred
	}

	// Handle different body types
	if c.uncheckedReturns {
		// Inferred above.
	} else if ctx.IsArrow {
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
				sourceType := bodyType
				targetType := expectedReturnType
				if ctx.IsAsync {
					sourceType = c.getAwaitedType(sourceType)
					targetType = c.getAwaitedType(targetType)
				}
				if _, ok := targetType.(*types.TypePredicateType); ok {
					targetType = types.Boolean
				}
				bodyExpr := exprBody
				// A parameterless callback only contextually typed here keeps
				// the type of a body that does not fit: relating it to the
				// parameter then reports (and elaborates) the mismatch, as tsc
				// does (`foo(() => g)` is "did you mean to call").
				deferToCaller := ctx.ReturnTypeAnnotation == nil && ctx.ContextualReturnType != nil &&
					len(ctx.ContextualParameterTypes) == 0 && len(ctx.Parameters) == 0 && ctx.RestParameter == nil
				if deferToCaller && !c.assignableToFresh(bodyExpr, sourceType, targetType) {
					finalReturnType = bodyType
				} else {
					fits := true
					if bodyExpr != nil && !c.assignableToFresh(bodyExpr, sourceType, targetType) {
						c.reportNotAssignable(exprBody, bodyExpr, sourceType, targetType, headAssign)
						fits = false
					} else if bodyExpr == nil && !c.isAssignableWithExpansion(sourceType, targetType) {
						c.reportNotAssignable(exprBody, nil, sourceType, targetType, headAssign)
						fits = false
					}
					finalReturnType = expectedReturnType
					if ctx.ReturnTypeAnnotation == nil && fits && !ctx.IsAsync {
						// Unannotated, the arrow returns its body's type: the
						// contextual type only typed the body (tsc's
						// getReturnTypeFromBody), and widens a literal it
						// does not ask for.
						finalReturnType = bodyType
						if lit, ok := bodyType.(*types.LiteralType); ok && !literalKeptByContext(lit, expectedReturnType) {
							finalReturnType = c.rel.GetWidenedType(lit)
						}
					}
				}
			} else {
				finalReturnType = bodyType
			}
		} else {
			// Block body for arrow function - use normal inference
			finalReturnType = c.inferBodyReturnType(ctx, expectedReturnType)
		}
	} else {
		// Regular function - use normal inference
		finalReturnType = c.inferBodyReturnType(ctx, expectedReturnType)
	}

	// Restore return context, this context, and async/generator context
	c.currentExpectedReturnType = outerExpectedReturnType
	c.currentInferredReturnTypes = outerInferredReturnTypes
	c.bodyYieldTypes = c.currentInferredYieldTypes
	c.currentInferredYieldTypes = outerInferredYieldTypes
	c.currentThisType = outerThisType
	c.implicitAnyThis = outerImplicitThis
	c.inAsyncFunction = outerInAsyncFunction
	c.inGeneratorFunction = outerInGeneratorFunction
	c.loopDepth = outerLoopDepth
	c.crossFunctionTargets = outerCrossTargets
	c.switchDepth = outerSwitchDepth
	c.activeLabels = outerActiveLabels
	c.functionNestingDepth--
	if !ctx.IsArrow {
		c.nonArrowFunctionDepth--
	}
	c.innermostFunctionIsArrow = outerInArrow

	return finalReturnType
}

// inferFinalReturnType handles return type inference logic
// inferBodyReturnType is inferFinalReturnType for ctx's block body: a body
// with no return statement whose end is unreachable returns never when the
// function may (tsc's mayReturnNever: function expressions, arrows and
// object literal methods, not declarations or class methods).
func (c *Checker) inferBodyReturnType(ctx *FunctionCheckContext, expectedReturnType types.Type) types.Type {
	if expectedReturnType == nil && ctx.MayReturnNever && !ctx.IsGenerator && len(c.currentInferredReturnTypes) == 0 {
		if block, ok := ctx.Body.(*parser.BlockStatement); ok && !c.endReachable(block) {
			return types.Never
		}
	}
	return c.inferFinalReturnType(expectedReturnType, ctx.FunctionName)
}

func (c *Checker) inferFinalReturnType(expectedReturnType types.Type, functionName string) types.Type {
	if expectedReturnType != nil {
		return expectedReturnType
	}

	// Infer from collected return types
	if len(c.currentInferredReturnTypes) == 0 {
		return types.Void // Functions with no return statements have void return type
	}

	finalType := returnTypeOfReturns(c.currentInferredReturnTypes)
	if checkerDebug {
		debugPrintf("// [Checker Function Common] Inferred return type for '%s': %s\n", functionName, finalType.String())
	}
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
		TypeParameters:    c.signatureTypeParameters(ctx.TypeParameters),
		ParameterNames:    parameterNameStrings(ctx.Parameters),
		ParameterTypes:    paramTypes,
		ReturnType:        finalReturnType,
		OptionalParams:    optionalParams,
		IsVariadic:        ctx.RestParameter != nil,
		RestParameterType: restParameterType,
	}
	dropThisParam(sig, ctx.Parameters)

	// Create unified ObjectType with call signature
	return types.NewFunctionType(sig)
}

func parameterNameStrings(params []*parser.Parameter) []string {
	if len(params) == 0 {
		return nil
	}
	names := make([]string, len(params))
	for i, param := range params {
		if param != nil && param.Name != nil {
			names[i] = param.Name.Value
		}
	}
	return names
}

func (c *Checker) signatureTypeParameters(typeParamNodes []*parser.TypeParameter) []*types.TypeParameter {
	if len(typeParamNodes) == 0 {
		return nil
	}
	typeParams := make([]*types.TypeParameter, 0, len(typeParamNodes))
	for _, typeParamNode := range typeParamNodes {
		if computed, ok := typeParamNode.GetComputedType().(*types.TypeParameterType); ok && computed.Parameter != nil {
			typeParams = append(typeParams, computed.Parameter)
		}
	}
	return typeParams
}

// returnTypeEnv is the scope a return type annotation resolves in: the type
// parameters plus the parameters themselves, which the annotation may name in a
// type query (`(a: X): typeof a`).
func (c *Checker) returnTypeEnv(typeParamEnv *Environment, params []*parser.Parameter, paramTypes []types.Type) *Environment {
	env := typeParamEnv
	for i, p := range params {
		if p == nil || p.IsThis || p.Name == nil || i >= len(paramTypes) {
			continue
		}
		if env == typeParamEnv {
			env = NewEnclosedEnvironment(typeParamEnv)
		}
		env.Define(p.Name.Value, paramTypes[i], false)
	}
	return env
}

// dropThisParam removes an explicit `this` parameter from a signature. It
// constrains the receiver, not the arguments, so it takes no part in call
// arity or assignability.
func dropThisParam(sig *types.Signature, params []*parser.Parameter) {
	if len(params) == 0 || params[0] == nil || !params[0].IsThis || len(sig.ParameterTypes) == 0 {
		return
	}
	sig.ThisType = sig.ParameterTypes[0]
	sig.ParameterTypes = sig.ParameterTypes[1:]
	if len(sig.ParameterNames) > 0 {
		sig.ParameterNames = sig.ParameterNames[1:]
	}
	if len(sig.OptionalParams) > 0 {
		sig.OptionalParams = sig.OptionalParams[1:]
	}
}

// implicitAnyParam is a parameter that so far has no written, initial or
// contextual type.
type implicitAnyParam struct {
	name *parser.Identifier
	rest bool
}

// noteImplicitAnyParameters records the parameters of a function being checked
// that have no written type, no initializer and no contextual type
// (noImplicitAny). A function may be checked more than once, first without and
// then with a contextual signature, so the report waits for the end of the
// program (reportImplicitAnyParameters) and a contextual visit withdraws it.
func (c *Checker) noteImplicitAnyParameters(ctx *FunctionCheckContext) {
	if !c.noImplicitAny {
		return
	}
	if c.implicitAnyParams == nil {
		c.implicitAnyParams = make(map[parser.Node]implicitAnyParam)
	}
	for i, param := range ctx.Parameters {
		if param == nil || param.IsThis || param.Name == nil || param.TypeAnnotation != nil || param.DefaultValue != nil ||
			param.Pattern != nil || param.IsDestructuring || strings.HasPrefix(param.Name.Value, "__destructured_param_") {
			continue
		}
		if (i < len(ctx.ContextualParameterTypes) && ctx.ContextualParameterTypes[i] != nil) || c.contextualParams[param] {
			delete(c.implicitAnyParams, param)
			continue
		}
		c.implicitAnyParams[param] = implicitAnyParam{name: param.Name}
	}
	if rest := ctx.RestParameter; rest != nil && rest.Name != nil && rest.TypeAnnotation == nil {
		if len(ctx.ContextualParameterTypes) > len(ctx.Parameters) || c.contextualParams[rest] {
			delete(c.implicitAnyParams, rest)
		} else {
			c.implicitAnyParams[rest] = implicitAnyParam{name: rest.Name, rest: true}
		}
	}
}

// reportImplicitAnyParameters reports TS7006 / TS7019 for every parameter
// still without a type once the whole program has been checked.
func (c *Checker) reportImplicitAnyParameters() {
	pending := make([]implicitAnyParam, 0, len(c.implicitAnyParams))
	for _, p := range c.implicitAnyParams {
		pending = append(pending, p)
	}
	sort.Slice(pending, func(i, j int) bool {
		return parser.GetTokenFromNode(pending[i].name).StartPos < parser.GetTokenFromNode(pending[j].name).StartPos
	})
	for _, p := range pending {
		if p.rest {
			c.addErrorWithCode(p.name, errors.TS7019, fmt.Sprintf("Rest parameter '%s' implicitly has an 'any[]' type.", p.name.Value))
		} else {
			c.addErrorWithCode(p.name, errors.TS7006, fmt.Sprintf("Parameter '%s' implicitly has an 'any' type.", p.name.Value))
		}
	}
	c.implicitAnyParams = nil
}

// hasContextualThis reports whether the function with the given body gets
// its this from where it is written, as tsc's getContextualThisParameterType
// does: a method or property value of an object literal (this is the
// literal) and the value assigned to `obj.x` or `obj[x]` (this is obj).
func (c *Checker) hasContextualThis(body parser.Node) bool {
	if c.program == nil || isNilNode(body) {
		return false
	}
	if c.contextualThisBodies == nil || c.contextualThisProgram != c.program {
		c.contextualThisProgram = c.program
		c.contextualThisBodies = map[parser.Node]bool{}
		mark := func(fn parser.Expression) {
			switch f := fn.(type) {
			case *parser.FunctionLiteral:
				if f.Body != nil {
					c.contextualThisBodies[f.Body] = true
				}
			case *parser.ShorthandMethod:
				if f.Body != nil {
					c.contextualThisBodies[f.Body] = true
				}
			case *parser.MethodDefinition:
				if f.Value != nil && f.Value.Body != nil {
					c.contextualThisBodies[f.Value.Body] = true
				}
			}
		}
		forEachDescendant(c.program, func(n parser.Node) {
			switch e := n.(type) {
			case *parser.ObjectLiteral:
				for _, p := range e.Properties {
					if p != nil {
						mark(p.Value)
					}
				}
			case *parser.ShorthandMethod:
				mark(e)
			case *parser.AssignmentExpression:
				switch e.Left.(type) {
				case *parser.MemberExpression, *parser.IndexExpression:
					mark(e.Value)
				}
			}
		})
	}
	return c.contextualThisBodies[body]
}

// returnTypeOfReturns is the type the returns of a function body make: bare
// `return;` statements (recorded as void) make it void, or contribute
// undefined beside returns with values, as in tsc.
func returnTypeOfReturns(returned []types.Type) types.Type {
	allVoid := true
	for _, t := range returned {
		if t != types.Void {
			allVoid = false
			break
		}
	}
	if allVoid {
		return types.Void
	}
	// never[] (a returned `[]`) is a subtype of every other returned array:
	// tsc's subtype reduction leaves `string[]` of string[] | never[].
	hasArray := slices.ContainsFunc(returned, func(t types.Type) bool {
		switch t := t.(type) {
		case *types.ArrayType:
			return t.ElementType != types.Never
		case *types.TupleType:
			return true
		}
		return false
	})
	normalized := make([]types.Type, 0, len(returned))
	for _, t := range returned {
		if t == types.Void {
			t = types.Undefined
		}
		if arr, ok := t.(*types.ArrayType); ok && hasArray && arr.ElementType == types.Never {
			continue
		}
		normalized = append(normalized, t)
	}
	return types.NewUnionType(normalized...)
}

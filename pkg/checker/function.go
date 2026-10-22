package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

func (c *Checker) checkFunctionLiteral(node *parser.FunctionLiteral) {
	// Use the unified function checking approach
	funcName := "<anonymous>"
	if node.Name != nil {
		funcName = node.Name.Value
	}

	// A declaration hoisted in a block was bound to its signature with the
	// return type unknown yet (see the BlockStatement case); once its body
	// is checked the binding takes the inferred type.
	hoistedType := node.GetComputedType()

	// Create function check context
	ctx := &FunctionCheckContext{
		FunctionName:            funcName,
		TypeParameters:          node.TypeParameters, // Support for generic functions
		Parameters:              node.Parameters,
		RestParameter:           node.RestParameter,
		ReturnTypeAnnotation:    node.ReturnTypeAnnotation,
		Body:                    node.Body,
		IsArrow:                 false,
		MayReturnNever:          !c.voidOnlyFunctions[node],
		IsGenerator:             node.IsGenerator, // Detect generator functions
		IsAsync:                 node.IsAsync,     // Detect async functions
		AllowSelfReference:      node.Name != nil, // Allow recursion for named functions
		AllowOverloadCompletion: node.Name != nil, // Check for overloads for named functions
	}

	// 1. Resolve parameters and signature
	// FIXME: a function expression gets contextual parameter types but no
	// contextual return type, so a generator expression passed where a
	// Generator<0, 0, 1> is expected widens `return 0` (tsc keeps 0;
	// generatorYieldContextualType).
	if ctxTypes, ok := c.functionLiteralContext[node]; ok {
		ctx.ContextualParameterTypes = ctxTypes
		delete(c.functionLiteralContext, node)
	}
	preliminarySignature, paramTypes, paramNames, restParameterType, restParameterName, typeParamEnv := c.resolveFunctionParametersWithContext(ctx)

	// 2. Setup function environment
	originalEnv := c.setupFunctionEnvironment(ctx, paramTypes, paramNames, restParameterType, restParameterName, preliminarySignature, typeParamEnv)

	// 3. Check function body and determine return type
	// For generator functions, we need to check the body against the inner return type,
	// not the full Generator<T, TReturn, TNext> type
	expectedReturnTypeForBody := preliminarySignature.ReturnType
	if node.ReturnTypeAnnotation == nil {
		// No annotation: the return type is inferred from the body (the
		// contextual resolution leaves an any placeholder).
		expectedReturnTypeForBody = nil
	}
	var innerReturnTypeFromAnnotation types.Type

	// Returns are contextually typed by a contextual return type, but not
	// checked against it (see InferUnderContext).
	if rt, ok := c.functionLiteralReturnContext[node]; ok {
		delete(c.functionLiteralReturnContext, node)
		if node.ReturnTypeAnnotation == nil && !node.IsGenerator && !node.IsAsync {
			expectedReturnTypeForBody = rt
			ctx.InferUnderContext = true
		}
	}
	if node.IsGenerator {
		// For generators, we want to check the body against the inner return type
		if preliminarySignature.ReturnType != nil {
			// If there's an explicit return type annotation, extract TReturn from Generator<T, TReturn, TNext>
			innerReturnTypeFromAnnotation = generatorBodyReturnType(preliminarySignature.ReturnType)
			expectedReturnTypeForBody = innerReturnTypeFromAnnotation
		} else {
			// No explicit return type annotation - use nil to allow inference
			expectedReturnTypeForBody = nil
			debugPrintf("// [Checker FuncLit] Generator function: no explicit return type, allowing inference\n")
		}
	}

	outerGeneratorAnnotation := c.currentGeneratorAnnotation
	c.currentGeneratorAnnotation = nil
	if node.IsGenerator {
		c.currentGeneratorAnnotation = preliminarySignature.ReturnType
		c.checkGeneratorAnnotation(node.ReturnTypeAnnotation, preliminarySignature.ReturnType, node.IsAsync)
	}
	finalReturnType := c.checkFunctionBody(ctx, expectedReturnTypeForBody)
	c.currentGeneratorAnnotation = outerGeneratorAnnotation

	// 4. Handle async generator functions (both flags) - wrap in AsyncGenerator<T, TReturn, TNext>
	if node.IsAsync && node.IsGenerator {
		// For async generator functions, return type is AsyncGenerator<YieldType, ReturnType, any>
		debugPrintf("// [Checker FuncLit] Async generator function detected, wrapping return type\n")

		// Use the collected yield types to create AsyncGenerator<T, TReturn, TNext>
		asyncGeneratorType := c.createAsyncGeneratorType(finalReturnType, c.bodyYieldTypes)
		finalReturnType = asyncGeneratorType
		if checkerDebug {
			debugPrintf("// [Checker FuncLit] Created async generator type: %s\n", finalReturnType.String())
		}
	} else if node.IsGenerator {
		// For generator functions, the actual return type becomes Generator<YieldType, ReturnType, any>
		debugPrintf("// [Checker FuncLit] Generator function detected, wrapping return type\n")

		// Use the collected yield types to create the proper Generator<T, TReturn, TNext> type
		generatorType := c.createGeneratorType(finalReturnType, c.bodyYieldTypes)
		finalReturnType = generatorType
		if checkerDebug {
			debugPrintf("// [Checker FuncLit] Created generator type: %s\n", finalReturnType.String())
		}
	} else if node.IsAsync {
		// For async functions, the actual return type becomes Promise<T>
		// But skip wrapping if already Promise<T> (explicit annotation)
		alreadyPromise := false
		if instType, ok := finalReturnType.(*types.InstantiatedType); ok {
			if instType.Generic != nil && instType.Generic.Name == "Promise" {
				alreadyPromise = true
			}
		}
		if !alreadyPromise {
			debugPrintf("// [Checker FuncLit] Async function detected, wrapping return type in Promise\n")
			innerType := finalReturnType
			if innerType == nil {
				innerType = types.Void
			}
			promiseType := c.createPromiseType(innerType)
			finalReturnType = promiseType
			if checkerDebug {
				debugPrintf("// [Checker FuncLit] Created Promise type: %s\n", finalReturnType.String())
			}
		}
	}

	// 5. Create final function type
	finalFuncType := c.createFinalFunctionType(ctx, paramTypes, finalReturnType, restParameterType)

	// 6. Set computed type on the FunctionLiteral node
	if checkerDebug {
		debugPrintf("// [Checker FuncLit] Setting computed type: %s\n", finalFuncType.String())
	}
	node.SetComputedType(finalFuncType)

	// 6. Check for overload completion if this is a named function
	if ctx.AllowOverloadCompletion && c.hasPendingOverloads(funcName) {
		c.completeOverloadedFunction(funcName, finalFuncType)
	}

	// 7. Restore environment
	c.env = originalEnv
	debugPrintf("// [Checker FuncLit] Restored environment to: %p\n", originalEnv)

	if node.Name != nil && hoistedType != nil && hoistedType != finalFuncType && node.ReturnTypeAnnotation == nil {
		if bound, _, ok := c.env.Resolve(funcName); ok && bound == hoistedType {
			c.env.Update(funcName, finalFuncType)
		}
	}
}

// createGeneratorType creates an instantiated Generator<T, TReturn, TNext> type
// based on the yielded and returned types from the generator function
func (c *Checker) createGeneratorType(returnType types.Type, yieldTypes []types.Type) types.Type {
	// Handle nil return type (generators without explicit return default to void)
	if returnType == nil {
		returnType = types.Void
	}

	// Determine the yield type (T) from collected yield expressions
	var yieldType types.Type
	if len(yieldTypes) == 0 {
		// No yield expressions found, use undefined
		yieldType = types.Undefined
	} else if len(yieldTypes) == 1 {
		// Single yield type
		yieldType = yieldTypes[0]
	} else {
		// Multiple yield types, create a union
		yieldType = types.NewUnionType(yieldTypes...)
	}

	// Create an instantiated Generator<T, TReturn, TNext> type
	if types.GeneratorGeneric != nil {
		return types.NewInstantiatedType(types.GeneratorGeneric, []types.Type{
			yieldType,     // T (yield type) - inferred from yield expressions
			returnType,    // TReturn (return type)
			types.Unknown, // TNext (sent type) - TypeScript uses unknown for this
		})
	}

	// Fallback if GeneratorGeneric is not initialized
	generatorObj := types.NewObjectType()
	generatorObj.WithProperty("next", types.NewOptionalFunction([]types.Type{types.Unknown}, types.Any, []bool{true}))
	generatorObj.WithProperty("return", types.NewOptionalFunction([]types.Type{returnType}, types.Any, []bool{true}))
	generatorObj.WithProperty("throw", types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{true}))
	return generatorObj
}

// createAsyncGeneratorType creates an instantiated AsyncGenerator<T, TReturn, TNext> type
// based on the yielded and returned types from the async generator function
func (c *Checker) createAsyncGeneratorType(returnType types.Type, yieldTypes []types.Type) types.Type {
	// Handle nil return type (async generators without explicit return default to void)
	if returnType == nil {
		returnType = types.Void
	}

	// Determine the yield type (T) from collected yield expressions
	var yieldType types.Type
	if len(yieldTypes) == 0 {
		// No yield expressions found, use undefined
		yieldType = types.Undefined
	} else if len(yieldTypes) == 1 {
		// Single yield type
		yieldType = yieldTypes[0]
	} else {
		// Multiple yield types, create a union
		yieldType = types.NewUnionType(yieldTypes...)
	}

	// Create an instantiated AsyncGenerator<T, TReturn, TNext> type
	if types.AsyncGeneratorGeneric != nil {
		return types.NewInstantiatedType(types.AsyncGeneratorGeneric, []types.Type{
			yieldType,     // T (yield type) - inferred from yield expressions
			returnType,    // TReturn (return type)
			types.Unknown, // TNext (sent type) - TypeScript uses unknown for this
		})
	}

	// Fallback if AsyncGeneratorGeneric is not initialized
	// AsyncGenerator methods return Promise<IteratorResult<T, TReturn>>
	asyncGeneratorObj := types.NewObjectType()
	// For simplicity, use Any for the promise return types in fallback
	asyncGeneratorObj.WithProperty("next", types.NewOptionalFunction([]types.Type{types.Unknown}, types.Any, []bool{true}))
	asyncGeneratorObj.WithProperty("return", types.NewOptionalFunction([]types.Type{returnType}, types.Any, []bool{true}))
	asyncGeneratorObj.WithProperty("throw", types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{true}))
	return asyncGeneratorObj
}

// createPromiseType creates an instantiated Promise<T> type
func (c *Checker) createPromiseType(valueType types.Type) types.Type {
	// Handle nil value type (shouldn't happen, but default to void)
	if valueType == nil {
		valueType = types.Void
	}

	// Use the global PromiseGeneric type (set by builtins) - same pattern as generators
	if types.PromiseGeneric != nil {
		return types.NewInstantiatedType(types.PromiseGeneric, []types.Type{valueType})
	}

	// Fallback if PromiseGeneric is not initialized
	// Create a simple object type representing Promise<T>
	promiseObj := types.NewObjectType()

	// Add then method: then<TResult1 = T, TResult2 = never>(
	//   onfulfilled?: (value: T) => TResult1 | PromiseLike<TResult1>,
	//   onrejected?: (reason: any) => TResult2 | PromiseLike<TResult2>
	// ): Promise<TResult1 | TResult2>
	promiseObj.WithProperty("then", types.NewOptionalFunction(
		[]types.Type{
			types.NewOptionalFunction([]types.Type{valueType}, types.Any, []bool{false}),
			types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{false}),
		},
		promiseObj,         // Returns another Promise
		[]bool{true, true}, // Both parameters optional
	))

	// Add catch method: catch<TResult = never>(onrejected?: (reason: any) => TResult): Promise<T | TResult>
	promiseObj.WithProperty("catch", types.NewOptionalFunction(
		[]types.Type{types.NewOptionalFunction([]types.Type{types.Any}, types.Any, []bool{false})},
		promiseObj,
		[]bool{true},
	))

	// Add finally method: finally(onfinally?: () => void): Promise<T>
	promiseObj.WithProperty("finally", types.NewOptionalFunction(
		[]types.Type{types.NewOptionalFunction([]types.Type{}, types.Void, []bool{})},
		promiseObj,
		[]bool{true},
	))

	return promiseObj
}

func (c *Checker) checkMethodDefinition(node *parser.MethodDefinition) {
	// Method definitions are essentially function literals with different kinds
	// Visit the underlying function literal first
	if node.Value != nil {
		c.visit(node.Value)
		// Copy the computed type from the function literal to the method definition
		if funcType := node.Value.GetComputedType(); funcType != nil {
			node.SetComputedType(funcType)
		} else {
			node.SetComputedType(types.Any)
		}
	} else {
		// No function value (shouldn't happen in practice)
		node.SetComputedType(types.Any)
	}
}

// generatorTypeArgs names the generic a generator's return annotation
// instantiates and its type arguments, whether it is still an
// instantiation or already substituted.
func generatorTypeArgs(t types.Type) (string, []types.Type) {
	switch tt := t.(type) {
	case *types.InstantiatedType:
		if tt.Generic != nil {
			return tt.Generic.Name, tt.TypeArguments
		}
	case *types.ObjectType:
		if tt.Instantiates != nil {
			return tt.Instantiates.Name, tt.GenericArgs
		}
	case *types.GenericTypeAliasForwardReference:
		// A lib iterator interface not yet resolved (Iterable<T>,
		// IterableIterator<T>): its name and arguments are all we need.
		return tt.AliasName, tt.TypeArguments
	}
	return "", nil
}

// generatorBodyReturnType is the type a generator body's return statements
// are checked against: TReturn of Generator<Y, R, N> (and its async and
// iterator kin); any for an Iterable annotation; the annotation itself when
// it is not one we model.
func generatorBodyReturnType(annotation types.Type) types.Type {
	name, args := generatorTypeArgs(annotation)
	switch name {
	case "Generator", "AsyncGenerator", "Iterator", "AsyncIterator", "IterableIterator", "AsyncIterableIterator", "IteratorObject":
		if len(args) >= 2 && args[1] != nil {
			return args[1]
		}
		return types.Any
	case "Iterable", "AsyncIterable":
		return types.Any
	}
	// An interface extending one of them (`interface I extends Iterator<0, 1,
	// 2> {}`) is flattened: its next() result still names the return type.
	if ret := iteratorReturnTypeOf(annotation); ret != nil {
		return ret
	}
	// Not modelled (a lib iterator interface we only know by name): the
	// annotation itself, as before generators were understood.
	return annotation
}

// iteratorReturnTypeOf is the value type of the `done: true` member of the
// result next() of the iterator type t returns, or nil when t has no such
// next().
func iteratorReturnTypeOf(t types.Type) types.Type {
	obj, ok := t.(*types.ObjectType)
	if !ok {
		return nil
	}
	next, ok := obj.Props()["next"].(*types.ObjectType)
	if !ok || len(next.CallSigs()) == 0 {
		return nil
	}
	return iterationResultValue(next.CallSigs()[0].ReturnType, true)
}

// iterationResultValue is the type of the `value` of the iteration results
// in result (an IteratorResult<Y, R>, or a hand-written union of the same
// shape): the members whose `done` is the literal true give the return
// side, the others the yield side. nil when there are none.
func iterationResultValue(result types.Type, returnSide bool) types.Type {
	var values []types.Type
	var collect func(t types.Type, depth int)
	collect = func(t types.Type, depth int) {
		if depth > 6 || t == nil {
			return
		}
		switch tt := t.(type) {
		case *types.UnionType:
			for _, m := range tt.Types {
				collect(m, depth+1)
			}
			return
		case *types.InstantiatedType:
			if tt.Generic != nil {
				collect(tt.Substitute(), depth+1)
			}
			return
		case *types.ObjectType:
			value, hasValue := tt.Props()["value"]
			if !hasValue {
				return
			}
			done, isLit := tt.Props()["done"].(*types.LiteralType)
			isReturn := isLit && done.Value.Type() == vm.TypeBoolean && done.Value.AsBoolean()
			if isReturn == returnSide {
				values = append(values, value)
			}
		}
	}
	collect(result, 0)
	if len(values) == 0 {
		return nil
	}
	return types.NewUnionType(values...)
}

// checkGeneratorAnnotation reports TS2322 on a generator's return annotation
// that a generator object cannot be assigned to. Only primitive annotations
// are judged: the lib iterator interfaces are not all modelled.
func (c *Checker) checkGeneratorAnnotation(annotation parser.Expression, declared types.Type, isAsync bool) {
	if annotation == nil || declared == nil || isAsync || types.GeneratorGeneric == nil {
		return
	}
	switch declared.(type) {
	case *types.Primitive, *types.LiteralType:
	default:
		return
	}
	if declared == types.Any || declared == types.Unknown {
		return
	}
	gen := types.NewInstantiatedType(types.GeneratorGeneric, []types.Type{types.Any, types.Any, types.Unknown})
	if !c.isAssignableWithExpansion(gen, declared) {
		c.addErrorWithCode(annotation, errors.TS2322, fmt.Sprintf("Type 'Generator<any, any, unknown>' is not assignable to type '%s'.", declared.String()))
	}
}

// contextualParameterTypes is the contextual type of each parameter of a
// function expression with the given parameters, from the expected type's
// call signature: nil when the expected type gives none (not callable,
// overloads that disagree on their parameters, an annotated parameter, or
// more parameters than the signature can take).
func contextualParameterTypes(expected types.Type, params []*parser.Parameter, rest *parser.RestParameter) []types.Type {
	obj, ok := expected.(*types.ObjectType)
	if !ok || !obj.IsCallable() || len(obj.CallSigs()) == 0 {
		return nil
	}
	sig := obj.CallSigs()[0]
	// With several overloads that disagree on their parameters there is
	// no single contextual signature (getContextualSignature), so the
	// parameters stay untyped.
	for _, other := range obj.CallSigs()[1:] {
		if !sameParameterTypes(sig, other) {
			return nil
		}
	}
	n := 0
	for _, p := range params {
		if p == nil {
			continue
		}
		if p.TypeAnnotation != nil {
			return nil
		}
		if !p.IsThis {
			n++
		}
	}
	if rest != nil {
		return nil
	}
	var restElem types.Type
	if sig.RestParameterType != nil {
		if arr, ok := sig.RestParameterType.(*types.ArrayType); ok {
			restElem = arr.ElementType
		}
	}
	if n > len(sig.ParameterTypes) && restElem == nil {
		return nil
	}
	out := make([]types.Type, 0, n)
	for i := 0; i < n; i++ {
		if i < len(sig.ParameterTypes) {
			out = append(out, sig.ParameterTypes[i])
		} else {
			out = append(out, restElem)
		}
	}
	return out
}

// markVoidOnly records fn as a function declaration or class method (see
// voidOnlyFunctions).
func (c *Checker) markVoidOnly(fn *parser.FunctionLiteral) {
	if fn == nil {
		return
	}
	if c.voidOnlyFunctions == nil {
		c.voidOnlyFunctions = make(map[*parser.FunctionLiteral]bool)
	}
	c.voidOnlyFunctions[fn] = true
}

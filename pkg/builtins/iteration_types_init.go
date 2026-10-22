package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// IterationTypesInitializer declares the lib generics of the iteration
// protocol (lib.es2015.iterable, lib.es2018.asynciterable) and the few
// others tsc's default lib defines structurally: Iterable, Iterator,
// IteratorResult and its two halves, IterableIterator, IteratorObject,
// ArrayIterator/MapIterator/SetIterator, the async counterparts, ArrayLike,
// Awaited, ThisParameterType, OmitThisParameter, PromiseSettledResult,
// PromiseFulfilledResult, PromiseRejectedResult and TypedPropertyDescriptor.
type IterationTypesInitializer struct{}

func (i *IterationTypesInitializer) Name() string { return "IterationTypes" }

// Priority runs after every initializer the types refer to (Promise,
// AsyncGenerator, Generator).
func (i *IterationTypesInitializer) Priority() int { return 1000 }

func (i *IterationTypesInitializer) InitRuntime(ctx *RuntimeContext) error { return nil }

// typeParam makes a type parameter and its reference.
func typeParam(name string, index int, def types.Type) (*types.TypeParameter, types.Type) {
	p := types.NewTypeParameter(name, index, nil)
	p.Default = def
	return p, &types.TypeParameterType{Parameter: p}
}

func optionalMember(o *types.ObjectType, name string, t types.Type) *types.ObjectType {
	o.WithProperty(name, t)
	o.OptionalProperties[name] = true
	return o
}

func (i *IterationTypesInitializer) InitTypes(ctx *TypeContext) error {
	any_, unknown := types.Any, types.Unknown
	promiseOf := func(t types.Type) types.Type {
		return types.NewInstantiatedType(types.PromiseGeneric, []types.Type{t})
	}
	define := func(name string, t types.Type) { _ = ctx.DefineTypeAlias(name, t) }

	define("IteratorYieldResult", types.IteratorYieldResultGeneric)
	define("IteratorReturnResult", types.IteratorReturnResultGeneric)
	define("IteratorResult", types.IteratorResultGeneric)
	resultOf := types.IteratorResultOf

	define("Iterator", types.IteratorGeneric)
	define("Iterable", types.IterableGeneric)
	define("IterableIterator", types.IterableIteratorGeneric)

	// interface IteratorObject<T, TReturn = unknown, TNext = unknown> extends Iterator<T, TReturn, TNext>
	// { [Symbol.iterator](): IteratorObject<T, TReturn, TNext> } plus the
	// ES2025 helper methods.
	ioT, ioTT := typeParam("T", 0, nil)
	ioR, ioRT := typeParam("TReturn", 1, unknown)
	ioN, ioNT := typeParam("TNext", 2, unknown)
	objectGeneric := &types.GenericType{Name: "IteratorObject", TypeParameters: []*types.TypeParameter{ioT, ioR, ioN}}
	objectOf := func(t types.Type) types.Type {
		return types.NewInstantiatedType(objectGeneric, []types.Type{t, types.Undefined, types.Unknown})
	}
	objectBody := types.NewObjectType()
	objectBody.WithProperty("next", types.NewOptionalFunction([]types.Type{ioNT}, resultOf(ioTT, ioRT), []bool{true}))
	optionalMember(objectBody, "return", types.NewOptionalFunction([]types.Type{ioRT}, resultOf(ioTT, ioRT), []bool{true}))
	optionalMember(objectBody, "throw", types.NewOptionalFunction([]types.Type{any_}, resultOf(ioTT, ioRT), []bool{true}))
	objectBody.WithProperty("__COMPUTED_PROPERTY__", types.NewSimpleFunction(nil,
		types.NewInstantiatedType(objectGeneric, []types.Type{ioTT, ioRT, ioNT})))
	uP, uT := typeParam("U", 0, nil)
	predicate := types.NewFunctionType(&types.Signature{ParameterNames: []string{"value", "index"}, ParameterTypes: []types.Type{ioTT, types.Number}, ReturnType: any_})
	objectBody.WithProperty("map", &types.GenericType{Name: "map", TypeParameters: []*types.TypeParameter{uP},
		Body: types.NewSimpleFunction([]types.Type{types.NewFunctionType(&types.Signature{
			ParameterNames: []string{"value", "index"}, ParameterTypes: []types.Type{ioTT, types.Number}, ReturnType: uT})}, objectOf(uT))})
	objectBody.WithProperty("filter", types.NewSimpleFunction([]types.Type{predicate}, objectOf(ioTT)))
	objectBody.WithProperty("take", types.NewSimpleFunction([]types.Type{types.Number}, objectOf(ioTT)))
	objectBody.WithProperty("drop", types.NewSimpleFunction([]types.Type{types.Number}, objectOf(ioTT)))
	objectBody.WithProperty("toArray", types.NewSimpleFunction(nil, &types.ArrayType{ElementType: ioTT}))
	objectBody.WithProperty("forEach", types.NewSimpleFunction([]types.Type{types.NewFunctionType(&types.Signature{
		ParameterNames: []string{"value", "index"}, ParameterTypes: []types.Type{ioTT, types.Number}, ReturnType: types.Void})}, types.Void))
	objectBody.WithProperty("some", types.NewSimpleFunction([]types.Type{predicate}, types.Boolean))
	objectBody.WithProperty("every", types.NewSimpleFunction([]types.Type{predicate}, types.Boolean))
	objectBody.WithProperty("find", types.NewSimpleFunction([]types.Type{predicate}, types.NewUnionType(ioTT, types.Undefined)))
	objectBody.IsInterface = true
	objectBody.GenericName, objectBody.GenericArgs = "IteratorObject", []types.Type{ioTT, ioRT, ioNT}
	objectGeneric.Body = objectBody
	define("IteratorObject", objectGeneric)

	// interface ArrayIterator<T> / MapIterator<T> / SetIterator<T> / StringIterator<T>
	//   extends IteratorObject<T, BuiltinIteratorReturn, unknown>
	// BuiltinIteratorReturn is `undefined` under strictBuiltinIteratorReturn and
	// `any` otherwise.
	// FIXME: Paserati's array/map/set iterators still return its own
	// Iterator<T>; these are the declared names only.
	for _, name := range []string{"ArrayIterator", "MapIterator", "SetIterator", "StringIterator"} {
		p, pT := typeParam("T", 0, nil)
		g := &types.GenericType{Name: name, TypeParameters: []*types.TypeParameter{p}}
		body := types.NewObjectType()
		inst := types.NewInstantiatedType(g, []types.Type{pT})
		res := resultOf(pT, any_)
		body.WithProperty("next", types.NewOptionalFunction([]types.Type{unknown}, res, []bool{true}))
		optionalMember(body, "return", types.NewOptionalFunction([]types.Type{any_}, res, []bool{true}))
		optionalMember(body, "throw", types.NewOptionalFunction([]types.Type{any_}, res, []bool{true}))
		body.WithProperty("__COMPUTED_PROPERTY__", types.NewSimpleFunction(nil, inst))
		body.IsInterface = true
		body.GenericName, body.GenericArgs = name, []types.Type{pT}
		g.Body = body
		define(name, g)
	}

	// interface AsyncIterator<T, TReturn = any, TNext = any> {
	//   next(value?: TNext): Promise<IteratorResult<T, TReturn>>;
	//   return?(value?: TReturn | PromiseLike<TReturn>): Promise<IteratorResult<T, TReturn>>;
	//   throw?(e?: any): Promise<IteratorResult<T, TReturn>>;
	// }
	aiT, aiTT := typeParam("T", 0, nil)
	aiR, aiRT := typeParam("TReturn", 1, any_)
	aiN, aiNT := typeParam("TNext", 2, any_)
	asyncIteratorGeneric := &types.GenericType{Name: "AsyncIterator", TypeParameters: []*types.TypeParameter{aiT, aiR, aiN}}
	asyncIteratorOf := func(t, ret, next types.Type) types.Type {
		return types.NewInstantiatedType(asyncIteratorGeneric, []types.Type{t, ret, next})
	}
	asyncResult := promiseOf(resultOf(aiTT, aiRT))
	returnParam := types.NewUnionType(aiRT, types.NewInstantiatedType(types.PromiseLikeGeneric, []types.Type{aiRT}))
	asyncBody := types.NewObjectType()
	asyncBody.WithProperty("next", types.NewOptionalFunction([]types.Type{aiNT}, asyncResult, []bool{true}))
	optionalMember(asyncBody, "return", types.NewOptionalFunction([]types.Type{returnParam}, asyncResult, []bool{true}))
	optionalMember(asyncBody, "throw", types.NewOptionalFunction([]types.Type{any_}, asyncResult, []bool{true}))
	asyncBody.IsInterface = true
	asyncBody.GenericName, asyncBody.GenericArgs = "AsyncIterator", []types.Type{aiTT, aiRT, aiNT}
	asyncIteratorGeneric.Body = asyncBody
	define("AsyncIterator", asyncIteratorGeneric)

	const asyncKey = "@@symbol:asyncIterator"

	// interface AsyncIterable<T, TReturn = any, TNext = any> { [Symbol.asyncIterator](): AsyncIterator<T, TReturn, TNext> }
	abT, abTT := typeParam("T", 0, nil)
	abR, abRT := typeParam("TReturn", 1, any_)
	abN, abNT := typeParam("TNext", 2, any_)
	asyncIterableBody := types.NewObjectType().WithProperty(asyncKey,
		types.NewSimpleFunction(nil, asyncIteratorOf(abTT, abRT, abNT)))
	asyncIterableBody.IsInterface = true
	asyncIterableBody.GenericName, asyncIterableBody.GenericArgs = "AsyncIterable", []types.Type{abTT, abRT, abNT}
	define("AsyncIterable", types.NewGenericType("AsyncIterable", []*types.TypeParameter{abT, abR, abN}, asyncIterableBody))

	// interface AsyncIterableIterator<T, TReturn = any, TNext = any> extends AsyncIterator<T, TReturn, TNext>
	// { [Symbol.asyncIterator](): AsyncIterableIterator<T, TReturn, TNext> }
	// interface AsyncIteratorObject<T, TReturn = unknown, TNext = unknown> extends AsyncIterator<...>
	// { [Symbol.asyncIterator](): AsyncIteratorObject<T, TReturn, TNext> }
	for _, spec := range []struct {
		name string
		def  types.Type
	}{{"AsyncIterableIterator", any_}, {"AsyncIteratorObject", unknown}} {
		p0, p0T := typeParam("T", 0, nil)
		p1, p1T := typeParam("TReturn", 1, spec.def)
		p2, p2T := typeParam("TNext", 2, spec.def)
		g := &types.GenericType{Name: spec.name, TypeParameters: []*types.TypeParameter{p0, p1, p2}}
		res := promiseOf(resultOf(p0T, p1T))
		body := types.NewObjectType()
		body.WithProperty("next", types.NewOptionalFunction([]types.Type{p2T}, res, []bool{true}))
		optionalMember(body, "return", types.NewOptionalFunction([]types.Type{
			types.NewUnionType(p1T, types.NewInstantiatedType(types.PromiseLikeGeneric, []types.Type{p1T}))}, res, []bool{true}))
		optionalMember(body, "throw", types.NewOptionalFunction([]types.Type{any_}, res, []bool{true}))
		body.WithProperty(asyncKey, types.NewSimpleFunction(nil, types.NewInstantiatedType(g, []types.Type{p0T, p1T, p2T})))
		body.IsInterface = true
		body.GenericName, body.GenericArgs = spec.name, []types.Type{p0T, p1T, p2T}
		g.Body = body
		define(spec.name, g)
	}

	// AsyncGenerator<T = unknown, TReturn = any, TNext = any>, the type
	// async generator functions are inferred with.
	if types.AsyncGeneratorGeneric != nil {
		define("AsyncGenerator", types.AsyncGeneratorGeneric)
	}

	// interface ArrayLike<T> { readonly length: number; readonly [n: number]: T }
	alP, alT := typeParam("T", 0, nil)
	arrayLike := types.NewObjectType().WithProperty("length", types.Number)
	arrayLike.ReadOnlyProperties = map[string]bool{"length": true}
	arrayLike.IndexSignatures = []*types.IndexSignature{{KeyType: types.Number, ValueType: alT, Readonly: true}}
	arrayLike.IsInterface = true
	arrayLike.GenericName, arrayLike.GenericArgs = "ArrayLike", []types.Type{alT}
	define("ArrayLike", types.NewGenericType("ArrayLike", []*types.TypeParameter{alP}, arrayLike))

	// interface PromiseFulfilledResult<T> { status: "fulfilled"; value: T }
	// interface PromiseRejectedResult { status: "rejected"; reason: any }
	// type PromiseSettledResult<T> = PromiseFulfilledResult<T> | PromiseRejectedResult
	pfP, pfT := typeParam("T", 0, nil)
	fulfilled := types.NewObjectType().
		WithProperty("status", &types.LiteralType{Value: vm.NewString("fulfilled")}).
		WithProperty("value", pfT)
	fulfilled.IsInterface = true
	fulfilled.GenericName, fulfilled.GenericArgs = "PromiseFulfilledResult", []types.Type{pfT}
	fulfilledGeneric := types.NewGenericType("PromiseFulfilledResult", []*types.TypeParameter{pfP}, fulfilled)
	define("PromiseFulfilledResult", fulfilledGeneric)
	rejected := types.NewObjectType().
		WithProperty("status", &types.LiteralType{Value: vm.NewString("rejected")}).
		WithProperty("reason", any_)
	rejected.IsInterface = true
	rejected.DisplayName = "PromiseRejectedResult"
	define("PromiseRejectedResult", rejected)
	psP, psT := typeParam("T", 0, nil)
	define("PromiseSettledResult", types.NewGenericType("PromiseSettledResult", []*types.TypeParameter{psP},
		types.NewUnionType(types.NewInstantiatedType(fulfilledGeneric, []types.Type{psT}), rejected)))

	// interface TypedPropertyDescriptor<T> {
	//   enumerable?: boolean; configurable?: boolean; writable?: boolean;
	//   value?: T; get?: () => T; set?: (value: T) => void;
	// }
	tdP, tdT := typeParam("T", 0, nil)
	descriptor := types.NewObjectType()
	optionalMember(descriptor, "enumerable", types.Boolean)
	optionalMember(descriptor, "configurable", types.Boolean)
	optionalMember(descriptor, "writable", types.Boolean)
	optionalMember(descriptor, "value", tdT)
	optionalMember(descriptor, "get", types.NewSimpleFunction(nil, tdT))
	optionalMember(descriptor, "set", types.NewFunctionType(&types.Signature{
		ParameterNames: []string{"value"}, ParameterTypes: []types.Type{tdT}, ReturnType: types.Void}))
	descriptor.IsInterface = true
	descriptor.GenericName, descriptor.GenericArgs = "TypedPropertyDescriptor", []types.Type{tdT}
	define("TypedPropertyDescriptor", types.NewGenericType("TypedPropertyDescriptor", []*types.TypeParameter{tdP}, descriptor))

	i.defineThisParameterTypes(define)
	return nil
}

// defineThisParameterTypes declares
//
//	ThisParameterType<T> = T extends (this: infer U, ...args: never) => any ? U : unknown
//	OmitThisParameter<T> = unknown extends ThisParameterType<T> ? T
//	  : T extends (...args: infer A) => infer R ? (...args: A) => R : T
func (i *IterationTypesInitializer) defineThisParameterTypes(define func(string, types.Type)) {
	tP, tT := typeParam("T", 0, nil)
	inferU := &types.InferType{TypeParameter: "U"}
	withThis := &types.Signature{ReturnType: types.Any, IsVariadic: true, RestParameterType: &types.ArrayType{ElementType: types.Never}, ThisType: inferU}
	thisGeneric := types.NewGenericType("ThisParameterType", []*types.TypeParameter{tP}, &types.ConditionalType{
		CheckType:   tT,
		ExtendsType: types.NewFunctionType(withThis),
		TrueType:    inferU,
		FalseType:   types.Unknown,
	})
	define("ThisParameterType", thisGeneric)

	oP, oT := typeParam("T", 0, nil)
	inferA, inferR := &types.InferType{TypeParameter: "A"}, &types.InferType{TypeParameter: "R"}
	pattern := types.NewFunctionType(&types.Signature{ReturnType: inferR, IsVariadic: true, RestParameterType: inferA})
	rebuilt := types.NewFunctionType(&types.Signature{ReturnType: inferR, IsVariadic: true, RestParameterType: inferA})
	define("OmitThisParameter", types.NewGenericType("OmitThisParameter", []*types.TypeParameter{oP}, &types.ConditionalType{
		CheckType:   types.Unknown,
		ExtendsType: types.NewInstantiatedType(thisGeneric, []types.Type{oT}),
		TrueType:    oT,
		FalseType: &types.ConditionalType{
			CheckType:   oT,
			ExtendsType: pattern,
			TrueType:    rebuilt,
			FalseType:   oT,
		},
	}))
}

package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// streamTypeSet holds the Web Streams typings of lib.dom.d.ts, shared by the
// ReadableStream, WritableStream and TransformStream initializers: the three
// stream interfaces refer to one another (pipeTo takes a WritableStream,
// TransformStream holds both ends), so they are built together, once per
// type environment.
type streamTypeSet struct {
	readable, writable, transform *types.GenericType

	readableCtor, writableCtor, transformCtor *types.ObjectType

	// aliases are the lib interface names user code may write, in
	// definition order.
	aliases []streamAlias
}

type streamAlias struct {
	name string
	typ  types.Type
}

// streamTypes returns ctx's stream typings, building them on first use.
func streamTypes(ctx *TypeContext) *streamTypeSet {
	if ctx.streams == nil {
		ctx.streams = newStreamTypeSet()
	}
	return ctx.streams
}

// defineStreamAliases defines the alias names of set that are not yet
// defined; each stream initializer calls it, so whichever runs first wins.
func defineStreamAliases(ctx *TypeContext, set *streamTypeSet) error {
	if ctx.streamAliasesDefined {
		return nil
	}
	ctx.streamAliasesDefined = true
	for _, a := range set.aliases {
		if err := ctx.DefineTypeAlias(a.name, a.typ); err != nil {
			return err
		}
	}
	return nil
}

// streamParam is one parameter of a stream member signature.
type streamParam struct {
	name     string
	typ      types.Type
	optional bool
}

func streamReq(name string, t types.Type) streamParam { return streamParam{name, t, false} }
func streamOpt(name string, t types.Type) streamParam { return streamParam{name, t, true} }

// streamSig builds a signature from named parameters.
func streamSig(ret types.Type, params ...streamParam) *types.Signature {
	sig := &types.Signature{ReturnType: ret}
	for _, p := range params {
		sig.ParameterNames = append(sig.ParameterNames, p.name)
		sig.ParameterTypes = append(sig.ParameterTypes, p.typ)
		sig.OptionalParams = append(sig.OptionalParams, p.optional)
	}
	return sig
}

// streamMethod is a method member: `name(params): ret`.
func streamMethod(ret types.Type, params ...streamParam) *types.ObjectType {
	return types.NewFunctionType(streamSig(ret, params...))
}

// streamCallback is a function-typed property member (`name?: (params) =>
// ret`, or a lib callback interface): its parameters are compared
// contravariantly under strictFunctionTypes.
func streamCallback(ret types.Type, params ...streamParam) *types.ObjectType {
	sig := streamSig(ret, params...)
	sig.StrictVariance = true
	return types.NewFunctionType(sig)
}

// streamTypeParam is a type parameter defaulting to any (`R = any`).
func streamTypeParam(name string, index int) (*types.TypeParameter, types.Type) {
	tp := &types.TypeParameter{Name: name, Default: types.Any, Index: index}
	return tp, &types.TypeParameterType{Parameter: tp}
}

func streamInst(g *types.GenericType, args ...types.Type) types.Type {
	return types.NewInstantiatedType(g, args)
}

func newStreamTypeSet() *streamTypeSet {
	s := &streamTypeSet{}
	alias := func(name string, t types.Type) { s.aliases = append(s.aliases, streamAlias{name, t}) }
	promise := func(t types.Type) types.Type { return streamInst(types.PromiseGeneric, t) }
	promiseVoid := promise(types.Void)
	nullableNumber := types.NewUnionType(types.Number, types.Null)

	// PromiseLike<T>, which lib.es5 declares and Paserati does not model
	// yet, is built here only for the callback return types below.
	// FIXME: define PromiseLike<T> globally (with then's generic overloads)
	// and use it from there.
	plT, plTType := streamTypeParam("T", 0)
	promiseLike := &types.GenericType{Name: "PromiseLike", TypeParameters: []*types.TypeParameter{plT}}
	promiseLike.Body = types.NewObjectType().WithProperty("then", streamMethod(streamInst(promiseLike, types.Any),
		streamOpt("onfulfilled", types.NewUnionType(streamCallback(types.Any, streamReq("value", plTType)), types.Null, types.Undefined)),
		streamOpt("onrejected", types.NewUnionType(streamCallback(types.Any, streamReq("reason", types.Any)), types.Null, types.Undefined))))
	voidOrPromiseLike := types.NewUnionType(types.Void, streamInst(promiseLike, types.Void))

	newGeneric := func(name string, params ...*types.TypeParameter) *types.GenericType {
		return &types.GenericType{Name: name, TypeParameters: params}
	}

	// interface QueuingStrategy<T = any> { highWaterMark?: number; size?: QueuingStrategySize<T> }
	qsT, qsTType := streamTypeParam("T", 0)
	queuingStrategy := newGeneric("QueuingStrategy", qsT)
	queuingStrategy.Body = types.NewObjectType().
		WithOptionalProperty("highWaterMark", types.Number).
		WithOptionalProperty("size", streamCallback(types.Number, streamReq("chunk", qsTType)))

	// interface StreamPipeOptions
	// FIXME: signal is AbortSignal in lib.dom.
	streamPipeOptions := types.NewObjectType().
		WithOptionalProperty("preventAbort", types.Boolean).
		WithOptionalProperty("preventCancel", types.Boolean).
		WithOptionalProperty("preventClose", types.Boolean).
		WithOptionalProperty("signal", types.Any)
	iteratorOptions := types.NewObjectType().WithOptionalProperty("preventCancel", types.Boolean)

	rsR, rsRType := streamTypeParam("R", 0)
	s.readable = newGeneric("ReadableStream", rsR)
	wsW, wsWType := streamTypeParam("W", 0)
	s.writable = newGeneric("WritableStream", wsW)
	tsI, tsIType := streamTypeParam("I", 0)
	tsO, tsOType := streamTypeParam("O", 1)
	s.transform = newGeneric("TransformStream", tsI, tsO)

	// interface ReadableWritablePair<R = any, W = any>
	pairR, pairRType := streamTypeParam("R", 0)
	pairW, pairWType := streamTypeParam("W", 1)
	pair := newGeneric("ReadableWritablePair", pairR, pairW)
	pair.Body = types.NewObjectType().
		WithProperty("readable", streamInst(s.readable, pairRType)).
		WithProperty("writable", streamInst(s.writable, pairWType))

	// interface ReadableStreamDefaultController<R = any>
	rcR, rcRType := streamTypeParam("R", 0)
	readableController := newGeneric("ReadableStreamDefaultController", rcR)
	readableController.Body = types.NewObjectType().
		WithReadOnlyProperty("desiredSize", nullableNumber).
		WithProperty("close", streamMethod(types.Void)).
		WithProperty("enqueue", streamMethod(types.Void, streamOpt("chunk", rcRType))).
		WithProperty("error", streamMethod(types.Void, streamOpt("e", types.Any)))

	// ReadableStreamReadResult<T> = ReadableStreamReadValueResult<T> | ReadableStreamReadDoneResult<T>
	// The two results are built as named object types rather than lazy
	// references: a union of InstantiatedTypes is not seen through by
	// destructuring and narrowing.
	readValueResult := func(t types.Type) types.Type {
		obj := types.NewObjectType().
			WithProperty("done", types.FalseType).
			WithProperty("value", t)
		obj.GenericName, obj.GenericArgs = "ReadableStreamReadValueResult", []types.Type{t}
		return obj
	}
	readDoneResult := func(t types.Type) types.Type {
		obj := types.NewObjectType().
			WithProperty("done", types.TrueType).
			WithProperty("value", types.NewUnionType(t, types.Undefined))
		obj.GenericName, obj.GenericArgs = "ReadableStreamReadDoneResult", []types.Type{t}
		return obj
	}
	vrT, vrTType := streamTypeParam("T", 0)
	vrT.Default = nil
	valueResult := newGeneric("ReadableStreamReadValueResult", vrT)
	valueResult.Body = readValueResult(vrTType)
	drT, drTType := streamTypeParam("T", 0)
	drT.Default = nil
	doneResult := newGeneric("ReadableStreamReadDoneResult", drT)
	doneResult.Body = readDoneResult(drTType)
	rrT, rrTType := streamTypeParam("T", 0)
	rrT.Default = nil
	readResult := newGeneric("ReadableStreamReadResult", rrT)
	readResult.Body = types.NewUnionType(readValueResult(rrTType), readDoneResult(rrTType))

	// interface ReadableStreamDefaultReader<R = any> extends ReadableStreamGenericReader
	rdR, rdRType := streamTypeParam("R", 0)
	reader := newGeneric("ReadableStreamDefaultReader", rdR)
	reader.Body = types.NewObjectType().
		WithReadOnlyProperty("closed", promiseVoid).
		WithProperty("cancel", streamMethod(promiseVoid, streamOpt("reason", types.Any))).
		// The ReadableStreamReadResult<R> alias, expanded: a reference to a
		// generic alias of a union is not resolved on member access.
		WithProperty("read", streamMethod(promise(types.NewUnionType(
			readValueResult(rdRType), readDoneResult(rdRType))))).
		WithProperty("releaseLock", streamMethod(types.Void))

	// interface ReadableStreamAsyncIterator<T> (lib.dom.asynciterable)
	// FIXME: lib's extends AsyncIteratorObject<T, BuiltinIteratorReturn,
	// unknown>, which Paserati does not model; this is its iterator surface.
	aiT, aiTType := streamTypeParam("T", 0)
	aiT.Default = nil
	asyncIter := newGeneric("ReadableStreamAsyncIterator", aiT)
	iterResult := types.NewUnionType(
		types.NewObjectType().WithOptionalProperty("done", types.FalseType).WithProperty("value", aiTType),
		types.NewObjectType().WithProperty("done", types.TrueType).WithProperty("value", types.Undefined))
	asyncIter.Body = types.NewObjectType().
		WithProperty("next", streamMethod(promise(iterResult))).
		WithOptionalProperty("return", streamMethod(promise(iterResult), streamOpt("value", types.Any))).
		WithProperty("@@symbol:asyncIterator", streamMethod(streamInst(asyncIter, aiTType)))

	// interface ReadableStream<R = any>
	// FIXME: lib's pipeThrough<T>(transform: ReadableWritablePair<T, R>):
	// ReadableStream<T>. A member method's own type parameters are dropped
	// when its interface is instantiated (see substituteMemberSignature),
	// so T would leak; the readable side is any until that is fixed.
	pipeThrough := streamSig(streamInst(s.readable, types.Any),
		streamReq("transform", streamInst(pair, types.Any, rsRType)),
		streamOpt("options", streamPipeOptions))
	// FIXME: getReader's BYOB overloads (`{ mode: "byob" }`) are not typed.
	// FIXME: the runtime has no values() yet.
	s.readable.Body = types.NewObjectType().
		WithReadOnlyProperty("locked", types.Boolean).
		WithProperty("cancel", streamMethod(promiseVoid, streamOpt("reason", types.Any))).
		WithProperty("getReader", streamMethod(streamInst(reader, rsRType))).
		WithProperty("pipeThrough", types.NewFunctionType(pipeThrough)).
		WithProperty("pipeTo", streamMethod(promiseVoid,
			streamReq("destination", streamInst(s.writable, rsRType)),
			streamOpt("options", streamPipeOptions))).
		WithProperty("tee", streamMethod(&types.TupleType{
			ElementTypes: []types.Type{streamInst(s.readable, rsRType), streamInst(s.readable, rsRType)},
		})).
		WithProperty("@@symbol:asyncIterator", streamMethod(streamInst(asyncIter, rsRType), streamOpt("options", iteratorOptions))).
		WithProperty("values", streamMethod(streamInst(asyncIter, rsRType), streamOpt("options", iteratorOptions)))

	// interface UnderlyingDefaultSource<R = any>
	usR, usRType := streamTypeParam("R", 0)
	source := newGeneric("UnderlyingDefaultSource", usR)
	usController := streamInst(readableController, usRType)
	source.Body = types.NewObjectType().
		WithOptionalProperty("cancel", streamCallback(voidOrPromiseLike, streamOpt("reason", types.Any))).
		WithOptionalProperty("pull", streamCallback(voidOrPromiseLike, streamReq("controller", usController))).
		WithOptionalProperty("start", streamCallback(types.Any, streamReq("controller", usController))).
		WithOptionalProperty("type", types.Undefined)

	// interface ReadableByteStreamController
	// FIXME: byobRequest is ReadableStreamBYOBRequest | null and enqueue's
	// chunk ArrayBufferView<ArrayBuffer> in lib.dom.
	byteController := types.NewObjectType().
		WithReadOnlyProperty("byobRequest", types.Any).
		WithReadOnlyProperty("desiredSize", nullableNumber).
		WithProperty("close", streamMethod(types.Void)).
		WithProperty("enqueue", streamMethod(types.Void, streamReq("chunk", types.Any))).
		WithProperty("error", streamMethod(types.Void, streamOpt("e", types.Any)))

	// interface UnderlyingByteSource
	byteSource := types.NewObjectType().
		WithOptionalProperty("autoAllocateChunkSize", types.Number).
		WithOptionalProperty("cancel", streamCallback(voidOrPromiseLike, streamOpt("reason", types.Any))).
		WithOptionalProperty("pull", streamCallback(voidOrPromiseLike, streamReq("controller", byteController))).
		WithOptionalProperty("start", streamCallback(types.Any, streamReq("controller", byteController))).
		WithProperty("type", &types.LiteralType{Value: vm.String("bytes")})

	// declare var ReadableStream: {
	//   prototype: ReadableStream;
	//   new (underlyingSource: UnderlyingByteSource, strategy?: { highWaterMark?: number }): ReadableStream<Uint8Array<ArrayBuffer>>;
	//   new <R = any>(underlyingSource?: UnderlyingDefaultSource<R>, strategy?: QueuingStrategy<R>): ReadableStream<R>;
	// }
	// lib's third overload (UnderlyingSource<R>, the union of both sources)
	// is folded into the second, whose source is optional here.
	// FIXME: the byte stream's chunks are Uint8Array<ArrayBuffer>, which is
	// declared after the streams (by priority); they are any here.
	byteNew := streamSig(streamInst(s.readable, types.Any),
		streamReq("underlyingSource", byteSource),
		streamOpt("strategy", types.NewObjectType().WithOptionalProperty("highWaterMark", types.Number)))
	ctorR, ctorRType := streamTypeParam("R", 0)
	readableNew := streamSig(streamInst(s.readable, ctorRType),
		streamOpt("underlyingSource", streamInst(source, ctorRType)),
		streamOpt("strategy", streamInst(queuingStrategy, ctorRType)))
	readableNew.TypeParameters = []*types.TypeParameter{ctorR}
	s.readableCtor = types.NewObjectType().
		WithConstructSignature(byteNew).
		WithConstructSignature(readableNew).
		WithProperty("prototype", streamInst(s.readable, types.Any))

	// interface WritableStreamDefaultController
	// FIXME: signal is AbortSignal in lib.dom.
	writableController := types.NewObjectType().
		WithReadOnlyProperty("signal", types.Any).
		WithProperty("error", streamMethod(types.Void, streamOpt("e", types.Any)))

	// interface WritableStreamDefaultWriter<W = any>
	wrW, wrWType := streamTypeParam("W", 0)
	writer := newGeneric("WritableStreamDefaultWriter", wrW)
	writer.Body = types.NewObjectType().
		WithReadOnlyProperty("closed", promiseVoid).
		WithReadOnlyProperty("desiredSize", nullableNumber).
		WithReadOnlyProperty("ready", promiseVoid).
		WithProperty("abort", streamMethod(promiseVoid, streamOpt("reason", types.Any))).
		WithProperty("close", streamMethod(promiseVoid)).
		WithProperty("releaseLock", streamMethod(types.Void)).
		WithProperty("write", streamMethod(promiseVoid, streamOpt("chunk", wrWType)))

	// interface WritableStream<W = any>
	s.writable.Body = types.NewObjectType().
		WithReadOnlyProperty("locked", types.Boolean).
		WithProperty("abort", streamMethod(promiseVoid, streamOpt("reason", types.Any))).
		WithProperty("close", streamMethod(promiseVoid)).
		WithProperty("getWriter", streamMethod(streamInst(writer, wsWType)))

	// interface UnderlyingSink<W = any>
	skW, skWType := streamTypeParam("W", 0)
	sink := newGeneric("UnderlyingSink", skW)
	sink.Body = types.NewObjectType().
		WithOptionalProperty("abort", streamCallback(voidOrPromiseLike, streamOpt("reason", types.Any))).
		WithOptionalProperty("close", streamCallback(voidOrPromiseLike)).
		WithOptionalProperty("start", streamCallback(types.Any, streamReq("controller", writableController))).
		WithOptionalProperty("type", types.Undefined).
		WithOptionalProperty("write", streamCallback(voidOrPromiseLike,
			streamReq("chunk", skWType), streamReq("controller", writableController)))

	// declare var WritableStream: { prototype: WritableStream; new <W = any>(...): WritableStream<W> }
	wctorW, wctorWType := streamTypeParam("W", 0)
	writableNew := streamSig(streamInst(s.writable, wctorWType),
		streamOpt("underlyingSink", streamInst(sink, wctorWType)),
		streamOpt("strategy", streamInst(queuingStrategy, wctorWType)))
	writableNew.TypeParameters = []*types.TypeParameter{wctorW}
	s.writableCtor = types.NewObjectType().
		WithConstructSignature(writableNew).
		WithProperty("prototype", streamInst(s.writable, types.Any))

	// interface TransformStreamDefaultController<O = any>
	tcO, tcOType := streamTypeParam("O", 0)
	transformController := newGeneric("TransformStreamDefaultController", tcO)
	transformController.Body = types.NewObjectType().
		WithReadOnlyProperty("desiredSize", nullableNumber).
		WithProperty("enqueue", streamMethod(types.Void, streamOpt("chunk", tcOType))).
		WithProperty("error", streamMethod(types.Void, streamOpt("reason", types.Any))).
		WithProperty("terminate", streamMethod(types.Void))

	// interface TransformStream<I = any, O = any>
	s.transform.Body = types.NewObjectType().
		WithReadOnlyProperty("readable", streamInst(s.readable, tsOType)).
		WithReadOnlyProperty("writable", streamInst(s.writable, tsIType))

	// interface Transformer<I = any, O = any>
	trI, trIType := streamTypeParam("I", 0)
	trO, trOType := streamTypeParam("O", 1)
	transformer := newGeneric("Transformer", trI, trO)
	trController := streamInst(transformController, trOType)
	transformer.Body = types.NewObjectType().
		WithOptionalProperty("flush", streamCallback(voidOrPromiseLike, streamReq("controller", trController))).
		WithOptionalProperty("readableType", types.Undefined).
		WithOptionalProperty("start", streamCallback(types.Any, streamReq("controller", trController))).
		WithOptionalProperty("transform", streamCallback(voidOrPromiseLike,
			streamReq("chunk", trIType), streamReq("controller", trController))).
		WithOptionalProperty("writableType", types.Undefined)

	// declare var TransformStream: { prototype: TransformStream; new <I = any, O = any>(...): TransformStream<I, O> }
	tctorI, tctorIType := streamTypeParam("I", 0)
	tctorO, tctorOType := streamTypeParam("O", 1)
	transformNew := streamSig(streamInst(s.transform, tctorIType, tctorOType),
		streamOpt("transformer", streamInst(transformer, tctorIType, tctorOType)),
		streamOpt("writableStrategy", streamInst(queuingStrategy, tctorIType)),
		streamOpt("readableStrategy", streamInst(queuingStrategy, tctorOType)))
	transformNew.TypeParameters = []*types.TypeParameter{tctorI, tctorO}
	s.transformCtor = types.NewObjectType().
		WithConstructSignature(transformNew).
		WithProperty("prototype", streamInst(s.transform, types.Any, types.Any))

	// Interface bodies print as `Name<Args>`, and so do their instantiations.
	for _, g := range []*types.GenericType{promiseLike, queuingStrategy, pair, readableController,
		reader, asyncIter, s.readable, source, writer, s.writable, sink,
		transformController, s.transform, transformer} {
		if obj, ok := g.Body.(*types.ObjectType); ok {
			obj.GenericName = g.Name
			for _, tp := range g.TypeParameters {
				obj.GenericArgs = append(obj.GenericArgs, &types.TypeParameterType{Parameter: tp})
			}
		}
	}

	alias("ReadableStream", s.readable)
	alias("WritableStream", s.writable)
	alias("TransformStream", s.transform)
	alias("ReadableWritablePair", pair)
	alias("QueuingStrategy", queuingStrategy)
	alias("StreamPipeOptions", streamPipeOptions)
	alias("ReadableStreamIteratorOptions", iteratorOptions)
	alias("ReadableStreamDefaultController", readableController)
	alias("ReadableStreamDefaultReader", reader)
	alias("ReadableStreamReadValueResult", valueResult)
	alias("ReadableStreamReadDoneResult", doneResult)
	alias("ReadableStreamReadResult", readResult)
	alias("ReadableStreamAsyncIterator", asyncIter)
	alias("UnderlyingDefaultSource", source)
	alias("UnderlyingByteSource", byteSource)
	alias("ReadableByteStreamController", byteController)
	alias("WritableStreamDefaultController", writableController)
	alias("WritableStreamDefaultWriter", writer)
	alias("UnderlyingSink", sink)
	alias("TransformStreamDefaultController", transformController)
	alias("Transformer", transformer)
	return s
}

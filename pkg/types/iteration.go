package types

import "github.com/nooga/paserati/pkg/vm"

// The lib's iteration-protocol result types, shared by the generator types
// (lib.es2015.iterable):
//
//	interface IteratorYieldResult<TYield> { done?: false; value: TYield }
//	interface IteratorReturnResult<TReturn> { done: true; value: TReturn }
//	type IteratorResult<T, TReturn = any> = IteratorYieldResult<T> | IteratorReturnResult<TReturn>
var IteratorYieldResultGeneric, IteratorReturnResultGeneric, IteratorResultGeneric = makeIteratorResultGenerics()

func makeIteratorResultGenerics() (*GenericType, *GenericType, *GenericType) {
	yP := NewTypeParameter("TYield", 0, nil)
	yT := &TypeParameterType{Parameter: yP}
	yieldBody := NewObjectType()
	yieldBody.WithProperty("done", &LiteralType{Value: vm.BooleanValue(false)})
	yieldBody.OptionalProperties["done"] = true
	yieldBody.WithProperty("value", yT)
	yieldBody.IsInterface = true
	yieldBody.GenericName, yieldBody.GenericArgs = "IteratorYieldResult", []Type{yT}
	yieldGeneric := NewGenericType("IteratorYieldResult", []*TypeParameter{yP}, yieldBody)

	rP := NewTypeParameter("TReturn", 0, nil)
	rT := &TypeParameterType{Parameter: rP}
	returnBody := NewObjectType().
		WithProperty("done", &LiteralType{Value: vm.BooleanValue(true)}).
		WithProperty("value", rT)
	returnBody.IsInterface = true
	returnBody.GenericName, returnBody.GenericArgs = "IteratorReturnResult", []Type{rT}
	returnGeneric := NewGenericType("IteratorReturnResult", []*TypeParameter{rP}, returnBody)

	t := NewTypeParameter("T", 0, nil)
	ret := NewTypeParameter("TReturn", 1, nil)
	ret.Default = Any
	result := NewGenericType("IteratorResult", []*TypeParameter{t, ret}, NewUnionType(
		NewInstantiatedType(yieldGeneric, []Type{&TypeParameterType{Parameter: t}}),
		NewInstantiatedType(returnGeneric, []Type{&TypeParameterType{Parameter: ret}})))
	return yieldGeneric, returnGeneric, result
}

// IteratorResultOf is IteratorResult<t, ret>, spelled as the union it is so
// that a value of it narrows by `done` wherever it came from.
func IteratorResultOf(t, ret Type) Type {
	return NewUnionType(
		NewInstantiatedType(IteratorYieldResultGeneric, []Type{t}),
		NewInstantiatedType(IteratorReturnResultGeneric, []Type{ret}))
}

// Iterator<T, TReturn = any, TNext = any>, Iterable<T, TReturn = any,
// TNext = any> and IterableIterator<T, TReturn = any, TNext = any>, the
// protocol interfaces the builtin collections are typed against.
var IteratorGeneric, IterableGeneric, IterableIteratorGeneric = makeIterationGenerics()

func makeIterationGenerics() (*GenericType, *GenericType, *GenericType) {
	params := func() (*TypeParameter, *TypeParameter, *TypeParameter, Type, Type, Type) {
		t := NewTypeParameter("T", 0, nil)
		ret := NewTypeParameter("TReturn", 1, nil)
		ret.Default = Any
		next := NewTypeParameter("TNext", 2, nil)
		next.Default = Any
		return t, ret, next, &TypeParameterType{Parameter: t}, &TypeParameterType{Parameter: ret}, &TypeParameterType{Parameter: next}
	}
	protocol := func(o *ObjectType, t, ret, next Type) {
		o.WithProperty("next", NewOptionalFunction([]Type{next}, IteratorResultOf(t, ret), []bool{true}))
		o.WithProperty("return", NewOptionalFunction([]Type{ret}, IteratorResultOf(t, ret), []bool{true}))
		o.WithProperty("throw", NewOptionalFunction([]Type{Any}, IteratorResultOf(t, ret), []bool{true}))
		o.OptionalProperties["return"], o.OptionalProperties["throw"] = true, true
	}

	// interface Iterator<T, TReturn, TNext> { next(value?: TNext): IteratorResult<T, TReturn>;
	//   return?(value?: TReturn): ...; throw?(e?: any): ... }
	p0, p1, p2, t0, t1, t2 := params()
	iteratorGeneric := &GenericType{Name: "Iterator", TypeParameters: []*TypeParameter{p0, p1, p2}}
	iteratorBody := NewObjectType()
	protocol(iteratorBody, t0, t1, t2)
	iteratorBody.IsInterface = true
	iteratorBody.GenericName, iteratorBody.GenericArgs = "Iterator", []Type{t0, t1, t2}
	iteratorGeneric.Body = iteratorBody

	// interface Iterable<T, TReturn, TNext> { [Symbol.iterator](): Iterator<T, TReturn, TNext> }
	q0, q1, q2, u0, u1, u2 := params()
	iterableBody := NewObjectType().WithProperty("__COMPUTED_PROPERTY__",
		NewSimpleFunction(nil, NewInstantiatedType(iteratorGeneric, []Type{u0, u1, u2})))
	iterableBody.IsInterface = true
	iterableBody.GenericName, iterableBody.GenericArgs = "Iterable", []Type{u0, u1, u2}
	iterableGeneric := NewGenericType("Iterable", []*TypeParameter{q0, q1, q2}, iterableBody)

	// interface IterableIterator<T, TReturn, TNext> extends Iterator<T, TReturn, TNext>
	// { [Symbol.iterator](): IterableIterator<T, TReturn, TNext> }
	r0, r1, r2, v0, v1, v2 := params()
	iterIterGeneric := &GenericType{Name: "IterableIterator", TypeParameters: []*TypeParameter{r0, r1, r2}}
	iterIterBody := NewObjectType()
	protocol(iterIterBody, v0, v1, v2)
	iterIterBody.WithProperty("__COMPUTED_PROPERTY__", NewSimpleFunction(nil,
		NewInstantiatedType(iterIterGeneric, []Type{v0, v1, v2})))
	iterIterBody.IsInterface = true
	iterIterBody.GenericName, iterIterBody.GenericArgs = "IterableIterator", []Type{v0, v1, v2}
	iterIterGeneric.Body = iterIterBody

	return iteratorGeneric, iterableGeneric, iterIterGeneric
}

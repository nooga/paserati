package builtins

import (
	"paserati/pkg/types"
)

type IteratorInitializer struct{}

func (i *IteratorInitializer) Name() string {
	return "Iterator"
}

func (i *IteratorInitializer) Priority() int {
	return PriorityIterator
}

func (i *IteratorInitializer) InitTypes(ctx *TypeContext) error {
	// Create generic type parameter T for iterator types
	tParam := &types.TypeParameter{Name: "T", Constraint: nil, Index: 0}
	tType := &types.TypeParameterType{Parameter: tParam}

	// Create IteratorResult<T> interface
	// interface IteratorResult<T> { value: T; done: boolean; }
	iteratorResultType := types.NewObjectType().
		WithProperty("value", tType).
		WithProperty("done", types.Boolean)

	// Create generic IteratorResult type
	iteratorResultGeneric := &types.GenericType{
		Name:           "IteratorResult",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           iteratorResultType,
	}

	// Create Iterator<T> interface
	// interface Iterator<T> { next(): IteratorResult<T>; }
	iteratorType := types.NewObjectType().
		WithProperty("next", types.NewSimpleFunction([]types.Type{}, 
			&types.InstantiatedType{
				Generic:       iteratorResultGeneric,
				TypeArguments: []types.Type{tType},
			}))

	// Create generic Iterator type
	iteratorGeneric := &types.GenericType{
		Name:           "Iterator",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           iteratorType,
	}

	// Create Iterable<T> interface
	// interface Iterable<T> { [Symbol.iterator](): Iterator<T>; }
	iterableType := types.NewObjectType().
		WithProperty("@@symbol:Symbol.iterator", types.NewSimpleFunction([]types.Type{},
			&types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{tType},
			}))

	// Create generic Iterable type
	iterableGeneric := &types.GenericType{
		Name:           "Iterable",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           iterableType,
	}

	// Register the types in global environment
	ctx.DefineGlobal("IteratorResult", iteratorResultGeneric)
	ctx.DefineGlobal("Iterator", iteratorGeneric)
	ctx.DefineGlobal("Iterable", iterableGeneric)

	return nil
}

func (i *IteratorInitializer) InitRuntime(ctx *RuntimeContext) error {
	// These are interface types - no runtime implementation needed
	// The actual iterators are implemented by Array, String, Generator, etc.
	return nil
}
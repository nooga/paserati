package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type AsyncGeneratorInitializer struct{}

func (g *AsyncGeneratorInitializer) Name() string {
	return "AsyncGenerator"
}

func (g *AsyncGeneratorInitializer) Priority() int {
	return PriorityAsyncGenerator
}

func (g *AsyncGeneratorInitializer) InitTypes(ctx *TypeContext) error {
	// Simplified type for AsyncGenerator - just define basic structure
	// Methods return promises, but we'll keep type system simple for now
	asyncGeneratorProtoType := types.NewObjectType().
		WithProperty("next", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("return", types.NewSimpleFunction([]types.Type{types.Any}, types.Any)).
		WithProperty("throw", types.NewSimpleFunction([]types.Type{types.Any}, types.Any))

	ctx.SetPrimitivePrototype("asyncgenerator", asyncGeneratorProtoType)

	asyncGeneratorCtorType := types.NewObjectType().
		WithProperty("prototype", asyncGeneratorProtoType)

	return ctx.DefineGlobal("AsyncGenerator", asyncGeneratorCtorType)
}

func (g *AsyncGeneratorInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// %AsyncIteratorPrototype% (27.1.3): [Symbol.asyncIterator]() returns
	// this, and [Symbol.asyncDispose]() calls return().
	asyncIteratorProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	asyncIteratorMethod := vm.NewNativeFunction(0, false, "[Symbol.asyncIterator]", func(args []vm.Value) (vm.Value, error) {
		return vmInstance.GetThis(), nil
	})
	wTrue, eFalse, cTrue := true, false, true
	asyncIteratorProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolAsyncIterator), asyncIteratorMethod, &wTrue, &eFalse, &cTrue)
	if vmInstance.SymbolAsyncDispose.Type() == vm.TypeSymbol {
		asyncDispose := vm.NewNativeFunction(0, false, "[Symbol.asyncDispose]", func(args []vm.Value) (vm.Value, error) {
			return vmInstance.AsyncIteratorDispose(vmInstance.GetThis()), nil
		})
		asyncIteratorProto.DefineOwnPropertyByKey(vm.NewSymbolKey(vmInstance.SymbolAsyncDispose), asyncDispose, &wTrue, &eFalse, &cTrue)
	}
	vmInstance.AsyncIteratorPrototype = vm.NewValueFromPlainObject(asyncIteratorProto)

	asyncGeneratorProto := vm.NewObject(vmInstance.AsyncIteratorPrototype).AsPlainObject()

	argOrUndefined := func(args []vm.Value) vm.Value {
		if len(args) > 0 {
			return args[0]
		}
		return vm.Undefined
	}
	asyncGeneratorProto.SetOwnNonEnumerable("next", vm.NewNativeFunction(1, false, "next", func(args []vm.Value) (vm.Value, error) {
		return vmInstance.AsyncGeneratorNext(vmInstance.GetThis(), argOrUndefined(args)), nil
	}))
	asyncGeneratorProto.SetOwnNonEnumerable("return", vm.NewNativeFunction(1, false, "return", func(args []vm.Value) (vm.Value, error) {
		return vmInstance.AsyncGeneratorReturn(vmInstance.GetThis(), argOrUndefined(args)), nil
	}))
	asyncGeneratorProto.SetOwnNonEnumerable("throw", vm.NewNativeFunction(1, false, "throw", func(args []vm.Value) (vm.Value, error) {
		return vmInstance.AsyncGeneratorThrow(vmInstance.GetThis(), argOrUndefined(args)), nil
	}))

	// Add AsyncGenerator.prototype[@@toStringTag] = "AsyncGenerator"
	// Per ECMAScript 25.5.1.5: writable: false, enumerable: false, configurable: true
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		agWFalse, agEFalse, agCTrue := false, false, true
		asyncGeneratorProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("AsyncGenerator"),
			&agWFalse, &agEFalse, &agCTrue,
		)
	}

	vmInstance.AsyncGeneratorPrototype = vm.NewValueFromPlainObject(asyncGeneratorProto)

	// Create AsyncGeneratorFunction.prototype (%AsyncGeneratorFunction.prototype%)
	// This is the [[Prototype]] of all async generator functions (async function*)
	// It inherits from Function.prototype and has a .prototype property pointing to AsyncGeneratorPrototype
	asyncGeneratorFunctionProto := vm.NewObject(vmInstance.FunctionPrototype).AsPlainObject()

	// Set the .prototype property to AsyncGeneratorPrototype
	// Per ECMAScript: AsyncGeneratorFunction.prototype.prototype === AsyncGenerator.prototype
	w, e, c := false, false, false // writable=false, enumerable=false, configurable=false
	asyncGeneratorFunctionProto.DefineOwnProperty("prototype", vmInstance.AsyncGeneratorPrototype, &w, &e, &c)
	// AsyncGenerator.prototype.constructor is %AsyncGeneratorFunction.prototype%
	// (27.6.1.1): { writable: false, enumerable: false, configurable: true }.
	ctorW, ctorE, ctorC := false, false, true
	asyncGeneratorProto.DefineOwnProperty("constructor", vm.NewValueFromPlainObject(asyncGeneratorFunctionProto), &ctorW, &ctorE, &ctorC)

	// Add AsyncGeneratorFunction.prototype[@@toStringTag] = "AsyncGeneratorFunction"
	// Per ECMAScript 25.4.3.4: writable: false, enumerable: false, configurable: true
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		wTrue, eTrue, cTrue := false, false, true
		asyncGeneratorFunctionProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("AsyncGeneratorFunction"),
			&wTrue, &eTrue, &cTrue,
		)
	}

	// Store in VM
	vmInstance.AsyncGeneratorFunctionPrototype = vm.NewValueFromPlainObject(asyncGeneratorFunctionProto)
	installDynamicFunctionConstructor(ctx, asyncGeneratorFunctionProto, "async function*", "AsyncGeneratorFunction")

	return nil
}

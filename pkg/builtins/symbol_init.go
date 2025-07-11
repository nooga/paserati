package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"sync"
)

// Global symbol registry for Symbol.for/Symbol.keyFor
var (
	globalSymbolRegistry = make(map[string]vm.Value)
	symbolRegistryMutex  sync.RWMutex
)

// Well-known symbols
var (
	SymbolIterator           vm.Value
	SymbolToStringTag        vm.Value
	SymbolHasInstance        vm.Value
	SymbolToPrimitive        vm.Value
	SymbolIsConcatSpreadable vm.Value
	SymbolSpecies            vm.Value
	SymbolMatch              vm.Value
	SymbolReplace            vm.Value
	SymbolSearch             vm.Value
	SymbolSplit              vm.Value
	SymbolUnscopables        vm.Value
	SymbolAsyncIterator      vm.Value
)

type SymbolInitializer struct{}

func (s *SymbolInitializer) Name() string {
	return "Symbol"
}

func (s *SymbolInitializer) Priority() int {
	return 5 // After Object, Function, Array but before other primitives
}

func (s *SymbolInitializer) InitTypes(ctx *TypeContext) error {
	// Create Symbol constructor type
	symbolCtorType := types.NewObjectType().
		WithProperty("for", types.NewSimpleFunction([]types.Type{types.String}, types.Symbol)).
		WithProperty("keyFor", types.NewSimpleFunction([]types.Type{types.Any}, types.NewUnionType(types.String, types.Undefined))).
		// Well-known symbols
		WithProperty("iterator", types.Symbol).
		WithProperty("toStringTag", types.Symbol).
		WithProperty("hasInstance", types.Symbol).
		WithProperty("toPrimitive", types.Symbol).
		WithProperty("isConcatSpreadable", types.Symbol).
		WithProperty("species", types.Symbol).
		WithProperty("match", types.Symbol).
		WithProperty("replace", types.Symbol).
		WithProperty("search", types.Symbol).
		WithProperty("split", types.Symbol).
		WithProperty("unscopables", types.Symbol).
		WithProperty("asyncIterator", types.Symbol).
		// Symbol constructor signature - returns symbol
		WithSimpleCallSignature([]types.Type{}, types.Symbol).  // Symbol()
		WithSimpleCallSignature(
			[]types.Type{types.NewUnionType(types.String, types.Number, types.Undefined)},
			types.Symbol,
		)  // Symbol(description)

	// Register Symbol constructor
	ctx.DefineGlobal("Symbol", symbolCtorType)

	// Register Symbol primitive type prototype
	symbolProtoType := types.NewObjectType().
		WithProperty("constructor", symbolCtorType).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("valueOf", types.NewSimpleFunction([]types.Type{}, types.Symbol)).
		WithProperty("description", types.NewUnionType(types.String, types.Undefined))

	ctx.SetPrimitivePrototype("symbol", symbolProtoType)

	return nil
}

func (s *SymbolInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Use the VM's Symbol.prototype
	symbolProto := vmInstance.SymbolPrototype.AsPlainObject()

	// Symbol.prototype.toString
	symbolProto.SetOwn("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		// Get 'this' value
		thisVal := vmInstance.GetThis()
		
		// Check if it's a symbol
		if !thisVal.IsSymbol() {
			// Should throw TypeError
			return vm.NewString("Symbol()"), nil
		}
		
		desc := thisVal.AsSymbol()
		if desc == "" {
			return vm.NewString("Symbol()"), nil
		}
		return vm.NewString("Symbol(" + desc + ")"), nil
	}))

	// Symbol.prototype.valueOf
	symbolProto.SetOwn("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		// Get 'this' value
		thisVal := vmInstance.GetThis()
		
		// Check if it's a symbol
		if !thisVal.IsSymbol() {
			// Should throw TypeError
			return vm.Undefined, nil
		}
		
		return thisVal, nil
	}))

	// Create Symbol constructor with properties (like Date does)
	ctorWithProps := vm.NewNativeFunctionWithProps(0, true, "Symbol", func(args []vm.Value) (vm.Value, error) {
		// Get description argument
		var description string
		if len(args) > 0 && args[0].Type() != vm.TypeUndefined {
			description = args[0].ToString()
		}

		// Create new symbol
		return vm.NewSymbol(description), nil
	})

	// Add prototype property - use the VM's SymbolPrototype
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("prototype", vmInstance.SymbolPrototype)

	// Symbol.prototype.constructor
	symbolProto.SetOwn("constructor", ctorWithProps)

	// Initialize well-known symbols
	SymbolIterator = vm.NewSymbol("Symbol.iterator")
	SymbolToStringTag = vm.NewSymbol("Symbol.toStringTag")
	SymbolHasInstance = vm.NewSymbol("Symbol.hasInstance")
	SymbolToPrimitive = vm.NewSymbol("Symbol.toPrimitive")
	SymbolIsConcatSpreadable = vm.NewSymbol("Symbol.isConcatSpreadable")
	SymbolSpecies = vm.NewSymbol("Symbol.species")
	SymbolMatch = vm.NewSymbol("Symbol.match")
	SymbolReplace = vm.NewSymbol("Symbol.replace")
	SymbolSearch = vm.NewSymbol("Symbol.search")
	SymbolSplit = vm.NewSymbol("Symbol.split")
	SymbolUnscopables = vm.NewSymbol("Symbol.unscopables")
	SymbolAsyncIterator = vm.NewSymbol("Symbol.asyncIterator")

	// Add static methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("for", vm.NewNativeFunction(1, false, "for", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			// Should throw TypeError, but return undefined for now
			return vm.Undefined, nil
		}

		key := args[0].ToString()

		symbolRegistryMutex.Lock()
		defer symbolRegistryMutex.Unlock()

		// Check if symbol already exists in registry
		if sym, exists := globalSymbolRegistry[key]; exists {
			return sym, nil
		}

		// Create new symbol and register it
		sym := vm.NewSymbol(key)
		globalSymbolRegistry[key] = sym
		return sym, nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("keyFor", vm.NewNativeFunction(1, false, "keyFor", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 || !args[0].IsSymbol() {
			// Should throw TypeError, but return undefined for now
			return vm.Undefined, nil
		}

		sym := args[0]

		symbolRegistryMutex.RLock()
		defer symbolRegistryMutex.RUnlock()

		// Search for the symbol in the registry
		for key, registeredSym := range globalSymbolRegistry {
			if sym.Is(registeredSym) {
				return vm.NewString(key), nil
			}
		}

		// Symbol not found in registry
		return vm.Undefined, nil
	}))

	// Add well-known symbols as static properties
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("iterator", SymbolIterator)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("toStringTag", SymbolToStringTag)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("hasInstance", SymbolHasInstance)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("toPrimitive", SymbolToPrimitive)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("isConcatSpreadable", SymbolIsConcatSpreadable)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("species", SymbolSpecies)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("match", SymbolMatch)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("replace", SymbolReplace)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("search", SymbolSearch)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("split", SymbolSplit)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("unscopables", SymbolUnscopables)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("asyncIterator", SymbolAsyncIterator)

	symbolCtor := ctorWithProps

	// Register Symbol constructor as global
	return ctx.DefineGlobal("Symbol", symbolCtor)
}
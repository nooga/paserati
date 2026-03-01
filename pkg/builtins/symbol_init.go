package builtins

import (
	"sync"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
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
	SymbolMatchAll           vm.Value
	SymbolReplace            vm.Value
	SymbolSearch             vm.Value
	SymbolSplit              vm.Value
	SymbolUnscopables        vm.Value
	SymbolAsyncIterator      vm.Value
	SymbolDispose            vm.Value
)

type SymbolInitializer struct{}

func (s *SymbolInitializer) Name() string {
	return "Symbol"
}

func (s *SymbolInitializer) Priority() int {
	return 1 // After Object (0) but before Array (2) - symbols needed for iterator protocol
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
		WithProperty("matchAll", types.Symbol).
		WithProperty("replace", types.Symbol).
		WithProperty("search", types.Symbol).
		WithProperty("split", types.Symbol).
		WithProperty("unscopables", types.Symbol).
		WithProperty("asyncIterator", types.Symbol).
		WithProperty("dispose", types.Symbol).
		// Symbol constructor signature - returns symbol
		WithSimpleCallSignature([]types.Type{}, types.Symbol). // Symbol()
		WithSimpleCallSignature(
			[]types.Type{types.NewUnionType(types.String, types.Number, types.Undefined)},
			types.Symbol,
		) // Symbol(description)

	// Register Symbol constructor
	_ = ctx.DefineGlobal("Symbol", symbolCtorType)

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

	// thisSymbolValue extracts the symbol from `this`:
	// - If `this` is a Symbol primitive, return it directly
	// - If `this` is a Symbol wrapper object (Object(sym)), return [[PrimitiveValue]]
	// - Otherwise return error
	thisSymbolValue := func() (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.IsSymbol() {
			return thisVal, nil
		}
		// Check for Symbol wrapper object
		if thisVal.IsObject() {
			po := thisVal.AsPlainObject()
			if po != nil {
				if pv, ok := po.GetOwn("[[PrimitiveValue]]"); ok && pv.IsSymbol() {
					return pv, nil
				}
			}
		}
		return vm.Undefined, vmInstance.NewTypeError("Symbol.prototype.valueOf requires that 'this' be a Symbol")
	}

	// Symbol.prototype.toString - per 20.4.3.3
	symbolProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		sym, err := thisSymbolValue()
		if err != nil {
			return vm.Undefined, err
		}

		desc := sym.AsSymbol()
		if desc == "" {
			return vm.NewString("Symbol()"), nil
		}
		return vm.NewString("Symbol(" + desc + ")"), nil
	}))

	// Symbol.prototype.valueOf - per 20.4.3.4
	symbolProto.SetOwnNonEnumerable("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		sym, err := thisSymbolValue()
		if err != nil {
			return vm.Undefined, err
		}
		return sym, nil
	}))

	// Symbol.prototype.description - accessor property per 20.4.3.2
	{
		eFalse, cTrue := false, true
		descriptionGetter := vm.NewNativeFunction(0, false, "get description", func(args []vm.Value) (vm.Value, error) {
			sym, err := thisSymbolValue()
			if err != nil {
				return vm.Undefined, err
			}
			desc := sym.AsSymbol()
			// Symbol() with no argument has description undefined
			// Symbol("") has description ""
			// We distinguish by checking the SymbolObject's HasDescription field
			symObj := sym.AsSymbolObject()
			if !symObj.HasDescription {
				return vm.Undefined, nil
			}
			return vm.NewString(desc), nil
		})
		symbolProto.DefineAccessorProperty("description", descriptionGetter, true, vm.Undefined, false, &eFalse, &cTrue)
	}

	// Symbol.prototype[Symbol.toPrimitive] - per 20.4.3.5
	// Defined after well-known symbols are initialized (see below)

	// Create Symbol constructor with properties (like Date does)
	ctorWithProps := vm.NewNativeFunctionWithProps(0, true, "Symbol", func(args []vm.Value) (vm.Value, error) {
		// Check if called with new - Symbol should throw when used as constructor
		// Per ECMAScript 19.4.1.1 step 1: "If NewTarget is not undefined, throw a TypeError"
		if vmInstance.IsConstructorCall() {
			return vm.Undefined, vmInstance.NewTypeError("Symbol is not a constructor")
		}

		// Get description argument
		if len(args) == 0 || args[0].Type() == vm.TypeUndefined {
			return vm.NewSymbolNoDescription(), nil
		}

		// Create new symbol with description
		return vm.NewSymbol(args[0].ToString()), nil
	})

	// Mark Symbol as a constructor for class extends validation
	// Per ECMAScript, Symbol has [[Construct]] but throws when invoked
	ctorWithProps.AsNativeFunctionWithProps().IsConstructor = true

	// Add prototype property - use the VM's SymbolPrototype
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vmInstance.SymbolPrototype)

	// Symbol.prototype.constructor
	symbolProto.SetOwnNonEnumerable("constructor", ctorWithProps)

	// Add Symbol.prototype[@@toStringTag] = "Symbol" (writable: false, enumerable: false, configurable: true)
	// Per ECMAScript 20.4.3.6
	if vmInstance.SymbolToStringTag.Type() == vm.TypeSymbol {
		wFalse, eFalse, cTrue := false, false, true
		symbolProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToStringTag),
			vm.NewString("Symbol"),
			&wFalse, &eFalse, &cTrue,
		)
	}

	// Initialize well-known symbols - reuse existing ones if already created
	// This ensures symbols are true singletons across VM resets
	if vmInstance.SymbolIterator.Type() != vm.TypeSymbol {
		// First initialization - create new symbols
		SymbolIterator = vm.NewSymbol("Symbol.iterator")
		SymbolToStringTag = vm.NewSymbol("Symbol.toStringTag")
		SymbolHasInstance = vm.NewSymbol("Symbol.hasInstance")
		SymbolToPrimitive = vm.NewSymbol("Symbol.toPrimitive")
		SymbolIsConcatSpreadable = vm.NewSymbol("Symbol.isConcatSpreadable")
		SymbolSpecies = vm.NewSymbol("Symbol.species")
		SymbolMatch = vm.NewSymbol("Symbol.match")
		SymbolMatchAll = vm.NewSymbol("Symbol.matchAll")
		SymbolReplace = vm.NewSymbol("Symbol.replace")
		SymbolSearch = vm.NewSymbol("Symbol.search")
		SymbolSplit = vm.NewSymbol("Symbol.split")
		SymbolUnscopables = vm.NewSymbol("Symbol.unscopables")
		SymbolAsyncIterator = vm.NewSymbol("Symbol.asyncIterator")
		SymbolDispose = vm.NewSymbol("Symbol.dispose")
	} else {
		// Reuse ALL existing symbols from VM (all are now stored as singletons)
		SymbolIterator = vmInstance.SymbolIterator
		SymbolToStringTag = vmInstance.SymbolToStringTag
		SymbolToPrimitive = vmInstance.SymbolToPrimitive
		SymbolHasInstance = vmInstance.SymbolHasInstance
		SymbolIsConcatSpreadable = vmInstance.SymbolIsConcatSpreadable
		SymbolSpecies = vmInstance.SymbolSpecies
		SymbolMatch = vmInstance.SymbolMatch
		SymbolMatchAll = vmInstance.SymbolMatchAll
		SymbolReplace = vmInstance.SymbolReplace
		SymbolSearch = vmInstance.SymbolSearch
		SymbolSplit = vmInstance.SymbolSplit
		SymbolUnscopables = vmInstance.SymbolUnscopables
		SymbolAsyncIterator = vmInstance.SymbolAsyncIterator
		SymbolDispose = vmInstance.SymbolDispose
	}

	// Add static methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("for", vm.NewNativeFunction(1, false, "for", func(args []vm.Value) (vm.Value, error) {
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

		// Create new registered symbol (cannot be used as WeakMap/WeakSet key per spec)
		sym := vm.NewRegisteredSymbol(key)
		globalSymbolRegistry[key] = sym
		return sym, nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("keyFor", vm.NewNativeFunction(1, false, "keyFor", func(args []vm.Value) (vm.Value, error) {
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
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("iterator", SymbolIterator)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("toStringTag", SymbolToStringTag)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("hasInstance", SymbolHasInstance)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("toPrimitive", SymbolToPrimitive)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("isConcatSpreadable", SymbolIsConcatSpreadable)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("species", SymbolSpecies)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("match", SymbolMatch)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("matchAll", SymbolMatchAll)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("replace", SymbolReplace)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("search", SymbolSearch)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("split", SymbolSplit)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("unscopables", SymbolUnscopables)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("asyncIterator", SymbolAsyncIterator)
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("dispose", SymbolDispose)

	symbolCtor := ctorWithProps

	// Store ALL well-known symbols on VM to ensure they are true singletons
	vmInstance.SymbolIterator = SymbolIterator
	vmInstance.SymbolToPrimitive = SymbolToPrimitive
	vmInstance.SymbolToStringTag = SymbolToStringTag
	vmInstance.SymbolHasInstance = SymbolHasInstance
	vmInstance.SymbolIsConcatSpreadable = SymbolIsConcatSpreadable
	vmInstance.SymbolSpecies = SymbolSpecies
	vmInstance.SymbolMatch = SymbolMatch
	vmInstance.SymbolMatchAll = SymbolMatchAll
	vmInstance.SymbolReplace = SymbolReplace
	vmInstance.SymbolSearch = SymbolSearch
	vmInstance.SymbolSplit = SymbolSplit
	vmInstance.SymbolUnscopables = SymbolUnscopables
	vmInstance.SymbolAsyncIterator = SymbolAsyncIterator
	vmInstance.SymbolDispose = SymbolDispose

	// Symbol.prototype[Symbol.toPrimitive] - per 20.4.3.5
	// Must be defined after well-known symbols are initialized
	if vmInstance.SymbolToPrimitive.Type() == vm.TypeSymbol {
		toPrimitiveFn := vm.NewNativeFunction(1, false, "[Symbol.toPrimitive]", func(args []vm.Value) (vm.Value, error) {
			sym, err := thisSymbolValue()
			if err != nil {
				return vm.Undefined, err
			}
			return sym, nil
		})
		wFalse, eFalse, cTrue := false, false, true
		symbolProto.DefineOwnPropertyByKey(
			vm.NewSymbolKey(vmInstance.SymbolToPrimitive),
			toPrimitiveFn,
			&wFalse, &eFalse, &cTrue,
		)
	}

	// Register Symbol constructor as global
	return ctx.DefineGlobal("Symbol", symbolCtor)
}

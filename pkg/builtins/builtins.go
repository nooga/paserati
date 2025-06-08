package builtins

import (
	"fmt"
	"math"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"sync"
	"time"
)

// BuiltinDefinition holds both the runtime function details and the static type info.
type BuiltinDefinition struct {
	Func *vm.NativeFunctionObject // The runtime representation (name, Go func, arity)
	Type types.Type               // The static type representation
}

// registry holds the mapping from built-in function names to their definitions.
var registry = make(map[string]BuiltinDefinition)

// registryOnce ensures that the registry is populated only once.
var registryOnce sync.Once

// GetFunc retrieves the runtime built-in function details (*vm.NativeFunctionObject) by name.
// Used by the compiler/VM.
func GetFunc(name string) *vm.NativeFunctionObject {
	def, ok := registry[name]
	if !ok {
		return nil // Not found
	}
	return def.Func
}

// GetType retrieves the static type information (types.Type) for a built-in by name.
// Primarily used by the checker? Or potentially GetAllTypes is enough.
func GetType(name string) types.Type {
	def, ok := registry[name]
	if !ok {
		return nil // Not found or type not associated? Return types.Unknown?
	}
	return def.Type
}

// GetAllTypes returns a map of built-in names to their static types (types.Type).
// Used by the checker to populate the initial environment.
func GetAllTypes() map[string]types.Type {
	allTypes := make(map[string]types.Type, len(registry))
	for name, def := range registry {
		if def.Type == nil {
			// This indicates an error during registration
			fmt.Printf("Warning: Builtin '%s' found in registry without a valid type.\n", name)
			// Optionally assign a default/error type (like types.Unknown) or skip
			// allTypes[name] = types.Unknown
			continue
		}
		allTypes[name] = def.Type
	}
	return allTypes
}

// prototypeRegistry holds primitive prototype type information
var prototypeRegistry = make(map[string]map[string]types.Type)

// RegisterPrototypeMethod registers a prototype method type for a primitive
func RegisterPrototypeMethod(primitiveName, methodName string, methodType types.Type) {
	if prototypeRegistry[primitiveName] == nil {
		prototypeRegistry[primitiveName] = make(map[string]types.Type)
	}
	prototypeRegistry[primitiveName][methodName] = methodType
}

// GetPrototypeMethodType returns the type of a prototype method for a primitive
func GetPrototypeMethodType(primitiveName, methodName string) types.Type {
	if methods, exists := prototypeRegistry[primitiveName]; exists {
		return methods[methodName]
	}
	return nil
}

// InitializeRegistry populates the built-in function registry.
func InitializeRegistry() {
	registryOnce.Do(func() {
		// Register clock
		register("clock", 0, false, clockImpl, types.NewSimpleFunction(
			[]types.Type{},
			types.Number,
		))

		// Register Array constructor with overloads
		registerArrayConstructor()

		// Register console object
		registerConsole()

		// Register String constructor and prototype methods
		registerString()

		// Register Array prototype methods
		registerArray()

		// Register Date constructor and methods
		registerDate()

		// Register Math object with methods and constants
		registerMath()

		// Register JSON object with parse and stringify methods
		registerJSON()

		// Register Object constructor
		registerObjectConstructor()

		// Register Function prototype methods
		registerFunction()

		// Register global constants
		registerValue("Infinity", vm.Number(math.Inf(1)), types.Number)
		registerValue("NaN", vm.Number(math.NaN()), types.Number)

		// TODO: Register other built-ins here
	})
}

// --- Built-in Implementations ---

// clockImpl implements the native clock() function.
func clockImpl(args []vm.Value) vm.Value {
	// Arity check is handled by the VM before calling this.
	// clock() takes 0 arguments.
	now := float64(time.Now().UnixNano()) / 1e9 // Seconds since epoch
	return vm.Number(now)
}

// arrayImpl implements the native Array() constructor function.
func arrayImpl(args []vm.Value) vm.Value {
	// Use the new helper function that properly handles Array constructor semantics
	return vm.NewArrayWithArgs(args)
}

// register is a helper to add a built-in function to the registry.
func register(name string, arity int, isVariadic bool, goFunc func([]vm.Value) vm.Value, fnType types.Type) {
	if fnType == nil {
		panic(fmt.Sprintf("Builtin registration for '%s' requires a non-nil Type", name))
	}
	// Check if the type is callable (ObjectType with call signatures)
	if objType, ok := fnType.(*types.ObjectType); ok {
		if !objType.IsCallable() {
			panic(fmt.Sprintf("Builtin registration for '%s': ObjectType must be callable", name))
		}
		// For now, we'll skip detailed variadic validation since ObjectType uses Signature
		// TODO: Add proper validation of signatures if needed
	}
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("Builtin '%s' already registered.", name))
	}
	runtimeFunc := &vm.NativeFunctionObject{
		Arity:    arity,
		Variadic: isVariadic,
		Name:     name,
		Fn:       goFunc,
	}
	registry[name] = BuiltinDefinition{
		Func: runtimeFunc,
		Type: fnType,
	}
}

// registerValue is a helper to add any VM value to the registry with a type
func registerValue(name string, value vm.Value, valueType types.Type) {
	if valueType == nil {
		panic(fmt.Sprintf("Builtin registration for '%s' requires a non-nil type", name))
	}
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("Builtin '%s' already registered.", name))
	}

	// Store the value in objectRegistry
	objectRegistry[name] = value

	// Store the type in registry (with nil Func since it's not a simple function)
	registry[name] = BuiltinDefinition{
		Func: nil,
		Type: valueType,
	}
}

// objectRegistry holds builtin objects (not functions)
var objectRegistry = make(map[string]vm.Value)

// registerObject registers a builtin object
func registerObject(name string, obj vm.Value, objType types.Type) {
	if _, exists := objectRegistry[name]; exists {
		panic(fmt.Sprintf("Builtin object '%s' already registered.", name))
	}
	objectRegistry[name] = obj

	// Also register the type
	registry[name] = BuiltinDefinition{
		Func: nil, // Not a function
		Type: objType,
	}
}

// GetObject retrieves a builtin object by name
func GetObject(name string) vm.Value {
	obj, ok := objectRegistry[name]
	if !ok {
		return vm.Undefined
	}
	return obj
}

// registerArrayConstructor registers the Array constructor with proper overloads
func registerArrayConstructor() {
	// Define the three Array constructor overloads using the new unified ObjectType:
	// 1. Array() - empty array
	// 2. Array(length: number) - array with specific length
	// 3. Array(...items: T[]) - array from rest parameters

	arrayType := &types.ArrayType{ElementType: types.Any}

	// Create overloaded function using the smart constructor
	overloadedArrayType := types.NewObjectType().
		WithCallSignature(types.Sig([]types.Type{}, arrayType)).                   // Array() -> any[]
		WithCallSignature(types.Sig([]types.Type{types.Number}, arrayType)).       // Array(length) -> any[]
		WithCallSignature(types.SigVariadic([]types.Type{}, arrayType, arrayType)) // Array(...items) -> any[]

	// Register the overloaded function
	if _, exists := registry["Array"]; exists {
		panic("Builtin 'Array' already registered.")
	}

	runtimeFunc := &vm.NativeFunctionObject{
		Arity:    -1, // Variadic
		Variadic: true,
		Name:     "Array",
		Fn:       arrayImpl,
	}

	registry["Array"] = BuiltinDefinition{
		Func: runtimeFunc,
		Type: overloadedArrayType,
	}
}

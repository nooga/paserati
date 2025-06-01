package builtins

import (
	"fmt"
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

// InitializeRegistry populates the built-in function registry.
func InitializeRegistry() {
	registryOnce.Do(func() {
		// Register clock
		register("clock", 0, false, clockImpl, &types.FunctionType{
			ParameterTypes: []types.Type{},
			ReturnType:     types.Number,
			IsVariadic:     false,
		})

		// Register Array
		register("Array", -1, true, arrayImpl, &types.FunctionType{
			ParameterTypes: []types.Type{&types.ArrayType{ElementType: types.Any}},
			ReturnType:     &types.ArrayType{ElementType: types.Any},
			IsVariadic:     true,
		})

		// Register console object
		registerConsole()

		// TODO: Register other built-ins here
	})
}

// registerConsole creates and registers the console object with its methods
func registerConsole() {
	// Create the console object as a DictObject
	consoleObj := vm.NewDictObject(vm.Undefined)
	consoleDict := consoleObj.AsDictObject()

	// We need to convert the NativeFunctionObject to a Value
	logValue := vm.NewNativeFunction(-1, true, "log", consoleLogImpl)
	consoleDict.SetOwn("log", logValue)

	// Define the type for console object
	consoleType := &types.ObjectType{
		Properties: map[string]types.Type{
			"log": &types.FunctionType{
				ParameterTypes: []types.Type{&types.ArrayType{ElementType: types.Any}}, // Variadic any[]
				ReturnType:     types.Void,
				IsVariadic:     true,
			},
		},
	}

	// Register the console object
	// Note: Since console is an object (not a function), we can't use the normal register helper
	// We need to register it directly in the registry
	registry["console"] = BuiltinDefinition{
		Func: nil, // Console is not a function itself
		Type: consoleType,
	}

	// We also need a way for the VM to get the console object
	// For now, let's create a special entry point for objects
	registerObject("console", consoleObj, consoleType)
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

// consoleLogImpl implements console.log(...args)
func consoleLogImpl(args []vm.Value) vm.Value {
	// Convert all arguments to strings and print them separated by spaces
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = arg.Inspect() // Use Inspect() for better formatting (like Node.js console.log)
	}

	// Print with space separation, followed by newline
	if len(parts) > 0 {
		for i, part := range parts {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(part)
		}
	}
	fmt.Println()

	// console.log returns undefined
	return vm.Undefined
}

// register is a helper to add a built-in function to the registry.
func register(name string, arity int, isVariadic bool, goFunc func([]vm.Value) vm.Value, fnType *types.FunctionType) {
	if fnType == nil {
		panic(fmt.Sprintf("Builtin registration for '%s' requires a non-nil FunctionType", name))
	}
	if fnType.IsVariadic != isVariadic {
		panic(fmt.Sprintf("Builtin registration mismatch for '%s': isVariadic flag (%t) != FunctionType.IsVariadic (%t)", name, isVariadic, fnType.IsVariadic))
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

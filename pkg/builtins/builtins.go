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
	Func *vm.BuiltinFunc // The runtime representation (name, Go func, arity)
	Type types.Type      // The static type representation
}

// registry holds the mapping from built-in function names to their definitions.
var registry = make(map[string]BuiltinDefinition)

// registryOnce ensures that the registry is populated only once.
var registryOnce sync.Once

// GetFunc retrieves the runtime built-in function details (*vm.BuiltinFunc) by name.
// Used by the compiler/VM.
func GetFunc(name string) *vm.BuiltinFunc {
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

		// TODO: Register other built-ins here
	})
}

// --- Built-in Implementations ---

// clockImpl implements the native clock() function.
func clockImpl(args []vm.Value) (vm.Value, error) {
	// Arity check is handled by the VM before calling this.
	// clock() takes 0 arguments.
	now := float64(time.Now().UnixNano()) / 1e9 // Seconds since epoch
	return vm.Number(now), nil
}

// arrayImpl implements the native Array() constructor function.
func arrayImpl(args []vm.Value) (vm.Value, error) {
	argCount := len(args)

	switch argCount {
	case 0:
		// Array() -> []
		return vm.NewArray(make([]vm.Value, 0)), nil
	case 1:
		// Array(N) -> [undefined, undefined, ..., undefined] (N times)
		firstArg := args[0]
		if firstArg.Type != vm.TypeNumber {
			// JS Array(nonNumber) -> [nonNumber]
			// Let's mimic this behavior for consistency, although it's a bit weird.
			// Alternatively, we could throw a TypeError.
			// For now, let's match JS:
			len := int(vm.AsNumber(firstArg))
			if len < 0 {
				return vm.Undefined(), fmt.Errorf("invalid array length: %.f", vm.AsNumber(firstArg))
			}
			elements := make([]vm.Value, len)
			for i := 0; i < len; i++ {
				elements[i] = vm.Undefined()
			}
			return vm.NewArray(elements), nil
			// return nil, fmt.Errorf("single argument to Array must be a non-negative integer number, got %s", firstArg.TypeName())
		}

		num := vm.AsNumber(firstArg)
		// Check if it's a non-negative integer
		if num < 0 || math.IsNaN(num) || math.IsInf(num, 0) || math.Floor(num) != num {
			// JS throws RangeError
			return vm.Undefined(), fmt.Errorf("invalid array length: %.f", num) // Use Undefined() as value with error
		}
		length := int(num) // Safe to convert after checks

		// Check potential excessive length
		// Define a reasonable max length if needed, e.g., 1 << 24
		// const maxArrayLength = 1 << 24
		// if length > maxArrayLength {
		//     return vm.Undefined(), fmt.Errorf("array length %d exceeds maximum allowed", length)
		// }

		elements := make([]vm.Value, length)
		// Fill with undefined (already done by make for []vm.Value if Undefined is zero value)
		// Explicitly set if Undefined() is not the zero value:
		for i := 0; i < length; i++ {
			elements[i] = vm.Undefined()
		}
		return vm.NewArray(elements), nil

	default: // > 1 arguments
		// Array(a, b, c) -> [a, b, c]
		// We can directly use the args slice passed in.
		// Make a copy to be safe, although maybe not strictly necessary if args isn't reused.
		elements := make([]vm.Value, argCount)
		copy(elements, args)
		return vm.NewArray(elements), nil
	}
}

// register is a helper to add a built-in function to the registry.
func register(name string, arity int, isVariadic bool, goFunc func([]vm.Value) (vm.Value, error), fnType *types.FunctionType) {
	if fnType == nil {
		panic(fmt.Sprintf("Builtin registration for '%s' requires a non-nil FunctionType", name))
	}
	// Ensure the provided type's variadic status matches the flag
	if fnType.IsVariadic != isVariadic {
		panic(fmt.Sprintf("Builtin registration mismatch for '%s': isVariadic flag (%t) != FunctionType.IsVariadic (%t)", name, isVariadic, fnType.IsVariadic))
	}

	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("Builtin '%s' already registered.", name))
	}

	// Create the runtime object (without the type info)
	runtimeFunc := &vm.BuiltinFunc{
		Name:  name,
		Func:  goFunc,
		Arity: arity,
		// No Type field here!
	}

	// Store both parts in the registry definition struct
	registry[name] = BuiltinDefinition{
		Func: runtimeFunc,
		Type: fnType,
	}

	// fmt.Printf("Registered builtin '%s' with Type: %s\n", name, fnType.String())
}

package builtins

import "sort"

// GetStandardInitializers returns all built-in initializers sorted by priority
func GetStandardInitializers() []BuiltinInitializer {
	var initializers []BuiltinInitializer
	
	// Global constants and backward compatibility functions
	initializers = append(initializers, &GlobalsInitializer{})
	
	// Utility types (Readonly<T>, etc.)
	initializers = append(initializers, &UtilityTypesInitializer{})
	
	// Core builtins
	initializers = append(initializers, &ObjectInitializer{})
	initializers = append(initializers, &FunctionInitializer{})
	initializers = append(initializers, &ArrayInitializer{})
	initializers = append(initializers, &ArgumentsInitializer{})
	initializers = append(initializers, &GeneratorInitializer{})
	
	// Additional builtins (to be migrated)
	initializers = append(initializers, &StringInitializer{})
	initializers = append(initializers, &NumberInitializer{})
	initializers = append(initializers, &BigIntInitializer{})
	initializers = append(initializers, &SymbolInitializer{})
	// initializers = append(initializers, &BooleanInitializer{})
	initializers = append(initializers, &MapInitializer{})
	initializers = append(initializers, &SetInitializer{})
	initializers = append(initializers, &RegExpInitializer{})
	initializers = append(initializers, &ErrorInitializer{})
	initializers = append(initializers, &TypeErrorInitializer{})
	initializers = append(initializers, &ReferenceErrorInitializer{})
	initializers = append(initializers, &SyntaxErrorInitializer{})
	initializers = append(initializers, &MathInitializer{})
	initializers = append(initializers, &JSONInitializer{})
	initializers = append(initializers, &ConsoleInitializer{})
	initializers = append(initializers, &DateInitializer{})
	initializers = append(initializers, &ArrayBufferInitializer{})
	initializers = append(initializers, &Uint8ArrayInitializer{})
	initializers = append(initializers, &Uint8ClampedArrayInitializer{})
	initializers = append(initializers, &Uint16ArrayInitializer{})
	initializers = append(initializers, &Int32ArrayInitializer{})
	initializers = append(initializers, &Float32ArrayInitializer{})
	initializers = append(initializers, &Float64ArrayInitializer{})
	
	// Sort by priority (lower numbers first)
	sort.Slice(initializers, func(i, j int) bool {
		return initializers[i].Priority() < initializers[j].Priority()
	})
	
	return initializers
}
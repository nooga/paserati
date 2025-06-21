package builtins

import "sort"

// GetStandardInitializers returns all built-in initializers sorted by priority
func GetStandardInitializers() []BuiltinInitializer {
	var initializers []BuiltinInitializer
	
	// Global constants and backward compatibility functions
	initializers = append(initializers, &GlobalsInitializer{})
	
	// Core builtins
	initializers = append(initializers, &ObjectInitializer{})
	initializers = append(initializers, &FunctionInitializer{})
	initializers = append(initializers, &ArrayInitializer{})
	
	// Additional builtins (to be migrated)
	initializers = append(initializers, &StringInitializer{})
	// initializers = append(initializers, &NumberInitializer{})
	// initializers = append(initializers, &BooleanInitializer{})
	initializers = append(initializers, &ErrorInitializer{})
	initializers = append(initializers, &MathInitializer{})
	initializers = append(initializers, &JSONInitializer{})
	initializers = append(initializers, &ConsoleInitializer{})
	initializers = append(initializers, &DateInitializer{})
	
	// Sort by priority (lower numbers first)
	sort.Slice(initializers, func(i, j int) bool {
		return initializers[i].Priority() < initializers[j].Priority()
	})
	
	return initializers
}
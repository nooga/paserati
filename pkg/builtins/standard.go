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
	initializers = append(initializers, &AsyncGeneratorInitializer{})
	initializers = append(initializers, &PromiseInitializer{})

	// Iterator protocol types
	initializers = append(initializers, &IteratorInitializer{})

	// Additional builtins (to be migrated)
	initializers = append(initializers, &StringInitializer{})
	initializers = append(initializers, &NumberInitializer{})
	initializers = append(initializers, &BigIntInitializer{})
	initializers = append(initializers, &SymbolInitializer{})
	initializers = append(initializers, &BooleanInitializer{})
	initializers = append(initializers, &MapInitializer{})
	initializers = append(initializers, &SetInitializer{})
	initializers = append(initializers, &WeakMapInitializer{})
	initializers = append(initializers, &WeakSetInitializer{})
	initializers = append(initializers, &WeakRefInitializer{})
	initializers = append(initializers, &RegExpInitializer{})
	initializers = append(initializers, &ErrorInitializer{})
	initializers = append(initializers, &TypeErrorInitializer{})
	initializers = append(initializers, &ReferenceErrorInitializer{})
	initializers = append(initializers, &SyntaxErrorInitializer{})
	// Minimal stubs for remaining native Error subclasses used by the harness
	initializers = append(initializers, &EvalErrorInitializer{})
	initializers = append(initializers, &RangeErrorInitializer{})
	initializers = append(initializers, &URIErrorInitializer{})
	initializers = append(initializers, &AggregateErrorInitializer{})
	initializers = append(initializers, &MathInitializer{})
	initializers = append(initializers, &JSONInitializer{})
	// Install Reflect after Object so it can delegate to Object.__ownKeys
	initializers = append(initializers, &ReflectInitializer{})
	initializers = append(initializers, &ProxyInitializer{})
	initializers = append(initializers, &ConsoleInitializer{})
	initializers = append(initializers, &BlobInitializer{})
	initializers = append(initializers, &FormDataInitializer{})
	initializers = append(initializers, &AbortControllerInitializer{})
	initializers = append(initializers, &FetchInitializer{})
	initializers = append(initializers, &DateInitializer{})
	initializers = append(initializers, &TemporalInitializer{})
	initializers = append(initializers, &PerformanceInitializer{})
	initializers = append(initializers, &ArrayBufferInitializer{})
	initializers = append(initializers, &SharedArrayBufferInitializer{})
	initializers = append(initializers, &DataViewInitializer{})
	initializers = append(initializers, &TypedArrayInitializer{}) // Abstract TypedArray base - must come before specific TypedArrays
	initializers = append(initializers, &Uint8ArrayInitializer{})
	initializers = append(initializers, &Uint8ClampedArrayInitializer{})
	initializers = append(initializers, &Uint16ArrayInitializer{})
	initializers = append(initializers, &Int8ArrayInitializer{})
	initializers = append(initializers, &Int16ArrayInitializer{})
	initializers = append(initializers, &Uint32ArrayInitializer{})
	initializers = append(initializers, &Int32ArrayInitializer{})
	initializers = append(initializers, &Float32ArrayInitializer{})
	initializers = append(initializers, &Float64ArrayInitializer{})
	initializers = append(initializers, &BigInt64ArrayInitializer{})
	initializers = append(initializers, &BigUint64ArrayInitializer{})
	initializers = append(initializers, &AtomicsInitializer{})

	// Paserati intrinsics (compile-time type reflection)
	initializers = append(initializers, &PaseratiInitializer{})

	// Sort by priority (lower numbers first)
	sort.Slice(initializers, func(i, j int) bool {
		return initializers[i].Priority() < initializers[j].Priority()
	})

	return initializers
}

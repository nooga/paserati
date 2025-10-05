package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// BuiltinInitializer is implemented by each builtin module
type BuiltinInitializer interface {
	// Name returns the module name (e.g., "Array", "String", "Math")
	Name() string

	// Priority returns initialization order (lower = earlier)
	Priority() int

	// InitTypes creates type definitions for the checker
	InitTypes(ctx *TypeContext) error

	// InitRuntime creates runtime values for the VM
	InitRuntime(ctx *RuntimeContext) error
}

// TypeContext provides everything needed for type initialization
type TypeContext struct {
	// Define a global type (constructor, namespace, etc.)
	DefineGlobal func(name string, typ types.Type) error

	// Define a type alias (e.g., "number" -> types.Number)
	DefineTypeAlias func(name string, typ types.Type) error

	// Get a previously defined type
	GetType func(name string) (types.Type, bool)

	// Store prototype types for primitives (for checker's getBuiltinType)
	SetPrimitivePrototype func(primitiveName string, prototypeType *types.ObjectType)
}

// RuntimeContext provides everything needed for runtime initialization
type RuntimeContext struct {
	// The VM instance
	VM *vm.VM

	// Define a global value
	DefineGlobal func(name string, value vm.Value) error

	// Get built-in prototypes (set as initializers run)
	ObjectPrototype   vm.Value
	FunctionPrototype vm.Value
	ArrayPrototype    vm.Value
}

// Priority constants for initialization order
const (
	PriorityObject    = 0   // Object must be first (base prototype)
	PriorityFunction  = 1   // Function second (inherits from Object)
	PriorityIterator  = 2   // Iterator types (needed for iterables)
	PriorityArray          = 3  // Array third (inherits from Object, implements Iterable)
	PriorityArguments      = 4  // Arguments object (array-like)
	PriorityGenerator      = 5  // Generator objects (inherits from Object, implements Iterable)
	PriorityAsyncGenerator = 6  // AsyncGenerator objects (like Generator but returns Promises)
	PriorityString         = 10 // String primitives
	PriorityNumber    = 11  // Number primitives
	PriorityBoolean   = 12  // Boolean primitives
	PriorityRegExp    = 13  // RegExp constructor
	PriorityMath      = 100 // Math object
	PriorityJSON      = 101 // JSON object
	PriorityConsole   = 102 // Console object
	PriorityDate      = 103 // Date constructor
)

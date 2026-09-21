package vm

// Realm represents an isolated JavaScript execution environment.
// Each realm has its own global object, built-in prototypes, and intrinsics.
// This is the foundation for ECMAScript Realm support and ShadowRealm API.
type Realm struct {
	// Identity
	id int // Unique realm identifier

	// Global environment
	GlobalObject            *PlainObject
	Heap                    *Heap
	globalsFromGlobalObject map[uint16]bool // Track globals read from GlobalObject

	// Built-in prototypes
	ObjectPrototype               Value
	FunctionPrototype             Value
	ArrayPrototype                Value
	StringPrototype               Value
	NumberPrototype               Value
	BigIntPrototype               Value
	BooleanPrototype              Value
	RegExpPrototype               Value
	MapPrototype                  Value
	SetPrototype                  Value
	WeakMapPrototype              Value
	WeakSetPrototype              Value
	WeakRefPrototype              Value
	FinalizationRegistryPrototype Value
	IntlSegmenterPrototype        Value // %Intl.Segmenter.prototype%
	GeneratorPrototype            Value
	AsyncGeneratorPrototype       Value
	IteratorPrototype             Value // %Iterator.prototype% - base for all iterators
	IteratorHelperPrototype       Value // %IteratorHelperPrototype% - for iterator helper objects
	WrapForValidIteratorPrototype Value // For Iterator.from() wrapped iterators
	ArrayIteratorPrototype        Value
	MapIteratorPrototype          Value
	SetIteratorPrototype          Value
	StringIteratorPrototype       Value
	RegExpStringIteratorPrototype Value
	PromisePrototype              Value
	ErrorPrototype                Value
	TypeErrorPrototype            Value
	ReferenceErrorPrototype       Value
	SyntaxErrorPrototype          Value
	RangeErrorPrototype           Value
	URIErrorPrototype             Value
	EvalErrorPrototype            Value
	AggregateErrorPrototype       Value
	SuppressedErrorPrototype      Value
	DisposableStackPrototype      Value
	AsyncDisposableStackPrototype Value
	SymbolPrototype               Value
	DatePrototype                 Value

	// TypedArray prototypes
	TypedArrayPrototype        Value // Abstract %TypedArray%.prototype
	Uint8ArrayPrototype        Value
	Uint8ClampedArrayPrototype Value
	Int8ArrayPrototype         Value
	Int16ArrayPrototype        Value
	Uint16ArrayPrototype       Value
	Uint32ArrayPrototype       Value
	Int32ArrayPrototype        Value
	Float16ArrayPrototype      Value
	Float32ArrayPrototype      Value
	Float64ArrayPrototype      Value
	BigInt64ArrayPrototype     Value
	BigUint64ArrayPrototype    Value
	ArrayBufferPrototype       Value
	SharedArrayBufferPrototype Value
	DataViewPrototype          Value

	// Function-related prototypes
	AsyncFunctionPrototype          Value
	GeneratorFunctionPrototype      Value // %GeneratorFunction.prototype%
	AsyncGeneratorFunctionPrototype Value // %AsyncGeneratorFunction.prototype%

	// Constructors (cached for instanceof checks and error creation)
	ErrorConstructor         Value
	TypedArrayConstructor    Value // Abstract %TypedArray% constructor
	AsyncFunctionConstructor Value
	ArrayConstructor         Value
	ObjectConstructor        Value
	FunctionConstructor      Value

	// Well-known symbols
	SymbolIterator           Value
	SymbolToPrimitive        Value
	SymbolToStringTag        Value
	SymbolHasInstance        Value
	SymbolIsConcatSpreadable Value
	SymbolSpecies            Value
	SymbolMatch              Value
	SymbolMatchAll           Value
	SymbolReplace            Value
	SymbolSearch             Value
	SymbolSplit              Value
	SymbolUnscopables        Value
	SymbolAsyncIterator      Value
	SymbolDispose            Value
	SymbolAsyncDispose       Value

	// Symbol registry for Symbol.for()
	SymbolRegistry map[string]Value

	// Intrinsic functions
	ThrowTypeErrorFunc Value // %ThrowTypeError% - for strict mode arguments.callee/caller

	// Module system (per-realm)
	ModuleContexts map[string]*ModuleContext

	// Parent VM reference
	vm *VM

	// Initialization state
	initialized bool
}

// NewRealm creates a new realm with uninitialized prototypes.
// Call InitializePrototypes() and InitializeSymbols() to set up built-ins.
func NewRealm(vm *VM, id int) *Realm {
	return &Realm{
		id:                      id,
		vm:                      vm,
		Heap:                    NewHeap(64),
		SymbolRegistry:          make(map[string]Value),
		ModuleContexts:          make(map[string]*ModuleContext),
		globalsFromGlobalObject: make(map[uint16]bool),
	}
}

// ID returns the unique identifier for this realm.
func (r *Realm) ID() int {
	return r.id
}

// VM returns the parent VM for this realm.
func (r *Realm) VM() *VM {
	return r.vm
}

// InitializePrototypes creates the prototype chain for this realm.
// This sets up the inheritance hierarchy for all built-in types.
func (r *Realm) InitializePrototypes() {
	// Object.prototype is the root (inherits from null)
	r.ObjectPrototype = NewObject(Null)

	// Core prototypes inherit from Object.prototype
	r.FunctionPrototype = NewObject(r.ObjectPrototype)
	r.ArrayPrototype = NewObject(r.ObjectPrototype)
	r.StringPrototype = NewObject(r.ObjectPrototype)
	r.NumberPrototype = NewObject(r.ObjectPrototype)
	r.BigIntPrototype = NewObject(r.ObjectPrototype)
	r.BooleanPrototype = NewObject(r.ObjectPrototype)
	r.SymbolPrototype = NewObject(r.ObjectPrototype)
	r.RegExpPrototype = NewObject(r.ObjectPrototype)
	r.DatePrototype = NewObject(r.ObjectPrototype)
	r.MapPrototype = NewObject(r.ObjectPrototype)
	r.SetPrototype = NewObject(r.ObjectPrototype)
	r.WeakMapPrototype = NewObject(r.ObjectPrototype)
	r.WeakSetPrototype = NewObject(r.ObjectPrototype)
	r.WeakRefPrototype = NewObject(r.ObjectPrototype)
	r.FinalizationRegistryPrototype = NewObject(r.ObjectPrototype)
	r.IntlSegmenterPrototype = NewObject(r.ObjectPrototype)
	r.PromisePrototype = NewObject(r.ObjectPrototype)
	r.ArrayBufferPrototype = NewObject(r.ObjectPrototype)
	r.SharedArrayBufferPrototype = NewObject(r.ObjectPrototype)
	r.DataViewPrototype = NewObject(r.ObjectPrototype)

	// Error prototypes
	r.ErrorPrototype = NewObject(r.ObjectPrototype)
	r.TypeErrorPrototype = NewObject(r.ErrorPrototype)
	r.ReferenceErrorPrototype = NewObject(r.ErrorPrototype)
	r.SyntaxErrorPrototype = NewObject(r.ErrorPrototype)
	r.RangeErrorPrototype = NewObject(r.ErrorPrototype)
	r.URIErrorPrototype = NewObject(r.ErrorPrototype)
	r.EvalErrorPrototype = NewObject(r.ErrorPrototype)
	r.AggregateErrorPrototype = NewObject(r.ErrorPrototype)
	r.SuppressedErrorPrototype = NewObject(r.ErrorPrototype)
	r.DisposableStackPrototype = NewObject(r.ObjectPrototype)
	r.AsyncDisposableStackPrototype = NewObject(r.ObjectPrototype)

	// TypedArray prototypes - inherit from Object.prototype
	// (TypedArrayPrototype is set up later by initializers)
	r.TypedArrayPrototype = NewObject(r.ObjectPrototype)
	r.Uint8ArrayPrototype = NewObject(r.TypedArrayPrototype)
	r.Uint8ClampedArrayPrototype = NewObject(r.TypedArrayPrototype)
	r.Int8ArrayPrototype = NewObject(r.TypedArrayPrototype)
	r.Int16ArrayPrototype = NewObject(r.TypedArrayPrototype)
	r.Uint16ArrayPrototype = NewObject(r.TypedArrayPrototype)
	r.Uint32ArrayPrototype = NewObject(r.TypedArrayPrototype)
	r.Int32ArrayPrototype = NewObject(r.TypedArrayPrototype)
	r.Float16ArrayPrototype = NewObject(r.TypedArrayPrototype)
	r.Float32ArrayPrototype = NewObject(r.TypedArrayPrototype)
	r.Float64ArrayPrototype = NewObject(r.TypedArrayPrototype)
	r.BigInt64ArrayPrototype = NewObject(r.TypedArrayPrototype)
	r.BigUint64ArrayPrototype = NewObject(r.TypedArrayPrototype)

	// Iterator prototypes
	r.IteratorPrototype = NewObject(r.ObjectPrototype)
	r.IteratorHelperPrototype = NewObject(r.IteratorPrototype)
	r.WrapForValidIteratorPrototype = NewObject(r.IteratorPrototype)
	r.ArrayIteratorPrototype = NewObject(r.IteratorPrototype)
	r.MapIteratorPrototype = NewObject(r.IteratorPrototype)
	r.SetIteratorPrototype = NewObject(r.IteratorPrototype)
	r.StringIteratorPrototype = NewObject(r.IteratorPrototype)
	r.RegExpStringIteratorPrototype = NewObject(r.IteratorPrototype)

	// Generator prototypes
	r.GeneratorPrototype = NewObject(r.IteratorPrototype)
	r.GeneratorFunctionPrototype = NewObject(r.FunctionPrototype)
	r.AsyncGeneratorPrototype = NewObject(r.ObjectPrototype)
	r.AsyncGeneratorFunctionPrototype = NewObject(r.FunctionPrototype)
	r.AsyncFunctionPrototype = NewObject(r.FunctionPrototype)

	// Create global object with ObjectPrototype in chain
	r.GlobalObject = NewObject(r.ObjectPrototype).AsPlainObject()
}

// InitializeSymbols gives this realm the VM's single canonical set of
// well-known symbols (see VM.ensureWellKnownSymbols) instead of minting
// its own. Per ECMA-262 6.1.5.1, well-known symbols are shared by every
// Realm of the same agent - a second Realm (e.g. one spun up by a host's
// `vm.createContext()`) must resolve Symbol.replace, Symbol.iterator, etc.
// to the exact same Symbol values the first Realm uses, or property
// lookups keyed by one Realm's copy silently miss on an object whose
// method was registered under another Realm's differently-identitied copy
// (paserati#486: vm.createContext() permanently broke String.prototype.replace
// process-wide because each Realm used to get its own distinct
// Symbol.replace).
func (r *Realm) InitializeSymbols() {
	r.vm.ensureWellKnownSymbols()
	r.SymbolIterator = r.vm.SymbolIterator
	r.SymbolToPrimitive = r.vm.SymbolToPrimitive
	r.SymbolToStringTag = r.vm.SymbolToStringTag
	r.SymbolHasInstance = r.vm.SymbolHasInstance
	r.SymbolIsConcatSpreadable = r.vm.SymbolIsConcatSpreadable
	r.SymbolSpecies = r.vm.SymbolSpecies
	r.SymbolMatch = r.vm.SymbolMatch
	r.SymbolMatchAll = r.vm.SymbolMatchAll
	r.SymbolReplace = r.vm.SymbolReplace
	r.SymbolSearch = r.vm.SymbolSearch
	r.SymbolSplit = r.vm.SymbolSplit
	r.SymbolUnscopables = r.vm.SymbolUnscopables
	r.SymbolAsyncIterator = r.vm.SymbolAsyncIterator
	r.SymbolDispose = r.vm.SymbolDispose
	r.SymbolAsyncDispose = r.vm.SymbolAsyncDispose
}

// GetGlobal retrieves a global variable by name from this realm.
func (r *Realm) GetGlobal(name string) (Value, bool) {
	if r.Heap.nameToIndex == nil {
		return Undefined, false
	}
	index, exists := r.Heap.nameToIndex[name]
	if !exists {
		return Undefined, false
	}
	return r.Heap.Get(index)
}

// SetGlobal sets a global variable in this realm.
func (r *Realm) SetGlobal(name string, value Value) {
	if r.Heap.nameToIndex == nil {
		r.Heap.nameToIndex = make(map[string]int)
	}
	index, exists := r.Heap.nameToIndex[name]
	if !exists {
		// Allocate a new index
		index = r.Heap.Size()
		r.Heap.nameToIndex[name] = index
	}
	_ = r.Heap.Set(index, value)
}

// DefineGlobal defines a new global in this realm (used by initializers).
func (r *Realm) DefineGlobal(name string, value Value) error {
	r.SetGlobal(name, value)
	return nil
}

// MarkInitialized marks this realm as fully initialized.
func (r *Realm) MarkInitialized() {
	r.initialized = true
}

// IsInitialized returns true if this realm has been fully initialized.
func (r *Realm) IsInitialized() bool {
	return r.initialized
}

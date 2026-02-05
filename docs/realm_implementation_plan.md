# Realm Infrastructure Implementation Plan

## Overview

This document outlines the plan to add JavaScript Realm support to Paserati. Realms are isolated execution environments with their own global object and built-in intrinsics.

### Goals
1. Enable proper realm isolation for ECMAScript compliance
2. Unlock ~200 cross-realm Test262 tests
3. Prepare foundation for ShadowRealm API (Stage 3)

### Non-Goals (for now)
- ShadowRealm user-facing API (separate follow-up)
- Cross-realm callable boundary wrapping
- Realm-based security isolation

---

## Phase 1: Realm Container Type

**Goal**: Define the Realm struct and move prototype/symbol storage from VM to Realm.

### 1.1 Create `pkg/vm/realm.go`

```go
package vm

// Realm represents an isolated JavaScript execution environment.
// Each realm has its own global object, built-in prototypes, and intrinsics.
type Realm struct {
    // Identity
    id int // Unique realm identifier

    // Global environment
    GlobalObject *PlainObject
    Heap         *Heap

    // Built-in prototypes (29 total)
    ObjectPrototype   Value
    FunctionPrototype Value
    ArrayPrototype    Value
    StringPrototype   Value
    NumberPrototype   Value
    BooleanPrototype  Value
    ErrorPrototype    Value
    TypeErrorPrototype     Value
    ReferenceErrorPrototype Value
    SyntaxErrorPrototype   Value
    RangeErrorPrototype    Value
    URIErrorPrototype      Value
    EvalErrorPrototype     Value
    RegExpPrototype   Value
    DatePrototype     Value
    MapPrototype      Value
    SetPrototype      Value
    WeakMapPrototype  Value
    WeakSetPrototype  Value
    PromisePrototype  Value
    SymbolPrototype   Value
    ArrayBufferPrototype      Value
    SharedArrayBufferPrototype Value
    DataViewPrototype Value
    TypedArrayPrototype Value
    // Individual TypedArray prototypes
    Uint8ArrayPrototype    Value
    Int8ArrayPrototype     Value
    Uint16ArrayPrototype   Value
    Int16ArrayPrototype    Value
    Uint32ArrayPrototype   Value
    Int32ArrayPrototype    Value
    Float32ArrayPrototype  Value
    Float64ArrayPrototype  Value
    BigInt64ArrayPrototype Value
    BigUint64ArrayPrototype Value
    Uint8ClampedArrayPrototype Value

    // Iterator prototypes
    IteratorPrototype       Value
    ArrayIteratorPrototype  Value
    MapIteratorPrototype    Value
    SetIteratorPrototype    Value
    StringIteratorPrototype Value
    RegExpStringIteratorPrototype Value

    // Generator/Async prototypes
    GeneratorPrototype         Value
    GeneratorFunctionPrototype Value
    AsyncGeneratorPrototype    Value
    AsyncGeneratorFunctionPrototype Value
    AsyncFunctionPrototype     Value

    // Well-known symbols (13)
    SymbolIterator       Value
    SymbolToStringTag    Value
    SymbolToPrimitive    Value
    SymbolHasInstance    Value
    SymbolIsConcatSpreadable Value
    SymbolMatch          Value
    SymbolMatchAll       Value
    SymbolReplace        Value
    SymbolSearch         Value
    SymbolSplit          Value
    SymbolSpecies        Value
    SymbolUnscopables    Value
    SymbolAsyncIterator  Value

    // Symbol registry for Symbol.for()
    SymbolRegistry map[string]Value

    // Intrinsic functions
    ThrowTypeErrorFunc Value
    ErrorConstructor   Value

    // Cached constructors (for instanceof checks, etc.)
    ArrayConstructor   Value
    ObjectConstructor  Value
    FunctionConstructor Value

    // Module system (per-realm)
    ModuleContexts map[string]*ModuleContext

    // Parent VM reference
    vm *VM
}

// NewRealm creates a new realm with uninitialized prototypes.
// Call InitializeRealm() to set up built-ins.
func NewRealm(vm *VM) *Realm {
    return &Realm{
        id:             vm.nextRealmID(),
        vm:             vm,
        Heap:           NewHeap(64),
        SymbolRegistry: make(map[string]Value),
        ModuleContexts: make(map[string]*ModuleContext),
    }
}

// InitializePrototypes creates the prototype chain for this realm.
func (r *Realm) InitializePrototypes() {
    // Object.prototype is the root (inherits from null)
    r.ObjectPrototype = NewObject(Null)

    // Core prototypes inherit from Object.prototype
    r.FunctionPrototype = NewObject(r.ObjectPrototype)
    r.ArrayPrototype = NewObject(r.ObjectPrototype)
    r.StringPrototype = NewObject(r.ObjectPrototype)
    r.NumberPrototype = NewObject(r.ObjectPrototype)
    r.BooleanPrototype = NewObject(r.ObjectPrototype)
    r.SymbolPrototype = NewObject(r.ObjectPrototype)
    r.RegExpPrototype = NewObject(r.ObjectPrototype)
    r.DatePrototype = NewObject(r.ObjectPrototype)
    r.MapPrototype = NewObject(r.ObjectPrototype)
    r.SetPrototype = NewObject(r.ObjectPrototype)
    r.WeakMapPrototype = NewObject(r.ObjectPrototype)
    r.WeakSetPrototype = NewObject(r.ObjectPrototype)
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

    // TypedArray prototypes
    r.TypedArrayPrototype = NewObject(r.ObjectPrototype)
    r.Uint8ArrayPrototype = NewObject(r.TypedArrayPrototype)
    r.Int8ArrayPrototype = NewObject(r.TypedArrayPrototype)
    r.Uint16ArrayPrototype = NewObject(r.TypedArrayPrototype)
    r.Int16ArrayPrototype = NewObject(r.TypedArrayPrototype)
    r.Uint32ArrayPrototype = NewObject(r.TypedArrayPrototype)
    r.Int32ArrayPrototype = NewObject(r.TypedArrayPrototype)
    r.Float32ArrayPrototype = NewObject(r.TypedArrayPrototype)
    r.Float64ArrayPrototype = NewObject(r.TypedArrayPrototype)
    r.BigInt64ArrayPrototype = NewObject(r.TypedArrayPrototype)
    r.BigUint64ArrayPrototype = NewObject(r.TypedArrayPrototype)
    r.Uint8ClampedArrayPrototype = NewObject(r.TypedArrayPrototype)

    // Iterator prototypes
    r.IteratorPrototype = NewObject(r.ObjectPrototype)
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

    // Create global object
    r.GlobalObject = NewObject(r.ObjectPrototype).AsPlainObject()
}

// InitializeSymbols creates well-known symbols for this realm.
func (r *Realm) InitializeSymbols() {
    r.SymbolIterator = NewSymbol("Symbol.iterator")
    r.SymbolToStringTag = NewSymbol("Symbol.toStringTag")
    r.SymbolToPrimitive = NewSymbol("Symbol.toPrimitive")
    r.SymbolHasInstance = NewSymbol("Symbol.hasInstance")
    r.SymbolIsConcatSpreadable = NewSymbol("Symbol.isConcatSpreadable")
    r.SymbolMatch = NewSymbol("Symbol.match")
    r.SymbolMatchAll = NewSymbol("Symbol.matchAll")
    r.SymbolReplace = NewSymbol("Symbol.replace")
    r.SymbolSearch = NewSymbol("Symbol.search")
    r.SymbolSplit = NewSymbol("Symbol.split")
    r.SymbolSpecies = NewSymbol("Symbol.species")
    r.SymbolUnscopables = NewSymbol("Symbol.unscopables")
    r.SymbolAsyncIterator = NewSymbol("Symbol.asyncIterator")
}
```

### 1.2 Update VM struct

```go
// In vm.go, update VM struct:
type VM struct {
    // Realm management
    defaultRealm  *Realm   // The initial/main realm
    currentRealm  *Realm   // Currently executing realm
    realmCounter  int      // For generating unique realm IDs

    // Execution state (remains per-VM, shared across realms)
    frames        [MaxFrames]CallFrame
    frameCount    int
    registerStack []Value
    // ... rest of execution state

    // Remove these (moved to Realm):
    // - All prototype fields
    // - All symbol fields
    // - GlobalObject
    // - heap
}

func (vm *VM) nextRealmID() int {
    vm.realmCounter++
    return vm.realmCounter
}

// CurrentRealm returns the active realm for the current execution.
func (vm *VM) CurrentRealm() *Realm {
    return vm.currentRealm
}
```

---

## Phase 2: Accessor Methods

**Goal**: Create accessor methods that read from current realm instead of VM fields.

### 2.1 Add realm accessors to VM

```go
// In vm.go or realm.go

// Prototype accessors - used throughout the codebase
func (vm *VM) ObjectPrototype() Value   { return vm.currentRealm.ObjectPrototype }
func (vm *VM) ArrayPrototype() Value    { return vm.currentRealm.ArrayPrototype }
func (vm *VM) FunctionPrototype() Value { return vm.currentRealm.FunctionPrototype }
func (vm *VM) StringPrototype() Value   { return vm.currentRealm.StringPrototype }
func (vm *VM) NumberPrototype() Value   { return vm.currentRealm.NumberPrototype }
func (vm *VM) BooleanPrototype() Value  { return vm.currentRealm.BooleanPrototype }
func (vm *VM) ErrorPrototype() Value    { return vm.currentRealm.ErrorPrototype }
func (vm *VM) TypeErrorPrototype() Value { return vm.currentRealm.TypeErrorPrototype }
// ... etc for all 29+ prototypes

// Symbol accessors
func (vm *VM) SymbolIterator() Value      { return vm.currentRealm.SymbolIterator }
func (vm *VM) SymbolToStringTag() Value   { return vm.currentRealm.SymbolToStringTag }
func (vm *VM) SymbolToPrimitive() Value   { return vm.currentRealm.SymbolToPrimitive }
// ... etc for all 13 symbols

// Global accessors
func (vm *VM) GlobalObject() *PlainObject { return vm.currentRealm.GlobalObject }
func (vm *VM) Heap() *Heap                { return vm.currentRealm.Heap }
```

### 2.2 Search and replace pattern

For each prototype field, search for patterns like:
- `vm.ObjectPrototype` → `vm.ObjectPrototype()` (or keep as field access to realm)
- `vm.ArrayPrototype.AsPlainObject()` → `vm.currentRealm.ArrayPrototype.AsPlainObject()`

**Files to update:**
- `vm.go` - ~30 references
- `vm_init.go` - ~15 references
- `op_getprop.go` - ~10 references
- `op_setprop.go` - ~5 references
- `call.go` - ~3 references
- `property_helpers.go` - ~5 references
- `exceptions.go` - ~3 references

---

## Phase 3: Initializer Updates

**Goal**: Update builtins initializers to work with Realm instead of VM.

### 3.1 Update RuntimeContext

```go
// In pkg/builtins/initializer.go

type RuntimeContext struct {
    VM     *vm.VM
    Realm  *vm.Realm  // NEW: Target realm for initialization
    Driver interface{}

    // These now come from Realm
    ObjectPrototype   vm.Value
    FunctionPrototype vm.Value
    ArrayPrototype    vm.Value

    // Helper to define globals in the realm
    DefineGlobal func(name string, value vm.Value) error
}

// NewRuntimeContext creates a context for initializing a specific realm
func NewRuntimeContext(vmInstance *vm.VM, realm *vm.Realm, driver interface{}) *RuntimeContext {
    ctx := &RuntimeContext{
        VM:     vmInstance,
        Realm:  realm,
        Driver: driver,
        ObjectPrototype:   realm.ObjectPrototype,
        FunctionPrototype: realm.FunctionPrototype,
        ArrayPrototype:    realm.ArrayPrototype,
    }

    ctx.DefineGlobal = func(name string, value vm.Value) error {
        // Store in realm's heap
        return realm.DefineGlobal(name, value)
    }

    return ctx
}
```

### 3.2 Update initializer pattern

Each initializer should store prototypes on the Realm:

```go
// Example: object_init.go
func (o *ObjectInitializer) InitRuntime(ctx *RuntimeContext) error {
    realm := ctx.Realm  // Use realm instead of VM

    objectProto := realm.ObjectPrototype.AsPlainObject()

    // Add methods to prototype
    objectProto.SetOwnNonEnumerable("hasOwnProperty", ...)
    objectProto.SetOwnNonEnumerable("toString", ...)

    // Create constructor
    objectCtor := vm.NewNativeFunction(...)

    // Store on realm
    ctx.DefineGlobal("Object", objectCtor)

    return nil
}
```

### 3.3 Symbol initializer special handling

```go
// symbol_init.go - symbols are per-realm now
func (s *SymbolInitializer) InitRuntime(ctx *RuntimeContext) error {
    realm := ctx.Realm

    // Use realm's symbols (already created in InitializeSymbols)
    // Register Symbol.for() to use realm's registry
    symbolForImpl := func(args []vm.Value) (vm.Value, error) {
        key := args[0].ToString()
        if existing, ok := realm.SymbolRegistry[key]; ok {
            return existing, nil
        }
        sym := vm.NewSymbol(key)
        realm.SymbolRegistry[key] = sym
        return sym, nil
    }

    // ... rest of symbol init
}
```

---

## Phase 4: VM Initialization Flow

**Goal**: Update NewVM and initialization to create default realm.

### 4.1 Update NewVM

```go
func NewVM() *VM {
    vm := &VM{
        // Execution state
        openUpvalues:   make([]*Upvalue, 0, 16),
        propCache:      make(map[int]*PropInlineCache),
        emptyRestArray: NewArray(),
        moduleContexts: make(map[string]*ModuleContext),
        // ... other execution state
    }

    // Create and initialize default realm
    vm.defaultRealm = NewRealm(vm)
    vm.defaultRealm.InitializePrototypes()
    vm.defaultRealm.InitializeSymbols()
    vm.currentRealm = vm.defaultRealm

    return vm
}
```

### 4.2 Update InitializeBuiltins

```go
// In driver.go
func (p *Paserati) initializeBuiltinsWithCustom(customInitializers []builtins.Initializer) error {
    realm := p.vm.CurrentRealm()

    ctx := builtins.NewRuntimeContext(p.vm, realm, p)

    for _, init := range allInitializers {
        if err := init.InitRuntime(ctx); err != nil {
            return err
        }
    }

    // Finalize realm globals
    realm.FinalizeGlobals()

    return nil
}
```

---

## Phase 5: Cross-Realm Support

**Goal**: Enable creating additional realms and basic cross-realm operations.

### 5.1 Realm creation API

```go
// CreateRealm creates a new isolated realm.
func (vm *VM) CreateRealm() *Realm {
    realm := NewRealm(vm)
    realm.InitializePrototypes()
    realm.InitializeSymbols()

    // Initialize built-ins for new realm
    // (requires driver reference - may need to store on VM)

    return realm
}

// WithRealm executes a function in a specific realm context.
func (vm *VM) WithRealm(realm *Realm, fn func()) {
    prev := vm.currentRealm
    vm.currentRealm = realm
    defer func() { vm.currentRealm = prev }()
    fn()
}
```

### 5.2 Test262 $262.createRealm() support

```go
// In test262 harness setup
createRealmImpl := func(args []vm.Value) (vm.Value, error) {
    newRealm := vmInstance.CreateRealm()

    // Initialize built-ins in new realm
    vmInstance.WithRealm(newRealm, func() {
        // Run initializers
    })

    // Return realm wrapper object with .global and .eval
    wrapper := vm.NewObject(vmInstance.ObjectPrototype())
    wrapperObj := wrapper.AsPlainObject()

    // .global - the new realm's global object
    wrapperObj.SetOwn("global", vm.NewValueFromPlainObject(newRealm.GlobalObject))

    // .eval - evaluate code in the new realm
    evalImpl := func(args []vm.Value) (vm.Value, error) {
        code := args[0].ToString()
        return vmInstance.WithRealmEval(newRealm, code)
    }
    wrapperObj.SetOwn("eval", vm.NewNativeFunction(1, false, "eval", evalImpl))

    return wrapper, nil
}
```

---

## Implementation Checklist

### Phase 1: Realm Container (Day 1-2)
- [ ] Create `pkg/vm/realm.go` with Realm struct
- [ ] Add all prototype fields to Realm
- [ ] Add all symbol fields to Realm
- [ ] Add GlobalObject and Heap to Realm
- [ ] Implement NewRealm(), InitializePrototypes(), InitializeSymbols()
- [ ] Add realm management fields to VM (defaultRealm, currentRealm, realmCounter)

### Phase 2: Accessor Migration (Day 2-4)
- [ ] Add accessor methods to VM for all prototypes
- [ ] Update `vm.go` references (~30 locations)
- [ ] Update `vm_init.go` references (~15 locations)
- [ ] Update `op_getprop.go` references (~10 locations)
- [ ] Update `op_setprop.go` references (~5 locations)
- [ ] Update `call.go` references (~3 locations)
- [ ] Update `property_helpers.go` references (~5 locations)
- [ ] Update `exceptions.go` references (~3 locations)
- [ ] Update any other files with prototype references

### Phase 3: Initializer Updates (Day 4-6)
- [ ] Update RuntimeContext to include Realm
- [ ] Update `object_init.go`
- [ ] Update `array_init.go`
- [ ] Update `function_init.go`
- [ ] Update `symbol_init.go` (special handling for registry)
- [ ] Update `error_init.go`
- [ ] Update `string_init.go`
- [ ] Update `number_init.go`
- [ ] Update `boolean_init.go`
- [ ] Update `regexp_init.go`
- [ ] Update `date_init.go`
- [ ] Update `map_init.go`
- [ ] Update `set_init.go`
- [ ] Update `promise_init.go`
- [ ] Update `typedarray_init.go`
- [ ] Update `arraybuffer_init.go`
- [ ] Update `dataview_init.go`
- [ ] Update `json_init.go`
- [ ] Update `math_init.go`
- [ ] Update `reflect_init.go`
- [ ] Update `proxy_init.go`
- [ ] Update `iterator_init.go`
- [ ] Update `generator_init.go`
- [ ] Update `async_init.go`
- [ ] Update remaining initializers

### Phase 4: VM Initialization (Day 6-7)
- [ ] Update NewVM() to create default realm
- [ ] Update driver initialization to use realm context
- [ ] Test basic execution still works
- [ ] Run smoke tests

### Phase 5: Cross-Realm Support (Day 7-9)
- [ ] Implement CreateRealm()
- [ ] Implement WithRealm()
- [ ] Add $262.createRealm() to test harness
- [ ] Test cross-realm object identity
- [ ] Test cross-realm instanceof behavior

### Phase 6: Testing & Cleanup (Day 9-10)
- [ ] Run full Test262 suite
- [ ] Verify no regressions
- [ ] Verify cross-realm tests now pass
- [ ] Clean up any temporary compatibility shims
- [ ] Update documentation

---

## Risk Mitigation

### Risk: Breaking existing tests
**Mitigation**: Keep old field names as aliases during migration, run tests after each phase.

### Risk: Performance regression from accessor indirection
**Mitigation**: Use direct field access `vm.currentRealm.ObjectPrototype` instead of method calls where performance-critical.

### Risk: Initializer order dependencies
**Mitigation**: Keep existing priority ordering, test each initializer individually.

### Risk: Symbol identity across realms
**Mitigation**: Each realm has its own symbols; cross-realm symbol comparison needs special handling.

---

## Future Work (Post-Implementation)

1. **ShadowRealm API** - User-facing `new ShadowRealm()` with callable boundary
2. **Realm GC** - Clean up realms when no longer referenced
3. **Cross-realm callable wrapping** - Functions that work across realm boundaries
4. **Module realm isolation** - Option to run modules in separate realms

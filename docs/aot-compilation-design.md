# Paserati AOT Compilation & Binary Module Format

## Vision

Paserati today compiles TypeScript to bytecode on-the-fly: parse, typecheck, compile, execute.
AOT compilation moves the first three stages to build time, producing a binary artifact that
the runtime loads and executes directly. This unlocks three deployment scenarios:

1. **Pre-compiled modules** -- individual `.ts` files compiled to `.psm` binaries that can be
   loaded alongside source modules and native (Go) modules in the same program
2. **Assemblies** -- tree-shaken bundles of multiple modules compiled into a single `.psra`
   blob with an entry point, suitable for distribution
3. **Embedded executables** -- a Go binary that `//go:embed`s an assembly blob and runs it,
   producing a single static executable from a TypeScript program

These compose naturally with two runtime scenarios:

- **Standalone binary**: `go build` produces `engine + blob = compiled TS program`
- **Serverless isolates**: load a blob into a `Paserati` instance in a goroutine, achieving
  near-zero cold boot since parsing and compilation are eliminated

And looking further ahead:

- **Native compilation**: a second compiler backend that emits Go source from the typed AST,
  compiled with `go build` into a fully native binary with type-specialized code paths

---

## Current Compilation Pipeline

Understanding what exists today is essential for knowing what to serialize.

```
Source (.ts)
    │
    ▼
┌─────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│  Lexer   │ ──► │  Parser  │ ──► │ Checker  │ ──► │ Compiler │ ──► │    VM    │
│          │     │          │     │          │     │          │     │          │
│ tokens   │     │ AST      │     │ types    │     │ *Chunk   │     │ execute  │
└─────────┘     └──────────┘     └──────────┘     └──────────┘     └──────────┘
                                                        │
                                              HeapAlloc (global indices)
```

The compiler produces `*vm.Chunk` -- this is the artifact we need to serialize.

### What Lives in a Chunk

| Field                  | Type                    | Serializable | Notes                                            |
|------------------------|-------------------------|--------------|--------------------------------------------------|
| `Code`                 | `[]byte`                | Trivial      | Raw bytecode, the core payload                   |
| `Constants`            | `[]Value`               | Medium       | Recursive: function constants contain sub-chunks |
| `Lines`                | `[]int`                 | Trivial      | Line map; optional in release builds             |
| `ExceptionTable`       | `[]ExceptionHandler`    | Trivial      | Flat struct of ints and bools                    |
| `MaxRegs`              | `int`                   | Trivial      | Register window size                             |
| `NumSpillSlots`        | `int`                   | Trivial      | Overflow slot count                              |
| `IsStrict`             | `bool`                  | Trivial      | Strict mode flag                                 |
| `HasSimpleParameterList` | `bool`                | Trivial      | Parameter optimization hint                      |
| `ScopeDesc`            | `*ScopeDescriptor`      | Easy         | Only present when module uses `eval()`           |
| `VarGlobalIndices`     | `[]uint16`              | Trivial      | Non-configurable global slots                    |
| `propInlineCaches`     | `[]*PropInlineCache`    | **Skip**     | Runtime-only; lazily rebuilt on first access      |

### What Lives in the Constant Pool

In practice, compile-time constant pools contain only these value types:

- **Strings** -- property names, string literals (tag + length-prefixed UTF-8)
- **Integers** -- `int64` payload
- **Floats** -- `float64` payload (IEEE 754)
- **Booleans, Undefined, Null** -- tag only
- **Function templates** -- `FunctionObject` with a nested `Chunk` (recursive)

Runtime-only types (`Closure`, `NativeFunction`, `Object`, `Array`, etc.) do not appear in
constant pools -- they are created at execution time. This makes serialization tractable.

### Function Templates in Constants

A `FunctionObject` in the constant pool is a template (not yet instantiated):

```
FunctionObject {
    Arity, Length, Variadic     // call signature
    Name                        // function name (string)
    Chunk *Chunk                // nested bytecode (recursive!)
    UpvalueCount                // number of captures
    RegisterSize                // register window requirement
    IsGenerator, IsAsync, ...   // behavioral flags
    NameBindingRegister         // NFE self-reference register
    // HomeRealm: nil at compile time -- set when instantiated as closure
    // Prototype: nil at compile time -- set when instantiated
}
```

The key insight: `Chunk` references form a tree rooted at the module's top-level chunk.
Serialization flattens this tree into a chunk table with index references replacing pointers.

---

## Binary Format Design

### Single Module (`.psm`)

```
┌────────────────────────────────────────┐
│ Header                                  │
│   magic:           [4]byte "PSRT"       │
│   version:         uint32               │
│   flags:           uint32               │
│   module_path:     string               │
│   builtin_hash:    uint32               │  ◄── verifies builtin layout matches
│   chunk_count:     uint16               │
│   string_count:    uint32               │
│   import_count:    uint16               │
│   export_count:    uint16               │
├────────────────────────────────────────┤
│ String Intern Table                     │  ◄── shared across all chunks
│   [length-prefixed UTF-8 strings]       │
├────────────────────────────────────────┤
│ Import Table                            │
│   [{module_specifier, import_name,      │
│     local_global_index}]                │
├────────────────────────────────────────┤
│ Export Table                            │
│   [{export_name, global_index}]         │
├────────────────────────────────────────┤
│ Chunk Table                             │
│   chunk[0] = module top-level           │
│   chunk[1..N] = nested functions        │
│                                         │
│   Each chunk:                           │
│     code_length + code_bytes            │
│     constant_count + constants          │
│       (function constants → chunk idx)  │
│     exception_table_count + entries     │
│     max_regs, num_spill_slots           │
│     flags (strict, simple_params)       │
│     scope_desc (optional, for eval)     │
│     var_global_indices                  │
├────────────────────────────────────────┤
│ Debug Info (optional, strippable)       │
│   line_tables, source_file_paths        │
└────────────────────────────────────────┘
```

**Constant encoding**: Each constant is `[type_tag:uint8][payload]`:
- `0x01` Undefined, `0x02` Null, `0x03` True, `0x04` False
- `0x10` Integer: `int64` (varint)
- `0x11` Float: `float64` (IEEE 754, 8 bytes)
- `0x20` String: `string_table_index` (varint)
- `0x30` Function: `chunk_table_index` (uint16) + function metadata

### Assembly (`.psra`)

An assembly bundles multiple modules with a shared string table:

```
┌────────────────────────────────────────┐
│ Assembly Header                         │
│   magic:           [4]byte "PSRA"       │
│   version:         uint32               │
│   flags:           uint32               │
│   entry_module:    string               │
│   module_count:    uint16               │
│   builtin_manifest_size: uint16         │
├────────────────────────────────────────┤
│ Builtin Manifest                        │  ◄── ordered list of builtin names
│   [{name, expected_heap_index}]         │      used to verify or relocate
├────────────────────────────────────────┤
│ Global String Intern Table              │  ◄── shared across ALL modules
│   [length-prefixed UTF-8 strings]       │
├────────────────────────────────────────┤
│ Module Directory                        │
│   [{module_path, offset, size}]         │  ◄── for random access
├────────────────────────────────────────┤
│ Module[0] (embedded .psm without        │
│   its own string table -- uses global)  │
│ Module[1]                               │
│ ...                                     │
│ Module[N]                               │
├────────────────────────────────────────┤
│ Assembly Debug Info (optional)           │
│   module dependency graph               │
│   source maps                           │
└────────────────────────────────────────┘
```

---

## Module Linkage Strategy

### The Problem: HeapAlloc Global Indices

Today, the compiler uses a unified `HeapAlloc` to assign global heap indices across all
modules in a single compilation session. When compiling `export const x = 42`, the compiler
assigns `x` a heap index (say, 87) via `HeapAlloc.GetOrAssignIndex("module_a::x")`. The
bytecode then uses `OpSetGlobal 87` and `OpGetGlobal 87`. When another module imports `x`,
it gets compiled with the **same** `HeapAlloc` instance, so it also knows index 87.

An AOT binary baked with index 87 would clash with a different compilation session.

### The Solution: Module-Local Globals + Import/Export Tables

Each AOT module has its own local global index space. Cross-module references use the
import/export tables, resolved at load time:

1. **At build time**: The compiler assigns module-local heap indices starting from 0
   (after builtins). The export table maps `{name -> local_index}`. The import table maps
   `{module_specifier, import_name -> local_index_to_fill}`.

2. **At load time**: The loader allocates fresh heap slots in the VM's unified heap.
   For each module it loads:
   - Allocates `N` heap slots (where N = module's global count)
   - Records a `local_index -> heap_index` relocation map
   - Patches `OpGetGlobal`/`OpSetGlobal` operands in the bytecode (2-byte patches)
   - Wires imports: looks up the exporting module's export table, maps the export's
     heap index into the importing module's local slot

3. **Why this works**: `OpGetGlobal`/`OpSetGlobal` use 16-bit operands embedded in the
   bytecode (`Code []byte`). Relocation is a simple in-place patch of two bytes per
   instruction. The bytecode is otherwise position-independent (jumps are relative or
   absolute within a chunk).

### Builtin Compatibility

Builtins occupy heap indices 0..N, assigned by `initializeBuiltins()` in priority order.
The assembly's builtin manifest records this layout. At load time:

- **Match**: Builtins initialized in same order -> indices align, no relocation needed
- **Mismatch**: Loader rejects the assembly with a clear error ("assembly compiled with
  builtin layout v3, runtime has v4") or performs relocation if supported

For the embedded binary scenario (engine + blob compiled together), the layout always
matches because the same binary initializes builtins in the same order.

---

## Deployment Scenario 1: Embedded Go Binary

```go
package main

import (
    "embed"
    "fmt"
    "os"
    "paserati/pkg/driver"
)

//go:embed app.psra
var appAssembly []byte

func main() {
    p := driver.NewPaserati()
    if err := p.LoadAssembly(appAssembly); err != nil {
        fmt.Fprintf(os.Stderr, "Failed to load assembly: %v\n", err)
        os.Exit(1)
    }
    result, err := p.RunAssembly(os.Args[1:])
    if err != nil {
        fmt.Fprintf(os.Stderr, "Runtime error: %v\n", err)
        os.Exit(1)
    }
    // ...
}
```

### What Happens at Runtime

```
Process starts
  │
  ├─ NewPaserati()
  │    ├─ NewVM()                      ~cheap (allocate register stack, maps)
  │    ├─ initializeBuiltins()         ~expensive (50 InitRuntime calls, prototype creation)
  │    └─ SyncPrototypesToRealm()      ~cheap
  │
  ├─ LoadAssembly(blob)
  │    ├─ Validate header + builtin manifest
  │    ├─ Deserialize string table
  │    ├─ Deserialize chunk table (reconstruct Chunk structs, wire FunctionObject constants)
  │    ├─ Allocate heap slots for module globals
  │    ├─ Relocate OpGetGlobal/OpSetGlobal operands
  │    ├─ Wire import tables (resolve cross-module references)
  │    └─ Register modules in vm.moduleContexts as pre-loaded
  │
  └─ RunAssembly()
       └─ Execute entry module (pure bytecode interpretation, no parsing/compiling)
```

**Key win**: Eliminates parsing, type checking, and compilation entirely at runtime.
The only setup cost is builtin initialization + deserialization.

### Implementation Effort

| Component                          | Effort | Description                                         |
|------------------------------------|--------|-----------------------------------------------------|
| Chunk serializer/deserializer      | Medium | Serialize Chunk tree, flatten function constants     |
| Value serializer (constant pool)   | Medium | Tag+payload encoding for strings, numbers, functions |
| Binary format writer               | Small  | Header, sections, string table                       |
| Binary format reader               | Small  | Validate, deserialize, reconstruct                   |
| `LoadAssembly()` on driver         | Medium | Heap allocation, relocation, module registration     |
| Global index relocation            | Medium | Patch OpGetGlobal/OpSetGlobal in loaded bytecode     |
| Skip checker/compiler for AOT      | Small  | `NewPaseratiForAOT()` constructor                    |
| CLI: `paserati compile`            | Small  | Wire up the pipeline                                 |
| CLI: `paserati run file.psra`      | Small  | Load + execute                                       |

**Estimated total: ~3-4 weeks**

---

## Deployment Scenario 2: Serverless Isolates

```go
// Host process -- runs once
assemblyBlob := loadAssemblyFromStorage("handler.psra")

// Per-request -- runs in goroutine, needs to be fast
func handleRequest(req Request) Response {
    p := driver.NewPaseratiIsolate(sharedRuntime)  // minimal setup
    p.LoadAssembly(assemblyBlob)                    // deserialize pre-compiled code
    p.SetGlobal("request", marshalRequest(req))
    result, _ := p.Run()
    p.Cleanup()
    return unmarshalResponse(result)
}
```

**Target**: sub-millisecond isolate spin-up. The assembly is pre-compiled, so no parsing or
compilation. The question is how fast we can create a fresh VM and load the bytecode.

### What Must Be Per-Isolate

Each isolate needs its own:
- VM instance (register stack, call frames, exception handling)
- Realm (heap, global object, prototypes, symbol registry)
- Inline caches (runtime property access caches)
- Module contexts (execution state per loaded module)
- Open upvalues (closure capture state)

### What Can Be Shared (Read-Only)

Across all isolates running the same assembly:
- `Chunk.Code` bytes (bytecode is read-only after relocation)
- `Chunk.Constants` (values in constant pool are immutable)
- String intern table
- Assembly metadata

---

## Landmines: Shared Mutable State

These are the specific pieces of the engine that currently prevent safe concurrent
multi-VM execution and need to be addressed for the serverless scenario.

### Landmine 1: `RootShape` (Package-Level Global)

**Location**: `pkg/vm/object.go`

```go
var DefaultObjectPrototype Value
var RootShape *Shape

func init() {
    RootShape = &Shape{
        fields:            []Field{},
        stringTransitions: make(map[string]*Shape),
        transitions:       make(map[string]*Shape),
    }
    protoObj := &PlainObject{prototype: Null, shape: RootShape}
    DefaultObjectPrototype = Value{typ: TypeObject, obj: unsafe.Pointer(protoObj)}
}
```

**Problem**: Every `PlainObject` created in any VM starts from this shared `RootShape` and
builds a shared transition tree. Under concurrent load:
- `sync.RWMutex` on Shape becomes a contention point
- Shape tree grows unboundedly (shapes from one isolate leak into the shared tree)
- Cross-isolate memory coupling (one isolate's object shapes affect another's cache behavior)

**Fix**: Per-realm `RootShape`. The `Realm` struct already exists -- add a `RootShape *Shape`
field and thread it through `NewObject()`, `NewObjectWithProto()`, etc. This is a medium-sized
refactor because `NewObject()` is called from many places, but it's straightforward.

**Severity**: **Blocker** for concurrent VMs (data race on Shape.mu).

### Landmine 2: `DefaultObjectPrototype` (Package-Level Global)

**Location**: Same as above.

**Problem**: All VMs share the same default prototype object. If one isolate executes
`Object.prototype.foo = 42`, every other isolate sees it.

**Fix**: The `Realm` struct already has an `ObjectPrototype` field. Stop using the
package-level `DefaultObjectPrototype` and use `realm.ObjectPrototype` instead. This is
a small change -- grep for `DefaultObjectPrototype` and replace with realm access.

**Severity**: **Blocker** for isolate safety (cross-isolate mutation).

### Landmine 3: `prototypeCache` (Package-Level Global)

**Location**: `pkg/vm/cache_prototype.go`

```go
var (
    prototypeCache      map[int]*PrototypeCache
    prototypeCacheMutex sync.RWMutex
)
```

**Problem**: Keyed by instruction pointer (integer). Two VMs executing the same bytecode
would collide on cache keys, causing cross-VM cache pollution. Also, mutex contention.

**Fix**: Move to per-VM storage (like how `propCache` is already per-VM on the `VM` struct),
or better yet, per-Chunk (like `propInlineCaches`). Since prototype caching is a performance
optimization, per-VM is simpler and avoids polluting the shared chunk data.

**Severity**: **Blocker** for concurrent VMs (data race + cache pollution).

### Landmine 4: `propInlineCaches` on Shared Chunks

**Location**: `pkg/vm/ic_site_cache.go`

```go
func (vm *VM) getOrCreatePropInlineCache(frame *CallFrame, siteIP int) *PropInlineCache {
    chunk := frame.closure.Fn.Chunk
    if chunk.propInlineCaches == nil {
        chunk.propInlineCaches = make([]*PropInlineCache, len(chunk.Code))
    }
    // ...
}
```

**Problem**: Inline caches are allocated lazily **on the Chunk itself**. If two goroutines
share the same `Chunk` pointer (because they loaded the same assembly), they race on
`propInlineCaches` and would get cross-isolate cache pollution even without a race.

**Fix**: Separate immutable chunk data (Code, Constants, ExceptionTable) from mutable
per-execution data (inline caches). Two approaches:

- **Template + Instance**: Share a `ChunkTemplate` (read-only), each VM gets a `ChunkInstance`
  with its own cache storage. This is the clean solution.
- **Per-VM IC storage**: Move inline caches entirely to the VM struct, keyed by
  `(chunk_id, site_ip)`. Simpler to implement but slightly slower lookup.

**Severity**: **Blocker** for shared-chunk execution (data race).

### Landmine 5: `ClearShapeCache()` is Process-Wide

**Location**: `pkg/vm/object.go`

```go
func ClearShapeCache() {
    if RootShape != nil {
        RootShape.mu.Lock()
        RootShape.stringTransitions = make(map[string]*Shape)
        RootShape.transitions = make(map[string]*Shape)
        RootShape.mu.Unlock()
    }
}
```

**Problem**: Called in test runners to prevent memory bloat. With per-realm shapes, this
becomes per-realm cleanup, which is fine. But if we keep the global `RootShape`, calling
this while other VMs are running would invalidate their shape caches.

**Fix**: Follows naturally from Landmine 1 fix (per-realm RootShape).

**Severity**: Minor (only affects cleanup path).

---

## Landmines: Builtin Initialization Cost

### The Bottleneck

`initializeBuiltins()` runs ~50 `InitRuntime()` calls, each of which:
- Creates prototype objects (PlainObject with methods)
- Wraps Go functions as `NativeFunctionObject` values
- Allocates heap slots and populates them
- Sets up prototype chains

For the embedded binary scenario (Scenario 1), this runs once at process start -- fine.
For serverless isolates (Scenario 2), this runs per-isolate -- the dominant cold boot cost.

### Solutions (Pick One)

**Option A: Builtin Snapshot + Clone (recommended for serverless)**

Initialize builtins once in the host process. Snapshot the resulting realm state (prototypes,
constructors, heap slots). For each new isolate, deep-clone the snapshot.

```go
// Once at host startup
template := driver.NewPaseratiTemplate()  // full init with builtins

// Per request (fast)
isolate := template.NewIsolate()  // deep-clone realm, skip InitRuntime
```

Deep-cloning a realm means copying:
- ~50 prototype objects (PlainObject with method functions)
- ~80 constructor/global objects
- Heap slots 0..N (builtin range)
- Symbol registry (well-known symbols)

This is O(builtin count), not O(module complexity). Should be <1ms.

**Challenge**: The clone must handle:
- Object graphs with internal references (prototype chains)
- Native functions (Go function pointers -- clone by reference, they're stateless)
- Value type containing `unsafe.Pointer` (need a pointer remapping table for cycles)

**Option B: Shared Read-Only Builtins + COW Overlay**

Builtins are initialized once and frozen. Each isolate gets a thin copy-on-write layer.
If user code mutates `Object.prototype`, only that isolate's overlay is affected.

```go
type COWObject struct {
    base     *PlainObject  // shared, read-only
    overlay  *PlainObject  // per-isolate, nil until first write
}
```

**Pros**: Near-zero per-isolate cost, memory efficient (shared prototypes)
**Cons**: Invasive change to property access hot path (every `GetOwn`/`SetOwn` must check overlay)

**Option C: Accept the Cost**

If builtin init takes ~2ms and your SLA allows it, just initialize per-isolate. This is the
simplest path and may be good enough for many use cases. Profile first.

---

## Things That Are NOT Problems

These aspects of the engine are already AOT-friendly:

### Register Allocation
Fully resolved at compile time. Register indices are embedded in bytecode as operand bytes.
No relocation needed -- they're chunk-local.

### Spill Slots
Same story. `NumSpillSlots` is a compile-time constant per chunk. The VM allocates the array
at frame push time.

### Exception Tables
All fields are ints/bools. PC values are absolute within the chunk. No external references.

### Bytecode Position Independence
Jumps within a chunk use absolute PCs (byte offsets into `Code`). No inter-chunk jumps exist.
Function calls go through `OpCall` which looks up the function in a register -- no PC
references to patch.

### Upvalue Capture Descriptors
`OpClosure` instructions encode capture information inline in the bytecode:
`[CaptureType:uint8 Index:uint8/uint16]` per upvalue. This is pure data in the `Code` array
and serializes naturally.

### Module References in Bytecode
`OpEvalModule`, `OpGetModuleExport`, and `OpCreateNamespace` reference modules by string
path (via constant pool index). The binary format stores these strings -- the loader resolves
them. No absolute addresses.

### Native Functions
Go function pointers appear only in the runtime heap (registered by builtins), never in
compile-time constant pools. AOT modules reference builtins via `OpGetGlobal` with indices
that resolve at load time. No native function serialization needed.

### `FunctionObject.HomeRealm`
Nil at compile time in constant pool templates. Set when the function is instantiated as a
closure at runtime (`OpClosure` handler). No serialization concern.

### Prototype Setup
Happens at runtime via `initializeBuiltins()`, not in bytecode. AOT modules assume builtins
exist in the heap and access them by index.

---

## Tree Shaking (Assembly Only)

Tree shaking applies when building assemblies from multi-module programs.

### Available Information

The compiler already tracks:
- `moduleBindings.ExportedNames` -- what each module exports
- `moduleBindings.ImportedNames` -- what each module imports and from where
- Module dependency graph via `ModuleLoader`

### Algorithm

1. Start from entry module(s)
2. Walk import graph, marking reachable modules
3. Within each module, mark reachable exports (exports used by importing modules)
4. Compile only reachable modules with only reachable exports
5. Optionally: analyze builtin usage to produce a minimal builtin manifest

### Limitations

- Dynamic `import()` makes static analysis incomplete (include all dynamically imported
  modules, or require explicit hints)
- `eval()` can reference any global (modules using eval defeat tree shaking)
- Re-exports (`export * from`) require including all exports from the source module
  unless usage analysis can prove otherwise

Tree shaking is an optimization, not a correctness requirement. Assemblies work without it --
they're just larger.

---

## Implementation Roadmap

### Phase 1: Core Serialization (Foundation)

**Goal**: Serialize and deserialize a single module's compiled Chunk.

1. Define binary encoding for Value constants (tag + payload)
2. Implement chunk tree flattening (recursive Chunk -> flat chunk table with indices)
3. Implement chunk tree reconstruction (flat table -> pointer graph)
4. String table interning (deduplicate across chunks)
5. Round-trip tests: compile a module, serialize, deserialize, compare

**Deliverable**: `pkg/aot/serialize.go`, `pkg/aot/deserialize.go`

### Phase 2: Single Module Binary Format

**Goal**: Compile a `.ts` file to a `.psm` binary and load it.

1. Define `.psm` file format (header, sections, as specified above)
2. Implement writer (Chunk + metadata -> bytes)
3. Implement reader (bytes -> Chunk + metadata)
4. Add `BinaryModuleResolver` to the module loader (resolves `.psm` files)
5. Implement global index relocation (patch OpGetGlobal/OpSetGlobal operands)
6. Integration: mix `.psm` and `.ts` modules in one program

**Deliverable**: `paserati compile -o output.psm input.ts`, modules can import `.psm` files

### Phase 3: Assembly Format

**Goal**: Bundle multiple modules into a `.psra` assembly with an entry point.

1. Define `.psra` format (assembly header, module directory, shared string table)
2. Implement assembly writer (multiple modules -> single blob)
3. Implement assembly reader (blob -> module set)
4. `LoadAssembly()` API on driver (load all modules, wire imports, register in VM)
5. `RunAssembly()` API (execute entry module)
6. Builtin manifest verification

**Deliverable**: `paserati compile -o output.psra --entry main.ts src/`

### Phase 4: Tree Shaking

**Goal**: Eliminate dead code from assemblies.

1. Build dependency graph from module loader data
2. Mark reachable exports starting from entry points
3. Compile only reachable modules/exports
4. Optional: minimize builtin set based on usage analysis

**Deliverable**: `paserati compile --tree-shake -o output.psra --entry main.ts`

### Phase 5: Isolate Safety (Required for Serverless)

**Goal**: Enable safe concurrent multi-VM execution.

1. Per-realm `RootShape` (eliminate package-level global)
2. Per-realm `DefaultObjectPrototype` (use realm field consistently)
3. Per-VM prototype cache (move from package-level to VM struct)
4. Separate `ChunkTemplate` (immutable) from per-VM inline caches
5. `NewPaseratiIsolate()` constructor (skip checker, compiler, use pre-loaded assembly)
6. Concurrent execution tests (race detector, stress tests)

**Deliverable**: Safe `go func() { isolate.Run() }()` from multiple goroutines

### Phase 6: Fast Isolate Boot (Optimization for Serverless)

**Goal**: Sub-millisecond isolate creation.

1. Implement realm snapshot (capture initialized builtin state)
2. Implement realm clone (deep-copy snapshot for new isolate)
3. Benchmark: measure isolate creation time
4. Optional: COW overlay for read-mostly builtins
5. `NewPaseratiTemplate()` / `template.NewIsolate()` API

**Deliverable**: <1ms isolate spin-up with full builtin support

### Phase 7: Native Go Backend (AST-to-Go Compiler)

**Goal**: Compile TypeScript to native Go source code via a second compiler backend.

The existing pipeline shares a frontend (lexer, parser, checker) across backends.
The Go backend walks the type-annotated AST and emits Go source that uses a slim
runtime library (`pkg/rt`) built on the existing value types.

```
                          Bytecode backend (current)
                              ┌──► Compiler ──► Chunk ──► VM
Source → Lexer → Parser → AST → Checker
                              └──► GoCompiler ──► .go files ──► go build ──► native binary
                          Go backend (new)
```

**Why this works**: The AST has everything the Go backend needs -- structured control flow
(`IfStatement`, `ForStatement`, `TryStatement`), explicit function boundaries, closure
capture information, and type annotations from the checker on every expression node via
`GetComputedType()`. The bytecode compiler destroys this structure; the AST preserves it.

**Type specialization**: The checker annotates every expression with its resolved type.
The Go backend can use this to emit specialized native Go code:

| TS type known at compile time | Generated Go code | Benefit |
|-------------------------------|-------------------|---------|
| `number + number` | `float64` addition | No Value boxing, native arithmetic |
| `string + string` | Go string concatenation | No type dispatch |
| `arr.length` where `arr: string[]` | `len(arr)` | No property lookup |
| `fn(x)` where `fn` is known | Direct Go function call | No dynamic dispatch |
| `any` / dynamic | `vm.Value` operations | Full JS semantics (fallback) |

**Generated code structure**: Each TS module becomes a Go source file. Functions become
Go functions. The full VM is still included in the binary for dynamic features (`eval()`,
`Function()` constructor, dynamic `import()`) but hot-path user code runs as native Go.

**Hard parts and their known solutions**:

- **Exception handling**: `TryStatement` in AST maps to Go `defer`/`recover` patterns.
  The AST gives you the try/catch/finally structure directly (unlike bytecode where
  you'd need to reconstruct it from flat ExceptionTable PC ranges).
- **Closures**: AST has explicit scope information. Generate Go closures that capture
  the right variables. Go's closure semantics (capture by reference) match JS.
- **Generators**: `YieldExpression` nodes in AST. Transform to explicit state machines
  (switch over state variable). This is a well-understood transformation -- Babel,
  TypeScript's own downlevel emit, C#, and Kotlin all do it.
- **Async/await**: Similar state machine transformation as generators, with promise
  integration.

**Runtime library (`pkg/rt`)**: A thin API layer over the existing `pkg/vm` types:

```go
package rt

type Runtime struct { realm *vm.Realm; heap *vm.Heap }

func (r *Runtime) GetProp(obj Value, name string) (Value, error)
func (r *Runtime) SetProp(obj Value, name string, val Value) error
func (r *Runtime) Call(fn Value, this Value, args []Value) (Value, error)
func (r *Runtime) NewObject() Value
func (r *Runtime) Throw(val Value)  // triggers panic, caught by try-block recover
func (r *Runtime) InstanceOf(val, ctor Value) (bool, error)
```

This reuses all existing value types, object model, prototype chains, and builtins.
The runtime library is NOT a reimplementation -- it's a calling convention adapter.

**Killer feature -- TypeScript as Go FFI**:

If TS modules compile to Go packages, Go code can import and call TypeScript functions
natively, and vice versa through the existing native module system. Bidirectional Go/TS
interop in a single static binary.

**Estimated effort**: ~4-5 months for production quality. The foundation (Phases 1-3) is
a prerequisite -- the runtime library, module linkage, and builtin system must exist first.

---

## Architecture After Full Implementation

```
                          BUILD TIME                              RUNTIME
                    ┌──────────────────────┐
  *.ts ──parse──►  │  AST                  │
                   │   │                   │
                   │  typecheck            │
                   │   │                   │
                   │  compile              │
                   │   │                   │
                   │  tree-shake (opt)     │
                   │   │                   │
                   │  serialize ───────────┼──► assembly.psra (binary blob)
                   └──────────────────────┘          │
                                                      │
         ┌────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────┐
│  Go Host Process                                         │
│                                                          │
│  ┌────────────────────────────────────┐                  │
│  │ Shared (read-only)                 │                  │
│  │  • ChunkTemplates (Code, Constants)│                  │
│  │  • String intern table             │                  │
│  │  • Assembly metadata               │                  │
│  │  • Builtin realm snapshot          │                  │
│  └───────────────┬────────────────────┘                  │
│                  │ clone per isolate                      │
│     ┌────────────┼────────────┐                          │
│     ▼            ▼            ▼                          │
│  ┌───────┐   ┌───────┐   ┌───────┐                      │
│  │ VM 1  │   │ VM 2  │   │ VM 3  │   goroutines         │
│  │ realm │   │ realm │   │ realm │                       │
│  │ heap  │   │ heap  │   │ heap  │                       │
│  │ ICs   │   │ ICs   │   │ ICs   │                       │
│  │ shapes│   │ shapes│   │ shapes│                       │
│  └───────┘   └───────┘   └───────┘                      │
└─────────────────────────────────────────────────────────┘
```

### Mixing Module Types

The module resolver chain handles all sources transparently:

```
import { foo } from "./lib.ts"         ──► FilesystemResolver (parse + compile)
import { bar } from "./prebuilt.psm"   ──► BinaryModuleResolver (deserialize)
import { baz } from "native-lib"       ──► NativeModuleResolver (Go interop)
```

All three resolve to `ModuleContext` entries in the VM with export tables.
From the bytecode's perspective, they're identical -- `OpEvalModule` + `OpGetModuleExport`.

---

## Summary of Effort

| Phase | What | Effort | Prerequisites |
|-------|------|--------|---------------|
| 1-3 | Bytecode serialization + assembly format | ~4 weeks | None |
| 4 | Tree shaking | ~2 weeks | Phases 1-3 |
| 5 | Isolate safety (shared state fixes) | ~3 weeks | Phases 1-3 |
| 6 | Fast isolate boot (realm snapshots) | ~2 weeks | Phase 5 |
| 7 | Native Go backend (AST-to-Go compiler) | ~4-5 months | Phases 1-3 |

Phases 1-3 are self-contained and deliver the embedded binary scenario.
Phases 5-6 are required for serverless isolates. Phase 4 is an optimization applicable to both.
Phase 7 is the endgame -- native-speed TypeScript compiled to Go.

The engine is in good shape for this work. The main refactoring (Phase 5) is removing four
package-level globals and threading realm/VM references through object creation paths.
The Realm struct already exists with the right fields -- it just needs to be used consistently.
The Go backend (Phase 7) reuses the entire frontend and runtime -- it's a new codegen target,
not a new engine.

## Object representation modernization: symbol keys, shapes, and ICs

### Goals

- Correct, first-class support for symbol keys (no stringization like `@@symbol:...`).
- Modern hidden-class (shape) model with versioning for fast, guarded property access.
- Inline caches (ICs) that exploit shape+key guards to avoid descriptor walks.
- Keep memory overhead low: per-object stores only values (and compact accessor slots); indexes live on shapes and are created lazily.

### Current state (findings)

Previous state used `@@symbol:` stringization for symbol keys. Migration status by area:

- Checker

  - `pkg/checker/expressions.go`:
    - `@@symbol:Symbol.iterator` and generic `@@symbol:` construction (multiple sites around member access and `yield*`).
  - `pkg/checker/class.go`:
    - `@@symbol:Symbol.iterator` for class computed props.

- Compiler

  - `pkg/compiler/compile_expression.go` and `pkg/compiler/compile_statement.go`: `yield*` and `for..of` now fetch `Symbol["iterator"]` at runtime and index with the singleton symbol; no constant-pool `@@symbol:` use.
  - `pkg/compiler/compile_class.go`: class element keys use `__COMPUTED_PROPERTY__` (no stringized `@@symbol:`).

- Builtins

  - `pkg/builtins/object_init.go`: `Object.defineProperty`/`getOwnPropertyDescriptor` accept real symbol keys; no `@@symbol:` conversion. For `DictObject`, symbol keys are currently ignored (no stringization fallback).
  - `pkg/builtins/array_init.go`, `pkg/builtins/string_init.go`, `pkg/builtins/generator_init.go`, `pkg/builtins/iterator_init.go`: prototypes register `[Symbol.iterator]` under native symbol keys only; legacy bridge removed. Type surface uses `__COMPUTED_PROPERTY__` placeholders.

- Test262 harness glue
  - `cmd/paserati-test262/test262_builtins.go`: `verifyProperty` and cleanup paths operate on real symbol keys via `DeleteOwnByKey`; no `@@symbol:` name construction remains.

These must be migrated to true symbol keys.

### Design overview

- Keys and shapes

  - A shape contains a shared, ordered `fields` array. Each `Field` has:
    - `keyKind` (string or symbol), `stringName` (if string), `symbolPtr` (if symbol), `offset` (index into values), `flags` (writable, enumerable, configurable), and `isAccessor`.
  - Add `version` to `Shape`. Bump on any layout/flags change: add/remove field, flip attributes, data/accessor kind switch, proto change.
  - Lazily add per-shape indexes past a small threshold:
    - `stringToOffset` and `symbolToOffset` maps (or perfect hashing later). Small shapes do linear scan.

- Objects

  - Per-object store only `values []Value` and compact accessor slots (parallel slice or side-array indexed by `offset`).
  - No per-object maps. Dictionary mode reserved for degenerate cases (future).

- Accessors

  - Keep getter/setter in per-object parallel slices indexed by offset. `isAccessor` on the field distinguishes kind.

- Symbol keys

  - No stringization. `Object.defineProperty`/`getOwnPropertyDescriptor` and internal lookups accept symbol keys directly.
  - Enumeration: `Object.keys/values/entries` use only enumerable string keys. `Object.getOwnPropertySymbols` returns symbols. `Reflect.ownKeys` returns strings then symbols.

- Inline caches (ICs)
  - For get: cache `baseShape+version`, `holderShape+version` (proto hit), `keyKind+keyIdentity`, `offset`, `isAccessor`, and `cachedGetter` if accessor.
  - For set: additionally cache `writable` (or full flags) and reuse `offset` for fast writes.
  - Guards: shape/version equality and key identity. On hit, perform direct load/call.
  - Polymorphism: 2–4 entries per site; megamorphic slow path fallback.

### Migration plan

1. Core VM data model

- Update `pkg/vm/object.go`:
  - Add `keyKind` and symbol identity to `Field` (or split storage cleanly) and `Shape.version`.
  - Add shape-level optional indexes and bumping logic in: `DefineOwnProperty`, `DefineAccessorProperty`, `DeleteOwn`, and prototype changes.
  - Replace any remaining name-based symbol handling with real symbol-key lookups.

2. Builtins

- `Object.defineProperty`/`getOwnPropertyDescriptor`:
  - Accept and store symbol keys directly; remove `@@symbol` conversion.
- `Array/String/Generator/Iterator` initializers:
  - Define `[Symbol.iterator]` using a real symbol key, not a string.
- Enumeration builtins:
  - Ensure correct separation of string vs symbol keys.

3. Compiler and checker

- Replace hard-coded `@@symbol:Symbol.iterator` strings with symbol constants:
  - Emit symbol constants in the chunk constant pool for well-known symbols (start with `Symbol.iterator`).
  - Checker: carry symbol identity through where needed instead of constructing strings.

### Getters/Setters and Private Fields

- Current findings

  - Checker encodes getter/setter methods using prefixed string keys like `__get__prop` and `__set__prop` to help the compiler detect optimizable sites.
  - The compiler emits optimistic getter/setter calls (direct method call when known) and falls back to generic `OpGetProp`/`OpSetProp`.
  - Private fields and accessors are stored as prefixed string keys as well (pattern similar to the `@@symbol` hack), which complicates correctness and prevents first-class treatment.

- Target design

  - Accessors are first-class in shapes via `Field.isAccessor`. The getter/setter functions are stored in compact per-object slots keyed by field offset (not by name), avoiding prefixed keys.
  - For public properties with getters/setters, property resolution returns the accessor kind through IC metadata; get-path invokes cached getter, set-path invokes cached setter when present.
  - For private fields (and private accessors), add a distinct key-kind: `keyKindPrivate`, with identity comprised of the declaring class’ private name (interned, not string userland-visible). These never enumerate and are not accessible via property name at runtime.
  - Shapes store private fields alongside public ones (with `keyKindPrivate`) so ICs can still guard and retrieve by offset. Visibility is enforced at compile time and VM op selection rather than by string prefixes.

- Compiler hooks

  - When the checker identifies a getter/setter access and deems it invocable at compile time (e.g., known object type and accessor presence), the compiler can emit an optimistic accessor call path directly:
    - Emit `OpGetProp`/`OpSetProp` as fallback, but also emit a direct `OpCallMethod` using the accessor slot if the IC resolves to accessor kind. This mirrors current optimistic strategy but without relying on `__get__/__set__` prefixed names.
  - For private fields and methods, generate dedicated opcodes or an `OpGetPrivate`/`OpSetPrivate` with a constant pool entry holding the private key identity. VM then uses shape+offset fast path without any name exposure.

- Checker updates

  - Stop materializing `__get__/__set__` names. Instead propagate accessor presence and types on the property itself.
  - For private fields, propagate a private-key identity (tied to the class) so the compiler can emit `OpGetPrivate`/`OpSetPrivate` and the runtime can use the private key kind.

- IC integration
  - Cache accessor kind and cached getter/setter function pointers in the IC entry to avoid re-checking descriptors.
  - For private keys, cache with `keyKindPrivate` and private-key identity; fast-path loads/stores by offset.

4. VM ICs

- Extend op_getprop/op_setprop caches to include shape/version and key identity, and to handle accessors.
  - Implement guarded fast paths; on miss, populate with the resolved info.

5. Test262 glue

- `verifyProperty` and friends in `cmd/paserati-test262/test262_builtins.go`:
  - Stop building `@@symbol:` names; pass symbol keys directly.

### Refactor checklist

- [x] `pkg/vm/object.go`: keys, shapes (key kind + symbol identity), basic symbol lookup helpers; added `OwnSymbolKeys`.
- [~] `pkg/vm/op_getprop.go`, `pkg/vm/op_setprop.go`: introduced `opGetPropSymbol` and routed symbol access for get/index; IC work pending.
- [~] `pkg/builtins/object_init.go`: accept symbol keys directly in `defineProperty`/`getOwnPropertyDescriptor`; added `getOwnPropertyNames`/`getOwnPropertySymbols`.
- [~] `pkg/builtins/array_init.go`, `string_init.go`, `generator_init.go`: register `[Symbol.iterator]` under native symbol keys; transitional `"@@symbol:..."` fallback retained during migration.
- [~] `pkg/compiler/*`: stop emitting `@@symbol:Symbol.iterator`; compile computed keys to fetch `Symbol["iterator"]` at runtime and use OpGetIndex/OpSetIndex with the singleton. Full sweep pending.
- Additionally: compiler now loads the global `Symbol` via the unified heap index (`GetOrAssignGlobalIndex("Symbol")`) rather than a string constant, ensuring runtime/global-index sync.
- [ ] `pkg/checker/*`: propagate symbol identity in member/computed accesses (no stringization); stop suggesting stringized keys.
- [ ] `cmd/paserati-test262/test262_builtins.go`: pass symbol keys directly; remove deletion via `@@symbol:`.

### Testing and validation

- Re-run harness `harness` subset focusing on property and symbol tests.
- Re-run smoke tests.
- Add microbenchmarks: hot property get/set for string and symbol keys (with/without accessors), and proto hits.
- Verify IC hit rates and shape invalidations with debug counters.

### Current status (dev notes)

- VM data model carries key kind and symbol identity on `Field`s; per-object stores only values and compact accessor maps keyed by stable hashes.
- Symbol property gets: `opGetPropSymbol` walks proto chains for plain objects and boxes primitives (strings, arrays, generators) to their prototypes. All legacy "@@symbol:" fallbacks are removed across get/index paths.
- Builtins expose `[Symbol.iterator]` for Array/String/Generator via real symbol keys only; legacy bridge keys removed. Type surfaces use `__COMPUTED_PROPERTY__` for computed symbol members.
- Object builtins accept symbol keys; enumeration APIs split names vs symbols. `DictObject` currently does not support symbol keys (sets/deletes on symbols are ignored rather than stringized).
- Compiler: yield\* and for-of now load the iterator key via `Symbol["iterator"]` at runtime (no symbol construction), then index with that symbol. The global `Symbol` is fetched by heap index (not constant name), synchronizing compiler/VM indices. Object literals store computed methods via `OpGetIndex`/`OpSetIndex` with the singleton.
- Reflect.ownKeys implemented and wired to `Object.__ownKeys` (strings first, then symbols).
- Fixed an `OpGetIndex` early-return bug (string/symbol/callable paths now `continue` the VM loop), which unblocked for-of/yield\* correctness without special-case hacks.
- VM debug prints are gated behind `debugVM=false` by default to keep smoke runs fast.
- IC groundwork: introduced a per-site symbol cache key helper `generateSymbolCacheKey(ip, sym)` (identity by symbol pointer). Next step is to integrate this into IC entries alongside string-key ICs.

### Next steps (short-term)

- IC phase 1: add `Shape.version` and bumping rules in `DefineOwnProperty(…)/DefineAccessorProperty(…)/DeleteOwn(…)/SetPrototypeOf` sites.
  - Version bumps on: field add/remove, data↔accessor kind flip, attribute changes (writable/enumerable/configurable), and prototype change of the base or any holder (see IC design below).
- IC phase 2: extend inline cache entries to guard on `baseShape+baseVersion`, `keyKind+identity`, and return `offset` and `isAccessor` (+cached getter/setter refs).
  - For prototype hits, also record `holderShape+holderVersion` and `protoDepth`.
  - For sets, cache `writable` and reuse `offset` for fast writes; miss if not writable.
- Wire symbol-key ICs: use `generateSymbolCacheKey(ip, sym)` so each symbol access site has its own cache namespace.
- Checker: finish removing all stringization of symbol keys; propagate symbol identity through member/computed accesses; complete class/object literal computed key handling.
- DictObject: either add symbol-key support or keep it out of hot paths (no IC) and document semantics.

Run smoke after each phase; keep `debugVM=false` and add targeted counters to validate hit rates.

### Concrete IC design (detailed)

- Data structures

  - Extend `type Shape` with `version uint32`.
  - Optional per-shape indexes for larger objects: `stringToOffset map[string]int`, `symbolToOffset map[*SymbolObject]int` (added lazily).
  - Extend `PropInlineCache` entry to:
    - `baseShape *Shape`
    - `baseVersion uint32`
    - `holderShape *Shape` (nil for own)
    - `holderVersion uint32`
    - `keyKind KeyKind`
    - `keyStr string` or `keySymPtr *SymbolObject`
    - `offset int`
    - `isAccessor bool`
    - `getter Value` / `setter Value` (if accessor)
    - `protoDepth int8` (0=own)

- Fast-path algorithm (Get)

  1. Compute site cache key:
     - For string: `generateCacheKey(ip, name)`
     - For symbol: `generateSymbolCacheKey(ip, sym)`
  2. Load entry. Guard checks:
     - `obj.shape == baseShape && baseShape.version == baseVersion`
     - if `protoDepth>0`: walk `protoDepth` steps; ensure `holder.shape==holderShape && holderShape.version==holderVersion`.
     - if `keyKind==KeyKindString`: key matches; if symbol: `objKeySymPtr == entry.keySymPtr`.
  3. If accessor: return `createBoundMethod(vm, base, getter)` for get; for set, call cached `setter`.
  4. Else: `return holder.values[offset]` (or base when own).

- Populate on miss (Get)

  - Resolve via current slow path. Identify holder (`own` vs proto object) and its shape/offset/kind/flags.
  - Store all fields into the IC entry as above.

- Set path

  - Same guards + check cached `writable` (or data kind). If accessor, call setter; else write to `holder.values[offset]` when own and writable, otherwise miss/fallback.

- Invalidation sources

  - Any shape mutation (add/remove field, flags flip, accessor/data switch) must bump `Shape.version`.
  - `SetPrototypeOf` or class creation that mutates prototype chains: bump a `protoEpoch` on `VM` or bump `version` on all shapes on that proto chain (cheap option: store `protoEpoch uint32` in IC entry and miss when changed). For simplicity: bump version on the affected prototype object’s shape and rely on holderVersion guard.

- Symbol-key specifics

  - IC site key must include symbol identity (by pointer). We already added `generateSymbolCacheKey(ip, sym)`.
  - `Field` for symbols stores `symbolVal` identity; matching remains pointer-based.

- Accessor caching
  - When accessor is found, IC stores `isAccessor=true` and caches getter/setter values plus offset. Getter binding uses `createBoundMethod(vm, base, getter)`.
  - Miss when accessor shape version changes (including swapping getter/setter functions).

### Implementation plan (PR-sized steps)

1. Shapes

   - Add `version` to `Shape`. Initialize to 1. Bump in `DefineOwnProperty*`, `DefineAccessorProperty*`, `DeleteOwn*`. Ensure transitions copy fields and start with incremented version.
   - Add helpers: `bumpVersion()`; optional `ensureIndexes()` to populate per-shape offset maps lazily.

2. VM Get fast path

   - Extend `PropInlineCacheEntry` with fields above; adjust lookup/update logic for polymorphic sites.
   - Update `opGetProp` and `opGetPropSymbol` to use extended guards and return directly on hit.
   - Populate entries from slow path resolution (`resolvePropertyWithCache`/proto walk).

3. VM Set fast path

   - Mirror Get design in `opSetProp`/`OpSetIndex` for string and symbol keys.

4. Stats and toggles

   - Keep existing cache stats; add hit/miss per-kind (string vs symbol) and accessor-hit counters. Add env toggles to enable/disable IC phases for bisecting regressions.

5. Tests
   - Run smoke after each step. Add microbench benchmarks (hot loops) to observe hit rate and speedups.

### Open items for getters/setters and private fields

- Transition optimistic accessor calls to rely on IC metadata (accessor kind + cached getter/setter) instead of `__get__/__set__` names; add dedicated VM ops if needed for private keys.
- Introduce `keyKindPrivate` with identity tied to declaring class; compiler to emit `OpGetPrivate`/`OpSetPrivate` with constant private identity; VM resolves by shape offset with guards.
- Next up: remove remaining legacy stringization in `vm.go` opcodes (index get/set paths), then update checker/compiler to avoid any `@@symbol:` strings entirely.

### Well-known symbols (singleton) policy

- Creation and ownership

  - Well-known symbols are created exactly once in builtins (e.g., `SymbolIterator`, `SymbolToStringTag`, etc.) and attached to `Symbol` as static properties. These are the only instances in the runtime.

- Compiler requirements

  - Never construct symbols in generated code. Do not emit `NewSymbol` in constant pools.
  - For computed keys like `[Symbol.iterator]`, compile to evaluate `Symbol["iterator"]` at runtime and then use `OpGetIndex`/`OpSetIndex`. This ensures the singleton identity is used without duplication.
  - Optional future optimization: introduce a “WellKnownSymbol” constant tag that resolves at chunk load to the VM’s singleton to skip the `Symbol["iterator"]` property read while still preserving identity.

- VM behavior
  - `OpGetIndex`/`opGetPropSymbol` use symbol identity to resolve properties; primitives (strings) are boxed to their prototypes for symbol lookups. No VM-side construction of symbols occurs along fast paths.

### Notes

- Start with one well-known symbol (`Symbol.iterator`). Add others incrementally.
- Keep small-shape linear scans; promote to indexes lazily to balance memory and speed.

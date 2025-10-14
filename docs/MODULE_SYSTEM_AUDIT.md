# Paserati Module System Audit

**Date**: 2025-10-06
**Goal**: Audit the current module system implementation and identify gaps to achieve:
1. Full ECMAScript module compliance
2. MJS-by-default semantics (modern TS/JS)
3. Modular, extensible module loading architecture
4. Lazy compilation with compile cache
5. Test262 module test compatibility

---

## Executive Summary

**Current State**: Paserati has a well-architected module loading system with good separation of concerns, but it's **not being used consistently**. The codebase operates in two modes:
- **Module mode**: Used when explicitly loading modules via `LoadModule()`
- **Script mode**: Used for REPL, direct file execution, and Test262

**Critical Finding**: Test262 **explicitly skips all module tests** (see `cmd/paserati-test262/main.go:453-456`):
```go
// Skip tests with imports for now (until we have full module support)
if strings.Contains(string(content), "import ") || strings.Contains(string(content), "export ") {
    return false, nil // Skipped
}
```

This means **zero Test262 module tests are running**, making the true module compliance rate **unknown**.

---

## Architecture Analysis

### Current Module System Components

#### 1. **Module Loader** (`pkg/modules/loader.go`)
**Status**: ✅ **Well-designed**, extensible architecture

**Features**:
- Resolver chain with priorities
- Parallel loading with worker pool
- Dependency analysis and topological sorting
- Module registry with caching
- Separation of concerns (parsing, type checking, compilation)

**Interfaces** (`pkg/modules/interfaces.go`):
- `ModuleResolver` - Pluggable resolver interface ✅
- `ModuleLoader` - Main loading coordination ✅
- `ModuleRegistry` - Cache management ✅
- `ParseWorkerPool` - Parallel parsing ✅
- `DependencyAnalyzer` - Dependency tracking ✅

#### 2. **Resolvers**
**Status**: ✅ **Modular** design matches your goals

**Implementations**:
- `NativeModuleResolver` (`pkg/driver/native_module.go`) - Go-based modules ✅
- `FilesystemResolver` (`pkg/modules/resolver_fs.go`) - File system loading ✅
- `MemoryResolver` (`pkg/modules/resolver_memory.go`) - In-memory modules (testing) ✅

**Extensibility**: ✅ Easy to add new resolvers (network, 9p, union FS, etc.)

#### 3. **Module Records** (`pkg/modules/types.go`)
**Status**: ✅ **Comprehensive** state tracking

**Tracked State**:
```go
type ModuleRecord struct {
    Specifier     string
    ResolvedPath  string
    Source        io.ReadCloser
    AST           *parser.Program
    Exports       map[string]types.Type
    ExportValues  map[string]vm.Value
    Dependencies  []string
    State         ModuleState  // Discovered, Parsing, Parsed, Checking, Checked, Compiling, Compiled, Evaluated
}
```

Supports lazy compilation ✅

#### 4. **AST Support** (`pkg/parser/ast.go:1820-2073`)
**Status**: ✅ **Complete** ES6 module syntax

**Supported**:
- `ImportDeclaration` with all specifier types (default, named, namespace) ✅
- `ExportNamedDeclaration` ✅
- `ExportDefaultDeclaration` ✅
- `ExportAllDeclaration` (including `export * as name`) ✅
- Type-only imports/exports (`import type`, `export type`) ✅

#### 5. **Compilation Support**
**Status**: ⚠️ **Partial** - module bindings exist

Files:
- `pkg/compiler/module_bindings.go` - Module-specific binding compilation
- `pkg/checker/module_environment.go` - Module-aware type checking

**Mode Toggle**: Both checker and compiler have `EnableModuleMode()` ✅

---

## Critical Gaps

### 1. **MJS-by-Default Not Enforced**
**Design Goal**: All code should run as modules by default
**Reality**: Script mode is default, module mode is opt-in

**Evidence**:
- REPL runs in script mode
- `Run()` method runs in script mode
- `RunFile()` runs in script mode unless the file imports something
- Only `LoadModule()` uses module mode

**Impact**: Inconsistent semantics between development and module usage

---

### 2. **Test262 Module Tests Completely Skipped**
**Evidence**: `cmd/paserati-test262/main.go:453-456`

```go
// Skip tests with imports for now (until we have full module support)
if strings.Contains(string(content), "import ") || strings.Contains(string(content), "export ") {
    return false, nil // Skipped
}
```

**Test262 Module Test Directories**:
- `test/language/module-code/` - Core module semantics (~200+ tests)
- `test/language/import/` - Import statement tests
- `test/language/export/` - Export statement tests
- `test/language/expressions/dynamic-import/` - Dynamic import() tests

**Estimated Skipped Tests**: **500-1000+ tests**

**Impact**: No validation of module spec compliance

---

### 3. **No Module Resolution Spec Compliance**
**ECMAScript Requirement**: Modules use module records and must resolve imports

**Current Issues**:
- ❌ No validation of module specifier syntax (bare specifiers, relative paths, URLs)
- ❌ No concept of "module type" (JavaScript, JSON, CSS, etc.)
- ❌ No import assertions/attributes support (ECMAScript 2025 feature)
- ⚠️ Filesystem resolver doesn't follow Node.js module resolution algorithm
- ❌ No support for package.json `exports` field
- ❌ No validation of circular dependencies (parsed but not validated per spec)

---

### 4. **VM Module Execution Model Unclear**
**ECMAScript Requirement**: Modules have their own environment, execute only once, and have specific evaluation semantics

**Questions**:
1. Does the VM maintain separate global scopes per module? (Unknown from audit)
2. How are module namespace objects represented? (Not found in VM)
3. Are modules evaluated lazily or eagerly? (Appears eager but not validated)
4. How are live bindings implemented? (Not evident in VM code)
5. Is Top-Level Await supported? (No evidence found)

**Critical**: `pkg/vm/vm.go` doesn't seem to have special module execution paths

---

### 5. **Module-Specific Features Missing**

#### Import Meta (`import.meta`)
**Status**: ❌ **Not implemented**
- Required by ES2020+
- Must provide `import.meta.url` at minimum
- Test262 has tests for this

#### Dynamic Import (`import()`)
**Status**: ❌ **Not implemented**
- Returns Promise<Module>
- Required for lazy loading
- Critical for modern JavaScript

#### Module Namespace Objects
**Status**: ❌ **Not evident in VM**
- Result of `import * as ns from "mod"`
- Should be a frozen exotic object
- Must have Symbol.toStringTag

#### Live Bindings
**Status**: ❌ **Unclear**
- Imported bindings must be "live" (update when exported binding changes)
- Requires special binding semantics
- Test262 tests this extensively

---

### 6. **File Extension Handling**
**Design Goal**: MJS by default
**Reality**: Treats `.ts`, `.js`, and no extension

**Issues**:
- No `.mjs` / `.cjs` distinction
- No validation that `.mts` is module, `.cts` is CommonJS (TypeScript convention)
- File extension affects module/script mode in Node.js and browsers

**Recommendation**: Since you want MJS-by-default, this might be OK, but should be explicit

---

### 7. **Compile Cache Not Implemented**
**Design Goal**: AOT compilation, bytecode cache, compiled deps

**Current State**:
- Module registry caches parsed AST ✅
- Module registry caches compiled bytecode ✅
- BUT: No persistence layer ❌
- No bytecode serialization/deserialization ❌
- No precompiled module distribution ❌

**Path Forward**:
- Implement bytecode serializer
- Add cache directory with hash-based lookup
- Support loading `.pbc` (Paserati Bytecode) files
- Allow distributing precompiled modules

---

## ECMAScript 2025 Module Specification Compliance

### Section 16.2: Module Semantics

**Required**:
1. **Module Records** (16.2.1.4-16.2.1.8)
   - Source Text Module Records ✅ (We have `ModuleRecord`)
   - Abstract Module Records ⚠️ (Partial - missing evaluation state machine)
   - Cyclic Module Records ❌ (Not implemented)

2. **Module Environment Records** (9.1.1.5)
   - Indirect import bindings ❌ (Not validated)
   - Immutable bindings for imports ⚠️ (Need to verify)
   - GetBindingValue for imports ❌ (Must resolve through exported module)

3. **Module Namespace Exotic Objects** (10.4.6)
   - [[Module]] internal slot ❌
   - [[Exports]] internal slot ❌
   - Frozen object ❌
   - Non-standard properties forbidden ❌

### Section 13.3.10: Import Calls

**Dynamic import()** syntax:
```javascript
const module = await import('./module.js');
```

**Status**: ❌ **Not implemented**
**Priority**: HIGH (required for modern JS)

### Section 16.2.1.6: Import Assertions/Attributes

**ECMAScript 2025 adds**:
```javascript
import json from './data.json' with { type: 'json' };
```

**Status**: ❌ **Not supported**
**Priority**: MEDIUM (new feature, less test coverage yet)

---

## Design Goals vs Implementation

| Goal | Status | Gap |
|------|--------|-----|
| **MJS by default** | ❌ | Script mode is default, not module mode |
| **Modular loading** | ✅ | Excellent resolver architecture |
| **FS/VFS/Network loading** | ⚠️ | FS works, others need implementation |
| **Go-based builtins** | ✅ | Native module system works well |
| **AOT compilation** | ❌ | No bytecode serialization/cache |
| **Lazy compilation** | ⚠️ | Framework supports it, not enforced |
| **Type-check once** | ⚠️ | Registry caches, but no persistence |
| **Custom providers** | ✅ | Easy to add new resolvers |
| **Module-first mode** | ❌ | Not default, inconsistently applied |

---

## Test262 Module Test Analysis

### Module Code Tests (`test/language/module-code/`)

**Categories**:
1. **Instantiation** (`instn-*`) - Module linking and binding
2. **Evaluation** (`eval-*`) - Module execution order
3. **Exports** (`export-*`) - Export syntax and semantics
4. **Parse Errors** (`parse-err-*`) - Invalid module syntax
5. **Early Errors** - Static semantic errors

**Example Tests Being Skipped**:
- `instn-star-star-cycle.js` - Cyclic imports with star exports
- `eval-rqstd-order.js` - Module evaluation order
- `export-default-asyncfunction-declaration-binding-exists.js` - Export semantics

**Pass Rate**: **0%** (all skipped)

### Import Tests (`test/language/import/`)

**Coverage**:
- Import syntax validation
- Module resolution semantics
- Binding semantics
- Namespace imports

**Pass Rate**: **0%** (all skipped)

### Export Tests (`test/language/export/`)

**Coverage**:
- Export syntax validation
- Re-export semantics
- Default exports
- Named exports

**Pass Rate**: **0%** (all skipped)

---

## Recommendations

### Phase 1: Module-First Mode (Priority: CRITICAL)

**Goal**: Make module mode the default for all TypeScript/JavaScript execution

**Tasks**:
1. Add `--script` flag to force script mode (for legacy compatibility)
2. Make `Run()`, `RunFile()`, and REPL default to module mode
3. Update Test262 runner to execute module tests in module mode
4. Treat all `.ts` and `.mts` files as modules by default
5. Add `.cts` support for CommonJS-style scripts (if needed)

**Breaking Change**: Yes, but aligns with modern TS/JS semantics

---

### Phase 2: VM Module Execution Model (Priority: CRITICAL)

**Goal**: Implement proper module execution semantics in VM

**Tasks**:
1. Implement Module Namespace Exotic Objects in VM
   - Add `TypeModuleNamespace` value type
   - Implement frozen object with [[Module]] and [[Exports]] slots
   - Prevent property additions/modifications

2. Implement Live Bindings
   - Imported bindings must be references, not copies
   - When exported value changes, import sees new value
   - Requires special binding semantics in VM

3. Add Module Evaluation State Machine
   - Track evaluation state per module (not started, evaluating, evaluated, error)
   - Prevent re-evaluation
   - Handle cyclic dependencies correctly

4. Implement `import.meta` object
   - Add `import.meta.url` with module's URL
   - Support extensibility for custom properties

5. Support Top-Level Await (if not already)
   - Modules can have async evaluation
   - Dependent modules must wait

**Tests**: Create tests/scripts/module_*.ts tests for each feature

---

### Phase 3: Dynamic Import (Priority: HIGH)

**Goal**: Implement `import()` expression returning Promise

**Tasks**:
1. Add `import()` to lexer/parser as CallExpression special case
2. Compile to special bytecode opcode (DYNAMIC_IMPORT)
3. VM must:
   - Load module asynchronously (can be sync internally, just return Promise)
   - Return Promise that resolves to module namespace object
   - Handle loading errors as Promise rejection

4. Add Test262 tests: `test/language/expressions/dynamic-import/`

---

### Phase 4: Module Resolution Spec Compliance (Priority: HIGH)

**Goal**: Implement Node.js-compatible module resolution

**Tasks**:
1. Implement bare specifier resolution
   - `import foo from "foo"` - look in node_modules
   - Support package.json `exports` field
   - Support package.json `main` field fallback

2. Validate specifier syntax
   - Reject invalid URLs
   - Handle file:// URLs
   - Support https:// URLs (for network loader)

3. Implement import assertions (ECMAScript 2025)
   - Parse `with { type: "json" }` syntax
   - Pass assertions to resolver
   - Validate assertion constraints

4. Add module type system
   - JavaScript modules (default)
   - JSON modules (with assertion)
   - CSS modules (future)
   - WebAssembly modules (future)

---

### Phase 5: Bytecode Cache & AOT Compilation (Priority: MEDIUM)

**Goal**: Enable distribution of compiled modules

**Tasks**:
1. Design bytecode format
   - Magic number + version
   - Source hash for validation
   - Serialized chunk + metadata
   - Export table

2. Implement bytecode serialization
   - `Compiler.SerializeBytecode()` → `[]byte`
   - Include type information for exports

3. Implement bytecode deserialization
   - `Compiler.DeserializeBytecode([]byte)` → `*Chunk`
   - Validate hash matches source

4. Add compile cache
   - `~/.paserati/cache/` directory
   - Hash-based lookup (SHA256 of source)
   - Automatic invalidation on source change

5. Support `.pbc` files
   - Precompiled Paserati Bytecode
   - Can be distributed instead of source
   - Resolver priority: `.pbc` > `.ts` > `.js`

6. Add CLI command
   - `paserati compile module.ts -o module.pbc`
   - `paserati compile-deps package/` - compile all dependencies

---

### Phase 6: Advanced Loaders (Priority: LOW)

**Goal**: Support custom module providers

**Tasks**:
1. Network loader
   - `import foo from "https://example.com/foo.ts"`
   - HTTP/HTTPS support with caching

2. 9P filesystem loader
   - Load from Plan 9 filesystem
   - Virtual filesystem support

3. Union filesystem loader
   - Overlay multiple module sources
   - Priority-based resolution

4. Plugin system for custom loaders
   - Allow client code to register loaders
   - Example: Database-backed modules, encrypted modules, etc.

---

## Immediate Action Items

1. **Remove Test262 module skip** (`cmd/paserati-test262/main.go:453-456`)
   - This will immediately show true module compliance rate
   - Expected: Many failures initially

2. **Make module mode default**
   - Update `driver.go` to always enable module mode
   - Add `--script` flag for legacy script mode
   - Update documentation

3. **Fix VM module execution**
   - Implement Module Namespace objects
   - Implement live bindings
   - Add module evaluation state tracking

4. **Add module smoke tests**
   - `tests/scripts/module_import_export.ts`
   - `tests/scripts/module_namespace.ts`
   - `tests/scripts/module_circular.ts`
   - `tests/scripts/module_live_bindings.ts`

5. **Implement `import.meta`**
   - Simple object with `.url` property
   - Should be straightforward

6. **Implement dynamic `import()`**
   - Returns Promise
   - More complex but high-value

---

## Module System Architecture - Ideal State

```
┌─────────────────────────────────────────────────────────────┐
│                         Paserati                            │
│                     (Module-First)                          │
└─────────────────────────────────────────────────────────────┘
                              │
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                      Module Loader                          │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Resolver Chain (Priority-Ordered)                   │  │
│  │  ┌───────────┬──────────┬─────────┬────────────┐    │  │
│  │  │ Native    │  Cache   │  FS     │  Network   │    │  │
│  │  │ (Go mods) │  (.pbc)  │  (.ts)  │  (https://)│    │  │
│  │  └───────────┴──────────┴─────────┴────────────┘    │  │
│  └──────────────────────────────────────────────────────┘  │
│                              │                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Module Registry (Cache)                             │  │
│  │  - Parsed AST                                        │  │
│  │  - Type-checked modules                              │  │
│  │  - Compiled bytecode                                 │  │
│  │  - Export values                                     │  │
│  └──────────────────────────────────────────────────────┘  │
│                              │                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Dependency Analyzer                                 │  │
│  │  - Topological sort                                  │  │
│  │  - Cycle detection                                   │  │
│  │  - Parallel load coordination                        │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ↓                               ↓
    ┌─────────────────┐             ┌─────────────────┐
    │  Type Checker   │             │    Compiler     │
    │  (Module Mode)  │────────────→│  (Module Mode)  │
    └─────────────────┘             └─────────────────┘
                                            │
                                            ↓
                                    ┌─────────────────┐
                                    │       VM        │
                                    │  - Module env   │
                                    │  - Namespace obj│
                                    │  - Live bindings│
                                    │  - import.meta  │
                                    │  - import()     │
                                    └─────────────────┘
```

---

## Summary

**Strengths**:
1. ✅ Excellent modular architecture for loaders and resolvers
2. ✅ Complete AST support for ES6 modules
3. ✅ Good separation of concerns
4. ✅ Native module system for Go integration
5. ✅ Parallel loading infrastructure

**Critical Gaps**:
1. ❌ Module mode not default (violates design goal)
2. ❌ Test262 module tests completely skipped (~1000+ tests)
3. ❌ VM lacks module execution semantics
4. ❌ No dynamic import()
5. ❌ No import.meta
6. ❌ No bytecode cache/AOT compilation

**Recommendation**:
Focus on **Phase 1 and Phase 2** immediately. Once module-first mode is default and VM has proper module semantics, tackle dynamic import() and import.meta. The architecture is solid - it's mainly about making module mode the default and implementing VM-level module features.

**Timeline Estimate**:
- Phase 1 (Module-First): 1-2 days
- Phase 2 (VM Module Model): 3-5 days
- Phase 3 (Dynamic Import): 2-3 days
- Phase 4 (Resolution Spec): 3-5 days
- Phase 5 (Bytecode Cache): 5-7 days
- Phase 6 (Advanced Loaders): As needed

**Total**: ~2-3 weeks for full module compliance and caching

# Re-export Issue Investigation: Evidence, Hypotheses, and Test Plan

## Problem Statement

Re-export functionality (`export * from "./module"`) fails when consumer modules try to import the re-exported values. Specifically:
- Re-export compilation appears to work correctly
- Consumer import resolution fails with `nilRegister R255` error
- Error: `compiler internal error: resolved local variable 'testValue' to nilRegister R255 unexpectedly`

## Evidence Gathered

### 1. **Global Index Allocation Works Correctly** ✅
**Location**: `pkg/compiler/compiler.go:1423` (`GetOrAssignGlobalIndex`)
**Evidence**: 
- Debug output shows builtins correctly assigned indices 0-20
- User variables correctly assigned indices 21+
- No overlap or overwriting of builtin indices
```
// [Compiler] GetOrAssignGlobalIndex: Assigned new index 21 to 'TEST' (total globals: 22)
// [VM] OpSetGlobal: Setting global[21] = R1 (value: 42, type: number)
```

### 2. **Single Module Exports Work Correctly** ✅
**Test**: `tests/scripts/module_global_test.ts`
**Evidence**:
```
// [VM] OpSetGlobal: Setting global[21] = R1 (value: Hello from module!, type: string)
// [VM] OpSetGlobal: Setting global[22] = R1 (value: 42, type: number)
```
- Exports are stored at correct global indices
- Values are accessible and printed correctly

### 3. **Re-export Compilation Logic Is Correct** ✅
**Location**: `pkg/compiler/compiler.go:1650` (`compileExportAllDeclaration`)
**Evidence**:
- Re-export transformation logic follows correct pattern:
  1. `DefineImport(exportName, sourceModule, exportName, ImportNamedRef)`
  2. `DefineExport(exportName, exportName, vm.Undefined, nil)`
  3. `emitImportResolve(tempReg, exportName, line)`
  4. `emitSetGlobal(globalIdx, tempReg, line)`

### 4. **Consumer Import Resolution Fails** ❌
**Location**: Consumer compilation in `simple_reexport_consumer.ts`
**Evidence**:
```
// [Compiler] Imported testValue: testValue = undefined (no module mode)
// DEBUG Identifier 'testValue': Found in symbol table, isLocal=true
// DEBUG Identifier 'testValue': LOCAL variable, register=R255
```
- Import is processed but marked as "no module mode"
- Variable exists in symbol table but resolves to `nilRegister` (R255)
- This suggests the imported variable was defined but never properly initialized

### 5. **Module Context Switching in VM** 🔍
**Location**: `pkg/vm/vm.go:2773` (`executeModule`)
**Evidence**:
- `executeModule` calls `vm.Interpret(chunk)` which creates fresh execution context
- Context saving/restoration at lines 2813-2826:
```go
// Save current execution context
savedFrame := vm.frames[vm.frameCount-1]
savedFrameCount := vm.frameCount
savedNextRegSlot := vm.nextRegSlot

// Execute the module in a new context
result, errs := vm.Interpret(chunk)

// Restore execution context
vm.frameCount = savedFrameCount
vm.nextRegSlot = savedNextRegSlot
vm.frames[vm.frameCount-1] = savedFrame
```

## Hypotheses

### Hypothesis A: **Global Table Setup Difference Between Normal vs Re-export**
**Theory**: Normal exports and re-exports have different global index allocation patterns
**Test**: Compare normal export vs re-export global index assignment patterns
**Key Question**: Does `export const X = 42` vs `export * from "./mod"` result in different global indices for the same exported names?
**Status**: 🔍 To Test

### Hypothesis B: **Module Context Stack vs Cache Issue**
**Theory**: Multi-level module dependencies use simple cache instead of proper context stack
**Evidence**: VM uses `map[string]*ModuleContext` for `moduleContexts`
**Key Question**: When Module A re-exports from Module B, and Module C imports from Module A, are we properly maintaining separate execution contexts?
**Status**: 🔍 To Test

### Hypothesis C: **Module Mode Detection Failure** ⭐ **PRIMARY SUSPECT**
**Theory**: Consumer module compilation doesn't detect module mode correctly
**Evidence**: Debug shows `"no module mode"` during import processing
**Key Question**: Why isn't module mode enabled for the consumer module during test execution?
**Status**: 🔍 To Test

### Hypothesis D: **Symbol Table Context Mismatch**
**Theory**: Re-exported symbols exist in wrong symbol table scope
**Evidence**: Consumer finds variable as "local" but with `nilRegister`
**Key Question**: Are re-exported symbols being stored in the wrong symbol table context?
**Status**: 🔍 To Test

## Investigation Plan

### Phase 1: Isolate Normal Export vs Re-export Behavior

**Test 1A: Normal Export Global Index Pattern**
```typescript
// tests/scripts/debug_normal_export.ts
export const normalValue = 123;
export function normalFunc() { return 456; }
console.log("normalValue:", normalValue);
// expect: 123
```

**Test 1B: Normal Export Consumer**
```typescript
// tests/scripts/debug_normal_consumer.ts  
import { normalValue, normalFunc } from "./debug_normal_export";
console.log("imported normalValue:", normalValue);
console.log("imported normalFunc():", normalFunc());
// expect: 123
```

**Expected Evidence**: If Hypothesis A is correct, we'll see different global index patterns. If both work the same, we can eliminate this hypothesis.

**Status**: 🔍 To Run

### Phase 2: Multi-Level Module Dependency Analysis

**Test 2A: Three-Level Module Chain**
```typescript
// tests/scripts/debug_source.ts
export const deepValue = 999;

// tests/scripts/debug_middle.ts  
export * from "./debug_source";

// tests/scripts/debug_consumer.ts
import { deepValue } from "./debug_middle";
console.log("deepValue:", deepValue);
// expect: 999
```

**Expected Evidence**: If Hypothesis B is correct, this will fail due to context stack issues.

**Status**: 🔍 To Run

### Phase 3: Module Mode Detection Analysis

**Test 3A: Module Mode Status Logging**
Add debug logging to key points:
- `pkg/compiler/compiler.go` in `IsModuleMode()`
- `pkg/driver/driver.go` in `EnableModuleMode()` calls
- Track when and why module mode is enabled/disabled

**Expected Evidence**: If Hypothesis C is correct, we'll see module mode not being enabled for consumer modules.

**Status**: 🔍 To Implement

### Phase 4: Symbol Table Scope Analysis

**Test 4A: Symbol Table State Logging**
Add debug logging to:
- `pkg/compiler/compiler.go` symbol table operations during import processing
- Track which symbol table context variables are stored in
- Compare normal export vs re-export symbol table states

**Expected Evidence**: If Hypothesis D is correct, we'll see re-exported symbols in wrong scope.

**Status**: 🔍 To Implement

## Code Locations to Instrument

### Critical Functions to Add Debug Logging:

1. **`pkg/compiler/compiler.go:645`** - Import resolution in identifier compilation
2. **`pkg/compiler/compile_statement.go`** - Import declaration processing  
3. **`pkg/driver/driver.go:336-337`** - Module mode enabling
4. **`pkg/vm/vm.go:2821`** - Module execution context switching
5. **`pkg/modules/loader.go`** - Module loading and caching logic

### Debug Output to Add:

```go
// In GetOrAssignGlobalIndex
debugPrintf("// [Compiler] Context: enclosing=%p, moduleMode=%v, globalCount=%d\n", 
    c.enclosing, c.IsModuleMode(), c.globalCount)

// In executeModule  
debugPrintf("// [VM] executeModule: '%s', cached=%v, executed=%v\n", 
    modulePath, exists, moduleCtx.executed)

// In import processing
debugPrintf("// [Compiler] Import processing: moduleMode=%v, bindingExists=%v\n",
    c.IsModuleMode(), exists)
```

## Elimination Strategy

### Step 1: Run Test 1A & 1B
- **If both pass**: Eliminate Hypothesis A, proceed to Step 2
- **If 1A passes but 1B fails**: Focus on import resolution logic
- **If both fail**: Focus on fundamental module system issues

### Step 2: Run Test 2A  
- **If passes**: Eliminate Hypothesis B
- **If fails**: Investigate module context stack implementation

### Step 3: Add Module Mode Logging
- **If module mode is correctly enabled**: Eliminate Hypothesis C
- **If module mode detection fails**: Fix module mode detection logic

### Step 4: Add Symbol Table Logging
- **If symbols are in correct scope**: Focus on register allocation
- **If symbols are in wrong scope**: Fix symbol table context handling

## Expected Root Cause Categories

Based on evidence patterns, the root cause is likely one of:

1. **Module loading orchestration**: Test framework or driver not enabling module mode properly
2. **Multi-module context management**: VM context switching corrupting global state
3. **Symbol table context isolation**: Re-exports creating symbols in wrong scope
4. **Import/export binding synchronization**: Timing issue between re-export creation and consumer import

## Investigation Log

### [DATE] - Investigation Start
- Created test plan and hypothesis framework
- Ready to begin systematic testing

---

## Test Results

### Phase 1 Results - **CRITICAL FINDINGS** ⚠️

**Test 1A (Normal Export)**: ✅ PASS
- `normalValue` correctly assigned global index 21
- Export compilation works correctly
- Value accessible within same module

**Test 1B (Normal Export Consumer)**: ❌ FAIL - **SAME ERROR AS RE-EXPORT!**
- Error: `compiler internal error: resolved local variable 'normalValue' to nilRegister R255 unexpectedly`
- Import processing shows: `"normalValue = undefined (no module mode)"`
- **CONCLUSION**: This is NOT a re-export specific issue - it's a fundamental import/export issue!

**Hypothesis A ELIMINATED**: ❌ Global table setup is identical between normal and re-export
**Hypothesis C CONFIRMED**: ✅ Module mode detection failure affects ALL imports, not just re-exports

### Phase 2 Results  
<!-- Results from Test 2A will be recorded here -->

### Phase 3 Results - **ROOT CAUSE IDENTIFIED** 🎯

**Root Cause Location**: `tests/scripts_test.go:238-290` (`compileAndInitializeVM`)

**The Problem**: 
- Test framework uses `compiler.NewCompiler()` without calling `EnableModuleMode`
- Line 290: `comp.Compile(program)` executes with `moduleBindings = nil`
- `IsModuleMode()` returns `false`, causing all imports to be marked as `"no module mode"`

**Evidence**:
- `EnableModuleMode` is only called in `driver.go` for `RunModule`/`RunModuleWithValue`  
- Test framework bypasses driver and calls compiler directly
- **All module-related tests fail**, not just re-exports

**SECONDARY ISSUE IDENTIFIED** 🚨

After fixing the test framework module mode detection, **a more serious issue was revealed**:

**Global Index Collision Between Modules**:
- **Consumer module compiler**: Assigns indices 21+ for exports (after builtins 0-20) ✅
- **Target module compiler**: **Starts fresh with index 0**, overwrites builtins! ❌

**Evidence**:
```
// Consumer compilation - correct indices:
// [Compiler] GetOrAssignGlobalIndex: Assigned new index 20 to 'undefined' (total globals: 21)

// Target module compilation - wrong indices:
// [Compiler] GetOrAssignGlobalIndex: Assigned new index 0 to 'normalValue' (total globals: 1)
// [VM] OpSetGlobal: Setting global[0] = R1 (value: 123)
// [VM] OpSetGlobal: Global[0] name is 'Array'  ⬅️ OVERWRITES ARRAY BUILTIN!
```

**Root Cause**: Each module gets a fresh compiler instance via `moduleLoader.SetCompilerFactory()`, but they all share the same VM global array. Module compilers don't coordinate global index allocation.

**Hypothesis B CONFIRMED**: ✅ Multi-module context management issue

### Phase 4 Results
<!-- Symbol table analysis results will be recorded here -->

---

## Conclusions

### **Investigation Complete** ✅

**Primary Finding**: Re-exports were a **red herring** - the issue affects **ALL module imports/exports**

### **Root Cause Identified**: Global Index Collision Between Module Compilers

**The Problem**:
1. **Test Framework Issue** (Fixed): Test framework wasn't enabling module mode
2. **Global Index Coordination Issue** (Main Problem): Each module gets a fresh compiler instance via `moduleLoader.SetCompilerFactory()`, but they all write to the same VM global array without coordination

**Critical Evidence**:
- **Consumer module**: Correctly assigns global indices 21+ (after builtins 0-20)
- **Target module**: Starts fresh with index 0, **overwrites Array builtin at global[0]**
- **Result**: `Cannot access property 'log' on non-object type 'function'` because `console` references corrupted Array

### **Confirmed Hypotheses**:
- ✅ **Hypothesis B**: Multi-module context management issue
- ✅ **Hypothesis C**: Module mode detection failure (fixed in test framework)
- ❌ **Hypothesis A**: Global table setup differences (eliminated)
- ❌ **Hypothesis D**: Symbol table context mismatch (not the core issue)

### **Key Locations for Fix**:

1. **`tests/scripts_test.go:292-318`** - Test framework module detection (✅ Fixed)
2. **`pkg/compiler/compiler.go:1423`** - `GetOrAssignGlobalIndex()` global allocation
3. **`pkg/modules/loader.go`** - Module loader compiler factory
4. **`pkg/driver/driver.go:648-651`** - Builtin global coordination pattern

### **Next Steps**: Implement Global Index Coordination

**Current Pattern** (works for single session):
```go
// driver.go - Pre-assign global indices in compiler to match VM ordering
for _, name := range globalNames {
    comp.GetOrAssignGlobalIndex(name)  // Assigns 0-20 for builtins
}
```

**Problem**: Module compilers created by `SetCompilerFactory()` don't get this initialization

**Potential Solutions**:
1. **Shared Global Index Coordinator**: All compilers coordinate through shared state
2. **Module-Scoped Globals**: Each module has isolated global space + shared builtins
3. **Cross-Module Index Resolution**: Compile-time resolution of module.export references

### **Architecture Implications**:

The current architecture assumes a single compiler session with coordinated global allocation. The module system breaks this assumption by creating independent compilers that compete for the same global space.

**Fix Priority**: HIGH - This affects all multi-module codebases, not just re-exports.

---

## Proposed Solution Analysis

### **User's Proposed Architecture**: Module-Scoped Global Tables

**Core Concept**: 
- **Global-globals (0-20)**: Shared builtins across all modules
- **Module-locals (21+)**: Each module has isolated global space
- **Cross-module references**: Compile-time resolution via `module_index:global_index`

**Implementation Strategy**:
1. **Copy global-globals (0-20) to each module table** - Builtins available everywhere
2. **Start module allocator from index 21** - Avoid builtin collision
3. **Preserve OpSetGlobal/OpGetGlobal opcodes** - No VM changes needed
4. **Expand MODULE's table, leave global-global intact** - Proper isolation

### **Analysis**: ✅ **Excellent Solution**

**Advantages**:
- ✅ **Backward Compatibility**: No opcode changes required
- ✅ **Proper Isolation**: Modules can't overwrite each other's globals
- ✅ **Performance**: Builtins shared, not duplicated per compiler
- ✅ **Scalability**: Compile-time cross-module resolution potential
- ✅ **Clean Architecture**: Clear separation between global vs module scope

**Technical Benefits**:
- **OpSetGlobal/OpGetGlobal unchanged**: Works with existing bytecode
- **Module independence**: Each module owns indices 21+ in its table
- **Builtin efficiency**: Single copy of Array, console, etc. in indices 0-20
- **Future optimization**: `OpGetModuleGlobal(moduleIdx, globalIdx)` for direct access

**Implementation Locations**:
1. **`pkg/compiler/compiler.go`**: Modify `GetOrAssignGlobalIndex()` for module-scoped allocation
2. **`pkg/vm/vm.go`**: Module-specific global tables in `ModuleContext` 
3. **`pkg/modules/loader.go`**: Initialize module compilers with builtin indices
4. **`pkg/driver/driver.go`**: Coordinate global-global setup

**Comparison with Alternatives**:

| Approach | Isolation | Performance | Compatibility | Complexity |
|----------|-----------|-------------|---------------|------------|
| **Shared Global Coordinator** | ❌ | ⚠️ | ✅ | ⚠️ |
| **Module-Scoped Tables** | ✅ | ✅ | ✅ | ⚠️ |
| **Cross-Module Opcodes** | ✅ | ✅ | ❌ | ❌ |

**Recommended**: **Module-Scoped Tables** - Best balance of all factors

### **Implementation Plan**:

1. **Phase 1**: Modify `ModuleContext` to include per-module global table
2. **Phase 2**: Update `GetOrAssignGlobalIndex()` for module-aware allocation  
3. **Phase 3**: Initialize module compilers with builtin global coordination
4. **Phase 4**: Test cross-module import/export scenarios
5. **Phase 5**: (Future) Add compile-time cross-module resolution opcodes
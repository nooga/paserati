# Module Execution Failure Analysis

## Problem Statement

Re-export functionality (`export * from "./module"`) is failing due to underlying module execution failures. Modules compile successfully but fail during runtime execution with "Cannot call non-function value of type 1" errors, preventing export collection and breaking the dependency chain.

## Current Status

### Working Components
- ✅ Module compilation and bytecode generation
- ✅ Global table coordination between driver and compiler
- ✅ Export name collection from module records
- ✅ Re-export syntax parsing and AST generation
- ✅ Execution context deep copying for register isolation
- ✅ Module caching and execution state management

### Failing Components
- ❌ Module runtime execution (fails with "Cannot call non-function" error)
- ❌ Export value collection (never triggered due to execution failure)
- ❌ Re-export dependency chain resolution

## Investigation Findings

### 1. Module Execution Flow Analysis

**Expected Flow:**
```
simple_reexport_consumer.ts
  → imports from simple_reexport_main.ts
    → re-exports from simple_export_source.ts
      → exports: testValue=123, testFunc=function
```

**Actual Flow:**
```
simple_reexport_consumer.ts
  → OpEvalModule('./simple_reexport_main')
    → OpEvalModule('./simple_export_source') ✅ (compiles)
    → OpGetModuleExport('testValue') ❌ (returns undefined - no exports collected)
    → "Cannot call non-function value" error propagates
```

### 2. Debug Output Evidence

From test execution:
```
// [VM] executeModule: CALLED for module './simple_reexport_main'
// [VM] executeModule: CALLED for module './simple_export_source'  
// [VM DEBUG] === FINISHED MODULE EXECUTION: ./simple_export_source (result: undefined, errors: 1) ===
// [VM] executeModule: Error 0: Runtime Error at 0:0: Cannot call non-function value of type 1
// [VM DEBUG] === FINISHED MODULE EXECUTION: ./simple_reexport_main (result: undefined, errors: 1) ===
// [VM] executeModule: Error 0: Runtime Error at 0:0: Cannot call non-function value of type 1
```

**Key Observations:**
- Modules are called and compiled successfully
- Both modules finish with exactly 1 error each
- No "marked as executed=true" or "exports collected" messages appear
- Modules never reach the success branch in `executeModule()`

### 3. Bytecode Analysis

**simple_export_source.ts bytecode:**
```
== module:./simple_export_source ==
0000      OpLoadConst      R1, 0 ('123')
0004      OpSetGlobal      GlobalIdx 21, R1  
0008      OpLoadConst      R0, 1 ('<function testFunc>')
0012      OpSetGlobal      GlobalIdx 22, R0
0016      OpReturn         R0
```

**Analysis:** This bytecode contains no function calls and should not produce "Cannot call non-function" errors. The operations are:
- Load constants into registers
- Set global variables  
- Return

**simple_reexport_main.ts bytecode:**
```
== module:./simple_reexport_main ==
0000      OpEvalModule     0 ('./simple_export_source')
0003      OpGetModuleExport R1, 0 ('./simple_export_source'), 1 ('testValue')
0009      OpSetGlobal      GlobalIdx 21, R1
0013      OpEvalModule     0 ('./simple_export_source')  
0016      OpGetModuleExport R1, 0 ('./simple_export_source'), 2 ('testFunc')
0022      OpSetGlobal      GlobalIdx 22, R1
0026      OpReturn         R0
```

**Analysis:** Contains `OpEvalModule` and `OpGetModuleExport` operations that depend on successful module execution.

## Hypotheses

### Hypothesis 1: Global Table Context Corruption During Nested Execution
**Theory:** During nested module execution, the global table context switching is corrupted, causing `OpSetGlobal` operations to fail or write to wrong memory locations.

**Evidence:**
- Fixed partial issue with `copy(moduleCtx.globals, vm.globals)` truncation
- Still seeing execution failures after the fix
- Error location "0:0" suggests context/location tracking issues

**Test:** Enable `OpSetGlobal` debug statements to trace where global assignments are failing.

### Hypothesis 2: Module Loader State Corruption
**Theory:** The module loader state becomes corrupted during recursive module loading, causing exports to be unavailable when `OpGetModuleExport` executes.

**Evidence:**
- `simple_export_source` executes but exports are never collected
- `OpGetModuleExport` operations return undefined
- Chicken-and-egg dependency resolution failure

**Test:** Add debug statements in `OpGetModuleExport` to trace export lookup failures.

### Hypothesis 3: Execution Context Stack Corruption
**Theory:** The execution context stack restoration is corrupted during nested module execution, causing register or frame state to be invalid.

**Evidence:**
- Deep copying fix was implemented for registers
- Error occurs during simple operations that shouldn't fail
- Module execution context switching is complex

**Test:** Add validation checks for execution context stack integrity.

### Hypothesis 4: VM State Inconsistency During Module Execution
**Theory:** The VM's global state (registers, frames, globals) becomes inconsistent during module execution transitions.

**Evidence:**  
- Modules compile correctly but fail at runtime
- Error message suggests type confusion ("type 1" instead of function)
- Issue affects even simple modules with no complex operations

**Test:** Add comprehensive VM state validation before and after module execution.

### Hypothesis 5: Error Propagation from Failed OpGetModuleExport
**Theory:** The initial failure is in `OpGetModuleExport` when trying to access exports from a module that hasn't completed execution, and this error is being misreported as "Cannot call non-function".

**Evidence:**
- `simple_export_source` might be failing silently
- `simple_reexport_main` tries to access its exports
- Error propagates up the chain
- Error location "0:0" suggests error reporting issues

**Test:** Add specific error handling and reporting for `OpGetModuleExport` failures.

## Debugging Strategy

### Phase 1: Isolate the Root Failure
1. **Test simple_export_source in isolation**
   - Run the module standalone to see if it executes successfully
   - Check if the issue is in the module itself or in the context

2. **Enable OpSetGlobal debugging**
   - Trace each global variable assignment
   - Verify that globals are being set in the correct module context

3. **Enable OpGetModuleExport debugging**
   - Trace export lookup operations
   - Check if exports exist in the module context when accessed

### Phase 2: Validate Context Management
1. **Add execution context validation**
   - Verify context stack integrity before/after module execution
   - Check that `currentModulePath` is correctly maintained

2. **Add global table validation**
   - Verify that module global tables are correctly sized and populated
   - Check for memory corruption or invalid references

### Phase 3: Error Source Identification
1. **Improve error reporting**
   - Add instruction pointer tracking to error messages
   - Include context information (module path, operation type)

2. **Add VM state snapshots**
   - Capture VM state before/after problematic operations
   - Compare state consistency

## Implementation Plan

### Immediate Actions
1. Enable detailed debugging for `OpSetGlobal` and `OpGetModuleExport`
2. Add module execution validation checks
3. Test hypothesis 1 (global table context corruption)

### Medium-term Actions
1. Implement comprehensive VM state validation
2. Improve error reporting with better context information
3. Add module execution isolation tests

### Success Criteria
- ✅ `simple_export_source` executes successfully and exports are collected
- ✅ `simple_reexport_main` can access exports from `simple_export_source`  
- ✅ `simple_reexport_consumer` can import from `simple_reexport_main`
- ✅ Test output shows "testValue: 123" instead of "testValue: undefined"

## BREAKTHROUGH: Root Cause Identified

After extensive debugging with VM instrumentation, we have identified the **exact root cause**:

### The Problem: Nested vm.run() Execution Loop Bug

**Location**: `pkg/vm/vm.go:3053` in `executeModule()` function:
```go
resultStatus, result := vm.run()  // ← PROBLEMATIC NESTED CALL
```

**What's happening:**

1. **Main `vm.run()` loop** executes re-export module bytecode
2. Hits `OpEvalModule` → calls `executeModule('./simple_export_source')`
3. `executeModule()` switches context to `./simple_export_source`
4. `executeModule()` calls **nested `vm.run()`** to execute source module
5. **Nested `vm.run()`** executes source module bytecode:
   - ✅ `OpLoadConst`, `OpSetGlobal` execute correctly (globals are set)
   - ✅ `OpReturn` should end the nested loop
6. **BUG**: Instead of exiting, the nested `vm.run()` continues executing
7. **BUG**: The nested loop picks up the **re-export module's remaining bytecode** (`OpGetModuleExport` instructions)
8. **BUG**: Those `OpGetModuleExport` instructions execute in the **source module context** (context not restored yet)
9. **BUG**: Modules never get marked as `executed=true` because nested loop doesn't complete properly

### Evidence From Debug Output:

```
// Context switch from './simple_reexport_main' to './simple_export_source'
// OpSetGlobal: global[21] = R1 (123, type: 4) [module: ./simple_export_source] ✅
// OpSetGlobal: global[22] = R0 ([Function: testFunc], type: 8) [module: ./simple_export_source] ✅
// OpGetModuleExport: Getting export 'testValue' from module './simple_export_source' [current module: ./simple_export_source] ❌
// getModuleExport: Module './simple_export_source' found, executed=false, exports count=0 ❌
```

The `OpGetModuleExport` instructions (which belong to the re-export module) are executing in the source module context, proving that the nested `vm.run()` is not properly isolated.

### Frame Isolation Bug

The **nested `vm.run()` call is not properly isolating module execution**. When the source module executes `OpReturn`, it should:
1. Exit the nested `vm.run()` and return control to `executeModule()`
2. `executeModule()` should restore the original context
3. Return control to the main `vm.run()` loop to continue re-export module execution

Instead, the nested `vm.run()` continues executing the calling module's bytecode in the wrong context.

### Impact

This explains all observed symptoms:
- ✅ **Globals set correctly** - Source module executes properly
- ❌ **Modules not marked as `executed=true`** - Nested `vm.run()` doesn't complete properly  
- ❌ **`OpGetModuleExport` runs in wrong context** - Bytecode continues in nested loop
- ❌ **"Cannot call non-function" errors** - Wrong context + undefined exports
- ❌ **Export collection never happens** - Requires `executed=true`

### Solution Required

Fix the **module execution isolation** in the `executeModule()` function to ensure:
1. Nested `vm.run()` only executes the module's own bytecode
2. Proper exit when module completes (`OpReturn`)
3. Correct context restoration after module execution
4. Proper return to the calling module's execution context

## References
- Module execution logic: `pkg/vm/vm.go:executeModule()`
- Export collection: `pkg/vm/vm.go:collectModuleExports()`
- Global table management: `pkg/vm/vm.go:getGlobalTable()`, `setGlobalInTable()`
- Re-export compilation: `pkg/compiler/compile_export.go`
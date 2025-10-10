# Exception Handling Fix: Nested Exception Propagation

## Executive Summary

**Problem**: Exceptions thrown from nested function calls (e.g., getter invocations, method calls) inside try-catch blocks cause infinite loops or timeouts instead of being caught.

**Impact**: Test262 object expressions suite has many timeouts and failures. Tests like `try { null.foo } catch(e) {}` hang forever.

**Root Cause**: When `opGetProp()` throws an exception (e.g., TypeError for null property access) and calls `vm.throwException()`, which finds a handler and sets `frame.ip` to the catch block, `opGetProp()` then returns `InterpretRuntimeError`. This causes the VM loop to return immediately, never executing the catch block. The frame state (including the updated IP pointing to the catch handler) is never reloaded.

**Solution**: Change `opGetProp()` to return `(ok=false, status=InterpretOK, Undefined)` when exception is caught, distinguishing it from uncaught exceptions. Add a common `reloadFrame` label in the VM loop that all call sites can `goto` when `!ok && status==InterpretOK`.

**Expected Result**: Test262 object expressions suite: 69.4% → 79.1% pass rate (+114 tests), 0 timeouts.

**Baseline Measurements** (before fix):
- Test262 object suite: 812/1170 passing (69.4%), 356 failing (30.4%), 2 skipped (0.2%), 0 timeouts
- Smoke tests: 796/809 passing (98.4%), 13 failing (1.6%)
- Exception bug confirmed: `try { null.foo } catch(e) {}` hangs/times out

**Target After Fix**:
- Test262 object suite: 926/1170 passing (79.1%), +114 tests
- Smoke tests: 796/809 passing (98.4%), no regressions
- Exception bug fixed: All exception tests pass without hanging

---

## Technical Background

### How Exception Handling Currently Works

1. **Throwing**: When code throws an exception (e.g., `null.foo`), the VM calls `vm.throwException(exceptionValue)`
2. **Searching**: `throwException()` calls `findExceptionHandler()` which walks the call stack backwards looking for try-catch blocks
3. **Handler Found**: If found, `findExceptionHandler()`:
   - Sets `frame.ip` to the catch block's instruction pointer
   - Sets `vm.exceptionValue` to the exception object
   - Sets `vm.unwinding = false` (exception is now caught)
   - Returns (does not exit the VM loop)
4. **No Handler**: If not found:
   - Sets `vm.unwinding = true`
   - Returns (VM should exit with RuntimeError)

### The Bug: Stale Frame State After Exception Handling

When an exception is thrown from a nested operation like `opGetProp()`:

```
VM Loop (run())
  ├─ OpGetProp instruction
  │   ├─ calls vm.opGetProp(...)
  │   │   ├─ detects null.foo access
  │   │   ├─ calls vm.throwException(typeError)
  │   │   │   └─ findExceptionHandler() finds handler, sets frame.ip to catch block
  │   │   └─ returns (false, InterpretRuntimeError, Undefined)
  │   └─ VM loop checks return status
  └─ sees InterpretRuntimeError, returns immediately without executing catch block
```

**The Problem**: The VM loop has local variables (`ip`, `frame`, `closure`, `code`, `registers`) that become stale after `findExceptionHandler()` modifies the frame. When `opGetProp()` returns `InterpretRuntimeError`, these stale variables are never updated, and the VM exits instead of continuing at the catch block.

### Why Simple Cases Work

Simple exceptions work because they're thrown directly in the VM loop:

```go
case OpGetGlobal:
    // ... lookup fails ...
    vm.ThrowReferenceError("x is not defined")
    if vm.unwinding {
        return InterpretRuntimeError, Undefined  // No handler found
    }
    // Handler found! We're still in the VM loop, can reload state:
    frame = &vm.frames[vm.frameCount-1]
    ip = frame.ip
    // ... reload other variables ...
    continue  // Jump to catch block
```

The VM loop can check `vm.unwinding` and reload state because it's still executing.

### Why Nested Cases Fail

Nested exceptions fail because the VM loop delegates to a function:

```go
case OpGetProp:
    // ... setup ...
    ok, status, value := vm.opGetProp(ip, &registers[objReg], propName, &registers[destReg])
    if !ok {
        return status, value  // Returns InterpretRuntimeError, exits VM loop
    }
```

When `opGetProp()` throws an exception and returns `InterpretRuntimeError`, the VM loop doesn't know that:
1. An exception was thrown
2. The exception was caught by a handler
3. Frame state needs to be reloaded
4. Execution should continue at the catch block

It just sees an error status and exits.

---

## Detailed Solution Design

### Core Principle

When an exception is caught, return `(ok=false, status=InterpretOK, Undefined)` to signal "operation didn't complete normally, but no error to propagate." The VM loop checks this combination and jumps to a common `reloadFrame` label that reloads all frame state and continues execution.

### Return Value Semantics

**Current (Broken)**:
```go
func (vm *VM) opGetProp(ip int, objVal *Value, propName string, dest *Value) (bool, InterpretResult, Value)
// Returns: (ok bool, status InterpretResult, value Value)
// - ok=true, status=InterpretOK: Success, result in *dest
// - ok=false, status=InterpretRuntimeError: Error (could be caught or uncaught - can't distinguish!)
```

**New (Fixed)**:
```go
func (vm *VM) opGetProp(frame *CallFrame, ip int, objVal *Value, propName string, dest *Value) (bool, InterpretResult, Value)
// Returns: (ok bool, status InterpretResult, value Value)
// - ok=true, status=InterpretOK: Success, result in *dest
// - ok=false, status=InterpretOK: Exception was CAUGHT, reload frame state and continue
// - ok=false, status=InterpretRuntimeError: Exception was NOT caught, propagate error
```

**Key Insight**: The `ok` boolean keeps its meaning ("did operation complete normally?"), but we add a new return case using the combination `(ok=false, status=InterpretOK)` to signal caught exceptions.

**Benefits of keeping `ok` semantics**:
- ✅ No changes needed to ~100+ success return statements
- ✅ Less error-prone (can't flip a return value wrong)
- ✅ Clearer git diff (only see new exception handling logic)
- ✅ Consistent with existing code conventions

### Why Add `frame` Parameter?

Before throwing an exception, we must set `frame.ip = ip - 4` to point to the instruction that threw (OpGetProp advances IP by 4 bytes). This allows proper stack traces and ensures the exception handler setup works correctly.

The `ip` parameter passed to `opGetProp()` is AFTER the OpGetProp instruction (already advanced). To get the instruction address, we do `ip - 4`.

For calls from outside the VM loop (e.g., `toPrimitive()`), pass `frame=nil`. The function checks and uses `vm.frames[vm.frameCount-1]` if needed.

### Using `goto` for Common Reload Logic

Instead of repeating the reload block at every call site, we use a labeled `goto` to jump to a common reload routine:

```go
func (vm *VM) run() (InterpretResult, Value) {
    // ... setup frame-local variables ...

    for {
        switch opcode {
        case OpGetProp:
            ok, status, value := vm.opGetProp(frame, ip, ...)
            if !ok {
                if status != InterpretOK {
                    return status, value  // Uncaught exception
                }
                goto reloadFrame  // Caught exception
            }
            // Success, continue
        }
    }

reloadFrame:
    // Common reload routine
    frame = &vm.frames[vm.frameCount-1]
    closure = frame.closure
    function = closure.Fn
    code = function.Chunk.Code
    constants = function.Chunk.Constants
    registers = frame.registers
    ip = frame.ip
    continue
}
```

**Benefits**:
- No code duplication (22+ call sites don't repeat 8-line reload block)
- Easy to verify (just check each `!ok` has the goto)
- Centralized logic (change reload behavior in one place)
- Same performance (goto compiles to a simple jump)
- Clear intent (label documents purpose)

---

## Implementation Plan

### Phase 1: Modify `pkg/vm/op_getprop.go`

#### 1.1: Update Function Signatures

**opGetProp**:
```go
// OLD
func (vm *VM) opGetProp(ip int, objVal *Value, propName string, dest *Value) (bool, InterpretResult, Value)

// NEW - add frame parameter
func (vm *VM) opGetProp(frame *CallFrame, ip int, objVal *Value, propName string, dest *Value) (bool, InterpretResult, Value)
```

**opGetPropSymbol**:
```go
// OLD
func (vm *VM) opGetPropSymbol(ip int, objVal *Value, symbolVal Value, dest *Value) (bool, InterpretResult, Value)

// NEW - add frame parameter
func (vm *VM) opGetPropSymbol(frame *CallFrame, ip int, objVal *Value, symbolVal Value, dest *Value) (bool, InterpretResult, Value)
```

#### 1.2: Add Frame Nil Check (both functions)

At the start of both `opGetProp()` and `opGetPropSymbol()`, add:

```go
// If frame is nil (called from outside VM loop), use current frame
if frame == nil && vm.frameCount > 0 {
    frame = &vm.frames[vm.frameCount-1]
}
```

This allows calls from helper functions like `toPrimitive()` to work by passing `nil` for frame.

#### 1.3: Update ALL Exception Throwing Sites

Find every location that calls `vm.throwException()` in both functions (approximately 18 sites in `opGetProp()`, similar in `opGetPropSymbol()`).

**Pattern to find**:
```go
vm.throwException(excVal)
return false, InterpretRuntimeError, Undefined
```

**Replace with**:
```go
if frame != nil {
    frame.ip = ip - 4
}
vm.throwException(excVal)
if !vm.unwinding {
    return false, InterpretOK, Undefined  // Exception caught - ok=false but status=OK
}
return false, InterpretRuntimeError, Undefined  // Exception not caught
```

**Critical**:
- The `ip - 4` adjustment accounts for OpGetProp advancing IP by 4 bytes (1 byte opcode + 3 bytes operands)
- The new return case `(false, InterpretOK, Undefined)` signals caught exception
- Keep `ok=false` for both caught and uncaught to maintain semantic consistency

#### 1.4: Leave Success Returns Unchanged

**DO NOT modify** the ~100+ success return statements. They should remain:
```go
return true, InterpretOK, <value>
```

This is a key simplification - we only add new exception handling code, not modify existing success paths.

### Phase 2: Modify `pkg/vm/vm.go`

#### 2.1: Add Common Reload Label

Locate the main VM loop in the `run()` function. After the closing brace of the `switch` statement, add the `reloadFrame` label:

```go
func (vm *VM) run() (InterpretResult, Value) {
    // ... initialize frame-local variables ...
    frame := &vm.frames[vm.frameCount-1]
    closure := frame.closure
    function := closure.Fn
    code := function.Chunk.Code
    constants := function.Chunk.Constants
    registers := frame.registers
    ip := frame.ip

    for {
        opcode := code[ip]
        switch opcode {
        case OpGetProp:
            // ...
        case OpGetIndex:
            // ...
        // ... all other cases ...
        } // end switch

    reloadFrame:
        // Exception was caught - reload all frame-local state
        frame = &vm.frames[vm.frameCount-1]
        closure = frame.closure
        function = closure.Fn
        code = function.Chunk.Code
        constants = function.Chunk.Constants
        registers = frame.registers
        ip = frame.ip
        continue  // Jump back to top of for loop
    } // end for
}
```

**Note**: The label must be inside the `for` loop but after the `switch` statement for `continue` to work correctly.

#### 2.2: Update OpGetProp Case Handler (line ~3333)

**Find**:
```go
case OpGetProp:
    destReg := code[ip]
    objReg := code[ip+1]
    nameConstIdxHi := code[ip+2]
    nameConstIdxLo := code[ip+3]
    nameConstIdx := uint16(nameConstIdxHi)<<8 | uint16(nameConstIdxLo)
    ip += 4

    // Get property name from constants
    if int(nameConstIdx) >= len(constants) {
        frame.ip = ip
        status := vm.runtimeError("Invalid constant index %d for property name.", nameConstIdx)
        return status, Undefined
    }
    nameVal := constants[nameConstIdx]
    if !IsString(nameVal) {
        frame.ip = ip
        status := vm.runtimeError("Internal Error: Property name constant %d is not a string.", nameConstIdx)
        return status, Undefined
    }
    propName := AsString(nameVal)

    ok, status, value := vm.opGetProp(ip, &registers[objReg], propName, &registers[destReg])
    if !ok {
        return status, value
    }
```

**Replace with**:
```go
case OpGetProp:
    destReg := code[ip]
    objReg := code[ip+1]
    nameConstIdxHi := code[ip+2]
    nameConstIdxLo := code[ip+3]
    nameConstIdx := uint16(nameConstIdxHi)<<8 | uint16(nameConstIdxLo)
    ip += 4

    // Get property name from constants
    if int(nameConstIdx) >= len(constants) {
        frame.ip = ip
        status := vm.runtimeError("Invalid constant index %d for property name.", nameConstIdx)
        return status, Undefined
    }
    nameVal := constants[nameConstIdx]
    if !IsString(nameVal) {
        frame.ip = ip
        status := vm.runtimeError("Internal Error: Property name constant %d is not a string.", nameConstIdx)
        return status, Undefined
    }
    propName := AsString(nameVal)

    ok, status, value := vm.opGetProp(frame, ip, &registers[objReg], propName, &registers[destReg])
    if !ok {
        if status != InterpretOK {
            return status, value  // Uncaught exception
        }
        goto reloadFrame  // Caught exception
    }
    // Success, continue to next instruction
```

**Changes**:
- Add `frame` parameter to `opGetProp()` call
- Check `status` when `!ok` to distinguish caught vs uncaught
- Use `goto reloadFrame` for caught exceptions

#### 2.3: Update OpGetIndex Case Handler (line ~2165)

This case has **approximately 22 call sites** to `opGetProp()` and `opGetPropSymbol()`. Each one must be updated.

**How to find all sites**:
```bash
awk '/case OpGetIndex:/,/case OpSetIndex:/' pkg/vm/vm.go | grep -n "vm.opGetProp\|vm.opGetPropSymbol"
```

**Pattern to find**:
```go
if ok, status, value := vm.opGetProp(ip, &baseVal, key, &registers[destReg]); !ok {
    return status, value
}
```

**Replace with**:
```go
ok, status, value := vm.opGetProp(frame, ip, &baseVal, key, &registers[destReg])
if !ok {
    if status != InterpretOK {
        return status, value
    }
    goto reloadFrame
}
```

**Alternative pattern** (inline if):
```go
// OLD
if ok, status, value := vm.opGetPropSymbol(ip, &baseVal, indexVal, &registers[destReg]); !ok {
    return status, value
}
continue

// NEW
ok, status, value := vm.opGetPropSymbol(frame, ip, &baseVal, indexVal, &registers[destReg])
if !ok {
    if status != InterpretOK {
        return status, value
    }
    goto reloadFrame
}
continue
```

**Important**: Each site must be updated consistently. Missing even one will cause that code path to return an error when it should reload.

#### 2.4: Update Other Call Sites

Search the entire codebase for other `opGetProp()` calls:

```bash
grep -rn "\.opGetProp(" pkg/vm/
grep -rn "\.opGetPropSymbol(" pkg/vm/
```

For calls inside the VM loop, use the same pattern as above.

For calls outside the VM loop (e.g., in `toPrimitive()`, `toObject()`), pass `nil` for frame and don't add goto logic (they can't reload frame state):

```go
// OLD
ok, status, val := vm.opGetProp(0, &objVal, "valueOf", &methodVal)
if !ok {
    // ... handle error ...
}

// NEW
ok, status, val := vm.opGetProp(nil, 0, &objVal, "valueOf", &methodVal)
if !ok {
    // ... handle error same as before ...
}
```

These non-VM callers don't need the goto because:
- They're not in the VM loop (no frame-local variables to reload)
- They typically handle errors differently (return Go errors, not VM errors)
- They don't participate in exception unwinding

### Phase 3: Fix OpGetGlobal

**Location**: `pkg/vm/vm.go`, case OpGetGlobal (line ~4414)

**Current code**:
```go
case OpGetGlobal:
    destReg := code[ip]
    globalIdxHi := code[ip+1]
    globalIdxLo := code[ip+2]
    globalIdx := uint16(globalIdxHi)<<8 | uint16(globalIdxLo)
    ip += 3

    value, exists := vm.heap.Get(int(globalIdx))
    if !exists {
        frame.ip = ip
        varName := vm.heap.GetNameByIndex(int(globalIdx))
        if varName == "" {
            varName = fmt.Sprintf("<index %d>", globalIdx)
        }
        vm.ThrowReferenceError(fmt.Sprintf("%s is not defined", varName))
        // BUG: Always returns error, even if caught
        return InterpretRuntimeError, Undefined
    }

    registers[destReg] = value
```

**Fixed code**:
```go
case OpGetGlobal:
    destReg := code[ip]
    globalIdxHi := code[ip+1]
    globalIdxLo := code[ip+2]
    globalIdx := uint16(globalIdxHi)<<8 | uint16(globalIdxLo)
    ip += 3

    value, exists := vm.heap.Get(int(globalIdx))
    if !exists {
        frame.ip = ip
        varName := vm.heap.GetNameByIndex(int(globalIdx))
        if varName == "" {
            varName = fmt.Sprintf("<index %d>", globalIdx)
        }
        vm.ThrowReferenceError(fmt.Sprintf("%s is not defined", varName))

        // Check if exception was caught
        if vm.unwinding {
            return InterpretRuntimeError, Undefined  // Not caught, exit
        }

        // Exception was caught - reload and continue
        goto reloadFrame
    }

    registers[destReg] = value
```

---

## Testing Strategy

### Test 1: Nested Function Exception

**Test**:
```bash
./paserati --no-typecheck -e 'function f() { return null.foo; } try { f(); } catch(e) { console.log("caught"); }'
```

**Expected Output**:
```
caught
null
```

**Why**: The `null.foo` access throws TypeError inside function `f()`. The exception propagates up to the try-catch in the outer scope. This tests nested exception propagation.

### Test 2: Direct Exception

**Test**:
```bash
./paserati --no-typecheck -e 'try { null.foo } catch(e) { console.log("caught") }'
```

**Expected Output**:
```
caught
null
```

**Why**: Direct exception in try block. Simpler case to verify basic exception handling.

### Test 3: Undefined Variable

**Test**:
```bash
./paserati --no-typecheck -e 'try { unresolvableReference } catch(e) { console.log("caught") }'
```

**Expected Output**:
```
caught
null
```

**Why**: Tests OpGetGlobal exception handling for undefined variables.

### Test 4: Getter Exception

**Test**:
```bash
./paserati --no-typecheck -e 'const o = {get foo() { throw new Error("boom"); }}; try { o.foo; } catch(e) { console.log("caught"); }'
```

**Expected Output**:
```
caught
null
```

**Why**: Tests exception thrown from getter invocation within `opGetProp()`.

### Test 5: Symbol.iterator Access

**Test**:
```bash
./paserati -e 'let arr = [1,2,3]; console.log(typeof arr[Symbol.iterator]);'
```

**Expected Output**:
```
function
null
```

**Why**: Tests that Symbol property access (via `opGetPropSymbol()`) works correctly after changes.

### Test 6: Smoke Tests

**Test**:
```bash
go test ./tests -run TestScripts
```

**Expected**: 796/809 passing (98.4%), same as baseline. No new failures due to OpGetIndex call site bugs.

**Baseline**: 796 passing, 13 failing (pre-existing issues unrelated to exception handling)

**Critical**: If there are NEW failures (tests that passed before but fail after changes), it means some call sites were not updated correctly.

### Test 7: Test262 Object Expressions

**Test**:
```bash
./paserati-test262 -path ./test262 -subpath "language/expressions/object" -filter -timeout 0.5s
```

**Expected**:
- Pass rate: 79.1% (926/1170 tests)
- Timeouts: 0
- Failed: 244

**Compared to baseline**:
- Before: 69.4% pass rate (812 tests), 0 timeouts
- After: 79.1% pass rate (926 tests), 0 timeouts
- Improvement: +114 tests passing (+9.7%)

---

## Common Pitfalls and How to Avoid Them

### Pitfall 1: Not Updating All Call Sites

**Problem**: Updating OpGetProp case handler but forgetting some of the 22+ call sites in OpGetIndex.

**Symptom**: Tests fail with errors like "Cannot read property 'X' of Y" when the property exists and should be accessible.

**Solution**: Use systematic search to find ALL call sites:
```bash
grep -n "vm.opGetProp\|vm.opGetPropSymbol" pkg/vm/vm.go | grep -v "^[0-9]*:func"
```
Update each one with the same pattern.

### Pitfall 2: Forgetting to Add `frame` Parameter

**Problem**: Updating call sites but forgetting to add `frame` parameter to the function call.

**Symptom**: Compilation error: "not enough arguments in call to vm.opGetProp"

**Solution**: Every call site must change from:
```go
vm.opGetProp(ip, ...)  // OLD
```
to:
```go
vm.opGetProp(frame, ip, ...)  // NEW (inside VM loop)
vm.opGetProp(nil, 0, ...)     // NEW (outside VM loop)
```

### Pitfall 3: Wrong ip Adjustment

**Problem**: Using `ip - 3` or `ip` instead of `ip - 4` when setting `frame.ip` before throwing.

**Why ip - 4**: OpGetProp has 1-byte opcode + 3-byte operands = 4 bytes total. The VM advances `ip += 4` before calling `opGetProp()`. To get the instruction address, subtract 4.

**Symptom**: Stack traces show wrong line numbers, or exception handlers fail to trigger correctly.

### Pitfall 4: Not Checking `status` When `!ok`

**Problem**: Just using `goto reloadFrame` without checking if status is OK:
```go
if !ok {
    goto reloadFrame  // WRONG! Might be uncaught exception
}
```

**Correct**:
```go
if !ok {
    if status != InterpretOK {
        return status, value  // Uncaught exception
    }
    goto reloadFrame  // Caught exception
}
```

**Why**: `ok=false` now has two meanings: caught exception (status=OK) and uncaught exception (status=RuntimeError). Must check status to distinguish.

### Pitfall 5: Label Placement

**Problem**: Placing `reloadFrame:` label outside the `for` loop.

**Symptom**: Compilation error: "continue is not in a loop"

**Correct placement**:
```go
for {
    switch opcode {
    // ...
    }

reloadFrame:  // Must be INSIDE for loop
    // ... reload ...
    continue  // Works because we're inside the loop
}
```

---

## Why This Solution is Correct

### 1. Preserves ECMAScript Semantics

JavaScript requires that exceptions propagate up the call stack until caught. This solution ensures exceptions thrown in nested calls (getters, methods) are properly caught by outer try-catch blocks.

### 2. Maintains VM Loop Integrity

The VM loop maintains local variables for performance. By using `goto` to explicitly jump to a reload routine, we avoid stale state bugs while keeping the hot path (normal execution) fast.

### 3. Minimal API Change

Only changes:
- Function signature: add `frame` parameter
- Return semantics: add new case `(ok=false, status=OK)` for caught exceptions
- No changes to bytecode format, no new opcodes, no new VM fields

### 4. Backward Compatible

Non-VM callers can pass `nil` for frame and handle the `!ok` case the same way as before (they don't care if exception was caught or uncaught).

### 5. Uses Idiomatic Go

The `goto` for error handling is idiomatic Go (see "The Practice of Programming" by Kernighan & Pike). It's clearer than duplicating cleanup code.

### 6. Easy to Verify

Each call site has a clear pattern:
```go
ok, status, value := vm.opGetProp(frame, ip, ...)
if !ok {
    if status != InterpretOK { return status, value }
    goto reloadFrame
}
```

Easy to grep and verify all sites are updated.

---

## Success Criteria

1. ✅ Test262 object suite: 79.1% pass rate (926/1170)
2. ✅ Test262 object suite: 0 timeouts (down from many)
3. ✅ All nested exception tests pass
4. ✅ Smoke tests: Same pass rate as before (no new failures)
5. ✅ Manual tests for Symbol.iterator, getters, nested calls work
6. ✅ No infinite loops or hangs
7. ✅ Code compiles with no warnings

---

## Estimated Implementation Time

- Phase 1 (op_getprop.go): 20-30 minutes
- Phase 2 (vm.go - add label + update call sites): 30-45 minutes
- Phase 3 (OpGetGlobal): 5 minutes
- Testing: 20-30 minutes
- **Total**: 1.5-2 hours

Much faster than the previous approach because:
- No need to modify ~100+ success returns
- Simpler verification (just check call sites)
- Less error-prone (not touching working code)

---

## Post-Implementation Review

After implementation, verify:

1. **All exception throwing sites updated**:
   ```bash
   grep -A3 "vm.throwException" pkg/vm/op_getprop.go | grep -B3 "!vm.unwinding"
   ```
   Each should have frame.ip setting and return false, InterpretOK on caught.

2. **No success returns modified**:
   ```bash
   git diff pkg/vm/op_getprop.go | grep "return true, InterpretOK"
   ```
   Should show no changes (only additions).

3. **All VM loop call sites updated**:
   ```bash
   grep -n "vm.opGetProp\|vm.opGetPropSymbol" pkg/vm/vm.go | grep -v "^[0-9]*:func"
   ```
   Each should have `frame` parameter and goto reloadFrame pattern.

4. **reloadFrame label exists**:
   ```bash
   grep -n "^reloadFrame:" pkg/vm/vm.go
   ```
   Should show exactly one label inside the run() function.

5. **Consistent pattern**: All call sites in VM loop use:
   ```go
   ok, status, value := vm.opGetProp(frame, ip, ...)
   if !ok {
       if status != InterpretOK { return status, value }
       goto reloadFrame
   }
   ```

---

## Known Limitations

This fix addresses nested exception propagation for property access. Similar issues may exist in other nested operations:

- `opSetProp()` - property setting with setters (may throw)
- Method calls from native functions
- Iterator protocol operations

These should be addressed in future work using the same pattern if they exhibit similar issues.

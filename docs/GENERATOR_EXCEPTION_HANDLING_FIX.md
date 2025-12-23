# Generator Exception Handling Fix

**Date**: 2025-12-23
**Issue**: Async generator exception handling fixes broke sync generator exception propagation
**Impact**: +97 async generator tests passing, -114 sync generator/destructuring tests failing
**Root Cause**: Duplicate throw detection preventing re-throws across native boundaries

---

## Executive Summary

The recent changes to fix async generator exception handling (converting exceptions to rejected promises) introduced a regression in sync generator exception propagation. The root cause is NOT in the generator-specific changes, but in a fundamental bug in the exception unwinding mechanism that prevents exceptions from propagating correctly across native function boundaries.

**Key Findings**:
1. The exception unwinding system has a "crossed native boundary" flag (`unwindingCrossedNative`) that tracks when exceptions cross from bytecode → native → bytecode
2. Duplicate throw detection (added to prevent infinite loops) incorrectly blocks legitimate re-throws across native boundaries
3. Generator frame cleanup was too aggressive, breaking subsequent generator operations after exceptions
4. Async generators need special handling (convert to rejected promises), sync generators need normal exception propagation

**Solution**: Fix the duplicate throw detection to allow re-throws when crossing native boundaries, remove premature frame cleanup, and keep async generator promise conversion.

---

## Detailed Investigation

### Test Results Analysis

**New Passes (+97)**:
- All async generator `yield*` error handling tests
- Tests like `language/expressions/async-generator/named-yield-star-getiter-async-get-abrupt.js`
- Pattern: Exceptions during async generator iteration should be converted to rejected promises

**New Failures (-114)**:
- All destructuring pattern tests with generators and elision
- Tests like `language/expressions/arrow-function/dstr/ary-ptrn-elision-step-err.js`
- Pattern: Sync generators throwing during destructuring iteration should propagate exceptions normally

### Exception Flow Trace

Using debug output from a simple sync generator exception:

```typescript
function* g() {
  throw new Error("TestError");
}

try {
  g().next();
} catch (e) {
  console.log("Caught:", e.message);
}
```

**Expected**: Exception caught by try-catch
**Actual**: Uncaught exception printed, try-catch not executed

**Debug Trace**:
```
[DEBUG OpThrow] About to throw at IP 34
[DEBUG throwException] exception=[object Object], frameCount=3, unwinding=false, crossedNative=false
[DEBUG unwindException] Checking frame 2 (g) at IP 34, isDirectCall=true
[DEBUG unwindException] Hit direct call boundary at frame 2 on FIRST PASS; marking crossed and stopping
[DEBUG throwException] exception=[object Object], frameCount=1, unwinding=true, crossedNative=true
[DEBUG throwException] Duplicate throw of same exception during unwind; ignoring rethrow  ← BUG!
```

**Call Stack During Exception**:
```
Frame 0: <script>              [Has try-catch handler]
  ↓ calls Generator.prototype.next() (native)

Frame 1: <sentinel>            [Marks vm.run() boundary]
Frame 2: g (generator body)    [isDirectCall=true, exception thrown here]
```

**Unwinding Steps**:
1. Exception thrown in Frame 2 (generator body)
2. `unwindException()` searches Frame 2 for handlers - none found
3. Frame 2 has `isDirectCall=true` and `unwindingCrossedNative=false`
4. Set `unwindingCrossedNative=true`, return to native caller
5. Native code (Generator.prototype.next in OpCallMethod) receives error
6. OpCallMethod error handler converts to ExceptionError and calls `vm.throwException()`
7. **BUG**: `throwException()` sees `unwinding=true` and same exception value
8. **BUG**: Treats as duplicate throw, silently returns without unwinding
9. Exception never reaches Frame 0 handler - printed as uncaught

### Root Cause: Duplicate Throw Detection

**Location**: `pkg/vm/exceptions.go:51-56`

```go
// Avoid double-throwing the same value in a single unwinding sequence
if vm.unwinding && vm.currentException.Is(value) {
    if debugExceptions {
        fmt.Printf("[DEBUG exceptions.go] Duplicate throw of same exception during unwind; ignoring rethrow\n")
    }
    return  // ← Blocks legitimate re-throws across native boundaries
}
```

**Intent**: Prevent infinite loops from repeatedly throwing the same exception
**Problem**: Doesn't distinguish between:
- **True duplicate**: Same exception thrown multiple times in tight loop (should block)
- **Native boundary re-throw**: Exception propagating from bytecode → native → bytecode (should allow)

The flag `vm.unwindingCrossedNative` exists precisely to track this distinction, but the duplicate throw check doesn't use it!

### Why Async Generators "Worked"

The async generator changes at `pkg/builtins/async_generator_init.go:93-94`:

```go
vmInstance.ClearErrors()
vmInstance.ClearUnwindingState()
```

These calls **clear** `vm.unwinding` and `vm.currentException`, so the duplicate throw check doesn't trigger. But this is a **workaround** that:
1. Masks the underlying bug
2. Prevents proper exception propagation for sync generators
3. Only works because async generators convert exceptions to promises (terminal state)

### Secondary Issue: Frame Cleanup

The changes at `pkg/vm/vm.go:8834-8836` (startGenerator) and `9010-9012` (resumeGenerator):

```go
genObj.State = GeneratorCompleted
genObj.Done = true
genObj.Frame = nil  // ← Too aggressive for sync generators
```

**Problem**: Setting `Frame = nil` immediately on exception breaks:
1. Subsequent `.next()` calls on the generator (tests verify generator is properly closed)
2. Destructuring patterns that iterate over generator (test: `iter.next()` after exception)
3. Error: `TypeError: Cannot read property 'done' of null`

**Why async generators can do this**: They convert to rejected promises and never resume - terminal state.
**Why sync generators cannot**: They may be checked again (e.g., `iter.next()` should return `{done: true}` after exception).

---

## The Fix

### Philosophy

1. **Correct exception propagation**: Re-throws across native boundaries are legitimate, not duplicates
2. **Minimal state changes**: Don't modify generator state until exception handling completes
3. **Clear separation**: Async generators have promise semantics, sync generators have exception semantics
4. **No workarounds**: Fix the root cause, not symptoms

### Change 1: Fix Duplicate Throw Detection

**File**: `pkg/vm/exceptions.go:51-56`

**Before**:
```go
if vm.unwinding && vm.currentException.Is(value) {
    return  // Blocks ALL re-throws
}
```

**After**:
```go
// Avoid double-throwing the same value UNLESS we're crossing a native boundary
// When unwindingCrossedNative=true, this is a legitimate re-throw from native code
if vm.unwinding && vm.currentException.Is(value) && !vm.unwindingCrossedNative {
    if debugExceptions {
        fmt.Printf("[DEBUG exceptions.go] Duplicate throw of same exception during unwind; ignoring rethrow\n")
    }
    return
}
```

**Rationale**:
- When `unwindingCrossedNative=false`: Same exception being thrown multiple times in bytecode - true duplicate, block it
- When `unwindingCrossedNative=true`: Exception crossed native boundary and is being re-thrown - legitimate, allow it
- Preserves duplicate throw protection while enabling proper exception propagation

### Change 2: Remove Premature Frame Cleanup from Sync Path

**File**: `pkg/vm/vm.go`

**Remove** lines 8831-8847 (startGenerator) and 9007-9023 (resumeGenerator):

```go
if status == InterpretRuntimeError {
    // Exception occurred - clean up frames before returning
    // The generator is now in an error state and won't be resumed
    genObj.State = GeneratorCompleted  // ← REMOVE
    genObj.Done = true                 // ← REMOVE
    genObj.Frame = nil                 // ← REMOVE

    // Pop frames...
    vm.frameCount--
    // ... (keep frame cleanup code, but remove generator state modification)

    if vm.unwinding && vm.currentException != Null {
        return Undefined, exceptionError{exception: vm.currentException}
    }
    return Undefined, fmt.Errorf("runtime error during generator execution")
}
```

**Rationale**:
- The exception hasn't been handled yet - don't assume generator is completed
- Frame cleanup (popping call stack) is still necessary
- Generator state modification should happen AFTER exception handling completes
- Let the exception propagate normally, then mark generator as completed if no handler catches it

**Keep** frame pop/cleanup code (lines 8840-8847, 9016-9023):
```go
// Pop the generator frame and sentinel frame
vm.frameCount--
if vm.frameCount > 0 && regSize > 0 {
    vm.nextRegSlot -= regSize
}
// Pop the sentinel frame if present
if vm.frameCount > 0 && vm.frames[vm.frameCount-1].isSentinelFrame {
    vm.frameCount--
}
```

This is necessary to clean up the call stack before returning to native code.

### Change 3: Simplify Async Generator Exception Handling

**File**: `pkg/builtins/async_generator_init.go:87-100`

**Before**:
```go
if err != nil {
    thisGen.State = vm.GeneratorCompleted
    thisGen.Done = true
    thisGen.Frame = nil
    vmInstance.ClearErrors()           // ← Remove workaround
    vmInstance.ClearUnwindingState()   // ← Remove workaround
    if ee, ok := err.(vm.ExceptionError); ok {
        return vmInstance.NewRejectedPromise(ee.GetExceptionValue()), nil
    }
    return vmInstance.NewRejectedPromise(vm.NewString(err.Error())), nil
}
```

**After**:
```go
if err != nil {
    // Async generators convert exceptions to rejected promises
    // Mark as completed since async generators don't resume after exception
    thisGen.State = vm.GeneratorCompleted
    thisGen.Done = true
    thisGen.Frame = nil  // OK for async - won't resume

    // Extract exception value and wrap in rejected promise
    if ee, ok := err.(vm.ExceptionError); ok {
        return vmInstance.NewRejectedPromise(ee.GetExceptionValue()), nil
    }
    return vmInstance.NewRejectedPromise(vm.NewString(err.Error())), nil
}
```

**Rationale**:
- `ClearErrors()` and `ClearUnwindingState()` are no longer needed - the duplicate throw fix handles this correctly
- Async generators are terminal after exception (promise-based), so state cleanup is correct here
- Exception is caught and converted to promise - no further propagation needed

### Change 4: Remove ClearErrors/ClearUnwindingState Methods (Optional Cleanup)

**File**: `pkg/vm/vm_init.go:463-476`

These methods were only added as a workaround and are no longer needed. However, they could be kept for other potential use cases. Recommend keeping but adding a comment:

```go
// ClearErrors clears all recorded errors from the VM.
// WARNING: This is rarely needed and can mask bugs. Only use when errors
// have been properly handled and you need to reset error state.
func (vm *VM) ClearErrors() {
    vm.errors = nil
}

// ClearUnwindingState clears the exception unwinding state.
// WARNING: This should only be called when native code has FULLY handled
// an exception (e.g., caught and converted to a different value/promise).
// Clearing prematurely can prevent proper exception propagation.
func (vm *VM) ClearUnwindingState() {
    vm.unwinding = false
    vm.unwindingCrossedNative = false
    vm.currentException = Null
}
```

---

## Testing Strategy

### 1. Manual Test Cases

**Sync Generator Exception Propagation**:
```typescript
function* g() {
  throw new Error("TestError");
}

try {
  g().next();
} catch (e) {
  console.log("Caught:", e.message);  // Should execute
}
```

**Sync Generator State After Exception**:
```typescript
function* g() {
  throw new Error("TestError");
}

const iter = g();
try {
  iter.next();
} catch (e) {
  console.log("Caught");
}
const result = iter.next();
console.log("done:", result.done);  // Should be true, not throw
```

**Async Generator Exception Conversion**:
```typescript
async function test() {
  const iter = (async function* () {
    throw new Error("TestError");
  })();

  try {
    await iter.next();
  } catch (e) {
    console.log("Caught:", e.message);  // Should execute
  }
}
```

**Destructuring with Generator Exception**:
```typescript
function* iter() {
  throw new Error("TestError");
}

try {
  const [, x] = iter();  // Elision calls next()
} catch (e) {
  console.log("Caught:", e.message);  // Should execute
}
```

### 2. Test262 Regression Check

**Baseline Command**:
```bash
./paserati-test262 -path ./test262 -subpath "language" -timeout 0.2s -diff baseline.txt
```

**Expected Results**:
- +97 async generator tests should STAY passing
- +114 sync generator/destructuring tests should START passing
- Net change: +211 tests passing
- 0 regressions

**Specific Test Suites to Check**:
```bash
# Async generators (should pass)
./paserati-test262 -path ./test262 -subpath "language/expressions/async-generator" -timeout 0.2s

# Sync generator destructuring (should pass)
./paserati-test262 -path ./test262 -subpath "language/expressions/arrow-function/dstr" -timeout 0.2s

# For-of with generators (should pass)
./paserati-test262 -path ./test262 -subpath "language/statements/for-of/dstr" -timeout 0.2s
```

### 3. Smoke Tests

```bash
# Must always pass
go test ./tests -run TestScripts
```

---

## Implementation Checklist

- [ ] Fix duplicate throw detection in `pkg/vm/exceptions.go`
- [ ] Remove generator state modification from `startGenerator` in `pkg/vm/vm.go`
- [ ] Remove generator state modification from `resumeGenerator` in `pkg/vm/vm.go`
- [ ] Remove `ClearErrors()`/`ClearUnwindingState()` calls from `pkg/builtins/async_generator_init.go`
- [ ] Add warnings to `ClearErrors()`/`ClearUnwindingState()` methods in `pkg/vm/vm_init.go`
- [ ] Run manual test cases
- [ ] Run smoke tests (`go test ./tests -run TestScripts`)
- [ ] Run Test262 regression check
- [ ] Verify net +211 test improvement
- [ ] Document results

---

## Risk Assessment

**Low Risk Changes**:
- Duplicate throw detection fix: One-line change, well-understood, aligns with existing `unwindingCrossedNative` flag
- Removing workaround calls: Simplification, no functional change after main fix

**Medium Risk Changes**:
- Removing generator state modification: Needs verification that exceptions still mark generators as completed eventually

**Mitigation**:
- Comprehensive manual testing before Test262 suite
- Incremental implementation with testing at each step
- Rollback plan: Git stash before changes

---

## Performance Impact

**Zero Performance Impact**:
- Changes are in exception handling path (already slow/cold)
- Duplicate throw check: Added one boolean check (negligible)
- Removed unnecessary state clearing (minor improvement)
- No changes to hot path (normal execution, non-exceptional cases)

---

## Future Work

### 1. Generator State Management Consolidation

Currently, generator state transitions are scattered:
- Frame management in `vm.go` (startGenerator, resumeGenerator)
- Exception handling in `vm.go` (this fix)
- Async conversion in `async_generator_init.go`
- Sync operations in `generator_init.go`

**Recommendation**: Create a `pkg/vm/generator_state.go` with centralized state machine:
```go
type GeneratorStateTransition int

const (
    TransitionStart GeneratorStateTransition = iota
    TransitionYield
    TransitionReturn
    TransitionException
    TransitionComplete
)

func (g *GeneratorObject) Transition(t GeneratorStateTransition, value Value) error {
    // Centralized validation and state updates
}
```

### 2. Exception Unwinding Documentation

The exception unwinding mechanism is complex and brittle. Need:
- Comprehensive documentation of all unwinding scenarios
- State machine diagram for exception propagation
- Test suite specifically for exception edge cases
- Formalization of "native boundary" semantics

### 3. Generator Test Suite

Create `tests/scripts/generators/` with comprehensive generator tests:
- Exception propagation (sync and async)
- State transitions
- Edge cases (exception during yield, return, etc.)
- Integration with destructuring, for-of, spread, etc.

---

## References

- **Exception Unwinding Analysis**: `docs/EXCEPTION_UNWINDING_ANALYSIS.md`
- **Generator Implementation Plan**: `docs/generators-implementation-plan.md`
- **Test262 Opportunities**: `docs/test262_fix_opportunities.md`
- **Async Generator Issues**: `docs/async-gen-issues.md`

---

## Conclusion

This fix addresses a fundamental issue in exception handling that was exposed by generator exception handling improvements. The root cause is a flawed duplicate throw detection that doesn't account for legitimate re-throws across native boundaries.

The fix is surgical: one condition added to duplicate throw detection, removal of premature state modifications, and simplification of workarounds. This should result in +211 net Test262 improvements with zero regressions.

The underlying architecture (exception unwinding with native boundaries) is sound - this fix aligns the implementation with the existing design rather than introducing new mechanisms.

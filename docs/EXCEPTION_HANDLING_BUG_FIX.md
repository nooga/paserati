# Exception Handling Bug Fix: Nested Function Calls

## Problem Summary

The exception handling system had a critical bug where exceptions thrown from deeply nested function calls were not being caught by surrounding try/catch blocks. This affected any scenario where:

1. A try/catch block contained a function call
2. That function (or functions it called) threw an exception
3. The exception should have been caught by the outer try/catch

## Root Cause Analysis

The bug was in the compiler's exception table generation in `pkg/compiler/compile_exception.go`. The issue was with the calculation of the `TryEnd` instruction pointer:

### Buggy Code:
```go
// WRONG: tryEnd calculated before jump instruction
tryEnd := len(c.chunk.Code)
normalExit := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
```

### Problem:
- When a function call inside a try block returns, it returns to the instruction **after** the call
- If an exception was thrown during the nested call, the return address would be **after** the `TryEnd` boundary
- This meant the exception table lookup would fail to find the handler

### Secondary Issue:
After fixing the compiler issue, a VM execution synchronization problem emerged:
- When an exception was caught, `frame.ip` was correctly updated to the catch block
- However, the local `ip` variable in the VM execution loop wasn't synchronized
- This caused the VM to continue executing at the wrong instruction pointer

## Solution

### 1. Compiler Fix
Move the `tryEnd` calculation to **after** the jump instruction is emitted:

```go
// CORRECT: tryEnd calculated after jump instruction  
normalExit := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
tryEnd := len(c.chunk.Code)
```

This ensures that the return address from nested function calls falls within the protected exception table range.

### 2. VM Synchronization Fix
Add complete variable resynchronization in the OpThrow case when an exception is handled:

```go
// Exception was handled, synchronize all cached variables
frame = &vm.frames[vm.frameCount-1]
closure = frame.closure
function = closure.Fn
code = function.Chunk.Code
constants = function.Chunk.Constants
registers = frame.registers
ip = frame.ip  // Critical: sync the local ip variable
```

## Test Case

The fix was validated with `tests/scripts/try_catch_nested_calls.ts`:

```typescript
function outer() {
    return inner();
}

function inner() {
    return deep();
}

function deep() {
    throw "deep error";
}

try {
    outer();  // Should catch the exception from deep()
} catch (e) {
    return e; // ✅ Now correctly returns "deep error"
}
```

## Impact

- ✅ All existing exception handling tests continue to pass
- ✅ Nested function call exceptions now work correctly  
- ✅ No performance impact on normal execution
- ✅ No breaking changes to existing code

## Files Modified

1. `pkg/compiler/compile_exception.go` - Fixed tryEnd calculation
2. `pkg/vm/vm.go` - Added variable resynchronization in OpThrow case

## Validation

All exception handling tests pass:
- Basic try/catch: ✅
- Nested function calls: ✅ (Previously broken, now fixed)
- Error objects: ✅
- Uncaught exceptions: ✅
- Optional catch parameters: ✅

This fix completes Phase 1 of the exception handling implementation, providing a solid foundation for implementing finally blocks and advanced features.
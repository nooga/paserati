# Regression Analysis: 50 Failing Tests After Generator Prologue Fix

## Summary

After fixing generator prologue exception propagation by setting `isDirectCall = false`, we gained +211 tests but regressed 50 tests. All regressions are related to array destructuring with missing iterators or nested patterns.

## Root Cause

**Double Exception Throwing**: When exceptions occur during generator parameter destructuring with `isDirectCall = false`:

1. Exception unwinds through prologue frame in nested `vm.run()`
2. Exception unwinding pops the prologue frame (decrements `frameCount`)
3. `executeGeneratorPrologue` returns `InterpretRuntimeError` with `vm.unwinding = true`
4. OpCreateGenerator sees the error status and calls `vm.throwException()` AGAIN
5. This creates a duplicate throw, causing the exception to escape try-catch blocks

## Evidence

### Test Pattern 1: Missing Iterator
```javascript
delete Array.prototype[Symbol.iterator];
var f = function*([x, y, z]) {};
assert.throws(TypeError, function() { f([1, 2, 3]); });
```

**Expected**: TypeError caught by assert.throws
**Actual**: Exception escapes and becomes uncaught (printed 3 times)

### Test Pattern 2: Nested Destructuring
```javascript
var f = function*([[] = function() { initCount += 1; return iter; }()]) { ... };
```

**Expected**: Initializer evaluated during destructuring
**Actual**: "undefined is not a function" error escapes

### Debug Output Analysis
```
PS4001 [ERROR]: Uncaught exception: TypeError: undefined is not a function
    at gen (<gen>:11:1)
    at <script> (<script>:10:1)
[repeated 3 times]
```

The exception is printed multiple times because it's being re-thrown repeatedly.

## Detailed Flow Comparison

### Before Fix (isDirectCall = true)
```
1. OpCreateGenerator calls executeGeneratorPrologue
2. Prologue frame has isDirectCall = true
3. Exception occurs during destructuring
4. Exception unwinding hits isDirectCall frame, STOPS
5. executeGeneratorPrologue cleans up frame, returns InterpretRuntimeError
6. OpCreateGenerator re-throws in outer context
7. Try-catch in outer vm.run() catches it ✓
```

### After Fix (isDirectCall = false) - BROKEN
```
1. OpCreateGenerator calls executeGeneratorPrologue
2. Prologue frame has isDirectCall = false
3. Exception occurs during destructuring
4. Exception unwinding CONTINUES through prologue frame
5. Exception unwinding pops prologue frame
6. executeGeneratorPrologue returns InterpretRuntimeError (vm.unwinding = true)
7. OpCreateGenerator sees error and calls vm.throwException() AGAIN
8. Exception is ALREADY unwinding, creating duplicate throw
9. Exception escapes try-catch ✗
```

## Solution

**Don't re-throw if already unwinding**. In OpCreateGenerator, check `vm.unwinding` status:

```go
if prologueStatus != InterpretOK {
    // Check if exception is already propagating
    if vm.unwinding {
        // Exception already unwinding through outer vm.run()
        // Just continue - don't re-throw
        continue
    }

    // Otherwise, re-throw in outer context
    frame.ip = callerIP
    if vm.currentException.Type() != TypeUndefined {
        vm.throwException(vm.currentException)
    } else if vm.lastThrownException.Type() != TypeUndefined {
        vm.throwException(vm.lastThrownException)
    } else {
        vm.throwException(NewString("Generator initialization failed"))
    }
    continue
}
```

## Affected Test Categories

1. **Generator destructuring with missing iterator** (19 tests)
   - Pattern: `ary-init-iter-get-err-array-prototype.js`
   - Tests delete `Array.prototype[Symbol.iterator]`

2. **Nested array patterns with initializers** (13 tests)
   - Pattern: `ary-ptrn-elem-ary-empty-init.js`
   - Tests nested destructuring like `[[] = expr]`

3. **Non-generator destructuring** (18 tests)
   - Arrow functions, regular methods, for-of loops
   - Same patterns but in different contexts

## Why These Tests Passed Before

With `isDirectCall = true`, exceptions stopped at the prologue frame boundary. The frame was cleaned up explicitly by `executeGeneratorPrologue`, and the re-throw happened cleanly in the outer context without vm.unwinding being true.

## Confidence Level

**HIGH** - The root cause is clear from debug output showing:
- Multiple exception prints (duplicate throws)
- Exceptions escaping try-catch blocks
- "undefined is not a function" errors for missing iterators

The fix is surgical: Just check `vm.unwinding` before re-throwing.

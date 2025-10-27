# Register Allocation Bugs in Function Calls

## Overview

This document details a critical set of register allocation bugs discovered in the Paserati compiler that affect function calls, particularly in generator methods with parameter defaults containing nested functions and `eval()` calls.

## Symptoms

1. **Runtime Error**: `Internal Error: Invalid target register 8 for return value`

   - Occurs when nested functions try to return values to registers that don't exist in the caller's frame
   - The caller (generator) has `RegisterSize=6` (R0-R5) but bytecode references R8

2. **Runtime Error**: `Implicit return missing in function?`

   - Occurs when VM's instruction pointer (IP) falls out of a function's bytecode
   - Indicates incorrect IP management or bytecode length calculation

3. **Test262 Failures**: Multiple failures in `language/expressions/object/scope-gen-meth-param-*` tests

## Root Causes

### Bug #1: Argument Registers Not Allocated (PARTIALLY FIXED)

**Location**: `pkg/compiler/compiler.go` - `compileArgumentsWithOptionalHandling()`

**Problem**:
When compiling function calls, the compiler calculated argument register positions arithmetically as `funcReg+1, funcReg+2, ...` without actually allocating them through the register allocator. This violated the fundamental rule: **all registers used must come from the allocator**.

```go
// BEFORE (WRONG):
for i := 0; i < finalArgCount; i++ {
    targetReg := firstTargetReg + Register(i)  // CALCULATED, not allocated!
    argRegs = append(argRegs, targetReg)
}
```

**Impact**:

- Argument registers (R5, R6, R7, etc.) were used without being tracked in `maxReg`
- Function's `RegisterSize` was calculated incorrectly (too small)
- At runtime, these registers didn't exist in the function's frame

**Fix Applied**:
Modified `compileArgumentsWithOptionalHandling()` and `compileArgumentsWithOptionalHandlingForNew()` to allocate registers:

```go
// AFTER (CORRECT):
if finalArgCount > 0 {
    // Ensure all argument registers are allocated
    if firstTargetReg == c.regAlloc.nextReg {
        c.regAlloc.AllocContiguous(finalArgCount)
    } else {
        lastArgReg := firstTargetReg + Register(finalArgCount) - 1
        if lastArgReg >= c.regAlloc.nextReg {
            needed := int(lastArgReg - c.regAlloc.nextReg + 1)
            c.regAlloc.AllocContiguous(needed)
        }
    }
}
```

**Status**: ✅ Fixed in `pkg/compiler/compiler.go` (lines 1440-1474) and `pkg/compiler/compile_expression.go` (lines 2106-2124)

### Bug #2: VM Calling Convention vs. Register Allocation Mismatch

**Location**: Multiple - affects how function calls are compiled and executed

**Problem**:
The VM's calling convention expects arguments at contiguous registers starting at `funcReg+1`:

```go
// pkg/vm/vm.go - how VM reads arguments
args := make([]Value, argCount)
for i := 0; i < argCount; i++ {
    args[i] = registers[funcReg+1+byte(i)]  // ASSUMES funcReg+1, funcReg+2, ...
}
```

However, the compiler's register allocator might allocate registers non-contiguously when there are free registers in the pool. This creates a fundamental mismatch:

- Compiler allocates: R0, R4, R7 (non-contiguous due to reuse)
- VM expects: R0, R1, R2 (contiguous)

**Impact**:

- Arguments end up in wrong positions
- Function calls receive incorrect values
- Undefined behavior when funcReg and arguments aren't contiguous

**Status**: ⚠️ Partially addressed by Bug #1 fix, but the fundamental architectural issue remains

### Bug #3: Destination Register Allocation (UNSOLVED)

**Location**: Unknown - requires further investigation

**Problem**:
Even after fixing argument register allocation, function calls still reference invalid destination registers. Example from debug output:

```
Generator has RegisterSize=6 (maxReg=5)  // Only R0-R5 available
// But somewhere an OpCall with dest=R8 is executed
```

The OpCall instruction's `destReg` parameter (where the return value goes) is being set to R8, but:

1. No OpCall with dest=R8 is emitted at compile time for the generator
2. The generator's bytecode is only 75 bytes (ends at offset 74)
3. Runtime error shows IP=237 in the generator, which is past the bytecode end

**Observed Evidence**:

```
[EMIT_CALL] dest=R2 funcReg=R4 argCount=1 maxReg=6 line=190 func=<anonymous>
```

This is the ONLY OpCall emitted for the generator during compilation, and it's valid (dest=R2 < maxReg=6).

**Theories**:

1. **Generator Runtime Behavior**: Generator functions have special runtime behavior (yield/resume/next). The bytecode might be getting modified or extended at runtime, or there's additional generated code we're not seeing.

2. **Nested Function Context**: The nested functions created in parameter defaults (`probe1 = function() { return x; }`) might be capturing incorrect frame references or register numbers from their closure.

3. **Eval Impact**: The `eval('var x = "inside";')` in parameter defaults creates dynamic scope, which might be interfering with register allocation or closure capture.

4. **IP Management Bug**: The IP=237 being beyond the bytecode (length 75) suggests the VM's instruction pointer management is broken, possibly in generator state transitions.

**Status**: ❌ NOT FIXED - Root cause unknown

## Test Cases

### Minimal Reproduction

```typescript
// tests/scripts/gen_method_eval_param_crash.ts
var x = "outside";
var probe1, probe2;
({
  *m(
    _ = (eval('var x = "inside";'),
    (probe1 = function () {
      return x;
    })),
    __ = (probe2 = function () {
      return x;
    })
  ) {},
})
  .m()
  .next();
console.log("probe1:", probe1());
console.log("probe2:", probe2());
```

**Expected**: Print "inside" twice
**Actual**: Runtime error "Implicit return missing in function?"

### Test262 Affected Tests

```
test262/test/language/expressions/object/scope-gen-meth-param-elem-var-close.js
test262/test/language/expressions/object/scope-gen-meth-param-elem-var-open.js
// ... and others in the same suite
```

## Diagnostic Output

### Compile-Time Analysis

With debug flags enabled (`debugPrint = true`):

```
[ARG_ALLOC] Already allocated: firstTarget=R5, lastArg=R5, nextReg=R6
[ARG_ALLOC] maxReg: before=5, after=5 (delta=0)
[REGSIZE] Generator '<anonymous>' has RegisterSize=6 (maxReg=5)
[EMIT_CALL] dest=R2 funcReg=R4 argCount=1 maxReg=6 line=190 func=<anonymous>
```

**Analysis**:

- Generator correctly compiles with maxReg=5 (RegisterSize=6)
- Only one OpCall emitted for the generator
- That OpCall uses valid registers (dest=R2 < maxReg=6)

### Generator Bytecode Disassembly

```
== <anonymous> ==
0000      OpLoadUndefined  R2
0002      OpStrictEqual    R3, R0, R2
0006      OpJumpIfFalse    R3, 29 (to 0039)
0010      OpGetGlobal      R4, GlobalIdx 38        // Get 'eval'
0014      OpLoadConst      R5, 0 ('var x = "inside";')
0018      OpCall           R2, R4, Args 1          // Call eval
0022      OpLoadConst      R4, 1 ('<function>')    // Load probe1 function
0026      OpMove           R5, R4
0029      OpSetGlobal      GlobalIdx 58, R5        // Set global 'probe1'
0033      OpMove           R3, R5
0036      OpMove           R0, R3                  // Move to parameter R0
0039      OpLoadUndefined  R3
0041      OpStrictEqual    R5, R1, R3
0045      OpJumpIfFalse    R5, 14 (to 0063)
0049      OpLoadConst      R3, 2 ('<function>')    // Load probe2 function
0053      OpMove           R5, R3
0056      OpSetGlobal      GlobalIdx 59, R5        // Set global 'probe2'
0060      OpMove           R1, R5
0063      OpLoadConst      R2, 3 ('<function>')    // Load probeBody function
0067      OpMove           R3, R2
0070      OpSetGlobal      GlobalIdx 60, R3        // Set global 'probeBody'
0074      OpReturnUndefined                        // END at offset 74
```

**Key Observations**:

1. Bytecode ends at offset 74
2. Only uses registers R0-R5 (valid for RegisterSize=6)
3. The OpCall at offset 0018 uses dest=R2, funcReg=R4 (both valid)
4. No instruction references R8 or higher registers

**Discrepancy**: Runtime error mentions IP=237, which is 163 bytes beyond the end! This is impossible unless:

- The bytecode is being modified at runtime
- The IP tracking is broken
- The error message shows the wrong IP

## Architecture Issues

### Register Allocator Design

The current register allocator uses a free-list approach:

```go
type RegisterAllocator struct {
    nextReg  Register           // Next register to allocate
    maxReg   Register           // Highest register ever allocated
    freeRegs []Register         // Stack of freed registers for reuse
}
```

**Problems**:

1. `maxReg` never decreases, even when registers are freed
2. Reused registers from `freeRegs` might not be contiguous
3. VM calling convention assumes contiguous register layout

### VM Calling Convention

```go
// VM expects arguments at specific offsets from funcReg
args[i] = registers[funcReg+1+byte(i)]
```

This is a **position-based** convention, not a **descriptor-based** one. The OpCall instruction only specifies:

- `destReg`: where to put return value
- `funcReg`: register containing the function
- `argCount`: number of arguments

It does NOT specify which registers contain the arguments - it assumes `funcReg+1, funcReg+2, ...`

**Alternative Designs**:

1. **Descriptor-based**: OpCall stores array of argument register numbers
2. **Always contiguous**: Allocator guarantees function + args are contiguous
3. **Argument marshalling**: Compiler emits moves to pack arguments contiguously before call

## Next Steps

### Immediate Fixes Needed

1. **✅ DONE**: Fix argument register allocation in `compileArgumentsWithOptionalHandling`
2. **🔄 IN PROGRESS**: Investigate why dest=R8 appears at runtime but not at compile time
3. **❌ TODO**: Fix "Implicit return missing" error (IP management)
4. **❌ TODO**: Understand generator bytecode execution model

### Investigation Tasks

1. **Trace Runtime Execution**: Enable VM debug tracing to see actual OpCall instructions executed
2. **Generator State Machine**: Review how generator functions are executed (yield/resume)
3. **Closure Capture**: Verify nested functions capture correct frame references
4. **Eval Scope**: Understand how eval affects scope and register allocation

### Long-Term Improvements

1. **Calling Convention Review**: Consider switching to descriptor-based calling convention
2. **Register Allocator Rewrite**: Implement allocator that guarantees contiguous blocks
3. **Validation Pass**: Add bytecode validation pass to catch invalid register references
4. **Generator Tests**: Expand test suite to cover generator parameter defaults

## References

### Code Locations

- **Register Allocator**: `pkg/compiler/regalloc.go`
- **Function Call Compilation**: `pkg/compiler/compile_expression.go` (`compileCallExpression`)
- **Argument Handling**: `pkg/compiler/compiler.go` (`compileArgumentsWithOptionalHandling`)
- **Function Literal Compilation**: `pkg/compiler/compile_literal.go` (`compileFunctionLiteral`)
- **VM Call Execution**: `pkg/vm/vm.go` (OpCall handler, line ~1696)
- **VM Call Preparation**: `pkg/vm/call.go` (`prepareCall`)

### Related Documents

- `docs/objects.txt` - Object implementation patterns
- `docs/generators-implementation-plan.md` - Generator implementation
- `README.md` - Project ethos and architecture philosophy

## Debug Flags

To reproduce and investigate:

```go
// pkg/compiler/compiler.go
const debugPrint = true         // Enable debug printf statements

// pkg/compiler/regalloc.go
const debugRegAlloc = true      // Trace register allocation

// pkg/vm/vm.go
const debugVM = true            // VM execution tracing
const debugCalls = true         // Function call tracing
```

## Conclusion

We have identified and partially fixed a critical register allocation bug where argument registers were calculated instead of allocated. However, a deeper issue remains where destination registers and/or instruction pointers become invalid at runtime, particularly in generator functions with complex parameter defaults.

The root cause appears to be a combination of:

1. Mismatch between allocator behavior and VM calling convention
2. Generator runtime behavior not properly reflected in bytecode
3. Possible IP management bugs in generator state transitions

Further investigation is needed to fully resolve these issues.

---

_Document created: 2025-01-27_  
_Status: Active Investigation_  
_Priority: High - Blocks Test262 compliance_

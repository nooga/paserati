# Tail Call Optimization (TCO) Design Document

**Date:** 2025-10-13
**Author:** Claude Code
**Status:** Design Phase

## 1. Executive Summary

This document outlines the design and implementation plan for proper Tail Call Optimization (TCO) in Paserati, as required by ECMAScript 2015 (ES6). TCO allows recursive functions with 100,000+ iterations to execute without stack overflow by reusing call frames instead of creating new ones.

**Goal:** Full ECMAScript compliance with tail call semantics, enabling Test262 TCO tests to pass.

**Scope:** Implement frame reuse for tail-positioned calls, maintaining compatibility with exception handling, upvalues, generators, and async functions.

**Current Baseline (2025-10-13):**
- **Language Test Suite Pass Rate:** 73.9% (17,456/23,634 tests passing)
- **Execution Time:** 1m15.552s for full language suite
- **Mode:** Currently running in unspecified mode (likely closer to strict mode semantics)
- **Note:** Paserati does not currently distinguish between strict/non-strict modes

**Performance Target:** Maintain or improve current 73.9% pass rate after TCO implementation.

---

## 2. Background

### 2.1 What is Tail Call Optimization?

A **tail call** is a function call that appears in tail position - meaning it's the last operation before returning from a function. In proper tail calls, the calling frame is no longer needed after the call, so it can be reused.

**Example:**
```javascript
'use strict';
function f(n) {
  if (n === 0) return 1;
  return f(n - 1);  // Tail call - last operation before return
}

f(100000);  // Should not stack overflow with TCO
```

**Non-tail call example:**
```javascript
function g(n) {
  if (n === 0) return 1;
  return 1 + g(n - 1);  // NOT a tail call - needs to add 1 after call returns
}
```

### 2.2 ECMAScript Requirements

Per **ES2015 Section 14.6.1 Static Semantics: HasCallInTailPosition**:
- Specified for **strict mode** in the spec
- Call must be in syntactic tail position
- After the call returns, the calling function immediately returns that value with no additional operations
- Must handle: return statements, if/else branches, switch cases, labeled statements, try/catch/finally, etc.

**Note on Strict Mode:** Paserati currently does not distinguish between strict and non-strict modes. The runtime operates in an unspecified mode that is semantically closer to strict mode. For TCO implementation, we will enable tail call optimization globally rather than gating it on strict mode detection.

Test262 validates TCO by requiring **100,000 consecutive tail calls** without stack overflow.

---

## 3. Current Architecture Analysis

### 3.1 Call Frame Structure

From `pkg/vm/vm.go:65-93`:
```go
type CallFrame struct {
    closure           *ClosureObject  // Function being executed
    ip                int              // Instruction pointer
    registers         []Value          // Window into registerStack
    targetRegister    byte             // Where result goes in CALLER
    thisValue         Value            // Method call 'this' binding
    isConstructorCall bool             // 'new' invocation
    newTargetValue    Value            // For new.target
    isDirectCall      bool             // Function.prototype.call
    isSentinelFrame   bool             // Sentinel for vm.run() return
    argCount          int              // Actual argument count
    args              []Value          // Argument values (copy)
    isNativeFrame     bool             // Native function frame
    nativeReturnCh    chan Value       // Native function channels
    nativeYieldCh     chan *BytecodeCall
    nativeCompleteCh  chan Value
    generatorObj      *GeneratorObject // Generator state
    promiseObj        *PromiseObject   // Promise state
}
```

**Key observations:**
- Frames are preallocated: `frames [MaxFrames]CallFrame` (MaxFrames = 10,000)
- Registers use window-based allocation: `registerStack [RegFileSize * MaxFrames]Value`
- Each frame has a window via `registers []Value` slice
- Frame reuse would need to preserve these invariants

### 3.2 Call Execution Flow

#### Normal Call Path (from `pkg/vm/call.go:100-284`):

1. **prepareCall()** - Sets up new call frame
   - Type checks (Closure, Function, NativeFunction, etc.)
   - Arity validation
   - Frame limit check (`frameCount == MaxFrames`)
   - Register space check
   - **Creates new frame** at `vm.frames[vm.frameCount]`
   - Copies arguments to new frame's register window
   - Increments `vm.frameCount++` and `vm.nextRegSlot += requiredRegs`
   - Returns `true` to switch frames

2. **OpCall execution** (from `pkg/vm/vm.go:1299-1508`):
   - Decodes: destReg, funcReg, argCount
   - Reads function and arguments from registers
   - Calls `prepareCall()`
   - If frame switch: updates local variables (frame, closure, function, code, constants, registers, ip)
   - If native: stores result directly, continues

3. **OpReturn execution** (from `pkg/vm/vm.go:1510-1683`):
   - Reads result from register
   - Handles finally blocks (checks `findAllExceptionHandlers()`)
   - Closes upvalues for returning frame
   - Handles generator completion
   - **Pops frame**: `vm.frameCount--`, `vm.nextRegSlot -= returningFrameRegSize`
   - Handles constructor return semantics
   - Places result in caller's targetRegister
   - Restores caller frame variables

### 3.3 Register Allocation

From `pkg/vm/vm.go:103-124`:
- Shared register stack: `registerStack [RegFileSize * MaxFrames]Value`
- `nextRegSlot` tracks next available register slot
- Each frame gets a window: `vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+requiredRegs]`
- On call: `vm.nextRegSlot += requiredRegs`
- On return: `vm.nextRegSlot -= returningFrameRegSize`

**TCO implication:** Frame reuse must preserve register window integrity.

### 3.4 Exception Handling

From `pkg/vm/exceptions.go:20-40`:
```go
func (vm *VM) findAllExceptionHandlers(pc int) []*ExceptionHandler {
    frame := &vm.frames[vm.frameCount-1]
    chunk := frame.closure.Fn.Chunk
    for i := range chunk.ExceptionTable {
        handler := &chunk.ExceptionTable[i]
        if pc >= handler.TryStart && pc < handler.TryEnd {
            handlers = append(handlers, handler)
        }
    }
    return handlers
}
```

**Exception handler structure** (from `pkg/vm/bytecode.go:457-465`):
```go
type ExceptionHandler struct {
    TryStart   int   // PC where try block starts
    TryEnd     int   // PC where try block ends
    HandlerPC  int   // Jump target for exceptions
    CatchReg   int   // Register for exception (-1 if finally only)
    IsCatch    bool  // true for catch blocks
    IsFinally  bool  // true for finally blocks
    FinallyReg int   // Register for pending action/value
}
```

**TCO implication:** Exception handlers are per-chunk, indexed by PC. Frame reuse preserves chunk association, so exception handling remains valid.

### 3.5 Upvalue Management

From `pkg/vm/vm.go:114-115`:
```go
openUpvalues []*Upvalue  // Upvalues pointing to registerStack
```

When returning (`pkg/vm/vm.go:1566`):
```go
vm.closeUpvalues(frame.registers)
```

**TCO implication:** Must close upvalues before frame reuse to prevent dangling references.

---

## 4. TCO Execution Model

### 4.1 Tail Position Detection (Compiler)

The compiler must identify tail-positioned calls according to ES2015 Section 14.6.1. A call is in tail position if:

1. It's in a **return statement**: `return f(x);`
2. It's the consequent/alternate of a tail-positioned conditional: `if (c) return f(x); else return g(y);`
3. It's in a tail-positioned case clause: `switch(x) { case 0: return f(y); }`
4. It's in a tail-positioned labeled statement: `label: return f(x);`
5. It's the body expression of an arrow function in tail position

**Not tail calls:**
- Calls followed by operations: `return 1 + f(x);`
- Calls with pending work: `f(x); cleanup(); return;`
- Calls in try blocks (exception handling needs caller frame)
- Constructor calls (`new F()`)
- Calls in finally blocks (pending action needs preservation)

**Compiler implementation:**
- Add `inTailPosition` bool field to Compiler
- Track tail position context during statement compilation
- Set `inTailPosition = true` when compiling return statement's argument
- Set `inTailPosition = false` for expressions with pending operations
- Emit OpTailCall instead of OpCall when `inTailPosition == true`

### 4.2 Frame Reuse Strategy

#### Option A: In-Place Frame Reuse (Recommended)

**Approach:** Reuse the current frame for the tail call instead of creating a new one.

**Steps:**
1. Detect tail call (OpTailCall instruction)
2. Evaluate new function and arguments into temporary registers
3. Close upvalues for current frame
4. **Reuse current frame**:
   - Keep same frame index in frames array
   - Update `closure` to new closure
   - Reset `ip = 0`
   - Copy new arguments to frame's register window
   - Keep same `targetRegister` (result goes to same caller register)
   - Clear generator/promise/native state
5. Continue execution from new function's start

**Advantages:**
- Simple implementation
- No frameCount manipulation
- Preserves exception handler context (still same frame stack depth)
- Register window stays at same position (simpler register management)

**Disadvantages:**
- Requires temporary registers for new function/args (could overlap with current registers)
- Needs careful ordering to avoid stomping on values

#### Option B: Pop-Then-Push Frame Reuse

**Approach:** Pop current frame, then create new frame in same position.

**Steps:**
1. Detect tail call
2. Save caller's targetRegister
3. Evaluate new function and arguments
4. Close upvalues for current frame
5. Pop frame: `vm.frameCount--`, `vm.nextRegSlot -= currentFrameRegSize`
6. Create new frame (prepareCall-like logic)
7. Use saved targetRegister for new frame

**Advantages:**
- Clearer semantics (explicit pop/push)
- Reuses existing prepareCall logic

**Disadvantages:**
- More complex frameCount manipulation
- Needs to preserve caller's targetRegister across pop
- More potential for bugs

**Decision: Use Option A (In-Place Frame Reuse)** for simplicity and safety.

### 4.3 Register Management for Tail Calls

**Challenge:** New function/args must be evaluated before overwriting current frame's registers.

**Solution: Temporary Register Staging**

1. Allocate temporary registers at end of current frame's window (or use reserved staging area)
2. Evaluate new function → tempFuncReg
3. Evaluate new arguments → tempArg0, tempArg1, ...
4. Close upvalues for current frame
5. Copy temps to frame's register window:
   - `frame.registers[0] = function` (implicit, stored in closure)
   - `frame.registers[0..N] = args`
6. Update frame metadata
7. Free temporary registers (or reuse as part of frame window)

**Alternative: Reserve Tail Call Staging Registers**

Reserve a fixed number of registers per frame specifically for tail call staging (e.g., last 16 registers). This ensures temps never conflict with current execution.

**Decision:** Use staging area approach with validation that requiredRegs ≤ currentFrameRegSize. If new function requires more registers, fall back to regular call (stack will grow).

### 4.4 Exception Handling Interaction

**Question:** What happens if a tail call is in a try block?

**Answer:** Tail calls in try blocks are **NOT** tail calls per ES2015 spec.

From ES2015 14.6.1 Note:
> A potential tail position call that is immediately followed by return GetValue of the call result is also a possible tail position call. **Function calls cannot return reference values, so** such a GetValue operation will always return the same value as the actual function call result.

However, **try blocks disable TCO** because:
1. The caller frame is needed for exception unwinding
2. Exception handlers reference the caller's PC
3. Finally blocks need the caller frame to restore after finally execution

**Compiler implementation:** Set `inTailPosition = false` inside try blocks.

### 4.5 Upvalue Handling

**Critical:** Must close upvalues BEFORE reusing frame to prevent dangling references.

**Sequence:**
```go
// Before reusing frame for tail call
vm.closeUpvalues(frame.registers)

// Now safe to overwrite registers and reuse frame
```

Existing `closeUpvalues()` logic (from `pkg/vm/vm.go`) already handles this correctly.

### 4.6 Generator and Async Function Interaction

**Generators:** Tail calls inside generator functions are **NOT** tail calls because:
- Generator frames must be preserved across yields
- Generator state restoration needs the frame intact

**Async Functions:** Tail calls inside async functions are **NOT** tail calls because:
- Async functions wrap in Promise
- Frame needed for microtask scheduling

**Compiler implementation:** Set `inTailPosition = false` inside generator/async function bodies.

---

## 5. Bytecode Design

### 5.1 New Opcode: OpTailCall

**Format:** `OpTailCall destReg funcReg argCount`

Same encoding as OpCall but signals frame reuse.

**From `pkg/vm/bytecode.go:58`:**
```go
OpCall   OpCode = 19  // Rx FuncReg ArgCount: Call function
OpReturn OpCode = 20  // Rx: Return value
```

**New:**
```go
OpTailCall OpCode = 109  // Rx FuncReg ArgCount: Tail call (frame reuse)
```

### 5.2 OpTailCall Execution

**From `pkg/vm/vm.go`, add new case in run() loop:**

```go
case OpTailCall:
    destReg := code[ip]
    ip++
    funcReg := code[ip]
    ip++
    argCount := code[ip]
    ip++

    // 1. Read function and arguments
    calleeVal := registers[funcReg]
    args := make([]Value, argCount)
    for i := 0; i < int(argCount); i++ {
        args[i] = registers[funcReg+1+byte(i)]
    }

    // 2. Type check - only support closures for now
    if calleeVal.Type() != TypeClosure {
        // Fall back to regular call for native functions, etc.
        // Native functions execute immediately, can't be tail optimized
        ip -= 3  // Rewind to start of instruction
        // Replace OpTailCall with OpCall and re-execute
        code[ip] = byte(OpCall)
        continue
    }

    calleeClosure := calleeVal.AsClosure()
    calleeFunc := calleeClosure.Fn

    // 3. Validate register requirements
    requiredRegs := calleeFunc.RegisterSize
    if requiredRegs > len(frame.registers) {
        // New function needs more registers than current frame has
        // Fall back to regular call (will grow stack)
        ip -= 3
        code[ip] = byte(OpCall)
        continue
    }

    // 4. Close upvalues for current frame BEFORE overwriting
    vm.closeUpvalues(frame.registers)

    // 5. Reuse current frame
    frame.closure = calleeClosure
    frame.ip = 0
    // Keep targetRegister unchanged (return to same caller location)
    frame.thisValue = Undefined  // Regular call has undefined 'this'
    frame.isConstructorCall = false
    frame.isDirectCall = false
    frame.isSentinelFrame = false
    frame.generatorObj = nil
    frame.promiseObj = nil
    frame.argCount = int(argCount)
    frame.args = args  // Already copied above

    // 6. Copy arguments to frame registers
    for i := 0; i < int(argCount) && i < len(frame.registers); i++ {
        frame.registers[i] = args[i]
    }
    // Pad with undefined
    for i := int(argCount); i < calleeFunc.Arity && i < len(frame.registers); i++ {
        frame.registers[i] = Undefined
    }

    // Handle rest parameters if variadic
    if calleeFunc.Variadic {
        extraArgCount := int(argCount) - calleeFunc.Arity
        var restArray Value
        if extraArgCount <= 0 {
            restArray = vm.emptyRestArray
        } else {
            restArray = NewArray()
            restArrayObj := restArray.AsArray()
            for i := 0; i < extraArgCount; i++ {
                argIndex := calleeFunc.Arity + i
                if argIndex < len(args) {
                    restArrayObj.Append(args[argIndex])
                }
            }
        }
        if calleeFunc.Arity < len(frame.registers) {
            frame.registers[calleeFunc.Arity] = restArray
        }
    }

    // Handle named function expression binding
    if calleeFunc.NameBindingRegister >= 0 && calleeFunc.NameBindingRegister < len(frame.registers) {
        frame.registers[calleeFunc.NameBindingRegister] = calleeVal
    }

    // 7. Switch to new function's code
    closure = calleeClosure
    function = calleeFunc
    code = function.Chunk.Code
    constants = function.Chunk.Constants
    registers = frame.registers
    ip = 0

    continue
```

### 5.3 OpTailCallMethod

For method tail calls (preserving 'this' binding):

**Format:** `OpTailCallMethod destReg funcReg thisReg argCount`

```go
OpTailCallMethod OpCode = 110  // Rx FuncReg ThisReg ArgCount: Tail method call
```

**Execution:** Same as OpTailCall but sets `frame.thisValue = thisVal` from thisReg.

---

## 6. Compiler Implementation Plan

### 6.1 Phase 1: Tail Position Tracking

**File:** `pkg/compiler/compiler.go`

**Add to Compiler struct:**
```go
type Compiler struct {
    // ... existing fields ...

    // --- Tail Call Optimization ---
    inTailPosition    bool  // True when compiling tail-positioned expression
    inGeneratorBody   bool  // True when compiling generator function body
    inAsyncBody       bool  // True when compiling async function body
    inTryBlock        bool  // True when compiling try block body
}
```

**Modifications:**

1. **compileReturnStatement()** - Set tail position
   ```go
   func (c *Compiler) compileReturnStatement(node *parser.ReturnStatement, hint Register) (Register, errors.PaseratiError) {
       if node.ReturnValue != nil {
           // Enable tail position for return value expression
           oldTailPos := c.inTailPosition
           c.inTailPosition = !c.inFinallyBlock && !c.inTryBlock && !c.inGeneratorBody && !c.inAsyncBody

           returnReg := c.regAlloc.Alloc()
           defer c.regAlloc.Free(returnReg)
           _, err := c.compileNode(node.ReturnValue, returnReg)

           c.inTailPosition = oldTailPos  // Restore
           // ... rest of return compilation ...
       }
   }
   ```

2. **compileTryStatement()** - Disable tail position
   ```go
   func (c *Compiler) compileTryStatement(node *parser.TryStatement, hint Register) (Register, errors.PaseratiError) {
       oldInTry := c.inTryBlock
       c.inTryBlock = true
       // ... compile try block ...
       c.inTryBlock = oldInTry
   }
   ```

3. **compileFunctionLiteral()** - Disable tail position for generators/async
   ```go
   func (c *Compiler) compileFunctionLiteral(node *parser.FunctionLiteral, nameHint string) (uint16, []*Symbol, errors.PaseratiError) {
       // ... create function compiler ...
       funcCompiler.inGeneratorBody = node.IsGenerator
       funcCompiler.inAsyncBody = node.IsAsync
       // ...
   }
   ```

4. **compileIfStatement()** - Propagate tail position to branches
   ```go
   func (c *Compiler) compileIfStatement(node *parser.IfStatement, hint Register) (Register, errors.PaseratiError) {
       // ... compile condition ...

       // Both branches inherit tail position
       _, err := c.compileNode(node.Consequence, hint)
       if node.Alternative != nil {
           _, err := c.compileNode(node.Alternative, hint)
       }
   }
   ```

5. **compileSwitchStatement()** - Propagate tail position to cases
   ```go
   func (c *Compiler) compileSwitchStatement(node *parser.SwitchStatement, hint Register) (Register, errors.PaseratiError) {
       // ... compile discriminant ...

       for _, caseClause := range node.Cases {
           // Last statement in case body inherits tail position
           // if there's no fallthrough
       }
   }
   ```

### 6.2 Phase 2: Emit OpTailCall

**File:** `pkg/compiler/compile_expression.go`

**Modify compileCallExpression():**

```go
func (c *Compiler) compileCallExpression(node *parser.CallExpression, hint Register) (Register, errors.PaseratiError) {
    // ... existing setup ...

    // After compiling function and arguments, check if this is a tail call
    isTailCall := c.inTailPosition

    // Emit appropriate call instruction
    if isTailCall {
        // Tail call - use OpTailCall for frame reuse
        c.emitTailCall(hint, funcReg, byte(actualArgCount), node.Token.Line)
    } else {
        // Normal call
        c.emitCall(hint, funcReg, byte(actualArgCount), node.Token.Line)
    }

    return hint, nil
}
```

**Add emit helper:**
```go
func (c *Compiler) emitTailCall(destReg Register, funcReg Register, argCount byte, line int) {
    c.emitOpCode(vm.OpTailCall, line)
    c.emitByte(byte(destReg))
    c.emitByte(byte(funcReg))
    c.emitByte(argCount)
}

func (c *Compiler) emitTailCallMethod(destReg Register, funcReg Register, thisReg Register, argCount byte, line int) {
    c.emitOpCode(vm.OpTailCallMethod, line)
    c.emitByte(byte(destReg))
    c.emitByte(byte(funcReg))
    c.emitByte(byte(thisReg))
    c.emitByte(argCount)
}
```

### 6.3 Phase 3: Bytecode Definitions

**File:** `pkg/vm/bytecode.go`

```go
const (
    // ... existing opcodes ...

    OpTailCall       OpCode = 109  // Rx FuncReg ArgCount: Tail call (frame reuse)
    OpTailCallMethod OpCode = 110  // Rx FuncReg ThisReg ArgCount: Tail method call
)
```

**Update String() method:**
```go
func (op OpCode) String() string {
    switch op {
    // ... existing cases ...
    case OpTailCall:
        return "OpTailCall"
    case OpTailCallMethod:
        return "OpTailCallMethod"
    default:
        return fmt.Sprintf("UnknownOpcode(%d)", op)
    }
}
```

### 6.4 Phase 4: VM Execution

**File:** `pkg/vm/vm.go`

Add OpTailCall and OpTailCallMethod cases in `run()` loop as described in Section 5.2.

**File:** `pkg/vm/op_tailcall.go` (new file for organization)

Extract tail call logic into dedicated file:
```go
package vm

// executeTailCall implements tail call optimization by reusing the current frame
func (vm *VM) executeTailCall(calleeVal Value, args []Value, frame *CallFrame,
                               closure **ClosureObject, function **FunctionObject,
                               code *[]byte, constants *[]Value, registers *[]Value,
                               ip *int) error {
    // Implementation from Section 5.2
    // ...
}
```

---

## 7. Testing Strategy

### 7.1 Unit Tests

**File:** `tests/scripts/tco_basic.ts`
```typescript
'use strict';
// expect: 1
let count = 0;
function f(n: number): number {
  if (n === 0) {
    count++;
    return 1;
  }
  return f(n - 1);
}
f(10000);
console.log(count);
```

**File:** `tests/scripts/tco_mutual.ts`
```typescript
'use strict';
// expect: 42
function even(n: number): boolean {
  if (n === 0) return true;
  return odd(n - 1);
}
function odd(n: number): boolean {
  if (n === 0) return false;
  return even(n - 1);
}
console.log(even(10000) ? 42 : 0);
```

**File:** `tests/scripts/tco_accumulator.ts`
```typescript
'use strict';
// expect: 5050
function sum(n: number, acc: number): number {
  if (n === 0) return acc;
  return sum(n - 1, acc + n);
}
console.log(sum(100, 0));
```

### 7.2 Test262 Validation

**Baseline Before TCO Implementation:**
```bash
# Full language suite (2025-10-13)
./paserati-test262 -path ./test262 -subpath "language" -suite -filter -timeout 0.5s

# Results:
# language (TOTAL): 73.9% pass rate (17,456/23,634 tests) - 1m15.552s
```

**Target TCO Tests:**
```bash
# Run all TCO tests
./paserati-test262 -path ./test262 -filter -timeout 2s | grep "test/language/statements" | grep tco

# Target tests:
# - test/language/statements/return/tco.js (onlyStrict flag)
# - test/language/statements/if/tco-if-body.js
# - test/language/statements/if/tco-else-body.js
# - test/language/statements/switch/tco-case-body.js
# - test/language/statements/try/tco-catch-finally.js (should be skipped - TCO disabled in try)
# - Many more in: for/tco-*.js, do-while/tco-*.js, labeled/tco.js, etc.
```

**Note:** Test262 TCO tests have `flags: [onlyStrict]` but since Paserati doesn't distinguish strict mode, these tests will run in our default mode. If tests fail due to strict mode requirements unrelated to TCO, we may need to add basic strict mode detection.

### 7.3 Performance Testing

**File:** `tests/scripts/tco_performance.ts`
```typescript
'use strict';
// expect: success
function fib(n: number, a: number, b: number): number {
  if (n === 0) return a;
  if (n === 1) return b;
  return fib(n - 1, b, a + b);
}

// Should complete without stack overflow
let result = fib(100000, 0, 1);
console.log("success");
```

### 7.4 Regression Testing

Ensure TCO doesn't break:
- Regular (non-tail) calls
- Method calls with 'this' binding
- Constructor calls
- Native function calls
- Generator/async functions
- Exception handling
- Upvalue capturing

---

## 8. Implementation Phases

### Phase 1: Foundation (Week 1)
- [ ] Add tail position tracking to Compiler struct
- [ ] Implement tail position propagation in return statements
- [ ] Add OpTailCall and OpTailCallMethod opcodes
- [ ] Create basic smoke tests

**Deliverable:** Compiler can identify tail calls and emit OpTailCall

### Phase 2: VM Execution (Week 2)
- [ ] Implement OpTailCall execution in VM
- [ ] Handle upvalue closing
- [ ] Handle frame reuse with register management
- [ ] Add fallback to regular call for edge cases

**Deliverable:** Simple tail recursive functions work (10,000 iterations)

### Phase 3: Comprehensive Tail Position (Week 3)
- [ ] Implement tail position tracking for if/else
- [ ] Implement tail position tracking for switch
- [ ] Implement tail position tracking for labeled statements
- [ ] Disable tail position in try blocks, generators, async

**Deliverable:** All tail position contexts work correctly

### Phase 4: Method Calls (Week 4)
- [ ] Implement OpTailCallMethod
- [ ] Handle 'this' binding preservation
- [ ] Test method recursion

**Deliverable:** Tail recursive methods work

### Phase 5: Test262 Compliance (Week 5)
- [ ] Run all TCO Test262 tests
- [ ] Fix edge cases discovered by tests
- [ ] Validate 100,000 iteration requirement
- [ ] Performance profiling

**Deliverable:** All Test262 TCO tests pass

### Phase 6: Polish (Week 6)
- [ ] Code review and cleanup
- [ ] Documentation
- [ ] Performance optimization
- [ ] Update CLAUDE.md with TCO status

**Deliverable:** Production-ready TCO implementation

---

## 9. Edge Cases and Limitations

### 9.1 Handled Edge Cases

1. **Native function calls:** Fall back to regular call (native executes immediately)
2. **Constructor calls:** Not tail calls by spec
3. **Insufficient registers:** Fall back to regular call if new function needs more registers
4. **Generator/async functions:** TCO disabled inside these functions
5. **Try blocks:** TCO disabled inside try blocks
6. **Finally blocks:** TCO disabled (pending actions need frame)

### 9.2 Implementation Limits

1. **Max recursion with TCO:** Limited by register stack size, not frame count
   - RegisterStack = `[RegFileSize * MaxFrames]Value` = very large
   - Practically unlimited for typical functions

2. **Mixed tail/non-tail:** Non-tail calls still grow stack normally

3. **Indirect calls:** Function stored in variable and called is still optimized if in tail position

### 9.3 Future Enhancements

1. **Tail call statistics:** Track TCO hit rate for profiling
2. **Bytecode annotations:** Mark functions as tail-recursive for better optimization
3. **Register window resizing:** Allow growing register window for larger tail calls

---

## 10. Performance Considerations

### 10.1 Expected Performance Impact

**Positive:**
- Eliminates stack growth for tail recursive functions
- Enables algorithms that were previously stack-overflow prone
- Reduces memory pressure (fewer frames allocated)

**Neutral/Minimal:**
- Frame reuse overhead is minimal (one-time upvalue close, register copy)
- OpTailCall vs OpCall execution time is comparable
- No impact on non-tail calls

**Potential Concerns:**
- Compiler overhead for tail position analysis (negligible, one-time at compile)
- Upvalue closing on every tail call (necessary for correctness)

### 10.2 Optimization Opportunities

1. **Fast path for simple tail calls:** If no upvalues, skip closing logic
2. **Inline tail call detection:** Merge OpTailCall into OpCall with flag bit
3. **Specialized opcodes:** OpTailCallNoArgs, OpTailCallOneArg for common cases

---

## 11. Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Breaks existing exception handling | High | Low | Comprehensive testing of try/catch/finally |
| Upvalue corruption | High | Medium | Validate upvalue closing in all paths |
| Register window corruption | High | Low | Extensive testing with varied register usage |
| Performance regression (< 73.9% pass rate) | High | Low | Profile before/after, validate with full test suite |
| Test262 compliance gaps | Medium | Medium | Iterative testing and fixing |
| Breaks generator/async | High | Low | Disable TCO in these contexts |
| Strict mode requirement gaps | Low | Low | Monitor Test262 TCO test results; add basic strict detection if needed |

---

## 12. Success Criteria

### 12.1 Functional Requirements

- ✅ Tail recursive functions with 100,000 iterations execute without stack overflow
- ✅ All Test262 TCO tests pass
- ✅ Mutual recursion works (even/odd example)
- ✅ Tail calls in all syntactic positions (return, if, switch, etc.)
- ✅ Method tail calls preserve 'this' binding
- ✅ Exception handling remains correct
- ✅ Upvalues handled correctly

### 12.2 Non-Functional Requirements

- ✅ **Maintain baseline:** Language suite pass rate ≥ 73.9% (no regressions)
- ✅ **Performance:** Full language suite execution time ≤ 1m30s (allowing small overhead)
- ✅ No performance regression on non-tail recursive code
- ✅ Minimal memory overhead
- ✅ Code is maintainable and well-documented
- ✅ Smoke tests (TestScripts) remain green

### 12.3 Regression Testing Baseline

Before TCO implementation (2025-10-13):
```
language (TOTAL)             23634    17456     6149        0       29    73.9%    1m15.552s
```

After TCO implementation, validate:
```bash
# Should maintain or improve pass rate
./paserati-test262 -path ./test262 -subpath "language" -suite -filter -timeout 0.5s

# Target: ≥ 73.9% pass rate, ideally higher with TCO tests passing
```

---

## 13. References

### ECMAScript Specification
- **ES2015 Section 14.6.1:** Static Semantics: HasCallInTailPosition
- **ES2015 Section 14.6.2:** Runtime Semantics: PrepareForTailCall

### Test262
- Test suite: `test262/test/language/statements/*/tco*.js`
- Helper: `test262/harness/tcoHelper.js` ($MAX_ITERATIONS = 100,000)

### Implementation Files
- `pkg/vm/vm.go` - CallFrame structure, OpCall/OpReturn execution
- `pkg/vm/call.go` - prepareCall function
- `pkg/vm/exceptions.go` - Exception handling
- `pkg/compiler/compile_expression.go` - compileCallExpression
- `pkg/compiler/compile_statement.go` - compileReturnStatement

---

## 14. Approval and Sign-off

**Design Review:** Pending
**Implementation Start:** After approval
**Target Completion:** 6 weeks from start

**Reviewers:**
- [ ] Architecture review
- [ ] Performance review
- [ ] Security review (upvalue handling)

---

**End of Design Document**

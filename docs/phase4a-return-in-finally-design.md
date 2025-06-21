# Phase 4a: Return Statements in Finally Blocks - Design Document

## Overview

This document outlines the design for implementing return statement handling within finally blocks, which is a critical feature for complete TypeScript/JavaScript compatibility.

## Problem Statement

In JavaScript/TypeScript, return statements in finally blocks have special semantics:
1. A return in finally **overrides** any pending return from try/catch
2. A return in finally **suppresses** any pending exception from try/catch  
3. Finally blocks without returns allow pending actions to continue normally

## Current Infrastructure

We already have the foundation in place:
- `PendingAction` enum: `ActionNone`, `ActionReturn`, `ActionThrow`
- VM fields: `pendingAction`, `pendingValue`, `finallyDepth`
- Finally block execution that saves/restores pending actions

## Design Approach

### 1. Compilation Strategy

We need to distinguish between returns in finally blocks vs regular returns:

**New Opcode**: `OpReturnFinally` 
- Similar to `OpReturn` but has special semantics when executed in finally context
- Format: `OpReturnFinally Rx` - return value in register Rx

**Compiler Changes**:
- Track finally block compilation context
- Emit `OpReturnFinally` instead of `OpReturn` when inside finally blocks
- Keep existing `OpReturn` for regular contexts

### 2. VM Execution Logic

**OpReturn Behavior** (unchanged):
- In try/catch: Set pending action if finally exists, otherwise return immediately
- Outside try/catch/finally: Return immediately

**OpReturnFinally Behavior** (new):
- Always return immediately, clearing any pending actions
- This implements the "finally return overrides everything" semantics

### 3. Implementation Details

#### Compiler Context Tracking
```go
type Compiler struct {
    // ... existing fields
    inFinallyBlock bool  // Track if we're compiling inside finally
}

func (c *Compiler) compileFinallyBlock(block *parser.BlockStatement) {
    prevInFinally := c.inFinallyBlock
    c.inFinallyBlock = true
    defer func() { c.inFinallyBlock = prevInFinally }()
    
    // Compile finally block contents
    c.compileNode(block, hint)
}

func (c *Compiler) compileReturnStatement(node *parser.ReturnStatement, hint Register) {
    // Compile return value
    if node.Value != nil {
        c.compileNode(node.Value, returnReg)
    } else {
        c.emitOpCode(vm.OpLoadUndefined, node.Token.Line)
        c.emitByte(byte(returnReg))
    }
    
    // Choose opcode based on context
    if c.inFinallyBlock {
        c.emitOpCode(vm.OpReturnFinally, node.Token.Line)
    } else {
        c.emitOpCode(vm.OpReturn, node.Token.Line)
    }
    c.emitByte(byte(returnReg))
}
```

#### VM Execution
```go
case OpReturnFinally:
    srcReg := code[ip]
    ip++
    result := registers[srcReg]
    
    // Finally returns override any pending actions
    vm.pendingAction = ActionNone
    vm.pendingValue = Undefined
    vm.finallyDepth = 0
    
    // Return immediately (same logic as OpReturn for actual return)
    frame.ip = ip
    vm.closeUpvalues(frame.registers)
    
    // ... rest of return logic (frame management, etc.)
```

## Test Cases

### Test 1: Finally Return Overrides Try Return
```typescript
function test1() {
    try {
        return "try";
    } finally {
        return "finally";
    }
}
// expect: "finally"
```

### Test 2: Finally Return Suppresses Exception
```typescript
function test2() {
    try {
        throw new Error("error");
    } finally {
        return "finally";
    }
}
// expect: "finally" (no exception)
```

### Test 3: Finally Without Return Preserves Try Return
```typescript
function test3() {
    try {
        return "try";
    } finally {
        console.log("cleanup");
    }
}
// expect: "try"
```

### Test 4: Finally Without Return Preserves Exception
```typescript
function test4() {
    try {
        throw new Error("error");
    } catch (e) {
        throw new Error("catch error");
    } finally {
        console.log("cleanup");
    }
}
// expect_runtime_error: catch error
```

### Test 5: Nested Finally Returns
```typescript
function test5() {
    try {
        try {
            return "inner try";
        } finally {
            return "inner finally";
        }
    } finally {
        return "outer finally";
    }
}
// expect: "outer finally"
```

## Edge Cases

1. **Multiple Returns in Finally**: Only the last executed return matters
2. **Finally with Catch**: Return in finally overrides both try and catch returns
3. **Nested Try/Finally**: Each finally can override its respective scope
4. **Finally with Exception Handling**: Return in finally suppresses exceptions

## Implementation Plan

1. ✅ Design document (this document)
2. 🔲 Add `OpReturnFinally` opcode
3. 🔲 Update compiler to track finally context and emit appropriate opcodes
4. 🔲 Implement VM execution logic for `OpReturnFinally`
5. 🔲 Create comprehensive test suite
6. 🔲 Update disassembly support for new opcode
7. 🔲 Integration testing and validation

## Backward Compatibility

This change is fully backward compatible:
- Existing `OpReturn` behavior unchanged
- New `OpReturnFinally` only used in new contexts
- No changes to existing exception handling

## Performance Considerations

- Minimal overhead: only one additional opcode
- No change to normal return performance
- Finally return handling is already an edge case, so performance impact is negligible

## Future Extensions

This design provides a foundation for:
- Phase 4d: Advanced control flow (break/continue in finally)
- Nested try/finally optimization
- Better debugging/stack trace support for finally returns
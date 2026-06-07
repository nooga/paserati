# Phase 4a: Return in Finally Blocks - Implementation Fix Plan

## Problem Statement

Returns inside try/catch blocks are not properly handled when there's a finally block. The finally block should execute before the return, and the original return value should be preserved (unless the finally block itself returns).

Current behavior:
- Returns in try/catch blocks are ignored when there's a finally block
- The function continues to the normal return at the end instead

Expected behavior:
- Returns in try/catch should set a "pending return" 
- Finally blocks execute with this pending return
- After finally completes, the pending return executes (unless finally overrides it)

## Solution Overview

Use a similar mechanism to exception handling - returns inside try/catch blocks with finally handlers become "pending actions" that execute after finally blocks complete.

## Implementation Details

### 1. Compiler Changes

#### Track Finally Context
```go
// In Compiler struct
tryFinallyDepth int  // Number of enclosing try-with-finally blocks
```

#### Update Try Statement Compilation
```go
func (c *Compiler) compileTryStatement(node *parser.TryStatement, hint Register) (Register, errors.PaseratiError) {
    if node.FinallyClause != nil {
        c.tryFinallyDepth++  // Increment when entering try-with-finally
        defer func() {
            c.tryFinallyDepth--  // Decrement when leaving
        }()
    }
    
    // Compile try block - any returns will see tryFinallyDepth > 0
    // Compile catch block - returns here also see tryFinallyDepth > 0  
    // Compile finally block
}
```

#### Update Return Statement Compilation
```go
func (c *Compiler) compileReturnStatement(node *parser.ReturnStatement, hint Register) (Register, errors.PaseratiError) {
    // Compile return value into valueReg
    
    if c.tryFinallyDepth > 0 {
        // We're inside at least one try-with-finally
        c.emitReturnFinally(valueReg, line)
    } else {
        // Normal return
        c.emitReturn(valueReg, line)
    }
}
```

#### Add OpReturnFinally Opcode
```go
// In opcodes.go
OpReturnFinally OpCode = 0x__ // Assign next available opcode

// In compiler emission methods
func (c *Compiler) emitReturnFinally(valueReg Register, line int) {
    c.emitOpCode(vm.OpReturnFinally, line)
    c.emitByte(byte(valueReg))
}
```

### 2. VM Changes

#### Add Pending Action Tracking
```go
// In VM struct
type PendingAction int
const (
    ActionNone PendingAction = iota
    ActionReturn
    ActionThrow
)

type VM struct {
    // ... existing fields ...
    pendingAction PendingAction
    pendingValue  Value
    finallyDepth  int
}
```

#### Implement OpReturnFinally
```go
case OpReturnFinally:
    // Read the return value register
    returnReg := frame.code[frame.ip+1]
    returnValue := frame.registers[returnReg]
    frame.ip += 2
    
    // Set pending return
    vm.pendingAction = ActionReturn
    vm.pendingValue = returnValue
    
    // Find and jump to finally handler(s)
    handlers := vm.findExceptionHandlers(frame.ip)
    for _, handler := range handlers {
        if handler.IsFinally {
            frame.ip = handler.HandlerPC
            vm.finallyDepth++
            continue startExecution // Jump to finally block
        }
    }
    
    // No finally handler found? Just return normally
    return returnValue, nil
```

#### Handle Pending Actions After Finally
At the end of finally blocks (or with explicit OpHandlePending):
```go
case OpHandlePending:  // Emitted by compiler at end of finally blocks
    if vm.finallyDepth > 0 {
        vm.finallyDepth--
    }
    
    if vm.pendingAction == ActionReturn && vm.finallyDepth == 0 {
        // All finally blocks completed, execute pending return
        return vm.pendingValue, nil
    } else if vm.pendingAction == ActionThrow && vm.finallyDepth == 0 {
        // Re-throw pending exception
        // ... exception handling code ...
    }
    
    // Continue normal execution
    frame.ip++
```

### 3. Nested Finally Handling

The mechanism naturally handles nested finally blocks:

1. Inner `return` sets pending action and jumps to inner finally
2. Inner finally executes with `finallyDepth = 1`
3. At end of inner finally, `finallyDepth` decrements but is still > 0
4. VM finds and jumps to outer finally
5. Outer finally executes with `finallyDepth = 1` 
6. At end of outer finally, `finallyDepth = 0`, pending return executes

If a finally block has its own return, it simply calls `OpReturnFinally` again, overriding any previous pending return.

## Test Cases to Verify

1. Simple try-return-finally
2. Nested try-finally with returns at different levels
3. Return in catch block with finally
4. Return in finally block (should override previous returns)
5. Multiple nested finally blocks
6. Exception in finally with pending return

## Benefits of This Approach

1. **Compile-time decision**: Compiler decides whether to use `OpReturn` or `OpReturnFinally`
2. **No runtime overhead**: Normal returns outside try-finally are unaffected
3. **Unified mechanism**: Returns and exceptions follow similar paths through finally blocks
4. **Handles nesting**: Works correctly with arbitrary nesting of try-finally blocks
5. **Clean separation**: Compiler tracks context, VM executes pending actions

## Implementation Order

1. Add `tryFinallyDepth` to Compiler struct
2. Update `compileTryStatement` to track finally depth
3. Update `compileReturnStatement` to emit `OpReturnFinally` when needed
4. Add pending action fields to VM
5. Implement `OpReturnFinally` in VM
6. Add `OpHandlePending` and emit at end of finally blocks
7. Test with `exception_control_flow_returns.ts`
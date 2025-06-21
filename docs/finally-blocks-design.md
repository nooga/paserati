# Finally Blocks Implementation Design

## Overview

This document outlines the design for implementing finally blocks in Paserati's exception handling system. Finally blocks are the next logical enhancement after completing Phases 1 & 2 (basic try/catch and Error objects).

## Key Challenges

Finally blocks are complex because they must execute in **all** exit scenarios:

1. **Normal completion** of try/catch
2. **Exception thrown** and caught
3. **Exception thrown** and uncaught  
4. **Return statement** in try/catch
5. **Break/Continue** in try/catch (inside loops)
6. **Throw in finally** (overrides pending actions)

## Design Approach: Exception Table Extension

Instead of a complex control flow stack, we'll extend the existing exception table approach that has proven successful for basic try/catch.

**Key Insight**: Finally blocks are **just another type of exception handler** that always "catches" all exit attempts.

### Core Data Structures

#### Updated Exception Handler
```go
type ExceptionHandler struct {
    TryStart     int    // PC where try block starts (inclusive)
    TryEnd       int    // PC where try block ends (exclusive)
    HandlerPC    int    // Where to jump when exception caught/finally needed
    CatchReg     int    // Register to store exception (-1 if finally only)
    IsCatch      bool   // true for catch, false for finally
    IsFinally    bool   // NEW: true for finally blocks
    FinallyReg   int    // NEW: Register to store pending action/value (-1 if not needed)
}
```

#### VM State for Pending Actions
```go
type PendingAction int
const (
    ActionNone PendingAction = iota
    ActionReturn
    ActionThrow
    ActionBreak    // Future: for break in loops
    ActionContinue // Future: for continue in loops
)

type VM struct {
    // ... existing fields
    pendingAction PendingAction
    pendingValue  Value
    finallyDepth  int // Track nested finally blocks
}
```

#### Updated AST
```go
type TryStatement struct {
    Token        lexer.Token      // The 'try' token
    Body         *BlockStatement  // The try block
    CatchClause  *CatchClause     // Optional catch clause
    FinallyBlock *BlockStatement  // NEW: Optional finally block
}
```

## Implementation Strategy

### Phase 3.1: AST and Parser Updates

**Files to Modify:**
- `pkg/parser/ast.go` - Add FinallyBlock field to TryStatement
- `pkg/parser/parser.go` - Enable finally parsing (currently rejected)
- `pkg/lexer/lexer.go` - Ensure FINALLY token is properly supported

**Changes:**
1. Update TryStatement AST node with FinallyBlock field
2. Remove the parser rejection of finally blocks
3. Add finally block parsing logic
4. Update String() methods for debugging

### Phase 3.2: Compiler Strategy

**Core Compiler Logic:**

For each try/finally block:
1. **Normal completion path**: Add finally handler covering normal exit
2. **Return statements**: Generate OpReturn that checks for finally blocks
3. **Exception path**: Finally executes after catch (or during unwinding if no catch)
4. **Finally-to-finally**: Handle nested finally blocks

**Exception Table Generation:**
```go
func (c *Compiler) compileTryStatement(node *parser.TryStatement) {
    tryStart := len(c.chunk.Code)
    
    // Compile try body
    c.compileNode(node.Body)
    
    // If finally exists, ALL exits must go through it
    if node.FinallyBlock != nil {
        // Add finally handler for normal completion
        finallyPC := c.generateFinallyHandler(node.FinallyBlock)
        normalExitJump := c.emitJump(OpJump) // Jump to finally
        
        // Compile catch if present
        if node.CatchClause != nil {
            catchPC := len(c.chunk.Code)
            c.compileCatchClause(node.CatchClause)
            // After catch, also jump to finally
            c.emitJump(OpJump) // Jump to finally
            
            // Add catch handler to exception table
            c.addExceptionHandler(tryStart, len(c.chunk.Code), catchPC, true, false)
        }
        
        // Add finally handler to exception table
        c.addExceptionHandler(tryStart, len(c.chunk.Code), finallyPC, false, true)
        
        // Patch jumps to point to finally block
        c.patchJump(normalExitJump)
    } else {
        // Original logic for try/catch without finally
        // ...
    }
}
```

### Phase 3.3: VM Execution Strategy

**Core Principle**: Finally blocks are **mandatory handlers** that always execute.

1. **During unwinding**: If finally handler found, execute it before continuing unwinding
2. **Normal execution**: Finally executes at end of try/catch
3. **Pending actions**: VM tracks what should happen after finally completes

**VM Modifications:**
```go
func (vm *VM) executeFinally(handler *ExceptionHandler) {
    // Save current pending action/value
    savedAction := vm.pendingAction
    savedValue := vm.pendingValue
    
    // Execute finally block
    frame := &vm.frames[vm.frameCount-1]
    frame.ip = handler.HandlerPC
    vm.finallyDepth++
    
    // After finally completes, resume pending action
    vm.finallyDepth--
    if vm.finallyDepth == 0 {
        switch savedAction {
        case ActionReturn:
            // Resume the return with saved value
            return vm.executeReturn(savedValue)
        case ActionThrow:
            // Resume throwing the saved exception
            vm.throwException(savedValue)
        case ActionNone:
            // Normal completion, continue execution
        }
    }
}
```

## Execution Examples

### Example 1: Normal Completion
```typescript
try {
    console.log("try");
} finally {
    console.log("finally");
}
console.log("after");
```

**Execution Flow:**
1. Execute try block → print "try"
2. Normal completion → jump to finally handler
3. Execute finally block → print "finally"  
4. Continue execution → print "after"

### Example 2: Exception with Finally
```typescript
try {
    throw "error";
} catch (e) {
    console.log("caught:", e);
} finally {
    console.log("finally");
}
```

**Execution Flow:**
1. Execute try block → throw "error"
2. Exception unwinding → find catch handler
3. Execute catch block → print "caught: error"
4. After catch → jump to finally handler
5. Execute finally block → print "finally"

### Example 3: Return in Try with Finally
```typescript
function test() {
    try {
        return "from try";
    } finally {
        console.log("finally");
    }
}
```

**Execution Flow:**
1. Execute try block → encounter return
2. Set pending action = ActionReturn, pending value = "from try"
3. Jump to finally handler
4. Execute finally block → print "finally"
5. Resume pending return → return "from try"

### Example 4: Exception in Finally (Override)
```typescript
try {
    return "from try";
} finally {
    throw "finally error"; // This overrides the pending return
}
```

**Execution Flow:**
1. Execute try block → encounter return, set pending action
2. Jump to finally handler
3. Execute finally block → throw "finally error"
4. Finally exception overrides pending return
5. Exception propagates as "finally error"

## Implementation Phases

### Phase 3a: Basic Finally ✅ **COMPLETED**
- ✅ AST updates for finally blocks
- ✅ Parser support for finally syntax  
- ✅ Basic compiler generation
- ✅ VM execution for normal completion
- ✅ Exception path with catch blocks

**Deliverables:**
- ✅ Finally blocks parse correctly
- ✅ Normal completion path works (try → finally → continue)
- ✅ Exception path works (try → catch → finally → continue)  
- ✅ Basic test cases pass
- ✅ No regressions in existing try/catch functionality

**Working Examples:**
```typescript
// Normal completion - ✅ WORKING
try { console.log("try"); } finally { console.log("finally"); }

// With exception and catch - ✅ WORKING  
try { throw "error"; } catch (e) { console.log("caught"); } finally { console.log("finally"); }
```

### Phase 3b: Exception Handling (Week 1-2)
- ✅ Finally execution during exception unwinding
- ✅ Proper interaction with catch blocks
- ✅ Exception table updates

**Deliverables:**
- try/catch/finally works correctly
- Exception unwinding goes through finally
- Complex control flow test cases

### Phase 3c: Control Flow (Week 2)
- ✅ Return statement handling
- ✅ Pending action tracking  
- ✅ Finally-in-finally support

**Deliverables:**
- Return statements in try/catch trigger finally
- Nested finally blocks work correctly
- Comprehensive control flow tests

### Phase 3d: Edge Cases (Week 2-3)
- ✅ Throw in finally block (overrides pending actions)
- ✅ Nested finally blocks
- ✅ Complex control flow scenarios

**Deliverables:**
- All edge cases handled correctly
- Performance benchmarks
- Documentation complete

## Benefits of This Design

1. **Reuses existing exception table** - minimal VM changes required
2. **Consistent with current architecture** - finally is just another handler type
3. **Handles all control flow** - return, throw, normal completion uniformly
4. **Extensible** - easy to add break/continue support later for loops
5. **Performance** - zero overhead when no finally blocks present
6. **Maintainable** - builds on proven exception handling foundation

## Testing Strategy

### Basic Finally Tests
```typescript
// Normal completion
try { console.log("try"); } finally { console.log("finally"); }

// Exception with finally
try { throw "error"; } finally { console.log("finally"); }

// Try/catch/finally
try { throw "error"; } catch (e) { console.log("caught"); } finally { console.log("finally"); }
```

### Control Flow Tests
```typescript
// Return in try
function test1() {
    try { return "try"; } finally { console.log("finally"); }
}

// Return in catch  
function test2() {
    try { throw "error"; } catch (e) { return "catch"; } finally { console.log("finally"); }
}

// Exception in finally (override)
function test3() {
    try { return "try"; } finally { throw "finally error"; }
}
```

### Nested Finally Tests
```typescript
// Nested try/finally
try {
    try { throw "inner"; } finally { console.log("inner finally"); }
} finally {
    console.log("outer finally");
}

// Complex nesting
function complex() {
    try {
        try { return "inner"; } finally { console.log("inner finally"); }
    } catch (e) {
        console.log("outer catch");
    } finally {
        console.log("outer finally");
    }
}
```

## Files to Modify

### Parser
- `pkg/parser/ast.go` - Add FinallyBlock field to TryStatement
- `pkg/parser/parser.go` - Enable finally parsing

### Compiler  
- `pkg/compiler/compile_exception.go` - Add finally compilation logic
- `pkg/vm/bytecode.go` - Update exception handler structure

### VM
- `pkg/vm/vm.go` - Add pending action state and finally execution
- `pkg/vm/exceptions.go` - Update exception handling for finally blocks

### Tests
- `tests/scripts/finally_*.ts` - Comprehensive finally block tests
- `tests/scripts/try_catch_finally_*.ts` - Integration tests

## Risk Mitigation

1. **Backward Compatibility**: All existing try/catch code continues to work unchanged
2. **Incremental Implementation**: Each phase can be tested independently  
3. **Fallback Strategy**: If complex cases prove problematic, can ship basic finally first
4. **Testing Strategy**: Comprehensive test suite covers all control flow scenarios
5. **Performance**: No impact on existing code, minimal overhead for finally blocks

## Future Enhancements

### Break/Continue Support
After finally blocks are stable, can extend to handle break/continue in loops:
```typescript
for (let i = 0; i < 5; i++) {
    try {
        if (i === 2) break;  // Should trigger finally
    } finally {
        console.log("finally", i);
    }
}
```

### Multiple Finally Blocks
Could potentially support multiple finally blocks for complex cleanup:
```typescript
try {
    // code
} finally {
    // cleanup 1
} finally {
    // cleanup 2  
}
```

### Async Finally
Future support for async/await in finally blocks:
```typescript
try {
    await someOperation();
} finally {
    await cleanup();
}
```

## Conclusion

This design leverages the proven exception table approach to implement finally blocks with minimal architectural changes. The phased implementation ensures backward compatibility while providing a solid foundation for advanced control flow features.

The key insight is treating finally blocks as special exception handlers that "catch" all exit attempts, allowing the existing VM unwinding logic to handle the complex control flow naturally.
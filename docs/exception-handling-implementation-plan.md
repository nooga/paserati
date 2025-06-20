# Exception Handling Implementation Plan

This document outlines the phased implementation of try/catch/finally/throw exception handling in Paserati, designed to be added incrementally without breaking existing functionality.

## 🎯 Current Status

- ✅ **Phase 1**: Basic try/catch/throw - **COMPLETED**
- 🚧 **Phase 2**: Error objects and stack traces - **PLANNED**  
- 🚧 **Phase 3**: Finally blocks - **PLANNED**
- 🚧 **Phase 4**: Advanced features - **PLANNED**

**Latest Update**: Phase 1 fully implemented with working try/catch blocks, exception unwinding, and uncaught exception handling.

## Overview

Exception handling will be implemented using an **exception table approach** with minimal new opcodes. The key insight is that we only need `OpThrow` - all control flow can be handled through existing jump instructions and VM-level unwinding logic.

## Design Principles

1. **Minimal bytecode changes** - Only add `OpThrow` opcode
2. **Table-driven approach** - Exception handlers stored separately from bytecode
3. **Non-breaking implementation** - Each phase can be merged without breaking existing code
4. **TypeScript compatibility** - Follow JavaScript/TypeScript exception semantics

## Architecture

### Exception Table

```go
type ExceptionHandler struct {
    TryStart     int    // PC where try block starts (inclusive)
    TryEnd       int    // PC where try block ends (exclusive)
    HandlerPC    int    // Where to jump when exception caught
    CatchReg     int    // Register to store exception (-1 if finally only)
    IsCatch      bool   // true for catch, false for finally
}

type Chunk struct {
    // ... existing fields
    ExceptionTable []ExceptionHandler
}
```

### VM Exception State

```go
type VM struct {
    // ... existing fields
    currentException Value  // Current thrown exception
    unwinding        bool   // True during exception unwinding
}
```

## Implementation Phases

### Phase 1: Basic Try/Catch (No Finally) ✅ **COMPLETED**

**Goal**: Implement basic exception throwing and catching without finally blocks.

**Status**: ✅ Fully implemented and tested as of latest commit.

#### 1.1 Parser/AST Changes
```go
// ast.go
type TryStatement struct {
    BaseStatement
    Token       lexer.Token      // 'try' token
    Body        *BlockStatement  // try block
    CatchClause *CatchClause     // optional catch
    // FinallyBlock not yet implemented
}

type CatchClause struct {
    Token     lexer.Token      // 'catch' token
    Parameter *Identifier      // exception variable (optional in ES2019+)
    Body      *BlockStatement  // catch block
}

type ThrowStatement struct {
    BaseStatement
    Token lexer.Token    // 'throw' token
    Value Expression     // expression to throw
}
```

#### 1.2 Lexer Updates ✅
- ✅ Added `TRY`, `CATCH`, `THROW`, `FINALLY` keywords to lexer

#### 1.3 Parser Implementation ✅
- ✅ Parse try/catch blocks with AST nodes (`TryStatement`, `CatchClause`, `ThrowStatement`)
- ✅ Parse throw statements with expression validation
- ✅ Reject finally blocks with clear error message (Phase 3 feature)
- ✅ Support optional catch parameters (ES2019+ syntax)

#### 1.4 Type Checker ✅
- ✅ Basic validation that throw has an expression
- ✅ Catch parameter typed as `any` by default
- ✅ Proper scope handling for catch parameters

#### 1.5 Compiler ✅
```go
func (c *Compiler) compileTryStatement(node *parser.TryStatement) {
    tryStart := c.currentPC()
    
    // Compile try body
    c.visit(node.Body)
    
    tryEnd := c.currentPC()
    normalExit := c.emitJump(OpJump)
    
    // Compile catch if present
    if node.CatchClause != nil {
        catchPC := c.currentPC()
        
        // Allocate register for exception
        catchReg := c.allocateRegister()
        if node.CatchClause.Parameter != nil {
            c.defineVariable(node.CatchClause.Parameter.Value, catchReg)
        }
        
        c.visit(node.CatchClause.Body)
        
        // Add to exception table
        handler := ExceptionHandler{
            TryStart:  tryStart,
            TryEnd:    tryEnd,
            HandlerPC: catchPC,
            CatchReg:  catchReg,
            IsCatch:   true,
        }
        c.currentChunk().ExceptionTable = append(c.currentChunk().ExceptionTable, handler)
    }
    
    c.patchJump(normalExit)
}
```

✅ **Implemented in `pkg/compiler/compile_exception.go`** with exception table generation and register allocation.

#### 1.6 VM Implementation ✅
- ✅ Added `OpThrow` opcode (opcode 65)
- ✅ Implemented exception unwinding logic in `pkg/vm/exceptions.go`
- ✅ Exception table search during unwinding
- ✅ Proper uncaught exception handling
- ✅ Register-based exception storage and IP synchronization
- ✅ Bytecode disassembly support for OpThrow

**Files Added/Modified:**
- `pkg/vm/exceptions.go` - Exception handling logic
- `pkg/compiler/compile_exception.go` - Compilation support
- `pkg/vm/bytecode.go` - OpThrow opcode and disassembly
- `pkg/vm/vm.go` - VM exception state and main loop integration

**✅ Working Features:**
```javascript
// Basic try/catch
try { throw 'error'; } catch (e) { console.log('caught:', e); }

// Complex control flow
try { 
  console.log('before throw'); 
  throw 'error'; 
  console.log('after throw'); // skipped
} catch (e) { 
  console.log('caught:', e); 
} 
console.log('after try/catch'); // executes

// Uncaught exceptions
throw 'uncaught error'; // terminates with proper error message

// Optional catch parameters (ES2019+)
try { throw 'error'; } catch { console.log('no parameter'); }
```

### Phase 2: Error Object Support 🚧 **PLANNED**

**Goal**: Add proper Error constructor and stack traces.

**Status**: 🚧 Not yet implemented. Ready for development after Phase 1 completion.

#### 2.1 Error Constructor
```go
// builtins/error_init.go
func InitError() BuiltinInitializer {
    return &errorInitializer{}
}

// Implement Error constructor with:
// - message property
// - name property (default "Error")
// - Basic toString() method
```

#### 2.2 Stack Trace Support
- Capture call stack when Error is constructed
- Add stack property to Error instances
- Format stack traces similar to Node.js/V8

### Phase 3: Finally Blocks 🚧 **PLANNED**

**Goal**: Add finally block support with proper control flow handling.

**Status**: 🚧 Not yet implemented. Depends on Phase 1 completion (✅ ready).

#### 3.1 Parser Updates
- Enable parsing of finally blocks
- Update AST to include FinallyBlock field

#### 3.2 Control Flow Tracking
```go
type VM struct {
    // ... existing fields
    finallyStack []FinallyContext  // Track pending finally blocks
}

type FinallyContext struct {
    pc              int    // PC to jump to for finally
    pendingAction   int    // What to do after finally (return/break/continue/rethrow)
    pendingValue    Value  // Value for return or rethrow
}
```

#### 3.3 Compiler Changes
- Generate finally entries in exception table
- Handle control flow through finally blocks
- Ensure finally executes for all exit paths

#### 3.4 VM Finally Execution
- Execute finally blocks during unwinding
- Handle pending returns/breaks/continues
- Proper exception propagation through finally

### Phase 4: Advanced Features 🚧 **PLANNED**

**Goal**: Add remaining exception handling features.

**Status**: 🚧 Future enhancement. Depends on Phases 1-3 completion.

#### 4.1 Nested Try/Catch
- Proper exception table lookup for nested handlers
- Correct unwinding through multiple try blocks

#### 4.2 Re-throwing
- Support for re-throwing in catch blocks
- Preserve original stack trace

#### 4.3 Custom Error Types
- TypeError, ReferenceError, SyntaxError constructors
- Instanceof checks for error types

#### 4.4 Optional Catch Binding
```javascript
try {
    // ...
} catch {  // No parameter
    // ...
}
```

## Testing Strategy

### Phase 1 Tests
```javascript
// Basic throw/catch
try {
    throw "error";
} catch (e) {
    console.log("caught:", e);
}

// Uncaught exception
function willThrow() {
    throw new Error("uncaught");
}

// Nested function calls
function outer() {
    inner();
}
function inner() {
    throw "from inner";
}
try {
    outer();
} catch (e) {
    console.log("caught from inner:", e);
}
```

### Phase 2 Tests
```javascript
// Error object
const err = new Error("test error");
console.log(err.message);
console.log(err.name);
console.log(err.stack);

// Custom error properties
err.code = "TEST001";
```

### Phase 3 Tests
```javascript
// Finally execution
try {
    console.log("try");
} finally {
    console.log("finally");
}

// Finally with exception
try {
    throw "error";
} catch (e) {
    console.log("catch");
} finally {
    console.log("finally");
}

// Finally with return
function testReturn() {
    try {
        return "from try";
    } finally {
        console.log("finally before return");
    }
}
```

## Migration Path

1. **Phase 1**: Can be merged immediately - adds new syntax without affecting existing code
2. **Phase 2**: Pure addition of Error constructor - no breaking changes
3. **Phase 3**: Finally blocks are optional - existing try/catch continues to work
4. **Phase 4**: All enhancements are backward compatible

## Performance Considerations

- Exception tables are only consulted during unwinding (zero cost for normal flow)
- No overhead for functions without try/catch
- Exception objects allocated only when thrown
- Stack traces captured lazily

## Future Enhancements

- **Async exception handling** - try/catch for promises and async/await
- **Exception filters** - catch blocks with conditions
- **Aggregate exceptions** - for parallel operations
- **Source maps** - accurate stack traces for transpiled code

## Implementation Notes

### Register Allocation
- Catch variable gets a dedicated register
- Finally blocks may need temporary registers for control flow state

### Bytecode Size
- No new opcodes except `OpThrow`
- Exception tables stored separately from instructions
- Minimal impact on bytecode size for non-throwing code

### Type System Integration
- `throw` expression has type `never`
- Catch clause parameter typed as `any` by default
- Could add typed catch clauses in future

## Conclusion

This phased approach allows us to build exception handling incrementally:
1. ✅ **Phase 1 Complete**: Basic try/catch provides immediate value with full exception unwinding
2. 🚧 **Phase 2 Next**: Add proper Error objects for better debugging
3. 🚧 **Phase 3 Future**: Implement finally for complete control flow
4. 🚧 **Phase 4 Future**: Polish with advanced features

**Phase 1 Success**: The implemented exception handling works seamlessly with Paserati's register-based VM, providing TypeScript-compliant exception semantics with zero performance overhead for normal execution. The exception table approach proves effective and the clean architecture enables easy extension for future phases.

Each phase is independently useful and won't break existing code, allowing for safe incremental development and testing.
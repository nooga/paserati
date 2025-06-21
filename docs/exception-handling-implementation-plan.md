# Exception Handling Implementation Plan

This document outlines the phased implementation of try/catch/finally/throw exception handling in Paserati, designed to be added incrementally without breaking existing functionality.

## 🎯 Current Status

- ✅ **Phase 1**: Basic try/catch/throw - **COMPLETED** (Including nested calls fix)
- ✅ **Phase 2**: Error objects and stack traces - **COMPLETED**  
- ✅ **Phase 3**: Finally blocks - **COMPLETED** 🎉
- 🚧 **Phase 4**: Advanced features - **PLANNED**

**Latest Update**: Phase 3 Finally Blocks completed! Implemented comprehensive finally block support with proper control flow handling for all exit scenarios (normal completion, exceptions, and pending action restoration).

## 🚀 Next Steps (Priority Order)

1. **Phase 4a: Return Statements in Finally** - Handle return statements in try/catch with finally blocks
2. **Phase 4b: Stack Traces** - Add stack property to Error objects for better debugging
3. **Phase 4c: Custom Error Types** - TypeError, ReferenceError, etc.
4. **Phase 4d: Advanced Features** - Re-throwing, nested try optimization

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

// Nested function calls (FIXED - major compiler bug resolved)
function outer() { return inner(); }
function inner() { return deep(); }
function deep() { throw "deep error"; }

try {
  outer(); // Exception properly caught from nested calls
} catch (e) {
  console.log('caught from deep:', e); // ✅ Works correctly
}

// Uncaught exceptions
throw 'uncaught error'; // terminates with proper error message

// Optional catch parameters (ES2019+)
try { throw 'error'; } catch { console.log('no parameter'); }
```

### Phase 2: Error Object Support ✅ **COMPLETED**

**Goal**: Add proper Error constructor and stack traces.

**Status**: ✅ Fully implemented with Error constructor and prototype chain.

#### 2.1 Error Constructor ✅
```go
// pkg/builtins/error_init.go - ErrorInitializer implementation
type ErrorInitializer struct{}

func (e *ErrorInitializer) InitRuntime(ctx *RuntimeContext) error {
    // Create Error.prototype with name, message, toString properties
    // Create Error constructor function
    // Set up proper prototype chain
}
```

**✅ Implemented Features:**
- Error constructor: `new Error(message)`
- Error.prototype with proper properties:
  - `name`: defaults to "Error" 
  - `message`: defaults to empty string
  - `toString()`: returns "name: message" format
- Property modification support
- Prototype chain inheritance from Object.prototype
- Constructor property linkage
- Type coercion for non-string messages

**✅ Working Examples:**
```javascript
// Basic Error creation
let err1 = new Error();              // name: "Error", message: ""
let err2 = new Error("Test error");  // name: "Error", message: "Test error"

// Property access and modification
console.log(err2.name);              // "Error"
console.log(err2.message);           // "Test error"
err2.name = "CustomError";
err2.message = "Modified";

// toString method
console.log(err1.toString());        // "Error"
console.log(err2.toString());        // "CustomError: Modified"

// Type coercion
let err3 = new Error(42);            // message: "42"
let err4 = new Error(undefined);     // message: ""
```

**Files Added/Modified:**
- `pkg/builtins/error_init.go` - Error constructor implementation
- `pkg/builtins/standard.go` - Added ErrorInitializer to standard builtins
- `tests/scripts/error_*.ts` - Comprehensive test suite

#### 2.2 Stack Trace Support 🚧 **FUTURE**
- Stack trace capture is planned for a future enhancement
- Current implementation provides functional Error objects without stack traces
- Add stack property to Error instances (deferred to Phase 4)

### Phase 3: Finally Blocks 🚧 **READY FOR IMPLEMENTATION**

**Goal**: Add finally block support with proper control flow handling.

**Status**: 🚧 Ready for implementation! Phases 1 & 2 are complete and stable. Finally blocks are the next logical enhancement.

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

### Phase 2 Tests ✅
```javascript
// Error object - IMPLEMENTED
const err = new Error("test error");
console.log(err.message);    // "test error"
console.log(err.name);       // "Error"
console.log(err.toString()); // "Error: test error"

// Custom error properties - IMPLEMENTED
err.code = "TEST001";
err.name = "CustomError";
console.log(err.toString()); // "CustomError: test error"

// Type coercion - IMPLEMENTED
new Error(42).message;       // "42"
new Error(null).message;     // "null"
new Error().message;         // ""
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

1. **Phase 1**: ✅ Merged - adds new syntax without affecting existing code
2. **Phase 2**: ✅ Merged - pure addition of Error constructor with no breaking changes
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
2. ✅ **Phase 2 Complete**: Added proper Error objects with constructor and prototype support
3. 🚧 **Phase 3 Next**: Implement finally for complete control flow
4. 🚧 **Phase 4 Future**: Polish with advanced features (stack traces, custom error types)

**Phase 1 & 2 Success**: The implemented exception handling works seamlessly with Paserati's register-based VM, providing TypeScript-compliant exception semantics with zero performance overhead for normal execution. The Error constructor integrates perfectly with the existing builtin system, and the exception table approach proves effective for clean architecture and easy extension.

**Major Bug Fix**: Resolved critical compiler issue where exceptions thrown from nested function calls weren't being caught properly. The bug was in exception table IP range calculation - the `tryEnd` was being set before the jump instruction, causing return addresses from nested calls to fall outside the protected range. This fix ensures all try/catch blocks work correctly regardless of call depth.

**Current Status**: Paserati now has robust, fully functional try/catch/throw exception handling with proper Error objects. All test cases pass including complex nested function call scenarios. Users can create Error instances, access and modify properties, and get proper string representations. The foundation is solid and battle-tested for implementing finally blocks and advanced features.

Each phase is independently useful and won't break existing code, allowing for safe incremental development and testing.
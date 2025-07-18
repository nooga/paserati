# Generators Implementation Plan

This document outlines the design and implementation strategy for adding generator functions (`function*` / `yield`) to Paserati while maintaining performance and correctness of the existing VM architecture.

## Overview

Generators enable suspendable functions that can pause execution with `yield` and resume later via the iterator protocol. This requires significant VM changes to support state persistence across function calls.

## Current VM Architecture

- **Register-based execution** with efficient CallFrame management
- **Stack-allocated frames** with automatic cleanup on return
- **Direct execution model** - functions run to completion
- **Performance-critical** - zero overhead for normal function calls

## Design Goals

1. **Zero overhead for non-generator functions** - preserve existing performance
2. **Correct state preservation** - exact register/closure state across yields
3. **Memory safety** - proper lifetime management for suspended state
4. **TypeScript compliance** - proper `Generator<T, TReturn, TNext>` types
5. **Iterator protocol** - standard `.next()`, `.return()`, `.throw()` methods

## Architecture Design

### 1. Dual-Mode Execution Model

**Normal Functions**: Continue using stack-allocated frames (fast path)
**Generator Functions**: Heap-allocate state on first yield (suspend path)

```go
type GeneratorState int
const (
    GeneratorCreated GeneratorState = iota
    GeneratorSuspended
    GeneratorCompleted
)

type GeneratorFrame struct {
    // Captured execution state
    closure     *ClosureObject
    ip          int               // Resume instruction pointer
    registers   []Value          // Heap-allocated register copy
    targetReg   byte             // Caller's destination register
    
    // Generator-specific state
    state       GeneratorState
    yieldValue  Value            // Last yielded value
    sentValue   Value            // Value sent via .next(value)
    
    // Exception state
    thrown      Value            // Exception thrown via .throw()
    hasThrown   bool
}

type GeneratorObject struct {
    Object
    frame       *GeneratorFrame  // Suspended execution state
    function    *FunctionObject  // Original generator function
    done        bool             // Iterator completion state
}
```

### 2. Function Object Extensions

```go
type FunctionObject struct {
    // ... existing fields
    IsGenerator bool             // New flag for generator functions
}
```

### 3. New Bytecode Operations

```go
// New opcodes for generator support
const (
    OpCreateGenerator OpCode = 74  // Create generator object instead of executing
    OpYield          OpCode = 75  // Suspend execution and yield value
    OpResumeGenerator OpCode = 76  // Internal: restore generator state
)
```

**OpCreateGenerator**: Used instead of OpCall for generator functions
- Creates GeneratorObject with initial state
- Returns generator to caller immediately
- No function execution until first .next() call

**OpYield**: Suspends execution and yields value
- Captures current frame state to heap
- Stores yield value in generator object
- Returns control to caller with yielded value

**OpResumeGenerator**: Internal opcode for resuming generators
- Restores register state from GeneratorFrame
- Continues execution from saved IP
- Handles sent values and thrown exceptions

## Implementation Plan

### Phase 1: Lexer & Parser (pkg/lexer/, pkg/parser/) ✅ COMPLETED

**Lexer Changes (`pkg/lexer/lexer.go`):** ✅
- Added YIELD token type to TokenType constants
- Added "yield" to keywords map
- Function* syntax parsing works correctly

**Parser Changes (`pkg/parser/parser.go`):** ✅
- Added IsGenerator field to FunctionLiteral struct
- Modified parseFunctionLiteral() to detect function* syntax (ASTERISK token)
- Added parseYieldExpression() function with proper precedence
- Registered yield expression parser in prefix functions

**AST Nodes (`pkg/parser/ast.go`):** ✅
- Added YieldExpression struct with Token and Value fields
- Implemented required AST node methods (String, expressionNode, etc.)
- Added BaseExpression embedding for type computation support

### Phase 2: Type System (pkg/types/, pkg/checker/) ✅ COMPLETED

**Type Definitions (`pkg/types/`):** ✅ COMPLETED
- GeneratorGeneric added to types/generic.go following ArrayGeneric pattern
- Generator<T, TReturn, TNext> generic type with proper type parameters
- IteratorResult<T, TReturn> embedded in generator body
- Built-in generator type registered in global generics

**Type Checker (`pkg/checker/`):** ✅ COMPLETED
- Added YieldExpression case in checkExpression (expressions.go)
- Modified function type inference for generators (function.go)
- Implemented Generator<T, TReturn, TNext> instantiation using GeneratorGeneric
- Generator functions now properly return instantiated Generator types
- Type checking passes: `function* gen() { yield 1; }; const g = gen(); g.next();`

### Phase 3: Compiler (pkg/compiler/) ✅ COMPLETED

**Compilation Strategy:**
```go
// Modify compileCallExpression for generator calls
func (c *Compiler) compileCallExpression(node *parser.CallExpression) {
    if calleeFunc.IsGenerator {
        c.emit(OpCreateGenerator, destReg, funcReg, argCount)
    } else {
        c.emit(OpCall, destReg, funcReg, argCount)
    }
}

// Add yield expression compilation
func (c *Compiler) compileYieldExpression(node *parser.YieldExpression) {
    valueReg := c.compileExpression(node.Value)
    c.emit(OpYield, valueReg)
}
```

**Implementation Status:**
- ✅ COMPLETED: Added OpCreateGenerator, OpYield, OpResumeGenerator opcodes (bytecode.go)
- ✅ COMPLETED: Modified compileCallExpression to detect generator functions via return type analysis
- ✅ COMPLETED: Added YieldExpression compilation case in compiler.go
- ✅ COMPLETED: Implemented compileYieldExpression function in compile_expression.go
- ✅ COMPLETED: Generator function calls now emit OpCreateGenerator instead of OpCall
- ✅ COMPLETED: Yield expressions now emit OpYield with proper value handling

### Phase 4: VM Execution (pkg/vm/)

**VM State Management (`pkg/vm/vm.go`):**
```go
// Add generator execution cases
case OpCreateGenerator:
    // Create GeneratorObject instead of calling function
    
case OpYield:
    // Capture frame state and suspend execution
    
case OpResumeGenerator:
    // Restore generator state and continue execution
```

**Generator Object (`pkg/vm/value.go`):**
```go
// Implement iterator protocol methods
func (g *GeneratorObject) Next(sentValue Value) IteratorResult
func (g *GeneratorObject) Return(returnValue Value) IteratorResult  
func (g *GeneratorObject) Throw(exception Value) IteratorResult
```

### Phase 5: Built-ins Integration (pkg/builtins/)

**Generator Prototype (`pkg/builtins/generator.go`):**
```go
type GeneratorInitializer struct{}

func (g *GeneratorInitializer) InitTypes(ctx *TypeContext) error {
    // Define Generator<T, TReturn, TNext> type
    // Add prototype methods: next, return, throw
}
```

## Detailed Integration Points

### 1. Function Call Sites
- **Location**: `pkg/vm/vm.go:OpCall` case
- **Change**: Check `function.IsGenerator` flag
- **Action**: Emit `OpCreateGenerator` instead of `OpCall`

### 2. Expression Compilation  
- **Location**: `pkg/compiler/compiler.go:compileExpression`
- **Change**: Add `YieldExpression` case
- **Action**: Compile yield value and emit `OpYield`

### 3. Type Inference
- **Location**: `pkg/checker/checker.go:function type inference`
- **Change**: Detect generator functions and wrap return type
- **Action**: `T` becomes `Generator<YieldType, T, any>`

### 4. Memory Management
- **Location**: `pkg/vm/heap.go` (if exists) or `pkg/vm/vm.go`
- **Change**: Add GeneratorFrame allocation/deallocation
- **Action**: Heap allocate frames, implement cleanup

## Performance Optimizations

### 1. Lazy State Capture
- Only heap-allocate GeneratorFrame on first yield
- Use stack frame for generators that never yield
- Copy-on-write for register arrays

### 2. Register Liveness Analysis
- Only copy live registers to heap
- Track variable lifetimes to minimize state size
- Pool GeneratorFrame objects for reuse

### 3. Fast Path Preservation
- Zero overhead for non-generator functions
- Branch prediction friendly generator checks
- Minimal impact on existing OpCall performance

## Current Implementation Status (COMPLETED)

### ✅ Basic Generator Functionality Working
The implementation now supports:
- **Generator Function Creation**: `function* gen() { yield 42; }`
- **Generator Object Creation**: `const g = gen();` creates a generator object
- **Iterator Protocol**: `g.next()` returns `{value: 42, done: false}`
- **Completion Handling**: Second call to `g.next()` returns `{value: undefined, done: true}`
- **Type System Integration**: Generator functions have proper TypeScript types
- **Property Access**: Generator objects can access `.next()`, `.return()`, `.throw()` methods
- **State Management**: Generators properly track their state (suspended, executing, completed)

### Verified Working Examples
```typescript
// Basic generator functionality
function* gen() { yield 42; }
const g = gen();
console.log(g.next()); // {value: 42, done: false}
console.log(g.next()); // {value: undefined, done: true}

// Generator type checking
function* numGen(): Generator<number, void, unknown> {
    yield 42;
}
```

### Technical Implementation Details
- **Opcodes**: OpCreateGenerator, OpYield, OpResumeGenerator
- **VM Integration**: Generator objects with proper prototype chain
- **Type System**: Generator<T, TReturn, TNext> generic type
- **Memory Management**: Generator frame and state preservation
- **Error Handling**: Proper error messages for invalid operations

## Testing Strategy

### Basic Test Cases
```typescript
// Basic generator functionality
function* simpleGenerator() {
    yield 1;
    yield 2;
    return 3;
}

// Generator with parameters
function* parameterizedGenerator(start: number) {
    yield start;
    yield start + 1;
}

// Generator with sent values
function* interactiveGenerator() {
    const x = yield 1;
    yield x * 2;
}
```

### Integration Tests
- Generator + closures
- Generator + exception handling  
- Generator + class methods
- Generator + async patterns (future)

## Risk Mitigation

### 1. State Corruption
- **Risk**: Incorrect register/closure capture
- **Mitigation**: Comprehensive state validation, extensive testing

### 2. Memory Leaks
- **Risk**: GeneratorFrame not cleaned up
- **Mitigation**: Reference counting, explicit cleanup, GC integration

### 3. Performance Regression
- **Risk**: Slowdown in normal function calls
- **Mitigation**: Benchmark existing performance, minimize hot path changes

## Implementation Order

1. **Basic syntax support** (lexer, parser, AST)
2. **Type system integration** (Generator types, type checking)
3. **Minimal VM support** (OpCreateGenerator, basic GeneratorObject)
4. **Iterator protocol** (.next() method, basic yield/resume)
5. **Advanced features** (.return(), .throw(), exception handling)
6. **Performance optimization** (lazy allocation, register liveness)
7. **Comprehensive testing** (edge cases, integration tests)

## Success Criteria

- [x] Parse `function*` syntax correctly ✅
- [x] Type check generators with proper `Generator<T, TReturn, TNext>` types ✅
- [ ] Execute basic yield/resume cycles
- [ ] Implement complete iterator protocol
- [ ] Pass all generator test cases
- [ ] Zero performance regression for normal functions
- [ ] Memory safety validated (no leaks or corruption)

## Progress Status

### ✅ COMPLETED
- **Phase 1**: Full lexer/parser support for `function*` and `yield`
- **Phase 2**: Complete type system integration for generators
  - GeneratorGeneric<T, TReturn, TNext> following ArrayGeneric pattern
  - YieldExpression type checking
  - Generator function return type inference
  - Type checking passes for basic generator usage
- **Phase 3**: Complete compiler support for generators
  - OpCreateGenerator, OpYield, OpResumeGenerator opcodes added
  - Generator function call detection via return type analysis
  - YieldExpression compilation with proper value handling
  - Compiler emits correct opcodes (verified by runtime error showing OpYield #75)
- **VM Foundation**: TypeGenerator, GeneratorObject, GeneratorFrame structures
- **Built-ins**: generator_init.go with Generator.prototype methods
- **Integration**: GeneratorInitializer registered with builtin system

### 🎯 CURRENT: Phase 4 - VM Execution & Testing
- ✅ COMPLETED: Phase 3 compiler implementation
- Need to implement OpCreateGenerator execution in VM 
- Need to implement OpYield execution and generator suspension
- Need to implement generator .next() method execution  
- Need to test basic generator functionality end-to-end

This plan provides a structured approach to implementing generators while preserving Paserati's performance characteristics and maintaining architectural integrity.
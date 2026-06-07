# Generator Parameter Destructuring Implementation Plan

## Executive Summary

**Problem**: Generator functions with destructuring parameters incorrectly delay TypeError until `.next()` is called, when it should throw immediately during construction.

**Root Cause**: Destructuring code is in the generator function body (as a prologue), which doesn't execute until `.next()` is called.

**Solution**: Introduce `OpInitYield` opcode to mark the boundary between initialization prologue and user code. During `OpCreateGenerator`, execute the function synchronously up to `OpInitYield`, validating parameters and saving state. If prologue fails, generator is never created. If it succeeds, generator starts in `GeneratorSuspendedStart` state with all destructured values ready.

**Benefits**:
- ✅ ECMAScript compliant (errors during construction)
- ✅ Minimal changes (no parser modifications needed)
- ✅ Elegant (single opcode handles all cases)
- ✅ Uniform (all generators behave consistently)
- ✅ Low risk (isolated to generator path)

**Expected Impact**: +200 Test262 tests passing

## Problem Statement

Generator functions with destructuring parameters don't throw TypeError when called with null/undefined until `.next()` is called, violating ECMAScript semantics.

### Current Behavior (WRONG)
```typescript
function* f({x}) {}
f(null); // Returns generator object (no error)
f(null).next(); // TypeError on .next() call
```

### Expected Behavior (ECMAScript Compliant)
```typescript
function* f({x}) {}
f(null); // TypeError immediately
```

## Root Cause Analysis

### Parser Issue
`transformFunctionWithDestructuring` (parser.go:2837) converts destructuring parameters into prologue statements placed in function BODY:

```go
// For function* f({x}) {}, transforms to:
function* f(__temp0) {
  const {x} = __temp0;  // <-- This is in the BODY (prologue)
  // ... rest of function
}
```

### Compiler Issue
When compiling generator function calls ([compile_expression.go:1633](/Users/nooga/lab/paserati/pkg/compiler/compile_expression.go#L1633)):
- Emits `OpCreateGenerator` immediately with arguments
- Generator body (including prologue) doesn't execute until `.next()`

### VM Behavior
`OpCreateGenerator` ([vm.go:6493](/Users/nooga/lab/paserati/pkg/vm/vm.go#L6493)):
- Creates generator object
- Stores arguments in `genObj.Args`
- Returns generator object WITHOUT executing body
- Body (with parameter destructuring) only runs on first `.next()` call

## ECMAScript Specification Requirements

**ECMAScript 15.5.2 EvaluateGeneratorBody**:
1. FunctionDeclarationInstantiation (parameter binding/destructuring) happens FIRST
2. Generator object creation happens AFTER parameter validation
3. This means parameter errors must throw BEFORE generator object is returned

## V8 Implementation Analysis

From research of V8 source code:

### Bytecode Generation
- `BuildGeneratorPrologue()` - handles generator state dispatch for RESUME
- `BuildGeneratorObjectVariableInitialization()` - creates generator object
- Parameter binding happens in normal function initialization order
- `SuspendGenerator` and `ResumeGenerator` bytecodes save/restore state

### Key Insight
V8 emits parameter binding bytecode BEFORE suspend/resume machinery. Parameters are validated during function initialization, not lazily in body.

## Solution Approach: Synchronous Prologue Execution with Init Yield

### Core Idea

Keep destructuring code with null checks inside the generator function body (as a prologue), but execute these instructions **synchronously during `OpCreateGenerator`** up to the beginning of user code. Use a special **"init yield"** instruction (`OpInitYield`) to mark where initialization ends and user code begins.

### Architecture

```typescript
// Source code:
function* f({x}) {
  console.log(x);
}

// After parser transformation (existing logic, no changes needed):
function* f(__destructured_param_0) {
  const {x} = __destructured_param_0;  // Destructuring with null check (prologue)
  console.log(x);                      // User code
}

// Compiled bytecode (NEW):
function* f(__destructured_param_0) {
  const {x} = __destructured_param_0;  // Destructuring with null check
  OpInitYield                          // <-- NEW: Marks end of initialization
  console.log(x);                      // User code starts here
}
```

### Execution Flow

#### 1. During `OpCreateGenerator` (Call-Time)

```go
case OpCreateGenerator:
    // ... existing code to create generator object ...

    // NEW: Start executing generator function synchronously
    status, _ := vm.callFunction(funcVal, args, genObj)

    // If error during prologue (e.g., destructuring null) → throw immediately
    if status != InterpretOK {
        return status, Undefined  // Generator object never returned
    }

    // Execution stopped at OpInitYield with saved state
    // Generator now in GeneratorSuspendedStart state
    registers[destReg] = genVal
```

#### 2. During `OpInitYield` (First Encounter)

```go
case OpInitYield:
    // Check if this is being hit during generator construction
    if frame.generatorObj.State == GeneratorStart {
        // Save current state (all destructured values in registers)
        frame.generatorObj.State = GeneratorSuspendedStart
        frame.generatorObj.Frame = &GeneratorFrame{
            pc:        ip + 1,  // Resume after OpInitYield
            registers: copy(registers),
            // ... other state ...
        }
        // Return control to OpCreateGenerator
        return InterpretOK, Undefined
    }

    // If hit during .next() (shouldn't happen, but be defensive)
    // This is a no-op - just continue to next instruction
    ip++
```

#### 3. During `.next()` Call (Execution-Time)

Generator resumes from saved state (after `OpInitYield`), with all destructured values already in registers. User code executes normally.

### Benefits

✅ **Correctness**: Parameter errors throw during construction (before generator exists)
✅ **Separation of Concerns**: Destructuring logic stays with function definition
✅ **Simplicity**: Compiler reuses existing `transformFunctionWithDestructuring` logic
✅ **Single Opcode**: `OpInitYield` handles all cases elegantly
✅ **Backwards Compatible**: Functions without destructuring get `OpInitYield` at start (no-op if no prologue)
✅ **No Caller Changes**: Caller code (`OpCreateGenerator` emission) unchanged
✅ **Matches V8 Architecture**: Parameter binding before first suspend point

### Comparison to V8

This approach mirrors V8's architecture where:
- Parameter binding happens during function initialization (before first suspend point)
- Generator state machine starts after parameters are validated
- `SuspendGenerator` bytecode marks suspension points in user code

Our `OpInitYield` serves as an implicit first suspend point that only triggers during construction.

## Recommended Implementation

### Phase 1: Add OpInitYield Bytecode

**File**: `pkg/vm/bytecode.go`

```go
// Add new opcode after OpCreateGenerator
OpInitYield OpCode = 101 // No operands: Mark end of generator initialization prologue
```

**File**: `pkg/vm/bytecode.go` (String() method)

```go
case OpInitYield:
    return "OpInitYield"
```

**File**: `pkg/vm/bytecode.go` (disassemble method)

```go
case OpInitYield:
    return c.simpleInstruction(builder, "OpInitYield", offset)
```

### Phase 2: Compiler Modifications

**File**: `pkg/compiler/compile_literal.go`

Find the function that compiles generator function bodies and emit `OpInitYield` after prologue:

```go
func (c *Compiler) compileFunctionBody(fn *parser.FunctionLiteral) {
    // ... existing parameter setup code ...

    if fn.IsGenerator || fn.IsAsyncGenerator {
        // After prologue statements (destructuring), emit OpInitYield
        prologueEndLine := // determine line number of first user statement
        c.emitOpCode(vm.OpInitYield, prologueEndLine)
    }

    // ... compile rest of body ...
}
```

**Key Challenge**: Need to detect where prologue ends and user code begins. Options:
1. Count prologue statements added by `transformFunctionWithDestructuring` (track in AST)
2. Add metadata to `FunctionLiteral` indicating prologue length
3. Emit `OpInitYield` immediately before first user statement

### Phase 3: VM Implementation

**File**: `pkg/vm/vm.go`

#### Modify `OpCreateGenerator`:

```go
case OpCreateGenerator:
    destReg := code[ip]
    funcReg := code[ip+1]
    argCount := int(code[ip+2])

    funcVal := registers[funcReg]
    if !funcVal.IsFunction() && !funcVal.IsClosure() {
        status := vm.runtimeError("OpCreateGenerator: not a function")
        return status, Undefined
    }

    // Create generator object
    genVal := NewGenerator(funcVal)
    genObj := genVal.AsGenerator()

    // Store arguments
    if argCount > 0 {
        genObj.Args = make([]Value, argCount)
        for i := 0; i < argCount; i++ {
            argReg := int(funcReg) + 1 + i
            if argReg < len(registers) {
                genObj.Args[i] = registers[argReg]
            } else {
                genObj.Args[i] = Undefined
            }
        }
    }

    // NEW: Execute generator prologue synchronously until OpInitYield
    genObj.State = GeneratorStart
    status, _ := vm.executeGeneratorPrologue(genObj, funcVal)
    if status != InterpretOK {
        // Prologue failed (e.g., destructuring null) - don't return generator
        return status, Undefined
    }

    // Generator now in GeneratorSuspendedStart state with saved registers
    registers[destReg] = genVal
    ip += 3
```

#### Add `OpInitYield` handler:

```go
case OpInitYield:
    // This opcode marks the end of generator initialization prologue
    if frame.generatorObj == nil {
        // Not in a generator (shouldn't happen)
        ip++
        break
    }

    genObj := frame.generatorObj

    // Check if this is first encounter (during construction)
    if genObj.State == GeneratorStart {
        // Save current state - prologue has computed all destructured values
        genObj.State = GeneratorSuspendedStart
        genObj.Frame = &GeneratorFrame{
            pc:        ip + 1, // Resume after OpInitYield
            registers: make([]Value, len(registers)),
            thisValue: frame.thisValue,
        }
        copy(genObj.Frame.registers, registers)

        // Return control to OpCreateGenerator
        return InterpretOK, Undefined
    }

    // If hit during .next() execution (shouldn't happen, but be defensive)
    // Just continue - this is a no-op
    ip++
```

#### Add helper function:

```go
func (vm *VM) executeGeneratorPrologue(genObj *Generator, funcVal Value) (InterpreterStatus, Value) {
    // Create frame for generator execution
    // Call the generator function (will stop at OpInitYield)
    // Similar to callFunction but for generator initialization
    // ...
}
```

### Phase 4: Parser Metadata (Optional but Recommended)

**File**: `pkg/parser/ast.go`

Add field to track prologue length:

```go
type FunctionLiteral struct {
    // ... existing fields ...
    PrologueLength int // Number of prologue statements (for OpInitYield placement)
}
```

**File**: `pkg/parser/parser.go`

Update `transformFunctionWithDestructuring` to set `PrologueLength`:

```go
func (p *Parser) transformFunctionWithDestructuring(fn *FunctionLiteral) *FunctionLiteral {
    // ... existing transformation logic ...

    // Track how many statements we added as prologue
    fn.PrologueLength = len(newStatements) - len(fn.Body.Statements)

    // ... rest of function ...
}
```

### Phase 5: Testing

**Test Files**:
- `tests/scripts/gen_param_dstr_null.ts` - Existing test should pass
- `tests/scripts/gen_param_dstr_undefined.ts` - Existing test should pass
- Add complex nesting tests
- Run Test262 generators suite - expect +200 tests passing

## Impact Analysis

### Test262 Expected Improvements

Based on failure analysis:
- Generators: ~52 tests currently failing for destructuring null/undefined
- Async generators: ~150+ tests with similar issues
- **Total expected improvement**: +200 tests

### Affected Suites
- `language/expressions/generators` - parameter destructuring tests
- `language/expressions/async-generators` - async parameter destructuring
- `language/statements/class` - generator methods with destructuring

## Implementation Timeline

1. **Research and Planning** (COMPLETED)
   - V8 implementation study
   - ECMAScript spec review
   - Current codebase analysis
   - Solution design and documentation

2. **Phase 1: Add OpInitYield Bytecode** (~30 minutes)
   - Add opcode constant
   - Add to String() method
   - Add disassembly support

3. **Phase 2: Parser Metadata** (~1 hour)
   - Add `PrologueLength` field to `FunctionLiteral`
   - Update `transformFunctionWithDestructuring` to track prologue
   - Test that metadata is correctly set

4. **Phase 3: Compiler Changes** (~2 hours)
   - Find where generator function bodies are compiled
   - Emit `OpInitYield` after prologue statements
   - Handle edge cases (no prologue, async generators)
   - Verify bytecode output with `-bytecode` flag

5. **Phase 4: VM Implementation** (~3-4 hours)
   - Implement `OpInitYield` handler
   - Modify `OpCreateGenerator` to execute prologue
   - Implement `executeGeneratorPrologue` helper
   - Handle state transitions correctly
   - Test with smoke tests

6. **Phase 5: Integration Testing** (~1 hour)
   - Run existing smoke tests
   - Run Test262 generators suite
   - Verify +200 test improvement
   - Check for regressions in non-generator code

7. **Phase 6: Cleanup** (~30 minutes)
   - Update documentation
   - Generate new baseline
   - Commit changes

**Total estimated time**: 7-9 hours of focused work

## Implementation Details and Edge Cases

### Generator State Transitions

Current states (from `pkg/vm/generator.go`):
- `GeneratorStart` - Generator created but not yet executed
- `GeneratorSuspendedStart` - Generator suspended before first yield (NEW usage)
- `GeneratorSuspendedYield` - Generator suspended at a yield point
- `GeneratorExecuting` - Generator currently running
- `GeneratorCompleted` - Generator finished execution

**State Flow**:
```
OpCreateGenerator → GeneratorStart
OpInitYield (first time) → GeneratorSuspendedStart (with saved state)
.next() call → GeneratorExecuting → OpYield → GeneratorSuspendedYield
.next() call → GeneratorExecuting → return → GeneratorCompleted
```

### Edge Cases to Handle

1. **Generator without destructuring**:
   - Still emit `OpInitYield` at function start
   - OpInitYield becomes no-op (just saves initial state)
   - Slight performance overhead acceptable for correctness

2. **Nested destructuring**:
   - Already handled by existing `transformFunctionWithDestructuring`
   - Prologue contains all null checks
   - Just need to execute it synchronously

3. **Default parameter values with destructuring**:
   - Already handled by parser transformation
   - Default evaluation happens in prologue
   - Executes synchronously during construction

4. **Rest parameters with destructuring**:
   - Already handled by parser transformation
   - Rest collection in prologue
   - Executes synchronously during construction

5. **Async generators**:
   - Same solution applies
   - OpInitYield marks end of initialization
   - Async behavior starts after prologue

6. **Generator methods in classes**:
   - Already handled by existing generator detection
   - `this` binding preserved in GeneratorFrame
   - No special handling needed

### Performance Considerations

**Overhead**:
- Every generator function gets `OpInitYield` (1 byte)
- Generators without destructuring pay small cost of state save at start
- Acceptable tradeoff for correctness and simplicity

**Benefits**:
- No runtime checks for "has destructuring" needed
- Uniform code path for all generators
- Simple VM implementation

### Debugging Support

With `-bytecode` flag, bytecode will show:
```
0000  OpGetParameter      r0 <- param[0]     ; __destructured_param_0
0003  OpDestructureObject r1 <- r0           ; {x} = __destructured_param_0
0006  OpInitYield                            ; <-- End of prologue
0007  OpLoadConstant      r2 <- "console"
...
```

This makes it clear where initialization ends and user code begins.

## Questions Answered

1. **Default Parameter Values**: How do default values interact with destructuring in generators?
   - ✅ **Answer**: Default values are already in the prologue (handled by parser transformation). They execute synchronously during construction, before OpInitYield.

2. **Rest Parameters**: How do rest parameters with destructuring work?
   - ✅ **Answer**: Rest collection already happens in prologue (parser transformation). Executes synchronously during construction.

3. **Type Annotations**: Do TypeScript type annotations on destructuring parameters need special handling?
   - ✅ **Answer**: No. Type checker already validates types; runtime only needs structural correctness (null/undefined checks).

4. **Async Generators**: Does the same fix apply to async generators?
   - ✅ **Answer**: Yes. Same approach - OpInitYield marks end of initialization. Async behavior starts after prologue completes.

5. **What about generators without destructuring?**
   - ✅ **Answer**: They still get OpInitYield at start (slight overhead). This simplifies implementation and provides uniform behavior.

6. **How does this handle deeply nested destructuring?**
   - ✅ **Answer**: Parser already transforms nested patterns into sequential prologue statements. All execute synchronously.

7. **What if prologue throws for non-destructuring reasons (e.g., TDZ)?**
   - ✅ **Answer**: Any prologue error prevents generator creation. This is correct - generator should not exist if initialization fails.

## Critical Implementation Notes

### Must-Have Features

1. **Generator State Management**:
   - Generator must start in `GeneratorStart` state
   - OpInitYield transitions to `GeneratorSuspendedStart` only on first encounter
   - Saved state must include all registers (destructured values)

2. **Error Propagation**:
   - Any error during prologue execution must bubble up through OpCreateGenerator
   - Generator object must NOT be returned if prologue fails
   - Error messages must be clear (e.g., "Cannot destructure 'undefined'")

3. **Frame Management**:
   - Generator frame must be properly initialized during prologue execution
   - `frame.generatorObj` must be set before executing prologue
   - Resume must continue from exact point after OpInitYield

4. **Uniform Behavior**:
   - ALL generators get OpInitYield (even without destructuring)
   - Simplifies implementation and testing
   - Small performance cost acceptable

### Testing Strategy

**Unit Tests** (smoke tests):
```typescript
// tests/scripts/gen_param_dstr_null.ts
function* f({x}) { return x; }
try { f(null).next(); } catch(e) { console.log("caught"); }
// expect: caught

// tests/scripts/gen_param_dstr_undefined.ts
function* f({x}) { return x; }
try { f(undefined).next(); } catch(e) { console.log("caught"); }
// expect: caught

// tests/scripts/gen_param_dstr_valid.ts
function* f({x}) { return x; }
console.log(f({x: 42}).next().value);
// expect: 42

// tests/scripts/gen_no_dstr.ts
function* f(x) { return x; }
console.log(f(42).next().value);
// expect: 42
```

**Integration Tests**:
- Run Test262 generators suite
- Target: +200 tests passing
- Zero regressions in existing tests

### Migration Path

This solution requires:
1. ✅ No parser changes (existing transformation works)
2. ✅ Minimal compiler changes (emit one opcode)
3. ✅ Moderate VM changes (new opcode + prologue execution)
4. ✅ No changes to built-ins or type system

**Risk Level**: LOW
- Isolated to generator execution path
- Does not affect regular functions
- Can be tested incrementally

## Related Issues

- Issue fixed in commit: +49 tests for regular function parameter destructuring
- Root cause: Same transformation issue, but affects generators more severely
- Previous fix: Added `emitDestructuringNullCheck` helper (compile_assignment_helpers.go)

## References

- ECMAScript Spec: https://tc39.es/ecma262/#sec-runtime-semantics-evaluategeneratorbody
- V8 Source: `src/interpreter/bytecode-generator.cc` (BuildGeneratorPrologue)
- Test262: `test/language/expressions/generators/` (parameter destructuring tests)
- Previous fix: commit message "+49 tests - Add destructuring null/undefined checks"

# Generator Parameter Destructuring - Implementation Analysis

**Date**: 2025-10-29
**Status**: Planning Phase - Ready for Implementation

## Executive Summary

After thorough code audit, the OpInitYield approach is **feasible and elegant**. The existing generator infrastructure supports synchronous prologue execution during construction. This document provides detailed implementation guidance based on actual codebase analysis.

## Current Architecture Analysis

### 1. Parser - Destructuring Transformation

**Location**: `pkg/parser/parser.go:2835-2900`

**Current Behavior**:
```go
func (p *Parser) transformFunctionWithDestructuring(fn *FunctionLiteral) *FunctionLiteral {
    // For ALL functions (including generators), transforms:
    //   function* f({x}) { body }
    // Into:
    //   function* f(__destructured_param_0) {
    //     const {x} = __destructured_param_0;  // ← Prologue statements
    //     body                                 // ← User code
    //   }
}
```

**Key Insight**: Parser already creates prologue statements for generators. No changes needed here.

**Metadata Available**:
- `fn.Parameters[i].IsDestructuring` - flags destructuring params
- Transformed params have synthetic names like `__destructured_param_N`
- Prologue statements are prepended to `fn.Body.Statements`

**Problem**: No way to know WHERE prologue ends and user code begins during compilation.

### 2. Compiler - Function Body Compilation

**Location**: `pkg/compiler/compile_literal.go:831-1120`

**Current Flow**:
```go
func (c *Compiler) compileFunctionLiteral(node *parser.FunctionLiteral, nameHint string) {
    // 1. Create function compiler
    // 2. Define parameters (skip IsDestructuring params)
    // 3. Handle default parameters
    // 4. Handle rest parameters
    // 5. Compile body (includes prologue from parser transformation)
    // 6. Emit final return
    // 7. Create Function object with isGenerator flag
}
```

**Generator-Specific Flags**:
- `functionCompiler.isGenerator = node.IsGenerator` (line 851)
- `funcValue = vm.NewFunction(..., node.IsGenerator, ...)` (line 1101)

**Where to Emit OpInitYield**:
- After compiling body statements BUT need to distinguish prologue from user code
- Options:
  1. Track prologue statement count in AST
  2. Emit OpInitYield at START of every generator (before any body)
  3. Parser marks prologue statements with metadata

### 3. Compiler - Generator Call Expression

**Location**: `pkg/compiler/compile_expression.go:1630-1646`

**Current Flow**:
```go
if isGeneratorCall {
    // Emit OpCreateGenerator
    c.emitOpCode(vm.OpCreateGenerator, node.Token.Line)
    c.emitByte(byte(hint))           // Destination register
    c.emitByte(byte(funcReg))        // Function register
    c.emitByte(byte(actualArgCount)) // Argument count
}
```

**Arguments Already in Registers**:
- Arguments compiled into `funcReg+1`, `funcReg+2`, etc.
- OpCreateGenerator reads them and stores in `genObj.Args`

**No Changes Needed Here**: OpCreateGenerator will execute prologue synchronously.

### 4. VM - OpCreateGenerator Implementation

**Location**: `pkg/vm/vm.go:6493-6527`

**Current Behavior**:
```go
case OpCreateGenerator:
    // 1. Get function from register
    // 2. Create generator object
    // 3. Store arguments in genObj.Args
    // 4. Put generator in destination register
    // 5. Continue (generator NOT executed)
```

**State After OpCreateGenerator**:
- Generator state: Not set explicitly (defaults to zero value = GeneratorSuspendedStart)
- Generator has: Function value, Args array, This value
- Generator does NOT have: Frame (nil until first execution)

**Required Changes**:
- After creating generator, call `vm.startGeneratorPrologue(genObj)`
- This executes until OpInitYield
- If error, don't return generator
- If success, generator in `GeneratorSuspendedStart` with saved Frame

### 5. VM - Generator Execution Flow

**Location**: `pkg/vm/vm.go:7735-7870`

**Current Flow**:

```
.next() called → executeGenerator() → checks state:
  - GeneratorSuspendedStart → startGenerator()
  - GeneratorSuspendedYield → resumeGenerator()
  - GeneratorCompleted → return { done: true }
```

**startGenerator() Details** (line 7770):
1. Creates sentinel frame (for stack isolation)
2. Calls `prepareCallWithGeneratorMode()` to set up function frame
3. Sets `frame.generatorObj = genObj`
4. Sets `genObj.State = GeneratorExecuting`
5. Runs VM until yield/return (vm.run())
6. Cleans up frames based on generator state

**resumeGenerator() Details** (line 7873):
1. Restores saved frame from `genObj.Frame`
2. Creates sentinel frame
3. Restores registers from saved state
4. Restores PC to resume point
5. Stores sent value in `outputReg`
6. Runs VM until yield/return

**Key Insight**: startGenerator() already has the infrastructure we need!

### 6. VM - OpYield Implementation

**Location**: `pkg/vm/vm.go:6529-6578`

**Current Behavior**:
```go
case OpYield:
    // 1. Get yielded value from register
    // 2. Save generator state
    // 3. Create/update genObj.Frame with current state
    // 4. Copy registers to saved frame
    // 5. Return { value, done: false }
```

**State Management**:
- Sets `genObj.State = GeneratorSuspendedYield`
- Saves PC (for resume point)
- Saves all registers
- Returns from vm.run() to caller

**OpInitYield Will Be Similar**: Save state, but transition to GeneratorSuspendedStart instead.

### 7. Generator State Machine

**Location**: `pkg/vm/value.go:161-168`

**States**:
```go
const (
    GeneratorSuspendedStart  // Initial - not yet executed (NEW USAGE: after prologue)
    GeneratorSuspendedYield  // Suspended at yield
    GeneratorExecuting       // Currently running
    GeneratorCompleted       // Finished
)
```

**State Transitions**:
```
OpCreateGenerator → (no state set, defaults to 0 = GeneratorSuspendedStart)
   ↓ (NEW: execute prologue)
OpInitYield → GeneratorSuspendedStart (with saved frame)
   ↓ .next()
startGenerator() → GeneratorExecuting
   ↓ hit OpYield
OpYield → GeneratorSuspendedYield
   ↓ .next()
resumeGenerator() → GeneratorExecuting
   ↓ hit OpReturn
OpReturn → GeneratorCompleted
```

**NEW State Flow With OpInitYield**:
```
OpCreateGenerator creates genObj
   ↓ (NEW: call startGeneratorPrologue)
Executes function with GeneratorStart state (new state needed!)
   ↓ compiles prologue, destructures params
OpInitYield (checks if state == GeneratorStart)
   ↓ YES: save frame, set GeneratorSuspendedStart, return to OpCreateGenerator
OpCreateGenerator returns generator
   ↓ .next()
executeGenerator sees GeneratorSuspendedStart
   ↓
startGenerator() resumes from saved PC (after OpInitYield)
```

**Problem Identified**: Need NEW state `GeneratorStart` to distinguish:
- Fresh generator (needs prologue execution)
- Generator with prologue done (ready for user code)

## Implementation Plan - Detailed

### Phase 1: Add OpInitYield Bytecode ✓

**File**: `pkg/vm/bytecode.go`

```go
// After OpYieldDelegated (line 203)
OpInitYield OpCode = 101 // No operands: Mark end of generator initialization
```

**Disassembly Support**:
```go
// In String() method
case OpInitYield:
    return "OpInitYield"

// In disassemble method
case OpInitYield:
    return c.simpleInstruction(builder, "OpInitYield", offset)
```

### Phase 2: Add GeneratorStart State

**File**: `pkg/vm/value.go`

```go
const (
    GeneratorStart          GeneratorState = iota // Just created, prologue not executed
    GeneratorSuspendedStart                       // Prologue executed, ready for first .next()
    GeneratorSuspendedYield                       // Suspended at yield
    GeneratorExecuting                            // Currently running
    GeneratorCompleted                            // Finished
)
```

**Rationale**: Distinguish "just created" from "prologue executed".

### Phase 3: Track Prologue Length in AST

**File**: `pkg/parser/ast.go`

```go
type FunctionLiteral struct {
    // ... existing fields ...
    PrologueStmtCount int // Number of prologue statements added by transformation
}
```

**File**: `pkg/parser/parser.go` (transformFunctionWithDestructuring)

```go
// After transformation
originalStmtCount := len(fn.Body.Statements)
fn.Body.Statements = append(newStatements, fn.Body.Statements...)
fn.PrologueStmtCount = len(fn.Body.Statements) - originalStmtCount
```

### Phase 4: Compiler Emits OpInitYield

**File**: `pkg/compiler/compile_literal.go`

**Location**: In `compileFunctionLiteral`, after compiling body but before finalizing

```go
// Around line 1038, BEFORE compiling body
if node.IsGenerator || node.IsAsyncGenerator {
    // For generators, emit OpInitYield after prologue statements
    // We'll emit it at the START to handle both cases:
    // - Generators with destructuring: prologue executes, then OpInitYield
    // - Generators without destructuring: OpInitYield immediately (no-op)

    // Option 1: Emit at start of function (SIMPLEST)
    functionCompiler.emitOpCode(vm.OpInitYield, node.Token.Line)

    // Option 2: Emit after prologue statements (MORE PRECISE)
    // Requires compiling prologue statements separately
}

// Then compile body as normal
bodyReg := functionCompiler.regAlloc.Alloc()
functionCompiler.isCompilingFunctionBody = true
_, err := functionCompiler.compileNode(node.Body, bodyReg)
```

**Decision Point**: Where to emit OpInitYield?

**Option A - Emit at START** (RECOMMENDED):
```go
// At line ~1038, before compiling body
if node.IsGenerator || node.IsAsyncGenerator {
    functionCompiler.emitOpCode(vm.OpInitYield, node.Token.Line)
}
```

**Pros**:
- Simple - no AST changes needed
- Works for ALL generators uniformly
- Prologue execution happens before OpInitYield is even hit

**Cons**:
- Generators without destructuring pay small cost of no-op

**Option B - Emit after N statements**:
```go
// Requires compiling statements one-by-one
prologueCount := node.PrologueStmtCount
for i, stmt := range node.Body.Statements {
    compileStatement(stmt)
    if i == prologueCount - 1 && (node.IsGenerator || node.IsAsyncGenerator) {
        functionCompiler.emitOpCode(vm.OpInitYield, stmt.Token.Line)
    }
}
```

**Pros**:
- More precise - only affects generators with destructuring
- Clear separation in bytecode

**Cons**:
- Requires AST changes
- More complex logic
- Need to handle BlockStatement compilation differently

**RECOMMENDATION**: Use Option A (emit at start). It's simpler and the performance cost is negligible.

**REVISED APPROACH**: Actually, we need to emit AFTER prologue, not before. Here's why:

The parser transforms:
```typescript
function* f({x}) { console.log(x); }
```

Into:
```typescript
function* f(__destructured_param_0) {
  const {x} = __destructured_param_0;  // ← Prologue
  console.log(x);                      // ← User code
}
```

The prologue is INSIDE the BlockStatement. We need OpInitYield AFTER `const {x}` but BEFORE `console.log(x)`.

**SOLUTION**: Emit OpInitYield at the START of the function, but BEFORE any prologue code. Then prologue executes, hits OpInitYield, saves state.

Wait, that doesn't work either. Let me reconsider...

**ACTUAL FLOW**:
1. Parser creates prologue statements in function body
2. Compiler compiles entire body (including prologue)
3. We need OpInitYield AFTER prologue bytecode

**CORRECT APPROACH**:
- Manually compile prologue statements separately
- Emit OpInitYield
- Compile rest of body

OR:

- Keep simple: Emit OpInitYield at very start
- Prologue doesn't execute until OpCreateGenerator calls the function
- OpInitYield is hit immediately → saves state
- Problem: Prologue hasn't run yet!

**REALIZATION**: We need to emit OpInitYield AFTER the prologue is COMPILED into bytecode.

**FINAL APPROACH**:

```go
// In compileFunctionLiteral, around line 1038

// Split body compilation for generators
if node.IsGenerator || node.IsAsyncGenerator {
    // Compile prologue statements
    prologueCount := node.PrologueStmtCount
    if prologueCount > 0 {
        for i := 0; i < prologueCount && i < len(node.Body.Statements); i++ {
            stmt := node.Body.Statements[i]
            stmtReg := functionCompiler.regAlloc.Alloc()
            _, err := functionCompiler.compileNode(stmt, stmtReg)
            functionCompiler.regAlloc.Free(stmtReg)
            if err != nil {
                functionCompiler.errors = append(functionCompiler.errors, err)
            }
        }
    }

    // Emit OpInitYield after prologue
    functionCompiler.emitOpCode(vm.OpInitYield, node.Token.Line)

    // Compile remaining statements (user code)
    for i := prologueCount; i < len(node.Body.Statements); i++ {
        stmt := node.Body.Statements[i]
        stmtReg := functionCompiler.regAlloc.Alloc()
        _, err := functionCompiler.compileNode(stmt, stmtReg)
        functionCompiler.regAlloc.Free(stmtReg)
        if err != nil {
            functionCompiler.errors = append(functionCompiler.errors, err)
        }
    }
} else {
    // Regular function - compile body as normal
    bodyReg := functionCompiler.regAlloc.Alloc()
    functionCompiler.isCompilingFunctionBody = true
    _, err := functionCompiler.compileNode(node.Body, bodyReg)
    functionCompiler.isCompilingFunctionBody = false
    functionCompiler.regAlloc.Free(bodyReg)
    if err != nil {
        functionCompiler.errors = append(functionCompiler.errors, err)
    }
}
```

**WAIT - Problem**: `compileNode(node.Body, bodyReg)` compiles the entire BlockStatement. We can't easily split it.

**BETTER APPROACH**: Emit OpInitYield at START of function (line 0), then on first execution:
1. OpInitYield is hit FIRST (before any prologue)
2. Check if this is initial call (state == GeneratorStart)
3. If yes, SKIP the yield (just set flag saying "skip next time")
4. Continue executing prologue
5. When OpInitYield is hit AGAIN on resume, it's a no-op

NO - that's too complex and wrong.

**CORRECT UNDERSTANDING**:

OpInitYield should be emitted AFTER prologue bytecode. The way to do this:

**Compile body statement-by-statement** for generators:

```go
if node.IsGenerator {
    // Compile each statement individually
    for i, stmt := range node.Body.Statements {
        // Compile statement
        compileStatement(stmt)

        // After prologue, emit OpInitYield
        if i == node.PrologueStmtCount - 1 {
            functionCompiler.emitOpCode(vm.OpInitYield, stmt.Token.Line)
        }
    }
}
```

But BlockStatement is a single node. Let me check how it's compiled...

Actually, looking at the code, `node.Body` is a `*BlockStatement`. Inside BlockStatement are `Statements []Statement`.

**SOLUTION**: Don't use `compileNode(node.Body, ...)`. Instead, manually iterate statements:

```go
// Around line 1038
if node.IsGenerator || node.IsAsyncGenerator {
    // Manually compile statements to inject OpInitYield
    prologueCount := node.PrologueStmtCount
    for i, stmt := range node.Body.Statements {
        stmtReg := functionCompiler.regAlloc.Alloc()
        _, err := functionCompiler.compileNode(stmt, stmtReg)
        functionCompiler.regAlloc.Free(stmtReg)
        if err != nil {
            functionCompiler.errors = append(functionCompiler.errors, err)
        }

        // After last prologue statement, emit OpInitYield
        if i == prologueCount - 1 {
            functionCompiler.emitOpCode(vm.OpInitYield, node.Body.Token.Line)
        }
    }
} else {
    // Regular functions - compile body as before
    bodyReg := functionCompiler.regAlloc.Alloc()
    functionCompiler.isCompilingFunctionBody = true
    _, err := functionCompiler.compileNode(node.Body, bodyReg)
    functionCompiler.isCompilingFunctionBody = false
    functionCompiler.regAlloc.Free(bodyReg)
    if err != nil {
        functionCompiler.errors = append(functionCompiler.errors, err)
    }
}
```

This is clean and precise!

### Phase 5: VM - Implement OpInitYield

**File**: `pkg/vm/vm.go`

**Location**: After OpYieldDelegated case (around line 6632)

```go
case OpInitYield:
    // This opcode marks the end of generator initialization prologue
    // On first execution (during OpCreateGenerator), it saves state
    // On resume (during .next()), it's a no-op

    if frame.generatorObj == nil {
        // Not in generator context - shouldn't happen, but be defensive
        ip++
        break
    }

    genObj := frame.generatorObj

    // Check if this is initialization (prologue just executed)
    if genObj.State == GeneratorStart {
        // Save current execution state
        genObj.State = GeneratorSuspendedStart
        genObj.Frame = &GeneratorFrame{
            pc:        ip + 1,  // Resume AFTER OpInitYield
            registers: make([]Value, len(registers)),
            thisValue: frame.thisValue,
            suspendPC: ip,
            outputReg: 0,  // Not used for init yield
        }
        copy(genObj.Frame.registers, registers)

        // Return control to OpCreateGenerator
        // This will cause vm.run() to return
        return InterpretOK, Undefined
    }

    // If state is not GeneratorStart, this is a resume from .next()
    // OpInitYield is a no-op on resume - just continue
    ip++
```

**Key Details**:
- Check `genObj.State == GeneratorStart` to detect initialization
- Save frame with PC = ip + 1 (resume after OpInitYield)
- Return `InterpretOK` to exit vm.run() cleanly
- On resume, it's a no-op (just increment IP)

### Phase 6: VM - Modify OpCreateGenerator

**File**: `pkg/vm/vm.go`

**Location**: OpCreateGenerator case (line 6493)

```go
case OpCreateGenerator:
    destReg := code[ip]
    funcReg := code[ip+1]
    argCount := int(code[ip+2])

    // Get the generator function
    funcVal := registers[funcReg]
    if !funcVal.IsFunction() && !funcVal.IsClosure() {
        status := vm.runtimeError("OpCreateGenerator: not a function")
        return status, Undefined
    }

    // Create generator object
    genVal := NewGenerator(funcVal)
    genObj := genVal.AsGenerator()

    // Store arguments for generator execution
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

    // NEW: Execute generator prologue synchronously
    genObj.State = GeneratorStart  // Set initial state
    status, _ := vm.startGeneratorPrologue(genObj)
    if status != InterpretOK {
        // Prologue failed (e.g., destructuring null)
        // Don't return generator - propagate error
        return status, Undefined
    }

    // Generator now in GeneratorSuspendedStart with saved state
    registers[destReg] = genVal
    ip += 3
```

**New Function Needed**: `vm.startGeneratorPrologue(genObj *GeneratorObject)`

### Phase 7: VM - Implement startGeneratorPrologue

**File**: `pkg/vm/vm.go`

**Location**: Add new function near startGenerator (around line 7770)

```go
// startGeneratorPrologue executes the generator prologue (parameter destructuring)
// synchronously during OpCreateGenerator. Stops at OpInitYield.
func (vm *VM) startGeneratorPrologue(genObj *GeneratorObject) (InterpreterStatus, Value) {
    funcVal := genObj.Function

    // Set up minimal caller context (no sentinel frame needed)
    // We'll execute directly and return when OpInitYield is hit

    // Prepare arguments
    args := genObj.Args
    if args == nil {
        args = []Value{}
    }

    // Use stored 'this' value
    thisValue := genObj.This
    if thisValue.Type() == 0 {
        thisValue = Undefined
    }

    // Create a temporary caller context
    // We need registers for the function call but no actual sentinel frame
    // Use a simplified version of prepareCall

    var funcObj *FunctionObject
    var closureObj *ClosureObject

    if funcVal.Type() == TypeFunction {
        funcObj = funcVal.AsFunction()
    } else if funcVal.Type() == TypeClosure {
        closureObj = funcVal.AsClosure()
        funcObj = closureObj.Fn
    } else {
        return InterpretRuntimeError, Undefined
    }

    // Check stack space
    if vm.frameCount >= MaxFrames {
        return InterpretRuntimeError, Undefined
    }

    // Allocate registers
    regSize := funcObj.RegisterSize
    if vm.nextRegSlot + regSize > len(vm.registerStack) {
        return InterpretRuntimeError, Undefined
    }

    // Set up frame
    frame := &vm.frames[vm.frameCount]
    frame.registers = vm.registerStack[vm.nextRegSlot : vm.nextRegSlot+regSize]
    frame.ip = 0  // Start from beginning
    frame.chunk = funcObj.Chunk
    frame.thisValue = thisValue
    frame.generatorObj = genObj  // Link frame to generator
    frame.isDirectCall = false
    frame.isSentinelFrame = false

    // Set closure if applicable
    if closureObj != nil {
        frame.closure = closureObj
    } else {
        frame.closure = nil
    }

    // Copy arguments to registers
    arity := funcObj.Arity
    for i := 0; i < arity && i < len(args); i++ {
        frame.registers[i] = args[i]
    }
    // Fill missing args with undefined
    for i := len(args); i < arity; i++ {
        frame.registers[i] = Undefined
    }

    // Advance frame count and register slot
    vm.frameCount++
    vm.nextRegSlot += regSize

    // Set generator state
    genObj.State = GeneratorStart

    // Execute until OpInitYield
    status, result := vm.run()

    // Clean up frame
    vm.frameCount--
    vm.nextRegSlot -= regSize

    // Check result
    if status != InterpretOK {
        // Prologue threw an error
        genObj.State = GeneratorCompleted
        genObj.Done = true
        return status, result
    }

    // Prologue executed successfully
    // Generator now in GeneratorSuspendedStart with saved frame
    return InterpretOK, Undefined
}
```

**Simpler Alternative** - Reuse startGenerator:

Actually, startGenerator already does most of this! We can factor out the common code.

```go
// Modified approach: Call startGenerator but return early after OpInitYield
func (vm *VM) startGeneratorPrologue(genObj *GeneratorObject) (InterpreterStatus, Value) {
    // Reuse startGenerator infrastructure but with special handling
    // Set a flag to indicate this is prologue-only execution
    genObj.State = GeneratorStart

    // Call a specialized version that stops at OpInitYield
    status, _ := vm.executeGeneratorPrologue(genObj)

    return status, Undefined
}

// executeGeneratorPrologue is similar to startGenerator but simpler
func (vm *VM) executeGeneratorPrologue(genObj *GeneratorObject) (InterpreterStatus, Value) {
    funcVal := genObj.Function
    args := genObj.Args
    if args == nil {
        args = []Value{}
    }
    thisValue := genObj.This
    if thisValue.Type() == 0 {
        thisValue = Undefined
    }

    // No sentinel frame needed - direct execution
    shouldSwitch, err := vm.prepareCall(funcVal, thisValue, args, 0, nil, 0)
    if err != nil {
        return InterpretRuntimeError, Undefined
    }

    if !shouldSwitch {
        // Native function - shouldn't happen for generators
        return InterpretRuntimeError, Undefined
    }

    // Set generator context
    if vm.frameCount > 0 {
        vm.frames[vm.frameCount-1].generatorObj = genObj
    }

    genObj.State = GeneratorStart

    // Execute until OpInitYield returns
    status, _ := vm.run()

    // Clean up frame
    if vm.frameCount > 0 {
        regSize := len(vm.frames[vm.frameCount-1].registers)
        vm.frameCount--
        vm.nextRegSlot -= regSize
    }

    return status, Undefined
}
```

This is cleaner and reuses existing infrastructure.

## Testing Strategy

### Smoke Tests

**Existing Tests** (already in codebase):
1. `tests/scripts/gen_param_dstr_null.ts` - expect TypeError
2. `tests/scripts/gen_param_dstr_undefined.ts` - expect TypeError
3. `tests/scripts/gen_param_dstr_valid.ts` - expect 42

**Additional Tests Needed**:
```typescript
// tests/scripts/gen_no_param_dstr.ts
// expect: 42
function* f(x) { yield x; }
console.log(f(42).next().value);

// tests/scripts/gen_nested_dstr.ts
// expect_runtime_error: TypeError
function* f({a: {b}}) { yield b; }
f({a: null});

// tests/scripts/gen_dstr_with_default.ts
// expect: 42
function* f({x = 42}) { yield x; }
console.log(f({}).next().value);

// tests/scripts/gen_method_dstr.ts
// expect_runtime_error: TypeError
const obj = {
  *method({x}) { yield x; }
};
obj.method(null);
```

### Debug Workflow

1. **Enable compiler debug** (compile_literal.go):
   ```go
   const debugCompiler = true  // Line ~10
   ```

2. **Enable VM debug** (vm.go):
   ```go
   const debugVM = true  // Line ~20
   ```

3. **Run single test**:
   ```bash
   go build -o paserati cmd/paserati/main.go
   ./paserati tests/scripts/gen_param_dstr_null.ts
   ```

4. **Check bytecode**:
   ```bash
   ./paserati -bytecode tests/scripts/gen_param_dstr_null.ts
   ```

5. **Run smoke tests**:
   ```bash
   go test ./tests -run TestScripts -v
   ```

### Test262 Validation

```bash
# Build test262 runner
go build -o paserati-test262 ./cmd/paserati-test262

# Baseline before changes
./paserati-test262 -path ./test262 -subpath "language/expressions/generators" -suite -filter -timeout 0.5s > before.txt

# After implementation
./paserati-test262 -path ./test262 -subpath "language/expressions/generators" -suite -filter -timeout 0.5s > after.txt

# Compare
diff before.txt after.txt

# Expected improvement: +200 tests
```

## Implementation Checklist

- [ ] Phase 1: Add OpInitYield opcode to bytecode.go
- [ ] Phase 2: Add GeneratorStart state to value.go
- [ ] Phase 3: Add PrologueStmtCount to AST (ast.go, parser.go)
- [ ] Phase 4: Modify compileFunctionLiteral to emit OpInitYield
- [ ] Phase 5: Implement OpInitYield handler in vm.go
- [ ] Phase 6: Modify OpCreateGenerator to call prologue
- [ ] Phase 7: Implement startGeneratorPrologue function
- [ ] Test: Run smoke tests
- [ ] Test: Validate with Test262
- [ ] Document: Update baseline.txt
- [ ] Commit: Create commit with all changes

## Risk Assessment

**Risk Level**: LOW

**Mitigations**:
- Changes isolated to generator code path
- No impact on regular functions
- Existing tests ensure no regressions
- Can test incrementally with debug flags

**Rollback Plan**:
- Git revert if issues arise
- Baseline.txt tracks Test262 state

## Next Steps

1. Review this analysis
2. Start with Phase 1 (opcodes)
3. Test each phase incrementally
4. Use debug flags liberally
5. Validate with smoke tests before Test262


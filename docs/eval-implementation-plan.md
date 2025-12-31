# Eval Implementation Plan

## Executive Summary

This document outlines the plan to make Paserati's `eval()` function 100% ECMAScript 2025 compliant. The current implementation has fundamental architectural gaps that require changes across the parser, compiler, and VM.

**Current State:**
- Direct eval tests: 32.2% pass rate (92/286)
- Indirect eval tests: 57.4% pass rate (35/61)

**Target State:**
- 95%+ compliance for both direct and indirect eval

## ECMAScript 2025 Eval Requirements

### Direct vs Indirect Eval

Per [MDN](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/eval) and [ECMAScript 2025](https://tc39.es/ecma262/2025/):

**Direct eval** - `eval(...)` called directly:
- Executes in caller's local scope
- Can read/write caller's local variables
- Inherits strict mode from calling context
- `var` declarations leak to caller's scope (in non-strict mode)

**Indirect eval** - any other invocation pattern:
- `(0, eval)("code")` - comma operator
- `const e = eval; e("code")` - aliased
- `eval?.("code")` - optional chaining
- `globalThis.eval("code")` - member access

Indirect eval:
- Executes in global scope (like a separate `<script>` tag)
- Cannot access caller's local variables
- Does NOT inherit strict mode from caller
- `var` declarations become global properties (in non-strict mode)
- `var` declarations are local to eval (in strict mode)

### Variable Scoping Rules

| Scenario | var behavior | let/const behavior |
|----------|-------------|-------------------|
| Direct eval, non-strict caller | Leaks to caller's scope | Scoped to eval block |
| Direct eval, strict caller | Scoped to eval block | Scoped to eval block |
| Indirect eval, non-strict source | Becomes global property | Scoped to eval block |
| Indirect eval, strict source | Scoped to eval block | Scoped to eval block |

### EvalDeclarationInstantiation Algorithm

Per ECMAScript section 19.2.1.3, this algorithm must:

1. **Check declarability**: Before creating any bindings, check if all var/function declarations CAN be created:
   - `CanDeclareGlobalVar(name)` - checks if global property can be created
   - `CanDeclareGlobalFunction(name)` - checks if function can override existing property

2. **Create bindings with correct attributes**:
   - Global var bindings: writable, enumerable, configurable
   - Existing properties: preserve original configurability

3. **Handle strict mode scoping**:
   - In strict mode, create a new declarative environment for var declarations
   - Var declarations do NOT become global properties in strict mode

## Current Implementation Analysis

### What Works
- Basic expression evaluation: `eval("1 + 2")` → 3
- Global variable access: `var x = 10; eval("x + 5")` → 15
- Strict mode detection in caller: `IsInStrictMode()` method exists

### Architecture Gaps

#### Gap 1: No Direct vs Indirect Detection

**Current:** `eval` is a native function with no knowledge of call pattern.

**File:** `pkg/builtins/globals_init.go:388-592`

```go
evalFunc := vm.NewNativeFunction(1, false, "eval", func(args []vm.Value) (vm.Value, error) {
    // No way to detect if called as eval(...) or (0,eval)(...)
    callerIsStrict := ctx.VM.IsInStrictMode()
    // Always passes callerIsStrict regardless of direct/indirect
    chunk, compileErrs := driver.CompileProgramWithStrictMode(prog, callerIsStrict)
})
```

**Solution:** Detect direct eval at compile time and generate a special opcode.

#### Gap 2: Wrong Strict Mode Handling for Indirect Eval

**Current:** Always passes `callerIsStrict` to the compiler.

**Spec:** Indirect eval should NOT inherit strict mode from caller.

**File:** `pkg/builtins/globals_init.go:551-569`

```go
// Current (wrong for indirect eval):
callerIsStrict := ctx.VM.IsInStrictMode()
chunk, compileErrs := driver.CompileProgramWithStrictMode(prog, callerIsStrict)
```

#### Gap 3: No Local Scope Access for Direct Eval

**Current:** `vm.Interpret(chunk)` creates a fresh execution context.

**File:** `pkg/vm/vm.go:499-589` - `Interpret()` method

**Spec:** Direct eval must access caller's local variables.

```javascript
function test() {
    var x = 42;
    return eval("x");  // Should return 42, currently throws ReferenceError
}
```

#### Gap 4: No EvalDeclarationInstantiation

**Current:** Vars are compiled as globals via `OpSetGlobal` without:
- Checking if the declaration can be created
- Proper property descriptor handling
- GlobalObject property creation

**Spec:** The algorithm must:
1. Check `CanDeclareGlobalVar` / `CanDeclareGlobalFunction`
2. Create properties on the global object with correct attributes
3. Throw TypeError if declaration conflicts with non-configurable property

#### Gap 5: Compiler Lacks Eval-Aware Compilation

**Current:** Compiler always treats top-level vars as globals.

**File:** `pkg/compiler/compiler.go:578-597`

```go
if c.enclosing == nil {
    varNames := collectVarDeclarations(program.Statements)
    for _, name := range varNames {
        globalIdx := c.GetOrAssignGlobalIndex(name)
        // Always global, even in strict eval where it should be local
    }
}
```

**Spec:** Var handling depends on eval mode:
- Strict eval: vars are local to eval
- Non-strict direct eval: vars leak to caller's scope
- Non-strict indirect eval: vars become global properties

## Implementation Plan

### Phase 1: Direct vs Indirect Detection

**Goal:** Distinguish direct eval calls at compile time.

#### 1.1 Parser Changes

Add detection in `parseCallExpression`:

```go
// pkg/parser/parser.go

type CallExpression struct {
    // ... existing fields
    IsDirectEvalCall bool  // NEW: true if this is `eval(...)`
}

func (p *Parser) parseCallExpression() *CallExpression {
    // ... existing logic

    // Detect direct eval: callee must be Identifier with name "eval"
    if ident, ok := callee.(*Identifier); ok && ident.Value == "eval" {
        call.IsDirectEvalCall = true
    }
}
```

#### 1.2 Compiler Changes

Generate different opcodes for direct vs indirect eval:

```go
// pkg/compiler/compiler.go

case *parser.CallExpression:
    if node.IsDirectEvalCall {
        // Generate OpDirectEval - includes caller's scope info
        return c.compileDirectEval(node, hint)
    }
    // Regular call, which may call the eval function indirectly
    return c.compileCall(node, hint)
```

#### 1.3 New Opcodes

```go
// pkg/vm/bytecode.go

OpDirectEval  OpCode = XX  // Direct eval: Rx ArgReg - eval with caller's scope
```

### Phase 2: Strict Mode Handling

**Goal:** Only inherit strict mode for direct eval.

#### 2.1 VM Changes

Add method to determine eval context:

```go
// pkg/vm/vm.go

type EvalContext struct {
    IsDirect      bool
    CallerStrict  bool
    CallerScope   *Scope  // For direct eval local variable access
}

func (vm *VM) EvalCode(code string, ctx EvalContext) (Value, error) {
    // Determine strict mode:
    // - Direct eval: inherit from caller OR "use strict" in eval code
    // - Indirect eval: only "use strict" in eval code
    inheritStrict := ctx.IsDirect && ctx.CallerStrict

    // Compile with appropriate strict mode
    chunk := compiler.CompileEval(code, inheritStrict, ctx.IsDirect)

    // Execute in appropriate scope
    if ctx.IsDirect {
        return vm.ExecuteInScope(chunk, ctx.CallerScope)
    }
    return vm.ExecuteGlobal(chunk)
}
```

### Phase 3: Local Scope Access for Direct Eval

**Goal:** Direct eval can read/write caller's local variables.

#### 3.1 Scope Capture

When compiling direct eval call, capture caller's scope:

```go
// pkg/compiler/compiler.go

func (c *Compiler) compileDirectEval(node *parser.CallExpression, hint Register) {
    // 1. Compile the code string argument
    // 2. Emit OpDirectEval with scope information

    // The scope information includes:
    // - Symbol table snapshot (for variable resolution)
    // - Register mapping (for accessing locals)
}
```

#### 3.2 Execution with Captured Scope

```go
// pkg/vm/vm.go

func (vm *VM) executeDirectEval(code string, callerScope *ScopeInfo) (Value, error) {
    // Compile with scope info to resolve local variables
    chunk := compiler.CompileEvalWithScope(code, callerScope)

    // Execute in caller's frame context
    // - Share caller's registers for local variable access
    // - Use caller's closure for upvalue access
}
```

### Phase 4: EvalDeclarationInstantiation

**Goal:** Implement full ECMAScript algorithm for declaration handling.

#### 4.1 Pre-flight Checks

Before executing eval code, check all declarations:

```go
// pkg/vm/eval.go (new file)

func (vm *VM) evalDeclarationInstantiation(body *parser.Program, varEnv, lexEnv Environment, strict bool) error {
    // 1. Collect all var and function declarations
    varDecls := collectVarDeclarations(body)
    funcDecls := collectFunctionDeclarations(body)

    // 2. Check if declarations can be created
    for _, fn := range funcDecls {
        if !vm.canDeclareGlobalFunction(fn.Name) {
            return NewTypeError("Cannot declare function " + fn.Name)
        }
    }

    for _, v := range varDecls {
        if !vm.canDeclareGlobalVar(v) {
            return NewTypeError("Cannot declare var " + v)
        }
    }

    // 3. Create function bindings
    for _, fn := range funcDecls {
        vm.createGlobalFunctionBinding(fn.Name, fn.Value, true)
    }

    // 4. Create var bindings
    for _, v := range varDecls {
        vm.createGlobalVarBinding(v, true)
    }

    return nil
}

func (vm *VM) canDeclareGlobalVar(name string) bool {
    // Check if name can be declared as global var
    if existing, ok := vm.GlobalObject.GetPropertyDescriptor(name); ok {
        // Property exists - check if configurable
        return existing.Configurable
    }
    return vm.GlobalObject.IsExtensible()
}

func (vm *VM) canDeclareGlobalFunction(name string) bool {
    if existing, ok := vm.GlobalObject.GetPropertyDescriptor(name); ok {
        // Can override if configurable OR if existing is already a function
        return existing.Configurable || existing.Value.IsFunction()
    }
    return vm.GlobalObject.IsExtensible()
}
```

#### 4.2 Property Creation

Create properties with correct descriptors:

```go
func (vm *VM) createGlobalVarBinding(name string, configurable bool) {
    if _, exists := vm.GlobalObject.GetOwn(name); exists {
        return  // Don't override existing
    }

    // Create property: writable=true, enumerable=true, configurable=true
    vm.GlobalObject.DefineProperty(name, PropertyDescriptor{
        Value:        Undefined,
        Writable:     true,
        Enumerable:   true,
        Configurable: configurable,
    })
}
```

### Phase 5: Compiler Eval Mode

**Goal:** Different compilation modes for different eval contexts.

#### 5.1 Eval Compilation Modes

```go
// pkg/compiler/compiler.go

type EvalMode int

const (
    EvalModeNone          EvalMode = iota  // Not in eval
    EvalModeDirectSloppy                   // Direct eval in sloppy mode
    EvalModeDirectStrict                   // Direct eval in strict mode
    EvalModeIndirectSloppy                 // Indirect eval, sloppy source
    EvalModeIndirectStrict                 // Indirect eval, strict source
)

func (c *Compiler) SetEvalMode(mode EvalMode, callerScope *ScopeInfo) {
    c.evalMode = mode
    c.evalCallerScope = callerScope
}
```

#### 5.2 Var Declaration Handling by Mode

```go
func (c *Compiler) compileVarStatement(node *parser.VarStatement, hint Register) {
    switch c.evalMode {
    case EvalModeDirectSloppy:
        // Vars leak to caller's scope
        // Need to emit OpSetLocal to caller's frame

    case EvalModeDirectStrict:
        // Vars are local to eval
        // Emit OpSetLocal to eval's own frame

    case EvalModeIndirectSloppy:
        // Vars become global properties
        // Emit OpSetGlobal AND create property on GlobalObject

    case EvalModeIndirectStrict:
        // Vars are local to eval
        // Emit OpSetLocal to eval's own frame

    default:
        // Regular script compilation
        // Current behavior
    }
}
```

## Testing Strategy

### Unit Tests

Add tests in `tests/scripts/`:

```typescript
// eval_direct_local_access.ts
// expect: 42
function test() {
    var x = 42;
    return eval("x");
}
test();

// eval_direct_var_leak.ts
// expect: 123
function test() {
    eval("var leaked = 123");
    return leaked;
}
test();

// eval_indirect_no_strict_inherit.ts
// expect: 1
"use strict";
// Indirect eval should allow 'with' even in strict caller
(0,eval)("var result = 1; with({}) result;");
result;

// eval_strict_no_var_leak.ts
// expect: undefined
(0,eval)("'use strict'; var strictVar = 99;");
typeof strictVar;
```

### Test262 Verification

Run eval-code suite after each phase:

```bash
./paserati-test262 -path ./test262 -subpath "language/eval-code" -diff baseline.txt
```

Target pass rates:
- Phase 1 complete: 50%+
- Phase 2 complete: 70%+
- Phase 3 complete: 85%+
- Phase 4 complete: 95%+

## Implementation Order

1. **Phase 1** - Direct vs Indirect Detection (Parser + Compiler changes)
2. **Phase 2** - Strict Mode Handling (VM changes)
3. **Phase 4** - EvalDeclarationInstantiation (VM + runtime checks)
4. **Phase 3** - Local Scope Access (Complex - requires frame sharing)
5. **Phase 5** - Compiler Eval Mode (Ties everything together)

Phases 1-2 can be done without breaking existing functionality. Phase 3 is the most complex and requires careful frame management. Phase 4 can be done incrementally. Phase 5 ties everything together.

## Risk Assessment

### High Risk
- **Phase 3** (Local Scope Access): Requires sharing frame registers between eval'd code and caller. Could introduce subtle bugs.

### Medium Risk
- **Phase 4** (EvalDeclarationInstantiation): Requires careful property descriptor handling. Could break existing tests.

### Low Risk
- **Phase 1** (Detection): Parser change is isolated. Backward compatible.
- **Phase 2** (Strict Mode): Simple conditional logic change.

## Files to Modify

| File | Changes |
|------|---------|
| `pkg/parser/ast.go` | Add `IsDirectEvalCall` to CallExpression |
| `pkg/parser/parser.go` | Detect direct eval in parseCallExpression |
| `pkg/compiler/compiler.go` | Add EvalMode, modify var compilation |
| `pkg/compiler/compile_expression.go` | Handle direct eval compilation |
| `pkg/vm/bytecode.go` | Add OpDirectEval opcode |
| `pkg/vm/vm.go` | Add EvalCode method, scope sharing |
| `pkg/vm/eval.go` (new) | EvalDeclarationInstantiation |
| `pkg/builtins/globals_init.go` | Update eval native function |

## Conclusion

Implementing spec-compliant eval requires coordinated changes across the parser, compiler, and VM. The key insight is that direct vs indirect eval must be detected at compile time, and each mode requires different:

1. Strict mode inheritance rules
2. Variable scoping behavior
3. GlobalObject property creation rules

The implementation should be done in phases to minimize risk and allow incremental testing against Test262.

# Destructuring Implementation Audit & Analysis

**Date**: 2025-10-12
**Status**: Critical Bug Identified
**Impact**: 12+ Test262 failures in object expression tests

## Executive Summary

This document provides a comprehensive audit of destructuring implementation across Paserati's parser, type checker, and compiler. A **critical bug** has been identified: **destructuring function parameters with default values are completely unimplemented**, causing all extracted variables to be `null` or contain garbage values.

**Test262 Impact**: At least 12 tests failing with pattern `Expected SameValue(«null», «7»)` are directly caused by this bug.

---

## 1. AST Representation (Parser Layer)

### 1.1 Core Destructuring AST Nodes

The parser defines comprehensive AST nodes for all destructuring contexts:

#### Assignment Context
- **`ArrayDestructuringAssignment`**: `[a, b, c] = expr`
  - Fields: `Elements []*DestructuringElement`, `Value Expression`
  - Used in: assignment expressions

- **`ObjectDestructuringAssignment`**: `{a, b} = expr`
  - Fields: `Properties []*DestructuringProperty`, `RestProperty *DestructuringElement`, `Value Expression`
  - Used in: assignment expressions

#### Declaration Context
- **`ArrayDestructuringDeclaration`**: `let/const/var [a, b, c] = expr`
  - Fields: `IsConst bool`, `Elements []*DestructuringElement`, `Value Expression`, `TypeAnnotation Expression`
  - Used in: variable declarations, for-of loops

- **`ObjectDestructuringDeclaration`**: `let/const/var {a, b} = expr`
  - Fields: `IsConst bool`, `Properties []*DestructuringProperty`, `RestProperty *DestructuringElement`, `Value Expression`, `TypeAnnotation Expression`
  - Used in: variable declarations, for-of loops

#### Parameter Context
- **`ArrayParameterPattern`**: `([a, b]: [number, number]) => {}`
  - Fields: `Elements []*DestructuringElement`
  - Used in: function parameters (Pattern field of Parameter)

- **`ObjectParameterPattern`**: `({x, y}: Point) => {}`
  - Fields: `Properties []*DestructuringProperty`, `RestProperty *DestructuringElement`
  - Used in: function parameters (Pattern field of Parameter)

#### Supporting Nodes
- **`DestructuringElement`**: Represents individual elements in array destructuring
  - Fields: `Target Expression`, `Default Expression`, `IsRest bool`
  - Supports: nested patterns, default values, rest elements

- **`DestructuringProperty`**: Represents properties in object destructuring
  - Fields: `Key Expression`, `Target Expression`, `Default Expression`
  - Supports: renamed bindings (`{x: y}`), default values

### 1.2 Parameter Representation

**`Parameter` struct** (ast.go:253-268):
```go
type Parameter struct {
    Token           lexer.Token
    Name            *Identifier      // For simple parameters
    Pattern         Expression       // For destructuring (ArrayParameterPattern/ObjectParameterPattern)
    TypeAnnotation  Expression
    ComputedType    types.Type
    Optional        bool
    DefaultValue    Expression       // Default value for THE PARAMETER (not pattern elements)
    IsThis          bool
    IsDestructuring bool             // Flag indicating destructuring
    // ... property modifiers ...
}
```

**Critical Design Detail**:
- `Pattern` holds the destructuring pattern (e.g., `[x, y]`)
- `DefaultValue` holds the default for THE ENTIRE PARAMETER (e.g., `[99, 99]` in `[[x, y] = [99, 99]]`)
- Individual elements within the pattern can ALSO have defaults (stored in `DestructuringElement.Default`)

**Example**: `function f([[x, y = 5] = [1, 2]])`
- Parameter has:
  - `Pattern`: `ArrayParameterPattern` containing nested `ArrayParameterPattern`
  - `DefaultValue`: `[1, 2]`
  - `IsDestructuring`: `true`
- Inner pattern element has:
  - `Target`: identifier `y`
  - `Default`: `5`

---

## 2. Type Checker Implementation

### 2.1 Implemented Contexts

The checker provides complete type checking for all destructuring contexts:

#### ✅ Array Destructuring
- **Assignment**: `checkArrayDestructuringAssignment()` (assignment.go:189)
  - Validates tuple type compatibility
  - Handles defaults, rest elements, nested patterns
  - Special handling for tuple types with `checkArrayDestructuringWithTuple()`

- **Declaration**: `checkArrayDestructuringDeclaration()` (checker.go:2923)
  - Type inference from RHS
  - Validates element types
  - Handles nested declarations via `checkDestructuringTarget()`

- **For-of loops**: `checkArrayDestructuringInForLoop()` (statements.go:721)
  - Validates iterable element types

#### ✅ Object Destructuring
- **Assignment**: `checkObjectDestructuringAssignment()` (assignment.go:314)
  - Property type resolution
  - Handles computed properties, rest properties

- **Declaration**: `checkObjectDestructuringDeclaration()` (checker.go:3050)
  - Property existence validation
  - Type compatibility checking

- **For-of loops**: `checkObjectDestructuringInForLoop()` (statements.go:756)

#### ✅ Nested Destructuring
- **`checkDestructuringTarget()`** (destructuring_nested.go:10)
  - Recursive descent for nested patterns
  - Handles: `[a, [b, c]]`, `{x, y: {z}}`, `[a, {b}]`

- **`checkDestructuringTargetForProperty()`** (destructuring_nested.go:27)
  - Property-specific nested checking

- **`checkDestructuringTargetForDeclaration()`** (destructuring_nested.go:193)
  - Declaration-specific nested checking with const validation

### 2.2 Parameter Destructuring

**Status**: ✅ Fully Implemented

The checker handles destructuring parameters in `function.go`:
- Validates parameter patterns against argument types
- Infers types for pattern elements
- Handles nested patterns and defaults

---

## 3. Compiler Implementation

### 3.1 Implemented Contexts

The compiler provides complete bytecode generation for most destructuring contexts:

#### ✅ Array Destructuring Assignment
- **`compileArrayDestructuringAssignment()`** (compile_assignment.go:578)
  - Compiles to iterator protocol or fast array access
  - Emits `OpGetIterator`, `OpIteratorNext`, `OpIteratorDone`, `OpIteratorValue`
  - Handles defaults, rest elements

- **`compileArrayDestructuringWithValueReg()`** (compile_assignment.go:847)
  - Helper for when RHS is already in a register

- **Nested support**: `compileNestedArrayDestructuring()` (compile_assignment.go:736)
  - Recursively handles `[a, [b, c]]` patterns

#### ✅ Object Destructuring Assignment
- **`compileObjectDestructuringAssignment()`** (compile_assignment.go:971)
  - Emits `OpGetProp` for each property
  - Handles computed properties, renamed bindings

- **`compileObjectDestructuringWithValueReg()`** (compile_assignment.go:910)

- **Rest properties**: `compileObjectRestProperty()` (compile_assignment.go:1165)
  - Emits `OpObjectRest` for `{a, ...rest}` patterns

#### ✅ Array Destructuring Declaration
- **`compileArrayDestructuringDeclaration()`** (compile_assignment.go:1307)
  - Two-phase compilation:
    1. Define all variables in symbol table
    2. Emit extraction bytecode

- **Fast path**: `compileArrayDestructuringFastPath()` (compile_assignment.go:1353)
  - For simple arrays without defaults/rest
  - Uses `OpGetIndex` directly

- **Iterator path**: `compileArrayDestructuringIteratorPath()` (compile_assignment.go:1448)
  - For complex cases with defaults/rest/nested patterns
  - Full iterator protocol

- **Helper**: `compileArrayDestructuringDeclarationWithValueReg()` (compile_nested_declarations.go:147)

#### ✅ Object Destructuring Declaration
- **`compileObjectDestructuringDeclaration()`** (compile_assignment.go:1588)
  - Similar two-phase approach

- **Helper**: `compileObjectDestructuringDeclarationWithValueReg()` (compile_nested_declarations.go:243)

- **Rest**: `compileObjectRestDeclaration()` (compile_assignment.go:1232)

#### ✅ For-of Loop Destructuring
- **`compileForOfArrayDestructuring()`** (compile_for_of_new.go:404)
  - Specialized path for `for (let [a, b] of items)`
  - Emits extraction code in loop body

### 3.2 Parameter Destructuring

**Status**: ❌ **COMPLETELY UNIMPLEMENTED**

#### Current Behavior (compile_literal.go:860-947)

When compiling function parameters:

```go
// Line 868-872: SKIP destructuring parameters
if param.IsDestructuring {
    debugPrintf("// [Compiling Function Literal] Skipping untransformed destructuring parameter\n")
    continue  // ⚠️ NO CODE GENERATED!
}

// Line 895-896: SKIP defaults for destructuring parameters
if param.IsDestructuring {
    continue  // ⚠️ DEFAULTS IGNORED!
}
```

**What happens**:
1. Destructuring parameters are detected via `IsDestructuring` flag
2. Compiler **skips them entirely** with comments expecting "desugared declarations"
3. **No such desugaring exists** in parser or checker
4. **No bytecode is generated** to extract pattern elements from argument values
5. Variables in the pattern remain uninitialized (null/undefined)

#### Example Failure

**Source**:
```typescript
function f([[x, y] = [99, 99]]) {
  console.log(x, y);
}
f([[1, 2]]);
```

**Expected**: `1 2`
**Actual**: `null null` (or garbage values)

**Why it fails**:
1. VM receives argument `[[1, 2]]` correctly
2. No synthetic parameter is created to hold it
3. No extraction code is emitted for the nested pattern
4. Variables `x` and `y` are never bound to values from the argument
5. Default value `[99, 99]` is never evaluated or used

---

## 4. Test Results Summary

### 4.1 Working Contexts

All tested with `./paserati --no-typecheck /tmp/test_all_destruct.js`:

| Context | Example | Status |
|---------|---------|--------|
| Array declaration | `let [a, b] = [1, 2]` | ✅ Works |
| Array with defaults | `let [a = 10, b = 20] = [3]` | ✅ Works |
| Nested array declaration | `let [[a, b]] = [[4, 5]]` | ✅ Works |
| Array assignment | `[a, b] = [6, 7]` | ✅ Works |
| Object declaration | `let {x, y} = {x: 8, y: 9}` | ✅ Works |
| For-of destructuring | `for (let [a, b] of items)` | ✅ Works |
| Simple param destructuring | `function f([a, b])` | ✅ Works |

### 4.2 Broken Context

| Context | Example | Expected | Actual | Test262 Impact |
|---------|---------|----------|--------|----------------|
| **Param with pattern + default** | `function f([[x] = [99]])` | `x = 42` when called with `[[42]]` | `x = null` or garbage | **12+ tests fail** |

**Specific failure modes**:
- Nested array with default: `[[x, y] = [1, 2]]` → both null
- Nested object with default: `[{a} = {a: 5}]` → undefined
- Complex patterns: `[[x, [y, z]] = [1, [2, 3]]]` → all null

---

## 5. Root Cause Analysis

### 5.1 Historical Context

From code comments (compile_literal.go:867, 870, 894-895):

```go
// Skip destructuring parameters if parser didn't transform them
// The desugared declaration statements in the function body will handle binding
```

**Evidence suggests**:
- Original design expected parser/checker to **transform** destructuring parameters
- Transformation would insert synthetic parameters and desugaring statements at function body start
- **This transformation was never implemented**
- Compiler code was written assuming it exists
- Comments warn but don't implement fallback

### 5.2 Why Simple Patterns Work But Defaults Don't

**Simple destructuring** (`function f([a, b])`) works because:
- Parser likely creates a workaround for the simplest case
- Or the VM happens to directly destructure first-level arrays
- **Hypothesis**: There may be special handling somewhere

**Patterns with defaults fail** because:
- Defaults require conditional logic: "use arg if defined, else use default"
- This needs explicit bytecode generation
- No code path exists to emit this logic for parameters

### 5.3 Missing Transformation

**What SHOULD happen** (not implemented):

For `function f([[x, y] = [99, 99]])`:

1. **Parser transformation**:
   ```typescript
   function f(__param0) {  // Synthetic parameter
     let [[x, y] = [99, 99]] = __param0;  // Desugaring statement
     // original body
   }
   ```

2. **OR Compiler direct handling**:
   - Create synthetic parameter register
   - At function start, emit destructuring bytecode
   - Handle default value logic explicitly

**Neither approach is implemented**.

---

## 6. Fix Strategies

### Strategy A: Parser/Checker Transformation (Recommended)

**Where**: `pkg/parser/parser.go` or `pkg/checker/function.go`

**When**: During function parameter processing

**How**:
1. Detect parameters with `IsDestructuring = true`
2. For each such parameter:
   - Create synthetic parameter name (e.g., `__param0`, `__param1`)
   - Replace pattern with simple parameter
   - Create destructuring declaration statement
   - Insert statement at beginning of function body

**Advantages**:
- Reuses existing, tested destructuring declaration compilation
- Keeps compiler simple
- Matches ECMAScript spec semantics closely
- Handles all edge cases (nested, defaults, rest) automatically

**Disadvantages**:
- Requires AST modification
- Needs careful preservation of source locations for error messages
- More invasive change

**Example transformation**:

```typescript
// Before
function f([[x, y] = [99, 99]], z) { body }

// After (AST transformation)
function f(__param0, z) {
  let [[x, y] = [99, 99]] = __param0;
  body
}
```

### Strategy B: Direct Compiler Implementation

**Where**: `pkg/compiler/compile_literal.go`

**When**: During function compilation, after parameter definition

**How**:
1. First pass: Create synthetic parameters for destructuring
   ```go
   if param.IsDestructuring {
       synthName := fmt.Sprintf("__param%d", i)
       synthReg := functionCompiler.regAlloc.Alloc()
       functionCompiler.currentSymbolTable.Define(synthName, synthReg)
       functionCompiler.regAlloc.Pin(synthReg)
       // Store mapping: param index -> synthetic register
   }
   ```

2. After all parameters defined, before body compilation:
   ```go
   for i, param := range node.Parameters {
       if param.IsDestructuring {
           synthReg := syntheticParamRegs[i]

           // Handle default value
           if param.DefaultValue != nil {
               // Emit: if (synthReg === undefined) synthReg = defaultValue
               functionCompiler.emitCheckUndefinedAndAssign(synthReg, param.DefaultValue, line)
           }

           // Emit destructuring extraction
           if arrayPattern, ok := param.Pattern.(*parser.ArrayParameterPattern); ok {
               functionCompiler.compileParameterArrayPattern(arrayPattern, synthReg, line)
           } else if objectPattern, ok := param.Pattern.(*parser.ObjectParameterPattern); ok {
               functionCompiler.compileParameterObjectPattern(objectPattern, synthReg, line)
           }
       }
   }
   ```

3. Implement new helper functions:
   - `compileParameterArrayPattern()`: Extract from array pattern
   - `compileParameterObjectPattern()`: Extract from object pattern
   - Reuse logic from existing destructuring declaration compilers

**Advantages**:
- No AST modification needed
- All changes confined to compiler
- Can optimize for parameter-specific patterns

**Disadvantages**:
- Duplicates destructuring logic
- More code to maintain
- Harder to ensure parity with declaration destructuring

### Strategy C: Hybrid Approach

**Where**: Both checker and compiler

**When**:
- Checker: Mark parameters needing desugaring
- Compiler: Implement direct handling for marked parameters

**How**:
1. Checker adds metadata to Parameter nodes
2. Compiler uses metadata to generate optimized code
3. Falls back to Strategy A transformation for complex cases

**Advantages**:
- Flexibility for optimization
- Gradual implementation path

**Disadvantages**:
- Most complex approach
- Higher maintenance burden

---

## 7. Recommended Implementation Plan

### Phase 1: Implement Strategy A (Parser Transformation)

**Priority**: CRITICAL
**Estimated Complexity**: Medium
**Files to modify**:
- `pkg/parser/parser.go`: Add `transformDestructuringParameters()` function
- Or `pkg/checker/function.go`: Transform during type checking phase

**Steps**:
1. Create transformation function:
   ```go
   func transformDestructuringParameters(funcNode *FunctionLiteral) {
       // For each destructuring parameter:
       // 1. Generate synthetic name
       // 2. Create simple parameter
       // 3. Create destructuring declaration
       // 4. Prepend to body.Statements
       // 5. Update parameter list
   }
   ```

2. Call during function parsing/checking:
   ```go
   // In parseFunctionParameters() or checkFunctionLiteral()
   if hasDestructuringParams(node) {
       transformDestructuringParameters(node)
   }
   ```

3. Test incrementally:
   - Simple array pattern: `function f([x])`
   - With defaults: `function f([x = 1])`
   - Nested: `function f([[x]])`
   - Nested with defaults: `function f([[x] = [1]])`
   - Object patterns: `function f({x})`
   - Mixed: `function f([x], {y}, z)`

### Phase 2: Update Compiler

**Priority**: CRITICAL
**Files to modify**:
- `pkg/compiler/compile_literal.go`

**Changes**:
1. Remove skip logic (lines 868-872, 895-896)
2. Add assertion that destructuring parameters have been transformed:
   ```go
   if param.IsDestructuring {
       panic("Destructuring parameters should have been transformed by parser/checker")
   }
   ```
3. Document the transformation requirement

### Phase 3: Comprehensive Testing

**Files to create**:
- `tests/scripts/param_destruct_array_simple.ts`
- `tests/scripts/param_destruct_array_default.ts`
- `tests/scripts/param_destruct_nested.ts`
- `tests/scripts/param_destruct_object.ts`
- `tests/scripts/param_destruct_mixed.ts`

**Test262 validation**:
```bash
./paserati-test262 -path ./test262 -subpath "language/expressions/object/dstr" \
                    -filter -timeout 0.5s
```

**Expected improvement**: 12+ additional tests passing

---

## 8. Additional Findings

### 8.1 Edge Cases to Consider

1. **Rest parameters with destructuring**:
   ```typescript
   function f([a, b], ...rest) { }  // rest is separate, OK
   function g([a, ...rest]) { }     // rest within pattern, needs handling
   ```

2. **Destructuring in arrow functions**:
   ```typescript
   const f = ([[x, y]]) => x + y;  // Same bug applies
   ```

3. **Destructuring in methods**:
   ```typescript
   const obj = {
     method([[x, y]]) { return x + y; }  // Same bug
   };
   ```

4. **Destructuring in class constructors**:
   ```typescript
   class C {
     constructor([[x, y]]) {  // Same bug + property parameter interaction
       this.x = x;
     }
   }
   ```

5. **Generator and async function parameters**:
   ```typescript
   function* gen([[x]]) { yield x; }  // Same bug
   async function f([[x]]) { return x; }  // Same bug
   ```

### 8.2 Related VM Considerations

The VM's `prepareCall()` function (pkg/vm/call.go) handles parameter passing:
- Assigns arguments to parameter registers sequentially
- **No special handling for destructuring** (expects compiler to have handled it)
- This is correct IF transformation happens at compile time

### 8.3 Performance Implications

**Current**:
- Simple parameters: Direct register assignment in VM
- Destructuring parameters: **Broken** (no implementation)

**After fix**:
- Simple parameters: Same (no change)
- Destructuring parameters: Additional bytecode execution at function entry
  - Acceptable overhead (same as var declaration destructuring)
  - Could optimize later with specialized opcodes

---

## 9. Impact Assessment

### 9.1 Test262 Failures

**Direct impact**: 12+ tests with pattern `Expected SameValue(«null», «7»)`

**Categories affected**:
- `language/expressions/object/dstr/meth-ary-ptrn-elem-*`: Method array pattern tests
- `language/expressions/object/dstr/async-gen-meth-ary-ptrn-*`: Async generator tests
- `language/expressions/object/dstr/gen-meth-ary-ptrn-*`: Generator method tests

**Potential additional impact**: Tests with:
- "Expected SameValue(«null», «777»)" (6 tests)
- "Expected SameValue(«0», «1»)" (6 tests)
- Any destructuring parameter test

### 9.2 User Code Impact

**Breaking**: Any code using destructuring in function parameters with:
- Nested patterns: `function f([[x]])`
- Defaults: `function f([x] = [1])`
- Combination: `function f([[x] = [1]])`

**Working**:
- Simple destructuring: `function f([x, y])`  (somehow works, needs investigation)
- No parameters: All other destructuring contexts

---

## 10. Conclusion

**Critical Bug Identified**: Destructuring function parameters with patterns and defaults are completely unimplemented due to missing transformation between parser and compiler.

**Severity**: HIGH - Breaks ECMAScript compliance, fails 12+ Test262 tests, affects user code

**Recommended Action**: Implement Strategy A (Parser/Checker transformation) immediately

**Expected Outcome**:
- ✅ 12+ Test262 tests pass
- ✅ Full ES6+ destructuring parameter support
- ✅ Compliance with ECMAScript specification
- ✅ No performance regression (acceptable overhead)

**Next Steps**:
1. Implement `transformDestructuringParameters()` in parser or checker
2. Remove skip logic in compiler
3. Add comprehensive test coverage
4. Validate with Test262 suite
5. Document transformation behavior

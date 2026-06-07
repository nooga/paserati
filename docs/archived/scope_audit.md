# Block Scope Implementation Audit

## Current State

### Scope Mechanisms
1. **Compiler nesting**: `c.enclosing` points to parent function's compiler
2. **Symbol table chain**: `c.currentSymbolTable.Outer` points to parent scope
3. **Function body flag**: `c.isCompilingFunctionBody` marks the function body BlockStatement

### Variable Access Logic
| Scenario | Condition | Access Method |
|----------|-----------|---------------|
| Global variable | `IsGlobal=true` | OpGetGlobal |
| Local in current scope | `definingTable == currentTable` | Direct register |
| Outer block (top level) | `c.enclosing == nil && definingTable != currentTable` | Direct register |
| Free variable (closure) | `c.enclosing != nil && definingTable != currentTable` | OpLoadFree |

## Problem: Inner Blocks in Functions

### Test Case
```typescript
function test() {
  let x = 'outer';
  {
    let x = 'inner';
  }
  return x;
}
test(); // Returns 'inner' ✗ (should return 'outer')
```

### Current Behavior
1. `compileFunctionLiteral` sets `isCompilingFunctionBody = true`
2. Compiles function body BlockStatement (no enclosed scope created ✓)
3. Body compilation includes compiling inner `{ let x = 'inner'; }`
4. Inner block sees `isCompilingFunctionBody = true` (still set!)
5. Inner block doesn't create enclosed scope ✗
6. Both `let x` declarations go to same symbol table
7. Second `let x` overwrites first one

### Root Cause
`isCompilingFunctionBody` flag:
- Set at start of function body compilation
- Affects ALL nested BlockStatements during compilation
- Not reset until after entire function body is compiled
- Prevents any inner blocks from creating enclosed scopes

## Solution Design

### Option 1: One-Shot Flag (CHOSEN)
Reset `isCompilingFunctionBody` immediately after first BlockStatement checks it.

**Implementation:**
```go
case *parser.BlockStatement:
    wasFunctionBody := c.isCompilingFunctionBody
    if wasFunctionBody {
        c.isCompilingFunctionBody = false  // Reset immediately
    }
    needsEnclosedScope := !wasFunctionBody
    // ... rest of BlockStatement compilation
```

**Pro:** Simple, minimal changes
**Con:** Need to ensure it doesn't break nested function compilation

### Option 2: Explicit Parent Tracking
Track if this specific BlockStatement is a function body via AST parent relationship.

**Pro:** More explicit
**Con:** Requires AST parent pointers or extra bookkeeping

### Option 3: Scope Depth Counter
Track nesting level of BlockStatements within function.

**Pro:** Could handle complex nesting
**Con:** More complex, overkill for this problem

## Key Distinction

**Within same function:**
- Outer block scope variable → Direct register access (NOT upvalue)
- All scopes share same compiler (`c`)
- Symbol tables form chain via `Outer` pointers

**Across function boundary:**
- Outer function variable → Upvalue (OpLoadFree)
- Different compilers (`c` vs `c.enclosing`)
- Need closure capture mechanism


## One-Shot Flag Trace Analysis

### Scenario 1: Simple Function with Inner Block
```typescript
function test() {
  let x = 'outer';
  {
    let x = 'inner';
  }
  return x;
}
```

**Compilation trace:**
1. `compileFunctionLiteral` creates `functionCompiler`
2. Sets `functionCompiler.isCompilingFunctionBody = true`
3. Calls `functionCompiler.compileNode(node.Body, ...)`
4. **BlockStatement case (function body):**
   - Checks `isCompilingFunctionBody` → true
   - `needsEnclosedScope = false` ✓
   - Sets `isCompilingFunctionBody = false` (RESET HERE)
   - Compiles statements in function body
5. Encounters inner `{ let x = 'inner'; }`
6. **BlockStatement case (inner block):**
   - Checks `isCompilingFunctionBody` → false (already reset!)
   - `needsEnclosedScope = true` ✓
   - Creates enclosed scope
   - Defines `x` in new scope
7. After block, restores previous scope
8. `return x` accesses outer `x` ✓

**Result:** ✓ Works correctly

### Scenario 2: Nested Functions
```typescript
function outer() {
  let x = 'outer';
  function inner() {
    return x;  // Should access as free variable
  }
  return inner();
}
```

**Compilation trace:**
1. `compileFunctionLiteral(outer)` creates `outerCompiler`
2. Sets `outerCompiler.isCompilingFunctionBody = true`
3. Compiles outer function body:
   - BlockStatement resets `outerCompiler.isCompilingFunctionBody = false`
4. Encounters `function inner() {...}`
5. Calls `outerCompiler.compileFunctionLiteral(inner, ...)`
6. Creates NEW `innerCompiler = newFunctionCompiler(outerCompiler)`
   - `innerCompiler.enclosing = outerCompiler` ✓
   - `innerCompiler.isCompilingFunctionBody = false` (fresh compiler state)
7. Sets `innerCompiler.isCompilingFunctionBody = true`
8. Compiles inner function body:
   - BlockStatement resets `innerCompiler.isCompilingFunctionBody = false`
9. `return x` in inner function:
   - Looks up `x`, finds in `outerCompiler.currentSymbolTable`
   - `definingTable != currentTable` AND `c.enclosing != nil`
   - Treats as free variable ✓
   - Emits OpLoadFree ✓

**Result:** ✓ Works correctly (nested functions use different compilers)

### Scenario 3: If-Statement Blocks
```typescript
function test(y) {
  if (y) {
    return x + 1;
  } else {
    return x + 2;
  }
}
```

**Compilation trace:**
1. `compileFunctionLiteral` sets `isCompilingFunctionBody = true`
2. Compiles function body BlockStatement:
   - Resets `isCompilingFunctionBody = false`
   - Compiles if-statement
3. If-statement compiles consequence BlockStatement:
   - Checks `isCompilingFunctionBody` → false
   - `needsEnclosedScope = true` ✓
   - Creates enclosed scope for if-consequence
   - Accesses `y` parameter from outer scope
   - `definingTable != currentTable` but `c.enclosing != nil`
   - Wait... this would treat `y` as free variable! ✗

**PROBLEM FOUND!**


## Solution: Distinguish Same-Function vs Cross-Function Scopes

### The Real Issue
When we check `c.enclosing != nil && definingTable != currentTable`, we can't tell:
- Variable from outer BLOCK scope in same function → Use direct register
- Variable from outer FUNCTION → Use upvalue (OpLoadFree)

### The Fix
Check if `definingTable` belongs to the current compiler's scope chain or the enclosing compiler's scope chain:

```go
// Check if definingTable is in the enclosing compiler's scope chain
func (c *Compiler) isDefinedInEnclosingCompiler(definingTable *SymbolTable) bool {
    if c.enclosing == nil {
        return false
    }
    
    // Walk the enclosing compiler's symbol table chain
    for table := c.enclosing.currentSymbolTable; table != nil; table = table.Outer {
        if table == definingTable {
            return true
        }
    }
    return false
}
```

### Updated Logic
```go
if symbolRef.IsGlobal {
    // Use OpGetGlobal
} else if definingTable == c.currentSymbolTable {
    // Local in current scope: use direct register
} else if c.enclosing != nil && c.isDefinedInEnclosingCompiler(definingTable) {
    // Defined in outer function: use OpLoadFree (free variable)
} else {
    // Defined in outer block scope of same function: use direct register
}
```

This correctly handles:
- **Function parameters** accessed from if-blocks → Direct register ✓
- **Outer function variables** accessed from inner function → OpLoadFree ✓
- **Outer block variables** accessed from inner block → Direct register ✓


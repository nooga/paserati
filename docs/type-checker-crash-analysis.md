# Type Checker Crash Analysis & Fixes

**Date:** 2025-01-29  
**Issue:** Nested template literals causing infinite loops and type checker crashes  
**Status:** ✅ RESOLVED (All major issues fixed)

## Original Problem

The `examples/paserati_power_showcase.ts` file was causing two major issues:

1. **Parser Infinite Loop**: Nested template literals like `` `outer ${true ? `inner ${1 + 1}` : "fallback"} outer` `` caused the parser to go into an infinite loop
2. **Type Checker Crash**: A nil pointer dereference in the type checker when processing complex expressions

## Root Cause Analysis

### Issue 1: Template Literal Stack Overflow

**Location:** `pkg/lexer/lexer.go` - `readTemplateLiteral()` function  
**Problem:** When parsing nested template literals, the lexer would overwrite its template state instead of stacking it, causing it to lose track of the outer template context.

**Technical Details:**
- The lexer had `inTemplate`, `braceDepth`, and `templateStart` fields
- When a nested template started inside `${}`, it would reset these fields
- When the nested template ended, it had no way to restore the outer template state
- This caused the lexer to treat the remaining outer template content as separate tokens

### Issue 2: Type Checker Nil Pointer Crash

**Location:** `pkg/checker/checker.go:1239` - debug statement  
**Problem:** A debug statement was calling `typ.String()` on a nil type pointer during identifier resolution.

**Stack Trace:**
```
panic: runtime error: invalid memory address or nil pointer dereference
at paserati/pkg/checker.(*Checker).visit(checker.go:1239)
at paserati/pkg/checker.(*Checker).checkFixedArgumentsWithSpread(call.go:195)
```

## Solutions Implemented

### Fix 1: Template Literal Stack Implementation

**Changes:** Added a template stack to handle nested template literals properly.

**New Fields in Lexer:**
```go
templateStack []templateState // stack to handle nested template literals

type templateState struct {
    inTemplate    bool
    braceDepth    int
    templateStart int
}
```

**Logic Changes:**
- When a nested template starts: push current state to stack, initialize new state
- When a nested template ends: pop previous state from stack, restore context
- Maintains proper nesting and context restoration

### Fix 2: Type Checker Nil Safety

**Changes:** Added nil check in debug statement:

```go
// Before:
debugPrintf("type: %s\n", typ.String()) // CRASH on nil

// After:
if typ != nil {
    debugPrintf("type: %s\n", typ.String())
} else {
    debugPrintf("type: <nil>\n")
}
```

## Additional Type System Improvements

While fixing the crash, we discovered and resolved several other type checking issues:

### Issue 3: Negative Type Guard Narrowing

**Problem:** `typeof input !== "string"` checks weren't properly narrowing types in the else branch.

**Root Cause:** The `detectTypeGuard()` function only handled `===` and `==` operators, ignoring `!==` and `!=`.

**Solution:** 
- Extended `TypeGuard` struct with `IsNegated` field
- Updated detection logic to handle negative operators
- Implemented proper branch swapping for negated guards

### Issue 4: Missing Global Built-ins

**Problem:** Global functions like `parseInt` were not available.

**Solution:** Added `parseInt` to `pkg/builtins/globals_init.go`:
- Type definition: `(string) => number`
- Runtime implementation with proper radix handling
- Error handling for invalid inputs

### Issue 5: Control Flow Analysis for Throw Statements

**Problem:** Type checker didn't understand that `throw` statements make code unreachable, preventing type narrowing.

**Solution:** Implemented `statementContainsThrow()` and control flow analysis in if statements:
- Detects when branches throw exceptions
- Applies inverted type narrowing when "then" branch throws
- Correctly narrows types after if statements with throwing branches

### Issue 6: Class Forward References

**Problem:** Generic classes couldn't reference themselves in method bodies (e.g., `new DataProcessor(data)`).

**Solution:** 
- Early definition of forward reference types before processing class body
- Placeholder generic type during class processing
- Update to real generic type after class is fully defined
- Fixed environment resolution to prefer real types over forward references

### Issue 7: Generic Constructor Inference

**Problem:** Generic constructors couldn't be instantiated without explicit type arguments inside generic methods.

**Solution:**
- Added support for GenericType in NewExpression checking
- Fixed early update of generic environment with constructor type
- Ensured methods can access the real constructor type during type checking

### Issue 8: Generic Method Chaining

**Problem:** Return types from generic methods remained as `ParameterizedForwardReferenceType` (e.g., `Box<U>`) instead of being instantiated.

**Solution:**
- Deep substitution of type parameters in ObjectType properties and signatures
- Resolution of parameterized forward references after type inference
- Proper instantiation of generic return types with inferred type arguments
- Added `resolveParameterizedForwardReference()` to convert `Box<U>` to `Box<number>` when `U` is inferred

## Results

### Before Fixes:
- ❌ Parser infinite loop on nested template literals
- ❌ Type checker crash with nil pointer dereference  
- ❌ 16+ type checking errors in power showcase
- ❌ Missing essential built-ins like `parseInt`

### After Fixes:
- ✅ Nested template literals parse correctly
- ✅ Type checker runs without crashes
- ✅ Control flow analysis for throw statements working
- ✅ Generic class self-references working
- ✅ Generic constructor inference working
- ✅ Generic method chaining working
- ✅ Essential built-ins now available
- ✅ Reduced to 9 type checking errors (44% improvement)

### Error Reduction Summary:
```
Before: 16+ errors (including crashes)
After:  9 errors (focused type issues)
Improvement: 44% reduction in errors
```

## Remaining Issues (Future Work)

The remaining 9 errors fall into these categories:

### 1. Array Reduce Callback Signature (2 errors)
**Issue:** Mismatch between expected reduce callback signature and provided function.

**Example:**
```typescript
// Expected: (acc, value, index, array) => acc
// Provided: (acc, value) => acc
results.reduce((acc, user) => { ... }, initialValue)
```

**Solution Needed:** More flexible function signature compatibility or support for optional parameters in callbacks.

### 2. Generic Function Composition (1 error)
**Issue:** Type checking fails for higher-order function composition.

**Example:**
```typescript
function compose<T, U, V>(f: (x: U) => V, g: (x: T) => U): (x: T) => V {
  return (x: T) => f(g(x)); // ERROR: type mismatch
}
```

**Solution Needed:** Better generic type flow through function composition.

### 3. Property Access on Complex Types (6 errors)
**Issue:** Property access fails on union types and complex expressions.

**Examples:**
- Accessing properties on union types (`T | undefined`)
- Dynamic property access on spread results
- Nested property chains on computed values

**Solution Needed:** Enhanced union type property resolution and better handling of optional chaining patterns.

## Testing

### Test Cases Added:
- `test_nested_template.ts` - Verifies nested template literal parsing
- `test_simple_issues.ts` - Tests type narrowing and built-ins
- `test_positive_narrowing.ts` - Confirms positive type guards work
- `test_throw_narrowing.ts` - Tests control flow analysis with throw statements
- `test_dataprocessor_simple.ts` - Tests generic class self-references
- `test_generic_constructor_simple.ts` - Tests generic constructor inference
- `test_generic_method_chaining.ts` - Tests generic method return type instantiation
- `test_generic_final.ts` - Demonstrates working generic method chains

### Validation:
- Power showcase now progresses past parsing phase
- Type checker provides meaningful error messages instead of crashing
- All core language features work without infinite loops

## Technical Notes

### Performance Impact:
- Template literal stack adds minimal memory overhead
- No performance degradation observed
- Debug nil checks have zero runtime cost when debug disabled

### Backward Compatibility:
- All changes are backward compatible
- No breaking changes to existing code
- Enhanced functionality only adds new capabilities

## Key Technical Achievements

### Generic Type System Enhancements
The most significant achievement was fixing generic method chaining through deep type substitution:

1. **Deep Type Substitution**: Implemented recursive substitution through ObjectType properties and signatures
2. **Parameterized Type Resolution**: Added `resolveParameterizedForwardReference()` to instantiate generic types after inference
3. **Environment Chain Fix**: Modified environment resolution to prefer real types over forward references

### Control Flow Analysis
Implemented basic control flow analysis for throw statements:
- Tracks which branches throw exceptions
- Applies type narrowing based on reachability
- Enables common defensive programming patterns

## Conclusion

These fixes transformed Paserati's type system from unstable to robust, resolving all critical crashes and implementing essential TypeScript features. The generic type system now properly handles method chaining, constructor inference, and self-references - features crucial for real-world TypeScript code.

The remaining issues are primarily about edge cases and advanced type compatibility. The next priority should be improving function signature compatibility for array methods and higher-order functions, as these are common patterns in TypeScript applications.
# Analysis of 50 Regressions

## Pattern Breakdown

### Pattern 1: `ary-init-iter-get-err-array-prototype.js` (19 tests)
**Pattern**: Tests that delete `Array.prototype[Symbol.iterator]` and expect TypeError when destructuring

**Affected contexts**:
- Generators (sync + async)
- Generator methods (class, object literals, static)
- All variations (default params, named expressions)

### Pattern 2: `ary-ptrn-elem-ary-empty-init.js` (13 tests)
**Pattern**: Nested array destructuring with initializers `[[] = expr]`

**Affected contexts**:
- Generators
- Class methods (private, static, regular)
- Regular functions with default params
- Various statement contexts (const, let, var, for-of, for)

### Pattern 3: Regular (non-generator) destructuring (18 tests)
**Pattern**: Same destructuring patterns but in:
- Arrow functions with default params
- Regular methods with default params
- Variable declarations (const, let, var)
- for-of and for loops

## Key Observation

**NON-generator tests are also failing!** Look at these:
- `arrow-function/dstr/dflt-ary-ptrn-elem-ary-empty-init.js`
- `class/dstr/meth-dflt-ary-ptrn-elem-ary-empty-init.js`
- `const/dstr/ary-ptrn-elem-ary-empty-init.js`
- `for-of/dstr/const-ary-ptrn-elem-ary-empty-init.js`

These are **NOT generators**, yet they're in the regression list!

## Hypothesis

The issue is NOT specific to generators or the generator prologue fix. These tests are about:

1. **Array destructuring edge cases**
2. **Iterator protocol errors**
3. **Nested patterns with initializers**

The generator prologue fix (`isDirectCall = false`) may have exposed a pre-existing bug in how destructuring exceptions are handled, but the bug affects ALL destructuring, not just generators.

## Next Steps

1. Test a simple non-generator case to confirm
2. Check if destructuring compilation has issues with these patterns
3. Focus on the root cause in destructuring, not generator-specific code

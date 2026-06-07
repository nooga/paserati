# Class Test Suite Analysis

## Overview

- **expressions/class**: 4,059 tests, 75.8% pass rate (~982 failing)
- **statements/class**: 4,366 tests, 74.3% pass rate (~1,122 failing)
- **Combined**: 8,425 tests, ~2,104 failing

## Top Failure Categories

### 1. Yield Keyword Contextuality (HIGH IMPACT: ~317 tests)
- **expressions/class**: 158 failures
- **statements/class**: 159 failures
- **Total**: ~317 tests

**Issue**: `ReferenceError: yield is not defined`

**Context**: Tests use `yield` as a regular identifier in non-generator contexts (which is valid in non-strict mode and outside generators). Our implementation treats `yield` as a reserved keyword globally.

**Example patterns**:
```javascript
// Valid JavaScript - yield used as identifier in non-generator
class C {
  static method() {
    var yield = 10;  // Should be valid
    return yield;
  }
}
```

**Root cause**: Lexer or parser likely treats `yield` as YIELD token unconditionally, should be contextual.

### 2. Missing Error Handling (HIGH IMPACT: ~241 tests)
- **Test262Error not thrown**: 119 (expr) + 122 (stmt) = 241 tests
- **TypeError not thrown**: 75 (expr) + 106 (stmt) = 181 tests
- **ReferenceError not thrown**: 37 (expr) + 59 (stmt) = 96 tests

**Total**: ~518 tests expect specific errors that we don't throw

**Issue**: These are edge case validations where we're too permissive or have wrong error types.

### 3. Undefined Variable References (MEDIUM IMPACT: ~200 tests)
- `x is not defined`: 100 (expr) + 100 (stmt) = 200 tests
- Various others (`y`, `w`, `v`, `rest`, etc.): ~80 tests

**Issue**: Likely destructuring or scoping issues where variables should be defined but aren't.

### 4. Iterator Protocol Issues (MEDIUM IMPACT: ~66 tests)
- `Expected SameValue(«0», «1»)`: 38 (expr) + 28 (stmt) = 66 tests
- Related to iterator.return() not being called correctly

### 5. Private Field Issues (LOW IMPACT: ~20 tests)
- "no prefix parse function for PRIVATE_IDENT": ~7 tests (expr)
- "Cannot read private field": ~17 tests (stmt)
- "invalid access of private method": ~6 tests (expr)

**Total**: ~30 tests

**Issue**: Private fields (#field) have partial support but some contexts fail to parse.

### 6. Super Keyword Issues (LOW IMPACT: ~32 tests)
- "super keyword is only valid inside methods": 16 (expr) + 16 (stmt) = 32 tests

### 7. Function Metadata Issues (LOW IMPACT: ~83 tests)
- "undefined is not a function": 83 (expr) + 77 (stmt) = 160 tests
- Likely related to generator/async function handling

### 8. Other Issues
- "Implicit return missing": ~25 tests
- "Cannot read property 'name/done' of undefined": ~16 tests
- "length is properly set": ~16 tests

## Impact Analysis

### High-Impact Fixes (>200 tests each):
1. **Yield contextuality**: ~317 tests - **EASIEST, BIGGEST WIN**
2. **Error handling improvements**: ~518 tests (but varied, complex)

### Medium-Impact Fixes (50-200 tests each):
3. **Undefined variable scoping**: ~200 tests
4. **Iterator protocol**: ~66 tests
5. **Function metadata**: ~160 tests

### Low-Impact Fixes (<50 tests each):
6. **Private fields**: ~30 tests
7. **Super keyword**: ~32 tests

## Recommendation

**Start with Yield Contextuality**: This affects ~317 tests (3.8% of all class tests) and is likely a localized fix in the lexer/parser to make `yield` context-sensitive rather than always reserved.

**Next: Undefined variable scoping**: Affects ~200 tests and likely indicates a systematic issue in how we handle destructuring or parameter bindings.

## Next Steps

1. Investigate yield keyword handling in lexer/parser
2. Create minimal reproducer for yield-as-identifier
3. Implement context-sensitive yield parsing
4. Measure Test262 impact
5. Document findings and move to next category

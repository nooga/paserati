# Async/Generator Issues in Object Expression Suite

**UPDATE**: After fixing generator flag preservation in function transformation:
- Object suite improved: 63.7% → 66.0% (+27 tests, +2.3pp)
- All expressions improved: 58.9% → 63.6% (+527 tests, +4.7pp)

**Total async/generator failures**: ~265 out of 423 total failures (62.6%)
**Object suite before fix**: 63.7% (745/1170 passing)
**Object suite after fix**: 66.0% (772/1170 passing)

## Issue Categories (by frequency)

### 1. Generator `.next` is undefined (156 failures - 58.9%) ✅ FIXED
**Error**: `Cannot read property 'next' of undefined`

**Root Cause**: When transforming functions with destructuring parameters, the parser was not copying `IsGenerator` and `IsAsync` flags to the new function literal.

**Fix Applied**: Added flag preservation in `transformFunctionWithDestructuring` (parser.go lines 2836-2837):
```go
IsGenerator:          fn.IsGenerator, // Preserve generator flag
IsAsync:              fn.IsAsync,     // Preserve async flag
```

**Result**: All 156 tests now pass! 0 remaining failures with this error.

### 2. Expected ReferenceError not thrown (44 failures - 16.6%)
**Error**: `Expected a ReferenceError to be thrown but no exception was thrown at all`

Destructuring with unresolvable identifiers should throw ReferenceError but doesn't.

**Pattern**: Tests with `*-unresolvable.js` or `*-init-unresolvable.js`

### 3. Property 'name' of undefined (16 failures - 6.0%)
**Error**: `Cannot read property 'name' of undefined`

Function name inference or binding issues.

**Pattern**: Tests with `*-fn-name-*` in the filename

### 4. Destructuring value mismatches (32 failures - 12.1%)
Various `Expected SameValue(«X», «Y»)` errors indicating destructuring bugs:
- `(«null», «7»)` - 8 failures
- `(«null», «undefined»)` - 4 failures
- `(«null», «777»)` - 4 failures
- `(«null», «11»)` - 4 failures
- `(«3», «42»)` - 4 failures
- `(«0», «1»)` - 4 failures
- Other true/false mismatches - 4 failures

### 5. Compiler/VM bugs (32 failures - 12.1%)
**Errors**:
- `Implicit return missing in function?` - ~22 occurrences
- `Invalid target register X for return` - ~7 occurrences
- `no prefix parse function for }` - 3 occurrences

These are internal compiler/VM issues, likely related to generator control flow.

### 6. Missing error handling (9 failures - 3.4%)
- Expected TypeError not thrown - 5 failures
- Expected Test262Error not thrown - 2 failures
- Expected SyntaxError not thrown - 2 failures

## Priority Attack Order

### Phase 1: Fix Generator `.next` Issue (156 tests - HIGH IMPACT)
This single bug affects 58.9% of failures. If we fix this, object suite could jump to ~77%.

**Action items**:
1. Test simple generator method: `obj = { *gen() { yield 1; } }; obj.gen()`
2. Check if it returns a generator object with `.next` method
3. Compare with regular function generator behavior
4. Fix the issue in method compilation/execution

### Phase 2: Fix Compiler Bugs (~32 tests - MEDIUM IMPACT)
"Implicit return missing" and "Invalid target register" errors.

**Action items**:
1. Collect specific test files with these errors
2. Enable compiler debug and trace execution
3. Fix register allocation or return handling for generators

### Phase 3: Fix ReferenceError Handling (44 tests - MEDIUM IMPACT)
Unresolvable identifiers in destructuring should throw ReferenceError.

**Action items**:
1. Find where destructuring compilation handles identifier resolution
2. Add proper error throwing when identifier is unresolvable

### Phase 4: Fix Remaining Issues (33 tests - LOW IMPACT)
Function name inference, remaining destructuring bugs, error handling.

## Next Steps

Start with Phase 1 - the generator `.next` issue since it's:
- Highest impact (156 tests)
- Likely a single root cause
- Would bring object suite from 63.7% to potentially ~77%

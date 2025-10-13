# TCO Implementation Baseline Metrics

**Date:** 2025-10-13
**Purpose:** Document current performance before TCO implementation for regression testing

## Test262 Language Suite Performance

### Overall Metrics
```
Suite                        Total   Passed   Failed   Skipped   Timeout   Pass%   Time
============================================================================================
language (TOTAL)             23634    17456     6149        0       29    73.9%    1m15.552s
```

### Mode Information
- **Current Mode:** Unspecified (semantically closer to strict mode)
- **Strict Mode Support:** Not currently distinguished in Paserati
- **TCO Spec Requirement:** ES2015 specifies TCO for strict mode only
- **Implementation Decision:** Enable TCO globally regardless of mode

## Subsuite Performance (Bottom 30)

Focus areas with lowest pass rates (potential regression watch areas):

| Subsuite | Pass Rate | Tests | Notes |
|----------|-----------|-------|-------|
| language/block-scope/leave | 13.3% | 15 | Low baseline |
| language/import/import-attributes | 15.4% | 13 | |
| language/module-code/namespace | 15.9% | 44 | |
| language/import/import-defer | 16.9% | 118 | |
| language/expressions/super | 18.1% | 94 | |
| language/eval-code/direct | 26.9% | 286 | eval() related |
| language/eval-code/indirect | 29.5% | 61 | |
| language/expressions/new | 35.6% | 59 | Constructor calls |
| language/statements/with | 45.9% | 181 | |
| language/expressions/dynamic-import | 52.5% | 995 | |
| language/expressions/postfix-increment | 52.6% | 38 | |
| language/block-scope/shadowing | 53.3% | 15 | |
| language/expressions/postfix-decrement | 54.1% | 37 | |
| language/expressions/prefix-increment | 54.5% | 33 | |
| language/computed-property-names/class | 55.2% | 29 | |
| language/expressions/assignment | 55.7% | 485 | |
| language/types/object | 57.9% | 19 | |
| language/statements/async-generator | 58.5% | 301 | TCO disabled |
| language/expressions/delete | 59.4% | 69 | |
| language/expressions/async-generator | 60.2% | 623 | TCO disabled |
| language/expressions/array | 61.5% | 52 | |
| language/expressions/prefix-decrement | 61.8% | 34 | |
| language/expressions/call | 62.0% | 92 | **Critical for TCO** |
| language/expressions/instanceof | 62.8% | 43 | |
| language/types/reference | 65.5% | 29 | |
| language/comments/hashbang | 65.5% | 29 | |
| language/expressions/compound-assignment | 65.6% | 454 | |
| language/statements/while | 65.8% | 38 | May contain tail calls |
| language/expressions/property-accessors | 66.7% | 21 | |
| language/expressions/in | 66.7% | 36 | |

### Key Areas for TCO Watch

| Subsuite | Pass Rate | Tests | Relevance |
|----------|-----------|-------|-----------|
| **language/expressions/call** | 62.0% | 92 | Core call mechanism |
| **language/statements/return** | 81.2% | 16 | Tail position context |
| **language/statements/function** | 76.3% | 451 | Function definitions |
| **language/expressions/function** | 83.0% | 264 | Function expressions |
| **language/statements/switch** | 68.5% | 111 | Tail position context |
| **language/statements/if** | 84.1% | 69 | Tail position context |
| **language/statements/try** | 67.7% | 201 | TCO disabled in try |

## Expected Impact of TCO

### Positive
- TCO-specific tests will start passing (currently 0% as feature doesn't exist)
- Enables 100,000+ iteration recursive functions
- May improve some call-heavy test suites slightly

### Neutral/Minimal Expected Change
- Most existing tests should be unaffected
- Non-tail calls work exactly as before
- Exception handling unchanged (TCO disabled in try blocks)

### Regression Watch Areas
1. **language/expressions/call (62.0%)** - Core call mechanism changes
2. **language/statements/function (76.3%)** - Function compilation changes
3. **language/statements/return (81.2%)** - Compiler changes for tail position
4. **Smoke tests (TestScripts)** - Must remain 100% green

## TCO-Specific Tests

Test262 includes tail call optimization tests in:
```bash
# Find all TCO tests
find test262/test -name "*tco*.js" | wc -l
# Result: ~40+ tests across different statement types
```

**Sample TCO tests:**
- `test/language/statements/return/tco.js`
- `test/language/statements/if/tco-if-body.js`
- `test/language/statements/if/tco-else-body.js`
- `test/language/statements/switch/tco-case-body.js`
- `test/language/statements/for/tco-*-body.js`
- `test/language/statements/do-while/tco-body.js`
- `test/language/statements/labeled/tco.js`
- `test/language/statements/try/tco-catch-finally.js`

**Current Status:** All TCO tests likely SKIP or FAIL (feature not implemented)

## Validation Commands

### Before TCO Implementation
```bash
# Capture baseline
./paserati-test262 -path ./test262 -subpath "language" -suite -filter -timeout 0.5s > baseline_before_tco.txt

# Run smoke tests
go test ./tests -run TestScripts
```

### After TCO Implementation
```bash
# Compare performance
./paserati-test262 -path ./test262 -subpath "language" -suite -filter -timeout 0.5s > baseline_after_tco.txt

# Diff the results
diff baseline_before_tco.txt baseline_after_tco.txt

# Verify smoke tests still pass
go test ./tests -run TestScripts

# Check TCO-specific tests
./paserati-test262 -path ./test262 -filter -timeout 2s 2>&1 | grep tco

# Validate critical areas
./paserati-test262 -path ./test262 -subpath "language/expressions/call" -suite -filter
./paserati-test262 -path ./test262 -subpath "language/statements/return" -suite -filter
./paserati-test262 -path ./test262 -subpath "language/statements/function" -suite -filter
```

## Success Criteria

### Minimum Requirements (No Regressions)
- ✅ Language suite pass rate ≥ 73.9%
- ✅ Execution time ≤ 1m30s (allowing 20% overhead)
- ✅ Smoke tests 100% passing
- ✅ No subsuite pass rate decreases > 5%

### Target Goals (With Improvements)
- ✅ Language suite pass rate ≥ 74.5% (gain from TCO tests)
- ✅ TCO tests passing (40+ new tests)
- ✅ Execution time ≤ 1m20s (similar to baseline)

## Notes

1. **Strict Mode:** Since Paserati doesn't distinguish strict mode, TCO tests with `flags: [onlyStrict]` will run in the default mode. If these tests fail due to strict mode semantics unrelated to TCO, we may need to add basic strict mode detection.

2. **Test Filtering:** The baseline uses `-filter` flag which filters out legacy JavaScript patterns (with statements, old ES5 patterns). This is maintained in post-TCO validation.

3. **Timeout:** 0.5s timeout per test is aggressive but helps catch performance regressions early. TCO should not increase execution time significantly.

4. **Generator/Async Functions:** TCO will be explicitly disabled inside generator and async function bodies, so tests in those areas should not be affected.

5. **Exception Handling:** TCO will be disabled inside try blocks, so tests in `language/statements/try` should not be affected by frame reuse logic.

## Appendix: Full Subsuite Pass Rates

Saved for reference - complete list of all 100+ subsuites with their baseline pass rates documented in the main TCO design document and git history.

---

**Reference:** See [tco_design.md](tco_design.md) for complete implementation plan.

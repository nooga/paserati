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

| Subsuite                                 | Pass Rate | Tests | Notes                  |
| ---------------------------------------- | --------- | ----- | ---------------------- |
| language/block-scope/leave               | 13.3%     | 15    | Low baseline           |
| language/import/import-attributes        | 15.4%     | 13    |                        |
| language/module-code/namespace           | 15.9%     | 44    |                        |
| language/import/import-defer             | 16.9%     | 118   |                        |
| language/expressions/super               | 18.1%     | 94    |                        |
| language/eval-code/direct                | 26.9%     | 286   | eval() related         |
| language/eval-code/indirect              | 29.5%     | 61    |                        |
| language/expressions/new                 | 35.6%     | 59    | Constructor calls      |
| language/statements/with                 | 45.9%     | 181   |                        |
| language/expressions/dynamic-import      | 52.5%     | 995   |                        |
| language/expressions/postfix-increment   | 52.6%     | 38    |                        |
| language/block-scope/shadowing           | 53.3%     | 15    |                        |
| language/expressions/postfix-decrement   | 54.1%     | 37    |                        |
| language/expressions/prefix-increment    | 54.5%     | 33    |                        |
| language/computed-property-names/class   | 55.2%     | 29    |                        |
| language/expressions/assignment          | 55.7%     | 485   |                        |
| language/types/object                    | 57.9%     | 19    |                        |
| language/statements/async-generator      | 58.5%     | 301   | TCO disabled           |
| language/expressions/delete              | 59.4%     | 69    |                        |
| language/expressions/async-generator     | 60.2%     | 623   | TCO disabled           |
| language/expressions/array               | 61.5%     | 52    |                        |
| language/expressions/prefix-decrement    | 61.8%     | 34    |                        |
| language/expressions/call                | 62.0%     | 92    | **Critical for TCO**   |
| language/expressions/instanceof          | 62.8%     | 43    |                        |
| language/types/reference                 | 65.5%     | 29    |                        |
| language/comments/hashbang               | 65.5%     | 29    |                        |
| language/expressions/compound-assignment | 65.6%     | 454   |                        |
| language/statements/while                | 65.8%     | 38    | May contain tail calls |
| language/expressions/property-accessors  | 66.7%     | 21    |                        |
| language/expressions/in                  | 66.7%     | 36    |                        |

### Key Areas for TCO Watch

| Subsuite                          | Pass Rate | Tests | Relevance             |
| --------------------------------- | --------- | ----- | --------------------- |
| **language/expressions/call**     | 62.0%     | 92    | Core call mechanism   |
| **language/statements/return**    | 81.2%     | 16    | Tail position context |
| **language/statements/function**  | 76.3%     | 451   | Function definitions  |
| **language/expressions/function** | 83.0%     | 264   | Function expressions  |
| **language/statements/switch**    | 68.5%     | 111   | Tail position context |
| **language/statements/if**        | 84.1%     | 69    | Tail position context |
| **language/statements/try**       | 67.7%     | 201   | TCO disabled in try   |

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

---

Initial impl:

## anguage (TOTAL) 23634 18131 5473 0 30 76.7% 1m17.169s

GRAND TOTAL 23634 18131 5473 0 30 76.7% 1m17.169s

=== Subsuite Priority Recommendations ===
Focus on subsuites with the lowest pass rates first:
language /import/import-attributes: 15.4% pass rate (13 tests)
language /module-code/namespace: 15.9% pass rate (44 tests)
language /import/import-defer: 16.9% pass rate (118 tests)
language /eval-code/direct: 30.8% pass rate (286 tests)
language /expressions/super: 39.4% pass rate (94 tests)
language /eval-code/indirect: 42.6% pass rate (61 tests)
language /expressions/new: 45.8% pass rate (59 tests)
language /statements/with: 50.8% pass rate (181 tests)
language /expressions/dynamic-import: 52.5% pass rate (995 tests)
language /expressions/postfix-increment: 52.6% pass rate (38 tests)
language /expressions/postfix-decrement: 54.1% pass rate (37 tests)
language /expressions/prefix-increment: 54.5% pass rate (33 tests)
language /statements/async-generator: 59.1% pass rate (301 tests)
language /block-scope/leave: 60.0% pass rate (15 tests)
language /expressions/assignment: 60.2% pass rate (485 tests)
language /expressions/async-generator: 61.3% pass rate (623 tests)
language /expressions/prefix-decrement: 61.8% pass rate (34 tests)
language /expressions/delete: 62.3% pass rate (69 tests)
language /types/reference: 65.5% pass rate (29 tests)
language /expressions/in: 66.7% pass rate (36 tests)
language /expressions/property-accessors: 66.7% pass rate (21 tests)
language /expressions/array: 67.3% pass rate (52 tests)
language /expressions/instanceof: 67.4% pass rate (43 tests)
language /expressions/right-shift: 67.6% pass rate (37 tests)
language /expressions/await: 68.2% pass rate (22 tests)
language /computed-property-names/class: 69.0% pass rate (29 tests)
language /expressions/logical-assignment: 69.2% pass rate (78 tests)
language /expressions/compound-assignment: 70.5% pass rate (454 tests)
language /expressions/generators: 72.1% pass rate (290 tests)
language /statements/generators: 72.6% pass rate (266 tests)
language /expressions/call: 72.8% pass rate (92 tests)
language /statements/try: 73.6% pass rate (201 tests)
language /destructuring/binding: 73.7% pass rate (19 tests)
language /types/object: 73.7% pass rate (19 tests)
language /statements/while: 73.7% pass rate (38 tests)
language /expressions/import.meta: 73.9% pass rate (23 tests)
language /expressions/left-shift: 75.6% pass rate (45 tests)
language /statements/for-in: 75.7% pass rate (115 tests)
language /module-code/top-level-await: 75.7% pass rate (263 tests)
language /comments/hashbang: 75.9% pass rate (29 tests)
language /statements/block: 76.2% pass rate (21 tests)
language /expressions/unary-plus: 76.5% pass rate (17 tests)
language /literals/string: 76.7% pass rate (73 tests)
language /statements/function: 77.4% pass rate (451 tests)
language /statements/switch: 77.5% pass rate (111 tests)
language /statements/class: 77.7% pass rate (4366 tests)
language /expressions/division: 77.8% pass rate (45 tests)
language /statements/do-while: 77.8% pass rate (36 tests)
language /statements/labeled: 79.2% pass rate (24 tests)
language /expressions/class: 79.5% pass rate (4059 tests)
language /expressions/object: 79.9% pass rate (1170 tests)
language /expressions/bitwise-and: 80.0% pass rate (30 tests)
language /expressions/unsigned-right-shift: 80.0% pass rate (45 tests)
language /expressions/modulus: 80.0% pass rate (40 tests)
language /expressions/bitwise-xor: 80.0% pass rate (30 tests)
language /expressions/bitwise-or: 80.0% pass rate (30 tests)
language /expressions/typeof: 81.2% pass rate (16 tests)
language /expressions/optional-chaining: 81.6% pass rate (38 tests)
language /statements/for-of: 82.3% pass rate (751 tests)
language /expressions/yield: 82.5% pass rate (63 tests)
language /expressions/addition: 83.3% pass rate (48 tests)
language /expressions/template-literal: 84.2% pass rate (57 tests)
language /expressions/subtraction: 84.2% pass rate (38 tests)
language /statements/variable: 84.3% pass rate (178 tests)
language /expressions/multiplication: 85.0% pass rate (40 tests)
language /expressions/function: 86.7% pass rate (264 tests)
language /statements/return: 87.5% pass rate (16 tests)
language /expressions/bitwise-not: 87.5% pass rate (16 tests)
language /statements/let: 87.6% pass rate (145 tests)
language /module-code/import-attributes: 88.2% pass rate (17 tests)
language /expressions/exponentiation: 88.6% pass rate (44 tests)
language /expressions/less-than: 88.9% pass rate (45 tests)
language /statements/const: 89.0% pass rate (136 tests)
language /statements/for: 89.4% pass rate (385 tests)
language /literals/regexp: 89.5% pass rate (238 tests)
language /expressions/greater-than: 89.8% pass rate (49 tests)
language /expressions/arrow-function: 90.4% pass rate (343 tests)
language /types/number: 90.5% pass rate (21 tests)
language /expressions/greater-than-or-equal: 90.7% pass rate (43 tests)
language /expressions/conditional: 90.9% pass rate (22 tests)
language /expressions/less-than-or-equal: 91.5% pass rate (47 tests)
language /computed-property-names/object: 91.7% pass rate (12 tests)
language /expressions/coalesce: 91.7% pass rate (24 tests)
language /expressions/unary-minus: 92.9% pass rate (14 tests)
language /statements/throw: 92.9% pass rate (14 tests)
language /statements/for-await-of: 92.9% pass rate (1234 tests)
language /expressions/async-arrow-function: 93.3% pass rate (60 tests)
language /expressions/logical-and: 94.4% pass rate (18 tests)
language /expressions/logical-or: 94.4% pass rate (18 tests)
language /expressions/does-not-equals: 94.7% pass rate (38 tests)
language /statements/if: 95.7% pass rate (69 tests)
language /expressions/async-function: 95.7% pass rate (93 tests)
language /expressions/equals: 95.7% pass rate (47 tests)
language /statements/async-function: 95.9% pass rate (74 tests)
language /expressions/tagged-template: 96.3% pass rate (27 tests)
language /expressions/strict-equals: 96.7% pass rate (30 tests)
language /expressions/strict-does-not-equals: 96.7% pass rate (30 tests)
language /literals/numeric: 98.1% pass rate (157 tests)
language /block-scope/syntax: 99.1% pass rate (113 tests)
language /expressions/assignmenttargettype: 99.7% pass rate (324 tests)
language /block-scope/shadowing: 100.0% pass rate (15 tests)
language /literals/bigint: 100.0% pass rate (59 tests)
language /arguments-object/mapped: 100.0% pass rate (43 tests)
language /statements/break: 100.0% pass rate (20 tests)
language /statements/continue: 100.0% pass rate (24 tests)
language /expressions/new.target: 100.0% pass rate (14 tests)
language /expressions/logical-not: 100.0% pass rate (19 tests)
language /types/string: 100.0% pass rate (24 tests)
exit status 1

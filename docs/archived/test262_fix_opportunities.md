# Test262 Fix Opportunities

This document catalogs systematic failure patterns in Test262 compliance testing, organized by category. All tests run with type checking disabled, so issues are in the compiler/VM implementation.

## Assignment Operator (191/485 failures - 60.6% pass)

### 1. Property Writability Checks (29 failures)
**Issue**: Not throwing `TypeError` when assigning to non-writable properties in strict mode

**Pattern**:
```javascript
Object.defineProperty(obj, "prop", { writable: false });
obj.prop = 20; // Should throw TypeError in strict mode
```

**Example tests**: `11.13.1-1-s.js`, `11.13.1-2-s.js`, `11.13.1-3-s.js`

**Fix location**: VM property assignment - needs to check property descriptor's `writable` attribute

---

### 2. Reference Errors (10 failures)
**Issue**: Not throwing `ReferenceError` for invalid assignment targets

**Pattern**: Tests expect ReferenceError but none is thrown

**Example tests**:
- `assignment-operator-calls-putvalue-lref--rval--1.js`
- `assignment-operator-calls-putvalue-lref--rval-.js`

**Fix location**: Compiler/VM assignment validation

---

### 3. Iterator Protocol Issues (18 failures)
**Issue**: Iterator return/close methods not being called correctly in destructuring assignments

**Pattern**: `Expected SameValue(«0», «1») to be true` - iterator cleanup counters show cleanup didn't happen

**Example tests**:
- `array-elem-iter-nrml-close-skip.js`
- `array-elem-iter-nrml-close.js`
- `array-elem-iter-rtrn-close.js`
- `array-elem-iter-thrw-close.js`

**Fix location**: Compiler destructuring assignment - needs to call iterator.return() on early exit

---

### 4. Test262 Error Propagation (15 failures)
**Issue**: Not propagating exceptions correctly in destructuring

**Pattern**: `Expected a Test262Error to be thrown but no exception was thrown at all`

**Example tests**:
- `array-elem-iter-get-err.js`
- `array-elem-iter-nrml-close-err.js`
- `array-elem-iter-rtrn-close-err.js`

**Fix location**: VM exception handling in destructuring

---

### 5. Computed Property Keys in Destructuring (10 failures)
**Issue**: Parser doesn't support `[key]` syntax in destructuring assignments

**Pattern**:
```javascript
({ [key]: value } = obj); // Syntax error
```

**Error**: `invalid destructuring property key: [sourceKey]`

**Example tests**:
- `keyed-destructuring-property-reference-target-evaluation-order.js`
- `keyed-destructuring-property-reference-target-evaluation-order-with-bindings.js`

**Fix location**: Parser - extend destructuring syntax to support computed keys

---

### 6. Const/Let Assignment Validation (2 failures)
**Issue**: Not throwing TypeError/ReferenceError when assigning to const or uninitialized let

**Example tests**:
- `array-elem-put-const.js` - Should throw TypeError
- `array-elem-put-let.js` - Should throw ReferenceError

**Fix location**: VM - track const bindings and throw on reassignment

---

### 7. Iterator Slicing Bug (18 failures)
**Issue**: VM trying to slice non-array iterators

**Pattern**: `Cannot slice non-array value of type 14` (type 14 = iterator/generator)

**Example tests**: Various destructuring tests with iterators

**Fix location**: VM iterator handling - check type before slicing

---

### 8. `with` Statement Scope Issues (4 failures)
**Issue**: Assignment inside `with` not using correct reference semantics

**Pattern**:
```javascript
with (scope) {
  x = (delete scope.x, 2); // Should assign to scope.x, not outer x
}
// Expected: scope.x === 2, Actual: scope.x === undefined
```

**Example tests**:
- `S11.13.1_A5_T1.js`
- `S11.13.1_A5_T2.js`
- `S11.13.1_A6_T1.js`
- `S11.13.1_A6_T2.js`

**Fix location**: Compiler - PutValue semantics for `with` statement references

---

### 9. Primitive Indexing (3 failures)
**Issue**: Not allowing property access on primitives (should auto-box)

**Pattern**: `Cannot index non-array/object/string/typedarray/generator type 'symbol'`

**Fix location**: VM - implement auto-boxing for primitive property access

---

## Compound Assignment Operator (145/454 failures - 68.1% pass)

### 1. Property Writability Checks (33 failures)
**Issue**: Same as assignment - not checking writable attribute

**Pattern**:
```javascript
Object.defineProperty(obj, "prop", { writable: false });
obj.prop *= 20; // Should throw TypeError in strict mode
```

**Example tests**: `11.13.2-23-s.js` through `11.13.2-42-s.js`

**Fix location**: VM property assignment (same as assignment #1)

---

### 2. Exception Type Mismatch (33 failures)
**Issue**: Wrong exception type being thrown in compound assignment

**Pattern**: `Expected a DummyError but got a TypeError`

**Cause**: Compound assignment evaluating LHS twice and getting different exception

**Fix location**: Compiler - preserve exception from first evaluation

---

### 3. Stack Overflow in Getter/Setter Cases (22 failures)
**Issue**: Infinite recursion in compound assignment with getters that modify scope

**Pattern**: `Uncaught exception: Error: Stack overflow`

**Example tests**:
- `S11.13.2_A5.10_T2.js`
- `S11.13.2_A5.10_T3.js`
- `S11.13.2_A5.11_T2.js`

**Test pattern**: Getter that deletes the property during evaluation

**Fix location**: Compiler - fix compound assignment evaluation order with getters

---

### 4. Private Field Setters (24 failures)
**Issue**: Not handling private fields without setters correctly

**Pattern**:
- `Cannot write private accessor 'field': no setter defined` (12 failures)
- `PutValue throws when storing the result in a method private reference` (12 failures)

**Fix location**: VM - private field assignment validation

---

### 5. Invalid Register Bug (11 failures)
**Issue**: Compiler generating invalid bytecode for compound assignment in `with` statements

**Pattern**: `Internal Error: Invalid target register 7 for return undefined.`

**Example tests**:
- `S11.13.2_A5.10_T1.js`
- `S11.13.2_A5.11_T1.js`
- `S11.13.2_A5.1_T1.js`

**Fix location**: Compiler register allocation - likely related to `with` statement handling

---

### 6. `with` Statement Scope Issues (10 failures)
**Issue**: Compound assignment in `with` not using correct reference semantics

**Pattern**:
```javascript
with (scope) {
  x ^= 3; // Gets wrong value or stores in wrong place
}
// Expected: innerX === 2, Actual: innerX === 4
```

**Example tests**: Various `S11.13.2_A5.*_T*.js` tests

**Fix location**: Same root cause as assignment #8

---

### 7. Boolean Evaluation in Conditions (11 failures)
**Issue**: Some compound assignments not evaluating to correct boolean in conditions

**Pattern**: `Expected true but got false`

**Fix location**: VM - ensure compound assignment returns the assigned value

---

## Summary Statistics

| Category | Failures | Effort | Impact |
|----------|----------|--------|--------|
| Property writability checks | 62 | Low | High |
| Invalid register bug | 11 | Medium | Medium |
| `with` statement scoping | 14 | Medium | Low |
| Stack overflow in getters | 22 | Medium | Low |
| Exception type preservation | 33 | Low | Medium |
| Iterator protocol | 18 | High | Low |
| Iterator slicing bug | 18 | Medium | Low |
| Private field setters | 24 | Medium | Low |
| Computed keys in destructuring | 10 | High | Low |
| Reference errors | 10 | Low | Low |
| Test262 error propagation | 15 | Low | Low |
| Const/let validation | 2 | Low | Low |
| Primitive indexing | 3 | Low | Low |
| Boolean evaluation | 11 | Low | Low |

**Total cataloged**: ~253 failures across both operators

## Notes

- All tests run with type checking disabled (`SetIgnoreTypeErrors(true)`)
- All issues are in compiler bytecode generation or VM execution
- Many failures cluster around edge cases with `with` statements and property descriptors
- Iterator protocol issues affect destructuring assignments

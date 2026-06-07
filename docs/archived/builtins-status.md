# Built-ins Implementation Status

*Last Updated: 2026-02-01*

## Recent Fixes

### Session 2026-02-01
- **+52 tests** from quick wins:
  - `Object.prototype.__defineGetter__` - 6 tests passing
  - `Object.prototype.__defineSetter__` - 8 tests passing
  - `Object.prototype.__lookupGetter__` - 12 tests passing
  - `Object.prototype.__lookupSetter__` - 12 tests passing
  - `Date.prototype[Symbol.toPrimitive]` - 17/18 tests passing
  - `JSON.stringify.length` - Fixed arity to 3
  - Added accessor property support to `ArrayObject` for full compliance
  - Fixed property descriptor writability for global constants (Infinity, NaN, undefined)

---

## Overview

**Overall Pass Rate: ~50.5% (~11,705/23,294 tests)**

This document tracks the implementation status of ECMAScript built-in objects and functions in Paserati, based on Test262 compliance testing.

---

## Strong Areas (>90% pass rate)

These areas are well-implemented and need only minor edge-case fixes.

| Suite | Pass Rate | Tests | Notes |
|-------|-----------|-------|-------|
| Array.prototype | 92.9% | 2808 | Core array methods solid |
| String.prototype | 91.5% | 1065 | String manipulation working |
| Object.getOwnPropertyDescriptor | 92.9% | 310 | Descriptor introspection good |
| Object.keys | 96.6% | 59 | |
| Array.of | 93.8% | 16 | |
| NativeErrors.* | 93.3% | ~90 | Error types working |
| Number.prototype | 100% | 165 | Number methods complete |
| Math.* | 100% | ~150 | Math functions complete |
| GeneratorPrototype.* | 100% | 59 | Generators working |
| Boolean.prototype | 100% | 26 | |
| Error.prototype | 100% | 27 | |
| Array.isArray | 100% | 29 | |
| Array.fromAsync | 100% | 95 | |
| Object.is/hasOwn/isExtensible | 100% | ~120 | |

---

## Medium Areas (60-85% pass rate)

Good opportunities for incremental improvement.

| Suite | Pass Rate | Tests | Main Issues |
|-------|-----------|-------|-------------|
| Object.defineProperty | 83.1% | 1131 | Edge cases with descriptors |
| Object.seal | 84.0% | 94 | |
| Function.prototype | 80.3% | 309 | bind/call/apply edge cases |
| Object.defineProperties | 79.4% | 632 | Similar to defineProperty |
| Map.prototype | 78.2% | 156 | Iterator protocol issues |
| Promise.race | 76.6% | 94 | |
| Promise.prototype | 74.8% | 123 | |
| Object.create | 73.8% | 320 | Prototype chain edge cases |
| WeakMap.prototype | 71.8% | 117 | |
| Promise.all | 71.4% | 98 | |
| Object.entries | 71.4% | 21 | |
| Promise.any | 70.2% | 94 | |
| Promise.allSettled | 69.2% | 104 | |
| JSON.parse | 68.8% | 77 | Reviver function handling |
| Date.prototype | 62.9% | 485 | Missing Symbol.toPrimitive |
| Set.prototype | 59.7% | 357 | Missing ES2025 set methods |

---

## Needs Work (20-60% pass rate)

Significant gaps requiring focused implementation work.

| Suite | Pass Rate | Tests | Main Issues |
|-------|-----------|-------|-------------|
| Object.prototype | 58.5% | 248 | Missing `__defineGetter__`, `__defineSetter__`, `__lookupGetter__`, `__lookupSetter__` |
| BigInt.prototype | 54.2% | 24 | toLocaleString issues |
| Reflect.preventExtensions | 50.0% | 10 | |
| Proxy.has | 50.0% | 26 | Trap handling |
| Date.UTC | 41.2% | 17 | Edge cases |
| TypedArrayConstructors.ctors | 40.5% | 116 | Constructor behavior |
| RegExp.prototype | 39.0% | 487 | Symbol.match/replace/search handling |
| JSON.stringify | 37.9% | 66 | Replacer function, proxy, toJSON |
| TypedArray.prototype | 35.7% | 1396 | Large suite, many method gaps |
| BigInt.asIntN/asUintN | 28.6% | 28 | |
| RegExp.property-escapes | 26.8% | 613 | Unicode property escapes |

---

## Not Implemented (0-20% pass rate)

Features requiring new implementation from scratch.

| Suite | Pass Rate | Tests | Priority | Notes |
|-------|-----------|-------|----------|-------|
| ArrayBuffer.prototype | 12.9% | 147 | Medium | resize, transfer methods |
| MapIteratorPrototype.next | 10.0% | 10 | Low | Iterator result shape |
| SetIteratorPrototype.next | 10.0% | 10 | Low | Iterator result shape |
| Iterator.prototype | 4.8% | 373 | Low | ES2025 Iterator helpers |
| DisposableStack.prototype | 1.3% | 78 | Low | ES2025 Explicit Resource Management |
| AsyncDisposableStack.prototype | 2.6% | 39 | Low | ES2025 |
| DataView.prototype | 0.0% | 499 | Medium | Binary data views |
| FinalizationRegistry.prototype | 0.0% | 31 | Low | GC callbacks |
| WeakRef.prototype | 0.0% | 13 | Low | Weak references |
| Proxy.apply | 0.0% | 14 | Medium | Function proxy traps |
| Atomics.* | 0.0% | ~400 | Low | SharedArrayBuffer/threading |
| Temporal.* | 0.0% | ~4000 | Low | New date/time API (stage 3) |
| Iterator.concat/from/zip | 0.0% | ~130 | Low | ES2025 Iterator helpers |
| JSON.rawJSON | 0.0% | 10 | Low | ES2025 |
| RegExp.escape | 0.0% | 20 | Low | ES2025 |
| RegExp.unicodeSets | 0.0% | 114 | Medium | `v` flag support |

---

## Fix Plan

### Phase 1: Quick Wins (1-2 hours)

Small fixes that unlock multiple tests:

1. **`Object.prototype.__defineGetter__`** - ~10 tests
2. **`Object.prototype.__defineSetter__`** - ~10 tests
3. **`Object.prototype.__lookupGetter__`** - ~10 tests
4. **`Object.prototype.__lookupSetter__`** - ~10 tests
5. **`Date.prototype[Symbol.toPrimitive]`** - ~15 tests
6. **`JSON.stringify` length property** - 1 test (set arity to 3)

### Phase 2: Medium Effort (half day each)

1. **RegExp Symbol methods** - Proper `Symbol.match`, `Symbol.replace`, `Symbol.search`, `Symbol.split` handling
2. **Set ES2025 methods** - `difference`, `intersection`, `union`, `symmetricDifference`, `isSubsetOf`, `isSupersetOf`, `isDisjointFrom`
3. **JSON.stringify improvements** - Replacer function handling, toJSON, proxy support
4. **TypedArray methods** - Fill gaps in prototype methods

### Phase 3: Large Effort (1+ days each)

1. **DataView** - Full implementation of typed views
2. **Iterator helpers** - ES2025 Iterator.prototype methods
3. **RegExp Unicode** - Property escapes, unicodeSets flag
4. **ArrayBuffer.prototype** - resize, transfer, detach

### Phase 4: Future/Optional

1. **Temporal API** - Large new date/time system
2. **Atomics/SharedArrayBuffer** - Threading primitives
3. **Explicit Resource Management** - DisposableStack, using declarations
4. **WeakRef/FinalizationRegistry** - GC features

---

## Test Commands

```bash
# Check specific suite
./paserati-test262 -path ./test262 -subpath "built-ins/Object/prototype" -timeout 0.2s

# Get suite breakdown
./paserati-test262 -path ./test262 -subpath "built-ins" -suite -timeout 0.2s

# Check against baseline
./paserati-test262 -path ./test262 -subpath "built-ins" -timeout 0.2s -diff baseline.txt
```

---

## Notes

- Temporal tests (~4000) significantly impact overall percentage but are a stage 3 proposal
- Atomics requires SharedArrayBuffer which needs threading support
- Iterator helpers are ES2025 and lower priority than core ES6-ES2023 features
- Focus on core built-ins (Object, Array, String, RegExp, Promise) for maximum impact

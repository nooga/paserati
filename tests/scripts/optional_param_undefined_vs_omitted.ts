// expect: true
// paserati#310 removed the compiler's phantom-undefined padding for a call
// site that omits a checker-known-optional argument, which fixed
// arguments.length AND fixed every native function whose spec text
// describes its optional parameter as "if X is not present" - a PRESENCE
// check, not a value check (Array.prototype.reduce/reduceRight's
// initialValue, splice/toSpliced's deleteCount, every Date setter's
// trailing time/date components). For those, an omitted argument and an
// EXPLICIT `undefined` argument are supposed to behave differently, and
// #310 alone makes both cases correct with no pkg/builtins change needed.
//
// A separate, smaller class of natives remained broken even after #310:
// ones whose spec explicitly says "if X is undefined" (a VALUE check) -
// Array.prototype.join's separator (paserati#306) - or WebIDL-defined Web
// APIs, where an omitted argument and one explicitly passed as `undefined`
// are equivalent by convention for a non-defaulted optional parameter
// (TextDecoder.prototype.decode's input, performance.getEntriesByName's
// type). Those still needed a `!args[i].IsUndefined()` guard alongside the
// length check. Number.parseInt's radix isn't really presence-vs-value at
// all (ToInt32(undefined) is spec'd to behave the same either way); its
// bug was a Go-implementation-defined float64->int conversion of NaN, not
// a spec violation - fixed for portability, not conformance.
//
// This file asserts BOTH shapes for both categories so a regression that
// re-adds call-site padding (breaking the presence-based group) or one
// that over-applies an IsUndefined() guard to a presence-based method
// (equally wrong, in the other direction) fails loudly.
const checks: boolean[] = [];

// --- Value-based (or WebIDL-equivalent): omitted and explicit undefined
// must behave IDENTICALLY. ---

// Array.prototype.join - ECMA-262 23.1.3.16 step 3: "If separator is
// undefined, let sep be the single-element String ','" (a value check).
checks.push([1, 2, 3].join() === "1,2,3");
checks.push([1, 2, 3].join(undefined) === "1,2,3");
checks.push([1, 2, 3].join("-") === "1-2-3");

// TextDecoder.prototype.decode - WebIDL optional BufferSource with no
// explicit default; undefined and omitted are equivalent by convention.
const decoder = new TextDecoder();
checks.push(decoder.decode() === "");
checks.push(decoder.decode(undefined) === "");

// performance.getEntriesByName - WebIDL optional DOMString, same rule.
performance.mark("optional-param-test-mark");
performance.measure("optional-param-test-measure", "optional-param-test-mark");
checks.push(performance.getEntriesByName("optional-param-test-measure").length === 1);
checks.push(performance.getEntriesByName("optional-param-test-measure", undefined).length === 1);
checks.push(performance.getEntriesByName("optional-param-test-measure", "measure").length === 1);
checks.push(performance.getEntriesByName("optional-param-test-measure", "mark").length === 0);

// Number.parseInt - portability fix, not a behavior change: both forms
// must still parse as base 10.
checks.push(Number.parseInt("10") === 10);
checks.push(Number.parseInt("10", undefined) === 10);
checks.push(Number.parseInt("ff", 16) === 255);
checks.push(Number.parseInt("10", 0) === 10); // radix 0 means "unspecified" per spec

// --- Presence-based: omitted and explicit undefined must behave
// DIFFERENTLY, per each function's own "if X is not present" spec text.
// These need NO pkg/builtins guard - #310 alone makes both cases correct,
// and adding an IsUndefined() guard here would be a NEW bug, not a fix. ---

// Array.prototype.reduce - ECMA-262 23.1.3.27: initialValue "present" vs
// "not present" changes whether arr[0] seeds the accumulator.
checks.push([1, 2, 3, 4].reduce((a: number, v: number) => a + v) === 10); // omitted: seed = arr[0]
checks.push(
  Number.isNaN([1, 2, 3, 4].reduce((a: any, v: number) => a + v, undefined))
); // explicit undefined: seed = undefined, undefined+1 = NaN

// Array.prototype.reduceRight - same presence rule.
checks.push([1, 2, 3, 4].reduceRight((a: number, v: number) => a + v) === 10);
checks.push(
  Number.isNaN([1, 2, 3, 4].reduceRight((a: any, v: number) => a + v, undefined))
);

// Array.prototype.splice - ECMA-262 23.1.3.30: deleteCount "not present"
// means "delete to the end"; explicitly present as undefined means
// ToIntegerOrInfinity(undefined) = 0, deleting nothing.
const omittedDeleteCount = [1, 2, 3, 4, 5];
const removed1 = omittedDeleteCount.splice(2);
checks.push(JSON.stringify(removed1) === "[3,4,5]");
checks.push(JSON.stringify(omittedDeleteCount) === "[1,2]");

const explicitUndefinedDeleteCount = [1, 2, 3];
const removed2 = explicitUndefinedDeleteCount.splice(1, undefined as any, "x");
checks.push(JSON.stringify(removed2) === "[]");
checks.push(JSON.stringify(explicitUndefinedDeleteCount) === '[1,"x",2,3]');

// Date.prototype.setMonth - ECMA-262 21.4.4.29: date "not present" keeps
// the current day-of-month; explicitly present as undefined runs
// ToNumber(undefined) = NaN, invalidating the whole date.
const keepsDay = new Date(2020, 0, 15);
keepsDay.setMonth(5);
checks.push(keepsDay.getMonth() === 5 && keepsDay.getDate() === 15);

const invalidated = new Date(2020, 0, 15);
invalidated.setMonth(5, undefined);
checks.push(Number.isNaN(invalidated.getTime()));

checks.every((c) => c === true);

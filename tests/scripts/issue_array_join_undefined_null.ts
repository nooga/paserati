// expect: true
// ECMA-262 23.1.3.16 Array.prototype.join step 6: "If element is undefined
// or null, let next be the empty String; otherwise, let next be
// ? ToString(element)." Per spec, Array.prototype.toString (23.1.3.36)
// implements this by looking up and calling the array's own "join" method;
// this codebase's toString instead inlines the same per-element rule
// directly rather than doing that Get("join")+Call (a separate, narrower,
// still-open gap - see test262 non-callable-join-string-tag.js, which
// checks the case where "join" has been replaced with a non-callable value
// and toString must fall back to Object.prototype.toString - not exercised
// or fixed here). Both toString and the default String(arr) conversion
// (via ToPrimitive calling toString, since Array.prototype.join hasn't
// been overridden here) share this element-stringification rule with join.
//
// This was previously not special-cased at all: undefined/null elements
// were passed straight through ToString, producing the literal text
// "undefined"/"null" in the joined result instead of an empty string. A
// real hole (from `delete arr[i]` or a literal elision `[1,,3]`) is read
// back as plain Undefined via [[Get]] before join ever sees it, so it hits
// the exact same code path and must join the same way.
const checks: boolean[] = [];

checks.push([1, undefined, 3].join(",") === "1,,3");
checks.push([1, null, 3].join(",") === "1,,3");
checks.push([1, undefined, null, 3].join(",") === "1,,,3");
checks.push(String([1, undefined, null, 3]) === "1,,,3");
checks.push([1, undefined, null, 3].toString() === "1,,,3");

// Note: join() called with the separator argument fully omitted (not even
// `undefined`) is a separate, pre-existing, unrelated bug - the type-checked
// call path pads a native call up to the callee's declared arity with
// explicit `undefined` arguments for omitted optional parameters, so `join`
// sees len(args) >= 1 and stringifies that padding `undefined` as the
// separator instead of falling back to its "," default (confirmed on
// unmodified main, unrelated to element stringification). Not exercised
// here; tracked separately. join(",") with an explicit separator (as used
// throughout this file) is unaffected.

// Leading/trailing undefined and null.
checks.push([undefined, 1, 2].join(",") === ",1,2");
checks.push([1, 2, null].join(",") === "1,2,");
checks.push([undefined].join(",") === "");
checks.push([null].join(",") === "");

// A real hole (delete) must join identically to a plain undefined element.
const deleted = [1, 2, 3];
delete deleted[1];
checks.push(deleted.join(",") === "1,,3");
checks.push(deleted.toString() === "1,,3");

// A real hole (literal elision) must join identically too.
const elided = [1, , 3];
checks.push(elided.join(",") === "1,,3");
checks.push(elided.toString() === "1,,3");

// A custom separator still applies between elements, empty string included.
checks.push([1, undefined, 3].join(" - ") === "1 -  - 3");

checks.every((c) => c === true);

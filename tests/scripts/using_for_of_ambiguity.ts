// no-typecheck
// Test disambiguation of `using` immediately followed by `of` in a for-loop
// head. This is genuinely ambiguous:
//   for (using of xs)       - for-of loop over a variable named `using`
//   for (using of = e;;)    - classic for-loop, `using` declares a variable
//                             literally named `of` (an initializer expression
//                             can never start with `=`, so this is the only
//                             unambiguous signal)
// `of` is not restricted as a `using` binding name outside the for-of/for-in
// head itself - only the for-of reading of `using of` is.
// expect: using/of ambiguity checks passed

// Classic for-loop: `using` declares a variable named `of`.
let ranClassic = false;
for (using of = { [Symbol.dispose]: () => {} };;) {
  ranClassic = true;
  break;
}
if (!ranClassic) throw new Error("for (using of = e;;) did not run its body");

// Real for-of loop: plain identifier `using` iterates over the array.
let seen: number[] = [];
for (using of [1, 2, 3]) {
  seen.push(using);
}
if (seen.join(",") !== "1,2,3") throw new Error("for (using of xs) regressed to a using-declaration");

// A normal `using x = e;;` for-loop still works (the already-working case
// this fix must not disturb).
let ranNamed = false;
for (using x = { [Symbol.dispose]: () => {} };;) {
  ranNamed = true;
  break;
}
if (!ranNamed) throw new Error("for (using x = e;;) regressed");

// The `await using` sibling predicate must agree with the plain `using` one:
// `await using of = e;;` also declares a variable named `of`.
let ranAwaitOf = false;
for (await using of = { [Symbol.dispose]: () => {} };;) {
  ranAwaitOf = true;
  break;
}
if (!ranAwaitOf) throw new Error("for (await using of = e;;) did not run its body");

// `await using x = e;;` (already-working case) must still work too.
let ranAwaitNamed = false;
for (await using x = { [Symbol.dispose]: () => {} };;) {
  ranAwaitNamed = true;
  break;
}
if (!ranAwaitNamed) throw new Error("for (await using x = e;;) regressed");

// A type-annotated `using of: T = e;;` is disambiguated the same way as `=`.
for (using of: any = { [Symbol.dispose]: () => {} };;) {
  break;
}

"using/of ambiguity checks passed";

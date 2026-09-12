// Test nested array spread with function calls
//
// This one caught a real regression during paserati#426's investigation:
// closureBindingDest (used when a top-level `let`'s value is a function/
// arrow literal) reused a predefined global symbol's stale, meaningless
// zero-valued Register field directly (missing the !sym.IsGlobal check its
// sibling non-function-value branch already had), aliasing getArray's
// closure onto whatever unrelated register shared that same stale number -
// harmless by accident as long as nothing else was ever allowed to reuse
// that number, which held only because a *different*, since-fixed
// register-leak bug kept it permanently (accidentally) reserved. Once that
// leak was fixed, the alias went live: the script's own completion-value
// register collided with getArray's closure register, and a later spread-
// element scratch register reused that same now-free number, silently
// overwriting the in-progress result array before the final spread could
// read it. Must remain a *bare* completion-value expression (not assigned
// to a variable first) to exercise this - `const result = [...]` routes
// into a fresh, unaliased temp instead and doesn't reproduce it.
let getArray = () => [10, 20];
let nested = [[1, 2], [3, 4]];
[...nested[0], ...getArray(), ...nested[1]];
// expect: [1, 2, 10, 20, 3, 4]
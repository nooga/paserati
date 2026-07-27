// A `let` binding narrows to its own last-assigned literal type in
// straight-line code, the same way TypeScript's control flow analysis does
// (see contextuallyTypeLogicalAnd01 in the TypeScript conformance suite):
// `y`'s flow type right after `let y = true;` is the literal `true`, not
// the declared `boolean`, so `y && ...` short-circuits entirely to the
// right operand instead of unioning in `false`.

let y = true;
let f: (s: string) => number = y && ((s) => s.length);

// Reassignment updates the tracked narrow type rather than leaving the old
// one behind: after `y = false`, `y && ...` short-circuits to `y` itself.
y = false;
let g: number | false = y && 1;

// expect: true
f("hi") === 2 && g === false;

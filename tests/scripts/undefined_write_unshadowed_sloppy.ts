// skip-typecheck
// expect: true
// paserati#440 follow-up: `undefined = 43` at global scope, with no local
// shadow, targets the real global `undefined` - a non-writable, non-
// configurable property per ECMAScript. In sloppy mode the assignment is a
// silent no-op (real Node: `undefined = 43; console.log(undefined)` still
// prints "undefined"), not a write. Compare against `void 0` rather than
// asserting `undefined` directly: if the write had wrongly landed as 43,
// `undefined === void 0` (i.e. 43 === undefined) is false, so this actually
// discriminates instead of restating the (broken) value either way.
// See undefined_write_unshadowed_strict.ts for the strict-mode TypeError side.
undefined = 43;
undefined === void 0;

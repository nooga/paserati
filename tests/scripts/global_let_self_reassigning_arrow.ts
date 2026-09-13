// expect: 2
// no-typecheck
// paserati#449, arrow-function form of the same self-reassigning top-level
// closure idiom (see global_let_self_reassigning_closure.ts) - the bug and
// the fix are identical for arrow literals, but the two are compiled through
// separate code paths (compileArrowFunctionWithName vs compileFunctionLiteral),
// so both need their own coverage.
let b = () => {
  b = 2;
};
b();
b;

// Flow narrowing of a `let` binding's own literal type (see
// flow_narrowed_literal.ts) is deliberately conservative: a branch that
// might reassign the variable invalidates the narrowing instead of trying
// to merge it, since assuming the pre-branch value survived would be
// unsound. `z` must still be checked against its declared `boolean` type
// here, not a stale narrowed `true`.

declare function cond(): boolean;

let z = true;
if (cond()) {
  z = false;
}
let w: true = z;

// expect_compile_error: Type 'boolean' is not assignable to type 'true'.

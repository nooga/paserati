// Regression / correctness pin for issue #239's fix.
//
// The two-operand path in compileInfixExpression now threads the caller's
// destination register straight into a non-simple right operand instead of
// allocating a fresh temp (see compile_expression.go), so a right-nested
// chain `a + (b + (c + ...))` no longer burns one register per nesting
// level held live for the whole recursive descent. That's safe *only*
// because the left operand of each level is still fully materialized into
// its own register before the right subtree is compiled (unchanged,
// pre-existing behavior) -- it must be, since evaluating the right subtree
// can run arbitrary user code (via ToPrimitive/valueOf) that reassigns the
// very variable the left operand came from. Each syntactic occurrence of a
// mutable operand has to capture its value at its *own* evaluation point;
// a later reassignment must not retroactively change an earlier capture.
// This is a genuine register-per-level floor, not a shortfall the fix left
// on the table (a checker-type-gated fast path was considered and rejected
// -- it would do nothing for `--no-typecheck`/test262, the mode that
// matters most here, and only "prove primitive" for literals anyway).
//
// These cases pin: (1) evaluation order is preserved across a right-nested
// chain even when every leaf's valueOf mutates the shared variable, (2) the
// `leftReg == hint` fallback (self-assignment, `x = x + (...)`) doesn't
// clobber the left operand before it's read, and (3) dest-aliases-right
// aliasing doesn't silently swap operand roles for non-commutative
// operators or reorder float-precision-sensitive grouping.
// no-typecheck
// expect: true

function valueOfChain(): boolean {
  let n = 0;
  let x = { valueOf() { x = 99; n++; return n; } };
  // Every occurrence of `x` reads (GetValue) the same original object --
  // that read happens before any reassignment, regardless of how deep it
  // sits, because reading a variable can't itself run user code. What
  // differs is *when each occurrence's ToPrimitive/valueOf call fires*:
  // that happens bottom-up, innermost combine first (x3's valueOf, then
  // x4's, then x2's, then x1's last), each bumping n and returning it, so
  // the four calls return 1, 2, 3, 4 in that order: C = x3+x4 = 1+2 = 3,
  // B = x2+C = 3+3 = 6, A = x1+B = 4+6 = 10. Node agrees (verified via the
  // `node` binary while writing this test) -- this pins that grouping and
  // valueOf call order against a regression, not just "some plausible sum".
  return (x + (x + (x + x))) === 10 && n === 4;
}

function selfAssignFallback(): boolean {
  // Forces leftReg (x's own register, via the pre-existing direct-read
  // optimization) to coincide with the destination the assignment writes
  // through. If the right-side hint-threading fix wrongly reused that same
  // register for the literal `5`, it would clobber x before the add reads
  // it, yielding 10 instead of 6.
  let x = 1;
  x = x + 5;
  return x === 6;
}

function selfAssignFallbackWithMutation(): boolean {
  let n = 0;
  let x = { valueOf() { x = 99; n++; return n; } };
  x = x + (x + x);
  return x === 6 && n === 3;
}

function nonCommutativeGrouping(): boolean {
  // Right grouping must survive: 1 - (2 - (3 - 4)) = 1 - (2 - (-1)) = 1 - 3 = -2.
  // A dest/operand-role mixup would instead compute something matching the
  // left-associative grouping (1 - 2 - 3 - 4 = -8) or another wrong value.
  return 1 - (2 - (3 - 4)) === -2;
}

function floatGroupingPreserved(): boolean {
  // Rounding makes these two groupings observably different; if the fix
  // reassociated anything, one of these would silently drift to the other.
  const rightGrouped = 0.1 + (0.2 + 0.3);
  const leftGrouped = (0.1 + 0.2) + 0.3;
  return rightGrouped === 0.6 && leftGrouped !== rightGrouped;
}

[
  valueOfChain(),
  selfAssignFallback(),
  selfAssignFallbackWithMutation(),
  nonCommutativeGrouping(),
  floatGroupingPreserved(),
].every((v) => v);

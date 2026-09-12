// skip-typecheck
// expect_runtime_error: before initialization
// Same TDZ gap as tdz_try_body_forward_ref.ts, but for a catch clause body
// (with a bound parameter) - compileTryStatement had a second, separately
// duplicated predefine pass here with the identical bug. See that file's
// comment for why only the "before initialization" substring is asserted.
try {
  try {
    throw new Error("boom");
  } catch (e1) {
    console.log(y);
    let y = 1;
  }
} catch (e) {
  throw e;
}

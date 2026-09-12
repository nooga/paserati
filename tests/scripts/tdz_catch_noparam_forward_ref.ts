// skip-typecheck
// expect_runtime_error: before initialization
// Same as tdz_catch_body_forward_ref.ts, for the parameter-less catch
// (ES2019+ `catch { ... }`) code path - a third separately duplicated
// predefine pass with the identical bug. See tdz_try_body_forward_ref.ts's
// comment for why only the "before initialization" substring is asserted.
try {
  try {
    throw new Error("boom");
  } catch {
    console.log(z);
    let z = 1;
  }
} catch (e) {
  throw e;
}

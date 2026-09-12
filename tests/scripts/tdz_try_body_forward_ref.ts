// skip-typecheck
// expect_runtime_error: before initialization
// A forward reference to a block-scoped let/const inside a try body must
// throw a TDZ ReferenceError, same as inside a plain block, if, or while
// (all of which already worked). compileTryStatement used to reimplement
// its own predefine pass for the try body with a plain (non-TDZ) Define and
// no Uninitialized marker, so the read silently saw the register's leftover
// value instead of throwing (real Node: ReferenceError: Cannot access 'x'
// before initialization). Asserting only the "before initialization"
// substring rather than the full message: register-based TDZ errors don't
// include the variable name ("Cannot access variable before initialization"
// vs. Node's "Cannot access 'x' before..."), a separate, pre-existing,
// general gap (also present for a plain `{ }` block) - not fixed here.
try {
  console.log(x);
  let x = 42;
} catch (e) {
  throw e;
}

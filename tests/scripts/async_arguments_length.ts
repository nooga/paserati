// expect: Promise { 3 }
// Regression test: executeAsyncFunctionBody (pkg/vm/async.go) builds an
// async function's initial frame by hand instead of going through the
// ordinary call path (prepareCall, call.go), and never set frame.args - the
// field OpGetArguments actually reads to build the `arguments` object
// (frame.argCount only sizes the register-copy loop). `arguments.length`
// read 0 inside every async function regardless of how many arguments were
// passed. `first` has no internal await, so it resolves synchronously and
// the resolved value is directly observable as the script's last-statement
// value (see async_function_basic.ts for the same convention).
async function first(a: number, b: number, c: number) {
  return arguments.length;
}
first(1, 2, 3);

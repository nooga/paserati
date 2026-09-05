// expect: Promise { [] }
// Regression test: an async function's frame setup (executeAsyncFunctionBody
// in pkg/vm/async.go) built its register window by hand instead of going
// through the ordinary call path (prepareCall in call.go), and never
// reproduced that path's variadic/rest-parameter handling. A rest-only
// async function called with fewer arguments than its arity left the rest
// register at its zeroed Undefined default instead of an empty array -
// `await af()` observed `undefined` where `[]` was expected. No `await`
// appears in this file: `af` has no internal await, so it resolves
// synchronously and the resolved value is directly observable as the
// script's last-statement value (see async_function_basic.ts for the same
// convention).
async function af(...args: any[]) {
  return args;
}
af();

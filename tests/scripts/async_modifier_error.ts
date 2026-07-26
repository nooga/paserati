// expect_compile_error: 'async' modifier cannot be used here.
// TS1042: `async` in front of a declaration that has no asynchronous form.
// Only the misplaced-modifier shape gets this code — `async` before something
// that is not a declaration at all stays a plain expression error.

async class C {
}

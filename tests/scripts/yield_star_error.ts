// Test yield* with non-iterable should fail
// expect_compile_error: must have a '[Symbol.iterator]()' method that returns an iterator

function* gen() {
  yield* 42; // numbers are not iterable
}
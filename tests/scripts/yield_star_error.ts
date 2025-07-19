// Test yield* with non-iterable should fail
// expect_compile_error: yield* expression must be an iterable

function* gen() {
  yield* 42; // numbers are not iterable
}
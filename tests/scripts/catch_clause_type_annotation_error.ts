// expect_compile_error: Catch clause variable type annotation must be 'any' or 'unknown' if specified.
try {
  throw 1;
} catch (e: string) {
  e;
}

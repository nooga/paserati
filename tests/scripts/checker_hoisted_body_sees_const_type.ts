// expect_compile_error: Type 'number' is not assignable to type 'string'
// A hoisted function body is checked before top-level initializers; it must
// still see a top-level const's inferred type, not any.
const B = 1;
function f() {
  const q: string = B;
  return q;
}

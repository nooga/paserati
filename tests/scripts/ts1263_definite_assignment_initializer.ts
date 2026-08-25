// TS1263: a `!` definite-assignment assertion and an initializer are
// mutually exclusive - the assertion promises a later, out-of-band write; an
// initializer means there's nothing to promise. Must be caught both at the
// top level and inside a function body.
function f() {
  let x!: number = 5;
}
f();
// expect_compile_error: Declarations with initializers cannot also have definite assignment assertions

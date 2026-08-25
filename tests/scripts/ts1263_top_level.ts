// TS1263 must also fire for a top-level declarator, not just inside a
// function body - top-level declarations go through the checker's separate
// hoisting pass (Pass 2), which needs its own check call.
const x!: number = 5;
x;
// expect_compile_error: Declarations with initializers cannot also have definite assignment assertions

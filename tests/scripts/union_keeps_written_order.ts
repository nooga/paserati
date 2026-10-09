// expect_compile_error: Type '"huge"' is not assignable to type '"small" | "medium" | "large"'.
type Size = "small" | "medium" | "large";
const s: Size = "huge";

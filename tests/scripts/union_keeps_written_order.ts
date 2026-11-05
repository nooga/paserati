// expect_compile_error: Type '"huge"' is not assignable to type 'Size'.
type Size = "small" | "medium" | "large";
const s: Size = "huge";

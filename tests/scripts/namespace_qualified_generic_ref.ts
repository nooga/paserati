// N.G<T> resolves G inside N instead of silently becoming any.
namespace N { export type G<T> = T[]; }
const x: N.G<number> = ["a"];

// expect_compile_error: Type 'string' is not assignable to type 'number'.

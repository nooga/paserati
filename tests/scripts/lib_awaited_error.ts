// expect_compile_error: Type 'string' is not assignable to type 'number'
type A = Awaited<Promise<Promise<number>>>;
const a: A = "nope";

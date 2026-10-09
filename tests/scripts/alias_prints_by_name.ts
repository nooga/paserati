// expect_compile_error: Type 'number' is not assignable to type 'Pair<string, number>'
type Pair<A, B> = { a: A; b: B };
const p: Pair<string, number> = 5;

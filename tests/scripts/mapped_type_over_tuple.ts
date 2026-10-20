// expect_compile_error: Type 'number' is not assignable to type '[string, number]'
type Mapped<T> = { [K in keyof T]: T[K] };
let r: Mapped<[string, number]> = 1;

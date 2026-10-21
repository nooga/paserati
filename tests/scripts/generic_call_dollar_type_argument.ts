// expect_compile_error: Type 'number' is not assignable to type 'string'.
// `f<$R>()` is a generic call however the type argument is spelled.
declare function f<T>(): T;
type $R = number;
const x: string = f<$R>();

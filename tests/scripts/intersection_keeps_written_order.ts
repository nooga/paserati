// expect_compile_error: Type '{ z: number }' is not assignable to type 'Z & A'.
// An intersection prints its members in the order they were written, as tsc does.
type Z = { z: number };
type A = { a: number };
const v: Z & A = { z: 1 };

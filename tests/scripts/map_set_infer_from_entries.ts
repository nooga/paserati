// expect_compile_error: Argument of type 'number' is not assignable to parameter of type 'string'
const m = new Map([["a", 1], ["b", 2]]);
m.set(1, 2);

// var with type mismatch should produce compile error

var count: string = 42;
// expect_compile_error: Type '42' is not assignable to type 'string'.
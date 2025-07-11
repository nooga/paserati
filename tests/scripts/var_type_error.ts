// var with type mismatch should produce compile error

var count: string = 42;
// expect_compile_error: cannot assign type '42' to variable 'count' of type 'string'
// expect_compile_error: Duplicate identifier 'a'
let x = {} as { a: number; a: number };

x;

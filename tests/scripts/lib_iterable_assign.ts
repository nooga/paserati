// expect_compile_error: is not assignable to type 'Iterable<string, any, any>'
const nums: Iterable<number> = [1, 2, 3];
const strs: Iterable<string> = nums;

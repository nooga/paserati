// expect_compile_error: Type 'Iterable<number, any, any>' is not assignable to type 'symbol'
declare const x: Iterable<number>;
const s: symbol = x;

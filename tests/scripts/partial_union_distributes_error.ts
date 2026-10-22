// expect_compile_error: Type '1 | undefined' is not assignable to type 'number'
declare const x: Partial<{ a: 1; c: 3 } | { b: 2; c: 3 }>;
if ('a' in x) { const n: number = x.a; }

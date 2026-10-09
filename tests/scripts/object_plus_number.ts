// expect_compile_error: Operator '+' cannot be applied to types 'D' and 'number'.
interface D { n: number }
const d: D = { n: 1 };
const x: number = d + 1;

// zod's TypeOf<T extends ZodType<any>> = T["_output"] applied to typeof S in
// a hoisted alias: the constraint is not checked against the unresolved
// typeof (no TS2344), and the result indexes S's type.
declare class ZodType<Output> { readonly _output: Output; }
type TypeOf<T extends ZodType<any>> = T["_output"];
declare const S: ZodType<{ n: number }>;
type T = TypeOf<typeof S>;
const ok: T = { n: 1 };
const bad: T = { n: "x" };

// expect_compile_error: Type 'string' is not assignable to type 'number'.

// The valid side of typeof_constraint_in_alias.ts: no false TS2344 or TS2322.
class ZodType<Output> { readonly _output!: Output; }
type TypeOf<T extends ZodType<any>> = T["_output"];
const S: ZodType<{ n: number }> = new ZodType<{ n: number }>();
type T = TypeOf<typeof S>;
const ok: T = { n: 1 };
ok.n;

// expect: 1

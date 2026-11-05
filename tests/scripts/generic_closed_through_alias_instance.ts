// expect_compile_error: Type 'number' is not assignable to type 'string'.
// A generic interface naming a type alias instantiated with its own type
// parameter (`CallerOverride<TContext>`, whose body names an interface
// declared further down) still names no type parameter but its own: its
// instantiations made while it was being declared (`use`'s return type) get
// the members declared after them, so `.query()` is found on the result.
export type CallerOverride<TContext> = (opts: {
  invoke: (opts: ProcedureCallOptions<TContext>) => Promise<unknown>;
}) => Promise<unknown>;
export interface Res<O> {
  _o?: O;
  ok: true;
}
export interface ProcedureBuilder<TContext, TOv> {
  a: number;
  use<Out>(
    fn: (o: { ctx: TContext; next: <O>(x: { ctx: O }) => Res<O> }) => Res<Out>,
  ): ProcedureBuilder<TContext, TOv & Out>;
  experimental_caller(caller: CallerOverride<TContext>): ProcedureBuilder<TContext, TOv>;
  assertion(value: unknown): asserts value is TContext;
  resolver?: LaterResolver;
  query(): TOv;
}
export type LaterResolver = (x: number) => void;
export interface ProcedureCallOptions<TContext> {
  ctx: TContext;
}
declare const p: ProcedureBuilder<{}, {}>;
const r = p.use((o) => o.next({ ctx: { foo: 1 } })).query();
const n: string = r.foo;

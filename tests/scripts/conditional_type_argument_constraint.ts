// expect: ok
// A deferred conditional type satisfies a constraint through its branches,
// an `infer` placeholder through the constraint its position implies, and
// the true branch through what the check type's constraint infers it to be
// (tRPC's CreateRootTypes<{ errorShape: ... }>).
interface S { code: number }
type EF<T extends S> = (o: number) => T;
type Inf<T> = T extends EF<infer TShape> ? TShape : S;
interface RT { ctx: object; shape: S; tr: boolean }
type C<T extends RT> = T;
interface Opts { f?: EF<any> }
export function g<O extends { f?: EF<S> }, P extends Opts>() {
  type R1 = C<{ ctx: {}; shape: S; tr: undefined extends O['f'] ? false : true }>;
  type R2 = C<{ ctx: {}; shape: undefined extends O['f'] ? S : S; tr: boolean }>;
  type R3 = C<{ ctx: {}; shape: Inf<O['f']>; tr: boolean }>;
  type R4 = C<{ ctx: {}; shape: undefined extends O['f'] ? S : Inf<O['f']>; tr: boolean }>;
  type R5 = C<{ ctx: {}; shape: Inf<P['f']>; tr: boolean }>;
}
"ok";

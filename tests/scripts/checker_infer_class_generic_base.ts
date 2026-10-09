// expect: ok
// A class passed as `new (...) => T` infers T with members inherited from its generic base.
class Base<D, S> { constructor(public definition: D, public state: S) {} }
class X extends Base<{ a: string }, { b?: number }> {}

function f<T extends { definition: any; state: any }>(cls: new (...args: any[]) => T, def: T["definition"]): T["state"] {
  return null as any;
}
const s: { b?: number } = f(X, { a: "x" });

function g<D, S>(cls: new (...args: any[]) => Base<D, S>, def: D): S {
  return null as any;
}
const s2: { b?: number } = g(X, { a: "x" });
"ok";

// expect: 3
// A deferred conditional type built by substituting a generic return type
// relates to the same conditional written out, though one branch holds an
// expanded Partial<Obs<T, E>> and the other the mapped type itself.
interface Obs<T, E> { onData: (v: T) => void; onError: (e: E) => void }
type W<T> = T extends 1 ? { a: Partial<Obs<T, Error>> } : { b: 2 };
function mk<T>(): W<T> { return { b: 2 } as W<T>; }
function f<T>(): W<T> {
  const p = mk<T>();
  return p;
}
type M<T> = { [K in keyof T]: T[K] extends infer V ? (V extends 1 ? "one" : "other") : never };
declare function mm<T>(): M<T>;
function g<T>(): M<T> { const q = mm<T>(); return q; }
const r = f<number>() as { b: number };
r.b + 1;

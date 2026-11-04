// expect: 1
// A conditional whose check type is keyof A & keyof B with B generic is
// deferred, and a key of a generic type compares with a string literal
// without TS2367 (its constraint is string | number | symbol).
type PI<TType, TWith> = keyof TType & keyof TWith extends never ? TType & TWith : { err: string };
interface Base<T> { useContext(): T; withTRPC: number }
type Next<T> = PI<Base<T>, { [K in keyof T]: 1 }>;
function flat<TFaux>(cb: (path: keyof TFaux) => any): TFaux { return cb("withTRPC" as keyof TFaux); }
function mk<T>(): Next<T> {
  return flat((key) => (key === "withTRPC" ? 1 : 2));
}
function isKey<T>(k: keyof T & string): boolean { return k === "x"; }
(mk<{ a: 1 }>() as any) + (isKey<{ x: 1 }>("x") ? 0 : 10);

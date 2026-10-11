// An infer pattern against a recursive generic interface (tRPC's
// inferObservableValue<T> = T extends Observable<infer V, unknown> ? V : never,
// whose pipe() returns Observable again) used to recurse in inference forever.
interface Obs<T, E> {
  pipe(): Obs<T, E>;
  map<U>(fn: (v: T) => U): Obs<U, E>;
  subscribe(cb: (v: T) => void): void;
}
type ValueOf<X> = X extends Obs<infer V, unknown> ? V : never;

function valueType<A, B>(o: Obs<A, B>): ValueOf<Obs<A, B>> {
  return null as any;
}

const n: ValueOf<Obs<number, string>> = 41;
const s: ValueOf<Obs<string, Error>> = "x";
n + s.length;

// expect: 42

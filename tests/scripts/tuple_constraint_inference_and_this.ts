// expect: 3
// A type parameter constrained to a tuple infers a tuple from an array
// literal ([A, B], not its constraint), `this` in an inherited method naming
// a generic declared later is the receiver's type, and a literal element
// widens unless its contextual type has literals of its kind.
interface Some {
  _z: unknown;
}
interface Base<N> {
  _z: N;
  optional(): Opt<this>;
}
interface Opt<T> {
  inner: T;
  _z: unknown;
}
interface Tup<T extends readonly [Some, ...Some[]]> extends Base<T> {
  items: T;
}
interface A extends Base<string> {
  a: 1;
}
interface B extends Base<number> {
  b: 2;
}
function tuple<T extends readonly [Some, ...Some[]]>(items: T): Tup<T> {
  const t: Tup<T> = {
    _z: items,
    items,
    optional() {
      return { inner: t, _z: undefined };
    },
  };
  return t;
}
function mk<N>(z: N): Base<N> {
  const b: Base<N> = {
    _z: z,
    optional() {
      return { inner: b, _z: undefined };
    },
  };
  return b;
}
const a = Object.assign(mk("s"), { a: 1 as const }) as A;
const b = Object.assign(mk(2), { b: 2 as const }) as B;
const t = tuple([a, b]);
const pair: [A, B] = t.items;
const opt: Opt<Tup<[A, B]>> = t.optional();
const keep: [1 | 2, number] = [1, 5];
const n: number = keep[1];
pair[1].b + opt.inner.items.length - keep[0] + (n === 5 ? 0 : 9);

// A generic interface naming itself with a different argument
// (`next(): Grow<Wrap<T>>` inside `interface Grow<T>`) is instantiated while
// its declaration is still being read. That instantiation must end up with
// every member, not only those declared before `next`, and walks for type
// parameters must not follow Grow<Wrap<Wrap<...>>> forever.
interface Wrap<T> { inner: T; }
interface Grow<T> {
  value: T;
  next(): Grow<Wrap<T>>;
  map<U>(f: (v: T) => U): Grow<U>;
  both<U>(other: Grow<U>): Grow<[T, U]>;
}
function make<T>(value: T): Grow<T> {
  const g: Grow<T> = {
    value,
    next: () => make({ inner: value }),
    map: (f) => make(f(value)),
    both: (o) => make([value, o.value] as [T, any]),
  };
  return g;
}
function deeper<A>(g: Grow<A>): Grow<Wrap<Wrap<A>>> {
  return g.next().next();
}
const g = make(1);
const h = deeper(deeper(g.map((x) => x + 1)));
h.value.inner.inner.inner.inner;

// expect: 2

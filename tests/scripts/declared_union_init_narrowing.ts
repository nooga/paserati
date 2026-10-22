// expect: 3
// A declared union narrows to its initializer; a closure's write doesn't undo that.
function createDeferred<TValue = void>() {
  let resolve!: (value: TValue) => void;
  const promise = new Promise<TValue>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}
type Deferred<T> = { promise: Promise<T>; resolve: (value: T) => void };

function run<H>(v: H): number {
  let d: null | Deferred<H> = createDeferred();
  const drop = () => {
    d = null;
  };
  d.resolve(v);
  return 1;
}

function pick<R>(p: Promise<R>, r: R): R {
  let value: R | symbol = Symbol.for("x");
  value = r;
  let q: Promise<R> | null = null;
  q ??= p.catch((e) => {
    throw e;
  });
  const typed: Promise<R> = q;
  return value;
}

run(1) + (pick(Promise.resolve(2), 2) as number);

// expect: 3
// Disposable / AsyncDisposable are lib interfaces keyed by the well-known
// dispose symbols, and an intersection with an unmodelled lib generic
// (AsyncIterator) still exposes that generic's members.
function makeResource<T>(thing: T, dispose: () => void): T & Disposable {
  const it = thing as T & Partial<Disposable>;
  it[Symbol.dispose] = () => {
    dispose();
  };
  return it as T & Disposable;
}

function asResource<T>(it: AsyncIterator<T>): AsyncIterator<T> & AsyncDisposable {
  return it as AsyncIterator<T> & AsyncDisposable;
}

let disposed = 0;
const r = makeResource({ n: 2 }, () => {
  disposed++;
});
r[Symbol.dispose]();
const gen = (async function* () {
  yield 1;
})();
const next = asResource(gen).next;
r.n + disposed + (typeof next === "function" ? 0 : 1);

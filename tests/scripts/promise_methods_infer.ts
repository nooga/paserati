// expect: done
// Promise's then, catch and finally are typed as lib.es5 declares them:
// their callbacks infer the resulting promise's value.
declare const p: Promise<number>;
function typed(p: Promise<number>) {
  const s: Promise<string> = p.then((x) => String(x));
  const either: Promise<number | string> = p.catch(() => "failed");
  const same: Promise<number> = p.finally(() => {});
  const kept: Promise<number> = p.then();
  const rethrown: Promise<number> = p.catch((e) => {
    throw e;
  });
  let onDone: null | ((v: number) => void) = null;
  const settled: Promise<void> = p.then(onDone);
  // A callback returning a promise is unwrapped: T | PromiseLike<T> infers T.
  const chained: Promise<number> = p.then(async (x) => x * 2);
  return [s, either, same, kept, rethrown, settled, chained];
}
async function awaited(p: Promise<number>): Promise<number> {
  const n: number = await p.then(async (x) => x + 1);
  return n;
}
"done";

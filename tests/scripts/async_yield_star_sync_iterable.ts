// yield* and for-await over plain sync iterables go through
// CreateAsyncFromSyncIterator: yielded promises are unwrapped, and
// return() is forwarded to the inner iterator.
// expect: 1,2,3|x,y|inner-closed,ret:9
const parts: string[] = [];

function* inner() {
  try {
    yield 1;
    yield Promise.resolve(2);
    yield 3;
  } finally {
    parts.push("inner-closed");
  }
}
async function* outer() {
  yield* inner();
}
const vals: number[] = [];
for await (const v of outer()) vals.push(v as number);
parts.length = 0;

const seen: string[] = [];
for await (const s of new Set(["x", Promise.resolve("y")])) seen.push(s as string);

const it = outer();
await it.next();
const r = await it.return(9);
parts.push("ret:" + r.value);

vals.join(",") + "|" + seen.join(",") + "|" + parts.join(",");

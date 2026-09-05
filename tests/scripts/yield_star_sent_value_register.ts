// expect: undefined,undefined,undefined|end|Cannot destructure 'null'
// #267: yield* calls iterator.next(sentValue) via OpCallMethod, which reads its
// argument from funcReg+1. The method and argument registers were allocated
// with two separate Alloc() calls, so once earlier statements in the generator
// had populated the free list they were no longer adjacent and next() received
// whatever stale value lived at funcReg+1 (the native next function itself, the
// iterator, ...) instead of undefined. The same latent bug existed for the
// TypeError thrown when destructuring null/undefined.
const seen: string[] = [];
function makeIter(): any {
  let i = 0;
  return {
    [Symbol.iterator]() { return this; },
    next(v: any) {
      seen.push(v === undefined ? "undefined" : typeof v);
      return i++ < 2 ? { value: i, done: false } : { value: "end", done: true };
    },
  };
}
function* outer(arg: any, cache: Map<any, any>) {
  // These statements churn the register free list before the yield*.
  const q2 = [arg, cache].map((v) => v);
  const c = cache.get(arg);
  const d = c && c.value;
  if (cache.has(arg)) { const v = cache.get(arg); if (v.valid) return v.value; }
  const r = yield* makeIter();
  return r;
}
const g = outer({}, new Map());
let step = g.next();
while (!step.done) { step = g.next(); }
const result = step.value;

// Destructuring null throws with the right message even after register churn.
function churnThenDestructure(arg: any, cache: Map<any, any>): string {
  const q2 = [arg, cache].map((v) => v);
  const c = cache.get(arg);
  const d = c && c.value;
  if (cache.has(arg)) { const v = cache.get(arg); if (v.valid) return v.value; }
  const src: any = null;
  try {
    const { x } = src;
    return String(x) + q2.length + d;
  } catch (e) {
    return (e as any).message;
  }
}
const msg = churnThenDestructure({}, new Map());

[seen.join(","), result, msg].join("|");

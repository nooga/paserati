// Exercises the for-of array fast path (OpIterFastCheck/OpArrayIterNext)
// against the behaviors it must preserve: hole normalization, length re-read
// on growth/truncation, index state shared with manual next() calls,
// deopt to the generic path when next is replaced, and iterator reuse after
// an early break (array iterators have no return method, so state persists).
// expect: 1,undefined,3,g1,g2,g3,t1,t2,m20,m30,r99,r99,b1,c2,c3
let log: any[] = [];

const a: any[] = [1, , 3];
for (const x of a) log.push(String(x));

const b: any[] = [1];
for (const x of b) {
  if (x < 3) b.push(x + 1);
  log.push("g" + x);
}

const c: any[] = [1, 2, 3, 4];
for (const x of c) {
  log.push("t" + x);
  if (x === 1) c.length = 2;
}

const it: any = [10, 20, 30][Symbol.iterator]();
it.next();
for (const x of it) log.push("m" + x);

const it2: any = [1, 2][Symbol.iterator]();
let calls = 0;
it2.next = () =>
  calls++ < 2 ? { value: 99, done: false } : { value: undefined, done: true };
for (const x of it2) log.push("r" + x);

const it3: any = [1, 2, 3][Symbol.iterator]();
for (const x of it3) {
  log.push("b" + x);
  break;
}
for (const x of it3) log.push("c" + x);

log.join(",");

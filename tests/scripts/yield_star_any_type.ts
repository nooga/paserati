// expect: 1-2-done
// Test that yield* accepts any type (TypeScript compatibility)
async function* gen() {
  const x: any = (async function*() { yield 1; yield 2; })();
  yield* x;
}

let g = gen();
let r1 = await g.next();
let r2 = await g.next();
let r3 = await g.next();

`${r1.value}-${r2.value}-${r3.done ? 'done' : 'not-done'}`;

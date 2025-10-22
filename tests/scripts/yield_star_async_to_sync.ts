// expect: 1-2-3-done
// Test yield* in async generator delegating to sync iterable
function* source() {
  yield 1;
  yield 2;
  yield 3;
}

async function* gen() {
  yield* source();
}

let g = gen();
let r1 = await g.next();
let r2 = await g.next();
let r3 = await g.next();
let r4 = await g.next();

`${r1.value}-${r2.value}-${r3.value}-${r4.done ? 'done' : 'not-done'}`;

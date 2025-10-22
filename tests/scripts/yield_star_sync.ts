// expect: 1-2-3-done
// Test yield* in regular (sync) generators
function* source() {
  yield 1;
  yield 2;
  yield 3;
}

function* gen() {
  yield* source();
}

let g = gen();
let r1 = g.next();
let r2 = g.next();
let r3 = g.next();
let r4 = g.next();

`${r1.value}-${r2.value}-${r3.value}-${r4.done ? 'done' : 'not-done'}`;

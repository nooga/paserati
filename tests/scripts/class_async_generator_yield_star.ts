// expect: 1-2-done
// Test yield* in class async generator method
async function* source() {
  yield 1;
  yield 2;
}

class C {
  async *gen() {
    yield* source();
  }
}

let c = new C();
let g = c.gen();
let r1 = await g.next();
let r2 = await g.next();
let r3 = await g.next();

`${r1.value}-${r2.value}-${r3.done ? 'done' : 'not-done'}`;

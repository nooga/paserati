// Test async generator with multiple yields using top-level await
// expect: done

async function* counter() {
  yield 1;
  yield 2;
  yield 3;
}

const gen = counter();
const r1 = await gen.next();
const r2 = await gen.next();
const r3 = await gen.next();
const r4 = await gen.next();

(r1.value === 1 && r1.done === false &&
 r2.value === 2 && r2.done === false &&
 r3.value === 3 && r3.done === false &&
 r4.done === true) ? "done" : "failed";

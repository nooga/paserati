// Test async generator return method using top-level await
// expect: success

async function* test() {
  yield 1;
  yield 2;
  yield 3;
}

const gen = test();
const r1 = await gen.next();
const r2 = await gen.return(99);
const r3 = await gen.next();

// Type cast r2.value to avoid type comparison error
// (the type system can't track that .return() injects the value)
const v2 = r2.value as any;
(r1.value === 1 && r1.done === false &&
 v2 === 99 && r2.done === true &&
 r3.done === true) ? "success" : "failed";

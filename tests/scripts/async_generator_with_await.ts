// Test async generator with await inside using top-level await
// expect: completed

async function* asyncCounter() {
  const p1 = Promise.resolve(10);
  yield await p1;

  const p2 = Promise.resolve(20);
  yield await p2;
}

const gen = asyncCounter();
const r1 = await gen.next();
const r2 = await gen.next();

(r1.value === 10 && r2.value === 20) ? "completed" : "failed";

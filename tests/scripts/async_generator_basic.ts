// Basic async generator test with top-level await
// expect: 42

async function* simpleAsyncGen() {
  yield 42;
}

const gen = simpleAsyncGen();
const result = await gen.next();
result.value;

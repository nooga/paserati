// Test: for-await-of at top level (TLA support)
// expect: 6

async function* asyncGen() {
  yield 1;
  yield 2;
  yield 3;
}

let sum = 0;
for await (const n of asyncGen()) {
  sum += n;
}

sum;

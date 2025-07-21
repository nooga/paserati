// Test for generator.return() method with explicit type annotation
// expect: 100

function* simpleGenerator(): Generator<number, number, any> {
  yield 1;
  yield 2;
  yield 3;
}

const gen = simpleGenerator();

// Get first value
const r1 = gen.next();
console.log("First value:", r1.value, "done:", r1.done);

// Call return() to force generator to complete with a specific value
const r2 = gen.return(100);
console.log("Return result:", r2.value, "done:", r2.done);

// Subsequent calls should return done:true with undefined value
const r3 = gen.next();
console.log("After return:", r3.value, "done:", r3.done);

// The return() result value
r2.value;
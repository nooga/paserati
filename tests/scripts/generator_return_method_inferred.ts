// Test for generator.return() method with inferred return type
// expect: 100

function* generatorWithReturn() {
  yield 1;
  yield 2;
  return 42; // This should make TReturn be number, not void
}

const gen = generatorWithReturn();

// Get first value
const r1 = gen.next();
console.log("First value:", r1.value, "done:", r1.done);

// Call return() with a number - should work since TReturn should be inferred as number
const r2 = gen.return(100);
console.log("Return result:", r2.value, "done:", r2.done);

// The return() result value
r2.value;
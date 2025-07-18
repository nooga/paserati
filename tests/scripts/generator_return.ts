// Generator with return statement test
// expect: 42

function* generatorWithReturn() {
  yield 10;
  yield 20;
  return 42; // Return value should appear in final iterator result
}

const gen = generatorWithReturn();
const result1 = gen.next(); // { value: 10, done: false }
const result2 = gen.next(); // { value: 20, done: false }
const result3 = gen.next(); // { value: 42, done: true }

console.log("First yield:", result1.value);
console.log("Second yield:", result2.value);
console.log("Return value:", result3.value);
console.log("Is done:", result3.done);

// Return value should be 42 when generator completes
result3.value;

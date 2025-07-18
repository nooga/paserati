// Basic generator functionality test
// expect: 42

function* simpleGenerator() {
  yield 42;
}

const gen = simpleGenerator();
console.log("Generator created:", typeof gen);
const result1 = gen.next();
console.log("After first next() call");

console.log("result1:", result1.value);
result1.value;

// Nested generators test
// expect: 1

function* innerGenerator() {
  yield 1;
  yield 2;
}

function* outerGenerator() {
  const gen = innerGenerator();
  const result = gen.next();
  yield result.value; // Should yield 1
  yield 3;
}

const gen = outerGenerator();
const result1 = gen.next();
const result2 = gen.next();

console.log("First nested yield:", result1.value);
console.log("Second nested yield:", result2.value);

result1.value;

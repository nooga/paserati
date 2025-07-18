// Generator with mixed types test
// expect: number

function* mixedTypeGenerator() {
  yield 42; // number
  yield "hello"; // string
  yield true; // boolean
}

const gen = mixedTypeGenerator();
const result1 = gen.next();
const result2 = gen.next();
const result3 = gen.next();

console.log("First type:", typeof result1.value);
console.log("Second type:", typeof result2.value);
console.log("Third type:", typeof result3.value);

typeof result1.value;

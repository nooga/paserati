// Generator with parameters test
// expect: 20

function* parameterGenerator() {
  const x = yield 10; // First yield returns 10, receives value from next()
  const y = yield x * 2; // Should yield received value * 2
  yield x + y; // Final yield
}

const gen = parameterGenerator();
const result1 = gen.next(); // Start generator, get 10
const result2 = gen.next(5); // Send 5 to generator, should get 5 * 2 = 10
const result3 = gen.next(15); // Send 15 to generator, should get 5 + 15 = 20

console.log("First yield:", result1.value);
console.log("Second yield:", result2.value);
console.log("Third yield:", result3.value);

// When parameter passing is implemented, this should be 20 (5 + 15)
// For now, testing the basic yield value
result3.value;

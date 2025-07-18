// Generator with loop yields test
// expect: 2

function* loopGenerator() {
  for (let i = 0; i < 5; i++) {
    yield i * 2;
  }
}

const gen = loopGenerator();
const result1 = gen.next(); // Should yield 0
const result2 = gen.next(); // Should yield 2
const result3 = gen.next(); // Should yield 4
const result4 = gen.next(); // Should yield 6
const result5 = gen.next(); // Should yield 8
const result6 = gen.next(); // Should be done

console.log("First yield:", result1.value);
console.log("Second yield:", result2.value);

// Second yield should be 2 (i=1, i*2=2)
result2.value;

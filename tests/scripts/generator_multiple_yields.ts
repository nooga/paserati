// Generator with multiple yields test
// expect: 1,2,3,done

function* multiYieldGenerator() {
  yield 1;
  yield 2;
  yield 3;
}

const gen = multiYieldGenerator();
const result1 = gen.next();
const result2 = gen.next();
const result3 = gen.next();
const result4 = gen.next(); // Should be done

console.log(
  result1.value + "," + result2.value + "," + result3.value + ",done"
);
result1.value + "," + result2.value + "," + result3.value + ",done";

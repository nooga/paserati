// expect: 23
// isSpreadableIterableType()/getSpreadElementType() only recognized
// Iterable<T>/Iterator<T> generic types, not Generator<T, TReturn, TNext> -
// a Generator implements Iterable<T> too, so spreading one in an array
// literal, a rest/variadic call, or a destructuring pattern is valid JS
// (confirmed against real tsc) but the type checker rejected all three with
// "spread syntax can only be applied to arrays". Noticed while verifying
// #204 ({ *static(e) { yield e } } - an object-literal generator method).
function* gen(n: number) {
  yield n;
  yield n + 1;
  yield n + 2;
}

// array literal spread
const arr = [...gen(1)]; // [1, 2, 3]

// spread into a rest/variadic parameter
function sumAll(...nums: number[]) {
  return nums.reduce((a, b) => a + b, 0);
}
const variadicSum = sumAll(...gen(4)); // 4+5+6 = 15

// array-destructuring (including rest) from a generator
const [first, ...rest] = gen(0); // first=0, rest=[1,2]

arr.reduce((a, b) => a + b, 0) + variadicSum + first + rest.length; // 6 + 15 + 0 + 2

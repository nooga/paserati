// expect: done
// Test for-of loops with array destructuring

// Basic array destructuring in for-of
const pairs: [number, number][] = [[1, 2], [3, 4], [5, 6]];
for (const [x, y] of pairs) {
  console.log(x + "," + y);
}

// Array destructuring with let
const tuples: [string, number][] = [["a", 1], ["b", 2]];
for (let [letter, num] of tuples) {
  console.log(letter + ":" + num);
}

// Nested array destructuring
const nested = [[[1, 2], [3, 4]], [[5, 6], [7, 8]]];
for (const [[a, b], [c, d]] of nested) {
  console.log(a + b + c + d);
}

// Array destructuring with rest
const lists: number[][] = [[1, 2, 3], [4, 5, 6, 7]];
for (const [first, ...rest] of lists) {
  console.log(first + ":" + rest.length);
}

// Array destructuring with default values
const withDefaults: [number, number?][] = [[1, 2], [3]];
for (const [x, y = 0] of withDefaults) {
  console.log(x + "," + y);
}

("done");

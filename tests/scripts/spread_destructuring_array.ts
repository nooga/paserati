// Test spread with array destructuring
let source = [1, 2, 3, 4, 5];
let [first, ...rest] = source;
[first, ...rest];
// expect: [1, 2, 3, 4, 5]
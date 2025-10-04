// expect: 6
// Test array elision in destructuring
let [, a, , b] = [1, 2, 3, 4];
a + b;

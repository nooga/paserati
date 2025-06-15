// Test rest elements in let declaration
let [first, ...rest] = [10, 20, 30, 40];
rest[1];
// expect: 30
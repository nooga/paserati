// expect: 300
// Test object with nested array destructuring and defaults
let {prop: [x, y] = [100, 200]} = {};
x + y;

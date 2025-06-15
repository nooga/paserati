// Test object rest with multiple extracted properties
let a = 0;
let b = 0;
let rest = {};
{a, b, ...rest} = {a: 1, b: 2, c: 3, d: 4};
b;
// expect: 2
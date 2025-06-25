// Test spread with object destructuring
let source = {a: 1, b: 2, c: 3, d: 4};
let {a, ...rest} = source;
({a, ...rest});
// expect: {a: 1, b: 2, c: 3, d: 4}
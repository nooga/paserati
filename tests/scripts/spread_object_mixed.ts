// Test object spread with literals and expressions
let obj = {b: 2, c: 3};
({a: 1, ...obj, d: 4, ...{e: 5, f: 6}, g: 7});
// expect: {a: 1, b: 2, c: 3, d: 4, e: 5, f: 6, g: 7}
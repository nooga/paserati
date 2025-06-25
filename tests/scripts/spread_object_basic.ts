// Test basic object spread syntax
let obj1 = {a: 1, b: 2};
let obj2 = {c: 3, d: 4};
({...obj1, ...obj2});
// expect: {a: 1, b: 2, c: 3, d: 4}
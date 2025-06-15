// Test object rest elements in let declaration
let {a, ...others} = {a: 100, b: 200, c: 300};
a;
// expect: 100
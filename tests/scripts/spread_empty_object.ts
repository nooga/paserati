// Test spread with empty objects
let empty = {};
let props = {a: 1, b: 2};
({...empty, ...props, ...empty});
// expect: {a: 1, b: 2}
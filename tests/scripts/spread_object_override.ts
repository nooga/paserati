// Test object spread property override
let base = {a: 1, b: 2};
({a: 10, ...base, b: 20});
// expect: {a: 1, b: 20}
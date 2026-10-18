// Test object spread property override
// (a is optional in base's type: a required one would be TS2783)
let base: { a?: number; b: number } = {a: 1, b: 2};
({a: 10, ...base, b: 20});
// expect: {a: 1, b: 20}

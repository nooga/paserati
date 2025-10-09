// BigInt keys in destructuring should be converted to strings
let { 1n: a } = { "1": "foo" };
a;
// expect: foo

// Test that rest object contains all remaining properties
let {a, ...rest} = {a: 1, b: 2, c: 3, d: 4, e: 5};

// Check counts - rest should have 4 properties (b, c, d, e)
let keys = Object.keys(rest);
keys.length;
// expect: 4
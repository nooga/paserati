// Test that rest object contains non-extracted properties
let {a, b, ...rest} = {a: 1, b: 2, c: 3, d: 4, e: 5};

// Check that rest contains c
rest.c;
// expect: 3
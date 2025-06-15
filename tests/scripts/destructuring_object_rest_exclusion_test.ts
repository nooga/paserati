// Test that rest object properly excludes extracted properties
let {a, b, ...rest} = {a: 1, b: 2, c: 3, d: 4, e: 5};

// Check that rest doesn't contain extracted properties
let hasA = "a" in rest;
let hasB = "b" in rest;
let hasC = "c" in rest;

// Should be false, false, true
hasA;
// expect: false
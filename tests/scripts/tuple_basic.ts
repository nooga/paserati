// Basic tuple type test - tests tuple type parsing and basic functionality
// expect: undefined

// For now, test that tuple type annotations parse correctly
// Array-to-tuple assignment will be implemented later with contextual typing
let point: [number, number];
let mixed: [string, number, boolean];

// Test optional elements parsing
let withOptional: [string, number?];

// Test that tuple-to-array assignment would work (when we have tuple values)
let nums: number[];

// Test empty tuple parsing
let empty: [];

undefined;

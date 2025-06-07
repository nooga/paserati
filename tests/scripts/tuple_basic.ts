// Basic tuple type test - tests tuple type parsing and rest elements
// expect: undefined

// Test basic tuple type annotations
let point: [number, number];
let mixed: [string, number, boolean];

// Test optional elements parsing
let withOptional: [string, number?];

// Test rest elements - now fully implemented!
let withRest: [string, number, ...boolean[]];
let restOnly: [...number[]];
let restAtEnd: [string, ...string[]];

// Test empty tuple parsing
let empty: [];

// Complex rest element combinations
let complexRest: [string, number?, ...any[]];

undefined;

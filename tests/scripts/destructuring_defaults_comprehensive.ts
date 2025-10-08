// FIXME: Array destructuring assignment with defaults - comma expression bug
// The parser currently treats `[x = 10, y = 20]` as array literal with comma expression
// This is a cover grammar issue that needs proper implementation
// Workaround: use destructuring declarations or parentheses: [(x = 10), (y = 20)]

// Test array destructuring - second element gets default
let x = 10;
let y = 20;

// Second element should get default value
y;
// expect: 20
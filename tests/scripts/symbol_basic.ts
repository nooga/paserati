// Test basic Symbol functionality

// Create symbols
const sym1 = Symbol();
const sym2 = Symbol();
const sym3 = Symbol("description");
const sym4 = Symbol("description");

// Symbols are unique - the final expression should be false
sym1 === sym2;

// expect: false
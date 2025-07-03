// Test Symbol static properties and methods

// Test Symbol.for and Symbol.keyFor
const sym1 = Symbol.for("global_key");
const sym2 = Symbol.for("global_key");
sym1 === sym2; // true (same global symbol)

// expect: true
// Test Symbol.keyFor
const sym = Symbol.for("test_key");
Symbol.keyFor(sym);

// expect: test_key
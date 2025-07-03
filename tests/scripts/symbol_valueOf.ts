// Test Symbol valueOf method
const sym = Symbol("test");
sym.valueOf() === sym;

// expect: true
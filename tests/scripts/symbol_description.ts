// Test Symbol description via toString

const sym1 = Symbol();
const sym2 = Symbol("test");

// Test toString which includes description
sym2.toString();

// expect: Symbol(test)
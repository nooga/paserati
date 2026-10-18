// Test Error with number argument
let err = new Error(42 as any); // tsc rejects a number message; the runtime coerces it
err.message;
// expect: 42
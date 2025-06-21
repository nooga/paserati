// Test Error with undefined message
let err = new Error(undefined);
err.message;
// expect: 
// Test Error property modification
let err = new Error("Initial message");
err.name = "CustomError";
err.message = "Modified message";
err.toString();
// expect: CustomError: Modified message
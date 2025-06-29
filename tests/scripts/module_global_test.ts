// Simple module with global variable - should not overwrite builtins
export const message = "Hello from module!";
export const number = 42;
console.log("message:", message);
console.log("number:", number);
message; // Return the message value

// expect: Hello from module!
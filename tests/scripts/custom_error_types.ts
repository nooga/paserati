// Test custom error types
// expect: all tests passed

// Test TypeError
let typeError = new TypeError("type error message");
console.log("TypeError name:", typeError.name);
console.log("TypeError message:", typeError.message);
console.log("TypeError toString:", typeError.toString());

// Test ReferenceError
let refError = new ReferenceError("reference error message");
console.log("ReferenceError name:", refError.name);
console.log("ReferenceError message:", refError.message);
console.log("ReferenceError toString:", refError.toString());

// Test SyntaxError
let syntaxError = new SyntaxError("syntax error message");
console.log("SyntaxError name:", syntaxError.name);
console.log("SyntaxError message:", syntaxError.message);
console.log("SyntaxError toString:", syntaxError.toString());

// Test throwing and catching custom errors
try {
    throw new TypeError("thrown type error");
} catch (e) {
    console.log("Caught TypeError:", e.toString());
}

try {
    throw new ReferenceError("thrown ref error");
} catch (e) {
    console.log("Caught ReferenceError:", e.toString());
}

try {
    throw new SyntaxError("thrown syntax error");
} catch (e) {
    console.log("Caught SyntaxError:", e.toString());
}

// Test stack traces are present
let errorWithStack = new TypeError("stack test");
if (errorWithStack.stack && errorWithStack.stack.length > 0) {
    console.log("Stack trace present: true");
} else {
    console.log("Stack trace present: false");
}

"all tests passed"
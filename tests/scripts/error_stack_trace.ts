// Test Error stack trace functionality

function outer() {
    return inner();
}

function inner() {
    return deep();
}

function deep() {
    return new Error("Test error");
}

let err = outer();

// Test that Error objects have stack property and contains function names
let hasStackProperty = typeof err.stack === "string";
let hasDeepInStack = err.stack.includes("deep");
let hasInnerInStack = err.stack.includes("inner");
let hasOuterInStack = err.stack.includes("outer");

// Test with thrown errors
let thrownErrorStackWorks = false;
function throwingFunction() {
    throw new Error("Thrown error");
}

try {
    throwingFunction();
} catch (e) {
    thrownErrorStackWorks = typeof e.stack === "string" && e.stack.includes("throwingFunction");
}

// Return overall result
// expect: true
hasStackProperty && hasDeepInStack && hasInnerInStack && hasOuterInStack && thrownErrorStackWorks;
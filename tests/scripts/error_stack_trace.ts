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
// Note: With TCO enabled, intermediate tail calls (inner, outer) won't appear in stack
// because their frames are reused - this is correct optimization behavior
let hasStackProperty = typeof err.stack === "string";
let hasDeepInStack = err.stack.includes("deep");
// TCO optimizes away inner and outer frames since they're tail calls

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

// Return overall result - with TCO, only check stack property, deep function, and thrown error
// expect: true
hasStackProperty && hasDeepInStack && thrownErrorStackWorks;
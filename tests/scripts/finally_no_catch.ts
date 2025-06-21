// Test try/finally (no catch) with exception - should be uncaught

console.log("before try");

try {
    console.log("in try");
    throw "uncaught error";
} finally {
    console.log("in finally");
}

console.log("this should not execute");

// expect_runtime_error: Uncaught exception: uncaught error
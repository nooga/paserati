// Test try/catch/finally execution

console.log("before try");

try {
    console.log("in try");
    throw "test error";
    console.log("after throw"); // should not execute
} catch (e) {
    console.log("in catch:", e);
} finally {
    console.log("in finally");
}

console.log("after try/catch/finally");

// expect: undefined
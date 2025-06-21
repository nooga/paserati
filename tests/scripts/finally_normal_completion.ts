// Test finally block execution on normal completion

console.log("before try");

try {
    console.log("in try");
} finally {
    console.log("in finally");
}

console.log("after try/finally");

// expect: undefined
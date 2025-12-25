// Test basic finally block parsing

// Try with only finally
try {
    console.log("try block");
} finally {
    console.log("finally block");
}

// Try with catch and finally
try {
    console.log("try block");
} catch (e) {
    console.log("catch block");
} finally {
    console.log("finally block");
}

// expect: undefined
// Error object integration with try/catch
let finalResult;
try {
    let err = new Error();
    err.name = "ValidationError";
    err.message = "Required field missing";
    err.field = "username";
    throw err;
} catch (e) {
    // Demonstrate full Error object functionality
    let summary = e.name + ": " + e.message + " (field: " + e.field + ")";
    finalResult = summary;
}
finalResult;
// expect: ValidationError: Required field missing (field: username)
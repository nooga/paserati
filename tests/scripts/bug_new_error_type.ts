// Test that new Error() returns a properly typed Error object
// Bug 6a: Error constructor needs ConstructSignatures
const err = new Error("test message");
err.message;

// expect: test message

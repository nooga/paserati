// expect_runtime_error: Invalid ArrayBuffer length
// Test error handling for invalid buffer sizes

// ArrayBuffer size must be non-negative
new ArrayBuffer(-1);
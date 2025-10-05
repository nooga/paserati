// Test top-level await with promise rejection
// expect_runtime_error: Uncaught (in promise): failed

const promise = Promise.reject("failed");
await promise;

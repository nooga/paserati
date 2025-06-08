// Test error cases for explicit 'this' parameter

// 'this' parameter without type annotation should fail
// expect_compile_error: 'this' parameter must have a type annotation
function BadFunction1(this) {
    return this;
}
// Test error case for multiple 'this' parameters

// 'this' parameter can only be first parameter
// expect_compile_error: 'this' parameter can only be the first parameter
function BadFunction(x: number, this: { value: number }) {
    return this.value + x;
}
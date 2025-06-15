// Test type checking for function parameter destructuring
function badFunction([a, b]: [string, string]) {
    return a + b;
}

// This should cause a type error
let nums: [number, number] = [1, 2];
let result = badFunction(nums); // [number, number] instead of [string, string]
result;
// expect_compile_error: cannot assign type